package administration

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/licensing"
)

// La puerta de la licencia y de la hora (etapa L1, Fase C; §4 y §6 de la especificación, decisiones L1-D1, L1-D6,
// L1-D12, L1-D14 y L1-D17).
//
// Toda petición pasa por Service.dispatch, y dispatch llama a licenseGate en dos lugares: antes de las rutas públicas
// (asistente, alta de la propia computadora y emparejamiento) y, para todas las demás, justo después de autenticar la
// terminal (credencial del dispositivo y su tipo de cliente). Así ninguna operación se escapa: tampoco la entrada con
// PIN, la credencial de envío del mesero, el agente de impresión ni los avisos de platillos listos. La clase de cada
// operación es licensing.ClassifyOperation (internal/licensing/operations.go), la única tabla.
//
// Qué aplica, en este orden:
//
//  1. **La hora** (§6.2): una operación que registra algo con fecha se rechaza con SYSTEM_CLOCK_INVALID mientras el
//     reloj de la computadora principal esté más de 10 minutos antes de la última hora confiable. También cobrar
//     (L1-D1). El bloqueo se quita solo en cuanto el reloj se corrige. Va primero: corregir la licencia no resolvería
//     el problema de la hora, y la respuesta dice además el estado de la licencia para que la pantalla muestre los dos
//     problemas por separado.
//  2. **El estado de la licencia** (tabla de la §6): LICENSE_NOT_ACTIVATED, LICENSE_EXPIRED o LICENSE_REVOKED, con sus
//     details. Abrir turno en Vencida o Revocada solo procede si hay cuentas pendientes de cobro (L1-D6).
//  3. **El cupo** no se decide aquí: lo aplica la Fase D en la misma transacción, después de autenticar la sesión y de
//     que el manejador haga su trabajo (LICENSE_SEAT_UNAVAILABLE). La puerta solo deja pasar lo que el estado permite.
//
// Las reglas firmes del contrato salen de la tabla: ningún estado borra datos (la puerta solo rechaza antes de
// escribir), ningún estado impide cobrar lo abierto en una instalación activada (RowCaptured se permite en Vencida y
// Revocada) y ninguno bloquea reportes, historial ni respaldos (RowRecords se permite siempre, también con la hora
// incorrecta). Activar, transferir, renovar, importar y liberar la licencia (RowLicense) siempre se permiten.
//
// Los rechazos son 409, para que no entren en la auditoría de accesos denegados de los 403, y llevan un mensaje en
// español sencillo para los clientes anteriores a la etapa; los nuevos deciden por el código y sus details.
//
// Sin gestor de licencias (Service.License nil: solo las pruebas que no tratan de la licencia) la puerta no aplica
// nada. main.go lo crea siempre, en serve y en el host de macOS y de Windows, antes de atender peticiones.

// SeatClassifier dice qué cupos puede ocupar una operación de la API (cash_registers, order_takers o
// kitchen_displays). Es la clasificación de la Fase D (licenseseats.ForOperation(method, route).Occupies()): la puerta
// no la repite. Mientras la Fase D no esté fusionada en esta rama no hay ninguna (seatsOfOperation devuelve nil) y la
// prueba de completitud solo revisa la tabla de la §6.
type SeatClassifier func(method, route string) []string

// seatsOfOperation es la clasificación de los cupos que usa la puerta. Al fusionar la Fase D se conecta a
// licenseseats.ForOperation(method, route).Occupies().
var seatsOfOperation SeatClassifier

// GateOperation es la fila completa de una operación: su clase en la tabla de la §6 y los cupos que puede ocupar.
type GateOperation struct {
	licensing.Operation
	Seats []string
}

// ClassifyGateOperation es la clase de una operación para la puerta y la prueba de completitud.
func ClassifyGateOperation(method, route string) GateOperation {
	operation := GateOperation{Operation: licensing.ClassifyOperation(method, route)}
	if seatsOfOperation != nil {
		operation.Seats = seatsOfOperation(method, strings.TrimPrefix(route, "/api/v1/"))
	}
	return operation
}

// publicLicenseRoute: las rutas que dispatch atiende antes de autenticar la terminal.
func publicLicenseRoute(route string) bool {
	return route == "setup/status" || route == "setup" || route == "devices/enroll" || strings.HasPrefix(route, "pairing/")
}

