package legacyimport

import "time"

// Expected son los conteos y totales de la validación posterior (§4.4) calculados directamente de las filas del
// paquete, con una pasada propia que no usa la correspondencia (plan.go) ni lo escrito en la base: así una falla en la
// correspondencia o en la escritura aparece como diferencia.
type Expected struct {
	Counts map[string]int64
	// SalesByYear: Σ(total + montoExtra − descuentos) de las ventas pagadas no canceladas, por el año del día comercial
	// de su último pago (o de su apertura si no tienen pagos).
	SalesByYear map[int]Money
	// PaymentsByMethodYear: Σ pagos.monto por forma de pago (cash, card, other) y año del día comercial del pago.
	PaymentsByMethodYear map[string]map[int]Money
	// CancelledLegacyStyle: lo que el anterior contaba como vendido de las canceladas (su «Registro de ventas» y sus
	// cortes las incluían), solo informativo.
	CancelledLegacyStyle Money
	WithdrawalsTotal     Money
	ExpensesTotal        Money
	PurchasesTotal       Money
	Purchases            int64
}

// Llaves de los conteos de la validación (las mismas en Expected y en lo leído de la base).
const (
	CountCategories      = "categories"
	CountProducts        = "products"
	CountComplements     = "complements"
	CountTables          = "tables"
	CountUsers           = "users"
	CountSales           = "sales"
	CountSalesPaid       = "sales_paid"
	CountSalesCancelled  = "sales_cancelled"
	CountSalesOpen       = "sales_open"
	CountLines           = "lines"
	CountLineComplements = "line_complements"
	CountQuantityChanges = "quantity_changes"
	CountDiscounts       = "discounts"
	CountCancellations   = "cancellations"
	CountPayments        = "payments"
	CountShifts          = "cash_shifts"
	CountWithdrawals     = "withdrawals"
	CountPrices          = "price_history"
	CountExpenses        = "expenses"
)

// computeExpected recorre las filas del paquete. yearOf da el año del día comercial de un instante.
func computeExpected(model *Model, yearOf func(time.Time) int) Expected {
	expected := Expected{
		Counts:               map[string]int64{},
		SalesByYear:          map[int]Money{},
		PaymentsByMethodYear: map[string]map[int]Money{},
		PurchasesTotal:       model.PurchasesTotal,
		Purchases:            model.Purchases,
	}
	// Los conteos salen del manifiesto del paquete (el lector ya comprobó que cada tabla trae exactamente esas filas);
	// solo si una tabla no está en el manifiesto se cuentan sus filas.
	manifest := func(table string, rows int) int64 {
		if count, found := model.Info.Tables[table]; found {
			return count
		}
		return int64(rows)
	}
	takeaway := int64(0)
	for _, table := range model.Tables {
		if table.ID == takeawayTableID {
			takeaway = 1
		}
	}
	expected.Counts[CountCategories] = manifest("categorias", len(model.Categories))
	expected.Counts[CountProducts] = manifest("productos", len(model.Products))
	expected.Counts[CountComplements] = manifest("complementos", len(model.Complements))
	expected.Counts[CountTables] = manifest("mesas", len(model.Tables)) - takeaway
	expected.Counts[CountUsers] = manifest("usuarios", len(model.Users))
	expected.Counts[CountSales] = manifest("ventas", len(model.Sales))
	expected.Counts[CountLines] = manifest("ventas_productos", len(model.Lines))
	expected.Counts[CountLineComplements] = manifest("ventas_complementos", len(model.LineComplements))
	expected.Counts[CountQuantityChanges] = manifest("ventas_modificaciones", len(model.QuantityChanges))
	expected.Counts[CountDiscounts] = manifest("descuentos", len(model.Discounts))
	expected.Counts[CountCancellations] = manifest("cancelaciones", len(model.Cancellations))
	expected.Counts[CountPayments] = manifest("pagos", len(model.Payments))
	expected.Counts[CountShifts] = manifest("cortes", len(model.Shifts))
	expected.Counts[CountWithdrawals] = manifest("cortes_parciales", len(model.Withdrawals))
	expected.Counts[CountExpenses] = manifest("salidas", len(model.Expenses))
	expected.Counts[CountPrices] = manifest("productos_precios", len(model.ProductPrices)) +
		manifest("complementos_precios", len(model.ComplementPrices))

	cancelled := map[int32]bool{}
	for _, cancellation := range model.Cancellations {
		cancelled[cancellation.Sale] = true
	}
	discounts := map[int32]Money{}
	for _, discount := range model.Discounts {
		discounts[discount.Sale] += discount.Amount
	}
	lastPayment := map[int32]time.Time{}
	for _, payment := range model.Payments {
		if payment.PaidAt.After(lastPayment[payment.Sale]) {
			lastPayment[payment.Sale] = payment.PaidAt
		}
	}
	for _, sale := range model.Sales {
		owed := sale.Total + sale.ChargeTotal.Or(0) - discounts[sale.ID]
		if cancelled[sale.ID] {
			expected.Counts[CountSalesCancelled]++
			expected.CancelledLegacyStyle += owed
			continue
		}
		if !sale.Paid.Is(1) {
			expected.Counts[CountSalesOpen]++
			continue
		}
		expected.Counts[CountSalesPaid]++
		instant, found := lastPayment[sale.ID]
		if !found {
			instant = sale.OpenedAt
		}
		expected.SalesByYear[yearOf(instant)] += owed
	}
	for _, payment := range model.Payments {
		method := MethodOther
		switch {
		case !payment.Type.Valid || payment.Type.Value == legacyCashType:
			method = MethodCash
		case payment.Type.Value == legacyCardType:
			method = MethodCard
		}
		if expected.PaymentsByMethodYear[method] == nil {
			expected.PaymentsByMethodYear[method] = map[int]Money{}
		}
		expected.PaymentsByMethodYear[method][yearOf(payment.PaidAt)] += payment.Amount.Or(0)
	}
	for _, withdrawal := range model.Withdrawals {
		expected.WithdrawalsTotal += withdrawal.Amount
	}
	for _, expense := range model.Expenses {
		expected.ExpensesTotal += expense.Amount
	}
	return expected
}
