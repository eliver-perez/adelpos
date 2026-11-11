package reports

import (
	"context"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/inventory"
)

// Compras por periodo (§15, obligatorio) y proveedores (fase B2). Fuentes: órdenes de compra y sus
// renglones, recepciones (de orden, directas y de mostrador) y sus renglones, devoluciones a
// proveedor y vínculos con gastos de caja de la etapa 8 y la 9A (proveedor kind='counter'). Reglas:
//   - una orden emitida no es mercancía recibida: «ordenado» son renglones de órdenes emitidas (sin
//     borradores; las canceladas solo con su filtro), «recibido» son renglones de recepciones y
//     «pendiente» lo que falta recibir de órdenes emitidas o parcialmente recibidas;
//   - cada recepción se suma una sola vez (sus renglones); el gasto de caja vinculado nunca suma como
//     compra: se informa como «pagado desde caja»; las devoluciones a proveedor se reportan aparte, a
//     su valor asociado (costo original), sin restarlas de lo comprado;
//   - los importes de compra (precio sin IVA, descuento, IVA y total) los ve quien consulta compras;
//     el valor de inventario y el costo por unidad base de una recepción exigen inventory.costs.read;
//   - fechas: la recepción con su hora (hora de inicio del día); la orden con su fecha de orden
//     (fecha capturada, sin hora).

const (
	receiptDateBasis  = "Hora de la recepción de la mercancía, con la hora de inicio del día."
	orderDateBasis    = "Fecha de la orden de compra (fecha capturada en la orden, sin hora)."
	cashMovementsRead = "cash.movements.read"
)

var purchaseModalities = []Option{
	{Value: "order", Label: "Orden de compra"},
	{Value: "direct", Label: "Recepción directa"},
	{Value: "counter", Label: "Compras de mostrador"},
}

func orderStateOptions(states ...string) []Option {
	options := []Option{}
	for _, state := range states {
		options = append(options, Option{Value: state, Label: inventory.PurchaseOrderStateLabel(state)})
	}
	return options
}

// modalitySQL clasifica una recepción r de proveedor supplier.
const modalitySQL = `CASE WHEN supplier.kind='counter' THEN 'counter' WHEN r.purchase_order_id IS NULL THEN 'direct' ELSE 'order' END`

func modalityLabel(code string) string {
	for _, option := range purchaseModalities {
		if option.Value == code {
			return option.Label
		}
	}
	return code
}

// receiptConditions son los filtros de los renglones l de recepciones r.
func receiptConditions(query *Query) conditions {
	list := conditions{inPeriod("r.occurred_at")}
	list.when(query, "supplier_id", "r.supplier_id = @supplier_id::uuid")
	list.when(query, "item_id", "l.item_id = @item_id::uuid")
	list.when(query, "category_id", "i.category_id = @category_id::uuid")
	list.when(query, "warehouse_id", "r.warehouse_id = @warehouse_id::uuid")
	list.when(query, "modality", modalitySQL+" = @modality")
	list.when(query, "state", "o.state = @state")
	return list
}

func purchaseFilters() []Filter {
	return []Filter{filterSupplier(), filterItem(), filterInventoryCategory(), filterWarehouse(),
		filterChoice("modality", "Modalidad", "", purchaseModalities...),
		filterChoice("state", "Estado de la orden", "", orderStateOptions("issued", "partially_received", "received", "closed", "cancelled")...)}
}

const receiptLineJoins = `JOIN app.inventory_receipts r ON r.id=l.receipt_id
        JOIN app.inventory_suppliers supplier ON supplier.id=r.supplier_id
        JOIN app.inventory_warehouses w ON w.id=r.warehouse_id
        JOIN app.inventory_items i ON i.id=l.item_id
        JOIN app.inventory_units unit ON unit.id=i.base_unit_id
        LEFT JOIN app.inventory_categories category ON category.id=i.category_id
        LEFT JOIN app.inventory_presentations presentation ON presentation.id=l.presentation_id
        LEFT JOIN app.inventory_units captured ON captured.id=l.unit_id
        LEFT JOIN app.inventory_purchase_orders o ON o.id=r.purchase_order_id`

