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

func Handle(ctx context.Context, tx pgx.Tx, request Request, audit Audit) (any, error) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(request.Route, "restaurant/"), "/"), "/")
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
	if parts[0] != "accounts" {
		if err := events.Append(ctx, tx, "restaurant."+parts[0]+".changed", "restaurant", nil, nil); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (handler *service) authorizeRoute(parts []string) error {
	if parts[0] != "accounts" {
		return nil
	}
	if err := handler.request.Actor.Require("accounts.read"); err != nil {
		return err
	}
	if len(parts) == 1 {
		if handler.request.Method == "POST" {
			var input struct {
				Mode string `json:"mode"`
			}
			if err := json.Unmarshal(handler.request.Body, &input); err != nil {
				return invalid("La cuenta requiere un cuerpo JSON válido.")
			}
			if input.Mode == "takeaway" {
				return handler.request.Actor.Require("takeaway.manage")
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
		}
	}
	return nil
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
		case "accounts":
			if method == "GET" {
				return "accounts.read"
			}
			if method == "POST" {
				return "accounts.open"
			}
		}
	}
	if len(parts) == 2 && method == "PATCH" {
		return map[string]string{
			"areas":    "restaurant.areas.manage",
			"tables":   "restaurant.tables.manage",
			"accounts": "accounts.manage",
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
				"cancel": "orders.cancel", "split": "accounts.split", "transfer": "accounts.transfer_items",
				"merge": "accounts.merge", "move": "accounts.move", "assign": "accounts.transfer",
				"void": "accounts.manage",
			}[action]
		}
	}
	return ""
}

func requestFingerprint(request Request) (string, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(request.Body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", invalid("La operación no contiene JSON válido.")
	}
	digest := sha256.Sum256([]byte(request.Method + " " + request.Route + "\n" + string(jsonValue(value))))
	return fmt.Sprintf("%x", digest), nil
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
			Message: "El identificador de operación ya corresponde a otra solicitud. No se repitieron sus cambios.",
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
	}
	return nil, invalid("Operación no implementada.")
}
