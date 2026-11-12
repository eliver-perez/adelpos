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

// unresolvedStates son los estados de un trabajo que aún puede producir papel.
const unresolvedStates = "('pending','processing','uncertain','error','intervention')"

// copyPending es el rechazo común de la reimpresión exacta y de la impresión con la plantilla
// vigente mientras el documento tenga otra copia sin resolver de cualquiera de las dos familias.
func copyPending() error {
	return &Error{Status: 409, Code: "PRINT_COPY_PENDING", Message: "Ya existe una copia de este documento pendiente o sin resolver " +
		"(reimpresión o impresión con la plantilla vigente). Revisa ese trabajo antes de solicitar otra."}
}

// sourceTicketOf devuelve el documento de origen de un ticket: el propio ticket o, si es una
// impresión con la plantilla vigente, el original que guarda su documento.
func sourceTicketOf(ctx context.Context, tx pgx.Tx, ticketID string) (string, error) {
	var kind, source string
	err := tx.QueryRow(ctx, "SELECT kind,coalesce(document->>'source_ticket_id','') FROM app.kitchen_tickets WHERE id=$1",
		ticketID).Scan(&kind, &source)
	if err != nil {
		return "", err
	}
	if kind == documents.TicketKindRerender && source != "" {
		return source, nil
	}
	return ticketID, nil
}

// documentCopiesUnresolved indica si un documento de origen tiene algún trabajo que aún puede
// producir papel en cualquiera de sus dos familias: los de su propio ticket (el original y sus
// copias exactas) y los de cada impresión con la plantilla vigente (su trabajo y las copias de ese
// trabajo desde la cola). Una sola consulta para que la reimpresión exacta y la impresión con la
// plantilla vigente se bloqueen entre sí: nunca hay dos hojas sin resolver del mismo documento.
func documentCopiesUnresolved(ctx context.Context, tx pgx.Tx, sourceTicketID string) (bool, error) {
	var unresolved bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.kitchen_print_jobs j JOIN app.kitchen_tickets t ON t.id=j.ticket_id
        WHERE (t.id=$1 OR (t.kind='document_rerender' AND t.document->>'source_ticket_id'=$2))
        AND j.state IN `+unresolvedStates+`)`, sourceTicketID, sourceTicketID).Scan(&unresolved)
	return unresolved, err
}

// requireNoPendingCopy rechaza una copia nueva (exacta o con la plantilla vigente) de un documento
// mientras exista otra sin resolver de cualquiera de sus dos familias.
func requireNoPendingCopy(ctx context.Context, tx pgx.Tx, ticketID string) error {
	source, err := sourceTicketOf(ctx, tx, ticketID)
	if err != nil {
		return err
	}
	unresolved, err := documentCopiesUnresolved(ctx, tx, source)
	if err != nil {
		return err
	}
	if unresolved {
		return copyPending()
	}
	return nil
}

// RerenderTarget valida que un ticket admite imprimirse con la plantilla vigente y devuelve la
// impresora de su trabajo original con su configuración actual (etapa 9C): la impresión con la
// plantilla vigente es un documento nuevo y usa el papel, las columnas y el destino vigentes de esa
// impresora, a diferencia de la reimpresión exacta, que conserva la fotografía. Como en las
// reimpresiones, se rechaza mientras el original siga sin resolver (400) o exista otra copia sin
// resolver del documento, exacta o con la plantilla vigente (409 PRINT_COPY_PENDING).
func RerenderTarget(ctx context.Context, tx pgx.Tx, sourceTicketID string) (*Printer, error) {
	original, err := one[PrintJob](ctx, tx, `SELECT to_jsonb(j)-'content'-'layout' FROM app.kitchen_print_jobs j
        WHERE ticket_id=$1 AND original_job_id IS NULL ORDER BY created_at LIMIT 1`, sourceTicketID)
	if err != nil {
		return nil, err
	}
	if original.State != "submitted" && original.State != "cancelled" {
		return nil, invalid("Resuelve primero el trabajo pendiente o incierto del documento original antes de imprimirlo con la plantilla vigente.")
	}
	if err := requireNoPendingCopy(ctx, tx, sourceTicketID); err != nil {
		return nil, err
	}
	// Las impresoras no se eliminan (los trabajos las referencian): la actual siempre existe.
	return one[Printer](ctx, tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", original.PrinterID)
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
	layout, err := documents.Render(version.Kind, version.Template, documents.Data{Kitchen: &data}, printer.RenderOptions())
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
