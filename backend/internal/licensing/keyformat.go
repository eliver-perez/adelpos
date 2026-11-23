package licensing

import (
	"errors"
	"strings"
)

// Formato del código comercial (L1-D4, Duda 4): el contrato solo dice que es un texto de 1 a 256 caracteres. Mientras
// no esté en la V1.1, AD-EL POS usa el del servidor de licencias: 15 símbolos de «23456789ABCDEFGHJKMNPQRSTUVWXYZ»
// (sin 0, 1, I, L, O ni U) en grupos de 3, 4, 4 y 4, con guiones opcionales y sin distinguir mayúsculas. Se
// normaliza antes de enviarlo: mayúsculas y con guiones («K7M-9R3X-6TVP-8CHD»).
//
// El código nunca se registra (contrato §8.7 y §12): ningún error de aquí lo incluye.

const licenseKeyAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// licenseKeyGroups son los tamaños de los grupos del código.
var licenseKeyGroups = []int{3, 4, 4, 4}

// ErrLicenseKeyFormat: el código no tiene el formato (LICENSE_KEY_INVALID_FORMAT en las rutas).
var ErrLicenseKeyFormat = errors.New("el código de la licencia no tiene el formato esperado")

// NormalizeLicenseKey valida y normaliza un código comercial. Acepta minúsculas, guiones en los lugares de los
// grupos o sin guiones, y espacios alrededor.
func NormalizeLicenseKey(text string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(text))
	compact := strings.ReplaceAll(value, "-", "")
	total := 0
	for _, size := range licenseKeyGroups {
		total += size
	}
	if len(compact) != total {
		return "", ErrLicenseKeyFormat
	}
	for _, symbol := range compact {
		if !strings.ContainsRune(licenseKeyAlphabet, symbol) {
			return "", ErrLicenseKeyFormat
		}
	}
	// Con guiones, deben estar exactamente entre los grupos.
	if strings.Contains(value, "-") && value != groupLicenseKey(compact) {
		return "", ErrLicenseKeyFormat
	}
	return groupLicenseKey(compact), nil
}

// groupLicenseKey pone los guiones entre los grupos de un código de 15 símbolos.
func groupLicenseKey(compact string) string {
	parts := make([]string, 0, len(licenseKeyGroups))
	start := 0
	for _, size := range licenseKeyGroups {
		parts = append(parts, compact[start:start+size])
		start += size
	}
	return strings.Join(parts, "-")
}
