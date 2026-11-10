package inventory

import (
	"strings"

	"github.com/shopspring/decimal"
)

// Alta y nuevas versiones de recetas. Reglas:
//   - el destino (producto con variante, complemento o preparación con su modalidad e insumo) se
//     fija al crear y no cambia; producto, variante y complemento deben existir en el catálogo;
//   - cada guardado crea una versión inmutable (también desactivar); la anterior se conserva;
//   - unidades de la misma dimensión que la unidad base del insumo (o que el rendimiento de la
//     preparación), convertidas y guardadas en la unidad base;
//   - sin ciclos entre preparaciones, tampoco a través del insumo que produce una almacenable;
//   - una preparación usada por recetas activas no se desactiva.

const maximumRecipeLines = 200

var maximumWastePercent = decimal.NewFromInt(100)

type recipeInput struct {
	OperationID            string            `json:"operation_id"`
	Revision               int               `json:"revision"`
	OwnerKind              string            `json:"owner_kind"`
	ProductID              *string           `json:"product_id"`
	VariantID              *string           `json:"variant_id"`
	ComplementID           *string           `json:"complement_id"`
	PreparationMode        *string           `json:"preparation_mode"`
	PreparationItemID      *string           `json:"preparation_item_id"`
	Name                   string            `json:"name"`
	Active                 bool              `json:"active"`
	YieldQuantity          string            `json:"yield_quantity"`
	YieldUnitID            *string           `json:"yield_unit_id"`
	ProductionWastePercent string            `json:"production_waste_percent"`
	Notes                  string            `json:"notes"`
	Lines                  []recipeLineInput `json:"lines"`
}

type recipeLineInput struct {
	ComponentKind       string  `json:"component_kind"`
	ItemID              *string `json:"item_id"`
	PreparationRecipeID *string `json:"preparation_recipe_id"`
	IngredientID        *string `json:"ingredient_id"`
	Quantity            string  `json:"quantity"`
	UnitID              *string `json:"unit_id"`
	TakeawayOnly        bool    `json:"takeaway_only"`
}

// recipeTarget es el destino validado de una receta.
type recipeTarget struct {
	kind              string
	productID         *string
	variantID         *string
	complementID      *string
	preparationMode   *string
	preparationItemID *string
	defaultName       string
	preparationItem   *Item
}

// preparedRecipeLine es un renglón validado listo para guardar.
type preparedRecipeLine struct {
	kind          string
	itemID        *string
	preparationID *string
	ingredientID  *string
	quantity      decimal.Decimal
	unitID        string
	baseQuantity  decimal.Decimal
	takeawayOnly  bool
}

// preparedVersion son los datos validados de una versión nueva.
type preparedVersion struct {
	name          string
	active        bool
	yieldQuantity decimal.Decimal
	yieldUnitID   string
	yieldBase     decimal.Decimal
	yieldDim      string
	waste         decimal.Decimal
	notes         string
	lines         []preparedRecipeLine
}

// recipePermission es el permiso que exige escribir una receta del tipo indicado.
func recipePermission(kind string) string {
	if kind == ownerPreparation {
		return permissionSubrecipesManage
	}
	return permissionRecipesManage
}