// Columnas de importes de compra de un renglón.
func purchaseAmountColumns() []Column {
	return []Column{
		described(quantityColumn("unit_price", "Precio unitario"), "Precio sin IVA por unidad o presentación capturada."),
		described(moneyColumn("discount", "Descuento"), "Descuento del renglón (el de la orden, prorrateado)."),
		described(moneyColumn("subtotal", "Importe sin IVA"), "Después del descuento."),
		{Key: "tax_rate", Label: "Tasa de IVA", Type: TypeInteger, Total: TotalNone, Sortable: true},
		described(moneyColumn("tax", "IVA"), "IVA registrado en el renglón."),
		described(moneyColumn("total", "Importe"), "Importe sin IVA + IVA."),
	}
}

func init() {
	permissions := []string{PermissionPurchases}
	register(Definition{
		ID: "purchases.received", Group: "purchases", Title: "Compras recibidas por periodo",
		Description: "Mercancía recibida en el periodo, renglón por renglón, con proveedor, modalidad, cantidades, importes e IVA.",
		Permissions: permissions, DateBasis: receiptDateBasis, Filters: purchaseFilters(),
		Columns: withColumns([]Column{dateTimeColumn("occurred_at", "Fecha"), codeColumn("receipt_folio", "Recepción"),
			codeColumn("order_folio", "Orden"), textColumn("modality", "Modalidad"), textColumn("supplier", "Proveedor"),
			optional(textColumn("counter_store", "Comercio")), columnWarehouse, columnItemCode, columnItemName,
			optional(columnItemCategory), quantityColumn("quantity", "Cantidad"), textColumn("capture_unit", "Unidad"),
			optional(quantityColumn("base_quantity", "Cantidad base")), optional(textColumn("base_unit", "Unidad base"))},
			purchaseAmountColumns(), []Column{textColumn("received_by", "Recibió"), optional(textColumn("supplier_reference", "Referencia")),
				requiring(optional(described(moneyColumn("inventory_value", "Valor de inventario"),
					"Valor con el que entró al inventario (con o sin IVA según la configuración copiada).")), PermissionCostsRead),
				inventoryCostColumn("unit_cost", "Costo por unidad base", "Costo de inventario por unidad base.")}),
		DefaultSort: Sort{Key: "occurred_at"},
		Query:       receivedQuery,
	})
	register(Definition{
		ID: "purchases.ordered", Group: "purchases", Title: "Compras ordenadas",
		Description: "Renglones de las órdenes emitidas en el periodo con lo recibido y lo pendiente de recibir.",
		Permissions: permissions, DateBasis: orderDateBasis,
		Filters: []Filter{filterSupplier(), filterItem(), filterInventoryCategory(), filterWarehouse(),
			filterChoice("state", "Estado de la orden", "", orderStateOptions("issued", "partially_received", "received", "closed", "cancelled")...)},
		Columns: withColumns([]Column{dateColumn("order_date", "Fecha"), codeColumn("folio", "Orden"), textColumn("state", "Estado"),
			textColumn("supplier", "Proveedor"), columnWarehouse, columnItemCode, columnItemName, optional(columnItemCategory),
			quantityColumn("quantity", "Cantidad"), textColumn("capture_unit", "Unidad")}, purchaseAmountColumns(),
			[]Column{quantityColumn("base_quantity", "Ordenado (unidad base)"), quantityColumn("received_quantity", "Recibido (unidad base)"),
				described(quantityColumn("pending_quantity", "Pendiente (unidad base)"), "Solo de órdenes emitidas o parcialmente recibidas."),
				textColumn("base_unit", "Unidad base"), countColumn("receipts", "Recepciones"),
				described(moneyColumn("received_total", "Importe recibido"), "Importe de los renglones recibidos de esta orden.")}),
		DefaultSort: Sort{Key: "order_date"},
		Query:       orderedQuery,
	})
	register(Definition{
		ID: "purchases.pending", Group: "purchases", Title: "Pendiente de recepción",
		Description: "Lo que falta recibir de las órdenes emitidas o parcialmente recibidas (situación actual).",
		Permissions: permissions, Period: PeriodNone, DateBasis: "Situación actual de las órdenes (sin periodo).",
		Filters: []Filter{filterSupplier(), filterItem(), filterWarehouse()},
		Columns: []Column{dateColumn("order_date", "Fecha de la orden"), codeColumn("folio", "Orden"), textColumn("state", "Estado"),
			textColumn("supplier", "Proveedor"), columnWarehouse, columnItemCode, columnItemName,
			quantityColumn("base_quantity", "Ordenado"), quantityColumn("received_quantity", "Recibido"),
			quantityColumn("pending_quantity", "Pendiente"), textColumn("base_unit", "Unidad base"),
			described(moneyColumn("pending_total", "Importe pendiente estimado"),
				"Importe del renglón de la orden en proporción a lo pendiente (con IVA); referencia, no un adeudo.")},
		DefaultSort: Sort{Key: "order_date"},
		Query:       pendingQuery,
	})
	register(Definition{
		ID: "purchases.by_supplier", Group: "purchases", Title: "Compras por proveedor",
		Description: "Por proveedor: recibido (subtotal, descuento, IVA y total), ordenado, devoluciones y lo pagado desde caja.",
		Permissions: permissions, DateBasis: receiptDateBasis + " Lo ordenado, con la fecha de la orden.",
		Filters: []Filter{filterSupplier(), filterChoice("modality", "Modalidad", "", purchaseModalities...), filterWarehouse()},
		Columns: []Column{textColumn("supplier", "Proveedor"), textColumn("kind", "Tipo"), countColumn("receipts", "Recepciones"),
			described(moneyColumn("subtotal", "Recibido sin IVA"), "Después de descuentos."), moneyColumn("discount", "Descuentos"),
			moneyColumn("tax", "IVA"), described(moneyColumn("total", "Total recibido"), "Total de las recepciones del periodo."),
			described(moneyColumn("ordered_total", "Ordenado"), "Total de las órdenes emitidas con fecha en el periodo (no es mercancía recibida)."),
			described(moneyColumn("returned_value", "Devoluciones a proveedor"), "Valor asociado (costo original) de las devoluciones del periodo; no se resta."),
			described(moneyColumn("cash_linked", "Pagado desde caja"), "Vínculos vigentes con gastos de caja de las recepciones; nunca es otra compra.")},
		DefaultSort: Sort{Key: "total", Descending: true},
		Query:       bySupplierQuery,
	})
	register(Definition{
		ID: "purchases.by_item", Group: "purchases", Title: "Compras por insumo",
		Description: "Cantidad recibida e importes de compra de cada insumo en el periodo.",
		Permissions: permissions, DateBasis: receiptDateBasis, Filters: purchaseFilters(),
		Columns: []Column{columnItemCode, columnItemName, columnItemCategory, quantityColumn("base_quantity", "Cantidad recibida"),
			textColumn("base_unit", "Unidad base"), countColumn("receipts", "Recepciones"), moneyColumn("discount", "Descuento"),
			described(moneyColumn("subtotal", "Importe sin IVA"), "Después de descuentos."), moneyColumn("tax", "IVA"), moneyColumn("total", "Importe"),
			described(quantityColumn("average_price", "Precio de compra promedio"),
				"Importe sin IVA ÷ cantidad base: precio de compra, no costo de inventario ni precio de venta.")},
		DefaultSort: Sort{Key: "total", Descending: true},
		Query:       byItemQuery,
	})
	register(Definition{
		ID: "purchases.supplier_returns", Group: "purchases", Title: "Devoluciones a proveedor",
		Description: "Devoluciones a proveedor del periodo a su valor asociado (costo original), aparte de las compras.",
		Permissions: permissions, DateBasis: "Hora de la devolución, con la hora de inicio del día.",
		Filters: []Filter{filterSupplier(), filterItem(), filterWarehouse()},
		Columns: []Column{dateTimeColumn("occurred_at", "Fecha"), codeColumn("folio", "Devolución"), codeColumn("receipt_folio", "Recepción"),
			textColumn("supplier", "Proveedor"), columnWarehouse, columnItemCode, columnItemName, quantityColumn("base_quantity", "Cantidad"),
			textColumn("base_unit", "Unidad base"), described(moneyColumn("value", "Valor asociado"), "Cantidad × costo original de la recepción."),
			textColumn("reason", "Motivo"), textColumn("actor", "Registró"),
			requiring(optional(described(moneyColumn("outflow_value", "Valor de salida"), "Valor al costo promedio vigente.")), PermissionCostsRead),
			requiring(optional(moneyColumn("unabsorbed", "Diferencia no absorbida")), PermissionCostsRead)},
		DefaultSort: Sort{Key: "occurred_at"},
		Query:       supplierReturnsQuery,
	})
	register(Definition{
		ID: "purchases.cash_links", Group: "purchases", Title: "Compras pagadas desde caja",
		Description: "Recepciones vinculadas con gastos de caja: el gasto no es otra compra ni otra salida de mercancía.",
		Permissions: permissions, DateBasis: receiptDateBasis,
		Filters: []Filter{filterSupplier(), filterChoice("state", "Vínculo", "active", Option{Value: "active", Label: "Vigentes"},
			Option{Value: "unlinked", Label: "Desvinculados"}, Option{Value: "all", Label: "Todos"})},
		Columns: []Column{dateTimeColumn("occurred_at", "Recepción"), codeColumn("receipt_folio", "Folio"), textColumn("supplier", "Proveedor"),
			described(moneyColumn("linked", "Vinculado"), "Importe vinculado con el gasto (solo vínculos vigentes)."),
			textColumn("status", "Estado del vínculo"), dateTimeColumn("linked_at", "Vinculó"), textColumn("linked_by", "Vinculó (usuario)"),
			requiring(dateTimeColumn("expense_at", "Fecha del gasto"), cashMovementsRead, cashShiftsReadAll, cashHistoryRead),
			requiring(textColumn("register", "Caja"), cashMovementsRead, cashShiftsReadAll, cashHistoryRead),
			requiring(textColumn("concept", "Concepto del gasto"), cashMovementsRead, cashShiftsReadAll, cashHistoryRead)},
		DefaultSort: Sort{Key: "occurred_at"},
		Query:       cashLinksQuery,
	})
	register(Definition{
		ID: "suppliers.activity", Group: "suppliers", Title: "Proveedores",
		Description: "Cada proveedor con sus datos de contacto, compras recibidas en el periodo, última recepción y órdenes abiertas.",
		Permissions: permissions, DateBasis: receiptDateBasis,
		Filters: []Filter{filterChoice("state", "Estado", "", Option{Value: "active", Label: "Activos"},
			Option{Value: "inactive", Label: "Inactivos"})},
		Columns: []Column{textColumn("supplier", "Proveedor"), textColumn("kind", "Tipo"), textColumn("active", "Activo"),
			optional(textColumn("contact", "Contacto")), textColumn("phone", "Teléfono"), optional(textColumn("mobile_phone", "Celular")),
			optional(textColumn("email", "Correo")), countColumn("receipts", "Recepciones"), moneyColumn("total", "Total recibido"),
			optional(dateTimeColumn("last_receipt_at", "Última recepción")), countColumn("open_orders", "Órdenes abiertas")},
		DefaultSort: Sort{Key: "supplier"},
		Query:       supplierActivityQuery,
	})
}

