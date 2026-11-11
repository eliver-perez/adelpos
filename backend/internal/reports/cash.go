package reports

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/payments"
)

// Reportes de caja (§12, fase B2). Fuentes: los registros reales de las etapas 6 y 7 (turnos,
// movimientos de cobro por turno, operaciones de efectivo, cortes y cierres). Dos vistas (§0-B):
//   - por turno, con la fecha comercial del turno (fecha de apertura en la zona del establecimiento,
//     como el historial de Caja; no usa la hora de inicio del día);
//   - por día, con la hora de inicio del día, para quien trabaja con un turno permanente.
//
// Paridad exacta con Caja:
//   - sin cash.expected.read no se calculan ni se envían el esperado, el efectivo físico recibido y
//     entregado por moneda, las diferencias, el resultado (sobrante o faltante) ni sus equivalencias;
//     lo demás (fondo, cobros por método, operaciones de efectivo por moneda y lo contado al cerrar)
//     es lo mismo que Caja muestra a quien consulta el turno;
//   - un turno en arqueo (closing) se excluye de todo agregado de caja y de cobros para todos, con un
//     aviso que solo dice cuántos turnos; en la lista de turnos aparece sin ningún importe;
//   - solo se incluyen los turnos que el usuario puede consultar en Caja (cash.shifts.read; otras
//     cajas con cash.shifts.read_all; cerrados con cash.history.read o si él los cerró), para que un
//     reporte consolidado no revele lo que Caja no le mostraría.
//
// El esperado de un turno abierto es payments.ExpectedCash (el cálculo único de cash_expected.go)
// sobre los componentes que suma aquí el mismo SQL que readCashFlows; el de un turno cerrado es el
// que guardó su cierre. Los descuentos no se atribuyen a turnos (no se replica readAdjustmentTotals).

const (
	cashShiftDateBasis = "Fecha comercial del turno: la fecha de su apertura en la zona del establecimiento (no usa la " +
		"hora de inicio del día), como el historial de Caja. Los importes son los de todo el turno."
	cashDayDateBasis = "Fecha de registro de cada cobro, devolución, operación de efectivo, apertura, corte y cierre, " +
		"agrupada por día con la hora de inicio del día (vista por día, sin depender de los turnos)."
	cashShiftsRead    = "cash.shifts.read"
	cashShiftsReadAll = "cash.shifts.read_all"
	cashHistoryRead   = "cash.history.read"
)

// cashScopeSQL limita los turnos (alias s) a los que el usuario puede consultar en Caja.
const cashScopeSQL = `(@scope_read AND (@scope_all OR s.register_id =
        (SELECT d.register_id FROM app.devices d WHERE d.id::text = @scope_device))
    AND (s.state <> 'closed' OR @scope_history OR s.closed_by::text = @scope_user))`

// shiftBusinessDateSQL es la fecha comercial del turno s (fecha calendario de apertura).
const shiftBusinessDateSQL = `coalesce(s.business_date,(s.opened_at AT TIME ZONE @tz)::date)`

// cashArgs agrega a los argumentos comunes los del alcance de Caja y los días del periodo.
func cashArgs(query *Query) pgx.NamedArgs {
	args := query.Args()
	args["scope_read"] = query.Actor.Can(cashShiftsRead)
	args["scope_all"] = query.Actor.Can(cashShiftsReadAll)
	args["scope_history"] = query.Actor.Can(cashHistoryRead)
	args["scope_user"] = query.Actor.UserID
	args["scope_device"] = query.Actor.DeviceID
	if query.Params.Period != nil {
		args["first_day"] = query.Params.Period.FirstDay
		args["last_day"] = query.Params.Period.LastDay
	}
	return args
}

// expectedVisible indica si el reporte puede calcular y enviar el esperado y las diferencias.
func expectedVisible(query *Query) bool {
	return query.Actor.Can(PermissionCashExpected)
}

// protectedCash marca una columna de efectivo físico, esperado o diferencias (cash.expected.read).
func protectedCash(column Column) Column {
	return requiring(column, PermissionCashExpected)
}

