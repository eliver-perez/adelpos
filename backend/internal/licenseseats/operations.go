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
	// SessionEnd: cerrar sesión o cerrar sesiones de otros (revocar una sesión o la terminal, cambiar la contraseña). Al
	// quedarse la terminal sin sesiones vigentes se libera su comandero, y al cerrarse la sesión que entró a
	// Preparación, su cupo de cocina: los dos cuentan solo con sesiones vigentes, y después de la operación se borran
	// las filas que ya no cuentan (Manager.Sweep). Cualquier otro cierre de sesiones (desactivar al usuario, cambiar su
	// PIN, una sesión vencida) libera igual en el conteo; la fila se borra en la siguiente asignación.
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
	case method == "POST" && (route == "auth/logout" || route == "auth/password"):
		return SessionEnd
	case method == "POST" && len(parts) == 3 && (parts[0] == "sessions" || parts[0] == "devices") && parts[2] == "revoke":
		return SessionEnd
	case method == "POST" && len(parts) == 4 && parts[0] == "devices" && parts[2] == "seat" && parts[3] == "release":
		return DeviceRelease
	}
	return None
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
