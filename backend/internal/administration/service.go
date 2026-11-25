package administration

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/backupschedule"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/httpapi"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/legacyimport"
	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/maintenance"
	"local/ad-el-pos/server/internal/reports"
)

type Service struct {
	Pool             *pgxpool.Pool
	ProvisioningFile string
	// Queues consulta las colas CUPS para el cajón (etapa 7.1). Sin valor, payments usa las
	// consultas de solo lectura del equipo; las pruebas de aceptación inyectan una simulada.
	Queues kitchen.QueueInspector
	// Rasters entrega los bits y la vista previa del logotipo para las líneas de imagen de la vista
	// previa del diseñador (etapa 9B; la implementación es de la fase B). Sin valor, la vista previa
	// no lleva la imagen.
	Rasters documents.RasterSource
	// Reports ejecuta las consultas de reportes (etapa 9A) en su transacción de solo lectura con un
	// semáforo propio. Sin valor se crea al primer uso; las pruebas pueden inyectar uno.
	Reports *reports.Runner
	// Stopping se cierra cuando el servidor empieza a apagarse (etapa 9E): los flujos SSE terminan de
	// inmediato y el cierre ordenado no agota su plazo. Sin valor (pruebas) nunca se activa.
	Stopping <-chan struct{}
	// Backups crea el respaldo previo a «Quitar datos de prueba» (etapa 9F): pg_dump con la conexión de la
	// configuración en serve y el respaldo de la instalación en el host de Windows. Sin valor no se
	// quita nada (las pruebas inyectan uno controlado).
	Backups maintenance.Backuper
	// Host informa el estado del servicio del sistema y permite reiniciarlo (etapa 9G, fase A;
	// service_host.go). Sin valor (serve y pruebas), el servidor no es un servicio administrado.
	Host HostControl
	// Identity es la identidad del servidor para emparejar terminales (etapa 9G, fase B; pairing.go): su
	// identificador y la huella SPKI del certificado de la escucha de red. Sin valor no se empareja.
	Identity *ServerIdentity
	// IdentityProblem explica (para la bitácora y Terminales) por qué no se cargó la identidad: el servidor
	// sigue atendiendo a la propia computadora, sin escucha de red ni emparejamiento (revisión, hallazgo 4).
	IdentityProblem string
	// NetworkPort es el puerto de la escucha HTTPS de la red local (0: sin escucha de red, como «serve» de
	// desarrollo sin network_listen_address o Windows hasta la Fase W). La computadora principal lo
	// muestra en Terminales junto a sus direcciones para emparejar a mano.
	NetworkPort int
	// ScheduledBackups hace los respaldos programados y «Respaldar ahora» (etapa 9G, fase F; backups.go). El
	// trabajador diario de main.go usa el mismo. Sin valor (pruebas que no lo necesitan), las rutas de respaldos
	// responden 503.
	ScheduledBackups *backupschedule.Runner
	// Legacy es la migración del sistema anterior (etapa 9H, fase C; legacy_migration.go): subidas, corridas y su
	// trabajador, que arranca main.go. Sin valor, las rutas de la migración responden 503.
	Legacy *legacyimport.Manager
	// License es el gestor de la licencia del servidor principal (etapa L1, fase A; internal/licensing): estado,
	// cupos de la licencia y última hora confiable. Lo carga main.go al arrancar, después de la identidad. Lo usan
	// las rutas license/* (Fase B), la puerta de la §6 y la hora (Fase C) y los cupos (Fase D). Sin valor
	// (pruebas que no lo necesitan) no hay licencia.
	License *licensing.Manager
	// LicenseServer es el cliente del servidor de licencias (etapa L1, fase B; license.go): activar, transferir,
	// consultar y liberar. Sin valor, esas rutas responden 503 LICENSE_SERVER_UNAVAILABLE.
	LicenseServer *licensing.Client
	// Seats aplica los cupos de la licencia (etapa L1, Fase D; license_seats.go e internal/licenseseats). Sin valor, o
	// sin fuente de límites, no hay licencia válida y ninguna operación que pide cupo procede (falla cerrada): main.go
	// lo crea siempre y las pruebas inyectan uno con los cupos y el reloj que necesitan (licenseseats/seatstest).
	Seats *licenseseats.Manager
	// Etapa L1, Fase C (license_gate.go, license_clock.go y license_monitor.go): la puerta de la licencia y la
	// protección de la hora. LicenseServerClock da la hora del servidor de licencias para «Comprobar la hora con
	// internet» (§6.3; lo implementa el cliente de la Fase B; sin valor, esa ruta responde 503). SupportTrust son las
	// llaves públicas de soporte del desbloqueo sin internet (§6.4); sin valor, las compiladas (solo las pruebas lo
	// cambian). licenseMonitorState es el vigilante que audita los bloqueos y publica license.changed.
	LicenseServerClock  LicenseServerClock
	SupportTrust        *licensing.SupportTrust
	licenseMonitorOnce  sync.Once
	licenseMonitorState *licenseMonitor
	// Logger recibe el detalle técnico de los fallos que la interfaz explica sin términos técnicos.
	Logger      *slog.Logger
	reportsOnce sync.Once
	streamMutex sync.Mutex
	streams     map[string]int
	// readyStreams es el canal de avisos de platillos listos abierto de cada dispositivo (etapa A2, parte D): uno por
	// dispositivo, aparte de los dos del canal de eventos; uno nuevo reemplaza al anterior (ready_alerts.go).
	readyStreams map[string]*readyStream
	// removalTurn deja una sola eliminación de datos de prueba a la vez en este servidor (etapa 9F,
	// maintenance.go); se crea al primer uso.
	removalOnce sync.Once
	removalTurn chan struct{}
}

