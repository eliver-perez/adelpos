package payments

// extendedRoute agrega las acciones sobre una cuenta que no pertenecen al cobro básico.
func extendedRoute(method, action string) *route {
	if method != "POST" {
		return nil
	}
	switch action {
	case "deliver":
		return &route{"takeaway.deliver", false, (*service).deliver}
	}
	return nil
}
