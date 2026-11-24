package licensing

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"time"
)

// Installation es la identidad de la computadora que verifica (contrato §2): su `server_id` y la llave
// pública de licencias, en base64url sin relleno.
type Installation struct {
	ServerID  string
	PublicKey string
}

// DatabaseProbe es lo que el gestor consulta de la base, sin tomar locks del negocio (datapresence.go).
type DatabaseProbe interface {
	// HasOperations indica si la base tiene operaciones: distingue «Sin activar» de «Sin activar con datos».
	HasOperations(ctx context.Context) (bool, error)
	// LatestActivity es la hora de la última actividad registrada (max de audit_events.occurred_at), con
	// falso si no hay ninguna. Solo inicia la última hora confiable cuando falta alguna de sus dos copias (L1-D15 y
	// revisión R2, hallazgo R2-3).
	LatestActivity(ctx context.Context) (time.Time, bool, error)
	// AcceptedRevisions es la segunda copia del historial de revisiones aceptadas, en la base (revisión R1, hallazgo
	// R1-3): por license_id, la revisión más alta y su sha256.
	AcceptedRevisions(ctx context.Context) (map[string]AcceptedRevision, error)
	// RecordAcceptedRevision sube esa copia para una licencia aceptada (nunca la baja).
	RecordAcceptedRevision(ctx context.Context, licenseID string, revision int64, sha256 string) error
	// TrustedTimeCopy es la segunda copia de la última hora confiable, en la base (revisión R2, hallazgo R2-3), con falso
	// si todavía no hay ninguna.
	TrustedTimeCopy(ctx context.Context) (TrustedCopy, bool, error)
	// RaiseTrustedTimeCopy sube esa copia (nunca la baja).
	RaiseTrustedTimeCopy(ctx context.Context, value TrustedCopy) error
	// ReplaceTrustedTimeCopy la reemplaza, también hacia atrás. Solo la usan la §6.3 y la §6.4 (ReplaceTrustedTime y
	// AcceptSupportUnlock).
	ReplaceTrustedTimeCopy(ctx context.Context, value TrustedCopy) error
}

// AcceptedRevision es la revisión más alta aceptada de una licencia en la copia de la base.
type AcceptedRevision struct {
	Revision int64
	SHA256   string
}

// TrustedCopy es la copia de la base de la última hora confiable con su fuente (un TimeSource o un ReplaceReason).
type TrustedCopy struct {
	Time   time.Time
	Source string
}

// knownTrustedSources son las fuentes que acepta la copia de la base (su restricción en la 0028).
var knownTrustedSources = map[string]bool{
	string(TimeFromServer): true, string(TimeFromLicense): true, string(TimeFromClock): true,
	string(TimeFromOperation): true, string(TimeFromAudit): true,
	string(ReplaceByInternetCheck): true, string(ReplaceBySupportUnlock): true,
}

// copySource es la fuente que se guarda en la copia de la base. Una fuente desconocida en estado.json (editada a mano
// o de otra versión) se guarda como reloj local, para que la copia se pueda seguir subiendo.
func copySource(source string) string {
	if knownTrustedSources[source] {
		return source
	}
	return string(TimeFromClock)
}

// databaseInstant es una hora como la guarda PostgreSQL (al microsegundo): así se compara con la copia de la base.
func databaseInstant(instant time.Time) time.Time {
	return instant.UTC().Truncate(time.Microsecond)
}

// databaseTimeout limita cada consulta del gestor a la base.
const databaseTimeout = 5 * time.Second

// Options es lo que necesita el gestor. Todos los campos son opcionales salvo Directory.
type Options struct {
	// Directory es la carpeta del estado de la licencia, identity/licencia (L1-D9).
	Directory string
	// Installation es la identidad de esta computadora. Sin ella (identidad dañada o sin llave de
	// licencias) no se verifica ni se acepta ninguna licencia, el estado es «Sin activar» y no se escribe nada
	// en Directory: así una llave que falló al crearse se puede crear en el siguiente arranque (una carpeta
	// identity/licencia con estado sin llave es una llave perdida, localsetup.EnsureLicenseKey).
	Installation *Installation
	// PrivateKey es la llave privada de licencias de la instalación (localsetup.EnsureLicenseKey). La usa el
	// cliente para la prueba de posesión y los `.licreq` (Fase B). Nunca se registra ni sale del equipo.
	PrivateKey ed25519.PrivateKey
	// IdentityProblem explica, para la bitácora, por qué no hay Installation.
	IdentityProblem string
	// Trust son las llaves de confianza. Sin valor, las del entorno compilado (CompiledTrust).
	Trust *TrustSet
	// Now es el reloj local. Sin valor, time.Now. Las pruebas inyectan uno simulado (L1-D16).
	Now func() time.Time
	// MachineID lee el identificador del equipo (IOPlatformUUID o MachineGuid). Sin valor, el del sistema
	// (machine_*.go). Las pruebas inyectan uno simulado.
	MachineID func() (string, error)
	// MachineRetryDelay es la espera entre los tres intentos de lectura de la huella. Sin valor, 200 ms.
	MachineRetryDelay time.Duration
	// Database es la consulta de la base. Sin valor, la base se considera sin operaciones, la última hora
	// confiable no se inicia con la auditoría y no tiene segunda copia en la base.
	Database DatabaseProbe
	// Logger recibe los avisos técnicos. Nunca recibe una licencia completa ni una llave.
	Logger *slog.Logger
}

