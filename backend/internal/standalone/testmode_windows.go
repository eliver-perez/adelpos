//go:build windows

package standalone

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// RealDataDir es la carpeta de datos real del usuario: <carpeta conocida LocalAppData>\<producto>. Se
// obtiene por la API de carpetas conocidas de Windows y no por la variable LOCALAPPDATA, que la prueba de
// humo cambia a propósito (etapa 9F, decisión D1).
func RealDataDir(productName string) (string, error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, windows.KF_FLAG_DEFAULT)
	if err != nil || base == "" {
		return "", errors.New("no se pudo consultar la carpeta de datos local del usuario")
	}
	return canonicalPath(filepath.Join(base, productName)), nil
}

// testModeHint explica cómo abrir la aplicación con otra carpeta de datos en Windows.
const testModeHint = "abre la aplicación con otra carpeta de datos (LOCALAPPDATA temporal)"
