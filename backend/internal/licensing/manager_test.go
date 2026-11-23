package licensing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Identificadores de equipo simulados: nunca el de la computadora donde corren las pruebas.
const (
	machineOriginal = "AAAAAAAA-1111-2222-3333-444444444444"
	machineOther    = "BBBBBBBB-5555-6666-7777-888888888888"
)

// testClock es un reloj simulado que las pruebas mueven.
type testClock struct {
	mutex   sync.Mutex
	current time.Time
}

func newTestClock(instant string) *testClock {
	parsed, err := time.Parse(time.RFC3339Nano, instant)
	if err != nil {
		panic(err)
	}
	return &testClock{current: parsed}
}

func (clock *testClock) now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.current
}

func (clock *testClock) set(instant string) {
	parsed, err := time.Parse(time.RFC3339Nano, instant)
	if err != nil {
		panic(err)
	}
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.current = parsed
}

// fakeDatabase simula la consulta de la base, con la copia del historial de revisiones (R1-3).
type fakeDatabase struct {
	mutex          sync.Mutex
	operations     bool
	operationCalls int
	latest         time.Time
	activityCalls  int
	failure        error
	revisions      map[string]AcceptedRevision
	revisionFail   error
}

func (database *fakeDatabase) AcceptedRevisions(context.Context) (map[string]AcceptedRevision, error) {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	if database.revisionFail != nil {
		return nil, database.revisionFail
	}
	copied := map[string]AcceptedRevision{}
	for licenseID, revision := range database.revisions {
		copied[licenseID] = revision
	}
	return copied, nil
}

func (database *fakeDatabase) RecordAcceptedRevision(_ context.Context, licenseID string, revision int64, sha256 string) error {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	if database.revisionFail != nil {
		return database.revisionFail
	}
	if database.revisions == nil {
		database.revisions = map[string]AcceptedRevision{}
	}
	if previous, exists := database.revisions[licenseID]; !exists || previous.Revision < revision {
		database.revisions[licenseID] = AcceptedRevision{Revision: revision, SHA256: sha256}
	}
	return nil
}

func (database *fakeDatabase) HasOperations(context.Context) (bool, error) {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	database.operationCalls++
	return database.operations, database.failure
}

func (database *fakeDatabase) LatestActivity(context.Context) (time.Time, bool, error) {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	database.activityCalls++
	return database.latest, !database.latest.IsZero(), database.failure
}

func (database *fakeDatabase) setOperations(value bool) {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	database.operations = value
}

// harness arma gestores sobre la misma carpeta, como reinicios del servidor.
type harness struct {
	t         *testing.T
	fixtures  fixtures
	directory string
	clock     *testClock
	machine   string
	database  *fakeDatabase
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		t:         t,
		fixtures:  loadFixtures(t),
		directory: filepath.Join(t.TempDir(), "identity", "licencia"),
		clock:     newTestClock("2026-11-21T12:00:00Z"),
		machine:   machineOriginal,
		database:  &fakeDatabase{},
	}
	// El registro firmado de la huella de la computadora original, como lo crea la llave de licencias (R1-2).
	writeTestMachineRecord(t, h.fixtures, filepath.Dir(h.directory), "installation-fixture-a", machineOriginal)
	return h
}

// writeTestMachineRecord firma el registro de la huella de una instalación de los archivos de prueba.
func writeTestMachineRecord(t *testing.T, loaded fixtures, identityDir, installation, machineID string) {
	t.Helper()
	if err := os.MkdirAll(identityDir, 0700); err != nil {
		t.Fatal(err)
	}
	normalized, err := normalizeMachineID(machineID)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteMachineRecord(identityDir, loaded.privateKeys[installation], loaded.installations[installation].ServerID, MachineFingerprint(normalized), time.Now()); err != nil {
		t.Fatal(err)
	}
}