// licenseGate aplica la tabla de la §6 y la hora a una operación (route sin /api/v1/). Devuelve el rechazo o nil.
func (service *Service) licenseGate(ctx context.Context, tx pgx.Tx, request Request, route string) error {
	manager := service.License
	if manager == nil {
		return nil
	}
	// Sin identidad, el emparejamiento ya responde 503 PAIRING_UNAVAILABLE con la causa real (pairing.go): la puerta
	// no la tapa con el estado «Sin activar» que también resulta de no tener identidad.
	if strings.HasPrefix(route, "pairing/") && service.Identity == nil {
		return nil
	}
	operation := licensing.ClassifyOperation(request.Method, route)
	var clock licensing.ClockStatus
	if operation.Dated {
		clock = manager.Clock()
		if clock.Blocked {
			// El vigilante registra el bloqueo en la auditoría en cuanto puede (license_monitor.go), en su propia
			// transacción: esta petición no toma una segunda conexión de la base.
			service.wakeLicenseMonitor()
			return service.clockFault(ctx, tx, clock, manager.Status())
		}
	}
	status := manager.Status()
	switch operation.Row.Decide(status.State) {
	case licensing.Deny:
		return licenseStateFault(status, operation.Row, false)
	case licensing.AllowWithPendingAccounts:
		pending, err := pendingAccounts(ctx, tx)
		if err != nil {
			return err
		}
		if !pending {
			return licenseStateFault(status, operation.Row, true)
		}
	}
	// La hora de cada operación con fecha aceptada alimenta la última hora confiable (L1-D15). El reloj ya se
	// comprobó: está dentro del margen.
	if operation.Dated {
		manager.ObserveTime(clock.LocalTime, licensing.TimeFromOperation)
	}
	return nil
}

