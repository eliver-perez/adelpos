package catalog

import (
	"github.com/shopspring/decimal"
)

// groupTotals resume los grupos de un producto para su precio unitario. Cada importe es la suma de
// los valores ya redondeados por grupo.
type groupTotals struct {
	// differences suma las diferencias de cada grupo tal como quedan en su desglose: nunca
	// negativas con la regla con piso; con signo cuando se aplica la opción de bajar el precio.
	differences decimal.Decimal
	// floorDifferences suma las diferencias con la regla con piso (la de siempre), también cuando
	// se aplica la opción: es el precio de referencia que la opción solo puede bajar.
	floorDifferences decimal.Decimal
	// extras suma los extras explícitos, que nunca se descuentan.
	extras decimal.Decimal
	// reductions guarda, por grupo y en el orden del desglose, lo que la opción de bajar el precio
	// permite bajar respecto de la regla con piso (cero o negativo); solo existe con la opción.
	reductions []decimal.Decimal
}

func (engine *calculator) groups(product Product, customization Customization, result *ProductBreakdown, decrease bool) (groupTotals, error) {
	totals := groupTotals{differences: decimal.Zero, floorDifferences: decimal.Zero, extras: decimal.Zero}
	selections := map[string][]IncludedOption{}
	for _, selection := range customization.Groups {
		if _, exists := selections[selection.GroupID]; exists {
			return totals, invalid("Un grupo de personalización está repetido.")
		}
		selections[selection.GroupID] = selection.Options
	}
	for _, binding := range product.Groups {
		group, exists := engine.snapshot.Groups[binding.GroupID]
		if !exists || !group.Active {
			return totals, unavailable("Un grupo de personalización está inactivo o no existe.")
		}
		selection, explicit := selections[binding.GroupID]
		if !explicit {
			selection = binding.Included
		}
		delete(selections, binding.GroupID)
		breakdown, floorDifference, err := engine.group(group, binding, selection, decrease)
		if err != nil {
			return totals, err
		}
		totals.differences = totals.differences.Add(amount(breakdown.Difference))
		totals.floorDifferences = totals.floorDifferences.Add(floorDifference)
		totals.extras = totals.extras.Add(amount(breakdown.Extras))
		if decrease {
			// Con los dos importes ya redondeados: diferencia con signo = piso + reducción, exacto.
			reduction := decimal.Min(amount(breakdown.Difference).Sub(floorDifference), decimal.Zero)
			totals.reductions = append(totals.reductions, reduction)
		}
		result.Groups = append(result.Groups, *breakdown)
	}
	if len(selections) > 0 {
		return totals, invalid("El grupo seleccionado no pertenece al producto.")
	}
	return totals, nil
}

