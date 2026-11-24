package licensing

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
)

// Huella del equipo (§1.1 de la especificación, L1-D10). Junto con la licencia aceptada se guarda una
// huella de la computadora; si la carpeta de datos aparece en otra (huella distinta), la licencia guardada se
// ignora y el estado es «Sin activar con datos», con Copied.
//
//   - macOS: IOPlatformUUID, leído con /usr/sbin/ioreg (machine_darwin.go).
//   - Windows: HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid, en la vista de 64 bits del registro
//     (machine_windows.go).
//   - Otros sistemas (solo desarrollo): /etc/machine-id (machine_other.go).
//
// Nunca se guarda ni se registra el identificador: solo SHA-256("ADELPOS-EQUIPO-V1\n" + identificador).

const machinePrefix = "ADELPOS-EQUIPO-V1\n"

// machineAttempts son los intentos de lectura antes de fallar cerrado (L1-D10).
const machineAttempts = 3

var (
	// Un UUID en cualquier caja (IOPlatformUUID va en mayúsculas y MachineGuid en minúsculas).
	machineUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	// /etc/machine-id: 32 dígitos hexadecimales.
	machineHexPattern = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
	// El renglón de ioreg: «"IOPlatformUUID" = "XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX"».
	ioregUUIDPattern = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]*)"`)
)

// errMachineID es el error genérico de lectura: nunca incluye lo que se leyó.
var errMachineID = errors.New("no se pudo leer el identificador de esta computadora")

// MachineFingerprint es la huella que se guarda: el SHA-256, en hexadecimal, del identificador normalizado
// con su prefijo.
func MachineFingerprint(machineID string) string {
	sum := sha256.Sum256([]byte(machinePrefix + machineID))
	return hex.EncodeToString(sum[:])
}

// normalizeMachineID acepta un UUID o 32 dígitos hexadecimales y lo deja en minúsculas, sin espacios.
func normalizeMachineID(text string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(text))
	if !machineUUIDPattern.MatchString(value) && !machineHexPattern.MatchString(value) {
		return "", errMachineID
	}
	return value, nil
}

// parseIOPlatformUUID saca el IOPlatformUUID de la salida de `ioreg -rd1 -c IOPlatformExpertDevice`.
func parseIOPlatformUUID(output []byte) (string, error) {
	matches := ioregUUIDPattern.FindAllSubmatch(output, -1)
	if len(matches) != 1 {
		return "", errMachineID
	}
	return normalizeMachineID(string(matches[0][1]))
}

// readFingerprint lee la huella del equipo con hasta tres intentos. Si ninguno funciona, devuelve un error
// y quien llama falla cerrado: trata el equipo como distinto (L1-D10).
func readFingerprint(read func() (string, error), delay time.Duration) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= machineAttempts; attempt++ {
		raw, err := read()
		if err == nil {
			normalized, normalizeErr := normalizeMachineID(raw)
			if normalizeErr == nil {
				return MachineFingerprint(normalized), nil
			}
			err = normalizeErr
		}
		lastErr = err
		if attempt < machineAttempts && delay > 0 {
			time.Sleep(delay)
		}
	}
	return "", lastErr
}
