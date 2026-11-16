package legacypkg

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func listDirectory(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// El paquete se escribe en un temporal privado junto al destino y solo al terminar recibe su nombre; nunca reemplaza
// un archivo existente.
func TestCreateOutputNeverOverwrites(t *testing.T) {
	directory := t.TempDir()
	final := filepath.Join(directory, "migracion.adelmig")
	output, err := CreateOutput(final)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := output.WriteString("contenido"); err != nil {
		t.Fatal(err)
	}
	names := listDirectory(t, directory)
	if len(names) != 1 || !strings.HasPrefix(names[0], ".migracion.adelmig.") || !strings.HasSuffix(names[0], ".partial") {
		t.Fatalf("temporal: %v", names)
	}
	if !isWindows() {
		info, err := os.Stat(filepath.Join(directory, names[0]))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("permisos del temporal: %v %v", info.Mode().Perm(), err)
		}
	}
	if err := output.Commit(); err != nil {
		t.Fatal(err)
	}
	if names := listDirectory(t, directory); len(names) != 1 || names[0] != "migracion.adelmig" {
		t.Fatalf("después de terminar: %v", names)
	}
	if _, err := CreateOutput(final); !errors.Is(err, ErrOutputExists) {
		t.Fatalf("se aceptó un destino existente: %v", err)
	}
	if _, err := CreateOutput(filepath.Join(directory, "paquete.zip")); !errors.Is(err, ErrOutputExtension) {
		t.Fatal("se aceptó otra terminación")
	}
}

// Si el destino aparece mientras se escribe, Commit no lo reemplaza y borra el temporal.
func TestCommitDoesNotReplaceAFileThatAppeared(t *testing.T) {
	directory := t.TempDir()
	final := filepath.Join(directory, "migracion.ADELMIG")
	output, err := CreateOutput(final)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(final, []byte("del propietario"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := output.Commit(); !errors.Is(err, ErrOutputExists) {
		t.Fatalf("Commit: %v", err)
	}
	content, err := os.ReadFile(final)
	if err != nil || string(content) != "del propietario" {
		t.Fatal("se reemplazó el archivo que apareció")
	}
	if names := listDirectory(t, directory); len(names) != 1 {
		t.Fatalf("quedó el temporal: %v", names)
	}
}

func TestAbortRemovesTheTemporaryFile(t *testing.T) {
	directory := t.TempDir()
	output, err := CreateOutput(filepath.Join(directory, "migracion.adelmig"))
	if err != nil {
		t.Fatal(err)
	}
	output.Abort()
	output.Abort()
	if names := listDirectory(t, directory); len(names) != 0 {
		t.Fatalf("quedó: %v", names)
	}
	if err := output.Commit(); err == nil {
		t.Fatal("Commit después de Abort")
	}
}

// El temporal de un paquete que quedó a medias (proceso terminado a la fuerza) se puede borrar después, y solo si su
// nombre es exactamente el de un temporal de CreateOutput y es un archivo regular.
func TestRemoveStalePartial(t *testing.T) {
	directory := t.TempDir()
	output, err := CreateOutput(filepath.Join(directory, "migracion.adelmig"))
	if err != nil {
		t.Fatal(err)
	}
	temporary := output.TemporaryPath()
	_ = output.File.Close()
	removed, err := RemoveStalePartial(temporary)
	if err != nil || !removed {
		t.Fatalf("no se borró el temporal: %v %v", removed, err)
	}
	if _, err := os.Stat(temporary); !os.IsNotExist(err) {
		t.Fatal("el temporal sigue ahí")
	}
	other := filepath.Join(directory, "notas.txt")
	if err := os.WriteFile(other, []byte("del propietario"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, ".migracion.adelmig.0123456789ab.partial")
	if err := os.Symlink(other, link); err == nil {
		if removed, _ := RemoveStalePartial(link); removed {
			t.Fatal("se borró un enlace")
		}
	}
	for _, path := range []string{other, "relativa/.x.adelmig.0123456789ab.partial", filepath.Join(directory, "x.adelmig")} {
		if removed, _ := RemoveStalePartial(path); removed {
			t.Fatalf("se borró %s", path)
		}
	}
	if content, err := os.ReadFile(other); err != nil || string(content) != "del propietario" {
		t.Fatal("se tocó un archivo ajeno")
	}
	if removed, err := RemoveStalePartial(temporary); removed || err != nil {
		t.Fatal("borrar dos veces")
	}
}
