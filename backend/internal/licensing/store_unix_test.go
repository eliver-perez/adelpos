//go:build !windows

package licensing

import (
	"os"
	"path/filepath"
	"testing"
)

// Un estado.json que no se puede leer (permisos) no se aparta ni se reemplaza: el gestor sigue en memoria,
// sin escribir, y al recuperar el permiso todo sigue igual.
func TestUnreadableStateIsNeverReplaced(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root lee cualquier archivo")
	}
	h := newHarness(t)
	manager := h.start()
	valid := h.accept(manager, "subscription-monthly")
	filename := filepath.Join(h.directory, stateFileName)
	before := mustRead(t, filename)
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
	if status := blocked.Status(); status.HasLicense() {
		t.Fatalf("sin poder leer el estado no hay licencia: %+v", status)
	}
	blocked.ObserveTime(h.clock.now().AddDate(1, 0, 0), TimeFromServer)
	if err := os.Chmod(filename, 0600); err != nil {
		t.Fatal(err)
	}
	if string(mustRead(t, filename)) != string(before) {
		t.Fatal("el estado no se reemplazó")
	}
	matches, _ := filepath.Glob(filepath.Join(h.directory, stateFileName+".danado-*"))
	if len(matches) != 0 {
		t.Fatal("un estado ilegible por permisos no se aparta como dañado")
	}
	if status := h.start().Status(); status.LicenseID != valid.LicenseID || status.State != StateActive {
		t.Fatalf("al recuperar el permiso sigue la licencia: %+v", status)
	}
}
