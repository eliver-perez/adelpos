// Package businessday calcula el día comercial («Hora de inicio del día», §0-B de la etapa 9A) con
// la zona horaria del establecimiento. Es un paquete neutral: lo usan inventario (día operativo de
// las cancelaciones y alertas, etapa 8) y reportes (periodos y agrupaciones, etapa 9A) para que
// exista una sola definición del día.
//
// La hora de inicio del día es inventory_settings.operating_day_cutoff (05:00 por omisión) y la
// zona es establishment.timezone. Un instante pertenece al día cuya fecha local tiene la hora de
// inicio en o antes de él, hasta la hora de inicio del día siguiente. Con 05:00, una venta de la
// 01:30 del 11 pertenece al día 10; con 00:00, el día comercial es la fecha del calendario.
//
// Cambio de horario: si la hora de inicio no existe ese día (adelanto), el día empieza en el primer
// instante posterior al salto; si ocurre dos veces (atraso), empieza en la primera. Cambiar la hora
// de inicio nunca reescribe registros: solo cambia cómo se agrupan los instantes guardados.
package businessday

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Day es el día comercial de un instante (fecha a medianoche UTC) y el instante en que termina.
type Day struct {
	Date     time.Time
	Deadline time.Time
}

// Config es la zona horaria y la hora de inicio del día vigentes del establecimiento.
type Config struct {
	Timezone string
	Location *time.Location
	Hour     int
	Minute   int
}

// DefaultConfig es la configuración que se usa cuando todavía no existe el establecimiento o la
// fila de parámetros de inventario: UTC con inicio a las 05:00 (el comportamiento de la etapa 8).
func DefaultConfig() Config {
	return Config{Timezone: "UTC", Location: time.UTC, Hour: 5, Minute: 0}
}

// Querier es lo mínimo que necesita Load: una transacción o una conexión de pgx.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Load lee la zona horaria y la hora de inicio del día. Sin establecimiento o sin parámetros usa
// DefaultConfig; una zona horaria que Go no reconoce se interpreta como UTC (igual que la etapa 8).
func Load(ctx context.Context, querier Querier) (Config, error) {
	var timezone string
	var hour, minute int
	err := querier.QueryRow(ctx, `SELECT e.timezone,extract(hour FROM s.operating_day_cutoff)::int,
            extract(minute FROM s.operating_day_cutoff)::int
        FROM app.inventory_settings s CROSS JOIN app.establishment e`).Scan(&timezone, &hour, &minute)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return Config{}, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.UTC
	}
	return Config{Timezone: timezone, Location: location, Hour: hour, Minute: minute}, nil
}

// Cutoff es la hora de inicio del día en formato HH:MM.
func (config Config) Cutoff() string {
	return fmt.Sprintf("%02d:%02d", config.Hour, config.Minute)
}

// Interval es la hora de inicio como intervalo de PostgreSQL («05:00:00») para agrupar en SQL con
// ((instante AT TIME ZONE zona) - intervalo)::date.
func (config Config) Interval() string {
	return fmt.Sprintf("%02d:%02d:00", config.Hour, config.Minute)
}

// DayOf devuelve el día comercial de un instante con esta configuración.
func (config Config) DayOf(instant time.Time) Day {
	return DayOf(instant, config.Location, config.Hour, config.Minute)
}

// Start devuelve el instante en que empieza el día comercial de una fecha.
func (config Config) Start(date time.Time) time.Time {
	return CutoffInstant(date.Year(), date.Month(), date.Day(), config.Hour, config.Minute, config.Location)
}

// Bounds devuelve el intervalo [inicio, fin) de los días comerciales first..last (ambos incluidos).
// Los reportes filtran con estos instantes (columna >= inicio AND columna < fin) para usar índices.
func (config Config) Bounds(first, last time.Time) (time.Time, time.Time) {
	return config.Start(first), config.Start(last.AddDate(0, 0, 1))
}

// DayOf calcula el día comercial de un instante con la hora de inicio hour:minute.
func DayOf(instant time.Time, location *time.Location, hour, minute int) Day {
	local := instant.In(location)
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	if local.Hour() < hour || (local.Hour() == hour && local.Minute() < minute) {
		date = date.AddDate(0, 0, -1)
	}
	next := date.AddDate(0, 0, 1)
	deadline := CutoffInstant(next.Year(), next.Month(), next.Day(), hour, minute, location)
	// En un atraso de horario la hora local puede repetirse: si el corte (su primera aparición) ya
	// ocurrió antes del instante, el instante pertenece al día siguiente.
	for !deadline.After(instant) {
		date = next
		next = date.AddDate(0, 0, 1)
		deadline = CutoffInstant(next.Year(), next.Month(), next.Day(), hour, minute, location)
	}
	return Day{Date: date, Deadline: deadline}
}

// wallClock expresa la hora local de un instante como si fuera UTC, para comparar relojes.
func wallClock(instant time.Time, location *time.Location) time.Time {
	local := instant.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(),
		time.UTC)
}

// CutoffInstant devuelve el primer instante en que el reloj local marca la hora de inicio en esa
// fecha o, si esa hora no existe por un adelanto de horario, el primer instante posterior al salto.
func CutoffInstant(year int, month time.Month, day, hour, minute int, location *time.Location) time.Time {
	target := time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
	// Las desviaciones posibles alrededor de la fecha (antes y después de un cambio de horario).
	offsets := map[int]bool{}
	for _, probe := range []time.Duration{-36 * time.Hour, 0, 36 * time.Hour} {
		_, offset := target.Add(probe).In(location).Zone()
		offsets[offset] = true
	}
	// Cada desviación da un candidato; los que marcan exactamente la hora de inicio son válidos y se
	// elige el primero (en un atraso de horario hay dos).
	var earliest, latest, best time.Time
	for offset := range offsets {
		candidate := target.Add(-time.Duration(offset) * time.Second)
		if earliest.IsZero() || candidate.Before(earliest) {
			earliest = candidate
		}
		if latest.IsZero() || candidate.After(latest) {
			latest = candidate
		}
		if wallClock(candidate, location).Equal(target) && (best.IsZero() || candidate.Before(best)) {
			best = candidate
		}
	}
	if !best.IsZero() {
		return best
	}
	// La hora no existe: búsqueda binaria del primer instante cuyo reloj local ya pasó el inicio.
	low, high := earliest, latest
	if !wallClock(high, location).After(target) {
		high = latest.Add(3 * time.Hour)
	}
	for high.Sub(low) > time.Second {
		middle := low.Add(high.Sub(low) / 2)
		if wallClock(middle, location).Before(target) {
			low = middle
		} else {
			high = middle
		}
	}
	return high.Truncate(time.Second)
}
