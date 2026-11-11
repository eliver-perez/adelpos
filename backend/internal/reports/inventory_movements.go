package reports

import (
	"context"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/inventory"
)

// Reportes del libro de movimientos: kardex, entradas, salidas, transferencias, ajustes y mermas.
// Exigen inventory.movements.read (como la consulta de movimientos de la etapa 8); los valores,
// costos unitarios, bases de costo y ajustes de costo exigen además inventory.costs.read, y la
// valuación del insumo después de cada movimiento, inventory.costs.read e inventory.valuation.read.

// movementConditions son los filtros comunes sobre el libro m y su insumo i.
func movementConditions(query *Query) conditions {
	list := conditions{inPeriod("m.occurred_at")}
	if !costsVisible(query) {
		list.add("m.kind <> 'cost_adjustment'")
	}
	list.when(query, "item_id", "m.item_id = @item_id::uuid")
	list.when(query, "warehouse_id", "m.warehouse_id = @warehouse_id::uuid")
	list.when(query, "category_id", "i.category_id = @category_id::uuid")
	list.when(query, "kind", "m.kind = @kind")
	return list
}

const movementJoins = `JOIN app.inventory_items i ON i.id=m.item_id
        JOIN app.inventory_units unit ON unit.id=i.base_unit_id
        LEFT JOIN app.inventory_categories category ON category.id=i.category_id
        LEFT JOIN app.inventory_warehouses w ON w.id=m.warehouse_id`

