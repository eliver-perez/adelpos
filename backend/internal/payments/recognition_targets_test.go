package payments

import (
	"testing"

	"github.com/shopspring/decimal"
)

func pointer[T any](value T) *T {
	return &value
}

// sumByRate agrega los renglones por tasa.
func sumByRate(lines []recognitionLine) map[int]recognitionAmounts {
	result := map[int]recognitionAmounts{}
	for _, line := range lines {
		current := result[line.Key.TaxRate]
		current.Quantity += line.Amounts.Quantity
		current.Gross = current.Gross.Add(line.Amounts.Gross)
		current.Promotion = current.Promotion.Add(line.Amounts.Promotion)
		current.Discount = current.Discount.Add(line.Amounts.Discount)
		current.Courtesy = current.Courtesy.Add(line.Amounts.Courtesy)
		current.Charge = current.Charge.Add(line.Amounts.Charge)
		current.Net = current.Net.Add(line.Amounts.Net)
		current.Base = current.Base.Add(line.Amounts.Base)
		current.Tax = current.Tax.Add(line.Amounts.Tax)
		result[line.Key.TaxRate] = current
	}
	return result
}

// El reconocimiento de una cuenta con dos tasas, descuento por partida, cargo y dos descuentos
// generales suma por tasa exactamente el desglose de IVA de finance.go, cada descuento general se
// reconoce completo y cada renglón cumple neto = bruto − promoción − descuentos + cargos = base + IVA.
func TestRecognitionTargetsMatchFinanceBreakdown(t *testing.T) {
	general := recognitionAttributes{AdjustmentKind: pointer("discount"), AdjustmentScope: pointer("account")}
	units := []recognitionUnit{
		{ID: "00000000-0000-4000-8000-00000000000a", TaxRate: 16, Subtotal: amount("110.00"), Promotion: amount("10.00"), Total: amount("100.00")},
		{ID: "00000000-0000-4000-8000-00000000000b", TaxRate: 16, Subtotal: amount("33.33"), Promotion: zero, Total: amount("33.33")},
		{ID: "00000000-0000-4000-8000-00000000000c", TaxRate: 8, Subtotal: amount("50.00"), Promotion: zero, Total: amount("50.00")},
	}
	allocations := []recognitionAllocation{{
		UnitID: units[0].ID, AdjustmentID: "00000000-0000-4000-8000-0000000000d1", Amount: amount("10.00"),
		Adjustment: recognitionAttributes{AdjustmentKind: pointer("discount"), AdjustmentScope: pointer("items")},
	}}
	adjustments := []recognitionAdjustment{
		{ID: "00000000-0000-4000-8000-0000000000e1", Kind: "charge", TaxRate: pointer(16), Amount: amount("12.50"),
			Attributes: recognitionAttributes{AdjustmentKind: pointer("charge"), AdjustmentScope: pointer("account")}},
		{ID: "00000000-0000-4000-8000-0000000000e2", Kind: "discount", Amount: amount("7.77"), Attributes: general},
		{ID: "00000000-0000-4000-8000-0000000000e3", Kind: "discount", Amount: amount("5.01"), Attributes: general},
	}
	lines := buildRecognitionTargets(units, allocations, adjustments, recognitionAttributes{})
	byRate := sumByRate(lines)
	// finance.go: bruto 16 % = 90 + 33.33 + 12.50; 8 % = 50; descuento general 12.78 repartido 9.34/3.44.
	expected := taxBreakdown(map[int]decimal.Decimal{16: amount("126.49"), 8: amount("46.56")})
	for _, line := range expected {
		got := byRate[line.Rate]
		if money(got.Net) != line.Total || money(got.Base) != line.Base || money(got.Tax) != line.Tax {
			t.Fatalf("Tasa %d: neto %s base %s IVA %s; finance.go %s %s %s", line.Rate, money(got.Net), money(got.Base), money(got.Tax),
				line.Total, line.Base, line.Tax)
		}
	}
	if total := byRate[16].Net.Add(byRate[8].Net); money(total) != "173.05" {
		t.Fatalf("El neto reconocido debe ser el total de la cuenta: %s", money(total))
	}
	perAdjustment := map[string]decimal.Decimal{}
	for _, line := range lines {
		amounts := line.Amounts
		if !amounts.Net.Equal(amounts.Gross.Sub(amounts.Promotion).Sub(amounts.Discount).Sub(amounts.Courtesy).Add(amounts.Charge)) ||
			!amounts.Net.Equal(amounts.Base.Add(amounts.Tax)) {
			t.Fatalf("Renglón incoherente: %+v", line)
		}
		if line.Key.Kind == sourceAdjustment {
			perAdjustment[line.Key.AdjustmentID] = perAdjustment[line.Key.AdjustmentID].Add(amounts.Discount).Add(amounts.Charge)
		}
	}
	if money(perAdjustment["00000000-0000-4000-8000-0000000000e2"]) != "7.77" ||
		money(perAdjustment["00000000-0000-4000-8000-0000000000e3"]) != "5.01" {
		t.Fatalf("Cada descuento general debe reconocerse completo: %v", perAdjustment)
	}
	if byRate[16].Quantity+byRate[8].Quantity != 3 || money(byRate[16].Promotion) != "10.00" || money(byRate[16].Discount.Add(byRate[8].Discount)) != "22.78" {
		t.Fatalf("Cantidades, promociones o descuentos inesperados: %+v", byRate)
	}
}

