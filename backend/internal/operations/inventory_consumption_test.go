//go:build integration

package operations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/features"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Consumo por liberación a cocina (§13, §14 y §0-B): el trabajador se llama directamente con
// inventory.ProcessPending y la reconciliación con ReconcileCancellations y una hora simulada.

// setFeature activa o desactiva «Control de inventario».
func (fixture *recipesFixture) setFeature(t *testing.T, enabled bool) {
	t.Helper()
	var current features.Feature
	for _, feature := range decodeResponse[[]features.Feature](t, fixture.adminCall(t, "GET", "settings/features", nil, 200)) {
		if feature.Code == features.InventoryControl {
			current = feature
		}
	}
	if current.Enabled == enabled {
		return
	}
	fixture.adminCall(t, "PUT", "settings/features", map[string]any{"code": features.InventoryControl, "enabled": enabled,
		"revision": current.Revision}, 200)
}

// process ejecuta el trabajador y devuelve cuántas solicitudes tomó.
func (fixture *recipesFixture) process(t *testing.T) int {
	t.Helper()
	processed, err := inventory.ProcessPending(fixture.ctx, fixture.pool, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return processed
}

// order abre una cuenta, captura y confirma las partidas (la confirmación libera a cocina).
func (fixture *recipesFixture) order(t *testing.T, name, mode, origin string, lines ...catalog.LineRequest) operations.Bill {
	t.Helper()
	body := map[string]any{"name": name, "mode": mode}
	if origin != "" {
		body["origin"] = origin
	}
	bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", body, 200))
	return fixture.capture(t, bill, lines)
}

// consumption suma el consumo neto (consumos menos reversiones) de las unidades de una cuenta por
// insumo y almacén: «código@almacén» → cantidad.
func (fixture *recipesFixture) consumption(t *testing.T, accountID string) map[string]string {
	t.Helper()
	return fixture.consumptionWhere(t, "u.account_id=$1", accountID)
}

func (fixture *recipesFixture) unitConsumption(t *testing.T, unitID string) map[string]string {
	t.Helper()
	return fixture.consumptionWhere(t, "u.id=$1", unitID)
}

func (fixture *recipesFixture) consumptionWhere(t *testing.T, filter string, argument string) map[string]string {
	t.Helper()
	rows, err := fixture.pool.Query(fixture.ctx, `SELECT i.code||'@'||w.name,(-sum(m.quantity))::text FROM app.inventory_movements m
        JOIN app.restaurant_item_units u ON u.id=m.restaurant_unit_id JOIN app.inventory_items i ON i.id=m.item_id
        JOIN app.inventory_warehouses w ON w.id=m.warehouse_id
        WHERE `+filter+` AND m.kind IN ('consumption','consumption_reversal') GROUP BY 1 HAVING sum(m.quantity)<>0`, argument)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var key, quantity string
		if err := rows.Scan(&key, &quantity); err != nil {
			t.Fatal(err)
		}
		result[key] = decimal.RequireFromString(quantity).String()
	}
	return result
}

func expectNet(t *testing.T, label string, got map[string]string, expected map[string]string) {
	t.Helper()
	if len(got) != len(expected) {
		t.Fatalf("%s: consumo %v, esperado %v", label, got, expected)
	}
	for key, quantity := range expected {
		if got[key] != quantity {
			t.Fatalf("%s: %s = %s, esperado %s (todo: %v)", label, key, got[key], quantity, got)
		}
	}
}

// accountUnits devuelve las unidades de una cuenta (también canceladas) en orden.
func (fixture *recipesFixture) accountUnits(t *testing.T, accountID string) []string {
	t.Helper()
	rows, err := fixture.pool.Query(fixture.ctx, "SELECT id FROM app.restaurant_item_units WHERE account_id=$1 ORDER BY item_id,ordinal", accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	units := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		units = append(units, id)
	}
	return units
}

func (fixture *recipesFixture) requests(t *testing.T) int {
	t.Helper()
	return rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_consumption_requests")
}

func (fixture *recipesFixture) unitMovements(t *testing.T) int {
	t.Helper()
	return rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements WHERE restaurant_unit_id IS NOT NULL")
}

func (fixture *recipesFixture) burgerLine(quantity int, onion, lettuce, jalapeno, notes string) catalog.LineRequest {
	line := catalog.LineRequest{ProductID: fixture.products["hamburguesa"].ID, Quantity: quantity}
	if onion != "" {
		line.Customization.Ingredients = append(line.Customization.Ingredients,
			catalog.IngredientSelection{IngredientID: fixture.ingredients["Cebolla"].ID, Quantity: onion})
	}
	if lettuce != "" {
		line.Customization.Ingredients = append(line.Customization.Ingredients,
			catalog.IngredientSelection{IngredientID: fixture.ingredients["Lechuga"].ID, Quantity: lettuce})
	}
	if jalapeno != "" {
		line.Customization.Ingredients = append(line.Customization.Ingredients,
			catalog.IngredientSelection{IngredientID: fixture.ingredients["Jalapeño"].ID, Quantity: jalapeno})
	}
	line.Customization.Notes = notes
	return line
}

// editLine modifica una partida confirmada con previsualización y huella.
func (fixture *recipesFixture) editLine(t *testing.T, bill operations.Bill, line catalog.LineRequest) operations.Bill {
	t.Helper()
	current := fixture.refresh(t, bill.Account.ID)
	confirmed := current.Lines[0]
	edit := operations.EditInput{Mutation: operations.Mutation{Revision: current.Account.Revision}, VersionID: confirmed.VersionID, Line: line,
		Reason: "Cambio solicitado por el cliente"}
	route := "restaurant/accounts/" + bill.Account.ID
	preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", route+"/edit-preview", edit, 200))
	edit.ExpectedFingerprint = preview.Fingerprint
	return decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", route+"/edit", edit, 200))
}

// cancelOne cancela una unidad de la primera partida de la cuenta.
func (fixture *recipesFixture) cancelOne(t *testing.T, bill operations.Bill) operations.Bill {
	t.Helper()
	current := fixture.refresh(t, bill.Account.ID)
	return decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/cancel", map[string]any{
		"revision": current.Account.Revision, "version_id": current.Lines[0].VersionID, "quantity": 1, "reason": "El cliente ya no lo quiso",
	}, 200))
}