func registerMovementReports() {
	movements := []string{PermissionInventory, permissionInventoryMovements}
	register(Definition{
		ID: "inventory.kardex", Group: "inventory", Title: "Kardex",
		Description: "Cada movimiento del libro con la existencia del almacén después de aplicarlo y, con permiso, su costo y valor.",
		Permissions: movements, DateBasis: inventoryMovementsDateBasis,
		Filters: []Filter{filterItem(), filterWarehouse(), filterInventoryCategory(),
			filterChoice("kind", "Tipo de movimiento", "", movementKindOptions(inventory.MovementKinds...)...)},
		Columns: []Column{dateTimeColumn("occurred_at", "Fecha"), columnItemCode, columnItemName, columnWarehouse,
			textColumn("kind", "Movimiento"), textColumn("source", "Origen"),
			described(quantityColumn("quantity", "Cantidad"), "Con signo, en la unidad base del insumo."), columnItemUnit,
			described(quantityColumn("stock_after", "Existencia después"), "Existencia teórica del almacén después del movimiento."),
			inventoryCostColumn("unit_cost", "Costo unitario", "Costo por unidad base aplicado al movimiento."),
			requiring(described(moneyColumn("value", "Valor"), "Valor con signo del movimiento."), PermissionCostsRead),
			requiring(optional(textColumn("cost_basis", "Base del costo")), PermissionCostsRead),
			requiring(textColumn("cost_pending", "Costo pendiente"), PermissionCostsRead),
			requiring(optional(described(Column{Key: "valuation_after", Label: "Valuación del insumo después", Type: TypeMoneyMXN,
				Total: TotalCustom, Sortable: true}, "Valor total del insumo después del movimiento (no se suma).")),
				PermissionCostsRead, PermissionValuationRead),
			textColumn("responsible", "Responsable"), optional(textColumn("reason", "Motivo")),
			optional(dateTimeColumn("recorded_at", "Registro"))},
		DefaultSort: Sort{Key: "occurred_at"},
		Query:       kardexQuery,
	})
	register(Definition{
		ID: "inventory.entries", Group: "inventory", Title: "Entradas de inventario",
		Description: "Entradas del periodo por insumo y tipo (compras, existencias iniciales, reversiones, ajustes, transferencias, elaboraciones y sobrantes).",
		Permissions: movements, DateBasis: inventoryMovementsDateBasis,
		Filters: []Filter{filterItem(), filterWarehouse(), filterInventoryCategory(),
			filterChoice("kind", "Tipo de entrada", "", movementKindOptions(entryKinds...)...)},
		Columns:     flowColumns("Cantidad recibida"),
		DefaultSort: Sort{Key: "item"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			return flowQuery(ctx, query, entryKinds, 1)
		},
	})
	register(Definition{
		ID: "inventory.outflows", Group: "inventory", Title: "Salidas de inventario",
		Description: "Salidas del periodo por insumo y tipo (consumos, mermas, ajustes, transferencias, devoluciones a proveedor, elaboraciones y faltantes).",
		Permissions: movements, DateBasis: inventoryMovementsDateBasis,
		Filters: []Filter{filterItem(), filterWarehouse(), filterInventoryCategory(),
			filterChoice("kind", "Tipo de salida", "", movementKindOptions(outflowKinds...)...)},
		Columns:     flowColumns("Cantidad que salió"),
		DefaultSort: Sort{Key: "item"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			return flowQuery(ctx, query, outflowKinds, -1)
		},
	})
	register(Definition{
		ID: "inventory.transfers", Group: "inventory", Title: "Transferencias",
		Description: "Renglones de las transferencias entre almacenes del periodo.",
		Permissions: movements, DateBasis: "Hora de registro de la transferencia, con la hora de inicio del día.",
		Filters: []Filter{filterItem(), filterWarehouse(), filterInventoryCategory()},
		Columns: []Column{dateTimeColumn("occurred_at", "Fecha"), codeColumn("folio", "Folio"), textColumn("from_warehouse", "Origen"),
			textColumn("to_warehouse", "Destino"), columnItemCode, columnItemName, quantityColumn("quantity", "Cantidad"), columnItemUnit,
			textColumn("actor", "Registró"),
			requiring(described(moneyColumn("value", "Valor trasladado"), "Valor al costo promedio de la salida."), PermissionCostsRead),
			optional(textColumn("notes", "Observaciones"))},
		DefaultSort: Sort{Key: "occurred_at"},
		Query:       transfersQuery,
	})
	register(Definition{
		ID: "inventory.adjustments", Group: "inventory", Title: "Ajustes de inventario",
		Description: "Ajustes de discrepancia (positivos y negativos) con su motivo, estado y autorización.",
		Permissions: movements, DateBasis: "Hora de aplicación del ajuste (o de su registro si no se aplicó), con la hora de inicio del día.",
		Filters: []Filter{filterItem(), filterWarehouse(), filterChoice("state", "Estado", "",
			Option{Value: "applied", Label: inventory.AdjustmentStateLabel("applied")},
			Option{Value: "pending_authorization", Label: inventory.AdjustmentStateLabel("pending_authorization")},
			Option{Value: "rejected", Label: inventory.AdjustmentStateLabel("rejected")})},
		Columns: []Column{dateTimeColumn("occurred_at", "Fecha"), codeColumn("folio", "Folio"), textColumn("state", "Estado"),
			columnWarehouse, textColumn("reason", "Motivo"), columnItemCode, columnItemName,
			described(quantityColumn("quantity", "Cantidad"), "Con signo: positiva entra y negativa sale."), columnItemUnit,
			textColumn("requested_by", "Registró"), textColumn("authorized_by", "Autorizó"),
			requiring(described(moneyColumn("value", "Valor"), "Valor con signo del movimiento aplicado."), PermissionCostsRead)},
		DefaultSort: Sort{Key: "occurred_at"},
		Query:       adjustmentsQuery,
	})
	register(Definition{
		ID: "inventory.waste", Group: "inventory", Title: "Mermas",
		Description: "Mermas operativas (con su motivo) y cancelaciones clasificadas «Preparado / merma». Las devoluciones de dinero no son merma.",
		Permissions: movements, DateBasis: "Hora de la merma operativa o de la cancelación de la partida, con la hora de inicio del día.",
		Filters: []Filter{filterChoice("kind", "Origen", "", Option{Value: "operational", Label: "Mermas operativas"},
			Option{Value: "cancellation", Label: "Cancelaciones «Preparado / merma»"}), filterItem(), filterWarehouse()},
		Columns: []Column{dateTimeColumn("occurred_at", "Fecha"), textColumn("origin", "Origen"), textColumn("classification", "Clasificación"),
			described(codeColumn("folio", "Folio"), "Folio de la merma operativa o de la cuenta de la partida cancelada."),
			optional(textColumn("product", "Producto cancelado")), columnWarehouse, columnItemCode,
			columnItemName, described(quantityColumn("quantity", "Cantidad"), "Cantidad perdida en la unidad base."), columnItemUnit,
			textColumn("responsible", "Registró o clasificó"),
			requiring(described(moneyColumn("value", "Costo"), "Costo de la merma (valor de los movimientos)."), PermissionCostsRead)},
		DefaultSort: Sort{Key: "occurred_at"},
		Query:       wasteQuery,
	})
}

