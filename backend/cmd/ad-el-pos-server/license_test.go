package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
)

// Etapa L1, fase A: al arrancar, el gestor de la licencia carga (o crea una sola vez) la llave de licencias
// junto a la identidad y queda «Sin activar»; sin identidad no escribe nada, y nunca impide arrancar.
func TestLoadLicenseAtStartup(t *testing.T) {
	directory := filepath.Join(t.TempDir(), localsetup.IdentityFolder)
	identity, err := localsetup.EnsureIdentity(directory)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	manager := loadLicense(context.Background(), logger, licenseOptions{identityDir: directory, identity: &identity})
	status := manager.Status()
	if status.State != licensing.StateNotActivated || status.IdentityUnavailable || status.Environment != licensing.CompiledEnvironment() {
		t.Fatalf("una instalación nueva queda sin activar, con identidad: %+v", status)
	}
	key, err := localsetup.LoadLicenseKey(directory)
	if err != nil {
		t.Fatal("la llave de licencias se crea al arrancar")
	}
	if _, err := os.Stat(filepath.Join(directory, localsetup.LicenseStateFolder, "estado.json")); err != nil {
		t.Fatal("el estado de la licencia se guarda en identity/licencia")
	}
	// Un segundo arranque usa la misma llave.
	loadLicense(context.Background(), logger, licenseOptions{identityDir: directory, identity: &identity})
	again, err := localsetup.LoadLicenseKey(directory)
	if err != nil || again.PublicKey != key.PublicKey {
		t.Fatal("la llave de licencias no se regenera al reiniciar")
	}

	// Sin identidad: sin activar, sin llave nueva y sin estado.
	bare := filepath.Join(t.TempDir(), localsetup.IdentityFolder)
	if err := os.MkdirAll(bare, 0700); err != nil {
		t.Fatal(err)
	}
	withoutIdentity := loadLicense(context.Background(), logger, licenseOptions{identityDir: bare})
	if status := withoutIdentity.Status(); status.State != licensing.StateNotActivated || !status.IdentityUnavailable {
		t.Fatalf("sin identidad: %+v", status)
	}
	if entries, _ := os.ReadDir(bare); len(entries) != 0 {
		t.Fatal("sin identidad no se escribe nada")
	}

	// Estado de licencia sin llave: la llave se perdió; no se crea otra y el servidor sigue.
	if err := os.Remove(filepath.Join(directory, localsetup.LicenseKeyFile)); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	lost := loadLicense(context.Background(), slog.New(slog.NewJSONHandler(&logs, nil)), licenseOptions{identityDir: directory, identity: &identity})
	if status := lost.Status(); !status.IdentityUnavailable {
		t.Fatalf("sin llave no hay identidad de licencias: %+v", status)
	}
	if _, err := os.Stat(filepath.Join(directory, localsetup.LicenseKeyFile)); !os.IsNotExist(err) {
		t.Fatal("una llave perdida no se regenera")
	}
	if !strings.Contains(logs.String(), "license_key_unavailable") {
		t.Fatal("la bitácora explica la llave perdida")
	}
}

// Las ayudas de las pruebas (llaves de vectors.json y reloj con desfase) solo se aplican con test_mode.
func TestLicenseTestHooksRequireTestMode(t *testing.T) {
	directory := filepath.Join(t.TempDir(), localsetup.IdentityFolder)
	identity, err := localsetup.EnsureIdentity(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ADEL_TEST_LICENSE_TRUST", filepath.Join("..", "..", "..", "contracts", "licencias", "v1", "fixtures", "vectors.json"))
	clockFile := filepath.Join(t.TempDir(), "reloj")
	if err := os.WriteFile(clockFile, []byte("-48h"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ADEL_TEST_CLOCK_FILE", clockFile)

	var withoutTestMode bytes.Buffer
	loadLicense(context.Background(), slog.New(slog.NewJSONHandler(&withoutTestMode, nil)), licenseOptions{identityDir: directory, identity: &identity})
	if strings.Contains(withoutTestMode.String(), "license_test_") {
		t.Fatal("sin test_mode no se aplica ninguna ayuda de las pruebas")
	}

	var withTestMode bytes.Buffer
	manager := loadLicense(context.Background(), slog.New(slog.NewJSONHandler(&withTestMode, nil)), licenseOptions{identityDir: directory, identity: &identity, testMode: true})
	if !strings.Contains(withTestMode.String(), "license_test_trust_enabled") || !strings.Contains(withTestMode.String(), "license_test_clock_enabled") {
		t.Fatalf("con test_mode se aplican las ayudas: %s", withTestMode.String())
	}
	if behind := time.Since(manager.LocalClock()); behind < 47*time.Hour || behind > 49*time.Hour {
		t.Fatalf("el reloj del gestor es el reloj con desfase: %s", behind)
	}
}
