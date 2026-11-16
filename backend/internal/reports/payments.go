package reports

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// Reportes de pagos y devoluciones (§10). Fuente: los registros financieros inmutables de payments
// (pagos, devoluciones y reversos de corrección) con su propia fecha (created_at): una devolución se
// refleja en el día en que se hizo, sin tocar la venta del día original. Cobrado neto = pagos −
// devoluciones − reversos de corrección (el pago nuevo de una corrección es un pago), de modo que
// una corrección nunca duplica lo cobrado. Las propinas se reportan aparte de la venta.
//
// Paridad con Caja: los cobros de un turno en arqueo (closing) se excluyen de todo agregado para
// todos, con un aviso; el efectivo físico por moneda (lo recibido, el cambio, los dólares) exige
// además cash.expected.read, igual que en Caja; y, como los reportes de caja, solo se incluyen los
// movimientos de los turnos que el usuario puede consultar en Caja (cashScopeSQL, con el aviso
// CASH_SCOPE): un reporte de cobros no revela otras cajas ni turnos cerrados que Caja no le
// mostraría (tampoco en el aviso de turnos en arqueo, en el dashboard ni en las exportaciones).
//
// Etapa 9H: con un lote importado vigente y el filtro «Datos», los cobros del sistema anterior
// (legacy_payments) se agregan como renglones propios marcados «Sistema anterior» (los del mismo día
// o del mismo usuario importado se juntan con los de AD-EL POS), tal como se guardaron y fuera del
// alcance de Caja y de los turnos en arqueo, que son de los turnos de AD-EL POS (legacy.go).

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

// legacyPaymentFilters son los filtros de cobros con «Datos» (reportes que incluyen el historial importado).
func legacyPaymentFilters() []Filter {
	return withDataSource(paymentFilters())
}

// legacyPaymentsSQL son los cobros del historial del periodo (alias m en la consulta que sigue), con el corte s y la
// venta v del anterior (legacyPaymentJoins).
const legacyPaymentsSQL = `WITH legacy_movements AS (
        SELECT p.batch_id,p.paid_at AS at,p.method,p.method_name,p.amount,p.legacy_shift_id,p.legacy_sale_id,p.sale_status,
            p.received_user_id,p.legacy_received_by,p.received_name,p.reference
        FROM app.legacy_payments p WHERE p.paid_at >= @from AND p.paid_at < @until
    )`

const legacyPaymentJoins = `
        LEFT JOIN app.legacy_cash_shifts s ON s.batch_id=m.batch_id AND s.legacy_id=m.legacy_shift_id
        LEFT JOIN app.legacy_sales v ON v.batch_id=m.batch_id AND v.legacy_id=m.legacy_sale_id`

// legacyPaymentMeasures son las medidas de cobros del historial: el anterior no registraba devoluciones, correcciones
// ni propinas.
const legacyPaymentMeasures = `count(*)::bigint AS payments,coalesce(sum(m.amount),0) AS collected,0::numeric AS refunds,
    0::numeric AS corrections,coalesce(sum(m.amount),0) AS net,0::numeric AS tips,coalesce(sum(m.amount),0) AS received_total,
    '` + LegacySourceLabel + `'::text AS data_source`

// legacyPaymentConditions son los filtros de cobros sobre el historial: el método, la caja y el turno son de AD-EL POS
// (lo excluyen); el usuario, el importado que recibió el pago.
func legacyPaymentConditions(query *Query) conditions {
	list := conditions{}
	list.excludeLegacy(query, "method_id", "register_id", "shift_id")
	list.when(query, "user_id", "m.received_user_id = @user_id::uuid")
	return list
}

// Expresiones del historial: la caja del cobro (la de su corte o, sin corte, la de su venta), la fecha comercial y el
// estado del corte.
const (
	legacyPaymentRegisterSQL = `coalesce(s.register_name,v.register_name,'Sin caja')`
	legacyShiftDateSQL       = `(s.opened_at AT TIME ZONE @tz)::date`
	legacyShiftStateSQL      = `CASE s.status WHEN 'open' THEN 'Abierto' WHEN 'closed' THEN 'Cerrado' WHEN 'unknown' THEN 'Desconocido' END`
)

// legacyPaymentsNote explica los cobros del historial.
const legacyPaymentsNote = "Sistema anterior: cobros tal como los guardó (también los de importe cero o negativo y los de ventas " +
	"que después se cancelaron o quedaron abiertas), sin devoluciones, correcciones ni propinas, que no registraba. No dependen " +
	"del alcance de Caja ni de los turnos en arqueo, que son de los turnos de AD-EL POS."

// legacyGrouping es la misma agrupación de un reporte de cobros sobre el historial (selectSQL y groupSQL, con m, s y v
// de legacyPaymentsSQL). merge, si no está vacía, es la clave con que se juntan los renglones de AD-EL POS y del
// historial (el día o el usuario).
type legacyGrouping struct {
	selectSQL string
	groupSQL  string
	merge     string
}

