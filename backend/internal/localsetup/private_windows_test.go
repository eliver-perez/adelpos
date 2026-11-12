//go:build windows

package localsetup

import (
	"testing"

	"local/ad-el-pos/server/internal/fileacl"
)

// requirePrivateKey comprueba en Windows la ACL de la clave privada (etapa 9E): los bits 0600 no protegen,
// así que solo la cuenta actual, SYSTEM y Administradores deben tener acceso.
func requirePrivateKey(t *testing.T, filename string) {
	t.Helper()
	if err := fileacl.RequirePrivate(filename); err != nil {
		t.Fatal("La clave privada no tiene permisos privados: " + err.Error())
	}
}
