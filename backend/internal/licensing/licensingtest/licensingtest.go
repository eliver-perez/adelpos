// Package licensingtest arma gestores de licencias para las pruebas automáticas de otros paquetes (la puerta de la
// licencia, la hora y el contrato de la API): licencias firmadas con las llaves de prueba de
// contracts/licencias/v1/fixtures/vectors.json (semillas públicas que solo usan las pruebas, §13 del contrato) para una
// instalación de esos mismos archivos, con un reloj simulado y una carpeta temporal. Nunca usa las llaves del servidor
// de licencias ni llama a ningún servidor.
//
// Solo lo importan archivos de prueba: fuera de una prueba (testing.Testing) no arma nada.
package licensingtest

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/licensing"
)

// Datos de vectors.json que usan estas licencias.
const (
	signingKid       = "adelpos-test-fixture-a"
	installationName = "installation-fixture-a"
	licenseID        = "10000000-0000-4000-8000-0000000000c1"
	activationID     = "30000000-0000-4000-8000-0000000000c1"
	// MachineID es el identificador de equipo simulado de la computadora original (nunca el real).
	MachineID = "CCCCCCCC-1111-2222-3333-444444444444"
	// OtherMachineID es el de otra computadora (una carpeta de datos copiada).
	OtherMachineID = "DDDDDDDD-5555-6666-7777-888888888888"
)

// Clock es un reloj simulado: las pruebas lo mueven en lugar de cambiar la hora de la computadora.
type Clock struct {
	mutex   sync.Mutex
	current time.Time
}

// NewClock empieza en el instante indicado.
func NewClock(start time.Time) *Clock {
	return &Clock{current: start.UTC()}
}

// Now es la hora simulada.
func (clock *Clock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.current
}

// Set mueve el reloj a un instante (también hacia atrás).
func (clock *Clock) Set(instant time.Time) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.current = instant.UTC()
}

// Advance adelanta (o atrasa, con una duración negativa) el reloj.
func (clock *Clock) Advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.current = clock.current.Add(duration)
}

// Fixture son los datos de una instalación de prueba: su carpeta identity/licencia, su reloj y su equipo.
type Fixture struct {
	t            *testing.T
	Clock        *Clock
	Directory    string
	Installation licensing.Installation
	Trust        licensing.TrustSet
	// Database es la consulta de la base del gestor (nil: sin operaciones).
	Database licensing.DatabaseProbe

	mutex    sync.Mutex
	machine  string
	signing  ed25519.PrivateKey
	revision int64
}

type vectorKey struct {
	Kid         string `json:"kid"`
	Environment string `json:"environment"`
	SeedHex     string `json:"seed_hex"`
	PublicKey   string `json:"public_key"`
}

type vectorInstallation struct {
	Name      string `json:"name"`
	ServerID  string `json:"server_id"`
	PublicKey string `json:"public_key"`
}

// vectorsFile busca contracts/licencias/v1/fixtures/vectors.json subiendo desde la carpeta de la prueba.
func vectorsFile() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for range 8 {
		candidate := filepath.Join(directory, "contracts", "licencias", "v1", "fixtures", "vectors.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		directory = filepath.Dir(directory)
	}
	return "", errors.New("no se encontraron los archivos de prueba del contrato")
}

// New prepara una instalación de prueba con su reloj (si clock es nil, empieza en 2026-11-23 18:00 UTC).
func New(t *testing.T, clock *Clock) *Fixture {
	t.Helper()
	if !testing.Testing() {
		panic("licensingtest solo se usa en las pruebas")
	}
	if clock == nil {
		clock = NewClock(time.Date(2026, 11, 23, 18, 0, 0, 0, time.UTC))
	}
	filename, err := vectorsFile()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		SigningKeys   []vectorKey          `json:"signing_keys"`
		Installations []vectorInstallation `json:"installations"`
	}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	fixture := &Fixture{
		t:         t,
		Clock:     clock,
		Directory: filepath.Join(t.TempDir(), "identity", "licencia"),
		machine:   MachineID,
		Trust:     licensing.TrustSet{Environment: "test", Keys: map[string]licensing.TrustedKey{}},
	}
	for _, key := range vectors.SigningKeys {
		if key.Kid != signingKid {
			continue
		}
		seed, err := hex.DecodeString(key.SeedHex)
		if err != nil || len(seed) != ed25519.SeedSize {
			t.Fatal("semilla de prueba inválida")
		}
		fixture.signing = ed25519.NewKeyFromSeed(seed)
		fixture.Trust.Keys[key.Kid] = licensing.TrustedKey{Environment: key.Environment,
			PublicKey: fixture.signing.Public().(ed25519.PublicKey)}
	}
	for _, installation := range vectors.Installations {
		if installation.Name == installationName {
			fixture.Installation = licensing.Installation{ServerID: installation.ServerID, PublicKey: installation.PublicKey}
		}
	}
	if fixture.signing == nil || fixture.Installation.ServerID == "" {
		t.Fatal("faltan la llave o la instalación de prueba en vectors.json")
	}
	return fixture
}

// SetMachine cambia el identificador del equipo (OtherMachineID simula la carpeta copiada a otra computadora).
func (fixture *Fixture) SetMachine(machine string) {
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	fixture.machine = machine
}

func (fixture *Fixture) machineID() (string, error) {
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	return fixture.machine, nil
}

// Start crea y carga un gestor nuevo sobre la carpeta de la instalación (un arranque del servidor).
func (fixture *Fixture) Start() *licensing.Manager {
	fixture.t.Helper()
	installation := fixture.Installation
	trust := fixture.Trust
	manager := licensing.NewManager(licensing.Options{
		Directory:         fixture.Directory,
		Installation:      &installation,
		Trust:             &trust,
		Now:               fixture.Clock.Now,
		MachineID:         fixture.machineID,
		MachineRetryDelay: time.Millisecond,
		Database:          fixture.Database,
	})
	if err := manager.Load(context.Background()); err != nil {
		fixture.t.Fatalf("no se pudo cargar el gestor de prueba: %v", err)
	}
	return manager
}