func flowColumns(quantityLabel string) []Column {
	return []Column{columnItemCode, columnItemName, columnItemCategory, textColumn("kind", "Tipo"),
		countColumn("movements", "Movimientos"), described(quantityColumn("quantity", quantityLabel), "En la unidad base del insumo."),
		columnItemUnit, requiring(described(moneyColumn("value", "Valor"), "Valor de los movimientos (sin signo)."), PermissionCostsRead)}
}

func kardexQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := movementConditions(query)
	costs := ""
	if costsVisible(query) {
		costs = ",m.unit_cost AS raw_unit_cost,m.value,m.cost_basis AS basis,m.cost_pending AS pending"
		if valuationVisible(query) {
			costs += ",m.valuation_value_after AS valuation_after"
		}
	}
	rows, err := query.collect(ctx, `SELECT m.occurred_at,m.recorded_at,i.code,i.name AS item,coalesce(w.name,'') AS warehouse,
            m.kind AS kind_code,m.source_kind,m.quantity,m.stock_after,unit.code AS unit,
            coalesce(actor.display_name,'') AS actor_name,coalesce(m.process,'') AS process,coalesce(m.reason,'') AS reason`+costs+`
        FROM app.inventory_movements m `+movementJoins+`
        LEFT JOIN app.users actor ON actor.id=m.actor_id
        WHERE true`+list.sql()+` ORDER BY m.occurred_at,m.seq`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["kind"] = inventory.MovementKindLabel(row["kind_code"].(string))
		row["source"] = inventory.SourceKindLabel(row["source_kind"].(string))
		row["responsible"] = row["actor_name"]
		if process, _ := row["process"].(string); process != "" {
			row["responsible"] = "Proceso de consumo de inventario"
		}
		if !costsVisible(query) {
			continue
		}
		if cost, ok := row["raw_unit_cost"].(decimal.Decimal); ok {
			row["unit_cost"] = cost.Round(unitCostDisplayScale)
		}
		row["cost_basis"] = inventory.CostBasisLabel(row["basis"].(string))
		row["cost_pending"] = ""
		if pending, _ := row["pending"].(bool); pending {
			row["cost_pending"] = "Sí"
		}
	}
	dataset, err := inventoryDataset(ctx, query, rows)
	if err != nil {
		return nil, err
	}
	if query.Has("item_id") {
		figures, err := kardexFigures(ctx, query)
		if err != nil {
			return nil, err
		}
		dataset.Summary = figures
	}
	return dataset, nil
}

// kardexFigures resume la existencia inicial, entradas, salidas y existencia final del insumo
// filtrado (y del almacén, si se indicó) en el periodo.
func kardexFigures(ctx context.Context, query *Query) ([]Figure, error) {
	warehouse := ""
	if query.Has("warehouse_id") {
		warehouse = " AND m.warehouse_id = @warehouse_id::uuid"
	}
	var opening, entries, outflows string
	err := query.Tx.QueryRow(ctx, `SELECT coalesce(sum(m.quantity) FILTER (WHERE m.occurred_at < @from),0)::text,
            coalesce(sum(m.quantity) FILTER (WHERE m.occurred_at >= @from AND m.quantity > 0),0)::text,
            coalesce(-sum(m.quantity) FILTER (WHERE m.occurred_at >= @from AND m.quantity < 0),0)::text
        FROM app.inventory_movements m WHERE m.item_id = @item_id::uuid AND m.occurred_at < @until`+warehouse, query.Args()).
		Scan(&opening, &entries, &outflows)
	if err != nil {
		return nil, err
	}
	start := decimal.RequireFromString(opening)
	in := decimal.RequireFromString(entries)
	out := decimal.RequireFromString(outflows)
	return []Figure{
		{Key: "opening", Label: "Existencia inicial", Type: TypeQuantity, Value: start},
		{Key: "entries", Label: "Entradas", Type: TypeQuantity, Value: in},
		{Key: "outflows", Label: "Salidas", Type: TypeQuantity, Value: out},
		{Key: "closing", Label: "Existencia final", Type: TypeQuantity, Value: start.Add(in).Sub(out)},
	}, nil
}

