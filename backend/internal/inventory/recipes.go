package inventory

import (
	"strings"
	"time"
)

// Recetas (§7 y §12 de la especificación). Una receta tiene identidad estable (producto con
// variante opcional, complemento o preparación) y versiones inmutables: cada guardado crea una
// versión con su vigencia, de modo que los consumos ya registrados nunca cambian y el trabajador
// resuelve cada liberación con la versión vigente a su fecha. Se administran con
// inventory.recipes.manage (productos y complementos) e inventory.subrecipes.manage
// (preparaciones). El costo teórico solo se entrega con inventory.costs.read.

var ownerKindLabels = map[string]string{
	ownerProduct:     "Producto",
	ownerComplement:  "Complemento",
	ownerPreparation: "Preparación",
}

var preparationModeLabels = map[string]string{
	preparationNow:    "Elaboración inmediata",
	preparationStored: "Preparada previamente (almacenable)",
}

var lineKindLabels = map[string]string{
	lineItem:        "Insumo",
	linePreparation: "Preparación",
	lineIngredient:  "Ingrediente del catálogo",
}

// ItemReference identifica un insumo en recetas y documentos.
type ItemReference struct {
	ID     string        `json:"id"`
	Code   string        `json:"code"`
	Name   string        `json:"name"`
	Kind   string        `json:"kind"`
	Active bool          `json:"active"`
	Unit   UnitReference `json:"unit"`
}

// RecipeSummary es una receta en el listado.
type RecipeSummary struct {
	ID                   string        `json:"id"`
	OwnerKind            string        `json:"owner_kind"`
	OwnerKindLabel       string        `json:"owner_kind_label"`
	Name                 string        `json:"name"`
	TargetName           string        `json:"target_name"`
	ProductID            *string       `json:"product_id"`
	VariantID            *string       `json:"variant_id"`
	ComplementID         *string       `json:"complement_id"`
	PreparationMode      *string       `json:"preparation_mode"`
	PreparationModeLabel *string       `json:"preparation_mode_label"`
	PreparationItemID    *string       `json:"preparation_item_id"`
	Active               bool          `json:"active"`
	Revision             int           `json:"revision"`
	EffectiveFrom        time.Time     `json:"effective_from"`
	LineCount            int           `json:"line_count"`
	YieldQuantity        string        `json:"yield_quantity"`
	YieldUnit            UnitReference `json:"yield_unit"`
	UpdatedAt            time.Time     `json:"updated_at"`
	UpdatedByName        *string       `json:"updated_by_name"`
}

