package administration

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/kitchen"
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
	Stopping    <-chan struct{}
	reportsOnce sync.Once
	streamMutex sync.Mutex
	streams     map[string]int
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
	Local             bool
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
	if route == "setup/status" && request.Method == "GET" {
		var configured bool
		err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.establishment)").Scan(&configured)
		return map[string]any{"configured": configured}, err
	}
	if route == "setup" && request.Method == "POST" {
		return service.setup(ctx, tx, request)
	}
	if route == "devices/enroll" && request.Method == "POST" {
		return enroll(ctx, tx, request)
	}
	if err := authenticateDevice(ctx, tx, request); err != nil {
		return nil, err
	}
	if route == "auth/login" && request.Method == "POST" {
		return login(ctx, tx, request)
	}
	principal, err := authenticateSession(ctx, tx, request)
	if err != nil {
		return nil, err
	}
	// Las denegaciones y acciones de soporte también forman parte del historial de seguridad.
	result, err := runProtected(ctx, tx, principal, request, route, func(work pgx.Tx) (any, error) {
		return service.protected(ctx, work, request, principal, route)
	})
	if err == nil && request.Method == "GET" && principal.Support {
		err = audit(ctx, tx, principal, "support.read", strings.Split(route, "/")[0], nil, nil, nil, true)
	}
	return result, err
}

func authenticateDevice(ctx context.Context, tx pgx.Tx, request Request) error {
	if !identifierPattern.MatchString(request.DeviceID) || len(request.DeviceToken) != 43 {
		return unauthorized("DEVICE_REQUIRED", "Esta terminal necesita autorización.")
	}
	var revoked bool
	err := tx.QueryRow(ctx, "SELECT revoked_at IS NOT NULL FROM app.devices WHERE id=$1 AND secret_hash=$2", request.DeviceID, TokenHash(request.DeviceToken)).Scan(&revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return unauthorized("DEVICE_REQUIRED", "Esta terminal necesita autorización.")
	}
	if err != nil {
		return err
	}
	if revoked {
		return unauthorized("DEVICE_REVOKED", "La autorización de esta terminal fue revocada.")
	}
	_, err = tx.Exec(ctx, "UPDATE app.devices SET last_seen_at=now() WHERE id=$1", request.DeviceID)
	return err
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
        s.id,s.device_id,s.revoked_at IS NOT NULL,coalesce(s.revoke_reason,''),s.expires_at,u.active
        FROM app.sessions s JOIN app.users u ON u.id=s.user_id JOIN app.roles r ON r.id=u.role_id
        WHERE s.token_hash=$1 AND s.device_id=$2`, TokenHash(request.SessionToken), request.DeviceID).Scan(
		&principal.UserID, &principal.Username, &principal.DisplayName, &principal.RoleID, &principal.RoleName,
		&principal.InitialPassword, &principal.Support, &principal.SessionID, &principal.DeviceID, &revoked, &reason, &expires, &active,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return principal, unauthorized("SESSION_INVALID", "La sesión no es válida.")
	}
	if err != nil {
		return principal, err
	}
	if revoked || !active {
		if reason == "replaced" {
			return principal, unauthorized("SESSION_REPLACED", "Tu sesión se cerró porque el usuario inició sesión en otra terminal.")
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
		return principal, &Fault{Status: 401, Code: "SESSION_EXPIRED", Message: "Tu sesión expiró. Inicia sesión nuevamente.", Commit: true}
	}
	principal.Permissions, err = effectivePermissions(ctx, tx, principal.UserID, principal.Support)
	if err != nil {
		return principal, err
	}
	_, err = tx.Exec(ctx, "UPDATE app.sessions SET last_seen_at=now() WHERE id=$1", principal.SessionID)
	return principal, err
}
