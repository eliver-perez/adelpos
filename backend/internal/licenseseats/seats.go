// Package licenseseats aplica los cupos de sesiones simultáneas de la licencia (etapa L1, Fase D; §5 de la
// especificación y §7 del contrato de licencias): cajas (cash_registers), comanderos (order_takers) y pantallas de
// cocina (kitchen_displays). Go es la única autoridad: las terminales y Android solo muestran lo que responde.
//
// Reglas (decisiones L1-D2, L1-D3, L1-D7, L1-D13 y L1-D16 del coordinador):
//
//   - Caja, por terminal (L1-D3): la terminal que abre el turno ocupa un cupo, y la que captura con un turno activo en
//     su caja (aunque lo abriera otra) ocupa el suyo en su primera captura. Se liberan todos al cerrar el turno, también
//     desde otra computadora. Cobrar y las operaciones de efectivo nunca piden cupo.
//   - Comandero, por terminal (L1-D2): varios meseros con su PIN en una tableta ocupan un cupo. Se ocupa al primer
//     intento de crear o modificar un pedido sin turno activo en su caja, también con la credencial de envío de una
//     tableta sin sesión; las previas solo renuevan la actividad. Cuenta por actividad: mientras su última captura
//     tenga menos de 15 minutos (OrderTakerIdle). Se borra al cerrarse la última sesión de esa terminal (cerrar sesión,
//     revocar, vencer, «Liberar cupo») y al abrir turno en su caja (revisión R3, R3-1). Volver a «Selecciona tu
//     usuario» no cierra ninguna sesión, así que no lo libera.
//   - Cocina (L1-D7): la terminal en Preparación (POST kitchen/display-mode), hasta que sale o se cierra la sesión que
//     entró. La vigencia de esa sesión se compara con el reloj de las sesiones (Options.SessionNow), el mismo con que
//     se escribe y se valida, no con el reloj de los cupos (R3-2).
//   - Asignación atómica: contar y ocupar en la misma transacción, con el lock consultivo 7820148311 (LockKey). La hora
//     es la de Go (Options.Now), nunca now() de PostgreSQL, y se lee ya con el lock tomado.
//   - Licencia nueva con menos cupos: quien ya tiene su cupo lo conserva; no se ocupan cupos nuevos hasta quedar dentro
//     del límite.
//
// Qué operación ocupa, renueva o libera cada cupo lo dice una sola función, ForOperation (operations.go). El servidor
// la aplica en un solo punto, administration.Service.dispatch, después de autenticar y en la misma transacción.
package licenseseats

import (
	"errors"
	"fmt"
	"time"
)

// Seat es uno de los tres cupos, con el nombre que lleva en `limits` de la licencia (contrato §4 y §7) y en la API.
type Seat string

const (
	CashRegisters   Seat = "cash_registers"
	OrderTakers     Seat = "order_takers"
	KitchenDisplays Seat = "kitchen_displays"
)

// Seats son los tres cupos en el orden en que se informan.
var Seats = []Seat{CashRegisters, OrderTakers, KitchenDisplays}

// Tipos de fila de app.license_seats (migración 0028, sección «Fase D»): uno por cupo.
const (
	kindCashRegister   = "cash_register"
	kindOrderTaker     = "order_taker"
	kindKitchenDisplay = "kitchen_display"
)

// kind es el tipo de fila del cupo.
func (seat Seat) kind() string {
	switch seat {
	case CashRegisters:
		return kindCashRegister
	case OrderTakers:
		return kindOrderTaker
	}
	return kindKitchenDisplay
}

// seatOfKind es el cupo de un tipo de fila.
func seatOfKind(kind string) Seat {
	switch kind {
	case kindCashRegister:
		return CashRegisters
	case kindOrderTaker:
		return OrderTakers
	}
	return KitchenDisplays
}

// Limits son los cupos de la licencia vigente (contrato §4: enteros de 0 a 999; en V1 no existe «sin límite»).
type Limits struct {
	CashRegisters   int
	OrderTakers     int
	KitchenDisplays int
}

