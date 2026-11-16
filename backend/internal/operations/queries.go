package operations

import (
	"context"
	"encoding/json"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
	"time"

	"github.com/jackc/pgx/v5"
)

func readOne[T any](ctx context.Context, tx pgx.Tx, query string, arguments ...any) (*T, error) {
	var encoded []byte
	if err := tx.QueryRow(ctx, query, arguments...).Scan(&encoded); err != nil {
		return nil, err
	}
	var result T
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func readList[T any](ctx context.Context, tx pgx.Tx, query string, arguments ...any) ([]T, error) {
	rows, err := tx.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []T{}
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var value T
		if err := json.Unmarshal(encoded, &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (handler *service) location() (*time.Location, error) {
	var timezone string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT timezone FROM app.establishment").Scan(&timezone); err != nil {
		return nil, err
	}
	return time.LoadLocation(timezone)
}

func (handler *service) touch(id string, revision int) error {
	result, err := handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_accounts SET revision=revision+1,updated_at=now()
        WHERE id=$1 AND revision=$2 AND state='open'`, id, revision)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return conflict()
	}
	return nil
}

func (handler *service) event(accountID string, relatedID *string, kind string, before, after any) error {
	payload := map[string]any{"before": before, "after": after}
	eventID := newID()
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.restaurant_events
        (id,account_id,related_account_id,operation_id,kind,payload,author_id,device_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, eventID, accountID, relatedID, handler.operationID, kind,
		jsonValue(payload), handler.request.Actor.UserID, handler.request.Actor.DeviceID)
	if err != nil {
		return err
	}
	if err := kitchen.Integrate(handler.ctx, handler.tx, eventID, accountID, kind, jsonValue(payload), kitchen.Actor{
		UserID: handler.request.Actor.UserID, DeviceID: handler.request.Actor.DeviceID, Permissions: handler.request.Actor.Permissions,
	}); err != nil {
		return err
	}
	var mode string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT mode FROM app.restaurant_accounts WHERE id=$1", accountID).Scan(&mode); err != nil {
		return err
	}
	scope := "restaurant"
	if mode == "takeaway" {
		scope = "takeaway"
	}
	if err := events.Append(handler.ctx, handler.tx, "restaurant."+kind, scope, &accountID, nil); err != nil {
		return err
	}
	return handler.audit(handler.ctx, handler.tx, "restaurant."+kind, "account", &accountID, before, after)
}

func ensureOpen(account *Account, revision int) error {
	if account.State != "open" {
		return accountNotOpen(account)
	}
	if account.Revision != revision {
		return conflict()
	}
	return nil
}

// AccountNotOpenReason es error.details.reason del rechazo de una operación sobre una cuenta que ya no está abierta.
const AccountNotOpenReason = "account_not_open"

// accountNotOpen rechaza una operación sobre una cuenta cobrada, cancelada o combinada mientras se capturaba (etapa
// A1, §6.4). Conserva el estado y el código de siempre (400 OPERATION_VALIDATION, compatible con el escritorio) y
// agrega en details el motivo y el estado de la cuenta, para que el teléfono lleve el pedido a «Requiere tu revisión»
// («La cuenta «Mesa 4» ya se cobró») en lugar de mostrar un error genérico. Con merged, la cuenta en que se combinó.
func accountNotOpen(account *Account) error {
	return &Error{
		Status:  400,
		Code:    "OPERATION_VALIDATION",
		Message: accountNotOpenMessage(account) + " No admite nuevas operaciones.",
		Details: map[string]any{
			"reason":         AccountNotOpenReason,
			"account_id":     account.ID,
			"account_state":  account.State,
			"merged_into_id": account.MergedIntoID,
		},
	}
}

// accountNotOpenMessage dice en palabras del restaurante qué pasó con la cuenta.
func accountNotOpenMessage(account *Account) string {
	switch account.State {
	case "closed":
		return "La cuenta «" + account.Name + "» ya se cobró."
	case "voided":
		return "La cuenta «" + account.Name + "» ya se canceló o se liberó."
	case "merged":
		return "La cuenta «" + account.Name + "» se combinó con otra cuenta."
	}
	return "La cuenta «" + account.Name + "» ya no está abierta."
}

func (handler *service) authorizeAccount(account *Account, write bool) error {
	if account.Mode == "takeaway" {
		permission := "takeaway.read"
		if write {
			permission = "takeaway.manage"
		}
		return handler.request.Actor.Require(permission)
	}
	return nil
}
