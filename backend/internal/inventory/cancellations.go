package inventory

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"local/ad-el-pos/server/internal/events"
)

// Clasificación de cancelaciones de unidades ya consumidas (§14 y §0-B «Cancelaciones sin
// clasificar»):
//   - «No preparado» (not_prepared): revierte exactamente el consumo neto registrado de la unidad
//     (consumption_reversal al valor original; con existencia negativa, el déficit cubierto se
//     realinea con un ajuste de costo separado, como en una recepción);
//   - «Preparado / merma» (prepared_waste): conserva el consumo y lo marca como merma; si antes se
//     había revertido, vuelve a consumir lo que esa clasificación revirtió (movimientos nuevos);
//   - hasta el corte del día operativo la clasifica inventory.waste.record; después, solo una
//     corrección con inventory.adjustments.authorize;
//   - el reconciliador aplica «No preparado» automático a las pendientes vencidas, idempotente.
// Los movimientos de una clasificación llevan como hora de negocio la de la cancelación (es cuando
// el insumo dejó o no de usarse) y como origen la clasificación (source_kind cancellation).

var classificationLabels = map[string]string{
	"pending":        "Pendiente de clasificar",
	"not_prepared":   "No preparado",
	"prepared_waste": "Preparado / merma",
}

var resolutionLabels = map[string]string{
	"manual":     "Clasificada por el encargado",
	"automatic":  "Automática al cierre del día operativo",
	"correction": "Corrección autorizada",
}

// classificationRow es una clasificación bloqueada para cambiarla.
type classificationRow struct {
	ID          string
	UnitID      string
	CancelledAt time.Time
	Deadline    time.Time
	State       string
	Resolution  string
	Revision    int
}

