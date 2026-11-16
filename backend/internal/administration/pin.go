package administration

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/catalog"
)

// PIN de los meseros (etapa A1, fase B; §3.1 de la especificación y decisiones A1-D1 a A1-D5). Cada usuario puede tener
// un PIN de 4 a 6 dígitos que solo sirve para entrar en los teléfonos y tabletas de meseros emparejados
// (pin_login.go). Lo asigna un usuario con users.manage en Personal › Usuarios (con el nombre corto y el color del
// distintivo) o lo define el propio usuario en «Mi PIN» con su contraseña actual. En ambos casos se aplican las mismas
// reglas (validatePIN) y se guarda con el hash de las contraseñas (Argon2id). Cambiar o quitar el PIN sube pin_version
// y cierra las sesiones del usuario en dispositivos android y su credencial de envío: el verificador local del
// dispositivo deja de valer porque la lista de usuarios trae otra versión.

// Las reglas del PIN: longitud, intentos y bloqueo (A1-D5).
const (
	pinMinimumLength = 4
	pinMaximumLength = 6
	pinMaxFailures   = 5
	pinFailureWindow = "15 minutes"
	pinLockDuration  = "5 minutes"
	shortNameLength  = 20
)

// pinDummyHash se verifica cuando el usuario no existe o no tiene PIN, para que el tiempo de respuesta no revele
// quién tiene PIN (como dummyHash en el inicio de sesión con contraseña).
var pinDummyHash = HashPassword(NewToken())

// BadgePalette es la lista cerrada de colores del distintivo de cada mesero en la pantalla de entrada del dispositivo.
// Es la misma paleta de las categorías del catálogo (catalog.Palette: cada color contrasta al menos 3:1 con el panel
// claro y con el oscuro). La interfaz la recibe de Go (GET users/badge-colors) y nunca escribe un color libre.
var BadgePalette = catalog.Palette

// commonPINs son PIN muy usados que no caen en las reglas generales (todos iguales, secuencias y patrones
// repetidos): los primeros lugares de los análisis públicos de PIN filtrados (fechas de 4 cifras muy repetidas, la
// columna central y las diagonales del teclado numérico, pares y nones, «LOVE» en el teclado telefónico) y sus
// equivalentes de 5 y 6 dígitos. Lista corta a propósito: el objetivo es descartar lo primero que alguien probaría.
var commonPINs = []string{
	// 4 dígitos.
	"1004", "2000", "2001", "1122", "6969", "1100", "2580", "0852", "1470", "0741", "3690", "0963",
	"1357", "2468", "7531", "8642", "5683", "0007", "1998", "1999", "1990", "2010", "2020", "1369", "1590",
	// 5 dígitos.
	"12321", "13579", "24680", "11223", "00700",
	// 6 dígitos.
	"112233", "159753", "147258", "258369", "369258", "123321", "159357", "147852", "789456", "112358",
	"102030", "100200", "998877", "111222", "222333", "123654", "741852", "963852", "147369", "520520",
}

// validatePIN aplica las mismas reglas en todos los lugares donde se define un PIN: de 4 a 6 dígitos y nunca uno
// trivial (todos los dígitos iguales, una secuencia ascendente o descendente —también la que da la vuelta, como
// 7890—, un bloque repetido —1212, 121212, 123123— o uno de la lista de muy usados).
func validatePIN(pin string) error {
	if len(pin) < pinMinimumLength || len(pin) > pinMaximumLength {
		return invalid("El PIN debe tener de 4 a 6 dígitos.")
	}
	for _, character := range pin {
		if character < '0' || character > '9' {
			return invalid("El PIN debe tener de 4 a 6 dígitos.")
		}
	}
	if trivialPIN(pin) {
		return &Fault{Status: 400, Code: "PIN_TOO_SIMPLE",
			Message: "Ese PIN es muy fácil de adivinar (números iguales, seguidos o repetidos, o de los más usados). Elige otro."}
	}
	return nil
}

// trivialPIN dice si un PIN de solo dígitos es fácil de adivinar.
func trivialPIN(pin string) bool {
	return sequentialPIN(pin, 0) || sequentialPIN(pin, 1) || sequentialPIN(pin, 9) || repeatedBlockPIN(pin) ||
		slices.Contains(commonPINs, pin)
}

