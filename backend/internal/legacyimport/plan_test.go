package legacyimport

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func mustLocation(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func loadSynthetic(t *testing.T, pkg *syntheticPackage) *Model {
	t.Helper()
	model, err := Load(context.Background(), pkg.source(), mustLocation(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	return model
}

func TestParseFixedIsExact(t *testing.T) {
	cases := map[string]int64{
		"0":            0,
		"12":           120000,
		"12.5":         125000,
		"-0.0001":      -1,
		"123.4567":     1234567,
		"1.500000":     15000,
		"00045.1000":   451000,
		"922337203685": 9223372036850000,
	}
	for text, expected := range cases {
		value, err := parseFixed(text, moneyScale)
		if err != nil || value != expected {
			t.Fatalf("parseFixed(%q) = %d %v; se esperaba %d", text, value, err, expected)
		}
	}
	for _, text := range []string{"", "-", "1.", ".5", "1.23456", "1e3", "12a", "9223372036854775807", "1,5"} {
		if _, err := parseFixed(text, moneyScale); err == nil {
			t.Fatalf("parseFixed(%q) debía fallar", text)
		}
	}
	if Money(-12345).String() != "-1.2345" || Money(5).String() != "0.0005" || Money(1234500).Display() != "123.45" {
		t.Fatalf("formato de importes: %s %s %s", Money(-12345), Money(5), Money(1234500).Display())
	}
	if roundMoney(Money(1100050), 2) != Money(1100100) || roundMoney(Money(-12350), 2) != Money(-12400) ||
		roundMoney(Money(-12349), 2) != Money(-12300) {
		t.Fatal("redondeo comercial a centavos")
	}
}

func TestDeduceRateAndEstimate(t *testing.T) {
	cases := []struct {
		subtotal, tax Money
		rate          int
		exact         bool
	}{
		{1000000, 0, 0, true},
		{258621, 41379, 16, true},
		{1000000, 80000, 8, true},
		{1000000, 100000, 8, false},
		{-200000, -32000, 16, true},
		{0, 0, 0, true},
		{0, 5, 0, false},
	}
	for _, item := range cases {
		rate, exact := deduceRate(item.subtotal, item.tax)
		if rate != item.rate || exact != item.exact {
			t.Fatalf("deduceRate(%s, %s) = %d %v", item.subtotal, item.tax, rate, exact)
		}
	}
	if estimateTax(300000, 16) != 41379 || estimateTax(-300000, 16) != -41379 || estimateTax(300000, 0) != 0 {
		t.Fatalf("IVA estimado: %s %s", estimateTax(300000, 16), estimateTax(-300000, 16))
	}
}

func TestDeriveUsername(t *testing.T) {
	used := map[string]bool{"admin": true, "sa": true}
	cases := []struct {
		original, expected string
		renamed            bool
	}{
		{"Admin", "admin-2", true},
		{"maria", "maria", false},
		{"MARIA", "maria-2", true},
		{"1beto", "u1beto", true},
		{"José", "jose", true},
		{"sa", "sa-2", true},
		{"  ", "usuario7", true},
		{"a", "a0", true},
	}
	for _, item := range cases {
		username, renamed := deriveUsername(item.original, 7, used)
		if username != item.expected || renamed != item.renamed {
			t.Fatalf("deriveUsername(%q) = %q %v; se esperaba %q", item.original, username, renamed, item.expected)
		}
		if !usernamePattern.MatchString(username) {
			t.Fatalf("nombre de usuario inválido: %q", username)
		}
	}
}

func TestLoadRejectsIncompatiblePackages(t *testing.T) {
	pkg := standardSynthetic()
	for index, name := range pkg.order {
		if name == "pagos" {
			pkg.order = append(pkg.order[:index], pkg.order[index+1:]...)
			break
		}
	}
	_, err := Load(context.Background(), pkg.source(), mustLocation(t), nil)
	if CodeOf(err) != CodeIncompatible || !strings.Contains(MessageOf(err), "pagos") {
		t.Fatalf("sin la tabla pagos: %v", err)
	}
	broken := standardSynthetic()
	broken.table("ventas").rows[0][9] = json.Number("12.34567")
	_, err = Load(context.Background(), broken.source(), mustLocation(t), nil)
	if CodeOf(err) != CodeIncompatible || strings.Contains(err.Error(), "12.34567") {
		t.Fatalf("un importe con cinco decimales: %v", err)
	}
	missing := standardSynthetic()
	missing.table("ventas").columns[5] = "fecha_apertura"
	_, err = Load(context.Background(), missing.source(), mustLocation(t), nil)
	if CodeOf(err) != CodeIncompatible || !strings.Contains(MessageOf(err), "fecha") {
		t.Fatalf("sin la columna fecha: %v", err)
	}
}

func TestCatalogCorrespondence(t *testing.T) {
	model := loadSynthetic(t, standardSynthetic())
	plan := BuildPlan(model, map[string]bool{"admin": true, "sa": true})
	catalog := plan.Catalog
	if len(catalog.Categories) != 3 || catalog.Categories[1].Name != "Platillos" {
		t.Fatalf("categorías: %+v", catalog.Categories)
	}
	products := map[int32]PlannedProduct{}
	for _, product := range catalog.Products {
		products[product.LegacyID] = product
	}
	water := products[1]
	if water.Code != "SA-1" || water.BasePrice != 300000 || water.TaxRate != 0 || !water.Active || water.PriceCanDecrease ||
		water.Description != "Nombre corto en el sistema anterior: Agua" {
		t.Fatalf("agua fresca (precio más reciente, 0 %%): %+v", water)
	}
	if soda := products[2]; soda.TaxRate != 16 || soda.RateAdjusted || soda.BasePrice != 300000 {
		t.Fatalf("refresco (16 %% en su precio más reciente): %+v", soda)
	}
	if steak := products[4]; !steak.PriceCanDecrease || steak.BasePrice != 1500000 {
		t.Fatalf("filete con complementos incluidos: %+v", steak)
	}
	if burger := products[5]; !burger.RateAdjusted || burger.TaxRate != 8 || !burger.PriceRounded || burger.BasePrice != 1100100 ||
		burger.PriceCanDecrease {
		t.Fatalf("hamburguesa (tasa ≈10 %% y tres decimales): %+v", burger)
	}
	if flan := products[6]; flan.Active || flan.HasPrice || flan.BasePrice != 0 {
		t.Fatalf("producto sin precio: %+v", flan)
	}
	if len(catalog.Complements) != 3 || catalog.Complements[0].Price != 200000 || catalog.Complements[0].TaxRate != 0 {
		t.Fatalf("complementos con su precio más reciente: %+v", catalog.Complements)
	}
	if len(catalog.Groups) != 2 || catalog.Groups[0].Name != "Complementos" || catalog.Groups[0].Key != "1,2,3" ||
		catalog.Groups[1].Name != "Complementos: Papas" || len(catalog.Groups[0].Options) != 3 ||
		catalog.Groups[0].Options[1].ReferenceValue != 150000 {
		t.Fatalf("grupos por conjunto de complementos: %+v", catalog.Groups)
	}
	if len(catalog.ProductGroups) != 2 || len(catalog.Defaults) != 2 || catalog.Defaults[0].Quantity != 1000 ||
		catalog.Defaults[0].OptionKey != "1,2,3:1" {
		t.Fatalf("grupos y cantidades incluidas: %+v %+v", catalog.ProductGroups, catalog.Defaults)
	}
	if len(catalog.Areas) != 1 || catalog.Areas[0].Name != "Salón" || len(catalog.Tables) != 3 {
		t.Fatalf("mesas sin «para llevar»: %+v %+v", catalog.Areas, catalog.Tables)
	}
	for index, table := range catalog.Tables {
		if table.X+tableWidth > 1000 || table.Y+tableHeight > 560 || table.X > 936 || table.Y > 504 {
			t.Fatalf("mesa fuera del lienzo: %+v", table)
		}
		if index == 1 && table.Seats != defaultSeats {
			t.Fatalf("mesa sin capacidad: %+v", table)
		}
		if index == 2 && table.Seats != maxSeats {
			t.Fatalf("mesa con capacidad de más: %+v", table)
		}
	}
	users := catalog.Users
	if len(users) != 3 || users[0].Username != "admin-2" || users[0].RoleCode != "administrator" || users[0].DisplayName != "Ana Pérez Ruiz" ||
		users[1].Username != "u1beto" || users[1].RoleCode != "waiter" || users[2].Username != "jose" || users[2].Active ||
		users[2].RoleCode != "waiter" {
		t.Fatalf("usuarios: %+v", users)
	}
}

func TestHistoryStatesFlagsAndLedger(t *testing.T) {
	model := loadSynthetic(t, standardSynthetic())
	plan := BuildPlan(model, map[string]bool{"admin": true})
	history := plan.History
	byID := map[int32]SalePlan{}
	for index, sale := range model.Sales {
		byID[sale.ID] = history.Sales[index]
	}
	expectFlags := func(id int32, flags ...string) {
		t.Helper()
		for _, flag := range flags {
			if !byID[id].Flags.has(flag) {
				t.Fatalf("la venta %d no tiene la marca %s: %v", id, flag, byID[id].Flags)
			}
		}
	}
	if byID[3].Status != SaleCancelled || byID[4].Status != SaleOpen || byID[1].Status != SalePaid {
		t.Fatal("estados de las ventas")
	}
	if byID[2].Mode != "takeaway" || byID[1].Mode != "dine_in" || byID[2].LinesAmount != 1550000 {
		t.Fatalf("venta 2: %+v", byID[2])
	}
	expectFlags(5, FlagTotalMismatch, FlagSubtotalMismatch)
	expectFlags(7, FlagOverpaid)
	expectFlags(8, FlagDuplicateFolio)
	expectFlags(9, FlagTaxEstimated, FlagLatePayment, FlagNoShift, FlagDuplicateFolio)
	expectFlags(10, FlagNoLines, FlagNoPayments)
	expectFlags(11, FlagUnderpaid)
	if byID[1].Flags.has(FlagTotalMismatch) || len(byID[6].Flags) != 0 {
		t.Fatalf("ventas sin incoherencias: %v %v", byID[1].Flags, byID[6].Flags)
	}
	if byID[9].Tax != 41379 || !byID[9].RecognizedAt.Equal(time.Date(2026, 1, 4, 10, 0, 0, 0, mustLocation(t))) {
		t.Fatalf("IVA estimado y fecha del último pago: %+v", byID[9])
	}
	if !byID[10].RecognizedAt.Equal(model.Sales[9].OpenedAt) {
		t.Fatal("una venta pagada sin pagos se reconoce en su apertura")
	}
	duplicates := 0
	for index, payment := range history.Payments {
		if payment.Flags.has(FlagPossibleDuplicate) {
			duplicates++
			if model.Payments[index].ID != 6 {
				t.Fatalf("pago repetido equivocado: %d", model.Payments[index].ID)
			}
		}
		if model.Payments[index].Type.Is(2) && (payment.Method != MethodCard || payment.MethodName != MethodCardName) {
			t.Fatal("forma de pago de tarjeta")
		}
	}
	if duplicates != 1 {
		t.Fatalf("pagos posiblemente repetidos: %d", duplicates)
	}
	shiftFlags := map[int32]flagSet{}
	for index, shift := range model.Shifts {
		shiftFlags[shift.ID] = history.Shifts[index].Flags
	}
	if len(shiftFlags[1]) != 0 || !shiftFlags[2].has(FlagSalesMismatch) || !shiftFlags[2].has(FlagWithdrawalsMismatch) ||
		history.Shifts[2].Status != "open" {
		t.Fatalf("cortes: %v", shiftFlags)
	}
	if !history.Withdrawals[1].Flags.has(FlagZeroOrNegative) || !history.Withdrawals[2].Flags.has(FlagDoubtfulAmount) {
		t.Fatal("retiros dudosos")
	}
	// El libro: por venta, Σ net = lo que se debía y Σ tax = el IVA de la venta, exactos; nada de canceladas ni abiertas.
	net := map[int]Money{}
	tax := map[int]Money{}
	kinds := map[string]int{}
	err := plan.Ledger(func(entry LedgerEntry) error {
		if entry.Net != entry.Gross-entry.Discount+entry.Charge || entry.Net != entry.Base+entry.Tax {
			t.Fatalf("renglón del libro incoherente: %+v", entry)
		}
		net[entry.SaleIndex] += entry.Net
		tax[entry.SaleIndex] += entry.Tax
		kinds[entry.Kind]++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for index, sale := range history.Sales {
		if sale.Status != SalePaid {
			if _, found := net[index]; found {
				t.Fatalf("la venta %d no es venta y está en el libro", model.Sales[index].ID)
			}
			continue
		}
		if net[index] != sale.Due || tax[index] != sale.Tax {
			t.Fatalf("libro de la venta %d: net %s (debe %s), IVA %s (venta %s)", model.Sales[index].ID, net[index], sale.Due,
				tax[index], sale.Tax)
		}
	}
	if byID[6].Due != 675000 || kinds[LedgerDiscount] != 1 || kinds[LedgerCharge] != 1 || kinds[LedgerDifference] != 1 {
		t.Fatalf("descuentos, cargos y diferencias del libro: %v, debe %s", kinds, byID[6].Due)
	}
}

func TestExpectedIsIndependent(t *testing.T) {
	model := loadSynthetic(t, standardSynthetic())
	location := mustLocation(t)
	expected := computeExpected(model, func(instant time.Time) int {
		return instant.In(location).Year()
	})
	if expected.Counts[CountSalesPaid] != 9 || expected.Counts[CountSalesCancelled] != 1 || expected.Counts[CountSalesOpen] != 1 ||
		expected.Counts[CountTables] != 3 {
		t.Fatalf("conteos: %v", expected.Counts)
	}
	sales := map[int]string{2019: "145.5000", 2024: "222.5000", 2025: "130.0000", 2026: "85.5000"}
	for year, amount := range sales {
		if expected.SalesByYear[year].String() != amount {
			t.Fatalf("venta de %d: %s (se esperaba %s)", year, expected.SalesByYear[year], amount)
		}
	}
	if expected.PaymentsByMethodYear[MethodCard][2024].String() != "155.0000" ||
		expected.PaymentsByMethodYear[MethodCash][2026].String() != "60.0000" {
		t.Fatalf("pagos por forma y año: %v", expected.PaymentsByMethodYear)
	}
	if expected.CancelledLegacyStyle.String() != "105.5000" || expected.WithdrawalsTotal.String() != "150200.0000" {
		t.Fatalf("canceladas al estilo del anterior y retiros: %s %s", expected.CancelledLegacyStyle, expected.WithdrawalsTotal)
	}
}
