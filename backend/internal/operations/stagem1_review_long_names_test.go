//go:build integration

package operations_test

import (
	"fmt"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/tableunions"
)

// Etapa M1, revisor independiente (§1.1, §2.2 y M1-D14), con la corrección de M1-D29: el nombre de una unión no tiene el
// límite del nombre de una cuenta. Una unión de nueve mesas «Terraza 1» a «Terraza 9» (mesas sin la palabra «Mesa», así
// que el nombre lleva cada nombre completo) se llama «Terraza 1 + Terraza 2 + … + Terraza 9»: 105 caracteres.
// app.restaurant_accounts.name admite de 1 a 100 (0004) y validateAccountInput rechaza más de 100, así que una cuenta
// sin nombre propio en esa unión lleva la referencia recortada en el último « + » que cabe, terminada en « + …»
// («Terraza 1 + … + Terraza 8 + …»), mientras la unión conserva su nombre completo. Cada camino de abajo (unir, la
// unión diferida, agregar, abrir otra cuenta, mover, la cola sin conexión y renombrar una mesa unida) funciona y deja la
// referencia recortada; el KDS no la repite como subtítulo. Antes de la corrección todos respondían 400.
func TestStageM1ReviewLongUnionName(t *testing.T) {
	fixture := newStageM1Fixture(t)
	t.Cleanup(func() { expectStageM1Invariants(t, fixture.restaurantFixture) })
	area := decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas",
		operations.Area{Name: "Terraza R", Active: true}, 200))
	names := []string{}
	for index := 1; index <= 10; index++ {
		name := fmt.Sprintf("Terraza %d", index)
		names = append(names, name)
		fixture.addTable(t, area.ID, name, 10+((index-1)%8)*120, 10+((index-1)/8)*90, 4)
	}
	nine := names[:9]
	expected := strings.Join(nine, " + ")
	if len(expected) <= 100 {
		t.Fatalf("el nombre de la unión de prueba debe pasar de 100 caracteres: %q", expected)
	}
	reference := "Terraza 1 + Terraza 2 + Terraza 3 + Terraza 4 + Terraza 5 + Terraza 6 + Terraza 7 + Terraza 8 + …"
	if tableunions.AccountReference(expected) != reference {
		t.Fatalf("referencia recortada: %q", tableunions.AccountReference(expected))
	}
	expectOK := func(t *testing.T, what string, status int, body []byte) {
		t.Helper()
		if status != 200 {
			t.Fatalf("%s: se esperaba 200 y respondió %d %s", what, status, body)
		}
	}
	expectName := func(t *testing.T, what, accountID, want string) {
		t.Helper()
		if name := fixture.refresh(t, accountID).Account.Name; name != want {
			t.Errorf("%s: la cuenta se llama %q, se esperaba %q", what, name, want)
		}
	}
	expectUnion := func(t *testing.T, table, want string) {
		t.Helper()
		if union := fixture.union(t, table); union == nil || union.Name != want {
			t.Errorf("la unión de %s: %+v, se esperaba %q", table, union, want)
		}
	}
	settle := func(t *testing.T) {
		t.Helper()
		listed := decodeResponse[struct {
			Items []operations.Account `json:"items"`
		}](t, fixture.adminCall(t, "GET", "restaurant/accounts?state=open&page_size=100", nil, 200))
		for _, account := range listed.Items {
			if account.ConfirmedQuantity == 0 {
				fixture.mutate(t, "POST", "restaurant/accounts/"+account.ID+"/discard", map[string]any{"revision": account.Revision}, 200)
				continue
			}
			if account.DraftQuantity > 0 {
				fixture.confirm(t, fixture.admin, account.ID)
			}
			fixture.pay(t, account.ID)
		}
	}

	t.Run("crecer una mesa ocupada con una cuenta sin nombre propio", func(t *testing.T) {
		defer settle(t)
		bill := fixture.sent(t, "Terraza 1", draftLine(fixture.coca, 1))
		status, body := fixture.raw(fixture.joiner, "", "POST", "restaurant/table-unions", map[string]any{
			"operation_id": randomID(), "start_table_id": fixture.id("Terraza 1"), "table_ids": fixture.ids(nine[1:]...)})
		expectOK(t, "createTableUnion de nueve mesas", status, body)
		expectUnion(t, "Terraza 5", expected)
		expectName(t, "unir", bill.Account.ID, reference)
	})

	t.Run("creación diferida sin nombre", func(t *testing.T) {
		defer settle(t)
		status, body := fixture.raw(fixture.joiner, "", "POST", "restaurant/accounts", map[string]any{
			"operation_id": randomID(), "mode": "dine_in", "table_id": fixture.id("Terraza 1"),
			"union_table_ids": fixture.ids(nine[1:]...), "seen_union_id": nil, "seen_account_ids": []string{},
			"items": []operations.DraftItem{draftLine(fixture.coca, 1)}})
		expectOK(t, "openAccount con union_table_ids de nueve mesas", status, body)
		bill := decodeResponse[operations.Bill](t, body)
		if bill.Account.Name != reference || bill.Account.TableUnion == nil || bill.Account.TableUnion.Name != expected {
			t.Errorf("la cuenta de la unión diferida: %q en %+v", bill.Account.Name, bill.Account.TableUnion)
		}
	})

	t.Run("agregar la novena mesa a una unión de ocho", func(t *testing.T) {
		defer settle(t)
		bill := fixture.sent(t, "Terraza 1", draftLine(fixture.coca, 1))
		union := fixture.joinNow(t, "Terraza 1", names[1:8]...)
		expectName(t, "unión de ocho, aún sin recortar", bill.Account.ID, union.Name)
		status, body := fixture.raw(fixture.joiner, "", "POST", "restaurant/table-unions/"+union.ID+"/tables", map[string]any{
			"operation_id": randomID(), "revision": union.Revision, "table_id": fixture.id("Terraza 9")})
		expectOK(t, "addTableToUnion de la novena mesa", status, body)
		expectUnion(t, "Terraza 9", expected)
		expectName(t, "agregar", bill.Account.ID, reference)
		// Quitar la novena: la cuenta vuelve a llamarse como la unión de ocho, que sí cabe.
		current := fixture.union(t, "Terraza 1")
		fixture.call(t, fixture.joiner, "POST", "restaurant/table-unions/"+union.ID+"/tables/"+fixture.id("Terraza 9")+"/remove",
			map[string]any{"operation_id": randomID(), "revision": current.Revision}, 200)
		expectName(t, "quitar", bill.Account.ID, union.Name)
	})

	t.Run("en una unión larga que ya existe", func(t *testing.T) {
		defer settle(t)
		// Con una cuenta con nombre propio la unión de nueve se crea y su cuenta no toma el nombre de la unión.
		named := fixture.openAt(t, fixture.joiner, "Terraza 1", map[string]any{"name": "Familia López"},
			draftLine(fixture.coca, 1))
		fixture.confirm(t, fixture.joiner, named.Account.ID)
		union := fixture.joinNow(t, "Terraza 1", nine[1:]...)
		if union.Name != expected {
			t.Fatalf("la unión: %q", union.Name)
		}
		expectName(t, "cuenta con nombre propio", named.Account.ID, "Familia López")
		status, body := fixture.raw(fixture.joiner, "", "POST", "restaurant/accounts", map[string]any{
			"operation_id": randomID(), "mode": "dine_in", "table_id": fixture.id("Terraza 5"), "seen_union_id": union.ID,
			"seen_account_ids": []string{named.Account.ID}, "items": []operations.DraftItem{draftLine(fixture.coca, 1)}})
		expectOK(t, "abrir otra cuenta sin nombre en la unión", status, body)
		if opened := decodeResponse[operations.Bill](t, body); opened.Account.Name != reference {
			t.Errorf("abrir: %q", opened.Account.Name)
		}
		alone := fixture.sent(t, "Terraza 10", draftLine(fixture.coca, 1))
		status, body = fixture.raw(fixture.joiner, "", "POST", "restaurant/accounts/"+alone.Account.ID+"/move", map[string]any{
			"operation_id": randomID(), "revision": alone.Account.Revision, "table_id": fixture.id("Terraza 2")})
		expectOK(t, "mover una cuenta sin nombre a la unión", status, body)
		expectName(t, "mover", alone.Account.ID, reference)
		status, body = fixture.raw(fixture.joiner, "", "POST", "restaurant/offline-orders", offlineBody(randomID(),
			fixture.catalogVersion(t), map[string]any{"table_id": fixture.id("Terraza 3"), "seen_union_id": union.ID,
				"seen_account_ids": openAccountIDs(t, fixture)}, offlineItem(fixture.coca.ID, 1, "35.00")))
		expectOK(t, "pedido de la cola sin conexión a una mesa de la unión", status, body)
		if result := decodeResponse[operations.OfflineOrderResult](t, body); result.Bill.Account.Name != reference {
			t.Errorf("la cola: %q", result.Bill.Account.Name)
		}
	})

	t.Run("renombrar una mesa de una unión larga y el KDS", func(t *testing.T) {
		defer settle(t)
		bill := fixture.sent(t, "Terraza 1", draftLine(fixture.hamburguesa, 1))
		fixture.joinNow(t, "Terraza 1", nine[1:]...)
		expectName(t, "unir con la hamburguesa", bill.Account.ID, reference)
		// El KDS titula con la unión completa y no repite la referencia recortada.
		found := card(t, fixture.orders(t, fixture.display), bill.Account.ID)
		if found == nil || found.Title != "Mesa "+expected || found.Subtitle != "" {
			t.Errorf("tarjeta del KDS: %+v", found)
		}
		table := fixture.table(t, "Terraza 9")
		renamed := table
		renamed.Name = "Terraza 99"
		status, body := fixture.raw(fixture.admin, "", "PATCH", "restaurant/tables/"+table.ID, withOperation(structMap(t, renamed)))
		expectOK(t, "renombrar una mesa unida", status, body)
		longer := strings.Join(append(append([]string{}, nine[:8]...), "Terraza 99"), " + ")
		expectUnion(t, "Terraza 1", longer)
		expectName(t, "renombrar", bill.Account.ID, tableunions.AccountReference(longer))
		// La mesa vuelve a su nombre para los demás casos.
		table = fixture.table(t, "Terraza 9")
		table.Name = "Terraza 9"
		fixture.mutate(t, "PATCH", "restaurant/tables/"+table.ID, table, 200)
	})
}

// openAccountIDs son las cuentas abiertas (para mandarlas como vistas).
func openAccountIDs(t *testing.T, fixture *stageM1Fixture) []string {
	t.Helper()
	listed := decodeResponse[struct {
		Items []operations.Account `json:"items"`
	}](t, fixture.adminCall(t, "GET", "restaurant/accounts?state=open&page_size=100", nil, 200))
	ids := []string{}
	for _, account := range listed.Items {
		ids = append(ids, account.ID)
	}
	return ids
}
