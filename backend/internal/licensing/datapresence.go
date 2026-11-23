package licensing

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Lo que el gestor consulta de la base (§6 del contrato y §2 de la especificación).
//
// **Qué cuenta como operación** para distinguir «Sin activar» de «Sin activar con datos»: cualquier registro
// de venta, cobro, turno o inventario, es decir, al menos una fila en
//
//   - app.restaurant_accounts (una cuenta abierta, cobrada, cancelada o juntada; también para llevar);
//   - app.cash_shifts (un turno de caja, abierto o cerrado);
//   - app.payments (un cobro);
//   - app.inventory_movements (un movimiento de inventario, también una existencia inicial);
//   - app.legacy_sales (una venta importada del sistema anterior, que se consulta en los reportes).
//
// No cuentan la configuración, el catálogo, los usuarios, las terminales, las sesiones ni la auditoría: el
// asistente inicial y el catálogo de prueba los crean en una instalación nueva.
//
// **Cómo se consulta, sin tomar locks del negocio**: un SELECT con EXISTS en una transacción de solo lectura.
// Un SELECT solo toma el lock AccessShare de cada tabla, que no espera ni bloquea a las escrituras del negocio
// ni a sus locks consultivos (7820148305 y los demás de CLAUDE.md §3); solo esperaría a un cambio de
// estructura (una migración), y para eso lleva lock_timeout. EXISTS se detiene en la primera fila.

// operationsQuery responde si hay al menos una operación (ver arriba).
const operationsQuery = `SELECT
    EXISTS (SELECT 1 FROM app.restaurant_accounts)
    OR EXISTS (SELECT 1 FROM app.cash_shifts)
    OR EXISTS (SELECT 1 FROM app.payments)
    OR EXISTS (SELECT 1 FROM app.inventory_movements)
    OR EXISTS (SELECT 1 FROM app.legacy_sales)`

// latestActivityQuery es la última actividad registrada: casi toda operación se audita y audit_events tiene
// el índice descendente audit_occurred_idx (avance §0.5.8).
const latestActivityQuery = `SELECT max(occurred_at) FROM app.audit_events`

// probeTimeout limita cada consulta: el arranque y el trabajador del gestor nunca esperan a la base.
const probeTimeout = 5 * time.Second

// PoolProbe es la consulta de la base del servidor (Options.Database).
type PoolProbe struct {
	Pool *pgxpool.Pool
}

// rowQuerier es lo que necesitan las consultas: el pool o una transacción (las pruebas usan una que se
// deshace).
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// HasOperations indica si la base tiene operaciones.
func (probe PoolProbe) HasOperations(ctx context.Context) (bool, error) {
	var found bool
	err := probe.readOnly(ctx, func(ctx context.Context, querier rowQuerier) error {
		value, err := hasOperations(ctx, querier)
		found = value
		return err
	})
	return found, err
}

// LatestActivity es la hora de la última actividad registrada, con falso si no hay ninguna.
func (probe PoolProbe) LatestActivity(ctx context.Context) (time.Time, bool, error) {
	var latest time.Time
	var found bool
	err := probe.readOnly(ctx, func(ctx context.Context, querier rowQuerier) error {
		value, present, err := latestActivity(ctx, querier)
		latest, found = value, present
		return err
	})
	return latest, found, err
}

// readOnly corre la consulta en una transacción de solo lectura, con plazo y lock_timeout.
func (probe PoolProbe) readOnly(ctx context.Context, query func(context.Context, rowQuerier) error) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	transaction, err := probe.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() {
		_ = transaction.Rollback(context.Background())
	}()
	if _, err := transaction.Exec(ctx, "SET LOCAL lock_timeout = '2s'"); err != nil {
		return err
	}
	return query(ctx, transaction)
}

func hasOperations(ctx context.Context, querier rowQuerier) (bool, error) {
	var found bool
	if err := querier.QueryRow(ctx, operationsQuery).Scan(&found); err != nil {
		return false, err
	}
	return found, nil
}

func latestActivity(ctx context.Context, querier rowQuerier) (time.Time, bool, error) {
	var latest *time.Time
	if err := querier.QueryRow(ctx, latestActivityQuery).Scan(&latest); err != nil {
		return time.Time{}, false, err
	}
	if latest == nil {
		return time.Time{}, false, nil
	}
	return latest.UTC(), true, nil
}
