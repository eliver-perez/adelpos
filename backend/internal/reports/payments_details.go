package reports

import "context"

// Detalle de cobros, pagos mixtos y parciales, devoluciones, correcciones, propinas y efectivo por
// moneda. Las columnas de efectivo físico (lo recibido y el cambio) exigen cash.expected.read, como
// en Caja: sumadas por turno permitirían deducir el esperado. Todos se limitan, como payments.go, a
// los turnos que el usuario puede consultar en Caja (paymentScope). Los pesos físicos de un
// movimiento anterior a la 0007 siguen la regla de Caja (physicalMXNInSQL y physicalMXNOutSQL).

var tipGrouping = []Option{
	{Value: "method", Label: "Por método"},
	{Value: "register", Label: "Por caja"},
	{Value: "shift", Label: "Por turno"},
	{Value: "day", Label: "Por día"},
	{Value: "user", Label: "Por usuario"},
}

var currencyGrouping = []Option{
	{Value: "day", Label: "Por día"},
	{Value: "shift", Label: "Por turno"},
}

var authorizationScopes = []Option{
	{Value: "all", Label: "Todas"},
	{Value: "discount", Label: "Descuentos"},
	{Value: "courtesy", Label: "Cortesías"},
	{Value: "adjustment_void", Label: "Cancelaciones de ajustes"},
	{Value: "refund", Label: "Devoluciones"},
	{Value: "correction", Label: "Correcciones"},
}

// groupingSQL devuelve la etiqueta y el agrupamiento de una agrupación de movimientos.
func groupingSQL(grouping, at string) (string, string) {
	switch grouping {
	case "method":
		return "(array_agg(m.method_name ORDER BY m.at DESC))[1]", "m.method_id"
	case "register":
		return "register.name", "s.register_id,register.name"
	case "shift":
		return shiftLabelSQL, "s.id,register.name,s.business_date,s.opened_at,opener.display_name"
	case "user":
		return "actor.display_name", "m.actor_id,actor.display_name"
	}
	return "to_char(" + businessDate(at) + ",'DD/MM/YYYY')", businessDate(at)
}

