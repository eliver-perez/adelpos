package licensing

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/fileacl"
)

// Almacén del estado de la licencia en identity/licencia (L1-D9). Está en identity/ y no en la base porque
// identity/ nunca viaja en un respaldo (contrato §2): restaurar un respaldo viejo no puede devolver una
// revisión anterior (por ejemplo, la de antes de una revocación) ni bajar la última hora confiable.
//
//   - estado.json: la licencia vigente (su JWS), la revisión más alta aceptada de cada `license_id`, la última
//     hora confiable con su fuente, la huella del equipo y la identidad a la que pertenece. Es el punto de
//     confirmación: se escribe completo en un archivo temporal, con fsync, y se renombra encima del anterior,
//     así que un corte deja el anterior o el nuevo, nunca uno a medias.
//   - vigente.lic: copia de la licencia vigente, tal como viaja, para soporte. Se escribe después de
//     estado.json con el mismo método; si no coincide con estado.json (un corte entre los dos o un cambio a
//     mano), al arrancar se vuelve a escribir desde estado.json.
//
// Carpeta 0700 y archivos 0600 (en Windows, la ACL privada de fileacl). Un solo proceso usa la carpeta: el
// servicio toma host.lock de la carpeta de datos.
//
// Nada de esto está firmado: el administrador del equipo puede borrarlo o cambiarlo, igual que puede
// restaurar una instantánea completa de la computadora (contrato §12). Lo que sí se garantiza es que una
// licencia guardada solo se usa si vuelve a pasar la §10 al arrancar (L1-D20).

const (
	stateFileName   = "estado.json"
	licenseFileName = "vigente.lic"
	// stateFormat es la versión del formato de estado.json.
	stateFormat = 1
	// maxStateSize limita lo que se lee de estado.json y de vigente.lic.
	maxStateSize = 1 << 20
)

var sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// errTooLarge es un archivo de estado más grande que maxStateSize: se trata como dañado.
var errTooLarge = errors.New("archivo demasiado grande")

// acceptedRecord es la revisión más alta aceptada de una licencia.
type acceptedRecord struct {
	Revision int64  `json:"license_revision"`
	SHA256   string `json:"sha256"`
}

// storedState es el contenido de estado.json.
type storedState struct {
	Format int `json:"format"`
	// Identidad a la que pertenece (para soporte): la licencia se comprueba siempre contra la identidad
	// cargada, no contra estos valores.
	ServerID              string `json:"server_id,omitempty"`
	InstallationPublicKey string `json:"installation_public_key,omitempty"`
	// MachineFingerprint es la huella del equipo donde se guardó (machine.go). Se registra la primera vez que
	// arranca esta versión y con cada licencia aceptada.
	MachineFingerprint string `json:"machine_fingerprint,omitempty"`
	// Accepted es la revisión más alta aceptada de cada license_id. Nunca baja ni se borra.
	Accepted map[string]acceptedRecord `json:"accepted_revisions"`
	// License es el JWS de la licencia vigente (vacío si todavía no hay ninguna).
	License string `json:"license_jws,omitempty"`
	// TrustedTime es la última hora confiable (§5.6) y TrustedSource su fuente.
	TrustedTime   *time.Time `json:"last_trusted_time,omitempty"`
	TrustedSource string     `json:"last_trusted_source,omitempty"`
	// Support es la solicitud de desbloqueo de soporte pendiente y los números ya usados (Fase C, unlock.go).
	Support   *supportState `json:"support_unlock,omitempty"`
	UpdatedAt time.Time     `json:"updated_at"`
}

func newStoredState() storedState {
	return storedState{Format: stateFormat, Accepted: map[string]acceptedRecord{}}
}

// clone copia el estado para preparar una escritura sin tocar el que está en memoria.
func (state storedState) clone() storedState {
	copied := state
	copied.Accepted = make(map[string]acceptedRecord, len(state.Accepted))
	for licenseID, record := range state.Accepted {
		copied.Accepted[licenseID] = record
	}
	if state.TrustedTime != nil {
		instant := *state.TrustedTime
		copied.TrustedTime = &instant
	}
	copied.Support = state.Support.clone()
	return copied
}

// accepted es la revisión aceptada de una licencia, en el formato del verificador, o nil.
func (state storedState) accepted(licenseID string) *Accepted {
	record, exists := state.Accepted[licenseID]
	if !exists {
		return nil
	}
	return &Accepted{LicenseID: licenseID, Revision: record.Revision, SHA256: record.SHA256}
}

// matchesAccepted indica si la licencia es exactamente la revisión aceptada de su license_id.
func (state storedState) matchesAccepted(license *License) bool {
	record, exists := state.Accepted[license.LicenseID]
	return exists && record.Revision == license.LicenseRevision && record.SHA256 == license.SHA256
}

// recordAccepted guarda la revisión de una licencia aceptada. La revisión de un license_id nunca retrocede:
// una menor, o una igual con otro contenido, es un error (el verificador ya la rechazó antes; esto protege el
// almacén por sí mismo).
func (state *storedState) recordAccepted(license *License) error {
	if err := checkRevision(license, state.accepted(license.LicenseID)); err != nil {
		return err
	}
	state.Accepted[license.LicenseID] = acceptedRecord{Revision: license.LicenseRevision, SHA256: license.SHA256}
	return nil
}

// raiseTrustedTime sube la última hora confiable. Devuelve si cambió.
func (state *storedState) raiseTrustedTime(instant time.Time, source string) bool {
	if instant.IsZero() {
		return false
	}
	instant = instant.UTC()
	if state.TrustedTime != nil && !instant.After(*state.TrustedTime) {
		return false
	}
	state.TrustedTime = &instant
	state.TrustedSource = source
	return true
}

