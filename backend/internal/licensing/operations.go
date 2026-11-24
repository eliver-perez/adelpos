package licensing

import "strings"

// Tabla de operaciones de la §6 del contrato en el backend (decisiones L1-D6, L1-D12 y L1-D14; avance §0.5.7).
//
// Cada operación de la API v1 (método y ruta relativa a /api/v1, con los identificadores reales) pertenece a una
// fila de la tabla de la §6 o a una de las clases que la tabla no lista (L1-D6). La clase dice:
//
//   - en qué estados de la licencia se permite (Row.Decide);
//   - si registra algo con fecha (Dated): con «Hora del equipo incorrecta» se bloquea (§6.2 de la especificación,
//     L1-D1 y L1-D14);
//   - y si está en la tabla (Known). Una escritura que no está toma la clase más restrictiva (RowUnknownWrite).
//
// Los cupos no se deciden aquí: qué operación ocupa, renueva o libera cada cupo es la clasificación de la Fase D
// (internal/licenseseats). La puerta (administration/license_gate.go) solo decide el estado y la hora; el cupo se
// aplica después, en la misma transacción. Así ninguna de las dos clasificaciones repite a la otra.
//
// Esta es la única tabla: la puerta la consulta en cada operación protegida y la prueba de completitud
// (administration/license_gate_test.go) exige que toda operación del puente del escritorio, del contrato
// openapi.json y de las rutas locales esté en ella.

// Row es la fila de la tabla de la §6 (F1 a F8 del avance) o una de las clases que la tabla no lista.
type Row string

const (
	// RowCapture es F1, «Crear pedidos y agregar productos a cuentas abiertas»: abrir una cuenta, el borrador,
	// enviar a cocina, modificar lo enviado y el pedido capturado sin conexión. Solo en Vigente y Tolerancia, con
	// cupo.
	RowCapture Row = "capture"
	// RowCaptured es F2, «Cobrar, cancelar y aplicar descuentos a lo ya capturado», con reorganizar cuentas
	// (dividir, juntar, mover, transferir productos, asignar mesero y datos del cliente; L1-D6), las precuentas y
	// los tickets. También en Vencida y Revocada: ningún estado impide cobrar lo abierto en una instalación
	// activada.
	RowCaptured Row = "captured"
	// RowOpenShift es F3, «Abrir turno de caja». En Vencida y Revocada, solo si hay cuentas pendientes de cobro
	// (L1-D6), y siempre con cupo.
	RowOpenShift Row = "open_shift"
	// RowCloseShift es F4, «Cerrar turno y hacer el corte», con las operaciones de efectivo del turno (ingresos,
	// retiros, gastos, correcciones, cortes parciales, tipo de cambio y abrir el cajón).
	RowCloseShift Row = "close_shift"
	// RowKitchen es F5, «KDS: mostrar pedidos ya enviados»: entrar en modo Cocina (kitchen/display-mode, con cupo),
	// avanzar estados, liberar lo programado y reintentar la impresión de cocina (L1-D6). Las lecturas de cocina,
	// también el tablero, son F6.
	RowKitchen Row = "kitchen"
	// RowRecords es F6, «Reportes, historial y respaldos», con toda lectura y las vistas previas que solo calculan.
	// Se permite en todos los estados: ningún estado bloquea reportes, historial ni respaldos.
	RowRecords Row = "records"
	// RowAdministration es F7, «Administración (usuarios, catálogo, configuración)», con el inventario y emparejar
	// terminales (L1-D6). No se permite sin activar.
	RowAdministration Row = "administration"
	// RowLicense es F8, «Activar, transferir, renovar o importar un .lic», con liberar la licencia, «Comprobar la
	// hora con internet» y el desbloqueo de soporte (L1-D6). Siempre se permite.
	RowLicense Row = "license"
	// RowAlways son las operaciones que la tabla no lista y siempre se permiten (L1-D6): la sesión (auth/*), las
	// preferencias, «Reiniciar», liberar un cupo, cerrar una sesión, el canal de eventos, los avisos de platillos
	// listos y el agente de impresión (que solo entrega trabajos ya registrados).
	RowAlways Row = "always"
	// RowStartup es el arranque (L1-D6): el asistente inicial y el alta de la propia computadora por la escucha
	// local. Sin ellos no se podría iniciar sesión para activar; sus propias reglas (solo sin establecimiento y solo
	// local) siguen en sus manejadores.
	RowStartup Row = "startup"
	// RowUnknownWrite es una escritura que no está en la tabla (L1-D12): la clase más restrictiva, que solo se
	// permite en Vigente y Tolerancia y registra con fecha. La prueba de completitud impide que una ruta conocida
	// caiga aquí.
	RowUnknownWrite Row = "unknown_write"
)

