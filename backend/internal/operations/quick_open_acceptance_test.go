//go:build integration

package operations_test

import (
	"encoding/json"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
)

// quickTable crea una mesa activa para las pruebas de apertura rápida.
func quickTable(t *testing.T, fixture *restaurantFixture, name string, x int) operations.Table {
	t.Helper()
	return decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
		AreaID: fixture.area.ID, Name: name, Shape: "rectangle", X: x, Y: 400, Width: 100, Height: 72, Seats: 2, Active: true,
	}, 200))
}

// openAccounts cuenta las cuentas abiertas de una mesa directamente en PostgreSQL.
func openAccounts(t *testing.T, fixture *restaurantFixture, tableID string) int {
	t.Helper()
	var count int
	err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM app.restaurant_accounts WHERE table_id=$1 AND state='open'", tableID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

// testQuickOpen cubre la etapa 6.2: abrir una cuenta sin formulario (nombre opcional con referencia
// neutra asignada por Go), editar sus datos vaciando el nombre, cambiar de mesa e idempotencia.
func testQuickOpen(t *testing.T, fixture *restaurantFixture) {
	first := quickTable(t, fixture, "Mesa 7", 20)
	second := quickTable(t, fixture, "Mesa 8", 160)
	var defaultList string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id FROM app.catalog_price_lists WHERE is_default AND active").Scan(&defaultList); err != nil {
		t.Fatal(err)
	}
	var adminID string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id FROM app.users WHERE username='admin'").Scan(&adminID); err != nil {
		t.Fatal(err)
	}

	t.Run("apertura rápida sin nombre en salón y para llevar", func(t *testing.T) {
		table := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "", "mode": "dine_in", "table_id": first.ID,
		}, 200))
		if table.Account.Name != "Mesa 7" || table.Account.TableID == nil || *table.Account.TableID != first.ID {
			t.Fatalf("La cuenta de mesa sin nombre debe tomar el nombre de la mesa: %q", table.Account.Name)
		}
		if table.Account.PriceListID != defaultList || table.Account.AssignedUserID != adminID {
			t.Fatal("La apertura rápida no usó la lista predeterminada y el responsable por omisión")
		}
		// Un cuerpo sin el campo nombre equivale a un nombre vacío.
		loose := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in"}, 200))
		if loose.Account.Name != "Cuenta" || loose.Account.TableID != nil {
			t.Fatalf("La cuenta sin mesa ni nombre debe llamarse «Cuenta»: %q", loose.Account.Name)
		}
		blank := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "   ", "mode": "takeaway", "origin": "Ventanilla",
		}, 200))
		if blank.Account.Name != "Pedido para llevar" || blank.Account.Origin != "Ventanilla" || blank.Account.CustomerName != "" {
			t.Fatalf("El pedido para llevar sin nombre debe llamarse «Pedido para llevar»: %q", blank.Account.Name)
		}
		// Un nombre capturado se conserva; las demás validaciones siguen iguales.
		named := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": " Familia López ", "mode": "dine_in", "table_id": first.ID,
		}, 200))
		if named.Account.Name != "Familia López" {
			t.Fatal("Se alteró un nombre capturado")
		}
		fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"mode": "takeaway", "schedule_kind": "delivery"}, 400)
		fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in", "table_id": randomID()}, 400)
		long := make([]byte, 101)
		for index := range long {
			long[index] = 'a'
		}
		fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": string(long), "mode": "dine_in"}, 400)
		if openAccounts(t, fixture, first.ID) != 2 {
			t.Fatal("Una apertura rechazada dejó una cuenta abierta")
		}
	})

	t.Run("editar datos vaciando el nombre restaura la referencia neutra", func(t *testing.T) {
		bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"mode": "dine_in", "table_id": second.ID,
		}, 200))
		body := accountUpdate(bill.Account)
		body["name"] = "Cumpleaños"
		body["notes"] = "Mesa junto a la ventana"
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PATCH", "restaurant/accounts/"+bill.Account.ID, body, 200))
		if bill.Account.Name != "Cumpleaños" || bill.Account.Notes != "Mesa junto a la ventana" {
			t.Fatal("No se guardaron los datos de la cuenta")
		}
		body = accountUpdate(bill.Account)
		body["name"] = " "
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PATCH", "restaurant/accounts/"+bill.Account.ID, body, 200))
		if bill.Account.Name != "Mesa 8" || bill.Account.Notes != "Mesa junto a la ventana" {
			t.Fatalf("Vaciar el nombre debe restaurar la referencia neutra: %q", bill.Account.Name)
		}
		takeaway := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "Mostrador 3", "mode": "takeaway", "origin": "Mostrador",
		}, 200))
		body = accountUpdate(takeaway.Account)
		body["name"] = ""
		body["customer_name"] = "Ana"
		body["phone"] = "5550009999"
		takeaway = decodeResponse[operations.Bill](t, fixture.mutate(t, "PATCH", "restaurant/accounts/"+takeaway.Account.ID, body, 200))
		if takeaway.Account.Name != "Pedido para llevar" || takeaway.Account.CustomerName != "Ana" || takeaway.Account.Phone != "5550009999" {
			t.Fatal("El pedido para llevar no conservó sus datos con el nombre vacío")
		}
	})

	t.Run("cambiar de mesa conserva un nombre capturado y actualiza la referencia neutra", func(t *testing.T) {
		unnamed := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"mode": "dine_in", "table_id": first.ID,
		}, 200))
		moved := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts/"+unnamed.Account.ID+"/move", map[string]any{
			"revision": unnamed.Account.Revision, "table_id": second.ID,
		}, 200))
		if moved.Account.Name != "Mesa 8" || *moved.Account.TableID != second.ID {
			t.Fatalf("La referencia neutra no siguió a la mesa nueva: %q", moved.Account.Name)
		}
		// Editar los datos con cambio de mesa sigue la misma regla.
		body := accountUpdate(moved.Account)
		body["table_id"] = first.ID
		edited := decodeResponse[operations.Bill](t, fixture.mutate(t, "PATCH", "restaurant/accounts/"+moved.Account.ID, body, 200))
		if edited.Account.Name != "Mesa 7" {
			t.Fatalf("La edición con cambio de mesa no actualizó la referencia neutra: %q", edited.Account.Name)
		}
		named := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "Mesa del fondo", "mode": "dine_in", "table_id": first.ID,
		}, 200))
		named = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts/"+named.Account.ID+"/move", map[string]any{
			"revision": named.Account.Revision, "table_id": second.ID,
		}, 200))
		if named.Account.Name != "Mesa del fondo" {
			t.Fatal("Cambiar de mesa alteró un nombre capturado")
		}
	})

	t.Run("doble envío de la apertura rápida con el mismo operation_id abre una sola cuenta", func(t *testing.T) {
		table := quickTable(t, fixture, "Mesa 9", 300)
		body := map[string]any{"operation_id": randomID(), "name": "", "mode": "dine_in", "table_id": table.ID}
		first := fixture.adminCall(t, "POST", "restaurant/accounts", body, 200)
		repeated := fixture.adminCall(t, "POST", "restaurant/accounts", body, 200)
		var left, right any
		_ = json.Unmarshal(first, &left)
		_ = json.Unmarshal(repeated, &right)
		if string(mustJSON(left)) != string(mustJSON(right)) {
			t.Fatal("El reintento de la apertura no devolvió el resultado original")
		}
		if openAccounts(t, fixture, table.ID) != 1 {
			t.Fatal("El doble envío abrió dos cuentas en la mesa")
		}
		body["mode"] = "takeaway"
		body["table_id"] = nil
		if errorCode(t, fixture.adminCall(t, "POST", "restaurant/accounts", body, 409)) != "IDEMPOTENCY_CONFLICT" {
			t.Fatal("Reutilizar el operation_id con otro contenido debe rechazarse")
		}
		if openAccounts(t, fixture, table.ID) != 1 {
			t.Fatal("El conflicto de idempotencia abrió otra cuenta")
		}
	})

	t.Run("dividir conserva su validación del nombre de la cuenta resultante", func(t *testing.T) {
		bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"mode": "dine_in", "table_id": second.ID,
		}, 200))
		bill = fixture.capture(t, bill, []catalog.LineRequest{{ProductID: fixture.product.ID, Quantity: 2}})
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/split", map[string]any{
			"revision": bill.Account.Revision, "new_account": map[string]string{"name": ""},
			"selections": []any{map[string]any{"version_id": bill.Lines[0].VersionID, "quantity": 1}},
		}, 400)
		after := fixture.refresh(t, bill.Account.ID)
		if after.Account.Revision != bill.Account.Revision || after.Account.ConfirmedQuantity != 2 {
			t.Fatal("La división rechazada alteró la cuenta")
		}
	})
}
