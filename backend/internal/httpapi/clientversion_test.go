package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/lanlistener"
	"local/ad-el-pos/server/internal/product"
)

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		left, right string
		expected    int
	}{
		{"0.10.0", "0.9.5", 1},
		{"0.9.5", "0.10.0", -1},
		{"0.10.0", "0.10.0", 0},
		{"1.0.0-beta.2", "1.0.0", -1},
		{"1.0.0", "1.0.0-beta.2", 1},
		{"1.0.0+build.7", "1.0.0", 0},
		{"2.0.0", "10.0.0", -1},
	} {
		result, ok := CompareVersions(tc.left, tc.right)
		if !ok || result != tc.expected {
			t.Fatalf("%s frente a %s: %d %v", tc.left, tc.right, result, ok)
		}
	}
	for _, invalid := range []string{"", "1", "1.2", "1.2.3.4", "v1.2.3", "1.2.x", "1..3", "1.2.3-", "1234567.0.0", "1.2.3 "} {
		if _, ok := CompareVersions(invalid, "0.9.5"); ok {
			t.Fatalf("se aceptó la versión %q", invalid)
		}
	}
}

// Etapa 9G (§5.1): un cliente anterior a la versión mínima recibe «Actualiza esta terminal» (426) en las rutas de la
// aplicación, pero siempre puede consultar la versión, la salud y el contrato; sin cabecera se acepta (herramientas
// y pruebas) y una cabecera inválida se rechaza.
func TestClientVersionCheck(t *testing.T) {
	h := handler(database.Health{Postgres: "available", Schema: "compatible"})
	metadata := product.Read()
	request := func(route string, headers map[string]string) (int, map[string]any) {
		r := httptest.NewRequest("GET", route, nil)
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}
	old := map[string]string{ClientVersionHeader: "0.0.1", ClientKindHeader: "android"}
	status, body := request("/api/v1/catalog/sync", old)
	if status != 426 {
		t.Fatalf("cliente viejo: %d", status)
	}
	problem := body["error"].(map[string]any)
	details := problem["details"].(map[string]any)
	if problem["code"] != "CLIENT_UPDATE_REQUIRED" || details["min_client_version"] != metadata.MinimumFor("android") ||
		details["client_version"] != "0.0.1" || problem["request_id"] == "" {
		t.Fatalf("rechazo del cliente viejo: %v", body)
	}
	if status, _ := request("/api/v1/events", old); status != 426 {
		t.Fatalf("el canal de eventos aceptó un cliente viejo: %d", status)
	}
	for _, route := range []string{"/api/v1/version", "/api/v1/health/live", "/api/v1/health/ready", "/api/v1/openapi.json"} {
		if status, _ := request(route, old); status != 200 {
			t.Fatalf("%s rechazó al cliente viejo: %d", route, status)
		}
	}
	// Sin cabecera (herramientas y pruebas) y con la versión vigente pasa a la aplicación (aquí, sin aplicación: 404).
	current := map[string]string{ClientVersionHeader: metadata.Version, ClientKindHeader: "desktop"}
	for _, headers := range []map[string]string{nil, current, {ClientVersionHeader: "99.0.0"}} {
		if status, _ := request("/api/v1/catalog/sync", headers); status != 404 {
			t.Fatalf("cliente aceptado %v: %d", headers, status)
		}
	}
	for _, headers := range []map[string]string{
		{ClientVersionHeader: "nueva"},
		{ClientVersionHeader: metadata.Version, ClientKindHeader: "reloj"},
		{ClientKindHeader: "tablet"},
	} {
		if status, body := request("/api/v1/catalog/sync", headers); status != 400 {
			t.Fatalf("cabecera inválida %v: %d %v", headers, status, body)
		}
	}
	status, version := request("/api/v1/version", nil)
	if status != 200 || version["min_client_version"] != metadata.MinClientVersion || version["openapi_path"] != "/api/v1/openapi.json" {
		t.Fatalf("versión: %v", version)
	}
	kinds, _ := version["client_kinds"].([]any)
	if len(kinds) != 2 {
		t.Fatalf("tipos de cliente: %v", version["client_kinds"])
	}
}

