package administration

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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

// OfflineCredentialHeader es la cabecera con la credencial de envío del mesero (junto a las del dispositivo).
const OfflineCredentialHeader = "X-Offline-Credential"

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
// existe mientras el PIN está bloqueado.
type pinUser struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Initial     string     `json:"initial"`
	Color       *string    `json:"color"`
	PINVersion  int        `json:"pin_version"`
	LockedUntil *time.Time `json:"locked_until"`
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
        (SELECT a.locked_until FROM app.pin_attempts a WHERE a.user_id=u.id AND a.locked_until>now())
        FROM app.users u WHERE `+waiterEligibility+` ORDER BY lower(coalesce(u.short_name,u.display_name)),u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []pinUser{}
	for rows.Next() {
		var user pinUser
		if err := rows.Scan(&user.ID, &user.Name, &user.Color, &user.PINVersion, &user.LockedUntil); err != nil {
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
//  2. Bloqueo vigente → 429 PIN_LOCKED con details.locked_until, sin verificar el PIN.
//  3. Se verifica siempre un hash (el del usuario o uno ficticio), para no revelar por el tiempo quién tiene PIN.
//  4. Un fallo (usuario inexistente, inactivo, de soporte, sin PIN o PIN incorrecto) cuenta para el bloqueo del
//     usuario (si existe) y se registra; el quinto en 15 minutos bloquea el PIN 5 minutos (429 PIN_LOCKED) y lo
//     registra una vez.
//  5. Con el PIN correcto se borran los intentos; sin waiter_devices.use responde 403 (y se registra).
//  6. Sesión como auth/login (sustituye solo la sesión anterior de ese usuario en ese dispositivo: el cambio rápido de
//     usuario no cierra las de los demás meseros) y credencial de envío nueva (la anterior de ese usuario en ese
//     dispositivo queda revocada).
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
		lockedUntil, err := currentPINLock(ctx, tx, userID)
		if err != nil {
			return nil, err
		}
		if lockedUntil != nil {
			return failedPINLogin(ctx, tx, request.DeviceID, &userID, "PIN_LOCKED", lockedUntil)
		}
	}
	usable := exists && hash != "" && active && !support
	if !usable {
		hash = pinDummyHash
	}
	verified := VerifyPassword(input.PIN, hash)
	if !usable || !verified {
		if !exists {
			return failedPINLogin(ctx, tx, request.DeviceID, nil, "PIN_INVALID", nil)
		}
		lockedUntil, err := recordPINFailure(ctx, tx, userID, request.DeviceID)
		if err != nil {
			return nil, err
		}
		if lockedUntil != nil {
			return failedPINLogin(ctx, tx, request.DeviceID, &userID, "PIN_LOCKED", lockedUntil)
		}
		return failedPINLogin(ctx, tx, request.DeviceID, &userID, "PIN_INVALID", nil)
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
	token, err := issueSession(ctx, tx, userID, request.DeviceID, "replaced", sameDevice)
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

// currentPINLock devuelve hasta cuándo está bloqueado el PIN del usuario (nil si no lo está) y bloquea su fila de
// intentos durante la transacción, para que dos intentos simultáneos cuenten uno después del otro.
func currentPINLock(ctx context.Context, tx pgx.Tx, userID string) (*time.Time, error) {
	var lockedUntil *time.Time
	err := tx.QueryRow(ctx, `SELECT CASE WHEN locked_until>now() THEN locked_until END FROM app.pin_attempts
        WHERE user_id=$1 FOR UPDATE`, userID).Scan(&lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return lockedUntil, err
}

// recordPINFailure cuenta un fallo del usuario (5 en 15 minutos, desde cualquier dispositivo) y, al llegar al
// quinto, bloquea su PIN 5 minutos, vuelve a empezar la cuenta y lo registra una sola vez (user.pin_locked, con el
// dispositivo). Devuelve hasta cuándo quedó bloqueado, o nil.
func recordPINFailure(ctx context.Context, tx pgx.Tx, userID, deviceID string) (*time.Time, error) {
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
		return nil, err
	}
	if failures < pinMaxFailures {
		return nil, nil
	}
	var lockedUntil time.Time
	err = tx.QueryRow(ctx, `UPDATE app.pin_attempts SET failures=0,window_start=now(),locked_until=now()+interval '`+pinLockDuration+`'
        WHERE user_id=$1 RETURNING locked_until`, userID).Scan(&lockedUntil)
	if err != nil {
		return nil, err
	}
	details := map[string]any{"locked_until": lockedUntil, "failures": pinMaxFailures}
	if err := audit(ctx, tx, Principal{DeviceID: deviceID}, "user.pin_locked", "user", &userID, nil, details, true); err != nil {
		return nil, err
	}
	return &lockedUntil, nil
}

// failedPINLogin registra la entrada fallida (sin el PIN) y responde: 401 PIN_INVALID o 429 PIN_LOCKED con
// details.locked_until. Se confirma para conservar el intento y el bloqueo.
func failedPINLogin(ctx context.Context, tx pgx.Tx, deviceID string, userID *string, code string, lockedUntil *time.Time) (any, error) {
	if err := audit(ctx, tx, Principal{DeviceID: deviceID}, "session.pin_login_failed", "user", userID, nil,
		map[string]string{"reason": code}, false); err != nil {
		return nil, err
	}
	if code == "PIN_LOCKED" {
		return nil, &Fault{Status: 429, Code: code, Commit: true,
			Message: "Demasiados intentos con este PIN. Espera unos minutos o pide a un administrador que lo desbloquee.",
			Details: map[string]any{"locked_until": lockedUntil}}
	}
	return nil, &Fault{Status: 401, Code: code, Commit: true, Message: "El PIN no es correcto."}
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
// android del dispositivo y el permiso waiter_devices.use. Devuelve al mesero con sus permisos vigentes: la operación
// los vuelve a exigir como con una sesión.
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
	_, err = tx.Exec(ctx, "UPDATE app.offline_order_credentials SET last_used_at=now() WHERE id=$1", credentialID)
	return principal, err
}
