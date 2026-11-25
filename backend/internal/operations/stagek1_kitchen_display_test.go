//go:build integration

package operations_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/licenseseats/seatstest"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/licensing/licensingtest"
	"local/ad-el-pos/server/internal/localsetup"
	"local/ad-el-pos/server/internal/operations"
)

// Etapa K1, Fase A (§2, §3 y §8 «Servidor» de la especificación; decisiones K1-D3 a K1-D17) contra PostgreSQL con la
// API real: la pantalla de cocina Android es un dispositivo kitchen sin usuario ni sesión. Comprueba:
//   - la lista cerrada: la pantalla no captura, no cobra ni administra (una petición por familia de rutas, también con
//     el Authorization de otra sesión), las rutas de la pantalla no responden a otro tipo de dispositivo y el tipo de
//     cliente debe coincidir;
//   - conectar (cupo y configuración) y el filtrado por estación y por recepción (impresora, pantalla o ambas), con la
//     impresión de hoy sin cambios;
//   - las tarjetas: mesa o «Para llevar» con el cliente, mesero, modificadores y notas, «NUEVO», «CANCELADO» y la tarjeta
//     cerrada por una cancelación hasta que se marca;
//   - marcar platillo y pedido (idempotente, por versión, estaciones «por pedido»), el aviso al mesero, recientes,
//     recuperar sin avisar y volver a marcar avisando;
//   - el acuse de recepción, los eventos de la pantalla (pedido nuevo, agregado, cancelación, marca desde otra
//     pantalla, encabezado de la cuenta, configuración) y la reconexión con el estado completo sin pérdidas ni
//     duplicados; revocar desconecta al momento;
//   - la versión instalada de los tres tipos, la contraseña del administrador para los ajustes, el cupo de cocina
//     ligado al dispositivo («Liberar cupo» y «Usar esta pantalla») y los estados de la licencia y de la hora.
//
// Usa su propia base desde cero.

type kitchenDisplayFixture struct {
	*waiterModeFixture
	display  testTerminal
	second   testTerminal
	printer  kitchen.Printer
	cocina   kitchen.Station
	barra    kitchen.Station
	papel    kitchen.Station
	pastel   catalog.Product
	waiterID string
}

// displayOrdersView es GET kitchen-display/orders.
type displayOrdersView struct {
	Cursor          int64                    `json:"cursor"`
	ServerTime      time.Time                `json:"server_time"`
	Display         kitchen.DisplayConfig    `json:"display"`
	Orders          []kitchen.DisplayCard    `json:"orders"`
	ReceiptsPending []kitchen.DisplayReceipt `json:"receipts_pending"`
	License         *administration.LicenseSummary
}

// displayMarkResult es la respuesta de marcar y recuperar.
type displayMarkResult struct {
	Updated   int `json:"updated"`
	Dismissed int `json:"dismissed"`
	Recovered int `json:"recovered"`
	Skipped   []struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	} `json:"skipped"`
}

// kitchenDevice registra una pantalla de cocina con la aplicación 0.1.0 como si se hubiera emparejado.
func (fixture *kitchenDisplayFixture) kitchenDevice(t *testing.T, name string) testTerminal {
	t.Helper()
	secret := administration.NewToken()
	var id string
	if err := fixture.pool.QueryRow(fixture.ctx, `INSERT INTO app.devices(name,secret_hash,kind,os,app_version,paired_at)
        VALUES ($1,$2,'kitchen','Android 14','0.1.0',now()) RETURNING id`, name, administration.TokenHash(secret)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return testTerminal{deviceID: id, deviceToken: secret}
}

// send hace una petición con las cabeceras dadas sin comprobar el estado. La pantalla manda X-Client-Kind kitchen y su
// versión; Authorization solo si la terminal trae sesión.
func (fixture *kitchenDisplayFixture) send(terminal testTerminal, method, route string, body any, headers map[string]string) (int, []byte) {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, "/api/v1/"+route, reader)
	request.RemoteAddr = "127.0.0.1:54100"
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	if terminal.sessionToken != "" {
		request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response.Code, append([]byte{}, response.Body.Bytes()...)
}

// kitchenHeaders son las cabeceras de la aplicación de cocina.
var kitchenHeaders = map[string]string{"X-Client-Kind": "kitchen", "X-Client-Version": "0.1.0"}

// displayCall es una petición de la pantalla que debe responder status.
func (fixture *kitchenDisplayFixture) displayCall(t *testing.T, terminal testTerminal, method, route string, body any, status int) []byte {
	t.Helper()
	code, response := fixture.send(terminal, method, route, body, kitchenHeaders)
	if code != status {
		t.Fatalf("%s %s de la pantalla: esperado %d, recibido %d: %s", method, route, status, code, response)
	}
	return response
}

// displayError es una petición de la pantalla que debe fallar con status y code.
func (fixture *kitchenDisplayFixture) displayError(t *testing.T, terminal testTerminal, method, route string, body any, status int,
	code string) []byte {
	t.Helper()
	response := fixture.displayCall(t, terminal, method, route, body, status)
	if got := errorCode(t, response); got != code {
		t.Fatalf("%s %s: esperado %s, recibido %s: %s", method, route, code, got, response)
	}
	return response
}

// orders es GET kitchen-display/orders de la pantalla.
func (fixture *kitchenDisplayFixture) orders(t *testing.T, terminal testTerminal) displayOrdersView {
	t.Helper()
	return decodeResponse[displayOrdersView](t, fixture.displayCall(t, terminal, "GET", "kitchen-display/orders", nil, 200))
}

// card es la tarjeta de la cuenta (nil si no está), y comprueba que no aparezca dos veces.
func card(t *testing.T, view displayOrdersView, accountID string) *kitchen.DisplayCard {
	t.Helper()
	var found *kitchen.DisplayCard
	for index := range view.Orders {
		if view.Orders[index].AccountID == accountID {
			if found != nil {
				t.Fatalf("la cuenta %s aparece en dos tarjetas: %+v", accountID, view.Orders)
			}
			found = &view.Orders[index]
		}
	}
	return found
}

// itemsOf son los platillos de la tarjeta de una estación.
func itemsOf(card *kitchen.DisplayCard, stationID string) []kitchen.DisplayItem {
	found := []kitchen.DisplayItem{}
	for _, item := range card.Items {
		if item.StationID == stationID {
			found = append(found, item)
		}
	}
	return found
}

// refs son las tareas de unos platillos, como las manda la pantalla.
func refs(items ...kitchen.DisplayItem) []kitchen.DisplayTaskRef {
	result := []kitchen.DisplayTaskRef{}
	for _, item := range items {
		result = append(result, item.Tasks...)
	}
	return result
}

// mark manda items/ready u orders/ready.
func (fixture *kitchenDisplayFixture) mark(t *testing.T, terminal testTerminal, action string, tasks []kitchen.DisplayTaskRef) displayMarkResult {
	t.Helper()
	return decodeResponse[displayMarkResult](t, fixture.displayCall(t, terminal, "POST", "kitchen-display/"+action,
		map[string]any{"tasks": tasks}, 200))
}

// displayEvents es lo que el canal de eventos entrega a la pantalla desde el cursor.
func (fixture *kitchenDisplayFixture) displayEvents(t *testing.T, terminal testTerminal, after int64) events.Batch {
	t.Helper()
	return decodeResponse[events.Batch](t, fixture.displayCall(t, terminal, "GET", fmt.Sprintf("events/snapshot?after=%d", after), nil, 200))
}

// hasEvent dice si el lote trae un evento de ese tipo con esa estación o ese agregado (vacío: cualquiera).
func hasEvent(batch events.Batch, kind, stationID, aggregateID string) bool {
	for _, event := range batch.Events {
		if event.Kind != kind {
			continue
		}
		if stationID != "" && (event.StationID == nil || *event.StationID != stationID) {
			continue
		}
		if aggregateID != "" && (event.AggregateID == nil || *event.AggregateID != aggregateID) {
			continue
		}
		return true
	}
	return false
}

// order abre una cuenta en la mesa a nombre del mesero y la envía a cocina.
func (fixture *kitchenDisplayFixture) order(t *testing.T, table int, items ...operations.DraftItem) string {
	t.Helper()
	opened := fixture.open(t, fixture.admin, map[string]any{"table_id": fixture.tables[table].ID, "seen_account_ids": nil,
		"assigned_user_id": fixture.waiterID}, items...)
	fixture.confirm(t, fixture.admin, opened.Account.ID)
	return opened.Account.ID
}

// addAndConfirm agrega partidas al borrador de una cuenta ya enviada y las envía (un agregado después del envío).
func (fixture *kitchenDisplayFixture) addAndConfirm(t *testing.T, accountID string, items ...operations.DraftItem) {
	t.Helper()
	fixture.save(t, fixture.admin, accountID, items...)
	fixture.confirm(t, fixture.admin, accountID)
}

// cancelOne cancela una unidad de la partida del producto en la cuenta.
func (fixture *kitchenDisplayFixture) cancelOne(t *testing.T, accountID, productID string) {
	t.Helper()
	bill := fixture.refresh(t, accountID)
	for _, line := range bill.Lines {
		if line.Request.ProductID == productID && line.Quantity > 0 {
			fixture.mutate(t, "POST", "restaurant/accounts/"+accountID+"/cancel", map[string]any{
				"revision": bill.Account.Revision, "version_id": line.VersionID, "quantity": 1, "reason": "El cliente cambió de opinión",
			}, 200)
			return
		}
	}
	t.Fatalf("la cuenta no tiene el producto %s", productID)
}

// notices cuenta los avisos de platillos listos de la cuenta.
func (fixture *kitchenDisplayFixture) notices(t *testing.T, accountID string) int {
	t.Helper()
	return fixture.count(t, "SELECT count(*)::int FROM app.ready_notices WHERE account_id=$1", accountID)
}

func TestStageK1KitchenDisplay(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &kitchenDisplayFixture{waiterModeFixture: &waiterModeFixture{restaurantFixture: newFixture(t)}}
	fixture.seedCatalog(t)
	fixture.printer = decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name: "Comandas K1", Transport: "escpos_tcp", Destination: "127.0.0.1", Port: 9108, Active: true, PaperWidth: 80, Copies: 1,
	}, 200))
	// Tres estaciones, una por recepción: «Solo pantalla de cocina» (por platillo), «Impresora y pantalla de cocina»
	// (por pedido) y «Solo impresora».
	fixture.cocina = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina K1", Active: true, Reception: "kds", Tracking: "item",
	}, 200))
	fixture.barra = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Barra K1", Active: true, Reception: "both", PrinterID: &fixture.printer.ID, Tracking: "order",
	}, 200))
	fixture.papel = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Postres K1", Active: true, Reception: "printer", PrinterID: &fixture.printer.ID,
	}, 200))
	fixture.pastel = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Pastel K1", Active: true}, CategoryID: fixture.category.ID, Code: "PASTEL-K1",
		Kind: "product", BasePrice: "60",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare", StationIDs: []string{fixture.cocina.ID}}, 200)
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &fixture.fries.ID, Mode: "prepare", StationIDs: []string{fixture.barra.ID}}, 200)
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &fixture.pastel.ID, Mode: "prepare", StationIDs: []string{fixture.papel.ID}}, 200)
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón K1", Active: true}, 200))
	for index := range 10 {
		fixture.tables = append(fixture.tables, decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: fmt.Sprintf("Mesa %d", index+1), Shape: "round", X: 20 + (index%8)*110, Y: 30 + (index/8)*100,
			Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200)))
	}
	inventoryUser(t, fixture.restaurantFixture, "mesero_k1", waiterPermissions)
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username='mesero_k1'").Scan(&fixture.waiterID); err != nil {
		t.Fatal(err)
	}
	fixture.display = fixture.kitchenDevice(t, "Pantalla cocina K1")
	fixture.second = fixture.kitchenDevice(t, "Pantalla barra K1")

	t.Run("emparejar como pantalla de cocina", func(t *testing.T) {
		testStageK1KitchenDisplayPairing(t, fixture)
	})
	t.Run("la pantalla no captura, no cobra ni administra: solo su lista cerrada", func(t *testing.T) {
		testStageK1KitchenDisplayClosedList(t, fixture)
	})
	t.Run("conectar, configuración y filtrado por estación y por recepción; la impresión no cambia", func(t *testing.T) {
		testStageK1KitchenDisplayStations(t, fixture)
	})
	t.Run("tarjetas: mesa, para llevar, mesero, modificadores, notas, NUEVO y CANCELADO", func(t *testing.T) {
		testStageK1KitchenDisplayCards(t, fixture)
	})
	t.Run("marcar platillo y pedido, aviso al mesero, recientes y recuperar", func(t *testing.T) {
		testStageK1KitchenDisplayMarks(t, fixture)
	})
	t.Run("eventos de la pantalla y reconexión con el estado completo", func(t *testing.T) {
		testStageK1KitchenDisplayEvents(t, fixture)
	})
	t.Run("versión instalada y contraseña del administrador para los ajustes", func(t *testing.T) {
		testStageK1KitchenDisplayVersionAndSettings(t, fixture)
	})
	t.Run("cupo de cocina ligado al dispositivo, Liberar cupo y Usar esta pantalla", func(t *testing.T) {
		testStageK1KitchenDisplaySeats(t, fixture)
	})
	t.Run("revocar desconecta al momento", func(t *testing.T) {
		testStageK1KitchenDisplayRevocation(t, fixture)
	})
	t.Run("estados de la licencia y hora del equipo incorrecta", func(t *testing.T) {
		testStageK1KitchenDisplayLicense(t, fixture)
	})
}