// cashWarnings avisa de los turnos en arqueo excluidos (solo cuántos) y del alcance limitado.
func cashWarnings(ctx context.Context, query *Query, args pgx.NamedArgs) ([]Warning, error) {
	warnings := []Warning{}
	condition := "true"
	if query.Params.Period != nil {
		condition = "s.opened_at < @until"
	}
	var closing int
	err := query.Tx.QueryRow(ctx, `SELECT count(*) FROM app.cash_shifts s WHERE s.state='closing' AND `+condition+
		` AND `+cashScopeSQL, args).Scan(&closing)
	if err != nil {
		return nil, err
	}
	if closing > 0 {
		warnings = append(warnings, Warning{Code: "SHIFT_CLOSING", Message: "Turnos en arqueo excluidos: " + strconv.Itoa(closing) +
			". Sus importes se incluirán cuando termine el arqueo."})
	}
	actor := query.Actor
	if !actor.Can(cashShiftsRead) || !actor.Can(cashShiftsReadAll) || !actor.Can(cashHistoryRead) {
		warnings = append(warnings, Warning{Code: "CASH_SCOPE", Message: "Solo se incluyen los turnos que puedes consultar en " +
			"Caja (los de otras cajas requieren consultar todas las cajas y los cerrados, el historial de turnos)."})
	}
	return warnings, nil
}

// Estados de un turno.
func shiftStateLabel(state string) string {
	switch state {
	case "open":
		return "Abierto"
	case "closing":
		return "En arqueo (sin importes)"
	}
	return "Cerrado"
}

// cashShiftFilters son los filtros de las vistas por turno.
func cashShiftFilters() []Filter {
	return []Filter{filterRegister(), filterUser("Abrió el turno"), filterChoice("state", "Estado del turno", "",
		Option{Value: "open", Label: "Abierto"}, Option{Value: "closing", Label: "En arqueo"},
		Option{Value: "closed", Label: "Cerrado"})}
}

// shiftScopeConditions son las condiciones de los turnos (alias s) de las vistas por turno.
func shiftScopeConditions(query *Query) conditions {
	list := conditions{cashScopeSQL}
	if query.Params.Period != nil {
		list.add(shiftBusinessDateSQL + " BETWEEN @first_day::date AND @last_day::date")
	}
	list.when(query, "register_id", "s.register_id = @register_id::uuid")
	list.when(query, "user_id", "s.opened_by = @user_id::uuid")
	list.when(query, "state", "s.state = @state")
	list.when(query, "shift_id", "s.id = @shift_id::uuid")
	return list
}

