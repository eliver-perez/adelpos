package documents

import (
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/plural"
)

// Bloques de los comprobantes y cortes de caja. La lógica es la de payments.renderCashDocument
// (renderReport, renderDeclared, renderSensitive y renderVoucher) de las etapas 7 y 7.1. El
// esperado y las diferencias solo se imprimen con RenderOptions.IncludeExpected y si el corte los
// conserva; en otro caso el bloque protegido imprime «Esperado y diferencias: no incluidos.».

var differenceLabels = map[string]string{
	"even":  "Sin diferencia",
	"over":  "Sobrante",
	"short": "Faltante",
}

var currencyNames = map[string]string{
	"MXN": "Pesos",
	"USD": "Dólares",
}

// zeroAmount indica si un importe guardado es cero (o no es un importe).
func zeroAmount(amount string) bool {
	value, err := decimal.NewFromString(amount)
	return err != nil || value.IsZero()
}

func renderCashBlock(block Block, data *CashData, options RenderOptions, w *writer) {
	if data.Voucher != nil {
		renderVoucherBlock(block, data.Kind, data.Voucher, w)
		return
	}
	if data.Cut != nil {
		renderCutBlock(block, data.Cut, options, w)
	}
}

func headingEnabled(block Block) bool {
	if options, ok := block.parsed.(*HeadingOptions); ok {
		return options.Heading
	}
	return true
}

func renderCutBlock(block Block, cut *CashCutData, options RenderOptions, w *writer) {
	totals := cut.Totals
	switch block.Type {
	case BlockOpeningFloat:
		w.row("Fondo inicial", pesos(cut.OpeningFloat))
		if cut.FloatUSD != nil {
			w.row("Fondo inicial en dólares", dollars(*cut.FloatUSD))
		}
		if cut.USDRate != nil {
			w.row("Tipo de cambio del turno", rate(*cut.USDRate))
		}
	case BlockSalesByMethod:
		if headingEnabled(block) {
			w.text("VENTAS Y PAGOS POR MÉTODO")
		}
		if len(totals.Methods) == 0 {
			w.text("Sin cobros registrados.")
		}
		for _, method := range totals.Methods {
			w.row(method.Name, pesos(method.Sales))
			if !zeroAmount(method.Tips) {
				w.row("   Propinas", pesos(method.Tips))
			}
			if !zeroAmount(method.Refunds) {
				w.row("   Devoluciones", pesos(method.Refunds))
			}
		}
	case BlockShiftTotals:
		w.row("Ventas", pesos(totals.Sales))
		w.row("Devoluciones", pesos(totals.Refunded))
		w.row("Propinas (no son venta)", pesos(totals.Tips))
		w.row("Descuentos ("+strconv.Itoa(totals.DiscountCount)+")", pesos(totals.Discounts))
		w.row("Cortesías ("+strconv.Itoa(totals.CourtesyCount)+")", pesos(totals.Courtesies))
		w.row("Cobros registrados", strconv.Itoa(totals.Payments))
	case BlockCashMovements:
		renderCashMovements(block, totals, w)
	case BlockDeclaredCash:
		renderDeclared(block, cut, w)
	case BlockExpectedDifferences:
		renderSensitive(cut, options.IncludeExpected, w)
	case BlockObservations:
		if cut.Declared == nil || strings.TrimSpace(cut.Declared.CashierNotes) == "" {
			return
		}
		section := sectionOptions(block)
		if section.Divider {
			w.rule("-")
		}
		if section.Heading {
			w.text("OBSERVACIONES")
		}
		w.text(cut.Declared.CashierNotes)
	}
}

// renderCashMovements escribe los ingresos, retiros, gastos y correcciones de cada moneda y las
// regularizaciones de turnos cerrados.
func renderCashMovements(block Block, totals CashTotals, w *writer) {
	if headingEnabled(block) {
		w.text("INGRESOS, RETIROS Y GASTOS")
	}
	for _, cash := range totals.Cash {
		w.text(currencyNames[cash.Currency])
		w.row("   Ingresos", currencyAmount(cash.Currency, cash.Income))
		w.row("   Retiros", currencyAmount(cash.Currency, cash.Withdrawals))
		w.row("   Gastos", currencyAmount(cash.Currency, cash.Expenses))
		if !zeroAmount(cash.CorrectionsIn) || !zeroAmount(cash.CorrectionsOut) {
			w.row("   Correcciones que entran", currencyAmount(cash.Currency, cash.CorrectionsIn))
			w.row("   Correcciones que salen", currencyAmount(cash.Currency, cash.CorrectionsOut))
		}
	}
	if totals.Regularizations > 0 {
		// La frase concuerda con el número: «1 regularización de un turno cerrado; no cambia este
		// turno.» y «2 regularizaciones de turnos cerrados; no cambian este turno.».
		regularizations := plural.Count(
			totals.Regularizations,
			"regularización de un turno cerrado",
			"regularizaciones de turnos cerrados",
		)
		verb := plural.Word(totals.Regularizations, "no cambia", "no cambian")
		w.text(regularizations + "; " + verb + " este turno.")
	}
}

