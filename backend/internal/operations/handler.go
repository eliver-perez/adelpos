package operations

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

// retiredVoidMessage explica a cualquier cliente que la anulación anterior ya no existe y qué usar.
const retiredVoidMessage = "Esta forma de cancelar cuentas ya no existe. Usa «Cancelar cuenta», con su motivo y, si hace falta, " +
	"la autorización de un supervisor, o «Descartar cuenta vacía» si la cuenta no tiene productos enviados."

// retiredRoute reconoce las rutas retiradas. Etapa 9F (§7.1, decisión D8 del coordinador): la
// anulación anterior con motivo libre (POST restaurant/accounts/{id}/void, etapas 4–9C) ya no tenía
// cliente en la aplicación y permitía anular sin motivo del catálogo ni autorización; redirigirla a
// «Cancelar cuenta» casi nunca procedería (solo aceptaba cuentas sin productos activos, que «Cancelar
// cuenta» rechaza como descartables), así que se retira.
func retiredRoute(method string, parts []string) bool {
	return method == "POST" && len(parts) == 3 && parts[0] == "accounts" && parts[2] == "void"
}

func Handle(ctx context.Context, tx pgx.Tx, request Request, audit Audit) (any, error) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(request.Route, "restaurant/"), "/"), "/")
	// Una ruta retirada responde 410 antes de comprobar permisos, tomar el lock o leer la cuenta: no
	// escribe nada (tampoco una denegación en la auditoría) y la transacción se revierte.
	if retiredRoute(request.Method, parts) {
		return nil, &Error{Status: 410, Code: "ROUTE_RETIRED", Message: retiredVoidMessage}
	}
	permission := routePermission(request.Method, parts)
	if permission == "" {
		return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "La ruta operativa no existe."}
	}
	if err := request.Actor.Require(permission); err != nil {
		return nil, err
	}
	if len(parts) > 1 && parts[0] != "accounts" && parts[0] != "layout" {
		if err := validateID(parts[1]); err != nil {
			return nil, err
		}
	}
	if parts[0] == "accounts" && len(parts) > 1 {
		if err := validateID(parts[1]); err != nil {
			return nil, err
		}
	}
	previewRoute := len(parts) == 3 && (parts[2] == "edit-preview" || parts[2] == "draft-preview")
	readOnly := request.Method == "GET" || previewRoute
	lock := "SELECT pg_advisory_xact_lock(7820148305)"
	if readOnly {
		lock = "SELECT pg_advisory_xact_lock_shared(7820148305)"
	}
	if _, err := tx.Exec(ctx, lock); err != nil {
		return nil, err
	}
	handler := service{ctx: ctx, tx: tx, request: request, audit: audit}
	if err := handler.authorizeRoute(parts); err != nil {
		return nil, err
	}
	if readOnly {
		return handler.dispatch(parts)
	}
	var mutation Mutation
	if err := json.Unmarshal(request.Body, &mutation); err != nil {
		return nil, invalid("La operación requiere un cuerpo JSON válido.")
	}
	if err := validateID(mutation.OperationID); err != nil {
		return nil, err
	}
	handler.operationID = mutation.OperationID
	fingerprint, err := requestFingerprint(request)
	if err != nil {
		return nil, err
	}
	cached, found, err := handler.replay(fingerprint)
	if err != nil || found {
		return cached, err
	}
	result, err := handler.dispatch(parts)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        VALUES ($1,$2,$3,$4,$5,$6)`, mutation.OperationID, request.Actor.UserID, request.Actor.DeviceID,
		request.Method+" "+request.Route, fingerprint, jsonValue(result))
	if err != nil {
		return nil, err
	}
	// El pedido capturado sin conexión (etapa 9G) publica los eventos de su cuenta, como la confirmación.
	if parts[0] != "accounts" && parts[0] != "offline-orders" {
		if err := events.Append(ctx, tx, "restaurant."+parts[0]+".changed", "restaurant", nil, nil); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (handler *service) authorizeRoute(parts []string) error {
	// Etapa 9G (§5.5): el pedido capturado sin conexión guarda y confirma en una sola operación, así que exige lo
	// mismo que capturar y confirmar; abrir la cuenta de una mesa exige además accounts.open (offline_orders.go).
	if parts[0] == "offline-orders" {
		return handler.request.Actor.Require("accounts.read", "orders.capture", "catalog.read", "catalog.prices.read")
	}
	if parts[0] != "accounts" {
		return nil
	}
	if err := handler.request.Actor.Require("accounts.read"); err != nil {
		return err
	}
	if len(parts) == 1 {
		if handler.request.Method == "POST" {
			var input struct {
				Mode  string            `json:"mode"`
				Items []json.RawMessage `json:"items"`
			}
			if err := json.Unmarshal(handler.request.Body, &input); err != nil {
				return invalid("La cuenta requiere un cuerpo JSON válido.")
			}
			if input.Mode == "takeaway" {
				if err := handler.request.Actor.Require("takeaway.manage"); err != nil {
					return err
				}
			}
			// Etapa 7.1: abrir la cuenta con su primer producto guarda además el borrador, así que exige
			// los mismos permisos que PUT …/draft antes de cualquier escritura.
			if len(input.Items) > 0 {
				return handler.request.Actor.Require("orders.capture", "catalog.read", "catalog.prices.read")
			}
		}
		return nil
	}
	account, err := handler.readAccount(parts[1])
	if err != nil {
		return err
	}
	previewRoute := len(parts) == 3 && (parts[2] == "edit-preview" || parts[2] == "draft-preview")
	writing := handler.request.Method != "GET" && !previewRoute
	if err := handler.authorizeAccount(account, writing); err != nil {
		return err
	}
	if len(parts) == 3 {
		switch parts[2] {
		case "draft", "preview", "draft-preview", "confirm", "edit-preview", "edit":
			return handler.request.Actor.Require("catalog.read", "catalog.prices.read")
		case "cancel":
			return handler.requireUnitsCancelRoute()
		}
	}
	return nil
}

// requireUnitsCancelRoute es el permiso de la ruta de «Cancelar unidades» (etapa 9F, §7.2): quien
// tiene orders.cancel cancela con su permiso, como antes; sin él, basta poder abrir cuentas
// (accounts.open, el mismo umbral de «Cancelar cuenta») para pedir la autorización de un supervisor,
// que la ruta decide antes de escribir (cancellations.go).
func (handler *service) requireUnitsCancelRoute() error {
	if handler.request.Actor.Can(unitsCancelPermission) {
		return nil
	}
	return handler.request.Actor.Require("accounts.open")
}

func routePermission(method string, parts []string) string {
	resource := parts[0]
	if len(parts) == 1 {
		switch resource {
		case "overview":
			if method == "GET" {
				return "restaurant.tables.read"
			}
		case "areas":
			if method == "GET" {
				return "restaurant.areas.read"
			}
			if method == "POST" {
				return "restaurant.areas.manage"
			}
		case "tables":
			if method == "GET" {
				return "restaurant.tables.read"
			}
			if method == "POST" {
				return "restaurant.tables.manage"
			}
		case "layout":
			if method == "PUT" {
				return "restaurant.tables.manage"
			}
		case "settings":
			if method == "GET" {
				return "accounts.read"
			}
			if method == "PUT" {
				return "restaurant.settings.manage"
			}
		case "staff":
			if method == "GET" {
				return "accounts.transfer"
			}
		// Etapa 9D: el catálogo de motivos de cancelación lo consulta quien cancela cuentas y lo
		// administra el permiso de políticas de cobro, el mismo de los motivos de descuento.
		case "cancel-reasons":
			if method == "GET" {
				return "accounts.read"
			}
			if method == "POST" {
				return cancelReasonsPermission
			}
		case "accounts":
			if method == "GET" {
				return "accounts.read"
			}
			if method == "POST" {
				return "accounts.open"
			}
		// Etapa 9G (§5.5): pedido capturado sin conexión por la futura aplicación Android.
		case "offline-orders":
			if method == "POST" {
				return "orders.confirm"
			}
		}
	}
	// Etapa 7.1: poner fuera de servicio o habilitar una mesa desde la operación.
	if resource == "tables" && len(parts) == 3 && parts[2] == "service" && method == "POST" {
		return "restaurant.tables.manage"
	}
	if len(parts) == 2 && method == "PATCH" {
		return map[string]string{
			"areas":          "restaurant.areas.manage",
			"tables":         "restaurant.tables.manage",
			"accounts":       "accounts.manage",
			"cancel-reasons": cancelReasonsPermission,
		}[resource]
	}
	if resource == "accounts" && len(parts) == 2 && method == "GET" {
		return "accounts.read"
	}
	if resource == "accounts" && len(parts) == 3 {
		action := parts[2]
		if method == "GET" {
			return map[string]string{"preview": "accounts.read", "history": "orders.history.read"}[action]
		}
		if method == "PUT" && action == "draft" {
			return "orders.capture"
		}
		if method == "POST" {
			return map[string]string{
				"draft-preview": "orders.capture",
				"confirm":       "orders.confirm", "edit-preview": "orders.edit", "edit": "orders.edit",
				// Etapa 9F: «Cancelar unidades» ya no exige orders.cancel en la ruta (sin él procede con
				// supervisor); authorizeRoute pide además orders.cancel o accounts.open.
				"cancel": "accounts.read", "split": "accounts.split", "transfer": "accounts.transfer_items",
				"merge": "accounts.merge", "move": "accounts.move", "assign": "accounts.transfer",
				"release": "accounts.open",
				// Etapa 9D: descartar una cuenta vacía (sin motivo) y cancelar una cuenta con motivo. La
				// cancelación no puede exigir accounts.cancel aquí: sin él procede con la autorización de un
				// supervisor, que la ruta decide antes de escribir (account_cancellation.go).
				"discard": "accounts.open", "cancel-account": "accounts.open",
			}[action]
		}
	}
	return ""
}

// requestFingerprint identifica el contenido de la operación. Como en los cobros, excluye la
// contraseña de una autorización de supervisor (etapa 9D): nunca se conserva un derivado del secreto
// y un reintento puede enviarse sin ella para recuperar el resultado ya confirmado.
func requestFingerprint(request Request) (string, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(request.Body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", invalid("La operación no contiene JSON válido.")
	}
	withoutAuthorizationPasswords(value)
	digest := sha256.Sum256([]byte(request.Method + " " + request.Route + "\n" + string(jsonValue(value))))
	return fmt.Sprintf("%x", digest), nil
}

// withoutAuthorizationPasswords quita la contraseña de cada objeto de autorización del cuerpo, en
// cualquier nivel. decode (encoding/json) acepta las claves sin distinguir mayúsculas («Authorization»,
// «PASSWORD»…), así que aquí también se comparan sin distinguirlas (strings.EqualFold sigue el mismo
// plegado de Unicode); si no, un cliente que escribiera «Password» dejaría en la huella un derivado de la
// contraseña (revisión independiente de la 9F, B3).
func withoutAuthorizationPasswords(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if authorization, ok := child.(map[string]any); ok && strings.EqualFold(key, "authorization") {
				for field := range authorization {
					if strings.EqualFold(field, "password") {
						delete(authorization, field)
					}
				}
			}
			withoutAuthorizationPasswords(child)
		}
	case []any:
		for _, child := range typed {
			withoutAuthorizationPasswords(child)
		}
	}
}

func (handler *service) replay(fingerprint string) (any, bool, error) {
	var actorID, deviceID, previousFingerprint string
	var result json.RawMessage
	err := handler.tx.QueryRow(handler.ctx, `SELECT actor_id,device_id,fingerprint,response
        FROM app.restaurant_operations WHERE id=$1`, handler.operationID).Scan(&actorID, &deviceID, &previousFingerprint, &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if actorID != handler.request.Actor.UserID || deviceID != handler.request.Actor.DeviceID || previousFingerprint != fingerprint {
		return nil, false, &Error{
			Status:  409,
			Code:    "IDEMPOTENCY_CONFLICT",
			Message: "Esta operación ya se había enviado con otros datos y no se repitió. Consulta el resultado antes de intentarlo de nuevo.",
		}
	}
	return result, true, nil
}

func (handler *service) dispatch(parts []string) (any, error) {
	switch parts[0] {
	case "overview":
		return handler.overview()
	case "areas":
		return handler.areas(parts)
	case "tables":
		return handler.tables(parts)
	case "layout":
		return handler.layout()
	case "settings":
		return handler.settings()
	case "staff":
		return readList[map[string]any](handler.ctx, handler.tx, `SELECT jsonb_build_object('id',id,'name',display_name)
            FROM app.users WHERE active AND NOT support ORDER BY display_name,id`)
	case "accounts":
		return handler.accounts(parts)
	case "cancel-reasons":
		return handler.cancelReasons(parts)
	case "offline-orders":
		return handler.offlineOrder()
	}
	return nil, invalid("Operación no implementada.")
}