func testStageK1KitchenDisplayPairing(t *testing.T, fixture *kitchenDisplayFixture) {
	identity, err := localsetup.EnsureIdentity(filepath.Join(t.TempDir(), localsetup.IdentityFolder))
	if err != nil {
		t.Fatal(err)
	}
	fixture.service.Identity = &administration.ServerIdentity{ServerID: identity.ServerID, Fingerprint: identity.Fingerprint}
	defer func() { fixture.service.Identity = nil }()
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	request := func(kind, clientKind string) (int, []byte) {
		return fixture.send(testTerminal{}, "POST", "pairing/requests", map[string]any{"name": "Tableta de cocina K1",
			"device_kind": kind, "os": "Android 14", "app_version": "0.1.0", "commitment": administration.PairingCommitment(nonce)},
			map[string]string{"X-Client-Kind": clientKind, "X-Client-Version": "0.1.0"})
	}
	// La aplicación de cocina solo se empareja como pantalla, y una pantalla solo desde ella.
	for _, wrong := range [][2]string{{"android", "kitchen"}, {"desktop", "kitchen"}, {"kitchen", "android"}, {"kitchen", "desktop"}} {
		if code, response := request(wrong[0], wrong[1]); code != 400 || errorCode(t, response) != "CLIENT_KIND_MISMATCH" {
			t.Fatalf("emparejar %s con X-Client-Kind %s: %d %s", wrong[0], wrong[1], code, response)
		}
	}
	code, response := request("kitchen", "kitchen")
	if code != 200 {
		t.Fatalf("pedir el código como pantalla: %d %s", code, response)
	}
	created := decodeResponse[map[string]string](t, response)
	secret := map[string]string{"X-Pairing-Secret": created["pairing_secret"], "X-Client-Kind": "kitchen", "X-Client-Version": "0.1.0"}
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
	// En Terminales la solicitud dice que es una pantalla de cocina.
	requests := decodeResponse[struct {
		Requests []map[string]any `json:"requests"`
	}](t, fixture.adminCall(t, "GET", "devices/pairing-requests", nil, 200)).Requests
	if len(requests) == 0 || requests[0]["device_kind"] != "kitchen" {
		t.Fatalf("solicitud en Terminales: %v", requests)
	}
	fixture.adminCall(t, "POST", "devices/pairing-requests/"+created["request_id"]+"/approve", map[string]string{"code": pairingCode}, 200)
	code, response = fixture.send(testTerminal{}, "POST", "pairing/requests/"+created["request_id"]+"/claim", map[string]any{}, secret)
	if code != 200 {
		t.Fatalf("reclamar: %d %s", code, response)
	}
	claimed := decodeResponse[map[string]string](t, response)
	paired := testTerminal{deviceID: claimed["device_id"], deviceToken: claimed["device_token"]}
	if fixture.count(t, "SELECT count(*)::int FROM app.devices WHERE id=$1 AND kind='kitchen' AND app_version='0.1.0'", paired.deviceID) != 1 {
		t.Fatal("la pantalla emparejada no quedó como kitchen")
	}
	// Desde ahí es una pantalla: se conecta y no inicia sesión.
	fixture.displayCall(t, paired, "POST", "kitchen-display/connect", map[string]any{}, 200)
	fixture.displayError(t, paired, "POST", "auth/login", map[string]string{"username": "admin", "password": "admin"}, 403, "KITCHEN_DISPLAY_ONLY")
	// Se revoca para que no ocupe cupo en lo que sigue.
	fixture.adminCall(t, "POST", "devices/"+paired.deviceID+"/revoke", map[string]any{}, 200)
}

