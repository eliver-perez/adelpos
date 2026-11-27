//go:build integration

package operations_test

import (
	"encoding/json"
	"testing"

	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
)

// Etapa M1 (Fase 0, prueba de reproducción de §5.2; para la Fase A). Con la configuración del propietario —estación
// «Cocina» con recepción «Solo pantalla de cocina», Hamburguesas a Cocina, Refrescos (Coca Cola) sin ruta con
// «Productos sin ruta» = «No necesitan preparación» (lo de fábrica de la K1.1) y Extras (papas) con ruta «Sin
// preparación»— Go debe decir, por renglón y por cuenta, si lo enviado necesita preparación, para que el escritorio y
// Android no pongan ninguna etiqueta de cocina a lo que no la necesita (decisión del propietario, §5.2) sin calcularlo
// ellos.
//
// Hoy falla: un renglón que no necesita preparación (liberado sin tareas) recibe preparation_state «ready» en cuanto se
// envía (kitchen.VersionPreparation, states.go:236, cuenta 0 tareas = 0 listas), igual que la hamburguesa ya preparada,
// y no hay ningún otro dato que los distinga. El nombre del campo (preparation_required) es la Decisión propuesta de la
// Fase 0; la Fase A lo ajusta si el coordinador decide otro.
func TestStageM1NoPreparationLines(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &unroutedFixture{kitchenDisplayFixture: &kitchenDisplayFixture{waiterModeFixture: &waiterModeFixture{
		restaurantFixture: newFixture(t)}}}
	hamburguesas, hamburguesa := fixture.product(t, "Hamburguesas", "Hamburguesa sencilla", "HAMBURGUESA", "120")
	_, coca := fixture.product(t, "Refrescos", "Coca Cola 600ml", "COCA-600", "35")
	extras, papas := fixture.product(t, "Extras", "Papas a la francesa", "PAPAS-FRANCESA", "45")
	fixture.cocina = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina", Active: true, Reception: "kds", Tracking: "inherit", Grouping: "inherit",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &hamburguesas.ID, Mode: "prepare",
		StationIDs: []string{fixture.cocina.ID}}, 200)
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &extras.ID, Mode: "none", StationIDs: []string{}}, 200)
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón M1", Active: true}, 200))
	for index, name := range []string{"Mesa 1", "Mesa 2"} {
		fixture.tables = append(fixture.tables, decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: name, Shape: "round", X: 20 + index*110, Y: 30, Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200)))
	}
	inventoryUser(t, fixture.restaurantFixture, "mesero_m1", waiterPermissions)
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username='mesero_m1'").Scan(&fixture.waiterID); err != nil {
		t.Fatal(err)
	}
	fixture.display = fixture.kitchenDevice(t, "Tableta de cocina M1")
	fixture.displayCall(t, fixture.display, "POST", "kitchen-display/connect", map[string]any{}, 200)

	// read devuelve la cuenta tal como la leen el escritorio y Android (GET restaurant/accounts/{id}), sin tipos, para
	// ver los campos nuevos.
	read := func(t *testing.T, accountID string) (map[string]any, map[string]map[string]any) {
		t.Helper()
		var bill struct {
			Account map[string]any   `json:"account"`
			Lines   []map[string]any `json:"lines"`
		}
		if err := json.Unmarshal(fixture.adminCall(t, "GET", "restaurant/accounts/"+accountID, nil, 200), &bill); err != nil {
			t.Fatal(err)
		}
		lines := map[string]map[string]any{}
		for _, line := range bill.Lines {
			lines[line["quote_line"].(map[string]any)["name"].(string)] = line
		}
		return bill.Account, lines
	}
	expectRequired := func(t *testing.T, where string, value map[string]any, want bool) {
		t.Helper()
		got, present := value["preparation_required"]
		if !present {
			t.Fatalf("%s: Go no dice si necesita preparación (preparation_required ausente); preparation_state=%v",
				where, value["preparation_state"])
		}
		if got != want {
			t.Fatalf("%s: preparation_required=%v, esperado %v (preparation_state=%v)", where, got, want, value["preparation_state"])
		}
	}

	t.Run("hamburguesa, Coca Cola sin ruta y papas «Sin preparación» en la misma cuenta", func(t *testing.T) {
		accountID := fixture.order(t, 0, draftLine(hamburguesa, 1), draftLine(coca, 1), draftLine(papas, 1))
		account, lines := read(t, accountID)
		// Hoy lo que no necesita preparación ya dice «ready» al enviarlo: no se distingue de lo preparado.
		for _, name := range []string{"Coca Cola 600ml", "Papas a la francesa"} {
			if lines[name]["preparation_state"] != "ready" {
				t.Logf("%s: preparation_state=%v", name, lines[name]["preparation_state"])
			}
			expectRequired(t, name+" recién enviada", lines[name], false)
		}
		expectRequired(t, "hamburguesa recién enviada", lines["Hamburguesa sencilla"], true)
		expectRequired(t, "la cuenta con algo que preparar", account, true)
		found := card(t, fixture.orders(t, fixture.display), accountID)
		if found == nil {
			t.Fatal("la pantalla no recibió la hamburguesa")
		}
		fixture.mark(t, fixture.display, "orders/ready", refs(found.Items...))
		account, lines = read(t, accountID)
		if lines["Hamburguesa sencilla"]["preparation_state"] != "ready" || account["preparation_state"] != "ready" {
			t.Fatalf("después de «Pedido listo»: hamburguesa %v, cuenta %v", lines["Hamburguesa sencilla"]["preparation_state"],
				account["preparation_state"])
		}
		expectRequired(t, "hamburguesa lista", lines["Hamburguesa sencilla"], true)
		expectRequired(t, "Coca Cola con la hamburguesa lista", lines["Coca Cola 600ml"], false)
	})

	t.Run("una cuenta con solo lo que no necesita preparación", func(t *testing.T) {
		accountID := fixture.order(t, 1, draftLine(coca, 1), draftLine(papas, 1))
		account, lines := read(t, accountID)
		expectRequired(t, "Coca Cola", lines["Coca Cola 600ml"], false)
		expectRequired(t, "papas", lines["Papas a la francesa"], false)
		// La cuenta entera no necesita preparación: el escritorio y Android tampoco le ponen etiqueta de cocina.
		expectRequired(t, "la cuenta", account, false)
	})

}
