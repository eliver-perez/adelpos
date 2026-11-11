package reports

import (
	"context"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/businessday"
)

// Reportes de ventas (§8). Fuente única: el libro app.sales_recognitions (migración 0017), con la
// fecha de reconocimiento (cierre de la cuenta) filtrada por instantes [desde, hasta) del periodo y
// agrupada por día comercial con la hora de inicio del día. Cada unidad se reconoce una vez aunque
// haya pasado por varias cuentas (división, transferencia o combinación) y un combo cuenta como la
// unidad vendida, sin sumar sus componentes. Las devoluciones no restan aquí: se reportan en su día
// con los cobros. Las propinas no son venta.

const salesDateBasis = "Fecha de cierre de la cuenta (reconocimiento de la venta), con la hora de inicio del día."

// ledgerMeasures son las medidas de venta de un grupo de renglones del libro r.
const ledgerMeasures = `count(DISTINCT r.account_id) FILTER (WHERE r.recognition_kind <> 'resettlement') AS accounts,
    coalesce(sum(r.quantity),0)::bigint AS quantity,coalesce(sum(r.gross),0) AS gross,coalesce(sum(r.promotion),0) AS promotion,
    coalesce(sum(r.discount),0) AS discount,coalesce(sum(r.courtesy),0) AS courtesy,coalesce(sum(r.charge),0) AS charge,
    coalesce(sum(r.net),0) AS net,coalesce(sum(r.base),0) AS base,coalesce(sum(r.tax),0) AS tax`

// salesFilters son los filtros comunes de los reportes de ventas.
func salesFilters() []Filter {
	return []Filter{filterMode(), filterArea(), filterTable(), filterCategory(), filterProduct(), filterPriceList(), filterOrigin()}
}

// ledgerConditions son las condiciones del periodo y de los filtros indicados sobre el libro r. Con
// un filtro de producto o categoría solo quedan los renglones de productos (los descuentos generales y
// los cargos de cuenta no pertenecen a un producto).
func ledgerConditions(query *Query) conditions {
	list := conditions{inPeriod("r.recognized_at")}
	list.when(query, "mode", "r.mode = @mode")
	list.when(query, "area_id", "r.area_id = @area_id::uuid")
	list.when(query, "table_id", "r.table_id = @table_id::uuid")
	list.when(query, "category_id", "r.category_id = @category_id::uuid")
	list.when(query, "product_id", "r.product_id = @product_id::uuid")
	list.when(query, "price_list_id", "r.price_list_id = @price_list_id::uuid")
	list.when(query, "origin", "r.origin = @origin")
	list.when(query, "register_id", "r.register_id = @register_id::uuid")
	return list
}

// latest elige el último valor registrado de un dato histórico del grupo (el nombre vigente al
// último reconocimiento del periodo).
func latest(column string) string {
	return "(array_agg(" + column + " ORDER BY r.sequence DESC) FILTER (WHERE " + column + " IS NOT NULL))[1]"
}

func productNotes(query *Query) []string {
	if query.Has("product_id") || query.Has("category_id") {
		return []string{"Con filtro de producto o categoría no se incluyen los descuentos generales ni los cargos de cuenta, que no pertenecen a un producto."}
	}
	return nil
}

// salesFigures son las cifras resumidas comunes: cuentas cerradas, venta neta, IVA y ticket promedio.
func salesFigures(ctx context.Context, query *Query) ([]Figure, error) {
	return salesFiguresWhere(ctx, query, ledgerConditions(query))
}

