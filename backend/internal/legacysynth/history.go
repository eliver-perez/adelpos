package legacysynth

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// shift es un corte (turno de caja) sintético.
type shift struct {
	id         int
	cashier    int
	register   int
	start      time.Time
	end        *time.Time
	opening    decimal.Decimal
	difference decimal.Decimal
	// main: los cortes que reciben los pagos; los otros son el traslapado y el abierto sin ventas.
	main bool
}

func timePointer(moment time.Time) *time.Time {
	return &moment
}

func shifts() []shift {
	return []shift{
		{1, 1, 1, stamp(2019, 9, 30, 8, 0, 0, 0), timePointer(stamp(2022, 2, 15, 21, 30, 0, 0)), amount("500.0000"), decimal.Zero, true},
		{2, 3, 1, stamp(2022, 2, 16, 8, 0, 0, 0), timePointer(stamp(2022, 12, 31, 21, 0, 0, 0)), amount("1000.0000"), amount("12.5000"), true},
		{3, 3, 2, stamp(2023, 1, 2, 8, 0, 0, 0), timePointer(stamp(2023, 12, 30, 21, 0, 0, 0)), amount("1000.0000"), amount("-3.2500"), true},
		{4, 1, 1, stamp(2024, 1, 2, 8, 0, 0, 0), timePointer(stamp(2024, 12, 31, 21, 0, 0, 0)), amount("800.0000"), decimal.Zero, true},
		{5, 2, 1, stamp(2025, 1, 2, 8, 0, 0, 0), timePointer(stamp(2025, 12, 31, 21, 0, 0, 0)), amount("800.0000"), amount("0.5000"), true},
		{6, 3, 1, stamp(2026, 1, 2, 8, 0, 0, 0), nil, amount("600.0000"), decimal.Zero, true},
		// Traslapado con el corte 3 en la otra caja, de otro cajero y sin ventas.
		{7, 2, 2, stamp(2023, 3, 1, 9, 0, 0, 0), timePointer(stamp(2023, 3, 1, 18, 0, 0, 0)), amount("300.0000"), decimal.Zero, false},
		// Abierto desde 2024 sin ventas.
		{8, 2, 2, stamp(2024, 6, 1, 8, 0, 0, 0), nil, amount("200.0000"), decimal.Zero, false},
	}
}

// shiftAt es el corte principal que cubre una fecha (0 si cae entre cortes).
func shiftAt(list []shift, moment time.Time) int {
	for _, current := range list {
		if !current.main || moment.Before(current.start) {
			continue
		}
		if current.end == nil || !moment.After(*current.end) {
			return current.id
		}
	}
	return 0
}

// saleTotals acumula los importes de una venta.
type saleTotals struct {
	subtotal decimal.Decimal
	tax      decimal.Decimal
	total    decimal.Decimal
}

func (totals *saleTotals) add(subtotal, tax, total decimal.Decimal) {
	totals.subtotal = totals.subtotal.Add(subtotal)
	totals.tax = totals.tax.Add(tax)
	totals.total = totals.total.Add(total)
}