// classification devuelve la cancelación de una unidad.
func (fixture *recipesFixture) classification(t *testing.T, unitID string) *inventory.Cancellation {
	t.Helper()
	trace := decodeResponse[inventory.UnitConsumptionTrace](t, fixture.adminCall(t, "GET", "inventory/consumption/"+unitID, nil, 200))
	return trace.Cancellation
}

func testConsumption(t *testing.T, fixture *recipesFixture) {
	cursor := eventCursor(t, fixture.restaurantFixture)
	var before operations.Bill
	var scheduled operations.Bill
	t.Run("inventory_b_disabled_and_activation", func(t *testing.T) {
		// Con la función desactivada, liberar a cocina no genera solicitudes ni movimientos.
		fixture.setFeature(t, false)
		before = fixture.order(t, "Antes de activar", "dine_in", "", fixture.burgerLine(1, "", "", "", ""))
		if fixture.refresh(t, before.Account.ID).Account.PreparationState != "released" {
			t.Fatal("La cuenta de prueba no se liberó a cocina")
		}
		// Pedido confirmado antes de activar, programado para después: aún no se libera.
		location, _ := time.LoadLocation("America/Monterrey")
		later := time.Now().In(location).Add(12 * time.Hour).Truncate(time.Minute)
		scheduled = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "Programado antes de activar", "mode": "takeaway", "origin": "Teléfono",
			"scheduled_local": later.Format("2006-01-02T15:04"), "schedule_kind": "preparation",
		}, 200))
		scheduled = fixture.capture(t, scheduled, []catalog.LineRequest{fixture.burgerLine(1, "", "", "", "")})
		if fixture.requests(t) != 0 || fixture.unitMovements(t) != 0 || fixture.process(t) != 0 {
			t.Fatal("Con la función desactivada se registró consumo")
		}
		fixture.setFeature(t, true)
		// La unidad liberada antes de activar nunca consume: ni al modificarla ni al cancelarla.
		fixture.editLine(t, before, fixture.burgerLine(1, "2", "", "", ""))
		fixture.cancelOne(t, before)
		if fixture.requests(t) != 0 || fixture.process(t) != 0 || fixture.unitMovements(t) != 0 {
			t.Fatal("Una unidad liberada antes de activar generó un ajuste")
		}
		// El pedido confirmado antes de activar y liberado después sí consume (la regla es la hora
		// de liberación). Es para llevar: incluye la bolsa.
		release := kitchenReleases(t, fixture.restaurantFixture, scheduled)[0]
		if release.State != "scheduled" {
			t.Fatalf("La liberación programada ya se había ejecutado: %s", release.State)
		}
		fixture.mutate(t, "POST", "kitchen/releases/"+release.ID+"/release", map[string]any{"revision": release.Revision}, 200)
		if fixture.requests(t) != 1 || fixture.process(t) != 1 {
			t.Fatal("La liberación posterior a la activación no registró su solicitud")
		}
		expectNet(t, "programado liberado después de activar", fixture.consumption(t, scheduled.Account.ID), map[string]string{
			"B-PAN@Cocina B": "1", "B-CARNE@Cocina B": "150", "B-QUESO@Cocina B": "1", "B-CEBOLLA@Cocina B": "20",
			"B-LECHUGA@Cocina B": "15", "B-BOLSA@Cocina B": "1",
		})
	})
	var released operations.Bill
	t.Run("inventory_b_release_once_and_idempotent", func(t *testing.T) {
		requests := fixture.requests(t)
		bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": "Liberación", "mode": "dine_in"}, 200))
		draft := []operations.DraftItem{{ID: randomID(), LineRequest: fixture.burgerLine(2, "", "", "", "")}}
		route := "restaurant/accounts/" + bill.Account.ID
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", route+"/draft", map[string]any{"revision": bill.Account.Revision, "items": draft}, 200))
		if fixture.requests(t) != requests {
			t.Fatal("La cuenta creada o el borrador generaron una solicitud de consumo")
		}
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "GET", route+"/preview", nil, 200))
		body := map[string]any{"operation_id": randomID(), "revision": bill.Account.Revision, "expected_fingerprint": preview.Fingerprint}
		released = decodeResponse[operations.Bill](t, fixture.adminCall(t, "POST", route+"/confirm", body, 200))
		// La liberación no espera al cálculo: con el trabajador detenido, la confirmación terminó y
		// dejó una solicitud pendiente por unidad, sin movimientos.
		units := fixture.accountUnits(t, released.Account.ID)
		if fixture.requests(t) != requests+2 || len(fixture.consumption(t, released.Account.ID)) != 0 ||
			rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_consumption_requests WHERE state='pending' AND restaurant_unit_id=ANY($1)",
				units) != 2 {
			t.Fatal("La confirmación no dejó una solicitud pendiente por unidad")
		}
		// Reintento HTTP idempotente: la misma respuesta, sin otra solicitud.
		fixture.adminCall(t, "POST", route+"/confirm", body, 200)
		if fixture.requests(t) != requests+2 {
			t.Fatal("El reintento de la confirmación duplicó solicitudes")
		}
		if processed := fixture.process(t); processed != 2 {
			t.Fatalf("El trabajador procesó %d solicitudes", processed)
		}
		expectNet(t, "dos unidades en salón", fixture.consumption(t, released.Account.ID), map[string]string{
			"B-PAN@Cocina B": "2", "B-CARNE@Cocina B": "300", "B-QUESO@Cocina B": "2", "B-CEBOLLA@Cocina B": "40", "B-LECHUGA@Cocina B": "30",
		})
		var reason string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT reason FROM app.inventory_movements WHERE restaurant_unit_id=$1 LIMIT 1",
			units[0]).Scan(&reason); err != nil || !strings.Contains(reason, "Estación: Inv Cocina") {
			t.Fatalf("El movimiento no registra la estación resuelta: %q %v", reason, err)
		}
		// Reprocesar (otra vuelta del trabajador, o una solicitud devuelta a pendiente como tras un
		// reinicio inesperado) no duplica movimientos.
		movements := fixture.unitMovements(t)
		if fixture.process(t) != 0 {
			t.Fatal("El trabajador volvió a tomar solicitudes procesadas")
		}
		if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.inventory_consumption_requests SET state='pending',processed_at=NULL
            WHERE restaurant_unit_id=$1`, units[0]); err != nil {
			t.Fatal(err)
		}
		if fixture.process(t) != 1 || fixture.unitMovements(t) != movements {
			t.Fatal("Reprocesar una solicitud duplicó movimientos")
		}
		// Queso: 2 piezas por $10. El programado dejó 1; estas dos llegan a 0 y luego a −1 (la
		// existencia negativa no bloquea y se valora al último costo válido de $5).
		rows, err := fixture.pool.Query(fixture.ctx, `SELECT m.stock_after::text,m.value::text,m.cost_basis FROM app.inventory_movements m
            WHERE m.item_id=$1 AND m.kind='consumption' ORDER BY m.seq`, fixture.items["queso"].ID)
		if err != nil {
			t.Fatal(err)
		}
		states := []string{}
		for rows.Next() {
			var stock, value, basis string
			if err := rows.Scan(&stock, &value, &basis); err != nil {
				t.Fatal(err)
			}
			states = append(states, decimal.RequireFromString(stock).String()+"|"+decimal.RequireFromString(value).String()+"|"+basis)
		}
		rows.Close()
		if !slices.Equal(states, []string{"1|-5|average", "0|-5|average", "-1|-5|reference"}) {
			t.Fatalf("Consumo con saldo positivo, cero y negativo: %v", states)
		}
	})
	t.Run("inventory_b_customizations", func(t *testing.T) {
		cases := []struct {
			name    string
			line    catalog.LineRequest
			onion   string
			lettuce string
		}{
			// «Sin cebolla» → 0; «Con cebolla» (ya incluida) → 1 porción; «Cebolla extra» → 2 porciones.
			{"sin cebolla", fixture.burgerLine(1, "0", "", "", ""), "", "15"},
			{"con cebolla", fixture.burgerLine(1, "1", "", "", ""), "20", "15"},
			{"cebolla extra", fixture.burgerLine(1, "2", "", "", ""), "40", "15"},
			// Jalapeño agregado sin porción de referencia: no se inventa consumo.
			{"ingrediente sin porción", fixture.burgerLine(1, "", "", "1", ""), "20", "15"},
			// La observación libre «Sin cebolla» sin selección estructurada: consumo normal.
			{"observación libre", fixture.burgerLine(1, "", "", "", "Sin cebolla"), "20", "15"},
			// Sin lechuga y 3 porciones de cebolla (cantidades del catálogo).
			{"tres porciones", fixture.burgerLine(1, "3", "0", "", ""), "60", ""},
		}
		for _, testCase := range cases {
			bill := fixture.order(t, "Personalización "+testCase.name, "dine_in", "", testCase.line)
			fixture.process(t)
			expected := map[string]string{"B-PAN@Cocina B": "1", "B-CARNE@Cocina B": "150", "B-QUESO@Cocina B": "1"}
			if testCase.onion != "" {
				expected["B-CEBOLLA@Cocina B"] = testCase.onion
			}
			if testCase.lettuce != "" {
				expected["B-LECHUGA@Cocina B"] = testCase.lettuce
			}
			expectNet(t, testCase.name, fixture.consumption(t, bill.Account.ID), expected)
			if testCase.name == "ingrediente sin porción" {
				trace := decodeResponse[inventory.UnitConsumptionTrace](t, fixture.adminCall(t, "GET",
					"inventory/consumption/"+fixture.accountUnits(t, bill.Account.ID)[0], nil, 200))
				if len(trace.Requests) != 1 || len(trace.Requests[0].Diagnostics) != 1 ||
					trace.Requests[0].Diagnostics[0].Code != "ingredient_without_portion" {
					t.Fatalf("Diagnóstico del ingrediente sin porción: %+v", trace.Requests)
				}
			}
		}
	})
	t.Run("inventory_b_complements_and_portions", func(t *testing.T) {
		group := func(options ...catalog.IncludedOption) catalog.LineRequest {
			return catalog.LineRequest{ProductID: fixture.products["filete"].ID, Quantity: 1, Customization: catalog.Customization{
				Groups: []catalog.GroupSelection{{GroupID: fixture.group.ID, Options: options}},
			}}
		}
		option := func(name, quantity string) catalog.IncludedOption {
			return catalog.IncludedOption{OptionID: fixture.options[name].ID, Quantity: quantity}
		}
		// El ejemplo del propietario: quitar arroz y poner pasta sigue en $140 y consume pasta y
		// ensalada, no arroz. El filete va a dos estaciones y consume una sola vez.
		substituted := fixture.order(t, "Filete con pasta", "dine_in", "", group(option("Ensalada B", "1"), option("Pasta B", "1")))
		if substituted.Account.Totals.Total != "140.00" {
			t.Fatalf("El precio del filete cambió: %s", substituted.Account.Totals.Total)
		}
		fixture.process(t)
		unit := fixture.accountUnits(t, substituted.Account.ID)[0]
		if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.kitchen_tasks WHERE unit_id=$1", unit) != 2 ||
			rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_consumption_requests WHERE restaurant_unit_id=$1", unit) != 1 {
			t.Fatal("El filete no se envió a dos estaciones con una sola solicitud")
		}
		expectNet(t, "arroz por pasta", fixture.consumption(t, substituted.Account.ID), map[string]string{
			"B-FILETE@Cocina B": "180", "B-LECHUGA@Cocina B": "40", "B-PASTA@Cocina B": "100",
		})
		// «2 porciones de papas»: 2 × la receta del complemento.
		fries := fixture.order(t, "Filete con papas", "dine_in", "", group(option("Papas B", "2")))
		fixture.process(t)
		expectNet(t, "dos porciones de papas", fixture.consumption(t, fries.Account.ID), map[string]string{
			"B-FILETE@Cocina B": "180", "B-PAPA@Cocina B": "300",
		})
		// Factor de la porción del catálogo: 2 medias porciones de arroz = 2 × 0.5 × 90 g.
		halves := fixture.order(t, "Filete con medias", "dine_in", "", group(option("Arroz B media", "2")))
		fixture.process(t)
		expectNet(t, "medias porciones", fixture.consumption(t, halves.Account.ID), map[string]string{
			"B-FILETE@Cocina B": "180", "B-ARROZ@Cocina B": "90",
		})
	})
	t.Run("inventory_b_combo_two_stations_and_takeaway", func(t *testing.T) {
		var mainSlot string
		for _, slot := range fixture.products["combo"].Slots {
			if slot.Name == "Principal" {
				mainSlot = slot.ID
			}
		}
		combo := catalog.LineRequest{ProductID: fixture.products["combo"].ID, Quantity: 1, Customization: catalog.Customization{
			Components: []catalog.ComponentSelection{{
				SlotID: mainSlot, ProductID: fixture.products["hamburguesa"].ID, Quantity: 1,
				Customization: catalog.Customization{Ingredients: []catalog.IngredientSelection{
					{IngredientID: fixture.ingredients["Cebolla"].ID, Quantity: "0"},
				}},
			}},
		}}
		bill := fixture.order(t, "Combo personalizado", "dine_in", "", combo)
		fixture.process(t)
		unit := fixture.accountUnits(t, bill.Account.ID)[0]
		stations := rowCount(t, fixture.restaurantFixture, "SELECT count(DISTINCT station_id) FROM app.kitchen_tasks WHERE unit_id=$1", unit)
		if stations != 2 || rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_consumption_requests WHERE restaurant_unit_id=$1",
			unit) != 1 {
			t.Fatalf("El combo debía ir a dos estaciones con una sola solicitud: %d estaciones", stations)
		}
		// Hamburguesa sin cebolla y charola en Cocina B; las papas en Freidora B; el refresco (Sin
		// preparación) hereda la estación del combo. Cada componente una sola vez.
		expectNet(t, "combo", fixture.consumption(t, bill.Account.ID), map[string]string{
			"B-CHAROLA@Cocina B": "1", "B-PAN@Cocina B": "1", "B-CARNE@Cocina B": "150", "B-QUESO@Cocina B": "1", "B-LECHUGA@Cocina B": "15",
			"B-PAPA@Freidora B": "120", "B-LATA@Cocina B": "1",
		})
		// Componentes «solo para llevar»: para llevar y ventanilla sí, salón no (caso anterior).
		for _, origin := range []string{"Para llevar", "Ventanilla"} {
			toGo := fixture.order(t, "Bolsa "+origin, "takeaway", origin, fixture.burgerLine(1, "0", "0", "", ""))
			fixture.process(t)
			expectNet(t, origin, fixture.consumption(t, toGo.Account.ID), map[string]string{
				"B-PAN@Cocina B": "1", "B-CARNE@Cocina B": "150", "B-QUESO@Cocina B": "1", "B-BOLSA@Cocina B": "1",
			})
		}
	})
	t.Run("inventory_b_operations_without_consumption", func(t *testing.T) {
		requests, movements := fixture.requests(t), fixture.unitMovements(t)
		// Reimpresión y cambio de impresora de la comanda de la liberación.
		release := kitchenReleases(t, fixture.restaurantFixture, released)[0]
		var jobID string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT j.id FROM app.kitchen_print_jobs j JOIN app.kitchen_tickets k ON k.id=j.ticket_id
            WHERE k.release_id=$1 ORDER BY j.created_at LIMIT 1`, release.ID).Scan(&jobID); err != nil {
			t.Fatal(err)
		}
		job := decodeResponse[struct {
			Job kitchen.PrintJob `json:"job"`
		}](t, fixture.adminCall(t, "GET", "kitchen/jobs/"+jobID, nil, 200)).Job
		fixture.mutate(t, "POST", "kitchen/jobs/"+jobID+"/resolve", map[string]any{"revision": job.Revision, "decision": "cancel",
			"reason": "Ensayo de reimpresión de inventario"}, 200)
		fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{"job_id": jobID, "printer_id": fixture.printers[1].ID,
			"reason": "Copia en otra impresora"}, 200)
		// División, transferencia, combinación y cambio de mesa.
		current := fixture.refresh(t, released.Account.ID)
		pair := decodeResponse[struct {
			Source operations.Bill `json:"source"`
			Target operations.Bill `json:"target"`
		}](t, fixture.mutate(t, "POST", "restaurant/accounts/"+current.Account.ID+"/split", operations.TransferInput{
			Mutation:   operations.Mutation{Revision: current.Account.Revision},
			NewAccount: &operations.AccountInput{Name: "Dividida de inventario"},
			Selections: []operations.Selection{{VersionID: current.Lines[0].VersionID, Quantity: 1}},
		}, 200))
		fixture.mutate(t, "POST", "restaurant/accounts/"+pair.Target.Account.ID+"/move", map[string]any{
			"revision": pair.Target.Account.Revision, "table_id": fixture.secondTable.ID,
		}, 200)
		target := fixture.refresh(t, pair.Target.Account.ID)
		source := fixture.refresh(t, pair.Source.Account.ID)
		fixture.mutate(t, "POST", "restaurant/accounts/"+source.Account.ID+"/merge", map[string]any{
			"revision": source.Account.Revision, "target_account_id": target.Account.ID, "target_revision": target.Account.Revision,
		}, 200)
		if fixture.requests(t) != requests || fixture.process(t) != 0 || fixture.unitMovements(t) != movements {
			t.Fatal("Reimprimir, cambiar de impresora, dividir, mover o combinar generó consumo")
		}
		released = fixture.refresh(t, target.Account.ID)
	})
	t.Run("inventory_b_modification_difference", func(t *testing.T) {
		bill := fixture.order(t, "Modificación", "dine_in", "", fixture.burgerLine(1, "", "", "", ""))
		fixture.process(t)
		unit := fixture.accountUnits(t, bill.Account.ID)[0]
		var originalLettuce string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT value::text FROM app.inventory_movements WHERE restaurant_unit_id=$1
            AND item_id=$2 AND kind='consumption'`, unit, fixture.items["lechuga"].ID).Scan(&originalLettuce); err != nil {
			t.Fatal(err)
		}
		movements := fixture.unitMovements(t)
		// Cebolla extra y sin lechuga: solo la diferencia (+20 g de cebolla y la reversión exacta de
		// los 15 g de lechuga), sin volver a consumir la receta.
		fixture.editLine(t, bill, fixture.burgerLine(1, "2", "0", "", ""))
		if fixture.process(t) != 1 || fixture.unitMovements(t) != movements+2 {
			t.Fatal("La modificación no registró solo la diferencia")
		}
		expectNet(t, "modificada", fixture.unitConsumption(t, unit), map[string]string{
			"B-PAN@Cocina B": "1", "B-CARNE@Cocina B": "150", "B-QUESO@Cocina B": "1", "B-CEBOLLA@Cocina B": "40",
		})
		var reversal string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT value::text FROM app.inventory_movements WHERE restaurant_unit_id=$1
            AND item_id=$2 AND kind='consumption_reversal' AND source_kind='amendment'`, unit, fixture.items["lechuga"].ID).Scan(&reversal); err != nil {
			t.Fatal(err)
		}
		if !decimal.RequireFromString(reversal).Equal(decimal.RequireFromString(originalLettuce).Neg()) {
			t.Fatalf("La reversión de la lechuga no fue exacta: %s frente a %s", reversal, originalLettuce)
		}
	})
	t.Run("inventory_b_recipe_change_between_release_and_processing", func(t *testing.T) {
		bill := fixture.order(t, "Receta cambiada", "dine_in", "", fixture.burgerLine(1, "0", "0", "", ""))
		recipe := fixture.recipes["hamburguesa"]
		units := fixture.units
		changed := decodeResponse[inventory.Recipe](t, fixture.write(t, fixture.recipeManager, "PUT", "inventory/recipes/"+recipe.ID, map[string]any{
			"revision": recipe.Revision, "active": true, "lines": []map[string]any{
				recipeItemLine(fixture.items["pan"], "1", units["pza"], false), recipeItemLine(fixture.items["carne"], "200", units["g"], false),
				recipeItemLine(fixture.items["queso"], "1", units["pza"], false), recipeItemLine(fixture.items["bolsa"], "1", units["pza"], true),
			}}, 200))
		fixture.process(t)
		// El consumo usa la versión vigente al liberar (150 g), no la guardada después (200 g).
		expectNet(t, "versión anterior", fixture.consumption(t, bill.Account.ID), map[string]string{
			"B-PAN@Cocina B": "1", "B-CARNE@Cocina B": "150", "B-QUESO@Cocina B": "1",
		})
		next := fixture.order(t, "Receta nueva", "dine_in", "", fixture.burgerLine(1, "0", "0", "", ""))
		fixture.process(t)
		expectNet(t, "versión nueva", fixture.consumption(t, next.Account.ID), map[string]string{
			"B-PAN@Cocina B": "1", "B-CARNE@Cocina B": "200", "B-QUESO@Cocina B": "1",
		})
		restored := decodeResponse[inventory.Recipe](t, fixture.write(t, fixture.recipeManager, "PUT", "inventory/recipes/"+recipe.ID, map[string]any{
			"revision": changed.Revision, "active": true, "lines": []map[string]any{
				recipeItemLine(fixture.items["pan"], "1", units["pza"], false), recipeItemLine(fixture.items["carne"], "150", units["g"], false),
				recipeItemLine(fixture.items["queso"], "1", units["pza"], false), recipeItemLine(fixture.items["bolsa"], "1", units["pza"], true),
			}}, 200))
		fixture.recipes["hamburguesa"] = restored
	})
	testCancellations(t, fixture)
	t.Run("inventory_b_stored_subrecipe_without_double_discount", func(t *testing.T) {
		before := map[string]int{}
		for _, key := range []string{"arroz", "arroz_rojo"} {
			before[key] = rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements WHERE item_id=$1", fixture.items[key].ID)
		}
		bill := fixture.order(t, "Enchiladas", "dine_in", "", catalog.LineRequest{ProductID: fixture.products["enchiladas"].ID, Quantity: 1})
		fixture.process(t)
		// 3 tortillas, la salsa descompuesta (100 ml = 80 g de tomate y 10 g de chile) y 200 g de
		// arroz rojo preparado: nunca el arroz crudo con que se elaboró.
		expectNet(t, "enchiladas", fixture.consumption(t, bill.Account.ID), map[string]string{
			"B-TORTILLA@Cocina B": "3", "B-TOMATE@Cocina B": "80", "B-CHILE@Cocina B": "10", "B-ARROZROJO@Cocina B": "200",
		})
		if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements WHERE item_id=$1", fixture.items["arroz"].ID) != before["arroz"] ||
			rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements WHERE item_id=$1", fixture.items["arroz_rojo"].ID) != before["arroz_rojo"]+1 {
			t.Fatal("La venta volvió a descontar los ingredientes de la subreceta elaborada")
		}
		// El costo del arroz rojo es el de su elaboración: 200 g × 56.80 / 4750 g.
		var value string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT value::text FROM app.inventory_movements WHERE item_id=$1 AND kind='consumption'`,
			fixture.items["arroz_rojo"].ID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		if !decimal.RequireFromString(value).Equal(decimal.RequireFromString("-2.391579")) {
			t.Fatalf("Valor del arroz rojo consumido: %s", value)
		}
	})
	t.Run("inventory_b_refund_without_reintegration", func(t *testing.T) {
		testRefundWithoutReintegration(t, fixture)
	})
	t.Run("inventory_b_failures_and_diagnostics", func(t *testing.T) {
		testConsumptionFailures(t, fixture)
	})
	t.Run("inventory_b_events_and_ledger", func(t *testing.T) {
		checkLedgerConsistency(t, fixture.inventoryFixture)
		var processes, missing int
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FILTER (WHERE process='inventory_worker'),
                count(*) FILTER (WHERE source_kind IN ('consumption','amendment','cancellation') AND restaurant_unit_id IS NULL)
            FROM app.inventory_movements`).Scan(&processes, &missing); err != nil {
			t.Fatal(err)
		}
		if processes == 0 || missing != 0 {
			t.Fatalf("Movimientos del trabajador %d; consumos sin unidad %d", processes, missing)
		}
		// Eventos del ámbito inventory publicados en las transacciones del trabajador, de la
		// clasificación y de las recetas; solo referencias, sin contenido (ningún costo por SSE).
		rows, err := fixture.pool.Query(fixture.ctx, `SELECT DISTINCT kind FROM app.realtime_events WHERE scope='inventory' AND sequence>$1
            ORDER BY kind`, cursor)
		if err != nil {
			t.Fatal(err)
		}
		kinds := []string{}
		for rows.Next() {
			var kind string
			if err := rows.Scan(&kind); err != nil {
				t.Fatal(err)
			}
			kinds = append(kinds, kind)
		}
		rows.Close()
		for _, kind := range []string{"inventory.alerts.changed", "inventory.movements.recorded", "inventory.recipes.changed",
			"inventory.stock.changed"} {
			if !slices.Contains(kinds, kind) {
				t.Fatalf("Falta el evento %s: %v", kind, kinds)
			}
		}
		if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.realtime_events WHERE scope='inventory' AND payload<>'{}'::jsonb") != 0 {
			t.Fatal("Un evento de inventario lleva contenido")
		}
	})
	measureReleaseWithInventory(t, fixture)
}