// Errores de Accept que no vienen del verificador (los del verificador son *VerifyError).
var (
	// ErrIdentityUnavailable: sin identidad o sin llave de licencias no se acepta ninguna licencia.
	ErrIdentityUnavailable = errors.New("la identidad del servidor o su llave de licencias no están disponibles")
	// ErrCopiedInstallation: la carpeta de datos se copió de otra computadora; antes de activar hay que crear
	// una identidad nueva (L1-D11). Con la identidad copiada el servidor de licencias vería la misma
	// instalación y las dos computadoras quedarían activas.
	ErrCopiedInstallation = errors.New("esta instalación se copió de otra computadora; crea una identidad nueva antes de activar")
	// ErrMachineUnverified: no se pudo leer la huella del equipo (falla cerrada, L1-D10).
	ErrMachineUnverified = errors.New("no se pudo comprobar la huella de esta computadora")
	// ErrStateNotSaved: la licencia era válida pero no se pudo guardar, así que no se aceptó (§10.5 del
	// contrato: se acepta solo lo que queda guardado de forma persistente).
	ErrStateNotSaved = errors.New("no se pudo guardar el estado de la licencia")
)

// Cada cuánto el trabajador del gestor (Run) toma la hora local y guarda la última hora confiable (L1-D15).
const tickInterval = time.Minute

// Manager es el gestor de la licencia del servidor Go principal: verifica (§10 del contrato), guarda en
// identity/licencia (store.go) y calcula el estado (§5.6 y §6) en memoria. Es seguro para uso concurrente.
//
// Interfaz para las demás fases (publicada en la Fase A; no se cambia sin avisar):
//
//   - Status(): el estado vigente, con los cupos.
//   - TrustedTime(): la última hora confiable.
//   - ObserveTime(t, fuente): sube la última hora confiable; nunca la baja.
//   - ReplaceTrustedTime(t, motivo): la reemplaza, también hacia atrás. Solo §6.3 y §6.4.
//   - Accept(jws): verifica y guarda una licencia (respuesta en línea o archivo .lic).
//   - LocalClock(): el reloj local inyectado, para que la puerta y los cupos usen el mismo.
type Manager struct {
	options   Options
	trust     TrustSet
	store     stateStore
	now       func() time.Time
	machineID func() (string, error)
	logger    *slog.Logger

	// timeMutex ordena las lecturas y escrituras de la copia de la base de la última hora confiable (revisión R2, hallazgos
	// R2-3 y R2-4): una subida o una lectura que empezó antes de una baja (§6.3 o §6.4) nunca llega después de ella. Se
	// toma siempre antes que mutex. La subida de cada minuto escribe en la base sin mutex, para que Status nunca espere a
	// la base.
	timeMutex sync.Mutex
	mutex     sync.Mutex
	// state es lo guardado en estado.json, más la última hora confiable todavía sin guardar (dirty).
	state storedState
	dirty bool
	// persistent indica que hay identidad y la carpeta se pudo preparar.
	persistent bool
	// stateLoaded indica que se conoce el estado del disco (leído, inexistente o dañado ya apartado). Sin él no se
	// escribe nada: un estado que no se pudo leer no se reemplaza (revisión R1, hallazgo R1-1). Cada escritura
	// lo vuelve a intentar.
	stateLoaded bool
	// databaseRevisions es la copia del historial de revisiones de la base (R1-3); databaseLoaded, que se leyó.
	databaseRevisions map[string]acceptedRecord
	databaseLoaded    bool
	// databaseTime es lo que tiene la copia de la base de la última hora confiable según lo último que se leyó o se
	// escribió (nil: ninguna), al microsegundo (R2-3). databaseTimeLoaded indica que la base ya respondió: se leyó su copia
	// y, si hacía falta, la última actividad de la auditoría. Mientras no, la semilla está pendiente y el trabajador la
	// reintenta cada minuto (R2-4).
	databaseTime       *time.Time
	databaseTimeLoaded bool
	// license es la licencia vigente ya verificada (nil si no hay ninguna).
	license *License
	// machine es la huella actual del equipo (vacía si no se pudo leer).
	machine           string
	copied            bool
	machineUnverified bool
	hasOperations     bool
}

