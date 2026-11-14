//go:build integration && !windows

package main

import (
	"crypto/tls"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/localsetup"
	"local/ad-el-pos/server/internal/standalone"
)

// checkServiceNetwork comprueba con el servidor real como servicio (etapa 9G, fase B): la identidad se creó
// en la carpeta de datos (carpeta 0700, llave 0600), la escucha HTTPS de red presenta ese certificado con
// TLS 1.2 o superior y no atiende HTTP sin cifrar, y la bitácora informa la escucha y la huella.
func checkServiceNetwork(t *testing.T, layout standalone.Layout, address string) {
	t.Helper()
	directory := filepath.Join(layout.DataDir, localsetup.IdentityFolder)
	requireMode(t, directory, 0700)
	requireMode(t, filepath.Join(directory, "server-key.pem"), 0600)
	identity, err := localsetup.LoadIdentity(directory)
	if err != nil {
		t.Fatal(err)
	}
	var presented [32]byte
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			presented = localsetup.SPKIFingerprint(state.PeerCertificates[0])
			return nil
		},
	}}}
	response, err := client.Get("https://" + address + "/api/v1/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 || presented != identity.Fingerprint {
		t.Fatalf("la escucha de red debe presentar el certificado del servicio: %d", response.StatusCode)
	}
	if plain, err := http.Get("http://" + address + "/api/v1/health/live"); err == nil {
		plain.Body.Close()
		if plain.StatusCode < 400 {
			t.Fatal("la escucha de red no atiende HTTP sin cifrar")
		}
	}
	log, _ := os.ReadFile(layout.ServerLog())
	if !strings.Contains(string(log), `"network_listen":"`+address+`"`) || !strings.Contains(string(log), identity.FingerprintHex()) {
		t.Fatalf("la bitácora informa la escucha de red y la huella: %s", log)
	}
	// La identidad se conserva al reiniciar: la próxima vez se carga la misma.
	again, err := localsetup.EnsureIdentity(directory)
	if err != nil || again.ServerID != identity.ServerID {
		t.Fatal("la identidad del servicio no debe cambiar")
	}
}
