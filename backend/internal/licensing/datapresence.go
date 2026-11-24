package licensing

import (
	"context"
	"errors"
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

// --- Copia del historial de revisiones aceptadas (revisión R1, hallazgo R1-3) ---------------------------------
//
// app.license_revisions (0028, sección «Fase A (revisión R1)») guarda, por license_id, la revisión más alta aceptada
// y su sha256: una segunda copia de lo que guarda estado.json. La revisión mínima aceptable es el máximo de las dos
// (manager.go). La de la base viaja en los respaldos, pero restaurar uno viejo solo baja esa copia, nunca la de
// identity/; y borrar o editar estado.json no toca la de la base. No reemplaza a L1-D9: es una protección más.

// AcceptedRevisions lee la copia de la base.
func (probe PoolProbe) AcceptedRevisions(ctx context.Context) (map[string]AcceptedRevision, error) {
	revisions := map[string]AcceptedRevision{}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	rows, err := probe.Pool.Query(ctx, `SELECT license_id::text, license_revision, sha256 FROM app.license_revisions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var licenseID, sha256 string
		var revision int64
		if err := rows.Scan(&licenseID, &revision, &sha256); err != nil {
			return nil, err
		}
		revisions[licenseID] = AcceptedRevision{Revision: revision, SHA256: sha256}
	}
	return revisions, rows.Err()
}

// RecordAcceptedRevision sube la copia de la base de una licencia aceptada; nunca la baja ni la reemplaza por una
// revisión igual con otro contenido (la §10 ya lo rechazó antes).
func (probe PoolProbe) RecordAcceptedRevision(ctx context.Context, licenseID string, revision int64, sha256 string) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	_, err := probe.Pool.Exec(ctx, `INSERT INTO app.license_revisions(license_id, license_revision, sha256)
        VALUES ($1, $2, $3)
        ON CONFLICT (license_id) DO UPDATE
            SET license_revision = EXCLUDED.license_revision, sha256 = EXCLUDED.sha256, updated_at = now()
            WHERE app.license_revisions.license_revision < EXCLUDED.license_revision`, licenseID, revision, sha256)
	return err
}

// --- Copia de la última hora confiable (revisión R2, hallazgo R2-3) --------------------------------------------
//
// app.license_trusted_time (0028, sección «Fase A (revisión R1)») tiene a lo más una fila: la segunda copia de la
// última hora confiable de estado.json, con su fuente. La que vale es el máximo de las dos (manager.go). Viaja en los
// respaldos, pero restaurar uno viejo solo baja esta copia, nunca la de identity/; y editar o borrar estado.json no toca
// esta. Solo la §6.3 y la §6.4 la bajan (ReplaceTrustedTimeCopy).

// trustedTimeRaiseStatement sube la copia; nunca la baja.
const trustedTimeRaiseStatement = `INSERT INTO app.license_trusted_time(id, trusted_time, source) VALUES (1, $1, $2)
    ON CONFLICT (id) DO UPDATE SET trusted_time = EXCLUDED.trusted_time, source = EXCLUDED.source, updated_at = now()
        WHERE app.license_trusted_time.trusted_time < EXCLUDED.trusted_time`

// trustedTimeReplaceStatement la reemplaza, también hacia atrás.
const trustedTimeReplaceStatement = `INSERT INTO app.license_trusted_time(id, trusted_time, source) VALUES (1, $1, $2)
    ON CONFLICT (id) DO UPDATE SET trusted_time = EXCLUDED.trusted_time, source = EXCLUDED.source, updated_at = now()`

// TrustedTimeCopy lee la copia de la base, con falso si todavía no hay ninguna.
func (probe PoolProbe) TrustedTimeCopy(ctx context.Context) (TrustedCopy, bool, error) {
	var value TrustedCopy
	var found bool
	err := probe.readOnly(ctx, func(ctx context.Context, querier rowQuerier) error {
		err := querier.QueryRow(ctx, `SELECT trusted_time, source FROM app.license_trusted_time WHERE id = 1`).
			Scan(&value.Time, &value.Source)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		value.Time = value.Time.UTC()
		found = true
		return nil
	})
	return value, found, err
}

// RaiseTrustedTimeCopy sube la copia de la base.
func (probe PoolProbe) RaiseTrustedTimeCopy(ctx context.Context, value TrustedCopy) error {
	return probe.writeTrustedTime(ctx, trustedTimeRaiseStatement, value)
}

// ReplaceTrustedTimeCopy reemplaza la copia de la base, también hacia atrás (§6.3 y §6.4).
func (probe PoolProbe) ReplaceTrustedTimeCopy(ctx context.Context, value TrustedCopy) error {
	return probe.writeTrustedTime(ctx, trustedTimeReplaceStatement, value)
}

// writeTrustedTime escribe la fila en una transacción corta propia, con plazo y lock_timeout: la escritura de cada
// minuto nunca espera a un cambio de estructura.
func (probe PoolProbe) writeTrustedTime(ctx context.Context, statement string, value TrustedCopy) error {
	if value.Time.IsZero() {
		return errors.New("la última hora confiable no es válida")
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	transaction, err := probe.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = transaction.Rollback(context.Background())
	}()
	if _, err := transaction.Exec(ctx, "SET LOCAL lock_timeout = '2s'"); err != nil {
		return err
	}
	if _, err := transaction.Exec(ctx, statement, value.Time.UTC(), value.Source); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}
