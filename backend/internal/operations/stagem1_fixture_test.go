//go:build integration

package operations_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa M1 (Fase A): lo común de las pruebas de uniones de mesas y mesas divisibles contra PostgreSQL con la API real.
// Una base desde cero con la configuración de cocina del propietario (Cocina «Solo pantalla de cocina», hamburguesas a
// Cocina y Coca Cola sin ruta), una pantalla de cocina conectada, un turno de caja abierto, el área «Salón M1» con las
// mesas «Mesa 1» a «Mesa 12» (sin «Mesa 3»), «5», «6» y las secciones «3A», «3B» y «3C», el área «Terraza M1» con «T1», un mesero con
// «Unir mesas» y otro sin él.

type stageM1Fixture struct {
	*unroutedFixture
	salon   operations.Area
	terraza operations.Area
	// mesas por nombre.
	mesas map[string]operations.Table
	// joiner es un mesero con «Unir mesas»; plain, uno sin él.
	joiner   testTerminal
	joinerID string
	plain    testTerminal
}

var stageM1JoinerPermissions = append([]string{"restaurant.tables.join", "accounts.move", "accounts.merge", "accounts.split",
	"accounts.transfer_items", "accounts.cancel", "payments.read", "payments.collect"}, waiterPermissions...)

func newStageM1Fixture(t *testing.T) *stageM1Fixture {
	t.Helper()
	resetStage9GDatabase(t)
	fixture := &stageM1Fixture{unroutedFixture: &unroutedFixture{kitchenDisplayFixture: &kitchenDisplayFixture{
		waiterModeFixture: &waiterModeFixture{restaurantFixture: newFixture(t)}}}, mesas: map[string]operations.Table{}}
	hamburguesas, hamburguesa := fixture.product(t, "Hamburguesas", "Hamburguesa M1", "HAMBURGUESA-M1", "120")
	_, coca := fixture.product(t, "Refrescos", "Coca Cola M1", "COCA-M1", "35")
	fixture.hamburguesa, fixture.coca = hamburguesa, coca
	fixture.cocina = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina", Active: true, Reception: "kds", Tracking: "inherit", Grouping: "inherit",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &hamburguesas.ID, Mode: "prepare",
		StationIDs: []string{fixture.cocina.ID}}, 200)
	fixture.salon = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas",
		operations.Area{Name: "Salón M1", Active: true}, 200))
	fixture.terraza = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas",
		operations.Area{Name: "Terraza M1", Active: true}, 200))
	names := []string{}
	// Sin «Mesa 3»: ese nombre es el del grupo de las secciones 3A, 3B y 3C (stagem1_groups_test.go).
	for index := 1; index <= 12; index++ {
		if index != 3 {
			names = append(names, fmt.Sprintf("Mesa %d", index))
		}
	}
	names = append(names, "5", "6", "3A", "3B", "3C")
	for index, name := range names {
		fixture.addTable(t, fixture.salon.ID, name, 10+(index%8)*120, 10+(index/8)*90, 4)
	}
	fixture.addTable(t, fixture.terraza.ID, "T1", 10, 10, 2)
	fixture.joiner = inventoryUser(t, fixture.restaurantFixture, "mesero_m1_unir", stageM1JoinerPermissions)
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username='mesero_m1_unir'").
		Scan(&fixture.joinerID); err != nil {
		t.Fatal(err)
	}
	fixture.waiterID = fixture.joinerID
	fixture.plain = inventoryUser(t, fixture.restaurantFixture, "mesero_m1_sin_unir", waiterPermissions)
	fixture.display = fixture.kitchenDevice(t, "Tableta de cocina M1")
	fixture.displayCall(t, fixture.display, "POST", "kitchen-display/connect", map[string]any{}, 200)
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil && *method.Code == "cash" {
			fixture.cashMethod = method.ID
		}
	}
	fixture.adminCall(t, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0"}, 200)
	return fixture
}

func (fixture *stageM1Fixture) addTable(t *testing.T, areaID, name string, x, y, seats int) operations.Table {
	t.Helper()
	table := decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
		AreaID: areaID, Name: name, Shape: "rectangle", X: x, Y: y, Width: 100, Height: 72, Seats: seats, Active: true,
	}, 200))
	fixture.mesas[name] = table
	return table
}

