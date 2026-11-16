package legacyimport

import (
	"sort"
	"time"
)

// Estados de una venta importada.
const (
	SalePaid      = "paid"
	SaleCancelled = "cancelled"
	SaleOpen      = "open"
)

// Formas de pago del historial (decisión del coordinador): «Efectivo» y «Tarjeta (sistema anterior)», sin crear un
// método nuevo en Cobros. Un tipo que el anterior no tuviera conserva su nombre como «other».
const (
	MethodCash        = "cash"
	MethodCard        = "card"
	MethodOther       = "other"
	MethodCashName    = "Efectivo"
	MethodCardName    = "Tarjeta (sistema anterior)"
	legacyCashType    = 1
	legacyCardType    = 2
	differenceConcept = "Diferencia del sistema anterior"
)

// lateAfter: un pago que llega más de 2 días después de abrir la venta (informativo, MIGRACION_ANALISIS.md §5).
const lateAfter = 48 * time.Hour

// doubtfulWithdrawal: un retiro de $100 000 o más es un posible error de captura (§5, fila 21).
const doubtfulWithdrawal = Money(100000 * moneyUnit)

// HistoryPlan son los datos derivados del historial: por venta, por renglón, por complemento, por pago, por
// descuento, por corte y por retiro, en el mismo orden que las filas del modelo.
type HistoryPlan struct {
	Sales       []SalePlan
	Lines       []LinePlan
	Complements []ComplementPlan
	Payments    []PaymentPlan
	Discounts   []DiscountPlan
	Shifts      []ShiftPlan
	Withdrawals []WithdrawalPlan
	Prices      []PricePlan

	saleIndex map[int32]int
	lineIndex map[int32]int
}

// SalePlan es una venta con su estado, sus fechas y sus sumas.
type SalePlan struct {
	Status         string
	RecognizedAt   time.Time
	LastPaymentAt  time.Time
	CancelledAt    time.Time
	Mode           string
	Tax            Money
	TaxEstimated   bool
	DiscountTotal  Money
	Due            Money
	LinesSubtotal  Money
	LinesTax       Money
	LinesAmount    Money
	LinesCount     int
	PaymentsAmount Money
	PaymentsCount  int
	Flags          flagSet
}

// LinePlan es un renglón con su IVA (guardado o estimado), su tasa y la suma de sus complementos.
type LinePlan struct {
	SaleIndex           int
	Tax                 Money
	TaxEstimated        bool
	TaxRate             int
	HasRate             bool
	ComplementsCount    int
	ComplementsSubtotal Money
	ComplementsTax      Money
	ComplementsTotal    Money
	ComplementEstimated bool
	Flags               flagSet
}

// ComplementPlan es un complemento vendido.
type ComplementPlan struct {
	LineIndex    int
	SaleIndex    int
	Tax          Money
	TaxEstimated bool
	Flags        flagSet
}

// PaymentPlan es un pago con su forma de pago del historial.
type PaymentPlan struct {
	SaleIndex  int
	Method     string
	MethodName string
	Flags      flagSet
}

// DiscountPlan es un descuento.
type DiscountPlan struct {
	SaleIndex int
	TypeName  string
	HasType   bool
	Flags     flagSet
}

// ShiftPlan es un corte con su estado, sus fechas (las del anterior o, si faltan, las estimadas de sus ventas y pagos)
// y sus marcas.
type ShiftPlan struct {
	Status   string
	OpenedAt time.Time
	ClosedAt time.Time
	Flags    flagSet
}

// WithdrawalPlan es un retiro.
type WithdrawalPlan struct {
	Flags flagSet
}

// PricePlan es un precio del historial con su tasa deducida.
type PricePlan struct {
	Kind    string
	Index   int
	TaxRate int
	Flags   flagSet
}

