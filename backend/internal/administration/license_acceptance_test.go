//go:build integration

package administration_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
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
	"local/ad-el-pos/server/internal/licenseseats/seatstest"
	"local/ad-el-pos/server/internal/licensetest"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
)

// Etapa L1, fase B: las rutas license/* contra el servidor de licencias simulado (httptest con TLS, en loopback, con
// las llaves de vectors.json): el estado con y sin sesión, el permiso license.manage, cada código de error, la
// solicitud sin internet, importar un .lic, liberar y la identidad nueva de una instalación copiada (solo local).

const licenseVectors = "../../../contracts/licencias/v1/fixtures/vectors.json"

type licenseEnvironment struct {
	t            *testing.T
	pool         *pgxpool.Pool
	service      *administration.Service
	handler      http.Handler
	provisioning string
}

type licenseActor struct {
	device  string
	secret  string
	session string
}

// send hace la petición; network la manda como si llegara por la escucha de red (con TLS y una IP privada).
func (env *licenseEnvironment) send(actor licenseActor, method, route string, body any, network bool) *httptest.ResponseRecorder {
	var reader io.Reader = bytes.NewReader(nil)
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, "/api/v1/"+route, reader)
	request.RemoteAddr = "127.0.0.1:50505"
	if network {
		request.RemoteAddr = "192.168.1.40:50505"
		request.TLS = &tls.ConnectionState{}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Device-ID", actor.device)
	request.Header.Set("X-Device-Token", actor.secret)
	if actor.session != "" {
		request.Header.Set("Authorization", "Bearer "+actor.session)
	}
	request.Header.Set("X-Provisioning-Token", env.provisioning)
	response := httptest.NewRecorder()
	env.handler.ServeHTTP(response, request)
	return response
}

// call hace la petición y comprueba el estado HTTP y, si se da, el código de error.
func (env *licenseEnvironment) call(actor licenseActor, method, route string, body any, status int, code string) map[string]any {
	env.t.Helper()
	response := env.send(actor, method, route, body, false)
	return env.check(response, method, route, status, code)
}

func (env *licenseEnvironment) check(response *httptest.ResponseRecorder, method, route string, status int, code string) map[string]any {
	env.t.Helper()
	var decoded map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &decoded)
	if response.Code != status {
		env.t.Fatalf("%s %s: esperado %d, recibido %d: %s", method, route, status, response.Code, response.Body.String())
	}
	if code != "" {
		failure, _ := decoded["error"].(map[string]any)
		if failure == nil || failure["code"] != code {
			env.t.Fatalf("%s %s: se esperaba %s: %s", method, route, code, response.Body.String())
		}
		return failure
	}
	return decoded
}

func (env *licenseEnvironment) scalar(query string, args ...any) int {
	env.t.Helper()
	var value int
	if err := env.pool.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		env.t.Fatal(err)
	}
	return value
}

