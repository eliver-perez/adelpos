//go:build integration

package apicontract_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/apicontract"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/httpapi"
	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/licenseseats/seatstest"
	"local/ad-el-pos/server/internal/localsetup"
	"local/ad-el-pos/server/internal/product"
)

// Pruebas de contrato de la API v1 (etapa 9G, §5.1) contra el servidor real (httpapi + administration) y PostgreSQL:
// un recorrido como el de la aplicación Android usa cada operación documentada en openapi.json; cada petición se
// valida contra el esquema de su cuerpo y cada respuesta contra el esquema de su estado. Falla si una ruta
// documentada ya no existe o cambió su estado, si una respuesta real deja de cumplir el esquema (una propiedad
// obligatoria que desaparece o cambia de tipo) o si una operación del documento no se usa en el recorrido.
//
// Para extenderlo (fases B, C, E y F): documentar la operación en openapi.json, explicarla en docs/API_V1.md y
// agregar su paso en contractScenario (o en una función nueva que reciba el mismo *contractClient).

type contractClient struct {
	t        *testing.T
	contract *apicontract.Contract
	handler  http.Handler
	device   string
	token    string
	session  string
	version  string
	// kind es X-Client-Kind: el del dispositivo de la credencial (revisión R3, B2: desktop en la terminal del asistente,
	// android en el teléfono emparejado).
	kind    string
	covered map[string][]int
	// fingerprint es la huella SPKI del certificado del servidor (etapa 9G, fase C: emparejamiento).
	fingerprint [32]byte
	// pool prepara estados que la API no produce sola en el recorrido (etapa A2: un trabajo de impresión que falló,
	// porque aquí no corre el trabajador de la cola).
	pool *pgxpool.Pool
	// seats son los cupos de la licencia del recorrido (etapa L1, Fase D): el máximo del contrato, salvo en el paso de
	// los cupos, que los baja para recibir LICENSE_SEAT_UNAVAILABLE.
	seats *seatstest.Source
}

func contractID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	raw[6] = raw[6]&15 | 64
	raw[8] = raw[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

// request arma la petición como la aplicación: credenciales de la terminal, sesión y cabeceras de cliente.
func (client *contractClient) request(ctx context.Context, method, path string, body any, headers map[string]string) *http.Request {
	var reader io.Reader = http.NoBody
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequestWithContext(ctx, method, "/api/v1"+path, reader)
	request.RemoteAddr = "127.0.0.1:52900"
	request.Header.Set("X-Device-ID", client.device)
	request.Header.Set("X-Device-Token", client.token)
	if client.session != "" {
		request.Header.Set("Authorization", "Bearer "+client.session)
	}
	request.Header.Set(httpapi.ClientVersionHeader, client.version)
	request.Header.Set(httpapi.ClientKindHeader, client.kind)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	return request
}

// setup hace una petición de preparación que no es parte del contrato (alta del catálogo, mesas…).
func (client *contractClient) setup(method, path string, body any, expected int) []byte {
	client.t.Helper()
	response := httptest.NewRecorder()
	client.handler.ServeHTTP(response, client.request(context.Background(), method, path, body, nil))
	if response.Code != expected {
		client.t.Fatalf("preparación %s %s: esperado %d, recibido %d: %s", method, path, expected, response.Code, response.Body.String())
	}
	return response.Body.Bytes()
}

// call hace una petición del contrato: valida el cuerpo, el estado documentado, el tipo de contenido y la respuesta.
func (client *contractClient) call(method, path string, body any, headers map[string]string, expected int) *httptest.ResponseRecorder {
	client.t.Helper()
	route, _, _ := strings.Cut(path, "?")
	operation, found := client.contract.Match(method, route)
	if !found {
		client.t.Fatalf("%s %s no está en el contrato", method, route)
	}
	if body != nil {
		schema, err := client.contract.RequestSchema(operation)
		if err != nil || schema == nil {
			client.t.Fatalf("%s: el contrato no documenta su cuerpo: %v", operation.ID, err)
		}
		encoded, _ := json.Marshal(body)
		if err := client.contract.ValidateJSON(schema, encoded); err != nil {
			client.t.Fatalf("%s: el cuerpo enviado no cumple el contrato: %v", operation.ID, err)
		}
	}
	response := httptest.NewRecorder()
	client.handler.ServeHTTP(response, client.request(context.Background(), method, path, body, headers))
	if response.Code != expected {
		client.t.Fatalf("%s %s: esperado %d, recibido %d: %s", method, path, expected, response.Code, response.Body.String())
	}
	client.check(operation, response.Code, response.Header().Get("Content-Type"), response.Body.Bytes())
	return response
}

// check valida una respuesta contra el contrato y la anota como cubierta.
func (client *contractClient) check(operation *apicontract.Operation, status int, contentType string, body []byte) {
	client.t.Helper()
	if status < 400 {
		if _, explicit := operation.Responses[fmt.Sprint(status)]; !explicit {
			client.t.Fatalf("%s: el estado %d no está documentado", operation.ID, status)
		}
		types, err := client.contract.MediaTypes(operation, status)
		if err != nil {
			client.t.Fatal(err)
		}
		media, _, _ := strings.Cut(contentType, ";")
		if len(types) > 0 && !slices.Contains(types, media) {
			client.t.Fatalf("%s: tipo de contenido %q no documentado (%v)", operation.ID, contentType, types)
		}
		if len(types) == 0 && len(body) > 0 {
			client.t.Fatalf("%s: el estado %d no debería llevar contenido", operation.ID, status)
		}
	}
	schema, documented, err := client.contract.ResponseSchema(operation, status)
	if err != nil || !documented {
		client.t.Fatalf("%s: estado %d sin respuesta documentada: %v", operation.ID, status, err)
	}
	if schema != nil && strings.HasPrefix(contentType, "application/json") {
		if err := client.contract.ValidateJSON(schema, body); err != nil {
			client.t.Fatalf("%s %d: la respuesta no cumple el contrato: %v\n%.600s", operation.ID, status, err, body)
		}
	}
	client.covered[operation.ID] = append(client.covered[operation.ID], status)
}

func decodeContract[T any](t *testing.T, body []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("respuesta inválida: %v", err)
	}
	return value
}

func TestContractV1(t *testing.T) {
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
	seats := seatstest.NewSource(seatstest.ContractMaximum, seatstest.ContractMaximum, seatstest.ContractMaximum)
	service := &administration.Service{Pool: pool, ProvisioningFile: filename,
		Seats: licenseseats.NewManager(licenseseats.Options{Limits: seats})}
	// Identidad del servidor para el emparejamiento (etapa 9G, fase C: rutas pairing/* del contrato).
	serverIdentity, err := localsetup.EnsureIdentity(filepath.Join(t.TempDir(), localsetup.IdentityFolder))
	if err != nil {
		t.Fatal(err)
	}
	service.Identity = &administration.ServerIdentity{ServerID: serverIdentity.ServerID, Fingerprint: serverIdentity.Fingerprint}
	handler := httpapi.New(func(ctx context.Context) database.Health {
		return database.Probe(ctx, pool)
	}, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)), service)
	contract, err := apicontract.Load()
	if err != nil {
		t.Fatal(err)
	}
	client := &contractClient{t: t, contract: contract, handler: handler, version: product.Read().Version, kind: "desktop", covered: map[string][]int{},
		fingerprint: serverIdentity.Fingerprint, pool: pool, seats: seats}
	// Asistente inicial: la terminal principal y el administrador.
	setup := httptest.NewRequest("POST", "/api/v1/setup", strings.NewReader(`{"name":"Contrato v1","timezone":"America/Monterrey",
        "currency":"MXN","theme":"light","terminal_name":"Terminal de contrato"}`))
	setup.RemoteAddr = "127.0.0.1:52900"
	setup.Header.Set("X-Provisioning-Token", provisioning)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, setup)
	credentials := decodeContract[map[string]any](t, recorder.Body.Bytes())
	client.device, client.token = fmt.Sprint(credentials["device_id"]), fmt.Sprint(credentials["device_token"])
	contractScenario(t, client)
	documented := []string{}
	missing := []string{}
	for _, operation := range contract.Operations {
		documented = append(documented, operation.ID)
		if len(client.covered[operation.ID]) == 0 {
			missing = append(missing, operation.ID)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("operaciones documentadas que el recorrido no usó (ruta inexistente o falta su paso): %v", missing)
	}
	t.Logf("operaciones del contrato comprobadas: %d", len(documented))
}

