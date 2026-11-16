package legacyimport

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrationLock es el lock consultivo propio de la migración (tabla de CLAUDE.md §3): pg_try_advisory_lock de sesión
// en una conexión propia del pool mientras dura un análisis o una importación, para que nunca corran dos aunque dos
// servidores compartan la base. Se toma primero y sin esperar (ocupado → la corrida falla con CodeBusy); con él
// tomado se hace el respaldo previo (7820148309, que se toma solo en su propia conexión, sin transacción abierta) y
// después la transacción de la importación toma 7820148305 → 7820148304 → 7820148308. Nadie más pide 7820148310, así
// que no puede formar un ciclo con los demás.
const MigrationLock = 7820148310

// Actor es quien pide algo de la migración (usuario y terminal de la sesión).
type Actor struct {
	UserID   string
	Username string
	DeviceID string
}

// AuditFunc registra un movimiento en app.audit_events dentro de la transacción indicada (la de la petición o una
// corta del trabajador). details nunca lleva datos del respaldo ni la frase de paso.
type AuditFunc func(ctx context.Context, tx pgx.Tx, actor Actor, action string, entityID string, details any, success bool) error

// BackupResult es el respaldo verificado previo a una importación.
type BackupResult struct {
	RunID    string
	FileName string
}

// BackupFunc hace el respaldo verificado de la base antes de importar, con el mecanismo de los respaldos de la 9G
// (backupschedule: pg_dump con serve, el de la instalación con el host). Si falla, no se importa.
type BackupFunc func(ctx context.Context, actor Actor, runID string) (BackupResult, error)

// Manager administra las subidas, las corridas y el trabajador de la migración. Lo crea administration y el
// trabajador (Run) arranca con serve y con el host, como los demás trabajadores de la 9G.
type Manager struct {
	Pool    *pgxpool.Pool
	WorkDir string
	Backup  BackupFunc
	Audit   AuditFunc
	Logger  *slog.Logger
	// MaxUploadBytes y UploadTTL: cero usa los predeterminados (1 GiB y 2 horas).
	MaxUploadBytes int64
	UploadTTL      time.Duration
	// Hooks y Now solo los usan las pruebas.
	Hooks *Hooks
	Now   func() time.Time

	mutex    sync.Mutex
	uploads  map[string]*upload
	jobs     chan job
	running  bool
	inflight sync.WaitGroup
}

// job es una corrida en cola con su frase de paso (solo en memoria, nunca en disco, en la base ni en bitácoras).
type job struct {
	runID      string
	kind       string
	actor      Actor
	uploadID   string
	passphrase string
}

// Pending es una corrida ya registrada que se entrega al trabajador después de confirmar la transacción de la
// petición.
type Pending struct {
	Run RunView
	job job
}

func (manager *Manager) now() time.Time {
	if manager.Now != nil {
		return manager.Now()
	}
	return time.Now()
}

// ensureState crea los mapas y la cola al primer uso (con el mutex tomado).
func (manager *Manager) ensureState() {
	if manager.uploads == nil {
		manager.uploads = map[string]*upload{}
	}
	if manager.jobs == nil {
		manager.jobs = make(chan job, 1)
	}
}

func (manager *Manager) logWarning(message string, arguments ...any) {
	if manager.Logger != nil {
		manager.Logger.Warn(message, arguments...)
	}
}

func (manager *Manager) logInfo(message string, arguments ...any) {
	if manager.Logger != nil {
		manager.Logger.Info(message, arguments...)
	}
}

// StartAnalysis registra un análisis en cola dentro de la transacción de la petición (con su auditoría). La corrida
// se entrega al trabajador con Dispatch después de confirmar.
func (manager *Manager) StartAnalysis(ctx context.Context, tx pgx.Tx, actor Actor, uploadID, passphrase string) (*Pending, error) {
	view, err := manager.completedUpload(actor, uploadID)
	if err != nil {
		return nil, err
	}
	if err := validPassphrase(passphrase); err != nil {
		return nil, err
	}
	run, err := insertRun(ctx, tx, RunView{Kind: KindAnalyze, UploadID: view.ID, packageSHA256: view.SHA256,
		PackageBytes: view.Size, RequestedBy: actor.UserID, requestedDevice: actor.DeviceID})
	if err != nil {
		return nil, err
	}
	if err := manager.audit(ctx, tx, actor, "legacy.analysis_requested", run.ID, map[string]any{
		"upload_id": view.ID, "package_bytes": view.Size, "package_sha256": view.SHA256}, true); err != nil {
		return nil, err
	}
	return &Pending{Run: run, job: job{runID: run.ID, kind: KindAnalyze, actor: actor, uploadID: view.ID, passphrase: passphrase}}, nil
}

