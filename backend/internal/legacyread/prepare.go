package legacyread

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"local/ad-el-pos/server/internal/legacypkg"
	"local/ad-el-pos/server/internal/product"
)

// Options son los datos de una preparación.
type Options struct {
	Backup     string
	Output     string
	Passphrase string
	// PageRows es el número de filas por página (cero = DefaultPageRows; las pruebas usan páginas chicas).
	PageRows int
	// ReadyTimeout es lo que se espera a que SQL Server arranque (cero = 5 minutos; con la emulación de Apple Silicon
	// el primer arranque tarda).
	ReadyTimeout time.Duration
	// KDF son los parámetros de Argon2id del paquete (cero = legacypkg.DefaultKDF).
	KDF legacypkg.KDFParams
}

// Preparer prepara paquetes con un Runner de docker y un Reporter del avance.
type Preparer struct {
	Runner Runner
	Events Reporter
	// TempDir es la carpeta temporal del usuario (tablas en preparación y copia del respaldo; "" = la del sistema).
	TempDir string
	// Now y Sleep permiten probar sin reloj real.
	Now   func() time.Time
	Sleep func(ctx context.Context, duration time.Duration) error
}

func (preparer *Preparer) now() time.Time {
	if preparer.Now != nil {
		return preparer.Now()
	}
	return time.Now()
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// backupFileInfo es el respaldo comprobado antes de empezar.
type backupFileInfo struct {
	path string
	name string
	size int64
}

// mtfSignature es el principio de todo respaldo de SQL Server (bloque TAPE del formato MTF).
const mtfSignature = "TAPE"

// checkInputs comprueba el respaldo, el destino y la frase de paso antes de arrancar nada.
func checkInputs(options Options) (backupFileInfo, error) {
	if err := legacypkg.ValidatePassphrase(options.Passphrase); err != nil {
		return backupFileInfo{}, fail(CodePassphraseInvalid, "", err)
	}
	if options.Backup == "" || options.Output == "" {
		return backupFileInfo{}, fail(CodeUsage, "", nil)
	}
	path, err := filepath.Abs(options.Backup)
	if err == nil {
		path, err = filepath.EvalSymlinks(path)
	}
	if err != nil {
		return backupFileInfo{}, fail(CodeBackupUnreadable, "", err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return backupFileInfo{}, fail(CodeBackupUnreadable, "", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return backupFileInfo{}, fail(CodeBackupUnreadable, "", err)
	}
	signature := make([]byte, len(mtfSignature))
	_, readErr := io.ReadFull(file, signature)
	_ = file.Close()
	if readErr != nil || string(signature) != mtfSignature {
		return backupFileInfo{}, fail(CodeBackupInvalid, "el archivo no empieza como un respaldo de SQL Server", readErr)
	}
	// El destino se prueba creando y quitando su temporal: terminación, que no exista y que se pueda escribir.
	probe, err := legacypkg.CreateOutput(options.Output)
	if errors.Is(err, legacypkg.ErrOutputExists) {
		return backupFileInfo{}, fail(CodeOutputExists, "", err)
	}
	if err != nil {
		return backupFileInfo{}, fail(CodeOutputInvalid, "", err)
	}
	probe.Abort()
	return backupFileInfo{path: path, name: filepath.Base(path), size: info.Size()}, nil
}

// fileDigest es el SHA-256 del respaldo (queda en el manifiesto para saber de qué archivo salió el paquete).
func fileDigest(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, contextReader{ctx: ctx, source: file}); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Prepare restaura el respaldo en un SQL Server temporal, extrae todas sus tablas y escribe el paquete cifrado.
// Pase lo que pase (error, cancelación o pánico) elimina el contenedor, su volumen, la copia del respaldo, las tablas
// en preparación y el paquete a medias.
func (preparer *Preparer) Prepare(ctx context.Context, options Options) (summary Summary, err error) {
	started := preparer.now()
	timings := map[string]float64{}
	backup, err := checkInputs(options)
	if err != nil {
		return Summary{}, err
	}
	sleep := preparer.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	ready := options.ReadyTimeout
	if ready == 0 {
		ready = 5 * time.Minute
	}
	pageRows := options.PageRows
	if pageRows <= 0 {
		pageRows = DefaultPageRows
	}
	box := newContainer(preparer.Runner, preparer.Events, preparer.TempDir, sleep, ready)
	writer, err := legacypkg.NewWriter(legacypkg.WriterOptions{SpoolDir: preparer.TempDir, KDF: options.KDF})
	if err != nil {
		return Summary{}, fail(CodeWriteFailed, "no se pudo preparar la carpeta temporal", err)
	}
	var output *legacypkg.Output
	defer func() {
		recovered := recover()
		if output != nil {
			output.Abort()
		}
		_ = writer.Close()
		_ = box.cleanup()
		if recovered != nil {
			summary = Summary{}
			err = fail(CodeInternal, fmt.Sprint("pánico: ", recovered), nil)
		}
	}()
	step := func(name string, work func() error) error {
		preparer.Events.Emit(stepStarted(name))
		stepStart := preparer.now()
		if err := work(); err != nil {
			return err
		}
		timings[name] = preparer.now().Sub(stepStart).Seconds()
		preparer.Events.Emit(stepFinished(name, timings[name]))
		return nil
	}
	if err := box.checkDocker(ctx); err != nil {
		return Summary{}, err
	}
	var image imageInfo
	if err := step(StepImage, func() (stepErr error) {
		image, stepErr = box.ensureImage(ctx)
		return stepErr
	}); err != nil {
		return Summary{}, err
	}
	if err := step(StepStart, func() error {
		if err := box.removeLeftovers(ctx); err != nil {
			return err
		}
		if err := box.start(ctx, image.ID, backup.path, backup.size); err != nil {
			return err
		}
		return box.waitReady(ctx)
	}); err != nil {
		return Summary{}, err
	}
	var set backupSet
	readerVersion := ""
	if err := step(StepRestore, func() error {
		var stepErr error
		set, stepErr = box.readHeader(ctx)
		if stepErr != nil {
			return stepErr
		}
		files, stepErr := box.readFiles(ctx, set.Position)
		if stepErr != nil {
			return stepErr
		}
		if stepErr := box.restore(ctx, set.Position, files); stepErr != nil {
			return stepErr
		}
		readerVersion = box.readerVersion(ctx)
		return nil
	}); err != nil {
		return Summary{}, err
	}
	sales := &SalesRange{}
	counts := []TableCount{}
	if err := step(StepRead, func() error {
		tables, stepErr := box.listTables(ctx)
		if stepErr != nil {
			return stepErr
		}
		for index, source := range tables {
			plan, planErr := planTable(source)
			if planErr != nil {
				return planErr
			}
			progress := tableReading{done: index, total: len(tables)}
			if plan.skipped != "" {
				rows, countErr := box.countTable(ctx, plan)
				if countErr != nil {
					return countErr
				}
				if skipErr := writer.SkipTable(legacypkg.SkippedTable{Name: plan.name, Rows: rows, Reason: plan.skipped}); skipErr != nil {
					return fail(CodeWriteFailed, "", skipErr)
				}
				preparer.Events.Emit(TableEvent{
					Event:     "table",
					Table:     plan.name,
					Status:    "skipped",
					Done:      index + 1,
					Total:     len(tables),
					TableRows: rows,
				})
				continue
			}
			preparer.Events.Emit(TableEvent{Event: "table", Table: plan.name, Status: "reading", Done: index, Total: len(tables)})
			rows, readErr := box.readTable(ctx, plan, writer, pageRows, progress, sales)
			if readErr != nil {
				return readErr
			}
			counts = append(counts, TableCount{Name: plan.name, Rows: rows})
			preparer.Events.Emit(TableEvent{
				Event:     "table",
				Table:     plan.name,
				Status:    "done",
				Done:      index + 1,
				Total:     len(tables),
				Rows:      rows,
				TableRows: rows,
			})
		}
		return nil
	}); err != nil {
		return Summary{}, err
	}
	var result legacypkg.Result
	if err := step(StepWrite, func() error {
		digest, digestErr := fileDigest(ctx, backup.path)
		if ctx.Err() != nil {
			return fail(CodeCancelled, "", ctx.Err())
		}
		if digestErr != nil {
			return fail(CodeBackupUnreadable, "", digestErr)
		}
		var createErr error
		output, createErr = legacypkg.CreateOutput(options.Output)
		if errors.Is(createErr, legacypkg.ErrOutputExists) {
			return fail(CodeOutputExists, "", createErr)
		}
		if createErr != nil {
			return fail(CodeOutputInvalid, "", createErr)
		}
		info := legacypkg.PackageInfo{
			CreatedAt: preparer.now(),
			Creator:   legacypkg.Creator{Product: product.Read().Name, Version: product.Read().Version, Tool: "legacy-prepare"},
			Source: legacypkg.Source{
				Kind:               legacypkg.SourceSQLServerBackup,
				BackupFile:         backup.name,
				BackupBytes:        backup.size,
				BackupSHA256:       digest,
				DatabaseName:       set.DatabaseName,
				BackupStartedAt:    set.StartedAt,
				BackupFinishedAt:   set.FinishedAt,
				ServerVersion:      set.ServerVersion,
				DatabaseVersion:    set.DatabaseVersion,
				CompatibilityLevel: set.CompatibilityLevel,
				Collation:          set.Collation,
				BackupSetPosition:  set.Position,
				ReaderImage:        image.Digest,
				ReaderImageID:      image.ID,
				ReaderVersion:      readerVersion,
			},
		}
		if sales.First != "" {
			info.Summary = &legacypkg.Summary{SalesTable: legacypkg.SalesTable, SalesFirst: sales.First, SalesLast: sales.Last}
		}
		var finishErr error
		result, finishErr = writer.Finish(output, options.Passphrase, info)
		if ctx.Err() != nil {
			return fail(CodeCancelled, "", ctx.Err())
		}
		if finishErr != nil {
			return fail(CodeWriteFailed, "", finishErr)
		}
		commitErr := output.Commit()
		if errors.Is(commitErr, legacypkg.ErrOutputExists) {
			return fail(CodeOutputExists, "", commitErr)
		}
		if commitErr != nil {
			return fail(CodeWriteFailed, "", commitErr)
		}
		return nil
	}); err != nil {
		return Summary{}, err
	}
	if err := step(StepCleanup, func() error {
		_ = writer.Close()
		if cleanupErr := box.cleanup(); cleanupErr != nil {
			// El paquete ya está completo: el lector temporal que no se pudo borrar se elimina al empezar la
			// siguiente preparación (por su etiqueta).
			preparer.Events.Emit(ProgressEvent{
				Event:   "progress",
				Step:    StepCleanup,
				Message: "No se pudo borrar el lector temporal; se borrará la próxima vez",
			})
		}
		return nil
	}); err != nil {
		return Summary{}, err
	}
	timings["total"] = preparer.now().Sub(started).Seconds()
	omitted := []OmittedColumnSummary{}
	for _, table := range result.Manifest.Tables {
		for _, column := range table.OmittedColumns {
			omitted = append(omitted, OmittedColumnSummary{
				Table:         table.Name,
				Column:        column.Name,
				Reason:        column.Reason,
				RowsWithValue: column.RowsWithValue,
			})
		}
	}
	summary = Summary{
		Output:             output.Path(),
		PackageBytes:       result.Bytes,
		ContentFingerprint: result.Manifest.ContentFingerprint,
		Backup: BackupSummary{
			File:          backup.name,
			Bytes:         backup.size,
			Database:      set.DatabaseName,
			StartedAt:     set.StartedAt,
			FinishedAt:    set.FinishedAt,
			ServerVersion: set.ServerVersion,
		},
		Tables:         counts,
		SkippedTables:  result.Manifest.SkippedTables,
		OmittedColumns: omitted,
		Timings:        timings,
	}
	if sales.First != "" {
		summary.Sales = sales
	}
	return summary, nil
}
