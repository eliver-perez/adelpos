package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/httpapi"
	"local/ad-el-pos/server/internal/identity"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
	"local/ad-el-pos/server/internal/maintenance"
	"local/ad-el-pos/server/internal/product"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server_stopped", "reason", err.Error())
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	command := "serve"
	args := os.Args[1:]
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		command, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet(product.Read().ServerBinary, flag.ContinueOnError)
	filename := flags.String("config", ".local/server.json", "archivo de configuración JSON")
	lanIP := flags.String("lan-ip", "", "IP privada opcional para el certificado TLS local")
	pid := flags.Int("pid", 0, "PID del servidor que «stop» debe detener (solo Windows)")
	logFile := flags.String("log-file", "", "archivo de bitácora JSON en lugar de la salida estándar")
	// Comandos de la instalación (Windows desde la etapa 9E, macOS desde la 9G): carpeta de datos, binarios
	// de PostgreSQL, vigilancia de la aplicación que lanzó el servidor (solo Windows), respaldo que se
	// restaura y, en macOS, la configuración de la instalación anterior que adopt-legacy copia. --pg-port y
	// --listen solo los usan las pruebas, con una carpeta de datos nueva.
	host := hostOptions{}
	flags.StringVar(&host.dataDir, "data-dir", "", "carpeta de datos de la instalación")
	flags.StringVar(&host.pgBin, "pg-bin", "", "carpeta de los binarios de PostgreSQL")
	flags.BoolVar(&host.watchStdin, "watch-stdin", false, "detenerse cuando se cierre la entrada estándar")
	flags.StringVar(&host.file, "file", "", "respaldo que se comprueba o restaura")
	flags.BoolVar(&host.pause, "pause", false, "esperar Entrar antes de cerrar la ventana")
	flags.IntVar(&host.pgPort, "pg-port", 0, "puerto de PostgreSQL de una carpeta de datos nueva (pruebas)")
	flags.StringVar(&host.listen, "listen", "", "dirección de Go de una carpeta de datos nueva (pruebas)")
	flags.StringVar(&host.fromConfig, "from-config", "", "server.json de la instalación anterior (adopt-legacy)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("argumentos adicionales no permitidos")
	}
	// Etapa 9E: en Windows el servidor corre sin consola (procesos ocultos o lanzados por la aplicación),
	// así que su bitácora va a un archivo. El archivo queda abierto hasta que termina el proceso.
	if *logFile != "" {
		output, err := openLogFile(*logFile)
		if err != nil {
			return err
		}
		*logger = *slog.New(slog.NewJSONHandler(output, nil))
	}
	if command == "version" {
		fmt.Println(product.Read().Version)
		return nil
	}
	// Etapa 9E: en Windows no hay SIGTERM; «stop» activa el evento de parada del servidor indicado.
	if command == "stop" {
		return stopProcess(*pid)
	}
	// Comandos de la instalación: host, backup, restore-check y restore (Windows desde la etapa 9E, macOS
	// desde la 9G) y adopt-legacy (macOS). Con SIGTERM o SIGINT se detienen en orden.
	if hostCommands[command] {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		ctx, stopWatch := withPlatformStop(ctx)
		defer stopWatch()
		return runHostCommand(ctx, logger, command, host, *logFile != "")
	}
	allowedCommands := map[string]bool{
		"serve": true, "migrate": true, "check-config": true,
		"prepare-setup": true, "provision-sa": true, "reset-sa": true, "tls-init": true,
		"create-enrollment": true,
	}
	if !allowedCommands[command] {
		return errors.New("comando no disponible; consulta DEVELOPMENT_MACOS.md")
	}
	cfg, err := config.Load(*filename)
	if err != nil {
		return err
	}
	if command == "check-config" {
		logger.Info("configuration_valid", "environment", cfg.Environment, "mode", cfg.InstallationMode)
		return nil
	}
	if err := licensing.CheckStartup(cfg.Environment, cfg.LicenseMode); err != nil {
		return err
	}
	if command == "prepare-setup" {
		return prepareSetup(*filename)
	}
	if command == "tls-init" {
		return localsetup.CreateTLS(filepath.Join(filepath.Dir(*filename), "tls"), *lanIP)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	// En Windows también se detiene en orden con el evento de parada (stop_windows.go); en macOS no cambia.
	ctx, stopWatch := withPlatformStop(ctx)
	defer stopWatch()
	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return errors.New("no se pudo crear el pool PostgreSQL")
	}
	defer pool.Close()
	// Etapa 9F: el respaldo previo a «Quitar datos de prueba» usa pg_dump con la conexión de esta
	// configuración (en la instalación de Windows, el respaldo de la instalación; host_windows.go).
	application := newApplication(pool, *filename, maintenance.NewDumpBackup(cfg, *filename), logger)
	if command == "provision-sa" || command == "reset-sa" {
		return provisionSupport(ctx, application, command == "reset-sa")
	}
	if command == "create-enrollment" {
		return createLocalEnrollment(ctx, application, *filename)
	}
	if command == "migrate" {
		migrationCtx, done := context.WithTimeout(ctx, 30*time.Second)
		defer done()
		count, err := database.Migrate(migrationCtx, pool)
		if err != nil {
			return err
		}
		logger.Info("migrations_completed", "applied", count)
		return nil
	}
	return serveHTTP(ctx, logger, cfg, pool, application, cfg.TestMode)
}

