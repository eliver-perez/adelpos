package inventory

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Costo teórico de las recetas para los reportes de la etapa 9A («Costo teórico por producto y por
// receta»). Es el mismo motor de recipe_costs.go (costCalculator y recipeStore), sin permisos ni
// locks: solo ejecuta SELECT en la transacción que recibe, que en los reportes es la de solo lectura
// con fotografía consistente. Quien lo llama decide si el usuario puede ver costos
// (inventory.costs.read); este archivo no lo comprueba. Los costos son los vigentes (los de hoy),
// igual que en la pantalla de recetas: el historial de versiones no lleva costos.

// TheoreticalCost es el costo teórico de la versión vigente de una receta.
type TheoreticalCost struct {
	RecipeID      string
	OwnerKind     string
	Name          string
	Active        bool
	Version       int
	ProductID     *string
	VariantID     *string
	ComplementID  *string
	YieldQuantity decimal.Decimal
	// UnitCost es el costo de una unidad vendida (producto o complemento: renglones ÷ rendimiento +
	// ingredientes incluidos del catálogo con su porción) o de una unidad base del rendimiento
	// (preparación). nil en una preparación sin rendimiento efectivo.
	UnitCost *decimal.Decimal
	// BatchCost es el costo de los renglones de la versión (una tanda de la receta).
	BatchCost decimal.Decimal
	// TakeawayCost es el costo adicional por unidad de los renglones «solo para llevar».
	TakeawayCost decimal.Decimal
	// Pending indica que algún renglón no tiene costo vigente (no suma): el costo es incompleto.
	Pending bool
}

// TheoreticalCosts calcula el costo teórico vigente de todas las recetas con versión vigente a la
// hora de la transacción, con las mismas reglas que GET inventory/recipes/{id}.
func TheoreticalCosts(ctx context.Context, tx pgx.Tx) ([]TheoreticalCost, error) {
	handler := &service{ctx: ctx, tx: tx}
	calculator, err := handler.newCostCalculator()
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT r.id,r.owner_kind,r.name,r.active,r.product_id::text,r.variant_id::text,r.complement_id::text
        FROM app.inventory_recipes r ORDER BY r.owner_kind,r.name,r.id`)
	if err != nil {
		return nil, err
	}
	recipes := []TheoreticalCost{}
	for rows.Next() {
		var recipe TheoreticalCost
		if err := rows.Scan(&recipe.RecipeID, &recipe.OwnerKind, &recipe.Name, &recipe.Active, &recipe.ProductID, &recipe.VariantID,
			&recipe.ComplementID); err != nil {
			rows.Close()
			return nil, err
		}
		recipes = append(recipes, recipe)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]TheoreticalCost, 0, len(recipes))
	for _, recipe := range recipes {
		costed, found, err := calculator.theoreticalCost(recipe)
		if err != nil {
			return nil, err
		}
		if found {
			result = append(result, costed)
		}
	}
	return result, nil
}

// theoreticalCost aplica a una receta la composición de attachRecipeCosts. found es false si la
// receta no tiene versión vigente a la hora de la consulta.
func (calculator *costCalculator) theoreticalCost(recipe TheoreticalCost) (TheoreticalCost, bool, error) {
	version, err := calculator.store.queryVersion("r.id=$2", recipe.RecipeID)
	if err != nil || version == nil {
		return recipe, false, err
	}
	recipe.Version = version.Version
	recipe.YieldQuantity = version.YieldBase
	normal, err := calculator.linesValue(version.Lines, 0, false)
	if err != nil {
		return recipe, false, err
	}
	recipe.BatchCost = normal.Value.Round(valueScale)
	recipe.Pending = normal.Pending
	if recipe.OwnerKind == ownerPreparation {
		if yield := version.effectiveYield(); yield.IsPositive() {
			unit := normal.Value.DivRound(yield, costScale)
			recipe.UnitCost = &unit
		}
		return recipe, true, nil
	}
	ingredients := costedValue{Value: zero}
	if recipe.OwnerKind == ownerProduct && recipe.ProductID != nil {
		if ingredients, err = calculator.productIngredientsValue(*recipe.ProductID, version); err != nil {
			return recipe, false, err
		}
	}
	takeaway, err := calculator.linesValue(version.Lines, 0, true)
	if err != nil {
		return recipe, false, err
	}
	unit := normal.Value.DivRound(version.YieldBase, intermediateScale).Add(ingredients.Value).Round(valueScale)
	recipe.UnitCost = &unit
	recipe.TakeawayCost = takeaway.Value.DivRound(version.YieldBase, intermediateScale).Round(valueScale)
	recipe.Pending = normal.Pending || ingredients.Pending || takeaway.Pending
	return recipe, true, nil
}

// productIngredientsValue es el valor de las porciones incluidas de los ingredientes del catálogo
// del producto (la del renglón de ingrediente de la receta o la habitual), como
// attachRecipeIngredients pero sin describir los ingredientes.
func (calculator *costCalculator) productIngredientsValue(productID string, version *recipeVersion) (costedValue, error) {
	overrides := map[string]recipeLine{}
	for _, line := range version.Lines {
		if line.Kind == lineIngredient {
			overrides[line.IngredientID] = line
		}
	}
	handler := calculator.handler
	rows, err := handler.tx.Query(handler.ctx, `SELECT pi.ingredient_id,pi.included_quantity::text
        FROM app.catalog_product_ingredients pi JOIN app.catalog_ingredients ing ON ing.id=pi.ingredient_id
        WHERE pi.product_id=$1 ORDER BY ing.name, ing.id`, productID)
	if err != nil {
		return costedValue{}, err
	}
	type included struct {
		ingredientID string
		quantity     decimal.Decimal
	}
	entries := []included{}
	for rows.Next() {
		var entry included
		var quantity string
		if err := rows.Scan(&entry.ingredientID, &quantity); err != nil {
			rows.Close()
			return costedValue{}, err
		}
		entry.quantity = parseDatabaseDecimal(quantity)
		entries = append(entries, entry)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return costedValue{}, err
	}
	total := costedValue{Value: zero}
	for _, entry := range entries {
		var portion *ingredientPortion
		if override, exists := overrides[entry.ingredientID]; exists {
			portion = &ingredientPortion{ItemID: override.ItemID, PreparationRecipeID: override.PreparationRecipeID,
				BaseQuantity: override.BaseQuantity}
		} else {
			portion, err = calculator.store.portion(entry.ingredientID)
			if err != nil {
				return costedValue{}, err
			}
		}
		if portion == nil {
			continue
		}
		var value costedValue
		if portion.PreparationRecipeID != "" {
			value, err = calculator.preparationValue(portion.PreparationRecipeID, portion.BaseQuantity.Mul(entry.quantity), 0)
		} else {
			value, err = calculator.itemValue(portion.ItemID, portion.BaseQuantity.Mul(entry.quantity))
		}
		if err != nil {
			return costedValue{}, err
		}
		total = total.plus(value)
	}
	return total, nil
}
