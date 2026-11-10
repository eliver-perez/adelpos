package inventory

import (
	"regexp"
	"time"
)

// Parámetros de inventario (fila única con revisión, inventory.settings.manage). La fase A crea y
// administra todas las columnas; las usan después:
//   - purchase_tax_in_cost: recepciones (fase C), que copian el valor al registrarse, por lo que
//     cambiarlo solo afecta recepciones futuras;
//   - operating_day_cutoff: día operativo de las cancelaciones sin clasificar (fase B), en la zona
//     horaria del establecimiento;
//   - warn_products_without_recipe y la frecuencia de conteo: alertas (fase D).

var countFrequencyLabels = map[string]string{
	"none":    "Sin frecuencia",
	"daily":   "Diaria",
	"weekly":  "Semanal",
	"monthly": "Mensual",
	"custom":  "Cada cierto número de días",
}

var cutoffPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

type Settings struct {
	PurchaseTaxInCost         bool      `json:"purchase_tax_in_cost"`
	OperatingDayCutoff        string    `json:"operating_day_cutoff"`
	WarnProductsWithoutRecipe bool      `json:"warn_products_without_recipe"`
	CountFrequency            string    `json:"count_frequency"`
	CountFrequencyLabel       string    `json:"count_frequency_label"`
	CountIntervalDays         *int      `json:"count_interval_days"`
	Revision                  int       `json:"revision"`
	UpdatedAt                 time.Time `json:"updated_at"`
	UpdatedByName             *string   `json:"updated_by_name"`
}

func (handler *service) settings() (*Settings, error) {
	var settings Settings
	err := handler.tx.QueryRow(handler.ctx, `SELECT s.purchase_tax_in_cost,to_char(s.operating_day_cutoff,'HH24:MI'),
            s.warn_products_without_recipe,s.count_frequency,s.count_interval_days,s.revision,s.updated_at,u.display_name
        FROM app.inventory_settings s LEFT JOIN app.users u ON u.id=s.updated_by`).Scan(
		&settings.PurchaseTaxInCost, &settings.OperatingDayCutoff, &settings.WarnProductsWithoutRecipe, &settings.CountFrequency,
		&settings.CountIntervalDays, &settings.Revision, &settings.UpdatedAt, &settings.UpdatedByName)
	if err != nil {
		return nil, err
	}
	settings.CountFrequencyLabel = countFrequencyLabels[settings.CountFrequency]
	return &settings, nil
}

func (handler *service) readSettings(_ []string) (any, error) {
	return handler.settings()
}

type settingsInput struct {
	OperationID               string `json:"operation_id"`
	Revision                  int    `json:"revision"`
	PurchaseTaxInCost         bool   `json:"purchase_tax_in_cost"`
	OperatingDayCutoff        string `json:"operating_day_cutoff"`
	WarnProductsWithoutRecipe bool   `json:"warn_products_without_recipe"`
	CountFrequency            string `json:"count_frequency"`
	CountIntervalDays         *int   `json:"count_interval_days"`
}

// saveSettings guarda los parámetros con revisión optimista, auditoría (antes y después) y evento.
func (handler *service) saveSettings(_ []string) (any, error) {
	var input settingsInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if !cutoffPattern.MatchString(input.OperatingDayCutoff) {
		return nil, invalid("La hora de corte debe tener el formato HH:MM (de 00:00 a 23:59).")
	}
	if _, known := countFrequencyLabels[input.CountFrequency]; !known {
		return nil, invalid("La frecuencia de conteo no es válida.")
	}
	if input.CountFrequency == "custom" {
		if input.CountIntervalDays == nil || *input.CountIntervalDays < 1 || *input.CountIntervalDays > 365 {
			return nil, invalid("Indica cada cuántos días se cuenta (de 1 a 365).")
		}
	} else if input.CountIntervalDays != nil {
		return nil, invalid("El intervalo de días solo aplica a la frecuencia «Cada cierto número de días».")
	}
	if _, err := handler.tx.Exec(handler.ctx, "SELECT 1 FROM app.inventory_settings FOR UPDATE"); err != nil {
		return nil, err
	}
	previous, err := handler.settings()
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("La configuración de inventario")
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_settings SET purchase_tax_in_cost=$1,operating_day_cutoff=$2::time,
            warn_products_without_recipe=$3,count_frequency=$4,count_interval_days=$5,updated_by=$6,revision=revision+1,
            updated_at=now()`,
		input.PurchaseTaxInCost, input.OperatingDayCutoff, input.WarnProductsWithoutRecipe, input.CountFrequency,
		input.CountIntervalDays, handler.actorReference())
	if err != nil {
		return nil, err
	}
	saved, err := handler.settings()
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.settings.changed", nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.settings_changed", "inventory_settings", nil, previous, saved)
}
