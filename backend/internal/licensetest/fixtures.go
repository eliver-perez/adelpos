//go:build !adelpos_production

// Package licensetest son las herramientas **solo de pruebas** de las licencias (etapa L1, Fase B): el emisor de
// licencias de prueba y el servidor de licencias simulado, firmados con las llaves públicas y deterministas de
// contracts/licencias/v1/fixtures/vectors.json (contrato §13).
//
// Nunca forma parte del servidor ni de un instalador: solo lo importan las pruebas de Go y la orden de pruebas
// cmd/license-test (que exige ADEL_TEST_MODE=1). Una compilación de producción no lo compila (etiqueta), y
// licensetest_guard_test.go comprueba que ./cmd/ad-el-pos-server no depende de él y que los guiones de los
// instaladores no compilan la orden. Nunca llama al servidor de licencias real.
package licensetest

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// DefaultKid es la llave de firma de prueba con que se firma por omisión (entorno «test»).
const DefaultKid = "adelpos-test-fixture-a"

// Fixtures son las llaves de los archivos de prueba del contrato.
type Fixtures struct {
	// SigningKeys son las llaves de firma del entorno «test», por kid.
	SigningKeys map[string]ed25519.PrivateKey
	// Installations son las dos instalaciones de prueba (installation-fixture-a y -b), con su llave privada.
	Installations map[string]FixtureInstallation
}

// FixtureInstallation es una instalación de los archivos de prueba.
type FixtureInstallation struct {
	ServerID   string
	PublicKey  string
	PrivateKey ed25519.PrivateKey
}

// LoadFixtures lee vectors.json y deriva las llaves de sus semillas públicas, comprobando cada pública.
func LoadFixtures(filename string) (Fixtures, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return Fixtures{}, errors.New("no se pudo leer el archivo de pruebas del contrato (vectors.json)")
	}
	var set struct {
		Version     string `json:"fixture_version"`
		SigningKeys []struct {
			Kid         string `json:"kid"`
			Environment string `json:"environment"`
			SeedHex     string `json:"seed_hex"`
			PublicKey   string `json:"public_key"`
		} `json:"signing_keys"`
		Installations []struct {
			Name      string `json:"name"`
			ServerID  string `json:"server_id"`
			SeedHex   string `json:"seed_hex"`
			PublicKey string `json:"public_key"`
		} `json:"installations"`
	}
	if err := json.Unmarshal(raw, &set); err != nil || set.Version != "1.0" {
		return Fixtures{}, errors.New("vectors.json no es la versión 1.0 de los archivos de prueba")
	}
	fixtures := Fixtures{SigningKeys: map[string]ed25519.PrivateKey{}, Installations: map[string]FixtureInstallation{}}
	for _, key := range set.SigningKeys {
		// Solo las del entorno «test»: la de producción de los casos negativos nunca firma aquí.
		if key.Environment != "test" {
			continue
		}
		private, err := privateFromSeed(key.SeedHex, key.PublicKey)
		if err != nil {
			return Fixtures{}, fmt.Errorf("llave %s: %w", key.Kid, err)
		}
		fixtures.SigningKeys[key.Kid] = private
	}
	for _, installation := range set.Installations {
		private, err := privateFromSeed(installation.SeedHex, installation.PublicKey)
		if err != nil {
			return Fixtures{}, fmt.Errorf("instalación %s: %w", installation.Name, err)
		}
		fixtures.Installations[installation.Name] = FixtureInstallation{
			ServerID:   installation.ServerID,
			PublicKey:  installation.PublicKey,
			PrivateKey: private,
		}
	}
	if _, ok := fixtures.SigningKeys[DefaultKid]; !ok {
		return Fixtures{}, errors.New("vectors.json no tiene la llave de firma " + DefaultKid)
	}
	return fixtures, nil
}

func privateFromSeed(seedHex, publicKey string) (ed25519.PrivateKey, error) {
	seed, err := hex.DecodeString(seedHex)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("semilla inválida")
	}
	private := ed25519.NewKeyFromSeed(seed)
	if base64.RawURLEncoding.EncodeToString(private.Public().(ed25519.PublicKey)) != publicKey {
		return nil, errors.New("la pública no corresponde a su semilla")
	}
	return private, nil
}

// encode es base64url sin relleno.
func encode(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

// SignJWS firma un contenido con una llave de prueba y el encabezado exacto del contrato (§3).
func (fixtures Fixtures) SignJWS(kid string, content []byte) (string, error) {
	private, ok := fixtures.SigningKeys[kid]
	if !ok {
		return "", errors.New("llave de prueba desconocida: " + kid)
	}
	kidJSON, _ := json.Marshal(kid)
	header := encode([]byte(`{"alg":"EdDSA","kid":` + string(kidJSON) + `,"typ":"adelpos-lic+jws"}`))
	body := encode(content)
	return header + "." + body + "." + encode(ed25519.Sign(private, []byte(header+"."+body))), nil
}