// NewManager prepara el gestor en memoria. Load lee el disco y la base.
func NewManager(options Options) *Manager {
	manager := &Manager{
		options:   options,
		store:     stateStore{directory: options.Directory},
		now:       options.Now,
		machineID: options.MachineID,
		logger:    options.Logger,
		state:     newStoredState(),
	}
	if manager.now == nil {
		manager.now = time.Now
	}
	if manager.machineID == nil {
		manager.machineID = readMachineID
	}
	if manager.logger == nil {
		manager.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if options.Trust != nil {
		manager.trust = *options.Trust
	} else {
		trust, err := CompiledTrust()
		if err != nil {
			// CheckStartup ya lo impide al arrancar; aquí solo queda sin llaves: no se acepta nada.
			manager.logger.Error("license_trust_invalid", "reason", err.Error())
			trust = TrustSet{Environment: compiledEnvironment, Keys: map[string]TrustedKey{}}
		}
		manager.trust = trust
	}
	return manager
}

// Load lee el estado guardado, comprueba la huella del equipo y la presencia de operaciones en la base, y
// vuelve a verificar la licencia guardada (L1-D20). Se llama una vez, al arrancar. El gestor queda utilizable
// aunque devuelva un error (por ejemplo, sin poder escribir la carpeta): el error es para la bitácora, y el
// estado queda en memoria.
func (manager *Manager) Load(ctx context.Context) error {
	manager.timeMutex.Lock()
	defer manager.timeMutex.Unlock()
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	var problems []error
	now := manager.now()

	// Sin identidad no se escribe nada en identity/licencia (ver Options.Installation).
	if manager.options.Installation == nil {
		manager.logger.Warn("license_identity_unavailable", "reason", manager.options.IdentityProblem)
	} else if err := manager.store.ensureDirectory(); err != nil {
		problems = append(problems, err)
	} else {
		manager.persistent = true
	}

	changed := false
	recoverFromFile := false
	if manager.persistent {
		manager.store.removeLeftovers()
		state, outcome, err := manager.store.read(now)
		if err != nil {
			// Sin poder leerlo (o apartarlo), no se escribe nada: el gestor sigue en memoria y cada escritura lo vuelve
			// a intentar (R1-1).
			problems = append(problems, err)
		} else {
			manager.state = state
			manager.stateLoaded = true
		}
		switch outcome {
		case stateDamaged:
			manager.logger.Error("license_state_damaged", "action", "set_aside")
			recoverFromFile = true
			changed = true
		case stateMissing:
			recoverFromFile = true
			changed = true
		}
	}

	manager.checkMachine()
	if manager.recordInstallation() {
		changed = true
	}
	if err := manager.loadDatabaseRevisions(ctx); err != nil {
		problems = append(problems, err)
	}
	if manager.loadDatabaseTimeLocked(ctx) {
		changed = true
	}
	if manager.loadLicense(recoverFromFile) {
		changed = true
	}
	manager.refreshOperations(ctx)

	if manager.stateLoaded && (changed || manager.dirty) {
		if err := manager.persist(); err != nil {
			problems = append(problems, err)
		}
	}
	// La copia de la base queda al día con lo que se cargó (R2-3). Al arrancar no hay nadie más esperando el mutex.
	if target, needed := manager.trustedCopyTargetLocked(); needed {
		manager.raiseTrustedCopyLocked(ctx, target)
	}
	status := manager.statusLocked()
	manager.logger.Info("license_loaded",
		"state", string(status.State),
		"environment", status.Environment,
		"license_id", status.LicenseID,
		"revision", status.Revision,
		"copied", status.Copied,
		"machine_unverified", status.MachineUnverified,
		"identity_unavailable", status.IdentityUnavailable,
	)
	return errors.Join(problems...)
}

// checkMachine lee la huella del equipo (tres intentos) y la compara con el registro firmado de identity/
// (machine_record.go; revisión R1, hallazgo R1-2). No depende de estado.json: si el registro falta, está
// alterado o es de otra huella, falla cerrado (Copied). Nunca registra una huella nueva.
func (manager *Manager) checkMachine() {
	delay := manager.options.MachineRetryDelay
	if delay == 0 {
		delay = 200 * time.Millisecond
	}
	fingerprint, err := readFingerprint(manager.machineID, delay)
	if err != nil {
		// Falla cerrada (L1-D10): la licencia guardada se ignora hasta que la huella se pueda leer.
		manager.machineUnverified = true
		manager.logger.Error("license_machine_unverified", "attempts", machineAttempts)
		return
	}
	manager.machine = fingerprint
	if manager.options.Installation == nil {
		return
	}
	recorded, problem := readMachineRecord(filepath.Dir(manager.options.Directory), *manager.options.Installation)
	switch {
	case problem != "":
		// Sin registro válido junto a la llave no se puede saber dónde se creó: se trata como copiada.
		manager.copied = true
		manager.logger.Warn("license_installation_copied", "reason", "machine_record_"+problem)
	case recorded != fingerprint:
		// La carpeta de datos se copió de otra computadora (o cambió la tarjeta madre): la licencia guardada se
		// ignora, sin borrarla. Para activar, primero se crea una identidad nueva (license/identity/regenerate).
		manager.copied = true
		manager.logger.Warn("license_installation_copied", "reason", "other_machine")
	}
}

// recordInstallation anota en el estado la identidad a la que pertenece. Si el estado es de otra identidad,
// solo se avisa: las licencias se comprueban siempre contra la identidad cargada.
func (manager *Manager) recordInstallation() bool {
	installation := manager.options.Installation
	if installation == nil || !manager.stateLoaded {
		return false
	}
	if manager.state.ServerID == "" {
		manager.state.ServerID = installation.ServerID
		manager.state.InstallationPublicKey = installation.PublicKey
		return true
	}
	if manager.state.ServerID != installation.ServerID || manager.state.InstallationPublicKey != installation.PublicKey {
		manager.logger.Warn("license_state_other_installation")
	}
	return false
}

// --- La última hora confiable en la base (revisión R2, hallazgos R2-3 y R2-4) -------------------------------------
//
// La última hora confiable vive en estado.json (L1-D9) y tiene una segunda copia en app.license_trusted_time (0028,
// sección «Fase A (revisión R1)»). La que vale es el **máximo** de las dos: editar estado.json no la baja, porque manda
// la de la base, y restaurar un respaldo viejo de la base tampoco, porque manda la de identity/. Solo la §6.3 y la §6.4
// bajan las dos (replaceTrustedTimeLocked).
//
//   - Al arrancar (Load) se lee la copia de la base y se sube la de estado.json hasta ella.
//   - La última actividad de la auditoría (max(audit_events.occurred_at)) sigue siendo la semilla de L1-D15: se usa solo
//     si falta alguna de las dos copias (la primera vez que arranca la 0.15.0, un estado.json dañado o una base de
//     antes de la 0028). Con las dos copias no se vuelve a leer: si no, la corrección de la §6.3 no podría bajarla.
//   - Si la base no responde al arrancar, la lectura queda pendiente y el trabajador la reintenta cada minuto antes de
//     observar el reloj (R2-4). Mientras tanto, el reloj local y las operaciones no se vuelven la primera hora
//     confiable (ObserveTime): la actividad ya registrada podría estar por delante.
//   - Cada minuto (Flush), y al apagar, la copia de la base se sube hasta la de estado.json.

// databaseTimeReading es lo que se leyó de la base para la última hora confiable.
type databaseTimeReading struct {
	copy        TrustedCopy
	copyFound   bool
	latest      time.Time
	latestFound bool
}

// readDatabaseTime lee la copia de la base y, si falta alguna de las dos copias, la última actividad de la auditoría.
// No toca el gestor: Load la llama con el mutex tomado y el trabajador sin él.
func (manager *Manager) readDatabaseTime(ctx context.Context, stateMissing bool) (databaseTimeReading, error) {
	queryCtx, cancel := context.WithTimeout(ctx, databaseTimeout)
	defer cancel()
	var reading databaseTimeReading
	copied, found, err := manager.options.Database.TrustedTimeCopy(queryCtx)
	if err != nil {
		return reading, err
	}
	reading.copy, reading.copyFound = copied, found
	if found && !stateMissing {
		return reading, nil
	}
	latest, present, err := manager.options.Database.LatestActivity(queryCtx)
	if err != nil {
		return reading, err
	}
	reading.latest, reading.latestFound = latest, present
	return reading, nil
}

// applyDatabaseTimeLocked sube la última hora confiable con lo leído de la base (con el mutex tomado). Devuelve si
// cambió.
func (manager *Manager) applyDatabaseTimeLocked(reading databaseTimeReading) bool {
	changed := false
	if reading.copyFound {
		known := databaseInstant(reading.copy.Time)
		manager.databaseTime = &known
		if manager.state.raiseTrustedTime(reading.copy.Time, copySource(reading.copy.Source)) {
			changed = true
		}
	}
	if reading.latestFound && manager.state.raiseTrustedTime(reading.latest, string(TimeFromAudit)) {
		changed = true
	}
	manager.databaseTimeLoaded = true
	return changed
}

// loadDatabaseTimeLocked es la lectura de Load. Si la base no responde, queda pendiente (R2-4): no es un error del
// arranque, solo queda en la bitácora.
func (manager *Manager) loadDatabaseTimeLocked(ctx context.Context) bool {
	if !manager.databaseTimePendingLocked() {
		return false
	}
	reading, err := manager.readDatabaseTime(ctx, manager.state.TrustedTime == nil)
	if err != nil {
		manager.logger.Warn("license_activity_unavailable", "reason", err.Error())
		return false
	}
	return manager.applyDatabaseTimeLocked(reading)
}

// databaseTimePendingLocked indica que hay base y todavía no respondió a la lectura de la última hora confiable.
func (manager *Manager) databaseTimePendingLocked() bool {
	return manager.options.Database != nil && !manager.databaseTimeLoaded
}

// retryDatabaseTime reintenta la lectura pendiente (R2-4). La consulta corre sin el mutex; timeMutex impide que una baja
// de la §6.3 o la §6.4 quede en medio y se pierda al subir con lo leído antes.
func (manager *Manager) retryDatabaseTime(ctx context.Context) {
	manager.timeMutex.Lock()
	defer manager.timeMutex.Unlock()
	manager.mutex.Lock()
	pending := manager.databaseTimePendingLocked()
	stateMissing := manager.state.TrustedTime == nil
	manager.mutex.Unlock()
	if !pending {
		return
	}
	reading, err := manager.readDatabaseTime(ctx, stateMissing)
	if err != nil {
		manager.logger.Warn("license_activity_unavailable", "reason", err.Error())
		return
	}
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.applyDatabaseTimeLocked(reading) {
		manager.dirty = true
	}
	manager.logger.Info("license_trusted_time_seeded")
}

// trustedCopyTargetLocked es lo que falta subir en la copia de la base: la última hora confiable, si va por delante de
// lo que tiene la base. Sin base, o sin haberla podido leer, nada (con el mutex tomado).
func (manager *Manager) trustedCopyTargetLocked() (TrustedCopy, bool) {
	if manager.options.Database == nil || !manager.databaseTimeLoaded || manager.state.TrustedTime == nil {
		return TrustedCopy{}, false
	}
	target := databaseInstant(*manager.state.TrustedTime)
	if manager.databaseTime != nil && !target.After(*manager.databaseTime) {
		return TrustedCopy{}, false
	}
	return TrustedCopy{Time: target, Source: copySource(manager.state.TrustedSource)}, true
}

// saveTrustedCopy sube la copia de la base. Un fallo solo queda en la bitácora: Flush lo reintenta cada minuto. No toma
// ningún mutex.
func (manager *Manager) saveTrustedCopy(ctx context.Context, target TrustedCopy) bool {
	writeCtx, cancel := context.WithTimeout(ctx, databaseTimeout)
	defer cancel()
	if err := manager.options.Database.RaiseTrustedTimeCopy(writeCtx, target); err != nil {
		manager.logger.Warn("license_trusted_copy_not_saved", "reason", err.Error())
		return false
	}
	return true
}

// noteTrustedCopyLocked anota lo que ya tiene la copia de la base después de subirla.
func (manager *Manager) noteTrustedCopyLocked(saved time.Time) {
	if manager.databaseTime == nil || saved.After(*manager.databaseTime) {
		known := saved
		manager.databaseTime = &known
	}
}

// raiseTrustedCopyLocked sube la copia de la base con el mutex tomado (Load y la vuelta atrás de una baja que no se
// guardó).
func (manager *Manager) raiseTrustedCopyLocked(ctx context.Context, target TrustedCopy) {
	if manager.saveTrustedCopy(ctx, target) {
		manager.noteTrustedCopyLocked(target.Time)
	}
}

// replaceTrustedTimeLocked guarda next, un estado con la última hora confiable reemplazada (también hacia atrás), en
// las dos copias: primero la de la base y después estado.json. Si falla la de la base, no cambia nada. Si falla
// estado.json después, la base ya bajó, pero vale el máximo de las dos, así que tampoco cambió nada: la copia de la
// base se vuelve a subir en el momento o, si no se puede, en el siguiente minuto (Flush). Se llama con timeMutex y
// mutex tomados; solo la usan la §6.3 y la §6.4.
func (manager *Manager) replaceTrustedTimeLocked(next storedState) error {
	replaced := *next.TrustedTime
	if manager.options.Database != nil {
		writeCtx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
		err := manager.options.Database.ReplaceTrustedTimeCopy(writeCtx, TrustedCopy{Time: replaced, Source: next.TrustedSource})
		cancel()
		if err != nil {
			manager.logger.Error("license_trusted_copy_not_saved", "reason", err.Error())
			return ErrStateNotSaved
		}
		known := databaseInstant(replaced)
		manager.databaseTime = &known
	}
	if manager.stateLoaded {
		if err := manager.store.write(next, manager.now()); err != nil {
			manager.logger.Error("license_state_not_saved", "reason", err.Error())
			if target, needed := manager.trustedCopyTargetLocked(); needed {
				manager.raiseTrustedCopyLocked(context.Background(), target)
			}
			return ErrStateNotSaved
		}
	}
	manager.state = next
	manager.dirty = false
	// La corrección fija la hora en las dos copias: una semilla pendiente (R2-4) ya no hace falta.
	if manager.options.Database != nil {
		manager.databaseTimeLoaded = true
	}
	return nil
}

// loadLicense vuelve a verificar la licencia guardada con la §10 completa (L1-D20). Prueba primero la de
// estado.json y después la copia de vigente.lic: un archivo alterado, firmado con otra llave o de otra
// instalación se ignora y queda la última válida. Las dos deben ser exactamente la revisión mínima aceptable de su
// licencia, el máximo de las dos copias del historial (estado.json y la base, R1-3). Solo si estado.json faltaba o
// estaba dañado y ninguna de las dos copias conoce la licencia, vigente.lic se acepta como primera revisión.
// Devuelve si el estado cambió.
func (manager *Manager) loadLicense(recoverFromFile bool) bool {
	installation := manager.options.Installation
	if installation == nil || !manager.stateLoaded {
		return false
	}
	verifier := manager.verifier()
	fileJWS, fileExists := manager.store.readLicenseFile()
	changed := false

	var chosen *License
	chosenJWS := ""
	if manager.state.License != "" {
		license, err := verifier.verifyContent(manager.state.License)
		if err == nil && manager.matchesFloor(license) {
			chosen, chosenJWS = license, manager.state.License
		} else {
			manager.logger.Error("license_stored_rejected", "reason", rejectionCode(err, "not_accepted_revision"))
		}
	}
	if chosen == nil && fileExists && fileJWS != "" {
		license, err := verifier.verifyContent(fileJWS)
		switch {
		case err != nil:
			manager.logger.Error("license_file_rejected", "reason", rejectionCode(err, ""))
		case manager.matchesFloor(license):
			chosen, chosenJWS = license, fileJWS
		case recoverFromFile && manager.floor(license.LicenseID) == nil && manager.state.recordAccepted(license) == nil:
			chosen, chosenJWS = license, fileJWS
			manager.logger.Warn("license_recovered_from_file", "license_id", license.LicenseID, "revision", license.LicenseRevision)
		default:
			manager.logger.Error("license_file_rejected", "reason", "not_accepted_revision")
		}
	}

	if chosen == nil {
		// Sin ninguna válida, lo guardado no se borra (sirve a soporte); se ignora hasta que se acepte otra.
		return changed
	}
	// La copia de estado.json se completa con la revisión (si venía de la base o de la recuperación).
	if !manager.state.matchesAccepted(chosen) {
		manager.state.Accepted[chosen.LicenseID] = acceptedRecord{Revision: chosen.LicenseRevision, SHA256: chosen.SHA256}
		changed = true
	}
	if manager.state.License != chosenJWS {
		manager.state.License = chosenJWS
		changed = true
	}
	// vigente.lic es la copia de estado.json: si falta o no coincide, se vuelve a escribir.
	if !fileExists || fileJWS != chosenJWS {
		if fileExists {
			manager.logger.Warn("license_file_restored")
		}
		if err := manager.store.writeLicenseFile(chosenJWS); err != nil {
			manager.logger.Error("license_file_not_saved", "reason", err.Error())
		}
	}
	manager.license = chosen
	if manager.state.raiseTrustedTime(chosen.IssuedAt, string(TimeFromLicense)) {
		changed = true
	}
	return changed
}

// floor es la revisión mínima aceptable de una licencia: la más alta de las dos copias del historial (estado.json y
// la base; revisión R1, hallazgo R1-3). Si se restaura un respaldo viejo de la base, manda la de identity/; si se borra o se
// edita estado.json, manda la de la base. Con la misma revisión en las dos, manda la de identity/.
func (manager *Manager) floor(licenseID string) *Accepted {
	best := manager.state.accepted(licenseID)
	if record, exists := manager.databaseRevisions[licenseID]; exists {
		if best == nil || record.Revision > best.Revision {
			best = &Accepted{LicenseID: licenseID, Revision: record.Revision, SHA256: record.SHA256}
		}
	}
	return best
}

// matchesFloor indica si la licencia es exactamente la revisión mínima aceptable de su license_id.
func (manager *Manager) matchesFloor(license *License) bool {
	floor := manager.floor(license.LicenseID)
	return floor != nil && floor.Revision == license.LicenseRevision && floor.SHA256 == license.SHA256
}

// loadDatabaseRevisions lee la copia del historial de la base. Sin base (pruebas) no hay copia.
func (manager *Manager) loadDatabaseRevisions(ctx context.Context) error {
	if manager.options.Database == nil || manager.databaseLoaded {
		return nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, databaseTimeout)
	defer cancel()
	revisions, err := manager.options.Database.AcceptedRevisions(queryCtx)
	if err != nil {
		manager.logger.Warn("license_revisions_unavailable", "reason", err.Error())
		return errors.New("no se pudo leer la copia del historial de revisiones de la base")
	}
	manager.databaseRevisions = make(map[string]acceptedRecord, len(revisions))
	for licenseID, revision := range revisions {
		manager.databaseRevisions[licenseID] = acceptedRecord{Revision: revision.Revision, SHA256: revision.SHA256}
	}
	manager.databaseLoaded = true
	return nil
}

// ensureStateLocked vuelve a leer el estado del disco si no se pudo leer al arrancar (R1-1). Sin poder leerlo, no se
// escribe nada: ErrStateNotSaved. Lo usan todas las escrituras.
func (manager *Manager) ensureStateLocked() error {
	if manager.options.Installation == nil {
		return ErrIdentityUnavailable
	}
	if manager.stateLoaded {
		return nil
	}
	if !manager.persistent {
		if err := manager.store.ensureDirectory(); err != nil {
			return ErrStateNotSaved
		}
		manager.persistent = true
	}
	state, outcome, err := manager.store.read(manager.now())
	if err != nil {
		return ErrStateNotSaved
	}
	// Lo que se observó en memoria mientras tanto no se pierde.
	if manager.state.TrustedTime != nil {
		state.raiseTrustedTime(*manager.state.TrustedTime, manager.state.TrustedSource)
	}
	manager.state = state
	manager.stateLoaded = true
	manager.recordInstallation()
	manager.loadLicense(outcome != stateRead)
	manager.logger.Info("license_state_recovered")
	return nil
}

// rejectionCode es el motivo estable de un rechazo, para la bitácora (nunca la licencia).
func rejectionCode(err error, fallback string) string {
	var rejected *VerifyError
	if errors.As(err, &rejected) {
		return rejected.Code
	}
	if err != nil {
		return "error"
	}
	return fallback
}

// refreshOperations consulta si la base tiene operaciones. Una vez que las tiene, ya no se vuelve a
// consultar: las operaciones no se borran.
func (manager *Manager) refreshOperations(ctx context.Context) {
	if manager.hasOperations || manager.options.Database == nil {
		return
	}
	found, err := manager.options.Database.HasOperations(ctx)
	if err != nil {
		// Se reintenta en el siguiente minuto (Run). Mientras tanto queda «Sin activar».
		manager.logger.Warn("license_operations_unavailable", "reason", err.Error())
		return
	}
	manager.hasOperations = found
}

func (manager *Manager) verifier() Verifier {
	return Verifier{
		Environment:  manager.trust.Environment,
		Keys:         manager.trust.Keys,
		Installation: *manager.options.Installation,
	}
}

// persist guarda estado.json con la última hora confiable que hubiera en memoria.
func (manager *Manager) persist() error {
	// Sin identidad no se escribe nada (Options.Installation); sin el estado del disco, tampoco (R1-1).
	if manager.options.Installation == nil {
		return nil
	}
	if err := manager.ensureStateLocked(); err != nil {
		return err
	}
	if err := manager.store.write(manager.state, manager.now()); err != nil {
		manager.logger.Error("license_state_not_saved", "reason", err.Error())
		return err
	}
	manager.dirty = false
	return nil
}

// Status es el estado de la licencia en este instante, calculado en memoria.
func (manager *Manager) Status() Status {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	return manager.statusLocked()
}

func (manager *Manager) statusLocked() Status {
	now := manager.now()
	status := Status{
		Environment:         manager.trust.Environment,
		Copied:              manager.copied,
		MachineUnverified:   manager.machineUnverified,
		IdentityUnavailable: manager.options.Installation == nil,
		EffectiveNow:        EffectiveNow(now, manager.state.TrustedTime),
	}
	if manager.state.TrustedTime != nil {
		status.TrustedTime = *manager.state.TrustedTime
	}
	license := manager.license
	// Una licencia de otra computadora o sin huella comprobada se ignora (§1.1 de la especificación).
	if license == nil || manager.copied || manager.machineUnverified || manager.options.Installation == nil {
		status.State = StateNotActivated
		if manager.hasOperations {
			status.State = StateNotActivatedWithData
		}
		return status
	}
	status.State = TemporalState(license, now, manager.state.TrustedTime)
	if status.State == StateRevoked && license.StatusReason != nil {
		status.RevocationReason = *license.StatusReason
	}
	status.LicenseID = license.LicenseID
	status.ActivationID = license.ActivationID
	status.LicenseeName = license.LicenseeName
	status.LicenseType = license.LicenseType
	if license.BillingPeriod != nil {
		status.BillingPeriod = *license.BillingPeriod
	}
	status.Revision = license.LicenseRevision
	status.IssuedAt = license.IssuedAt
	status.PeriodStartsAt = license.PeriodStartsAt
	status.UpdatesUntil = license.UpdatesUntil
	status.Plan = license.Plan
	status.Limits = license.Limits
	status.Features = make(map[string]bool, len(license.Features))
	for key, enabled := range license.Features {
		status.Features[key] = enabled
	}
	status.GraceDays = int(license.GraceDays)
	status.Perpetual = license.ExpiresAt == nil
	if license.ExpiresAt != nil {
		expires := *license.ExpiresAt
		status.ExpiresAt = &expires
		status.GraceEndsAt = GraceLimit(license)
	}
	return status
}

// TrustedTime es la última hora confiable (cero si todavía no hay ninguna).
func (manager *Manager) TrustedTime() time.Time {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.state.TrustedTime == nil {
		return time.Time{}
	}
	return *manager.state.TrustedTime
}

// ObserveTime alimenta la última hora confiable: solo sube. El `server_time` y el `issued_at` se guardan de
// inmediato; el reloj local y las operaciones, en memoria, y Run los guarda cada minuto y al apagar (si se
// corta la luz se pierde a lo más un minuto, dentro del margen de 10 minutos de la §6.1).
func (manager *Manager) ObserveTime(instant time.Time, source TimeSource) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	// Revisión R2, hallazgo R2-4: mientras la base no haya respondido, el reloj local y las operaciones (que llevan la hora
	// del reloj local) no se vuelven la primera hora confiable. La actividad ya registrada podría estar por delante, y las
	// operaciones que se acepten mientras tanto quedan en la auditoría, que es la semilla.
	localSource := source == TimeFromClock || source == TimeFromOperation
	if localSource && manager.state.TrustedTime == nil && manager.databaseTimePendingLocked() {
		return
	}
	if !manager.state.raiseTrustedTime(instant, string(source)) {
		return
	}
	manager.dirty = true
	if source == TimeFromServer || source == TimeFromLicense {
		_ = manager.persist()
	}
}

