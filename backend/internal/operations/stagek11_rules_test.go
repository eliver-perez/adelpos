//go:build integration

package operations_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa K1.1 (Fase A): las demás reglas de la §2 y la §3 de la especificación y de las decisiones K11-D1 a K11-D5, sobre
// la base y la configuración del propietario de TestStageK11UnroutedProducts (stagek11_unrouted_test.go): «Enviar a una
// estación», una estación o una impresora desactivadas, combos, una modificación con la ruta congelada, los pendientes al
// cobrar, al cerrar con la entrega, al cancelar, al combinar y al reabrir, un pedido para llevar cobrado y sin entregar,
// un bloqueado de antes de la 0.17.0, los dos 409 y la cobertura con cada aviso.

// stationTasks son los platillos (nombre de la partida) de las tareas vivas de la cuenta, por estación.
func (fixture *unroutedFixture) stationTasks(t *testing.T, accountID string) map[string][]string {
	t.Helper()
	rows, err := fixture.pool.Query(fixture.ctx, `SELECT t.station_id::text,t.instructions->>'name' FROM app.kitchen_tasks t
        JOIN app.restaurant_item_units u ON u.id=t.unit_id WHERE u.account_id=$1 AND t.state<>'cancelled' ORDER BY 1,2`, accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := map[string][]string{}
	for rows.Next() {
		var station, name string
		if err := rows.Scan(&station, &name); err != nil {
			t.Fatal(err)
		}
		result[station] = append(result[station], name)
	}
	return result
}

// counter abre una cuenta de salón sin mesa a nombre del mesero y la envía a cocina.
func (fixture *unroutedFixture) counter(t *testing.T, name string, items ...operations.DraftItem) string {
	t.Helper()
	opened := fixture.open(t, fixture.admin, map[string]any{"name": name, "assigned_user_id": fixture.waiterID}, items...)
	fixture.confirm(t, fixture.admin, opened.Account.ID)
	return opened.Account.ID
}

// takeaway abre un pedido para llevar programado dentro de dos horas y lo envía: la cocina lo recibe a su hora.
func (fixture *unroutedFixture) takeaway(t *testing.T, name string, items ...operations.DraftItem) string {
	t.Helper()
	location, err := time.LoadLocation(fixture.timezone(t))
	if err != nil {
		t.Fatal(err)
	}
	scheduled := time.Now().In(location).Add(2 * time.Hour).Truncate(time.Hour).Format("2006-01-02T15:04")
	opened := fixture.open(t, fixture.admin, map[string]any{"mode": "takeaway", "name": name, "origin": "Ventanilla",
		"scheduled_local": scheduled, "schedule_kind": "delivery"}, items...)
	fixture.confirm(t, fixture.admin, opened.Account.ID)
	return opened.Account.ID
}

func (fixture *unroutedFixture) timezone(t *testing.T) string {
	t.Helper()
	var zone string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT timezone FROM app.establishment").Scan(&zone); err != nil {
		t.Fatal(err)
	}
	return zone
}

// release es la única liberación de la cuenta (por su cuenta original).
func (fixture *unroutedFixture) release(t *testing.T, accountID string) (string, string, int) {
	t.Helper()
	var id, state string
	var revision int
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT id::text,state,revision FROM app.kitchen_releases WHERE account_id=$1`,
		accountID).Scan(&id, &state, &revision); err != nil {
		t.Fatalf("liberación de la cuenta: %v", err)
	}
	return id, state, revision
}

// pendingChange es el aviso pendiente de la cuenta (terminado o no).
func (fixture *unroutedFixture) pendingChange(t *testing.T, accountID string) (string, int, *string) {
	t.Helper()
	var eventID string
	var revision int
	var reason *string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT event_id::text,revision,dismissed_reason FROM app.kitchen_pending_changes
        WHERE account_id=$1`, accountID).Scan(&eventID, &revision, &reason); err != nil {
		t.Fatalf("aviso pendiente de la cuenta: %v", err)
	}
	return eventID, revision, reason
}

// onBoard dice cuántas liberaciones y avisos de la cuenta muestra «Por enviar a cocina» (GET kitchen/board).
func (fixture *unroutedFixture) onBoard(t *testing.T, accountID string) (int, int) {
	t.Helper()
	board := decodeResponse[struct {
		Releases       []map[string]any `json:"releases"`
		PendingChanges []map[string]any `json:"pending_changes"`
	}](t, fixture.adminCall(t, "GET", "kitchen/board", nil, 200))
	releases, changes := 0, 0
	for _, release := range board.Releases {
		if release["account_id"] == accountID {
			releases++
		}
	}
	for _, change := range board.PendingChanges {
		if change["account_id"] == accountID {
			changes++
		}
	}
	return releases, changes
}

