package catalog

import (
	"time"

	"github.com/shopspring/decimal"
)

type calculator struct {
	snapshot  Snapshot
	now       time.Time
	location  *time.Location
	override  bool
	remaining int
}

func Calculate(snapshot Snapshot, request PreviewRequest, now time.Time, location *time.Location) (*Quote, error) {
	if !snapshot.PriceList.Active {
		return nil, unavailable("La lista de precios está inactiva.")
	}
	if len(request.Lines) == 0 || len(request.Lines) > 20 {
		return nil, invalid("La previsualización admite entre 1 y 20 productos.")
	}
	quote := &Quote{
		CalculatedAt:         now.UTC(),
		Timezone:             location.String(),
		PriceListID:          snapshot.PriceList.ID,
		PriceListRevision:    snapshot.PriceList.Revision,
		PriceListName:        snapshot.PriceList.Name,
		AvailabilityOverride: request.OverrideAvailability,
		Lines:                []QuoteLine{},
		Promotions:           []PromotionResult{},
	}
	subtotal := decimal.Zero
	units := 0
	engine := calculator{
		snapshot:  snapshot,
		now:       now,
		location:  location,
		override:  request.OverrideAvailability,
		remaining: 1000,
	}
	for _, line := range request.Lines {
		units += line.Quantity
		if line.Quantity < 1 || units > 1000 {
			return nil, invalid("Las cantidades deben ser enteras positivas; el límite de previsualización es 1000 unidades.")
		}
		// En la raíz nada restringe la opción de bajar el precio: decide la del propio producto.
		breakdown, err := engine.product(line.ProductID, line.VariantID, line.Customization, 0, true)
		if err != nil {
			return nil, err
		}
		lineSubtotal := amount(breakdown.UnitPrice).Mul(decimal.NewFromInt(int64(line.Quantity)))
		quote.Lines = append(quote.Lines, QuoteLine{
			ProductBreakdown: *breakdown,
			Quantity:         line.Quantity,
			Subtotal:         money(lineSubtotal),
			Discount:         "0.00",
			Total:            money(lineSubtotal),
		})
		subtotal = subtotal.Add(lineSubtotal)
	}
	quote.Subtotal = money(subtotal)
	applyPromotions(snapshot, quote, now, location)
	discount := decimal.Zero
	for _, line := range quote.Lines {
		discount = discount.Add(amount(line.Discount))
	}
	quote.Discount = money(discount)
	quote.Total = money(subtotal.Sub(discount))
	return quote, nil
}

func effectivePrice(
	product Product,
	variantID *string,
	list PriceList,
	now time.Time,
	location *time.Location,
	override bool,
) (decimal.Decimal, string, error) {
	price := product.BasePrice
	origin := "Precio base del producto"
	if variantID != nil {
		found := false
		for _, variant := range product.Variants {
			if variant.ID == *variantID {
				found = true
				if !variant.Active || (!override && !scheduleAvailable(variant.Availability, now, location)) {
					return decimal.Zero, "", unavailable("La presentación está inactiva o fuera de horario.")
				}
				price, origin = variant.Price, "Precio base de la presentación"
			}
		}
		if !found {
			return decimal.Zero, "", invalid("La presentación seleccionada no pertenece al producto.")
		}
	}
	for _, entry := range list.Entries {
		sameVariant := entry.VariantID == nil && variantID == nil
		if entry.VariantID != nil && variantID != nil {
			sameVariant = *entry.VariantID == *variantID
		}
		if entry.ProductID == product.ID && sameVariant {
			return amount(entry.Price), "Lista: " + list.Name, nil
		}
	}
	if list.Fallback == "reject" {
		return decimal.Zero, "", invalid("El producto o presentación no tiene precio específico en la lista seleccionada.")
	}
	return amount(price), origin + " (respaldo de " + list.Name + ")", nil
}

