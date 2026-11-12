package administration

import (
	"context"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/events"
)

// Umbral de cuenta abierta por mucho tiempo (etapa 9D, §3.4 y decisión 11 del coordinador): horas
// desde la apertura a partir de las cuales la tarjeta de la cuenta y su mesa muestran una advertencia
// discreta. Es una sola columna de restaurant_settings (12 por omisión, CHECK de 1 a 72 en la 0021),
// que Go entrega con el mapa y las cuentas (settings.long_open_hours y long_open calculado). Se lee
// con cualquier sesión, como las funciones del establecimiento, y se cambia en «Funciones del
// establecimiento» con su mismo permiso (settings.features.manage). PUT restaurant/settings
// (umbrales del tablero para llevar) no lo cambia.

const (
	minimumLongOpenHours = 1
	maximumLongOpenHours = 72
)

// LongOpenHoursSettings es la respuesta de settings/long-open-hours. Revision es la de la fila de
// restaurant_settings, compartida con los umbrales del tablero para llevar.
type LongOpenHoursSettings struct {
	LongOpenHours int `json:"long_open_hours"`
	MinimumHours  int `json:"minimum_hours"`
	MaximumHours  int `json:"maximum_hours"`
	Revision      int `json:"revision"`
}

func readLongOpenHours(ctx context.Context, tx pgx.Tx, lock bool) (*LongOpenHoursSettings, error) {
	query := "SELECT long_open_hours,revision FROM app.restaurant_settings"
	if lock {
		query += " FOR UPDATE"
	}
	settings := LongOpenHoursSettings{MinimumHours: minimumLongOpenHours, MaximumHours: maximumLongOpenHours}
	if err := tx.QueryRow(ctx, query).Scan(&settings.LongOpenHours, &settings.Revision); err != nil {
		return nil, err
	}
	return &settings, nil
}

func longOpenHoursSettings(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	if request.Method == "GET" {
		return readLongOpenHours(ctx, tx, false)
	}
	if request.Method != "PUT" {
		return nil, invalid("Método no válido para el umbral de cuenta abierta.")
	}
	// Se autoriza antes de cualquier lectura con bloqueo o escritura.
	if err := principal.require("settings.features.manage"); err != nil {
		return nil, err
	}
	var input struct {
		LongOpenHours int `json:"long_open_hours"`
		Revision      int `json:"revision"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if input.LongOpenHours < minimumLongOpenHours || input.LongOpenHours > maximumLongOpenHours {
		return nil, &Fault{Status: 422, Code: "LONG_OPEN_HOURS_INVALID",
			Message: "Indica de 1 a 72 horas para advertir las cuentas abiertas por mucho tiempo."}
	}
	previous, err := readLongOpenHours(ctx, tx, true)
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, &Fault{Status: 409, Code: "REVISION_CONFLICT",
			Message: "La configuración cambió en otra terminal. Actualiza y vuelve a guardar."}
	}
	if previous.LongOpenHours == input.LongOpenHours {
		return previous, nil
	}
	if _, err := tx.Exec(ctx, "UPDATE app.restaurant_settings SET long_open_hours=$1,revision=revision+1",
		input.LongOpenHours); err != nil {
		return nil, err
	}
	saved, err := readLongOpenHours(ctx, tx, false)
	if err != nil {
		return nil, err
	}
	// El mapa, las cuentas y el tablero para llevar vuelven a leer la marca de cuenta abierta.
	if err := events.Append(ctx, tx, "restaurant.settings.changed", "restaurant", nil, nil); err != nil {
		return nil, err
	}
	if err := events.Append(ctx, tx, "restaurant.settings.changed", "takeaway", nil, nil); err != nil {
		return nil, err
	}
	auditBefore := map[string]int{"long_open_hours": previous.LongOpenHours, "revision": previous.Revision}
	auditAfter := map[string]int{"long_open_hours": saved.LongOpenHours, "revision": saved.Revision}
	return saved, audit(ctx, tx, principal, "settings.long_open_hours_changed", "restaurant_settings", nil, auditBefore, auditAfter, true)
}
