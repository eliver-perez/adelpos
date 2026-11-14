package administration

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Limpieza de las solicitudes de emparejamiento (revisión independiente, hallazgo 2): fuera de las peticiones,
// por lotes cortos y con el índice por created_at, para que ni muchas filas ni una limpieza larga bloqueen la
// creación de solicitudes (que toma un bloqueo de la tabla).

const pairingCleanupBatch = 500

// PairingCleanupInterval es cada cuánto el trabajador vuelve a limpiar (las pruebas lo acortan).
var PairingCleanupInterval = time.Hour

// CleanPairingRequests borra por lotes las solicitudes de más de pairingRetention y devuelve cuántas borró.
// Cada lote es una sentencia propia (sin una transacción larga).
func CleanPairingRequests(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var total int64
	for {
		result, err := pool.Exec(ctx, `DELETE FROM app.device_pairing_requests WHERE id IN (
                SELECT id FROM app.device_pairing_requests WHERE created_at < now() - $1::interval
                ORDER BY created_at LIMIT $2)`, pairingRetention.String(), pairingCleanupBatch)
		if err != nil {
			return total, err
		}
		total += result.RowsAffected()
		if result.RowsAffected() < pairingCleanupBatch {
			return total, nil
		}
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
	}
}

// PairingCleaner limpia al arrancar y después cada PairingCleanupInterval hasta que se cancela el contexto.
type PairingCleaner struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

func (cleaner *PairingCleaner) Run(ctx context.Context) {
	for {
		removed, err := CleanPairingRequests(ctx, cleaner.Pool)
		if err != nil && ctx.Err() == nil && cleaner.Logger != nil {
			cleaner.Logger.Warn("pairing_cleanup_failed", "reason", err.Error())
		}
		if removed > 0 && cleaner.Logger != nil {
			cleaner.Logger.Info("pairing_cleanup", "removed", removed)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(PairingCleanupInterval):
		}
	}
}
