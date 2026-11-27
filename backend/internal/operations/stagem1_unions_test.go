//go:build integration

package operations_test

import (
	"encoding/json"
	"slices"
	"testing"

	"local/ad-el-pos/server/internal/operations"
)

// Etapa M1 (Fase A, §7 «Uniones (servidor)»): unir libres (diferida), crecer una ocupada, los rechazos con su código y
// su texto exactos, agregar y quitar (también la mesa con las cuentas), deshacerse sola en cada camino, la cuenta
// reabierta por una devolución, la creación diferida que encuentra una mesa ocupada, el permiso (también con PIN), la
// idempotencia y seen_union_id. Las invariantes de M1-D7 se comprueban después de cada caso. Usa su propia base desde
// cero.
func TestStageM1TableUnions(t *testing.T) {
	fixture := newStageM1Fixture(t)
	t.Cleanup(func() { expectStageM1Invariants(t, fixture.restaurantFixture) })
	cases := []struct {
		name string
		run  func(*testing.T, *stageM1Fixture)
	}{
		{"unir libres es diferido y nace con el primer producto", testStageM1DeferredJoin},
		{"crecer una mesa ocupada", testStageM1GrowOccupied},
		{"rechazos", testStageM1Rejections},
		{"agregar y quitar mesas", testStageM1AddRemove},
		{"se deshace sola en cada camino", testStageM1Dissolves},
		{"la cuenta reabierta por una devolución vuelve a su mesa", testStageM1Reopened},
		{"la creación diferida encuentra una mesa ocupada", testStageM1DeferredRace},
		{"el permiso Unir mesas", testStageM1Permission},
		{"idempotencia", testStageM1Idempotency},
		{"seen_union_id", testStageM1SeenUnion},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			item.run(t, fixture)
			expectStageM1Invariants(t, fixture.restaurantFixture)
		})
	}
}

func testStageM1DeferredJoin(t *testing.T, fixture *stageM1Fixture) {
	before := fixture.count(t, "SELECT count(*)::int FROM app.table_unions")
	// Con todas libres no se guarda nada: la unión vive en la pantalla hasta el primer producto.
	fixture.reject(t, fixture.joiner, "POST", "restaurant/table-unions", map[string]any{
		"start_table_id": fixture.id("Mesa 5"), "table_ids": fixture.ids("Mesa 6"),
	}, 409, "TABLE_UNION_DEFERRED", "La unión se guarda con el primer producto.")
	preview := decodeResponse[struct {
		Name     string                    `json:"name"`
		Seats    int                       `json:"seats"`
		TableIDs []string                  `json:"table_ids"`
		Problems []operations.UnionProblem `json:"problems"`
	}](t, fixture.call(t, fixture.joiner, "POST", "restaurant/table-unions/preview", map[string]any{
		"table_ids": fixture.ids("Mesa 6", "Mesa 5")}, 200))
	if preview.Name != "Mesa 5 + 6" || preview.Seats != 8 || !slices.Equal(preview.TableIDs, fixture.ids("Mesa 5", "Mesa 6")) ||
		len(preview.Problems) != 0 {
		t.Fatalf("vista previa: %+v", preview)
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.table_unions") != before {
		t.Fatal("la vista previa o el rechazo diferido guardaron una unión")
	}
	bill := fixture.deferred(t, "Mesa 5", "Mesa 6")
	account := bill.Account
	if account.Name != "Mesa 5 + 6" || account.TableUnion == nil || account.TableUnion.Name != "Mesa 5 + 6" ||
		!slices.Equal(account.TableUnion.TableIDs, fixture.ids("Mesa 5", "Mesa 6")) || *account.TableID != fixture.id("Mesa 5") ||
		*account.TableName != "Mesa 5" {
		t.Fatalf("la cuenta de la unión: %+v %+v", account, account.TableUnion)
	}
	union := fixture.union(t, "Mesa 6")
	if union == nil || union.Name != "Mesa 5 + 6" || union.Seats != 8 || union.AccountCount != 1 || union.State != "occupied" ||
		union.AreaID != fixture.salon.ID || union.GroupID != nil || union.Revision != 1 {
		t.Fatalf("la unión en el mapa: %+v", union)
	}
	// M1-D10: las dos mesas se leen ocupadas con lo de la unión.
	for _, name := range []string{"Mesa 5", "Mesa 6"} {
		table := fixture.table(t, name)
		if table.State != "occupied" || table.AccountCount != 1 || table.UnionID == nil || *table.UnionID != union.ID {
			t.Fatalf("%s en el mapa: %+v", name, table)
		}
	}
	// listAccounts de cualquier mesa de la unión devuelve las cuentas de toda la unión.
	listed := decodeResponse[struct {
		Items []operations.Account `json:"items"`
	}](t, fixture.adminCall(t, "GET", "restaurant/accounts?table_id="+fixture.id("Mesa 6"), nil, 200))
	if len(listed.Items) != 1 || listed.Items[0].ID != account.ID {
		t.Fatalf("listAccounts de la Mesa 6: %+v", listed.Items)
	}
	detail := fixture.detail(t, union.ID)
	if len(detail.Union.Tables) != 2 || len(detail.Union.History) != 2 || detail.Union.History[0].Kind != "joined" ||
		detail.Union.History[0].UserName == "" || detail.Union.History[0].DeviceName == "" {
		t.Fatalf("historial de la unión: %+v", detail.Union)
	}
	if kinds := fixture.accountHistoryKinds(t, account.ID); kinds["account.table_union"] != 1 {
		t.Fatalf("historial de la cuenta: %v", kinds)
	}
	// restaurant/state también trae la unión.
	state := decodeResponse[struct {
		Unions []operations.TableUnion `json:"unions"`
	}](t, fixture.adminCall(t, "GET", "restaurant/state", nil, 200))
	if !slices.ContainsFunc(state.Unions, func(item operations.TableUnion) bool { return item.ID == union.ID }) {
		t.Fatalf("restaurant/state sin la unión: %+v", state.Unions)
	}
	fixture.confirm(t, fixture.joiner, account.ID)
	fixture.pay(t, account.ID)
	if fixture.union(t, "Mesa 5") != nil {
		t.Fatal("la unión no se deshizo al cobrar")
	}
}

