//go:build integration

package administration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/database"
)

// TestStageA3FactoryRolesReview es la prueba del revisor independiente de la Fase B de la etapa A3 (§0 de
// docs/etapas/ETAPA_A3_AJUSTES.md: roles de fábrica y «Restablecer permisos predeterminados», decisiones A3-D2 y A3-D6).
// Completa TestStageA3FactoryRoles con lo que esa prueba no cubre: el límite de descuento como regla de delegación del
// restablecimiento, dos confirmaciones a la vez, dos restablecimientos a la vez que juntos dejarían el sistema sin
// administrador, que Go no impone las dependencias al guardar pero cada ruta sigue exigiendo su permiso, y (hallazgo
// del revisor) que el Supervisor de fábrica no debe poder cerrar la sesión del Administrador.
func TestStageA3FactoryRolesReview(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	reset := func() {
		if _, err := pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
			t.Fatal(err)
		}
	}
	reset()
	defer reset()
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	env, service := newPinEnvironment(t, ctx, pool)
	run := func(name string, test func(t *testing.T)) {
		parent := env.t
		t.Run(name, func(t *testing.T) {
			env.t = t
			defer func() {
				env.t = parent
			}()
			test(t)
		})
	}

	created := env.call(pinActor{}, "POST", "setup", map[string]any{"name": "Restaurante revisión A3", "timezone": "America/Monterrey",
		"currency": "MXN", "theme": "light", "terminal_name": "Caja principal"}, 200)
	desktop := pinActor{device: fmt.Sprint(created["device_id"]), secret: fmt.Sprint(created["device_token"])}
	login := func(username, password string) pinActor {
		actor := desktop
		actor.session = fmt.Sprint(env.call(desktop, "POST", "auth/login", map[string]string{"username": username, "password": password}, 200)["session_token"])
		return actor
	}
	admin := login("admin", "admin")
	roleID := func(code string) string {
		var id string
		if err := pool.QueryRow(ctx, "SELECT id FROM app.roles WHERE code=$1", code).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	rolePermissions := func(id string) []string {
		rows, err := pool.Query(ctx, "SELECT permission_key FROM app.role_permissions WHERE role_id=$1 ORDER BY permission_key", id)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		keys := []string{}
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				t.Fatal(err)
			}
			keys = append(keys, key)
		}
		return keys
	}
	patchRole := func(id, name string, permissions []string, limit *string) {
		body := map[string]any{"name": name, "permissions": permissions}
		if limit != nil {
			body["discount_limit"] = *limit
		}
		env.call(admin, "PATCH", "roles/"+id, body, 200)
	}
	newUser := func(username, role string) string {
		return fmt.Sprint(env.call(admin, "POST", "users", map[string]any{"username": username, "display_name": username,
			"role_id": role, "active": true, "password": "clave-" + username}, 200)["id"])
	}
	restored := func(id string) int {
		return env.scalar("SELECT count(*) FROM app.audit_events WHERE action='role.defaults_restored' AND entity_id=$1", id)
	}
	allPermissions := func() []string {
		keys := []string{}
		for _, permission := range administration.Permissions {
			keys = append(keys, permission.Key)
		}
		return keys
	}
	// Un jefe de personal: edita roles y tiene todo lo del Mesero de fábrica, sin límite de descuento propio (0 %).
	staffManager := append(administration.FactoryRolePermissions("waiter"), "roles.read", "roles.manage", "permissions.read",
		"permissions.manage", "users.read")
	managerRole := fmt.Sprint(env.call(admin, "POST", "roles", map[string]any{"name": "Jefe de personal revisión", "permissions": staffManager}, 200)["id"])
	newUser("jefe.rv", managerRole)
	manager := login("jefe.rv", "clave-jefe.rv")

	run("RV_restablecer_respeta_el_limite_de_descuento", func(t *testing.T) {
		// El Mesero tiene un límite de descuento mayor que el del actor: como al editarlo (requireLimitWithin), no se
		// restablece ni se ve la vista previa, aunque el actor tenga todos sus permisos actuales y los de fábrica.
		waiterRole := roleID("waiter")
		factory := administration.FactoryRolePermissions("waiter")
		limit := "10"
		patchRole(waiterRole, "Mesero", factory[1:], &limit)
		problem := env.fails(manager, "GET", "roles/"+waiterRole+"/defaults", nil, 403, "PERMISSION_DENIED")
		if !strings.Contains(fmt.Sprint(problem["message"]), "límite de descuento") {
			t.Fatalf("vista previa con un límite mayor que el del actor: %v", problem)
		}
		env.fails(manager, "POST", "roles/"+waiterRole+"/defaults", map[string]any{"expected": factory[1:]}, 403, "PERMISSION_DENIED")
		if got := rolePermissions(waiterRole); !slices.Equal(got, factory[1:]) || restored(waiterRole) != 0 {
			t.Fatalf("un restablecimiento rechazado por el límite cambió el rol: %v", got)
		}
		// Sin el límite mayor, el mismo actor sí lo restablece.
		empty := ""
		patchRole(waiterRole, "Mesero", factory[1:], &empty)
		preview := env.call(manager, "GET", "roles/"+waiterRole+"/defaults", nil, 200)
		env.call(manager, "POST", "roles/"+waiterRole+"/defaults", map[string]any{"expected": preview["current"]}, 200)
		if !slices.Equal(rolePermissions(waiterRole), factory) || restored(waiterRole) != 1 {
			t.Fatal("el jefe de personal debía poder restablecer el Mesero")
		}
	})

	run("RV_dos_confirmaciones_a_la_vez", func(t *testing.T) {
		// Dos ventanas confirman la misma vista previa al mismo tiempo: el lock administrativo las serializa, una cambia
		// el rol y la otra responde changed false, con un solo registro en el Registro de movimientos.
		cashierRole := roleID("cashier")
		factory := administration.FactoryRolePermissions("cashier")
		patchRole(cashierRole, "Cajero", factory[3:], nil)
		preview := env.call(admin, "GET", "roles/"+cashierRole+"/defaults", nil, 200)
		body := map[string]any{"expected": preview["current"]}
		codes := make([]int, 2)
		changed := make([]any, 2)
		var group sync.WaitGroup
		for index := range codes {
			group.Add(1)
			go func(index int) {
				defer group.Done()
				response := env.handler(admin, "POST", "roles/"+cashierRole+"/defaults", body)
				codes[index] = response.Code
				decoded := map[string]any{}
				_ = json.Unmarshal(response.Body.Bytes(), &decoded)
				changed[index] = decoded["changed"]
			}(index)
		}
		group.Wait()
		if codes[0] != 200 || codes[1] != 200 {
			t.Fatalf("dos confirmaciones a la vez: %v", codes)
		}
		if !(changed[0] == true && changed[1] == false) && !(changed[0] == false && changed[1] == true) {
			t.Fatalf("una debía cambiar el rol y la otra no: %v", changed)
		}
		if !slices.Equal(rolePermissions(cashierRole), factory) || restored(cashierRole) != 1 {
			t.Fatalf("dos confirmaciones: %v, registros %d", rolePermissions(cashierRole), restored(cashierRole))
		}
	})

	run("RV_dependencias_no_impuestas_y_la_ruta_valida", func(t *testing.T) {
		// A3-D4: Go guarda un rol sin una dependencia (la interfaz la habría marcado sola), pero cada ruta sigue exigiendo
		// su permiso: «Abrir cuentas» sin «Consultar cuentas e importes» no abre ninguna cuenta.
		partial := fmt.Sprint(env.call(admin, "POST", "roles", map[string]any{"name": "Sin consultar cuentas",
			"permissions": []string{"accounts.open", "restaurant.areas.read", "restaurant.tables.read"}}, 200)["id"])
		newUser("parcial.rv", partial)
		user := login("parcial.rv", "clave-parcial.rv")
		env.fails(user, "POST", "restaurant/accounts", map[string]any{"operation_id": pinTestID(), "mode": "dine_in",
			"table_id": pinTestID(), "seen_account_ids": []string{}, "items": []map[string]any{}}, 403, "PERMISSION_DENIED")
		if env.scalar("SELECT count(*) FROM app.restaurant_accounts") != 0 {
			t.Fatal("se abrió una cuenta sin consultar cuentas")
		}
	})

	run("RV_dos_restablecimientos_no_dejan_sin_administrador", func(t *testing.T) {
		// Los dos únicos administradores completos están en el Supervisor y en Finanzas (con todos los permisos) y el
		// soporte restablece los dos roles a la vez. Cada restablecimiento solo deja un administrador, pero juntos no
		// dejarían ninguno: el lock administrativo los serializa y el segundo se rechaza.
		supervisorRole, financeRole := roleID("supervisor"), roleID("finance")
		all := allPermissions()
		patchRole(supervisorRole, "Supervisor", all, nil)
		patchRole(financeRole, "Finanzas", all, nil)
		newUser("jefa.rv", supervisorRole)
		newUser("contadora.rv", financeRole)
		supportPassword := administration.NewToken()
		if err := service.ProvisionSupport(ctx, supportPassword, false); err != nil {
			t.Fatal(err)
		}
		support := login("sa", supportPassword)
		env.call(support, "PATCH", "users/"+adminUserID(t, pool), map[string]any{"username": "admin", "display_name": "Administrador",
			"role_id": roleID("administrator"), "active": false}, 200)
		roles := []string{supervisorRole, financeRole}
		codes := make([]int, len(roles))
		var group sync.WaitGroup
		for index, id := range roles {
			group.Add(1)
			go func(index int, id string) {
				defer group.Done()
				response := env.handler(support, "POST", "roles/"+id+"/defaults", map[string]any{"expected": all})
				codes[index] = response.Code
			}(index, id)
		}
		group.Wait()
		slices.Sort(codes)
		if !slices.Equal(codes, []int{200, 400}) {
			t.Fatalf("dos restablecimientos que juntos dejarían el sistema sin administrador: %v", codes)
		}
		full := 0
		for _, id := range roles {
			if len(rolePermissions(id)) == len(all) {
				full++
			}
		}
		if full != 1 || restored(supervisorRole)+restored(financeRole) != 1 {
			t.Fatalf("debía quedar un administrador completo: %d roles completos", full)
		}
		// Vuelve el administrador para las subpruebas siguientes.
		env.call(support, "PATCH", "users/"+adminUserID(t, pool), map[string]any{"username": "admin", "display_name": "Administrador",
			"role_id": roleID("administrator"), "active": true}, 200)
		admin = login("admin", "admin")
	})

	run("RV_supervisor_de_fabrica_no_cierra_la_sesion_del_administrador", func(t *testing.T) {
		// Hallazgo del revisor: el Supervisor de fábrica trae sessions.manage («Revocar sesiones», grupo Personal) y
		// POST sessions/{id}/revoke no aplica la regla de delegación (delegation.go: nadie administra a un usuario con
		// más permisos que los suyos), así que un supervisor de una instalación nueva cierra las sesiones del
		// Administrador cuando quiera. Un supervisor de fábrica no debe poder hacerlo.
		supervisorRole := roleID("supervisor")
		empty := ""
		patchRole(supervisorRole, "Supervisor", administration.FactoryRolePermissions("supervisor"), &empty)
		newUser("supervisora.rv", supervisorRole)
		supervisor := login("supervisora.rv", "clave-supervisora.rv")
		var adminSession string
		if err := pool.QueryRow(ctx, `SELECT s.id FROM app.sessions s JOIN app.users u ON u.id=s.user_id
            WHERE u.username='admin' AND s.revoked_at IS NULL`).Scan(&adminSession); err != nil {
			t.Fatal(err)
		}
		// La lista de sesiones (sessions.read) muestra la del Administrador, con su botón de revocar en la pantalla.
		listed := env.handler(supervisor, "GET", "sessions", nil)
		if listed.Code != 200 || !strings.Contains(listed.Body.String(), adminSession) {
			t.Fatalf("lista de sesiones del supervisor: %d", listed.Code)
		}
		revoke := env.handler(supervisor, "POST", "sessions/"+adminSession+"/revoke", map[string]any{})
		me := env.handler(admin, "GET", "auth/me", nil)
		if revoke.Code != 403 || me.Code != 200 {
			t.Fatalf("el Supervisor de fábrica cerró la sesión del Administrador: revocar %d (%s), auth/me del administrador %d (%s)",
				revoke.Code, strings.TrimSpace(revoke.Body.String()), me.Code, strings.TrimSpace(me.Body.String()))
		}
	})
}