func testStageK1KitchenDisplayClosedList(t *testing.T, fixture *kitchenDisplayFixture) {
	accountsBefore := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts")
	deniedBefore := fixture.count(t, `SELECT count(*)::int FROM app.audit_events WHERE action='access.denied' AND device_id=$1
        AND actor_kind='device'`, fixture.display.deviceID)
	// La pantalla manda además el Authorization de la sesión del administrador: nunca se llega a la sesión.
	stolen := fixture.display
	stolen.sessionToken = fixture.admin.sessionToken
	accountID := "00000000-0000-4000-8000-000000000001"
	families := []struct{ method, route string }{
		{"POST", "auth/login"}, {"GET", "auth/me"}, {"POST", "auth/logout"}, {"GET", "auth/pin-users"}, {"POST", "auth/pin-login"},
		{"GET", "restaurant/state"}, {"GET", "restaurant/accounts"}, {"POST", "restaurant/accounts"},
		{"POST", "restaurant/accounts/" + accountID + "/confirm"}, {"POST", "restaurant/offline-orders"},
		{"POST", "payments/accounts/" + accountID + "/payments"}, {"POST", "payments/shifts"},
		{"GET", "kitchen/board"}, {"POST", "kitchen/states"}, {"POST", "kitchen/display-mode"},
		{"GET", "catalog/products"}, {"POST", "catalog/categories"}, {"GET", "users"}, {"POST", "roles"},
		{"GET", "devices"}, {"POST", "devices/" + fixture.second.deviceID + "/revoke"}, {"GET", "settings/features"},
		{"GET", "maintenance/backups"}, {"POST", "maintenance/test-data/remove"}, {"POST", "license/activate"},
		{"GET", "reports/sales"}, {"PUT", "print-agent/printers"}, {"GET", "ready-alerts"}, {"GET", "inventory/items"},
		{"GET", "audit"}, {"GET", "documents/templates"}, {"PUT", "preferences"}, {"GET", "permissions/catalog"},
	}
	for _, family := range families {
		code, response := fixture.send(stolen, family.method, family.route, map[string]any{}, kitchenHeaders)
		if code != 403 || errorCode(t, response) != "KITCHEN_DISPLAY_ONLY" {
			t.Fatalf("%s %s respondió a la pantalla: %d %s", family.method, family.route, code, response)
		}
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts") != accountsBefore {
		t.Fatal("la pantalla abrió una cuenta")
	}
	denied := fixture.count(t, `SELECT count(*)::int FROM app.audit_events WHERE action='access.denied' AND device_id=$1
        AND actor_kind='device' AND actor_id IS NULL`, fixture.display.deviceID)
	if denied != deniedBefore+len(families) {
		t.Fatalf("auditoría de los rechazos: %d de %d", denied-deniedBefore, len(families))
	}
	// Sí responden el estado de la licencia del dispositivo y el canal de eventos.
	status := decodeResponse[map[string]any](t, fixture.displayCall(t, stolen, "GET", "license/status", nil, 200))
	if _, found := status["state"]; !found {
		t.Fatalf("estado de la licencia: %v", status)
	}
	fixture.displayCall(t, fixture.display, "GET", "events/snapshot?after=0", nil, 200)
	// El canal de eventos solo admite el cursor.
	fixture.displayError(t, fixture.display, "GET", "events/snapshot?after=0&station_id="+fixture.cocina.ID, nil, 400, "VALIDATION_ERROR")
	// Las rutas de la pantalla no responden a una terminal de escritorio (aunque tenga sesión y todos los permisos).
	code, response := fixture.send(fixture.admin, "GET", "kitchen-display/orders", nil, map[string]string{"X-Client-Kind": "desktop"})
	if code != 403 || errorCode(t, response) != "KITCHEN_DISPLAY_REQUIRED" {
		t.Fatalf("escritorio en la ruta de la pantalla: %d %s", code, response)
	}
	if fixture.count(t, `SELECT count(*)::int FROM app.audit_events WHERE action='access.denied' AND device_id=$1
        AND after_state->>'code'='KITCHEN_DISPLAY_REQUIRED'`, fixture.admin.deviceID) != 1 {
		t.Fatal("auditoría de KITCHEN_DISPLAY_REQUIRED")
	}
	// El tipo de cliente debe ser el de la pantalla.
	for _, kind := range []string{"desktop", "android"} {
		code, response := fixture.send(fixture.display, "GET", "kitchen-display/orders", nil, map[string]string{"X-Client-Kind": kind})
		if code != 400 || errorCode(t, response) != "CLIENT_KIND_MISMATCH" {
			t.Fatalf("pantalla con X-Client-Kind %s: %d %s", kind, code, response)
		}
	}
	// La aplicación de escritorio tampoco puede usar una credencial de otro tipo con X-Client-Kind kitchen.
	code, response = fixture.send(fixture.admin, "GET", "auth/me", nil, map[string]string{"X-Client-Kind": "kitchen"})
	if code != 400 || errorCode(t, response) != "CLIENT_KIND_MISMATCH" {
		t.Fatalf("escritorio con X-Client-Kind kitchen: %d %s", code, response)
	}
}