// ImportInput es lo que confirma el propietario para importar.
type ImportInput struct {
	UploadID      string
	Passphrase    string
	AnalysisRunID string
	Confirm       bool
	// ReplaceBatchID es el lote vigente que confirma reemplazar ("" si el análisis dijo que no había).
	ReplaceBatchID string
}

// StartImport registra una importación en cola: exige la confirmación explícita, un análisis correcto del mismo
// paquete y, si hay un lote vigente, la confirmación de reemplazar ese lote.
func (manager *Manager) StartImport(ctx context.Context, tx pgx.Tx, actor Actor, input ImportInput) (*Pending, error) {
	if !input.Confirm {
		return nil, &Error{Code: CodeUploadInvalid, Message: "Confirma la importación para continuar."}
	}
	view, err := manager.completedUpload(actor, input.UploadID)
	if err != nil {
		return nil, err
	}
	if err := validPassphrase(input.Passphrase); err != nil {
		return nil, err
	}
	analysis, err := FindRun(ctx, tx, input.AnalysisRunID)
	if err != nil {
		return nil, err
	}
	if analysis == nil || analysis.Kind != KindAnalyze || analysis.Status != StatusSucceeded || analysis.UploadID != view.ID ||
		analysis.packageSHA256 != view.SHA256 {
		return nil, &Error{Code: CodeBatchChanged, Message: "Analiza el paquete y revisa el resumen antes de importarlo."}
	}
	current, err := CurrentBatchID(ctx, tx)
	if err != nil {
		return nil, err
	}
	if current != strings.ToLower(input.ReplaceBatchID) {
		return nil, &Error{Code: CodeBatchChanged,
			Message: "El lote importado cambió desde que revisaste el resumen. Vuelve a analizar el paquete antes de importar."}
	}
	analysisID := analysis.ID
	var replace *string
	if current != "" {
		replace = &current
	}
	run, err := insertRun(ctx, tx, RunView{Kind: KindImport, UploadID: view.ID, packageSHA256: view.SHA256,
		PackageBytes: view.Size, AnalysisRunID: &analysisID, ReplaceBatchID: replace, RequestedBy: actor.UserID,
		requestedDevice: actor.DeviceID})
	if err != nil {
		return nil, err
	}
	details := map[string]any{"upload_id": view.ID, "package_bytes": view.Size, "package_sha256": view.SHA256,
		"analysis_run_id": analysisID, "replace_batch_id": replace}
	if err := manager.audit(ctx, tx, actor, "legacy.import_requested", run.ID, details, true); err != nil {
		return nil, err
	}
	return &Pending{Run: run, job: job{runID: run.ID, kind: KindImport, actor: actor, uploadID: view.ID,
		passphrase: input.Passphrase}}, nil
}

func validPassphrase(passphrase string) error {
	if len(passphrase) < 8 || len(passphrase) > 1024 {
		return &Error{Code: CodeWrongPassphrase, Message: "Escribe la frase de paso del paquete (al menos 8 caracteres)."}
	}
	return nil
}

func (manager *Manager) completedUpload(actor Actor, id string) (UploadView, error) {
	view, err := manager.GetUpload(actor, id)
	if err != nil {
		return UploadView{}, err
	}
	if view.Status != UploadComplete {
		return UploadView{}, uploadError(CodeUploadInvalid, "El paquete todavía no termina de subirse.")
	}
	return view, nil
}

// Dispatch entrega al trabajador una corrida ya confirmada. Si el trabajador no corre (o la cola está llena, lo
// que no debería pasar porque solo hay una corrida activa), la corrida falla con su motivo.
func (manager *Manager) Dispatch(pending *Pending) {
	manager.mutex.Lock()
	manager.ensureState()
	running := manager.running
	jobs := manager.jobs
	manager.mutex.Unlock()
	if running {
		select {
		case jobs <- pending.job:
			return
		default:
		}
	}
	manager.finishRun(context.Background(), pending.job.runID, StatusFailed, nil, &Error{Code: CodeFailed,
		Message: "El servidor no pudo empezar la migración. Reinícialo e inténtalo de nuevo."}, nil, nil, "")
}

