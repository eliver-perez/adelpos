//go:build !adelpos_production

package licensetest

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"local/ad-el-pos/server/internal/licensing"
)

// Servidor de licencias simulado **solo para pruebas**: cumple la API en línea del contrato (§8 y
// contracts/licencias/v1/openapi.yaml) en memoria, firmando con las llaves de vectors.json. Lo usan las pruebas de Go
// (con httptest y TLS, en loopback) y la orden `license-test server` para Playwright.
//
// Implementa: desafío de un solo uso de 5 minutos para su acción, producto e instalación; prueba de posesión
// (activate y transfer con la llave enviada, refresh y deactivate con la de la activación); revisión global por
// licencia; la misma instalación que vuelve a activar recibe su revisión vigente; ACTIVATION_LIMIT con
// transfer_available; la transferencia por autoservicio con su cierre `transferred`, pendiente hasta que la
// computadora anterior consulte o libere; TRANSFER_LIMIT; deactivate con su cierre `deactivated`; reintentos
// idénticos con la misma respuesta y REQUEST_CONFLICT; y los errores de transporte (404, 405, 413 y 415).
//
// Cada código comercial con el formato del servidor (L1-D4) es una licencia distinta; su primer grupo dice cómo se
// comporta (Behaviors). Además, las pruebas piden errores concretos con FailNext, una respuesta perdida con DropNext,
// una licencia alterada con AlterNext, y revocan o renuevan como lo haría el panel. La orden de pruebas expone lo mismo
// en /_prueba/* (control.go).

// Behaviors es el primer grupo del código comercial y lo que hace la licencia. Un prefijo desconocido es un código
// inexistente (404 LICENSE_NOT_FOUND).
var Behaviors = map[string]string{
	"PRP": "perpetua, Plan elemental (3 cajas, 3 comanderos, 3 pantallas de cocina)",
	"BSC": "perpetua, Plan básico (1 caja, 0 comanderos, 0 pantallas de cocina)",
	"MNS": "suscripción mensual: vence un mes después de la primera activación, con 5 días de tolerancia",
	"PRB": "prueba de 15 días desde la primera activación, Plan básico",
	"GRC": "suscripción que venció hace 2 días: en tolerancia",
	"VNC": "suscripción que venció hace 40 días: vencida",
	"RVK": "licencia revocada por el propietario: 403 REVOKED",
	"ACT": "activa en otra computadora: 409 ACTIVATION_LIMIT con transfer_available true; transferir funciona",
	"ACN": "activa en otra computadora con una transferencia pendiente: ACTIVATION_LIMIT con transfer_available false, y transferir responde 409 TRANSFER_LIMIT",
	"TMP": "activar y transferir responden 503 TEMPORARY_UNAVAILABLE",
	"RAT": "activar y transferir responden 429 RATE_LIMITED con Retry-After: 30",
	"PRF": "activar y transferir responden 401 INVALID_PROOF",
}

// keyPattern es el formato del código comercial (L1-D4), ya normalizado.
var keyPattern = regexp.MustCompile(`^[23456789ABCDEFGHJKMNPQRSTUVWXYZ]{3}(-[23456789ABCDEFGHJKMNPQRSTUVWXYZ]{4}){3}$`)

var (
	uuidPattern    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	serverIDFormat = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	productPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
)

// Server es el servidor de licencias simulado. Es seguro para uso concurrente.
type Server struct {
	fixtures Fixtures
	// Now es el reloj del servidor (por omisión, time.Now); Offset se le suma (la hora del servidor en las pruebas
	// de la §6.3).
	Now func() time.Time

	mutex       sync.Mutex
	offset      time.Duration
	licenses    map[string]*simLicense
	activations map[string]*simActivation
	challenges  map[string]*simChallenge
	replies     map[string]*simReply
	scripted    []Scripted
	alterNext   bool
	dropNext    map[string]bool
	calls       []Call
}

// Scripted es un error que el servidor responde a la siguiente petición de una acción (FailNext).
type Scripted struct {
	// Action: challenge, activate, transfer, refresh o deactivate.
	Action string
	Code   string
	// RetryAfter va en el encabezado Retry-After (en segundos), si no es cero.
	RetryAfter int
	// TransferAvailable va en details de ACTIVATION_LIMIT.
	TransferAvailable bool
}

