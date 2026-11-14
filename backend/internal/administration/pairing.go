package administration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Emparejamiento de terminales con código (etapa 9G, fase B; §3.2 y decisión D5).
//
// Protocolo (la terminal T habla por HTTPS con la escucha de red; Rust fija durante toda la sesión el
// primer certificado que ve y calcula su huella SPKI F_T):
//
//  1. T genera un número aleatorio nT de 32 bytes y envía la solicitud con el compromiso C = SHA-256(nT)
//     (POST pairing/requests: nombre propuesto, tipo, sistema, versión de la aplicación y C). El servidor S
//     responde con su número aleatorio nS de 32 bytes, el id de la solicitud y un secreto propio de la
//     solicitud (solo su SHA-256 queda en la base) con el que T la consulta.
//  2. T revela nT (POST pairing/requests/{id}/reveal). S comprueba SHA-256(nT) = C; si no coincide, rechaza
//     la solicitud.
//  3. Ambos derivan el código de 6 dígitos con PairingCode(F, nT, nS, id): T con F_T (la huella del
//     certificado que recibió) y S con su propia huella F_S. T muestra el código; en la computadora
//     principal un usuario con devices.manage lo escribe (POST devices/pairing-requests/{id}/approve, solo
//     por la escucha de loopback). Con F_T ≠ F_S (un intermediario con otro certificado) los códigos no
//     coinciden.
//  4. T consulta el estado con su secreto y, aprobada la solicitud, reclama una sola vez su credencial de
//     dispositivo (POST pairing/requests/{id}/claim), que Rust guarda en el almacén seguro.
//
// Qué protege el compromiso y qué no (revisión independiente, hallazgo 1). M está entre T y S y debe igualar
// dos códigos: el que T calcula con la huella de M (código_T = H(F_M ‖ nT ‖ nS' ‖ id')) y el que S calcula con
// la suya (código_S = H(F_S ‖ nT' ‖ nS ‖ id)). El compromiso impide ajustar los números dentro de una misma
// solicitud: T no revela nT hasta recibir nS', así que M fija nS' sin conocer nT; y S no entrega nS hasta
// recibir el compromiso de nT'. Pero en cuanto T revela nT, código_T queda fijo y M lo conoce, y M sí puede
// abrir muchas solicitudes en S: cada una le da un nS nuevo con el que calcula sin conexión el código_S que
// esperaría S, y si no coincide la abandona (la cancela o la deja vencer) y prueba otra. Cada solicitud es un
// intento con probabilidad 1/10⁶, así que la seguridad real la da cuántas solicitudes acepta S: hay un
// tope global de pairingGlobalLimit solicitudes creadas por ventana de pairingWindow, sin importar su estado
// ni su origen (las canceladas y rechazadas siguen contando), además del límite por grupo de origen (la IPv4,
// el /64 de una IPv6 y todo el enlace local como un solo grupo). Con 30 cada 10 minutos, M tiene como máximo
// 30 intentos por cada 10 minutos que el administrador tarde en escribir el código: probabilidad ≤ 3×10⁻⁵ por
// ventana (antes del tope, unas 3 000 pruebas en 3 s, del orden de 1–5 % por emparejamiento). Además, cada
// intento abandonado queda a la vista: Terminales muestra en la computadora principal cuántas solicitudes se
// cancelaron, rechazaron o vencieron sin aprobarse en la última ventana y si se alcanzó el tope (y el Registro de
// movimientos lo anota una vez por ventana).

const (
	// pairingLifetime: el código vence a los 5 minutos (D5); aprobada la solicitud, la terminal tiene
	// otros 5 minutos para recoger su credencial.
	pairingLifetime = 5 * time.Minute
	// pairingAttempts: 5 códigos incorrectos rechazan la solicitud.
	pairingAttempts = 5
	// Límites (hallazgo 1 de la revisión): en cada ventana de 10 minutos, 5 solicitudes por grupo de origen y
	// 30 en total, cuente su estado lo que cuente. pairingRetention: el trabajador de limpieza borra por lotes
	// las de más de 7 días (pairing_cleanup.go).
	pairingPerSource       = 5
	pairingWindow          = 10 * time.Minute
	pairingGlobalLimit     = 30
	pairingRetention       = 7 * 24 * time.Hour
	pairingCodeDigits      = 6
	pairingCodeModulus     = 1000000
	pairingNonceHexLength  = 64
	pairingSecretLength    = 43
	pairingNameMaxLength   = 80
	pairingOSMaxLength     = 80
	pairingVersionMaxChars = 40
)

