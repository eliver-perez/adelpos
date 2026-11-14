//go:build integration

package administration_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/httpapi"
	"local/ad-el-pos/server/internal/lanlistener"
	"local/ad-el-pos/server/internal/localsetup"
	"local/ad-el-pos/server/internal/product"
)

// pairingEndpoint es una de las dos escuchas reales del servidor en la prueba: la local (HTTP en loopback)
// o la de red (HTTPS con el certificado del servidor, también desde 127.0.0.1).
type pairingEndpoint struct {
	base   string
	client *http.Client
}

type pairingReply struct {
	status int
	body   map[string]any
	raw    []byte
}

func (reply pairingReply) text(key string) string {
	value, _ := reply.body[key].(string)
	return value
}

func (reply pairingReply) code() string {
	failure, _ := reply.body["error"].(map[string]any)
	value, _ := failure["code"].(string)
	return value
}

func (endpoint pairingEndpoint) send(t *testing.T, actor terminal, method, route string, headers map[string]string, body any) pairingReply {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, endpoint.base+"/api/v1/"+route, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	// Como Rust (pairing.rs y api.rs): toda terminal se identifica con su versión; por la escucha de red es
	// obligatoria (etapa 9G, fase C).
	request.Header.Set("X-Client-Version", product.Read().Version)
	request.Header.Set("X-Client-Kind", "desktop")
	if actor.id != "" {
		request.Header.Set("X-Device-ID", actor.id)
		request.Header.Set("X-Device-Token", actor.secret)
	}
	if actor.session != "" {
		request.Header.Set("Authorization", "Bearer "+actor.session)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := endpoint.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	reply := pairingReply{status: response.StatusCode, raw: raw}
	_ = json.Unmarshal(raw, &reply.body)
	return reply
}

// pairingTerminal simula la terminal nueva (lo que hace Rust en pairing.rs): fija el primer certificado que
// ve en la escucha de red, envía el compromiso de su número, lo revela y deriva el código con la huella que
// recibió.
type pairingTerminal struct {
	endpoint    pairingEndpoint
	pinned      *[32]byte
	nonce       []byte
	requestID   string
	secret      string
	serverNonce []byte
	serverID    string
}

func newPairingTerminal(t *testing.T, base string) *pairingTerminal {
	t.Helper()
	terminal := &pairingTerminal{}
	var mutex sync.Mutex
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// La terminal no confía en una autoridad: fija la llave del primer certificado de la sesión.
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			mutex.Lock()
			defer mutex.Unlock()
			seen := localsetup.SPKIFingerprint(state.PeerCertificates[0])
			if terminal.pinned == nil {
				terminal.pinned = &seen
				return nil
			}
			if *terminal.pinned != seen {
				return errors.New("el servidor cambió de certificado")
			}
			return nil
		},
	}
	terminal.endpoint = pairingEndpoint{base: base, client: &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: config}}}
	terminal.nonce = make([]byte, 32)
	if _, err := rand.Read(terminal.nonce); err != nil {
		t.Fatal(err)
	}
	return terminal
}

func (terminal *pairingTerminal) start(t *testing.T, name string) pairingReply {
	t.Helper()
	reply := terminal.endpoint.send(t, terminal0, "POST", "pairing/requests", nil, map[string]string{
		"name": name, "device_kind": "desktop", "os": "macOS", "app_version": "0.10.0",
		"commitment": administration.PairingCommitment(terminal.nonce),
	})
	if reply.status == 200 {
		terminal.requestID = reply.text("request_id")
		terminal.secret = reply.text("pairing_secret")
		terminal.serverID = reply.text("server_id")
		terminal.serverNonce, _ = hex.DecodeString(reply.text("server_nonce"))
	}
	return reply
}

var terminal0 = terminal{}

func (terminal *pairingTerminal) headers() map[string]string {
	return map[string]string{"X-Pairing-Secret": terminal.secret}
}