func testStageK1KitchenDisplayStations(t *testing.T, fixture *kitchenDisplayFixture) {
	// Antes de conectarse no muestra pedidos.
	fixture.displayError(t, fixture.display, "GET", "kitchen-display/orders", nil, 409, "KITCHEN_DISPLAY_NOT_CONNECTED")
	connected := decodeResponse[struct {
		Display kitchen.DisplayConfig `json:"display"`
		Seat    struct {
			Held bool `json:"held"`
		} `json:"seat"`
		ServerTime    time.Time `json:"server_time"`
		Establishment struct {
			Name     string `json:"name"`
			Timezone string `json:"timezone"`
		} `json:"establishment"`
	}](t, fixture.displayCall(t, fixture.display, "POST", "kitchen-display/connect", map[string]any{}, 200))
	config := connected.Display
	if !connected.Seat.Held || connected.ServerTime.IsZero() || connected.Establishment.Timezone != "America/Monterrey" ||
		config.TextSize != "normal" || config.Theme != "dark" || !config.SoundEnabled || config.SoundVolume != "medium" ||
		config.WarningMinutes != 10 || config.LateMinutes != 20 || !config.AllStations || config.Name != "Pantalla cocina K1" {
		t.Fatalf("configuración al conectar: %+v", connected)
	}
	// Solo las estaciones que reciben en pantalla, con su seguimiento efectivo, en el orden de Cocina (por nombre: las
	// tres tienen el mismo orden).
	if len(config.Stations) != 2 || config.Stations[0].ID != fixture.barra.ID || config.Stations[0].Tracking != "order" ||
		config.Stations[1].ID != fixture.cocina.ID || config.Stations[1].Tracking != "item" {
		t.Fatalf("estaciones de la pantalla: %+v", config.Stations)
	}
	// Repetir connect deja lo mismo; la pantalla ocupa un cupo de cocina y Terminales lo dice.
	fixture.displayCall(t, fixture.display, "POST", "kitchen-display/connect", nil, 200)
	if fixture.count(t, `SELECT count(*)::int FROM app.license_seats WHERE device_id=$1 AND kind='kitchen_display'
        AND session_id IS NULL`, fixture.display.deviceID) != 1 {
		t.Fatal("cupo de cocina de la pantalla")
	}
	devices := decodeResponse[[]map[string]any](t, fixture.adminCall(t, "GET", "devices", nil, 200))
	for _, device := range devices {
		if device["id"] == fixture.display.deviceID {
			seats, _ := device["seats"].([]any)
			settings, _ := device["kitchen_display"].(map[string]any)
			if len(seats) != 1 || seats[0] != "kitchen_displays" || device["kind"] != "kitchen" || settings["theme"] != "dark" ||
				device["online"] != true {
				t.Fatalf("la pantalla en Terminales: %v", device)
			}
		}
	}
	// Un pedido con un platillo de cada estación: la pantalla muestra Cocina y Barra, nunca Postres (solo impresora).
	cursor := fixture.orders(t, fixture.display).Cursor
	accountID := fixture.order(t, 0, draftLine(fixture.product, 1), draftLine(fixture.fries, 1), draftLine(fixture.pastel, 1))
	view := fixture.orders(t, fixture.display)
	found := card(t, view, accountID)
	if found == nil || len(itemsOf(found, fixture.cocina.ID)) != 1 || len(itemsOf(found, fixture.barra.ID)) != 1 ||
		len(itemsOf(found, fixture.papel.ID)) != 0 || len(found.Items) != 2 {
		t.Fatalf("tarjeta filtrada por recepción: %+v", found)
	}
	// La impresión de hoy no cambia: Barra (ambas) y Postres (impresora) imprimen su comanda; Cocina (pantalla) no.
	for station, expected := range map[string]int{fixture.cocina.ID: 0, fixture.barra.ID: 1, fixture.papel.ID: 1} {
		jobs := fixture.count(t, `SELECT count(*)::int FROM app.kitchen_print_jobs j JOIN app.kitchen_tickets t ON t.id=j.ticket_id
            JOIN app.kitchen_releases r ON r.id=t.release_id WHERE r.account_id=$1 AND $2::uuid=ANY(t.station_ids)`, accountID, station)
		if jobs != expected {
			t.Fatalf("trabajos de impresión de la estación %s: %d, esperados %d", station, jobs, expected)
		}
	}
	// Eventos de pedido nuevo de las estaciones de la pantalla, no de Postres.
	batch := fixture.displayEvents(t, fixture.display, cursor)
	if !hasEvent(batch, "kitchen.ticket.created", fixture.cocina.ID, "") || !hasEvent(batch, "kitchen.ticket.created", fixture.barra.ID, "") ||
		hasEvent(batch, "kitchen.ticket.created", fixture.papel.ID, "") || hasEvent(batch, "kitchen.released", "", "") {
		t.Fatalf("eventos del pedido nuevo: %+v", batch.Events)
	}
	// Acuse de recepción: la pantalla confirma las comandas de Cocina y Barra; Comandas deja de decir «pendiente».
	if len(view.ReceiptsPending) < 2 {
		t.Fatalf("comandas por confirmar: %+v", view.ReceiptsPending)
	}
	for _, receipt := range view.ReceiptsPending {
		if receipt.StationID == fixture.papel.ID {
			t.Fatalf("la comanda de una estación de impresora no se confirma en pantalla: %+v", receipt)
		}
	}
	acknowledged := decodeResponse[map[string]int](t, fixture.displayCall(t, fixture.display, "POST", "kitchen-display/receipts",
		map[string]any{"receipts": view.ReceiptsPending}, 200))
	if acknowledged["acknowledged"] != len(view.ReceiptsPending) {
		t.Fatalf("acuses: %v", acknowledged)
	}
	again := decodeResponse[map[string]int](t, fixture.displayCall(t, fixture.display, "POST", "kitchen-display/receipts",
		map[string]any{"receipts": view.ReceiptsPending}, 200))
	if again["acknowledged"] != 0 || len(fixture.orders(t, fixture.display).ReceiptsPending) != 0 {
		t.Fatalf("el acuse se repitió: %v", again)
	}
	if fixture.count(t, `SELECT count(*)::int FROM app.kitchen_kds_receipts WHERE device_id=$1 AND user_id IS NULL`,
		fixture.display.deviceID) != len(view.ReceiptsPending) {
		t.Fatal("acuses sin usuario de la pantalla")
	}
	tickets := decodeResponse[[]map[string]any](t, fixture.adminCall(t, "GET", "kitchen/tickets?station_id="+fixture.cocina.ID, nil, 200))
	for _, ticket := range tickets {
		if ticket["pending_kds"] == true {
			t.Fatalf("Comandas sigue diciendo recepción pendiente: %v", ticket)
		}
	}
	// Una comanda de Postres no se confirma en pantalla: no es de sus estaciones.
	var papelTicket string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT t.id::text FROM app.kitchen_tickets t JOIN app.kitchen_releases r
        ON r.id=t.release_id WHERE r.account_id=$1 AND $2::uuid=ANY(t.station_ids)`, accountID, fixture.papel.ID).Scan(&papelTicket); err != nil {
		t.Fatal(err)
	}
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/receipts", map[string]any{"receipts": []map[string]string{
		{"ticket_id": papelTicket, "station_id": fixture.papel.ID}}}, 403, "STATION_SCOPE_DENIED")
	// Las estaciones de la pantalla se eligen con PUT kitchen/access, solo entre las que reciben en pantalla.
	access := func(body map[string]any, status int) []byte {
		body["device_id"] = fixture.display.deviceID
		return fixture.mutate(t, "PUT", "kitchen/access", body, status)
	}
	access(map[string]any{"all_stations": false, "station_ids": []string{fixture.papel.ID}, "expected_station_ids": []string{},
		"expected_all_stations": true}, 400)
	cursor = fixture.orders(t, fixture.display).Cursor
	access(map[string]any{"all_stations": false, "station_ids": []string{fixture.barra.ID}, "expected_station_ids": []string{},
		"expected_all_stations": true}, 200)
	if !hasEvent(fixture.displayEvents(t, fixture.display, cursor), kitchen.DisplayConfigurationChangedKind, "", "") {
		t.Fatal("el cambio de estaciones no avisó a la pantalla")
	}
	only := fixture.orders(t, fixture.display)
	if len(only.Display.Stations) != 1 || only.Display.Stations[0].ID != fixture.barra.ID || only.Display.AllStations {
		t.Fatalf("pantalla solo con Barra: %+v", only.Display)
	}
	if found := card(t, only, accountID); found == nil || len(found.Items) != 1 || found.Items[0].StationID != fixture.barra.ID {
		t.Fatalf("tarjeta solo con Barra: %+v", found)
	}
	access(map[string]any{"all_stations": true, "station_ids": []string{}, "expected_station_ids": []string{fixture.barra.ID},
		"expected_all_stations": false}, 200)
	// Dejar el pedido listo para las siguientes pruebas.
	view = fixture.orders(t, fixture.display)
	fixture.mark(t, fixture.display, "orders/ready", refs(card(t, view, accountID).Items...))
	if card(t, fixture.orders(t, fixture.display), accountID) != nil {
		t.Fatal("la tarjeta marcada sigue en pantalla")
	}
}

func testStageK1KitchenDisplayCards(t *testing.T, fixture *kitchenDisplayFixture) {
	// Una mesa con un platillo modificado (sin cebolla, variante y nota) para el mesero.
	filete := draftLine(fixture.product, 2)
	filete.VariantID = &fixture.product.Variants[0].ID
	filete.Customization.Ingredients = []catalog.IngredientSelection{{IngredientID: fixture.product.Ingredients[0].IngredientID, Quantity: "0"}}
	filete.Customization.Notes = "Bien dorado"
	accountID := fixture.order(t, 1, filete)
	view := fixture.orders(t, fixture.display)
	found := card(t, view, accountID)
	if found == nil || found.Title != "Mesa 2" || found.Subtitle != "" || found.Mode != "dine_in" || found.WaiterName != "Prueba mesero_k1" ||
		found.Status != "open" || found.Folio < 1 || found.ElapsedSeconds < 0 || found.SentAt.IsZero() {
		t.Fatalf("encabezado de la mesa: %+v", found)
	}
	items := itemsOf(found, fixture.cocina.ID)
	if len(items) != 1 || items[0].Quantity != 2 || items[0].Name != "Filete" || items[0].Status != "pending" || items[0].IsNew ||
		items[0].Notes != "Bien dorado" || items[0].Tracking != "item" || len(items[0].Tasks) != 2 || len(items[0].Parts) != 1 {
		t.Fatalf("platillo de la mesa: %+v", items)
	}
	part := items[0].Parts[0]
	lines := []string{}
	for _, line := range part.Lines {
		lines = append(lines, line.Text)
	}
	if !part.ShowName || part.Name != "Filete · Grande" || !slices.Contains(lines, "SIN Cebolla") {
		t.Fatalf("modificadores del platillo: %+v", part)
	}
	if bytes.Contains(fixture.displayCall(t, fixture.display, "GET", "kitchen-display/orders", nil, 200), []byte(`"price"`)) {
		t.Fatal("la pantalla no recibe precios")
	}
	// Agregado después del envío: «NUEVO» en la misma tarjeta.
	fixture.addAndConfirm(t, accountID, draftLine(fixture.fries, 1))
	found = card(t, fixture.orders(t, fixture.display), accountID)
	added := itemsOf(found, fixture.barra.ID)
	if len(added) != 1 || !added[0].IsNew || len(itemsOf(found, fixture.cocina.ID)) != 1 || itemsOf(found, fixture.cocina.ID)[0].IsNew {
		t.Fatalf("agregado después del envío: %+v", found)
	}
	// Cancelar una unidad: tachada como CANCELADO, sin desaparecer.
	fixture.cancelOne(t, accountID, fixture.product.ID)
	found = card(t, fixture.orders(t, fixture.display), accountID)
	statuses := map[string]int{}
	for _, item := range itemsOf(found, fixture.cocina.ID) {
		statuses[item.Status] += item.Quantity
	}
	if statuses["pending"] != 1 || statuses["cancelled"] != 1 || found.Status != "open" {
		t.Fatalf("cancelación dentro de la tarjeta: %+v", found)
	}
	// Para llevar con el nombre del cliente.
	takeaway := fixture.open(t, fixture.admin, map[string]any{"mode": "takeaway", "customer_name": "Lucía", "seen_account_ids": nil,
		"assigned_user_id": fixture.waiterID}, draftLine(fixture.product, 1))
	fixture.confirm(t, fixture.admin, takeaway.Account.ID)
	carry := card(t, fixture.orders(t, fixture.display), takeaway.Account.ID)
	if carry == nil || carry.Title != "Para llevar" || carry.Subtitle != "Lucía" || carry.Mode != "takeaway" || carry.CustomerName != "Lucía" {
		t.Fatalf("encabezado de para llevar: %+v", carry)
	}
	// Del más antiguo al más nuevo.
	view = fixture.orders(t, fixture.display)
	for index := 1; index < len(view.Orders); index++ {
		if view.Orders[index].SentAt.Before(view.Orders[index-1].SentAt) {
			t.Fatalf("orden de las tarjetas: %+v", view.Orders)
		}
	}
	// Una tarjeta cerrada por una cancelación se queda tachada hasta que alguien marca el pedido.
	alone := fixture.order(t, 2, draftLine(fixture.product, 1))
	fixture.cancelOne(t, alone, fixture.product.ID)
	cancelled := card(t, fixture.orders(t, fixture.display), alone)
	if cancelled == nil || cancelled.Status != "cancelled" || len(cancelled.Items) != 1 || cancelled.Items[0].Status != "cancelled" {
		t.Fatalf("tarjeta cerrada por la cancelación: %+v", cancelled)
	}
	// La otra pantalla también la ve; al marcarla en una, desaparece de las dos.
	fixture.displayCall(t, fixture.second, "POST", "kitchen-display/connect", map[string]any{}, 200)
	if card(t, fixture.orders(t, fixture.second), alone) == nil {
		t.Fatal("la segunda pantalla no ve la cancelación")
	}
	result := fixture.mark(t, fixture.display, "orders/ready", refs(cancelled.Items...))
	if result.Dismissed != 1 || result.Updated != 0 {
		t.Fatalf("marcar la tarjeta cancelada: %+v", result)
	}
	if card(t, fixture.orders(t, fixture.display), alone) != nil || card(t, fixture.orders(t, fixture.second), alone) != nil {
		t.Fatal("la tarjeta cancelada sigue en pantalla después de marcarla")
	}
	// Una cancelación de hace más de 30 minutos ya no deja la tarjeta.
	old := fixture.order(t, 3, draftLine(fixture.product, 1))
	fixture.cancelOne(t, old, fixture.product.ID)
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.kitchen_tasks t SET updated_at=now()-interval '31 minutes'
        FROM app.restaurant_item_units u WHERE u.id=t.unit_id AND u.account_id=$1`, old); err != nil {
		t.Fatal(err)
	}
	if card(t, fixture.orders(t, fixture.display), old) != nil {
		t.Fatal("una cancelación vieja sigue en pantalla")
	}
	// Dejar las tarjetas abiertas listas.
	for _, id := range []string{accountID, takeaway.Account.ID} {
		fixture.mark(t, fixture.display, "orders/ready", refs(card(t, fixture.orders(t, fixture.display), id).Items...))
	}
}

