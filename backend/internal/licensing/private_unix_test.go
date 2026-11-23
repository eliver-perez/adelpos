//go:build !windows

package licensing

import (
	"os"
	"path/filepath"
	"testing"
)

// requirePrivateStore comprueba los permisos de identity/licencia: carpeta 0700 y archivos 0600.
func requirePrivateStore(t *testing.T, directory string) {
	t.Helper()
	info, err := os.Stat(directory)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("la carpeta del estado de la licencia no es privada (0700)")
	}
	for _, name := range []string{stateFileName, licenseFileName} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("%s no es privado (0600)", name)
		}
	}
}
