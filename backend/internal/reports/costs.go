package reports

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/inventory"
)

// Costos y márgenes (§14 y §0-B «Márgenes y costos», fase B2). Exigen reports.sales.read e
// inventory.costs.read. Reglas:
//   - el costo de una venta sale de los movimientos de consumo reales de sus unidades (consumption +
//     consumption_reversal ligados a la unidad), nunca de la receta actual por el costo actual; los
//     ajustes de costo no se reparten entre productos: se reportan como concepto del periodo;
//   - estado del costo de cada unidad reconocida: «Sin costo» (sin consumo registrado: liberada sin
//     control de inventario o sin liberar, sin receta, o con una solicitud de consumo pendiente o con
//     error), «Estimado» (algún consumo con costo pendiente o valuado con el último costo válido o el
//     de referencia por existencia negativa o en cero, o con configuración incompleta) y «Firme»;
//   - margen estimado = venta sin IVA de las unidades con costo − su costo; las unidades «Sin costo»
//     no entran al margen y el reporte dice qué porcentaje de la venta tiene costo firme. Es margen
//     bruto estimado sobre ingredientes, nunca utilidad contable (no incluye gastos generales);
//   - la venta es la del libro de reconocimientos (fecha de cierre de la cuenta), neta de promociones
//     y descuentos por partida; los descuentos y cargos generales de cuenta no pertenecen a un
//     producto y se informan aparte;
//   - el costo teórico es el del motor de recetas de la etapa 8 con los costos vigentes.

const marginDateBasis = "Fecha de cierre de la cuenta (reconocimiento de la venta), con la hora de inicio del día; el costo es el de " +
	"los consumos reales de esas unidades."

func costPermissions() []string {
	return []string{PermissionSales, PermissionCostsRead}
}

// Diagnósticos del trabajador de consumo que indican configuración incompleta (resolution.go).
const incompleteDiagnostics = `'no_configuration','ingredient_without_portion','complement_without_configuration',
    'preparation_unavailable','depth_exceeded','preparation_cycle'`

// Estados del costo de una unidad y su clasificación visible.
var costStatusLabels = map[string]string{
	"firm":         "Firme",
	"provisional":  "Estimado",
	"incomplete":   "Estimado",
	"pending":      "Sin costo",
	"not_released": "Sin costo",
	"no_recipe":    "Sin costo",
}

var costReasonLabels = map[string]string{
	"firm":         "Costo firme",
	"provisional":  "Costo provisional (existencia negativa o en cero, o costo pendiente)",
	"incomplete":   "Configuración incompleta (ingrediente o complemento sin porción o receta)",
	"pending":      "Consumo pendiente de procesar o con error",
	"not_released": "Sin consumo registrado (liberada sin control de inventario o aún no liberada)",
	"no_recipe":    "Sin receta: la liberación no generó consumo",
}

