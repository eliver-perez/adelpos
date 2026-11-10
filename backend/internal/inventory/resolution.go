package inventory

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// Resolución del consumo teórico de una unidad comercial (§6 del diseño). Es una función pura: no
// lee la base de datos. Recibe el desglose canónico de la versión (quote_line), lo indispensable
// del catálogo congelado del pedido (pricing_snapshot), la modalidad congelada al liberar, la
// configuración de inventario vigente a la fecha de la liberación (recetas, porciones) y el
// enrutamiento de cocina de la unidad (estaciones por parte y asignación estación → almacén a esa
// misma fecha). Devuelve la cantidad base por (insumo, almacén) y un diagnóstico de lo que no tenía
// configuración. Nunca inventa consumos:
//   - producto o variante: la receta de la variante prevalece sobre la del producto; sus renglones
//     de insumo se multiplican por 1/rendimiento y sus preparaciones inmediatas se descomponen;
//   - ingredientes del catálogo: porciones pedidas × porción (la del renglón de ingrediente de la
//     receta del producto o, si no hay, la porción habitual vigente); sin porción, nada;
//   - opciones de grupo: porciones pedidas × factor de la porción del catálogo (1 si no tiene) ×
//     (receta del complemento; si no, la configuración estándar del producto vinculado; si no, la
//     porción habitual del ingrediente vinculado; si no, nada), sin importar el precio;
//   - componentes de combo: cada uno con su propia personalización × su cantidad, recursivo;
//   - renglones «solo para llevar» solo con la modalidad takeaway (incluye ventanilla);
//   - preparaciones almacenables: se consume su insumo preparado, nunca sus ingredientes;
//   - almacén por parte del árbol según la estación de cocina que la recibió.

// maximumDepth corta la recursión de combos y preparaciones (el catálogo ya limita los combos a 8
// niveles y las recetas se validan sin ciclos; esto es una defensa adicional).
const maximumDepth = 8

// intermediateScale es la precisión de los factores intermedios (divisiones entre rendimientos);
// el resultado final se redondea a quantityScale por (insumo, almacén).
const intermediateScale = 20

var hundred = decimal.NewFromInt(100)

// recipeLine es un renglón de una versión de receta, ya convertido a la unidad base.
type recipeLine struct {
	Kind                string
	ItemID              string
	PreparationRecipeID string
	IngredientID        string
	BaseQuantity        decimal.Decimal
	TakeawayOnly        bool
}

// recipeVersion es una versión de receta vigente a una fecha.
type recipeVersion struct {
	RecipeID          string
	VersionID         string
	Version           int
	OwnerKind         string
	PreparationMode   string
	PreparationItemID string
	Active            bool
	YieldBase         decimal.Decimal
	WastePercent      decimal.Decimal
	Lines             []recipeLine
}

// effectiveYield es el rendimiento base descontada la merma de elaboración.
func (version *recipeVersion) effectiveYield() decimal.Decimal {
	if version.WastePercent.IsZero() {
		return version.YieldBase
	}
	return version.YieldBase.Mul(hundred.Sub(version.WastePercent)).DivRound(hundred, intermediateScale)
}

// ingredientPortion es la porción vigente de un ingrediente del catálogo: un insumo o una
// preparación y su cantidad base por porción.
type ingredientPortion struct {
	ItemID              string
	PreparationRecipeID string
	BaseQuantity        decimal.Decimal
}

// consumptionConfig entrega la configuración de inventario vigente a la fecha de la liberación.
// Devolver nil significa «no existe» (la versión inactiva se devuelve con Active falso).
type consumptionConfig interface {
	productRecipe(productID string, variantID *string) (*recipeVersion, error)
	complementRecipe(complementID string) (*recipeVersion, error)
	preparation(recipeID string) (*recipeVersion, error)
	portion(ingredientID string) (*ingredientPortion, error)
}

// unitBreakdown es lo que el inventario lee del desglose canónico de la partida (quote_line).
type unitBreakdown struct {
	ProductID   string                `json:"product_id"`
	VariantID   *string               `json:"variant_id"`
	Name        string                `json:"name"`
	Ingredients []ingredientBreakdown `json:"ingredients"`
	Groups      []groupBreakdown      `json:"groups"`
	Components  []componentBreakdown  `json:"components"`
}

