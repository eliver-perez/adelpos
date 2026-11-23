package licensing

import (
	"os"
	"path/filepath"
	"testing"
)

// Revisión independiente R1 de la etapa L1: hallazgos del almacén identity/licencia. Cada prueba falla hasta que
// se corrija su hallazgo (doc-R1.md del coordinador).

// Hallazgo R1-2 · Carpeta copiada a otra computadora sin estado.json (o con estado.json dañado o sin la
// huella): la huella guardada desaparece, checkMachine registra la de la computadora nueva como si fuera la
// primera vez y la licencia de vigente.lic se recupera. La copia queda Vigente con la misma identidad
// (server_id y llave), así que el servidor de licencias vería una sola instalación en dos computadoras.
func TestReviewR1DefectCopiedFolderWithoutStateIsNotDetected(t *testing.T) {
	cases := map[string]func(h *harness){
		"estado.json borrado": func(h *harness) {
			if err := os.Remove(filepath.Join(h.directory, stateFileName)); err != nil {
				h.t.Fatal(err)
			}
		},
		"estado.json dañado": func(h *harness) {
			if err := os.WriteFile(filepath.Join(h.directory, stateFileName), []byte("{"), 0600); err != nil {
				h.t.Fatal(err)
			}
		},
		"sin machine_fingerprint": func(h *harness) {
			state := h.readState()
			state.MachineFingerprint = ""
			h.writeState(state)
		},
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.database.setOperations(true)
			h.accept(h.start(), "subscription-monthly")

			// La carpeta aparece en otra computadora y se toca un solo archivo.
			h.machine = machineOther
			tamper(h)
			status := h.start().Status()
			if status.HasLicense() || !status.Copied {
				t.Fatalf("la licencia de la carpeta copiada sigue en uso en otra computadora: estado %s, Copied %v", status.State, status.Copied)
			}
		})
	}
}

// Hallazgo R1-3 · Retroceso de revisión borrando estado.json: la revisión más alta aceptada solo vive en
// estado.json. Si falta (o está dañado), vigente.lic se acepta como «primera revisión», aunque sea una revisión
// anterior a la ya aceptada. Basta copiar el .lic activo de antes de liberar la licencia y borrar estado.json
// para volver a operar con una licencia que ya se liberó (y que se puede activar en otra computadora).
func TestReviewR1DefectDeletedStateBringsBackAnOldRevision(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	h.accept(manager, "subscription-monthly")
	closed := h.accept(manager, "closing-deactivated")
	if closed.State != StateRevoked || closed.Revision != 2 {
		t.Fatalf("la licencia quedó liberada: %+v", closed)
	}

	// El usuario de la cuenta del servicio (dueña de la carpeta de datos) borra estado.json y deja en vigente.lic
	// el .lic de antes de liberar.
	if err := os.WriteFile(filepath.Join(h.directory, licenseFileName), []byte(h.jws("subscription-monthly")), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(h.directory, stateFileName)); err != nil {
		t.Fatal(err)
	}
	status := h.start().Status()
	if status.State == StateActive || status.Revision < 2 {
		t.Fatalf("§10.4: una revisión anterior a la aceptada no vuelve a valer (estado %s, revisión %d)", status.State, status.Revision)
	}
}