// dismissedHistory comprueba la fila del historial de cocina de lo que se dio por terminado: el motivo y el actor de la
// operación (el administrador).
func (fixture *unroutedFixture) dismissedHistory(t *testing.T, kind, entityID, reason string) {
	t.Helper()
	var found int
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*)::int FROM app.kitchen_history h JOIN app.users u ON u.id=h.actor_id
        WHERE h.kind=$1 AND h.entity_id=$2 AND h.after_value->>'reason'=$3 AND u.username='admin'`, kind, entityID, reason).
		Scan(&found); err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Fatalf("historial %s de %s con el motivo %s: %d filas", kind, entityID, reason, found)
	}
}

// setUnrouted guarda «Productos sin ruta» con el valor que tiene hoy como esperado.
func (fixture *unroutedFixture) setUnrouted(t *testing.T, stationID *string) kitchen.UnroutedView {
	t.Helper()
	current := decodeResponse[kitchen.SettingsView](t, fixture.adminCall(t, "GET", "kitchen/settings", nil, 200))
	return decodeResponse[kitchen.UnroutedView](t, fixture.mutate(t, "PUT", "kitchen/unrouted", map[string]any{
		"station_id": stationID, "expected_station_id": current.UnroutedStationID,
	}, 200))
}

// setStationActive activa o desactiva una estación con su revisión vigente.
func (fixture *unroutedFixture) setStationActive(t *testing.T, station *kitchen.Station, active bool) {
	t.Helper()
	station.Active = active
	*station = decodeResponse[kitchen.Station](t, fixture.mutate(t, "PATCH", "kitchen/stations/"+station.ID, *station, 200))
}

// setPrinterActive activa o desactiva la impresora de comandas de la prueba.
func (fixture *unroutedFixture) setPrinterActive(t *testing.T, name string, active bool) {
	t.Helper()
	for _, printer := range decodeResponse[[]kitchen.Printer](t, fixture.adminCall(t, "GET", "kitchen/printers", nil, 200)) {
		if printer.Name == name {
			printer.Active = active
			fixture.mutate(t, "PATCH", "kitchen/printers/"+printer.ID, printer, 200)
			return
		}
	}
	t.Fatalf("no existe la impresora %s", name)
}

// modifyNotes cambia las observaciones de la primera partida activa de la cuenta (una modificación de algo enviado).
func (fixture *unroutedFixture) modifyNotes(t *testing.T, accountID, notes string) operations.Bill {
	t.Helper()
	bill := fixture.refresh(t, accountID)
	line := bill.Lines[0]
	line.Request.Customization.Notes = notes
	body := map[string]any{"revision": bill.Account.Revision, "version_id": line.VersionID, "line": line.Request,
		"reason": "Cambio del cliente"}
	path := "restaurant/accounts/" + accountID
	preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", path+"/edit-preview", body, 200))
	body["expected_fingerprint"] = preview.Fingerprint
	return decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", path+"/edit", body, 200))
}

// pay cobra en efectivo el saldo completo de la cuenta.
func (fixture *unroutedFixture) pay(t *testing.T, accountID string) {
	t.Helper()
	finance := decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+accountID, nil, 200))
	fixture.mutate(t, "POST", "payments/accounts/"+accountID+"/payments", map[string]any{
		"method_id": fixture.cashMethod, "mode": "total", "expected_balance": finance.Balance,
	}, 200)
}

// cancelAccount usa «Cancelar cuenta» con un motivo del catálogo.
func (fixture *unroutedFixture) cancelAccount(t *testing.T, accountID string) {
	t.Helper()
	reasons := decodeResponse[[]operations.CancelReason](t, fixture.adminCall(t, "GET", "restaurant/cancel-reasons", nil, 200))
	fixture.mutate(t, "POST", "restaurant/accounts/"+accountID+"/cancel-account", map[string]any{
		"revision": fixture.refresh(t, accountID).Account.Revision, "reason_id": reasons[0].ID, "description": "Prueba K1.1",
	}, 200)
}

// releaseNow es «Enviar a cocina ahora» con la revisión dada; devuelve el estado y el cuerpo.
func (fixture *unroutedFixture) releaseNow(t *testing.T, releaseID, operationID string, revision, status int) []byte {
	t.Helper()
	return fixture.adminCall(t, "POST", "kitchen/releases/"+releaseID+"/release",
		map[string]any{"operation_id": operationID, "revision": revision}, status)
}

func (fixture *unroutedFixture) state(t *testing.T, accountID string) string {
	t.Helper()
	return fixture.refresh(t, accountID).Account.State
}

// testStageK11SendToStation: con «Enviar a una estación», la Coca Cola y las papas llegan a esa estación y lo demás a
// Cocina; la decisión queda congelada (unrouted) en la liberación.
func testStageK11SendToStation(t *testing.T, fixture *unroutedFixture) {
	fixture.barra = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Barra K1.1", Active: true, Reception: "kds", Tracking: "inherit", Grouping: "inherit",
	}, 200))
	saved := fixture.setUnrouted(t, &fixture.barra.ID)
	if saved.Effective != "station" || saved.StationName == nil || *saved.StationName != "Barra K1.1" {
		t.Fatalf("«Enviar a una estación»: %+v", saved)
	}
	accountID := fixture.counter(t, "Barra K1.1", draftLine(fixture.machacado, 1), draftLine(fixture.coca, 1), draftLine(fixture.papas, 1))
	tasks := fixture.stationTasks(t, accountID)
	if !slices.Equal(tasks[fixture.cocina.ID], []string{"Machacado Natural"}) ||
		!slices.Equal(tasks[fixture.barra.ID], []string{"Coca Cola 600ml", "Papas a la francesa"}) || len(tasks) != 2 {
		t.Fatalf("platillos por estación: %v", tasks)
	}
	found := card(t, fixture.orders(t, fixture.display), accountID)
	if found == nil || len(itemsOf(found, fixture.barra.ID)) != 2 || len(itemsOf(found, fixture.cocina.ID)) != 1 {
		t.Fatalf("la pantalla no muestra lo de las dos estaciones: %+v", found)
	}
	releaseID, state, _ := fixture.release(t, accountID)
	var snapshot string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT routing_snapshot->>$2 FROM app.kitchen_releases WHERE id=$1",
		releaseID, fixture.coca.ID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	var frozen kitchen.Route
	if err := json.Unmarshal([]byte(snapshot), &frozen); err != nil {
		t.Fatal(err)
	}
	if state != "released" || !frozen.Unrouted || frozen.Mode != "prepare" || !slices.Equal(frozen.StationIDs, []string{fixture.barra.ID}) {
		t.Fatalf("liberación %s con la ruta congelada %+v", state, frozen)
	}
	if releases, _ := fixture.onBoard(t, accountID); releases != 0 {
		t.Fatal("el pedido quedó en «Por enviar a cocina»")
	}
	fixture.setUnrouted(t, nil)
}

// testStageK11FrozenModification: una modificación de algo ya enviado sigue la decisión congelada aunque después se
// configure la ruta: el flan que fue a Barra por «Productos sin ruta» sigue en Barra.
func testStageK11FrozenModification(t *testing.T, fixture *unroutedFixture) {
	postres, flan := fixture.product(t, "Postres K1.1", "Flan napolitano", "FLAN-K11", "40")
	fixture.setUnrouted(t, &fixture.barra.ID)
	accountID := fixture.counter(t, "Flan K1.1", draftLine(flan, 1))
	fixture.setUnrouted(t, nil)
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &postres.ID, Mode: "prepare",
		StationIDs: []string{fixture.cocina.ID}}, 200)
	bill := fixture.modifyNotes(t, accountID, "Sin caramelo")
	tasks := fixture.stationTasks(t, accountID)
	if !slices.Equal(tasks[fixture.barra.ID], []string{"Flan napolitano"}) || len(tasks) != 1 {
		t.Fatalf("la modificación cambió de estación: %v", tasks)
	}
	if bill.Account.PreparationState == "change_pending" || fixture.count(t,
		"SELECT count(*)::int FROM app.kitchen_pending_changes WHERE account_id=$1", accountID) != 0 {
		t.Fatal("la modificación quedó como aviso pendiente")
	}
	var notes string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT t.instructions->>'notes' FROM app.kitchen_tasks t
        JOIN app.restaurant_item_units u ON u.id=t.unit_id WHERE u.account_id=$1 AND t.state<>'cancelled'`, accountID).Scan(&notes); err != nil ||
		notes != "Sin caramelo" {
		t.Fatalf("la tarea de Barra no recibió la modificación: %q %v", notes, err)
	}
}

