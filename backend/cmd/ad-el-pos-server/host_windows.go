//go:build windows

package main

// Comandos de la instalación de Windows (etapa 9E, §3.1 y §3.8):
//
//   - host: lo lanza la aplicación instalada. Prepara la carpeta de datos y PostgreSQL, respalda antes de
//     aplicar migraciones sobre datos existentes, sirve igual que «serve» y al terminar detiene PostgreSQL.
//     Con --watch-stdin se detiene cuando la aplicación cierra su entrada estándar (también si la
//     aplicación termina mal: Windows cierra la tubería).
//   - backup, restore-check y restore: respaldos con los pg_dump y pg_restore incluidos (host.go).
//
// El servicio de Windows, la modalidad y el paso de datos de la instalación por usuario son de la Fase W
// de la etapa 9G (en macOS: host_unix.go).

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
	"local/ad-el-pos/server/internal/product"
	"local/ad-el-pos/server/internal/standalone"
)

var hostCommands = map[string]bool{"host": true, "backup": true, "restore-check": true, "restore": true}

// noInstallationDataMessage explica que la carpeta de datos todavía no existe.
const noInstallationDataMessage = "todavía no hay datos de AD-EL POS en este equipo; abre la aplicación una vez"

func runHostCommand(ctx context.Context, logger *slog.Logger, command string, options hostOptions, logToFile bool) error {
	layout, err := hostLayout(options)
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
	return runConsoleMaintenance(ctx, logger, command, layout, options, logToFile)
}

// pauseServerForRestore: en Windows el servidor vive con la aplicación; para restaurar hay que cerrarla.
func pauseServerForRestore(context.Context, *slog.Logger, standalone.Layout) (*standalone.HostLock, func(), error) {
	return nil, nil, errors.New("cierra AD-EL POS antes de restaurar un respaldo")
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
	// Etapa 9G, fase B: la identidad del servidor se crea en la carpeta de datos, pero en Windows todavía
	// no hay escucha de red (servicio, modalidad y regla del firewall son de la Fase W).
	network := networkOptions{IdentityDir: filepath.Join(layout.DataDir, localsetup.IdentityFolder)}
	return serveHTTP(ctx, logger, cfg, pool, newApplication(pool, layout.ConfigFile(), backups, logger), testMode, network)
}
