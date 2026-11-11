package reports

import (
	"context"

	"local/ad-el-pos/server/internal/payments"
)

// Detalle de caja: operaciones de efectivo, cortes parciales y cierres, y operaciones por
// responsable. Mismo alcance que Caja y sin los turnos en arqueo (ver cash.go). Los descuentos no se
// atribuyen a los turnos ni a las cajas (readAdjustmentTotals no se replica): están en Descuentos y
// cortesías con la fecha del reconocimiento de su venta.

var cashOperationKinds = []Option{
	{Value: "income", Label: "Ingresos"},
	{Value: "withdrawal", Label: "Retiros"},
	{Value: "expense", Label: "Gastos"},
	{Value: "correction", Label: "Correcciones"},
	{Value: "regularization", Label: "Regularizaciones"},
}

var cashCurrencies = []Option{
	{Value: "MXN", Label: "Pesos"},
	{Value: "USD", Label: "Dólares"},
}

const cashOperationsDateBasis = "Fecha de registro de cada operación, con la hora de inicio del día."

func registerCashDetails() {
	register(Definition{
		ID: "cash.operations", Group: "cash", Title: "Movimientos de efectivo",
		Description: "Ingresos, retiros, gastos, correcciones y regularizaciones de efectivo, con su moneda, responsable y autorización.",
		Permissions: []string{PermissionCash}, DateBasis: cashOperationsDateBasis,
		Filters: []Filter{filterChoice("kind", "Tipo", "", cashOperationKinds...), filterChoice("currency", "Moneda", "", cashCurrencies...),
			filterRegister(), filterShift(), filterUser("Registró")},
		Columns: []Column{dateTimeColumn("created_at", "Fecha"), columnShiftRegister,
			dateColumn("business_date", "Fecha comercial del turno"), textColumn("kind", "Tipo"), textColumn("type", "Clasificación"),
			textColumn("concept", "Concepto"), textColumn("currency", "Moneda"),
			described(moneyColumn("in_mxn", "Entrada en pesos"), "Entradas de efectivo en pesos (sin regularizaciones)."),
			described(moneyColumn("out_mxn", "Salida en pesos"), "Salidas de efectivo en pesos (sin regularizaciones)."),
			described(dollarColumn("in_usd", "Entrada en dólares"), "Entradas de efectivo en dólares."),
			described(dollarColumn("out_usd", "Salida en dólares"), "Salidas de efectivo en dólares."),
			optional(described(moneyColumn("regularization_mxn", "Regularización en pesos"),
				"Corrección de un turno cerrado registrada en el vigente: no mueve efectivo ni cuenta en el esperado (con signo).")),
			optional(described(dollarColumn("regularization_usd", "Regularización en dólares"), "Con signo; no cuenta en el esperado.")),
			optional(Column{Key: "usd_rate", Label: "Tipo de cambio", Type: TypeQuantity, Sortable: true}),
			textColumn("user", "Registró"), textColumn("authorized_by", "Autorizó"), optional(textColumn("authorization", "Autorización")),
			optional(textColumn("reference", "Referencia"))},
		DefaultSort: Sort{Key: "created_at"},
		Query:       cashOperationsQuery,
	})
	register(Definition{
		ID: "cash.cuts", Group: "cash", Title: "Cortes parciales y cierres",
		Description: "Cada corte parcial y corte definitivo con lo ocurrido desde el corte anterior y, con permiso, el esperado y las diferencias.",
		Permissions: []string{PermissionCash}, DateBasis: "Fecha de registro del corte, con la hora de inicio del día.",
		Filters: []Filter{filterRegister(), filterShift(), filterChoice("kind", "Tipo de corte", "",
			Option{Value: "partial", Label: "Cortes parciales"}, Option{Value: "final", Label: "Cortes definitivos (cierres)"})},
		Columns: []Column{dateTimeColumn("created_at", "Fecha"), columnShiftRegister, dateColumn("business_date", "Fecha comercial del turno"),
			textColumn("cut", "Corte"), optional(dateTimeColumn("covered_from", "Desde")), optional(dateTimeColumn("covered_to", "Hasta")),
			textColumn("created_by", "Registró"),
			described(countColumn("payments", "Cobros"), "Cobros desde el corte anterior."),
			described(moneyColumn("sales", "Ventas cobradas"), "Desde el corte anterior, sin propinas ni devoluciones."),
			described(moneyColumn("tips", "Propinas"), "Desde el corte anterior."),
			described(moneyColumn("refunds", "Devoluciones"), "Desde el corte anterior."),
			described(moneyColumn("income_mxn", "Ingresos"), "Ingresos en pesos desde el corte anterior."),
			described(moneyColumn("withdrawals_mxn", "Retiros"), "Retiros en pesos desde el corte anterior."),
			described(moneyColumn("expenses_mxn", "Gastos"), "Gastos en pesos desde el corte anterior."),
			protectedCash(described(moneyColumn("expected_mxn", "Esperado en pesos"), "Esperado acumulado al momento del corte.")),
			protectedCash(described(dollarColumn("expected_usd", "Esperado en dólares"), "Esperado acumulado al momento del corte.")),
			protectedCash(described(moneyColumn("difference_mxn", "Diferencia en pesos"), "Solo en el corte definitivo.")),
			protectedCash(described(dollarColumn("difference_usd", "Diferencia en dólares"), "Solo en el corte definitivo."))},
		DefaultSort: Sort{Key: "created_at"},
		Query:       cashCutsQuery,
	})
	register(Definition{
		ID: "cash.by_user", Group: "cash", Title: "Operaciones de caja por responsable",
		Description: "Aperturas, cierres, cortes y operaciones de efectivo registradas o autorizadas por cada usuario.",
		Permissions: []string{PermissionCash}, DateBasis: "Fecha de cada apertura, cierre, corte u operación, con la hora de inicio del día.",
		Filters: []Filter{filterRegister(), filterUser("Usuario")},
		Columns: []Column{textColumn("user", "Usuario"),
			countColumn("openings", "Aperturas"), countColumn("closings", "Cierres"), countColumn("partial_cuts", "Cortes parciales"),
			countColumn("operations", "Operaciones registradas"), columnIncomeMXN, columnWithdrawalsMXN, columnExpensesMXN,
			columnIncomeUSD, columnWithdrawalsUSD, columnExpensesUSD,
			optional(countColumn("corrections", "Correcciones")), optional(countColumn("regularizations", "Regularizaciones")),
			described(countColumn("authorizations", "Autorizaciones a otros"),
				"Retiros, gastos o correcciones de otra persona que autorizó como supervisor.")},
		DefaultSort: Sort{Key: "user"},
		Query:       cashByUserQuery,
	})
}

func cashOperationsQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{"s.state <> 'closing'", cashScopeSQL, inPeriod("o.created_at")}
	switch query.Params.Value("kind") {
	case "":
	case "regularization":
		list.add("o.regularization")
	default:
		list.add("o.kind = @kind AND NOT o.regularization")
	}
	list.when(query, "currency", "o.currency = @currency")
	list.when(query, "register_id", "o.register_id = @register_id::uuid")
	list.when(query, "shift_id", "o.shift_id = @shift_id::uuid")
	list.when(query, "user_id", "o.actor_id = @user_id::uuid")
	rows, err := query.collect(ctx, `SELECT o.created_at,register.name AS register,`+shiftBusinessDateSQL+` AS business_date,
            o.kind AS kind_code,coalesce(o.subtype,'') AS subtype,coalesce(category.name,'') AS category,o.concept,o.currency,
            o.regularization,
            CASE WHEN NOT o.regularization AND o.currency='MXN' AND o.direction='in' THEN o.amount ELSE 0 END AS in_mxn,
            CASE WHEN NOT o.regularization AND o.currency='MXN' AND o.direction='out' THEN o.amount ELSE 0 END AS out_mxn,
            CASE WHEN NOT o.regularization AND o.currency='USD' AND o.direction='in' THEN o.amount ELSE 0 END AS in_usd,
            CASE WHEN NOT o.regularization AND o.currency='USD' AND o.direction='out' THEN o.amount ELSE 0 END AS out_usd,
            CASE WHEN o.regularization AND o.currency='MXN' THEN CASE o.direction WHEN 'in' THEN o.amount ELSE -o.amount END
                ELSE 0 END AS regularization_mxn,
            CASE WHEN o.regularization AND o.currency='USD' THEN CASE o.direction WHEN 'in' THEN o.amount ELSE -o.amount END
                ELSE 0 END AS regularization_usd,
            o.usd_rate,actor.display_name AS "user",coalesce(authorizer.display_name,'') AS authorized_by,
            CASE o.authorization_mode WHEN 'self' THEN 'Con permiso propio' WHEN 'supervisor' THEN 'De supervisor' ELSE '' END
                AS authorization,o.reference,o.exceptional
        FROM app.cash_operations o
        JOIN app.cash_shifts s ON s.id=o.shift_id
        JOIN app.registers register ON register.id=o.register_id
        JOIN app.users actor ON actor.id=o.actor_id
        LEFT JOIN app.users authorizer ON authorizer.id=o.authorized_by
        LEFT JOIN app.cash_expense_categories category ON category.id=o.category_id
        WHERE true`+list.sql(), cashArgs(query))
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		kind, _ := row["kind_code"].(string)
		subtype, _ := row["subtype"].(string)
		regularization, _ := row["regularization"].(bool)
		row["kind"] = cashOperationLabel(kind, regularization)
		row["type"] = payments.CashOperationTypeLabel(kind, subtype)
		if kind == "expense" {
			row["type"] = row["category"]
		}
		if exceptional, _ := row["exceptional"].(bool); exceptional {
			row["type"] = row["type"].(string) + " (salida excepcional)"
		}
	}
	return cashDataset(ctx, query, rows)
}