// salesFiguresWhere calcula las cifras resumidas sobre los renglones del libro que cumplen list (las
// mismas condiciones que los renglones del reporte).
func salesFiguresWhere(ctx context.Context, query *Query, list conditions) ([]Figure, error) {
	var accounts, quantity int64
	var net, tax decimal.Decimal
	var netText, taxText string
	err := query.Tx.QueryRow(ctx, `SELECT count(DISTINCT r.account_id) FILTER (WHERE r.recognition_kind <> 'resettlement'),
            coalesce(sum(r.net),0)::text,coalesce(sum(r.tax),0)::text,coalesce(sum(r.quantity),0)
        FROM app.sales_recognitions r WHERE true`+list.sql(), query.Args()).Scan(&accounts, &netText, &taxText, &quantity)
	if err != nil {
		return nil, err
	}
	net = decimal.RequireFromString(netText)
	tax = decimal.RequireFromString(taxText)
	figures := []Figure{
		{Key: "accounts", Label: "Cuentas cerradas", Type: TypeInteger, Value: accounts},
		{Key: "net", Label: "Venta neta", Type: TypeMoneyMXN, Value: net},
		{Key: "tax", Label: "IVA incluido", Type: TypeMoneyMXN, Value: tax},
		{Key: "quantity", Label: "Unidades vendidas", Type: TypeInteger, Value: quantity},
	}
	if accounts > 0 {
		figures = append(figures, Figure{Key: "average_ticket", Label: "Ticket promedio", Type: TypeMoneyMXN,
			Value: net.DivRound(decimal.NewFromInt(accounts), 2)})
	}
	return figures, nil
}

// ledgerReport es una consulta agrupada del libro: select y group son las expresiones de la
// dimensión, extra son condiciones adicionales y order el orden de la consulta.
func ledgerReport(ctx context.Context, query *Query, selectSQL, groupSQL, extra, order string) ([]Row, error) {
	list := ledgerConditions(query)
	if extra != "" {
		list.add(extra)
	}
	sql := "SELECT " + selectSQL + "," + ledgerMeasures + " FROM app.sales_recognitions r WHERE true" + list.sql()
	if groupSQL != "" {
		sql += " GROUP BY " + groupSQL
	}
	if order != "" {
		sql += " ORDER BY " + order
	}
	return query.collect(ctx, sql, query.Args())
}

// withFigures completa un conjunto con las cifras de venta y las notas de producto.
func withFigures(ctx context.Context, query *Query, rows []Row) (*Dataset, error) {
	figures, err := salesFigures(ctx, query)
	if err != nil {
		return nil, err
	}
	return &Dataset{Rows: rows, Summary: figures, Notes: productNotes(query)}, nil
}

