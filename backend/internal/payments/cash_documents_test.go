package payments

import (
	"strings"
	"testing"
	"time"
)

func testHeader(t *testing.T) *documentHeader {
	t.Helper()
	location, err := time.LoadLocation("America/Monterrey")
	if err != nil {
		t.Fatal(err)
	}
	return &documentHeader{establishment: "ADEL POS", location: location}
}

func finalReportSource() cashDocumentSource {
	closedBy := "Cajera"
	opened := time.Date(2026, 11, 9, 15, 0, 0, 0, time.UTC)
	differences := CashDifferences{MXN: "-100.00", USD: "0.00", ResultMXN: resultShort, ResultUSD: resultEven}
	report := &CashReport{
		ID: "corte", Kind: "final", Sequence: 1, CoveredFrom: opened, CoveredTo: opened.Add(8 * time.Hour),
		CreatedByName: "Cajera", DeviceName: "Terminal 1",
		Snapshot: reportSnapshot{
			Control: reportControl{RegisterName: "Caja 1", BusinessDate: "2026-11-09", OpenedByName: "Cajera", OpeningFloat: "500.00"},
			Accumulated: shiftTotals{
				Payments: 2, Sales: "300.00", Tips: "20.00", Refunded: "50.00", Discounts: "10.00", DiscountCount: 1,
				Courtesies: "0.00", Methods: []methodTotals{
					{MethodName: "Efectivo", IsCash: true, Sales: "200.00", Tips: "20.00", Refunds: "50.00", Net: "170.00"},
					{MethodName: "Tarjeta", Sales: "100.00", Tips: "0.00", Refunds: "0.00", Net: "100.00"},
				},
				Cash: []currencyOperations{{Currency: currencyMXN, Income: "100.00", Withdrawals: "200.00", Expenses: "30.00",
					CorrectionsIn: "0.00", CorrectionsOut: "0.00"}},
			},
			Declared: &declaredCash{CountedMXN: "440.00", CountedUSD: "0.00", ExtraMXN: "0.00", ExtraUSD: "0.00",
				CashierNotes: "Faltan monedas"},
			Sensitive: &sensitiveCash{Expected: CashExpectation{MXN: "540.00", USD: "0.00"}, Differences: &differences},
		},
	}
	shift := &Shift{RegisterName: "Caja 1", BusinessDate: "2026-11-09", OpenedAt: opened, State: shiftClosed, ClosedByName: &closedBy}
	return cashDocumentSource{kind: documentFinalCut, shift: shift, report: report}
}

// El corte definitivo contiene lo que pide §20 y el esperado y las diferencias solo si se incluyen.
func TestFinalCutDocumentShowsExpectedOnlyWhenIncluded(t *testing.T) {
	source := finalReportSource()
	document := &documentLines{width: 42}
	renderReport(document, testHeader(t), source, true)
	text := document.String()
	for _, fragment := range []string{
		"Caja: Caja 1", "Turno del 09/11/2026", "Apertura", "09/11/2026 09:00", "Cierre", "Cerró: Cajera", "Fondo inicial",
		"VENTAS Y PAGOS POR MÉTODO", "Efectivo", "Tarjeta", "Devoluciones", "Propinas (no son venta)", "Descuentos (1)",
		"Cortesías (0)", "Ingresos", "Retiros", "Gastos", "EFECTIVO DECLARADO", "$440.00", "EFECTIVO ESPERADO", "$540.00",
		"Pesos · Faltante", "-$100.00", "OBSERVACIONES", "Faltan monedas",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("El corte definitivo no muestra %q:\n%s", fragment, text)
		}
	}
	for _, line := range document.lines {
		if len([]rune(line)) > 42 {
			t.Fatalf("La línea excede el ancho del papel: %q", line)
		}
	}
	hidden := &documentLines{width: 42}
	renderReport(hidden, testHeader(t), source, false)
	text = hidden.String()
	if strings.Contains(text, "$540.00") || strings.Contains(text, "-$100.00") || strings.Contains(text, "Faltante") ||
		!strings.Contains(text, "no incluidos") || !strings.Contains(text, "$440.00") {
		t.Fatalf("El corte sin permiso reveló el esperado o perdió lo declarado:\n%s", text)
	}
}

// Un comprobante de retiro identifica caja, turno, importe, autorización y firma, sin esperado.
func TestWithdrawalVoucherContent(t *testing.T) {
	label := "Resguardo"
	authorizer := "Encargada"
	mode := "supervisor"
	operation := &CashOperation{
		Kind: operationWithdrawal, Currency: currencyUSD, Amount: "20.00", Concept: "Resguardo", TypeLabel: &label,
		RegisterName: "Caja 1", USDRate: optionalMoney(amount("18.50")), EquivalentMXN: "370.00", Reference: "Sobre 4",
		ActorName: "Cajera", DeviceName: "Terminal 1", AuthorizedByName: &authorizer, AuthorizationMode: &mode,
		CreatedAt: time.Date(2026, 11, 9, 18, 30, 0, 0, time.UTC), Status: "recorded",
	}
	source := cashDocumentSource{kind: operationWithdrawal, shift: &Shift{BusinessDate: "2026-11-09"}, operation: operation}
	document := &documentLines{width: 30}
	document.text(source.title())
	renderVoucher(document, testHeader(t), source)
	text := document.String()
	for _, fragment := range []string{
		"COMPROBANTE DE RETIRO", "Caja: Caja 1", "Turno del 09/11/2026", "09/11/2026 12:30", "Motivo: Resguardo", "US$20.00",
		"$18.50/US$", "$370.00", "Referencia: Sobre 4", "Registró: Cajera", "Autorizó: Encargada", "Autorización de supervisor", "Firma",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("El comprobante no muestra %q:\n%s", fragment, text)
		}
	}
	if strings.Contains(strings.ToLower(text), "esperado") {
		t.Fatal("Un comprobante de operación mencionó el efectivo esperado")
	}
}

func TestDocumentFormatting(t *testing.T) {
	if currencyText(currencyMXN, "-5.00") != "-$5.00" || currencyText(currencyUSD, "7.30") != "US$7.30" ||
		currencyText(currencyMXN, "0") != "$0.00" || !zeroAmount("0") || zeroAmount("0.50") {
		t.Fatal("Formato de importes incorrecto")
	}
	// Etapa 7.1: comprobantes y cortes usan el separador de miles del formato único.
	if currencyText(currencyMXN, "-1500.00") != "-$1,500.00" || currencyText(currencyUSD, "12345.6") != "US$12,345.60" {
		t.Fatal("Los documentos de caja no separan los miles")
	}
	if displayDate("2026-11-09") != "09/11/2026" || displayDate("sin fecha") != "sin fecha" {
		t.Fatal("Formato de fecha comercial incorrecto")
	}
	if documentTitle(documentPartialCut, 3) != "CORTE PARCIAL #3" || documentTitle(operationExpense, 0) != "COMPROBANTE DE GASTO" {
		t.Fatal("Títulos de documento incorrectos")
	}
}
