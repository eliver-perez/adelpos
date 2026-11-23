// Verificador de referencia del contrato de licencias de AD-EL POS V1.0.
//
// Aplica las reglas que debe aplicar el servidor Go del restaurante (contrato, secciones 5.6
// y 10) y las ejecuta contra todos los archivos de prueba compartidos:
//
//	go run . ../fixtures/vectors.json
//
// No es un validador de producción: es la base, legible y sin dependencias, de la integración
// de AD-EL POS. Solo usa la biblioteca estándar de Go.
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type signingKey struct {
	Kid         string `json:"kid"`
	Environment string `json:"environment"`
	SeedHex     string `json:"seed_hex"`
	PublicKey   string `json:"public_key"`
}

type installationKey struct {
	Name      string `json:"name"`
	ServerID  string `json:"server_id"`
	SeedHex   string `json:"seed_hex"`
	PublicKey string `json:"public_key"`
}

type licenseCase struct {
	Name         string          `json:"name"`
	Kid          string          `json:"kid"`
	Installation string          `json:"installation"`
	Content      json.RawMessage `json:"content"`
	HeaderJSON   string          `json:"header_json"`
	ContentJSON  string          `json:"content_json"`
	JWS          string          `json:"jws"`
	SHA256       string          `json:"sha256"`
}

type verifierCase struct {
	Environment  string `json:"environment"`
	Installation string `json:"installation"`
}

type negativeCase struct {
	Name          string       `json:"name"`
	JWS           string       `json:"jws"`
	Verifier      verifierCase `json:"verifier"`
	ExpectedError string       `json:"expected_error"`
}

type acceptedCase struct {
	LicenseID string `json:"license_id"`
	Revision  int64  `json:"license_revision"`
	SHA256    string `json:"sha256"`
}

type revisionCase struct {
	Name         string        `json:"name"`
	Installation string        `json:"installation"`
	Accepted     *acceptedCase `json:"accepted"`
	JWS          string        `json:"jws"`
	Expected     string        `json:"expected"`
}

type stateCase struct {
	Name          string  `json:"name"`
	License       string  `json:"license"`
	LocalClock    string  `json:"local_clock"`
	LastTrusted   *string `json:"last_trusted_time"`
	ExpectedState string  `json:"expected_state"`
}

