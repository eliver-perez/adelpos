package kitchen

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/documents"
)

// Configuración actual de la impresora al volver a enviar un trabajo (revisión 9F, M2). Un trabajo guarda la
// fotografía de su impresora al encolarse; si la impresión falló porque la impresora estaba mal configurada
// (destino, cola o transporte) y el propietario la corrige con «Configurar impresora», volver a enviarlo con
// la fotografía vieja fallaría igual. Por eso reintentar un trabajo que nunca salió (por la cola o desde el
// aviso del documento) toma la impresora con su configuración vigente y, si su papel o sus columnas cambiaron,
// prepara el documento de nuevo para ella con los mismos datos guardados y la misma versión de plantilla con
// que se emitió (como la reimpresión de una comanda): solo cambia el ancho, nunca el contenido. La reimpresión
// con motivo de una precuenta, un ticket o un documento de caja (ReprintCommercial) también toma la impresora
// vigente, pero conserva el papel y las columnas del original: es una copia idéntica (decisión de la 9C).

// StoredRenderer vuelve a preparar un documento comercial o de caja ya emitido (precuenta, ticket de pago,
// comprobante o corte) para el papel y las columnas de otra impresora, con sus datos guardados y la versión
// de plantilla con que se emitió. Lo aporta el módulo de pagos, que conoce esos datos (kitchen no importa
// payments): administración lo pasa en Request y pagos en sus propias llamadas.
type StoredRenderer func(ctx context.Context, tx pgx.Tx, ticketKind string, document []byte, templateVersionID *string,
	options documents.RenderOptions) (*documents.Layout, error)

// RenderUnavailable es el rechazo de un documento que no se puede preparar para el papel o las columnas
// actuales de su impresora (sin datos guardados, una muestra del diseñador o una impresión con la plantilla
// vigente): se cancela y se vuelve a pedir desde su pantalla.
func RenderUnavailable() error {
	return &Error{Status: 409, Code: "PRINT_RENDER_UNAVAILABLE", Message: "La impresora cambió de papel o de columnas y este " +
		"documento no se puede preparar de nuevo para ella: cancélalo y vuelve a imprimirlo desde su pantalla."}
}

// refreshedPrint es lo que se envía otra vez: la impresora con su configuración actual y, si se preparó de
// nuevo (Rendered), el contenido y la IR para su papel.
type refreshedPrint struct {
	Printer  Printer
	Content  string
	Layout   *documents.Layout
	Rendered bool
}

// refreshPrint prepara un trabajo para enviarse otra vez a printer, la impresora con su configuración actual.
// Con el mismo papel y las mismas columnas que la fotografía, el contenido no cambia; si cambiaron, se prepara
// de nuevo con los datos guardados de su ticket. Una prueba de ancho conserva las columnas que probaba.
func refreshPrint(ctx context.Context, tx pgx.Tx, job *PrintJob, ticketKind string, printer Printer,
	renderer StoredRenderer) (*refreshedPrint, error) {
	var stored []byte
	var purpose string
	var templateVersionID *string
	if err := tx.QueryRow(ctx, `SELECT document,coalesce(document->>'purpose',''),template_version_id::text
        FROM app.kitchen_tickets WHERE id=$1`, job.TicketID).Scan(&stored, &purpose, &templateVersionID); err != nil {
		return nil, err
	}
	if ticketKind == "test" && purpose == widthTestPurpose {
		// La prueba de ancho imprime la regla de las columnas que se probaban, quizá aún sin guardar: se
		// conservan; si cambió el papel, la regla ya no sirve.
		if printer.PaperWidth != job.PrinterSnapshot.PaperWidth {
			return nil, &Error{Status: 409, Code: "PRINT_RENDER_UNAVAILABLE",
				Message: "La impresora cambió de papel: cancela esta prueba de ancho e imprime otra."}
		}
		printer.Columns = job.PrinterSnapshot.Columns
	}
	result := &refreshedPrint{Printer: printer}
	if printer.RenderOptions() == job.PrinterSnapshot.RenderOptions() {
		return result, nil
	}
	layout, err := renderStoredTicket(ctx, tx, ticketKind, purpose, stored, templateVersionID, printer.RenderOptions(), renderer)
	if err != nil {
		return nil, err
	}
	result.Content = layout.Text()
	if !layout.Plain() {
		result.Layout = layout.ForQueue()
	}
	result.Rendered = true
	return result, nil
}

// renderStoredTicket prepara el documento guardado de un ticket para otro ancho: las comandas, los avisos y la
// prueba técnica con la versión de plantilla que guardaron; las precuentas, los tickets y los documentos de
// caja con el renderizador de pagos. Los demás no se pueden preparar de nuevo.
func renderStoredTicket(ctx context.Context, tx pgx.Tx, ticketKind, purpose string, stored []byte, templateVersionID *string,
	options documents.RenderOptions, renderer StoredRenderer) (*documents.Layout, error) {
	switch {
	case ticketKind == "normal" || ticketKind == "modification" || ticketKind == "cancellation" ||
		(ticketKind == "test" && purpose != widthTestPurpose):
		document, err := decodeDocument(stored)
		if err != nil {
			return nil, err
		}
		handler := service{ctx: ctx, tx: tx}
		return handler.renderStored(*document, options)
	case isCommercialKind(ticketKind) && !isDesignerKind(ticketKind) && renderer != nil:
		layout, err := renderer(ctx, tx, ticketKind, stored, templateVersionID, options)
		if err != nil {
			return nil, err
		}
		if layout == nil {
			return nil, RenderUnavailable()
		}
		return layout, nil
	}
	return nil, RenderUnavailable()
}

// decodeDocument lee el documento guardado de una comanda, un aviso o una prueba técnica.
func decodeDocument(stored []byte) (*Document, error) {
	var document Document
	if err := json.Unmarshal(stored, &document); err != nil {
		return nil, err
	}
	return &document, nil
}

// currentPrinter lee la impresora con su configuración vigente (las impresoras no se eliminan: los trabajos
// las referencian).
func currentPrinter(ctx context.Context, tx pgx.Tx, id string) (*Printer, error) {
	return one[Printer](ctx, tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", id)
}

// storeRefreshedContent guarda en el trabajo el contenido y la IR preparados para la impresora actual.
func storeRefreshedContent(ctx context.Context, tx pgx.Tx, jobID string, refreshed *refreshedPrint) error {
	if !refreshed.Rendered {
		return nil
	}
	var layout []byte
	if refreshed.Layout != nil {
		layout = encoded(refreshed.Layout)
	}
	_, err := tx.Exec(ctx, "UPDATE app.kitchen_print_jobs SET content=$1,layout=$2 WHERE id=$3", refreshed.Content, layout, jobID)
	return err
}
