package inventory

import (
	"time"

	"github.com/shopspring/decimal"
)

// Porción habitual de los ingredientes del catálogo (§8 y decisión 2 del diseño): la cantidad de un
// insumo que representa una porción del ingrediente. El consumo de un ingrediente es porciones
// pedidas × porción (0 con «Sin», 1 con lo incluido, 2 con «Extra» cuando lo incluido es 1). Una
// receta de producto puede sobrescribirla con un renglón de ingrediente. Historial de solo
// inserción: guardar crea una fila nueva con su vigencia; quitar la porción guarda una fila sin
// insumo. Se administra con inventory.recipes.manage. La revisión es el número de filas del
// historial del ingrediente.

const portionHistoryLimit = 20

// IngredientPortion es un ingrediente del catálogo con su porción vigente e historial reciente.
type IngredientPortion struct {
	IngredientID   string              `json:"ingredient_id"`
	IngredientName string              `json:"ingredient_name"`
	CatalogUnit    string              `json:"catalog_unit"`
	Active         bool                `json:"active"`
	Configured     bool                `json:"configured"`
	Item           *ItemReference      `json:"item"`
	Quantity       *string             `json:"quantity"`
	Unit           *UnitReference      `json:"unit"`
	BaseQuantity   *string             `json:"base_quantity"`
	EffectiveFrom  *time.Time          `json:"effective_from"`
	UpdatedByName  *string             `json:"updated_by_name"`
	Revision       int                 `json:"revision"`
	History        []PortionHistoryRow `json:"history"`
}

// PortionHistoryRow es una fila del historial (la más reciente primero).
type PortionHistoryRow struct {
	ItemName      *string   `json:"item_name"`
	Quantity      *string   `json:"quantity"`
	UnitCode      *string   `json:"unit_code"`
	BaseQuantity  *string   `json:"base_quantity"`
	EffectiveFrom time.Time `json:"effective_from"`
	ActorName     *string   `json:"actor_name"`
}

// IngredientPortionPage es una página del listado de porciones.
type IngredientPortionPage struct {
	Items    []*IngredientPortion `json:"items"`
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

// listIngredientPortions lista los ingredientes del catálogo con su porción vigente; configured
// filtra los que tienen (true) o no tienen (false) porción.
func (handler *service) listIngredientPortions(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "search", "active", "configured", "page", "page_size"); err != nil {
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
		set.add("g.name ILIKE $%d", pattern)
	}
	active, err := optionalBool(query, "active", "estado")
	if err != nil {
		return nil, err
	}
	if active != nil {
		set.add("g.active=$%d", *active)
	}
	configured, err := optionalBool(query, "configured", "porción")
	if err != nil {
		return nil, err
	}
	if configured != nil {
		set.add("(current.item_id IS NOT NULL)=$%d", *configured)
	}
	from := ` FROM app.catalog_ingredients g LEFT JOIN LATERAL (SELECT p.item_id FROM app.inventory_ingredient_portions p
        WHERE p.ingredient_id=g.id ORDER BY p.effective_from DESC, p.seq DESC LIMIT 1) current ON true`
	result := IngredientPortionPage{Items: []*IngredientPortion{}, Page: page, PageSize: size}
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*)"+from+set.where(), set.arguments...).Scan(&result.Total); err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, "SELECT g.id"+from+set.where()+" ORDER BY g.active DESC, lower(g.name), g.id LIMIT "+
		set.next(size)+" OFFSET "+set.next((page-1)*size), set.arguments...)
	if err != nil {
		return nil, err
	}
	identifiers := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		identifiers = append(identifiers, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range identifiers {
		portion, err := handler.ingredientPortion(id)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, portion)
	}
	return result, nil
}