// Call es una petición que recibió el servidor, para las comprobaciones de las pruebas.
type Call struct {
	Action    string
	RequestID string
	Status    int
	Code      string
	// Replayed: la respuesta guardada de un reintento idéntico.
	Replayed bool
}

type simLicense struct {
	key          string
	id           string
	behavior     string
	revision     int64
	licenseType  string
	plan         string
	expiresAt    time.Time
	startsAt     time.Time
	datesFixed   bool
	revoked      bool
	current      *simActivation
	pendingFrom  *simActivation
	licenseeName string
}

type simActivation struct {
	id        string
	license   *simLicense
	serverID  string
	publicKey string
	active    bool
	jws       string
	endedAt   time.Time
}

type simChallenge struct {
	action       string
	serverID     string
	activationID string
	nonce        string
	expiresAt    time.Time
	used         bool
}

type simReply struct {
	fingerprint string
	status      int
	header      http.Header
	body        []byte
}

// NewServer crea el servidor con las llaves de los archivos de prueba.
func NewServer(fixtures Fixtures) *Server {
	return &Server{
		fixtures:    fixtures,
		Now:         time.Now,
		licenses:    map[string]*simLicense{},
		activations: map[string]*simActivation{},
		challenges:  map[string]*simChallenge{},
		replies:     map[string]*simReply{},
		dropNext:    map[string]bool{},
	}
}

func (server *Server) now() time.Time {
	return server.Now().Add(server.offset).UTC()
}

// --- Controles para las pruebas ----------------------------------------------------------------------------------

// FailNext hace que la siguiente petición de la acción responda ese error (después de validar su forma).
func (server *Server) FailNext(scripted Scripted) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.scripted = append(server.scripted, scripted)
}

// DropNext atiende la siguiente petición de la acción, guarda su respuesta y corta la conexión sin enviarla: el
// cliente debe reintentar con el mismo cuerpo y recibir la misma respuesta.
func (server *Server) DropNext(action string) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.dropNext[action] = true
}

// AlterNext altera la firma de la siguiente licencia que responda: el cliente debe ignorarla.
func (server *Server) AlterNext() {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.alterNext = true
}

// SetOffset fija el desfase de la hora del servidor respecto de Now.
func (server *Server) SetOffset(offset time.Duration) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.offset = offset
}

// Calls son las peticiones recibidas.
func (server *Server) Calls() []Call {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return append([]Call(nil), server.calls...)
}

// Revoke es la revocación del propietario en el panel: la activación vigente recibe su cierre revoked/revoked y la
// licencia ya no se activa (403 REVOKED).
func (server *Server) Revoke(licenseKey string) error {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	license := server.licenses[licenseKey]
	if license == nil {
		return errors.New("el servidor simulado no conoce esa licencia")
	}
	license.revoked = true
	if license.current != nil {
		if err := server.closeActivation(license.current, licensing.ReasonRevoked); err != nil {
			return err
		}
	}
	return nil
}

// Renew es una renovación o un cambio en el panel: más meses de vigencia (una suscripción o prueba) y una revisión
// nueva para la activación vigente, que la computadora recibe al consultar.
func (server *Server) Renew(licenseKey string, months int) error {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	license := server.licenses[licenseKey]
	if license == nil || license.current == nil {
		return errors.New("el servidor simulado no tiene esa licencia activa")
	}
	if license.licenseType != TypePerpetual {
		license.expiresAt = license.expiresAt.AddDate(0, months, 0)
	}
	jws, err := server.sign(license, license.current, "")
	if err != nil {
		return err
	}
	license.current.jws = jws
	return nil
}

// State es un resumen del servidor para las pruebas.
type State struct {
	Licenses []LicenseState `json:"licenses"`
}

