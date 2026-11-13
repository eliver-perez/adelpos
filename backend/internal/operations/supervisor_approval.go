package operations

import (
	"strings"

	"local/ad-el-pos/server/internal/payments"
)

// Autorización puntual de un supervisor en la operación del restaurante (etapa 9D para «Cancelar
// cuenta» y etapa 9F para «Cancelar unidades» y la reducción de cantidades desde «Modificar»). Sigue
// el mismo mecanismo de los cobros: basta el permiso propio; sin él, el usuario y la contraseña de
// otra persona que sí lo tenga, validados por administration en la misma transacción (sin sesión, con
// el límite de intentos del inicio de sesión) y siempre antes de la primera escritura de la ruta. Nadie
// se autoriza a sí mismo y la contraseña nunca entra en la huella idempotente (requestFingerprint), la
// respuesta guardada, los eventos ni la auditoría.

// supervisorRule describe una operación que, sin el permiso propio, procede con la autorización de un
// supervisor que lo tenga: el permiso y los mensajes con que Go la pide o la rechaza.
type supervisorRule struct {
	permission   string
	required     string
	insufficient string
}

// accountCancelRule es la regla de «Cancelar cuenta» (etapa 9D).
var accountCancelRule = supervisorRule{
	permission:   cancelPermission,
	required:     "Cancelar una cuenta requiere la autorización de un supervisor con permiso para cancelar cuentas.",
	insufficient: "El supervisor indicado no tiene permiso para cancelar cuentas.",
}

// unitsCancelRule es la regla de «Cancelar unidades» y de reducir la cantidad de una partida ya
// enviada desde «Modificar» (etapa 9F, §7.2).
var unitsCancelRule = supervisorRule{
	permission:   unitsCancelPermission,
	required:     "Cancelar o reducir unidades enviadas a cocina requiere la autorización de un supervisor con permiso para cancelar partidas.",
	insufficient: "El supervisor indicado no tiene permiso para cancelar partidas o cantidades.",
}

// approveWithSupervisor decide, antes de cualquier escritura, si el usuario actúa con su propio
// permiso o si hace falta una autorización de supervisor válida. Devuelve el supervisor que autorizó
// o nil si bastó el permiso propio (una autorización enviada en ese caso se ignora).
func (handler *service) approveWithSupervisor(rule supervisorRule, input *payments.AuthorizationInput) (*payments.Supervisor, error) {
	if handler.request.Actor.Can(rule.permission) {
		return nil, nil
	}
	if input == nil || strings.TrimSpace(input.Username) == "" {
		return nil, &Error{Status: 403, Code: "AUTHORIZATION_REQUIRED", Message: rule.required}
	}
	if handler.request.Authorize == nil {
		return nil, &Error{Status: 403, Code: "PERMISSION_DENIED", Message: "No tienes permiso para esta operación del restaurante."}
	}
	supervisor, err := handler.request.Authorize(handler.ctx, handler.tx, input.Username, input.Password)
	if err != nil {
		return nil, err
	}
	if supervisor.UserID == handler.request.Actor.UserID {
		return nil, &Error{Status: 403, Code: "SELF_AUTHORIZATION", Message: "No puedes autorizar tu propia operación."}
	}
	if !supervisor.Can(rule.permission) {
		return nil, &Error{Status: 403, Code: "AUTHORIZATION_INSUFFICIENT", Message: rule.insufficient}
	}
	return supervisor, nil
}

// approveCancellation aplica la regla de «Cancelar cuenta».
func (handler *service) approveCancellation(input *payments.AuthorizationInput) (*payments.Supervisor, error) {
	return handler.approveWithSupervisor(accountCancelRule, input)
}

// authorizationFields son los datos de quién autorizó que se guardan en el evento de una cancelación
// o reducción de unidades (etapa 9F), con la misma forma que account.cancelled: el identificador, el
// usuario y el nombre visible del supervisor, o null en los tres si bastó el permiso propio. Nunca
// llevan la contraseña.
func (handler *service) authorizationFields(supervisor *payments.Supervisor) (map[string]any, error) {
	fields := map[string]any{"authorized_by": nil, "authorized_by_username": nil, "authorized_by_name": nil}
	if supervisor == nil {
		return fields, nil
	}
	var name string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT display_name FROM app.users WHERE id=$1", supervisor.UserID).Scan(&name); err != nil {
		return nil, err
	}
	fields["authorized_by"] = supervisor.UserID
	fields["authorized_by_username"] = supervisor.Username
	fields["authorized_by_name"] = name
	return fields, nil
}

// auditSupervisor deja auditada por sí misma una autorización de supervisor (sin contraseña), como
// en los cobros y en «Cancelar cuenta». scope identifica la operación autorizada.
func (handler *service) auditSupervisor(accountID string, supervisor *payments.Supervisor, scope string, details map[string]any) error {
	if supervisor == nil {
		return nil
	}
	after := map[string]any{"scope": scope, "supervisor": supervisor.Username}
	for key, value := range details {
		after[key] = value
	}
	return handler.audit(handler.ctx, handler.tx, "restaurant.supervisor_authorized", "account", &accountID, nil, after)
}
