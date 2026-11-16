package legacyimport

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/businessday"
)

// Locks consultivos de la importación (tabla de CLAUDE.md §3). La transacción toma, solo para escribir el catálogo,
// las mesas y los usuarios, 7820148305 (operación) → 7820148304 (catálogo) → 7820148308 (inventario) exclusivos, en
// ese orden y después de escribir el historial (el volumen grande no los retiene). El lock propio de la migración
// (7820148310) lo toma el trabajador antes, en su propia conexión (worker.go).
const (
	operationLock = 7820148305
	catalogLock   = 7820148304
	inventoryLock = 7820148308
)

// lockWait es lo más que la importación espera los locks de la operación (en una instalación limpia nadie los tiene
// por mucho tiempo).
const lockWait = "30s"

// historyTables en el orden en que se borran al reemplazar un lote.
var historyTables = []string{"legacy_sales_recognitions", "legacy_price_history", "legacy_cash_withdrawals",
	"legacy_cash_shifts", "legacy_payments", "legacy_cancellations", "legacy_discounts", "legacy_quantity_changes",
	"legacy_line_complements", "legacy_sale_lines", "legacy_sales"}

// Progress informa el avance de una fase: hechos y total (0 si no se conoce).
type Progress func(phase string, done, total int64)

// Fases del avance.
const (
	PhaseQueued     = "queued"
	PhaseReading    = "reading"
	PhaseChecking   = "checking"
	PhaseBackup     = "backup"
	PhaseRemoving   = "removing_previous"
	PhaseHistory    = "writing_history"
	PhaseLocks      = "waiting_locks"
	PhaseCatalog    = "writing_catalog"
	PhaseValidating = "validating"
	PhaseCommitting = "committing"
	PhaseDone       = "done"
)

// Hooks permiten a las pruebas provocar una interrupción o una diferencia dentro de la transacción. Nil en
// producción.
type Hooks struct {
	AfterHistory func(ctx context.Context, tx pgx.Tx) error
	AfterLive    func(ctx context.Context, tx pgx.Tx) error
}

// ImportRequest es una importación autorizada.
type ImportRequest struct {
	RunID    string
	UserID   string
	Username string
	DeviceID string
	// ExpectedBatchID es el lote vigente que el propietario confirmó reemplazar ("" si confirmó que no había).
	ExpectedBatchID string
	PackageSHA256   string
	PackageBytes    int64
	BackupFile      string
	Hooks           *Hooks
}

// Environment es lo que la correspondencia necesita de la instalación: la configuración del día comercial (zona y
// hora de inicio), los nombres de usuario que no pertenecen al lote vigente y el lote vigente.
type Environment struct {
	Config            businessday.Config
	ReservedUsernames map[string]bool
	CurrentBatch      *BatchView
	Blockers          []Blocker
}

// ReadEnvironment lee el entorno en una transacción corta de solo lectura.
func ReadEnvironment(ctx context.Context, pool *pgxpool.Pool) (Environment, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Environment{}, err
	}
	defer tx.Rollback(context.Background())
	return readEnvironment(ctx, tx)
}

func readEnvironment(ctx context.Context, tx pgx.Tx) (Environment, error) {
	environment := Environment{ReservedUsernames: map[string]bool{}}
	config, err := businessday.Load(ctx, tx)
	if err != nil {
		return environment, err
	}
	environment.Config = config
	current, err := CurrentBatchID(ctx, tx)
	if err != nil {
		return environment, err
	}
	rows, err := tx.Query(ctx, `SELECT username FROM app.users u WHERE NOT EXISTS (SELECT 1 FROM app.legacy_id_map m
        WHERE m.batch_id = $1 AND m.entity = 'user' AND m.new_id = u.id)`, nullableText(current))
	if err != nil {
		return environment, err
	}
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			rows.Close()
			return environment, err
		}
		environment.ReservedUsernames[username] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return environment, err
	}
	if current != "" {
		batch, err := loadBatchView(ctx, tx, current)
		if err != nil {
			return environment, err
		}
		environment.CurrentBatch = batch
	}
	environment.Blockers, err = CheckInstallation(ctx, tx)
	return environment, err
}