// purchaseFigures resume recepciones, total, lo pagado desde caja y las devoluciones del periodo.
func purchaseFigures(ctx context.Context, query *Query, list conditions) ([]Figure, error) {
	var receipts int64
	var total, linked, returned string
	err := query.Tx.QueryRow(ctx, `WITH lines AS (
            SELECT l.receipt_id,l.total FROM app.inventory_receipt_lines l `+receiptLineJoins+` WHERE true`+list.sql()+`
        ),
        receipts AS (SELECT DISTINCT receipt_id FROM lines)
        SELECT (SELECT count(*) FROM receipts),(SELECT coalesce(sum(total),0) FROM lines)::text,
            (SELECT coalesce(sum(cl.amount),0) FROM app.inventory_receipt_cash_links cl JOIN receipts x ON x.receipt_id=cl.receipt_id
                WHERE cl.unlinked_at IS NULL)::text,
            (SELECT coalesce(sum(sr.value),0) FROM app.inventory_supplier_returns sr WHERE `+inPeriod("sr.occurred_at")+`)::text`,
		query.Args()).Scan(&receipts, &total, &linked, &returned)
	if err != nil {
		return nil, err
	}
	return []Figure{
		{Key: "receipts", Label: "Recepciones", Type: TypeInteger, Value: receipts},
		{Key: "total", Label: "Total recibido", Type: TypeMoneyMXN, Value: decimal.RequireFromString(total)},
		{Key: "cash_linked", Label: "Pagado desde caja", Type: TypeMoneyMXN, Value: decimal.RequireFromString(linked),
			Hint: "Parte de estas compras vinculada con gastos de caja: ya está en el total, no es otra compra."},
		{Key: "returned_value", Label: "Devoluciones a proveedor del periodo", Type: TypeMoneyMXN,
			Value: decimal.RequireFromString(returned), Hint: "Aparte de las compras, a costo original; no se restan."},
	}, nil
}

func receivedQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := receiptConditions(query)
	costs := ""
	if costsVisible(query) {
		costs = ",l.inventory_value,l.unit_cost AS raw_unit_cost"
	}
	rows, err := query.collect(ctx, `SELECT r.occurred_at,r.folio::text AS receipt_folio,coalesce(o.folio::text,'') AS order_folio,
            `+modalitySQL+` AS modality_code,supplier.trade_name AS supplier,r.counter_store,w.name AS warehouse,i.code,i.name AS item,
            coalesce(category.name,'Sin categoría') AS category,l.quantity,coalesce(presentation.name,captured.code,unit.code) AS capture_unit,
            l.base_quantity,unit.code AS base_unit,l.unit_price,l.discount,l.subtotal,l.tax_rate,l.tax,l.total,
            receiver.display_name AS received_by,r.supplier_reference`+costs+`
        FROM app.inventory_receipt_lines l `+receiptLineJoins+`
        JOIN app.users receiver ON receiver.id=r.received_by
        WHERE true`+list.sql()+` ORDER BY r.occurred_at,r.folio,l.line_no`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["modality"] = modalityLabel(row["modality_code"].(string))
		if cost, ok := row["raw_unit_cost"].(decimal.Decimal); ok {
			row["unit_cost"] = cost.Round(unitCostDisplayScale)
		}
	}
	figures, err := purchaseFigures(ctx, query, list)
	if err != nil {
		return nil, err
	}
	return &Dataset{Rows: rows, Summary: figures, Notes: []string{"Una orden emitida no es mercancía recibida: lo ordenado está " +
		"en «Compras ordenadas». El gasto de caja vinculado no suma como compra."}}, nil
}

