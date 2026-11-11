package reports

import (
	"context"
)

// Reportes de descuentos y cortesías (§11). Fuente: el libro de reconocimientos, de modo que cada
// descuento se fecha con el reconocimiento de su venta (el cierre de la cuenta) y su porcentaje sobre
// ventas cuadra con los reportes de ventas. Los descuentos cancelados no se reconocen. El motivo es el
// nombre histórico guardado al registrar el descuento (reason_name); sin motivo del catálogo se
// clasifica como «Sin motivo del catálogo», sin atribuirle uno actual.

const discountsDateBasis = "Fecha de reconocimiento de la venta (cierre de la cuenta) a la que pertenece el descuento."

func discountFilters() []Filter {
	return []Filter{filterReason(), filterChoice("kind", "Tipo", "", Option{Value: "discount", Label: "Descuentos"},
		Option{Value: "courtesy", Label: "Cortesías"}), filterMode(), filterProduct(), filterCategory()}
}

// discountConditions son las condiciones del libro para los renglones de descuentos y cortesías.
func discountConditions(query *Query) conditions {
	list := ledgerConditions(query)
	list.add("r.adjustment_kind = 'discount'")
	if query.Params.Value("reason_id") == NoReason {
		list.add("r.reason_id IS NULL")
	} else {
		list.when(query, "reason_id", "r.reason_id = @reason_id::uuid")
	}
	switch query.Params.Value("kind") {
	case "discount":
		list.add("NOT r.is_courtesy")
	case "courtesy":
		list.add("r.is_courtesy")
	}
	return list
}

var discountAmountColumns = []Column{
	described(countColumn("records", "Registros"), "Descuentos o cortesías distintos."),
	columnDiscount,
	columnCourtesy,
	described(moneyColumn("total_discount", "Total descontado"), "Descuentos + cortesías."),
}

const discountAmounts = `count(DISTINCT r.adjustment_id)::bigint AS records,coalesce(sum(r.discount),0) AS discount,
    coalesce(sum(r.courtesy),0) AS courtesy,coalesce(sum(r.discount+r.courtesy),0) AS total_discount`

func discountShare(rows []Row, totals Row) {
	total := decimalOf(totals, "total_discount")
	for _, row := range rows {
		row["share"] = percentOf(decimalOf(row, "total_discount"), total)
	}
	totals["share"] = percentOf(total, total)
}

// discountDetail lista cada descuento o cortesía reconocido en el periodo.
func discountDetail(extra string) func(ctx context.Context, query *Query) (*Dataset, error) {
	return func(ctx context.Context, query *Query) (*Dataset, error) {
		list := discountConditions(query)
		if extra != "" {
			list.add(extra)
		}
		rows, err := query.collect(ctx, `SELECT min(r.recognized_at) AS recognized_at,min(r.adjustment_created_at) AS created_at,
                min(r.account_folio)::text AS folio,
                CASE WHEN bool_or(r.is_courtesy) THEN 'Cortesía' ELSE 'Descuento' END AS kind,
                CASE min(r.adjustment_scope) WHEN 'items' THEN 'Por partida' ELSE 'General de cuenta' END AS scope,
                coalesce(min(r.reason_name),'`+NoReasonLabel+`') AS reason,coalesce(min(r.concept),'') AS concept,
                coalesce(sum(r.discount+r.courtesy),0) AS total_discount,coalesce(min(actor.display_name),'') AS applied_by,
                coalesce(min(authorizer.display_name),'') AS authorized_by
            FROM app.sales_recognitions r
            LEFT JOIN app.users actor ON actor.id=r.adjustment_actor_id
            LEFT JOIN app.users authorizer ON authorizer.id=r.adjustment_authorized_by
            WHERE true`+list.sql()+` GROUP BY r.adjustment_id HAVING sum(r.discount+r.courtesy) <> 0`, query.Args())
		if err != nil {
			return nil, err
		}
		return &Dataset{Rows: rows}, nil
	}
}

