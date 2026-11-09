//go:build integration

package operations_test

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// taxRate devuelve un apuntador a una tasa propia de producto.
func taxRate(value int) *int {
	return &value
}

// assertExactTax comprueba que cada tasa cumpla base + IVA = total y que los totales sumen el esperado.
func assertExactTax(t *testing.T, lines []payments.TaxLine, total string) {
	t.Helper()
	sum := decimal.Zero
	for _, line := range lines {
		if !amountOf(t, line.Base).Add(amountOf(t, line.Tax)).Equal(amountOf(t, line.Total)) {
			t.Fatalf("Base más IVA no es exactamente el total de la tasa %d: %+v", line.Rate, line)
		}
		sum = sum.Add(amountOf(t, line.Total))
	}
	if !sum.Equal(amountOf(t, total)) {
		t.Fatalf("Los totales por tasa suman %s y se esperaba %s: %+v", sum, total, lines)
	}
}

// unitRates resume la tasa efectiva guardada en cada unidad, por nombre de partida.
func unitRates(finance payments.Finance) map[string][]int {
	result := map[string][]int{}
	for _, line := range finance.Lines {
		for _, unit := range line.Units {
			result[line.Name] = append(result[line.Name], unit.TaxRate)
		}
	}
	return result
}

func (fixture *paymentsFixture) setGeneralTaxRate(t *testing.T, rate string) {
	t.Helper()
	settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
	fixture.adminCall(t, "PUT", "payments/tax", map[string]any{"operation_id": randomID(), "revision": settings.Revision, "tax_rate": rate}, 200)
}

