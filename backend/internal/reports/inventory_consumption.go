package reports

import (
	"context"

	"local/ad-el-pos/server/internal/inventory"
)

// Conteos físicos y consumo por liberación a cocina.
//   - Conteos: cada conteo con sus renglones contados y las diferencias contra la existencia teórica
//     a la hora de la captura (como la etapa 8). Exigen inventory.stock.read, porque la existencia
//     teórica y la diferencia revelan existencias.
//   - Consumo: movimientos consumption + consumption_reversal de las unidades comerciales (el neto),
//     fechados con la liberación canónica a cocina (una por unidad aunque se haya enviado a varias
//     estaciones: el consumo se calcula una sola vez y aquí se cuentan unidades distintas). Los ajustes
//     de costo nunca forman parte del consumo (se reportan en Costos y márgenes como concepto del
//     periodo). Exigen inventory.movements.read; los valores, inventory.costs.read.

const consumptionDateBasis = "Hora de la liberación a cocina de cada unidad (hora de negocio del consumo), con la hora de inicio del día."

// consumptionConditions filtra los movimientos de consumo de unidades comerciales (alias m).
func consumptionConditions(query *Query) conditions {
	list := conditions{"m.kind IN ('consumption','consumption_reversal')", "m.restaurant_unit_id IS NOT NULL", inPeriod("m.occurred_at")}
	list.when(query, "item_id", "m.item_id = @item_id::uuid")
	list.when(query, "warehouse_id", "m.warehouse_id = @warehouse_id::uuid")
	list.when(query, "category_id", "i.category_id = @category_id::uuid")
	return list
}

func consumptionValue(query *Query) string {
	if costsVisible(query) {
		return ",-sum(m.value) AS value"
	}
	return ""
}

func registerControlReports() {
	stock := []string{PermissionInventory, PermissionStockRead}
	register(Definition{
		ID: "inventory.counts", Group: "inventory", Title: "Conteos físicos",
		Description: "Conteos del periodo con su estado, renglones contados y renglones con diferencia.",
		Permissions: stock, DateBasis: "Fecha de creación del conteo, con la hora de inicio del día.",
		Filters: []Filter{filterWarehouse(), filterChoice("state", "Estado", "",
			Option{Value: "open", Label: inventory.CountStateLabel("open")}, Option{Value: "submitted", Label: inventory.CountStateLabel("submitted")},
			Option{Value: "approved", Label: inventory.CountStateLabel("approved")}, Option{Value: "applied", Label: inventory.CountStateLabel("applied")},
			Option{Value: "cancelled", Label: inventory.CountStateLabel("cancelled")})},
		Columns: []Column{dateTimeColumn("created_at", "Creado"), codeColumn("folio", "Folio"), columnWarehouse, textColumn("scope", "Alcance"),
			textColumn("state", "Estado"), countColumn("lines", "Renglones"), countColumn("counted_lines", "Contados"),
			countColumn("difference_lines", "Con diferencia"), textColumn("created_by", "Creó"),
			optional(dateTimeColumn("applied_at", "Aplicado")), optional(textColumn("applied_by", "Aplicó")),
			requiring(described(moneyColumn("applied_value", "Valor de los ajustes"), "Valor con signo de los sobrantes y faltantes aplicados."),
				PermissionCostsRead)},
		DefaultSort: Sort{Key: "created_at"},
		Query:       countsQuery,
	})
	register(Definition{
		ID: "inventory.count_differences", Group: "inventory", Title: "Diferencias de conteo",
		Description: "Renglones contados con diferencia contra la existencia teórica a la hora de su captura.",
		Permissions: stock, DateBasis: "Hora de captura de cada renglón, con la hora de inicio del día.",
		Filters: []Filter{filterWarehouse(), filterItem(), filterInventoryCategory()},
		Columns: []Column{dateTimeColumn("captured_at", "Capturado"), codeColumn("folio", "Folio"), columnWarehouse, textColumn("state", "Estado"),
			columnItemCode, columnItemName, columnItemUnit, quantityColumn("theoretical", "Teórico"), quantityColumn("counted", "Contado"),
			described(quantityColumn("difference", "Diferencia"), "Contado − teórico."), textColumn("captured_by", "Contó"),
			requiring(described(moneyColumn("value", "Valor del ajuste"), "Valor con signo del movimiento aplicado."), PermissionCostsRead)},
		DefaultSort: Sort{Key: "captured_at"},
		Query:       countDifferencesQuery,
	})
}

func countsQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{inPeriod("c.created_at")}
	list.when(query, "warehouse_id", "c.warehouse_id = @warehouse_id::uuid")
	list.when(query, "state", "c.state = @state")
	value := ""
	if costsVisible(query) {
		value = `,(SELECT coalesce(sum(m.value),0) FROM app.inventory_count_lines l JOIN app.inventory_movements m ON m.id=l.movement_id
                WHERE l.count_id=c.id) AS applied_value`
	}
	rows, err := query.collect(ctx, `SELECT c.created_at,c.folio::text AS folio,w.name AS warehouse,c.scope AS scope_code,c.state AS state_code,
            (SELECT count(*) FROM app.inventory_count_lines l WHERE l.count_id=c.id)::bigint AS lines,
            (SELECT count(*) FROM app.inventory_count_lines l WHERE l.count_id=c.id AND l.counted_quantity IS NOT NULL)::bigint AS counted_lines,
            (SELECT count(*) FROM app.inventory_count_lines l WHERE l.count_id=c.id AND l.difference <> 0)::bigint AS difference_lines,
            creator.display_name AS created_by,c.applied_at,coalesce(applier.display_name,'') AS applied_by`+value+`
        FROM app.inventory_counts c
        JOIN app.inventory_warehouses w ON w.id=c.warehouse_id
        JOIN app.users creator ON creator.id=c.created_by
        LEFT JOIN app.users applier ON applier.id=c.applied_by
        WHERE true`+list.sql()+` ORDER BY c.created_at,c.folio`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["scope"] = inventory.CountScopeLabel(row["scope_code"].(string))
		row["state"] = inventory.CountStateLabel(row["state_code"].(string))
	}
	return inventoryDataset(ctx, query, rows)
}

func countDifferencesQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{"l.difference IS NOT NULL", "l.difference <> 0", "c.state <> 'cancelled'", inPeriod("l.captured_at")}
	list.when(query, "warehouse_id", "c.warehouse_id = @warehouse_id::uuid")
	list.when(query, "item_id", "l.item_id = @item_id::uuid")
	list.when(query, "category_id", "i.category_id = @category_id::uuid")
	value := ""
	if costsVisible(query) {
		value = ",m.value"
	}
	rows, err := query.collect(ctx, `SELECT l.captured_at,c.folio::text AS folio,w.name AS warehouse,c.state AS state_code,i.code,i.name AS item,
            unit.code AS unit,l.theoretical_quantity AS theoretical,l.counted_quantity AS counted,l.difference,
            coalesce(counter.display_name,'') AS captured_by`+value+`
        FROM app.inventory_count_lines l
        JOIN app.inventory_counts c ON c.id=l.count_id
        JOIN app.inventory_warehouses w ON w.id=c.warehouse_id
        JOIN app.inventory_items i ON i.id=l.item_id
        JOIN app.inventory_units unit ON unit.id=i.base_unit_id
        LEFT JOIN app.users counter ON counter.id=l.captured_by
        LEFT JOIN app.inventory_movements m ON m.id=l.movement_id
        WHERE true`+list.sql()+` ORDER BY l.captured_at,c.folio,l.line_no`, query.Args())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["state"] = inventory.CountStateLabel(row["state_code"].(string))
	}
	return inventoryDataset(ctx, query, rows)
}