func testStageK1KitchenDisplayMarks(t *testing.T, fixture *kitchenDisplayFixture) {
	accountID := fixture.order(t, 4, draftLine(fixture.product, 2), draftLine(fixture.fries, 1))
	found := card(t, fixture.orders(t, fixture.display), accountID)
	cocina := itemsOf(found, fixture.cocina.ID)
	barra := itemsOf(found, fixture.barra.ID)
	if len(cocina) != 1 || len(barra) != 1 || barra[0].Tracking != "order" {
		t.Fatalf("tarjeta para marcar: %+v", found)
	}
	// En una estación «por pedido» no se marca un platillo suelto.
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/items/ready", map[string]any{"tasks": barra[0].Tasks}, 400, "KITCHEN_VALIDATION")
	// Una tarea de otra estación (o inexistente) no se marca, y no se marca nada del lote.
	var papelTask string
	papelAccount := fixture.order(t, 5, draftLine(fixture.pastel, 1))
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT t.id::text FROM app.kitchen_tasks t JOIN app.restaurant_item_units u
        ON u.id=t.unit_id WHERE u.account_id=$1`, papelAccount).Scan(&papelTask); err != nil {
		t.Fatal(err)
	}
	mixed := append(append([]kitchen.DisplayTaskRef{}, cocina[0].Tasks...), kitchen.DisplayTaskRef{ID: papelTask, VersionID: cocina[0].Tasks[0].VersionID})
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/items/ready", map[string]any{"tasks": mixed}, 403, "STATION_SCOPE_DENIED")
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/items/ready", map[string]any{"tasks": []kitchen.DisplayTaskRef{
		{ID: "00000000-0000-4000-8000-0000000000ff", VersionID: cocina[0].Tasks[0].VersionID}}}, 404, "NOT_FOUND")
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/items/ready", map[string]any{"tasks": []kitchen.DisplayTaskRef{}}, 400, "KITCHEN_VALIDATION")
	if fixture.count(t, `SELECT count(*)::int FROM app.kitchen_tasks t JOIN app.restaurant_item_units u ON u.id=t.unit_id
        WHERE u.account_id=$1 AND t.state='ready'`, accountID) != 0 {
		t.Fatal("un lote rechazado marcó algo")
	}
	// Una versión distinta no se marca y se informa.
	stale := []kitchen.DisplayTaskRef{{ID: cocina[0].Tasks[0].ID, VersionID: barra[0].Tasks[0].VersionID}}
	if result := fixture.mark(t, fixture.display, "items/ready", stale); result.Updated != 0 || len(result.Skipped) != 1 ||
		result.Skipped[0].Reason != "changed" {
		t.Fatalf("versión distinta: %+v", result)
	}
	// Marcar el platillo: el aviso al mesero de la A2, uno por unidad, como hoy.
	if fixture.notices(t, accountID) != 0 {
		t.Fatal("avisos antes de marcar")
	}
	if result := fixture.mark(t, fixture.display, "items/ready", cocina[0].Tasks); result.Updated != 2 || len(result.Skipped) != 0 {
		t.Fatalf("marcar el platillo: %+v", result)
	}
	if fixture.notices(t, accountID) != 2 || fixture.count(t, `SELECT count(*)::int FROM app.ready_notices WHERE account_id=$1
        AND recipient_user_id=$2`, accountID, fixture.waiterID) != 2 {
		t.Fatal("el aviso al mesero al marcar desde la pantalla")
	}
	firstSequence := fixture.count(t, "SELECT max(sequence)::int FROM app.ready_notices WHERE account_id=$1", accountID)
	// Repetir la marca (el reintento silencioso) no cambia nada ni vuelve a avisar.
	if result := fixture.mark(t, fixture.display, "items/ready", cocina[0].Tasks); result.Updated != 0 || len(result.Skipped) != 2 ||
		result.Skipped[0].Reason != "already_ready" {
		t.Fatalf("marca repetida: %+v", result)
	}
	if fixture.notices(t, accountID) != 2 {
		t.Fatal("la marca repetida volvió a avisar")
	}
	found = card(t, fixture.orders(t, fixture.display), accountID)
	if items := itemsOf(found, fixture.cocina.ID); len(items) != 1 || items[0].Status != "ready" || items[0].ReadyAt == nil {
		t.Fatalf("platillo listo atenuado: %+v", items)
	}
	// El pedido completo: en Barra («por pedido») se marca el pedido de la estación; la tarjeta se cierra sola.
	if result := fixture.mark(t, fixture.display, "orders/ready", refs(found.Items...)); result.Updated != 1 {
		t.Fatalf("marcar el pedido: %+v", result)
	}
	if card(t, fixture.orders(t, fixture.display), accountID) != nil || fixture.notices(t, accountID) != 3 {
		t.Fatal("el pedido completo")
	}
	// Recientes: lo marcado, del más reciente al más antiguo.
	recent := decodeResponse[kitchen.DisplayRecent](t, fixture.displayCall(t, fixture.display, "GET", "kitchen-display/recent", nil, 200))
	var entry *kitchen.DisplayRecentOrder
	for index := range recent.Orders {
		if recent.Orders[index].AccountID == accountID {
			entry = &recent.Orders[index]
		}
	}
	if recent.WindowMinutes != 30 || entry == nil || len(entry.Items) != 2 || entry.Title != "Mesa 5" {
		t.Fatalf("recientes: %+v", recent)
	}
	var recentFilete kitchen.DisplayItem
	for _, item := range entry.Items {
		if item.StationID == fixture.cocina.ID {
			recentFilete = item
		}
	}
	// Recuperar no avisa y borra los avisos que todavía no llegaban; la tarjeta vuelve a la pantalla.
	if result := decodeResponse[displayMarkResult](t, fixture.displayCall(t, fixture.display, "POST", "kitchen-display/recover",
		map[string]any{"tasks": recentFilete.Tasks}, 200)); result.Recovered != 2 {
		t.Fatalf("recuperar: %+v", result)
	}
	if fixture.count(t, `SELECT count(*)::int FROM app.ready_notices n JOIN app.restaurant_item_units u ON u.id=n.unit_id
        JOIN app.kitchen_tasks t ON t.unit_id=u.id WHERE t.station_id=$1 AND u.account_id=$2`, fixture.cocina.ID, accountID) != 0 {
		t.Fatal("recuperar dejó los avisos")
	}
	back := card(t, fixture.orders(t, fixture.display), accountID)
	if items := itemsOf(back, fixture.cocina.ID); back == nil || len(items) != 1 || items[0].Status != "pending" || items[0].Preparing {
		t.Fatalf("la tarjeta recuperada: %+v", back)
	}
	// Marcada directamente: vuelve a «Por preparar».
	if fixture.count(t, `SELECT count(*)::int FROM app.kitchen_tasks WHERE id=ANY($1::uuid[]) AND state='released' AND started_at IS NULL
        AND ready_at IS NULL`, []string{recentFilete.Tasks[0].ID, recentFilete.Tasks[1].ID}) != 2 {
		t.Fatal("estado anterior de la tarea recuperada")
	}
	// Recuperar otra vez es idempotente; volver a marcar sí avisa (otro aviso).
	if result := decodeResponse[displayMarkResult](t, fixture.displayCall(t, fixture.display, "POST", "kitchen-display/recover",
		map[string]any{"tasks": recentFilete.Tasks}, 200)); result.Recovered != 0 || result.Skipped[0].Reason != "not_ready" {
		t.Fatalf("recuperar dos veces: %+v", result)
	}
	fixture.mark(t, fixture.display, "items/ready", recentFilete.Tasks)
	if fixture.notices(t, accountID) != 3 || fixture.count(t, "SELECT max(sequence)::int FROM app.ready_notices WHERE account_id=$1", accountID) <= firstSequence {
		t.Fatal("volver a marcar no avisó otra vez")
	}
	// Lo que ya estaba «En preparación» vuelve a «En preparación».
	fixture.mutate(t, "POST", "kitchen/states", map[string]any{"state": "preparing", "tasks": []map[string]any{}}, 400)
	preparing := fixture.order(t, 6, draftLine(fixture.product, 1))
	task := itemsOf(card(t, fixture.orders(t, fixture.display), preparing), fixture.cocina.ID)[0].Tasks
	var revision int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT revision FROM app.kitchen_tasks WHERE id=$1", task[0].ID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	fixture.mutate(t, "POST", "kitchen/states", map[string]any{"state": "preparing", "tasks": []map[string]any{{"id": task[0].ID, "revision": revision}}}, 200)
	if items := itemsOf(card(t, fixture.orders(t, fixture.display), preparing), fixture.cocina.ID); !items[0].Preparing {
		t.Fatalf("en preparación desde el escritorio: %+v", items)
	}
	fixture.mark(t, fixture.display, "items/ready", task)
	fixture.displayCall(t, fixture.display, "POST", "kitchen-display/recover", map[string]any{"tasks": task}, 200)
	if fixture.count(t, "SELECT count(*)::int FROM app.kitchen_tasks WHERE id=$1 AND state='preparing' AND started_at IS NOT NULL", task[0].ID) != 1 {
		t.Fatal("la tarea que ya estaba en preparación")
	}
	// Después de 30 minutos ya no se recupera.
	fixture.mark(t, fixture.display, "items/ready", task)
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_tasks SET ready_at=now()-interval '31 minutes' WHERE id=$1", task[0].ID); err != nil {
		t.Fatal(err)
	}
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/recover", map[string]any{"tasks": task}, 409, "KITCHEN_NOT_RECOVERABLE")
	// El historial y la auditoría firman la pantalla, sin usuario.
	if fixture.count(t, `SELECT count(*)::int FROM app.kitchen_history WHERE device_id=$1 AND actor_id IS NULL AND operation_id IS NULL
        AND kind IN ('states.changed','states.recovered')`, fixture.display.deviceID) < 4 {
		t.Fatal("historial de la pantalla")
	}
}

func testStageK1KitchenDisplayEvents(t *testing.T, fixture *kitchenDisplayFixture) {
	view := fixture.orders(t, fixture.display)
	cursor := view.Cursor
	accountID := fixture.order(t, 7, draftLine(fixture.product, 1))
	// Mientras la pantalla está «desconectada» pasan: un agregado, una cancelación, una marca desde la otra pantalla,
	// un cambio de mesa y un cambio de configuración de cocina.
	fixture.addAndConfirm(t, accountID, draftLine(fixture.product, 1), draftLine(fixture.fries, 1))
	fixture.cancelOne(t, accountID, fixture.product.ID)
	other := card(t, fixture.orders(t, fixture.second), accountID)
	fixture.mark(t, fixture.second, "orders/ready", refs(itemsOf(other, fixture.barra.ID)...))
	bill := fixture.refresh(t, accountID)
	fixture.mutate(t, "POST", "restaurant/accounts/"+accountID+"/move", map[string]any{"revision": bill.Account.Revision,
		"table_id": fixture.tables[8].ID}, 200)
	// Una cuenta sin nada en las estaciones de la pantalla (solo un borrador) no le manda nada.
	draftOnly := fixture.open(t, fixture.admin, map[string]any{"table_id": fixture.tables[9].ID, "seen_account_ids": nil}, draftLine(fixture.product, 1))
	draftBill := fixture.refresh(t, draftOnly.Account.ID)
	fixture.mutate(t, "POST", "restaurant/accounts/"+draftOnly.Account.ID+"/move", map[string]any{"revision": draftBill.Account.Revision,
		"table_id": fixture.tables[0].ID}, 200)
	fixture.adminCall(t, "PATCH", "devices/"+fixture.second.deviceID, map[string]any{"name": "Pantalla barra K1", "keyboard_mode": "auto",
		"kitchen_display_theme": "light"}, 200)
	batch := fixture.displayEvents(t, fixture.display, cursor)
	for _, wanted := range []struct{ kind, station, aggregate string }{
		{"kitchen.ticket.created", fixture.cocina.ID, ""},
		{"kitchen.ticket.created", fixture.barra.ID, ""},
		{"kitchen.state.changed", fixture.barra.ID, ""},
		{"restaurant.account.move", "", accountID},
	} {
		if !hasEvent(batch, wanted.kind, wanted.station, wanted.aggregate) {
			t.Fatalf("falta %s en los eventos de la pantalla: %+v", wanted.kind, batch.Events)
		}
	}
	if hasEvent(batch, "restaurant.account.move", "", draftOnly.Account.ID) || hasEvent(batch, "restaurant.draft.saved", "", "") ||
		hasEvent(batch, kitchen.DisplayConfigurationChangedKind, "", fixture.second.deviceID) ||
		hasEvent(batch, kitchen.PreparationChangedKind, "", "") {
		t.Fatalf("la pantalla recibió eventos que no son suyos: %+v", batch.Events)
	}
	for _, event := range batch.Events {
		if event.Scope != "kitchen" && event.Scope != "restaurant" && event.Scope != "configuration" {
			t.Fatalf("ámbito de un evento de la pantalla: %+v", event)
		}
	}
	// La otra pantalla sí recibe el cambio de su configuración, con ella como agregado.
	if !hasEvent(fixture.displayEvents(t, fixture.second, cursor), kitchen.DisplayConfigurationChangedKind, "", fixture.second.deviceID) {
		t.Fatal("el cambio de la pantalla no le llegó")
	}
	// Los límites de color generales llegan a todas (sin agregado) y salen en su configuración.
	timing := decodeResponse[kitchen.SettingsView](t, fixture.adminCall(t, "GET", "kitchen/settings", nil, 200))
	if timing.DisplayWarningMinutes != 10 || timing.DisplayLateMinutes != 20 {
		t.Fatalf("límites de color en kitchen/settings: %+v", timing)
	}
	limitsCursor := fixture.orders(t, fixture.display).Cursor
	fixture.mutate(t, "PUT", "kitchen/timing", map[string]any{"revision": timing.Revision, "automatic_release": timing.AutomaticRelease,
		"lead_minutes": timing.LeadMinutes, "tracking": timing.Tracking, "display_warning_minutes": 8, "display_late_minutes": 8}, 400)
	fixture.mutate(t, "PUT", "kitchen/timing", map[string]any{"revision": timing.Revision, "automatic_release": timing.AutomaticRelease,
		"lead_minutes": timing.LeadMinutes, "tracking": timing.Tracking, "display_warning_minutes": 8, "display_late_minutes": 15}, 200)
	for _, terminal := range []testTerminal{fixture.display, fixture.second} {
		configuration := fixture.displayEvents(t, terminal, limitsCursor)
		found := false
		for _, event := range configuration.Events {
			found = found || (event.Kind == kitchen.DisplayConfigurationChangedKind && event.AggregateID == nil)
		}
		if !found {
			t.Fatalf("los límites de color no llegaron a todas las pantallas: %+v", configuration.Events)
		}
	}
	if limits := fixture.orders(t, fixture.display).Display; limits.WarningMinutes != 8 || limits.LateMinutes != 15 {
		t.Fatalf("límites de color en la pantalla: %+v", limits)
	}
	// El escritorio sigue guardando la configuración de cocina con lo que leyó (con los límites incluidos).
	saved := decodeResponse[kitchen.SettingsView](t, fixture.adminCall(t, "GET", "kitchen/settings", nil, 200))
	body := map[string]any{}
	encoded, _ := json.Marshal(saved)
	_ = json.Unmarshal(encoded, &body)
	fixture.mutate(t, "PUT", "kitchen/settings", body, 200)
	// license.changed llega a la pantalla.
	licenseCursor := fixture.orders(t, fixture.display).Cursor
	if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.realtime_events(id,kind,scope) VALUES ($1,'license.changed','configuration')`,
		events.NewID()); err != nil {
		t.Fatal(err)
	}
	if !hasEvent(fixture.displayEvents(t, fixture.display, licenseCursor), events.LicenseChangedKind, "", "") {
		t.Fatal("license.changed no llegó a la pantalla")
	}
	// Reconexión: el estado completo trae la tarjeta una sola vez, con lo agregado, lo cancelado y la marca de la otra
	// pantalla; desde su cursor no queda nada pendiente que no esté en el estado.
	recovered := fixture.orders(t, fixture.display)
	found := card(t, recovered, accountID)
	if found == nil || found.Title != "Mesa 9" {
		t.Fatalf("tarjeta después de reconectar: %+v", found)
	}
	statuses := map[string]int{}
	for _, item := range found.Items {
		statuses[item.StationID+"|"+item.Status] += item.Quantity
	}
	if statuses[fixture.cocina.ID+"|pending"] != 1 || statuses[fixture.cocina.ID+"|cancelled"] != 1 ||
		statuses[fixture.barra.ID+"|ready"] != 1 {
		t.Fatalf("estado completo de la tarjeta: %v", statuses)
	}
	if tail := fixture.displayEvents(t, fixture.display, recovered.Cursor); len(tail.Events) != 0 || tail.Cursor < recovered.Cursor {
		t.Fatalf("eventos después del cursor del estado completo: %+v", tail)
	}
	// Un cursor fuera de la ventana pide volver a leer todo.
	if reset := fixture.displayEvents(t, fixture.display, recovered.Cursor+100000); !reset.Reset {
		t.Fatalf("cursor fuera de la ventana: %+v", reset)
	}
	fixture.mark(t, fixture.display, "orders/ready", refs(found.Items...))
}