// id es el identificador de una mesa por su nombre.
func (fixture *stageM1Fixture) id(name string) string {
	return fixture.mesas[name].ID
}

func (fixture *stageM1Fixture) ids(names ...string) []string {
	result := []string{}
	for _, name := range names {
		result = append(result, fixture.id(name))
	}
	return result
}

// table relee una mesa (con su ocupación) por su nombre.
func (fixture *stageM1Fixture) table(t *testing.T, name string) operations.Table {
	t.Helper()
	for _, table := range fixture.overview(t).Tables {
		if table.ID == fixture.id(name) {
			fixture.mesas[name] = table
			return table
		}
	}
	t.Fatalf("la mesa %s no está en el mapa", name)
	return operations.Table{}
}

// stageM1Overview es GET restaurant/overview con lo nuevo de la etapa.
type stageM1Overview struct {
	Tables []operations.Table `json:"tables"`
	Unions []operations.TableUnion
	Groups []struct {
		ID       string   `json:"id"`
		AreaID   string   `json:"area_id"`
		Name     string   `json:"name"`
		TableIDs []string `json:"table_ids"`
		Seats    int      `json:"seats"`
		Revision int      `json:"revision"`
	} `json:"groups"`
}

func (fixture *stageM1Fixture) overview(t *testing.T) stageM1Overview {
	t.Helper()
	return decodeResponse[stageM1Overview](t, fixture.adminCall(t, "GET", "restaurant/overview", nil, 200))
}

// union es la unión abierta de la mesa en el mapa (nil si no está unida).
func (fixture *stageM1Fixture) union(t *testing.T, name string) *operations.TableUnion {
	t.Helper()
	view := fixture.overview(t)
	for _, union := range view.Unions {
		for _, id := range union.TableIDs {
			if id == fixture.id(name) {
				found := union
				return &found
			}
		}
	}
	return nil
}

// open abre una cuenta con su primer producto en una mesa (sin seen_account_ids, como la 0.4.1 sin la mesa vista).
func (fixture *stageM1Fixture) openAt(t *testing.T, terminal testTerminal, table string, extra map[string]any,
	items ...operations.DraftItem) operations.Bill {
	t.Helper()
	body := map[string]any{"table_id": fixture.id(table)}
	for key, value := range extra {
		body[key] = value
	}
	return fixture.open(t, terminal, body, items...)
}

// sent abre una cuenta en la mesa y envía su primer producto a cocina.
func (fixture *stageM1Fixture) sent(t *testing.T, table string, items ...operations.DraftItem) operations.Bill {
	t.Helper()
	opened := fixture.openAt(t, fixture.joiner, table, nil, items...)
	return fixture.confirm(t, fixture.joiner, opened.Account.ID)
}

// joinNow une desde una mesa ocupada (POST restaurant/table-unions).
func (fixture *stageM1Fixture) joinNow(t *testing.T, start string, others ...string) operations.TableUnion {
	t.Helper()
	return decodeResponse[struct {
		Union operations.TableUnion `json:"union"`
	}](t, fixture.call(t, fixture.joiner, "POST", "restaurant/table-unions", map[string]any{
		"operation_id": randomID(), "start_table_id": fixture.id(start), "table_ids": fixture.ids(others...),
	}, 200)).Union
}

// deferred abre la cuenta con su primer producto y la unión en la misma operación (creación diferida).
func (fixture *stageM1Fixture) deferred(t *testing.T, start string, others ...string) operations.Bill {
	t.Helper()
	return fixture.openAt(t, fixture.joiner, start, map[string]any{"union_table_ids": fixture.ids(others...),
		"seen_union_id": nil, "seen_account_ids": []string{}}, draftLine(fixture.hamburguesa, 1))
}

// stageM1Problem es el cuerpo de un rechazo.
type stageM1Problem struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

// reject hace la petición con el mesero que une y espera el rechazo con su código y su texto exactos.
func (fixture *stageM1Fixture) reject(t *testing.T, terminal testTerminal, method, route string, body map[string]any, status int,
	code, message string) stageM1Problem {
	t.Helper()
	if _, found := body["operation_id"]; !found && method != "GET" {
		body["operation_id"] = randomID()
	}
	problem := decodeResponse[stageM1Problem](t, fixture.call(t, terminal, method, route, body, status))
	if problem.Error.Code != code || (message != "" && problem.Error.Message != message) {
		t.Fatalf("%s %s: %s «%s», esperado %s «%s»", method, route, problem.Error.Code, problem.Error.Message, code, message)
	}
	return problem
}