func registerPaymentDetails() {
	physical := func(column Column) Column {
		return requiring(column, PermissionCashExpected)
	}
	register(Definition{
		ID: "payments.list", Group: "payments", Title: "Detalle de cobros",
		Description: "Cada cobro, devolución y reverso de corrección del periodo.",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis, Filters: paymentFilters(),
		Columns: []Column{dateTimeColumn("created_at", "Fecha"), codeColumn("folio", "Folio"), textColumn("kind", "Tipo"),
			textColumn("method", "Método"), moneyColumn("amount", "Importe"), moneyColumn("tip", "Propina"),
			physical(optional(moneyColumn("received", "Recibido"))), physical(optional(moneyColumn("change", "Cambio"))),
			physical(optional(dollarColumn("received_usd", "Dólares recibidos"))),
			physical(optional(Column{Key: "usd_rate", Label: "Tipo de cambio", Type: TypeQuantity, Sortable: true})),
			textColumn("register", "Caja"), dateColumn("business_date", "Fecha comercial del turno"), textColumn("user", "Registró"),
			optional(textColumn("reference", "Referencia"))},
		DefaultSort: Sort{Key: "created_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			return paymentDataset(ctx, query, paymentMovements+`
                SELECT m.at AS created_at,a.folio::text AS folio,
                    CASE m.kind WHEN 'payment' THEN CASE WHEN m.corrective THEN 'Cobro de corrección' ELSE 'Cobro' END
                        WHEN 'refund' THEN 'Devolución' ELSE 'Reverso de corrección' END AS kind,
                    m.method_name AS method,m.amount,m.tip,
                    CASE WHEN m.kind='payment' THEN p.received END AS received,
                    CASE WHEN m.kind='payment' AND p.is_cash THEN p.change_given END AS change,
                    CASE WHEN m.kind='payment' THEN p.received_usd END AS received_usd,
                    CASE WHEN m.kind='payment' THEN p.usd_rate END AS usd_rate,
                    register.name AS register,coalesce(s.business_date,(s.opened_at AT TIME ZONE @tz)::date) AS business_date,
                    actor.display_name AS "user",coalesce(CASE WHEN m.kind='payment' THEN p.reference ELSE refund.reference END,'') AS reference
                FROM movements m
                JOIN app.cash_shifts s ON s.id=m.shift_id
                JOIN app.registers register ON register.id=m.register_id
                JOIN app.users actor ON actor.id=m.actor_id
                JOIN app.restaurant_accounts a ON a.id=m.account_id
                JOIN app.payments p ON p.id=m.payment_id
                LEFT JOIN app.payment_refunds refund ON refund.id=m.refund_id
                WHERE true`+paymentConditions(query).sql())
		},
	})
	register(Definition{
		ID: "payments.mixed_partial", Group: "payments", Title: "Pagos mixtos y parciales",
		Description: "Cuentas con más de un pago en el periodo: mixtas (varios métodos) o parciales (varios pagos del mismo método).",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis,
		Filters: []Filter{filterRegister(), filterUser("Usuario que registró")},
		Columns: []Column{codeColumn("folio", "Folio"), dateTimeColumn("first_at", "Primer pago"), dateTimeColumn("last_at", "Último pago"),
			countColumn("payments", "Pagos"), countColumn("methods", "Métodos"), textColumn("method_names", "Métodos usados"),
			moneyColumn("collected", "Importe cobrado"), moneyColumn("tips", "Propinas"), textColumn("classification", "Clasificación")},
		DefaultSort: Sort{Key: "first_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := append(paymentScope(), inPeriod("p.created_at"))
			list.when(query, "register_id", "p.register_id = @register_id::uuid")
			list.when(query, "user_id", "p.actor_id = @user_id::uuid")
			dataset, err := paymentDataset(ctx, query, `WITH account_payments AS (
                    SELECT p.account_id,count(*)::bigint AS payments,count(DISTINCT p.method_id)::bigint AS methods,
                        string_agg(DISTINCT p.method_name,', ') AS method_names,sum(p.amount) AS collected,sum(p.tip) AS tips,
                        min(p.created_at) AS first_at,max(p.created_at) AS last_at
                    FROM app.payments p JOIN app.cash_shifts s ON s.id=p.shift_id
                    WHERE true`+list.sql()+` GROUP BY p.account_id
                )
                SELECT a.folio::text AS folio,x.first_at,x.last_at,x.payments,x.methods,x.method_names,x.collected,x.tips,
                    CASE WHEN x.methods > 1 THEN 'Mixto (varios métodos)' ELSE 'Parcial (varios pagos)' END AS classification
                FROM account_payments x JOIN app.restaurant_accounts a ON a.id=x.account_id
                WHERE x.payments > 1`)
			if err != nil {
				return nil, err
			}
			dataset.Notes = append(dataset.Notes, "Se derivan de los pagos registrados: no existe una marca de pago mixto o parcial.")
			return dataset, nil
		},
	})
	register(Definition{
		ID: "payments.refunds", Group: "payments", Title: "Devoluciones",
		Description: "Devoluciones en el día en que se hicieron (no modifican la venta del día original).",
		Permissions: []string{PermissionPayments}, DateBasis: "Fecha de la devolución, con la hora de inicio del día.",
		Filters: []Filter{filterMethod(), filterRegister(), filterUser("Usuario que devolvió")},
		Columns: []Column{dateTimeColumn("created_at", "Fecha"), codeColumn("folio", "Folio"), textColumn("method", "Método de salida"),
			moneyColumn("amount", "Importe devuelto"), moneyColumn("tip", "Propina devuelta"), textColumn("reason", "Motivo"),
			textColumn("refunded_by", "Devolvió"), textColumn("authorized_by", "Autorizó"), textColumn("register", "Caja"),
			optional(textColumn("original_method", "Método del pago original")), optional(dateTimeColumn("original_at", "Fecha del pago original"))},
		DefaultSort: Sort{Key: "created_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := append(paymentScope(), "r.kind = 'refund'", inPeriod("r.created_at"))
			list.when(query, "method_id", "r.method_id = @method_id::uuid")
			list.when(query, "register_id", "r.register_id = @register_id::uuid")
			list.when(query, "user_id", "r.actor_id = @user_id::uuid")
			return paymentDataset(ctx, query, `SELECT r.created_at,a.folio::text AS folio,r.method_name AS method,r.amount,r.tip,r.reason,
                    actor.display_name AS refunded_by,coalesce(authorizer.display_name,'') AS authorized_by,register.name AS register,
                    p.method_name AS original_method,p.created_at AS original_at
                FROM app.payment_refunds r
                JOIN app.cash_shifts s ON s.id=r.shift_id
                JOIN app.payments p ON p.id=r.payment_id
                JOIN app.restaurant_accounts a ON a.id=r.account_id
                JOIN app.users actor ON actor.id=r.actor_id
                LEFT JOIN app.users authorizer ON authorizer.id=r.authorized_by
                JOIN app.registers register ON register.id=r.register_id
                WHERE true`+list.sql())
		},
	})
	register(Definition{
		ID: "payments.corrections", Group: "payments", Title: "Correcciones de pagos",
		Description: "Cada corrección: el reverso del pago original y el pago correcto que lo sustituye.",
		Permissions: []string{PermissionPayments}, DateBasis: "Fecha de la corrección, con la hora de inicio del día.",
		Filters: []Filter{filterRegister(), filterUser("Usuario que corrigió")},
		Columns: []Column{dateTimeColumn("created_at", "Fecha"), codeColumn("folio", "Folio"),
			textColumn("original_method", "Método original"), moneyColumn("original_amount", "Importe revertido"),
			textColumn("new_method", "Método correcto"), moneyColumn("new_amount", "Importe correcto"), textColumn("reason", "Motivo"),
			textColumn("corrected_by", "Corrigió"), textColumn("authorized_by", "Autorizó")},
		DefaultSort: Sort{Key: "created_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := append(paymentScope(), "r.kind = 'correction'", inPeriod("r.created_at"))
			list.when(query, "register_id", "r.register_id = @register_id::uuid")
			list.when(query, "user_id", "r.actor_id = @user_id::uuid")
			return paymentDataset(ctx, query, `SELECT r.created_at,a.folio::text AS folio,r.method_name AS original_method,
                    r.amount AS original_amount,n.method_name AS new_method,n.amount AS new_amount,r.reason,
                    actor.display_name AS corrected_by,coalesce(authorizer.display_name,'') AS authorized_by
                FROM app.payment_refunds r
                JOIN app.cash_shifts s ON s.id=r.shift_id
                JOIN app.restaurant_accounts a ON a.id=r.account_id
                JOIN app.users actor ON actor.id=r.actor_id
                LEFT JOIN app.users authorizer ON authorizer.id=r.authorized_by
                LEFT JOIN app.payments n ON n.corrects_payment_id=r.payment_id AND n.operation_id=r.operation_id
                WHERE true`+list.sql())
		},
	})
	register(Definition{
		ID: "payments.tips", Group: "payments", Title: "Propinas",
		Description: "Propinas separadas de las ventas, por método, caja, turno, día o usuario.",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis,
		Filters: append([]Filter{filterChoice("group_by", "Agrupar", "method", tipGrouping...)}, paymentFilters()...),
		Columns: []Column{textColumn("group_label", "Agrupación"), moneyColumn("tips_collected", "Propinas cobradas"),
			moneyColumn("tips_refunded", "Propinas devueltas"), moneyColumn("tips", "Propinas netas")},
		DefaultSort: Sort{Key: "tips", Descending: true},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			label, group := groupingSQL(query.Params.Value("group_by"), "m.at")
			return paymentDataset(ctx, query, paymentMovements+` SELECT `+label+` AS group_label,
                    coalesce(sum(m.tip) FILTER (WHERE m.kind='payment'),0) AS tips_collected,
                    coalesce(-sum(m.tip) FILTER (WHERE m.kind<>'payment'),0) AS tips_refunded,coalesce(sum(m.tip),0) AS tips
                FROM movements m JOIN app.cash_shifts s ON s.id=m.shift_id `+shiftJoins+`
                JOIN app.users actor ON actor.id=m.actor_id
                WHERE true`+paymentConditions(query).sql()+` GROUP BY `+group+` HAVING sum(abs(m.tip)) > 0`)
		},
	})
	register(Definition{
		ID: "payments.currency", Group: "payments", Title: "Efectivo por moneda",
		Description: "Pesos y dólares físicos recibidos y entregados, con la equivalencia al tipo de cambio de cada cobro.",
		Permissions: []string{PermissionPayments, PermissionCashExpected},
		DateBasis:   "Fecha del movimiento de efectivo (cobro, devolución o reverso), con la hora de inicio del día.",
		Filters: []Filter{filterChoice("group_by", "Agrupar", "day", currencyGrouping...), filterRegister(), filterShift(),
			filterUser("Usuario que registró")},
		Columns: []Column{textColumn("group_label", "Agrupación"),
			described(moneyColumn("mxn_in", "Pesos recibidos"), "Pesos físicos que entraron al cajón."),
			described(moneyColumn("mxn_out", "Pesos entregados"), "Cambio y devoluciones en pesos."),
			described(dollarColumn("usd_in", "Dólares recibidos"), "Dólares físicos recibidos (nunca se suman con pesos)."),
			described(dollarColumn("usd_out", "Cambio en dólares"), "Dólares físicos entregados como cambio."),
			described(moneyColumn("usd_in_equivalent", "Equivalencia de los dólares"),
				"Pesos que representaron los dólares recibidos, al tipo de cambio de cada cobro."),
			textColumn("rates", "Tipo de cambio")},
		DefaultSort: Sort{},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			grouping := query.Params.Value("group_by")
			label, group := "to_char("+businessDate("c.created_at")+",'DD/MM/YYYY')", businessDate("c.created_at")
			order := businessDate("c.created_at")
			if grouping == "shift" {
				label, group, order = shiftLabelSQL, "s.id,register.name,s.business_date,s.opened_at,opener.display_name", "s.opened_at"
			}
			list := append(paymentScope(), "c.is_cash", inPeriod("c.created_at"))
			list.when(query, "register_id", "c.register_id = @register_id::uuid")
			list.when(query, "shift_id", "c.shift_id = @shift_id::uuid")
			list.when(query, "user_id", "c.actor_id = @user_id::uuid")
			return paymentDataset(ctx, query, `SELECT `+label+` AS group_label,
                    coalesce(sum(`+physicalMXNInSQL+`),0) AS mxn_in,coalesce(sum(`+physicalMXNOutSQL+`),0) AS mxn_out,
                    coalesce(sum(c.usd_in),0) AS usd_in,coalesce(sum(c.usd_out),0) AS usd_out,
                    coalesce(sum(round(c.usd_in*c.usd_rate,2)) FILTER (WHERE c.usd_in > 0),0) AS usd_in_equivalent,
                    coalesce(CASE WHEN min(c.usd_rate) = max(c.usd_rate) THEN to_char(min(c.usd_rate),'FM999990.00')
                        ELSE to_char(min(c.usd_rate),'FM999990.00')||' – '||to_char(max(c.usd_rate),'FM999990.00') END,'—') AS rates
                FROM app.cash_movements c JOIN app.cash_shifts s ON s.id=c.shift_id `+shiftJoins+`
                WHERE true`+list.sql()+` GROUP BY `+group+` ORDER BY min(`+order+`)`)
		},
	})
}
