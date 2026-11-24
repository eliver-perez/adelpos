package administration

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/licensing"
)

// Rutas de la protección de la hora (etapa L1, Fase C; §6.3 y §6.4 de la especificación, decisiones L1-D5 y L1-D19):
//
//   - POST license/time-check: «Comprobar la hora con internet».
//   - POST license/support-request: el código de solicitud del desbloqueo de soporte.
//   - POST license/support-unlock: pegar o importar la respuesta de soporte.
//
// Son de la pantalla «Sistema detenido por la fecha y hora de la computadora», que se muestra en la computadora
// principal: solo se aceptan por la escucha local (request.Local) y con una sesión de cualquier usuario, sin permiso
// especial, porque quien esté en la caja debe poder resolverlo. La puerta las deja pasar en todos los estados y con la
// hora incorrecta (RowLicense). Las demás rutas license/* (activar, transferir, consultar…) son de la Fase B.
//
// Revisión R2, hallazgo R2-1: las dos formas de bajar la última hora confiable (la §6.3 y la §6.4) corren después de
// confirmar la petición (completeDeferred) y siguen el mismo orden: se comprueba y se prepara todo sin cambiar nada; la
// auditoría con los dos valores se confirma en su propia transacción corta; solo entonces se baja la hora, en sus dos
// copias (R2-3). Si la auditoría no se puede confirmar, no se baja nada (y la respuesta de soporte no se gasta). Si la
// baja falla después, se audita otra vez la misma acción con success=false y applied=false, y se responde con error: la
// hora no cambió. Lo único que puede quedar es una auditoría de una baja que no llegó a aplicarse si el servidor se
// apaga justo en medio; nunca una baja sin su auditoría.

// LicenseServerClock entrega la hora del servidor de licencias para la §6.3 (L1-D5): con activación, el `server_time`
// de la consulta de actualización (refresh, contractual); sin ella, el `expires_at` del desafío menos 300 s. Lo
// implementa el cliente de la Fase B. Un error que ya es un *Fault se publica tal cual; cualquier otro se informa como
// LICENSE_SERVER_UNAVAILABLE.
type LicenseServerClock interface {
	ServerTime(ctx context.Context) (time.Time, error)
}

// timeCheckTimeout limita la consulta al servidor de licencias: el cliente de la Fase B hace el desafío y la consulta,
// con sus reintentos, dentro del plazo de las rutas de la licencia (licenseTimeout).
const timeCheckTimeout = 60 * time.Second

// clockAuditTimeout limita cada transacción corta de la auditoría de estas rutas.
const clockAuditTimeout = 10 * time.Second

// futureOperationsLimit es cuántos identificadores de cada tipo se listan en la auditoría de la §6.3.
const futureOperationsLimit = 20

// Acciones de la auditoría de estas rutas.
const (
	timeCheckAction             = "license.clock_internet_check"
	timeCorrectedAction         = "license.clock_corrected"
	supportRequestedAction      = "license.support_requested"
	supportUnlockedAction       = "license.support_unlocked"
	supportUnlockRejectedAction = "license.support_unlock_rejected"
)

// licenseClockRoute indica si la ruta es de este archivo.
func licenseClockRoute(route string) bool {
	return route == "license/time-check" || route == "license/support-request" || route == "license/support-unlock"
}

func (service *Service) licenseClockRoutes(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string) (any, error) {
	if request.Method != "POST" {
		return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
	}
	// Revisión R2, hallazgo R2-6: desde otra terminal no es un problema de permisos, así que es un 409, como los rechazos
	// de la puerta: un 403 dentro de runProtected quedaría en la auditoría como un acceso denegado.
	if !request.Local {
		return nil, &Fault{Status: 409, Code: "LICENSE_LOCAL_ONLY",
			Message: "Esto se hace en la computadora principal, en la pantalla de la fecha y hora."}
	}
	if service.License == nil {
		return nil, identityUnavailable()
	}
	switch route {
	case "license/time-check":
		return service.checkTimeWithInternet(ctx, tx, request, principal)
	case "license/support-request":
		return service.supportRequest(ctx, tx, request, principal)
	}
	return service.supportUnlock(ctx, tx, request, principal)
}

func identityUnavailable() error {
	return &Fault{Status: 503, Code: "IDENTITY_UNAVAILABLE",
		Message: "La identidad de este servidor no está disponible. Reinicia el servidor; si sigue igual, comunícate con soporte."}
}