// shiftMetricsSQL arma la consulta de los turnos filtrados con sus totales: cobros (como el corte:
// ventas sin devoluciones, propinas, devoluciones y neto por movimiento), operaciones de efectivo
// por moneda sin regularizaciones, cortes parciales, cierre y venta reconocida. Con protected agrega
// los componentes del esperado y el efectivo físico por moneda (solo con cash.expected.read).
func shiftMetricsSQL(list conditions, protected bool) string {
	physical := ""
	if protected {
		physical = `,
            coalesce(sum(` + payments.ExpectedPaymentsMXNSQL + `) FILTER (WHERE c.is_cash),0) AS flow_mxn,
            coalesce(sum(` + payments.ExpectedPaymentsUSDSQL + `) FILTER (WHERE c.is_cash),0) AS flow_usd,
            coalesce(sum(CASE WHEN c.mxn_in IS NULL THEN c.cash_received ELSE c.mxn_in END) FILTER (WHERE c.is_cash),0) AS mxn_in,
            coalesce(sum(CASE WHEN c.mxn_in IS NULL THEN c.cash_received-c.net_amount ELSE c.mxn_out END)
                FILTER (WHERE c.is_cash),0) AS mxn_out,
            coalesce(sum(coalesce(c.usd_in,0)) FILTER (WHERE c.is_cash),0) AS usd_in,
            coalesce(sum(coalesce(c.usd_out,0)) FILTER (WHERE c.is_cash),0) AS usd_out`
	}
	closure := ""
	if protected {
		closure = `,cl.expected_mxn AS closure_expected_mxn,cl.expected_usd AS closure_expected_usd,
            cl.difference_mxn,cl.difference_usd,cl.difference_equivalent_mxn`
	}
	return `WITH scoped AS (
            SELECT s.* FROM app.cash_shifts s WHERE true` + list.sql() + `
        ),
        movements AS (
            SELECT c.shift_id,
                count(*) FILTER (WHERE c.kind='payment')::bigint AS payments,
                count(*) FILTER (WHERE c.kind='refund')::bigint AS refund_count,
                coalesce(sum(c.sale_amount) FILTER (WHERE c.kind<>'refund'),0) AS sales,
                coalesce(sum(c.tip_amount) FILTER (WHERE c.kind<>'refund'),0) AS tips,
                coalesce(-sum(c.net_amount) FILTER (WHERE c.kind='refund'),0) AS refunds,
                coalesce(sum(c.sale_amount),0) AS collected,
                coalesce(sum(c.tip_amount),0) AS net_tips,
                coalesce(sum(c.net_amount),0) AS net,
                coalesce(sum(c.net_amount) FILTER (WHERE c.is_cash),0) AS cash_net` + physical + `
            FROM app.cash_movements c WHERE c.shift_id IN (SELECT id FROM scoped) GROUP BY c.shift_id
        ),
        operations AS (
            SELECT o.shift_id,
                count(*) FILTER (WHERE NOT o.regularization)::bigint AS operations,
                count(*) FILTER (WHERE o.regularization)::bigint AS regularizations,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='income' AND o.currency='MXN' AND NOT o.regularization),0) AS income_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='withdrawal' AND o.currency='MXN' AND NOT o.regularization),0)
                    AS withdrawals_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='expense' AND o.currency='MXN' AND NOT o.regularization),0) AS expenses_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='correction' AND o.direction='in' AND o.currency='MXN'
                    AND NOT o.regularization),0) AS corrections_in_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='correction' AND o.direction='out' AND o.currency='MXN'
                    AND NOT o.regularization),0) AS corrections_out_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='income' AND o.currency='USD' AND NOT o.regularization),0) AS income_usd,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='withdrawal' AND o.currency='USD' AND NOT o.regularization),0)
                    AS withdrawals_usd,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='expense' AND o.currency='USD' AND NOT o.regularization),0) AS expenses_usd,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='correction' AND o.direction='in' AND o.currency='USD'
                    AND NOT o.regularization),0) AS corrections_in_usd,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='correction' AND o.direction='out' AND o.currency='USD'
                    AND NOT o.regularization),0) AS corrections_out_usd
            FROM app.cash_operations o WHERE o.shift_id IN (SELECT id FROM scoped) GROUP BY o.shift_id
        ),
        cuts AS (
            SELECT r.shift_id,count(*) FILTER (WHERE r.kind='partial')::bigint AS partial_cuts
            FROM app.cash_shift_reports r WHERE r.shift_id IN (SELECT id FROM scoped) GROUP BY r.shift_id
        ),
        sales AS (
            SELECT r.shift_id,count(DISTINCT r.account_id) FILTER (WHERE r.recognition_kind <> 'resettlement')::bigint AS accounts,
                coalesce(sum(r.quantity),0)::bigint AS units,coalesce(sum(r.net),0) AS recognized_net,
                coalesce(sum(r.tax),0) AS recognized_tax
            FROM app.sales_recognitions r WHERE r.shift_id IN (SELECT id FROM scoped) GROUP BY r.shift_id
        )
        SELECT s.id::text AS shift_key,s.state AS state_code,s.register_id::text AS register_key,register.name AS register,
            ` + shiftBusinessDateSQL + ` AS business_date,s.opened_at,s.closed_at,opener.display_name AS opened_by,
            coalesce(closer.display_name,'') AS closed_by,s.opening_float,s.opening_float_usd,s.usd_rate,
            coalesce(m.payments,0) AS payments,coalesce(m.refund_count,0) AS refund_count,coalesce(m.sales,0) AS sales,
            coalesce(m.tips,0) AS tips,coalesce(m.refunds,0) AS refunds,coalesce(m.collected,0) AS collected,
            coalesce(m.net_tips,0) AS net_tips,coalesce(m.net,0) AS net,coalesce(m.cash_net,0) AS cash_net,
            coalesce(o.operations,0) AS operations,coalesce(o.regularizations,0) AS regularizations,
            coalesce(o.income_mxn,0) AS income_mxn,coalesce(o.withdrawals_mxn,0) AS withdrawals_mxn,
            coalesce(o.expenses_mxn,0) AS expenses_mxn,coalesce(o.corrections_in_mxn,0) AS corrections_in_mxn,
            coalesce(o.corrections_out_mxn,0) AS corrections_out_mxn,coalesce(o.income_usd,0) AS income_usd,
            coalesce(o.withdrawals_usd,0) AS withdrawals_usd,coalesce(o.expenses_usd,0) AS expenses_usd,
            coalesce(o.corrections_in_usd,0) AS corrections_in_usd,coalesce(o.corrections_out_usd,0) AS corrections_out_usd,
            coalesce(cu.partial_cuts,0) AS partial_cuts,(cl.shift_id IS NOT NULL) AS has_closure,
            cl.counted_mxn,cl.counted_usd,
            coalesce(x.accounts,0) AS accounts,coalesce(x.units,0) AS units,coalesce(x.recognized_net,0) AS recognized_net,
            coalesce(x.recognized_tax,0) AS recognized_tax` + closure + strippedPhysical(protected) + `
        FROM scoped s
        JOIN app.registers register ON register.id=s.register_id
        JOIN app.users opener ON opener.id=s.opened_by
        LEFT JOIN app.users closer ON closer.id=s.closed_by
        LEFT JOIN movements m ON m.shift_id=s.id
        LEFT JOIN operations o ON o.shift_id=s.id
        LEFT JOIN cuts cu ON cu.shift_id=s.id
        LEFT JOIN sales x ON x.shift_id=s.id
        LEFT JOIN app.cash_shift_closures cl ON cl.shift_id=s.id
        ORDER BY s.opened_at,s.id`
}

