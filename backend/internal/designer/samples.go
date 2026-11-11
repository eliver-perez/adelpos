package designer

import (
	"encoding/json"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/kitchen"
)

// Muestras impresas y compatibilidad de impresoras. Una muestra es un documento con datos de
// muestra, identificado al inicio y al final como MUESTRA, que viaja por la cola de siempre
// (ticket document_sample): nunca crea ventas, pagos, movimientos ni documentos comerciales o de
// caja. Su resultado se consulta en la cola como el de cualquier trabajo (incierto incluido).

// SampleResult es la respuesta de POST documents/samples.
type SampleResult struct {
	Status    string `json:"status"`
	JobID     string `json:"job_id"`
	TicketID  string `json:"ticket_id"`
	PrinterID string `json:"printer_id"`
	Message   string `json:"message"`
}

func (handler *service) printSample(_ []string) (any, error) {
	var input struct {
		OperationID string          `json:"operation_id"`
		Kind        string          `json:"kind"`
		Layout      json.RawMessage `json:"layout"`
		PrinterID   string          `json:"printer_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	kind, found := documents.ParseKind(input.Kind)
	if !found {
		return nil, invalid("El tipo de documento no existe.")
	}
	if err := validID(input.PrinterID); err != nil {
		return nil, err
	}
	template, err := documents.ParseTemplate(kind, input.Layout)
	if err != nil {
		return nil, templateInvalid(err)
	}
	printer, err := kitchen.ReadPrinter(handler.ctx, handler.tx, input.PrinterID)
	if err != nil {
		return nil, err
	}
	identity, err := documents.LoadIdentity(handler.ctx, handler.tx)
	if err != nil {
		return nil, err
	}
	layout, err := documents.Render(kind, template, documents.SampleData(kind, identity), documents.RenderOptions{
		PaperWidth: printer.PaperWidth, IncludeExpected: handler.request.Actor.Can("cash.expected.read"),
	})
	if err != nil {
		return nil, err
	}
	documents.AddSampleMarkers(layout)
	ticket := map[string]any{
		"kind": documents.TicketKindSample, "document_kind": kind, "text_size": layout.TextSize, "content": layout.Text(),
	}
	job, ticketID, err := kitchen.EnqueueRendered(handler.ctx, handler.tx, kitchen.RenderedPrint{
		Kind: documents.TicketKindSample, Document: ticket, Layout: layout, PrinterID: printer.ID, History: "print.sample_requested",
		OperationID: handler.operationID, ActorID: handler.request.Actor.UserID, DeviceID: handler.request.Actor.DeviceID,
	})
	if err != nil {
		return nil, err
	}
	result := SampleResult{Status: job.State, JobID: job.ID, TicketID: ticketID, PrinterID: printer.ID, Message: job.LastError}
	return result, handler.audit(handler.ctx, handler.tx, "documents.sample_printed", "kitchen_printer", &printer.ID, nil, map[string]any{
		"kind": kind, "job_id": job.ID, "ticket_id": ticketID, "block_count": len(template.Blocks),
	})
}

// Compatibility es el estado de una capacidad de la impresora: supported, unsupported o unknown.
type Compatibility struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// PrinterCompatibility es una impresora activa con lo que puede imprimir del diseñador.
type PrinterCompatibility struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Transport  string        `json:"transport"`
	PaperWidth int           `json:"paper_width"`
	Columns    int           `json:"columns"`
	Format     Compatibility `json:"format"`
	Logo       Compatibility `json:"logo"`
}

// printers lista las impresoras activas con su compatibilidad de formato (negrita y tamaños) y de
// logotipo. Para CUPS se consulta la cola solo con lecturas (lpstat y lpoptions): una cola que no
// admite datos raw imprime el texto sin formato ni logotipo. El resultado en papel queda como
// prueba física pendiente.
func (handler *service) printers(_ []string) (any, error) {
	printers, err := kitchen.ActivePrinters(handler.ctx, handler.tx)
	if err != nil {
		return nil, err
	}
	inspector := handler.request.Queues
	if inspector == nil {
		inspector = kitchen.HostQueueInspector{}
	}
	// Sin logotipo cargado, el bloque «Logotipo» no imprime nada en ninguna impresora: el motivo lo
	// dice (el estado sigue siendo el de la impresora).
	identity, err := documents.LoadIdentity(handler.ctx, handler.tx)
	if err != nil {
		return nil, err
	}
	logoMissing := len(identity.Logo) == 0
	result := []PrinterCompatibility{}
	for _, printer := range printers {
		item := PrinterCompatibility{
			ID: printer.ID, Name: printer.Name, Transport: printer.Transport, PaperWidth: printer.PaperWidth,
			Columns: documents.ColumnsFor(printer.PaperWidth),
		}
		switch printer.Transport {
		case "escpos_tcp":
			supported := Compatibility{Status: "supported", Reason: "La impresora recibe comandos ESC/POS por red; el resultado en papel es una prueba física pendiente."}
			item.Format, item.Logo = supported, supported
		case "cups":
			capability := inspector.InspectQueue(handler.ctx, printer.Destination)
			switch {
			case !capability.Checked:
				unknown := Compatibility{Status: "unknown", Reason: capability.Reason}
				item.Format, item.Logo = unknown, unknown
			case capability.Raw:
				supported := Compatibility{Status: "supported", Reason: "La cola admite datos raw: recibe los comandos ESC/POS; el resultado en papel es una prueba física pendiente."}
				item.Format, item.Logo = supported, supported
			default:
				unsupported := Compatibility{Status: "unsupported", Reason: "La cola no admite datos raw: imprime el texto sin negrita, tamaños ni logotipo. " + capability.Reason}
				item.Format, item.Logo = unsupported, unsupported
			}
		default:
			unsupported := Compatibility{Status: "unsupported", Reason: "El agente de impresión aún no existe."}
			item.Format, item.Logo = unsupported, unsupported
		}
		if logoMissing {
			item.Logo.Reason = "No hay logotipo cargado: el bloque «Logotipo» no imprime nada. " + item.Logo.Reason
		}
		result = append(result, item)
	}
	return result, nil
}