// LicenseState es el estado de una licencia simulada. El código comercial se muestra: es de este servidor simulado,
// nunca de una licencia real.
type LicenseState struct {
	LicenseKey      string `json:"license_key"`
	LicenseID       string `json:"license_id"`
	Revision        int64  `json:"license_revision"`
	Revoked         bool   `json:"revoked"`
	ActiveServerID  string `json:"active_server_id"`
	PendingTransfer bool   `json:"pending_transfer"`
}

// Snapshot es el estado de las licencias simuladas.
func (server *Server) Snapshot() State {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	state := State{Licenses: []LicenseState{}}
	for _, license := range server.licenses {
		item := LicenseState{
			LicenseKey:      license.key,
			LicenseID:       license.id,
			Revision:        license.revision,
			Revoked:         license.revoked,
			PendingTransfer: license.pendingFrom != nil,
		}
		if license.current != nil {
			item.ActiveServerID = license.current.serverID
		}
		state.Licenses = append(state.Licenses, item)
	}
	return state
}

// --- HTTP ----------------------------------------------------------------------------------------------------------

// ServeHTTP atiende la API del contrato y, en /_prueba/, los controles de la orden de pruebas.
func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.URL.Path, "/_prueba/") {
		server.serveControl(writer, request)
		return
	}
	action, known := map[string]string{
		"/v1/activations/challenge":  "challenge",
		"/v1/activations":            licensing.ActionActivate,
		"/v1/activations/transfer":   licensing.ActionTransfer,
		"/v1/activations/refresh":    licensing.ActionRefresh,
		"/v1/activations/deactivate": licensing.ActionDeactivate,
	}[request.URL.Path]
	// Errores de transporte (§8.7): INVALID_REQUEST con su HTTP estándar.
	if !known {
		writeError(writer, http.StatusNotFound, licensing.CodeInvalidRequest, nil, nil)
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", "POST")
		writeError(writer, http.StatusMethodNotAllowed, licensing.CodeInvalidRequest, nil, nil)
		return
	}
	if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		writeError(writer, http.StatusUnsupportedMediaType, licensing.CodeInvalidRequest, nil, nil)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, 16<<10))
	if err != nil {
		writeError(writer, http.StatusRequestEntityTooLarge, licensing.CodeInvalidRequest, nil, nil)
		return
	}
	status, header, payload, drop := server.handle(action, body)
	if drop {
		// La respuesta ya quedó guardada: se corta la conexión sin enviarla, como una respuesta perdida.
		if hijacker, ok := writer.(http.Hijacker); ok {
			if connection, _, err := hijacker.Hijack(); err == nil {
				_ = connection.Close()
				return
			}
		}
		panic(http.ErrAbortHandler)
	}
	for name, values := range header {
		for _, value := range values {
			writer.Header().Add(name, value)
		}
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(payload)
}

// handle atiende una petición de la API ya leída. Devuelve también si hay que perder la respuesta.
func (server *Server) handle(action string, body []byte) (int, http.Header, []byte, bool) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if action == "challenge" {
		status, header, payload := server.challenge(body)
		return status, header, payload, false
	}
	request, fingerprint, err := parseOperation(action, body)
	if err != nil {
		status, header, payload := errorResponse(http.StatusBadRequest, licensing.CodeInvalidRequest, request.RequestIDOrNil(), nil, 0)
		server.record(action, request.RequestID, status, licensing.CodeInvalidRequest, false)
		return status, header, payload, false
	}
	requestID := request.RequestID
	if scripted, ok := server.takeScripted(action); ok {
		status, header, payload := scriptedResponse(scripted, &requestID)
		server.record(action, requestID, status, scripted.Code, false)
		return status, header, payload, false
	}
	// Reintentos (§8.7): por product_id + request_id. El mismo contenido recibe exactamente la misma respuesta; otro
	// contenido, REQUEST_CONFLICT.
	replyKey := strings.ToLower(request.ProductID + "/" + requestID)
	if stored, exists := server.replies[replyKey]; exists {
		if stored.fingerprint != action+"\n"+fingerprint {
			status, header, payload := errorResponse(http.StatusConflict, licensing.CodeRequestConflict, &requestID, nil, 0)
			server.record(action, requestID, status, licensing.CodeRequestConflict, false)
			return status, header, payload, false
		}
		server.record(action, requestID, stored.status, codeOf(stored.body), true)
		return stored.status, stored.header.Clone(), append([]byte(nil), stored.body...), false
	}
	status, header, payload := server.operation(action, request)
	// Las fallas temporales no se guardan: el reintento debe atenderse de verdad.
	if status != http.StatusServiceUnavailable && status != http.StatusTooManyRequests {
		server.replies[replyKey] = &simReply{fingerprint: action + "\n" + fingerprint, status: status, header: header.Clone(), body: payload}
	}
	server.record(action, requestID, status, codeOf(payload), false)
	drop := server.dropNext[action]
	delete(server.dropNext, action)
	return status, header, payload, drop
}

