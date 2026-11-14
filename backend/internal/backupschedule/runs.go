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
	ID                 string
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
	SizeBytes          int64
	Verified           bool
	FailureReason      string
	RemovedAt          *time.Time
	OperationID        string
	RequestedBy        string
	RequestedDevice    string
}

// runColumns son las columnas que lee scanRun, en su orden.
const runColumns = `id::text, kind, status, scheduled_for, started_at, finished_at, coalesce(requested_directory, ''),
    coalesce(directory, ''), used_default_directory, coalesce(fallback_reason, ''), coalesce(file_name, ''),
    coalesce(size_bytes, 0), verified, coalesce(failure_reason, ''), removed_at, coalesce(operation_id::text, ''),
    coalesce(requested_by::text, ''), coalesce(requested_device::text, '')`

func scanRun(row pgx.Row) (Run, error) {
	var run Run
	err := row.Scan(&run.ID, &run.Kind, &run.Status, &run.ScheduledFor, &run.StartedAt, &run.FinishedAt,
		&run.RequestedDirectory, &run.Directory, &run.UsedDefault, &run.FallbackReason, &run.FileName, &run.SizeBytes,
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
	rows, err := querier.Query(ctx, `SELECT `+runColumns+` FROM app.backup_runs ORDER BY started_at DESC, id LIMIT $1`, limit)
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

// lastRun devuelve la última ejecución terminada que cumple la condición (sin condición, cualquiera terminada).
func lastRun(ctx context.Context, querier Querier, condition string) (*Run, error) {
	query := `SELECT ` + runColumns + ` FROM app.backup_runs WHERE status <> 'running'`
	if condition != "" {
		query += " AND " + condition
	}
	query += " ORDER BY started_at DESC, id LIMIT 1"
	run, err := scanRun(querier.QueryRow(ctx, query))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// LastBackup es el último respaldo terminado (correcto o no) para el estado del sistema; nil si nunca hubo uno.
func LastBackup(ctx context.Context, querier Querier) (*RunView, error) {
	run, err := lastRun(ctx, querier, "")
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

// Running indica si hay una ejecución en curso.
func Running(ctx context.Context, querier Querier) (bool, error) {
	var running bool
	err := querier.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.backup_runs WHERE status = 'running')`).Scan(&running)
	return running, err
}

// slotHistory resume los respaldos automáticos que empezaron en o después del instante programado.
func slotHistory(ctx context.Context, querier Querier, slot time.Time) (SlotHistory, error) {
	var history SlotHistory
	err := querier.QueryRow(ctx, `SELECT
            count(*) FILTER (WHERE status = 'succeeded'),
            count(*) FILTER (WHERE status <> 'running'),
            max(finished_at),
            coalesce(bool_or(status = 'running'), false)
        FROM app.backup_runs
        WHERE kind = 'automatic' AND started_at >= $1`, slot).Scan(&history.Succeeded, &history.Attempts,
		&history.LastFinished, &history.Running)
	return history, err
}

// Alert es el aviso persistente al administrador (pantalla de inicio, Mantenimiento y «Solo servidor»):
//
//   - «failed»: el último respaldo terminado falló; Since es el primer fallo desde el último correcto y Failures
//     cuántos van. Sigue hasta que haya un respaldo correcto posterior.
//   - «fallback»: el último respaldo fue correcto, pero la carpeta elegida no estaba disponible y se guardó en la
//     predeterminada. Sigue hasta que un respaldo correcto quede en la carpeta elegida (o se vuelva a la
//     predeterminada o se elija otra carpeta).
type Alert struct {
	Kind     string    `json:"kind"`
	Reason   string    `json:"reason"`
	Since    time.Time `json:"since"`
	Failures int       `json:"failures"`
}

// ComputeAlert calcula el aviso vigente con la carpeta configurada ahora; nil si no hay nada que avisar.
func ComputeAlert(ctx context.Context, querier Querier, configuredDirectory string) (*Alert, error) {
	last, err := lastRun(ctx, querier, "")
	if err != nil || last == nil {
		return nil, err
	}
	failures := 0
	var since time.Time
	if last.Status == StatusFailed {
		err := querier.QueryRow(ctx, `SELECT count(*), min(started_at) FROM app.backup_runs
            WHERE status = 'failed' AND started_at > coalesce(
                (SELECT max(started_at) FROM app.backup_runs WHERE status = 'succeeded'), '-infinity')`).
			Scan(&failures, &since)
		if err != nil {
			return nil, err
		}
	}
	return alertFor(*last, failures, since, configuredDirectory), nil
}

// alertFor decide el aviso con el último respaldo terminado, los fallos desde el último correcto (y el primero de
// ellos) y la carpeta configurada ahora.
func alertFor(last Run, failures int, firstFailure time.Time, configuredDirectory string) *Alert {
	if last.Status == StatusFailed {
		return &Alert{Kind: "failed", Reason: last.FailureReason, Since: firstFailure.UTC(), Failures: failures}
	}
	sameFolder := configuredDirectory != "" && last.RequestedDirectory != "" &&
		SameDirectory(configuredDirectory, last.RequestedDirectory)
	if last.Status == StatusSucceeded && last.FallbackReason != "" && sameFolder {
		since := last.StartedAt
		if last.FinishedAt != nil {
			since = *last.FinishedAt
		}
		return &Alert{Kind: "fallback", Reason: last.FallbackReason, Since: since.UTC()}
	}
	return nil
}
