package reports

import (
	"context"

	"local/ad-el-pos/server/internal/payments"
)

// Caja por día (§0-B «Los reportes no dependen de los turnos»): lo ocurrido en cada día comercial
// (hora de inicio del día) en todos los turnos que el usuario puede consultar, sin depender de que
// los turnos se cierren. Un turno permanente reparte sus movimientos entre los días en que
// ocurrieron. Los turnos en arqueo se excluyen por completo (también sus días anteriores) para
// todos. El efectivo físico por moneda y el esperado, lo contado y las diferencias de los cierres
// del día exigen cash.expected.read; nunca se muestran para un turno en arqueo.

// cashDaySQL arma la consulta por día; protected agrega el efectivo físico y los esperados y
// diferencias de los cierres del día.
func cashDaySQL(list conditions, protected bool) string {
	physical, physicalSelect, closures, closureSelect := "", "", "", ""
	if protected {
		physical = `,
                coalesce(sum(` + physicalMXNInSQL + `) FILTER (WHERE c.is_cash),0) AS mxn_in,
                coalesce(sum(` + physicalMXNOutSQL + `) FILTER (WHERE c.is_cash),0) AS mxn_out,
                coalesce(sum(coalesce(c.usd_in,0)) FILTER (WHERE c.is_cash),0) AS usd_in,
                coalesce(sum(coalesce(c.usd_out,0)) FILTER (WHERE c.is_cash),0) AS usd_out,
                coalesce(sum(` + payments.ExpectedPaymentsMXNSQL + `) FILTER (WHERE c.is_cash),0) AS flow_mxn,
                coalesce(sum(` + payments.ExpectedPaymentsUSDSQL + `) FILTER (WHERE c.is_cash),0) AS flow_usd`
		physicalSelect = `,coalesce(m.mxn_in,0) AS mxn_in,coalesce(m.mxn_out,0) AS mxn_out,coalesce(m.usd_in,0) AS usd_in,
            coalesce(m.usd_out,0) AS usd_out,coalesce(m.flow_mxn,0)+coalesce(o.in_mxn,0)-coalesce(o.out_mxn,0) AS cash_flow_mxn,
            coalesce(m.flow_usd,0)+coalesce(o.in_usd,0)-coalesce(o.out_usd,0) AS cash_flow_usd`
		closures = `,coalesce(sum(cl.expected_mxn),0) AS closing_expected_mxn,coalesce(sum(cl.expected_usd),0) AS closing_expected_usd,
                coalesce(sum(cl.difference_mxn),0) AS difference_mxn,coalesce(sum(cl.difference_usd),0) AS difference_usd,
                coalesce(sum(greatest(cl.difference_mxn,0)),0) AS over_mxn,coalesce(sum(greatest(-cl.difference_mxn,0)),0) AS short_mxn,
                coalesce(sum(greatest(cl.difference_usd,0)),0) AS over_usd,coalesce(sum(greatest(-cl.difference_usd,0)),0) AS short_usd,
                sum(cl.difference_equivalent_mxn) AS difference_equivalent_mxn`
		closureSelect = `,coalesce(cl.closing_expected_mxn,0) AS closing_expected_mxn,
            coalesce(cl.closing_expected_usd,0) AS closing_expected_usd,coalesce(cl.difference_mxn,0) AS difference_mxn,
            coalesce(cl.difference_usd,0) AS difference_usd,coalesce(cl.over_mxn,0) AS over_mxn,coalesce(cl.short_mxn,0) AS short_mxn,
            coalesce(cl.over_usd,0) AS over_usd,coalesce(cl.short_usd,0) AS short_usd,cl.difference_equivalent_mxn`
	}
	return `WITH scoped AS (
            SELECT s.* FROM app.cash_shifts s WHERE s.state <> 'closing'` + list.sql() + `
        ),
        m AS (
            SELECT ` + businessDate("c.created_at") + ` AS day,
                count(*) FILTER (WHERE c.kind='payment')::bigint AS payments,
                coalesce(sum(c.sale_amount) FILTER (WHERE c.kind<>'refund'),0) AS sales,
                coalesce(sum(c.tip_amount) FILTER (WHERE c.kind<>'refund'),0) AS tips,
                coalesce(-sum(c.net_amount) FILTER (WHERE c.kind='refund'),0) AS refunds,
                coalesce(sum(c.net_amount),0) AS net,
                coalesce(sum(c.net_amount) FILTER (WHERE c.is_cash),0) AS cash_net` + physical + `
            FROM app.cash_movements c JOIN scoped s ON s.id=c.shift_id
            WHERE ` + inPeriod("c.created_at") + ` GROUP BY 1
        ),
        o AS (
            SELECT ` + businessDate("o.created_at") + ` AS day,
                count(*)::bigint AS operations,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='income' AND o.currency='MXN'),0) AS income_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='withdrawal' AND o.currency='MXN'),0) AS withdrawals_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='expense' AND o.currency='MXN'),0) AS expenses_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='correction' AND o.direction='in' AND o.currency='MXN'),0)
                    AS corrections_in_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='correction' AND o.direction='out' AND o.currency='MXN'),0)
                    AS corrections_out_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='income' AND o.currency='USD'),0) AS income_usd,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='withdrawal' AND o.currency='USD'),0) AS withdrawals_usd,
                coalesce(sum(o.amount) FILTER (WHERE o.kind='expense' AND o.currency='USD'),0) AS expenses_usd,
                coalesce(sum(o.amount) FILTER (WHERE o.direction='in' AND o.currency='MXN'),0) AS in_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.direction='out' AND o.currency='MXN'),0) AS out_mxn,
                coalesce(sum(o.amount) FILTER (WHERE o.direction='in' AND o.currency='USD'),0) AS in_usd,
                coalesce(sum(o.amount) FILTER (WHERE o.direction='out' AND o.currency='USD'),0) AS out_usd
            FROM app.cash_operations o JOIN scoped s ON s.id=o.shift_id
            WHERE NOT o.regularization AND ` + inPeriod("o.created_at") + ` GROUP BY 1
        ),
        op AS (
            SELECT ` + businessDate("s.opened_at") + ` AS day,count(*)::bigint AS openings,
                coalesce(sum(s.opening_float),0) AS opening_float,coalesce(sum(s.opening_float_usd),0) AS opening_float_usd
            FROM scoped s WHERE ` + inPeriod("s.opened_at") + ` GROUP BY 1
        ),
        cu AS (
            SELECT ` + businessDate("r.created_at") + ` AS day,count(*)::bigint AS partial_cuts
            FROM app.cash_shift_reports r JOIN scoped s ON s.id=r.shift_id
            WHERE r.kind='partial' AND ` + inPeriod("r.created_at") + ` GROUP BY 1
        ),
        cl AS (
            SELECT ` + businessDate("cl.closed_at") + ` AS day,count(*)::bigint AS closings,
                coalesce(sum(cl.counted_mxn),0) AS counted_mxn,coalesce(sum(cl.counted_usd),0) AS counted_usd` + closures + `
            FROM app.cash_shift_closures cl JOIN scoped s ON s.id=cl.shift_id
            WHERE ` + inPeriod("cl.closed_at") + ` GROUP BY 1
        ),
        days AS (
            SELECT day FROM m UNION SELECT day FROM o UNION SELECT day FROM op UNION SELECT day FROM cu UNION SELECT day FROM cl
        )
        SELECT d.day,coalesce(op.openings,0) AS openings,coalesce(op.opening_float,0) AS opening_float,
            coalesce(op.opening_float_usd,0) AS opening_float_usd,coalesce(m.payments,0) AS payments,coalesce(m.sales,0) AS sales,
            coalesce(m.tips,0) AS tips,coalesce(m.refunds,0) AS refunds,coalesce(m.net,0) AS net,coalesce(m.cash_net,0) AS cash_net,
            coalesce(o.operations,0) AS operations,coalesce(o.income_mxn,0) AS income_mxn,
            coalesce(o.withdrawals_mxn,0) AS withdrawals_mxn,coalesce(o.expenses_mxn,0) AS expenses_mxn,
            coalesce(o.corrections_in_mxn,0) AS corrections_in_mxn,coalesce(o.corrections_out_mxn,0) AS corrections_out_mxn,
            coalesce(o.income_usd,0) AS income_usd,coalesce(o.withdrawals_usd,0) AS withdrawals_usd,
            coalesce(o.expenses_usd,0) AS expenses_usd,coalesce(cu.partial_cuts,0) AS partial_cuts,coalesce(cl.closings,0) AS closings,
            coalesce(cl.counted_mxn,0) AS counted_mxn,coalesce(cl.counted_usd,0) AS counted_usd` + physicalSelect + closureSelect + `
        FROM days d
        LEFT JOIN m ON m.day=d.day LEFT JOIN o ON o.day=d.day LEFT JOIN op ON op.day=d.day LEFT JOIN cu ON cu.day=d.day
        LEFT JOIN cl ON cl.day=d.day
        ORDER BY d.day`
}

