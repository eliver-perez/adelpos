package licensing

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// El reloj de los procesos de prueba suma el desfase del archivo, y sin archivo no cambia la hora.
func TestOffsetClock(t *testing.T) {
	if !testHooksAllowed {
		if _, err := OffsetClock("x"); err == nil {
			t.Fatal("una compilación de producción no usa el reloj de las pruebas")
		}
		return
	}
	filename := filepath.Join(t.TempDir(), "reloj")
	clock, err := OffsetClock(filename)
	if err != nil {
		t.Fatal(err)
	}
	if difference := clock().Sub(time.Now()); difference > time.Minute || difference < -time.Minute {
		t.Fatal("sin archivo no hay desfase")
	}
	if err := os.WriteFile(filename, []byte("-36h\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if offset := readClockOffset(filename); offset != -36*time.Hour {
		t.Fatalf("desfase leído: %v", offset)
	}
	if err := os.WriteFile(filename, []byte("mañana"), 0600); err != nil {
		t.Fatal(err)
	}
	if offset := readClockOffset(filename); offset != 0 {
		t.Fatal("un desfase inválido es cero")
	}
	if _, err := OffsetClock(" "); err == nil {
		t.Fatal("sin archivo es un error")
	}
}