func testStageM1GrowOccupied(t *testing.T, fixture *stageM1Fixture) {
	opened := fixture.sent(t, "Mesa 1", draftLine(fixture.hamburguesa, 1))
	if opened.Account.Name != "Mesa 1" {
		t.Fatalf("la cuenta sin nombre: %s", opened.Account.Name)
	}
	union := fixture.joinNow(t, "Mesa 1", "Mesa 2")
	if union.Name != "Mesa 1 + 2" || union.AccountCount != 1 || union.State != "occupied" {
		t.Fatalf("crecer la Mesa 1: %+v", union)
	}
	account := fixture.refresh(t, opened.Account.ID).Account
	if account.Name != "Mesa 1 + 2" || account.TableUnion == nil || account.TableUnion.ID != union.ID ||
		account.Revision != opened.Account.Revision {
		t.Fatalf("la cuenta que creció: %+v", account)
	}
	// El KDS titula la tarjeta con la unión y no repite el nombre de la cuenta.
	found := card(t, fixture.orders(t, fixture.display), account.ID)
	if found == nil || found.Title != "Mesa 1 + 2" || found.Subtitle != "" {
		t.Fatalf("tarjeta del KDS: %+v", found)
	}
	fixture.pay(t, account.ID)
}

func testStageM1Rejections(t *testing.T, fixture *stageM1Fixture) {
	seven := fixture.sent(t, "Mesa 7", draftLine(fixture.coca, 1))
	eight := fixture.sent(t, "Mesa 8", draftLine(fixture.coca, 1))
	route := "restaurant/table-unions"
	fixture.reject(t, fixture.joiner, "POST", route, map[string]any{"start_table_id": fixture.id("Mesa 7"),
		"table_ids": fixture.ids("Mesa 8")}, 409, "TABLE_UNION_BOTH_OCCUPIED",
		"La Mesa 7 y la Mesa 8 tienen cuentas abiertas. Para juntar sus cuentas usa «Combinar cuentas».")
	// Empezar desde una libre y agregar una ocupada (M1-D6).
	fixture.reject(t, fixture.joiner, "POST", route, map[string]any{"start_table_id": fixture.id("Mesa 9"),
		"table_ids": fixture.ids("Mesa 8")}, 409, "TABLE_UNION_OCCUPIED",
		"Solo se pueden unir mesas libres: la Mesa 8 ya está ocupada.")
	fixture.reject(t, fixture.joiner, "POST", route, map[string]any{"start_table_id": fixture.id("Mesa 7"),
		"table_ids": fixture.ids("T1")}, 409, "TABLE_UNION_AREA", "Las mesas deben ser de la misma área.")
	// Fuera de servicio y desactivada.
	ten := fixture.table(t, "Mesa 10")
	fixture.mutate(t, "POST", "restaurant/tables/"+ten.ID+"/service", map[string]any{"revision": ten.Revision, "out_of_service": true}, 200)
	fixture.reject(t, fixture.joiner, "POST", route, map[string]any{"start_table_id": fixture.id("Mesa 7"),
		"table_ids": fixture.ids("Mesa 10")}, 409, "TABLE_UNION_UNAVAILABLE", "La Mesa 10 está fuera de servicio o desactivada.")
	eleven := fixture.table(t, "Mesa 11")
	eleven.Active = false
	fixture.mutate(t, "PATCH", "restaurant/tables/"+eleven.ID, eleven, 200)
	fixture.reject(t, fixture.joiner, "POST", route, map[string]any{"start_table_id": fixture.id("Mesa 7"),
		"table_ids": fixture.ids("Mesa 11")}, 409, "TABLE_UNION_UNAVAILABLE", "La Mesa 11 está fuera de servicio o desactivada.")
	// Ya unida: con la unión y sus cuentas en los detalles.
	union := fixture.joinNow(t, "Mesa 7", "Mesa 12")
	problem := fixture.reject(t, fixture.joiner, "POST", route, map[string]any{"start_table_id": fixture.id("Mesa 8"),
		"table_ids": fixture.ids("Mesa 12")}, 409, "TABLE_IN_UNION", "La Mesa 12 ya está unida con otras mesas (Mesa 7 + 12).")
	accounts, _ := problem.Error.Details["accounts"].([]any)
	unionDetails, _ := problem.Error.Details["union"].(map[string]any)
	if problem.Error.Details["table_id"] != fixture.id("Mesa 12") || problem.Error.Details["union_id"] != union.ID ||
		len(accounts) != 1 || accounts[0] != seven.Account.ID || unionDetails["name"] != "Mesa 7 + 12" {
		t.Fatalf("detalles de TABLE_IN_UNION: %+v", problem.Error.Details)
	}
	// La vista previa da los mismos problemas sin escribir.
	preview := decodeResponse[struct {
		Problems []operations.UnionProblem `json:"problems"`
	}](t, fixture.call(t, fixture.joiner, "POST", route+"/preview", map[string]any{
		"table_ids": fixture.ids("Mesa 8", "Mesa 12", "Mesa 10", "T1")}, 200))
	codes := []string{}
	for _, item := range preview.Problems {
		codes = append(codes, item.Code)
	}
	if !slices.Equal(codes, []string{"TABLE_UNION_UNAVAILABLE", "TABLE_IN_UNION", "TABLE_UNION_AREA"}) {
		t.Fatalf("problemas de la vista previa: %+v", preview.Problems)
	}
	// Formas inválidas: 400.
	fixture.reject(t, fixture.joiner, "POST", route, map[string]any{"start_table_id": fixture.id("Mesa 8"),
		"table_ids": []string{}}, 400, "OPERATION_VALIDATION", "")
	fixture.reject(t, fixture.joiner, "POST", route, map[string]any{"start_table_id": fixture.id("Mesa 8"),
		"table_ids": fixture.ids("Mesa 9", "Mesa 9")}, 400, "OPERATION_VALIDATION", "")
	fixture.reject(t, fixture.joiner, "POST", route, map[string]any{"start_table_id": "no-es-uuid",
		"table_ids": fixture.ids("Mesa 9")}, 400, "OPERATION_VALIDATION", "")
	// Una mesa unida no cambia de área (M1-D10) ni se pone fuera de servicio (tiene las cuentas de la unión).
	twelve := fixture.table(t, "Mesa 12")
	moved := twelve
	moved.AreaID = fixture.terraza.ID
	fixture.reject(t, fixture.restaurantFixture.admin, "PATCH", "restaurant/tables/"+twelve.ID, structMap(t, moved), 409,
		"TABLE_IN_UNION", "La Mesa 12 ya está unida con otras mesas (Mesa 7 + 12).")
	fixture.reject(t, fixture.restaurantFixture.admin, "POST", "restaurant/tables/"+twelve.ID+"/service",
		map[string]any{"revision": twelve.Revision, "out_of_service": true}, 409, "TABLE_HAS_OPEN_ACCOUNTS", "")
	// Limpieza: las mesas vuelven a servicio y las cuentas se cobran.
	ten = fixture.table(t, "Mesa 10")
	fixture.mutate(t, "POST", "restaurant/tables/"+ten.ID+"/service", map[string]any{"revision": ten.Revision, "out_of_service": false}, 200)
	eleven = fixture.table(t, "Mesa 11")
	eleven.Active = true
	fixture.mutate(t, "PATCH", "restaurant/tables/"+eleven.ID, eleven, 200)
	fixture.pay(t, seven.Account.ID)
	fixture.pay(t, eight.Account.ID)
}

