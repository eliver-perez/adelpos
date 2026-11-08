// Package events conserva notificaciones transaccionales y el contrato versionado de replay.
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