type ingredientBreakdown struct {
	IngredientID string `json:"ingredient_id"`
	Requested    string `json:"requested"`
}

type groupBreakdown struct {
	GroupID string           `json:"group_id"`
	Options []optionSnapshot `json:"options"`
}

type optionSnapshot struct {
	OptionID     string  `json:"option_id"`
	ComplementID string  `json:"complement_id"`
	PortionID    *string `json:"portion_id"`
	Requested    string  `json:"requested"`
}

type componentBreakdown struct {
	Quantity int            `json:"quantity"`
	Product  *unitBreakdown `json:"product"`
}

// pricingSnapshot es lo que el inventario lee del catálogo congelado del pedido. catalog.Snapshot
// se serializa sin etiquetas: sus claves llevan mayúscula inicial.
type pricingSnapshot struct {
	Products    map[string]snapshotProduct    `json:"Products"`
	Complements map[string]snapshotComplement `json:"Complements"`
	Portions    map[string]snapshotPortion    `json:"Portions"`
}

type snapshotProduct struct {
	Ingredients []struct {
		IngredientID     string `json:"ingredient_id"`
		IncludedQuantity string `json:"included_quantity"`
	} `json:"ingredients"`
}

type snapshotComplement struct {
	ProductID    *string `json:"product_id"`
	IngredientID *string `json:"ingredient_id"`
}

type snapshotPortion struct {
	Factor string `json:"factor"`
}

// stationRef es una estación de cocina que recibió una parte del árbol de la unidad.
type stationRef struct {
	ID   string
	Name string
}

// unitRouting son las estaciones por parte (path de kitchen_tasks.instructions.parts) y la
// asignación estación → almacén vigente a la fecha de la liberación (solo asignaciones con almacén;
// las que vuelven al predeterminado no cuentan como asignación).
type unitRouting struct {
	StationsByPath   map[string][]stationRef
	Assignments      map[string]string
	DefaultWarehouse string
}

// consumptionKey identifica un consumo por insumo y almacén.
type consumptionKey struct {
	ItemID      string
	WarehouseID string
}

// Diagnostic describe algo sin configuración (o un corte defensivo) durante la resolución. No
// contiene cantidades de costo.
type Diagnostic struct {
	Code         string  `json:"code"`
	Path         string  `json:"path"`
	ProductID    *string `json:"product_id,omitempty"`
	IngredientID *string `json:"ingredient_id,omitempty"`
	ComplementID *string `json:"complement_id,omitempty"`
	RecipeID     *string `json:"recipe_id,omitempty"`
}

// Códigos de diagnóstico de la resolución.
const (
	diagnosticNoConfiguration       = "no_configuration"
	diagnosticIngredientNoPortion   = "ingredient_without_portion"
	diagnosticComplementNoConfig    = "complement_without_configuration"
	diagnosticPreparationNotActive  = "preparation_unavailable"
	diagnosticDepthExceeded         = "depth_exceeded"
	diagnosticPreparationCycle      = "preparation_cycle"
	diagnosticNotConsumed           = "unit_not_consumed"
	diagnosticNothingConsumed       = "nothing_consumed"
	diagnosticAlreadyReversed       = "unit_reversed"
	diagnosticUnitAlreadyRegistered = "unit_already_registered"
)

// resolution es el resultado de resolver una unidad.
type resolution struct {
	Quantities  map[consumptionKey]decimal.Decimal
	Stations    map[string][]string
	Diagnostics []Diagnostic
}

// resolvedEntry es un consumo final redondeado.
type resolvedEntry struct {
	Key      consumptionKey
	Quantity decimal.Decimal
}

// entries devuelve los consumos redondeados a seis decimales (mitad alejándose de cero), sin
// ceros, en orden determinista (insumo, almacén).
func (result *resolution) entries() []resolvedEntry {
	list := make([]resolvedEntry, 0, len(result.Quantities))
	for key, quantity := range result.Quantities {
		rounded := quantity.Round(quantityScale)
		if rounded.IsZero() {
			continue
		}
		list = append(list, resolvedEntry{Key: key, Quantity: rounded})
	}
	sort.Slice(list, func(left, right int) bool {
		if list[left].Key.ItemID != list[right].Key.ItemID {
			return list[left].Key.ItemID < list[right].Key.ItemID
		}
		return list[left].Key.WarehouseID < list[right].Key.WarehouseID
	})
	return list
}

