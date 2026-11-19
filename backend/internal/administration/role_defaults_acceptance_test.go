//go:build integration

package administration_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/database"
)

// TestStageA3FactoryRoles cubre los roles de fábrica y «Restablecer permisos predeterminados» de la etapa A3 (§3.1,
// decisiones A3-D2 y A3-D6) por HTTP real (httpapi + administration) y PostgreSQL: una instalación nueva trae cada rol
// de fábrica con su conjunto; un mesero nuevo con su PIN abre una cuenta, captura y envía sin tocar permisos, y Go le
// rechaza lo que su rol no tiene; el catálogo agrupado; el restablecimiento con su vista previa, su confirmación contra
// lo mostrado (ROLE_CHANGED), su repetición sin cambios y su auditoría, sin tocar el límite de descuento ni las
// excepciones por usuario; y las mismas reglas que PATCH roles/{id}: sin escalada, nunca el rol propio, solo los roles
// de fábrica y nunca sin un administrador.
func TestStageA3FactoryRoles(t *testing.T) {
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

	created := env.call(pinActor{}, "POST", "setup", map[string]any{"name": "Restaurante A3", "timezone": "America/Monterrey",
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
	patchRole := func(id, name string, permissions []string) {
		env.call(admin, "PATCH", "roles/"+id, map[string]any{"name": name, "permissions": permissions}, 200)
	}
	newUser := func(username, role string) string {
		return fmt.Sprint(env.call(admin, "POST", "users", map[string]any{"username": username, "display_name": username,
			"role_id": role, "active": true, "password": "clave-" + username}, 200)["id"])
	}
	restored := func(id string) int {
		return env.scalar("SELECT count(*) FROM app.audit_events WHERE action='role.defaults_restored' AND entity_id=$1", id)
	}
	stringsOf := func(value any) []string {
		keys := []string{}
		for _, item := range value.([]any) {
			keys = append(keys, fmt.Sprint(item))
		}
		sort.Strings(keys)
		return keys
	}

	run("A3_instalacion_nueva_roles_de_fabrica", func(t *testing.T) {
		for _, code := range []string{"waiter", "cashier", "kitchen", "supervisor", "inventory", "finance"} {
			expected := administration.FactoryRolePermissions(code)
			if len(expected) == 0 {
				t.Fatalf("%s no tiene conjunto de fábrica", code)
			}
			if got := rolePermissions(roleID(code)); !slices.Equal(got, expected) {
				t.Fatalf("%s en una instalación nueva: %v\nse esperaba %v", code, got, expected)
			}
		}
		if got := len(rolePermissions(roleID("administrator"))); got != len(administration.Permissions) {
			t.Fatalf("el Administrador debía tener los %d permisos: %d", len(administration.Permissions), got)
		}
	})

	run("A3_mesero_nuevo_con_pin_atiende_una_mesa_sin_tocar_permisos", func(t *testing.T) {
		category := env.call(admin, "POST", "catalog/categories", map[string]any{"name": "Platos A3", "active": true}, 200)
		product := env.call(admin, "POST", "catalog/products", map[string]any{"name": "Enchiladas A3", "active": true,
			"category_id": category["id"], "code": "ENCH-A3", "kind": "product", "base_price": "95"}, 200)
		area := env.call(admin, "POST", "restaurant/areas", map[string]any{"operation_id": pinTestID(), "name": "Salón A3", "active": true}, 200)
		table := env.call(admin, "POST", "restaurant/tables", map[string]any{"operation_id": pinTestID(), "area_id": area["id"],
			"name": "Mesa 1 A3", "shape": "rectangle", "x": 20, "y": 30, "width": 100, "height": 72, "seats": 4, "active": true}, 200)
		waiterID := newUser("mesero.a3", roleID("waiter"))
		env.call(admin, "PUT", "users/"+waiterID+"/pin", map[string]any{"pin": "4829"}, 200)
		tablet := env.waiterDevice("Tableta A3")
		entered := env.call(tablet, "POST", "auth/pin-login", map[string]string{"user_id": waiterID, "pin": "4829"}, 200)
		waiter := tablet
		waiter.session = fmt.Sprint(entered["session_token"])
		state := env.call(waiter, "GET", "restaurant/state", nil, 200)
		if got := stringsOf(state["user"].(map[string]any)["permissions"]); !slices.Equal(got, administration.FactoryRolePermissions("waiter")) {
			t.Fatalf("permisos de la sesión con PIN del mesero de fábrica: %v", got)
		}
		env.call(waiter, "GET", "restaurant/overview", nil, 200)
		customization := map[string]any{"ingredients": []any{}, "groups": []any{}, "components": []any{}, "notes": ""}
		opened := env.call(waiter, "POST", "restaurant/accounts", map[string]any{"operation_id": pinTestID(), "mode": "dine_in",
			"table_id": table["id"], "seen_account_ids": []string{}, "items": []map[string]any{{"id": pinTestID(),
				"product_id": product["id"], "variant_id": nil, "quantity": 2, "customization": customization}}}, 200)
		account := opened["account"].(map[string]any)
		route := "restaurant/accounts/" + fmt.Sprint(account["id"])
		preview := env.call(waiter, "GET", route+"/preview", nil, 200)
		sent := env.call(waiter, "POST", route+"/confirm", map[string]any{"operation_id": pinTestID(),
			"revision": account["revision"], "expected_fingerprint": preview["fingerprint"]}, 200)
		if confirmed := sent["account"].(map[string]any)["confirmed_quantity"]; confirmed != float64(2) {
			t.Fatalf("el pedido del mesero no se envió a cocina: %v", sent["account"])
		}
		// El servidor rechaza lo que el rol no tiene aunque una pantalla lo ofreciera: el Mesero de fábrica no cancela
		// unidades (opcional, §3.1), no cobra ni administra.
		lines := sent["lines"].([]any)
		version := lines[0].(map[string]any)["version_id"]
		env.fails(waiter, "POST", route+"/cancel", map[string]any{"operation_id": pinTestID(),
			"revision": sent["account"].(map[string]any)["revision"], "version_id": version, "quantity": 1,
			"reason": "El cliente cambió de opinión"}, 403, "PERMISSION_DENIED")
		env.fails(waiter, "GET", "payments/methods", nil, 403, "PERMISSION_DENIED")
		env.fails(waiter, "GET", "roles/"+roleID("waiter")+"/defaults", nil, 403, "PERMISSION_DENIED")
	})

	run("A3_catalogo_de_permisos", func(t *testing.T) {
		catalog := env.call(admin, "GET", "permissions/catalog", nil, 200)
		if len(catalog["permissions"].([]any)) != len(administration.Permissions) || len(catalog["groups"].([]any)) != 10 ||
			len(catalog["capabilities"].([]any)) != 11 || len(catalog["factory_roles"].([]any)) != 6 {
			t.Fatalf("catálogo: %d permisos, %d grupos, %d capacidades, %d roles de fábrica", len(catalog["permissions"].([]any)),
				len(catalog["groups"].([]any)), len(catalog["capabilities"].([]any)), len(catalog["factory_roles"].([]any)))
		}
		// Revisión R2 (A3-D17): el conjunto de cada rol de fábrica, para que la pantalla no ofrezca lo que Go rechaza.
		if got := stringsOf(catalog["factory_permissions"].(map[string]any)["waiter"]); !slices.Equal(got, administration.FactoryRolePermissions("waiter")) {
			t.Fatalf("conjunto de fábrica del Mesero en el catálogo: %v", got)
		}
		first := catalog["groups"].([]any)[0].(map[string]any)
		if first["name"] != "Mesas y cuentas" {
			t.Fatalf("primer grupo: %v", first["name"])
		}
		newUser("cocina.a3", roleID("kitchen"))
		cook := login("cocina.a3", "clave-cocina.a3")
		env.fails(cook, "GET", "permissions/catalog", nil, 403, "PERMISSION_DENIED")
	})

	run("A3_restablecer_con_vista_previa_y_auditoria", func(t *testing.T) {
		waiterRole := roleID("waiter")
		factory := administration.FactoryRolePermissions("waiter")
		changed := []string{"orders.edit", "orders.cancel"}
		for _, key := range factory {
			if key != "takeaway.deliver" && key != "payments.tickets.print" {
				changed = append(changed, key)
			}
		}
		env.call(admin, "PATCH", "roles/"+waiterRole, map[string]any{"name": "Mesero", "permissions": changed, "discount_limit": "5"}, 200)
		// Una excepción por usuario que el restablecimiento no toca.
		waiterUser := newUser("mesera.a3", waiterRole)
		env.call(admin, "PUT", "users/"+waiterUser+"/permissions", map[string]any{"overrides": map[string]string{"orders.edit": "deny"}}, 200)
		preview := env.call(admin, "GET", "roles/"+waiterRole+"/defaults", nil, 200)
		if got := stringsOf(preview["add"]); !slices.Equal(got, []string{"payments.tickets.print", "takeaway.deliver"}) {
			t.Fatalf("se agregará: %v", got)
		}
		if got := stringsOf(preview["remove"]); !slices.Equal(got, []string{"orders.cancel", "orders.edit"}) {
			t.Fatalf("se quitará: %v", got)
		}
		if got := stringsOf(preview["factory"]); !slices.Equal(got, factory) {
			t.Fatalf("conjunto de fábrica: %v", got)
		}
		if preview["role"].(map[string]any)["code"] != "waiter" || restored(waiterRole) != 0 {
			t.Fatalf("vista previa: %v", preview["role"])
		}
		// La vista previa no escribe nada.
		if got := rolePermissions(waiterRole); len(got) != len(changed) {
			t.Fatalf("la vista previa cambió el rol: %v", got)
		}
		done := env.call(admin, "POST", "roles/"+waiterRole+"/defaults", map[string]any{"expected": preview["current"]}, 200)
		if done["changed"] != true || !slices.Equal(rolePermissions(waiterRole), factory) {
			t.Fatalf("restablecer: %v; quedó %v", done, rolePermissions(waiterRole))
		}
		if restored(waiterRole) != 1 || env.scalar(`SELECT count(*) FROM app.audit_events WHERE action='role.defaults_restored'
            AND entity_id=$1 AND jsonb_array_length(before_state->'permissions')=$2 AND after_state->'added' @> '["takeaway.deliver"]'
            AND after_state->'removed' @> '["orders.edit"]' AND success`, waiterRole, len(changed)) != 1 {
			t.Fatal("el restablecimiento no quedó en el Registro de movimientos con lo de antes, lo agregado y lo quitado")
		}
		if limit := env.scalar("SELECT max_percent::int FROM app.role_discount_limits WHERE role_id=$1", waiterRole); limit != 5 {
			t.Fatalf("el restablecimiento cambió el límite de descuento: %d", limit)
		}
		if env.scalar("SELECT count(*) FROM app.user_permissions WHERE user_id=$1 AND permission_key='orders.edit' AND effect='deny'", waiterUser) != 1 {
			t.Fatal("el restablecimiento quitó la excepción del usuario")
		}
		// Repetir la confirmación (por ejemplo, si se perdió la respuesta) no cambia ni registra nada más.
		again := env.call(admin, "POST", "roles/"+waiterRole+"/defaults", map[string]any{"expected": preview["current"]}, 200)
		if again["changed"] != false || restored(waiterRole) != 1 {
			t.Fatalf("repetir el restablecimiento: %v (registros %d)", again, restored(waiterRole))
		}
		// Sin la lista de la vista previa no se confirma.
		env.fails(admin, "POST", "roles/"+waiterRole+"/defaults", map[string]any{}, 400, "VALIDATION_ERROR")
	})

	run("A3_restablecer_rechaza_si_el_rol_cambio", func(t *testing.T) {
		cashierRole := roleID("cashier")
		factory := administration.FactoryRolePermissions("cashier")
		patchRole(cashierRole, "Cajero", factory[1:])
		preview := env.call(admin, "GET", "roles/"+cashierRole+"/defaults", nil, 200)
		// Otra persona cambia el rol entre la vista previa y la confirmación.
		patchRole(cashierRole, "Cajero", factory[2:])
		env.fails(admin, "POST", "roles/"+cashierRole+"/defaults", map[string]any{"expected": preview["current"]}, 409, "ROLE_CHANGED")
		if got := rolePermissions(cashierRole); !slices.Equal(got, factory[2:]) || restored(cashierRole) != 0 {
			t.Fatalf("un restablecimiento rechazado cambió el rol: %v", got)
		}
		fresh := env.call(admin, "GET", "roles/"+cashierRole+"/defaults", nil, 200)
		env.call(admin, "POST", "roles/"+cashierRole+"/defaults", map[string]any{"expected": fresh["current"]}, 200)
		if !slices.Equal(rolePermissions(cashierRole), factory) {
			t.Fatal("con la vista previa nueva sí se restablece")
		}
	})

	run("A3_supervisor_cierra_sesiones_solo_de_quien_puede_administrar", func(t *testing.T) {
		// Revisión R1 (A3-D16): el Supervisor de fábrica conserva «Revocar sesiones», con la regla de delegación: cierra
		// la sesión de un cajero (sus permisos están dentro de los del supervisor), no la del Administrador.
		newUser("supervisor.a3", roleID("supervisor"))
		supervisor := login("supervisor.a3", "clave-supervisor.a3")
		newUser("cajero.a3", roleID("cashier"))
		cashier := login("cajero.a3", "clave-cajero.a3")
		sessionOf := func(username string) string {
			var id string
			if err := pool.QueryRow(ctx, `SELECT s.id FROM app.sessions s JOIN app.users u ON u.id=s.user_id
                WHERE u.username=$1 AND s.revoked_at IS NULL ORDER BY s.created_at DESC LIMIT 1`, username).Scan(&id); err != nil {
				t.Fatal(err)
			}
			return id
		}
		problem := env.fails(supervisor, "POST", "sessions/"+sessionOf("admin")+"/revoke", map[string]any{}, 403, "PERMISSION_DENIED")
		if problem["message"] != "No puedes cerrar la sesión de un usuario con permisos que tú no tienes." {
			t.Fatalf("cerrar la sesión del Administrador: %v", problem)
		}
		env.call(admin, "GET", "auth/me", nil, 200)
		env.call(supervisor, "POST", "sessions/"+sessionOf("cajero.a3")+"/revoke", map[string]any{}, 200)
		env.fails(cashier, "GET", "auth/me", nil, 401, "SESSION_REVOKED")
		if env.scalar("SELECT count(*) FROM app.audit_events WHERE action='session.revoked' AND actor_username='supervisor.a3'") != 1 {
			t.Fatal("el cierre de la sesión del cajero no quedó en el Registro de movimientos")
		}
	})

	run("A3_restablecer_sin_escalada_ni_rol_propio", func(t *testing.T) {
		supervisorRole := roleID("supervisor")
		// Un jefe de personal que edita roles pero no tiene, por ejemplo, autorizar devoluciones.
		manager := administration.FactoryRolePermissions("cashier")
		manager = append(manager, "roles.read", "roles.manage", "permissions.read", "permissions.manage", "users.read")
		managerRole := fmt.Sprint(env.call(admin, "POST", "roles", map[string]any{"name": "Jefe de personal A3", "permissions": manager}, 200)["id"])
		newUser("jefe.a3", managerRole)
		boss := login("jefe.a3", "clave-jefe.a3")
		patchRole(supervisorRole, "Supervisor", []string{"users.read"})
		problem := env.fails(boss, "GET", "roles/"+supervisorRole+"/defaults", nil, 403, "PERMISSION_DENIED")
		if !strings.Contains(fmt.Sprint(problem["message"]), "No puedes conceder") {
			t.Fatalf("escalada por la vista previa: %v", problem)
		}
		env.fails(boss, "POST", "roles/"+supervisorRole+"/defaults", map[string]any{"expected": []string{"users.read"}}, 403, "PERMISSION_DENIED")
		if got := rolePermissions(supervisorRole); !slices.Equal(got, []string{"users.read"}) || restored(supervisorRole) != 0 {
			t.Fatalf("una escalada rechazada cambió el rol: %v", got)
		}
		// Un rol con un permiso que el actor no tiene tampoco se restablece (como al editarlo).
		patchRole(supervisorRole, "Supervisor", []string{"users.read", "audit.read"})
		problem = env.fails(boss, "GET", "roles/"+supervisorRole+"/defaults", nil, 403, "PERMISSION_DENIED")
		if !strings.Contains(fmt.Sprint(problem["message"]), "No puedes editar un rol") {
			t.Fatalf("rol con permisos que el actor no tiene: %v", problem)
		}
		// Quien puede editar roles no restablece el propio: el Cajero con permisos de roles no toca el Cajero.
		patchRole(roleID("cashier"), "Cajero", manager)
		newUser("cajera.a3", roleID("cashier"))
		cashier := login("cajera.a3", "clave-cajera.a3")
		problem = env.fails(cashier, "POST", "roles/"+roleID("cashier")+"/defaults", map[string]any{"expected": manager}, 403, "PERMISSION_DENIED")
		if !strings.Contains(fmt.Sprint(problem["message"]), "propio rol") {
			t.Fatalf("restablecer el rol propio: %v", problem)
		}
		// Sin permisos de roles, nada.
		cook := login("cocina.a3", "clave-cocina.a3")
		env.fails(cook, "GET", "roles/"+roleID("waiter")+"/defaults", nil, 403, "PERMISSION_DENIED")
	})

	run("A3_restablecer_solo_roles_de_fabrica", func(t *testing.T) {
		custom := fmt.Sprint(env.call(admin, "POST", "roles", map[string]any{"name": "Rol propio A3", "permissions": []string{"users.read"}}, 200)["id"])
		for _, id := range []string{custom, roleID("administrator")} {
			problem := env.fails(admin, "GET", "roles/"+id+"/defaults", nil, 400, "VALIDATION_ERROR")
			if !strings.Contains(fmt.Sprint(problem["message"]), "roles de fábrica") {
				t.Fatalf("rol sin permisos predeterminados: %v", problem)
			}
			env.fails(admin, "POST", "roles/"+id+"/defaults", map[string]any{"expected": []string{}}, 400, "VALIDATION_ERROR")
		}
		env.fails(admin, "GET", "roles/"+roleID("support")+"/defaults", nil, 403, "PERMISSION_DENIED")
		env.fails(admin, "GET", "roles/"+pinTestID()+"/defaults", nil, 404, "NOT_FOUND")
	})

	run("A3_restablecer_nunca_deja_sin_administrador", func(t *testing.T) {
		// El único administrador completo queda en el rol Supervisor (con todos los permisos) y lo restablece el soporte
		// reservado, que tiene todos pero no cuenta como administrador: Go lo rechaza y nada cambia.
		supervisorRole := roleID("supervisor")
		all := []string{}
		for _, permission := range administration.Permissions {
			all = append(all, permission.Key)
		}
		patchRole(supervisorRole, "Supervisor", all)
		newUser("jefa.a3", supervisorRole)
		supportPassword := administration.NewToken()
		if err := service.ProvisionSupport(ctx, supportPassword, false); err != nil {
			t.Fatal(err)
		}
		support := login("sa", supportPassword)
		adminID := adminUserID(t, pool)
		env.call(support, "PATCH", "users/"+adminID, map[string]any{"username": "admin", "display_name": "Administrador",
			"role_id": roleID("administrator"), "active": false}, 200)
		preview := env.call(support, "GET", "roles/"+supervisorRole+"/defaults", nil, 200)
		problem := env.fails(support, "POST", "roles/"+supervisorRole+"/defaults", map[string]any{"expected": preview["current"]}, 400, "VALIDATION_ERROR")
		if !strings.Contains(fmt.Sprint(problem["message"]), "administrador") {
			t.Fatalf("quedarse sin administrador: %v", problem)
		}
		if got := rolePermissions(supervisorRole); len(got) != len(all) || restored(supervisorRole) != 0 {
			t.Fatalf("el restablecimiento rechazado cambió el Supervisor: %d permisos", len(got))
		}
	})
}
