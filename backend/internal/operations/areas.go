package operations

import "strings"

const areaQuery = `SELECT to_jsonb(a) || jsonb_build_object('table_count',
    (SELECT count(*) FROM app.restaurant_tables t WHERE t.area_id=a.id)) FROM app.restaurant_areas a`

func (handler *service) areas(parts []string) (any, error) {
	if handler.request.Method == "GET" {
		return readList[Area](handler.ctx, handler.tx, areaQuery+" ORDER BY a.sort_order,a.name,a.id")
	}
	var input struct {
		OperationID string `json:"operation_id"`
		Area
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	area := input.Area
	area.Name = strings.TrimSpace(area.Name)
	if area.Name == "" || len(area.Name) > 100 || len(area.Description) > 2000 {
		return nil, invalid("Indica un área con nombre de hasta 100 caracteres y descripción de hasta 2000.")
	}
	var before any
	if len(parts) == 1 {
		if area.ID != "" || area.Revision != 0 {
			return nil, invalid("El área nueva no debe reutilizar identidad ni revisión.")
		}
		area.ID = newID()
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.restaurant_areas(id,name,description,sort_order,active)
            VALUES ($1,$2,$3,$4,$5)`, area.ID, area.Name, area.Description, area.SortOrder, area.Active)
		if err != nil {
			return nil, err
		}
	} else {
		old, err := readOne[Area](handler.ctx, handler.tx, areaQuery+" WHERE a.id=$1", parts[1])
		if err != nil {
			return nil, err
		}
		if area.Revision != old.Revision || (area.ID != "" && area.ID != old.ID) {
			return nil, conflict()
		}
		area.ID = old.ID
		before = old
		if !area.Active {
			var occupied bool
			err := handler.tx.QueryRow(handler.ctx, `SELECT EXISTS(SELECT FROM app.restaurant_accounts a
                JOIN app.restaurant_tables t ON t.id=a.table_id WHERE t.area_id=$1 AND a.state='open')`, area.ID).Scan(&occupied)
			if err != nil {
				return nil, err
			}
			if occupied {
				return nil, invalid("Mueve o resuelve las cuentas abiertas antes de desactivar el área.")
			}
		}
		_, err = handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_areas SET name=$1,description=$2,sort_order=$3,
            active=$4,revision=revision+1,updated_at=now() WHERE id=$5 AND revision=$6`,
			area.Name, area.Description, area.SortOrder, area.Active, area.ID, area.Revision)
		if err != nil {
			return nil, err
		}
	}
	saved, err := readOne[Area](handler.ctx, handler.tx, areaQuery+" WHERE a.id=$1", area.ID)
	if err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "restaurant.area.saved", "area", &area.ID, before, saved)
}

func (handler *service) overview() (any, error) {
	if err := handler.request.Actor.Require("restaurant.areas.read"); err != nil {
		return nil, err
	}
	areas, err := readList[Area](handler.ctx, handler.tx, areaQuery+" ORDER BY a.sort_order,a.name,a.id")
	if err != nil {
		return nil, err
	}
	tables, err := readList[Table](handler.ctx, handler.tx, tableQuery+" ORDER BY t.area_id,t.name,t.id")
	if err != nil {
		return nil, err
	}
	settings, err := handler.readSettings()
	if err != nil {
		return nil, err
	}
	location, err := handler.location()
	if err != nil {
		return nil, err
	}
	return map[string]any{"areas": areas, "tables": tables, "settings": settings, "timezone": location.String()}, nil
}

func (handler *service) readSettings() (*Settings, error) {
	return readOne[Settings](handler.ctx, handler.tx, "SELECT to_jsonb(s) FROM app.restaurant_settings s")
}

func (handler *service) settings() (any, error) {
	if handler.request.Method == "GET" {
		return handler.readSettings()
	}
	var input struct {
		OperationID string `json:"operation_id"`
		Settings
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	old, err := handler.readSettings()
	if err != nil {
		return nil, err
	}
	if old.Revision != input.Revision {
		return nil, conflict()
	}
	validWarning := input.WarningMinutes >= 1 && input.WarningMinutes <= 240
	validOverdue := input.OverdueMinutes >= input.WarningMinutes && input.OverdueMinutes <= 1440
	if !validWarning || !validOverdue {
		return nil, invalid("La advertencia admite de 1 a 240 minutos; el atraso debe ser igual o mayor, hasta 1440.")
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_settings SET warning_minutes=$1,overdue_minutes=$2,revision=revision+1`, input.WarningMinutes, input.OverdueMinutes)
	if err != nil {
		return nil, err
	}
	saved, err := handler.readSettings()
	if err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "restaurant.settings.saved", "restaurant_settings", nil, old, saved)
}
