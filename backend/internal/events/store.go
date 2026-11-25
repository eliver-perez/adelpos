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

// Etapa L1 (Fase C): license.changed avisa que cambió el estado de la licencia o el bloqueo por la hora del equipo. Lo
// reciben todas las personas que leen el canal, sin importar sus permisos ni su estación, para el aviso fijo de cada
// terminal; no lleva datos (solo dice que vuelvan a leer auth/me o restaurant/state). Se guarda en el ámbito
// «configuration», que ya existe en la base, así que no necesita migración.
const (
	LicenseChangedKind  = "license.changed"
	licenseChangedScope = "configuration"
)

// AppendLicenseChanged publica license.changed en la transacción (con el lock 7820148306, como todo productor).
func AppendLicenseChanged(ctx context.Context, tx pgx.Tx) error {
	return Append(ctx, tx, LicenseChangedKind, licenseChangedScope, nil, nil)
}

// Head es el cursor del último evento confirmado (0 si no hay ninguno). Etapa A1: GET restaurant/state lo entrega como
// event_cursor para que el cliente abra el canal desde el momento de la lectura sin perder nada.
func Head(ctx context.Context, tx pgx.Tx) (int64, error) {
	var head int64
	err := tx.QueryRow(ctx, "SELECT coalesce(max(sequence),0) FROM app.realtime_events").Scan(&head)
	return head, err
}

// Read entrega los eventos posteriores al cursor que la persona puede recibir. stations son las estaciones
// de cocina que puede ver (etapa 9F: en la terminal, intersectadas con las que atiende el usuario; con
// kitchen.stations.all, todas): un evento de una estación fuera de ellas no se entrega.
func Read(ctx context.Context, tx pgx.Tx, after int64, permissions []string, stations *stationscope.Scope, stationFilter string) (*Batch, error) {
	allStations := slices.Contains(permissions, "kitchen.stations.all")
	return ReadWith(ctx, tx, after, func(candidates []Change) ([]Change, error) {
		accepted := []Change{}
		for _, event := range candidates {
			// Etapa L1 (Fase C): el cambio de la licencia o de la hora llega a todas las terminales (ver LicenseChangedKind).
			if IsLicenseChanged(event) {
				accepted = append(accepted, event)
				continue
			}
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
			accepted = append(accepted, event)
		}
		return accepted, nil
	})
}

// IsLicenseChanged dice si el evento es license.changed, que reciben todas las terminales.
func IsLicenseChanged(event Change) bool {
	return event.Kind == LicenseChangedKind && event.Scope == licenseChangedScope
}

// ReadWith entrega los eventos posteriores al cursor que acepta filter, con la misma ventana de replay y el mismo
// cursor que Read: el cursor avanza también sobre los eventos que filter descarta. filter recibe el lote completo (hasta
// 200 eventos, en orden) para decidir con una sola consulta si hace falta. Etapa K1 (Fase A): lo usa la pantalla de
// cocina Android, que no tiene permisos de rol sino su propio filtro (kitchen.DisplayEvents).
func ReadWith(ctx context.Context, tx pgx.Tx, after int64, filter func([]Change) ([]Change, error)) (*Batch, error) {
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
	rows, err := tx.Query(ctx, `SELECT to_jsonb(e) FROM app.realtime_events e WHERE sequence>$1 ORDER BY sequence LIMIT 200`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := []Change{}
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
		candidates = append(candidates, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if len(candidates) == 0 {
		return batch, nil
	}
	accepted, err := filter(candidates)
	if err != nil {
		return nil, err
	}
	batch.Events = accepted
	return batch, nil
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
