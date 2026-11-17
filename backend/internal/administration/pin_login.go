package administration

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/httpapi"
)

// Entrada con PIN en los dispositivos de meseros (etapa A1, fase B; §3.1 y decisiones A1-D1, A1-D3 y A1-D5). Estas
// rutas llevan la credencial del dispositivo y no una sesión, y solo las acepta un dispositivo emparejado de tipo
// android (las terminales de escritorio siguen con contraseña):
//
//   - GET auth/pin-users: los usuarios que pueden entrar con PIN en ese dispositivo y lo que el dispositivo necesita
//     antes de que alguien entre (su nombre y su bloqueo por inactividad).
//   - POST auth/pin-login {user_id, pin}: la sesión del mesero, como auth/login, con la versión del PIN y la
//     credencial de envío de pedidos sin conexión (se entrega una sola vez; sustituye a la anterior de ese mesero en
//     ese dispositivo).
//
// Y la credencial de envío (authenticateOfflineCredential) sustituye a la sesión solo en POST restaurant/offline-orders.

// OfflineCredentialHeader es la cabecera con la credencial de envío del mesero (junto a las del dispositivo). Es la
// misma de httpapi, que la rechaza en las rutas técnicas (revisión R3, B1).
const OfflineCredentialHeader = httpapi.OfflineCredentialHeader

// offlineOrdersRoute es la única ruta que acepta la credencial de envío.
const offlineOrdersRoute = "restaurant/offline-orders"

// waiterDevicePermission es el permiso para entrar con PIN en los dispositivos de meseros (A1-D2).
const waiterDevicePermission = "waiter_devices.use"

// requireWaiterDevice exige que el dispositivo autenticado sea un teléfono o una tableta de meseros (A1-D3).
func requireWaiterDevice(ctx context.Context, tx pgx.Tx, deviceID string) error {
	var kind string
	if err := tx.QueryRow(ctx, "SELECT kind FROM app.devices WHERE id=$1", deviceID).Scan(&kind); err != nil {
		return err
	}
	if kind != "android" {
		return &Fault{Status: 403, Code: "PIN_DEVICE_NOT_ALLOWED",
			Message: "La entrada con PIN solo está disponible en los teléfonos y tabletas de meseros. En esta terminal entra con tu usuario y contraseña."}
	}
	return nil
}

// waiterEligibility es la condición SQL de los usuarios que pueden entrar con PIN: activos, no la identidad reservada,
// con PIN y con el permiso efectivo (una excepción individual prevalece sobre el rol, como en effectivePermissions).
const waiterEligibility = `u.active AND NOT u.support AND u.pin_hash IS NOT NULL AND (
        EXISTS (SELECT FROM app.user_permissions up WHERE up.user_id=u.id AND up.permission_key='waiter_devices.use' AND up.effect='allow')
        OR (NOT EXISTS (SELECT FROM app.user_permissions up WHERE up.user_id=u.id AND up.permission_key='waiter_devices.use')
            AND EXISTS (SELECT FROM app.role_permissions rp WHERE rp.role_id=u.role_id AND rp.permission_key='waiter_devices.use')))`

// pinUser es un mesero en la pantalla de entrada del dispositivo: solo lo que el botón necesita (nada de roles ni
// permisos). name es el nombre corto o, si no tiene, el visible; color es de la paleta o nulo; locked_until solo
// existe mientras dura un bloqueo por tiempo y requires_unlock (siempre presente) dice si el PIN quedó bloqueado
// hasta que un administrador lo desbloquee (A1-D16; entonces locked_until es nulo).
type pinUser struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Initial        string     `json:"initial"`
	Color          *string    `json:"color"`
	PINVersion     int        `json:"pin_version"`
	LockedUntil    *time.Time `json:"locked_until"`
	RequiresUnlock bool       `json:"requires_unlock"`
}