func init() {
	register(Definition{
		ID: "discounts.summary", Group: "discounts", Title: "Descuentos y cortesías por día",
		Description: "Descuentos por partida, generales y cortesías de cada día, con su porcentaje sobre las ventas.",
		Permissions: []string{PermissionDiscounts}, DateBasis: discountsDateBasis, Filters: salesFilters(),
		Columns: []Column{dateColumn("day", "Día"),
			described(moneyColumn("item_discounts", "Descuentos por partida"), "Descuentos manuales asignados a unidades."),
			described(moneyColumn("general_discounts", "Descuentos generales"), "Descuentos de cuenta sin asignación a unidades."),
			columnCourtesy,
			described(moneyColumn("total_discount", "Total descontado"), "Descuentos + cortesías."),
			described(moneyColumn("sales_before_discounts", "Venta antes de descuentos"),
				"Importe bruto − promociones + cargos: la venta sobre la que se aplicaron los descuentos."),
			described(percentColumn("discount_percent", "% sobre ventas"), "Total descontado entre la venta antes de descuentos.")},
		DefaultSort: Sort{Key: "day"},
		Totals: func(rows []Row, totals Row) {
			for _, row := range append([]Row{totals}, rows...) {
				row["discount_percent"] = percentOf(decimalOf(row, "total_discount"), decimalOf(row, "sales_before_discounts"))
			}
		},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, `SELECT `+businessDate("r.recognized_at")+` AS day,
                    coalesce(sum(r.discount) FILTER (WHERE r.source_kind='allocation'),0) AS item_discounts,
                    coalesce(sum(r.discount) FILTER (WHERE r.source_kind='adjustment'),0) AS general_discounts,
                    coalesce(sum(r.courtesy),0) AS courtesy,coalesce(sum(r.discount+r.courtesy),0) AS total_discount,
                    coalesce(sum(r.gross-r.promotion+r.charge),0) AS sales_before_discounts
                FROM app.sales_recognitions r WHERE true`+ledgerConditions(query).sql()+` GROUP BY 1`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: fillDays(rows, query.Params.Period, query.definition.Columns)}, nil
		},
	})
	register(Definition{
		ID: "discounts.by_reason", Group: "discounts", Title: "Descuentos por motivo",
		Description: "Descuentos y cortesías por el nombre del motivo registrado; sin motivo, «Sin motivo del catálogo».",
		Permissions: []string{PermissionDiscounts}, DateBasis: discountsDateBasis, Filters: discountFilters(),
		Columns: withColumns([]Column{textColumn("reason", "Motivo"), textColumn("kind", "Tipo")}, discountAmountColumns,
			[]Column{described(percentColumn("share", "% del total"), "Participación en el total descontado.")}),
		DefaultSort: Sort{Key: "total_discount", Descending: true}, Totals: discountShare,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, `SELECT coalesce(r.reason_name,'`+NoReasonLabel+`') AS reason,
                    CASE WHEN r.is_courtesy THEN 'Cortesía' ELSE 'Descuento' END AS kind,`+discountAmounts+`
                FROM app.sales_recognitions r WHERE true`+discountConditions(query).sql()+`
                GROUP BY coalesce(r.reason_name,'`+NoReasonLabel+`'),r.is_courtesy`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows, Notes: []string{"El motivo es el nombre que tenía al registrar el descuento: renombrar un motivo no cambia el historial."}}, nil
		},
	})
	register(Definition{
		ID: "discounts.by_user", Group: "discounts", Title: "Descuentos por usuario",
		Description: "Descuentos y cortesías por el usuario que los aplicó.",
		Permissions: []string{PermissionDiscounts}, DateBasis: discountsDateBasis, Filters: discountFilters(),
		Columns:     withColumns([]Column{textColumn("user", "Aplicó")}, discountAmountColumns),
		DefaultSort: Sort{Key: "total_discount", Descending: true},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, `SELECT coalesce(actor.display_name,'Sin usuario') AS "user",`+discountAmounts+`
                FROM app.sales_recognitions r LEFT JOIN app.users actor ON actor.id=r.adjustment_actor_id
                WHERE true`+discountConditions(query).sql()+` GROUP BY r.adjustment_actor_id,actor.display_name`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows}, nil
		},
	})
	register(Definition{
		ID: "discounts.by_product", Group: "discounts", Title: "Descuentos por producto",
		Description: "Descuentos y cortesías por partida de cada producto (los generales de cuenta no se reparten).",
		Permissions: []string{PermissionDiscounts}, DateBasis: discountsDateBasis, Filters: discountFilters(),
		Columns: withColumns([]Column{codeColumn("code", "Código"), textColumn("product", "Producto"),
			described(countColumn("units", "Unidades con descuento"), "Unidades con un descuento o cortesía por partida.")},
			discountAmountColumns),
		DefaultSort: Sort{Key: "total_discount", Descending: true},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := discountConditions(query)
			list.add("r.source_kind = 'allocation'")
			rows, err := query.collect(ctx, `SELECT `+latest("r.product_code")+` AS code,`+latest("r.product_name")+` AS product,
                    count(DISTINCT r.unit_id)::bigint AS units,`+discountAmounts+`
                FROM app.sales_recognitions r WHERE true`+list.sql()+` GROUP BY r.product_id`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows}, nil
		},
	})
	detailColumns := []Column{dateTimeColumn("recognized_at", "Reconocido"), optional(dateTimeColumn("created_at", "Registrado")),
		codeColumn("folio", "Folio"), textColumn("kind", "Tipo"), textColumn("scope", "Alcance"), textColumn("reason", "Motivo"),
		textColumn("concept", "Concepto"), described(moneyColumn("total_discount", "Importe"), "Importe reconocido del descuento o cortesía."),
		textColumn("applied_by", "Aplicó"), textColumn("authorized_by", "Autorizó")}
	register(Definition{
		ID: "discounts.list", Group: "discounts", Title: "Detalle de descuentos y cortesías",
		Description: "Cada descuento o cortesía reconocido en el periodo, con su motivo histórico y quién lo aplicó y autorizó.",
		Permissions: []string{PermissionDiscounts}, DateBasis: discountsDateBasis, Filters: discountFilters(),
		Columns: detailColumns, DefaultSort: Sort{Key: "recognized_at"}, Query: discountDetail(""),
	})
	register(Definition{
		ID: "discounts.general", Group: "discounts", Title: "Descuentos generales de cuenta",
		Description: "Descuentos aplicados al total de la cuenta, sin asignación a productos.",
		Permissions: []string{PermissionDiscounts}, DateBasis: discountsDateBasis, Filters: discountFilters(),
		Columns: detailColumns, DefaultSort: Sort{Key: "recognized_at"}, Query: discountDetail("r.adjustment_scope = 'account'"),
	})
	register(Definition{
		ID: "discounts.courtesies", Group: "discounts", Title: "Cortesías",
		Description: "Cortesías al 100 % (por partida o de toda la cuenta): venta cero con su importe aquí.",
		Permissions: []string{PermissionDiscounts}, DateBasis: discountsDateBasis, Filters: discountFilters(),
		Columns: detailColumns, DefaultSort: Sort{Key: "recognized_at"}, Query: discountDetail("r.is_courtesy"),
	})
	register(Definition{
		ID: "discounts.authorizations", Group: "discounts", Title: "Autorizaciones de supervisor",
		Description: "Autorizaciones puntuales de supervisor: descuentos, cortesías, cancelaciones de ajustes, devoluciones y correcciones.",
		Permissions: []string{PermissionDiscounts}, DateBasis: "Fecha de la autorización, con la hora de inicio del día.",
		Filters: []Filter{filterChoice("scope", "Tipo", "all", authorizationScopes...), filterUser("Autorizó")},
		Columns: []Column{dateTimeColumn("created_at", "Fecha"), codeColumn("folio", "Folio"), textColumn("scope", "Tipo"),
			moneyColumn("amount", "Importe"), textColumn("requested_by", "Solicitó"), textColumn("authorized_by", "Autorizó")},
		DefaultSort: Sort{Key: "created_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := conditions{inPeriod("x.created_at")}
			if scope := query.Params.Value("scope"); scope != "" && scope != "all" {
				list.add("x.scope = @scope")
			}
			list.when(query, "user_id", "x.authorized_by = @user_id::uuid")
			rows, err := query.collect(ctx, `SELECT x.created_at,a.folio::text AS folio,
                    CASE x.scope WHEN 'discount' THEN 'Descuento' WHEN 'courtesy' THEN 'Cortesía'
                        WHEN 'adjustment_void' THEN 'Cancelación de ajuste' WHEN 'refund' THEN 'Devolución' ELSE 'Corrección' END AS scope,
                    x.amount,requester.display_name AS requested_by,authorizer.display_name AS authorized_by
                FROM app.payment_authorizations x
                JOIN app.restaurant_accounts a ON a.id=x.account_id
                JOIN app.users requester ON requester.id=x.requested_by
                JOIN app.users authorizer ON authorizer.id=x.authorized_by
                WHERE true`+list.sql(), query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows, Notes: []string{"Una autorización por operación: una devolución que abarca varios pagos aparece una sola vez."}}, nil
		},
	})
}