// marginBaseSQL es la base de los márgenes: los renglones del libro de reconocimientos del periodo
// que pertenecen a una unidad (la unidad y sus descuentos por partida) y el estado y costo neto de
// cada unidad. El costo de un renglón es su cantidad (1, o −1 si una reliquidación la retiró) por el
// costo neto de la unidad.
func marginBaseSQL(list conditions) string {
	return `WITH recognized AS (
            SELECT r.* FROM app.sales_recognitions r
            WHERE r.unit_id IS NOT NULL` + list.sql() + `
        ),
        units AS (SELECT DISTINCT unit_id FROM recognized),
        unit_costs AS (
            SELECT m.restaurant_unit_id AS unit_id,-sum(m.value) AS cost,count(*) AS movements,
                bool_or(m.cost_pending OR m.cost_basis IN ('reference','configured','pending')) AS provisional
            FROM app.inventory_movements m JOIN units u ON u.unit_id=m.restaurant_unit_id
            WHERE m.kind IN ('consumption','consumption_reversal') GROUP BY 1
        ),
        requests AS (
            SELECT q.restaurant_unit_id AS unit_id,bool_or(q.state <> 'processed') AS pending,
                bool_or(q.state = 'processed' AND EXISTS(SELECT FROM jsonb_array_elements(q.diagnostics) d
                    WHERE d->>'code' IN (` + incompleteDiagnostics + `))) AS incomplete
            FROM app.inventory_consumption_requests q JOIN units u ON u.unit_id=q.restaurant_unit_id GROUP BY 1
        ),
        status AS (
            SELECT u.unit_id,coalesce(k.cost,0) AS cost,
                CASE WHEN coalesce(q.pending,false) THEN 'pending'
                    WHEN c.restaurant_unit_id IS NULL THEN 'not_released'
                    WHEN coalesce(k.movements,0) = 0 THEN 'no_recipe'
                    WHEN k.provisional THEN 'provisional'
                    WHEN coalesce(q.incomplete,false) THEN 'incomplete'
                    ELSE 'firm' END AS status
            FROM units u
            LEFT JOIN unit_costs k ON k.unit_id=u.unit_id
            LEFT JOIN requests q ON q.unit_id=u.unit_id
            LEFT JOIN app.inventory_unit_consumptions c ON c.restaurant_unit_id=u.unit_id
        )`
}

// marginMeasures son las medidas de margen de un grupo de renglones r con su estado st.
const marginMeasures = `coalesce(sum(r.quantity),0)::bigint AS units,coalesce(sum(r.net),0) AS net,coalesce(sum(r.base),0) AS base,
    coalesce(sum(r.base) FILTER (WHERE st.status IN ('firm','provisional','incomplete')),0) AS costed_base,
    coalesce(sum(r.quantity*st.cost) FILTER (WHERE st.status IN ('firm','provisional','incomplete')),0) AS cost,
    coalesce(sum(r.base) FILTER (WHERE st.status='firm'),0) AS firm_base,
    coalesce(sum(r.quantity) FILTER (WHERE st.status='firm'),0)::bigint AS firm_units,
    coalesce(sum(r.quantity) FILTER (WHERE st.status IN ('provisional','incomplete')),0)::bigint AS estimated_units,
    coalesce(sum(r.quantity) FILTER (WHERE st.status IN ('pending','not_released','no_recipe')),0)::bigint AS uncosted_units`

// marginConditions son el periodo y los filtros de producto y categoría sobre el libro r.
func marginConditions(query *Query) conditions {
	list := conditions{inPeriod("r.recognized_at")}
	list.when(query, "product_id", "r.product_id = @product_id::uuid")
	list.when(query, "category_id", "r.category_id = @category_id::uuid")
	return list
}

var marginColumns = []Column{
	described(countColumn("units", "Unidades"), "Unidades comerciales vendidas (reconocidas) en el periodo."),
	described(moneyColumn("net", "Venta neta"), "Con IVA, después de promociones y descuentos por partida."),
	described(moneyColumn("base", "Venta sin IVA"), "Venta neta menos su IVA."),
	optional(described(moneyColumn("costed_base", "Venta sin IVA con costo"), "Venta sin IVA de las unidades con costo firme o estimado.")),
	described(moneyColumn("cost", "Costo de ingredientes"), "Costo de los consumos reales de las unidades con costo."),
	described(moneyColumn("margin", "Margen estimado"), "Venta sin IVA con costo − costo de ingredientes. No es utilidad contable."),
	percentColumn("margin_percent", "Margen %"),
	described(percentColumn("firm_share", "% de la venta con costo firme"), "Venta sin IVA de las unidades con costo firme ÷ venta sin IVA."),
	optional(countColumn("firm_units", "Unidades con costo firme")),
	optional(countColumn("estimated_units", "Unidades con costo estimado")),
	described(countColumn("uncosted_units", "Unidades sin costo"), "No entran al margen."),
	textColumn("status", "Estado del costo"),
}

