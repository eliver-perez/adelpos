package legacyimport

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Corridas de análisis e importación (app.legacy_import_runs): las crea la ruta dentro de la transacción de
// autenticación (en cola) y el trabajador las avanza en transacciones cortas propias.

// Tipos y estados de una corrida.
const (
	KindAnalyze       = "analyze"
	KindImport        = "import"
	StatusQueued      = "queued"
	StatusRunning     = "running"
	StatusSucceeded   = "succeeded"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted"
)

// Querier es una transacción o el pool.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
}

// RunView es una corrida tal como la ve el asistente.
type RunView struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind"`
	Status          string          `json:"status"`
	Phase           string          `json:"phase"`
	Progress        json.RawMessage `json:"progress"`
	UploadID        string          `json:"upload_id"`
	PackageBytes    int64           `json:"package_bytes"`
	AnalysisRunID   *string         `json:"analysis_run_id"`
	ReplaceBatchID  *string         `json:"replace_batch_id"`
	BatchID         *string         `json:"batch_id"`
	BackupFile      *string         `json:"backup_file"`
	ErrorCode       *string         `json:"error_code"`
	ErrorMessage    *string         `json:"error_message"`
	WithDifferences bool            `json:"with_differences"`
	Summary         json.RawMessage `json:"summary"`
	Timings         json.RawMessage `json:"timings_ms"`
	RequestedBy     string          `json:"requested_by"`
	CreatedAt       time.Time       `json:"created_at"`
	StartedAt       *time.Time      `json:"started_at"`
	FinishedAt      *time.Time      `json:"finished_at"`
	packageSHA256   string
	requestedDevice string
}

const runColumns = `id::text, kind, status, phase, progress, upload_id::text, package_bytes, analysis_run_id::text,
    replace_batch_id::text, batch_id::text, backup_file, error_code, error_message, with_differences, summary, timings,
    requested_by::text, created_at, started_at, finished_at, package_sha256, requested_device::text`

func scanRun(row pgx.Row) (RunView, error) {
	var run RunView
	var summary []byte
	err := row.Scan(&run.ID, &run.Kind, &run.Status, &run.Phase, &run.Progress, &run.UploadID, &run.PackageBytes,
		&run.AnalysisRunID, &run.ReplaceBatchID, &run.BatchID, &run.BackupFile, &run.ErrorCode, &run.ErrorMessage,
		&run.WithDifferences, &summary, &run.Timings, &run.RequestedBy, &run.CreatedAt, &run.StartedAt, &run.FinishedAt,
		&run.packageSHA256, &run.requestedDevice)
	if summary != nil {
		run.Summary = summary
	} else {
		run.Summary = json.RawMessage("null")
	}
	return run, err
}

// FindRun lee una corrida (nil si no existe).
func FindRun(ctx context.Context, querier Querier, id string) (*RunView, error) {
	run, err := scanRun(querier.QueryRow(ctx, "SELECT "+runColumns+" FROM app.legacy_import_runs WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// ActiveRun es la corrida en cola o en curso (nil si no hay).
func ActiveRun(ctx context.Context, querier Querier) (*RunView, error) {
	run, err := scanRun(querier.QueryRow(ctx, "SELECT "+runColumns+` FROM app.legacy_import_runs
        WHERE status IN ('queued', 'running') ORDER BY seq DESC LIMIT 1`))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// RecentRuns son las últimas corridas, sin su resumen (para la lista del asistente).
func RecentRuns(ctx context.Context, querier Querier, limit int) ([]RunView, error) {
	rows, err := querier.Query(ctx, "SELECT "+runColumns+" FROM app.legacy_import_runs ORDER BY seq DESC LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []RunView{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		run.Summary = json.RawMessage("null")
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// insertRun registra una corrida en cola. Si ya hay otra en cola o en curso, CodeBusy.
func insertRun(ctx context.Context, querier Querier, run RunView) (RunView, error) {
	inserted, err := scanRun(querier.QueryRow(ctx, `INSERT INTO app.legacy_import_runs(id, kind, status, phase, upload_id,
            package_sha256, package_bytes, analysis_run_id, replace_batch_id, requested_by, requested_device)
        VALUES (gen_random_uuid(), $1, 'queued', 'queued', $2, $3, $4, $5, $6, $7, $8)
        RETURNING `+runColumns, run.Kind, run.UploadID, run.packageSHA256, run.PackageBytes, run.AnalysisRunID,
		run.ReplaceBatchID, run.RequestedBy, run.requestedDevice))
	var problem *pgconn.PgError
	if errors.As(err, &problem) && problem.Code == "23505" {
		return RunView{}, busyError()
	}
	return inserted, err
}

func busyError() *Error {
	return &Error{Code: CodeBusy, Message: "Ya hay una migración en curso. Espera a que termine y vuelve a intentarlo."}
}

// progressView es el avance que guarda la corrida.
type progressView struct {
	Done    int64 `json:"done"`
	Total   int64 `json:"total"`
	Percent int   `json:"percent"`
}
