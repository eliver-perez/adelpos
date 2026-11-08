package kitchen

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"

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
		document := Document{ID: events.NewID(), Kind: kind, CreatedAt: time.Now().UTC(), Context: context, Settings: *settings, Sections: group}
		ids := []string{}
		for _, section := range group {
			ids = append(ids, section.Station.ID)
		}
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_tickets(id,source_event_id,release_id,kind,station_ids,document)
            VALUES ($1,$2,$3,$4,$5,$6)`, document.ID, eventID, releaseID, kind, ids, encoded(document))
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

func (handler *service) enqueue(document Document, printer Printer, originalID *string) (*PrintJob, error) {
	state := "pending"
	message := ""
	if !printer.Active || printer.Transport == "agent" {
		state = "intervention"
		message = "El destino está inactivo o requiere un agente de impresión aún no implementado."
	}
	jobID := events.NewID()
	var operationID *string
	if handler.operationID != "" {
		operationID = &handler.operationID
	}
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_print_jobs(id,ticket_id,printer_id,operation_id,original_job_id,
        printer_snapshot,content,state,last_error) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, jobID, document.ID, printer.ID,
		operationID, originalID, encoded(printer), RenderText(document, printer.PaperWidth), state, message)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "printing.queued", "printing", &jobID, nil); err != nil {
		return nil, err
	}
	job, err := one[PrintJob](handler.ctx, handler.tx, "SELECT to_jsonb(j)-'content' FROM app.kitchen_print_jobs j WHERE id=$1", jobID)
	return job, err
}
