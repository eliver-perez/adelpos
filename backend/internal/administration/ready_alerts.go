package administration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/internal/httpapi"
)

// Avisos de platillos listos (etapa A2, parte D; §4 de la especificación y decisiones A2-D4 y A2-D9). El teléfono o
// la tableta de meseros recibe lo que cocina marcó «Listo» para los meseros que entraron en él, aunque esté en
// «¿Quién eres?» o con la aplicación cerrada (lo mantiene su servicio en primer plano). Por eso estas rutas llevan
// solo la credencial del dispositivo, como el agente de impresión: sin sesión de usuario.
//
//   - GET ready-alerts?after=<cursor> es la lectura de recuperación y GET ready-alerts/stream su canal SSE (con
//     Last-Event-ID), con el mismo patrón del canal de eventos: una lectura cada 750 ms y cierre a los 30 s.
//   - Solo dispositivos android no revocados (revocado: 401 DEVICE_REVOKED, como siempre; de otro tipo, 403).
//   - Go decide a quién llega (no el dispositivo): un aviso llega al dispositivo D si su destinatario M entró con PIN
//     en D durante el día comercial en curso y además el aviso es para D (al «Listo», M tenía en D su sesión con PIN
//     vigente de actividad más reciente) o para todos (al «Listo», M no tenía ninguna vigente en un android). Ver
//     kitchen.recordReadyNotices.
//   - Solo los datos del aviso: cuenta, modalidad, mesa o folio y nombre del cliente, mesero, platillos y cantidades.
//     Nunca precios ni importes.
//   - El cursor es el último aviso que Go revisó para el dispositivo (le corresponda o no): el dispositivo lo guarda y
//     vuelve con after=cursor; lo que pasó mientras no estuvo llega entonces, una sola vez.

// readyAlertsGroups es cuántos avisos entrega una lectura como máximo; con más, el cursor es el del último entregado
// y el resto llega en la siguiente.
const readyAlertsGroups = 50

// ReadyAlertsBatch es la respuesta de GET ready-alerts y cada mensaje de su canal.
type ReadyAlertsBatch struct {
	Cursor int64 `json:"cursor"`
	// Active dice si el dispositivo debe mantener su servicio de avisos: el aviso está activado en Terminales y hay
	// un mesero con sesión con PIN vigente en él o que entró en él con PIN durante el día comercial en curso.
	Active bool `json:"active"`
	// DayEndsAt es el fin del día comercial en curso (la hora de inicio del día siguiente): ahí dejan de contar los
	// meseros que entraron hoy.
	DayEndsAt time.Time          `json:"day_ends_at"`
	Settings  ReadyAlertSettings `json:"settings"`
	Alerts    []ReadyAlert       `json:"alerts"`
}

// ReadyAlertSettings es lo que se elige en Terminales › Editar del dispositivo.
type ReadyAlertSettings struct {
	Enabled   bool `json:"enabled"`
	Sound     bool `json:"sound"`
	Vibration bool `json:"vibration"`
}

// ReadyAlert es un «Listo» de cocina para una cuenta: las unidades que pasaron a «Listo» en la misma operación de
// cocina, agrupadas por platillo. ID es el número del último aviso del grupo (el cursor crece con él).
type ReadyAlert struct {
	ID              int64       `json:"id"`
	AccountID       string      `json:"account_id"`
	Mode            string      `json:"mode"`
	TableName       *string     `json:"table_name"`
	Folio           int64       `json:"folio"`
	CustomerName    string      `json:"customer_name"`
	WaiterUserID    string      `json:"waiter_user_id"`
	WaiterShortName string      `json:"waiter_short_name"`
	Items           []ReadyItem `json:"items"`
	ReadyAt         time.Time   `json:"ready_at"`
}

// ReadyItem es un platillo listo: el nombre visible del producto (con su presentación, «Filete · Grande», como en la
// comanda) y cuántas unidades.
type ReadyItem struct {
	ProductName string `json:"product_name"`
	Quantity    int    `json:"quantity"`
}