// pinUsers responde GET auth/pin-users al dispositivo android sin sesión.
func pinUsers(ctx context.Context, tx pgx.Tx, request Request) (any, error) {
	if err := requireWaiterDevice(ctx, tx, request.DeviceID); err != nil {
		return nil, err
	}
	device, err := object(ctx, tx, "SELECT id,name,idle_lock_seconds FROM app.devices WHERE id=$1", request.DeviceID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT u.id,coalesce(u.short_name,u.display_name),u.badge_color,u.pin_version,
        (SELECT a.locked_until FROM app.pin_attempts a WHERE a.user_id=u.id AND a.locked_until>now()),
        coalesce((SELECT a.requires_unlock FROM app.pin_attempts a WHERE a.user_id=u.id),false)
        FROM app.users u WHERE `+waiterEligibility+` ORDER BY lower(coalesce(u.short_name,u.display_name)),u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []pinUser{}
	for rows.Next() {
		var user pinUser
		if err := rows.Scan(&user.ID, &user.Name, &user.Color, &user.PINVersion, &user.LockedUntil, &user.RequiresUnlock); err != nil {
			return nil, err
		}
		user.Initial = badgeInitial(user.Name)
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"device": device, "users": users}, nil
}

// pinLogin responde POST auth/pin-login {user_id, pin}. El orden importa:
//
//  1. Solo un dispositivo android (A1-D3) y un PIN con forma de PIN (400 si no; no cuenta como intento).
//  2. Bloqueo vigente → 429 PIN_LOCKED sin verificar el PIN: por tiempo con details.locked_until, o hasta que un
//     administrador lo desbloquee con details.locked_until nulo y details.requires_unlock (A1-D16).
//  3. Se verifica siempre un hash (el del usuario o uno ficticio), para no revelar por el tiempo quién tiene PIN.
//  4. Un fallo (usuario inexistente, inactivo, de soporte, sin PIN o PIN incorrecto) cuenta para el bloqueo del
//     usuario (si existe) y se registra; el quinto en 15 minutos bloquea el PIN 5 minutos (429 PIN_LOCKED) y lo
//     registra una vez; el tercer bloqueo seguido en 24 horas lo deja bloqueado hasta que un administrador lo
//     desbloquee (registro propio).
//  5. Con el PIN correcto se borran los intentos y los bloqueos anteriores; sin waiter_devices.use responde 403 (y se
//     registra).
//  6. Sesión como auth/login (sustituye solo la sesión anterior de ese usuario en ese dispositivo: el cambio rápido de
//     usuario no cierra las de los demás meseros), marcada como sesión con PIN (solo permisos de mesero, A1-D17), y
//     credencial de envío nueva (la anterior de ese usuario en ese dispositivo queda revocada).
func pinLogin(ctx context.Context, tx pgx.Tx, request Request) (any, error) {
	if err := requireWaiterDevice(ctx, tx, request.DeviceID); err != nil {
		return nil, err
	}
	var input struct {
		UserID string `json:"user_id"`
		PIN    string `json:"pin"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if !identifierPattern.MatchString(input.UserID) || len(input.PIN) < pinMinimumLength || len(input.PIN) > pinMaximumLength ||
		strings.Trim(input.PIN, "0123456789") != "" {
		return nil, invalid("Elige tu nombre y escribe tu PIN de 4 a 6 dígitos.")
	}
	userID := strings.ToLower(input.UserID)
	var username, hash string
	var active, support bool
	var version int
	err := tx.QueryRow(ctx, "SELECT username,coalesce(pin_hash,''),active,support,pin_version FROM app.users WHERE id=$1 FOR UPDATE",
		userID).Scan(&username, &hash, &active, &support, &version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	exists := err == nil
	if exists {
		lock, err := currentPINLock(ctx, tx, userID)
		if err != nil {
			return nil, err
		}
		if lock.active() {
			return failedPINLogin(ctx, tx, request.DeviceID, &userID, "PIN_LOCKED", lock)
		}
	}
	usable := exists && hash != "" && active && !support
	if !usable {
		hash = pinDummyHash
	}
	verified := VerifyPassword(input.PIN, hash)
	if !usable || !verified {
		if !exists {
			return failedPINLogin(ctx, tx, request.DeviceID, nil, "PIN_INVALID", pinLock{})
		}
		lock, err := recordPINFailure(ctx, tx, userID, request.DeviceID)
		if err != nil {
			return nil, err
		}
		if lock.active() {
			return failedPINLogin(ctx, tx, request.DeviceID, &userID, "PIN_LOCKED", lock)
		}
		return failedPINLogin(ctx, tx, request.DeviceID, &userID, "PIN_INVALID", pinLock{})
	}
	if _, err := tx.Exec(ctx, "DELETE FROM app.pin_attempts WHERE user_id=$1", userID); err != nil {
		return nil, err
	}
	permissions, err := effectivePermissions(ctx, tx, userID, false)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(permissions, waiterDevicePermission) {
		if err := audit(ctx, tx, Principal{DeviceID: request.DeviceID}, "session.pin_login_failed", "user", &userID, nil,
			map[string]string{"reason": "PERMISSION_DENIED"}, false); err != nil {
			return nil, err
		}
		return nil, &Fault{Status: 403, Code: "PERMISSION_DENIED", Commit: true,
			Message: "No tienes permiso para entrar con PIN en los teléfonos y tabletas de meseros."}
	}
	token, err := issueSession(ctx, tx, userID, request.DeviceID, "replaced", sameDevice, true)
	if err != nil {
		return nil, err
	}
	credential, err := issueOfflineCredential(ctx, tx, userID, request.DeviceID, version)
	if err != nil {
		return nil, err
	}
	principal := Principal{UserID: userID, Username: username, DeviceID: request.DeviceID}
	if err := audit(ctx, tx, principal, "session.pin_login", "user", &userID, nil, map[string]int{"pin_version": version}, true); err != nil {
		return nil, err
	}
	return map[string]any{"session_token": token, "pin_version": version, "offline_credential": credential}, nil
}

// pinLock es el bloqueo vigente del PIN de un usuario: por tiempo (until) o hasta que un administrador lo desbloquee
// (requiresUnlock, A1-D16).
type pinLock struct {
	until          *time.Time
	requiresUnlock bool
}

func (lock pinLock) active() bool {
	return lock.until != nil || lock.requiresUnlock
}

// currentPINLock devuelve el bloqueo vigente del PIN del usuario y bloquea su fila de intentos durante la transacción,
// para que dos intentos simultáneos cuenten uno después del otro.
func currentPINLock(ctx context.Context, tx pgx.Tx, userID string) (pinLock, error) {
	var lock pinLock
	err := tx.QueryRow(ctx, `SELECT CASE WHEN locked_until>now() THEN locked_until END,requires_unlock FROM app.pin_attempts
        WHERE user_id=$1 FOR UPDATE`, userID).Scan(&lock.until, &lock.requiresUnlock)
	if errors.Is(err, pgx.ErrNoRows) {
		return pinLock{}, nil
	}
	return lock, err
}

// recordPINFailure cuenta un fallo del usuario (5 en 15 minutos, desde cualquier dispositivo). Al quinto bloquea su PIN
// y vuelve a empezar la cuenta de fallos (al vencer el bloqueo hay otros 5 intentos). Los bloqueos se cuentan en una
// serie que empieza con el primero y dura 24 horas; un acierto la borra (borra toda la fila). Los dos primeros de la
// serie son de 5 minutos (user.pin_locked, A1-D5); el tercero deja el PIN bloqueado hasta que un administrador lo
// desbloquee (user.pin_locked_until_unlock, A1-D16). Cada bloqueo se registra una sola vez, con el dispositivo del
// quinto fallo. Devuelve el bloqueo que quedó (vacío si todavía no se bloquea).
func recordPINFailure(ctx context.Context, tx pgx.Tx, userID, deviceID string) (pinLock, error) {
	var failures int
	err := tx.QueryRow(ctx, `INSERT INTO app.pin_attempts AS a (user_id,failures,window_start,last_device_id)
        VALUES ($1,1,now(),$2)
        ON CONFLICT (user_id) DO UPDATE SET
            failures=CASE WHEN a.window_start<now()-interval '`+pinFailureWindow+`' THEN 1 ELSE a.failures+1 END,
            window_start=CASE WHEN a.window_start<now()-interval '`+pinFailureWindow+`' THEN now() ELSE a.window_start END,
            locked_until=NULL,
            last_device_id=excluded.last_device_id
        RETURNING failures`, userID, deviceID).Scan(&failures)
	if err != nil {
		return pinLock{}, err
	}
	if failures < pinMaxFailures {
		return pinLock{}, nil
	}
	// Una serie que empezó hace más de 24 horas se olvida y este bloqueo es el primero de una nueva.
	var lock pinLock
	var locks int
	err = tx.QueryRow(ctx, `UPDATE app.pin_attempts SET
            locks=CASE WHEN lock_series_started_at IS NULL OR lock_series_started_at<now()-interval '`+pinLockSeriesWindow+`'
                THEN 1 ELSE locks+1 END,
            lock_series_started_at=CASE WHEN lock_series_started_at IS NULL
                OR lock_series_started_at<now()-interval '`+pinLockSeriesWindow+`' THEN now() ELSE lock_series_started_at END
        WHERE user_id=$1 RETURNING locks`, userID).Scan(&locks)
	if err != nil {
		return pinLock{}, err
	}
	action := "user.pin_locked"
	if locks >= pinLocksBeforeUnlock {
		action = "user.pin_locked_until_unlock"
		lock.requiresUnlock = true
		_, err = tx.Exec(ctx, `UPDATE app.pin_attempts SET failures=0,window_start=now(),locked_until=NULL,requires_unlock=true
            WHERE user_id=$1`, userID)
	} else {
		var until time.Time
		err = tx.QueryRow(ctx, `UPDATE app.pin_attempts SET failures=0,window_start=now(),locked_until=now()+interval '`+pinLockDuration+`'
            WHERE user_id=$1 RETURNING locked_until`, userID).Scan(&until)
		lock.until = &until
	}
	if err != nil {
		return pinLock{}, err
	}
	details := map[string]any{"locked_until": lock.until, "requires_unlock": lock.requiresUnlock, "failures": pinMaxFailures,
		"lock_in_series": locks}
	if err := audit(ctx, tx, Principal{DeviceID: deviceID}, action, "user", &userID, nil, details, true); err != nil {
		return pinLock{}, err
	}
	return lock, nil
}

// failedPINLogin registra la entrada fallida (sin el PIN) y responde: 401 PIN_INVALID o 429 PIN_LOCKED con
// details.locked_until y details.requires_unlock (siempre las dos). Se confirma para conservar el intento y el bloqueo.
func failedPINLogin(ctx context.Context, tx pgx.Tx, deviceID string, userID *string, code string, lock pinLock) (any, error) {
	if err := audit(ctx, tx, Principal{DeviceID: deviceID}, "session.pin_login_failed", "user", userID, nil,
		map[string]string{"reason": code}, false); err != nil {
		return nil, err
	}
	if code != "PIN_LOCKED" {
		return nil, &Fault{Status: 401, Code: code, Commit: true, Message: "El PIN no es correcto."}
	}
	details := map[string]any{"locked_until": lock.until, "requires_unlock": lock.requiresUnlock}
	if lock.requiresUnlock {
		return nil, &Fault{Status: 429, Code: code, Commit: true, Details: details,
			Message: "Este PIN quedó bloqueado por demasiados intentos. Pide a un administrador que lo desbloquee en Personal › Usuarios."}
	}
	return nil, &Fault{Status: 429, Code: code, Commit: true, Details: details,
		Message: "Demasiados intentos con este PIN. Espera unos minutos o pide a un administrador que lo desbloquee."}
}

// pinLoginSlots limita cuántas entradas con PIN verifican su Argon2id a la vez en este servidor (revisión R3, O4): cada
// verificación usa 64 MiB, y un dispositivo emparejado podría mandar muchas en paralelo. Las demás esperan su turno sin
// haber abierto transacción; si no lo obtienen a tiempo, responden 503 PIN_LOGIN_BUSY.
var pinLoginSlots = make(chan struct{}, 4)

// pinLoginWait es cuánto espera una entrada con PIN su turno.
const pinLoginWait = 5 * time.Second

// pinLoginRequest dice si la petición es una entrada con PIN.
func pinLoginRequest(request Request) bool {
	return request.Method == "POST" && strings.TrimPrefix(request.Path, "/api/v1/") == "auth/pin-login"
}

// reservePINCheck toma un turno de verificación y devuelve cómo soltarlo.
func reservePINCheck(ctx context.Context) (func(), error) {
	wait, cancel := context.WithTimeout(ctx, pinLoginWait)
	defer cancel()
	select {
	case pinLoginSlots <- struct{}{}:
		return func() {
			<-pinLoginSlots
		}, nil
	case <-wait.Done():
		return nil, &Fault{Status: 503, Code: "PIN_LOGIN_BUSY",
			Message: "Hay muchas entradas con PIN al mismo tiempo. Intenta de nuevo en unos segundos."}
	}
}

// issueOfflineCredential crea la credencial de envío del mesero en el dispositivo (A1-D1) y revoca la anterior. Se
// entrega una sola vez; en la base solo queda su SHA-256 con la versión del PIN con que se emitió.
func issueOfflineCredential(ctx context.Context, tx pgx.Tx, userID, deviceID string, pinVersion int) (string, error) {
	if _, err := tx.Exec(ctx, `UPDATE app.offline_order_credentials SET revoked_at=now(),revoke_reason='replaced'
        WHERE user_id=$1 AND device_id=$2 AND revoked_at IS NULL`, userID, deviceID); err != nil {
		return "", err
	}
	credential := NewToken()
	_, err := tx.Exec(ctx, `INSERT INTO app.offline_order_credentials(user_id,device_id,secret_hash,pin_version)
        VALUES ($1,$2,$3,$4)`, userID, deviceID, TokenHash(credential), pinVersion)
	return credential, err
}

// offlineCredentialInvalid es la respuesta a una credencial de envío que ya no vale: el dispositivo conserva los
// pedidos y pide al mesero que entre con su PIN con conexión (eso emite una credencial nueva).
func offlineCredentialInvalid() error {
	return unauthorized("OFFLINE_CREDENTIAL_INVALID",
		"La autorización para enviar los pedidos de este mesero ya no es válida. Que el mesero entre con su PIN para enviarlos.")
}

// authenticateOfflineCredential identifica al mesero por su credencial de envío en lugar de una sesión (A1-D1). El
// dispositivo ya se autenticó (no revocado). Revisa en cada petición, sin caché: la credencial del mismo dispositivo y
// no revocada, el usuario activo y no de soporte, su PIN vigente con la misma pin_version con que se emitió, el tipo
// android del dispositivo y el permiso waiter_devices.use. Devuelve al mesero con sus permisos vigentes de mesero
// (solo los de la lista cerrada de waiter_scope.go, A1-D17): la operación los vuelve a exigir como con una sesión.
func authenticateOfflineCredential(ctx context.Context, tx pgx.Tx, request Request) (Principal, error) {
	var principal Principal
	if len(request.OfflineCredential) != 43 {
		return principal, offlineCredentialInvalid()
	}
	var credentialID, kind string
	var revoked, hasPIN bool
	var issuedVersion, currentVersion int
	err := tx.QueryRow(ctx, `SELECT c.id,c.revoked_at IS NOT NULL,c.pin_version,u.id,u.username,u.display_name,u.role_id,r.name,
        u.initial_password,u.support,u.active,u.pin_hash IS NOT NULL,u.pin_version,d.kind
        FROM app.offline_order_credentials c JOIN app.users u ON u.id=c.user_id JOIN app.roles r ON r.id=u.role_id
        JOIN app.devices d ON d.id=c.device_id
        WHERE c.secret_hash=$1 AND c.device_id=$2`, TokenHash(request.OfflineCredential), request.DeviceID).Scan(
		&credentialID, &revoked, &issuedVersion, &principal.UserID, &principal.Username, &principal.DisplayName,
		&principal.RoleID, &principal.RoleName, &principal.InitialPassword, &principal.Support, &principal.Active, &hasPIN,
		&currentVersion, &kind,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return principal, offlineCredentialInvalid()
	}
	if err != nil {
		return principal, err
	}
	if revoked || !principal.Active || principal.Support || !hasPIN || issuedVersion != currentVersion || kind != "android" {
		return principal, offlineCredentialInvalid()
	}
	principal.DeviceID = request.DeviceID
	principal.Permissions, err = effectivePermissions(ctx, tx, principal.UserID, false)
	if err != nil {
		return principal, err
	}
	if !slices.Contains(principal.Permissions, waiterDevicePermission) {
		return principal, offlineCredentialInvalid()
	}
	principal.Permissions = waiterScope(principal.Permissions)
	principal.PINSession = true
	_, err = tx.Exec(ctx, "UPDATE app.offline_order_credentials SET last_used_at=now() WHERE id=$1", credentialID)
	return principal, err
}