// orderLineSQL arma los renglones de órdenes con lo recibido y lo pendiente.
const orderLineSQL = `SELECT o.order_date,o.folio::text AS folio,o.state AS state_code,supplier.trade_name AS supplier,w.name AS warehouse,
        i.code,i.name AS item,coalesce(category.name,'Sin categoría') AS category,l.quantity,
        coalesce(presentation.name,captured.code,unit.code) AS capture_unit,l.unit_price,l.discount,l.subtotal,l.tax_rate,l.tax,l.total,
        l.base_quantity,coalesce(received.quantity,0) AS received_quantity,
        CASE WHEN o.state IN ('issued','partially_received') THEN greatest(l.base_quantity-coalesce(received.quantity,0),0) ELSE 0 END
            AS pending_quantity,
        unit.code AS base_unit,coalesce(received.receipts,0)::bigint AS receipts,coalesce(received.total,0) AS received_total
    FROM app.inventory_purchase_order_lines l
    JOIN app.inventory_purchase_orders o ON o.id=l.order_id
    JOIN app.inventory_suppliers supplier ON supplier.id=o.supplier_id
    JOIN app.inventory_warehouses w ON w.id=o.expected_warehouse_id
    JOIN app.inventory_items i ON i.id=l.item_id
    JOIN app.inventory_units unit ON unit.id=i.base_unit_id
    LEFT JOIN app.inventory_categories category ON category.id=i.category_id
    LEFT JOIN app.inventory_presentations presentation ON presentation.id=l.presentation_id
    LEFT JOIN app.inventory_units captured ON captured.id=l.unit_id
    LEFT JOIN LATERAL (
        SELECT sum(rl.base_quantity) AS quantity,count(DISTINCT rl.receipt_id) AS receipts,sum(rl.total) AS total
        FROM app.inventory_receipt_lines rl WHERE rl.order_id=l.order_id AND rl.order_line_no=l.line_no
    ) received ON true`

func orderedQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{"o.state <> 'draft'", "o.order_date BETWEEN @first_day::date AND @last_day::date"}
	if query.Has("state") {
		list.add("o.state = @state")
	} else {
		list.add("o.state <> 'cancelled'")
	}
	list.when(query, "supplier_id", "o.supplier_id = @supplier_id::uuid")
	list.when(query, "item_id", "l.item_id = @item_id::uuid")
	list.when(query, "category_id", "i.category_id = @category_id::uuid")
	list.when(query, "warehouse_id", "o.expected_warehouse_id = @warehouse_id::uuid")
	rows, err := query.collect(ctx, orderLineSQL+` WHERE true`+list.sql()+` ORDER BY o.order_date,o.folio,l.line_no`, cashArgs(query))
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["state"] = inventory.PurchaseOrderStateLabel(row["state_code"].(string))
	}
	notes := []string{"Sin borradores; las órdenes canceladas solo aparecen con su filtro de estado. Lo recibido incluye recepciones de cualquier fecha."}
	return &Dataset{Rows: rows, Notes: notes}, nil
}

func pendingQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{"o.state IN ('issued','partially_received')"}
	list.when(query, "supplier_id", "o.supplier_id = @supplier_id::uuid")
	list.when(query, "item_id", "l.item_id = @item_id::uuid")
	list.when(query, "warehouse_id", "o.expected_warehouse_id = @warehouse_id::uuid")
	rows, err := query.collect(ctx, `SELECT x.* FROM (`+orderLineSQL+` WHERE true`+list.sql()+`) x WHERE x.pending_quantity > 0
        ORDER BY x.order_date,x.folio`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["state"] = inventory.PurchaseOrderStateLabel(row["state_code"].(string))
		base := decimalOf(row, "base_quantity")
		if base.IsPositive() {
			row["pending_total"] = decimalOf(row, "total").Mul(decimalOf(row, "pending_quantity")).DivRound(base, 2)
		}
	}
	return &Dataset{Rows: rows}, nil
}