// ingredientPortion lee la porción vigente de un ingrediente con su historial.
func (handler *service) ingredientPortion(ingredientID string) (*IngredientPortion, error) {
	portion := &IngredientPortion{IngredientID: ingredientID, History: []PortionHistoryRow{}}
	err := handler.tx.QueryRow(handler.ctx, "SELECT name,unit,active FROM app.catalog_ingredients WHERE id=$1", ingredientID).
		Scan(&portion.IngredientName, &portion.CatalogUnit, &portion.Active)
	if isNoRows(err) {
		return nil, notFound("El ingrediente del catálogo no existe.")
	}
	if err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT p.item_id,i.name,p.quantity::text,u.id,u.code,u.name,u.dimension,
            p.base_quantity::text,p.effective_from,actor.display_name,count(*) OVER (),
            row_number() OVER (ORDER BY p.effective_from DESC, p.seq DESC)
        FROM app.inventory_ingredient_portions p
        LEFT JOIN app.inventory_items i ON i.id=p.item_id
        LEFT JOIN app.inventory_units u ON u.id=p.unit_id
        LEFT JOIN app.users actor ON actor.id=p.actor_id
        WHERE p.ingredient_id=$1 ORDER BY p.effective_from DESC, p.seq DESC`, ingredientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var itemID, itemName, quantity, unitID, unitCode, unitName, unitDimension, base, actor *string
		var effective time.Time
		var total, position int
		if err := rows.Scan(&itemID, &itemName, &quantity, &unitID, &unitCode, &unitName, &unitDimension, &base, &effective, &actor,
			&total, &position); err != nil {
			return nil, err
		}
		history := PortionHistoryRow{ItemName: itemName, UnitCode: unitCode, EffectiveFrom: effective, ActorName: actor,
			Quantity: optionalQuantityText(parseDatabaseOptional(quantity)), BaseQuantity: optionalQuantityText(parseDatabaseOptional(base))}
		if position == 1 {
			portion.Revision = total
			portion.EffectiveFrom = &effective
			portion.UpdatedByName = actor
			if itemID != nil {
				portion.Configured = true
				portion.Quantity, portion.BaseQuantity = history.Quantity, history.BaseQuantity
				portion.Unit = &UnitReference{ID: *unitID, Code: *unitCode, Name: *unitName, Dimension: *unitDimension}
				portion.Item = &ItemReference{ID: *itemID, Name: *itemName}
			}
		}
		if position <= portionHistoryLimit {
			portion.History = append(portion.History, history)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if portion.Item != nil {
		item, err := handler.itemReference(portion.Item.ID)
		if err != nil {
			return nil, err
		}
		portion.Item = item
	}
	return portion, nil
}

type portionInput struct {
	OperationID string  `json:"operation_id"`
	Revision    int     `json:"revision"`
	ItemID      *string `json:"item_id"`
	Quantity    *string `json:"quantity"`
	UnitID      *string `json:"unit_id"`
}

// saveIngredientPortion registra la porción habitual del ingrediente de la ruta (o la retira con
// item_id null). Guardar la misma porción vigente no agrega historial.
func (handler *service) saveIngredientPortion(parts []string) (any, error) {
	var input portionInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	itemID, err := validateOptionalID(input.ItemID)
	if err != nil {
		return nil, err
	}
	unitID, err := validateOptionalID(input.UnitID)
	if err != nil {
		return nil, err
	}
	previous, err := handler.ingredientPortion(parts[1])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("La porción del ingrediente")
	}
	var quantity, base *decimal.Decimal
	var savedUnit *string
	if itemID != nil {
		item, err := handler.readItem(*itemID)
		if err != nil {
			return nil, err
		}
		if !item.Active {
			return nil, conflict("ITEM_INACTIVE", "El insumo «"+item.Name+"» está desactivado.")
		}
		if input.Quantity == nil {
			return nil, invalid("Indica la cantidad del insumo por porción.")
		}
		captured, err := parseQuantity(*input.Quantity, "La cantidad por porción")
		if err != nil {
			return nil, err
		}
		unit := item.BaseUnit.ID
		if unitID != nil {
			unit = *unitID
		}
		converted, err := handler.toBaseQuantity(item, captured, &unit)
		if err != nil {
			return nil, err
		}
		quantity, base, savedUnit = &captured, &converted, &unit
	} else if input.Quantity != nil || unitID != nil {
		return nil, invalid("Para quitar la porción envía el insumo vacío, sin cantidad ni unidad.")
	}
	if samePortion(previous, itemID, quantity, savedUnit) {
		return previous, nil
	}
	if !previous.Configured && itemID == nil {
		return previous, nil
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_ingredient_portions(ingredient_id,item_id,quantity,unit_id,
            base_quantity,actor_id,operation_id)
        VALUES ($1,$2,$3::numeric,$4,$5::numeric,$6,$7)`, parts[1], itemID, optionalDecimalText(quantity), savedUnit,
		optionalDecimalText(base), handler.actorReference(), handler.operationReference())
	if err != nil {
		return nil, err
	}
	saved, err := handler.ingredientPortion(parts[1])
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.recipes.changed", &saved.IngredientID); err != nil {
		return nil, err
	}
	before := map[string]any{"item_id": nil, "quantity": previous.Quantity, "unit": previous.Unit}
	if previous.Item != nil {
		before["item_id"] = previous.Item.ID
	}
	after := map[string]any{"item_id": itemID, "quantity": saved.Quantity, "unit": saved.Unit, "base_quantity": saved.BaseQuantity,
		"effective_from": saved.EffectiveFrom}
	err = handler.audit(handler.ctx, handler.tx, "inventory.ingredient_portion_saved", "catalog_ingredient", &saved.IngredientID, before, after)
	return saved, err
}

// samePortion compara la porción enviada con la vigente (insumo, cantidad capturada y unidad).
func samePortion(previous *IngredientPortion, itemID *string, quantity *decimal.Decimal, unitID *string) bool {
	if itemID == nil || previous.Item == nil {
		return itemID == nil && previous.Item == nil
	}
	if previous.Item.ID != *itemID || previous.Quantity == nil || previous.Unit == nil || quantity == nil || unitID == nil {
		return false
	}
	return parseDatabaseDecimal(*previous.Quantity).Equal(*quantity) && previous.Unit.ID == *unitID
}