// RecipePage es una página del listado de recetas.
type RecipePage struct {
	Items    []RecipeSummary `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

// Recipe es el detalle de una receta con su versión vigente y su historial.
type Recipe struct {
	RecipeSummary
	ProductName     *string              `json:"product_name"`
	VariantName     *string              `json:"variant_name"`
	ComplementName  *string              `json:"complement_name"`
	PreparationItem *ItemReference       `json:"preparation_item"`
	Version         RecipeVersionDetail  `json:"version"`
	Versions        []RecipeVersionEntry `json:"versions"`
	Ingredients     []RecipeIngredient   `json:"ingredients"`
	UsedBy          []RecipeUse          `json:"used_by"`
	Cost            RecipeCost           `json:"cost"`
}

// RecipeVersionDetail es una versión inmutable con sus renglones.
type RecipeVersionDetail struct {
	ID                     string        `json:"id"`
	RecipeID               string        `json:"recipe_id"`
	Version                int           `json:"version"`
	Name                   string        `json:"name"`
	Active                 bool          `json:"active"`
	YieldQuantity          string        `json:"yield_quantity"`
	YieldUnit              UnitReference `json:"yield_unit"`
	YieldBaseQuantity      string        `json:"yield_base_quantity"`
	ProductionWastePercent string        `json:"production_waste_percent"`
	EffectiveYield         string        `json:"effective_yield"`
	EffectiveFrom          time.Time     `json:"effective_from"`
	Notes                  string        `json:"notes"`
	CreatedByName          *string       `json:"created_by_name"`
	Lines                  []RecipeLine  `json:"lines"`
}

// RecipeLine es un renglón tal como lo entrega la API. Cost solo con inventory.costs.read.
type RecipeLine struct {
	LineNo              int            `json:"line_no"`
	ComponentKind       string         `json:"component_kind"`
	ComponentKindLabel  string         `json:"component_kind_label"`
	Item                *ItemReference `json:"item"`
	PreparationRecipeID *string        `json:"preparation_recipe_id"`
	PreparationName     *string        `json:"preparation_name"`
	IngredientID        *string        `json:"ingredient_id"`
	IngredientName      *string        `json:"ingredient_name"`
	Quantity            string         `json:"quantity"`
	Unit                UnitReference  `json:"unit"`
	BaseQuantity        string         `json:"base_quantity"`
	BaseUnit            UnitReference  `json:"base_unit"`
	TakeawayOnly        bool           `json:"takeaway_only"`
	Cost                *string        `json:"cost"`
	CostDisplay         *string        `json:"cost_display"`
	CostPending         *bool          `json:"cost_pending"`
}

// RecipeVersionEntry es una versión del historial (la más reciente primero).
type RecipeVersionEntry struct {
	ID            string    `json:"id"`
	Version       int       `json:"version"`
	Active        bool      `json:"active"`
	EffectiveFrom time.Time `json:"effective_from"`
	LineCount     int       `json:"line_count"`
	CreatedByName *string   `json:"created_by_name"`
}

// RecipeIngredient es un ingrediente del catálogo del producto de la receta con la porción que se
// le aplica (el renglón de ingrediente de esta receta o la porción habitual).
type RecipeIngredient struct {
	IngredientID     string         `json:"ingredient_id"`
	Name             string         `json:"name"`
	Included         string         `json:"included"`
	Removable        bool           `json:"removable"`
	Addable          bool           `json:"addable"`
	PortionSource    string         `json:"portion_source"`
	Item             *ItemReference `json:"item"`
	PreparationName  *string        `json:"preparation_name"`
	PortionQuantity  *string        `json:"portion_quantity"`
	IncludedQuantity *string        `json:"included_quantity"`
	Cost             *string        `json:"cost"`
	CostDisplay      *string        `json:"cost_display"`
	CostPending      *bool          `json:"cost_pending"`
}

// RecipeUse es una receta cuya versión vigente usa esta preparación.
type RecipeUse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	OwnerKind string `json:"owner_kind"`
	Active    bool   `json:"active"`
}

// RecipeCost es el costo teórico de la versión vigente (solo con inventory.costs.read): costo de
// una unidad vendida (renglones fijos e ingredientes incluidos; las opciones de grupo y los
// componentes de combo tienen sus propias recetas), el adicional de los renglones «solo para
// llevar» y, en preparaciones, el costo de un lote y por unidad base del rendimiento.
type RecipeCost struct {
	CostsVisible         bool    `json:"costs_visible"`
	UnitCost             *string `json:"unit_cost"`
	UnitCostDisplay      *string `json:"unit_cost_display"`
	TakeawayCost         *string `json:"takeaway_cost"`
	TakeawayCostDisplay  *string `json:"takeaway_cost_display"`
	BatchCost            *string `json:"batch_cost"`
	BatchCostDisplay     *string `json:"batch_cost_display"`
	YieldUnitCost        *string `json:"yield_unit_cost"`
	YieldUnitCostDisplay *string `json:"yield_unit_cost_display"`
	CostPending          *bool   `json:"cost_pending"`
}

// recipeRowColumns lee la identidad de la receta con los nombres de su destino.
const recipeRowColumns = `SELECT r.id,r.owner_kind,r.name,r.product_id,r.variant_id,r.complement_id,r.preparation_mode,
        r.preparation_item_id,r.active,r.revision,r.updated_at,updater.display_name,p.name,variant.name,c.name,
        latest.effective_from,latest.yield_quantity::text,u.id,u.code,u.name,u.dimension,
        (SELECT count(*) FROM app.inventory_recipe_lines l WHERE l.version_id=latest.id)
        FROM app.inventory_recipes r
        JOIN app.inventory_recipe_versions latest ON latest.recipe_id=r.id AND latest.version=r.revision
        JOIN app.inventory_units u ON u.id=latest.yield_unit_id
        LEFT JOIN app.catalog_products p ON p.id=r.product_id
        LEFT JOIN app.catalog_variants variant ON variant.id=r.variant_id
        LEFT JOIN app.catalog_complements c ON c.id=r.complement_id
        LEFT JOIN app.users updater ON updater.id=r.updated_by`

type recipeRow struct {
	summary        RecipeSummary
	productName    *string
	variantName    *string
	complementName *string
}

func scanRecipeRow(row interface{ Scan(...any) error }) (*recipeRow, error) {
	var result recipeRow
	summary := &result.summary
	var yield string
	err := row.Scan(&summary.ID, &summary.OwnerKind, &summary.Name, &summary.ProductID, &summary.VariantID, &summary.ComplementID,
		&summary.PreparationMode, &summary.PreparationItemID, &summary.Active, &summary.Revision, &summary.UpdatedAt,
		&summary.UpdatedByName, &result.productName, &result.variantName, &result.complementName, &summary.EffectiveFrom, &yield,
		&summary.YieldUnit.ID, &summary.YieldUnit.Code, &summary.YieldUnit.Name, &summary.YieldUnit.Dimension, &summary.LineCount)
	if err != nil {
		return nil, err
	}
	summary.YieldQuantity = quantityText(parseDatabaseDecimal(yield))
	summary.OwnerKindLabel = ownerKindLabels[summary.OwnerKind]
	if summary.PreparationMode != nil {
		label := preparationModeLabels[*summary.PreparationMode]
		summary.PreparationModeLabel = &label
	}
	summary.TargetName = recipeTargetName(summary.OwnerKind, result.productName, result.variantName, result.complementName, summary.PreparationModeLabel)
	return &result, nil
}

// recipeTargetName nombra el destino de una receta para los listados.
func recipeTargetName(kind string, product, variant, complement, mode *string) string {
	switch kind {
	case ownerProduct:
		name := ""
		if product != nil {
			name = *product
		}
		if variant != nil {
			name += " · " + *variant
		}
		return name
	case ownerComplement:
		if complement != nil {
			return *complement
		}
	case ownerPreparation:
		if mode != nil {
			return *mode
		}
	}
	return ""
}

func (handler *service) readRecipeRow(id string) (*recipeRow, error) {
	row, err := scanRecipeRow(handler.tx.QueryRow(handler.ctx, recipeRowColumns+" WHERE r.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("La receta no existe.")
	}
	return row, err
}

// listRecipes lista recetas con búsqueda (nombre de la receta o de su destino), filtros de tipo,
// estado y producto, y paginación.
func (handler *service) listRecipes(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "search", "owner_kind", "active", "product_id", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	pattern, err := searchPattern(query)
	if err != nil {
		return nil, err
	}
	if pattern != "" {
		set.add(`(r.name ILIKE $%[1]d OR p.name ILIKE $%[1]d OR variant.name ILIKE $%[1]d OR c.name ILIKE $%[1]d)`, pattern)
	}
	if kind := query.Get("owner_kind"); kind != "" {
		if _, known := ownerKindLabels[kind]; !known {
			return nil, invalid("El filtro de tipo de receta no es válido.")
		}
		set.add("r.owner_kind=$%d", kind)
	}
	active, err := optionalBool(query, "active", "estado")
	if err != nil {
		return nil, err
	}
	if active != nil {
		set.add("r.active=$%d", *active)
	}
	productID, err := optionalQueryID(query, "product_id", "producto")
	if err != nil {
		return nil, err
	}
	if productID != nil {
		set.add("r.product_id=$%d", *productID)
	}
	from := ` FROM app.inventory_recipes r LEFT JOIN app.catalog_products p ON p.id=r.product_id
        LEFT JOIN app.catalog_variants variant ON variant.id=r.variant_id LEFT JOIN app.catalog_complements c ON c.id=r.complement_id`
	result := RecipePage{Items: []RecipeSummary{}, Page: page, PageSize: size}
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*)"+from+set.where(), set.arguments...).Scan(&result.Total); err != nil {
		return nil, err
	}
	statement := recipeRowColumns + set.where() + ` ORDER BY r.active DESC, array_position(ARRAY['product','complement','preparation'],
        r.owner_kind), lower(r.name), r.id LIMIT ` + set.next(size) + " OFFSET " + set.next((page-1)*size)
	rows, err := handler.tx.Query(handler.ctx, statement, set.arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		row, err := scanRecipeRow(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, row.summary)
	}
	return result, rows.Err()
}

// readRecipeRoute devuelve el detalle de una receta.
func (handler *service) readRecipeRoute(parts []string) (any, error) {
	return handler.recipeDetail(parts[1])
}

// recipeDetail arma el detalle completo de una receta: versión vigente, historial, ingredientes del
// producto, recetas que usan la preparación y costo teórico.
func (handler *service) recipeDetail(id string) (*Recipe, error) {
	row, err := handler.readRecipeRow(id)
	if err != nil {
		return nil, err
	}
	recipe := &Recipe{
		RecipeSummary: row.summary, ProductName: row.productName, VariantName: row.variantName, ComplementName: row.complementName,
		Versions: []RecipeVersionEntry{}, Ingredients: []RecipeIngredient{}, UsedBy: []RecipeUse{},
	}
	if recipe.PreparationItemID != nil {
		item, err := handler.itemReference(*recipe.PreparationItemID)
		if err != nil {
			return nil, err
		}
		recipe.PreparationItem = item
	}
	var versionID string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT id FROM app.inventory_recipe_versions WHERE recipe_id=$1 AND version=$2",
		id, recipe.Revision).Scan(&versionID); err != nil {
		return nil, err
	}
	version, err := handler.versionDetail(versionID)
	if err != nil {
		return nil, err
	}
	recipe.Version = *version
	if err := handler.attachVersionHistory(recipe); err != nil {
		return nil, err
	}
	if err := handler.attachRecipeUses(recipe); err != nil {
		return nil, err
	}
	if err := handler.attachRecipeCosts(recipe); err != nil {
		return nil, err
	}
	return recipe, nil
}

// itemReference lee un insumo resumido.
func (handler *service) itemReference(id string) (*ItemReference, error) {
	var item ItemReference
	err := handler.tx.QueryRow(handler.ctx, `SELECT i.id,i.code,i.name,i.kind,i.active,u.id,u.code,u.name,u.dimension
        FROM app.inventory_items i JOIN app.inventory_units u ON u.id=i.base_unit_id WHERE i.id=$1`, id).Scan(&item.ID, &item.Code,
		&item.Name, &item.Kind, &item.Active, &item.Unit.ID, &item.Unit.Code, &item.Unit.Name, &item.Unit.Dimension)
	if isNoRows(err) {
		return nil, notFound("El insumo no existe.")
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// readRecipeVersionRoute devuelve una versión del historial (sin costos: los costos teóricos son
// siempre los de hoy).
func (handler *service) readRecipeVersionRoute(parts []string) (any, error) {
	return handler.versionDetail(parts[1])
}

// versionDetail lee una versión con sus renglones, nombres y unidades.
func (handler *service) versionDetail(versionID string) (*RecipeVersionDetail, error) {
	var detail RecipeVersionDetail
	var yield, base, waste string
	err := handler.tx.QueryRow(handler.ctx, `SELECT v.id,v.recipe_id,v.version,v.name,v.active,v.yield_quantity::text,
            u.id,u.code,u.name,u.dimension,v.yield_base_quantity::text,v.production_waste_percent::text,v.effective_from,v.notes,
            author.display_name
        FROM app.inventory_recipe_versions v JOIN app.inventory_units u ON u.id=v.yield_unit_id
        LEFT JOIN app.users author ON author.id=v.created_by WHERE v.id=$1`, versionID).Scan(&detail.ID, &detail.RecipeID,
		&detail.Version, &detail.Name, &detail.Active, &yield, &detail.YieldUnit.ID, &detail.YieldUnit.Code, &detail.YieldUnit.Name,
		&detail.YieldUnit.Dimension, &base, &waste, &detail.EffectiveFrom, &detail.Notes, &detail.CreatedByName)
	if isNoRows(err) {
		return nil, notFound("La versión de receta no existe.")
	}
	if err != nil {
		return nil, err
	}
	detail.YieldQuantity = quantityText(parseDatabaseDecimal(yield))
	detail.YieldBaseQuantity = quantityText(parseDatabaseDecimal(base))
	wastePercent := parseDatabaseDecimal(waste)
	detail.ProductionWastePercent = wastePercent.String()
	effective := (&recipeVersion{YieldBase: parseDatabaseDecimal(base), WastePercent: wastePercent}).effectiveYield()
	detail.EffectiveYield = quantityText(effective)
	rows, err := handler.tx.Query(handler.ctx, `SELECT l.line_no,l.component_kind,l.item_id,i.code,i.name,i.kind,i.active,
            iu.id,iu.code,iu.name,iu.dimension,l.preparation_recipe_id,prep.name,pu.id,pu.code,pu.name,pu.dimension,
            l.ingredient_id,ing.name,l.quantity::text,u.id,u.code,u.name,u.dimension,l.base_quantity::text,l.takeaway_only
        FROM app.inventory_recipe_lines l
        JOIN app.inventory_units u ON u.id=l.unit_id
        LEFT JOIN app.inventory_items i ON i.id=l.item_id
        LEFT JOIN app.inventory_units iu ON iu.id=i.base_unit_id
        LEFT JOIN app.inventory_recipes prep ON prep.id=l.preparation_recipe_id
        LEFT JOIN LATERAL (SELECT `+preparationBaseUnit+` AS unit_id FROM app.inventory_recipes r WHERE r.id=prep.id) pb ON true
        LEFT JOIN app.inventory_units pu ON pu.id=pb.unit_id
        LEFT JOIN app.catalog_ingredients ing ON ing.id=l.ingredient_id
        WHERE l.version_id=$1 ORDER BY l.line_no`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	detail.Lines = []RecipeLine{}
	for rows.Next() {
		line, err := scanRecipeLine(rows)
		if err != nil {
			return nil, err
		}
		detail.Lines = append(detail.Lines, line)
	}
	return &detail, rows.Err()
}

func scanRecipeLine(rows interface{ Scan(...any) error }) (RecipeLine, error) {
	var line RecipeLine
	var itemID, itemCode, itemName, itemKind, itemUnitID, itemUnitCode, itemUnitName, itemUnitDimension *string
	var itemActive *bool
	var prepUnitID, prepUnitCode, prepUnitName, prepUnitDimension *string
	var quantity, base string
	err := rows.Scan(&line.LineNo, &line.ComponentKind, &itemID, &itemCode, &itemName, &itemKind, &itemActive, &itemUnitID,
		&itemUnitCode, &itemUnitName, &itemUnitDimension, &line.PreparationRecipeID, &line.PreparationName, &prepUnitID, &prepUnitCode,
		&prepUnitName, &prepUnitDimension, &line.IngredientID, &line.IngredientName, &quantity, &line.Unit.ID, &line.Unit.Code,
		&line.Unit.Name, &line.Unit.Dimension, &base, &line.TakeawayOnly)
	if err != nil {
		return RecipeLine{}, err
	}
	line.ComponentKindLabel = lineKindLabels[line.ComponentKind]
	line.Quantity = quantityText(parseDatabaseDecimal(quantity))
	line.BaseQuantity = quantityText(parseDatabaseDecimal(base))
	if itemID != nil {
		line.Item = &ItemReference{ID: *itemID, Code: *itemCode, Name: *itemName, Kind: *itemKind, Active: *itemActive,
			Unit: UnitReference{ID: *itemUnitID, Code: *itemUnitCode, Name: *itemUnitName, Dimension: *itemUnitDimension}}
		line.BaseUnit = line.Item.Unit
	} else if prepUnitID != nil {
		line.BaseUnit = UnitReference{ID: *prepUnitID, Code: *prepUnitCode, Name: *prepUnitName, Dimension: *prepUnitDimension}
	}
	return line, nil
}

// attachVersionHistory agrega el historial de versiones (la más reciente primero, hasta 50).
func (handler *service) attachVersionHistory(recipe *Recipe) error {
	rows, err := handler.tx.Query(handler.ctx, `SELECT v.id,v.version,v.active,v.effective_from,
            (SELECT count(*) FROM app.inventory_recipe_lines l WHERE l.version_id=v.id),author.display_name
        FROM app.inventory_recipe_versions v LEFT JOIN app.users author ON author.id=v.created_by
        WHERE v.recipe_id=$1 ORDER BY v.version DESC LIMIT 50`, recipe.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var entry RecipeVersionEntry
		if err := rows.Scan(&entry.ID, &entry.Version, &entry.Active, &entry.EffectiveFrom, &entry.LineCount, &entry.CreatedByName); err != nil {
			return err
		}
		recipe.Versions = append(recipe.Versions, entry)
	}
	return rows.Err()
}

// attachRecipeUses agrega las recetas cuya versión vigente usa esta preparación (o su insumo).
func (handler *service) attachRecipeUses(recipe *Recipe) error {
	if recipe.OwnerKind != ownerPreparation {
		return nil
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT DISTINCT r.id,r.name,r.owner_kind,r.active
        FROM app.inventory_recipes r
        JOIN app.inventory_recipe_versions v ON v.recipe_id=r.id AND v.version=r.revision
        JOIN app.inventory_recipe_lines l ON l.version_id=v.id
        WHERE r.id<>$1 AND (l.preparation_recipe_id=$1 OR ($2::uuid IS NOT NULL AND l.item_id=$2::uuid))
        ORDER BY r.name, r.id`, recipe.ID, recipe.PreparationItemID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var use RecipeUse
		if err := rows.Scan(&use.ID, &use.Name, &use.OwnerKind, &use.Active); err != nil {
			return err
		}
		recipe.UsedBy = append(recipe.UsedBy, use)
	}
	return rows.Err()
}

// normalizeKind recorta y pasa a minúsculas un valor de lista cerrada.
func normalizeKind(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
