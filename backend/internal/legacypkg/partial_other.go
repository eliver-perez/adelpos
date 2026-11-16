//go:build !windows

package legacypkg

import (
	"os"

	"local/ad-el-pos/server/internal/fileacl"
)

// createPartial crea el temporal del paquete: 0600 desde el primer byte, sin seguir enlaces ni reemplazar nada.
func createPartial(path string) (*os.File, error) {
	return fileacl.CreatePrivate(path)
}