// newApplication arma el servicio de administración que comparten serve y el modo host de Windows. El
// token de aprovisionamiento vive junto al archivo de configuración.
func newApplication(pool *pgxpool.Pool, configFile string, backups maintenance.Backuper, logger *slog.Logger) *administration.Service {
	return &administration.Service{
		Pool:             pool,
		ProvisioningFile: filepath.Join(filepath.Dir(configFile), "provisioning-token"),
		Backups:          backups,
		Logger:           logger,
		// Consultas de solo lectura de las colas CUPS del equipo para el cajón (etapa 7.1).
		Queues: kitchen.HostQueueInspector{},
		// Vista previa del logotipo en el diseñador de documentos (etapa 9B).
		Rasters: identity.NewRasters(pool),
	}
}

// serveHTTP sirve la API con los trabajadores de impresión e inventario hasta que se cancela el contexto
// (SIGTERM en macOS, también el servicio al reiniciarse o al dejar de ser servidor; evento de parada o
// cierre de la aplicación en Windows) y cierra en orden. testMode
// lo informa /api/v1/version (etapa 9F): test_mode de la configuración o ADEL_TEST_MODE en el host.
func serveHTTP(ctx context.Context, logger *slog.Logger, cfg config.Config, pool *pgxpool.Pool, application *administration.Service, testMode bool) error {
	probe := func(ctx context.Context) database.Health {
		return database.Probe(ctx, pool)
	}
	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           httpapi.NewWithOptions(httpapi.Options{TestMode: testMode}, probe, cfg.AllowedOrigins, logger, application),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return errors.New("no se pudo abrir el puerto HTTP; compruebe dirección y puerto")
	}
	// Al empezar el apagado los flujos de eventos (SSE) terminan; las demás peticiones terminan su
	// trabajo como siempre. Sin esto el cierre esperaba 5 s a un flujo abierto y salía con error.
	stopping := make(chan struct{})
	application.Stopping = stopping
	server.RegisterOnShutdown(func() {
		close(stopping)
	})
	workerContext, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		// Los documentos con logotipo cargan sus conversiones guardadas (etapa 9B).
		worker := &kitchen.Worker{Pool: pool, Transport: kitchen.HostTransport{}, Logger: logger, Rasters: identity.NewRasters(pool)}
		worker.Run(workerContext)
	}()
	// Trabajador de consumo de inventario (etapa 8): procesa las solicitudes de consumo y reconcilia
	// las cancelaciones vencidas; al iniciar procesa lo que quedó pendiente.
	inventoryDone := make(chan struct{})
	go func() {
		defer close(inventoryDone)
		(&inventory.Worker{Pool: pool, Logger: logger}).Run(workerContext)
	}()
	defer func() { stopWorker(); <-workerDone; <-inventoryDone }()
	stopped := make(chan error, 1)
	go func() {
		if cfg.TLSCertificate != "" {
			stopped <- server.ServeTLS(listener, cfg.TLSCertificate, cfg.TLSPrivateKey)
			return
		}
		stopped <- server.Serve(listener)
	}()
	logger.Info("server_started", "product", product.Read().TechnicalID, "version", product.Read().Version, "environment", cfg.Environment, "mode", cfg.InstallationMode, "listen", cfg.ListenAddress, "test_mode", testMode)
	select {
	case err := <-stopped:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("falló el servidor HTTP")
		}
	case <-ctx.Done():
		shutdownCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return errors.New("se agotó el cierre ordenado HTTP")
		}
	}
	logger.Info("server_shutdown_complete")
	return nil
}
