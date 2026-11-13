package kitchen

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
)

func (handler *service) jobAction(id, action string) (any, error) {
	if err := handler.request.Actor.Require("kitchen.stations.all"); err != nil {
		return nil, err
	}
	var input struct {
		OperationID string  `json:"operation_id"`
		Revision    int     `json:"revision"`
		Decision    string  `json:"decision"`
		Reason      string  `json:"reason"`
		PrinterID   *string `json:"printer_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	job, err := one[PrintJob](handler.ctx, handler.tx, "SELECT to_jsonb(j) FROM app.kitchen_print_jobs j WHERE id=$1 FOR UPDATE", id)
	if err != nil {
		return nil, err
	}
	if job.Revision != input.Revision {
		return nil, conflict()
	}
	if job.State == "processing" {
		return nil, invalid("El transporte está en proceso. Espera su resultado o la reconciliación de su lease.")
	}
	// Etapa 9F (decisión D3): reintentar un trabajo que nunca salió en papel (error confirmado o en
	// intervención) es un toque, sin motivo; si se escribe uno, se valida y se guarda. Resolver un
	// resultado incierto (que pudo haber salido) sigue exigiendo motivo.
	if action != "retry" || input.Reason != "" {
		if err := validReason(input.Reason); err != nil {
			return nil, err
		}
	}
	var ticketKind string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT kind FROM app.kitchen_tickets WHERE id=$1", job.TicketID).Scan(&ticketKind); err != nil {
		return nil, err
	}
	// Un comando del cajón solo se cancela o se reconoce desde la cola: reenviarlo abriría el
	// cajón sin el permiso ni el motivo que exige la apertura manual en Caja.
	if ticketKind == KindCashDrawer && (action == "retry" || input.Decision == "send") {
		return nil, invalid("Un comando de apertura del cajón no se reenvía desde la cola. Si hace falta abrirlo, usa «Abrir cajón» en Caja, con su permiso y motivo.")
	}
	if action == "retry" {
		if job.State != "error" && job.State != "intervention" {
			return nil, invalid("Un resultado incierto requiere resolución explícita; no se reintenta como error confirmado.")
		}
		input.Decision = "send"
	}
	state := ""
	switch input.Decision {
	case "accepted":
		if job.State != "uncertain" && job.State != "intervention" {
			return nil, invalid("Solo se reconcilian resultados inciertos o en intervención.")
		}
		state = "submitted"
	case "cancel":
		state = "cancelled"
	case "send":
		if job.State == "submitted" || job.State == "cancelled" {
			return nil, invalid("Usa reimpresión para solicitar otra copia de un trabajo finalizado.")
		}
		state = "pending"
	default:
		return nil, invalid("Selecciona cancelar, reconocer aceptación conocida o autorizar envío.")
	}
	printer := job.PrinterSnapshot
	if input.PrinterID != nil {
		if err := handler.request.Actor.Require("kitchen.printers.manage"); err != nil {
			return nil, err
		}
		found, err := one[Printer](handler.ctx, handler.tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", input.PrinterID)
		if err != nil {
			return nil, err
		}
		if !found.Active {
			return nil, invalid("El destino debe estar activo.")
		}
		printer = *found
	}
	// Autorizar envío tras incertidumbre reconoce el riesgo de una segunda copia.
	// No se toca la liberación, el estado comercial ni el punto de consumo futuro.
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_print_jobs SET state=$1,printer_id=$2,printer_snapshot=$3,
        next_attempt_at=now(),last_error=$4,result=$5,revision=revision+1,updated_at=now() WHERE id=$6`,
		state, printer.ID, encoded(printer), input.Reason, encoded(map[string]string{"decision": input.Decision, "actor": handler.request.Actor.UserID}), id)
	if err != nil {
		return nil, err
	}
	saved, err := one[PrintJob](handler.ctx, handler.tx, "SELECT to_jsonb(j)-'content'-'layout' FROM app.kitchen_print_jobs j WHERE id=$1", id)
	if err != nil {
		return nil, err
	}
	job.Content = ""
	job.Layout = nil
	return saved, handler.history("print."+action, &id, job, saved)
}

// validReason exige un motivo de entre 5 y 500 caracteres.
func validReason(reason string) error {
	if len(strings.TrimSpace(reason)) < 5 || len(reason) > 500 {
		return invalid("Indica un motivo de entre 5 y 500 caracteres.")
	}
	return nil
}