type Request struct {
	Method            string
	Path              string
	Query             url.Values
	Body              json.RawMessage
	DeviceID          string
	DeviceToken       string
	SessionToken      string
	ProvisioningToken string
	// Local: la petición llegó por la escucha de loopback (nunca por la escucha de red, aunque venga de
	// la propia computadora; etapa 9G, D1).
	Local bool
	// SourceAddress es la IP de origen de la conexión (límite de solicitudes de emparejamiento por
	// origen, etapa 9G) y PairingSecret el secreto de la solicitud (X-Pairing-Secret) con el que la
	// terminal que se empareja consulta su solicitud.
	SourceAddress string
	PairingSecret string
	// IfNoneMatch es la cabecera del mismo nombre (etapa 9G): la imagen binaria del catálogo responde 304 si coincide.
	IfNoneMatch string
	// OfflineCredential es la credencial de envío del mesero (X-Offline-Credential, etapa A1, A1-D1): sustituye a la
	// sesión solo en POST restaurant/offline-orders (pin_login.go).
	OfflineCredential string
	// ClientKind es X-Client-Kind (etapa A1, revisión R3, B2): con la credencial de un dispositivo debe coincidir con el
	// tipo con que se registró (matchClientKind), para que nadie tome la versión mínima de otro tipo de cliente.
	ClientKind string
	// ClientVersion es X-Client-Version (etapa K1, Fase A, K1-D9): la versión instalada de la aplicación, que Go guarda en
	// el dispositivo en cada petición autenticada (touchDevice) para que Terminales muestre la instalada y no la de
	// cuando se emparejó.
	ClientVersion string
}

type Fault struct {
	Status  int
	Code    string
	Message string
	Commit  bool
	// Details son datos públicos opcionales del rechazo; se publican en error.details.
	Details any
	// Rejection es la auditoría de un rechazo que debe conservarse aunque la operación se
	// revierta (etapa 7.1, rechazos por efectivo insuficiente). Nunca se publica en la respuesta.
	Rejection *RejectionAudit
}

// RejectionAudit describe el registro de auditoría (success=false) de un rechazo.
type RejectionAudit struct {
	Action   string
	Entity   string
	EntityID *string
	Details  any
}

func (fault *Fault) Error() string {
	return fault.Code
}

func invalid(message string) error {
	return &Fault{Status: 400, Code: "VALIDATION_ERROR", Message: message}
}

func denied() error {
	return &Fault{Status: 403, Code: "PERMISSION_DENIED", Message: "No tienes permiso para esta operación."}
}

func unauthorized(code, message string) error {
	return &Fault{Status: 401, Code: code, Message: message}
}

