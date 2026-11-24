//go:build integration

package administration_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/licensing/licensingtest"
)

// Etapa L1, Fase C: la puerta en las rutas que no llevan sesión de usuario y en el canal de eventos, por la API v1
// contra PostgreSQL: el asistente y la entrada siempre; emparejar terminales es administración («activar la computadora
// principal antes de emparejar»), salvo que falte la identidad (503 con la causa real); la entrada con PIN, los avisos
// de platillos listos y el canal de eventos siguen con la hora incorrecta; license.changed llega a todas las personas
// que leen el canal; y las rutas de la hora solo se aceptan en la computadora principal.
func TestStageL1LicenseGateWithoutSession(t *testing.T) {
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
	env, service := newPinEnvironment(t, ctx, pool)
	clock := licensingtest.NewClock(time.Now().UTC().Truncate(time.Second))
	licenses := licensingtest.New(t, clock)
	licenses.Database = licensing.PoolProbe{Pool: pool}
	manager := licenses.Start()
	service.License = manager
	service.Seats = administration.LicenseSeats(manager)

	// Sin activar: el asistente y la entrada siempre.
	created := env.call(pinActor{}, "POST", "setup", map[string]any{"name": "Restaurante licencia", "timezone": "America/Monterrey",
		"currency": "MXN", "theme": "light", "terminal_name": "Caja principal"}, 200)
	desktop := pinActor{device: fmt.Sprint(created["device_id"]), secret: fmt.Sprint(created["device_token"])}
	admin := desktop
	admin.session = fmt.Sprint(env.call(desktop, "POST", "auth/login", map[string]string{"username": "admin", "password": "admin"}, 200)["session_token"])
	pairing := map[string]string{"name": "Tableta nueva", "device_kind": "android", "os": "Android 15", "app_version": "0.4.0",
		"commitment": strings.Repeat("a", 64)}
	// Sin identidad, el emparejamiento responde con la causa real.
	env.fails(pinActor{}, "POST", "pairing/requests", pairing, 503, "PAIRING_UNAVAILABLE")
	service.Identity = &administration.ServerIdentity{ServerID: licenses.Installation.ServerID}
	problem := env.fails(pinActor{}, "POST", "pairing/requests", pairing, 409, "LICENSE_NOT_ACTIVATED")
	if !strings.Contains(fmt.Sprint(problem["message"]), "Avisa al encargado") {
		t.Fatalf("mensaje para los clientes anteriores: %v", problem)
	}
	env.fails(admin, "POST", "roles", map[string]any{"name": "Meseros L1", "permissions": []string{"accounts.read"}}, 409,
		"LICENSE_NOT_ACTIVATED")

	// Vigente: se empareja y se administra.
	licenses.Accept(manager, licensingtest.Perpetual(3, 3, 3))
	if reply := env.handler(pinActor{}, "POST", "pairing/requests", pairing); reply.Code == 409 {
		t.Fatalf("con licencia, la puerta deja emparejar: %s", reply.Body.String())
	}
	waiterRole := fmt.Sprint(env.call(admin, "POST", "roles", map[string]any{"name": "Meseros L1", "permissions": []string{
		"waiter_devices.use", "orders.capture", "accounts.read", "accounts.open", "catalog.read", "catalog.prices.read",
		"restaurant.tables.read", "events.read"}}, 200)["id"])
	waiterID := fmt.Sprint(env.call(admin, "POST", "users", map[string]any{"username": "lucia.l1", "display_name": "Lucía L1",
		"role_id": waiterRole, "active": true, "password": "clave-lucia.l1"}, 200)["id"])
	env.call(admin, "PUT", "users/"+waiterID+"/pin", map[string]any{"pin": "4829", "short_name": "Lucía", "badge_color": ""}, 200)
	tablet := env.waiterDevice("Tableta L1")
	if err := service.CheckLicense(ctx); err != nil {
		t.Fatal(err)
	}
	var cursor int64
	if err := pool.QueryRow(ctx, "SELECT coalesce(max(sequence),0) FROM app.realtime_events").Scan(&cursor); err != nil {
		t.Fatal(err)
	}

	// Hora incorrecta: la entrada con PIN, los avisos de platillos listos, la sesión y el canal de eventos siguen; la
	// administración no.
	last := manager.TrustedTime()
	clock.Set(last.Add(-2 * time.Hour))
	waiter := tablet
	waiter.session = fmt.Sprint(env.call(tablet, "POST", "auth/pin-login", map[string]string{"user_id": waiterID, "pin": "4829"},
		200)["session_token"])
	env.call(tablet, "GET", "ready-alerts", nil, 200)
	me := env.call(waiter, "GET", "auth/me", nil, 200)
	license, _ := me["license"].(map[string]any)
	clockState, _ := license["clock"].(map[string]any)
	if license["state"] != "active" || clockState["blocked"] != true || clockState["local_time_text"] == "" {
		t.Fatalf("auth/me del mesero con el bloqueo: %v", license)
	}
	state := env.call(waiter, "GET", "restaurant/state", nil, 200)
	if stateLicense, _ := state["license"].(map[string]any); stateLicense["state"] != "active" {
		t.Fatalf("restaurant/state con la licencia: %v", state["license"])
	}
	env.fails(waiter, "POST", "restaurant/accounts", map[string]any{"operation_id": pinTestID(), "name": "Mesa", "mode": "dine_in"},
		409, "SYSTEM_CLOCK_INVALID")
	env.fails(admin, "POST", "roles", map[string]any{"name": "Hora", "permissions": []string{"accounts.read"}}, 409, "SYSTEM_CLOCK_INVALID")
	// El vigilante publica license.changed y lo recibe también quien no tiene permisos de configuración.
	if err := service.CheckLicense(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot := env.call(waiter, "GET", fmt.Sprintf("events/snapshot?after=%d", cursor), nil, 200)
	events, _ := snapshot["events"].([]any)
	delivered := false
	for _, item := range events {
		if event, _ := item.(map[string]any); event["kind"] == "license.changed" {
			delivered = true
		}
	}
	if !delivered {
		t.Fatalf("el mesero no recibió license.changed: %v", snapshot)
	}

	// Las rutas de la hora solo en la computadora principal (escucha local).
	_, err = service.Execute(ctx, administration.Request{Method: "POST", Path: "/api/v1/license/support-request", Body: []byte(`{}`),
		DeviceID: admin.device, DeviceToken: admin.secret, SessionToken: admin.session, Local: false})
	var fault *administration.Fault
	if !errors.As(err, &fault) || fault.Status != 403 || fault.Code != "LICENSE_LOCAL_ONLY" {
		t.Fatalf("desde la red: %v", err)
	}
	reply := env.call(admin, "POST", "license/support-request", map[string]any{}, 200)
	if code := fmt.Sprint(reply["request_code"]); len(code) != 29 || reply["expires_at_text"] == "" {
		t.Fatalf("solicitud de soporte: %v", reply)
	}
	clock.Set(last)
	env.call(admin, "POST", "roles", map[string]any{"name": "Hora corregida", "permissions": []string{"accounts.read"}}, 200)
}