// history son las ventas, sus renglones, complementos, modificaciones, descuentos, cancelaciones, pagos, cortes,
// retiros y salidas.
func (generator *generator) history(catalog catalogData) {
	list := shifts()
	shiftSales := map[int]decimal.Decimal{}
	first := stamp(2019, 10, 3, 0, 0, 0, 0)
	last := stamp(2026, 11, 10, 0, 0, 0, 0)
	span := int(last.Sub(first).Hours() / 24)
	lineID := 0
	paymentID := 0
	cancellationID := 0
	cardFolio := ""
	soldLateProduct := false
	for index := 0; index < salesCount; index++ {
		id := index + 1
		moment := first.AddDate(0, 0, index*span/salesCount).Add(
			time.Duration(8+generator.random.IntN(13))*time.Hour +
				time.Duration(generator.random.IntN(60))*time.Minute +
				time.Duration(generator.random.IntN(60))*time.Second +
				time.Duration(millis[generator.random.IntN(len(millis))])*time.Millisecond)
		cancelled := index%23 == 5
		open := index >= salesCount-3
		withoutLines := index == 40 || index == 41
		register := 1
		if index%5 == 0 {
			register = 3
		} else if moment.Year() >= 2022 && index%3 == 1 {
			register = 2
		}
		cashier := index%3 + 1
		table := 1
		if index%2 == 1 {
			table = 2 + index%7
		}
		var name any
		if table == 1 {
			name = difficultTexts[index%len(difficultTexts)]
			if index%6 == 0 {
				name = fmt.Sprintf("Cliente sintético %d", id)
			}
		} else if index%4 == 1 {
			name = "Mesa de prueba"
		}
		totals := saleTotals{}
		if !withoutLines {
			products := []int{}
			for count := 1 + generator.random.IntN(3); count > 0; count-- {
				products = append(products, catalog.sellable[generator.random.IntN(len(catalog.sellable))])
			}
			// Un renglón de un producto antes de su primer precio, y el producto al 16 % desde 11/2025.
			if !soldLateProduct && moment.After(stamp(2023, 5, 1, 0, 0, 0, 0)) {
				products = append(products, 19)
				soldLateProduct = true
			}
			if moment.After(stamp(2025, 11, 1, 0, 0, 0, 0)) && index%2 == 0 {
				products = append(products, 20)
			}
			for position, product := range products {
				lineID++
				generator.saleLine(catalog, &totals, lineID, id, index, position, product, moment)
			}
		}
		// Totales incoherentes: el total (o el subtotal, o el IVA) no cuadra con los renglones.
		switch index {
		case 60:
			totals.subtotal = totals.subtotal.Add(amount("10"))
			totals.total = totals.total.Add(amount("10"))
		case 61:
			totals.total = totals.total.Add(amount("10"))
		case 62:
			totals.tax = totals.tax.Add(amount("0.0001"))
		}
		discount := decimal.Zero
		if (index%19 == 2 || index == 57) && !cancelled && !withoutLines {
			discount = decimal.Min(totals.total, amount("15"))
			if index == 57 {
				discount = totals.total
			}
			descriptions := []any{"Atención al cliente", "atención al cliente", "Atención al cliente ", "", nil, "Promo «2×1»"}
			generator.add("descuentos", int64(1+index%3), int64(id), descriptions[index%len(descriptions)], moneyText(discount))
		}
		var extra any
		extraAmount := decimal.Zero
		if index%13 == 4 {
			extras := []any{"Servicio a domicilio", "Descorche", "Cargo «especial» | barra"}
			extra = extras[index%len(extras)]
			extraAmount = amount("25")
			switch index {
			case 17:
				extraAmount = decimal.Zero
			case 30:
				extra = nil
			}
		}
		due := totals.total.Add(extraAmount).Sub(discount)
		paid := decimal.Zero
		saleShift := 0
		if !cancelled && !open {
			for _, payment := range generator.payments(index, moment, due) {
				paymentID++
				kind := int64(1)
				var folio any = ""
				if index%10 == 3 {
					folio = nil
				}
				if moment.Year() >= 2024 && index%3 == 0 {
					kind = 2
					folio = fmt.Sprintf("T%06d", 100000+paymentID*37)
					if index == 201 && cardFolio != "" {
						folio = cardFolio
					}
					cardFolio = folio.(string)
				}
				paymentShift := shiftAt(list, payment.moment)
				if payment.moment.Year() == 2023 && paymentID%4 == 0 {
					paymentShift = 0
				}
				var shiftValue any
				if paymentShift != 0 {
					shiftValue = int64(paymentShift)
				}
				generator.add("pagos", int64(paymentID), int64(id), folio, kind, moneyText(payment.amount), int64(cashier),
					when(payment.moment), shiftValue)
				paid = paid.Add(payment.amount)
				saleShift = paymentShift
			}
		}
		if cancelled {
			cancellationID++
			generator.add("cancelaciones", int64(cancellationID), int64(id), int64(1+index%2), when(moment.Add(25*time.Minute)))
			if index%2 == 0 {
				saleShift = shiftAt(list, moment)
			}
		}
		if saleShift != 0 {
			shiftSales[saleShift] = shiftSales[saleShift].Add(due)
		}
		folio := fmt.Sprintf("%d", id)
		if index == 150 {
			folio = fmt.Sprintf("%d", id-1)
		}
		paidFlag := int64(1)
		if open {
			paidFlag = 0
		}
		inTable := int64(0)
		if cancelled && table != 1 {
			inTable = 1
		}
		var shiftValue any
		if saleShift != 0 {
			shiftValue = int64(saleShift)
		}
		var extraValue any
		if extra != nil || index%13 == 4 {
			extraValue = moneyText(extraAmount)
		} else {
			extraValue = "0.0000"
		}
		generator.add("ventas", int64(id), int64(register), int64(table), name, folio, when(moment), int64(cashier),
			int64(0), moneyText(totals.subtotal), moneyText(totals.tax), moneyText(totals.total), moneyText(paid),
			paidFlag, extra, extraValue, inTable, shiftValue, int64(0))
	}
	generator.shiftsAndWithdrawals(list, shiftSales)
}

