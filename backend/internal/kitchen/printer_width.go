package kitchen

import (
	"time"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
)

// Prueba de ancho de una impresora (etapa 9C, §5.3): POST kitchen/printers/{id}/width-test, con
// kitchen.printers.manage (es parte de configurar la impresora, como elegir sus columnas). Imprime
// por la cola de siempre la regla numerada de documents.RenderWidthTest con las columnas elegidas en
// el formulario (aún sin guardar) o, si no se indican, con las de la impresora, en un ticket «test»
// (como la prueba técnica): nunca crea ventas, pagos, movimientos de caja ni de inventario. El
// trabajo lleva la fotografía de la impresora con las columnas probadas, para que CUPS de texto use
// la misma densidad que usará al guardarlas; una copia desde la cola repite el mismo texto.

// widthTestPurpose marca en el documento del ticket «test» que es una prueba de ancho (la prueba
// técnica guarda una comanda de prueba, que se vuelve a renderizar al copiarse).
const widthTestPurpose = "width_test"

// WidthTestResult es la respuesta de la prueba de ancho.
type WidthTestResult struct {
	Job        *PrintJob `json:"job"`
	TicketID   string    `json:"ticket_id"`
	PaperWidth int       `json:"paper_width"`
	Columns    int       `json:"columns"`
}

func (handler *service) widthTest(printerID string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Columns     *int   `json:"columns"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	printer, err := one[Printer](handler.ctx, handler.tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", printerID)
	if err != nil {
		return nil, err
	}
	columns := printer.TextColumns()
	if input.Columns != nil {
		if !documents.ValidColumns(printer.PaperWidth, *input.Columns) {
			return nil, invalid(columnsMessage(printer.PaperWidth, *input.Columns))
		}
		columns = *input.Columns
	}
	var timezone string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT timezone FROM app.establishment").Scan(&timezone); err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.UTC
	}
	layout := documents.RenderWidthTest(documents.WidthTest{
		PaperWidth: printer.PaperWidth, Columns: columns, PrinterName: cleanText(printer.Name),
		LocalDateTime: time.Now().In(location).Format("02/01/2006 15:04"),
	})
	target := *printer
	target.Columns = nil
	if columns != documents.ColumnsFor(printer.PaperWidth) {
		target.Columns = &columns
	}
	ticketID := events.NewID()
	ticket := map[string]any{
		"kind": "test", "purpose": widthTestPurpose, "printer_id": printer.ID, "printer_name": printer.Name,
		"paper_width": layout.PaperWidth, "columns": columns, "text_size": documents.TextSizeNormal, "content": layout.Text(),
	}
	if _, err := handler.tx.Exec(handler.ctx, "INSERT INTO app.kitchen_tickets(id,kind,station_ids,document) VALUES ($1,'test','{}',$2)",
		ticketID, encoded(ticket)); err != nil {
		return nil, err
	}
	job, err := handler.enqueueLayout(ticketID, target, nil, layout)
	if err != nil {
		return nil, err
	}
	result := WidthTestResult{Job: job, TicketID: ticketID, PaperWidth: layout.PaperWidth, Columns: columns}
	return result, handler.history("print.width_test_requested", &job.ID, nil, map[string]any{
		"job": job, "printer_id": printer.ID, "paper_width": layout.PaperWidth, "columns": columns,
	})
}

// widthTestTicket indica si un ticket es una prueba de ancho: se muestra y se copia con su texto.
func (handler *service) widthTestTicket(ticketID string) (bool, error) {
	var purpose string
	err := handler.tx.QueryRow(handler.ctx, "SELECT coalesce(document->>'purpose','') FROM app.kitchen_tickets WHERE id=$1",
		ticketID).Scan(&purpose)
	return purpose == widthTestPurpose, err
}
