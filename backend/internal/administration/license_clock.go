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

// LicenseServerClock entrega la hora del servidor de licencias para la §6.3 (L1-D5): con activación, el `server_time`
// de la consulta de actualización (refresh, contractual); sin ella, el `expires_at` del desafío menos 300 s. Lo
// implementa el cliente de la Fase B. Un error que ya es un *Fault se publica tal cual; cualquier otro se informa como
// LICENSE_SERVER_UNAVAILABLE.
type LicenseServerClock interface {
	ServerTime(ctx context.Context) (time.Time, error)
}

// timeCheckTimeout limita la consulta al servidor de licencias (por debajo de los 10 s de la petición).
const timeCheckTimeout = 8 * time.Second

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
	if !request.Local {
		return nil, &Fault{Status: 403, Code: "LICENSE_LOCAL_ONLY",
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

// decodeOptional lee un cuerpo JSON que puede venir vacío.
func decodeOptional(body []byte, destination any) error {
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}
	return decode(body, destination)
}

// checkTimeWithInternet es «Comprobar la hora con internet» (§6.3, L1-D5): compara la hora del servidor de licencias
// con el reloj de la computadora principal.
//
//   - Si coinciden dentro del margen, el reloj está bien. Si la última hora confiable estaba en el futuro, se reemplaza
//     por el server_time (el único caso, con el desbloqueo de soporte, en que baja) y se audita con los dos valores. Las
//     operaciones registradas con fecha futura no se modifican: se listan en esa auditoría para que el propietario
//     decida.
//   - Si no coinciden, se rechaza con LICENSE_TIME_MISMATCH y la hora del servidor, para corregir el reloj a esa hora.
func (service *Service) checkTimeWithInternet(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	var input struct{}
	if err := decodeOptional(request.Body, &input); err != nil {
		return nil, err
	}
	if service.LicenseServerClock == nil {
		return nil, &Fault{Status: 503, Code: "LICENSE_SERVER_UNAVAILABLE",
			Message: "No se pudo comprobar la hora con internet. Revisa la conexión a internet e inténtalo de nuevo."}
	}
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
	before := manager.Clock()
	location, timezone, err := restaurantLocation(ctx, tx)
	if err != nil {
		return nil, err
	}
	check := map[string]any{
		"local_time":         before.LocalTime,
		"server_time":        serverTime,
		"last_activity_time": nullableTime(before.LastActivity),
		"margin_minutes":     int(licensing.ClockMargin / time.Minute),
	}
	if !licensing.ServerTimeMatches(before.LocalTime, serverTime) {
		check["matches"] = false
		if err := audit(ctx, tx, principal, timeCheckAction, "license", nil, nil, check, false); err != nil {
			return nil, err
		}
		difference := before.LocalTime.Sub(serverTime)
		return nil, &Fault{Status: 409, Code: "LICENSE_TIME_MISMATCH", Commit: true,
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
	check["matches"] = true
	replaced := false
	if !before.LastActivity.IsZero() && before.LastActivity.After(serverTime) {
		previous, err := manager.ReplaceTrustedTime(serverTime, licensing.ReplaceByInternetCheck)
		if err != nil {
			return nil, stateNotSaved()
		}
		replaced = true
		future, err := futureOperations(ctx, tx, serverTime.Add(licensing.ClockMargin))
		if err != nil {
			return nil, err
		}
		check["previous_last_activity_time"] = previous
		check["future_operations"] = future
		if err := audit(ctx, tx, principal, timeCorrectedAction, "license", nil,
			map[string]any{"last_activity_time": previous}, check, true); err != nil {
			return nil, err
		}
	} else if err := audit(ctx, tx, principal, timeCheckAction, "license", nil, nil, check, true); err != nil {
		return nil, err
	}
	service.wakeLicenseMonitor()
	return map[string]any{
		"matches":                     true,
		"replaced":                    replaced,
		"server_time":                 serverTime,
		"server_time_text":            licensing.ScreenTime(serverTime, location),
		"previous_last_activity_time": nullableTime(before.LastActivity),
		"clock":                       clockDetails(manager.Clock(), manager.Status(), location, timezone),
	}, nil
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
	switch {
	case errors.Is(err, licensing.ErrIdentityUnavailable):
		return nil, identityUnavailable()
	case errors.Is(err, licensing.ErrStateNotSaved):
		return nil, stateNotSaved()
	case err != nil:
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

// supportUnlock acepta la respuesta de soporte pegada como texto o importada de un archivo .desbloqueo (el escritorio
// lee el archivo y envía su texto): una sola vez, solo en esta instalación y dentro de las 24 horas de la solicitud.
// Cada intento queda en la auditoría, también los rechazados.
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
	manager := service.License
	result, err := manager.AcceptSupportUnlock(input.Response, trust)
	var rejected *licensing.SupportUnlockError
	switch {
	case errors.As(err, &rejected):
		if err := audit(ctx, tx, principal, supportUnlockRejectedAction, "license", nil, nil,
			map[string]any{"reason": rejected.Reason, "source": input.Source}, false); err != nil {
			return nil, err
		}
		return nil, &Fault{Status: 400, Code: "SUPPORT_UNLOCK_INVALID", Message: supportUnlockMessages[rejected.Reason],
			Details: map[string]any{"reason": rejected.Reason}, Commit: true}
	case errors.Is(err, licensing.ErrIdentityUnavailable):
		return nil, identityUnavailable()
	case errors.Is(err, licensing.ErrStateNotSaved):
		return nil, stateNotSaved()
	case err != nil:
		return nil, err
	}
	location, timezone, err := restaurantLocation(ctx, tx)
	if err != nil {
		return nil, err
	}
	clock := manager.Clock()
	if err := audit(ctx, tx, principal, supportUnlockedAction, "license", nil,
		map[string]any{"last_activity_time": nullableTime(result.PreviousTrustedTime)},
		map[string]any{
			"request_code": result.RequestCode, "last_activity_time": result.NewTrustedTime, "local_time": clock.LocalTime,
			"source": input.Source,
		}, true); err != nil {
		return nil, err
	}
	service.wakeLicenseMonitor()
	return map[string]any{
		"accepted":                    true,
		"previous_last_activity_time": nullableTime(result.PreviousTrustedTime),
		"last_activity_time":          result.NewTrustedTime,
		"clock":                       clockDetails(clock, manager.Status(), location, timezone),
	}, nil
}
