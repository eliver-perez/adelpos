package licensing

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"log/slog"
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
	// falso si no hay ninguna. Solo inicia la última hora confiable cuando todavía no existe.
	LatestActivity(ctx context.Context) (time.Time, bool, error)
}

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
	// Database es la consulta de la base. Sin valor, la base se considera sin operaciones y la última hora
	// confiable no se inicia con la auditoría.
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

	mutex sync.Mutex
	// state es lo guardado en estado.json, más la última hora confiable todavía sin guardar (dirty).
	state storedState
	dirty bool
	// persistent indica que hay identidad y la carpeta se pudo preparar: solo entonces se escribe.
	persistent bool
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
			// Sin poder leerlo (o apartarlo), no se escribe nada: el gestor sigue en memoria.
			problems = append(problems, err)
			manager.persistent = false
		}
		manager.state = state
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

	if manager.checkMachine() {
		changed = true
	}
	if manager.recordInstallation() {
		changed = true
	}
	if manager.seedTrustedTime(ctx) {
		changed = true
	}
	if manager.loadLicense(recoverFromFile) {
		changed = true
	}
	manager.refreshOperations(ctx)

	if changed || manager.dirty {
		if err := manager.persist(); err != nil {
			problems = append(problems, err)
		}
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

// checkMachine lee la huella del equipo (tres intentos) y la compara con la guardada. La primera vez que
// arranca esta versión la registra. Devuelve si el estado cambió.
func (manager *Manager) checkMachine() bool {
	delay := manager.options.MachineRetryDelay
	if delay == 0 {
		delay = 200 * time.Millisecond
	}
	fingerprint, err := readFingerprint(manager.machineID, delay)
	if err != nil {
		// Falla cerrada (L1-D10): la licencia guardada se ignora hasta que la huella se pueda leer.
		manager.machineUnverified = true
		manager.logger.Error("license_machine_unverified", "attempts", machineAttempts)
		return false
	}
	manager.machine = fingerprint
	if manager.state.MachineFingerprint == "" {
		manager.state.MachineFingerprint = fingerprint
		return true
	}
	if manager.state.MachineFingerprint != fingerprint {
		// La carpeta de datos se copió de otra computadora (o cambió la tarjeta madre): la licencia guardada se
		// ignora, sin borrarla. Para activar, primero se crea una identidad nueva (Fase B, L1-D11).
		manager.copied = true
		manager.logger.Warn("license_installation_copied")
	}
	return false
}

// recordInstallation anota en el estado la identidad a la que pertenece. Si el estado es de otra identidad,
// solo se avisa: las licencias se comprueban siempre contra la identidad cargada.
func (manager *Manager) recordInstallation() bool {
	installation := manager.options.Installation
	if installation == nil || !manager.persistent {
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

// seedTrustedTime inicia la última hora confiable con la última actividad de la base, solo si todavía no
// existe (L1-D15, Duda 16): si se leyera siempre, la corrección de la §6.3 no podría bajarla.
func (manager *Manager) seedTrustedTime(ctx context.Context) bool {
	if manager.state.TrustedTime != nil || manager.options.Database == nil {
		return false
	}
	latest, found, err := manager.options.Database.LatestActivity(ctx)
	if err != nil {
		manager.logger.Warn("license_activity_unavailable", "reason", err.Error())
		return false
	}
	if !found {
		return false
	}
	return manager.state.raiseTrustedTime(latest, string(TimeFromAudit))
}

// loadLicense vuelve a verificar la licencia guardada con la §10 completa (L1-D20). Prueba primero la de
// estado.json y después la copia de vigente.lic: un archivo alterado, firmado con otra llave o de otra
// instalación se ignora y queda la última válida. Las dos deben ser exactamente la revisión aceptada; solo si
// estado.json faltaba o estaba dañado, vigente.lic se acepta como primera revisión (recuperación).
// Devuelve si el estado cambió.
func (manager *Manager) loadLicense(recoverFromFile bool) bool {
	installation := manager.options.Installation
	if installation == nil || !manager.persistent {
		return false
	}
	verifier := manager.verifier()
	fileJWS, fileExists := manager.store.readLicenseFile()
	changed := false

	var chosen *License
	chosenJWS := ""
	if manager.state.License != "" {
		license, err := verifier.verifyContent(manager.state.License)
		if err == nil && manager.state.matchesAccepted(license) {
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
		case manager.state.matchesAccepted(license):
			chosen, chosenJWS = license, fileJWS
		case recoverFromFile && manager.state.recordAccepted(license) == nil:
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
	if !manager.persistent {
		return nil
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
	if !manager.state.raiseTrustedTime(instant, string(source)) {
		return
	}
	manager.dirty = true
	if source == TimeFromServer || source == TimeFromLicense {
		_ = manager.persist()
	}
}

// ReplaceTrustedTime reemplaza la última hora confiable, también hacia atrás, y la guarda de inmediato.
// Devuelve el valor anterior para la auditoría. Solo la usan la §6.3 (ReplaceByInternetCheck) y la §6.4
// (ReplaceBySupportUnlock); es lo único que la baja. Si no se puede guardar, no cambia nada.
func (manager *Manager) ReplaceTrustedTime(instant time.Time, reason ReplaceReason) (time.Time, error) {
	if reason != ReplaceByInternetCheck && reason != ReplaceBySupportUnlock {
		return time.Time{}, errors.New("motivo desconocido para reemplazar la última hora confiable")
	}
	if instant.IsZero() {
		return time.Time{}, errors.New("la nueva hora confiable no es válida")
	}
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	previous := time.Time{}
	if manager.state.TrustedTime != nil {
		previous = *manager.state.TrustedTime
	}
	next := manager.state.clone()
	replaced := instant.UTC()
	next.TrustedTime = &replaced
	next.TrustedSource = string(reason)
	if manager.persistent {
		if err := manager.store.write(next, manager.now()); err != nil {
			manager.logger.Error("license_state_not_saved", "reason", err.Error())
			return previous, ErrStateNotSaved
		}
	}
	manager.state = next
	manager.dirty = false
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
	license, err := manager.verifier().verifyContent(jws)
	if err != nil {
		return manager.statusLocked(), err
	}
	if err := checkRevision(license, manager.state.accepted(license.LicenseID)); err != nil {
		return manager.statusLocked(), err
	}
	if manager.license != nil && manager.license.SHA256 == license.SHA256 {
		// La misma licencia vigente otra vez (por ejemplo, un reintento): no cambia nada.
		return manager.statusLocked(), nil
	}

	// Se prepara el estado nuevo y se guarda antes de cambiar la memoria: si no se puede guardar, la licencia
	// no se acepta (§10.5).
	next := manager.state.clone()
	if err := next.recordAccepted(license); err != nil {
		return manager.statusLocked(), err
	}
	next.License = jws
	next.MachineFingerprint = manager.machine
	next.ServerID = manager.options.Installation.ServerID
	next.InstallationPublicKey = manager.options.Installation.PublicKey
	next.raiseTrustedTime(license.IssuedAt, string(TimeFromLicense))
	if !manager.persistent {
		if err := manager.store.ensureDirectory(); err != nil {
			return manager.statusLocked(), ErrStateNotSaved
		}
		manager.persistent = true
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

// Flush guarda la última hora confiable que quedó en memoria.
func (manager *Manager) Flush() error {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if !manager.dirty {
		return nil
	}
	return manager.persist()
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