// marginTotals completa margen, porcentajes y estado de cada renglón y del total.
func marginTotals(rows []Row, totals Row) {
	firm := decimal.Zero
	for _, row := range rows {
		completeMargin(row)
		firm = firm.Add(decimalOf(row, "firm_base"))
	}
	totals["firm_base"] = firm
	completeMargin(totals)
}

func completeMargin(row Row) {
	costed := decimalOf(row, "costed_base")
	margin := costed.Sub(decimalOf(row, "cost"))
	row["margin"] = margin
	row["margin_percent"] = percentOf(margin, costed)
	row["firm_share"] = percentOf(decimalOf(row, "firm_base"), decimalOf(row, "base"))
	row["status"] = marginStatus(row)
}

// marginStatus resume el estado del costo de un grupo de unidades.
func marginStatus(row Row) string {
	firm, _ := row["firm_units"].(int64)
	estimated, _ := row["estimated_units"].(int64)
	uncosted, _ := row["uncosted_units"].(int64)
	switch {
	case firm == 0 && estimated == 0 && uncosted == 0:
		return "Sin ventas"
	case firm == 0 && estimated == 0:
		return "Sin costo"
	case estimated == 0 && uncosted == 0:
		return "Firme"
	case estimated == 0:
		return "Firme parcial (hay unidades sin costo)"
	case uncosted > 0:
		return "Estimado (hay unidades sin costo)"
	}
	return "Estimado"
}

// marginDataset agrega los avisos y las cifras del periodo: margen total, porcentaje con costo
// firme, ajustes de costo (concepto del periodo) y descuentos y cargos generales de cuenta.
func marginDataset(ctx context.Context, query *Query, rows []Row) (*Dataset, error) {
	dataset, err := inventoryDataset(ctx, query, rows)
	if err != nil {
		return nil, err
	}
	var adjustments, accountAdjustments string
	if err := query.Tx.QueryRow(ctx, `SELECT
            (SELECT coalesce(sum(m.value),0) FROM app.inventory_movements m WHERE m.kind='cost_adjustment' AND `+inPeriod("m.occurred_at")+`)::text,
            (SELECT coalesce(sum(r.base),0) FROM app.sales_recognitions r WHERE r.source_kind='adjustment' AND `+inPeriod("r.recognized_at")+`)::text`,
		query.Args()).Scan(&adjustments, &accountAdjustments); err != nil {
		return nil, err
	}
	dataset.Summary = append(dataset.Summary,
		Figure{Key: "cost_adjustments", Label: "Ajustes de costo del periodo", Type: TypeMoneyMXN,
			Value: decimal.RequireFromString(adjustments),
			Hint:  "Diferencias de costo por existencias negativas y devoluciones (positivo aumenta el valor del inventario). No se reparten entre productos."},
		Figure{Key: "account_adjustments", Label: "Descuentos y cargos generales de cuenta (sin IVA)", Type: TypeMoneyMXN,
			Value: decimal.RequireFromString(accountAdjustments), Hint: "No pertenecen a un producto: no entran al margen por producto."})
	dataset.Notes = append(dataset.Notes, "Margen bruto estimado sobre el costo de ingredientes consumidos: no incluye gastos "+
		"generales, nómina ni impuestos y no es utilidad contable.")
	return dataset, nil
}

