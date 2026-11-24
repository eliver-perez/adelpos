//go:build integration

package apicontract_test

import (
	"context"
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
	"local/ad-el-pos/server/internal/apicontract"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/httpapi"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/licensing/licensingtest"
	"local/ad-el-pos/server/internal/product"
)

// Etapa L1, Fase C: el estado de la licencia y los rechazos de la puerta en el contrato v1, contra el servidor real con
// un gestor de licencias de prueba (licencias firmadas con las llaves de vectors.json y reloj simulado): `license` de
// getIdentity y getRecoveryState en cada estado y con la hora incorrecta, LicenseRejection y SystemClockInvalid. Cada
// respuesta se valida contra su operación y, además, contra el esquema exacto del rechazo.
func TestContractV1License(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("No se pudo abrir el pool temporal")
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	provisioning := administration.NewToken()
	filename := filepath.Join(t.TempDir(), "provisioning-token")
	if err := os.WriteFile(filename, []byte(provisioning), 0600); err != nil {
		t.Fatal(err)
	}
	clock := licensingtest.NewClock(time.Now().UTC().Truncate(time.Second))
	licenses := licensingtest.New(t, clock)
	licenses.Database = licensing.PoolProbe{Pool: pool}
	manager := licenses.Start()
	service := &administration.Service{Pool: pool, ProvisioningFile: filename, License: manager,
		Seats: administration.LicenseSeats(manager)}
	handler := httpapi.New(func(ctx context.Context) database.Health {
		return database.Probe(ctx, pool)
	}, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)), service)
	contract, err := apicontract.Load()
	if err != nil {
		t.Fatal(err)
	}
	client := &contractClient{t: t, contract: contract, handler: handler, version: product.Read().Version, kind: "desktop",
		covered: map[string][]int{}, pool: pool}
	setup := httptest.NewRequest("POST", "/api/v1/setup", strings.NewReader(`{"name":"Contrato licencia","timezone":"America/Monterrey",
        "currency":"MXN","theme":"light","terminal_name":"Terminal de contrato"}`))
	setup.RemoteAddr = "127.0.0.1:52900"
	setup.Header.Set("X-Provisioning-Token", provisioning)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, setup)
	credentials := decodeContract[map[string]any](t, recorder.Body.Bytes())
	client.device, client.token = fmt.Sprint(credentials["device_id"]), fmt.Sprint(credentials["device_token"])
	login := decodeContract[map[string]string](t, client.call("POST", "/auth/login",
		map[string]string{"username": "admin", "password": "admin"}, nil, 200).Body.Bytes())
	client.session = login["session_token"]

	exact := func(name string, body []byte) {
		t.Helper()
		if err := contract.ValidateJSON(map[string]any{"$ref": "#/components/schemas/" + name}, body); err != nil {
			t.Fatalf("la respuesta no cumple %s: %v\n%s", name, err, body)
		}
	}
	licenseOf := func(path string) map[string]any {
		t.Helper()
		body := client.call("GET", path, nil, nil, 200).Body.Bytes()
		response := decodeContract[map[string]any](t, body)
		license, _ := response["license"].(map[string]any)
		if license == nil {
			t.Fatalf("%s sin license: %s", path, body)
		}
		return license
	}
	openAccount := map[string]any{"operation_id": contractID(), "mode": "dine_in", "name": "Contrato licencia"}

	// Sin activar.
	if state := licenseOf("/auth/me")["state"]; state != "not_activated" {
		t.Fatalf("estado: %v", state)
	}
	rejected := client.call("POST", "/restaurant/accounts", openAccount, nil, 409).Body.Bytes()
	exact("LicenseRejection", rejected)
	if !strings.Contains(string(rejected), `"LICENSE_NOT_ACTIVATED"`) {
		t.Fatalf("sin activar: %s", rejected)
	}

	// Vigente, por vencer: los días que faltan.
	licenses.Accept(manager, licensingtest.Subscription(clock.Now().Add(5*24*time.Hour), 2, 2, 1))
	if license := licenseOf("/restaurant/state"); license["state"] != "active" || license["days_until_expiry"] != float64(5) {
		t.Fatalf("vigente: %v", license)
	}
	// Tolerancia.
	clock.Advance(7 * 24 * time.Hour)
	if license := licenseOf("/auth/me"); license["state"] != "grace" || license["grace_days_remaining"] != float64(3) {
		t.Fatalf("tolerancia: %v", license)
	}
	// Vencida.
	clock.Advance(4 * 24 * time.Hour)
	openAccount["operation_id"] = contractID()
	expired := client.call("POST", "/restaurant/accounts", openAccount, nil, 409).Body.Bytes()
	exact("LicenseRejection", expired)
	if !strings.Contains(string(expired), `"LICENSE_EXPIRED"`) {
		t.Fatalf("vencida: %s", expired)
	}

	// Hora incorrecta con una licencia vigente.
	licenses.Accept(manager, licensingtest.Perpetual(2, 2, 1))
	client.call("POST", "/restaurant/accounts", map[string]any{"operation_id": contractID(), "mode": "dine_in", "name": "Antes"}, nil, 200)
	clock.Set(manager.TrustedTime().Add(-2 * time.Hour))
	openAccount["operation_id"] = contractID()
	invalid := client.call("POST", "/restaurant/accounts", openAccount, nil, 409).Body.Bytes()
	exact("SystemClockInvalid", invalid)
	license := licenseOf("/auth/me")
	clockState, _ := license["clock"].(map[string]any)
	if clockState["blocked"] != true || clockState["license_ok"] != true || clockState["timezone"] != "America/Monterrey" {
		t.Fatalf("hora incorrecta: %v", license)
	}
	licenseOf("/restaurant/state")
}