func (server *Server) record(action, requestID string, status int, code string, replayed bool) {
	server.calls = append(server.calls, Call{Action: action, RequestID: requestID, Status: status, Code: code, Replayed: replayed})
}

func (server *Server) takeScripted(action string) (Scripted, bool) {
	for index, scripted := range server.scripted {
		if scripted.Action == action {
			server.scripted = append(server.scripted[:index], server.scripted[index+1:]...)
			return scripted, true
		}
	}
	return Scripted{}, false
}

// codeOf es el error.code de una respuesta, u «OK».
func codeOf(payload []byte) string {
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(payload, &envelope) == nil && envelope.Error != nil {
		return envelope.Error.Code
	}
	return "OK"
}

// challenge atiende POST /v1/activations/challenge (§8.1).
func (server *Server) challenge(body []byte) (int, http.Header, []byte) {
	// Exactamente los cuatro campos (additionalProperties: false); activation_id puede ser null.
	var fields map[string]json.RawMessage
	var action, product, serverID string
	parsed := json.Unmarshal(body, &fields) == nil && len(fields) == 4 &&
		json.Unmarshal(fields["action"], &action) == nil && json.Unmarshal(fields["product_id"], &product) == nil &&
		json.Unmarshal(fields["server_id"], &serverID) == nil && fields["activation_id"] != nil
	if !parsed {
		server.record("challenge", "", http.StatusBadRequest, licensing.CodeInvalidRequest, false)
		return errorResponse(http.StatusBadRequest, licensing.CodeInvalidRequest, nil, nil, 0)
	}
	if scripted, ok := server.takeScripted("challenge"); ok {
		status, header, payload := scriptedResponse(scripted, nil)
		server.record("challenge", "", status, scripted.Code, false)
		return status, header, payload
	}
	activationID := ""
	nullActivation := string(bytes.TrimSpace(fields["activation_id"])) == "null"
	if !nullActivation && json.Unmarshal(fields["activation_id"], &activationID) != nil {
		return errorResponse(http.StatusBadRequest, licensing.CodeInvalidRequest, nil, nil, 0)
	}
	valid := productPattern.MatchString(product) && serverIDFormat.MatchString(serverID)
	switch action {
	case licensing.ActionActivate, licensing.ActionTransfer:
		valid = valid && nullActivation
	case licensing.ActionRefresh, licensing.ActionDeactivate:
		valid = valid && !nullActivation && uuidPattern.MatchString(activationID)
	default:
		valid = false
	}
	if !valid {
		server.record("challenge", "", http.StatusBadRequest, licensing.CodeInvalidRequest, false)
		return errorResponse(http.StatusBadRequest, licensing.CodeInvalidRequest, nil, nil, 0)
	}
	id := NewUUID()
	nonce := make([]byte, 32)
	_, _ = rand.Read(nonce)
	challenge := &simChallenge{
		action:       action,
		serverID:     serverID,
		activationID: activationID,
		nonce:        encode(nonce),
		expiresAt:    server.now().Add(5 * time.Minute),
	}
	server.challenges[id] = challenge
	server.record("challenge", "", http.StatusOK, "OK", false)
	return jsonResponse(http.StatusOK, map[string]string{
		"challenge_id": id,
		"nonce":        challenge.nonce,
		"expires_at":   instant(challenge.expiresAt),
	})
}

