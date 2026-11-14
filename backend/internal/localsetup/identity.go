package localsetup

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"local/ad-el-pos/server/internal/fileacl"
)

// Identidad del servidor del restaurante (etapa 9G, fase B; decisiones D1 y D6).
//
// Al primer arranque del servidor (el servicio de macOS o «serve» de desarrollo) se crea, en la carpeta
// IdentityFolder junto a server.json:
//
//   - identity.json: el identificador del servidor (server_id, UUID aleatorio) y cuándo se creó;
//   - server-cert.pem: un certificado propio autofirmado ECDSA P-256 de larga vigencia;
//   - server-key.pem: su llave privada (0600; carpeta 0700 y, en Windows, ACL privada).
//
// Las terminales no confían en una autoridad ni en el nombre o la IP del certificado: durante el
// emparejamiento fijan la huella SPKI (SHA-256 de la llave pública del certificado) y desde entonces solo
// se conectan a un servidor que presente esa misma llave. Por eso la identidad **nunca se reemplaza sola**:
// si falta una parte o está dañada, el servidor no arranca y pide revisión (regenerarla obligaría a volver a
// emparejar todas las terminales).
const (
	IdentityFolder   = "identity"
	identityFile     = "identity.json"
	certificateFile  = "server-cert.pem"
	privateKeyFile   = "server-key.pem"
	identityValidity = 30
)

var serverIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ServerIdentity es la identidad cargada: el identificador, la huella SPKI y el certificado con su llave
// para la escucha HTTPS.
type ServerIdentity struct {
	ServerID    string
	Fingerprint [32]byte
	Certificate tls.Certificate
}

// FingerprintHex es la huella SPKI en hexadecimal en minúsculas (64 caracteres), el formato que guardan las
// terminales y la base de datos.
func (identity ServerIdentity) FingerprintHex() string {
	return hex.EncodeToString(identity.Fingerprint[:])
}

type identityRecord struct {
	ServerID  string    `json:"server_id"`
	CreatedAt time.Time `json:"created_at"`
}

// SPKIFingerprint es el SHA-256 de la llave pública del certificado (SubjectPublicKeyInfo en DER), lo
// mismo que calcula la terminal con el certificado que recibe en el intercambio TLS.
func SPKIFingerprint(certificate *x509.Certificate) [32]byte {
	return sha256.Sum256(certificate.RawSubjectPublicKeyInfo)
}

// EnsureIdentity carga la identidad de la carpeta o, si la carpeta no tiene ninguno de sus archivos, la
// crea. Una identidad incompleta o ilegible es un error: nunca se reemplaza automáticamente.
func EnsureIdentity(directory string) (ServerIdentity, error) {
	present := 0
	for _, name := range []string{identityFile, certificateFile, privateKeyFile} {
		if _, err := os.Lstat(filepath.Join(directory, name)); err == nil {
			present++
		}
	}
	if present == 3 {
		return LoadIdentity(directory)
	}
	if present != 0 {
		return ServerIdentity{}, errors.New("la identidad del servidor está incompleta; no se reemplazará automáticamente porque las terminales emparejadas dejarían de conectarse (revisión manual necesaria)")
	}
	if err := createIdentity(directory); err != nil {
		return ServerIdentity{}, err
	}
	return LoadIdentity(directory)
}

// LoadIdentity lee y comprueba una identidad existente: identificador válido, certificado ECDSA P-256 y
// llave privada que corresponde al certificado.
func LoadIdentity(directory string) (ServerIdentity, error) {
	damaged := errors.New("la identidad del servidor no es válida; no se reemplazará automáticamente (revisión manual necesaria)")
	record, err := readIdentityRecord(filepath.Join(directory, identityFile))
	if err != nil {
		return ServerIdentity{}, damaged
	}
	certificate, err := tls.LoadX509KeyPair(filepath.Join(directory, certificateFile), filepath.Join(directory, privateKeyFile))
	if err != nil || len(certificate.Certificate) != 1 {
		return ServerIdentity{}, damaged
	}
	parsed, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return ServerIdentity{}, damaged
	}
	publicKey, ok := parsed.PublicKey.(*ecdsa.PublicKey)
	if !ok || publicKey.Curve != elliptic.P256() {
		return ServerIdentity{}, damaged
	}
	certificate.Leaf = parsed
	return ServerIdentity{ServerID: record.ServerID, Fingerprint: SPKIFingerprint(parsed), Certificate: certificate}, nil
}

func readIdentityRecord(filename string) (identityRecord, error) {
	file, err := os.Open(filename)
	if err != nil {
		return identityRecord{}, err
	}
	defer file.Close()
	var record identityRecord
	decoder := json.NewDecoder(io.LimitReader(file, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return identityRecord{}, err
	}
	if !serverIDPattern.MatchString(record.ServerID) {
		return identityRecord{}, errors.New("identificador de servidor inválido")
	}
	return record, nil
}

// newServerID es un UUID versión 4 en minúsculas.
func newServerID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	text := hex.EncodeToString(value[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", text[0:8], text[8:12], text[12:16], text[16:20], text[20:32]), nil
}

// createIdentity genera el identificador, la llave ECDSA P-256 y el certificado autofirmado. Los archivos se
// crean con O_EXCL (nunca sobrescriben) y la llave privada con 0600.
func createIdentity(directory string) error {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return errors.New("no se pudo crear la carpeta de la identidad del servidor")
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return errors.New("no se pudo proteger la carpeta de la identidad del servidor")
	}
	// En Windows los bits 0700/0600 no protegen: la carpeta recibe una ACL privada que heredan los
	// archivos (como las claves TLS de la etapa 9E). En macOS no hace nada más.
	if err := fileacl.Protect(directory); err != nil {
		return err
	}
	serverID, err := newServerID()
	if err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	// El nombre y las direcciones del certificado no se usan para confiar en él (las terminales fijan la
	// llave): el servidor sigue siendo el mismo aunque cambie su IP por DHCP.
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "AD-EL POS " + serverID, Organization: []string{"AD-EL POS"}},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(identityValidity, 0, 0),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	record, err := json.MarshalIndent(identityRecord{ServerID: serverID, CreatedAt: now}, "", "  ")
	if err != nil {
		return err
	}
	// La llave primero y el registro al final: si algo falla a medias queda una identidad incompleta, que
	// EnsureIdentity no completa por su cuenta.
	files := []struct {
		name string
		body []byte
	}{
		{privateKeyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})},
		{certificateFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})},
		{identityFile, append(record, '\n')},
	}
	for _, file := range files {
		if err := writeExclusive(filepath.Join(directory, file.name), file.body); err != nil {
			return errors.New("no se pudo guardar la identidad del servidor")
		}
	}
	return nil
}

func writeExclusive(filename string, body []byte) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(body)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
