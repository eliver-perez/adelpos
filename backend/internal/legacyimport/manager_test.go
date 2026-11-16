package legacyimport

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Si la base no responde, el registro del resultado de una corrida se reintenta mientras el trabajador siga vigente,
// pero al apagarse el servidor se hace un solo intento: el apagado no se alarga (revisión de la Fase C, M-1). Sin base:
// un puerto local donde nadie escucha.
func TestFinishRunRetriesOnlyWhileTheWorkerRuns(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nadie@127.0.0.1:1/adel_pos_test_sin_base?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var records bytes.Buffer
	manager := &Manager{Pool: pool, Logger: slog.New(slog.NewTextHandler(&records, nil))}
	failure := &Error{Code: CodeInterrupted, Message: interruptedMessage}
	attempts := func() int {
		return strings.Count(records.String(), "legacy_run_not_recorded")
	}

	// Apagándose: un intento y nada de espera.
	stopped, stop := context.WithCancel(context.Background())
	stop()
	started := time.Now()
	manager.finishRun(stopped, "00000000-0000-4000-8000-000000000001", StatusInterrupted, nil, failure, nil, nil, "")
	if elapsed := time.Since(started); attempts() != 1 || elapsed > 2*time.Second {
		t.Fatalf("al apagarse: %d intentos en %s", attempts(), elapsed)
	}

	// El trabajador se apaga a la mitad de las esperas: deja de reintentar en ese momento.
	records.Reset()
	running, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(700*time.Millisecond, cancel)
	defer timer.Stop()
	started = time.Now()
	manager.finishRun(running, "00000000-0000-4000-8000-000000000001", StatusInterrupted, nil, failure, nil, nil, "")
	if elapsed := time.Since(started); attempts() != 2 || elapsed > 3*time.Second {
		t.Fatalf("apagado durante los reintentos: %d intentos en %s", attempts(), elapsed)
	}
}
