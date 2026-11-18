package payments

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
)

// Impresión de precuentas, tickets y documentos de caja desde la etapa 9F (§4.1, §4.2 y §4.5):
//   - impresora válida (decisión D4): la impresora de tickets de Cobros debe estar configurada, activa y con
//     un transporte que ya imprime; si no, se avisa antes de encolar (precuenta y «Imprimir después» de
//     caja: 409 PRINTER_NOT_CONFIGURED o PRINTER_UNAVAILABLE; ticket de pago y documentos al registrar:
//     not_configured con el motivo, sin hacer fallar el pago ni la operación);
//   - reintentar un documento que nunca salió en papel es un toque, sin motivo, con el permiso de imprimirlo;
//   - el nombre visible de la precuenta es una lista cerrada.

// Motivos de una impresora que no sirve para estos documentos (detalle público del rechazo).
const (
	printerMissing = "missing"
)

// printerScopeDevice es details.scope del rechazo sin impresora válida de un dispositivo android (etapa A2, A2-D3): la
// impresora que falta o no sirve es la de ese dispositivo, que se elige en Terminales, no la de Cobros.
const printerScopeDevice = "device"

// Documentos que avisan la falta de impresora (detalle público del rechazo).
const (
	documentPrecheck = "precheck"
	documentReceipt  = "receipt"
	documentCash     = "cash_document"
)

// PrecheckNameInfo es una opción del nombre visible del documento previo al cobro.
type PrecheckNameInfo struct {
	Key string
	// Label es el nombre en botones y avisos; Plural, en «No hay impresora configurada para …»; Title, el
	// encabezado impreso.
	Label  string
	Plural string
	Title  string
}

// PrecheckNames es la lista cerrada de Go y del CHECK payment_settings_precheck_name_check (0022). El
// sistema anterior llamaba «ticket de venta» al documento que se entregaba antes de cobrar.
var PrecheckNames = []PrecheckNameInfo{
	{Key: "precheck", Label: "Precuenta", Plural: "precuentas", Title: "PRECUENTA"},
	{Key: "sales_ticket", Label: "Ticket de venta", Plural: "tickets de venta", Title: "TICKET DE VENTA"},
	{Key: "bill", Label: "Cuenta", Plural: "cuentas", Title: "CUENTA"},
}

// ValidPrecheckName indica si la clave pertenece a la lista cerrada.
func ValidPrecheckName(key string) bool {
	for _, info := range PrecheckNames {
		if info.Key == key {
			return true
		}
	}
	return false
}

// precheckName devuelve la opción de la clave; una clave desconocida es «Precuenta».
func precheckName(key string) PrecheckNameInfo {
	for _, info := range PrecheckNames {
		if info.Key == key {
			return info
		}
	}
	return PrecheckNames[0]
}

// printedPrecheckTitle es el encabezado impreso que guarda una precuenta nueva: vacío con «Precuenta», de
// modo que el documento guardado y su texto son idénticos a los anteriores a la etapa 9F.
func printedPrecheckTitle(key string) string {
	info := precheckName(key)
	if info.Key == PrecheckNames[0].Key {
		return ""
	}
	return info.Title
}

