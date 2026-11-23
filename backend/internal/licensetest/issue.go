//go:build !adelpos_production

package licensetest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"local/ad-el-pos/server/internal/licensing"
)

// Emisor de licencias de prueba: un .lic firmado con una llave de vectors.json para una instalación concreta (su
// server_id y su llave pública), con el contenido exacto de la §4. Sirve para poner el Go temporal de las pruebas en
// cualquier estado importándolo por license/import: vigente, tolerancia, vencida, revocada o con 0 comanderos.

// Tipos de licencia (§4).
const (
	TypePerpetual    = "perpetual"
	TypeSubscription = "subscription"
	TypeTrial        = "trial"
)

// Planes iniciales del catálogo del servidor de licencias (§11.1), con sus cupos.
var plans = map[string]struct {
	name   string
	limits licensing.Limits
}{
	"basico":    {"Plan básico", licensing.Limits{CashRegisters: 1}},
	"elemental": {"Plan elemental", licensing.Limits{CashRegisters: 3, OrderTakers: 3, KitchenDisplays: 3}},
	"premium":   {"Plan premium", licensing.Limits{CashRegisters: 5, OrderTakers: 10, KitchenDisplays: 5}},
}

// IssueOptions es el contenido de la licencia. Lo que queda en cero toma el valor por omisión.
type IssueOptions struct {
	// Kid es la llave de firma de prueba (por omisión, adelpos-test-fixture-a).
	Kid string
	// ServerID y PublicKey son los de la instalación destinataria.
	ServerID  string
	PublicKey string
	// Type: perpetual (por omisión), subscription o trial.
	Type string
	// BillingPeriod de una suscripción (por omisión, monthly).
	BillingPeriod string
	// IssuedAt (por omisión, ahora) y PeriodStartsAt (por omisión, IssuedAt).
	IssuedAt       time.Time
	PeriodStartsAt time.Time
	// ExpiresAt: por omisión, IssuedAt + 1 mes en suscripción y + 15 días en prueba. Una perpetua no vence.
	ExpiresAt time.Time
	// GraceDays: el contrato exige 5 en suscripción y 0 en prueba y perpetua (§4); nil toma ese valor y otro hace
	// que Issue falle con el motivo de la verificación.
	GraceDays *int
	// UpdatesUntil: por omisión, ExpiresAt (suscripción y prueba) o IssuedAt + 6 meses (perpetua).
	UpdatesUntil time.Time
	// PlanID (basico, elemental o premium; por omisión elemental) y Limits: sin Limits, los del plan; con otros,
	// la licencia queda personalizada (custom).
	PlanID string
	Limits *licensing.Limits
	// Status: active (por omisión) o revoked, con Reason (deactivated, transferred o revoked; por omisión revoked).
	Status string
	Reason string
	// Revision (por omisión 1), LicenseID y ActivationID (por omisión, nuevos).
	Revision     int64
	LicenseID    string
	ActivationID string
	// LicenseeName (por omisión «Restaurante de prueba»).
	LicenseeName string
}

// licenseContent es el contenido de la §4, con los campos en su orden.
type licenseContent struct {
	SchemaVersion         string          `json:"schema_version"`
	ProductID             string          `json:"product_id"`
	Environment           string          `json:"environment"`
	LicenseID             string          `json:"license_id"`
	ActivationID          string          `json:"activation_id"`
	ServerID              string          `json:"server_id"`
	InstallationPublicKey string          `json:"installation_public_key"`
	LicenseeName          string          `json:"licensee_name"`
	LicenseType           string          `json:"license_type"`
	BillingPeriod         *string         `json:"billing_period"`
	LicenseStatus         string          `json:"license_status"`
	StatusReason          *string         `json:"status_reason"`
	LicenseRevision       int64           `json:"license_revision"`
	IssuedAt              string          `json:"issued_at"`
	PeriodStartsAt        string          `json:"period_starts_at"`
	ExpiresAt             *string         `json:"expires_at"`
	GraceDays             int             `json:"grace_days"`
	UpdatesUntil          string          `json:"updates_until"`
	Plan                  contentPlan     `json:"plan"`
	Limits                contentLimits   `json:"limits"`
	Features              map[string]bool `json:"features"`
}

type contentPlan struct {
	PlanID string `json:"plan_id"`
	Name   string `json:"name"`
	Custom bool   `json:"custom"`
}

type contentLimits struct {
	CashRegisters   int `json:"cash_registers"`
	OrderTakers     int `json:"order_takers"`
	KitchenDisplays int `json:"kitchen_displays"`
}

// instant es el formato que firma el servidor de licencias: seis decimales (§3).
func instant(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000Z")
}

// NewUUID es un UUID versión 4 en minúsculas.
func NewUUID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	text := hex.EncodeToString(value[:])
	return text[0:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:32]
}