func stateNotSaved() error {
	return &Fault{Status: 503, Code: "LICENSE_STATE_NOT_SAVED",
		Message: "No se pudo guardar el cambio en esta computadora. Inténtalo de nuevo; si sigue igual, comunícate con soporte."}
}

// supportInstallationFault es el error de una instalación que no puede pedir ni aceptar un desbloqueo de soporte
// (revisión R2, hallazgo R2-5), o nil si el error es otro. Una carpeta de datos copiada de otra computadora tiene la
// misma identidad y la misma solicitud pendiente que la original: primero se crea una identidad nueva, como para activar.
func supportInstallationFault(err error) *Fault {
	switch {
	case errors.Is(err, licensing.ErrIdentityUnavailable):
		return identityUnavailable().(*Fault)
	case errors.Is(err, licensing.ErrCopiedInstallation):
		return &Fault{Status: 409, Code: "LICENSE_INSTALLATION_COPIED",
			Message: "Esta instalación se copió de otra computadora. Crea una identidad nueva para esta computadora en la " +
				"pantalla de la licencia y después pide el código de desbloqueo."}
	case errors.Is(err, licensing.ErrMachineUnverified):
		return &Fault{Status: 503, Code: "IDENTITY_UNAVAILABLE",
			Message: "No se pudo comprobar esta computadora. Reinicia el servidor; si sigue igual, comunícate con soporte.",
			Details: map[string]string{"reason": "machine_unverified"}}
	case errors.Is(err, licensing.ErrStateNotSaved):
		return stateNotSaved().(*Fault)
	}
	return nil
}

// clockAudit escribe una auditoría de estas rutas en su propia transacción corta y la confirma. Devuelve la zona horaria
// del restaurante, para la respuesta.
func (service *Service) clockAudit(ctx context.Context, principal Principal, action string, before, after any,
	success bool) (*time.Location, string, error) {
	tx, err := service.Pool.Begin(ctx)
	if err != nil {
		return time.UTC, "UTC", err
	}
	defer tx.Rollback(context.Background())
	location, timezone, err := restaurantLocation(ctx, tx)
	if err != nil {
		return time.UTC, "UTC", err
	}
	if err := audit(ctx, tx, principal, action, "license", nil, before, after, success); err != nil {
		return time.UTC, "UTC", err
	}
	if err := tx.Commit(ctx); err != nil {
		return time.UTC, "UTC", err
	}
	return location, timezone, nil
}

// auditNotApplied registra que una baja de la hora ya auditada no se aplicó (R2-1): la misma acción, con success=false,
// applied=false y el motivo. Corre con su propio plazo, aunque el de la petición ya se haya agotado; si tampoco se puede
// escribir, solo queda en la bitácora.
func (service *Service) auditNotApplied(ctx context.Context, principal Principal, action string, before any,
	after map[string]any, reason string) {
	details := make(map[string]any, len(after)+2)
	for key, value := range after {
		details[key] = value
	}
	details["applied"] = false
	details["reason"] = reason
	auditContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), clockAuditTimeout)
	defer cancel()
	if _, _, err := service.clockAudit(auditContext, principal, action, before, details, false); err != nil && service.Logger != nil {
		service.Logger.Error("license_clock_not_applied_unaudited", "action", action, "reason", err.Error())
	}
}

// decodeOptional lee un cuerpo JSON que puede venir vacío.
func decodeOptional(body []byte, destination any) error {
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}
	return decode(body, destination)
}

// timeCheck es «Comprobar la hora con internet» ya autorizada: la consulta al servidor de licencias corre después de
// confirmar la autenticación (completeDeferred), fuera de la transacción, como las operaciones en línea de la Fase B: la
// red nunca retiene una conexión de la base ni un lock.
type timeCheck struct {
	principal Principal
}

// checkTimeWithInternet autoriza «Comprobar la hora con internet» (§6.3, L1-D5); la comprobación la hace
// completeTimeCheck.
func (service *Service) checkTimeWithInternet(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	var input struct{}
	if err := decodeOptional(request.Body, &input); err != nil {
		return nil, err
	}
	if service.LicenseServerClock == nil {
		return nil, &Fault{Status: 503, Code: "LICENSE_SERVER_UNAVAILABLE",
			Message: "No se pudo comprobar la hora con internet. Revisa la conexión a internet e inténtalo de nuevo."}
	}
	return &timeCheck{principal: principal}, nil
}

