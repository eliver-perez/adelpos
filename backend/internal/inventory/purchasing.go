package inventory

import (
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

// Compras (fase C, §16–§20 de la especificación): proveedores, órdenes de compra, recepciones,
// devoluciones a proveedor y vínculos opcionales con gastos de caja. Reglas comunes:
//   - las lecturas de compras las ve quien tiene cualquiera de los permisos de compras; los importes
//     de compra (precio sin IVA, descuento, subtotal, IVA, total y valor asociado de una devolución)
//     son visibles para ellos, y los datos de costeo de inventario (valor de inventario y costo por
//     unidad base de una recepción, valor de salida al promedio, ajuste de costo y diferencia no
//     absorbida de una devolución) solo con inventory.costs.read;
//   - los importes se calculan aquí con decimales y se redondean a centavos mitad alejándose de cero
//     (purchaseLineAmounts); React nunca suma ni redondea;
//   - crear o emitir una orden no toca existencias ni caja; la recepción es la única que aumenta
//     existencias (movimiento receipt por renglón con Apply) y la devolución la única que las
//     disminuye por compras (supplier_return). Ninguna crea gastos ni ingresos de caja.

// purchasingPermissions son los permisos que dan acceso a las lecturas de compras.
var purchasingPermissions = []string{
	permissionSuppliersManage, permissionPurchasesCreate, permissionPurchasesAuthorize, permissionPurchasesReceive,
	permissionSupplierReturns, permissionPurchasesLinkCash,
}

// Permisos de caja (payments) que el inventario respeta para leer gastos. inventory no importa
// payments: los repite como texto y lee las tablas de caja por SQL en solo lectura.
const (
	cashPermissionMovementsRead = "cash.movements.read"
	cashPermissionShiftsReadAll = "cash.shifts.read_all"
	cashPermissionHistoryRead   = "cash.history.read"
)

// resolvePurchasing enumera las rutas de compras. Un solo identificador por ruta (segunda
// posición); el insumo de un proveedor viaja en el cuerpo.
func resolvePurchasing(method string, parts []string) *route {
	length := len(parts)
	resource := parts[0]
	switch {
	case length == 1:
		switch resource + " " + method {
		case "suppliers GET":
			return purchasingRead((*service).listSuppliers)
		case "suppliers POST":
			return write((*service).createSupplier, permissionSuppliersManage)
		case "purchase-orders GET":
			return purchasingRead((*service).listPurchaseOrders)
		case "purchase-orders POST":
			return write((*service).createPurchaseOrder, permissionPurchasesCreate)
		case "receipts GET":
			return purchasingRead((*service).listReceipts)
		case "receipts POST":
			// La recepción directa (sin orden) exige además inventory.purchases.create; se comprueba
			// al decodificar, antes de escribir.
			return write((*service).recordReceipt, permissionPurchasesReceive)
		case "supplier-returns GET":
			return purchasingRead((*service).listSupplierReturns)
		case "supplier-returns POST":
			return write((*service).recordSupplierReturn, permissionSupplierReturns)
		case "cash-expenses GET":
			return read((*service).listCashExpenses, permissionPurchasesLinkCash, cashPermissionMovementsRead).withCash()
		}
	case length == 2:
		switch resource + " " + method {
		case "suppliers GET":
			return purchasingRead((*service).readSupplierRoute)
		case "suppliers PATCH":
			return write((*service).updateSupplier, permissionSuppliersManage)
		case "purchase-orders GET":
			return purchasingRead((*service).readPurchaseOrderRoute)
		case "purchase-orders PATCH":
			return write((*service).updatePurchaseOrder, permissionPurchasesCreate)
		case "receipts GET":
			return purchasingRead((*service).readReceiptRoute)
		case "supplier-returns GET":
			return purchasingRead((*service).readSupplierReturnRoute)
		}
	case length == 3:
		switch resource + " " + parts[2] + " " + method {
		case "suppliers items PUT":
			return write((*service).saveSupplierItem, permissionSuppliersManage)
		case "purchase-orders issue POST":
			return write((*service).issuePurchaseOrder, permissionPurchasesAuthorize)
		case "purchase-orders cancel POST":
			return write((*service).cancelPurchaseOrder, permissionPurchasesAuthorize)
		case "purchase-orders close POST":
			return write((*service).closePurchaseOrder, permissionPurchasesAuthorize)
		case "receipts cash-links POST":
			return write((*service).linkCashExpense, permissionPurchasesLinkCash, cashPermissionMovementsRead).withCash()
		case "cash-links unlink POST":
			return write((*service).unlinkCashExpense, permissionPurchasesLinkCash, cashPermissionMovementsRead).withCash()
		}
	case length == 4 && resource == "suppliers" && parts[2] == "items" && parts[3] == "remove" && method == "POST":
		return write((*service).removeSupplierItem, permissionSuppliersManage)
	}
	return nil
}

// SupplierReference es el proveedor resumido que acompaña a órdenes, recepciones y devoluciones.
// Kind distingue un proveedor formal de «Compras de mostrador» (etapa 9A) en listas y reportes.
type SupplierReference struct {
	ID        string `json:"id"`
	TradeName string `json:"trade_name"`
	Kind      string `json:"kind"`
	Active    bool   `json:"active"`
}

// PresentationReference es la presentación resumida de un renglón de compra.
type PresentationReference struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	BaseQuantity string `json:"base_quantity"`
	Active       bool   `json:"active"`
}

