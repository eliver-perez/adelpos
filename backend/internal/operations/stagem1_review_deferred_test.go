//go:build integration

package operations_test

import (
	"testing"

	"local/ad-el-pos/server/internal/operations"
)

// Etapa M1, revisor independiente (§2.3 «Una unión diferida…», M1-D9): una unión que vivía solo en la pantalla y que al
// guardarse (POST restaurant/accounts con union_table_ids o union_group_id) encuentra una mesa ocupada, unida, un
// producto agotado o una terminal sin «Unir mesas» no guarda nada: ni la cuenta, ni la unión con sus mesas y su
// historial, ni los renglones del borrador, ni eventos de la cuenta o de tiempo real, ni la clave de la operación, ni
// movimientos en el registro (salvo la denegación de un 403, que el registro conserva). La misma clave con otro cuerpo
// es IDEMPOTENCY_CONFLICT, y repetir la que entró después de que la unión cambió devuelve la primera respuesta sin
// escribir nada.
func TestStageM1ReviewDeferredLeavesNothing(t *testing.T) {
	fixture := newStageM1Fixture(t)
	t.Cleanup(func() { expectStageM1Invariants(t, fixture.restaurantFixture) })
	// Otra terminal con «Unir mesas».
	other := inventoryUser(t, fixture.restaurantFixture, "mesero_m1_revisor_dos", stageM1JoinerPermissions)
	group := decodeResponse[struct {
		Group struct {
			ID string `json:"id"`
		} `json:"group"`
	}](t, fixture.mutate(t, "POST", "restaurant/table-groups", map[string]any{"area_id": fixture.salon.ID, "name": "Mesa 3",
		"table_ids": fixture.ids("3A", "3B", "3C")}, 200)).Group
	counted := []string{"restaurant_accounts", "table_unions", "table_union_members", "table_union_events",
		"restaurant_draft_items", "restaurant_events", "realtime_events", "restaurant_operations", "kitchen_unit_releases"}
	snapshot := func(t *testing.T) map[string]int {
		t.Helper()
		counts := map[string]int{}
		for _, table := range counted {
			counts[table] = fixture.count(t, "SELECT count(*)::int FROM app."+table)
		}
		counts["audit_events (operación)"] = fixture.count(t, `SELECT count(*)::int FROM app.audit_events
            WHERE success AND (action LIKE 'restaurant.%' OR action LIKE 'payments.%')`)
		counts["audit_events (denegaciones)"] = fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE NOT success")
		return counts
	}
	deferredBody := func(start string, extra map[string]any) map[string]any {
		body := map[string]any{"operation_id": randomID(), "mode": "dine_in", "table_id": fixture.id(start),
			"seen_union_id": nil, "seen_account_ids": []string{},
			"items": []operations.DraftItem{draftLine(fixture.hamburguesa, 1), draftLine(fixture.coca, 2)}}
		for key, value := range extra {
			body[key] = value
		}
		return body
	}
	expectNothing := func(t *testing.T, before map[string]int, denied int) {
		t.Helper()
		after := snapshot(t)
		for key, value := range before {
			want := value
			if key == "audit_events (denegaciones)" {
				want += denied
			}
			if after[key] != want {
				t.Errorf("%s: %d antes, %d después (se esperaban %d)", key, value, after[key], want)
			}
		}
	}
	settle := func(t *testing.T, accounts ...operations.Bill) {
		t.Helper()
		for _, bill := range accounts {
			fixture.pay(t, bill.Account.ID)
		}
	}

	t.Run("una mesa se ocupó mientras tanto", func(t *testing.T) {
		busy := fixture.sent(t, "Mesa 2", draftLine(fixture.coca, 1))
		defer settle(t, busy)
		before := snapshot(t)
		fixture.reject(t, fixture.joiner, "POST", "restaurant/accounts", deferredBody("Mesa 1", map[string]any{
			"union_table_ids": fixture.ids("Mesa 2", "Mesa 4")}), 409, "TABLE_UNION_OCCUPIED",
			"Solo se pueden unir mesas libres: la Mesa 2 ya está ocupada.")
		expectNothing(t, before, 0)
	})

	t.Run("una mesa se unió mientras tanto", func(t *testing.T) {
		joined := fixture.deferred(t, "Mesa 4", "Mesa 5")
		defer func() {
			fixture.confirm(t, fixture.admin, joined.Account.ID)
			settle(t, joined)
		}()
		before := snapshot(t)
		problem := fixture.reject(t, fixture.joiner, "POST", "restaurant/accounts", deferredBody("Mesa 1", map[string]any{
			"union_table_ids": fixture.ids("Mesa 2", "Mesa 5")}), 409, "TABLE_IN_UNION",
			"La Mesa 5 ya está unida con otras mesas (Mesa 4 + 5).")
		if problem.Error.Details["union_id"] != joined.Account.TableUnion.ID {
			t.Fatalf("los detalles no traen la unión: %+v", problem.Error.Details)
		}
		expectNothing(t, before, 0)
	})

	t.Run("una sección del grupo se ocupó mientras tanto", func(t *testing.T) {
		section := fixture.sent(t, "3C", draftLine(fixture.coca, 1))
		defer settle(t, section)
		before := snapshot(t)
		fixture.reject(t, fixture.joiner, "POST", "restaurant/accounts", deferredBody("3A", map[string]any{
			"union_group_id": group.ID}), 409, "TABLE_GROUP_SECTION_OCCUPIED",
			"La sección 3C está ocupada. Únela con las secciones libres para crecer.")
		expectNothing(t, before, 0)
	})

	t.Run("un producto se agotó mientras tanto", func(t *testing.T) {
		fixture.adminCall(t, "POST", "catalog/products/"+fixture.hamburguesa.ID+"/sold-out",
			map[string]any{"operation_id": randomID(), "sold_out": true}, 200)
		defer fixture.adminCall(t, "POST", "catalog/products/"+fixture.hamburguesa.ID+"/sold-out",
			map[string]any{"operation_id": randomID(), "sold_out": false}, 200)
		before := snapshot(t)
		status, body := fixture.raw(fixture.joiner, "", "POST", "restaurant/accounts", deferredBody("Mesa 1", map[string]any{
			"union_table_ids": fixture.ids("Mesa 2")}))
		if status != 409 {
			t.Fatalf("producto agotado en la unión diferida: %d %s", status, body)
		}
		expectNothing(t, before, 0)
	})

	t.Run("sin el permiso Unir mesas", func(t *testing.T) {
		before := snapshot(t)
		status, body := fixture.raw(fixture.plain, "", "POST", "restaurant/accounts", deferredBody("Mesa 1", map[string]any{
			"union_table_ids": fixture.ids("Mesa 2")}))
		if status != 403 || errorCode(t, body) != "PERMISSION_DENIED" {
			t.Fatalf("sin «Unir mesas»: %d %s", status, body)
		}
		// Lo único que queda es la denegación en el registro.
		expectNothing(t, before, 1)
	})

	t.Run("la misma clave con otro cuerpo y la repetición después de que la unión cambió", func(t *testing.T) {
		body := deferredBody("Mesa 1", map[string]any{"union_table_ids": fixture.ids("Mesa 2")})
		status, first := fixture.raw(fixture.joiner, "", "POST", "restaurant/accounts", body)
		if status != 200 {
			t.Fatalf("la unión diferida: %d %s", status, first)
		}
		bill := decodeResponse[operations.Bill](t, first)
		defer func() {
			fixture.confirm(t, fixture.admin, bill.Account.ID)
			fixture.pay(t, bill.Account.ID)
		}()
		before := snapshot(t)
		changed := deferredBody("Mesa 1", map[string]any{"union_table_ids": fixture.ids("Mesa 4")})
		changed["operation_id"] = body["operation_id"]
		fixture.reject(t, fixture.joiner, "POST", "restaurant/accounts", changed, 409, "IDEMPOTENCY_CONFLICT", "")
		expectNothing(t, before, 0)
		// La unión cambia en otra terminal (se le agrega Mesa 4); la repetición devuelve la respuesta de entonces.
		union := fixture.union(t, "Mesa 1")
		fixture.call(t, other, "POST", "restaurant/table-unions/"+union.ID+"/tables", map[string]any{
			"operation_id": randomID(), "revision": union.Revision, "table_id": fixture.id("Mesa 4")}, 200)
		before = snapshot(t)
		status, replayed := fixture.raw(fixture.joiner, "", "POST", "restaurant/accounts", body)
		if status != 200 || !sameJSON(t, first, replayed) {
			t.Fatalf("la repetición: %d %s\nprimera: %s", status, replayed, first)
		}
		expectNothing(t, before, 0)
	})
}