// completeTimeCheck compara la hora del servidor de licencias con el reloj de la computadora principal:
//
//   - Si coinciden dentro del margen, el reloj está bien. Si la última hora confiable estaba en el futuro, se reemplaza
//     por el server_time (el único caso, con el desbloqueo de soporte, en que baja) y se audita con los dos valores. Las
//     operaciones registradas con fecha futura no se modifican: se listan en esa auditoría para que el propietario
//     decida.
//   - Si no coinciden, se rechaza con LICENSE_TIME_MISMATCH y la hora del servidor, para corregir el reloj a esa hora.
//
// La auditoría va en una transacción corta aparte, después de la consulta. Cuando la hora baja, esa auditoría se
// confirma antes de bajarla (R2-1, ver arriba).
func (service *Service) completeTimeCheck(ctx context.Context, check *timeCheck) (any, error) {
	serverContext, cancel := context.WithTimeout(ctx, timeCheckTimeout)
	serverTime, err := service.LicenseServerClock.ServerTime(serverContext)
	cancel()
	if err != nil {
		var fault *Fault
		if errors.As(err, &fault) {
			return nil, fault
		}
		return nil, &Fault{Status: 503, Code: "LICENSE_SERVER_UNAVAILABLE",
			Message: "No se pudo comprobar la hora con internet. Revisa la conexión a internet e inténtalo de nuevo."}
	}
	serverTime = serverTime.UTC()
	manager := service.License
	// El server_time no va firmado (solo lo protege el TLS). Como piso, nunca puede ser anterior a la emisión de la
	// licencia vigente, que sí está firmada por el mismo servidor: una respuesta así no se usa para bajar la hora.
	if status := manager.Status(); status.HasLicense() && serverTime.Before(status.IssuedAt.Add(-licensing.ClockMargin)) {
		return nil, &Fault{Status: 502, Code: "LICENSE_RESPONSE_INVALID",
			Message: "La hora que respondió el servidor de licencias no es válida. Inténtalo más tarde o comunícate con soporte."}
	}
	// Antes de preparar una posible baja se guarda la última hora confiable que está en memoria, en sus dos copias: el
	// valor anterior que registra la auditoría es el que queda guardado si la baja no se aplica (R2-1). Si no se puede
	// guardar, el trabajador lo reintenta cada minuto, y la baja tampoco se podría guardar.
	_ = manager.Flush()
	before := manager.Clock()
	auditContext, cancelAudit := context.WithTimeout(context.WithoutCancel(ctx), clockAuditTimeout)
	defer cancelAudit()
	tx, err := service.Pool.Begin(auditContext)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	location, timezone, err := restaurantLocation(auditContext, tx)
	if err != nil {
		return nil, err
	}
	details := map[string]any{
		"local_time":         before.LocalTime,
		"server_time":        serverTime,
		"last_activity_time": nullableTime(before.LastActivity),
		"margin_minutes":     int(licensing.ClockMargin / time.Minute),
	}
	if !licensing.ServerTimeMatches(before.LocalTime, serverTime) {
		details["matches"] = false
		if err := audit(auditContext, tx, check.principal, timeCheckAction, "license", nil, nil, details, false); err != nil {
			return nil, err
		}
		if err := tx.Commit(auditContext); err != nil {
			return nil, err
		}
		difference := before.LocalTime.Sub(serverTime)
		return nil, &Fault{Status: 409, Code: "LICENSE_TIME_MISMATCH",
			Message: "La fecha y hora de esta computadora no coinciden con la de internet. Corrígela a " +
				licensing.ScreenTime(serverTime, location) + " y vuelve a comprobar.",
			Details: map[string]any{
				"server_time":        serverTime,
				"local_time":         before.LocalTime,
				"server_time_text":   licensing.ScreenTime(serverTime, location),
				"local_time_text":    licensing.ScreenTime(before.LocalTime, location),
				"difference_minutes": int(difference / time.Minute),
				"timezone":           timezone,
			}}
	}
	details["matches"] = true
	replaced := !before.LastActivity.IsZero() && before.LastActivity.After(serverTime)
	if replaced {
		// La baja (R2-1): primero las operaciones con fecha futura y la auditoría con los dos valores, confirmadas; después
		// la hora, en sus dos copias. Si la auditoría no se confirma, no se baja nada.
		future, err := futureOperations(auditContext, tx, serverTime.Add(licensing.ClockMargin))
		if err != nil {
			return nil, err
		}
		previous := map[string]any{"last_activity_time": before.LastActivity}
		details["previous_last_activity_time"] = before.LastActivity
		details["future_operations"] = future
		if err := audit(auditContext, tx, check.principal, timeCorrectedAction, "license", nil, previous, details, true); err != nil {
			return nil, err
		}
		if err := tx.Commit(auditContext); err != nil {
			return nil, err
		}
		if _, err := manager.ReplaceTrustedTime(serverTime, licensing.ReplaceByInternetCheck); err != nil {
			service.auditNotApplied(ctx, check.principal, timeCorrectedAction, previous, details, "not_saved")
			return nil, stateNotSaved()
		}
	} else {
		if err := audit(auditContext, tx, check.principal, timeCheckAction, "license", nil, nil, details, true); err != nil {
			return nil, err
		}
		if err := tx.Commit(auditContext); err != nil {
			return nil, err
		}
	}
	// Las terminales se enteran en el momento (el vigilante también lo vería en su siguiente revisión).
	service.notifyLicense(ctx)
	return map[string]any{
		"matches":                     true,
		"replaced":                    replaced,
		"server_time":                 serverTime,
		"server_time_text":            licensing.ScreenTime(serverTime, location),
		"previous_last_activity_time": nullableTime(before.LastActivity),
		"clock":                       clockDetails(manager.Clock(), manager.Status(), location, timezone),
	}, nil
}

