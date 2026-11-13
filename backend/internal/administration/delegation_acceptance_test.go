//go:build integration

package administration_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
)

type administrationCall func(actor terminal, method, route string, body any, expected int) json.RawMessage

// delegationEnvironment reúne el estado compartido de la aceptación de etapa 2.
type delegationEnvironment struct {
	ctx  context.Context
	pool *pgxpool.Pool
	call administrationCall
	// send hace la petición sin comprobar el resultado (etapa 9F: reenvío concurrente desde otra goroutine).
	send            func(context.Context, terminal, string, string, any) *httptest.ResponseRecorder
	text            func(json.RawMessage, string) string
	device          terminal
	admin           terminal
	supportPassword string
	// service es el servicio que atiende las llamadas (etapa 9F: respaldo controlado de mantenimiento).
	service *administration.Service
}

func (env *delegationEnvironment) login(t *testing.T, username, password string) terminal {
	t.Helper()
	actor := env.device
	actor.session = env.text(env.call(actor, "POST", "auth/login", map[string]string{"username": username, "password": password}, 200), "session_token")
	return actor
}

func (env *delegationEnvironment) createRole(t *testing.T, name string, permissions ...string) string {
	t.Helper()
	return env.text(env.call(env.admin, "POST", "roles", map[string]any{"name": name, "permissions": permissions}, 200), "id")
}

func (env *delegationEnvironment) createUser(t *testing.T, username, roleID string) (string, terminal) {
	t.Helper()
	password := "clave-temporal-" + username
	id := env.text(env.call(env.admin, "POST", "users", map[string]any{
		"username": username, "display_name": "Prueba " + username, "role_id": roleID, "active": true, "password": password,
	}, 200), "id")
	return id, env.login(t, username, password)
}