// receiptPrinter devuelve la impresora de tickets de Cobros y, si no puede imprimir, el motivo: missing (sin
// configurar), inactive (desactivada) o unsupported (de otra terminal sin terminal elegida).
//
// Etapa 9G (fase E, decisión D10): cada caja puede tener su propia impresora de tickets (por ejemplo, la USB de la
// Caja 2 por medio de su agente). Precuentas, tickets de pago, documentos de caja y sus reintentos y reimpresiones
// salen en la impresora de la caja de la terminal que los pide; si esa caja no tiene una propia (o la terminal no
// tiene caja), en la impresora de tickets general, como hasta ahora.
//
// Etapa A2 (parte C, decisión A2-D3): un teléfono o tableta de meseros (dispositivo android) imprime solo en su
// «Impresora de precuentas» (devices.precheck_printer_id, Terminales › Editar). Sin ella el motivo es missing con el
// ámbito del dispositivo (deviceScoped): nunca pasa a la impresora de su caja ni a la general de Cobros, para que el
// mesero no mande su precuenta a otra impresora sin saberlo. Reintentar y reimprimir usan esta misma resolución.
func (handler *service) receiptPrinter() (*kitchen.Printer, string, error) {
	var devicePrinter, registerPrinter *string
	err := handler.tx.QueryRow(handler.ctx, `SELECT d.kind,d.precheck_printer_id::text,rp.printer_id::text FROM app.devices d
        LEFT JOIN app.register_receipt_printers rp ON rp.register_id=d.register_id WHERE d.id=$1`,
		handler.request.Actor.DeviceID).Scan(&handler.requesterKind, &devicePrinter, &registerPrinter)
	if err != nil && !isNoRows(err) {
		return nil, "", err
	}
	printerID := registerPrinter
	if handler.requesterKind == "android" {
		printerID = devicePrinter
		if printerID == nil {
			return nil, printerMissing, nil
		}
	}
	if printerID == nil {
		settings, err := handler.settings()
		if err != nil {
			return nil, "", err
		}
		printerID = settings.ReceiptPrinterID
	}
	if printerID == nil {
		return nil, printerMissing, nil
	}
	printer, err := kitchen.ReadPrinter(handler.ctx, handler.tx, *printerID)
	if err != nil {
		return nil, "", err
	}
	return printer, kitchen.PrinterProblem(printer), nil
}

// deviceScoped dice si la impresora de los documentos de quien pide es la de su dispositivo (un android, A2-D3). Se
// sabe después de receiptPrinter, que lee el tipo del dispositivo.
func (handler *service) deviceScoped() bool {
	return handler.requesterKind == "android"
}

// printerUnavailableError es el rechazo de un documento sin impresora válida, con el documento y el motivo
// en los detalles públicos para que la interfaz ofrezca «Configurar impresora» y «Reintentar». Desde la etapa A2,
// details.scope es «device» si la impresora es la del dispositivo que pide (se elige en Terminales).
func (handler *service) printerUnavailableError(document, problem string, printer *kitchen.Printer) error {
	details := map[string]string{"document": document, "problem": problem}
	if handler.deviceScoped() {
		details["scope"] = printerScopeDevice
	}
	if problem == printerMissing {
		return &Error{Status: 409, Code: "PRINTER_NOT_CONFIGURED", Message: handler.missingPrinterMessage(document), Details: details}
	}
	details["printer_name"] = printer.Name
	return &Error{Status: 409, Code: "PRINTER_UNAVAILABLE", Message: kitchen.UnavailablePrinterMessage(problem, printer.Name), Details: details}
}

// missingPrinterMessage dice qué documento no tiene impresora, con el nombre visible de la precuenta. En un
// dispositivo android (A2-D3) dice que falta la impresora del dispositivo y quién la asigna.
func (handler *service) missingPrinterMessage(document string) string {
	if handler.deviceScoped() {
		return "Este dispositivo no tiene impresora asignada. Pide al administrador que la elija en Terminales."
	}
	switch document {
	case documentPrecheck:
		settings, err := handler.settings()
		name := PrecheckNames[0]
		if err == nil {
			name = precheckName(settings.PrecheckName)
		}
		return "No hay impresora configurada para " + name.Plural + "."
	case documentReceipt:
		return "No hay impresora configurada para tickets de pago."
	}
	return "No hay impresora configurada para los documentos de caja."
}

// unavailableOutcome describe, sin rechazar la operación, que un ticket de pago o un documento de caja no se
// encoló por falta de impresora válida: el pago o la operación ya quedaron registrados.
func (handler *service) unavailableOutcome(document, problem string, printer *kitchen.Printer, subject string) PrintOutcome {
	outcome := PrintOutcome{Requested: true, Status: "not_configured", Problem: problem}
	reason := handler.missingPrinterMessage(document)
	if problem != printerMissing {
		reason = kitchen.UnavailablePrinterMessage(problem, printer.Name)
		outcome.PrinterName = printer.Name
	}
	outcome.Message = strings.TrimSuffix(reason, ".") + "; " + subject + " quedó registrado."
	return outcome
}