// operationRequest son los campos de activate, transfer, refresh y deactivate.
type operationRequest struct {
	RequestID             string `json:"request_id"`
	ProductID             string `json:"product_id"`
	LicenseKey            string `json:"license_key,omitempty"`
	ServerID              string `json:"server_id"`
	InstallationPublicKey string `json:"installation_public_key,omitempty"`
	ActivationID          string `json:"activation_id,omitempty"`
	ChallengeID           string `json:"challenge_id"`
	Proof                 string `json:"proof"`
	AppVersion            string `json:"app_version"`
}

// RequestIDOrNil es el request_id para un error: el enviado si es un UUID, o null.
func (request operationRequest) RequestIDOrNil() *string {
	if uuidPattern.MatchString(request.RequestID) {
		value := request.RequestID
		return &value
	}
	return nil
}

// parseOperation exige exactamente los campos de la acción (openapi.yaml: ActivationRequest o
// ExistingActivationRequest) y devuelve una huella del contenido para los reintentos.
func parseOperation(action string, body []byte) (operationRequest, string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return operationRequest{}, "", errors.New("no es un objeto")
	}
	expected := []string{"request_id", "product_id", "server_id", "challenge_id", "proof", "app_version"}
	if action == licensing.ActionActivate || action == licensing.ActionTransfer {
		expected = append(expected, "license_key", "installation_public_key")
	} else {
		expected = append(expected, "activation_id")
	}
	var request operationRequest
	texts := map[string]*string{
		"request_id": &request.RequestID, "product_id": &request.ProductID, "license_key": &request.LicenseKey,
		"server_id": &request.ServerID, "installation_public_key": &request.InstallationPublicKey,
		"activation_id": &request.ActivationID, "challenge_id": &request.ChallengeID, "proof": &request.Proof,
		"app_version": &request.AppVersion,
	}
	if value, ok := fields["request_id"]; ok {
		_ = json.Unmarshal(value, &request.RequestID)
	}
	if len(fields) != len(expected) {
		return request, "", errors.New("campos faltantes o sobrantes")
	}
	for _, name := range expected {
		value, ok := fields[name]
		if !ok || json.Unmarshal(value, texts[name]) != nil {
			return request, "", errors.New("campo faltante o de otro tipo")
		}
	}
	valid := uuidPattern.MatchString(request.RequestID) && productPattern.MatchString(request.ProductID) &&
		serverIDFormat.MatchString(request.ServerID) && uuidPattern.MatchString(request.ChallengeID) &&
		request.AppVersion != "" && len(request.AppVersion) <= 100
	if action == licensing.ActionActivate || action == licensing.ActionTransfer {
		valid = valid && len(request.LicenseKey) >= 1 && len(request.LicenseKey) <= 256 && len(request.InstallationPublicKey) == 43
	} else {
		valid = valid && uuidPattern.MatchString(request.ActivationID)
	}
	if !valid {
		return request, "", errors.New("formato inválido")
	}
	canonical, _ := json.Marshal(request)
	return request, string(canonical), nil
}

// operation aplica las reglas de la acción (§8.3 a §8.6).
func (server *Server) operation(action string, request operationRequest) (int, http.Header, []byte) {
	requestID := request.RequestID
	switch action {
	case licensing.ActionActivate, licensing.ActionTransfer:
		// Una falla temporal o un límite de intentos no consumen el desafío (el servidor real revierte la
		// operación): el reintento con el mismo cuerpo puede salir bien.
		if key, err := licensing.NormalizeLicenseKey(request.LicenseKey); err == nil {
			switch key[:3] {
			case "TMP":
				return errorResponse(http.StatusServiceUnavailable, licensing.CodeTemporaryUnavailable, &requestID, nil, 0)
			case "RAT":
				return errorResponse(http.StatusTooManyRequests, licensing.CodeRateLimited, &requestID, nil, 30)
			}
		}
		if err := server.useChallenge(request, action, ""); err != nil {
			return errorResponse(http.StatusUnauthorized, licensing.CodeInvalidProof, &requestID, nil, 0)
		}
		if !licensing.VerifyProof(server.proofMessage(action, request), request.Proof, request.InstallationPublicKey) {
			return errorResponse(http.StatusUnauthorized, licensing.CodeInvalidProof, &requestID, nil, 0)
		}
		return server.activateOrTransfer(action, request)
	default:
		activation := server.activations[strings.ToLower(request.ActivationID)]
		if err := server.useChallenge(request, action, request.ActivationID); err != nil || activation == nil || activation.serverID != request.ServerID {
			return errorResponse(http.StatusUnauthorized, licensing.CodeInvalidProof, &requestID, nil, 0)
		}
		if !licensing.VerifyProof(server.proofMessage(action, request), request.Proof, activation.publicKey) {
			return errorResponse(http.StatusUnauthorized, licensing.CodeInvalidProof, &requestID, nil, 0)
		}
		if action == licensing.ActionRefresh {
			return server.refresh(activation, requestID)
		}
		return server.deactivate(activation, requestID)
	}
}

