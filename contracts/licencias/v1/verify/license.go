package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Valor de «typ» en el encabezado de la licencia firmada (contrato, sección 3).
const jwsType = "adelpos-lic+jws"

// Producto de la licencia. Es fijo (contrato, sección 2).
const productID = "adel_pos"

// Motivos por los que se rechaza una licencia. El texto es estable: los archivos de prueba
// lo usan como resultado esperado.
const (
	ErrMalformed          = "malformed_jws"
	ErrInvalidHeader      = "invalid_header"
	ErrUnknownKid         = "unknown_kid"
	ErrInvalidSignature   = "invalid_signature"
	ErrIncompatibleSchema = "incompatible_schema"
	ErrInvalidContent     = "invalid_content"
	ErrOtherProduct       = "other_product"
	ErrEnvironment        = "environment_mismatch"
	ErrOtherInstallation  = "other_installation"
	ErrStaleRevision      = "stale_revision"
)

// VerifyError dice por qué se rechazó una licencia.
type VerifyError struct {
	Code   string
	Detail string
}

func (e *VerifyError) Error() string {
	if e.Detail == "" {
		return e.Code
	}
	return e.Code + ": " + e.Detail
}

func reject(code, detail string) error {
	return &VerifyError{Code: code, Detail: detail}
}

// TrustedKey es una llave pública de firma en la que confía esta versión de AD-EL POS.
type TrustedKey struct {
	Environment string
	PublicKey   ed25519.PublicKey
}

// Installation es la identidad de la computadora que verifica: su «server_id» y su llave
// pública de licencias, en base64url.
type Installation struct {
	ServerID  string
	PublicKey string
}

// Accepted es la revisión más alta que la instalación ya aceptó para una licencia.
type Accepted struct {
	LicenseID string
	Revision  int64
	SHA256    string
}

// Verifier reúne lo que hace falta para verificar una licencia sin internet.
//
// Keys contiene solo las llaves del entorno de esta versión: una versión de producción no
// incluye ninguna llave de pruebas, y al revés.
type Verifier struct {
	Environment  string
	Keys         map[string]TrustedKey
	Installation Installation
}

// Plan es informativo. Los derechos los marca Limits.
type Plan struct {
	PlanID string
	Name   string
	Custom bool
}

// Limits son los cupos de sesiones simultáneas.
type Limits struct {
	CashRegisters   int64
	OrderTakers     int64
	KitchenDisplays int64
}

// License es el contenido de una licencia ya verificada (contrato, sección 4).
type License struct {
	SchemaVersion         string
	ProductID             string
	Environment           string
	LicenseID             string
	ActivationID          string
	ServerID              string
	InstallationPublicKey string
	LicenseeName          string
	LicenseType           string
	BillingPeriod         *string
	LicenseStatus         string
	StatusReason          *string
	LicenseRevision       int64
	IssuedAt              time.Time
	PeriodStartsAt        time.Time
	ExpiresAt             *time.Time
	GraceDays             int64
	UpdatesUntil          time.Time
	Plan                  Plan
	Limits                Limits
	Features              map[string]bool

	// SHA256 del JWS completo, en hexadecimal: es lo que se guarda como revisión aceptada.
	SHA256 string
}