func testStageM1AddRemove(t *testing.T, fixture *stageM1Fixture) {
	opened := fixture.sent(t, "Mesa 1", draftLine(fixture.hamburguesa, 1))
	union := fixture.joinNow(t, "Mesa 1", "Mesa 2")
	route := "restaurant/table-unions/" + union.ID
	added := decodeResponse[struct {
		Union operations.TableUnion `json:"union"`
	}](t, fixture.call(t, fixture.joiner, "POST", route+"/tables", map[string]any{"operation_id": randomID(),
		"revision": union.Revision, "table_id": fixture.id("Mesa 4")}, 200)).Union
	if added.Name != "Mesa 1 + 2 + 4" || added.Seats != 12 || added.Revision != union.Revision+1 || len(added.TableIDs) != 3 {
		t.Fatalf("agregar la Mesa 4: %+v", added)
	}
	if name := fixture.refresh(t, opened.Account.ID).Account.Name; name != "Mesa 1 + 2 + 4" {
		t.Fatalf("la cuenta no tomó el nombre nuevo: %s", name)
	}
	// Con la revisión anterior: otra terminal ya la cambió.
	fixture.reject(t, fixture.joiner, "POST", route+"/tables", map[string]any{"revision": union.Revision,
		"table_id": fixture.id("Mesa 9")}, 409, "TABLE_UNION_CONFLICT",
		"La unión cambió en otra terminal. Revisa las mesas e inténtalo otra vez.")
	// Agregar una ocupada.
	nine := fixture.sent(t, "Mesa 9", draftLine(fixture.coca, 1))
	fixture.reject(t, fixture.joiner, "POST", route+"/tables", map[string]any{"revision": added.Revision,
		"table_id": fixture.id("Mesa 9")}, 409, "TABLE_UNION_OCCUPIED", "Solo se pueden unir mesas libres: la Mesa 9 ya está ocupada.")
	// Quitar una mesa libre no mueve cuentas.
	removed := decodeResponse[struct {
		Union     operations.TableUnion `json:"union"`
		Dissolved bool                  `json:"dissolved"`
	}](t, fixture.call(t, fixture.joiner, "POST", route+"/tables/"+fixture.id("Mesa 2")+"/remove",
		map[string]any{"operation_id": randomID(), "revision": added.Revision}, 200))
	account := fixture.refresh(t, opened.Account.ID).Account
	if removed.Dissolved || removed.Union.Name != "Mesa 1 + 4" || removed.Union.State != "occupied" ||
		*account.TableID != fixture.id("Mesa 1") || account.Name != "Mesa 1 + 4" {
		t.Fatalf("quitar la Mesa 2: %+v %s %s", removed, *account.TableID, account.Name)
	}
	if table := fixture.table(t, "Mesa 2"); table.State != "free" || table.UnionID != nil {
		t.Fatalf("la Mesa 2 no quedó libre: %+v", table)
	}
	// Quitar la mesa con las cuentas: pasan a la que queda; con una sola mesa, la unión se deshace.
	last := decodeResponse[struct {
		Union     operations.TableUnion `json:"union"`
		Dissolved bool                  `json:"dissolved"`
	}](t, fixture.call(t, fixture.joiner, "POST", route+"/tables/"+fixture.id("Mesa 1")+"/remove",
		map[string]any{"operation_id": randomID(), "revision": removed.Union.Revision}, 200))
	account = fixture.refresh(t, opened.Account.ID).Account
	if !last.Dissolved || last.Union.State != "dissolved" || *account.TableID != fixture.id("Mesa 4") ||
		account.Name != "Mesa 4" || account.TableUnion != nil || account.State != "open" {
		t.Fatalf("quitar la Mesa 1: %+v; cuenta %s %s %+v", last, *account.TableID, account.Name, account.TableUnion)
	}
	if table := fixture.table(t, "Mesa 1"); table.State != "free" {
		t.Fatalf("la Mesa 1 no quedó libre: %+v", table)
	}
	if table := fixture.table(t, "Mesa 4"); table.State != "occupied" || table.UnionID != nil {
		t.Fatalf("la Mesa 4 con la cuenta: %+v", table)
	}
	detail := fixture.detail(t, union.ID)
	kinds := []string{}
	for _, entry := range detail.Union.History {
		kinds = append(kinds, entry.Kind)
	}
	if !slices.Equal(kinds, []string{"joined", "joined", "table_added", "table_removed", "table_removed", "dissolved"}) ||
		detail.Union.State != "dissolved" {
		t.Fatalf("historial de la unión: %v %s", kinds, detail.Union.State)
	}
	// Una unión deshecha ya no cambia.
	fixture.reject(t, fixture.joiner, "POST", route+"/tables", map[string]any{"revision": last.Union.Revision,
		"table_id": fixture.id("Mesa 2")}, 409, "TABLE_UNION_NOT_OPEN", "Esta unión de mesas ya se deshizo.")
	if kinds := fixture.accountHistoryKinds(t, opened.Account.ID); kinds["account.table_union"] != 4 {
		t.Fatalf("historial de la cuenta (unir, agregar, quitar, deshacer): %v", kinds)
	}
	fixture.pay(t, account.ID)
	fixture.pay(t, nine.Account.ID)
}

