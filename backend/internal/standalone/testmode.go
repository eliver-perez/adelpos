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

// RefuseRealDataDir impide que el modo de pruebas use la carpeta de datos real del usuario: allí están el
// clúster, la configuración y los respaldos del restaurante. La comparación no distingue mayúsculas ni la
// forma corta o larga de la ruta.
func RefuseRealDataDir(layout Layout, productName string) error {
	real, err := RealDataDir(productName)
	if err != nil {
		return err
	}
	if samePath(layout.DataDir, real) {
		return errors.New("el modo de pruebas (ADEL_TEST_MODE=1) no puede usar la carpeta de datos real de AD-EL POS; " +
			"abre la aplicación con otra carpeta de datos (LOCALAPPDATA temporal)")
	}
	return nil
}
