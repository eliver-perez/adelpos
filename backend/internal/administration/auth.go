package administration

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var dummyHash = HashPassword(NewToken())

func login(ctx context.Context, tx pgx.Tx, request Request) (any, error) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	if len(input.Username) > 40 || len(input.Password) > 256 {
		return nil, invalid("Las credenciales exceden la longitud permitida.")
	}
	_, err := tx.Exec(ctx, "INSERT INTO app.login_attempts(device_id,username) VALUES ($1,$2) ON CONFLICT DO NOTHING", request.DeviceID, input.Username)
	if err != nil {
		return nil, err
	}
	var failures int
	var blocked bool
	err = tx.QueryRow(ctx, `SELECT failures,coalesce(locked_until>now(),false)
        FROM app.login_attempts WHERE device_id=$1 AND username=$2 FOR UPDATE`, request.DeviceID, input.Username).Scan(&failures, &blocked)
	if err != nil {
		return nil, err
	}
	if blocked {
		return failedLogin(ctx, tx, request.DeviceID, input.Username, "LOGIN_THROTTLED")
	}
	var userID, hash string
	var active bool
	err = tx.QueryRow(ctx, "SELECT id,coalesce(password_hash,''),active FROM app.users WHERE username=$1 FOR UPDATE", input.Username).Scan(&userID, &hash, &active)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	found := err == nil && hash != ""
	if !found {
		hash = dummyHash
	}
	verified := VerifyPassword(input.Password, hash)
	if !found || !active || !verified {
		if err := recordFailedAttempt(ctx, tx, request.DeviceID, input.Username); err != nil {
			return nil, err
		}
		return failedLogin(ctx, tx, request.DeviceID, input.Username, "INVALID_CREDENTIALS")
	}
	_, err = tx.Exec(ctx, "DELETE FROM app.login_attempts WHERE device_id=$1 AND username=$2", request.DeviceID, input.Username)
	if err != nil {
		return nil, err
	}
	token, err := issueSession(ctx, tx, userID, request.DeviceID, "replaced")
	if err != nil {
		return nil, err
	}
	principal := Principal{UserID: userID, Username: input.Username, DeviceID: request.DeviceID}
	if err := audit(ctx, tx, principal, "session.login", "user", &userID, nil, nil, true); err != nil {
		return nil, err
	}
	return map[string]any{"session_token": token}, nil
}

// recordFailedAttempt aplica el límite de cinco fallos en quince minutos por usuario y terminal.
// Lo comparten el inicio de sesión y las autorizaciones puntuales de supervisor.
func recordFailedAttempt(ctx context.Context, tx pgx.Tx, deviceID, username string) error {
	_, err := tx.Exec(ctx, `UPDATE app.login_attempts SET
        failures=CASE WHEN window_start < now()-interval '15 minutes' THEN 1 ELSE failures+1 END,
        locked_until=CASE WHEN failures>=4 AND window_start>=now()-interval '15 minutes' THEN now()+interval '15 minutes' ELSE NULL END,
        window_start=CASE WHEN window_start<now()-interval '15 minutes' THEN now() ELSE window_start END
        WHERE device_id=$1 AND username=$2`, deviceID, username)
	return err
}

func failedLogin(ctx context.Context, tx pgx.Tx, deviceID, username, code string) (any, error) {
	if err := audit(ctx, tx, Principal{DeviceID: deviceID}, "session.login_failed", "authentication", nil, nil, map[string]string{"attempted_username": username, "reason": code}, false); err != nil {
		return nil, err
	}
	message := "Usuario o contraseña inválidos."
	status := 401
	if code == "LOGIN_THROTTLED" {
		message = "Demasiados intentos. Espera 15 minutos antes de volver a intentar."
		status = 429
	}
	return nil, &Fault{Status: status, Code: code, Message: message, Commit: true}
}