type Principal struct {
	UserID          string   `json:"id"`
	Username        string   `json:"username"`
	DisplayName     string   `json:"display_name"`
	RoleID          string   `json:"role_id"`
	RoleName        string   `json:"role_name"`
	InitialPassword bool     `json:"initial_password"`
	Support         bool     `json:"support"`
	Active          bool     `json:"active"`
	SessionID       string   `json:"session_id"`
	DeviceID        string   `json:"device_id"`
	Permissions     []string `json:"permissions"`
	// PINSession: la sesión se abrió con PIN en un dispositivo de meseros (etapa A1, A1-D17) y sus permisos ya están
	// acotados a los de mesero (waiter_scope.go). Lo hereda la sesión nueva de «Cambiar contraseña».
	PINSession bool `json:"-"`
}

func (principal Principal) require(key string) error {
	if !slices.Contains(principal.Permissions, key) {
		return denied()
	}
	return nil
}

var identifierPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,39}$`)

func decode(body json.RawMessage, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return invalid("La solicitud contiene campos o valores inválidos.")
	}
	return nil
}

func list(ctx context.Context, tx pgx.Tx, query string, args ...any) (json.RawMessage, error) {
	var result json.RawMessage
	err := tx.QueryRow(ctx, "SELECT COALESCE(jsonb_agg(to_jsonb(records)), '[]'::jsonb) FROM ("+query+") records", args...).Scan(&result)
	return result, err
}

func object(ctx context.Context, tx pgx.Tx, query string, args ...any) (json.RawMessage, error) {
	var result json.RawMessage
	err := tx.QueryRow(ctx, "SELECT to_jsonb(record) FROM ("+query+") record", args...).Scan(&result)
	return result, err
}

func audit(ctx context.Context, tx pgx.Tx, principal Principal, action, entity string, entityID *string, before, after any, success bool) error {
	var actorID, deviceID *string
	actorName, actorKind := "local_owner", "local_owner"
	if principal.UserID == "" && principal.DeviceID != "" {
		actorName, actorKind = "terminal", "device"
	}
	if principal.UserID != "" {
		actorID = &principal.UserID
		actorName, actorKind = principal.Username, "user"
	}
	if principal.DeviceID != "" {
		deviceID = &principal.DeviceID
	}
	previous, err := json.Marshal(before)
	if err != nil {
		return err
	}
	next, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.audit_events(actor_id,actor_username,actor_kind,device_id,action,entity_type,entity_id,success,before_state,after_state)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, actorID, actorName, actorKind, deviceID, action, entity, entityID, success, previous, next)
	return err
}

func (service *Service) Execute(ctx context.Context, request Request) (any, error) {
	// Etapa A1 (revisión R3, O4): pocas entradas con PIN a la vez (cada una verifica un Argon2id de 64 MiB). Se espera el
	// turno antes de abrir la transacción, para que las que esperan no ocupen conexiones de la base ni bloqueen las
	// demás operaciones del servidor.
	if pinLoginRequest(request) {
		release, err := reservePINCheck(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
	}
	transaction, err := service.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer transaction.Rollback(context.Background())
	result, operationError := service.dispatch(ctx, transaction, request)
	if shouldCommit(operationError) {
		if err := transaction.Commit(ctx); err != nil {
			return nil, err
		}
	}
	// Etapa L1, Fase D (hallazgo R2-2, L1-D43; license_seats.go): el rechazo por falta de cupo revierte la transacción
	// entera, también la actividad con fecha futura que los cupos bajaron a «ahora». Se baja otra vez en una transacción
	// propia, ya revertida la de la petición (que tenía esas filas bloqueadas).
	if seatUnavailable(operationError) {
		_ = transaction.Rollback(ctx)
		service.settleFutureSeatActivity(ctx)
	}
	return result, operationError
}

// shouldCommit confirma solo operaciones correctas o fallos que deben conservar su rastro
// (auditoría de denegaciones, intentos de acceso o sesiones expiradas).
func shouldCommit(operationError error) bool {
	var fault *Fault
	return operationError == nil || (errors.As(operationError, &fault) && fault.Commit)
}

// runProtected ejecuta una operación autenticada dentro de un savepoint. Si la operación
// termina en 403, se deshace todo lo que hubiera escrito, aunque la verificación de permiso
// ocurriera después de una escritura, y solo se conserva la auditoría de la denegación.
// Así la regla "toda autorización antes de la primera escritura" no depende de convención:
// una autorización tardía nunca confirma cambios parciales. Un rechazo que pide su propia
// auditoría (etapa 7.1: retiro o gasto por encima del efectivo disponible) sigue el mismo
// camino: se revierte el trabajo y se confirma solo su registro con success=false.
func runProtected(ctx context.Context, tx pgx.Tx, principal Principal, request Request, route string, operation func(pgx.Tx) (any, error)) (any, error) {
	work, err := tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	result, operationError := operation(work)
	var fault *Fault
	if errors.As(operationError, &fault) && fault.Status == 403 {
		if err := work.Rollback(ctx); err != nil {
			return nil, err
		}
		details := map[string]string{"method": request.Method, "operation": strings.Split(route, "/")[0]}
		if err := audit(ctx, tx, principal, "access.denied", "authorization", nil, nil, details, false); err != nil {
			return nil, err
		}
		fault.Commit = true
		return nil, operationError
	}
	if fault != nil && fault.Rejection != nil {
		if err := work.Rollback(ctx); err != nil {
			return nil, err
		}
		rejection := fault.Rejection
		if err := audit(ctx, tx, principal, rejection.Action, rejection.Entity, rejection.EntityID, nil, rejection.Details, false); err != nil {
			return nil, err
		}
		fault.Commit = true
		return nil, operationError
	}
	if operationError != nil && !shouldCommit(operationError) {
		// La transacción completa se revertirá; se descarta también el savepoint.
		if err := work.Rollback(ctx); err != nil {
			return nil, err
		}
		return result, operationError
	}
	if err := work.Commit(ctx); err != nil {
		return nil, err
	}
	return result, operationError
}

func (service *Service) dispatch(ctx context.Context, tx pgx.Tx, request Request) (any, error) {
	route := strings.TrimPrefix(request.Path, "/api/v1/")
	// Credencial de envío del mesero (etapa A1, A1-D1; revisión R3, B1): solo sirve para entregar un pedido capturado
	// sin conexión. En cualquier otra ruta, también las que no piden credencial (asistente, emparejamiento, entrada con
	// contraseña o con PIN), se rechaza antes de atenderla, para que nunca abra ni pruebe nada más.
	if request.OfflineCredential != "" && (route != offlineOrdersRoute || request.Method != "POST") {
		return nil, unauthorized("OFFLINE_CREDENTIAL_NOT_ALLOWED",
			"La autorización de envío solo sirve para entregar pedidos capturados sin conexión.")
	}
	// Etapa L1, Fase C (license_gate.go): la puerta de la licencia y de la hora. Las rutas públicas (asistente, alta de la
	// propia computadora y emparejamiento) pasan aquí, antes de atenderlas; todas las demás, en cuanto se autentica la
	// terminal (abajo).
	if publicLicenseRoute(route) {
		if err := service.licenseGate(ctx, tx, request, route); err != nil {
			return nil, err
		}
	}
	if route == "setup/status" && request.Method == "GET" {
		var configured bool
		err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.establishment)").Scan(&configured)
		return map[string]any{"configured": configured}, err
	}
	if route == "setup" && request.Method == "POST" {
		return service.setup(ctx, tx, request)
	}
	if route == "devices/enroll" && request.Method == "POST" {
		// Etapa 9G (§0-B, «ninguna terminal se conecta sin emparejarse»): el código de 43 caracteres solo
		// autoriza una terminal en la propia computadora; las de otras computadoras se emparejan con código.
		if !request.Local {
			return nil, &Fault{Status: 403, Code: "ENROLLMENT_LOCAL_ONLY", Message: "Las terminales de otras computadoras se emparejan con el código que muestran al buscar el servidor."}
		}
		return enroll(ctx, tx, request)
	}
	// Emparejamiento con código (etapa 9G, fase B): la terminal nueva todavía no tiene credencial.
	if strings.HasPrefix(route, "pairing/") {
		return service.pairingPublic(ctx, tx, request, route)
	}
	// Avisos de platillos listos (etapa A2, parte D): el lock de la operación compartido se toma antes de tocar la fila
	// del dispositivo (revisión R8): quien tiene ese lock en exclusiva y después bloquea la fila de un dispositivo (por
	// ejemplo, al guardar sus estaciones de cocina) no forma un interbloqueo con la lectura de cada 750 ms.
	readyRoute := route == "ready-alerts" && request.Method == "GET"
	if readyRoute {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7820148305)"); err != nil {
			return nil, err
		}
	}
	kind, err := authenticateDevice(ctx, tx, request)
	if err != nil {
		return nil, err
	}
	if err := matchClientKind(kind, request); err != nil {
		return nil, err
	}
	// Etapa K1 (Fase A, K1-D4): el punto único de la pantalla de cocina Android. Un dispositivo kitchen solo llega a su
	// lista cerrada de rutas (kitchen_display.go), sin usuario ni sesión: nunca a la entrada con contraseña o PIN, la
	// credencial de envío, el agente de impresión, los avisos de platillos listos ni authenticateSession, aunque mande el
	// Authorization de otra sesión. Lo demás responde 403 KITCHEN_DISPLAY_ONLY. Va antes de la puerta de la licencia: la
	// puerta se aplica dentro, solo a las rutas de la lista.
	if kind == kitchenDisplayKind {
		return service.kitchenDisplay(ctx, tx, request, route)
	}
	// Al revés: las rutas de la pantalla de cocina con la credencial de otro tipo de dispositivo.
	if strings.HasPrefix(route, kitchenDisplayPrefix) {
		return nil, kitchenDisplayDenied(ctx, tx, Principal{DeviceID: request.DeviceID}, request, route,
			"KITCHEN_DISPLAY_REQUIRED", "Esta operación es solo de las pantallas de cocina.")
	}
	// Etapa L1, Fase C (license_gate.go): la puerta, para todo lo que sigue (entrada con contraseña o PIN, credencial de
	// envío, agente de impresión, avisos de platillos listos y toda operación con sesión).
	if err := service.licenseGate(ctx, tx, request, route); err != nil {
		return nil, err
	}
	if route == "auth/login" && request.Method == "POST" {
		return login(ctx, tx, request)
	}
	// Entrada con PIN en los teléfonos y tabletas de meseros (etapa A1, fase B; pin_login.go): con la credencial del
	// dispositivo android y sin sesión.
	if route == "auth/pin-users" && request.Method == "GET" {
		return pinUsers(ctx, tx, request)
	}
	if route == "auth/pin-login" && request.Method == "POST" {
		return pinLogin(ctx, tx, request)
	}
	// Credencial de envío del mesero (etapa A1, A1-D1): en lugar de una sesión. La ruta ya se comprobó al principio.
	if request.OfflineCredential != "" {
		if request.SessionToken != "" {
			return nil, invalid("Envía la sesión o la autorización de envío del mesero, no las dos.")
		}
		principal, err := authenticateOfflineCredential(ctx, tx, request)
		if err != nil {
			return nil, err
		}
		// Etapa L1 (Fase D): el pedido de la cola pide cupo como cualquier captura (license_seats.go).
		return service.withSeats(ctx, tx, request, principal, route, func() (any, error) {
			return runProtected(ctx, tx, principal, request, route, func(work pgx.Tx) (any, error) {
				return service.protected(ctx, work, request, principal, route)
			})
		})
	}
	// Agente de impresión de la terminal (etapa 9G, fase E; print_agent.go): solo con la credencial de la terminal,
	// sin sesión de usuario, porque imprime aunque nadie haya iniciado sesión en esa caja.
	if strings.HasPrefix(route, "print-agent/") {
		return service.printAgent(ctx, tx, request, route)
	}
	// Avisos de platillos listos (etapa A2, parte D; ready_alerts.go): solo con la credencial del teléfono o la
	// tableta, porque llegan aunque nadie tenga la sesión abierta (en «¿Quién eres?» o con la aplicación cerrada).
	if readyRoute {
		return readyAlerts(ctx, tx, request)
	}
	// Etapa L1, fase B (license.go): el resumen del estado de la licencia con solo la credencial del dispositivo, para
	// el teléfono o la tableta en «¿Quién eres?». Con sesión, GET license/status responde el estado completo.
	if route == "license/status" && request.Method == "GET" && request.SessionToken == "" {
		return service.licenseDeviceSummary(), nil
	}
	principal, err := authenticateSession(ctx, tx, request)
	if err != nil {
		return nil, err
	}
	// Las denegaciones y acciones de soporte también forman parte del historial de seguridad. Etapa L1 (Fase D): los
	// cupos de la licencia se aplican aquí, en un solo punto, después de autenticar y en la misma transacción
	// (license_seats.go; la clasificación es licenseseats.ForOperation).
	result, err := service.withSeats(ctx, tx, request, principal, route, func() (any, error) {
		return runProtected(ctx, tx, principal, request, route, func(work pgx.Tx) (any, error) {
			return service.protected(ctx, work, request, principal, route)
		})
	})
	if err == nil && request.Method == "GET" && principal.Support {
		err = audit(ctx, tx, principal, "support.read", strings.Split(route, "/")[0], nil, nil, nil, true)
	}
	return result, err
}

// authenticateDevice comprueba la credencial del dispositivo y devuelve su tipo (desktop, android o kitchen). Anota su
// última conexión y su versión instalada (touchDevice), salvo en una pantalla de cocina (etapa K1, Fase A): su
// manejador lo anota después de tomar el lock de la operación cuando escribe (kitchen_display.go), para que la fila del
// dispositivo siempre se bloquee después de 7820148305, en el mismo orden que PUT kitchen/access.
func authenticateDevice(ctx context.Context, tx pgx.Tx, request Request) (string, error) {
	if !identifierPattern.MatchString(request.DeviceID) || len(request.DeviceToken) != 43 {
		return "", unauthorized("DEVICE_REQUIRED", "Esta terminal necesita autorización.")
	}
	var revoked bool
	var kind string
	err := tx.QueryRow(ctx, "SELECT revoked_at IS NOT NULL,kind FROM app.devices WHERE id=$1 AND secret_hash=$2",
		request.DeviceID, TokenHash(request.DeviceToken)).Scan(&revoked, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", unauthorized("DEVICE_REQUIRED", "Esta terminal necesita autorización.")
	}
	if err != nil {
		return "", err
	}
	if revoked {
		return "", unauthorized("DEVICE_REVOKED", "La autorización de esta terminal fue revocada.")
	}
	if kind == kitchenDisplayKind {
		return kind, nil
	}
	return kind, touchDevice(ctx, tx, request)
}

// touchDevice anota la última conexión del dispositivo y, desde la etapa K1 (Fase A, K1-D9), la versión instalada que
// informa X-Client-Version, para los tres tipos (escritorio, meseros y cocina): así Terminales muestra la versión nueva
// al actualizar una aplicación encima, sin emparejar otra vez. Solo se guarda una versión válida de 40 caracteres o
// menos (el CHECK de devices.app_version); sin cabecera se conserva la anterior.
func touchDevice(ctx context.Context, tx pgx.Tx, request Request) error {
	version := installedVersion(request.ClientVersion)
	touch := "UPDATE app.devices SET last_seen_at=now(),app_version=coalesce(NULLIF($2,''),app_version) WHERE id=$1"
	// La lectura de avisos de platillos listos llega cada 750 ms desde el canal de cada teléfono (etapa A2, revisión
	// R4): «última conexión» se anota como mucho una vez por minuto (Terminales considera en línea lo de los últimos 2),
	// para no escribir en la base en cada lectura. Una versión nueva se anota enseguida.
	if request.Method == "GET" && request.Path == "/api/v1/ready-alerts" {
		touch += " AND (last_seen_at < now() - interval '1 minute' OR app_version IS DISTINCT FROM coalesce(NULLIF($2,''),app_version))"
	}
	_, err := tx.Exec(ctx, touch, request.DeviceID, version)
	return err
}

// installedVersion es la versión de X-Client-Version si es una versión del producto válida (la misma regla de
// httpapi.CompareVersions) de 40 caracteres o menos, o vacía.
func installedVersion(text string) string {
	if text == "" || len(text) > 40 {
		return ""
	}
	if _, valid := httpapi.CompareVersions(text, "0.0.0"); !valid {
		return ""
	}
	return text
}

// matchClientKind compara X-Client-Kind con el tipo con que se registró el dispositivo (revisión R3, B2): la versión
// mínima se elige por el tipo que declara el cliente (httpapi.checkClient), y un escritorio que dijera «android»
// tomaría la mínima de Android. La aplicación Android solo corre en dispositivos android, la de cocina (etapa K1,
// K1-D3) solo en los kitchen y la de escritorio en los de escritorio. Sin la cabecera (herramientas locales y pruebas
// por la escucha local) no hay nada que comparar: el tipo del dispositivo decide igual qué puede hacer.
func matchClientKind(kind string, request Request) error {
	if request.ClientKind == "" {
		return nil
	}
	if request.ClientKind != expectedClientKind(kind) {
		return &Fault{Status: 400, Code: "CLIENT_KIND_MISMATCH",
			Message: "Esta aplicación no corresponde al tipo de equipo con que se emparejó esta terminal. Empareja de nuevo esta terminal."}
	}
	return nil
}

// expectedClientKind es el X-Client-Kind que corresponde a un tipo de dispositivo.
func expectedClientKind(kind string) string {
	switch kind {
	case "android":
		return "android"
	case kitchenDisplayKind:
		return kitchenDisplayKind
	}
	return "desktop"
}

func authenticateSession(ctx context.Context, tx pgx.Tx, request Request) (Principal, error) {
	var principal Principal
	if len(request.SessionToken) != 43 {
		return principal, unauthorized("SESSION_REQUIRED", "Inicia sesión para continuar.")
	}
	var revoked, active bool
	var reason string
	var expires time.Time
	err := tx.QueryRow(ctx, `SELECT u.id,u.username,u.display_name,u.role_id,r.name,u.initial_password,u.support,
        s.id,s.device_id,s.revoked_at IS NOT NULL,coalesce(s.revoke_reason,''),s.expires_at,u.active,s.opened_with_pin
        FROM app.sessions s JOIN app.users u ON u.id=s.user_id JOIN app.roles r ON r.id=u.role_id
        WHERE s.token_hash=$1 AND s.device_id=$2`, TokenHash(request.SessionToken), request.DeviceID).Scan(
		&principal.UserID, &principal.Username, &principal.DisplayName, &principal.RoleID, &principal.RoleName,
		&principal.InitialPassword, &principal.Support, &principal.SessionID, &principal.DeviceID, &revoked, &reason, &expires, &active,
		&principal.PINSession,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return principal, unauthorized("SESSION_INVALID", "La sesión no es válida.")
	}
	if err != nil {
		return principal, err
	}
	if revoked || !active {
		// Desde la etapa 9H solo un nuevo inicio de sesión en esta misma terminal reemplaza la sesión.
		if reason == "replaced" {
			return principal, unauthorized("SESSION_REPLACED", "Tu sesión se cerró porque el usuario inició sesión de nuevo en esta terminal.")
		}
		return principal, unauthorized("SESSION_REVOKED", "Tu sesión fue revocada. Inicia sesión nuevamente.")
	}
	principal.Active = active
	if time.Now().After(expires) {
		if _, err := tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=now(),revoke_reason='expired' WHERE id=$1", principal.SessionID); err != nil {
			return principal, err
		}
		if err := audit(ctx, tx, principal, "session.expired", "session", &principal.SessionID, nil, nil, true); err != nil {
			return principal, err
		}
		// Etapa L1 (revisión R3, R3-1): si era la última sesión vigente de la terminal, su comandero se libera ahora, con el
		// mismo reloj con que se comprobó el vencimiento.
		if err := licenseseats.ReleaseClosedSessions(ctx, tx, time.Now()); err != nil {
			return principal, err
		}
		return principal, &Fault{Status: 401, Code: "SESSION_EXPIRED", Message: "Tu sesión expiró. Inicia sesión nuevamente.", Commit: true}
	}
	principal.Permissions, err = effectivePermissions(ctx, tx, principal.UserID, principal.Support)
	if err != nil {
		return principal, err
	}
	// Etapa A1 (A1-D17): una sesión abierta con PIN solo tiene los permisos de mesero del usuario, en toda ruta y en lo
	// que informan auth/me y restaurant/state.
	if principal.PINSession {
		principal.Permissions = waiterScope(principal.Permissions)
	}
	_, err = tx.Exec(ctx, "UPDATE app.sessions SET last_seen_at=now() WHERE id=$1", principal.SessionID)
	return principal, err
}
