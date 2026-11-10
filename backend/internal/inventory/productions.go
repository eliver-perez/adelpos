package inventory

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Elaboraciones de subrecetas almacenables (§12 «Subreceta preparada previamente»). Registrar una
// elaboración (inventory.production.record):
//   - consume los insumos de la versión vigente de la receta × lotes (production_out; las
//     preparaciones inmediatas se descomponen y los insumos preparados se consumen como insumo);
//   - produce el insumo preparado con el valor total consumido (production_in, entrada a valor
//     exacto), de modo que su costo refleja lo que costó elaborarlo;
//   - guarda lo esperado (lotes × rendimiento efectivo), lo producido realmente y la merma de
//     elaboración (lotes × rendimiento − producido, nunca negativa).
// Después, vender un producto que usa la preparación consume el insumo preparado y nunca vuelve a
// descontar sus ingredientes. Es idempotente por operation_id y por entry_key. Las existencias
// negativas no bloquean: se avisan en warnings.

var maximumBatches = decimal.NewFromInt(100000)

// Production es una elaboración registrada. Los valores solo con inventory.costs.read.
type Production struct {
	ID                string             `json:"id"`
	RecipeID          string             `json:"recipe_id"`
	RecipeName        string             `json:"recipe_name"`
	RecipeVersion     int                `json:"recipe_version"`
	Item              ItemReference      `json:"item"`
	Warehouse         WarehouseReference `json:"warehouse"`
	Batches           string             `json:"batches"`
	ExpectedQuantity  string             `json:"expected_quantity"`
	ProducedQuantity  string             `json:"produced_quantity"`
	WasteQuantity     string             `json:"waste_quantity"`
	OccurredAt        time.Time          `json:"occurred_at"`
	Notes             string             `json:"notes"`
	ActorName         *string            `json:"actor_name"`
	CostsVisible      bool               `json:"costs_visible"`
	ConsumedValue     *string            `json:"consumed_value"`
	ConsumedValueShow *string            `json:"consumed_value_display"`
	UnitCost          *string            `json:"unit_cost"`
	UnitCostDisplay   *string            `json:"unit_cost_display"`
	Lines             []ProductionLine   `json:"lines"`
	Warnings          []Warning          `json:"warnings"`
}

// ProductionLine es un insumo consumido por la elaboración.
type ProductionLine struct {
	Item         ItemReference `json:"item"`
	Quantity     string        `json:"quantity"`
	StockAfter   string        `json:"stock_after"`
	Value        *string       `json:"value"`
	ValueDisplay *string       `json:"value_display"`
}

// ProductionPage es una página de elaboraciones (la más reciente primero).
type ProductionPage struct {
	Items        []*Production `json:"items"`
	Total        int           `json:"total"`
	Page         int           `json:"page"`
	PageSize     int           `json:"page_size"`
	CostsVisible bool          `json:"costs_visible"`
}

type productionInput struct {
	OperationID      string     `json:"operation_id"`
	RecipeID         string     `json:"recipe_id"`
	WarehouseID      *string    `json:"warehouse_id"`
	Batches          string     `json:"batches"`
	ProducedQuantity *string    `json:"produced_quantity"`
	OccurredAt       *time.Time `json:"occurred_at"`
	Notes            string     `json:"notes"`
}

