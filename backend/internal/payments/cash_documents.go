package payments

import (
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
)

// Documentos de caja (etapa 7, §20): comprobantes de ingreso, retiro y gasto, corte parcial y corte
// definitivo. Go los renderiza con la misma plantilla de texto de los documentos comerciales y los
// imprime por la cola durable de la etapa 5 (kitchen_tickets con tipo cash_report o cash_voucher),
// en la impresora de tickets de Cobros.
//
//   - La vista previa es de solo lectura y usa la misma plantilla que la impresión.
//   - Imprimir nunca crea movimientos financieros, nunca vuelve a cerrar un turno y nunca cambia un
//     cierre: solo registra el documento y su trabajo de impresión.
//   - El efectivo esperado y las diferencias solo se imprimen si quien genera el documento puede
//     consultarlos (cash.expected.read) y el turno no está en arqueo. Un documento que los contiene
//     solo se reimprime con ese permiso y fuera del arqueo.
//   - Cada corte u operación tiene un solo documento; las copias posteriores son reimpresiones del
//     mismo ticket, con el permiso de reimprimir.
//   - Un fallo de impresión nunca revierte ni bloquea la operación o el cierre que la solicitó.
//   - La aceptación de CUPS o TCP no confirma que el papel salió: la respuesta solo informa el
//     estado del trabajo en la cola.

// Tipos de documento de caja (cash_documents.kind).
const (
	documentPartialCut = "partial_cut"
	documentFinalCut   = "final_cut"
)

// PrintOutcome es el resultado de una impresión solicitada. Status es el estado del trabajo en la
// cola (pending, intervention…) o not_configured (sin impresora) o failed (no se pudo encolar).
type PrintOutcome struct {
	Requested        bool   `json:"requested"`
	Status           string `json:"status,omitempty"`
	DocumentID       string `json:"document_id,omitempty"`
	TicketID         string `json:"ticket_id,omitempty"`
	JobID            string `json:"job_id,omitempty"`
	IncludesExpected bool   `json:"includes_expected"`
	Message          string `json:"message,omitempty"`
}

// CashDocumentRecord es un documento de caja registrado con el estado de su último trabajo.
type CashDocumentRecord struct {
	ID               string    `json:"id"`
	Kind             string    `json:"kind"`
	ShiftID          string    `json:"shift_id"`
	ReportID         *string   `json:"report_id"`
	CashOperationID  *string   `json:"cash_operation_id"`
	TicketID         string    `json:"ticket_id"`
	IncludesExpected bool      `json:"includes_expected"`
	CreatedByName    string    `json:"created_by_name"`
	CreatedAt        time.Time `json:"created_at"`
	JobID            *string   `json:"job_id"`
	JobState         *string   `json:"job_state"`
	PrinterName      *string   `json:"printer_name"`
	Copies           int       `json:"copies"`
}

// CashDocumentPreview es la vista previa de un documento de caja renderizada por Go.
type CashDocumentPreview struct {
	Kind             string              `json:"kind"`
	Title            string              `json:"title"`
	Content          string              `json:"content"`
	IncludesExpected bool                `json:"includes_expected"`
	PaperWidth       int                 `json:"paper_width"`
	Document         *CashDocumentRecord `json:"document"`
}

// cashDocumentSource es lo que se imprime: un corte o una operación de efectivo de un turno.
type cashDocumentSource struct {
	kind      string
	shift     *Shift
	report    *CashReport
	operation *CashOperation
}

// documentHeader reúne el nombre y la zona horaria del establecimiento.
type documentHeader struct {
	establishment string
	location      *time.Location
}

func (handler *service) readDocumentHeader() (*documentHeader, error) {
	var name, timezone string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT name,timezone FROM app.establishment").Scan(&name, &timezone); err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}
	return &documentHeader{establishment: name, location: location}, nil
}

func (header *documentHeader) local(moment time.Time) string {
	return moment.In(header.location).Format("02/01/2006 15:04")
}

// displayDate expresa una fecha comercial AAAA-MM-DD como dd/mm/aaaa.
func displayDate(value string) string {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return parsed.Format("02/01/2006")
}

