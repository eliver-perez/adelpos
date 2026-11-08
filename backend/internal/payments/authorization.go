package payments

import (
	"github.com/shopspring/decimal"
)

// AuthorizationInput es la autorización puntual de un supervisor enviada con la operación.
// La contraseña solo existe en esta solicitud: no se guarda ni forma parte de la huella idempotente.
type AuthorizationInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// approval describe quién habilitó una operación sensible.
type approval struct {
	scope      string
	supervisor *Supervisor
}

func (item *approval) authorizedBy() *string {
	if item == nil || item.supervisor == nil {
		return nil
	}
	return &item.supervisor.UserID
}

func authorizationRequired(message string) error {
	return &Error{Status: 403, Code: "AUTHORIZATION_REQUIRED", Message: message}
}

// discountLimit obtiene el límite de descuento vigente de un usuario según su rol. La identidad
// reservada de soporte conserva el máximo; sin configuración, el límite es 0 %.
func (handler *service) discountLimit(userID string) (decimal.Decimal, error) {
	var limit string
	err := handler.tx.QueryRow(handler.ctx, `SELECT CASE WHEN u.support THEN 100::numeric ELSE coalesce(l.max_percent,0) END::numeric(5,2)::text
        FROM app.users u LEFT JOIN app.role_discount_limits l ON l.role_id=u.role_id WHERE u.id=$1`, userID).Scan(&limit)
	if err != nil {
		return zero, err
	}
	return decimal.RequireFromString(limit), nil
}

// approve resuelve, antes de cualquier escritura, si el actor puede ejecutar la operación por sí
// mismo o si se necesita una autorización de supervisor válida. allowed indica si el actor
// cumple; supervisorCan valida los permisos efectivos actuales del supervisor.
func (handler *service) approve(scope string, allowed bool, input *AuthorizationInput, message string, supervisorCan func(*Supervisor) (bool, error)) (*approval, error) {
	if allowed {
		return &approval{scope: scope}, nil
	}
	if input == nil || input.Username == "" {
		return nil, authorizationRequired(message)
	}
	if handler.request.Authorize == nil {
		return nil, denied()
	}
	supervisor, err := handler.request.Authorize(handler.ctx, handler.tx, input.Username, input.Password)
	if err != nil {
		return nil, err
	}
	if supervisor.UserID == handler.request.Actor.UserID {
		return nil, &Error{Status: 403, Code: "SELF_AUTHORIZATION", Message: "No puedes autorizar tu propia operación."}
	}
	permitted, err := supervisorCan(supervisor)
	if err != nil {
		return nil, err
	}
	if !permitted {
		return nil, &Error{Status: 403, Code: "AUTHORIZATION_INSUFFICIENT", Message: "El supervisor indicado no tiene permiso o límite suficiente para autorizar esta operación."}
	}
	return &approval{scope: scope, supervisor: supervisor}, nil
}

// recordApproval guarda la autorización ligada a la operación concreta; operation_id es único,
// por lo que la misma autorización no puede reutilizarse en otra operación.
func (handler *service) recordApproval(item *approval, accountID string, amount decimal.Decimal) error {
	if item == nil || item.supervisor == nil {
		return nil
	}
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.payment_authorizations(id,operation_id,scope,account_id,amount,requested_by,authorized_by,device_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, newID(), handler.operationID, item.scope, accountID, money(amount),
		handler.request.Actor.UserID, item.supervisor.UserID, handler.request.Actor.DeviceID)
	if err != nil {
		return err
	}
	return handler.audit(handler.ctx, handler.tx, "payments.supervisor_authorized", "account", &accountID, nil, map[string]string{
		"scope": item.scope, "supervisor": item.supervisor.Username, "amount": money(amount),
	})
}
