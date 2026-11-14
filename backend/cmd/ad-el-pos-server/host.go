package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/product"
	"local/ad-el-pos/server/internal/standalone"
)

// hostOptions son los indicadores de los comandos de la instalación: host, backup, restore-check,
// restore (Windows desde la etapa 9E y macOS desde la 9G) y adopt-legacy (macOS, etapa 9G). Se declaran en
// todas las plataformas para que el análisis de argumentos sea el mismo.
type hostOptions struct {
	dataDir    string
	pgBin      string
	file       string
	watchStdin bool
	pause      bool
	// pgPort y listen (etapa 9G) cambian el puerto de PostgreSQL y la dirección de Go de una carpeta de
	// datos nueva; solo las pruebas los usan (la instalación conserva 55432 y 127.0.0.1:8181).
	pgPort int
	listen string
	// fromConfig es el server.json de la instalación anterior que adopt-legacy copia (macOS).
	fromConfig string
}

// hostLayout arma las rutas de la instalación con los indicadores.
func hostLayout(options hostOptions) (standalone.Layout, error) {
	layout, err := standalone.NewLayout(options.dataDir, options.pgBin, product.Read().Name)
	if err != nil {
		return standalone.Layout{}, err
	}
	if options.pgPort != 0 {
		if options.pgPort < 1024 || options.pgPort > 65535 {
			return standalone.Layout{}, errors.New("--pg-port debe ser un puerto entre 1024 y 65535")
		}
		layout.Port = options.pgPort
	}
	layout.Listen = options.listen
	return layout, nil
}

// hostTestMode lee ADEL_TEST_MODE: «1» activa el modo de pruebas y exige una carpeta de datos que no sea
// la real; cualquier otro valor distinto de vacío es un error (no se adivina la intención).
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

// runConsoleMaintenance ejecuta backup, restore-check o restore desde la consola: solo mensajes para el
// restaurante en pantalla y el detalle técnico en la bitácora de mantenimiento.
func runConsoleMaintenance(ctx context.Context, logger *slog.Logger, command string, layout standalone.Layout, options hostOptions, logToFile bool) error {
	if !logToFile {
		if output, err := openLogFile(layout.MaintenanceLog()); err == nil {
			*logger = *slog.New(slog.NewJSONHandler(output, nil))
		}
	}
	err := runMaintenance(ctx, logger, command, layout, options)
	if err != nil {
		fmt.Println("No se pudo completar: " + err.Error())
	}
	if options.pause {
		fmt.Print("\nPresiona Entrar para cerrar esta ventana…")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	return err
}

// installationBackup crea el respaldo previo a «Quitar datos de prueba» (etapa 9F, D2) con el respaldo de
// la instalación: los pg_dump y pg_restore de la instalación, la carpeta backups de los datos y los
// permisos privados.
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

// runMaintenance ejecuta un respaldo, un ensayo de restauración o una restauración. Si el servidor está
// en marcha, PostgreSQL ya corre y el respaldo y el ensayo usan esa instancia; si no, el comando toma el
// bloqueo de la carpeta, arranca PostgreSQL y lo vuelve a detener. Restaurar reemplaza la base completa,
// así que exige el servidor detenido: en Windows se pide cerrar la aplicación y en macOS el comando pausa
// el servicio y lo reanuda al terminar (pauseServerForRestore, propio de cada sistema).
func runMaintenance(ctx context.Context, logger *slog.Logger, command string, layout standalone.Layout, options hostOptions) error {
	if _, err := os.Stat(layout.ConfigFile()); err != nil {
		return errors.New(noInstallationDataMessage)
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
		paused, resume, err := pauseServerForRestore(ctx, logger, layout)
		if err != nil {
			return err
		}
		// Se reanuda al final, después de soltar el bloqueo y de detener PostgreSQL (orden inverso de
		// los defer): el servicio relanzado encuentra la carpeta libre.
		defer resume()
		lock = paused
		running = false
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