// ServerIdentity es la identidad pública del servidor que usa el emparejamiento: su identificador y la
// huella SPKI de su certificado (localsetup.EnsureIdentity). Sin identidad no hay emparejamiento.
type ServerIdentity struct {
	ServerID    string
	Fingerprint [32]byte
}

// DeviceKinds es la lista cerrada de tipos de dispositivo (D9): escritorio y, en el futuro, comandero
// Android y cocina. Coincide con los CHECK de la migración 0023.
var DeviceKinds = []string{"desktop", "android", "kitchen"}

var hexNoncePattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// PairingCode deriva el código de 6 dígitos de la solicitud: SHA-256(huella SPKI ‖ nT ‖ nS ‖ id) con los
// cuatro campos en binario (32 + 32 + 32 + 16 bytes); los primeros 8 bytes del resumen como entero sin
// signo big-endian, módulo 10⁶, con ceros a la izquierda. Rust (desktop/src-tauri/src/pairing.rs) y el
// puente de las pruebas (desktop/e2e/native-driver.ts) calculan exactamente lo mismo.
func PairingCode(fingerprint [32]byte, terminalNonce, serverNonce []byte, requestID string) (string, error) {
	if len(terminalNonce) != 32 || len(serverNonce) != 32 {
		return "", errors.New("los números aleatorios deben tener 32 bytes")
	}
	identifier, err := uuidBytes(requestID)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	digest.Write(fingerprint[:])
	digest.Write(terminalNonce)
	digest.Write(serverNonce)
	digest.Write(identifier[:])
	sum := digest.Sum(nil)
	value := binary.BigEndian.Uint64(sum[:8]) % pairingCodeModulus
	return fmt.Sprintf("%0*d", pairingCodeDigits, value), nil
}

// PairingCommitment es el compromiso de la terminal: SHA-256(nT) en hexadecimal.
func PairingCommitment(terminalNonce []byte) string {
	sum := sha256.Sum256(terminalNonce)
	return hex.EncodeToString(sum[:])
}

func uuidBytes(text string) ([16]byte, error) {
	var value [16]byte
	if !identifierPattern.MatchString(text) {
		return value, errors.New("identificador inválido")
	}
	decoded, err := hex.DecodeString(strings.ReplaceAll(text, "-", ""))
	if err != nil || len(decoded) != 16 {
		return value, errors.New("identificador inválido")
	}
	copy(value[:], decoded)
	return value, nil
}

func decodeNonce(text string) ([]byte, bool) {
	if !hexNoncePattern.MatchString(text) {
		return nil, false
	}
	value, err := hex.DecodeString(text)
	return value, err == nil && len(value) == 32
}

// cleanLabel recorta y valida un texto corto que la terminal informa (nombre, sistema, versión): sin
// caracteres de control y con un máximo de caracteres.
func cleanLabel(text string, maximum int) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > maximum {
		return "", false
	}
	for _, character := range text {
		if unicode.IsControl(character) {
			return "", false
		}
	}
	return text, true
}

func pairingFault(status int, code, message string) error {
	return &Fault{Status: status, Code: code, Message: message}
}

func pairingNotFound() error {
	return pairingFault(404, "PAIRING_NOT_FOUND", "La solicitud de emparejamiento no existe o ya no es válida.")
}

// pairingStateSQL calcula el estado visible: una solicitud pendiente o aprobada cuyo plazo terminó está
// vencida, aunque la fila conserve su estado.
const pairingStateSQL = `CASE WHEN r.state IN ('pending','approved') AND r.expires_at <= now() THEN 'expired' ELSE r.state END`

// pairingPublic atiende las rutas de la terminal que se empareja (sin credencial de dispositivo):
// pairing/requests y pairing/requests/{id}[/reveal|/claim|/cancel|/name].
func (service *Service) pairingPublic(ctx context.Context, tx pgx.Tx, request Request, route string) (any, error) {
	if service.Identity == nil {
		return nil, pairingUnavailable(service)
	}
	parts := strings.Split(route, "/")
	if len(parts) < 2 || parts[1] != "requests" {
		return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
	}
	if len(parts) == 2 && request.Method == "POST" {
		return service.createPairingRequest(ctx, tx, request)
	}
	if len(parts) < 3 || !identifierPattern.MatchString(parts[2]) {
		return nil, pairingNotFound()
	}
	requestID := parts[2]
	if len(parts) == 3 && request.Method == "GET" {
		return pairingStatus(ctx, tx, request, requestID)
	}
	if len(parts) == 4 && request.Method == "POST" {
		switch parts[3] {
		case "reveal":
			return revealPairing(ctx, tx, request, requestID)
		case "claim":
			return service.claimPairing(ctx, tx, request, requestID)
		case "cancel":
			return cancelPairing(ctx, tx, request, requestID)
		case "name":
			return renamePairing(ctx, tx, request, requestID)
		}
	}
	return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
}

