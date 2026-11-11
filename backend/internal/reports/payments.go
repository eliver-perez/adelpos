package reports

import (
	"context"
	"strconv"
)

// Reportes de pagos y devoluciones (§10). Fuente: los registros financieros inmutables de payments
// (pagos, devoluciones y reversos de corrección) con su propia fecha (created_at): una devolución se
// refleja en el día en que se hizo, sin tocar la venta del día original. Cobrado neto = pagos −
// devoluciones − reversos de corrección (el pago nuevo de una corrección es un pago), de modo que
// una corrección nunca duplica lo cobrado. Las propinas se reportan aparte de la venta.
//
// Paridad con Caja: los cobros de un turno en arqueo (closing) se excluyen de todo agregado para
// todos, con un aviso; el efectivo físico por moneda (lo recibido, el cambio, los dólares) exige
// además cash.expected.read, igual que en Caja.

const paymentsDateBasis = "Fecha de registro de cada cobro, devolución o corrección, con la hora de inicio del día."

// paymentMovements son los movimientos financieros firmados del periodo (pagos en positivo,
// devoluciones y reversos de corrección en negativo), sin los turnos en arqueo.
const paymentMovements = `WITH movements AS (
        SELECT p.created_at AS at,'payment'::text AS kind,p.id AS payment_id,NULL::uuid AS refund_id,p.account_id,p.method_id,
            p.method_name,p.is_cash,p.register_id,p.shift_id,p.actor_id,p.amount,p.tip,p.corrects_payment_id IS NOT NULL AS corrective
        FROM app.payments p WHERE p.created_at >= @from AND p.created_at < @until
        UNION ALL
        SELECT r.created_at,r.kind,r.payment_id,r.id,r.account_id,r.method_id,r.method_name,r.is_cash,r.register_id,r.shift_id,
            r.actor_id,-r.amount,-r.tip,false
        FROM app.payment_refunds r WHERE r.created_at >= @from AND r.created_at < @until
    )`

const paymentMeasures = `count(*) FILTER (WHERE m.kind='payment')::bigint AS payments,
    coalesce(sum(m.amount) FILTER (WHERE m.kind='payment'),0) AS collected,
    coalesce(-sum(m.amount) FILTER (WHERE m.kind='refund'),0) AS refunds,
    coalesce(-sum(m.amount) FILTER (WHERE m.kind='correction'),0) AS corrections,
    coalesce(sum(m.amount),0) AS net,coalesce(sum(m.tip),0) AS tips,coalesce(sum(m.amount+m.tip),0) AS received_total`

var paymentMeasureColumns = []Column{
	described(countColumn("payments", "Cobros"), "Pagos registrados (incluye el pago nuevo de una corrección)."),
	described(moneyColumn("collected", "Importe cobrado"), "Suma de los pagos aplicados a la venta, sin propinas."),
	described(moneyColumn("refunds", "Devoluciones"), "Devoluciones registradas en el periodo."),
	described(moneyColumn("corrections", "Correcciones"), "Reversos de pagos corregidos (el pago correcto aparece en «Importe cobrado»)."),
	described(moneyColumn("net", "Cobrado neto"), "Importe cobrado − devoluciones − correcciones."),
	described(moneyColumn("tips", "Propinas netas"), "Propinas cobradas menos propinas devueltas; no son venta."),
	described(optional(moneyColumn("received_total", "Cobrado con propinas")), "Cobrado neto + propinas netas."),
}

func paymentFilters() []Filter {
	return []Filter{filterMethod(), filterRegister(), filterShift(), filterUser("Usuario que registró")}
}

func paymentConditions(query *Query) conditions {
	list := conditions{"s.state <> 'closing'"}
	list.when(query, "method_id", "m.method_id = @method_id::uuid")
	list.when(query, "register_id", "m.register_id = @register_id::uuid")
	list.when(query, "shift_id", "m.shift_id = @shift_id::uuid")
	list.when(query, "user_id", "m.actor_id = @user_id::uuid")
	return list
}

// closingShiftsWarning avisa cuántos turnos en arqueo tienen movimientos en el periodo (sus importes
// no se incluyen). Nunca dice cuánto.
func closingShiftsWarning(ctx context.Context, query *Query) (*Warning, error) {
	var count int
	err := query.Tx.QueryRow(ctx, `SELECT count(*) FROM app.cash_shifts s WHERE s.state='closing' AND (
            EXISTS(SELECT FROM app.payments p WHERE p.shift_id=s.id AND p.created_at >= @from AND p.created_at < @until)
            OR EXISTS(SELECT FROM app.payment_refunds r WHERE r.shift_id=s.id AND r.created_at >= @from AND r.created_at < @until)
            OR EXISTS(SELECT FROM app.sales_recognitions x WHERE x.shift_id=s.id AND x.recognized_at >= @from AND x.recognized_at < @until))`,
		query.Args()).Scan(&count)
	if err != nil || count == 0 {
		return nil, err
	}
	return &Warning{Code: "SHIFT_CLOSING", Message: "Turnos en arqueo excluidos: " + strconv.Itoa(count) +
		". Sus importes se incluirán cuando termine el arqueo."}, nil
}