// Rows son todas las filas y clases, en el orden de la tabla (para la documentación y las pruebas).
var Rows = []Row{
	RowCapture, RowCaptured, RowOpenShift, RowCloseShift, RowKitchen, RowRecords, RowAdministration, RowLicense,
	RowAlways, RowStartup, RowUnknownWrite,
}

// Decision es lo que la tabla de la §6 dice de una fila en un estado.
type Decision string

const (
	// Allow: se permite (con los cupos que pida la operación, que aplica la Fase D).
	Allow Decision = "allow"
	// Deny: no se permite en este estado.
	Deny Decision = "deny"
	// AllowWithPendingAccounts: abrir turno en Vencida o Revocada, solo si hay al menos una cuenta abierta con
	// unidades activas (L1-D6). Lo comprueba la puerta con la base.
	AllowWithPendingAccounts Decision = "allow_with_pending_accounts"
)

// Decide es la celda de la tabla de la §6 para esta fila y el estado de la licencia. Las celdas «—» del contrato
// (Sin activar: no hay nada capturado) se tratan como «No», salvo los reportes, el historial y los respaldos, que
// ningún estado bloquea.
func (row Row) Decide(state State) Decision {
	switch row {
	case RowRecords, RowLicense, RowAlways, RowStartup:
		return Allow
	}
	switch state {
	case StateActive, StateGrace:
		return Allow
	case StateExpired, StateRevoked:
		switch row {
		case RowCaptured, RowCloseShift, RowKitchen, RowAdministration:
			return Allow
		case RowOpenShift:
			return AllowWithPendingAccounts
		}
		return Deny
	}
	// Sin activar y Sin activar con datos: solo lo de arriba.
	return Deny
}

// Operation es la clase de una operación de la API en la tabla de la §6.
type Operation struct {
	Row Row
	// Dated indica que la operación registra algo con fecha (capturar, cobrar, cancelar, turnos, cocina,
	// inventario y administración): con «Hora del equipo incorrecta» se bloquea (L1-D14). Las lecturas, los
	// reportes y sus exportaciones, los respaldos, la licencia, la sesión, las preferencias, «Reiniciar», el canal
	// de eventos y el agente de impresión siguen disponibles.
	Dated bool
	// Known indica que la operación está en la tabla. Falso es la regla de lo desconocido: una lectura cuenta como
	// reporte (RowRecords) y una escritura toma RowUnknownWrite.
	Known bool
}

func known(row Row, dated bool) Operation {
	return Operation{Row: row, Dated: dated, Known: true}
}

// unknownOperation es la regla de lo que no está en la tabla: leer no registra nada; escribir toma la clase más
// restrictiva (L1-D12).
func unknownOperation(method string) Operation {
	if readMethod(method) {
		return Operation{Row: RowRecords}
	}
	return Operation{Row: RowUnknownWrite, Dated: true}
}

func readMethod(method string) bool {
	return method == "GET" || method == "HEAD"
}

