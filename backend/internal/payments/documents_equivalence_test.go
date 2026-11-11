package payments

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/documents"
)

// Equivalencia exacta de la etapa 9B: las plantillas predeterminadas de la precuenta, el ticket,
// los tres comprobantes y los dos cortes producen, byte a byte, el mismo texto que las plantillas
// anteriores (copias literales en legacy_documents_test.go) para 58 y 80 mm y una matriz amplia de
// casos.

// commercialVariants son precuentas y tickets con los casos que cambian el formato.
func commercialVariants() []*CommercialDocument {
	longName := strings.Repeat("Corte de res a la parrilla ", 3)
	lines := []DocumentLine{
		{Quantity: 2, Name: "Corte de res", Notes: "Término medio\x1b\x40\x1d\x56\x00 sin sal\tni pimienta", Amount: "1250.00"},
		{Quantity: 12, Name: longName, Amount: "123456.78"},
		{Quantity: 1, Name: "Refresco", Notes: "   ", Amount: "25.00"},
	}
	adjustments := []DocumentAdjustment{
		{Concept: "Descuento", Reason: "Atención", Amount: "-185.00"},
		{Concept: "Cortesía", Reason: "Cortesía de la casa con un motivo muy largo para cortar", Amount: "-50.00"},
		{Concept: "Servicio a domicilio", Amount: "30.00"},
	}
	taxes := []TaxLine{
		{Rate: 16, Base: "1896.55", Tax: "303.45", Total: "2200.00"},
		{Rate: 8, Base: "92.59", Tax: "7.41", Total: "100.00"},
		{Rate: 0, Base: "50.00", Tax: "0.00", Total: "50.00"},
	}
	cash := DocumentPayment{Method: "Efectivo", Amount: "2200.00", Tip: "100.00", Received: textPointer("2500.00"), Change: "200.00"}
	card := DocumentPayment{Method: "Tarjeta de débito", Amount: "150.00", Tip: "0.00", Reference: "AUT-123456\x07", Change: "0.00"}
	dollarsUSD := DocumentPayment{
		Method: "Efectivo", Amount: "1850.00", Tip: "0.00", Received: textPointer("1942.50"), Change: "92.50",
		ReceivedMXN: textPointer("1000.00"), ReceivedUSD: textPointer("50.95"), USDRate: textPointer("18.50"),
		ChangeCurrency: textPointer(currencyUSD), ChangeUSD: textPointer("5.00"),
	}
	dollarsMXN := dollarsUSD
	dollarsMXN.ChangeCurrency = textPointer(currencyMXN)
	dollarsMXN.ChangeUSD = nil
	dollarsMXN.Received = nil
	base := func(kind string) *CommercialDocument {
		return &CommercialDocument{
			Kind: kind, Establishment: "Restaurante de prueba", Folio: 21, AccountName: "Mesa 4", Context: "Mesa 4",
			LocalTime: "09/11/2026 14:00", Cashier: "Cajera", Lines: []DocumentLine{}, Adjustments: []DocumentAdjustment{},
			Tax: []TaxLine{}, Total: "0.00", Payments: []DocumentPayment{}, Paid: "0.00", Tips: "0.00", Balance: "0.00",
		}
	}
	variants := []*CommercialDocument{}
	for _, kind := range []string{"precheck", "receipt"} {
		for _, payment := range []*DocumentPayment{nil, &cash, &card, &dollarsUSD, &dollarsMXN} {
			for _, mixed := range []bool{false, true} {
				for _, pending := range []bool{false, true} {
					for _, usd := range []bool{false, true} {
						document := base(kind)
						document.Lines = lines
						document.Adjustments = adjustments
						document.Tax = taxes
						document.Total = "2350.00"
						if payment != nil && kind == "receipt" {
							copied := *payment
							document.Payment = &copied
						}
						if mixed {
							document.Payments = []DocumentPayment{cash, card, dollarsUSD}
							document.Paid = "4200.00"
							document.Tips = "100.00"
						}
						if pending {
							document.Balance = "150.50"
						}
						if usd {
							document.USDTotal = textPointer("127.03")
							document.USDRate = textPointer("18.50")
						}
						variants = append(variants, document)
					}
				}
			}
		}
		// Documentos mínimos, con datos vacíos y con nombres largos que cortan.
		empty := base(kind)
		variants = append(variants, empty)
		long := base(kind)
		long.Establishment = "Restaurante con un nombre comercial larguísimo que corta"
		long.AccountName = "Pedido para llevar de un cliente con nombre largo"
		long.Context = "Para llevar"
		long.Cashier = "Cajera con nombre y apellidos largos para cortar"
		long.Folio = 1234567
		variants = append(variants, long)
	}
	return variants
}

