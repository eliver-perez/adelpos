package catalog

import (
	"github.com/shopspring/decimal"
)

func (engine *calculator) groups(product Product, customization Customization, result *ProductBreakdown) (decimal.Decimal, error) {
	selections := map[string][]IncludedOption{}
	for _, selection := range customization.Groups {
		if _, exists := selections[selection.GroupID]; exists {
			return decimal.Zero, invalid("Un grupo de personalización está repetido.")
		}
		selections[selection.GroupID] = selection.Options
	}
	total := decimal.Zero
	for _, binding := range product.Groups {
		group, exists := engine.snapshot.Groups[binding.GroupID]
		if !exists || !group.Active {
			return decimal.Zero, unavailable("Un grupo de personalización está inactivo o no existe.")
		}
		selection, explicit := selections[binding.GroupID]
		if !explicit {
			selection = binding.Included
		}
		delete(selections, binding.GroupID)
		breakdown, err := engine.group(group, binding, selection)
		if err != nil {
			return decimal.Zero, err
		}
		total = total.Add(amount(breakdown.Difference)).Add(amount(breakdown.Extras))
		result.Groups = append(result.Groups, *breakdown)
	}
	if len(selections) > 0 {
		return decimal.Zero, invalid("El grupo seleccionado no pertenece al producto.")
	}
	return total, nil
}

func (engine *calculator) group(group OptionGroup, binding ProductGroup, selections []IncludedOption) (*GroupBreakdown, error) {
	original := map[string]decimal.Decimal{}
	for _, included := range binding.Included {
		original[included.OptionID] = amount(included.Quantity)
	}
	requested := map[string]decimal.Decimal{}
	for _, selection := range selections {
		if _, exists := requested[selection.OptionID]; exists {
			return nil, invalid("Una opción está solicitada dos veces.")
		}
		quantity, err := parseQuantity(selection.Quantity)
		if err != nil {
			return nil, err
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
				return nil, err
			}
		}
		if option.MaxQuantity != nil && quantity.GreaterThan(amount(*option.MaxQuantity)) {
			return nil, invalid("La cantidad supera el máximo configurado para la opción.")
		}
		originalValue = originalValue.Add(included.Mul(amount(option.ReferenceValue)))
		requestedValue = requestedValue.Add(quantity.Mul(amount(option.ReferenceValue)))
		quantityTotal = quantityTotal.Add(quantity)
		additional := positiveDifference(quantity, included)
		extras = extras.Add(additional.Mul(amount(option.ExtraPrice)))
		if group.PricingMode == "quantity" && additional.IsPositive() {
			charge, err := tieredCharge(option.Tiers, additional)
			if err != nil {
				return nil, err
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
		return nil, invalid("Una opción solicitada no pertenece al grupo.")
	}
	minimum, maximum := group.MinQuantity, group.MaxQuantity
	if binding.MinQuantity != nil {
		minimum = *binding.MinQuantity
	}
	if binding.MaxQuantity != nil {
		maximum = binding.MaxQuantity
	}
	if quantityTotal.LessThan(amount(minimum)) || (maximum != nil && quantityTotal.GreaterThan(amount(*maximum))) {
		return nil, invalid("La selección no respeta los límites del grupo " + group.Name + ".")
	}
	difference := positiveDifference(requestedValue, originalValue)
	if group.PricingMode == "quantity" {
		difference = tierCharges
	}
	// La equivalencia se redondea por grupo. Retirar opciones nunca reduce el precio base.
	result.OriginalValue, result.RequestedValue = money(originalValue), money(requestedValue)
	result.Difference, result.Extras = money(difference), money(extras)
	return result, nil
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
			return unavailable(reason)
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