func testStageM1Dissolves(t *testing.T, fixture *stageM1Fixture) {
	reasons := decodeResponse[[]operations.CancelReason](t, fixture.adminCall(t, "GET", "restaurant/cancel-reasons", nil, 200))
	cases := []struct {
		name      string
		dissolves bool
		act       func(t *testing.T, accountID string)
	}{
		{"cobro", true, func(t *testing.T, accountID string) { fixture.pay(t, accountID) }},
		{"cancelación", true, func(t *testing.T, accountID string) {
			fixture.mutate(t, "POST", "restaurant/accounts/"+accountID+"/cancel-account", map[string]any{
				"revision": fixture.refresh(t, accountID).Account.Revision, "reason_id": reasons[0].ID, "description": "Prueba M1",
			}, 200)
		}},
		{"mover a una mesa fuera de la unión", true, func(t *testing.T, accountID string) {
			moved := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts/"+accountID+"/move", map[string]any{
				"revision": fixture.refresh(t, accountID).Account.Revision, "table_id": fixture.id("Mesa 12"),
			}, 200))
			if moved.Account.Name != "Mesa 12" || moved.Account.TableUnion != nil {
				t.Fatalf("la cuenta movida conserva el nombre de la unión: %s", moved.Account.Name)
			}
			fixture.pay(t, accountID)
		}},
		{"combinar con una cuenta de otra mesa", true, func(t *testing.T, accountID string) {
			target := fixture.sent(t, "Mesa 11", draftLine(fixture.coca, 1))
			fixture.mutate(t, "POST", "restaurant/accounts/"+accountID+"/merge", map[string]any{
				"revision": fixture.refresh(t, accountID).Account.Revision, "target_account_id": target.Account.ID,
				"target_revision": fixture.refresh(t, target.Account.ID).Account.Revision,
			}, 200)
			fixture.pay(t, target.Account.ID)
		}},
		{"dividir deja la cuenta nueva en la unión", false, func(t *testing.T, accountID string) {
			bill := fixture.refresh(t, accountID)
			split := decodeResponse[struct {
				Target operations.Bill `json:"target"`
			}](t, fixture.mutate(t, "POST", "restaurant/accounts/"+accountID+"/split", map[string]any{
				"revision": bill.Account.Revision, "new_account": map[string]any{"name": "Parte de la unión", "mode": "dine_in"},
				"selections": []map[string]any{{"version_id": bill.Lines[0].VersionID, "quantity": 1}},
			}, 200))
			if split.Target.Account.TableUnion == nil {
				t.Fatal("la cuenta dividida salió de la unión")
			}
			fixture.pay(t, accountID)
			if fixture.union(t, "Mesa 5") == nil {
				t.Fatal("la unión se deshizo con otra cuenta abierta")
			}
			fixture.pay(t, split.Target.Account.ID)
		}},
		{"transferir a una cuenta fuera de la unión", false, func(t *testing.T, accountID string) {
			target := fixture.sent(t, "Mesa 11", draftLine(fixture.coca, 1))
			bill := fixture.refresh(t, accountID)
			fixture.mutate(t, "POST", "restaurant/accounts/"+accountID+"/transfer", map[string]any{
				"revision": bill.Account.Revision, "target_account_id": target.Account.ID,
				"target_revision": fixture.refresh(t, target.Account.ID).Account.Revision,
				"selections":      []map[string]any{{"version_id": bill.Lines[0].VersionID, "quantity": 1}},
			}, 200)
			if fixture.union(t, "Mesa 5") == nil {
				t.Fatal("la unión se deshizo con su cuenta abierta")
			}
			fixture.pay(t, accountID)
			fixture.pay(t, target.Account.ID)
		}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			bill := fixture.deferred(t, "Mesa 5", "Mesa 6")
			bill = fixture.confirm(t, fixture.joiner, bill.Account.ID)
			if item.name == "dividir deja la cuenta nueva en la unión" || item.name == "transferir a una cuenta fuera de la unión" {
				bill = fixture.save(t, fixture.joiner, bill.Account.ID, draftLine(fixture.coca, 2))
				bill = fixture.confirm(t, fixture.joiner, bill.Account.ID)
			}
			unionID := bill.Account.TableUnion.ID
			item.act(t, bill.Account.ID)
			if fixture.union(t, "Mesa 5") != nil || fixture.union(t, "Mesa 6") != nil {
				t.Fatal("la unión sigue abierta sin cuentas")
			}
			var reason string
			if err := fixture.pool.QueryRow(fixture.ctx, "SELECT dissolve_reason FROM app.table_unions WHERE id=$1", unionID).
				Scan(&reason); err != nil || reason != "no_accounts" {
				t.Fatalf("motivo al deshacerse: %q %v", reason, err)
			}
			expectStageM1Invariants(t, fixture.restaurantFixture)
		})
	}
	// Descartar y liberar una cuenta vacía (la unión que nace al unir una mesa ocupada por una cuenta sin productos).
	for _, action := range []string{"discard", "release"} {
		t.Run(action, func(t *testing.T) {
			items := []operations.DraftItem{}
			if action == "discard" {
				items = append(items, draftLine(fixture.coca, 1))
			}
			opened := fixture.openAt(t, fixture.joiner, "Mesa 5", nil, items...)
			fixture.joinNow(t, "Mesa 5", "Mesa 6")
			fixture.mutate(t, "POST", "restaurant/accounts/"+opened.Account.ID+"/"+action, map[string]any{
				"revision": fixture.refresh(t, opened.Account.ID).Account.Revision}, 200)
			if fixture.union(t, "Mesa 6") != nil {
				t.Fatalf("la unión no se deshizo al %s", action)
			}
		})
	}
}