// ClassifyOperation es la clase de una operación por su método y su ruta (relativa a /api/v1, con o sin ese
// prefijo, con los identificadores reales y sin la consulta).
func ClassifyOperation(method, route string) Operation {
	route = strings.TrimPrefix(route, "/")
	route = strings.TrimPrefix(route, "api/v1/")
	route, _, _ = strings.Cut(route, "?")
	route = strings.Trim(route, "/")
	parts := strings.Split(route, "/")
	read := readMethod(method)
	switch parts[0] {
	case "version", "health", "openapi.json":
		// No pasan por la administración (httpapi/router.go); están en la tabla para la prueba de completitud.
		return known(RowAlways, false)
	case "setup":
		return classifySetup(method, parts)
	case "auth", "preferences", "terminal", "events", "ready-alerts", "print-agent":
		// La sesión, las preferencias de la persona y de la terminal, el canal de eventos (y su lectura por
		// cursor), los avisos de platillos listos y el agente de impresión de cada terminal (L1-D6 y L1-D14).
		return known(RowAlways, false)
	case "license":
		return known(RowLicense, false)
	case "pairing":
		// Emparejar terminales es administración (L1-D6: «activar la computadora principal antes de emparejar»).
		// La terminal que espera su aprobación solo consulta su propia solicitud.
		if read {
			return known(RowRecords, false)
		}
		return known(RowAdministration, true)
	case "devices":
		return classifyDevices(method, parts)
	case "sessions":
		if read {
			return known(RowRecords, false)
		}
		if method == "POST" && len(parts) == 3 && parts[2] == "revoke" {
			// Cerrar una sesión siempre se permite (avance §0.5.7): libera la terminal y su cupo.
			return known(RowAlways, false)
		}
		return unknownOperation(method)
	case "maintenance":
		return classifyMaintenance(method, parts)
	case "reports":
		// Todos los reportes, también las exportaciones: ningún estado los bloquea y no registran nada.
		return known(RowRecords, false)
	case "restaurant":
		return classifyRestaurant(method, parts)
	case "payments":
		return classifyPayments(method, parts)
	case "kitchen":
		return classifyKitchen(method, parts)
	case "catalog":
		// El catálogo es administración, también «Agotado». La vista previa del producto solo calcula.
		if read || (method == "POST" && len(parts) == 2 && parts[1] == "preview") {
			return known(RowRecords, false)
		}
		return known(RowAdministration, true)
	case "inventory":
		// El inventario es administración y registra con fecha (L1-D6).
		if read {
			return known(RowRecords, false)
		}
		return known(RowAdministration, true)
	case "documents":
		// Diseñador e identidad documental: administración; las vistas previas solo calculan.
		if read || (method == "POST" && parts[len(parts)-1] == "preview") {
			return known(RowRecords, false)
		}
		return known(RowAdministration, true)
	case "users", "roles", "permissions", "settings", "registers", "audit":
		if read {
			return known(RowRecords, false)
		}
		return known(RowAdministration, true)
	}
	return unknownOperation(method)
}

// classifySetup: el asistente inicial y su estado (arranque, L1-D6).
func classifySetup(method string, parts []string) Operation {
	switch {
	case len(parts) == 2 && parts[1] == "status" && readMethod(method):
		return known(RowStartup, false)
	case len(parts) == 1 && method == "POST":
		// Solo sin establecimiento (lo comprueba setup.go). No registra operaciones del negocio y, sin él, no se
		// llega a la pantalla de la licencia ni a la de la hora.
		return known(RowStartup, false)
	}
	return unknownOperation(method)
}

// classifyDevices: Terminales.
func classifyDevices(method string, parts []string) Operation {
	if readMethod(method) {
		return known(RowRecords, false)
	}
	switch {
	case method == "POST" && len(parts) == 2 && parts[1] == "enroll":
		// El alta de la propia computadora por la escucha local (arranque, L1-D6). Sin ella la terminal de la
		// computadora principal no podría iniciar sesión, ni siquiera para ver la pantalla de la hora.
		return known(RowStartup, false)
	case method == "POST" && len(parts) == 4 && parts[2] == "seat" && parts[3] == "release":
		// «Liberar cupo» siempre se permite (L1-D6): solo cierra sesiones y libera.
		return known(RowAlways, false)
	}
	// Modificar o revocar terminales, invitar y aprobar o rechazar solicitudes de emparejamiento.
	return known(RowAdministration, true)
}