func testStageK1KitchenDisplayVersionAndSettings(t *testing.T, fixture *kitchenDisplayFixture) {
	version := func(deviceID string) string {
		var value string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(app_version,'') FROM app.devices WHERE id=$1", deviceID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	// La pantalla actualizada encima: Terminales muestra la versión instalada sin emparejar otra vez.
	fixture.send(fixture.display, "GET", "kitchen-display/orders", nil, map[string]string{"X-Client-Kind": "kitchen", "X-Client-Version": "0.1.3"})
	if version(fixture.display.deviceID) != "0.1.3" {
		t.Fatalf("versión de la pantalla: %s", version(fixture.display.deviceID))
	}
	// También en el canal de eventos y en los otros dos tipos de aplicación; una versión inválida no se guarda.
	fixture.send(fixture.display, "GET", "events/snapshot?after=0", nil, map[string]string{"X-Client-Kind": "kitchen", "X-Client-Version": "0.1.4"})
	fixture.send(fixture.display, "GET", "kitchen-display/orders", nil, map[string]string{"X-Client-Kind": "kitchen", "X-Client-Version": "versión"})
	if version(fixture.display.deviceID) != "0.1.4" {
		t.Fatalf("versión de la pantalla por el canal: %s", version(fixture.display.deviceID))
	}
	tablet := (&readyFixture{waiterModeFixture: fixture.waiterModeFixture}).tabletWithVersion(t, "Tableta K1", "0.4.0")
	fixture.send(tablet, "GET", "auth/pin-users", nil, map[string]string{"X-Client-Kind": "android", "X-Client-Version": "0.4.1"})
	fixture.send(fixture.admin, "GET", "auth/me", nil, map[string]string{"X-Client-Kind": "desktop", "X-Client-Version": "0.16.0"})
	if version(tablet.deviceID) != "0.4.1" || version(fixture.admin.deviceID) != "0.16.0" {
		t.Fatalf("versión instalada de la tableta y del escritorio: %s %s", version(tablet.deviceID), version(fixture.admin.deviceID))
	}
	devices := decodeResponse[[]map[string]any](t, fixture.adminCall(t, "GET", "devices", nil, 200))
	for _, device := range devices {
		if device["id"] == fixture.display.deviceID && device["app_version"] != "0.1.4" {
			t.Fatalf("versión en Terminales: %v", device)
		}
	}
	// Ajustes de la tableta: la contraseña de quien administra terminales, sin sesión ni token.
	sessionsBefore := fixture.count(t, "SELECT count(*)::int FROM app.sessions")
	authorized := decodeResponse[map[string]any](t, fixture.displayCall(t, fixture.display, "POST", "kitchen-display/settings-access",
		map[string]string{"username": "admin", "password": "admin"}, 200))
	if authorized["authorized"] != true || len(authorized) != 1 || fixture.count(t, "SELECT count(*)::int FROM app.sessions") != sessionsBefore {
		t.Fatalf("ajustes con el administrador: %v", authorized)
	}
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/settings-access", map[string]string{"username": "admin", "password": "otra"},
		401, "AUTHORIZATION_INVALID")
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/settings-access",
		map[string]string{"username": "mesero_k1", "password": "Clave temporal de mesero_k1"}, 403, "PERMISSION_DENIED")
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/settings-access", map[string]string{"username": "", "password": ""},
		400, "VALIDATION_ERROR")
	if fixture.count(t, `SELECT count(*)::int FROM app.audit_events WHERE device_id=$1 AND action='kitchen_display.settings_opened'
        AND after_state->>'username'='admin'`, fixture.display.deviceID) != 1 ||
		fixture.count(t, `SELECT count(*)::int FROM app.audit_events WHERE device_id=$1 AND action='authorization.failed'`,
			fixture.display.deviceID) != 1 {
		t.Fatal("auditoría de los ajustes de la tableta")
	}
	// Terminales cambia la letra, el tema y el sonido; solo en una pantalla y con valores de su lista.
	fixture.adminCall(t, "PATCH", "devices/"+fixture.display.deviceID, map[string]any{"name": "Pantalla cocina K1", "keyboard_mode": "auto",
		"kitchen_display_text_size": "large", "kitchen_display_theme": "light", "kitchen_display_sound": false,
		"kitchen_display_volume": "high"}, 200)
	config := fixture.orders(t, fixture.display).Display
	if config.TextSize != "large" || config.Theme != "light" || config.SoundEnabled || config.SoundVolume != "high" {
		t.Fatalf("ajustes desde Terminales: %+v", config)
	}
	fixture.adminCall(t, "PATCH", "devices/"+fixture.display.deviceID, map[string]any{"name": "Pantalla cocina K1", "keyboard_mode": "auto",
		"kitchen_display_volume": "max"}, 400)
	fixture.adminCall(t, "PATCH", "devices/"+fixture.admin.deviceID, map[string]any{"name": "Prueba principal", "keyboard_mode": "auto",
		"kitchen_display_theme": "light"}, 400)
	configuration := decodeResponse[map[string]any](t, fixture.adminCall(t, "GET", "devices/"+fixture.display.deviceID+"/configuration", nil, 200))
	options, _ := configuration["kitchen_display_station_options"].([]any)
	device, _ := configuration["device"].(map[string]any)
	settings, _ := device["kitchen_display"].(map[string]any)
	// La versión es la de la última petición de la pantalla (las de esta prueba mandan 0.1.0).
	if len(options) != 2 || settings["text_size"] != "large" || device["app_version"] != "0.1.0" {
		t.Fatalf("configuración de la pantalla en Terminales: %v", configuration)
	}
}