// renderDeclared escribe lo que el cajero contó en el arqueo (solo en el corte definitivo).
func renderDeclared(block Block, cut *CashCutData, w *writer) {
	declared := cut.Declared
	if declared == nil {
		return
	}
	section := sectionOptions(block)
	if section.Divider {
		w.rule("-")
	}
	if section.Heading {
		w.text("EFECTIVO DECLARADO")
	}
	w.row("Pesos", pesos(declared.CountedMXN))
	if cut.Dollars {
		w.row("Dólares", dollars(declared.CountedUSD))
	}
	if !zeroAmount(declared.ExtraMXN) {
		w.row("   Incluye adicional en pesos", pesos(declared.ExtraMXN))
	}
	if !zeroAmount(declared.ExtraUSD) {
		w.row("   Incluye adicional en dólares", dollars(declared.ExtraUSD))
	}
}

// renderSensitive escribe el efectivo esperado y las diferencias por moneda, o la nota de que no
// se incluyen (sin permiso, durante el arqueo o si el corte no los conserva).
func renderSensitive(cut *CashCutData, includeExpected bool, w *writer) {
	sensitive := cut.Sensitive
	if !includeExpected || sensitive == nil {
		w.text("Esperado y diferencias: no incluidos.")
		return
	}
	w.text("EFECTIVO ESPERADO")
	w.row("Pesos", pesos(sensitive.ExpectedMXN))
	w.row("Dólares", dollars(sensitive.ExpectedUSD))
	if sensitive.TotalEquivalentMXN != nil {
		w.row("Total equivalente en pesos", pesos(*sensitive.TotalEquivalentMXN))
	}
	differences := sensitive.Differences
	if differences == nil {
		return
	}
	w.text("DIFERENCIAS (CONTADO - ESPERADO)")
	w.row("Pesos · "+differenceLabels[differences.ResultMXN], pesos(differences.MXN))
	w.row("Dólares · "+differenceLabels[differences.ResultUSD], dollars(differences.USD))
	if differences.EquivalentMXN != nil {
		w.row("Equivalente en pesos", pesos(*differences.EquivalentMXN))
	}
}

// renderVoucherBlock escribe el detalle y las firmas de un comprobante de ingreso, retiro o gasto.
// Nunca contiene el efectivo esperado de la caja.
func renderVoucherBlock(block Block, kind Kind, voucher *CashVoucherData, w *writer) {
	switch block.Type {
	case BlockVoucherDetail:
		options, _ := block.parsed.(*VoucherDetailOptions)
		showReference := options == nil || options.ShowReference
		showNotes := options == nil || options.ShowNotes
		if kind == KindCashExpense && voucher.Category != nil {
			w.text("Categoría: " + *voucher.Category)
		}
		if voucher.TypeLabel != nil {
			// Un retiro nombra su motivo; un ingreso, su tipo.
			label := "Tipo: "
			if kind == KindCashWithdrawal {
				label = "Motivo: "
			}
			w.text(label + *voucher.TypeLabel)
		}
		w.text("Concepto: " + voucher.Concept)
		w.row("Importe", currencyAmount(voucher.Currency, voucher.Amount))
		if voucher.Currency == currencyUSD && voucher.USDRate != nil {
			w.row("Tipo de cambio", rate(*voucher.USDRate))
			w.row("Equivale a", pesos(voucher.EquivalentMXN))
		}
		if showReference && voucher.Reference != "" {
			w.text("Referencia: " + voucher.Reference)
		}
		if showNotes && voucher.Notes != "" {
			w.text("Observaciones: " + voucher.Notes)
		}
		if voucher.Exceptional {
			w.text("Salida excepcional: " + voucher.Reason)
		}
	case BlockSignatures:
		options, _ := block.parsed.(*SignaturesOptions)
		showStatus := options == nil || options.ShowStatus
		signatureLine := options == nil || options.SignatureLine
		w.text("Registró: " + voucher.ActorName + " · " + voucher.DeviceName)
		if voucher.AuthorizedByName != nil && voucher.AuthorizationMode != nil {
			w.text("Autorizó: " + *voucher.AuthorizedByName)
			if *voucher.AuthorizationMode == "supervisor" {
				w.text("   Autorización de supervisor")
			} else {
				w.text("   Con su propio permiso")
			}
		}
		if showStatus && voucher.Corrected {
			w.text("Estado: corregido por una corrección posterior.")
		}
		if signatureLine {
			w.text("")
			w.text("Firma: ______________________")
		}
	}
}