func flowQuery(ctx context.Context, query *Query, kinds []string, sign int) (*Dataset, error) {
	list := movementConditions(query)
	list.add("m.kind IN (" + sqlList(kinds) + ")")
	value := ""
	if costsVisible(query) {
		value = ",abs(sum(m.value)) AS value"
	}
	quantity := "sum(m.quantity)"
	if sign < 0 {
		quantity = "-sum(m.quantity)"
	}
	rows, err := query.collect(ctx, `SELECT i.code,(array_agg(i.name))[1] AS item,(array_agg(coalesce(category.name,'Sin categoría')))[1] AS category,
            m.kind AS kind_code,count(*)::bigint AS movements,`+quantity+` AS quantity,(array_agg(unit.code))[1] AS unit`+value+`
        FROM app.inventory_movements m `+movementJoins+`
        WHERE true`+list.sql()+` GROUP BY m.item_id,i.code,m.kind ORDER BY 2,m.kind`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["kind"] = inventory.MovementKindLabel(row["kind_code"].(string))
	}
	return inventoryDataset(ctx, query, rows)
}

func transfersQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{inPeriod("t.occurred_at")}
	list.when(query, "item_id", "l.item_id = @item_id::uuid")
	list.when(query, "warehouse_id", "(t.from_warehouse_id = @warehouse_id::uuid OR t.to_warehouse_id = @warehouse_id::uuid)")
	list.when(query, "category_id", "i.category_id = @category_id::uuid")
	value := ""
	if costsVisible(query) {
		value = ",-out.value AS value"
	}
	rows, err := query.collect(ctx, `SELECT t.occurred_at,t.folio::text AS folio,source.name AS from_warehouse,target.name AS to_warehouse,
            i.code,i.name AS item,l.quantity,unit.code AS unit,actor.display_name AS actor,t.notes`+value+`
        FROM app.inventory_transfer_lines l
        JOIN app.inventory_transfers t ON t.id=l.transfer_id
        JOIN app.inventory_warehouses source ON source.id=t.from_warehouse_id
        JOIN app.inventory_warehouses target ON target.id=t.to_warehouse_id
        JOIN app.inventory_items i ON i.id=l.item_id
        JOIN app.inventory_units unit ON unit.id=i.base_unit_id
        JOIN app.users actor ON actor.id=t.actor_id
        JOIN app.inventory_movements out ON out.id=l.out_movement_id
        WHERE true`+list.sql()+` ORDER BY t.occurred_at,t.folio,l.line_no`, query.Args())
	if err != nil {
		return nil, err
	}
	return inventoryDataset(ctx, query, rows)
}

func adjustmentsQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{"a.kind = 'discrepancy'", inPeriod("coalesce(a.applied_at,a.requested_at)")}
	list.when(query, "item_id", "l.item_id = @item_id::uuid")
	list.when(query, "warehouse_id", "a.warehouse_id = @warehouse_id::uuid")
	list.when(query, "state", "a.state = @state")
	value := ""
	if costsVisible(query) {
		value = ",movement.value"
	}
	rows, err := query.collect(ctx, `SELECT coalesce(a.applied_at,a.requested_at) AS occurred_at,a.folio::text AS folio,a.state AS state_code,
            w.name AS warehouse,a.reason AS reason_code,a.reason_detail,i.code,i.name AS item,l.quantity,unit.code AS unit,
            requester.display_name AS requested_by,coalesce(authorizer.display_name,'') AS authorized_by`+value+`
        FROM app.inventory_adjustment_lines l
        JOIN app.inventory_adjustments a ON a.id=l.adjustment_id
        JOIN app.inventory_warehouses w ON w.id=a.warehouse_id
        JOIN app.inventory_items i ON i.id=l.item_id
        JOIN app.inventory_units unit ON unit.id=i.base_unit_id
        JOIN app.users requester ON requester.id=a.requested_by
        LEFT JOIN app.users authorizer ON authorizer.id=a.authorized_by
        LEFT JOIN app.inventory_movements movement ON movement.id=l.movement_id
        WHERE true`+list.sql()+` ORDER BY 1,a.folio,l.line_no`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["state"] = inventory.AdjustmentStateLabel(row["state_code"].(string))
		row["reason"] = reasonText(row)
	}
	return inventoryDataset(ctx, query, rows)
}

// reasonText es el motivo de un ajuste con su detalle.
func reasonText(row Row) string {
	reason := inventory.AdjustmentReasonLabel(row["reason_code"].(string))
	if detail, _ := row["reason_detail"].(string); detail != "" {
		reason += " · " + detail
	}
	return reason
}

