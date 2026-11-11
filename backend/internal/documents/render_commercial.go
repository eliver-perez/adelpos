package documents

import (
	"strconv"
	"strings"
)

// Bloques de la precuenta y el ticket de pago. La lógica es la de payments.renderDocument y
// cashWithDollars de las etapas 6 a 7.1; los separadores que solo acompañan a un bloque cuando hay
// datos («ESTE PAGO», «PAGOS REGISTRADOS») viven dentro del bloque.

const currencyUSD = "USD"

func renderCommercialBlock(block Block, data *CommercialData, w *writer) {
	switch block.Type {
	case BlockProducts:
		options, _ := block.parsed.(*NotesOptions)
		showNotes := options == nil || options.ShowNotes
		for _, line := range data.Lines {
			w.row(strconv.Itoa(line.Quantity)+" x "+line.Name, pesos(line.Amount))
			if showNotes && strings.TrimSpace(line.Notes) != "" {
				w.text("   Nota: " + line.Notes)
			}
		}
	case BlockAdjustments:
		// Un descuento se imprime «-$50.00»; con motivo del catálogo, «Descuento · Atención».
		options, _ := block.parsed.(*ReasonOptions)
		showReason := options == nil || options.ShowReason
		for _, adjustment := range data.Adjustments {
			concept := adjustment.Concept
			if showReason && adjustment.Reason != "" {
				concept += " · " + adjustment.Reason
			}
			w.row(concept, pesos(adjustment.Amount))
		}
	case BlockTaxes:
		options, _ := block.parsed.(*TaxesOptions)
		showBase := options == nil || options.ShowBase
		for _, tax := range data.Tax {
			rate := strconv.Itoa(tax.Rate)
			if showBase {
				w.row("Subtotal sin IVA "+rate+" %", pesos(tax.Base))
			}
			w.row("IVA "+rate+" %", pesos(tax.Tax))
		}
	case BlockTotal:
		w.row("TOTAL", pesos(data.Total))
	case BlockUSDTotal:
		// Línea informativa (etapa 7.1): el total en dólares con el tipo de cambio del turno.
		if data.USDTotal != nil && data.USDRate != nil {
			w.row("Total en dólares (T.C. "+pesos(*data.USDRate)+")", dollars(*data.USDTotal))
		}
	case BlockThisPayment:
		renderThisPayment(block, data, w)
	case BlockPayments:
		renderRegisteredPayments(block, data, w)
	case BlockBalance:
		if data.Balance != "0.00" {
			w.row("SALDO PENDIENTE", pesos(data.Balance))
		}
	}
}

func sectionOptions(block Block) SectionOptions {
	if options, ok := block.parsed.(*SectionOptions); ok {
		return *options
	}
	return SectionOptions{Divider: true, Heading: true}
}

// renderThisPayment detalla el pago del ticket: método, referencia, efectivo recibido (o pesos y
// dólares), cambio y propina. La precuenta no tiene pago propio.
func renderThisPayment(block Block, data *CommercialData, w *writer) {
	payment := data.Payment
	if payment == nil {
		return
	}
	options := sectionOptions(block)
	if options.Divider {
		w.rule("-")
	}
	if options.Heading {
		w.text("ESTE PAGO")
	}
	w.row(payment.Method, pesos(payment.Amount))
	if payment.Reference != "" {
		w.text("   Ref. " + payment.Reference)
	}
	if payment.ReceivedUSD != nil {
		cashWithDollars(*payment, w)
	} else if payment.Received != nil {
		w.row("   Efectivo recibido", pesos(*payment.Received))
		w.row("   Cambio", pesos(payment.Change))
	}
	if payment.Tip != "0.00" {
		w.row("   Propina (no es venta)", pesos(payment.Tip))
	}
}

// cashWithDollars detalla un efectivo con dólares: lo recibido en cada moneda, el tipo de cambio
// del turno, el total en pesos y el cambio con la moneda en que se entregó.
func cashWithDollars(payment CommercialPayment, w *writer) {
	w.row("   Pesos recibidos", pesos(valueOf(payment.ReceivedMXN)))
	w.row("   Dólares recibidos", dollars(valueOf(payment.ReceivedUSD)))
	w.row("   Tipo de cambio", rate(valueOf(payment.USDRate)))
	if payment.Received != nil {
		w.row("   Total recibido", pesos(*payment.Received))
	}
	if payment.ChangeCurrency != nil && *payment.ChangeCurrency == currencyUSD && payment.ChangeUSD != nil {
		w.row("   Cambio en dólares", dollars(*payment.ChangeUSD))
		w.row("   Equivale a", pesos(payment.Change))
	} else {
		w.row("   Cambio en pesos", pesos(payment.Change))
	}
}

func valueOf(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return *pointer
}

// renderRegisteredPayments lista los pagos de la cuenta con lo pagado y las propinas.
func renderRegisteredPayments(block Block, data *CommercialData, w *writer) {
	if len(data.Payments) == 0 {
		return
	}
	options := sectionOptions(block)
	if options.Divider {
		w.rule("-")
	}
	if options.Heading {
		w.text("PAGOS REGISTRADOS")
	}
	for _, payment := range data.Payments {
		w.row(payment.Method, pesos(payment.Amount))
	}
	w.row("Pagado", pesos(data.Paid))
	if data.Tips != "0.00" {
		w.row("Propinas", pesos(data.Tips))
	}
}