func init() {
	register(Definition{
		ID: "costs.margin_by_product", Group: "costs", Title: "Margen estimado por producto",
		Description: "Venta sin IVA, costo real de los consumos y margen estimado de cada producto, con el estado de su costo.",
		Permissions: costPermissions(), DateBasis: marginDateBasis, Filters: []Filter{filterCategory(), filterProduct()},
		Columns:     withColumns([]Column{textColumn("product", "Producto"), optional(textColumn("category", "Categoría"))}, marginColumns),
		DefaultSort: Sort{Key: "base", Descending: true}, Totals: marginTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, marginBaseSQL(marginConditions(query))+`
                SELECT `+latest("r.product_name")+` AS product,`+latest("r.category_name")+` AS category,`+marginMeasures+`
                FROM recognized r JOIN status st ON st.unit_id=r.unit_id GROUP BY r.product_id`, query.Args())
			if err != nil {
				return nil, err
			}
			return marginDataset(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "costs.margin_by_category", Group: "costs", Title: "Margen estimado por categoría",
		Description: "Venta sin IVA, costo real de los consumos y margen estimado por categoría de producto (costo por categoría).",
		Permissions: costPermissions(), DateBasis: marginDateBasis, Filters: []Filter{filterCategory()},
		Columns:     withColumns([]Column{textColumn("category", "Categoría")}, marginColumns),
		DefaultSort: Sort{Key: "base", Descending: true}, Totals: marginTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, marginBaseSQL(marginConditions(query))+`
                SELECT coalesce(`+latest("r.category_name")+`,'Sin categoría') AS category,`+marginMeasures+`
                FROM recognized r JOIN status st ON st.unit_id=r.unit_id GROUP BY r.category_id`, query.Args())
			if err != nil {
				return nil, err
			}
			return marginDataset(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "costs.margin_by_period", Group: "costs", Title: "Margen estimado por periodo",
		Description: "Margen estimado de cada día comercial, con los ajustes de costo del día como concepto aparte.",
		Permissions: costPermissions(), DateBasis: marginDateBasis, Filters: []Filter{filterCategory(), filterProduct()},
		Columns: withColumns([]Column{dateColumn("day", "Día")}, marginColumns, []Column{
			described(moneyColumn("cost_adjustments", "Ajustes de costo"),
				"Ajustes de costo registrados en el día (concepto del periodo, no se restan del margen)."),
		}),
		DefaultSort: Sort{Key: "day"}, Totals: marginTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, marginBaseSQL(marginConditions(query))+`,
                margins AS (
                    SELECT `+businessDate("r.recognized_at")+` AS day,`+marginMeasures+`
                    FROM recognized r JOIN status st ON st.unit_id=r.unit_id GROUP BY 1
                ),
                adjustments AS (
                    SELECT `+businessDate("m.occurred_at")+` AS day,sum(m.value) AS cost_adjustments FROM app.inventory_movements m
                    WHERE m.kind='cost_adjustment' AND `+inPeriod("m.occurred_at")+` GROUP BY 1
                )
                SELECT coalesce(g.day,a.day) AS day,coalesce(g.units,0) AS units,coalesce(g.net,0) AS net,coalesce(g.base,0) AS base,
                    coalesce(g.costed_base,0) AS costed_base,coalesce(g.cost,0) AS cost,coalesce(g.firm_base,0) AS firm_base,
                    coalesce(g.firm_units,0) AS firm_units,coalesce(g.estimated_units,0) AS estimated_units,
                    coalesce(g.uncosted_units,0) AS uncosted_units,coalesce(a.cost_adjustments,0) AS cost_adjustments
                FROM margins g FULL JOIN adjustments a ON a.day=g.day ORDER BY 1`, query.Args())
			if err != nil {
				return nil, err
			}
			return marginDataset(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "costs.cost_status", Group: "costs", Title: "Estado del costo de las ventas",
		Description: "Unidades vendidas sin costo o con costo estimado por producto, con el dato que falta para un costo firme.",
		Permissions: costPermissions(), DateBasis: marginDateBasis,
		Filters: []Filter{filterCategory(), filterProduct(), filterChoice("state", "Estado", "",
			Option{Value: "firm", Label: "Firme"}, Option{Value: "estimated", Label: "Estimado"}, Option{Value: "uncosted", Label: "Sin costo"})},
		Columns: []Column{textColumn("product", "Producto"), textColumn("status", "Estado del costo"), textColumn("reason", "Qué falta"),
			countColumn("units", "Unidades"), described(moneyColumn("base", "Venta sin IVA"), "Venta sin IVA de esas unidades."),
			described(moneyColumn("cost", "Costo registrado"), "Costo de sus consumos reales (cero sin costo).")},
		DefaultSort: Sort{Key: "product"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			states := map[string]string{"firm": "'firm'", "estimated": "'provisional','incomplete'",
				"uncosted": "'pending','not_released','no_recipe'"}
			filter := ""
			if state := query.Params.Value("state"); state != "" {
				filter = " WHERE st.status IN (" + states[state] + ")"
			}
			rows, err := query.collect(ctx, marginBaseSQL(marginConditions(query))+`
                SELECT `+latest("r.product_name")+` AS product,st.status AS status_code,coalesce(sum(r.quantity),0)::bigint AS units,
                    coalesce(sum(r.base),0) AS base,coalesce(sum(r.quantity*st.cost),0) AS cost
                FROM recognized r JOIN status st ON st.unit_id=r.unit_id`+filter+` GROUP BY r.product_id,st.status`, query.Args())
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				code, _ := row["status_code"].(string)
				row["status"] = costStatusLabels[code]
				row["reason"] = costReasonLabels[code]
			}
			return inventoryDataset(ctx, query, rows)
		},
	})
	registerCostDetails()
}

