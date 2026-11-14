package events

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/stationscope"
)

type Change struct {
	Sequence    int64     `json:"sequence"`
	ID          string    `json:"id"`
	Version     int       `json:"version"`
	Kind        string    `json:"kind"`
	Scope       string    `json:"scope"`
	StationID   *string   `json:"station_id"`
	AggregateID *string   `json:"aggregate_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type Batch struct {
	Cursor int64    `json:"cursor"`
	Reset  bool     `json:"reset"`
	Events []Change `json:"events"`
}

func NewID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	raw[6] = raw[6]&15 | 64
	raw[8] = raw[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

func Append(ctx context.Context, tx pgx.Tx, kind, scope string, aggregateID, stationID *string) error {
	// La secuencia no puede adelantarse al commit de otra transacción pendiente.
	// Todos los productores toman este lock al final de sus cambios, hasta el commit.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7820148306)"); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO app.realtime_events(id,kind,scope,aggregate_id,station_id)
        VALUES ($1,$2,$3,$4,$5)`, NewID(), kind, scope, aggregateID, stationID)
	return err
}

// Read entrega los eventos posteriores al cursor que la persona puede recibir. stations son las estaciones
// de cocina que puede ver (etapa 9F: en la terminal, intersectadas con las que atiende el usuario; con
// kitchen.stations.all, todas): un evento de una estación fuera de ellas no se entrega.
func Read(ctx context.Context, tx pgx.Tx, after int64, permissions []string, stations *stationscope.Scope, stationFilter string) (*Batch, error) {
	var head, oldest int64
	if err := tx.QueryRow(ctx, "SELECT coalesce(max(sequence),0),coalesce(min(sequence),0) FROM app.realtime_events").Scan(&head, &oldest); err != nil {
		return nil, err
	}
	batch := &Batch{Cursor: after, Events: []Change{}}
	// Ventana de replay acotada. El cliente reconstruye estado si quedó fuera de ella.
	if after < max(oldest-1, head-5000) || after > head {
		batch.Cursor, batch.Reset = head, true
		return batch, nil
	}
	allStations := slices.Contains(permissions, "kitchen.stations.all")
	rows, err := tx.Query(ctx, `SELECT to_jsonb(e) FROM app.realtime_events e WHERE sequence>$1 ORDER BY sequence LIMIT 200`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var event Change
		if err := json.Unmarshal(encoded, &event); err != nil {
			return nil, err
		}
		batch.Cursor = event.Sequence
		if stationFilter != "" && (event.StationID == nil || *event.StationID != stationFilter) {
			continue
		}
		if !allStations && event.StationID == nil && (event.Scope == "kitchen" || event.Scope == "printing") {
			continue
		}
		stationAllowed := event.StationID == nil || stations.Allows(*event.StationID)
		if !stationAllowed || !allowed(event.Scope, permissions) {
			continue
		}
		batch.Events = append(batch.Events, event)
	}
	return batch, rows.Err()
}

func allowed(scope string, permissions []string) bool {
	// Los eventos de caja (etapa 7) solo informan qué turno cambió, sin importes. Los reciben
	// quienes consultan turnos y quienes cobran (payments.read), para que una terminal sepa de
	// inmediato que la caja entró en arqueo o se cerró.
	if scope == "cash" {
		return slices.Contains(permissions, "cash.shifts.read") || slices.Contains(permissions, "payments.read")
	}
	// Los eventos de inventario (etapa 8) solo identifican el agregado que cambió, nunca costos;
	// los recibe quien consulta inventario.
	required := map[string]string{
		"restaurant":    "accounts.read",
		"takeaway":      "takeaway.read",
		"kitchen":       "kitchen.states.read",
		"printing":      "kitchen.queue.read",
		"configuration": "kitchen.stations.read",
		"inventory":     "inventory.read",
		// Diseñador de documentos (etapa 9B): plantillas e identidad documental cambiaron, sin
		// contenido; lo reciben quienes consultan plantillas.
		"documents": "documents.templates.read",
		// Catálogo (etapa 9G, decisión D8): cambió el catálogo o un producto se agotó o volvió a estar disponible,
		// con el producto como agregado y sin precios; lo reciben quienes consultan el catálogo.
		"catalog": "catalog.read",
	}[scope]
	return slices.Contains(permissions, required)
}
