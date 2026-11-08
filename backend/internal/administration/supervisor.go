package administration

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/payments"
)

// supervisorAuthorizer valida una autorización puntual de supervisor dentro de la misma
// transacción de la operación autorizada. No crea ni reemplaza sesiones, por lo que el
// supervisor conserva la suya en su propia terminal; comparte el límite de intentos del inicio
// de sesión y nunca registra la contraseña. Un usuario no puede autorizarse a sí mismo.
func supervisorAuthorizer(principal Principal) payments.Authorizer {
	return func(ctx context.Context, tx pgx.Tx, username, password string) (*payments.Supervisor, error) {
		username = strings.ToLower(strings.TrimSpace(username))
		if username == "" || len(username) > 40 || password == "" || len(password) > 256 {
			return nil, &Fault{Status: 400, Code: "AUTHORIZATION_REQUIRED", Message: "Indica el usuario y la contraseña del supervisor que autoriza."}
		}
		if username == principal.Username {
			return nil, &Fault{Status: 403, Code: "SELF_AUTHORIZATION", Message: "No puedes autorizar tu propia operación; solicita la autorización de otro supervisor."}
		}
		_, err := tx.Exec(ctx, "INSERT INTO app.login_attempts(device_id,username) VALUES ($1,$2) ON CONFLICT DO NOTHING", principal.DeviceID, username)
		if err != nil {
			return nil, err
		}
		var blocked bool
		err = tx.QueryRow(ctx, `SELECT coalesce(locked_until>now(),false) FROM app.login_attempts
            WHERE device_id=$1 AND username=$2 FOR UPDATE`, principal.DeviceID, username).Scan(&blocked)
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, failedAuthorization(ctx, tx, principal, username, "AUTHORIZATION_THROTTLED")
		}
		var userID, hash string
		var active, support bool
		err = tx.QueryRow(ctx, "SELECT id,coalesce(password_hash,''),active,support FROM app.users WHERE username=$1", username).Scan(&userID, &hash, &active, &support)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		found := err == nil && hash != ""
		if !found {
			hash = dummyHash
		}
		verified := VerifyPassword(password, hash)
		if !found || !active || !verified {
			if err := recordFailedAttempt(ctx, tx, principal.DeviceID, username); err != nil {
				return nil, err
			}
			return nil, failedAuthorization(ctx, tx, principal, username, "AUTHORIZATION_INVALID")
		}
		if _, err := tx.Exec(ctx, "DELETE FROM app.login_attempts WHERE device_id=$1 AND username=$2", principal.DeviceID, username); err != nil {
			return nil, err
		}
		// Se evalúan los permisos efectivos actuales del supervisor: rol más excepciones.
		permissions, err := effectivePermissions(ctx, tx, userID, support)
		if err != nil {
			return nil, err
		}
		return &payments.Supervisor{UserID: userID, Username: username, Permissions: permissions}, nil
	}
}

// failedAuthorization audita el intento y confirma el contador de fallos aunque la operación
// solicitada no se realice. No distingue usuarios inexistentes de contraseñas incorrectas.
func failedAuthorization(ctx context.Context, tx pgx.Tx, principal Principal, username, code string) error {
	details := map[string]string{"attempted_username": username, "reason": code}
	if err := audit(ctx, tx, principal, "authorization.failed", "authorization", nil, nil, details, false); err != nil {
		return err
	}
	if code == "AUTHORIZATION_THROTTLED" {
		return &Fault{Status: 429, Code: code, Message: "Demasiados intentos de autorización. Espera 15 minutos antes de volver a intentar.", Commit: true}
	}
	return &Fault{Status: 401, Code: code, Message: "Usuario o contraseña del supervisor inválidos.", Commit: true}
}
