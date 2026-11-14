package kitchen

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
)

// Documentos comerciales (precuentas y tickets de pago) y documentos de caja (comprobantes y
// cortes, etapa 7) creados por el módulo de pagos. Se imprimen por la misma cola durable de la
// etapa 5; su contenido llega ya renderizado y nunca se reinterpreta como comanda. El comando de
// apertura del cajón (cash_drawer) también viaja por la cola, pero no es un documento: su único
// contenido binario lo genera el controlador ESC/POS.

// Tipos de ticket de caja (etapa 7).
const (
	KindCashReport  = "cash_report"
	KindCashVoucher = "cash_voucher"
	KindCashDrawer  = "cash_drawer"
)

// isCommercialKind indica un documento ya renderizado que se imprime y reimprime tal cual. Desde la
// etapa 9B incluye las muestras del diseñador y los históricos impresos con la plantilla vigente.
func isCommercialKind(kind string) bool {
	return kind == "receipt" || kind == "precheck" || kind == KindCashReport || kind == KindCashVoucher || isDesignerKind(kind)
}

// isDesignerKind indica una muestra del diseñador o un histórico impreso con la plantilla vigente.
func isDesignerKind(kind string) bool {
	return kind == documents.TicketKindSample || kind == documents.TicketKindRerender
}

// isCashKind indica un ticket que pertenece a la administración de caja: sus copias y su
// contenido se gobiernan desde Caja, con los permisos de caja, y no desde la cola de cocina.
func isCashKind(kind string) bool {
	return kind == KindCashReport || kind == KindCashVoucher || kind == KindCashDrawer
}

// renderedKinds es la lista SQL de los tipos que no son comandas: no aparecen en su listado.
const renderedKinds = "'receipt','precheck','cash_report','cash_voucher','cash_drawer','document_sample','document_rerender'"

// CommercialPrint es un documento comercial o de caja por encolar. Layout es su IR (etapa 9B): el
// contenido del trabajo es su texto y, si lleva atributos o imágenes, el trabajo guarda la IR. Sin
// Layout se encola Content como texto plano.
type CommercialPrint struct {
	Kind     string
	Document any
	Content  string
	Layout   *documents.Layout
	// TemplateVersionID es la versión de plantilla con la que se renderizó (etapa 9B).
	TemplateVersionID *string
	PrinterID         string
	OperationID       string
	ActorID           string
	DeviceID          string
}

// sanitizeContent elimina controles de usuario: solo el controlador del transporte genera comandos.
func sanitizeContent(content string) string {
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		lines[index] = cleanText(line)
	}
	return strings.Join(lines, "\n")
}

