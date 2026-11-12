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