func lockClassification(ctx context.Context, tx pgx.Tx, id string) (*classificationRow, error) {
	var row classificationRow
	err := tx.QueryRow(ctx, `SELECT id,restaurant_unit_id,cancelled_at,deadline_at,state,coalesce(resolution,''),revision
        FROM app.inventory_cancellation_classifications WHERE id=$1 FOR UPDATE`, id).Scan(&row.ID, &row.UnitID, &row.CancelledAt,
		&row.Deadline, &row.State, &row.Resolution, &row.Revision)
	if isNoRows(err) {
		return nil, notFound("La cancelación no existe.")
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// classificationChange es el cambio que se aplica a una clasificación.
type classificationChange struct {
	state       string
	resolution  string
	actorID     *string
	operationID *string
	notes       *string
}

// applyClassification escribe los movimientos compensatorios del cambio y actualiza la
// clasificación. Devuelve cuántos movimientos escribió.
func applyClassification(ctx context.Context, tx pgx.Tx, row *classificationRow, change classificationChange) (int, error) {
	next := row.Revision + 1
	plan := consumptionTarget{
		unitID: row.UnitID, entryPrefix: "classification:" + row.ID + ":" + itoa(next), sourceKind: "cancellation", sourceID: row.ID,
		occurredAt: row.CancelledAt, actorID: change.actorID, process: workerProcess, operationID: change.operationID,
	}
	unitState := "consumed"
	switch change.state {
	case "not_prepared":
		// Objetivo cero: revierte exactamente el neto registrado.
		plan.reason = "Cancelación no preparada"
		plan.target = []resolvedEntry{}
		unitState = "reversed"
	case "prepared_waste":
		// Vuelve a consumir lo que revirtieron clasificaciones anteriores (si hubo); sin ellas no hay
		// movimientos: el consumo original permanece y se marca como merma.
		plan.reason = "Cancelación preparada (merma)"
		reversed, err := unitNet(ctx, tx, row.UnitID, []string{"cancellation"})
		if err != nil {
			return 0, err
		}
		current, err := unitNet(ctx, tx, row.UnitID, nil)
		if err != nil {
			return 0, err
		}
		for key, entry := range current {
			restore := entry.quantity.Sub(reversed[key].quantity)
			plan.target = append(plan.target, resolvedEntry{Key: key, Quantity: restore})
		}
	}
	written, err := adjustUnitConsumption(ctx, tx, plan)
	if err != nil {
		return 0, err
	}
	var actor *string
	if change.resolution != "automatic" {
		actor = change.actorID
	}
	tag, err := tx.Exec(ctx, `UPDATE app.inventory_cancellation_classifications SET state=$2,resolution=$3,classified_by=$4,
            classified_at=clock_timestamp(),notes=coalesce($5,notes),revision=$6,updated_at=clock_timestamp()
        WHERE id=$1 AND revision=$7`, row.ID, change.state, change.resolution, actor, change.notes, next, row.Revision)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() != 1 {
		return 0, revisionConflict("La cancelación")
	}
	_, err = tx.Exec(ctx, `UPDATE app.inventory_unit_consumptions SET state=$2,updated_at=clock_timestamp() WHERE restaurant_unit_id=$1`,
		row.UnitID, unitState)
	return written, err
}

// ReconcileCancellations aplica «No preparado» automático a toda clasificación pendiente cuyo
// corte ya pasó a la hora now. Cada una en su propia transacción con el lock de inventario; una que
// falle no detiene las demás. Idempotente: solo toma las que siguen pendientes. Audita con el
// proceso como responsable.
func ReconcileCancellations(ctx context.Context, pool *pgxpool.Pool, now time.Time) (int, error) {
	skipped := []string{}
	reconciled := 0
	for attempt := 0; attempt < 500; attempt++ {
		done, id, err := reconcileNext(ctx, pool, now, skipped)
		if err != nil {
			if ctx.Err() != nil {
				return reconciled, ctx.Err()
			}
			if id == "" {
				return reconciled, err
			}
			skipped = append(skipped, id)
			continue
		}
		if !done {
			break
		}
		reconciled++
	}
	return reconciled, nil
}

// reconcileNext reconcilia una clasificación vencida; devuelve false si no queda ninguna.
func reconcileNext(ctx context.Context, pool *pgxpool.Pool, now time.Time, skipped []string) (bool, string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, "", err
	}
	defer tx.Rollback(context.Background())
	if err := ensureLedgerLock(ctx, tx); err != nil {
		return false, "", err
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM app.inventory_cancellation_classifications
        WHERE state='pending' AND deadline_at<=$1 AND NOT (id=ANY($2::uuid[]))
        ORDER BY deadline_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`, now, skipped).Scan(&id)
	if isNoRows(err) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	row, err := lockClassification(ctx, tx, id)
	if err != nil {
		return false, id, err
	}
	written, err := applyClassification(ctx, tx, row, classificationChange{state: "not_prepared", resolution: "automatic"})
	if err != nil {
		return false, id, err
	}
	before := map[string]any{"state": row.State, "revision": row.Revision}
	after := map[string]any{"state": "not_prepared", "resolution": "automatic", "revision": row.Revision + 1, "deadline_at": row.Deadline,
		"reconciled_at": now, "movements": written}
	_, err = tx.Exec(ctx, `INSERT INTO app.audit_events(actor_id,actor_username,actor_kind,device_id,action,entity_type,entity_id,success,
            before_state,after_state)
        VALUES (NULL,$1,'process',NULL,'inventory.cancellation_reconciled','inventory_cancellation',$2,true,$3,$4)`,
		workerProcess, id, jsonValue(before), jsonValue(after))
	if err != nil {
		return false, id, err
	}
	if err := events.Append(ctx, tx, "inventory.alerts.changed", Scope, &id, nil); err != nil {
		return false, id, err
	}
	if written > 0 {
		if err := events.Append(ctx, tx, "inventory.stock.changed", Scope, &row.UnitID, nil); err != nil {
			return false, id, err
		}
		if err := events.Append(ctx, tx, "inventory.movements.recorded", Scope, &id, nil); err != nil {
			return false, id, err
		}
	}
	return true, id, tx.Commit(ctx)
}
