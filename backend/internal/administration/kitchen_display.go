package administration

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/payments"
)

// Pantalla de cocina para Android (etapa K1, Fase A; §2 y §3 de la especificación, decisiones K1-D3, K1-D4, K1-D6,
// K1-D7, K1-D9, K1-D12, K1-D13 y K1-D16 del coordinador).
//
// La pantalla es un dispositivo de tipo kitchen (emparejado con device_kind kitchen y X-Client-Kind kitchen) sin
// usuario ni sesión: queda siempre abierta en modo cocina. dispatch la autentica por su credencial y la manda aquí, en
// un solo punto, antes de toda ruta de sesión, PIN, envío, agente o avisos (service.go). Aquí:
//
//   - la lista cerrada de rutas (kitchenDisplayRoutes). Todo lo demás responde 403 KITCHEN_DISPLAY_ONLY y queda en la
//     auditoría, aunque la petición lleve el Authorization de otra sesión: nunca se llega a authenticateSession;
//   - la puerta de la licencia (license_gate.go, con la fila F5 de kitchen-display/* en internal/licensing);
//   - el principal: el dispositivo con la lista cerrada de permisos kitchen.DisplayPermissions, nunca de un rol;
//   - el cupo de cocina ligado al dispositivo (K1-D6): connect lo ocupa con el claim y el lock 7820148311 de la L1; las
//     demás rutas de pedidos lo exigen;
//   - la contraseña del administrador para los ajustes de la tableta (K1-D13) y para «Volver a emparejar», que revoca
//     la propia pantalla y suelta su cupo (K1-D32);
//   - la lógica de cocina está en kitchen/display.go y reutiliza la de la 9F.
//
// Orden de los locks: las escrituras toman 7820148305 exclusivo antes de tocar la fila del dispositivo (última
// conexión y versión instalada, touchDevice), como PUT kitchen/access; connect toca la fila y después toma 7820148311,
// el último, sin publicar nada con él tomado (K1-D19).

// kitchenDisplayKind es el tipo de dispositivo de la pantalla de cocina (el reservado desde la 0023) y su X-Client-Kind.
const kitchenDisplayKind = "kitchen"

// kitchenDisplayPrefix es el prefijo de las rutas propias de la pantalla.
const kitchenDisplayPrefix = "kitchen-display/"

// kitchenDisplayRoutes es la lista cerrada de lo que responde a una pantalla de cocina (método y ruta).
var kitchenDisplayRoutes = map[string]bool{
	"POST kitchen-display/connect":         true,
	"GET kitchen-display/orders":           true,
	"GET kitchen-display/recent":           true,
	"POST kitchen-display/items/ready":     true,
	"POST kitchen-display/orders/ready":    true,
	"POST kitchen-display/recover":         true,
	"POST kitchen-display/receipts":        true,
	"POST kitchen-display/settings-access": true,
	"POST kitchen-display/unpair":          true,
	// El canal de eventos (GET events llega aquí como events/snapshot) y el resumen de la licencia del dispositivo.
	"GET events/snapshot": true,
	"GET license/status":  true,
}

// kitchenDisplayWrites son las rutas que escriben en cocina: toman 7820148305 exclusivo.
var kitchenDisplayWrites = map[string]bool{
	"kitchen-display/items/ready":  true,
	"kitchen-display/orders/ready": true,
	"kitchen-display/recover":      true,
	"kitchen-display/receipts":     true,
}