// createRecipe da de alta una receta con su versión 1.
func (handler *service) createRecipe(_ []string) (any, error) {
	var input recipeInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	input.OwnerKind = normalizeKind(input.OwnerKind)
	if _, known := ownerKindLabels[input.OwnerKind]; !known {
		return nil, invalid("El tipo de receta debe ser producto, complemento o preparación.")
	}
	if err := handler.request.Actor.Require(recipePermission(input.OwnerKind)); err != nil {
		return nil, err
	}
	target, err := handler.validateRecipeTarget(input)
	if err != nil {
		return nil, err
	}
	id := newID()
	version, err := handler.validateRecipeVersion(input, target, id, "")
	if err != nil {
		return nil, err
	}
	if err := handler.checkRecipeCycles(id, target, version.lines); err != nil {
		return nil, err
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_recipes(id,owner_kind,product_id,variant_id,complement_id,
            preparation_mode,preparation_item_id,name,active,revision,created_by,updated_by)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,1,$10,$10)`, id, target.kind, target.productID, target.variantID, target.complementID,
		target.preparationMode, target.preparationItemID, version.name, version.active, handler.actorReference())
	if err != nil {
		return nil, recipeConflict(err)
	}
	if err := handler.insertRecipeVersion(id, 1, version); err != nil {
		return nil, err
	}
	saved, err := handler.recipeDetail(id)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.recipes.changed", &id); err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, "inventory.recipe_created", "inventory_recipe", &id, nil,
		recipeAuditState(saved)); err != nil {
		return nil, err
	}
	return saved, nil
}

// updateRecipe guarda una versión nueva de la receta con revisión optimista (revision = número de
// la versión vigente). El destino no cambia.
func (handler *service) updateRecipe(parts []string) (any, error) {
	var input recipeInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	row, err := handler.readRecipeRow(parts[1])
	if err != nil {
		return nil, err
	}
	previous := row.summary
	if err := handler.request.Actor.Require(recipePermission(previous.OwnerKind)); err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("La receta")
	}
	if err := sameRecipeTarget(input, previous); err != nil {
		return nil, err
	}
	target := recipeTarget{
		kind: previous.OwnerKind, productID: previous.ProductID, variantID: previous.VariantID, complementID: previous.ComplementID,
		preparationMode: previous.PreparationMode, preparationItemID: previous.PreparationItemID,
		// Sin nombre en la versión nueva se conserva el vigente.
		defaultName: previous.Name,
	}
	if target.preparationItemID != nil {
		item, err := handler.readItem(*target.preparationItemID)
		if err != nil {
			return nil, err
		}
		target.preparationItem = item
	}
	version, err := handler.validateRecipeVersion(input, target, previous.ID, previous.YieldUnit.Dimension)
	if err != nil {
		return nil, err
	}
	if err := handler.checkRecipeCycles(previous.ID, target, version.lines); err != nil {
		return nil, err
	}
	if previous.Active && !version.active && previous.OwnerKind == ownerPreparation {
		if err := handler.checkPreparationUnused(previous.ID); err != nil {
			return nil, err
		}
	}
	before, err := handler.recipeDetail(previous.ID)
	if err != nil {
		return nil, err
	}
	next := previous.Revision + 1
	if err := handler.insertRecipeVersion(previous.ID, next, version); err != nil {
		return nil, err
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_recipes SET name=$2,active=$3,revision=$4,updated_by=$5,updated_at=now()
        WHERE id=$1`, previous.ID, version.name, version.active, next, handler.actorReference())
	if err != nil {
		return nil, recipeConflict(err)
	}
	saved, err := handler.recipeDetail(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.recipes.changed", &saved.ID); err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, "inventory.recipe_versioned", "inventory_recipe", &saved.ID,
		recipeAuditState(before), recipeAuditState(saved)); err != nil {
		return nil, err
	}
	return saved, nil
}

// recipeAuditState es el estado auditado de una receta (con su versión y renglones; la auditoría
// puede contener costos y solo la consulta audit.read).
func recipeAuditState(recipe *Recipe) map[string]any {
	return map[string]any{
		"id": recipe.ID, "owner_kind": recipe.OwnerKind, "name": recipe.Name, "target": recipe.TargetName, "active": recipe.Active,
		"version": recipe.Version.Version, "version_id": recipe.Version.ID, "yield_quantity": recipe.Version.YieldQuantity,
		"yield_unit": recipe.Version.YieldUnit.Code, "production_waste_percent": recipe.Version.ProductionWastePercent,
		"lines": recipe.Version.Lines,
	}
}

// sameRecipeTarget rechaza un intento de cambiar el destino de una receta existente.
func sameRecipeTarget(input recipeInput, previous RecipeSummary) error {
	changed := func(sent, current *string) bool {
		if sent == nil || *sent == "" {
			return false
		}
		return current == nil || !strings.EqualFold(*sent, *current)
	}
	if (input.OwnerKind != "" && normalizeKind(input.OwnerKind) != previous.OwnerKind) || changed(input.ProductID, previous.ProductID) ||
		changed(input.VariantID, previous.VariantID) || changed(input.ComplementID, previous.ComplementID) ||
		changed(input.PreparationMode, previous.PreparationMode) || changed(input.PreparationItemID, previous.PreparationItemID) {
		return conflict("RECIPE_TARGET_IMMUTABLE",
			"El destino de una receta, la modalidad de una preparación y su insumo no cambian. Crea otra receta.")
	}
	return nil
}

