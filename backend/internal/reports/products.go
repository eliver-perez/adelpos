package reports

import (
	"context"

	"github.com/shopspring/decimal"
)

// Reportes de productos y categorías (§9). Las ventas salen del libro de reconocimientos con los
// nombres registrados al vender (un producto renombrado, desactivado o archivado conserva su
// historial). Un combo es la unidad vendida: los componentes solo se reportan como cantidades
// preparadas, sin importe propio, y nunca suman al ingreso.

func productRanking(ctx context.Context, query *Query) (*Dataset, error) {
	rows, err := ledgerReport(ctx, query, latest("r.product_code")+" AS code,"+latest("r.product_name")+" AS product,"+
		"coalesce("+latest("r.category_name")+",'Sin categoría') AS category,r.product_id::text AS product_ref",
		"r.product_id", "r.product_id IS NOT NULL", "")
	if err != nil {
		return nil, err
	}
	return productDataset(ctx, query, rows)
}

// unsoldProducts agrega con cantidad cero los productos activos del catálogo sin ventas en el
// periodo, para que «menos vendidos» muestre también lo que no se vendió.
func unsoldProducts(ctx context.Context, query *Query) (*Dataset, error) {
	dataset, err := productRanking(ctx, query)
	if err != nil {
		return nil, err
	}
	if query.Has("price_list_id") || query.Has("mode") || query.Has("area_id") || query.Has("table_id") || query.Has("origin") {
		return dataset, nil
	}
	sold := map[string]bool{}
	for _, row := range dataset.Rows {
		if reference, ok := row["product_ref"].(string); ok {
			sold[reference] = true
		}
	}
	list := conditions{}
	list.when(query, "product_id", "p.id = @product_id::uuid")
	list.when(query, "category_id", "p.category_id = @category_id::uuid")
	candidates, err := query.collect(ctx, `SELECT p.code,p.name AS product,c.name AS category,p.id::text AS product_ref
        FROM app.catalog_products p JOIN app.catalog_categories c ON c.id=p.category_id
        WHERE p.active AND NOT p.archived`+list.sql(), query.Args())
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		if sold[candidate["product_ref"].(string)] {
			continue
		}
		dataset.Rows = append(dataset.Rows, zeroMeasures(candidate, query.definition.Columns))
	}
	dataset.Notes = append(dataset.Notes, "Incluye con cantidad cero los productos activos del catálogo sin ventas en el periodo.")
	return dataset, nil
}

func trendDataset(ctx context.Context, query *Query) (*Dataset, error) {
	previous := query.Params.Previous
	if previous == nil {
		computed := query.Params.Period.Previous(query.Now, query.Config)
		previous = &computed
		query.Params.Previous = previous
	}
	current, err := productRanking(ctx, query)
	if err != nil {
		return nil, err
	}
	list := ledgerConditions(query)
	list.add("r.product_id IS NOT NULL")
	before, err := collectRows(ctx, query.Tx, `SELECT r.product_id::text AS product_ref,`+latest("r.product_code")+` AS code,
            `+latest("r.product_name")+` AS product,coalesce(`+latest("r.category_name")+`,'Sin categoría') AS category,
            coalesce(sum(r.quantity),0)::bigint AS previous_quantity
        FROM app.sales_recognitions r WHERE true`+list.sql()+` GROUP BY r.product_id`, query.PeriodArgs(previous),
		map[string]string{"previous_quantity": TypeInteger})
	if err != nil {
		return nil, err
	}
	byProduct := map[string]Row{}
	for _, row := range current.Rows {
		row["previous_quantity"] = int64(0)
		byProduct[row["product_ref"].(string)] = row
	}
	for _, row := range before {
		reference := row["product_ref"].(string)
		if existing, found := byProduct[reference]; found {
			existing["previous_quantity"] = row["previous_quantity"]
			continue
		}
		row["quantity"] = int64(0)
		row["net"] = decimal.Zero
		current.Rows = append(current.Rows, row)
	}
	for _, row := range current.Rows {
		quantity, _ := row["quantity"].(int64)
		previousQuantity, _ := row["previous_quantity"].(int64)
		row["change"] = quantity - previousQuantity
		row["change_percent"] = percentOf(decimal.NewFromInt(quantity-previousQuantity), decimal.NewFromInt(previousQuantity))
	}
	return current, nil
}