// classifyMaintenance: Mantenimiento.
func classifyMaintenance(method string, parts []string) Operation {
	if len(parts) < 2 {
		return unknownOperation(method)
	}
	switch parts[1] {
	case "service":
		// Estado y «Reiniciar» siempre (L1-D6).
		return known(RowAlways, false)
	case "backups":
		// Respaldos: estado, «Respaldar ahora», carpeta y ajustes. Ningún estado los bloquea ni la hora (L1-D14).
		return known(RowRecords, false)
	case "legacy", "test-data":
		// Migración del sistema anterior y «Quitar datos de prueba»: administración.
		if readMethod(method) {
			return known(RowRecords, false)
		}
		return known(RowAdministration, true)
	}
	return unknownOperation(method)
}

// classifyRestaurant: restaurant/* (mesas, cuentas y pedidos).
func classifyRestaurant(method string, parts []string) Operation {
	if readMethod(method) {
		return known(RowRecords, false)
	}
	length := len(parts)
	resource := ""
	if length > 1 {
		resource = parts[1]
	}
	switch {
	case length == 2 && resource == "accounts" && method == "POST":
		// Abrir una cuenta, con o sin productos, también para llevar.
		return known(RowCapture, true)
	case length == 2 && resource == "offline-orders" && method == "POST":
		return known(RowCapture, true)
	case length == 3 && resource == "accounts" && method == "PATCH":
		// Datos de la cuenta ya abierta (lo ya capturado).
		return known(RowCaptured, true)
	case length == 4 && resource == "accounts":
		return classifyAccountAction(method, parts[3])
	case resource == "areas" || resource == "tables" || resource == "layout" || resource == "settings" ||
		resource == "cancel-reasons":
		// Áreas, mesas (también fuera de servicio), plano, configuración y motivos de cancelación.
		return known(RowAdministration, true)
	}
	return unknownOperation(method)
}

// classifyAccountAction: restaurant/accounts/{id}/<acción>.
func classifyAccountAction(method, action string) Operation {
	switch method + " " + action {
	case "PUT draft", "POST confirm", "POST edit":
		return known(RowCapture, true)
	case "POST draft-preview", "POST edit-preview":
		// Las previas de la captura solo calculan.
		return known(RowRecords, false)
	case "POST cancel", "POST cancel-account", "POST discard", "POST release", "POST customer", "POST split",
		"POST transfer", "POST merge", "POST move", "POST assign", "POST void":
		// Sobre lo ya capturado: cancelar, descartar o liberar una cuenta vacía, datos del cliente y reorganizar la
		// cuenta (L1-D6). «void» se retiró en la etapa 9F y responde 410 sin escribir nada.
		return known(RowCaptured, true)
	}
	return unknownOperation(method)
}

// classifyPayments: payments/* (cobros, caja y su configuración).
func classifyPayments(method string, parts []string) Operation {
	if readMethod(method) {
		return known(RowRecords, false)
	}
	length := len(parts)
	if length < 2 {
		return unknownOperation(method)
	}
	resource := parts[1]
	switch resource {
	case "accounts":
		if length < 4 {
			return unknownOperation(method)
		}
		return classifyPaymentAction(method, strings.Join(parts[3:], "/"))
	case "shifts":
		if length == 2 && method == "POST" {
			return known(RowOpenShift, true)
		}
		if length >= 4 {
			return classifyShiftAction(method, strings.Join(parts[3:], "/"))
		}
	case "cash":
		return classifyCash(method, parts)
	case "cash-reports", "cash-operations", "cash-documents":
		// Imprimir y reimprimir reportes y documentos de caja: son reportes (avance §0.5.7).
		if length == 4 {
			switch parts[3] {
			case "print", "reprint", "retry", "rerender":
				return known(RowRecords, false)
			}
		}
	case "settings", "tax", "methods", "discount-reasons":
		// Configuración del cobro: IVA, dólares, métodos de pago y motivos de descuento.
		return known(RowAdministration, true)
	}
	return unknownOperation(method)
}

