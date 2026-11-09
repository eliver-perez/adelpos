package payments

import (
	"testing"

	"github.com/shopspring/decimal"
)

func amount(value string) decimal.Decimal {
	return decimal.RequireFromString(value)
}

// Cada conversión se redondea a centavos con mitad alejándose de cero.
func TestCurrencyConversionsRoundHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		usd, rate, want string
	}{
		{"20", "18.50", "370.00"},
		{"22.22", "18", "399.96"},
		{"0.03", "18.50", "0.56"},
		{"0.01", "18.25", "0.18"},
		{"100", "19.99", "1999.00"},
	}
	for _, item := range cases {
		if got := money(usdToMXN(amount(item.usd), amount(item.rate))); got != item.want {
			t.Fatalf("%s USD a %s: esperado %s, obtenido %s", item.usd, item.rate, item.want, got)
		}
	}
	reverse := []struct {
		mxn, rate, want string
	}{
		{"50", "18", "2.78"},
		{"0.09", "18", "0.01"},
		{"400", "18", "22.22"},
		{"60", "18.50", "3.24"},
		{"0", "18", "0.00"},
	}
	for _, item := range reverse {
		if got := money(mxnToUSD(amount(item.mxn), amount(item.rate))); got != item.want {
			t.Fatalf("$%s a dólares con %s: esperado %s, obtenido %s", item.mxn, item.rate, item.want, got)
		}
	}
}

// El faltante en dólares es el mínimo que, convertido, cubre el faltante en pesos.
func TestUSDToCoverAlwaysCovers(t *testing.T) {
	for _, item := range []struct{ mxn, rate, want string }{
		{"400", "18", "22.23"},
		{"36", "18", "2.00"},
		{"0.04", "18", "0.01"},
		{"0", "18", "0.00"},
	} {
		cover := usdToCover(amount(item.mxn), amount(item.rate))
		if money(cover) != item.want {
			t.Fatalf("Faltante de $%s: esperado US$%s, obtenido US$%s", item.mxn, item.want, money(cover))
		}
		if usdToMXN(cover, amount(item.rate)).LessThan(amount(item.mxn)) {
			t.Fatalf("US$%s no cubre $%s", money(cover), item.mxn)
		}
	}
}

// Pesos y dólares en la misma operación, con el cambio expresado en ambas monedas.
func TestSettleCashWithUSDMixesCurrenciesAndKeepsExactChange(t *testing.T) {
	rate := amount("18.50")
	mixed := settleCashWithUSD(amount("400"), amount("100"), amount("20"), rate, currencyMXN)
	if !mixed.covered || money(mixed.receivedUSDMXN) != "370.00" || money(mixed.receivedTotal) != "470.00" {
		t.Fatalf("La conversión de lo recibido no es exacta: %+v", mixed)
	}
	if money(mixed.change) != "70.00" || money(mixed.changeUSD) != "3.78" || mixed.changeCurrency != currencyMXN {
		t.Fatalf("El cambio no se expresó en ambas monedas: %+v", mixed)
	}
	physical := mixed.physical(rate)
	if money(physical.mxnIn) != "100.00" || money(physical.usdIn) != "20.00" || money(physical.mxnOut) != "70.00" || physical.usdOut.Sign() != 0 {
		t.Fatalf("El efecto físico en pesos no corresponde: %+v", physical)
	}
	inDollars := settleCashWithUSD(amount("400"), amount("0"), amount("25"), rate, currencyUSD)
	if money(inDollars.change) != "62.50" || money(inDollars.changeUSD) != "3.38" || inDollars.changeCurrency != currencyUSD {
		t.Fatalf("El cambio en dólares no se calculó: %+v", inDollars)
	}
	dollars := inDollars.physical(rate)
	if dollars.mxnOut.Sign() != 0 || money(dollars.usdOut) != "3.38" || money(dollars.usdIn) != "25.00" {
		t.Fatalf("El cambio en dólares no salió del cajón de dólares: %+v", dollars)
	}
	reversed := dollars.reversed()
	if money(reversed.usdOut) != "25.00" || money(reversed.usdIn) != "3.38" || reversed.mxnIn.Sign() != 0 {
		t.Fatalf("El reverso no invirtió los movimientos físicos: %+v", reversed)
	}
	exact := settleCashWithUSD(amount("370"), amount("0"), amount("20"), rate, currencyUSD)
	if !exact.covered || exact.change.Sign() != 0 || exact.changeCurrency != currencyMXN {
		t.Fatalf("Sin cambio, la moneda del cambio debe quedar en pesos: %+v", exact)
	}
	short := settleCashWithUSD(amount("400"), amount("0"), amount("21.62"), rate, currencyMXN)
	if short.covered || money(short.missing) != "0.03" {
		t.Fatalf("Un efectivo insuficiente se dio por cubierto: %+v", short)
	}
}

func TestParseRateAndDifference(t *testing.T) {
	for _, valid := range []string{"19.50", "18", "0.5", "999999.99"} {
		if _, err := parseRate(valid); err != nil {
			t.Fatalf("Se rechazó un tipo de cambio válido %q", valid)
		}
	}
	for _, rejected := range []string{"", "0", "0.00", "-18", "19.505", "19,50", "1e2", "1000000", " 19"} {
		if _, err := parseRate(rejected); err == nil {
			t.Fatalf("Se aceptó un tipo de cambio inválido %q", rejected)
		}
	}
	if got := money(rateDifference(amount("19.50"), amount("1.95"))); got != "900.00" {
		t.Fatalf("Diferencia porcentual incorrecta: %s", got)
	}
	if got := money(rateDifference(amount("18.00"), amount("20.00"))); got != "10.00" {
		t.Fatalf("Diferencia porcentual incorrecta: %s", got)
	}
}
