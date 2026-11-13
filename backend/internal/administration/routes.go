package administration

import (
	"context"
	"errors"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/designer"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (service *Service) protected(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string) (any, error) {
	if route == "events/snapshot" && request.Method == "GET" {
		return service.eventSnapshot(ctx, tx, request, principal)
	}
	if strings.HasPrefix(route, "kitchen/") {
		result, err := kitchen.Handle(ctx, tx, kitchen.Request{Method: request.Method, Route: route, Query: request.Query, Body: request.Body,
			Actor: kitchen.Actor{UserID: principal.UserID, DeviceID: principal.DeviceID, Permissions: principal.Permissions}},
			func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error {
				return audit(ctx, tx, principal, action, entity, id, before, after, true)
			})
		var problem *kitchen.Error
		if errors.As(err, &problem) {
			return nil, &Fault{Status: problem.Status, Code: problem.Code, Message: problem.Message, Details: problem.Details}
		}
		return result, err
	}
	if request.Method == "GET" && route == "auth/me" {
		return currentIdentity(ctx, tx, principal)
	}
	if request.Method == "POST" && route == "auth/logout" {
		if _, err := tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=now(),revoke_reason='logout' WHERE id=$1", principal.SessionID); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, audit(ctx, tx, principal, "session.logout", "session", &principal.SessionID, nil, nil, true)
	}
	if request.Method == "POST" && route == "auth/password" {
		return changePassword(ctx, tx, principal, request.Body)
	}
	if route == "preferences" {
		return preferences(ctx, tx, request, principal, principal.UserID, false)
	}
	if route == "terminal/preferences" && request.Method == "PUT" {
		return terminalPreferences(ctx, tx, request, principal)
	}
	if strings.HasPrefix(route, "restaurant/") {
		// Desde la etapa 9D la operación recibe el autorizador de supervisor, como los cobros, para
		// «Cancelar cuenta» sin el permiso propio.
		result, err := operations.Handle(ctx, tx, operations.Request{
			Method: request.Method, Route: route, Query: request.Query, Body: request.Body,
			Actor:     operations.Actor{UserID: principal.UserID, DeviceID: principal.DeviceID, Permissions: principal.Permissions},
			Authorize: supervisorAuthorizer(principal),
		}, func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error {
			return audit(ctx, tx, principal, action, entity, id, before, after, true)
		})
		var problem *operations.Error
		if errors.As(err, &problem) {
			return nil, &Fault{Status: problem.Status, Code: problem.Code, Message: problem.Message, Details: problem.Details}
		}
		var catalogProblem *catalog.Error
		if errors.As(err, &catalogProblem) {
			return nil, &Fault{Status: catalogProblem.Status, Code: catalogProblem.Code, Message: catalogProblem.Message}
		}
		// Una operación comercial puede fallar por una guarda financiera o por la integración de cocina;
		// ambas se traducen a su error público en lugar de un 503 genérico.
		var paymentProblem *payments.Error
		if errors.As(err, &paymentProblem) {
			return nil, paymentFault(paymentProblem)
		}
		var kitchenProblem *kitchen.Error
		if errors.As(err, &kitchenProblem) {
			return nil, &Fault{Status: kitchenProblem.Status, Code: kitchenProblem.Code, Message: kitchenProblem.Message, Details: kitchenProblem.Details}
		}
		return result, err
	}
	if strings.HasPrefix(route, "payments/") {
		result, err := payments.Handle(ctx, tx, payments.Request{
			Method: request.Method, Route: route, Query: request.Query, Body: request.Body,
			Actor:     payments.Actor{UserID: principal.UserID, DeviceID: principal.DeviceID, Permissions: principal.Permissions},
			Authorize: supervisorAuthorizer(principal),
			Queues:    service.Queues,
		}, func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error {
			return audit(ctx, tx, principal, action, entity, id, before, after, true)
		})
		var problem *payments.Error
		if errors.As(err, &problem) {
			return nil, paymentFault(problem)
		}
		var kitchenProblem *kitchen.Error
		if errors.As(err, &kitchenProblem) {
			return nil, &Fault{Status: kitchenProblem.Status, Code: kitchenProblem.Code, Message: kitchenProblem.Message, Details: kitchenProblem.Details}
		}
		return result, err
	}
	if strings.HasPrefix(route, "catalog/") {
		result, err := catalog.Handle(ctx, tx, catalog.Request{
			Method: request.Method, Route: route, Query: request.Query, Body: request.Body,
			Actor: catalog.Actor{Permissions: principal.Permissions},
		}, func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error {
			return audit(ctx, tx, principal, action, entity, id, before, after, true)
		})
		var problem *catalog.Error
		if errors.As(err, &problem) {
			return nil, &Fault{Status: problem.Status, Code: problem.Code, Message: problem.Message}
		}
		return result, err
	}
	// Inventario (etapa 8): insumos, almacenes, existencias, libro de movimientos y costos.
	if strings.HasPrefix(route, "inventory/") {
		result, err := inventory.Handle(ctx, tx, inventory.Request{
			Method: request.Method, Route: route, Query: request.Query, Body: request.Body,
			Actor: inventory.Actor{UserID: principal.UserID, DeviceID: principal.DeviceID, Permissions: principal.Permissions},
		}, func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error {
			return audit(ctx, tx, principal, action, entity, id, before, after, true)
		})
		var problem *inventory.Error
		if errors.As(err, &problem) {
			return nil, &Fault{Status: problem.Status, Code: problem.Code, Message: problem.Message}
		}
		return result, err
	}
	// Identidad documental (etapa 9B, fase B2): documents/identity*.
	if strings.HasPrefix(route, "documents/identity") {
		return handleDocumentIdentity(ctx, tx, request, principal, route)
	}
	// Diseñador de documentos (etapa 9B, fase A): plantillas, versiones, vista previa, muestras e
	// impresoras compatibles. La identidad documental (documents/identity*) es de la fase B.
	if strings.HasPrefix(route, "documents/") && !strings.HasPrefix(route, "documents/identity") {
		return handleDesigner(ctx, tx, request, principal, route, service)
	}
	// Reportes (etapa 9A): permisos y forma en esta transacción; la consulta, después (reports.go).
	if strings.HasPrefix(route, "reports/") {
		return handleReports(ctx, tx, request, principal, route)
	}
	// Mantenimiento del administrador (etapa 9F): «Quitar datos de prueba» (maintenance.go).
	if strings.HasPrefix(route, "maintenance/") {
		return service.maintenanceRoutes(ctx, tx, request, principal, route)
	}
	// Las funciones del establecimiento se identifican por código, no por UUID.
	if route == "settings/features" {
		return establishmentFeatures(ctx, tx, request, principal)
	}
	// «Hora de inicio del día» (etapa 9A): un solo valor, editable solo aquí.
	if route == "settings/business-day" {
		return businessDaySettings(ctx, tx, request, principal)
	}
	// Umbral de cuenta abierta por mucho tiempo (etapa 9D, §3.4), en «Funciones del establecimiento».
	if route == "settings/long-open-hours" {
		return longOpenHoursSettings(ctx, tx, request, principal)
	}
	parts := strings.Split(route, "/")
	if len(parts) > 1 && parts[1] != "invitations" && !identifierPattern.MatchString(parts[1]) {
		return nil, invalid("El identificador no es válido.")
	}
	switch parts[0] {
	case "users":
		return users(ctx, tx, request, principal, parts)
	case "roles":
		return roles(ctx, tx, request, principal, parts)
	case "permissions":
		if err := principal.require("permissions.read"); err != nil {
			return nil, err
		}
		if request.Method == "GET" && len(parts) == 1 {
			return Permissions, nil
		}
	case "settings":
		return settings(ctx, tx, request, principal)
	case "registers":
		return registers(ctx, tx, request, principal, parts)
	case "devices":
		return devices(ctx, tx, request, principal, parts)
	case "sessions":
		return sessions(ctx, tx, request, principal, parts)
	case "audit":
		result, err := auditLog(ctx, tx, request, principal)
		actor := catalog.Actor{Permissions: principal.Permissions}
		if !actor.Can("catalog.prices.read") {
			return catalog.RedactPrices(result, actor), err
		}
		return result, err
	}
	return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
}