func (service *Service) createPairingRequest(ctx context.Context, tx pgx.Tx, request Request) (any, error) {
	var input struct {
		Name       string `json:"name"`
		DeviceKind string `json:"device_kind"`
		OS         string `json:"os"`
		AppVersion string `json:"app_version"`
		Commitment string `json:"commitment"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	name, validName := cleanLabel(input.Name, pairingNameMaxLength)
	system, validSystem := cleanLabel(input.OS, pairingOSMaxLength)
	version, validVersion := cleanLabel(input.AppVersion, pairingVersionMaxChars)
	if !validName || !validSystem || !validVersion || !hexNoncePattern.MatchString(input.Commitment) {
		return nil, invalid("Revisa el nombre de la terminal (máximo 80 caracteres).")
	}
	validKind := false
	for _, kind := range DeviceKinds {
		if kind == input.DeviceKind {
			validKind = true
		}
	}
	if !validKind {
		return nil, invalid("Tipo de terminal no reconocido.")
	}
	source := net.ParseIP(request.SourceAddress)
	if source == nil {
		return nil, pairingFault(400, "PAIRING_SOURCE_UNKNOWN", "No se pudo identificar la conexión de la terminal.")
	}
	var establishment string
	err := tx.QueryRow(ctx, "SELECT name FROM app.establishment").Scan(&establishment)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pairingFault(409, "SETUP_REQUIRED", "El servidor todavía no está preparado. Termina primero la configuración en la computadora principal.")
	}
	if err != nil {
		return nil, err
	}
	// Las solicitudes se crean de una en una: así los límites se cuentan sin carreras. Es una operación poco
	// frecuente y la transacción es corta: los dos conteos recorren solo la ventana (índices por created_at y
	// por grupo de origen), acotada por el propio tope global.
	if _, err := tx.Exec(ctx, "LOCK TABLE app.device_pairing_requests IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return nil, err
	}
	group := SourceGroup(source)
	var recentGroup, recentAll int
	err = tx.QueryRow(ctx, `SELECT
            (SELECT count(*) FROM app.device_pairing_requests WHERE source_group = $1 AND created_at > now() - $2::interval),
            (SELECT count(*) FROM app.device_pairing_requests WHERE created_at > now() - $2::interval)`,
		group, pairingWindow.String()).Scan(&recentGroup, &recentAll)
	if err != nil {
		return nil, err
	}
	if recentAll >= pairingGlobalLimit {
		return nil, pairingLimitReached(ctx, tx)
	}
	if recentGroup >= pairingPerSource {
		return nil, pairingFault(429, "PAIRING_RATE_LIMITED", "Esta terminal ya pidió varios códigos. Espera unos minutos antes de intentarlo de nuevo.")
	}
	serverNonce := make([]byte, 32)
	if _, err := rand.Read(serverNonce); err != nil {
		return nil, err
	}
	secret := NewToken()
	expires := time.Now().Add(pairingLifetime)
	var requestID string
	// La creación no se audita (una terminal que pide y cancela sin revelar su número no deja movimientos):
	// el Registro de movimientos anota la solicitud cuando la terminal revela su número y puede aprobarse.
	err = tx.QueryRow(ctx, `INSERT INTO app.device_pairing_requests(proposed_name,device_kind,os,app_version,source_address,
            source_group,secret_hash,terminal_commitment,server_nonce,server_fingerprint,expires_at)
        VALUES ($1,$2,$3,$4,$5::inet,$6,$7,$8,$9,$10,$11) RETURNING id`,
		name, input.DeviceKind, system, version, source.String(), group, TokenHash(secret), input.Commitment,
		hex.EncodeToString(serverNonce), hex.EncodeToString(service.Identity.Fingerprint[:]), expires).Scan(&requestID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"request_id":     requestID,
		"server_nonce":   hex.EncodeToString(serverNonce),
		"pairing_secret": secret,
		"expires_at":     expires,
		"server_id":      service.Identity.ServerID,
		"server_name":    establishment,
		"name":           name,
	}, nil
}

// SourceGroup es el grupo de origen del límite por terminal: la IPv4 exacta, el prefijo /64 de una IPv6 (una
// terminal puede tomar cualquier dirección de su /64) y todo el enlace local (169.254.0.0/16 y fe80::/10) como un
// solo grupo, porque ahí cada equipo elige direcciones a voluntad.
func SourceGroup(address net.IP) string {
	if v4 := address.To4(); v4 != nil {
		if v4.IsLinkLocalUnicast() {
			return "link-local"
		}
		return "v4:" + v4.String()
	}
	if address.IsLinkLocalUnicast() {
		return "link-local"
	}
	prefix := address.Mask(net.CIDRMask(64, 128))
	return "v6:" + prefix.String() + "/64"
}

// pairingLimitReached responde que se alcanzó el tope global. La primera vez en cada ventana lo anota en el
// Registro de movimientos (un solo movimiento por ventana, aunque se insista), y esa anotación se conserva.
func pairingLimitReached(ctx context.Context, tx pgx.Tx) error {
	fault := &Fault{
		Status:  429,
		Code:    "PAIRING_LIMIT_REACHED",
		Message: "Se pidieron demasiados códigos de terminal en los últimos minutos. Espera unos 10 minutos y vuelve a intentarlo; si nadie del restaurante los pidió, avisa al administrador.",
	}
	var noted bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.audit_events
        WHERE action = 'device.pairing_limit_reached' AND occurred_at > now() - $1::interval)`, pairingWindow.String()).Scan(&noted)
	if err != nil {
		return err
	}
	if !noted {
		if err := auditAs(ctx, tx, "terminal_nueva", "pairing", nil, "device.pairing_limit_reached", "device_pairing_request", nil,
			map[string]int{"limit": pairingGlobalLimit, "window_minutes": int(pairingWindow / time.Minute)}, false); err != nil {
			return err
		}
		fault.Commit = true
	}
	return fault
}

