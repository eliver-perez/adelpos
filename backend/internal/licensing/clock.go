package licensing

import (
	"strconv"
	"time"
)

// Protección de la hora del equipo (§6 de la especificación; decisiones L1-D1, L1-D5, L1-D14, L1-D15 y L1-D16).
//
// La regla: la hora de la computadora principal no puede estar antes de la última actividad del sistema. La «última
// hora confiable» de la §5.6 del contrato la guarda el gestor (manager.go, en identity/licencia/estado.json) y se
// alimenta del `server_time`, del `issued_at` de las licencias aceptadas, del reloj local cada minuto y de cada
// operación con fecha que acepta la puerta (TimeFromOperation). Solo sube; la bajan únicamente la comprobación con
// internet (§6.3) y el desbloqueo de soporte (§6.4).
//
// Con reloj_local < última_hora_confiable − ClockMargin, el sistema entra en «Hora del equipo incorrecta»: se bloquea
// todo lo que registra algo con fecha, también cobrar (L1-D1: la hora no es uno de los seis estados de la licencia; es
// una protección local que el contrato deja a AD-EL POS). Si el reloj se corrige y queda dentro del margen, el bloqueo
// se quita solo: no hay nada que guardar, porque cada revisión vuelve a comparar las dos horas.
//
// Se revisa al arrancar, cada minuto (el vigilante de administration/license_clock.go) y en cada operación protegida
// (la puerta), solo en el servidor Go principal: las terminales y Android usan la hora del servidor.

// ClockMargin es el margen de la §6.1 (L1-D16): 10 minutos, por los ajustes normales del reloj y el reposo del equipo.
const ClockMargin = 10 * time.Minute

// ClockStatus es una revisión de la hora del equipo.
type ClockStatus struct {
	// Blocked: «Hora del equipo incorrecta».
	Blocked bool
	// LocalTime es el reloj de la computadora principal en el momento de la revisión.
	LocalTime time.Time
	// LastActivity es la última hora confiable (cero si todavía no hay ninguna: entonces nunca se bloquea).
	LastActivity time.Time
	// Margin es ClockMargin.
	Margin time.Duration
}

// CheckClock aplica la regla de la §6.2 a un reloj local y una última hora confiable.
func CheckClock(local, lastActivity time.Time) ClockStatus {
	status := ClockStatus{LocalTime: local.UTC(), Margin: ClockMargin}
	if lastActivity.IsZero() {
		return status
	}
	status.LastActivity = lastActivity.UTC()
	status.Blocked = local.Before(lastActivity.Add(-ClockMargin))
	return status
}

// Clock revisa la hora del equipo con el reloj del gestor (el mismo reloj simulado de las pruebas, L1-D16) y su
// última hora confiable, leídos juntos.
func (manager *Manager) Clock() ClockStatus {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	trusted := time.Time{}
	if manager.state.TrustedTime != nil {
		trusted = *manager.state.TrustedTime
	}
	return CheckClock(manager.now(), trusted)
}

// ServerTimeMatches dice si la hora del servidor de licencias y el reloj local coinciden dentro del margen (§6.3):
// entonces el reloj está bien y una última hora confiable posterior estaba en el futuro.
func ServerTimeMatches(local, server time.Time) bool {
	difference := local.Sub(server)
	if difference < 0 {
		difference = -difference
	}
	return difference <= ClockMargin
}

var screenWeekdays = []string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}

var screenMonths = []string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

// ScreenTime es una fecha completa para la pantalla de la hora (§6.2 de la especificación), en la zona horaria del
// restaurante: «lunes 23 nov 2026, 10:15». Sin zona válida, la hora se muestra en UTC.
func ScreenTime(instant time.Time, location *time.Location) string {
	if instant.IsZero() {
		return ""
	}
	if location == nil {
		location = time.UTC
	}
	local := instant.In(location)
	return screenWeekdays[local.Weekday()] + " " + strconv.Itoa(local.Day()) + " " + screenMonths[local.Month()-1] + " " +
		strconv.Itoa(local.Year()) + ", " + local.Format("15:04")
}
