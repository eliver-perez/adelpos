package reports

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/internal/payments"
	"local/ad-el-pos/server/internal/plural"
)

// «En curso» (§0-B): las cuentas abiertas con su total, lo pagado y su saldo, con la fórmula única
// de payments.AccountStandingSQL. No se suman a las ventas. Se distinguen:
//   - «Pagada sin entregar»: para llevar liquidado que aún no se entrega (se reconocerá al entregarse);
//   - «Reabierta»: cuenta que ya se cerró alguna vez (su venta ya se reconoció; un cambio posterior
//     se reconocerá como diferencia al volver a cerrarse), incluida la que quedó abierta con saldo 0
//     tras una devolución y la cancelación de sus unidades;
//   - «Con saldo pendiente» y «Sin cobro».

var progressStates = []Option{
	{Value: "all", Label: "Todas"},
	{Value: "with_balance", Label: "Con saldo pendiente"},
	{Value: "paid_undelivered", Label: "Pagadas sin entregar"},
	{Value: "reopened", Label: "Reabiertas"},
}

var progressColumns = []Column{
	codeColumn("folio", "Folio"),
	textColumn("account", "Cuenta"),
	textColumn("mode", "Modalidad"),
	textColumn("place", "Mesa u origen"),
	dateTimeColumn("opened_at", "Apertura"),
	textColumn("responsible", "Responsable"),
	countColumn("units", "Unidades"),
	moneyColumn("total", "Total"),
	moneyColumn("paid", "Pagado"),
	moneyColumn("balance", "Saldo"),
	textColumn("status", "Situación"),
}

func inProgress(defaultState string) func(ctx context.Context, query *Query) (*Dataset, error) {
	return func(ctx context.Context, query *Query) (*Dataset, error) {
		state := query.Params.Value("state")
		if state == "" {
			state = defaultState
		}
		list := conditions{}
		list.when(query, "mode", "x.account_mode = @mode")
		switch state {
		case "with_balance":
			list.add("x.balance > 0")
		case "paid_undelivered":
			list.add("x.units > 0 AND x.balance <= 0 AND x.account_mode = 'takeaway' AND NOT x.delivered")
		case "reopened":
			list.add("x.reopened")
		}
		rows, err := query.collect(ctx, `SELECT x.folio,x.account,x.mode,x.place,x.opened_at,x.responsible,x.units,x.total,x.paid,x.balance,
                CASE
                    WHEN x.units > 0 AND x.balance <= 0 AND x.account_mode='takeaway' AND NOT x.delivered THEN 'Pagada sin entregar'
                    WHEN x.reopened THEN 'Reabierta'
                    WHEN x.balance > 0 AND x.paid > 0 THEN 'Con saldo pendiente (abonada)'
                    WHEN x.balance > 0 THEN 'Con saldo pendiente'
                    ELSE 'Sin productos confirmados' END AS status
            FROM (
                SELECT a.folio::text AS folio,a.name AS account,a.mode AS account_mode,
                    CASE a.mode WHEN 'dine_in' THEN 'Salón' ELSE 'Para llevar' END AS mode,
                    coalesce(t.name,CASE WHEN a.mode='takeaway' THEN a.origin END,'Sin mesa') AS place,
                    a.created_at AS opened_at,u.display_name AS responsible,f.units::bigint AS units,f.total,f.paid,f.balance,
                    EXISTS(SELECT FROM app.takeaway_deliveries d WHERE d.account_id=a.id) AS delivered,
                    (EXISTS(SELECT FROM app.audit_events e WHERE e.action='payments.account_closed' AND e.entity_id=a.id)
                        OR EXISTS(SELECT FROM app.sales_recognitions r WHERE r.account_id=a.id)
                        OR EXISTS(SELECT FROM app.restaurant_item_units unit JOIN app.sales_recognitions r ON r.unit_id=unit.id
                            WHERE unit.account_id=a.id AND unit.state='active')) AS reopened
                FROM app.restaurant_accounts a
                CROSS JOIN LATERAL (`+payments.AccountStandingSQL("a.id")+`) f
                LEFT JOIN app.restaurant_tables t ON t.id=a.table_id
                LEFT JOIN app.users u ON u.id=a.assigned_user_id
                WHERE a.state='open'
            ) x
            WHERE true`+list.sql()+`
            ORDER BY x.opened_at`, query.Args())
		if err != nil {
			return nil, err
		}
		figures := []Figure{
			{Key: "accounts", Label: "Cuentas en curso", Type: TypeInteger, Value: int64(len(rows))},
		}
		total, balance := decimal.Zero, decimal.Zero
		for _, row := range rows {
			total = total.Add(decimalOf(row, "total"))
			balance = balance.Add(decimalOf(row, "balance"))
		}
		figures = append(figures,
			Figure{Key: "total", Label: "Total en curso", Type: TypeMoneyMXN, Value: total},
			Figure{Key: "balance", Label: "Saldo pendiente", Type: TypeMoneyMXN, Value: balance})
		return &Dataset{Rows: rows, Summary: figures, Notes: []string{
			"Situación al momento de la consulta. Las cuentas en curso no se suman a las ventas: cuentan al cerrarse.",
		}}, nil
	}
}

