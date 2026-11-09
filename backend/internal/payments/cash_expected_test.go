package payments

import (
	"testing"

	"github.com/shopspring/decimal"
)

func rateOf(value string) *decimal.Decimal {
	rate := amount(value)
	return &rate
}

// El esperado suma cada moneda por separado: fondo, efecto físico de los cobros y operaciones.
func TestExpectationSeparatesCurrencies(t *testing.T) {
	flows := cashFlows{
		floatMXN:         amount("1000"),
		floatUSD:         amount("50"),
		paymentsMXN:      amount("15"),
		paymentsUSD:      amount("7.30"),
		operationsInMXN:  amount("200"),
		operationsOutMXN: amount("150.50"),
		operationsInUSD:  amount("20"),
		operationsOutUSD: amount("20"),
	}
	expected := flows.expectation(rateOf("18.50"))
	if money(expected.mxn) != "1064.50" || money(expected.usd) != "57.30" {
		t.Fatalf("esperado por moneda incorrecto: %s MXN, %s USD", money(expected.mxn), money(expected.usd))
	}
	public := expected.public()
	// US$57.30 × 18.50 = 1060.05; el equivalente nunca sustituye las cantidades físicas.
	if public.MXN != "1064.50" || public.USD != "57.30" || *public.USDRate != "18.50" ||
		*public.USDEquivalentMXN != "1060.05" || *public.TotalEquivalentMXN != "2124.55" {
		t.Fatalf("equivalencias incorrectas: %+v", public)
	}
	if money(expected.amount(currencyUSD)) != "57.30" || money(expected.amount(currencyMXN)) != "1064.50" {
		t.Fatal("amount no devuelve el esperado de la moneda indicada")
	}
}

// Sin tipo de cambio no se expresa ninguna equivalencia: solo las cantidades físicas.
func TestExpectationWithoutRateHasNoEquivalent(t *testing.T) {
	expected := cashFlows{floatMXN: amount("500"), paymentsMXN: amount("-20")}.expectation(nil)
	public := expected.public()
	if public.MXN != "480.00" || public.USD != "0.00" || public.USDRate != nil || public.USDEquivalentMXN != nil ||
		public.TotalEquivalentMXN != nil {
		t.Fatalf("un turno solo en pesos expresó equivalencias: %+v", public)
	}
}

// La diferencia es contado − esperado por moneda: sobrante positivo, faltante negativo.
func TestDifferencesPerCurrency(t *testing.T) {
	expected := cashExpectation{mxn: amount("5000"), usd: amount("57.30"), rate: rateOf("18.50")}
	cases := []struct {
		counted, countedUSD string
		mxn, usd            string
		resultMXN           string
		resultUSD           string
		equivalent          string
	}{
		{"5000", "57.30", "0.00", "0.00", resultEven, resultEven, "0.00"},
		{"4850", "57.30", "-150.00", "0.00", resultShort, resultEven, "-150.00"},
		{"5020.50", "67.30", "20.50", "10.00", resultOver, resultOver, "205.50"},
		{"5000", "47.30", "0.00", "-10.00", resultEven, resultShort, "-185.00"},
		// −US$0.03 × 18.50 = −0.555, redondeado mitad alejándose de cero a −0.56.
		{"5000", "57.27", "0.00", "-0.03", resultEven, resultShort, "-0.56"},
	}
	for _, item := range cases {
		differences := computeDifferences(amount(item.counted), amount(item.countedUSD), expected).public()
		if differences.MXN != item.mxn || differences.USD != item.usd || differences.ResultMXN != item.resultMXN ||
			differences.ResultUSD != item.resultUSD || differences.EquivalentMXN == nil || *differences.EquivalentMXN != item.equivalent {
			t.Fatalf("contado %s/%s: diferencias incorrectas %+v", item.counted, item.countedUSD, differences)
		}
	}
	withoutRate := computeDifferences(amount("90"), zero, cashExpectation{mxn: amount("100")}).public()
	if withoutRate.MXN != "-10.00" || withoutRate.ResultMXN != resultShort || withoutRate.EquivalentMXN != nil {
		t.Fatalf("sin tipo de cambio no debe haber equivalencia: %+v", withoutRate)
	}
}

func TestDifferenceResult(t *testing.T) {
	if differenceResult(amount("0.01")) != resultOver || differenceResult(amount("-0.01")) != resultShort ||
		differenceResult(zero) != resultEven {
		t.Fatal("la clasificación de diferencias no distingue sobrante, faltante y sin diferencia")
	}
}

// Lo ocurrido desde el corte anterior es el acumulado actual menos el anterior.
func TestShiftTotalsMinusPrevious(t *testing.T) {
	previous := shiftTotals{
		Payments: 2, CashOperations: 1, Sales: "100.00", Tips: "5.00", Refunded: "0.00", Discounts: "10.00", Courtesies: "0.00",
		Methods: []methodTotals{{MethodName: "Efectivo", IsCash: true, Sales: "100.00", Tips: "5.00", Refunds: "0.00", Net: "105.00"}},
		Cash:    []currencyOperations{{Currency: currencyMXN, Income: "50.00", Withdrawals: "0.00", Expenses: "0.00", CorrectionsIn: "0.00", CorrectionsOut: "0.00"}},
	}
	current := shiftTotals{
		Payments: 3, CashOperations: 3, Sales: "160.00", Tips: "5.00", Refunded: "20.00", Discounts: "10.00", Courtesies: "0.00",
		Methods: []methodTotals{
			{MethodName: "Efectivo", IsCash: true, Sales: "100.00", Tips: "5.00", Refunds: "20.00", Net: "85.00"},
			{MethodName: "Tarjeta de débito", IsCash: false, Sales: "60.00", Tips: "0.00", Refunds: "0.00", Net: "60.00"},
		},
		Cash: []currencyOperations{
			{Currency: currencyMXN, Income: "80.00", Withdrawals: "300.00", Expenses: "0.00", CorrectionsIn: "0.00", CorrectionsOut: "0.00"},
		},
	}
	delta := current.minus(previous)
	if delta.Payments != 1 || delta.CashOperations != 2 || delta.Sales != "60.00" || delta.Refunded != "20.00" || delta.Discounts != "0.00" {
		t.Fatalf("totales desde el corte anterior incorrectos: %+v", delta)
	}
	if len(delta.Methods) != 2 || delta.Methods[0].Net != "-20.00" || delta.Methods[1].Sales != "60.00" {
		t.Fatalf("métodos desde el corte anterior incorrectos: %+v", delta.Methods)
	}
	if delta.Cash[0].Income != "30.00" || delta.Cash[0].Withdrawals != "300.00" {
		t.Fatalf("operaciones desde el corte anterior incorrectas: %+v", delta.Cash)
	}
}