// sequentialPIN dice si cada dígito es el anterior más step, módulo 10: step 0 son todos iguales (0000), step 1 una
// secuencia ascendente (1234, 7890) y step 9 una descendente (4321, 0987).
func sequentialPIN(pin string, step int) bool {
	for index := 1; index < len(pin); index++ {
		previous := int(pin[index-1] - '0')
		current := int(pin[index] - '0')
		if current != (previous+step)%10 {
			return false
		}
	}
	return true
}

// repeatedBlockPIN dice si el PIN es un bloque de 2 o 3 dígitos repetido completo (1212, 121212, 123123).
func repeatedBlockPIN(pin string) bool {
	for _, period := range []int{2, 3} {
		if len(pin)%period != 0 || len(pin) == period {
			continue
		}
		repeated := true
		for index := period; index < len(pin); index++ {
			if pin[index] != pin[index-period] {
				repeated = false
				break
			}
		}
		if repeated {
			return true
		}
	}
	return false
}

// normalizeBadge valida el nombre corto y el color del distintivo. Un nombre vacío se guarda como NULL (el
// dispositivo usa el nombre visible); el color debe ser uno de la paleta (en su forma canónica) o ninguno.
func normalizeBadge(shortName, color string) (*string, *string, error) {
	shortName = strings.TrimSpace(shortName)
	var name *string
	if shortName != "" {
		if utf8.RuneCountInString(shortName) > shortNameLength {
			return nil, nil, invalid("El nombre corto admite hasta 20 caracteres.")
		}
		name = &shortName
	}
	color = strings.ToLower(strings.TrimSpace(color))
	if color == "" {
		return name, nil, nil
	}
	for _, entry := range BadgePalette {
		if entry.Hex == color {
			canonical := entry.Hex
			return name, &canonical, nil
		}
	}
	return nil, nil, invalid("Elige un color de la paleta o deja el distintivo sin color.")
}

// badgeInitial es la inicial del botón del mesero: la primera letra (o dígito) del nombre que se muestra, en
// mayúscula.
func badgeInitial(name string) string {
	for _, character := range name {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return string(unicode.ToUpper(character))
		}
	}
	return ""
}

// revokeWaiterAccess cierra las sesiones del usuario en dispositivos android (salvo keepSession, la sesión desde la
// que se hace el cambio) y revoca su credencial de envío en todos los dispositivos. Lo usan cambiar o quitar el PIN.
// Las sesiones de escritorio no se tocan: el PIN no sirve en ellas.
func revokeWaiterAccess(ctx context.Context, tx pgx.Tx, userID, reason, keepSession string) (int64, int64, error) {
	sessions, err := tx.Exec(ctx, `UPDATE app.sessions s SET revoked_at=now(),revoke_reason=$2
        FROM app.devices d WHERE d.id=s.device_id AND d.kind='android' AND s.user_id=$1 AND s.revoked_at IS NULL
        AND ($3='' OR s.id::text<>$3)`, userID, reason, keepSession)
	if err != nil {
		return 0, 0, err
	}
	credentials, err := revokeOfflineCredentials(ctx, tx, "user_id", userID, reason)
	if err != nil {
		return 0, 0, err
	}
	return sessions.RowsAffected(), credentials, nil
}