func init() {
	register(Definition{
		ID: "sales.summary", Group: "sales", Title: "Ventas del periodo",
		Description: "Venta neta del periodo por tasa de IVA, con subtotal sin IVA, IVA y total.",
		Permissions: []string{PermissionSales}, Comparison: false, DateBasis: salesDateBasis, Filters: salesFilters(),
		Columns:     withColumns([]Column{textColumn("rate", "Tasa de IVA")}, salesMeasureColumns()),
		DefaultSort: Sort{},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, "r.tax_rate::text||' %' AS rate", "r.tax_rate", "", "r.tax_rate DESC")
			if err != nil {
				return nil, err
			}
			return withFigures(ctx, query, rows)
		},
	})
	dayColumns := withColumns([]Column{dateColumn("day", "Día"), columnAccounts}, salesMeasureColumns(),
		[]Column{columnAverageTicket})
	register(Definition{
		ID: "sales.by_day", Group: "sales", Title: "Ventas por día",
		Description: "Venta de cada día comercial del periodo, incluidos los días sin ventas.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(), Columns: dayColumns,
		DefaultSort: Sort{Key: "day"}, Totals: averageTicketTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, businessDate("r.recognized_at")+" AS day", "1", "", "1")
			if err != nil {
				return nil, err
			}
			return withFigures(ctx, query, fillDays(rows, query.Params.Period, dayColumns))
		},
	})
	register(Definition{
		ID: "sales.by_week", Group: "sales", Title: "Ventas por semana",
		Description: "Venta por semana del año (ISO 8601, de lunes a domingo).",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(),
		Columns: withColumns([]Column{codeColumn("week", "Semana"), dateColumn("week_start", "Del"), dateColumn("week_end", "Al"),
			columnAccounts}, salesMeasureColumns(), []Column{columnAverageTicket}),
		DefaultSort: Sort{Key: "week_start"}, Totals: averageTicketTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			day := businessDate("r.recognized_at")
			rows, err := ledgerReport(ctx, query, "to_char("+day+",'IYYY-\"W\"IW') AS week,date_trunc('week',"+day+")::date AS week_start,"+
				"(date_trunc('week',"+day+")::date+6) AS week_end", "1,2,3", "", "2")
			if err != nil {
				return nil, err
			}
			return withFigures(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "sales.by_month", Group: "sales", Title: "Ventas por mes",
		Description: "Venta por mes calendario de días comerciales.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(),
		Columns: withColumns([]Column{codeColumn("month", "Mes"), textColumn("month_label", "Nombre del mes"), columnAccounts},
			salesMeasureColumns(), []Column{columnAverageTicket}),
		DefaultSort: Sort{Key: "month"}, Totals: averageTicketTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, "to_char("+businessDate("r.recognized_at")+",'YYYY-MM') AS month", "1", "", "1")
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				row["month_label"] = monthLabel(row["month"])
			}
			return withFigures(ctx, query, rows)
		},
	})
	hourColumns := []Column{textColumn("hour", "Hora"), columnAccounts, columnQuantity, columnNet, columnShare}
	register(Definition{
		ID: "sales.by_hour", Group: "sales", Title: "Ventas por hora",
		Description: "Venta por hora del día (hora local del cierre), en el orden del día comercial.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(), Columns: hourColumns,
		Totals: shareTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, localHour("r.recognized_at")+" AS hour_number", "1", "", "1")
			if err != nil {
				return nil, err
			}
			return withFigures(ctx, query, fillHours(rows, query.Config, hourColumns))
		},
	})
	register(Definition{
		ID: "sales.by_category", Group: "categories", Title: "Ventas por categoría",
		Description: "Cantidades e ingresos por categoría, con el nombre registrado al vender.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(),
		Columns:     withColumns([]Column{textColumn("category", "Categoría")}, salesMeasureColumns(), []Column{columnShare}),
		DefaultSort: Sort{Key: "net", Descending: true}, Totals: shareTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, "coalesce("+latest("r.category_name")+",'Sin categoría') AS category",
				"r.category_id", "r.product_id IS NOT NULL", "")
			if err != nil {
				return nil, err
			}
			return productDataset(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "sales.by_product", Group: "sales", Title: "Ventas por producto",
		Description: "Cantidades e ingresos por producto, con el nombre y la categoría registrados al vender.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(),
		Columns: withColumns([]Column{codeColumn("code", "Código"), textColumn("product", "Producto"),
			textColumn("category", "Categoría")}, salesMeasureColumns(), []Column{columnShare}),
		DefaultSort: Sort{Key: "net", Descending: true}, Totals: shareTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, latest("r.product_code")+" AS code,"+latest("r.product_name")+" AS product,"+
				"coalesce("+latest("r.category_name")+",'Sin categoría') AS category", "r.product_id", "r.product_id IS NOT NULL", "")
			if err != nil {
				return nil, err
			}
			return productDataset(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "sales.by_variant", Group: "sales", Title: "Ventas por variante",
		Description: "Cantidades e ingresos por producto y variante.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(),
		Columns: withColumns([]Column{codeColumn("code", "Código"), textColumn("product", "Producto"),
			textColumn("variant", "Variante")}, salesMeasureColumns(), []Column{columnShare}),
		DefaultSort: Sort{Key: "net", Descending: true}, Totals: shareTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, latest("r.product_code")+" AS code,"+latest("r.product_name")+" AS product,"+
				"coalesce("+latest("r.variant_name")+",'Sin variante') AS variant", "r.product_id,r.variant_id", "r.product_id IS NOT NULL", "")
			if err != nil {
				return nil, err
			}
			return productDataset(ctx, query, rows)
		},
	})
	modeColumns := withColumns([]Column{textColumn("mode", "Modalidad"), columnAccounts}, salesMeasureColumns(),
		[]Column{columnAverageTicket, columnShare})
	register(Definition{
		ID: "sales.by_mode", Group: "sales", Title: "Ventas por modalidad",
		Description: "Venta de salón y de para llevar o ventanilla.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(), Columns: modeColumns,
		DefaultSort: Sort{Key: "net", Descending: true}, Totals: combineTotals(averageTicketTotals, shareTotals),
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, "CASE r.mode WHEN 'dine_in' THEN 'Salón' ELSE 'Para llevar y ventanilla' END AS mode",
				"r.mode", "", "")
			if err != nil {
				return nil, err
			}
			return withFigures(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "sales.dine_in", Group: "sales", Title: "Ventas de salón",
		Description: "Venta de salón por área y mesa (la mesa de la cuenta al cerrarse).",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis,
		Filters: []Filter{filterArea(), filterTable(), filterCategory(), filterProduct(), filterPriceList()},
		Columns: withColumns([]Column{textColumn("area", "Área"), textColumn("table", "Mesa"), columnAccounts}, salesMeasureColumns(),
			[]Column{columnAverageTicket, columnShare}),
		DefaultSort: Sort{Key: "net", Descending: true}, Totals: combineTotals(averageTicketTotals, shareTotals),
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, "coalesce("+latest("r.area_name")+",'Sin área') AS area,"+
				"coalesce("+latest("r.table_name")+",'Sin mesa') AS \"table\"", "r.area_id,r.table_id", "r.mode = 'dine_in'", "")
			if err != nil {
				return nil, err
			}
			return withFigures(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "sales.takeaway", Group: "sales", Title: "Ventas para llevar y ventanilla",
		Description: "Venta para llevar por origen (Para llevar, Mostrador, Teléfono, Ventanilla…).",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis,
		Filters: []Filter{filterOrigin(), filterCategory(), filterProduct(), filterPriceList()},
		Columns: withColumns([]Column{textColumn("origin", "Origen"), columnAccounts}, salesMeasureColumns(),
			[]Column{columnAverageTicket, columnShare}),
		DefaultSort: Sort{Key: "net", Descending: true}, Totals: combineTotals(averageTicketTotals, shareTotals),
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, "r.origin AS origin", "r.origin", "r.mode = 'takeaway'", "")
			if err != nil {
				return nil, err
			}
			return withFigures(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "sales.by_price_list", Group: "sales", Title: "Ventas por lista de precios",
		Description: "Venta por la lista de precios con que se confirmó cada pedido (los ajustes de cuenta, con la lista de la cuenta).",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: salesFilters(),
		Columns:     withColumns([]Column{textColumn("price_list", "Lista de precios")}, salesMeasureColumns(), []Column{columnShare}),
		DefaultSort: Sort{Key: "net", Descending: true}, Totals: shareTotals,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := ledgerReport(ctx, query, "coalesce("+latest("r.price_list_name")+",'Sin lista') AS price_list",
				"r.price_list_id", "", "")
			if err != nil {
				return nil, err
			}
			return withFigures(ctx, query, rows)
		},
	})
	register(Definition{
		ID: "sales.by_shift", Group: "sales", Title: "Ventas por turno",
		Description: "Venta reconocida en cada turno: la venta pertenece al turno de la operación que cerró la cuenta.",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis + " Turno de la operación que cerró la cuenta.",
		Filters: []Filter{filterRegister(), filterMode()},
		Columns: withColumns([]Column{textColumn("register", "Caja"), dateColumn("business_date", "Fecha comercial del turno"),
			textColumn("opened_by", "Abrió"), columnAccounts}, salesMeasureColumns(), []Column{columnAverageTicket}),
		DefaultSort: Sort{Key: "business_date"}, Totals: averageTicketTotals,
		Query: salesByShift,
	})
	register(Definition{
		ID: "sales.accounts", Group: "sales", Title: "Cuentas cerradas",
		Description: "Detalle de cada cierre reconocido en el periodo (las reliquidaciones aparecen aparte).",
		Permissions: []string{PermissionSales}, DateBasis: salesDateBasis, Filters: []Filter{filterMode(), filterArea(), filterTable()},
		Columns: []Column{codeColumn("folio", "Folio"), dateTimeColumn("recognized_at", "Cierre"), textColumn("kind", "Tipo"),
			textColumn("mode", "Modalidad"), textColumn("place", "Mesa u origen"), columnQuantity, columnGross, columnPromotion,
			columnDiscount, columnCourtesy, columnCharge, columnNet, columnTax},
		DefaultSort: Sort{Key: "recognized_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := ledgerConditions(query)
			rows, err := query.collect(ctx, `SELECT min(r.account_folio)::text AS folio,min(r.recognized_at) AS recognized_at,
                    CASE min(r.recognition_kind) WHEN 'initial' THEN 'Cierre' WHEN 'backfill' THEN 'Cierre (relleno inicial)'
                        ELSE 'Reliquidación' END AS kind,
                    CASE min(r.mode) WHEN 'dine_in' THEN 'Salón' ELSE 'Para llevar' END AS mode,
                    coalesce(min(r.table_name),min(r.origin)) AS place,
                    coalesce(sum(r.quantity),0)::bigint AS quantity,sum(r.gross) AS gross,sum(r.promotion) AS promotion,
                    sum(r.discount) AS discount,sum(r.courtesy) AS courtesy,sum(r.charge) AS charge,sum(r.net) AS net,sum(r.tax) AS tax
                FROM app.sales_recognitions r WHERE true`+list.sql()+`
                GROUP BY r.closing_id ORDER BY 2`, query.Args())
			if err != nil {
				return nil, err
			}
			return withFigures(ctx, query, rows)
		},
	})
	registerSalesEvolution()
}