// contractScenario es el recorrido de una terminal Android: versión, sesión, catálogo, «Agotado», cuentas y
// pedidos, pedido sin conexión, eventos y recuperación del estado.
func contractScenario(t *testing.T, client *contractClient) {
	client.call("GET", "/version", nil, nil, 200)
	client.call("GET", "/health/ready", nil, nil, 200)
	client.call("GET", "/openapi.json", nil, nil, 200)
	login := decodeContract[map[string]string](t, client.call("POST", "/auth/login",
		map[string]string{"username": "admin", "password": "admin"}, nil, 200).Body.Bytes())
	client.session = login["session_token"]
	client.call("GET", "/auth/me", nil, nil, 200)
	// Un cliente anterior a la versión mínima recibe «Actualiza esta terminal» con el error documentado.
	client.call("GET", "/auth/me", nil, map[string]string{httpapi.ClientVersionHeader: "0.0.1"}, 426)
	// Etapa L1 (fase B): el estado de la licencia, con sesión y con solo la credencial del dispositivo.
	status := decodeContract[map[string]any](t, client.call("GET", "/license/status", nil, nil, 200).Body.Bytes())
	if status["state"] != "not_activated" || status["license"] != nil {
		t.Fatalf("una instalación sin licencia: %v", status)
	}
	withoutSession := *client
	withoutSession.session = ""
	summary := decodeContract[map[string]any](t, withoutSession.call("GET", "/license/status", nil, nil, 200).Body.Bytes())
	if len(summary) != 3 || summary["state"] != "not_activated" {
		t.Fatalf("el resumen sin sesión: %v", summary)
	}

	// Catálogo de prueba (preparación, fuera del contrato).
	category := decodeContract[map[string]any](t, client.setup("POST", "/catalog/categories",
		map[string]any{"name": "Contrato", "active": true}, 200))
	picture := image.NewRGBA(image.Rect(0, 0, 3, 3))
	picture.Set(1, 1, color.RGBA{B: 200, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	uploaded := decodeContract[map[string]any](t, client.setup("POST", "/catalog/images",
		map[string]string{"content_base64": base64.StdEncoding.EncodeToString(encoded.Bytes())}, 200))
	imageID := fmt.Sprint(uploaded["id"])
	complement := decodeContract[map[string]any](t, client.setup("POST", "/catalog/complements",
		map[string]any{"name": "Arroz contrato", "active": true}, 200))
	group := decodeContract[map[string]any](t, client.setup("POST", "/catalog/groups", map[string]any{
		"name": "Guarnición contrato", "active": true, "pricing_mode": "value", "min_quantity": "0",
		"options": []map[string]any{{"complement_id": complement["id"], "reference_value": "10", "extra_price": "5", "active": true}},
	}, 200))
	burger := decodeContract[map[string]any](t, client.setup("POST", "/catalog/products", map[string]any{
		"name": "Hamburguesa contrato", "active": true, "category_id": category["id"], "code": "HAMB-CONTRATO", "kind": "product",
		"base_price": "120", "image_id": imageID, "groups": []map[string]any{{"group_id": group["id"]}},
		"variants": []map[string]any{{"name": "Doble", "code": "HAMB-CONTRATO-D", "price": "160", "active": true}},
	}, 200))
	fries := decodeContract[map[string]any](t, client.setup("POST", "/catalog/products", map[string]any{
		"name": "Papas contrato", "active": true, "category_id": category["id"], "code": "PAPAS-CONTRATO", "kind": "product",
		"base_price": "40",
	}, 200))
	area := decodeContract[map[string]any](t, client.setup("POST", "/restaurant/areas",
		map[string]any{"operation_id": contractID(), "name": "Salón contrato", "active": true}, 200))
	tables := []map[string]any{}
	for index := range 2 {
		tables = append(tables, decodeContract[map[string]any](t, client.setup("POST", "/restaurant/tables", map[string]any{
			"operation_id": contractID(), "area_id": area["id"], "name": fmt.Sprintf("Mesa contrato %d", index+1), "shape": "rectangle",
			"x": 20 + index*140, "y": 30, "width": 100, "height": 72, "seats": 4, "active": true,
		}, 200)))
	}

	// Catálogo descargable completo, por cambios, imágenes y productos de la captura.
	full := decodeContract[map[string]any](t, client.call("GET", "/catalog/sync", nil, nil, 200).Body.Bytes())
	version := int64(full["catalog_version"].(float64))
	epoch := fmt.Sprint(full["catalog_epoch"])
	client.call("GET", fmt.Sprintf("/catalog/sync?since=%d&epoch=%s", version, epoch), nil, nil, 200)
	defaultList := full["price_list"].(map[string]any)["id"]
	client.call("GET", fmt.Sprintf("/catalog/products?category_id=%s&price_list_id=%s", category["id"], defaultList), nil, nil, 200)
	client.call("GET", "/catalog/images/"+imageID, nil, nil, 200)
	content := client.call("GET", "/catalog/images/"+imageID+"/content", nil, nil, 200)
	client.call("GET", "/catalog/images/"+imageID+"/content", nil, map[string]string{"If-None-Match": content.Header().Get("ETag")}, 304)

	// «Agotado» y «Disponible otra vez».
	client.call("POST", fmt.Sprintf("/catalog/products/%s/sold-out", fries["id"]),
		map[string]any{"operation_id": contractID(), "sold_out": true, "indefinite": true}, nil, 200)
	client.call("GET", fmt.Sprintf("/catalog/sync?since=%d&epoch=%s", version, epoch), nil, nil, 200)
	client.call("POST", fmt.Sprintf("/catalog/products/%s/sold-out", fries["id"]),
		map[string]any{"operation_id": contractID(), "sold_out": false}, nil, 200)

	// Cuenta con su primer producto, borrador, cálculo y confirmación.
	customization := map[string]any{"ingredients": []any{}, "groups": []any{}, "components": []any{}, "notes": ""}
	opened := decodeContract[map[string]any](t, client.call("POST", "/restaurant/accounts", map[string]any{
		"operation_id": contractID(), "mode": "dine_in", "table_id": tables[0]["id"], "seen_account_ids": []string{},
		"items": []map[string]any{{"id": contractID(), "product_id": burger["id"], "variant_id": nil, "quantity": 1, "customization": customization}},
	}, nil, 200).Body.Bytes())
	account := opened["account"].(map[string]any)
	accountID := fmt.Sprint(account["id"])
	client.call("GET", "/restaurant/accounts", nil, nil, 200)
	client.call("GET", "/restaurant/accounts/"+accountID, nil, nil, 200)
	saved := decodeContract[map[string]any](t, client.call("PUT", "/restaurant/accounts/"+accountID+"/draft", map[string]any{
		"operation_id": contractID(), "revision": account["revision"],
		"items": []map[string]any{
			{"id": contractID(), "product_id": burger["id"], "variant_id": nil, "quantity": 2, "customization": customization},
			{"id": contractID(), "product_id": fries["id"], "variant_id": nil, "quantity": 1, "customization": customization},
		},
	}, nil, 200).Body.Bytes())
	preview := decodeContract[map[string]any](t, client.call("GET", "/restaurant/accounts/"+accountID+"/preview", nil, nil, 200).Body.Bytes())
	client.call("POST", "/restaurant/accounts/"+accountID+"/confirm", map[string]any{
		"operation_id": contractID(), "revision": saved["account"].(map[string]any)["revision"], "expected_fingerprint": preview["fingerprint"],
	}, nil, 200)

	// Pedido capturado sin conexión: sin cambios se acepta; con un precio distinto requiere confirmación.
	offline := func(seen string) map[string]any {
		return map[string]any{
			"operation_id":    contractID(),
			"catalog_version": version,
			"captured_at":     time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano),
			"destination":     map[string]any{"table_id": tables[1]["id"], "seen_account_ids": []string{}},
			"items": []map[string]any{{"id": contractID(), "product_id": fries["id"], "variant_id": nil, "quantity": 1,
				"customization": customization, "seen_unit_price": seen}},
		}
	}
	client.call("POST", "/restaurant/offline-orders", offline("40.00"), nil, 200)
	stale := offline("35.00")
	stale["destination"] = map[string]any{"account_id": accountID}
	rejection := decodeContract[map[string]any](t, client.call("POST", "/restaurant/offline-orders", stale, nil, 409).Body.Bytes())
	details := rejection["error"].(map[string]any)["details"].(map[string]any)
	stale["confirmation"] = map[string]any{"catalog_version": details["catalog_version"], "changes": details["changes"]}
	accepted := decodeContract[map[string]any](t, client.call("POST", "/restaurant/offline-orders", stale, nil, 200).Body.Bytes())
	// El envío original que llega tarde con la misma clave recibe el pedido que ya entró (revisión de la fase D).
	delete(stale, "confirmation")
	late := decodeContract[map[string]any](t, client.call("POST", "/restaurant/offline-orders", stale, nil, 409).Body.Bytes())
	lateError := late["error"].(map[string]any)
	if lateError["code"] != "OFFLINE_ORDER_ALREADY_ACCEPTED" ||
		lateError["details"].(map[string]any)["result"].(map[string]any)["order_id"] != accepted["order_id"] {
		t.Fatalf("envío tardío del pedido ya aceptado: %v", late)
	}
	// Etapa A1 (fase E): mesas, modificar y cancelar unidades, cuenta que ya no está abierta y el borrador en el pedido
	// sin conexión.
	waiterData := contractWaiterData{
		accountID: accountID, tableID: fmt.Sprint(tables[0]["id"]), burger: fmt.Sprint(burger["id"]), fries: fmt.Sprint(fries["id"]),
		version: version, customization: customization,
	}
	contractWaiterMode(t, client, waiterData)

	// Eventos en tiempo real: el primer mensaje del flujo cumple EventBatch.
	streamContext, stop := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer stop()
	stream := httptest.NewRecorder()
	client.handler.ServeHTTP(stream, client.request(streamContext, "GET", "/events?after=0", nil, nil))
	operation, _ := client.contract.Match("GET", "/events")
	client.check(operation, stream.Code, stream.Header().Get("Content-Type"), nil)
	scanner := bufio.NewScanner(bytes.NewReader(stream.Body.Bytes()))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	batch := ""
	for scanner.Scan() {
		if data, found := strings.CutPrefix(scanner.Text(), "data: "); found {
			batch = data
			break
		}
	}
	if err := client.contract.ValidateJSON(map[string]any{"$ref": "#/components/schemas/EventBatch"}, []byte(batch)); err != nil {
		t.Fatalf("el mensaje del canal de eventos no cumple el contrato: %v (%q)", err, batch)
	}
	// Etapa A1: confirmar la cuenta avisó en el ámbito restaurant que cambió su preparación.
	if !strings.Contains(batch, `"kind":"restaurant.account.preparation_changed","scope":"restaurant","station_id":null,"aggregate_id":"`+accountID+`"`) {
		t.Fatalf("el canal no trae el evento de preparación de la cuenta: %.2000s", batch)
	}

	// Recuperación del estado y cierre de sesión.
	state := decodeContract[map[string]any](t, client.call("GET", "/restaurant/state", nil, nil, 200).Body.Bytes())
	if accounts, _ := state["accounts"].([]any); len(accounts) < 2 {
		t.Fatalf("el estado no trae las cuentas abiertas: %v", state["accounts"])
	}
	// Etapa A1: el cursor del canal de eventos al leer el estado; desde él no queda nada pendiente.
	cursor, _ := state["event_cursor"].(float64)
	if cursor < 1 {
		t.Fatalf("cursor de eventos del estado: %v", state["event_cursor"])
	}
	phone := contractPairing(t, client)
	// Etapa A1 (fase B): el teléfono emparejado muestra a los meseros, uno entra con PIN y la cola entrega un pedido con
	// la credencial de envío (sin sesión).
	order := offline("40.00")
	order["destination"] = map[string]any{"account_id": accountID}
	contractWaiterPIN(t, client, phone, order)
	// Etapa A2 (parte B): Para llevar desde el teléfono.
	contractTakeaway(t, client, phone, waiterData)
	// Etapa A2 (parte C): la precuenta desde el teléfono, en su impresora.
	contractPrecheck(t, client, phone, waiterData)
	// Etapa A2 (parte D): los avisos de platillos listos del teléfono, con su lectura y su canal.
	contractReadyAlerts(t, client, phone, waiterData)
	// Etapa K1 (Fase A): la pantalla de cocina para Android, emparejada como kitchen y sin sesión.
	contractKitchenDisplay(t, client, waiterData)
	// Etapa K1.1 (Fase A): la cobertura del enrutamiento, «Productos sin ruta» y los dos 409 del panel de cocina.
	contractKitchenCoverage(t, client, waiterData)
	// Etapa M1 (Fase A): uniones de mesas y mesas divisibles.
	contractTableUnions(t, client, waiterData)
	// Etapa L1 (Fase D): el modo Cocina y el rechazo por cupo de la licencia.
	contractSeats(t, client, phone, waiterData)
	client.call("POST", "/auth/logout", nil, nil, 200)
}

// contractSeats recorre los cupos de la licencia (etapa L1, Fase D): Preparación ocupa y libera el cupo de cocina
// (setKitchenDisplayMode) y, sin cupo, Preparación y el pedido de la cola del teléfono reciben 409
// LICENSE_SEAT_UNAVAILABLE con LicenseSeatUnavailable, sin guardar nada. El rechazo de las demás capturas se recorre en
// internal/operations (stagel1_seats_test.go).
func contractSeats(t *testing.T, client *contractClient, phone *contractClient, data contractWaiterData) {
	seatError := func(body []byte, seat string, limit float64) {
		t.Helper()
		if err := client.contract.ValidateJSON(map[string]any{"$ref": "#/components/schemas/LicenseSeatUnavailable"}, body); err != nil {
			t.Fatalf("el rechazo por cupo no cumple LicenseSeatUnavailable: %v (%s)", err, body)
		}
		details := decodeContract[map[string]any](t, body)["error"].(map[string]any)["details"].(map[string]any)
		if details["seat"] != seat || details["limit"] != limit {
			t.Fatalf("rechazo por cupo: %s", body)
		}
	}
	entered := decodeContract[map[string]any](t, client.call("POST", "/kitchen/display-mode", map[string]any{"active": true}, nil, 200).Body.Bytes())
	if entered["active"] != true {
		t.Fatalf("entrar a Preparación: %v", entered)
	}
	client.call("POST", "/kitchen/display-mode", map[string]any{"active": false}, nil, 200)
	client.seats.Set(seatstest.ContractMaximum, 0, 0)
	defer client.seats.Set(seatstest.ContractMaximum, seatstest.ContractMaximum, seatstest.ContractMaximum)
	seatError(client.call("POST", "/kitchen/display-mode", map[string]any{"active": true}, nil, 409).Body.Bytes(), "kitchen_displays", 0)
	// El teléfono ya ocupa su cupo de comandero (quien lo tiene lo conserva con una licencia nueva con menos cupos): se
	// libera desde Terminales (administración, fuera del contrato) y el mesero vuelve a entrar con su PIN.
	client.setup("POST", "/devices/"+phone.device+"/seat/release", map[string]any{}, 200)
	// El pedido de la cola del teléfono, con la credencial de envío: rechazo definitivo y la clave sin registrar.
	customization := data.customization
	waiterID := ""
	if err := client.pool.QueryRow(context.Background(), "SELECT id::text FROM app.users WHERE username='ana.contrato'").Scan(&waiterID); err != nil {
		t.Fatal(err)
	}
	login := decodeContract[map[string]any](t, phone.call("POST", "/auth/pin-login",
		map[string]string{"user_id": waiterID, "pin": "4829"}, nil, 200).Body.Bytes())
	order := map[string]any{
		"operation_id": contractID(), "catalog_version": data.version,
		"captured_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano),
		"destination": map[string]any{"account_id": data.accountID},
		"items": []map[string]any{{"id": contractID(), "product_id": data.fries, "variant_id": nil, "quantity": 1,
			"customization": customization, "seen_unit_price": "40.00"}},
	}
	queue := *phone
	headers := map[string]string{"X-Offline-Credential": fmt.Sprint(login["offline_credential"])}
	seatError(queue.call("POST", "/restaurant/offline-orders", order, headers, 409).Body.Bytes(), "order_takers", 0)
	var recorded int
	if err := client.pool.QueryRow(context.Background(), "SELECT count(*)::int FROM app.restaurant_operations WHERE id=$1",
		order["operation_id"]).Scan(&recorded); err != nil || recorded != 0 {
		t.Fatalf("el pedido rechazado por cupo registró su clave: %d %v", recorded, err)
	}
}

// contractWaiterData es lo que el recorrido del modo Mesero toma del recorrido principal.
type contractWaiterData struct {
	accountID     string
	tableID       string
	burger        string
	fries         string
	version       int64
	customization map[string]any
}

// contractWaiterMode es lo que usa el modo Mesero de la etapa A1 (fase E): las mesas con su mapa, modificar y cancelar
// unidades enviadas, el rechazo de una cuenta que ya no está abierta (con details.reason) y un pedido sin conexión que
// sustituye un renglón del borrador (y otro que ya no está como lo vio el dispositivo).
func contractWaiterMode(t *testing.T, client *contractClient, data contractWaiterData) {
	overview := decodeContract[map[string]any](t, client.call("GET", "/restaurant/overview", nil, nil, 200).Body.Bytes())
	if areas, _ := overview["areas"].([]any); len(areas) != 1 {
		t.Fatalf("áreas: %v", overview["areas"])
	}
	if tables, _ := overview["tables"].([]any); len(tables) != 2 || tables[0].(map[string]any)["shape"] != "rectangle" {
		t.Fatalf("mesas: %v", overview["tables"])
	}
	route := "/restaurant/accounts/" + data.accountID
	bill := decodeContract[map[string]any](t, client.call("GET", route, nil, nil, 200).Body.Bytes())
	line := func(product string) map[string]any {
		for _, value := range bill["lines"].([]any) {
			candidate := value.(map[string]any)
			if candidate["state"] == "active" && candidate["request"].(map[string]any)["product_id"] == product {
				return candidate
			}
		}
		t.Fatalf("la cuenta no tiene una partida activa de %s: %v", product, bill["lines"])
		return nil
	}
	burger := line(data.burger)
	revision := bill["account"].(map[string]any)["revision"]
	// Cambiar solo las observaciones conserva los importes; la vista previa da la huella que exige la modificación.
	changed := map[string]any{"product_id": data.burger, "variant_id": nil, "quantity": burger["quantity"],
		"customization": map[string]any{"ingredients": []any{}, "groups": []any{}, "components": []any{}, "notes": "Sin cebolla"}}
	preview := decodeContract[map[string]any](t, client.call("POST", route+"/edit-preview", map[string]any{
		"revision": revision, "version_id": burger["version_id"], "line": changed,
	}, nil, 200).Body.Bytes())
	bill = decodeContract[map[string]any](t, client.call("POST", route+"/edit", map[string]any{
		"operation_id": contractID(), "revision": revision, "version_id": burger["version_id"], "line": changed,
		"expected_fingerprint": preview["fingerprint"], "reason": "El cliente la pidió sin cebolla",
	}, nil, 200).Body.Bytes())
	fries := line(data.fries)
	bill = decodeContract[map[string]any](t, client.call("POST", route+"/cancel", map[string]any{
		"operation_id": contractID(), "revision": bill["account"].(map[string]any)["revision"], "version_id": fries["version_id"],
		"quantity": 1, "reason": "El cliente ya no las quiso",
	}, nil, 200).Body.Bytes())

	// Una cuenta que se canceló mientras tanto: 400 OPERATION_VALIDATION con details.reason.
	item := map[string]any{"id": contractID(), "product_id": data.fries, "variant_id": nil, "quantity": 1, "customization": data.customization}
	discarded := decodeContract[map[string]any](t, client.call("POST", "/restaurant/accounts", map[string]any{
		"operation_id": contractID(), "mode": "dine_in", "items": []map[string]any{item},
	}, nil, 200).Body.Bytes())["account"].(map[string]any)
	discardedID := fmt.Sprint(discarded["id"])
	// Etapa A3 (A3-D8): discardAccount en el contrato. Una cuenta con solo borrador es descartable y no liberable; una
	// con productos enviados no se descarta (409 ACCOUNT_NOT_EMPTY) y sigue abierta.
	if discarded["discardable"] != true || discarded["releasable"] != false {
		t.Fatalf("cuenta con solo borrador: discardable %v, releasable %v", discarded["discardable"], discarded["releasable"])
	}
	if opened := bill["account"].(map[string]any); opened["discardable"] != false || opened["releasable"] != false {
		t.Fatalf("cuenta con productos enviados: discardable %v, releasable %v", opened["discardable"], opened["releasable"])
	}
	notEmpty := decodeContract[map[string]any](t, client.call("POST", route+"/discard", map[string]any{
		"operation_id": contractID(), "revision": bill["account"].(map[string]any)["revision"],
	}, nil, 409).Body.Bytes())
	if code := notEmpty["error"].(map[string]any)["code"]; code != "ACCOUNT_NOT_EMPTY" {
		t.Fatalf("descartar una cuenta con productos enviados: %v", code)
	}
	discardedBill := decodeContract[map[string]any](t, client.call("POST", "/restaurant/accounts/"+discardedID+"/discard",
		map[string]any{"operation_id": contractID(), "revision": discarded["revision"]}, nil, 200).Body.Bytes())
	if account := discardedBill["account"].(map[string]any); account["state"] != "voided" || account["discardable"] != false {
		t.Fatalf("cuenta descartada: %v", account)
	}
	if draft, _ := discardedBill["draft"].([]any); len(draft) != 0 {
		t.Fatalf("el descarte debía borrar el borrador: %v", discardedBill["draft"])
	}
	rejected := decodeContract[map[string]any](t, client.call("PUT", "/restaurant/accounts/"+discardedID+"/draft", map[string]any{
		"operation_id": contractID(), "revision": discarded["revision"], "items": []map[string]any{item},
	}, nil, 400).Body.Bytes())
	details, _ := json.Marshal(rejected["error"].(map[string]any)["details"])
	if err := client.contract.ValidateJSON(map[string]any{"$ref": "#/components/schemas/AccountNotOpenDetails"}, details); err != nil ||
		!strings.Contains(string(details), `"account_state":"voided"`) {
		t.Fatalf("detalle de la cuenta que ya no está abierta: %v %s", err, details)
	}

	contractOfflineNeverRepeats(t, client, data, bill["account"].(map[string]any)["revision"])
}

// contractOfflineNeverRepeats es el pedido sin conexión frente al borrador y a lo ya enviado (A1-D6 y A1-D18): un
// renglón guardado con el id de una partida se sustituye aunque el pedido no lo nombre; una partida ya enviada es
// draft_item_sent y al confirmar se envía lo demás; la clave de un «Enviar a cocina» ya aplicado es
// OFFLINE_ORDER_ALREADY_ACCEPTED (origin confirm_draft); y a una mesa cuyas partidas ya están en una cuenta suya,
// items_in_account con la cuenta.
func contractOfflineNeverRepeats(t *testing.T, client *contractClient, data contractWaiterData, revision any) {
	route := "/restaurant/accounts/" + data.accountID
	line := func(quantity int) map[string]any {
		return map[string]any{"id": contractID(), "product_id": data.fries, "variant_id": nil, "quantity": quantity, "customization": data.customization}
	}
	item := func(saved map[string]any) map[string]any {
		return map[string]any{"id": saved["id"], "product_id": data.fries, "variant_id": nil, "quantity": saved["quantity"],
			"customization": data.customization, "seen_unit_price": "40.00"}
	}
	order := func(key string, destination map[string]any, items ...map[string]any) map[string]any {
		return map[string]any{
			"operation_id": key, "catalog_version": data.version,
			"captured_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano),
			"destination": destination, "items": items,
		}
	}
	account := map[string]any{"account_id": data.accountID}
	saved, other := line(2), line(1)
	client.call("PUT", route+"/draft", map[string]any{
		"operation_id": contractID(), "revision": revision, "items": []map[string]any{saved, other},
	}, nil, 200)
	// El renglón guardado con el id de la partida se quita aunque el pedido no lo nombre; el otro se queda.
	accepted := decodeContract[map[string]any](t, client.call("POST", "/restaurant/offline-orders",
		order(contractID(), account, item(saved)), nil, 200).Body.Bytes())
	removed, _ := accepted["removed_draft_item_ids"].([]any)
	remaining, _ := accepted["bill"].(map[string]any)["draft"].([]any)
	if len(removed) != 1 || removed[0] != saved["id"] || len(remaining) != 1 || remaining[0].(map[string]any)["id"] != other["id"] {
		t.Fatalf("pedido que sustituye el borrador: quitados %v, quedan %v", removed, remaining)
	}
	// La partida ya enviada: draft_item_sent; confirmado, entra solo lo demás.
	extra := line(1)
	resend := order(contractID(), account, item(saved), item(extra))
	rejection := decodeContract[map[string]any](t, client.call("POST", "/restaurant/offline-orders", resend, nil, 409).Body.Bytes())
	details := rejection["error"].(map[string]any)["details"].(map[string]any)
	if list, _ := details["changes"].([]any); len(list) != 1 || list[0].(map[string]any)["kind"] != "draft_item_sent" {
		t.Fatalf("partida ya enviada: %v", details)
	}
	resend["confirmation"] = map[string]any{"catalog_version": details["catalog_version"], "changes": details["changes"]}
	accepted = decodeContract[map[string]any](t, client.call("POST", "/restaurant/offline-orders", resend, nil, 200).Body.Bytes())
	if sent, _ := accepted["already_sent_item_ids"].([]any); len(sent) != 1 || sent[0] != saved["id"] {
		t.Fatalf("ya enviadas: %v", accepted["already_sent_item_ids"])
	}
	// «Enviar a cocina» con la clave K que entró sin respuesta; la cola manda el pedido con K: ya aceptado.
	key := contractID()
	current := decodeContract[map[string]any](t, client.call("GET", route, nil, nil, 200).Body.Bytes())
	preview := decodeContract[map[string]any](t, client.call("GET", route+"/preview", nil, nil, 200).Body.Bytes())
	client.call("POST", route+"/confirm", map[string]any{
		"operation_id": key, "revision": current["account"].(map[string]any)["revision"], "expected_fingerprint": preview["fingerprint"],
	}, nil, 200)
	repeated := client.call("POST", "/restaurant/offline-orders", order(key, account, item(other)), nil, 409).Body.Bytes()
	if err := client.contract.ValidateJSON(map[string]any{"$ref": "#/components/schemas/OfflineOrderAlreadyAccepted"}, repeated); err != nil ||
		!strings.Contains(string(repeated), `"origin":"confirm_draft"`) {
		t.Fatalf("pedido con la clave de un Enviar a cocina aplicado: %v %s", err, repeated)
	}
	// A la mesa, con partidas que ya están en su cuenta abierta: items_in_account con la cuenta.
	redirect := decodeContract[map[string]any](t, client.call("POST", "/restaurant/offline-orders",
		order(contractID(), map[string]any{"table_id": data.tableID, "seen_account_ids": []string{}}, item(other)), nil, 409).Body.Bytes())
	changes, _ := redirect["error"].(map[string]any)["details"].(map[string]any)["changes"].([]any)
	if len(changes) != 1 || changes[0].(map[string]any)["kind"] != "items_in_account" || changes[0].(map[string]any)["account_id"] != data.accountID {
		t.Fatalf("partidas ya en una cuenta de la mesa: %v", redirect)
	}
}

// contractWaiterPIN es la entrada con PIN de la etapa A1 (fase B): el administrador da a un mesero su PIN (fuera del
// contrato, en Personal › Usuarios); el teléfono emparejado pide la lista sin sesión, el PIN incorrecto se rechaza, el
// correcto entrega la sesión y la credencial de envío, y con esa credencial (sin sesión) se entrega un pedido
// capturado sin conexión.
func contractWaiterPIN(t *testing.T, client *contractClient, phone *contractClient, order map[string]any) {
	role := decodeContract[map[string]any](t, client.setup("POST", "/roles", map[string]any{"name": "Meseros contrato",
		"permissions": []string{"waiter_devices.use", "orders.confirm", "orders.capture", "accounts.read", "accounts.open",
			"catalog.read", "catalog.prices.read"}}, 200))
	waiter := decodeContract[map[string]any](t, client.setup("POST", "/users", map[string]any{"username": "ana.contrato",
		"display_name": "Ana del contrato", "role_id": role["id"], "active": true, "password": "clave-del-contrato"}, 200))
	waiterID := fmt.Sprint(waiter["id"])
	client.setup("PUT", "/users/"+waiterID+"/pin", map[string]any{"pin": "4829", "short_name": "Ana", "badge_color": "#3591d4"}, 200)
	list := decodeContract[map[string]any](t, phone.call("GET", "/auth/pin-users", nil, nil, 200).Body.Bytes())
	users, _ := list["users"].([]any)
	if len(users) != 1 || users[0].(map[string]any)["id"] != waiterID || users[0].(map[string]any)["color"] != "#3591d4" {
		t.Fatalf("meseros del teléfono: %v", list)
	}
	phone.call("POST", "/auth/pin-login", map[string]string{"user_id": waiterID, "pin": "4828"}, nil, 401)
	login := decodeContract[map[string]any](t, phone.call("POST", "/auth/pin-login",
		map[string]string{"user_id": waiterID, "pin": "4829"}, nil, 200).Body.Bytes())
	waiterPhone := *phone
	waiterPhone.session = fmt.Sprint(login["session_token"])
	identity := decodeContract[map[string]any](t, waiterPhone.call("GET", "/auth/me", nil, nil, 200).Body.Bytes())
	if identity["terminal"].(map[string]any)["idle_lock_seconds"] != float64(120) {
		t.Fatalf("bloqueo por inactividad del teléfono: %v", identity["terminal"])
	}
	queue := *phone
	accepted := decodeContract[map[string]any](t, queue.call("POST", "/restaurant/offline-orders", order,
		map[string]string{"X-Offline-Credential": fmt.Sprint(login["offline_credential"])}, 200).Body.Bytes())
	if accepted["status"] != "accepted" {
		t.Fatalf("pedido con la credencial de envío: %v", accepted)
	}
	// Otro mesero falla 5 veces: 429 PIN_LOCKED con su bloqueo por tiempo (PinLockedError) y la lista lo informa.
	other := decodeContract[map[string]any](t, client.setup("POST", "/users", map[string]any{"username": "beto.contrato",
		"display_name": "Beto del contrato", "role_id": role["id"], "active": true, "password": "clave-del-contrato-2"}, 200))
	otherID := fmt.Sprint(other["id"])
	client.setup("PUT", "/users/"+otherID+"/pin", map[string]any{"pin": "6152", "short_name": "Beto", "badge_color": ""}, 200)
	for range 4 {
		phone.call("POST", "/auth/pin-login", map[string]string{"user_id": otherID, "pin": "6153"}, nil, 401)
	}
	locked := decodeContract[map[string]any](t, phone.call("POST", "/auth/pin-login",
		map[string]string{"user_id": otherID, "pin": "6153"}, nil, 429).Body.Bytes())
	details := locked["error"].(map[string]any)["details"].(map[string]any)
	if details["requires_unlock"] != false || details["locked_until"] == nil {
		t.Fatalf("bloqueo por tiempo: %v", locked)
	}
	for _, entry := range decodeContract[map[string]any](t, phone.call("GET", "/auth/pin-users", nil, nil, 200).Body.Bytes())["users"].([]any) {
		if user := entry.(map[string]any); user["id"] == otherID && (user["locked_until"] == nil || user["requires_unlock"] != false) {
			t.Fatalf("la lista no informa el bloqueo: %v", user)
		}
	}
}

// contractPairing es el emparejamiento de un teléfono Android (etapa 9G, fases B y C): pide el código con el
// compromiso de su número, lo revela, cambia su nombre, espera la aprobación (que se hace en la computadora
// principal, fuera del contrato) y reclama su credencial una sola vez; otra solicitud se cancela.
func contractPairing(t *testing.T, client *contractClient) *contractClient {
	// Las rutas pairing/* las llama el teléfono que se empareja (todavía sin credencial), como cliente android.
	terminal := *client
	terminal.kind = "android"
	nonce := make([]byte, 32)
	_, _ = rand.Read(nonce)
	created := decodeContract[map[string]string](t, terminal.call("POST", "/pairing/requests", map[string]any{
		"name": "Mesero contrato", "device_kind": "android", "os": "Android 15", "app_version": client.version,
		"commitment": administration.PairingCommitment(nonce),
	}, nil, 200).Body.Bytes())
	requestID := created["request_id"]
	secret := map[string]string{"X-Pairing-Secret": created["pairing_secret"]}
	terminal.call("POST", "/pairing/requests/"+requestID+"/reveal", map[string]string{"terminal_nonce": hex.EncodeToString(nonce)}, secret, 200)
	terminal.call("POST", "/pairing/requests/"+requestID+"/name", map[string]string{"name": "Mesero Ana contrato"}, secret, 200)
	state := decodeContract[map[string]string](t, terminal.call("GET", "/pairing/requests/"+requestID, nil, secret, 200).Body.Bytes())
	if state["state"] != "pending" || state["name"] != "Mesero Ana contrato" {
		t.Fatalf("solicitud pendiente: %v", state)
	}
	// El teléfono deriva el código con la huella que vio en el intercambio TLS; el administrador lo escribe.
	serverNonce, err := hex.DecodeString(created["server_nonce"])
	if err != nil || len(serverNonce) != 32 {
		t.Fatalf("número del servidor: %q", created["server_nonce"])
	}
	code, err := administration.PairingCode(client.fingerprint, nonce, serverNonce, requestID)
	if err != nil {
		t.Fatal(err)
	}
	client.setup("POST", "/devices/pairing-requests/"+requestID+"/approve", map[string]string{"code": code}, 200)
	if approved := decodeContract[map[string]string](t, terminal.call("GET", "/pairing/requests/"+requestID, nil, secret, 200).Body.Bytes()); approved["state"] != "approved" {
		t.Fatalf("solicitud aprobada: %v", approved)
	}
	claimed := decodeContract[map[string]string](t, terminal.call("POST", "/pairing/requests/"+requestID+"/claim", map[string]any{}, secret, 200).Body.Bytes())
	if claimed["name"] != "Mesero Ana contrato" || claimed["server_id"] != created["server_id"] {
		t.Fatalf("credencial reclamada: %v", claimed)
	}
	again := httptest.NewRecorder()
	terminal.handler.ServeHTTP(again, terminal.request(context.Background(), "POST", "/pairing/requests/"+requestID+"/claim", map[string]any{}, secret))
	if again.Code != 409 || strings.Contains(again.Body.String(), "device_token") {
		t.Fatalf("la credencial se entrega una sola vez: %d %s", again.Code, again.Body.String())
	}
	other := decodeContract[map[string]string](t, terminal.call("POST", "/pairing/requests", map[string]any{
		"name": "Mesero que se arrepiente", "device_kind": "android", "os": "Android 15", "app_version": client.version,
		"commitment": administration.PairingCommitment(serverNonce),
	}, nil, 200).Body.Bytes())
	terminal.call("POST", "/pairing/requests/"+other["request_id"]+"/cancel", map[string]any{},
		map[string]string{"X-Pairing-Secret": other["pairing_secret"]}, 200)
	// El teléfono ya emparejado (etapa A1): sus credenciales, sin sesión.
	phone := *client
	phone.device, phone.token, phone.session, phone.kind = claimed["device_id"], claimed["device_token"], "", "android"
	return &phone
}

// contractTakeaway es Para llevar en la aplicación de meseros (etapa A2, parte B): un mesero con el rol de la guía (los
// permisos de la A1 más takeaway.*) entra con PIN; la cola entrega con la credencial de envío un pedido para llevar
// nuevo (destination.takeaway); la lista de pendientes y restaurant/state lo traen con su cliente y sus estados; el
// mesero cambia el nombre y el teléfono (updateTakeawayCustomer) y solo puede marcarlo entregado cuando la caja ya lo
// cobró (deliverTakeaway: BALANCE_PENDING antes, finance null después porque el mesero no consulta cobros).
func contractTakeaway(t *testing.T, client *contractClient, phone *contractClient, data contractWaiterData) {
	role := decodeContract[map[string]any](t, client.setup("POST", "/roles", map[string]any{"name": "Meseros para llevar contrato",
		"permissions": []string{"waiter_devices.use", "orders.confirm", "orders.capture", "accounts.read", "accounts.open",
			"catalog.read", "catalog.prices.read", "events.read", "takeaway.read", "takeaway.manage", "takeaway.deliver"}}, 200))
	waiter := decodeContract[map[string]any](t, client.setup("POST", "/users", map[string]any{"username": "carla.contrato",
		"display_name": "Carla del contrato", "role_id": role["id"], "active": true, "password": "clave-del-contrato-3"}, 200))
	waiterID := fmt.Sprint(waiter["id"])
	client.setup("PUT", "/users/"+waiterID+"/pin", map[string]any{"pin": "7395", "short_name": "Carla", "badge_color": ""}, 200)
	login := decodeContract[map[string]any](t, phone.call("POST", "/auth/pin-login",
		map[string]string{"user_id": waiterID, "pin": "7395"}, nil, 200).Body.Bytes())
	waiterPhone := *phone
	waiterPhone.session = fmt.Sprint(login["session_token"])
	queue := *phone
	order := map[string]any{
		"operation_id": contractID(), "catalog_version": data.version,
		"captured_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano),
		"destination": map[string]any{"takeaway": map[string]any{"customer_name": "Juan", "phone": "5551234567"}},
		"items": []map[string]any{{"id": contractID(), "product_id": data.fries, "variant_id": nil, "quantity": 1,
			"customization": data.customization, "seen_unit_price": "40.00"}},
	}
	accepted := decodeContract[map[string]any](t, queue.call("POST", "/restaurant/offline-orders", order,
		map[string]string{"X-Offline-Credential": fmt.Sprint(login["offline_credential"])}, 200).Body.Bytes())
	account := accepted["bill"].(map[string]any)["account"].(map[string]any)
	accountID := fmt.Sprint(account["id"])
	if account["mode"] != "takeaway" || account["table_id"] != nil || account["origin"] != "Para llevar" || account["customer_name"] != "Juan" ||
		account["assigned_user_id"] != waiterID {
		t.Fatalf("pedido para llevar sin conexión: %v", account)
	}
	pending := func(source []any) map[string]any {
		for _, value := range source {
			if candidate := value.(map[string]any); candidate["id"] == accountID {
				return candidate
			}
		}
		return nil
	}
	list := decodeContract[map[string]any](t, waiterPhone.call("GET", "/restaurant/accounts?mode=takeaway&state=open", nil, nil, 200).Body.Bytes())
	if found := pending(list["items"].([]any)); found == nil || found["payment_state"] != "not_recorded" || found["delivery_state"] != "not_delivered" {
		t.Fatalf("pendientes para llevar: %v", list["items"])
	}
	state := decodeContract[map[string]any](t, waiterPhone.call("GET", "/restaurant/state", nil, nil, 200).Body.Bytes())
	if found := pending(state["accounts"].([]any)); found == nil || found["mode"] != "takeaway" {
		t.Fatalf("restaurant/state del mesero sin el pedido para llevar: %v", state["accounts"])
	}
	updated := decodeContract[map[string]any](t, waiterPhone.call("POST", "/restaurant/accounts/"+accountID+"/customer", map[string]any{
		"operation_id": contractID(), "customer_name": "Juan Pérez", "phone": "",
	}, nil, 200).Body.Bytes())
	if saved := updated["account"].(map[string]any); saved["customer_name"] != "Juan Pérez" || saved["phone"] != "" ||
		saved["revision"] != account["revision"] {
		t.Fatalf("datos del cliente: %v", saved)
	}
	deliver := "/payments/accounts/" + accountID + "/deliver"
	rejected := decodeContract[map[string]any](t, waiterPhone.call("POST", deliver, map[string]any{"operation_id": contractID()}, nil, 409).Body.Bytes())
	if rejected["error"].(map[string]any)["code"] != "BALANCE_PENDING" {
		t.Fatalf("entregar sin pagar: %v", rejected)
	}
	// La caja lo cobra (preparación, fuera del contrato: turno y cobro en efectivo).
	methods := decodeContract[[]map[string]any](t, client.setup("GET", "/payments/methods", nil, 200))
	cash := ""
	for _, method := range methods {
		if method["code"] == "cash" {
			cash = fmt.Sprint(method["id"])
		}
	}
	client.setup("POST", "/payments/shifts", map[string]any{"operation_id": contractID(), "opening_float": "0"}, 200)
	finance := decodeContract[map[string]any](t, client.setup("GET", "/payments/accounts/"+accountID, nil, 200))
	client.setup("POST", "/payments/accounts/"+accountID+"/payments", map[string]any{
		"operation_id": contractID(), "method_id": cash, "mode": "total", "expected_balance": finance["balance"],
	}, 200)
	delivered := decodeContract[map[string]any](t, waiterPhone.call("POST", deliver, map[string]any{"operation_id": contractID()}, nil, 200).Body.Bytes())
	if delivered["delivered"] != true || delivered["closed"] != true || delivered["finance"] != nil {
		t.Fatalf("entrega desde el teléfono: %v", delivered)
	}
}

// contractPrecheck es la precuenta impresa desde el teléfono (etapa A2, parte C): un mesero con payments.tickets.print
// entra con PIN; sin impresora del dispositivo printPrecheck es PRINTER_NOT_CONFIGURED con details.scope «device»; con
// la impresora asignada en Terminales (preparación, fuera del contrato) se encola, getIdentity la informa,
// getDocumentStatus sigue el trabajo y retryDocument lo reintenta cuando falló (409 PRINT_NOT_FAILED antes).
func contractPrecheck(t *testing.T, client *contractClient, phone *contractClient, data contractWaiterData) {
	role := decodeContract[map[string]any](t, client.setup("POST", "/roles", map[string]any{"name": "Meseros que imprimen contrato",
		"permissions": []string{"waiter_devices.use", "orders.confirm", "orders.capture", "accounts.read", "accounts.open",
			"catalog.read", "catalog.prices.read", "payments.tickets.print"}}, 200))
	waiter := decodeContract[map[string]any](t, client.setup("POST", "/users", map[string]any{"username": "dora.contrato",
		"display_name": "Dora del contrato", "role_id": role["id"], "active": true, "password": "clave-del-contrato-4"}, 200))
	waiterID := fmt.Sprint(waiter["id"])
	client.setup("PUT", "/users/"+waiterID+"/pin", map[string]any{"pin": "8316", "short_name": "Dora", "badge_color": ""}, 200)
	login := decodeContract[map[string]any](t, phone.call("POST", "/auth/pin-login",
		map[string]string{"user_id": waiterID, "pin": "8316"}, nil, 200).Body.Bytes())
	waiterPhone := *phone
	waiterPhone.session = fmt.Sprint(login["session_token"])
	printRoute := "/payments/accounts/" + data.accountID + "/precheck/print"
	rejected := waiterPhone.call("POST", printRoute, map[string]any{"operation_id": contractID()}, nil, 409).Body.Bytes()
	if err := client.contract.ValidateJSON(map[string]any{"$ref": "#/components/schemas/PrinterRejection"}, rejected); err != nil ||
		!strings.Contains(string(rejected), `"scope":"device"`) {
		t.Fatalf("precuenta sin impresora del dispositivo: %v %s", err, rejected)
	}
	// El administrador le asigna una impresora de red en Terminales (preparación, fuera del contrato).
	printer := decodeContract[map[string]any](t, client.setup("POST", "/kitchen/printers", map[string]any{"operation_id": contractID(),
		"name": "Precuentas contrato", "transport": "escpos_tcp", "destination": "127.0.0.1", "port": 9109, "paper_width": 80,
		"copies": 1, "active": true}, 200))
	client.setup("PATCH", "/devices/"+phone.device, map[string]any{"name": "Mesero Ana contrato", "register_id": nil,
		"keyboard_mode": "auto", "precheck_printer_id": printer["id"]}, 200)
	identity := decodeContract[map[string]any](t, waiterPhone.call("GET", "/auth/me", nil, nil, 200).Body.Bytes())
	if assigned, _ := identity["terminal"].(map[string]any)["precheck_printer"].(map[string]any); assigned == nil ||
		assigned["name"] != "Precuentas contrato" {
		t.Fatalf("impresora de precuentas del teléfono: %v", identity["terminal"])
	}
	printed := decodeContract[map[string]any](t, waiterPhone.call("POST", printRoute, map[string]any{"operation_id": contractID()}, nil, 200).Body.Bytes())
	if printed["printer_name"] != "Precuentas contrato" || printed["terminal_name"] != nil {
		t.Fatalf("precuenta encolada: %v", printed)
	}
	ticketID := fmt.Sprint(printed["ticket_id"])
	route := "/payments/accounts/" + data.accountID + "/documents/"
	waiterPhone.call("GET", route+"status?ticket_id="+ticketID, nil, nil, 200)
	retry := map[string]any{"operation_id": contractID(), "ticket_id": ticketID}
	waiterPhone.call("POST", route+"retry", retry, nil, 409)
	// La impresora no respondió (aquí no corre el trabajador de la cola: se deja el trabajo como lo dejaría).
	if _, err := client.pool.Exec(context.Background(), `UPDATE app.kitchen_print_jobs SET state='error',attempts=1,
        result='{"code":"unreachable"}' WHERE id=$1`, printed["job_id"]); err != nil {
		t.Fatal(err)
	}
	failed := decodeContract[map[string]any](t, waiterPhone.call("GET", route+"status?ticket_id="+ticketID, nil, nil, 200).Body.Bytes())
	if failed["job_state"] != "error" || failed["job_problem"] != "unreachable" {
		t.Fatalf("estado de la precuenta que no salió: %v", failed)
	}
	retry["operation_id"] = contractID()
	retried := decodeContract[map[string]any](t, waiterPhone.call("POST", route+"retry", retry, nil, 200).Body.Bytes())
	if retried["ticket_id"] != ticketID || retried["printer_name"] != "Precuentas contrato" {
		t.Fatalf("reintento: %v", retried)
	}
}

// contractReadyAlerts son los avisos de platillos listos (etapa A2, parte D): un mesero entra con PIN en el teléfono;
// la computadora principal le abre una cuenta y cocina (KDS) la marca «Listo» (preparación, fuera del contrato); el
// teléfono, solo con su credencial, recibe el aviso con getReadyAlerts y abre su canal streamReadyAlerts, cuyo primer
// mensaje cumple ReadyAlertsBatch.
func contractReadyAlerts(t *testing.T, client *contractClient, phone *contractClient, data contractWaiterData) {
	role := decodeContract[map[string]any](t, client.setup("POST", "/roles", map[string]any{"name": "Meseros con avisos contrato",
		"permissions": []string{"waiter_devices.use", "accounts.read", "accounts.open", "orders.capture", "orders.confirm"}}, 200))
	waiter := decodeContract[map[string]any](t, client.setup("POST", "/users", map[string]any{"username": "eva.contrato",
		"display_name": "Eva del contrato", "role_id": role["id"], "active": true, "password": "clave-del-contrato-5"}, 200))
	waiterID := fmt.Sprint(waiter["id"])
	client.setup("PUT", "/users/"+waiterID+"/pin", map[string]any{"pin": "5274", "short_name": "Eva", "badge_color": ""}, 200)
	phone.call("POST", "/auth/pin-login", map[string]string{"user_id": waiterID, "pin": "5274"}, nil, 200)
	// Cocina en pantalla para la categoría de las papas y una cuenta de Eva enviada a cocina (preparación).
	categoryID := ""
	if err := client.pool.QueryRow(context.Background(), "SELECT category_id::text FROM app.catalog_products WHERE id=$1",
		data.fries).Scan(&categoryID); err != nil {
		t.Fatal(err)
	}
	station := decodeContract[map[string]any](t, client.setup("POST", "/kitchen/stations", map[string]any{"operation_id": contractID(),
		"name": "Cocina contrato", "active": true, "reception": "kds", "target_minutes": 12, "warning_minutes": 15,
		"overdue_minutes": 25, "grouping": "inherit", "tracking": "inherit"}, 200))
	client.setup("POST", "/kitchen/routes", map[string]any{"operation_id": contractID(), "category_id": categoryID,
		"mode": "prepare", "station_ids": []any{station["id"]}}, 200)
	opened := decodeContract[map[string]any](t, client.setup("POST", "/restaurant/accounts", map[string]any{
		"operation_id": contractID(), "mode": "dine_in", "assigned_user_id": waiterID,
		"items": []map[string]any{{"id": contractID(), "product_id": data.fries, "variant_id": nil, "quantity": 2,
			"customization": data.customization}},
	}, 200))["account"].(map[string]any)
	accountID := fmt.Sprint(opened["id"])
	preview := decodeContract[map[string]any](t, client.setup("GET", "/restaurant/accounts/"+accountID+"/preview", nil, 200))
	client.setup("POST", "/restaurant/accounts/"+accountID+"/confirm", map[string]any{"operation_id": contractID(),
		"revision": opened["revision"], "expected_fingerprint": preview["fingerprint"]}, 200)
	rows, err := client.pool.Query(context.Background(), `SELECT t.id::text,t.revision FROM app.kitchen_tasks t
        JOIN app.restaurant_item_units u ON u.id=t.unit_id WHERE u.account_id=$1`, accountID)
	if err != nil {
		t.Fatal(err)
	}
	tasks := []map[string]any{}
	for rows.Next() {
		var id string
		var revision int
		if err := rows.Scan(&id, &revision); err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, map[string]any{"id": id, "revision": revision})
	}
	rows.Close()
	if len(tasks) != 2 {
		t.Fatalf("tareas de cocina: %v", tasks)
	}
	client.setup("POST", "/kitchen/states", map[string]any{"operation_id": contractID(), "state": "ready", "tasks": tasks}, 200)
	// El teléfono, solo con su credencial (phone no tiene sesión), recibe el aviso.
	batch := decodeContract[map[string]any](t, phone.call("GET", "/ready-alerts?after=0", nil, nil, 200).Body.Bytes())
	alerts, _ := batch["alerts"].([]any)
	if batch["active"] != true || len(alerts) != 1 {
		t.Fatalf("avisos del teléfono: %v", batch)
	}
	alert := alerts[0].(map[string]any)
	items, _ := alert["items"].([]any)
	if alert["account_id"] != accountID || alert["waiter_short_name"] != "Eva" || alert["key"] == "" || len(items) != 1 ||
		items[0].(map[string]any)["quantity"] != float64(2) || items[0].(map[string]any)["product_name"] != "Papas contrato" {
		t.Fatalf("aviso: %v", alert)
	}
	// El canal: el primer mensaje cumple el contrato.
	streamContext, stop := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer stop()
	stream := httptest.NewRecorder()
	phone.handler.ServeHTTP(stream, phone.request(streamContext, "GET", "/ready-alerts/stream?after=0", nil, nil))
	operation, _ := client.contract.Match("GET", "/ready-alerts/stream")
	client.check(operation, stream.Code, stream.Header().Get("Content-Type"), nil)
	scanner := bufio.NewScanner(bytes.NewReader(stream.Body.Bytes()))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	message := ""
	for scanner.Scan() {
		if data, found := strings.CutPrefix(scanner.Text(), "data: "); found {
			message = data
			break
		}
	}
	if err := client.contract.ValidateJSON(map[string]any{"$ref": "#/components/schemas/ReadyAlertsBatch"}, []byte(message)); err != nil ||
		!strings.Contains(message, accountID) {
		t.Fatalf("el mensaje del canal de avisos no cumple el contrato: %v (%q)", err, message)
	}
}

