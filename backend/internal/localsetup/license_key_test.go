package localsetup

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// Etapa L1, fase A: una instalación 0.14.0 (los tres archivos de la identidad, sin llave de licencias) crea la
// llave una sola vez, sin tocar la identidad, y después siempre carga la misma.
func TestLicenseKeyIsCreatedOnceForAnExistingIdentity(t *testing.T) {
	directory := filepath.Join(t.TempDir(), IdentityFolder)
	identity, err := EnsureIdentity(directory)
	if err != nil {
		t.Fatal(err)
	}
	before := identityFiles(t, directory)

	first, err := EnsureLicenseKey(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.PrivateKey) != ed25519.PrivateKeySize {
		t.Fatal("la llave de licencias es Ed25519")
	}
	public, err := base64.RawURLEncoding.Strict().DecodeString(first.PublicKey)
	if err != nil || len(public) != ed25519.PublicKeySize || !first.PrivateKey.Public().(ed25519.PublicKey).Equal(ed25519.PublicKey(public)) {
		t.Fatal("la pública es la de la privada, en base64url sin relleno")
	}
	requirePrivateKey(t, filepath.Join(directory, LicenseKeyFile))

	second, err := EnsureLicenseKey(directory)
	if err != nil || second.PublicKey != first.PublicKey || !second.PrivateKey.Equal(first.PrivateKey) {
		t.Fatal("la llave de licencias nunca se regenera")
	}
	// La identidad (server_id y certificado ECDSA) no cambia, y se sigue cargando con la llave al lado.
	if after := identityFiles(t, directory); after != before {
		t.Fatal("crear la llave de licencias no toca la identidad")
	}
	reloaded, err := EnsureIdentity(directory)
	if err != nil || reloaded.ServerID != identity.ServerID || reloaded.Fingerprint != identity.Fingerprint {
		t.Fatal("la identidad se carga igual con la llave de licencias al lado")
	}
	// Dos instalaciones tienen llaves distintas.
	otherDirectory := filepath.Join(t.TempDir(), IdentityFolder)
	if _, err := EnsureIdentity(otherDirectory); err != nil {
		t.Fatal(err)
	}
	other, err := EnsureLicenseKey(otherDirectory)
	if err != nil || other.PublicKey == first.PublicKey {
		t.Fatal("dos instalaciones no comparten llave de licencias")
	}
}

// Un estado de licencia sin llave es una llave perdida: error, y nunca se crea otra.
func TestLicenseStateWithoutKeyIsAnError(t *testing.T) {
	for _, name := range []string{"estado.json", "vigente.lic"} {
		directory := filepath.Join(t.TempDir(), IdentityFolder)
		if _, err := EnsureIdentity(directory); err != nil {
			t.Fatal(err)
		}
		state := filepath.Join(directory, LicenseStateFolder)
		if err := os.MkdirAll(state, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(state, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := EnsureLicenseKey(directory); err == nil {
			t.Fatalf("con %s y sin llave debe fallar", name)
		}
		if _, err := os.Lstat(filepath.Join(directory, LicenseKeyFile)); !os.IsNotExist(err) {
			t.Fatalf("con %s no se crea otra llave", name)
		}
	}
	// Una carpeta de estado vacía no es estado: la llave se crea.
	directory := filepath.Join(t.TempDir(), IdentityFolder)
	if _, err := EnsureIdentity(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, LicenseStateFolder), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureLicenseKey(directory); err != nil {
		t.Fatalf("una carpeta vacía no impide crear la llave: %v", err)
	}
}

// Una llave dañada no se reemplaza, y los temporales de una creación interrumpida se descartan.
func TestDamagedLicenseKeyIsNeverReplaced(t *testing.T) {
	directory := filepath.Join(t.TempDir(), IdentityFolder)
	if _, err := EnsureIdentity(directory); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(directory, licenseKeyTemporary+"123")
	if err := os.WriteFile(leftover, []byte("a medias"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureLicenseKey(directory); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Fatal("el temporal de una creación interrumpida se descarta")
	}
	// Una llave ECDSA (la del certificado) en lugar de la Ed25519: se rechaza y no se toca.
	foreign, err := os.ReadFile(filepath.Join(directory, privateKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(directory, LicenseKeyFile)
	if err := os.WriteFile(keyFile, foreign, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureLicenseKey(directory); err == nil {
		t.Fatal("una llave que no es Ed25519 se rechaza")
	}
	if body, _ := os.ReadFile(keyFile); string(body) != string(foreign) {
		t.Fatal("la llave dañada no se reemplaza")
	}
	if err := os.WriteFile(keyFile, []byte("no es una llave"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureLicenseKey(directory); err == nil {
		t.Fatal("un archivo ilegible se rechaza")
	}
	// Sin carpeta de identidad no se crea nada.
	if _, err := EnsureLicenseKey(filepath.Join(t.TempDir(), "no-existe")); err == nil {
		t.Fatal("sin carpeta de identidad es un error")
	}
}

// identityFiles resume el contenido de los tres archivos de la identidad.
func identityFiles(t *testing.T, directory string) string {
	t.Helper()
	summary := ""
	for _, name := range []string{identityFile, certificateFile, privateKeyFile} {
		body, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		summary += name + ":" + string(body) + "\n"
	}
	return summary
}