// currencyText expresa un importe guardado por Go en su moneda, con dos decimales y el signo antes
// del símbolo.
func currencyText(currency, amount string) string {
	symbol := "$"
	if currency == currencyUSD {
		symbol = "US$"
	}
	if value, err := decimal.NewFromString(amount); err == nil {
		amount = money(value)
	}
	if strings.HasPrefix(amount, "-") {
		return "-" + symbol + strings.TrimPrefix(amount, "-")
	}
	return symbol + amount
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

var differenceLabels = map[string]string{
	resultEven:  "Sin diferencia",
	resultOver:  "Sobrante",
	resultShort: "Faltante",
}

var currencyNames = map[string]string{
	currencyMXN: "Pesos",
	currencyUSD: "Dólares",
}

// documentTitle es el encabezado impreso de cada tipo de documento.
func documentTitle(kind string, sequence int) string {
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

// documentLines acumula las líneas de un documento con el ancho del papel.
type documentLines struct {
	width int
	lines []string
}

func (document *documentLines) text(value string) {
	document.lines = append(document.lines, wrapWords(value, document.width)...)
}

// wrapWords reparte un texto en líneas del ancho del papel cortando entre palabras; una palabra
// más larga que el ancho se corta como en los documentos comerciales.
func wrapWords(value string, width int) []string {
	words := strings.Fields(printable(value))
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
		pieces := fit(word, width)
		lines = append(lines, pieces[:len(pieces)-1]...)
		current = pieces[len(pieces)-1]
	}
	return append(lines, current)
}

func (document *documentLines) pair(left, right string) {
	document.lines = append(document.lines, row(left, right, document.width)...)
}

func (document *documentLines) divider() {
	document.lines = append(document.lines, strings.Repeat("-", document.width))
}

func (document *documentLines) String() string {
	return strings.Join(document.lines, "\n")
}

// sourceTitle es el título del documento según su origen.
func (source cashDocumentSource) title() string {
	if source.report != nil {
		return documentTitle(source.kind, source.report.Sequence)
	}
	return documentTitle(source.kind, 0)
}

// reportIncludesExpected decide si el corte se imprime con el efectivo esperado: solo para quien
// puede consultarlo, fuera del arqueo de cierre y si el snapshot lo conserva.
func (handler *service) reportIncludesExpected(source cashDocumentSource) bool {
	if source.report == nil || source.report.Snapshot.Sensitive == nil {
		return false
	}
	return handler.expectedVisible(source.shift)
}

// renderCashDocument produce el texto del documento con el ancho indicado.
func (handler *service) renderCashDocument(source cashDocumentSource, includeExpected bool, width int) (string, error) {
	header, err := handler.readDocumentHeader()
	if err != nil {
		return "", err
	}
	document := &documentLines{width: width}
	document.text(header.establishment)
	document.text(source.title())
	if source.report != nil {
		renderReport(document, header, source, includeExpected)
	} else {
		renderVoucher(document, header, source)
	}
	document.divider()
	document.text("Documento interno de caja. No es un comprobante fiscal (CFDI).")
	return document.String(), nil
}

// renderReport escribe el contenido de un corte (§20): control, ventas y pagos por método,
// devoluciones, propinas, descuentos y cortesías, ingresos, retiros, gastos, lo declarado, el
// esperado y las diferencias (según permisos) y las observaciones.
func renderReport(document *documentLines, header *documentHeader, source cashDocumentSource, includeExpected bool) {
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
		document.pair("Tipo de cambio del turno", pesosText(*control.USDRate)+"/US$")
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
		document.text(currencyNames[cash.Currency])
		document.pair("   Ingresos", currencyText(cash.Currency, cash.Income))
		document.pair("   Retiros", currencyText(cash.Currency, cash.Withdrawals))
		document.pair("   Gastos", currencyText(cash.Currency, cash.Expenses))
		if !zeroAmount(cash.CorrectionsIn) || !zeroAmount(cash.CorrectionsOut) {
			document.pair("   Correcciones que entran", currencyText(cash.Currency, cash.CorrectionsIn))
			document.pair("   Correcciones que salen", currencyText(cash.Currency, cash.CorrectionsOut))
		}
	}
	if totals.Regularizations > 0 {
		document.text("Regularizaciones de turnos cerrados: " + strconv.Itoa(totals.Regularizations) + " (no cambian este turno).")
	}
	renderDeclared(document, report, shiftHandlesDollars(shift))
	renderSensitive(document, report, includeExpected)
	if report.Snapshot.Declared != nil && strings.TrimSpace(report.Snapshot.Declared.CashierNotes) != "" {
		document.divider()
		document.text("OBSERVACIONES")
		document.text(report.Snapshot.Declared.CashierNotes)
	}
}

