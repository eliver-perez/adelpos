package inventory

import (
	"strings"
	"time"
)

// Niveles por almacén (§24 «Permitir niveles mínimos por almacén e insumo»). Un nivel es la
// existencia mínima y el punto de reabastecimiento de un insumo en un almacén, en su unidad base:
//   - sobrescriben los del insumo para ese almacén: las alertas de ese almacén y la consulta de
//     existencias filtrada por él comparan la existencia del almacén con su nivel; los del insumo
//     siguen comparándose con la existencia total;
//   - se administran con inventory.items.manage, con revisión optimista (0 = sin nivel) y auditoría;
//     dejar los dos valores vacíos quita el nivel del almacén.

// StockLevel es el nivel de un insumo en un almacén con la existencia del almacén. Quantity y las
// banderas solo se entregan con inventory.stock.read.
type StockLevel struct {
	Item             ItemReference      `json:"item"`
	Warehouse        WarehouseReference `json:"warehouse"`
	MinStock         *string            `json:"min_stock"`
	ReorderPoint     *string            `json:"reorder_point"`
	ItemMinStock     *string            `json:"item_min_stock"`
	ItemReorderPoint *string            `json:"item_reorder_point"`
	StockVisible     bool               `json:"stock_visible"`
	Quantity         *string            `json:"quantity"`
	BelowMinimum     *bool              `json:"below_minimum"`
	AtReorderPoint   *bool              `json:"at_reorder_point"`
	Configured       bool               `json:"configured"`
	Revision         int                `json:"revision"`
	UpdatedAt        *time.Time         `json:"updated_at"`
	UpdatedByName    *string            `json:"updated_by_name"`
}