// ReplaceTrustedTime reemplaza la última hora confiable, también hacia atrás, y la guarda de inmediato en las dos
// copias, la de la base y estado.json (revisión R2, hallazgo R2-3). Devuelve el valor anterior para la auditoría. Solo
// la usan la §6.3 (ReplaceByInternetCheck) y la §6.4 (ReplaceBySupportUnlock); es lo único que la baja. Si no se puede
// guardar, no cambia nada (ErrStateNotSaved). La ruta audita la baja antes de llamarla (R2-1).
func (manager *Manager) ReplaceTrustedTime(instant time.Time, reason ReplaceReason) (time.Time, error) {
	if reason != ReplaceByInternetCheck && reason != ReplaceBySupportUnlock {
		return time.Time{}, errors.New("motivo desconocido para reemplazar la última hora confiable")
	}
	if instant.IsZero() {
		return time.Time{}, errors.New("la nueva hora confiable no es válida")
	}
	manager.timeMutex.Lock()
	defer manager.timeMutex.Unlock()
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	previous := time.Time{}
	if manager.state.TrustedTime != nil {
		previous = *manager.state.TrustedTime
	}
	if manager.options.Installation != nil {
		if err := manager.ensureStateLocked(); err != nil {
			return previous, ErrStateNotSaved
		}
	}
	next := manager.state.clone()
	replaced := instant.UTC()
	next.TrustedTime = &replaced
	next.TrustedSource = string(reason)
	if err := manager.replaceTrustedTimeLocked(next); err != nil {
		return previous, err
	}
	manager.logger.Info("license_trusted_time_replaced", "reason", string(reason), "previous", previous, "replaced", replaced)
	return previous, nil
}