// start crea un gestor nuevo (un arranque) para la instalación A de los archivos de prueba.
func (h *harness) start() *Manager {
	h.t.Helper()
	installation := h.fixtures.installations["installation-fixture-a"]
	manager := NewManager(Options{
		Directory:    h.directory,
		Installation: &installation,
		Trust:        h.fixtures.testTrust(),
		Now:          h.clock.now,
		MachineID: func() (string, error) {
			return h.machine, nil
		},
		Database: h.database,
	})
	if err := manager.Load(context.Background()); err != nil {
		h.t.Fatalf("Load: %v", err)
	}
	return manager
}

func (h *harness) jws(name string) string {
	return h.fixtures.license(h.t, name).JWS
}

func (h *harness) negative(name string) string {
	h.t.Helper()
	for _, item := range h.fixtures.set.Negative {
		if item.Name == name {
			return item.JWS
		}
	}
	h.t.Fatalf("no existe el caso negativo %s", name)
	return ""
}

func (h *harness) accept(manager *Manager, name string) Status {
	h.t.Helper()
	status, err := manager.Accept(h.jws(name))
	if err != nil {
		h.t.Fatalf("Accept(%s): %v", name, err)
	}
	return status
}

func (h *harness) readState() storedState {
	h.t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.directory, stateFileName))
	if err != nil {
		h.t.Fatal(err)
	}
	var state storedState
	if err := json.Unmarshal(raw, &state); err != nil {
		h.t.Fatal(err)
	}
	return state
}

func (h *harness) writeState(state storedState) {
	h.t.Helper()
	body, err := json.Marshal(state)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.directory, stateFileName), body, 0600); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) licenseFile() string {
	h.t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.directory, licenseFileName))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(raw)
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	if codeOf(err) != code {
		t.Fatalf("se esperaba %s y resultó %v", code, err)
	}
}

// La licencia aceptada sobrevive a un reinicio, con su estado, sus cupos y la última hora confiable.
func TestManagerPersistsTheAcceptedLicense(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	if status := manager.Status(); status.State != StateNotActivated || status.HasLicense() || status.Limits != (Limits{}) {
		t.Fatalf("una instalación nueva está sin activar y sin cupos: %+v", status)
	}
	status := h.accept(manager, "subscription-monthly")
	if status.State != StateActive || !status.HasLicense() || status.Environment != "test" {
		t.Fatalf("la licencia aceptada está vigente: %+v", status)
	}
	if status.Limits != (Limits{CashRegisters: 3, OrderTakers: 3, KitchenDisplays: 3}) || status.Plan.PlanID == "" {
		t.Fatalf("cupos y plan de la licencia: %+v", status)
	}
	if status.LicenseType != "subscription" || status.BillingPeriod != "monthly" || status.Perpetual || status.GraceDays != 5 {
		t.Fatalf("tipo y periodo: %+v", status)
	}
	if status.ExpiresAt == nil || status.GraceEndsAt == nil || status.GraceEndsAt.Sub(*status.ExpiresAt) != 5*24*time.Hour {
		t.Fatalf("vencimiento y fin de la tolerancia: %+v", status)
	}
	// El issued_at alimenta la última hora confiable.
	if status.TrustedTime.Before(status.IssuedAt) {
		t.Fatalf("la última hora confiable incluye el issued_at: %+v", status)
	}
	if h.licenseFile() != h.jws("subscription-monthly") {
		t.Fatal("vigente.lic es la licencia tal como viaja")
	}

	restarted := h.start()
	again := restarted.Status()
	if again.State != StateActive || again.LicenseID != status.LicenseID || again.Revision != status.Revision {
		t.Fatalf("después de reiniciar sigue la misma licencia: %+v", again)
	}
	if !again.TrustedTime.Equal(status.TrustedTime) {
		t.Fatal("la última hora confiable se conserva")
	}
	requirePrivateStore(t, h.directory)
}

