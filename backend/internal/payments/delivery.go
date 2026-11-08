package payments

import (
	"local/ad-el-pos/server/internal/events"
)

// insertDelivery registra la entrega de un pedido para llevar. La clave primaria por cuenta
// impide dobles entregas aunque se repita la solicitud con otro identificador.
func (handler *service) insertDelivery(accountID string) error {
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.takeaway_deliveries(account_id,actor_id,device_id,operation_id)
        VALUES ($1,$2,$3,$4)`, accountID, handler.request.Actor.UserID, handler.request.Actor.DeviceID, handler.operationID)
	if err != nil {
		return err
	}
	if err := events.Append(handler.ctx, handler.tx, "takeaway.delivered", "takeaway", &accountID, nil); err != nil {
		return err
	}
	return handler.audit(handler.ctx, handler.tx, "takeaway.delivered", "account", &accountID, nil, map[string]string{"delivery_state": "delivered"})
}

// deliver confirma la entrega de un pedido para llevar ya liquidado. Preparación, pago y entrega
// siguen siendo estados independientes: estar pagado no implica estar entregado.
func (handler *service) deliver(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	accountID := parts[1]
	if _, err := readAccountRecord(handler.ctx, handler.tx, accountID, true); err != nil {
		return nil, err
	}
	finance, err := loadFinance(handler.ctx, handler.tx, accountID)
	if err != nil {
		return nil, err
	}
	if finance.Mode != "takeaway" {
		return nil, invalid("Solo los pedidos para llevar se marcan como entregados.")
	}
	if finance.Delivery != nil {
		return nil, conflict("ALREADY_DELIVERED", "El pedido ya fue entregado.")
	}
	if !finance.Settled {
		return nil, conflict("BALANCE_PENDING", "El pedido tiene saldo pendiente. Cóbralo antes de marcarlo como entregado.")
	}
	if err := handler.insertDelivery(accountID); err != nil {
		return nil, err
	}
	closed, err := handler.closeIfSettled(accountID)
	if err != nil {
		return nil, err
	}
	saved, err := loadFinance(handler.ctx, handler.tx, accountID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"finance": saved, "closed": closed, "delivered": true}, nil
}