// contractKitchenDisplay recorre la pantalla de cocina (etapa K1, Fase A): se empareja como kitchen con X-Client-Kind
// kitchen, se conecta (cupo y configuración), lee sus tarjetas con el cursor, confirma las comandas, marca, lee los
// recientes, recupera, marca el pedido completo y comprueba la contraseña del administrador; el canal de eventos con su
// credencial cumple EventBatch. Una terminal de escritorio recibe 403 en sus rutas. Usa la estación «Cocina contrato»
// (recepción en pantalla) del recorrido de los avisos.
func contractKitchenDisplay(t *testing.T, client *contractClient, data contractWaiterData) {
	terminal := *client
	terminal.kind, terminal.version, terminal.session = "kitchen", "0.1.0", ""
	nonce := make([]byte, 32)
	_, _ = rand.Read(nonce)
	// La aplicación de cocina solo se empareja como pantalla de cocina, y una pantalla solo desde ella.
	wrong := terminal
	wrong.kind = "android"
	for _, attempt := range []*contractClient{&terminal, &wrong} {
		kind := "android"
		if attempt == &wrong {
			kind = "kitchen"
		}
		attempt.call("POST", "/pairing/requests", map[string]any{"name": "Pantalla equivocada", "device_kind": kind, "os": "Android 14",
			"app_version": "0.1.0", "commitment": administration.PairingCommitment(nonce)}, nil, 400)
	}
	created := decodeContract[map[string]string](t, terminal.call("POST", "/pairing/requests", map[string]any{
		"name": "Pantalla contrato", "device_kind": "kitchen", "os": "Android 14", "app_version": "0.1.0",
		"commitment": administration.PairingCommitment(nonce),
	}, nil, 200).Body.Bytes())
	requestID := created["request_id"]
	secret := map[string]string{"X-Pairing-Secret": created["pairing_secret"]}
	terminal.call("POST", "/pairing/requests/"+requestID+"/reveal", map[string]string{"terminal_nonce": hex.EncodeToString(nonce)}, secret, 200)
	serverNonce, err := hex.DecodeString(created["server_nonce"])
	if err != nil {
		t.Fatal(err)
	}
	code, err := administration.PairingCode(client.fingerprint, nonce, serverNonce, requestID)
	if err != nil {
		t.Fatal(err)
	}
	client.setup("POST", "/devices/pairing-requests/"+requestID+"/approve", map[string]string{"code": code}, 200)
	claimed := decodeContract[map[string]string](t, terminal.call("POST", "/pairing/requests/"+requestID+"/claim", map[string]any{}, secret, 200).Body.Bytes())
	display := terminal
	display.device, display.token = claimed["device_id"], claimed["device_token"]
	var kind string
	if err := client.pool.QueryRow(context.Background(), "SELECT kind FROM app.devices WHERE id=$1", display.device).Scan(&kind); err != nil || kind != "kitchen" {
		t.Fatalf("la pantalla emparejada: %q %v", kind, err)
	}
	connected := decodeContract[map[string]any](t, display.call("POST", "/kitchen-display/connect", map[string]any{}, nil, 200).Body.Bytes())
	if connected["seat"].(map[string]any)["held"] != true {
		t.Fatalf("conectar la pantalla: %v", connected)
	}
	before := decodeContract[map[string]any](t, display.call("GET", "/kitchen-display/orders", nil, nil, 200).Body.Bytes())
	cursor := int64(before["cursor"].(float64))
	// Un pedido de papas a la estación en pantalla.
	opened := decodeContract[map[string]any](t, client.setup("POST", "/restaurant/accounts", map[string]any{
		"operation_id": contractID(), "mode": "dine_in",
		"items": []map[string]any{{"id": contractID(), "product_id": data.fries, "variant_id": nil, "quantity": 1,
			"customization": data.customization}},
	}, 200))["account"].(map[string]any)
	accountID := fmt.Sprint(opened["id"])
	preview := decodeContract[map[string]any](t, client.setup("GET", "/restaurant/accounts/"+accountID+"/preview", nil, 200))
	client.setup("POST", "/restaurant/accounts/"+accountID+"/confirm", map[string]any{"operation_id": contractID(),
		"revision": opened["revision"], "expected_fingerprint": preview["fingerprint"]}, 200)
	view := decodeContract[map[string]any](t, display.call("GET", "/kitchen-display/orders", nil, nil, 200).Body.Bytes())
	tasks := []any{}
	for _, entry := range view["orders"].([]any) {
		order := entry.(map[string]any)
		if order["account_id"] != accountID {
			continue
		}
		for _, item := range order["items"].([]any) {
			tasks = append(tasks, item.(map[string]any)["tasks"].([]any)...)
		}
	}
	receipts, _ := view["receipts_pending"].([]any)
	if len(tasks) != 1 || len(receipts) == 0 {
		t.Fatalf("tarjetas de la pantalla: %v", view)
	}
	display.call("POST", "/kitchen-display/receipts", map[string]any{"receipts": receipts}, nil, 200)
	display.call("POST", "/kitchen-display/items/ready", map[string]any{"tasks": tasks}, nil, 200)
	display.call("GET", "/kitchen-display/recent", nil, nil, 200)
	display.call("POST", "/kitchen-display/recover", map[string]any{"tasks": tasks}, nil, 200)
	display.call("POST", "/kitchen-display/orders/ready", map[string]any{"tasks": tasks}, nil, 200)
	display.call("POST", "/kitchen-display/settings-access", map[string]string{"username": "admin", "password": "admin"}, nil, 200)
	// La terminal de escritorio no usa las rutas de la pantalla, y la pantalla no usa las de la sesión.
	client.call("GET", "/kitchen-display/orders", nil, nil, 403)
	display.call("GET", "/restaurant/state", nil, nil, 403)
	// El canal de eventos con la credencial de la pantalla: el primer mensaje cumple EventBatch.
	streamContext, stop := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer stop()
	stream := httptest.NewRecorder()
	display.handler.ServeHTTP(stream, display.request(streamContext, "GET", fmt.Sprintf("/events?after=%d", cursor), nil, nil))
	operation, _ := client.contract.Match("GET", "/events")
	client.check(operation, stream.Code, stream.Header().Get("Content-Type"), nil)
	scanner := bufio.NewScanner(bytes.NewReader(stream.Body.Bytes()))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	message := ""
	for scanner.Scan() {
		if data, found := strings.CutPrefix(scanner.Text(), "data: "); found {
			message = data
			break
		}
	}
	if err := client.contract.ValidateJSON(map[string]any{"$ref": "#/components/schemas/EventBatch"}, []byte(message)); err != nil ||
		!strings.Contains(message, "kitchen.ticket.created") {
		t.Fatalf("el canal de la pantalla no cumple el contrato: %v (%.600s)", err, message)
	}
	// K1-D32: «Volver a emparejar» revoca la pantalla (al final: después ya nada le responde).
	display.call("POST", "/kitchen-display/unpair", map[string]string{"username": "admin", "password": "otra"}, nil, 401)
	display.call("POST", "/kitchen-display/unpair", map[string]string{"username": "admin", "password": "admin"}, nil, 200)
	display.call("GET", "/kitchen-display/orders", nil, nil, 401)
	client.call("POST", "/kitchen-display/unpair", map[string]string{"username": "admin", "password": "admin"}, nil, 403)
}