// strippedPhysical selecciona los componentes físicos solo cuando se pueden ver.
func strippedPhysical(protected bool) string {
	if !protected {
		return ""
	}
	return `,coalesce(m.flow_mxn,0) AS flow_mxn,coalesce(m.flow_usd,0) AS flow_usd,coalesce(m.mxn_in,0) AS mxn_in,
            coalesce(m.mxn_out,0) AS mxn_out,coalesce(m.usd_in,0) AS usd_in,coalesce(m.usd_out,0) AS usd_out`
}

// shiftIdentityKeys son las claves que conserva la fila de un turno en arqueo (sin importes).
var shiftIdentityKeys = map[string]bool{
	"shift_key": true, "state_code": true, "register_key": true, "register": true, "business_date": true, "opened_at": true,
	"closed_at": true, "opened_by": true, "closed_by": true, "state": true,
}

// cashShiftRows consulta los turnos con sus totales y completa en Go el estado, el esperado (con
// payments.ExpectedCash o el guardado por el cierre), las diferencias y los resultados. Un turno en
// arqueo queda sin importes para todos.
func cashShiftRows(ctx context.Context, query *Query, list conditions) ([]Row, error) {
	protected := expectedVisible(query)
	rows, err := query.collect(ctx, shiftMetricsSQL(list, protected), cashArgs(query))
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		state, _ := row["state_code"].(string)
		row["state"] = shiftStateLabel(state)
		if state == "closing" {
			for key := range row {
				if !shiftIdentityKeys[key] {
					row[key] = nil
				}
			}
			continue
		}
		completeShiftCash(row, state, protected)
	}
	return rows, nil
}