// DocumentRetry es el reintento de un documento comercial o de caja desde su propio aviso (etapa 9F,
// §4.2): lo pide el módulo de pagos, que ya validó el permiso de imprimir ese documento y que el trabajo
// le pertenece. Reason es opcional.
type DocumentRetry struct {
	JobID       string
	TicketID    string
	Reason      string
	OperationID string
	ActorID     string
	DeviceID    string
}

// RetryDocument vuelve a poner en la cola un trabajo de una precuenta, un ticket o un documento de caja que
// nunca salió en papel (error confirmado o en intervención), con la misma impresora y sin motivo, como
// «Reintentar» de la cola. Debe ser el trabajo más reciente de su documento; una impresora que ya no puede
// imprimir (desactivada o de otra terminal) responde PRINTER_UNAVAILABLE para que la persona la configure.
// Un resultado incierto nunca se reintenta desde aquí: se resuelve en la cola.
func RetryDocument(ctx context.Context, tx pgx.Tx, input DocumentRetry) (*PrintJob, error) {
	if err := validID(input.JobID); err != nil {
		return nil, err
	}
	if input.Reason != "" {
		if err := validReason(input.Reason); err != nil {
			return nil, err
		}
	}
	job, err := one[PrintJob](ctx, tx, "SELECT to_jsonb(j)-'content'-'layout' FROM app.kitchen_print_jobs j WHERE id=$1 FOR UPDATE", input.JobID)
	if err != nil {
		return nil, err
	}
	var kind string
	var latest bool
	if err := tx.QueryRow(ctx, `SELECT t.kind, NOT EXISTS(SELECT FROM app.kitchen_print_jobs later WHERE later.ticket_id=t.id
        AND (later.created_at,later.id)>($2::timestamptz,$1::uuid)) FROM app.kitchen_tickets t WHERE t.id=$3`,
		job.ID, job.CreatedAt, job.TicketID).Scan(&kind, &latest); err != nil {
		return nil, err
	}
	if job.TicketID != input.TicketID || !isCommercialKind(kind) || isDesignerKind(kind) {
		return nil, invalid("El trabajo no pertenece a este documento.")
	}
	if !latest {
		return nil, &Error{Status: 409, Code: "PRINT_JOB_SUPERSEDED", Message: "El documento ya tiene una impresión más reciente; revisa su estado actual."}
	}
	if job.State != "error" && job.State != "intervention" {
		if job.State == "uncertain" {
			return nil, &Error{Status: 409, Code: "PRINT_UNCERTAIN", Message: "No se sabe si el documento salió en papel: revisa la impresora y resuélvelo en Cocina › Cola de impresión."}
		}
		return nil, &Error{Status: 409, Code: "PRINT_NOT_FAILED", Message: "El documento no tiene una impresión fallida que reintentar."}
	}
	current, err := one[Printer](ctx, tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", job.PrinterID)
	if err != nil {
		return nil, err
	}
	if problem := PrinterProblem(current); problem != "" {
		return nil, UnavailablePrinterError(problem, current.Name)
	}
	handler := service{ctx: ctx, tx: tx, operationID: input.OperationID, request: Request{Actor: Actor{UserID: input.ActorID, DeviceID: input.DeviceID}}}
	_, err = tx.Exec(ctx, `UPDATE app.kitchen_print_jobs SET state='pending',next_attempt_at=now(),last_error=$1,result=$2,
        revision=revision+1,updated_at=now() WHERE id=$3`, input.Reason,
		encoded(map[string]string{"decision": "send", "actor": input.ActorID, "origin": "document"}), job.ID)
	if err != nil {
		return nil, err
	}
	saved, err := one[PrintJob](ctx, tx, "SELECT to_jsonb(j)-'content'-'layout' FROM app.kitchen_print_jobs j WHERE id=$1", job.ID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(ctx, tx, "printing.queued", "printing", &job.ID, nil); err != nil {
		return nil, err
	}
	return saved, handler.history("print.retry", &job.ID, job, saved)
}

func (handler *service) reprint() (any, error) {
	if err := handler.request.Actor.Require("kitchen.stations.all"); err != nil {
		return nil, err
	}
	var input struct {
		OperationID string  `json:"operation_id"`
		JobID       string  `json:"job_id"`
		PrinterID   *string `json:"printer_id"`
		Reason      string  `json:"reason"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validID(input.JobID); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(input.Reason)) < 5 || len(input.Reason) > 500 {
		return nil, invalid("Indica el motivo de la copia adicional (5–500 caracteres).")
	}
	original, err := one[PrintJob](handler.ctx, handler.tx, "SELECT to_jsonb(j) FROM app.kitchen_print_jobs j WHERE id=$1", input.JobID)
	if err != nil {
		return nil, err
	}
	var kind string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT kind FROM app.kitchen_tickets WHERE id=$1", original.TicketID).Scan(&kind); err != nil {
		return nil, err
	}
	// Las copias de documentos de caja exigen los permisos de caja (reimprimir y, si incluyen el
	// efectivo esperado, consultarlo); el cajón nunca se vuelve a abrir con una copia.
	if isCashKind(kind) {
		return nil, invalid("Los documentos y comandos de caja se reimprimen o repiten desde Caja, con sus permisos.")
	}
	// Un documento de caja impreso con la plantilla vigente sigue las reglas de caja: no se copia
	// desde la cola (podría contener el efectivo esperado).
	if kind == documents.TicketKindRerender {
		sourceKind, err := handler.rerenderSourceKind(original.TicketID)
		if err != nil {
			return nil, err
		}
		if isCashKind(sourceKind) {
			return nil, invalid("Los documentos de caja se reimprimen desde Caja, con sus permisos.")
		}
	}
	if original.State != "submitted" && original.State != "cancelled" {
		return nil, invalid("Resuelve primero el trabajo pendiente o incierto para no generar copias descontroladas.")
	}
	rootID := original.ID
	if original.OriginalJobID != nil {
		rootID = *original.OriginalJobID
	}
	// Una sola copia sin resolver por documento: ni otra copia exacta ni una impresión con la
	// plantilla vigente (ni sus copias) pueden seguir pendientes.
	if err := requireNoPendingCopy(handler.ctx, handler.tx, original.TicketID); err != nil {
		return nil, err
	}
	document, err := one[Document](handler.ctx, handler.tx, "SELECT document FROM app.kitchen_tickets WHERE id=$1", original.TicketID)
	if err != nil {
		return nil, err
	}
	printer := original.PrinterSnapshot
	if input.PrinterID != nil {
		if err := handler.request.Actor.Require("kitchen.printers.manage"); err != nil {
			return nil, err
		}
		current, err := one[Printer](handler.ctx, handler.tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", input.PrinterID)
		if err != nil {
			return nil, err
		}
		printer = *current
	}
	// Una copia usa la presentación original: no añade una leyenda REIMPRESIÓN. Un documento
	// comercial (y una prueba de ancho, etapa 9C) conserva su contenido ya renderizado en lugar de
	// interpretarse como comanda.
	widthTest := false
	if kind == "test" {
		widthTest, err = handler.widthTestTicket(original.TicketID)
		if err != nil {
			return nil, err
		}
	}
	var job *PrintJob
	if isCommercialKind(kind) || widthTest {
		job, err = handler.enqueueJob(original.TicketID, printer, &rootID, original.Content, original.Layout)
	} else {
		job, err = handler.enqueue(*document, printer, &rootID)
	}
	if err != nil {
		return nil, err
	}
	original.Content = ""
	original.Layout = nil
	return job, handler.history("print.reprinted", &job.ID, original, map[string]any{"job": job, "reason": input.Reason})
}

func (handler *service) testPrint() (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		PrinterID   string `json:"printer_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	printer, err := one[Printer](handler.ctx, handler.tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", input.PrinterID)
	if err != nil {
		return nil, err
	}
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	var name, timezone string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT name,timezone FROM app.establishment").Scan(&name, &timezone); err != nil {
		return nil, err
	}
	settings.ShowPrices = false
	document := Document{ID: events.NewID(), Kind: "test", CreatedAt: time.Now().UTC(), Settings: *settings,
		Context: TicketContext{Establishment: name, Timezone: timezone, AccountName: "Prueba técnica autorizada", Employee: handler.request.Actor.UserID},
		Sections: []Section{{
			Station: Station{Name: "Impresora: " + printer.Name},
			Items: []Instructions{{
				Name:     "Acentos: á é í ó ú ñ Ñ · café",
				Quantity: 1,
				Parts:    []Part{},
				Notes:    "Comprobar ancho, legibilidad y corte configurado.",
			}},
		}},
	}
	_, err = handler.tx.Exec(handler.ctx, "INSERT INTO app.kitchen_tickets(id,kind,station_ids,document) VALUES ($1,'test','{}',$2)", document.ID, encoded(document))
	if err != nil {
		return nil, err
	}
	job, err := handler.enqueue(document, *printer, nil)
	if err != nil {
		return nil, err
	}
	return job, handler.history("print.test_requested", &job.ID, nil, job)
}
