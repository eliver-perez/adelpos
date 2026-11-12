package kitchen

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/events"
)

type Permission struct {
	Key         string
	Description string
}

var Permissions = []Permission{
	{"kitchen.stations.read", "Consultar configuración de estaciones"},
	{"kitchen.stations.manage", "Administrar estaciones y ámbitos de terminal"},
	{"kitchen.stations.all", "Operar todas las estaciones del establecimiento"},
	{"kitchen.routing.manage", "Administrar rutas de preparación"},
	{"kitchen.printers.manage", "Administrar impresoras y destinos"},
	{"kitchen.printers.test", "Solicitar impresión de prueba"},
	{"kitchen.formats.manage", "Administrar formato y agrupación"},
	{"kitchen.timing.manage", "Administrar tiempos y programación"},
	{"kitchen.release", "Liberar pedidos a preparación"},
	{"kitchen.release_early", "Liberar anticipadamente pedidos programados"},
	{"kitchen.tickets.read", "Consultar comandas por estación"},
	{"kitchen.states.read", "Consultar preparación"},
	{"kitchen.states.manage", "Actualizar preparación"},
	{"kitchen.queue.read", "Consultar trabajos e intentos de impresión"},
	{"kitchen.queue.resolve", "Resolver errores y resultados inciertos"},
	{"kitchen.queue.retry", "Reintentar errores confirmados"},
	{"kitchen.reprint", "Solicitar reimpresión"},
	{"kitchen.history.read", "Consultar historial de cocina"},
	{"kitchen.kds.ack", "Confirmar recepción KDS"},
	{"events.read", "Recibir eventos autenticados"},
}

