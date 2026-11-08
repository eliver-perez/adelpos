package operations

import (
	"local/ad-el-pos/server/internal/catalog"
)

func validateCustomizationShape(customization catalog.Customization, depth int) error {
	if depth > 8 || len(customization.Notes) > 500 || len(customization.Ingredients) > 100 || len(customization.Groups) > 30 || len(customization.Components) > 30 {
		return invalid("La personalización supera sus límites de tamaño o profundidad.")
	}
	for _, group := range customization.Groups {
		if len(group.Options) > 100 {
			return invalid("Un grupo admite hasta 100 opciones seleccionadas.")
		}
	}
	for _, component := range customization.Components {
		if component.Quantity < 0 || component.Quantity > 100 {
			return invalid("La cantidad de un componente no es válida.")
		}
		if err := validateCustomizationShape(component.Customization, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (handler *service) validateDraftItems(accountID string, items []DraftItem) error {
	if len(items) > 20 {
		return invalid("Cada borrador admite hasta 20 partidas; confirma este lote antes de capturar otro.")
	}
	total := 0
	seen := map[string]bool{}
	for _, item := range items {
		if err := validateID(item.ID); err != nil {
			return err
		}
		if err := validateID(item.ProductID); err != nil {
			return err
		}
		if seen[item.ID] || item.Quantity < 1 {
			return invalid("Las partidas del borrador deben ser únicas y tener cantidades positivas.")
		}
		seen[item.ID] = true
		total += item.Quantity
		if total > 1000 {
			return invalid("El borrador admite hasta 1000 unidades por confirmación.")
		}
		if err := validateCustomizationShape(item.Customization, 0); err != nil {
			return err
		}
		var exists, foreignDraft bool
		err := handler.tx.QueryRow(handler.ctx, `SELECT EXISTS(SELECT FROM app.catalog_products WHERE id=$1),
            EXISTS(SELECT FROM app.restaurant_draft_items WHERE id=$2 AND account_id<>$3)`, item.ProductID, item.ID, accountID).Scan(&exists, &foreignDraft)
		if err != nil {
			return err
		}
		if !exists || foreignDraft {
			return invalid("Un producto no existe o la partida pertenece a otra cuenta.")
		}
		if item.VariantID != nil {
			if err := validateID(*item.VariantID); err != nil {
				return err
			}
			err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.catalog_variants WHERE id=$1 AND product_id=$2)", *item.VariantID, item.ProductID).Scan(&exists)
			if err != nil {
				return err
			}
			if !exists {
				return invalid("La presentación no pertenece al producto del borrador.")
			}
		}
	}
	return nil
}

func (handler *service) saveDraft(id string) (any, error) {
	var input struct {
		Mutation
		Items []DraftItem `json:"items"`
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
	if err := handler.validateDraftItems(id, input.Items); err != nil {
		return nil, err
	}
	before, err := handler.readDraft(id)
	if err != nil {
		return nil, err
	}
	if err := handler.touch(id, input.Revision); err != nil {
		return nil, err
	}
	if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.restaurant_draft_items WHERE account_id=$1", id); err != nil {
		return nil, err
	}
	for index, item := range input.Items {
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.restaurant_draft_items
            (id,account_id,product_id,variant_id,quantity,customization,position,author_id,device_id)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, item.ID, id, item.ProductID, item.VariantID, item.Quantity,
			jsonValue(item.Customization), index, handler.request.Actor.UserID, handler.request.Actor.DeviceID)
		if err != nil {
			return nil, err
		}
	}
	if err := handler.event(id, nil, "draft.saved", before, input.Items); err != nil {
		return nil, err
	}
	return handler.readBill(id)
}

func draftRequests(items []DraftItem) []catalog.LineRequest {
	requests := make([]catalog.LineRequest, 0, len(items))
	for _, item := range items {
		requests = append(requests, item.LineRequest)
	}
	return requests
}

func (handler *service) previewUnsavedDraft(id string) (any, error) {
	var input struct {
		Revision int         `json:"revision"`
		Items    []DraftItem `json:"items"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	if err := ensureOpen(account, input.Revision); err != nil {
		return nil, err
	}
	if err := handler.validateDraftItems(id, input.Items); err != nil {
		return nil, err
	}
	if len(input.Items) == 0 {
		return &QuoteResult{AccountID: id, Revision: account.Revision, Combined: account.Totals}, nil
	}
	preview, _, err := handler.calculateDraft(account, input.Items)
	return preview, err
}

func (handler *service) calculateDraft(account *Account, items []DraftItem) (*QuoteResult, catalog.Snapshot, error) {
	quote, snapshot, err := catalog.QuoteForOrder(handler.ctx, handler.tx, catalog.PreviewRequest{
		PriceListID: &account.PriceListID,
		Lines:       draftRequests(items),
	})
	if err != nil {
		return nil, snapshot, err
	}
	if err := validateStorageAmount(quote); err != nil {
		return nil, snapshot, err
	}
	fingerprint := quoteFingerprint(quote, map[string]any{"account_id": account.ID, "revision": account.Revision, "draft": items})
	result := &QuoteResult{
		AccountID: account.ID, Revision: account.Revision, Fingerprint: fingerprint, Quote: quote,
		Combined:      addTotals(account.Totals, quoteTotals(quote)),
		PricingPolicy: "Los precios y promociones de este lote se fijan al confirmar. Los productos anteriores conservan sus importes.",
	}
	return result, snapshot, nil
}

func (handler *service) previewDraft(id string) (any, error) {
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	if err := handler.authorizeAccount(account, false); err != nil {
		return nil, err
	}
	if err := ensureOpen(account, account.Revision); err != nil {
		return nil, err
	}
	items, err := handler.readDraft(id)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return &QuoteResult{AccountID: id, Revision: account.Revision, Combined: account.Totals}, nil
	}
	result, _, err := handler.calculateDraft(account, items)
	return result, err
}