// resolver recorre el desglose de una unidad acumulando consumos.
type resolver struct {
	config   consumptionConfig
	snapshot pricingSnapshot
	takeaway bool
	routing  unitRouting
	result   *resolution
}

// resolveUnit calcula el consumo de una unidad comercial. Los errores solo provienen de la
// configuración (lecturas); la falta de configuración se informa en el diagnóstico.
func resolveUnit(breakdown unitBreakdown, snapshot pricingSnapshot, serviceMode string, config consumptionConfig,
	routing unitRouting) (*resolution, error) {
	state := &resolver{
		config:   config,
		snapshot: snapshot,
		takeaway: serviceMode == "takeaway",
		routing:  routing,
		result: &resolution{
			Quantities:  map[consumptionKey]decimal.Decimal{},
			Stations:    map[string][]string{},
			Diagnostics: []Diagnostic{},
		},
	}
	if err := state.visitProduct(breakdown, "root", decimal.NewFromInt(1), 0); err != nil {
		return nil, err
	}
	return state.result, nil
}

// warehouseFor elige el almacén de una parte: entre las estaciones que la recibieron (por nombre),
// la primera con almacén asignado; si ninguna tiene asignación, el predeterminado. Una parte que
// ninguna estación recibió hereda la de su padre (root/0/1 → root/0 → root).
func (state *resolver) warehouseFor(path string) (string, string) {
	for current := path; ; {
		stations := state.routing.StationsByPath[current]
		if len(stations) > 0 {
			ordered := append([]stationRef{}, stations...)
			sort.Slice(ordered, func(left, right int) bool {
				if ordered[left].Name != ordered[right].Name {
					return ordered[left].Name < ordered[right].Name
				}
				return ordered[left].ID < ordered[right].ID
			})
			for _, station := range ordered {
				if warehouse, assigned := state.routing.Assignments[station.ID]; assigned {
					return warehouse, station.Name
				}
			}
			return state.routing.DefaultWarehouse, ""
		}
		separator := strings.LastIndex(current, "/")
		if separator < 0 {
			return state.routing.DefaultWarehouse, ""
		}
		current = current[:separator]
	}
}

// add acumula una cantidad base de un insumo en el almacén de la parte.
func (state *resolver) add(itemID string, quantity decimal.Decimal, warehouse, station string) {
	if !quantity.IsPositive() {
		return
	}
	key := consumptionKey{ItemID: itemID, WarehouseID: warehouse}
	state.result.Quantities[key] = state.result.Quantities[key].Add(quantity)
	if station != "" {
		names := state.result.Stations[warehouse]
		for _, name := range names {
			if name == station {
				return
			}
		}
		state.result.Stations[warehouse] = append(names, station)
	}
}

func (state *resolver) diagnose(diagnostic Diagnostic) {
	for _, existing := range state.result.Diagnostics {
		if existing.Code == diagnostic.Code && existing.Path == diagnostic.Path && sameOptional(existing.ProductID, diagnostic.ProductID) &&
			sameOptional(existing.IngredientID, diagnostic.IngredientID) && sameOptional(existing.ComplementID, diagnostic.ComplementID) &&
			sameOptional(existing.RecipeID, diagnostic.RecipeID) {
			return
		}
	}
	state.result.Diagnostics = append(state.result.Diagnostics, diagnostic)
}