func Handle(ctx context.Context, tx pgx.Tx, request Request, audit Audit) (any, error) {
	parts := strings.Split(strings.TrimPrefix(request.Route, "kitchen/"), "/")
	permission := routePermission(request.Method, parts)
	if permission == "" {
		return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "La ruta de cocina no existe."}
	}
	if request.Method == "GET" && parts[0] == "settings" {
		if request.Actor.Can("kitchen.formats.manage") {
			permission = "kitchen.formats.manage"
		} else if request.Actor.Can("kitchen.timing.manage") {
			permission = "kitchen.timing.manage"
		}
	}
	if err := request.Actor.Require(permission); err != nil {
		return nil, err
	}
	// kitchen/print-jobs/rerender (etapa 9B) es una acción con nombre fijo, sin identificador.
	if len(parts) > 1 && !isRerenderRoute(parts) {
		if err := validID(parts[1]); err != nil {
			return nil, err
		}
	}
	handler := service{ctx: ctx, tx: tx, request: request, audit: audit}
	if request.Method == "GET" {
		return handler.read(parts)
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7820148305)"); err != nil {
		return nil, err
	}
	var mutation struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(request.Body, &mutation); err != nil {
		return nil, invalid("La operación requiere JSON válido.")
	}
	if err := validID(mutation.OperationID); err != nil {
		return nil, err
	}
	handler.operationID = mutation.OperationID
	var canonical any
	decoder := json.NewDecoder(strings.NewReader(string(request.Body)))
	decoder.UseNumber()
	if err := decoder.Decode(&canonical); err != nil {
		return nil, invalid("La operación requiere JSON válido.")
	}
	digest := sha256.Sum256([]byte(request.Method + " " + request.Route + "\n" + string(encoded(canonical))))
	fingerprint := fmt.Sprintf("%x", digest)
	var actorID, deviceID, previousFingerprint string
	var response json.RawMessage
	err := tx.QueryRow(ctx, `SELECT actor_id,device_id,fingerprint,response FROM app.restaurant_operations
        WHERE id=$1`, mutation.OperationID).Scan(&actorID, &deviceID, &previousFingerprint, &response)
	if err == nil {
		if actorID != request.Actor.UserID || deviceID != request.Actor.DeviceID || previousFingerprint != fingerprint {
			return nil, &Error{Status: 409, Code: "IDEMPOTENCY_CONFLICT", Message: "Ese identificador corresponde a otra operación."}
		}
		// Revalidar también ámbitos antes de devolver información de un resultado previo.
		if err := handler.authorizeResource(parts); err != nil {
			return nil, err
		}
		return response, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	result, err := handler.write(parts)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        VALUES ($1,$2,$3,$4,$5,$6)`, mutation.OperationID, request.Actor.UserID, request.Actor.DeviceID,
		request.Method+" "+request.Route, fingerprint, encoded(result))
	if err != nil {
		return nil, err
	}
	scope := "configuration"
	if parts[0] == "jobs" || parts[0] == "reprint" || parts[0] == "test-print" || parts[0] == "print-jobs" || isWidthTestRoute(parts) {
		scope = "printing"
	}
	if parts[0] == "states" || parts[0] == "releases" {
		scope = "kitchen"
	}
	if err := events.Append(ctx, tx, "kitchen."+parts[0]+".changed", scope, nil, nil); err != nil {
		return nil, err
	}
	return result, nil
}

// isWidthTestRoute identifica kitchen/printers/{id}/width-test (etapa 9C).
func isWidthTestRoute(parts []string) bool {
	return len(parts) == 3 && parts[0] == "printers" && parts[2] == "width-test"
}

// isRerenderRoute identifica kitchen/print-jobs/rerender.
func isRerenderRoute(parts []string) bool {
	return len(parts) == 2 && parts[0] == "print-jobs" && parts[1] == "rerender"
}

func routePermission(method string, parts []string) string {
	if len(parts) > 3 {
		return ""
	}
	resource := parts[0]
	if method == "GET" {
		if len(parts) > 1 && !(len(parts) == 2 && (resource == "tickets" || resource == "jobs" || resource == "releases")) {
			return ""
		}
		return map[string]string{
			"stations": "kitchen.stations.read", "printers": "kitchen.printers.manage",
			"discovery": "kitchen.printers.manage", "routes": "kitchen.routing.manage", "targets": "kitchen.routing.manage",
			"settings": "kitchen.stations.read", "board": "kitchen.states.read",
			"releases": "kitchen.states.read", "tickets": "kitchen.tickets.read",
			"jobs": "kitchen.queue.read", "history": "kitchen.history.read",
			"access": "kitchen.stations.manage", "outputs": "kitchen.stations.manage",
		}[resource]
	}
	if len(parts) == 1 {
		if (resource == "settings" || resource == "access" || resource == "timing") && method != "PUT" {
			return ""
		}
		if resource != "settings" && resource != "access" && resource != "timing" && method != "POST" {
			return ""
		}
		return map[string]string{
			"stations": "kitchen.stations.manage", "printers": "kitchen.printers.manage",
			"routes": "kitchen.routing.manage", "settings": "kitchen.formats.manage",
			"states": "kitchen.states.manage", "access": "kitchen.stations.manage", "timing": "kitchen.timing.manage",
			"test-print": "kitchen.printers.test", "reprint": "kitchen.reprint",
		}[resource]
	}
	if isRerenderRoute(parts) && method == "POST" {
		return "kitchen.reprint"
	}
	if len(parts) == 2 && method == "PATCH" {
		return map[string]string{"stations": "kitchen.stations.manage", "printers": "kitchen.printers.manage", "routes": "kitchen.routing.manage"}[resource]
	}
	if len(parts) == 3 && method == "POST" {
		if resource == "changes" && parts[2] == "retry" {
			return "kitchen.release"
		}
		if resource == "releases" && parts[2] == "release" {
			return "kitchen.release"
		}
		if resource == "releases" && parts[2] == "schedule" {
			return "kitchen.timing.manage"
		}
		if resource == "jobs" && parts[2] == "retry" {
			return "kitchen.queue.retry"
		}
		if resource == "jobs" && parts[2] == "resolve" {
			return "kitchen.queue.resolve"
		}
		if resource == "tickets" && parts[2] == "ack" {
			return "kitchen.kds.ack"
		}
		// Etapa 9C (§5.3): la prueba de ancho es parte de configurar la impresora.
		if resource == "printers" && parts[2] == "width-test" {
			return "kitchen.printers.manage"
		}
	}
	return ""
}

func (handler *service) write(parts []string) (any, error) {
	switch parts[0] {
	case "stations":
		return handler.saveStation(parts)
	case "printers":
		if isWidthTestRoute(parts) {
			return handler.widthTest(parts[1])
		}
		return handler.savePrinter(parts)
	case "routes":
		return handler.saveRoute(parts)
	case "settings":
		return handler.saveSettings()
	case "timing":
		return handler.saveTiming()
	case "access":
		return handler.saveAccess()
	case "changes":
		return handler.retryChange(parts[1])
	case "releases":
		return handler.releaseAction(parts[1], parts[2])
	case "states":
		return handler.saveStates()
	case "jobs":
		return handler.jobAction(parts[1], parts[2])
	case "test-print":
		return handler.testPrint()
	case "reprint":
		return handler.reprint()
	case "print-jobs":
		return handler.rerender()
	case "tickets":
		return handler.ackTicket(parts[1])
	}
	return nil, invalid("La operación de cocina no existe.")
}
