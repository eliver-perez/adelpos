//go:build integration

package operations_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/httpapi"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/lanlistener"
	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/licenseseats/seatstest"
	"local/ad-el-pos/server/internal/localsetup"
	"local/ad-el-pos/server/internal/operations"
)

// Revisión independiente R1 de la etapa K1 (seguridad de la pantalla de cocina Android), contra PostgreSQL y con el
// enrutador completo de la API (httpapi, con la versión mínima por tipo de cliente) delante del servicio. No corrige
// nada del producto: cada subprueba afirma el comportamiento seguro que pide la especificación (§2 y §3) y las
// decisiones K1-D3, K1-D4, K1-D6, K1-D7, K1-D8, K1-D12, K1-D13 y K1-D16; si una falla, es un hallazgo de la revisión.
//
//   - el emparejamiento como kitchen: el tipo queda fijo, no se reclama dos veces y el X-Client-Kind debe coincidir;
//   - el punto único de dispatch: un kitchen no llega a ninguna ruta fuera de su lista cerrada (también con el
//     Authorization de otra sesión, mayúsculas, %2F, barras dobles, otros métodos y por la escucha de red), y
//     kitchen-display/* no responde a otro tipo de dispositivo;
//   - el alcance por estación al marcar, recuperar, confirmar y en los eventos;
//   - la contraseña del administrador (settings-access);
//   - el cupo kitchen_displays: asignación atómica con varias pantallas a la vez, sin liberarse por desconexión,
//     «Liberar cupo» frente a connect a la vez y revocar.
//
// Usa su propia base desde cero.

// r1Fixture es el andamiaje de la Fase A con el enrutador completo delante del servicio.
type r1Fixture struct {
	*kitchenDisplayFixture
	router http.Handler
}

// r1Response es una respuesta del enrutador completo.
type r1Response struct {
	status int
	body   []byte
}

// code es el código de error de la respuesta (vacío si no es un error JSON).
func (response r1Response) code() string {
	var problem struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(response.body, &problem) != nil {
		return ""
	}
	return problem.Error.Code
}

