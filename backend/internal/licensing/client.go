package licensing

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Cliente en línea del servidor de licencias (contrato §8; §3 de la especificación; Fase B).
//
//   - HTTPS con TLS verificado, `Content-Type: application/json`, cuerpos de 16 KiB como máximo, plazos por intento,
//     sin redirecciones y sin proxy.
//   - Cada operación pide su desafío (§8.1) y firma la prueba de posesión (§8.2) con la llave de licencias.
//   - Los reintentos reenvían **el mismo cuerpo**, con el mismo `request_id` (§8.7): una respuesta perdida se
//     recupera sin duplicar nada.
//   - Se decide por `error.code`, nunca por el HTTP ni por el `message`, que nunca se muestra ni se registra.
//   - Se respeta `Retry-After`.
//   - Cada `license_jws` pasa por la §10 (Manager.Accept) antes de aceptarse, y solo el `server_time` de una respuesta
//     correcta (200, con su forma, su `request_id` y su licencia aceptada) alimenta la última hora confiable (L1-D26).
//   - `TEMPORARY_UNAVAILABLE` y los errores de red nunca cambian la licencia instalada: no se llega a Accept.
//   - En test_mode solo se habla con loopback, salvo ADEL_LICENSE_ALLOW_REMOTE=1 (L1-D18), y lo comprueba también la
//     conexión, no solo el nombre: ninguna prueba automática llega al servidor de licencias real.
//
// Solo el servidor Go principal usa este cliente (contrato §1). Nunca registra el código comercial ni la llave.

// Rutas del contrato (§8).
const (
	challengePath  = "/v1/activations/challenge"
	activatePath   = "/v1/activations"
	transferPath   = "/v1/activations/transfer"
	refreshPath    = "/v1/activations/refresh"
	deactivatePath = "/v1/activations/deactivate"
)

// Acciones del desafío y de la prueba de posesión.
const (
	ActionActivate   = "activate"
	ActionTransfer   = "transfer"
	ActionRefresh    = "refresh"
	ActionDeactivate = "deactivate"
)

// Códigos de error del contrato (§8.7).
const (
	CodeInvalidRequest       = "INVALID_REQUEST"
	CodeIncompatibleSchema   = "INCOMPATIBLE_SCHEMA"
	CodeInvalidProof         = "INVALID_PROOF"
	CodeRevoked              = "REVOKED"
	CodeLicenseNotFound      = "LICENSE_NOT_FOUND"
	CodeActivationLimit      = "ACTIVATION_LIMIT"
	CodeTransferLimit        = "TRANSFER_LIMIT"
	CodeRequestConflict      = "REQUEST_CONFLICT"
	CodeRateLimited          = "RATE_LIMITED"
	CodeTemporaryUnavailable = "TEMPORARY_UNAVAILABLE"
)

// Motivos de un TransportError (details.reason de LICENSE_SERVER_UNAVAILABLE y LICENSE_SERVER_ERROR).
const (
	ReasonNotConfigured    = "not_configured"     // la compilación no tiene dirección (producción todavía)
	ReasonRemoteNotAllowed = "remote_not_allowed" // test_mode sin ADEL_LICENSE_ALLOW_REMOTE=1 y un destino que no es loopback
	ReasonNetwork          = "network"            // sin conexión, DNS o conexión cortada
	ReasonTLS              = "tls"                // el certificado no se pudo verificar
	ReasonTimeout          = "timeout"            // se agotó el plazo
	ReasonInvalidResponse  = "invalid_response"   // una respuesta que no es la del contrato
)

const (
	// maxRequestBody es el límite del contrato para un cuerpo (§8).
	maxRequestBody = 16 << 10
	// maxResponseBody limita lo que se lee de una respuesta.
	maxResponseBody = 64 << 10
	// attemptTimeout es el plazo de cada intento.
	attemptTimeout = 15 * time.Second
	// maxAttempts son los intentos de una misma petición (el primero y dos reintentos).
	maxAttempts = 3
	// maxRetryAfterWait es la espera más larga de Retry-After que se hace dentro de una operación; una mayor se
	// devuelve a quien llamó (LICENSE_SERVER_BUSY con los segundos).
	maxRetryAfterWait = 5 * time.Second
	// challengeLifetime es la vida del desafío (§8.1): su expires_at menos esto es la hora del servidor (L1-D5).
	challengeLifetime = 5 * time.Minute
)