// estimateTax estima el IVA incluido en un importe con una tasa: total × tasa / (100 + tasa), redondeado al
// diezmilésimo (mitad alejándose de cero). Solo para un IVA que el anterior no guardó (marca tax_estimated).
func estimateTax(total Money, rate int) Money {
	if rate == 0 {
		return 0
	}
	numerator := int64(total) * int64(rate)
	denominator := int64(100 + rate)
	quotient := numerator / denominator
	remainder := numerator % denominator
	if remainder*2 >= denominator {
		quotient++
	} else if remainder*2 <= -denominator {
		quotient--
	}
	return Money(quotient)
}

// pricesByItem agrupa los precios por producto o complemento.
func pricesByItem(prices []Price) map[int32][]Price {
	grouped := map[int32][]Price{}
	for _, price := range prices {
		grouped[price.Item] = append(grouped[price.Item], price)
	}
	return grouped
}

// buildHistory deriva el historial: estados, fechas de reconocimiento, sumas, IVA estimado y marcas. No cambia
// ningún importe guardado.
func buildHistory(model *Model) *HistoryPlan {
	plan := &HistoryPlan{
		Sales:       make([]SalePlan, len(model.Sales)),
		Lines:       make([]LinePlan, len(model.Lines)),
		Complements: make([]ComplementPlan, len(model.LineComplements)),
		Payments:    make([]PaymentPlan, len(model.Payments)),
		Discounts:   make([]DiscountPlan, len(model.Discounts)),
		Shifts:      make([]ShiftPlan, len(model.Shifts)),
		Withdrawals: make([]WithdrawalPlan, len(model.Withdrawals)),
		saleIndex:   make(map[int32]int, len(model.Sales)),
		lineIndex:   make(map[int32]int, len(model.Lines)),
	}
	for index, sale := range model.Sales {
		plan.saleIndex[sale.ID] = index
	}
	for index, line := range model.Lines {
		plan.lineIndex[line.ID] = index
	}
	productPrices := pricesByItem(model.ProductPrices)
	complementPrices := pricesByItem(model.ComplementPrices)
	products := map[int32]bool{}
	for _, product := range model.Products {
		products[product.ID] = true
	}
	complements := map[int32]bool{}
	for _, complement := range model.Complements {
		complements[complement.ID] = true
	}
	plan.planLines(model, productPrices, products)
	plan.planComplements(model, complementPrices, complements)
	plan.planSales(model)
	plan.planShifts(model)
	plan.planWithdrawals(model)
	plan.planPrices(model)
	return plan
}

func (plan *HistoryPlan) sale(id int32) int {
	index, found := plan.saleIndex[id]
	if !found {
		return -1
	}
	return index
}

func (plan *HistoryPlan) line(id int32) int {
	index, found := plan.lineIndex[id]
	if !found {
		return -1
	}
	return index
}

func (plan *HistoryPlan) planLines(model *Model, prices map[int32][]Price, products map[int32]bool) {
	for index, line := range model.Lines {
		state := &plan.Lines[index]
		state.SaleIndex = plan.sale(line.Sale)
		if state.SaleIndex < 0 {
			state.Flags.add(FlagUnknownSale)
		}
		if !products[line.Product] {
			state.Flags.add(FlagUnknownProduct)
		}
		if line.Quantity == 0 {
			state.Flags.add(FlagZeroQuantity)
		}
		var opened time.Time
		if state.SaleIndex >= 0 {
			opened = model.Sales[state.SaleIndex].OpenedAt
		}
		if line.Tax.Valid {
			state.Tax = line.Tax.Value
			if line.Subtotal+line.Tax.Value != line.Total {
				state.Flags.add(FlagAmountsInconsistent)
			}
			if line.Subtotal != 0 {
				rate, exact := deduceRate(line.Subtotal, line.Tax.Value)
				state.TaxRate, state.HasRate = rate, true
				if !exact {
					state.Flags.add(FlagRateNotStandard)
				}
			}
		}
		if !state.HasRate || !line.Tax.Valid {
			// Sin IVA guardado (o sin subtotal para deducirlo): la tasa del producto vigente en la fecha de la venta.
			if price, _ := priceAt(prices[line.Product], opened); !price.StartsAt.IsZero() {
				state.TaxRate, _ = deduceRate(price.Subtotal, price.Tax.Or(0))
				state.HasRate = true
			}
		}
		if !line.Tax.Valid {
			state.Tax = estimateTax(line.Total, state.TaxRate)
			state.TaxEstimated = true
			state.Flags.add(FlagTaxEstimated)
		}
	}
}