// completeShiftCash calcula los valores de caja derivados de un turno visible.
func completeShiftCash(row Row, state string, protected bool) {
	inMXN := decimalOf(row, "income_mxn").Add(decimalOf(row, "corrections_in_mxn"))
	outMXN := decimalOf(row, "withdrawals_mxn").Add(decimalOf(row, "expenses_mxn")).Add(decimalOf(row, "corrections_out_mxn"))
	inUSD := decimalOf(row, "income_usd").Add(decimalOf(row, "corrections_in_usd"))
	outUSD := decimalOf(row, "withdrawals_usd").Add(decimalOf(row, "expenses_usd")).Add(decimalOf(row, "corrections_out_usd"))
	row["operations_net_mxn"] = inMXN.Sub(outMXN)
	row["operations_net_usd"] = inUSD.Sub(outUSD)
	if !protected {
		return
	}
	hasClosure, _ := row["has_closure"].(bool)
	switch {
	case hasClosure:
		row["expected_mxn"] = row["closure_expected_mxn"]
		row["expected_usd"] = row["closure_expected_usd"]
		row["result"] = differenceLabel(decimalOf(row, "difference_mxn"), decimalOf(row, "difference_usd"))
		addOverShort(row, decimalOf(row, "difference_mxn"), decimalOf(row, "difference_usd"))
	case state == "open":
		mxn, usd := payments.ExpectedCash(payments.CashFlowTotals{
			FloatMXN:         decimalOf(row, "opening_float"),
			FloatUSD:         decimalOf(row, "opening_float_usd"),
			PaymentsMXN:      decimalOf(row, "flow_mxn"),
			PaymentsUSD:      decimalOf(row, "flow_usd"),
			OperationsInMXN:  inMXN,
			OperationsOutMXN: outMXN,
			OperationsInUSD:  inUSD,
			OperationsOutUSD: outUSD,
		})
		row["expected_mxn"] = mxn
		row["expected_usd"] = usd
	}
}

// differenceLabel resume el resultado del arqueo por moneda.
func differenceLabel(mxn, usd decimal.Decimal) string {
	label := func(value decimal.Decimal, currency string) string {
		switch value.Sign() {
		case 1:
			return "Sobrante en " + currency
		case -1:
			return "Faltante en " + currency
		}
		return ""
	}
	parts := []string{}
	for _, part := range []string{label(mxn, "pesos"), label(usd, "dólares")} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "Sin diferencia"
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[0] + " · " + parts[1]
}

// addOverShort separa sobrantes y faltantes por moneda (los faltantes en positivo).
func addOverShort(row Row, mxn, usd decimal.Decimal) {
	row["over_mxn"] = decimal.Max(mxn, decimal.Zero)
	row["short_mxn"] = decimal.Max(mxn.Neg(), decimal.Zero)
	row["over_usd"] = decimal.Max(usd, decimal.Zero)
	row["short_usd"] = decimal.Max(usd.Neg(), decimal.Zero)
}