// Etapa 9G, fase C: por la escucha de red la versión del cliente es obligatoria (426 sin ella); las rutas técnicas
// siguen respondiendo y por la escucha local nada cambia.
func TestClientVersionIsRequiredOverTheNetwork(t *testing.T) {
	h := handler(database.Health{Postgres: "available", Schema: "compatible"})
	metadata := product.Read()
	request := func(route string, network bool, headers map[string]string) (int, map[string]any) {
		r := httptest.NewRequest("GET", route, nil)
		if network {
			r = r.WithContext(lanlistener.WithNetwork(r.Context()))
		}
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}
	for _, route := range []string{"/api/v1/catalog/sync", "/api/v1/pairing/requests", "/api/v1/events"} {
		status, body := request(route, true, map[string]string{ClientKindHeader: "android"})
		if status != 426 {
			t.Fatalf("%s sin versión por la red: %d %v", route, status, body)
		}
		problem := body["error"].(map[string]any)
		details := problem["details"].(map[string]any)
		if problem["code"] != "CLIENT_UPDATE_REQUIRED" || details["min_client_version"] != metadata.MinimumFor("android") {
			t.Fatalf("rechazo sin versión: %v", body)
		}
		// Por la escucha local sin cabecera se sigue aceptando (aquí, sin aplicación: 404, o 501 sin eventos).
		if status, _ := request(route, false, nil); status == 426 || status == 400 {
			t.Fatalf("%s sin versión por la escucha local: %d", route, status)
		}
	}
	current := map[string]string{ClientVersionHeader: metadata.Version, ClientKindHeader: "desktop"}
	if status, _ := request("/api/v1/catalog/sync", true, current); status != 404 {
		t.Fatalf("cliente vigente por la red: %d", status)
	}
	for _, route := range []string{"/api/v1/version", "/api/v1/health/live", "/api/v1/health/ready", "/api/v1/openapi.json"} {
		if status, _ := request(route, true, nil); status != 200 {
			t.Fatalf("%s por la red sin versión: %d", route, status)
		}
	}
}

