package localsetup

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Etapa 9G, fase B: la identidad del servidor se crea una vez, se vuelve a cargar igual y nunca se
// reemplaza sola.
func TestServerIdentityIsCreatedOnceAndReloaded(t *testing.T) {
	directory := filepath.Join(t.TempDir(), IdentityFolder)
	first, err := EnsureIdentity(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !serverIDPattern.MatchString(first.ServerID) {
		t.Fatalf("identificador inválido: %q", first.ServerID)
	}
	leaf := first.Certificate.Leaf
	key, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() {
		t.Fatal("el certificado debe ser ECDSA P-256")
	}
	if leaf.NotAfter.Before(time.Now().AddDate(25, 0, 0)) {
		t.Fatalf("el certificado debe ser de larga vigencia: %s", leaf.NotAfter)
	}
	if first.Fingerprint != sha256.Sum256(leaf.RawSubjectPublicKeyInfo) || len(first.FingerprintHex()) != 64 {
		t.Fatal("la huella debe ser el SHA-256 del SPKI")
	}
	second, err := EnsureIdentity(directory)
	if err != nil {
		t.Fatal(err)
	}
	if second.ServerID != first.ServerID || second.Fingerprint != first.Fingerprint {
		t.Fatal("la identidad cambió al volver a cargarla")
	}
	requirePrivateFolder(t, directory)
	requirePrivateKey(t, filepath.Join(directory, privateKeyFile))
	other, err := EnsureIdentity(filepath.Join(t.TempDir(), IdentityFolder))
	if err != nil {
		t.Fatal(err)
	}
	if other.ServerID == first.ServerID || other.Fingerprint == first.Fingerprint {
		t.Fatal("dos servidores no pueden compartir identidad")
	}
}

func TestDamagedOrIncompleteIdentityIsNeverReplaced(t *testing.T) {
	directory := filepath.Join(t.TempDir(), IdentityFolder)
	if _, err := EnsureIdentity(directory); err != nil {
		t.Fatal(err)
	}
	keyBefore, err := os.ReadFile(filepath.Join(directory, privateKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	// Sin el certificado: no se genera otra identidad ni se toca la llave.
	if err := os.Remove(filepath.Join(directory, certificateFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureIdentity(directory); err == nil {
		t.Fatal("una identidad incompleta no debe completarse sola")
	}
	keyAfter, err := os.ReadFile(filepath.Join(directory, privateKeyFile))
	if err != nil || string(keyAfter) != string(keyBefore) {
		t.Fatal("la llave privada se modificó")
	}
	if _, err := os.Stat(filepath.Join(directory, certificateFile)); err == nil {
		t.Fatal("se creó otro certificado")
	}
	// Un registro ilegible tampoco se reemplaza.
	broken := filepath.Join(t.TempDir(), IdentityFolder)
	if _, err := EnsureIdentity(broken); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, identityFile), []byte(`{"server_id":"no-es-un-uuid"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureIdentity(broken); err == nil {
		t.Fatal("un identificador inválido debe rechazarse")
	}
	// Una llave que no corresponde al certificado se rechaza.
	mixed := filepath.Join(t.TempDir(), IdentityFolder)
	if _, err := EnsureIdentity(mixed); err != nil {
		t.Fatal(err)
	}
	foreign, err := os.ReadFile(filepath.Join(broken, privateKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mixed, privateKeyFile), foreign, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIdentity(mixed); err == nil {
		t.Fatal("una llave ajena debe rechazarse")
	}
}
