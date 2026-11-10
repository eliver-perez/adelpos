package inventory

import (
	"time"

	"github.com/shopspring/decimal"
)

// Costo teórico de una receta (solo con inventory.costs.read). Recorre la versión vigente:
//   - insumo: cantidad base × costo vigente del insumo (promedio V/Q con existencia y valor
//     positivos; si no, el último costo válido; si no, el costo de referencia; sin ninguno, el
//     renglón queda «pendiente» y no suma);
//   - preparación inmediata: costo de sus renglones × cantidad ÷ rendimiento efectivo (recursivo);
//   - preparación almacenable o insumo preparado: el costo vigente del insumo preparado (nunca
//     vuelve a costear sus ingredientes);
//   - producto: renglones fijos ÷ rendimiento + ingredientes incluidos del catálogo × su porción
//     (la del renglón de ingrediente o la habitual); los «solo para llevar» se informan aparte.
// Los costos teóricos son los de hoy: el historial de versiones no lleva costos.

// costCalculator calcula costos con caché por insumo y preparación.
type costCalculator struct {
	handler *service
	store   *recipeStore
	items   map[string]*decimal.Decimal
	loaded  map[string]bool
}

func (handler *service) newCostCalculator() (*costCalculator, error) {
	var now time.Time
	if err := handler.tx.QueryRow(handler.ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return nil, err
	}
	return &costCalculator{
		handler: handler, store: newRecipeStore(handler.ctx, handler.tx, now), items: map[string]*decimal.Decimal{},
		loaded: map[string]bool{},
	}, nil
}

// itemCost es el costo vigente por unidad base de un insumo (nil si no tiene ninguno).
func (calculator *costCalculator) itemCost(itemID string) (*decimal.Decimal, error) {
	if calculator.loaded[itemID] {
		return calculator.items[itemID], nil
	}
	var quantity, value, last, reference *string
	err := calculator.handler.tx.QueryRow(calculator.handler.ctx, `SELECT v.quantity::text,v.value::text,v.last_valid_cost::text,
            i.reference_cost::text
        FROM app.inventory_items i LEFT JOIN app.inventory_valuations v ON v.item_id=i.id WHERE i.id=$1`, itemID).
		Scan(&quantity, &value, &last, &reference)
	if err != nil && !isNoRows(err) {
		return nil, err
	}
	calculator.loaded[itemID] = true
	var cost *decimal.Decimal
	if quantity != nil && value != nil {
		cost = averageCost(parseDatabaseDecimal(*quantity), parseDatabaseDecimal(*value), parseDatabaseOptional(last))
	}
	if cost == nil {
		cost = parseDatabaseOptional(reference)
	}
	calculator.items[itemID] = cost
	return cost, nil
}

// costedValue es un valor teórico con su marca de costo pendiente.
type costedValue struct {
	Value   decimal.Decimal
	Pending bool
}

func (left costedValue) plus(right costedValue) costedValue {
	return costedValue{Value: left.Value.Add(right.Value), Pending: left.Pending || right.Pending}
}

// itemValue es el valor de una cantidad base de un insumo.
func (calculator *costCalculator) itemValue(itemID string, quantity decimal.Decimal) (costedValue, error) {
	cost, err := calculator.itemCost(itemID)
	if err != nil {
		return costedValue{}, err
	}
	if cost == nil {
		return costedValue{Value: zero, Pending: true}, nil
	}
	return costedValue{Value: quantity.Mul(*cost)}, nil
}

// preparationValue es el valor de una cantidad (unidad base del rendimiento) de una preparación.
func (calculator *costCalculator) preparationValue(recipeID string, quantity decimal.Decimal, depth int) (costedValue, error) {
	if depth > maximumDepth {
		return costedValue{Pending: true}, nil
	}
	version, err := calculator.store.preparation(recipeID)
	if err != nil {
		return costedValue{}, err
	}
	if version == nil || !version.Active {
		return costedValue{Pending: true}, nil
	}
	if version.PreparationMode == preparationStored {
		return calculator.itemValue(version.PreparationItemID, quantity)
	}
	yield := version.effectiveYield()
	if !yield.IsPositive() {
		return costedValue{Pending: true}, nil
	}
	batch, err := calculator.linesValue(version.Lines, depth+1, false)
	if err != nil {
		return costedValue{}, err
	}
	return costedValue{Value: batch.Value.Mul(quantity).DivRound(yield, intermediateScale), Pending: batch.Pending}, nil
}