func (plan *HistoryPlan) planComplements(model *Model, prices map[int32][]Price, complements map[int32]bool) {
	for index, complement := range model.LineComplements {
		state := &plan.Complements[index]
		state.LineIndex = plan.line(complement.Line)
		state.SaleIndex = -1
		if state.LineIndex < 0 {
			state.Flags.add(FlagUnknownLine)
		} else {
			state.SaleIndex = plan.Lines[state.LineIndex].SaleIndex
		}
		if !complements[complement.Complement] {
			state.Flags.add(FlagUnknownComplement)
		}
		if complement.Total < 0 {
			state.Flags.add(FlagNegativeAmount)
		}
		if complement.Tax.Valid {
			state.Tax = complement.Tax.Value
			if complement.Subtotal+complement.Tax.Value != complement.Total {
				state.Flags.add(FlagAmountsInconsistent)
			}
		} else {
			var opened time.Time
			if state.SaleIndex >= 0 {
				opened = model.Sales[state.SaleIndex].OpenedAt
			}
			rate := 0
			if price, _ := priceAt(prices[complement.Complement], opened); !price.StartsAt.IsZero() {
				rate, _ = deduceRate(price.Subtotal, price.Tax.Or(0))
			}
			state.Tax = estimateTax(complement.Total, rate)
			state.TaxEstimated = true
			state.Flags.add(FlagTaxEstimated)
		}
		if state.LineIndex >= 0 {
			line := &plan.Lines[state.LineIndex]
			line.ComplementsCount++
			line.ComplementsSubtotal += complement.Subtotal
			line.ComplementsTax += state.Tax
			line.ComplementsTotal += complement.Total
			if state.TaxEstimated {
				line.ComplementEstimated = true
			}
		}
	}
}