// ServerError es un error del contrato que respondió el servidor de licencias. Se decide por Code.
type ServerError struct {
	Code       string
	HTTPStatus int
	// RetryAfter es la espera que pidió el servidor (Retry-After), o cero.
	RetryAfter time.Duration
	// TransferAvailable es details.transfer_available de ACTIVATION_LIMIT (nil en los demás).
	TransferAvailable *bool
}

func (e *ServerError) Error() string {
	return "el servidor de licencias respondió " + e.Code
}

// TransportError es una falla sin respuesta del contrato: red, TLS, plazo, una respuesta inesperada o un destino no
// permitido.
type TransportError struct {
	Reason string
}

func (e *TransportError) Error() string {
	return "no se pudo hablar con el servidor de licencias: " + e.Reason
}

// ResponseRejectedError: la respuesta era correcta en su forma, pero su licencia no pasó la §10. Se ignora y se sigue
// con la licencia instalada (LICENSE_RESPONSE_INVALID).
type ResponseRejectedError struct {
	Reason string
}

func (e *ResponseRejectedError) Error() string {
	return "la licencia que respondió el servidor de licencias no es válida: " + e.Reason
}

// ErrNotActivated: refresh, deactivate y los .licreq de renew y deactivate necesitan una activación instalada.
var ErrNotActivated = errors.New("esta computadora no tiene una activación de licencia")

// ClientOptions configura el cliente.
type ClientOptions struct {
	// BaseURL es la dirección base (license_server_url de server.json; sin ella, DefaultServerURL).
	BaseURL string
	// AppVersion es la versión de AD-EL POS que se envía (app_version).
	AppVersion string
	// TestMode: el servidor de las pruebas solo habla con loopback, salvo AllowRemote (ADEL_LICENSE_ALLOW_REMOTE=1).
	TestMode    bool
	AllowRemote bool
	// RootCAs reemplaza a las autoridades del sistema (pruebas: el certificado del servidor simulado).
	RootCAs *x509.CertPool
	// Sleep espera entre reintentos. Sin valor, una espera real que respeta el contexto.
	Sleep func(ctx context.Context, wait time.Duration) error
	// Logger recibe solo códigos y motivos, nunca cuerpos.
	Logger *slog.Logger
}

// Client habla con el servidor de licencias en nombre del gestor. Una operación a la vez.
type Client struct {
	manager   *Manager
	options   ClientOptions
	baseURL   *url.URL
	problem   *TransportError
	http      *http.Client
	logger    *slog.Logger
	operation sync.Mutex
}

// NewClient prepara el cliente. Una dirección inválida o no permitida no impide arrancar: cada operación responde
// con su motivo (LICENSE_SERVER_UNAVAILABLE).
func NewClient(manager *Manager, options ClientOptions) *Client {
	client := &Client{manager: manager, options: options, logger: options.Logger}
	if client.logger == nil {
		client.logger = manager.logger
	}
	if client.options.Sleep == nil {
		client.options.Sleep = sleepContext
	}
	base := strings.TrimSpace(options.BaseURL)
	if base == "" {
		base = DefaultServerURL
	}
	parsed, problem := parseServerURL(base)
	if problem == nil && options.TestMode && !options.AllowRemote && !loopbackHost(parsed.Hostname()) {
		problem = &TransportError{Reason: ReasonRemoteNotAllowed}
	}
	if problem != nil {
		client.problem = problem
		client.logger.Warn("license_server_disabled", "reason", problem.Reason)
	}
	client.baseURL = parsed
	client.http = newHTTPClient(options)
	return client
}

