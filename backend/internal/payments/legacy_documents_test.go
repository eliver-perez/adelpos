// Copias literales de las plantillas de texto de payments anteriores a la etapa 9B (commit fd1de81):
// renderDocument y cashWithDollars de documents.go, y renderCashDocument con sus ayudantes de
// cash_documents.go, con los nombres prefijados con «legacy». Son la referencia de las pruebas de
// equivalencia. Única adaptación: legacyRenderCashDocument recibe el encabezado ya leído en lugar
// de leerlo de la base. Los ayudantes de formato de importes (currencyText, rateText, zeroAmount,
// pesosText y dollarsText) conservan su nombre: las pruebas anteriores los usan. No modificar.

package payments

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/moneytext"
	"local/ad-el-pos/server/internal/plural"
)

// legacyPrintable elimina controles e invisibles: solo el controlador de impresión genera comandos.
func legacyPrintable(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return ' '
		}
		return character
	}, value)
}

func legacyFit(text string, width int) []string {
	runes := []rune(legacyPrintable(text))
	lines := []string{}
	for len(runes) > width {
		lines = append(lines, string(runes[:width]))
		runes = runes[width:]
	}
	return append(lines, string(runes))
}

// legacyRow alinea un concepto a la izquierda y un importe a la derecha dentro del ancho del papel.
func legacyRow(left, right string, width int) []string {
	left = legacyPrintable(left)
	right = legacyPrintable(right)
	space := width - len([]rune(right)) - 1
	if space < 4 {
		return append(legacyFit(left, width), right)
	}
	parts := legacyFit(left, space)
	last := parts[len(parts)-1]
	parts[len(parts)-1] = last + strings.Repeat(" ", width-len([]rune(last))-len([]rune(right))) + right
	return parts
}

func legacyRenderDocument(document *CommercialDocument, width int) string {
	lines := []string{}
	add := func(values ...string) {
		lines = append(lines, values...)
	}
	divider := strings.Repeat("-", width)
	add(legacyFit(document.Establishment, width)...)
	if document.Kind == "precheck" {
		add("PRECUENTA")
	} else {
		add("TICKET DE PAGO")
	}
	add(legacyFit("Folio "+decimal.NewFromInt(document.Folio).String()+" · "+document.AccountName, width)...)
	add(legacyFit(document.Context+" · "+document.LocalTime, width)...)
	add(legacyFit("Atendió: "+document.Cashier, width)...)
	add(divider)
	for _, line := range document.Lines {
		add(legacyRow(decimal.NewFromInt(int64(line.Quantity)).String()+" x "+line.Name, pesosText(line.Amount), width)...)
		if strings.TrimSpace(line.Notes) != "" {
			add(legacyFit("   Nota: "+line.Notes, width)...)
		}
	}
	// Los importes usan el formato único de moneytext; un descuento se imprime «-$50.00». Un
	// descuento o cortesía con motivo del catálogo lo muestra junto al concepto («Descuento · Atención»).
	for _, adjustment := range document.Adjustments {
		concept := adjustment.Concept
		if adjustment.Reason != "" {
			concept += " · " + adjustment.Reason
		}
		add(legacyRow(concept, pesosText(adjustment.Amount), width)...)
	}
	add(divider)
	for _, tax := range document.Tax {
		add(legacyRow("Subtotal sin IVA "+decimal.NewFromInt(int64(tax.Rate)).String()+" %", pesosText(tax.Base), width)...)
		add(legacyRow("IVA "+decimal.NewFromInt(int64(tax.Rate)).String()+" %", pesosText(tax.Tax), width)...)
	}
	add(legacyRow("TOTAL", pesosText(document.Total), width)...)
	// Línea informativa (etapa 7.1): el total en dólares con el tipo de cambio del turno.
	if document.USDTotal != nil && document.USDRate != nil {
		add(legacyRow("Total en dólares (T.C. "+pesosText(*document.USDRate)+")", dollarsText(*document.USDTotal), width)...)
	}
	if document.Payment != nil {
		add(divider)
		add("ESTE PAGO")
		add(legacyRow(document.Payment.Method, pesosText(document.Payment.Amount), width)...)
		if document.Payment.Reference != "" {
			add(legacyFit("   Ref. "+document.Payment.Reference, width)...)
		}
		if document.Payment.ReceivedUSD != nil {
			add(legacyCashWithDollars(*document.Payment, width)...)
		} else if document.Payment.Received != nil {
			add(legacyRow("   Efectivo recibido", pesosText(*document.Payment.Received), width)...)
			add(legacyRow("   Cambio", pesosText(document.Payment.Change), width)...)
		}
		if document.Payment.Tip != "0.00" {
			add(legacyRow("   Propina (no es venta)", pesosText(document.Payment.Tip), width)...)
		}
	}
	if len(document.Payments) > 0 {
		add(divider)
		add("PAGOS REGISTRADOS")
		for _, payment := range document.Payments {
			add(legacyRow(payment.Method, pesosText(payment.Amount), width)...)
		}
		add(legacyRow("Pagado", pesosText(document.Paid), width)...)
		if document.Tips != "0.00" {
			add(legacyRow("Propinas", pesosText(document.Tips), width)...)
		}
	}
	if document.Balance != "0.00" {
		add(legacyRow("SALDO PENDIENTE", pesosText(document.Balance), width)...)
	}
	add(divider)
	add(legacyFit("Documento comercial. No es un comprobante fiscal (CFDI).", width)...)
	return strings.Join(lines, "\n")
}

