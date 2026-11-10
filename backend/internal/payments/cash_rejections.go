package payments

import "time"

// Intentos rechazados por efectivo insuficiente (etapa 7.1, §3.5). La etapa 7 documentó que quien
// autoriza salidas sin ver el esperado podría deducirlo probando importes: cada rechazo queda en
// audit_events (acción cash.outflow_rejected, entidad cash_shift, success=false) aunque la
// operación se revierta, y un administrador con cash.expected.read los consulta por turno. El
// registro nunca contiene el esperado ni cuánto faltó.

// RejectedOutflow es un retiro o gasto rechazado por efectivo insuficiente, sin identificadores
// internos de usuarios ni terminales.
type RejectedOutflow struct {
	ID                string    `json:"id"`
	OccurredAt        time.Time `json:"occurred_at"`
	Kind              string    `json:"kind"`
	Currency          string    `json:"currency"`
	AttemptedAmount   string    `json:"attempted_amount"`
	ActorName         string    `json:"actor_name"`
	DeviceName        string    `json:"device_name"`
	RegisterName      string    `json:"register_name"`
	AuthorizationMode *string   `json:"authorization_mode"`
	AuthorizedByName  *string   `json:"authorized_by_name"`
}

// La consulta usa el índice parcial audit_events_cash_outflow_rejected_idx (migración 0009). Los
// datos de detalle los escribió Go al rechazar; los nombres se leen al consultar.
const rejectedOutflowsQuery = `SELECT a.id,a.occurred_at,coalesce(a.after_state->>'kind',''),coalesce(a.after_state->>'currency',''),
    coalesce(a.after_state->>'attempted_amount',''),coalesce(actor.display_name,a.actor_username),coalesce(d.name,''),
    coalesce(r.name,''),a.after_state->>'authorization_mode',authorizer.display_name
    FROM app.audit_events a
    LEFT JOIN app.users actor ON actor.id=a.actor_id
    LEFT JOIN app.devices d ON d.id=a.device_id
    LEFT JOIN app.registers r ON r.id::text=a.after_state->>'register_id'
    LEFT JOIN app.users authorizer ON authorizer.id::text=a.after_state->>'authorized_by'
    WHERE a.action=$1 AND a.entity_type='cash_shift' AND a.entity_id=$2 AND NOT a.success
    ORDER BY a.occurred_at,a.id`

// shiftRejectedOutflows atiende GET payments/shifts/{id}/rejected-outflows (cash.expected.read):
// los intentos rechazados del turno, con las reglas de consulta del turno. Durante el arqueo de
// cierre la captura es ciega para todos y no se entregan: un importe rechazado acota el esperado.
func (handler *service) shiftRejectedOutflows(parts []string) (any, error) {
	shift, err := handler.readableShift(parts[1])
	if err != nil {
		return nil, err
	}
	result := map[string]any{"shift_id": shift.ID, "visible": false, "rejections": []RejectedOutflow{}}
	if !handler.expectedVisible(shift) {
		result["message"] = "Durante el arqueo de cierre no se consultan los intentos rechazados."
		return result, nil
	}
	rows, err := handler.tx.Query(handler.ctx, rejectedOutflowsQuery, outflowRejection, shift.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rejections := []RejectedOutflow{}
	for rows.Next() {
		var item RejectedOutflow
		if err := rows.Scan(&item.ID, &item.OccurredAt, &item.Kind, &item.Currency, &item.AttemptedAmount, &item.ActorName,
			&item.DeviceName, &item.RegisterName, &item.AuthorizationMode, &item.AuthorizedByName); err != nil {
			return nil, err
		}
		rejections = append(rejections, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result["visible"] = true
	result["rejections"] = rejections
	return result, nil
}