// readyAlertsQuery son los avisos del dispositivo $1 posteriores al cursor $3 y hasta $4 (lo último registrado al
// leer), del día comercial que empezó en $2, agrupados por la operación de cocina que los marcó «Listo» (misma cuenta,
// mismo destinatario y mismo instante de la transacción).
const readyAlertsQuery = `WITH entered AS (
        SELECT DISTINCT user_id FROM app.sessions WHERE device_id=$1 AND opened_with_pin AND created_at>=$2
    ), notices AS (
        SELECT n.sequence,n.account_id,n.recipient_user_id,n.ready_at,
            coalesce(v.quote_line->>'name','')||coalesce(' · '||variant.name,'') AS product_name
        FROM app.ready_notices n
        JOIN app.restaurant_item_versions v ON v.id=n.version_id
        LEFT JOIN app.catalog_variants variant ON variant.id::text=v.quote_line->>'variant_id'
        WHERE n.sequence>$3 AND n.sequence<=$4 AND n.ready_at>=$2
            AND n.recipient_user_id IN (SELECT user_id FROM entered)
            AND (n.recipient_device_id IS NULL OR n.recipient_device_id=$1)
    ), groups AS (
        SELECT max(sequence) AS id,account_id,recipient_user_id,ready_at FROM notices
        GROUP BY account_id,recipient_user_id,ready_at ORDER BY max(sequence) LIMIT $5
    )
    SELECT jsonb_build_object('id',g.id,'account_id',a.id,'mode',a.mode,'table_name',t.name,'folio',a.folio,
        'customer_name',a.customer_name,'waiter_user_id',g.recipient_user_id,
        'waiter_short_name',coalesce(nullif(waiter.short_name,''),waiter.display_name),
        'items',(SELECT jsonb_agg(jsonb_build_object('product_name',item.product_name,'quantity',item.quantity)
                ORDER BY item.first_sequence)
            FROM (SELECT product_name,count(*)::integer AS quantity,min(sequence) AS first_sequence FROM notices item
                WHERE item.account_id=g.account_id AND item.recipient_user_id=g.recipient_user_id AND item.ready_at=g.ready_at
                GROUP BY product_name) item),
        'ready_at',g.ready_at)
    FROM groups g JOIN app.restaurant_accounts a ON a.id=g.account_id
    LEFT JOIN app.restaurant_tables t ON t.id=a.table_id JOIN app.users waiter ON waiter.id=g.recipient_user_id
    ORDER BY g.id`