// legacyCashWithDollars detalla un efectivo con dólares: lo recibido en cada moneda, el tipo de cambio
// del turno, el total en pesos y el cambio con la moneda en que se entregó.
func legacyCashWithDollars(payment DocumentPayment, width int) []string {
	lines := []string{}
	lines = append(lines, legacyRow("   Pesos recibidos", pesosText(*payment.ReceivedMXN), width)...)
	lines = append(lines, legacyRow("   Dólares recibidos", dollarsText(*payment.ReceivedUSD), width)...)
	lines = append(lines, legacyRow("   Tipo de cambio", rateText(*payment.USDRate), width)...)
	if payment.Received != nil {
		lines = append(lines, legacyRow("   Total recibido", pesosText(*payment.Received), width)...)
	}
	if payment.ChangeCurrency != nil && *payment.ChangeCurrency == currencyUSD && payment.ChangeUSD != nil {
		lines = append(lines, legacyRow("   Cambio en dólares", dollarsText(*payment.ChangeUSD), width)...)
		lines = append(lines, legacyRow("   Equivale a", pesosText(payment.Change), width)...)
	} else {
		lines = append(lines, legacyRow("   Cambio en pesos", pesosText(payment.Change), width)...)
	}
	return lines
}

// currencyText expresa un importe guardado por Go en su moneda con el formato único de
// moneytext: miles, dos decimales y el signo antes del símbolo («-$1,234.56», «US$20.00»).
func currencyText(currency, amount string) string {
	return moneytext.Format(currency, amount)
}

// rateText expresa el tipo de cambio de un turno: pesos por dólar («$18.50/US$»).
func rateText(rate string) string {
	return pesosText(rate) + "/US$"
}

// zeroAmount indica si un importe guardado es cero (o no es un importe).
func zeroAmount(amount string) bool {
	value, err := decimal.NewFromString(amount)
	return err != nil || value.IsZero()
}

func pesosText(amount string) string {
	return currencyText(currencyMXN, amount)
}

func dollarsText(amount string) string {
	return currencyText(currencyUSD, amount)
}

var legacyDifferenceLabels = map[string]string{
	resultEven:  "Sin diferencia",
	resultOver:  "Sobrante",
	resultShort: "Faltante",
}

var legacyCurrencyNames = map[string]string{
	currencyMXN: "Pesos",
	currencyUSD: "Dólares",
}

// legacyDocumentTitle es el encabezado impreso de cada tipo de documento.
func legacyDocumentTitle(kind string, sequence int) string {
	switch kind {
	case documentPartialCut:
		return "CORTE PARCIAL #" + strconv.Itoa(sequence)
	case documentFinalCut:
		return "CORTE DEFINITIVO"
	case operationIncome:
		return "COMPROBANTE DE INGRESO"
	case operationWithdrawal:
		return "COMPROBANTE DE RETIRO"
	}
	return "COMPROBANTE DE GASTO"
}

// legacyDocumentLines acumula las líneas de un documento con el ancho del papel.
type legacyDocumentLines struct {
	width int
	lines []string
}

func (document *legacyDocumentLines) text(value string) {
	document.lines = append(document.lines, legacyWrapWords(value, document.width)...)
}

// legacyWrapWords reparte un texto en líneas del ancho del papel cortando entre palabras; una palabra
// más larga que el ancho se corta como en los documentos comerciales.
func legacyWrapWords(value string, width int) []string {
	words := strings.Fields(legacyPrintable(value))
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{}
	current := ""
	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if len([]rune(candidate)) <= width {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
		}
		pieces := legacyFit(word, width)
		lines = append(lines, pieces[:len(pieces)-1]...)
		current = pieces[len(pieces)-1]
	}
	return append(lines, current)
}

func (document *legacyDocumentLines) pair(left, right string) {
	document.lines = append(document.lines, legacyRow(left, right, document.width)...)
}

func (document *legacyDocumentLines) divider() {
	document.lines = append(document.lines, strings.Repeat("-", document.width))
}