// Accept verifica una licencia con la §10 del contrato y, si es válida, la guarda y pasa a ser la vigente.
// Sirve igual para una respuesta en línea y para un archivo .lic. Una licencia rechazada (*VerifyError) no
// cambia nada: se sigue con la última válida. Devuelve el estado resultante.
func (manager *Manager) Accept(jws string) (Status, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	switch {
	case manager.options.Installation == nil:
		return manager.statusLocked(), ErrIdentityUnavailable
	case manager.copied:
		return manager.statusLocked(), ErrCopiedInstallation
	case manager.machineUnverified:
		return manager.statusLocked(), ErrMachineUnverified
	}
	// Sin conocer las dos copias del historial no se acepta nada (R1-1 y R1-3): se vuelven a leer.
	if err := manager.ensureStateLocked(); err != nil {
		return manager.statusLocked(), err
	}
	if manager.options.Database != nil && !manager.databaseLoaded {
		if err := manager.loadDatabaseRevisions(context.Background()); err != nil {
			return manager.statusLocked(), ErrStateNotSaved
		}
	}
	license, err := manager.verifier().verifyContent(jws)
	if err != nil {
		return manager.statusLocked(), err
	}
	if err := checkRevision(license, manager.floor(license.LicenseID)); err != nil {
		return manager.statusLocked(), err
	}
	if manager.license != nil && manager.license.SHA256 == license.SHA256 {
		// La misma licencia vigente otra vez (por ejemplo, un reintento): no cambia nada.
		return manager.statusLocked(), nil
	}

	// Se prepara el estado nuevo y se guarda antes de cambiar la memoria: si no se puede guardar, la licencia
	// no se acepta (§10.5). Primero la copia de la base y después estado.json (R1-3): si falla la de la base, no se
	// acepta; si falla estado.json después, la base ya tiene la revisión y la misma licencia se puede aceptar otra vez.
	next := manager.state.clone()
	if err := next.recordAccepted(license); err != nil {
		return manager.statusLocked(), err
	}
	next.License = jws
	next.MachineFingerprint = manager.machine
	next.ServerID = manager.options.Installation.ServerID
	next.InstallationPublicKey = manager.options.Installation.PublicKey
	next.raiseTrustedTime(license.IssuedAt, string(TimeFromLicense))
	if manager.options.Database != nil {
		recordCtx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
		err := manager.options.Database.RecordAcceptedRevision(recordCtx, license.LicenseID, license.LicenseRevision, license.SHA256)
		cancel()
		if err != nil {
			manager.logger.Error("license_revision_not_saved", "reason", err.Error())
			return manager.statusLocked(), ErrStateNotSaved
		}
		if manager.databaseRevisions == nil {
			manager.databaseRevisions = map[string]acceptedRecord{}
		}
		if previous, exists := manager.databaseRevisions[license.LicenseID]; !exists || previous.Revision < license.LicenseRevision {
			manager.databaseRevisions[license.LicenseID] = acceptedRecord{Revision: license.LicenseRevision, SHA256: license.SHA256}
		}
	}
	if err := manager.store.write(next, manager.now()); err != nil {
		manager.logger.Error("license_state_not_saved", "reason", err.Error())
		return manager.statusLocked(), ErrStateNotSaved
	}
	manager.state = next
	manager.dirty = false
	manager.license = license
	// La copia para soporte; si falla, se vuelve a escribir al arrancar.
	if err := manager.store.writeLicenseFile(jws); err != nil {
		manager.logger.Error("license_file_not_saved", "reason", err.Error())
	}
	status := manager.statusLocked()
	manager.logger.Info("license_accepted",
		"license_id", license.LicenseID,
		"revision", license.LicenseRevision,
		"license_status", license.LicenseStatus,
		"state", string(status.State),
	)
	return status, nil
}

