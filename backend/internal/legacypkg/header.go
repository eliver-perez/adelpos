package legacypkg

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

// Signature son los primeros 8 bytes de todo paquete: «ADELMIG» y un byte NUL, para que ningún archivo de texto
// empiece igual (la comprobación de datos reales del repositorio la busca, scripts/check-legacy-data.sh).
const Signature = "ADELMIG\x00"

// FormatVersion es la versión del formato que escribe y lee esta versión de AD-EL POS.
const FormatVersion uint16 = 1

const (
	kdfName         = "argon2id"
	cipherName      = "AES-256-GCM"
	compressionName = "gzip"
	archiveName     = "tar"
	saltSize        = 32
	noncePrefixSize = 7
	keySize         = 32
	keyCheckSize    = sha256.Size
	// fixedHeaderSize: firma, versión (uint16) y longitud del JSON de parámetros (uint32).
	fixedHeaderSize = len(Signature) + 2 + 4
	// Textos de derivación de las subllaves y del valor de comprobación (fijos del formato v1).
	infoEncryption = "adelmig v1 encryption key"
	infoKeyCheck   = "adelmig v1 key check key"
	keyCheckLabel  = "adelmig v1 key check value"
)

// Header es lo que el encabezado en claro dice del paquete (sin la sal ni el valor de comprobación).
type Header struct {
	Version   uint16
	KDF       KDFParams
	ChunkSize int
}

// headerKDF es la derivación de la llave tal como se guarda en el JSON del encabezado.
type headerKDF struct {
	Name      string `json:"name"`
	Salt      []byte `json:"salt"`
	Time      uint32 `json:"time"`
	MemoryKiB uint32 `json:"memory_kib"`
	Threads   uint8  `json:"threads"`
}

// headerParams es el JSON corto del encabezado. Los []byte se guardan en base64.
type headerParams struct {
	KDF         headerKDF `json:"kdf"`
	Cipher      string    `json:"cipher"`
	ChunkSize   int       `json:"chunk_size"`
	NoncePrefix []byte    `json:"nonce_prefix"`
	KeyCheck    []byte    `json:"key_check"`
	Compression string    `json:"compression"`
	Archive     string    `json:"archive"`
}

// packageKeys son la llave del cifrado y la del valor de comprobación, derivadas de la frase de paso.
type packageKeys struct {
	encryption []byte
	check      []byte
}

// normalizePassphrase usa la forma NFC: la misma frase escrita en una Mac o en Windows (por ejemplo con «ñ» o
// acentos) da la misma llave.
func normalizePassphrase(passphrase string) []byte {
	return []byte(norm.NFC.String(passphrase))
}

// ValidatePassphrase comprueba la frase de paso que elige el propietario al preparar un paquete.
func ValidatePassphrase(passphrase string) error {
	if !utf8.ValidString(passphrase) {
		return invalid("La frase de paso tiene caracteres que no se pueden leer.")
	}
	if len(passphrase) > MaxPassphraseBytes {
		return invalid("La frase de paso es demasiado larga.")
	}
	if utf8.RuneCountInString(norm.NFC.String(passphrase)) < MinPassphraseLength {
		return invalid("La frase de paso debe tener al menos 8 caracteres.")
	}
	return nil
}

// deriveKeys deriva con Argon2id una llave maestra y de ella, con HKDF-SHA-256, las subllaves del cifrado y de la
// comprobación (así el valor de comprobación nunca revela nada de la llave del cifrado).
func deriveKeys(passphrase string, kdf headerKDF) (packageKeys, error) {
	master := argon2.IDKey(normalizePassphrase(passphrase), kdf.Salt, kdf.Time, kdf.MemoryKiB, kdf.Threads, keySize)
	encryption, err := hkdf.Key(sha256.New, master, kdf.Salt, infoEncryption, keySize)
	if err != nil {
		return packageKeys{}, err
	}
	check, err := hkdf.Key(sha256.New, master, kdf.Salt, infoKeyCheck, keySize)
	if err != nil {
		return packageKeys{}, err
	}
	return packageKeys{encryption: encryption, check: check}, nil
}

// keyCheckValue es el HMAC-SHA-256 de un texto fijo con la subllave de comprobación: distingue «frase de paso
// incorrecta» de «paquete dañado» antes de descifrar nada.
func keyCheckValue(keys packageKeys) []byte {
	mac := hmac.New(sha256.New, keys.check)
	mac.Write([]byte(keyCheckLabel))
	return mac.Sum(nil)
}

// newHeaderParams prepara los parámetros de un paquete nuevo con sal y prefijo de nonce aleatorios y calcula sus
// llaves y el valor de comprobación.
func newHeaderParams(passphrase string, kdf KDFParams, chunkSize int) (headerParams, packageKeys, error) {
	params := headerParams{
		KDF: headerKDF{
			Name:      kdfName,
			Salt:      make([]byte, saltSize),
			Time:      kdf.Time,
			MemoryKiB: kdf.MemoryKiB,
			Threads:   kdf.Threads,
		},
		Cipher:      cipherName,
		ChunkSize:   chunkSize,
		NoncePrefix: make([]byte, noncePrefixSize),
		Compression: compressionName,
		Archive:     archiveName,
	}
	if _, err := rand.Read(params.KDF.Salt); err != nil {
		return headerParams{}, packageKeys{}, err
	}
	if _, err := rand.Read(params.NoncePrefix); err != nil {
		return headerParams{}, packageKeys{}, err
	}
	keys, err := deriveKeys(passphrase, params.KDF)
	if err != nil {
		return headerParams{}, packageKeys{}, err
	}
	params.KeyCheck = keyCheckValue(keys)
	return params, keys, nil
}

