package administration

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

const rolesQuery = `SELECT r.id,r.code,r.name,r.created_at,
    (SELECT l.max_percent::text FROM app.role_discount_limits l WHERE l.role_id=r.id) AS discount_limit,
    coalesce((SELECT jsonb_agg(p.permission_key ORDER BY p.permission_key) FROM app.role_permissions p WHERE p.role_id=r.id),'[]'::jsonb) AS permissions
    FROM app.roles r WHERE NOT r.reserved`

func roles(ctx context.Context, tx pgx.Tx, request Request, principal Principal, parts []string) (any, error) {
	if request.Method == "GET" && len(parts) == 1 {
		if err := principal.require("roles.read"); err != nil {
			return nil, err
		}
		return list(ctx, tx, rolesQuery+" ORDER BY r.name")
	}
	if err := principal.require("roles.manage"); err != nil {
		return nil, err
	}
	if (request.Method != "POST" || len(parts) != 1) && (request.Method != "PATCH" || len(parts) != 2) {
		return nil, invalid("Operación de rol no válida.")
	}
	var input struct {
		Name          string   `json:"name"`
		Permissions   []string `json:"permissions"`
		DiscountLimit *string  `json:"discount_limit"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if len(input.Name) < 1 || len(input.Name) > 80 {
		return nil, invalid("El nombre de rol debe tener entre 1 y 80 caracteres.")
	}
	if err := principal.require("permissions.manage"); err != nil {
		return nil, err
	}
	for _, key := range input.Permissions {
		if !knownPermission(key) {
			return nil, invalid("El rol contiene un permiso desconocido.")
		}
	}
	// Delegación segura: un rol nunca puede otorgar permisos que el actor no posee.
	if err := principal.requireWithin(input.Permissions, "No puedes conceder a un rol permisos que tú no tienes."); err != nil {
		return nil, err
	}
	// El límite de descuento del rol tampoco puede superar el del actor.
	var discountLimit *decimal.Decimal
	if input.DiscountLimit != nil && *input.DiscountLimit != "" {
		value, err := parseDiscountLimit(*input.DiscountLimit)
		if err != nil {
			return nil, err
		}
		if err := requireLimitValue(ctx, tx, principal, value); err != nil {
			return nil, err
		}
		discountLimit = &value
	}
	var roleID string
	var previous json.RawMessage
	action := "role.created"
	if len(parts) == 1 {
		if err := tx.QueryRow(ctx, "INSERT INTO app.roles(name) VALUES ($1) RETURNING id", input.Name).Scan(&roleID); err != nil {
			return nil, err
		}
		if err := insertRolePermissions(ctx, tx, roleID, input.Permissions); err != nil {
			return nil, err
		}
		if err := saveDiscountLimit(ctx, tx, roleID, input.DiscountLimit, discountLimit); err != nil {
			return nil, err
		}
	} else {
		roleID = parts[1]
		// Serializa cambios que pueden alterar quién administra el sistema.
		if err := lockAdministration(ctx, tx); err != nil {
			return nil, err
		}
		var reserved bool
		if err := tx.QueryRow(ctx, "SELECT reserved FROM app.roles WHERE id=$1 FOR UPDATE", roleID).Scan(&reserved); err != nil {
			return nil, err
		}
		if reserved {
			return nil, denied()
		}
		// Nadie modifica el rol que le otorga sus propios permisos.
		if roleID == principal.RoleID {
			return nil, delegationDenied("No puedes modificar tu propio rol.")
		}
		current, err := rolePermissionKeys(ctx, tx, roleID)
		if err != nil {
			return nil, err
		}
		if err := principal.requireWithin(current, "No puedes editar un rol con permisos que tú no tienes."); err != nil {
			return nil, err
		}
		if err := requireLimitWithin(ctx, tx, principal, roleID); err != nil {
			return nil, err
		}
		previous, err = object(ctx, tx, rolesQuery+" AND r.id=$1", roleID)
		if err != nil {
			return nil, err
		}
		// El último administrador se determina por permisos efectivos de todos los usuarios del rol.
		err = preserveAdministration(ctx, tx, func() error {
			if _, err := tx.Exec(ctx, "UPDATE app.roles SET name=$1 WHERE id=$2", input.Name, roleID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "DELETE FROM app.role_permissions WHERE role_id=$1", roleID); err != nil {
				return err
			}
			if err := insertRolePermissions(ctx, tx, roleID, input.Permissions); err != nil {
				return err
			}
			return saveDiscountLimit(ctx, tx, roleID, input.DiscountLimit, discountLimit)
		})
		if err != nil {
			return nil, err
		}
		action = "role.updated"
	}
	next, err := object(ctx, tx, rolesQuery+" AND r.id=$1", roleID)
	if err != nil {
		return nil, err
	}
	return next, audit(ctx, tx, principal, action, "role", &roleID, previous, next, true)
}

// saveDiscountLimit conserva el límite si no se envía, lo elimina (0 %) con cadena vacía o lo guarda.
func saveDiscountLimit(ctx context.Context, tx pgx.Tx, roleID string, requested *string, value *decimal.Decimal) error {
	if requested == nil {
		return nil
	}
	if value == nil {
		_, err := tx.Exec(ctx, "DELETE FROM app.role_discount_limits WHERE role_id=$1", roleID)
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO app.role_discount_limits(role_id,max_percent) VALUES ($1,$2)
        ON CONFLICT(role_id) DO UPDATE SET max_percent=excluded.max_percent`, roleID, value.StringFixed(2))
	return err
}

func insertRolePermissions(ctx context.Context, tx pgx.Tx, roleID string, keys []string) error {
	for _, key := range keys {
		if _, err := tx.Exec(ctx, "INSERT INTO app.role_permissions(role_id,permission_key) VALUES ($1,$2) ON CONFLICT DO NOTHING", roleID, key); err != nil {
			return err
		}
	}
	return nil
}