// recordProduction registra una elaboración.
func (handler *service) recordProduction(_ []string) (any, error) {
	var input productionInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.RecipeID); err != nil {
		return nil, invalid("Elige la preparación almacenable que se elaboró.")
	}
	batches, err := parseQuantity(input.Batches, "El número de lotes")
	if err != nil {
		return nil, err
	}
	if batches.GreaterThan(maximumBatches) {
		return nil, invalid("El número de lotes excede el máximo admitido.")
	}
	notes := strings.TrimSpace(input.Notes)
	if textLength(notes) > 500 {
		return nil, invalid("La nota de la elaboración admite hasta 500 caracteres.")
	}
	occurredAt, err := handler.initialTime(input.OccurredAt)
	if err != nil {
		return nil, invalid("La fecha de la elaboración no puede ser futura.")
	}
	row, err := handler.readRecipeRow(strings.ToLower(input.RecipeID))
	if err != nil {
		return nil, err
	}
	recipe := row.summary
	if recipe.OwnerKind != ownerPreparation || recipe.PreparationMode == nil || *recipe.PreparationMode != preparationStored {
		return nil, conflict("RECIPE_NOT_STORED", "Solo se registran elaboraciones de preparaciones almacenables.")
	}
	if !recipe.Active {
		return nil, conflict("RECIPE_INACTIVE", "La preparación «"+recipe.Name+"» está desactivada.")
	}
	warehouse, err := handler.productionWarehouse(input.WarehouseID)
	if err != nil {
		return nil, err
	}
	calculator, err := handler.newCostCalculator()
	if err != nil {
		return nil, err
	}
	version, err := calculator.store.queryVersion("r.id=$2", recipe.ID)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveBatch(version, batches, calculator.store, warehouse.ID)
	if err != nil {
		return nil, err
	}
	if len(resolved.Diagnostics) > 0 {
		return nil, conflict("PREPARATION_INACTIVE", "Una preparación que usa la receta está desactivada: revísala antes de elaborar.")
	}
	entries := resolved.entries()
	if len(entries) == 0 {
		return nil, conflict("RECIPE_EMPTY", "La receta no consume ningún insumo.")
	}
	gross := version.YieldBase.Mul(batches).Round(quantityScale)
	expected := version.effectiveYield().Mul(batches).Round(quantityScale)
	produced := expected
	if input.ProducedQuantity != nil && strings.TrimSpace(*input.ProducedQuantity) != "" {
		produced, err = parseQuantity(*input.ProducedQuantity, "La cantidad producida")
		if err != nil {
			return nil, err
		}
	}
	if !produced.IsPositive() {
		return nil, invalid("La cantidad producida debe ser mayor que cero.")
	}
	waste := gross.Sub(produced)
	if waste.IsNegative() {
		waste = zero
	}
	id := newID()
	result := &Production{
		ID: id, RecipeID: recipe.ID, RecipeName: recipe.Name, RecipeVersion: version.Version, Warehouse: *warehouse,
		Batches: quantityText(batches), ExpectedQuantity: quantityText(expected), ProducedQuantity: quantityText(produced),
		WasteQuantity: quantityText(waste), OccurredAt: occurredAt, Notes: notes, CostsVisible: handler.costsVisible(),
		Lines: []ProductionLine{}, Warnings: []Warning{},
	}
	consumed := zero
	for index, entry := range entries {
		number := index + 1
		applied, err := Apply(handler.ctx, handler.tx, MovementInput{
			EntryKey: "production:" + id + ":" + itoa(number), Kind: KindProductionOut, ItemID: entry.Key.ItemID,
			WarehouseID: warehouse.ID, Quantity: entry.Quantity, OccurredAt: occurredAt, SourceKind: "production", SourceID: &id,
			SourceLine: &number, Reason: reference("Elaboración de " + recipe.Name), ActorID: handler.actorReference(),
			OperationID: handler.operationReference(),
		})
		if err != nil {
			return nil, err
		}
		consumed = consumed.Add(applied.Movement.Value.Neg())
		item, err := handler.itemReference(entry.Key.ItemID)
		if err != nil {
			return nil, err
		}
		line := ProductionLine{Item: *item, Quantity: quantityText(entry.Quantity), StockAfter: quantityText(applied.StockAfter)}
		if handler.costsVisible() {
			line.Value, line.ValueDisplay = moneyPointers(applied.Movement.Value.Neg())
		}
		if applied.NegativeStock {
			result.Warnings = append(result.Warnings, Warning{Code: "NEGATIVE_STOCK", ItemID: &item.ID,
				Message: "La existencia de «" + item.Name + "» en «" + warehouse.Name + "» quedó negativa."})
		}
		result.Lines = append(result.Lines, line)
	}
	itemID := *recipe.PreparationItemID
	producedEntry, err := Apply(handler.ctx, handler.tx, MovementInput{
		EntryKey: "production:" + id + ":in", Kind: KindProductionIn, ItemID: itemID, WarehouseID: warehouse.ID, Quantity: produced,
		Value: &consumed, OccurredAt: occurredAt, SourceKind: "production", SourceID: &id, Reason: reference("Elaboración de " + recipe.Name),
		ActorID: handler.actorReference(), OperationID: handler.operationReference(),
	})
	if err != nil {
		return nil, err
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_productions(id,recipe_id,recipe_version_id,item_id,warehouse_id,
            batches,expected_quantity,produced_quantity,waste_quantity,consumed_value,occurred_at,notes,actor_id,operation_id,movement_id)
        VALUES ($1,$2,$3,$4,$5,$6::numeric,$7::numeric,$8::numeric,$9::numeric,$10::numeric,$11,$12,$13,$14,$15)`, id, recipe.ID,
		version.VersionID, itemID, warehouse.ID, batches.String(), expected.String(), produced.String(), waste.String(),
		consumed.Round(valueScale).String(), occurredAt, notes, handler.actorReference(), handler.operationReference(),
		producedEntry.Movement.ID)
	if err != nil {
		return nil, err
	}
	item, err := handler.itemReference(itemID)
	if err != nil {
		return nil, err
	}
	result.Item = *item
	if handler.costsVisible() {
		result.ConsumedValue, result.ConsumedValueShow = moneyPointers(consumed)
		result.UnitCost, result.UnitCostDisplay = costPointers(consumed.DivRound(produced, costScale))
	}
	if err := handler.publish("inventory.stock.changed", &warehouse.ID); err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.movements.recorded", &id); err != nil {
		return nil, err
	}
	audited := map[string]any{
		"recipe_id": recipe.ID, "recipe_version": version.Version, "item_id": itemID, "warehouse_id": warehouse.ID,
		"batches": result.Batches, "expected_quantity": result.ExpectedQuantity, "produced_quantity": result.ProducedQuantity,
		"waste_quantity": result.WasteQuantity, "consumed_value": consumed.Round(valueScale).String(), "occurred_at": occurredAt,
	}
	err = handler.audit(handler.ctx, handler.tx, "inventory.production_recorded", "inventory_production", &id, nil, audited)
	return result, err
}

// productionWarehouse devuelve el almacén indicado (activo) o el predeterminado.
func (handler *service) productionWarehouse(requested *string) (*WarehouseReference, error) {
	warehouseID, err := validateOptionalID(requested)
	if err != nil {
		return nil, err
	}
	var warehouse *Warehouse
	if warehouseID == nil {
		warehouse, err = handler.defaultWarehouse()
	} else {
		warehouse, err = handler.readWarehouse(*warehouseID)
	}
	if err != nil {
		return nil, err
	}
	if !warehouse.Active {
		return nil, conflict("WAREHOUSE_INACTIVE", "El almacén «"+warehouse.Name+"» está desactivado.")
	}
	return &WarehouseReference{ID: warehouse.ID, Name: warehouse.Name}, nil
}

// listProductions lista elaboraciones (inventory.production.record o inventory.movements.read).
func (handler *service) listProductions(_ []string) (any, error) {
	if !handler.request.Actor.Can(permissionProductionRecord) && !handler.request.Actor.Can(permissionMovementsRead) {
		return nil, denied()
	}
	query := handler.request.Query
	if err := allowQuery(query, "item_id", "warehouse_id", "from", "until", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	for key, column := range map[string]string{"item_id": "p.item_id", "warehouse_id": "p.warehouse_id"} {
		identifier, err := optionalQueryID(query, key, "identificador")
		if err != nil {
			return nil, err
		}
		if identifier != nil {
			set.add(column+"=$%d", *identifier)
		}
	}
	from, until, err := handler.dateRange(query)
	if err != nil {
		return nil, err
	}
	if from != nil {
		set.add("p.occurred_at>=$%d", *from)
	}
	if until != nil {
		set.add("p.occurred_at<$%d", *until)
	}
	result := ProductionPage{Items: []*Production{}, Page: page, PageSize: size, CostsVisible: handler.costsVisible()}
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_productions p"+set.where(), set.arguments...).
		Scan(&result.Total); err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT p.id,p.recipe_id,r.name,v.version,p.item_id,p.warehouse_id,w.name,p.batches::text,
            p.expected_quantity::text,p.produced_quantity::text,p.waste_quantity::text,p.consumed_value::text,p.occurred_at,p.notes,
            actor.display_name
        FROM app.inventory_productions p JOIN app.inventory_recipes r ON r.id=p.recipe_id
        JOIN app.inventory_recipe_versions v ON v.id=p.recipe_version_id JOIN app.inventory_warehouses w ON w.id=p.warehouse_id
        LEFT JOIN app.users actor ON actor.id=p.actor_id`+set.where()+` ORDER BY p.occurred_at DESC, p.id LIMIT `+set.next(size)+
		" OFFSET "+set.next((page-1)*size), set.arguments...)
	if err != nil {
		return nil, err
	}
	type pending struct {
		production *Production
		itemID     string
		value      decimal.Decimal
		produced   decimal.Decimal
	}
	list := []pending{}
	for rows.Next() {
		production := &Production{Lines: []ProductionLine{}, Warnings: []Warning{}, CostsVisible: handler.costsVisible()}
		var itemID, batches, expected, produced, waste, value string
		if err := rows.Scan(&production.ID, &production.RecipeID, &production.RecipeName, &production.RecipeVersion, &itemID,
			&production.Warehouse.ID, &production.Warehouse.Name, &batches, &expected, &produced, &waste, &value, &production.OccurredAt,
			&production.Notes, &production.ActorName); err != nil {
			rows.Close()
			return nil, err
		}
		production.Batches = quantityText(parseDatabaseDecimal(batches))
		production.ExpectedQuantity = quantityText(parseDatabaseDecimal(expected))
		production.ProducedQuantity = quantityText(parseDatabaseDecimal(produced))
		production.WasteQuantity = quantityText(parseDatabaseDecimal(waste))
		list = append(list, pending{production: production, itemID: itemID, value: parseDatabaseDecimal(value),
			produced: parseDatabaseDecimal(produced)})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, entry := range list {
		item, err := handler.itemReference(entry.itemID)
		if err != nil {
			return nil, err
		}
		entry.production.Item = *item
		if handler.costsVisible() {
			entry.production.ConsumedValue, entry.production.ConsumedValueShow = moneyPointers(entry.value)
			entry.production.UnitCost, entry.production.UnitCostDisplay = costPointers(entry.value.DivRound(entry.produced, costScale))
		}
		result.Items = append(result.Items, entry.production)
	}
	return result, nil
}
