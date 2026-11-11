package identity

// Error es un rechazo con su código público y un mensaje en español que la interfaz muestra tal
// cual. La administración lo traduce a su Fault (un 403 revierte la operación con su savepoint).
type Error struct {
	Status  int
	Code    string
	Message string
}

func (problem *Error) Error() string {
	return problem.Message
}

// invalid es un dato rechazado por las reglas de la identidad o del logotipo.
func invalid(message string) error {
	return &Error{Status: 422, Code: "VALIDATION_ERROR", Message: message}
}

func denied() error {
	return &Error{Status: 403, Code: "PERMISSION_DENIED", Message: "No tienes permiso para esta operación."}
}

func conflict(code, message string) error {
	return &Error{Status: 409, Code: code, Message: message}
}

func notFound(message string) error {
	return &Error{Status: 404, Code: "NOT_FOUND", Message: message}
}

// revisionConflict avisa que otra persona guardó antes; la interfaz conserva lo escrito.
func revisionConflict() error {
	return conflict(
		"REVISION_CONFLICT",
		"Otra persona guardó la identidad del establecimiento. Tus cambios siguen en pantalla: revisa la versión guardada antes de volver a guardar.",
	)
}

// noLogo responde a retirar o previsualizar sin logotipo vigente.
func noLogo() error {
	return conflict("NO_LOGO", "No hay un logotipo cargado.")
}