// licenseClientClock adapta el cliente del servidor de licencias de la Fase B (licensing.Client.ServerTime: refresh
// con activación; sin ella, el desafío, L1-D5) a LicenseServerClock, con los códigos de error de la Fase B
// (licenseFault).
type licenseClientClock struct {
	client *licensing.Client
}

// LicenseClientClock es la hora del servidor de licencias para la §6.3, a partir del cliente de la Fase B (serveHTTP).
func LicenseClientClock(client *licensing.Client) LicenseServerClock {
	return licenseClientClock{client: client}
}

func (clock licenseClientClock) ServerTime(ctx context.Context) (time.Time, error) {
	if clock.client == nil {
		return time.Time{}, licenseFault(&licensing.TransportError{Reason: licensing.ReasonNotConfigured})
	}
	value, err := clock.client.ServerTime(ctx)
	if err != nil {
		return time.Time{}, licenseFault(err)
	}
	return value.Time, nil
}

// licenseNotice es una respuesta de una ruta de la licencia que cambió el estado dentro de la transacción (importar un
// .lic, la identidad nueva): después de confirmarla, completeDeferred avisa a las terminales (notifyLicense) y entrega
// la respuesta.
type licenseNotice struct {
	result any
}

// notifyLicense revisa la licencia y la hora en el momento y publica license.changed si algo cambió (CheckLicense), en
// su propia transacción. Se llama siempre fuera de la transacción de una petición. Un fallo solo queda en la bitácora:
// el vigilante lo vuelve a intentar en su siguiente revisión.
func (service *Service) notifyLicense(ctx context.Context) {
	if err := service.CheckLicense(context.WithoutCancel(ctx)); err != nil && service.Logger != nil {
		service.Logger.Warn("license_notice_failed", "reason", err.Error())
	}
}

func nullableTime(instant time.Time) *time.Time {
	if instant.IsZero() {
		return nil
	}
	return &instant
}

// futureOperation es un tipo de registro con fecha posterior a la hora corregida.
type futureOperation struct {
	Kind  string     `json:"kind"`
	Count int        `json:"count"`
	First *time.Time `json:"first"`
	Last  *time.Time `json:"last"`
	IDs   []string   `json:"ids"`
}

// futureOperationsQuery lista, sin modificarlos, los registros de operación con fecha posterior a $1 (la hora
// corregida más el margen): cuentas, pedidos, cobros, turnos (abiertos o cerrados) y movimientos de inventario, con
// hasta $2 identificadores de cada tipo.
const futureOperationsQuery = `WITH future AS (
        SELECT 'accounts' AS kind, id, created_at AS recorded_at FROM app.restaurant_accounts WHERE created_at > $1
        UNION ALL SELECT 'orders', id, created_at FROM app.restaurant_orders WHERE created_at > $1
        UNION ALL SELECT 'payments', id, created_at FROM app.payments WHERE created_at > $1
        UNION ALL SELECT 'cash_shifts', id, greatest(opened_at, coalesce(closed_at, opened_at)) FROM app.cash_shifts
            WHERE opened_at > $1 OR closed_at > $1
        UNION ALL SELECT 'inventory_movements', id, recorded_at FROM app.inventory_movements WHERE recorded_at > $1
    )
    SELECT kind, count(*)::int, min(recorded_at), max(recorded_at),
        (array_agg(id::text ORDER BY recorded_at, id))[1:$2::int]
    FROM future GROUP BY kind ORDER BY kind`

