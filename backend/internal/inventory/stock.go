package inventory

import (
	"time"

	"github.com/shopspring/decimal"
)

// Existencias por insumo y almacén (§6). Se consultan con inventory.stock.read; los costos y el
// valor del insumo solo con inventory.costs.read y el valor total del inventario solo con
// inventory.valuation.read. El costo es uno solo por insumo: value es el valor total del insumo en
// todos los almacenes, aunque el listado se filtre por almacén.

// StockRow es la existencia de un insumo: en el almacén filtrado o total si no hay filtro.
type StockRow struct {
	ItemID         string        `json:"item_id"`
	ItemCode       string        `json:"item_code"`
	ItemName       string        `json:"item_name"`
	CategoryName   *string       `json:"category_name"`
	Unit           UnitReference `json:"unit"`
	Active         bool          `json:"active"`
	Quantity       string        `json:"quantity"`
	TotalQuantity  string        `json:"total_quantity"`
	Negative       bool          `json:"negative"`
	MinStock       *string       `json:"min_stock"`
	ReorderPoint   *string       `json:"reorder_point"`
	BelowMinimum   bool          `json:"below_minimum"`
	AtReorderPoint bool          `json:"at_reorder_point"`
	// LevelScope indica con qué se comparan los niveles: «item» (los del insumo contra la existencia
	// total) o «warehouse» (el nivel del almacén filtrado contra su existencia; fase D).
	LevelScope         string             `json:"level_scope"`
	Warehouses         []StockByWarehouse `json:"warehouses"`
	AverageCost        *string            `json:"average_cost"`
	AverageCostDisplay *string            `json:"average_cost_display"`
	Value              *string            `json:"value"`
	ValueDisplay       *string            `json:"value_display"`
	CostPending        *bool              `json:"cost_pending"`
	UpdatedAt          *time.Time         `json:"updated_at"`
}

// StockByWarehouse es el desglose por almacén (solo almacenes con existencia distinta de cero).
type StockByWarehouse struct {
	WarehouseID   string `json:"warehouse_id"`
	WarehouseName string `json:"warehouse_name"`
	Quantity      string `json:"quantity"`
	Negative      bool   `json:"negative"`
}

// StockPage es una página del listado de existencias.
type StockPage struct {
	Items             []*StockRow         `json:"items"`
	Total             int                 `json:"total"`
	Page              int                 `json:"page"`
	PageSize          int                 `json:"page_size"`
	Warehouse         *WarehouseReference `json:"warehouse"`
	CostsVisible      bool                `json:"costs_visible"`
	ValuationVisible  bool                `json:"valuation_visible"`
	TotalValue        *string             `json:"total_value"`
	TotalValueDisplay *string             `json:"total_value_display"`
}

// WarehouseReference identifica un almacén por nombre en las respuestas.
type WarehouseReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// stockLevels son las expresiones SQL de los niveles que se comparan en el listado: sin filtro de
// almacén, los del insumo contra la existencia total; con filtro, el nivel del almacén (fase D) contra
// la existencia del almacén cuando existe, y si no, los del insumo contra la existencia total.
type stockLevels struct {
	join     string
	minimum  string
	reorder  string
	quantity string
	scope    string
}

var totalStockLevels = stockLevels{
	minimum: "i.min_stock", reorder: "i.reorder_point", quantity: "coalesce(v.quantity,0)", scope: "'item'",
}

// warehouseStockLevels usa el nivel del almacén del primer argumento de la consulta.
var warehouseStockLevels = stockLevels{
	join:     " LEFT JOIN app.inventory_stock_levels sl ON sl.item_id=i.id AND sl.warehouse_id=$1",
	minimum:  "CASE WHEN sl.item_id IS NULL THEN i.min_stock ELSE sl.min_stock END",
	reorder:  "CASE WHEN sl.item_id IS NULL THEN i.reorder_point ELSE sl.reorder_point END",
	quantity: "CASE WHEN sl.item_id IS NULL THEN coalesce(v.quantity,0) ELSE scoped.quantity END",
	scope:    "CASE WHEN sl.item_id IS NULL THEN 'item' ELSE 'warehouse' END",
}