// audit registra un movimiento si hay quién lo haga (las pruebas pueden no tenerlo).
func (manager *Manager) audit(ctx context.Context, tx pgx.Tx, actor Actor, action, entityID string, details any, success bool) error {
	if manager.Audit == nil {
		return nil
	}
	return manager.Audit(ctx, tx, actor, action, entityID, details, success)
}

// auditShort registra un movimiento en una transacción corta propia (el trabajador, fuera de la petición).
func (manager *Manager) auditShort(actor Actor, action, entityID string, details any, success bool) {
	if manager.Audit == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := manager.Pool.Begin(ctx)
	if err != nil {
		manager.logWarning("legacy_audit_failed", "action", action, "reason", err.Error())
		return
	}
	defer tx.Rollback(context.Background())
	if err := manager.Audit(ctx, tx, actor, action, entityID, details, success); err != nil {
		manager.logWarning("legacy_audit_failed", "action", action, "reason", err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		manager.logWarning("legacy_audit_failed", "action", action, "reason", err.Error())
	}
}

// updateProgress guarda la fase y el avance de una corrida en una transacción corta.
func (manager *Manager) updateProgress(runID, phase string, progress progressView) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	encoded, _ := json.Marshal(progress)
	if _, err := manager.Pool.Exec(ctx, `UPDATE app.legacy_import_runs SET phase = $2, progress = $3, updated_at = now()
        WHERE id = $1 AND status = 'running'`, runID, phase, encoded); err != nil {
		manager.logWarning("legacy_progress_not_recorded", "reason", err.Error())
	}
}

// finishRun registra el resultado de una corrida (también al apagarse el servidor: cada intento con su propio plazo).
// Si la base no responde, reintenta mientras ctx (el del trabajador) siga vigente: al apagarse el servidor solo se hace
// un intento, para no alargar el apagado (lo que no se pudo registrar lo cierra la recuperación al volver a arrancar).
func (manager *Manager) finishRun(ctx context.Context, runID, status string, summary any, failure *Error, batchID *string,
	timings map[string]int64, backupFile string) {
	var encoded []byte
	if summary != nil {
		encoded, _ = json.Marshal(summary)
	}
	var code, message *string
	if failure != nil {
		code, message = &failure.Code, &failure.Message
	}
	if timings == nil {
		timings = map[string]int64{}
	}
	encodedTimings, _ := json.Marshal(timings)
	withDifferences := false
	if report, ok := summary.(*Report); ok {
		withDifferences = !report.Validation.OK
	}
	// Si la base no responde unos segundos, se reintenta: una corrida que quedara «en curso» bloquearía toda migración
	// (índice único de corridas activas) hasta la recuperación periódica del trabajador.
	wait := finishRetryFirst
	for attempt := 1; ; attempt++ {
		attemptCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := manager.Pool.Exec(attemptCtx, `UPDATE app.legacy_import_runs SET status = $2, phase = $3, summary = $4,
                error_code = $5, error_message = $6, batch_id = $7, timings = $8, with_differences = $9,
                backup_file = nullif($10, ''), finished_at = greatest(now(), coalesce(started_at, now())),
                started_at = coalesce(started_at, now()), updated_at = now()
            WHERE id = $1 AND status IN ('queued', 'running')`,
			runID, status, PhaseDone, encoded, code, message, batchID, encodedTimings, withDifferences, backupFile)
		cancel()
		if err == nil {
			return
		}
		manager.logWarning("legacy_run_not_recorded", "attempt", attempt, "reason", err.Error())
		if attempt == finishAttempts || ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		wait *= 2
	}
}

// Reintentos del registro del resultado de una corrida: hasta 5 intentos con esperas de 0.5, 1, 2 y 4 s.
const (
	finishAttempts   = 5
	finishRetryFirst = 500 * time.Millisecond
)

// staleAfter es cuánto debe llevar una corrida sin cambios para que la recuperación periódica la trate como un resto
// (una recién registrada se entrega al trabajador en milisegundos y una en curso tiene el lock de la migración).
const staleAfter = 2 * time.Minute

// Mensajes de una corrida que la recuperación cierra sin que se haya importado nada: al arrancar (el servidor se
// detuvo) y en la recuperación periódica (el servidor siguió, pero la corrida quedó sin terminar o sin su resultado).
const (
	interruptedMessage = "La migración se interrumpió porque el servidor se detuvo. No se importó nada; vuelve a intentarlo."
	leftoverMessage    = "La migración se interrumpió antes de terminar. No se importó nada; vuelve a intentarlo."
)

