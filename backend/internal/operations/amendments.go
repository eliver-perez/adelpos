package operations

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/catalog"
)

type versionRecord struct {
	ID        string              `json:"id"`
	ItemID    string              `json:"item_id"`
	OrderID   string              `json:"order_id"`
	Version   int                 `json:"version"`
	Request   catalog.LineRequest `json:"request"`
	QuoteLine catalog.QuoteLine   `json:"quote_line"`
}

type editCalculation struct {
	Result   *QuoteResult
	Snapshot catalog.Snapshot
	Previous *versionRecord
	Units    []Unit
	Preserve bool
}

func (handler *service) heldUnits(accountID, versionID string) ([]Unit, error) {
	if err := validateID(versionID); err != nil {
		return nil, err
	}
	units, err := readList[Unit](handler.ctx, handler.tx, `SELECT jsonb_build_object('id',id,'item_id',item_id,
        'version_id',version_id,'ordinal',ordinal,'subtotal',subtotal::text,'discount',discount::text,'total',total::text)
        FROM app.restaurant_item_units WHERE account_id=$1 AND version_id=$2 AND state='active' ORDER BY ordinal,id`, accountID, versionID)
	if err != nil {
		return nil, err
	}
	if len(units) == 0 {
		return nil, invalid("La cuenta no conserva unidades activas de esa partida; actualiza su contenido.")
	}
	return units, nil
}

func unitTotals(units []Unit) Totals {
	total := Totals{Subtotal: "0.00", Discount: "0.00", Total: "0.00"}
	for _, unit := range units {
		total = addTotals(total, Totals{Subtotal: unit.Subtotal, Discount: unit.Discount, Total: unit.Total})
	}
	return total
}

func subtractTotals(first, second Totals) Totals {
	return Totals{
		Subtotal: money(value(first.Subtotal).Sub(value(second.Subtotal))),
		Discount: money(value(first.Discount).Sub(value(second.Discount))),
		Total:    money(value(first.Total).Sub(value(second.Total))),
	}
}

func sameRecipe(first, second catalog.LineRequest) bool {
	// Comparar copias evita alterar el pedido original al normalizar el orden de selecciones.
	normalized := func(request catalog.LineRequest) catalog.LineRequest {
		var result catalog.LineRequest
		_ = json.Unmarshal(jsonValue(request), &result)
		result.Quantity = 0
		normalizeRecipe(&result.Customization)
		return result
	}
	return reflect.DeepEqual(normalized(first), normalized(second))
}

func normalizeRecipe(customization *catalog.Customization) {
	customization.Notes = ""
	if customization.Ingredients == nil {
		customization.Ingredients = []catalog.IngredientSelection{}
	}
	if customization.Groups == nil {
		customization.Groups = []catalog.GroupSelection{}
	}
	if customization.Components == nil {
		customization.Components = []catalog.ComponentSelection{}
	}
	sort.Slice(customization.Ingredients, func(first, second int) bool {
		return customization.Ingredients[first].IngredientID < customization.Ingredients[second].IngredientID
	})
	sort.Slice(customization.Groups, func(first, second int) bool {
		return customization.Groups[first].GroupID < customization.Groups[second].GroupID
	})
	for index := range customization.Groups {
		group := &customization.Groups[index]
		if group.Options == nil {
			group.Options = []catalog.IncludedOption{}
		}
		sort.Slice(group.Options, func(first, second int) bool {
			return group.Options[first].OptionID < group.Options[second].OptionID
		})
	}
	sort.Slice(customization.Components, func(first, second int) bool {
		return customization.Components[first].SlotID < customization.Components[second].SlotID
	})
	for index := range customization.Components {
		normalizeRecipe(&customization.Components[index].Customization)
	}
}

func copyPreparationNotes(breakdown *catalog.ProductBreakdown, customization catalog.Customization) {
	breakdown.Notes = customization.Notes
	for index := range breakdown.Components {
		component := &breakdown.Components[index]
		if component.Product == nil {
			continue
		}
		child := catalog.Customization{}
		for _, selection := range customization.Components {
			if selection.SlotID == component.SlotID {
				child = selection.Customization
			}
		}
		copyPreparationNotes(component.Product, child)
	}
}

