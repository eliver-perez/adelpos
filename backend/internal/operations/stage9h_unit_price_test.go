//go:build integration

package operations_test

import (
	"encoding/json"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa 9H, fase A (§2.3) contra PostgreSQL con la API real: el precio unitario de la precuenta y el
// ticket lo calcula Go. Cada partida del documento guarda el precio comercial de una unidad (el precio
// unitario del motor del catálogo con sus complementos e ingredientes extra, antes de promociones y
// descuentos) y el importe sigue siendo el exacto de sus unidades. Con la plantilla predeterminada nada
// cambia; con «Mostrar precio unitario» la precuenta, la reimpresión con la plantilla vigente y la vista
// previa del diseñador con el documento real lo muestran. Ningún trabajo llega a una impresora (la
// cola del equipo no existe y el trabajador no corre).

func TestStage9HUnitPriceInCommercialDocuments(t *testing.T) {
	resetStage9DDatabase(t)
	fixture := &paymentsFixture{restaurantFixture: newFixture(t), methods: map[string]payments.Method{}, registers: map[string]string{}}
	fixture.seedCatalog(t)
	tickets := fixture.createPrinter(t, kitchen.Printer{
		Name: "Tickets 9H", Transport: "cups", Destination: "ADEL_TEST_9H_TICKETS", Active: true, PaperWidth: 80, Copies: 1,
	})
	fixture.receiptSettings(t, tickets.ID, false)
	// Tres filetes con una porción extra de cebolla (150.00 + 2.50) y dos papas (40.00).
	onion := catalog.LineRequest{ProductID: fixture.product.ID, Quantity: 3}
	onion.Customization.Ingredients = []catalog.IngredientSelection{{IngredientID: fixture.product.Ingredients[0].IngredientID, Quantity: "2"}}
	bill := fixture.account(t, "Precio unitario 9H", onion, line(fixture.fries, 2))
	route := "payments/accounts/" + bill.Account.ID

	var original payments.CommercialDocument
	t.Run("cada partida guarda el precio unitario de Go y la predeterminada no lo imprime", func(t *testing.T) {
		original = decodeResponse[payments.CommercialDocument](t, fixture.adminCall(t, "GET", route+"/precheck", nil, 200))
		expected := map[string][2]string{"Filete": {"152.50", "457.50"}, "Papas producto": {"40.00", "80.00"}}
		for _, item := range original.Lines {
			values, found := expected[item.Name]
			if !found || item.UnitPrice != values[0] || item.Amount != values[1] {
				t.Fatalf("Partida %+v; se esperaba precio unitario y importe %v", item, values)
			}
		}
		if len(original.Lines) != 2 || strings.Contains(original.Content, "×") {
			t.Fatalf("La precuenta predeterminada no debe mostrar el precio unitario:\n%s", original.Content)
		}
	})

	t.Run("la precuenta impresa guarda el precio unitario en su documento", func(t *testing.T) {
		printed := decodeResponse[printingResult](t, fixture.post(t, fixture.admin, bill.Account.ID, "precheck/print", map[string]any{}, 200))
		var stored []byte
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT document FROM app.kitchen_tickets WHERE id=$1", printed.TicketID).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		var document payments.CommercialDocument
		if err := json.Unmarshal(stored, &document); err != nil {
			t.Fatal(err)
		}
		if len(document.Lines) != 2 || document.Lines[0].UnitPrice == "" || document.Lines[1].UnitPrice == "" {
			t.Fatalf("El documento guardado no tiene el precio unitario: %+v", document.Lines)
		}
		// La vista previa del diseñador con ese documento real y la opción activada lo muestra.
		layout := documents.Default(documents.KindPrecheck)
		enableUnitPrice(t, layout)
		preview := decodeResponse[struct {
			Text string `json:"text"`
		}](t, fixture.adminCall(t, "POST", "documents/preview", map[string]any{
			"kind": "precheck", "layout": json.RawMessage(layout.Encode()), "paper_width": 80,
			"source": map[string]any{"ticket_id": printed.TicketID},
		}, 200))
		for _, fragment := range []string{"3 × $152.50 Filete", "$457.50", "2 × $40.00 Papas producto", "$80.00"} {
			if !strings.Contains(preview.Text, fragment) {
				t.Fatalf("La vista previa con el documento real no muestra %q:\n%s", fragment, preview.Text)
			}
		}
	})

	t.Run("con la opción activa la precuenta muestra el precio unitario y el mismo total", func(t *testing.T) {
		created := decodeResponse[struct {
			ID       string          `json:"id"`
			Revision int             `json:"revision"`
			Layout   json.RawMessage `json:"layout"`
		}](t, fixture.mutate(t, "POST", "documents/templates", map[string]any{
			"kind": "precheck", "name": "Precuenta con precio unitario 9H", "source_version_id": documents.SystemVersionID(documents.KindPrecheck),
		}, 200))
		layout, err := documents.ParseTemplate(documents.KindPrecheck, created.Layout)
		if err != nil {
			t.Fatal(err)
		}
		enableUnitPrice(t, layout)
		saved := decodeResponse[struct {
			Revision int             `json:"revision"`
			Layout   json.RawMessage `json:"layout"`
		}](t, fixture.mutate(t, "PUT", "documents/templates/"+created.ID, map[string]any{
			"revision": created.Revision, "layout": json.RawMessage(layout.Encode()), "note": "Precio unitario",
		}, 200))
		if !strings.Contains(string(saved.Layout), `"show_unit_price":true`) {
			t.Fatalf("La versión guardada debe conservar la opción: %s", saved.Layout)
		}
		fixture.mutate(t, "POST", "documents/templates/"+created.ID+"/activate", map[string]any{"revision": saved.Revision}, 200)
		current := decodeResponse[payments.CommercialDocument](t, fixture.adminCall(t, "GET", route+"/precheck", nil, 200))
		if current.Total != original.Total || current.Balance != original.Balance {
			t.Fatalf("El total cambió: %s → %s", original.Total, current.Total)
		}
		for _, fragment := range []string{"3 × $152.50 Filete", "2 × $40.00 Papas producto", "$457.50", "$80.00"} {
			if !strings.Contains(current.Content, fragment) {
				t.Fatalf("La precuenta no muestra %q:\n%s", fragment, current.Content)
			}
		}
		// La comanda de cocina no ofrece la opción: Go rechaza la clave en la lista de cocina (la misma
		// plantilla sin ella es válida).
		kitchenLayout := func(options map[string]any) []byte {
			raw, err := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "items", "type": "kitchen_items", "options": options}}})
			if err != nil {
				t.Fatal(err)
			}
			return raw
		}
		if _, err := documents.ParseTemplate(documents.KindKitchenOrder, kitchenLayout(map[string]any{})); err != nil {
			t.Fatal(err)
		}
		if _, err := documents.ParseTemplate(documents.KindKitchenOrder, kitchenLayout(map[string]any{"show_unit_price": true})); err == nil {
			t.Fatal("La lista de cocina no debe aceptar el precio unitario")
		}
	})
}

// enableUnitPrice activa «Mostrar precio unitario» en la lista de productos de una plantilla.
func enableUnitPrice(t *testing.T, template *documents.Template) {
	t.Helper()
	for index := range template.Blocks {
		if template.Blocks[index].Type == documents.BlockProducts {
			template.Blocks[index].Options = json.RawMessage(`{"show_notes":true,"show_unit_price":true}`)
			return
		}
	}
	t.Fatal("La plantilla no tiene lista de productos")
}