// pendingAccounts dice si hay cuentas pendientes de cobro: al menos una cuenta abierta con unidades activas (L1-D6).
// Es una lectura sin locks consultivos, antes de que el manejador tome 7820148305.
func pendingAccounts(ctx context.Context, tx pgx.Tx) (bool, error) {
	var pending bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT FROM app.restaurant_accounts account WHERE account.state = 'open'
        AND EXISTS (SELECT FROM app.restaurant_item_units unit WHERE unit.account_id = account.id AND unit.state = 'active'))`).
		Scan(&pending)
	return pending, err
}

// stateFaultSuffix es lo que sigue disponible con la licencia vencida o revocada (§7 de la especificación: «qué se
// puede hacer»).
const stateFaultSuffix = " Se puede seguir cobrando lo abierto, hacer cortes y consultar reportes. Avisa al encargado."

// licenseStateFault es el rechazo por el estado de la licencia (L1-D17). noPendingAccounts: abrir turno en Vencida o
// Revocada sin cuentas pendientes de cobro.
func licenseStateFault(status licensing.Status, row licensing.Row, noPendingAccounts bool) error {
	switch status.State {
	case licensing.StateExpired:
		message := "La licencia del sistema venció: esta operación ya no está disponible." + stateFaultSuffix
		if row == licensing.RowCapture {
			message = "La licencia del sistema venció: ya no se pueden capturar pedidos." + stateFaultSuffix
		}
		if noPendingAccounts {
			message = "La licencia del sistema venció: solo se puede abrir turno para cobrar cuentas pendientes, y no hay " +
				"ninguna. Avisa al encargado."
		}
		details := map[string]any{"operation": row, "expires_at": status.ExpiresAt, "grace_ended_at": status.GraceEndsAt}
		if row == licensing.RowOpenShift {
			details["pending_accounts"] = !noPendingAccounts
		}
		return &Fault{Status: 409, Code: "LICENSE_EXPIRED", Message: message, Details: details}
	case licensing.StateRevoked:
		cause := "La licencia del sistema fue revocada"
		switch status.RevocationReason {
		case licensing.ReasonDeactivated:
			cause = "La licencia se liberó en esta computadora"
		case licensing.ReasonTransferred:
			cause = "La licencia se transfirió a otra computadora"
		}
		message := cause + ": esta operación ya no está disponible." + stateFaultSuffix
		if row == licensing.RowCapture {
			message = cause + ": ya no se pueden capturar pedidos." + stateFaultSuffix
		}
		if noPendingAccounts {
			message = cause + ": solo se puede abrir turno para cobrar cuentas pendientes, y no hay ninguna. Avisa al encargado."
		}
		details := map[string]any{"operation": row, "reason": status.RevocationReason}
		if row == licensing.RowOpenShift {
			details["pending_accounts"] = !noPendingAccounts
		}
		return &Fault{Status: 409, Code: "LICENSE_REVOKED", Message: message, Details: details}
	}
	message := "El sistema no tiene una licencia activada: esta operación no está disponible. Avisa al encargado."
	switch {
	case status.Copied:
		message = "Esta instalación se copió de otra computadora. Actívala o transfiere la licencia."
	case status.MachineUnverified:
		message = "No se pudo comprobar esta computadora. Reinicia el servidor; si sigue igual, comunícate con soporte."
	}
	return &Fault{Status: 409, Code: "LICENSE_NOT_ACTIVATED", Message: message, Details: map[string]any{
		"operation":          row,
		"state":              notActivatedState(status.State),
		"copied":             status.Copied,
		"machine_unverified": status.MachineUnverified,
	}}
}

// notActivatedState es details.state: «not_activated» o «not_activated_with_data» (un estado inesperado se informa
// como «not_activated»).
func notActivatedState(state licensing.State) licensing.State {
	if state == licensing.StateNotActivatedWithData {
		return state
	}
	return licensing.StateNotActivated
}

// clockMessage es el mensaje de las terminales para la hora incorrecta (§6.2 de la especificación): explícito, nunca un
// error genérico, y sin confundirlo con la licencia, la conexión o los permisos.
const clockMessage = "No se puede registrar: la computadora principal tiene la fecha y hora incorrectas. Avisa al encargado."

// ClockDetails es la causa del bloqueo por hora, lista para la pantalla (§6.2): las dos fechas completas en la zona
// horaria del restaurante. Es el details de SYSTEM_CLOCK_INVALID y el `clock` del estado de la licencia.
type ClockDetails struct {
	Blocked bool `json:"blocked"`
	// LocalTime es la hora de la computadora principal y LastActivityTime la última actividad registrada (la última
	// hora confiable; nil si todavía no hay ninguna).
	LocalTime        time.Time  `json:"local_time"`
	LastActivityTime *time.Time `json:"last_activity_time"`
	MarginMinutes    int        `json:"margin_minutes"`
	// Timezone es la zona horaria del restaurante con que se escribieron los textos.
	Timezone             string `json:"timezone"`
	LocalTimeText        string `json:"local_time_text"`
	LastActivityTimeText string `json:"last_activity_time_text"`
	// LicenseState es el estado de la licencia en ese momento. LicenseOK: Vigente o Tolerancia; solo entonces la
	// pantalla dice «Tu licencia está bien. Este bloqueo es solo por la fecha y hora.».
	LicenseState licensing.State `json:"license_state"`
	LicenseOK    bool            `json:"license_ok"`
}

// restaurantLocation es la zona horaria del restaurante (UTC si todavía no hay establecimiento o no es válida).
func restaurantLocation(ctx context.Context, tx pgx.Tx) (*time.Location, string, error) {
	var name string
	if err := tx.QueryRow(ctx, "SELECT coalesce((SELECT timezone FROM app.establishment), '')").Scan(&name); err != nil {
		return time.UTC, "UTC", err
	}
	if name == "" {
		return time.UTC, "UTC", nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC, "UTC", nil
	}
	return location, name, nil
}

// clockDetails arma la causa del bloqueo (o el estado de la hora) con la zona del restaurante.
func clockDetails(clock licensing.ClockStatus, status licensing.Status, location *time.Location, timezone string) ClockDetails {
	details := ClockDetails{
		Blocked:       clock.Blocked,
		LocalTime:     clock.LocalTime,
		MarginMinutes: int(clock.Margin / time.Minute),
		Timezone:      timezone,
		LocalTimeText: licensing.ScreenTime(clock.LocalTime, location),
		LicenseState:  status.State,
		LicenseOK:     status.State == licensing.StateActive || status.State == licensing.StateGrace,
	}
	if !clock.LastActivity.IsZero() {
		lastActivity := clock.LastActivity
		details.LastActivityTime = &lastActivity
		details.LastActivityTimeText = licensing.ScreenTime(lastActivity, location)
	}
	return details
}

// clockFault es el rechazo por «Hora del equipo incorrecta»: SYSTEM_CLOCK_INVALID (no empieza con «LICENSE» para que
// nadie lo confunda con la licencia).
func (service *Service) clockFault(ctx context.Context, tx pgx.Tx, clock licensing.ClockStatus, status licensing.Status) error {
	location, timezone, err := restaurantLocation(ctx, tx)
	if err != nil {
		return err
	}
	return &Fault{Status: 409, Code: "SYSTEM_CLOCK_INVALID", Message: clockMessage,
		Details: clockDetails(clock, status, location, timezone)}
}

// LicenseSummary es el estado de la licencia que reciben las terminales en auth/me y restaurant/state (§6.2 y §7 de la
// especificación): con él el escritorio y Android muestran el aviso fijo de la licencia y el de la hora. Go es la única
// autoridad: los días y las fechas ya vienen calculados.
type LicenseSummary struct {
	State licensing.State `json:"state"`
	// Environment es el entorno de firma en que confía esta versión («test» en las de desarrollo).
	Environment       string  `json:"environment"`
	Copied            bool    `json:"copied"`
	MachineUnverified bool    `json:"machine_unverified"`
	RevocationReason  *string `json:"revocation_reason"`
	LicenseType       *string `json:"license_type"`
	Perpetual         bool    `json:"perpetual"`
	// ExpiresAt y GraceEndsAt son instantes exclusivos (nil en una perpetua o sin licencia).
	ExpiresAt   *time.Time `json:"expires_at"`
	GraceDays   int        `json:"grace_days"`
	GraceEndsAt *time.Time `json:"grace_ends_at"`
	// DaysUntilExpiry son los días que faltan para el vencimiento (solo en Vigente con vencimiento) y
	// GraceDaysRemaining los que quedan de tolerancia antes de que se bloquee la captura (solo en Tolerancia),
	// redondeados hacia arriba.
	DaysUntilExpiry    *int `json:"days_until_expiry"`
	GraceDaysRemaining *int `json:"grace_days_remaining"`
	// Clock es el estado de la hora del equipo, con las dos fechas para el aviso fijo.
	Clock ClockDetails `json:"clock"`
}

// daysUntil son los días que faltan hasta limit desde now, redondeados hacia arriba (nunca menos de cero).
func daysUntil(limit, now time.Time) int {
	if !limit.After(now) {
		return 0
	}
	return int(math.Ceil(limit.Sub(now).Hours() / 24))
}

// licenseSummary es el estado de la licencia para las terminales, o nil sin gestor.
func (service *Service) licenseSummary(ctx context.Context, tx pgx.Tx) (*LicenseSummary, error) {
	manager := service.License
	if manager == nil {
		return nil, nil
	}
	location, timezone, err := restaurantLocation(ctx, tx)
	if err != nil {
		return nil, err
	}
	status := manager.Status()
	summary := &LicenseSummary{
		State:             status.State,
		Environment:       status.Environment,
		Copied:            status.Copied,
		MachineUnverified: status.MachineUnverified,
		Perpetual:         status.Perpetual,
		ExpiresAt:         status.ExpiresAt,
		GraceDays:         status.GraceDays,
		GraceEndsAt:       status.GraceEndsAt,
		Clock:             clockDetails(manager.Clock(), status, location, timezone),
	}
	if status.RevocationReason != "" {
		reason := status.RevocationReason
		summary.RevocationReason = &reason
	}
	if status.LicenseType != "" {
		licenseType := status.LicenseType
		summary.LicenseType = &licenseType
	}
	switch {
	case status.State == licensing.StateActive && status.ExpiresAt != nil:
		days := daysUntil(*status.ExpiresAt, status.EffectiveNow)
		summary.DaysUntilExpiry = &days
	case status.State == licensing.StateGrace && status.GraceEndsAt != nil:
		days := daysUntil(*status.GraceEndsAt, status.EffectiveNow)
		summary.GraceDaysRemaining = &days
	}
	return summary, nil
}

// attachLicense agrega `license` (LicenseSummary) a la respuesta de auth/me o de restaurant/state. Sin gestor la
// respuesta queda igual.
func (service *Service) attachLicense(ctx context.Context, tx pgx.Tx) func(any, error) (any, error) {
	return func(result any, err error) (any, error) {
		if err != nil || service.License == nil {
			return result, err
		}
		response, isMap := result.(map[string]any)
		if !isMap {
			return result, err
		}
		summary, err := service.licenseSummary(ctx, tx)
		if err != nil {
			return nil, err
		}
		response["license"] = summary
		return response, nil
	}
}
