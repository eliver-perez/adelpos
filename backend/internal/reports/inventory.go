package reports

import (
	"context"
	"strconv"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/inventory"
)

// Reportes de inventarios (§13, fase B2). Fuentes: las entidades de la etapa 8 (insumos, existencias
// por almacén, valuación por insumo, libro inmutable de movimientos, ajustes y mermas, transferencias,
// conteos, clasificación de cancelaciones y solicitudes de consumo), leídas por SQL en la transacción
// de solo lectura. Permisos como en la etapa 8 (además de reports.inventory.read):
//   - existencias (cantidades): inventory.stock.read; libro de movimientos: inventory.movements.read;
//   - costos unitarios, valores y «costo pendiente»: inventory.costs.read (sin él tampoco se listan los
//     ajustes de costo, que son solo valor);
//   - valor de un insumo: inventory.costs.read e inventory.valuation.read; valor total del
//     inventario: inventory.valuation.read.
//
// Inventario teórico: las existencias son las del libro (lo que debería haber); los conteos físicos
// y sus diferencias se reportan aparte. El consumo es el de la liberación canónica a cocina (una vez
// por unidad comercial, aunque se envíe a varias estaciones). Una devolución financiera nunca es una
// merma ni reintegra ingredientes. Si hay solicitudes de consumo pendientes o con error en el
// periodo, el reporte lo avisa: los datos pueden estar incompletos.

const (
	permissionInventoryMovements = "inventory.movements.read"
	inventoryMovementsDateBasis  = "Hora de negocio de cada movimiento del libro (liberación a cocina para el consumo, " +
		"aplicación para ajustes, captura para conteos), con la hora de inicio del día."
	inventoryCurrentDateBasis = "Situación actual del libro de inventario (sin periodo)."
)

// Escalas de presentación: los costos unitarios por unidad base se muestran con cuatro decimales
// (como la etapa 8) y las cantidades conservan sus seis decimales.
const unitCostDisplayScale = 4

// inventoryCostColumn es un costo unitario por unidad base (cantidad decimal, no importe en pesos con
// dos decimales, para no perder precisión en costos por gramo o mililitro).
func inventoryCostColumn(key, label, description string) Column {
	return requiring(described(Column{Key: key, Label: label, Type: TypeQuantity, Sortable: true}, description), PermissionCostsRead)
}

func quantityColumn(key, label string) Column {
	return Column{Key: key, Label: label, Type: TypeQuantity, Sortable: true}
}

func costsVisible(query *Query) bool {
	return query.Actor.Can(PermissionCostsRead)
}

func valuationVisible(query *Query) bool {
	return query.Actor.Can(PermissionValuationRead)
}

// averageCost replica la regla de la etapa 8 (inventory/stock.go): V/Q con existencia y valor
// positivos; si no, el último costo válido (nil si nunca hubo).
func averageCost(quantity, value decimal.Decimal, last any) any {
	if quantity.IsPositive() && value.IsPositive() {
		return value.DivRound(quantity, 10).Round(unitCostDisplayScale)
	}
	if typed, ok := last.(decimal.Decimal); ok {
		return typed.Round(unitCostDisplayScale)
	}
	return nil
}

