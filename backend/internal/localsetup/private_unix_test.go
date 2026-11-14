//go:build !windows

package localsetup

import (
	"os"
	"testing"
)

// requirePrivateKey comprueba los permisos 0600 de la clave privada (macOS y Linux).
func requirePrivateKey(t *testing.T, filename string) {
	t.Helper()
	permissions, _ := os.Stat(filename)
	if permissions.Mode().Perm() != 0600 {
		t.Fatal("La clave privada no tiene permisos privados")
	}
}

// requirePrivateFolder comprueba los permisos 0700 de la carpeta de la identidad (etapa 9G).
func requirePrivateFolder(t *testing.T, directory string) {
	t.Helper()
	info, err := os.Stat(directory)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("La carpeta de la identidad no es privada")
	}
}
