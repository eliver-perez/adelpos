//go:build integration

package reports_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"local/ad-el-pos/server/internal/reports"
)

// El ejecutor de reportes consulta en una transacción REPEATABLE READ READ ONLY con sus límites de
// tiempo, sin locks consultivos; un intento de escritura falla; con el semáforo lleno responde
// REPORTS_BUSY sin esperar; una consulta cancelada por tiempo responde REPORT_TIMEOUT.
func TestReportRunnerIsReadOnlyAndBounded(t *testing.T) {
	ctx, pool := openPool(t)
	runner := reports.NewRunner(pool, reports.Slots)
	err := runner.Read(ctx, func(tx pgx.Tx) error {
		var isolation, readOnly, statement, lock, idle string
		if err := tx.QueryRow(ctx, `SELECT current_setting('transaction_isolation'),current_setting('transaction_read_only'),
                current_setting('statement_timeout'),current_setting('lock_timeout'),
                current_setting('idle_in_transaction_session_timeout')`).Scan(&isolation, &readOnly, &statement, &lock, &idle); err != nil {
			return err
		}
		if isolation != "repeatable read" || readOnly != "on" || statement != "30s" || lock != "2s" || idle != "45s" {
			t.Fatalf("Transacción de reportes: %s %s %s %s %s", isolation, readOnly, statement, lock, idle)
		}
		var advisory int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory'`).Scan(&advisory); err != nil {
			return err
		}
		if advisory != 0 {
			t.Fatalf("El reporte tomó %d locks consultivos", advisory)
		}
		_, err := tx.Exec(ctx, `INSERT INTO app.registers(name) VALUES ('Escritura desde un reporte')`)
		return err
	})
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) || databaseError.Code != "25006" {
		t.Fatalf("Una escritura en la transacción de reportes debía fallar por solo lectura: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM app.registers WHERE name='Escritura desde un reporte'").Scan(&count); err != nil || count != 0 {
		t.Fatal("La escritura de un reporte quedó registrada")
	}
	first, _ := runner.Acquire()
	second, _ := runner.Acquire()
	err = runner.Read(ctx, func(pgx.Tx) error {
		t.Fatal("Con el semáforo lleno la consulta no debía ejecutarse")
		return nil
	})
	var problem *reports.Error
	if !errors.As(err, &problem) || problem.Code != "REPORTS_BUSY" || problem.Status != 503 {
		t.Fatalf("Con el semáforo lleno se esperaba REPORTS_BUSY: %v", err)
	}
	if _, ok := runner.Acquire(); ok {
		t.Fatal("El semáforo admitió un tercer reporte")
	}
	first()
	second()
	err = runner.Read(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout='50ms'"); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), "SELECT pg_sleep(1)")
		return err
	})
	if !errors.As(err, &problem) || problem.Code != "REPORT_TIMEOUT" {
		t.Fatalf("Una consulta que excede su tiempo debía responder REPORT_TIMEOUT: %v", err)
	}
	if err := runner.Read(ctx, func(pgx.Tx) error { return nil }); err != nil {
		t.Fatalf("El semáforo no se liberó: %v", err)
	}
}
