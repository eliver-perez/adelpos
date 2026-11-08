package kitchen

import "slices"

func (handler *service) saveTiming() (any, error) {
	var input struct {
		OperationID      string `json:"operation_id"`
		Revision         int    `json:"revision"`
		AutomaticRelease bool   `json:"automatic_release"`
		LeadMinutes      int    `json:"lead_minutes"`
		Tracking         string `json:"tracking"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	before, err := handler.settings()
	if err != nil {
		return nil, err
	}
	if before.Revision != input.Revision {
		return nil, conflict()
	}
	if input.LeadMinutes < 0 || input.LeadMinutes > 1440 || !slices.Contains([]string{"item", "order"}, input.Tracking) {
		return nil, invalid("Revisa la anticipación (0–1440) y el seguimiento.")
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_settings
		SET automatic_release=$1,lead_minutes=$2,tracking=$3,revision=revision+1`,
		input.AutomaticRelease, input.LeadMinutes, input.Tracking)
	if err != nil {
		return nil, err
	}
	saved, err := handler.settings()
	if err != nil {
		return nil, err
	}
	return saved, handler.history("timing.saved", nil, before, saved)
}

func (handler *service) settings() (*Settings, error) {
	return one[Settings](handler.ctx, handler.tx, "SELECT to_jsonb(s) FROM app.kitchen_settings s")
}

func (handler *service) saveSettings() (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Settings
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	before, err := handler.settings()
	if err != nil {
		return nil, err
	}
	setting := input.Settings
	if before.Revision != setting.Revision {
		return nil, conflict()
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
        quantity_style=$10,revision=revision+1`, setting.Grouping, setting.Tracking, setting.AutomaticRelease,
		setting.LeadMinutes, setting.ShowPrices, setting.ShowNotes, setting.ShowEstablishment, setting.TextSize,
		setting.LineSpacing, setting.QuantityStyle)
	if err != nil {
		return nil, err
	}
	saved, err := handler.settings()
	if err != nil {
		return nil, err
	}
	return saved, handler.history("settings.saved", nil, before, saved)
}
