package licensing

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Desbloqueo de soporte sin internet (§6.4 de la especificación, L1-D19 y avance §0.5.10).
//
// 1. La pantalla de la hora pide un **código de solicitud** (Manager.SupportRequest): 24 caracteres en base32 de
//    Crockford, en seis grupos de cuatro (XXXX-XXXX-XXXX-XXXX-XXXX-XXXX), fáciles de dictar por teléfono. Son 120 bits:
//
//      bits   campo
//      4      versión (1)
//      40     etiqueta de la instalación: los primeros 5 bytes de SHA-256("ADELPOS-SOPORTE-V1\n" + server_id)
//      40     número aleatorio de un solo uso
//      24     última hora confiable, en minutos desde 2026-01-01 UTC (unos 31 años)
//      12     verificación: los primeros 12 bits de SHA-256 de los 15 bytes con este campo en cero, para detectar un
//             error al dictar
//
//    La solicitud pendiente se guarda en estado.json (una sola: una nueva reemplaza a la anterior).
//
// 2. El propietario de AD-EL POS la procesa con `tools/soporte-desbloqueo responder` y la llave privada de soporte.
//    La **respuesta** es un JWS compacto con el encabezado exacto {"alg":"EdDSA","kid":"<kid>","typ":"adelpos-unlock+jws"}
//    y el contenido SupportResponse. Se pega como texto (por ejemplo, por WhatsApp) o se importa como archivo
//    .desbloqueo.
//
// 3. Se acepta (Manager.AcceptSupportUnlock) **una sola vez**, solo en esta instalación y dentro de las 24 horas de la
//    solicitud, medidas con el reloj local respecto al de la solicitud (L1-D19: si el reloj se corrige en medio, se pide
//    una solicitud nueva). Al aceptarla, la última hora confiable pasa a new_trusted_time (también hacia atrás) y el
//    número queda usado, en una sola escritura.
//
// La respuesta lleva la etiqueta de 40 bits del server_id y no el server_id completo (L1-D19): así el código cabe en 24
// caracteres. Junto con el número de un solo uso, ata la respuesta a esta instalación y a esta solicitud.

const (
	supportCodeVersion = 1
	// supportTagPrefix es el prefijo de la etiqueta de la instalación.
	supportTagPrefix = "ADELPOS-SOPORTE-V1\n"
	// supportAlphabet es el base32 de Crockford: 0-9 y A-Z sin I, L, O ni U.
	supportAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	// SupportValidity es cuánto vale una solicitud: 24 horas.
	SupportValidity = 24 * time.Hour
	// SupportJWSType es el `typ` del encabezado de la respuesta.
	SupportJWSType = "adelpos-unlock+jws"
	// SupportSchemaVersion es la versión del contenido de la respuesta.
	SupportSchemaVersion = "1.0"
	// supportUsedMemory es cuántos números usados se recuerdan para responder «ya se usó».
	supportUsedMemory = 16
	// supportMaxResponse limita el texto de una respuesta pegada o importada.
	supportMaxResponse = 4096
)

// supportEpoch es el origen de los minutos de la última hora confiable en el código.
var supportEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Motivos de un desbloqueo rechazado (details.reason de SUPPORT_UNLOCK_INVALID, avance §0.6.2).
const (
	SupportReasonUsed              = "used"
	SupportReasonOtherInstallation = "other_installation"
	SupportReasonExpired           = "expired"
	SupportReasonAltered           = "altered"
	SupportReasonNotRequested      = "not_requested"
)

// SupportUnlockError es un desbloqueo rechazado. No cambia nada.
type SupportUnlockError struct {
	Reason string
}

func (problem *SupportUnlockError) Error() string {
	return "desbloqueo de soporte rechazado: " + problem.Reason
}

func supportRejected(reason string) error {
	return &SupportUnlockError{Reason: reason}
}