func testStageM1Reopened(t *testing.T, fixture *stageM1Fixture) {
	bill := fixture.deferred(t, "Mesa 5", "Mesa 6")
	bill = fixture.confirm(t, fixture.joiner, bill.Account.ID)
	fixture.pay(t, bill.Account.ID)
	// La Mesa 5 (donde quedó la cuenta) se une a otra unión mientras tanto.
	other := fixture.sent(t, "Mesa 4", draftLine(fixture.coca, 1))
	union := fixture.joinNow(t, "Mesa 4", "Mesa 5")
	fixture.mutate(t, "POST", "payments/accounts/"+bill.Account.ID+"/refunds", map[string]any{"mode": "total",
		"method_id": fixture.cashMethod, "reason": "Devolución de prueba M1"}, 200)
	reopened := fixture.refresh(t, bill.Account.ID).Account
	if reopened.State != "open" || *reopened.TableID != fixture.id("Mesa 5") || reopened.TableUnion == nil ||
		reopened.TableUnion.ID != union.ID {
		t.Fatalf("la cuenta reabierta: %+v %+v", reopened, reopened.TableUnion)
	}
	if current := fixture.union(t, "Mesa 4"); current == nil || current.AccountCount != 2 {
		t.Fatalf("la unión con la cuenta reabierta: %+v", current)
	}
	fixture.pay(t, reopened.ID)
	fixture.pay(t, other.Account.ID)
}