// kitchenDisplay atiende toda petición de un dispositivo kitchen ya autenticado.
func (service *Service) kitchenDisplay(ctx context.Context, tx pgx.Tx, request Request, route string) (any, error) {
	principal := Principal{DeviceID: request.DeviceID, Permissions: slices.Clone(kitchen.DisplayPermissions)}
	if !kitchenDisplayRoutes[request.Method+" "+route] {
		return nil, kitchenDisplayDenied(ctx, tx, principal, request, route, "KITCHEN_DISPLAY_ONLY",
			"Esta pantalla de cocina solo muestra y marca pedidos.")
	}
	if err := service.licenseGate(ctx, tx, request, route); err != nil {
		return nil, err
	}
	if kitchenDisplayWrites[route] {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7820148305)"); err != nil {
			return nil, err
		}
	}
	if err := touchKitchenDisplay(ctx, tx, request); err != nil {
		return nil, err
	}
	switch route {
	case "events/snapshot":
		return kitchenDisplayEvents(ctx, tx, request)
	case "license/status":
		return service.licenseDeviceSummary(), nil
	case "kitchen-display/connect":
		return service.kitchenDisplayConnect(ctx, tx, request, principal)
	case "kitchen-display/settings-access":
		return kitchenDisplaySettingsAccess(ctx, tx, request, principal)
	case "kitchen-display/unpair":
		return service.kitchenDisplayUnpair(ctx, tx, request, principal)
	}
	if err := service.requireKitchenDisplaySeat(ctx, tx, request.DeviceID); err != nil {
		return nil, err
	}
	if route == "kitchen-display/orders" {
		return service.kitchenDisplayOrders(ctx, tx, request, principal)
	}
	// Recientes, marcar, recuperar y el acuse: en su savepoint, para que un 403 (una estación que la pantalla ya no
	// muestra) revierta lo escrito y quede solo su auditoría, como en las rutas con sesión.
	return runProtected(ctx, tx, principal, request, route, func(work pgx.Tx) (any, error) {
		return runKitchenDisplay(ctx, work, request, route, principal)
	})
}

// touchKitchenDisplay anota la última conexión y la versión instalada de la pantalla (como touchDevice) y vuelve a
// comprobar la revocación (revisión R1, hallazgo R1-2, K1-D29): authenticateDevice lee revoked_at sin bloquear la fila,
// así que una revocación que se confirma mientras la petición espera la fila o el lock de cocina no se veía. El UPDATE
// espera la fila del dispositivo y, en READ COMMITTED, vuelve a evaluar la condición sobre la versión confirmada: si la
// pantalla ya está revocada no cambia ninguna fila y la petición responde 401 DEVICE_REVOKED, antes de cualquier ruta de
// la lista (en las escrituras, después de 7820148305).
func touchKitchenDisplay(ctx context.Context, tx pgx.Tx, request Request) error {
	touched, err := tx.Exec(ctx, `UPDATE app.devices SET last_seen_at=now(),app_version=coalesce(NULLIF($2,''),app_version)
        WHERE id=$1 AND revoked_at IS NULL`, request.DeviceID, installedVersion(request.ClientVersion))
	if err != nil {
		return err
	}
	if touched.RowsAffected() == 0 {
		return unauthorized("DEVICE_REVOKED", "La autorización de esta terminal fue revocada.")
	}
	return nil
}

// runKitchenDisplay llama a la lógica de cocina de la pantalla con la auditoría firmada por el dispositivo.
func runKitchenDisplay(ctx context.Context, tx pgx.Tx, request Request, route string, principal Principal) (any, error) {
	result, err := kitchen.HandleDisplay(ctx, tx, kitchen.DisplayRequest{Method: request.Method, Route: route,
		Body: request.Body, DeviceID: request.DeviceID},
		func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error {
			return audit(ctx, tx, principal, action, entity, id, before, after, true)
		})
	return result, kitchenFault(err)
}

// kitchenFault traduce un error del paquete de cocina a su respuesta pública.
func kitchenFault(err error) error {
	var problem *kitchen.Error
	if errors.As(err, &problem) {
		return &Fault{Status: problem.Status, Code: problem.Code, Message: problem.Message, Details: problem.Details}
	}
	return err
}

