package reports

import (
	"context"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/businessday"
)

// Secciones del resumen ejecutivo de la fase B2: caja, compras, inventario, costos y margen. Cada
// sección exige los mismos permisos que sus reportes detallados y calcula sus valores con las mismas
// definiciones; una sección sin permiso no se calcula. Caja nunca incluye el esperado ni las
// diferencias salvo en su sección propia (con cash.expected.read) y excluye los turnos en arqueo para
// todos. El margen estimado solo tiene valor con datos suficientes (al menos una unidad vendida con
// costo); si no, viaja en null.

// periodCashArgs son los argumentos de caja para un periodo del dashboard.
func periodCashArgs(query *Query, period *businessday.Period) pgx.NamedArgs {
	args := cashArgs(query)
	args["from"] = period.From
	args["until"] = period.Until
	return args
}

func cashSection(ctx context.Context, query *Query, period *businessday.Period) (map[string]any, error) {
	args := periodCashArgs(query, period)
	var openings int64
	var income, withdrawals, expenses, cashNet string
	err := query.Tx.QueryRow(ctx, `WITH scoped AS (
            SELECT s.* FROM app.cash_shifts s WHERE s.state <> 'closing' AND `+cashScopeSQL+`
        )
        SELECT (SELECT count(*) FROM scoped s WHERE `+inPeriod("s.opened_at")+`),
            (SELECT coalesce(sum(o.amount) FILTER (WHERE o.kind='income'),0) FROM app.cash_operations o JOIN scoped s ON s.id=o.shift_id
                WHERE o.currency='MXN' AND NOT o.regularization AND `+inPeriod("o.created_at")+`)::text,
            (SELECT coalesce(sum(o.amount) FILTER (WHERE o.kind='withdrawal'),0) FROM app.cash_operations o JOIN scoped s ON s.id=o.shift_id
                WHERE o.currency='MXN' AND `+inPeriod("o.created_at")+`)::text,
            (SELECT coalesce(sum(o.amount) FILTER (WHERE o.kind='expense'),0) FROM app.cash_operations o JOIN scoped s ON s.id=o.shift_id
                WHERE o.currency='MXN' AND `+inPeriod("o.created_at")+`)::text,
            (SELECT coalesce(sum(c.net_amount),0) FROM app.cash_movements c JOIN scoped s ON s.id=c.shift_id
                WHERE c.is_cash AND `+inPeriod("c.created_at")+`)::text`, args).Scan(&openings, &income, &withdrawals, &expenses, &cashNet)
	if err != nil {
		return nil, err
	}
	values := map[string]any{
		"openings": openings, "income": decimalText(income), "withdrawals": decimalText(withdrawals),
		"expenses": decimalText(expenses), "cash_net": decimalText(cashNet),
	}
	// Turnos abiertos y en arqueo: situación actual, solo en el periodo consultado.
	if period == query.Params.Period {
		var open, closing int64
		if err := query.Tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE s.state='open'),count(*) FILTER (WHERE s.state='closing')
            FROM app.cash_shifts s WHERE s.state <> 'closed' AND `+cashScopeSQL, args).Scan(&open, &closing); err != nil {
			return nil, err
		}
		values["open_shifts"] = open
		values["closing_shifts"] = closing
	}
	return values, nil
}

func cashDifferencesSection(ctx context.Context, query *Query, period *businessday.Period) (map[string]any, error) {
	var closings int64
	var over, short, difference string
	err := query.Tx.QueryRow(ctx, `SELECT count(*),coalesce(sum(greatest(cl.difference_mxn,0)),0)::text,
            coalesce(sum(greatest(-cl.difference_mxn,0)),0)::text,coalesce(sum(cl.difference_mxn),0)::text
        FROM app.cash_shift_closures cl JOIN app.cash_shifts s ON s.id=cl.shift_id
        WHERE `+cashScopeSQL+` AND `+inPeriod("cl.closed_at"), periodCashArgs(query, period)).Scan(&closings, &over, &short, &difference)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"closings": closings, "over_mxn": decimalText(over), "short_mxn": decimalText(short), "difference_mxn": decimalText(difference),
	}, nil
}

func purchasesSection(ctx context.Context, query *Query, period *businessday.Period) (map[string]any, error) {
	args := query.PeriodArgs(period)
	args["first_day"] = period.FirstDay
	args["last_day"] = period.LastDay
	var receipts int64
	var received, ordered, returned string
	err := query.Tx.QueryRow(ctx, `SELECT
            (SELECT count(*) FROM app.inventory_receipts r WHERE `+inPeriod("r.occurred_at")+`),
            (SELECT coalesce(sum(r.total),0) FROM app.inventory_receipts r WHERE `+inPeriod("r.occurred_at")+`)::text,
            (SELECT coalesce(sum(o.total),0) FROM app.inventory_purchase_orders o WHERE o.state NOT IN ('draft','cancelled')
                AND o.order_date BETWEEN @first_day::date AND @last_day::date)::text,
            (SELECT round(coalesce(sum(sr.value),0),2) FROM app.inventory_supplier_returns sr WHERE `+inPeriod("sr.occurred_at")+`)::text`,
		args).Scan(&receipts, &received, &ordered, &returned)
	if err != nil {
		return nil, err
	}
	values := map[string]any{
		"receipts": receipts, "received": decimalText(received), "ordered": decimalText(ordered), "returned": decimalText(returned),
	}
	if period == query.Params.Period {
		var pending int64
		if err := query.Tx.QueryRow(ctx, `SELECT count(*) FROM app.inventory_purchase_orders WHERE state IN ('issued','partially_received')`).
			Scan(&pending); err != nil {
			return nil, err
		}
		values["pending_orders"] = pending
	}
	return values, nil
}

func inventorySection(ctx context.Context, query *Query, period *businessday.Period) (map[string]any, error) {
	var units, waste, cancelledWaste int64
	err := query.Tx.QueryRow(ctx, `SELECT
            (SELECT count(DISTINCT m.restaurant_unit_id) FROM app.inventory_movements m WHERE m.kind IN ('consumption','consumption_reversal')
                AND m.restaurant_unit_id IS NOT NULL AND `+inPeriod("m.occurred_at")+`),
            (SELECT count(DISTINCT m.source_id) FROM app.inventory_movements m WHERE m.kind='waste' AND `+inPeriod("m.occurred_at")+`),
            (SELECT count(*) FROM app.inventory_cancellation_classifications c WHERE c.state='prepared_waste' AND `+inPeriod("c.cancelled_at")+`)`,
		query.PeriodArgs(period)).Scan(&units, &waste, &cancelledWaste)
	if err != nil {
		return nil, err
	}
	values := map[string]any{"consumed_units": units, "waste_records": waste + cancelledWaste}
	if period == query.Params.Period {
		var pending int64
		if err := query.Tx.QueryRow(ctx, `SELECT count(*) FROM app.inventory_consumption_requests WHERE state <> 'processed'`).
			Scan(&pending); err != nil {
			return nil, err
		}
		values["pending_requests"] = pending
	}
	return values, nil
}

func inventoryCostsSection(ctx context.Context, query *Query, period *businessday.Period) (map[string]any, error) {
	var consumption, waste, cancelled, adjustments string
	err := query.Tx.QueryRow(ctx, `SELECT
            (SELECT coalesce(-sum(m.value),0) FROM app.inventory_movements m WHERE m.kind IN ('consumption','consumption_reversal')
                AND m.restaurant_unit_id IS NOT NULL AND `+inPeriod("m.occurred_at")+`)::text,
            (SELECT coalesce(-sum(m.value),0) FROM app.inventory_movements m WHERE m.kind='waste' AND `+inPeriod("m.occurred_at")+`)::text,
            (SELECT coalesce(-sum(m.value),0) FROM app.inventory_cancellation_classifications c
                JOIN app.inventory_movements m ON m.restaurant_unit_id=c.restaurant_unit_id AND m.kind IN ('consumption','consumption_reversal')
                WHERE c.state='prepared_waste' AND `+inPeriod("c.cancelled_at")+`)::text,
            (SELECT coalesce(sum(m.value),0) FROM app.inventory_movements m WHERE m.kind='cost_adjustment' AND `+inPeriod("m.occurred_at")+`)::text`,
		query.PeriodArgs(period)).Scan(&consumption, &waste, &cancelled, &adjustments)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"consumption_value": decimalText(consumption), "waste_value": decimalText(waste).Add(decimalText(cancelled)),
		"cost_adjustments": decimalText(adjustments),
	}, nil
}

func marginSection(ctx context.Context, query *Query, period *businessday.Period) (map[string]any, error) {
	return marginFigure(ctx, query, query.PeriodArgs(period))
}

func init() {
	registerDashboard(DashboardSection{
		ID: "cash", Title: "Caja", Permissions: []string{PermissionCash},
		Indicators: []IndicatorDefinition{
			{Key: "open_shifts", Label: "Turnos abiertos", Type: TypeInteger, ReportID: "cash.shifts", Hint: "Situación actual."},
			{Key: "closing_shifts", Label: "Turnos en arqueo", Type: TypeInteger, ReportID: "cash.shifts",
				Hint: "Situación actual; sus importes no se incluyen hasta que termine el arqueo."},
			{Key: "openings", Label: "Aperturas de turno", Type: TypeInteger, ReportID: "cash.by_day"},
			{Key: "cash_net", Label: "Neto cobrado en efectivo", Type: TypeMoneyMXN, ReportID: "cash.by_day",
				Hint: "Cobros menos devoluciones con métodos de efectivo, sin turnos en arqueo."},
			{Key: "income", Label: "Ingresos de efectivo", Type: TypeMoneyMXN, ReportID: "cash.operations"},
			{Key: "withdrawals", Label: "Retiros", Type: TypeMoneyMXN, ReportID: "cash.operations"},
			{Key: "expenses", Label: "Gastos de caja", Type: TypeMoneyMXN, ReportID: "cash.operations",
				Hint: "Los vinculados con compras no son otra compra."},
		},
		Compute:    cashSection,
		CashScoped: true,
	})
	registerDashboard(DashboardSection{
		ID: "cash_differences", Title: "Diferencias de caja", Permissions: []string{PermissionCash, PermissionCashExpected},
		Indicators: []IndicatorDefinition{
			{Key: "closings", Label: "Turnos cerrados", Type: TypeInteger, ReportID: "cash.shifts"},
			{Key: "over_mxn", Label: "Sobrantes en pesos", Type: TypeMoneyMXN, ReportID: "cash.shifts"},
			{Key: "short_mxn", Label: "Faltantes en pesos", Type: TypeMoneyMXN, ReportID: "cash.shifts"},
			{Key: "difference_mxn", Label: "Diferencia neta en pesos", Type: TypeMoneyMXN, ReportID: "cash.by_register"},
		},
		Compute:    cashDifferencesSection,
		CashScoped: true,
	})
	registerDashboard(DashboardSection{
		ID: "purchases", Title: "Compras", Permissions: []string{PermissionPurchases},
		Indicators: []IndicatorDefinition{
			{Key: "received", Label: "Compras recibidas", Type: TypeMoneyMXN, ReportID: "purchases.received",
				Hint: "Total de las recepciones; lo pagado desde caja no se suma otra vez."},
			{Key: "receipts", Label: "Recepciones", Type: TypeInteger, ReportID: "purchases.received"},
			{Key: "ordered", Label: "Compras ordenadas", Type: TypeMoneyMXN, ReportID: "purchases.ordered",
				Hint: "Órdenes emitidas; no es mercancía recibida."},
			{Key: "pending_orders", Label: "Órdenes por recibir", Type: TypeInteger, ReportID: "purchases.pending", Hint: "Situación actual."},
			{Key: "returned", Label: "Devoluciones a proveedor", Type: TypeMoneyMXN, ReportID: "purchases.supplier_returns"},
		},
		Compute: purchasesSection,
	})
	registerDashboard(DashboardSection{
		ID: "inventory", Title: "Inventario", Permissions: []string{PermissionInventory, permissionInventoryMovements},
		Indicators: []IndicatorDefinition{
			{Key: "consumed_units", Label: "Unidades con consumo teórico", Type: TypeInteger, ReportID: "inventory.consumption_by_day",
				Hint: "Cada unidad liberada a cocina una vez."},
			{Key: "waste_records", Label: "Mermas registradas", Type: TypeInteger, ReportID: "inventory.waste",
				Hint: "Mermas operativas y cancelaciones «Preparado / merma»."},
			{Key: "pending_requests", Label: "Consumos por procesar", Type: TypeInteger, ReportID: "inventory.consumption_by_day",
				Hint: "Situación actual; con pendientes los datos pueden estar incompletos."},
		},
		Compute: inventorySection,
	})
	registerDashboard(DashboardSection{
		ID: "inventory_costs", Title: "Costos de inventario",
		Permissions: []string{PermissionInventory, permissionInventoryMovements, PermissionCostsRead},
		Indicators: []IndicatorDefinition{
			{Key: "consumption_value", Label: "Consumo teórico valuado", Type: TypeMoneyMXN, ReportID: "inventory.consumption_by_item"},
			{Key: "waste_value", Label: "Costo de mermas", Type: TypeMoneyMXN, ReportID: "inventory.waste"},
			{Key: "cost_adjustments", Label: "Ajustes de costo", Type: TypeMoneyMXN, ReportID: "costs.cost_adjustments",
				Hint: "Concepto del periodo; no se reparte entre productos."},
		},
		Compute: inventoryCostsSection,
	})
	registerDashboard(DashboardSection{
		ID: "margin", Title: "Margen estimado", Permissions: []string{PermissionSales, PermissionCostsRead},
		Indicators: []IndicatorDefinition{
			{Key: "cost_of_sales", Label: "Costo de lo vendido", Type: TypeMoneyMXN, ReportID: "costs.margin_by_product",
				Hint: "Consumos reales de las unidades vendidas con costo."},
			{Key: "margin", Label: "Margen estimado", Type: TypeMoneyMXN, ReportID: "costs.margin_by_period",
				Hint: "Venta sin IVA con costo − costo; solo con datos suficientes. No es utilidad contable."},
			{Key: "margin_percent", Label: "Margen estimado %", Type: TypePercent, ReportID: "costs.margin_by_period"},
			{Key: "firm_share", Label: "% de la venta con costo firme", Type: TypePercent, ReportID: "costs.cost_status"},
			{Key: "uncosted_units", Label: "Unidades vendidas sin costo", Type: TypeInteger, ReportID: "costs.cost_status"},
		},
		Compute: marginSection,
	})
}
