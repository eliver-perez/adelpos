package designer

import (
	"context"
	"encoding/json"
	"time"

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
	data, err := handler.sampleData(kind)
	if err != nil {
		return nil, err
	}
	// La muestra sale con el papel y las columnas de la impresora elegida (etapa 9C).
	options := printer.RenderOptions()
	options.IncludeExpected = handler.request.Actor.Can("cash.expected.read")
	layout, err := documents.Render(kind, template, data, options)
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
// Code identifica el motivo para las pantallas y las pruebas; Reason lo explica en español.
type Compatibility struct {
	Status string `json:"status"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// Motivos de la compatibilidad de una impresora.
const (
	compatibilityNetwork     = "escpos_network"
	compatibilityDisabled    = "escpos_disabled"
	compatibilityRawQueue    = "queue_raw"
	compatibilityNotRaw      = "queue_not_raw"
	compatibilityUnavailable = "queue_unavailable"
	compatibilityTimeout     = "queue_timeout"
	compatibilityAgent       = "agent"
)

// PrinterCompatibility es una impresora activa con lo que puede imprimir del diseñador. Columns son
// sus columnas de texto efectivas (desde la etapa 9C, las elegidas en Cocina → Impresoras) y
// DefaultColumns las predeterminadas de su papel.
type PrinterCompatibility struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Transport      string        `json:"transport"`
	PaperWidth     int           `json:"paper_width"`
	Columns        int           `json:"columns"`
	DefaultColumns int           `json:"default_columns"`
	Format         Compatibility `json:"format"`
	Logo           Compatibility `json:"logo"`
}

// printerProbeBudget limita el tiempo total de las consultas a CUPS de una petición
// GET documents/printers (cada consulta tiene además su propio límite de 3 s): corren dentro de la
// transacción autenticada, que retiene la fila de la terminal, y una cola que no responde no debe
// retenerla, como en el cajón (payments/cash_drawer.go). Esta ruta no toma locks consultivos. Es una
// variable para que las pruebas lo acorten.
var printerProbeBudget = 4 * time.Second

// printers lista las impresoras activas con su compatibilidad de formato (negrita y tamaños) y de
// logotipo. Una impresora CUPS solo recibe formato y logotipo si está marcada «Esta impresora
// entiende ESC/POS (formato y logotipo)» y su cola admite datos raw; solo entonces se consulta la
// cola (lpstat y lpoptions, solo lecturas), con un límite total de tiempo. El resultado en papel
// queda como prueba física pendiente.
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
	return printerCompatibilities(handler.ctx, inspector, printers, len(identity.Logo) == 0), nil
}

// printerCompatibilities evalúa las impresoras una tras otra con el límite total de tiempo: las
// colas que ya no alcanzan a consultarse quedan «Sin confirmar» con su motivo.
func printerCompatibilities(ctx context.Context, inspector kitchen.QueueInspector, printers []kitchen.Printer, logoMissing bool) []PrinterCompatibility {
	probe, cancel := context.WithTimeout(ctx, printerProbeBudget)
	defer cancel()
	result := []PrinterCompatibility{}
	for _, printer := range printers {
		item := PrinterCompatibility{
			ID: printer.ID, Name: printer.Name, Transport: printer.Transport, PaperWidth: printer.PaperWidth,
			Columns: printer.TextColumns(), DefaultColumns: documents.ColumnsFor(printer.PaperWidth),
		}
		item.Format = printerCompatibility(probe, inspector, printer)
		item.Logo = item.Format
		if logoMissing {
			item.Logo.Reason = "No hay logotipo cargado: el bloque «Logotipo» no imprime nada. " + item.Logo.Reason
		}
		result = append(result, item)
	}
	return result
}

// printerCompatibility decide si una impresora reproduce el formato y el logotipo.
func printerCompatibility(ctx context.Context, inspector kitchen.QueueInspector, printer kitchen.Printer) Compatibility {
	switch printer.Transport {
	case "escpos_tcp":
		return Compatibility{Status: "supported", Code: compatibilityNetwork,
			Reason: "La impresora recibe comandos ESC/POS por red; el resultado en papel es una prueba física pendiente."}
	case "cups":
		return cupsCompatibility(ctx, inspector, printer)
	}
	return Compatibility{Status: "unsupported", Code: compatibilityAgent, Reason: "El agente de impresión aún no existe."}
}

// cupsCompatibility aplica la regla de CUPS: primero la marca de la impresora (sin consultar la
// cola si está desactivada) y después la cola, mientras quede tiempo del límite total.
func cupsCompatibility(ctx context.Context, inspector kitchen.QueueInspector, printer kitchen.Printer) Compatibility {
	if !printer.AcceptsESCPos {
		return Compatibility{Status: "unsupported", Code: compatibilityDisabled,
			Reason: "Desactivado en la impresora: «Esta impresora entiende ESC/POS (formato y logotipo)» está apagado, " +
				"así que imprime el texto sin negrita, tamaños ni logotipo. Actívalo en Cocina → Impresoras solo si la impresora entiende ESC/POS."}
	}
	if ctx.Err() != nil {
		return Compatibility{Status: "unknown", Code: compatibilityTimeout,
			Reason: "CUPS no respondió a tiempo: no se consultó la cola " + printer.Destination + ". Vuelve a consultar en unos momentos."}
	}
	capability := inspector.InspectQueue(ctx, printer.Destination)
	switch {
	case !capability.Checked:
		return Compatibility{Status: "unknown", Code: compatibilityUnavailable, Reason: capability.Reason}
	case capability.Raw:
		return Compatibility{Status: "supported", Code: compatibilityRawQueue,
			Reason: "La impresora entiende ESC/POS y su cola admite datos raw; el resultado en papel es una prueba física pendiente."}
	}
	return Compatibility{Status: "unsupported", Code: compatibilityNotRaw,
		Reason: "Cola sin raw: imprime el texto sin negrita, tamaños ni logotipo. " + capability.Reason}
}