func (document *legacyDocumentLines) String() string {
	return strings.Join(document.lines, "\n")
}

// sourceTitle es el título del documento según su origen.
func legacySourceTitle(source cashDocumentSource) string {
	if source.report != nil {
		return legacyDocumentTitle(source.kind, source.report.Sequence)
	}
	return legacyDocumentTitle(source.kind, 0)
}

// legacyRenderCashDocument produce el texto del documento con el ancho indicado.
func legacyRenderCashDocument(header *documentHeader, source cashDocumentSource, includeExpected bool, width int) (string, error) {
	document := &legacyDocumentLines{width: width}
	document.text(header.establishment)
	document.text(legacySourceTitle(source))
	if source.report != nil {
		legacyRenderReport(document, header, source, includeExpected)
	} else {
		legacyRenderVoucher(document, header, source)
	}
	document.divider()
	document.text("Documento interno de caja. No es un comprobante fiscal (CFDI).")
	return document.String(), nil
}

// legacyRenderReport escribe el contenido de un corte (§20): control, ventas y pagos por método,
// devoluciones, propinas, descuentos y cortesías, ingresos, retiros, gastos, lo declarado, el
// esperado y las diferencias (según permisos) y las observaciones.
func legacyRenderReport(document *legacyDocumentLines, header *documentHeader, source cashDocumentSource, includeExpected bool) {
	report := source.report
	shift := source.shift
	control := report.Snapshot.Control
	totals := report.Snapshot.Accumulated
	document.text("Caja: " + control.RegisterName)
	document.text("Turno del " + displayDate(control.BusinessDate) + " · responsable " + control.OpenedByName)
	document.pair("Apertura", header.local(shift.OpenedAt))
	if report.Kind == "final" {
		document.pair("Cierre", header.local(report.CoveredTo))
		if shift.ClosedByName != nil {
			document.text("Cerró: " + *shift.ClosedByName)
		}
	} else {
		document.pair("Corte", header.local(report.CoveredTo))
		if report.PreviousReportID != nil {
			document.pair("Corte anterior", header.local(report.CoveredFrom))
		}
	}
	document.text("Realizó: " + report.CreatedByName + " · " + report.DeviceName)
	document.divider()
	document.pair("Fondo inicial", pesosText(control.OpeningFloat))
	if control.FloatUSD != nil {
		document.pair("Fondo inicial en dólares", dollarsText(*control.FloatUSD))
	}
	if control.USDRate != nil {
		document.pair("Tipo de cambio del turno", rateText(*control.USDRate))
	}
	document.divider()
	document.text("VENTAS Y PAGOS POR MÉTODO")
	if len(totals.Methods) == 0 {
		document.text("Sin cobros registrados.")
	}
	for _, method := range totals.Methods {
		document.pair(method.MethodName, pesosText(method.Sales))
		if !zeroAmount(method.Tips) {
			document.pair("   Propinas", pesosText(method.Tips))
		}
		if !zeroAmount(method.Refunds) {
			document.pair("   Devoluciones", pesosText(method.Refunds))
		}
	}
	document.pair("Ventas", pesosText(totals.Sales))
	document.pair("Devoluciones", pesosText(totals.Refunded))
	document.pair("Propinas (no son venta)", pesosText(totals.Tips))
	document.pair("Descuentos ("+strconv.Itoa(totals.DiscountCount)+")", pesosText(totals.Discounts))
	document.pair("Cortesías ("+strconv.Itoa(totals.CourtesyCount)+")", pesosText(totals.Courtesies))
	document.pair("Cobros registrados", strconv.Itoa(totals.Payments))
	document.divider()
	document.text("INGRESOS, RETIROS Y GASTOS")
	for _, cash := range totals.Cash {
		document.text(legacyCurrencyNames[cash.Currency])
		document.pair("   Ingresos", currencyText(cash.Currency, cash.Income))
		document.pair("   Retiros", currencyText(cash.Currency, cash.Withdrawals))
		document.pair("   Gastos", currencyText(cash.Currency, cash.Expenses))
		if !zeroAmount(cash.CorrectionsIn) || !zeroAmount(cash.CorrectionsOut) {
			document.pair("   Correcciones que entran", currencyText(cash.Currency, cash.CorrectionsIn))
			document.pair("   Correcciones que salen", currencyText(cash.Currency, cash.CorrectionsOut))
		}
	}
	if totals.Regularizations > 0 {
		// La frase concuerda con el número: «1 regularización de un turno cerrado; no cambia este
		// turno.» y «2 regularizaciones de turnos cerrados; no cambian este turno.».
		regularizations := plural.Count(
			totals.Regularizations,
			"regularización de un turno cerrado",
			"regularizaciones de turnos cerrados",
		)
		verb := plural.Word(totals.Regularizations, "no cambia", "no cambian")
		document.text(regularizations + "; " + verb + " este turno.")
	}
	legacyRenderDeclared(document, report, shiftHandlesDollars(shift))
	legacyRenderSensitive(document, report, includeExpected)
	if report.Snapshot.Declared != nil && strings.TrimSpace(report.Snapshot.Declared.CashierNotes) != "" {
		document.divider()
		document.text("OBSERVACIONES")
		document.text(report.Snapshot.Declared.CashierNotes)
	}
}