// handleDesigner delega documents/* en el diseñador con la misma transacción, el actor y la
// auditoría, y traduce los errores del diseñador y de los módulos de origen (cocina y cobros) que
// aplica la vista previa con un documento real.
func handleDesigner(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string, service *Service) (any, error) {
	result, err := designer.Handle(ctx, tx, designer.Request{
		Method: request.Method, Route: route, Query: request.Query, Body: request.Body,
		Actor:   designer.Actor{UserID: principal.UserID, DeviceID: principal.DeviceID, Permissions: principal.Permissions},
		Queues:  service.Queues,
		Rasters: service.documentRasters(),
	}, func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error {
		return audit(ctx, tx, principal, action, entity, id, before, after, true)
	})
	var problem *designer.Error
	if errors.As(err, &problem) {
		return nil, &Fault{Status: problem.Status, Code: problem.Code, Message: problem.Message, Details: problem.Details}
	}
	var kitchenProblem *kitchen.Error
	if errors.As(err, &kitchenProblem) {
		return nil, &Fault{Status: kitchenProblem.Status, Code: kitchenProblem.Code, Message: kitchenProblem.Message, Details: kitchenProblem.Details}
	}
	var paymentProblem *payments.Error
	if errors.As(err, &paymentProblem) {
		return nil, paymentFault(paymentProblem)
	}
	return result, err
}

// paymentFault traduce un error financiero a su respuesta pública. Si el rechazo pide su propia
// auditoría (etapa 7.1), runProtected revierte la operación y confirma solo ese registro.
func paymentFault(problem *payments.Error) *Fault {
	fault := &Fault{Status: problem.Status, Code: problem.Code, Message: problem.Message, Details: problem.Details}
	if problem.Rejection != nil {
		entityID := problem.Rejection.EntityID
		fault.Rejection = &RejectionAudit{
			Action:   problem.Rejection.Action,
			Entity:   problem.Rejection.Entity,
			EntityID: &entityID,
			Details:  problem.Rejection.Details,
		}
	}
	return fault
}

func currentIdentity(ctx context.Context, tx pgx.Tx, principal Principal) (any, error) {
	preference, err := readPreferences(ctx, tx, principal.UserID, principal.Permissions)
	if err != nil {
		return nil, err
	}
	// La terminal informa su pantalla de inicio (etapa 9C, §1.5): la aplicación abre en ella al
	// iniciar sesión y el logotipo vuelve a ella. La preferencia por usuario ya no decide el inicio.
	terminal, err := object(ctx, tx, "SELECT id,name,register_id,keyboard_mode,home_screen FROM app.devices WHERE id=$1", principal.DeviceID)
	if err != nil {
		return nil, err
	}
	// Etapa 9F (§4.5): el nombre visible de la precuenta lo leen todos los usuarios con la sesión, porque
	// aparece en botones, avisos y el menú de quien no puede consultar la configuración de cobro.
	establishment, err := object(ctx, tx, `SELECT e.name,e.timezone,e.currency,s.precheck_name
        FROM app.establishment e CROSS JOIN app.payment_settings s`)
	if err != nil {
		return nil, err
	}
	return map[string]any{"user": principal, "preferences": preference, "terminal": terminal, "establishment": establishment}, nil
}