func recipeConflict(err error) error {
	if !uniqueViolation(err) {
		return err
	}
	switch constraintName(err) {
	case "inventory_recipes_preparation_name_key":
		return conflict("DUPLICATE_NAME", "Ya existe una preparación con ese nombre.")
	case "inventory_recipes_preparation_item_key":
		return conflict("RECIPE_EXISTS", "El insumo preparado ya tiene su receta de elaboración.")
	}
	return conflict("RECIPE_EXISTS", "Ese producto, variante o complemento ya tiene receta: edítala para crear una versión nueva.")
}

// validateRecipeTarget valida el destino de una receta nueva contra el catálogo y el inventario.
func (handler *service) validateRecipeTarget(input recipeInput) (recipeTarget, error) {
	target := recipeTarget{kind: input.OwnerKind}
	productID, err := validateOptionalID(input.ProductID)
	if err != nil {
		return target, err
	}
	variantID, err := validateOptionalID(input.VariantID)
	if err != nil {
		return target, err
	}
	complementID, err := validateOptionalID(input.ComplementID)
	if err != nil {
		return target, err
	}
	itemID, err := validateOptionalID(input.PreparationItemID)
	if err != nil {
		return target, err
	}
	mode := ""
	if input.PreparationMode != nil {
		mode = normalizeKind(*input.PreparationMode)
	}
	switch target.kind {
	case ownerProduct:
		if productID == nil || complementID != nil || mode != "" || itemID != nil {
			return target, invalid("La receta de producto requiere el producto (y, si aplica, su variante) y nada más.")
		}
		var name string
		err := handler.tx.QueryRow(handler.ctx, "SELECT name FROM app.catalog_products WHERE id=$1", *productID).Scan(&name)
		if isNoRows(err) {
			return target, notFound("El producto no existe en el catálogo.")
		}
		if err != nil {
			return target, err
		}
		target.defaultName = name
		if variantID != nil {
			var variant string
			err := handler.tx.QueryRow(handler.ctx, "SELECT name FROM app.catalog_variants WHERE id=$1 AND product_id=$2", *variantID,
				*productID).Scan(&variant)
			if isNoRows(err) {
				return target, notFound("La variante no existe o no pertenece al producto.")
			}
			if err != nil {
				return target, err
			}
			target.defaultName = name + " · " + variant
		}
		target.productID, target.variantID = productID, variantID
	case ownerComplement:
		if complementID == nil || productID != nil || variantID != nil || mode != "" || itemID != nil {
			return target, invalid("La receta de complemento requiere el complemento y nada más.")
		}
		var name string
		err := handler.tx.QueryRow(handler.ctx, "SELECT name FROM app.catalog_complements WHERE id=$1", *complementID).Scan(&name)
		if isNoRows(err) {
			return target, notFound("El complemento no existe en el catálogo.")
		}
		if err != nil {
			return target, err
		}
		target.defaultName = name
		target.complementID = complementID
	case ownerPreparation:
		if productID != nil || variantID != nil || complementID != nil {
			return target, invalid("Una preparación no se liga a productos ni complementos del catálogo.")
		}
		if _, known := preparationModeLabels[mode]; !known {
			return target, invalid("Elige si la preparación es de elaboración inmediata o almacenable.")
		}
		target.preparationMode = &mode
		if mode == preparationNow {
			if itemID != nil {
				return target, invalid("Una preparación de elaboración inmediata no produce un insumo.")
			}
			return target, nil
		}
		if itemID == nil {
			return target, invalid("Elige el insumo preparado que produce la preparación almacenable.")
		}
		item, err := handler.readItem(*itemID)
		if err != nil {
			return target, err
		}
		if item.Kind != "prepared" {
			return target, conflict("ITEM_NOT_PREPARED", "El insumo «"+item.Name+"» no es de tipo «Preparación almacenable».")
		}
		if !item.Active {
			return target, conflict("ITEM_INACTIVE", "El insumo «"+item.Name+"» está desactivado.")
		}
		target.preparationItemID = itemID
		target.preparationItem = item
		target.defaultName = item.Name
	}
	return target, nil
}