// RecoverInterrupted cierra las corridas que quedaron en cola o en curso porque su servidor se detuvo. Solo con el lock
// de la migración tomado (entonces nada está en curso en ningún servidor); si está ocupado, no hace nada.
func (manager *Manager) RecoverInterrupted(ctx context.Context) error {
	return manager.closeLeftoverRuns(ctx, 0)
}

// closeLeftoverRuns cierra las corridas en cola o en curso sin cambios desde hace más de olderThan según el reloj de la
// base (0 = todas), solo con el lock de la migración tomado (si está ocupado, algo está en curso y no hace nada):
//   - una importación cuyo lote quedó confirmado (legacy_import_batches.run_id; el proceso terminó justo después de la
//     confirmación y antes de registrar el resultado) queda «succeeded» con el informe y la validación del lote;
//   - las demás quedan «interrupted»: su transacción ya se deshizo y no quedó nada a medias.
func (manager *Manager) closeLeftoverRuns(ctx context.Context, olderThan time.Duration) error {
	connection, acquired, err := manager.lock(ctx)
	if err != nil || !acquired {
		return err
	}
	defer manager.unlock(connection)
	message := interruptedMessage
	if olderThan > 0 {
		message = leftoverMessage
	}
	stale := "($1::bigint = 0 OR r.updated_at < now() - $1::bigint * interval '1 millisecond')"
	rows, err := manager.Pool.Query(ctx, `UPDATE app.legacy_import_runs r SET status = 'succeeded', phase = 'done',
            summary = b.report, batch_id = b.id, with_differences = b.has_differences, error_code = NULL,
            error_message = NULL, backup_file = coalesce(nullif(b.report->>'backup_file', ''), r.backup_file),
            started_at = coalesce(r.started_at, b.started_at),
            finished_at = greatest(coalesce(b.completed_at, now()), coalesce(r.started_at, b.started_at)), updated_at = now()
        FROM app.legacy_import_batches b
        WHERE b.run_id = r.id AND b.status IN ('completed', 'replaced') AND b.report IS NOT NULL
            AND r.kind = 'import' AND r.status IN ('queued', 'running') AND `+stale+`
        RETURNING r.id::text, r.requested_by::text, r.requested_device::text,
            coalesce((SELECT u.username FROM app.users u WHERE u.id = r.requested_by), ''), b.id::text,
            coalesce(b.report->>'replaced_batch_id', ''), NOT b.has_differences`, olderThan.Milliseconds())
	if err != nil {
		return err
	}
	type recovered struct {
		run, batch, replaced string
		actor                Actor
		validationOK         bool
	}
	completed := []recovered{}
	for rows.Next() {
		var item recovered
		if err := rows.Scan(&item.run, &item.actor.UserID, &item.actor.DeviceID, &item.actor.Username, &item.batch,
			&item.replaced, &item.validationOK); err != nil {
			rows.Close()
			return err
		}
		completed = append(completed, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	tag, err := manager.Pool.Exec(ctx, `UPDATE app.legacy_import_runs r SET status = 'interrupted', phase = 'done',
            error_code = $2, error_message = $3, started_at = coalesce(r.started_at, now()),
            finished_at = greatest(now(), coalesce(r.started_at, now())), updated_at = now()
        WHERE r.status IN ('queued', 'running') AND `+stale, olderThan.Milliseconds(), CodeInterrupted, message)
	if err != nil {
		return err
	}
	for _, item := range completed {
		manager.auditShort(item.actor, "legacy.import_finished", item.run, map[string]any{"batch_id": item.batch,
			"replaced_batch_id": item.replaced, "validation_ok": item.validationOK, "recovered": true}, true)
		if item.replaced != "" {
			manager.auditShort(item.actor, "legacy.batch_replaced", item.batch, map[string]any{
				"batch_id": item.batch, "replaced_batch_id": item.replaced, "recovered": true}, true)
		}
	}
	if len(completed) > 0 || tag.RowsAffected() > 0 {
		manager.logWarning("legacy_runs_recovered", "succeeded", len(completed), "interrupted", tag.RowsAffected())
	}
	return nil
}

// lock toma el lock de la migración sin esperar en una conexión propia.
func (manager *Manager) lock(ctx context.Context) (*pgxpool.Conn, bool, error) {
	connection, err := manager.Pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	var acquired bool
	if err := connection.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", int64(MigrationLock)).Scan(&acquired); err != nil {
		connection.Release()
		return nil, false, err
	}
	if !acquired {
		connection.Release()
		return nil, false, nil
	}
	return connection, true, nil
}

// unlock suelta el lock; si no se puede, cierra la conexión (PostgreSQL suelta los locks de sesión al cerrarla).
func (manager *Manager) unlock(connection *pgxpool.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var released bool
	if err := connection.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", int64(MigrationLock)).Scan(&released); err != nil || !released {
		_ = connection.Conn().Close(ctx)
	}
	connection.Release()
}

