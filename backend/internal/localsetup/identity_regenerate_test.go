package localsetup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Etapa L1, fase B (L1-D11): la identidad nueva aparta la anterior completa, con su llave de licencias y su estado, y
// crea server_id, certificado y llave de licencias nuevos.
func TestRegenerateIdentitySetsTheOldOneAside(t *testing.T) {
	directory := filepath.Join(t.TempDir(), IdentityFolder)
	previous, err := EnsureIdentity(directory)
	if err != nil {
		t.Fatal(err)
	}
	previousKey, err := EnsureLicenseKey(directory, testFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, LicenseStateFolder), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, LicenseStateFolder, "estado.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 23, 16, 30, 0, 0, time.UTC)
	identity, key, aside, err := RegenerateIdentity(directory, now, testFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if identity.ServerID == previous.ServerID || identity.Fingerprint == previous.Fingerprint || key.PublicKey == previousKey.PublicKey {
		t.Fatal("la identidad nueva es otra: server_id, certificado y llave de licencias")
	}
	if filepath.Base(aside) != "identity-reemplazada-20261123-163000" {
		t.Fatalf("carpeta apartada: %s", aside)
	}
	old, err := LoadIdentity(aside)
	if err != nil || old.ServerID != previous.ServerID {
		t.Fatal("la identidad anterior queda apartada, completa")
	}
	if _, err := os.Stat(filepath.Join(aside, LicenseStateFolder, "estado.json")); err != nil {
		t.Fatal("el estado de la licencia anterior se aparta con su identidad")
	}
	if _, err := os.Stat(filepath.Join(directory, LicenseStateFolder)); !os.IsNotExist(err) {
		t.Fatal("la identidad nueva empieza sin estado de licencia")
	}
	requirePrivateFolder(t, directory)
	requirePrivateKey(t, filepath.Join(directory, LicenseKeyFile))
	// Otra regeneración en el mismo segundo no pisa la apartada.
	_, _, second, err := RegenerateIdentity(directory, now, testFingerprint)
	if err != nil || second == aside || filepath.Base(second) != "identity-reemplazada-20261123-163000-2" {
		t.Fatalf("segunda identidad apartada: %s %v", second, err)
	}
	// Una identidad que no se puede leer no se reemplaza.
	damaged := filepath.Join(t.TempDir(), IdentityFolder)
	if err := os.MkdirAll(damaged, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := RegenerateIdentity(damaged, now, testFingerprint); err == nil {
		t.Fatal("una identidad ilegible no se regenera")
	}
}
