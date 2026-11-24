package licensing

import (
	"errors"
	"testing"
)

// La lectura de la huella en macOS sale del renglón IOPlatformUUID de ioreg; en Windows, de MachineGuid.
// Aquí se prueban con salidas simuladas (la lectura real de Windows, en la Fase W).
func TestMachineIdentifierParsing(t *testing.T) {
	output := []byte(`+-o J316sAP  <class IOPlatformExpertDevice, id 0x100000234, registered, matched, active, busy 0 (125 ms), retain 39>
    {
      "IOPlatformSerialNumber" = "XXXXXXXXXX"
      "IOPlatformUUID" = "12345678-ABCD-4EF0-9123-456789ABCDEF"
      "manufacturer" = <"Apple Inc.">
    }
`)
	value, err := parseIOPlatformUUID(output)
	if err != nil || value != "12345678-abcd-4ef0-9123-456789abcdef" {
		t.Fatalf("IOPlatformUUID: %q %v", value, err)
	}
	for _, broken := range []string{"", `"IOPlatformUUID" = ""`, `"IOPlatformUUID" = "no-es-un-uuid"`, `"IOPlatformUUID" = "12345678-ABCD-4EF0-9123-456789ABCDEF" "IOPlatformUUID" = "12345678-ABCD-4EF0-9123-456789ABCDE0"`} {
		if _, err := parseIOPlatformUUID([]byte(broken)); err == nil {
			t.Fatalf("salida inválida aceptada: %q", broken)
		}
	}
	// MachineGuid (minúsculas, con espacios por si acaso) y /etc/machine-id (32 dígitos).
	if value, err := normalizeMachineID(" 0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0\n"); err != nil || value != "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0" {
		t.Fatalf("MachineGuid: %q %v", value, err)
	}
	if _, err := normalizeMachineID("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal("machine-id de 32 dígitos")
	}
	// La huella es el SHA-256 con prefijo del valor normalizado: la misma con mayúsculas o minúsculas.
	upper, _ := normalizeMachineID("12345678-ABCD-4EF0-9123-456789ABCDEF")
	lower, _ := normalizeMachineID("12345678-abcd-4ef0-9123-456789abcdef")
	if MachineFingerprint(upper) != MachineFingerprint(lower) || len(MachineFingerprint(lower)) != 64 {
		t.Fatal("la huella no depende de la caja")
	}
	if MachineFingerprint(lower) == MachineFingerprint("12345678-abcd-4ef0-9123-456789abcde0") {
		t.Fatal("dos equipos distintos tienen huellas distintas")
	}
}

func TestMachineFingerprintRetriesThreeTimes(t *testing.T) {
	attempts := 0
	fingerprint, err := readFingerprint(func() (string, error) {
		attempts++
		if attempts < 3 {
			return "", errors.New("todavía no")
		}
		return machineOriginal, nil
	}, 0)
	if err != nil || attempts != 3 || fingerprint != MachineFingerprint("aaaaaaaa-1111-2222-3333-444444444444") {
		t.Fatalf("el tercer intento basta: %d %v", attempts, err)
	}
	attempts = 0
	if _, err := readFingerprint(func() (string, error) {
		attempts++
		return "", errors.New("nunca")
	}, 0); err == nil || attempts != 3 {
		t.Fatal("después de tres intentos falla")
	}
}