func nullableText(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

// loadBatchView lee un lote.
func loadBatchView(ctx context.Context, tx pgx.Tx, id string) (*BatchView, error) {
	batch := &BatchView{}
	err := tx.QueryRow(ctx, `SELECT b.id::text, b.status, b.started_at, b.completed_at, b.imported_by_username, b.backup_taken_at,
            b.package_fingerprint, coalesce((b.counts->>'legacy_sales')::bigint, 0), b.sales_first_at, b.sales_last_at,
            b.has_differences
        FROM app.legacy_import_batches b WHERE b.id = $1`, id).Scan(&batch.ID, &batch.Status, &batch.ImportedAt,
		&batch.CompletedAt, &batch.ImportedBy, &batch.BackupTakenAt, &batch.Fingerprint, &batch.Sales, &batch.SalesFirstAt,
		&batch.SalesLastAt, &batch.Differences)
	if err != nil {
		return nil, err
	}
	return batch, nil
}

// yearOf es el año del día comercial de un instante.
func yearOf(config businessday.Config) func(time.Time) int {
	return func(instant time.Time) int {
		return config.DayOf(instant).Date.Year()
	}
}

// Analyze hace el resumen previo de un paquete ya leído: la correspondencia en seco, sus totales, sus marcas, si
// existe un lote que se reemplazaría y si la instalación permite importar.
func Analyze(model *Model, environment Environment, sizeBytes int64, sha256 string) (*Plan, *Preview) {
	plan := BuildPlan(model, environment.ReservedUsernames)
	expected := computeExpected(model, yearOf(environment.Config))
	preview := &Preview{
		Summary:      buildSummary(plan, expected, sizeBytes, sha256),
		CurrentBatch: environment.CurrentBatch,
		WillReplace:  environment.CurrentBatch != nil,
		Clean:        len(environment.Blockers) == 0,
		Blockers:     environment.Blockers,
	}
	if preview.Blockers == nil {
		preview.Blockers = []Blocker{}
	}
	return plan, preview
}

// Import escribe el paquete en una sola transacción: el historial completo, el catálogo, las mesas y los usuarios,
// la correspondencia, la validación y el lote; si había un lote vigente, lo reemplaza completo (sus filas del
// historial se borran y lo vivo se actualiza en su lugar). O queda el lote nuevo completo o el anterior intacto: un
// error, una cancelación del contexto o un apagado a la mitad deshacen todo.
func Import(ctx context.Context, pool *pgxpool.Pool, plan *Plan, config businessday.Config, request ImportRequest,
	progress Progress) (*Report, error) {
	started := time.Now()
	report := func(phase string, done, total int64) {
		if progress != nil {
			progress(phase, done, total)
		}
	}
	expected := computeExpected(plan.Model, yearOf(config))
	summary := buildSummary(plan, expected, request.PackageBytes, request.PackageSHA256)
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer connection.Release()
	tx, err := connection.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	// El lote vigente (bloqueado hasta el final) debe ser el que el propietario confirmó reemplazar.
	var current string
	err = tx.QueryRow(ctx, `SELECT id::text FROM app.legacy_import_batches WHERE status = 'completed' FOR UPDATE`).Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if current != request.ExpectedBatchID {
		return nil, &Error{Code: CodeBatchChanged,
			Message: "El lote importado cambió desde que revisaste el resumen. Vuelve a analizar el paquete antes de importar."}
	}
	previous, err := loadIDMap(ctx, tx, current)
	if err != nil {
		return nil, err
	}
	ids := resolveIDs(&plan.Catalog, previous)
	batch := newUUID()
	if _, err := tx.Exec(ctx, "SELECT set_config('adel.legacy_import', $1, true)", uuidText(batch)); err != nil {
		return nil, err
	}
	if err := insertBatch(ctx, tx, plan, batch, config, request, started); err != nil {
		return nil, err
	}
	timings := map[string]int64{}
	if current != "" {
		phaseStart := time.Now()
		for index, table := range historyTables {
			report(PhaseRemoving, int64(index), int64(len(historyTables)))
			if _, err := tx.Exec(ctx, "DELETE FROM app."+table+" WHERE batch_id = $1", current); err != nil {
				return nil, err
			}
		}
		timings["remove_previous"] = time.Since(phaseStart).Milliseconds()
	}
	phaseStart := time.Now()
	written, err := writeHistory(ctx, tx, plan, ids, batch, func(done, total int64) {
		report(PhaseHistory, done, total)
	})
	if err != nil {
		return nil, err
	}
	timings["write_history"] = time.Since(phaseStart).Milliseconds()
	if request.Hooks != nil && request.Hooks.AfterHistory != nil {
		if err := request.Hooks.AfterHistory(ctx, tx); err != nil {
			return nil, err
		}
	}
	// Validación del historial antes de tomar los locks de la operación (no los necesita).
	report(PhaseValidating, 0, 0)
	phaseStart = time.Now()
	sales, payments, withdrawals, unbalanced, err := actualMoney(ctx, tx, batch, config)
	if err != nil {
		return nil, err
	}
	timings["validate_history"] = time.Since(phaseStart).Milliseconds()
	// Lo vivo, con los locks de la operación, el catálogo y el inventario exclusivos (en ese orden).
	report(PhaseLocks, 0, 0)
	lockStart := time.Now()
	if err := takeOperationLocks(ctx, tx); err != nil {
		return nil, err
	}
	lockedAt := time.Now()
	timings["lock_wait"] = lockedAt.Sub(lockStart).Milliseconds()
	blockers, err := CheckInstallation(ctx, tx)
	if err != nil {
		return nil, err
	}
	if len(blockers) > 0 {
		return nil, notCleanError(blockers)
	}
	report(PhaseCatalog, 0, 0)
	live, err := replaceLive(ctx, tx, &plan.Catalog, ids, previous)
	if err != nil {
		return nil, err
	}
	if err := writeIDMap(ctx, tx, batch, ids, live.Retired); err != nil {
		return nil, err
	}
	if request.Hooks != nil && request.Hooks.AfterLive != nil {
		if err := request.Hooks.AfterLive(ctx, tx); err != nil {
			return nil, err
		}
	}
	counts, err := actualCounts(ctx, tx, batch)
	if err != nil {
		return nil, err
	}
	validation := compare(expected, counts, sales, payments, withdrawals, unbalanced)
	if current != "" {
		if _, err := tx.Exec(ctx, "DELETE FROM app.legacy_id_map WHERE batch_id = $1", current); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE app.legacy_import_batches SET status = 'replaced', replaced_at = now(), replaced_by = $2
            WHERE id = $1`, current, batch); err != nil {
			return nil, err
		}
	}
	result := &Report{
		Summary:         summary,
		BatchID:         uuidText(batch),
		ReplacedBatchID: current,
		Validation:      validation,
		Written:         written,
		Retired:         len(live.Retired),
		TimingsMS:       timings,
		BackupFile:      request.BackupFile,
	}
	report(PhaseCommitting, 0, 0)
	timings["locks_held_before_commit"] = time.Since(lockedAt).Milliseconds()
	timings["total_before_commit"] = time.Since(started).Milliseconds()
	encodedReport, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	encodedValidation, err := json.Marshal(validation)
	if err != nil {
		return nil, err
	}
	encodedCounts, err := json.Marshal(written)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE app.legacy_import_batches SET status = 'completed', completed_at = now(), counts = $2,
            validation = $3, report = $4, has_differences = $5 WHERE id = $1`,
		batch, encodedCounts, encodedValidation, encodedReport, !validation.OK); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	// Cuánto quedaron tomados los locks de la operación: hasta la confirmación.
	timings["locks_held"] = time.Since(lockedAt).Milliseconds()
	timings["total"] = time.Since(started).Milliseconds()
	return result, nil
}

// takeOperationLocks toma 7820148305 → 7820148304 → 7820148308 exclusivos con un tiempo de espera acotado.
func takeOperationLocks(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '"+lockWait+"'"); err != nil {
		return err
	}
	for _, key := range []int64{operationLock, catalogLock, inventoryLock} {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", key); err != nil {
			if ctx.Err() == nil {
				return &Error{Code: CodeLocksBusy, Detail: err.Error(),
					Message: "AD-EL POS está ocupado con otra operación y no se pudo importar. Inténtalo de nuevo en unos minutos."}
			}
			return err
		}
	}
	_, err := tx.Exec(ctx, "SET LOCAL lock_timeout = 0")
	return err
}