// parseServerURL exige https, un host y nada más que una ruta base.
func parseServerURL(text string) (*url.URL, *TransportError) {
	if text == "" {
		return nil, &TransportError{Reason: ReasonNotConfigured}
	}
	parsed, err := url.Parse(text)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, &TransportError{Reason: ReasonNotConfigured}
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed, nil
}

// loopbackHost: una IP de loopback o «localhost».
func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// newHTTPClient arma el cliente HTTP: TLS 1.2 o más, sin proxy ni redirecciones y, en test_mode sin AllowRemote,
// una conexión que se niega a cualquier dirección que no sea de loopback (también si un nombre la resuelve así).
func newHTTPClient(options ClientOptions) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if options.TestMode && !options.AllowRemote {
		dialer.Control = loopbackOnly
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: options.RootCAs},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: attemptTimeout,
		MaxIdleConns:          2,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   attemptTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

var errRemoteNotAllowed = errors.New("destino no permitido en el modo de pruebas")

// loopbackOnly es el control de la conexión en test_mode: solo direcciones de loopback, ya resueltas.
func loopbackOnly(_ string, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errRemoteNotAllowed
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errRemoteNotAllowed
	}
	return nil
}

func sleepContext(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// newRequestID es un UUID versión 4 en minúsculas (§2: una por operación).
func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	text := hex.EncodeToString(value[:])
	return text[0:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:32]
}

// formatInstant es el formato que firma el servidor: UTC con seis decimales (§3).
func formatInstant(instant time.Time) string {
	return instant.UTC().Format("2006-01-02T15:04:05.000000Z")
}

// --- Operaciones -----------------------------------------------------------------------------------------------------

// Activate activa la licencia del código comercial en esta computadora (§8.3). Si la misma instalación ya está
// activa, recibe su revisión vigente. ACTIVATION_LIMIT llega como *ServerError con TransferAvailable.
func (client *Client) Activate(ctx context.Context, licenseKey string) (Status, error) {
	return client.activateOrTransfer(ctx, ActionActivate, activatePath, licenseKey)
}

// Transfer mueve la licencia a esta computadora por autoservicio (§8.4). Solo se usa después de ACTIVATION_LIMIT con
// transfer_available y la confirmación del usuario; TRANSFER_LIMIT es «Comunícate con soporte».
func (client *Client) Transfer(ctx context.Context, licenseKey string) (Status, error) {
	return client.activateOrTransfer(ctx, ActionTransfer, transferPath, licenseKey)
}

func (client *Client) activateOrTransfer(ctx context.Context, action, path, licenseKey string) (Status, error) {
	normalized, err := NormalizeLicenseKey(licenseKey)
	if err != nil {
		return client.manager.Status(), err
	}
	client.operation.Lock()
	defer client.operation.Unlock()
	installation, private, err := client.manager.signingIdentity()
	if err != nil {
		return client.manager.Status(), err
	}
	challenge, err := client.challenge(ctx, action, installation.ServerID, nil)
	if err != nil {
		return client.manager.Status(), err
	}
	proof := SignProof(private, ProofMessage(action, challenge.ChallengeID, challenge.Nonce, ProductID, installation.ServerID, nil))
	request := activationRequest{
		RequestID:             newRequestID(),
		ProductID:             ProductID,
		LicenseKey:            normalized,
		ServerID:              installation.ServerID,
		InstallationPublicKey: installation.PublicKey,
		ChallengeID:           challenge.ChallengeID,
		Proof:                 proof,
		AppVersion:            client.appVersion(),
	}
	var response licenseResponse
	if err := client.post(ctx, action, path, request, request.RequestID, &response); err != nil {
		return client.manager.Status(), err
	}
	return client.acceptResponse(action, request.RequestID, response.RequestID, response.LicenseJWS, response.ServerTime)
}

// Refresh consulta la revisión vigente de la activación (§8.5): «Buscar actualización» y la consulta en segundo plano.
// Si la activación terminó, llega su revisión de cierre.
func (client *Client) Refresh(ctx context.Context) (Status, error) {
	client.operation.Lock()
	defer client.operation.Unlock()
	status, _, err := client.refreshLocked(ctx)
	return status, err
}

