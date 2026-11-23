package localsetup

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Identidad nueva de una instalación copiada (etapa L1, fase B; L1-D11). Solo la pide el propietario desde la pantalla
// de la licencia, con confirmación y en la computadora principal: la identidad nunca se reemplaza sola.
//
// La carpeta de la identidad completa (identity.json, el certificado y su llave, la llave de licencias y el estado de
// la licencia) se aparta a identity-reemplazada-AAAAMMDD-HHMMSS, junto a ella, para soporte, y nunca se vuelve a usar.
// Después se crean los tres archivos nuevos y la llave de licencias nueva, con la misma creación atómica de siempre.
// Si la creación falla, la carpeta anterior vuelve a su lugar y no cambia nada.

// replacedIdentityPrefix es el inicio del nombre de una identidad apartada.
const replacedIdentityPrefix = "identity-reemplazada-"

// RegenerateIdentity aparta la identidad de la carpeta y crea una nueva. Devuelve la identidad, la llave de licencias
// y la ruta de la carpeta apartada.
func RegenerateIdentity(directory string, now time.Time) (ServerIdentity, LicenseKey, string, error) {
	if _, err := LoadIdentity(directory); err != nil {
		return ServerIdentity{}, LicenseKey{}, "", errors.New("la identidad actual no se puede leer; revisión manual necesaria")
	}
	parent := filepath.Dir(directory)
	aside := filepath.Join(parent, replacedIdentityPrefix+now.UTC().Format("20060102-150405"))
	for attempt := 2; ; attempt++ {
		if _, err := os.Lstat(aside); errors.Is(err, os.ErrNotExist) {
			break
		}
		aside = filepath.Join(parent, replacedIdentityPrefix+now.UTC().Format("20060102-150405")+"-"+strconv.Itoa(attempt))
	}
	if err := os.Rename(directory, aside); err != nil {
		return ServerIdentity{}, LicenseKey{}, "", errors.New("no se pudo apartar la identidad actual")
	}
	syncDirectory(parent)
	restore := func() {
		_ = os.RemoveAll(directory)
		_ = os.Rename(aside, directory)
		syncDirectory(parent)
	}
	identity, err := EnsureIdentity(directory)
	if err != nil {
		restore()
		return ServerIdentity{}, LicenseKey{}, "", err
	}
	key, err := EnsureLicenseKey(directory)
	if err != nil {
		restore()
		return ServerIdentity{}, LicenseKey{}, "", err
	}
	return identity, key, aside, nil
}