func sameOptional(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func reference(value string) *string {
	copied := value
	return &copied
}

// requestedQuantity interpreta las porciones pedidas del desglose (cadenas decimales de Go).
// Una cadena vacía o inválida cuenta como cero: el catálogo ya validó la selección.
func requestedQuantity(text string) decimal.Decimal {
	value, err := decimal.NewFromString(strings.TrimSpace(text))
	if err != nil || value.IsNegative() {
		return zero
	}
	return value
}

// visitProduct resuelve un producto del árbol (raíz o componente de combo) con su multiplicador.
func (state *resolver) visitProduct(product unitBreakdown, path string, multiplier decimal.Decimal, depth int) error {
	if depth > maximumDepth {
		state.diagnose(Diagnostic{Code: diagnosticDepthExceeded, Path: path, ProductID: reference(product.ProductID)})
		return nil
	}
	warehouse, station := state.warehouseFor(path)
	before := len(state.result.Quantities)
	consumedBefore := state.totalUnits()
	overrides := map[string]recipeLine{}
	recipe, err := state.config.productRecipe(product.ProductID, product.VariantID)
	if err != nil {
		return err
	}
	if recipe != nil && recipe.Active {
		for _, line := range recipe.Lines {
			if line.Kind == lineIngredient {
				overrides[line.IngredientID] = line
			}
		}
		if err := state.addRecipe(recipe, multiplier, warehouse, station, path, depth, map[string]bool{}); err != nil {
			return err
		}
	}
	for _, ingredient := range product.Ingredients {
		requested := requestedQuantity(ingredient.Requested)
		if requested.IsZero() {
			continue
		}
		if err := state.addIngredient(ingredient.IngredientID, requested.Mul(multiplier), overrides, warehouse, station, path,
			product.ProductID, depth); err != nil {
			return err
		}
	}
	for _, group := range product.Groups {
		for _, option := range group.Options {
			requested := requestedQuantity(option.Requested)
			if requested.IsZero() {
				continue
			}
			factor := decimal.NewFromInt(1)
			if option.PortionID != nil {
				if portion, exists := state.snapshot.Portions[*option.PortionID]; exists {
					if parsed := requestedQuantity(portion.Factor); parsed.IsPositive() {
						factor = parsed
					}
				}
			}
			if err := state.addComplement(option.ComplementID, requested.Mul(factor).Mul(multiplier), warehouse, station, path,
				depth); err != nil {
				return err
			}
		}
	}
	hasComponents := false
	for index, component := range product.Components {
		if component.Product == nil || component.Quantity <= 0 {
			continue
		}
		hasComponents = true
		childPath := path + "/" + itoa(index)
		if err := state.visitProduct(*component.Product, childPath, multiplier.Mul(decimal.NewFromInt(int64(component.Quantity))),
			depth+1); err != nil {
			return err
		}
	}
	// Un producto sin receta, sin porciones y sin componentes no consume nada: se señala para la
	// alerta de productos sin configuración, nunca con un consumo inventado.
	if !hasComponents && (recipe == nil || !recipe.Active) && len(state.result.Quantities) == before &&
		state.totalUnits().Equal(consumedBefore) {
		state.diagnose(Diagnostic{Code: diagnosticNoConfiguration, Path: path, ProductID: reference(product.ProductID)})
	}
	return nil
}

// totalUnits suma las cantidades acumuladas (para saber si una parte aportó consumo).
func (state *resolver) totalUnits() decimal.Decimal {
	total := zero
	for _, quantity := range state.result.Quantities {
		total = total.Add(quantity)
	}
	return total
}

// addRecipe agrega los renglones fijos de una receta de producto o complemento: cada unidad
// consume multiplicador ÷ rendimiento de cada renglón. Los renglones de ingrediente no se agregan
// aquí: sobrescriben la porción del ingrediente y dependen de las porciones pedidas.
func (state *resolver) addRecipe(recipe *recipeVersion, multiplier decimal.Decimal, warehouse, station, path string, depth int,
	visiting map[string]bool) error {
	per := multiplier
	if !recipe.YieldBase.Equal(decimal.NewFromInt(1)) && recipe.YieldBase.IsPositive() {
		per = multiplier.DivRound(recipe.YieldBase, intermediateScale)
	}
	for _, line := range recipe.Lines {
		if line.TakeawayOnly && !state.takeaway {
			continue
		}
		switch line.Kind {
		case lineItem:
			state.add(line.ItemID, line.BaseQuantity.Mul(per), warehouse, station)
		case linePreparation:
			if err := state.addPreparation(line.PreparationRecipeID, line.BaseQuantity.Mul(per), warehouse, station, path, depth+1,
				visiting); err != nil {
				return err
			}
		}
	}
	return nil
}

// addPreparation consume una cantidad (en la unidad base del rendimiento) de una preparación: la
// almacenable consume su insumo preparado y la inmediata se descompone en sus renglones ×
// cantidad ÷ rendimiento efectivo.
func (state *resolver) addPreparation(recipeID string, quantity decimal.Decimal, warehouse, station, path string, depth int,
	visiting map[string]bool) error {
	if depth > maximumDepth {
		state.diagnose(Diagnostic{Code: diagnosticDepthExceeded, Path: path, RecipeID: reference(recipeID)})
		return nil
	}
	if visiting[recipeID] {
		state.diagnose(Diagnostic{Code: diagnosticPreparationCycle, Path: path, RecipeID: reference(recipeID)})
		return nil
	}
	preparation, err := state.config.preparation(recipeID)
	if err != nil {
		return err
	}
	if preparation == nil || !preparation.Active {
		state.diagnose(Diagnostic{Code: diagnosticPreparationNotActive, Path: path, RecipeID: reference(recipeID)})
		return nil
	}
	if preparation.PreparationMode == preparationStored {
		state.add(preparation.PreparationItemID, quantity, warehouse, station)
		return nil
	}
	yield := preparation.effectiveYield()
	if !yield.IsPositive() {
		state.diagnose(Diagnostic{Code: diagnosticPreparationNotActive, Path: path, RecipeID: reference(recipeID)})
		return nil
	}
	per := quantity.DivRound(yield, intermediateScale)
	visiting[recipeID] = true
	defer delete(visiting, recipeID)
	for _, line := range preparation.Lines {
		if line.TakeawayOnly && !state.takeaway {
			continue
		}
		switch line.Kind {
		case lineItem:
			state.add(line.ItemID, line.BaseQuantity.Mul(per), warehouse, station)
		case linePreparation:
			if err := state.addPreparation(line.PreparationRecipeID, line.BaseQuantity.Mul(per), warehouse, station, path, depth+1,
				visiting); err != nil {
				return err
			}
		}
	}
	return nil
}

// addIngredient consume porciones de un ingrediente del catálogo: con el renglón de ingrediente de
// la receta del producto si existe; si no, con la porción habitual vigente; si no hay ninguna,
// nada (se señala «sin porción de referencia»).
func (state *resolver) addIngredient(ingredientID string, portions decimal.Decimal, overrides map[string]recipeLine, warehouse,
	station, path, productID string, depth int) error {
	if override, exists := overrides[ingredientID]; exists {
		return state.addTarget(override.ItemID, override.PreparationRecipeID, override.BaseQuantity.Mul(portions), warehouse, station,
			path, depth)
	}
	portion, err := state.config.portion(ingredientID)
	if err != nil {
		return err
	}
	if portion == nil {
		state.diagnose(Diagnostic{Code: diagnosticIngredientNoPortion, Path: path, ProductID: reference(productID),
			IngredientID: reference(ingredientID)})
		return nil
	}
	return state.addTarget(portion.ItemID, portion.PreparationRecipeID, portion.BaseQuantity.Mul(portions), warehouse, station, path, depth)
}

// addTarget consume una cantidad base de un insumo o de una preparación.
func (state *resolver) addTarget(itemID, preparationID string, quantity decimal.Decimal, warehouse, station, path string, depth int) error {
	if preparationID != "" {
		return state.addPreparation(preparationID, quantity, warehouse, station, path, depth+1, map[string]bool{})
	}
	state.add(itemID, quantity, warehouse, station)
	return nil
}

// addComplement consume porciones de una opción de grupo: receta del complemento; si no, la
// configuración estándar del producto vinculado (su receta y sus ingredientes incluidos); si no, la
// porción habitual del ingrediente vinculado. El precio de la opción no interviene.
func (state *resolver) addComplement(complementID string, portions decimal.Decimal, warehouse, station, path string, depth int) error {
	recipe, err := state.config.complementRecipe(complementID)
	if err != nil {
		return err
	}
	if recipe != nil && recipe.Active {
		return state.addRecipe(recipe, portions, warehouse, station, path, depth, map[string]bool{})
	}
	complement := state.snapshot.Complements[complementID]
	if complement.ProductID != nil {
		consumed, err := state.addLinkedProduct(*complement.ProductID, portions, warehouse, station, path, depth)
		if err != nil || consumed {
			return err
		}
	}
	if complement.IngredientID != nil {
		portion, err := state.config.portion(*complement.IngredientID)
		if err != nil {
			return err
		}
		if portion != nil {
			return state.addTarget(portion.ItemID, portion.PreparationRecipeID, portion.BaseQuantity.Mul(portions), warehouse, station,
				path, depth)
		}
	}
	state.diagnose(Diagnostic{Code: diagnosticComplementNoConfig, Path: path, ComplementID: reference(complementID)})
	return nil
}

// addLinkedProduct consume la configuración estándar de un producto vinculado a un complemento:
// los renglones de su receta y sus ingredientes en la cantidad incluida (con la porción del renglón
// de ingrediente o la habitual). Devuelve si aportó algún consumo.
func (state *resolver) addLinkedProduct(productID string, portions decimal.Decimal, warehouse, station, path string, depth int) (bool, error) {
	before := state.totalUnits()
	recipe, err := state.config.productRecipe(productID, nil)
	if err != nil {
		return false, err
	}
	overrides := map[string]recipeLine{}
	if recipe != nil && recipe.Active {
		for _, line := range recipe.Lines {
			if line.Kind == lineIngredient {
				overrides[line.IngredientID] = line
			}
		}
		if err := state.addRecipe(recipe, portions, warehouse, station, path, depth, map[string]bool{}); err != nil {
			return false, err
		}
	}
	for _, ingredient := range state.snapshot.Products[productID].Ingredients {
		included := requestedQuantity(ingredient.IncludedQuantity)
		if included.IsZero() {
			continue
		}
		if override, exists := overrides[ingredient.IngredientID]; exists {
			if err := state.addTarget(override.ItemID, override.PreparationRecipeID, override.BaseQuantity.Mul(included).Mul(portions),
				warehouse, station, path, depth); err != nil {
				return false, err
			}
			continue
		}
		portion, err := state.config.portion(ingredient.IngredientID)
		if err != nil {
			return false, err
		}
		if portion == nil {
			continue
		}
		if err := state.addTarget(portion.ItemID, portion.PreparationRecipeID, portion.BaseQuantity.Mul(included).Mul(portions), warehouse,
			station, path, depth); err != nil {
			return false, err
		}
	}
	return !state.totalUnits().Equal(before), nil
}

// decodeBreakdown interpreta el quote_line de una versión. Los arreglos nulos (pedidos creados
// desde Go con slices nil) cuentan como vacíos.
func decodeBreakdown(encoded []byte) (unitBreakdown, error) {
	var breakdown unitBreakdown
	if err := json.Unmarshal(encoded, &breakdown); err != nil {
		return unitBreakdown{}, err
	}
	return breakdown, nil
}

// decodeSnapshot interpreta el pricing_snapshot del pedido de la versión.
func decodeSnapshot(encoded []byte) (pricingSnapshot, error) {
	snapshot := pricingSnapshot{}
	if len(encoded) > 0 && string(encoded) != "null" {
		if err := json.Unmarshal(encoded, &snapshot); err != nil {
			return pricingSnapshot{}, err
		}
	}
	return snapshot, nil
}

// resolveBatch calcula lo que consumen los lotes de una preparación almacenable al elaborarse: sus
// renglones × lotes (los de preparación inmediata se descomponen; los insumos preparados se
// consumen como insumo), todo en el almacén de la elaboración.
func resolveBatch(version *recipeVersion, batches decimal.Decimal, config consumptionConfig, warehouse string) (*resolution, error) {
	state := &resolver{
		config:  config,
		routing: unitRouting{DefaultWarehouse: warehouse},
		result: &resolution{
			Quantities:  map[consumptionKey]decimal.Decimal{},
			Stations:    map[string][]string{},
			Diagnostics: []Diagnostic{},
		},
	}
	visiting := map[string]bool{version.RecipeID: true}
	for _, line := range version.Lines {
		switch line.Kind {
		case lineItem:
			state.add(line.ItemID, line.BaseQuantity.Mul(batches), warehouse, "")
		case linePreparation:
			if err := state.addPreparation(line.PreparationRecipeID, line.BaseQuantity.Mul(batches), warehouse, "", "root", 1,
				visiting); err != nil {
				return nil, err
			}
		}
	}
	return state.result, nil
}
