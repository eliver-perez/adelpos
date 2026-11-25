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
}

// timingView lee la configuración con los límites de las pantallas.
func (handler *service) timingView() (*SettingsView, error) {
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	view := &SettingsView{Settings: *settings}
	err = handler.tx.QueryRow(handler.ctx, "SELECT display_warning_minutes,display_late_minutes FROM app.kitchen_settings").
		Scan(&view.DisplayWarningMinutes, &view.DisplayLateMinutes)
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
