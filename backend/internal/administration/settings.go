package administration

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const settingsQuery = "SELECT name,timezone,currency,phone,email,address,theme,configured_at,updated_at FROM app.establishment"

func settings(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	key := "settings.read"
	if request.Method == "PUT" {
		key = "settings.manage"
	}
	if err := principal.require(key); err != nil {
		return nil, err
	}
	if request.Method == "GET" {
		return object(ctx, tx, settingsQuery)
	}
	if request.Method != "PUT" {
		return nil, invalid("Método no válido para configuración.")
	}
	var input EstablishmentInput
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(); err != nil {
		return nil, err
	}
	previous, err := object(ctx, tx, settingsQuery+" FOR UPDATE")
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE app.establishment
        SET name=$1,timezone=$2,currency=$3,phone=$4,email=$5,address=$6,theme=$7,updated_at=now()`,
		input.Name, input.Timezone, input.Currency, input.Phone, input.Email, input.Address, input.Theme,
	)
	if err != nil {
		return nil, err
	}
	next, err := object(ctx, tx, settingsQuery)
	if err != nil {
		return nil, err
	}
	return next, audit(ctx, tx, principal, "settings.updated", "establishment", nil, previous, next, true)
}