// productDataset agrega la conciliación con los ajustes de cuenta que no pertenecen a productos.
func productDataset(ctx context.Context, query *Query, rows []Row) (*Dataset, error) {
	dataset, err := withFigures(ctx, query, rows)
	if err != nil {
		return nil, err
	}
	if query.Has("product_id") || query.Has("category_id") {
		return dataset, nil
	}
	var discounts, charges string
	list := ledgerConditions(query)
	list.add("r.source_kind = 'adjustment'")
	if err := query.Tx.QueryRow(ctx, `SELECT coalesce(sum(r.discount+r.courtesy),0)::text,coalesce(sum(r.charge),0)::text
        FROM app.sales_recognitions r WHERE true`+list.sql(), query.Args()).Scan(&discounts, &charges); err != nil {
		return nil, err
	}
	dataset.Summary = append(dataset.Summary,
		Figure{Key: "account_discounts", Label: "Descuentos y cortesías generales de cuenta", Type: TypeMoneyMXN,
			Value: decimal.RequireFromString(discounts), Hint: "No pertenecen a un producto; la venta neta del periodo ya los descuenta."},
		Figure{Key: "account_charges", Label: "Cargos generales de cuenta", Type: TypeMoneyMXN, Value: decimal.RequireFromString(charges)})
	return dataset, nil
}