// contractKitchenCoverage recorre la configuración guiada de cocina (etapa K1.1, Fase A; K11-D3 a K11-D5): la cobertura
// del enrutamiento con un producto sin ruta, el tablero con las estaciones desactivadas de un aviso pendiente, «Productos sin ruta» con su valor esperado (409 KITCHEN_UNROUTED_CHANGED y
// 400 con una estación que no existe), «Enviar a cocina ahora» de un pedido programado (y su 409
// KITCHEN_RELEASE_NOT_PENDING) y «Volver a avisar» de un aviso para una estación desactivada (y el 409
// KITCHEN_CHANGE_NOT_PENDING de un aviso que se dio por terminado al cancelar su cuenta). Usa la estación «Cocina
// contrato» (recepción en pantalla) y la ruta de la categoría «Contrato» del recorrido de los avisos.
func contractKitchenCoverage(t *testing.T, client *contractClient, data contractWaiterData) {
	ctx := context.Background()
	refrescos := decodeContract[map[string]any](t, client.setup("POST", "/catalog/categories",
		map[string]any{"name": "Refrescos contrato", "active": true}, 200))
	soda := decodeContract[map[string]any](t, client.setup("POST", "/catalog/products", map[string]any{
		"name": "Refresco contrato", "active": true, "category_id": refrescos["id"], "code": "REFRESCO-CONTRATO", "kind": "product",
		"base_price": "30",
	}, 200))
	var stationID string
	if err := client.pool.QueryRow(ctx, "SELECT id::text FROM app.kitchen_stations WHERE name='Cocina contrato'").Scan(&stationID); err != nil {
		t.Fatal(err)
	}
	coverage := decodeContract[map[string]any](t, client.call("GET", "/kitchen/coverage", nil, nil, 200).Body.Bytes())
	unrouted := coverage["unrouted"].(map[string]any)
	if unrouted["effective"] != "none" || unrouted["station_id"] != nil {
		t.Fatalf("«Productos sin ruta» por omisión: %v", unrouted)
	}
	foundSoda := false
	for _, entry := range coverage["unrouted_items"].([]any) {
		group := entry.(map[string]any)
		products, _ := group["products"].([]any)
		if group["category_id"] == refrescos["id"] && group["whole_category"] == true && len(products) == 1 &&
			products[0].(map[string]any)["id"] == soda["id"] && products[0].(map[string]any)["reason"] == "no_route" {
			foundSoda = true
		}
	}
	if !foundSoda {
		t.Fatalf("el refresco sin ruta no está en unrouted_items: %v", coverage["unrouted_items"])
	}

	// «Productos sin ruta»: a la estación, con su valor esperado.
	save := map[string]any{"operation_id": contractID(), "station_id": stationID, "expected_station_id": nil}
	saved := decodeContract[map[string]any](t, client.call("PUT", "/kitchen/unrouted", save, nil, 200).Body.Bytes())
	if saved["effective"] != "station" || saved["station_id"] != stationID || saved["station_name"] != "Cocina contrato" {
		t.Fatalf("«Enviar a una estación»: %v", saved)
	}
	repeated := decodeContract[map[string]any](t, client.call("PUT", "/kitchen/unrouted", save, nil, 200).Body.Bytes())
	if repeated["station_id"] != stationID {
		t.Fatalf("la repetición devolvió otra respuesta: %v", repeated)
	}
	stale := decodeContract[map[string]any](t, client.call("PUT", "/kitchen/unrouted", map[string]any{"operation_id": contractID(),
		"station_id": nil, "expected_station_id": nil}, nil, 409).Body.Bytes())
	if stale["error"].(map[string]any)["code"] != "KITCHEN_UNROUTED_CHANGED" {
		t.Fatalf("valor esperado viejo: %v", stale)
	}
	client.call("PUT", "/kitchen/unrouted", map[string]any{"operation_id": contractID(), "station_id": contractID(),
		"expected_station_id": stationID}, nil, 400)
	client.call("PUT", "/kitchen/unrouted", map[string]any{"operation_id": contractID(), "station_id": nil,
		"expected_station_id": stationID}, nil, 200)

	open := func(body map[string]any, product string) string {
		t.Helper()
		body["operation_id"] = contractID()
		body["items"] = []map[string]any{{"id": contractID(), "product_id": product, "variant_id": nil, "quantity": 1,
			"customization": data.customization}}
		opened := decodeContract[map[string]any](t, client.setup("POST", "/restaurant/accounts", body, 200))["account"].(map[string]any)
		accountID := fmt.Sprint(opened["id"])
		preview := decodeContract[map[string]any](t, client.setup("GET", "/restaurant/accounts/"+accountID+"/preview", nil, 200))
		client.setup("POST", "/restaurant/accounts/"+accountID+"/confirm", map[string]any{"operation_id": contractID(),
			"revision": opened["revision"], "expected_fingerprint": preview["fingerprint"]}, 200)
		return accountID
	}
	// Un pedido para llevar programado, enviado a cocina antes de su hora.
	location, err := time.LoadLocation("America/Monterrey")
	if err != nil {
		t.Fatal(err)
	}
	scheduled := open(map[string]any{"mode": "takeaway", "name": "Programado contrato", "origin": "Ventanilla",
		"scheduled_local": time.Now().In(location).Add(3 * time.Hour).Format("2006-01-02T15:04"), "schedule_kind": "delivery"}, data.fries)
	var releaseID string
	var revision int
	if err := client.pool.QueryRow(ctx, "SELECT id::text,revision FROM app.kitchen_releases WHERE account_id=$1 AND state='scheduled'",
		scheduled).Scan(&releaseID, &revision); err != nil {
		t.Fatalf("pedido programado: %v", err)
	}
	release := map[string]any{"operation_id": contractID(), "revision": revision}
	sent := decodeContract[map[string]any](t, client.call("POST", "/kitchen/releases/"+releaseID+"/release", release, nil, 200).Body.Bytes())
	if sent["state"] != "released" || sent["released_at"] == nil {
		t.Fatalf("«Enviar a cocina ahora»: %v", sent)
	}
	client.call("POST", "/kitchen/releases/"+releaseID+"/release", release, nil, 200)
	again := decodeContract[map[string]any](t, client.call("POST", "/kitchen/releases/"+releaseID+"/release",
		map[string]any{"operation_id": contractID(), "revision": sent["revision"]}, nil, 409).Body.Bytes())
	if again["error"].(map[string]any)["code"] != "KITCHEN_RELEASE_NOT_PENDING" {
		t.Fatalf("enviar otra vez: %v", again)
	}

	// Un aviso para la estación desactivada que ya tenía la comanda: se vuelve a avisar al reactivarla.
	var station map[string]any
	for _, entry := range decodeContract[[]map[string]any](t, client.setup("GET", "/kitchen/stations", nil, 200)) {
		if entry["id"] == stationID {
			station = entry
		}
	}
	setActive := func(active bool) {
		t.Helper()
		body := map[string]any{"operation_id": contractID()}
		for _, key := range []string{"id", "name", "description", "sort_order", "reception", "printer_id", "target_minutes",
			"warning_minutes", "overdue_minutes", "grouping", "tracking", "revision"} {
			body[key] = station[key]
		}
		body["active"] = active
		station = decodeContract[map[string]any](t, client.setup("PATCH", "/kitchen/stations/"+stationID, body, 200))
	}
	pendingChange := func(accountID string) (string, int) {
		t.Helper()
		var eventID string
		var changeRevision int
		if err := client.pool.QueryRow(ctx, "SELECT event_id::text,revision FROM app.kitchen_pending_changes WHERE account_id=$1",
			accountID).Scan(&eventID, &changeRevision); err != nil {
			t.Fatalf("aviso pendiente: %v", err)
		}
		return eventID, changeRevision
	}
	modified := open(map[string]any{"mode": "dine_in"}, data.fries)
	setActive(false)
	bill := decodeContract[map[string]any](t, client.setup("GET", "/restaurant/accounts/"+modified, nil, 200))
	line := bill["lines"].([]any)[0].(map[string]any)
	client.setup("POST", "/restaurant/accounts/"+modified+"/cancel", map[string]any{"operation_id": contractID(),
		"revision": bill["account"].(map[string]any)["revision"], "version_id": line["version_id"], "quantity": 1,
		"reason": "El cliente ya no lo quiso"}, 200)
	eventID, changeRevision := pendingChange(modified)
	// El panel «Por enviar a cocina» dice a qué estación desactivada no se pudo avisar.
	board := decodeContract[map[string]any](t, client.call("GET", "/kitchen/board", nil, nil, 200).Body.Bytes())
	inactive := []string{}
	for _, entry := range board["pending_changes"].([]any) {
		change := entry.(map[string]any)
		if change["event_id"] != eventID {
			continue
		}
		stations, _ := change["inactive_stations"].([]any)
		for _, station := range stations {
			inactive = append(inactive, fmt.Sprint(station.(map[string]any)["name"]))
		}
	}
	if !slices.Equal(inactive, []string{"Cocina contrato"}) {
		t.Fatalf("estaciones desactivadas del aviso pendiente: %v (%v)", inactive, board["pending_changes"])
	}
	setActive(true)
	client.call("POST", "/kitchen/changes/"+eventID+"/retry", map[string]any{"operation_id": contractID(), "revision": changeRevision}, nil, 200)
	// Cancelar la cuenta da por terminado su aviso pendiente.
	cancelled := open(map[string]any{"mode": "dine_in"}, data.fries)
	setActive(false)
	reasonID := ""
	for _, reason := range decodeContract[[]map[string]any](t, client.setup("GET", "/restaurant/cancel-reasons", nil, 200)) {
		if reason["active"] == true {
			reasonID = fmt.Sprint(reason["id"])
		}
	}
	bill = decodeContract[map[string]any](t, client.setup("GET", "/restaurant/accounts/"+cancelled, nil, 200))
	client.setup("POST", "/restaurant/accounts/"+cancelled+"/cancel-account", map[string]any{"operation_id": contractID(),
		"revision": bill["account"].(map[string]any)["revision"], "reason_id": reasonID, "description": "Cuenta del contrato"}, 200)
	eventID, changeRevision = pendingChange(cancelled)
	finished := decodeContract[map[string]any](t, client.call("POST", "/kitchen/changes/"+eventID+"/retry",
		map[string]any{"operation_id": contractID(), "revision": changeRevision}, nil, 409).Body.Bytes())
	if finished["error"].(map[string]any)["code"] != "KITCHEN_CHANGE_NOT_PENDING" {
		t.Fatalf("aviso de una cuenta cancelada: %v", finished)
	}
	setActive(true)
}

