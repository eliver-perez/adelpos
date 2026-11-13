package payments

import (
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
)

// Documentos de caja (etapa 7, §20): comprobantes de ingreso, retiro y gasto, corte parcial y corte
// definitivo. Desde la etapa 9B Go los renderiza con el renderizador único de internal/documents y
// la plantilla activa de su tipo, y los imprime por la cola durable de la etapa 5 (kitchen_tickets
// con tipo cash_report o cash_voucher), en la impresora de tickets de Cobros.
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
	// Problem y PrinterName explican un documento no encolado por falta de impresora válida (etapa
	// 9F, decisión D4): missing, inactive o unsupported, y la impresora de tickets de Cobros.
	Problem     string `json:"problem,omitempty"`
	PrinterName string `json:"printer_name,omitempty"`
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
	// JobProblem es la clave del problema del último trabajo si exige atención (etapa 9F).
	JobProblem  *string `json:"job_problem"`
	PrinterName *string `json:"printer_name"`
	Copies      int     `json:"copies"`
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

// documentTitle es el encabezado impreso de cada tipo de documento.
func documentTitle(kind string, sequence int) string {
	documentKind, found := documents.CashKind(kind)
	if !found {
		documentKind = documents.KindCashExpense
	}
	return documents.CashTitle(documentKind, sequence)
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

// renderedCash es un documento de caja renderizado con la plantilla activa de su tipo: la IR, los
// datos estructurados que guarda el ticket (etapa 9B), la versión usada y si incluye lo protegido.
type renderedCash struct {
	layout          *documents.Layout
	data            documents.CashData
	version         *documents.Version
	includeExpected bool
}

// renderCashSource renderiza un comprobante o corte al ancho indicado (papel y columnas de la
// impresora de tickets) con la plantilla activa de su tipo. El esperado y las diferencias solo se
// incluyen para quien puede consultarlos y fuera del arqueo; si no se incluyen, tampoco quedan en
// los datos estructurados.
func (handler *service) renderCashSource(source cashDocumentSource, width documents.RenderOptions) (*renderedCash, error) {
	header, err := handler.readDocumentHeader()
	if err != nil {
		return nil, err
	}
	kind, found := documents.CashKind(source.kind)
	if !found {
		return nil, invalid("El tipo de documento de caja no existe.")
	}
	includeExpected := handler.reportIncludesExpected(source)
	data := cashData(header, source, includeExpected)
	identity, err := documents.LoadIdentity(handler.ctx, handler.tx)
	if err != nil {
		return nil, err
	}
	identity.Name = header.establishment
	data.Identity = identity
	version, err := documents.ActiveVersion(handler.ctx, handler.tx, kind)
	if err != nil {
		return nil, err
	}
	options := width
	options.IncludeExpected = includeExpected
	layout, err := documents.Render(kind, version.Template, documents.Data{Cash: &data}, options)
	if err != nil {
		return nil, err
	}
	return &renderedCash{layout: layout, data: data, version: version, includeExpected: includeExpected}, nil
}

// cashPrinter devuelve la impresora de tickets configurada en Cobros (el mismo criterio de los
// documentos comerciales de la etapa 6) y su ancho: papel y, desde la etapa 9C, columnas (80 mm con
// las predeterminadas si no hay una configurada).
// Desde la etapa 9F también devuelve por qué no puede imprimir (missing, inactive o unsupported).
func (handler *service) cashPrinter() (*kitchen.Printer, string, documents.RenderOptions, error) {
	printer, problem, err := handler.receiptPrinter()
	if err != nil {
		return nil, "", documents.RenderOptions{}, err
	}
	if printer == nil {
		return nil, problem, documents.RenderOptions{PaperWidth: 80}, nil
	}
	return printer, problem, printer.RenderOptions(), nil
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
	records, err := handler.listCashDocuments(condition, identifier)
	if err != nil || len(records) == 0 {
		return nil, err
	}
	return &records[0], nil
}

// enqueueCashDocument registra el documento y su trabajo en la cola con la transacción del
// servicio. No valida permisos: lo hacen las acciones que lo llaman, antes de escribir.
func (handler *service) enqueueCashDocument(source cashDocumentSource, printerID string, width documents.RenderOptions) (PrintOutcome, error) {
	rendered, err := handler.renderCashSource(source, width)
	if err != nil {
		return PrintOutcome{}, err
	}
	includeExpected := rendered.includeExpected
	content := rendered.layout.Text()
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
	// Desde la 9B guarda además la versión de plantilla y los datos estructurados (sin el esperado
	// si el documento no lo incluye) para poder imprimirlo después con la plantilla vigente.
	ticket := map[string]any{
		"kind": ticketKind, "document_kind": source.kind, "document_id": documentID, "shift_id": source.shift.ID,
		"report_id": reportID, "cash_operation_id": operationID, "includes_expected": includeExpected,
		"title": source.title(), "content": content, "template_version_id": rendered.version.ID, "data": rendered.data,
	}
	job, ticketID, err := kitchen.EnqueueCommercial(handler.ctx, handler.tx, kitchen.CommercialPrint{
		Kind: ticketKind, Document: ticket, Content: content, Layout: rendered.layout, TemplateVersionID: &rendered.version.ID,
		PrinterID: printerID, OperationID: handler.operationID,
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
		printer, problem, width, err := scoped.cashPrinter()
		if err != nil {
			return nil, err
		}
		// Sin impresora válida (etapa 9F, decisión D4) no se encola: la operación ya quedó registrada y
		// el documento se imprime después desde Caja, una vez configurada.
		if problem != "" {
			return scoped.unavailableOutcome(documentCash, problem, printer, subject), nil
		}
		return scoped.enqueueCashDocument(source, printer.ID, width)
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
	_, _, width, err := handler.cashPrinter()
	if err != nil {
		return nil, err
	}
	rendered, err := handler.renderCashSource(source, width)
	if err != nil {
		return nil, err
	}
	existing, err := handler.existingDocument(source)
	if err != nil {
		return nil, err
	}
	return CashDocumentPreview{
		Kind: source.kind, Title: source.title(), Content: rendered.layout.Text(), IncludesExpected: rendered.includeExpected,
		PaperWidth: rendered.layout.PaperWidth, Document: existing,
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
	printer, problem, width, err := handler.cashPrinter()
	if err != nil {
		return nil, err
	}
	if problem != "" {
		return nil, handler.printerUnavailableError(documentCash, problem, printer)
	}
	outcome, err := handler.enqueueCashDocument(source, printer.ID, width)
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
// Desde la etapa 9F (decisión D3) pide motivo, que queda en la auditoría y en el historial del trabajo.
func (handler *service) reprintCashDocument(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Reason      string `json:"reason"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	document, shift, err := handler.printableCashDocument(parts[1], "se reimprime")
	if err != nil {
		return nil, err
	}
	job, err := kitchen.ReprintCommercial(handler.ctx, handler.tx, document.TicketID, input.Reason, handler.operationID,
		handler.request.Actor.UserID, handler.request.Actor.DeviceID)
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
		"document_id": document.ID, "kind": document.Kind, "job_id": job.ID, "reason": input.Reason,
	})
}

// printableCashDocument lee un documento de caja con las reglas de consulta de su turno. Un documento con
// el efectivo esperado exige poder consultarlo y no se imprime durante el arqueo de cierre.
func (handler *service) printableCashDocument(documentID, action string) (CashDocumentRecord, *Shift, error) {
	documents, err := handler.listCashDocuments("d.id=$1", documentID)
	if err != nil {
		return CashDocumentRecord{}, nil, err
	}
	if len(documents) == 0 {
		return CashDocumentRecord{}, nil, notFound("El documento de caja no existe.")
	}
	document := documents[0]
	shift, err := handler.readableShift(document.ShiftID)
	if err != nil {
		return CashDocumentRecord{}, nil, err
	}
	if document.IncludesExpected {
		if err := handler.request.Actor.Require(permissionExpectedRead); err != nil {
			return CashDocumentRecord{}, nil, err
		}
		if shift.State == shiftClosing {
			return CashDocumentRecord{}, nil, conflict("SHIFT_CLOSING", "El documento contiene el efectivo esperado y "+shift.RegisterName+
				" está en arqueo de cierre: "+action+" cuando termine el arqueo.")
		}
	}
	return document, shift, nil
}

// retryCashDocument vuelve a enviar a la impresora un documento de caja que nunca salió en papel (POST
// payments/cash-documents/{id}/retry, etapa 9F): un toque, sin motivo, con el permiso de imprimir
// documentos de caja y las reglas de consulta de su turno.
func (handler *service) retryCashDocument(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		JobID       string `json:"job_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	document, shift, err := handler.printableCashDocument(parts[1], "se imprime")
	if err != nil {
		return nil, err
	}
	job, err := handler.retryJob(document.TicketID, input.JobID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.document.printed", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	result := PrintOutcome{
		Requested: true, Status: job.State, DocumentID: document.ID, TicketID: document.TicketID, JobID: job.ID,
		IncludesExpected: document.IncludesExpected, Message: job.LastError,
	}
	return result, handler.audit(handler.ctx, handler.tx, "cash.document_retried", "cash_shift", &shift.ID, nil, map[string]any{
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

var cashDocumentQuery = `SELECT d.id,d.kind,d.shift_id,d.report_id,d.cash_operation_id,d.ticket_id,d.includes_expected,
    u.display_name,d.created_at,latest.id,latest.state,latest.problem,latest.printer_name,
    (SELECT count(*) FROM app.kitchen_print_jobs j WHERE j.ticket_id=d.ticket_id)::int
    FROM app.cash_documents d JOIN app.users u ON u.id=d.created_by
    LEFT JOIN LATERAL (SELECT j.id,j.state,` + jobProblemColumn + ` AS problem,p.name AS printer_name FROM app.kitchen_print_jobs j
        JOIN app.kitchen_printers p ON p.id=j.printer_id WHERE j.ticket_id=d.ticket_id ORDER BY j.created_at DESC,j.id DESC LIMIT 1) latest ON true`

// jobProblemColumn es la clave del problema del trabajo (alias j, etapa 9F).
var jobProblemColumn = kitchen.JobProblemSQL("j")

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
			&item.IncludesExpected, &item.CreatedByName, &item.CreatedAt, &item.JobID, &item.JobState, &item.JobProblem, &item.PrinterName,
			&item.Copies); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