// printReceipt imprime el ticket de un pago que no lo tiene (POST payments/accounts/{id}/documents/receipt,
// etapa 9F): «Reintentar» del aviso de un ticket que no se imprimió por falta de impresora válida. Exige
// payments.tickets.print; si el ticket ya existe, se reimprime con su propio permiso y motivo. Desde la
// revisión 9F (B1) solo procede si el ticket quedó sin imprimir por falta de impresora válida al registrar el
// pago (la respuesta guardada de su operation_id dice not_configured) y el pago no tiene devoluciones: no es
// un camino para emitir días después el comprobante de cualquier pago sin motivo.
func (handler *service) printReceipt(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		PaymentID   string `json:"payment_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.PaymentID); err != nil {
		return nil, err
	}
	finance, err := loadFinance(handler.ctx, handler.tx, parts[1])
	if err != nil {
		return nil, err
	}
	var payment *PaymentRecord
	for index := range finance.Payments {
		if finance.Payments[index].ID == input.PaymentID {
			payment = &finance.Payments[index]
		}
	}
	if payment == nil {
		return nil, notFound("El pago no pertenece a esta cuenta.")
	}
	var printed bool
	if err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.payment_documents WHERE payment_id=$1 AND kind='receipt')",
		payment.ID).Scan(&printed); err != nil {
		return nil, err
	}
	if printed {
		return nil, conflict("DOCUMENT_EXISTS", "Este ticket ya se imprimió. Para otra copia usa «Reimprimir ticket de pago».")
	}
	pending, err := handler.receiptPendingForPrinter(payment.ID)
	if err != nil {
		return nil, err
	}
	if !pending {
		return nil, conflict("RECEIPT_NOT_PENDING", "Este pago no tiene un ticket pendiente: aquí solo se imprime el ticket que no "+
			"salió al cobrar por falta de una impresora válida. Para otra copia de un ticket ya impreso usa «Reimprimir ticket de pago».")
	}
	if decimal.RequireFromString(payment.RefundedAmount).Sign() != 0 || payment.Corrected {
		return nil, conflict("PAYMENT_REFUNDED", "El pago tiene devoluciones registradas: su ticket ya no se imprime desde aquí.")
	}
	printer, problem, err := handler.receiptPrinter()
	if err != nil {
		return nil, err
	}
	if problem != "" {
		return nil, handler.printerUnavailableError(documentReceipt, problem, printer)
	}
	result, err := handler.enqueueDocument(finance, "receipt", payment)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.document.printed", handler.scope(finance), &finance.AccountID, nil); err != nil {
		return nil, err
	}
	return result, handler.audit(handler.ctx, handler.tx, "payments.receipt_printed", "account", &finance.AccountID, nil, result)
}

// receiptPendingForPrinter indica si el ticket del pago quedó sin imprimir por falta de impresora válida: la
// respuesta guardada del cobro (restaurant_operations, por su operation_id) informa not_configured.
func (handler *service) receiptPendingForPrinter(paymentID string) (bool, error) {
	var status string
	err := handler.tx.QueryRow(handler.ctx, `SELECT coalesce(o.response->'printing'->>'status','') FROM app.payments p
        JOIN app.restaurant_operations o ON o.id=p.operation_id WHERE p.id=$1`, paymentID).Scan(&status)
	if isNoRows(err) {
		return false, nil
	}
	return status == "not_configured", err
}

// retryDocument vuelve a enviar a la impresora una precuenta o un ticket que nunca salió en papel (POST
// payments/accounts/{id}/documents/retry, etapa 9F): un toque, sin motivo, con el permiso de imprimir
// tickets y precuentas, sin pasar por la cola de cocina (que exige operar todas las estaciones).
func (handler *service) retryDocument(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		TicketID    string `json:"ticket_id"`
		JobID       string `json:"job_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.TicketID); err != nil {
		return nil, err
	}
	var kind string
	err := handler.tx.QueryRow(handler.ctx, "SELECT kind FROM app.payment_documents WHERE ticket_id=$1 AND account_id=$2",
		input.TicketID, parts[1]).Scan(&kind)
	if isNoRows(err) {
		return nil, notFound("El documento no pertenece a esta cuenta.")
	}
	if err != nil {
		return nil, err
	}
	job, err := handler.retryJob(input.TicketID, input.JobID, commercialDocument(kind))
	if err != nil {
		return nil, err
	}
	accountID := parts[1]
	var mode string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT mode FROM app.restaurant_accounts WHERE id=$1", accountID).Scan(&mode); err != nil {
		return nil, err
	}
	scope := "restaurant"
	if mode == "takeaway" {
		scope = "takeaway"
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.document.printed", scope, &accountID, nil); err != nil {
		return nil, err
	}
	result := map[string]any{"status": job.State, "job_id": job.ID, "ticket_id": input.TicketID}
	if err := handler.addJobFeedback(result, job.ID); err != nil {
		return nil, err
	}
	return result, handler.audit(handler.ctx, handler.tx, "payments.document_retried", "account", &accountID, nil, result)
}

