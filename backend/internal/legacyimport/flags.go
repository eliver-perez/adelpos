package legacyimport

// Marcas de incoherencia (flags text[] de las tablas del historial): lista cerrada. Los datos incoherentes se importan
// tal como estaban, con su marca, y se listan en el informe (§4.2); nunca se corrigen solos. Listed: aparece renglón
// por renglón en la lista de datos incoherentes del PDF (folio, fecha, marca e importes); las demás son informativas
// y solo se cuentan. La misma lista está en docs/MIGRACION_ANALISIS.md §6.

// Entidades a las que pertenece una marca.
const (
	EntitySale       = "sale"
	EntityLine       = "line"
	EntityComplement = "complement"
	EntityPayment    = "payment"
	EntityDiscount   = "discount"
	EntityShift      = "shift"
	EntityWithdrawal = "withdrawal"
	EntityPrice      = "price"
)

// FlagInfo describe una marca.
type FlagInfo struct {
	Entity string
	Code   string
	Label  string
	Listed bool
}

// Marcas de ventas.
const (
	FlagNoLines               = "no_lines"
	FlagTotalMismatch         = "total_mismatch"
	FlagSubtotalMismatch      = "subtotal_mismatch"
	FlagTaxMismatch           = "tax_mismatch"
	FlagAmountsInconsistent   = "amounts_inconsistent"
	FlagOverpaid              = "overpaid"
	FlagUnderpaid             = "underpaid"
	FlagNoPayments            = "no_payments"
	FlagCancelledWithPayments = "cancelled_with_payments"
	FlagOpenWithPayments      = "open_with_payments"
	FlagPaidAmountMismatch    = "paid_amount_mismatch"
	FlagNoShift               = "no_shift"
	FlagLatePayment           = "late_payment"
	FlagDuplicateFolio        = "duplicate_folio"
	FlagTaxEstimated          = "tax_estimated"
	FlagNegativeTax           = "negative_tax"
	FlagChargeWithoutAmount   = "charge_without_amount"
	FlagChargeWithoutConcept  = "charge_without_concept"
	FlagUnknownTable          = "unknown_table"
	FlagUnknownCashier        = "unknown_cashier"
	FlagUnknownRegister       = "unknown_register"
	FlagUnknownShift          = "unknown_shift"
)

// Marcas de renglones y complementos.
const (
	FlagZeroQuantity      = "zero_quantity"
	FlagUnknownProduct    = "unknown_product"
	FlagUnknownSale       = "unknown_sale"
	FlagRateNotStandard   = "rate_not_standard"
	FlagNegativeAmount    = "negative_amount"
	FlagUnknownComplement = "unknown_complement"
	FlagUnknownLine       = "unknown_line"
)

// Marcas de pagos, descuentos, cortes y retiros.
const (
	FlagZeroAmount          = "zero_amount"
	FlagPossibleDuplicate   = "possible_duplicate"
	FlagShiftDiffers        = "shift_differs"
	FlagUnknownMethod       = "unknown_method"
	FlagBeforeSale          = "before_sale"
	FlagExceedsTotal        = "exceeds_total"
	FlagZeroOrNegative      = "zero_or_negative"
	FlagUnknownType         = "unknown_type"
	FlagSalesMismatch       = "sales_mismatch"
	FlagWithdrawalsMismatch = "withdrawals_mismatch"
	FlagNoActivity          = "no_activity"
	FlagUnknownStatus       = "unknown_status"
	FlagDoubtfulAmount      = "doubtful_amount"
)

