//go:build windows

package licensing

import "testing"

// Prueba de lectura de MachineGuid en Windows (se ejecuta en la Fase W, en una computadora con Windows; aquí
// solo se compila con GOOS=windows).
func TestMachineIdentifierReadsOnWindows(t *testing.T) {
	first, err := readFingerprint(readMachineID, 0)
	if err != nil {
		t.Fatalf("no se pudo leer MachineGuid: %v", err)
	}
	second, err := readFingerprint(readMachineID, 0)
	if err != nil || first != second || !sha256HexPattern.MatchString(first) {
		t.Fatal("la huella del equipo es estable")
	}
}
