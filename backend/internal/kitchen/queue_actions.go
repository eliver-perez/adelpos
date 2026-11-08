package kitchen

import (
	"strings"
	"time"

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
	if len(strings.TrimSpace(input.Reason)) < 5 || len(input.Reason) > 500 {
		return nil, invalid("Indica un motivo de entre 5 y 500 caracteres.")
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
	saved, err := one[PrintJob](handler.ctx, handler.tx, "SELECT to_jsonb(j)-'content' FROM app.kitchen_print_jobs j WHERE id=$1", id)
	if err != nil {
		return nil, err
	}
	job.Content = ""
	return saved, handler.history("print."+action, &id, job, saved)
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
	if original.State != "submitted" && original.State != "cancelled" {
		return nil, invalid("Resuelve primero el trabajo pendiente o incierto para no generar copias descontroladas.")
	}
	rootID := original.ID
	if original.OriginalJobID != nil {
		rootID = *original.OriginalJobID
	}
	var unresolved bool
	if err := handler.tx.QueryRow(handler.ctx, `SELECT EXISTS(SELECT FROM app.kitchen_print_jobs
		WHERE original_job_id=$1 AND state IN ('pending','processing','uncertain','error','intervention'))`, rootID).Scan(&unresolved); err != nil {
		return nil, err
	}
	if unresolved {
		return nil, &Error{Status: 409, Code: "PRINT_COPY_PENDING", Message: "Ya existe una copia pendiente o sin resolver. Revisa ese trabajo antes de solicitar otra."}
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
	// Una copia usa la presentación original: no añade una leyenda REIMPRESIÓN.
	job, err := handler.enqueue(*document, printer, &rootID)
	if err != nil {
		return nil, err
	}
	original.Content = ""
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