func testCancellations(t *testing.T, fixture *recipesFixture) {
	t.Run("inventory_b_reduction_and_not_prepared", func(t *testing.T) {
		bill := fixture.order(t, "Reducción", "dine_in", "", fixture.burgerLine(2, "", "", "", ""))
		fixture.process(t)
		requests := fixture.requests(t)
		// Reducir la cantidad cancela una unidad liberada: queda una cancelación por clasificar.
		// La unidad que permanece se registra como modificada (sin diferencia) y la sobrante como
		// cancelada.
		movements := fixture.unitMovements(t)
		fixture.editLine(t, bill, fixture.burgerLine(1, "", "", "", ""))
		if fixture.requests(t) != requests+2 || fixture.process(t) != 2 || fixture.unitMovements(t) != movements {
			t.Fatal("La reducción de cantidad no registró la cancelación")
		}
		var unit string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT id FROM app.restaurant_item_units WHERE account_id=$1 AND state='cancelled'`,
			bill.Account.ID).Scan(&unit); err != nil {
			t.Fatal(err)
		}
		pending := fixture.classification(t, unit)
		if pending == nil || pending.State != "pending" || pending.Expired || !pending.DeadlineAt.After(pending.CancelledAt) ||
			len(pending.Lines) != 5 {
			t.Fatalf("Cancelación pendiente: %+v", pending)
		}
		listed := decodeResponse[inventory.CancellationPage](t, fixture.call(t, fixture.wasteClerk, "GET", "inventory/cancellations?state=pending", nil, 200))
		if listed.Pending < 1 || !listed.Items[0].CanClassify || listed.Items[0].Lines[0].Value != nil {
			t.Fatalf("Listado de cancelaciones pendientes: %+v", listed)
		}
		// Sin permiso no se clasifica; con inventory.waste.record, «No preparado» revierte exacto.
		fixture.write(t, fixture.stock, "POST", "inventory/cancellations/"+pending.ID+"/classify",
			map[string]any{"revision": pending.Revision, "state": "not_prepared"}, 403)
		saved := decodeResponse[inventory.Cancellation](t, fixture.write(t, fixture.wasteClerk, "POST",
			"inventory/cancellations/"+pending.ID+"/classify", map[string]any{"revision": pending.Revision, "state": "not_prepared",
				"notes": "No se alcanzó a preparar"}, 200))
		if saved.State != "not_prepared" || saved.Resolution == nil || *saved.Resolution != "manual" || saved.ClassifiedByName == nil {
			t.Fatalf("Clasificación «No preparado»: %+v", saved)
		}
		var quantity, value string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT sum(quantity)::text,sum(value)::text FROM app.inventory_movements
            WHERE restaurant_unit_id=$1 AND kind IN ('consumption','consumption_reversal')`, unit).Scan(&quantity, &value); err != nil {
			t.Fatal(err)
		}
		if !decimal.RequireFromString(quantity).IsZero() || !decimal.RequireFromString(value).IsZero() {
			t.Fatalf("La reversión no fue exacta: cantidad %s, valor %s", quantity, value)
		}
		expectNet(t, "unidad restante", fixture.consumption(t, bill.Account.ID), map[string]string{
			"B-PAN@Cocina B": "1", "B-CARNE@Cocina B": "150", "B-QUESO@Cocina B": "1", "B-CEBOLLA@Cocina B": "20", "B-LECHUGA@Cocina B": "15",
		})
	})
	t.Run("inventory_b_cancellation_as_waste", func(t *testing.T) {
		bill := fixture.order(t, "Merma", "dine_in", "", fixture.burgerLine(1, "", "", "", ""))
		fixture.process(t)
		unit := fixture.accountUnits(t, bill.Account.ID)[0]
		fixture.cancelOne(t, bill)
		fixture.process(t)
		pending := fixture.classification(t, unit)
		movements := fixture.unitMovements(t)
		saved := decodeResponse[inventory.Cancellation](t, fixture.write(t, fixture.wasteClerk, "POST",
			"inventory/cancellations/"+pending.ID+"/classify", map[string]any{"revision": pending.Revision, "state": "prepared_waste"}, 200))
		if saved.State != "prepared_waste" || fixture.unitMovements(t) != movements {
			t.Fatalf("«Preparado / merma» no debe mover existencias: %+v", saved)
		}
		expectNet(t, "merma", fixture.unitConsumption(t, unit), map[string]string{
			"B-PAN@Cocina B": "1", "B-CARNE@Cocina B": "150", "B-QUESO@Cocina B": "1", "B-CEBOLLA@Cocina B": "20", "B-LECHUGA@Cocina B": "15",
		})
		// Repetir la misma clasificación no escribe nada; cambiarla antes del corte genera la
		// reversión compensatoria.
		fixture.write(t, fixture.wasteClerk, "POST", "inventory/cancellations/"+pending.ID+"/classify",
			map[string]any{"revision": saved.Revision, "state": "prepared_waste"}, 200)
		changed := decodeResponse[inventory.Cancellation](t, fixture.write(t, fixture.wasteClerk, "POST",
			"inventory/cancellations/"+pending.ID+"/classify", map[string]any{"revision": saved.Revision, "state": "not_prepared"}, 200))
		if changed.State != "not_prepared" || len(fixture.unitConsumption(t, unit)) != 0 {
			t.Fatalf("El cambio de clasificación no revirtió el consumo: %+v", changed)
		}
	})
	t.Run("inventory_b_reconciliation_after_cutoff", func(t *testing.T) {
		bill := fixture.order(t, "Sin clasificar", "dine_in", "", fixture.burgerLine(1, "", "", "", ""))
		fixture.process(t)
		unit := fixture.accountUnits(t, bill.Account.ID)[0]
		fixture.cancelOne(t, bill)
		fixture.process(t)
		pending := fixture.classification(t, unit)
		// Antes del corte no se reconcilia nada.
		if reconciled, err := inventory.ReconcileCancellations(fixture.ctx, fixture.pool, pending.DeadlineAt.Add(-time.Minute)); err != nil || reconciled != 0 {
			t.Fatalf("Se reconcilió antes del corte: %d %v", reconciled, err)
		}
		// Pasado el corte (hora simulada), «No preparado» automático; repetirlo no hace nada.
		reconciled, err := inventory.ReconcileCancellations(fixture.ctx, fixture.pool, pending.DeadlineAt.Add(time.Minute))
		if err != nil || reconciled < 1 {
			t.Fatalf("No se reconcilió la cancelación vencida: %d %v", reconciled, err)
		}
		if again, err := inventory.ReconcileCancellations(fixture.ctx, fixture.pool, pending.DeadlineAt.Add(time.Minute)); err != nil || again != 0 {
			t.Fatalf("La reconciliación no es idempotente: %d %v", again, err)
		}
		automatic := fixture.classification(t, unit)
		if automatic.State != "not_prepared" || automatic.Resolution == nil || *automatic.Resolution != "automatic" ||
			automatic.ClassifiedByName != nil || len(fixture.unitConsumption(t, unit)) != 0 {
			t.Fatalf("Clasificación automática: %+v", automatic)
		}
		if rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.audit_events WHERE action='inventory.cancellation_reconciled'
            AND actor_kind='process' AND actor_username='inventory_worker' AND entity_id=$1`, automatic.ID) != 1 {
			t.Fatal("La reconciliación automática no se auditó con el proceso como responsable")
		}
		// Después del corte, corregir exige inventory.adjustments.authorize: la «corrección» a merma
		// vuelve a consumir lo revertido con movimientos nuevos.
		fixture.write(t, fixture.wasteClerk, "POST", "inventory/cancellations/"+automatic.ID+"/classify",
			map[string]any{"revision": automatic.Revision, "state": "prepared_waste"}, 403)
		corrected := decodeResponse[inventory.Cancellation](t, fixture.adminWrite(t, "POST", "inventory/cancellations/"+automatic.ID+"/classify",
			map[string]any{"revision": automatic.Revision, "state": "prepared_waste", "notes": "Sí se preparó"}, 200))
		if corrected.State != "prepared_waste" || corrected.Resolution == nil || *corrected.Resolution != "correction" {
			t.Fatalf("Corrección autorizada: %+v", corrected)
		}
		expectNet(t, "corregida", fixture.unitConsumption(t, unit), map[string]string{
			"B-PAN@Cocina B": "1", "B-CARNE@Cocina B": "150", "B-QUESO@Cocina B": "1", "B-CEBOLLA@Cocina B": "20", "B-LECHUGA@Cocina B": "15",
		})
	})
	t.Run("inventory_b_worker_start_processes_pending", func(t *testing.T) {
		// Go estuvo apagado durante el corte: al iniciar, el trabajador procesa lo pendiente y
		// reconcilia las cancelaciones vencidas.
		first := fixture.order(t, "Apagado", "dine_in", "", fixture.burgerLine(1, "", "", "", ""))
		fixture.process(t)
		unit := fixture.accountUnits(t, first.Account.ID)[0]
		fixture.cancelOne(t, first)
		fixture.process(t)
		pending := fixture.classification(t, unit)
		if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.inventory_cancellation_classifications
            SET cancelled_at=now()-interval '2 days',deadline_at=now()-interval '1 day' WHERE id=$1`, pending.ID); err != nil {
			t.Fatal(err)
		}
		waiting := fixture.order(t, "Pendiente al iniciar", "dine_in", "", fixture.burgerLine(1, "", "", "", ""))
		ctx, cancel := context.WithCancel(fixture.ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			(&inventory.Worker{Pool: fixture.pool}).Run(ctx)
		}()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			state := fixture.classification(t, unit)
			if state.State == "not_prepared" && len(fixture.consumption(t, waiting.Account.ID)) > 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		cancel()
		<-done
		if fixture.classification(t, unit).State != "not_prepared" || len(fixture.consumption(t, waiting.Account.ID)) != 5 {
			t.Fatal("El trabajador no procesó lo pendiente al iniciar")
		}
	})
}

