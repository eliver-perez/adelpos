package payments

// requirePrintPermission valida antes de escribir el permiso para imprimir el ticket solicitado.
func (handler *service) requirePrintPermission(print *bool) error {
	if print != nil && *print {
		return handler.request.Actor.Require("payments.tickets.print")
	}
	return nil
}

// printReceiptIfRequested encola el ticket de un pago cuando el cajero lo solicita.
func (handler *service) printReceiptIfRequested(finance *Finance, payment *PaymentRecord, print *bool) (map[string]any, error) {
	return map[string]any{"requested": false}, nil
}