// LocalClock es el reloj local del gestor (Options.Now o time.Now). La puerta (Fase C) y los cupos
// (Fase D) usan el mismo, para que el reloj simulado de las pruebas sea uno solo (L1-D16).
func (manager *Manager) LocalClock() time.Time {
	return manager.now()
}

// Flush guarda la última hora confiable que quedó en memoria, en estado.json y en la copia de la base (R2-3). La base
// se escribe sin el mutex: Status no la espera.
func (manager *Manager) Flush() error {
	manager.timeMutex.Lock()
	defer manager.timeMutex.Unlock()
	manager.mutex.Lock()
	var err error
	if manager.dirty {
		err = manager.persist()
	}
	target, needed := manager.trustedCopyTargetLocked()
	manager.mutex.Unlock()
	if needed && manager.saveTrustedCopy(context.Background(), target) {
		manager.mutex.Lock()
		manager.noteTrustedCopyLocked(target.Time)
		manager.mutex.Unlock()
	}
	return err
}

// Run es el trabajador del gestor mientras el servidor funciona: cada minuto toma la hora local como hora
// confiable (L1-D15), vuelve a consultar si la base ya tiene operaciones (mientras no las tenga) y guarda; al
// cancelarse el contexto (apagado) guarda por última vez.
func (manager *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = manager.Flush()
			return
		case <-ticker.C:
			manager.tick(ctx)
		}
	}
}

