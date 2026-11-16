package legacyimport

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/internal/plural"
)

// Validación posterior (§4.4): dentro de la misma transacción de la importación, lo escrito en la base contra lo
// calculado del paquete por una pasada independiente (expected.go). Las diferencias de dinero nunca se ocultan: si
// algo no cuadra, la validación lo dice con el importe y la importación termina «con diferencias».

// Validation es el resultado de la validación.
type Validation struct {
	OK                  bool     `json:"ok"`
	HasMoneyDifferences bool     `json:"has_money_differences"`
	Checks              []Check  `json:"checks"`
	Notes               []string `json:"notes"`
}

// Check es una comparación: lo esperado del paquete, lo escrito y la diferencia (escrito − esperado).
type Check struct {
	Group      string `json:"group"`
	Key        string `json:"key"`
	Label      string `json:"label"`
	Kind       string `json:"kind"`
	Expected   string `json:"expected"`
	Actual     string `json:"actual"`
	Difference string `json:"difference"`
	OK         bool   `json:"ok"`
}

// Grupos de comparaciones.
const (
	GroupCounts   = "counts"
	GroupSales    = "sales_by_year"
	GroupPayments = "payments_by_method_year"
	GroupTotals   = "totals"
	KindCount     = "count"
	KindMoney     = "money"
)

var countLabels = []struct {
	key, label string
}{
	{CountCategories, "Categorías"},
	{CountProducts, "Productos"},
	{CountComplements, "Complementos"},
	{CountTables, "Mesas"},
	{CountUsers, "Usuarios"},
	{CountSales, "Ventas (todas)"},
	{CountSalesPaid, "Ventas pagadas"},
	{CountSalesCancelled, "Ventas canceladas"},
	{CountSalesOpen, "Ventas abiertas"},
	{CountLines, "Renglones"},
	{CountLineComplements, "Complementos vendidos"},
	{CountQuantityChanges, "Cambios de cantidad"},
	{CountDiscounts, "Descuentos"},
	{CountCancellations, "Cancelaciones"},
	{CountPayments, "Pagos"},
	{CountShifts, "Cortes"},
	{CountWithdrawals, "Retiros"},
	{CountExpenses, "Salidas de efectivo (egresos)"},
	{CountPrices, "Historial de precios"},
}

var methodLabels = map[string]string{MethodCash: MethodCashName, MethodCard: MethodCardName, MethodOther: "Otras formas de pago"}