func testStageK1KitchenDisplaySeats(t *testing.T, fixture *kitchenDisplayFixture) {
	// Una licencia con un solo cupo de cocina (ya lo ocupa la primera pantalla; la segunda también lo tenía con la
	// licencia anterior): la licencia nueva con menos cupos no interrumpe a nadie.
	source := seatstest.NewSource(seatstest.ContractMaximum, seatstest.ContractMaximum, 1)
	fixture.service.Seats = licenseseats.NewManager(licenseseats.Options{Limits: source})
	defer func() { fixture.service.Seats = seatstest.Roomy() }()
	fixture.adminCall(t, "POST", "devices/"+fixture.second.deviceID+"/seat/release", map[string]any{}, 200)
	// «Liberar cupo» de la segunda: deja de mostrar pedidos, avisa y no lo vuelve a ocupar sola.
	fixture.displayError(t, fixture.second, "GET", "kitchen-display/orders", nil, 409, "KITCHEN_DISPLAY_SEAT_RELEASED")
	fixture.displayError(t, fixture.second, "POST", "kitchen-display/connect", map[string]any{}, 409, "KITCHEN_DISPLAY_SEAT_RELEASED")
	fixture.displayError(t, fixture.second, "POST", "kitchen-display/items/ready", map[string]any{"tasks": []kitchen.DisplayTaskRef{}}, 409,
		"KITCHEN_DISPLAY_SEAT_RELEASED")
	// «Usar esta pantalla» sin cupo libre: el mensaje de la §6 con los datos.
	response := fixture.displayError(t, fixture.second, "POST", "kitchen-display/connect", map[string]any{"take_seat": true}, 409,
		"LICENSE_SEAT_UNAVAILABLE")
	problem := decodeResponse[struct {
		Error struct {
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}](t, response)
	if problem.Error.Details["seat"] != "kitchen_displays" || problem.Error.Details["in_use"] != float64(1) ||
		problem.Error.Details["limit"] != float64(1) || problem.Error.Message !=
		"No hay cupo de pantalla de cocina libre (1 de 1 en uso). Revoca otra pantalla en Terminales o amplía tu licencia." {
		t.Fatalf("sin cupo: %+v", problem)
	}
	// Sigue en «cupo liberado» (el intento sin cupo no se guardó).
	fixture.displayError(t, fixture.second, "POST", "kitchen-display/connect", map[string]any{}, 409, "KITCHEN_DISPLAY_SEAT_RELEASED")
	// Un corte de red no libera nada: la primera conserva su cupo aunque no se conecte.
	if fixture.count(t, `SELECT count(*)::int FROM app.license_seats WHERE device_id=$1`, fixture.display.deviceID) != 1 {
		t.Fatal("la primera perdió su cupo")
	}
	// Liberar el cupo de la primera: la segunda lo toma con «Usar esta pantalla».
	fixture.adminCall(t, "POST", "devices/"+fixture.display.deviceID+"/seat/release", map[string]any{}, 200)
	fixture.displayCall(t, fixture.second, "POST", "kitchen-display/connect", map[string]any{"take_seat": true}, 200)
	fixture.orders(t, fixture.second)
	if fixture.count(t, `SELECT count(*)::int FROM app.audit_events WHERE device_id=$1 AND action='kitchen_display.seat_taken'`,
		fixture.second.deviceID) != 1 || fixture.count(t, `SELECT count(*)::int FROM app.audit_events WHERE action='license.seat_released'
        AND entity_id=$1`, fixture.display.deviceID) != 1 {
		t.Fatal("auditoría del cupo")
	}
	// Liberar una pantalla sin cupo: no tiene nada que liberar.
	fixture.adminCall(t, "POST", "devices/"+fixture.display.deviceID+"/seat/release", map[string]any{}, 409)
	// Con más cupos, la primera vuelve con «Usar esta pantalla».
	source.Set(seatstest.ContractMaximum, seatstest.ContractMaximum, 2)
	fixture.displayCall(t, fixture.display, "POST", "kitchen-display/connect", map[string]any{"take_seat": true}, 200)
	fixture.orders(t, fixture.display)
	// Sin licencia válida no se ocupa ningún cupo (protección de respaldo de la puerta, que lo rechaza antes).
	third := fixture.kitchenDevice(t, "Pantalla K1 sin licencia")
	source.Withdraw()
	fixture.displayError(t, third, "POST", "kitchen-display/connect", map[string]any{}, 409, "LICENSE_NOT_ACTIVATED")
	if fixture.count(t, "SELECT count(*)::int FROM app.license_seats WHERE device_id=$1", third.deviceID) != 0 {
		t.Fatal("una pantalla ocupó cupo sin licencia válida")
	}
}

func testStageK1KitchenDisplayRevocation(t *testing.T, fixture *kitchenDisplayFixture) {
	server := httptest.NewServer(fixture.service)
	defer server.Close()
	request, err := http.NewRequestWithContext(context.Background(), "GET", server.URL+"/api/v1/events?after=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Device-ID", fixture.second.deviceID)
	request.Header.Set("X-Device-Token", fixture.second.deviceToken)
	request.Header.Set("X-Client-Kind", "kitchen")
	request.Header.Set("X-Client-Version", "0.1.0")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("canal de la pantalla: %d", response.StatusCode)
	}
	lines := make(chan string, 64)
	go func() {
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	waitFor := func(prefix string) string {
		deadline := time.After(5 * time.Second)
		for {
			select {
			case line, open := <-lines:
				if !open {
					t.Fatalf("el canal terminó sin %q", prefix)
				}
				if strings.HasPrefix(line, prefix) {
					return line
				}
			case <-deadline:
				t.Fatalf("no llegó %q", prefix)
			}
		}
	}
	waitFor("event: changes")
	revokedAt := time.Now()
	fixture.adminCall(t, "POST", "devices/"+fixture.second.deviceID+"/revoke", map[string]any{}, 200)
	waitFor("event: interrupted")
	if data := waitFor("data: "); !strings.Contains(data, `"DEVICE_REVOKED"`) {
		t.Fatalf("motivo de la interrupción: %s", data)
	}
	if time.Since(revokedAt) > 2*time.Second {
		t.Fatalf("revocar tardó %s en desconectar", time.Since(revokedAt))
	}
	fixture.displayError(t, fixture.second, "GET", "kitchen-display/orders", nil, 401, "DEVICE_REVOKED")
	// Revocar libera su cupo de cocina.
	if fixture.count(t, "SELECT count(*)::int FROM app.license_seats WHERE device_id=$1", fixture.second.deviceID) != 0 {
		t.Fatal("la pantalla revocada conserva su cupo")
	}
}

func testStageK1KitchenDisplayLicense(t *testing.T, fixture *kitchenDisplayFixture) {
	// La captura es antes de poner el gestor de licencias (sin activar no se captura).
	accountID := fixture.order(t, 0, draftLine(fixture.product, 1))
	clock := licensingtest.NewClock(time.Now().UTC().Truncate(time.Second))
	licenses := licensingtest.New(t, clock)
	licenses.Database = licensing.PoolProbe{Pool: fixture.pool}
	manager := licenses.Start()
	fixture.service.License = manager
	fixture.service.Seats = administration.LicenseSeats(manager)
	defer func() {
		fixture.service.License = nil
		fixture.service.Seats = seatstest.Roomy()
	}()
	// Sin activar (con datos: la base ya tiene ventas): no muestra pedidos; el canal sigue abierto.
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/connect", map[string]any{}, 409, "LICENSE_NOT_ACTIVATED")
	fixture.displayError(t, fixture.display, "GET", "kitchen-display/orders", nil, 409, "LICENSE_NOT_ACTIVATED")
	fixture.displayError(t, fixture.display, "GET", "kitchen-display/recent", nil, 409, "LICENSE_NOT_ACTIVATED")
	fixture.displayCall(t, fixture.display, "GET", "events/snapshot?after=0", nil, 200)
	fixture.displayCall(t, fixture.display, "GET", "license/status", nil, 200)
	fixture.displayCall(t, fixture.display, "POST", "kitchen-display/settings-access", map[string]string{"username": "admin", "password": "admin"}, 200)
	// Vigente.
	expires := clock.Now().Add(72 * time.Hour)
	licenses.Accept(manager, licensingtest.Subscription(expires, 3, 3, 3))
	fixture.displayCall(t, fixture.display, "POST", "kitchen-display/connect", map[string]any{}, 200)
	view := fixture.orders(t, fixture.display)
	if view.License == nil || view.License.State != licensing.StateActive || card(t, view, accountID) == nil {
		t.Fatalf("pantalla con la licencia vigente: %+v", view.License)
	}
	// Vencida: sigue mostrando y marcando.
	clock.Set(expires.Add(6 * 24 * time.Hour))
	view = fixture.orders(t, fixture.display)
	if view.License.State != licensing.StateExpired {
		t.Fatalf("estado: %s", view.License.State)
	}
	items := card(t, view, accountID).Items
	fixture.mark(t, fixture.display, "items/ready", items[0].Tasks)
	fixture.displayCall(t, fixture.display, "POST", "kitchen-display/recover", map[string]any{"tasks": items[0].Tasks}, 200)
	// Revocada: igual.
	licenses.Accept(manager, licensingtest.Revoked(licensing.ReasonTransferred))
	if state := fixture.orders(t, fixture.display).License.State; state != licensing.StateRevoked {
		t.Fatalf("estado: %s", state)
	}
	fixture.mark(t, fixture.display, "items/ready", items[0].Tasks)
	fixture.displayCall(t, fixture.display, "POST", "kitchen-display/recover", map[string]any{"tasks": items[0].Tasks}, 200)
	// Hora del equipo incorrecta: se lee, pero no se marca ni se recupera ni se confirma.
	licenses.Accept(manager, licensingtest.Perpetual(3, 3, 3))
	if err := fixture.service.CheckLicense(context.Background()); err != nil {
		t.Fatal(err)
	}
	clock.Set(manager.TrustedTime().Add(-3 * time.Hour))
	view = fixture.orders(t, fixture.display)
	if !view.License.Clock.Blocked || card(t, view, accountID) == nil {
		t.Fatalf("pantalla con la hora incorrecta: %+v", view.License.Clock)
	}
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/items/ready", map[string]any{"tasks": items[0].Tasks}, 409, "SYSTEM_CLOCK_INVALID")
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/recover", map[string]any{"tasks": items[0].Tasks}, 409, "SYSTEM_CLOCK_INVALID")
	fixture.displayError(t, fixture.display, "POST", "kitchen-display/receipts", map[string]any{"receipts": []map[string]string{
		{"ticket_id": "00000000-0000-4000-8000-0000000000aa", "station_id": fixture.cocina.ID}}}, 409, "SYSTEM_CLOCK_INVALID")
	fixture.displayCall(t, fixture.display, "GET", "kitchen-display/recent", nil, 200)
}
