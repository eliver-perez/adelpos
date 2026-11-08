package payments

import (
	"testing"

	"github.com/shopspring/decimal"
)

func amounts(values ...string) []decimal.Decimal {
	result := make([]decimal.Decimal, len(values))
	for index, value := range values {
		result[index] = decimal.RequireFromString(value)
	}
	return result
}

func TestParseAmountAcceptsOnlyExactMoney(t *testing.T) {
	for _, valid := range []string{"0", "0.5", "10", "10.25", "9999999999.99"} {
		if _, err := parseAmount(valid, "prueba"); err != nil {
			t.Fatalf("Se rechazó un importe válido %q", valid)
		}
	}
	for _, rejected := range []string{"", "-1", "+1", "1.234", "1..2", "1.2.3", ".5", "1e3", " 1", "1,00", "abc", "01", "10000000000"} {
		if _, err := parseAmount(rejected, "prueba"); err == nil {
			t.Fatalf("Se aceptó un importe inválido %q", rejected)
		}
	}
}

func TestAllocatePreservesEveryCent(t *testing.T) {
	cases := []struct {
		total   string
		weights []string
		want    []string
	}{
		{"100.00", []string{"50", "50"}, []string{"50.00", "50.00"}},
		{"10.00", []string{"1", "1", "1"}, []string{"3.34", "3.33", "3.33"}},
		{"0.01", []string{"1", "1"}, []string{"0.01", "0.00"}},
		{"30.00", []string{"150", "50"}, []string{"22.50", "7.50"}},
		{"7.77", []string{"0", "0", "0"}, []string{"2.59", "2.59", "2.59"}},
		{"100.00", []string{"33.33", "33.33", "33.34"}, []string{"33.33", "33.33", "33.34"}},
	}
	for _, testCase := range cases {
		result := allocate(decimal.RequireFromString(testCase.total), amounts(testCase.weights...))
		sum := decimal.Zero
		for index, value := range result {
			sum = sum.Add(value)
			if money(value) != testCase.want[index] {
				t.Fatalf("Reparto de %s entre %v: posición %d = %s, se esperaba %s", testCase.total, testCase.weights, index, money(value), testCase.want[index])
			}
		}
		if !sum.Equal(decimal.RequireFromString(testCase.total)) {
			t.Fatalf("El reparto de %s perdió o duplicó centavos: %s", testCase.total, money(sum))
		}
	}
}

func TestTaxBreakdownIsExactForEveryRate(t *testing.T) {
	for _, total := range []string{"0.01", "0.03", "1.00", "99.99", "116.00", "123.45", "100000.07"} {
		for _, rate := range allowedTaxRates {
			lines := taxBreakdown(map[int]decimal.Decimal{rate: decimal.RequireFromString(total)})
			base := decimal.RequireFromString(lines[0].Base)
			tax := decimal.RequireFromString(lines[0].Tax)
			if !base.Add(tax).Equal(decimal.RequireFromString(total)) {
				t.Fatalf("Base + IVA distinto del total %s con tasa %d", total, rate)
			}
			if rate == 0 && !tax.IsZero() {
				t.Fatalf("La tasa 0 %% generó IVA en %s", total)
			}
		}
	}
	lines := taxBreakdown(map[int]decimal.Decimal{16: decimal.RequireFromString("116.00")})
	if lines[0].Base != "100.00" || lines[0].Tax != "16.00" {
		t.Fatalf("Desglose inesperado de $116 al 16 %%: %+v", lines[0])
	}
	lines = taxBreakdown(map[int]decimal.Decimal{8: decimal.RequireFromString("108.00")})
	if lines[0].Base != "100.00" || lines[0].Tax != "8.00" {
		t.Fatalf("Desglose inesperado de $108 al 8 %%: %+v", lines[0])
	}
}

func TestTaxRateListIsClosed(t *testing.T) {
	for _, rate := range []int{16, 8, 0} {
		if !validTaxRate(rate) {
			t.Fatalf("Se rechazó la tasa permitida %d", rate)
		}
	}
	for _, rate := range []int{15, 1, -8, 160, 100} {
		if validTaxRate(rate) {
			t.Fatalf("Se aceptó la tasa no permitida %d", rate)
		}
	}
}

func TestPercentOfNeverUnderstatesADiscount(t *testing.T) {
	if percentOf(decimal.RequireFromString("10"), decimal.RequireFromString("30")).String() != "33.34" {
		t.Fatal("El porcentaje de un importe fijo se redondeó hacia abajo")
	}
	if percentOf(decimal.RequireFromString("5"), decimal.Zero).String() != "100" {
		t.Fatal("Un descuento sobre base cero debe evaluarse como 100 %")
	}
}
