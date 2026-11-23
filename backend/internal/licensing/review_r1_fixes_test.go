package licensing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Correcciones de la revisión R1 (decisiones L1-D27 a L1-D29), además de las pruebas del revisor.

// R1-2: el registro de la huella vive junto a la llave, firmado con ella. Si falta, está alterado o es de otra
// instalación, la instalación se trata como copiada aunque estado.json esté intacto, y nunca se registra otra huella.
func TestMachineRecordFailsClosed(t *testing.T) {
	identityRecord := func(h *harness) string {
		return filepath.Join(filepath.Dir(h.directory), MachineRecordFile)
	}
	cases := map[string]func(h *harness){
		"sin registro": func(h *harness) {
			if err := os.Remove(identityRecord(h)); err != nil {
				h.t.Fatal(err)
			}
		},
		"registro alterado": func(h *harness) {
			raw := mustRead(h.t, identityRecord(h))
			var record map[string]any
			if err := json.Unmarshal(raw, &record); err != nil {
				h.t.Fatal(err)
			}
			normalized, _ := normalizeMachineID(machineOther)
			record["machine_fingerprint"] = MachineFingerprint(normalized)
			edited, _ := json.Marshal(record)
			if err := os.WriteFile(identityRecord(h), edited, 0600); err != nil {
				h.t.Fatal(err)
			}
		},
		"registro de otra instalación": func(h *harness) {
			writeTestMachineRecord(h.t, h.fixtures, filepath.Dir(h.directory), "installation-fixture-b", machineOriginal)
		},
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.database.setOperations(true)
			h.accept(h.start(), "subscription-monthly")
			tamper(h)
			before, _ := os.ReadFile(identityRecord(h))
			manager := h.start()
			status := manager.Status()
			if status.HasLicense() || !status.Copied || status.State != StateNotActivatedWithData {
				t.Fatalf("falla cerrada: %+v", status)
			}
			if _, err := manager.Accept(h.jws("subscription-monthly")); !errors.Is(err, ErrCopiedInstallation) {
				t.Fatalf("no se acepta nada: %v", err)
			}
			after, _ := os.ReadFile(identityRecord(h))
			if string(after) != string(before) {
				t.Fatal("el registro nunca se vuelve a crear solo")
			}
		})
	}
}

// start2 arranca un gestor sin exigir que Load termine sin errores (estado ilegible).
func (h *harness) start2() *Manager {
	h.t.Helper()
	installation := h.fixtures.installations["installation-fixture-a"]
	manager := NewManager(Options{
		Directory:    h.directory,
		Installation: &installation,
		Trust:        h.fixtures.testTrust(),
		Now:          h.clock.now,
		MachineID: func() (string, error) {
			return h.machine, nil
		},
		Database: h.database,
	})
	_ = manager.Load(context.Background())
	return manager
}

// R1-3: la revisión mínima aceptable es el máximo de las dos copias (estado.json y la base). Un respaldo viejo de la
// base no la baja; sin poder escribir la copia de la base, no se acepta nada; sin poder leerla, tampoco.
func TestRevisionFloorUsesBothCopies(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	h.accept(manager, "subscription-monthly")
	h.accept(manager, "closing-deactivated")
	if record := h.database.revisions[closedLicenseID]; record.Revision != 2 {
		t.Fatalf("la copia de la base sube al aceptar: %+v", record)
	}

	// Un respaldo viejo de la base (revisión 1): manda estado.json.
	h.database.revisions[closedLicenseID] = AcceptedRevision{Revision: 1, SHA256: h.fixtures.license(t, "subscription-monthly").SHA256}
	restarted := h.start()
	if status := restarted.Status(); status.State != StateRevoked || status.Revision != 2 {
		t.Fatalf("con la base vieja sigue el cierre: %+v", status)
	}
	if _, err := restarted.Accept(h.jws("subscription-monthly")); codeOf(err) != ErrStaleRevision {
		t.Fatalf("la revisión 1 sigue rechazada: %v", err)
	}

	// Sin poder escribir la copia de la base, la licencia no se acepta y estado.json no cambia.
	other := newHarness(t)
	failing := other.start()
	other.database.revisionFail = errors.New("sin base")
	if _, err := failing.Accept(other.jws("subscription-monthly")); !errors.Is(err, ErrStateNotSaved) {
		t.Fatalf("sin la copia de la base no se acepta: %v", err)
	}
	if _, err := os.Stat(filepath.Join(other.directory, licenseFileName)); !os.IsNotExist(err) {
		t.Fatal("no se escribió nada")
	}
	// Sin poder leerla al arrancar, Accept la vuelve a leer y, si sigue sin poder, no acepta.
	unread := other.start2()
	if _, err := unread.Accept(other.jws("subscription-monthly")); !errors.Is(err, ErrStateNotSaved) {
		t.Fatalf("sin leer la copia de la base no se acepta: %v", err)
	}
	other.database.revisionFail = nil
	if status, err := unread.Accept(other.jws("subscription-monthly")); err != nil || status.State != StateActive {
		t.Fatalf("con la base de vuelta se acepta: %+v %v", status, err)
	}
}
