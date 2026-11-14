package backupschedule

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

// Estados de una ejecución.
const (
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// Motivos de un respaldo fallido (failure_reason).
const (
	// FailureDirectory: ni la carpeta elegida ni la predeterminada aceptaron el archivo.
	FailureDirectory = "directory_unavailable"
	// FailureDump: pg_dump no pudo crear el respaldo (o tardó más del plazo).
	FailureDump = "dump_failed"
	// FailureVerify: el respaldo se creó pero no se pudo leer con pg_restore --list; se descartó.
	FailureVerify = "verify_failed"
	// FailureInterrupted: el servidor se detuvo mientras respaldaba.
	FailureInterrupted = "interrupted"
)

// Run es una ejecución de app.backup_runs.
type Run struct {
	ID string
	// Seq es el orden en que se registró (lo da la base, no el reloj).
	Seq                int64
	Kind               string
	Status             string
	ScheduledFor       *time.Time
	StartedAt          time.Time
	FinishedAt         *time.Time
	RequestedDirectory string
	Directory          string
	UsedDefault        bool
	FallbackReason     string
	FileName           string
	PartialFile        string
	SizeBytes          int64
	Verified           bool
	FailureReason      string
	RemovedAt          *time.Time
	OperationID        string
	RequestedBy        string
	RequestedDevice    string
}

// runColumns son las columnas que lee scanRun, en su orden.
const runColumns = `id::text, seq, kind, status, scheduled_for, started_at, finished_at, coalesce(requested_directory, ''),
    coalesce(directory, ''), used_default_directory, coalesce(fallback_reason, ''), coalesce(file_name, ''),
    coalesce(partial_file, ''), coalesce(size_bytes, 0), verified, coalesce(failure_reason, ''), removed_at, coalesce(operation_id::text, ''),
    coalesce(requested_by::text, ''), coalesce(requested_device::text, '')`

func scanRun(row pgx.Row) (Run, error) {
	var run Run
	err := row.Scan(&run.ID, &run.Seq, &run.Kind, &run.Status, &run.ScheduledFor, &run.StartedAt, &run.FinishedAt,
		&run.RequestedDirectory, &run.Directory, &run.UsedDefault, &run.FallbackReason, &run.FileName, &run.PartialFile,
		&run.SizeBytes,
		&run.Verified, &run.FailureReason, &run.RemovedAt, &run.OperationID, &run.RequestedBy, &run.RequestedDevice)
	return run, err
}

// RunView es una ejecución tal como la ven Mantenimiento, el estado del sistema y la pantalla de «Solo servidor». La
// carpeta se muestra por su nombre (folder_name y volume_name); la ruta completa (directory) solo aparece en
// «Información para soporte».
type RunView struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	FileName   string     `json:"file_name"`
	SizeBytes  *int64     `json:"size_bytes"`
	// Location es «default» (carpeta predeterminada) o «custom» (la elegida); vacío si no se llegó a elegir carpeta.
	Location       string `json:"location"`
	FolderName     string `json:"folder_name"`
	VolumeName     string `json:"volume_name"`
	Directory      string `json:"directory"`
	UsedDefault    bool   `json:"used_default_directory"`
	FallbackReason string `json:"fallback_reason"`
	Verified       bool   `json:"verified"`
	FailureReason  string `json:"failure_reason"`
	// Removed: la retención borró el archivo. Available (solo en la lista): el archivo sigue en su carpeta.
	Removed   bool  `json:"removed"`
	Available *bool `json:"available,omitempty"`
}

// View convierte una ejecución en su vista. checkFile comprueba además si el archivo sigue en su carpeta.
func View(run Run, checkFile bool) RunView {
	view := RunView{
		ID:             run.ID,
		Kind:           run.Kind,
		Status:         run.Status,
		StartedAt:      run.StartedAt.UTC(),
		FileName:       run.FileName,
		Directory:      run.Directory,
		UsedDefault:    run.UsedDefault,
		FallbackReason: run.FallbackReason,
		Verified:       run.Verified,
		FailureReason:  run.FailureReason,
		Removed:        run.RemovedAt != nil,
	}
	if run.FinishedAt != nil {
		finished := run.FinishedAt.UTC()
		view.FinishedAt = &finished
	}
	if run.SizeBytes > 0 {
		size := run.SizeBytes
		view.SizeBytes = &size
	}
	if run.Directory != "" {
		view.Location = "custom"
		if run.UsedDefault {
			view.Location = "default"
		}
		view.FolderName, view.VolumeName = Describe(run.Directory)
	}
	if checkFile && run.Status == StatusSucceeded {
		available := false
		if run.RemovedAt == nil {
			info, err := os.Stat(filepath.Join(run.Directory, run.FileName))
			available = err == nil && info.Mode().IsRegular()
		}
		view.Available = &available
	}
	return view
}