func init() {
	register(Definition{
		ID: "sales.in_progress", Group: "sales", Title: "En curso",
		Description: "Cuentas abiertas con su total, lo pagado y su saldo; incluye las pagadas sin entregar y las reabiertas.",
		Permissions: []string{PermissionSales}, Period: PeriodNone,
		DateBasis: "Situación actual de las cuentas abiertas (sin periodo).",
		Filters:   []Filter{filterChoice("state", "Situación", "all", progressStates...), filterMode()},
		Columns:   progressColumns, DefaultSort: Sort{Key: "opened_at"}, Query: inProgress("all"),
	})
	register(Definition{
		ID: "payments.pending", Group: "payments", Title: "Importes pendientes",
		Description: "Cuentas abiertas con saldo pendiente de cobro.",
		Permissions: []string{PermissionPayments}, Period: PeriodNone,
		DateBasis: "Situación actual de las cuentas abiertas (sin periodo).",
		Filters:   []Filter{filterChoice("state", "Situación", "with_balance", progressStates...), filterMode()},
		Columns:   progressColumns, DefaultSort: Sort{Key: "balance", Descending: true}, Query: inProgress("with_balance"),
	})
}

// Evolución de ventas (§8): venta neta por día, semana o mes del periodo frente al periodo anterior
// equivalente, alineada por posición (primer día con primer día, etc.). Si el anterior tiene más
// cubetas (enero frente a febrero, un año de 54 semanas parciales), las sobrantes van al final con
// solo su valor anterior: los totales comparan siempre los dos periodos completos.

var evolutionGrouping = []Option{
	{Value: "day", Label: "Por día"},
	{Value: "week", Label: "Por semana"},
	{Value: "month", Label: "Por mes"},
}

func registerSalesEvolution() {
	register(Definition{
		ID: "sales.evolution", Group: "sales", Title: "Evolución de ventas",
		Description: "Venta neta del periodo frente al periodo anterior equivalente, por día, semana o mes.",
		Permissions: []string{PermissionSales}, Comparison: true, DateBasis: salesDateBasis,
		Filters: append([]Filter{filterChoice("group_by", "Agrupar", "day", evolutionGrouping...)}, salesFilters()...),
		Columns: []Column{
			textColumn("bucket", "Periodo"),
			dateColumn("bucket_start", "Desde"),
			columnNet,
			optional(dateColumn("previous_start", "Desde (anterior)")),
			moneyColumn("previous_net", "Venta neta anterior"),
			described(Column{Key: "change", Label: "Variación", Type: TypeMoneyMXN, Total: TotalCustom, Sortable: true},
				"Venta neta − venta neta anterior."),
			percentColumn("change_percent", "Variación %"),
		},
		DefaultSort: Sort{Key: "bucket_start"},
		Totals: func(rows []Row, totals Row) {
			change := decimalOf(totals, "net").Sub(decimalOf(totals, "previous_net"))
			totals["change"] = change
			totals["change_percent"] = percentOf(change, decimalOf(totals, "previous_net").Abs())
		},
		Query: salesEvolution,
	})
}

