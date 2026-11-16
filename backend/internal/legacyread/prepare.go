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
	// Now y Sleep permiten probar sin reloj real; ProcessAlive, sin procesos reales (nil = el sistema).
	Now          func() time.Time
	Sleep        func(ctx context.Context, duration time.Duration) error
	ProcessAlive func(pid int) bool
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
	path     string
	name     string
	size     int64
	modified time.Time
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
	return backupFileInfo{path: path, name: filepath.Base(path), size: info.Size(), modified: info.ModTime()}, nil
}

// unchanged comprueba que el respaldo sigue igual que al empezar (tamaño y fecha de modificación): si alguien lo
// reemplazó o lo seguía copiando mientras se leía, el paquete no correspondería a su SHA-256.
func (backup backupFileInfo) unchanged() error {
	info, err := os.Stat(backup.path)
	if err != nil {
		return fail(CodeBackupUnreadable, "", err)
	}
	if info.Size() != backup.size || !info.ModTime().Equal(backup.modified) {
		return fail(CodeBackupChanged, "", nil)
	}
	return nil
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
//
// Al terminar (bien, con error, con Ctrl-C o SIGTERM, al cerrarse la entrada con --watch-stdin o con un pánico) elimina
// el contenedor, su volumen, la copia del respaldo, las tablas en preparación y el paquete a medias. Si el proceso
// termina a la fuerza (SIGKILL o el cierre forzado de Windows) no puede limpiar: lo que quedó lo borra la siguiente
// preparación o legacy-prepare --cleanup (workspace.go).
func (preparer *Preparer) Prepare(ctx context.Context, options Options) (summary Summary, err error) {
	started := preparer.now()
	timings := map[string]float64{}
	backup, err := checkInputs(options)
	if err != nil {
		return Summary{}, err
	}
	// Una sola preparación a la vez: dos se destruirían los contenedores (y se repartirían la memoria de Docker).
	space, err := openWorkspace(preparer.TempDir)
	if err != nil {
		return Summary{}, err
	}
	defer space.close()
	leftovers := CleanupReport{}
	if staleErr := space.removeStale(&leftovers); staleErr != nil {
		preparer.Events.Emit(ProgressEvent{
			Event:   "progress",
			Step:    StepStart,
			Message: "No se pudieron borrar algunos restos de una preparación anterior",
		})
	}
	run, err := space.newRun()
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
	box := newContainer(preparer.Runner, preparer.Events, run, sleep, ready, preparer.ProcessAlive)
	writer, err := legacypkg.NewWriter(legacypkg.WriterOptions{SpoolDir: run, KDF: options.KDF})
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
	digest := ""
	if err := step(StepStart, func() error {
		if err := box.removeLeftovers(ctx, &leftovers); err != nil {
			return err
		}
		// El SHA-256 se calcula antes de que SQL Server lea el respaldo; al final se comprueba que no cambió.
		var digestErr error
		digest, digestErr = fileDigest(ctx, backup.path)
		if ctx.Err() != nil {
			return fail(CodeCancelled, "", ctx.Err())
		}
		if digestErr != nil {
			return fail(CodeBackupUnreadable, "", digestErr)
		}
		if err := box.start(ctx, image.ID, backup.path, backup.size); err != nil {
			// Un respaldo que cambió de tamaño (se seguía copiando) tampoco se ve completo dentro del contenedor.
			if changed := backup.unchanged(); changed != nil {
				return changed
			}
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
		return preparer.readTables(ctx, box, writer, tables, pageRows, sales, &counts)
	}); err != nil {
		return Summary{}, err
	}
	var result legacypkg.Result
	if err := step(StepWrite, func() error {
		if err := backup.unchanged(); err != nil {
			return err
		}
		var createErr error
		output, createErr = legacypkg.CreateOutput(options.Output)
		if errors.Is(createErr, legacypkg.ErrOutputExists) {
			return fail(CodeOutputExists, "", createErr)
		}
		if createErr != nil {
			return fail(CodeOutputInvalid, "", createErr)
		}
		if err := space.recordPartial(output.TemporaryPath()); err != nil {
			return fail(CodeWriteFailed, "no se pudo registrar el paquete a medias", err)
		}
		info := packageInfo(preparer.now(), backup, digest, set, image, readerVersion, sales)
		var finishErr error
		result, finishErr = writer.Finish(output, options.Passphrase, info)
		if ctx.Err() != nil {
			return fail(CodeCancelled, "", ctx.Err())
		}
		if finishErr != nil {
			return fail(CodeWriteFailed, "", finishErr)
		}
		// El paquete escrito se abre y se recorre completo antes de darle su nombre: un paquete con nombre siempre se
		// puede importar.
		verifyStart := preparer.now()
		if err := verifyWritten(ctx, output, options.Passphrase, result.Manifest); err != nil {
			return err
		}
		timings["verify"] = preparer.now().Sub(verifyStart).Seconds()
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
			// siguiente preparación (su proceso ya no existirá).
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
	return buildSummary(output, result, backup, set, sales, counts, timings, leftovers), nil
}

// readTables lee todas las tablas del catálogo en el paquete (las excluidas solo se cuentan).
func (preparer *Preparer) readTables(
	ctx context.Context,
	box *container,
	writer *legacypkg.Writer,
	tables []sourceTable,
	pageRows int,
	sales *SalesRange,
	counts *[]TableCount,
) error {
	for index, source := range tables {
		plan, err := planTable(source)
		if err != nil {
			return err
		}
		progress := tableReading{done: index, total: len(tables)}
		if plan.skipped != "" {
			rows, err := box.countTable(ctx, plan)
			if err != nil {
				return err
			}
			if err := writer.SkipTable(legacypkg.SkippedTable{Name: plan.name, Rows: rows, Reason: plan.skipped}); err != nil {
				return fail(CodeWriteFailed, "", err)
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
		rows, err := box.readTable(ctx, plan, writer, pageRows, progress, sales)
		if err != nil {
			return err
		}
		*counts = append(*counts, TableCount{Name: plan.name, Rows: rows})
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
}

// packageInfo son los datos del manifiesto que no salen de las tablas.
func packageInfo(
	now time.Time,
	backup backupFileInfo,
	digest string,
	set backupSet,
	image imageInfo,
	readerVersion string,
	sales *SalesRange,
) legacypkg.PackageInfo {
	info := legacypkg.PackageInfo{
		CreatedAt: now,
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
			BackupSets:         set.Sets,
			ReaderImage:        image.Digest,
			ReaderImageID:      image.ID,
			ReaderVersion:      readerVersion,
		},
	}
	if sales.First != "" {
		info.Summary = &legacypkg.Summary{SalesTable: legacypkg.SalesTable, SalesFirst: sales.First, SalesLast: sales.Last}
	}
	return info
}

// verifyWritten recorre el paquete recién escrito con la misma frase de paso y comprueba que su contenido es el que se
// escribió.
func verifyWritten(ctx context.Context, output *legacypkg.Output, passphrase string, expected legacypkg.Manifest) error {
	if err := output.Sync(); err != nil {
		return fail(CodeWriteFailed, "", err)
	}
	file, err := os.Open(output.TemporaryPath())
	if err != nil {
		return fail(CodeWriteFailed, "no se pudo abrir el paquete escrito para comprobarlo", err)
	}
	defer file.Close()
	manifest, err := legacypkg.Verify(contextReader{ctx: ctx, source: file}, passphrase, legacypkg.OpenOptions{})
	if ctx.Err() != nil {
		return fail(CodeCancelled, "", ctx.Err())
	}
	if err != nil {
		return fail(CodeWriteFailed, "el paquete escrito no pasó la comprobación", err)
	}
	if manifest.ContentFingerprint != expected.ContentFingerprint {
		return fail(CodeWriteFailed, "el paquete escrito no tiene el contenido esperado", nil)
	}
	return nil
}

// buildSummary arma el resumen final.
func buildSummary(
	output *legacypkg.Output,
	result legacypkg.Result,
	backup backupFileInfo,
	set backupSet,
	sales *SalesRange,
	counts []TableCount,
	timings map[string]float64,
	leftovers CleanupReport,
) Summary {
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
	summary := Summary{
		Output:             output.Path(),
		PackageBytes:       result.Bytes,
		ContentFingerprint: result.Manifest.ContentFingerprint,
		Verified:           true,
		Backup: BackupSummary{
			File:          backup.name,
			Bytes:         backup.size,
			Database:      set.DatabaseName,
			StartedAt:     set.StartedAt,
			FinishedAt:    set.FinishedAt,
			ServerVersion: set.ServerVersion,
			Sets:          set.Sets,
		},
		Tables:            counts,
		SkippedTables:     result.Manifest.SkippedTables,
		OmittedColumns:    omitted,
		Timings:           timings,
		PreviousLeftovers: leftovers,
	}
	if sales.First != "" {
		summary.Sales = sales
	}
	return summary
}

// Cleanup borra los restos de preparaciones que terminaron a la fuerza (legacy-prepare --cleanup; la aplicación lo
// llama al abrir el asistente): las carpetas de trabajo, el paquete a medias que registraron y, si Docker está en
// marcha, los contenedores y volúmenes del lector cuyo proceso ya no existe. Con una preparación en curso responde
// ALREADY_RUNNING y no toca nada. dockerChecked dice si se pudo revisar Docker.
func (preparer *Preparer) Cleanup(ctx context.Context) (report CleanupReport, dockerChecked bool, err error) {
	space, err := openWorkspace(preparer.TempDir)
	if err != nil {
		return CleanupReport{}, false, err
	}
	defer space.close()
	if err := space.removeStale(&report); err != nil {
		return report, false, err
	}
	if preparer.Runner == nil {
		return report, false, nil
	}
	box := newContainer(preparer.Runner, preparer.Events, space.root, sleepContext, 0, preparer.ProcessAlive)
	if err := box.checkDocker(ctx); err != nil {
		return report, false, nil
	}
	if err := box.removeLeftovers(ctx, &report); err != nil {
		return report, false, err
	}
	return report, true, nil
}