func wasteQuery(ctx context.Context, query *Query) (*Dataset, error) {
	operational := conditions{"m.kind = 'waste'", inPeriod("m.occurred_at")}
	operational.when(query, "item_id", "m.item_id = @item_id::uuid")
	operational.when(query, "warehouse_id", "m.warehouse_id = @warehouse_id::uuid")
	cancelled := conditions{"c.state = 'prepared_waste'", inPeriod("c.cancelled_at"), "m.kind IN ('consumption','consumption_reversal')"}
	cancelled.when(query, "item_id", "m.item_id = @item_id::uuid")
	cancelled.when(query, "warehouse_id", "m.warehouse_id = @warehouse_id::uuid")
	value, cancelledValue := "", ""
	if costsVisible(query) {
		value = ",-m.value AS value"
		cancelledValue = ",-sum(m.value) AS value"
	}
	parts := []string{}
	if query.Params.Value("kind") != "cancellation" {
		parts = append(parts, `SELECT m.occurred_at,'Merma operativa' AS origin,a.reason AS reason_code,a.reason_detail,a.folio::text AS folio,
                '' AS product,w.name AS warehouse,i.code,i.name AS item,-m.quantity AS quantity,unit.code AS unit,
                coalesce(actor.display_name,'') AS responsible`+value+`
            FROM app.inventory_movements m `+movementJoins+`
            JOIN app.inventory_adjustment_lines l ON l.movement_id=m.id
            JOIN app.inventory_adjustments a ON a.id=l.adjustment_id
            LEFT JOIN app.users actor ON actor.id=m.actor_id
            WHERE true`+operational.sql())
	}
	if query.Params.Value("kind") != "operational" {
		parts = append(parts, `SELECT c.cancelled_at AS occurred_at,'Cancelación de partida' AS origin,'prepared_waste' AS reason_code,
                '' AS reason_detail,account.folio::text AS folio,(array_agg(version.quote_line->>'name'))[1] AS product,
                (array_agg(w.name))[1] AS warehouse,i.code,(array_agg(i.name))[1] AS item,-sum(m.quantity) AS quantity,
                (array_agg(unit.code))[1] AS unit,coalesce((array_agg(classifier.display_name))[1],'') AS responsible`+cancelledValue+`
            FROM app.inventory_cancellation_classifications c
            JOIN app.inventory_movements m ON m.restaurant_unit_id=c.restaurant_unit_id
            `+movementJoins+`
            JOIN app.restaurant_item_units u ON u.id=c.restaurant_unit_id
            JOIN app.restaurant_item_versions version ON version.id=u.version_id
            JOIN app.restaurant_accounts account ON account.id=u.account_id
            LEFT JOIN app.users classifier ON classifier.id=c.classified_by
            WHERE true`+cancelled.sql()+`
            GROUP BY c.id,c.cancelled_at,account.folio,m.item_id,m.warehouse_id,i.code HAVING sum(m.quantity) <> 0`)
	}
	sql := parts[0]
	if len(parts) == 2 {
		sql += " UNION ALL " + parts[1]
	}
	rows, err := query.collect(ctx, sql+" ORDER BY 1", query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row["reason_code"] == "prepared_waste" {
			row["classification"] = "Preparado / merma"
			continue
		}
		row["classification"] = reasonText(row)
	}
	dataset, err := inventoryDataset(ctx, query, rows)
	if err != nil {
		return nil, err
	}
	var notPrepared, pending int64
	if err := query.Tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE c.state='not_prepared'),count(*) FILTER (WHERE c.state='pending')
        FROM app.inventory_cancellation_classifications c WHERE `+inPeriod("c.cancelled_at"), query.Args()).Scan(&notPrepared, &pending); err != nil {
		return nil, err
	}
	dataset.Summary = append(dataset.Summary,
		Figure{Key: "not_prepared", Label: "Cancelaciones «No preparado»", Type: TypeInteger, Value: notPrepared,
			Hint: "Su consumo se revirtió: no son merma."},
		Figure{Key: "pending_classification", Label: "Cancelaciones por clasificar", Type: TypeInteger, Value: pending,
			Hint: "Siguen contando como consumo hasta que se clasifiquen o venza su día operativo."})
	dataset.Notes = append(dataset.Notes, "Una devolución de dinero nunca es una merma ni reintegra ingredientes.")
	return dataset, nil
}
