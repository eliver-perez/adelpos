package kitchen

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
)

func (handler *service) ticketsForRelease(release *Release, plan []preparedUnit) error {
	kind := "normal"
	reason := ""
	var sourceKind string
	err := handler.tx.QueryRow(handler.ctx, `SELECT kind,coalesce(payload->'after'->>'reason','')
		FROM app.restaurant_events WHERE id=$1`, release.SourceEventID).Scan(&sourceKind, &reason)
	if err != nil {
		return err
	}
	if sourceKind == "item.modified" {
		kind = "modification"
	}
	accounts := map[string]map[string]Section{}
	for _, entry := range plan {
		sections := accounts[entry.Unit.AccountID]
		if sections == nil {
			sections = map[string]Section{}
			accounts[entry.Unit.AccountID] = sections
		}
		for id, parts := range entry.Routing.Parts {
			section := sections[id]
			section.Station = entry.Routing.Stations[id]
			section.Items = append(section.Items, Instructions{ItemID: entry.Unit.ItemID, VersionID: entry.Unit.VersionID,
				UnitIDs:  []string{entry.Unit.ID},
				Name:     entry.Version.Quote.Name,
				Quantity: 1,
				Parts:    parts,
				Price:    entry.Unit.Total,
				Notes:    entry.Version.Request.Customization.Notes,
				Reason:   reason,
			})
			sections[id] = section
		}
	}
	for accountID, sections := range accounts {
		context, err := readContext(handler.ctx, handler.tx, accountID, release.OrderID)
		if err != nil {
			return err
		}
		if err := handler.persistDocuments(*context, sections, kind, &release.SourceEventID, &release.ID); err != nil {
			return err
		}
	}
	return nil
}

func groupItems(items []Instructions) []Instructions {
	result := []Instructions{}
	positions := map[string]int{}
	for _, item := range items {
		key := item.ItemID + "/" + item.VersionID + "/" + string(encoded(item.Parts)) + "/" + item.Reason
		position, exists := positions[key]
		if !exists {
			positions[key] = len(result)
			result = append(result, item)
			continue
		}
		grouped := &result[position]
		grouped.Quantity += item.Quantity
		grouped.UnitIDs = append(grouped.UnitIDs, item.UnitIDs...)
		if grouped.Price != "" && item.Price != "" {
			grouped.Price = decimal.RequireFromString(grouped.Price).Add(decimal.RequireFromString(item.Price)).StringFixed(2)
		}
		if grouped.Before != nil && item.Before != nil {
			previous := *grouped.Before
			previous.Quantity += item.Before.Quantity
			previous.UnitIDs = append(append([]string{}, previous.UnitIDs...), item.Before.UnitIDs...)
			grouped.Before = &previous
		}
	}
	return result
}

