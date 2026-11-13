package operations

import (
	"strings"

	"local/ad-el-pos/server/internal/payments"
)

// «Cancelar unidades» (POST restaurant/accounts/{id}/cancel). Desde la etapa 9F (§7.2) sigue el
// criterio de «Cancelar cuenta» de la 9D: con orders.cancel basta el permiso propio; sin él, la
// autorización puntual de un supervisor que lo tenga (approveWithSupervisor, antes de la primera
// escritura). Por eso la ruta ya no exige orders.cancel: basta accounts.open o el propio
// orders.cancel (authorizeRoute), para que quien no tiene el permiso pueda pedir la autorización. El
// evento item.cancelled guarda quién autorizó (authorized_by, authorized_by_username y
// authorized_by_name, null si bastó el permiso propio) y la autorización se audita aparte, sin
// contraseña. Cada cuenta abierta trae cancel_units_requires_authorization, calculado por Go.

// unitsCancelPermission es el permiso propio para cancelar o reducir unidades ya enviadas.
const unitsCancelPermission = "orders.cancel"

func (handler *service) cancel(id string) (any, error) {
	var input struct {
		Mutation
		VersionID     string                       `json:"version_id"`
		Quantity      int                          `json:"quantity"`
		Reason        string                       `json:"reason"`
		Authorization *payments.AuthorizationInput `json:"authorization"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(input.Reason)) < 5 || len(input.Reason) > 500 {
		return nil, invalid("Indica un motivo de cancelación de entre 5 y 500 caracteres.")
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
	units, err := handler.heldUnits(id, input.VersionID)
	if err != nil {
		return nil, err
	}
	if input.Quantity < 1 || input.Quantity > len(units) {
		return nil, invalid("La cantidad por cancelar excede las unidades activas de esa partida en la cuenta.")
	}
	// Solo se cancelan unidades sin pago ni descuento manual; las pagadas requieren devolución previa.
	free, held, err := handler.splitFreeUnits(units)
	if err != nil {
		return nil, err
	}
	if input.Quantity > len(free) {
		return nil, handler.requireUnitsFree(held)
	}
	selected := free[len(free)-input.Quantity:]
	// Las validaciones de la partida van antes de la autorización: una cantidad imposible o una
	// unidad pagada se informan sin pedir la contraseña del supervisor (ni contar un intento).
	supervisor, err := handler.approveWithSupervisor(unitsCancelRule, input.Authorization)
	if err != nil {
		return nil, err
	}
	previous, err := readOne[versionRecord](handler.ctx, handler.tx,
		"SELECT to_jsonb(v) FROM app.restaurant_item_versions v WHERE id=$1", input.VersionID)
	if err != nil {
		return nil, err
	}
	authorization, err := handler.authorizationFields(supervisor)
	if err != nil {
		return nil, err
	}
	// Desde aquí se escribe: la autorización y todos los permisos ya se comprobaron.
	if err := handler.touch(id, input.Revision); err != nil {
		return nil, err
	}
	identifiers := make([]string, 0, len(selected))
	for _, unit := range selected {
		if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_item_units SET state='cancelled',cancelled_at=now() WHERE id=$1", unit.ID); err != nil {
			return nil, err
		}
		identifiers = append(identifiers, unit.ID)
	}
	if err := handler.requireAccountCovered(id); err != nil {
		return nil, err
	}
	if err := handler.auditSupervisor(id, supervisor, "item_cancel", map[string]any{
		"version_id": input.VersionID, "quantity": input.Quantity, "amount": unitTotals(selected).Total,
	}); err != nil {
		return nil, err
	}
	after := map[string]any{
		"version_id": input.VersionID, "quantity": input.Quantity, "unit_ids": identifiers, "reason": input.Reason,
	}
	for key, value := range authorization {
		after[key] = value
	}
	if err := handler.event(id, nil, "item.cancelled", map[string]any{
		"units":      selected,
		"request":    previous.Request,
		"quote_line": previous.QuoteLine,
	}, after); err != nil {
		return nil, err
	}
	return handler.readBill(id)
}