// saleLine agrega un renglón (y sus complementos y modificaciones) a una venta.
func (generator *generator) saleLine(
	catalog catalogData,
	totals *saleTotals,
	lineID, saleID, index, position, product int,
	moment time.Time,
) {
	quantity := int64(1 + generator.random.IntN(3))
	original := quantity
	unit := priceAt(catalog.productPrices[product], moment)
	switch {
	case index%31 == 11 && position == 0:
		// Reducción autorizada: de 2 a 1.
		quantity, original = 1, 2
		generator.add("ventas_modificaciones", int64(lineID), "2.000", "1.000", int64(1+index%2), when(moment.Add(10*time.Minute)))
	case index%53 == 12 && position == 0:
		quantity, original = 3, 1
		generator.add("ventas_modificaciones", int64(lineID), "1.000", "3.000", int64(1), when(moment.Add(12*time.Minute)))
	case index%11 == 7 && position == 0:
		// Modificación sin cambio de cantidad (complementos u observaciones).
		generator.add("ventas_modificaciones", int64(lineID), quantityText(decimal.NewFromInt(quantity)),
			quantityText(decimal.NewFromInt(quantity)), int64(2), nil)
	}
	if index%17 == 6 && position > 0 {
		// Agregado después de la primera orden.
		original = 0
	}
	if index%43 == 8 && position == 0 {
		// Renglón en cantidad 0 que se conserva (importe 0).
		quantity = 0
	}
	count := decimal.NewFromInt(quantity)
	subtotal := unit.subtotal.Mul(count)
	tax := unit.tax.Mul(count)
	total := unit.total.Mul(count)
	var notes any
	switch index % 5 {
	case 0:
		notes = difficultTexts[(index/5)%len(difficultTexts)]
	case 1:
		notes = ""
	case 2:
		notes = nil
	default:
		notes = "Sin observaciones"
	}
	generator.add("ventas_productos", int64(lineID), int64(saleID), int64(product), quantityText(count),
		quantityText(decimal.NewFromInt(original)), moneyText(subtotal), moneyText(tax), moneyText(total), notes)
	totals.add(subtotal, tax, total)
	for relationIndex, link := range catalog.relations[product] {
		if (index+relationIndex+position)%2 == 1 {
			continue
		}
		ordered := decimal.NewFromInt(int64(generator.random.IntN(3)))
		if link.included.IsPositive() && generator.random.IntN(3) == 0 {
			// Se quitó un complemento incluido: el sistema anterior lo bonificaba (importe negativo).
			ordered = decimal.Zero
		}
		difference := ordered.Sub(link.included).Mul(count)
		complementPrice := priceAt(catalog.complementPrices[link.complement], moment)
		values := []any{int64(lineID), int64(link.complement), quantityText(link.included), quantityText(ordered),
			moneyText(complementPrice.subtotal.Mul(difference)), moneyText(complementPrice.tax.Mul(difference)),
			moneyText(complementPrice.total.Mul(difference))}
		generator.add("ventas_complementos", values...)
		totals.add(complementPrice.subtotal.Mul(difference), complementPrice.tax.Mul(difference), complementPrice.total.Mul(difference))
		if index == 77 && relationIndex == 0 {
			// Dos filas idénticas (la tabla no tiene llave primaria).
			generator.add("ventas_complementos", values...)
			totals.add(complementPrice.subtotal.Mul(difference), complementPrice.tax.Mul(difference), complementPrice.total.Mul(difference))
		}
	}
}

