package catalog

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// QuoteForOrder reutiliza exactamente el motor de previsualización en la transacción del pedido.
// El llamador autentica al actor y autoriza captura/confirmación antes de invocarlo.
func QuoteForOrder(ctx context.Context, tx pgx.Tx, input PreviewRequest) (*Quote, Snapshot, error) {
	if input.OverrideAvailability {
		return nil, Snapshot{}, invalid("La confirmación de pedidos debe respetar la disponibilidad vigente.")
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7820148304)"); err != nil {
		return nil, Snapshot{}, err
	}
	if len(input.Lines) < 1 || len(input.Lines) > 20 {
		return nil, Snapshot{}, invalid("Un pedido admite entre 1 y 20 partidas por confirmación.")
	}
	snapshot, err := loadSnapshot(ctx, tx, input)
	if err != nil {
		return nil, snapshot, err
	}
	location, err := restaurantLocation(ctx, tx)
	if err != nil {
		return nil, snapshot, err
	}
	quote, err := Calculate(snapshot, input, time.Now(), location)
	return quote, snapshot, err
}