func cashOperationLabel(kind string, regularization bool) string {
	if regularization {
		return "Regularización"
	}
	switch kind {
	case "income":
		return "Ingreso"
	case "withdrawal":
		return "Retiro"
	case "expense":
		return "Gasto"
	}
	return "Corrección"
}

func cashCutsQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{"s.state <> 'closing'", cashScopeSQL, inPeriod("r.created_at")}
	list.when(query, "register_id", "s.register_id = @register_id::uuid")
	list.when(query, "shift_id", "s.id = @shift_id::uuid")
	list.when(query, "kind", "r.kind = @kind")
	// El bloque «sensitive» del corte solo se lee con cash.expected.read, como en Caja.
	sensitive := ""
	if expectedVisible(query) {
		sensitive = `,(r.snapshot->'sensitive'->'expected'->>'mxn')::numeric AS expected_mxn,
            (r.snapshot->'sensitive'->'expected'->>'usd')::numeric AS expected_usd,
            (r.snapshot->'sensitive'->'differences'->>'mxn')::numeric AS difference_mxn,
            (r.snapshot->'sensitive'->'differences'->>'usd')::numeric AS difference_usd`
	}
	cash := func(field string) string {
		return `coalesce((SELECT (e->>'` + field + `')::numeric FROM jsonb_array_elements(r.snapshot->'since_previous'->'cash') e
            WHERE e->>'currency'='MXN'),0)`
	}
	rows, err := query.collect(ctx, `SELECT r.created_at,register.name AS register,`+shiftBusinessDateSQL+` AS business_date,
            CASE r.kind WHEN 'partial' THEN 'Corte parcial '||r.sequence ELSE 'Corte definitivo' END AS cut,
            r.covered_from,r.covered_to,author.display_name AS created_by,
            coalesce((r.snapshot->'since_previous'->>'payments')::bigint,0) AS payments,
            coalesce((r.snapshot->'since_previous'->>'sales')::numeric,0) AS sales,
            coalesce((r.snapshot->'since_previous'->>'tips')::numeric,0) AS tips,
            coalesce((r.snapshot->'since_previous'->>'refunded')::numeric,0) AS refunds,
            `+cash("income")+` AS income_mxn,`+cash("withdrawals")+` AS withdrawals_mxn,`+cash("expenses")+` AS expenses_mxn`+sensitive+`
        FROM app.cash_shift_reports r
        JOIN app.cash_shifts s ON s.id=r.shift_id
        JOIN app.registers register ON register.id=s.register_id
        JOIN app.users author ON author.id=r.created_by
        WHERE true`+list.sql(), cashArgs(query))
	if err != nil {
		return nil, err
	}
	dataset, err := cashDataset(ctx, query, rows)
	if err != nil {
		return nil, err
	}
	dataset.Notes = append(dataset.Notes, "Los importes son los guardados en cada corte (lo ocurrido desde el corte anterior). "+
		"Los descuentos del corte no se reportan aquí: no pertenecen al turno.")
	return dataset, nil
}