// actualCounts cuenta lo escrito del lote: el historial en sus tablas y lo vivo por su correspondencia (que siga
// existiendo en el catálogo, las mesas y los usuarios).
func actualCounts(ctx context.Context, tx pgx.Tx, batch pgtype.UUID) (map[string]int64, error) {
	counts := map[string]int64{}
	queries := map[string]string{
		CountSales:           "SELECT count(*) FROM app.legacy_sales WHERE batch_id = $1",
		CountSalesPaid:       "SELECT count(*) FROM app.legacy_sales WHERE batch_id = $1 AND status = 'paid'",
		CountSalesCancelled:  "SELECT count(*) FROM app.legacy_sales WHERE batch_id = $1 AND status = 'cancelled'",
		CountSalesOpen:       "SELECT count(*) FROM app.legacy_sales WHERE batch_id = $1 AND status = 'open'",
		CountLines:           "SELECT count(*) FROM app.legacy_sale_lines WHERE batch_id = $1",
		CountLineComplements: "SELECT count(*) FROM app.legacy_line_complements WHERE batch_id = $1",
		CountQuantityChanges: "SELECT count(*) FROM app.legacy_quantity_changes WHERE batch_id = $1",
		CountDiscounts:       "SELECT count(*) FROM app.legacy_discounts WHERE batch_id = $1",
		CountCancellations:   "SELECT count(*) FROM app.legacy_cancellations WHERE batch_id = $1",
		CountPayments:        "SELECT count(*) FROM app.legacy_payments WHERE batch_id = $1",
		CountShifts:          "SELECT count(*) FROM app.legacy_cash_shifts WHERE batch_id = $1",
		CountWithdrawals:     "SELECT count(*) FROM app.legacy_cash_withdrawals WHERE batch_id = $1",
		CountExpenses:        "SELECT count(*) FROM app.legacy_cash_expenses WHERE batch_id = $1",
		CountPrices:          "SELECT count(*) FROM app.legacy_price_history WHERE batch_id = $1",
		CountCategories: `SELECT count(*) FROM app.legacy_id_map m JOIN app.catalog_categories c ON c.id = m.new_id
            WHERE m.batch_id = $1 AND m.entity = 'category' AND m.state = 'active' AND m.legacy_key <> 'sin-categoria'`,
		CountProducts: `SELECT count(*) FROM app.legacy_id_map m JOIN app.catalog_products p ON p.id = m.new_id
            WHERE m.batch_id = $1 AND m.entity = 'product' AND m.state = 'active'`,
		CountComplements: `SELECT count(*) FROM app.legacy_id_map m JOIN app.catalog_complements c ON c.id = m.new_id
            WHERE m.batch_id = $1 AND m.entity = 'complement' AND m.state = 'active'`,
		CountTables: `SELECT count(*) FROM app.legacy_id_map m JOIN app.restaurant_tables t ON t.id = m.new_id
            WHERE m.batch_id = $1 AND m.entity = 'table' AND m.state = 'active'`,
		CountUsers: `SELECT count(*) FROM app.legacy_id_map m JOIN app.users u ON u.id = m.new_id
            WHERE m.batch_id = $1 AND m.entity = 'user' AND m.state = 'active'`,
	}
	for key, query := range queries {
		var count int64
		if err := tx.QueryRow(ctx, query, batch).Scan(&count); err != nil {
			return nil, fmt.Errorf("conteo %s: %w", key, err)
		}
		counts[key] = count
	}
	return counts, nil
}

// yearExpression es el año del día comercial de un instante en SQL, como businessday.DayOf.
const yearExpression = "extract(year FROM ((%s AT TIME ZONE $2) - $3::interval))::int"

// writtenMoney son los totales escritos del historial.
type writtenMoney struct {
	sales       map[int]Money
	payments    map[string]map[int]Money
	withdrawals Money
	expenses    Money
	unbalanced  int64
	// withoutLedger: ventas pagadas sin ningún renglón en el libro (sin renglones ni importes; revisión de la Fase C,
	// M-7). No son una diferencia: solo explican por qué el conteo de cuentas de los reportes da menos.
	withoutLedger int64
}

// actualMoney lee los totales escritos: venta por año (Σ net del libro), pagos por forma de pago y año, retiros,
// egresos y las ventas cuyo libro no suma lo que se debía.
func actualMoney(ctx context.Context, tx pgx.Tx, batch pgtype.UUID, config businessday.Config) (writtenMoney, error) {
	written, err := readMoney(ctx, tx, batch, config)
	if err != nil {
		return writtenMoney{}, err
	}
	var expensesText string
	if err := tx.QueryRow(ctx, `SELECT coalesce(sum(amount), 0)::text FROM app.legacy_cash_expenses WHERE batch_id = $1`,
		batch).Scan(&expensesText); err != nil {
		return writtenMoney{}, err
	}
	written.expenses, err = ParseDisplayMoney(expensesText)
	return written, err
}

// ledgerCheckSQL cuenta, de las ventas pagadas del lote, las que su libro no suma lo que se debía y las que no tienen
// ningún renglón en el libro. Suma el libro de cada venta con una subconsulta LATERAL que lo busca por el índice
// (batch_id, legacy_sale_id), así que el plan no depende de la estimación del lote (revisión de la Fase E, D-1): con las
// estadísticas de otro lote el planificador estimaba el lote nuevo en una fila y, con una subconsulta agregada del lote
// dentro de la unión (o con la unión directa), elegía un nested loop que volvía a recorrer el libro del lote por cada
// venta (horas con el volumen real).
const ledgerCheckSQL = `SELECT count(*) FILTER (WHERE coalesce(l.net, 0) <> s.amount_due), count(*) FILTER (WHERE l.entries = 0)
        FROM app.legacy_sales s
        CROSS JOIN LATERAL (SELECT sum(r.net) AS net, count(*) AS entries FROM app.legacy_sales_recognitions r
            WHERE r.batch_id = s.batch_id AND r.legacy_sale_id = s.legacy_id) l
        WHERE s.batch_id = $1 AND s.status = 'paid'`