func bySupplierQuery(ctx context.Context, query *Query) (*Dataset, error) {
	receipts := conditions{inPeriod("r.occurred_at")}
	receipts.when(query, "supplier_id", "r.supplier_id = @supplier_id::uuid")
	receipts.when(query, "modality", modalitySQL+" = @modality")
	receipts.when(query, "warehouse_id", "r.warehouse_id = @warehouse_id::uuid")
	orders := conditions{"o.state NOT IN ('draft','cancelled')", "o.order_date BETWEEN @first_day::date AND @last_day::date"}
	orders.when(query, "supplier_id", "o.supplier_id = @supplier_id::uuid")
	orders.when(query, "warehouse_id", "o.expected_warehouse_id = @warehouse_id::uuid")
	if query.Has("modality") && query.Params.Value("modality") != "order" {
		orders.add("false")
	}
	returns := conditions{inPeriod("sr.occurred_at")}
	returns.when(query, "supplier_id", "sr.supplier_id = @supplier_id::uuid")
	returns.when(query, "warehouse_id", "sr.warehouse_id = @warehouse_id::uuid")
	returns.when(query, "modality", "EXISTS(SELECT FROM app.inventory_receipts r JOIN app.inventory_suppliers supplier ON supplier.id=r.supplier_id "+
		"WHERE r.id=sr.receipt_id AND "+modalitySQL+" = @modality)")
	rows, err := query.collect(ctx, `WITH received AS (
            SELECT r.supplier_id,count(*)::bigint AS receipts,sum(r.subtotal) AS subtotal,sum(r.discount) AS discount,sum(r.tax) AS tax,
                sum(r.total) AS total,
                sum((SELECT coalesce(sum(cl.amount),0) FROM app.inventory_receipt_cash_links cl
                    WHERE cl.receipt_id=r.id AND cl.unlinked_at IS NULL)) AS cash_linked
            FROM app.inventory_receipts r JOIN app.inventory_suppliers supplier ON supplier.id=r.supplier_id
            WHERE true`+receipts.sql()+` GROUP BY r.supplier_id
        ),
        ordered AS (
            SELECT o.supplier_id,sum(o.total) AS ordered_total FROM app.inventory_purchase_orders o WHERE true`+orders.sql()+`
            GROUP BY o.supplier_id
        ),
        returned AS (
            SELECT sr.supplier_id,sum(sr.value) AS returned_value FROM app.inventory_supplier_returns sr WHERE true`+returns.sql()+`
            GROUP BY sr.supplier_id
        ),
        suppliers AS (SELECT supplier_id FROM received UNION SELECT supplier_id FROM ordered UNION SELECT supplier_id FROM returned)
        SELECT supplier.trade_name AS supplier,CASE supplier.kind WHEN 'counter' THEN 'Compras de mostrador' ELSE 'Proveedor' END AS kind,
            coalesce(received.receipts,0) AS receipts,coalesce(received.subtotal,0) AS subtotal,coalesce(received.discount,0) AS discount,
            coalesce(received.tax,0) AS tax,coalesce(received.total,0) AS total,coalesce(ordered.ordered_total,0) AS ordered_total,
            round(coalesce(returned.returned_value,0),2) AS returned_value,coalesce(received.cash_linked,0) AS cash_linked
        FROM suppliers x JOIN app.inventory_suppliers supplier ON supplier.id=x.supplier_id
        LEFT JOIN received ON received.supplier_id=x.supplier_id
        LEFT JOIN ordered ON ordered.supplier_id=x.supplier_id
        LEFT JOIN returned ON returned.supplier_id=x.supplier_id`, cashArgs(query))
	if err != nil {
		return nil, err
	}
	return &Dataset{Rows: rows, Notes: []string{"Lo ordenado y las devoluciones se informan aparte y no se suman a lo recibido; " +
		"lo pagado desde caja ya está incluido en el total recibido."}}, nil
}

func byItemQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := receiptConditions(query)
	rows, err := query.collect(ctx, `SELECT i.code,(array_agg(i.name))[1] AS item,(array_agg(coalesce(category.name,'Sin categoría')))[1] AS category,
            sum(l.base_quantity) AS base_quantity,(array_agg(unit.code))[1] AS base_unit,count(DISTINCT l.receipt_id)::bigint AS receipts,
            sum(l.discount) AS discount,sum(l.subtotal) AS subtotal,sum(l.tax) AS tax,sum(l.total) AS total
        FROM app.inventory_receipt_lines l `+receiptLineJoins+`
        WHERE true`+list.sql()+` GROUP BY l.item_id,i.code ORDER BY 2`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		quantity := decimalOf(row, "base_quantity")
		if quantity.IsPositive() {
			row["average_price"] = decimalOf(row, "subtotal").DivRound(quantity, unitCostDisplayScale)
		}
	}
	figures, err := purchaseFigures(ctx, query, list)
	if err != nil {
		return nil, err
	}
	return &Dataset{Rows: rows, Summary: figures}, nil
}

func supplierReturnsQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{inPeriod("sr.occurred_at")}
	list.when(query, "supplier_id", "sr.supplier_id = @supplier_id::uuid")
	list.when(query, "item_id", "l.item_id = @item_id::uuid")
	list.when(query, "warehouse_id", "sr.warehouse_id = @warehouse_id::uuid")
	costs := ""
	if costsVisible(query) {
		costs = ",l.outflow_value,l.unabsorbed"
	}
	rows, err := query.collect(ctx, `SELECT sr.occurred_at,sr.folio::text AS folio,r.folio::text AS receipt_folio,supplier.trade_name AS supplier,
            w.name AS warehouse,i.code,i.name AS item,l.base_quantity,unit.code AS base_unit,l.value,sr.reason,actor.display_name AS actor`+costs+`
        FROM app.inventory_supplier_return_lines l
        JOIN app.inventory_supplier_returns sr ON sr.id=l.return_id
        JOIN app.inventory_receipts r ON r.id=sr.receipt_id
        JOIN app.inventory_suppliers supplier ON supplier.id=sr.supplier_id
        JOIN app.inventory_warehouses w ON w.id=sr.warehouse_id
        JOIN app.inventory_items i ON i.id=l.item_id
        JOIN app.inventory_units unit ON unit.id=i.base_unit_id
        JOIN app.users actor ON actor.id=sr.actor_id
        WHERE true`+list.sql()+` ORDER BY sr.occurred_at,sr.folio,l.line_no`, query.Args())
	if err != nil {
		return nil, err
	}
	return &Dataset{Rows: rows, Notes: []string{"Las devoluciones a proveedor no restan de las compras del periodo: se informan aparte."}}, nil
}

func cashLinksQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{inPeriod("r.occurred_at")}
	list.when(query, "supplier_id", "r.supplier_id = @supplier_id::uuid")
	switch query.Params.Value("state") {
	case "active":
		list.add("cl.unlinked_at IS NULL")
	case "unlinked":
		list.add("cl.unlinked_at IS NOT NULL")
	}
	expense := ""
	if query.Actor.CanAll([]string{cashMovementsRead, cashShiftsReadAll, cashHistoryRead}) {
		expense = ",o.created_at AS expense_at,register.name AS register,o.concept"
	}
	rows, err := query.collect(ctx, `SELECT r.occurred_at,r.folio::text AS receipt_folio,supplier.trade_name AS supplier,
            CASE WHEN cl.unlinked_at IS NULL THEN cl.amount ELSE 0 END AS linked,
            CASE WHEN cl.unlinked_at IS NOT NULL THEN 'Desvinculado'
                WHEN EXISTS(SELECT FROM app.cash_operations fix WHERE fix.corrects_operation_id=cl.cash_operation_id)
                    THEN 'Gasto corregido después del vínculo' ELSE 'Vigente' END AS status,
            cl.linked_at,linker.display_name AS linked_by`+expense+`
        FROM app.inventory_receipt_cash_links cl
        JOIN app.inventory_receipts r ON r.id=cl.receipt_id
        JOIN app.inventory_suppliers supplier ON supplier.id=r.supplier_id
        JOIN app.users linker ON linker.id=cl.linked_by
        JOIN app.cash_operations o ON o.id=cl.cash_operation_id
        JOIN app.registers register ON register.id=o.register_id
        WHERE true`+list.sql()+` ORDER BY r.occurred_at,cl.linked_at`, query.Args())
	if err != nil {
		return nil, err
	}
	return &Dataset{Rows: rows, Notes: []string{"El gasto vinculado ya está en el total de la compra: nunca se suma como otra compra " +
		"ni como otra salida de mercancía. Los datos del gasto exigen los permisos de consulta de Caja."}}, nil
}

func supplierActivityQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{}
	switch query.Params.Value("state") {
	case "active":
		list.add("s.active")
	case "inactive":
		list.add("NOT s.active")
	}
	rows, err := query.collect(ctx, `SELECT s.trade_name AS supplier,CASE s.kind WHEN 'counter' THEN 'Compras de mostrador' ELSE 'Proveedor' END AS kind,
            CASE WHEN s.active THEN 'Sí' ELSE 'No' END AS active,s.contact,
            trim(both ' · ' FROM concat_ws(' · ',nullif(s.phone,''),nullif(s.phone_secondary,''))) AS phone,s.mobile_phone,s.email,
            (SELECT count(*) FROM app.inventory_receipts r WHERE r.supplier_id=s.id AND `+inPeriod("r.occurred_at")+`)::bigint AS receipts,
            (SELECT coalesce(sum(r.total),0) FROM app.inventory_receipts r WHERE r.supplier_id=s.id AND `+inPeriod("r.occurred_at")+`) AS total,
            (SELECT max(r.occurred_at) FROM app.inventory_receipts r WHERE r.supplier_id=s.id) AS last_receipt_at,
            (SELECT count(*) FROM app.inventory_purchase_orders o WHERE o.supplier_id=s.id
                AND o.state IN ('issued','partially_received'))::bigint AS open_orders
        FROM app.inventory_suppliers s WHERE true`+list.sql()+` ORDER BY s.kind DESC,s.trade_name`, query.Args())
	if err != nil {
		return nil, err
	}
	return &Dataset{Rows: rows}, nil
}