// EnqueueCommercial guarda el documento y su trabajo de impresión en la transacción del pago. Un
// fallo posterior de la impresora nunca revierte el pago: el trabajo queda en la cola con su estado.
func EnqueueCommercial(ctx context.Context, tx pgx.Tx, input CommercialPrint) (*PrintJob, string, error) {
	if !isCommercialKind(input.Kind) || isDesignerKind(input.Kind) {
		return nil, "", invalid("Tipo de documento comercial no válido.")
	}
	printer, err := one[Printer](ctx, tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", input.PrinterID)
	if err != nil {
		return nil, "", err
	}
	ticketID := events.NewID()
	if _, err := tx.Exec(ctx, "INSERT INTO app.kitchen_tickets(id,kind,station_ids,document,template_version_id) VALUES ($1,$2,'{}',$3,$4)",
		ticketID, input.Kind, encoded(input.Document), input.TemplateVersionID); err != nil {
		return nil, "", err
	}
	handler := service{ctx: ctx, tx: tx, operationID: input.OperationID, request: Request{Actor: Actor{UserID: input.ActorID, DeviceID: input.DeviceID}}}
	var job *PrintJob
	if input.Layout != nil {
		job, err = handler.enqueueLayout(ticketID, *printer, nil, input.Layout)
	} else {
		job, err = handler.enqueueContent(ticketID, *printer, nil, sanitizeContent(input.Content))
	}
	if err != nil {
		return nil, "", err
	}
	return job, ticketID, handler.history("print."+input.Kind+"_requested", &job.ID, nil, job)
}

// Impresora válida para precuentas, tickets y documentos de caja (etapa 9F, §4.1, decisión D4): configurada,
// activa y con un transporte que ya imprime. Sin ella, el módulo de pagos avisa en el momento con «Configurar
// impresora» y no encola un trabajo que no podría salir. Desde la etapa 9G (fase E) la impresora USB de una
// terminal es válida (la imprime su agente; si la terminal está apagada, el trabajo espera); una «agent» sin
// terminal elegida (anterior a la 9G) sigue sin serlo.

// PrinterProblem devuelve por qué una impresora no puede imprimir esos documentos: ProblemInactive o
// ProblemUnsupported; vacío si puede.
func PrinterProblem(printer *Printer) string {
	if !printer.Active {
		return ProblemInactive
	}
	if printer.Transport == "agent" && !printer.AgentPrinter() {
		return ProblemUnsupported
	}
	return ""
}

// UnavailablePrinterError es el rechazo 409 PRINTER_UNAVAILABLE de una impresora que no puede imprimir, con
// el motivo y su nombre en los detalles públicos.
func UnavailablePrinterError(problem, name string) error {
	return &Error{Status: 409, Code: "PRINTER_UNAVAILABLE", Message: UnavailablePrinterMessage(problem, name),
		Details: map[string]string{"problem": problem, "printer_name": name}}
}

// UnavailablePrinterMessage es el texto del motivo para el restaurante.
func UnavailablePrinterMessage(problem, name string) string {
	if problem == ProblemInactive {
		return inactivePrinterMessage(name)
	}
	return unsupportedPrinterMessage
}

// RenderedPrint es una muestra del diseñador o un histórico impreso con la plantilla vigente (etapa
// 9B). Printer, si existe, es la fotografía de la impresora del documento original y prevalece
// sobre PrinterID (la impresora vigente elegida para una muestra).
type RenderedPrint struct {
	Kind     string
	Document any
	Layout   *documents.Layout
	// TemplateVersionID es la versión vigente usada (nil en una muestra, cuyo layout no se guardó).
	TemplateVersionID *string
	PrinterID         string
	Printer           *Printer
	History           string
	// Reason es el motivo de una impresión con la plantilla vigente (otra copia de un documento ya emitido,
	// revisión 9F B2): queda en el historial del trabajo junto con él. Vacío en las muestras.
	Reason      string
	OperationID string
	ActorID     string
	DeviceID    string
}

// EnqueueRendered guarda el ticket y su trabajo en la cola durable de siempre, con el mismo lease,
// control de duplicados y resultados inciertos. Nunca crea ventas, pagos, movimientos ni documentos
// comerciales o de caja: solo el ticket del tipo indicado y su trabajo.
func EnqueueRendered(ctx context.Context, tx pgx.Tx, input RenderedPrint) (*PrintJob, string, error) {
	if !isDesignerKind(input.Kind) || input.Layout == nil {
		return nil, "", invalid("Tipo de documento renderizado no válido.")
	}
	printer := input.Printer
	if printer == nil {
		found, err := one[Printer](ctx, tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", input.PrinterID)
		if err != nil {
			return nil, "", err
		}
		printer = found
	}
	ticketID := events.NewID()
	if _, err := tx.Exec(ctx, "INSERT INTO app.kitchen_tickets(id,kind,station_ids,document,template_version_id) VALUES ($1,$2,'{}',$3,$4)",
		ticketID, input.Kind, encoded(input.Document), input.TemplateVersionID); err != nil {
		return nil, "", err
	}
	handler := service{ctx: ctx, tx: tx, operationID: input.OperationID, request: Request{Actor: Actor{UserID: input.ActorID, DeviceID: input.DeviceID}}}
	job, err := handler.enqueueLayout(ticketID, *printer, nil, input.Layout)
	if err != nil {
		return nil, "", err
	}
	if input.Reason != "" {
		return job, ticketID, handler.history(input.History, &job.ID, nil, map[string]any{"job": job, "reason": input.Reason})
	}
	return job, ticketID, handler.history(input.History, &job.ID, nil, job)
}

// CommercialReprint es una reimpresión con motivo de una precuenta, un ticket o un documento de caja (etapa 9F,
// decisión D3). Printer es la impresora de esos documentos con su configuración vigente (la impresora de
// tickets de Cobros, ya validada por pagos); sin ella, la del trabajo original con su configuración vigente.
type CommercialReprint struct {
	TicketID    string
	Reason      string
	OperationID string
	ActorID     string
	DeviceID    string
	Printer     *Printer
}

// ReprintCommercial solicita otra copia idéntica de un documento comercial. Como en las comandas,
// se rechaza mientras exista otra copia pendiente o sin resolver del documento, exacta o con la
// plantilla vigente. Desde la revisión 9F (M2) la copia sale con la configuración vigente de la impresora
// (destino, transporte, corte y copias), no con la fotografía del original, que podía apuntar a una cola o
// una dirección ya corregidas; como reimpresión exacta (decisión de la 9C) conserva el contenido y el papel y
// las columnas con que se emitió, para que salga idéntica.
func ReprintCommercial(ctx context.Context, tx pgx.Tx, input CommercialReprint) (*PrintJob, error) {
	// Etapa 9F (decisión D3): reimprimir un documento que ya salió (o se canceló) pide motivo, como la
	// copia adicional de la cola; queda en el historial del trabajo y en la auditoría de quien lo pide.
	if err := validReason(input.Reason); err != nil {
		return nil, err
	}
	var kind string
	if err := tx.QueryRow(ctx, "SELECT kind FROM app.kitchen_tickets WHERE id=$1", input.TicketID).Scan(&kind); err != nil {
		return nil, err
	}
	if !isCommercialKind(kind) {
		return nil, invalid("El documento no es un ticket, una precuenta ni un documento de caja.")
	}
	original, err := one[PrintJob](ctx, tx, `SELECT to_jsonb(j) FROM app.kitchen_print_jobs j
        WHERE ticket_id=$1 AND original_job_id IS NULL ORDER BY created_at LIMIT 1`, input.TicketID)
	if err != nil {
		return nil, err
	}
	if original.State != "submitted" && original.State != "cancelled" {
		return nil, invalid("Resuelve primero el trabajo pendiente o incierto para no generar copias descontroladas.")
	}
	// Ni otra copia exacta ni una impresión con la plantilla vigente del mismo documento pueden
	// seguir sin resolver.
	if err := requireNoPendingCopy(ctx, tx, input.TicketID); err != nil {
		return nil, err
	}
	target := input.Printer
	if target == nil {
		target, err = currentPrinter(ctx, tx, original.PrinterID)
		if err != nil {
			return nil, err
		}
	}
	if problem := PrinterProblem(target); problem != "" {
		return nil, UnavailablePrinterError(problem, target.Name)
	}
	printer := *target
	printer.PaperWidth = original.PrinterSnapshot.PaperWidth
	printer.Columns = original.PrinterSnapshot.Columns
	handler := service{ctx: ctx, tx: tx, operationID: input.OperationID, request: Request{Actor: Actor{UserID: input.ActorID, DeviceID: input.DeviceID}}}
	// La copia conserva el contenido y, si lo tiene, el layout con atributos del original.
	job, err := handler.enqueueJob(input.TicketID, printer, &original.ID, original.Content, original.Layout)
	if err != nil {
		return nil, err
	}
	return job, handler.history("print.reprinted", &job.ID, nil, map[string]any{"job": job, "reason": input.Reason})
}

// DrawerPulse es la orden de apertura del cajón: el pin del conector (0 = pin 2, 1 = pin 5). El
// controlador la traduce a ESC p; nunca se imprime texto ni se corta papel.
type DrawerPulse struct {
	Pin int `json:"pin"`
}

// DrawerCommand es un comando de apertura del cajón encolado por el módulo de pagos.
type DrawerCommand struct {
	PrinterID    string
	Pin          int
	RegisterName string
	Reason       string
	OperationID  string
	ActorID      string
	DeviceID     string
}

// CompatibleDrawerTransport indica si un transporte puede entregar los bytes ESC/POS del pulso sin
// alterar: la conexión ESC/POS directa por red o, desde la etapa 7.1, una cola CUPS en modo raw.
// Para CUPS no basta el transporte: la cola debe admitir datos raw (QueueInspector), lo que se
// comprueba al configurar el cajón y otra vez antes de cada envío. No se asume que todo dispositivo
// admite el pulso. La impresora de una terminal se decide con CompatibleDrawerPrinter.
func CompatibleDrawerTransport(transport string) bool {
	return transport == "escpos_tcp" || transport == "cups"
}

// CompatibleDrawerPrinter es CompatibleDrawerTransport con la impresora completa: desde la etapa 9G (fase E) la
// impresora USB de una terminal también abre el cajón de su caja (su agente envía el pulso por la cola de esa
// computadora en modo raw); una «agent» sin terminal, no.
func CompatibleDrawerPrinter(transport string, agentDeviceID *string) bool {
	if transport == "agent" {
		return agentDeviceID != nil && *agentDeviceID != ""
	}
	return CompatibleDrawerTransport(transport)
}

// EnqueueDrawerPulse guarda el comando de apertura del cajón y su trabajo en la transacción de la
// operación. Se rechaza si la impresora no existe, está inactiva o su transporte no es compatible:
// así nunca se encola un comando que no podría llegar al cajón. La cola CUPS no se consulta aquí
// (dentro de la transacción del cobro): el transporte la vuelve a verificar antes de enviar.
func EnqueueDrawerPulse(ctx context.Context, tx pgx.Tx, command DrawerCommand) (*PrintJob, string, error) {
	printer, err := one[Printer](ctx, tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", command.PrinterID)
	if err != nil {
		return nil, "", err
	}
	if !printer.Active {
		return nil, "", &Error{Status: 409, Code: "DRAWER_UNAVAILABLE", Message: "La impresora del cajón está desactivada; no se envió el comando de apertura."}
	}
	if !CompatibleDrawerPrinter(printer.Transport, printer.AgentDeviceID) {
		return nil, "", &Error{Status: 409, Code: "DRAWER_PROTOCOL_UNSUPPORTED", Message: "La impresora del cajón no usa ESC/POS por red, una cola CUPS ni la impresora de una terminal; no se envió el comando de apertura."}
	}
	if command.Pin != 0 && command.Pin != 1 {
		return nil, "", invalid("El pin del cajón debe ser 0 (conector 2) o 1 (conector 5).")
	}
	// Un comando ocupa una sola copia aunque la impresora imprima varias de cada documento.
	printer.Copies = 1
	ticketID := events.NewID()
	document := map[string]any{
		"kind":          KindCashDrawer,
		"register_name": command.RegisterName,
		"reason":        command.Reason,
		"drawer_pulse":  DrawerPulse{Pin: command.Pin},
	}
	if _, err := tx.Exec(ctx, "INSERT INTO app.kitchen_tickets(id,kind,station_ids,document) VALUES ($1,$2,'{}',$3)",
		ticketID, KindCashDrawer, encoded(document)); err != nil {
		return nil, "", err
	}
	handler := service{ctx: ctx, tx: tx, operationID: command.OperationID, request: Request{Actor: Actor{UserID: command.ActorID, DeviceID: command.DeviceID}}}
	// El contenido solo describe el comando en la cola; el transporte nunca lo imprime.
	content := cleanText("Comando de apertura del cajón de " + command.RegisterName)
	job, err := handler.enqueueContent(ticketID, *printer, nil, content)
	if err != nil {
		return nil, "", err
	}
	return job, ticketID, handler.history("print.drawer_requested", &job.ID, nil, job)
}
