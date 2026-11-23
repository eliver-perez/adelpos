package licensing

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Las llaves embebidas en la compilación de desarrollo son exactamente las del entorno «test» que exportó el
// propietario (§1.2 de la especificación), cada una con su kid. Si se agrega una llave (ROTAR_LLAVES.md),
// esta lista se actualiza con ella.
func TestCompiledTrustIsTheOwnersTestKeyList(t *testing.T) {
	if BuildEnvironment != "development" {
		t.Skip("solo en la compilación de desarrollo")
	}
	expected := map[string]string{
		"adelpos-test-202611-1": "yxcFqz1rSQYi03-2slIpjx82xRhKkX_AtbguaV2zZ00",
	}
	trust, err := CompiledTrust()
	if err != nil {
		t.Fatal(err)
	}
	if trust.Environment != "test" || CompiledEnvironment() != "test" || len(trust.Keys) != len(expected) {
		t.Fatalf("entorno y número de llaves: %s %d", trust.Environment, len(trust.Keys))
	}
	for kid, public := range expected {
		key, known := trust.Keys[kid]
		if !known || key.Environment != "test" || encodeBase64URL(key.PublicKey) != public {
			t.Fatalf("la llave %s no es la del propietario", kid)
		}
	}
	// La copia embebida es la de config/licencias (sync-check también lo vigila).
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "licencias", "llaves-test.json"))
	if err != nil || string(source) != string(compiledKeys) {
		t.Fatal("la copia embebida no es config/licencias/llaves-test.json")
	}
	// La dirección por omisión es HTTPS.
	if !strings.HasPrefix(DefaultServerURL, "https://") {
		t.Fatal("la dirección del servidor de licencias de pruebas debe ser HTTPS")
	}
}

func TestKeyFilesAreStrict(t *testing.T) {
	zero := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	cases := map[string]string{
		"otro entorno":          `{"environment":"production","keys":[{"kid":"a","environment":"production","public_key":"` + zero + `"}]}`,
		"llave de otro entorno": `{"environment":"test","keys":[{"kid":"a","environment":"production","public_key":"` + zero + `"}]}`,
		"sin llaves":            `{"environment":"test","keys":[]}`,
		"sin kid":               `{"environment":"test","keys":[{"kid":"","environment":"test","public_key":"` + zero + `"}]}`,
		"llave corta":           `{"environment":"test","keys":[{"kid":"a","environment":"test","public_key":"AAAA"}]}`,
		"con relleno":           `{"environment":"test","keys":[{"kid":"a","environment":"test","public_key":"` + zero + `="}]}`,
		"campo desconocido":     `{"environment":"test","keys":[{"kid":"a","environment":"test","public_key":"` + zero + `","seed_hex":"00"}]}`,
		"kid repetido":          `{"environment":"test","keys":[{"kid":"a","environment":"test","public_key":"` + zero + `"},{"kid":"a","environment":"test","public_key":"` + base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", 32))) + `"}]}`,
	}
	for name, raw := range cases {
		if _, err := parseKeyFile([]byte(raw), "test"); err == nil {
			t.Fatalf("%s: se aceptó", name)
		}
	}
	valid := `{"environment":"test","keys":[{"kid":"a","environment":"test","public_key":"` + zero + `"}]}`
	if set, err := parseKeyFile([]byte(valid), "test"); err != nil || len(set.Keys) != 1 {
		t.Fatalf("un archivo válido: %v", err)
	}
}

// ADEL_TEST_LICENSE_TRUST: el Go temporal de las pruebas confía además en las llaves de prueba de
// vectors.json, solo las de su entorno (nunca la de producción de los casos negativos).
func TestTestKeysAddOnlyTheirEnvironment(t *testing.T) {
	if !testHooksAllowed {
		t.Skip("una compilación de producción no confía en llaves de prueba")
	}
	trust, err := CompiledTrust()
	if err != nil {
		t.Fatal(err)
	}
	combined, err := trust.WithTestKeys(filepath.Join(fixturesDirectory, "vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kid := range []string{"adelpos-test-202611-1", "adelpos-test-fixture-a", "adelpos-test-fixture-b"} {
		if _, known := combined.Keys[kid]; !known {
			t.Fatalf("falta la llave %s", kid)
		}
	}
	if _, known := combined.Keys["adelpos-prod-fixture-a"]; known || len(combined.Keys) != 3 {
		t.Fatal("la llave de producción de los casos negativos no se agrega")
	}
	if len(trust.Keys) != 1 {
		t.Fatal("el conjunto compilado no cambia")
	}
	if _, err := trust.WithTestKeys(filepath.Join(t.TempDir(), "no-existe.json")); err == nil {
		t.Fatal("un archivo inexistente es un error")
	}
}