// addJobFeedback agrega a la respuesta de una impresión lo que Go ya sabe del trabajo recién encolado (etapa A2, parte
// C): problem, la clave del problema que ya tiene (agent_offline si la terminal de la impresora no consulta, ya al
// encolar; o el de un trabajo que no pudo salir), y printer_name y terminal_name (la terminal, si es la impresora USB
// de una terminal), para que el teléfono diga «Enviada a Caja 1» o «La Caja 2 está desconectada». Lo que pase después
// (por ejemplo, que la impresora de red no responda) se consulta con documents/status.
func (handler *service) addJobFeedback(result map[string]any, jobID string) error {
	var problem, printerName, terminalName *string
	err := handler.tx.QueryRow(handler.ctx, `SELECT `+kitchen.JobProblemSQL("j")+`,p.name,`+kitchen.JobTerminalSQL("j")+`
        FROM app.kitchen_print_jobs j LEFT JOIN app.kitchen_printers p ON p.id=j.printer_id WHERE j.id=$1`, jobID).
		Scan(&problem, &printerName, &terminalName)
	if err != nil {
		return err
	}
	result["problem"], result["printer_name"], result["terminal_name"] = problem, printerName, terminalName
	return nil
}

// validReprintReason exige el motivo (5–500 caracteres) de una copia de un documento ya emitido: reimprimirlo
// (decisión D3) o imprimirlo con la plantilla vigente (revisión 9F, B2). Se valida antes de buscar la
// impresora, con el mismo texto que la cola de cocina.
func validReprintReason(reason string) error {
	if len(strings.TrimSpace(reason)) < 5 || len(reason) > 500 {
		return invalid("Indica un motivo de entre 5 y 500 caracteres.")
	}
	return nil
}

// commercialDocument es el documento que avisa la falta de impresora para una precuenta o un ticket de pago.
func commercialDocument(kind string) string {
	if kind == "precheck" {
		return documentPrecheck
	}
	return documentReceipt
}

// documentPrinter devuelve la impresora de tickets de Cobros con su configuración vigente para volver a
// imprimir un documento ya emitido (reintento o reimpresión, revisión 9F M2): es la impresora de esos
// documentos (decisión D4) y la que se corrige con «Configurar impresora». Sin una válida, el rechazo con el
// motivo para que el aviso ofrezca «Configurar impresora».
func (handler *service) documentPrinter(document string) (*kitchen.Printer, error) {
	printer, problem, err := handler.receiptPrinter()
	if err != nil {
		return nil, err
	}
	if problem != "" {
		return nil, handler.printerUnavailableError(document, problem, printer)
	}
	return printer, nil
}

// retryJob reintenta el trabajo indicado (o, sin él, el más reciente) del documento en la impresora de tickets
// con su configuración vigente (revisión 9F, M2); document dice qué documento es para el aviso sin impresora.
func (handler *service) retryJob(ticketID, jobID, document string) (*kitchen.PrintJob, error) {
	if jobID == "" {
		err := handler.tx.QueryRow(handler.ctx, `SELECT id FROM app.kitchen_print_jobs WHERE ticket_id=$1
            ORDER BY created_at DESC,id DESC LIMIT 1`, ticketID).Scan(&jobID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, conflict("PRINT_NOT_FAILED", "El documento no tiene una impresión fallida que reintentar.")
		}
		if err != nil {
			return nil, err
		}
	}
	printer, err := handler.documentPrinter(document)
	if err != nil {
		return nil, err
	}
	return kitchen.RetryDocument(handler.ctx, handler.tx, kitchen.DocumentRetry{
		JobID: jobID, TicketID: ticketID, OperationID: handler.operationID,
		ActorID: handler.request.Actor.UserID, DeviceID: handler.request.Actor.DeviceID,
		Printer: printer, Renderer: RenderStoredDocument,
	})
}