// validateRecipeVersion valida nombre, rendimiento, merma, notas y renglones de una versión.
// previousDimension es la dimensión del rendimiento vigente (vacía al crear): el de una preparación
// no cambia de dimensión, porque los renglones que la usan están en esa dimensión.
func (handler *service) validateRecipeVersion(input recipeInput, target recipeTarget, recipeID, previousDimension string) (preparedVersion, error) {
	version := preparedVersion{active: input.Active}
	version.name = strings.TrimSpace(input.Name)
	if version.name == "" {
		version.name = strings.TrimSpace(target.defaultName)
	}
	if err := validateName(&version.name, "El nombre de la receta", 100); err != nil {
		return version, err
	}
	version.notes = input.Notes
	if err := validateDescription(&version.notes, 1000); err != nil {
		return version, err
	}
	if err := handler.validateRecipeYield(input, target, &version, previousDimension); err != nil {
		return version, err
	}
	if len(input.Lines) > maximumRecipeLines {
		return version, invalid("Una receta admite hasta 200 renglones.")
	}
	if version.active && len(input.Lines) == 0 {
		return version, invalid("Una receta activa requiere al menos un renglón.")
	}
	ingredients := map[string]bool{}
	for index, line := range input.Lines {
		prepared, err := handler.validateRecipeLine(line, index+1, target, recipeID, version.active)
		if err != nil {
			return version, err
		}
		if prepared.ingredientID != nil {
			if ingredients[*prepared.ingredientID] {
				return version, invalid("Renglón " + itoa(index+1) + ": el ingrediente ya tiene su porción en otro renglón.")
			}
			ingredients[*prepared.ingredientID] = true
		}
		version.lines = append(version.lines, prepared)
	}
	return version, nil
}

// validateRecipeYield valida el rendimiento y la merma de elaboración.
//   - Producto y complemento: en porciones (1 porción por omisión) y sin merma.
//   - Preparación almacenable: en la dimensión de la unidad base de su insumo.
//   - Preparación inmediata: cualquier dimensión, que ya no cambia en versiones posteriores.
func (handler *service) validateRecipeYield(input recipeInput, target recipeTarget, version *preparedVersion, previousDimension string) error {
	text := strings.TrimSpace(input.YieldQuantity)
	if text == "" {
		text = "1"
	}
	quantity, err := parseQuantity(text, "El rendimiento")
	if err != nil {
		return err
	}
	unitID, err := validateOptionalID(input.YieldUnitID)
	if err != nil {
		return err
	}
	if unitID == nil {
		switch {
		case target.kind != ownerPreparation:
			if err := handler.tx.QueryRow(handler.ctx, "SELECT id FROM app.inventory_units WHERE code='porcion'").Scan(&version.yieldUnitID); err != nil {
				return err
			}
		case target.preparationItem != nil:
			version.yieldUnitID = target.preparationItem.BaseUnit.ID
		default:
			return invalid("Indica la unidad del rendimiento de la preparación.")
		}
	} else {
		version.yieldUnitID = *unitID
	}
	unit, _, err := handler.unitMeasure(version.yieldUnitID, false)
	if err != nil {
		return err
	}
	version.yieldDim = unit.Dimension
	switch {
	case target.kind != ownerPreparation && unit.Dimension != "portion":
		return invalid("El rendimiento de una receta de producto o complemento se expresa en porciones.")
	case target.preparationItem != nil && unit.Dimension != target.preparationItem.BaseUnit.Dimension:
		return invalid("El rendimiento debe estar en la dimensión de la unidad base de «" + target.preparationItem.Name + "».")
	case previousDimension != "" && target.kind == ownerPreparation && unit.Dimension != previousDimension:
		return conflict("YIELD_DIMENSION_LOCKED", "El rendimiento de la preparación conserva su dimensión ("+
			dimensionLabels[previousDimension]+"): las recetas que la usan están en esa dimensión.")
	}
	// La unidad base de una preparación es la de su insumo preparado (almacenable) o la unidad de
	// equivalencia 1 de su dimensión (inmediata); productos y complementos, la porción.
	baseMeasure := measure{Dimension: unit.Dimension, Factor: decimal.NewFromInt(1)}
	if target.preparationItem != nil {
		itemMeasure, _, err := handler.unitMeasure(target.preparationItem.BaseUnit.ID, true)
		if err != nil {
			return err
		}
		baseMeasure = itemMeasure
	}
	base, err := convertQuantity(quantity, unit, baseMeasure)
	if err != nil {
		return err
	}
	version.yieldQuantity, version.yieldBase = quantity, base
	wasteText := strings.TrimSpace(input.ProductionWastePercent)
	if wasteText == "" {
		wasteText = "0"
	}
	waste, err := decimal.NewFromString(wasteText)
	if err != nil || waste.IsNegative() || !waste.LessThan(maximumWastePercent) || waste.Exponent() < -4 {
		return invalid("La merma de elaboración es un porcentaje de 0 a menos de 100, con hasta cuatro decimales.")
	}
	if target.kind != ownerPreparation && !waste.IsZero() {
		return invalid("La merma de elaboración solo aplica a preparaciones.")
	}
	version.waste = waste
	return nil
}

