package backupschedule

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/maintenance"
)

// LockKey es el lock consultivo de los respaldos programados (tabla de locks de CLAUDE.md §3). Se toma con
// pg_try_advisory_lock, de sesión, en una conexión propia del pool que se conserva todo el respaldo: un solo
// respaldo a la vez aunque dos servidores compartan la base, sin esperar (si está ocupado, no se respalda otra vez).
// Se toma solo, sin ninguna transacción abierta, y mientras se tiene únicamente se escriben app.backup_runs y la
// auditoría de «Respaldar ahora» en transacciones cortas sin otros locks consultivos; nunca se toma dentro de una
// transacción que tenga otro.
const LockKey = 7820148309

// DefaultRunTimeout es el plazo de un respaldo completo (pg_dump y verificación).
const DefaultRunTimeout = 30 * time.Minute

// finishTimeout es el plazo para registrar el resultado, también cuando el servidor se está apagando.
const finishTimeout = 10 * time.Second

// ErrBusy: ya hay un respaldo en curso (el lock está tomado).
var ErrBusy = errors.New("ya hay un respaldo en curso")

// ErrOperationConflict: la clave de «Respaldar ahora» ya la usó otro usuario u otra terminal.
var ErrOperationConflict = errors.New("la clave de operación ya se usó con otros datos")

// Runner hace los respaldos: los automáticos del trabajador (Worker) y los de «Respaldar ahora» (administration),
// por el mismo camino (elegir la carpeta, crear, verificar, renombrar, registrar y, en los automáticos, conservar
// solo los últimos).
type Runner struct {
	Pool   *pgxpool.Pool
	Target maintenance.ScheduledBackuper
	Logger *slog.Logger
	// Now permite simular la hora en las pruebas; nil usa time.Now.
	Now func() time.Time
	// RunTimeout es el plazo de cada respaldo; cero usa DefaultRunTimeout.
	RunTimeout time.Duration

	// starting serializa el comienzo de «Respaldar ahora» en este servidor: un reenvío de la misma clave encuentra la
	// ejecución que dejó el primero en lugar de responder que hay otro respaldo en curso.
	starting sync.Mutex
	// base es el contexto del trabajador: al apagarse el servidor, los respaldos en curso se interrumpen y se
	// registran. inflight espera a los de «Respaldar ahora» antes de cerrar el pool.
	baseMutex sync.Mutex
	base      context.Context
	inflight  sync.WaitGroup
}

// NewRunner arma el ejecutor de respaldos con el respaldo de la instalación.
func NewRunner(pool *pgxpool.Pool, target maintenance.ScheduledBackuper, logger *slog.Logger) *Runner {
	return &Runner{Pool: pool, Target: target, Logger: logger}
}

func (runner *Runner) now() time.Time {
	if runner.Now != nil {
		return runner.Now()
	}
	return time.Now()
}

func (runner *Runner) runTimeout() time.Duration {
	if runner.RunTimeout > 0 {
		return runner.RunTimeout
	}
	return DefaultRunTimeout
}

// bind fija el contexto del trabajador.
func (runner *Runner) bind(ctx context.Context) {
	runner.baseMutex.Lock()
	defer runner.baseMutex.Unlock()
	runner.base = ctx
}

// baseContext es el contexto del trabajador o, sin trabajador (pruebas), uno que no se cancela.
func (runner *Runner) baseContext() context.Context {
	runner.baseMutex.Lock()
	defer runner.baseMutex.Unlock()
	if runner.base == nil {
		return context.Background()
	}
	return runner.base
}

// Wait espera a que terminen los respaldos de «Respaldar ahora» en curso.
func (runner *Runner) Wait() {
	runner.inflight.Wait()
}

func (runner *Runner) logInfo(message string, arguments ...any) {
	if runner.Logger != nil {
		runner.Logger.Info(message, arguments...)
	}
}

func (runner *Runner) logWarning(message string, arguments ...any) {
	if runner.Logger != nil {
		runner.Logger.Warn(message, arguments...)
	}
}

// lockSession es el lock 7820148309 tomado en una conexión propia.
type lockSession struct {
	connection *pgxpool.Conn
}

// lock toma el lock de los respaldos sin esperar; ErrBusy si otro lo tiene.
func (runner *Runner) lock(ctx context.Context) (*lockSession, error) {
	connection, err := runner.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	var acquired bool
	if err := connection.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", int64(LockKey)).Scan(&acquired); err != nil {
		connection.Release()
		return nil, err
	}
	if !acquired {
		connection.Release()
		return nil, ErrBusy
	}
	return &lockSession{connection: connection}, nil
}

