package licensing

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Los casos de prueba del contrato (§13), copiados sin cambios de la etiqueta v1.0.0 en
// contracts/licencias/v1 (contracts/licencias/ORIGEN.md). Si un caso no coincide, el error está en la
// integración: estos archivos no se editan.
const fixturesDirectory = "../../../contracts/licencias/v1/fixtures"

type fixtureSigningKey struct {
	Kid         string `json:"kid"`
	Environment string `json:"environment"`
	SeedHex     string `json:"seed_hex"`
	PublicKey   string `json:"public_key"`
}

type fixtureInstallation struct {
	Name      string `json:"name"`
	ServerID  string `json:"server_id"`
	SeedHex   string `json:"seed_hex"`
	PublicKey string `json:"public_key"`
}

type fixtureLicense struct {
	Name         string          `json:"name"`
	Kid          string          `json:"kid"`
	Installation string          `json:"installation"`
	Content      json.RawMessage `json:"content"`
	JWS          string          `json:"jws"`
	SHA256       string          `json:"sha256"`
}

type fixtureVerifier struct {
	Environment  string `json:"environment"`
	Installation string `json:"installation"`
}

type fixtureNegative struct {
	Name          string          `json:"name"`
	JWS           string          `json:"jws"`
	Verifier      fixtureVerifier `json:"verifier"`
	ExpectedError string          `json:"expected_error"`
}

type fixtureAccepted struct {
	LicenseID string `json:"license_id"`
	Revision  int64  `json:"license_revision"`
	SHA256    string `json:"sha256"`
}

type fixtureRevision struct {
	Name         string           `json:"name"`
	Installation string           `json:"installation"`
	Accepted     *fixtureAccepted `json:"accepted"`
	JWS          string           `json:"jws"`
	Expected     string           `json:"expected"`
}

type fixtureState struct {
	Name          string  `json:"name"`
	License       string  `json:"license"`
	LocalClock    string  `json:"local_clock"`
	LastTrusted   *string `json:"last_trusted_time"`
	ExpectedState string  `json:"expected_state"`
}

type fixtureProof struct {
	Action       string  `json:"action"`
	Installation string  `json:"installation"`
	ChallengeID  string  `json:"challenge_id"`
	Nonce        string  `json:"nonce"`
	ProductID    string  `json:"product_id"`
	ServerID     string  `json:"server_id"`
	ActivationID *string `json:"activation_id"`
	Message      string  `json:"message"`
	Proof        string  `json:"proof"`
}

type fixtureRequest struct {
	Action       string            `json:"action"`
	Installation string            `json:"installation"`
	Payload      map[string]string `json:"payload"`
	Message      string            `json:"message"`
	EnvelopeJSON string            `json:"envelope_json"`
}

type fixtureSet struct {
	Version       string                `json:"fixture_version"`
	SigningKeys   []fixtureSigningKey   `json:"signing_keys"`
	Installations []fixtureInstallation `json:"installations"`
	Licenses      []fixtureLicense      `json:"licenses"`
	Negative      []fixtureNegative     `json:"negative_licenses"`
	Revisions     []fixtureRevision     `json:"revision_cases"`
	States        []fixtureState        `json:"temporal_states"`
	Proofs        []fixtureProof        `json:"proofs"`
	Requests      []fixtureRequest      `json:"offline_requests"`
}

// fixtures son los archivos de prueba ya leídos, con las llaves derivadas de sus semillas públicas.
type fixtures struct {
	set           fixtureSet
	installations map[string]Installation
	privateKeys   map[string]ed25519.PrivateKey
	signingKeys   map[string]ed25519.PrivateKey
	trusted       map[string]map[string]TrustedKey
}