// paymentScope son las condiciones comunes de los turnos s de los cobros: sin turnos en arqueo y
// dentro del alcance de Caja del usuario.
func paymentScope() conditions {
	return conditions{"s.state <> 'closing'", cashScopeSQL}
}

func paymentConditions(query *Query) conditions {
	list := paymentScope()
	list.when(query, "method_id", "m.method_id = @method_id::uuid")
	list.when(query, "register_id", "m.register_id = @register_id::uuid")
	list.when(query, "shift_id", "m.shift_id = @shift_id::uuid")
	list.when(query, "user_id", "m.actor_id = @user_id::uuid")
	return list
}

// closingActivitySQL es la condición de un turno s en arqueo con movimientos en el periodo.
const closingActivitySQL = `s.state='closing' AND (
            EXISTS(SELECT FROM app.payments p WHERE p.shift_id=s.id AND p.created_at >= @from AND p.created_at < @until)
            OR EXISTS(SELECT FROM app.payment_refunds r WHERE r.shift_id=s.id AND r.created_at >= @from AND r.created_at < @until)
            OR EXISTS(SELECT FROM app.sales_recognitions x WHERE x.shift_id=s.id AND x.recognized_at >= @from AND x.recognized_at < @until))`

// closingShiftsWarning avisa cuántos turnos en arqueo tienen movimientos en el periodo (sus importes
// no se incluyen). Nunca dice cuánto. Lo usan los reportes que excluyen todos los turnos en arqueo
// sin limitarse al alcance de Caja (ventas por turno y empleados).
func closingShiftsWarning(ctx context.Context, query *Query) (*Warning, error) {
	return countClosingShifts(ctx, query, `SELECT count(*) FROM app.cash_shifts s WHERE `+closingActivitySQL, query.Args())
}

func countClosingShifts(ctx context.Context, query *Query, sql string, args pgx.NamedArgs) (*Warning, error) {
	var count int
	if err := query.Tx.QueryRow(ctx, sql, args).Scan(&count); err != nil || count == 0 {
		return nil, err
	}
	return &Warning{Code: "SHIFT_CLOSING", Message: "Turnos en arqueo excluidos: " + strconv.Itoa(count) +
		". Sus importes se incluirán cuando termine el arqueo."}, nil
}

// paymentWarnings son los avisos de un reporte de cobros: cuántos turnos en arqueo de su alcance se
// excluyeron y, si el alcance de Caja es limitado, CASH_SCOPE.
func paymentWarnings(ctx context.Context, query *Query) ([]Warning, error) {
	warnings := []Warning{}
	closing, err := countClosingShifts(ctx, query, `SELECT count(*) FROM app.cash_shifts s WHERE `+closingActivitySQL+
		` AND `+cashScopeSQL, cashArgs(query))
	if err != nil {
		return nil, err
	}
	if closing != nil {
		warnings = append(warnings, *closing)
	}
	if cashScopeLimited(query.Actor) {
		warnings = append(warnings, cashScopeWarning())
	}
	return warnings, nil
}

// paymentDataset ejecuta una consulta de movimientos (con los argumentos del alcance de Caja) y
// agrega sus avisos.
func paymentDataset(ctx context.Context, query *Query, sql string) (*Dataset, error) {
	rows, err := query.collect(ctx, sql, cashArgs(query))
	if err != nil {
		return nil, err
	}
	warnings, err := paymentWarnings(ctx, query)
	if err != nil {
		return nil, err
	}
	return &Dataset{Rows: rows, Warnings: warnings}, nil
}