// validate comprueba un estado leído del disco. Uno inválido se aparta como dañado.
func (state storedState) validate() error {
	if state.Format != stateFormat {
		return errors.New("formato desconocido")
	}
	if state.Accepted == nil {
		return errors.New("sin revisiones aceptadas")
	}
	for licenseID, record := range state.Accepted {
		if !uuidPattern.MatchString(licenseID) || record.Revision < 1 || !sha256HexPattern.MatchString(record.SHA256) {
			return errors.New("revisión aceptada inválida")
		}
	}
	if state.MachineFingerprint != "" && !sha256HexPattern.MatchString(state.MachineFingerprint) {
		return errors.New("huella del equipo inválida")
	}
	if state.TrustedTime != nil && state.TrustedTime.IsZero() {
		return errors.New("última hora confiable inválida")
	}
	return nil
}

// stateStore lee y escribe identity/licencia.
type stateStore struct {
	directory string
}

// readOutcome dice qué se encontró al leer estado.json.
type readOutcome int

const (
	stateMissing     readOutcome = iota // no existe: primera vez
	stateRead                           // leído y válido
	stateDamaged                        // contenido inválido: se apartó y se empieza de nuevo
	stateUnavailable                    // no se pudo leer (permisos, disco): no se toca y no se escribe
)

// ensureDirectory crea la carpeta privada si falta (0700; en Windows, la ACL privada).
func (store stateStore) ensureDirectory() error {
	if err := os.MkdirAll(store.directory, 0700); err != nil {
		return errors.New("no se pudo crear la carpeta del estado de la licencia")
	}
	if err := os.Chmod(store.directory, 0700); err != nil {
		return errors.New("no se pudo proteger la carpeta del estado de la licencia")
	}
	if err := fileacl.Protect(store.directory); err != nil {
		return err
	}
	return nil
}

// removeLeftovers descarta los temporales de una escritura interrumpida (nunca los archivos definitivos).
func (store stateStore) removeLeftovers() {
	entries, err := os.ReadDir(store.directory)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && (strings.HasPrefix(name, "."+stateFileName+"-") || strings.HasPrefix(name, "."+licenseFileName+"-")) {
			_ = os.Remove(filepath.Join(store.directory, name))
		}
	}
}

// read lee estado.json. Si su contenido está dañado lo aparta (estado.json.danado-AAAAMMDD-HHMMSS, para
// soporte) y devuelve un estado nuevo: la licencia de vigente.lic se puede recuperar todavía (manager.go). Si
// no se puede leer (permisos, disco), no lo toca y devuelve un error: el gestor sigue en memoria sin escribir,
// para no reemplazar un estado que quizá es válido.
func (store stateStore) read(now time.Time) (storedState, readOutcome, error) {
	filename := filepath.Join(store.directory, stateFileName)
	raw, err := readLimited(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return newStoredState(), stateMissing, nil
	}
	if err != nil && !errors.Is(err, errTooLarge) {
		return newStoredState(), stateUnavailable, errors.New("no se pudo leer el estado de la licencia")
	}
	if err == nil {
		var state storedState
		decodeErr := json.Unmarshal(raw, &state)
		if decodeErr == nil {
			decodeErr = state.validate()
		}
		if decodeErr == nil {
			return state, stateRead, nil
		}
	}
	damaged := filename + ".danado-" + now.UTC().Format("20060102-150405")
	if renameErr := os.Rename(filename, damaged); renameErr != nil {
		return newStoredState(), stateDamaged, errors.New("el estado de la licencia está dañado y no se pudo apartar")
	}
	return newStoredState(), stateDamaged, nil
}

// write guarda estado.json de forma atómica.
func (store stateStore) write(state storedState, now time.Time) error {
	state.Format = stateFormat
	state.UpdatedAt = now.UTC()
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(store.directory, stateFileName, append(body, '\n'))
}

// readLicenseFile lee vigente.lic; falso si no existe o no se puede leer.
func (store stateStore) readLicenseFile() (string, bool) {
	raw, err := readLimited(filepath.Join(store.directory, licenseFileName))
	if err != nil {
		return "", false
	}
	return string(raw), true
}

// writeLicenseFile guarda vigente.lic de forma atómica, sin salto de línea final (como viaja un .lic).
func (store stateStore) writeLicenseFile(jws string) error {
	return writeAtomic(store.directory, licenseFileName, []byte(jws))
}

func readLimited(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxStateSize+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxStateSize {
		return nil, errTooLarge
	}
	return raw, nil
}

// writeAtomic escribe un archivo nuevo junto al definitivo (0600), lo guarda en disco y lo renombra encima:
// un corte en medio deja el archivo anterior completo y, a lo más, un temporal que se descarta al arrancar.
func writeAtomic(directory, name string, body []byte) error {
	temporary, err := os.CreateTemp(directory, "."+name+"-*")
	if err != nil {
		return errors.New("no se pudo escribir el estado de la licencia")
	}
	temporaryName := temporary.Name()
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(temporaryName)
		}
	}()
	_, writeErr := temporary.Write(body)
	chmodErr := temporary.Chmod(0600)
	syncErr := temporary.Sync()
	closeErr := temporary.Close()
	if writeErr != nil || chmodErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("no se pudo escribir el estado de la licencia")
	}
	if err := os.Rename(temporaryName, filepath.Join(directory, name)); err != nil {
		return errors.New("no se pudo guardar el estado de la licencia")
	}
	complete = true
	syncDirectory(directory)
	return nil
}

// syncDirectory pide al sistema que guarde en disco la lista de archivos de la carpeta (sin efecto donde
// una carpeta no se puede abrir para ello, como en Windows).
func syncDirectory(directory string) {
	handle, err := os.Open(directory)
	if err != nil {
		return
	}
	_ = handle.Sync()
	_ = handle.Close()
}
