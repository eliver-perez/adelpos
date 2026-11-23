//go:build !adelpos_production

package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/licensetest"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
)

const vectorsFile = "../../../contracts/licencias/v1/fixtures/vectors.json"

func lastLine(t *testing.T, output *bytes.Buffer) result {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	var value result
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &value); err != nil {
		t.Fatalf("salida JSON: %q", output.String())
	}
	return value
}

// Sin ADEL_TEST_MODE=1 la orden no hace nada.
func TestLicenseTestRequiresTestMode(t *testing.T) {
	t.Setenv("ADEL_TEST_MODE", "")
	var output bytes.Buffer
	if code := run(context.Background(), []string{"issue"}, &output, nil); code != 2 || lastLine(t, &output).Event != "error" {
		t.Fatalf("sin modo de pruebas: %d %s", code, output.String())
	}
	output.Reset()
	if code := run(context.Background(), []string{"server", "--ca-out", filepath.Join(t.TempDir(), "ca.pem")}, &output, nil); code != 2 {
		t.Fatalf("sin modo de pruebas, el servidor: %d", code)
	}
}

// issue firma un .lic para la instalación de una carpeta identity, que el gestor de esa instalación acepta, en el
// estado pedido.
func TestLicenseTestIssue(t *testing.T) {
	t.Setenv("ADEL_TEST_MODE", "1")
	t.Setenv("ADEL_TEST_LICENSE_TRUST", vectorsFile)
	identityDir := filepath.Join(t.TempDir(), localsetup.IdentityFolder)
	identity, err := localsetup.EnsureIdentity(identityDir)
	if err != nil {
		t.Fatal(err)
	}
	key, err := localsetup.EnsureLicenseKey(identityDir, func() (string, error) {
		return licensing.MachineFingerprint("dddddddd-1111-2222-3333-444444444444"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := licensetest.LoadFixtures(vectorsFile)
	if err != nil {
		t.Fatal(err)
	}
	trust := fixtures.Trust()
	manager := licensing.NewManager(licensing.Options{
		Directory:    filepath.Join(identityDir, localsetup.LicenseStateFolder),
		Installation: &licensing.Installation{ServerID: identity.ServerID, PublicKey: key.PublicKey},
		PrivateKey:   key.PrivateKey,
		Trust:        &trust,
		MachineID: func() (string, error) {
			return "DDDDDDDD-1111-2222-3333-444444444444", nil
		},
	})
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		arguments []string
		state     licensing.State
	}{
		{[]string{"--type", "subscription", "--expires-at", "10d", "--revision", "1"}, licensing.StateActive},
		{[]string{"--type", "subscription", "--expires-at", "-2d", "--revision", "2"}, licensing.StateGrace},
		{[]string{"--type", "subscription", "--expires-at", "-6d", "--revision", "3"}, licensing.StateExpired},
		{[]string{"--type", "trial", "--plan", "basico", "--expires-at", "720h", "--revision", "4"}, licensing.StateActive},
		{[]string{"--status", "revoked", "--reason", "transferred", "--revision", "5"}, licensing.StateRevoked},
	}
	licenseID := licensetest.NewUUID()
	for _, item := range cases {
		file := filepath.Join(t.TempDir(), "prueba.lic")
		var output bytes.Buffer
		arguments := append([]string{"issue", "--identity", identityDir, "--license-id", licenseID, "--output", file}, item.arguments...)
		if code := run(context.Background(), arguments, &output, nil); code != 0 {
			t.Fatalf("%v: %s", item.arguments, output.String())
		}
		line := lastLine(t, &output)
		if line.Event != "issued" || line.Output != file || line.LicenseID != licenseID {
			t.Fatalf("línea JSON: %+v", line)
		}
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		status, err := manager.ImportLicense(string(content))
		if err != nil || status.State != item.state {
			t.Fatalf("%v: %+v %v", item.arguments, status, err)
		}
	}
	// Cupos propios: 0 comanderos.
	var output bytes.Buffer
	if code := run(context.Background(), []string{"issue", "--identity", identityDir, "--order-takers", "0", "--revision", "9", "--license-id", licenseID}, &output, nil); code != 0 {
		t.Fatal(output.String())
	}
	status, err := manager.ImportLicense(lastLine(t, &output).Output)
	if err != nil || status.Limits != (licensing.Limits{CashRegisters: 3, OrderTakers: 0, KitchenDisplays: 3}) || !status.Plan.Custom {
		t.Fatalf("cupos propios: %+v %v", status, err)
	}
	// Un contenido que el contrato no admite falla con su motivo y no escribe nada.
	output.Reset()
	if code := run(context.Background(), []string{"issue", "--identity", identityDir, "--type", "subscription", "--grace-days", "3"}, &output, nil); code != 1 {
		t.Fatalf("grace_days inválido: %d %s", code, output.String())
	}
}

// server atiende el contrato en loopback con TLS y su certificado en --ca-out: un cliente que confía en él activa.
func TestLicenseTestServer(t *testing.T) {
	t.Setenv("ADEL_TEST_MODE", "1")
	t.Setenv("ADEL_TEST_LICENSE_TRUST", vectorsFile)
	var output bytes.Buffer
	if code := run(context.Background(), []string{"server", "--listen", "192.0.2.1:9", "--ca-out", filepath.Join(t.TempDir(), "ca.pem")}, &output, nil); code != 2 {
		t.Fatal("el servidor simulado solo escucha en loopback")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	ready := make(chan string, 1)
	finished := make(chan int, 1)
	var serverOutput bytes.Buffer
	go func() {
		finished <- run(ctx, []string{"server", "--listen", "127.0.0.1:0", "--ca-out", caFile}, &serverOutput, ready)
	}()
	var url string
	select {
	case url = <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("el servidor simulado no arrancó")
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pem)
	fixtures, err := licensetest.LoadFixtures(vectorsFile)
	if err != nil {
		t.Fatal(err)
	}
	installation := fixtures.Installations["installation-fixture-a"]
	trust := fixtures.Trust()
	identityDir := t.TempDir()
	if err := licensing.WriteMachineRecord(identityDir, installation.PrivateKey, installation.ServerID, licensing.MachineFingerprint("eeeeeeee-1111-2222-3333-444444444444"), time.Now()); err != nil {
		t.Fatal(err)
	}
	manager := licensing.NewManager(licensing.Options{
		Directory:    filepath.Join(identityDir, "licencia"),
		Installation: &licensing.Installation{ServerID: installation.ServerID, PublicKey: installation.PublicKey},
		PrivateKey:   installation.PrivateKey,
		Trust:        &trust,
		MachineID: func() (string, error) {
			return "EEEEEEEE-1111-2222-3333-444444444444", nil
		},
	})
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	client := licensing.NewClient(manager, licensing.ClientOptions{BaseURL: url, TestMode: true, RootCAs: roots, AppVersion: "0.15.0"})
	status, err := client.Activate(context.Background(), "BSC-2222-2222-2222")
	if err != nil || status.State != licensing.StateActive || status.Limits.OrderTakers != 0 {
		t.Fatalf("activación contra el servidor simulado: %+v %v", status, err)
	}
	cancel()
	if code := <-finished; code != 0 {
		t.Fatalf("el servidor simulado no se detuvo en orden: %s", serverOutput.String())
	}
}
