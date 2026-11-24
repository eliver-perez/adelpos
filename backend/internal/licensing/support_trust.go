package licensing

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Llaves de soporte para el desbloqueo sin internet (§6.4 de la especificación, L1-D19).
//
// Hay una por entorno de compilación (BuildEnvironment): la de desarrollo la genera `soporte-desbloqueo
// generar-llave` en ~/.adel-pos-dev/soporte/, fuera del repositorio; la de producción se genera con el paso a
// producción. Aquí solo viaja la pública: config/licencias/soporte-<entorno>.pub.json, que scripts/sync-metadata.mjs
// copia a esta carpeta y support_development.go o support_production.go embeben. La privada nunca está en el
// repositorio, en el instalador ni en la salida de una orden. Sin soporte-production.pub.json, una compilación de
// producción no compila (igual que sin llaves-production.json).

// SupportTrust son las llaves públicas de soporte en que confía esta versión, todas de un mismo entorno de
// compilación.
type SupportTrust struct {
	Environment string
	Keys        map[string]ed25519.PublicKey
}

// SupportKeyFile es el formato de config/licencias/soporte-<entorno>.pub.json (lo escribe soporte-desbloqueo).
type SupportKeyFile struct {
	Environment string             `json:"environment"`
	Keys        []SupportKeyRecord `json:"keys"`
}

// SupportKeyRecord es una llave pública de soporte con su `kid`.
type SupportKeyRecord struct {
	Kid       string `json:"kid"`
	PublicKey string `json:"public_key"`
}

// CompiledSupportTrust son las llaves de soporte embebidas al compilar.
func CompiledSupportTrust() (SupportTrust, error) {
	return ParseSupportKeys(compiledSupportKeys, BuildEnvironment)
}

// ParseSupportKeys lee un archivo de llaves públicas de soporte y exige el entorno indicado, `kid` distintos y llaves
// Ed25519 de 32 bytes en base64url canónico. Sin ninguna llave es un error: el desbloqueo de soporte no está
// disponible.
func ParseSupportKeys(raw []byte, environment string) (SupportTrust, error) {
	var file SupportKeyFile
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return SupportTrust{}, errors.New("el archivo de llaves de soporte no es válido")
	}
	if file.Environment != environment {
		return SupportTrust{}, fmt.Errorf("las llaves de soporte son del entorno %q y esta versión es de %q", file.Environment, environment)
	}
	if len(file.Keys) == 0 {
		return SupportTrust{}, errors.New("no hay ninguna llave de soporte")
	}
	trust := SupportTrust{Environment: environment, Keys: make(map[string]ed25519.PublicKey, len(file.Keys))}
	for _, record := range file.Keys {
		kid := strings.TrimSpace(record.Kid)
		if kid == "" || kid != record.Kid {
			return SupportTrust{}, errors.New("una llave de soporte no tiene kid")
		}
		public, canonical := decodeBase64URL(record.PublicKey)
		if !canonical || len(public) != ed25519.PublicKeySize {
			return SupportTrust{}, fmt.Errorf("la llave de soporte %q no es una llave Ed25519 en base64url", kid)
		}
		if _, repeated := trust.Keys[kid]; repeated {
			return SupportTrust{}, fmt.Errorf("el kid de soporte %q está repetido", kid)
		}
		trust.Keys[kid] = ed25519.PublicKey(public)
	}
	return trust, nil
}

// EncodeSupportPublicKey es la pública en base64url sin relleno, como va en el archivo de llaves.
func EncodeSupportPublicKey(public ed25519.PublicKey) string {
	return encodeBase64URL(public)
}