func (plan *HistoryPlan) planSales(model *Model) {
	cancellations := map[int32]time.Time{}
	for _, cancellation := range model.Cancellations {
		current, found := cancellations[cancellation.Sale]
		if !found || cancellation.CancelledAt.Before(current) {
			cancellations[cancellation.Sale] = cancellation.CancelledAt
		}
	}
	for index, line := range model.Lines {
		state := plan.Lines[index]
		if state.SaleIndex < 0 {
			continue
		}
		sale := &plan.Sales[state.SaleIndex]
		sale.LinesCount++
		sale.LinesSubtotal += line.Subtotal + state.ComplementsSubtotal
		sale.LinesTax += state.Tax + state.ComplementsTax
		sale.LinesAmount += line.Total + state.ComplementsTotal
		if state.TaxEstimated || state.ComplementEstimated {
			sale.TaxEstimated = true
		}
	}
	// Complementos de renglones que no existen no suman a ninguna venta (quedan marcados).
	for index, discount := range model.Discounts {
		state := &plan.Discounts[index]
		state.SaleIndex = plan.sale(discount.Sale)
		if state.SaleIndex < 0 {
			state.Flags.add(FlagUnknownSale)
			continue
		}
		plan.Sales[state.SaleIndex].DiscountTotal += discount.Amount
		if discount.Amount <= 0 {
			state.Flags.add(FlagZeroOrNegative)
		}
		if discount.Amount > model.Sales[state.SaleIndex].Total {
			state.Flags.add(FlagExceedsTotal)
		}
	}
	discountTypes := map[int32]string{}
	for _, row := range model.DiscountTypes {
		discountTypes[row.ID] = row.Name
	}
	for index, discount := range model.Discounts {
		if discount.Type.Valid {
			if name, found := discountTypes[discount.Type.Value]; found {
				plan.Discounts[index].TypeName, plan.Discounts[index].HasType = name, true
				continue
			}
		}
		plan.Discounts[index].Flags.add(FlagUnknownType)
	}
	paymentTypes := map[int32]string{}
	for _, row := range model.PaymentTypes {
		paymentTypes[row.ID] = row.Name
	}
	shifts := map[int32]bool{}
	for _, shift := range model.Shifts {
		shifts[shift.ID] = true
	}
	for index, payment := range model.Payments {
		state := &plan.Payments[index]
		state.SaleIndex = plan.sale(payment.Sale)
		state.Method, state.MethodName = paymentMethod(payment.Type, paymentTypes)
		if state.Method == MethodOther {
			state.Flags.add(FlagUnknownMethod)
		}
		amount := payment.Amount.Or(0)
		if amount == 0 {
			state.Flags.add(FlagZeroAmount)
		}
		if amount < 0 {
			state.Flags.add(FlagNegativeAmount)
		}
		if !payment.Shift.Valid {
			state.Flags.add(FlagNoShift)
		}
		if state.SaleIndex < 0 {
			state.Flags.add(FlagUnknownSale)
			continue
		}
		sale := &plan.Sales[state.SaleIndex]
		original := model.Sales[state.SaleIndex]
		sale.PaymentsAmount += amount
		sale.PaymentsCount++
		if payment.PaidAt.After(sale.LastPaymentAt) {
			sale.LastPaymentAt = payment.PaidAt
		}
		if payment.PaidAt.Before(original.OpenedAt) {
			state.Flags.add(FlagBeforeSale)
		}
		if payment.Shift.Valid && original.Shift.Valid && payment.Shift.Value != original.Shift.Value {
			state.Flags.add(FlagShiftDiffers)
		}
	}
	folios := map[string]int{}
	for _, sale := range model.Sales {
		folios[sale.Folio]++
	}
	tables := map[int32]bool{}
	for _, table := range model.Tables {
		tables[table.ID] = true
	}
	users := map[int32]bool{}
	for _, user := range model.Users {
		users[user.ID] = true
	}
	registers := map[int32]bool{}
	for _, register := range model.Registers {
		registers[register.ID] = true
	}
	_, hasRegisters := model.TableRows["cajas"]
	_, hasShifts := model.TableRows["cortes"]
	for index, original := range model.Sales {
		sale := &plan.Sales[index]
		if original.Table.Valid && original.Table.Value != takeawayTableID {
			sale.Mode = "dine_in"
		} else {
			sale.Mode = "takeaway"
		}
		cancelledAt, cancelled := cancellations[original.ID]
		switch {
		case cancelled:
			sale.Status = SaleCancelled
			sale.CancelledAt = cancelledAt
		case original.Paid.Is(1):
			sale.Status = SalePaid
		default:
			sale.Status = SaleOpen
		}
		sale.Tax = original.Tax.Or(0)
		if !original.Tax.Valid {
			// Sin IVA guardado en la venta: la suma del IVA de sus renglones y complementos (guardado o estimado).
			sale.Tax = sale.LinesTax
			sale.TaxEstimated = true
		}
		if sale.TaxEstimated {
			sale.Flags.add(FlagTaxEstimated)
		}
		sale.Due = original.Total + original.ChargeTotal.Or(0) - sale.DiscountTotal
		plan.saleFlags(original, sale, folios, tables, users, registers, shifts, hasRegisters, hasShifts)
	}
	plan.recognize(model)
	plan.markDuplicatePayments(model)
}

