//go:build integration

package administration

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/database"
)

// TestDeniedOperationRollsBackItsWrites verifica la regla "toda autorización antes de la primera
// escritura" de forma independiente de cada módulo: una operación que escribe y después termina
// en 403 no confirma nada salvo la auditoría de la denegación.
func TestDeniedOperationRollsBackItsWrites(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("No se pudo crear el pool de prueba")
	}
	// Se cierra después de la limpieza de filas, que se registra más adelante (orden LIFO).
	t.Cleanup(pool.Close)
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// La identidad reservada existe desde la migración 0002; se usa solo como actor auditado.
	var supportID string
	if err := pool.QueryRow(ctx, "SELECT id FROM app.users WHERE username='sa'").Scan(&supportID); err != nil {
		t.Fatal(err)
	}
	principal := Principal{UserID: supportID, Username: "sa", Support: true}
	const marker = "Diagnóstico H2"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM app.registers WHERE name LIKE $1", marker+"%")
	})

	// execute reproduce la decisión de commit de Service.Execute sobre runProtected.
	execute := func(operation func(pgx.Tx) (any, error)) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		_, operationError := runProtected(ctx, tx, principal, Request{Method: "POST"}, "diagnostico_h2/escritura", operation)
		if shouldCommit(operationError) {
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
		return operationError
	}
	write := func(work pgx.Tx, name string) {
		if _, err := work.Exec(ctx, "INSERT INTO app.registers(name) VALUES ($1)", name); err != nil {
			t.Fatal(err)
		}
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var value int
		if err := pool.QueryRow(ctx, query, args...).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	denials := func() int {
		return count(`SELECT count(*) FROM app.audit_events WHERE action='access.denied'
            AND actor_id=$1 AND after_state->>'operation'='diagnostico_h2'`, supportID)
	}

	t.Run("escritura_seguida_de_403_se_revierte_y_audita", func(t *testing.T) {
		err := execute(func(work pgx.Tx) (any, error) {
			write(work, marker+" previa a denegación")
			return nil, denied()
		})
		var fault *Fault
		if !errors.As(err, &fault) || fault.Status != 403 {
			t.Fatalf("Se esperaba una denegación 403: %v", err)
		}
		if count("SELECT count(*) FROM app.registers WHERE name=$1", marker+" previa a denegación") != 0 {
			t.Fatal("Una escritura anterior a la denegación quedó confirmada")
		}
		if denials() != 1 {
			t.Fatal("La denegación no quedó auditada")
		}
	})
	t.Run("operacion_autorizada_confirma", func(t *testing.T) {
		err := execute(func(work pgx.Tx) (any, error) {
			write(work, marker+" autorizada")
			return "ok", nil
		})
		if err != nil || count("SELECT count(*) FROM app.registers WHERE name=$1", marker+" autorizada") != 1 {
			t.Fatalf("La operación autorizada no se confirmó: %v", err)
		}
	})
	t.Run("error_de_validacion_revierte_sin_auditar_denegacion", func(t *testing.T) {
		err := execute(func(work pgx.Tx) (any, error) {
			write(work, marker+" inválida")
			return nil, invalid("Validación de prueba.")
		})
		if err == nil || count("SELECT count(*) FROM app.registers WHERE name=$1", marker+" inválida") != 0 {
			t.Fatal("Un error de validación confirmó cambios")
		}
		if denials() != 1 {
			t.Fatal("Un error de validación se auditó como denegación")
		}
	})
}