// Importes de compra.
var (
	purchaseAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,9})(\.[0-9]{1,2})?$`)
	purchasePricePattern  = regexp.MustCompile(`^(0|[1-9][0-9]{0,6})(\.[0-9]{1,6})?$`)
	// maximumPurchasePrice coincide con los CHECK de unit_price (precio sin IVA por presentación o unidad).
	maximumPurchasePrice = decimal.NewFromInt(1000000)
	// maximumLineAmount limita el importe de un renglón para que el documento quepa en numeric(18,2).
	maximumLineAmount = decimal.RequireFromString("9999999999.99")
	// purchaseTaxRates es la lista cerrada de tasas de IVA de compra (la misma de ventas).
	purchaseTaxRates = map[int]bool{16: true, 8: true, 0: true}
)

// parsePurchaseAmount acepta un importe en pesos no negativo con hasta dos decimales.
func parsePurchaseAmount(text, subject string) (decimal.Decimal, error) {
	trimmed := strings.TrimSpace(text)
	if !purchaseAmountPattern.MatchString(trimmed) {
		return zero, invalid(subject + " no es válido: usa un importe sin signo con hasta dos decimales.")
	}
	return decimal.RequireFromString(trimmed), nil
}

// parsePurchasePrice acepta un precio unitario sin IVA no negativo con hasta seis decimales.
func parsePurchasePrice(text, subject string) (decimal.Decimal, error) {
	trimmed := strings.TrimSpace(text)
	if !purchasePricePattern.MatchString(trimmed) {
		return zero, invalid(subject + " no es válido: usa un número sin signo con hasta seis decimales.")
	}
	value := decimal.RequireFromString(trimmed)
	if value.GreaterThan(maximumPurchasePrice) {
		return zero, invalid(subject + " excede el máximo admitido.")
	}
	return value, nil
}

// validTaxRate exige una tasa de IVA de la lista cerrada 16/8/0.
func validTaxRate(rate *int, subject string) (int, error) {
	if rate == nil {
		return 0, invalid(subject + ": elige la tasa de IVA (16, 8 o 0 %).")
	}
	if !purchaseTaxRates[*rate] {
		return 0, invalid(subject + ": la tasa de IVA debe ser 16, 8 o 0 %.")
	}
	return *rate, nil
}

// money es la cadena JSON de un importe de compra: siempre dos decimales.
func money(value decimal.Decimal) string {
	return value.StringFixed(2)
}

// roundMoney redondea a centavos mitad alejándose de cero, como el resto del sistema.
func roundMoney(value decimal.Decimal) decimal.Decimal {
	return value.Round(2)
}

// PurchaseAmounts son los importes de un renglón o de un documento de compra.
//   - Gross: cantidad capturada × precio unitario sin IVA, redondeado a centavos;
//   - Discount: descuento comercial en pesos (nunca mayor que el importe);
//   - Subtotal: importe − descuento (base del IVA);
//   - Tax: subtotal × tasa ÷ 100, redondeado a centavos por renglón;
//   - Total: subtotal + IVA.
//
// Los importes del documento son la suma exacta de los de sus renglones (no se recalcula el IVA
// sobre el total), igual que en el sistema anterior, así que el documento siempre cuadra.
type PurchaseAmounts struct {
	Gross    decimal.Decimal
	Discount decimal.Decimal
	Subtotal decimal.Decimal
	Tax      decimal.Decimal
	Total    decimal.Decimal
}

// purchaseLineAmounts calcula los importes de un renglón. Es una función pura.
func purchaseLineAmounts(quantity, unitPrice, discount decimal.Decimal, rate int) (PurchaseAmounts, error) {
	if !quantity.IsPositive() {
		return PurchaseAmounts{}, invalid("La cantidad del renglón debe ser mayor que cero.")
	}
	if unitPrice.IsNegative() || discount.IsNegative() {
		return PurchaseAmounts{}, invalid("El precio y el descuento del renglón no pueden ser negativos.")
	}
	if !purchaseTaxRates[rate] {
		return PurchaseAmounts{}, invalid("La tasa de IVA debe ser 16, 8 o 0 %.")
	}
	gross := roundMoney(quantity.Mul(unitPrice))
	if gross.GreaterThan(maximumLineAmount) {
		return PurchaseAmounts{}, invalid("El importe del renglón excede el máximo admitido.")
	}
	if discount.GreaterThan(gross) {
		return PurchaseAmounts{}, invalid("El descuento no puede ser mayor que el importe del renglón.")
	}
	subtotal := gross.Sub(discount)
	tax := roundMoney(subtotal.Mul(decimal.NewFromInt(int64(rate))).Div(hundred))
	return PurchaseAmounts{Gross: gross, Discount: discount, Subtotal: subtotal, Tax: tax, Total: subtotal.Add(tax)}, nil
}

// add suma los importes de un renglón a los del documento.
func (amounts *PurchaseAmounts) add(line PurchaseAmounts) {
	amounts.Gross = amounts.Gross.Add(line.Gross)
	amounts.Discount = amounts.Discount.Add(line.Discount)
	amounts.Subtotal = amounts.Subtotal.Add(line.Subtotal)
	amounts.Tax = amounts.Tax.Add(line.Tax)
	amounts.Total = amounts.Total.Add(line.Total)
}

// proratedDiscount reparte el descuento de un renglón de orden entre sus recepciones en proporción
// a lo recibido, en centavos y de forma acumulada: la parte de una recepción es
// redondeo(D × min(antes + ahora, Q) ÷ Q) − redondeo(D × min(antes, Q) ÷ Q). Así, las recepciones que
// completan la cantidad ordenada suman exactamente el descuento de la orden (12 + 8 de 20 cajas
// reciben 60 % y 40 %) y lo recibido de más no lleva descuento. Es una función pura.
func proratedDiscount(discount, ordered, receivedBefore, receivingNow decimal.Decimal) decimal.Decimal {
	if !discount.IsPositive() || !ordered.IsPositive() {
		return zero
	}
	share := func(received decimal.Decimal) decimal.Decimal {
		if received.GreaterThanOrEqual(ordered) {
			return discount
		}
		return roundMoney(discount.Mul(received).Div(ordered))
	}
	portion := share(receivedBefore.Add(receivingNow)).Sub(share(receivedBefore))
	if portion.IsNegative() {
		return zero
	}
	return portion
}

// purchaseText recorta un texto opcional de compras (observaciones, referencia) con su límite.
// singleLine rechaza saltos de línea y caracteres de control.
func purchaseText(value, subject string, maximum int, singleLine bool) (string, error) {
	trimmed := strings.TrimSpace(value)
	if textLength(trimmed) > maximum {
		return "", invalid(subject + " admite hasta " + itoa(maximum) + " caracteres.")
	}
	if singleLine && hasControlCharacters(trimmed) {
		return "", invalid(subject + " no admite saltos de línea ni caracteres de control.")
	}
	return trimmed, nil
}

// purchaseReason valida el motivo obligatorio de una cancelación, un cierre, una devolución o una
// desvinculación (5 a 500 caracteres).
func purchaseReason(value, subject string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if textLength(trimmed) < 5 || textLength(trimmed) > 500 {
		return "", invalid(subject + " debe tener entre 5 y 500 caracteres.")
	}
	return trimmed, nil
}
