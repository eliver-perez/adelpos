package kitchen

import (
	"encoding/json"

	"local/ad-el-pos/server/internal/events"
)

type commercialChange struct {
	OrderID   string   `json:"order_id"`
	VersionID string   `json:"version_id"`
	Modified  []string `json:"modified_unit_ids"`
	Added     []string `json:"added_unit_ids"`
	Cancelled []string `json:"cancelled_unit_ids"`
	UnitIDs   []string `json:"unit_ids"`
	Reason    string   `json:"reason"`
}
type taskChange struct {
	Unit     unitRecord
	Previous []Task
	Routing  *routingResult
	Version  *versionSnapshot
	Release  *Release
}

func (handler *service) integrateChange(eventID, accountID, kind string, payload []byte) error {
	var change commercialChange
	if err := json.Unmarshal(payload, &change); err != nil {
		return err
	}
	if kind == "item.cancelled" {
		change.Cancelled = change.UnitIDs
	}
	affected := append(append([]string{}, change.Modified...), change.Cancelled...)
	pending, err := many[Release](handler.ctx, handler.tx, `SELECT to_jsonb(r) FROM app.kitchen_releases r
        WHERE r.unit_ids && $1::uuid[] AND r.state IN ('pending','scheduled','blocked') ORDER BY created_at`, affected)
	if err != nil {
		return err
	}
	if len(pending) > 0 && len(change.Added) > 0 {
		_, err := handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_releases SET unit_ids=ARRAY(SELECT DISTINCT unnest(unit_ids||$1::uuid[])),
            revision=revision+1 WHERE id=$2`, change.Added, pending[0].ID)
		if err != nil {
			return err
		}
		change.Added = nil
	}
	units, err := loadUnits(handler.ctx, handler.tx, affected)
	if err != nil {
		return err
	}
	plan := []taskChange{}
	warnings := []string{}
	for _, unit := range units {
		previous, err := many[Task](handler.ctx, handler.tx, "SELECT to_jsonb(t) FROM app.kitchen_tasks t WHERE unit_id=$1 AND state<>'cancelled' ORDER BY station_id", unit.ID)
		if err != nil {
			return err
		}
		if len(previous) == 0 {
			continue
		}
		release, err := readRelease(handler.ctx, handler.tx, previous[0].ReleaseID)
		if err != nil {
			return err
		}
		entry := taskChange{Unit: unit, Previous: previous, Release: release}
		if unit.State == "active" {
			version, err := readVersion(handler.ctx, handler.tx, unit.VersionID)
			if err != nil {
				return err
			}
			routing, err := resolveRouting(handler.ctx, handler.tx, version.Quote.ProductBreakdown, version.Catalog, release.RoutingSnapshot)
			if err != nil {
				return err
			}
			for stationID := range routing.Stations {
				if frozen, exists := release.StationSnapshot[stationID]; exists {
					routing.Stations[stationID] = frozen
				}
			}
			entry.Version, entry.Routing = version, routing
			warnings = append(warnings, routing.Warnings...)
		}
		for _, task := range previous {
			station, err := one[Station](handler.ctx, handler.tx, "SELECT to_jsonb(s) FROM app.kitchen_stations s WHERE id=$1", task.StationID)
			if err != nil {
				return err
			}
			if !station.Active {
				warnings = append(warnings, "Aviso pendiente para estación inactiva: "+station.Name)
			}
		}
		plan = append(plan, entry)
	}
	if len(warnings) > 0 {
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_pending_changes(event_id,account_id,kind,payload,warnings)
            VALUES ($1,$2,$3,$4,$5) ON CONFLICT(event_id) DO UPDATE SET warnings=excluded.warnings,revision=kitchen_pending_changes.revision+1`,
			eventID, accountID, kind, payload, encoded(warnings))
		if err != nil {
			return err
		}
		for _, entry := range plan {
			for _, task := range entry.Previous {
				if err := events.Append(handler.ctx, handler.tx, "kitchen.change.pending", "kitchen", &eventID, &task.StationID); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := handler.applyTaskChanges(eventID, change.Reason, plan); err != nil {
		return err
	}
	if len(change.Added) > 0 {
		if _, err := handler.createRelease(eventID, accountID, change.OrderID, change.Added); err != nil {
			return err
		}
	}
	// Cancelar antes de liberar no genera preparación ni papel para algo que cocina nunca recibió.
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_releases r SET state='cancelled',revision=revision+1
        WHERE state IN ('pending','scheduled','blocked') AND unit_ids && $1::uuid[]
        AND NOT EXISTS(SELECT FROM app.restaurant_item_units u WHERE u.id=ANY(r.unit_ids) AND u.state='active')`, affected)
	if err != nil {
		return err
	}
	_, err = handler.tx.Exec(handler.ctx, "DELETE FROM app.kitchen_pending_changes WHERE event_id=$1", eventID)
	return err
}

func (handler *service) applyTaskChanges(eventID, reason string, plan []taskChange) error {
	type notice struct {
		Release  *Release
		Kind     string
		Sections map[string]Section
	}
	notices := map[string]*notice{}
	addNotice := func(release *Release, station Station, kind string, item Instructions) {
		key := release.ID + "/" + kind
		entry := notices[key]
		if entry == nil {
			entry = &notice{Release: release, Kind: kind, Sections: map[string]Section{}}
			notices[key] = entry
		}
		section := entry.Sections[station.ID]
		section.Station = station
		section.Items = append(section.Items, item)
		entry.Sections[station.ID] = section
	}
	for _, entry := range plan {
		visited := map[string]bool{}
		for _, old := range entry.Previous {
			visited[old.StationID] = true
			station, err := one[Station](handler.ctx, handler.tx, "SELECT to_jsonb(s) FROM app.kitchen_stations s WHERE id=$1", old.StationID)
			if err != nil {
				return err
			}
			if frozen, exists := entry.Release.StationSnapshot[old.StationID]; exists {
				station = &frozen
			}
			parts, remains := []Part{}, false
			if entry.Routing != nil {
				parts, remains = entry.Routing.Parts[old.StationID]
			}
			if entry.Unit.State == "cancelled" || !remains {
				if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.kitchen_tasks SET state='cancelled',revision=revision+1,updated_at=now() WHERE id=$1", old.ID); err != nil {
					return err
				}
				instruction := old.Instructions
				instruction.Reason = reason
				addNotice(entry.Release, *station, "cancellation", instruction)
				continue
			}
			instruction := Instructions{ItemID: entry.Unit.ItemID, VersionID: entry.Unit.VersionID, UnitIDs: []string{entry.Unit.ID},
				Name: entry.Version.Quote.Name, Quantity: 1, Parts: parts, Price: entry.Unit.Total, Notes: entry.Version.Request.Customization.Notes}
			preparationChanged := old.Instructions.Name != instruction.Name || old.Instructions.Notes != instruction.Notes ||
				string(encoded(old.Instructions.Parts)) != string(encoded(instruction.Parts))
			if preparationChanged {
				instruction.Before = &old.Instructions
				instruction.Reason = reason
				addNotice(entry.Release, *station, "modification", instruction)
			}
			// Una modificación económica sin cambio de receta no reinicia la preparación.
			state := old.State
			if preparationChanged {
				state = "released"
			}
			stored := instruction
			stored.Before = nil
			_, err = handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_tasks SET version_id=$1,instructions=$2,state=$3,
                started_at=CASE WHEN $4 THEN NULL ELSE started_at END,ready_at=CASE WHEN $4 THEN NULL ELSE ready_at END,
                revision=revision+1,updated_at=now() WHERE id=$5`, entry.Unit.VersionID, encoded(stored), state, preparationChanged, old.ID)
			if err != nil {
				return err
			}
		}
		if entry.Routing == nil {
			continue
		}
		for stationID, parts := range entry.Routing.Parts {
			if visited[stationID] {
				continue
			}
			instruction := Instructions{ItemID: entry.Unit.ItemID, VersionID: entry.Unit.VersionID, UnitIDs: []string{entry.Unit.ID},
				Name: entry.Version.Quote.Name, Quantity: 1, Parts: parts, Price: entry.Unit.Total, Reason: reason, Notes: entry.Version.Request.Customization.Notes}
			_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_tasks(id,unit_id,release_id,station_id,version_id,instructions)
                VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT(unit_id,station_id) DO UPDATE SET
                state='released',version_id=excluded.version_id,instructions=excluded.instructions,
                started_at=NULL,ready_at=NULL,revision=kitchen_tasks.revision+1,updated_at=now()`, events.NewID(), entry.Unit.ID,
				entry.Release.ID, stationID, entry.Unit.VersionID, encoded(instruction))
			if err != nil {
				return err
			}
			addNotice(entry.Release, entry.Routing.Stations[stationID], "modification", instruction)
		}
		// Ampliar el snapshot solo mediante la modificación explícita, nunca por cambiar configuración.
		merged := entry.Release.RoutingSnapshot
		for id, route := range entry.Routing.Snapshot {
			merged[id] = route
		}
		if entry.Release.StationSnapshot == nil {
			entry.Release.StationSnapshot = map[string]Station{}
		}
		for id, station := range entry.Routing.Stations {
			entry.Release.StationSnapshot[id] = station
		}
		_, err := handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_releases
			SET routing_snapshot=$1,station_snapshot=$2,revision=revision+1 WHERE id=$3`,
			encoded(merged), encoded(entry.Release.StationSnapshot), entry.Release.ID)
		if err != nil {
			return err
		}
	}
	for _, notice := range notices {
		context := notice.Release.ContextSnapshot
		if err := handler.persistDocuments(context, notice.Sections, notice.Kind, &eventID, &notice.Release.ID); err != nil {
			return err
		}
	}
	return nil
}

func (handler *service) retryChange(id string) (any, error) {
	if !handler.request.Actor.Can("kitchen.stations.all") {
		return nil, &Error{Status: 403, Code: "STATION_SCOPE_DENIED", Message: "Se requiere acceso a todas las estaciones afectadas."}
	}
	var input struct {
		OperationID string `json:"operation_id"`
		Revision    int    `json:"revision"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	var accountID, kind string
	var payload []byte
	var revision int
	err := handler.tx.QueryRow(handler.ctx, `SELECT account_id,kind,payload,revision
		FROM app.kitchen_pending_changes WHERE event_id=$1`, id).
		Scan(&accountID, &kind, &payload, &revision)
	if err != nil {
		return nil, err
	}
	if revision != input.Revision {
		return nil, conflict()
	}
	if err := handler.integrateChange(id, accountID, kind, payload); err != nil {
		return nil, err
	}
	return map[string]bool{"reviewed": true}, handler.history("change.retried", &id, nil, map[string]string{"event_id": id})
}