type payment struct {
	moment time.Time
	amount decimal.Decimal
}

// payments son los pagos de una venta: uno normal, partido, repetido, de menos, con uno en cero o uno negativo.
func (generator *generator) payments(index int, moment time.Time, due decimal.Decimal) []payment {
	at := moment.Add(time.Duration(30+generator.random.IntN(90)) * time.Minute)
	if index%41 == 9 {
		at = at.AddDate(0, 0, 3)
	}
	switch {
	case index == 100:
		return []payment{{at, due.Sub(amount("5"))}}
	case index == 110:
		return []payment{{at, decimal.Zero}, {at.Add(time.Minute), due}}
	case index == 120:
		return []payment{{at, amount("-5")}, {at.Add(time.Minute), due.Add(amount("5"))}}
	case index%37 == 7:
		return []payment{{at, due}, {at.Add(2 * time.Minute), due}}
	case index%29 == 3 && due.GreaterThan(amount("1")):
		half := due.Div(decimal.NewFromInt(2)).Round(4)
		return []payment{{at, half}, {at.Add(3 * time.Minute), due.Sub(half)}}
	}
	return []payment{{at, due}}
}

// shiftsAndWithdrawals escribe los cortes con su venta, retiros y diferencia (como los guardaba el sistema anterior),
// los retiros (cortes parciales, también en cero, negativos y uno enorme) y las salidas de efectivo.
func (generator *generator) shiftsAndWithdrawals(list []shift, sales map[int]decimal.Decimal) {
	withdrawals := []struct {
		shift  int
		amount string
		reason string
		moment time.Time
		user   int
	}{
		{2, "1000.0000", "Retiro para cambio", stamp(2022, 5, 10, 13, 0, 0, 0), 1},
		{2, "1500000.0000", "Depósito «banco» | ¿error de captura?", stamp(2022, 9, 1, 19, 15, 0, 123), 1},
		{3, "0.0000", "", stamp(2023, 2, 14, 15, 0, 0, 0), 2},
		{3, "-50.0000", "Devolución\nal fondo", stamp(2023, 8, 20, 11, 30, 0, 457), 2},
		{4, "2000.0000", "Pago a proveedor", stamp(2024, 4, 4, 16, 0, 0, 0), 1},
		{5, "800.0000", "Retiro", stamp(2025, 6, 30, 20, 0, 0, 997), 2},
		{6, "300.0000", "Retiro parcial", stamp(2026, 2, 2, 12, 0, 0, 0), 3},
	}
	withdrawn := map[int]decimal.Decimal{}
	for index, withdrawal := range withdrawals {
		generator.add("cortes_parciales", int64(index+1), int64(withdrawal.shift), withdrawal.amount, withdrawal.reason,
			when(withdrawal.moment), int64(withdrawal.user))
		withdrawn[withdrawal.shift] = withdrawn[withdrawal.shift].Add(amount(withdrawal.amount))
	}
	for _, current := range list {
		if current.end == nil {
			// Abierto: fin y venta en su valor predeterminado (0), sin retiros ni diferencia calculados.
			generator.add("cortes", int64(current.id), int64(current.cashier), moneyText(current.opening), "0.0000",
				"0.0000", int64(current.register), "A", nil, when(current.start), nil, nil, nil)
			continue
		}
		sale := sales[current.id]
		if current.id == 4 {
			// La venta guardada no coincide con la suma de sus ventas.
			sale = sale.Add(amount("1"))
		}
		withdrawalTotal := withdrawn[current.id]
		closing := current.opening.Add(sale).Sub(withdrawalTotal).Add(current.difference)
		generator.add("cortes", int64(current.id), int64(current.cashier), moneyText(current.opening), moneyText(closing),
			moneyText(sale), int64(current.register), "C", int64(current.cashier), when(current.start), when(*current.end),
			moneyText(withdrawalTotal), moneyText(current.difference))
	}
	generator.add("salidas", int64(4), "120.5000", when(stamp(2024, 3, 3, 12, 0, 0, 0)), int64(1))
	generator.add("salidas", int64(5), "80.0000", when(stamp(2025, 8, 8, 9, 30, 0, 3)), int64(2))
}