func (env *delegationEnvironment) scalar(t *testing.T, query string, args ...any) int {
	t.Helper()
	var value int
	if err := env.pool.QueryRow(env.ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func (env *delegationEnvironment) userBody(username, roleID string, active bool) map[string]any {
	return map[string]any{"username": username, "display_name": "Prueba " + username, "role_id": roleID, "active": active}
}

func (env *delegationEnvironment) deniedCount(t *testing.T) int {
	t.Helper()
	return env.scalar(t, "SELECT count(*) FROM app.audit_events WHERE action='access.denied'")
}

// testDelegationRules cubre los hallazgos H1, M1 y la parte observable de H2 por HTTP real.
func testDelegationRules(t *testing.T, env *delegationEnvironment) {
	var adminID, adminRoleID string
	if err := env.pool.QueryRow(env.ctx, "SELECT id,role_id FROM app.users WHERE username='admin'").Scan(&adminID, &adminRoleID); err != nil {
		t.Fatal(err)
	}
	adminPermissions := func() int {
		return env.scalar(t, "SELECT count(*) FROM app.role_permissions WHERE role_id=$1", adminRoleID)
	}
	registered := adminPermissions()

	readerRole := env.createRole(t, "Delegación: lectura de usuarios", "users.read")
	cashRole := env.createRole(t, "Delegación: lectura de cajas", "registers.read")
	usersRole := env.createRole(t, "Delegación: gestión de usuarios", "users.read", "users.manage", "roles.read")
	managerID, manager := env.createUser(t, "gestor_usuarios", usersRole)
	readerID, _ := env.createUser(t, "lector_usuarios", readerRole)

	t.Run("H1_users_manage_no_crea_ni_asigna_roles_superiores", func(t *testing.T) {
		denials := env.deniedCount(t)
		env.call(manager, "POST", "users", map[string]any{
			"username": "escalado", "display_name": "Escalado", "role_id": adminRoleID, "active": true, "password": "clave-escalada-1",
		}, 403)
		env.call(manager, "POST", "users", map[string]any{
			"username": "cajas_ajenas", "display_name": "Cajas", "role_id": cashRole, "active": true, "password": "clave-escalada-2",
		}, 403)
		if env.scalar(t, "SELECT count(*) FROM app.users WHERE username IN ('escalado','cajas_ajenas')") != 0 {
			t.Fatal("Una denegación de delegación creó usuarios")
		}
		if env.deniedCount(t) != denials+2 {
			t.Fatal("Las denegaciones de delegación no quedaron auditadas")
		}
		// Asignar un rol que solo contiene permisos propios sigue permitido.
		env.call(manager, "POST", "users", map[string]any{
			"username": "lector_delegado", "display_name": "Lector delegado", "role_id": readerRole, "active": true, "password": "clave-delegada-1",
		}, 200)
		env.call(manager, "PATCH", "users/"+readerID, env.userBody("lector_usuarios", adminRoleID, true), 403)
		env.call(manager, "PATCH", "users/"+adminID, map[string]any{"username": "admin", "display_name": "Administrador", "role_id": adminRoleID, "active": false}, 403)
	})

	t.Run("H1_nadie_modifica_su_propio_rol", func(t *testing.T) {
		env.call(manager, "PATCH", "users/"+managerID, env.userBody("gestor_usuarios", adminRoleID, true), 403)
		env.call(manager, "PATCH", "users/"+managerID, env.userBody("gestor_usuarios", readerRole, true), 403)
		if env.scalar(t, "SELECT count(*) FROM app.users WHERE id=$1 AND role_id=$2", managerID, usersRole) != 1 {
			t.Fatal("El usuario modificó su propio rol")
		}
		// Editar el nombre propio sin cambiar el rol sigue permitido.
		renamed := env.userBody("gestor_usuarios", usersRole, true)
		renamed["display_name"] = "Gestor renombrado"
		env.call(manager, "PATCH", "users/"+managerID, renamed, 200)
		env.call(env.admin, "PATCH", "users/"+adminID, map[string]any{"username": "admin", "display_name": "Administrador", "role_id": readerRole, "active": true}, 403)
	})

	t.Run("H1_permissions_manage_no_autoconcede_ni_supera_lo_propio", func(t *testing.T) {
		permissionsRole := env.createRole(t, "Delegación: gestión de permisos", "permissions.read", "permissions.manage", "users.read")
		permissionsID, permissions := env.createUser(t, "gestor_permisos", permissionsRole)
		env.call(permissions, "PUT", "users/"+permissionsID+"/permissions", map[string]any{"overrides": map[string]string{"users.manage": "allow"}}, 403)
		env.call(permissions, "PUT", "users/"+permissionsID+"/permissions", map[string]any{"overrides": map[string]string{"users.read": "deny"}}, 403)
		env.call(permissions, "PUT", "users/"+readerID+"/permissions", map[string]any{"overrides": map[string]string{"users.manage": "allow"}}, 403)
		env.call(permissions, "PUT", "users/"+adminID+"/permissions", map[string]any{"overrides": map[string]string{"audit.read": "deny"}}, 403)
		if env.scalar(t, "SELECT count(*) FROM app.user_permissions WHERE user_id IN ($1,$2,$3)", permissionsID, readerID, adminID) != 0 {
			t.Fatal("Una excepción denegada quedó persistida")
		}
		env.call(permissions, "PUT", "users/"+readerID+"/permissions", map[string]any{"overrides": map[string]string{"permissions.read": "allow"}}, 200)

		// Retirar un deny también concede: no puede devolver un permiso que el actor no tiene.
		mixedRole := env.createRole(t, "Delegación: usuarios y cajas", "users.read", "registers.read")
		mixedID, _ := env.createUser(t, "mixto", mixedRole)
		env.call(env.admin, "PUT", "users/"+mixedID+"/permissions", map[string]any{"overrides": map[string]string{"registers.read": "deny"}}, 200)
		env.call(permissions, "PUT", "users/"+mixedID+"/permissions", map[string]any{"overrides": map[string]string{"registers.read": "inherit"}}, 403)
		if env.scalar(t, "SELECT count(*) FROM app.user_permissions WHERE user_id=$1 AND effect='deny'", mixedID) != 1 {
			t.Fatal("Se retiró un deny que otorgaba un permiso ajeno")
		}
	})

	t.Run("H1_roles_manage_no_edita_su_rol_ni_amplia_roles", func(t *testing.T) {
		rolesRole := env.createRole(t, "Delegación: gestión de roles", "roles.read", "roles.manage", "permissions.read", "permissions.manage")
		_, roles := env.createUser(t, "gestor_roles", rolesRole)
		env.call(roles, "PATCH", "roles/"+rolesRole, map[string]any{
			"name": "Delegación: gestión de roles", "permissions": []string{"roles.read", "roles.manage", "permissions.read", "permissions.manage", "audit.read"},
		}, 403)
		// Aun sin ampliar permisos, el rol propio no se edita.
		env.call(roles, "PATCH", "roles/"+rolesRole, map[string]any{
			"name": "Delegación: rol propio renombrado", "permissions": []string{"roles.read", "roles.manage", "permissions.read", "permissions.manage"},
		}, 403)
		env.call(roles, "POST", "roles", map[string]any{"name": "Delegación: ampliado", "permissions": []string{"audit.read"}}, 403)
		env.call(roles, "PATCH", "roles/"+adminRoleID, map[string]any{"name": "Administrador", "permissions": []string{"roles.read"}}, 403)
		subset := env.text(env.call(roles, "POST", "roles", map[string]any{"name": "Delegación: subconjunto", "permissions": []string{"roles.read"}}, 200), "id")
		env.call(roles, "PATCH", "roles/"+subset, map[string]any{"name": "Delegación: subconjunto", "permissions": []string{"roles.read", "audit.read"}}, 403)
		if adminPermissions() != registered || env.scalar(t, "SELECT count(*) FROM app.roles WHERE name='Delegación: ampliado'") != 0 {
			t.Fatal("Una edición de rol denegada modificó datos")
		}
	})

	t.Run("M1_ultimo_administrador_por_permisos_efectivos", func(t *testing.T) {
		// El único administrador no puede desactivarse a sí mismo.
		env.call(env.admin, "PATCH", "users/"+adminID, map[string]any{"username": "admin", "display_name": "Administrador", "role_id": adminRoleID, "active": false}, 400)
		// sa conserva todos los permisos, pero no cuenta como administrador ordinario.
		support := env.login(t, "sa", env.supportPassword)
		env.call(support, "PATCH", "roles/"+adminRoleID, map[string]any{"name": "Administrador", "permissions": []string{}}, 400)
		env.call(support, "PUT", "users/"+adminID+"/permissions", map[string]any{"overrides": map[string]string{"users.manage": "deny"}}, 400)
		env.call(support, "PATCH", "users/"+adminID, map[string]any{"username": "admin", "display_name": "Administrador", "role_id": readerRole, "active": true}, 400)
		if adminPermissions() != registered || env.scalar(t, "SELECT count(*) FROM app.user_permissions WHERE user_id=$1", adminID) != 0 {
			t.Fatal("Se perdió el último administrador por un cambio rechazado")
		}

		// Con un segundo administrador completo, otro rol puede asumir la administración.
		var keys []string
		if err := env.pool.QueryRow(env.ctx, "SELECT array_agg(key ORDER BY key) FROM app.permissions").Scan(&keys); err != nil {
			t.Fatal(err)
		}
		alternateRole := env.text(env.call(support, "POST", "roles", map[string]any{"name": "Delegación: administración alterna", "permissions": keys}, 200), "id")
		alternateID, _ := env.createUser(t, "admin_alterno", alternateRole)
		env.call(support, "PUT", "users/"+adminID+"/permissions", map[string]any{"overrides": map[string]string{"users.manage": "deny"}}, 200)
		// admin conserva el rol Administrador, pero ya no tiene todos los permisos efectivos:
		// desactivar al alterno dejaría al sistema sin administradores completos.
		env.call(support, "PATCH", "users/"+alternateID, map[string]any{"username": "admin_alterno", "display_name": "Prueba admin_alterno", "role_id": alternateRole, "active": false}, 400)
		env.call(support, "PUT", "users/"+adminID+"/permissions", map[string]any{"overrides": map[string]string{"users.manage": "inherit"}}, 200)
		env.call(support, "PATCH", "users/"+alternateID, map[string]any{"username": "admin_alterno", "display_name": "Prueba admin_alterno", "role_id": alternateRole, "active": false}, 200)
	})
}