func (server *Server) proofMessage(action string, request operationRequest) string {
	challenge := server.challenges[strings.ToLower(request.ChallengeID)]
	var activation *string
	if request.ActivationID != "" {
		activation = &request.ActivationID
	}
	nonce := ""
	if challenge != nil {
		nonce = challenge.nonce
	}
	return licensing.ProofMessage(action, request.ChallengeID, nonce, request.ProductID, request.ServerID, activation)
}

// useChallenge consume el desafío: de un solo uso, vigente y de esa acción, producto e instalación.
func (server *Server) useChallenge(request operationRequest, action, activationID string) error {
	challenge := server.challenges[strings.ToLower(request.ChallengeID)]
	if challenge == nil || challenge.used || !server.now().Before(challenge.expiresAt) || challenge.action != action ||
		challenge.serverID != request.ServerID || request.ProductID != licensing.ProductID ||
		!strings.EqualFold(challenge.activationID, activationID) {
		return errors.New("desafío inválido")
	}
	challenge.used = true
	return nil
}

// license devuelve la licencia de un código, creándola la primera vez según su prefijo. Nil: código inexistente.
func (server *Server) license(licenseKey string) *simLicense {
	key, err := licensing.NormalizeLicenseKey(licenseKey)
	if err != nil || !keyPattern.MatchString(key) {
		return nil
	}
	if existing := server.licenses[key]; existing != nil {
		return existing
	}
	behavior := key[:3]
	if _, known := Behaviors[behavior]; !known {
		return nil
	}
	now := server.now()
	license := &simLicense{key: key, id: NewUUID(), behavior: behavior, licenseType: TypePerpetual, plan: "elemental", licenseeName: "Restaurante simulado"}
	switch behavior {
	case "BSC":
		license.plan = "basico"
	case "MNS":
		license.licenseType = TypeSubscription
	case "PRB":
		license.licenseType = TypeTrial
		license.plan = "basico"
	case "GRC":
		license.licenseType = TypeSubscription
		license.startsAt, license.expiresAt, license.datesFixed = now.AddDate(0, -1, -2), now.AddDate(0, 0, -2), true
	case "VNC":
		license.licenseType = TypeSubscription
		license.startsAt, license.expiresAt, license.datesFixed = now.AddDate(0, -1, -40), now.AddDate(0, 0, -40), true
	case "RVK":
		license.revoked = true
	case "ACT", "ACN":
		// Activa en otra computadora (una instalación fantasma); ACN además con una transferencia pendiente.
		phantom := server.newActivation(license, NewUUID(), encode(randomPublicKey()))
		server.fixDates(license)
		if jws, err := server.sign(license, phantom, ""); err == nil {
			phantom.jws = jws
		}
		license.current = phantom
		if behavior == "ACN" {
			license.pendingFrom = &simActivation{id: NewUUID(), license: license}
		}
	}
	server.licenses[key] = license
	return license
}

func randomPublicKey() []byte {
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	return public
}

func (server *Server) newActivation(license *simLicense, serverID, publicKey string) *simActivation {
	activation := &simActivation{id: NewUUID(), license: license, serverID: serverID, publicKey: publicKey, active: true}
	server.activations[activation.id] = activation
	return activation
}