// classifyPaymentAction: payments/accounts/{id}/<acción>.
func classifyPaymentAction(method, action string) Operation {
	if method != "POST" {
		return unknownOperation(method)
	}
	switch action {
	case "preview", "discounts/preview", "refunds/preview":
		return known(RowRecords, false)
	case "payments", "discounts", "charges", "adjustments/void", "refunds", "corrections", "deliver",
		"precheck/print", "documents/reprint", "documents/retry", "documents/receipt", "documents/rerender":
		// Cobrar, descuentos, cargos, devoluciones, correcciones, «Entregado», precuentas y tickets.
		return known(RowCaptured, true)
	}
	return unknownOperation(method)
}

// classifyShiftAction: payments/shifts/{id}/<acción>.
func classifyShiftAction(method, action string) Operation {
	switch method + " " + action {
	case "POST income", "POST withdrawals", "POST expenses", "POST corrections", "POST partial-cuts",
		"POST closing", "POST closing/cancel", "POST closing/confirm", "POST closure/comments", "POST usd-rate",
		"PUT closing/count":
		return known(RowCloseShift, true)
	}
	return unknownOperation(method)
}

// classifyCash: payments/cash/*.
func classifyCash(method string, parts []string) Operation {
	if len(parts) < 3 {
		return unknownOperation(method)
	}
	switch parts[2] {
	case "count-preview":
		return known(RowRecords, false)
	case "open-drawer":
		// Abrir el cajón a mano es una operación de efectivo del turno.
		return known(RowCloseShift, true)
	case "denominations", "expense-categories", "limit", "drawers":
		// Denominaciones, categorías de gastos, tope de caja y cajones (también «Probar apertura»).
		return known(RowAdministration, true)
	}
	return unknownOperation(method)
}

// classifyKitchen: kitchen/*.
func classifyKitchen(method string, parts []string) Operation {
	length := len(parts)
	resource := ""
	if length > 1 {
		resource = parts[1]
	}
	if readMethod(method) {
		// Toda lectura de cocina es F6, también el tablero de Preparación (GET kitchen/board): lo lee además la vista de
		// una cuenta (L1-D28), y entrar en modo Cocina ya lo controla kitchen/display-mode (F5, con cupo).
		return known(RowRecords, false)
	}
	switch {
	case length == 2 && resource == "display-mode" && method == "POST":
		// Entrar o salir de Preparación (Fase D): solo ocupa o libera el cupo de cocina; no registra nada con fecha.
		return known(RowKitchen, false)
	case length == 2 && resource == "states" && method == "POST":
		return known(RowKitchen, true)
	case length == 4 && resource == "tickets" && parts[3] == "ack" && method == "POST":
		return known(RowKitchen, true)
	case length == 4 && resource == "releases" && (parts[3] == "release" || parts[3] == "schedule"):
		return known(RowKitchen, true)
	case length == 4 && resource == "changes" && parts[3] == "retry":
		return known(RowKitchen, true)
	case length == 4 && resource == "jobs" && (parts[3] == "retry" || parts[3] == "resolve"):
		return known(RowKitchen, true)
	case length == 2 && (resource == "reprint" || resource == "test-print"):
		return known(RowKitchen, true)
	case length == 3 && resource == "print-jobs" && parts[2] == "rerender":
		return known(RowKitchen, true)
	case resource == "stations" || resource == "printers" || resource == "routes" || resource == "settings" ||
		resource == "timing" || resource == "access":
		// Configuración de cocina: estaciones, impresoras (también la prueba de ancho), rutas, formatos, tiempos y
		// acceso.
		return known(RowAdministration, true)
	}
	return unknownOperation(method)
}
