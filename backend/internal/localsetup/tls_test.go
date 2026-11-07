package localsetup

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTLSRequiresTrustedCAAndCorrectHostname(t *testing.T) {
	directory := t.TempDir()
	if err := CreateTLS(directory, "192.168.1.20"); err != nil {
		t.Fatal(err)
	}
	if err := CreateTLS(directory, "192.168.1.20"); err == nil {
		t.Fatal("Se reemplazaron certificados existentes")
	}
	pair, err := tls.LoadX509KeyPair(filepath.Join(directory, "server.pem"), filepath.Join(directory, "server.key"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	if response, err := http.Get(server.URL); err == nil {
		response.Body.Close()
		t.Fatal("Se confió en una CA no autorizada")
	}
	encoded, err := os.ReadFile(filepath.Join(directory, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(encoded) {
		t.Fatal("La CA no es válida")
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal("No se aceptó el certificado confiable")
	}
	response.Body.Close()
	certificatePEM, _ := os.ReadFile(filepath.Join(directory, "server.pem"))
	block, _ := pem.Decode(certificatePEM)
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil || certificate.VerifyHostname("192.168.1.20") != nil || certificate.VerifyHostname("192.168.1.21") == nil {
		t.Fatal("La validación de identidad del servidor no funciona")
	}
	permissions, _ := os.Stat(filepath.Join(directory, "server.key"))
	if permissions.Mode().Perm() != 0600 {
		t.Fatal("La clave privada no tiene permisos privados")
	}
}