// content arma el contenido de la licencia con los valores por omisión.
func (options IssueOptions) content() (licenseContent, error) {
	if options.ServerID == "" || options.PublicKey == "" {
		return licenseContent{}, errors.New("falta la instalación destinataria (server_id y llave pública)")
	}
	licenseType := options.Type
	if licenseType == "" {
		licenseType = TypePerpetual
	}
	issued := options.IssuedAt
	if issued.IsZero() {
		issued = time.Now()
	}
	starts := options.PeriodStartsAt
	if starts.IsZero() {
		starts = issued
	}
	planID := options.PlanID
	if planID == "" {
		planID = "elemental"
	}
	plan, known := plans[planID]
	if !known {
		return licenseContent{}, errors.New("plan desconocido: usa basico, elemental o premium")
	}
	limits := plan.limits
	custom := false
	if options.Limits != nil {
		limits = *options.Limits
		custom = limits != plan.limits
	}
	content := licenseContent{
		SchemaVersion:         "1.0",
		ProductID:             licensing.ProductID,
		Environment:           "test",
		LicenseID:             options.LicenseID,
		ActivationID:          options.ActivationID,
		ServerID:              options.ServerID,
		InstallationPublicKey: options.PublicKey,
		LicenseeName:          options.LicenseeName,
		LicenseType:           licenseType,
		LicenseStatus:         "active",
		LicenseRevision:       options.Revision,
		IssuedAt:              instant(issued),
		PeriodStartsAt:        instant(starts),
		Plan:                  contentPlan{PlanID: planID, Name: plan.name, Custom: custom},
		Limits:                contentLimits{CashRegisters: limits.CashRegisters, OrderTakers: limits.OrderTakers, KitchenDisplays: limits.KitchenDisplays},
		Features:              map[string]bool{},
	}
	if content.LicenseID == "" {
		content.LicenseID = NewUUID()
	}
	if content.ActivationID == "" {
		content.ActivationID = NewUUID()
	}
	if content.LicenseeName == "" {
		content.LicenseeName = "Restaurante de prueba"
	}
	if content.LicenseRevision == 0 {
		content.LicenseRevision = 1
	}
	grace := 0
	switch licenseType {
	case TypeSubscription:
		period := options.BillingPeriod
		if period == "" {
			period = "monthly"
		}
		content.BillingPeriod = &period
		expires := options.ExpiresAt
		if expires.IsZero() {
			expires = issued.AddDate(0, 1, 0)
		}
		text := instant(expires)
		content.ExpiresAt = &text
		content.UpdatesUntil = text
		grace = 5
	case TypeTrial:
		expires := options.ExpiresAt
		if expires.IsZero() {
			expires = issued.AddDate(0, 0, 15)
		}
		text := instant(expires)
		content.ExpiresAt = &text
		content.UpdatesUntil = text
	case TypePerpetual:
		content.UpdatesUntil = instant(issued.AddDate(0, 6, 0))
	default:
		return licenseContent{}, errors.New("tipo de licencia desconocido: usa perpetual, subscription o trial")
	}
	if !options.UpdatesUntil.IsZero() {
		content.UpdatesUntil = instant(options.UpdatesUntil)
	}
	if options.GraceDays != nil {
		grace = *options.GraceDays
	}
	content.GraceDays = grace
	switch options.Status {
	case "", "active":
	case "revoked":
		reason := options.Reason
		if reason == "" {
			reason = licensing.ReasonRevoked
		}
		content.LicenseStatus = "revoked"
		content.StatusReason = &reason
	default:
		return licenseContent{}, errors.New("estado desconocido: usa active o revoked")
	}
	return content, nil
}

// Issue firma la licencia y devuelve el JWS (el contenido de un .lic). Antes la verifica con la §10, como lo hará el
// servidor, para que un parámetro inválido falle aquí con su motivo.
func (fixtures Fixtures) Issue(options IssueOptions) (string, error) {
	content, err := options.content()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return "", err
	}
	kid := options.Kid
	if kid == "" {
		kid = DefaultKid
	}
	jws, err := fixtures.SignJWS(kid, raw)
	if err != nil {
		return "", err
	}
	verifier := licensing.Verifier{
		Environment:  "test",
		Keys:         fixtures.trustedKeys(),
		Installation: licensing.Installation{ServerID: options.ServerID, PublicKey: options.PublicKey},
	}
	if _, err := verifier.Verify(jws, nil); err != nil {
		return "", errors.New("la licencia no pasa la verificación del contrato: " + err.Error())
	}
	return jws, nil
}

// trustedKeys son las públicas de las llaves de firma de prueba.
func (fixtures Fixtures) trustedKeys() map[string]licensing.TrustedKey {
	keys := make(map[string]licensing.TrustedKey, len(fixtures.SigningKeys))
	for kid, private := range fixtures.SigningKeys {
		keys[kid] = licensing.TrustedKey{Environment: "test", PublicKey: private.Public().(ed25519.PublicKey)}
	}
	return keys
}

// Trust son las llaves de firma de prueba como llaves de confianza del entorno «test», para un gestor de pruebas.
func (fixtures Fixtures) Trust() licensing.TrustSet {
	return licensing.TrustSet{Environment: "test", Keys: fixtures.trustedKeys()}
}
