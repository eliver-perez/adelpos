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
	"local/ad-el-pos/server/internal/kitchen"
)

// Avisos de platillos listos (etapa A2, parte D; §4 de la especificación, decisiones A2-D4 y A2-D9 y las correcciones de
// la revisión independiente R1–R12). El teléfono o la tableta de meseros recibe lo que cocina marcó «Listo» para los
// meseros que entraron en él, aunque esté en «¿Quién eres?» o con la aplicación cerrada (lo mantiene su servicio en
// primer plano). Por eso estas rutas llevan solo la credencial del dispositivo, como el agente de impresión: sin sesión.
//
//   - GET ready-alerts?after=<cursor> es la lectura de recuperación y GET ready-alerts/stream su canal SSE (con
//     Last-Event-ID). El canal revisa la base cada 750 ms pero solo manda un mensaje cuando hay avisos nuevos o
//     cambian active, settings o day_ends_at; en silencio, un comentario «: ping» cada 15 s; dura 5 minutos. Un canal
//     nuevo del mismo dispositivo reemplaza al anterior (tras un cambio de red el viejo puede seguir medio abierto).
//   - Solo dispositivos android no revocados (revocado: 401 DEVICE_REVOKED; de otro tipo, 403).
//   - Go decide a quién llega: un aviso llega al dispositivo D si su destinatario M «está» en D (tiene en D una sesión
//     con PIN creada desde el inicio del día comercial, o una de antes que sigue vigente y se usó en los últimos 30
//     minutos, A2-D12: la misma regla que active) y el aviso es posterior a la entrada de M en D, y además el aviso es para D (al «Listo», D era el dispositivo de la sesión con PIN vigente
//     más reciente de M que podía recibirlo) o para todos (no había ninguno). Ver kitchen.recordReadyNotices.
//   - Solo los datos del aviso: cuenta (la vigente, si se combinó), modalidad, mesa o folio y nombre del cliente,
//     mesero, platillos y cantidades. Nunca precios ni importes; las unidades canceladas después no se avisan.
//   - El cursor es el último aviso que Go revisó para el dispositivo (le corresponda o no). key es la clave de cada
//     aviso: el dispositivo descarta los repetidos por ella (el número puede repetirse si se restaura un respaldo).

// readyAlertsGroups es cuántos avisos entrega una lectura como máximo; con más, el cursor es el del último entregado
// y el resto llega en la siguiente.
const readyAlertsGroups = 50

// Ritmo del canal: cada cuánto revisa la base, cada cuánto manda «: ping» si no hubo nada y cuánto dura.
const (
	readyStreamPoll     = 750 * time.Millisecond
	readyStreamPing     = 15 * time.Second
	readyStreamLifetime = 5 * time.Minute
)

