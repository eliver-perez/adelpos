//go:build !adelpos_production

package licensetest

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/licensing"
)

// El servidor simulado cumple lo que el cliente necesita del contrato (§8 y openapi.yaml): errores de transporte,
// campos exactos, desafío de un solo uso, reintento idéntico con la misma respuesta y REQUEST_CONFLICT.
func TestSimulatedServerFollowsTheContract(t *testing.T) {
	fixtures, err := LoadFixtures("../../../contracts/licencias/v1/fixtures/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(fixtures)
	listener, roots := server.StartTLS()
	defer listener.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}
	post := func(path, contentType string, body any) (int, http.Header, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		request, _ := http.NewRequest(http.MethodPost, listener.URL+path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", contentType)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		payload, _ := io.ReadAll(response.Body)
		var decoded map[string]any
		_ = json.Unmarshal(payload, &decoded)
		return response.StatusCode, response.Header, decoded
	}
	errorCode := func(body map[string]any) string {
		inner, _ := body["error"].(map[string]any)
		code, _ := inner["code"].(string)
		return code
	}

	// Errores de transporte.
	if status, _, body := post("/v1/no-existe", "application/json", map[string]any{}); status != 404 || errorCode(body) != licensing.CodeInvalidRequest {
		t.Fatalf("ruta inexistente: %d %v", status, body)
	}
	response, err := client.Get(listener.URL + "/v1/activations")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 405 || response.Header.Get("Allow") != "POST" {
		t.Fatalf("otro método: %d", response.StatusCode)
	}
	if status, _, _ := post("/v1/activations", "text/plain", map[string]any{}); status != 415 {
		t.Fatalf("otro tipo de contenido: %d", status)
	}
	if status, _, _ := post("/v1/activations", "application/json", map[string]string{"relleno": strings.Repeat("x", 17<<10)}); status != 413 {
		t.Fatalf("más de 16 KiB: %d", status)
	}

	installation := fixtures.Installations["installation-fixture-a"]
	challenge := func() map[string]any {
		status, _, body := post("/v1/activations/challenge", "application/json", map[string]any{
			"action": "activate", "product_id": "adel_pos", "server_id": installation.ServerID, "activation_id": nil,
		})
		if status != 200 {
			t.Fatalf("desafío: %d %v", status, body)
		}
		return body
	}
	// Un campo sobrante se rechaza.
	if status, _, body := post("/v1/activations/challenge", "application/json", map[string]any{
		"action": "activate", "product_id": "adel_pos", "server_id": installation.ServerID, "activation_id": nil, "extra": 1,
	}); status != 400 || errorCode(body) != licensing.CodeInvalidRequest {
		t.Fatalf("campo sobrante: %d %v", status, body)
	}
	activation := func(requestID, key string, challengeBody map[string]any) map[string]any {
		id := challengeBody["challenge_id"].(string)
		message := licensing.ProofMessage("activate", id, challengeBody["nonce"].(string), "adel_pos", installation.ServerID, nil)
		return map[string]any{
			"request_id": requestID, "product_id": "adel_pos", "license_key": key, "server_id": installation.ServerID,
			"installation_public_key": installation.PublicKey, "challenge_id": id,
			"proof": licensing.SignProof(installation.PrivateKey, message), "app_version": "0.15.0",
		}
	}
	first := activation(NewUUID(), "PRP-3333-3333-3333", challenge())
	status, _, body := post("/v1/activations", "application/json", first)
	if status != 200 || body["license_jws"] == nil || body["server_time"] == nil {
		t.Fatalf("activar: %d %v", status, body)
	}
	// El reintento idéntico recibe la misma respuesta (aunque el desafío ya se usó).
	if status, _, again := post("/v1/activations", "application/json", first); status != 200 || again["license_jws"] != body["license_jws"] || again["server_time"] != body["server_time"] {
		t.Fatalf("reintento idéntico: %d %v", status, again)
	}
	// El mismo request_id con otro contenido: REQUEST_CONFLICT.
	conflict := activation(first["request_id"].(string), "PRP-4444-4444-4444", challenge())
	if status, _, body := post("/v1/activations", "application/json", conflict); status != 409 || errorCode(body) != licensing.CodeRequestConflict {
		t.Fatalf("REQUEST_CONFLICT: %d %v", status, body)
	}
	// Un desafío usado no sirve para otra petición.
	reused := activation(NewUUID(), "PRP-5555-5555-5555", challenge())
	if status, _, _ := post("/v1/activations", "application/json", reused); status != 200 {
		t.Fatal("primera petición con el desafío")
	}
	reused["request_id"] = NewUUID()
	if status, _, body := post("/v1/activations", "application/json", reused); status != 401 || errorCode(body) != licensing.CodeInvalidProof {
		t.Fatalf("desafío usado: %d %v", status, body)
	}
	// RATE_LIMITED lleva Retry-After.
	if status, header, body := post("/v1/activations", "application/json", activation(NewUUID(), "RAT-2222-2222-2222", challenge())); status != 429 || header.Get("Retry-After") != "30" || errorCode(body) != licensing.CodeRateLimited {
		t.Fatalf("RATE_LIMITED: %d %v", status, body)
	}
}