func (client *Client) refreshLocked(ctx context.Context) (Status, time.Time, error) {
	installation, private, err := client.manager.signingIdentity()
	if err != nil {
		return client.manager.Status(), time.Time{}, err
	}
	activationID, ok := client.manager.currentActivation()
	if !ok {
		return client.manager.Status(), time.Time{}, ErrNotActivated
	}
	request, err := client.existingActivationRequest(ctx, ActionRefresh, installation, private, activationID)
	if err != nil {
		return client.manager.Status(), time.Time{}, err
	}
	var response licenseResponse
	if err := client.post(ctx, ActionRefresh, refreshPath, request, request.RequestID, &response); err != nil {
		return client.manager.Status(), time.Time{}, err
	}
	status, err := client.acceptResponse(ActionRefresh, request.RequestID, response.RequestID, response.LicenseJWS, response.ServerTime)
	if err != nil {
		return status, time.Time{}, err
	}
	serverTime, _ := parseInstant(response.ServerTime)
	return status, serverTime, nil
}

// Deactivate libera la licencia de esta computadora (§8.6). La revisión de cierre deja el estado «Revocada» con el
// motivo de su status_reason (deactivated, o transferred o revoked si la activación ya había terminado).
func (client *Client) Deactivate(ctx context.Context) (Status, error) {
	client.operation.Lock()
	defer client.operation.Unlock()
	installation, private, err := client.manager.signingIdentity()
	if err != nil {
		return client.manager.Status(), err
	}
	activationID, ok := client.manager.currentActivation()
	if !ok {
		return client.manager.Status(), ErrNotActivated
	}
	request, err := client.existingActivationRequest(ctx, ActionDeactivate, installation, private, activationID)
	if err != nil {
		return client.manager.Status(), err
	}
	var response deactivationResponse
	if err := client.post(ctx, ActionDeactivate, deactivatePath, request, request.RequestID, &response); err != nil {
		return client.manager.Status(), err
	}
	if response.Status != "deactivated" || !uuidAnyCase(response.ActivationID) || !instantPattern.MatchString(response.DeactivatedAt) {
		client.logger.Warn("license_server_invalid_response", "action", ActionDeactivate)
		return client.manager.Status(), &TransportError{Reason: ReasonInvalidResponse}
	}
	return client.acceptResponse(ActionDeactivate, request.RequestID, response.RequestID, response.LicenseJWS, response.ServerTime)
}

// ServerTime es la hora del servidor de licencias para «Comprobar la hora con internet» (§6.3, L1-D5).
type ServerTime struct {
	// Time es la hora del servidor de licencias, en UTC.
	Time time.Time
	// Source es de dónde salió: ServerTimeFromRefresh (el server_time de una consulta correcta, contractual) o
	// ServerTimeFromChallenge (el expires_at de un desafío menos sus 5 minutos, sin activación).
	Source string
}

// Fuentes de ServerTime.
const (
	ServerTimeFromRefresh   = "refresh"
	ServerTimeFromChallenge = "challenge"
)

// ServerTime entrega la hora del servidor de licencias (L1-D5): con una activación instalada, la de `refresh`, cuyo
// server_time es contractual (y como toda respuesta correcta también alimenta la última hora confiable y puede traer
// una revisión nueva); sin activación (o con la instalación copiada), la de un desafío de `activate`, que no se usa:
// su expires_at menos 300 s. La hora del desafío no alimenta la última hora confiable (L1-D26): quien llama decide.
func (client *Client) ServerTime(ctx context.Context) (ServerTime, error) {
	client.operation.Lock()
	defer client.operation.Unlock()
	if _, ok := client.manager.currentActivation(); ok {
		if _, _, err := client.manager.signingIdentity(); err == nil {
			_, serverTime, err := client.refreshLocked(ctx)
			if err != nil {
				return ServerTime{}, err
			}
			return ServerTime{Time: serverTime, Source: ServerTimeFromRefresh}, nil
		}
	}
	serverID := ""
	if installation, ok := client.manager.installation(); ok {
		serverID = installation.ServerID
	}
	if serverID == "" {
		return ServerTime{}, ErrIdentityUnavailable
	}
	challenge, err := client.challenge(ctx, ActionActivate, serverID, nil)
	if err != nil {
		return ServerTime{}, err
	}
	expires, ok := parseInstant(challenge.ExpiresAt)
	if !ok {
		return ServerTime{}, &TransportError{Reason: ReasonInvalidResponse}
	}
	return ServerTime{Time: expires.Add(-challengeLifetime), Source: ServerTimeFromChallenge}, nil
}

