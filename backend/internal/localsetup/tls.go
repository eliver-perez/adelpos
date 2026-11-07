// Package localsetup prepara archivos locales sin alterar la confianza global del equipo.
package localsetup

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func CreateTLS(directory, lanIP string) error {
	addresses := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	if lanIP != "" {
		address := net.ParseIP(lanIP)
		if address == nil || !address.IsPrivate() {
			return errors.New("lan-ip debe ser una IP privada específica")
		}
		addresses = append(addresses, address)
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	for _, name := range []string{"ca.pem", "ca.key", "server.pem", "server.key"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err == nil {
			return errors.New("ya existen certificados; no se reemplazarán automáticamente")
		}
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now()
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	caSerial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return err
	}
	ca := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "CA local de desarrollo"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(5, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	serverSerial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return err
	}
	certificate := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject:      pkix.Name{CommonName: "Servidor local"},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.AddDate(1, 0, 0),
		IPAddresses:  addresses,
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, certificate, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	caPrivate, err := x509.MarshalPKCS8PrivateKey(caKey)
	if err != nil {
		return err
	}
	serverPrivate, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		return err
	}
	files := map[string]*pem.Block{
		"ca.pem":     {Type: "CERTIFICATE", Bytes: caDER},
		"ca.key":     {Type: "PRIVATE KEY", Bytes: caPrivate},
		"server.pem": {Type: "CERTIFICATE", Bytes: serverDER},
		"server.key": {Type: "PRIVATE KEY", Bytes: serverPrivate},
	}
	for name, block := range files {
		file, err := os.OpenFile(filepath.Join(directory, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		err = pem.Encode(file, block)
		closeError := file.Close()
		if err != nil {
			return err
		}
		if closeError != nil {
			return closeError
		}
	}
	return nil
}