// unlock suelta el lock. Si no se puede, cierra la conexión: PostgreSQL suelta los locks de sesión al cerrarla.
func (session *lockSession) unlock() {
	ctx, cancel := context.WithTimeout(context.Background(), finishTimeout)
	defer cancel()
	var released bool
	err := session.connection.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", int64(LockKey)).Scan(&released)
	if err != nil || !released {
		_ = session.connection.Conn().Close(ctx)
	}
	session.connection.Release()
}

// RecoverInterrupted marca como interrumpidas las ejecuciones que quedaron «en curso» porque su servidor se detuvo.
// Solo lo hace con el lock tomado (entonces ningún respaldo está en curso); si está ocupado, no hace nada.
func (runner *Runner) RecoverInterrupted(ctx context.Context) error {
	session, err := runner.lock(ctx)
	if errors.Is(err, ErrBusy) {
		return nil
	}
	if err != nil {
		return err
	}
	defer session.unlock()
	return runner.markInterrupted(ctx)
}

// markInterrupted cierra como fallidas («interrupted») las ejecuciones en curso. Exige el lock tomado.
func (runner *Runner) markInterrupted(ctx context.Context) error {
	tag, err := runner.Pool.Exec(ctx, `UPDATE app.backup_runs
        SET status = 'failed', finished_at = greatest($1::timestamptz, started_at), failure_reason = 'interrupted'
        WHERE status = 'running'`, runner.now())
	if err == nil && tag.RowsAffected() > 0 {
		runner.logWarning("backup_interrupted_recorded", "runs", tag.RowsAffected())
	}
	return err
}

// insertRun registra el comienzo de una ejecución.
func (runner *Runner) insertRun(ctx context.Context, run Run) (Run, error) {
	return scanRun(runner.Pool.QueryRow(ctx, `INSERT INTO app.backup_runs(
            id, kind, status, scheduled_for, started_at, requested_directory, operation_id, requested_by, requested_device)
        VALUES (gen_random_uuid(), $1, 'running', $2, $3, $4, $5::uuid, $6::uuid, $7::uuid)
        RETURNING `+runColumns,
		run.Kind, run.ScheduledFor, run.StartedAt, nullable(run.RequestedDirectory), nullable(run.OperationID),
		nullable(run.RequestedBy), nullable(run.RequestedDevice)))
}

// RunScheduled hace el respaldo automático del instante programado slot si todavía toca: con el lock tomado vuelve
// a revisar el historial (otro servidor sobre la misma base pudo hacerlo mientras tanto). Devuelve nil sin error si
// ya no tocaba y ErrBusy si hay otro respaldo en curso.
func (runner *Runner) RunScheduled(ctx context.Context, slot time.Time) (*Run, error) {
	session, err := runner.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer session.unlock()
	if err := runner.markInterrupted(ctx); err != nil {
		return nil, err
	}
	history, err := slotHistory(ctx, runner.Pool, slot)
	if err != nil {
		return nil, err
	}
	if !Due(runner.now(), history) {
		return nil, nil
	}
	settings, err := LoadSettings(ctx, runner.Pool)
	if err != nil {
		return nil, err
	}
	scheduled := slot
	run, err := runner.insertRun(ctx, Run{
		Kind:               KindAutomatic,
		ScheduledFor:       &scheduled,
		StartedAt:          runner.now(),
		RequestedDirectory: settings.Directory,
	})
	if err != nil {
		return nil, err
	}
	final := runner.execute(ctx, run, settings)
	return &final, nil
}

// ManualRequest es un «Respaldar ahora» con su clave de operación, quién lo pidió y desde qué terminal.
type ManualRequest struct {
	OperationID string
	UserID      string
	DeviceID    string
}

// Started es el comienzo de «Respaldar ahora»: la ejecución recién registrada y el canal por el que llega al
// terminar o, en un reenvío de la misma clave, la ejecución que ya existía (Replayed, sin canal).
type Started struct {
	Run      Run
	Done     <-chan Run
	Replayed bool
}

