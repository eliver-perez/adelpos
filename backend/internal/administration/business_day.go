package administration

import (
	"context"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/events"
)

// «Hora de inicio del día» (§0-B de la etapa 9A). Es una sola configuración: la hora de corte del
// día operativo que creó la etapa 8 (inventory_settings.operating_day_cutoff), en la zona horaria del
// establecimiento. Desde la 9A se edita solo aquí, en la configuración general (settings.manage); en
// Inventarios → Parámetros se muestra en solo lectura y su PUT ya no la cambia. Con 00:00 el día
// comercial es la fecha del calendario; con 05:00, una venta de la 01:30 pertenece al día anterior.
// Cambiarla no reescribe registros: solo cambia cómo se agrupan los días en las consultas y el día
// operativo de las cancelaciones futuras.

var businessDayPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// BusinessDaySettings es la respuesta de settings/business-day.
type BusinessDaySettings struct {
	OperatingDayCutoff string    `json:"operating_day_cutoff"`
	Timezone           string    `json:"timezone"`
	Revision           int       `json:"revision"`
	UpdatedAt          time.Time `json:"updated_at"`
	UpdatedByName      *string   `json:"updated_by_name"`
}

func readBusinessDay(ctx context.Context, tx pgx.Tx, lock bool) (*BusinessDaySettings, error) {
	query := `SELECT to_char(s.operating_day_cutoff,'HH24:MI'),coalesce(e.timezone,'UTC'),s.revision,s.updated_at,u.display_name
        FROM app.inventory_settings s LEFT JOIN app.establishment e ON true LEFT JOIN app.users u ON u.id=s.updated_by`
	if lock {
		query += " FOR UPDATE OF s"
	}
	var settings BusinessDaySettings
	err := tx.QueryRow(ctx, query).Scan(&settings.OperatingDayCutoff, &settings.Timezone, &settings.Revision, &settings.UpdatedAt,
		&settings.UpdatedByName)
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

func businessDaySettings(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	if request.Method == "GET" {
		if err := principal.require("settings.read"); err != nil {
			return nil, err
		}
		return readBusinessDay(ctx, tx, false)
	}
	if request.Method != "PUT" {
		return nil, invalid("Método no válido para la hora de inicio del día.")
	}
	// Se autoriza antes de cualquier lectura con bloqueo o escritura.
	if err := principal.require("settings.manage"); err != nil {
		return nil, err
	}
	var input struct {
		OperatingDayCutoff string `json:"operating_day_cutoff"`
		Revision           int    `json:"revision"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if !businessDayPattern.MatchString(input.OperatingDayCutoff) {
		return nil, invalid("La hora de inicio del día debe tener el formato HH:MM (de 00:00 a 23:59).")
	}
	previous, err := readBusinessDay(ctx, tx, true)
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, &Fault{Status: 409, Code: "REVISION_CONFLICT",
			Message: "La configuración cambió en otra terminal. Actualiza y vuelve a guardar."}
	}
	if previous.OperatingDayCutoff == input.OperatingDayCutoff {
		return previous, nil
	}
	// Comparte la fila y la revisión de los parámetros de inventario: Inventarios → Parámetros verá el
	// cambio de revisión y recargará.
	if _, err := tx.Exec(ctx, `UPDATE app.inventory_settings SET operating_day_cutoff=$1::time,updated_by=$2,
            revision=revision+1,updated_at=now()`, input.OperatingDayCutoff, principal.UserID); err != nil {
		return nil, err
	}
	saved, err := readBusinessDay(ctx, tx, false)
	if err != nil {
		return nil, err
	}
	if err := events.Append(ctx, tx, "inventory.settings.changed", "inventory", nil, nil); err != nil {
		return nil, err
	}
	before := map[string]string{"operating_day_cutoff": previous.OperatingDayCutoff}
	after := map[string]string{"operating_day_cutoff": saved.OperatingDayCutoff}
	return saved, audit(ctx, tx, principal, "settings.business_day_changed", "inventory_settings", nil, before, after, true)
}