// Columnas de caja reutilizadas.
var (
	columnShiftRegister     = textColumn("register", "Caja")
	columnShiftDate         = dateColumn("business_date", "Fecha comercial")
	columnShiftState        = textColumn("state", "Estado")
	columnShiftOpenedBy     = textColumn("opened_by", "Abrió")
	columnCashPayments      = described(countColumn("payments", "Cobros"), "Pagos registrados en el turno (incluye el pago nuevo de una corrección).")
	columnCashSales         = described(moneyColumn("sales", "Ventas cobradas"), "Importe aplicado a la venta de los cobros, menos los reversos de corrección; sin propinas ni devoluciones.")
	columnCashTips          = described(moneyColumn("tips", "Propinas"), "Propinas cobradas.")
	columnCashRefunds       = described(moneyColumn("refunds", "Devoluciones"), "Devoluciones registradas (con su propina).")
	columnCashNet           = described(moneyColumn("net", "Neto cobrado"), "Ventas cobradas + propinas − devoluciones, de todos los métodos (como el corte).")
	columnCashMethodNet     = described(moneyColumn("cash_net", "Neto en efectivo"), "Neto de los cobros y devoluciones con métodos de efectivo, como el total del método en Caja.")
	columnIncomeMXN         = described(moneyColumn("income_mxn", "Ingresos"), "Ingresos de efectivo en pesos distintos de una venta.")
	columnWithdrawalsMXN    = described(moneyColumn("withdrawals_mxn", "Retiros"), "Retiros de efectivo en pesos.")
	columnExpensesMXN       = described(moneyColumn("expenses_mxn", "Gastos"), "Gastos pagados en efectivo en pesos.")
	columnCorrectionsInMXN  = optional(moneyColumn("corrections_in_mxn", "Correcciones (entrada)"))
	columnCorrectionsOutMXN = optional(moneyColumn("corrections_out_mxn", "Correcciones (salida)"))
	columnIncomeUSD         = optional(dollarColumn("income_usd", "Ingresos en dólares"))
	columnWithdrawalsUSD    = optional(dollarColumn("withdrawals_usd", "Retiros en dólares"))
	columnExpensesUSD       = optional(dollarColumn("expenses_usd", "Gastos en dólares"))
	columnPartialCuts       = described(countColumn("partial_cuts", "Cortes parciales"), "Cortes parciales registrados (informativos: no cierran el turno).")
	columnCountedMXN        = described(moneyColumn("counted_mxn", "Contado en pesos"), "Lo declarado en el arqueo de cierre.")
	columnCountedUSD        = described(dollarColumn("counted_usd", "Contado en dólares"), "Lo declarado en el arqueo de cierre.")
	columnExpectedMXN       = protectedCash(described(moneyColumn("expected_mxn", "Esperado en pesos"),
		"Efectivo esperado del turno (al cierre, el guardado por el cierre; abierto, el vigente)."))
	columnExpectedUSD     = protectedCash(described(dollarColumn("expected_usd", "Esperado en dólares"), "Efectivo esperado en dólares."))
	columnMXNIn           = protectedCash(optional(described(moneyColumn("mxn_in", "Pesos recibidos"), "Pesos físicos recibidos en cobros.")))
	columnMXNOut          = protectedCash(optional(described(moneyColumn("mxn_out", "Pesos entregados"), "Cambio y devoluciones en pesos.")))
	columnUSDIn           = protectedCash(optional(described(dollarColumn("usd_in", "Dólares recibidos"), "Dólares físicos recibidos.")))
	columnUSDOut          = protectedCash(optional(described(dollarColumn("usd_out", "Cambio en dólares"), "Dólares físicos entregados.")))
	columnDifferenceMXN   = protectedCash(described(moneyColumn("difference_mxn", "Diferencia en pesos"), "Contado − esperado."))
	columnDifferenceUSD   = protectedCash(described(dollarColumn("difference_usd", "Diferencia en dólares"), "Contado − esperado."))
	columnDifferenceEquiv = protectedCash(optional(described(moneyColumn("difference_equivalent_mxn", "Diferencia equivalente en pesos"),
		"Diferencia en pesos más la de dólares al tipo de cambio del turno (informativa).")))
	columnOverMXN  = protectedCash(described(moneyColumn("over_mxn", "Sobrantes en pesos"), "Suma de las diferencias positivas."))
	columnShortMXN = protectedCash(described(moneyColumn("short_mxn", "Faltantes en pesos"), "Suma de las diferencias negativas, en positivo."))
	columnOverUSD  = protectedCash(optional(described(dollarColumn("over_usd", "Sobrantes en dólares"), "Suma de las diferencias positivas.")))
	columnShortUSD = protectedCash(optional(described(dollarColumn("short_usd", "Faltantes en dólares"),
		"Suma de las diferencias negativas, en positivo.")))
)

// cashFigures cuenta los turnos por estado y, con cash.expected.read, suma sobrantes y faltantes.
func cashFigures(rows []Row, protected bool) []Figure {
	var open, closing, closed int64
	for _, row := range rows {
		switch row["state_code"] {
		case "open":
			open++
		case "closing":
			closing++
		default:
			closed++
		}
	}
	figures := []Figure{
		{Key: "shifts", Label: "Turnos", Type: TypeInteger, Value: int64(len(rows))},
		{Key: "open_shifts", Label: "Abiertos", Type: TypeInteger, Value: open},
		{Key: "closing_shifts", Label: "En arqueo", Type: TypeInteger, Value: closing},
		{Key: "closed_shifts", Label: "Cerrados", Type: TypeInteger, Value: closed},
	}
	if !protected {
		return figures
	}
	over, short := decimal.Zero, decimal.Zero
	for _, row := range rows {
		over = over.Add(decimalOf(row, "over_mxn"))
		short = short.Add(decimalOf(row, "short_mxn"))
	}
	return append(figures,
		Figure{Key: "over_mxn", Label: "Sobrantes en pesos", Type: TypeMoneyMXN, Value: over, Permissions: []string{PermissionCashExpected}},
		Figure{Key: "short_mxn", Label: "Faltantes en pesos", Type: TypeMoneyMXN, Value: short, Permissions: []string{PermissionCashExpected}})
}

