package catalog

import (
	"github.com/shopspring/decimal"
)

// componentTotals resume los componentes de un combo para su precio unitario.
type componentTotals struct {
	// charges suma los cargos de los componentes (sustitución, tramos y personalización con la
	// regla con piso); nunca se descuentan.
	charges decimal.Decimal
	// adjustments suma las reducciones por complementos de los componentes (cero o negativas), que
	// solo existen cuando el combo también tiene la opción de bajar el precio (§3.1).
	adjustments decimal.Decimal
}

// components calcula los componentes de un combo. decrease indica si el combo aplica la opción «El
// precio puede bajar al cambiar complementos»; sin ella, ningún componente baja el precio aunque
// tenga la opción. Las sustituciones de ranura no son complementos y no cambian.
func (engine *calculator) components(product Product, customization Customization, result *ProductBreakdown, depth int, decrease bool) (componentTotals, error) {
	totals := componentTotals{charges: decimal.Zero, adjustments: decimal.Zero}
	selections := map[string]ComponentSelection{}
	for _, selection := range customization.Components {
		if _, exists := selections[selection.SlotID]; exists {
			return totals, invalid("Un componente del combo está repetido.")
		}
		selections[selection.SlotID] = selection
	}
	for _, slot := range product.Slots {
		selected, explicit := selections[slot.ID]
		if !explicit {
			selected = ComponentSelection{SlotID: slot.ID, ProductID: slot.DefaultProductID, Quantity: slot.DefaultQuantity}
		}
		delete(selections, slot.ID)
		if selected.Quantity < slot.MinQuantity || selected.Quantity > slot.MaxQuantity {
			return totals, invalid("La cantidad del componente " + slot.Name + " está fuera de sus límites.")
		}
		var allowed *ComboOption
		for index := range slot.Options {
			if slot.Options[index].ProductID == selected.ProductID {
				allowed = &slot.Options[index]
			}
		}
		if allowed == nil {
			return totals, invalid("El producto no es una sustitución permitida en el combo.")
		}
		component := ComponentBreakdown{
			SlotID:   slot.ID,
			Name:     slot.Name,
			Quantity: selected.Quantity,
			Charge:   "0.00",
		}
		if selected.Quantity > 0 {
			child, err := engine.product(selected.ProductID, selected.VariantID, selected.Customization, depth+1, decrease)
			if err != nil {
				return totals, err
			}
			covered := min(selected.Quantity, slot.IncludedQuantity)
			charge := amount(allowed.SubstitutionPrice).Mul(decimal.NewFromInt(int64(covered)))
			additional := max(0, selected.Quantity-slot.IncludedQuantity)
			if additional > 0 {
				tierCharge, err := tieredCharge(allowed.Tiers, decimal.NewFromInt(int64(additional)))
				if err != nil {
					return totals, err
				}
				charge = charge.Add(tierCharge)
			}
			// El combo cubre el producto; solamente se suman sus personalizaciones, una vez por unidad.
			units := decimal.NewFromInt(int64(selected.Quantity))
			customizationCharge := amount(child.UnitPrice).Sub(amount(child.BasePrice)).Mul(units)
			if child.ComplementAdjustment != "" {
				// La reducción por complementos del componente no rebaja su cargo: se suma aparte y se
				// aplica en el núcleo del combo, con sus límites. El cargo conserva la regla con piso.
				childAdjustment := amount(child.ComplementAdjustment).Mul(units)
				customizationCharge = customizationCharge.Sub(childAdjustment)
				totals.adjustments = totals.adjustments.Add(childAdjustment)
			}
			charge = charge.Add(customizationCharge).Round(2)
			component.Product, component.Charge = child, money(charge)
			totals.charges = totals.charges.Add(charge)
		}
		result.Components = append(result.Components, component)
	}
	if len(selections) > 0 {
		return totals, invalid("El componente solicitado no pertenece al combo.")
	}
	return totals, nil
}