func loadFixtures(t *testing.T) fixtures {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixturesDirectory, "vectors.json"))
	if err != nil {
		t.Fatalf("no se pudieron leer los archivos de prueba del contrato: %v", err)
	}
	var loaded fixtures
	if err := json.Unmarshal(raw, &loaded.set); err != nil || loaded.set.Version != "1.0" {
		t.Fatal("vectors.json no es la versión 1.0 de los archivos de prueba")
	}
	loaded.installations = map[string]Installation{}
	loaded.privateKeys = map[string]ed25519.PrivateKey{}
	for _, installation := range loaded.set.Installations {
		private := privateKeyFromSeed(t, installation.Name, installation.SeedHex, installation.PublicKey)
		loaded.installations[installation.Name] = Installation{ServerID: installation.ServerID, PublicKey: installation.PublicKey}
		loaded.privateKeys[installation.Name] = private
	}
	// Llaves de confianza de cada entorno: una versión de producción nunca incluye las de pruebas.
	loaded.trusted = map[string]map[string]TrustedKey{"test": {}, "production": {}}
	loaded.signingKeys = map[string]ed25519.PrivateKey{}
	for _, key := range loaded.set.SigningKeys {
		private := privateKeyFromSeed(t, key.Kid, key.SeedHex, key.PublicKey)
		loaded.signingKeys[key.Kid] = private
		loaded.trusted[key.Environment][key.Kid] = TrustedKey{
			Environment: key.Environment,
			PublicKey:   private.Public().(ed25519.PublicKey),
		}
	}
	return loaded
}

func (loaded fixtures) verifier(environment, installation string) Verifier {
	return Verifier{Environment: environment, Keys: loaded.trusted[environment], Installation: loaded.installations[installation]}
}

// testTrust son las llaves de prueba del entorno «test» de los archivos de prueba, para el gestor.
func (loaded fixtures) testTrust() *TrustSet {
	return &TrustSet{Environment: "test", Keys: loaded.trusted["test"]}
}