type proofCase struct {
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

type requestCase struct {
	Action       string            `json:"action"`
	Installation string            `json:"installation"`
	Payload      map[string]string `json:"payload"`
	Message      string            `json:"message"`
	EnvelopeJSON string            `json:"envelope_json"`
}

type fixtureSet struct {
	Version       string            `json:"fixture_version"`
	SigningKeys   []signingKey      `json:"signing_keys"`
	Installations []installationKey `json:"installations"`
	Licenses      []licenseCase     `json:"licenses"`
	Negative      []negativeCase    `json:"negative_licenses"`
	Revisions     []revisionCase    `json:"revision_cases"`
	States        []stateCase       `json:"temporal_states"`
	Proofs        []proofCase       `json:"proofs"`
	Requests      []requestCase     `json:"offline_requests"`
}

// failures junta todo lo que no coincide, para mostrarlo completo al final.
type failures []string

func (f *failures) add(format string, values ...any) {
	*f = append(*f, fmt.Sprintf(format, values...))
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "uso: go run . ../fixtures/vectors.json")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var fixtures fixtureSet
	if err := json.Unmarshal(raw, &fixtures); err != nil || fixtures.Version != "1.0" {
		fmt.Fprintln(os.Stderr, "no se pudieron leer los archivos de prueba, versión 1.0")
		os.Exit(2)
	}

	var failed failures
	installations := map[string]Installation{}
	for _, installation := range fixtures.Installations {
		checkSeed(&failed, installation.Name, installation.SeedHex, installation.PublicKey)
		installations[installation.Name] = Installation{ServerID: installation.ServerID, PublicKey: installation.PublicKey}
	}
	// Llaves de confianza de cada entorno: una versión de producción nunca incluye las de pruebas.
	trusted := map[string]map[string]TrustedKey{"test": {}, "production": {}}
	for _, key := range fixtures.SigningKeys {
		checkSeed(&failed, key.Kid, key.SeedHex, key.PublicKey)
		public, _ := decodeBase64URL(key.PublicKey)
		trusted[key.Environment][key.Kid] = TrustedKey{Environment: key.Environment, PublicKey: ed25519.PublicKey(public)}
	}
	verifier := func(environment, installation string) Verifier {
		return Verifier{Environment: environment, Keys: trusted[environment], Installation: installations[installation]}
	}
	directory := filepath.Dir(os.Args[1])

	// Licencias válidas: se aceptan y su contenido es el publicado.
	accepted := map[string]*License{}
	for _, item := range fixtures.Licenses {
		// Cada licencia la verifica la instalación a la que va: después de una transferencia, la nueva.
		license, err := verifier("test", item.Installation).Verify(item.JWS, nil)
		if err != nil {
			failed.add("licencia %s: se rechazó (%v)", item.Name, err)
			continue
		}
		accepted[item.Name] = license
		checkContent(&failed, item, license)
		file, err := os.ReadFile(filepath.Join(directory, "licenses", item.Name+".lic"))
		if err != nil || string(file) != item.JWS {
			failed.add("licencia %s: el archivo .lic no coincide", item.Name)
		}
	}

	// Casos negativos: se rechazan, y por el motivo esperado.
	for _, item := range fixtures.Negative {
		_, err := verifier(item.Verifier.Environment, item.Verifier.Installation).Verify(item.JWS, nil)
		if code := codeOf(err); code != item.ExpectedError {
			failed.add("caso negativo %s: se esperaba %s y resultó %s", item.Name, item.ExpectedError, code)
		}
	}

	// Aunque una versión de producción incluyera por error una llave de pruebas, no la acepta:
	// además de conocer la llave, comprueba que sea de su propio entorno.
	mixed := Verifier{
		Environment:  "production",
		Keys:         trusted["test"],
		Installation: installations["installation-fixture-a"],
	}
	if len(fixtures.Licenses) > 0 {
		if _, err := mixed.Verify(fixtures.Licenses[0].JWS, nil); codeOf(err) != ErrUnknownKid {
			failed.add("una instalación de producción aceptó una llave de pruebas (%s)", codeOf(err))
		}
	}

	// Regla de la revisión: nunca se retrocede.
	for _, item := range fixtures.Revisions {
		var previous *Accepted
		if item.Accepted != nil {
			previous = &Accepted{
				LicenseID: item.Accepted.LicenseID,
				Revision:  item.Accepted.Revision,
				SHA256:    item.Accepted.SHA256,
			}
		}
		_, err := verifier("test", item.Installation).Verify(item.JWS, previous)
		if code := codeOf(err); code != item.Expected {
			failed.add("caso de revisión %s: se esperaba %s y resultó %s", item.Name, item.Expected, code)
		}
	}

	// Estado temporal con ahora_efectivo = máximo(reloj local, última hora confiable).
	for _, item := range fixtures.States {
		license, known := accepted[item.License]
		clock, errClock := time.Parse(time.RFC3339Nano, item.LocalClock)
		if !known || errClock != nil {
			failed.add("estado %s: caso mal formado", item.Name)
			continue
		}
		var lastTrusted *time.Time
		if item.LastTrusted != nil {
			parsed, err := time.Parse(time.RFC3339Nano, *item.LastTrusted)
			if err != nil {
				failed.add("estado %s: última hora confiable inválida", item.Name)
				continue
			}
			lastTrusted = &parsed
		}
		if state := TemporalState(license, clock, lastTrusted); state != item.ExpectedState {
			failed.add("estado %s: se esperaba %s y resultó %s", item.Name, item.ExpectedState, state)
		}
	}

	// Pruebas de posesión de las cuatro acciones.
	for _, item := range fixtures.Proofs {
		message := ProofMessage(item.Action, item.ChallengeID, item.Nonce, item.ProductID, item.ServerID, item.ActivationID)
		if message != item.Message {
			failed.add("prueba de posesión %s: el mensaje no coincide", item.Action)
		}
		if !VerifyProof(message, item.Proof, installations[item.Installation].PublicKey) {
			failed.add("prueba de posesión %s: la firma no verifica", item.Action)
		}
		if VerifyProof(message+" ", item.Proof, installations[item.Installation].PublicKey) {
			failed.add("prueba de posesión %s: verifica con otro mensaje", item.Action)
		}
	}

	// Solicitudes sin internet de las tres acciones.
	for _, item := range fixtures.Requests {
		payload, err := VerifyOfflineRequest([]byte(item.EnvelopeJSON))
		if err != nil {
			failed.add("solicitud %s: %v", item.Action, err)
			continue
		}
		if len(payload) != len(item.Payload) || payload["action"] != item.Action {
			failed.add("solicitud %s: el contenido no coincide", item.Action)
		}
		for field, value := range item.Payload {
			if payload[field] != value {
				failed.add("solicitud %s: el campo %s no coincide", item.Action, field)
			}
		}
		file, err := os.ReadFile(filepath.Join(directory, "requests", item.Action+".licreq"))
		if err != nil || string(file) != item.EnvelopeJSON {
			failed.add("solicitud %s: el archivo .licreq no coincide", item.Action)
		}
	}

	if len(failed) > 0 {
		for _, line := range failed {
			fmt.Fprintln(os.Stderr, "FALLA ·", line)
		}
		os.Exit(1)
	}
	fmt.Printf(
		"OK: %d licencias, %d casos negativos, %d casos de revisión, %d estados temporales, "+
			"%d pruebas de posesión y %d solicitudes .licreq.\n",
		len(fixtures.Licenses), len(fixtures.Negative), len(fixtures.Revisions), len(fixtures.States),
		len(fixtures.Proofs), len(fixtures.Requests),
	)
}

// codeOf convierte el resultado de una verificación en el texto que usan los archivos de prueba.
func codeOf(err error) string {
	if err == nil {
		return "accepted"
	}
	if rejected, ok := err.(*VerifyError); ok {
		return rejected.Code
	}
	return "error"
}

// checkSeed comprueba que la llave pública publicada sale de su semilla pública.
func checkSeed(failed *failures, name, seedHex, publicKey string) {
	seed, err := hex.DecodeString(seedHex)
	if err != nil || len(seed) != ed25519.SeedSize {
		failed.add("llave %s: semilla inválida", name)
		return
	}
	derived := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	public, canonical := decodeBase64URL(publicKey)
	if !canonical || string(derived) != string(public) {
		failed.add("llave %s: la pública no corresponde a su semilla", name)
	}
}

// checkContent compara la licencia verificada con el contenido publicado en los archivos de prueba.
func checkContent(failed *failures, item licenseCase, license *License) {
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
			CashRegisters   int64 `json:"cash_registers"`
			OrderTakers     int64 `json:"order_takers"`
			KitchenDisplays int64 `json:"kitchen_displays"`
		} `json:"limits"`
		Features map[string]bool `json:"features"`
	}
	if err := json.Unmarshal(item.Content, &expected); err != nil {
		failed.add("licencia %s: contenido publicado ilegible", item.Name)
		return
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
		failed.add("licencia %s: el contenido verificado no es el publicado (%s)", item.Name, license.describe())
	}
}

func equalText(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
