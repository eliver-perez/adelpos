//go:build integration

package administration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/httpapi"
)

// pinActor es quien hace la petición: la credencial del dispositivo y, según el caso, la sesión o la credencial de
// envío del mesero.
type pinActor struct {
	device     string
	secret     string
	session    string
	credential string
}

type pinEnvironment struct {
	t       *testing.T
	ctx     context.Context
	pool    *pgxpool.Pool
	handler func(actor pinActor, method, route string, body any) *httptest.ResponseRecorder
}

// call hace la petición y exige el estado; devuelve el cuerpo decodificado.
func (env *pinEnvironment) call(actor pinActor, method, route string, body any, expected int) map[string]any {
	env.t.Helper()
	response := env.handler(actor, method, route, body)
	if response.Code != expected {
		env.t.Fatalf("%s %s: esperado %d, recibido %d: %s", method, route, expected, response.Code, response.Body.String())
	}
	decoded := map[string]any{}
	if strings.HasPrefix(strings.TrimSpace(response.Body.String()), "{") {
		if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
			env.t.Fatal(err)
		}
	}
	return decoded
}

// fails exige el estado y el código de error.
func (env *pinEnvironment) fails(actor pinActor, method, route string, body any, expected int, code string) map[string]any {
	env.t.Helper()
	reply := env.call(actor, method, route, body, expected)
	problem, _ := reply["error"].(map[string]any)
	if problem == nil || problem["code"] != code {
		env.t.Fatalf("%s %s: esperado %s, recibido %v", method, route, code, reply)
	}
	return problem
}

func (env *pinEnvironment) scalar(query string, args ...any) int {
	env.t.Helper()
	var value int
	if err := env.pool.QueryRow(env.ctx, query, args...).Scan(&value); err != nil {
		env.t.Fatal(err)
	}
	return value
}