func combineTotals(functions ...func(rows []Row, totals Row)) func(rows []Row, totals Row) {
	return func(rows []Row, totals Row) {
		for _, function := range functions {
			function(rows, totals)
		}
	}
}

func monthLabel(value any) any {
	text, ok := value.(string)
	if !ok || len(text) != 7 {
		return nil
	}
	year, yearErr := strconv.Atoi(text[:4])
	month, monthErr := strconv.Atoi(text[5:])
	if yearErr != nil || monthErr != nil || month < 1 || month > 12 {
		return nil
	}
	return businessday.MonthLabel(year, time.Month(month))
}

// fillDays agrega con ceros los días del periodo sin ventas (para tendencias sin huecos).
func fillDays(rows []Row, period *businessday.Period, columns []Column) []Row {
	if period == nil {
		return rows
	}
	byDay := map[string]Row{}
	for _, row := range rows {
		if day, ok := row["day"].(time.Time); ok {
			byDay[day.Format("2006-01-02")] = row
		}
	}
	result := make([]Row, 0, period.Days)
	for day := period.First(); !day.After(period.Last()); day = day.AddDate(0, 0, 1) {
		if row, found := byDay[day.Format("2006-01-02")]; found {
			result = append(result, row)
			continue
		}
		result = append(result, zeroMeasures(Row{"day": day}, columns))
	}
	return result
}