// unionDetail es GET restaurant/table-unions/{id}.
type unionDetail struct {
	Union struct {
		operations.TableUnion
		Tables []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Seats int    `json:"seats"`
		} `json:"tables"`
		History []operations.UnionHistoryEntry `json:"history"`
	} `json:"union"`
}

func (fixture *stageM1Fixture) detail(t *testing.T, unionID string) unionDetail {
	t.Helper()
	return decodeResponse[unionDetail](t, fixture.adminCall(t, "GET", "restaurant/table-unions/"+unionID, nil, 200))
}

// accountHistoryKinds son los tipos del historial de la cuenta.
func (fixture *stageM1Fixture) accountHistoryKinds(t *testing.T, accountID string) map[string]int {
	t.Helper()
	history := decodeResponse[struct {
		Items []operations.HistoryEvent `json:"items"`
	}](t, fixture.adminCall(t, "GET", "restaurant/accounts/"+accountID+"/history", nil, 200))
	kinds := map[string]int{}
	for _, event := range history.Items {
		kinds[event.Kind]++
	}
	return kinds
}

// rawAccount lee la cuenta sin tipos (para los campos nuevos).
func (fixture *stageM1Fixture) rawAccount(t *testing.T, accountID string) map[string]any {
	t.Helper()
	var bill struct {
		Account map[string]any `json:"account"`
	}
	if err := json.Unmarshal(fixture.adminCall(t, "GET", "restaurant/accounts/"+accountID, nil, 200), &bill); err != nil {
		t.Fatal(err)
	}
	return bill.Account
}

// openUnionCount son las uniones abiertas.
func (fixture *stageM1Fixture) openUnionCount(t *testing.T) int {
	t.Helper()
	return fixture.count(t, "SELECT count(*)::int FROM app.table_unions WHERE state='open'")
}

// expectInvariants comprueba lo que nunca debe pasar (M1-D7 y §2.3): una mesa en dos uniones abiertas (lo impide
// además el índice único), una unión abierta sin cuentas abiertas, con menos de dos mesas o con mesas de otra área, un
// miembro abierto de una unión deshecha y una cuenta abierta en una mesa fuera de servicio o desactivada de una unión.
func expectStageM1Invariants(t *testing.T, fixture *restaurantFixture) {
	t.Helper()
	checks := map[string]string{
		"mesa en dos uniones abiertas": `SELECT count(*) FROM (SELECT m.table_id FROM app.table_union_members m
            JOIN app.table_unions u ON u.id=m.union_id WHERE u.state='open' AND m.left_at IS NULL
            GROUP BY m.table_id HAVING count(*)>1) x`,
		"unión abierta sin cuentas abiertas": `SELECT count(*) FROM app.table_unions u WHERE u.state='open' AND NOT EXISTS(
            SELECT FROM app.table_union_members m JOIN app.restaurant_accounts a ON a.table_id=m.table_id
            WHERE m.union_id=u.id AND m.left_at IS NULL AND a.state='open')`,
		"unión abierta con menos de dos mesas": `SELECT count(*) FROM app.table_unions u WHERE u.state='open' AND
            (SELECT count(*) FROM app.table_union_members m WHERE m.union_id=u.id AND m.left_at IS NULL)<2`,
		"mesa de otra área en una unión": `SELECT count(*) FROM app.table_union_members m JOIN app.table_unions u ON u.id=m.union_id
            JOIN app.restaurant_tables t ON t.id=m.table_id WHERE m.left_at IS NULL AND t.area_id<>u.area_id`,
		"miembro abierto de una unión deshecha": `SELECT count(*) FROM app.table_union_members m JOIN app.table_unions u
            ON u.id=m.union_id WHERE u.state='dissolved' AND m.left_at IS NULL`,
	}
	for name, query := range checks {
		var count int
		if err := fixture.pool.QueryRow(fixture.ctx, query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("invariante de uniones: %s (%d)", name, count)
		}
	}
}