var (
	uuidPattern    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	uuidV4Pattern  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	keyPattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
	instantPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?Z$`)
	controlPattern = regexp.MustCompile(`[\x00-\x1f\x7f]`)
)

// decodeBase64URL acepta solo base64url sin relleno y en su forma canónica.
func decodeBase64URL(text string) ([]byte, bool) {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != text {
		return nil, false
	}
	return raw, true
}

// Verify aplica, en orden, las comprobaciones de la sección 10 del contrato. Devuelve la
// licencia solo si todas pasan; quien llama la guarda junto con su revisión aceptada.
//
// accepted es la revisión aceptada antes para esa misma licencia, o nil si no hay ninguna.
// Si pertenece a otra licencia no se toma en cuenta.
func (v Verifier) Verify(jws string, accepted *Accepted) (*License, error) {
	// 1. Tres segmentos en base64url.
	segments := strings.Split(jws, ".")
	if len(segments) != 3 {
		return nil, reject(ErrMalformed, "se esperaban tres segmentos")
	}
	headerJSON, okHeader := decodeBase64URL(segments[0])
	contentJSON, okContent := decodeBase64URL(segments[1])
	signature, okSignature := decodeBase64URL(segments[2])
	if !okHeader || !okContent || !okSignature || len(signature) != ed25519.SignatureSize {
		return nil, reject(ErrMalformed, "base64url inválido")
	}

	// 2. Encabezado exacto: los mismos tres campos, en el mismo orden y nada más.
	var header struct {
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil || header.Kid == "" {
		return nil, reject(ErrInvalidHeader, "no se pudo leer el encabezado")
	}
	kidJSON, _ := json.Marshal(header.Kid)
	exact := `{"alg":"EdDSA","kid":` + string(kidJSON) + `,"typ":"` + jwsType + `"}`
	if string(headerJSON) != exact {
		return nil, reject(ErrInvalidHeader, "el encabezado no es el del contrato")
	}

	// 3. La llave debe ser de confianza para el entorno de esta versión.
	key, known := v.Keys[header.Kid]
	if !known || key.Environment != v.Environment {
		return nil, reject(ErrUnknownKid, header.Kid)
	}

	// 4. Firma Ed25519 sobre «encabezado.contenido», tal como llegaron.
	if !ed25519.Verify(key.PublicKey, []byte(segments[0]+"."+segments[1]), signature) {
		return nil, reject(ErrInvalidSignature, "")
	}

	// 5. Contenido de la sección 4.
	license, err := parseContent(contentJSON)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(jws))
	license.SHA256 = hex.EncodeToString(sum[:])

	// 6. El entorno del contenido es el de la llave que firmó.
	if license.Environment != key.Environment {
		return nil, reject(ErrEnvironment, license.Environment)
	}

	// 7. La licencia es de esta computadora: misma identidad y misma llave.
	if license.ServerID != v.Installation.ServerID || license.InstallationPublicKey != v.Installation.PublicKey {
		return nil, reject(ErrOtherInstallation, "")
	}

	// 8. La revisión nunca retrocede. Una revisión igual solo vale si es idéntica byte por byte.
	if accepted != nil && accepted.LicenseID == license.LicenseID {
		if license.LicenseRevision < accepted.Revision {
			return nil, reject(ErrStaleRevision, "revisión menor que la aceptada")
		}
		if license.LicenseRevision == accepted.Revision && license.SHA256 != accepted.SHA256 {
			return nil, reject(ErrStaleRevision, "misma revisión con otro contenido")
		}
	}
	return license, nil
}

// parseContent comprueba que el contenido cumpla la sección 4: todos los campos obligatorios,
// con su tipo y su formato. Los campos adicionales desconocidos se ignoran.
func parseContent(contentJSON []byte) (*License, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(contentJSON))
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, reject(ErrInvalidContent, "el contenido no es un objeto JSON")
	}
	reader := contentReader{fields: fields}
	license := &License{}

	// Una versión que no empieza con «1.» es incompatible, antes de mirar lo demás.
	license.SchemaVersion = reader.text("schema_version")
	if reader.err == nil && !strings.HasPrefix(license.SchemaVersion, "1.") {
		return nil, reject(ErrIncompatibleSchema, license.SchemaVersion)
	}
	license.ProductID = reader.text("product_id")
	if reader.err == nil && license.ProductID != productID {
		return nil, reject(ErrOtherProduct, license.ProductID)
	}

	license.Environment = reader.oneOf("environment", "test", "production")
	license.LicenseID = reader.matching("license_id", uuidPattern)
	license.ActivationID = reader.matching("activation_id", uuidPattern)
	license.ServerID = reader.matching("server_id", uuidV4Pattern)
	license.InstallationPublicKey = reader.publicKey("installation_public_key")
	license.LicenseeName = reader.name("licensee_name")
	license.LicenseType = reader.oneOf("license_type", "perpetual", "subscription", "trial")
	license.BillingPeriod = reader.optionalOneOf("billing_period", "monthly", "quarterly", "semiannual", "annual")
	license.LicenseStatus = reader.oneOf("license_status", "active", "revoked")
	license.StatusReason = reader.optionalOneOf("status_reason", "transferred", "deactivated", "revoked")
	license.LicenseRevision = reader.integer("license_revision", 1, 1<<53)
	license.IssuedAt = reader.instant("issued_at")
	license.PeriodStartsAt = reader.instant("period_starts_at")
	license.ExpiresAt = reader.optionalInstant("expires_at")
	license.GraceDays = reader.integer("grace_days", 0, 5)
	license.UpdatesUntil = reader.instant("updates_until")
	license.Plan = reader.plan("plan")
	license.Limits = reader.limits("limits")
	license.Features = reader.features("features")
	if reader.err != nil {
		return nil, reader.err
	}

	// Reglas que dependen del tipo y del estado.
	switch license.LicenseType {
	case "perpetual":
		if license.BillingPeriod != nil || license.ExpiresAt != nil || license.GraceDays != 0 {
			return nil, reject(ErrInvalidContent, "una perpetua no vence ni tiene periodo de cobro ni tolerancia")
		}
	case "subscription":
		if license.BillingPeriod == nil || license.ExpiresAt == nil || license.GraceDays != 5 {
			return nil, reject(ErrInvalidContent, "una suscripción lleva periodo, vencimiento y 5 días de tolerancia")
		}
	case "trial":
		if license.BillingPeriod != nil || license.ExpiresAt == nil || license.GraceDays != 0 {
			return nil, reject(ErrInvalidContent, "una prueba vence, sin periodo de cobro y sin tolerancia")
		}
	}
	if (license.LicenseStatus == "active") != (license.StatusReason == nil) {
		return nil, reject(ErrInvalidContent, "el motivo va solo en las licencias revocadas")
	}
	return license, nil
}

// contentReader lee los campos uno por uno y conserva el primer error.
type contentReader struct {
	fields map[string]json.RawMessage
	err    error
}

func (r *contentReader) fail(field string) {
	if r.err == nil {
		r.err = reject(ErrInvalidContent, field)
	}
}

// raw devuelve el valor de un campo obligatorio.
func (r *contentReader) raw(field string) (json.RawMessage, bool) {
	value, present := r.fields[field]
	if !present {
		r.fail(field)
		return nil, false
	}
	return value, true
}

func isNull(value json.RawMessage) bool {
	return string(bytes.TrimSpace(value)) == "null"
}

func (r *contentReader) text(field string) string {
	value, present := r.raw(field)
	if !present {
		return ""
	}
	var text string
	if isNull(value) || json.Unmarshal(value, &text) != nil {
		r.fail(field)
		return ""
	}
	return text
}

func (r *contentReader) oneOf(field string, allowed ...string) string {
	text := r.text(field)
	for _, candidate := range allowed {
		if text == candidate {
			return text
		}
	}
	r.fail(field)
	return ""
}

// optionalOneOf admite null además de los valores indicados.
func (r *contentReader) optionalOneOf(field string, allowed ...string) *string {
	value, present := r.raw(field)
	if !present || isNull(value) {
		return nil
	}
	text := r.oneOf(field, allowed...)
	if text == "" {
		return nil
	}
	return &text
}

func (r *contentReader) matching(field string, pattern *regexp.Regexp) string {
	text := r.text(field)
	if !pattern.MatchString(text) {
		r.fail(field)
		return ""
	}
	return text
}

func (r *contentReader) publicKey(field string) string {
	text := r.text(field)
	raw, canonical := decodeBase64URL(text)
	if !canonical || len(raw) != ed25519.PublicKeySize {
		r.fail(field)
		return ""
	}
	return text
}

// name es un texto de 1 a 190 caracteres, sin caracteres de control y que no esté en blanco.
func (r *contentReader) name(field string) string {
	text := r.text(field)
	if strings.TrimSpace(text) == "" || len([]rune(text)) > 190 || controlPattern.MatchString(text) {
		r.fail(field)
		return ""
	}
	return text
}

// integer exige un número JSON entero dentro del rango. Un texto como "3" no vale.
func (r *contentReader) integer(field string, minimum, maximum int64) int64 {
	value, present := r.raw(field)
	if !present {
		return 0
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if isNull(value) || bytes.HasPrefix(bytes.TrimSpace(value), []byte(`"`)) || decoder.Decode(&number) != nil {
		r.fail(field)
		return 0
	}
	integer, err := number.Int64()
	if err != nil || integer < minimum || integer > maximum {
		r.fail(field)
		return 0
	}
	return integer
}