// insertBatch registra el lote en curso (status importing) dentro de la transacción.
func insertBatch(ctx context.Context, tx pgx.Tx, plan *Plan, batch pgtype.UUID, config businessday.Config,
	request ImportRequest, started time.Time) error {
	model := plan.Model
	info := model.Info
	source := info.Source
	if len(source) == 0 || !json.Valid(source) {
		source = json.RawMessage(`{}`)
	}
	var createdAt *time.Time
	if !info.CreatedAt.IsZero() {
		created := info.CreatedAt
		createdAt = &created
	}
	var first, last *time.Time
	for _, sale := range model.Sales {
		opened := sale.OpenedAt
		if first == nil || opened.Before(*first) {
			first = &opened
		}
		if last == nil || opened.After(*last) {
			value := opened
			last = &value
		}
	}
	var sha *string
	if request.PackageSHA256 != "" {
		sha = &request.PackageSHA256
	}
	_, err := tx.Exec(ctx, `INSERT INTO app.legacy_import_batches(id, status, run_id, package_fingerprint, package_sha256,
            package_bytes, format_version, package_created_at, package_tool, package_product_version, backup_taken_at, source,
            timezone, imported_by, imported_by_username, imported_device, started_at, sales_first_at, sales_last_at)
        VALUES ($1, 'importing', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		batch, nullableText(request.RunID), info.Fingerprint, sha, request.PackageBytes, info.FormatVersion, createdAt,
		info.Tool, info.ProductVersion, parseBackupTime(info.BackupTakenAt, model.Location), source, config.Timezone,
		nullableText(request.UserID), request.Username, nullableText(request.DeviceID), started, first, last)
	return err
}
