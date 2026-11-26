package kitchen

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/events"
)

// Etapa K1.1 (K11-D4): una cuenta cobrada, cerrada o cancelada no deja nada en «Por enviar a cocina». «Ya no está
// abierta» se decide por la pertenencia actual de las unidades: un pedido combinado, dividido o transferido sigue
// pendiente en la cuenta abierta donde quedaron sus unidades, y un pedido para llevar cobrado y todavía no entregado
// sigue abierto (su pedido programado sigue llegando a la cocina a su hora). Lo que se da por terminado solo cambia de
// estado, con su registro en el historial de cocina: una liberación pasa a «cancelled» y un aviso pendiente recibe
// dismissed_at. Nunca se tocan ventas, pagos, cuentas, cortes ni inventario. La migración 0030 aplica la misma regla a
// lo que ya existía; estas mismas condiciones están escritas allí.

// releaseOpenSQL: alguna unidad activa de la liberación r sigue en una cuenta abierta.
const releaseOpenSQL = `EXISTS(SELECT FROM app.restaurant_item_units u JOIN app.restaurant_accounts a ON a.id=u.account_id
        WHERE u.id=ANY(r.unit_ids) AND u.state='active' AND a.state='open')`

// changeUnitsSQL son las unidades que nombra el aviso pendiente c (modificadas, agregadas y canceladas).
const changeUnitsSQL = `coalesce(c.payload->'modified_unit_ids','[]')||coalesce(c.payload->'added_unit_ids','[]')||
        coalesce(c.payload->'cancelled_unit_ids','[]')||coalesce(c.payload->'unit_ids','[]')`

// changeOpenSQL: el aviso pendiente c sigue siendo de una cuenta abierta. Alguna de sus unidades activas está en una
// cuenta abierta, o su cuenta (o aquella en la que terminó combinada, también en cadena) sigue abierta: una
// cancelación deja sus unidades canceladas en la cuenta original, que al combinarse ya no es la abierta.
const changeOpenSQL = `(EXISTS(SELECT FROM app.restaurant_item_units u JOIN app.restaurant_accounts a ON a.id=u.account_id
        WHERE u.state='active' AND a.state='open' AND u.id::text IN (SELECT jsonb_array_elements_text(` + changeUnitsSQL + `)))
    OR EXISTS(WITH RECURSIVE chain(id,depth) AS (
            SELECT c.account_id,0
            UNION ALL
            SELECT a.merged_into_id,chain.depth+1 FROM chain JOIN app.restaurant_accounts a ON a.id=chain.id
            WHERE a.state='merged' AND a.merged_into_id IS NOT NULL AND chain.depth<32)
        SELECT FROM chain JOIN app.restaurant_accounts a ON a.id=chain.id WHERE a.state='open'))`

// accountFamilySQL es la cuenta $1 y las que terminaron combinadas en ella, también en cadena: sus pendientes siguen
// apuntando a la cuenta original aunque sus unidades se hayan movido al combinar.
const accountFamilySQL = `family(id) AS (
        SELECT $1::uuid
        UNION
        SELECT a.id FROM app.restaurant_accounts a JOIN family f ON a.merged_into_id=f.id)`

// Motivos con que se da por terminado un pendiente (after_value.reason del historial y dismissed_reason).
const (
	dismissedAccountClosed    = "account_closed"
	dismissedAccountCancelled = "account_cancelled"
)

func releaseNotPending() error {
	return &Error{Status: 409, Code: "KITCHEN_RELEASE_NOT_PENDING", Message: "Este pedido ya no está pendiente."}
}

func changeNotPending() error {
	return &Error{Status: 409, Code: "KITCHEN_CHANGE_NOT_PENDING", Message: "Este aviso ya no está pendiente."}
}

// releaseStillPending dice si la liberación todavía espera ir a la cocina: no se envió ni se canceló y alguna de sus
// unidades activas sigue en una cuenta abierta.
func releaseStillPending(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	var pending bool
	err := tx.QueryRow(ctx, `SELECT r.state IN ('pending','scheduled','blocked') AND `+releaseOpenSQL+`
        FROM app.kitchen_releases r WHERE r.id=$1`, id).Scan(&pending)
	return pending, err
}

