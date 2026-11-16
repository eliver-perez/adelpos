package operations

import (
	"strings"

	"local/ad-el-pos/server/internal/payments"
)

// Autorización puntual de un supervisor en la operación del restaurante (etapa 9D, «Cancelar
// cuenta»). Sigue el mismo mecanismo de los cobros: basta el permiso propio; sin él, el usuario y la
// contraseña de otra persona que sí lo tenga, validados por administration en la misma transacción
// (sin sesión, con el límite de intentos del inicio de sesión) y siempre antes de la primera escritura
// de la ruta. Nadie se autoriza a sí mismo y la contraseña nunca entra en la huella idempotente
// (requestFingerprint), la respuesta guardada, los eventos ni la auditoría.
//
// La 9F aplicó esta misma regla a «Cancelar unidades» y a reducir la cantidad desde «Modificar»; la
// etapa 9H la retiró de esas dos operaciones (decisión del propietario: basta el permiso propio
// orders.cancel y queda registrado quién lo hizo). Solo «Cancelar cuenta» la usa.

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