func testRefundWithoutReintegration(t *testing.T, fixture *recipesFixture) {
	methods := map[string]string{}
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil {
			methods[*method.Code] = method.ID
		}
	}
	// Un turno abierto en la caja de la terminal (el de la aceptación o uno nuevo).
	status := rawStatus(fixture.restaurantFixture, fixture.admin, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0"})
	if status != 200 && status != 409 {
		t.Fatalf("No se pudo abrir un turno para el cobro: %d", status)
	}
	bill := fixture.order(t, "Devolución", "dine_in", "", fixture.burgerLine(1, "", "", "", ""))
	fixture.process(t)
	consumed := fixture.consumption(t, bill.Account.ID)
	finance := decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+bill.Account.ID, nil, 200))
	fixture.adminCall(t, "POST", "payments/accounts/"+bill.Account.ID+"/payments", map[string]any{"operation_id": randomID(),
		"method_id": methods["debit"], "mode": "total", "reference": "AUT-INV-1", "expected_balance": finance.Balance}, 200)
	requests, movements := fixture.requests(t), fixture.unitMovements(t)
	fixture.adminCall(t, "POST", "payments/accounts/"+bill.Account.ID+"/refunds", map[string]any{"operation_id": randomID(),
		"mode": "total", "method_id": methods["debit"], "reason": "Devolución de inventario"}, 200)
	if fixture.requests(t) != requests || fixture.process(t) != 0 || fixture.unitMovements(t) != movements {
		t.Fatal("La devolución financiera reintegró ingredientes")
	}
	expectNet(t, "devolución", fixture.consumption(t, bill.Account.ID), consumed)
}

