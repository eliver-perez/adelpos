package administration

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

const rolesQuery = `SELECT r.id,r.code,r.name,r.created_at,
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
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
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
	var roleID string
	var previous json.RawMessage
	action := "role.created"
	if len(parts) == 1 {
		if err := tx.QueryRow(ctx, "INSERT INTO app.roles(name) VALUES ($1) RETURNING id", input.Name).Scan(&roleID); err != nil {
			return nil, err
		}
	} else {
		roleID = parts[1]
		var reserved bool
		if err := tx.QueryRow(ctx, "SELECT reserved FROM app.roles WHERE id=$1 FOR UPDATE", roleID).Scan(&reserved); err != nil {
			return nil, err
		}
		if reserved {
			return nil, denied()
		}
		var err error
		previous, err = object(ctx, tx, rolesQuery+" AND r.id=$1", roleID)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, "UPDATE app.roles SET name=$1 WHERE id=$2", input.Name, roleID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM app.role_permissions WHERE role_id=$1", roleID); err != nil {
			return nil, err
		}
		action = "role.updated"
	}
	for _, key := range input.Permissions {
		if _, err := tx.Exec(ctx, "INSERT INTO app.role_permissions(role_id,permission_key) VALUES ($1,$2) ON CONFLICT DO NOTHING", roleID, key); err != nil {
			return nil, err
		}
	}
	next, err := object(ctx, tx, rolesQuery+" AND r.id=$1", roleID)
	if err != nil {
		return nil, err
	}
	return next, audit(ctx, tx, principal, action, "role", &roleID, previous, next, true)
}