// contractTableUnions recorre las uniones de mesas y las mesas divisibles (etapa M1, Fase A): la vista previa, unir con
// todas libres (409 TABLE_UNION_DEFERRED) y la unión que nace con el primer producto (openAccount con union_table_ids),
// la unión con su historial, agregar y quitar mesas, unir desde una mesa ocupada y crear, cambiar y quitar un grupo. Las
// lecturas con unions, groups, union_id, group_id, table_union y preparation_required se validan en los pasos de siempre
// (overview y restaurant/state) y aquí.
func contractTableUnions(t *testing.T, client *contractClient, data contractWaiterData) {
	area := decodeContract[map[string]any](t, client.setup("POST", "/restaurant/areas",
		map[string]any{"operation_id": contractID(), "name": "Uniones contrato", "active": true}, 200))
	tables := []string{}
	for index := range 5 {
		created := decodeContract[map[string]any](t, client.setup("POST", "/restaurant/tables", map[string]any{
			"operation_id": contractID(), "area_id": area["id"], "name": fmt.Sprintf("Mesa U%d", index+1), "shape": "rectangle",
			"x": 20 + index*140, "y": 30, "width": 100, "height": 72, "seats": 4, "active": true,
		}, 200))
		tables = append(tables, fmt.Sprint(created["id"]))
	}
	item := func() []map[string]any {
		return []map[string]any{{"id": contractID(), "product_id": data.fries, "variant_id": nil, "quantity": 1,
			"customization": data.customization}}
	}
	preview := decodeContract[map[string]any](t, client.call("POST", "/restaurant/table-unions/preview",
		map[string]any{"table_ids": []string{tables[1], tables[0]}}, nil, 200).Body.Bytes())
	if preview["name"] != "Mesa U1 + U2" || len(preview["problems"].([]any)) != 0 {
		t.Fatalf("vista previa: %v", preview)
	}
	deferred := decodeContract[map[string]any](t, client.call("POST", "/restaurant/table-unions", map[string]any{
		"operation_id": contractID(), "start_table_id": tables[0], "table_ids": []string{tables[1]}}, nil, 409).Body.Bytes())
	if deferred["error"].(map[string]any)["code"] != "TABLE_UNION_DEFERRED" {
		t.Fatalf("unir libres: %v", deferred)
	}
	opened := decodeContract[map[string]any](t, client.call("POST", "/restaurant/accounts", map[string]any{
		"operation_id": contractID(), "mode": "dine_in", "table_id": tables[0], "union_table_ids": []string{tables[1]},
		"seen_union_id": nil, "seen_account_ids": []string{}, "items": item()}, nil, 200).Body.Bytes())
	union := opened["account"].(map[string]any)["table_union"].(map[string]any)
	unionID := fmt.Sprint(union["id"])
	detail := decodeContract[map[string]any](t, client.call("GET", "/restaurant/table-unions/"+unionID, nil, nil, 200).Body.Bytes())
	revision := detail["union"].(map[string]any)["revision"]
	added := decodeContract[map[string]any](t, client.call("POST", "/restaurant/table-unions/"+unionID+"/tables", map[string]any{
		"operation_id": contractID(), "revision": revision, "table_id": tables[2]}, nil, 200).Body.Bytes())
	removed := decodeContract[map[string]any](t, client.call("POST",
		"/restaurant/table-unions/"+unionID+"/tables/"+tables[1]+"/remove", map[string]any{"operation_id": contractID(),
			"revision": added["union"].(map[string]any)["revision"]}, nil, 200).Body.Bytes())
	if removed["dissolved"] != false || removed["union"].(map[string]any)["name"] != "Mesa U1 + U3" {
		t.Fatalf("quitar una mesa: %v", removed)
	}
	// M1-D32: abrir en la mesa que salió con la unión vista (que sigue abierta sin ella) responde TABLE_UNION_CONFLICT.
	conflicted := decodeContract[map[string]any](t, client.call("POST", "/restaurant/accounts", map[string]any{
		"operation_id": contractID(), "mode": "dine_in", "table_id": tables[1], "seen_union_id": unionID,
		"seen_account_ids": []string{}, "items": item()}, nil, 409).Body.Bytes())
	if conflicted["error"].(map[string]any)["code"] != "TABLE_UNION_CONFLICT" {
		t.Fatalf("seen_union_id de una unión abierta sin la mesa: %v", conflicted)
	}
	// Unir desde una mesa ocupada.
	occupied := decodeContract[map[string]any](t, client.setup("POST", "/restaurant/accounts", map[string]any{
		"operation_id": contractID(), "mode": "dine_in", "table_id": tables[3], "items": item()}, 200))
	grown := decodeContract[map[string]any](t, client.call("POST", "/restaurant/table-unions", map[string]any{
		"operation_id": contractID(), "start_table_id": tables[3], "table_ids": []string{tables[4]}}, nil, 200).Body.Bytes())
	if grown["union"].(map[string]any)["name"] != "Mesa U4 + U5" {
		t.Fatalf("crecer una mesa ocupada: %v", grown)
	}
	client.call("GET", "/restaurant/overview", nil, nil, 200)
	client.call("GET", "/restaurant/state", nil, nil, 200)
	// Las cuentas se descartan (solo tienen borrador): las uniones se deshacen solas.
	for _, account := range []map[string]any{opened["account"].(map[string]any), occupied["account"].(map[string]any)} {
		current := decodeContract[map[string]any](t, client.setup("GET", "/restaurant/accounts/"+fmt.Sprint(account["id"]), nil, 200))
		client.setup("POST", "/restaurant/accounts/"+fmt.Sprint(account["id"])+"/discard", map[string]any{
			"operation_id": contractID(), "revision": current["account"].(map[string]any)["revision"]}, 200)
	}
	// Mesas divisibles.
	group := decodeContract[map[string]any](t, client.call("POST", "/restaurant/table-groups", map[string]any{
		"operation_id": contractID(), "area_id": area["id"], "name": "Mesa U", "table_ids": []string{tables[0], tables[1], tables[2]},
	}, nil, 200).Body.Bytes())["group"].(map[string]any)
	changed := decodeContract[map[string]any](t, client.call("PATCH", "/restaurant/table-groups/"+fmt.Sprint(group["id"]), map[string]any{
		"operation_id": contractID(), "revision": group["revision"], "name": "Mesa en U"}, nil, 200).Body.Bytes())["group"].(map[string]any)
	client.call("PATCH", "/restaurant/table-groups/"+fmt.Sprint(group["id"]), map[string]any{
		"operation_id": contractID(), "revision": changed["revision"], "active": false}, nil, 200)
}