// revokeOfflineCredentials revoca las credenciales de envío vigentes de un usuario o de un dispositivo.
func revokeOfflineCredentials(ctx context.Context, tx pgx.Tx, column, id, reason string) (int64, error) {
	if column != "user_id" && column != "device_id" {
		return 0, invalid("Columna de credenciales no válida.")
	}
	result, err := tx.Exec(ctx, `UPDATE app.offline_order_credentials SET revoked_at=now(),revoke_reason=$2
        WHERE `+column+`=$1 AND revoked_at IS NULL`, id, reason)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// badgeColors responde la paleta del distintivo (GET users/badge-colors) a quien puede consultar usuarios.
func badgeColors(principal Principal) (any, error) {
	if err := principal.require("users.read"); err != nil {
		return nil, err
	}
	return BadgePalette, nil
}

// userPIN atiende las rutas de administración del PIN de un usuario (users.manage, Personal › Usuarios):
//
//   - PUT users/{id}/pin {pin?, short_name, badge_color}: asigna o cambia el PIN (pin) y guarda el nombre corto y el
//     color. Sin pin, solo cambia el nombre corto y el color de un usuario que ya tiene PIN.
//   - POST users/{id}/pin/remove: quita el PIN (se puede volver a asignar).
//   - POST users/{id}/pin/unlock: quita el bloqueo por intentos fallidos.
//
// Siguen el patrón de las demás rutas de usuarios: lock de administración, nunca la identidad reservada y delegación
// segura (el actor debe tener los permisos del usuario).
func userPIN(ctx context.Context, tx pgx.Tx, request Request, principal Principal, userID string, parts []string) (any, error) {
	action := ""
	switch {
	case len(parts) == 3 && request.Method == "PUT":
		action = "set"
	case len(parts) == 4 && parts[3] == "remove" && request.Method == "POST":
		action = "remove"
	case len(parts) == 4 && parts[3] == "unlock" && request.Method == "POST":
		action = "unlock"
	default:
		return nil, invalid("Operación de PIN no válida.")
	}
	if err := principal.require("users.manage"); err != nil {
		return nil, err
	}
	if err := lockAdministration(ctx, tx); err != nil {
		return nil, err
	}
	if err := editableUser(ctx, tx, userID); err != nil {
		return nil, err
	}
	if err := requireManageableUser(ctx, tx, principal, userID); err != nil {
		return nil, err
	}
	switch action {
	case "set":
		return assignUserPIN(ctx, tx, request, principal, userID)
	case "remove":
		return removeUserPIN(ctx, tx, principal, userID)
	default:
		return unlockUserPIN(ctx, tx, principal, userID)
	}
}

func assignUserPIN(ctx context.Context, tx pgx.Tx, request Request, principal Principal, userID string) (any, error) {
	var input struct {
		PIN        *string `json:"pin"`
		ShortName  string  `json:"short_name"`
		BadgeColor string  `json:"badge_color"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	shortName, color, err := normalizeBadge(input.ShortName, input.BadgeColor)
	if err != nil {
		return nil, err
	}
	var hasPIN bool
	if err := tx.QueryRow(ctx, "SELECT pin_hash IS NOT NULL FROM app.users WHERE id=$1", userID).Scan(&hasPIN); err != nil {
		return nil, err
	}
	if input.PIN == nil && !hasPIN {
		return nil, invalid("Escribe el PIN de 4 a 6 dígitos.")
	}
	// Quien define su propio PIN lo hace en «Mi PIN» con su contraseña actual: así una sesión de administrador
	// desatendida no basta para darse un PIN.
	if input.PIN != nil && userID == principal.UserID {
		return nil, delegationDenied("Define tu propio PIN en «Mi PIN», con tu contraseña actual.")
	}
	previous, err := object(ctx, tx, "SELECT "+publicUserColumns+" FROM app.users u JOIN app.roles r ON r.id=u.role_id WHERE u.id=$1", userID)
	if err != nil {
		return nil, err
	}
	if input.PIN == nil {
		if _, err := tx.Exec(ctx, "UPDATE app.users SET short_name=$1,badge_color=$2,updated_at=now() WHERE id=$3", shortName, color, userID); err != nil {
			return nil, err
		}
		next, err := publicUser(ctx, tx, userID)
		if err != nil {
			return nil, err
		}
		return next, audit(ctx, tx, principal, "user.pin_badge_changed", "user", &userID, previous, next, true)
	}
	if err := validatePIN(*input.PIN); err != nil {
		return nil, err
	}
	var version int
	err = tx.QueryRow(ctx, `UPDATE app.users SET pin_hash=$1,pin_set_at=now(),pin_version=pin_version+1,short_name=$2,
        badge_color=$3,updated_at=now() WHERE id=$4 RETURNING pin_version`, HashPassword(*input.PIN), shortName, color, userID).Scan(&version)
	if err != nil {
		return nil, err
	}
	// Un PIN nuevo empieza sin intentos fallidos ni bloqueo.
	if _, err := tx.Exec(ctx, "DELETE FROM app.pin_attempts WHERE user_id=$1", userID); err != nil {
		return nil, err
	}
	sessions, credentials, err := revokeWaiterAccess(ctx, tx, userID, "pin_changed", "")
	if err != nil {
		return nil, err
	}
	next, err := publicUser(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	details := map[string]any{
		"set_by":              "administrator",
		"replaced":            hasPIN,
		"pin_version":         version,
		"short_name":          shortName,
		"badge_color":         color,
		"revoked_sessions":    sessions,
		"revoked_credentials": credentials,
	}
	return next, audit(ctx, tx, principal, "user.pin_set", "user", &userID, nil, details, true)
}

func removeUserPIN(ctx context.Context, tx pgx.Tx, principal Principal, userID string) (any, error) {
	var hasPIN bool
	if err := tx.QueryRow(ctx, "SELECT pin_hash IS NOT NULL FROM app.users WHERE id=$1", userID).Scan(&hasPIN); err != nil {
		return nil, err
	}
	if !hasPIN {
		return nil, &Fault{Status: 409, Code: "PIN_NOT_SET", Message: "Este usuario no tiene PIN."}
	}
	var version int
	err := tx.QueryRow(ctx, `UPDATE app.users SET pin_hash=NULL,pin_set_at=NULL,pin_version=pin_version+1,updated_at=now()
        WHERE id=$1 RETURNING pin_version`, userID).Scan(&version)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM app.pin_attempts WHERE user_id=$1", userID); err != nil {
		return nil, err
	}
	sessions, credentials, err := revokeWaiterAccess(ctx, tx, userID, "pin_removed", "")
	if err != nil {
		return nil, err
	}
	next, err := publicUser(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	details := map[string]any{"pin_version": version, "revoked_sessions": sessions, "revoked_credentials": credentials}
	return next, audit(ctx, tx, principal, "user.pin_removed", "user", &userID, nil, details, true)
}

func unlockUserPIN(ctx context.Context, tx pgx.Tx, principal Principal, userID string) (any, error) {
	var lockedUntil *time.Time
	err := tx.QueryRow(ctx, `DELETE FROM app.pin_attempts WHERE user_id=$1
        RETURNING CASE WHEN locked_until>now() THEN locked_until END`, userID).Scan(&lockedUntil)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	next, err := publicUser(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	// Solo se registra si de verdad estaba bloqueado (desbloquear un PIN libre no cambia nada).
	if lockedUntil == nil {
		return next, nil
	}
	return next, audit(ctx, tx, principal, "user.pin_unlocked", "user", &userID, nil, map[string]any{"locked_until": lockedUntil}, true)
}

// ownPIN es «Mi PIN» (PUT auth/pin {current_password, pin}): el propio usuario define o cambia su PIN con su
// contraseña actual. Solo quien puede entrar con PIN en los dispositivos de meseros (waiter_devices.use); nunca la
// identidad reservada. Cierra sus sesiones en dispositivos android (menos la actual) y su credencial de envío.
func ownPIN(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	if request.Method != "PUT" {
		return nil, invalid("Operación de PIN no válida.")
	}
	if principal.Support {
		return nil, denied()
	}
	if err := principal.require("waiter_devices.use"); err != nil {
		return nil, err
	}
	var input struct {
		Current string `json:"current_password"`
		PIN     string `json:"pin"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if len(input.Current) > 256 {
		return nil, invalid("La contraseña excede la longitud permitida.")
	}
	var currentHash string
	var hadPIN bool
	err := tx.QueryRow(ctx, "SELECT coalesce(password_hash,''),pin_hash IS NOT NULL FROM app.users WHERE id=$1 FOR UPDATE",
		principal.UserID).Scan(&currentHash, &hadPIN)
	if err != nil {
		return nil, err
	}
	if currentHash == "" {
		currentHash = dummyHash
	}
	if !VerifyPassword(input.Current, currentHash) {
		return nil, unauthorized("INVALID_CREDENTIALS", "La contraseña actual no es correcta.")
	}
	if err := validatePIN(input.PIN); err != nil {
		return nil, err
	}
	var version int
	err = tx.QueryRow(ctx, `UPDATE app.users SET pin_hash=$1,pin_set_at=now(),pin_version=pin_version+1,updated_at=now()
        WHERE id=$2 RETURNING pin_version`, HashPassword(input.PIN), principal.UserID).Scan(&version)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM app.pin_attempts WHERE user_id=$1", principal.UserID); err != nil {
		return nil, err
	}
	sessions, credentials, err := revokeWaiterAccess(ctx, tx, principal.UserID, "pin_changed", principal.SessionID)
	if err != nil {
		return nil, err
	}
	details := map[string]any{
		"set_by":              "self",
		"replaced":            hadPIN,
		"pin_version":         version,
		"revoked_sessions":    sessions,
		"revoked_credentials": credentials,
	}
	if err := audit(ctx, tx, principal, "user.pin_set", "user", &principal.UserID, nil, details, true); err != nil {
		return nil, err
	}
	return map[string]any{"has_pin": true, "pin_version": version}, nil
}

// publicUser es el usuario tal como lo ven Personal › Usuarios y la auditoría (sin hashes).
func publicUser(ctx context.Context, tx pgx.Tx, userID string) (any, error) {
	return object(ctx, tx, "SELECT "+publicUserColumns+" FROM app.users u JOIN app.roles r ON r.id=u.role_id WHERE u.id=$1", userID)
}