// ListRuns devuelve las ejecuciones más recientes primero.
func ListRuns(ctx context.Context, querier Querier, limit int) ([]Run, error) {
	rows, err := querier.Query(ctx, `SELECT `+runColumns+` FROM app.backup_runs ORDER BY seq DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// ClockTolerance es cuánto puede adelantarse el inicio registrado de un respaldo respecto de la hora actual sin
// considerarlo «del futuro». Un respaldo hecho con el reloj adelantado (y después corregido) no cuenta como el respaldo
// del día ni como el último correcto: así no deja la programación ni el aviso detenidos hasta esa fecha.
const ClockTolerance = 5 * time.Minute

// StaleAfter es cuánto puede seguir «en curso» una ejecución: el plazo de un respaldo más un margen. Una más vieja es un
// resto (su servidor se detuvo o no pudo registrar el resultado): no se muestra en curso ni detiene la programación, y
// el trabajador la cierra como interrumpida en cuanto toma el lock.
const StaleAfter = DefaultRunTimeout + 5*time.Minute

// lastRun devuelve la última ejecución terminada (por el orden de registro de la base) que cumple la condición, sin las
// que empezaron «en el futuro» respecto de now.
func lastRun(ctx context.Context, querier Querier, condition string, now time.Time) (*Run, error) {
	query := `SELECT ` + runColumns + ` FROM app.backup_runs WHERE status <> 'running' AND started_at <= $1`
	if condition != "" {
		query += " AND " + condition
	}
	query += " ORDER BY seq DESC LIMIT 1"
	run, err := scanRun(querier.QueryRow(ctx, query, now.Add(ClockTolerance)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// LastBackup es el último respaldo terminado (correcto o no) para el estado del sistema; nil si nunca hubo uno.
func LastBackup(ctx context.Context, querier Querier, now time.Time) (*RunView, error) {
	run, err := lastRun(ctx, querier, "", now)
	if err != nil || run == nil {
		return nil, err
	}
	view := View(*run, false)
	return &view, nil
}

// FindOperation busca la ejecución de una clave de «Respaldar ahora».
func FindOperation(ctx context.Context, querier Querier, operationID string) (*Run, error) {
	run, err := scanRun(querier.QueryRow(ctx, `SELECT `+runColumns+` FROM app.backup_runs WHERE operation_id = $1`, operationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// Running indica si hay una ejecución en curso que no sea un resto (StaleAfter).
func Running(ctx context.Context, querier Querier, now time.Time) (bool, error) {
	var running bool
	err := querier.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.backup_runs WHERE status = 'running'
        AND started_at > $1 AND started_at <= $2)`, now.Add(-StaleAfter), now.Add(ClockTolerance)).Scan(&running)
	return running, err
}