// recognize fija la fecha de reconocimiento de cada venta pagada (decisión del coordinador, precisada en la revisión de
// la Fase C): el pago con el que lo acumulado, contando solo pagos mayores que cero en orden de fecha y luego de id,
// alcanza lo que se debía; si nunca lo alcanza, el último pago mayor que cero; si no tiene pagos mayores que cero, su
// apertura. Así un pago repetido, en cero o negativo registrado después no mueve la venta de día, turno ni año (esos
// pagos se importan igual, cuentan en los cobros y llevan su marca).
func (plan *HistoryPlan) recognize(model *Model) {
	order := make([]int, 0, len(model.Payments))
	for index := range model.Payments {
		if plan.Payments[index].SaleIndex >= 0 && model.Payments[index].Amount.Or(0) > 0 {
			order = append(order, index)
		}
	}
	sort.Slice(order, func(left, right int) bool {
		first, second := model.Payments[order[left]], model.Payments[order[right]]
		firstSale, secondSale := plan.Payments[order[left]].SaleIndex, plan.Payments[order[right]].SaleIndex
		if firstSale != secondSale {
			return firstSale < secondSale
		}
		if !first.PaidAt.Equal(second.PaidAt) {
			return first.PaidAt.Before(second.PaidAt)
		}
		return first.ID < second.ID
	})
	recognized := make([]time.Time, len(model.Sales))
	reached := make([]bool, len(model.Sales))
	accumulated := make([]Money, len(model.Sales))
	for _, index := range order {
		saleIndex := plan.Payments[index].SaleIndex
		if reached[saleIndex] {
			continue
		}
		payment := model.Payments[index]
		accumulated[saleIndex] += payment.Amount.Value
		// Sin alcanzar lo que se debía, queda el último pago mayor que cero.
		recognized[saleIndex] = payment.PaidAt
		if accumulated[saleIndex] >= plan.Sales[saleIndex].Due {
			reached[saleIndex] = true
		}
	}
	for index, sale := range model.Sales {
		state := &plan.Sales[index]
		if state.Status != SalePaid {
			continue
		}
		state.RecognizedAt = recognized[index]
		if state.RecognizedAt.IsZero() {
			state.RecognizedAt = sale.OpenedAt
		}
	}
}

// paymentMethod traduce el tipo de pago del anterior: 1 «Efectivo» y 2 «Tarjeta Credito/Debito» (los dos de
// tipos_pagos); un pago sin tipo era efectivo (predeterminado 1 en el anterior).
func paymentMethod(legacyType NullInt, names map[int32]string) (string, string) {
	if !legacyType.Valid || legacyType.Value == legacyCashType {
		return MethodCash, MethodCashName
	}
	if legacyType.Value == legacyCardType {
		return MethodCard, MethodCardName
	}
	if name, found := names[legacyType.Value]; found && name != "" {
		return MethodOther, cleanName(name, 100, "Otra forma de pago (sistema anterior)")
	}
	return MethodOther, "Otra forma de pago (sistema anterior)"
}

