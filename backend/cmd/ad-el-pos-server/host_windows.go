//go:build windows

package main

// Comandos de la instalación de Windows (etapa 9E, §3.1 y §3.8):
//
//   - host: lo lanza la aplicación instalada. Prepara la carpeta de datos y PostgreSQL, respalda antes de
//     aplicar migraciones sobre datos existentes, sirve igual que «serve» y al terminar detiene PostgreSQL.
//     Con --watch-stdin se detiene cuando la aplicación cierra su entrada estándar (también si la
//     aplicación termina mal: Windows cierra la tubería).
//   - backup, restore-check y restore: respaldos con los pg_dump y pg_restore incluidos.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/product"
	"local/ad-el-pos/server/internal/standalone"
)

var hostCommands = map[string]bool{"host": true, "backup": true, "restore-check": true, "restore": true}

func runHostCommand(ctx context.Context, logger *slog.Logger, command string, options hostOptions, logToFile bool) error {
	layout, err := standalone.NewLayout(options.dataDir, options.pgBin, product.Read().Name)
	if err != nil {
		return err
	}
	// Modo de pruebas (etapa 9F, decisión D1): con ADEL_TEST_MODE=1 ningún comando de la instalación usa
	// la carpeta de datos real del usuario. La prueba de humo abre la aplicación instalada con otra
	// carpeta (LOCALAPPDATA temporal); la aplicación hereda la variable y se la pasa a este proceso.
	testMode, err := hostTestMode(layout)
	if err != nil {
		return err
	}
	if command == "host" {
		return runHost(ctx, logger, layout, options, logToFile, testMode)
	}
	// En la consola solo se muestran mensajes para el restaurante; el detalle técnico va a la bitácora.
	if !logToFile {
		if output, err := openLogFile(layout.MaintenanceLog()); err == nil {
			*logger = *slog.New(slog.NewJSONHandler(output, nil))
		}
	}
	err = runMaintenance(ctx, logger, command, layout, options)
	if err != nil {
		fmt.Println("No se pudo completar: " + err.Error())
	}
	if options.pause {
		fmt.Print("\nPresiona Entrar para cerrar esta ventana…")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	return err
}

// hostTestMode lee ADEL_TEST_MODE: «1» activa el modo de pruebas y exige una carpeta de datos que no sea
// la real del usuario; cualquier otro valor distinto de vacío es un error (no se adivina la intención).
func hostTestMode(layout standalone.Layout) (bool, error) {
	value := os.Getenv("ADEL_TEST_MODE")
	if value == "" {
		return false, nil
	}
	if value != "1" {
		return false, errors.New("ADEL_TEST_MODE solo admite el valor 1")
	}
	if err := standalone.RefuseRealDataDir(layout, product.Read().Name); err != nil {
		return false, err
	}
	return true, nil
}

func runHost(ctx context.Context, logger *slog.Logger, layout standalone.Layout, options hostOptions, logToFile bool, testMode bool) error {
	// El servidor de la instalación corre sin consola: su bitácora va a la carpeta de datos.
	if !logToFile {
		output, err := openLogFile(layout.ServerLog())
		if err != nil {
			return err
		}
		*logger = *slog.New(slog.NewJSONHandler(output, nil))
	}
	lock, err := standalone.AcquireLock(layout)
	if err != nil {
		return err
	}
	defer lock.Release()
	if options.watchStdin {
		watched, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			// La aplicación nunca escribe: la lectura termina cuando cierra la tubería o deja de existir.
			_, _ = io.Copy(io.Discard, os.Stdin)
			logger.Info("host_parent_closed")
			cancel()
		}()
		ctx = watched
	}
	started := time.Now()
	logger.Info("host_starting", "version", product.Read().Version, "data_dir", layout.DataDir, "test_mode", testMode)
	cluster := standalone.NewCluster(layout, logger)
	// PostgreSQL se detiene al final pase lo que pase, también si la preparación falló después de iniciarlo.
	defer func() {
		stopContext, done := context.WithTimeout(context.Background(), 90*time.Second)
		defer done()
		_ = cluster.Stop(stopContext)
	}()
	cfg, err := standalone.Prepare(ctx, cluster, product.Read().DatabaseName)
	if err != nil {
		return err
	}
	if err := licensing.CheckStartup(cfg.Environment, cfg.LicenseMode); err != nil {
		return err
	}
	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return errors.New("no se pudo crear el pool PostgreSQL")
	}
	defer pool.Close()
	if err := migrateWithBackup(ctx, logger, layout, cfg, pool); err != nil {
		return err
	}
	logger.Info("host_ready", "duration_ms", time.Since(started).Milliseconds())
	backups := installationBackup{layout: layout, database: cfg.Database}
	return serveHTTP(ctx, logger, cfg, pool, newApplication(pool, layout.ConfigFile(), backups, logger), testMode)
}