// pairingUnavailable explica por qué no se empareja: sin identidad cargada (dañada o incompleta) el servidor
// sigue atendiendo a la propia computadora, pero no a terminales nuevas.
func pairingUnavailable(service *Service) error {
	if service.IdentityProblem != "" {
		return pairingFault(503, "PAIRING_UNAVAILABLE", "Este servidor no puede emparejar terminales porque su identidad necesita revisión de soporte.")
	}
	return pairingFault(503, "PAIRING_UNAVAILABLE", "Este servidor todavía no puede emparejar terminales.")
}

// auditPairing registra un movimiento de una terminal que todavía no tiene identidad (la solicitud de
// emparejamiento): autor «pairing», sin usuario ni dispositivo.
func auditPairing(ctx context.Context, tx pgx.Tx, action, requestID string, after any) error {
	return auditAs(ctx, tx, "terminal_nueva", "pairing", nil, action, "device_pairing_request", &requestID, after, true)
}

// auditAs inserta un movimiento con un autor explícito (los datos nunca contienen secretos ni números
// del emparejamiento).
func auditAs(ctx context.Context, tx pgx.Tx, actorName, actorKind string, deviceID *string, action, entity string, entityID *string, after any, success bool) error {
	encoded, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.audit_events(actor_username,actor_kind,device_id,action,entity_type,entity_id,success,before_state,after_state)
        VALUES ($1,$2,$3,$4,$5,$6,$7,'null'::jsonb,$8)`, actorName, actorKind, deviceID, action, entity, entityID, success, encoded)
	return err
}

// pairingRow es la solicitud de emparejamiento leída con su secreto (bloqueada para actualizarla).
type pairingRow struct {
	state           string
	visibleState    string
	name            string
	kind            string
	system          string
	version         string
	commitment      string
	terminalNonce   *string
	serverNonce     string
	fingerprint     string
	failedAttempts  int
	approvedName    *string
	expiresAt       time.Time
	deviceID        *string
	revealedAlready bool
}

// lockPairing lee la solicitud y la bloquea hasta el fin de la transacción. withSecret exige que el
// secreto de la petición coincida (rutas de la terminal); un id inexistente o un secreto distinto
// responden igual, sin revelar si la solicitud existe.
func lockPairing(ctx context.Context, tx pgx.Tx, requestID, secret string, withSecret bool) (pairingRow, error) {
	var row pairingRow
	var secretHash string
	err := tx.QueryRow(ctx, `SELECT r.state, `+pairingStateSQL+`, r.proposed_name, r.device_kind, r.os, r.app_version,
            r.terminal_commitment, r.terminal_nonce, r.server_nonce, r.server_fingerprint, r.failed_attempts,
            r.approved_name, r.expires_at, r.device_id, r.secret_hash
        FROM app.device_pairing_requests r WHERE r.id = $1 FOR UPDATE`, requestID).Scan(
		&row.state, &row.visibleState, &row.name, &row.kind, &row.system, &row.version, &row.commitment,
		&row.terminalNonce, &row.serverNonce, &row.fingerprint, &row.failedAttempts, &row.approvedName,
		&row.expiresAt, &row.deviceID, &secretHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, pairingNotFound()
	}
	if err != nil {
		return row, err
	}
	row.revealedAlready = row.terminalNonce != nil
	if withSecret {
		if len(secret) != pairingSecretLength || subtle.ConstantTimeCompare([]byte(TokenHash(secret)), []byte(secretHash)) != 1 {
			return row, pairingNotFound()
		}
	}
	return row, nil
}

// pairingStateFault explica por qué una solicitud ya no admite la operación.
func pairingStateFault(state string) error {
	switch state {
	case "expired":
		return pairingFault(409, "PAIRING_EXPIRED", "El código venció. Pide un código nuevo en la terminal.")
	case "rejected":
		return pairingFault(409, "PAIRING_REJECTED", "La solicitud fue rechazada.")
	case "cancelled":
		return pairingFault(409, "PAIRING_CANCELLED", "La terminal canceló la solicitud.")
	case "claimed":
		return pairingFault(409, "PAIRING_ALREADY_CLAIMED", "Esta terminal ya recibió su autorización; para repetirlo hay que emparejarla de nuevo.")
	case "approved":
		return pairingFault(409, "PAIRING_ALREADY_APPROVED", "La solicitud ya fue aprobada.")
	}
	return pairingFault(409, "PAIRING_NOT_PENDING", "La solicitud ya no está pendiente.")
}

func pairingStatus(ctx context.Context, tx pgx.Tx, request Request, requestID string) (any, error) {
	row, err := lockPairing(ctx, tx, requestID, request.PairingSecret, true)
	if err != nil {
		return nil, err
	}
	name := row.name
	if row.approvedName != nil {
		name = *row.approvedName
	}
	return map[string]any{"state": row.visibleState, "expires_at": row.expiresAt, "name": name}, nil
}

func revealPairing(ctx context.Context, tx pgx.Tx, request Request, requestID string) (any, error) {
	var input struct {
		TerminalNonce string `json:"terminal_nonce"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	row, err := lockPairing(ctx, tx, requestID, request.PairingSecret, true)
	if err != nil {
		return nil, err
	}
	if row.visibleState != "pending" {
		return nil, pairingStateFault(row.visibleState)
	}
	if row.revealedAlready {
		return nil, pairingFault(409, "PAIRING_ALREADY_REVEALED", "La terminal ya envió su parte del código.")
	}
	nonce, valid := decodeNonce(input.TerminalNonce)
	// El número revelado debe ser el comprometido al crear la solicitud; si no, la solicitud se rechaza (es
	// un error de la terminal o alguien intenta ajustar el código).
	if !valid || subtle.ConstantTimeCompare([]byte(PairingCommitment(nonce)), []byte(row.commitment)) != 1 {
		if _, err := tx.Exec(ctx, `UPDATE app.device_pairing_requests
                SET state='rejected', reject_reason='commitment', decided_at=now() WHERE id=$1`, requestID); err != nil {
			return nil, err
		}
		if err := auditAs(ctx, tx, "terminal_nueva", "pairing", nil, "device.pairing_rejected", "device_pairing_request", &requestID,
			map[string]string{"reason": "commitment"}, false); err != nil {
			return nil, err
		}
		return nil, &Fault{Status: 400, Code: "PAIRING_COMMITMENT_MISMATCH", Message: "La terminal envió un código que no corresponde a su solicitud. Pide un código nuevo.", Commit: true}
	}
	if _, err := tx.Exec(ctx, `UPDATE app.device_pairing_requests SET terminal_nonce=$2, revealed_at=now() WHERE id=$1`,
		requestID, input.TerminalNonce); err != nil {
		return nil, err
	}
	// Desde aquí la solicitud se puede aprobar: queda en el Registro de movimientos.
	if err := auditPairing(ctx, tx, "device.pairing_requested", requestID, map[string]any{
		"name": row.name, "device_kind": row.kind, "os": row.system, "app_version": row.version,
	}); err != nil {
		return nil, err
	}
	return map[string]any{"state": "pending", "expires_at": row.expiresAt}, nil
}

