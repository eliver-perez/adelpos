package reports

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/internal/payments"
)

// Secciones del resumen ejecutivo de la fase B1: ventas, cobros y descuentos. Cada valor sale de la
// misma consulta que su reporte detallado (mismas definiciones de métricas).

func decimalText(text string) decimal.Decimal {
	return decimal.RequireFromString(text)
}

func salesSection(ctx context.Context, query *Query, period *businessday.Period) (map[string]any, error) {
	var accounts, units int64
	var sales, net string
	err := query.Tx.QueryRow(ctx, `SELECT count(DISTINCT r.account_id) FILTER (WHERE r.recognition_kind <> 'resettlement'),
            coalesce(sum(r.quantity),0),coalesce(sum(r.gross-r.promotion+r.charge),0)::text,coalesce(sum(r.net),0)::text
        FROM app.sales_recognitions r WHERE r.recognized_at >= @from AND r.recognized_at < @until`, query.PeriodArgs(period)).
		Scan(&accounts, &units, &sales, &net)
	if err != nil {
		return nil, err
	}
	values := map[string]any{
		"sales": decimalText(sales), "net_sales": decimalText(net), "accounts": accounts, "units": units, "average_ticket": nil,
	}
	if accounts > 0 {
		values["average_ticket"] = decimalText(net).DivRound(decimal.NewFromInt(accounts), 2)
	}
	// «En curso» es la situación actual: solo tiene valor en el periodo consultado, no en el anterior.
	if period == query.Params.Period {
		var open int64
		var total, balance string
		err := query.Tx.QueryRow(ctx, `SELECT count(*),coalesce(sum(f.total),0)::text,coalesce(sum(greatest(f.balance,0)),0)::text
            FROM app.restaurant_accounts a CROSS JOIN LATERAL (`+payments.AccountStandingSQL("a.id")+`) f
            WHERE a.state='open' AND f.units > 0`).Scan(&open, &total, &balance)
		if err != nil {
			return nil, err
		}
		values["open_accounts"] = open
		values["open_total"] = decimalText(total)
		values["open_balance"] = decimalText(balance)
	}
	return values, nil
}

func salesSeries(ctx context.Context, query *Query) ([]Series, error) {
	rows, err := query.Tx.Query(ctx, `SELECT `+businessDate("r.recognized_at")+`,coalesce(sum(r.net),0)::text
        FROM app.sales_recognitions r WHERE r.recognized_at >= @from AND r.recognized_at < @until GROUP BY 1`, query.Args())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDay := map[string]decimal.Decimal{}
	for rows.Next() {
		var day time.Time
		var net string
		if err := rows.Scan(&day, &net); err != nil {
			return nil, err
		}
		byDay[day.Format("2006-01-02")] = decimalText(net)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	series := Series{Key: "net_sales_by_day", Label: "Venta neta por día", Type: TypeMoneyMXN, ReportID: "sales.by_day"}
	period := query.Params.Period
	for day := period.First(); !day.After(period.Last()); day = day.AddDate(0, 0, 1) {
		series.Points = append(series.Points, Point{Date: day, Value: byDay[day.Format("2006-01-02")]})
	}
	return []Series{series}, nil
}

func paymentsSection(ctx context.Context, query *Query, period *businessday.Period) (map[string]any, error) {
	var count int64
	var net, refunds, tips string
	err := query.Tx.QueryRow(ctx, paymentMovements+`
        SELECT count(*) FILTER (WHERE m.kind='payment'),coalesce(sum(m.amount),0)::text,
            coalesce(-sum(m.amount) FILTER (WHERE m.kind='refund'),0)::text,coalesce(sum(m.tip),0)::text
        FROM movements m JOIN app.cash_shifts s ON s.id=m.shift_id WHERE s.state <> 'closing'`, query.PeriodArgs(period)).
		Scan(&count, &net, &refunds, &tips)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"collected": decimalText(net), "payments": count, "refunds": decimalText(refunds), "tips": decimalText(tips),
	}, nil
}

func discountsSection(ctx context.Context, query *Query, period *businessday.Period) (map[string]any, error) {
	var discount, courtesy, base string
	err := query.Tx.QueryRow(ctx, `SELECT coalesce(sum(r.discount),0)::text,coalesce(sum(r.courtesy),0)::text,
            coalesce(sum(r.gross-r.promotion+r.charge),0)::text
        FROM app.sales_recognitions r WHERE r.recognized_at >= @from AND r.recognized_at < @until`, query.PeriodArgs(period)).
		Scan(&discount, &courtesy, &base)
	if err != nil {
		return nil, err
	}
	total := decimalText(discount).Add(decimalText(courtesy))
	return map[string]any{
		"discounts": decimalText(discount), "courtesies": decimalText(courtesy),
		"discount_percent": percentOf(total, decimalText(base)),
	}, nil
}

func init() {
	registerDashboard(DashboardSection{
		ID: "sales", Title: "Ventas", Permissions: []string{PermissionSales},
		Indicators: []IndicatorDefinition{
			{Key: "sales", Label: "Ventas del periodo", Type: TypeMoneyMXN, ReportID: "sales.summary",
				Hint: "Importe de productos y cargos de las cuentas cerradas, antes de descuentos y cortesías."},
			{Key: "net_sales", Label: "Ventas netas", Type: TypeMoneyMXN, ReportID: "sales.by_day",
				Hint: "Después de promociones, descuentos y cortesías; IVA incluido, sin propinas."},
			{Key: "accounts", Label: "Cuentas cerradas", Type: TypeInteger, ReportID: "sales.accounts"},
			{Key: "average_ticket", Label: "Ticket promedio", Type: TypeMoneyMXN, ReportID: "sales.by_day"},
			{Key: "units", Label: "Productos vendidos", Type: TypeInteger, ReportID: "products.top"},
			{Key: "open_accounts", Label: "Cuentas en curso", Type: TypeInteger, ReportID: "sales.in_progress",
				Hint: "Situación actual; no se suman a las ventas."},
			{Key: "open_balance", Label: "Saldo pendiente en curso", Type: TypeMoneyMXN, ReportID: "sales.in_progress"},
		},
		Compute: salesSection,
		Series:  salesSeries,
	})
	registerDashboard(DashboardSection{
		ID: "payments", Title: "Cobros", Permissions: []string{PermissionPayments},
		Indicators: []IndicatorDefinition{
			{Key: "collected", Label: "Cobrado neto", Type: TypeMoneyMXN, ReportID: "payments.by_method",
				Hint: "Cobros menos devoluciones y correcciones, sin propinas. No es lo mismo que la venta."},
			{Key: "payments", Label: "Cobros registrados", Type: TypeInteger, ReportID: "payments.list"},
			{Key: "refunds", Label: "Devoluciones", Type: TypeMoneyMXN, ReportID: "payments.refunds"},
			{Key: "tips", Label: "Propinas", Type: TypeMoneyMXN, ReportID: "payments.tips", Hint: "Separadas de las ventas."},
		},
		Compute: paymentsSection,
	})
	registerDashboard(DashboardSection{
		ID: "discounts", Title: "Descuentos y cortesías", Permissions: []string{PermissionDiscounts},
		Indicators: []IndicatorDefinition{
			{Key: "discounts", Label: "Descuentos", Type: TypeMoneyMXN, ReportID: "discounts.list"},
			{Key: "courtesies", Label: "Cortesías", Type: TypeMoneyMXN, ReportID: "discounts.courtesies"},
			{Key: "discount_percent", Label: "% de descuentos sobre ventas", Type: TypePercent, ReportID: "discounts.summary"},
		},
		Compute: discountsSection,
	})
}
