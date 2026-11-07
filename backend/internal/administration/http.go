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
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, 64*1024))
	if err != nil {
		writeFault(writer, request, &Fault{Status: 413, Code: "REQUEST_TOO_LARGE", Message: "La solicitud excede el tamaño permitido."})
		return
	}
	if len(body) > 0 && !json.Valid(body) {
		writeFault(writer, request, &Fault{Status: 400, Code: "INVALID_JSON", Message: "La solicitud no contiene JSON válido."})
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
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
	if err != nil {
		writeFault(writer, request, publicFault(err))
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
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": map[string]string{
		"code":       fault.Code,
		"message":    fault.Message,
		"request_id": writer.Header().Get("X-Request-ID"),
	}})
}