func TestLicenseRoutes(t *testing.T) {
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

	// La computadora principal de prueba: la instalación A de los archivos de prueba, con su registro de la huella, y el
	// servidor de licencias simulado.
	fixtures, err := licensetest.LoadFixtures(licenseVectors)
	if err != nil {
		t.Fatal(err)
	}
	simulated := licensetest.NewServer(fixtures)
	listener, roots := simulated.StartTLS()
	defer listener.Close()
	installation := fixtures.Installations["installation-fixture-a"]
	identityDir := filepath.Join(t.TempDir(), localsetup.IdentityFolder)
	machine := "ffffffff-1111-2222-3333-444444444444"
	if err := os.MkdirAll(identityDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := licensing.WriteMachineRecord(identityDir, installation.PrivateKey, installation.ServerID, licensing.MachineFingerprint(machine), time.Now()); err != nil {
		t.Fatal(err)
	}
	trust := fixtures.Trust()
	manager := licensing.NewManager(licensing.Options{
		Directory:    filepath.Join(identityDir, localsetup.LicenseStateFolder),
		Installation: &licensing.Installation{ServerID: installation.ServerID, PublicKey: installation.PublicKey},
		PrivateKey:   installation.PrivateKey,
		Trust:        &trust,
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
	service := &administration.Service{Pool: pool, ProvisioningFile: tokenFile, Seats: seatstest.Roomy(), License: manager, LicenseServer: client}
	handler := httpapi.New(func(ctx context.Context) database.Health {
		return database.Probe(ctx, pool)
	}, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)), service)
	env := &licenseEnvironment{t: t, pool: pool, service: service, handler: handler, provisioning: provisioning}

	created := env.call(licenseActor{}, "POST", "setup", map[string]any{"name": "Licencias", "timezone": "America/Monterrey",
		"currency": "MXN", "theme": "light", "terminal_name": "Caja principal"}, 200, "")
	device := licenseActor{device: created["device_id"].(string), secret: created["device_token"].(string)}
	login := func(username, password string) licenseActor {
		actor := device
		actor.session = env.call(device, "POST", "auth/login", map[string]string{"username": username, "password": password}, 200, "")["session_token"].(string)
		return actor
	}
	admin := login("admin", "admin")

	// Estado sin licencia: con sesión, completo; con solo la credencial del dispositivo, el resumen.
	status := env.call(admin, "GET", "license/status", nil, 200, "")
	if status["state"] != "not_activated" || status["license"] != nil || status["can_manage"] != true || status["environment"] != "test" ||
		status["server_id"] != installation.ServerID || status["identity_regeneration_available"] != false {
		t.Fatalf("estado sin licencia: %v", status)
	}
	summary := env.call(device, "GET", "license/status", nil, 200, "")
	if len(summary) != 3 || summary["state"] != "not_activated" || summary["copied"] != false {
		t.Fatalf("resumen sin sesión: %v", summary)
	}

	// Cada código de error, con su texto propio.
	env.call(admin, "POST", "license/activate", map[string]string{"license_key": "no-es-un-codigo"}, 400, "LICENSE_KEY_INVALID_FORMAT")
	env.call(admin, "POST", "license/activate", map[string]string{"license_key": "ZZZ-2222-2222-2222"}, 404, "LICENSE_KEY_NOT_FOUND")
	env.call(admin, "POST", "license/activate", map[string]string{"license_key": "RVK-2222-2222-2222"}, 409, "LICENSE_SERVER_REVOKED")
	unavailable := env.call(admin, "POST", "license/activate", map[string]string{"license_key": "TMP-2222-2222-2222"}, 503, "LICENSE_SERVER_UNAVAILABLE")
	if unavailable["details"].(map[string]any)["reason"] != "temporary" {
		t.Fatalf("falla temporal: %v", unavailable)
	}
	busy := env.call(admin, "POST", "license/activate", map[string]string{"license_key": "RAT-2222-2222-2222"}, 429, "LICENSE_SERVER_BUSY")
	if busy["details"].(map[string]any)["retry_after_seconds"] != float64(30) {
		t.Fatalf("Retry-After: %v", busy)
	}
	env.call(admin, "POST", "license/activate", map[string]string{"license_key": "PRF-2222-2222-2222"}, 502, "LICENSE_SERVER_ERROR")
	pending := env.call(admin, "POST", "license/activate", map[string]string{"license_key": "ACN-2222-2222-2222"}, 409, "LICENSE_ACTIVATION_LIMIT")
	if pending["details"].(map[string]any)["transfer_available"] != false {
		t.Fatalf("con transferencia pendiente: %v", pending)
	}
	env.call(admin, "POST", "license/transfer", map[string]any{"license_key": "ACN-2222-2222-2222", "confirmed": true}, 409, "LICENSE_TRANSFER_LIMIT")
	if state := env.call(admin, "GET", "license/status", nil, 200, ""); state["state"] != "not_activated" {
		t.Fatalf("ningún error instala nada: %v", state)
	}

	// ACTIVATION_LIMIT con transferencia disponible, la confirmación y la transferencia.
	limit := env.call(admin, "POST", "license/activate", map[string]string{"license_key": "act-2222-2222-2222"}, 409, "LICENSE_ACTIVATION_LIMIT")
	if limit["details"].(map[string]any)["transfer_available"] != true {
		t.Fatalf("transferencia disponible: %v", limit)
	}
	env.call(admin, "POST", "license/transfer", map[string]any{"license_key": "ACT-2222-2222-2222"}, 400, "VALIDATION_ERROR")
	moved := env.call(admin, "POST", "license/transfer", map[string]any{"license_key": "ACT-2222-2222-2222", "confirmed": true}, 200, "")
	if moved["state"] != "active" || moved["license"] == nil {
		t.Fatalf("transferida: %v", moved)
	}

	// Sin license.manage: ver el estado sí, administrar no. Etapa L1, fase C: dar de alta a la cajera es administración,
	// que la puerta de la licencia no permite sin activar (tabla de la §6): va después de activar.
	var cashierRole string
	if err := pool.QueryRow(ctx, "SELECT id FROM app.roles WHERE code='cashier'").Scan(&cashierRole); err != nil {
		t.Fatal(err)
	}
	env.call(admin, "POST", "users", map[string]any{"username": "cajera", "display_name": "Cajera", "role_id": cashierRole,
		"active": true, "password": "clave-cajera"}, 200, "")
	cashier := login("cajera", "clave-cajera")
	if view := env.call(cashier, "GET", "license/status", nil, 200, ""); view["can_manage"] != false {
		t.Fatalf("sin permiso no administra: %v", view)
	}
	for _, route := range []string{"license/activate", "license/refresh", "license/deactivate", "license/offline-request", "license/import", "license/identity/regenerate"} {
		env.call(cashier, "POST", route, map[string]any{}, 403, "PERMISSION_DENIED")
	}
	if env.scalar("SELECT count(*)::int FROM app.role_permissions rp JOIN app.roles r ON r.id=rp.role_id WHERE rp.permission_key='license.manage' AND r.code<>'administrator'") != 0 {
		t.Fatal("license.manage solo es del administrador")
	}

	// Activar otra licencia, buscar actualización con una revisión alterada (se ignora) y sin cambios.
	activated := env.call(admin, "POST", "license/activate", map[string]string{"license_key": "MNS-2222-2222-2222"}, 200, "")
	license := activated["license"].(map[string]any)
	if activated["state"] != "active" || license["license_type"] != "subscription" || license["limits"].(map[string]any)["order_takers"] != float64(3) {
		t.Fatalf("activada: %v", activated)
	}
	simulated.AlterNext()
	rejected := env.call(admin, "POST", "license/refresh", nil, 502, "LICENSE_RESPONSE_INVALID")
	if rejected["details"].(map[string]any)["reason"] != licensing.ErrInvalidSignature {
		t.Fatalf("licencia alterada: %v", rejected)
	}
	if refreshed := env.call(admin, "POST", "license/refresh", nil, 200, ""); refreshed["state"] != "active" {
		t.Fatalf("buscar actualización: %v", refreshed)
	}

	// Sin internet: la solicitud .licreq y el archivo .lic.
	request := env.call(admin, "POST", "license/offline-request", map[string]string{"action": "renew"}, 200, "")
	payload, err := licensing.VerifyOfflineRequest([]byte(request["content"].(string)))
	if err != nil || payload["action"] != "renew" || payload["license_id"] != license["license_id"] || !strings.HasSuffix(request["filename"].(string), ".licreq") {
		t.Fatalf("solicitud de renovación: %v %v", payload, err)
	}
	withKey := env.call(admin, "POST", "license/offline-request", map[string]string{"action": "activate", "license_key": "k7m-9r3x-6tvp-8chd"}, 200, "")
	if payload, err := licensing.VerifyOfflineRequest([]byte(withKey["content"].(string))); err != nil || payload["license_key"] != "K7M-9R3X-6TVP-8CHD" {
		t.Fatalf("solicitud de activación con código: %v %v", payload, err)
	}
	env.call(admin, "POST", "license/offline-request", map[string]string{"action": "renew", "license_key": "K7M-9R3X-6TVP-8CHD"}, 400, "VALIDATION_ERROR")
	env.call(admin, "POST", "license/offline-request", map[string]string{"action": "transfer"}, 400, "VALIDATION_ERROR")
	// Liberar, con confirmación: queda «Revocada» por deactivated.
	env.call(admin, "POST", "license/deactivate", map[string]any{}, 400, "VALIDATION_ERROR")
	released := env.call(admin, "POST", "license/deactivate", map[string]any{"confirmed": true}, 200, "")
	if released["state"] != "revoked" || released["revocation_reason"] != "deactivated" {
		t.Fatalf("liberada: %v", released)
	}
	// Importar un .lic: con el salto de línea de un editor; uno alterado se rechaza y sigue la última válida.
	issued, err := fixtures.Issue(licensetest.IssueOptions{
		ServerID: installation.ServerID, PublicKey: installation.PublicKey, Type: licensetest.TypeSubscription,
		Revision: 50, ExpiresAt: time.Now().AddDate(0, 0, -2),
	})
	if err != nil {
		t.Fatal(err)
	}
	imported := env.call(admin, "POST", "license/import", map[string]string{"license": issued + "\n"}, 200, "")
	if imported["state"] != "grace" {
		t.Fatalf("el .lic importado deja la licencia en tolerancia: %v", imported)
	}
	importedID := imported["license"].(map[string]any)["license_id"]
	invalid := env.call(admin, "POST", "license/import", map[string]string{"license": issued[:len(issued)-4] + "AAAA"}, 400, "LICENSE_FILE_INVALID")
	if invalid["details"].(map[string]any)["reason"] != licensing.ErrInvalidSignature {
		t.Fatalf(".lic alterado: %v", invalid)
	}
	if state := env.call(admin, "GET", "license/status", nil, 200, ""); state["state"] != "grace" {
		t.Fatalf("sigue la última válida: %v", state)
	}
	if env.scalar("SELECT count(*)::int FROM app.audit_events WHERE action='license.import_rejected' AND NOT success") != 1 {
		t.Fatal("el .lic rechazado queda en la auditoría")
	}
	if env.scalar("SELECT count(*)::int FROM app.license_revisions WHERE license_id=$1 AND license_revision=50", importedID) != 1 {
		t.Fatal("la copia del historial de la base sube con el .lic importado")
	}

	// La auditoría nunca lleva el código comercial.
	if env.scalar(`SELECT count(*)::int FROM app.audit_events
        WHERE coalesce(after_state::text,'') || coalesce(before_state::text,'') ~* '(2222-2222-2222|9r3x-6tvp)'`) != 0 {
		t.Fatal("la auditoría de la licencia no lleva el código comercial")
	}
	if env.scalar("SELECT count(*)::int FROM app.audit_events WHERE action='license.activated'") != 1 ||
		env.scalar("SELECT count(*)::int FROM app.audit_events WHERE action='license.activation_failed' AND NOT success") < 6 ||
		env.scalar("SELECT count(*)::int FROM app.audit_events WHERE action='license.transferred'") != 1 ||
		env.scalar("SELECT count(*)::int FROM app.audit_events WHERE action='license.offline_request_created'") != 2 {
		t.Fatal("cada operación de la licencia queda en la auditoría")
	}

	// La identidad nueva: solo en una instalación copiada y solo por la escucha local.
	env.call(admin, "POST", "license/identity/regenerate", map[string]any{"confirmed": true}, 409, "IDENTITY_REGENERATION_NOT_NEEDED")
	// Desde la red es un 409 que dice dónde se hace, no un problema de permisos: no queda como acceso denegado en la
	// auditoría (L1-D56, como R2-6).
	deniedBefore := env.scalar("SELECT count(*)::int FROM app.audit_events WHERE action='access.denied'")
	fromNetwork := env.check(env.send(admin, "POST", "license/identity/regenerate", map[string]any{"confirmed": true}, true), "POST", "license/identity/regenerate", 409, "LICENSE_IDENTITY_LOCAL_ONLY")
	if message, _ := fromNetwork["message"].(string); !strings.Contains(message, "computadora principal") {
		t.Fatalf("la identidad nueva desde la red dice dónde se hace: %v", fromNetwork)
	}
	if denied := env.scalar("SELECT count(*)::int FROM app.audit_events WHERE action='access.denied'"); denied != deniedBefore {
		t.Fatalf("la identidad nueva desde la red quedó como acceso denegado: %d antes, %d después", deniedBefore, denied)
	}
	testRegenerateCopiedIdentity(t, env, admin)
}

// testRegenerateCopiedIdentity: una carpeta de datos copiada a otra computadora (otra huella) crea su identidad nueva;
// se revocan las terminales emparejadas por red y se conserva la última hora confiable.
func testRegenerateCopiedIdentity(t *testing.T, env *licenseEnvironment, admin licenseActor) {
	ctx := context.Background()
	identityDir := filepath.Join(t.TempDir(), localsetup.IdentityFolder)
	original, err := localsetup.EnsureIdentity(identityDir)
	if err != nil {
		t.Fatal(err)
	}
	created := "aaaaaaaa-1111-2222-3333-444444444444"
	if _, err := localsetup.EnsureLicenseKey(identityDir, func() (string, error) {
		return licensing.MachineFingerprint(created), nil
	}); err != nil {
		t.Fatal(err)
	}
	key, err := localsetup.LoadLicenseKey(identityDir)
	if err != nil {
		t.Fatal(err)
	}
	copied := licensing.NewManager(licensing.Options{
		Directory:    filepath.Join(identityDir, localsetup.LicenseStateFolder),
		Installation: &licensing.Installation{ServerID: original.ServerID, PublicKey: key.PublicKey},
		PrivateKey:   key.PrivateKey,
		MachineID: func() (string, error) {
			return "bbbbbbbb-1111-2222-3333-444444444444", nil
		},
		Database: licensing.PoolProbe{Pool: env.pool},
	})
	if err := copied.Load(ctx); err != nil {
		t.Fatal(err)
	}
	trusted := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Microsecond)
	copied.ObserveTime(trusted, licensing.TimeFromServer)
	previousLicense, previousServer := env.service.License, env.service.LicenseServer
	env.service.License, env.service.LicenseServer = copied, nil
	defer func() {
		env.service.License, env.service.LicenseServer = previousLicense, previousServer
	}()
	status := env.call(admin, "GET", "license/status", nil, 200, "")
	if status["copied"] != true || status["identity_regeneration_available"] != true {
		t.Fatalf("instalación copiada: %v", status)
	}
	env.call(admin, "POST", "license/activate", map[string]string{"license_key": "PRP-2222-2222-2222"}, 409, "LICENSE_INSTALLATION_COPIED")
	// Una terminal emparejada por red, con su sesión.
	if _, err := env.pool.Exec(ctx, `INSERT INTO app.devices(id,name,secret_hash,kind,server_fingerprint)
        VALUES ('00000000-0000-4000-8000-00000000c001','Terminal copiada','hash-copia','desktop',$1)`, strings.Repeat("ab", 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO app.sessions(user_id,device_id,token_hash,expires_at)
        SELECT id,'00000000-0000-4000-8000-00000000c001','hash-sesion-copia',now()+interval '1 hour' FROM app.users WHERE username='admin'`); err != nil {
		t.Fatal(err)
	}
	env.call(admin, "POST", "license/identity/regenerate", map[string]any{}, 400, "VALIDATION_ERROR")
	result := env.call(admin, "POST", "license/identity/regenerate", map[string]any{"confirmed": true}, 200, "")
	if result["restart_required"] != true || result["revoked_terminals"] != float64(1) || result["server_id"] == original.ServerID {
		t.Fatalf("identidad nueva: %v", result)
	}
	after := result["status"].(map[string]any)
	if after["copied"] != false || after["state"] != "not_activated" || after["server_id"] != result["server_id"] {
		t.Fatalf("después de la identidad nueva: %v", after)
	}
	if !copied.TrustedTime().Equal(trusted) {
		t.Fatalf("la última hora confiable se conserva: %v", copied.TrustedTime())
	}
	if env.scalar("SELECT count(*)::int FROM app.devices WHERE id='00000000-0000-4000-8000-00000000c001' AND revoked_at IS NOT NULL") != 1 ||
		env.scalar("SELECT count(*)::int FROM app.sessions WHERE device_id='00000000-0000-4000-8000-00000000c001' AND revoke_reason='identity_regenerated'") != 1 {
		t.Fatal("las terminales emparejadas por red se revocan, con sus sesiones")
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(identityDir), "identity-reemplazada-*"))
	if len(matches) != 1 {
		t.Fatalf("la identidad anterior se aparta: %v", matches)
	}
	if env.scalar("SELECT count(*)::int FROM app.audit_events WHERE action='license.identity_regenerated' AND success") != 1 {
		t.Fatal("la identidad nueva queda en la auditoría")
	}
}