// SupportCode es un código de solicitud ya leído.
type SupportCode struct {
	// Text es el código normalizado, con sus guiones.
	Text string
	// InstallationTag y Nonce van en hexadecimal (10 caracteres cada uno).
	InstallationTag string
	Nonce           string
	// TrustedTime es la última hora confiable al pedirlo, al minuto (cero si no había).
	TrustedTime time.Time
}

// SupportInstallationTag es la etiqueta de 40 bits de una instalación, en hexadecimal.
func SupportInstallationTag(serverID string) string {
	digest := sha256.Sum256([]byte(supportTagPrefix + serverID))
	return hex.EncodeToString(digest[:5])
}

// EncodeSupportCode arma el código de solicitud.
func EncodeSupportCode(installationTag, nonce string, trusted time.Time) (string, error) {
	tag, errTag := hex.DecodeString(installationTag)
	random, errNonce := hex.DecodeString(nonce)
	if errTag != nil || errNonce != nil || len(tag) != 5 || len(random) != 5 {
		return "", errors.New("etiqueta o número de la solicitud inválidos")
	}
	var buffer [15]byte
	putBits(buffer[:], 0, 4, supportCodeVersion)
	putBits(buffer[:], 4, 40, bytesValue(tag))
	putBits(buffer[:], 44, 40, bytesValue(random))
	putBits(buffer[:], 84, 24, supportMinutes(trusted))
	putBits(buffer[:], 108, 12, supportChecksum(buffer))
	return formatSupportCode(buffer), nil
}

// ParseSupportCode lee un código dictado: acepta minúsculas, espacios y guiones, O como 0 e I o L como 1. Comprueba la
// versión y la verificación.
func ParseSupportCode(text string) (SupportCode, error) {
	symbols := make([]byte, 0, 24)
	for _, char := range strings.ToUpper(text) {
		switch {
		case char == '-' || char == ' ' || char == '\t' || char == '\n' || char == '\r':
			continue
		case char == 'O':
			char = '0'
		case char == 'I' || char == 'L':
			char = '1'
		}
		index := strings.IndexRune(supportAlphabet, char)
		if index < 0 {
			return SupportCode{}, errors.New("el código de solicitud tiene un carácter que no es válido")
		}
		symbols = append(symbols, byte(index))
	}
	if len(symbols) != 24 {
		return SupportCode{}, errors.New("el código de solicitud debe tener 24 caracteres")
	}
	var buffer [15]byte
	for position, symbol := range symbols {
		putBits(buffer[:], position*5, 5, uint64(symbol))
	}
	if getBits(buffer[:], 0, 4) != supportCodeVersion {
		return SupportCode{}, errors.New("el código de solicitud es de otra versión")
	}
	if getBits(buffer[:], 108, 12) != supportChecksum(buffer) {
		return SupportCode{}, errors.New("el código de solicitud no es correcto; revisa que se haya copiado completo")
	}
	code := SupportCode{
		Text:            formatSupportCode(buffer),
		InstallationTag: hex.EncodeToString(valueBytes(getBits(buffer[:], 4, 40), 5)),
		Nonce:           hex.EncodeToString(valueBytes(getBits(buffer[:], 44, 40), 5)),
	}
	if minutes := getBits(buffer[:], 84, 24); minutes > 0 {
		code.TrustedTime = supportEpoch.Add(time.Duration(minutes) * time.Minute)
	}
	return code, nil
}

// supportMinutes son los minutos desde supportEpoch, en 24 bits (0 si no hay hora o es anterior).
func supportMinutes(trusted time.Time) uint64 {
	if trusted.IsZero() || !trusted.After(supportEpoch) {
		return 0
	}
	minutes := uint64(trusted.Sub(supportEpoch) / time.Minute)
	if minutes >= 1<<24 {
		return 1<<24 - 1
	}
	return minutes
}

// supportChecksum son los primeros 12 bits de SHA-256 del código con su campo de verificación en cero.
func supportChecksum(buffer [15]byte) uint64 {
	putBits(buffer[:], 108, 12, 0)
	digest := sha256.Sum256(buffer[:])
	return uint64(digest[0])<<4 | uint64(digest[1]>>4)
}

