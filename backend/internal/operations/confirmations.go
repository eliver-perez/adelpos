package operations

import (
	"local/ad-el-pos/server/internal/catalog"
)

func (handler *service) confirm(id string) (any, error) {
	var input struct {
		Mutation
		ExpectedFingerprint string `json:"expected_fingerprint"`
	}
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
	items, err := handler.readDraft(id)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, invalid("No hay productos pendientes de confirmación.")
	}
	preview, snapshot, err := handler.calculateDraft(account, items)
	if err != nil {
		return nil, err
	}
	if input.ExpectedFingerprint == "" || preview.Fingerprint != input.ExpectedFingerprint {
		return nil, quoteChanged()
	}
	if err := handler.touch(id, input.Revision); err != nil {
		return nil, err
	}
	orderID, err := handler.insertOrder(id, "confirmation", preview.Quote, snapshot)
	if err != nil {
		return nil, err
	}
	confirmed := []map[string]any{}
	for index, item := range items {
		itemID, versionID := newID(), newID()
		_, err := handler.tx.Exec(handler.ctx, "INSERT INTO app.restaurant_items(id,original_order_id,product_id) VALUES ($1,$2,$3)", itemID, orderID, item.ProductID)
		if err != nil {
			return nil, err
		}
		line := preview.Quote.Lines[index]
		if err := handler.insertVersion(versionID, itemID, orderID, 1, item.LineRequest, line); err != nil {
			return nil, err
		}
		units, err := handler.insertUnits(id, itemID, versionID, line, 1)
		if err != nil {
			return nil, err
		}
		confirmed = append(confirmed, map[string]any{"item_id": itemID, "version_id": versionID, "unit_ids": units, "request": item.LineRequest, "quote_line": line})
	}
	if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.restaurant_draft_items WHERE account_id=$1", id); err != nil {
		return nil, err
	}
	if err := handler.event(id, nil, "order.confirmed", nil, map[string]any{
		"order_id": orderID, "items": confirmed, "quote": preview.Quote, "preparation_state": "confirmed_pending_dispatch",
	}); err != nil {
		return nil, err
	}
	return handler.readBill(id)
}

func quoteChanged() error {
	return &Error{Status: 409, Code: "QUOTE_CHANGED", Message: "El cálculo cambió o falta revisarlo. Actualiza el desglose y vuelve a confirmar; el borrador se conserva."}
}

func (handler *service) insertOrder(accountID, kind string, quote *catalog.Quote, snapshot catalog.Snapshot) (string, error) {
	id := newID()
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.restaurant_orders
        (id,original_account_id,operation_id,kind,author_id,device_id,price_list_id,pricing_snapshot,quote)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, accountID, handler.operationID, kind,
		handler.request.Actor.UserID, handler.request.Actor.DeviceID, quote.PriceListID, jsonValue(snapshot), jsonValue(quote))
	return id, err
}

func (handler *service) insertVersion(id, itemID, orderID string, version int, request catalog.LineRequest, quote catalog.QuoteLine) error {
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.restaurant_item_versions
        (id,item_id,order_id,version,request,quote_line,author_id,device_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, id, itemID, orderID, version, jsonValue(request), jsonValue(quote),
		handler.request.Actor.UserID, handler.request.Actor.DeviceID)
	return err
}

func (handler *service) insertUnits(accountID, itemID, versionID string, line catalog.QuoteLine, firstOrdinal int) ([]string, error) {
	identifiers := make([]string, 0, line.Quantity)
	for index := 0; index < line.Quantity; index++ {
		id := newID()
		subtotal := unitMoney(line.Subtotal, line.Quantity, index)
		discount := unitMoney(line.Discount, line.Quantity, index)
		total := money(value(subtotal).Sub(value(discount)))
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.restaurant_item_units
            (id,item_id,version_id,ordinal,account_id,subtotal,discount,total)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, id, itemID, versionID, firstOrdinal+index, accountID, subtotal, discount, total)
		if err != nil {
			return nil, err
		}
		identifiers = append(identifiers, id)
	}
	return identifiers, nil
}
