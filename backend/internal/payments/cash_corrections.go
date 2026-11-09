package payments

import (
	"strings"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
)

// Correcciones y regularizaciones de operaciones de efectivo. Nunca se edita ni borra la
// operación original: se registra una corrección enlazada que invierte exactamente su efecto
// (misma moneda e importe, dirección opuesta) y cada original se corrige una sola vez. Para
// cambiar un importe se corrige la original y se registra una operación nueva.
//
//   - Si el turno de la original sigue abierto, la corrección se registra en ese mismo turno y
//     actualiza su efectivo esperado.
//   - Si ya está cerrado, es una regularización: se registra en el turno vigente de la misma
//     caja, enlazada y con motivo, sin cambiar la diferencia registrada del turno cerrado ni el
//     efectivo esperado del turno vigente (no movió dinero en él), de modo que no puede usarse
//     para ocultar una diferencia de caja. No existe un «ajuste de diferencia».
//
// Requieren cash.corrections.record y autorización con la regla de §0-B: el propio permiso
// cash.corrections.authorize basta; si no, otra persona con ese permiso.

// CorrectionInput es la solicitud de corrección o regularización de una operación de efectivo.
type CorrectionInput struct {
	OperationID         string              `json:"operation_id"`
	CorrectsOperationID string              `json:"corrects_operation_id"`
	Reason              string              `json:"reason"`
	Authorization       *AuthorizationInput `json:"authorization"`
}

// oppositeDirection invierte el efecto físico de una operación.
func oppositeDirection(direction string) string {
	if direction == directionIn {
		return directionOut
	}
	return directionIn
}

// correctionConcept describe la corrección a partir del concepto original, dentro de 120 caracteres.
func correctionConcept(original *CashOperation, regularization bool) string {
	prefix := "Corrección: "
	if regularization {
		prefix = "Regularización: "
	}
	concept := []rune(prefix + original.Concept)
	if len(concept) > 120 {
		concept = concept[:120]
	}
	return string(concept)
}

// correctCashOperation registra la corrección (o regularización) en el turno de la ruta, que debe
// ser el turno abierto de la caja de la terminal.
func (handler *service) correctCashOperation(parts []string) (any, error) {
	var input CorrectionInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.CorrectsOperationID); err != nil {
		return nil, invalid("Indica la operación de efectivo que se corrige.")
	}
	reason := strings.TrimSpace(input.Reason)
	if textLength(reason) < 5 || textLength(reason) > 500 {
		return nil, invalid("Indica el motivo de la corrección (5–500 caracteres).")
	}
	shift, err := handler.operableShift(parts[1])
	if err != nil {
		return nil, err
	}
	original, err := handler.readCashOperation(input.CorrectsOperationID)
	if err != nil {
		return nil, err
	}
	regularization, err := handler.correctionTarget(shift, original)
	if err != nil {
		return nil, err
	}
	approved, err := handler.approve("cash_correction", handler.request.Actor.Can(permissionCorrectionsAuthorize), input.Authorization,
		"Corregir un movimiento de efectivo requiere la autorización de un supervisor.",
		func(supervisor *Supervisor) (bool, error) {
			return supervisor.Can(permissionCorrectionsAuthorize), nil
		})
	if err != nil {
		return nil, err
	}
	// A partir de aquí se escribe: permisos, autorización, turno y operación original ya se verificaron.
	draft := &operationDraft{
		kind:      operationCorrection,
		currency:  original.Currency,
		amount:    decimal.RequireFromString(original.Amount),
		direction: oppositeDirection(original.Direction),
		concept:   correctionConcept(original, regularization),
		reason:    reason,
	}
	id, err := handler.insertCashOperation(shift, draft, original.USDRate, approved, &original.ID, regularization)
	if err != nil {
		return nil, err
	}
	correction, err := handler.readCashOperation(id)
	if err != nil {
		return nil, err
	}
	saved, err := handler.readCashOperation(original.ID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.operation.recorded", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	action := "cash.operation_corrected"
	if regularization {
		action = "cash.operation_regularized"
	}
	if err := handler.audit(handler.ctx, handler.tx, action, "cash_operation", &original.ID,
		map[string]any{"operation_id": original.ID, "kind": original.Kind, "amount": original.Amount, "currency": original.Currency},
		map[string]any{"correction_id": id, "shift_id": shift.ID, "regularization": regularization, "reason": reason}); err != nil {
		return nil, err
	}
	return map[string]any{"correction": correction, "original": saved}, nil
}

// correctionTarget decide si la corrección pertenece al mismo turno o es una regularización y
// rechaza los casos que no proceden.
func (handler *service) correctionTarget(shift *Shift, original *CashOperation) (bool, error) {
	if original.Kind == operationCorrection {
		return false, invalid("Una corrección no se corrige. Si hace falta, registra una operación nueva.")
	}
	if original.CorrectedByID != nil {
		return false, conflict("OPERATION_ALREADY_CORRECTED", "La operación ya tiene una corrección registrada.")
	}
	if original.ShiftID == shift.ID {
		return false, nil
	}
	originalShift, err := handler.readShift("s.id=$1", original.ShiftID)
	if err != nil {
		return false, err
	}
	if originalShift.State != shiftClosed {
		return false, conflict("CORRECTION_SHIFT", "La operación pertenece a otro turno activo; corrígela desde la caja "+
			originalShift.RegisterName+".")
	}
	if originalShift.RegisterID != shift.RegisterID {
		return false, conflict("REGULARIZATION_REGISTER", "La regularización se registra en el turno vigente de la caja "+
			originalShift.RegisterName+".")
	}
	return true, nil
}