// replay devuelve la ejecución de la misma clave si existe y es del mismo usuario y terminal.
func replay(run *Run, request ManualRequest) (Started, bool, error) {
	if run == nil {
		return Started{}, false, nil
	}
	if run.Kind != KindManual || run.RequestedBy != request.UserID || run.RequestedDevice != request.DeviceID {
		return Started{}, false, ErrOperationConflict
	}
	return Started{Run: *run, Replayed: true}, true, nil
}

// StartManual empieza «Respaldar ahora»: registra la ejecución con su clave y hace el respaldo en segundo plano (con
// el contexto del trabajador, no con el de la petición: si la terminal se desconecta, el respaldo termina igual y
// queda registrado). finished se llama al terminar (la auditoría). Un reenvío de la misma clave devuelve la misma
// ejecución (en curso o terminada) sin respaldar otra vez.
func (runner *Runner) StartManual(ctx context.Context, request ManualRequest, finished func(Run)) (Started, error) {
	runner.starting.Lock()
	defer runner.starting.Unlock()
	existing, err := FindOperation(ctx, runner.Pool, request.OperationID)
	if err != nil {
		return Started{}, err
	}
	if started, found, err := replay(existing, request); found || err != nil {
		return started, err
	}
	session, err := runner.lock(ctx)
	if err != nil {
		return Started{}, err
	}
	release := true
	defer func() {
		if release {
			session.unlock()
		}
	}()
	if err := runner.markInterrupted(ctx); err != nil {
		return Started{}, err
	}
	// Con el lock tomado, la clave se vuelve a buscar: otro servidor sobre la misma base pudo registrarla.
	existing, err = FindOperation(ctx, runner.Pool, request.OperationID)
	if err != nil {
		return Started{}, err
	}
	if started, found, err := replay(existing, request); found || err != nil {
		return started, err
	}
	settings, err := LoadSettings(ctx, runner.Pool)
	if err != nil {
		return Started{}, err
	}
	run, err := runner.insertRun(ctx, Run{
		Kind:               KindManual,
		StartedAt:          runner.now(),
		RequestedDirectory: settings.Directory,
		OperationID:        request.OperationID,
		RequestedBy:        request.UserID,
		RequestedDevice:    request.DeviceID,
	})
	if err != nil {
		return Started{}, err
	}
	done := make(chan Run, 1)
	release = false
	runner.inflight.Add(1)
	go func() {
		defer runner.inflight.Done()
		defer session.unlock()
		final := runner.execute(runner.baseContext(), run, settings)
		done <- final
		if finished != nil {
			finished(final)
		}
	}()
	return Started{Run: run, Done: done}, nil
}

// outcome es lo que se sabe al terminar un respaldo.
type outcome struct {
	directory      string
	usedDefault    bool
	fallbackReason string
	fileName       string
	size           int64
	verified       bool
	failure        string
}

// execute hace el respaldo, registra el resultado y, si fue un automático correcto, aplica la retención.
func (runner *Runner) execute(parent context.Context, run Run, settings Settings) Run {
	ctx, cancel := context.WithTimeout(parent, runner.runTimeout())
	defer cancel()
	result := runner.produce(ctx, run, settings)
	final := runner.finish(run, result)
	if final.Status == StatusSucceeded {
		runner.logInfo("backup_completed", "kind", final.Kind, "file", final.FileName, "size", final.SizeBytes,
			"directory", final.Directory, "used_default_directory", final.UsedDefault, "fallback_reason", final.FallbackReason)
		if final.Kind == KindAutomatic {
			runner.retain(final, settings.Keep)
		}
	} else {
		runner.logWarning("backup_failed", "kind", final.Kind, "reason", final.FailureReason, "directory", final.Directory)
	}
	return final
}

// chooseDirectory elige la carpeta: la configurada si está disponible o, si no, la predeterminada (que se crea si
// falta, con permisos 0700). ok es falso si tampoco la predeterminada acepta archivos.
func (runner *Runner) chooseDirectory(configured string) (directory string, usedDefault bool, fallback string, ok bool) {
	if configured != "" {
		reason := CheckDirectory(configured)
		if reason == "" {
			return configured, false, "", true
		}
		fallback = reason
		runner.logWarning("backup_directory_unavailable", "directory", configured, "reason", reason)
	}
	directory = runner.Target.DefaultDirectory()
	if err := os.MkdirAll(directory, 0700); err != nil {
		return directory, true, fallback, false
	}
	if reason := CheckDirectory(directory); reason != "" {
		return directory, true, fallback, false
	}
	return directory, true, fallback, true
}

