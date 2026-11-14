//go:build windows

package standalone

import (
	"errors"
	"os"
	"path/filepath"
)

// newPlatformLayout completa las rutas que no se indicaron: datos en %LOCALAPPDATA%\<producto> y
// PostgreSQL en la carpeta «postgresql\bin» junto al ejecutable del servidor.
func newPlatformLayout(dataDir, pgBin, productName string) (Layout, error) {
	if dataDir == "" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			cache, err := os.UserCacheDir()
			if err != nil {
				return Layout{}, errors.New("no se encontró la carpeta de datos local del usuario")
			}
			base = cache
		}
		dataDir = filepath.Join(base, productName)
	}
	if pgBin == "" {
		executable, err := os.Executable()
		if err != nil {
			return Layout{}, errors.New("no se encontró la carpeta del programa")
		}
		pgBin = filepath.Join(filepath.Dir(executable), "postgresql", "bin")
	}
	// Forma larga y absoluta: el marcador del clúster y las comparaciones de procesos no dependen de cómo
	// llegó la ruta (corta, relativa o con otras mayúsculas).
	return Layout{DataDir: canonicalPath(dataDir), PGBin: canonicalPath(pgBin), Port: ClusterPort}, nil
}

// tool es la ruta de un programa de PostgreSQL. PostgreSQL deduce su carpeta share de la ruta con que se
// ejecuta y, al crear el clúster, initdb escribe esa ruta dentro de una sentencia SQL en la codificación
// de Windows: con acentos (un usuario «José» instala en C:\Users\José\…) la base UTF-8 la rechaza y el
// clúster no se crea. Por eso los programas se ejecutan por la ruta corta de Windows (8.3, siempre en
// ASCII) cuando la carpeta tiene caracteres fuera de ASCII. La carpeta de datos sí admite acentos.
func (layout Layout) tool(name string) string {
	return filepath.Join(asciiPath(layout.PGBin), name+".exe")
}