func (handler *service) calculateEdit(account *Account, input EditInput) (*editCalculation, error) {
	if err := ensureOpen(account, input.Revision); err != nil {
		return nil, err
	}
	if err := handler.authorizeAccount(account, true); err != nil {
		return nil, err
	}
	if input.Line.Quantity < 1 || input.Line.Quantity > 1000 {
		return nil, invalid("La cantidad editada debe estar entre 1 y 1000; utiliza Cancelar para retirar todas las unidades.")
	}
	if err := validateCustomizationShape(input.Line.Customization, 0); err != nil {
		return nil, err
	}
	units, err := handler.heldUnits(account.ID, input.VersionID)
	if err != nil {
		return nil, err
	}
	if input.Line.Quantity < len(units) {
		if err := handler.request.Actor.Require("orders.cancel"); err != nil {
			return nil, err
		}
	}
	previous, err := readOne[versionRecord](handler.ctx, handler.tx, "SELECT to_jsonb(v) FROM app.restaurant_item_versions v WHERE id=$1", input.VersionID)
	if err != nil {
		return nil, err
	}
	if previous.Request.ProductID != input.Line.ProductID {
		return nil, invalid("Una modificación conserva la identidad del producto; captura otro producto como partida nueva.")
	}
	calculation := &editCalculation{Previous: previous, Units: units}
	calculation.Preserve = sameRecipe(previous.Request, input.Line) && input.Line.Quantity <= len(units)
	var quote *catalog.Quote
	policy := "El aumento de cantidad o cambio de personalización recalcula esta partida con reglas vigentes. Las demás partidas conservan sus precios."
	if calculation.Preserve {
		quote, err = readOne[catalog.Quote](handler.ctx, handler.tx, "SELECT quote FROM app.restaurant_orders WHERE id=$1", previous.OrderID)
		if err != nil {
			return nil, err
		}
		totals := unitTotals(units[:input.Line.Quantity])
		line := previous.QuoteLine
		copyPreparationNotes(&line.ProductBreakdown, input.Line.Customization)
		line.Quantity = input.Line.Quantity
		line.Subtotal, line.Discount, line.Total = totals.Subtotal, totals.Discount, totals.Total
		quote.Lines = []catalog.QuoteLine{line}
		quote.Subtotal, quote.Discount, quote.Total = totals.Subtotal, totals.Discount, totals.Total
		quote.Promotions = []catalog.PromotionResult{}
		quote.CalculatedAt = time.Now().UTC()
		policy = "Se conservan los precios y centavos históricos: cambiar observaciones o reducir cantidades no reevalúa el catálogo ni las promociones."
	} else {
		quote, calculation.Snapshot, err = catalog.QuoteForOrder(handler.ctx, handler.tx, catalog.PreviewRequest{
			PriceListID: &account.PriceListID, Lines: []catalog.LineRequest{input.Line},
		})
		if err != nil {
			return nil, err
		}
		if err := validateStorageAmount(quote); err != nil {
			return nil, err
		}
	}
	remaining := subtractTotals(account.Totals, unitTotals(units))
	fingerprint := quoteFingerprint(quote, map[string]any{
		"account_id": account.ID, "revision": account.Revision, "version_id": input.VersionID, "request": input.Line, "preserve": calculation.Preserve,
	})
	calculation.Result = &QuoteResult{
		AccountID: account.ID, Revision: account.Revision, Quote: quote, Fingerprint: fingerprint,
		Combined: addTotals(remaining, quoteTotals(quote)), PricingPolicy: policy,
	}
	return calculation, nil
}

func (handler *service) editPreview(id string) (any, error) {
	var input EditInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	calculation, err := handler.calculateEdit(account, input)
	if err != nil {
		return nil, err
	}
	return calculation.Result, nil
}

func (handler *service) edit(id string) (any, error) {
	var input EditInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(input.Reason)) < 5 || len(input.Reason) > 500 {
		return nil, invalid("Indica un motivo de modificación de entre 5 y 500 caracteres.")
	}
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	calculation, err := handler.calculateEdit(account, input)
	if err != nil {
		return nil, err
	}
	if calculation.Result.Fingerprint != input.ExpectedFingerprint {
		return nil, quoteChanged()
	}
	if err := handler.touch(id, input.Revision); err != nil {
		return nil, err
	}
	change, err := handler.persistEdit(account, input, calculation)
	if err != nil {
		return nil, err
	}
	if err := handler.event(id, nil, "item.modified", map[string]any{
		"version_id": input.VersionID,
		"request":    calculation.Previous.Request,
		"quote_line": calculation.Previous.QuoteLine,
		"units":      calculation.Units,
	}, change); err != nil {
		return nil, err
	}
	return handler.readBill(id)
}