func registerConsumptionReports() {
	movements := []string{PermissionInventory, permissionInventoryMovements}
	unitsColumn := described(countColumn("units", "Unidades"),
		"Unidades comerciales distintas que consumieron (una unidad enviada a varias estaciones cuenta una vez).")
	register(Definition{
		ID: "inventory.consumption_by_item", Group: "inventory", Title: "Consumo por ingrediente",
		Description: "Consumo neto de cada insumo por las unidades liberadas a cocina en el periodo.",
		Permissions: movements, DateBasis: consumptionDateBasis,
		Filters: []Filter{filterItem(), filterWarehouse(), filterInventoryCategory()},
		Columns: []Column{columnItemCode, columnItemName, columnItemCategory, columnItemUnit,
			described(quantityColumn("quantity", "Consumo"), "Consumo neto (consumos − reversiones) en la unidad base."), unitsColumn,
			requiring(described(moneyColumn("value", "Costo del consumo"), "Valor neto de los movimientos de consumo."), PermissionCostsRead)},
		DefaultSort: Sort{Key: "item"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, `SELECT i.code,(array_agg(i.name))[1] AS item,
                    (array_agg(coalesce(category.name,'Sin categoría')))[1] AS category,(array_agg(unit.code))[1] AS unit,
                    -sum(m.quantity) AS quantity,count(DISTINCT m.restaurant_unit_id)::bigint AS units`+consumptionValue(query)+`
                FROM app.inventory_movements m `+movementJoins+`
                WHERE true`+consumptionConditions(query).sql()+` GROUP BY m.item_id,i.code HAVING sum(m.quantity) <> 0
                ORDER BY 2`, query.Args())
			if err != nil {
				return nil, err
			}
			return consumptionDataset(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "inventory.consumption_by_day", Group: "inventory", Title: "Consumo por periodo",
		Description: "Consumo de cada día comercial: unidades que consumieron, movimientos y, con permiso, su costo.",
		Permissions: movements, DateBasis: consumptionDateBasis,
		Filters: []Filter{filterItem(), filterWarehouse(), filterInventoryCategory()},
		Columns: []Column{dateColumn("day", "Día"), unitsColumn, countColumn("movements", "Movimientos"),
			described(quantityColumn("quantity", "Consumo del insumo"), "Solo con filtro de insumo (unidad base)."),
			requiring(described(moneyColumn("value", "Costo del consumo"), "Valor neto de los movimientos de consumo."), PermissionCostsRead)},
		DefaultSort: Sort{Key: "day"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			quantity := "NULL::numeric"
			if query.Has("item_id") {
				quantity = "-sum(m.quantity)"
			}
			rows, err := query.collect(ctx, `SELECT `+businessDate("m.occurred_at")+` AS day,count(DISTINCT m.restaurant_unit_id)::bigint AS units,
                    count(*)::bigint AS movements,`+quantity+` AS quantity`+consumptionValue(query)+`
                FROM app.inventory_movements m JOIN app.inventory_items i ON i.id=m.item_id
                WHERE true`+consumptionConditions(query).sql()+` GROUP BY 1 ORDER BY 1`, query.Args())
			if err != nil {
				return nil, err
			}
			return consumptionDataset(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "inventory.consumption_by_product", Group: "inventory", Title: "Consumo por producto",
		Description: "Insumos que consumió cada producto vendido o preparado en el periodo (complementos y componentes de combo incluidos).",
		Permissions: movements, DateBasis: consumptionDateBasis,
		Filters: []Filter{filterProduct(), filterItem(), filterWarehouse()},
		Columns: []Column{textColumn("product", "Producto"), columnItemCode, columnItemName, columnItemUnit,
			described(quantityColumn("quantity", "Consumo"), "Consumo neto en la unidad base."), unitsColumn,
			requiring(described(moneyColumn("value", "Costo del consumo"), "Valor neto de los movimientos de consumo."), PermissionCostsRead)},
		DefaultSort: Sort{Key: "product"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := consumptionConditions(query)
			list.when(query, "product_id", "item.product_id = @product_id::uuid")
			rows, err := query.collect(ctx, `SELECT (array_agg(version.quote_line->>'name' ORDER BY m.seq DESC))[1] AS product,i.code,
                    (array_agg(i.name))[1] AS item,(array_agg(unit.code))[1] AS unit,-sum(m.quantity) AS quantity,
                    count(DISTINCT m.restaurant_unit_id)::bigint AS units`+consumptionValue(query)+`
                FROM app.inventory_movements m `+movementJoins+`
                JOIN app.restaurant_item_units u ON u.id=m.restaurant_unit_id
                JOIN app.restaurant_items item ON item.id=u.item_id
                JOIN app.restaurant_item_versions version ON version.id=u.version_id
                WHERE true`+list.sql()+` GROUP BY item.product_id,m.item_id,i.code HAVING sum(m.quantity) <> 0
                ORDER BY 1,3`, query.Args())
			if err != nil {
				return nil, err
			}
			return consumptionDataset(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "inventory.consumption_by_kitchen", Group: "inventory", Title: "Consumo asociado a cocina",
		Description: "Consumo de las liberaciones a cocina por almacén de consumo y modalidad; cada unidad consume una sola vez.",
		Permissions: movements, DateBasis: consumptionDateBasis,
		Filters: []Filter{filterWarehouse(), filterMode()},
		Columns: []Column{columnWarehouse, textColumn("mode", "Modalidad"), unitsColumn, countColumn("movements", "Movimientos"),
			countColumn("items", "Insumos distintos"),
			requiring(described(moneyColumn("value", "Costo del consumo"), "Valor neto de los movimientos de consumo."), PermissionCostsRead)},
		DefaultSort: Sort{Key: "warehouse"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := consumptionConditions(query)
			list.when(query, "mode", "uc.service_mode = @mode")
			rows, err := query.collect(ctx, `SELECT coalesce(w.name,'') AS warehouse,
                    CASE uc.service_mode WHEN 'dine_in' THEN 'Salón' ELSE 'Para llevar y ventanilla' END AS mode,
                    count(DISTINCT m.restaurant_unit_id)::bigint AS units,count(*)::bigint AS movements,
                    count(DISTINCT m.item_id)::bigint AS items`+consumptionValue(query)+`
                FROM app.inventory_movements m `+movementJoins+`
                JOIN app.inventory_unit_consumptions uc ON uc.restaurant_unit_id=m.restaurant_unit_id
                WHERE true`+list.sql()+` GROUP BY w.name,uc.service_mode ORDER BY 1,2`, query.Args())
			if err != nil {
				return nil, err
			}
			dataset, err := consumptionDataset(ctx, query, rows)
			if err != nil {
				return nil, err
			}
			dataset.Notes = append(dataset.Notes, "Una unidad que consume en dos almacenes aparece en ambos renglones; el total de "+
				"unidades del resumen la cuenta una sola vez.")
			return dataset, nil
		},
	})
}

// consumptionDataset agrega el aviso de consumo pendiente y el total de unidades distintas del
// periodo (cada una una vez, aunque aparezca en varios renglones).
func consumptionDataset(ctx context.Context, query *Query, rows []Row) (*Dataset, error) {
	dataset, err := inventoryDataset(ctx, query, rows)
	if err != nil {
		return nil, err
	}
	list := consumptionConditions(query)
	list.when(query, "mode", "EXISTS(SELECT FROM app.inventory_unit_consumptions uc WHERE uc.restaurant_unit_id=m.restaurant_unit_id AND uc.service_mode = @mode)")
	list.when(query, "product_id", "EXISTS(SELECT FROM app.restaurant_item_units u JOIN app.restaurant_items item ON item.id=u.item_id "+
		"WHERE u.id=m.restaurant_unit_id AND item.product_id = @product_id::uuid)")
	var units int64
	if err := query.Tx.QueryRow(ctx, `SELECT count(DISTINCT m.restaurant_unit_id) FROM app.inventory_movements m
        JOIN app.inventory_items i ON i.id=m.item_id WHERE true`+list.sql(), query.Args()).Scan(&units); err != nil {
		return nil, err
	}
	dataset.Summary = append(dataset.Summary, Figure{Key: "units", Label: "Unidades con consumo", Type: TypeInteger, Value: units,
		Hint: "Cada unidad comercial una sola vez, aunque se haya enviado a varias estaciones."})
	return dataset, nil
}
