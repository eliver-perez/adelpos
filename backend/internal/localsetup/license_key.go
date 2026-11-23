package localsetup

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Llave de licencias de la instalación (etapa L1, fase A; contrato §2 y L1-D9).
//
// Es un par Ed25519 exclusivo para licencias, aparte del certificado ECDSA de la red local. Vive en
// identity/license-key.pem (PKCS#8, 0600, en la carpeta 0700 de la identidad; en Windows hereda su ACL privada)
// y no en identity.json, que se lee con DisallowUnknownFields: un campo nuevo ahí rompería la lectura de un
// servidor 0.14.0. Igual que el server_id:
//
//   - se crea **una sola vez**: en una instalación nueva, justo después de la identidad; en una instalación
//     0.14.0 (los tres archivos de la identidad, sin llave), la primera vez que arranca la 0.15.0;
//   - **nunca se regenera sola**: si falta pero ya hay estado de licencia (identity/licencia con estado.json o
//     vigente.lic), la llave se perdió y el servidor no crea otra, porque la licencia guardada es de la llave
//     perdida y el servidor de licencias vería otra instalación. Crear una identidad nueva es una decisión del
//     propietario desde la pantalla de la licencia (Fase B, L1-D11);
//   - no viaja en los respaldos (son solo la base) y la privada nunca se registra ni sale del equipo.
const (
	// LicenseKeyFile es el nombre de la llave de licencias dentro de la carpeta de la identidad.
	LicenseKeyFile = "license-key.pem"
	// LicenseStateFolder es la carpeta del estado de la licencia dentro de la carpeta de la identidad
	// (internal/licensing, store.go).
	LicenseStateFolder = "licencia"
	// licenseKeyTemporary es el prefijo de los temporales de una creación interrumpida.
	licenseKeyTemporary = ".license-key-"
)

// LicenseKey es la llave de licencias cargada: la privada (para la prueba de posesión y los .licreq) y la
// pública en base64url sin relleno, la `installation_public_key` del contrato.
type LicenseKey struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  string
}

// errLicenseKeyDamaged no dice nada de lo que se leyó.
var errLicenseKeyDamaged = errors.New("la llave de licencias de la instalación no es válida; no se reemplazará automáticamente (revisión manual necesaria)")

// EnsureLicenseKey carga la llave de licencias de la carpeta de la identidad o, si no existe y todavía no hay
// estado de licencia, la crea. La carpeta de la identidad ya debe existir (EnsureIdentity).
func EnsureLicenseKey(directory string) (LicenseKey, error) {
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		return LicenseKey{}, errors.New("no existe la carpeta de la identidad del servidor")
	}
	removeLicenseKeyLeftovers(directory)
	filename := filepath.Join(directory, LicenseKeyFile)
	if _, err := os.Lstat(filename); err == nil {
		return LoadLicenseKey(directory)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return LicenseKey{}, errLicenseKeyDamaged
	}
	if hasLicenseState(directory) {
		return LicenseKey{}, errors.New("falta la llave de licencias de esta instalación, pero ya hay una licencia guardada; no se creará otra automáticamente (revisión manual necesaria)")
	}
	if err := createLicenseKey(directory); err != nil {
		return LicenseKey{}, err
	}
	return LoadLicenseKey(directory)
}

// hasLicenseState indica si ya hay estado de licencia: identity/licencia con estado.json o vigente.lic (una
// carpeta vacía no cuenta).
func hasLicenseState(directory string) bool {
	for _, name := range []string{"estado.json", "vigente.lic"} {
		if _, err := os.Lstat(filepath.Join(directory, LicenseStateFolder, name)); err == nil {
			return true
		}
	}
	return false
}

// LoadLicenseKey lee y comprueba la llave: PEM «PRIVATE KEY» con un PKCS#8 Ed25519 y nada más.
func LoadLicenseKey(directory string) (LicenseKey, error) {
	file, err := os.Open(filepath.Join(directory, LicenseKeyFile))
	if err != nil {
		return LicenseKey{}, errLicenseKeyDamaged
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 4096))
	if err != nil {
		return LicenseKey{}, errLicenseKeyDamaged
	}
	block, rest := pem.Decode(raw)
	if block == nil || block.Type != "PRIVATE KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		return LicenseKey{}, errLicenseKeyDamaged
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return LicenseKey{}, errLicenseKeyDamaged
	}
	private, ok := parsed.(ed25519.PrivateKey)
	if !ok || len(private) != ed25519.PrivateKeySize {
		return LicenseKey{}, errLicenseKeyDamaged
	}
	public := private.Public().(ed25519.PublicKey)
	return LicenseKey{PrivateKey: private, PublicKey: base64.RawURLEncoding.EncodeToString(public)}, nil
}

// createLicenseKey genera la llave y la escribe completa en un temporal (0600, fsync) que después se enlaza con
// el nombre definitivo: el nombre definitivo solo aparece con la llave completa, y un enlace nunca reemplaza un
// archivo que ya exista.
func createLicenseKey(directory string) error {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return errors.New("no se pudo generar la llave de licencias")
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return errors.New("no se pudo generar la llave de licencias")
	}
	body := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	temporary, err := os.CreateTemp(directory, licenseKeyTemporary+"*")
	if err != nil {
		return errors.New("no se pudo guardar la llave de licencias")
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	_, writeErr := temporary.Write(body)
	chmodErr := temporary.Chmod(0600)
	syncErr := temporary.Sync()
	closeErr := temporary.Close()
	if writeErr != nil || chmodErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("no se pudo guardar la llave de licencias")
	}
	filename := filepath.Join(directory, LicenseKeyFile)
	if err := os.Link(temporaryName, filename); err != nil {
		if _, statErr := os.Lstat(filename); statErr == nil {
			// Otro proceso la creó al mismo tiempo: se usa esa, nunca se reemplaza.
			return nil
		}
		// Un sistema de archivos sin enlaces: renombrar, que tampoco deja una llave a medias.
		if renameErr := os.Rename(temporaryName, filename); renameErr != nil {
			return errors.New("no se pudo guardar la llave de licencias")
		}
	}
	syncDirectory(directory)
	return nil
}

// removeLicenseKeyLeftovers descarta los temporales de una creación interrumpida (nunca la llave definitiva).
func removeLicenseKeyLeftovers(directory string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), licenseKeyTemporary) {
			_ = os.Remove(filepath.Join(directory, entry.Name()))
		}
	}
}