// interruptedOr distingue un respaldo interrumpido por el apagado del servidor de otro fallo.
func interruptedOr(ctx context.Context, failure string) string {
	if errors.Is(ctx.Err(), context.Canceled) {
		return FailureInterrupted
	}
	return failure
}

// produce crea el respaldo con un nombre temporal (.partial), lo verifica y solo entonces le da su nombre definitivo.
func (runner *Runner) produce(ctx context.Context, run Run, settings Settings) outcome {
	var result outcome
	directory, usedDefault, fallback, ok := runner.chooseDirectory(settings.Directory)
	result.directory, result.usedDefault, result.fallbackReason = directory, usedDefault, fallback
	if !ok {
		result.failure = FailureDirectory
		return result
	}
	database := runner.Target.DatabaseName()
	CleanPartials(directory, database)
	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		result.failure = FailureDump
		return result
	}
	name := FileName(database, run.Kind, runner.now(), hex.EncodeToString(suffix))
	final := filepath.Join(directory, name)
	partial := final + partialSuffix
	if err := runner.Target.Dump(ctx, partial); err != nil {
		_ = os.Remove(partial)
		runner.logWarning("backup_dump_failed", "reason", err.Error())
		result.failure = interruptedOr(ctx, FailureDump)
		return result
	}
	info, err := os.Stat(partial)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		_ = os.Remove(partial)
		result.failure = FailureDump
		return result
	}
	// Verificación de cada respaldo: si no se puede leer, no se conserva (no sirve para restaurar).
	if err := runner.Target.Verify(ctx, partial); err != nil {
		_ = os.Remove(partial)
		runner.logWarning("backup_verification_failed", "reason", err.Error())
		result.failure = interruptedOr(ctx, FailureVerify)
		return result
	}
	if err := os.Rename(partial, final); err != nil {
		_ = os.Remove(partial)
		result.failure = FailureDump
		return result
	}
	result.fileName, result.size, result.verified = name, info.Size(), true
	return result
}

// finish registra el resultado (también si el servidor se está apagando: con su propio plazo).
func (runner *Runner) finish(run Run, result outcome) Run {
	ctx, cancel := context.WithTimeout(context.Background(), finishTimeout)
	defer cancel()
	status := StatusSucceeded
	if result.failure != "" {
		status = StatusFailed
		result.fileName, result.size, result.verified = "", 0, false
	}
	var size *int64
	if result.size > 0 {
		size = &result.size
	}
	final, err := scanRun(runner.Pool.QueryRow(ctx, `UPDATE app.backup_runs SET
            status = $2, finished_at = greatest($3::timestamptz, started_at), directory = $4,
            used_default_directory = $5, fallback_reason = $6, file_name = $7, size_bytes = $8, verified = $9,
            failure_reason = $10
        WHERE id = $1 AND status = 'running'
        RETURNING `+runColumns,
		run.ID, status, runner.now(), nullable(result.directory), result.usedDefault, nullable(result.fallbackReason),
		nullable(result.fileName), size, result.verified, nullable(result.failure)))
	if err != nil {
		// Si no se pudo registrar, la fila queda en curso y se marca como interrumpida la próxima vez.
		runner.logWarning("backup_result_not_recorded", "reason", err.Error())
		run.Status = status
		run.Directory, run.UsedDefault, run.FallbackReason = result.directory, result.usedDefault, result.fallbackReason
		run.FileName, run.SizeBytes, run.Verified, run.FailureReason = result.fileName, result.size, result.verified, result.failure
		return run
	}
	return final
}

// retain conserva los últimos keep respaldos automáticos de la carpeta donde quedó el respaldo y anota en el registro
// los que borró.
func (runner *Runner) retain(run Run, keep int) {
	removed, err := Retain(run.Directory, runner.Target.DatabaseName(), keep)
	if err != nil {
		runner.logWarning("backup_retention_incomplete", "directory", run.Directory, "reason", err.Error())
	}
	if len(removed) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), finishTimeout)
	defer cancel()
	_, err = runner.Pool.Exec(ctx, `UPDATE app.backup_runs SET removed_at = $3
        WHERE status = 'succeeded' AND removed_at IS NULL AND directory = $1 AND file_name = ANY($2)`,
		run.Directory, removed, runner.now())
	if err != nil {
		runner.logWarning("backup_retention_not_recorded", "reason", err.Error())
	}
	runner.logInfo("backup_retention_applied", "directory", run.Directory, "removed", len(removed), "keep", keep)
}
