package payments

import (
	"strings"
	"testing"
)

func textPointer(value string) *string {
	return &value
}

// La precuenta y el ticket usan el formato único de importes: miles, dos decimales y el signo antes
// del símbolo. Un descuento se imprime «-$50.00» y nunca «$-50.00» (etapa 7.1).
func TestCommercialDocumentUsesSharedMoneyFormat(t *testing.T) {
	document := &CommercialDocument{
		Kind: "receipt", Establishment: "ADEL POS", Folio: 21, AccountName: "Mesa 4", Context: "Mesa 4",
		LocalTime: "09/11/2026 14:00", Cashier: "Cajera",
		Lines:       []DocumentLine{{Quantity: 2, Name: "Corte de res", Amount: "1250.00"}},
		Adjustments: []DocumentAdjustment{{Concept: "Descuento", Amount: "-50.00"}, {Concept: "Servicio", Amount: "1000.00"}},
		Tax:         []TaxLine{{Rate: 16, Base: "1896.55", Tax: "303.45", Total: "2200.00"}},
		Total:       "2200.00",
		Payments:    []DocumentPayment{{Method: "Efectivo", Amount: "2200.00", Tip: "0.00", Change: "0.00"}},
		Paid:        "2200.00", Tips: "0.00", Balance: "0.00",
		Payment: &DocumentPayment{
			Method: "Efectivo", Amount: "2200.00", Tip: "100.00", Received: textPointer("2500.00"), Change: "200.00",
		},
	}
	text := renderDocument(document, 42)
	for _, fragment := range []string{
		"$1,250.00", "-$50.00", "$1,000.00", "$1,896.55", "$303.45", "$2,200.00", "$2,500.00", "$200.00", "$100.00",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("El ticket no muestra %q:\n%s", fragment, text)
		}
	}
	for _, unformatted := range []string{"$-50.00", "$1250.00", "$2200.00", "$1000.00"} {
		if strings.Contains(text, unformatted) {
			t.Fatalf("El ticket conserva el formato anterior %q:\n%s", unformatted, text)
		}
	}
}

// El efectivo con dólares imprime cada moneda con su símbolo y el tipo de cambio en pesos por
// dólar.
func TestCashWithDollarsUsesSharedMoneyFormat(t *testing.T) {
	payment := DocumentPayment{
		Method: "Efectivo", Amount: "1850.00", Tip: "0.00", Received: textPointer("1942.50"), Change: "92.50",
		ReceivedMXN: textPointer("1000.00"), ReceivedUSD: textPointer("50.95"), USDRate: textPointer("18.50"),
		ChangeCurrency: textPointer(currencyUSD), ChangeUSD: textPointer("5.00"),
	}
	text := strings.Join(cashWithDollars(payment, 42), "\n")
	for _, fragment := range []string{"$1,000.00", "US$50.95", "$18.50/US$", "$1,942.50", "US$5.00", "$92.50"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("El detalle en dólares no muestra %q:\n%s", fragment, text)
		}
	}
}

// Los mensajes con importes usan el mismo formato que los documentos.
func TestMessageAmountsUseSharedMoneyFormat(t *testing.T) {
	if formatPesos(amount("1234.5")) != "$1,234.50" || formatDollars(amount("2.7")) != "US$2.70" {
		t.Fatal("Los mensajes no usan el formato único de importes")
	}
	if rateText("18.50") != "$18.50/US$" {
		t.Fatalf("Tipo de cambio mal expresado: %s", rateText("18.50"))
	}
}

// La línea de regularizaciones del corte concuerda con el número.
func TestReportRegularizationsAgreeWithCount(t *testing.T) {
	for _, item := range []struct {
		count int
		want  string
	}{
		{1, "1 regularización de un turno cerrado; no cambia este turno."},
		{2, "2 regularizaciones de turnos cerrados; no cambian este turno."},
	} {
		source := finalReportSource()
		source.report.Snapshot.Accumulated.Regularizations = item.count
		document := &documentLines{width: 80}
		renderReport(document, testHeader(t), source, false)
		if !strings.Contains(document.String(), item.want) {
			t.Fatalf("El corte no muestra %q:\n%s", item.want, document.String())
		}
	}
}
