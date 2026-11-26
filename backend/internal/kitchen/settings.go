package kitchen

import "slices"

func (handler *service) saveTiming() (any, error) {
	var input struct {
		OperationID      string `json:"operation_id"`
		Revision         int    `json:"revision"`
		AutomaticRelease bool   `json:"automatic_release"`
		LeadMinutes      int    `json:"lead_minutes"`
		Tracking         string `json:"tracking"`
		// Etapa K1 (Fase A, K1-D17): los límites de color generales de las pantallas de cocina Android. Opcionales: sin
		// ellos se conservan, así que el escritorio anterior sigue guardando los tiempos sin tocarlos.
		DisplayWarningMinutes *int `json:"display_warning_minutes"`
		DisplayLateMinutes    *int `json:"display_late_minutes"`
		// Etapa K1.1 (K11-D3): se acepta y no se usa, como en PUT kitchen/settings: «Productos sin ruta» solo se guarda
		// con PUT kitchen/unrouted.
		UnroutedStationID *string `json:"unrouted_station_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	before, err := handler.timingView()
	if err != nil {
		return nil, err
	}
	if before.Revision != input.Revision {
		return nil, conflict()
	}
	if input.LeadMinutes < 0 || input.LeadMinutes > 1440 || !slices.Contains([]string{"item", "order"}, input.Tracking) {
		return nil, invalid("Revisa la anticipación (0–1440) y el seguimiento.")
	}
	warning, late := before.DisplayWarningMinutes, before.DisplayLateMinutes
	if input.DisplayWarningMinutes != nil {
		warning = *input.DisplayWarningMinutes
	}
	if input.DisplayLateMinutes != nil {
		late = *input.DisplayLateMinutes
	}
	// La misma regla que las restricciones de la 0029: «atención» de 1 a 1440 minutos y «retraso» después de «atención»,
	// hasta 2880.
	if warning < 1 || warning > 1440 || late <= warning || late > 2880 {
		return nil, invalid("El aviso de atención va de 1 a 1440 minutos y el de retraso debe ser mayor, hasta 2880.")
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_settings
		SET automatic_release=$1,lead_minutes=$2,tracking=$3,display_warning_minutes=$4,display_late_minutes=$5,
		revision=revision+1`,
		input.AutomaticRelease, input.LeadMinutes, input.Tracking, warning, late)
	if err != nil {
		return nil, err
	}
	saved, err := handler.timingView()
	if err != nil {
		return nil, err
	}
	return saved, handler.history("timing.saved", nil, before, saved)
}

func (handler *service) settings() (*Settings, error) {
	return one[Settings](handler.ctx, handler.tx, "SELECT to_jsonb(s) FROM app.kitchen_settings s")
}

// SettingsView es la configuración general de cocina tal como la leen GET kitchen/settings y PUT kitchen/timing: la de
// siempre más, desde la etapa K1 (Fase A), los límites de color generales de las pantallas de cocina Android. No es
// parte de Settings, que viaja en cada comanda guardada: así las comandas no cambian de forma.
type SettingsView struct {
	Settings
	DisplayWarningMinutes int `json:"display_warning_minutes"`
	DisplayLateMinutes    int `json:"display_late_minutes"`
	// UnroutedStationID es la estación del ajuste «Productos sin ruta» (etapa K1.1, K11-D3); null con «No necesitan
	// preparación». Solo se lee aquí: se guarda con PUT kitchen/unrouted.
	UnroutedStationID *string `json:"unrouted_station_id"`
}

// timingView lee la configuración con los límites de las pantallas y la estación de «Productos sin ruta».
func (handler *service) timingView() (*SettingsView, error) {
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	view := &SettingsView{Settings: *settings}
	err = handler.tx.QueryRow(handler.ctx, `SELECT display_warning_minutes,display_late_minutes,unrouted_station_id::text
        FROM app.kitchen_settings`).Scan(&view.DisplayWarningMinutes, &view.DisplayLateMinutes, &view.UnroutedStationID)
	return view, err
}

