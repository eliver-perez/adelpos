//go:build !windows

package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/standalone"
)

// Etapa 9G, fase A: el servidor deja de servir cuando la computadora deja de ser servidor (se quita el
// archivo de habilitación o la modalidad pasa a «Solo terminal») y solo se reinicia si launchd lo va a
// relanzar.
func TestWatchServiceStopsWhenTheComputerStopsBeingServer(t *testing.T) {
	previous := serviceWatchInterval
	serviceWatchInterval = 20 * time.Millisecond
	defer func() {
		serviceWatchInterval = previous
	}()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	for _, change := range []string{"habilitación", "modalidad"} {
		layout := standalone.Layout{DataDir: t.TempDir()}
		if err := standalone.WriteMode(layout, standalone.ModeServerOnly); err != nil {
			t.Fatal(err)
		}
		if err := standalone.SetServiceEnabled(layout, true); err != nil {
			t.Fatal(err)
		}
		ctx, stop := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			watchService(ctx, layout, logger, stop)
			close(done)
		}()
		time.Sleep(100 * time.Millisecond)
		if ctx.Err() != nil {
			t.Fatal("con servidor habilitado se sigue sirviendo")
		}
		if change == "habilitación" {
			if err := standalone.SetServiceEnabled(layout, false); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := standalone.WriteMode(layout, standalone.ModeTerminalOnly); err != nil {
				t.Fatal(err)
			}
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("el cambio de %s no detuvo el servidor", change)
		}
		if ctx.Err() == nil {
			t.Fatalf("el cambio de %s debe cancelar el servidor", change)
		}
	}
}

func TestServiceControlRestartsOnlyWhenLaunchdRelaunches(t *testing.T) {
	previous := restartDelay
	restartDelay = 10 * time.Millisecond
	defer func() {
		restartDelay = previous
	}()
	layout := standalone.Layout{DataDir: t.TempDir()}
	if err := standalone.WriteMode(layout, standalone.ModeServerTerminal); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	control := &serviceControl{
		layout:  layout,
		started: time.Now(),
		stop:    stop,
		logger:  slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	if control.Restart() {
		t.Fatal("sin launchd nadie lo relanzaría")
	}
	control.managed = true
	if control.Restart() {
		t.Fatal("sin archivo de habilitación launchd no lo relanzaría")
	}
	if err := standalone.SetServiceEnabled(layout, true); err != nil {
		t.Fatal(err)
	}
	status := control.HostStatus()
	if !status.Managed || !status.ServiceEnabled || status.InstallationMode != standalone.ModeServerTerminal {
		t.Fatalf("estado inesperado: %+v", status)
	}
	if !control.Restart() || !control.Restart() {
		t.Fatal("con launchd y habilitación el reinicio se programa (una sola vez aunque se pida dos)")
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("el reinicio debe detener el servidor después de responder")
	}
}
