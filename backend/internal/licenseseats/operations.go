package licenseseats

import "strings"

// Effect es lo que una operación de la API hace con los cupos. Lo calcula ForOperation; administration.Service.dispatch
// lo aplica en un solo punto y la puerta de la Fase C lo usa en su prueba de completitud.
type Effect string

const (
	// None: la operación no ocupa, no renueva ni libera ningún cupo. Es la de casi todas las rutas: lecturas, cobros,
	// operaciones de efectivo, cancelaciones, reorganizar cuentas, cocina (salvo el modo Cocina), catálogo,
	// inventario, reportes y administración.
	None Effect = "none"
	// Capture: crear o modificar un pedido (abrir una cuenta, guardar el borrador, enviar a cocina, modificar lo
	// enviado y el pedido capturado sin conexión). Si la caja de la terminal tiene un turno activo, ocupa el cupo de
	// caja de la terminal (L1-D3) y libera su comandero; si no, ocupa el cupo de comandero, o renueva su actividad si ya
	// lo tiene (L1-D13). Sin cupo, la operación se rechaza y no se guarda nada.
	Capture Effect = "capture"
	// Preview: las previas de la captura (draft-preview y edit-preview: entrar a la mesa para capturar). Solo renuevan
	// la actividad del comandero si la terminal ya lo tiene; nunca ocupan ni rechazan.
	Preview Effect = "preview"
	// OpenShift: abrir turno. La terminal ocupa el cupo de caja del turno nuevo (sin cupo no se abre) y se liberan los
	// comanderos de las terminales de esa caja, que desde ahí capturan como caja.
	OpenShift Effect = "open_shift"
	// CloseShift: confirmar el arqueo de cierre. Libera los cupos de caja de todas las terminales que usaban ese turno,
	// también desde otra computadora. No escribe: el cupo cuenta solo mientras su turno está activo.
	CloseShift Effect = "close_shift"
	// KitchenDisplay: POST kitchen/display-mode {active}. Ocupa el cupo de cocina al entrar a Preparación y lo libera al
	// salir. Lo hace la propia ruta (administration/license_seats.go).
	KitchenDisplay Effect = "kitchen_display"
	// SessionEnd: las operaciones que cierran sesiones (cerrar sesión, revocar una sesión o la terminal, cambiar la
	// contraseña o el PIN, desactivar al usuario, crear una identidad nueva). Después de la operación se borra el
	// comandero de cada terminal que se quedó sin ninguna sesión vigente (ReleaseClosedSessions, revisión R3, R3-1); al
	// cerrarse la sesión que entró a Preparación, su cupo de cocina deja de contar. Una sesión vencida libera igual
	// (authenticateSession, al encontrarla vencida).
	SessionEnd Effect = "session_end"
	// DeviceRelease: POST devices/{id}/seat/release. Libera el comandero y la cocina de esa terminal y cierra sus
	// sesiones. Lo hace la propia ruta. Las cajas se liberan cerrando el turno.
	DeviceRelease Effect = "device_release"
)

// Effects son todos los efectos, para las pruebas y la documentación.
var Effects = []Effect{None, Capture, Preview, OpenShift, CloseShift, KitchenDisplay, SessionEnd, DeviceRelease}

// Occupies son los cupos que la operación puede ocupar (y por los que puede rechazarse).
func (effect Effect) Occupies() []Seat {
	switch effect {
	case Capture:
		return []Seat{CashRegisters, OrderTakers}
	case OpenShift:
		return []Seat{CashRegisters}
	case KitchenDisplay:
		return []Seat{KitchenDisplays}
	}
	return nil
}

// Releases son los cupos que la operación puede liberar.
func (effect Effect) Releases() []Seat {
	switch effect {
	case Capture, OpenShift:
		return []Seat{OrderTakers}
	case CloseShift:
		return []Seat{CashRegisters}
	case KitchenDisplay:
		return []Seat{KitchenDisplays}
	case SessionEnd, DeviceRelease:
		return []Seat{OrderTakers, KitchenDisplays}
	}
	return nil
}

// Renews indica si la operación renueva la actividad del comandero que ya tiene la terminal.
func (effect Effect) Renews() bool {
	return effect == Capture || effect == Preview
}

// ForOperation clasifica una operación de la API por su método y su ruta (relativa a /api/v1, con o sin ese prefijo y
// con los identificadores reales). Es la única clasificación de los cupos: no se repite en los manejadores.
func ForOperation(method, route string) Effect {
	route = strings.Trim(strings.TrimPrefix(route, "/api/v1/"), "/")
	parts := strings.Split(route, "/")
	switch {
	case method == "POST" && route == "restaurant/accounts":
		return Capture
	case method == "POST" && route == "restaurant/offline-orders":
		return Capture
	case len(parts) == 4 && parts[0] == "restaurant" && parts[1] == "accounts":
		return accountEffect(method, parts[3])
	case method == "POST" && route == "payments/shifts":
		return OpenShift
	case method == "POST" && len(parts) == 5 && parts[0] == "payments" && parts[1] == "shifts" &&
		parts[3] == "closing" && parts[4] == "confirm":
		return CloseShift
	case method == "POST" && route == "kitchen/display-mode":
		return KitchenDisplay
	case method == "POST" && (route == "auth/logout" || route == "auth/password" || route == "license/identity/regenerate"):
		return SessionEnd
	case method == "PUT" && route == "auth/pin":
		return SessionEnd
	case method == "POST" && len(parts) == 3 && (parts[0] == "sessions" || parts[0] == "devices") && parts[2] == "revoke":
		return SessionEnd
	case parts[0] == "users" && len(parts) >= 2 && userSessionEnd(method, parts):
		return SessionEnd
	case method == "POST" && len(parts) == 4 && parts[0] == "devices" && parts[2] == "seat" && parts[3] == "release":
		return DeviceRelease
	}
	return None
}

// userSessionEnd dice si la acción sobre un usuario puede cerrar sus sesiones: editarlo (al desactivarlo), cambiar su
// PIN o quitárselo (cierra sus sesiones en los dispositivos de meseros).
func userSessionEnd(method string, parts []string) bool {
	switch {
	case method == "PATCH" && len(parts) == 2:
		return true
	case method == "PUT" && len(parts) == 3 && parts[2] == "pin":
		return true
	case method == "POST" && len(parts) == 4 && parts[2] == "pin" && parts[3] == "remove":
		return true
	}
	return false
}

// accountEffect clasifica las acciones sobre una cuenta (restaurant/accounts/{id}/<acción>).
func accountEffect(method, action string) Effect {
	switch method + " " + action {
	case "PUT draft", "POST confirm", "POST edit":
		return Capture
	case "POST draft-preview", "POST edit-preview":
		return Preview
	}
	return None
}
