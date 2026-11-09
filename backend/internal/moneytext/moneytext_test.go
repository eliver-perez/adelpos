package moneytext

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/shopspring/decimal"
)

// formatCase es una fila de la tabla compartida con Vitest (testdata/cases.json).
type formatCase struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
	Printed  string `json:"printed"`
}

type caseTable struct {
	Format       []formatCase `json:"format"`
	Signed       []formatCase `json:"signed"`
	Denomination []formatCase `json:"denomination"`
}

func readCases(t *testing.T) caseTable {
	t.Helper()
	content, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var table caseTable
	if err := json.Unmarshal(content, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Format) == 0 || len(table.Signed) == 0 || len(table.Denomination) == 0 {
		t.Fatal("La tabla compartida de casos está incompleta")
	}
	return table
}

func checkCases(t *testing.T, name string, cases []formatCase, format func(currency, amount string) string) {
	t.Helper()
	for _, item := range cases {
		got := format(item.Currency, item.Amount)
		if got != item.Printed {
			t.Errorf("%s(%q, %q) = %q; se esperaba %q", name, item.Currency, item.Amount, got, item.Printed)
		}
	}
}

// Las tres funciones de texto cumplen la misma tabla que la interfaz.
func TestSharedCaseTable(t *testing.T) {
	table := readCases(t)
	checkCases(t, "Format", table.Format, Format)
	checkCases(t, "FormatSigned", table.Signed, FormatSigned)
	checkCases(t, "FormatDenomination", table.Denomination, FormatDenomination)
}

// FormatDecimal fija dos decimales como los importes de la API y después aplica Format.
func TestFormatDecimal(t *testing.T) {
	cases := []struct {
		currency string
		value    decimal.Decimal
		want     string
	}{
		{CurrencyMXN, decimal.RequireFromString("1000"), "$1,000.00"},
		{CurrencyMXN, decimal.RequireFromString("-50.5"), "-$50.50"},
		{CurrencyUSD, decimal.RequireFromString("2.7"), "US$2.70"},
		{CurrencyMXN, decimal.RequireFromString("1060.055"), "$1,060.06"},
		{CurrencyMXN, decimal.Zero, "$0.00"},
	}
	for _, item := range cases {
		if got := FormatDecimal(item.currency, item.value); got != item.want {
			t.Errorf("FormatDecimal(%s, %s) = %q; se esperaba %q", item.currency, item.value, got, item.want)
		}
	}
}

// Una cadena con más dígitos de los que representa un float64 se agrupa sin perder precisión.
func TestLargeAmountsKeepEveryDigit(t *testing.T) {
	amount := "123456789012345678901234567890.12"
	want := "$123,456,789,012,345,678,901,234,567,890.12"
	if got := Format(CurrencyMXN, amount); got != want {
		t.Fatalf("Format(%q) = %q; se esperaba %q", amount, got, want)
	}
}
