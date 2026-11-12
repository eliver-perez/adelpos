package operations

import (
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/payments"
)

// releaseReason es el motivo fijo con que queda anulada una cuenta vacía liberada.
const releaseReason = "Cuenta vacía liberada"

// accountWithoutRecords aplica la definición de cuenta vacía de la etapa 7.1 (decisión 7): sin
// unidades en ningún estado (tampoco canceladas), sin pedidos confirmados en ella aunque sus unidades
// se hayan transferido, sin haber recibido una combinación, sin registros financieros (payments) y
// sin registros de cocina (kitchen). Cada módulo responde por sus tablas. Con allowDraft (etapa 9D,
// decisión 1 del coordinador: «Descartar») admite además un borrador sin enviar; sin él, como en
// «Liberar mesa», el borrador también cuenta como contenido.
func (handler *service) accountWithoutRecords(accountID string, allowDraft bool) (bool, error) {
	var operational bool
	err := handler.tx.QueryRow(handler.ctx, `SELECT (NOT $2::boolean AND EXISTS(SELECT FROM app.restaurant_draft_items WHERE account_id=$1))
        OR EXISTS(SELECT FROM app.restaurant_item_units WHERE account_id=$1)
        OR EXISTS(SELECT FROM app.restaurant_orders WHERE original_account_id=$1)
        OR EXISTS(SELECT FROM app.restaurant_accounts WHERE merged_into_id=$1)`, accountID, allowDraft).Scan(&operational)
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

// accountEmpty es la definición estricta de «Liberar mesa»: tampoco admite borrador.
func (handler *service) accountEmpty(accountID string) (bool, error) {
	return handler.accountWithoutRecords(accountID, false)
}

// discardable indica si «Descartar» procede (etapa 9D, §4.2): cuenta abierta sin unidades, pedidos,
// combinaciones ni registros financieros o de cocina, aunque tenga un borrador sin enviar. Las
// cuentas con productos confirmados se descartan sin consultar más tablas. «Liberar mesa» procede
// con la misma comprobación y sin borrador (Releasable = Discardable y DraftQuantity = 0: cada
// partida del borrador tiene al menos una unidad).
func (handler *service) discardable(account *Account) (bool, error) {
	if account.State != "open" || account.ConfirmedQuantity > 0 {
		return false, nil
	}
	return handler.accountWithoutRecords(account.ID, true)
}

// releaseAccount libera una cuenta vacía (POST restaurant/accounts/{id}/release, accounts.open): sin
// motivo ni autorización adicional, la deja anulada con el motivo fijo, auditada como
// restaurant.account.released y con su evento, todo en la misma transacción. Una cuenta con cualquier
// contenido responde ACCOUNT_NOT_EMPTY y sigue el flujo de descarte o de cancelación.
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
			Message: "La cuenta tiene productos, borradores, pagos, ajustes, documentos o registros de cocina. Descártala si solo tiene un borrador o cancélala con su motivo.",
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

// discardAccount descarta una cuenta vacía (POST restaurant/accounts/{id}/discard, accounts.open;
// etapa 9D, §4.2 y decisión 1 del coordinador): sin motivo ni autorización, admite un borrador sin
// enviar y lo borra en la misma transacción. No es una cancelación: la cuenta queda en el estado
// anulado actual con el evento account.discarded (auditado como restaurant.account.discarded), sin
// registro en account_cancellations, por lo que no aparece en «Cuentas canceladas» ni en otros
// reportes. Una cuenta con productos enviados, pagos o registros de cocina responde
// ACCOUNT_NOT_EMPTY y se cancela con su motivo.
func (handler *service) discardAccount(id string) (any, error) {
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
	empty, err := handler.accountWithoutRecords(id, true)
	if err != nil {
		return nil, err
	}
	if !empty {
		return nil, &Error{
			Status:  409,
			Code:    "ACCOUNT_NOT_EMPTY",
			Message: "La cuenta tiene productos enviados, pagos, ajustes, documentos o registros de cocina. Usa «Cancelar cuenta» con su motivo.",
		}
	}
	draft, err := handler.readDraft(id)
	if err != nil {
		return nil, err
	}
	if err := handler.touch(id, input.Revision); err != nil {
		return nil, err
	}
	if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.restaurant_draft_items WHERE account_id=$1", id); err != nil {
		return nil, err
	}
	if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_accounts SET state='voided' WHERE id=$1", id); err != nil {
		return nil, err
	}
	saved, err := handler.readBill(id)
	if err != nil {
		return nil, err
	}
	return saved, handler.event(id, nil, "account.discarded", map[string]any{"account": account, "draft": draft}, map[string]any{
		"account": saved.Account,
	})
}