// tick es un paso de Run (las pruebas lo llaman directamente). La consulta de la base corre sin el mutex,
// para que Status (que usa cada operación protegida) nunca espere a la base.
func (manager *Manager) tick(ctx context.Context) {
	// Revisión R2, hallazgo R2-4: si la base no respondió al arrancar, primero se reintenta su lectura (la copia de la hora
	// y la semilla de la auditoría); el reloj se observa después.
	manager.retryDatabaseTime(ctx)
	manager.ObserveTime(manager.now(), TimeFromClock)
	manager.mutex.Lock()
	needsCheck := !manager.hasOperations && manager.options.Database != nil
	manager.mutex.Unlock()
	if needsCheck {
		found, err := manager.options.Database.HasOperations(ctx)
		if err != nil {
			manager.logger.Warn("license_operations_unavailable", "reason", err.Error())
		} else if found {
			manager.mutex.Lock()
			manager.hasOperations = true
			manager.mutex.Unlock()
		}
	}
	_ = manager.Flush()
}

// --- Fase B: lo que el cliente en línea, los .licreq y la identidad nueva necesitan del gestor --------------------

// signingIdentity es la identidad con que se firma una prueba de posesión o un .licreq. No se firma nada sin
// identidad, con la instalación copiada (L1-D11: antes hay que crear una identidad nueva) ni sin huella comprobada.
func (manager *Manager) signingIdentity() (Installation, ed25519.PrivateKey, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	switch {
	case manager.options.Installation == nil || len(manager.options.PrivateKey) != ed25519.PrivateKeySize:
		return Installation{}, nil, ErrIdentityUnavailable
	case manager.copied:
		return Installation{}, nil, ErrCopiedInstallation
	case manager.machineUnverified:
		return Installation{}, nil, ErrMachineUnverified
	}
	return *manager.options.Installation, manager.options.PrivateKey, nil
}