// lineValue es el valor de un renglón por rendimiento de su receta.
func (calculator *costCalculator) lineValue(line recipeLine, depth int) (costedValue, error) {
	if line.ItemID != "" {
		return calculator.itemValue(line.ItemID, line.BaseQuantity)
	}
	return calculator.preparationValue(line.PreparationRecipeID, line.BaseQuantity, depth)
}

// linesValue suma los renglones de insumo y preparación (sin los de ingrediente); takeaway elige
// entre los renglones normales y los «solo para llevar».
func (calculator *costCalculator) linesValue(lines []recipeLine, depth int, takeaway bool) (costedValue, error) {
	total := costedValue{Value: zero}
	for _, line := range lines {
		if line.Kind == lineIngredient || line.TakeawayOnly != takeaway {
			continue
		}
		value, err := calculator.lineValue(line, depth)
		if err != nil {
			return costedValue{}, err
		}
		total = total.plus(value)
	}
	return total, nil
}

func moneyPointers(value decimal.Decimal) (*string, *string) {
	exact, display := exactMoneyText(value.Round(valueScale)), displayMoneyText(value)
	return &exact, &display
}

func costPointers(value decimal.Decimal) (*string, *string) {
	exact, display := exactMoneyText(value.Round(costScale)), displayUnitCostText(value)
	return &exact, &display
}

// attachRecipeCosts agrega a una receta sus ingredientes del catálogo (productos) y, con
// inventory.costs.read, el costo teórico de la versión vigente y de cada renglón.
func (handler *service) attachRecipeCosts(recipe *Recipe) error {
	visible := handler.costsVisible()
	recipe.Cost.CostsVisible = visible
	calculator, err := handler.newCostCalculator()
	if err != nil {
		return err
	}
	version, err := calculator.store.queryVersion("r.id=$2", recipe.ID)
	if err != nil {
		return err
	}
	if version == nil {
		return nil
	}
	if visible {
		for index := range recipe.Version.Lines {
			line := version.Lines[index]
			if line.Kind == lineIngredient {
				continue
			}
			value, err := calculator.lineValue(line, 0)
			if err != nil {
				return err
			}
			recipe.Version.Lines[index].Cost, recipe.Version.Lines[index].CostDisplay = moneyPointers(value.Value)
			pending := value.Pending
			recipe.Version.Lines[index].CostPending = &pending
		}
	}
	ingredients := costedValue{Value: zero}
	if recipe.OwnerKind == ownerProduct && recipe.ProductID != nil {
		ingredients, err = handler.attachRecipeIngredients(recipe, version, calculator, visible)
		if err != nil {
			return err
		}
	}
	if !visible {
		return nil
	}
	normal, err := calculator.linesValue(version.Lines, 0, false)
	if err != nil {
		return err
	}
	if recipe.OwnerKind == ownerPreparation {
		recipe.Cost.BatchCost, recipe.Cost.BatchCostDisplay = moneyPointers(normal.Value)
		if yield := version.effectiveYield(); yield.IsPositive() {
			recipe.Cost.YieldUnitCost, recipe.Cost.YieldUnitCostDisplay = costPointers(normal.Value.DivRound(yield, costScale))
		}
		pending := normal.Pending
		recipe.Cost.CostPending = &pending
		return nil
	}
	takeaway, err := calculator.linesValue(version.Lines, 0, true)
	if err != nil {
		return err
	}
	unit := normal.Value.DivRound(version.YieldBase, intermediateScale).Add(ingredients.Value)
	recipe.Cost.UnitCost, recipe.Cost.UnitCostDisplay = moneyPointers(unit)
	recipe.Cost.TakeawayCost, recipe.Cost.TakeawayCostDisplay = moneyPointers(takeaway.Value.DivRound(version.YieldBase, intermediateScale))
	pending := normal.Pending || ingredients.Pending || takeaway.Pending
	recipe.Cost.CostPending = &pending
	return nil
}

