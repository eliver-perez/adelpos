//go:build !adelpos_production

package licensetest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http/httptest"
	"time"
)

// StartTLS arranca el servidor simulado con httptest y TLS en loopback, y devuelve sus autoridades de confianza para
// el cliente (ClientOptions.RootCAs). Quien lo llama lo cierra.
func (server *Server) StartTLS() (*httptest.Server, *x509.CertPool) {
	listener := httptest.NewUnstartedServer(server)
	// Los apretones de manos que las pruebas rechazan a propósito (TLS no verificado) no llenan la salida.
	listener.Config.ErrorLog = log.New(io.Discard, "", 0)
	listener.StartTLS()
	pool := x509.NewCertPool()
	pool.AddCert(listener.Certificate())
	return listener, pool
}

// SelfSignedCertificate crea el certificado del servidor simulado como proceso: ECDSA P-256 para 127.0.0.1, ::1 y
// localhost, de un día. Devuelve el certificado para servir y su PEM, que el Go temporal recibe como autoridad
// (ADEL_TEST_LICENSE_CA): así el cliente sigue verificando TLS también en las pruebas.
func SelfSignedCertificate() (tls.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Servidor de licencias simulado"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, certificatePEM, nil
}