// renderDeclared escribe lo que el cajero contó en el arqueo (solo en el corte definitivo).
func renderDeclared(document *documentLines, report *CashReport, dollars bool) {
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

// renderSensitive escribe el efectivo esperado y las diferencias por moneda, o una nota de que no
// se incluyen.
func renderSensitive(document *documentLines, report *CashReport, includeExpected bool) {
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
	document.pair("Pesos · "+differenceLabels[differences.ResultMXN], pesosText(differences.MXN))
	document.pair("Dólares · "+differenceLabels[differences.ResultUSD], dollarsText(differences.USD))
	if differences.EquivalentMXN != nil {
		document.pair("Equivalente en pesos", pesosText(*differences.EquivalentMXN))
	}
}

// renderVoucher escribe el comprobante de un ingreso, retiro o gasto. Nunca contiene el efectivo
// esperado de la caja.
func renderVoucher(document *documentLines, header *documentHeader, source cashDocumentSource) {
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
		document.pair("Tipo de cambio", pesosText(*operation.USDRate)+"/US$")
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

// cashPrinter devuelve la impresora de tickets configurada en Cobros (el mismo criterio de los
// documentos comerciales de la etapa 6) y el ancho de texto del papel.
func (handler *service) cashPrinter() (*string, int, error) {
	settings, err := handler.settings()
	if err != nil {
		return nil, 0, err
	}
	if settings.ReceiptPrinterID == nil {
		return nil, paperWidth(80), nil
	}
	var columns int
	if err := handler.tx.QueryRow(handler.ctx, "SELECT paper_width FROM app.kitchen_printers WHERE id=$1",
		*settings.ReceiptPrinterID).Scan(&columns); err != nil {
		return nil, 0, err
	}
	return settings.ReceiptPrinterID, paperWidth(columns), nil
}

// existingDocument devuelve el documento ya registrado de un corte o de una operación.
func (handler *service) existingDocument(source cashDocumentSource) (*CashDocumentRecord, error) {
	condition := "d.cash_operation_id=$1"
	identifier := ""
	if source.report != nil {
		condition = "d.report_id=$1"
		identifier = source.report.ID
	} else {
		identifier = source.operation.ID
	}
	documents, err := handler.listCashDocuments(condition, identifier)
	if err != nil || len(documents) == 0 {
		return nil, err
	}
	return &documents[0], nil
}

// enqueueCashDocument registra el documento y su trabajo en la cola con la transacción del
// servicio. No valida permisos: lo hacen las acciones que lo llaman, antes de escribir.
func (handler *service) enqueueCashDocument(source cashDocumentSource, printerID string, width int) (PrintOutcome, error) {
	includeExpected := handler.reportIncludesExpected(source)
	content, err := handler.renderCashDocument(source, includeExpected, width)
	if err != nil {
		return PrintOutcome{}, err
	}
	ticketKind := kitchen.KindCashVoucher
	var reportID, operationID *string
	if source.report != nil {
		ticketKind = kitchen.KindCashReport
		reportID = &source.report.ID
	} else {
		operationID = &source.operation.ID
	}
	documentID := newID()
	// El ticket conserva el contenido impreso y lo necesario para que la cola aplique los permisos
	// de caja (turno e inclusión del esperado), sin claves que la cola interprete como comanda.
	ticket := map[string]any{
		"kind": ticketKind, "document_kind": source.kind, "document_id": documentID, "shift_id": source.shift.ID,
		"report_id": reportID, "cash_operation_id": operationID, "includes_expected": includeExpected,
		"title": source.title(), "content": content,
	}
	job, ticketID, err := kitchen.EnqueueCommercial(handler.ctx, handler.tx, kitchen.CommercialPrint{
		Kind: ticketKind, Document: ticket, Content: content, PrinterID: printerID, OperationID: handler.operationID,
		ActorID: handler.request.Actor.UserID, DeviceID: handler.request.Actor.DeviceID,
	})
	if err != nil {
		return PrintOutcome{}, err
	}
	if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_documents(id,ticket_id,shift_id,report_id,cash_operation_id,kind,
        includes_expected,created_by,device_id,operation_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		documentID, ticketID, source.shift.ID, reportID, operationID, source.kind, includeExpected,
		handler.request.Actor.UserID, handler.request.Actor.DeviceID, handler.operationID); err != nil {
		return PrintOutcome{}, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.document.printed", cashScope, &source.shift.ID, nil); err != nil {
		return PrintOutcome{}, err
	}
	return PrintOutcome{
		Requested: true, Status: job.State, DocumentID: documentID, TicketID: ticketID, JobID: job.ID,
		IncludesExpected: includeExpected, Message: job.LastError,
	}, nil
}

// printRequested resuelve la bandera print de una operación o corte: solo se imprime si se pide
// expresamente (la impresión física es opcional) y exige cash.reports.print antes de escribir.
func (handler *service) printRequested(print *bool) (bool, error) {
	if print == nil || !*print {
		return false, nil
	}
	if err := handler.request.Actor.Require(permissionReportsPrint); err != nil {
		return false, err
	}
	return true, nil
}

// printAfterRecording imprime el documento de una operación o corte recién registrado. Se ejecuta
// en un punto de guardado: cualquier fallo al encolarlo se revierte solo a ese punto y la operación
// o el cierre ya válidos se confirman igual, con el estado de la impresión en la respuesta.
func (handler *service) printAfterRecording(source cashDocumentSource, subject string) PrintOutcome {
	// También la consulta de la impresora va dentro del punto de guardado: un error de PostgreSQL
	// fuera de él abortaría la transacción completa y, con ella, la operación o el cierre.
	outcome, err := handler.withSavepoint(func(scoped *service) (any, error) {
		printerID, width, err := scoped.cashPrinter()
		if err != nil {
			return nil, err
		}
		if printerID == nil {
			return PrintOutcome{Requested: true, Status: "not_configured", Message: "No hay impresora de tickets configurada en Cobros; " +
				subject + " quedó registrado."}, nil
		}
		return scoped.enqueueCashDocument(source, *printerID, width)
	})
	if err != nil {
		return PrintOutcome{Requested: true, Status: "failed", Message: "No se pudo encolar el documento; " + subject +
			" quedó registrado. Imprímelo después desde Caja."}
	}
	return outcome.(PrintOutcome)
}

// withSavepoint ejecuta una acción secundaria (impresión o comando del cajón) dentro de un punto
// de guardado de la transacción: si falla, se deshace solo lo que escribió y la operación
// principal continúa.
func (handler *service) withSavepoint(action func(scoped *service) (any, error)) (any, error) {
	nested, err := handler.tx.Begin(handler.ctx)
	if err != nil {
		return nil, err
	}
	scoped := *handler
	scoped.tx = nested
	result, err := action(&scoped)
	if err != nil {
		_ = nested.Rollback(handler.ctx)
		return nil, err
	}
	if err := nested.Commit(handler.ctx); err != nil {
		return nil, err
	}
	return result, nil
}

// reportSource lee un corte con las reglas de consulta de su turno.
func (handler *service) reportSource(reportID string) (cashDocumentSource, error) {
	report, err := handler.readReportRecord(reportID)
	if err != nil {
		return cashDocumentSource{}, err
	}
	shift, err := handler.readableShift(report.ShiftID)
	if err != nil {
		return cashDocumentSource{}, err
	}
	kind := documentPartialCut
	if report.Kind == "final" {
		kind = documentFinalCut
	}
	return cashDocumentSource{kind: kind, shift: shift, report: report}, nil
}

// operationSource lee una operación de efectivo imprimible con las reglas de consulta de su
// turno. Las correcciones y regularizaciones no tienen comprobante propio.
func (handler *service) operationSource(operationID string) (cashDocumentSource, error) {
	operation, err := handler.readCashOperation(operationID)
	if err != nil {
		return cashDocumentSource{}, err
	}
	if operation.Kind == operationCorrection {
		return cashDocumentSource{}, conflict("CASH_DOCUMENT_UNSUPPORTED",
			"Las correcciones y regularizaciones no tienen comprobante impreso; consulta su detalle en Movimientos.")
	}
	shift, err := handler.readableShift(operation.ShiftID)
	if err != nil {
		return cashDocumentSource{}, err
	}
	return cashDocumentSource{kind: operation.Kind, shift: shift, operation: operation}, nil
}

// previewSource renderiza la vista previa sin escribir: los mismos datos, permisos y plantilla
// que la impresión, con el ancho de la impresora de tickets (80 mm si no hay una configurada).
func (handler *service) previewSource(source cashDocumentSource) (any, error) {
	_, width, err := handler.cashPrinter()
	if err != nil {
		return nil, err
	}
	includeExpected := handler.reportIncludesExpected(source)
	content, err := handler.renderCashDocument(source, includeExpected, width)
	if err != nil {
		return nil, err
	}
	existing, err := handler.existingDocument(source)
	if err != nil {
		return nil, err
	}
	columns := 80
	if width == paperWidth(58) {
		columns = 58
	}
	return CashDocumentPreview{
		Kind: source.kind, Title: source.title(), Content: content, IncludesExpected: includeExpected, PaperWidth: columns,
		Document: existing,
	}, nil
}

func (handler *service) previewReportDocument(parts []string) (any, error) {
	source, err := handler.reportSource(parts[1])
	if err != nil {
		return nil, err
	}
	return handler.previewSource(source)
}

func (handler *service) previewOperationDocument(parts []string) (any, error) {
	source, err := handler.operationSource(parts[1])
	if err != nil {
		return nil, err
	}
	return handler.previewSource(source)
}

func (handler *service) printReportDocument(parts []string) (any, error) {
	if err := decodeOperationOnly(handler.request.Body); err != nil {
		return nil, err
	}
	source, err := handler.reportSource(parts[1])
	if err != nil {
		return nil, err
	}
	return handler.printLater(source)
}

func (handler *service) printOperationDocument(parts []string) (any, error) {
	if err := decodeOperationOnly(handler.request.Body); err != nil {
		return nil, err
	}
	source, err := handler.operationSource(parts[1])
	if err != nil {
		return nil, err
	}
	return handler.printLater(source)
}

// decodeOperationOnly valida el cuerpo de una acción que solo lleva su operation_id.
func decodeOperationOnly(body []byte) error {
	var input struct {
		OperationID string `json:"operation_id"`
	}
	return decode(body, &input)
}

// printLater imprime el documento de un corte u operación que aún no lo tiene (por ejemplo,
// porque no se pidió al registrarlo o no había impresora). Si ya existe, la copia es una
// reimpresión con su propio permiso. Nunca crea movimientos ni toca el turno o su cierre.
func (handler *service) printLater(source cashDocumentSource) (any, error) {
	existing, err := handler.existingDocument(source)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, conflict("CASH_DOCUMENT_EXISTS", "Este documento ya se imprimió. Para otra copia usa «Reimprimir».")
	}
	printerID, width, err := handler.cashPrinter()
	if err != nil {
		return nil, err
	}
	if printerID == nil {
		return nil, conflict("PRINTER_NOT_CONFIGURED", "No hay impresora de tickets configurada en Cobros.")
	}
	outcome, err := handler.enqueueCashDocument(source, *printerID, width)
	if err != nil {
		return nil, err
	}
	return outcome, handler.audit(handler.ctx, handler.tx, "cash.document_printed", "cash_shift", &source.shift.ID, nil, map[string]any{
		"document_id": outcome.DocumentID, "kind": source.kind, "job_id": outcome.JobID, "includes_expected": outcome.IncludesExpected,
	})
}

// reprintCashDocument solicita otra copia idéntica de un documento de caja, sin crear movimientos
// ni volver a cerrar el turno. Un documento con el efectivo esperado exige poder consultarlo y no
// se reimprime durante el arqueo de cierre.
func (handler *service) reprintCashDocument(parts []string) (any, error) {
	if err := decodeOperationOnly(handler.request.Body); err != nil {
		return nil, err
	}
	documents, err := handler.listCashDocuments("d.id=$1", parts[1])
	if err != nil {
		return nil, err
	}
	if len(documents) == 0 {
		return nil, notFound("El documento de caja no existe.")
	}
	document := documents[0]
	shift, err := handler.readableShift(document.ShiftID)
	if err != nil {
		return nil, err
	}
	if document.IncludesExpected {
		if err := handler.request.Actor.Require(permissionExpectedRead); err != nil {
			return nil, err
		}
		if shift.State == shiftClosing {
			return nil, conflict("SHIFT_CLOSING", "El documento contiene el efectivo esperado y "+shift.RegisterName+
				" está en arqueo de cierre: se reimprime cuando termine el arqueo.")
		}
	}
	job, err := kitchen.ReprintCommercial(handler.ctx, handler.tx, document.TicketID, handler.operationID, handler.request.Actor.UserID,
		handler.request.Actor.DeviceID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.document.reprinted", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	result := PrintOutcome{
		Requested: true, Status: job.State, DocumentID: document.ID, TicketID: document.TicketID, JobID: job.ID,
		IncludesExpected: document.IncludesExpected, Message: job.LastError,
	}
	return result, handler.audit(handler.ctx, handler.tx, "cash.document_reprinted", "cash_shift", &shift.ID, nil, map[string]any{
		"document_id": document.ID, "kind": document.Kind, "job_id": job.ID,
	})
}

// shiftDocuments lista los documentos de caja de un turno con el estado de su último trabajo.
func (handler *service) shiftDocuments(parts []string) (any, error) {
	shift, err := handler.readableShift(parts[1])
	if err != nil {
		return nil, err
	}
	documents, err := handler.listCashDocuments("d.shift_id=$1", shift.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"shift_id": shift.ID, "documents": documents}, nil
}

const cashDocumentQuery = `SELECT d.id,d.kind,d.shift_id,d.report_id,d.cash_operation_id,d.ticket_id,d.includes_expected,
    u.display_name,d.created_at,latest.id,latest.state,latest.printer_name,
    (SELECT count(*) FROM app.kitchen_print_jobs j WHERE j.ticket_id=d.ticket_id)::int
    FROM app.cash_documents d JOIN app.users u ON u.id=d.created_by
    LEFT JOIN LATERAL (SELECT j.id,j.state,p.name AS printer_name FROM app.kitchen_print_jobs j
        JOIN app.kitchen_printers p ON p.id=j.printer_id WHERE j.ticket_id=d.ticket_id ORDER BY j.created_at DESC,j.id DESC LIMIT 1) latest ON true`

// listCashDocuments devuelve los documentos que cumplen la condición, en orden cronológico.
func (handler *service) listCashDocuments(where string, argument any) ([]CashDocumentRecord, error) {
	rows, err := handler.tx.Query(handler.ctx, cashDocumentQuery+" WHERE "+where+" ORDER BY d.created_at,d.id", argument)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCashDocuments(rows)
}

func scanCashDocuments(rows pgx.Rows) ([]CashDocumentRecord, error) {
	result := []CashDocumentRecord{}
	for rows.Next() {
		var item CashDocumentRecord
		if err := rows.Scan(&item.ID, &item.Kind, &item.ShiftID, &item.ReportID, &item.CashOperationID, &item.TicketID,
			&item.IncludesExpected, &item.CreatedByName, &item.CreatedAt, &item.JobID, &item.JobState, &item.PrinterName,
			&item.Copies); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
