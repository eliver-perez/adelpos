package kitchen

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/events"
)

// Documentos comerciales (precuentas y tickets de pago) creados por el módulo de pagos. Se
// imprimen por la misma cola durable de la etapa 5; su contenido llega ya renderizado y nunca
// se reinterpreta como comanda.

func isCommercialKind(kind string) bool {
	return kind == "receipt" || kind == "precheck"
}

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
		return nil, invalid("El documento no es un ticket ni una precuenta.")
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
