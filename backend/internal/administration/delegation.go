package administration

import (
	"context"
	"regexp"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Reglas de delegación administrativa.
//
// Nadie puede conceder, mediante roles, asignaciones o excepciones, permisos efectivos que no
// posee, ni administrar a un usuario con más permisos que los suyos, ni modificar su propio rol
// o sus propias excepciones. Así, users.manage, roles.manage y permissions.manage pueden
// delegarse sin que equivalgan a convertirse en administrador.

// administrativeLock serializa los cambios que pueden alterar quién administra el sistema.
const administrativeLock = "SELECT pg_advisory_xact_lock(7820148303)"

const lastAdministratorMessage = "Debe permanecer al menos un administrador ordinario activo."

func delegationDenied(message string) error {
	return &Fault{Status: 403, Code: "PERMISSION_DENIED", Message: message}
}

// holdsAll indica si el actor posee cada uno de los permisos indicados.
func (principal Principal) holdsAll(keys []string) bool {
	for _, key := range keys {
		if !slices.Contains(principal.Permissions, key) {
			return false
		}
	}
	return true
}

func (principal Principal) requireWithin(keys []string, message string) error {
	if !principal.holdsAll(keys) {
		return delegationDenied(message)
	}
	return nil
}

func lockAdministration(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, administrativeLock)
	return err
}

func rolePermissionKeys(ctx context.Context, tx pgx.Tx, roleID string) ([]string, error) {
	rows, err := tx.Query(ctx, "SELECT permission_key FROM app.role_permissions WHERE role_id=$1 ORDER BY permission_key", roleID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func userRoleID(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var roleID string
	err := tx.QueryRow(ctx, "SELECT role_id FROM app.users WHERE id=$1", userID).Scan(&roleID)
	return roleID, err
}

func userOverrides(ctx context.Context, tx pgx.Tx, userID string) (map[string]string, error) {
	rows, err := tx.Query(ctx, "SELECT permission_key,effect FROM app.user_permissions WHERE user_id=$1", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	overrides := map[string]string{}
	for rows.Next() {
		var key, effect string
		if err := rows.Scan(&key, &effect); err != nil {
			return nil, err
		}
		overrides[key] = effect
	}
	return overrides, rows.Err()
}

// projectedPermissions calcula los permisos efectivos que resultarían de un rol y unas
// excepciones, con la misma precedencia que effectivePermissions: allow, después deny y
// después lo heredado del rol. Permite autorizar un cambio antes de escribirlo.
func projectedPermissions(ctx context.Context, tx pgx.Tx, roleID string, overrides map[string]string) ([]string, error) {
	inherited, err := rolePermissionKeys(ctx, tx, roleID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT key FROM app.permissions ORDER BY key")
	if err != nil {
		return nil, err
	}
	registered, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, key := range registered {
		switch overrides[key] {
		case "allow":
			result = append(result, key)
		case "deny":
			continue
		default:
			if slices.Contains(inherited, key) {
				result = append(result, key)
			}
		}
	}
	return result, nil
}

// requireManageableUser impide administrar a un usuario cuyos permisos efectivos actuales o cuyo
// límite de descuento superan los del actor (por ejemplo, desactivar o degradar a un administrador).
func requireManageableUser(ctx context.Context, tx pgx.Tx, principal Principal, userID string) error {
	current, err := effectivePermissions(ctx, tx, userID, false)
	if err != nil {
		return err
	}
	if err := principal.requireWithin(current, "No puedes administrar a un usuario con permisos que tú no tienes."); err != nil {
		return err
	}
	roleID, err := userRoleID(ctx, tx, userID)
	if err != nil {
		return err
	}
	return requireLimitWithin(ctx, tx, principal, roleID)
}

var discountLimitPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,2})(\.[0-9]{1,2})?$`)

// parseDiscountLimit valida un límite de descuento por rol: porcentaje de 0 a 100 con dos decimales.
func parseDiscountLimit(text string) (decimal.Decimal, error) {
	if !discountLimitPattern.MatchString(text) {
		return decimal.Zero, invalid("El límite de descuento debe ser un porcentaje de 0 a 100 con hasta dos decimales.")
	}
	value := decimal.RequireFromString(text)
	if value.GreaterThan(decimal.NewFromInt(100)) {
		return decimal.Zero, invalid("El límite de descuento no puede superar el 100 %.")
	}
	return value, nil
}

// actorDiscountLimit es el límite del actor según su rol; la identidad de soporte conserva el máximo.
func actorDiscountLimit(ctx context.Context, tx pgx.Tx, principal Principal) (decimal.Decimal, error) {
	if principal.Support {
		return decimal.NewFromInt(100), nil
	}
	return roleDiscountLimit(ctx, tx, principal.RoleID)
}

func roleDiscountLimit(ctx context.Context, tx pgx.Tx, roleID string) (decimal.Decimal, error) {
	var limit string
	err := tx.QueryRow(ctx, "SELECT coalesce((SELECT max_percent FROM app.role_discount_limits WHERE role_id=$1),0)::numeric(5,2)::text", roleID).Scan(&limit)
	if err != nil {
		return decimal.Zero, err
	}
	return decimal.RequireFromString(limit), nil
}

// requireLimitWithin impide que un rol con un límite de descuento mayor que el del actor se
// asigne, se edite o se conceda a otro usuario.
func requireLimitWithin(ctx context.Context, tx pgx.Tx, principal Principal, roleID string) error {
	limit, err := roleDiscountLimit(ctx, tx, roleID)
	if err != nil {
		return err
	}
	return requireLimitValue(ctx, tx, principal, limit)
}

func requireLimitValue(ctx context.Context, tx pgx.Tx, principal Principal, limit decimal.Decimal) error {
	own, err := actorDiscountLimit(ctx, tx, principal)
	if err != nil {
		return err
	}
	if limit.GreaterThan(own) {
		return delegationDenied("No puedes conceder un límite de descuento mayor que el tuyo.")
	}
	return nil
}

// fullAdministrators cuenta los usuarios ordinarios activos que conservan todos los permisos
// registrados. Por las reglas de delegación, solo un usuario así puede restituir cualquier
// permiso a otros; sa no cuenta porque es una identidad reservada de soporte.
func fullAdministrators(ctx context.Context, tx pgx.Tx) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM app.users u
        WHERE u.active AND NOT u.support AND NOT EXISTS (
            SELECT FROM app.permissions p
            LEFT JOIN app.user_permissions individual ON individual.user_id=u.id AND individual.permission_key=p.key
            LEFT JOIN app.role_permissions inherited ON inherited.role_id=u.role_id AND inherited.permission_key=p.key
            WHERE individual.effect IS DISTINCT FROM 'allow'
            AND NOT (individual.effect IS NULL AND inherited.permission_key IS NOT NULL)
        )`).Scan(&count)
	return count, err
}

// preserveAdministration ejecuta un cambio administrativo y lo rechaza si deja al sistema sin
// administradores completos cuando antes había al menos uno. El error de validación revierte
// la transacción completa, por lo que el cambio nunca se confirma.
func preserveAdministration(ctx context.Context, tx pgx.Tx, change func() error) error {
	before, err := fullAdministrators(ctx, tx)
	if err != nil {
		return err
	}
	if err := change(); err != nil {
		return err
	}
	after, err := fullAdministrators(ctx, tx)
	if err != nil {
		return err
	}
	if before > 0 && after == 0 {
		return invalid(lastAdministratorMessage)
	}
	return nil
}
