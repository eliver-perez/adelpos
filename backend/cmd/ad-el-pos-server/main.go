package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/httpapi"
	"local/ad-el-pos/server/internal/licensing"
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
	if command != "serve" && command != "migrate" && command != "check-config" {
		return errors.New("comando esperado: serve, migrate, check-config o version")
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return errors.New("no se pudo crear el pool PostgreSQL")
	}
	defer pool.Close()
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
	server := &http.Server{Addr: cfg.ListenAddress, Handler: httpapi.New(func(ctx context.Context) database.Health { return database.Probe(ctx, pool) }, cfg.AllowedOrigins, logger), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return errors.New("no se pudo abrir el puerto HTTP; compruebe dirección y puerto")
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
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
