package operations

import (
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/payments"
)

// releaseReason es el motivo fijo con que queda anulada una cuenta vacía liberada.
const releaseReason = "Cuenta vacía liberada"

// accountEmpty aplica la definición estricta de cuenta vacía de la etapa 7.1 (decisión 7): sin
// partidas en el borrador, sin unidades en ningún estado (tampoco canceladas), sin pedidos confirmados
// en ella aunque sus unidades se hayan transferido, sin haber recibido una combinación, sin registros
// financieros (payments) y sin registros de cocina (kitchen). Cada módulo responde por sus tablas.
func (handler *service) accountEmpty(accountID string) (bool, error) {
	var operational bool
	err := handler.tx.QueryRow(handler.ctx, `SELECT EXISTS(SELECT FROM app.restaurant_draft_items WHERE account_id=$1)
        OR EXISTS(SELECT FROM app.restaurant_item_units WHERE account_id=$1)
        OR EXISTS(SELECT FROM app.restaurant_orders WHERE original_account_id=$1)
        OR EXISTS(SELECT FROM app.restaurant_accounts WHERE merged_into_id=$1)`, accountID).Scan(&operational)
	if err != nil || operational {
		return false, err
	}
	financial, err := payments.HasAccountRecords(handler.ctx, handler.tx, accountID)
	if err != nil || financial {
		return false, err
	}
	preparation, err := kitchen.HasAccountRecords(handler.ctx, handler.tx, accountID)
	if err != nil {
		return false, err
	}
	return !preparation, nil
}

// releasable indica si «Liberar mesa» procede. Las cuentas con productos activos o borrador se
// descartan sin consultar más tablas: la comprobación completa solo corre para las candidatas.
func (handler *service) releasable(account *Account) (bool, error) {
	if account.State != "open" || account.ConfirmedQuantity > 0 || account.DraftQuantity > 0 {
		return false, nil
	}
	return handler.accountEmpty(account.ID)
}

// releaseAccount libera una cuenta vacía (POST restaurant/accounts/{id}/release, accounts.open): sin
// motivo ni autorización adicional, la deja anulada con el motivo fijo, auditada como
// restaurant.account.released y con su evento, todo en la misma transacción. Una cuenta con cualquier
// contenido responde ACCOUNT_NOT_EMPTY y sigue el flujo normal de anulación o cancelación.
func (handler *service) releaseAccount(id string) (any, error) {
	var input Mutation
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	if err := handler.authorizeAccount(account, true); err != nil {
		return nil, err
	}
	if err := ensureOpen(account, input.Revision); err != nil {
		return nil, err
	}
	empty, err := handler.accountEmpty(id)
	if err != nil {
		return nil, err
	}
	if !empty {
		return nil, &Error{
			Status:  409,
			Code:    "ACCOUNT_NOT_EMPTY",
			Message: "La cuenta tiene productos, borradores, pagos, ajustes, documentos o registros de cocina. Usa la cancelación o la anulación con su motivo.",
		}
	}
	if err := handler.touch(id, input.Revision); err != nil {
		return nil, err
	}
	if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_accounts SET state='voided' WHERE id=$1", id); err != nil {
		return nil, err
	}
	saved, err := handler.readBill(id)
	if err != nil {
		return nil, err
	}
	return saved, handler.event(id, nil, "account.released", map[string]any{"account": account}, map[string]any{
		"account": saved.Account, "reason": releaseReason,
	})
}