func testStageM1DeferredRace(t *testing.T, fixture *stageM1Fixture) {
	accounts := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts")
	unions := fixture.count(t, "SELECT count(*)::int FROM app.table_unions")
	// Otra terminal ocupó la Mesa 6 mientras la unión vivía en la pantalla.
	other := fixture.sent(t, "Mesa 6", draftLine(fixture.coca, 1))
	fixture.reject(t, fixture.joiner, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in",
		"table_id": fixture.id("Mesa 5"), "union_table_ids": fixture.ids("Mesa 6"), "seen_union_id": nil,
		"items": []operations.DraftItem{draftLine(fixture.hamburguesa, 1)}}, 409, "TABLE_UNION_OCCUPIED",
		"Solo se pueden unir mesas libres: la Mesa 6 ya está ocupada.")
	if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts") != accounts+1 ||
		fixture.count(t, "SELECT count(*)::int FROM app.table_unions") != unions {
		t.Fatal("el rechazo dejó algo guardado")
	}
	fixture.pay(t, other.Account.ID)
}

func testStageM1Permission(t *testing.T, fixture *stageM1Fixture) {
	// Sin «Unir mesas»: la vista previa, unir y la unión con el primer producto responden 403 sin escribir.
	for _, check := range []struct {
		route string
		body  map[string]any
	}{
		{"restaurant/table-unions/preview", map[string]any{"table_ids": fixture.ids("Mesa 5", "Mesa 6")}},
		{"restaurant/table-unions", map[string]any{"start_table_id": fixture.id("Mesa 5"), "table_ids": fixture.ids("Mesa 6")}},
		{"restaurant/accounts", map[string]any{"mode": "dine_in", "table_id": fixture.id("Mesa 5"),
			"union_table_ids": fixture.ids("Mesa 6"), "items": []operations.DraftItem{draftLine(fixture.coca, 1)}}},
	} {
		body := check.body
		body["operation_id"] = randomID()
		code := errorCode(t, fixture.call(t, fixture.plain, "POST", check.route, body, 403))
		if code != "PERMISSION_DENIED" {
			t.Fatalf("%s sin el permiso: %s", check.route, code)
		}
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts WHERE table_id=$1 AND state='open'", fixture.id("Mesa 5")) != 0 {
		t.Fatal("sin el permiso se abrió una cuenta")
	}
	// La lectura no lo pide.
	fixture.call(t, fixture.plain, "GET", "restaurant/overview", nil, 200)
	// Con PIN en una tableta de meseros: el permiso está en la lista de mesero.
	fixture.adminCall(t, "PUT", "users/"+fixture.joinerID+"/pin", map[string]any{"pin": "4829", "short_name": "Uni",
		"badge_color": "#3591d4"}, 200)
	phone := fixture.waiterPhone(t)
	code, response := fixture.raw(phone, "", "POST", "auth/pin-login", map[string]string{"user_id": fixture.joinerID, "pin": "4829"})
	login := decodeResponse[struct {
		Session string `json:"session_token"`
	}](t, response)
	if code != 200 || login.Session == "" {
		t.Fatalf("entrada con PIN: %d %s", code, response)
	}
	tablet := phone
	tablet.sessionToken = login.Session
	bill := fixture.openAt(t, tablet, "Mesa 5", map[string]any{"union_table_ids": fixture.ids("Mesa 6")}, draftLine(fixture.coca, 1))
	if bill.Account.TableUnion == nil {
		t.Fatal("con PIN no se creó la unión")
	}
	bill = fixture.confirm(t, tablet, bill.Account.ID)
	fixture.pay(t, bill.Account.ID)
}