func formatSupportCode(buffer [15]byte) string {
	var builder strings.Builder
	for position := range 24 {
		if position > 0 && position%4 == 0 {
			builder.WriteByte('-')
		}
		builder.WriteByte(supportAlphabet[getBits(buffer[:], position*5, 5)])
	}
	return builder.String()
}

// putBits escribe value en width bits a partir del bit offset (el más significativo primero).
func putBits(buffer []byte, offset, width int, value uint64) {
	for bit := range width {
		position := offset + bit
		mask := byte(1) << (7 - position%8)
		if value>>(width-1-bit)&1 == 1 {
			buffer[position/8] |= mask
		} else {
			buffer[position/8] &^= mask
		}
	}
}

// getBits lee width bits a partir del bit offset.
func getBits(buffer []byte, offset, width int) uint64 {
	var value uint64
	for bit := range width {
		position := offset + bit
		value = value<<1 | uint64(buffer[position/8]>>(7-position%8)&1)
	}
	return value
}

func bytesValue(raw []byte) uint64 {
	var value uint64
	for _, item := range raw {
		value = value<<8 | uint64(item)
	}
	return value
}

func valueBytes(value uint64, size int) []byte {
	raw := make([]byte, size)
	for index := size - 1; index >= 0; index-- {
		raw[index] = byte(value)
		value >>= 8
	}
	return raw
}

// SupportResponse es el contenido de la respuesta firmada.
type SupportResponse struct {
	SchemaVersion   string `json:"schema_version"`
	ProductID       string `json:"product_id"`
	Environment     string `json:"environment"`
	InstallationTag string `json:"installation_tag"`
	RequestNonce    string `json:"request_nonce"`
	RequestCode     string `json:"request_code"`
	NewTrustedTime  string `json:"new_trusted_time"`
	IssuedAt        string `json:"issued_at"`
}