// fixDates fija las fechas en la primera activación (§5.2); las de GRC y VNC ya vienen fijas.
func (server *Server) fixDates(license *simLicense) {
	if license.datesFixed {
		return
	}
	now := server.now()
	license.startsAt = now
	switch license.licenseType {
	case TypeSubscription:
		license.expiresAt = now.AddDate(0, 1, 0)
	case TypeTrial:
		license.expiresAt = now.AddDate(0, 0, 15)
	}
	license.datesFixed = true
}

// sign firma la revisión siguiente de la licencia para una activación. reason vacío: active; si no, el cierre.
func (server *Server) sign(license *simLicense, activation *simActivation, reason string) (string, error) {
	license.revision++
	options := IssueOptions{
		ServerID:       activation.serverID,
		PublicKey:      activation.publicKey,
		Type:           license.licenseType,
		IssuedAt:       server.now(),
		PeriodStartsAt: license.startsAt,
		ExpiresAt:      license.expiresAt,
		PlanID:         license.plan,
		Revision:       license.revision,
		LicenseID:      license.id,
		ActivationID:   activation.id,
		LicenseeName:   license.licenseeName,
	}
	if reason != "" {
		options.Status = "revoked"
		options.Reason = reason
	}
	// Se firma sin verificar contra una instalación: la «otra computadora» de ACT y ACN no tiene llave privada.
	if !serverIDFormat.MatchString(activation.serverID) || len(activation.publicKey) != 43 {
		return "", errors.New("instalación inválida")
	}
	content, err := options.content()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return "", err
	}
	return server.fixtures.SignJWS(DefaultKid, raw)
}

// closeActivation firma el cierre de una activación vigente.
func (server *Server) closeActivation(activation *simActivation, reason string) error {
	jws, err := server.sign(activation.license, activation, reason)
	if err != nil {
		return err
	}
	activation.jws = jws
	activation.active = false
	activation.endedAt = server.now()
	if activation.license.current == activation {
		activation.license.current = nil
	}
	return nil
}

// activateOrTransfer son las reglas de §8.3 y §8.4, en su orden.
func (server *Server) activateOrTransfer(action string, request operationRequest) (int, http.Header, []byte) {
	requestID := request.RequestID
	license := server.license(request.LicenseKey)
	if license == nil {
		return errorResponse(http.StatusNotFound, licensing.CodeLicenseNotFound, &requestID, nil, 0)
	}
	if license.behavior == "PRF" {
		return errorResponse(http.StatusUnauthorized, licensing.CodeInvalidProof, &requestID, nil, 0)
	}
	if license.revoked {
		return errorResponse(http.StatusForbidden, licensing.CodeRevoked, &requestID, nil, 0)
	}
	current := license.current
	// La misma instalación ya está activa: su revisión vigente, sin otra activación.
	if current != nil && current.serverID == request.ServerID && current.publicKey == request.InstallationPublicKey {
		return server.licenseReply(current.jws, requestID)
	}
	if current != nil {
		if action == licensing.ActionActivate {
			available := license.pendingFrom == nil
			return errorResponse(http.StatusConflict, licensing.CodeActivationLimit, &requestID, &available, 0)
		}
		if license.pendingFrom != nil {
			return errorResponse(http.StatusConflict, licensing.CodeTransferLimit, &requestID, nil, 0)
		}
		// Transferencia por autoservicio: cierre transferred de la anterior, revisión active de la nueva, pendiente.
		if err := server.closeActivation(current, licensing.ReasonTransferred); err != nil {
			return errorResponse(http.StatusServiceUnavailable, licensing.CodeTemporaryUnavailable, &requestID, nil, 0)
		}
		license.pendingFrom = current
	}
	// Ninguna activación vigente: una activación normal (también en transfer), con su primer periodo si es la primera.
	server.fixDates(license)
	activation := server.newActivation(license, request.ServerID, request.InstallationPublicKey)
	jws, err := server.sign(license, activation, "")
	if err != nil {
		return errorResponse(http.StatusServiceUnavailable, licensing.CodeTemporaryUnavailable, &requestID, nil, 0)
	}
	activation.jws = jws
	license.current = activation
	return server.licenseReply(jws, requestID)
}