func cashByUserQuery(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{cashScopeSQL}
	list.when(query, "register_id", "s.register_id = @register_id::uuid")
	user := ""
	if query.Has("user_id") {
		user = " WHERE u.user_id = @user_id::uuid"
	}
	rows, err := query.collect(ctx, `WITH scoped AS (
            SELECT s.* FROM app.cash_shifts s WHERE s.state <> 'closing'`+list.sql()+`
        ),
        opened AS (
            SELECT s.opened_by AS user_id,count(*)::bigint AS openings FROM scoped s WHERE `+inPeriod("s.opened_at")+` GROUP BY 1
        ),
        closed AS (
            SELECT cl.closed_by AS user_id,count(*)::bigint AS closings FROM app.cash_shift_closures cl JOIN scoped s ON s.id=cl.shift_id
            WHERE `+inPeriod("cl.closed_at")+` GROUP BY 1
        ),
        cuts AS (
            SELECT r.created_by AS user_id,count(*)::bigint AS partial_cuts FROM app.cash_shift_reports r JOIN scoped s ON s.id=r.shift_id
            WHERE r.kind='partial' AND `+inPeriod("r.created_at")+` GROUP BY 1
        ),
        recorded AS (
            SELECT o.actor_id AS user_id,count(*)::bigint AS operations,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='income' AND o.currency='MXN' AND NOT o.regularization),0) AS income_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='withdrawal' AND o.currency='MXN'),0) AS withdrawals_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='expense' AND o.currency='MXN'),0) AS expenses_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='income' AND o.currency='USD'),0) AS income_usd,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='withdrawal' AND o.currency='USD'),0) AS withdrawals_usd,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='expense' AND o.currency='USD'),0) AS expenses_usd,
                count(*) FILTER (WHERE o.kind='correction' AND NOT o.regularization)::bigint AS corrections,
                count(*) FILTER (WHERE o.regularization)::bigint AS regularizations
            FROM app.cash_operations o JOIN scoped s ON s.id=o.shift_id WHERE `+inPeriod("o.created_at")+` GROUP BY 1
        ),
        authorized AS (
            SELECT o.authorized_by AS user_id,count(*)::bigint AS authorizations FROM app.cash_operations o JOIN scoped s ON s.id=o.shift_id
            WHERE o.authorization_mode='supervisor' AND `+inPeriod("o.created_at")+` GROUP BY 1
        ),
        u AS (
            SELECT user_id FROM opened UNION SELECT user_id FROM closed UNION SELECT user_id FROM cuts
            UNION SELECT user_id FROM recorded UNION SELECT user_id FROM authorized
        )
        SELECT person.display_name AS "user",coalesce(opened.openings,0) AS openings,coalesce(closed.closings,0) AS closings,
            coalesce(cuts.partial_cuts,0) AS partial_cuts,coalesce(recorded.operations,0) AS operations,
            coalesce(recorded.income_mxn,0) AS income_mxn,coalesce(recorded.withdrawals_mxn,0) AS withdrawals_mxn,
            coalesce(recorded.expenses_mxn,0) AS expenses_mxn,coalesce(recorded.income_usd,0) AS income_usd,
            coalesce(recorded.withdrawals_usd,0) AS withdrawals_usd,coalesce(recorded.expenses_usd,0) AS expenses_usd,
            coalesce(recorded.corrections,0) AS corrections,coalesce(recorded.regularizations,0) AS regularizations,
            coalesce(authorized.authorizations,0) AS authorizations
        FROM u JOIN app.users person ON person.id=u.user_id
        LEFT JOIN opened ON opened.user_id=u.user_id LEFT JOIN closed ON closed.user_id=u.user_id
        LEFT JOIN cuts ON cuts.user_id=u.user_id LEFT JOIN recorded ON recorded.user_id=u.user_id
        LEFT JOIN authorized ON authorized.user_id=u.user_id`+user, cashArgs(query))
	if err != nil {
		return nil, err
	}
	return cashDataset(ctx, query, rows)
}
