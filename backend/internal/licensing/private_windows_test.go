//go:build windows

package licensing

import (
	"path/filepath"
	"testing"

	"local/ad-el-pos/server/internal/fileacl"
)

// requirePrivateStore comprueba en Windows la ACL privada de identity/licencia y de sus archivos (los bits
// 0700 y 0600 no protegen en Windows).
func requirePrivateStore(t *testing.T, directory string) {
	t.Helper()
	for _, path := range []string{directory, filepath.Join(directory, stateFileName), filepath.Join(directory, licenseFileName)} {
		if err := fileacl.RequirePrivate(path); err != nil {
			t.Fatalf("%s no es privado: %v", path, err)
		}
	}
}
