package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"local/ad-el-pos/server/internal/database"
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
	if problem["code"] != "CLIENT_UPDATE_REQUIRED" || details["min_client_version"] != metadata.MinClientVersion ||
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