// readyAlerts atiende GET ready-alerts con la credencial del dispositivo (ya autenticada: authenticateDevice).
func readyAlerts(ctx context.Context, tx pgx.Tx, request Request) (any, error) {
	for key := range request.Query {
		if key != "after" {
			return nil, invalid("Parámetro de avisos no permitido.")
		}
	}
	after := int64(0)
	if text := request.Query.Get("after"); text != "" {
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err != nil || parsed < 0 {
			return nil, invalid("El cursor de avisos no es válido.")
		}
		after = parsed
	}
	var kind string
	var settings ReadyAlertSettings
	if err := tx.QueryRow(ctx, `SELECT kind,ready_alerts_enabled,ready_alerts_sound,ready_alerts_vibration
        FROM app.devices WHERE id=$1`, request.DeviceID).Scan(&kind, &settings.Enabled, &settings.Sound, &settings.Vibration); err != nil {
		return nil, err
	}
	if kind != "android" {
		return nil, &Fault{Status: 403, Code: "READY_ALERTS_DEVICE_NOT_ALLOWED",
			Message: "Los avisos de platillos listos solo llegan a teléfonos y tabletas de meseros."}
	}
	// El lock de la operación compartido: cocina escribe los avisos con él en exclusiva hasta su commit, así que
	// ninguno queda a medias detrás del cursor (lo mismo que restaurant/state con el cursor de eventos).
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7820148305)"); err != nil {
		return nil, err
	}
	config, err := businessday.Load(ctx, tx)
	if err != nil {
		return nil, err
	}
	day := config.DayOf(time.Now())
	dayStart := config.Start(day.Date)
	var head int64
	if err := tx.QueryRow(ctx, "SELECT coalesce(max(sequence),0) FROM app.ready_notices").Scan(&head); err != nil {
		return nil, err
	}
	// Un cursor mayor que el último aviso solo pasa si la base volvió atrás (un respaldo restaurado): se lee desde el
	// principio del día comercial y el dispositivo descarta los que ya avisó.
	if after > head {
		after = 0
	}
	batch := &ReadyAlertsBatch{Cursor: head, DayEndsAt: day.Deadline, Settings: settings, Alerts: []ReadyAlert{}}
	if settings.Enabled {
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.sessions WHERE device_id=$1 AND opened_with_pin
            AND (created_at>=$2 OR (revoked_at IS NULL AND expires_at>now())))`, request.DeviceID, dayStart).Scan(&batch.Active)
		if err != nil {
			return nil, err
		}
	}
	// Apagado o sin meseros: sin avisos (no es un error) y el cursor avanza, para que al volver a activarse o al
	// entrar un mesero no lleguen avisos viejos.
	if !batch.Active {
		return batch, nil
	}
	alerts, err := collectJSON[ReadyAlert](ctx, tx, readyAlertsQuery, request.DeviceID, dayStart, after, head, readyAlertsGroups)
	if err != nil {
		return nil, err
	}
	batch.Alerts = alerts
	if len(alerts) == readyAlertsGroups {
		batch.Cursor = alerts[len(alerts)-1].ID
	}
	return batch, nil
}

// collectJSON lee filas de una sola columna jsonb.
func collectJSON[T any](ctx context.Context, tx pgx.Tx, query string, args ...any) ([]T, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []T{}
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var value T
		if err := json.Unmarshal(encoded, &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

// serveReadyAlerts es GET ready-alerts/stream: el mismo patrón del canal de eventos (serveEvents) con su propio
// contador, un canal por dispositivo, para no competir con los dos de eventos. Cada mensaje «alerts» lleva id = el
// cursor; al reconectarse, Last-Event-ID (o after) continúa desde ahí. Cada lectura revalida el dispositivo: si fue
// revocado manda «interrupted» con el código y termina.
func (service *Service) serveReadyAlerts(writer http.ResponseWriter, request *http.Request, local bool) {
	arguments := Request{Method: "GET", Path: "/api/v1/ready-alerts", Query: request.URL.Query(),
		DeviceID: request.Header.Get("X-Device-ID"), DeviceToken: request.Header.Get("X-Device-Token"), Local: local,
		OfflineCredential: request.Header.Get(OfflineCredentialHeader),
		ClientKind:        request.Header.Get(httpapi.ClientKindHeader)}
	if arguments.Query.Get("after") == "" && request.Header.Get("Last-Event-ID") != "" {
		arguments.Query.Set("after", request.Header.Get("Last-Event-ID"))
	}
	read := func() (*ReadyAlertsBatch, error) {
		ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
		defer cancel()
		result, err := service.Execute(ctx, arguments)
		if err != nil {
			return nil, err
		}
		return result.(*ReadyAlertsBatch), nil
	}
	initial, err := read()
	if err != nil {
		writeFault(writer, request, publicFault(err))
		return
	}
	service.streamMutex.Lock()
	if service.readyStreams == nil {
		service.readyStreams = map[string]int{}
	}
	if service.readyStreams[arguments.DeviceID] >= 1 {
		service.streamMutex.Unlock()
		writeFault(writer, request, &Fault{Status: 429, Code: "READY_ALERTS_CONNECTION_LIMIT",
			Message: "Este dispositivo ya tiene abierta su conexión de avisos."})
		return
	}
	service.readyStreams[arguments.DeviceID]++
	service.streamMutex.Unlock()
	defer func() {
		service.streamMutex.Lock()
		service.readyStreams[arguments.DeviceID]--
		service.streamMutex.Unlock()
	}()
	writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Accel-Buffering", "no")
	control := http.NewResponseController(writer)
	send := func(name string, value any, cursor int64) error {
		_ = control.SetWriteDeadline(time.Now().Add(10 * time.Second))
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\n\n", cursor, name, encoded); err != nil {
			return err
		}
		return control.Flush()
	}
	if err := send("alerts", initial, initial.Cursor); err != nil {
		return
	}
	arguments.Query.Set("after", strconv.FormatInt(initial.Cursor, 10))
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	lifetime := time.NewTimer(30 * time.Second)
	defer lifetime.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-service.Stopping:
			return
		case <-lifetime.C:
			return
		case <-ticker.C:
			batch, err := read()
			if err != nil {
				problem := publicFault(err)
				_ = send("interrupted", map[string]any{"error": problem.Code, "status": problem.Status}, 0)
				return
			}
			if err := send("alerts", batch, batch.Cursor); err != nil {
				return
			}
			arguments.Query.Set("after", strconv.FormatInt(batch.Cursor, 10))
		}
	}
}
