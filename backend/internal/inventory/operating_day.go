package inventory

import (
	"time"
)

// Día operativo (§0-B «Cancelaciones sin clasificar»). Se define con la hora de corte del
// establecimiento (inventory_settings.operating_day_cutoff, 05:00 por omisión) en su zona horaria
// (establishment.timezone) y no depende de los turnos de caja: un instante pertenece al día cuya
// fecha local tiene la hora de corte en o antes de él, hasta el corte del día siguiente. Una
// cancelación a las 02:00 del 11 con corte a las 05:00 pertenece al día operativo del 10 y vence
// a las 05:00 del 11.
//
// Cambio de horario: si la hora de corte no existe ese día (adelanto), el corte es el primer
// instante posterior al salto; si ocurre dos veces (atraso), es la primera vez. El vencimiento
// siempre es posterior al instante de la cancelación.

// OperatingDay es el día operativo de un instante y el corte que lo cierra.
type OperatingDay struct {
	Date     time.Time
	Deadline time.Time
}

// operatingDayOf calcula el día operativo de un instante con la hora de corte hour:minute.
func operatingDayOf(instant time.Time, location *time.Location, hour, minute int) OperatingDay {
	local := instant.In(location)
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	if local.Hour() < hour || (local.Hour() == hour && local.Minute() < minute) {
		date = date.AddDate(0, 0, -1)
	}
	next := date.AddDate(0, 0, 1)
	deadline := cutoffInstant(next.Year(), next.Month(), next.Day(), hour, minute, location)
	// En un atraso de horario la hora local puede repetirse: si el corte (su primera aparición) ya
	// ocurrió antes del instante, el instante pertenece al día siguiente.
	for !deadline.After(instant) {
		date = next
		next = date.AddDate(0, 0, 1)
		deadline = cutoffInstant(next.Year(), next.Month(), next.Day(), hour, minute, location)
	}
	return OperatingDay{Date: date, Deadline: deadline}
}

// wallClock expresa la hora local de un instante como si fuera UTC, para comparar relojes.
func wallClock(instant time.Time, location *time.Location) time.Time {
	local := instant.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(),
		time.UTC)
}

// cutoffInstant devuelve el primer instante en que el reloj local marca la hora de corte en esa
// fecha o, si esa hora no existe por un adelanto de horario, el primer instante posterior al salto.
func cutoffInstant(year int, month time.Month, day, hour, minute int, location *time.Location) time.Time {
	target := time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
	// Las desviaciones posibles alrededor de la fecha (antes y después de un cambio de horario).
	offsets := map[int]bool{}
	for _, probe := range []time.Duration{-36 * time.Hour, 0, 36 * time.Hour} {
		_, offset := target.Add(probe).In(location).Zone()
		offsets[offset] = true
	}
	// Cada desviación da un candidato; los que marcan exactamente la hora de corte son válidos y se
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
	// La hora no existe: búsqueda binaria del primer instante cuyo reloj local ya pasó el corte.
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
