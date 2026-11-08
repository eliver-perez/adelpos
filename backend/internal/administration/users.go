package administration

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

const publicUserColumns = "u.id,u.username,u.display_name,u.role_id,r.name AS role_name,u.active,u.initial_password,u.created_at,u.updated_at"

type UserInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	RoleID      string `json:"role_id"`
	Active      bool   `json:"active"`
	Password    string `json:"password,omitempty"`
}

func users(ctx context.Context, tx pgx.Tx, request Request, principal Principal, parts []string) (any, error) {
	if len(parts) == 3 && parts[2] == "permissions" {
		return userPermissions(ctx, tx, request, principal, parts[1])
	}
	if len(parts) == 3 && parts[2] == "preferences" {
		return preferences(ctx, tx, request, principal, parts[1], true)
	}
	if request.Method == "GET" && len(parts) == 1 {
		if err := principal.require("users.read"); err != nil {
			return nil, err
		}
		return list(ctx, tx, "SELECT "+publicUserColumns+" FROM app.users u JOIN app.roles r ON r.id=u.role_id WHERE NOT u.support ORDER BY u.username")
	}
	if err := principal.require("users.manage"); err != nil {
		return nil, err
	}
	if (request.Method != "POST" || len(parts) != 1) && (request.Method != "PATCH" || len(parts) != 2) {
		return nil, invalid("Operación de usuario no válida.")
	}
	var input UserInput
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !usernamePattern.MatchString(input.Username) || input.Username == "sa" || len(input.DisplayName) < 1 || len(input.DisplayName) > 100 || !identifierPattern.MatchString(input.RoleID) {
		return nil, invalid("Revisa usuario, nombre y rol. El nombre sa está reservado.")
	}
	var roleAllowed bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.roles WHERE id=$1 AND NOT reserved)", input.RoleID).Scan(&roleAllowed); err != nil {
		return nil, err
	}
	if !roleAllowed {
		return nil, denied()
	}
	// Delegación segura: el rol asignado nunca puede otorgar permisos que el actor no posee.
	assigned, err := rolePermissionKeys(ctx, tx, input.RoleID)
	if err != nil {
		return nil, err
	}
	if err := principal.requireWithin(assigned, "No puedes asignar un rol con permisos que tú no tienes."); err != nil {
		return nil, err
	}
	var userID string
	var previous json.RawMessage
	action := "user.created"
	if len(parts) == 1 {
		if !validPassword(input.Password) {
			return nil, invalid("La contraseña debe tener entre 8 y 256 caracteres.")
		}
		err := tx.QueryRow(ctx, `INSERT INTO app.users(username,display_name,password_hash,role_id,active)
            VALUES ($1,$2,$3,$4,$5) RETURNING id`, input.Username, input.DisplayName, HashPassword(input.Password), input.RoleID, input.Active).Scan(&userID)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO app.user_preferences(user_id) VALUES ($1)", userID); err != nil {
			return nil, err
		}
	} else {
		userID = parts[1]
		// Serializa cambios administrativos para proteger al último administrador activo.
		if err := lockAdministration(ctx, tx); err != nil {
			return nil, err
		}
		if err := editableUser(ctx, tx, userID); err != nil {
			return nil, err
		}
		currentRole, err := userRoleID(ctx, tx, userID)
		if err != nil {
			return nil, err
		}
		// Nadie modifica su propio rol; debe hacerlo otro usuario con permisos suficientes.
		if userID == principal.UserID && currentRole != input.RoleID {
			return nil, delegationDenied("No puedes modificar tu propio rol.")
		}
		if err := requireManageableUser(ctx, tx, principal, userID); err != nil {
			return nil, err
		}
		// Las excepciones individuales del usuario se conservan con el nuevo rol; el resultado
		// completo tampoco puede superar los permisos del actor.
		overrides, err := userOverrides(ctx, tx, userID)
		if err != nil {
			return nil, err
		}
		projected, err := projectedPermissions(ctx, tx, input.RoleID, overrides)
		if err != nil {
			return nil, err
		}
		if err := principal.requireWithin(projected, "El usuario resultante tendría permisos que tú no tienes."); err != nil {
			return nil, err
		}
		previous, err = object(ctx, tx, "SELECT "+publicUserColumns+" FROM app.users u JOIN app.roles r ON r.id=u.role_id WHERE u.id=$1", userID)
		if err != nil {
			return nil, err
		}
		if input.Password != "" {
			return nil, invalid("El cambio de contraseña se realiza desde el formulario personal de contraseña.")
		}
		// El último administrador se determina por permisos efectivos, no por el código del rol.
		err = preserveAdministration(ctx, tx, func() error {
			_, err := tx.Exec(ctx, "UPDATE app.users SET username=$1,display_name=$2,role_id=$3,active=$4,updated_at=now() WHERE id=$5", input.Username, input.DisplayName, input.RoleID, input.Active, userID)
			if err != nil {
				return err
			}
			if !input.Active {
				_, err = tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=now(),revoke_reason='user_disabled' WHERE user_id=$1 AND revoked_at IS NULL", userID)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
		action = "user.updated"
	}
	next, err := object(ctx, tx, "SELECT "+publicUserColumns+" FROM app.users u JOIN app.roles r ON r.id=u.role_id WHERE u.id=$1", userID)
	if err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, principal, action, "user", &userID, previous, next, true); err != nil {
		return nil, err
	}
	return next, nil
}

func editableUser(ctx context.Context, tx pgx.Tx, userID string) error {
	var reserved bool
	if err := tx.QueryRow(ctx, "SELECT support FROM app.users WHERE id=$1 FOR UPDATE", userID).Scan(&reserved); err != nil {
		return err
	}
	if reserved {
		return denied()
	}
	return nil
}

func userPermissions(ctx context.Context, tx pgx.Tx, request Request, principal Principal, userID string) (any, error) {
	key := "permissions.read"
	if request.Method == "PUT" {
		key = "permissions.manage"
	}
	if err := principal.require(key); err != nil {
		return nil, err
	}
	if err := editableUser(ctx, tx, userID); err != nil {
		return nil, err
	}
	if request.Method == "PUT" {
		var input struct {
			Overrides map[string]string `json:"overrides"`
		}
		if err := decode(request.Body, &input); err != nil {
			return nil, err
		}
		for permission, effect := range input.Overrides {
			if !knownPermission(permission) || (effect != "inherit" && effect != "allow" && effect != "deny") {
				return nil, invalid("Los estados permitidos son inherit, allow y deny para permisos registrados.")
			}
		}
		if err := lockAdministration(ctx, tx); err != nil {
			return nil, err
		}
		// Nadie amplía ni reduce sus propias excepciones.
		if userID == principal.UserID {
			return nil, delegationDenied("No puedes modificar tus propias excepciones de permisos.")
		}
		if err := requireManageableUser(ctx, tx, principal, userID); err != nil {
			return nil, err
		}
		// Se proyecta el resultado antes de escribir: tanto un allow como retirar un deny pueden
		// otorgar permisos, y ninguno puede quedar fuera de los permisos del actor.
		roleID, err := userRoleID(ctx, tx, userID)
		if err != nil {
			return nil, err
		}
		resulting, err := userOverrides(ctx, tx, userID)
		if err != nil {
			return nil, err
		}
		for permission, effect := range input.Overrides {
			if effect == "inherit" {
				delete(resulting, permission)
			} else {
				resulting[permission] = effect
			}
		}
		projected, err := projectedPermissions(ctx, tx, roleID, resulting)
		if err != nil {
			return nil, err
		}
		if err := principal.requireWithin(projected, "No puedes conceder permisos que tú no tienes."); err != nil {
			return nil, err
		}
		previous, err := list(ctx, tx, "SELECT permission_key,effect FROM app.user_permissions WHERE user_id=$1 ORDER BY permission_key", userID)
		if err != nil {
			return nil, err
		}
		err = preserveAdministration(ctx, tx, func() error {
			for permission, effect := range input.Overrides {
				var err error
				if effect == "inherit" {
					_, err = tx.Exec(ctx, "DELETE FROM app.user_permissions WHERE user_id=$1 AND permission_key=$2", userID, permission)
				} else {
					_, err = tx.Exec(ctx, `INSERT INTO app.user_permissions(user_id,permission_key,effect) VALUES ($1,$2,$3)
                        ON CONFLICT(user_id,permission_key) DO UPDATE SET effect=excluded.effect`, userID, permission, effect)
				}
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		next, err := list(ctx, tx, "SELECT permission_key,effect FROM app.user_permissions WHERE user_id=$1 ORDER BY permission_key", userID)
		if err != nil {
			return nil, err
		}
		if err := audit(ctx, tx, principal, "user.permissions_changed", "user", &userID, previous, next, true); err != nil {
			return nil, err
		}
	} else if request.Method != "GET" {
		return nil, invalid("Método no válido para permisos.")
	}
	effective, err := effectivePermissions(ctx, tx, userID, false)
	if err != nil {
		return nil, err
	}
	overrides, err := list(ctx, tx, "SELECT permission_key,effect FROM app.user_permissions WHERE user_id=$1 ORDER BY permission_key", userID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"effective": effective, "overrides": overrides}, nil
}