func TestDefaultCommercialTemplatesMatchLegacyRenderer(t *testing.T) {
	cases := 0
	for index, document := range commercialVariants() {
		for _, width := range []int{30, 42} {
			want := legacyRenderDocument(document, width)
			got := renderDocument(document, width)
			if got != want {
				t.Fatalf("El documento %d (%s, %d columnas) cambió.\nAntes:\n%s\nAhora:\n%s", index, document.Kind, width, want, got)
			}
			cases++
		}
	}
	if cases < 150 {
		t.Fatalf("La matriz comercial es demasiado pequeña: %d casos", cases)
	}
}

// El detalle de efectivo con dólares es el mismo que el de la plantilla anterior.
func TestCashWithDollarsMatchesLegacyLines(t *testing.T) {
	payment := DocumentPayment{
		Method: "Efectivo", Amount: "1850.00", Tip: "0.00", Received: textPointer("1942.50"), Change: "92.50",
		ReceivedMXN: textPointer("1000.00"), ReceivedUSD: textPointer("50.95"), USDRate: textPointer("18.50"),
		ChangeCurrency: textPointer(currencyUSD), ChangeUSD: textPointer("5.00"),
	}
	for _, width := range []int{30, 42} {
		text := strings.Join(cashWithDollars(payment, width), "\n")
		if !strings.Contains(text, strings.Join(legacyCashWithDollars(payment, width), "\n")) {
			t.Fatalf("El detalle en dólares cambió:\n%s", text)
		}
	}
}

func cashHeader(t *testing.T, name string) *documentHeader {
	t.Helper()
	header := testHeader(t)
	header.establishment = name
	return header
}

// voucherVariants son comprobantes con y sin categoría, tipo, referencia, observaciones,
// autorización, corrección, salida excepcional y dólares.
func voucherVariants() []cashDocumentSource {
	sources := []cashDocumentSource{}
	created := time.Date(2026, 11, 9, 18, 30, 0, 0, time.UTC)
	for _, kind := range []string{operationIncome, operationWithdrawal, operationExpense} {
		for mask := 0; mask < 64; mask++ {
			operation := &CashOperation{
				Kind: kind, Currency: currencyMXN, Amount: "1500.00", Concept: "Concepto del movimiento\x1b con controles", RegisterName: "Caja 1",
				EquivalentMXN: "1500.00", ActorName: "Cajera", DeviceName: "Terminal 1", CreatedAt: created, Status: "recorded",
			}
			if mask&1 != 0 {
				operation.CategoryName = textPointer("Insumos de limpieza")
				operation.TypeLabel = textPointer("Resguardo en caja fuerte")
			}
			if mask&2 != 0 {
				operation.Reference = "Sobre 4"
				operation.Notes = "Observaciones largas del movimiento para comprobar el corte entre palabras del comprobante"
			}
			if mask&4 != 0 {
				operation.AuthorizedByName = textPointer("Encargada")
				operation.AuthorizationMode = textPointer("supervisor")
				if mask&32 != 0 {
					operation.AuthorizationMode = textPointer("permission")
				}
			}
			if mask&8 != 0 {
				operation.Status = "corrected"
			}
			if mask&16 != 0 && kind != operationIncome {
				operation.Exceptional = true
				operation.Reason = "Pago urgente a proveedor autorizado"
			}
			if mask&32 != 0 {
				operation.Currency = currencyUSD
				operation.Amount = "20.00"
				operation.USDRate = textPointer("18.50")
				operation.EquivalentMXN = "370.00"
			}
			sources = append(sources, cashDocumentSource{kind: kind, shift: &Shift{BusinessDate: "2026-11-09"}, operation: operation})
		}
	}
	return sources
}