// existingActivationRequest pide el desafío y arma el cuerpo de refresh o deactivate.
func (client *Client) existingActivationRequest(ctx context.Context, action string, installation Installation, private []byte, activationID string) (existingActivationRequest, error) {
	challenge, err := client.challenge(ctx, action, installation.ServerID, &activationID)
	if err != nil {
		return existingActivationRequest{}, err
	}
	proof := SignProof(private, ProofMessage(action, challenge.ChallengeID, challenge.Nonce, ProductID, installation.ServerID, &activationID))
	return existingActivationRequest{
		RequestID:    newRequestID(),
		ProductID:    ProductID,
		ServerID:     installation.ServerID,
		ActivationID: activationID,
		ChallengeID:  challenge.ChallengeID,
		Proof:        proof,
		AppVersion:   client.appVersion(),
	}, nil
}

// challenge pide un desafío (§8.1) y comprueba su forma.
func (client *Client) challenge(ctx context.Context, action, serverID string, activationID *string) (challengeResponse, error) {
	request := challengeRequest{Action: action, ProductID: ProductID, ServerID: serverID, ActivationID: activationID}
	var response challengeResponse
	if err := client.post(ctx, action, challengePath, request, "", &response); err != nil {
		return challengeResponse{}, err
	}
	nonce, okNonce := decodeBase64URL(response.Nonce)
	if !uuidAnyCase(response.ChallengeID) || !okNonce || len(nonce) != 32 || !instantPattern.MatchString(response.ExpiresAt) {
		client.logger.Warn("license_server_invalid_response", "action", action, "step", "challenge")
		return challengeResponse{}, &TransportError{Reason: ReasonInvalidResponse}
	}
	return response, nil
}

// acceptResponse verifica la licencia de una respuesta correcta y, solo si se acepta, toma su server_time como hora
// confiable (L1-D26). Una licencia que no pasa la §10 se ignora: sigue la instalada.
func (client *Client) acceptResponse(action, sentRequestID, receivedRequestID, jws, serverTime string) (Status, error) {
	parsedTime, okTime := parseInstant(serverTime)
	if !strings.EqualFold(sentRequestID, receivedRequestID) || !okTime || jws == "" {
		client.logger.Warn("license_server_invalid_response", "action", action)
		return client.manager.Status(), &TransportError{Reason: ReasonInvalidResponse}
	}
	status, err := client.manager.Accept(jws)
	if err != nil {
		var rejected *VerifyError
		if errors.As(err, &rejected) {
			client.logger.Warn("license_response_rejected", "action", action, "reason", rejected.Code)
			return status, &ResponseRejectedError{Reason: rejected.Code}
		}
		return status, err
	}
	client.manager.ObserveTime(parsedTime, TimeFromServer)
	client.logger.Info("license_server_ok", "action", action, "state", string(status.State), "revision", status.Revision)
	return client.manager.Status(), nil
}

func (client *Client) appVersion() string {
	version := strings.TrimSpace(client.options.AppVersion)
	if version == "" || len(version) > 100 || controlPattern.MatchString(version) {
		return "0.0.0"
	}
	return version
}

// --- Transporte ------------------------------------------------------------------------------------------------------

