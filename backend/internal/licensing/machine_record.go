package licensing

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Registro firmado de la huella del equipo (revisión R1, hallazgo R1-2; decisión del coordinador L1-D27).
//
// La huella con que se compara el equipo al arrancar no vive en estado.json: borrar o dañar ese archivo la anularía y
// una carpeta copiada a otra computadora quedaría vigente. Vive en identity/license-machine.json, junto a la llave de
// licencias, firmada con esa llave y atada al server_id y a la llave pública:
//
//   - se crea con la llave, en el mismo paso (localsetup.EnsureLicenseKey): primero este registro y al final la
//     llave, así que un corte deja a lo más un registro sin llave, que se descarta, nunca una llave sin registro;
//   - si la llave existe y el registro falta, está alterado, es de otra instalación o de otra huella, se falla
//     cerrado: la instalación se trata como copiada (Copied; «Sin activar» o «Sin activar con datos», L1-D26);
//   - nunca se vuelve a crear solo: el único camino es crear una identidad nueva (license/identity/regenerate).
//
// Qué no protege: quien tiene la llave privada de licencias (está en la misma carpeta, con los mismos permisos) puede
// firmar otro registro para otra computadora. Es el administrador del equipo, que también puede clonar la máquina
// completa (contrato §12). El registro evita que la copia se active con solo borrar o editar un archivo.

// MachineRecordFile es el registro de la huella dentro de la carpeta de la identidad.
const MachineRecordFile = "license-machine.json"

// machineRecordPrefix es el inicio del mensaje firmado.
const machineRecordPrefix = "ADELPOS-EQUIPO-REGISTRO-V1\n"

// Motivos por los que el registro no vale (para la bitácora).
const (
	machineRecordMissing      = "missing"
	machineRecordInvalid      = "invalid"
	machineRecordOtherInstall = "other_installation"
)

type machineRecord struct {
	Format                int    `json:"format"`
	ServerID              string `json:"server_id"`
	InstallationPublicKey string `json:"installation_public_key"`
	MachineFingerprint    string `json:"machine_fingerprint"`
	CreatedAt             string `json:"created_at"`
	Signature             string `json:"signature"`
}

func machineRecordMessage(record machineRecord) []byte {
	return []byte(machineRecordPrefix + record.ServerID + "\n" + record.InstallationPublicKey + "\n" +
		record.MachineFingerprint + "\n" + record.CreatedAt)
}

// ReadMachineFingerprint lee la huella de este equipo, con los tres intentos de L1-D10.
func ReadMachineFingerprint() (string, error) {
	return readFingerprint(readMachineID, 200*time.Millisecond)
}

// WriteMachineRecord firma y guarda el registro de la huella en la carpeta de la identidad (escritura atómica, 0600).
// Solo lo usan la creación de la llave de licencias y las pruebas.
func WriteMachineRecord(identityDir string, private ed25519.PrivateKey, serverID, fingerprint string, now time.Time) error {
	if len(private) != ed25519.PrivateKeySize || !uuidV4Pattern.MatchString(serverID) || !sha256HexPattern.MatchString(fingerprint) {
		return errors.New("registro de la huella inválido")
	}
	record := machineRecord{
		Format:                1,
		ServerID:              serverID,
		InstallationPublicKey: encodeBase64URL(private.Public().(ed25519.PublicKey)),
		MachineFingerprint:    fingerprint,
		CreatedAt:             formatInstant(now),
	}
	record.Signature = encodeBase64URL(ed25519.Sign(private, machineRecordMessage(record)))
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(identityDir, MachineRecordFile, append(body, '\n'))
}

// readMachineRecord lee y verifica el registro con la identidad cargada. Devuelve la huella registrada, o el motivo
// por el que no vale.
func readMachineRecord(identityDir string, installation Installation) (string, string) {
	raw, err := readLimited(filepath.Join(identityDir, MachineRecordFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", machineRecordMissing
	}
	if err != nil {
		return "", machineRecordInvalid
	}
	var record machineRecord
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || record.Format != 1 || !sha256HexPattern.MatchString(record.MachineFingerprint) {
		return "", machineRecordInvalid
	}
	if record.ServerID != installation.ServerID || record.InstallationPublicKey != installation.PublicKey {
		return "", machineRecordOtherInstall
	}
	public, okPublic := decodeBase64URL(installation.PublicKey)
	signature, okSignature := decodeBase64URL(record.Signature)
	if !okPublic || !okSignature || len(public) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(ed25519.PublicKey(public), machineRecordMessage(record), signature) {
		return "", machineRecordInvalid
	}
	return record.MachineFingerprint, ""
}

// RemoveMachineRecord borra un registro sin llave (el de una creación interrumpida). Nunca se llama con llave.
func RemoveMachineRecord(identityDir string) {
	_ = os.Remove(filepath.Join(identityDir, MachineRecordFile))
}