// Un corte en medio de una escritura nunca deja un estado a medias: los temporales se descartan, un corte
// entre estado.json y vigente.lic se repara, y un estado.json dañado se aparta y se recupera la licencia.
func TestManagerSurvivesAnInterruptedWrite(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	h.accept(manager, "subscription-monthly")

	// 1. Temporales de una escritura interrumpida (el renombrado nunca ocurrió).
	for _, name := range []string{"." + stateFileName + "-123", "." + licenseFileName + "-456"} {
		if err := os.WriteFile(filepath.Join(h.directory, name), []byte(`{"format":1,"accep`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	status := h.start().Status()
	if status.State != StateActive || status.Revision != 1 {
		t.Fatalf("los temporales no cambian nada: %+v", status)
	}
	entries, _ := os.ReadDir(h.directory)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			t.Fatalf("el temporal %s se descarta", entry.Name())
		}
	}

	// 2. Corte entre las dos escrituras de Accept: estado.json ya tiene la revisión 2 y vigente.lic sigue con la 1.
	manager = h.start()
	h.accept(manager, "closing-deactivated")
	if err := os.WriteFile(filepath.Join(h.directory, licenseFileName), []byte(h.jws("subscription-monthly")), 0600); err != nil {
		t.Fatal(err)
	}
	status = h.start().Status()
	if status.State != StateRevoked || status.Revision != 2 || status.RevocationReason != ReasonDeactivated {
		t.Fatalf("manda estado.json: %+v", status)
	}
	if h.licenseFile() != h.jws("closing-deactivated") {
		t.Fatal("vigente.lic se vuelve a escribir desde estado.json")
	}

	// 3. estado.json a medias (como si no fuera atómico): se aparta y se recupera la licencia de vigente.lic.
	raw, err := os.ReadFile(filepath.Join(h.directory, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.directory, stateFileName), raw[:len(raw)/2], 0600); err != nil {
		t.Fatal(err)
	}
	status = h.start().Status()
	if status.State != StateRevoked || status.Revision != 2 {
		t.Fatalf("la licencia se recupera de vigente.lic: %+v", status)
	}
	matches, _ := filepath.Glob(filepath.Join(h.directory, stateFileName+".danado-*"))
	if len(matches) != 1 {
		t.Fatalf("el estado dañado se aparta para soporte: %v", matches)
	}
	if record := h.readState().Accepted[status.LicenseID]; record.Revision != 2 {
		t.Fatalf("la revisión aceptada se reconstruye: %+v", record)
	}
}

// La revisión de una licencia nunca retrocede, ni en el verificador ni en el almacén, y tampoco después de
// reiniciar.
func TestManagerRevisionNeverGoesBack(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	h.accept(manager, "subscription-annual")
	status := h.accept(manager, "renewed-subscription")
	if status.Revision != 2 || status.LicenseType != "subscription" {
		t.Fatalf("una revisión mayor de la misma licencia se acepta: %+v", status)
	}
	// Menor: se rechaza y queda la última.
	_, err := manager.Accept(h.jws("subscription-annual"))
	requireCode(t, err, ErrStaleRevision)
	// Igual con otro contenido (el mismo contenido firmado con la otra llave de confianza): se rechaza.
	renewed := h.fixtures.license(t, "renewed-subscription")
	var content map[string]any
	if err := json.Unmarshal(renewed.Content, &content); err != nil {
		t.Fatal(err)
	}
	contentJSON, _ := json.Marshal(content)
	resigned := h.fixtures.sign(t, "adelpos-test-fixture-b", contentJSON)
	if _, err := h.fixtures.verifier("test", "installation-fixture-a").Verify(resigned, nil); err != nil {
		t.Fatalf("la licencia firmada de nuevo es válida por sí misma: %v", err)
	}
	_, err = manager.Accept(resigned)
	requireCode(t, err, ErrStaleRevision)
	if again := manager.Status(); again.Revision != 2 || again.LicenseID != status.LicenseID {
		t.Fatalf("sigue la revisión 2: %+v", again)
	}
	// La misma revisión, idéntica byte por byte, se acepta sin cambiar nada.
	if same, err := manager.Accept(h.jws("renewed-subscription")); err != nil || same.Revision != 2 {
		t.Fatalf("la misma licencia otra vez: %v", err)
	}

	// Otra licencia es independiente (y con menos cupos se acepta igual), y la revisión más alta de la primera
	// se conserva aunque deje de ser la vigente.
	other := h.accept(manager, "scheduled-reduction-applied")
	if other.LicenseID == status.LicenseID || other.Revision != 3 || other.Limits != (Limits{CashRegisters: 1}) {
		t.Fatalf("otra licencia, revisión 3, con menos cupos: %+v", other)
	}
	restarted := h.start()
	_, err = restarted.Accept(h.jws("subscription-annual"))
	requireCode(t, err, ErrStaleRevision)
	state := h.readState()
	if state.Accepted[status.LicenseID].Revision != 2 || state.Accepted[other.LicenseID].Revision != 3 {
		t.Fatalf("revisiones aceptadas por licencia: %+v", state.Accepted)
	}

	// El almacén por sí mismo tampoco retrocede.
	lower, err := restarted.verifier().verifyContent(h.jws("subscription-annual"))
	if err != nil {
		t.Fatal(err)
	}
	if err := state.recordAccepted(lower); codeOf(err) != ErrStaleRevision {
		t.Fatalf("el almacén no guarda una revisión menor: %v", err)
	}
	if state.Accepted[status.LicenseID].Revision != 2 {
		t.Fatal("la revisión guardada no cambia")
	}
}

// Un archivo alterado, de una llave no confiable o de otra instalación se ignora y queda la última licencia
// válida: al aceptarlo y también si aparece en el disco.
func TestManagerIgnoresAlteredForeignAndOtherInstallationFiles(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	valid := h.accept(manager, "subscription-monthly")
	stateBefore, _ := os.ReadFile(filepath.Join(h.directory, stateFileName))

	rejected := map[string]string{
		"altered-signature": ErrInvalidSignature,
		"unknown-kid":       ErrUnknownKid,
		"other-product":     ErrOtherProduct,
	}
	for name, code := range rejected {
		_, err := manager.Accept(h.negative(name))
		requireCode(t, err, code)
	}
	_, err := manager.Accept(h.jws("self-service-transfer-new"))
	requireCode(t, err, ErrOtherInstallation)
	if status := manager.Status(); status.LicenseID != valid.LicenseID || status.State != StateActive {
		t.Fatalf("sigue la última válida: %+v", status)
	}
	if stateAfter, _ := os.ReadFile(filepath.Join(h.directory, stateFileName)); string(stateAfter) != string(stateBefore) {
		t.Fatal("un rechazo no escribe nada")
	}

	// En el disco: vigente.lic reemplazado por uno alterado, de otra llave o de otra instalación.
	for _, foreign := range []string{h.negative("altered-signature"), h.negative("unknown-kid"), h.jws("self-service-transfer-new")} {
		if err := os.WriteFile(filepath.Join(h.directory, licenseFileName), []byte(foreign), 0600); err != nil {
			t.Fatal(err)
		}
		if status := h.start().Status(); status.LicenseID != valid.LicenseID || status.State != StateActive {
			t.Fatalf("un vigente.lic ajeno se ignora: %+v", status)
		}
		if h.licenseFile() != h.jws("subscription-monthly") {
			t.Fatal("vigente.lic vuelve a ser la licencia válida")
		}
	}

	// La licencia de estado.json alterada: queda la copia válida de vigente.lic, que es la revisión aceptada.
	state := h.readState()
	state.License = h.negative("altered-signature")
	h.writeState(state)
	if status := h.start().Status(); status.LicenseID != valid.LicenseID || status.State != StateActive {
		t.Fatalf("se usa la copia válida: %+v", status)
	}
	if h.readState().License != h.jws("subscription-monthly") {
		t.Fatal("estado.json vuelve a tener la licencia válida")
	}

	// Las dos alteradas: no queda ninguna licencia válida (sin borrar las revisiones aceptadas).
	state = h.readState()
	state.License = h.negative("altered-content")
	h.writeState(state)
	if err := os.WriteFile(filepath.Join(h.directory, licenseFileName), []byte(h.negative("altered-signature")), 0600); err != nil {
		t.Fatal(err)
	}
	if status := h.start().Status(); status.State != StateNotActivated || status.HasLicense() {
		t.Fatalf("sin ninguna válida queda sin activar: %+v", status)
	}
	if state := h.readState(); state.Accepted[valid.LicenseID].Revision != 1 || state.License != h.negative("altered-content") {
		t.Fatal("lo guardado no se borra: las revisiones aceptadas siguen y la licencia ignorada queda para soporte")
	}
	// Importar otra vez el .lic válido (la misma revisión, idéntica) la vuelve a instalar.
	reimported := h.start()
	if status := h.accept(reimported, "subscription-monthly"); status.State != StateActive || status.LicenseID != valid.LicenseID {
		t.Fatalf("el .lic válido se vuelve a instalar: %+v", status)
	}
	if h.readState().License != h.jws("subscription-monthly") || h.licenseFile() != h.jws("subscription-monthly") {
		t.Fatal("los dos archivos vuelven a tener la licencia válida")
	}
}

// Con otra huella del equipo la licencia guardada se ignora: «Sin activar con datos» con Copied, o «Sin
// activar» con Copied si la base no tiene operaciones. Activar exige antes una identidad nueva.
func TestManagerDetectsACopiedInstallation(t *testing.T) {
	h := newHarness(t)
	h.database.setOperations(true)
	manager := h.start()
	h.accept(manager, "subscription-monthly")
	if h.readState().MachineFingerprint != MachineFingerprint(strings.ToLower(machineOriginal)) {
		t.Fatal("la huella guardada es la del equipo donde se aceptó")
	}
	if strings.Contains(string(mustRead(t, filepath.Join(h.directory, stateFileName))), strings.ToLower(machineOriginal)) {
		t.Fatal("el identificador del equipo nunca se guarda, solo su huella")
	}

	h.machine = machineOther
	copied := h.start()
	status := copied.Status()
	if status.State != StateNotActivatedWithData || !status.Copied || status.HasLicense() || status.Limits != (Limits{}) || status.LicenseID != "" {
		t.Fatalf("la licencia de otra computadora se ignora: %+v", status)
	}
	if _, err := copied.Accept(h.jws("subscription-monthly")); !errors.Is(err, ErrCopiedInstallation) {
		t.Fatalf("no se activa con la identidad copiada: %v", err)
	}
	if h.readState().License != h.jws("subscription-monthly") {
		t.Fatal("la licencia ignorada no se borra")
	}

	h.database.setOperations(false)
	if status := h.start().Status(); status.State != StateNotActivated || !status.Copied {
		t.Fatalf("sin operaciones queda sin activar, con Copied: %+v", status)
	}

	// De vuelta en la computadora original, la licencia sigue.
	h.machine = machineOriginal
	if status := h.start().Status(); status.State != StateActive || status.Copied {
		t.Fatalf("en la computadora original sigue vigente: %+v", status)
	}
}

// Si la huella no se puede leer tras tres intentos, falla cerrada: se trata como distinta, con su propio
// indicador.
func TestManagerFailsClosedWithoutMachineFingerprint(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	h.accept(manager, "subscription-monthly")

	attempts := 0
	installation := h.fixtures.installations["installation-fixture-a"]
	broken := NewManager(Options{
		Directory:    h.directory,
		Installation: &installation,
		Trust:        h.fixtures.testTrust(),
		Now:          h.clock.now,
		MachineID: func() (string, error) {
			attempts++
			return "", errors.New("sin ioreg")
		},
		MachineRetryDelay: time.Millisecond,
	})
	if err := broken.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	status := broken.Status()
	if attempts != 3 || status.State != StateNotActivated || !status.MachineUnverified || status.Copied {
		t.Fatalf("tres intentos y falla cerrada: %d %+v", attempts, status)
	}
	if _, err := broken.Accept(h.jws("subscription-monthly")); !errors.Is(err, ErrMachineUnverified) {
		t.Fatalf("no se acepta una licencia sin huella: %v", err)
	}
	// Un valor que no es un identificador tampoco vale.
	garbage := NewManager(Options{
		Directory:    h.directory,
		Installation: &installation,
		Trust:        h.fixtures.testTrust(),
		MachineID: func() (string, error) {
			return "no es un identificador", nil
		},
		MachineRetryDelay: time.Millisecond,
	})
	_ = garbage.Load(context.Background())
	if !garbage.Status().MachineUnverified {
		t.Fatal("un identificador inválido falla cerrado")
	}
}

// Los estados de la §5.6 en el gestor: vigente, tolerancia y vencida en sus límites exactos, perpetua y
// revocada con su motivo; atrasar el reloj nunca prolonga la licencia.
func TestManagerTemporalStates(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	h.accept(manager, "subscription-monthly")
	cases := []struct {
		clock string
		state State
	}{
		{"2026-12-16T06:29:59.999999Z", StateActive},
		{"2026-12-16T06:30:00Z", StateGrace},
		{"2026-12-21T06:29:59.999999Z", StateGrace},
		{"2026-12-21T06:30:00Z", StateExpired},
	}
	for _, item := range cases {
		h.clock.set(item.clock)
		if status := manager.Status(); status.State != item.state {
			t.Fatalf("%s: se esperaba %s y resultó %s", item.clock, item.state, status.State)
		}
	}
	// El reloj atrasado no la hace vigente otra vez: la última hora confiable ya pasó el vencimiento.
	manager.ObserveTime(h.clock.now(), TimeFromClock)
	h.clock.set("2026-11-25T00:00:00Z")
	if status := manager.Status(); status.State != StateExpired || !status.EffectiveNow.Equal(status.TrustedTime) {
		t.Fatalf("atrasar el reloj no prolonga la licencia: %+v", status)
	}

	// Perpetua: vigente diez años después.
	other := newHarness(t)
	perpetual := other.start()
	other.accept(perpetual, "perpetual-0-months")
	other.clock.set("2036-11-16T06:30:00Z")
	if status := perpetual.Status(); status.State != StateActive || !status.Perpetual || status.ExpiresAt != nil || status.GraceEndsAt != nil {
		t.Fatalf("perpetua: %+v", status)
	}
	// Revocada con su motivo (el cierre `deactivated` lo prueba TestManagerSurvivesAnInterruptedWrite).
	status := other.accept(perpetual, "closing-transferred")
	if status.State != StateRevoked || status.RevocationReason != ReasonTransferred || !status.HasLicense() {
		t.Fatalf("cierre por transferencia: %+v", status)
	}
}

// La última hora confiable solo sube con ObserveTime, se guarda (de inmediato el server_time; cada minuto el
// reloj y las operaciones) y solo ReplaceTrustedTime la baja, con un motivo permitido.
func TestManagerTrustedTime(t *testing.T) {
	h := newHarness(t)
	h.database.latest = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	manager := h.start()
	if !manager.TrustedTime().Equal(h.database.latest) || h.readState().TrustedSource != string(TimeFromAudit) {
		t.Fatalf("sin hora confiable, la inicia la última actividad de la base: %v", manager.TrustedTime())
	}

	later := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	manager.ObserveTime(later, TimeFromServer)
	manager.ObserveTime(later.Add(-time.Hour), TimeFromServer)
	if !manager.TrustedTime().Equal(later) {
		t.Fatal("la última hora confiable nunca baja con ObserveTime")
	}
	if saved := h.readState().TrustedTime; saved == nil || !saved.Equal(later) {
		t.Fatal("el server_time se guarda de inmediato")
	}

	// El reloj y las operaciones quedan en memoria hasta el siguiente minuto (Run) o el apagado.
	operation := later.Add(30 * time.Second)
	manager.ObserveTime(operation, TimeFromOperation)
	if saved := h.readState().TrustedTime; !saved.Equal(later) {
		t.Fatal("una operación no escribe el disco cada vez")
	}
	if err := manager.Flush(); err != nil {
		t.Fatal(err)
	}
	if saved := h.readState().TrustedTime; !saved.Equal(operation) {
		t.Fatal("Flush guarda la hora pendiente")
	}
	h.clock.set("2026-11-22T10:05:00Z")
	manager.tick(context.Background())
	if saved := h.readState().TrustedTime; !saved.Equal(h.clock.now()) || h.readState().TrustedSource != string(TimeFromClock) {
		t.Fatal("cada minuto se toma y se guarda la hora local")
	}

	// La auditoría ya no la vuelve a iniciar: si no, la §6.3 no podría bajarla (Duda 16).
	h.database.latest = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	restarted := h.start()
	if !restarted.TrustedTime().Equal(h.clock.now()) {
		t.Fatal("con hora confiable guardada, la auditoría no la cambia")
	}

	// Solo la §6.3 y la §6.4 la reemplazan, también hacia atrás.
	if _, err := restarted.ReplaceTrustedTime(later, ReplaceReason("otro")); err == nil {
		t.Fatal("un motivo desconocido no la reemplaza")
	}
	earlier := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	previous, err := restarted.ReplaceTrustedTime(earlier, ReplaceByInternetCheck)
	if err != nil || !previous.Equal(h.clock.now()) || !restarted.TrustedTime().Equal(earlier) {
		t.Fatalf("la §6.3 la baja y devuelve la anterior: %v %v", previous, err)
	}
	if state := h.readState(); !state.TrustedTime.Equal(earlier) || state.TrustedSource != string(ReplaceByInternetCheck) {
		t.Fatal("el reemplazo se guarda de inmediato")
	}
	if _, err := restarted.ReplaceTrustedTime(earlier.Add(time.Hour), ReplaceBySupportUnlock); err != nil {
		t.Fatal(err)
	}
}

// «Sin activar con datos» sale de la consulta de la base, que se repite cada minuto mientras no haya
// operaciones y deja de consultarse cuando las hay.
func TestManagerNotActivatedWithData(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	if status := manager.Status(); status.State != StateNotActivated {
		t.Fatalf("sin operaciones: %+v", status)
	}
	h.database.setOperations(true)
	manager.tick(context.Background())
	if status := manager.Status(); status.State != StateNotActivatedWithData {
		t.Fatalf("con operaciones: %+v", status)
	}
	calls := h.database.operationCalls
	manager.tick(context.Background())
	if h.database.operationCalls != calls {
		t.Fatal("con operaciones ya no se vuelve a consultar")
	}
	// Una falla de la base no cambia el estado ni detiene al gestor.
	failing := newHarness(t)
	failing.database.failure = errors.New("sin base")
	if status := failing.start().Status(); status.State != StateNotActivated {
		t.Fatalf("sin base: %+v", status)
	}
}

// Sin identidad o sin llave de licencias no se acepta nada y no se escribe nada en identity/licencia: así una
// llave que falló al crearse se puede crear en el siguiente arranque.
func TestManagerWithoutIdentity(t *testing.T) {
	loaded := loadFixtures(t)
	directory := filepath.Join(t.TempDir(), "identity", "licencia")
	manager := NewManager(Options{
		Directory:       directory,
		IdentityProblem: "sin llave",
		Trust:           loaded.testTrust(),
		MachineID: func() (string, error) {
			return machineOriginal, nil
		},
	})
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if status.State != StateNotActivated || !status.IdentityUnavailable {
		t.Fatalf("sin identidad: %+v", status)
	}
	if _, err := manager.Accept(loaded.license(t, "subscription-monthly").JWS); !errors.Is(err, ErrIdentityUnavailable) {
		t.Fatalf("sin identidad no se acepta: %v", err)
	}
	manager.ObserveTime(time.Now(), TimeFromServer)
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("sin identidad no se crea identity/licencia")
	}
}

// Las llaves de confianza son las del entorno del gestor: una licencia de prueba no vale con las llaves
// embebidas (las del servidor de licencias de pruebas), que no son las de los archivos de prueba.
func TestManagerUsesOnlyItsTrustedKeys(t *testing.T) {
	loaded := loadFixtures(t)
	installation := loaded.installations["installation-fixture-a"]
	identityDir := t.TempDir()
	writeTestMachineRecord(t, loaded, identityDir, "installation-fixture-a", machineOriginal)
	manager := NewManager(Options{
		Directory:    filepath.Join(identityDir, "licencia"),
		Installation: &installation,
		MachineID: func() (string, error) {
			return machineOriginal, nil
		},
	})
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := manager.Accept(loaded.license(t, "subscription-monthly").JWS)
	requireCode(t, err, ErrUnknownKid)
}

func mustRead(t *testing.T, filename string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