func futureOperations(ctx context.Context, tx pgx.Tx, after time.Time) ([]futureOperation, error) {
	rows, err := tx.Query(ctx, futureOperationsQuery, after, futureOperationsLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	listed := []futureOperation{}
	for rows.Next() {
		var item futureOperation
		var first, last time.Time
		if err := rows.Scan(&item.Kind, &item.Count, &first, &last, &item.IDs); err != nil {
			return nil, err
		}
		item.First, item.Last = &first, &last
		listed = append(listed, item)
	}
	return listed, rows.Err()
}

// supportRequest entrega el código de solicitud del desbloqueo de soporte (§6.4): el pendiente, si sigue vigente, o
// uno nuevo (también con {"renew": true}), que reemplaza al anterior.
func (service *Service) supportRequest(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	var input struct {
		Renew bool `json:"renew"`
	}
	if err := decodeOptional(request.Body, &input); err != nil {
		return nil, err
	}
	info, err := service.License.SupportRequest(input.Renew)
	if fault := supportInstallationFault(err); fault != nil {
		return nil, fault
	}
	if err != nil {
		return nil, err
	}
	location, timezone, err := restaurantLocation(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !info.Reused {
		if err := audit(ctx, tx, principal, supportRequestedAction, "license", nil, nil, map[string]any{
			"request_code": info.Code, "created_at": info.CreatedAt, "last_activity_time": nullableTime(info.TrustedTime),
		}, true); err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"request_code":            info.Code,
		"created_at":              info.CreatedAt,
		"expires_at":              info.ExpiresAt,
		"expires_at_text":         licensing.ScreenTime(info.ExpiresAt, location),
		"last_activity_time":      nullableTime(info.TrustedTime),
		"last_activity_time_text": licensing.ScreenTime(info.TrustedTime, location),
		"timezone":                timezone,
		"reused":                  info.Reused,
	}, nil
}

// supportUnlockMessages son los mensajes de un desbloqueo rechazado, por motivo (SUPPORT_UNLOCK_INVALID).
var supportUnlockMessages = map[string]string{
	licensing.SupportReasonUsed:              "Esta respuesta de soporte ya se usó. Pide un código nuevo y envíalo a soporte.",
	licensing.SupportReasonOtherInstallation: "Esta respuesta de soporte es de otra computadora.",
	licensing.SupportReasonExpired:           "La solicitud venció: vale 24 horas. Pide un código nuevo y envíalo a soporte.",
	licensing.SupportReasonAltered:           "La respuesta de soporte no es válida. Revisa que se haya copiado completa.",
	licensing.SupportReasonNotRequested:      "Esta respuesta no corresponde al código de esta computadora. Pide un código nuevo y envíalo a soporte.",
}

// supportTrust son las llaves de soporte en que confía el servidor: las compiladas, o las que inyecta una prueba.
func (service *Service) supportTrust() (licensing.SupportTrust, error) {
	if service.SupportTrust != nil {
		return *service.SupportTrust, nil
	}
	return licensing.CompiledSupportTrust()
}

// supportUnlockAttempt es un desbloqueo de soporte ya autorizado: se comprueba, se audita y se aplica después de
// confirmar la petición (completeSupportUnlock), con su auditoría antes de bajar la hora (R2-1).
type supportUnlockAttempt struct {
	principal Principal
	response  string
	source    string
	trust     licensing.SupportTrust
}

// supportUnlock autoriza la respuesta de soporte pegada como texto o importada de un archivo .desbloqueo (el escritorio
// lee el archivo y envía su texto). La comprobación y el desbloqueo los hace completeSupportUnlock.
func (service *Service) supportUnlock(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	var input struct {
		Response string `json:"response"`
		Source   string `json:"source"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if input.Source == "" {
		input.Source = "text"
	}
	if input.Source != "text" && input.Source != "file" {
		return nil, invalid("Indica si la respuesta se pegó como texto o se importó de un archivo.")
	}
	if strings.TrimSpace(input.Response) == "" {
		return nil, invalid("Pega la respuesta de soporte.")
	}
	trust, err := service.supportTrust()
	if err != nil {
		return nil, &Fault{Status: 503, Code: "SUPPORT_UNLOCK_UNAVAILABLE",
			Message: "El desbloqueo de soporte no está disponible en esta versión. Comunícate con soporte."}
	}
	return &supportUnlockAttempt{principal: principal, response: input.Response, source: input.Source, trust: trust}, nil
}

// completeSupportUnlock acepta la respuesta: una sola vez, solo en esta instalación y dentro de las 24 horas de la
// solicitud. Cada intento queda en la auditoría, también los rechazados. Primero se comprueba sin cambiar nada; un
// rechazo se audita y no gasta nada. Una respuesta válida se audita (license.support_unlocked, con los dos valores) y
// solo después se aplica: si la auditoría no se confirma, la respuesta sigue sin usar y se puede volver a pegar (R2-1).
func (service *Service) completeSupportUnlock(ctx context.Context, attempt *supportUnlockAttempt) (any, error) {
	manager := service.License
	auditContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), clockAuditTimeout)
	defer cancel()
	result, err := manager.CheckSupportUnlock(attempt.response, attempt.trust)
	if err != nil {
		return nil, service.supportUnlockFailed(auditContext, attempt, err)
	}
	clock := manager.Clock()
	previous := map[string]any{"last_activity_time": nullableTime(result.PreviousTrustedTime)}
	details := map[string]any{
		"request_code": result.RequestCode, "last_activity_time": result.NewTrustedTime, "local_time": clock.LocalTime,
		"source": attempt.source,
	}
	location, timezone, err := service.clockAudit(auditContext, attempt.principal, supportUnlockedAction, previous, details, true)
	if err != nil {
		return nil, err
	}
	applied, err := manager.AcceptSupportUnlock(attempt.response, attempt.trust)
	if err != nil {
		// Ya auditado y no aplicado: por ejemplo, no se pudo guardar, o la solicitud cambió en medio. El número de un solo
		// uso no se gastó (salvo que otro intento lo haya usado justo antes).
		reason := "not_saved"
		var rejected *licensing.SupportUnlockError
		if errors.As(err, &rejected) {
			reason = rejected.Reason
		}
		service.auditNotApplied(ctx, attempt.principal, supportUnlockedAction, previous, details, reason)
		return nil, supportUnlockFault(err)
	}
	service.wakeLicenseMonitor()
	return map[string]any{
		"accepted":                    true,
		"previous_last_activity_time": nullableTime(applied.PreviousTrustedTime),
		"last_activity_time":          applied.NewTrustedTime,
		"clock":                       clockDetails(manager.Clock(), manager.Status(), location, timezone),
	}, nil
}

// supportUnlockFailed audita un desbloqueo rechazado antes de aplicarse (license.support_unlock_rejected, con el
// motivo) y devuelve su error. Si la auditoría no se puede escribir, el error es ese: no se cambió nada.
func (service *Service) supportUnlockFailed(ctx context.Context, attempt *supportUnlockAttempt, err error) error {
	reason := ""
	var rejected *licensing.SupportUnlockError
	switch {
	case errors.As(err, &rejected):
		reason = rejected.Reason
	case errors.Is(err, licensing.ErrCopiedInstallation):
		reason = "installation_copied"
	case errors.Is(err, licensing.ErrMachineUnverified):
		reason = "machine_unverified"
	}
	if reason != "" {
		if _, _, auditErr := service.clockAudit(ctx, attempt.principal, supportUnlockRejectedAction, nil,
			map[string]any{"reason": reason, "source": attempt.source}, false); auditErr != nil {
			return auditErr
		}
	}
	return supportUnlockFault(err)
}

// supportUnlockFault es el error público de un desbloqueo que no se aplicó.
func supportUnlockFault(err error) error {
	var rejected *licensing.SupportUnlockError
	if errors.As(err, &rejected) {
		return &Fault{Status: 400, Code: "SUPPORT_UNLOCK_INVALID", Message: supportUnlockMessages[rejected.Reason],
			Details: map[string]any{"reason": rejected.Reason}}
	}
	if fault := supportInstallationFault(err); fault != nil {
		return fault
	}
	return err
}
