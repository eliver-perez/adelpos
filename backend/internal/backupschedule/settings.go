package backupschedule

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
)

// Querier es lo mínimo que necesitan las lecturas: una transacción, una conexión o el pool de pgx.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Límites de cuántos respaldos automáticos se conservan.
const (
	MinKeep     = 1
	MaxKeep     = 365
	DefaultKeep = 30
)

// timePattern es una hora HH:MM de 00:00 a 23:59.
var timePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// Settings es la configuración de los respaldos automáticos (app.backup_settings).
type Settings struct {
	Enabled bool
	// Time es la hora HH:MM; vacía, la «Hora de inicio del día».
	Time string
	Keep int
	// Directory es la carpeta elegida; vacía, la predeterminada.
	Directory string
	// EnabledAt es desde cuándo están activos (para el aviso de respaldo atrasado).
	EnabledAt time.Time
	// InstallationID identifica la instalación en el nombre de sus respaldos si el servidor no tiene identidad.
	InstallationID string
}

// DefaultSettings es la configuración de una instalación nueva (la misma de la migración).
func DefaultSettings() Settings {
	return Settings{Enabled: true, Keep: DefaultKeep}
}

// LoadSettings lee la configuración. Sin fila (no debería pasar después de la migración) usa la predeterminada.
func LoadSettings(ctx context.Context, querier Querier) (Settings, error) {
	var settings Settings
	var scheduled, directory *string
	err := querier.QueryRow(ctx, `SELECT enabled, to_char(scheduled_time, 'HH24:MI'), keep_count, directory, enabled_at,
            installation_id::text
        FROM app.backup_settings WHERE singleton`).Scan(&settings.Enabled, &scheduled, &settings.Keep, &directory,
		&settings.EnabledAt, &settings.InstallationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	if scheduled != nil {
		settings.Time = *scheduled
	}
	if directory != nil {
		settings.Directory = *directory
	}
	return settings, nil
}

// ValidTime indica si una hora configurada es válida (vacía = la «Hora de inicio del día»).
func ValidTime(value string) bool {
	return value == "" || timePattern.MatchString(value)
}

// ParseTime separa una hora HH:MM válida en hora y minutos.
func ParseTime(value string) (int, int, error) {
	if !timePattern.MatchString(value) {
		return 0, 0, errors.New("hora no válida")
	}
	var hour, minute int
	if _, err := fmt.Sscanf(value, "%d:%d", &hour, &minute); err != nil {
		return 0, 0, err
	}
	return hour, minute, nil
}

// SaveSchedule guarda si están activos, la hora y cuántos conservar (la carpeta se guarda aparte, SaveDirectory).
func SaveSchedule(ctx context.Context, tx pgx.Tx, settings Settings, userID string) error {
	if !ValidTime(settings.Time) || settings.Keep < MinKeep || settings.Keep > MaxKeep {
		return errors.New("configuración de respaldos no válida")
	}
	var scheduled *string
	if settings.Time != "" {
		scheduled = &settings.Time
	}
	// Al activarlos de nuevo, el aviso de respaldo atrasado se mide desde ese momento.
	_, err := tx.Exec(ctx, `UPDATE app.backup_settings
        SET enabled = $1, scheduled_time = $2::time, keep_count = $3, updated_by = $4, updated_at = now(),
            enabled_at = CASE WHEN $1 AND NOT enabled THEN now() ELSE enabled_at END
        WHERE singleton`, settings.Enabled, scheduled, settings.Keep, nullable(userID))
	return err
}

// SaveDirectory guarda la carpeta elegida (vacía = la predeterminada).
func SaveDirectory(ctx context.Context, tx pgx.Tx, directory, userID string) error {
	_, err := tx.Exec(ctx, `UPDATE app.backup_settings SET directory = $1, updated_by = $2, updated_at = now()
        WHERE singleton`, nullable(directory), nullable(userID))
	return err
}

// nullable convierte un texto vacío en NULL.
func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