func testStageM1Idempotency(t *testing.T, fixture *stageM1Fixture) {
	opened := fixture.sent(t, "Mesa 1", draftLine(fixture.coca, 1))
	key := randomID()
	body := map[string]any{"operation_id": key, "start_table_id": fixture.id("Mesa 1"), "table_ids": fixture.ids("Mesa 2")}
	first := fixture.call(t, fixture.joiner, "POST", "restaurant/table-unions", body, 200)
	second := fixture.call(t, fixture.joiner, "POST", "restaurant/table-unions", body, 200)
	if canonical(t, first) != canonical(t, second) || fixture.openUnionCount(t) != 1 {
		t.Fatalf("la misma clave no devolvió la misma unión:\n%s\n%s", first, second)
	}
	other := map[string]any{"operation_id": key, "start_table_id": fixture.id("Mesa 1"), "table_ids": fixture.ids("Mesa 9")}
	if code := errorCode(t, fixture.call(t, fixture.joiner, "POST", "restaurant/table-unions", other, 409)); code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("misma clave con otro cuerpo: %s", code)
	}
	fixture.pay(t, opened.Account.ID)
}

func testStageM1SeenUnion(t *testing.T, fixture *stageM1Fixture) {
	bill := fixture.deferred(t, "Mesa 5", "Mesa 6")
	union := bill.Account.TableUnion
	// La terminal veía la Mesa 6 sin unir (seen_union_id null): TABLE_IN_UNION con la unión y sus cuentas.
	problem := fixture.reject(t, fixture.joiner, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in",
		"table_id": fixture.id("Mesa 6"), "seen_union_id": nil, "seen_account_ids": []string{},
		"items": []operations.DraftItem{draftLine(fixture.coca, 1)}}, 409, "TABLE_IN_UNION",
		"La Mesa 6 ya está unida con otras mesas (Mesa 5 + 6).")
	if problem.Error.Details["union_id"] != union.ID {
		t.Fatalf("detalles: %+v", problem.Error.Details)
	}
	// Con otra unión vista, igual.
	fixture.reject(t, fixture.joiner, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in",
		"table_id": fixture.id("Mesa 6"), "seen_union_id": randomID(), "seen_account_ids": []string{},
		"items": []operations.DraftItem{draftLine(fixture.coca, 1)}}, 409, "TABLE_IN_UNION", "")
	// Sin el campo (la 0.4.1): seen_account_ids contra toda la unión.
	fixture.reject(t, fixture.joiner, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in",
		"table_id": fixture.id("Mesa 6"), "seen_account_ids": []string{},
		"items": []operations.DraftItem{draftLine(fixture.coca, 1)}}, 409, "TABLE_HAS_ACCOUNT", "")
	// Con la unión vista y sus cuentas vistas: otra cuenta en la unión, con su nombre.
	second := fixture.openAt(t, fixture.joiner, "Mesa 6", map[string]any{"seen_union_id": union.ID,
		"seen_account_ids": []string{bill.Account.ID}}, draftLine(fixture.coca, 1))
	if second.Account.TableUnion == nil || second.Account.TableUnion.ID != union.ID || second.Account.Name != "Mesa 5 + 6" {
		t.Fatalf("la segunda cuenta de la unión: %+v", second.Account)
	}
	if current := fixture.union(t, "Mesa 5"); current == nil || current.AccountCount != 2 {
		t.Fatalf("la unión con dos cuentas: %+v", current)
	}
	// M1-D32: una mesa sin unir con seen_union_id de una unión que sigue abierta (la mesa salió de ella o nunca estuvo):
	// TABLE_UNION_CONFLICT, sin guardar nada, para que la cuenta no caiga en otro lugar del que veía el mesero.
	accounts := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts")
	conflicted := fixture.reject(t, fixture.joiner, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in",
		"table_id": fixture.id("Mesa 9"), "seen_union_id": union.ID, "seen_account_ids": []string{},
		"items": []operations.DraftItem{draftLine(fixture.coca, 1)}}, 409, "TABLE_UNION_CONFLICT",
		"La unión cambió en otra terminal. Revisa las mesas e inténtalo otra vez.")
	if conflicted.Error.Details["union_id"] != union.ID || conflicted.Error.Details["table_id"] != fixture.id("Mesa 9") ||
		fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts") != accounts {
		t.Fatalf("seen_union_id de una unión abierta sin la mesa: %+v", conflicted.Error.Details)
	}
	for _, id := range []string{bill.Account.ID, second.Account.ID} {
		fixture.confirm(t, fixture.joiner, id)
		fixture.pay(t, id)
	}
	// Con la unión vista ya deshecha y la mesa libre, la cuenta se abre como siempre, en la mesa sola.
	free := fixture.openAt(t, fixture.joiner, "Mesa 6", map[string]any{"seen_union_id": union.ID, "seen_account_ids": []string{}},
		draftLine(fixture.coca, 1))
	if free.Account.TableUnion != nil || free.Account.Name != "Mesa 6" {
		t.Fatalf("una mesa de una unión deshecha: %+v", free.Account)
	}
	fixture.confirm(t, fixture.joiner, free.Account.ID)
	fixture.pay(t, free.Account.ID)
}

// structMap convierte un valor en el mapa de su JSON (para reutilizar la mesa leída como cuerpo de un PATCH).
func structMap(t *testing.T, value any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return decodeResponse[map[string]any](t, encoded)
}