func (plan *HistoryPlan) saleFlags(original Sale, sale *SalePlan, folios map[string]int, tables, users, registers,
	shifts map[int32]bool, hasRegisters, hasShifts bool) {
	if sale.LinesCount == 0 {
		sale.Flags.add(FlagNoLines)
	}
	if original.Total != sale.LinesAmount {
		sale.Flags.add(FlagTotalMismatch)
	}
	if original.Subtotal != sale.LinesSubtotal {
		sale.Flags.add(FlagSubtotalMismatch)
	}
	if sale.Tax != sale.LinesTax {
		sale.Flags.add(FlagTaxMismatch)
	}
	if original.Subtotal+sale.Tax != original.Total {
		sale.Flags.add(FlagAmountsInconsistent)
	}
	if sale.Tax < 0 {
		sale.Flags.add(FlagNegativeTax)
	}
	switch sale.Status {
	case SalePaid:
		if sale.PaymentsCount == 0 {
			sale.Flags.add(FlagNoPayments)
		} else if sale.PaymentsAmount > sale.Due {
			sale.Flags.add(FlagOverpaid)
		} else if sale.PaymentsAmount < sale.Due {
			sale.Flags.add(FlagUnderpaid)
		}
		if !original.Shift.Valid {
			sale.Flags.add(FlagNoShift)
		}
		if sale.PaymentsCount > 0 && sale.LastPaymentAt.Sub(original.OpenedAt) > lateAfter {
			sale.Flags.add(FlagLatePayment)
		}
	case SaleCancelled:
		if sale.PaymentsCount > 0 {
			sale.Flags.add(FlagCancelledWithPayments)
		}
	case SaleOpen:
		if sale.PaymentsCount > 0 {
			sale.Flags.add(FlagOpenWithPayments)
		}
	}
	if original.AmountPaid.Valid && original.AmountPaid.Value != sale.PaymentsAmount {
		sale.Flags.add(FlagPaidAmountMismatch)
	}
	if folios[original.Folio] > 1 {
		sale.Flags.add(FlagDuplicateFolio)
	}
	chargeAmount := original.ChargeTotal.Or(0)
	if original.ChargeSet && original.Charge != "" && chargeAmount == 0 {
		sale.Flags.add(FlagChargeWithoutAmount)
	}
	if chargeAmount != 0 && cleanName(original.Charge, 250, "") == "" {
		sale.Flags.add(FlagChargeWithoutConcept)
	}
	if original.Table.Valid && !tables[original.Table.Value] {
		sale.Flags.add(FlagUnknownTable)
	}
	if original.Cashier.Valid && !users[original.Cashier.Value] {
		sale.Flags.add(FlagUnknownCashier)
	}
	if hasRegisters && original.Register.Valid && !registers[original.Register.Value] {
		sale.Flags.add(FlagUnknownRegister)
	}
	if hasShifts && original.Shift.Valid && !shifts[original.Shift.Value] {
		sale.Flags.add(FlagUnknownShift)
	}
}

// markDuplicatePayments marca como posiblemente repetido un pago con el mismo importe que otro anterior de la misma
// venta, cuando la venta se cobró de más (§5, fila 8: casi todos los cobros de más son un pago repetido).
func (plan *HistoryPlan) markDuplicatePayments(model *Model) {
	overpaid := map[int][]int{}
	for index := range model.Payments {
		sale := plan.Payments[index].SaleIndex
		if sale >= 0 && plan.Sales[sale].Flags.has(FlagOverpaid) {
			overpaid[sale] = append(overpaid[sale], index)
		}
	}
	for _, payments := range overpaid {
		sort.SliceStable(payments, func(left, right int) bool {
			return model.Payments[payments[left]].PaidAt.Before(model.Payments[payments[right]].PaidAt)
		})
		seen := map[Money]bool{}
		for _, index := range payments {
			amount := model.Payments[index].Amount.Or(0)
			if seen[amount] && amount != 0 {
				plan.Payments[index].Flags.add(FlagPossibleDuplicate)
			}
			seen[amount] = true
		}
	}
}

