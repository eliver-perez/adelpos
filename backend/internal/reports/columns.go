package reports

import "github.com/shopspring/decimal"

// Constructores de columnas para que las declaraciones de los reportes sean breves y homogéneas.

func textColumn(key, label string) Column {
	return Column{Key: key, Label: label, Type: TypeText, Sortable: true}
}

func codeColumn(key, label string) Column {
	return Column{Key: key, Label: label, Type: TypeCode, Sortable: true}
}

func dateColumn(key, label string) Column {
	return Column{Key: key, Label: label, Type: TypeDate, Sortable: true}
}

func dateTimeColumn(key, label string) Column {
	return Column{Key: key, Label: label, Type: TypeDateTime, Sortable: true}
}

// countColumn es un entero que se suma en los totales.
func countColumn(key, label string) Column {
	return Column{Key: key, Label: label, Type: TypeInteger, Total: TotalSum, Sortable: true}
}

// moneyColumn es un importe en pesos que se suma en los totales.
func moneyColumn(key, label string) Column {
	return Column{Key: key, Label: label, Type: TypeMoneyMXN, Total: TotalSum, Sortable: true}
}

// dollarColumn es un importe en dólares (nunca se suma con pesos).
func dollarColumn(key, label string) Column {
	return Column{Key: key, Label: label, Type: TypeMoneyUSD, Total: TotalSum, Sortable: true}
}

// percentColumn es un porcentaje cuyo total calcula el reporte.
func percentColumn(key, label string) Column {
	return Column{Key: key, Label: label, Type: TypePercent, Total: TotalCustom, Sortable: true}
}

func optional(column Column) Column {
	column.Visibility = VisibilityOptional
	return column
}

func described(column Column, description string) Column {
	column.Description = description
	return column
}

func requiring(column Column, permissions ...string) Column {
	column.Permissions = permissions
	return column
}

// Medidas de venta del libro de reconocimientos (definiciones en doc.go, «Definiciones de métricas»).
var (
	columnAccounts = described(countColumn("accounts", "Cuentas"),
		"Cuentas cuyo cierre (primer reconocimiento) ocurrió en el periodo; una reliquidación no vuelve a contarla.")
	columnQuantity = described(countColumn("quantity", "Cantidad"),
		"Unidades comerciales vendidas (un combo cuenta como una unidad).")
	columnGross = described(moneyColumn("gross", "Importe bruto"),
		"Precio de las unidades antes de promociones, con IVA incluido.")
	columnPromotion = described(optional(moneyColumn("promotion", "Promociones")),
		"Descuento de las promociones del catálogo.")
	columnDiscount = described(moneyColumn("discount", "Descuentos"),
		"Descuentos manuales vigentes (por partida y generales), sin cortesías.")
	columnCourtesy = described(moneyColumn("courtesy", "Cortesías"),
		"Importe de las cortesías al 100 %.")
	columnCharge = described(moneyColumn("charge", "Cargos"),
		"Cargos generales de cuenta.")
	columnNet = described(moneyColumn("net", "Venta neta"),
		"Importe bruto − promociones − descuentos − cortesías + cargos; IVA incluido, sin propinas.")
	columnBase = described(optional(moneyColumn("base", "Subtotal sin IVA")),
		"Venta neta menos su IVA.")
	columnTax = described(moneyColumn("tax", "IVA"),
		"IVA incluido en la venta neta, calculado por cuenta y tasa con las tasas registradas.")
	columnAverageTicket = described(Column{Key: "average_ticket", Label: "Ticket promedio", Type: TypeMoneyMXN,
		Total: TotalCustom, Sortable: true}, "Venta neta entre cuentas cerradas.")
	columnShare = described(percentColumn("share", "% de la venta"),
		"Participación en la venta neta del conjunto filtrado.")
)

// salesMeasureColumns son las medidas de venta en su orden habitual.
func salesMeasureColumns() []Column {
	return []Column{columnQuantity, columnGross, columnPromotion, columnDiscount, columnCourtesy, columnCharge, columnNet,
		columnBase, columnTax}
}

// withColumns concatena listas de columnas.
func withColumns(groups ...[]Column) []Column {
	result := []Column{}
	for _, group := range groups {
		result = append(result, group...)
	}
	return result
}

// shareTotals calcula la participación de cada renglón en la venta neta y el 100 % del total.
func shareTotals(rows []Row, totals Row) {
	net := decimalOf(totals, "net")
	for _, row := range rows {
		row["share"] = percentOf(decimalOf(row, "net"), net)
	}
	totals["share"] = percentOf(net, net)
}

// averageTicketTotals calcula el ticket promedio de cada renglón y del total.
func averageTicketTotals(rows []Row, totals Row) {
	for _, row := range rows {
		row["average_ticket"] = averageTicket(row)
	}
	totals["average_ticket"] = averageTicket(totals)
}

func averageTicket(row Row) any {
	accounts := decimalOf(row, "accounts")
	if accounts.IsZero() {
		return nil
	}
	return decimalOf(row, "net").DivRound(accounts, 2)
}

// zeroMeasures completa con ceros las medidas de un renglón sin datos (días u horas sin ventas).
func zeroMeasures(row Row, columns []Column) Row {
	for _, column := range columns {
		if _, found := row[column.Key]; found {
			continue
		}
		switch column.Type {
		case TypeInteger:
			row[column.Key] = int64(0)
		case TypeMoneyMXN, TypeMoneyUSD, TypeQuantity:
			row[column.Key] = decimal.Zero
		}
	}
	return row
}
