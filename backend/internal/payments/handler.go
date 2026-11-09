package payments

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// route describe una ruta financiera: permiso base, si solo lee y qué acción la atiende.
type route struct {
	permission string
	readOnly   bool
	action     func(handler *service, parts []string) (any, error)
}

func resolve(method string, parts []string) *route {
	if selected := cashRoute(method, parts); selected != nil {
		return selected
	}
	length := len(parts)
	switch {
	case length == 1 && parts[0] == "settings" && method == "GET":
		return &route{"payments.read", true, (*service).readSettings}
	case length == 1 && parts[0] == "settings" && method == "PUT":
		return &route{"payments.settings.manage", false, (*service).saveSettings}
	case length == 2 && parts[0] == "settings" && parts[1] == "usd" && method == "PUT":
		return &route{"payments.settings.manage", false, (*service).saveUSDSettings}
	case length == 1 && parts[0] == "tax" && method == "PUT":
		return &route{"payments.tax.manage", false, (*service).saveTaxRate}
	case length == 1 && parts[0] == "methods" && method == "GET":
		return &route{"payments.read", true, (*service).listMethods}
	case length == 1 && parts[0] == "methods" && method == "POST":
		return &route{"payments.methods.manage", false, (*service).createMethod}
	case length == 2 && parts[0] == "methods" && method == "PATCH":
		return &route{"payments.methods.manage", false, (*service).updateMethod}
	case length == 2 && parts[0] == "shifts" && parts[1] == "current" && method == "GET":
		return &route{"payments.read", true, (*service).currentShift}
	case length == 1 && parts[0] == "shifts" && method == "POST":
		return &route{"cash.shifts.open", false, (*service).openShift}
	case length == 3 && parts[0] == "shifts" && parts[2] == "movements" && method == "GET":
		return &route{"cash.shifts.read", true, (*service).shiftMovements}
	case length == 3 && parts[0] == "shifts" && parts[2] == "usd-rate" && method == "POST":
		return &route{"payments.usd_rate.manage", false, (*service).setShiftRate}
	}
	if length < 2 || parts[0] != "accounts" {
		return nil
	}
	// El puente nativo solo admite un identificador por ruta: los registros anidados (ajuste,
	// documento o pago) viajan en el cuerpo y se validan allí.
	action := strings.Join(parts[2:], "/")
	switch {
	case action == "" && method == "GET":
		return &route{"payments.read", true, (*service).readAccountFinance}
	case action == "preview" && method == "POST":
		return &route{"payments.read", true, (*service).previewPayment}
	case action == "payments" && method == "POST":
		return &route{"payments.collect", false, (*service).registerPayment}
	}
	return extendedRoute(method, action)
}

// Handle atiende las rutas financieras dentro de la transacción autenticada de administración.
// Toda escritura toma primero el lock operativo 7820148305, el mismo de transferencias y
// combinaciones, porque comparte el agregado de la cuenta; las lecturas lo toman compartido. Las
// rutas de caja (etapa 7) usan el mismo lock: un cobro y el cierre de su turno nunca se intercalan.
func Handle(ctx context.Context, tx pgx.Tx, request Request, audit Audit) (any, error) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(request.Route, "payments/"), "/"), "/")
	selected := resolve(request.Method, parts)
	if selected == nil {
		return nil, notFound("La ruta financiera no existe.")
	}
	if err := request.Actor.Require(selected.permission); err != nil {
		return nil, err
	}
	if len(parts) > 1 && (parts[0] == "accounts" || parts[0] == "methods" || (parts[0] == "shifts" && parts[1] != "current")) {
		if err := validateID(parts[1]); err != nil {
			return nil, err
		}
	}
	if identifier, found := cashIdentifier(parts); found {
		if err := validateID(identifier); err != nil {
			return nil, err
		}
	}
	lock := "SELECT pg_advisory_xact_lock(7820148305)"
	if selected.readOnly {
		lock = "SELECT pg_advisory_xact_lock_shared(7820148305)"
	}
	if _, err := tx.Exec(ctx, lock); err != nil {
		return nil, err
	}
	handler := &service{ctx: ctx, tx: tx, request: request, audit: audit}
	if parts[0] == "accounts" {
		if err := handler.authorizeAccount(parts[1]); err != nil {
			return nil, err
		}
	}
	if selected.readOnly {
		return selected.action(handler, parts)
	}
	var mutation struct {
		OperationID string `json:"operation_id"`
	}
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
	result, err := selected.action(handler, parts)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        VALUES ($1,$2,$3,$4,$5,$6)`, mutation.OperationID, request.Actor.UserID, request.Actor.DeviceID,
		request.Method+" "+request.Route, fingerprint, jsonValue(result))
	if err != nil {
		return nil, err
	}
	return result, nil
}

// authorizeAccount exige consultar cuentas y, para pedidos para llevar, su permiso propio.
func (handler *service) authorizeAccount(accountID string) error {
	if err := handler.request.Actor.Require("accounts.read"); err != nil {
		return err
	}
	record, err := readAccountRecord(handler.ctx, handler.tx, accountID, false)
	if err != nil {
		return err
	}
	if record.Mode == "takeaway" {
		return handler.request.Actor.Require("takeaway.read")
	}
	return nil
}

// requestFingerprint identifica el contenido de la operación sin incluir la contraseña de una
// autorización de supervisor: nunca se conserva un derivado del secreto y un reintento puede
// enviarse sin ella para recuperar el resultado ya confirmado.
func requestFingerprint(request Request) (string, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(request.Body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", invalid("La operación no contiene JSON válido.")
	}
	if fields, ok := value.(map[string]any); ok {
		if authorization, ok := fields["authorization"].(map[string]any); ok {
			delete(authorization, "password")
		}
	}
	digest := sha256.Sum256([]byte(request.Method + " " + request.Route + "\n" + string(jsonValue(value))))
	return fmt.Sprintf("%x", digest), nil
}

func (handler *service) replay(fingerprint string) (any, bool, error) {
	var actorID, deviceID, previousFingerprint string
	var result json.RawMessage
	err := handler.tx.QueryRow(handler.ctx, `SELECT actor_id,device_id,fingerprint,response
        FROM app.restaurant_operations WHERE id=$1`, handler.operationID).Scan(&actorID, &deviceID, &previousFingerprint, &result)
	if isNoRows(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if actorID != handler.request.Actor.UserID || deviceID != handler.request.Actor.DeviceID || previousFingerprint != fingerprint {
		return nil, false, conflict("IDEMPOTENCY_CONFLICT", "El identificador de operación ya corresponde a otra solicitud. No se repitieron sus cambios.")
	}
	return result, true, nil
}
