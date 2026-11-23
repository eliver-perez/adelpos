//go:build integration

package administration_test

import (
	"context"
	"io"
	"log/slog"
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
	"local/ad-el-pos/server/internal/licensetest"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/licensing/licensingtest"
	"local/ad-el-pos/server/internal/localsetup"
)

// Etapa L1, Fase C con la Fase B unida: las rutas license/* avisan a las terminales en el momento (license.changed
// después de activar, transferir, buscar actualización, liberar e importar) y «Comprobar la hora con internet» (§6.3)
// pasa por el cliente real del servidor de licencias contra el servidor simulado de la Fase B (httptest con TLS, en
// loopback, con las llaves de vectors.json), con su refresh y su server_time: coincide (la hora confiable baja y el
// bloqueo se quita), no coincide (LICENSE_TIME_MISMATCH con la hora del servidor) y con un error del servidor de
// licencias (los códigos de la Fase B).
// El reloj del gestor es simulado; el del servidor simulado es el real con un desfase.
func TestStageL1LicenseNoticesAndTimeCheckWithLicenseServer(t *testing.T) {
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

	fixtures, err := licensetest.LoadFixtures(licenseVectors)
	if err != nil {
		t.Fatal(err)
	}
	simulated := licensetest.NewServer(fixtures)
	listener, roots := simulated.StartTLS()
	defer listener.Close()
	installation := fixtures.Installations["installation-fixture-a"]
	identityDir := filepath.Join(t.TempDir(), localsetup.IdentityFolder)
	machine := "eeeeeeee-1111-2222-3333-444444444444"
	if err := os.MkdirAll(identityDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := licensing.WriteMachineRecord(identityDir, installation.PrivateKey, installation.ServerID, licensing.MachineFingerprint(machine), time.Now()); err != nil {
		t.Fatal(err)
	}
	clock := licensingtest.NewClock(time.Now().UTC().Truncate(time.Second))
	trust := fixtures.Trust()
	manager := licensing.NewManager(licensing.Options{
		Directory:    filepath.Join(identityDir, localsetup.LicenseStateFolder),
		Installation: &licensing.Installation{ServerID: installation.ServerID, PublicKey: installation.PublicKey},
		PrivateKey:   installation.PrivateKey,
		Trust:        &trust,
		Now:          clock.Now,
		MachineID: func() (string, error) {
			return machine, nil
		},
		Database: licensing.PoolProbe{Pool: pool},
	})
	if err := manager.Load(ctx); err != nil {
		t.Fatal(err)
	}
	client := licensing.NewClient(manager, licensing.ClientOptions{
		BaseURL: listener.URL, AppVersion: "0.15.0", TestMode: true, RootCAs: roots,
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	provisioning := administration.NewToken()
	tokenFile := filepath.Join(t.TempDir(), "provisioning-token")
	if err := os.WriteFile(tokenFile, []byte(provisioning), 0600); err != nil {
		t.Fatal(err)
	}
	// Como serveHTTP: los cupos de la licencia real, el cliente y la hora de ese mismo cliente.
	service := &administration.Service{Pool: pool, ProvisioningFile: tokenFile, License: manager,
		Seats: administration.LicenseSeats(manager), LicenseServer: client,
		LicenseServerClock: administration.LicenseClientClock(client)}
	handler := httpapi.New(func(ctx context.Context) database.Health {
		return database.Probe(ctx, pool)
	}, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)), service)
	env := &licenseEnvironment{t: t, pool: pool, service: service, handler: handler, provisioning: provisioning}
	created := env.call(licenseActor{}, "POST", "setup", map[string]any{"name": "Licencias C", "timezone": "America/Monterrey",
		"currency": "MXN", "theme": "light", "terminal_name": "Caja principal"}, 200, "")
	device := licenseActor{device: created["device_id"].(string), secret: created["device_token"].(string)}
	admin := device
	admin.session = env.call(device, "POST", "auth/login", map[string]string{"username": "admin", "password": "admin"}, 200, "")["session_token"].(string)

	// El vigilante toma la referencia al arrancar (aquí, a mano); desde ahí cada ruta avisa lo que cambie.
	if err := service.CheckLicense(ctx); err != nil {
		t.Fatal(err)
	}
	notices := func() int {
		return env.scalar("SELECT count(*)::int FROM app.realtime_events WHERE kind='license.changed'")
	}
	expectNotice := func(what string, before int) int {
		t.Helper()
		after := notices()
		if after != before+1 {
			t.Fatalf("%s: se esperaba un license.changed y hubo %d", what, after-before)
		}
		return after
	}
	count := notices()

	// Activar.
	env.call(admin, "POST", "license/activate", map[string]string{"license_key": "MNS-2222-2222-2222"}, 200, "")
	count = expectNotice("activar", count)
	// Buscar actualización sin cambios: no hay nada que avisar; con una renovación, sí.
	env.call(admin, "POST", "license/refresh", nil, 200, "")
	if notices() != count {
		t.Fatal("una consulta sin cambios no avisa")
	}
	if err := simulated.Renew("MNS-2222-2222-2222", 1); err != nil {
		t.Fatal(err)
	}
	env.call(admin, "POST", "license/refresh", nil, 200, "")
	count = expectNotice("buscar actualización con renovación", count)
	// Liberar.
	env.call(admin, "POST", "license/deactivate", map[string]any{"confirmed": true}, 200, "")
	count = expectNotice("liberar", count)
	// Importar un .lic (después de confirmar la transacción).
	issued, err := fixtures.Issue(licensetest.IssueOptions{ServerID: installation.ServerID, PublicKey: installation.PublicKey,
		Revision: 40})
	if err != nil {
		t.Fatal(err)
	}
	env.call(admin, "POST", "license/import", map[string]string{"license": issued}, 200, "")
	count = expectNotice("importar", count)
	// Transferir a esta computadora.
	env.call(admin, "POST", "license/activate", map[string]string{"license_key": "ACT-2222-2222-2222"}, 409, "LICENSE_ACTIVATION_LIMIT")
	env.call(admin, "POST", "license/transfer", map[string]any{"license_key": "ACT-2222-2222-2222", "confirmed": true}, 200, "")
	count = expectNotice("transferir", count)

	// §6.3 con el cliente real. La fecha saltó dos días al futuro, se registró actividad y se corrigió el reloj.
	real := clock.Now()
	clock.Set(real.Add(48 * time.Hour))
	env.call(admin, "POST", "roles", map[string]any{"name": "En el futuro", "permissions": []string{"accounts.read"}}, 200, "")
	clock.Set(real)
	env.call(admin, "POST", "roles", map[string]any{"name": "Bloqueada", "permissions": []string{"accounts.read"}}, 409, "SYSTEM_CLOCK_INVALID")
	// La puerta despierta al vigilante, que registra el bloqueo y lo avisa (aquí, a mano: el vigilante no corre).
	if err := service.CheckLicense(ctx); err != nil {
		t.Fatal(err)
	}
	count = expectNotice("el bloqueo", count)
	refreshesBefore := 0
	for _, call := range simulated.Calls() {
		if call.Action == licensing.ActionRefresh {
			refreshesBefore++
		}
	}
	corrected := env.call(admin, "POST", "license/time-check", map[string]any{}, 200, "")
	if corrected["matches"] != true || corrected["replaced"] != true || manager.Clock().Blocked {
		t.Fatalf("coincide con el servidor de licencias: %v", corrected)
	}
	refreshes := 0
	for _, call := range simulated.Calls() {
		if call.Action == licensing.ActionRefresh {
			refreshes++
		}
	}
	if refreshes != refreshesBefore+1 {
		t.Fatal("con activación, la hora sale de la consulta de actualización (refresh)")
	}
	if env.scalar("SELECT count(*)::int FROM app.audit_events WHERE action='license.clock_corrected' AND success") != 1 {
		t.Fatal("la corrección queda en la auditoría")
	}
	count = expectNotice("la hora corregida", count)
	env.call(admin, "POST", "roles", map[string]any{"name": "Corregida", "permissions": []string{"accounts.read"}}, 200, "")

	// Un error del servidor de licencias llega con los códigos de la Fase B (licenseFault), sin cambiar nada.
	simulated.FailNext(licensetest.Scripted{Action: "challenge", Code: "INVALID_REQUEST"})
	env.call(admin, "POST", "license/time-check", map[string]any{}, 502, "LICENSE_SERVER_ERROR")

	// No coincide: el servidor de licencias va 3 horas adelante; responde su hora y la última hora confiable sube con su
	// server_time (como toda respuesta correcta), así que el reloj queda bloqueado hasta corregirlo.
	simulated.SetOffset(3 * time.Hour)
	mismatch := env.call(admin, "POST", "license/time-check", map[string]any{}, 409, "LICENSE_TIME_MISMATCH")
	details := mismatch["details"].(map[string]any)
	if details["server_time"] == nil || details["server_time_text"] == "" {
		t.Fatalf("no coincide: %v", mismatch)
	}
	if env.scalar("SELECT count(*)::int FROM app.audit_events WHERE action='license.clock_internet_check' AND NOT success") != 1 {
		t.Fatal("la comprobación que no coincide queda en la auditoría")
	}
}