// Una cortesía general al 100 % deja la venta neta en cero con su importe en «Cortesías».
func TestCourtesyRecognizesZeroSale(t *testing.T) {
	units := []recognitionUnit{
		{ID: "00000000-0000-4000-8000-000000000001", TaxRate: 16, Subtotal: amount("25.00"), Promotion: zero, Total: amount("25.00")},
		{ID: "00000000-0000-4000-8000-000000000002", TaxRate: 16, Subtotal: amount("25.00"), Promotion: zero, Total: amount("25.00")},
	}
	courtesy := recognitionAdjustment{ID: "00000000-0000-4000-8000-000000000003", Kind: "discount", Amount: amount("50.00"),
		Attributes: recognitionAttributes{AdjustmentKind: pointer("discount"), AdjustmentScope: pointer("account"), IsCourtesy: true}}
	lines := buildRecognitionTargets(units, nil, []recognitionAdjustment{courtesy}, recognitionAttributes{})
	totals := sumByRate(lines)[16]
	if !totals.Net.IsZero() || money(totals.Courtesy) != "50.00" || !totals.Discount.IsZero() || !totals.Tax.IsZero() ||
		money(totals.Gross) != "50.00" || totals.Quantity != 2 {
		t.Fatalf("La cortesía total debe reconocer venta 0 y su importe en cortesías: %+v", totals)
	}
}

// El residuo de centavos del IVA se reparte entre los renglones de mayor neto: con siete unidades de
// $0.10 al 16 % el IVA por renglón es 0.01 (0.07 en total), pero el de la cuenta ($0.70) es 0.10; los
// tres centavos de diferencia van a tres renglones, uno a cada uno.
func TestTaxResidualDistribution(t *testing.T) {
	units := []recognitionUnit{}
	for index := 0; index < 7; index++ {
		units = append(units, recognitionUnit{
			ID: "00000000-0000-4000-8000-00000000001" + string(rune('0'+index)), TaxRate: 16,
			Subtotal: amount("0.10"), Promotion: zero, Total: amount("0.10"),
		})
	}
	lines := buildRecognitionTargets(units, nil, nil, recognitionAttributes{})
	totals := sumByRate(lines)[16]
	expected := taxBreakdown(map[int]decimal.Decimal{16: amount("0.70")})[0]
	if money(totals.Tax) != expected.Tax || money(totals.Base) != expected.Base {
		t.Fatalf("IVA %s base %s; desglose %s %s", money(totals.Tax), money(totals.Base), expected.Tax, expected.Base)
	}
	for _, line := range lines {
		if line.Amounts.Tax.IsNegative() || line.Amounts.Tax.GreaterThan(amount("0.02")) {
			t.Fatalf("Residuo mal repartido: %+v", line.Amounts)
		}
	}
}
