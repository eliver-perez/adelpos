package legacyimport

import (
	"context"
	"time"
)

// execute corre un análisis o una importación de principio a fin y registra su resultado. Toda la corrida tiene el
// lock de la migración (7820148310) en su propia conexión. La frase de paso solo vive en job mientras se abre el
// paquete.
func (manager *Manager) execute(ctx context.Context, next job) {
	started := time.Now()
	timings := map[string]int64{}
	connection, acquired, err := manager.lock(ctx)
	if err != nil || !acquired {
		failure := busyError()
		if err != nil {
			failure = asError(ctx, err)
		}
		manager.finishRun(next.runID, StatusFailed, nil, failure, nil, timings, "")
		manager.auditFinished(next, false, failure.Code, nil)
		return
	}
	defer manager.unlock(connection)
	if _, err := manager.Pool.Exec(context.Background(), `UPDATE app.legacy_import_runs SET status = 'running', phase = $2,
            started_at = now(), updated_at = now() WHERE id = $1 AND status = 'queued'`, next.runID, PhaseReading); err != nil {
		manager.finishRun(next.runID, StatusFailed, nil, asError(ctx, err), nil, timings, "")
		return
	}
	reporter := &progressReporter{manager: manager, runID: next.runID, kind: next.kind}
	view, path, err := manager.claimUpload(next.actor, next.uploadID)
	if err != nil {
		manager.fail(ctx, next, err, timings)
		return
	}
	succeeded := false
	defer func() {
		manager.releaseUpload(view.ID, succeeded && next.kind == KindImport)
	}()
	environment, err := ReadEnvironment(ctx, manager.Pool)
	if err != nil {
		manager.fail(ctx, next, err, timings)
		return
	}
	// Lectura completa del paquete (y comprobación de su integridad) antes de cualquier escritura.
	phaseStart := time.Now()
	source, file, err := OpenPackage(path, next.passphrase)
	next.passphrase = ""
	if err != nil {
		manager.fail(ctx, next, err, timings)
		return
	}
	model, err := Load(ctx, source, environment.Config.Location, func(_ string, read, total int64) {
		reporter.report(PhaseReading, read, total)
	})
	file.Close()
	if err != nil {
		manager.fail(ctx, next, err, timings)
		return
	}
	timings["read"] = time.Since(phaseStart).Milliseconds()
	if next.kind == KindAnalyze {
		reporter.report(PhaseChecking, 0, 0)
		_, preview := Analyze(model, environment, view.Size, view.SHA256)
		timings["total"] = time.Since(started).Milliseconds()
		manager.finishRun(next.runID, StatusSucceeded, preview, nil, nil, timings, "")
		manager.auditFinished(next, true, "", map[string]any{"sales": preview.Counts[CountSales], "clean": preview.Clean,
			"will_replace": preview.WillReplace})
		succeeded = true
		return
	}
	report, backupFile, err := manager.importModel(ctx, next, model, view, reporter, timings)
	if err != nil {
		var summary any
		if problem := asError(ctx, err); problem.Code == CodeNotClean {
			summary = map[string]any{"blockers": problem.Blockers}
		}
		failure := asError(ctx, err)
		status := StatusFailed
		if failure.Code == CodeInterrupted {
			status = StatusInterrupted
		}
		manager.finishRun(next.runID, status, summary, failure, nil, timings, backupFile)
		manager.auditFinished(next, false, failure.Code, nil)
		manager.logWarning("legacy_import_failed", "run", next.runID, "code", failure.Code, "reason", failure.Detail)
		return
	}
	for key, value := range report.TimingsMS {
		timings[key] = value
	}
	timings["total_run"] = time.Since(started).Milliseconds()
	batch := report.BatchID
	manager.finishRun(next.runID, StatusSucceeded, report, nil, &batch, timings, backupFile)
	details := map[string]any{"batch_id": report.BatchID, "replaced_batch_id": report.ReplacedBatchID,
		"validation_ok": report.Validation.OK, "money_differences": report.Validation.HasMoneyDifferences,
		"written": report.Written, "backup_file": backupFile, "locks_held_ms": report.TimingsMS["locks_held"]}
	manager.auditFinished(next, true, "", details)
	if report.ReplacedBatchID != "" {
		manager.auditShort(next.actor, "legacy.batch_replaced", report.BatchID, map[string]any{
			"batch_id": report.BatchID, "replaced_batch_id": report.ReplacedBatchID}, true)
	}
	manager.logInfo("legacy_import_completed", "run", next.runID, "batch", report.BatchID,
		"validation_ok", report.Validation.OK, "locks_held_ms", report.TimingsMS["locks_held"])
	succeeded = true
}