func testProductTaxRates(t *testing.T, fixture *paymentsFixture) {
	cash := fixture.methods["cash"].ID
	debit := fixture.methods["debit"].ID
	createProduct := func(name, code, price string, rate *int) catalog.Product {
		return decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
			Metadata: catalog.Metadata{Name: name, Active: true}, CategoryID: fixture.category.ID, Code: code, Kind: "product",
			BasePrice: price, TaxRate: rate,
		}, 200))
	}
	general := createProduct("Platillo con tasa general", "IVA_GENERAL", "116", nil)
	border := createProduct("Platillo de frontera", "IVA_FRONTERA", "108", taxRate(8))
	exempt := createProduct("Producto exento", "IVA_EXENTO", "50", taxRate(0))
	combo := decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Combo de frontera", Active: true}, CategoryID: fixture.category.ID, Code: "IVA_COMBO",
		Kind: "combo", BasePrice: "90", TaxRate: taxRate(8),
		Slots: []catalog.ComboSlot{{
			Name:             "Acompañante exento",
			MaxQuantity:      2,
			DefaultProductID: exempt.ID,
			Options: []catalog.ComboOption{{
				ProductID:         exempt.ID,
				SubstitutionPrice: "0",
				Tiers:             []catalog.QuantityTier{{From: 1, UnitPrice: "10"}, {From: 2, UnitPrice: "40"}},
			}},
		}},
	}, 200))
	comboLine := catalog.LineRequest{ProductID: combo.ID, Quantity: 1, Customization: catalog.Customization{
		Components: []catalog.ComponentSelection{{SlotID: combo.Slots[0].ID, ProductID: exempt.ID, Quantity: 1}},
	}}
	var printerID string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id FROM app.kitchen_printers WHERE name='Tickets de caja de aceptación'").Scan(&printerID); err != nil {
		t.Fatal(err)
	}
	fixture.receiptSettings(t, printerID, false)

	t.Run("T1_tasa_efectiva_por_unidad_herencia_y_combo", func(t *testing.T) {
		bill := fixture.account(t, "Tasas mixtas", line(general, 1), line(border, 1), line(exempt, 1), comboLine)
		finance := fixture.finance(t, bill.Account.ID)
		rates := unitRates(finance)
		if rates[general.Name][0] != 16 || rates[border.Name][0] != 8 || rates[exempt.Name][0] != 0 || rates[combo.Name][0] != 8 {
			t.Fatalf("Las unidades no guardaron su tasa efectiva: %+v", rates)
		}
		if finance.Total != "374.00" || len(finance.Tax) != 3 {
			t.Fatalf("El desglose no agrupó tres tasas: %s %+v", finance.Total, finance.Tax)
		}
		want := map[int][3]string{16: {"100.00", "16.00", "116.00"}, 8: {"192.59", "15.41", "208.00"}, 0: {"50.00", "0.00", "50.00"}}
		for _, tax := range finance.Tax {
			expected := want[tax.Rate]
			if tax.Base != expected[0] || tax.Tax != expected[1] || tax.Total != expected[2] {
				t.Fatalf("Desglose incorrecto para %d %%: %+v", tax.Rate, tax)
			}
		}
		assertExactTax(t, finance.Tax, finance.Total)
		var stored int
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FROM app.restaurant_item_versions v JOIN app.restaurant_items i ON i.id=v.item_id
            WHERE i.product_id=$1 AND v.tax_rate=8`, combo.ID).Scan(&stored); err != nil || stored != 1 {
			t.Fatal("El combo no guardó su propia tasa para todo su importe")
		}
		fixture.registers["tax_account"] = bill.Account.ID
	})

	t.Run("T2_precuenta_ticket_y_devoluciones_con_desglose_exacto_por_tasa", func(t *testing.T) {
		accountID := fixture.registers["tax_account"]
		precheck := decodeResponse[payments.CommercialDocument](t, fixture.adminCall(t, "GET", "payments/accounts/"+accountID+"/precheck", nil, 200))
		for _, fragment := range []string{"Subtotal sin IVA 16 %", "IVA 16 %", "Subtotal sin IVA 8 %", "$192.59", "IVA 8 %", "$15.41", "Subtotal sin IVA 0 %", "IVA 0 %", "$374.00"} {
			if !strings.Contains(precheck.Content, fragment) {
				t.Fatalf("La precuenta no desglosa %q:\n%s", fragment, precheck.Content)
			}
		}
		assertExactTax(t, precheck.Tax, precheck.Total)
		deposit := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, accountID, map[string]any{
			"method_id": cash, "mode": "partial", "amount": "100",
		}, 200))
		finance := fixture.finance(t, accountID)
		borderLine := lineOf(t, finance, border)
		fixture.pay(t, fixture.admin, accountID, map[string]any{
			"method_id": debit, "mode": "total", "selections": []map[string]any{{"version_id": borderLine.VersionID, "quantity": 1}},
		}, 200)
		printed := decodeResponse[struct {
			Printing printingResult `json:"printing"`
		}](t, fixture.pay(t, fixture.admin, accountID, map[string]any{"method_id": cash, "mode": "total", "print": true}, 200))
		var content string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT content FROM app.kitchen_print_jobs WHERE id=$1", printed.Printing.JobID).Scan(&content); err != nil {
			t.Fatal(err)
		}
		for _, fragment := range []string{"Subtotal sin IVA 16 %", "IVA 8 %", "$15.41", "IVA 0 %", "TOTAL"} {
			if !strings.Contains(content, fragment) {
				t.Fatalf("El ticket no desglosa %q:\n%s", fragment, content)
			}
		}
		// Devolución por productos: congela la tasa de la unidad devuelta.
		items := decodeResponse[refundResult](t, fixture.post(t, fixture.admin, accountID, "refunds", map[string]any{
			"mode": "items", "method_id": cash, "reason": "Producto de frontera devuelto",
			"selections": []map[string]any{{"version_id": borderLine.VersionID, "quantity": 1}},
		}, 200))
		frozen := items.Finance.Refunds[len(items.Finance.Refunds)-1].Tax
		if len(frozen) != 1 || frozen[0].Rate != 8 || frozen[0].Base != "100.00" || frozen[0].Tax != "8.00" {
			t.Fatalf("La devolución por productos no congeló la tasa de la unidad: %+v", frozen)
		}
		// Devolución de la parte general de un pago: se reparte entre las tres tasas de la cuenta.
		parts := decodeResponse[refundResult](t, fixture.post(t, fixture.admin, accountID, "refunds", map[string]any{
			"mode": "payment", "payment_id": deposit.Payment.ID, "amount": "100", "method_id": cash, "reason": "Abono devuelto al cliente",
		}, 200))
		spread := parts.Finance.Refunds[len(parts.Finance.Refunds)-1].Tax
		if len(spread) != 3 {
			t.Fatalf("La devolución general no se repartió entre las tasas presentes: %+v", spread)
		}
		assertExactTax(t, spread, "100.00")
	})

	t.Run("T3_cambios_de_tasa_solo_afectan_lo_que_se_confirma_despues", func(t *testing.T) {
		before := fixture.account(t, "Hereda antes del cambio", line(general, 1))
		fixture.setGeneralTaxRate(t, "8")
		after := fixture.account(t, "Hereda después del cambio", line(general, 1))
		if unitRates(fixture.finance(t, before.Account.ID))[general.Name][0] != 16 || unitRates(fixture.finance(t, after.Account.ID))[general.Name][0] != 8 {
			t.Fatal("La herencia de la tasa general no tomó la vigente al confirmar o alteró una cuenta anterior")
		}
		fixture.setGeneralTaxRate(t, "16")
		if unitRates(fixture.finance(t, before.Account.ID))[general.Name][0] != 16 {
			t.Fatal("Restablecer la tasa general alteró una versión confirmada")
		}
		bill := fixture.account(t, "Edición con tasa propia", line(border, 1))
		current := decodeResponse[catalog.Product](t, fixture.adminCall(t, "GET", "catalog/products/"+border.ID, nil, 200))
		current.TaxRate = taxRate(0)
		fixture.adminCall(t, "PATCH", "catalog/products/"+border.ID, current, 200)
		// Una edición que conserva importes (observaciones) conserva la tasa histórica.
		bill = fixture.refresh(t, bill.Account.ID)
		request := bill.Lines[0].Request
		request.Quantity = bill.Lines[0].Quantity
		request.Customization.Notes = "Sin hielo"
		edit := operations.EditInput{
			Mutation: operations.Mutation{Revision: bill.Account.Revision}, VersionID: bill.Lines[0].VersionID, Line: request, Reason: "Indicación del cliente",
		}
		route := "restaurant/accounts/" + bill.Account.ID
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", route+"/edit-preview", edit, 200))
		edit.ExpectedFingerprint = preview.Fingerprint
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", route+"/edit", edit, 200))
		if rates := unitRates(fixture.finance(t, bill.Account.ID))[border.Name]; len(rates) != 1 || rates[0] != 8 {
			t.Fatalf("Una edición que conserva importes cambió la tasa histórica: %+v", rates)
		}
		// Una edición con nueva cotización toma la tasa efectiva vigente del producto.
		request = bill.Lines[0].Request
		request.Quantity = 2
		edit = operations.EditInput{
			Mutation: operations.Mutation{Revision: bill.Account.Revision}, VersionID: bill.Lines[0].VersionID, Line: request, Reason: "Otra pieza para el cliente",
		}
		preview = decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", route+"/edit-preview", edit, 200))
		edit.ExpectedFingerprint = preview.Fingerprint
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", route+"/edit", edit, 200))
		finance := fixture.finance(t, bill.Account.ID)
		if rates := unitRates(finance)[border.Name]; len(rates) != 2 || rates[0] != 0 || rates[1] != 0 {
			t.Fatalf("Una edición con nueva cotización no tomó la tasa vigente del producto: %+v", rates)
		}
		assertExactTax(t, finance.Tax, finance.Total)
	})
}
