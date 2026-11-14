//go:build !windows

package standalone

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// SystemDataDir es la carpeta de datos del servicio (etapa 9G, decisión D3): en macOS
// /Library/Application Support/<producto>, de la cuenta que instaló y con permisos 0700. No sale de
// ninguna variable: es también la carpeta que el modo de pruebas rechaza (testmode_unix.go).
func SystemDataDir(productName string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join("/Library", "Application Support", productName)
	}
	return filepath.Join("/var", "lib", productName)
}

// newPlatformLayout completa las rutas que no se indicaron. Carpeta de datos: la indicada (--data-dir),
// ADEL_DATA_DIR (pruebas) o la del servicio. PostgreSQL: el indicado (--pg-bin), el postgres_bin de la
// configuración existente, ADEL_PG_BIN o el de Homebrew, en ese orden, como el respaldo de macOS
// (internal/maintenance). PostgreSQL todavía no viaja dentro del paquete de macOS: incluirlo exige
// descargar binarios y queda para la distribución de la etapa 10.
func newPlatformLayout(dataDir, pgBin, productName string) (Layout, error) {
	if dataDir == "" {
		dataDir = os.Getenv("ADEL_DATA_DIR")
	}
	if dataDir == "" {
		dataDir = SystemDataDir(productName)
	}
	dataDir = canonicalPath(dataDir)
	if pgBin == "" {
		pgBin = configuredPostgresBin(filepath.Join(dataDir, "server.json"))
	}
	if pgBin == "" {
		pgBin = os.Getenv("ADEL_PG_BIN")
	}
	if pgBin == "" {
		pgBin = homebrewPostgresBin()
	}
	// La carpeta de PostgreSQL se conserva sin resolver enlaces: el enlace «opt» de Homebrew sigue
	// apuntando a la versión vigente después de actualizarla.
	absoluteBin, err := filepath.Abs(pgBin)
	if err != nil {
		absoluteBin = filepath.Clean(pgBin)
	}
	return Layout{DataDir: dataDir, PGBin: absoluteBin, Port: ClusterPort}, nil
}

// configuredPostgresBin lee solo postgres_bin de una configuración existente (sin validarla: la
// validación completa la hace config.Load al preparar). Sin archivo o sin el campo, vacío.
func configuredPostgresBin(filename string) string {
	file, err := os.Open(filename)
	if err != nil {
		return ""
	}
	defer file.Close()
	var partial struct {
		PostgresBin string `json:"postgres_bin"`
	}
	if err := json.NewDecoder(io.LimitReader(file, 65537)).Decode(&partial); err != nil {
		return ""
	}
	if !filepath.IsAbs(partial.PostgresBin) {
		return ""
	}
	return partial.PostgresBin
}

// homebrewPostgresBin es la carpeta estable de PostgreSQL 18 de Homebrew (el enlace «opt» sobrevive a
// las actualizaciones de la versión menor).
func homebrewPostgresBin() string {
	if runtime.GOARCH == "arm64" {
		return "/opt/homebrew/opt/postgresql@18/bin"
	}
	return "/usr/local/opt/postgresql@18/bin"
}

// tool es la ruta de un programa de PostgreSQL.
func (layout Layout) tool(name string) string {
	return filepath.Join(layout.PGBin, name)
}
