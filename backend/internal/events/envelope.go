// Package events reserva el contrato de transporte; aún no existe un bus ni un WebSocket activo.
package events

import (
	"encoding/json"
	"time"
)

type Envelope struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Version     int             `json:"version"`
	OccurredAt  time.Time       `json:"occurred_at"`
	OperationID string          `json:"operation_id,omitempty"`
	Payload     json.RawMessage `json:"payload"`
}