// product calcula el desglose de un producto. allowDecrease indica si el contexto admite la opción
// «El precio puede bajar al cambiar complementos» (§3.1): siempre en la raíz; en el componente de
// un combo, solo si el combo también la aplica.
func (engine *calculator) product(id string, variantID *string, customization Customization, depth int, allowDecrease bool) (*ProductBreakdown, error) {
	engine.remaining--
	if engine.remaining < 0 {
		return nil, invalid("La configuración expande demasiados componentes para una previsualización.")
	}
	snapshot, now, location, override := engine.snapshot, engine.now, engine.location, engine.override
	if depth > 8 || len(customization.Notes) > 500 {
		return nil, invalid("La personalización supera ocho niveles o las observaciones exceden 500 caracteres.")
	}
	product, exists := snapshot.Products[id]
	if !exists {
		return nil, invalid("El producto seleccionado no existe.")
	}
	category, exists := snapshot.Categories[product.CategoryID]
	if !exists {
		return nil, invalid("La categoría del producto no existe.")
	}
	if reason := productAvailabilityReason(

		product,
		category,
		now,
		location,
		override,
	); reason != "" {
		return nil, unavailable(product.Name + ": " + reason)
	}
	base, origin, err := effectivePrice(

		product,
		variantID,
		snapshot.PriceList,
		now,
		location,
		override,
	)
	if err != nil {
		return nil, err
	}
	result := &ProductBreakdown{
		ProductID:   id,
		Revision:    product.Revision,
		CategoryID:  product.CategoryID,
		VariantID:   variantID,
		Name:        product.Name,
		BasePrice:   money(base),
		PriceOrigin: origin,
		Notes:       customization.Notes,
		Ingredients: []IngredientBreakdown{},
		Groups:      []GroupBreakdown{},
		Components:  []ComponentBreakdown{},
	}
	ingredientExtras, err := calculateIngredients(snapshot, product, customization, result)
	if err != nil {
		return nil, err
	}
	// Las indicaciones rápidas elegidas se validan contra los ingredientes ya calculados y se
	// congelan en el desglose; no generan cargos propios (etapa 8).
	quickNotes, err := resolveQuickNotes(snapshot.QuickNotes, product, customization.QuickNotes, result.Ingredients)
	if err != nil {
		return nil, err
	}
	result.QuickNotes = quickNotes
	decrease := product.PriceCanDecrease && allowDecrease
	groups, err := engine.groups(product, customization, result, decrease)
	if err != nil {
		return nil, err
	}
	components, err := engine.components(product, customization, result, depth, decrease)
	if err != nil {
		return nil, err
	}
	// Sin la opción, el camino es el de siempre: base + ingredientes + Σ(diferencia + extras de cada
	// grupo) + cargos de componentes.
	groupExtras := groups.differences.Add(groups.extras)
	if decrease {
		adjustment := complementAdjustment(base, groups, components, result)
		groupExtras = groups.floorDifferences.Add(adjustment).Add(groups.extras)
		result.PriceCanDecrease = true
		result.ComplementAdjustment = money(adjustment)
	}
	result.IngredientExtras = money(ingredientExtras)
	result.GroupExtras = money(groupExtras)
	result.ComponentExtras = money(components.charges)
	result.UnitPrice = money(base.Add(ingredientExtras).Add(groupExtras).Add(components.charges))
	return result, nil
}

// complementAdjustment aplica la opción «El precio puede bajar al cambiar complementos» (§3.1) y
// devuelve la reducción respecto de la regla con piso: cero o negativa, nunca positiva. El núcleo
// del precio es base + diferencias con signo de los grupos + reducciones de los componentes (en un
// combo que también tiene la opción), y nunca queda:
//   - por debajo de cero;
//   - por debajo del valor de referencia de los complementos que permanecen seleccionados (los
//     propios y, en un combo, los de sus componentes);
//   - por encima del núcleo con la regla con piso: la opción solo permite bajar el precio.
//
// Los extras explícitos, los ingredientes adicionales y los cargos de los componentes se suman
// después del núcleo, así que estos cargos independientes nunca se descuentan.
func complementAdjustment(base decimal.Decimal, groups groupTotals, components componentTotals, result *ProductBreakdown) decimal.Decimal {
	floorCore := base.Add(groups.floorDifferences)
	signedCore := base.Add(groups.differences).Add(components.adjustments)
	core := decimal.Max(signedCore, remainingComplementValue(result), decimal.Zero)
	core = decimal.Min(core, floorCore)
	return core.Sub(floorCore)
}

// remainingComplementValue es el valor de referencia de los complementos que permanecen
// seleccionados en un desglose: el valor solicitado de sus grupos y, en un combo, el de sus
// componentes por cada unidad.
func remainingComplementValue(breakdown *ProductBreakdown) decimal.Decimal {
	total := decimal.Zero
	for _, group := range breakdown.Groups {
		total = total.Add(amount(group.RequestedValue))
	}
	for _, component := range breakdown.Components {
		if component.Product == nil {
			continue
		}
		units := decimal.NewFromInt(int64(component.Quantity))
		total = total.Add(remainingComplementValue(component.Product).Mul(units))
	}
	return total
}

func calculateIngredients(snapshot Snapshot, product Product, customization Customization, result *ProductBreakdown) (decimal.Decimal, error) {
	requested := map[string]string{}
	for _, selection := range customization.Ingredients {
		if _, exists := requested[selection.IngredientID]; exists {
			return decimal.Zero, invalid("Un ingrediente está seleccionado dos veces.")
		}
		requested[selection.IngredientID] = selection.Quantity
	}
	total := decimal.Zero
	for _, rule := range product.Ingredients {
		value, exists := requested[rule.IngredientID]
		if !exists {
			value = rule.IncludedQuantity
		}
		delete(requested, rule.IngredientID)
		quantity, err := parseQuantity(value)
		if err != nil {
			return decimal.Zero, err
		}
		original := amount(rule.IncludedQuantity)
		if quantity.GreaterThan(amount(rule.MaxQuantity)) || (quantity.LessThan(original) && !rule.Removable) || (quantity.GreaterThan(original) && !rule.Addable) {
			return decimal.Zero, invalid("La cantidad solicitada de un ingrediente no está permitida.")
		}
		ingredient, exists := snapshot.Ingredients[rule.IngredientID]
		if !exists || (quantity.IsPositive() && !ingredient.Active) {
			return decimal.Zero, unavailable("Un ingrediente solicitado está inactivo o no existe.")
		}
		extra := positiveDifference(quantity, original).Mul(amount(rule.ExtraPrice)).Round(2)
		total = total.Add(extra)
		result.Ingredients = append(result.Ingredients, IngredientBreakdown{
			IngredientID:   rule.IngredientID,
			Name:           ingredient.Name,
			Included:       original.String(),
			Requested:      quantity.String(),
			Extra:          money(extra),
			RecipeQuantity: rule.RecipeQuantity,
		})
	}
	if len(requested) > 0 {
		return decimal.Zero, invalid("Se solicitó un ingrediente que no está configurado en el producto.")
	}
	return total, nil
}