// testStageK11InactiveStation: una ruta a una estación desactivada no detiene nada; ese producto se trata como sin ruta
// y lo demás llega.
func testStageK11InactiveStation(t *testing.T, fixture *unroutedFixture) {
	fixture.papel = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Freidora K1.1", Active: true, Reception: "kds", Tracking: "inherit", Grouping: "inherit",
	}, 200))
	frituras, aros := fixture.product(t, "Frituras K1.1", "Aros de cebolla", "AROS-K11", "50")
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &frituras.ID, Mode: "prepare",
		StationIDs: []string{fixture.papel.ID}}, 200)
	fixture.setStationActive(t, &fixture.papel, false)
	accountID := fixture.counter(t, "Freidora K1.1", draftLine(fixture.machacado, 1), draftLine(aros, 1))
	fixture.expectArrived(t, accountID, 2, "Machacado Natural")
	var snapshot string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT routing_snapshot->>$2 FROM app.kitchen_releases WHERE account_id=$1",
		accountID, aros.ID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	var frozen kitchen.Route
	if err := json.Unmarshal([]byte(snapshot), &frozen); err != nil {
		t.Fatal(err)
	}
	if !frozen.Unrouted || frozen.Mode != "none" || !slices.Equal(frozen.OmittedStationIDs, []string{fixture.papel.ID}) {
		t.Fatalf("ruta congelada de los aros: %+v", frozen)
	}
}