// refresh (§8.5): la revisión vigente de la activación, o su cierre; el cierre de la computadora anterior confirma
// la transferencia pendiente.
func (server *Server) refresh(activation *simActivation, requestID string) (int, http.Header, []byte) {
	if !activation.active && activation.license.pendingFrom == activation {
		activation.license.pendingFrom = nil
	}
	return server.licenseReply(activation.jws, requestID)
}

// deactivate (§8.6): libera la licencia y entrega el cierre deactivated; si ya había terminado, su cierre existente.
func (server *Server) deactivate(activation *simActivation, requestID string) (int, http.Header, []byte) {
	if activation.active {
		if err := server.closeActivation(activation, licensing.ReasonDeactivated); err != nil {
			return errorResponse(http.StatusServiceUnavailable, licensing.CodeTemporaryUnavailable, &requestID, nil, 0)
		}
	} else if activation.license.pendingFrom == activation {
		activation.license.pendingFrom = nil
	}
	jws := server.maybeAlter(activation.jws)
	return jsonResponse(http.StatusOK, map[string]string{
		"activation_id":  activation.id,
		"deactivated_at": instant(activation.endedAt),
		"status":         "deactivated",
		"license_jws":    jws,
		"server_time":    instant(server.now()),
		"request_id":     requestID,
	})
}

func (server *Server) licenseReply(jws, requestID string) (int, http.Header, []byte) {
	return jsonResponse(http.StatusOK, map[string]string{
		"license_jws": server.maybeAlter(jws),
		"server_time": instant(server.now()),
		"request_id":  requestID,
	})
}

// maybeAlter altera la firma de la siguiente licencia si se pidió (AlterNext).
func (server *Server) maybeAlter(jws string) string {
	if !server.alterNext || jws == "" {
		return jws
	}
	server.alterNext = false
	last := jws[len(jws)-2]
	replacement := byte('A')
	if last == 'A' {
		replacement = 'B'
	}
	return jws[:len(jws)-2] + string(replacement) + jws[len(jws)-1:]
}

// --- Respuestas ----------------------------------------------------------------------------------------------------

func scriptedResponse(scripted Scripted, requestID *string) (int, http.Header, []byte) {
	status := map[string]int{
		licensing.CodeInvalidRequest: 400, licensing.CodeIncompatibleSchema: 400, licensing.CodeInvalidProof: 401,
		licensing.CodeRevoked: 403, licensing.CodeLicenseNotFound: 404, licensing.CodeActivationLimit: 409,
		licensing.CodeTransferLimit: 409, licensing.CodeRequestConflict: 409, licensing.CodeRateLimited: 429,
		licensing.CodeTemporaryUnavailable: 503,
	}[scripted.Code]
	if status == 0 {
		status = 500
	}
	var transfer *bool
	if scripted.Code == licensing.CodeActivationLimit {
		value := scripted.TransferAvailable
		transfer = &value
	}
	return errorResponse(status, scripted.Code, requestID, transfer, scripted.RetryAfter)
}

func errorResponse(status int, code string, requestID *string, transferAvailable *bool, retryAfter int) (int, http.Header, []byte) {
	details := map[string]any{}
	if transferAvailable != nil {
		details["transfer_available"] = *transferAvailable
	}
	var id any
	if requestID != nil {
		id = *requestID
	}
	payload, _ := json.Marshal(map[string]any{"error": map[string]any{
		"code":       code,
		"message":    "Respuesta del servidor de licencias simulado.",
		"request_id": id,
		"details":    details,
	}})
	header := http.Header{}
	if retryAfter > 0 {
		header.Set("Retry-After", strconv.Itoa(retryAfter))
	}
	return status, header, payload
}

func writeError(writer http.ResponseWriter, status int, code string, requestID *string, transferAvailable *bool) {
	_, _, payload := errorResponse(status, code, requestID, transferAvailable, 0)
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(payload)
}

func jsonResponse(status int, value any) (int, http.Header, []byte) {
	payload, _ := json.Marshal(value)
	return status, http.Header{}, payload
}