func pinTestID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	raw[6] = raw[6]&15 | 64
	raw[8] = raw[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

// waiterDevice registra un teléfono o tableta de meseros como si se hubiera emparejado (el emparejamiento se prueba
// aparte en pairing_acceptance_test.go y en el contrato).
func (env *pinEnvironment) waiterDevice(name string) pinActor {
	env.t.Helper()
	secret := administration.NewToken()
	var id string
	err := env.pool.QueryRow(env.ctx, `INSERT INTO app.devices(name,secret_hash,kind,os,app_version,paired_at)
        VALUES ($1,$2,'android','Android 15','0.1.0',now()) RETURNING id`, name, administration.TokenHash(secret)).Scan(&id)
	if err != nil {
		env.t.Fatal(err)
	}
	return pinActor{device: id, secret: secret}
}

// TestStageA1WaiterPIN cubre la fase B de la etapa A1 por HTTP real (httpapi + administration) y PostgreSQL: PIN
// válido, inválido y trivial; bloqueo al quinto fallo y desbloqueo; solo dispositivos android; permiso
// waiter_devices.use; pin_version; la credencial de envío (acepta pedidos sin sesión, solo en esa ruta, con los
// permisos vigentes, inválida tras cambiar o quitar el PIN, desactivar al usuario o revocar el dispositivo); cierre de
// las sesiones android al cambiar el PIN; «Mi PIN»; el bloqueo por inactividad; y que ningún registro guarda el PIN,
// su hash ni la credencial.
func TestStageA1WaiterPIN(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	provisioning := administration.NewToken()
	tokenFile := filepath.Join(t.TempDir(), "provisioning-token")
	if err := os.WriteFile(tokenFile, []byte(provisioning), 0600); err != nil {
		t.Fatal(err)
	}
	service := &administration.Service{Pool: pool, ProvisioningFile: tokenFile}
	handler := httpapi.New(func(ctx context.Context) database.Health {
		return database.Probe(ctx, pool)
	}, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)), service)
	env := &pinEnvironment{t: t, ctx: ctx, pool: pool}
	env.handler = func(actor pinActor, method, route string, body any) *httptest.ResponseRecorder {
		var reader io.Reader = bytes.NewReader(nil)
		if body != nil {
			encoded, _ := json.Marshal(body)
			reader = bytes.NewReader(encoded)
		}
		request := httptest.NewRequestWithContext(context.Background(), method, "/api/v1/"+route, reader)
		request.RemoteAddr = "127.0.0.1:50606"
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Provisioning-Token", provisioning)
		if actor.device != "" {
			request.Header.Set("X-Device-ID", actor.device)
			request.Header.Set("X-Device-Token", actor.secret)
		}
		if actor.session != "" {
			request.Header.Set("Authorization", "Bearer "+actor.session)
		}
		if actor.credential != "" {
			request.Header.Set(administration.OfflineCredentialHeader, actor.credential)
		}
		// Como la aplicación Android: su versión y su tipo (la 0.1.0 pasa con la mínima por tipo de cliente).
		request.Header.Set(httpapi.ClientVersionHeader, "0.1.0")
		request.Header.Set(httpapi.ClientKindHeader, "android")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	// Cada subprueba usa su propio *testing.T en los ayudantes (un fallo no detiene a la prueba principal desde otra).
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

	// Instalación nueva: el asistente concede waiter_devices.use al administrador y al rol Mesero (A1-D2).
	created := env.call(pinActor{}, "POST", "setup", map[string]any{"name": "Restaurante PIN", "timezone": "America/Monterrey",
		"currency": "MXN", "theme": "light", "terminal_name": "Caja principal"}, 200)
	desktop := pinActor{device: fmt.Sprint(created["device_id"]), secret: fmt.Sprint(created["device_token"])}
	admin := desktop
	admin.session = fmt.Sprint(env.call(desktop, "POST", "auth/login", map[string]string{"username": "admin", "password": "admin"}, 200)["session_token"])
	if holders := env.scalar(`SELECT count(*) FROM app.role_permissions rp JOIN app.roles r ON r.id=rp.role_id
        WHERE rp.permission_key='waiter_devices.use' AND r.code IN ('administrator','waiter')`); holders != 2 {
		t.Fatalf("waiter_devices.use en una instalación nueva: %d roles", holders)
	}
	if others := env.scalar(`SELECT count(*) FROM app.role_permissions rp JOIN app.roles r ON r.id=rp.role_id
        WHERE r.code='waiter'`); others != 1 {
		t.Fatalf("el rol Mesero solo debe recibir waiter_devices.use: %d permisos", others)
	}

	// Meseros con los permisos del pedido sin conexión, y una cajera sin el permiso del PIN.
	waiterPermissions := []string{"waiter_devices.use", "orders.confirm", "orders.capture", "accounts.read", "accounts.open",
		"catalog.read", "catalog.prices.read", "restaurant.tables.read"}
	waiterRole := fmt.Sprint(env.call(admin, "POST", "roles", map[string]any{"name": "Meseros A1", "permissions": waiterPermissions}, 200)["id"])
	cashierRole := fmt.Sprint(env.call(admin, "POST", "roles", map[string]any{"name": "Cajeros A1", "permissions": []string{"accounts.read"}}, 200)["id"])
	newUser := func(username, display, role string) string {
		return fmt.Sprint(env.call(admin, "POST", "users", map[string]any{"username": username, "display_name": display,
			"role_id": role, "active": true, "password": "clave-" + username}, 200)["id"])
	}
	ana := newUser("ana.a1", "Ana López", waiterRole)
	beto := newUser("beto.a1", "Beto Ruiz", waiterRole)
	carla := newUser("carla.a1", "Carla Díaz", cashierRole)
	tablet := env.waiterDevice("Tableta 1 A1")
	phone := env.waiterDevice("Teléfono 2 A1")
	colorsReply := env.handler(admin, "GET", "users/badge-colors", nil)
	var colors []map[string]string
	if err := json.Unmarshal(colorsReply.Body.Bytes(), &colors); err != nil || len(colors) != len(administration.BadgePalette) {
		t.Fatalf("paleta del distintivo: %v %s", err, colorsReply.Body.String())
	}
	blue := colors[8]["hex"]

	run("A1B_01_pin_invalido_y_trivial", func(t *testing.T) {
		for _, pin := range []string{"123", "1234567", "12a4", " 4829", "４８２９"} {
			env.fails(admin, "PUT", "users/"+ana+"/pin", map[string]any{"pin": pin}, 400, "VALIDATION_ERROR")
		}
		for _, pin := range []string{"1234", "0000", "1111", "4321", "7890", "1212", "121212", "123123", "2580", "112233", "654321"} {
			env.fails(admin, "PUT", "users/"+ana+"/pin", map[string]any{"pin": pin}, 400, "PIN_TOO_SIMPLE")
		}
		env.fails(admin, "PUT", "users/"+ana+"/pin", map[string]any{"pin": "4829", "badge_color": "#123456"}, 400, "VALIDATION_ERROR")
		env.fails(admin, "PUT", "users/"+ana+"/pin", map[string]any{"pin": "4829", "short_name": strings.Repeat("a", 21)}, 400, "VALIDATION_ERROR")
		// Sin PIN no se puede guardar solo el distintivo.
		env.fails(admin, "PUT", "users/"+ana+"/pin", map[string]any{"short_name": "Ana"}, 400, "VALIDATION_ERROR")
		// Sin users.manage no se asigna.
		cashier := desktop
		cashier.session = fmt.Sprint(env.call(desktop, "POST", "auth/login", map[string]string{"username": "carla.a1", "password": "clave-carla.a1"}, 200)["session_token"])
		env.fails(cashier, "PUT", "users/"+ana+"/pin", map[string]any{"pin": "4829"}, 403, "PERMISSION_DENIED")
		if env.scalar("SELECT count(*) FROM app.users WHERE pin_hash IS NOT NULL") != 0 {
			t.Fatal("un PIN rechazado quedó guardado")
		}
	})

	run("A1B_02_asignar_pin_lista_del_dispositivo", func(t *testing.T) {
		user := env.call(admin, "PUT", "users/"+ana+"/pin", map[string]any{"pin": "4829", "short_name": " Ana ", "badge_color": strings.ToUpper(blue)}, 200)
		if user["has_pin"] != true || user["short_name"] != "Ana" || user["badge_color"] != blue || user["pin_locked_until"] != nil {
			t.Fatalf("usuario con PIN: %v", user)
		}
		var hash string
		var version int
		if err := pool.QueryRow(ctx, "SELECT pin_hash,pin_version FROM app.users WHERE id=$1", ana).Scan(&hash, &version); err != nil ||
			!strings.HasPrefix(hash, "$argon2id$") || strings.Contains(hash, "4829") || version != 1 {
			t.Fatalf("PIN guardado: %q %d %v", hash, version, err)
		}
		env.call(admin, "PUT", "users/"+carla+"/pin", map[string]any{"pin": "7305"}, 200)
		// La lista solo en un dispositivo android y sin sesión.
		env.fails(desktop, "GET", "auth/pin-users", nil, 403, "PIN_DEVICE_NOT_ALLOWED")
		env.fails(pinActor{}, "GET", "auth/pin-users", nil, 401, "DEVICE_REQUIRED")
		list := env.call(tablet, "GET", "auth/pin-users", nil, 200)
		device := list["device"].(map[string]any)
		if device["name"] != "Tableta 1 A1" || device["idle_lock_seconds"] != float64(120) || device["id"] != tablet.device {
			t.Fatalf("dispositivo en la lista: %v", device)
		}
		users := list["users"].([]any)
		if len(users) != 1 {
			t.Fatalf("solo Ana tiene PIN y permiso: %v", users)
		}
		entry := users[0].(map[string]any)
		if entry["id"] != ana || entry["name"] != "Ana" || entry["initial"] != "A" || entry["color"] != blue ||
			entry["pin_version"] != float64(1) || entry["locked_until"] != nil || len(entry) != 6 {
			t.Fatalf("Ana en la lista: %v", entry)
		}
		if env.scalar("SELECT count(*) FROM app.audit_events WHERE action='user.pin_set' AND entity_id=$1 AND after_state->>'set_by'='administrator'", ana) != 1 {
			t.Fatal("no se registró quién definió el PIN")
		}
	})

	var anaSession, anaCredential string
	run("A1B_03_entrada_con_pin_bloqueo_y_desbloqueo", func(t *testing.T) {
		env.fails(desktop, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "4829"}, 403, "PIN_DEVICE_NOT_ALLOWED")
		env.fails(tablet, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "48"}, 400, "VALIDATION_ERROR")
		env.fails(tablet, "POST", "auth/pin-login", map[string]string{"user_id": pinTestID(), "pin": "4829"}, 401, "PIN_INVALID")
		for attempt := 1; attempt <= 4; attempt++ {
			env.fails(tablet, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "4828"}, 401, "PIN_INVALID")
		}
		// El quinto fallo en 15 minutos bloquea el PIN 5 minutos, también desde otro dispositivo y con el PIN correcto.
		locked := env.fails(phone, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "4828"}, 429, "PIN_LOCKED")
		until, err := time.Parse(time.RFC3339Nano, fmt.Sprint(locked["details"].(map[string]any)["locked_until"]))
		if err != nil || until.Sub(time.Now()) < 4*time.Minute || until.Sub(time.Now()) > 6*time.Minute {
			t.Fatalf("bloqueo de 5 minutos: %v %v", locked, err)
		}
		env.fails(tablet, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "4829"}, 429, "PIN_LOCKED")
		if env.scalar(`SELECT count(*) FROM app.audit_events WHERE action='user.pin_locked' AND entity_id=$1 AND device_id=$2`, ana, phone.device) != 1 ||
			env.scalar("SELECT count(*) FROM app.audit_events WHERE action='user.pin_locked'") != 1 {
			t.Fatal("el bloqueo debe registrarse una sola vez, con el dispositivo")
		}
		if env.scalar("SELECT count(*) FROM app.audit_events WHERE action='session.pin_login_failed' AND entity_id=$1", ana) != 6 {
			t.Fatal("las entradas fallidas no quedaron registradas")
		}
		listed := env.call(tablet, "GET", "auth/pin-users", nil, 200)["users"].([]any)[0].(map[string]any)
		if listed["locked_until"] == nil {
			t.Fatal("la lista del dispositivo no dice hasta cuándo está bloqueado")
		}
		users := env.handler(admin, "GET", "users", nil)
		if !strings.Contains(users.Body.String(), `"pin_locked_until":"`) {
			t.Fatal("Personal › Usuarios no ve el bloqueo")
		}
		env.call(admin, "POST", "users/"+ana+"/pin/unlock", map[string]any{}, 200)
		if env.scalar("SELECT count(*) FROM app.audit_events WHERE action='user.pin_unlocked' AND entity_id=$1", ana) != 1 {
			t.Fatal("el desbloqueo no quedó registrado")
		}
		// Desbloquear un PIN que no está bloqueado no registra nada.
		env.call(admin, "POST", "users/"+ana+"/pin/unlock", map[string]any{}, 200)
		if env.scalar("SELECT count(*) FROM app.audit_events WHERE action='user.pin_unlocked'") != 1 {
			t.Fatal("un desbloqueo sin bloqueo quedó registrado")
		}
		login := env.call(tablet, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "4829"}, 200)
		anaSession, anaCredential = fmt.Sprint(login["session_token"]), fmt.Sprint(login["offline_credential"])
		if len(anaSession) != 43 || len(anaCredential) != 43 || login["pin_version"] != float64(1) || anaSession == anaCredential {
			t.Fatalf("respuesta de la entrada con PIN: %v", login)
		}
		if env.scalar("SELECT count(*) FROM app.pin_attempts WHERE user_id=$1", ana) != 0 {
			t.Fatal("un acierto debe borrar los intentos")
		}
		identity := env.call(pinActor{device: tablet.device, secret: tablet.secret, session: anaSession}, "GET", "auth/me", nil, 200)
		if identity["terminal"].(map[string]any)["idle_lock_seconds"] != float64(120) {
			t.Fatalf("la identidad no trae el bloqueo por inactividad: %v", identity["terminal"])
		}
		// Sin waiter_devices.use no entra aunque el PIN sea correcto (y no aparece en la lista).
		env.fails(tablet, "POST", "auth/pin-login", map[string]string{"user_id": carla, "pin": "7305"}, 403, "PERMISSION_DENIED")
	})

	run("A1B_04_cambio_rapido_y_una_sesion_por_usuario_y_dispositivo", func(t *testing.T) {
		env.call(admin, "PUT", "users/"+beto+"/pin", map[string]any{"pin": "6152"}, 200)
		beto1 := env.call(tablet, "POST", "auth/pin-login", map[string]string{"user_id": beto, "pin": "6152"}, 200)
		anaActor := pinActor{device: tablet.device, secret: tablet.secret, session: anaSession}
		betoActor := pinActor{device: tablet.device, secret: tablet.secret, session: fmt.Sprint(beto1["session_token"])}
		env.call(anaActor, "GET", "auth/me", nil, 200)
		env.call(betoActor, "GET", "auth/me", nil, 200)
		// Ana entra otra vez en la misma tableta: solo se reemplaza su sesión y su credencial de envío.
		again := env.call(tablet, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "4829"}, 200)
		env.fails(anaActor, "GET", "auth/me", nil, 401, "SESSION_REPLACED")
		env.call(betoActor, "GET", "auth/me", nil, 200)
		previous := anaCredential
		anaSession, anaCredential = fmt.Sprint(again["session_token"]), fmt.Sprint(again["offline_credential"])
		if env.scalar(`SELECT count(*) FROM app.offline_order_credentials WHERE user_id=$1 AND device_id=$2 AND revoked_at IS NULL`, ana, tablet.device) != 1 ||
			env.scalar(`SELECT count(*) FROM app.offline_order_credentials WHERE user_id=$1 AND revoke_reason='replaced'`, ana) != 1 {
			t.Fatal("una sola credencial de envío vigente por usuario y dispositivo")
		}
		anaCredentialActor := pinActor{device: tablet.device, secret: tablet.secret, credential: previous}
		env.fails(anaCredentialActor, "POST", "restaurant/offline-orders", map[string]any{}, 401, "OFFLINE_CREDENTIAL_INVALID")
		// La credencial no es una sesión: el conteo por tipo solo cuenta las sesiones android vigentes.
		summary := env.call(admin, "GET", "devices/sessions-summary", nil, 200)
		if summary["by_kind"].(map[string]any)["android"] != float64(2) {
			t.Fatalf("sesiones android: %v", summary)
		}
	})

	// Catálogo y mesa para el pedido sin conexión (preparación).
	category := env.call(admin, "POST", "catalog/categories", map[string]any{"name": "Bebidas A1", "active": true}, 200)
	product := env.call(admin, "POST", "catalog/products", map[string]any{"name": "Agua A1", "active": true,
		"category_id": category["id"], "code": "AGUA-A1", "kind": "product", "base_price": "25"}, 200)
	area := env.call(admin, "POST", "restaurant/areas", map[string]any{"operation_id": pinTestID(), "name": "Terraza A1", "active": true}, 200)
	table := env.call(admin, "POST", "restaurant/tables", map[string]any{"operation_id": pinTestID(), "area_id": area["id"],
		"name": "Mesa A1", "shape": "rectangle", "x": 20, "y": 30, "width": 100, "height": 72, "seats": 4, "active": true}, 200)
	// El primer pedido abre la cuenta de la mesa; los siguientes van a esa cuenta.
	var destination map[string]any = map[string]any{"table_id": table["id"], "seen_account_ids": []string{}}
	order := func() map[string]any {
		version := env.scalar("SELECT coalesce(max(version),0) FROM app.catalog_change_log")
		return map[string]any{
			"operation_id":    pinTestID(),
			"catalog_version": version,
			"captured_at":     time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano),
			"destination":     destination,
			"items": []map[string]any{{"id": pinTestID(), "product_id": product["id"], "variant_id": nil, "quantity": 1,
				"customization":   map[string]any{"ingredients": []any{}, "groups": []any{}, "components": []any{}, "notes": ""},
				"seen_unit_price": "25.00"}},
		}
	}
	credentialActor := func() pinActor {
		return pinActor{device: tablet.device, secret: tablet.secret, credential: anaCredential}
	}

	run("A1B_05_credencial_de_envio", func(t *testing.T) {
		accepted := env.call(credentialActor(), "POST", "restaurant/offline-orders", order(), 200)
		if accepted["status"] != "accepted" {
			t.Fatalf("pedido con la credencial de envío: %v", accepted)
		}
		destination = map[string]any{"account_id": accepted["account_id"]}
		if env.scalar(`SELECT count(*) FROM app.audit_events WHERE action='restaurant.offline_order.accepted' AND actor_id=$1 AND device_id=$2`,
			ana, tablet.device) != 1 {
			t.Fatal("el pedido debe quedar a nombre del mesero y del dispositivo")
		}
		if env.scalar("SELECT count(*) FROM app.offline_order_credentials WHERE user_id=$1 AND last_used_at IS NOT NULL AND revoked_at IS NULL", ana) != 1 {
			t.Fatal("no se registró el uso de la credencial")
		}
		// Solo en esa ruta: en cualquier otra, 401 (también con el método equivocado).
		env.fails(credentialActor(), "GET", "restaurant/state", nil, 401, "OFFLINE_CREDENTIAL_NOT_ALLOWED")
		env.fails(credentialActor(), "GET", "restaurant/offline-orders", nil, 401, "OFFLINE_CREDENTIAL_NOT_ALLOWED")
		env.fails(credentialActor(), "GET", "auth/me", nil, 401, "OFFLINE_CREDENTIAL_NOT_ALLOWED")
		both := credentialActor()
		both.session = anaSession
		env.fails(both, "POST", "restaurant/offline-orders", order(), 400, "VALIDATION_ERROR")
		// Desde otro dispositivo no vale, ni con una credencial inventada.
		env.fails(pinActor{device: phone.device, secret: phone.secret, credential: anaCredential}, "POST", "restaurant/offline-orders", order(), 401, "OFFLINE_CREDENTIAL_INVALID")
		env.fails(pinActor{device: tablet.device, secret: tablet.secret, credential: administration.NewToken()}, "POST", "restaurant/offline-orders", order(), 401, "OFFLINE_CREDENTIAL_INVALID")
		// Con los permisos vigentes: sin orders.confirm, 403; sin waiter_devices.use la credencial ya no vale.
		env.call(admin, "PUT", "users/"+ana+"/permissions", map[string]any{"overrides": map[string]string{"orders.confirm": "deny"}}, 200)
		env.fails(credentialActor(), "POST", "restaurant/offline-orders", order(), 403, "PERMISSION_DENIED")
		env.call(admin, "PUT", "users/"+ana+"/permissions", map[string]any{"overrides": map[string]string{"orders.confirm": "inherit", "waiter_devices.use": "deny"}}, 200)
		env.fails(credentialActor(), "POST", "restaurant/offline-orders", order(), 401, "OFFLINE_CREDENTIAL_INVALID")
		if users := env.call(tablet, "GET", "auth/pin-users", nil, 200)["users"].([]any); len(users) != 1 {
			t.Fatalf("sin el permiso, Ana sale de la lista: %v", users)
		}
		env.call(admin, "PUT", "users/"+ana+"/permissions", map[string]any{"overrides": map[string]string{"waiter_devices.use": "inherit"}}, 200)
		env.call(credentialActor(), "POST", "restaurant/offline-orders", order(), 200)
	})

	run("A1B_06_cambiar_pin_cierra_lo_android_y_mi_pin", func(t *testing.T) {
		// Ana también tiene sesión en la caja (contraseña): el PIN no la toca.
		anaDesktop := desktop
		anaDesktop.session = fmt.Sprint(env.call(desktop, "POST", "auth/login", map[string]string{"username": "ana.a1", "password": "clave-ana.a1"}, 200)["session_token"])
		env.call(admin, "PUT", "users/"+ana+"/pin", map[string]any{"pin": "3958", "short_name": "Ana", "badge_color": blue}, 200)
		env.fails(pinActor{device: tablet.device, secret: tablet.secret, session: anaSession}, "GET", "auth/me", nil, 401, "SESSION_REVOKED")
		env.fails(credentialActor(), "POST", "restaurant/offline-orders", order(), 401, "OFFLINE_CREDENTIAL_INVALID")
		env.call(anaDesktop, "GET", "auth/me", nil, 200)
		listed := env.call(tablet, "GET", "auth/pin-users", nil, 200)["users"].([]any)
		for _, entry := range listed {
			if entry.(map[string]any)["id"] == ana && entry.(map[string]any)["pin_version"] != float64(2) {
				t.Fatalf("pin_version después del cambio: %v", entry)
			}
		}
		env.fails(tablet, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "4829"}, 401, "PIN_INVALID")
		// Solo el nombre corto y el color: no cambia el PIN ni cierra nada.
		login := env.call(tablet, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "3958"}, 200)
		anaSession, anaCredential = fmt.Sprint(login["session_token"]), fmt.Sprint(login["offline_credential"])
		env.call(admin, "PUT", "users/"+ana+"/pin", map[string]any{"short_name": "Anita", "badge_color": ""}, 200)
		env.call(pinActor{device: tablet.device, secret: tablet.secret, session: anaSession}, "GET", "auth/me", nil, 200)
		if env.scalar("SELECT pin_version FROM app.users WHERE id=$1", ana) != 2 {
			t.Fatal("cambiar el distintivo no debe cambiar la versión del PIN")
		}
		// «Mi PIN»: contraseña actual, mismas reglas y cierra lo android menos la sesión actual.
		env.fails(anaDesktop, "PUT", "auth/pin", map[string]string{"current_password": "otra", "pin": "5071"}, 401, "INVALID_CREDENTIALS")
		env.fails(anaDesktop, "PUT", "auth/pin", map[string]string{"current_password": "clave-ana.a1", "pin": "1111"}, 400, "PIN_TOO_SIMPLE")
		own := env.call(anaDesktop, "PUT", "auth/pin", map[string]string{"current_password": "clave-ana.a1", "pin": "5071"}, 200)
		if own["has_pin"] != true || own["pin_version"] != float64(3) {
			t.Fatalf("«Mi PIN»: %v", own)
		}
		env.call(anaDesktop, "GET", "auth/me", nil, 200)
		env.fails(pinActor{device: tablet.device, secret: tablet.secret, session: anaSession}, "GET", "auth/me", nil, 401, "SESSION_REVOKED")
		env.fails(credentialActor(), "POST", "restaurant/offline-orders", order(), 401, "OFFLINE_CREDENTIAL_INVALID")
		if env.scalar("SELECT count(*) FROM app.audit_events WHERE action='user.pin_set' AND entity_id=$1 AND after_state->>'set_by'='self' AND actor_id=$1", ana) != 1 {
			t.Fatal("«Mi PIN» no quedó registrado como definido por el propio usuario")
		}
		me := env.call(anaDesktop, "GET", "auth/me", nil, 200)
		if me["user"].(map[string]any)["has_pin"] != true {
			t.Fatalf("la identidad no dice que tiene PIN: %v", me["user"])
		}
		// Sin waiter_devices.use, «Mi PIN» se niega; y el administrador no se da un PIN por la ruta de administración.
		cashier := desktop
		cashier.session = fmt.Sprint(env.call(desktop, "POST", "auth/login", map[string]string{"username": "carla.a1", "password": "clave-carla.a1"}, 200)["session_token"])
		env.fails(cashier, "PUT", "auth/pin", map[string]string{"current_password": "clave-carla.a1", "pin": "5071"}, 403, "PERMISSION_DENIED")
		var adminID string
		if err := pool.QueryRow(ctx, "SELECT id FROM app.users WHERE username='admin'").Scan(&adminID); err != nil {
			t.Fatal(err)
		}
		env.fails(admin, "PUT", "users/"+adminID+"/pin", map[string]any{"pin": "5071"}, 403, "PERMISSION_DENIED")
		env.call(admin, "PUT", "auth/pin", map[string]string{"current_password": "admin", "pin": "8061"}, 200)
	})

	run("A1B_07_quitar_pin_desactivar_y_revocar", func(t *testing.T) {
		betoLogin := env.call(phone, "POST", "auth/pin-login", map[string]string{"user_id": beto, "pin": "6152"}, 200)
		removed := env.call(admin, "POST", "users/"+beto+"/pin/remove", map[string]any{}, 200)
		if removed["has_pin"] != false {
			t.Fatalf("PIN quitado: %v", removed)
		}
		env.fails(pinActor{device: phone.device, secret: phone.secret, session: fmt.Sprint(betoLogin["session_token"])}, "GET", "auth/me", nil, 401, "SESSION_REVOKED")
		env.fails(pinActor{device: phone.device, secret: phone.secret, credential: fmt.Sprint(betoLogin["offline_credential"])},
			"POST", "restaurant/offline-orders", order(), 401, "OFFLINE_CREDENTIAL_INVALID")
		env.fails(admin, "POST", "users/"+beto+"/pin/remove", map[string]any{}, 409, "PIN_NOT_SET")
		for _, entry := range env.call(phone, "GET", "auth/pin-users", nil, 200)["users"].([]any) {
			if entry.(map[string]any)["id"] == beto {
				t.Fatal("un usuario sin PIN sigue en la lista")
			}
		}
		// Desactivar al usuario revoca su credencial (user_disabled).
		login := env.call(tablet, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "5071"}, 200)
		anaCredential = fmt.Sprint(login["offline_credential"])
		env.call(admin, "PATCH", "users/"+ana, map[string]any{"username": "ana.a1", "display_name": "Ana López", "role_id": waiterRole, "active": false}, 200)
		env.fails(credentialActor(), "POST", "restaurant/offline-orders", order(), 401, "OFFLINE_CREDENTIAL_INVALID")
		if env.scalar("SELECT count(*) FROM app.offline_order_credentials WHERE user_id=$1 AND revoke_reason='user_disabled'", ana) != 1 {
			t.Fatal("desactivar al usuario debe revocar su credencial de envío")
		}
		env.fails(tablet, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "5071"}, 401, "PIN_INVALID")
		env.call(admin, "PATCH", "users/"+ana, map[string]any{"username": "ana.a1", "display_name": "Ana López", "role_id": waiterRole, "active": true}, 200)
		login = env.call(tablet, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "5071"}, 200)
		anaCredential = fmt.Sprint(login["offline_credential"])
		// Revocar el dispositivo: DEVICE_REVOKED y sus credenciales de envío revocadas.
		env.call(admin, "POST", "devices/"+tablet.device+"/revoke", map[string]any{}, 200)
		env.fails(credentialActor(), "POST", "restaurant/offline-orders", order(), 401, "DEVICE_REVOKED")
		env.fails(tablet, "GET", "auth/pin-users", nil, 401, "DEVICE_REVOKED")
		if env.scalar("SELECT count(*) FROM app.offline_order_credentials WHERE device_id=$1 AND revoked_at IS NULL", tablet.device) != 0 ||
			env.scalar("SELECT count(*) FROM app.offline_order_credentials WHERE device_id=$1 AND revoke_reason='device_revoked'", tablet.device) != 1 {
			t.Fatal("revocar el dispositivo debe revocar sus credenciales de envío")
		}
	})

	run("A1B_08_bloqueo_por_inactividad", func(t *testing.T) {
		var name, keyboard string
		var register *string
		if err := pool.QueryRow(ctx, "SELECT name,keyboard_mode,register_id FROM app.devices WHERE id=$1", phone.device).Scan(&name, &keyboard, &register); err != nil {
			t.Fatal(err)
		}
		body := func(seconds any) map[string]any {
			values := map[string]any{"name": name, "keyboard_mode": keyboard, "register_id": register}
			if seconds != nil {
				values["idle_lock_seconds"] = seconds
			}
			return values
		}
		saved := env.call(admin, "PATCH", "devices/"+phone.device, body(600), 200)
		if saved["idle_lock_seconds"] != float64(600) {
			t.Fatalf("bloqueo guardado: %v", saved)
		}
		for _, rejected := range []any{29, 3601, 0, "120"} {
			env.call(admin, "PATCH", "devices/"+phone.device, body(rejected), 400)
		}
		env.call(admin, "PATCH", "devices/"+phone.device, body(nil), 200)
		if env.scalar("SELECT idle_lock_seconds FROM app.devices WHERE id=$1", phone.device) != 600 {
			t.Fatal("una edición sin idle_lock_seconds cambió el bloqueo")
		}
		if device := env.call(phone, "GET", "auth/pin-users", nil, 200)["device"].(map[string]any); device["idle_lock_seconds"] != float64(600) {
			t.Fatalf("el dispositivo no recibe su bloqueo: %v", device)
		}
		login := env.call(phone, "POST", "auth/pin-login", map[string]string{"user_id": ana, "pin": "5071"}, 200)
		state := env.call(pinActor{device: phone.device, secret: phone.secret, session: fmt.Sprint(login["session_token"])}, "GET", "restaurant/state", nil, 200)
		if state["terminal"].(map[string]any)["idle_lock_seconds"] != float64(600) {
			t.Fatalf("restaurant/state sin el bloqueo: %v", state["terminal"])
		}
		if _, err := pool.Exec(ctx, "UPDATE app.devices SET idle_lock_seconds=10 WHERE id=$1", phone.device); err == nil {
			t.Fatal("la base aceptó un bloqueo fuera de los límites")
		}
	})

	run("A1B_09_ningun_registro_guarda_secretos", func(t *testing.T) {
		rows, err := pool.Query(ctx, `SELECT action,coalesce(before_state::text,''),coalesce(after_state::text,'') FROM app.audit_events
            WHERE action LIKE 'user.pin%' OR action LIKE 'session.pin%' OR action LIKE 'user.%' OR action LIKE 'restaurant.offline%'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		seen := map[string]bool{}
		for rows.Next() {
			var action, before, after string
			if err := rows.Scan(&action, &before, &after); err != nil {
				t.Fatal(err)
			}
			seen[action] = true
			for _, secret := range []string{"4829", "3958", "5071", "6152", "7305", "8061", "argon2", "pin_hash", anaCredential, anaSession, "secret_hash"} {
				if strings.Contains(before+after, secret) {
					t.Fatalf("%s guarda un secreto (%s): %s %s", action, secret, before, after)
				}
			}
		}
		for _, action := range []string{"user.pin_set", "user.pin_removed", "user.pin_locked", "user.pin_unlocked", "user.pin_badge_changed",
			"session.pin_login", "session.pin_login_failed"} {
			if !seen[action] {
				t.Fatalf("falta el registro %s", action)
			}
		}
	})
}
