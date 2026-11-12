//go:build windows

package fileacl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProtectLeavesOnlyOwnerSystemAndAdministrators(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "secreto")
	if err := os.WriteFile(filename, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{directory, filename} {
		if err := Protect(path); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if err := RequirePrivate(path); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	// Un archivo creado después dentro de la carpeta protegida hereda la misma ACL privada.
	later := filepath.Join(directory, "posterior")
	if err := os.WriteFile(later, []byte("y"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RequirePrivate(later); err != nil {
		t.Fatalf("archivo posterior: %v", err)
	}
}

func TestProtectRejectsMissingPath(t *testing.T) {
	if err := Protect(filepath.Join(t.TempDir(), "no-existe")); err == nil {
		t.Fatal("se esperaba un error con una ruta inexistente")
	}
}