// consumptionWarning avisa de las solicitudes de consumo pendientes o con error (en el periodo, o
// todas si el reporte no tiene periodo). Nunca dice importes.
func consumptionWarning(ctx context.Context, query *Query) (*Warning, error) {
	condition := "true"
	if query.Params.Period != nil {
		condition = "q.released_at < @until AND q.occurred_at >= @from"
	}
	var pending, failed int
	err := query.Tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE q.state='pending'),count(*) FILTER (WHERE q.state='failed')
        FROM app.inventory_consumption_requests q WHERE q.state <> 'processed' AND `+condition, query.Args()).Scan(&pending, &failed)
	if err != nil || pending+failed == 0 {
		return nil, err
	}
	message := "Los datos pueden estar incompletos: hay " + strconv.Itoa(pending) + " solicitudes de consumo pendientes de procesar"
	if failed > 0 {
		message += " y " + strconv.Itoa(failed) + " con error que se reintentan"
	}
	return &Warning{Code: "CONSUMPTION_PENDING", Message: message + "."}, nil
}

// inventoryDataset agrega el aviso de consumo pendiente.
func inventoryDataset(ctx context.Context, query *Query, rows []Row) (*Dataset, error) {
	dataset := &Dataset{Rows: rows}
	warning, err := consumptionWarning(ctx, query)
	if err != nil {
		return nil, err
	}
	if warning != nil {
		dataset.Warnings = append(dataset.Warnings, *warning)
	}
	return dataset, nil
}

// stockStatus clasifica la existencia contra sus niveles.
func stockStatus(quantity decimal.Decimal, minimum, reorder any) string {
	switch {
	case quantity.IsNegative():
		return "Negativa"
	case minimum != nil && quantity.LessThan(minimum.(decimal.Decimal)):
		return "Bajo mínimo"
	case reorder != nil && quantity.LessThanOrEqual(reorder.(decimal.Decimal)):
		return "En punto de reabastecimiento"
	case quantity.IsZero():
		return "Sin existencia"
	}
	return "Suficiente"
}

var stockStates = []Option{
	{Value: "negative", Label: "Negativa"},
	{Value: "below_minimum", Label: "Bajo mínimo"},
	{Value: "reorder", Label: "En punto de reabastecimiento"},
	{Value: "zero", Label: "Sin existencia"},
}

// stockStateCondition traduce el filtro de situación sobre una cantidad y sus niveles.
func stockStateCondition(state, quantity, minimum, reorder string) string {
	switch state {
	case "negative":
		return quantity + " < 0"
	case "below_minimum":
		return minimum + " IS NOT NULL AND " + quantity + " < " + minimum
	case "reorder":
		return reorder + " IS NOT NULL AND " + quantity + " <= " + reorder
	case "zero":
		return quantity + " = 0"
	}
	return "true"
}

func movementKindOptions(kinds ...string) []Option {
	options := []Option{}
	for _, kind := range kinds {
		options = append(options, Option{Value: kind, Label: inventory.MovementKindLabel(kind)})
	}
	return options
}

var (
	entryKinds = []string{inventory.KindInitial, inventory.KindReceipt, inventory.KindConsumptionReversal, inventory.KindAdjustmentIn,
		inventory.KindTransferIn, inventory.KindProductionIn, inventory.KindCountIn}
	outflowKinds = []string{inventory.KindConsumption, inventory.KindWaste, inventory.KindAdjustmentOut, inventory.KindTransferOut,
		inventory.KindSupplierReturn, inventory.KindProductionOut, inventory.KindCountOut}
)

func sqlList(values []string) string {
	text := ""
	for index, value := range values {
		if index > 0 {
			text += ","
		}
		text += "'" + value + "'"
	}
	return text
}

// Columnas comunes de insumo.
var (
	columnItemCode     = codeColumn("code", "Código")
	columnItemName     = textColumn("item", "Insumo")
	columnItemCategory = textColumn("category", "Categoría")
	columnItemUnit     = textColumn("unit", "Unidad")
	columnWarehouse    = textColumn("warehouse", "Almacén")
)

const itemJoins = `JOIN app.inventory_items i ON i.id=x.item_id
        JOIN app.inventory_units unit ON unit.id=i.base_unit_id
        LEFT JOIN app.inventory_categories category ON category.id=i.category_id`

const itemSelect = `i.code,i.name AS item,coalesce(category.name,'Sin categoría') AS category,unit.code AS unit`

func init() {
	registerStockReports()
	registerMovementReports()
	registerControlReports()
	registerConsumptionReports()
}

func registerStockReports() {
	register(Definition{
		ID: "inventory.stock", Group: "inventory", Title: "Existencias actuales",
		Description: "Existencia teórica total de cada insumo con sus niveles y, con permiso, costo promedio y valor.",
		Permissions: []string{PermissionInventory, PermissionStockRead}, Period: PeriodNone, DateBasis: inventoryCurrentDateBasis,
		Filters: []Filter{filterInventoryCategory(), filterItem(), filterChoice("state", "Situación", "", stockStates...)},
		Columns: []Column{columnItemCode, columnItemName, columnItemCategory, columnItemUnit,
			described(quantityColumn("quantity", "Existencia"), "Existencia teórica total (todos los almacenes), en la unidad base."),
			optional(quantityColumn("min_stock", "Mínimo")), optional(quantityColumn("reorder_point", "Punto de reabastecimiento")),
			textColumn("status", "Situación"),
			inventoryCostColumn("average_cost", "Costo promedio", "Costo vigente por unidad base (promedio o último costo válido)."),
			requiring(described(moneyColumn("value", "Valor"), "Valuación del insumo (todos sus almacenes)."), PermissionCostsRead,
				PermissionValuationRead),
			requiring(textColumn("cost_pending", "Costo pendiente"), PermissionCostsRead)},
		DefaultSort: Sort{Key: "item"},
		Query:       stockQuery,
	})
	register(Definition{
		ID: "inventory.stock_by_warehouse", Group: "inventory", Title: "Existencias por almacén",
		Description: "Existencia teórica de cada insumo en cada almacén con los niveles del almacén (o los del insumo).",
		Permissions: []string{PermissionInventory, PermissionStockRead}, Period: PeriodNone, DateBasis: inventoryCurrentDateBasis,
		Filters: []Filter{filterWarehouse(), filterInventoryCategory(), filterItem(),
			filterChoice("state", "Situación", "", stockStates...)},
		Columns: []Column{columnWarehouse, columnItemCode, columnItemName, columnItemCategory, columnItemUnit,
			described(quantityColumn("quantity", "Existencia"), "Existencia teórica en el almacén."),
			optional(quantityColumn("min_stock", "Mínimo")), optional(quantityColumn("reorder_point", "Punto de reabastecimiento")),
			optional(textColumn("level_scope", "Niveles")), textColumn("status", "Situación"),
			optional(dateTimeColumn("updated_at", "Último movimiento"))},
		DefaultSort: Sort{Key: "warehouse"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			return warehouseStockQuery(ctx, query, "")
		},
	})
	register(Definition{
		ID: "inventory.negative", Group: "inventory", Title: "Insumos con existencia negativa",
		Description: "Insumos y almacenes con existencia teórica negativa (la venta nunca se bloquea por ello).",
		Permissions: []string{PermissionInventory, PermissionStockRead}, Period: PeriodNone, DateBasis: inventoryCurrentDateBasis,
		Filters: []Filter{filterWarehouse(), filterInventoryCategory()},
		Columns: []Column{columnWarehouse, columnItemCode, columnItemName, columnItemCategory, columnItemUnit,
			described(quantityColumn("quantity", "Existencia"), "Existencia teórica negativa en el almacén."),
			optional(dateTimeColumn("updated_at", "Último movimiento")),
			requiring(textColumn("cost_pending", "Costo pendiente"), PermissionCostsRead)},
		DefaultSort: Sort{Key: "quantity"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			return warehouseStockQuery(ctx, query, "x.quantity < 0")
		},
	})
	register(Definition{
		ID: "inventory.below_minimum", Group: "inventory", Title: "Insumos bajo mínimo",
		Description: "Insumos cuya existencia total está bajo su mínimo y almacenes bajo el mínimo de su nivel.",
		Permissions: []string{PermissionInventory, PermissionStockRead}, Period: PeriodNone, DateBasis: inventoryCurrentDateBasis,
		Filters: []Filter{filterWarehouse(), filterInventoryCategory()},
		Columns: []Column{textColumn("scope", "Alcance"), columnItemCode, columnItemName, columnItemCategory, columnItemUnit,
			quantityColumn("quantity", "Existencia"), quantityColumn("min_stock", "Mínimo"),
			optional(quantityColumn("reorder_point", "Punto de reabastecimiento")),
			described(quantityColumn("missing", "Faltante para el mínimo"), "Mínimo − existencia.")},
		DefaultSort: Sort{Key: "item"},
		Query:       belowMinimumQuery,
	})
	register(Definition{
		ID: "inventory.valuation", Group: "inventory", Title: "Valuación del inventario",
		Description: "Valor del inventario por insumo con su costo promedio, y valor total.",
		Permissions: []string{PermissionInventory, PermissionValuationRead}, Period: PeriodNone, DateBasis: inventoryCurrentDateBasis,
		Filters: []Filter{filterInventoryCategory()},
		Columns: []Column{columnItemCode, columnItemName, columnItemCategory, columnItemUnit,
			requiring(quantityColumn("quantity", "Existencia"), PermissionStockRead),
			inventoryCostColumn("average_cost", "Costo promedio", "Costo vigente por unidad base."),
			requiring(described(moneyColumn("value", "Valor"), "Valuación del insumo (todos sus almacenes)."), PermissionCostsRead),
			requiring(textColumn("cost_pending", "Costo pendiente"), PermissionCostsRead)},
		DefaultSort: Sort{Key: "item"},
		Query:       valuationQuery,
	})
	register(Definition{
		ID: "inventory.average_costs", Group: "inventory", Title: "Costos promedio",
		Description: "Costo promedio vigente, último costo válido y costo de referencia de cada insumo.",
		Permissions: []string{PermissionInventory, PermissionCostsRead}, Period: PeriodNone, DateBasis: inventoryCurrentDateBasis,
		Filters: []Filter{filterInventoryCategory(), filterItem()},
		Columns: []Column{columnItemCode, columnItemName, columnItemCategory, columnItemUnit,
			requiring(quantityColumn("quantity", "Existencia"), PermissionStockRead),
			inventoryCostColumn("average_cost", "Costo promedio", "Costo vigente por unidad base (promedio o último costo válido)."),
			inventoryCostColumn("last_valid_cost", "Último costo válido", "Último promedio con existencia positiva."),
			inventoryCostColumn("reference_cost", "Costo de referencia", "Costo configurado en el insumo."),
			requiring(described(quantityColumn("pending_cost_quantity", "Cantidad con costo pendiente"),
				"Parte del déficit valorada en cero por falta de costo."), PermissionCostsRead)},
		DefaultSort: Sort{Key: "item"},
		Query:       averageCostsQuery,
	})
}

// itemConditions son los filtros comunes de insumo (alias i).
func itemConditions(query *Query, list *conditions) {
	list.when(query, "category_id", "i.category_id = @category_id::uuid")
	list.when(query, "item_id", "i.id = @item_id::uuid")
}

func stockQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{"(i.active OR coalesce(v.quantity,0) <> 0)"}
	itemConditions(query, &list)
	if query.Has("state") {
		list.add(stockStateCondition(query.Params.Value("state"), "coalesce(v.quantity,0)", "i.min_stock", "i.reorder_point"))
	}
	costs := ""
	if costsVisible(query) {
		costs = ",v.value AS item_value,v.last_valid_cost,(coalesce(v.pending_cost_quantity,0) > 0) AS pending"
	}
	rows, err := query.collect(ctx, `SELECT `+itemSelect+`,coalesce(v.quantity,0) AS quantity,i.min_stock,i.reorder_point`+costs+`
        FROM app.inventory_items i JOIN app.inventory_units unit ON unit.id=i.base_unit_id
        LEFT JOIN app.inventory_categories category ON category.id=i.category_id
        LEFT JOIN app.inventory_valuations v ON v.item_id=i.id
        WHERE true`+list.sql()+` ORDER BY i.name,i.id`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		quantity := decimalOf(row, "quantity")
		row["status"] = stockStatus(quantity, row["min_stock"], row["reorder_point"])
		completeItemCosts(query, row, quantity)
	}
	return inventoryDataset(ctx, query, rows)
}

// completeItemCosts agrega costo promedio, valor y costo pendiente según los permisos.
func completeItemCosts(query *Query, row Row, quantity decimal.Decimal) {
	if !costsVisible(query) {
		return
	}
	value := decimalOf(row, "item_value")
	row["average_cost"] = averageCost(quantity, value, row["last_valid_cost"])
	if valuationVisible(query) {
		row["value"] = value
	}
	row["cost_pending"] = ""
	if pending, _ := row["pending"].(bool); pending {
		row["cost_pending"] = "Sí"
	}
}

func warehouseStockQuery(ctx context.Context, query *Query, extra string) (*Dataset, error) {
	list := conditions{"(x.quantity <> 0 OR sl.item_id IS NOT NULL)"}
	if extra != "" {
		list.add(extra)
	}
	itemConditions(query, &list)
	list.when(query, "warehouse_id", "x.warehouse_id = @warehouse_id::uuid")
	minimum := "CASE WHEN sl.item_id IS NULL THEN i.min_stock ELSE sl.min_stock END"
	reorder := "CASE WHEN sl.item_id IS NULL THEN i.reorder_point ELSE sl.reorder_point END"
	if query.Has("state") {
		list.add(stockStateCondition(query.Params.Value("state"), "x.quantity", "("+minimum+")", "("+reorder+")"))
	}
	pending := ""
	if costsVisible(query) {
		pending = ",(coalesce(v.pending_cost_quantity,0) > 0) AS pending"
	}
	rows, err := query.collect(ctx, `SELECT w.name AS warehouse,`+itemSelect+`,x.quantity,`+minimum+` AS min_stock,
            `+reorder+` AS reorder_point,CASE WHEN sl.item_id IS NULL THEN 'Del insumo' ELSE 'Del almacén' END AS level_scope,
            x.updated_at`+pending+`
        FROM app.inventory_stock x
        JOIN app.inventory_warehouses w ON w.id=x.warehouse_id
        `+itemJoins+`
        LEFT JOIN app.inventory_stock_levels sl ON sl.item_id=x.item_id AND sl.warehouse_id=x.warehouse_id
        LEFT JOIN app.inventory_valuations v ON v.item_id=x.item_id
        WHERE true`+list.sql()+` ORDER BY w.is_default DESC,w.name,i.name`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["status"] = stockStatus(decimalOf(row, "quantity"), row["min_stock"], row["reorder_point"])
		if costsVisible(query) {
			row["cost_pending"] = ""
			if flag, _ := row["pending"].(bool); flag {
				row["cost_pending"] = "Sí"
			}
		}
	}
	return inventoryDataset(ctx, query, rows)
}

func belowMinimumQuery(ctx context.Context, query *Query) (*Dataset, error) {
	itemFilter := conditions{}
	itemConditions(query, &itemFilter)
	totals := ""
	if !query.Has("warehouse_id") {
		totals = `SELECT 'Existencia total' AS scope,` + itemSelect + `,coalesce(v.quantity,0) AS quantity,i.min_stock,i.reorder_point,
                i.min_stock-coalesce(v.quantity,0) AS missing
            FROM app.inventory_items i JOIN app.inventory_units unit ON unit.id=i.base_unit_id
            LEFT JOIN app.inventory_categories category ON category.id=i.category_id
            LEFT JOIN app.inventory_valuations v ON v.item_id=i.id
            WHERE i.active AND i.min_stock IS NOT NULL AND coalesce(v.quantity,0) < i.min_stock` + itemFilter.sql() + `
            UNION ALL `
	}
	warehouse := conditions{}
	itemConditions(query, &warehouse)
	warehouse.when(query, "warehouse_id", "sl.warehouse_id = @warehouse_id::uuid")
	rows, err := query.collect(ctx, totals+`SELECT w.name AS scope,`+itemSelect+`,coalesce(x.quantity,0) AS quantity,sl.min_stock,
            sl.reorder_point,sl.min_stock-coalesce(x.quantity,0) AS missing
        FROM app.inventory_stock_levels sl
        JOIN app.inventory_warehouses w ON w.id=sl.warehouse_id
        JOIN app.inventory_items i ON i.id=sl.item_id
        JOIN app.inventory_units unit ON unit.id=i.base_unit_id
        LEFT JOIN app.inventory_categories category ON category.id=i.category_id
        LEFT JOIN app.inventory_stock x ON x.item_id=sl.item_id AND x.warehouse_id=sl.warehouse_id
        WHERE i.active AND sl.min_stock IS NOT NULL AND coalesce(x.quantity,0) < sl.min_stock`+warehouse.sql(), query.Args())
	if err != nil {
		return nil, err
	}
	return inventoryDataset(ctx, query, rows)
}

func valuationQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{"(i.active OR coalesce(v.quantity,0) <> 0 OR coalesce(v.value,0) <> 0)"}
	itemConditions(query, &list)
	costs := ""
	if costsVisible(query) {
		costs = ",v.value AS item_value,v.last_valid_cost,(coalesce(v.pending_cost_quantity,0) > 0) AS pending"
	}
	rows, err := query.collect(ctx, `SELECT `+itemSelect+`,coalesce(v.quantity,0) AS quantity`+costs+`
        FROM app.inventory_items i JOIN app.inventory_units unit ON unit.id=i.base_unit_id
        LEFT JOIN app.inventory_categories category ON category.id=i.category_id
        LEFT JOIN app.inventory_valuations v ON v.item_id=i.id
        WHERE true`+list.sql()+` ORDER BY i.name,i.id`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		completeItemCosts(query, row, decimalOf(row, "quantity"))
	}
	dataset, err := inventoryDataset(ctx, query, rows)
	if err != nil {
		return nil, err
	}
	// El valor total del inventario (todos los insumos) solo exige inventory.valuation.read, como en la
	// etapa 8; es independiente de los filtros.
	var total string
	if err := query.Tx.QueryRow(ctx, "SELECT coalesce(sum(value),0)::text FROM app.inventory_valuations").Scan(&total); err != nil {
		return nil, err
	}
	dataset.Summary = append(dataset.Summary, Figure{Key: "total_value", Label: "Valor total del inventario", Type: TypeMoneyMXN,
		Value: decimal.RequireFromString(total), Permissions: []string{PermissionValuationRead},
		Hint: "Todos los insumos, sin filtros; un solo costo promedio por insumo (no por almacén)."})
	return dataset, nil
}

func averageCostsQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{"(i.active OR coalesce(v.quantity,0) <> 0)"}
	itemConditions(query, &list)
	rows, err := query.collect(ctx, `SELECT `+itemSelect+`,coalesce(v.quantity,0) AS quantity,v.value AS item_value,
            v.last_valid_cost AS last_cost,i.reference_cost AS reference,coalesce(v.pending_cost_quantity,0) AS pending_cost_quantity
        FROM app.inventory_items i JOIN app.inventory_units unit ON unit.id=i.base_unit_id
        LEFT JOIN app.inventory_categories category ON category.id=i.category_id
        LEFT JOIN app.inventory_valuations v ON v.item_id=i.id
        WHERE true`+list.sql()+` ORDER BY i.name,i.id`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["average_cost"] = averageCost(decimalOf(row, "quantity"), decimalOf(row, "item_value"), row["last_cost"])
		if last, ok := row["last_cost"].(decimal.Decimal); ok {
			row["last_valid_cost"] = last.Round(unitCostDisplayScale)
		}
		if reference, ok := row["reference"].(decimal.Decimal); ok {
			row["reference_cost"] = reference.Round(unitCostDisplayScale)
		}
	}
	return inventoryDataset(ctx, query, rows)
}