func registerCostDetails() {
	register(Definition{
		ID: "costs.theoretical_products", Group: "costs", Title: "Costo teórico por producto",
		Description: "Costo teórico vigente de una unidad de cada producto o variante con receta (motor de recetas), frente a su precio sin IVA.",
		Permissions: costPermissions(), Period: PeriodNone, DateBasis: "Recetas y costos vigentes (sin periodo).",
		Filters: []Filter{filterCategory(), filterProduct()},
		Columns: []Column{textColumn("product", "Producto"), textColumn("variant", "Variante"), optional(textColumn("recipe", "Receta")),
			optional(countColumn("version", "Versión")),
			described(moneyColumn("price", "Precio vigente"), "Precio de lista del catálogo, con IVA."),
			described(moneyColumn("price_base", "Precio sin IVA"), "Con la tasa vigente del producto."),
			described(moneyColumn("unit_cost", "Costo teórico"), "Receta ÷ rendimiento + porciones incluidas, al costo vigente."),
			described(optional(moneyColumn("takeaway_cost", "Costo adicional para llevar")), "Renglones «solo para llevar»."),
			described(moneyColumn("margin", "Margen teórico"), "Precio sin IVA − costo teórico; no considera promociones ni descuentos."),
			percentColumn("margin_percent", "Margen teórico %"), textColumn("status", "Estado")},
		DefaultSort: Sort{Key: "product"},
		Totals: func(rows []Row, totals Row) {
			for _, row := range rows {
				row["margin_percent"] = percentOf(decimalOf(row, "margin"), decimalOf(row, "price_base"))
			}
			totals["margin_percent"] = percentOf(decimalOf(totals, "margin"), decimalOf(totals, "price_base"))
		},
		Query: theoreticalProductsQuery,
	})
	register(Definition{
		ID: "costs.theoretical_recipes", Group: "costs", Title: "Costo teórico por receta",
		Description: "Costo teórico vigente de cada receta de producto, complemento y preparación.",
		Permissions: costPermissions(), Period: PeriodNone, DateBasis: "Recetas y costos vigentes (sin periodo).",
		Filters: []Filter{filterChoice("kind", "Tipo de receta", "", Option{Value: "product", Label: "Productos"},
			Option{Value: "complement", Label: "Complementos"}, Option{Value: "preparation", Label: "Preparaciones"})},
		Columns: []Column{textColumn("recipe", "Receta"), textColumn("kind", "Tipo"), textColumn("active", "Activa"),
			countColumn("version", "Versión"), quantityColumn("yield", "Rendimiento (unidad base)"),
			described(moneyColumn("batch_cost", "Costo de la receta"), "Costo de sus renglones (una tanda)."),
			described(Column{Key: "unit_cost", Label: "Costo unitario", Type: TypeQuantity, Sortable: true},
				"Por unidad vendida (producto o complemento) o por unidad base del rendimiento (preparación)."),
			textColumn("status", "Estado")},
		DefaultSort: Sort{Key: "recipe"},
		Query:       theoreticalRecipesQuery,
	})
	register(Definition{
		ID: "costs.consumption_valued", Group: "costs", Title: "Consumo valuado",
		Description: "Costo de los consumos del periodo por categoría de insumo e insumo (costo por categoría de ingrediente).",
		Permissions: costPermissions(), DateBasis: consumptionDateBasis, Filters: []Filter{filterInventoryCategory(), filterItem(), filterWarehouse()},
		Columns: []Column{columnItemCategory, columnItemCode, columnItemName, columnItemUnit,
			described(quantityColumn("quantity", "Consumo"), "Consumo neto en la unidad base."),
			described(moneyColumn("value", "Costo del consumo"), "Valor neto de los movimientos de consumo."),
			percentColumn("share", "% del costo")},
		DefaultSort: Sort{Key: "value", Descending: true},
		Totals: func(rows []Row, totals Row) {
			total := decimalOf(totals, "value")
			for _, row := range rows {
				row["share"] = percentOf(decimalOf(row, "value"), total)
			}
			totals["share"] = percentOf(total, total)
		},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, `SELECT (array_agg(coalesce(category.name,'Sin categoría')))[1] AS category,i.code,
                    (array_agg(i.name))[1] AS item,(array_agg(unit.code))[1] AS unit,-sum(m.quantity) AS quantity,-sum(m.value) AS value
                FROM app.inventory_movements m `+movementJoins+`
                WHERE true`+consumptionConditions(query).sql()+` GROUP BY m.item_id,i.code ORDER BY 1,3`, query.Args())
			if err != nil {
				return nil, err
			}
			return inventoryDataset(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "costs.waste", Group: "costs", Title: "Costo de mermas",
		Description: "Costo de las mermas operativas por motivo y de las cancelaciones «Preparado / merma» del periodo.",
		Permissions: costPermissions(), DateBasis: "Hora de la merma operativa o de la cancelación de la partida, con la hora de inicio del día.",
		Filters: []Filter{filterWarehouse()},
		Columns: []Column{textColumn("origin", "Origen"), textColumn("classification", "Clasificación"), countColumn("records", "Registros"),
			described(moneyColumn("value", "Costo"), "Valor de los movimientos de la merma.")},
		DefaultSort: Sort{Key: "value", Descending: true},
		Query:       wasteCostQuery,
	})
	register(Definition{
		ID: "costs.cost_adjustments", Group: "costs", Title: "Ajustes de costo",
		Description: "Ajustes de costo del periodo (diferencias por existencias negativas y devoluciones a proveedor) como concepto del periodo.",
		Permissions: costPermissions(), DateBasis: inventoryMovementsDateBasis, Filters: []Filter{filterItem(), filterInventoryCategory()},
		Columns: []Column{dateTimeColumn("occurred_at", "Fecha"), columnItemCode, columnItemName, textColumn("source", "Origen"),
			optional(textColumn("related", "Movimiento relacionado")),
			described(moneyColumn("value", "Ajuste"), "Con signo: positivo aumenta el valor del inventario."), optional(textColumn("reason", "Motivo"))},
		DefaultSort: Sort{Key: "occurred_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := conditions{"m.kind = 'cost_adjustment'", inPeriod("m.occurred_at")}
			list.when(query, "item_id", "m.item_id = @item_id::uuid")
			list.when(query, "category_id", "i.category_id = @category_id::uuid")
			rows, err := query.collect(ctx, `SELECT m.occurred_at,i.code,i.name AS item,m.source_kind,coalesce(related.kind,'') AS related_kind,
                    m.value,coalesce(m.reason,'') AS reason
                FROM app.inventory_movements m JOIN app.inventory_items i ON i.id=m.item_id
                LEFT JOIN app.inventory_movements related ON related.id=m.related_movement_id
                WHERE true`+list.sql()+` ORDER BY m.occurred_at,m.seq`, query.Args())
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				row["source"] = inventory.SourceKindLabel(row["source_kind"].(string))
				if related, _ := row["related_kind"].(string); related != "" {
					row["related"] = inventory.MovementKindLabel(related)
				}
			}
			dataset, err := inventoryDataset(ctx, query, rows)
			if err != nil {
				return nil, err
			}
			dataset.Notes = append(dataset.Notes, "Los ajustes de costo no se reparten entre productos ni forman parte del costo de una venta.")
			return dataset, nil
		},
	})
}