func renamePairing(ctx context.Context, tx pgx.Tx, request Request, requestID string) (any, error) {
	var input struct {
		Name string `json:"name"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	name, valid := cleanLabel(input.Name, pairingNameMaxLength)
	if !valid {
		return nil, invalid("Revisa el nombre de la terminal (máximo 80 caracteres).")
	}
	row, err := lockPairing(ctx, tx, requestID, request.PairingSecret, true)
	if err != nil {
		return nil, err
	}
	if row.visibleState != "pending" {
		return nil, pairingStateFault(row.visibleState)
	}
	if _, err := tx.Exec(ctx, "UPDATE app.device_pairing_requests SET proposed_name=$2 WHERE id=$1", requestID, name); err != nil {
		return nil, err
	}
	return map[string]any{"state": "pending", "name": name}, nil
}

func cancelPairing(ctx context.Context, tx pgx.Tx, request Request, requestID string) (any, error) {
	row, err := lockPairing(ctx, tx, requestID, request.PairingSecret, true)
	if err != nil {
		return nil, err
	}
	if row.visibleState != "pending" && row.visibleState != "approved" {
		return map[string]any{"state": row.visibleState}, nil
	}
	if _, err := tx.Exec(ctx, "UPDATE app.device_pairing_requests SET state='cancelled', decided_at=now() WHERE id=$1", requestID); err != nil {
		return nil, err
	}
	// Una solicitud cancelada antes de revelar el número no se auditó al crearse y tampoco se audita ahora
	// (sigue contando para los límites y para el aviso de Terminales).
	if row.revealedAlready {
		if err := auditPairing(ctx, tx, "device.pairing_cancelled", requestID, nil); err != nil {
			return nil, err
		}
	}
	return map[string]any{"state": "cancelled"}, nil
}

// claimPairing entrega una sola vez la credencial de dispositivo: crea la terminal con un token nuevo de
// 256 bits (en la base solo su SHA-256) y marca la solicitud como reclamada en la misma transacción, con la
// fila bloqueada. Un segundo reclamo, también simultáneo, recibe PAIRING_ALREADY_CLAIMED sin token.
func (service *Service) claimPairing(ctx context.Context, tx pgx.Tx, request Request, requestID string) (any, error) {
	row, err := lockPairing(ctx, tx, requestID, request.PairingSecret, true)
	if err != nil {
		return nil, err
	}
	if row.visibleState != "approved" {
		if row.visibleState == "pending" {
			return nil, pairingFault(409, "PAIRING_NOT_APPROVED", "La solicitud todavía no se aprueba en la computadora principal.")
		}
		return nil, pairingStateFault(row.visibleState)
	}
	name := row.name
	if row.approvedName != nil {
		name = *row.approvedName
	}
	token := NewToken()
	var deviceID string
	err = tx.QueryRow(ctx, `INSERT INTO app.devices(name,secret_hash,kind,os,app_version,paired_at,server_fingerprint)
        VALUES ($1,$2,$3,$4,$5,now(),$6) RETURNING id`, name, TokenHash(token), row.kind, row.system, row.version, row.fingerprint).Scan(&deviceID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE app.device_pairing_requests SET state='claimed', device_id=$2, claimed_at=now() WHERE id=$1`,
		requestID, deviceID); err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, Principal{DeviceID: deviceID}, "device.paired", "device", &deviceID, nil, map[string]string{
		"name": name, "kind": row.kind, "os": row.system, "app_version": row.version, "pairing_request_id": requestID,
	}, true); err != nil {
		return nil, err
	}
	var establishment string
	if err := tx.QueryRow(ctx, "SELECT name FROM app.establishment").Scan(&establishment); err != nil {
		return nil, err
	}
	return map[string]any{
		"device_id":    deviceID,
		"device_token": token,
		"server_id":    service.Identity.ServerID,
		"server_name":  establishment,
		"name":         name,
	}, nil
}