// stockStates son los filtros de situación de la existencia.
func (levels stockLevels) stockStates() map[string]string {
	return map[string]string{
		// existencia (filtrada) negativa
		"negative": "scoped.quantity < 0",
		// existencia filtrada en cero
		"zero": "scoped.quantity = 0",
		// existencia filtrada positiva
		"positive": "scoped.quantity > 0",
		// existencia por debajo del mínimo (total con el del insumo; del almacén con su nivel)
		"below_minimum": "(" + levels.minimum + ") IS NOT NULL AND " + levels.quantity + " < (" + levels.minimum + ")",
		// existencia en el punto de reabastecimiento o por debajo
		"reorder": "(" + levels.reorder + ") IS NOT NULL AND " + levels.quantity + " <= (" + levels.reorder + ")",
	}
}

// listStock lista la existencia por insumo con filtros de almacén, categoría, estado del insumo,
// situación de la existencia y búsqueda, con paginación.
func (handler *service) listStock(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "warehouse_id", "category_id", "search", "state", "active", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	result := StockPage{Items: []*StockRow{}, Page: page, PageSize: size, CostsVisible: handler.costsVisible(),
		ValuationVisible: handler.valuationVisible()}
	set := filters{}
	warehouseID, err := optionalQueryID(query, "warehouse_id", "almacén")
	if err != nil {
		return nil, err
	}
	scope := "TRUE"
	levels := totalStockLevels
	if warehouseID != nil {
		warehouse, err := handler.readWarehouse(*warehouseID)
		if err != nil {
			return nil, err
		}
		result.Warehouse = &WarehouseReference{ID: warehouse.ID, Name: warehouse.Name}
		set.arguments = append(set.arguments, warehouse.ID)
		scope = "s.warehouse_id=$1"
		levels = warehouseStockLevels
	}
	if err := handler.stockFilters(&set, levels); err != nil {
		return nil, err
	}
	from := `FROM app.inventory_items i
        JOIN app.inventory_units u ON u.id=i.base_unit_id
        LEFT JOIN app.inventory_categories c ON c.id=i.category_id
        LEFT JOIN app.inventory_valuations v ON v.item_id=i.id
        CROSS JOIN LATERAL (SELECT coalesce(sum(s.quantity),0) AS quantity, max(s.updated_at) AS updated_at
            FROM app.inventory_stock s WHERE s.item_id=i.id AND ` + scope + `) scoped` + levels.join
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) "+from+set.where(), set.arguments...).Scan(&result.Total); err != nil {
		return nil, err
	}
	statement := `SELECT i.id,i.code,i.name,c.name,u.id,u.code,u.name,u.dimension,i.active,scoped.quantity::text,
            coalesce(v.quantity,0)::text,(` + levels.minimum + `)::text,(` + levels.reorder + `)::text,coalesce(v.value,0)::text,
            v.last_valid_cost::text,coalesce(v.pending_cost_quantity,0)::text,scoped.updated_at,(` + levels.quantity + `)::text,
            ` + levels.scope + ` ` + from + set.where() +
		" ORDER BY i.active DESC, i.name, i.id LIMIT " + set.next(size) + " OFFSET " + set.next((page-1)*size)
	rows, err := handler.tx.Query(handler.ctx, statement, set.arguments...)
	if err != nil {
		return nil, err
	}
	byID := map[string]*StockRow{}
	for rows.Next() {
		row, err := handler.scanStockRow(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		result.Items = append(result.Items, row)
		byID[row.ItemID] = row
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := handler.attachWarehouseBreakdown(byID); err != nil {
		return nil, err
	}
	if result.ValuationVisible {
		total, err := handler.totalInventoryValue()
		if err != nil {
			return nil, err
		}
		exact, display := exactMoneyText(total), displayMoneyText(total)
		result.TotalValue, result.TotalValueDisplay = &exact, &display
	}
	return result, nil
}

// stockFilters agrega los filtros de categoría, búsqueda, estado del insumo y situación.
func (handler *service) stockFilters(set *filters, levels stockLevels) error {
	query := handler.request.Query
	if category := query.Get("category_id"); category == "none" {
		set.clauses = append(set.clauses, "i.category_id IS NULL")
	} else {
		categoryID, err := optionalQueryID(query, "category_id", "categoría")
		if err != nil {
			return err
		}
		if categoryID != nil {
			set.add("i.category_id=$%d", *categoryID)
		}
	}
	pattern, err := searchPattern(query)
	if err != nil {
		return err
	}
	if pattern != "" {
		set.add("(i.name ILIKE $%[1]d OR i.code ILIKE $%[1]d OR i.barcode ILIKE $%[1]d)", pattern)
	}
	active, err := optionalBool(query, "active", "estado")
	if err != nil {
		return err
	}
	if active != nil {
		set.add("i.active=$%d", *active)
	}
	if state := query.Get("state"); state != "" {
		condition, known := levels.stockStates()[state]
		if !known {
			return invalid("El filtro de situación de la existencia no es válido.")
		}
		set.clauses = append(set.clauses, condition)
	}
	return nil
}

func (handler *service) scanStockRow(rows interface{ Scan(...any) error }) (*StockRow, error) {
	row := &StockRow{Warehouses: []StockByWarehouse{}}
	var quantity, total, value, pending, levelQuantity string
	var minimumStock, reorder, last *string
	err := rows.Scan(&row.ItemID, &row.ItemCode, &row.ItemName, &row.CategoryName, &row.Unit.ID, &row.Unit.Code, &row.Unit.Name,
		&row.Unit.Dimension, &row.Active, &quantity, &total, &minimumStock, &reorder, &value, &last, &pending, &row.UpdatedAt,
		&levelQuantity, &row.LevelScope)
	if err != nil {
		return nil, err
	}
	scoped := parseDatabaseDecimal(quantity)
	totalQuantity := parseDatabaseDecimal(total)
	row.Quantity = quantityText(scoped)
	row.TotalQuantity = quantityText(totalQuantity)
	row.Negative = scoped.IsNegative()
	minimumValue := parseDatabaseOptional(minimumStock)
	reorderValue := parseDatabaseOptional(reorder)
	row.MinStock = optionalQuantityText(minimumValue)
	row.ReorderPoint = optionalQuantityText(reorderValue)
	compared := parseDatabaseDecimal(levelQuantity)
	row.BelowMinimum = minimumValue != nil && compared.LessThan(*minimumValue)
	row.AtReorderPoint = reorderValue != nil && compared.LessThanOrEqual(*reorderValue)
	if handler.costsVisible() {
		totalValue := parseDatabaseDecimal(value)
		average := averageCost(totalQuantity, totalValue, parseDatabaseOptional(last))
		if average != nil {
			exact, display := exactMoneyText(*average), displayUnitCostText(*average)
			row.AverageCost, row.AverageCostDisplay = &exact, &display
		}
		exactValue, displayValue := exactMoneyText(totalValue), displayMoneyText(totalValue)
		row.Value, row.ValueDisplay = &exactValue, &displayValue
		pendingCost := parseDatabaseDecimal(pending).IsPositive()
		row.CostPending = &pendingCost
	}
	return row, nil
}

// averageCost es el costo vigente del insumo: V/Q con existencia positiva y, si no, el último
// costo válido (nil si nunca hubo).
func averageCost(quantity, value decimal.Decimal, last *decimal.Decimal) *decimal.Decimal {
	if quantity.IsPositive() && value.IsPositive() {
		average := value.DivRound(quantity, costScale)
		return &average
	}
	return last
}

// attachWarehouseBreakdown agrega a cada fila sus almacenes con existencia distinta de cero.
func (handler *service) attachWarehouseBreakdown(rows map[string]*StockRow) error {
	if len(rows) == 0 {
		return nil
	}
	identifiers := make([]string, 0, len(rows))
	for id := range rows {
		identifiers = append(identifiers, id)
	}
	result, err := handler.tx.Query(handler.ctx, `SELECT s.item_id,w.id,w.name,s.quantity::text
        FROM app.inventory_stock s JOIN app.inventory_warehouses w ON w.id=s.warehouse_id
        WHERE s.item_id = ANY($1::uuid[]) AND s.quantity<>0
        ORDER BY w.is_default DESC, w.name, w.id`, identifiers)
	if err != nil {
		return err
	}
	defer result.Close()
	for result.Next() {
		var itemID, quantity string
		var entry StockByWarehouse
		if err := result.Scan(&itemID, &entry.WarehouseID, &entry.WarehouseName, &quantity); err != nil {
			return err
		}
		value := parseDatabaseDecimal(quantity)
		entry.Quantity = quantityText(value)
		entry.Negative = value.IsNegative()
		if row := rows[itemID]; row != nil {
			row.Warehouses = append(row.Warehouses, entry)
		}
	}
	return result.Err()
}

// totalInventoryValue es la valuación total del inventario: Σ valores de todos los insumos.
func (handler *service) totalInventoryValue() (decimal.Decimal, error) {
	var total string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT coalesce(sum(value),0)::text FROM app.inventory_valuations").Scan(&total); err != nil {
		return zero, err
	}
	return parseDatabaseDecimal(total), nil
}