// Claims es el contenido variable de una licencia de prueba.
type Claims struct {
	// Type es perpetual, subscription o trial.
	Type string
	// Revoked con Reason (deactivated, transferred o revoked) es una licencia revocada.
	Revoked bool
	Reason  string
	// IssuedAt es la emisión (sin valor, una hora antes del reloj).
	IssuedAt time.Time
	// ExpiresAt es el vencimiento de una suscripción o una prueba.
	ExpiresAt time.Time
	Limits    licensing.Limits
}

// Sign firma una licencia de prueba para la instalación, con una revisión mayor que la anterior.
func (fixture *Fixture) Sign(claims Claims) string {
	fixture.t.Helper()
	fixture.mutex.Lock()
	fixture.revision++
	revision := fixture.revision
	fixture.mutex.Unlock()
	issued := claims.IssuedAt
	if issued.IsZero() {
		issued = fixture.Clock.Now().Add(-time.Hour)
	}
	instant := func(value time.Time) string {
		return value.UTC().Format("2006-01-02T15:04:05.000000Z")
	}
	content := map[string]any{
		"schema_version":          "1.0",
		"product_id":              licensing.ProductID,
		"environment":             "test",
		"license_id":              licenseID,
		"activation_id":           activationID,
		"server_id":               fixture.Installation.ServerID,
		"installation_public_key": fixture.Installation.PublicKey,
		"licensee_name":           "Restaurante de Pruebas",
		"license_type":            claims.Type,
		"billing_period":          nil,
		"license_status":          "active",
		"status_reason":           nil,
		"license_revision":        revision,
		"issued_at":               instant(issued),
		"period_starts_at":        instant(issued),
		"expires_at":              nil,
		"grace_days":              0,
		"updates_until":           instant(issued.AddDate(1, 0, 0)),
		"plan":                    map[string]any{"plan_id": "pruebas", "name": "Plan de pruebas", "custom": false},
		"limits": map[string]any{
			"cash_registers":   claims.Limits.CashRegisters,
			"order_takers":     claims.Limits.OrderTakers,
			"kitchen_displays": claims.Limits.KitchenDisplays,
		},
		"features": map[string]any{},
	}
	switch claims.Type {
	case "subscription":
		content["billing_period"] = "monthly"
		content["grace_days"] = 5
		content["expires_at"] = instant(claims.ExpiresAt)
	case "trial":
		content["expires_at"] = instant(claims.ExpiresAt)
	}
	if claims.Revoked {
		content["license_status"] = "revoked"
		content["status_reason"] = claims.Reason
	}
	body, err := json.Marshal(content)
	if err != nil {
		fixture.t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	header := encode([]byte(fmt.Sprintf(`{"alg":"EdDSA","kid":%q,"typ":"adelpos-lic+jws"}`, signingKid)))
	payload := encode(body)
	return header + "." + payload + "." + encode(ed25519.Sign(fixture.signing, []byte(header+"."+payload)))
}

// Accept firma y acepta una licencia en el gestor; la prueba falla si no se acepta.
func (fixture *Fixture) Accept(manager *licensing.Manager, claims Claims) licensing.Status {
	fixture.t.Helper()
	status, err := manager.Accept(fixture.Sign(claims))
	if err != nil {
		fixture.t.Fatalf("la licencia de prueba no se aceptó: %v", err)
	}
	return status
}

// Perpetual es una perpetua con esos cupos.
func Perpetual(cashRegisters, orderTakers, kitchenDisplays int) Claims {
	return Claims{Type: "perpetual", Limits: licensing.Limits{CashRegisters: cashRegisters, OrderTakers: orderTakers,
		KitchenDisplays: kitchenDisplays}}
}

// Subscription es una suscripción mensual (5 días de tolerancia) que vence en expiresAt.
func Subscription(expiresAt time.Time, cashRegisters, orderTakers, kitchenDisplays int) Claims {
	claims := Perpetual(cashRegisters, orderTakers, kitchenDisplays)
	claims.Type = "subscription"
	claims.ExpiresAt = expiresAt
	return claims
}

// Revoked es una perpetua revocada con ese motivo.
func Revoked(reason string) Claims {
	claims := Perpetual(3, 3, 3)
	claims.Revoked = true
	claims.Reason = reason
	return claims
}

// SupportKey es una llave de soporte de prueba (nunca la de ~/.adel-pos-dev/soporte): su privada para firmar
// respuestas y la confianza que se inyecta en el servidor (administration.Service.SupportTrust).
type SupportKey struct {
	Kid     string
	Private ed25519.PrivateKey
	Trust   licensing.SupportTrust
}

// NewSupportKey crea una llave de soporte de prueba del entorno de compilación.
func NewSupportKey(t *testing.T) SupportKey {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	kid := "soporte-de-prueba"
	return SupportKey{Kid: kid, Private: private, Trust: licensing.SupportTrust{
		Environment: licensing.BuildEnvironment, Keys: map[string]ed25519.PublicKey{kid: public}}}
}

// Respond firma la respuesta a un código de solicitud, con la nueva última hora confiable.
func (key SupportKey) Respond(t *testing.T, code string, newTrustedTime, issuedAt time.Time) string {
	t.Helper()
	parsed, err := licensing.ParseSupportCode(code)
	if err != nil {
		t.Fatal(err)
	}
	response, err := licensing.SignSupportResponse(key.Private, key.Kid, licensing.BuildEnvironment, parsed, newTrustedTime, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