// Of es el límite de un cupo.
func (limits Limits) Of(seat Seat) int {
	switch seat {
	case CashRegisters:
		return limits.CashRegisters
	case OrderTakers:
		return limits.OrderTakers
	}
	return limits.KitchenDisplays
}

// LimitsSource entrega los cupos de la licencia vigente. Los límites todavía no existen en esta fase: la Fase C los
// conecta al gestor de licencias (internal/licensing) al fusionar con la Fase A (ver main.go).
//
// SeatLimits devuelve los cupos y true, o false si no hay ninguna licencia válida («Sin activar» o «Sin activar con
// datos»). Sin licencia válida no se ocupa ningún cupo: nunca hay un valor «sin límite».
type LimitsSource interface {
	SeatLimits() (Limits, bool)
}

// LimitsFunc adapta una función a LimitsSource, como http.HandlerFunc.
type LimitsFunc func() (Limits, bool)

// SeatLimits llama a la función.
func (function LimitsFunc) SeatLimits() (Limits, bool) {
	return function()
}

// LockKey es el lock consultivo de los cupos: de transacción y exclusivo. Es el último que toma una operación: después
// de 7820148305 y de cualquier otro lock consultivo que haya tomado su manejador (7820148304, 7820148308 y 7820148306, el
// de los eventos). Con él tomado nunca se publica un evento ni se pide otro lock (revisión R3, R3-3). Las rutas que solo
// tocan cupos (modo Cocina y liberar desde Terminales) lo toman solo. Ver CLAUDE.md §3.
const LockKey = 7820148311

// OrderTakerIdle es cuánto dura el cupo de comandero sin actividad de captura (§5 de la especificación: 15 minutos). Es
// un valor del backend, no de la interfaz; Options.OrderTakerIdle lo sustituye solo en las pruebas.
const OrderTakerIdle = 15 * time.Minute

// Options es lo que necesita el gestor de cupos.
type Options struct {
	// Limits son los cupos de la licencia. Sin valor, no hay licencia válida y no se ocupa ningún cupo (falla cerrada).
	Limits LimitsSource
	// Now es el reloj de Go (L1-D16). Sin valor, time.Now. La Fase C le pasa el reloj local del gestor de licencias
	// (licensing.Manager.LocalClock) y las pruebas uno simulado. Con él se escribe y se compara la actividad.
	Now func() time.Time
	// SessionNow es el reloj de las sesiones: el mismo con que administration escribe y valida su vencimiento
	// (issueSession y authenticateSession; hoy time.Now). Sin valor, time.Now. Con él se decide si sigue vigente la sesión
	// de una pantalla de cocina, para que un desfase del reloj de los cupos no la dé por vencida (revisión R3, R3-2).
	SessionNow func() time.Time
	// ClockTrusted dice si la hora de Go (Now) es confiable en este momento: la protección de la hora de la Fase C no
	// está en «Hora del equipo incorrecta». Sin valor, siempre lo es. Solo con la hora confiable se baja a «ahora» una
	// actividad de comandero registrada en el futuro (hallazgo R2-2, L1-D43): con el reloj atrasado, «ahora» es la hora
	// equivocada y bajarla liberaría antes de tiempo el cupo de una terminal que sí está capturando.
	ClockTrusted func() bool
	// OrderTakerIdle sustituye los 15 minutos del comandero (solo las pruebas). Sin valor, OrderTakerIdle.
	OrderTakerIdle time.Duration
}

// Manager aplica los cupos dentro de la transacción de cada petición (store.go). Es seguro para uso concurrente: no
// guarda estado propio, todo está en app.license_seats.
type Manager struct {
	limits       LimitsSource
	now          func() time.Time
	sessionNow   func() time.Time
	clockTrusted func() bool
	idle         time.Duration
}

// NewManager prepara el gestor de cupos.
func NewManager(options Options) *Manager {
	manager := &Manager{limits: options.Limits, now: options.Now, sessionNow: options.SessionNow,
		clockTrusted: options.ClockTrusted, idle: options.OrderTakerIdle}
	if manager.now == nil {
		manager.now = time.Now
	}
	if manager.sessionNow == nil {
		manager.sessionNow = time.Now
	}
	if manager.clockTrusted == nil {
		manager.clockTrusted = func() bool { return true }
	}
	if manager.idle <= 0 {
		manager.idle = OrderTakerIdle
	}
	return manager
}

