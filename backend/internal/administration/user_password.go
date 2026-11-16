package administration

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// «Asignar contraseña» (etapa 9H, decisión del coordinador): POST users/{id}/password {password}. Un usuario con
// users.manage asigna una contraseña inicial a un usuario que no tiene ninguna (los usuarios importados del sistema
// anterior entran sin contraseña y sin acceso, y AD-EL POS no tiene PIN). Queda con initial_password = true, así que
// al entrar debe cambiarla. Nunca cambia la contraseña de un usuario que ya tiene una (eso lo hace cada quien desde su
// formulario personal). Sigue el patrón de las demás rutas de usuarios: delegación segura (el actor debe tener los
// permisos del usuario), lock de administración, sin restricción de escucha local, y auditada sin la contraseña.
func assignPassword(ctx context.Context, tx pgx.Tx, request Request, principal Principal, userID string) (any, error) {
	if request.Method != "POST" {
		return nil, invalid("Operación de usuario no válida.")
	}
	if err := principal.require("users.manage"); err != nil {
		return nil, err
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if !validPassword(input.Password) {
		return nil, invalid("La contraseña debe tener entre 8 y 256 caracteres.")
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
	var hasPassword bool
	if err := tx.QueryRow(ctx, "SELECT password_hash IS NOT NULL FROM app.users WHERE id=$1", userID).Scan(&hasPassword); err != nil {
		return nil, err
	}
	if hasPassword {
		return nil, &Fault{Status: 409, Code: "PASSWORD_ALREADY_SET",
			Message: "Este usuario ya tiene contraseña; solo él puede cambiarla desde su formulario personal."}
	}
	if _, err := tx.Exec(ctx, "UPDATE app.users SET password_hash=$1,initial_password=true,updated_at=now() WHERE id=$2",
		HashPassword(input.Password), userID); err != nil {
		return nil, err
	}
	next, err := object(ctx, tx, "SELECT "+publicUserColumns+" FROM app.users u JOIN app.roles r ON r.id=u.role_id WHERE u.id=$1", userID)
	if err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, principal, "user.password_assigned", "user", &userID, nil,
		map[string]bool{"initial_password": true}, true); err != nil {
		return nil, err
	}
	return next, nil
}