// encodeHeader arma el encabezado completo: firma, versión, longitud y JSON de parámetros, y el SHA-256 de todo eso
// (detecta un encabezado dañado aunque no se conozca la frase de paso).
func encodeHeader(params headerParams) ([]byte, error) {
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if len(encoded) > maxHeaderParamsBytes {
		return nil, errors.New("parámetros del encabezado demasiado largos")
	}
	var header bytes.Buffer
	header.WriteString(Signature)
	_ = binary.Write(&header, binary.BigEndian, FormatVersion)
	_ = binary.Write(&header, binary.BigEndian, uint32(len(encoded)))
	header.Write(encoded)
	checksum := sha256.Sum256(header.Bytes())
	header.Write(checksum[:])
	return header.Bytes(), nil
}

// readHeader lee y valida el encabezado. Devuelve los bytes exactos (su SHA-256 es el AAD de los bloques) y los
// parámetros, ya acotados por los límites.
func readHeader(source io.Reader, limits Limits) ([]byte, headerParams, error) {
	fixed := make([]byte, fixedHeaderSize)
	read, err := io.ReadFull(source, fixed)
	if read < len(Signature) || string(fixed[:len(Signature)]) != Signature {
		return nil, headerParams{}, notPackage()
	}
	if err != nil {
		return nil, headerParams{}, damaged("encabezado truncado", err)
	}
	version := binary.BigEndian.Uint16(fixed[len(Signature):])
	if version == 0 {
		return nil, headerParams{}, damaged("versión del formato 0", nil)
	}
	if version > FormatVersion {
		return nil, headerParams{}, unsupportedVersion(version)
	}
	length := binary.BigEndian.Uint32(fixed[len(Signature)+2:])
	if length == 0 || length > maxHeaderParamsBytes {
		return nil, headerParams{}, damaged("longitud de los parámetros del encabezado", nil)
	}
	rest := make([]byte, int(length)+sha256.Size)
	if _, err := io.ReadFull(source, rest); err != nil {
		return nil, headerParams{}, damaged("encabezado truncado", err)
	}
	raw := append(fixed, rest...)
	body := raw[:len(raw)-sha256.Size]
	checksum := sha256.Sum256(body)
	if !hmac.Equal(checksum[:], raw[len(raw)-sha256.Size:]) {
		return nil, headerParams{}, damaged("el SHA-256 del encabezado no coincide", nil)
	}
	params, err := decodeHeaderParams(body[fixedHeaderSize:])
	if err != nil {
		return nil, headerParams{}, err
	}
	if err := checkHeaderParams(params, limits); err != nil {
		return nil, headerParams{}, err
	}
	return raw, params, nil
}

// decodeHeaderParams decodifica el JSON de parámetros de forma estricta (sin campos desconocidos ni datos de más).
func decodeHeaderParams(encoded []byte) (headerParams, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var params headerParams
	if err := decoder.Decode(&params); err != nil {
		return headerParams{}, damaged("JSON del encabezado", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return headerParams{}, damaged("datos de más en el JSON del encabezado", nil)
	}
	return params, nil
}

// checkHeaderParams valida los parámetros contra el formato v1 y contra los límites, antes de gastar memoria o tiempo
// en Argon2.
func checkHeaderParams(params headerParams, limits Limits) error {
	if params.KDF.Name != kdfName || params.Cipher != cipherName || params.Compression != compressionName ||
		params.Archive != archiveName {
		return damaged("algoritmos del encabezado", nil)
	}
	if len(params.KDF.Salt) != saltSize || len(params.NoncePrefix) != noncePrefixSize ||
		len(params.KeyCheck) != keyCheckSize {
		return damaged("tamaños de la sal, el nonce o la comprobación", nil)
	}
	if params.KDF.Time < minArgonTime || params.KDF.MemoryKiB < minArgonMemoryKiB || params.KDF.Threads == 0 {
		return damaged("parámetros de Argon2 por debajo del mínimo", nil)
	}
	if params.KDF.Time > limits.MaxArgonTime || params.KDF.MemoryKiB > limits.MaxArgonMemoryKiB ||
		params.KDF.Threads > limits.MaxArgonThreads {
		return limitExceeded("parámetros de Argon2")
	}
	if params.ChunkSize < MinChunkSize {
		return damaged("tamaño de bloque", nil)
	}
	if params.ChunkSize > limits.MaxChunkSize {
		return limitExceeded("tamaño de bloque")
	}
	return nil
}

// checkPassphrase deriva las llaves del paquete y comprueba la frase de paso con el valor de comprobación.
func checkPassphrase(passphrase string, params headerParams) (packageKeys, error) {
	keys, err := deriveKeys(passphrase, params.KDF)
	if err != nil {
		return packageKeys{}, damaged("derivación de la llave", err)
	}
	if !hmac.Equal(keyCheckValue(keys), params.KeyCheck) {
		return packageKeys{}, wrongPassphrase()
	}
	return keys, nil
}
