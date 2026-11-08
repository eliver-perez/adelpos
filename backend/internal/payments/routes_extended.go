package payments

// extendedRoute agrega las acciones sobre una cuenta que no pertenecen al cobro básico. Las
// previsualizaciones solo calculan y toman el lock compartido.
func extendedRoute(method, action string) *route {
	if method == "GET" && action == "precheck" {
		return &route{"payments.read", true, (*service).precheck}
	}
	if method != "POST" {
		return nil
	}
	switch action {
	case "precheck/print":
		return &route{"payments.tickets.print", false, (*service).printPrecheck}
	case "documents/reprint":
		return &route{"payments.tickets.reprint", false, (*service).reprintDocument}
	case "deliver":
		return &route{"takeaway.deliver", false, (*service).deliver}
	case "discounts/preview":
		return &route{"payments.read", true, (*service).previewDiscount}
	case "discounts":
		// El permiso efectivo (aplicar, cortesía o autorización) se resuelve dentro de la operación.
		return &route{"payments.read", false, (*service).applyDiscount}
	case "charges":
		return &route{"payments.charges.create", false, (*service).addCharge}
	case "adjustments/void":
		return &route{"payments.read", false, (*service).voidAdjustment}
	case "refunds/preview":
		return &route{"payments.read", true, (*service).previewRefund}
	case "refunds":
		return &route{"payments.read", false, (*service).refund}
	case "corrections":
		return &route{"payments.read", false, (*service).correct}
	}
	return nil
}
