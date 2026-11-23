package licensing

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Llaves de confianza por entorno (§1.2 de la especificación, contrato §3 y L1-D18).
//
// El entorno se fija al compilar, con la etiqueta adelpos_production:
//
//   - trust_development.go (sin la etiqueta) embebe llaves-test.json: confía solo en el entorno «test».
//   - trust_production.go (con la etiqueta) embebe llaves-production.json, que todavía no existe: una
//     compilación de producción sin llaves de producción no compila.
//
// Los archivos de llaves viven en config/licencias/ y scripts/sync-metadata.mjs los copia a esta carpeta
// (sync-check lo vigila), porque go:embed no puede leer fuera del módulo. El procedimiento para agregar una
// llave nueva está en docs/licencias/ROTAR_LLAVES.md.

// TrustedKey es una llave pública de firma en la que confía esta versión de AD-EL POS.
type TrustedKey struct {
	Environment string
	PublicKey   ed25519.PublicKey
}

// TrustSet son las llaves de confianza de esta versión, todas de un mismo entorno de firma (contrato §3):
// una versión de desarrollo solo confía en las de prueba y una de producción solo en las de producción.
type TrustSet struct {
	Environment string
	Keys        map[string]TrustedKey
}

// keyFile es el formato de la exportación del servidor de licencias (`bin/signing-key.php --action=export`,
// §3 de la guía) y de config/licencias/llaves-*.json.
type keyFile struct {
	Environment string          `json:"environment"`
	Keys        []keyFileRecord `json:"keys"`
}

type keyFileRecord struct {
	Kid         string `json:"kid"`
	Environment string `json:"environment"`
	PublicKey   string `json:"public_key"`
}

// CompiledEnvironment es el entorno de firma en que confía esta compilación: «test» o «production».
func CompiledEnvironment() string {
	return compiledEnvironment
}

// CompiledTrust son las llaves embebidas al compilar, del entorno de esta versión.
func CompiledTrust() (TrustSet, error) {
	return parseKeyFile(compiledKeys, compiledEnvironment)
}

// parseKeyFile lee un archivo de llaves y exige que todas sean del entorno indicado, con `kid` distintos y
// llaves Ed25519 de 32 bytes en base64url canónico. Un archivo vacío o sin llaves es un error: una versión
// que no confía en ninguna llave no podría activarse nunca.
func parseKeyFile(raw []byte, environment string) (TrustSet, error) {
	var file keyFile
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return TrustSet{}, errors.New("el archivo de llaves de licencias no es válido")
	}
	if file.Environment != environment {
		return TrustSet{}, fmt.Errorf("el archivo de llaves es del entorno %q y esta versión confía en %q", file.Environment, environment)
	}
	if len(file.Keys) == 0 {
		return TrustSet{}, errors.New("el archivo de llaves de licencias no tiene ninguna llave")
	}
	set := TrustSet{Environment: environment, Keys: make(map[string]TrustedKey, len(file.Keys))}
	for _, record := range file.Keys {
		if err := addTrustedKey(&set, record); err != nil {
			return TrustSet{}, err
		}
	}
	return set, nil
}

// addTrustedKey agrega una llave del entorno del conjunto. Un `kid` repetido con otra llave es un error.
func addTrustedKey(set *TrustSet, record keyFileRecord) error {
	if strings.TrimSpace(record.Kid) == "" || record.Kid != strings.TrimSpace(record.Kid) {
		return errors.New("una llave de licencias no tiene kid")
	}
	if record.Environment != set.Environment {
		return fmt.Errorf("la llave %q no es del entorno %q", record.Kid, set.Environment)
	}
	public, canonical := decodeBase64URL(record.PublicKey)
	if !canonical || len(public) != ed25519.PublicKeySize {
		return fmt.Errorf("la llave %q no es una llave Ed25519 en base64url", record.Kid)
	}
	if previous, exists := set.Keys[record.Kid]; exists {
		if !previous.PublicKey.Equal(ed25519.PublicKey(public)) {
			return fmt.Errorf("el kid %q aparece con dos llaves distintas", record.Kid)
		}
		return nil
	}
	set.Keys[record.Kid] = TrustedKey{Environment: record.Environment, PublicKey: ed25519.PublicKey(public)}
	return nil
}

// WithTestKeys agrega las llaves de firma de prueba de un archivo `vectors.json` del contrato (sección
// `signing_keys`), solo las del entorno de este conjunto. Es para el Go temporal de las pruebas
// (ADEL_TEST_LICENSE_TRUST, solo con test_mode; L1-D18) y nunca está disponible en una compilación de
// producción: así ninguna versión de producción confía en las llaves de los archivos de prueba.
func (set TrustSet) WithTestKeys(filename string) (TrustSet, error) {
	if !testHooksAllowed {
		return TrustSet{}, errors.New("una versión de producción no confía en llaves de prueba")
	}
	file, err := os.Open(filename)
	if err != nil {
		return TrustSet{}, errors.New("no se pudo abrir el archivo de llaves de prueba")
	}
	defer file.Close()
	var fixtures struct {
		SigningKeys []keyFileRecord `json:"signing_keys"`
	}
	if err := json.NewDecoder(io.LimitReader(file, 4<<20)).Decode(&fixtures); err != nil || len(fixtures.SigningKeys) == 0 {
		return TrustSet{}, errors.New("el archivo de llaves de prueba no tiene signing_keys")
	}
	combined := TrustSet{Environment: set.Environment, Keys: make(map[string]TrustedKey, len(set.Keys)+len(fixtures.SigningKeys))}
	for kid, key := range set.Keys {
		combined.Keys[kid] = key
	}
	for _, record := range fixtures.SigningKeys {
		// Las llaves de otro entorno (la de producción de los casos negativos) no se agregan.
		if record.Environment != set.Environment {
			continue
		}
		if err := addTrustedKey(&combined, record); err != nil {
			return TrustSet{}, err
		}
	}
	return combined, nil
}