// kitchenDisplayDenied es el rechazo de una ruta que no corresponde al tipo de dispositivo (KITCHEN_DISPLAY_ONLY a una
// pantalla de cocina, KITCHEN_DISPLAY_REQUIRED a otra terminal), con su auditoría access.denied, que se conserva.
func kitchenDisplayDenied(ctx context.Context, tx pgx.Tx, principal Principal, request Request, route, code, message string) error {
	details := map[string]string{"method": request.Method, "operation": strings.Split(route, "/")[0], "code": code}
	if err := audit(ctx, tx, principal, "access.denied", "authorization", nil, nil, details, false); err != nil {
		return err
	}
	return &Fault{Status: 403, Code: code, Message: message, Commit: true}
}

// kitchenDisplayEvents es la lectura del canal de eventos de la pantalla (kitchen.DisplayEvents): solo admite after.
func kitchenDisplayEvents(ctx context.Context, tx pgx.Tx, request Request) (any, error) {
	for key := range request.Query {
		if key != "after" {
			return nil, invalid("Parámetro de eventos no permitido.")
		}
	}
	after := int64(0)
	if text := request.Query.Get("after"); text != "" {
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err != nil || parsed < 0 {
			return nil, invalid("El cursor de eventos no es válido.")
		}
		after = parsed
	}
	batch, err := kitchen.DisplayEvents(ctx, tx, request.DeviceID, after)
	if err != nil {
		return nil, kitchenFault(err)
	}
	return batch, nil
}

// kitchenDisplayView es lo que la pantalla necesita además de sus tarjetas: su configuración, el establecimiento y el
// resumen de la licencia (el mismo `license` de restaurant/state; nil sin gestor de licencias).
func (service *Service) kitchenDisplayView(ctx context.Context, tx pgx.Tx, deviceID string) (map[string]any, error) {
	config, err := kitchen.LoadDisplayConfig(ctx, tx, deviceID)
	if err != nil {
		return nil, kitchenFault(err)
	}
	establishment, err := object(ctx, tx, "SELECT e.name,e.timezone FROM app.establishment e")
	if err != nil {
		return nil, err
	}
	license, err := service.licenseSummary(ctx, tx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"display": config, "establishment": establishment, "license": license}, nil
}

// kitchenDisplayConnect atiende POST kitchen-display/connect {take_seat?}: ocupa el cupo de cocina de la pantalla (o lo
// conserva) y devuelve su configuración. Repetirlo deja lo mismo. Después de «Liberar cupo» en Terminales responde
// KITCHEN_DISPLAY_SEAT_RELEASED y no lo vuelve a ocupar solo: solo con take_seat («Usar esta pantalla», K1-D6).
func (service *Service) kitchenDisplayConnect(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	var input struct {
		TakeSeat bool `json:"take_seat"`
	}
	if len(request.Body) > 0 {
		if err := decode(request.Body, &input); err != nil {
			return nil, err
		}
	}
	var releasedAt *time.Time
	if err := tx.QueryRow(ctx, "SELECT kitchen_display_seat_released_at FROM app.devices WHERE id=$1",
		request.DeviceID).Scan(&releasedAt); err != nil {
		return nil, err
	}
	if releasedAt != nil {
		if !input.TakeSeat {
			return nil, seatReleasedFault()
		}
		// La fila del dispositivo y la auditoría van antes de 7820148311, que toma el claim (el último lock).
		if _, err := tx.Exec(ctx, "UPDATE app.devices SET kitchen_display_seat_released_at=NULL WHERE id=$1", request.DeviceID); err != nil {
			return nil, err
		}
		if err := audit(ctx, tx, principal, "kitchen_display.seat_taken", "device", &principal.DeviceID,
			map[string]any{"seat_released_at": releasedAt}, map[string]any{"seat_released_at": nil}, true); err != nil {
			return nil, err
		}
	}
	if err := service.seats().ConnectKitchenDisplay(ctx, tx, request.DeviceID); err != nil {
		return nil, displaySeatFault(err)
	}
	view, err := service.kitchenDisplayView(ctx, tx, request.DeviceID)
	if err != nil {
		return nil, err
	}
	view["seat"] = map[string]bool{"held": true}
	view["server_time"] = time.Now().UTC()
	return view, nil
}

