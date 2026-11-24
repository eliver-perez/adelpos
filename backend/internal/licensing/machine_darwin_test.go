//go:build darwin

package licensing

import "testing"

// En macOS la huella se lee de verdad con ioreg (solo lectura del hardware; el valor nunca se muestra) y es
// estable entre dos lecturas.
func TestMachineIdentifierReadsOnMacOS(t *testing.T) {
	first, err := readFingerprint(readMachineID, 0)
	if err != nil {
		t.Fatalf("no se pudo leer el IOPlatformUUID: %v", err)
	}
	second, err := readFingerprint(readMachineID, 0)
	if err != nil || first != second || !sha256HexPattern.MatchString(first) {
		t.Fatal("la huella del equipo es estable")
	}
}