// routed manda una petición por el enrutador completo con el destino tal cual (para probar %2F, mayúsculas y barras
// dobles). network la manda como si llegara por la escucha HTTPS de la red local.
func (fixture *r1Fixture) routed(terminal testTerminal, method, target string, body any, headers map[string]string, network bool) r1Response {
	var encoded []byte
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	request := httptest.NewRequest(method, target, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:54300"
	if network {
		request.RemoteAddr = "192.168.77.20:54300"
		request.TLS = &tls.ConnectionState{}
		request = request.WithContext(lanlistener.WithNetwork(request.Context()))
	}
	if terminal.deviceID != "" {
		request.Header.Set("X-Device-ID", terminal.deviceID)
		request.Header.Set("X-Device-Token", terminal.deviceToken)
	}
	if terminal.sessionToken != "" {
		request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	fixture.router.ServeHTTP(recorder, request)
	return r1Response{status: recorder.Code, body: append([]byte{}, recorder.Body.Bytes()...)}
}

// sessionSeen es la última vez que authenticateSession validó la sesión del administrador.
func (fixture *r1Fixture) sessionSeen(t *testing.T) time.Time {
	t.Helper()
	var seen time.Time
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT last_seen_at FROM app.sessions WHERE token_hash=$1",
		administration.TokenHash(fixture.admin.sessionToken)).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	return seen
}

// tasksAt son las tareas (id y versión) de la cuenta en la estación, en orden estable.
func (fixture *r1Fixture) tasksAt(t *testing.T, accountID, stationID string) []kitchen.DisplayTaskRef {
	t.Helper()
	rows, err := fixture.pool.Query(fixture.ctx, `SELECT t.id::text,t.version_id::text FROM app.kitchen_tasks t
        JOIN app.restaurant_item_units u ON u.id=t.unit_id WHERE u.account_id=$1 AND t.station_id=$2 ORDER BY t.id`,
		accountID, stationID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := []kitchen.DisplayTaskRef{}
	for rows.Next() {
		var task kitchen.DisplayTaskRef
		if err := rows.Scan(&task.ID, &task.VersionID); err != nil {
			t.Fatal(err)
		}
		result = append(result, task)
	}
	if len(result) == 0 {
		t.Fatalf("la cuenta %s no tiene tareas en la estación %s", accountID, stationID)
	}
	return result
}

// taskState es el estado de una tarea.
func (fixture *r1Fixture) taskState(t *testing.T, id string) string {
	t.Helper()
	var state string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT state FROM app.kitchen_tasks WHERE id=$1", id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

// ticketOf es una comanda de la cuenta que incluye la estación.
func (fixture *r1Fixture) ticketOf(t *testing.T, accountID, stationID string) string {
	t.Helper()
	var ticket string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT t.id::text FROM app.kitchen_tickets t JOIN app.kitchen_releases r
        ON r.id=t.release_id WHERE r.account_id=$1 AND $2::uuid=ANY(t.station_ids) ORDER BY t.created_at LIMIT 1`,
		accountID, stationID).Scan(&ticket); err != nil {
		t.Fatal(err)
	}
	return ticket
}

// restrict deja la pantalla con solo esas estaciones (PUT kitchen/access, como Terminales).
func (fixture *r1Fixture) restrict(t *testing.T, display testTerminal, stations ...string) {
	t.Helper()
	var all bool
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT kitchen_all_stations FROM app.devices WHERE id=$1", display.deviceID).Scan(&all); err != nil {
		t.Fatal(err)
	}
	current := []string{}
	rows, err := fixture.pool.Query(fixture.ctx, "SELECT station_id::text FROM app.kitchen_device_stations WHERE device_id=$1 ORDER BY station_id", display.deviceID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		current = append(current, id)
	}
	rows.Close()
	fixture.mutate(t, "PUT", "kitchen/access", map[string]any{"device_id": display.deviceID, "all_stations": false,
		"station_ids": stations, "expected_station_ids": current, "expected_all_stations": all}, 200)
}

func TestStageK1R1KitchenDisplaySecurity(t *testing.T) {
	resetStage9GDatabase(t)
	base := &kitchenDisplayFixture{waiterModeFixture: &waiterModeFixture{restaurantFixture: newFixture(t)}}
	fixture := &r1Fixture{kitchenDisplayFixture: base}
	fixture.router = httpapi.NewWithOptions(httpapi.Options{TestMode: true}, func(context.Context) database.Health {
		return database.Health{Postgres: "available", Schema: "compatible"}
	}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), fixture.service)
	fixture.seedCatalog(t)
	fixture.printer = decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name: "Comandas R1", Transport: "escpos_tcp", Destination: "127.0.0.1", Port: 9109, Active: true, PaperWidth: 80, Copies: 1,
	}, 200))
	// Tres estaciones, una por recepción: Cocina (solo pantalla), Barra (impresora y pantalla) y Postres (solo impresora).
	fixture.cocina = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina R1", Active: true, Reception: "kds", Tracking: "item",
	}, 200))
	fixture.barra = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Barra R1", Active: true, Reception: "both", PrinterID: &fixture.printer.ID, Tracking: "item",
	}, 200))
	fixture.papel = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Postres R1", Active: true, Reception: "printer", PrinterID: &fixture.printer.ID,
	}, 200))
	fixture.pastel = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Pastel R1", Active: true}, CategoryID: fixture.category.ID, Code: "PASTEL-R1",
		Kind: "product", BasePrice: "60",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare", StationIDs: []string{fixture.cocina.ID}}, 200)
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &fixture.fries.ID, Mode: "prepare", StationIDs: []string{fixture.barra.ID}}, 200)
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &fixture.pastel.ID, Mode: "prepare", StationIDs: []string{fixture.papel.ID}}, 200)
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón R1", Active: true}, 200))
	for index := range 6 {
		fixture.tables = append(fixture.tables, decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: fmt.Sprintf("Mesa R%d", index+1), Shape: "round", X: 20 + index*110, Y: 30,
			Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200)))
	}
	inventoryUser(t, fixture.restaurantFixture, "mesero_r1", waiterPermissions)
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username='mesero_r1'").Scan(&fixture.waiterID); err != nil {
		t.Fatal(err)
	}
	fixture.display = fixture.kitchenDevice(t, "Pantalla cocina R1")
	fixture.second = fixture.kitchenDevice(t, "Pantalla barra R1")
	fixture.displayCall(t, fixture.display, "POST", "kitchen-display/connect", map[string]any{}, 200)
	fixture.displayCall(t, fixture.second, "POST", "kitchen-display/connect", map[string]any{}, 200)

	t.Run("emparejar como kitchen: tipo fijo, un solo reclamo y X-Client-Kind", func(t *testing.T) {
		testR1Pairing(t, fixture)
	})
	t.Run("lista cerrada en dispatch con el enrutador completo, local y de red", func(t *testing.T) {
		testR1ClosedList(t, fixture)
	})
	t.Run("kitchen-display/* con otro tipo de dispositivo", func(t *testing.T) {
		testR1OtherKinds(t, fixture)
	})
	t.Run("versión mínima por tipo de cliente por la escucha de red", func(t *testing.T) {
		testR1ClientVersion(t, fixture)
	})
	t.Run("alcance por estación al marcar, recuperar, confirmar y en los eventos", func(t *testing.T) {
		testR1StationScope(t, fixture)
	})
	t.Run("contraseña del administrador sin sesión, con el límite de intentos y sin registrarla", func(t *testing.T) {
		testR1SettingsAccess(t, fixture)
	})
	t.Run("cupo de cocina: varias pantallas a la vez, desconexión, Liberar cupo y revocar", func(t *testing.T) {
		testR1Seats(t, fixture)
	})
	t.Run("una marca que esperaba el lock de cocina no se aplica si la pantalla se revoca mientras", func(t *testing.T) {
		testR1RevokedInFlight(t, fixture)
	})
}

func testR1Pairing(t *testing.T, fixture *r1Fixture) {
	identity, err := localsetup.EnsureIdentity(filepath.Join(t.TempDir(), localsetup.IdentityFolder))
	if err != nil {
		t.Fatal(err)
	}
	fixture.service.Identity = &administration.ServerIdentity{ServerID: identity.ServerID, Fingerprint: identity.Fingerprint}
	defer func() { fixture.service.Identity = nil }()
	// Pide, revela, aprueba en la computadora principal y devuelve la solicitud y sus cabeceras.
	pair := func(kind string, headers map[string]string) (string, map[string]string) {
		nonce := make([]byte, 32)
		if _, err := rand.Read(nonce); err != nil {
			t.Fatal(err)
		}
		code, response := fixture.send(testTerminal{}, "POST", "pairing/requests", map[string]any{"name": "Tableta R1 " + kind,
			"device_kind": kind, "os": "Android 14", "app_version": "0.1.0", "commitment": administration.PairingCommitment(nonce)}, headers)
		if code != 200 {
			t.Fatalf("pedir el código como %s: %d %s", kind, code, response)
		}
		created := decodeResponse[map[string]string](t, response)
		secret := map[string]string{"X-Pairing-Secret": created["pairing_secret"]}
		for key, value := range headers {
			secret[key] = value
		}
		if code, response := fixture.send(testTerminal{}, "POST", "pairing/requests/"+created["request_id"]+"/reveal",
			map[string]string{"terminal_nonce": hex.EncodeToString(nonce)}, secret); code != 200 {
			t.Fatalf("revelar: %d %s", code, response)
		}
		serverNonce, err := hex.DecodeString(created["server_nonce"])
		if err != nil {
			t.Fatal(err)
		}
		pairingCode, err := administration.PairingCode(identity.Fingerprint, nonce, serverNonce, created["request_id"])
		if err != nil {
			t.Fatal(err)
		}
		// Antes de aprobar no hay credencial.
		if code, response := fixture.send(testTerminal{}, "POST", "pairing/requests/"+created["request_id"]+"/claim", map[string]any{}, secret); code != 409 ||
			errorCode(t, response) != "PAIRING_NOT_APPROVED" {
			t.Fatalf("reclamar sin aprobar: %d %s", code, response)
		}
		fixture.adminCall(t, "POST", "devices/pairing-requests/"+created["request_id"]+"/approve", map[string]string{"code": pairingCode}, 200)
		return created["request_id"], secret
	}
	devicesBefore := fixture.count(t, "SELECT count(*)::int FROM app.devices")
	requestID, secret := pair("kitchen", kitchenHeaders)
	// El reclamo entrega la credencial una sola vez: el segundo, también con otro X-Client-Kind, no crea otra terminal.
	code, response := fixture.send(testTerminal{}, "POST", "pairing/requests/"+requestID+"/claim", map[string]any{}, secret)
	if code != 200 {
		t.Fatalf("reclamar: %d %s", code, response)
	}
	claimed := decodeResponse[map[string]string](t, response)
	paired := testTerminal{deviceID: claimed["device_id"], deviceToken: claimed["device_token"]}
	for _, kind := range []string{"kitchen", "desktop", "android", ""} {
		headers := map[string]string{"X-Pairing-Secret": secret["X-Pairing-Secret"], "X-Client-Kind": kind, "X-Client-Version": "0.1.0"}
		if code, response := fixture.send(testTerminal{}, "POST", "pairing/requests/"+requestID+"/claim", map[string]any{}, headers); code != 409 ||
			errorCode(t, response) != "PAIRING_ALREADY_CLAIMED" {
			t.Fatalf("segundo reclamo con X-Client-Kind %q: %d %s", kind, code, response)
		}
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.devices") != devicesBefore+1 ||
		fixture.count(t, "SELECT count(*)::int FROM app.devices WHERE id=$1 AND kind='kitchen'", paired.deviceID) != 1 {
		t.Fatal("el reclamo no dejó una sola pantalla kitchen")
	}
	// El tipo no se cambia después: PATCH devices/{id} no acepta kind (ni ninguna ruta lo escribe) y la credencial de la
	// pantalla no sirve como escritorio ni como meseros.
	if code, response := fixture.send(fixture.admin, "PATCH", "devices/"+paired.deviceID, map[string]any{"name": "Tableta R1",
		"keyboard_mode": "auto", "kind": "desktop"}, nil); code != 400 {
		t.Fatalf("PATCH con kind: %d %s", code, response)
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.devices WHERE id=$1 AND kind='kitchen'", paired.deviceID) != 1 {
		t.Fatal("el tipo de la pantalla cambió")
	}
	for _, kind := range []string{"desktop", "android"} {
		code, response := fixture.send(paired, "GET", "auth/me", nil, map[string]string{"X-Client-Kind": kind, "X-Client-Version": "0.15.0"})
		if code != 400 || errorCode(t, response) != "CLIENT_KIND_MISMATCH" {
			t.Fatalf("pantalla con X-Client-Kind %s: %d %s", kind, code, response)
		}
	}
	// Sin X-Client-Kind (herramientas locales) sigue siendo una pantalla: su lista cerrada.
	if code, response := fixture.send(paired, "GET", "auth/me", nil, nil); code != 403 || errorCode(t, response) != "KITCHEN_DISPLAY_ONLY" {
		t.Fatalf("pantalla sin X-Client-Kind: %d %s", code, response)
	}
	// Al revés: una solicitud de escritorio o de meseros no se vuelve pantalla al reclamarla con X-Client-Kind kitchen.
	desktopRequest, desktopSecret := pair("desktop", nil)
	desktopSecret["X-Client-Kind"] = "kitchen"
	desktopSecret["X-Client-Version"] = "0.1.0"
	code, response = fixture.send(testTerminal{}, "POST", "pairing/requests/"+desktopRequest+"/claim", map[string]any{}, desktopSecret)
	if code != 200 {
		t.Fatalf("reclamar la de escritorio: %d %s", code, response)
	}
	desktop := decodeResponse[map[string]string](t, response)
	if fixture.count(t, "SELECT count(*)::int FROM app.devices WHERE id=$1 AND kind='desktop'", desktop["device_id"]) != 1 {
		t.Fatal("la solicitud de escritorio no quedó como escritorio")
	}
	asDesktop := testTerminal{deviceID: desktop["device_id"], deviceToken: desktop["device_token"]}
	if code, response := fixture.send(asDesktop, "GET", "kitchen-display/orders", nil, kitchenHeaders); code != 400 ||
		errorCode(t, response) != "CLIENT_KIND_MISMATCH" {
		t.Fatalf("escritorio con X-Client-Kind kitchen: %d %s", code, response)
	}
	if code, response := fixture.send(asDesktop, "GET", "kitchen-display/orders", nil, nil); code != 403 ||
		errorCode(t, response) != "KITCHEN_DISPLAY_REQUIRED" {
		t.Fatalf("escritorio sin X-Client-Kind en la pantalla: %d %s", code, response)
	}
	for _, id := range []string{paired.deviceID, asDesktop.deviceID} {
		fixture.adminCall(t, "POST", "devices/"+id+"/revoke", map[string]any{}, 200)
	}
}

func testR1ClosedList(t *testing.T, fixture *r1Fixture) {
	someID := "00000000-0000-4000-8000-000000000001"
	stolen := fixture.display
	stolen.sessionToken = fixture.admin.sessionToken
	headers := map[string]string{"X-Client-Kind": "kitchen", "X-Client-Version": "0.1.0"}
	// Una ruta de cada familia (sesión, entrada, PIN, envío, agente, avisos, restaurante, cobros, inventario, reportes,
	// mantenimiento, terminales, emparejamiento administrado, licencia, ajustes, cocina del escritorio, catálogo…).
	routes := []string{
		"auth/login", "auth/me", "auth/logout", "auth/password", "auth/pin-users", "auth/pin-login", "auth/sessions",
		"restaurant/state", "restaurant/accounts", "restaurant/accounts/" + someID, "restaurant/accounts/" + someID + "/confirm",
		"restaurant/offline-orders", "restaurant/areas", "takeaway/orders",
		"payments/shifts", "payments/accounts/" + someID + "/payments", "payments/registers",
		"inventory/items", "inventory/movements", "reports/sales", "reports/export/sales",
		"maintenance/backups", "maintenance/test-data/remove", "maintenance/restart",
		"devices", "devices/" + fixture.second.deviceID, "devices/" + fixture.second.deviceID + "/revoke",
		"devices/" + fixture.display.deviceID + "/seat/release", "devices/" + fixture.display.deviceID + "/configuration",
		"devices/pairing-requests", "devices/invitations", "devices/sessions-summary",
		"license/activate", "license/refresh", "license/deactivate", "license/identity/regenerate", "license/clock/check",
		"license/status/extra",
		"settings/features", "settings/general", "settings/security",
		"print-agent/printers", "print-agent/jobs", "ready-alerts", "ready-alerts/stream",
		"kitchen/board", "kitchen/states", "kitchen/display-mode", "kitchen/access", "kitchen/tickets", "kitchen/settings",
		"catalog/products", "catalog/categories", "users", "roles", "audit", "documents/templates", "preferences",
		"permissions/catalog", "terminal/home", "events/snapshot/extra",
		// Variantes de la propia lista cerrada: otro método o una ruta parecida no son la ruta.
		"kitchen-display", "kitchen-display/", "kitchen-display/orders/", "kitchen-display/orders/ready/", "kitchen-display/state",
		"KITCHEN-DISPLAY/orders", "Kitchen-Display/connect", "kitchen-display/ORDERS", "Auth/Me", "AUTH/LOGIN",
	}
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"}
	allowed := map[string]bool{
		"POST kitchen-display/connect": true, "GET kitchen-display/orders": true, "GET kitchen-display/recent": true,
		"POST kitchen-display/items/ready": true, "POST kitchen-display/orders/ready": true, "POST kitchen-display/recover": true,
		"POST kitchen-display/receipts": true, "POST kitchen-display/settings-access": true, "POST kitchen-display/unpair": true,
		"GET events/snapshot": true, "GET license/status": true,
	}
	accountsBefore := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts")
	sessionsBefore := fixture.count(t, "SELECT count(*)::int FROM app.sessions")
	seen := fixture.sessionSeen(t)
	checked := 0
	for _, network := range []bool{false, true} {
		for _, route := range routes {
			for _, method := range methods {
				if allowed[method+" "+route] {
					continue
				}
				response := fixture.routed(stolen, method, "/api/v1/"+route, map[string]any{}, headers, network)
				checked++
				if response.status >= 200 && response.status < 300 {
					t.Fatalf("%s %s (red %v) respondió %d a la pantalla: %s", method, route, network, response.status, response.body)
				}
				// Todo lo que llega a la aplicación responde 403 KITCHEN_DISPLAY_ONLY. HEAD puede venir sin cuerpo.
				if response.status != 403 || (method != "HEAD" && response.code() != "KITCHEN_DISPLAY_ONLY") {
					t.Fatalf("%s %s (red %v): %d %s", method, route, network, response.status, response.body)
				}
			}
		}
	}
	// Rutas escapadas, con barras dobles o con «..»: el enrutador las limpia (redirección) o las decodifica a una ruta de
	// la lista; ninguna llega a otra cosa.
	for _, target := range []string{
		"/api/v1/auth%2Fme", "/api/v1/auth%2flogin", "/api/v1/restaurant%2Fstate", "/api/v1//auth/me", "/api/v1/kitchen-display/../auth/me",
		"/api/v1/kitchen-display/./orders", "/api/v1/kitchen-display//orders", "/api/v1/%61uth/me", "/api/v1/devices%2F" + fixture.second.deviceID + "%2Frevoke",
	} {
		for _, method := range []string{"GET", "POST"} {
			response := fixture.routed(stolen, method, target, map[string]any{}, headers, true)
			if response.status >= 200 && response.status < 300 {
				t.Fatalf("%s %s respondió %d a la pantalla: %s", method, target, response.status, response.body)
			}
			if response.status == 403 && response.code() != "KITCHEN_DISPLAY_ONLY" {
				t.Fatalf("%s %s: %d %s", method, target, response.status, response.body)
			}
		}
	}
	// La ruta de la lista escapada es la misma ruta (se decodifica antes de compararla): no abre nada más.
	if response := fixture.routed(fixture.display, "GET", "/api/v1/kitchen-display%2Forders", nil, headers, true); response.status != 200 {
		t.Fatalf("la ruta escapada de la lista: %d %s", response.status, response.body)
	}
	// OPTIONS lo responde el enrutador sin llegar a la aplicación (sin Origin permitido, 403 PREFLIGHT_REJECTED).
	for _, route := range []string{"auth/me", "kitchen-display/orders", "restaurant/state"} {
		response := fixture.routed(stolen, "OPTIONS", "/api/v1/"+route, nil, headers, true)
		if response.status != 403 || response.code() != "PREFLIGHT_REJECTED" {
			t.Fatalf("OPTIONS %s: %d %s", route, response.status, response.body)
		}
	}
	// La credencial de envío del mesero tampoco abre nada a la pantalla.
	offline := map[string]string{"X-Client-Kind": "kitchen", "X-Client-Version": "0.1.0", administration.OfflineCredentialHeader: strings.Repeat("a", 43)}
	if response := fixture.routed(fixture.display, "POST", "/api/v1/restaurant/offline-orders", map[string]any{}, offline, true); response.status != 403 ||
		response.code() != "KITCHEN_DISPLAY_ONLY" {
		t.Fatalf("credencial de envío en la pantalla: %d %s", response.status, response.body)
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts") != accountsBefore ||
		fixture.count(t, "SELECT count(*)::int FROM app.sessions") != sessionsBefore {
		t.Fatal("la pantalla abrió una cuenta o una sesión")
	}
	// authenticateSession nunca corrió con el Authorization de la sesión del administrador.
	if !fixture.sessionSeen(t).Equal(seen) {
		t.Fatal("una petición de la pantalla validó la sesión del administrador")
	}
	// Las rutas públicas de antes de autenticar no dan nada más que a cualquiera sin credencial.
	if response := fixture.routed(stolen, "POST", "/api/v1/setup", map[string]any{"name": "Otro"}, headers, true); response.status < 400 {
		t.Fatalf("setup con la pantalla: %d %s", response.status, response.body)
	}
	if response := fixture.routed(stolen, "POST", "/api/v1/devices/enroll", map[string]any{"code": strings.Repeat("a", 43), "name": "x"}, headers, true); response.status != 403 ||
		response.code() != "ENROLLMENT_LOCAL_ONLY" {
		t.Fatalf("enroll por la red: %d %s", response.status, response.body)
	}
	t.Logf("%d combinaciones de ruta y método rechazadas a la pantalla", checked)
}

func testR1OtherKinds(t *testing.T, fixture *r1Fixture) {
	phone := fixture.waiterPhone(t)
	phoneWithSession := phone
	phoneWithSession.sessionToken = fixture.admin.sessionToken
	routes := []struct{ method, route string }{
		{"POST", "kitchen-display/connect"}, {"GET", "kitchen-display/orders"}, {"GET", "kitchen-display/recent"},
		{"POST", "kitchen-display/items/ready"}, {"POST", "kitchen-display/orders/ready"}, {"POST", "kitchen-display/recover"},
		{"POST", "kitchen-display/receipts"}, {"POST", "kitchen-display/settings-access"}, {"POST", "kitchen-display/unpair"},
		{"GET", "kitchen-display/orders/"}, {"HEAD", "kitchen-display/orders"}, {"DELETE", "kitchen-display/connect"},
	}
	clients := []struct {
		name     string
		terminal testTerminal
		headers  map[string]string
	}{
		{"escritorio con sesión", fixture.admin, map[string]string{"X-Client-Kind": "desktop", "X-Client-Version": "0.15.0"}},
		{"escritorio sin X-Client-Kind", fixture.admin, map[string]string{"X-Client-Version": "0.15.0"}},
		{"escritorio sin sesión", testTerminal{deviceID: fixture.admin.deviceID, deviceToken: fixture.admin.deviceToken},
			map[string]string{"X-Client-Kind": "desktop", "X-Client-Version": "0.15.0"}},
		{"meseros sin sesión", phone, map[string]string{"X-Client-Kind": "android", "X-Client-Version": "0.4.0"}},
		{"meseros con el Authorization de otra sesión", phoneWithSession, map[string]string{"X-Client-Kind": "android", "X-Client-Version": "0.4.0"}},
	}
	settingsBefore := fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action LIKE 'kitchen_display.%'")
	for _, client := range clients {
		for _, network := range []bool{false, true} {
			for _, route := range routes {
				body := map[string]any{"username": "admin", "password": "admin"}
				response := fixture.routed(client.terminal, route.method, "/api/v1/"+route.route, body, client.headers, network)
				if response.status != 403 || (route.method != "HEAD" && response.code() != "KITCHEN_DISPLAY_REQUIRED") {
					t.Fatalf("%s: %s %s (red %v): %d %s", client.name, route.method, route.route, network, response.status, response.body)
				}
			}
		}
	}
	// Escapada o en mayúsculas no es la ruta de la pantalla: nunca llega a su lógica.
	for _, target := range []string{"/api/v1/kitchen-display%2Forders", "/api/v1/KITCHEN-DISPLAY/orders", "/api/v1/kitchen-display//orders"} {
		response := fixture.routed(fixture.admin, "GET", target, nil, nil, true)
		if response.status >= 200 && response.status < 300 {
			t.Fatalf("%s respondió %d al escritorio: %s", target, response.status, response.body)
		}
	}
	// La pantalla con el X-Client-Kind de otro tipo, ni sus rutas.
	for _, kind := range []string{"desktop", "android"} {
		response := fixture.routed(fixture.display, "GET", "/api/v1/kitchen-display/orders", nil,
			map[string]string{"X-Client-Kind": kind, "X-Client-Version": "0.15.0"}, true)
		if response.status != 400 || response.code() != "CLIENT_KIND_MISMATCH" {
			t.Fatalf("pantalla con X-Client-Kind %s: %d %s", kind, response.status, response.body)
		}
	}
	// Sin credencial de dispositivo: 401.
	if response := fixture.routed(testTerminal{}, "GET", "/api/v1/kitchen-display/orders", nil, kitchenHeaders, true); response.status != 401 ||
		response.code() != "DEVICE_REQUIRED" {
		t.Fatalf("sin credencial: %d %s", response.status, response.body)
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action LIKE 'kitchen_display.%'") != settingsBefore {
		t.Fatal("otro tipo de dispositivo llegó a la lógica de la pantalla")
	}
	fixture.adminCall(t, "POST", "devices/"+phone.deviceID+"/revoke", map[string]any{}, 200)
}

func testR1ClientVersion(t *testing.T, fixture *r1Fixture) {
	for _, check := range []struct {
		headers map[string]string
		status  int
		code    string
	}{
		{map[string]string{"X-Client-Kind": "kitchen", "X-Client-Version": "0.0.9"}, 426, "CLIENT_UPDATE_REQUIRED"},
		{map[string]string{"X-Client-Kind": "kitchen"}, 426, "CLIENT_UPDATE_REQUIRED"},
		{map[string]string{"X-Client-Kind": "KITCHEN", "X-Client-Version": "0.1.0"}, 400, "CLIENT_KIND_INVALID"},
		{map[string]string{"X-Client-Kind": "kitchen", "X-Client-Version": "0.1"}, 400, "CLIENT_VERSION_INVALID"},
		{map[string]string{"X-Client-Kind": "kitchen", "X-Client-Version": "0.1.0"}, 200, ""},
	} {
		response := fixture.routed(fixture.display, "GET", "/api/v1/kitchen-display/orders", nil, check.headers, true)
		if response.status != check.status || (check.code != "" && response.code() != check.code) {
			t.Fatalf("versión %v: %d %s", check.headers, response.status, response.body)
		}
	}
	// La mínima de la pantalla (0.1.0) no le sirve a una terminal de escritorio que diga ser pantalla.
	response := fixture.routed(fixture.admin, "GET", "/api/v1/auth/me", nil, map[string]string{"X-Client-Kind": "kitchen", "X-Client-Version": "0.1.0"}, true)
	if response.status != 400 || response.code() != "CLIENT_KIND_MISMATCH" {
		t.Fatalf("escritorio 0.1.0 como pantalla: %d %s", response.status, response.body)
	}
}

func testR1StationScope(t *testing.T, fixture *r1Fixture) {
	// La primera pantalla solo muestra Cocina; la segunda, solo Barra. Postres es solo de impresora.
	fixture.restrict(t, fixture.display, fixture.cocina.ID)
	fixture.restrict(t, fixture.second, fixture.barra.ID)
	cursor := fixture.orders(t, fixture.display).Cursor
	accountID := fixture.order(t, 0, draftLine(fixture.product, 2), draftLine(fixture.fries, 1), draftLine(fixture.pastel, 1))
	// Una cuenta sin nada de Cocina (Barra y Postres): su encabezado no le interesa a la primera pantalla.
	foreign := fixture.order(t, 1, draftLine(fixture.fries, 1), draftLine(fixture.pastel, 1))
	view := fixture.orders(t, fixture.display)
	own := card(t, view, accountID)
	if own == nil || len(own.Items) != 1 || own.Items[0].StationID != fixture.cocina.ID || card(t, view, foreign) != nil {
		t.Fatalf("tarjetas de la pantalla de Cocina: %+v", view.Orders)
	}
	for _, card := range view.Orders {
		for _, item := range card.Items {
			if item.StationID != fixture.cocina.ID {
				t.Fatalf("la pantalla de Cocina muestra otra estación: %+v", item)
			}
		}
	}
	for _, receipt := range view.ReceiptsPending {
		if receipt.StationID != fixture.cocina.ID {
			t.Fatalf("acuse pendiente de otra estación: %+v", receipt)
		}
	}
	cocina := fixture.tasksAt(t, accountID, fixture.cocina.ID)
	barra := fixture.tasksAt(t, accountID, fixture.barra.ID)
	papel := fixture.tasksAt(t, accountID, fixture.papel.ID)
	expectDenied := func(route string, tasks []kitchen.DisplayTaskRef) {
		t.Helper()
		fixture.displayError(t, fixture.display, "POST", "kitchen-display/"+route, map[string]any{"tasks": tasks}, 403, "STATION_SCOPE_DENIED")
	}
	// Una tarea ajena en el lote: 403 y no se marca ni la propia.
	expectDenied("items/ready", append(append([]kitchen.DisplayTaskRef{}, cocina...), barra[0]))
	expectDenied("orders/ready", append(append([]kitchen.DisplayTaskRef{}, cocina...), papel[0]))
	expectDenied("orders/ready", barra)
	for _, task := range cocina {
		if state := fixture.taskState(t, task.ID); state != "released" {
			t.Fatalf("una marca rechazada cambió la tarea de Cocina: %s", state)
		}
	}
	// La versión vista debe ser la de la tarea; un identificador en mayúsculas no es otra tarea.
	changed := []kitchen.DisplayTaskRef{{ID: cocina[0].ID, VersionID: barra[0].VersionID}}
	if barra[0].VersionID == cocina[0].VersionID {
		changed[0].VersionID = "00000000-0000-4000-8000-000000000002"
	}
	result := fixture.mark(t, fixture.display, "items/ready", changed)
	if result.Updated != 0 || len(result.Skipped) != 1 || result.Skipped[0].Reason != "changed" {
		t.Fatalf("marca con otra versión: %+v", result)
	}
	upper := []kitchen.DisplayTaskRef{{ID: strings.ToUpper(barra[0].ID), VersionID: barra[0].VersionID}}
	if code, response := fixture.send(fixture.display, "POST", "kitchen-display/items/ready", map[string]any{"tasks": upper}, kitchenHeaders); code < 400 {
		t.Fatalf("tarea ajena en mayúsculas: %d %s", code, response)
	}
	if fixture.taskState(t, barra[0].ID) != "released" {
		t.Fatal("la tarea de Barra cambió con el identificador en mayúsculas")
	}
	// La segunda pantalla marca Barra; la primera no la puede recuperar.
	fixture.mark(t, fixture.second, "orders/ready", barra)
	expectDenied("recover", barra)
	if fixture.taskState(t, barra[0].ID) != "ready" {
		t.Fatal("la primera pantalla recuperó una tarea de Barra")
	}
	// Recientes de la primera no trae nada de Barra.
	recent := decodeResponse[kitchen.DisplayRecent](t, fixture.displayCall(t, fixture.display, "GET", "kitchen-display/recent", nil, 200))
	for _, order := range recent.Orders {
		for _, item := range order.Items {
			if item.StationID != fixture.cocina.ID {
				t.Fatalf("recientes con otra estación: %+v", item)
			}
		}
	}
	// Acuses de comandas de otras estaciones: 403 (o 400 si la comanda no tiene esa estación), nunca un acuse.
	receiptsBefore := fixture.count(t, "SELECT count(*)::int FROM app.kitchen_kds_receipts WHERE device_id=$1", fixture.display.deviceID)
	for _, receipt := range []map[string]string{
		{"ticket_id": fixture.ticketOf(t, accountID, fixture.barra.ID), "station_id": fixture.barra.ID},
		{"ticket_id": fixture.ticketOf(t, accountID, fixture.papel.ID), "station_id": fixture.papel.ID},
		{"ticket_id": fixture.ticketOf(t, foreign, fixture.barra.ID), "station_id": fixture.barra.ID},
		{"ticket_id": fixture.ticketOf(t, accountID, fixture.barra.ID), "station_id": strings.ToUpper(fixture.cocina.ID)},
	} {
		code, response := fixture.send(fixture.display, "POST", "kitchen-display/receipts", map[string]any{"receipts": []map[string]string{receipt}}, kitchenHeaders)
		if code < 400 {
			t.Fatalf("acuse ajeno %v: %d %s", receipt, code, response)
		}
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.kitchen_kds_receipts WHERE device_id=$1", fixture.display.deviceID) != receiptsBefore {
		t.Fatal("la pantalla confirmó una comanda de otra estación")
	}
	// Eventos: nada de otras estaciones, ni el encabezado de la cuenta sin Cocina, ni la configuración de otra pantalla.
	current := fixture.refresh(t, foreign)
	var adminID string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username='admin'").Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	fixture.mutate(t, "POST", "restaurant/accounts/"+foreign+"/assign", map[string]any{"revision": current.Account.Revision,
		"assigned_user_id": adminID}, 200)
	ownBill := fixture.refresh(t, accountID)
	fixture.mutate(t, "POST", "restaurant/accounts/"+accountID+"/assign", map[string]any{"revision": ownBill.Account.Revision,
		"assigned_user_id": adminID}, 200)
	fixture.adminCall(t, "PATCH", "devices/"+fixture.second.deviceID, map[string]any{"name": "Pantalla barra R1", "keyboard_mode": "auto",
		"kitchen_display_theme": "light"}, 200)
	batch := fixture.displayEvents(t, fixture.display, cursor)
	allowedKinds := map[string]bool{"license.changed": true, kitchen.DisplayConfigurationChangedKind: true, "kitchen.ticket.created": true,
		"kitchen.state.changed": true, "kitchen.change.pending": true, "kitchen.kds.received": true, "restaurant.account.updated": true,
		"restaurant.account.move": true, "restaurant.account.assign": true, "restaurant.account.merged": true, "restaurant.account.split": true,
		"restaurant.account.cancelled": true, "restaurant.items.transferred": true}
	ownAssign := false
	for _, event := range batch.Events {
		if !allowedKinds[event.Kind] {
			t.Fatalf("evento que la pantalla no debe recibir: %+v", event)
		}
		if event.StationID != nil && *event.StationID != fixture.cocina.ID {
			t.Fatalf("evento de otra estación: %+v", event)
		}
		if event.AggregateID != nil && (*event.AggregateID == foreign || *event.AggregateID == fixture.second.deviceID) {
			t.Fatalf("evento de una cuenta sin Cocina o de otra pantalla: %+v", event)
		}
		if event.Kind == "restaurant.account.assign" && event.AggregateID != nil && *event.AggregateID == accountID {
			ownAssign = true
		}
	}
	if !ownAssign {
		t.Fatalf("la pantalla no recibió el cambio de mesero de su cuenta: %+v", batch.Events)
	}
	// La estación deja de recibir en pantalla: desaparece del alcance al momento (sin reiniciar ni reconectar).
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_stations SET reception='printer' WHERE id=$1", fixture.barra.ID); err != nil {
		t.Fatal(err)
	}
	second := fixture.orders(t, fixture.second)
	if len(second.Display.Stations) != 0 || len(second.Orders) != 0 {
		t.Fatalf("pantalla de una estación que pasó a solo impresora: %+v", second)
	}
	fixture.displayError(t, fixture.second, "POST", "kitchen-display/recover", map[string]any{"tasks": barra}, 403, "STATION_SCOPE_DENIED")
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_stations SET reception='both' WHERE id=$1", fixture.barra.ID); err != nil {
		t.Fatal(err)
	}
	// Dejar lo de Cocina listo para lo que sigue.
	fixture.mark(t, fixture.display, "orders/ready", cocina)
}

func testR1SettingsAccess(t *testing.T, fixture *r1Fixture) {
	access := func(terminal testTerminal, username, password string) (int, []byte) {
		return fixture.send(terminal, "POST", "kitchen-display/settings-access", map[string]string{"username": username, "password": password}, kitchenHeaders)
	}
	sessionsBefore := fixture.count(t, "SELECT count(*)::int FROM app.sessions")
	openBefore := fixture.count(t, "SELECT count(*)::int FROM app.sessions WHERE revoked_at IS NULL")
	// Con éxito: solo {authorized: true}, sin sesión, sin token y sin permisos.
	code, response := access(fixture.display, "admin", "admin")
	if code != 200 || strings.TrimSpace(string(response)) != `{"authorized":true}` {
		t.Fatalf("contraseña del administrador: %d %s", code, response)
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.sessions") != sessionsBefore ||
		fixture.count(t, "SELECT count(*)::int FROM app.sessions WHERE revoked_at IS NULL") != openBefore {
		t.Fatal("settings-access creó o cerró una sesión")
	}
	// Con el Authorization del administrador y una contraseña equivocada: la sesión no sustituye a la contraseña.
	stolen := fixture.display
	stolen.sessionToken = fixture.admin.sessionToken
	if code, response := access(stolen, "admin", "otra"); code != 401 || errorCode(t, response) != "AUTHORIZATION_INVALID" {
		t.Fatalf("contraseña equivocada con Authorization: %d %s", code, response)
	}
	// Un usuario válido sin devices.manage: 403, sin token.
	if code, response := access(fixture.display, "mesero_r1", "Clave temporal de mesero_r1"); code != 403 || errorCode(t, response) != "PERMISSION_DENIED" ||
		strings.Contains(string(response), "token") {
		t.Fatalf("usuario sin devices.manage: %d %s", code, response)
	}
	// El mismo límite de intentos que la entrada (app.login_attempts por terminal y usuario): cinco fallos y después ni
	// la contraseña correcta pasa.
	password := "Clave R1 que no se registra"
	for attempt := range 5 {
		if code, response := access(fixture.second, "admin", password); code != 401 || errorCode(t, response) != "AUTHORIZATION_INVALID" {
			t.Fatalf("intento %d: %d %s", attempt+1, code, response)
		}
	}
	if code, response := access(fixture.second, "admin", "admin"); code != 429 || errorCode(t, response) != "AUTHORIZATION_THROTTLED" {
		t.Fatalf("después de cinco fallos: %d %s", code, response)
	}
	if fixture.count(t, `SELECT count(*)::int FROM app.login_attempts WHERE device_id=$1 AND username='admin' AND locked_until>now()`,
		fixture.second.deviceID) != 1 {
		t.Fatal("el bloqueo no quedó en la tabla de la entrada")
	}
	// El bloqueo es de esa pantalla: la otra sigue con su propio contador.
	if code, response := access(fixture.display, "admin", "admin"); code != 200 {
		t.Fatalf("otra pantalla después del bloqueo: %d %s", code, response)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, "DELETE FROM app.login_attempts WHERE device_id=$1", fixture.second.deviceID); err != nil {
		t.Fatal(err)
	}
	// La contraseña no queda en la auditoría, el historial ni los intentos.
	for _, query := range []string{
		"SELECT count(*)::int FROM app.audit_events WHERE before_state::text LIKE '%'||$1||'%' OR after_state::text LIKE '%'||$1||'%'",
		"SELECT count(*)::int FROM app.kitchen_history WHERE coalesce(before_value::text,'') LIKE '%'||$1||'%' OR coalesce(after_value::text,'') LIKE '%'||$1||'%'",
		"SELECT count(*)::int FROM app.login_attempts WHERE username LIKE '%'||$1||'%'",
	} {
		if fixture.count(t, query, password) != 0 {
			t.Fatalf("la contraseña quedó registrada: %s", query)
		}
	}
	// Un usuario desactivado no autoriza.
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.users SET active=false WHERE id=$1", fixture.waiterID); err != nil {
		t.Fatal(err)
	}
	if code, response := access(fixture.display, "mesero_r1", "Clave temporal de mesero_r1"); code != 401 {
		t.Fatalf("usuario desactivado: %d %s", code, response)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.users SET active=true WHERE id=$1", fixture.waiterID); err != nil {
		t.Fatal(err)
	}
	// R1-1 (corregido, K1-D29): un usuario de más de 40 caracteres o una contraseña de más de 256 son credenciales
	// incorrectas (401), sin validarse contra nadie ni contar un intento; los campos vacíos, 400.
	attemptsBefore := fixture.count(t, "SELECT count(*)::int FROM app.login_attempts")
	for name, credentials := range map[string][2]string{
		"usuario de 41 caracteres":     {strings.Repeat("a", 41), "x"},
		"contraseña de 257 caracteres": {"admin", strings.Repeat("x", 257)},
	} {
		if code, response := access(fixture.display, credentials[0], credentials[1]); code != 401 || errorCode(t, response) != "AUTHORIZATION_INVALID" {
			t.Fatalf("%s: %d %s", name, code, response)
		}
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.login_attempts") != attemptsBefore {
		t.Fatal("un usuario o una contraseña demasiado largos contaron un intento")
	}
	for name, credentials := range map[string][2]string{"usuario vacío": {"  ", "admin"}, "contraseña vacía": {"admin", ""}} {
		if code, response := access(fixture.display, credentials[0], credentials[1]); code != 400 || errorCode(t, response) != "VALIDATION_ERROR" {
			t.Fatalf("%s: %d %s", name, code, response)
		}
	}
}

func testR1Seats(t *testing.T, fixture *r1Fixture) {
	clock := seatstest.NewClock(time.Now().UTC())
	source := seatstest.NewSource(seatstest.ContractMaximum, seatstest.ContractMaximum, 1)
	fixture.service.Seats = licenseseats.NewManager(licenseseats.Options{Limits: source, Now: clock.Now})
	defer func() { fixture.service.Seats = seatstest.Roomy() }()
	resetSeats := func() {
		t.Helper()
		if _, err := fixture.pool.Exec(fixture.ctx, "DELETE FROM app.license_seats"); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.devices SET kitchen_display_seat_released_at=NULL WHERE kind='kitchen'"); err != nil {
			t.Fatal(err)
		}
	}
	inUse := func() int {
		t.Helper()
		return fixture.count(t, `SELECT count(*)::int FROM app.license_seats s JOIN app.devices d ON d.id=s.device_id
            WHERE s.kind='kitchen_display' AND (s.session_id IS NOT NULL OR (d.kind='kitchen' AND d.revoked_at IS NULL))`)
	}
	// Varias pantallas a la vez con un solo cupo: exactamente una lo ocupa (asignación atómica con 7820148311).
	displays := []testTerminal{}
	for index := range 8 {
		displays = append(displays, fixture.kitchenDevice(t, fmt.Sprintf("Pantalla R1 simultánea %d", index+1)))
	}
	for round := range 5 {
		resetSeats()
		var wait sync.WaitGroup
		codes := make([]int, len(displays))
		start := make(chan struct{})
		for index, display := range displays {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				codes[index], _ = fixture.send(display, "POST", "kitchen-display/connect", map[string]any{}, kitchenHeaders)
			}()
		}
		close(start)
		wait.Wait()
		success := 0
		for _, code := range codes {
			switch code {
			case 200:
				success++
			case 409:
			default:
				t.Fatalf("ronda %d: respuesta inesperada %v", round+1, codes)
			}
		}
		if success != 1 || inUse() != 1 {
			t.Fatalf("ronda %d: %d pantallas ocuparon el único cupo (%d en uso): %v", round+1, success, inUse(), codes)
		}
	}
	// Una pantalla Android y el modo Cocina del escritorio a la vez: también uno solo.
	for round := range 5 {
		resetSeats()
		var wait sync.WaitGroup
		var displayCode, desktopCode int
		start := make(chan struct{})
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			displayCode, _ = fixture.send(displays[0], "POST", "kitchen-display/connect", map[string]any{}, kitchenHeaders)
		}()
		go func() {
			defer wait.Done()
			<-start
			desktopCode, _ = fixture.send(fixture.admin, "POST", "kitchen/display-mode", map[string]any{"active": true}, nil)
		}()
		close(start)
		wait.Wait()
		if (displayCode == 200) == (desktopCode == 200) || inUse() != 1 {
			t.Fatalf("ronda %d: pantalla %d, escritorio %d, en uso %d", round+1, displayCode, desktopCode, inUse())
		}
	}
	fixture.send(fixture.admin, "POST", "kitchen/display-mode", map[string]any{"active": false}, nil)
	// Una desconexión no libera el cupo: la que lo tiene no se comunica en tres días y la otra sigue sin cupo.
	resetSeats()
	holder, other := displays[0], displays[1]
	fixture.displayCall(t, holder, "POST", "kitchen-display/connect", map[string]any{}, 200)
	clock.Advance(72 * time.Hour)
	fixture.displayError(t, other, "POST", "kitchen-display/connect", map[string]any{}, 409, "LICENSE_SEAT_UNAVAILABLE")
	fixture.displayCall(t, holder, "GET", "kitchen-display/orders", nil, 200)
	// «Liberar cupo» frente a connect de la misma pantalla al mismo tiempo: nunca queda «cupo liberado» con el cupo
	// ocupado (la pantalla no lo vuelve a ocupar sola, K1-D6).
	source.Set(seatstest.ContractMaximum, seatstest.ContractMaximum, 3)
	for round := range 15 {
		if code, response := fixture.send(holder, "POST", "kitchen-display/connect", map[string]any{"take_seat": true}, kitchenHeaders); code != 200 {
			t.Fatalf("ronda %d: volver a ocupar: %d %s", round+1, code, response)
		}
		var wait sync.WaitGroup
		var releaseCode, connectCode int
		start := make(chan struct{})
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			releaseCode, _ = fixture.send(fixture.admin, "POST", "devices/"+holder.deviceID+"/seat/release", map[string]any{}, nil)
		}()
		go func() {
			defer wait.Done()
			<-start
			connectCode, _ = fixture.send(holder, "POST", "kitchen-display/connect", map[string]any{}, kitchenHeaders)
		}()
		close(start)
		wait.Wait()
		released := fixture.count(t, "SELECT count(*)::int FROM app.devices WHERE id=$1 AND kitchen_display_seat_released_at IS NOT NULL", holder.deviceID)
		held := fixture.count(t, "SELECT count(*)::int FROM app.license_seats WHERE device_id=$1 AND kind='kitchen_display'", holder.deviceID)
		if releaseCode != 200 || (connectCode != 200 && connectCode != 409) || released != 1 || held != 0 {
			t.Fatalf("ronda %d: liberar %d, connect %d, cupo liberado %d, cupo ocupado %d", round+1, releaseCode, connectCode, released, held)
		}
		// Después de «Liberar cupo», ninguna ruta lo vuelve a ocupar sin «Usar esta pantalla».
		fixture.displayError(t, holder, "POST", "kitchen-display/connect", map[string]any{}, 409, "KITCHEN_DISPLAY_SEAT_RELEASED")
		fixture.displayError(t, holder, "GET", "kitchen-display/orders", nil, 409, "KITCHEN_DISPLAY_SEAT_RELEASED")
		fixture.displayError(t, holder, "POST", "kitchen-display/connect", map[string]any{"take_seat": false}, 409, "KITCHEN_DISPLAY_SEAT_RELEASED")
		if fixture.count(t, "SELECT count(*)::int FROM app.license_seats WHERE device_id=$1", holder.deviceID) != 0 {
			t.Fatalf("ronda %d: la pantalla volvió a ocupar el cupo sola", round+1)
		}
	}
	// Revocar sí lo libera, y la otra lo toma.
	source.Set(seatstest.ContractMaximum, seatstest.ContractMaximum, 1)
	resetSeats()
	fixture.displayCall(t, holder, "POST", "kitchen-display/connect", map[string]any{}, 200)
	fixture.displayError(t, other, "POST", "kitchen-display/connect", map[string]any{}, 409, "LICENSE_SEAT_UNAVAILABLE")
	fixture.adminCall(t, "POST", "devices/"+holder.deviceID+"/revoke", map[string]any{}, 200)
	fixture.displayCall(t, other, "POST", "kitchen-display/connect", map[string]any{}, 200)
	if inUse() != 1 {
		t.Fatalf("en uso después de revocar: %d", inUse())
	}
}

func testR1RevokedInFlight(t *testing.T, fixture *r1Fixture) {
	display := fixture.kitchenDevice(t, "Pantalla R1 revocada en curso")
	fixture.displayCall(t, display, "POST", "kitchen-display/connect", map[string]any{}, 200)
	accountID := fixture.order(t, 2, draftLine(fixture.product, 1))
	tasks := fixture.tasksAt(t, accountID, fixture.cocina.ID)
	// Otra escritura de cocina tiene 7820148305: la marca de la pantalla ya se autenticó y espera el lock.
	holder, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(context.Background())
	if _, err := holder.Exec(fixture.ctx, "SELECT pg_advisory_xact_lock(7820148305)"); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		code int
		body []byte
	}
	done := make(chan outcome, 1)
	go func() {
		code, body := fixture.send(display, "POST", "kitchen-display/items/ready", map[string]any{"tasks": tasks}, kitchenHeaders)
		done <- outcome{code, body}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for fixture.count(t, `SELECT count(*)::int FROM pg_locks WHERE locktype='advisory' AND NOT granted
        AND ((classid::bigint << 32) | objid::bigint) = 7820148305`) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("la marca no llegó a esperar el lock de cocina")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Terminales la revoca mientras espera; después se suelta el lock.
	fixture.adminCall(t, "POST", "devices/"+display.deviceID+"/revoke", map[string]any{}, 200)
	if err := holder.Rollback(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	result := <-done
	// R1-2 (corregido, K1-D29): la marca ve la revocación al tocar la fila del dispositivo, después del lock.
	if result.code != 401 || errorCode(t, result.body) != "DEVICE_REVOKED" {
		t.Fatalf("la marca en curso de una pantalla revocada: %d %s", result.code, result.body)
	}
	for _, task := range tasks {
		if state := fixture.taskState(t, task.ID); state != "released" {
			t.Fatalf("la tarea cambió después de revocar la pantalla: %s", state)
		}
	}
	testR1RevokedSettingsInFlight(t, fixture)
}

// testR1RevokedSettingsInFlight es el hallazgo R1-2: settings-access no exige el cupo, así que nada volvía a mirar la
// revocación después de authenticateDevice. Con la fila del dispositivo bloqueada por otra transacción, Terminales revoca
// la pantalla (queda primero en la cola de la fila) y después llega settings-access: su authenticateDevice ya leyó «no
// revocada» y su touchDevice espera detrás de la revocación. Antes respondía 200 {"authorized": true} a una pantalla ya
// revocada; con la corrección (K1-D29) responde 401 DEVICE_REVOKED, lo único que acepta la prueba, y no crea ninguna
// sesión.
func testR1RevokedSettingsInFlight(t *testing.T, fixture *r1Fixture) {
	display := fixture.kitchenDevice(t, "Pantalla R1 ajustes en curso")
	holder, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(context.Background())
	if _, err := holder.Exec(fixture.ctx, "SELECT id FROM app.devices WHERE id=$1 FOR UPDATE", display.deviceID); err != nil {
		t.Fatal(err)
	}
	waiting := func(prefix string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for fixture.count(t, `SELECT count(*)::int FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE $1||'%'`, prefix) == 0 {
			if time.Now().After(deadline) {
				t.Fatalf("no llegó a esperar: %s", prefix)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	revoked := make(chan int, 1)
	go func() {
		code, _ := fixture.send(fixture.admin, "POST", "devices/"+display.deviceID+"/revoke", map[string]any{}, nil)
		revoked <- code
	}()
	waiting("UPDATE app.devices SET revoked_at")
	sessionsBefore := fixture.count(t, "SELECT count(*)::int FROM app.sessions")
	type outcome struct {
		code int
		body []byte
	}
	opened := make(chan outcome, 1)
	go func() {
		code, body := fixture.send(display, "POST", "kitchen-display/settings-access", map[string]string{"username": "admin", "password": "admin"}, kitchenHeaders)
		opened <- outcome{code, body}
	}()
	waiting("UPDATE app.devices SET last_seen_at")
	if err := holder.Rollback(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if code := <-revoked; code != 200 {
		t.Fatalf("revocar: %d", code)
	}
	result := <-opened
	if result.code != 401 || errorCode(t, result.body) != "DEVICE_REVOKED" {
		t.Fatalf("settings-access en curso de una pantalla revocada (R1-2): %d %s", result.code, result.body)
	}
	if fixture.count(t, `SELECT count(*)::int FROM app.audit_events WHERE device_id=$1 AND action='kitchen_display.settings_opened'`,
		display.deviceID) != 0 {
		t.Fatal("la pantalla revocada abrió los ajustes")
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.sessions") != sessionsBefore {
		t.Fatal("settings-access creó una sesión")
	}
	if code, response := fixture.send(display, "POST", "kitchen-display/settings-access", map[string]string{"username": "admin", "password": "admin"},
		kitchenHeaders); code != 401 || errorCode(t, response) != "DEVICE_REVOKED" {
		t.Fatalf("settings-access después de revocar: %d %s", code, response)
	}
}