func readMoney(ctx context.Context, tx pgx.Tx, batch pgtype.UUID, config businessday.Config) (writtenMoney, error) {
	sales, payments, withdrawals, err := readSalesAndPayments(ctx, tx, batch, config)
	if err != nil {
		return writtenMoney{}, err
	}
	written := writtenMoney{sales: sales, payments: payments, withdrawals: withdrawals}
	// Cada venta pagada: Σ net de su libro = lo que se debía. Cuenta las que no y las que no tienen ningún renglón.
	err = tx.QueryRow(ctx, ledgerCheckSQL, batch).Scan(&written.unbalanced, &written.withoutLedger)
	return written, err
}

func readSalesAndPayments(ctx context.Context, tx pgx.Tx, batch pgtype.UUID, config businessday.Config) (map[int]Money,
	map[string]map[int]Money, Money, error) {
	sales := map[int]Money{}
	payments := map[string]map[int]Money{}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT %s, sum(net)::text FROM app.legacy_sales_recognitions
        WHERE batch_id = $1 GROUP BY 1`, fmt.Sprintf(yearExpression, "recognized_at")), batch, config.Timezone, config.Interval())
	if err != nil {
		return nil, nil, 0, err
	}
	for rows.Next() {
		var year int
		var total string
		if err := rows.Scan(&year, &total); err != nil {
			rows.Close()
			return nil, nil, 0, err
		}
		amount, err := ParseDisplayMoney(total)
		if err != nil {
			rows.Close()
			return nil, nil, 0, err
		}
		sales[year] = amount
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, 0, err
	}
	rows, err = tx.Query(ctx, fmt.Sprintf(`SELECT method, %s, sum(amount)::text FROM app.legacy_payments
        WHERE batch_id = $1 GROUP BY 1, 2`, fmt.Sprintf(yearExpression, "paid_at")), batch, config.Timezone, config.Interval())
	if err != nil {
		return nil, nil, 0, err
	}
	for rows.Next() {
		var method, total string
		var year int
		if err := rows.Scan(&method, &year, &total); err != nil {
			rows.Close()
			return nil, nil, 0, err
		}
		amount, err := ParseDisplayMoney(total)
		if err != nil {
			rows.Close()
			return nil, nil, 0, err
		}
		if payments[method] == nil {
			payments[method] = map[int]Money{}
		}
		payments[method][year] = amount
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, 0, err
	}
	var withdrawalsText string
	if err := tx.QueryRow(ctx, `SELECT coalesce(sum(amount), 0)::text FROM app.legacy_cash_withdrawals WHERE batch_id = $1`,
		batch).Scan(&withdrawalsText); err != nil {
		return nil, nil, 0, err
	}
	withdrawals, err := ParseDisplayMoney(withdrawalsText)
	if err != nil {
		return nil, nil, 0, err
	}
	return sales, payments, withdrawals, nil
}

// validate compara lo escrito con lo esperado.
func validate(ctx context.Context, tx pgx.Tx, batch pgtype.UUID, expected Expected, config businessday.Config) (Validation, error) {
	counts, err := actualCounts(ctx, tx, batch)
	if err != nil {
		return Validation{}, err
	}
	written, err := actualMoney(ctx, tx, batch, config)
	if err != nil {
		return Validation{}, err
	}
	return compare(expected, counts, written), nil
}

// compare arma las comparaciones (función pura: la prueba unitaria la usa con diferencias provocadas).
func compare(expected Expected, counts map[string]int64, written writtenMoney) Validation {
	sales, payments := written.sales, written.payments
	validation := Validation{OK: true, Notes: []string{}}
	addCount := func(group, key, label string, want, got int64) {
		check := Check{Group: group, Key: key, Label: label, Kind: KindCount, Expected: strconv.FormatInt(want, 10),
			Actual: strconv.FormatInt(got, 10), Difference: strconv.FormatInt(got-want, 10), OK: want == got}
		if !check.OK {
			validation.OK = false
		}
		validation.Checks = append(validation.Checks, check)
	}
	addMoney := func(group, key, label string, want, got Money) {
		check := Check{Group: group, Key: key, Label: label, Kind: KindMoney, Expected: want.String(), Actual: got.String(),
			Difference: (got - want).String(), OK: want == got}
		if !check.OK {
			validation.OK = false
			validation.HasMoneyDifferences = true
		}
		validation.Checks = append(validation.Checks, check)
	}
	for _, item := range countLabels {
		addCount(GroupCounts, item.key, item.label, expected.Counts[item.key], counts[item.key])
	}
	years := map[int]bool{}
	for year := range expected.SalesByYear {
		years[year] = true
	}
	for year := range sales {
		years[year] = true
	}
	for _, year := range sortedYears(years) {
		addMoney(GroupSales, fmt.Sprintf("sales_%d", year), fmt.Sprintf("Total vendido %d", year), expected.SalesByYear[year],
			sales[year])
	}
	methods := []string{MethodCash, MethodCard, MethodOther}
	for _, method := range methods {
		years := map[int]bool{}
		for year := range expected.PaymentsByMethodYear[method] {
			years[year] = true
		}
		for year := range payments[method] {
			years[year] = true
		}
		for _, year := range sortedYears(years) {
			addMoney(GroupPayments, fmt.Sprintf("payments_%s_%d", method, year),
				fmt.Sprintf("%s %d", methodLabels[method], year), expected.PaymentsByMethodYear[method][year],
				payments[method][year])
		}
	}
	addMoney(GroupTotals, "withdrawals", "Total de retiros", expected.WithdrawalsTotal, written.withdrawals)
	// Egresos: las salidas de efectivo (en el respaldo real, ninguna). Las compras recibidas («Egresos» en la interfaz
	// anterior) no se importan: si el paquete trae alguna, la diferencia queda a la vista.
	addMoney(GroupTotals, "expenses", "Total de egresos (salidas de efectivo)", expected.ExpensesTotal, written.expenses)
	if expected.Purchases > 0 {
		addMoney(GroupTotals, "purchases", "Total de compras recibidas (no se importan)", expected.PurchasesTotal, 0)
	}
	addCount(GroupTotals, "unbalanced_sales", "Ventas cuyo libro no suma lo que se debía", 0, written.unbalanced)
	if expected.Counts[CountExpenses] == 0 && expected.Purchases == 0 {
		validation.Notes = append(validation.Notes, "El sistema anterior no tenía egresos registrados: su total es 0.")
	}
	validation.Notes = append(validation.Notes, fmt.Sprintf(
		"Las ventas canceladas no son venta; el sistema anterior las contaba como vendidas: %s en total.",
		expected.CancelledLegacyStyle.Display()))
	validation.Notes = append(validation.Notes,
		"El sistema anterior no guardaba dólares ni tipo de cambio: los pagos en dólares quedaron como efectivo en pesos.")
	if written.withoutLedger > 0 {
		count := int(written.withoutLedger)
		validation.Notes = append(validation.Notes, plural.Count(count, "venta pagada", "ventas pagadas")+" "+
			plural.Word(count, "no tiene", "no tienen")+" renglones ni importes (total 0): "+
			plural.Word(count, "no queda", "no quedan")+" en el libro de ventas, así que «Cuentas cerradas» y el conteo de "+
			"cuentas de los reportes dan "+strconv.Itoa(count)+" menos que «Ventas pagadas».")
	}
	return validation
}

func sortedYears(years map[int]bool) []int {
	list := make([]int, 0, len(years))
	for year := range years {
		list = append(list, year)
	}
	sort.Ints(list)
	return list
}