// pairingAdministration atiende las solicitudes en Sistema › Terminales: devices/pairing-requests (lista,
// devices.read) y devices/pairing-requests/{id}/approve|reject (devices.manage, solo en la computadora
// principal: la petición debe llegar por la escucha de loopback).
func (service *Service) pairingAdministration(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string) (any, error) {
	parts := strings.Split(route, "/")
	if len(parts) == 2 && request.Method == "GET" {
		if err := principal.require("devices.read"); err != nil {
			return nil, err
		}
		requests, err := list(ctx, tx, `SELECT r.id, r.proposed_name AS name, r.device_kind, r.os, r.app_version, r.created_at,
                r.expires_at, r.revealed_at IS NOT NULL AS ready, `+fmt.Sprint(pairingAttempts)+` - r.failed_attempts AS attempts_left
            FROM app.device_pairing_requests r
            WHERE r.state = 'pending' AND r.expires_at > now()
            ORDER BY r.created_at`)
		if err != nil {
			return nil, err
		}
		// La aprobación solo se acepta en la computadora principal: la interfaz de otra terminal muestra
		// las solicitudes sin el campo del código.
		// Indicio de un intento de suplantación (hallazgo 1): solicitudes de la última ventana que se cancelaron,
		// se rechazaron o vencieron sin aprobarse, y si se alcanzó el tope global.
		var abandoned, recentAll int
		err = tx.QueryRow(ctx, `SELECT
                count(*) FILTER (WHERE state IN ('cancelled','rejected') OR (state IN ('pending','approved') AND expires_at <= now())),
                count(*)
            FROM app.device_pairing_requests WHERE created_at > now() - $1::interval`, pairingWindow.String()).Scan(&abandoned, &recentAll)
		if err != nil {
			return nil, err
		}
		result := map[string]any{
			"requests":          requests,
			"approve_here":      request.Local,
			"recent_abandoned":  abandoned,
			"limit_reached":     recentAll >= pairingGlobalLimit,
			"window_minutes":    int(pairingWindow / time.Minute),
			"pairing_available": service.Identity != nil,
		}
		if request.Local {
			// La computadora principal muestra cómo escribir su dirección en una terminal nueva (la
			// búsqueda automática en la red es de la fase C).
			result["network"] = map[string]any{
				"available": service.NetworkPort > 0,
				"port":      service.NetworkPort,
				"addresses": localAddresses(),
			}
		}
		return result, nil
	}
	if len(parts) != 4 || request.Method != "POST" || !identifierPattern.MatchString(parts[2]) || (parts[3] != "approve" && parts[3] != "reject") {
		return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
	}
	if err := principal.require("devices.manage"); err != nil {
		return nil, err
	}
	if !request.Local {
		return nil, pairingFault(403, "PAIRING_LOCAL_ONLY", "Las terminales nuevas se aprueban en la computadora principal del restaurante.")
	}
	requestID := parts[2]
	if parts[3] == "reject" {
		row, err := lockPairing(ctx, tx, requestID, "", false)
		if err != nil {
			return nil, err
		}
		if row.visibleState != "pending" {
			return nil, pairingStateFault(row.visibleState)
		}
		if _, err := tx.Exec(ctx, `UPDATE app.device_pairing_requests
                SET state='rejected', reject_reason='rejected', decided_at=now(), decided_by=$2 WHERE id=$1`, requestID, principal.UserID); err != nil {
			return nil, err
		}
		return map[string]any{"state": "rejected"}, audit(ctx, tx, principal, "device.pairing_rejected", "device_pairing_request", &requestID, nil,
			map[string]string{"name": row.name, "reason": "rejected"}, true)
	}
	return service.approvePairing(ctx, tx, request, principal, requestID)
}