// kitchenDisplayOrders atiende GET kitchen-display/orders: el estado completo de la pantalla. El cursor de eventos se lee
// antes que las tarjetas, en la misma transacción: todo cambio que confirme otra transacción después tiene un evento
// posterior al cursor, así que abrir el canal desde él nunca pierde nada (a lo más se vuelve a leer de más).
func (service *Service) kitchenDisplayOrders(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	cursor, err := events.Head(ctx, tx)
	if err != nil {
		return nil, err
	}
	result, err := runKitchenDisplay(ctx, tx, request, "kitchen-display/orders", principal)
	if err != nil {
		return nil, err
	}
	orders := result.(*kitchen.DisplayOrders)
	view, err := service.kitchenDisplayView(ctx, tx, request.DeviceID)
	if err != nil {
		return nil, err
	}
	view["cursor"] = cursor
	view["server_time"] = orders.ServerTime
	view["orders"] = orders.Orders
	view["receipts_pending"] = orders.ReceiptsPending
	return view, nil
}

// requireKitchenDisplaySeat exige que la pantalla ocupe su cupo de cocina (K1-D6): sin él no muestra ni marca pedidos.
func (service *Service) requireKitchenDisplaySeat(ctx context.Context, tx pgx.Tx, deviceID string) error {
	held, err := service.seats().DeviceSeats(ctx, tx, deviceID)
	if err != nil {
		return err
	}
	if slices.Contains(held, licenseseats.KitchenDisplays) {
		return nil
	}
	var released bool
	if err := tx.QueryRow(ctx, "SELECT kitchen_display_seat_released_at IS NOT NULL FROM app.devices WHERE id=$1",
		deviceID).Scan(&released); err != nil {
		return err
	}
	if released {
		return seatReleasedFault()
	}
	return &Fault{Status: 409, Code: "KITCHEN_DISPLAY_NOT_CONNECTED",
		Message: "Esta pantalla se está conectando con la computadora principal."}
}

// seatReleasedFault es el estado «cupo liberado» de K1-D6.
func seatReleasedFault() error {
	return &Fault{Status: 409, Code: "KITCHEN_DISPLAY_SEAT_RELEASED", Message: "Esta pantalla dejó su cupo de cocina."}
}

// displaySeatFault traduce el rechazo de los cupos para la pantalla: sin cupo, el mensaje de la §6 de la especificación
// con los datos de details (seat, in_use y limit), como seatFault.
func displaySeatFault(err error) error {
	var unavailable *licenseseats.UnavailableError
	if errors.As(err, &unavailable) {
		message := "No hay cupo de pantalla de cocina libre (" + strconv.Itoa(unavailable.InUse) + " de " +
			strconv.Itoa(unavailable.Limit) + " en uso). Revoca otra pantalla en Terminales o amplía tu licencia."
		if unavailable.Limit == 0 {
			message = "La licencia no incluye pantallas de cocina (0 de 0). Amplía tu licencia para usar esta pantalla."
		}
		return &Fault{Status: 409, Code: "LICENSE_SEAT_UNAVAILABLE", Message: message, Details: map[string]any{
			"seat":   unavailable.Seat,
			"in_use": unavailable.InUse,
			"limit":  unavailable.Limit,
		}}
	}
	return seatFault(err)
}

// kitchenDisplaySettingsAccess atiende POST kitchen-display/settings-access {username, password} (§5.4, K1-D13): los
// ajustes de la tableta piden la contraseña de un administrador. La tableta no recibe ningún token: solo
// {authorized: true}.
func kitchenDisplaySettingsAccess(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	supervisor, err := kitchenDisplayAdministrator(ctx, tx, request, principal, "kitchen_display.settings_denied",
		"Ese usuario no puede abrir los ajustes de esta pantalla.")
	if err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, principal, "kitchen_display.settings_opened", "device", &principal.DeviceID, nil,
		map[string]string{"username": supervisor.Username}, true); err != nil {
		return nil, err
	}
	return map[string]bool{"authorized": true}, nil
}