func bucketStart(date time.Time, grouping string) time.Time {
	switch grouping {
	case "week":
		return date.AddDate(0, 0, -((int(date.Weekday()) + 6) % 7))
	case "month":
		return time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return date
}

func nextBucket(date time.Time, grouping string) time.Time {
	switch grouping {
	case "week":
		return date.AddDate(0, 0, 7)
	case "month":
		return date.AddDate(0, 1, 0)
	}
	return date.AddDate(0, 0, 1)
}

func bucketLabel(date time.Time, grouping string) string {
	switch grouping {
	case "week":
		year, week := date.ISOWeek()
		return fmt.Sprintf("Semana %d de %d", week, year)
	case "month":
		return businessday.MonthLabel(date.Year(), date.Month())
	}
	return businessday.DayLabel(date, true)
}

// bucketTotals suma la venta neta del periodo por cubeta.
func bucketTotals(ctx context.Context, query *Query, period *businessday.Period, grouping string) (map[string]decimal.Decimal, error) {
	expression := businessDate("r.recognized_at")
	switch grouping {
	case "week":
		expression = "date_trunc('week'," + expression + ")::date"
	case "month":
		expression = "date_trunc('month'," + expression + ")::date"
	}
	rows, err := query.Tx.Query(ctx, `SELECT `+expression+`,coalesce(sum(r.net),0)::text FROM app.sales_recognitions r
        WHERE true`+ledgerConditions(query).sql()+` GROUP BY 1`, query.PeriodArgs(period))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]decimal.Decimal{}
	for rows.Next() {
		var date time.Time
		var net string
		if err := rows.Scan(&date, &net); err != nil {
			return nil, err
		}
		result[date.Format("2006-01-02")] = decimal.RequireFromString(net)
	}
	return result, rows.Err()
}

func buckets(period *businessday.Period, grouping string) []time.Time {
	result := []time.Time{}
	for date := bucketStart(period.First(), grouping); !date.After(period.Last()); date = nextBucket(date, grouping) {
		result = append(result, date)
	}
	return result
}

func salesEvolution(ctx context.Context, query *Query) (*Dataset, error) {
	grouping := query.Params.Value("group_by")
	period := query.Params.Period
	previous := query.Params.Previous
	if previous == nil {
		computed := period.Previous(query.Now, query.Config)
		previous = &computed
		query.Params.Previous = previous
	}
	current, err := bucketTotals(ctx, query, period, grouping)
	if err != nil {
		return nil, err
	}
	before, err := bucketTotals(ctx, query, previous, grouping)
	if err != nil {
		return nil, err
	}
	rows, extra := evolutionRows(grouping, period, previous, current, before)
	notes := productNotes(query)
	if extra > 0 {
		singular, pluralForm := bucketNouns(grouping)
		notes = append(notes, "El periodo anterior tiene "+plural.Count(extra, singular, pluralForm)+" más: "+
			plural.Word(extra, "aparece", "aparecen")+" al final con solo su venta anterior, para que el total anterior y la "+
			"variación sean los del periodo anterior completo.")
	}
	return &Dataset{Rows: rows, Notes: notes}, nil
}

// evolutionRows alinea por posición las cubetas del periodo con las del periodo anterior. Las
// cubetas del anterior sin equivalente (un mes, una semana o un año anterior con más cubetas)
// aparecen al final como renglones con solo su valor anterior, de modo que el total «anterior» y su
// variación usan la suma completa del periodo anterior. Devuelve también cuántos renglones extra hay.
func evolutionRows(grouping string, period, previous *businessday.Period, current, before map[string]decimal.Decimal) ([]Row, int) {
	currentBuckets := buckets(period, grouping)
	previousBuckets := buckets(previous, grouping)
	rows := []Row{}
	for index, start := range currentBuckets {
		net := current[start.Format("2006-01-02")]
		row := Row{"bucket": bucketLabel(start, grouping), "bucket_start": start, "net": net, "previous_net": decimal.Zero}
		if index < len(previousBuckets) {
			previousStart := previousBuckets[index]
			row["previous_start"] = previousStart
			row["previous_net"] = before[previousStart.Format("2006-01-02")]
		}
		change := net.Sub(decimalOf(row, "previous_net"))
		row["change"] = change
		row["change_percent"] = percentOf(change, decimalOf(row, "previous_net").Abs())
		rows = append(rows, row)
	}
	extra := 0
	for index := len(currentBuckets); index < len(previousBuckets); index++ {
		previousStart := previousBuckets[index]
		rows = append(rows, Row{
			"bucket": bucketLabel(previousStart, grouping) + " (solo en el periodo anterior)", "bucket_start": nil, "net": nil,
			"previous_start": previousStart, "previous_net": before[previousStart.Format("2006-01-02")], "change": nil,
			"change_percent": nil,
		})
		extra++
	}
	return rows, extra
}

// bucketNouns son el singular y el plural de la cubeta de una agrupación.
func bucketNouns(grouping string) (string, string) {
	switch grouping {
	case "week":
		return "semana", "semanas"
	case "month":
		return "mes", "meses"
	}
	return "día", "días"
}