// anyRunning indica si hay alguna fila en curso, aunque sea un resto: el trabajador entonces intenta cerrarla.
func anyRunning(ctx context.Context, querier Querier) (bool, error) {
	var running bool
	err := querier.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.backup_runs WHERE status = 'running')`).Scan(&running)
	return running, err
}

// slotHistory resume los respaldos automáticos que empezaron entre el instante programado y ahora (con la tolerancia
// del reloj); los «del futuro» no cuentan y una fila en curso solo cuenta si no es un resto. Un intento interrumpido
// (el servidor se detuvo) cuenta para el máximo de intentos, pero no hace esperar RetryDelay: se reintenta al volver.
func slotHistory(ctx context.Context, querier Querier, slot, now time.Time) (SlotHistory, error) {
	var history SlotHistory
	err := querier.QueryRow(ctx, `SELECT
            count(*) FILTER (WHERE status = 'succeeded'),
            count(*) FILTER (WHERE status <> 'running'),
            max(finished_at) FILTER (WHERE status <> 'running' AND failure_reason IS DISTINCT FROM 'interrupted'),
            coalesce(bool_or(status = 'running' AND started_at > $3), false)
        FROM app.backup_runs
        WHERE kind = 'automatic' AND started_at >= $1 AND started_at <= $2`, slot, now.Add(ClockTolerance),
		now.Add(-StaleAfter)).Scan(&history.Succeeded, &history.Attempts, &history.LastFinished, &history.Running)
	return history, err
}

// OverdueAfter es cuánto puede pasar sin un respaldo correcto, con los respaldos automáticos activos, antes de avisar
// (un día y un margen para el respaldo que se hace al encender la computadora).
const OverdueAfter = 26 * time.Hour

// Alert es el aviso persistente al administrador (pantalla de inicio, Mantenimiento y «Solo servidor»):
//
//   - «failed»: el último respaldo terminado falló; Since es el primer fallo desde el último correcto y Failures
//     cuántos van. Sigue hasta que haya un respaldo correcto posterior.
//   - «overdue»: los respaldos automáticos están activos y no hay un respaldo correcto en más de OverdueAfter (la
//     computadora estuvo apagada a la hora del respaldo días seguidos, la programación no puede leerse, el reloj se
//     movió…). Since es el último respaldo correcto (Reason «stale») o, sin ninguno, desde cuándo se esperaba el primero
//     (Reason «never»). Sigue hasta un respaldo correcto.
//   - «fallback»: el último respaldo fue correcto, pero la carpeta elegida no estaba disponible y se guardó en la
//     predeterminada. Sigue hasta que un respaldo correcto quede en la carpeta elegida (o se vuelva a la
//     predeterminada o se elija otra carpeta).
type Alert struct {
	Kind     string    `json:"kind"`
	Reason   string    `json:"reason"`
	Since    time.Time `json:"since"`
	Failures int       `json:"failures"`
}

// alertInput es lo que necesita la decisión del aviso.
type alertInput struct {
	Now time.Time
	// Last es el último respaldo terminado y LastSuccess el último correcto (sin los «del futuro»).
	Last        *Run
	LastSuccess *Run
	// Failures y FirstFailure: los fallos desde el último correcto.
	Failures     int
	FirstFailure time.Time
	// Enabled, Configured y Expected: respaldos activos, establecimiento configurado y desde cuándo se espera un
	// respaldo (lo más reciente entre la activación y la configuración del establecimiento, sin fechas «del futuro»).
	Enabled             bool
	Configured          bool
	Expected            time.Time
	ConfiguredDirectory string
}

// ComputeAlert calcula el aviso vigente con la configuración actual; nil si no hay nada que avisar.
func ComputeAlert(ctx context.Context, querier Querier, settings Settings, now time.Time) (*Alert, error) {
	input := alertInput{Now: now, Enabled: settings.Enabled, ConfiguredDirectory: settings.Directory}
	var err error
	if input.Last, err = lastRun(ctx, querier, "", now); err != nil {
		return nil, err
	}
	if input.LastSuccess, err = lastRun(ctx, querier, "status = 'succeeded'", now); err != nil {
		return nil, err
	}
	if input.Last != nil && input.Last.Status == StatusFailed {
		afterSeq := int64(0)
		if input.LastSuccess != nil {
			afterSeq = input.LastSuccess.Seq
		}
		err := querier.QueryRow(ctx, `SELECT count(*), coalesce(min(started_at), $3) FROM app.backup_runs
            WHERE status = 'failed' AND seq > $1 AND started_at <= $2`, afterSeq, now.Add(ClockTolerance), input.Last.StartedAt).
			Scan(&input.Failures, &input.FirstFailure)
		if err != nil {
			return nil, err
		}
	}
	var configuredAt *time.Time
	if err := querier.QueryRow(ctx, "SELECT (SELECT configured_at FROM app.establishment)").Scan(&configuredAt); err != nil {
		return nil, err
	}
	if configuredAt != nil {
		input.Configured = true
		input.Expected = latestPast(now, *configuredAt, settings.EnabledAt)
	}
	return alertFor(input), nil
}

// latestPast es el más reciente de los instantes que no están «en el futuro» respecto de now (cero si ninguno).
func latestPast(now time.Time, instants ...time.Time) time.Time {
	var latest time.Time
	for _, instant := range instants {
		if instant.IsZero() || instant.After(now.Add(ClockTolerance)) {
			continue
		}
		if instant.After(latest) {
			latest = instant
		}
	}
	return latest
}

// alertFor decide el aviso: primero el fallo, después el atraso y por último la carpeta no disponible.
func alertFor(input alertInput) *Alert {
	if input.Last != nil && input.Last.Status == StatusFailed {
		return &Alert{Kind: "failed", Reason: input.Last.FailureReason, Since: input.FirstFailure.UTC(), Failures: input.Failures}
	}
	if input.Enabled && input.Configured {
		// Se mide desde lo más reciente entre el último respaldo correcto y desde cuándo se espera uno (al activar los
		// respaldos o configurar el establecimiento no se avisa enseguida).
		reference, reason, since := input.Expected, "never", input.Expected
		if input.LastSuccess != nil {
			reason, since = "stale", input.LastSuccess.StartedAt
			if input.LastSuccess.StartedAt.After(reference) {
				reference = input.LastSuccess.StartedAt
			}
		}
		if !reference.IsZero() && input.Now.Sub(reference) > OverdueAfter {
			return &Alert{Kind: "overdue", Reason: reason, Since: since.UTC()}
		}
	}
	last := input.Last
	if last == nil || last.Status != StatusSucceeded || last.FallbackReason == "" {
		return nil
	}
	sameFolder := input.ConfiguredDirectory != "" && last.RequestedDirectory != "" &&
		SameDirectory(input.ConfiguredDirectory, last.RequestedDirectory)
	if !sameFolder {
		return nil
	}
	since := last.StartedAt
	if last.FinishedAt != nil {
		since = *last.FinishedAt
	}
	return &Alert{Kind: "fallback", Reason: last.FallbackReason, Since: since.UTC()}
}