// installation es la identidad cargada, aunque no se pueda firmar con ella.
func (manager *Manager) installation() (Installation, bool) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.options.Installation == nil {
		return Installation{}, false
	}
	return *manager.options.Installation, true
}

// currentActivation es el activation_id de la licencia instalada que se usa (también una revisión de cierre). Falso
// sin licencia válida o con la licencia ignorada (copia o huella sin comprobar).
func (manager *Manager) currentActivation() (string, bool) {
	_, activationID, ok := manager.currentLicenseIDs()
	return activationID, ok
}

func (manager *Manager) currentLicenseIDs() (string, string, bool) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.license == nil || manager.copied || manager.machineUnverified || manager.options.Installation == nil {
		return "", "", false
	}
	return manager.license.LicenseID, manager.license.ActivationID, true
}

// ServerID es el server_id de la identidad cargada (vacío sin identidad).
func (manager *Manager) ServerID() string {
	installation, _ := manager.installation()
	return installation.ServerID
}

// Directory es la carpeta identity/licencia del gestor.
func (manager *Manager) Directory() string {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	return manager.store.directory
}

// SwitchInstallation pasa el gestor a una identidad nueva (license/identity/regenerate, L1-D11), sin reiniciar:
// otra carpeta de estado, sin licencia, con la huella de este equipo y **la misma última hora confiable** (L1-D25:
// crear una identidad nueva no reinicia la protección de la hora). La carpeta anterior ya se apartó con la identidad
// anterior y no se vuelve a usar.
func (manager *Manager) SwitchInstallation(directory string, installation Installation, private ed25519.PrivateKey) error {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.machine == "" {
		return ErrMachineUnverified
	}
	// La identidad nueva trae su registro firmado de la huella de este equipo (localsetup.EnsureLicenseKey).
	if recorded, problem := readMachineRecord(filepath.Dir(directory), installation); problem != "" || recorded != manager.machine {
		return ErrCopiedInstallation
	}
	store := stateStore{directory: directory}
	if err := store.ensureDirectory(); err != nil {
		return err
	}
	next := newStoredState()
	next.ServerID = installation.ServerID
	next.InstallationPublicKey = installation.PublicKey
	next.MachineFingerprint = manager.machine
	if manager.state.TrustedTime != nil {
		next.raiseTrustedTime(*manager.state.TrustedTime, manager.state.TrustedSource)
	}
	if err := store.write(next, manager.now()); err != nil {
		return ErrStateNotSaved
	}
	identity := installation
	manager.options.Installation = &identity
	manager.options.PrivateKey = private
	manager.options.IdentityProblem = ""
	manager.options.Directory = directory
	manager.store = store
	manager.state = next
	manager.dirty = false
	manager.persistent = true
	manager.stateLoaded = true
	manager.license = nil
	manager.copied = false
	manager.logger.Info("license_installation_switched", "server_id", installation.ServerID)
	return nil
}

// CurrentMachine es la huella de este equipo que se leyó al arrancar (falso si no se pudo leer). La identidad nueva
// de una instalación copiada firma con ella su registro (license/identity/regenerate).
func (manager *Manager) CurrentMachine() (string, bool) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	return manager.machine, manager.machine != ""
}

// SigningProblem dice por qué no se puede firmar con la identidad (sin identidad, copiada o sin huella), o nil. Las
// rutas lo comprueban antes de ir a la red.
func (manager *Manager) SigningProblem() error {
	_, _, err := manager.signingIdentity()
	return err
}