// StockLevelPage es una página de niveles configurados.
type StockLevelPage struct {
	Items    []StockLevel `json:"items"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
}

const stockLevelColumns = `SELECT i.id,i.code,i.name,i.kind,i.active,u.id,u.code,u.name,u.dimension,w.id,w.name,l.min_stock::text,
        l.reorder_point::text,i.min_stock::text,i.reorder_point::text,coalesce(s.quantity,0)::text,l.revision,l.updated_at,
        updater.display_name
        FROM app.inventory_stock_levels l
        JOIN app.inventory_items i ON i.id=l.item_id
        JOIN app.inventory_units u ON u.id=i.base_unit_id
        JOIN app.inventory_warehouses w ON w.id=l.warehouse_id
        LEFT JOIN app.inventory_stock s ON s.item_id=l.item_id AND s.warehouse_id=l.warehouse_id
        LEFT JOIN app.users updater ON updater.id=l.updated_by`

func (handler *service) scanStockLevel(row interface{ Scan(...any) error }) (*StockLevel, error) {
	var level StockLevel
	var minimum, reorder, itemMinimum, itemReorder *string
	var quantity string
	var revision *int
	err := row.Scan(&level.Item.ID, &level.Item.Code, &level.Item.Name, &level.Item.Kind, &level.Item.Active, &level.Item.Unit.ID,
		&level.Item.Unit.Code, &level.Item.Unit.Name, &level.Item.Unit.Dimension, &level.Warehouse.ID, &level.Warehouse.Name, &minimum,
		&reorder, &itemMinimum, &itemReorder, &quantity, &revision, &level.UpdatedAt, &level.UpdatedByName)
	if err != nil {
		return nil, err
	}
	level.Configured = revision != nil
	if revision != nil {
		level.Revision = *revision
	}
	minimumValue := parseDatabaseOptional(minimum)
	reorderValue := parseDatabaseOptional(reorder)
	level.MinStock = optionalQuantityText(minimumValue)
	level.ReorderPoint = optionalQuantityText(reorderValue)
	level.ItemMinStock = optionalQuantityText(parseDatabaseOptional(itemMinimum))
	level.ItemReorderPoint = optionalQuantityText(parseDatabaseOptional(itemReorder))
	level.StockVisible = handler.request.Actor.Can(permissionStockRead)
	if level.StockVisible {
		stock := parseDatabaseDecimal(quantity)
		level.Quantity = optionalQuantityText(&stock)
		below := minimumValue != nil && stock.LessThan(*minimumValue)
		atReorder := reorderValue != nil && stock.LessThanOrEqual(*reorderValue)
		level.BelowMinimum, level.AtReorderPoint = &below, &atReorder
	}
	return &level, nil
}

// listStockLevels lista los niveles configurados con filtros de almacén, insumo y búsqueda.
func (handler *service) listStockLevels(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "warehouse_id", "item_id", "search", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	for key, column := range map[string]string{"warehouse_id": "l.warehouse_id", "item_id": "l.item_id"} {
		identifier, err := optionalQueryID(query, key, "identificador")
		if err != nil {
			return nil, err
		}
		if identifier != nil {
			set.add(column+"=$%d", *identifier)
		}
	}
	pattern, err := searchPattern(query)
	if err != nil {
		return nil, err
	}
	if pattern != "" {
		set.add("(i.name ILIKE $%[1]d OR i.code ILIKE $%[1]d)", pattern)
	}
	result := StockLevelPage{Items: []StockLevel{}, Page: page, PageSize: size}
	err = handler.tx.QueryRow(handler.ctx, `SELECT count(*) FROM app.inventory_stock_levels l
        JOIN app.inventory_items i ON i.id=l.item_id`+set.where(), set.arguments...).Scan(&result.Total)
	if err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, stockLevelColumns+set.where()+` ORDER BY lower(w.name), lower(i.name), i.id LIMIT `+
		set.next(size)+" OFFSET "+set.next((page-1)*size), set.arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		level, err := handler.scanStockLevel(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *level)
	}
	return result, rows.Err()
}

// readStockLevel lee el nivel de un insumo en un almacén; sin nivel configurado devuelve los datos
// del insumo y del almacén con revisión 0.
func (handler *service) readStockLevel(itemID, warehouseID string) (*StockLevel, error) {
	level, err := handler.scanStockLevel(handler.tx.QueryRow(handler.ctx, `SELECT i.id,i.code,i.name,i.kind,i.active,u.id,u.code,u.name,
            u.dimension,w.id,w.name,l.min_stock::text,l.reorder_point::text,i.min_stock::text,i.reorder_point::text,
            coalesce(s.quantity,0)::text,l.revision,l.updated_at,updater.display_name
        FROM app.inventory_items i
        JOIN app.inventory_units u ON u.id=i.base_unit_id
        CROSS JOIN app.inventory_warehouses w
        LEFT JOIN app.inventory_stock_levels l ON l.item_id=i.id AND l.warehouse_id=w.id
        LEFT JOIN app.inventory_stock s ON s.item_id=i.id AND s.warehouse_id=w.id
        LEFT JOIN app.users updater ON updater.id=l.updated_by
        WHERE i.id=$1 AND w.id=$2`, itemID, warehouseID))
	if isNoRows(err) {
		return nil, notFound("El insumo o el almacén no existen.")
	}
	return level, err
}

type stockLevelInput struct {
	OperationID  string  `json:"operation_id"`
	ItemID       string  `json:"item_id"`
	WarehouseID  string  `json:"warehouse_id"`
	Revision     int     `json:"revision"`
	MinStock     *string `json:"min_stock"`
	ReorderPoint *string `json:"reorder_point"`
}

// saveStockLevel crea, cambia o quita (los dos valores vacíos) el nivel de un insumo en un almacén.
func (handler *service) saveStockLevel(_ []string) (any, error) {
	var input stockLevelInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.ItemID); err != nil {
		return nil, invalid("Elige el insumo.")
	}
	if err := validateID(input.WarehouseID); err != nil {
		return nil, invalid("Elige el almacén.")
	}
	minimum, err := parseOptionalQuantity(input.MinStock, "La existencia mínima del almacén")
	if err != nil {
		return nil, err
	}
	reorder, err := parseOptionalQuantity(input.ReorderPoint, "El punto de reabastecimiento del almacén")
	if err != nil {
		return nil, err
	}
	itemID, warehouseID := strings.ToLower(input.ItemID), strings.ToLower(input.WarehouseID)
	_, err = handler.tx.Exec(handler.ctx, `SELECT 1 FROM app.inventory_stock_levels WHERE item_id=$1 AND warehouse_id=$2 FOR UPDATE`,
		itemID, warehouseID)
	if err != nil {
		return nil, err
	}
	previous, err := handler.readStockLevel(itemID, warehouseID)
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("El nivel del insumo en el almacén")
	}
	switch {
	case minimum == nil && reorder == nil && previous.Configured:
		_, err = handler.tx.Exec(handler.ctx, "DELETE FROM app.inventory_stock_levels WHERE item_id=$1 AND warehouse_id=$2", itemID,
			warehouseID)
	case minimum == nil && reorder == nil:
		// Nada configurado y nada que configurar: sin cambios.
		return previous, nil
	case previous.Configured:
		_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_stock_levels SET min_stock=$3::numeric,reorder_point=$4::numeric,
                revision=revision+1,updated_by=$5,updated_at=now() WHERE item_id=$1 AND warehouse_id=$2`, itemID, warehouseID,
			optionalDecimalText(minimum), optionalDecimalText(reorder), handler.request.Actor.UserID)
	default:
		_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_stock_levels(item_id,warehouse_id,min_stock,reorder_point,updated_by)
            VALUES ($1,$2,$3::numeric,$4::numeric,$5)`, itemID, warehouseID, optionalDecimalText(minimum), optionalDecimalText(reorder),
			handler.request.Actor.UserID)
	}
	if err != nil {
		return nil, err
	}
	saved, err := handler.readStockLevel(itemID, warehouseID)
	if err != nil {
		return nil, err
	}
	for _, kind := range []string{"inventory.items.changed", "inventory.alerts.changed"} {
		if err := handler.publish(kind, &itemID); err != nil {
			return nil, err
		}
	}
	audited := func(level *StockLevel) map[string]any {
		return map[string]any{"item_id": level.Item.ID, "item_name": level.Item.Name, "warehouse_id": level.Warehouse.ID,
			"warehouse_name": level.Warehouse.Name, "min_stock": level.MinStock, "reorder_point": level.ReorderPoint,
			"revision": level.Revision}
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.stock_level_saved", "inventory_stock_level", &itemID,
		audited(previous), audited(saved))
}
