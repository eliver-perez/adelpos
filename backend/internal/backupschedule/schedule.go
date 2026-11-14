// Package backupschedule hace los respaldos programados del servidor (etapa 9G, fase F; §7 de la especificación): un
// respaldo automático al día a la hora configurada (por omisión, la «Hora de inicio del día»), en la carpeta elegida
// (otro disco o una USB) o, si no está disponible, en la predeterminada; cada respaldo se verifica con pg_restore
// --list y solo se conservan los últimos N automáticos. «Respaldar ahora» (Sistema › Mantenimiento) usa el mismo
// camino. El respaldo en sí es el de la instalación (maintenance.ScheduledBackuper: pg_dump con serve y el de
// internal/standalone en el host de macOS y de Windows).
//
// Reglas de la programación:
//
//   - El instante programado de cada día es la hora configurada en la zona horaria del establecimiento. Con un cambio
//     de horario se comporta como la «Hora de inicio del día» (businessday.CutoffInstant): si la hora no existe ese
//     día (adelanto), es el primer instante después del salto; si ocurre dos veces (atraso), la primera, y la segunda
//     ya no repite el respaldo porque el del día ya se hizo.
//   - Toca respaldar cuando no hay un respaldo automático correcto que haya empezado en o después del último instante
//     programado. Así, si la computadora estaba apagada a esa hora, el respaldo se hace al arrancar (después de la
//     espera de arranque, StartupDelay), y si se cambia la hora a media jornada no se repite el respaldo de ese día.
//   - Un intento fallido se reintenta después de RetryDelay, como máximo MaxAttempts intentos por instante
//     programado (un intento interrumpido porque el servidor se detuvo se reintenta al volver, sin esperar); el aviso
//     al administrador sigue hasta que haya un respaldo correcto posterior.
//   - Un respaldo que empezó «en el futuro» (más de ClockTolerance; el reloj estaba adelantado) no cuenta como el del
//     día ni como el último, y una fila «en curso» de más de StaleAfter es un resto que el trabajador cierra: ninguno
//     detiene la programación. Si aun así no hay un respaldo correcto en más de OverdueAfter, se avisa («atrasado»).
//   - La retención y la limpieza de restos solo tocan los archivos que esta instalación registró en app.backup_runs.
//   - Solo hay un respaldo a la vez: el lock consultivo 7820148309 (pg_try_advisory_lock, de sesión) lo toma quien
//     respalda; si está ocupado, el automático espera a la siguiente revisión y «Respaldar ahora» responde que ya hay
//     uno en curso.
package backupschedule

import (
	"time"

	"local/ad-el-pos/server/internal/businessday"
)

// RetryDelay es la espera antes de reintentar un respaldo automático que falló.
const RetryDelay = time.Hour

// MaxAttempts es el número máximo de intentos automáticos por instante programado.
const MaxAttempts = 3

// localDate es la fecha local de un instante, a medianoche UTC, para contar días sin que el cambio de horario mueva
// la cuenta.
func localDate(instant time.Time, location *time.Location) time.Time {
	local := instant.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// slotOn es el instante programado de una fecha local.
func slotOn(date time.Time, location *time.Location, hour, minute int) time.Time {
	return businessday.CutoffInstant(date.Year(), date.Month(), date.Day(), hour, minute, location)
}

// LastSlot devuelve el último instante programado en o antes de now: la hora hour:minute del día local de now o, si
// todavía no llega, la del día anterior.
func LastSlot(now time.Time, location *time.Location, hour, minute int) time.Time {
	date := localDate(now, location)
	slot := slotOn(date, location, hour, minute)
	for slot.After(now) {
		date = date.AddDate(0, 0, -1)
		slot = slotOn(date, location, hour, minute)
	}
	return slot
}

// NextSlot devuelve el primer instante programado posterior a now.
func NextSlot(now time.Time, location *time.Location, hour, minute int) time.Time {
	date := localDate(now, location)
	slot := slotOn(date, location, hour, minute)
	for !slot.After(now) {
		date = date.AddDate(0, 0, 1)
		slot = slotOn(date, location, hour, minute)
	}
	return slot
}

// SlotHistory resume los respaldos automáticos que empezaron en o después de un instante programado.
type SlotHistory struct {
	// Succeeded es cuántos terminaron correctos (verificados).
	Succeeded int
	// Attempts es cuántos terminaron, correctos o no.
	Attempts int
	// LastFinished es cuándo terminó el último intento; nil sin intentos.
	LastFinished *time.Time
	// Running indica que hay uno en curso.
	Running bool
}

// Due decide si toca respaldar con el historial del último instante programado: no hay un respaldo correcto ni uno
// en curso, quedan intentos y, si el último falló, ya pasó la espera para reintentar.
func Due(now time.Time, history SlotHistory) bool {
	if history.Succeeded > 0 || history.Running {
		return false
	}
	if history.Attempts >= MaxAttempts {
		return false
	}
	if history.LastFinished != nil && now.Sub(*history.LastFinished) < RetryDelay {
		return false
	}
	return true
}