// validateRecipeLine valida un renglón y lo convierte a la unidad base de su destino.
func (handler *service) validateRecipeLine(input recipeLineInput, number int, target recipeTarget, recipeID string, active bool) (preparedRecipeLine,
	error) {
	prefix := "Renglón " + itoa(number) + ": "
	line := preparedRecipeLine{kind: normalizeKind(input.ComponentKind), takeawayOnly: input.TakeawayOnly}
	if _, known := lineKindLabels[line.kind]; !known {
		return line, invalid(prefix + "el tipo de componente debe ser insumo, preparación o ingrediente del catálogo.")
	}
	if line.kind == lineIngredient && target.kind != ownerProduct {
		return line, invalid(prefix + "solo las recetas de producto sobrescriben la porción de un ingrediente del catálogo.")
	}
	if line.takeawayOnly && (line.kind == lineIngredient || target.kind == ownerPreparation) {
		return line, invalid(prefix + "«solo para llevar» aplica a insumos y preparaciones de recetas de producto o complemento.")
	}
	itemID, err := validateOptionalID(input.ItemID)
	if err != nil {
		return line, invalid(prefix + "el insumo no es válido.")
	}
	preparationID, err := validateOptionalID(input.PreparationRecipeID)
	if err != nil {
		return line, invalid(prefix + "la preparación no es válida.")
	}
	ingredientID, err := validateOptionalID(input.IngredientID)
	if err != nil {
		return line, invalid(prefix + "el ingrediente no es válido.")
	}
	switch line.kind {
	case lineItem:
		if itemID == nil || preparationID != nil || ingredientID != nil {
			return line, invalid(prefix + "elige el insumo.")
		}
	case linePreparation:
		if preparationID == nil || itemID != nil || ingredientID != nil {
			return line, invalid(prefix + "elige la preparación.")
		}
	case lineIngredient:
		if ingredientID == nil || (itemID == nil) == (preparationID == nil) {
			return line, invalid(prefix + "elige el ingrediente del catálogo y el insumo o la preparación que consume.")
		}
		var exists bool
		if err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.catalog_ingredients WHERE id=$1)", *ingredientID).
			Scan(&exists); err != nil {
			return line, err
		}
		if !exists {
			return line, notFound("El ingrediente del catálogo no existe.")
		}
	}
	line.itemID, line.preparationID, line.ingredientID = itemID, preparationID, ingredientID
	quantity, err := parseQuantity(input.Quantity, prefix+"la cantidad")
	if err != nil {
		return line, err
	}
	line.quantity = quantity
	unitID, err := validateOptionalID(input.UnitID)
	if err != nil {
		return line, invalid(prefix + "la unidad no es válida.")
	}
	if itemID != nil {
		return line, handler.convertItemLine(&line, unitID, target, prefix, active)
	}
	return line, handler.convertPreparationLine(&line, unitID, recipeID, prefix, active)
}