// testStageK11InactivePrinter: una impresora desactivada no detiene nada: la comanda queda en «Requiere intervención» y,
// con «Impresora y pantalla», la pantalla la recibe igual.
func testStageK11InactivePrinter(t *testing.T, fixture *unroutedFixture) {
	if fixture.cocina.Reception != "both" {
		t.Fatalf("Cocina debía estar en «Impresora y pantalla»: %s", fixture.cocina.Reception)
	}
	fixture.setPrinterActive(t, "Comandas K1.1", false)
	accountID := fixture.counter(t, "Impresora K1.1", draftLine(fixture.machacado, 1), draftLine(fixture.coca, 1))
	fixture.expectArrived(t, accountID, 2, "Machacado Natural")
	var state, code string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT j.state,coalesce(j.result->>'code','') FROM app.kitchen_print_jobs j
        JOIN app.kitchen_tickets t ON t.id=j.ticket_id JOIN app.kitchen_releases r ON r.id=t.release_id WHERE r.account_id=$1`,
		accountID).Scan(&state, &code); err != nil {
		t.Fatal(err)
	}
	if state != "intervention" || code != kitchen.ProblemInactive {
		t.Fatalf("trabajo de la impresora desactivada: %s %s", state, code)
	}
	fixture.setPrinterActive(t, "Comandas K1.1", true)
}

// testStageK11Combos: un combo con componentes y sin ruta propia no tiene parte propia (cada componente se resuelve
// solo: el machacado a Cocina, la Coca Cola sin preparación) y un combo vacío sin ruta sigue «Productos sin ruta».
func testStageK11Combos(t *testing.T, fixture *unroutedFixture) {
	combos := decodeResponse[catalog.Category](t, fixture.adminCall(t, "POST", "catalog/categories", catalog.Category{
		Metadata: catalog.Metadata{Name: "Combos K1.1", Active: true},
	}, 200))
	slot := func(name string, product catalog.Product, quantity int) catalog.ComboSlot {
		return catalog.ComboSlot{Name: name, MinQuantity: quantity, MaxQuantity: 1, IncludedQuantity: quantity,
			DefaultQuantity: quantity, DefaultProductID: product.ID,
			Options: []catalog.ComboOption{{ProductID: product.ID, SubstitutionPrice: "0",
				Tiers: []catalog.QuantityTier{{From: 1, UnitPrice: "10"}}}}}
	}
	full := decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Desayuno completo K1.1", Active: true}, CategoryID: combos.ID, Code: "COMBO-K11",
		Kind: "combo", BasePrice: "120",
		Slots: []catalog.ComboSlot{slot("Plato", fixture.machacado, 1), slot("Bebida", fixture.coca, 1)},
	}, 200))
	empty := decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Combo para compartir K1.1", Active: true}, CategoryID: combos.ID, Code: "VACIO-K11",
		Kind: "combo", BasePrice: "80", Slots: []catalog.ComboSlot{slot("Papas opcionales", fixture.papas, 0)},
	}, 200))
	// Cada componente en su lugar, por nombre (el catálogo no garantiza el orden de los lugares).
	selections := []catalog.ComponentSelection{}
	for _, entry := range full.Slots {
		product := fixture.machacado
		if entry.Name == "Bebida" {
			product = fixture.coca
		}
		selections = append(selections, catalog.ComponentSelection{SlotID: entry.ID, ProductID: product.ID, Quantity: 1})
	}
	withComponents := operations.DraftItem{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: full.ID, Quantity: 1,
		Customization: catalog.Customization{Components: selections}}}
	accountID := fixture.counter(t, "Combos K1.1", withComponents, draftLine(empty, 1))
	fixture.expectArrived(t, accountID, 2, "Desayuno completo K1.1")
	var parts []string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT coalesce(array_agg(part->>'name' ORDER BY part->>'path'),'{}')
        FROM app.kitchen_tasks t JOIN app.restaurant_item_units u ON u.id=t.unit_id
        CROSS JOIN LATERAL jsonb_array_elements(t.instructions->'parts') part WHERE u.account_id=$1`, accountID).Scan(&parts); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(parts, []string{"Machacado Natural"}) {
		t.Fatalf("partes del combo en Cocina: %v", parts)
	}
	var snapshot map[string]kitchen.Route
	var raw []byte
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT routing_snapshot FROM app.kitchen_releases WHERE account_id=$1",
		accountID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if _, frozen := snapshot[full.ID]; frozen || !snapshot[fixture.coca.ID].Unrouted || snapshot[fixture.coca.ID].Mode != "none" ||
		!snapshot[empty.ID].Unrouted || snapshot[empty.ID].Mode != "none" {
		t.Fatalf("ruta congelada de los combos: %+v", snapshot)
	}
	coverage := decodeResponse[kitchen.Coverage](t, fixture.adminCall(t, "GET", "kitchen/coverage", nil, 200))
	for _, group := range coverage.UnroutedItems {
		if group.CategoryID == combos.ID {
			if !group.WholeCategory || len(group.Products) != 1 || group.Products[0].ID != empty.ID || group.Products[0].Reason != "no_route" {
				t.Fatalf("lo sin ruta de los combos: %+v", group)
			}
			return
		}
	}
	t.Fatalf("el combo vacío no está en «Sin ruta»: %+v", coverage.UnroutedItems)
}