// Limits son los cupos de la licencia vigente y si hay licencia válida (sin fuente, no la hay).
func (manager *Manager) Limits() (Limits, bool) {
	if manager.limits == nil {
		return Limits{}, false
	}
	limits, valid := manager.limits.SeatLimits()
	if !valid {
		return Limits{}, false
	}
	return limits, true
}

// Window es la hora de Go de los cupos y el corte de inactividad del comandero: una fila de comandero con actividad en
// ese instante o antes ya no cuenta.
func (manager *Manager) Window() (now, cutoff time.Time) {
	now, cutoff, _ = manager.times()
	return now, cutoff
}

// SessionNow es la hora de las sesiones (Options.SessionNow): una pantalla de cocina cuenta mientras su sesión venza
// después (revisión R3, R3-2).
func (manager *Manager) SessionNow() time.Time {
	return manager.sessionNow()
}

// ClockTrusted dice si la hora de Go es confiable en este momento (Options.ClockTrusted): solo entonces se baja a
// «ahora» una actividad de comandero registrada en el futuro (hallazgo R2-2, L1-D43).
func (manager *Manager) ClockTrusted() bool {
	return manager.clockTrusted()
}

// times es la hora de Go, el corte del comandero y la hora de las sesiones.
func (manager *Manager) times() (now, cutoff, sessionNow time.Time) {
	now = manager.now()
	return now, now.Add(-manager.idle), manager.sessionNow()
}

// ErrNoValidLicense: no hay ninguna licencia válida, así que no se ocupa ningún cupo. La puerta de la Fase C rechaza
// antes esas operaciones con su propio código (LICENSE_NOT_ACTIVATED); esto es la protección de respaldo.
var ErrNoValidLicense = errors.New("no hay una licencia válida")

// ErrSessionClosed: la sesión que pide entrar a Preparación ya no está vigente (se cerró mientras llegaba la petición).
// Su cupo de cocina no contaría, así que no se ocupa.
var ErrSessionClosed = errors.New("la sesión ya no está vigente")

// UnavailableError es el rechazo por falta de cupo: el cupo, cuántos están en uso (sin contar a quien pide) y el límite
// de la licencia. La API lo publica como 409 LICENSE_SEAT_UNAVAILABLE con details {seat, in_use, limit}.
type UnavailableError struct {
	Seat  Seat
	InUse int
	Limit int
}

func (problem *UnavailableError) Error() string {
	return problem.Message()
}

// Message es el texto en español sencillo para el usuario (§5 de la especificación): qué cupo falta, cuántos usa la
// licencia y qué hacer. Lo muestran tal cual los clientes anteriores a la etapa; los nuevos lo arman con details.
func (problem *UnavailableError) Message() string {
	if problem.Limit == 0 {
		switch problem.Seat {
		case CashRegisters:
			return "La licencia no incluye cajas (0 de 0). Avisa al encargado."
		case OrderTakers:
			return "La licencia no incluye comanderos (0 de 0): solo se capturan pedidos en una terminal con el turno de " +
				"caja abierto."
		}
		return "La licencia no incluye pantallas de cocina (0 de 0): Preparación no está disponible."
	}
	usage := fmt.Sprintf("(%d de %d en uso)", problem.InUse, problem.Limit)
	switch problem.Seat {
	case CashRegisters:
		return "No hay cupo de caja libre " + usage + ". Se libera al cerrar un turno de caja."
	case OrderTakers:
		return "No hay cupo de comandero libre " + usage + ". Pide que se cierre la sesión de otra terminal."
	}
	return "No hay cupo de pantalla de cocina libre " + usage + ". Pide que otra terminal salga de Preparación."
}

// Usage es un cupo con cuántos hay en uso y el límite de la licencia (0 sin licencia válida).
type Usage struct {
	InUse int `json:"in_use"`
	Limit int `json:"limit"`
}