// instant acepta solo UTC terminado en «Z», con 0 a 9 decimales. Nunca una hora con desfase.
func (r *contentReader) instant(field string) time.Time {
	text := r.text(field)
	if !instantPattern.MatchString(text) {
		r.fail(field)
		return time.Time{}
	}
	instant, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		r.fail(field)
		return time.Time{}
	}
	return instant.UTC()
}

func (r *contentReader) optionalInstant(field string) *time.Time {
	value, present := r.raw(field)
	if !present || isNull(value) {
		return nil
	}
	instant := r.instant(field)
	return &instant
}

// object devuelve un lector de los campos de un objeto anidado.
func (r *contentReader) object(field string) (*contentReader, bool) {
	value, present := r.raw(field)
	if !present {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if isNull(value) || json.Unmarshal(value, &fields) != nil || fields == nil {
		r.fail(field)
		return nil, false
	}
	return &contentReader{fields: fields}, true
}

func (r *contentReader) plan(field string) Plan {
	inner, ok := r.object(field)
	if !ok {
		return Plan{}
	}
	plan := Plan{PlanID: inner.matching("plan_id", keyPattern), Name: inner.name("name")}
	custom, present := inner.raw("custom")
	if !present || json.Unmarshal(custom, &plan.Custom) != nil || isNull(custom) {
		inner.fail("custom")
	}
	if inner.err != nil {
		r.fail(field)
	}
	return plan
}

// limits: enteros de 0 a 999. En V1 no existe «sin límite».
func (r *contentReader) limits(field string) Limits {
	inner, ok := r.object(field)
	if !ok {
		return Limits{}
	}
	limits := Limits{
		CashRegisters:   inner.integer("cash_registers", 0, 999),
		OrderTakers:     inner.integer("order_takers", 0, 999),
		KitchenDisplays: inner.integer("kitchen_displays", 0, 999),
	}
	if inner.err != nil {
		r.fail(field)
	}
	return limits
}

// features: objeto «clave → true». El cliente conserva las claves, conozca o no el módulo;
// quien las usa ignora las que no conoce, sin habilitarlas para nada.
func (r *contentReader) features(field string) map[string]bool {
	inner, ok := r.object(field)
	if !ok {
		return nil
	}
	features := make(map[string]bool, len(inner.fields))
	for key, value := range inner.fields {
		if !keyPattern.MatchString(key) || string(bytes.TrimSpace(value)) != "true" {
			r.fail(field)
			return nil
		}
		features[key] = true
	}
	return features
}

// describe resume una licencia en una línea, para los mensajes del programa.
func (l *License) describe() string {
	return fmt.Sprintf("%s r%d %s", l.LicenseID, l.LicenseRevision, l.LicenseStatus)
}