// kitchenDisplayUnpair atiende POST kitchen-display/unpair {username, password} (K1-D32): «Volver a emparejar» en la
// tableta. Con la contraseña de un administrador, la pantalla se revoca a sí misma igual que Terminales › «Revocar»
// (revokeDevice: suelta el cupo, cierra lo suyo y corta el canal con interrupted DEVICE_REVOKED) y así la pantalla
// nueva encuentra el cupo libre. Después, toda petición de esta pantalla responde 401 DEVICE_REVOKED. La auditoría
// kitchen_display.unpaired lleva al administrador como autor y a la pantalla como dispositivo.
func (service *Service) kitchenDisplayUnpair(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	supervisor, err := kitchenDisplayAdministrator(ctx, tx, request, principal, "kitchen_display.unpair_denied",
		"Ese usuario no puede volver a emparejar esta pantalla.")
	if err != nil {
		return nil, err
	}
	previous, err := object(ctx, tx, devicesQuery+" WHERE d.id=$1", principal.DeviceID)
	if err != nil {
		return nil, err
	}
	if err := revokeDevice(ctx, tx, principal.DeviceID, service.seats()); err != nil {
		return nil, err
	}
	author := Principal{UserID: supervisor.UserID, Username: supervisor.Username, DeviceID: principal.DeviceID}
	if err := audit(ctx, tx, author, "kitchen_display.unpaired", "device", &principal.DeviceID, previous,
		map[string]bool{"revoked": true}, true); err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

// kitchenDisplayAdministrator valida {username, password} de un administrador en una ruta de la pantalla
// (settings-access y unpair). Se valida con supervisorAuthorizer en esta misma transacción, sin crear ni reemplazar
// sesiones, con el mismo límite de intentos que el inicio de sesión (compartido entre las dos rutas: es por pantalla y
// usuario) y sin registrar la contraseña. «Administrador» es quien puede administrar terminales (devices.manage); sin
// ese permiso, 403 con la auditoría denied y el texto de la ruta.
func kitchenDisplayAdministrator(ctx context.Context, tx pgx.Tx, request Request, principal Principal, denied, deniedMessage string) (*payments.Supervisor, error) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	username := strings.ToLower(strings.TrimSpace(input.Username))
	if username == "" || input.Password == "" {
		return nil, invalid("Escribe el usuario y la contraseña de un administrador.")
	}
	// Revisión R1 (hallazgo R1-1, K1-D29): las mismas longitudes que el inicio de sesión (40 y 256). Un usuario o una
	// contraseña más largos no son de nadie: se responde como credenciales incorrectas, sin validarlos ni contar un
	// intento (no los recibe supervisorAuthorizer, que respondería con el código y el texto de las autorizaciones de
	// cobros).
	if len(username) > 40 || len(input.Password) > 256 {
		return nil, settingsAccessInvalid()
	}
	supervisor, err := supervisorAuthorizer(principal)(ctx, tx, username, input.Password)
	var fault *Fault
	if errors.As(err, &fault) && fault.Code == "AUTHORIZATION_INVALID" {
		// El mismo rechazo (y su intento contado) con el texto de la pantalla, no el del supervisor de cobros.
		fault.Message = settingsAccessInvalid().(*Fault).Message
	}
	if err != nil {
		return nil, err
	}
	if !slices.Contains(supervisor.Permissions, "devices.manage") {
		if err := audit(ctx, tx, principal, denied, "device", &principal.DeviceID, nil,
			map[string]string{"username": supervisor.Username}, false); err != nil {
			return nil, err
		}
		return nil, &Fault{Status: 403, Code: "PERMISSION_DENIED", Message: deniedMessage, Commit: true}
	}
	return supervisor, nil
}

// settingsAccessInvalid es el rechazo de un usuario o una contraseña incorrectos en los ajustes de la pantalla.
func settingsAccessInvalid() error {
	return &Fault{Status: 401, Code: "AUTHORIZATION_INVALID", Message: "El usuario o la contraseña no son correctos."}
}