// attachRecipeIngredients describe los ingredientes del catálogo del producto con la porción que
// se les aplica y devuelve el valor de la cantidad incluida (para el costo de una unidad).
func (handler *service) attachRecipeIngredients(recipe *Recipe, version *recipeVersion, calculator *costCalculator,
	visible bool) (costedValue, error) {
	overrides := map[string]recipeLine{}
	for _, line := range version.Lines {
		if line.Kind == lineIngredient {
			overrides[line.IngredientID] = line
		}
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT pi.ingredient_id,ing.name,pi.included_quantity::text,pi.removable,pi.addable
        FROM app.catalog_product_ingredients pi JOIN app.catalog_ingredients ing ON ing.id=pi.ingredient_id
        WHERE pi.product_id=$1 ORDER BY ing.name, ing.id`, *recipe.ProductID)
	if err != nil {
		return costedValue{}, err
	}
	type entry struct {
		RecipeIngredient
		included decimal.Decimal
	}
	entries := []entry{}
	for rows.Next() {
		var item entry
		var included string
		if err := rows.Scan(&item.IngredientID, &item.Name, &included, &item.Removable, &item.Addable); err != nil {
			rows.Close()
			return costedValue{}, err
		}
		item.included = parseDatabaseDecimal(included)
		item.Included = item.included.String()
		entries = append(entries, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return costedValue{}, err
	}
	total := costedValue{Value: zero}
	for _, item := range entries {
		ingredient := item.RecipeIngredient
		ingredient.PortionSource = "none"
		var portion *ingredientPortion
		if override, exists := overrides[item.IngredientID]; exists {
			ingredient.PortionSource = "recipe"
			portion = &ingredientPortion{ItemID: override.ItemID, PreparationRecipeID: override.PreparationRecipeID,
				BaseQuantity: override.BaseQuantity}
		} else {
			portion, err = calculator.store.portion(item.IngredientID)
			if err != nil {
				return costedValue{}, err
			}
			if portion != nil {
				ingredient.PortionSource = "habitual"
			}
		}
		if portion != nil {
			if err := handler.describePortionTarget(&ingredient, portion); err != nil {
				return costedValue{}, err
			}
			perPortion := quantityText(portion.BaseQuantity)
			includedQuantity := quantityText(portion.BaseQuantity.Mul(item.included))
			ingredient.PortionQuantity, ingredient.IncludedQuantity = &perPortion, &includedQuantity
			if visible {
				var value costedValue
				if portion.PreparationRecipeID != "" {
					value, err = calculator.preparationValue(portion.PreparationRecipeID, portion.BaseQuantity.Mul(item.included), 0)
				} else {
					value, err = calculator.itemValue(portion.ItemID, portion.BaseQuantity.Mul(item.included))
				}
				if err != nil {
					return costedValue{}, err
				}
				ingredient.Cost, ingredient.CostDisplay = moneyPointers(value.Value)
				pending := value.Pending
				ingredient.CostPending = &pending
				total = total.plus(value)
			}
		}
		recipe.Ingredients = append(recipe.Ingredients, ingredient)
	}
	return total, nil
}

// describePortionTarget nombra el insumo o la preparación de una porción.
func (handler *service) describePortionTarget(ingredient *RecipeIngredient, portion *ingredientPortion) error {
	if portion.ItemID != "" {
		item, err := handler.itemReference(portion.ItemID)
		if err != nil {
			return err
		}
		ingredient.Item = item
		return nil
	}
	var name string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT name FROM app.inventory_recipes WHERE id=$1", portion.PreparationRecipeID).
		Scan(&name); err != nil {
		return err
	}
	ingredient.PreparationName = &name
	return nil
}