func init() {
	rankingColumns := []Column{codeColumn("code", "Código"), textColumn("product", "Producto"), textColumn("category", "Categoría"),
		columnQuantity, columnNet, columnShare}
	register(Definition{
		ID: "products.top", Group: "products", Title: "Productos más vendidos",
		Description: "Productos ordenados por unidades vendidas, con su venta neta.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(), Columns: rankingColumns,
		DefaultSort: Sort{Key: "quantity", Descending: true}, Totals: shareTotals, Query: productRanking,
	})
	register(Definition{
		ID: "products.bottom", Group: "products", Title: "Productos menos vendidos",
		Description: "Productos ordenados de menos a más unidades vendidas, incluidos los activos sin ventas.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(), Columns: rankingColumns,
		DefaultSort: Sort{Key: "quantity"}, Totals: shareTotals, Query: unsoldProducts,
	})
	register(Definition{
		ID: "products.combos", Group: "products", Title: "Combos vendidos",
		Description: "Combos comerciales vendidos: el combo es la venta; sus componentes no suman ingresos.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(),
		Columns: withColumns([]Column{codeColumn("code", "Código"), textColumn("product", "Combo")}, salesMeasureColumns(),
			[]Column{columnShare}),
		DefaultSort: Sort{Key: "quantity", Descending: true}, Totals: shareTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, latest("r.product_code")+" AS code,"+latest("r.product_name")+" AS product",
				"r.product_id", "r.product_kind = 'combo'", "")
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows, Notes: []string{"Los componentes de cada combo se consultan en «Componentes preparados de combos», sin importe."}}, nil
		},
	})
	register(Definition{
		ID: "products.combo_components", Group: "products", Title: "Componentes preparados de combos",
		Description: "Cantidades de cada componente preparado dentro de los combos vendidos; sin importe propio.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: []Filter{filterProduct(), filterMode()},
		Columns: []Column{textColumn("component", "Componente"), textColumn("slot", "Ranura"), textColumn("combo", "Combo"),
			described(countColumn("quantity", "Cantidad preparada"), "Unidades del componente dentro de los combos vendidos.")},
		DefaultSort: Sort{Key: "quantity", Descending: true},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := ledgerConditions(query)
			list.add("r.source_kind = 'unit' AND r.product_kind = 'combo'")
			rows, err := query.collect(ctx, `SELECT component.value->'product'->>'name' AS component,component.value->>'name' AS slot,
                    `+latest("r.product_name")+` AS combo,
                    sum(r.quantity*(component.value->>'quantity')::integer)::bigint AS quantity
                FROM app.sales_recognitions r
                JOIN app.restaurant_item_versions v ON v.id=r.version_id
                CROSS JOIN LATERAL jsonb_array_elements(coalesce(v.quote_line->'components','[]'::jsonb)) component
                WHERE component.value->'product' IS NOT NULL AND (component.value->>'quantity')::integer > 0`+list.sql()+`
                GROUP BY r.product_id,component.value->'product'->>'product_id',component.value->'product'->>'name',
                    component.value->>'name'`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows, Notes: []string{"Cantidades preparadas, no ventas: el ingreso es el del combo."}}, nil
		},
	})
	register(Definition{
		ID: "products.complements", Group: "products", Title: "Complementos seleccionados",
		Description: "Complementos elegidos en los grupos de las partidas vendidas (también dentro de combos).",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: []Filter{filterProduct(), filterCategory(), filterMode()},
		Columns: []Column{textColumn("group", "Grupo"), textColumn("complement", "Complemento"),
			described(countColumn("units", "Partidas"), "Unidades vendidas que llevaron el complemento."),
			described(Column{Key: "portions", Label: "Porciones", Type: TypeQuantity, Total: TotalSum, Sortable: true},
				"Porciones solicitadas en total."),
			described(Column{Key: "extra_portions", Label: "Porciones adicionales", Type: TypeQuantity, Total: TotalSum, Sortable: true},
				"Porciones por encima de las incluidas.")},
		DefaultSort: Sort{Key: "units", Descending: true},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := ledgerConditions(query)
			list.add("r.source_kind = 'unit'")
			rows, err := query.collect(ctx, `WITH lines AS (
                    SELECT r.quantity,v.quote_line AS line FROM app.sales_recognitions r
                    JOIN app.restaurant_item_versions v ON v.id=r.version_id WHERE true`+list.sql()+`
                    UNION ALL
                    SELECT r.quantity*(component.value->>'quantity')::integer,component.value->'product'
                    FROM app.sales_recognitions r
                    JOIN app.restaurant_item_versions v ON v.id=r.version_id
                    CROSS JOIN LATERAL jsonb_array_elements(coalesce(v.quote_line->'components','[]'::jsonb)) component
                    WHERE component.value->'product' IS NOT NULL AND (component.value->>'quantity')::integer > 0`+list.sql()+`
                )
                SELECT grp.value->>'name' AS "group",choice.value->>'name' AS complement,
                    sum(lines.quantity) FILTER (WHERE (choice.value->>'requested')::numeric > 0)::bigint AS units,
                    coalesce(sum(lines.quantity*(choice.value->>'requested')::numeric),0) AS portions,
                    coalesce(sum(lines.quantity*greatest((choice.value->>'requested')::numeric-(choice.value->>'included')::numeric,0)),0)
                        AS extra_portions
                FROM lines
                CROSS JOIN LATERAL jsonb_array_elements(coalesce(lines.line->'groups','[]'::jsonb)) grp
                CROSS JOIN LATERAL jsonb_array_elements(coalesce(grp.value->'options','[]'::jsonb)) choice
                WHERE (choice.value->>'requested')::numeric > 0
                GROUP BY grp.value->>'group_id',grp.value->>'name',choice.value->>'complement_id',choice.value->>'name'`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows}, nil
		},
	})
	register(Definition{
		ID: "products.cancelled", Group: "products", Title: "Productos cancelados",
		Description: "Unidades confirmadas que se cancelaron (también al cancelar su cuenta), con la fecha de cancelación, el motivo, " +
			"quién canceló y quién lo autorizó.",
		Permissions: []string{PermissionSales}, DateBasis: "Fecha de cancelación de la unidad, con la hora de inicio del día.",
		Filters: []Filter{filterProduct(), filterMode()},
		Columns: []Column{dateTimeColumn("cancelled_at", "Cancelación"), codeColumn("folio", "Folio"), textColumn("product", "Producto"),
			countColumn("quantity", "Cantidad"), moneyColumn("amount", "Importe"), textColumn("reason", "Motivo"),
			textColumn("cancelled_by", "Canceló"),
			// Etapa 9F (§7.2): el supervisor que autorizó, como en «Modificaciones de cantidades».
			described(textColumn("authorized_by", "Autorizó"), unitsAuthorizedDescription),
			textColumn("released", "Liberada a cocina")},
		DefaultSort: Sort{Key: "cancelled_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := conditions{inPeriod("u.cancelled_at"), "u.state = 'cancelled'"}
			list.when(query, "product_id", "i.product_id = @product_id::uuid")
			list.when(query, "mode", "a.mode = @mode")
			rows, err := query.collect(ctx, `SELECT u.cancelled_at,a.folio::text AS folio,coalesce(v.quote_line->>'name','') AS product,
                    1::bigint AS quantity,u.total AS amount,
                    CASE WHEN event.kind='account.cancelled' THEN 'Cuenta cancelada: '||coalesce(event.reason,'')
                        ELSE coalesce(event.reason,'') END AS reason,coalesce(author.display_name,'') AS cancelled_by,
                    coalesce(approver.display_name,'') AS authorized_by,
                    CASE WHEN EXISTS(SELECT FROM app.kitchen_unit_releases k WHERE k.unit_id=u.id) THEN 'Sí' ELSE 'No' END AS released
                FROM app.restaurant_item_units u
                JOIN app.restaurant_item_versions v ON v.id=u.version_id
                JOIN app.restaurant_items i ON i.id=u.item_id
                JOIN app.restaurant_accounts a ON a.id=u.account_id
                LEFT JOIN LATERAL (
                    SELECT e.kind,e.payload->'after'->>'reason' AS reason,e.author_id,
                        e.payload->'after'->>'authorized_by' AS authorized_by FROM app.restaurant_events e
                    WHERE e.account_id=u.account_id AND e.kind IN ('item.cancelled','item.modified','account.cancelled')
                        AND (coalesce(e.payload->'after'->'unit_ids','[]'::jsonb) ? u.id::text
                            OR coalesce(e.payload->'after'->'cancelled_unit_ids','[]'::jsonb) ? u.id::text)
                    ORDER BY e.sequence DESC LIMIT 1
                ) event ON true
                LEFT JOIN app.users author ON author.id=event.author_id
                LEFT JOIN app.users approver ON approver.id=event.authorized_by::uuid
                WHERE true`+list.sql(), query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows, Notes: []string{"Una cancelación no es una devolución: no mueve dinero. Las unidades canceladas nunca se reconocen como venta."}}, nil
		},
	})
	register(Definition{
		ID: "products.discounted", Group: "products", Title: "Productos con descuentos",
		Description: "Descuentos y cortesías por partida asignados a cada producto (los generales de cuenta no se asignan).",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(),
		Columns: []Column{codeColumn("code", "Código"), textColumn("product", "Producto"),
			described(countColumn("units", "Unidades con descuento"), "Unidades con un descuento o cortesía por partida."),
			columnDiscount, columnCourtesy, moneyColumn("total_discount", "Total descontado")},
		DefaultSort: Sort{Key: "total_discount", Descending: true},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := ledgerConditions(query)
			list.add("r.source_kind = 'allocation'")
			rows, err := query.collect(ctx, `SELECT `+latest("r.product_code")+` AS code,`+latest("r.product_name")+` AS product,
                    count(DISTINCT r.unit_id)::bigint AS units,sum(r.discount) AS discount,sum(r.courtesy) AS courtesy,
                    sum(r.discount+r.courtesy) AS total_discount
                FROM app.sales_recognitions r WHERE true`+list.sql()+` GROUP BY r.product_id`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows}, nil
		},
	})
	register(Definition{
		ID: "products.trends", Group: "products", Title: "Tendencia de demanda",
		Description: "Unidades vendidas por producto frente al periodo anterior equivalente.",
		Permissions: []string{PermissionSales}, Comparison: true, DateBasis: salesDateBasis, Filters: salesFilters(),
		Columns: []Column{codeColumn("code", "Código"), textColumn("product", "Producto"), textColumn("category", "Categoría"),
			columnQuantity, countColumn("previous_quantity", "Cantidad anterior"),
			described(countColumn("change", "Variación"), "Cantidad − cantidad anterior."), percentColumn("change_percent", "Variación %")},
		DefaultSort: Sort{Key: "change", Descending: true},
		Totals: func(rows []Row, totals Row) {
			quantity, _ := totals["quantity"].(int64)
			previous, _ := totals["previous_quantity"].(int64)
			totals["change_percent"] = percentOf(decimal.NewFromInt(quantity-previous), decimal.NewFromInt(previous))
		},
		Query: trendDataset,
	})
}