// SignSupportResponse firma la respuesta a un código con la llave privada de soporte (lo usa soporte-desbloqueo;
// las pruebas, con llaves propias). environment es el entorno de compilación de la versión que la recibirá.
func SignSupportResponse(private ed25519.PrivateKey, kid, environment string, code SupportCode, newTrustedTime,
	issuedAt time.Time) (string, error) {
	if len(private) != ed25519.PrivateKeySize || strings.TrimSpace(kid) == "" || newTrustedTime.IsZero() {
		return "", errors.New("faltan datos para firmar la respuesta")
	}
	content, err := json.Marshal(SupportResponse{
		SchemaVersion:   SupportSchemaVersion,
		ProductID:       ProductID,
		Environment:     environment,
		InstallationTag: code.InstallationTag,
		RequestNonce:    code.Nonce,
		RequestCode:     code.Text,
		NewTrustedTime:  newTrustedTime.UTC().Format(time.RFC3339),
		IssuedAt:        issuedAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return "", err
	}
	kidJSON, _ := json.Marshal(kid)
	header := encodeBase64URL([]byte(`{"alg":"EdDSA","kid":` + string(kidJSON) + `,"typ":"` + SupportJWSType + `"}`))
	body := encodeBase64URL(content)
	return header + "." + body + "." + encodeBase64URL(ed25519.Sign(private, []byte(header+"."+body))), nil
}

// verifiedSupportResponse es una respuesta con la firma y el formato comprobados.
type verifiedSupportResponse struct {
	content        SupportResponse
	newTrustedTime time.Time
}

// VerifySupportResponse comprueba la firma y el formato de una respuesta (sin el estado de la solicitud). El texto
// pegado puede traer espacios y saltos de línea: se quitan. Cualquier problema es «altered».
func (trust SupportTrust) VerifySupportResponse(text string) (SupportResponse, time.Time, error) {
	verified, err := trust.verify(text)
	if err != nil {
		return SupportResponse{}, time.Time{}, err
	}
	return verified.content, verified.newTrustedTime, nil
}

func (trust SupportTrust) verify(text string) (verifiedSupportResponse, error) {
	if len(text) > supportMaxResponse {
		return verifiedSupportResponse{}, supportRejected(SupportReasonAltered)
	}
	compact := strings.Join(strings.Fields(text), "")
	segments := strings.Split(compact, ".")
	if len(segments) != 3 {
		return verifiedSupportResponse{}, supportRejected(SupportReasonAltered)
	}
	headerJSON, okHeader := decodeBase64URL(segments[0])
	contentJSON, okContent := decodeBase64URL(segments[1])
	signature, okSignature := decodeBase64URL(segments[2])
	if !okHeader || !okContent || !okSignature || len(signature) != ed25519.SignatureSize {
		return verifiedSupportResponse{}, supportRejected(SupportReasonAltered)
	}
	var header struct {
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil || header.Kid == "" {
		return verifiedSupportResponse{}, supportRejected(SupportReasonAltered)
	}
	kidJSON, _ := json.Marshal(header.Kid)
	if string(headerJSON) != `{"alg":"EdDSA","kid":`+string(kidJSON)+`,"typ":"`+SupportJWSType+`"}` {
		return verifiedSupportResponse{}, supportRejected(SupportReasonAltered)
	}
	public, trusted := trust.Keys[header.Kid]
	if !trusted || !ed25519.Verify(public, []byte(segments[0]+"."+segments[1]), signature) {
		return verifiedSupportResponse{}, supportRejected(SupportReasonAltered)
	}
	var content SupportResponse
	decoder := json.NewDecoder(strings.NewReader(string(contentJSON)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&content); err != nil {
		return verifiedSupportResponse{}, supportRejected(SupportReasonAltered)
	}
	newTrustedTime, errTrusted := time.Parse(time.RFC3339, content.NewTrustedTime)
	_, errIssued := time.Parse(time.RFC3339, content.IssuedAt)
	if content.SchemaVersion != SupportSchemaVersion || content.ProductID != ProductID ||
		content.Environment != trust.Environment || errTrusted != nil || errIssued != nil ||
		!newTrustedTime.After(supportEpoch) || len(content.InstallationTag) != 10 || len(content.RequestNonce) != 10 {
		return verifiedSupportResponse{}, supportRejected(SupportReasonAltered)
	}
	return verifiedSupportResponse{content: content, newTrustedTime: newTrustedTime.UTC()}, nil
}

// supportState es lo que estado.json guarda del desbloqueo de soporte.
type supportState struct {
	// Pending es la solicitud pendiente (una sola).
	Pending *pendingSupportRequest `json:"pending,omitempty"`
	// Used son los últimos números ya usados, para decir «ya se usó» en lugar de «no se pidió».
	Used []string `json:"used_nonces,omitempty"`
}

// pendingSupportRequest es una solicitud de desbloqueo todavía sin usar.
type pendingSupportRequest struct {
	Code  string `json:"code"`
	Nonce string `json:"nonce"`
	// CreatedAt es el reloj local al pedirla: las 24 horas se miden con el mismo reloj (L1-D19).
	CreatedAt   time.Time  `json:"created_at"`
	TrustedTime *time.Time `json:"last_trusted_time,omitempty"`
}

func (support *supportState) clone() *supportState {
	if support == nil {
		return nil
	}
	copied := &supportState{Used: append([]string(nil), support.Used...)}
	if support.Pending != nil {
		pending := *support.Pending
		if support.Pending.TrustedTime != nil {
			instant := *support.Pending.TrustedTime
			pending.TrustedTime = &instant
		}
		copied.Pending = &pending
	}
	return copied
}

// SupportRequestInfo es la solicitud que muestra la pantalla de la hora.
type SupportRequestInfo struct {
	Code string
	// CreatedAt y ExpiresAt son del reloj local de la computadora principal.
	CreatedAt time.Time
	ExpiresAt time.Time
	// TrustedTime es la última hora confiable al pedirla (cero si no había).
	TrustedTime time.Time
	// Reused indica que se entregó la solicitud pendiente, sin crear otra.
	Reused bool
}

func (pending *pendingSupportRequest) info(reused bool) SupportRequestInfo {
	info := SupportRequestInfo{
		Code:      pending.Code,
		CreatedAt: pending.CreatedAt,
		ExpiresAt: pending.CreatedAt.Add(SupportValidity),
		Reused:    reused,
	}
	if pending.TrustedTime != nil {
		info.TrustedTime = *pending.TrustedTime
	}
	return info
}

// supportWindowOpen dice si una solicitud creada con el reloj local en created sigue vigente en now: menos de 24
// horas después y nunca antes de crearla (salvo el margen de los ajustes normales del reloj). Si el reloj se corrigió
// en medio, se pide una solicitud nueva.
func supportWindowOpen(created, now time.Time) bool {
	elapsed := now.Sub(created)
	return elapsed >= -ClockMargin && elapsed < SupportValidity
}

// SupportRequest entrega la solicitud de desbloqueo pendiente si sigue vigente o, si no la hay o renew lo pide, crea
// otra (que reemplaza a la anterior) y la guarda antes de entregarla.
func (manager *Manager) SupportRequest(renew bool) (SupportRequestInfo, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if err := manager.supportInstallationProblemLocked(); err != nil {
		return SupportRequestInfo{}, err
	}
	installation := manager.options.Installation
	now := manager.now()
	if pending := manager.state.Support.pending(); pending != nil && !renew && supportWindowOpen(pending.CreatedAt, now) {
		return pending.info(true), nil
	}
	var random [5]byte
	if _, err := rand.Read(random[:]); err != nil {
		return SupportRequestInfo{}, errors.New("no se pudo crear la solicitud de desbloqueo")
	}
	trusted := time.Time{}
	if manager.state.TrustedTime != nil {
		trusted = *manager.state.TrustedTime
	}
	nonce := hex.EncodeToString(random[:])
	code, err := EncodeSupportCode(SupportInstallationTag(installation.ServerID), nonce, trusted)
	if err != nil {
		return SupportRequestInfo{}, err
	}
	pending := &pendingSupportRequest{Code: code, Nonce: nonce, CreatedAt: now.UTC()}
	if !trusted.IsZero() {
		instant := trusted.UTC()
		pending.TrustedTime = &instant
	}
	next := manager.state.clone()
	if next.Support == nil {
		next.Support = &supportState{}
	}
	next.Support.Pending = pending
	if err := manager.store.write(next, now); err != nil {
		manager.logger.Error("license_state_not_saved", "reason", err.Error())
		return SupportRequestInfo{}, ErrStateNotSaved
	}
	manager.state = next
	manager.dirty = false
	manager.logger.Info("license_support_requested", "code", code)
	return pending.info(false), nil
}

// supportInstallationProblemLocked dice por qué esta instalación no puede pedir ni aceptar un desbloqueo de soporte, o
// nil. La §6.4 es «solo en esa instalación»: como Accept, se niega sin identidad, en una carpeta de datos copiada de
// otra computadora (que tendría la misma identidad y la misma solicitud pendiente) y sin la huella del equipo comprobada
// (revisión R2, hallazgo R2-5). Como toda escritura del estado, sin el estado del disco tampoco (R1-1).
func (manager *Manager) supportInstallationProblemLocked() error {
	switch {
	case manager.options.Installation == nil:
		return ErrIdentityUnavailable
	case manager.copied:
		return ErrCopiedInstallation
	case manager.machineUnverified:
		return ErrMachineUnverified
	}
	return manager.ensureStateLocked()
}

func (support *supportState) pending() *pendingSupportRequest {
	if support == nil {
		return nil
	}
	return support.Pending
}

func (support *supportState) used(nonce string) bool {
	if support == nil {
		return false
	}
	for _, item := range support.Used {
		if item == nonce {
			return true
		}
	}
	return false
}

// SupportUnlockResult es un desbloqueo de soporte aceptado.
type SupportUnlockResult struct {
	RequestCode string
	// PreviousTrustedTime es la última hora confiable que se reemplazó (cero si no había) y NewTrustedTime la de la
	// respuesta.
	PreviousTrustedTime time.Time
	NewTrustedTime      time.Time
}

// CheckSupportUnlock comprueba una respuesta de soporte igual que AcceptSupportUnlock, sin cambiar nada: la ruta la
// audita antes de aplicarla, para que el desbloqueo y su auditoría pasen juntos (revisión R2, hallazgo R2-1).
func (manager *Manager) CheckSupportUnlock(response string, trust SupportTrust) (SupportUnlockResult, error) {
	verified, err := trust.verify(response)
	if err != nil {
		return SupportUnlockResult{}, err
	}
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	result, _, err := manager.prepareSupportUnlockLocked(verified)
	return result, err
}

// AcceptSupportUnlock comprueba una respuesta de soporte contra la solicitud pendiente y, si vale, reemplaza la última
// hora confiable (también hacia atrás) en sus dos copias y marca el número como usado en la misma escritura de
// estado.json (R2-3). Un rechazo es un *SupportUnlockError con su motivo y no cambia nada; si no se puede guardar,
// ErrStateNotSaved, tampoco cambia nada y el número sigue sin usar.
func (manager *Manager) AcceptSupportUnlock(response string, trust SupportTrust) (SupportUnlockResult, error) {
	verified, err := trust.verify(response)
	if err != nil {
		return SupportUnlockResult{}, err
	}
	manager.timeMutex.Lock()
	defer manager.timeMutex.Unlock()
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	result, next, err := manager.prepareSupportUnlockLocked(verified)
	if err != nil {
		return SupportUnlockResult{}, err
	}
	if err := manager.replaceTrustedTimeLocked(next); err != nil {
		return SupportUnlockResult{}, err
	}
	manager.logger.Info("license_trusted_time_replaced", "reason", string(ReplaceBySupportUnlock),
		"previous", result.PreviousTrustedTime, "replaced", result.NewTrustedTime)
	return result, nil
}

// prepareSupportUnlockLocked comprueba una respuesta ya verificada contra esta instalación y su solicitud pendiente, y
// prepara el estado que la aplica (sin guardarlo).
func (manager *Manager) prepareSupportUnlockLocked(verified verifiedSupportResponse) (SupportUnlockResult, storedState, error) {
	if err := manager.supportInstallationProblemLocked(); err != nil {
		return SupportUnlockResult{}, storedState{}, err
	}
	installation := manager.options.Installation
	content := verified.content
	if content.InstallationTag != SupportInstallationTag(installation.ServerID) {
		return SupportUnlockResult{}, storedState{}, supportRejected(SupportReasonOtherInstallation)
	}
	support := manager.state.Support
	if support.used(content.RequestNonce) {
		return SupportUnlockResult{}, storedState{}, supportRejected(SupportReasonUsed)
	}
	pending := support.pending()
	if pending == nil || pending.Nonce != content.RequestNonce {
		return SupportUnlockResult{}, storedState{}, supportRejected(SupportReasonNotRequested)
	}
	if pending.Code != content.RequestCode {
		return SupportUnlockResult{}, storedState{}, supportRejected(SupportReasonAltered)
	}
	if !supportWindowOpen(pending.CreatedAt, manager.now()) {
		return SupportUnlockResult{}, storedState{}, supportRejected(SupportReasonExpired)
	}

	result := SupportUnlockResult{RequestCode: pending.Code, NewTrustedTime: verified.newTrustedTime}
	if manager.state.TrustedTime != nil {
		result.PreviousTrustedTime = *manager.state.TrustedTime
	}
	next := manager.state.clone()
	replaced := verified.newTrustedTime
	next.TrustedTime = &replaced
	next.TrustedSource = string(ReplaceBySupportUnlock)
	next.Support.Pending = nil
	next.Support.Used = append(next.Support.Used, content.RequestNonce)
	if len(next.Support.Used) > supportUsedMemory {
		next.Support.Used = next.Support.Used[len(next.Support.Used)-supportUsedMemory:]
	}
	return result, next, nil
}