// Etapa A1 (decisión A1-D4): la versión mínima es por tipo de cliente. La aplicación Android 0.1.0 pasa aunque sea
// anterior a la mínima del escritorio (0.10.0); un escritorio anterior a la suya y un Android anterior a la suya
// reciben 426 con la mínima de su tipo; sin tipo se exige la general (la del escritorio). GET version informa ambas y
// conserva min_client_version con la del escritorio.
func TestClientMinimumVersionByKind(t *testing.T) {
	h := handler(database.Health{Postgres: "available", Schema: "compatible"})
	metadata := product.Read()
	if metadata.MinimumFor("desktop") != "0.10.0" || metadata.MinimumFor("android") != "0.1.0" ||
		metadata.MinClientVersion != metadata.MinimumFor("desktop") || metadata.MinimumFor("") != metadata.MinClientVersion {
		t.Fatalf("versiones mínimas de product.json: %+v", metadata)
	}
	request := func(network bool, headers map[string]string) (int, map[string]any) {
		r := httptest.NewRequest("GET", "/api/v1/restaurant/state", nil)
		if network {
			r = r.WithContext(lanlistener.WithNetwork(r.Context()))
		}
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}
	for _, network := range []bool{false, true} {
		for _, tc := range []struct {
			kind, version string
			status        int
			minimum       string
		}{
			{"android", "0.1.0", 404, ""},
			{"android", "0.1.7", 404, ""},
			{"android", "0.0.9", 426, "0.1.0"},
			{"android", "0.1.0-beta.1", 426, "0.1.0"},
			{"desktop", "0.10.0", 404, ""},
			{"desktop", "0.11.0", 404, ""},
			{"desktop", "0.9.9", 426, "0.10.0"},
			{"desktop", "0.1.0", 426, "0.10.0"},
			{"", "0.1.0", 426, "0.10.0"},
			{"", metadata.Version, 404, ""},
		} {
			headers := map[string]string{ClientVersionHeader: tc.version}
			if tc.kind != "" {
				headers[ClientKindHeader] = tc.kind
			}
			status, body := request(network, headers)
			if status != tc.status {
				t.Fatalf("%q %s (red %v): %d %v", tc.kind, tc.version, network, status, body)
			}
			if tc.status == 426 {
				details := body["error"].(map[string]any)["details"].(map[string]any)
				if details["min_client_version"] != tc.minimum || details["client_version"] != tc.version {
					t.Fatalf("%q %s: detalles del rechazo %v", tc.kind, tc.version, details)
				}
			}
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/version", nil))
	version := map[string]any{}
	if err := json.Unmarshal(w.Body.Bytes(), &version); err != nil || w.Code != 200 {
		t.Fatalf("versión: %d %v", w.Code, err)
	}
	minimums, _ := version["min_client_versions"].(map[string]any)
	if version["min_client_version"] != "0.10.0" || minimums["desktop"] != "0.10.0" || minimums["android"] != "0.1.0" ||
		len(minimums) != len(ClientKinds) {
		t.Fatalf("versiones mínimas informadas: %v %v", version["min_client_version"], version["min_client_versions"])
	}
	capabilities, _ := version["capabilities"].([]any)
	found := map[string]bool{}
	for _, capability := range capabilities {
		found[capability.(string)] = true
	}
	if !found["waiter-pin-v1"] || !found["client-versions-by-kind"] || !found["waiter-mode-v1"] {
		t.Fatalf("capacidades de la etapa A1: %v", capabilities)
	}
}

func TestOpenAPIDocumentIsServed(t *testing.T) {
	h := handler(database.Health{Postgres: "available", Schema: "compatible"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/openapi.json", nil))
	var document map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &document) != nil || document["openapi"] != "3.1.0" {
		t.Fatalf("contrato: %d %.200s", w.Code, w.Body.String())
	}
	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest("POST", "/api/v1/openapi.json", nil))
	if post.Code != 405 {
		t.Fatalf("el contrato admite escritura: %d", post.Code)
	}
}

// Etapa A1 (revisión R3, B1): la credencial de envío del mesero solo vale en POST restaurant/offline-orders. En cualquier
// otra ruta, también las técnicas (versión, salud, contrato) y el canal de eventos, el servidor responde 401
// OFFLINE_CREDENTIAL_NOT_ALLOWED antes de atenderla; en su ruta pasa a la aplicación (aquí, sin aplicación: 405 o 404).
func TestOfflineCredentialOnlyOnItsRoute(t *testing.T) {
	h := handler(database.Health{Postgres: "available", Schema: "compatible"})
	send := func(method, route string, credential bool) (int, map[string]any) {
		r := httptest.NewRequest(method, route, nil)
		if credential {
			r.Header.Set(OfflineCredentialHeader, "credencial-de-prueba-de-43-caracteres-aaaaa")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}
	for _, route := range [][2]string{
		{"GET", "/api/v1/version"}, {"GET", "/api/v1/health/live"}, {"GET", "/api/v1/health/ready"},
		{"GET", "/api/v1/openapi.json"}, {"GET", "/api/v1/events"}, {"GET", "/api/v1/setup/status"},
		{"POST", "/api/v1/auth/pin-login"}, {"GET", "/api/v1/restaurant/offline-orders"},
		{"POST", "/api/v1/restaurant/offline-orders/"}, {"GET", "/missing"},
	} {
		status, body := send(route[0], route[1], true)
		problem, _ := body["error"].(map[string]any)
		if status != 401 || problem["code"] != "OFFLINE_CREDENTIAL_NOT_ALLOWED" {
			t.Fatalf("%s %s con la credencial: %d %v", route[0], route[1], status, body)
		}
		if status, _ := send(route[0], route[1], false); status == 401 {
			t.Fatalf("%s %s sin la credencial no debía responder 401", route[0], route[1])
		}
	}
	if status, body := send("POST", "/api/v1/restaurant/offline-orders", true); status == 401 {
		t.Fatalf("la ruta de los pedidos sin conexión rechazó la credencial: %v", body)
	}
}