// cashDataset arma el conjunto con los avisos de caja.
func cashDataset(ctx context.Context, query *Query, rows []Row) (*Dataset, error) {
	warnings, err := cashWarnings(ctx, query, cashArgs(query))
	if err != nil {
		return nil, err
	}
	return &Dataset{Rows: rows, Warnings: warnings}, nil
}

func init() {
	register(Definition{
		ID: "cash.shifts", Group: "cash", Title: "Turnos de caja",
		Description: "Historial por turno con su fecha comercial: aperturas, fondos, cobros, operaciones de efectivo, cortes, " +
			"cierres y, con permiso, esperado y diferencias por moneda con el tipo de cambio del turno.",
		Permissions: []string{PermissionCash}, DateBasis: cashShiftDateBasis, Filters: cashShiftFilters(),
		Columns: []Column{columnShiftRegister, columnShiftDate, columnShiftState, dateTimeColumn("opened_at", "Apertura"),
			optional(dateTimeColumn("closed_at", "Cierre")), columnShiftOpenedBy, optional(textColumn("closed_by", "Cerró")),
			described(moneyColumn("opening_float", "Fondo inicial"), "Fondo inicial en pesos."),
			optional(dollarColumn("opening_float_usd", "Fondo en dólares")),
			optional(described(Column{Key: "usd_rate", Label: "Tipo de cambio", Type: TypeQuantity, Sortable: true},
				"Tipo de cambio fijo del turno (histórico).")),
			columnCashPayments, columnCashSales, columnCashTips, columnCashRefunds, columnCashNet, columnCashMethodNet,
			columnIncomeMXN, columnWithdrawalsMXN, columnExpensesMXN, columnCorrectionsInMXN, columnCorrectionsOutMXN,
			columnIncomeUSD, columnWithdrawalsUSD, columnExpensesUSD,
			optional(countColumn("regularizations", "Regularizaciones")), columnPartialCuts, columnCountedMXN, columnCountedUSD,
			columnExpectedMXN, columnExpectedUSD, columnMXNIn, columnMXNOut, columnUSDIn, columnUSDOut, columnDifferenceMXN,
			columnDifferenceUSD, columnDifferenceEquiv, protectedCash(textColumn("result", "Resultado"))},
		DefaultSort: Sort{Key: "opened_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := cashShiftRows(ctx, query, shiftScopeConditions(query))
			if err != nil {
				return nil, err
			}
			dataset, err := cashDataset(ctx, query, rows)
			if err != nil {
				return nil, err
			}
			dataset.Summary = cashFigures(rows, expectedVisible(query))
			dataset.Notes = append(dataset.Notes, "Un turno permanente aparece en la fecha de su apertura con todos sus importes; "+
				"lo ocurrido en cada día está en «Caja por día».")
			return dataset, nil
		},
	})
	register(Definition{
		ID: "cash.sales_collections_by_shift", Group: "cash", Title: "Ventas y cobros por turno",
		Description: "Lo vendido (cuentas cuyo cierre ocurrió en el turno) y lo cobrado (pagos del turno) en cada turno, sin sumarlos entre sí.",
		Permissions: []string{PermissionCash}, DateBasis: cashShiftDateBasis +
			" La venta pertenece al turno de la operación que cerró la cuenta; el cobro, al turno del pago.",
		Filters: []Filter{filterRegister(), filterUser("Abrió el turno")},
		Columns: []Column{columnShiftRegister, columnShiftDate, columnShiftState, columnShiftOpenedBy,
			requiring(described(countColumn("accounts", "Cuentas cerradas"), "Cuentas cuyo cierre se registró en el turno."), PermissionSales),
			requiring(described(countColumn("units", "Unidades vendidas"), "Unidades comerciales reconocidas en el turno."), PermissionSales),
			requiring(described(moneyColumn("recognized_net", "Venta neta"), "Venta reconocida al cerrar las cuentas en el turno (IVA incluido)."),
				PermissionSales),
			requiring(optional(moneyColumn("recognized_tax", "IVA de la venta")), PermissionSales),
			columnCashPayments,
			described(moneyColumn("collected", "Cobrado neto"), "Cobros − devoluciones − reversos de corrección, sin propinas."),
			described(moneyColumn("net_tips", "Propinas netas"), "Propinas cobradas − propinas devueltas."),
			columnCashRefunds},
		DefaultSort: Sort{Key: "business_date"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := cashShiftRows(ctx, query, shiftScopeConditions(query))
			if err != nil {
				return nil, err
			}
			dataset, err := cashDataset(ctx, query, rows)
			if err != nil {
				return nil, err
			}
			dataset.Notes = append(dataset.Notes, "La venta y el cobro son conceptos distintos: una cuenta puede cobrarse en un turno y "+
				"cerrarse en otro. Lo reconocido sin turno (cortesías o entregas sin turno abierto) no aparece aquí.")
			return dataset, nil
		},
	})
	register(Definition{
		ID: "cash.by_register", Group: "cash", Title: "Historial por caja",
		Description: "Totales de los turnos de cada caja en el periodo y, con permiso, sobrantes y faltantes por moneda.",
		Permissions: []string{PermissionCash}, DateBasis: cashShiftDateBasis, Filters: []Filter{filterRegister()},
		Columns: []Column{columnShiftRegister, countColumn("shifts", "Turnos"), countColumn("closed_shifts", "Cerrados"),
			countColumn("closing_shifts", "En arqueo"), described(moneyColumn("opening_float", "Fondos iniciales"), "Suma de los fondos iniciales en pesos."),
			columnCashPayments, columnCashSales, columnCashTips, columnCashRefunds, columnCashNet, columnCashMethodNet,
			columnIncomeMXN, columnWithdrawalsMXN, columnExpensesMXN, columnIncomeUSD, columnWithdrawalsUSD, columnExpensesUSD,
			columnPartialCuts, columnCountedMXN, columnCountedUSD, columnOverMXN, columnShortMXN, columnDifferenceMXN, columnOverUSD,
			columnShortUSD, columnDifferenceUSD, columnDifferenceEquiv},
		DefaultSort: Sort{Key: "register"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			shifts, err := cashShiftRows(ctx, query, shiftScopeConditions(query))
			if err != nil {
				return nil, err
			}
			return cashDataset(ctx, query, aggregateByRegister(shifts))
		},
	})
	registerCashDay()
	registerCashDetails()
}

