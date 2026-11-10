package inventory

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Lectura de recetas y porciones vigentes a una fecha. La usa el trabajador de consumo (a la
// fecha de la liberación, para que un cambio posterior de receta no altere el consumo) y el costo
// teórico (a la hora actual). Las versiones, sus renglones y las porciones son de solo inserción,
// por lo que la lectura es estable sin locks de catálogo.

// Tipos de renglón y modalidades de preparación.
const (
	lineItem          = "item"
	linePreparation   = "preparation"
	lineIngredient    = "ingredient"
	ownerProduct      = "product"
	ownerComplement   = "complement"
	ownerPreparation  = "preparation"
	preparationStored = "stored"
	preparationNow    = "immediate"
)

// versionAtColumns lee la versión vigente a $1 de las recetas filtradas.
const versionAtColumns = `SELECT r.id,r.owner_kind,coalesce(r.preparation_mode,''),coalesce(r.preparation_item_id::text,''),
        v.id,v.version,v.active,v.yield_base_quantity::text,v.production_waste_percent::text
        FROM app.inventory_recipes r
        JOIN LATERAL (SELECT * FROM app.inventory_recipe_versions x WHERE x.recipe_id=r.id AND x.effective_from<=$1
            ORDER BY x.effective_from DESC, x.version DESC LIMIT 1) v ON true`

// recipeStore resuelve la configuración vigente a una fecha con caché por consulta.
type recipeStore struct {
	ctx         context.Context
	tx          pgx.Tx
	at          time.Time
	recipes     map[string]*recipeVersion
	products    map[string]*recipeVersion
	complements map[string]*recipeVersion
	portions    map[string]*ingredientPortion
	loaded      map[string]bool
}

func newRecipeStore(ctx context.Context, tx pgx.Tx, at time.Time) *recipeStore {
	return &recipeStore{
		ctx: ctx, tx: tx, at: at,
		recipes:     map[string]*recipeVersion{},
		products:    map[string]*recipeVersion{},
		complements: map[string]*recipeVersion{},
		portions:    map[string]*ingredientPortion{},
		loaded:      map[string]bool{},
	}
}

// queryVersion lee una versión vigente (o nil si la receta no existía a la fecha) con sus renglones.
func (store *recipeStore) queryVersion(filter string, arguments ...any) (*recipeVersion, error) {
	values := append([]any{store.at}, arguments...)
	var version recipeVersion
	var yield, waste string
	err := store.tx.QueryRow(store.ctx, versionAtColumns+" WHERE "+filter, values...).Scan(&version.RecipeID, &version.OwnerKind,
		&version.PreparationMode, &version.PreparationItemID, &version.VersionID, &version.Version, &version.Active, &yield, &waste)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	version.YieldBase = parseDatabaseDecimal(yield)
	version.WastePercent = parseDatabaseDecimal(waste)
	lines, err := readVersionLines(store.ctx, store.tx, version.VersionID)
	if err != nil {
		return nil, err
	}
	version.Lines = lines
	store.recipes[version.RecipeID] = &version
	return &version, nil
}

// readVersionLines lee los renglones de una versión en su orden.
func readVersionLines(ctx context.Context, tx pgx.Tx, versionID string) ([]recipeLine, error) {
	rows, err := tx.Query(ctx, `SELECT component_kind,coalesce(item_id::text,''),coalesce(preparation_recipe_id::text,''),
            coalesce(ingredient_id::text,''),base_quantity::text,takeaway_only
        FROM app.inventory_recipe_lines WHERE version_id=$1 ORDER BY line_no`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []recipeLine{}
	for rows.Next() {
		var line recipeLine
		var quantity string
		if err := rows.Scan(&line.Kind, &line.ItemID, &line.PreparationRecipeID, &line.IngredientID, &quantity, &line.TakeawayOnly); err != nil {
			return nil, err
		}
		line.BaseQuantity = parseDatabaseDecimal(quantity)
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

// productRecipe devuelve la receta de la variante si existe y está activa a la fecha; si no, la del
// producto.
func (store *recipeStore) productRecipe(productID string, variantID *string) (*recipeVersion, error) {
	if variantID != nil && *variantID != "" {
		key := productID + "/" + *variantID
		if !store.loaded["variant:"+key] {
			version, err := store.queryVersion("r.owner_kind='product' AND r.product_id=$2 AND r.variant_id=$3", productID, *variantID)
			if err != nil {
				return nil, err
			}
			store.products[key] = version
			store.loaded["variant:"+key] = true
		}
		if version := store.products[key]; version != nil && version.Active {
			return version, nil
		}
	}
	if !store.loaded["product:"+productID] {
		version, err := store.queryVersion("r.owner_kind='product' AND r.product_id=$2 AND r.variant_id IS NULL", productID)
		if err != nil {
			return nil, err
		}
		store.products[productID] = version
		store.loaded["product:"+productID] = true
	}
	return store.products[productID], nil
}

func (store *recipeStore) complementRecipe(complementID string) (*recipeVersion, error) {
	if !store.loaded["complement:"+complementID] {
		version, err := store.queryVersion("r.owner_kind='complement' AND r.complement_id=$2", complementID)
		if err != nil {
			return nil, err
		}
		store.complements[complementID] = version
		store.loaded["complement:"+complementID] = true
	}
	return store.complements[complementID], nil
}

func (store *recipeStore) preparation(recipeID string) (*recipeVersion, error) {
	if version, cached := store.recipes[recipeID]; cached {
		return version, nil
	}
	if store.loaded["recipe:"+recipeID] {
		return nil, nil
	}
	store.loaded["recipe:"+recipeID] = true
	version, err := store.queryVersion("r.id=$2 AND r.owner_kind='preparation'", recipeID)
	if err != nil {
		return nil, err
	}
	return version, nil
}

// portion devuelve la porción habitual vigente a la fecha (nil si no existe o se retiró).
func (store *recipeStore) portion(ingredientID string) (*ingredientPortion, error) {
	if store.loaded["portion:"+ingredientID] {
		return store.portions[ingredientID], nil
	}
	store.loaded["portion:"+ingredientID] = true
	var itemID *string
	var quantity *string
	err := store.tx.QueryRow(store.ctx, `SELECT item_id::text,base_quantity::text FROM app.inventory_ingredient_portions
        WHERE ingredient_id=$1 AND effective_from<=$2 ORDER BY effective_from DESC, seq DESC LIMIT 1`, ingredientID, store.at).
		Scan(&itemID, &quantity)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if itemID == nil || quantity == nil {
		return nil, nil
	}
	portion := &ingredientPortion{ItemID: *itemID, BaseQuantity: parseDatabaseDecimal(*quantity)}
	store.portions[ingredientID] = portion
	return portion, nil
}