// legacyRenderDeclared escribe lo que el cajero contó en el arqueo (solo en el corte definitivo).
func legacyRenderDeclared(document *legacyDocumentLines, report *CashReport, dollars bool) {
	declared := report.Snapshot.Declared
	if declared == nil {
		return
	}
	document.divider()
	document.text("EFECTIVO DECLARADO")
	document.pair("Pesos", pesosText(declared.CountedMXN))
	if dollars {
		document.pair("Dólares", dollarsText(declared.CountedUSD))
	}
	if !zeroAmount(declared.ExtraMXN) {
		document.pair("   Incluye adicional en pesos", pesosText(declared.ExtraMXN))
	}
	if !zeroAmount(declared.ExtraUSD) {
		document.pair("   Incluye adicional en dólares", dollarsText(declared.ExtraUSD))
	}
}

// legacyRenderSensitive escribe el efectivo esperado y las diferencias por moneda, o una nota de que no
// se incluyen.
func legacyRenderSensitive(document *legacyDocumentLines, report *CashReport, includeExpected bool) {
	document.divider()
	sensitive := report.Snapshot.Sensitive
	if !includeExpected || sensitive == nil {
		document.text("Esperado y diferencias: no incluidos.")
		return
	}
	expected := sensitive.Expected
	document.text("EFECTIVO ESPERADO")
	document.pair("Pesos", pesosText(expected.MXN))
	document.pair("Dólares", dollarsText(expected.USD))
	if expected.TotalEquivalentMXN != nil {
		document.pair("Total equivalente en pesos", pesosText(*expected.TotalEquivalentMXN))
	}
	differences := sensitive.Differences
	if differences == nil {
		return
	}
	document.text("DIFERENCIAS (CONTADO - ESPERADO)")
	document.pair("Pesos · "+legacyDifferenceLabels[differences.ResultMXN], pesosText(differences.MXN))
	document.pair("Dólares · "+legacyDifferenceLabels[differences.ResultUSD], dollarsText(differences.USD))
	if differences.EquivalentMXN != nil {
		document.pair("Equivalente en pesos", pesosText(*differences.EquivalentMXN))
	}
}

// legacyRenderVoucher escribe el comprobante de un ingreso, retiro o gasto. Nunca contiene el efectivo
// esperado de la caja.
func legacyRenderVoucher(document *legacyDocumentLines, header *documentHeader, source cashDocumentSource) {
	operation := source.operation
	document.text("Caja: " + operation.RegisterName)
	document.text("Turno del " + displayDate(source.shift.BusinessDate))
	document.pair("Fecha", header.local(operation.CreatedAt))
	document.divider()
	if operation.Kind == operationExpense && operation.CategoryName != nil {
		document.text("Categoría: " + *operation.CategoryName)
	}
	if operation.TypeLabel != nil {
		label := "Tipo: "
		if operation.Kind == operationWithdrawal {
			label = "Motivo: "
		}
		document.text(label + *operation.TypeLabel)
	}
	document.text("Concepto: " + operation.Concept)
	document.pair("Importe", currencyText(operation.Currency, operation.Amount))
	if operation.Currency == currencyUSD && operation.USDRate != nil {
		document.pair("Tipo de cambio", rateText(*operation.USDRate))
		document.pair("Equivale a", pesosText(operation.EquivalentMXN))
	}
	if operation.Reference != "" {
		document.text("Referencia: " + operation.Reference)
	}
	if operation.Notes != "" {
		document.text("Observaciones: " + operation.Notes)
	}
	if operation.Exceptional {
		document.text("Salida excepcional: " + operation.Reason)
	}
	document.divider()
	document.text("Registró: " + operation.ActorName + " · " + operation.DeviceName)
	if operation.AuthorizedByName != nil && operation.AuthorizationMode != nil {
		document.text("Autorizó: " + *operation.AuthorizedByName)
		if *operation.AuthorizationMode == "supervisor" {
			document.text("   Autorización de supervisor")
		} else {
			document.text("   Con su propio permiso")
		}
	}
	if operation.Status == "corrected" {
		document.text("Estado: corregido por una corrección posterior.")
	}
	document.text("")
	document.text("Firma: ______________________")
}
