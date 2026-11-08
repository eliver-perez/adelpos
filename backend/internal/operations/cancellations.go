package operations

import "strings"

func (handler *service) cancel(id string) (any, error) {
	var input struct {
		Mutation
		VersionID string `json:"version_id"`
		Quantity  int    `json:"quantity"`
		Reason    string `json:"reason"`
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
	previous, err := readOne[versionRecord](handler.ctx, handler.tx,
		"SELECT to_jsonb(v) FROM app.restaurant_item_versions v WHERE id=$1", input.VersionID)
	if err != nil {
		return nil, err
	}
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
	if err := handler.event(id, nil, "item.cancelled", map[string]any{
		"units":      selected,
		"request":    previous.Request,
		"quote_line": previous.QuoteLine,
	}, map[string]any{
		"version_id": input.VersionID, "quantity": input.Quantity, "unit_ids": identifiers, "reason": input.Reason,
	}); err != nil {
		return nil, err
	}
	return handler.readBill(id)
}