// post envía una petición con reintentos del mismo cuerpo y decodifica la respuesta 200 en into. requestID es el de la
// petición (vacío en el desafío), solo para comprobar el del error.
func (client *Client) post(ctx context.Context, action, path string, request any, requestID string, into any) error {
	if client.problem != nil {
		return client.problem
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if len(body) > maxRequestBody {
		return &TransportError{Reason: ReasonInvalidResponse}
	}
	endpoint := client.baseURL.String() + path
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		status, header, payload, transportErr := client.send(ctx, endpoint, body)
		if transportErr != nil {
			err := transportErr
			lastErr = err
			client.logger.Warn("license_server_unreachable", "action", action, "attempt", attempt, "reason", err.Reason)
			if ctx.Err() != nil || attempt == maxAttempts {
				return err
			}
			if waitErr := client.options.Sleep(ctx, backoff(attempt)); waitErr != nil {
				return err
			}
			continue
		}
		if status == http.StatusOK {
			decoder := json.NewDecoder(bytes.NewReader(payload))
			if err := decoder.Decode(into); err != nil {
				client.logger.Warn("license_server_invalid_response", "action", action)
				return &TransportError{Reason: ReasonInvalidResponse}
			}
			return nil
		}
		serverErr := parseServerError(status, header, payload)
		if serverErr == nil {
			client.logger.Warn("license_server_invalid_response", "action", action, "http_status", status)
			return &TransportError{Reason: ReasonInvalidResponse}
		}
		lastErr = serverErr
		client.logger.Warn("license_server_error", "action", action, "attempt", attempt, "code", serverErr.Code)
		wait, retry := retryWait(serverErr, attempt)
		if !retry || attempt == maxAttempts {
			return serverErr
		}
		if waitErr := client.options.Sleep(ctx, wait); waitErr != nil {
			return serverErr
		}
	}
	return lastErr
}

// send hace un intento. Una falla sin respuesta HTTP es un *TransportError.
func (client *Client) send(ctx context.Context, endpoint string, body []byte) (int, http.Header, []byte, *TransportError) {
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, &TransportError{Reason: ReasonNotConfigured}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "AD-EL POS/"+client.appVersion())
	response, err := client.http.Do(request)
	if err != nil {
		return 0, nil, nil, classifyTransport(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody+1))
	if err != nil {
		return 0, nil, nil, classifyTransport(err)
	}
	if len(payload) > maxResponseBody {
		return 0, nil, nil, &TransportError{Reason: ReasonInvalidResponse}
	}
	return response.StatusCode, response.Header, payload, nil
}

// classifyTransport da el motivo de una falla sin respuesta.
func classifyTransport(err error) *TransportError {
	if errors.Is(err, errRemoteNotAllowed) {
		return &TransportError{Reason: ReasonRemoteNotAllowed}
	}
	var certificate *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	if errors.As(err, &certificate) || errors.As(err, &unknownAuthority) || errors.As(err, &hostname) || errors.As(err, &invalid) {
		return &TransportError{Reason: ReasonTLS}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &TransportError{Reason: ReasonTimeout}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &TransportError{Reason: ReasonTimeout}
	}
	return &TransportError{Reason: ReasonNetwork}
}

// backoff es la espera antes del reintento tras una falla de red o una falla temporal.
func backoff(attempt int) time.Duration {
	return time.Duration(attempt) * time.Second
}

// retryWait decide si se reintenta un error del contrato con el mismo cuerpo: solo TEMPORARY_UNAVAILABLE y
// RATE_LIMITED, y solo si la espera pedida (Retry-After) cabe en la operación.
func retryWait(serverErr *ServerError, attempt int) (time.Duration, bool) {
	switch serverErr.Code {
	case CodeTemporaryUnavailable, CodeRateLimited:
		wait := backoff(attempt)
		if serverErr.RetryAfter > 0 {
			wait = serverErr.RetryAfter
		}
		if serverErr.Code == CodeRateLimited && serverErr.RetryAfter == 0 {
			return 0, false
		}
		return wait, wait <= maxRetryAfterWait
	default:
		return 0, false
	}
}