func theoreticalProductsQuery(ctx context.Context, query *Query) (*Dataset, error) {
	costs, err := inventory.TheoreticalCosts(ctx, query.Tx)
	if err != nil {
		return nil, err
	}
	list := conditions{}
	list.when(query, "product_id", "p.id = @product_id::uuid")
	list.when(query, "category_id", "p.category_id = @category_id::uuid")
	type catalogEntry struct {
		product, variant string
		price            decimal.Decimal
		rate             int
	}
	catalog := map[string]catalogEntry{}
	rows, err := query.Tx.Query(ctx, `SELECT p.id::text||'/'||coalesce(v.id::text,''),p.name,coalesce(v.name,''),
            coalesce(v.price,p.base_price)::text,coalesce(p.tax_rate,settings.tax_rate,16)
        FROM app.catalog_products p
        JOIN app.catalog_variants v ON v.product_id=p.id
        LEFT JOIN app.payment_settings settings ON true
        WHERE true`+list.sql()+`
        UNION ALL
        SELECT p.id::text||'/',p.name,'',p.base_price::text,coalesce(p.tax_rate,settings.tax_rate,16)
        FROM app.catalog_products p LEFT JOIN app.payment_settings settings ON true WHERE true`+list.sql(), query.Args())
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key, price string
		var entry catalogEntry
		if err := rows.Scan(&key, &entry.product, &entry.variant, &price, &entry.rate); err != nil {
			rows.Close()
			return nil, err
		}
		entry.price = decimal.RequireFromString(price)
		catalog[key] = entry
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := []Row{}
	for _, cost := range costs {
		if cost.OwnerKind != "product" || cost.ProductID == nil || cost.UnitCost == nil {
			continue
		}
		key := *cost.ProductID + "/"
		if cost.VariantID != nil {
			key += *cost.VariantID
		}
		entry, found := catalog[key]
		if !found {
			continue
		}
		base := entry.price.DivRound(decimal.NewFromInt(int64(100+entry.rate)).Div(decimal.NewFromInt(100)), 8).Round(2)
		status := "Completo"
		if cost.Pending {
			status = "Incompleto: algún insumo sin costo"
		}
		if !cost.Active {
			status = "Receta inactiva"
		}
		variant := entry.variant
		if variant == "" {
			variant = "—"
		}
		result = append(result, Row{
			"product": entry.product, "variant": variant, "recipe": cost.Name, "version": int64(cost.Version), "price": entry.price,
			"price_base": base, "unit_cost": *cost.UnitCost, "takeaway_cost": cost.TakeawayCost, "margin": base.Sub(*cost.UnitCost),
			"status": status,
		})
	}
	dataset := &Dataset{Rows: result}
	dataset.Notes = append(dataset.Notes, "Costo teórico con los costos vigentes de hoy (no histórico); no incluye complementos "+
		"elegidos ni componentes de combo. El costo real de lo vendido está en «Margen estimado».")
	return dataset, nil
}

