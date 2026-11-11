package reports

import "net/http"

// Error es un rechazo público del módulo de reportes; administration lo traduce a su Fault. El
// mensaje está en español y nunca contiene importes protegidos.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (problem *Error) Error() string {
	return problem.Code
}

func invalid(message string) error {
	return &Error{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: message}
}

func denied() error {
	return &Error{Status: http.StatusForbidden, Code: "PERMISSION_DENIED", Message: "No tienes permiso para consultar este reporte."}
}

func notFound(message string) error {
	return &Error{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: message}
}

// busy responde cuando las conexiones reservadas para reportes están ocupadas: el reporte no espera
// (no compite con cobros, captura ni cocina) y la terminal puede reintentar.
func busy() error {
	return &Error{
		Status:  http.StatusServiceUnavailable,
		Code:    "REPORTS_BUSY",
		Message: "Hay otros reportes en curso. Espera unos segundos y vuelve a intentarlo.",
	}
}

// tooLarge responde cuando el conjunto filtrado excede el máximo que se procesa en memoria.
func tooLarge() error {
	return &Error{
		Status:  http.StatusUnprocessableEntity,
		Code:    "REPORT_TOO_LARGE",
		Message: "El reporte tiene demasiados renglones. Reduce el periodo o agrega filtros.",
	}
}

// timedOut responde cuando la consulta excede su límite de tiempo (statement_timeout).
func timedOut() error {
	return &Error{
		Status:  http.StatusServiceUnavailable,
		Code:    "REPORT_TIMEOUT",
		Message: "El reporte tardó demasiado. Reduce el periodo o agrega filtros y vuelve a intentarlo.",
	}
}