// cashDayConditions son los filtros de caja de la vista por día (sobre los turnos s).
func cashDayConditions(query *Query) conditions {
	list := conditions{cashScopeSQL}
	list.when(query, "register_id", "s.register_id = @register_id::uuid")
	list.when(query, "shift_id", "s.id = @shift_id::uuid")
	return list
}

func registerCashDay() {
	register(Definition{
		ID: "cash.by_day", Group: "cash", Title: "Caja por día",
		Description: "Cobros, propinas, devoluciones, operaciones de efectivo, aperturas, cortes y cierres de cada día comercial, " +
			"para restaurantes que trabajan con un turno permanente.",
		Permissions: []string{PermissionCash}, DateBasis: cashDayDateBasis,
		Filters: []Filter{filterRegister(), filterShift()},
		Columns: []Column{dateColumn("day", "Día"),
			described(countColumn("openings", "Aperturas"), "Turnos abiertos en el día."),
			described(moneyColumn("opening_float", "Fondos iniciales"), "Fondos en pesos de los turnos abiertos en el día."),
			optional(dollarColumn("opening_float_usd", "Fondos en dólares")),
			columnCashPayments, columnCashSales, columnCashTips, columnCashRefunds, columnCashNet, columnCashMethodNet,
			optional(countColumn("operations", "Operaciones de efectivo")),
			columnIncomeMXN, columnWithdrawalsMXN, columnExpensesMXN, columnCorrectionsInMXN, columnCorrectionsOutMXN,
			columnIncomeUSD, columnWithdrawalsUSD, columnExpensesUSD, columnPartialCuts,
			described(countColumn("closings", "Cierres"), "Turnos cerrados en el día."), columnCountedMXN, columnCountedUSD,
			columnMXNIn, columnMXNOut, columnUSDIn, columnUSDOut,
			protectedCash(optional(described(moneyColumn("cash_flow_mxn", "Flujo neto de efectivo en pesos"),
				"Pesos físicos de cobros y devoluciones más operaciones de efectivo del día."))),
			protectedCash(optional(described(dollarColumn("cash_flow_usd", "Flujo neto de efectivo en dólares"),
				"Dólares físicos de cobros más operaciones de efectivo del día."))),
			protectedCash(described(moneyColumn("closing_expected_mxn", "Esperado al cierre (pesos)"),
				"Esperado de los turnos que se cerraron en el día.")),
			protectedCash(optional(dollarColumn("closing_expected_usd", "Esperado al cierre (dólares)"))),
			columnOverMXN, columnShortMXN, columnDifferenceMXN, columnOverUSD, columnShortUSD, columnDifferenceUSD,
			columnDifferenceEquiv},
		DefaultSort: Sort{Key: "day"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, cashDaySQL(cashDayConditions(query), expectedVisible(query)), cashArgs(query))
			if err != nil {
				return nil, err
			}
			dataset, err := cashDataset(ctx, query, rows)
			if err != nil {
				return nil, err
			}
			dataset.Notes = append(dataset.Notes, "La fecha comercial de un turno es la de su apertura; esta vista agrupa cada "+
				"registro por el día en que ocurrió, con la hora de inicio del día. Lo contado y las diferencias son los de los "+
				"turnos que se cerraron ese día.")
			return dataset, nil
		},
	})
}
