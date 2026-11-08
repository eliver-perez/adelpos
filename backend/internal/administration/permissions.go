package administration

import (
	"context"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"

	"github.com/jackc/pgx/v5"
)

type Permission struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

// Este registro enumera operaciones existentes; los módulos futuros añadirán sus propias claves.
var Permissions = []Permission{
	{"users.read", "Consultar usuarios"},
	{"users.manage", "Crear, editar y desactivar usuarios"},
	{"roles.read", "Consultar roles"},
	{"roles.manage", "Crear y modificar roles"},
	{"permissions.read", "Consultar permisos efectivos"},
	{"permissions.manage", "Modificar permisos de rol y excepciones individuales"},
	{"devices.read", "Consultar terminales"},
	{"devices.manage", "Enrolar, configurar y revocar terminales"},
	{"sessions.read", "Consultar sesiones"},
	{"sessions.manage", "Revocar sesiones"},
	{"settings.read", "Consultar configuración general"},
	{"settings.manage", "Modificar configuración general"},
	{"registers.read", "Consultar cajas físicas"},
	{"registers.manage", "Crear y configurar cajas físicas"},
	{"audit.read", "Consultar auditoría"},
}

func init() {
	for _, permission := range kitchen.Permissions {
		Permissions = append(Permissions, Permission{Key: permission.Key, Description: permission.Description})
	}
	for _, permission := range operations.Permissions {
		Permissions = append(Permissions, Permission{Key: permission.Key, Description: permission.Description})
	}
	for _, permission := range catalog.Permissions {
		Permissions = append(Permissions, Permission{Key: permission.Key, Description: permission.Description})
	}
}

func knownPermission(key string) bool {
	for _, permission := range Permissions {
		if permission.Key == key {
			return true
		}
	}
	return false
}

func ensureFoundations(ctx context.Context, tx pgx.Tx) error {
	for _, permission := range Permissions {
		_, err := tx.Exec(ctx, "INSERT INTO app.permissions(key, description) VALUES ($1,$2) ON CONFLICT DO NOTHING", permission.Key, permission.Description)
		if err != nil {
			return err
		}
	}
	roles := []struct {
		code, name string
		reserved   bool
	}{
		{"administrator", "Administrador", false},
		{"supervisor", "Supervisor", false},
		{"cashier", "Cajero", false},
		{"waiter", "Mesero", false},
		{"kitchen", "Cocina", false},
		{"inventory", "Inventarios", false},
		{"finance", "Finanzas", false},
		{"support", "Soporte reservado", true},
	}
	for _, role := range roles {
		_, err := tx.Exec(ctx, "INSERT INTO app.roles(code,name,reserved) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING", role.code, role.name, role.reserved)
		if err != nil {
			return err
		}
	}
	// La identidad reservada existe sin credencial hasta que el propietario la aprovisiona localmente.
	_, err := tx.Exec(ctx, `INSERT INTO app.users(username,display_name,role_id,support)
        SELECT 'sa','Soporte',id,true FROM app.roles WHERE code='support'
        ON CONFLICT DO NOTHING`)
	return err
}

func effectivePermissions(ctx context.Context, tx pgx.Tx, userID string, support bool) ([]string, error) {
	if support {
		keys := make([]string, 0, len(Permissions))
		for _, permission := range Permissions {
			keys = append(keys, permission.Key)
		}
		return keys, nil
	}
	rows, err := tx.Query(ctx, `SELECT p.key FROM app.permissions p
        JOIN app.users u ON u.id=$1
        LEFT JOIN app.user_permissions individual ON individual.user_id=u.id AND individual.permission_key=p.key
        LEFT JOIN app.role_permissions inherited ON inherited.role_id=u.role_id AND inherited.permission_key=p.key
        WHERE individual.effect='allow' OR (individual.effect IS NULL AND inherited.permission_key IS NOT NULL)
        ORDER BY p.key`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}