// registerSums son las medidas que el historial por caja suma de sus turnos (los turnos en arqueo ya
// no tienen importes).
var registerSums = []string{
	"opening_float", "sales", "tips", "refunds", "net", "cash_net", "income_mxn", "withdrawals_mxn", "expenses_mxn",
	"income_usd", "withdrawals_usd", "expenses_usd", "counted_mxn", "counted_usd", "over_mxn", "short_mxn", "difference_mxn",
	"over_usd", "short_usd", "difference_usd", "difference_equivalent_mxn",
}

func aggregateByRegister(shifts []Row) []Row {
	order := []string{}
	byRegister := map[string]Row{}
	for _, shift := range shifts {
		key, _ := shift["register_key"].(string)
		row, found := byRegister[key]
		if !found {
			row = Row{"register": shift["register"], "shifts": int64(0), "closed_shifts": int64(0), "closing_shifts": int64(0),
				"payments": int64(0), "partial_cuts": int64(0)}
			for _, measure := range registerSums {
				row[measure] = decimal.Zero
			}
			byRegister[key] = row
			order = append(order, key)
		}
		row["shifts"] = row["shifts"].(int64) + 1
		switch shift["state_code"] {
		case "closing":
			row["closing_shifts"] = row["closing_shifts"].(int64) + 1
			continue
		case "closed":
			row["closed_shifts"] = row["closed_shifts"].(int64) + 1
		}
		if value, ok := shift["payments"].(int64); ok {
			row["payments"] = row["payments"].(int64) + value
		}
		if value, ok := shift["partial_cuts"].(int64); ok {
			row["partial_cuts"] = row["partial_cuts"].(int64) + value
		}
		for _, measure := range registerSums {
			row[measure] = decimalOf(row, measure).Add(decimalOf(shift, measure))
		}
	}
	rows := make([]Row, 0, len(order))
	for _, key := range order {
		rows = append(rows, byRegister[key])
	}
	return rows
}
