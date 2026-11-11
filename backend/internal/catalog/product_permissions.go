package catalog

import "reflect"

func validateProductPermissions(product, previous *Product, actor Actor) error {
	if product.Kind == "combo" || (previous != nil && previous.Kind == "combo") {
		if err := actor.Require("catalog.combos.manage"); err != nil {
			return err
		}
	}
	pricesChanged := previous == nil
	stateChanged := !product.Active || product.Archived
	availabilityChanged := hasSchedule(product.Availability) || product.TemporaryUnavailable
	imageChanged := product.ImageID != nil
	if previous != nil {
		// La tasa de IVA propia y la opción «El precio puede bajar al cambiar complementos» son
		// atributos comerciales: cambiarlas exige el mismo permiso que el precio.
		pricesChanged = previous.BasePrice != product.BasePrice ||
			!reflect.DeepEqual(previous.TaxRate, product.TaxRate) ||
			previous.PriceCanDecrease != product.PriceCanDecrease ||
			!reflect.DeepEqual(previous.Variants, product.Variants) ||
			!reflect.DeepEqual(previous.Ingredients, product.Ingredients) ||
			!reflect.DeepEqual(previous.Groups, product.Groups) ||
			!reflect.DeepEqual(previous.Slots, product.Slots)
		stateChanged = previous.Active != product.Active || previous.Archived != product.Archived
		availabilityChanged = !reflect.DeepEqual(previous.Availability, product.Availability) ||
			previous.TemporaryUnavailable != product.TemporaryUnavailable
		imageChanged = !reflect.DeepEqual(previous.ImageID, product.ImageID)
	}
	previousVariants := map[string]Variant{}
	if previous != nil {
		for _, variant := range previous.Variants {
			previousVariants[variant.ID] = variant
		}
	}
	for _, variant := range product.Variants {
		old, exists := previousVariants[variant.ID]
		if exists {
			availabilityChanged = availabilityChanged || !reflect.DeepEqual(old.Availability, variant.Availability)
			stateChanged = stateChanged || old.Active != variant.Active
		} else {
			availabilityChanged = availabilityChanged || hasSchedule(variant.Availability)
			stateChanged = stateChanged || !variant.Active
		}
		delete(previousVariants, variant.ID)
	}
	for _, omitted := range previousVariants {
		stateChanged = stateChanged || omitted.Active
	}
	requirements := []struct {
		changed    bool
		permission string
	}{
		{pricesChanged, "catalog.prices.manage"},
		{stateChanged, "catalog.products.state"},
		{availabilityChanged, "catalog.availability.manage"},
		{imageChanged, "catalog.images.manage"},
	}
	for _, requirement := range requirements {
		if requirement.changed {
			if err := actor.Require(requirement.permission); err != nil {
				return err
			}
		}
	}
	return nil
}