var recipeKindLabels = map[string]string{"product": "Producto", "complement": "Complemento", "preparation": "Preparación"}

func theoreticalRecipesQuery(ctx context.Context, query *Query) (*Dataset, error) {
	costs, err := inventory.TheoreticalCosts(ctx, query.Tx)
	if err != nil {
		return nil, err
	}
	kind := query.Params.Value("kind")
	rows := []Row{}
	for _, cost := range costs {
		if kind != "" && cost.OwnerKind != kind {
			continue
		}
		row := Row{"recipe": cost.Name, "kind": recipeKindLabels[cost.OwnerKind], "active": "Sí", "version": int64(cost.Version),
			"yield": cost.YieldQuantity, "batch_cost": cost.BatchCost, "status": "Completo"}
		if !cost.Active {
			row["active"] = "No"
		}
		if cost.UnitCost != nil {
			row["unit_cost"] = cost.UnitCost.Round(unitCostDisplayScale)
		}
		if cost.Pending {
			row["status"] = "Incompleto: algún insumo sin costo"
		}
		rows = append(rows, row)
	}
	return &Dataset{Rows: rows, Notes: []string{"Costos vigentes de hoy; el historial de versiones no lleva costos."}}, nil
}

func wasteCostQuery(ctx context.Context, query *Query) (*Dataset, error) {
	warehouse := ""
	if query.Has("warehouse_id") {
		warehouse = " AND m.warehouse_id = @warehouse_id::uuid"
	}
	rows, err := query.collect(ctx, `SELECT 'Merma operativa' AS origin,a.reason AS reason_code,''::text AS reason_detail,
            count(DISTINCT a.id)::bigint AS records,-sum(m.value) AS value
        FROM app.inventory_movements m
        JOIN app.inventory_adjustment_lines l ON l.movement_id=m.id
        JOIN app.inventory_adjustments a ON a.id=l.adjustment_id
        WHERE m.kind='waste' AND `+inPeriod("m.occurred_at")+warehouse+` GROUP BY a.reason
        UNION ALL
        SELECT 'Cancelación de partida','prepared_waste','',count(DISTINCT c.id)::bigint,coalesce(-sum(m.value),0)
        FROM app.inventory_cancellation_classifications c
        JOIN app.inventory_movements m ON m.restaurant_unit_id=c.restaurant_unit_id AND m.kind IN ('consumption','consumption_reversal')
        WHERE c.state='prepared_waste' AND `+inPeriod("c.cancelled_at")+warehouse+` HAVING count(*) > 0`, query.Args())
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
	dataset.Notes = append(dataset.Notes, "Una devolución de dinero nunca es merma.")
	return dataset, nil
}

