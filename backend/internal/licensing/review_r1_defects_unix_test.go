//go:build !windows

package licensing

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Hallazgo R1-1 · Accept reemplaza un estado.json que no se pudo leer. Load lo deja intacto y sigue en
// memoria con un estado vacío (Decisión propuesta 7 de la Fase A), pero Accept vuelve a preparar la carpeta y
// escribe ese estado vacío encima: se pierden la revisión más alta aceptada (§10.4), la última hora confiable y
// la huella guardada. Con eso, una revisión anterior a la aceptada se acepta y queda guardada.
//
// En macOS el servicio corre con la cuenta del usuario (no como root), así que basta un estado.json que esa
// cuenta no puede leer, por ejemplo uno que dejó un proceso con sudo; en Windows, un antivirus que lo tiene
// abierto. La consulta automática de la Fase B llama a Accept al arrancar.
func TestReviewR1DefectAcceptOverwritesAnUnreadableState(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root lee cualquier archivo")
	}
	h := newHarness(t)
	manager := h.start()
	h.accept(manager, "subscription-monthly")
	h.accept(manager, "closing-deactivated")
	// Una hora confiable posterior al issued_at (como la del reloj de cada minuto).
	trusted := time.Date(2027, 1, 10, 9, 0, 0, 0, time.UTC)
	manager.ObserveTime(trusted, TimeFromServer)

	filename := filepath.Join(h.directory, stateFileName)
	if err := os.Chmod(filename, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(filename, 0600)
	})
	installation := h.fixtures.installations["installation-fixture-a"]
	blocked := NewManager(Options{
		Directory:    h.directory,
		Installation: &installation,
		Trust:        h.fixtures.testTrust(),
		Now:          h.clock.now,
		MachineID: func() (string, error) {
			return h.machine, nil
		},
	})
	if err := blocked.Load(t.Context()); err == nil {
		t.Fatal("un estado que no se puede leer es un error para la bitácora")
	}

	// La revisión 1 (activa) es anterior a la 2 (el cierre `deactivated`) ya aceptada.
	status, err := blocked.Accept(h.jws("subscription-monthly"))
	if err == nil {
		t.Errorf("§10.4: sin poder leer el historial no se acepta una revisión (se aceptó la %d, %s)", status.Revision, status.State)
	}
	if err := os.Chmod(filename, 0600); err != nil {
		t.Fatal(err)
	}
	state := h.readState()
	if record := state.Accepted["11111111-1111-4111-8111-111111111111"]; record.Revision != 2 {
		t.Errorf("la revisión más alta aceptada se perdió: quedó la %d", record.Revision)
	}
	if state.TrustedTime == nil || state.TrustedTime.Before(trusted) {
		t.Errorf("la última hora confiable bajó: %v (era %v)", state.TrustedTime, trusted)
	}
	if after := h.start().Status(); after.State == StateActive {
		t.Errorf("después de reiniciar, la licencia liberada vuelve a estar vigente (revisión %d)", after.Revision)
	}
}