// importModel comprueba la instalación y el lote confirmado, hace el respaldo verificado y escribe.
func (manager *Manager) importModel(ctx context.Context, next job, model *Model, view UploadView, reporter *progressReporter,
	timings map[string]int64) (*Report, string, error) {
	reporter.report(PhaseChecking, 0, 0)
	run, err := FindRun(ctx, manager.Pool, next.runID)
	if err != nil {
		return nil, "", err
	}
	environment, err := ReadEnvironment(ctx, manager.Pool)
	if err != nil {
		return nil, "", err
	}
	expected := ""
	if run != nil && run.ReplaceBatchID != nil {
		expected = *run.ReplaceBatchID
	}
	current := ""
	if environment.CurrentBatch != nil {
		current = environment.CurrentBatch.ID
	}
	if current != expected {
		return nil, "", &Error{Code: CodeBatchChanged,
			Message: "El lote importado cambió desde que revisaste el resumen. Vuelve a analizar el paquete antes de importar."}
	}
	if len(environment.Blockers) > 0 {
		return nil, "", notCleanError(environment.Blockers)
	}
	plan, _ := Analyze(model, environment, view.Size, view.SHA256)
	// Respaldo verificado de la base actual: sin él, no se importa.
	reporter.report(PhaseBackup, 0, 0)
	phaseStart := time.Now()
	if manager.Backup == nil {
		return nil, "", &Error{Code: CodeBackupFailed,
			Message: "No se pudo hacer el respaldo previo de la base y no se importó nada. Revisa los respaldos en Mantenimiento."}
	}
	backup, err := manager.Backup(ctx, next.actor, next.runID)
	timings["backup"] = time.Since(phaseStart).Milliseconds()
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		problem := asError(ctx, err)
		if problem.Code == CodeFailed {
			problem = &Error{Code: CodeBackupFailed, Detail: err.Error(),
				Message: "No se pudo hacer el respaldo previo de la base y no se importó nada. Revisa los respaldos en Mantenimiento."}
		}
		return nil, "", problem
	}
	report, err := Import(ctx, manager.Pool, plan, environment.Config, ImportRequest{
		RunID: next.runID, UserID: next.actor.UserID, Username: next.actor.Username, DeviceID: next.actor.DeviceID,
		ExpectedBatchID: expected, PackageSHA256: view.SHA256, PackageBytes: view.Size, BackupFile: backup.FileName,
		Hooks: manager.Hooks,
	}, reporter.report)
	return report, backup.FileName, err
}

// fail registra una corrida fallida antes de importar.
func (manager *Manager) fail(ctx context.Context, next job, err error, timings map[string]int64) {
	failure := asError(ctx, err)
	status := StatusFailed
	if failure.Code == CodeInterrupted {
		status = StatusInterrupted
	}
	manager.finishRun(next.runID, status, nil, failure, nil, timings, "")
	manager.auditFinished(next, false, failure.Code, nil)
	if failure.Detail != "" {
		manager.logWarning("legacy_run_failed", "run", next.runID, "code", failure.Code, "reason", failure.Detail)
	}
}

// auditFinished registra el resultado de una corrida (sin datos del paquete).
func (manager *Manager) auditFinished(next job, success bool, code string, details map[string]any) {
	action := "legacy.analysis_finished"
	if next.kind == KindImport {
		action = "legacy.import_finished"
	}
	if details == nil {
		details = map[string]any{}
	}
	if code != "" {
		details["error_code"] = code
	}
	manager.auditShort(next.actor, action, next.runID, details, success)
}
