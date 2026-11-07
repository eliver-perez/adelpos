package catalog

import (
	"github.com/shopspring/decimal"
)

func (engine *calculator) components(product Product, customization Customization, result *ProductBreakdown, depth int) (decimal.Decimal, error) {
	selections := map[string]ComponentSelection{}
	for _, selection := range customization.Components {
		if _, exists := selections[selection.SlotID]; exists {
			return decimal.Zero, invalid("Un componente del combo está repetido.")
		}
		selections[selection.SlotID] = selection
	}
	total := decimal.Zero
	for _, slot := range product.Slots {
		selected, explicit := selections[slot.ID]
		if !explicit {
			selected = ComponentSelection{SlotID: slot.ID, ProductID: slot.DefaultProductID, Quantity: slot.DefaultQuantity}
		}
		delete(selections, slot.ID)
		if selected.Quantity < slot.MinQuantity || selected.Quantity > slot.MaxQuantity {
			return decimal.Zero, invalid("La cantidad del componente " + slot.Name + " está fuera de sus límites.")
		}
		var allowed *ComboOption
		for index := range slot.Options {
			if slot.Options[index].ProductID == selected.ProductID {
				allowed = &slot.Options[index]
			}
		}
		if allowed == nil {
			return decimal.Zero, invalid("El producto no es una sustitución permitida en el combo.")
		}
		component := ComponentBreakdown{
			SlotID:   slot.ID,
			Name:     slot.Name,
			Quantity: selected.Quantity,
			Charge:   "0.00",
		}
		if selected.Quantity > 0 {
			child, err := engine.product(selected.ProductID, selected.VariantID, selected.Customization, depth+1)
			if err != nil {
				return decimal.Zero, err
			}
			covered := min(selected.Quantity, slot.IncludedQuantity)
			charge := amount(allowed.SubstitutionPrice).Mul(decimal.NewFromInt(int64(covered)))
			additional := max(0, selected.Quantity-slot.IncludedQuantity)
			if additional > 0 {
				tierCharge, err := tieredCharge(allowed.Tiers, decimal.NewFromInt(int64(additional)))
				if err != nil {
					return decimal.Zero, err
				}
				charge = charge.Add(tierCharge)
			}
			// El combo cubre el producto; solamente se suman sus personalizaciones, una vez por unidad.
			customizationCharge := amount(child.UnitPrice).Sub(amount(child.BasePrice)).Mul(decimal.NewFromInt(int64(selected.Quantity)))
			charge = charge.Add(customizationCharge).Round(2)
			component.Product, component.Charge = child, money(charge)
			total = total.Add(charge)
		}
		result.Components = append(result.Components, component)
	}
	if len(selections) > 0 {
		return decimal.Zero, invalid("El componente solicitado no pertenece al combo.")
	}
	return total, nil
}
