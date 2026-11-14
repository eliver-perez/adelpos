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
	// Indicaciones rápidas elegidas (etapa 8): solo su forma; el borrador no ejecuta el motor y
	// Go las valida contra el catálogo al cotizar y al confirmar.
	if err := catalog.CheckQuickNoteShape(customization.QuickNotes); err != nil {
		return err
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
	if err := handler.rejectSoldOutCapture(account.PriceListID, input.Items, before); err != nil {
		return nil, err
	}
	if err := handler.touch(id, input.Revision); err != nil {
		return nil, err
	}
	if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.restaurant_draft_items WHERE account_id=$1", id); err != nil {
		return nil, err
	}
	if err := handler.insertDraftItems(id, input.Items); err != nil {
		return nil, err
	}
	if err := handler.event(id, nil, "draft.saved", before, input.Items); err != nil {
		return nil, err
	}
	return handler.readBill(id)
}

// insertDraftItems guarda las partidas del borrador en su orden. La comparten guardar el borrador y
// abrir una cuenta con su primer producto (etapa 7.1).
func (handler *service) insertDraftItems(accountID string, items []DraftItem) error {
	for index, item := range items {
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.restaurant_draft_items
            (id,account_id,product_id,variant_id,quantity,customization,position,author_id,device_id)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, item.ID, accountID, item.ProductID, item.VariantID, item.Quantity,
			jsonValue(item.Customization), index, handler.request.Actor.UserID, handler.request.Actor.DeviceID)
		if err != nil {
			return err
		}
	}
	return nil
}

// rejectSoldOutCapture rechaza al guardar el borrador las partidas nuevas que piden un producto agotado (revisión
// independiente de la fase D, hallazgo 3). Solo revisa lo que se agrega: una partida nueva, una que cambió de
// producto, presentación o personalización, o una a la que se le suben unidades. Una partida que ya estaba guardada y
// se conserva igual o con menos unidades no se rechaza (su producto pudo agotarse después): así el mesero puede
// quitarla o reducirla, y la confirmación sigue rechazando el producto agotado como siempre.
func (handler *service) rejectSoldOutCapture(priceListID string, items []DraftItem, previous []DraftItem) error {
	saved := map[string]DraftItem{}
	for _, item := range previous {
		saved[item.ID] = item
	}
	lines := []catalog.LineRequest{}
	for _, item := range items {
		before, exists := saved[item.ID]
		if exists && sameDraftLine(before, item) && item.Quantity <= before.Quantity {
			continue
		}
		lines = append(lines, item.LineRequest)
	}
	return catalog.RejectSoldOut(handler.ctx, handler.tx, &priceListID, lines)
}

// sameDraftLine compara producto, presentación y personalización de dos partidas, sin distinguir una lista vacía de
// una ausente.
func sameDraftLine(left, right DraftItem) bool {
	if left.ProductID != right.ProductID {
		return false
	}
	if (left.VariantID == nil) != (right.VariantID == nil) || (left.VariantID != nil && *left.VariantID != *right.VariantID) {
		return false
	}
	return string(jsonValue(normalizedCustomization(left.Customization))) ==
		string(jsonValue(normalizedCustomization(right.Customization)))
}

func normalizedCustomization(customization catalog.Customization) catalog.Customization {
	if customization.Ingredients == nil {
		customization.Ingredients = []catalog.IngredientSelection{}
	}
	if customization.Groups == nil {
		customization.Groups = []catalog.GroupSelection{}
	}
	groups := make([]catalog.GroupSelection, 0, len(customization.Groups))
	for _, group := range customization.Groups {
		if group.Options == nil {
			group.Options = []catalog.IncludedOption{}
		}
		groups = append(groups, group)
	}
	customization.Groups = groups
	components := make([]catalog.ComponentSelection, 0, len(customization.Components))
	for _, component := range customization.Components {
		component.Customization = normalizedCustomization(component.Customization)
		components = append(components, component)
	}
	customization.Components = components
	if len(customization.QuickNotes) == 0 {
		customization.QuickNotes = nil
	}
	return customization
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