// AccountClosed da por terminado lo pendiente de cocina de una cuenta que se acaba de cerrar (payments.closeIfSettled,
// en su misma transacción y con el lock 7820148305 que ya tomó la operación de cobro), con el actor de esa operación.
func AccountClosed(ctx context.Context, tx pgx.Tx, accountID string, actor Actor) error {
	handler := service{ctx: ctx, tx: tx, request: Request{Actor: actor}, preparation: map[string]bool{}}
	if err := handler.dismissAccountPending(accountID, dismissedAccountClosed); err != nil {
		return err
	}
	return handler.publishPreparation()
}

// dismissAccountPending da por terminadas las liberaciones y los avisos pendientes de la cuenta (y de las combinadas en
// ella) que ya no son de una cuenta abierta: la liberación pasa a «cancelled» (release.dismissed) y el aviso recibe
// dismissed_at (change.dismissed), cada uno con su fila en el historial con el motivo y el actor de la operación.
// Publica un evento de cocina para que el panel se actualice. No crea tareas, comandas ni consumos.
func (handler *service) dismissAccountPending(accountID, reason string) error {
	releases, err := handler.tx.Query(handler.ctx, `WITH RECURSIVE `+accountFamilySQL+`,
        stale AS (
            SELECT r.id,r.state,r.warnings FROM app.kitchen_releases r
            WHERE r.state IN ('pending','scheduled','blocked')
                AND (r.account_id IN (SELECT id FROM family) OR EXISTS(SELECT FROM app.restaurant_item_units u
                    WHERE u.id=ANY(r.unit_ids) AND u.account_id IN (SELECT id FROM family)))
                AND NOT `+releaseOpenSQL+`
            FOR UPDATE OF r)
        UPDATE app.kitchen_releases r SET state='cancelled',revision=r.revision+1 FROM stale
        WHERE r.id=stale.id AND r.state IN ('pending','scheduled','blocked')
        RETURNING r.id::text,stale.state,stale.warnings`, accountID)
	if err != nil {
		return err
	}
	type dismissed struct {
		id       string
		state    string
		warnings json.RawMessage
	}
	cancelled, err := pgx.CollectRows(releases, func(row pgx.CollectableRow) (dismissed, error) {
		var entry dismissed
		err := row.Scan(&entry.id, &entry.state, &entry.warnings)
		return entry, err
	})
	if err != nil {
		return err
	}
	changes, err := handler.tx.Query(handler.ctx, `WITH RECURSIVE `+accountFamilySQL+`,
        stale AS (
            SELECT c.event_id,c.kind,c.warnings FROM app.kitchen_pending_changes c
            WHERE c.dismissed_at IS NULL
                AND (c.account_id IN (SELECT id FROM family) OR EXISTS(SELECT FROM app.restaurant_item_units u
                    WHERE u.account_id IN (SELECT id FROM family)
                    AND u.id::text IN (SELECT jsonb_array_elements_text(`+changeUnitsSQL+`))))
                AND NOT `+changeOpenSQL+`
            FOR UPDATE OF c)
        UPDATE app.kitchen_pending_changes c SET dismissed_at=now(),dismissed_reason=$2 FROM stale
        WHERE c.event_id=stale.event_id AND c.dismissed_at IS NULL
        RETURNING c.event_id::text,c.account_id::text,stale.kind,stale.warnings`, accountID, reason)
	if err != nil {
		return err
	}
	type dismissedChange struct {
		eventID   string
		accountID string
		kind      string
		warnings  json.RawMessage
	}
	finished, err := pgx.CollectRows(changes, func(row pgx.CollectableRow) (dismissedChange, error) {
		var entry dismissedChange
		err := row.Scan(&entry.eventID, &entry.accountID, &entry.kind, &entry.warnings)
		return entry, err
	})
	if err != nil {
		return err
	}
	for _, release := range cancelled {
		before := map[string]any{"state": release.state, "warnings": release.warnings}
		after := map[string]any{"state": "cancelled", "reason": reason, "account_id": accountID}
		if err := handler.history("release.dismissed", &release.id, before, after); err != nil {
			return err
		}
	}
	for _, change := range finished {
		before := map[string]any{"kind": change.kind, "warnings": change.warnings}
		after := map[string]any{"dismissed": true, "reason": reason, "account_id": accountID}
		if err := handler.history("change.dismissed", &change.eventID, before, after); err != nil {
			return err
		}
		// Sin el aviso pendiente, la cuenta deja de decir «Cambio pendiente en cocina».
		handler.notePreparationAccount(change.accountID)
	}
	if len(cancelled)+len(finished) == 0 {
		return nil
	}
	return events.Append(handler.ctx, handler.tx, "kitchen.releases.changed", "kitchen", nil, nil)
}