func (service *Service) approvePairing(ctx context.Context, tx pgx.Tx, request Request, principal Principal, requestID string) (any, error) {
	var input struct {
		Code string  `json:"code"`
		Name *string `json:"name"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	code := strings.TrimSpace(input.Code)
	if len(code) != pairingCodeDigits || strings.Trim(code, "0123456789") != "" {
		return nil, invalid("Escribe los 6 números que muestra la terminal.")
	}
	var approvedName *string
	if input.Name != nil {
		name, valid := cleanLabel(*input.Name, pairingNameMaxLength)
		if !valid {
			return nil, invalid("Revisa el nombre de la terminal (máximo 80 caracteres).")
		}
		approvedName = &name
	}
	row, err := lockPairing(ctx, tx, requestID, "", false)
	if err != nil {
		return nil, err
	}
	if row.visibleState != "pending" {
		return nil, pairingStateFault(row.visibleState)
	}
	if !row.revealedAlready {
		return nil, pairingFault(409, "PAIRING_NOT_READY", "La terminal todavía no termina de pedir su código. Espera unos segundos.")
	}
	// El código se calcula con la huella guardada al crear la solicitud (la del certificado con el que
	// el servidor respondió); si la identidad cambiara en medio, la huella vigente no coincidiría.
	fingerprint, err := hex.DecodeString(row.fingerprint)
	if err != nil || len(fingerprint) != 32 || row.fingerprint != hex.EncodeToString(service.identityFingerprint()) {
		return nil, pairingFault(409, "PAIRING_IDENTITY_CHANGED", "La identidad de este servidor cambió. Pide un código nuevo en la terminal.")
	}
	terminalNonce, validTerminal := decodeNonce(*row.terminalNonce)
	serverNonce, validServer := decodeNonce(row.serverNonce)
	if !validTerminal || !validServer {
		return nil, errors.New("solicitud de emparejamiento dañada")
	}
	var pinned [32]byte
	copy(pinned[:], fingerprint)
	expected, err := PairingCode(pinned, terminalNonce, serverNonce, requestID)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) != 1 {
		return nil, wrongPairingCode(ctx, tx, principal, requestID, row)
	}
	name := row.name
	if approvedName != nil {
		name = *approvedName
	}
	// Aprobada, la terminal tiene otros 5 minutos para recoger su credencial.
	_, err = tx.Exec(ctx, `UPDATE app.device_pairing_requests
        SET state='approved', approved_name=$2, decided_at=now(), decided_by=$3, expires_at=now() + $4::interval WHERE id=$1`,
		requestID, name, principal.UserID, pairingLifetime.String())
	if err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, principal, "device.pairing_approved", "device_pairing_request", &requestID, nil, map[string]string{
		"name": name, "device_kind": row.kind, "os": row.system, "app_version": row.version,
	}, true); err != nil {
		return nil, err
	}
	return map[string]any{"state": "approved", "name": name}, nil
}

// wrongPairingCode cuenta un código incorrecto (se conserva aunque la petición falle) y al quinto rechaza
// la solicitud.
func wrongPairingCode(ctx context.Context, tx pgx.Tx, principal Principal, requestID string, row pairingRow) error {
	attempts := row.failedAttempts + 1
	if _, err := tx.Exec(ctx, "UPDATE app.device_pairing_requests SET failed_attempts=$2 WHERE id=$1", requestID, attempts); err != nil {
		return err
	}
	if err := audit(ctx, tx, principal, "device.pairing_code_failed", "device_pairing_request", &requestID, nil,
		map[string]any{"name": row.name, "attempt": attempts}, false); err != nil {
		return err
	}
	if attempts >= pairingAttempts {
		if _, err := tx.Exec(ctx, `UPDATE app.device_pairing_requests
                SET state='rejected', reject_reason='attempts', decided_at=now(), decided_by=$2 WHERE id=$1`, requestID, principal.UserID); err != nil {
			return err
		}
		if err := audit(ctx, tx, principal, "device.pairing_rejected", "device_pairing_request", &requestID, nil,
			map[string]string{"name": row.name, "reason": "attempts"}, true); err != nil {
			return err
		}
		return &Fault{Status: 409, Code: "PAIRING_ATTEMPTS_EXCEEDED", Message: "Se escribieron 5 códigos incorrectos y la solicitud se rechazó. Pide un código nuevo en la terminal.", Commit: true}
	}
	return &Fault{
		Status:  400,
		Code:    "PAIRING_CODE_INVALID",
		Message: "El código no coincide con el de la terminal. Revísalo e inténtalo de nuevo.",
		Commit:  true,
		Details: map[string]int{"attempts_left": pairingAttempts - attempts},
	}
}

func (service *Service) identityFingerprint() []byte {
	if service.Identity == nil {
		return nil
	}
	return service.Identity.Fingerprint[:]
}

// localAddresses son las direcciones de la computadora principal que una terminal puede escribir: sus IPv4
// privadas (RFC 1918) y, si el sistema lo informa, su nombre «.local».
func localAddresses() []string {
	addresses := []string{}
	interfaces, err := net.InterfaceAddrs()
	if err == nil {
		for _, address := range interfaces {
			network, ok := address.(*net.IPNet)
			if !ok {
				continue
			}
			if v4 := network.IP.To4(); v4 != nil && v4.IsPrivate() {
				addresses = append(addresses, v4.String())
			}
		}
	}
	sort.Strings(addresses)
	if host, err := os.Hostname(); err == nil && strings.HasSuffix(strings.ToLower(host), ".local") {
		addresses = append(addresses, strings.ToLower(host))
	}
	return addresses
}
