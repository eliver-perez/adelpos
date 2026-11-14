//go:build integration

package apicontract_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
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
	covered  map[string][]int
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
	request.Header.Set(httpapi.ClientKindHeader, "android")
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
	service := &administration.Service{Pool: pool, ProvisioningFile: filename}
	handler := httpapi.New(func(ctx context.Context) database.Health {
		return database.Probe(ctx, pool)
	}, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)), service)
	contract, err := apicontract.Load()
	if err != nil {
		t.Fatal(err)
	}
	client := &contractClient{t: t, contract: contract, handler: handler, version: product.Read().Version, covered: map[string][]int{}}
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
	client.call("GET", fmt.Sprintf("/catalog/sync?since=%d", version), nil, nil, 200)
	defaultList := full["price_list"].(map[string]any)["id"]
	client.call("GET", fmt.Sprintf("/catalog/products?category_id=%s&price_list_id=%s", category["id"], defaultList), nil, nil, 200)
	client.call("GET", "/catalog/images/"+imageID, nil, nil, 200)
	content := client.call("GET", "/catalog/images/"+imageID+"/content", nil, nil, 200)
	client.call("GET", "/catalog/images/"+imageID+"/content", nil, map[string]string{"If-None-Match": content.Header().Get("ETag")}, 304)

	// «Agotado» y «Disponible otra vez».
	client.call("POST", fmt.Sprintf("/catalog/products/%s/sold-out", fries["id"]),
		map[string]any{"operation_id": contractID(), "sold_out": true, "indefinite": true}, nil, 200)
	client.call("GET", fmt.Sprintf("/catalog/sync?since=%d", version), nil, nil, 200)
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
	client.call("POST", "/restaurant/offline-orders", stale, nil, 200)

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

	// Recuperación del estado y cierre de sesión.
	state := decodeContract[map[string]any](t, client.call("GET", "/restaurant/state", nil, nil, 200).Body.Bytes())
	if accounts, _ := state["accounts"].([]any); len(accounts) < 2 {
		t.Fatalf("el estado no trae las cuentas abiertas: %v", state["accounts"])
	}
	client.call("POST", "/auth/logout", nil, nil, 200)
}
