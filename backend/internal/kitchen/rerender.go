package kitchen

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/documents"
)

// Impresión de un histórico con la plantilla vigente (etapa 9B). Es una acción distinta de la
// reimpresión exacta: vuelve a renderizar los datos históricos del documento con la plantilla
// activa de su tipo y la identidad vigente, en un ticket nuevo «document_rerender» que guarda el
// ticket original, la versión usada y el texto. Nunca crea ventas, pagos, documentos comerciales
// ni de caja, ni cambia el original.

// rerenderUnresolved son los estados de un trabajo que aún puede producir papel.
const rerenderUnresolved = "('pending','processing','uncertain','error','intervention')"

// RerenderTarget valida que un ticket admite imprimirse con la plantilla vigente y devuelve la
// fotografía de la impresora de su trabajo original. Como en las reimpresiones, se rechaza mientras
// el original o una impresión anterior con la plantilla vigente sigan sin resolver.
func RerenderTarget(ctx context.Context, tx pgx.Tx, sourceTicketID string) (*Printer, error) {
	original, err := one[PrintJob](ctx, tx, `SELECT to_jsonb(j)-'content'-'layout' FROM app.kitchen_print_jobs j
        WHERE ticket_id=$1 AND original_job_id IS NULL ORDER BY created_at LIMIT 1`, sourceTicketID)
	if err != nil {
		return nil, err
	}
	if original.State != "submitted" && original.State != "cancelled" {
		return nil, invalid("Resuelve primero el trabajo pendiente o incierto del documento original antes de imprimirlo con la plantilla vigente.")
	}
	var unresolved bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.kitchen_print_jobs j JOIN app.kitchen_tickets t ON t.id=j.ticket_id
        WHERE t.kind='document_rerender' AND t.document->>'source_ticket_id'=$1 AND j.state IN `+rerenderUnresolved+`)`,
		sourceTicketID).Scan(&unresolved); err != nil {
		return nil, err
	}
	if unresolved {
		return nil, &Error{Status: 409, Code: "PRINT_COPY_PENDING", Message: "Ya existe una impresión con la plantilla vigente pendiente o sin resolver. Revisa ese trabajo antes de solicitar otra."}
	}
	printer := original.PrinterSnapshot
	return &printer, nil
}

// rerenderSourceKind es el tipo del documento original de un histórico impreso con la plantilla
// vigente.
func (handler *service) rerenderSourceKind(ticketID string) (string, error) {
	var sourceKind string
	err := handler.tx.QueryRow(handler.ctx, "SELECT coalesce(document->>'source_kind','') FROM app.kitchen_tickets WHERE id=$1", ticketID).Scan(&sourceKind)
	return sourceKind, err
}

// rerender imprime una comanda o aviso de cocina histórico con la plantilla vigente
// (POST kitchen/print-jobs/rerender). Exige además del permiso de reimprimir el de imprimir con la
// plantilla vigente; los tickets, precuentas y documentos de caja tienen su propia acción en su
// módulo, con sus permisos.
func (handler *service) rerender() (any, error) {
	if err := handler.request.Actor.Require("kitchen.stations.all", documents.PermissionRenderCurrent); err != nil {
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
		return nil, invalid("Indica el motivo de la impresión con la plantilla vigente (5–500 caracteres).")
	}
	if input.PrinterID != nil {
		if err := handler.request.Actor.Require("kitchen.printers.manage"); err != nil {
			return nil, err
		}
		if err := validID(*input.PrinterID); err != nil {
			return nil, err
		}
	}
	original, err := one[PrintJob](handler.ctx, handler.tx, "SELECT to_jsonb(j)-'content'-'layout' FROM app.kitchen_print_jobs j WHERE id=$1", input.JobID)
	if err != nil {
		return nil, err
	}
	var kind string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT kind FROM app.kitchen_tickets WHERE id=$1", original.TicketID).Scan(&kind); err != nil {
		return nil, err
	}
	if kind != "normal" && kind != "modification" && kind != "cancellation" {
		return nil, invalid("Desde la cola solo las comandas y avisos de cocina se imprimen con la plantilla vigente; los tickets y documentos de caja tienen su propia acción.")
	}
	printer, err := RerenderTarget(handler.ctx, handler.tx, original.TicketID)
	if err != nil {
		return nil, err
	}
	if input.PrinterID != nil {
		current, err := one[Printer](handler.ctx, handler.tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", input.PrinterID)
		if err != nil {
			return nil, err
		}
		printer = current
	}
	document, err := one[Document](handler.ctx, handler.tx, "SELECT document FROM app.kitchen_tickets WHERE id=$1", original.TicketID)
	if err != nil {
		return nil, err
	}
	version, err := documents.ActiveVersion(handler.ctx, handler.tx, documentKind(kind))
	if err != nil {
		return nil, err
	}
	identity, err := documents.LoadIdentity(handler.ctx, handler.tx)
	if err != nil {
		return nil, err
	}
	data := DocumentData(*document)
	data.Identity = identity
	layout, err := documents.Render(version.Kind, version.Template, documents.Data{Kitchen: &data}, documents.RenderOptions{PaperWidth: printer.PaperWidth})
	if err != nil {
		return nil, err
	}
	ticket := map[string]any{
		"kind": documents.TicketKindRerender, "source_ticket_id": original.TicketID, "source_kind": kind,
		"source_job_id": original.ID, "document_kind": version.Kind, "template_version_id": version.ID,
		"text_size": layout.TextSize, "has_prices": hasPrices(document.Sections), "content": layout.Text(),
	}
	versionID := version.ID
	job, ticketID, err := EnqueueRendered(handler.ctx, handler.tx, RenderedPrint{
		Kind: documents.TicketKindRerender, Document: ticket, Layout: layout, TemplateVersionID: &versionID, Printer: printer, History: "print.rerendered",
		OperationID: handler.operationID, ActorID: handler.request.Actor.UserID, DeviceID: handler.request.Actor.DeviceID,
	})
	if err != nil {
		return nil, err
	}
	if handler.audit != nil {
		if err := handler.audit(handler.ctx, handler.tx, "print.rerendered", "kitchen_ticket", &original.TicketID, nil, map[string]any{
			"job_id": job.ID, "ticket_id": ticketID, "template_version_id": version.ID, "reason": input.Reason,
		}); err != nil {
			return nil, err
		}
	}
	return map[string]any{"job": job, "ticket_id": ticketID, "template_version_id": version.ID}, nil
}

// hasPrices indica si la comanda guardó importes (la cola exige consultar precios para verlos).
func hasPrices(sections []Section) bool {
	for _, section := range sections {
		for _, item := range section.Items {
			if item.Price != "" {
				return true
			}
		}
	}
	return false
}