// fillHours devuelve las 24 horas en el orden del día comercial (desde la hora de inicio), con
// ceros en las horas sin ventas.
func fillHours(rows []Row, config businessday.Config, columns []Column) []Row {
	byHour := map[int64]Row{}
	for _, row := range rows {
		if hour, ok := row["hour_number"].(int64); ok {
			byHour[hour] = row
		}
	}
	result := make([]Row, 0, 24)
	for offset := 0; offset < 24; offset++ {
		hour := int64((config.Hour + offset) % 24)
		row, found := byHour[hour]
		if !found {
			row = zeroMeasures(Row{}, columns)
		}
		delete(row, "hour_number")
		row["hour"] = hourLabel(hour)
		result = append(result, row)
	}
	return result
}

func hourLabel(hour int64) string {
	text := strconv.FormatInt(hour, 10)
	if hour < 10 {
		text = "0" + text
	}
	return text + ":00 – " + text + ":59"
}

func salesByShift(ctx context.Context, query *Query) (*Dataset, error) {
	list := ledgerConditions(query)
	// Paridad con Caja: las ventas de un turno en arqueo no se muestran hasta que termine.
	list.add("(r.shift_id IS NULL OR NOT EXISTS(SELECT FROM app.cash_shifts closing WHERE closing.id=r.shift_id AND closing.state='closing'))")
	rows, err := query.collect(ctx, `SELECT coalesce(register.name,'Sin turno') AS register,
            coalesce(s.business_date,(s.opened_at AT TIME ZONE @tz)::date) AS business_date,opener.display_name AS opened_by,`+
		ledgerMeasures+`
        FROM app.sales_recognitions r
        LEFT JOIN app.cash_shifts s ON s.id=r.shift_id
        LEFT JOIN app.registers register ON register.id=s.register_id
        LEFT JOIN app.users opener ON opener.id=s.opened_by
        WHERE true`+list.sql()+`
        GROUP BY r.shift_id,register.name,s.business_date,s.opened_at,opener.display_name`, query.Args())
	if err != nil {
		return nil, err
	}
	// Las cifras resumidas excluyen los turnos en arqueo igual que los renglones.
	figures, err := salesFiguresWhere(ctx, query, list)
	if err != nil {
		return nil, err
	}
	dataset := &Dataset{Rows: rows, Summary: figures, Notes: productNotes(query)}
	warning, err := closingShiftsWarning(ctx, query)
	if err != nil {
		return nil, err
	}
	if warning != nil {
		dataset.Warnings = append(dataset.Warnings, *warning)
	}
	dataset.Notes = append(dataset.Notes,
		"«Sin turno»: cortesías o entregas que cerraron la cuenta sin un turno activo en la caja de la terminal, o cuentas del relleno inicial cerradas sin pago.")
	return dataset, nil
}