func (handler *service) saveSettings() (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Settings
		// Etapa K1 (Fase A): el escritorio guarda la configuración con lo que leyó de GET kitchen/settings, que desde la
		// K1 trae los límites de color de las pantallas de cocina. Opcionales: sin ellos se conservan; si cambian, exigen
		// kitchen.timing.manage, como los demás tiempos.
		DisplayWarningMinutes *int `json:"display_warning_minutes"`
		DisplayLateMinutes    *int `json:"display_late_minutes"`
		// Etapa K1.1 (K11-D3): el escritorio guarda la configuración con todo lo que leyó de GET kitchen/settings, que
		// desde la 0.17.0 trae la estación de «Productos sin ruta». Se acepta y no se usa: PUT kitchen/settings no la
		// toca (se guarda con PUT kitchen/unrouted, con su propio valor esperado).
		UnroutedStationID *string `json:"unrouted_station_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	view, err := handler.timingView()
	if err != nil {
		return nil, err
	}
	before := &view.Settings
	setting := input.Settings
	if before.Revision != setting.Revision {
		return nil, conflict()
	}
	warning, late := view.DisplayWarningMinutes, view.DisplayLateMinutes
	if input.DisplayWarningMinutes != nil {
		warning = *input.DisplayWarningMinutes
	}
	if input.DisplayLateMinutes != nil {
		late = *input.DisplayLateMinutes
	}
	if warning != view.DisplayWarningMinutes || late != view.DisplayLateMinutes {
		if err := handler.request.Actor.Require("kitchen.timing.manage"); err != nil {
			return nil, err
		}
		if warning < 1 || warning > 1440 || late <= warning || late > 2880 {
			return nil, invalid("El aviso de atención va de 1 a 1440 minutos y el de retraso debe ser mayor, hasta 2880.")
		}
	}
	if !slices.Contains([]string{"independent", "unified"}, setting.Grouping) || !slices.Contains([]string{"item", "order"}, setting.Tracking) ||
		!slices.Contains([]string{"normal", "large"}, setting.TextSize) || !slices.Contains([]string{"prefix", "multiplier"}, setting.QuantityStyle) ||
		setting.LeadMinutes < 0 || setting.LeadMinutes > 1440 || setting.LineSpacing < 1 || setting.LineSpacing > 3 {
		return nil, invalid("Revisa las opciones del formato, seguimiento y anticipación.")
	}
	if before.AutomaticRelease != setting.AutomaticRelease || before.LeadMinutes != setting.LeadMinutes || before.Tracking != setting.Tracking {
		if err := handler.request.Actor.Require("kitchen.timing.manage"); err != nil {
			return nil, err
		}
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_settings SET grouping=$1,tracking=$2,automatic_release=$3,
        lead_minutes=$4,show_prices=$5,show_notes=$6,show_establishment=$7,text_size=$8,line_spacing=$9,
        quantity_style=$10,display_warning_minutes=$11,display_late_minutes=$12,revision=revision+1`, setting.Grouping,
		setting.Tracking, setting.AutomaticRelease, setting.LeadMinutes, setting.ShowPrices, setting.ShowNotes,
		setting.ShowEstablishment, setting.TextSize, setting.LineSpacing, setting.QuantityStyle, warning, late)
	if err != nil {
		return nil, err
	}
	saved, err := handler.timingView()
	if err != nil {
		return nil, err
	}
	return saved, handler.history("settings.saved", nil, view, saved)
}

// UnroutedView es el ajuste «Productos sin ruta» (etapa K1.1, K11-D3) tal como lo leen GET kitchen/coverage y
// PUT kitchen/unrouted. Effective es «none» (no necesitan preparación: sin estación elegida o con la del ajuste
// desactivada) o «station» (van a esa estación, activa). Los datos de la estación son null sin estación elegida.
type UnroutedView struct {
	StationID     *string `json:"station_id"`
	StationName   *string `json:"station_name"`
	StationActive *bool   `json:"station_active"`
	Effective     string  `json:"effective"`
}

func (handler *service) unroutedView() (*UnroutedView, error) {
	view := &UnroutedView{Effective: "none"}
	err := handler.tx.QueryRow(handler.ctx, `SELECT s.id::text,s.name,s.active FROM app.kitchen_settings k
        LEFT JOIN app.kitchen_stations s ON s.id=k.unrouted_station_id`).Scan(&view.StationID, &view.StationName, &view.StationActive)
	if err != nil {
		return nil, err
	}
	if view.StationActive != nil && *view.StationActive {
		view.Effective = "station"
	}
	return view, nil
}

// saveUnrouted guarda «Productos sin ruta» (PUT kitchen/unrouted, kitchen.routing.manage): station_id null es «No
// necesitan preparación» (el valor de siempre) y una estación es «Enviar a una estación». Como PUT kitchen/access, usa
// el valor esperado en lugar de la revisión de kitchen_settings, para no chocar con quien guarda «Comandas y tiempos».
// La estación debe existir y estar activa, con cualquier recepción; si después se desactiva, lo sin ruta no necesita
// preparación mientras tanto (decideRoute) y la cobertura lo avisa.
func (handler *service) saveUnrouted() (any, error) {
	var input struct {
		OperationID       string  `json:"operation_id"`
		StationID         *string `json:"station_id"`
		ExpectedStationID *string `json:"expected_station_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	before, err := handler.unroutedView()
	if err != nil {
		return nil, err
	}
	if !sameStation(before.StationID, input.ExpectedStationID) {
		return nil, &Error{Status: 409, Code: "KITCHEN_UNROUTED_CHANGED",
			Message: "Alguien más cambió «Productos sin ruta». Revisa el ajuste actual antes de guardar."}
	}
	if input.StationID != nil {
		active := false
		if identifierPattern.MatchString(*input.StationID) {
			err := handler.tx.QueryRow(handler.ctx, "SELECT coalesce((SELECT active FROM app.kitchen_stations WHERE id=$1),false)",
				*input.StationID).Scan(&active)
			if err != nil {
				return nil, err
			}
		}
		if !active {
			return nil, invalid("Elige una estación activa.")
		}
	}
	if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.kitchen_settings SET unrouted_station_id=$1", input.StationID); err != nil {
		return nil, err
	}
	saved, err := handler.unroutedView()
	if err != nil {
		return nil, err
	}
	return saved, handler.history("unrouted.saved", nil, before, saved)
}

// sameStation compara dos estaciones opcionales (dos ausencias son iguales).
func sameStation(first, second *string) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return *first == *second
}