// convertItemLine valida el insumo de un renglón y convierte su cantidad a la unidad base.
func (handler *service) convertItemLine(line *preparedRecipeLine, unitID *string, target recipeTarget, prefix string, active bool) error {
	item, err := handler.readItem(*line.itemID)
	if err != nil {
		return err
	}
	if active && !item.Active {
		return conflict("ITEM_INACTIVE", "El insumo «"+item.Name+"» está desactivado.")
	}
	if target.preparationItemID != nil && item.ID == *target.preparationItemID {
		return conflict("RECIPE_CYCLE", "La preparación no puede consumir el mismo insumo que produce.")
	}
	line.unitID = item.BaseUnit.ID
	if unitID != nil {
		line.unitID = *unitID
	}
	base, err := handler.toBaseQuantity(item, line.quantity, &line.unitID)
	if err != nil {
		return invalidPrefixed(prefix, err)
	}
	line.baseQuantity = base
	return nil
}

// preparationBaseUnit es la expresión SQL de la unidad base de la preparación r: la unidad base de
// su insumo preparado (almacenable) o la unidad de equivalencia 1 de la dimensión de su rendimiento
// (inmediata; la dimensión no cambia entre versiones).
const preparationBaseUnit = `coalesce((SELECT i.base_unit_id FROM app.inventory_items i WHERE i.id=r.preparation_item_id),
        (SELECT su.id FROM app.inventory_recipe_versions first JOIN app.inventory_units fu ON fu.id=first.yield_unit_id
            JOIN app.inventory_units su ON su.system AND su.factor=1 AND su.dimension=fu.dimension
            WHERE first.recipe_id=r.id AND first.version=1))`

// convertPreparationLine valida la preparación de un renglón y convierte su cantidad a la unidad
// base de la preparación.
func (handler *service) convertPreparationLine(line *preparedRecipeLine, unitID *string, recipeID, prefix string, active bool) error {
	if *line.preparationID == recipeID {
		return conflict("RECIPE_CYCLE", "Una preparación no puede usarse a sí misma.")
	}
	var kind, name, baseUnitID string
	var preparationActive bool
	err := handler.tx.QueryRow(handler.ctx, `SELECT r.owner_kind,r.name,r.active,`+preparationBaseUnit+`
        FROM app.inventory_recipes r WHERE r.id=$1`, *line.preparationID).Scan(&kind, &name, &preparationActive, &baseUnitID)
	if isNoRows(err) {
		return notFound("La preparación no existe.")
	}
	if err != nil {
		return err
	}
	if kind != ownerPreparation {
		return invalid(prefix + "solo se pueden usar recetas de preparación como componente.")
	}
	if active && !preparationActive {
		return conflict("PREPARATION_INACTIVE", "La preparación «"+name+"» está desactivada.")
	}
	baseMeasure, _, err := handler.unitMeasure(baseUnitID, true)
	if err != nil {
		return err
	}
	line.unitID = baseUnitID
	if unitID != nil {
		line.unitID = *unitID
	}
	unit, _, err := handler.unitMeasure(line.unitID, false)
	if err != nil {
		return err
	}
	base, err := convertQuantity(line.quantity, unit, baseMeasure)
	if err != nil {
		return invalidPrefixed(prefix, err)
	}
	line.baseQuantity = base
	return nil
}

// invalidPrefixed agrega el número de renglón al mensaje de un rechazo de validación.
func invalidPrefixed(prefix string, err error) error {
	if problem, ok := err.(*Error); ok && problem.Code == "INVENTORY_VALIDATION" && problem.Message != "" {
		message := []rune(problem.Message)
		return invalid(prefix + strings.ToLower(string(message[:1])) + string(message[1:]))
	}
	return err
}

