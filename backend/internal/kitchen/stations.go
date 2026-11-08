package kitchen

import (
	"slices"
	"strings"

	"local/ad-el-pos/server/internal/events"
)

func (handler *service) saveStation(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Station
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	station := input.Station
	if station.Grouping == "" {
		station.Grouping = "inherit"
	}
	if station.Tracking == "" {
		station.Tracking = "inherit"
	}
	if station.TargetMinutes == 0 {
		station.TargetMinutes = 15
	}
	if station.WarningMinutes == 0 {
		station.WarningMinutes = 15
	}
	if station.OverdueMinutes == 0 {
		station.OverdueMinutes = 25
	}
	station.Name = strings.TrimSpace(station.Name)
	if station.Name == "" || len(station.Name) > 100 || len(station.Description) > 2000 {
		return nil, invalid("Revisa el nombre (100) y descripción (2000) de la estación.")
	}
	if !slices.Contains([]string{"printer", "kds", "both"}, station.Reception) ||
		!slices.Contains([]string{"inherit", "independent", "unified"}, station.Grouping) ||
		!slices.Contains([]string{"inherit", "item", "order"}, station.Tracking) {
		return nil, invalid("Revisa recepción, agrupación y seguimiento de la estación.")
	}
	if station.TargetMinutes < 1 || station.TargetMinutes > 1440 || station.WarningMinutes < 1 || station.WarningMinutes > 1440 ||
		station.OverdueMinutes < station.WarningMinutes || station.OverdueMinutes > 2880 {
		return nil, invalid("Los tiempos deben ser positivos y el atraso no puede anteceder a la advertencia.")
	}
	if station.Reception != "kds" && station.PrinterID == nil {
		return nil, invalid("La estación requiere una impresora configurada.")
	}
	if station.PrinterID != nil {
		var active bool
		if err := handler.tx.QueryRow(handler.ctx, "SELECT active FROM app.kitchen_printers WHERE id=$1", station.PrinterID).Scan(&active); err != nil {
			return nil, err
		}
		if station.Active && !active {
			return nil, invalid("Selecciona una impresora activa.")
		}
	}
	var before *Station
	if len(parts) == 1 && (station.TargetMinutes != 15 || station.WarningMinutes != 15 || station.OverdueMinutes != 25) {
		if err := handler.request.Actor.Require("kitchen.timing.manage"); err != nil {
			return nil, err
		}
	}
	if len(parts) == 1 {
		if station.ID != "" || station.Revision != 0 {
			return nil, invalid("Una estación nueva no reutiliza identidad ni revisión.")
		}
		station.ID = events.NewID()
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_stations(id,name,description,active,sort_order,reception,printer_id,
            target_minutes,warning_minutes,overdue_minutes,grouping,tracking) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			station.ID, station.Name, station.Description, station.Active, station.SortOrder, station.Reception, station.PrinterID,
			station.TargetMinutes, station.WarningMinutes, station.OverdueMinutes, station.Grouping, station.Tracking)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		before, err = one[Station](handler.ctx, handler.tx, "SELECT to_jsonb(s) FROM app.kitchen_stations s WHERE id=$1", parts[1])
		if err != nil {
			return nil, err
		}
		if station.ID != parts[1] || station.Revision != before.Revision {
			return nil, conflict()
		}
		if station.TargetMinutes != before.TargetMinutes || station.WarningMinutes != before.WarningMinutes || station.OverdueMinutes != before.OverdueMinutes {
			if err := handler.request.Actor.Require("kitchen.timing.manage"); err != nil {
				return nil, err
			}
		}
		_, err = handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_stations SET name=$1,description=$2,active=$3,sort_order=$4,
            reception=$5,printer_id=$6,target_minutes=$7,warning_minutes=$8,overdue_minutes=$9,grouping=$10,tracking=$11,
            revision=revision+1,updated_at=now() WHERE id=$12`, station.Name, station.Description, station.Active, station.SortOrder,
			station.Reception, station.PrinterID, station.TargetMinutes, station.WarningMinutes, station.OverdueMinutes,
			station.Grouping, station.Tracking, station.ID)
		if err != nil {
			return nil, err
		}
	}
	saved, err := one[Station](handler.ctx, handler.tx, "SELECT to_jsonb(s) FROM app.kitchen_stations s WHERE id=$1", station.ID)
	if err != nil {
		return nil, err
	}
	return saved, handler.history("station.saved", &station.ID, before, saved)
}

func (handler *service) saveAccess() (any, error) {
	var input struct {
		OperationID        string   `json:"operation_id"`
		DeviceID           string   `json:"device_id"`
		StationIDs         []string `json:"station_ids"`
		ExpectedStationIDs []string `json:"expected_station_ids"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validID(input.DeviceID); err != nil {
		return nil, err
	}
	if len(input.StationIDs) > 100 {
		return nil, invalid("Una terminal admite hasta 100 estaciones.")
	}
	seen := map[string]bool{}
	for _, id := range input.StationIDs {
		if err := validID(id); err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, invalid("La estación está repetida.")
		}
		seen[id] = true
	}
	before, err := many[string](handler.ctx, handler.tx, `SELECT to_jsonb(station_id) FROM app.kitchen_device_stations WHERE device_id=$1 ORDER BY station_id`, input.DeviceID)
	if err != nil {
		return nil, err
	}
	slices.Sort(input.ExpectedStationIDs)
	if !slices.Equal(before, input.ExpectedStationIDs) {
		return nil, conflict()
	}
	var available bool
	if err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.devices WHERE id=$1 AND revoked_at IS NULL)", input.DeviceID).Scan(&available); err != nil {
		return nil, err
	}
	if !available {
		return nil, invalid("La terminal debe existir y estar autorizada.")
	}
	if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.kitchen_device_stations WHERE device_id=$1", input.DeviceID); err != nil {
		return nil, err
	}
	for _, id := range input.StationIDs {
		if _, err := handler.tx.Exec(handler.ctx, "INSERT INTO app.kitchen_device_stations(device_id,station_id) VALUES ($1,$2)", input.DeviceID, id); err != nil {
			return nil, err
		}
	}
	return input, handler.history("station.access.saved", &input.DeviceID, before, input.StationIDs)
}
