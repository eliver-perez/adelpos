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

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/httpapi"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
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
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("argumentos adicionales no permitidos")
	}
	if command == "version" {
		fmt.Println(product.Read().Version)
		return nil
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
	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return errors.New("no se pudo crear el pool PostgreSQL")
	}
	defer pool.Close()
	application := &administration.Service{
		Pool:             pool,
		ProvisioningFile: filepath.Join(filepath.Dir(*filename), "provisioning-token"),
		// Consultas de solo lectura de las colas CUPS del equipo para el cajón (etapa 7.1).
		Queues: kitchen.HostQueueInspector{},
	}
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
	probe := func(ctx context.Context) database.Health {
		return database.Probe(ctx, pool)
	}
	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           httpapi.New(probe, cfg.AllowedOrigins, logger, application),
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
	workerContext, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		(&kitchen.Worker{Pool: pool, Transport: kitchen.HostTransport{}, Logger: logger}).Run(workerContext)
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
	logger.Info("server_started", "product", product.Read().TechnicalID, "version", product.Read().Version, "environment", cfg.Environment, "mode", cfg.InstallationMode, "listen", cfg.ListenAddress)
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