// paymentDataset ejecuta una consulta de movimientos y agrega el aviso de turnos en arqueo.
func paymentDataset(ctx context.Context, query *Query, sql string) (*Dataset, error) {
	rows, err := query.collect(ctx, sql, query.Args())
	if err != nil {
		return nil, err
	}
	dataset := &Dataset{Rows: rows}
	warning, err := closingShiftsWarning(ctx, query)
	if err != nil {
		return nil, err
	}
	if warning != nil {
		dataset.Warnings = append(dataset.Warnings, *warning)
	}
	return dataset, nil
}

// paymentsBy agrupa los movimientos por una dimensión.
func paymentsBy(selectSQL, joins, groupSQL string) func(ctx context.Context, query *Query) (*Dataset, error) {
	return func(ctx context.Context, query *Query) (*Dataset, error) {
		return paymentDataset(ctx, query, paymentMovements+` SELECT `+selectSQL+`,`+paymentMeasures+`
            FROM movements m JOIN app.cash_shifts s ON s.id=m.shift_id `+joins+`
            WHERE true`+paymentConditions(query).sql()+` GROUP BY `+groupSQL)
	}
}

// shiftLabelSQL es la etiqueta de un turno: caja, fecha comercial y quién lo abrió.
const shiftLabelSQL = `register.name||' · '||to_char(coalesce(s.business_date,(s.opened_at AT TIME ZONE @tz)::date),'DD/MM/YYYY')
    ||' · '||opener.display_name`

const shiftJoins = `JOIN app.registers register ON register.id=s.register_id JOIN app.users opener ON opener.id=s.opened_by`

func init() {
	register(Definition{
		ID: "payments.by_method", Group: "payments", Title: "Cobros por método",
		Description: "Cobros, devoluciones, correcciones y propinas por método de pago (devoluciones por su método de salida).",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis, Filters: paymentFilters(),
		Columns:     withColumns([]Column{textColumn("method", "Método")}, paymentMeasureColumns),
		DefaultSort: Sort{Key: "net", Descending: true},
		Query:       paymentsBy("(array_agg(m.method_name ORDER BY m.at DESC))[1] AS method", "", "m.method_id"),
	})
	register(Definition{
		ID: "payments.by_register", Group: "payments", Title: "Cobros por caja",
		Description: "Cobros, devoluciones, correcciones y propinas por caja física.",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis, Filters: paymentFilters(),
		Columns:     withColumns([]Column{textColumn("register", "Caja")}, paymentMeasureColumns),
		DefaultSort: Sort{Key: "register"},
		Query:       paymentsBy("register.name AS register", "JOIN app.registers register ON register.id=m.register_id", "m.register_id,register.name"),
	})
	register(Definition{
		ID: "payments.by_shift", Group: "payments", Title: "Cobros por turno",
		Description: "Cobros de cada turno con su fecha comercial (vista por turno); los turnos en arqueo se excluyen.",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis + " Turno de cada registro.",
		Filters: paymentFilters(),
		Columns: withColumns([]Column{textColumn("register", "Caja"), dateColumn("business_date", "Fecha comercial del turno"),
			textColumn("opened_by", "Abrió"), textColumn("shift_state", "Estado")}, paymentMeasureColumns),
		DefaultSort: Sort{Key: "business_date"},
		Query: paymentsBy(`register.name AS register,coalesce(s.business_date,(s.opened_at AT TIME ZONE @tz)::date) AS business_date,
                opener.display_name AS opened_by,CASE s.state WHEN 'open' THEN 'Abierto' ELSE 'Cerrado' END AS shift_state`,
			shiftJoins, "m.shift_id,register.name,s.business_date,s.opened_at,opener.display_name,s.state"),
	})
	register(Definition{
		ID: "payments.by_user", Group: "payments", Title: "Cobros por usuario",
		Description: "Cobros por el usuario que los registró (las devoluciones, por quien devolvió).",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis, Filters: paymentFilters(),
		Columns:     withColumns([]Column{textColumn("user", "Usuario")}, paymentMeasureColumns),
		DefaultSort: Sort{Key: "net", Descending: true},
		Query:       paymentsBy("actor.display_name AS \"user\"", "JOIN app.users actor ON actor.id=m.actor_id", "m.actor_id,actor.display_name"),
	})
	register(Definition{
		ID: "payments.by_day", Group: "payments", Title: "Cobros por día",
		Description: "Cobros por día comercial (vista por día, sin depender de los turnos).",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis, Filters: paymentFilters(),
		Columns:     withColumns([]Column{dateColumn("day", "Día")}, paymentMeasureColumns),
		DefaultSort: Sort{Key: "day"},
		Query:       paymentsBy(businessDate("m.at")+" AS day", "", "1"),
	})
	registerPaymentDetails()
}
