package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
)

// Revisión independiente R1 de la etapa L1: las llaves de vectors.json (semillas públicas) solo valen con
// test_mode. Se comprueba con el resultado de la verificación, no con la bitácora: sin test_mode, una licencia
// de los archivos de prueba se rechaza en el paso del kid (unknown_kid); con test_mode, el kid ya es de
// confianza y el rechazo llega después, en el destinatario (other_installation), porque la licencia es de otra
// instalación.
func TestReviewR1FixtureTrustOnlyWithTestMode(t *testing.T) {
	vectors := filepath.Join("..", "..", "..", "contracts", "licencias", "v1", "fixtures", "vectors.json")
	raw, err := os.ReadFile(vectors)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Licenses []struct {
			JWS string `json:"jws"`
		} `json:"licenses"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil || len(fixtures.Licenses) == 0 {
		t.Fatal("vectors.json sin licencias")
	}
	t.Setenv("ADEL_TEST_LICENSE_TRUST", vectors)
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	reasonOf := func(testMode bool) string {
		directory := filepath.Join(t.TempDir(), localsetup.IdentityFolder)
		identity, err := localsetup.EnsureIdentity(directory)
		if err != nil {
			t.Fatal(err)
		}
		manager := loadLicense(context.Background(), logger, licenseOptions{identityDir: directory, identity: &identity, testMode: testMode})
		_, err = manager.Accept(fixtures.Licenses[0].JWS)
		var rejected *licensing.VerifyError
		if !errors.As(err, &rejected) {
			t.Fatalf("se esperaba un rechazo del verificador y resultó %v", err)
		}
		return rejected.Code
	}
	if code := reasonOf(false); code != licensing.ErrUnknownKid {
		t.Fatalf("sin test_mode, las llaves de vectors.json no son de confianza: %s", code)
	}
	if code := reasonOf(true); code != licensing.ErrOtherInstallation {
		t.Fatalf("con test_mode, las llaves de vectors.json sí son de confianza: %s", code)
	}
}