// marginFigure resume el margen del periodo para el resumen ejecutivo: nil si no hay datos
// suficientes (ninguna unidad vendida con costo).
func marginFigure(ctx context.Context, query *Query, args pgx.NamedArgs) (map[string]any, error) {
	var units, costedUnits int64
	var base, costed, cost, firm string
	err := query.Tx.QueryRow(ctx, marginBaseSQL(conditions{inPeriod("r.recognized_at")})+`
        SELECT coalesce(sum(r.quantity),0)::bigint,
            coalesce(sum(r.quantity) FILTER (WHERE st.status IN ('firm','provisional','incomplete')),0)::bigint,
            coalesce(sum(r.base),0)::text,
            coalesce(sum(r.base) FILTER (WHERE st.status IN ('firm','provisional','incomplete')),0)::text,
            coalesce(sum(r.quantity*st.cost) FILTER (WHERE st.status IN ('firm','provisional','incomplete')),0)::text,
            coalesce(sum(r.base) FILTER (WHERE st.status='firm'),0)::text
        FROM recognized r JOIN status st ON st.unit_id=r.unit_id`, args).Scan(&units, &costedUnits, &base, &costed, &cost, &firm)
	if err != nil {
		return nil, err
	}
	values := map[string]any{"margin": nil, "margin_percent": nil, "firm_share": nil, "cost_of_sales": nil,
		"uncosted_units": units - costedUnits}
	if costedUnits <= 0 {
		return values, nil
	}
	costedBase := decimal.RequireFromString(costed)
	margin := costedBase.Sub(decimal.RequireFromString(cost))
	values["margin"] = margin
	values["cost_of_sales"] = decimal.RequireFromString(cost)
	values["margin_percent"] = percentOf(margin, costedBase)
	values["firm_share"] = percentOf(decimal.RequireFromString(firm), decimal.RequireFromString(base))
	return values, nil
}