func (handler *service) persistDocuments(context TicketContext, sections map[string]Section, kind string, eventID, releaseID *string) error {
	settings, err := handler.settings()
	if err != nil {
		return err
	}
	// Etapa 9B: cada comanda guarda la versión de la plantilla activa de su tipo y una copia de la
	// identidad del establecimiento; su reimpresión usa esas mismas copias. El enrutamiento, la
	// agrupación y la cola no dependen de la plantilla.
	version, err := documents.ActiveVersion(handler.ctx, handler.tx, documentKind(kind))
	if err != nil {
		return err
	}
	identity, err := documents.LoadIdentity(handler.ctx, handler.tx)
	if err != nil {
		return err
	}
	textSize := documents.EffectiveTextSize(version.Template, settings.TextSize)
	groups := map[string][]Section{}
	stations := make([]Station, 0, len(sections))
	for _, section := range sections {
		stations = append(stations, section.Station)
	}
	sort.Slice(stations, func(first, second int) bool {
		if stations[first].SortOrder != stations[second].SortOrder {
			return stations[first].SortOrder < stations[second].SortOrder
		}
		return stations[first].Name < stations[second].Name
	})
	for _, station := range stations {
		section := sections[station.ID]
		section.Items = groupItems(section.Items)
		if !settings.ShowPrices {
			for index := range section.Items {
				clearInstructionPrices(&section.Items[index])
			}
		}
		grouping := station.Grouping
		if grouping == "inherit" {
			grouping = settings.Grouping
		}
		key := station.ID
		if grouping == "unified" && station.PrinterID != nil && station.Reception != "kds" {
			key = "printer/" + *station.PrinterID
		}
		groups[key] = append(groups[key], section)
	}
	for _, group := range groups {
		versionID := version.ID
		copied := identity
		document := Document{ID: events.NewID(), Kind: kind, CreatedAt: time.Now().UTC(), Context: context, Settings: *settings, Sections: group,
			TemplateVersionID: &versionID, Identity: &copied, TextSize: textSize}
		ids := []string{}
		for _, section := range group {
			ids = append(ids, section.Station.ID)
		}
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_tickets(id,source_event_id,release_id,kind,station_ids,document,
            template_version_id) VALUES ($1,$2,$3,$4,$5,$6,$7)`, document.ID, eventID, releaseID, kind, ids, encoded(document), versionID)
		if err != nil {
			return err
		}
		destination := group[0].Station
		if destination.Reception != "kds" && destination.PrinterID != nil {
			printer, err := one[Printer](handler.ctx, handler.tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", destination.PrinterID)
			if err != nil {
				return err
			}
			if _, err := handler.enqueue(document, *printer, nil); err != nil {
				return err
			}
		}
		for _, id := range ids {
			if err := events.Append(handler.ctx, handler.tx, "kitchen.ticket.created", "kitchen", &document.ID, &id); err != nil {
				return err
			}
		}
	}
	return nil
}

func clearInstructionPrices(instruction *Instructions) {
	instruction.Price = ""
	if instruction.Before != nil {
		before := *instruction.Before
		clearInstructionPrices(&before)
		instruction.Before = &before
	}
}

// enqueue renderiza una comanda guardada al ancho de la impresora destino (papel y columnas), con
// la versión de plantilla que guardó al emitirse, y la encola. Una reimpresión desde la cola pasa
// la fotografía del trabajo original: sale con las columnas con que se emitió (las fotografías
// anteriores a la etapa 9C no las tienen y salen con las predeterminadas, como entonces).
func (handler *service) enqueue(document Document, printer Printer, originalID *string) (*PrintJob, error) {
	layout, err := handler.renderStored(document, printer.RenderOptions())
	if err != nil {
		return nil, err
	}
	return handler.enqueueLayout(document.ID, printer, originalID, layout)
}

// enqueueLayout encola un documento renderizado: el contenido es siempre su texto (Text) y, solo si
// lleva atributos o imágenes, el trabajo guarda además la IR para el transporte. Los documentos de
// texto plano (todas las plantillas predeterminadas) producen trabajos idénticos a los anteriores.
func (handler *service) enqueueLayout(ticketID string, printer Printer, originalID *string, layout *documents.Layout) (*PrintJob, error) {
	var stored *documents.Layout
	if !layout.Plain() {
		stored = layout.ForQueue()
	}
	return handler.enqueueJob(ticketID, printer, originalID, layout.Text(), stored)
}

// enqueueContent guarda un trabajo de texto plano ya renderizado.
func (handler *service) enqueueContent(ticketID string, printer Printer, originalID *string, content string) (*PrintJob, error) {
	return handler.enqueueJob(ticketID, printer, originalID, content, nil)
}

// enqueueJob guarda un trabajo con su contenido ya renderizado y, si lo tiene, su layout con
// atributos. Comandas y documentos comerciales comparten así la misma cola durable, el lease y la
// protección contra duplicados.
func (handler *service) enqueueJob(ticketID string, printer Printer, originalID *string, content string, layout *documents.Layout) (*PrintJob, error) {
	state := "pending"
	message := ""
	var result any
	// Una impresora desactivada o de otra terminal (agente) no puede imprimir: el trabajo queda en
	// «Requiere intervención» con el motivo en palabras del restaurante (etapa 9F, §4.3). Las precuentas,
	// los tickets y los documentos de caja ya no llegan aquí con esas impresoras (decisión D4).
	if !printer.Active || printer.Transport == "agent" {
		state = "intervention"
		code := ProblemUnsupported
		message = unsupportedPrinterMessage
		if !printer.Active {
			code = ProblemInactive
			message = inactivePrinterMessage(printer.Name)
		}
		result = Outcome{State: state, Code: code, Message: message}
	}
	jobID := events.NewID()
	var operationID *string
	if handler.operationID != "" {
		operationID = &handler.operationID
	}
	var storedLayout []byte
	if layout != nil {
		storedLayout = encoded(layout)
	}
	var storedResult []byte
	if result != nil {
		storedResult = encoded(result)
	}
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_print_jobs(id,ticket_id,printer_id,operation_id,original_job_id,
        printer_snapshot,content,state,last_error,layout,result) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,coalesce($11,'{}'::jsonb))`, jobID, ticketID,
		printer.ID, operationID, originalID, encoded(printer), content, state, message, storedLayout, storedResult)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "printing.queued", "printing", &jobID, nil); err != nil {
		return nil, err
	}
	job, err := one[PrintJob](handler.ctx, handler.tx, "SELECT to_jsonb(j)-'content'-'layout' FROM app.kitchen_print_jobs j WHERE id=$1", jobID)
	return job, err
}