// paymentsBy agrupa los movimientos por una dimensión y, con el historial, sus cobros por la misma dimensión.
func paymentsBy(selectSQL, joins, groupSQL string, legacy legacyGrouping) func(ctx context.Context, query *Query) (*Dataset, error) {
	return func(ctx context.Context, query *Query) (*Dataset, error) {
		dataset := &Dataset{Rows: []Row{}}
		if query.legacy.Native {
			native, err := paymentDataset(ctx, query, paymentMovements+` SELECT `+selectSQL+`,`+paymentMeasures+`
                FROM movements m JOIN app.cash_shifts s ON s.id=m.shift_id `+joins+`
                WHERE true`+paymentConditions(query).sql()+` GROUP BY `+groupSQL)
			if err != nil {
				return nil, err
			}
			query.markNative(native.Rows)
			dataset = native
		}
		if !query.legacy.History {
			return dataset, nil
		}
		rows, err := query.collect(ctx, legacyPaymentsSQL+` SELECT `+legacy.selectSQL+`,`+legacyPaymentMeasures+`
            FROM legacy_movements m`+legacyPaymentJoins+`
            WHERE true`+legacyPaymentConditions(query).sql()+` GROUP BY `+legacy.groupSQL, query.Args())
		if err != nil {
			return nil, err
		}
		dataset.Rows = append(dataset.Rows, rows...)
		if legacy.merge != "" {
			dataset.Rows = mergeRows(dataset.Rows, legacy.merge, query.definition.Columns)
		}
		dataset.Notes = append(dataset.Notes, legacyPaymentsNote)
		return dataset, nil
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
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis, Filters: legacyPaymentFilters(),
		Columns:     withLegacyColumns(withColumns([]Column{textColumn("method", "Método")}, paymentMeasureColumns), false),
		DefaultSort: Sort{Key: "net", Descending: true},
		Query: paymentsBy("(array_agg(m.method_name ORDER BY m.at DESC))[1] AS method", "", "m.method_id",
			legacyGrouping{selectSQL: "m.method_name AS method", groupSQL: "m.method,m.method_name"}),
	})
	register(Definition{
		ID: "payments.by_register", Group: "payments", Title: "Cobros por caja",
		Description: "Cobros, devoluciones, correcciones y propinas por caja física.",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis, Filters: legacyPaymentFilters(),
		Columns:     withLegacyColumns(withColumns([]Column{textColumn("register", "Caja")}, paymentMeasureColumns), false),
		DefaultSort: Sort{Key: "register"},
		Query: paymentsBy("register.name AS register", "JOIN app.registers register ON register.id=m.register_id", "m.register_id,register.name",
			legacyGrouping{selectSQL: legacyPaymentRegisterSQL + " AS register", groupSQL: "1"}),
	})
	register(Definition{
		ID: "payments.by_shift", Group: "payments", Title: "Cobros por turno",
		Description: "Cobros de cada turno con su fecha comercial (vista por turno); los turnos en arqueo se excluyen.",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis + " Turno de cada registro.",
		Filters: legacyPaymentFilters(),
		Columns: withLegacyColumns(withColumns([]Column{textColumn("register", "Caja"),
			dateColumn("business_date", "Fecha comercial del turno"), textColumn("opened_by", "Abrió"),
			textColumn("shift_state", "Estado")}, paymentMeasureColumns), false),
		DefaultSort: Sort{Key: "business_date"},
		Query: paymentsBy(`register.name AS register,coalesce(s.business_date,(s.opened_at AT TIME ZONE @tz)::date) AS business_date,
                opener.display_name AS opened_by,CASE s.state WHEN 'open' THEN 'Abierto' ELSE 'Cerrado' END AS shift_state`,
			shiftJoins, "m.shift_id,register.name,s.business_date,s.opened_at,opener.display_name,s.state",
			// Un renglón por corte del anterior; «Sin turno», los cobros sin corte.
			legacyGrouping{selectSQL: `CASE WHEN m.legacy_shift_id IS NULL THEN 'Sin turno' ELSE coalesce(min(s.register_name),
                    'Corte '||m.legacy_shift_id) END AS register,min(` + legacyShiftDateSQL + `) AS business_date,
                min(s.cashier_name) AS opened_by,min(` + legacyShiftStateSQL + `) AS shift_state`, groupSQL: "m.legacy_shift_id"}),
	})
	register(Definition{
		ID: "payments.by_user", Group: "payments", Title: "Cobros por usuario",
		Description: "Cobros por el usuario que los registró (las devoluciones, por quien devolvió).",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis, Filters: legacyPaymentFilters(),
		Columns:     withLegacyColumns(withColumns([]Column{textColumn("user", "Usuario")}, paymentMeasureColumns), false),
		DefaultSort: Sort{Key: "net", Descending: true},
		// user_ref (no se envía) junta los cobros de un usuario importado en los dos sistemas.
		Query: paymentsBy("actor.display_name AS \"user\",m.actor_id::text AS user_ref", "JOIN app.users actor ON actor.id=m.actor_id",
			"m.actor_id,actor.display_name", legacyGrouping{
				selectSQL: `coalesce(min(m.received_name),'Sin usuario') AS "user",
                    coalesce(m.received_user_id::text,'legacy:'||coalesce(m.legacy_received_by::text,'')) AS user_ref`,
				groupSQL: "m.received_user_id,m.legacy_received_by", merge: "user_ref"}),
	})
	register(Definition{
		ID: "payments.by_day", Group: "payments", Title: "Cobros por día",
		Description: "Cobros por día comercial (vista por día, sin depender de los turnos).",
		Permissions: []string{PermissionPayments}, DateBasis: paymentsDateBasis, Filters: legacyPaymentFilters(),
		Columns:     withLegacyColumns(withColumns([]Column{dateColumn("day", "Día")}, paymentMeasureColumns), false),
		DefaultSort: Sort{Key: "day"},
		Query: paymentsBy(businessDate("m.at")+" AS day", "", "1",
			legacyGrouping{selectSQL: businessDate("m.at") + " AS day", groupSQL: "1", merge: "day"}),
	})
	registerPaymentDetails()
}
