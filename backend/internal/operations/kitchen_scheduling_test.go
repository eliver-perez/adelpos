//go:build integration

package operations_test

import (
	"testing"
	"time"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
)

// accountUpdate reproduce el documento editable de una cuenta tal como lo envía el escritorio.
func accountUpdate(account operations.Account) map[string]any {
	scheduleKind := ""
	if account.ScheduleKind != nil {
		scheduleKind = *account.ScheduleKind
	}
	return map[string]any{
		"revision":         account.Revision,
		"name":             account.Name,
		"mode":             account.Mode,
		"table_id":         account.TableID,
		"price_list_id":    account.PriceListID,
		"customer_name":    account.CustomerName,
		"phone":            account.Phone,
		"origin":           account.Origin,
		"notes":            account.Notes,
		"scheduled_local":  account.ScheduledLocal,
		"schedule_kind":    scheduleKind,
		"assigned_user_id": account.AssignedUserID,
	}
}

func testKitchenSchedulingAndGrouping(t *testing.T, fixture *restaurantFixture, printer kitchen.Printer, first, second kitchen.Station, product catalog.Product) {
	t.Run("combo enruta hijos sin duplicar unidades y productos nuevos generan comanda normal", func(t *testing.T) {
		fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{
			ProductID:  &fixture.fries.ID,
			Mode:       "prepare",
			StationIDs: []string{second.ID},
		}, 200)
		before := kitchenCount(t, fixture, "kitchen_unit_releases")
		bill := fixture.capture(t, newAccount(t, fixture, "Combo por estación"), []catalog.LineRequest{{
			ProductID: fixture.combo.ID,
			Quantity:  2,
			Customization: catalog.Customization{Components: []catalog.ComponentSelection{{
				SlotID:    fixture.combo.Slots[0].ID,
				ProductID: fixture.fries.ID,
				Quantity:  2,
			}}},
		}})
		release := kitchenReleases(t, fixture, bill)[0]
		if kitchenCount(t, fixture, "kitchen_unit_releases") != before+2 {
			t.Fatal("Un hijo de combo creó otra unidad comercial o liberación canónica")
		}
		found := false
		for _, task := range kitchenTasks(t, fixture) {
			if task.ReleaseID != release.ID || task.StationID != second.ID {
				continue
			}
			for _, part := range task.Instructions.Parts {
				if part.ProductID == fixture.fries.ID && part.Quantity == 2 {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("Las papas elegidas no llegaron con su cantidad a la estación explícita")
		}
		previousTickets := kitchenCount(t, fixture, "kitchen_tickets")
		bill = fixture.capture(t, bill, []catalog.LineRequest{{ProductID: product.ID, Quantity: 1}})
		var normal, other int
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FILTER(WHERE kind='normal'),count(*) FILTER(WHERE kind<>'normal')
			FROM app.kitchen_tickets WHERE release_id=$1`, kitchenReleases(t, fixture, bill)[0].ID).Scan(&normal, &other); err != nil {
			t.Fatal(err)
		}
		if normal != 2 || other != 0 || kitchenCount(t, fixture, "kitchen_tickets") != previousTickets+2 {
			t.Fatal("Agregar productos nuevos no produjo comandas normales separadas")
		}
	})
	t.Run("cocina agrupación unificada y preparación por pedido", func(t *testing.T) {
		settings := decodeResponse[kitchen.Settings](t, fixture.adminCall(t, "GET", "kitchen/settings", nil, 200))
		settings.Grouping = "unified"
		settings.Tracking = "order"
		fixture.mutate(t, "PUT", "kitchen/settings", settings, 200)
		before := kitchenCount(t, fixture, "kitchen_print_jobs")
		bill := fixture.capture(t, newAccount(t, fixture, "Unificado"), []catalog.LineRequest{{ProductID: product.ID, Quantity: 1}})
		if kitchenCount(t, fixture, "kitchen_print_jobs") != before+1 {
			t.Fatal("La agrupación unificada generó más de un trabajo físico")
		}
		releases := kitchenReleases(t, fixture, bill)
		release := releases[0]
		var sections int
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT jsonb_array_length(document->'sections') FROM app.kitchen_tickets WHERE release_id=$1", release.ID).Scan(&sections); err != nil {
			t.Fatal(err)
		}
		if sections != 2 {
			t.Fatal("Se mezclaron las estaciones sin conservar secciones identificadas")
		}
		tasks := kitchenTasks(t, fixture)
		for _, task := range tasks {
			if task.ReleaseID == release.ID {
				fixture.mutate(t, "POST", "kitchen/states", map[string]any{"state": "ready", "tasks": []any{map[string]any{"id": task.ID, "revision": task.Revision}}}, 400)
				break
			}
		}
		fixture.mutate(t, "POST", "kitchen/states", map[string]any{"release_id": release.ID, "revision": release.Revision, "state": "ready"}, 200)
		if fixture.refresh(t, bill.Account.ID).Account.PreparationState != "ready" {
			t.Fatal("No se actualizó el pedido completo")
		}
		adapter := &recordingTransport{state: "submitted"}
		if _, err := (&kitchen.Worker{Pool: fixture.pool, Transport: adapter}).PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if len(adapter.jobs) != 1 {
			t.Fatal("No se recuperó la cola pendiente")
		}
		unitsBefore := kitchenCount(t, fixture, "kitchen_unit_releases")
		jobsBefore := kitchenCount(t, fixture, "kitchen_print_jobs")
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/move", map[string]any{
			"revision": bill.Account.Revision,
			"table_id": fixture.secondTable.ID,
		}, 200))
		if kitchenCount(t, fixture, "kitchen_print_jobs") != jobsBefore {
			t.Fatal("Mover una cuenta preparada a otra mesa generó otra comanda")
		}
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/split", map[string]any{
			"revision": bill.Account.Revision, "new_account": map[string]string{"name": "Separada preparada"},
			"selections": []any{map[string]any{"version_id": bill.Lines[0].VersionID, "quantity": 1}},
		}, 200)
		if kitchenCount(t, fixture, "kitchen_unit_releases") != unitsBefore || kitchenCount(t, fixture, "kitchen_print_jobs") != jobsBefore {
			t.Fatal("La división de cuenta produjo otra preparación o impresión")
		}
	})
	t.Run("cocina programación automática y liberación manual anticipada", func(t *testing.T) {
		settings := decodeResponse[kitchen.Settings](t, fixture.adminCall(t, "GET", "kitchen/settings", nil, 200))
		settings.AutomaticRelease = true
		settings.LeadMinutes = 25
		fixture.mutate(t, "PUT", "kitchen/settings", settings, 200)
		location, _ := time.LoadLocation("America/Monterrey")
		scheduled := time.Now().In(location).Add(2 * time.Hour).Format("2006-01-02T15:04")
		bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "Programado automático", "mode": "takeaway", "origin": "Ventanilla", "scheduled_local": scheduled, "schedule_kind": "delivery",
		}, 200))
		bill = fixture.capture(t, bill, []catalog.LineRequest{{ProductID: product.ID, Quantity: 1}})
		release := kitchenReleases(t, fixture, bill)[0]
		if release.State != "scheduled" || release.DueAt == nil || !release.DueAt.Equal(release.ScheduledAt.Add(-25*time.Minute)) {
			t.Fatal("La anticipación programada no se conservó")
		}
		worker := kitchen.Worker{Pool: fixture.pool, Transport: &recordingTransport{state: "submitted"}}
		before := kitchenCount(t, fixture, "kitchen_unit_releases")
		if err := worker.ReleaseDue(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if kitchenCount(t, fixture, "kitchen_unit_releases") != before {
			t.Fatal("El programador liberó antes de tiempo")
		}
		// Vencimiento controlado en la base temporal; Go compara con su reloj real.
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_releases SET due_at=now()-interval '1 second' WHERE id=$1", release.ID); err != nil {
			t.Fatal(err)
		}
		if err := worker.ReleaseDue(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if err := (&kitchen.Worker{Pool: fixture.pool}).ReleaseDue(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if kitchenCount(t, fixture, "kitchen_unit_releases") != before+1 {
			t.Fatal("Un reinicio lógico perdió o duplicó la liberación")
		}
		manual := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "Programado manual", "mode": "takeaway", "scheduled_local": scheduled, "schedule_kind": "preparation",
		}, 200))
		manual = fixture.capture(t, manual, []catalog.LineRequest{{ProductID: product.ID, Quantity: 1}})
		next := kitchenReleases(t, fixture, manual)[0]
		fixture.mutate(t, "POST", "kitchen/releases/"+next.ID+"/release", map[string]any{"revision": next.Revision}, 200)
		if fixture.refresh(t, manual.Account.ID).Account.PreparationState != "released" {
			t.Fatal("La liberación manual autorizada no se ejecutó")
		}
	})
	t.Run("reprogramación de cocina sobrevive a ediciones de la cuenta", func(t *testing.T) {
		location, _ := time.LoadLocation("America/Monterrey")
		original := time.Now().In(location).Add(3 * time.Hour).Truncate(time.Minute)
		bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "Reprogramado en cocina", "mode": "takeaway", "origin": "Teléfono",
			"scheduled_local": original.Format("2006-01-02T15:04"), "schedule_kind": "preparation",
		}, 200))
		bill = fixture.capture(t, bill, []catalog.LineRequest{{ProductID: product.ID, Quantity: 1}})
		release := kitchenReleases(t, fixture, bill)[0]
		kitchenTime := original.Add(time.Hour)
		fixture.mutate(t, "POST", "kitchen/releases/"+release.ID+"/schedule", map[string]any{
			"revision": release.Revision, "scheduled_local": kitchenTime.Format("2006-01-02T15:04"),
			"automatic": false, "lead_minutes": 10,
		}, 200)

		// Corregir el teléfono no debe revertir la hora elegida en cocina.
		account := fixture.refresh(t, bill.Account.ID).Account
		update := accountUpdate(account)
		update["phone"] = "8180000000"
		fixture.mutate(t, "PATCH", "restaurant/accounts/"+account.ID, update, 200)
		current := kitchenReleases(t, fixture, bill)[0]
		if current.ScheduledAt == nil || !current.ScheduledAt.Equal(kitchenTime) || current.Automatic || current.LeadMinutes != 10 {
			t.Fatal("Una edición sin cambio de horario revirtió la reprogramación de cocina")
		}

		// Una nueva programación capturada en la cuenta sí sustituye la de cocina.
		accountTime := original.Add(2 * time.Hour)
		account = fixture.refresh(t, bill.Account.ID).Account
		update = accountUpdate(account)
		update["scheduled_local"] = accountTime.Format("2006-01-02T15:04")
		fixture.mutate(t, "PATCH", "restaurant/accounts/"+account.ID, update, 200)
		current = kitchenReleases(t, fixture, bill)[0]
		if current.ScheduledAt == nil || !current.ScheduledAt.Equal(accountTime) || current.State != "scheduled" {
			t.Fatal("La nueva programación de la cuenta no llegó a la liberación pendiente")
		}
	})
	t.Run("cocina rutas inexistentes no aparentan liberación y sin preparación es explícito", func(t *testing.T) {
		category := decodeResponse[catalog.Category](t, fixture.adminCall(t, "POST", "catalog/categories", catalog.Category{Metadata: catalog.Metadata{Name: "Sin ruta temporal", Active: true}}, 200))
		item := decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
			Metadata: catalog.Metadata{Name: "Producto sin ruta", Active: true}, CategoryID: category.ID, Kind: "product", Code: "SIN_RUTA", BasePrice: "10",
		}, 200))
		bill := fixture.capture(t, newAccount(t, fixture, "Falta ruta"), []catalog.LineRequest{{ProductID: item.ID, Quantity: 1}})
		release := kitchenReleases(t, fixture, bill)[0]
		if release.State != "blocked" || len(release.Warnings) == 0 || bill.Account.PreparationState != "pending_release" {
			t.Fatal("Una ruta inexistente aparentó entrega a cocina")
		}
		fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &item.ID, Mode: "none", StationIDs: []string{}}, 200)
		fixture.mutate(t, "POST", "kitchen/releases/"+release.ID+"/release", map[string]any{"revision": release.Revision}, 200)
		if fixture.refresh(t, bill.Account.ID).Account.PreparationState != "ready" {
			t.Fatal("No se respetó Sin preparación")
		}
	})
}