// ReadyAlertsBatch es la respuesta de GET ready-alerts y cada mensaje «alerts» de su canal.
type ReadyAlertsBatch struct {
	Cursor int64 `json:"cursor"`
	// Active dice si el dispositivo debe mantener su servicio de avisos: el aviso está activado en Terminales y hay
	// un mesero que entró en él con PIN durante el día comercial en curso, o con una sesión con PIN de antes que sigue
	// vigente y se usó en los últimos 30 minutos (A2-D12).
	Active bool `json:"active"`
	// DayEndsAt es el fin del día comercial en curso (la hora de inicio del día siguiente).
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
// cocina, agrupadas por platillo. ID es el número del último aviso del grupo (el cursor) y Key su clave, que nunca se
// reutiliza.
type ReadyAlert struct {
	ID              int64       `json:"id"`
	Key             string      `json:"key"`
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

// readyPresenceSQL son las sesiones con PIN del dispositivo $1 que hacen que su mesero «esté» en él: creadas desde el
// inicio del día comercial $2, o vigentes y usadas en los últimos $3 segundos (kitchen.ReadyAlertsRecentUse, A2-D12;
// una de las del día que también lo cumpla sale dos veces, sin efecto en el mínimo). Dos consultas, cada una con su
// índice parcial (revisión R5).
const readyPresenceSQL = `SELECT user_id,created_at FROM app.sessions WHERE device_id=$1 AND opened_with_pin AND created_at>=$2
        UNION ALL
        SELECT user_id,created_at FROM app.sessions WHERE device_id=$1 AND opened_with_pin AND revoked_at IS NULL
            AND expires_at>now() AND last_seen_at>=now()-make_interval(secs=>$3)`

// readyAlertsQuery son los avisos del dispositivo $1 posteriores al cursor $4 y hasta $5 (lo último registrado al
// leer), del día comercial que empezó en $2, agrupados por su clave (la operación de cocina en una cuenta), hasta $6
// grupos ($3 es kitchen.ReadyAlertsRecentUse en segundos, para la presencia). Cada fila es un grupo: su último número,
// si le queda alguna unidad activa y el aviso. La cuenta del aviso sigue las combinaciones hasta la vigente (revisión
// R10).
const readyAlertsQuery = `WITH RECURSIVE presence AS (
        SELECT user_id,min(created_at) AS since FROM (` + readyPresenceSQL + `) counted GROUP BY user_id
    ), notices AS (
        SELECT n.sequence,n.alert_key,n.account_id,n.recipient_user_id,n.ready_at,u.state AS unit_state,
            coalesce(v.quote_line->>'name','')||coalesce(' · '||variant.name,'') AS product_name
        FROM app.ready_notices n
        JOIN presence p ON p.user_id=n.recipient_user_id AND n.ready_at>=p.since
        JOIN app.restaurant_item_units u ON u.id=n.unit_id
        JOIN app.restaurant_item_versions v ON v.id=n.version_id
        LEFT JOIN app.catalog_variants variant ON variant.id::text=v.quote_line->>'variant_id'
        WHERE n.sequence>$4 AND n.sequence<=$5 AND n.ready_at>=$2
            AND (n.recipient_device_id IS NULL OR n.recipient_device_id=$1)
    ), grouped AS (
        SELECT alert_key,max(sequence) AS last,(array_agg(account_id))[1] AS account_id,
            (array_agg(recipient_user_id))[1] AS recipient_user_id,max(ready_at) AS ready_at
        FROM notices GROUP BY alert_key ORDER BY max(sequence) LIMIT $6
    ), chain AS (
        SELECT g.alert_key,a.id,a.merged_into_id,0 AS depth FROM grouped g JOIN app.restaurant_accounts a ON a.id=g.account_id
        UNION ALL
        SELECT c.alert_key,a.id,a.merged_into_id,c.depth+1 FROM chain c JOIN app.restaurant_accounts a ON a.id=c.merged_into_id
        WHERE c.depth<20
    ), current_account AS (
        SELECT DISTINCT ON (alert_key) alert_key,id FROM chain ORDER BY alert_key,depth DESC
    )
    SELECT g.last,EXISTS(SELECT FROM notices x WHERE x.alert_key=g.alert_key AND x.unit_state='active'),
        jsonb_build_object('id',g.last,'key',g.alert_key,'account_id',a.id,'mode',a.mode,'table_name',t.name,
        'folio',a.folio,'customer_name',a.customer_name,'waiter_user_id',g.recipient_user_id,
        'waiter_short_name',coalesce(nullif(waiter.short_name,''),waiter.display_name),
        'items',coalesce((SELECT jsonb_agg(jsonb_build_object('product_name',item.product_name,'quantity',item.quantity)
                ORDER BY item.first_sequence)
            FROM (SELECT product_name,count(*)::integer AS quantity,min(sequence) AS first_sequence FROM notices item
                WHERE item.alert_key=g.alert_key AND item.unit_state='active' GROUP BY product_name) item),'[]'::jsonb),
        'ready_at',g.ready_at)
    FROM grouped g JOIN current_account c ON c.alert_key=g.alert_key JOIN app.restaurant_accounts a ON a.id=c.id
    LEFT JOIN app.restaurant_tables t ON t.id=a.table_id JOIN app.users waiter ON waiter.id=g.recipient_user_id
    ORDER BY g.last`

// readyAlerts atiende GET ready-alerts con la credencial del dispositivo, ya autenticada (authenticateDevice) y con el
// lock de la operación compartido ya tomado por dispatch antes de tocar la fila del dispositivo (revisión R8): cocina
// escribe los avisos con ese lock en exclusiva hasta su commit, así que ninguno queda a medias detrás del cursor.
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
	// principio del día comercial y el dispositivo descarta por key los que ya avisó.
	if after > head {
		after = 0
	}
	recent := kitchen.ReadyAlertsRecentUse.Seconds()
	batch := &ReadyAlertsBatch{Cursor: head, DayEndsAt: day.Deadline, Settings: settings, Alerts: []ReadyAlert{}}
	if settings.Enabled {
		// La misma regla que readyPresenceSQL (A2-D12): una sesión de antes del inicio del día solo cuenta si sigue
		// vigente y se usó hace poco; si no, la última entrada de la noche mantendría el servicio activo hasta la mañana.
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.sessions WHERE device_id=$1 AND opened_with_pin AND created_at>=$2)
            OR EXISTS(SELECT FROM app.sessions WHERE device_id=$1 AND opened_with_pin AND revoked_at IS NULL AND expires_at>now()
                AND last_seen_at>=now()-make_interval(secs=>$3))`,
			request.DeviceID, dayStart, recent).Scan(&batch.Active)
		if err != nil {
			return nil, err
		}
	}
	// Apagado o sin meseros: sin avisos (no es un error) y el cursor avanza, para que al volver a activarse o al
	// entrar un mesero no lleguen avisos viejos.
	if !batch.Active {
		return batch, nil
	}
	rows, err := tx.Query(ctx, readyAlertsQuery, request.DeviceID, dayStart, recent, after, head, readyAlertsGroups)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := 0
	last := int64(0)
	for rows.Next() {
		var active bool
		var encoded []byte
		if err := rows.Scan(&last, &active, &encoded); err != nil {
			return nil, err
		}
		groups++
		// Un grupo cuyas unidades se cancelaron todas después del «Listo» ya no se avisa, pero cuenta para el cursor.
		if !active {
			continue
		}
		var alert ReadyAlert
		if err := json.Unmarshal(encoded, &alert); err != nil {
			return nil, err
		}
		batch.Alerts = append(batch.Alerts, alert)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if groups == readyAlertsGroups {
		batch.Cursor = last
	}
	return batch, nil
}

// readyStream es un canal de avisos abierto; stop lo cierra cuando otro del mismo dispositivo lo reemplaza.
type readyStream struct {
	stop chan struct{}
}

// openReadyStream registra el canal del dispositivo y cierra el anterior, si lo había (revisión R12).
func (service *Service) openReadyStream(deviceID string) *readyStream {
	stream := &readyStream{stop: make(chan struct{})}
	service.streamMutex.Lock()
	defer service.streamMutex.Unlock()
	if service.readyStreams == nil {
		service.readyStreams = map[string]*readyStream{}
	}
	if previous := service.readyStreams[deviceID]; previous != nil {
		close(previous.stop)
	}
	service.readyStreams[deviceID] = stream
	return stream
}

// closeReadyStream lo quita del registro si sigue siendo el vigente del dispositivo.
func (service *Service) closeReadyStream(deviceID string, stream *readyStream) {
	service.streamMutex.Lock()
	defer service.streamMutex.Unlock()
	if service.readyStreams[deviceID] == stream {
		delete(service.readyStreams, deviceID)
	}
}

// sameReadyState dice si un lote sin avisos no cambia nada de lo que ya sabe el dispositivo.
func sameReadyState(previous, current *ReadyAlertsBatch) bool {
	return previous.Active == current.Active && previous.Settings == current.Settings && previous.DayEndsAt.Equal(current.DayEndsAt)
}

// interruptionCode es el código de «interrupted» (revisión R6): el del rechazo si el dispositivo ya no vale (401 o
// 403, por ejemplo DEVICE_REVOKED) o TEMPORARY si fue un tropiezo (la base no respondió a tiempo, 5xx): entonces el
// dispositivo solo se reconecta con espera creciente.
func interruptionCode(problem *Fault) string {
	if problem.Status >= 400 && problem.Status < 500 {
		return problem.Code
	}
	return "TEMPORARY"
}

// serveReadyAlerts es GET ready-alerts/stream. Revisa la base cada 750 ms con la misma lectura que GET ready-alerts,
// pero solo manda «alerts» (con id: <cursor>) cuando hay avisos nuevos o cambian active, settings o day_ends_at; si no,
// un comentario «: ping» cada 15 s para que el dispositivo sepa que la conexión sigue viva. Dura 5 minutos; al volver,
// Last-Event-ID (o after) continúa desde ahí. Si la lectura falla manda «interrupted» sin id (el cursor del dispositivo
// no cambia) con {"code": …} y termina.
func (service *Service) serveReadyAlerts(writer http.ResponseWriter, request *http.Request, local bool) {
	arguments := Request{Method: "GET", Path: "/api/v1/ready-alerts", Query: request.URL.Query(),
		DeviceID: request.Header.Get("X-Device-ID"), DeviceToken: request.Header.Get("X-Device-Token"), Local: local,
		OfflineCredential: request.Header.Get(OfflineCredentialHeader),
		ClientKind:        request.Header.Get(httpapi.ClientKindHeader),
		ClientVersion:     request.Header.Get(httpapi.ClientVersionHeader)}
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
	stream := service.openReadyStream(arguments.DeviceID)
	defer service.closeReadyStream(arguments.DeviceID, stream)
	writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Accel-Buffering", "no")
	control := http.NewResponseController(writer)
	write := func(text string) error {
		_ = control.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := fmt.Fprint(writer, text); err != nil {
			return err
		}
		return control.Flush()
	}
	send := func(batch *ReadyAlertsBatch) error {
		encoded, err := json.Marshal(batch)
		if err != nil {
			return err
		}
		return write(fmt.Sprintf("id: %d\nevent: alerts\ndata: %s\n\n", batch.Cursor, encoded))
	}
	if err := send(initial); err != nil {
		return
	}
	known := initial
	lastWrite := time.Now()
	arguments.Query.Set("after", strconv.FormatInt(initial.Cursor, 10))
	ticker := time.NewTicker(readyStreamPoll)
	defer ticker.Stop()
	lifetime := time.NewTimer(readyStreamLifetime)
	defer lifetime.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-service.Stopping:
			return
		case <-stream.stop:
			// Otro canal del mismo dispositivo lo reemplazó.
			return
		case <-lifetime.C:
			return
		case <-ticker.C:
			batch, err := read()
			if err != nil {
				encoded, _ := json.Marshal(map[string]string{"code": interruptionCode(publicFault(err))})
				_ = write(fmt.Sprintf("event: interrupted\ndata: %s\n\n", encoded))
				return
			}
			// El cursor avanza aunque no se mande nada (avisos de otros dispositivos): la siguiente lectura sigue de ahí.
			arguments.Query.Set("after", strconv.FormatInt(batch.Cursor, 10))
			if len(batch.Alerts) > 0 || !sameReadyState(known, batch) {
				if err := send(batch); err != nil {
					return
				}
				known, lastWrite = batch, time.Now()
				continue
			}
			if time.Since(lastWrite) >= readyStreamPing {
				if err := write(": ping\n\n"); err != nil {
					return
				}
				lastWrite = time.Now()
			}
		}
	}
}