// Flags es la lista cerrada, en el orden en que la presenta el informe.
var Flags = []FlagInfo{
	{EntitySale, FlagTotalMismatch, "Venta cuyo total no cuadra con sus renglones (la diferencia queda como «Diferencia del sistema anterior»)", true},
	{EntitySale, FlagSubtotalMismatch, "Venta cuyo subtotal no cuadra con sus renglones", true},
	{EntitySale, FlagTaxMismatch, "Venta cuyo IVA no cuadra con sus renglones", true},
	{EntitySale, FlagAmountsInconsistent, "Venta con subtotal + IVA distinto del total", true},
	{EntitySale, FlagNoLines, "Venta sin renglones", true},
	{EntitySale, FlagOverpaid, "Venta pagada con pagos de más", true},
	{EntitySale, FlagUnderpaid, "Venta pagada con pagos de menos", true},
	{EntitySale, FlagNoPayments, "Venta pagada sin pagos (se cuenta en su fecha de apertura)", true},
	{EntitySale, FlagCancelledWithPayments, "Venta cancelada con pagos", true},
	{EntitySale, FlagOpenWithPayments, "Venta abierta con pagos", true},
	{EntitySale, FlagPaidAmountMismatch, "Venta cuyo «monto pagado» no coincide con sus pagos", true},
	{EntitySale, FlagDuplicateFolio, "Folio repetido", true},
	{EntitySale, FlagTaxEstimated, "IVA estimado (no estaba guardado)", true},
	{EntitySale, FlagNegativeTax, "Venta con IVA negativo", true},
	{EntitySale, FlagChargeWithoutAmount, "Cargo extra con descripción y sin importe", true},
	{EntitySale, FlagChargeWithoutConcept, "Cargo extra sin descripción", true},
	{EntitySale, FlagUnknownTable, "Venta de una mesa que no existe", true},
	{EntitySale, FlagUnknownCashier, "Venta de un cajero que no existe", true},
	{EntitySale, FlagUnknownRegister, "Venta de una caja que no existe", true},
	{EntitySale, FlagUnknownShift, "Venta de un corte que no existe", true},
	{EntitySale, FlagNoShift, "Venta pagada sin corte («Sin turno»)", false},
	{EntitySale, FlagLatePayment, "Último pago más de 2 días después de abrir la venta", false},
	{EntityLine, FlagAmountsInconsistent, "Renglón con subtotal + IVA distinto del total", false},
	{EntityLine, FlagZeroQuantity, "Renglón con cantidad 0", false},
	{EntityLine, FlagUnknownProduct, "Renglón de un producto que no existe", true},
	{EntityLine, FlagUnknownSale, "Renglón de una venta que no existe", true},
	{EntityLine, FlagRateNotStandard, "Renglón con una tasa de IVA distinta de 0, 8 o 16 %", false},
	{EntityLine, FlagTaxEstimated, "Renglón con IVA estimado", false},
	{EntityComplement, FlagNegativeAmount, "Complemento con importe negativo (se quitó uno incluido y se bonificó)", false},
	{EntityComplement, FlagAmountsInconsistent, "Complemento con subtotal + IVA distinto del total", false},
	{EntityComplement, FlagTaxEstimated, "Complemento con IVA estimado", false},
	{EntityComplement, FlagUnknownComplement, "Complemento que no existe en el catálogo anterior", true},
	{EntityComplement, FlagUnknownLine, "Complemento de un renglón que no existe", true},
	{EntityPayment, FlagZeroAmount, "Pago en cero", true},
	{EntityPayment, FlagNegativeAmount, "Pago negativo", true},
	{EntityPayment, FlagPossibleDuplicate, "Pago posiblemente repetido (mismo importe en una venta cobrada de más)", true},
	{EntityPayment, FlagUnknownMethod, "Pago con una forma de pago desconocida", true},
	{EntityPayment, FlagBeforeSale, "Pago anterior a la apertura de su venta", true},
	{EntityPayment, FlagUnknownSale, "Pago de una venta que no existe", true},
	{EntityPayment, FlagNoShift, "Pago sin corte", false},
	{EntityPayment, FlagShiftDiffers, "Pago en un corte distinto del de su venta", false},
	{EntityDiscount, FlagExceedsTotal, "Descuento mayor que el total de la venta", true},
	{EntityDiscount, FlagZeroOrNegative, "Descuento en cero o negativo", true},
	{EntityDiscount, FlagUnknownSale, "Descuento de una venta que no existe", true},
	{EntityDiscount, FlagUnknownType, "Descuento de un tipo que no existe", false},
	{EntityShift, FlagSalesMismatch, "Corte cuya «venta» guardada no coincide con sus ventas", true},
	{EntityShift, FlagWithdrawalsMismatch, "Corte cuyos «retiros» guardados no coinciden con sus retiros", true},
	{EntityShift, FlagUnknownStatus, "Corte con un estado desconocido", true},
	{EntityShift, FlagNoActivity, "Corte sin ventas ni pagos", false},
	{EntityWithdrawal, FlagZeroOrNegative, "Retiro en cero o negativo", true},
	{EntityWithdrawal, FlagDoubtfulAmount, "Retiro de $100 000 o más (posible error de captura)", true},
	{EntityWithdrawal, FlagUnknownShift, "Retiro de un corte que no existe", true},
	{EntityPrice, FlagRateNotStandard, "Precio con una tasa de IVA distinta de 0, 8 o 16 %", false},
}

// flagInfo busca la descripción de una marca de una entidad.
func flagInfo(entity, code string) (FlagInfo, bool) {
	for _, info := range Flags {
		if info.Entity == entity && info.Code == code {
			return info, true
		}
	}
	return FlagInfo{}, false
}

// flagSet arma la lista de marcas de una fila sin repetir y en el orden en que se agregan.
type flagSet []string

func (flags *flagSet) add(code string) {
	for _, existing := range *flags {
		if existing == code {
			return
		}
	}
	*flags = append(*flags, code)
}

func (flags flagSet) has(code string) bool {
	for _, existing := range flags {
		if existing == code {
			return true
		}
	}
	return false
}

// array es la lista para PostgreSQL (nunca nula).
func (flags flagSet) array() []string {
	if flags == nil {
		return []string{}
	}
	return []string(flags)
}