// installationBackup crea el respaldo previo a «Quitar datos de prueba» (etapa 9F, D2) con el respaldo de
// la instalación: los pg_dump y pg_restore incluidos, la carpeta backups de los datos y la ACL privada.
type installationBackup struct {
	layout   standalone.Layout
	database config.Database
}

func (backup installationBackup) Backup(ctx context.Context) (string, error) {
	filename, err := standalone.Backup(ctx, backup.layout, backup.database, "antes-de-quitar-datos-de-prueba")
	if err != nil {
		return "", err
	}
	return filepath.Base(filename), nil
}

// migrateWithBackup aplica las migraciones pendientes. Sobre una base que ya tiene datos (alguna migración
// aplicada) primero crea un respaldo comprobado: nunca se migra la base del restaurante sin él.
func migrateWithBackup(ctx context.Context, logger *slog.Logger, layout standalone.Layout, cfg config.Config, pool *pgxpool.Pool) error {
	applied, pending, err := database.MigrationStatus(ctx, pool)
	if err != nil {
		return err
	}
	if pending == 0 {
		return nil
	}
	if applied > 0 {
		filename, err := standalone.Backup(ctx, layout, cfg.Database, "antes-de-migrar")
		if err != nil {
			return err
		}
		logger.Info("premigration_backup_created", "file", filepath.Base(filename), "applied", applied, "pending", pending)
	}
	migrationContext, done := context.WithTimeout(ctx, 5*time.Minute)
	defer done()
	count, err := database.Migrate(migrationContext, pool)
	if err != nil {
		return err
	}
	logger.Info("migrations_completed", "applied", count)
	return nil
}

// runMaintenance ejecuta un respaldo, un ensayo de restauración o una restauración. Si AD-EL POS está
// abierto, PostgreSQL ya corre y el respaldo y el ensayo usan esa instancia; si está cerrado, el comando
// toma el bloqueo de la carpeta, arranca PostgreSQL y lo vuelve a detener. Restaurar exige la aplicación
// cerrada: reemplaza la base completa.
func runMaintenance(ctx context.Context, logger *slog.Logger, command string, layout standalone.Layout, options hostOptions) error {
	if _, err := os.Stat(layout.ConfigFile()); err != nil {
		return errors.New("todavía no hay datos de AD-EL POS en este equipo; abre la aplicación una vez")
	}
	cfg, err := config.Load(layout.ConfigFile())
	if err != nil {
		return err
	}
	lock, err := standalone.AcquireLock(layout)
	running := errors.Is(err, standalone.ErrLocked)
	if err != nil && !running {
		return err
	}
	if running && command == "restore" {
		return errors.New("cierra AD-EL POS antes de restaurar un respaldo")
	}
	if !running {
		defer lock.Release()
		cluster := standalone.NewCluster(layout, logger)
		if err := cluster.Start(ctx); err != nil {
			return err
		}
		defer func() {
			stopContext, done := context.WithTimeout(context.Background(), 90*time.Second)
			defer done()
			_ = cluster.Stop(stopContext)
		}()
	}
	switch command {
	case "backup":
		filename, err := standalone.Backup(ctx, layout, cfg.Database, "")
		if err != nil {
			return err
		}
		fmt.Println("Respaldo creado y comprobado:")
		fmt.Println("  " + filename)
		return nil
	case "restore-check":
		file := options.file
		if file == "" {
			latest := standalone.LatestBackups(layout)
			if len(latest) == 0 {
				return errors.New("no hay respaldos en la carpeta de respaldos")
			}
			file = filepath.Join(layout.Backups(), latest[0])
		}
		report, err := standalone.RestoreCheck(ctx, layout, cfg.Database, file)
		if err != nil {
			return err
		}
		if !report.Equal {
			return fmt.Errorf("el respaldo %s se restauró, pero sus conteos difieren de la base actual (%d tablas)", report.Backup, len(report.Tables))
		}
		fmt.Printf("Ensayo de restauración correcto: %s, %d tablas con conteos idénticos.\n", report.Backup, len(report.Tables))
		return nil
	case "restore":
		if options.file == "" {
			return errors.New("indica el respaldo con --file; ninguno se restaura por omisión")
		}
		safety, counts, err := standalone.Restore(ctx, layout, cfg.Database, options.file)
		if err != nil {
			return err
		}
		fmt.Printf("Respaldo restaurado: %d tablas. Respaldo de seguridad del estado anterior: %s\n", len(counts), filepath.Base(safety))
		return nil
	}
	return errors.New("comando de mantenimiento desconocido")
}