func (plan *HistoryPlan) planShifts(model *Model) {
	expected := map[int32]Money{}
	activity := map[int32]bool{}
	// La primera y la última fecha de las ventas (su apertura) y los pagos de cada corte, para las fechas que falten.
	first := map[int32]time.Time{}
	last := map[int32]time.Time{}
	seen := func(shift int32, moment time.Time) {
		activity[shift] = true
		if moment.IsZero() {
			return
		}
		if current, found := first[shift]; !found || moment.Before(current) {
			first[shift] = moment
		}
		if current, found := last[shift]; !found || moment.After(current) {
			last[shift] = moment
		}
	}
	for index, sale := range model.Sales {
		if !sale.Shift.Valid {
			continue
		}
		// La «venta» del corte del anterior: Σ(total + montoExtra) − Σ descuentos de las ventas con ese corte,
		// incluidas tarjeta y canceladas (CerrarCorte.cs:325-370).
		expected[sale.Shift.Value] += sale.Total + sale.ChargeTotal.Or(0) - plan.Sales[index].DiscountTotal
		seen(sale.Shift.Value, sale.OpenedAt)
	}
	for _, payment := range model.Payments {
		if payment.Shift.Valid {
			seen(payment.Shift.Value, payment.PaidAt)
		}
	}
	withdrawals := map[int32]Money{}
	for _, withdrawal := range model.Withdrawals {
		if withdrawal.Shift.Valid {
			withdrawals[withdrawal.Shift.Value] += withdrawal.Amount
		}
	}
	for index, shift := range model.Shifts {
		state := &plan.Shifts[index]
		switch shift.Status {
		case "A", "a":
			state.Status = "open"
		case "C", "c":
			state.Status = "closed"
		default:
			state.Status = "unknown"
			state.Flags.add(FlagUnknownStatus)
		}
		if state.Status == "closed" && shift.Sales.Valid && shift.Sales.Value != expected[shift.ID] {
			state.Flags.add(FlagSalesMismatch)
		}
		if state.Status == "closed" && shift.Withdrawals.Valid && shift.Withdrawals.Value != withdrawals[shift.ID] {
			state.Flags.add(FlagWithdrawalsMismatch)
		}
		if !activity[shift.ID] {
			state.Flags.add(FlagNoActivity)
		}
		shiftDates(shift, state, first[shift.ID], last[shift.ID])
	}
}

// shiftDates fija las fechas de un corte (revisión de la Fase C, M-8): las del anterior; sin fecha_inicio (columna
// posterior al script y con nulos), la de su primera venta o pago, para que aparezca en «Turnos de caja» y en «Ventas
// por turno» (marca start_estimated); cerrado y sin fecha_fin, la de su última venta o pago (end_estimated). Un corte
// sin fecha de inicio ni ventas o pagos queda sin fecha (no_start). Una fecha estimada nunca queda del otro lado de la
// que sí estaba guardada.
func shiftDates(shift Shift, state *ShiftPlan, first, last time.Time) {
	state.OpenedAt, state.ClosedAt = shift.OpenedAt, shift.ClosedAt
	if shift.OpenedAt.IsZero() {
		if first.IsZero() {
			state.Flags.add(FlagNoStart)
		} else {
			state.OpenedAt = first
			if !shift.ClosedAt.IsZero() && shift.ClosedAt.Before(first) {
				state.OpenedAt = shift.ClosedAt
			}
			state.Flags.add(FlagStartEstimated)
		}
	}
	if shift.ClosedAt.IsZero() && state.Status == "closed" && !last.IsZero() {
		state.ClosedAt = last
		if !state.OpenedAt.IsZero() && last.Before(state.OpenedAt) {
			state.ClosedAt = state.OpenedAt
		}
		state.Flags.add(FlagEndEstimated)
	}
}

func (plan *HistoryPlan) planWithdrawals(model *Model) {
	shifts := map[int32]bool{}
	for _, shift := range model.Shifts {
		shifts[shift.ID] = true
	}
	for index, withdrawal := range model.Withdrawals {
		state := &plan.Withdrawals[index]
		if withdrawal.Amount <= 0 {
			state.Flags.add(FlagZeroOrNegative)
		}
		if withdrawal.Amount >= doubtfulWithdrawal {
			state.Flags.add(FlagDoubtfulAmount)
		}
		if !withdrawal.Shift.Valid || !shifts[withdrawal.Shift.Value] {
			state.Flags.add(FlagUnknownShift)
		}
	}
}

func (plan *HistoryPlan) planPrices(model *Model) {
	add := func(kind string, prices []Price) {
		for index, price := range prices {
			rate, exact := deduceRate(price.Subtotal, price.Tax.Or(0))
			state := PricePlan{Kind: kind, Index: index, TaxRate: rate}
			if !exact {
				state.Flags.add(FlagRateNotStandard)
			}
			plan.Prices = append(plan.Prices, state)
		}
	}
	add("product", model.ProductPrices)
	add("complement", model.ComplementPrices)
}