func issueSession(ctx context.Context, tx pgx.Tx, userID, deviceID, reason string) (string, error) {
	// El lock de usuario y el índice parcial garantizan una sola sesión incluso con accesos simultáneos.
	if _, err := tx.Exec(ctx, "SELECT id FROM app.users WHERE id=$1 FOR UPDATE", userID); err != nil {
		return "", err
	}
	replaced, err := tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=now(),revoke_reason=$2 WHERE user_id=$1 AND revoked_at IS NULL", userID, reason)
	if err != nil {
		return "", err
	}
	if replaced.RowsAffected() > 0 {
		principal := Principal{UserID: userID, DeviceID: deviceID}
		if err := tx.QueryRow(ctx, "SELECT username FROM app.users WHERE id=$1", userID).Scan(&principal.Username); err != nil {
			return "", err
		}
		if err := audit(ctx, tx, principal, "session."+reason, "user", &userID, nil, map[string]int64{"revoked_sessions": replaced.RowsAffected()}, true); err != nil {
			return "", err
		}
	}
	token := NewToken()
	_, err = tx.Exec(ctx, "INSERT INTO app.sessions(user_id,device_id,token_hash,expires_at) VALUES ($1,$2,$3,$4)", userID, deviceID, TokenHash(token), time.Now().Add(12*time.Hour))
	return token, err
}

func changePassword(ctx context.Context, tx pgx.Tx, principal Principal, body []byte) (any, error) {
	var input struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decode(body, &input); err != nil {
		return nil, err
	}
	if !validPassword(input.New) || (principal.Support && len(input.New) < 12) {
		return nil, invalid("La nueva contraseña debe tener al menos 8 caracteres (12 para soporte), máximo 256.")
	}
	var currentHash string
	if err := tx.QueryRow(ctx, "SELECT password_hash FROM app.users WHERE id=$1 FOR UPDATE", principal.UserID).Scan(&currentHash); err != nil {
		return nil, err
	}
	if !VerifyPassword(input.Current, currentHash) {
		return nil, unauthorized("INVALID_CREDENTIALS", "La contraseña actual no es correcta.")
	}
	_, err := tx.Exec(ctx, "UPDATE app.users SET password_hash=$1,initial_password=false,updated_at=now() WHERE id=$2", HashPassword(input.New), principal.UserID)
	if err != nil {
		return nil, err
	}
	token, err := issueSession(ctx, tx, principal.UserID, principal.DeviceID, "password_changed")
	if err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, principal, "user.password_changed", "user", &principal.UserID, nil, map[string]bool{"initial_password": false}, true); err != nil {
		return nil, err
	}
	return map[string]string{"session_token": token}, nil
}

func enroll(ctx context.Context, tx pgx.Tx, request Request) (any, error) {
	var input struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if len(input.Code) != 43 || input.Name == "" || len(input.Name) > 80 {
		return nil, invalid("Indica un código válido y un nombre de terminal de hasta 80 caracteres.")
	}
	var enrollmentID string
	err := tx.QueryRow(ctx, "SELECT id FROM app.enrollments WHERE code_hash=$1 AND used_at IS NULL AND expires_at>now() FOR UPDATE", TokenHash(input.Code)).Scan(&enrollmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, unauthorized("ENROLLMENT_INVALID", "El código no es válido, expiró o ya fue utilizado.")
	}
	if err != nil {
		return nil, err
	}
	token := NewToken()
	var deviceID string
	if err := tx.QueryRow(ctx, "INSERT INTO app.devices(name,secret_hash) VALUES ($1,$2) RETURNING id", input.Name, TokenHash(token)).Scan(&deviceID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "UPDATE app.enrollments SET used_at=now() WHERE id=$1", enrollmentID); err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, Principal{DeviceID: deviceID}, "device.enrolled", "device", &deviceID, nil, map[string]string{"name": input.Name}, true); err != nil {
		return nil, err
	}
	return map[string]string{"device_id": deviceID, "device_token": token}, nil
}
