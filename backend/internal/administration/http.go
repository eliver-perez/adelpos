package administration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (service *Service) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	host, _, _ := net.SplitHostPort(request.RemoteAddr)
	address := net.ParseIP(host)
	local := address != nil && address.IsLoopback()
	if !local && request.TLS == nil {
		writeFault(writer, request, &Fault{Status: 403, Code: "TLS_REQUIRED", Message: "Las conexiones de red requieren TLS verificado."})
		return
	}
	if request.URL.Path == "/api/v1/events" && request.Method == "GET" {
		service.serveEvents(writer, request, local)
		return
	}
	limit := int64(64 * 1024)
	if strings.HasPrefix(request.URL.Path, "/api/v1/catalog/") || strings.HasPrefix(request.URL.Path, "/api/v1/restaurant/") || strings.HasPrefix(request.URL.Path, "/api/v1/kitchen/") {
		limit = 2 * 1024 * 1024
	}
	// La existencia inicial admite hasta 500 renglones por operación (etapa 8).
	if strings.HasPrefix(request.URL.Path, "/api/v1/inventory/") {
		limit = 2 * 1024 * 1024
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, limit))
	if err != nil {
		writeFault(writer, request, &Fault{Status: 413, Code: "REQUEST_TOO_LARGE", Message: "La solicitud excede el tamaño permitido."})
		return
	}
	if len(body) > 0 && !json.Valid(body) {
		writeFault(writer, request, &Fault{Status: 400, Code: "INVALID_JSON", Message: "La solicitud no contiene JSON válido."})
		return
	}
	// Los reportes (etapa 9A) tienen su propio tiempo de respuesta: la consulta puede durar hasta el
	// statement_timeout de su transacción de solo lectura.
	timeout := 10 * time.Second
	if strings.HasPrefix(request.URL.Path, "/api/v1/reports/") {
		timeout = reportTimeout
	}
	// Las exportaciones (fase C1) además generan el archivo con todos los renglones del reporte.
	if strings.HasPrefix(request.URL.Path, "/api/v1/reports/export/") {
		timeout = exportTimeout
	}
	// El servidor HTTP corta toda respuesta a los 15 s (WriteTimeout): una ruta con más tiempo amplía
	// su propio plazo de escritura. Sin soporte (pruebas con httptest) no hace falta.
	if timeout > 10*time.Second {
		_ = http.NewResponseController(writer).SetWriteDeadline(time.Now().Add(timeout + 5*time.Second))
	}
	ctx, cancel := context.WithTimeout(request.Context(), timeout)
	defer cancel()
	result, err := service.Execute(ctx, Request{
		Method:            request.Method,
		Path:              request.URL.Path,
		Query:             request.URL.Query(),
		Body:              body,
		DeviceID:          request.Header.Get("X-Device-ID"),
		DeviceToken:       request.Header.Get("X-Device-Token"),
		SessionToken:      strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "),
		ProvisioningToken: request.Header.Get("X-Provisioning-Token"),
		Local:             local,
	})
	if err == nil {
		result, err = service.completeDeferred(ctx, result)
	}
	if err != nil {
		writeFault(writer, request, publicFault(err))
		return
	}
	// Un reporte exportado se envía como archivo binario, no como JSON.
	if writeExport(writer, result) {
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(result)
}

func publicFault(err error) *Fault {
	var fault *Fault
	if errors.As(err, &fault) {
		return fault
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return &Fault{Status: 404, Code: "NOT_FOUND", Message: "El registro no existe."}
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		if databaseError.Code == "23505" {
			return &Fault{Status: 409, Code: "ALREADY_EXISTS", Message: "Ya existe un registro con ese nombre o identificador."}
		}
		if databaseError.Code == "23503" || databaseError.Code == "23514" || databaseError.Code == "22P02" {
			return &Fault{Status: 400, Code: "VALIDATION_ERROR", Message: "Los datos o sus relaciones no son válidos."}
		}
	}
	return &Fault{Status: 503, Code: "SERVICE_UNAVAILABLE", Message: "No se pudo completar la operación. Comprueba la disponibilidad del servidor."}
}

func writeFault(writer http.ResponseWriter, request *http.Request, fault *Fault) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(fault.Status)
	body := map[string]any{
		"code":       fault.Code,
		"message":    fault.Message,
		"request_id": writer.Header().Get("X-Request-ID"),
	}
	// Los detalles solo existen en rechazos que la terminal resuelve con datos públicos (etapa 7.1:
	// TABLE_HAS_ACCOUNT con las cuentas que otra terminal abrió en la mesa).
	if fault.Details != nil {
		body["details"] = fault.Details
	}
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": body})
}