// parseServerError lee un error del contrato ({"error":{code, message, request_id, details}}). Devuelve nil si no
// tiene esa forma o el código no es del contrato.
func parseServerError(status int, header http.Header, payload []byte) *ServerError {
	var envelope struct {
		Error *struct {
			Code    string          `json:"code"`
			Message *string         `json:"message"`
			Details json.RawMessage `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Error == nil {
		return nil
	}
	if !knownServerCode(envelope.Error.Code) {
		return nil
	}
	result := &ServerError{Code: envelope.Error.Code, HTTPStatus: status, RetryAfter: parseRetryAfter(header.Get("Retry-After"))}
	if result.Code == CodeActivationLimit {
		var details struct {
			TransferAvailable *bool `json:"transfer_available"`
		}
		if len(envelope.Error.Details) > 0 && json.Unmarshal(envelope.Error.Details, &details) == nil {
			result.TransferAvailable = details.TransferAvailable
		}
		if result.TransferAvailable == nil {
			// Sin el dato, no se ofrece la transferencia.
			unavailable := false
			result.TransferAvailable = &unavailable
		}
	}
	return result
}

func knownServerCode(code string) bool {
	switch code {
	case CodeInvalidRequest, CodeIncompatibleSchema, CodeInvalidProof, CodeRevoked, CodeLicenseNotFound,
		CodeActivationLimit, CodeTransferLimit, CodeRequestConflict, CodeRateLimited, CodeTemporaryUnavailable:
		return true
	default:
		return false
	}
}

// parseRetryAfter acepta segundos o una fecha HTTP. Cero si falta o no se entiende.
func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if instant, err := http.ParseTime(value); err == nil {
		if wait := time.Until(instant); wait > 0 {
			return wait
		}
	}
	return 0
}

// parseInstant lee un instante del contrato (UTC con Z y 0 a 9 decimales).
func parseInstant(text string) (time.Time, bool) {
	if !instantPattern.MatchString(text) {
		return time.Time{}, false
	}
	instant, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, false
	}
	return instant.UTC(), true
}

// uuidAnyCase: los UUID distintos de server_id se aceptan en mayúsculas o minúsculas (§2).
func uuidAnyCase(text string) bool {
	return uuidPattern.MatchString(strings.ToLower(text))
}

// --- Cuerpos del contrato (openapi.yaml) -----------------------------------------------------------------------------

type challengeRequest struct {
	Action       string  `json:"action"`
	ProductID    string  `json:"product_id"`
	ServerID     string  `json:"server_id"`
	ActivationID *string `json:"activation_id"`
}

type challengeResponse struct {
	ChallengeID string `json:"challenge_id"`
	Nonce       string `json:"nonce"`
	ExpiresAt   string `json:"expires_at"`
}

type activationRequest struct {
	RequestID             string `json:"request_id"`
	ProductID             string `json:"product_id"`
	LicenseKey            string `json:"license_key"`
	ServerID              string `json:"server_id"`
	InstallationPublicKey string `json:"installation_public_key"`
	ChallengeID           string `json:"challenge_id"`
	Proof                 string `json:"proof"`
	AppVersion            string `json:"app_version"`
}

type existingActivationRequest struct {
	RequestID    string `json:"request_id"`
	ProductID    string `json:"product_id"`
	ServerID     string `json:"server_id"`
	ActivationID string `json:"activation_id"`
	ChallengeID  string `json:"challenge_id"`
	Proof        string `json:"proof"`
	AppVersion   string `json:"app_version"`
}

type licenseResponse struct {
	LicenseJWS string `json:"license_jws"`
	ServerTime string `json:"server_time"`
	RequestID  string `json:"request_id"`
}

type deactivationResponse struct {
	ActivationID  string `json:"activation_id"`
	DeactivatedAt string `json:"deactivated_at"`
	Status        string `json:"status"`
	LicenseJWS    string `json:"license_jws"`
	ServerTime    string `json:"server_time"`
	RequestID     string `json:"request_id"`
}