// testStageK11PendingOnPayment: al cobrar una cuenta de salón, su aviso pendiente (una modificación para una estación
// desactivada que ya tenía la comanda) se da por terminado con su historial; «Volver a avisar» responde 409 y reabrir la
// cuenta con una devolución no lo revive.
func testStageK11PendingOnPayment(t *testing.T, fixture *unroutedFixture) {
	accountID := fixture.counter(t, "Cobrar K1.1", draftLine(fixture.machacado, 1))
	fixture.setStationActive(t, &fixture.cocina, false)
	if bill := fixture.modifyNotes(t, accountID, "Bien dorado"); bill.Account.PreparationState != "change_pending" {
		t.Fatalf("la modificación para la estación desactivada: %s", bill.Account.PreparationState)
	}
	if _, changes := fixture.onBoard(t, accountID); changes != 1 {
		t.Fatal("el aviso para la estación desactivada no está en el panel")
	}
	eventID, revision, _ := fixture.pendingChange(t, accountID)
	// El panel nombra la estación desactivada a la que no se pudo avisar (inactive_stations).
	board := decodeResponse[struct {
		PendingChanges []struct {
			EventID          string `json:"event_id"`
			InactiveStations []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"inactive_stations"`
		} `json:"pending_changes"`
	}](t, fixture.adminCall(t, "GET", "kitchen/board", nil, 200))
	named := ""
	for _, change := range board.PendingChanges {
		if change.EventID == eventID && len(change.InactiveStations) == 1 && change.InactiveStations[0].ID == fixture.cocina.ID {
			named = change.InactiveStations[0].Name
		}
	}
	if named != "Cocina" {
		t.Fatalf("estaciones desactivadas del aviso: %+v", board.PendingChanges)
	}
	fixture.pay(t, accountID)
	if state := fixture.state(t, accountID); state != "closed" {
		t.Fatalf("la cuenta cobrada quedó %s", state)
	}
	_, _, reason := fixture.pendingChange(t, accountID)
	if reason == nil || *reason != "account_closed" {
		t.Fatalf("el aviso de la cuenta cobrada: %v", reason)
	}
	fixture.dismissedHistory(t, "change.dismissed", eventID, "account_closed")
	if _, changes := fixture.onBoard(t, accountID); changes != 0 {
		t.Fatal("la cuenta cobrada dejó su aviso en el panel")
	}
	if state := fixture.refresh(t, accountID).Account.PreparationState; state == "change_pending" {
		t.Fatal("la cuenta cobrada sigue con «Cambio pendiente en cocina»")
	}
	fixture.setStationActive(t, &fixture.cocina, true)
	retry := map[string]any{"operation_id": randomID(), "revision": revision}
	if code := errorCode(t, fixture.adminCall(t, "POST", "kitchen/changes/"+eventID+"/retry", retry, 409)); code != "KITCHEN_CHANGE_NOT_PENDING" {
		t.Fatalf("«Volver a avisar» un aviso terminado: %s", code)
	}
	// Una devolución reabre la cuenta: lo terminado no revive.
	fixture.mutate(t, "POST", "payments/accounts/"+accountID+"/refunds", map[string]any{"mode": "total",
		"method_id": fixture.cashMethod, "reason": "Devolución de prueba K1.1"}, 200)
	if state := fixture.state(t, accountID); state != "open" {
		t.Fatalf("la devolución no reabrió la cuenta: %s", state)
	}
	if _, changes := fixture.onBoard(t, accountID); changes != 0 {
		t.Fatal("reabrir la cuenta revivió su aviso")
	}
	fixture.adminCall(t, "POST", "kitchen/changes/"+eventID+"/retry", map[string]any{"operation_id": randomID(), "revision": revision}, 409)
	if pending := fixture.count(t, "SELECT count(*)::int FROM app.kitchen_pending_changes WHERE account_id=$1 AND dismissed_at IS NULL",
		accountID); pending != 0 {
		t.Fatal("reabrir la cuenta revivió su aviso")
	}
}

// testStageK11PendingTakeaway: un pedido para llevar cobrado y sin entregar sigue abierto y su pedido programado llega a
// la cocina a su hora; al entregarse se cierra. Un programado que no se envió se da por terminado al cerrar con la
// entrega, y «Enviar a cocina ahora» responde 409.
func testStageK11PendingTakeaway(t *testing.T, fixture *unroutedFixture) {
	timing := func(automatic bool) {
		t.Helper()
		current := decodeResponse[kitchen.SettingsView](t, fixture.adminCall(t, "GET", "kitchen/settings", nil, 200))
		fixture.mutate(t, "PUT", "kitchen/timing", map[string]any{"revision": current.Revision, "automatic_release": automatic,
			"lead_minutes": current.LeadMinutes, "tracking": current.Tracking}, 200)
	}
	timing(true)
	automatic := fixture.takeaway(t, "Automático K1.1", draftLine(fixture.machacado, 1), draftLine(fixture.coca, 1))
	releaseID, state, _ := fixture.release(t, automatic)
	if state != "scheduled" {
		t.Fatalf("pedido programado: %s", state)
	}
	fixture.pay(t, automatic)
	if account := fixture.state(t, automatic); account != "open" {
		t.Fatalf("el pedido para llevar cobrado y sin entregar quedó %s", account)
	}
	if releases, _ := fixture.onBoard(t, automatic); releases != 1 {
		t.Fatal("el programado de un pedido cobrado y sin entregar salió del panel")
	}
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_releases SET due_at=now()-interval '1 second' WHERE id=$1",
		releaseID); err != nil {
		t.Fatal(err)
	}
	if err := (&kitchen.Worker{Pool: fixture.pool}).ReleaseDue(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	fixture.expectArrived(t, automatic, 2, "Machacado Natural")
	fixture.mutate(t, "POST", "payments/accounts/"+automatic+"/deliver", map[string]any{}, 200)
	if account := fixture.state(t, automatic); account != "closed" {
		t.Fatalf("entregado y pagado: %s", account)
	}
	timing(false)

	manual := fixture.takeaway(t, "Manual K1.1", draftLine(fixture.machacado, 1))
	releaseID, _, revision := fixture.release(t, manual)
	fixture.pay(t, manual)
	if releases, _ := fixture.onBoard(t, manual); releases != 1 {
		t.Fatal("el programado de un pedido cobrado y sin entregar salió del panel")
	}
	fixture.mutate(t, "POST", "payments/accounts/"+manual+"/deliver", map[string]any{}, 200)
	if _, state, _ := fixture.release(t, manual); state != "cancelled" {
		t.Fatalf("el programado del pedido entregado quedó %s", state)
	}
	fixture.dismissedHistory(t, "release.dismissed", releaseID, "account_closed")
	if releases, _ := fixture.onBoard(t, manual); releases != 0 {
		t.Fatal("el pedido entregado dejó su programado en el panel")
	}
	if code := errorCode(t, fixture.releaseNow(t, releaseID, randomID(), revision+1, 409)); code != "KITCHEN_RELEASE_NOT_PENDING" {
		t.Fatalf("«Enviar a cocina ahora» del pedido cerrado: %s", code)
	}
	if released := fixture.count(t, `SELECT count(*)::int FROM app.kitchen_unit_releases k JOIN app.restaurant_item_units u
        ON u.id=k.unit_id WHERE u.account_id=$1`, manual); released != 0 {
		t.Fatal("darlo por terminado mandó el pedido a la cocina")
	}
}

// testStageK11PendingCancel: «Cancelar cuenta» da por terminado lo que la cocina no recibió (con su historial y sin
// papel) y también el aviso para una estación desactivada que ya tenía la comanda.
func testStageK11PendingCancel(t *testing.T, fixture *unroutedFixture) {
	scheduled := fixture.takeaway(t, "Cancelado K1.1", draftLine(fixture.machacado, 1))
	releaseID, _, _ := fixture.release(t, scheduled)
	fixture.cancelAccount(t, scheduled)
	if _, state, _ := fixture.release(t, scheduled); state != "cancelled" {
		t.Fatalf("el programado de la cuenta cancelada quedó %s", state)
	}
	fixture.dismissedHistory(t, "release.dismissed", releaseID, "account_cancelled")
	if tickets := fixture.count(t, "SELECT count(*)::int FROM app.kitchen_tickets WHERE release_id=$1", releaseID); tickets != 0 {
		t.Fatal("cancelar lo que la cocina no recibió imprimió algo")
	}

	sent := fixture.counter(t, "Cancelado en cocina K1.1", draftLine(fixture.machacado, 1))
	fixture.setStationActive(t, &fixture.cocina, false)
	fixture.cancelAccount(t, sent)
	eventID, revision, reason := fixture.pendingChange(t, sent)
	if reason == nil || *reason != "account_cancelled" {
		t.Fatalf("el aviso de la cuenta cancelada: %v", reason)
	}
	fixture.dismissedHistory(t, "change.dismissed", eventID, "account_cancelled")
	if releases, changes := fixture.onBoard(t, sent); releases+changes != 0 {
		t.Fatal("la cuenta cancelada dejó algo en el panel")
	}
	if state := fixture.refresh(t, sent).Account.PreparationState; state != "cancelled" {
		t.Fatalf("preparación de la cuenta cancelada: %s", state)
	}
	fixture.setStationActive(t, &fixture.cocina, true)
	if code := errorCode(t, fixture.adminCall(t, "POST", "kitchen/changes/"+eventID+"/retry",
		map[string]any{"operation_id": randomID(), "revision": revision}, 409)); code != "KITCHEN_CHANGE_NOT_PENDING" {
		t.Fatalf("«Volver a avisar» de la cuenta cancelada: %s", code)
	}
}

// testStageK11PendingMerge: combinar no da nada por terminado (las unidades siguen en una cuenta abierta); al cerrarse la
// cuenta de destino, el programado de la combinada se da por terminado.
func testStageK11PendingMerge(t *testing.T, fixture *unroutedFixture) {
	source := fixture.takeaway(t, "Combinado K1.1", draftLine(fixture.machacado, 1))
	target := fixture.takeaway(t, "Combinado K1.1", draftLine(fixture.hamburguesa, 1))
	releaseID, _, _ := fixture.release(t, source)
	sourceBill, targetBill := fixture.refresh(t, source), fixture.refresh(t, target)
	fixture.mutate(t, "POST", "restaurant/accounts/"+source+"/merge", map[string]any{"revision": sourceBill.Account.Revision,
		"target_account_id": target, "target_revision": targetBill.Account.Revision}, 200)
	if releases, _ := fixture.onBoard(t, source); releases != 1 {
		t.Fatal("combinar sacó del panel el programado de la cuenta combinada")
	}
	if _, state, _ := fixture.release(t, source); state != "scheduled" {
		t.Fatalf("el programado de la cuenta combinada quedó %s", state)
	}
	fixture.pay(t, target)
	fixture.mutate(t, "POST", "payments/accounts/"+target+"/deliver", map[string]any{}, 200)
	if _, state, _ := fixture.release(t, source); state != "cancelled" {
		t.Fatalf("al cerrar la cuenta de destino, el programado combinado quedó %s", state)
	}
	fixture.dismissedHistory(t, "release.dismissed", releaseID, "account_closed")
	if releases, _ := fixture.onBoard(t, source); releases != 0 {
		t.Fatal("el programado combinado sigue en el panel")
	}
}

// testStageK11OldBlocked: un envío bloqueado de antes de la 0.17.0 de una cuenta abierta sigue en el panel y «Enviar a
// cocina ahora» lo manda con la regla nueva (lo que tiene ruta llega; lo demás no necesita preparación). Repetir con el
// mismo operation_id devuelve la respuesta guardada; otra vez, 409.
func testStageK11OldBlocked(t *testing.T, fixture *unroutedFixture) {
	accountID := fixture.takeaway(t, "Bloqueado viejo K1.1", draftLine(fixture.machacado, 1), draftLine(fixture.coca, 1))
	releaseID, _, _ := fixture.release(t, accountID)
	// Así lo dejó la 0.16.0: bloqueado por la Coca Cola sin ruta, sin nada liberado.
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.kitchen_releases SET state='blocked',
        warnings='["Sin ruta de preparación: Coca Cola 600ml"]',revision=revision+1 WHERE id=$1`, releaseID); err != nil {
		t.Fatal(err)
	}
	if releases, _ := fixture.onBoard(t, accountID); releases != 1 {
		t.Fatal("el bloqueado viejo de una cuenta abierta no está en el panel")
	}
	_, _, revision := fixture.release(t, accountID)
	operationID := randomID()
	first := fixture.releaseNow(t, releaseID, operationID, revision, 200)
	released := decodeResponse[kitchen.Release](t, first)
	if released.State != "released" || len(released.Warnings) != 0 {
		t.Fatalf("«Enviar a cocina ahora» del bloqueado viejo: %+v", released)
	}
	fixture.expectArrived(t, accountID, 2, "Machacado Natural")
	// La respuesta guardada vuelve de la base (jsonb): se compara por contenido.
	var left, right any
	_ = json.Unmarshal(first, &left)
	_ = json.Unmarshal(fixture.releaseNow(t, releaseID, operationID, revision, 200), &right)
	if string(mustJSON(left)) != string(mustJSON(right)) {
		t.Fatalf("la repetición no devolvió la respuesta guardada:\n%s\n%s", mustJSON(left), mustJSON(right))
	}
	if code := errorCode(t, fixture.releaseNow(t, releaseID, randomID(), released.Revision, 409)); code != "KITCHEN_RELEASE_NOT_PENDING" {
		t.Fatalf("«Enviar a cocina ahora» de algo ya enviado: %s", code)
	}
}

// testStageK11Coverage: la cobertura (GET kitchen/coverage) con lo que llega a cada estación y pantalla, lo sin ruta y
// cada aviso con su texto; «Productos sin ruta» con su valor esperado y su validación; sin kitchen.stations.read, 403.
func testStageK11Coverage(t *testing.T, fixture *unroutedFixture) {
	read := func() kitchen.Coverage {
		t.Helper()
		return decodeResponse[kitchen.Coverage](t, fixture.adminCall(t, "GET", "kitchen/coverage", nil, 200))
	}
	coverage := read()
	if coverage.Unrouted.Effective != "none" || coverage.Unrouted.StationID != nil {
		t.Fatalf("«Productos sin ruta»: %+v", coverage.Unrouted)
	}
	var cocina *kitchen.CoverageStation
	for index := range coverage.Stations {
		if coverage.Stations[index].ID == fixture.cocina.ID {
			cocina = &coverage.Stations[index]
		}
	}
	if cocina == nil || !slices.Contains(cocina.DisplayIDs, fixture.display.deviceID) || cocina.Printer == nil ||
		cocina.Printer.Name != "Comandas K1.1" {
		t.Fatalf("Cocina en la cobertura: %+v", cocina)
	}
	categories := []string{}
	for _, category := range cocina.Categories {
		categories = append(categories, fmt.Sprintf("%s:%d", category.Name, category.Products))
	}
	if !slices.Equal(categories, []string{"Desayunos:1", "Hamburguesas:1", "Postres K1.1:1"}) {
		t.Fatalf("categorías de Cocina: %v", categories)
	}
	unrouted := map[string]string{}
	for _, group := range coverage.UnroutedItems {
		for _, product := range group.Products {
			unrouted[product.Name] = fmt.Sprintf("%s:%v:%s", group.CategoryName, group.WholeCategory, product.Reason)
		}
	}
	if unrouted["Coca Cola 600ml"] != "Refrescos:true:no_route" || unrouted["Papas a la francesa"] != "Extras:true:no_route" ||
		unrouted["Aros de cebolla"] != "Frituras K1.1:true:inactive_station" {
		t.Fatalf("lo sin ruta: %v", unrouted)
	}
	for _, category := range coverage.Categories {
		if category.Name == "Refrescos" && (category.RouteID != nil || len(category.StationIDs) != 0) ||
			category.Name == "Desayunos" && (category.RouteID == nil || *category.RouteMode != "prepare" ||
				!slices.Equal(category.StationIDs, []string{fixture.cocina.ID})) {
			t.Fatalf("categoría con su ruta: %+v", category)
		}
	}

	// Los avisos que faltan: una pantalla sin estaciones, otra con solo una estación que imprime, una estación de
	// pantalla sin nada asignado, «Productos sin ruta» a una estación desactivada y una impresora desactivada.
	empty := fixture.kitchenDevice(t, "Tableta sin estaciones K1.1")
	onlyPrinter := fixture.kitchenDevice(t, "Tableta de la caja K1.1")
	var printerID string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.kitchen_printers WHERE name='Comandas K1.1'").Scan(&printerID); err != nil {
		t.Fatal(err)
	}
	caja := decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Caja K1.1", Active: true, Reception: "printer", PrinterID: &printerID, Tracking: "inherit", Grouping: "inherit",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Pasteles K1.1", Active: true, Reception: "kds", Tracking: "inherit", Grouping: "inherit",
	}, 200)
	// Lo que elige Terminales para cada pantalla: la primera sin ninguna estación, la segunda solo la que imprime.
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.devices SET kitchen_all_stations=false WHERE id=ANY($1::uuid[])",
		[]string{empty.deviceID, onlyPrinter.deviceID}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, "INSERT INTO app.kitchen_device_stations(device_id,station_id) VALUES ($1,$2)",
		onlyPrinter.deviceID, caja.ID); err != nil {
		t.Fatal(err)
	}
	fixture.setUnrouted(t, &fixture.barra.ID)
	fixture.setStationActive(t, &fixture.barra, false)
	fixture.setPrinterActive(t, "Comandas K1.1", false)
	coverage = read()
	messages := map[string][]string{}
	for _, warning := range coverage.Warnings {
		messages[warning.Code] = append(messages[warning.Code], warning.Message)
		switch warning.Code {
		case "display_without_stations", "display_no_screen_stations":
			if warning.DeviceID == nil || warning.StationID != nil {
				t.Fatalf("aviso de pantalla sin su pantalla: %+v", warning)
			}
		case "route_station_inactive":
			if warning.StationID == nil || (warning.CategoryID == nil) == (warning.ProductID == nil) {
				t.Fatalf("aviso de ruta sin lo que hay que corregir: %+v", warning)
			}
		default:
			if warning.StationID == nil || warning.DeviceID != nil {
				t.Fatalf("aviso de estación sin su estación: %+v", warning)
			}
		}
	}
	expected := map[string][]string{
		"display_without_stations":   {"Esta pantalla no tiene estaciones."},
		"display_no_screen_stations": {"Ninguna de sus estaciones recibe comandas en pantalla."},
		"station_without_routes":     {"La estación Pasteles K1.1 no tiene categorías ni productos asignados."},
		"route_station_inactive":     {"Frituras K1.1 va a Freidora K1.1, que está desactivada: sus productos se tratan como productos sin ruta."},
		"unrouted_station_inactive":  {"Los productos sin ruta iban a Barra K1.1, que está desactivada. Mientras siga así, no necesitan preparación."},
		"station_printer_inactive": {
			"La impresora Comandas K1.1 de Caja K1.1 está desactivada: sus comandas esperan en la cola.",
			"La impresora Comandas K1.1 de Cocina está desactivada: sus comandas esperan en la cola.",
		},
	}
	for code, texts := range expected {
		if !slices.Equal(messages[code], texts) {
			t.Fatalf("avisos %s: %v (todos: %v)", code, messages[code], messages)
		}
	}
	if len(messages) != len(expected) {
		t.Fatalf("avisos de más: %v", messages)
	}
	if coverage.Unrouted.Effective != "none" || coverage.Unrouted.StationActive == nil || *coverage.Unrouted.StationActive {
		t.Fatalf("«Productos sin ruta» a una estación desactivada: %+v", coverage.Unrouted)
	}
	for _, display := range coverage.Displays {
		if (display.DeviceID == empty.deviceID || display.DeviceID == onlyPrinter.deviceID) && len(display.StationIDs) != 0 {
			t.Fatalf("estaciones de una pantalla sin estaciones de pantalla: %+v", display)
		}
	}
	// Con la estación del ajuste desactivada, lo sin ruta no necesita preparación mientras tanto.
	accountID := fixture.counter(t, "Barra desactivada K1.1", draftLine(fixture.machacado, 1), draftLine(fixture.coca, 1))
	fixture.expectArrived(t, accountID, 2, "Machacado Natural")

	// «Productos sin ruta»: una estación desactivada o que no existe, 400; un valor esperado viejo, 409.
	body := func(station, expected any) map[string]any {
		return map[string]any{"station_id": station, "expected_station_id": expected}
	}
	invalid := fixture.mutate(t, "PUT", "kitchen/unrouted", body(fixture.barra.ID, fixture.barra.ID), 400)
	if !strings.Contains(string(invalid), "Elige una estación activa.") {
		t.Fatalf("estación desactivada: %s", invalid)
	}
	fixture.mutate(t, "PUT", "kitchen/unrouted", body(randomID(), fixture.barra.ID), 400)
	if code := errorCode(t, fixture.mutate(t, "PUT", "kitchen/unrouted", body(nil, nil), 409)); code != "KITCHEN_UNROUTED_CHANGED" {
		t.Fatalf("valor esperado viejo: %s", code)
	}
	if saved := fixture.setUnrouted(t, nil); saved.Effective != "none" || saved.StationID != nil {
		t.Fatalf("«No necesitan preparación»: %+v", saved)
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.kitchen_history WHERE kind='unrouted.saved'") == 0 {
		t.Fatal("«Productos sin ruta» no dejó historial")
	}
	fixture.setStationActive(t, &fixture.barra, true)
	fixture.setPrinterActive(t, "Comandas K1.1", true)

	// La cobertura es para quien ve la configuración de cocina; guardar el ajuste, para quien administra las rutas.
	reader := inventoryUser(t, fixture.restaurantFixture, "lector_k11", []string{"kitchen.stations.read"})
	fixture.call(t, reader, "GET", "kitchen/coverage", nil, 200)
	fixture.call(t, reader, "PUT", "kitchen/unrouted", map[string]any{"operation_id": randomID(), "station_id": nil,
		"expected_station_id": nil}, 403)
	other := inventoryUser(t, fixture.restaurantFixture, "sin_cocina_k11", []string{"accounts.read"})
	if code := errorCode(t, fixture.call(t, other, "GET", "kitchen/coverage", nil, 403)); code != "PERMISSION_DENIED" {
		t.Fatalf("cobertura sin permiso: %s", code)
	}
}
