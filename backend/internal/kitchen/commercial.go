package kitchen

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
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

// isCommercialKind indica un documento ya renderizado que se imprime y reimprime tal cual.
func isCommercialKind(kind string) bool {
	return kind == "receipt" || kind == "precheck" || kind == KindCashReport || kind == KindCashVoucher
}

// isCashKind indica un ticket que pertenece a la administración de caja: sus copias y su
// contenido se gobiernan desde Caja, con los permisos de caja, y no desde la cola de cocina.
func isCashKind(kind string) bool {
	return kind == KindCashReport || kind == KindCashVoucher || kind == KindCashDrawer
}

// renderedKinds es la lista SQL de los tipos que no son comandas: no aparecen en su listado.
const renderedKinds = "'receipt','precheck','cash_report','cash_voucher','cash_drawer'"

type CommercialPrint struct {
	Kind        string
	Document    any
	Content     string
	PrinterID   string
	OperationID string
	ActorID     string
	DeviceID    string
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
	if !isCommercialKind(input.Kind) {
		return nil, "", invalid("Tipo de documento comercial no válido.")
	}
	printer, err := one[Printer](ctx, tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", input.PrinterID)
	if err != nil {
		return nil, "", err
	}
	ticketID := events.NewID()
	if _, err := tx.Exec(ctx, "INSERT INTO app.kitchen_tickets(id,kind,station_ids,document) VALUES ($1,$2,'{}',$3)",
		ticketID, input.Kind, encoded(input.Document)); err != nil {
		return nil, "", err
	}
	handler := service{ctx: ctx, tx: tx, operationID: input.OperationID, request: Request{Actor: Actor{UserID: input.ActorID, DeviceID: input.DeviceID}}}
	job, err := handler.enqueueContent(ticketID, *printer, nil, sanitizeContent(input.Content))
	if err != nil {
		return nil, "", err
	}
	return job, ticketID, handler.history("print."+input.Kind+"_requested", &job.ID, nil, job)
}

// ReprintCommercial solicita otra copia idéntica de un documento comercial. Como en las comandas,
// se rechaza mientras exista otra copia pendiente o sin resolver de la misma familia.
func ReprintCommercial(ctx context.Context, tx pgx.Tx, ticketID, operationID, actorID, deviceID string) (*PrintJob, error) {
	var kind string
	if err := tx.QueryRow(ctx, "SELECT kind FROM app.kitchen_tickets WHERE id=$1", ticketID).Scan(&kind); err != nil {
		return nil, err
	}
	if !isCommercialKind(kind) {
		return nil, invalid("El documento no es un ticket, una precuenta ni un documento de caja.")
	}
	original, err := one[PrintJob](ctx, tx, `SELECT to_jsonb(j) FROM app.kitchen_print_jobs j
        WHERE ticket_id=$1 AND original_job_id IS NULL ORDER BY created_at LIMIT 1`, ticketID)
	if err != nil {
		return nil, err
	}
	if original.State != "submitted" && original.State != "cancelled" {
		return nil, invalid("Resuelve primero el trabajo pendiente o incierto para no generar copias descontroladas.")
	}
	var unresolved bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.kitchen_print_jobs
        WHERE original_job_id=$1 AND state IN ('pending','processing','uncertain','error','intervention'))`, original.ID).Scan(&unresolved); err != nil {
		return nil, err
	}
	if unresolved {
		return nil, &Error{Status: 409, Code: "PRINT_COPY_PENDING", Message: "Ya existe una copia pendiente o sin resolver. Revisa ese trabajo antes de solicitar otra."}
	}
	handler := service{ctx: ctx, tx: tx, operationID: operationID, request: Request{Actor: Actor{UserID: actorID, DeviceID: deviceID}}}
	job, err := handler.enqueueContent(ticketID, original.PrinterSnapshot, &original.ID, original.Content)
	if err != nil {
		return nil, err
	}
	return job, handler.history("print.reprinted", &job.ID, nil, job)
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
// comprueba al configurar el cajón y otra vez antes de cada envío. El agente de impresión aún no
// existe. No se asume que todo dispositivo admite el pulso.
func CompatibleDrawerTransport(transport string) bool {
	return transport == "escpos_tcp" || transport == "cups"
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
	if !CompatibleDrawerTransport(printer.Transport) {
		return nil, "", &Error{Status: 409, Code: "DRAWER_PROTOCOL_UNSUPPORTED", Message: "La impresora del cajón no usa ESC/POS por red ni una cola CUPS; no se envió el comando de apertura."}
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