// cutVariants son cortes parciales y definitivos con y sin esperado, diferencias, declarado,
// dólares, métodos, correcciones, regularizaciones y observaciones.
func cutVariants() []cashDocumentSource {
	sources := []cashDocumentSource{}
	for _, final := range []bool{false, true} {
		for mask := 0; mask < 64; mask++ {
			source := finalReportSource()
			report := source.report
			shift := source.shift
			if !final {
				source.kind = documentPartialCut
				report.Kind = "partial"
				report.Sequence = 3
				report.Snapshot.Declared = nil
				report.Snapshot.Sensitive.Differences = nil
				shift.ClosedByName = nil
				if mask&1 != 0 {
					previous := "corte-anterior"
					report.PreviousReportID = &previous
				}
			} else if mask&1 != 0 {
				shift.ClosedByName = nil
			}
			if mask&2 != 0 {
				report.Snapshot.Sensitive = nil
			} else if mask&4 != 0 {
				report.Snapshot.Sensitive.Expected.TotalEquivalentMXN = textPointer("1110.00")
				if report.Snapshot.Sensitive.Differences != nil {
					report.Snapshot.Sensitive.Differences.EquivalentMXN = textPointer("-92.50")
					report.Snapshot.Sensitive.Differences.ResultUSD = resultOver
				}
			}
			if mask&8 != 0 {
				report.Snapshot.Control.FloatUSD = textPointer("100.00")
				report.Snapshot.Control.USDRate = textPointer("18.50")
				shift.OpeningFloatUSD = textPointer("100.00")
				report.Snapshot.Accumulated.Cash = append(report.Snapshot.Accumulated.Cash, currencyOperations{
					Currency: currencyUSD, Income: "10.00", Withdrawals: "5.00", Expenses: "0.00", CorrectionsIn: "1.00", CorrectionsOut: "0.00",
				})
				if report.Snapshot.Declared != nil {
					report.Snapshot.Declared.CountedUSD = "105.00"
					report.Snapshot.Declared.ExtraMXN = "12.50"
					report.Snapshot.Declared.ExtraUSD = "1.00"
				}
			}
			if mask&16 != 0 {
				report.Snapshot.Accumulated.Methods = nil
				report.Snapshot.Accumulated.Regularizations = 1
			} else if mask&32 == 0 {
				report.Snapshot.Accumulated.Regularizations = 2
			}
			if mask&32 != 0 && report.Snapshot.Declared != nil {
				report.Snapshot.Declared.CashierNotes = "   "
			}
			sources = append(sources, source)
		}
	}
	return sources
}

func TestDefaultCashTemplatesMatchLegacyRenderer(t *testing.T) {
	cases := 0
	check := func(source cashDocumentSource, include bool, width int, name string) {
		t.Helper()
		header := cashHeader(t, name)
		want, err := legacyRenderCashDocument(header, source, include, width)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Join(renderDefaultCash(header, source, include, width), "\n")
		if got != want {
			t.Fatalf("El documento de caja %s (%d columnas, esperado %v) cambió.\nAntes:\n%s\nAhora:\n%s", source.kind, width, include, want, got)
		}
		cases++
	}
	for _, name := range []string{"ADEL POS", "Restaurante con un nombre comercial larguísimo para cortar"} {
		for _, width := range []int{30, 42} {
			for _, source := range voucherVariants() {
				check(source, false, width, name)
			}
			for _, source := range cutVariants() {
				// includeExpected es la decisión de reportIncludesExpected: nunca sin snapshot sensible.
				for _, include := range []bool{false, true} {
					if include && source.report.Snapshot.Sensitive == nil {
						continue
					}
					check(source, include, width, name)
				}
			}
		}
	}
	if cases < 1000 {
		t.Fatalf("La matriz de caja es demasiado pequeña: %d casos", cases)
	}
}

// Sin el esperado incluido, los datos estructurados que guarda el ticket no lo contienen.
func TestCashDataOmitsSensitiveWhenNotIncluded(t *testing.T) {
	source := finalReportSource()
	data := cashData(testHeader(t), source, false)
	if data.Cut == nil || data.Cut.Sensitive != nil {
		t.Fatal("Los datos de un corte sin el esperado conservaron el bloque sensible")
	}
	included := cashData(testHeader(t), source, true)
	if included.Cut.Sensitive == nil || included.Cut.Sensitive.Differences == nil {
		t.Fatal("Los datos de un corte con el esperado lo perdieron")
	}
	if fmt.Sprint(documents.Default(documents.KindCashFinalCut).Blocks[0].Type) != documents.BlockEstablishmentName {
		t.Fatal("La plantilla del corte no empieza con el nombre del establecimiento")
	}
}