func (loaded fixtures) license(t *testing.T, name string) fixtureLicense {
	t.Helper()
	for _, item := range loaded.set.Licenses {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("no existe la licencia de prueba %s", name)
	return fixtureLicense{}
}

// sign firma un contenido con una llave de firma de los archivos de prueba (semillas públicas), con el
// encabezado exacto del contrato. Sirve para armar casos que vectors.json no trae.
func (loaded fixtures) sign(t *testing.T, kid string, content []byte) string {
	t.Helper()
	private, known := loaded.signingKeys[kid]
	if !known {
		t.Fatalf("no existe la llave de prueba %s", kid)
	}
	kidJSON, _ := json.Marshal(kid)
	header := encodeBase64URL([]byte(`{"alg":"EdDSA","kid":` + string(kidJSON) + `,"typ":"` + jwsType + `"}`))
	body := encodeBase64URL(content)
	return header + "." + body + "." + encodeBase64URL(ed25519.Sign(private, []byte(header+"."+body)))
}

// privateKeyFromSeed deriva la llave de una semilla pública de los archivos de prueba y comprueba que su
// pública sea la publicada. Nunca se muestra.
func privateKeyFromSeed(t *testing.T, name, seedHex, publicKey string) ed25519.PrivateKey {
	t.Helper()
	seed, err := hex.DecodeString(seedHex)
	if err != nil || len(seed) != ed25519.SeedSize {
		t.Fatalf("llave %s: semilla inválida", name)
	}
	private := ed25519.NewKeyFromSeed(seed)
	public, canonical := decodeBase64URL(publicKey)
	if !canonical || !private.Public().(ed25519.PublicKey).Equal(ed25519.PublicKey(public)) {
		t.Fatalf("llave %s: la pública no corresponde a su semilla", name)
	}
	return private
}

// codeOf convierte el resultado de una verificación en el texto que usan los archivos de prueba.
func codeOf(err error) string {
	if err == nil {
		return "accepted"
	}
	var rejected *VerifyError
	if errors.As(err, &rejected) {
		return rejected.Code
	}
	return "error"
}

// TestContractVectors recorre los 67 casos de vectors.json, igual que el verificador de referencia: 20
// licencias válidas, 17 negativas, 10 de revisión, 13 estados temporales, 4 pruebas de posesión y 3 `.licreq`.
func TestContractVectors(t *testing.T) {
	loaded := loadFixtures(t)
	counts := []struct {
		name     string
		got      int
		expected int
	}{
		{"licencias", len(loaded.set.Licenses), 20},
		{"casos negativos", len(loaded.set.Negative), 17},
		{"casos de revisión", len(loaded.set.Revisions), 10},
		{"estados temporales", len(loaded.set.States), 13},
		{"pruebas de posesión", len(loaded.set.Proofs), 4},
		{"solicitudes .licreq", len(loaded.set.Requests), 3},
	}
	total := 0
	for _, count := range counts {
		if count.got != count.expected {
			t.Fatalf("%s: se esperaban %d y hay %d", count.name, count.expected, count.got)
		}
		total += count.got
	}
	if total != 67 {
		t.Fatalf("se esperaban 67 casos y hay %d", total)
	}

	accepted := map[string]*License{}
	t.Run("licencias", func(t *testing.T) {
		for _, item := range loaded.set.Licenses {
			t.Run(item.Name, func(t *testing.T) {
				// Cada licencia la verifica la instalación a la que va: después de una transferencia, la nueva.
				license, err := loaded.verifier("test", item.Installation).Verify(item.JWS, nil)
				if err != nil {
					t.Fatalf("se rechazó: %v", err)
				}
				accepted[item.Name] = license
				checkFixtureContent(t, item, license)
				file, err := os.ReadFile(filepath.Join(fixturesDirectory, "licenses", item.Name+".lic"))
				if err != nil || string(file) != item.JWS {
					t.Fatal("el archivo .lic no coincide")
				}
			})
		}
	})

	t.Run("negativos", func(t *testing.T) {
		for _, item := range loaded.set.Negative {
			t.Run(item.Name, func(t *testing.T) {
				_, err := loaded.verifier(item.Verifier.Environment, item.Verifier.Installation).Verify(item.JWS, nil)
				if code := codeOf(err); code != item.ExpectedError {
					t.Fatalf("se esperaba %s y resultó %s", item.ExpectedError, code)
				}
			})
		}
		// Aunque una versión de producción incluyera por error una llave de pruebas, no la acepta: además de
		// conocer la llave, comprueba que sea de su propio entorno.
		mixed := Verifier{Environment: "production", Keys: loaded.trusted["test"], Installation: loaded.installations["installation-fixture-a"]}
		if _, err := mixed.Verify(loaded.set.Licenses[0].JWS, nil); codeOf(err) != ErrUnknownKid {
			t.Fatalf("una instalación de producción aceptó una llave de pruebas (%s)", codeOf(err))
		}
	})

	t.Run("revisiones", func(t *testing.T) {
		for _, item := range loaded.set.Revisions {
			t.Run(item.Name, func(t *testing.T) {
				var previous *Accepted
				if item.Accepted != nil {
					previous = &Accepted{LicenseID: item.Accepted.LicenseID, Revision: item.Accepted.Revision, SHA256: item.Accepted.SHA256}
				}
				_, err := loaded.verifier("test", item.Installation).Verify(item.JWS, previous)
				if code := codeOf(err); code != item.Expected {
					t.Fatalf("se esperaba %s y resultó %s", item.Expected, code)
				}
			})
		}
	})

	t.Run("estados temporales", func(t *testing.T) {
		for _, item := range loaded.set.States {
			t.Run(item.Name, func(t *testing.T) {
				license, known := accepted[item.License]
				clock, errClock := time.Parse(time.RFC3339Nano, item.LocalClock)
				if !known || errClock != nil {
					t.Fatal("caso mal formado")
				}
				var lastTrusted *time.Time
				if item.LastTrusted != nil {
					parsed, err := time.Parse(time.RFC3339Nano, *item.LastTrusted)
					if err != nil {
						t.Fatal("última hora confiable inválida")
					}
					lastTrusted = &parsed
				}
				if state := TemporalState(license, clock, lastTrusted); string(state) != item.ExpectedState {
					t.Fatalf("se esperaba %s y resultó %s", item.ExpectedState, state)
				}
			})
		}
	})

	t.Run("pruebas de posesión", func(t *testing.T) {
		for _, item := range loaded.set.Proofs {
			t.Run(item.Action, func(t *testing.T) {
				message := ProofMessage(item.Action, item.ChallengeID, item.Nonce, item.ProductID, item.ServerID, item.ActivationID)
				if message != item.Message {
					t.Fatal("el mensaje no coincide")
				}
				publicKey := loaded.installations[item.Installation].PublicKey
				if !VerifyProof(message, item.Proof, publicKey) {
					t.Fatal("la firma no verifica")
				}
				if VerifyProof(message+" ", item.Proof, publicKey) {
					t.Fatal("verifica con otro mensaje")
				}
				// Ed25519 es determinista: la firma de AD-EL POS es exactamente la publicada.
				if SignProof(loaded.privateKeys[item.Installation], message) != item.Proof {
					t.Fatal("la prueba firmada por AD-EL POS no es la publicada")
				}
			})
		}
	})

	t.Run("solicitudes sin internet", func(t *testing.T) {
		for _, item := range loaded.set.Requests {
			t.Run(item.Action, func(t *testing.T) {
				payload, err := VerifyOfflineRequest([]byte(item.EnvelopeJSON))
				if err != nil {
					t.Fatal(err)
				}
				if len(payload) != len(item.Payload) || payload["action"] != item.Action {
					t.Fatal("el contenido no coincide")
				}
				for field, value := range item.Payload {
					if payload[field] != value {
						t.Fatalf("el campo %s no coincide", field)
					}
				}
				file, err := os.ReadFile(filepath.Join(fixturesDirectory, "requests", item.Action+".licreq"))
				if err != nil || string(file) != item.EnvelopeJSON {
					t.Fatal("el archivo .licreq no coincide")
				}
				// Un sobre con la firma de otro contenido no verifica.
				var envelope map[string]string
				if err := json.Unmarshal(file, &envelope); err != nil {
					t.Fatal(err)
				}
				other := loaded.set.Requests[(indexOfRequest(loaded.set.Requests, item.Action)+1)%len(loaded.set.Requests)]
				var otherEnvelope map[string]string
				if err := json.Unmarshal([]byte(other.EnvelopeJSON), &otherEnvelope); err != nil {
					t.Fatal(err)
				}
				envelope["signature_b64u"] = otherEnvelope["signature_b64u"]
				tampered, _ := json.Marshal(envelope)
				if _, err := VerifyOfflineRequest(tampered); err == nil {
					t.Fatal("un .licreq con la firma de otro contenido verifica")
				}
			})
		}
	})
}

func indexOfRequest(requests []fixtureRequest, action string) int {
	for index, item := range requests {
		if item.Action == action {
			return index
		}
	}
	return -1
}

// checkFixtureContent compara la licencia verificada con el contenido publicado en los archivos de prueba.
func checkFixtureContent(t *testing.T, item fixtureLicense, license *License) {
	t.Helper()
	var expected struct {
		LicenseID       string  `json:"license_id"`
		LicenseType     string  `json:"license_type"`
		BillingPeriod   *string `json:"billing_period"`
		LicenseStatus   string  `json:"license_status"`
		StatusReason    *string `json:"status_reason"`
		LicenseRevision int64   `json:"license_revision"`
		ExpiresAt       *string `json:"expires_at"`
		GraceDays       int64   `json:"grace_days"`
		Plan            struct {
			PlanID string `json:"plan_id"`
			Custom bool   `json:"custom"`
		} `json:"plan"`
		Limits struct {
			CashRegisters   int `json:"cash_registers"`
			OrderTakers     int `json:"order_takers"`
			KitchenDisplays int `json:"kitchen_displays"`
		} `json:"limits"`
		Features map[string]bool `json:"features"`
	}
	if err := json.Unmarshal(item.Content, &expected); err != nil {
		t.Fatal("contenido publicado ilegible")
	}
	same := expected.LicenseID == license.LicenseID &&
		expected.LicenseType == license.LicenseType &&
		equalText(expected.BillingPeriod, license.BillingPeriod) &&
		expected.LicenseStatus == license.LicenseStatus &&
		equalText(expected.StatusReason, license.StatusReason) &&
		expected.LicenseRevision == license.LicenseRevision &&
		expected.GraceDays == license.GraceDays &&
		expected.Plan.PlanID == license.Plan.PlanID &&
		expected.Plan.Custom == license.Plan.Custom &&
		expected.Limits.CashRegisters == license.Limits.CashRegisters &&
		expected.Limits.OrderTakers == license.Limits.OrderTakers &&
		expected.Limits.KitchenDisplays == license.Limits.KitchenDisplays &&
		len(expected.Features) == len(license.Features) &&
		item.SHA256 == license.SHA256
	if (expected.ExpiresAt == nil) != (license.ExpiresAt == nil) {
		same = false
	}
	if expected.ExpiresAt != nil && license.ExpiresAt != nil {
		parsed, err := time.Parse(time.RFC3339Nano, *expected.ExpiresAt)
		same = same && err == nil && parsed.Equal(*license.ExpiresAt)
	}
	if !same {
		t.Fatalf("el contenido verificado no es el publicado (%s r%d)", license.LicenseID, license.LicenseRevision)
	}
}

func equalText(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