// insertRecipeVersion guarda la versión y sus renglones. effective_from es la hora del guardado.
func (handler *service) insertRecipeVersion(recipeID string, number int, version preparedVersion) error {
	versionID := newID()
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_recipe_versions(id,recipe_id,version,name,active,yield_quantity,
            yield_unit_id,yield_base_quantity,production_waste_percent,notes,created_by,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6::numeric,$7,$8::numeric,$9::numeric,$10,$11,$12)`, versionID, recipeID, number, version.name,
		version.active, version.yieldQuantity.String(), version.yieldUnitID, version.yieldBase.String(), version.waste.String(),
		version.notes, handler.actorReference(), handler.operationReference())
	if err != nil {
		return err
	}
	for index, line := range version.lines {
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_recipe_lines(version_id,line_no,component_kind,item_id,
                preparation_recipe_id,ingredient_id,quantity,unit_id,base_quantity,takeaway_only)
            VALUES ($1,$2,$3,$4,$5,$6,$7::numeric,$8,$9::numeric,$10)`, versionID, index+1, line.kind, line.itemID, line.preparationID,
			line.ingredientID, line.quantity.String(), line.unitID, line.baseQuantity.String(), line.takeawayOnly)
		if err != nil {
			return err
		}
	}
	return nil
}

// checkPreparationUnused impide desactivar una preparación que usa la versión vigente de una receta
// activa: esa receta dejaría de consumir sin aviso. (Desactivar una almacenable solo impide nuevas
// elaboraciones: su insumo preparado se sigue consumiendo como cualquier insumo.)
func (handler *service) checkPreparationUnused(recipeID string) error {
	var name *string
	err := handler.tx.QueryRow(handler.ctx, `SELECT min(r.name) FROM app.inventory_recipes r
        JOIN app.inventory_recipe_versions v ON v.recipe_id=r.id AND v.version=r.revision
        JOIN app.inventory_recipe_lines l ON l.version_id=v.id
        WHERE r.active AND r.id<>$1 AND l.preparation_recipe_id=$1`, recipeID).Scan(&name)
	if err != nil {
		return err
	}
	if name != nil {
		return conflict("PREPARATION_IN_USE", "La preparación se usa en la receta «"+*name+"»: quítala de ahí antes de desactivarla.")
	}
	return nil
}

// checkRecipeCycles impide ciclos entre preparaciones. Las aristas salen de la versión vigente de
// cada preparación: hacia las preparaciones que usa y hacia la preparación almacenable que produce
// cada insumo que consume. Con los renglones nuevos, ninguna ruta puede volver a la receta.
func (handler *service) checkRecipeCycles(recipeID string, target recipeTarget, lines []preparedRecipeLine) error {
	if target.kind != ownerPreparation {
		return nil
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT r.id,l.preparation_recipe_id,producer.id
        FROM app.inventory_recipes r
        JOIN app.inventory_recipe_versions v ON v.recipe_id=r.id AND v.version=r.revision
        JOIN app.inventory_recipe_lines l ON l.version_id=v.id
        LEFT JOIN app.inventory_recipes producer ON producer.preparation_item_id=l.item_id
        WHERE r.owner_kind='preparation' AND r.id<>$1`, recipeID)
	if err != nil {
		return err
	}
	edges := map[string][]string{}
	for rows.Next() {
		var from string
		var preparation, producer *string
		if err := rows.Scan(&from, &preparation, &producer); err != nil {
			rows.Close()
			return err
		}
		for _, next := range []*string{preparation, producer} {
			if next != nil {
				edges[from] = append(edges[from], *next)
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, line := range lines {
		if line.preparationID != nil {
			edges[recipeID] = append(edges[recipeID], *line.preparationID)
		}
		if line.itemID != nil {
			var producer string
			err := handler.tx.QueryRow(handler.ctx, "SELECT id FROM app.inventory_recipes WHERE preparation_item_id=$1", *line.itemID).
				Scan(&producer)
			if err != nil && !isNoRows(err) {
				return err
			}
			if producer != "" {
				edges[recipeID] = append(edges[recipeID], producer)
			}
		}
	}
	if pathReturns(edges, recipeID) {
		return conflict("RECIPE_CYCLE", "La receta formaría un ciclo: una de sus preparaciones (o el insumo que produce) la usa a ella.")
	}
	return nil
}

// pathReturns indica si algún camino desde origin vuelve a origin.
func pathReturns(edges map[string][]string, origin string) bool {
	visited := map[string]bool{}
	stack := append([]string{}, edges[origin]...)
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == origin {
			return true
		}
		if visited[current] {
			continue
		}
		visited[current] = true
		stack = append(stack, edges[current]...)
	}
	return false
}