// group calcula un grupo y devuelve su desglose y su diferencia con la regla con piso. Con
// decrease (opción «El precio puede bajar al cambiar complementos», §3.1) la diferencia del
// desglose lleva signo: en modo valor es valor solicitado − valor incluido (quitar sin reemplazo
// también resta); en modo cantidad son los tramos de las porciones adicionales más la diferencia
// de valor solo si es negativa, min(0, valor solicitado − valor incluido): §0-B solo permite bajar
// por diferencias negativas de valor, así que sustituir por un complemento más caro nunca baja el
// precio (sin valores de referencia no cambia). Sin la opción, la diferencia del desglose es la de
// siempre y coincide con la del piso.
func (engine *calculator) group(group OptionGroup, binding ProductGroup, selections []IncludedOption, decrease bool) (*GroupBreakdown, decimal.Decimal, error) {
	original := map[string]decimal.Decimal{}
	for _, included := range binding.Included {
		original[included.OptionID] = amount(included.Quantity)
	}
	requested := map[string]decimal.Decimal{}
	for _, selection := range selections {
		if _, exists := requested[selection.OptionID]; exists {
			return nil, decimal.Zero, invalid("Una opción está solicitada dos veces.")
		}
		quantity, err := parseQuantity(selection.Quantity)
		if err != nil {
			return nil, decimal.Zero, err
		}
		requested[selection.OptionID] = quantity
	}
	result := &GroupBreakdown{
		GroupID:  group.ID,
		Revision: group.Revision,
		Name:     group.Name,
		Mode:     group.PricingMode,
		Options:  []OptionBreakdown{},
	}
	originalValue, requestedValue, extras, tierCharges, quantityTotal := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	for _, option := range group.Options {
		included := original[option.ID]
		quantity := requested[option.ID]
		delete(requested, option.ID)
		if quantity.IsPositive() {
			if err := engine.selectedOptionAvailable(option); err != nil {
				return nil, decimal.Zero, err
			}
		}
		if option.MaxQuantity != nil && quantity.GreaterThan(amount(*option.MaxQuantity)) {
			return nil, decimal.Zero, invalid("La cantidad supera el máximo configurado para la opción.")
		}
		originalValue = originalValue.Add(included.Mul(amount(option.ReferenceValue)))
		requestedValue = requestedValue.Add(quantity.Mul(amount(option.ReferenceValue)))
		quantityTotal = quantityTotal.Add(quantity)
		additional := positiveDifference(quantity, included)
		extras = extras.Add(additional.Mul(amount(option.ExtraPrice)))
		if group.PricingMode == "quantity" && additional.IsPositive() {
			charge, err := tieredCharge(option.Tiers, additional)
			if err != nil {
				return nil, decimal.Zero, err
			}
			tierCharges = tierCharges.Add(charge)
		}
		result.Options = append(result.Options, OptionBreakdown{
			OptionID:     option.ID,
			ComplementID: option.ComplementID,
			PortionID:    option.PortionID,
			Name:         option.Name,
			Included:     included.String(),
			Requested:    quantity.String(),
		})
	}
	if len(requested) > 0 {
		return nil, decimal.Zero, invalid("Una opción solicitada no pertenece al grupo.")
	}
	minimum, maximum := group.MinQuantity, group.MaxQuantity
	if binding.MinQuantity != nil {
		minimum = *binding.MinQuantity
	}
	if binding.MaxQuantity != nil {
		maximum = binding.MaxQuantity
	}
	if quantityTotal.LessThan(amount(minimum)) || (maximum != nil && quantityTotal.GreaterThan(amount(*maximum))) {
		return nil, decimal.Zero, invalid("La selección no respeta los límites del grupo " + group.Name + ".")
	}
	difference := positiveDifference(requestedValue, originalValue)
	if group.PricingMode == "quantity" {
		difference = tierCharges
	}
	// La equivalencia se redondea por grupo. Con la regla con piso, retirar opciones nunca reduce el
	// precio base.
	floorDifference := amount(money(difference))
	if decrease {
		difference = requestedValue.Sub(originalValue)
		if group.PricingMode == "quantity" {
			difference = tierCharges.Add(decimal.Min(difference, decimal.Zero))
		}
	}
	result.OriginalValue, result.RequestedValue = money(originalValue), money(requestedValue)
	result.Difference, result.Extras = money(difference), money(extras)
	return result, floorDifference, nil
}

func (engine *calculator) selectedOptionAvailable(option GroupOption) error {
	snapshot := engine.snapshot
	complement, exists := snapshot.Complements[option.ComplementID]
	if !option.Active || !exists || !complement.Active {
		return unavailable("Un complemento solicitado está inactivo.")
	}
	if option.PortionID != nil {
		portion, exists := snapshot.Portions[*option.PortionID]
		if !exists || !portion.Active {
			return unavailable("La porción solicitada está inactiva.")
		}
	}
	if complement.IngredientID != nil {
		ingredient, exists := snapshot.Ingredients[*complement.IngredientID]
		if !exists || !ingredient.Active {
			return unavailable("El ingrediente del complemento está inactivo.")
		}
	}
	if complement.ProductID != nil {
		product, exists := snapshot.Products[*complement.ProductID]
		if !exists {
			return unavailable("No se pudo verificar el producto del complemento.")
		}
		if reason := productAvailabilityReason(

			product,
			snapshot.Categories[product.CategoryID],
			engine.now,
			engine.location,
			engine.override,
		); reason != "" {
			// Un complemento que es un producto agotado (etapa 9G) no se puede elegir.
			if isSoldOutReason(reason) {
				return soldOut(product.ID, "El complemento «"+product.Name+"» está agotado. Elige otro complemento.")
			}
			return unavailableProduct(product.ID, reason)
		}
	}
	return nil
}

func tieredCharge(tiers []QuantityTier, quantity decimal.Decimal) (decimal.Decimal, error) {
	if !quantity.Equal(quantity.Truncate(0)) || quantity.GreaterThan(decimal.NewFromInt(1000)) || len(tiers) == 0 || tiers[0].From != 1 {
		return decimal.Zero, invalid(
			"Las unidades adicionales requieren cantidades enteras y tramos de precio explícitos desde la unidad 1.",
		)
	}
	total := decimal.Zero
	tierIndex := 0
	for unit := 1; unit <= int(quantity.IntPart()); unit++ {
		for tierIndex+1 < len(tiers) && tiers[tierIndex+1].From <= unit {
			tierIndex++
		}
		total = total.Add(amount(tiers[tierIndex].UnitPrice))
	}
	return total, nil
}