func (terminal *pairingTerminal) reveal(t *testing.T) pairingReply {
	t.Helper()
	return terminal.endpoint.send(t, terminal0, "POST", "pairing/requests/"+terminal.requestID+"/reveal", terminal.headers(),
		map[string]string{"terminal_nonce": hex.EncodeToString(terminal.nonce)})
}

func (terminal *pairingTerminal) code(t *testing.T) string {
	t.Helper()
	code, err := administration.PairingCode(*terminal.pinned, terminal.nonce, terminal.serverNonce, terminal.requestID)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func (terminal *pairingTerminal) status(t *testing.T) string {
	t.Helper()
	reply := terminal.endpoint.send(t, terminal0, "GET", "pairing/requests/"+terminal.requestID, terminal.headers(), nil)
	if reply.status != 200 {
		t.Fatalf("estado de la solicitud: %d %s", reply.status, reply.code())
	}
	return reply.text("state")
}

func (terminal *pairingTerminal) claim(t *testing.T) pairingReply {
	t.Helper()
	return terminal.endpoint.send(t, terminal0, "POST", "pairing/requests/"+terminal.requestID+"/claim", terminal.headers(), map[string]any{})
}

// wrongCode es un código de 6 dígitos distinto del correcto.
func wrongCode(correct string) string {
	if correct == "000000" {
		return "111111"
	}
	return "000000"
}

// testPairingAndTerminals cubre la etapa 9G, fase B (§3, §9): emparejamiento con código correcto e
// incorrecto, vencido, límite de intentos, aprobación solo por la escucha local, credencial entregada una
// sola vez (también con reclamos simultáneos), límite de solicitudes por origen, lista y configuración de
// terminales, auditoría con usuario y dispositivo, revocación inmediata con el SSE cortado y conteo de
// sesiones por tipo de dispositivo. Usa las dos escuchas reales: HTTP en loopback y HTTPS de red.
func testPairingAndTerminals(t *testing.T, env *delegationEnvironment) {
	identity, err := localsetup.EnsureIdentity(filepath.Join(t.TempDir(), localsetup.IdentityFolder))
	if err != nil {
		t.Fatal(err)
	}
	env.service.Identity = &administration.ServerIdentity{ServerID: identity.ServerID, Fingerprint: identity.Fingerprint}
	defer func() {
		env.service.Identity = nil
	}()
	handler := httpapi.New(func(ctx context.Context) database.Health {
		return database.Probe(ctx, env.pool)
	}, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)), env.service)
	localServer := httptest.NewServer(handler)
	defer localServer.Close()
	network, err := lanlistener.Listen("127.0.0.1:0", identity.Certificate, handler)
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() {
		served <- network.Serve()
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = network.Shutdown(ctx)
		<-served
	}()
	networkBase := "https://" + network.Addr().String()
	networkPort := network.Addr().(*net.TCPAddr).Port
	env.service.NetworkPort = networkPort
	defer func() {
		env.service.NetworkPort = 0
	}()
	local := pairingEndpoint{base: localServer.URL, client: &http.Client{Timeout: 10 * time.Second}}
	// El administrador también puede llegar por la escucha de red (otra terminal o la misma computadora
	// por su IP): ahí nunca se aprueba.
	adminOverNetwork := pairingEndpoint{base: networkBase, client: &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
	}}}
	approve := func(endpoint pairingEndpoint, requestID string, body map[string]any) pairingReply {
		return endpoint.send(t, env.admin, "POST", "devices/pairing-requests/"+requestID+"/approve", nil, body)
	}
	// Las solicitudes anteriores dejan de contar para el límite por origen (todas vienen de 127.0.0.1).
	forgetRecentRequests := func() {
		if _, err := env.pool.Exec(env.ctx, "UPDATE app.device_pairing_requests SET created_at = created_at - interval '11 minutes'"); err != nil {
			t.Fatal(err)
		}
	}
	countAudit := func(action, requestID string) int {
		return env.scalar(t, "SELECT count(*) FROM app.audit_events WHERE action=$1 AND entity_id=$2", action, requestID)
	}

	var paired terminal
	var pairedName string
	t.Run("stage9g_emparejamiento_con_codigo_correcto", func(t *testing.T) {
		newcomer := newPairingTerminal(t, networkBase)
		started := newcomer.start(t, "Caja 2")
		if started.status != 200 || newcomer.serverID != identity.ServerID || started.text("server_name") == "" {
			t.Fatalf("crear la solicitud: %d %s", started.status, started.raw)
		}
		if *newcomer.pinned != identity.Fingerprint {
			t.Fatal("la terminal debe ver el certificado del servidor")
		}
		if reply := approve(local, newcomer.requestID, map[string]any{"code": newcomer.code(t)}); reply.status != 409 || reply.code() != "PAIRING_NOT_READY" {
			t.Fatalf("sin revelar el número no se aprueba: %d %s", reply.status, reply.code())
		}
		if reply := newcomer.reveal(t); reply.status != 200 {
			t.Fatalf("revelar: %d %s", reply.status, reply.code())
		}
		listed := local.send(t, env.admin, "GET", "devices/pairing-requests", nil, nil)
		if listed.status != 200 || listed.body["approve_here"] != true || !strings.Contains(string(listed.raw), newcomer.requestID) {
			t.Fatalf("la solicitud pendiente debe aparecer en la computadora principal: %s", listed.raw)
		}
		if strings.Contains(string(listed.raw), "nonce") || strings.Contains(string(listed.raw), "secret") || strings.Contains(string(listed.raw), newcomer.code(t)) {
			t.Fatal("la lista no muestra números, secretos ni el código")
		}
		// La computadora principal ve cómo escribir su dirección en la terminal nueva.
		networkInfo, _ := listed.body["network"].(map[string]any)
		if networkInfo["available"] != true || networkInfo["port"] != float64(networkPort) || networkInfo["addresses"] == nil {
			t.Fatalf("datos de la escucha de red para emparejar a mano: %v", networkInfo)
		}
		if newcomer.status(t) != "pending" {
			t.Fatal("sin aprobar sigue pendiente")
		}
		if reply := newcomer.claim(t); reply.status != 409 || reply.code() != "PAIRING_NOT_APPROVED" {
			t.Fatalf("sin aprobar no hay credencial: %d %s", reply.status, reply.code())
		}
		reply := approve(local, newcomer.requestID, map[string]any{"code": newcomer.code(t), "name": "Caja de la barra"})
		if reply.status != 200 || reply.text("state") != "approved" {
			t.Fatalf("aprobar con el código correcto: %d %s", reply.status, reply.raw)
		}
		if newcomer.status(t) != "approved" {
			t.Fatal("la terminal debe ver su solicitud aprobada")
		}
		claimed := newcomer.claim(t)
		if claimed.status != 200 || claimed.text("device_token") == "" || claimed.text("server_id") != identity.ServerID {
			t.Fatalf("reclamar la credencial: %d %s", claimed.status, claimed.raw)
		}
		paired = terminal{id: claimed.text("device_id"), secret: claimed.text("device_token")}
		pairedName = claimed.text("name")
		if pairedName != "Caja de la barra" {
			t.Fatalf("el nombre aprobado debe ser el de la terminal: %q", pairedName)
		}
		// La credencial se entrega una sola vez.
		again := newcomer.claim(t)
		if again.status != 409 || again.code() != "PAIRING_ALREADY_CLAIMED" || again.text("device_token") != "" {
			t.Fatalf("la credencial no se entrega dos veces: %d %s", again.status, again.raw)
		}
		var kind, system, version, fingerprint string
		var pairedAt *time.Time
		err := env.pool.QueryRow(env.ctx, "SELECT kind, os, app_version, server_fingerprint, paired_at FROM app.devices WHERE id=$1", paired.id).Scan(&kind, &system, &version, &fingerprint, &pairedAt)
		if err != nil || kind != "desktop" || system != "macOS" || version != "0.10.0" || fingerprint != identity.FingerprintHex() || pairedAt == nil {
			t.Fatalf("datos de la terminal emparejada: %v %s %s %s %s", err, kind, system, version, fingerprint)
		}
		if countAudit("device.pairing_approved", newcomer.requestID) != 1 {
			t.Fatal("la aprobación se audita")
		}
		var approverDevice, approver string
		if err := env.pool.QueryRow(env.ctx, `SELECT device_id::text, actor_id::text FROM app.audit_events
                WHERE action='device.pairing_approved' AND entity_id=$1`, newcomer.requestID).Scan(&approverDevice, &approver); err != nil || approverDevice != env.admin.id {
			t.Fatalf("la aprobación guarda la terminal y el usuario que aprobó: %v %s", err, approverDevice)
		}
		if env.scalar(t, "SELECT count(*) FROM app.audit_events WHERE action='device.paired' AND entity_id=$1", paired.id) != 1 {
			t.Fatal("el emparejamiento se audita")
		}
		// La terminal emparejada trabaja por la escucha de red con su credencial.
		if reply := newcomer.endpoint.send(t, paired, "POST", "auth/login", nil, map[string]string{"username": "admin", "password": "incorrecta"}); reply.status != 401 || reply.code() != "INVALID_CREDENTIALS" {
			t.Fatalf("la terminal emparejada llega al inicio de sesión: %d %s", reply.status, reply.code())
		}
	})

	// Etapa 9G, fase C: por la escucha de red la versión del cliente es obligatoria (426); por la escucha local
	// una herramienta sin cabeceras sigue entrando.
	t.Run("stage9g_escucha_de_red_exige_la_version_del_cliente", func(t *testing.T) {
		bare := func(endpoint pairingEndpoint) int {
			request, err := http.NewRequest("POST", endpoint.base+"/api/v1/pairing/requests", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			response, err := endpoint.client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			return response.StatusCode
		}
		if status := bare(adminOverNetwork); status != 426 {
			t.Fatalf("sin versión por la red: %d", status)
		}
		if status := bare(local); status != 400 {
			t.Fatalf("sin versión por la escucha local llega a la validación: %d", status)
		}
	})

	t.Run("stage9g_codigo_incorrecto_y_limite_de_intentos", func(t *testing.T) {
		forgetRecentRequests()
		newcomer := newPairingTerminal(t, networkBase)
		if reply := newcomer.start(t, "Caja 3"); reply.status != 200 {
			t.Fatalf("crear: %d %s", reply.status, reply.code())
		}
		newcomer.reveal(t)
		wrong := wrongCode(newcomer.code(t))
		for attempt := 1; attempt <= 4; attempt++ {
			reply := approve(local, newcomer.requestID, map[string]any{"code": wrong})
			details, _ := reply.body["error"].(map[string]any)["details"].(map[string]any)
			if reply.status != 400 || reply.code() != "PAIRING_CODE_INVALID" || details["attempts_left"] != float64(5-attempt) {
				t.Fatalf("intento %d: %d %s", attempt, reply.status, reply.raw)
			}
		}
		if reply := approve(local, newcomer.requestID, map[string]any{"code": "12a456"}); reply.status != 400 || reply.code() != "VALIDATION_ERROR" {
			t.Fatalf("un código que no son 6 números no cuenta como intento: %d %s", reply.status, reply.code())
		}
		if reply := approve(local, newcomer.requestID, map[string]any{"code": wrong}); reply.status != 409 || reply.code() != "PAIRING_ATTEMPTS_EXCEEDED" {
			t.Fatalf("el quinto código incorrecto rechaza la solicitud: %d %s", reply.status, reply.code())
		}
		if reply := approve(local, newcomer.requestID, map[string]any{"code": newcomer.code(t)}); reply.status != 409 || reply.code() != "PAIRING_REJECTED" {
			t.Fatalf("después ni el código correcto la aprueba: %d %s", reply.status, reply.code())
		}
		if newcomer.status(t) != "rejected" {
			t.Fatal("la terminal ve la solicitud rechazada")
		}
		if reply := newcomer.claim(t); reply.status != 409 || reply.code() != "PAIRING_REJECTED" {
			t.Fatalf("una solicitud rechazada no entrega credencial: %d %s", reply.status, reply.code())
		}
		if countAudit("device.pairing_code_failed", newcomer.requestID) != 5 || countAudit("device.pairing_rejected", newcomer.requestID) != 1 {
			t.Fatal("cada código incorrecto y el rechazo quedan en el Registro de movimientos")
		}
	})

	t.Run("stage9g_codigo_vencido", func(t *testing.T) {
		forgetRecentRequests()
		newcomer := newPairingTerminal(t, networkBase)
		newcomer.start(t, "Caja 4")
		newcomer.reveal(t)
		if _, err := env.pool.Exec(env.ctx, "UPDATE app.device_pairing_requests SET expires_at = now() - interval '1 second' WHERE id=$1", newcomer.requestID); err != nil {
			t.Fatal(err)
		}
		if reply := approve(local, newcomer.requestID, map[string]any{"code": newcomer.code(t)}); reply.status != 409 || reply.code() != "PAIRING_EXPIRED" {
			t.Fatalf("un código vencido no se aprueba: %d %s", reply.status, reply.code())
		}
		if newcomer.status(t) != "expired" {
			t.Fatal("la terminal ve el código vencido")
		}
		listed := local.send(t, env.admin, "GET", "devices/pairing-requests", nil, nil)
		if strings.Contains(string(listed.raw), newcomer.requestID) {
			t.Fatal("una solicitud vencida ya no está pendiente")
		}
		// Aprobada pero sin reclamar a tiempo, tampoco entrega credencial.
		late := newPairingTerminal(t, networkBase)
		late.start(t, "Caja 5")
		late.reveal(t)
		if reply := approve(local, late.requestID, map[string]any{"code": late.code(t)}); reply.status != 200 {
			t.Fatalf("aprobar: %d %s", reply.status, reply.code())
		}
		if _, err := env.pool.Exec(env.ctx, "UPDATE app.device_pairing_requests SET expires_at = now() - interval '1 second' WHERE id=$1", late.requestID); err != nil {
			t.Fatal(err)
		}
		if reply := late.claim(t); reply.status != 409 || reply.code() != "PAIRING_EXPIRED" {
			t.Fatalf("una aprobación vencida no entrega credencial: %d %s", reply.status, reply.code())
		}
	})

	t.Run("stage9g_aprobacion_rechazada_por_la_escucha_de_red", func(t *testing.T) {
		forgetRecentRequests()
		newcomer := newPairingTerminal(t, networkBase)
		newcomer.start(t, "Caja 6")
		newcomer.reveal(t)
		denials := env.deniedCount(t)
		// Por la escucha de red la conexión viene de 127.0.0.1, pero no es «local».
		reply := approve(adminOverNetwork, newcomer.requestID, map[string]any{"code": newcomer.code(t)})
		if reply.status != 403 || reply.code() != "PAIRING_LOCAL_ONLY" {
			t.Fatalf("la aprobación por la red debe rechazarse: %d %s", reply.status, reply.code())
		}
		if env.deniedCount(t) != denials+1 {
			t.Fatal("el intento queda auditado como acceso denegado")
		}
		if newcomer.status(t) != "pending" || countAudit("device.pairing_code_failed", newcomer.requestID) != 0 {
			t.Fatal("el intento por la red no gasta intentos ni cambia la solicitud")
		}
		if reply := adminOverNetwork.send(t, env.admin, "POST", "devices/pairing-requests/"+newcomer.requestID+"/reject", nil, map[string]any{}); reply.status != 403 {
			t.Fatalf("tampoco se rechaza por la red: %d", reply.status)
		}
		listed := adminOverNetwork.send(t, env.admin, "GET", "devices/pairing-requests", nil, nil)
		if listed.status != 200 || listed.body["approve_here"] != false || listed.body["network"] != nil {
			t.Fatalf("por la red la lista dice que se aprueba en la computadora principal: %s", listed.raw)
		}
		// El asistente inicial y el código de 43 caracteres tampoco se aceptan por la escucha de red.
		if reply := adminOverNetwork.send(t, terminal0, "POST", "setup", nil, map[string]any{}); reply.status != 403 {
			t.Fatalf("el asistente no se acepta por la red: %d", reply.status)
		}
		if reply := adminOverNetwork.send(t, terminal0, "POST", "devices/enroll", nil, map[string]string{"code": strings.Repeat("a", 43), "name": "Intrusa"}); reply.status != 403 || reply.code() != "ENROLLMENT_LOCAL_ONLY" {
			t.Fatalf("el código de autorización no se acepta por la red: %d %s", reply.status, reply.code())
		}
		// En la computadora principal sí se rechaza.
		if reply := local.send(t, env.admin, "POST", "devices/pairing-requests/"+newcomer.requestID+"/reject", nil, map[string]any{}); reply.status != 200 {
			t.Fatalf("rechazar en la computadora principal: %d %s", reply.status, reply.code())
		}
		if newcomer.status(t) != "rejected" {
			t.Fatal("la terminal ve la solicitud rechazada")
		}
		if countAudit("device.pairing_rejected", newcomer.requestID) != 1 {
			t.Fatal("el rechazo se audita")
		}
	})

	t.Run("stage9g_numero_que_no_corresponde_al_compromiso", func(t *testing.T) {
		forgetRecentRequests()
		newcomer := newPairingTerminal(t, networkBase)
		newcomer.start(t, "Caja 7")
		newcomer.nonce[0] ^= 0xff
		if reply := newcomer.reveal(t); reply.status != 400 || reply.code() != "PAIRING_COMMITMENT_MISMATCH" {
			t.Fatalf("un número distinto del comprometido se rechaza: %d %s", reply.status, reply.code())
		}
		if newcomer.status(t) != "rejected" {
			t.Fatal("la solicitud queda rechazada")
		}
		// Sin el secreto de la solicitud no se consulta ni se reclama nada.
		stranger := newcomer.endpoint.send(t, terminal0, "GET", "pairing/requests/"+newcomer.requestID, map[string]string{"X-Pairing-Secret": strings.Repeat("x", 43)}, nil)
		if stranger.status != 404 {
			t.Fatalf("sin el secreto la solicitud no existe: %d", stranger.status)
		}
	})

	t.Run("stage9g_credencial_una_sola_vez_con_reclamos_simultaneos", func(t *testing.T) {
		forgetRecentRequests()
		newcomer := newPairingTerminal(t, networkBase)
		newcomer.start(t, "Caja 8")
		newcomer.reveal(t)
		if reply := approve(local, newcomer.requestID, map[string]any{"code": newcomer.code(t)}); reply.status != 200 {
			t.Fatalf("aprobar: %d %s", reply.status, reply.code())
		}
		var wait sync.WaitGroup
		results := make([]pairingReply, 4)
		for index := range results {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				results[index] = newcomer.claim(t)
			}(index)
		}
		wait.Wait()
		tokens := 0
		for _, result := range results {
			if result.status == 200 && result.text("device_token") != "" {
				tokens++
			} else if result.status != 409 || result.code() != "PAIRING_ALREADY_CLAIMED" {
				t.Fatalf("reclamo simultáneo inesperado: %d %s", result.status, result.raw)
			}
		}
		if tokens != 1 {
			t.Fatalf("la credencial se entregó %d veces", tokens)
		}
		if env.scalar(t, "SELECT count(*) FROM app.devices WHERE name='Caja 8'") != 1 {
			t.Fatal("se creó una sola terminal")
		}
	})

	t.Run("stage9g_limite_de_solicitudes_por_origen", func(t *testing.T) {
		forgetRecentRequests()
		for index := 0; index < 5; index++ {
			if reply := newPairingTerminal(t, networkBase).start(t, "Caja repetida"); reply.status != 200 {
				t.Fatalf("solicitud %d: %d %s", index, reply.status, reply.code())
			}
		}
		if reply := newPairingTerminal(t, networkBase).start(t, "Caja repetida"); reply.status != 429 || reply.code() != "PAIRING_RATE_LIMITED" {
			t.Fatalf("la sexta solicitud del mismo origen se limita: %d %s", reply.status, reply.code())
		}
		forgetRecentRequests()
		if _, err := env.pool.Exec(env.ctx, "UPDATE app.device_pairing_requests SET state='cancelled' WHERE state='pending'"); err != nil {
			t.Fatal(err)
		}
	})

	var user terminal
	t.Run("stage9g_lista_configuracion_y_auditoria_con_usuario_y_terminal", func(t *testing.T) {
		role := env.createRole(t, "Cajero 9G emparejado", "devices.read", "events.read")
		userID, _ := env.createUser(t, "caja.emparejada9g", role)
		networkEndpoint := newPairingTerminal(t, networkBase).endpoint
		login := networkEndpoint.send(t, paired, "POST", "auth/login", nil, map[string]string{"username": "caja.emparejada9g", "password": "clave-temporal-caja.emparejada9g"})
		if login.status != 200 {
			t.Fatalf("inicio de sesión en la terminal emparejada: %d %s", login.status, login.code())
		}
		user = paired
		user.session = login.text("session_token")
		var auditedUser, auditedDevice string
		err := env.pool.QueryRow(env.ctx, `SELECT actor_id::text, device_id::text FROM app.audit_events
            WHERE action='session.login' AND actor_id=$1 ORDER BY occurred_at DESC LIMIT 1`, userID).Scan(&auditedUser, &auditedDevice)
		if err != nil || auditedUser != userID || auditedDevice != paired.id {
			t.Fatalf("el Registro de movimientos guarda usuario y terminal: %v %s %s", err, auditedUser, auditedDevice)
		}
		devices := local.send(t, env.admin, "GET", "devices", nil, nil)
		var listed []map[string]any
		if err := json.Unmarshal(devices.raw, &listed); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, device := range listed {
			if device["id"] != paired.id {
				continue
			}
			found = true
			if device["kind"] != "desktop" || device["os"] != "macOS" || device["online"] != true || device["current_user"] != "Prueba caja.emparejada9g" || device["name"] != pairedName {
				t.Fatalf("datos de la terminal en la lista: %v", device)
			}
		}
		if !found {
			t.Fatal("la terminal emparejada aparece en la lista")
		}
		configuration := local.send(t, env.admin, "GET", "devices/"+paired.id+"/configuration", nil, nil)
		if configuration.status != 200 || configuration.body["printers"] == nil || configuration.body["kitchen_all_stations"] != true {
			t.Fatalf("configuración de la terminal: %d %s", configuration.status, configuration.raw)
		}
	})

	t.Run("stage9g_sesiones_activas_por_tipo", func(t *testing.T) {
		summary := func(actor terminal) map[string]any {
			reply := local.send(t, actor, "GET", "devices/sessions-summary", nil, nil)
			if reply.status != 200 {
				t.Fatalf("conteo de sesiones: %d %s", reply.status, reply.code())
			}
			return reply.body
		}
		expected := env.scalar(t, `SELECT count(*) FROM app.sessions s JOIN app.devices d ON d.id=s.device_id
            WHERE s.revoked_at IS NULL AND s.expires_at > now() AND d.revoked_at IS NULL AND d.kind='desktop'`)
		value := summary(env.admin)
		byKind := value["by_kind"].(map[string]any)
		if byKind["desktop"] != float64(expected) || byKind["android"] != float64(0) || byKind["kitchen"] != float64(0) || value["limited"] != false {
			t.Fatalf("conteo por tipo: %v (se esperaban %d de escritorio)", value, expected)
		}
		// Un comandero Android con sesión activa cuenta aparte; no se limita nada.
		var androidID, userID string
		if err := env.pool.QueryRow(env.ctx, "SELECT id FROM app.users WHERE username='caja.emparejada9g'").Scan(&userID); err != nil {
			t.Fatal(err)
		}
		if err := env.pool.QueryRow(env.ctx, `INSERT INTO app.devices(name, secret_hash, kind, os, app_version, paired_at)
                VALUES ('Comandero 9G', md5(random()::text), 'android', 'Android 15', '1.0.0', now()) RETURNING id`).Scan(&androidID); err != nil {
			t.Fatal(err)
		}
		role := env.createRole(t, "Mesero 9G", "devices.read")
		waiterID, _ := env.createUser(t, "mesero.android9g", role)
		if _, err := env.pool.Exec(env.ctx, `UPDATE app.sessions SET device_id=$1 WHERE user_id=$2 AND revoked_at IS NULL`, androidID, waiterID); err != nil {
			t.Fatal(err)
		}
		value = summary(env.admin)
		byKind = value["by_kind"].(map[string]any)
		if byKind["android"] != float64(1) || value["total"] != float64(expected+1) {
			t.Fatalf("el comandero cuenta como Android: %v", value)
		}
		role = env.createRole(t, "Sin terminales 9G", "users.read")
		_, outsider := env.createUser(t, "sin.terminales9g", role)
		if reply := local.send(t, outsider, "GET", "devices/sessions-summary", nil, nil); reply.status != 403 {
			t.Fatalf("el conteo requiere devices.read: %d", reply.status)
		}
		_ = userID
	})

	t.Run("stage9g_revocacion_inmediata_corta_el_sse", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(env.ctx, 15*time.Second)
		defer cancel()
		stream, err := http.NewRequestWithContext(ctx, "GET", networkBase+"/api/v1/events", nil)
		if err != nil {
			t.Fatal(err)
		}
		stream.Header.Set("X-Device-ID", user.id)
		stream.Header.Set("X-Device-Token", user.secret)
		stream.Header.Set("Authorization", "Bearer "+user.session)
		stream.Header.Set("X-Client-Version", product.Read().Version)
		stream.Header.Set("X-Client-Kind", "desktop")
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true,
			VerifyConnection: func(state tls.ConnectionState) error {
				if localsetup.SPKIFingerprint(state.PeerCertificates[0]) != identity.Fingerprint {
					return errors.New("certificado distinto del fijado")
				}
				return nil
			},
		}}}
		response, err := client.Do(stream)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("el SSE por la escucha cifrada debe abrir: %d", response.StatusCode)
		}
		reader := bufio.NewReader(response.Body)
		frames := make(chan string, 16)
		go func() {
			defer close(frames)
			for {
				line, err := reader.ReadString('\n')
				if strings.HasPrefix(line, "event: ") || strings.HasPrefix(line, "data: ") {
					frames <- strings.TrimSpace(line)
				}
				if err != nil {
					return
				}
			}
		}()
		if first := <-frames; first != "event: changes" {
			t.Fatalf("primer evento: %q", first)
		}
		revokedAt := time.Now()
		if reply := local.send(t, env.admin, "POST", "devices/"+paired.id+"/revoke", nil, map[string]any{}); reply.status != 200 {
			t.Fatalf("revocar: %d %s", reply.status, reply.code())
		}
		interrupted := false
		for frame := range frames {
			if strings.Contains(frame, "DEVICE_REVOKED") {
				interrupted = true
			}
		}
		elapsed := time.Since(revokedAt)
		if !interrupted || elapsed > 3*time.Second {
			t.Fatalf("la terminal revocada pierde el SSE en segundos (avisado: %v, %s)", interrupted, elapsed)
		}
		networkEndpoint := newPairingTerminal(t, networkBase).endpoint
		if reply := networkEndpoint.send(t, user, "GET", "auth/me", nil, nil); reply.status != 401 || reply.code() != "DEVICE_REVOKED" {
			t.Fatalf("cada petición de la terminal revocada se rechaza: %d %s", reply.status, reply.code())
		}
		if env.scalar(t, "SELECT count(*) FROM app.sessions WHERE device_id=$1 AND revoked_at IS NULL", paired.id) != 0 {
			t.Fatal("las sesiones de la terminal revocada se cierran")
		}
	})
}