// rawStatus envía una petición y devuelve solo su código HTTP.
func rawStatus(fixture *restaurantFixture, terminal testTerminal, method, route string, body any) int {
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:51000"
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response.Code
}

func testConsumptionFailures(t *testing.T, fixture *recipesFixture) {
	poisoned := fixture.order(t, "Falla controlada", "dine_in", "", fixture.burgerLine(1, "", "", "", ""))
	healthy := fixture.order(t, "Unidad sana", "dine_in", "", fixture.burgerLine(1, "", "", "", ""))
	unit := fixture.accountUnits(t, poisoned.Account.ID)[0]
	// Un disparador de prueba hace fallar la solicitud de una sola unidad.
	if _, err := fixture.pool.Exec(fixture.ctx, `CREATE FUNCTION app.inventory_test_failure() RETURNS trigger LANGUAGE plpgsql AS $$
        BEGIN RAISE EXCEPTION 'falla de prueba'; END; $$;
        CREATE TRIGGER inventory_test_failure BEFORE INSERT ON app.inventory_unit_consumptions FOR EACH ROW
        WHEN (NEW.restaurant_unit_id='`+unit+`') EXECUTE FUNCTION app.inventory_test_failure()`); err != nil {
		t.Fatal(err)
	}
	cursor := eventCursor(t, fixture.restaurantFixture)
	fixture.process(t)
	if kinds := eventKinds(fixture.inventoryEvents(t, fixture.stock, cursor)); !slices.Contains(kinds, "inventory.alerts.changed") ||
		!slices.Contains(kinds, "inventory.stock.changed") || !slices.Contains(kinds, "inventory.movements.recorded") {
		t.Fatalf("Eventos del trabajador: %v", kinds)
	}
	var state, lastError string
	var attempts int
	var next time.Time
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT state,last_error,attempts,next_attempt_at FROM app.inventory_consumption_requests
        WHERE restaurant_unit_id=$1`, unit).Scan(&state, &lastError, &attempts, &next); err != nil {
		t.Fatal(err)
	}
	if state != "failed" || attempts != 1 || strings.Contains(lastError, "prueba") || len(fixture.consumption(t, poisoned.Account.ID)) != 0 {
		t.Fatalf("Solicitud fallida: %s %d %v %q", state, attempts, next, lastError)
	}
	// Espera creciente: no se reintenta en la vuelta inmediata.
	fixture.process(t)
	if rowCount(t, fixture.restaurantFixture, "SELECT attempts FROM app.inventory_consumption_requests WHERE restaurant_unit_id=$1", unit) != 1 {
		t.Fatal("La solicitud fallida se reintentó sin esperar")
	}
	// La falla de una unidad no detiene a las demás.
	if len(fixture.consumption(t, healthy.Account.ID)) != 5 {
		t.Fatal("La falla de una unidad detuvo el consumo de otra")
	}
	listed := decodeResponse[inventory.ConsumptionRequestPage](t, fixture.call(t, fixture.stock, "GET", "inventory/consumption-requests", nil, 200))
	if listed.Failed != 1 || len(listed.Items) != 1 || listed.Items[0].State != "failed" || listed.Items[0].LastError == nil {
		t.Fatalf("Diagnóstico de solicitudes fallidas: %+v", listed)
	}
	fixture.call(t, fixture.wasteClerk, "GET", "inventory/consumption-requests", nil, 403)
	// Corregida la causa y cumplida la espera, se procesa sin duplicar.
	if _, err := fixture.pool.Exec(fixture.ctx, `DROP TRIGGER inventory_test_failure ON app.inventory_unit_consumptions;
        DROP FUNCTION app.inventory_test_failure();
        UPDATE app.inventory_consumption_requests SET next_attempt_at=now()-interval '1 second' WHERE restaurant_unit_id='`+unit+`'`); err != nil {
		t.Fatal(err)
	}
	if fixture.process(t) != 1 || len(fixture.consumption(t, poisoned.Account.ID)) != 5 {
		t.Fatal("La solicitud no se procesó tras corregir la falla")
	}
	// Traza de una unidad: cantidades para quien consulta movimientos; valores solo con costos.
	hidden := decodeResponse[inventory.UnitConsumptionTrace](t, fixture.call(t, fixture.stock, "GET", "inventory/consumption/"+unit, nil, 200))
	visible := decodeResponse[inventory.UnitConsumptionTrace](t, fixture.call(t, fixture.costs, "GET", "inventory/consumption/"+unit, nil, 200))
	if !hidden.Consumed || len(hidden.Lines) != 5 || hidden.Lines[0].Value != nil || hidden.CostsVisible || len(hidden.Movements) != 5 ||
		hidden.Movements[0].Value != nil || visible.Lines[0].Value == nil || len(hidden.Requests) != 1 || hidden.Requests[0].Attempts != 1 {
		t.Fatalf("Traza de consumo: %+v / %+v", hidden, visible)
	}
	fixture.call(t, fixture.wasteClerk, "GET", "inventory/consumption/"+unit, nil, 403)
}

// measureReleaseWithInventory mide (§32.17) el tiempo de la confirmación con envío a cocina con la
// función activada y desactivada. Solo se ejecuta con ADEL_INVENTORY_MEASURE=1 (prueba marcada
// para medir); informa media y percentil 95 de cada caso.
func measureReleaseWithInventory(t *testing.T, fixture *recipesFixture) {
	t.Run("inventory_b_measure_release", func(t *testing.T) {
		if os.Getenv("ADEL_INVENTORY_MEASURE") != "1" {
			t.Skip("Medición opt-in: ADEL_INVENTORY_MEASURE=1")
		}
		iterations := 200
		durations := map[bool][]time.Duration{}
		confirm := func(enabled bool) {
			bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": "Medición", "mode": "dine_in"}, 200))
			route := "restaurant/accounts/" + bill.Account.ID
			draft := []operations.DraftItem{{ID: randomID(), LineRequest: fixture.burgerLine(1, "", "", "", "")}}
			bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", route+"/draft", map[string]any{"revision": bill.Account.Revision, "items": draft}, 200))
			preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "GET", route+"/preview", nil, 200))
			started := time.Now()
			fixture.mutate(t, "POST", route+"/confirm", map[string]any{"revision": bill.Account.Revision, "expected_fingerprint": preview.Fingerprint}, 200)
			durations[enabled] = append(durations[enabled], time.Since(started))
		}
		// Calentamiento y bloques alternados de 50 para no favorecer a ninguno de los dos casos.
		for _, enabled := range []bool{false, true} {
			fixture.setFeature(t, enabled)
			for index := 0; index < 5; index++ {
				confirm(enabled)
			}
		}
		durations = map[bool][]time.Duration{}
		for block := 0; block < iterations/50; block++ {
			for _, enabled := range []bool{false, true} {
				fixture.setFeature(t, enabled)
				for index := 0; index < 50; index++ {
					confirm(enabled)
				}
			}
		}
		started := time.Now()
		processed := fixture.process(t)
		elapsed := time.Since(started)
		for _, enabled := range []bool{false, true} {
			list := durations[enabled]
			sort.Slice(list, func(left, right int) bool { return list[left] < list[right] })
			var total time.Duration
			for _, duration := range list {
				total += duration
			}
			t.Logf("Medición confirmación con envío a cocina, inventario activado=%v: n=%d media=%v p95=%v máx=%v", enabled, len(list),
				total/time.Duration(len(list)), list[len(list)*95/100], list[len(list)-1])
		}
		if processed != iterations+5 {
			t.Fatalf("El trabajador procesó %d de %d solicitudes", processed, iterations+5)
		}
		t.Logf("Medición trabajador: %d solicitudes en %v (%v por solicitud)", processed, elapsed, elapsed/time.Duration(processed))
		fixture.setFeature(t, true)
	})
}