// Run es el trabajador de la migración: al arrancar cierra las corridas que quedaron sin terminar y borra los archivos
// que hayan quedado en la carpeta de trabajo; después atiende las corridas una a una y cada minuto borra las subidas
// vencidas y cierra las corridas que llevan más de staleAfter sin terminar (con el lock de la migración libre).
// Termina al cancelarse ctx, después de que la corrida en curso registre su resultado (una importación a la mitad
// se deshace sola: su transacción nunca se confirma).
func (manager *Manager) Run(ctx context.Context) {
	manager.mutex.Lock()
	manager.ensureState()
	manager.running = true
	jobs := manager.jobs
	manager.mutex.Unlock()
	defer func() {
		manager.mutex.Lock()
		manager.running = false
		manager.mutex.Unlock()
	}()
	if err := manager.RecoverInterrupted(ctx); err != nil && ctx.Err() == nil {
		manager.logWarning("legacy_recovery_failed", "reason", err.Error())
	}
	if removed := manager.cleanOrphans(); removed > 0 {
		manager.logInfo("legacy_work_files_removed", "files", removed)
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if removed := manager.cleanExpired(); removed > 0 {
				manager.logInfo("legacy_uploads_expired", "uploads", removed)
			}
			// Una corrida que quedó en cola o en curso sin que nadie la atienda (por ejemplo, porque no se pudo
			// registrar su resultado) se cierra cuando el lock de la migración está libre.
			if err := manager.closeLeftoverRuns(ctx, staleAfter); err != nil && ctx.Err() == nil {
				manager.logWarning("legacy_recovery_failed", "reason", err.Error())
			}
		case next := <-jobs:
			manager.inflight.Add(1)
			manager.execute(ctx, next)
			manager.inflight.Done()
		}
	}
}

// Wait espera a que termine la corrida en curso.
func (manager *Manager) Wait() {
	manager.inflight.Wait()
}

// percent reparte el avance de las fases en un solo porcentaje.
func percent(kind, phase string, done, total int64) int {
	fraction := 0.0
	if total > 0 {
		fraction = float64(done) / float64(total)
		if fraction > 1 {
			fraction = 1
		}
	}
	if kind == KindAnalyze {
		if phase == PhaseReading {
			return int(fraction * 95)
		}
		return 97
	}
	ranges := map[string][2]float64{
		PhaseReading:    {0, 40},
		PhaseChecking:   {40, 42},
		PhaseBackup:     {42, 50},
		PhaseRemoving:   {50, 55},
		PhaseHistory:    {55, 90},
		PhaseValidating: {90, 92},
		PhaseLocks:      {92, 93},
		PhaseCatalog:    {93, 97},
		PhaseCommitting: {97, 99},
	}
	span, found := ranges[phase]
	if !found {
		return 0
	}
	return int(span[0] + (span[1]-span[0])*fraction)
}

// progressReporter guarda el avance como mucho cada medio segundo (y siempre al cambiar de fase).
type progressReporter struct {
	manager *Manager
	runID   string
	kind    string
	phase   string
	last    time.Time
}

func (reporter *progressReporter) report(phase string, done, total int64) {
	now := time.Now()
	if phase == reporter.phase && now.Sub(reporter.last) < 500*time.Millisecond {
		return
	}
	reporter.phase, reporter.last = phase, now
	reporter.manager.updateProgress(reporter.runID, phase, progressView{Done: done, Total: total,
		Percent: percent(reporter.kind, phase, done, total)})
}

// asError convierte cualquier error en uno de la migración; un contexto cancelado es una interrupción.
func asError(ctx context.Context, err error) *Error {
	var problem *Error
	if errors.As(err, &problem) {
		return problem
	}
	if ctx.Err() != nil {
		return &Error{Code: CodeInterrupted, Message: interruptedMessage}
	}
	return failure("La migración no se pudo completar. No se importó nada; inténtalo de nuevo.", err)
}
