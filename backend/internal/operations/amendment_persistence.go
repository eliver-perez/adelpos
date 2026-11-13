package operations

// persistEdit escribe la versión nueva y mueve, agrega o cancela unidades. Devuelve el contenido
// posterior del evento item.modified (edit agrega quién autorizó cuando la modificación reduce).
func (handler *service) persistEdit(account *Account, input EditInput, calculation *editCalculation) (map[string]any, error) {
	previous := calculation.Previous
	orderID := previous.OrderID
	if !calculation.Preserve {
		created, err := handler.insertOrder(account.ID, "amendment", calculation.Result.Quote, calculation.Snapshot)
		if err != nil {
			return nil, err
		}
		orderID = created
	}
	versionID := newID()
	var version, maximumOrdinal int
	err := handler.tx.QueryRow(handler.ctx, `SELECT (SELECT coalesce(max(version),0)+1 FROM app.restaurant_item_versions WHERE item_id=$1),
        (SELECT coalesce(max(ordinal),0) FROM app.restaurant_item_units WHERE item_id=$1)`, previous.ItemID).Scan(&version, &maximumOrdinal)
	if err != nil {
		return nil, err
	}
	line := calculation.Result.Quote.Lines[0]
	// Cambiar la tasa (del producto o la general) solo afecta lo que se confirma después: una
	// edición con nueva cotización toma la tasa efectiva vigente del producto raíz, y una edición
	// que conserva importes conserva también la tasa histórica de la versión anterior.
	var taxRate int
	if calculation.Preserve {
		if err := handler.tx.QueryRow(handler.ctx, "SELECT tax_rate FROM app.restaurant_item_versions WHERE id=$1", previous.ID).Scan(&taxRate); err != nil {
			return nil, err
		}
	} else {
		taxRate, err = handler.effectiveTaxRate(input.Line.ProductID)
		if err != nil {
			return nil, err
		}
	}
	if err := handler.insertVersion(versionID, previous.ItemID, orderID, version, input.Line, line, taxRate); err != nil {
		return nil, err
	}
	modified, added, cancelled := []string{}, []string{}, []string{}
	for index, unit := range calculation.Units {
		if index >= input.Line.Quantity {
			_, err := handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_item_units SET state='cancelled',cancelled_at=now() WHERE id=$1", unit.ID)
			if err != nil {
				return nil, err
			}
			cancelled = append(cancelled, unit.ID)
			continue
		}
		subtotal, discount, total := unit.Subtotal, unit.Discount, unit.Total
		if !calculation.Preserve {
			subtotal = unitMoney(line.Subtotal, line.Quantity, index)
			discount = unitMoney(line.Discount, line.Quantity, index)
			total = money(value(subtotal).Sub(value(discount)))
		}
		_, err := handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_item_units SET version_id=$1,subtotal=$2,
            discount=$3,total=$4 WHERE id=$5`, versionID, subtotal, discount, total, unit.ID)
		if err != nil {
			return nil, err
		}
		modified = append(modified, unit.ID)
	}
	for index := len(calculation.Units); index < input.Line.Quantity; index++ {
		unitID := newID()
		subtotal := unitMoney(line.Subtotal, line.Quantity, index)
		discount := unitMoney(line.Discount, line.Quantity, index)
		total := money(value(subtotal).Sub(value(discount)))
		maximumOrdinal++
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.restaurant_item_units
            (id,item_id,version_id,ordinal,account_id,subtotal,discount,total)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, unitID, previous.ItemID, versionID, maximumOrdinal, account.ID, subtotal, discount, total)
		if err != nil {
			return nil, err
		}
		added = append(added, unitID)
	}
	return map[string]any{
		"item_id": previous.ItemID, "version_id": versionID, "order_id": orderID, "request": input.Line,
		"quote_line": line, "modified_unit_ids": modified, "added_unit_ids": added, "cancelled_unit_ids": cancelled,
		"reason": input.Reason, "preserved_prices": calculation.Preserve,
	}, nil
}
