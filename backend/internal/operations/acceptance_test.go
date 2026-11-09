//go:build integration

package operations_test

import (
	"encoding/json"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
)

func TestRestaurantAcceptance(t *testing.T) {
	fixture := newFixture(t)
	fixture.seedCatalog(t)

	t.Run("áreas mesas distribución y revisiones", func(t *testing.T) {
		fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón", Active: true}, 200))
		fixture.secondaryArea = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Terraza", Active: true}, 200))
		fixture.table = decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: "Mesa 1", Shape: "rectangle", X: 20, Y: 30, Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200))
		fixture.secondTable = decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.secondaryArea.ID, Name: "Mesa 2", Shape: "round", X: 140, Y: 30, Width: 90, Height: 90, Seats: 4, Active: true,
		}, 200))
		stale := fixture.table
		fixture.mutate(t, "PUT", "restaurant/layout", map[string]any{"tables": []any{map[string]any{
			"id": fixture.table.ID, "revision": fixture.table.Revision, "x": 220, "y": 140, "width": 100, "height": 72, "shape": "rectangle",
		}}}, 200)
		fixture.mutate(t, "PATCH", "restaurant/tables/"+fixture.table.ID, stale, 409)
		tables := decodeResponse[[]operations.Table](t, fixture.adminCall(t, "GET", "restaurant/tables", nil, 200))
		for _, table := range tables {
			if table.ID == fixture.table.ID {
				fixture.table = table
			}
		}
		if fixture.table.X != 220 || fixture.table.Y != 140 || fixture.table.State != "free" {
			t.Fatal("La distribución no se conservó o se simuló ocupación")
		}
	})

	t.Run("varias cuentas por mesa y borradores sin confirmación implícita", func(t *testing.T) {
		fixture.account = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "Familia A", "mode": "dine_in", "table_id": fixture.table.ID,
		}, 200))
		fixture.secondAccount = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "Familia B", "mode": "dine_in", "table_id": fixture.table.ID,
		}, 200))
		fixture.adminCall(t, "GET", "restaurant/overview", nil, 200)
		tables := decodeResponse[[]operations.Table](t, fixture.adminCall(t, "GET", "restaurant/tables", nil, 200))
		for _, table := range tables {
			if table.ID == fixture.table.ID && (table.AccountCount != 2 || table.State != "occupied") {
				t.Fatal("La mesa no deriva su estado de las cuentas reales")
			}
		}
		items := []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.product.ID, Quantity: 2}}}
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", "restaurant/accounts/"+fixture.account.Account.ID+"/draft-preview", map[string]any{
			"revision": fixture.account.Account.Revision, "items": items,
		}, 200))
		unchanged := fixture.refresh(t, fixture.account.Account.ID)
		if preview.Combined.Total != "300.00" || unchanged.Account.Revision != fixture.account.Account.Revision || len(unchanged.Draft) != 0 || len(unchanged.Lines) != 0 {
			t.Fatal("Cotizar una selección sin guardar debe reutilizar Go sin persistir ni confirmar productos")
		}
		fixture.account = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", "restaurant/accounts/"+fixture.account.Account.ID+"/draft", map[string]any{
			"revision": fixture.account.Account.Revision, "items": items,
		}, 200))
		if len(fixture.account.Lines) != 0 || fixture.account.Account.DraftQuantity != 2 || fixture.account.Account.Totals.Total != "0.00" {
			t.Fatal("Guardar borrador produjo un pedido confirmado")
		}
	})

	t.Run("confirmación expresa huella y reintento idempotente", func(t *testing.T) {
		route := "restaurant/accounts/" + fixture.account.Account.ID
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "GET", route+"/preview", nil, 200))
		if preview.Quote.Total != "300.00" {
			t.Fatal("La cuenta no reutilizó el motor Go")
		}
		fixture.mutate(t, "POST", route+"/confirm", map[string]any{"revision": fixture.account.Account.Revision, "expected_fingerprint": "obsoleta"}, 409)
		body := map[string]any{"operation_id": randomID(), "revision": fixture.account.Account.Revision, "expected_fingerprint": preview.Fingerprint}
		first := fixture.adminCall(t, "POST", route+"/confirm", body, 200)
		repeated := fixture.adminCall(t, "POST", route+"/confirm", body, 200)
		var left, right any
		_ = json.Unmarshal(first, &left)
		_ = json.Unmarshal(repeated, &right)
		if string(mustJSON(left)) != string(mustJSON(right)) {
			t.Fatal("El reintento no devolvió el mismo resultado persistente")
		}
		fixture.account = decodeResponse[operations.Bill](t, first)
		if fixture.account.Account.ConfirmedQuantity != 2 || len(fixture.account.Draft) != 0 {
			t.Fatal("Confirmación inconsistente")
		}
		body["expected_fingerprint"] = "otra solicitud"
		fixture.adminCall(t, "POST", route+"/confirm", body, 409)
		var count int
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM app.restaurant_orders WHERE original_account_id=$1", fixture.account.Account.ID).Scan(&count); err != nil || count != 1 {
			t.Fatal("Se duplicó la confirmación")
		}
	})

	t.Run("precios históricos y edición de observaciones", func(t *testing.T) {
		fixture.product.BasePrice = "900"
		fixture.product = decodeResponse[catalog.Product](t, fixture.adminCall(t, "PATCH", "catalog/products/"+fixture.product.ID, fixture.product, 200))
		fixture.account = fixture.refresh(t, fixture.account.Account.ID)
		if fixture.account.Account.Totals.Total != "300.00" {
			t.Fatal("Un cambio de catálogo alteró precios confirmados")
		}
		line := fixture.account.Lines[0]
		request := line.Request
		request.Quantity = line.Quantity
		request.Customization.Notes = "Salsa aparte"
		edit := operations.EditInput{
			Mutation:  operations.Mutation{Revision: fixture.account.Account.Revision},
			VersionID: line.VersionID, Line: request, Reason: "Cambio solicitado por el cliente",
		}
		route := "restaurant/accounts/" + fixture.account.Account.ID
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", route+"/edit-preview", edit, 200))
		if preview.Quote.Total != "300.00" {
			t.Fatal("Cambiar notas volvió a cotizar un precio confirmado")
		}
		edit.ExpectedFingerprint = preview.Fingerprint
		fixture.account = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", route+"/edit", edit, 200))
		if fixture.account.Lines[0].Request.Customization.Notes != "Salsa aparte" {
			t.Fatal("Se perdieron las observaciones")
		}
	})

	t.Run("división parcial conserva identidades importes y personalización", func(t *testing.T) {
		line := fixture.account.Lines[0]
		input := operations.TransferInput{
			Mutation:   operations.Mutation{Revision: fixture.account.Account.Revision},
			NewAccount: &operations.AccountInput{Name: "Cuenta dividida"},
			Selections: []operations.Selection{{VersionID: line.VersionID, Quantity: 1}},
		}
		var pair struct {
			Source operations.Bill `json:"source"`
			Target operations.Bill `json:"target"`
		}
		pair = decodeResponse[struct {
			Source operations.Bill `json:"source"`
			Target operations.Bill `json:"target"`
		}](t,
			fixture.mutate(t, "POST", "restaurant/accounts/"+fixture.account.Account.ID+"/split", input, 200))
		fixture.account, fixture.splitAccount = pair.Source, pair.Target
		if pair.Source.Account.Totals.Total != "150.00" || pair.Target.Account.Totals.Total != "150.00" {
			t.Fatal("La división modificó o duplicó dinero")
		}
		if pair.Source.Lines[0].ItemID != pair.Target.Lines[0].ItemID || pair.Source.Lines[0].VersionID != pair.Target.Lines[0].VersionID {
			t.Fatal("La división duplicó la identidad comercial")
		}
		if pair.Target.Lines[0].Request.Customization.Notes != "Salsa aparte" {
			t.Fatal("La división perdió personalizaciones")
		}
	})

	t.Run("transferencia parcial combinación y movimiento de mesa", func(t *testing.T) {
		line := fixture.splitAccount.Lines[0]
		fixture.mutate(t, "POST", "restaurant/accounts/"+fixture.splitAccount.Account.ID+"/transfer", operations.TransferInput{
			Mutation:        operations.Mutation{Revision: fixture.splitAccount.Account.Revision},
			TargetAccountID: &fixture.secondAccount.Account.ID, TargetRevision: fixture.secondAccount.Account.Revision,
			Selections: []operations.Selection{{VersionID: line.VersionID, Quantity: 1}},
		}, 200)
		fixture.secondAccount = fixture.refresh(t, fixture.secondAccount.Account.ID)
		fixture.mutate(t, "POST", "restaurant/accounts/"+fixture.secondAccount.Account.ID+"/move", map[string]any{
			"revision": fixture.secondAccount.Account.Revision, "table_id": fixture.secondTable.ID,
		}, 200)
		fixture.secondAccount = fixture.refresh(t, fixture.secondAccount.Account.ID)
		fixture.mutate(t, "POST", "restaurant/accounts/"+fixture.account.Account.ID+"/merge", map[string]any{
			"revision": fixture.account.Account.Revision, "target_account_id": fixture.secondAccount.Account.ID, "target_revision": fixture.secondAccount.Account.Revision,
		}, 200)
		fixture.secondAccount = fixture.refresh(t, fixture.secondAccount.Account.ID)
		fixture.account = fixture.refresh(t, fixture.account.Account.ID)
		if fixture.secondAccount.Account.Totals.Total != "300.00" || fixture.secondAccount.Account.ConfirmedQuantity != 2 || fixture.account.Account.State != "merged" {
			t.Fatal("La combinación perdió unidades o historial")
		}
		if fixture.secondAccount.Account.TableID == nil || *fixture.secondAccount.Account.TableID != fixture.secondTable.ID {
			t.Fatal("El cambio de mesa no persistió")
		}
	})

	t.Run("cancelación parcial sin devolución financiera e historial", func(t *testing.T) {
		line := fixture.secondAccount.Lines[0]
		fixture.secondAccount = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts/"+fixture.secondAccount.Account.ID+"/cancel", map[string]any{
			"revision": fixture.secondAccount.Account.Revision, "version_id": line.VersionID, "quantity": 1, "reason": "Cliente retiró una porción",
		}, 200))
		if fixture.secondAccount.Account.Totals.Total != "150.00" || fixture.secondAccount.Account.ConfirmedQuantity != 1 {
			t.Fatal("Cancelación parcial incorrecta")
		}
		history := fixture.adminCall(t, "GET", "restaurant/accounts/"+fixture.secondAccount.Account.ID+"/history", nil, 200)
		var result struct {
			Items []operations.HistoryEvent `json:"items"`
		}
		_ = json.Unmarshal(history, &result)
		if len(result.Items) < 3 {
			t.Fatal("Falta historial durable de movimientos y cancelación")
		}
	})

	t.Run("mesa histórica desactivada sin eliminar cuentas", func(t *testing.T) {
		split := fixture.refresh(t, fixture.splitAccount.Account.ID)
		fixture.mutate(t, "POST", "restaurant/accounts/"+split.Account.ID+"/void", map[string]any{
			"revision": split.Account.Revision, "reason": "Cuenta vacía tras transferencia",
		}, 200)
		fixture.table.Active = false
		fixture.table = decodeResponse[operations.Table](t, fixture.mutate(t, "PATCH", "restaurant/tables/"+fixture.table.ID, fixture.table, 200))
		fixture.refresh(t, fixture.account.Account.ID)
		if fixture.table.Active || fixture.table.State != "out_of_service" {
			t.Fatal("La mesa desactivada no conservó su estado")
		}
	})

	testCaptureRules(t, fixture)
	testTakeaway(t, fixture)
	testSecurityAndConcurrency(t, fixture)
	testKitchenAcceptance(t, fixture)
	testPayments(t, fixture)
	testOrderPositions(t, fixture)
	testQuickOpen(t, fixture)
	testTablesStage71(t, fixture)
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
