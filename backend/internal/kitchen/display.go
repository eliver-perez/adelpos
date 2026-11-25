package kitchen

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/stationscope"
)

// Pantalla de cocina para Android (etapa K1, Fase A; §2 a §4 de la especificación y decisiones K1-D4, K1-D5, K1-D7,
// K1-D8, K1-D11, K1-D12 y K1-D14 del coordinador).
//
// La pantalla es un dispositivo kitchen sin usuario ni sesión. administration la autentica por su credencial y solo
// la deja llegar a su lista cerrada de rutas (administration/kitchen_display.go); aquí está la lógica de cocina, que
// reutiliza la de la 9F sin copiar sus reglas:
//
//   - el alcance: las estaciones de la pantalla (stationscope, con la pantalla como terminal y sin usuario) que reciben
//     comandas en pantalla (recepción «kds» o «both», K1-D11);
//   - marcar listo: advanceTasks de states.go, con las mismas validaciones (estación visible, versión vigente, nada
//     cancelado) y la misma publicación (kitchen.state.changed por estación, la preparación de la cuenta y, con ella, el
//     aviso de platillos listos de la A2: recordReadyNotices);
//   - el acuse de recepción: recordReceipt de reads.go (K1-D12);
//   - los renglones de cada parte: documents.KitchenPartDetails, los mismos de la comanda impresa.
//
// Lo nuevo de la K1 es la tarjeta de pedido (K1-D5), «Recuperar» (K1-D8) y dar por vista una cancelación. Las marcas
// son idempotentes por estado y versión (K1-D4): repetir una marca no cambia nada, y una tarea cuya versión cambió no se
// marca y se informa. No usan restaurant_operations (su actor es un usuario); el historial va sin usuario y con el
// dispositivo.

// DisplayPermissions son los permisos de la pantalla de cocina: una lista cerrada en código (K1-D4), nunca de un rol,
// para que nadie pueda ampliarlos desde Personal. Ver preparación, avanzarla y recibir eventos; nada más.
var DisplayPermissions = []string{"kitchen.states.read", "kitchen.states.manage", "events.read"}

// DisplayConfigurationChangedKind avisa a las pantallas de cocina que cambió su configuración (K1-D7): sus estaciones,
// su letra, tema o sonido, su cupo, o la configuración general de cocina (límites de color, estaciones, seguimiento).
// Va en el ámbito «configuration» con la pantalla como agregado, o sin agregado si es para todas. No lleva datos.
const DisplayConfigurationChangedKind = "kitchen_display.configuration.changed"

// DisplayRecentWindow es la ventana de «Recientes» y de «Recuperar» (§4 y K1-D8), y cuánto se queda en pantalla una
// tarjeta cerrada por una cancelación que nadie marcó.
const DisplayRecentWindow = 30 * time.Minute

// displayCardLimit es el máximo de tarjetas de una lectura.
const displayCardLimit = 200

// displayTaskLimit es el máximo de tareas de una marca, como en POST kitchen/states.
const displayTaskLimit = 1000

// displayReceiptLimit es el máximo de acuses de una petición.
const displayReceiptLimit = 500

// displayKitchenKinds son los eventos de cocina por estación que recibe la pantalla (K1-D7): pedido nuevo, agregado,
// modificación o cancelación (la comanda), una marca o una recuperación, una modificación pendiente y un acuse.
var displayKitchenKinds = map[string]bool{
	"kitchen.ticket.created": true,
	"kitchen.state.changed":  true,
	"kitchen.change.pending": true,
	"kitchen.kds.received":   true,
}

// displayAccountKinds son los cambios de una cuenta que cambian el encabezado de su tarjeta (K1-D7): datos de la
// cuenta (nombre, cliente), mesa, mesero, combinar, dividir, transferir y cancelar la cuenta. La pantalla los recibe
// solo de las cuentas con tareas en sus estaciones y sin contenido.
var displayAccountKinds = map[string]bool{
	"restaurant.account.updated":   true,
	"restaurant.account.move":      true,
	"restaurant.account.assign":    true,
	"restaurant.account.merged":    true,
	"restaurant.account.split":     true,
	"restaurant.account.cancelled": true,
	"restaurant.items.transferred": true,
}

// AppendDisplayConfigurationChanged publica kitchen_display.configuration.changed para una pantalla (deviceID) o para
// todas (nil).
func AppendDisplayConfigurationChanged(ctx context.Context, tx pgx.Tx, deviceID *string) error {
	return events.Append(ctx, tx, DisplayConfigurationChangedKind, "configuration", deviceID, nil)
}

// DisplayStation es una estación que muestra la pantalla, con su seguimiento efectivo: «item» (se toca cada platillo)
// u «order» (solo el pedido completo, K1-D14).
type DisplayStation struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Tracking string `json:"tracking"`
	order    int
}

// DisplayConfig es la configuración de la pantalla: la eligen Terminales (letra, tema, sonido, estaciones) y la
// configuración general de cocina (límites de color).
type DisplayConfig struct {
	DeviceID       string           `json:"device_id"`
	Name           string           `json:"name"`
	TextSize       string           `json:"text_size"`
	Theme          string           `json:"theme"`
	SoundEnabled   bool             `json:"sound_enabled"`
	SoundVolume    string           `json:"sound_volume"`
	WarningMinutes int              `json:"warning_minutes"`
	LateMinutes    int              `json:"late_minutes"`
	AllStations    bool             `json:"all_stations"`
	Stations       []DisplayStation `json:"stations"`
}

// LoadDisplayConfig lee la configuración de la pantalla de cocina.
func LoadDisplayConfig(ctx context.Context, tx pgx.Tx, deviceID string) (*DisplayConfig, error) {
	config := &DisplayConfig{DeviceID: deviceID}
	err := tx.QueryRow(ctx, `SELECT d.name,d.kitchen_display_text_size,d.kitchen_display_theme,d.kitchen_display_sound,
            d.kitchen_display_volume,d.kitchen_all_stations,cfg.display_warning_minutes,cfg.display_late_minutes
        FROM app.devices d CROSS JOIN app.kitchen_settings cfg WHERE d.id=$1`, deviceID).Scan(&config.Name, &config.TextSize,
		&config.Theme, &config.SoundEnabled, &config.SoundVolume, &config.AllStations, &config.WarningMinutes,
		&config.LateMinutes)
	if err != nil {
		return nil, err
	}
	_, stations, err := loadDisplayScope(ctx, tx, deviceID)
	if err != nil {
		return nil, err
	}
	config.Stations = stations
	return config, nil
}

// loadDisplayScope es el alcance de la pantalla: las estaciones que muestra la terminal (todas o su selección; no hay
// usuario, así que el lado del usuario es «todas») que reciben comandas en pantalla (K1-D11), en el orden de Cocina.
func loadDisplayScope(ctx context.Context, tx pgx.Tx, deviceID string) (*stationscope.Scope, []DisplayStation, error) {
	base, err := stationscope.Load(ctx, tx, deviceID, "", true)
	if err != nil {
		return nil, nil, err
	}
	rows, err := tx.Query(ctx, `SELECT s.id::text,s.name,CASE WHEN s.tracking='inherit' THEN cfg.tracking ELSE s.tracking END
        FROM app.kitchen_stations s CROSS JOIN app.kitchen_settings cfg
        WHERE s.reception IN ('kds','both') ORDER BY s.sort_order,s.name,s.id`)
	if err != nil {
		return nil, nil, err
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (DisplayStation, error) {
		var station DisplayStation
		err := row.Scan(&station.ID, &station.Name, &station.Tracking)
		return station, err
	})
	if err != nil {
		return nil, nil, err
	}
	stations := []DisplayStation{}
	ids := []string{}
	for _, station := range candidates {
		if base.Allows(station.ID) {
			station.order = len(stations)
			stations = append(stations, station)
			ids = append(ids, station.ID)
		}
	}
	return base.Within(ids), stations, nil
}

// DisplayRequest es una petición de la pantalla de cocina ya autenticada: método, ruta sin el prefijo de la API
// («kitchen-display/…»), cuerpo y el dispositivo.
type DisplayRequest struct {
	Method   string
	Route    string
	Body     json.RawMessage
	DeviceID string
}

// HandleDisplay atiende las rutas de cocina de la pantalla: GET kitchen-display/orders y recent, y POST
// kitchen-display/items/ready, orders/ready, recover y receipts. Las escrituras toman 7820148305 exclusivo, como toda
// escritura de cocina (K1-D19). audit escribe en el Registro de movimientos con la pantalla como autor.
func HandleDisplay(ctx context.Context, tx pgx.Tx, request DisplayRequest, audit Audit) (any, error) {
	if err := validID(request.DeviceID); err != nil {
		return nil, err
	}
	action := strings.TrimPrefix(request.Route, "kitchen-display/")
	write := request.Method == "POST" && (action == "items/ready" || action == "orders/ready" || action == "recover" ||
		action == "receipts")
	read := request.Method == "GET" && (action == "orders" || action == "recent")
	if !write && !read {
		return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "La ruta de la pantalla de cocina no existe."}
	}
	if write {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7820148305)"); err != nil {
			return nil, err
		}
	}
	scope, stations, err := loadDisplayScope(ctx, tx, request.DeviceID)
	if err != nil {
		return nil, err
	}
	display := &displayService{
		service: service{ctx: ctx, tx: tx, audit: audit, preparation: map[string]bool{}, scope: scope,
			request: Request{Method: request.Method, Route: request.Route, Body: request.Body,
				Actor: Actor{DeviceID: request.DeviceID, Permissions: DisplayPermissions}}},
		stations: map[string]DisplayStation{},
	}
	for _, station := range stations {
		display.stations[station.ID] = station
	}
	switch action {
	case "orders":
		return display.orders()
	case "recent":
		return display.recent()
	}
	var result any
	switch action {
	case "items/ready":
		result, err = display.markReady(false)
	case "orders/ready":
		result, err = display.markReady(true)
	case "recover":
		result, err = display.recover()
	case "receipts":
		result, err = display.receipts()
	}
	if err != nil {
		return nil, err
	}
	// La preparación de las cuentas cambió: el mesero se entera y, si quedó lista, recibe su aviso (preparation.go).
	return result, display.publishPreparation()
}

type displayService struct {
	service
	stations map[string]DisplayStation
}

// --- Tarjetas (K1-D5) -------------------------------------------------------------------------------------------------

// displayTask es una tarea de cocina con lo que la tarjeta necesita de su unidad, su cuenta y su liberación.
type displayTask struct {
	ID              string       `json:"id"`
	UnitID          string       `json:"unit_id"`
	ReleaseID       string       `json:"release_id"`
	StationID       string       `json:"station_id"`
	VersionID       string       `json:"version_id"`
	State           string       `json:"state"`
	Instructions    Instructions `json:"instructions"`
	ReleasedAt      time.Time    `json:"released_at"`
	StartedAt       *time.Time   `json:"started_at"`
	ReadyAt         *time.Time   `json:"ready_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
	AccountID       string       `json:"account_id"`
	UnitState       string       `json:"unit_state"`
	UnitVersionID   string       `json:"unit_version_id"`
	UnitCancelledAt *time.Time   `json:"unit_cancelled_at"`
	ItemID          string       `json:"item_id"`
	Ordinal         int          `json:"ordinal"`
	Employee        string       `json:"employee"`
	DismissedAt     *time.Time   `json:"dismissed_at"`
}

// displayTaskColumns son las columnas de displayTask (t: la tarea, u: su unidad, r: su liberación).
const displayTaskColumns = `jsonb_build_object('id',t.id,'unit_id',t.unit_id,'release_id',t.release_id,
        'station_id',t.station_id,'version_id',t.version_id,'state',t.state,'instructions',t.instructions,
        'released_at',t.released_at,'started_at',t.started_at,'ready_at',t.ready_at,'updated_at',t.updated_at,
        'account_id',u.account_id,'unit_state',u.state,'unit_version_id',u.version_id,'unit_cancelled_at',u.cancelled_at,
        'item_id',u.item_id,'ordinal',u.ordinal,'employee',coalesce(r.context_snapshot->>'employee',''),
        'dismissed_at',(SELECT dismissal.dismissed_at FROM app.kitchen_display_dismissals dismissal WHERE dismissal.task_id=t.id))
    FROM app.kitchen_tasks t JOIN app.restaurant_item_units u ON u.id=t.unit_id JOIN app.kitchen_releases r ON r.id=t.release_id`

// cancelled: la tarea o su unidad se cancelaron (el tablero de la 9F muestra igual una unidad cancelada cuya tarea
// todavía no se procesó).
func (task displayTask) cancelled() bool {
	return task.State == "cancelled" || task.UnitState == "cancelled"
}

// cancelledAt es cuándo se canceló: la última escritura de la tarea cancelada (una tarea cancelada no se vuelve a
// tocar) o la cancelación de su unidad.
func (task displayTask) cancelledAt() time.Time {
	if task.State != "cancelled" && task.UnitCancelledAt != nil {
		return *task.UnitCancelledAt
	}
	return task.UpdatedAt
}

// pending: por preparar o en preparación, de una unidad activa.
func (task displayTask) pending() bool {
	return !task.cancelled() && (task.State == "released" || task.State == "preparing")
}

// ready: lista y de una unidad activa.
func (task displayTask) ready() bool {
	return !task.cancelled() && task.State == "ready"
}

// closedAt es cuándo dejó de estar pendiente: al quedar lista o, si nunca lo estuvo, al cancelarse.
func (task displayTask) closedAt() (time.Time, bool) {
	if task.pending() {
		return time.Time{}, false
	}
	if task.ReadyAt != nil {
		return *task.ReadyAt, true
	}
	return task.cancelledAt(), true
}

// dismissed: una pantalla ya dio por vista esta cancelación (después de que ocurrió).
func (task displayTask) dismissed() bool {
	return task.DismissedAt != nil && !task.DismissedAt.Before(task.cancelledAt())
}

// pendingChange: la unidad tiene una versión que cocina todavía no recibió (una modificación pendiente).
func (task displayTask) pendingChange() bool {
	return task.UnitVersionID != task.VersionID
}

// currentRound es la ronda en curso de una cuenta (tareas ordenadas por llegada): una ronda nueva empieza con una
// tarea que llega cuando todo lo anterior de la cuenta en estas estaciones ya estaba listo o cancelado (K1-D5). Así lo
// agregado mientras la tarjeta sigue abierta entra en ella («NUEVO») y lo que llega después de cerrarla abre otra.
func currentRound(tasks []displayTask) []displayTask {
	start := 0
	allClosed := true
	var lastClosed time.Time
	for index, task := range tasks {
		if index > 0 && allClosed && !lastClosed.After(task.ReleasedAt) {
			start = index
		}
		closedAt, closed := task.closedAt()
		if !closed {
			allClosed = false
			continue
		}
		if closedAt.After(lastClosed) {
			lastClosed = closedAt
		}
	}
	return tasks[start:]
}

// roundStatus dice si la ronda se muestra como tarjeta y con qué estado:
//   - «open» si queda algo pendiente;
//   - «cancelled» si lo último que pasó fue cancelar lo que faltaba, esa cancelación es de los últimos 30 minutos y
//     ninguna pantalla la dio por vista (§4: lo cancelado no desaparece «hasta que se marque el pedido»);
//   - nada (se cierra sola) si lo último fue marcar listo.
func roundStatus(round []displayTask, now time.Time) (string, bool) {
	for _, task := range round {
		if task.pending() {
			return "open", true
		}
	}
	var lastReady time.Time
	for _, task := range round {
		if task.ReadyAt != nil && task.ReadyAt.After(lastReady) {
			lastReady = *task.ReadyAt
		}
	}
	for _, task := range round {
		if !task.cancelled() || task.ReadyAt != nil || task.dismissed() {
			continue
		}
		at := task.cancelledAt()
		if at.After(now.Add(-DisplayRecentWindow)) && !at.Before(lastReady) {
			return "cancelled", true
		}
	}
	return "", false
}

// DisplayTaskRef es una tarea tal como la manda la pantalla al marcar o recuperar: su identificador y la versión que
// vio (K1-D4: si cambió, no se marca).
type DisplayTaskRef struct {
	ID        string `json:"id"`
	VersionID string `json:"version_id"`
}

// DisplayPart es una parte de un platillo con sus renglones, los de la comanda impresa. ShowName dice si su nombre
// aporta algo (un componente de combo, una variante o un nombre distinto del platillo).
type DisplayPart struct {
	Name     string                    `json:"name"`
	Quantity int                       `json:"quantity"`
	ShowName bool                      `json:"show_name"`
	Lines    []documents.KitchenDetail `json:"lines"`
}

// DisplayItem es un platillo de una tarjeta: las unidades iguales de una partida en una estación, del mismo envío y el
// mismo estado.
type DisplayItem struct {
	ID            string           `json:"id"`
	StationID     string           `json:"station_id"`
	StationName   string           `json:"station_name"`
	Name          string           `json:"name"`
	Quantity      int              `json:"quantity"`
	Status        string           `json:"status"`
	Preparing     bool             `json:"preparing"`
	IsNew         bool             `json:"is_new"`
	PendingChange bool             `json:"pending_change"`
	Tracking      string           `json:"tracking"`
	Notes         string           `json:"notes"`
	Parts         []DisplayPart    `json:"parts"`
	ReadyAt       *time.Time       `json:"ready_at"`
	Tasks         []DisplayTaskRef `json:"tasks"`
	sortKey       displaySortKey
}

type displaySortKey struct {
	releasedAt time.Time
	station    int
	ordinal    int
	id         string
}

func (key displaySortKey) less(other displaySortKey) bool {
	if !key.releasedAt.Equal(other.releasedAt) {
		return key.releasedAt.Before(other.releasedAt)
	}
	if key.station != other.station {
		return key.station < other.station
	}
	if key.ordinal != other.ordinal {
		return key.ordinal < other.ordinal
	}
	return key.id < other.id
}

// displayHeader es el encabezado de la tarjeta (la cuenta).
type displayHeader struct {
	ID           string  `json:"id"`
	Folio        int64   `json:"folio"`
	Mode         string  `json:"mode"`
	Name         string  `json:"name"`
	CustomerName string  `json:"customer_name"`
	TableName    *string `json:"table_name"`
	WaiterName   string  `json:"waiter_name"`
}

// DisplayCardHeader son los datos de la cuenta que muestra una tarjeta.
type DisplayCardHeader struct {
	AccountID    string  `json:"account_id"`
	Folio        int64   `json:"folio"`
	Mode         string  `json:"mode"`
	Title        string  `json:"title"`
	Subtitle     string  `json:"subtitle"`
	TableName    *string `json:"table_name"`
	AccountName  string  `json:"account_name"`
	CustomerName string  `json:"customer_name"`
	WaiterName   string  `json:"waiter_name"`
}

// DisplayCard es una tarjeta de pedido: una cuenta en las estaciones de la pantalla, en su ronda en curso.
type DisplayCard struct {
	ID string `json:"id"`
	DisplayCardHeader
	SentAt         time.Time     `json:"sent_at"`
	ElapsedSeconds int64         `json:"elapsed_seconds"`
	Status         string        `json:"status"`
	Items          []DisplayItem `json:"items"`
}

// DisplayReceipt es una comanda de una estación que la pantalla muestra y nadie ha confirmado.
type DisplayReceipt struct {
	TicketID  string `json:"ticket_id"`
	StationID string `json:"station_id"`
}

// DisplayOrders es el estado completo de las tarjetas (GET kitchen-display/orders). administration le agrega el cursor
// de eventos (leído antes), la configuración y la licencia.
type DisplayOrders struct {
	ServerTime      time.Time        `json:"server_time"`
	Orders          []DisplayCard    `json:"orders"`
	ReceiptsPending []DisplayReceipt `json:"receipts_pending"`
}

// DisplayRecentOrder es una cuenta con lo marcado listo en los últimos 30 minutos.
type DisplayRecentOrder struct {
	DisplayCardHeader
	MarkedAt time.Time     `json:"marked_at"`
	Items    []DisplayItem `json:"items"`
}

// DisplayRecent es GET kitchen-display/recent.
type DisplayRecent struct {
	ServerTime    time.Time            `json:"server_time"`
	WindowMinutes int                  `json:"window_minutes"`
	Orders        []DisplayRecentOrder `json:"orders"`
}

// databaseNow es la hora de la transacción en PostgreSQL: la misma con que se escriben released_at, ready_at y las
// cancelaciones que la tarjeta compara.
func (display *displayService) databaseNow() (time.Time, error) {
	var now time.Time
	err := display.tx.QueryRow(display.ctx, "SELECT now()").Scan(&now)
	return now.UTC(), err
}

// orders arma las tarjetas: las cuentas con algo pendiente o con una cancelación reciente en las estaciones de la
// pantalla, cada una con su ronda en curso, de la más antigua a la más nueva.
func (display *displayService) orders() (*DisplayOrders, error) {
	now, err := display.databaseNow()
	if err != nil {
		return nil, err
	}
	result := &DisplayOrders{ServerTime: now, Orders: []DisplayCard{}, ReceiptsPending: []DisplayReceipt{}}
	stations := display.scope.IDs()
	if len(stations) == 0 {
		return result, nil
	}
	accounts, err := many[string](display.ctx, display.tx, `SELECT DISTINCT to_jsonb(u.account_id::text)
        FROM app.kitchen_tasks t JOIN app.restaurant_item_units u ON u.id=t.unit_id
        WHERE t.station_id = ANY($1::uuid[]) AND (t.state IN ('released','preparing')
            OR (t.state='cancelled' AND t.updated_at > now() - make_interval(secs => $2)))`,
		stations, DisplayRecentWindow.Seconds())
	if err != nil || len(accounts) == 0 {
		return result, err
	}
	tasks, err := many[displayTask](display.ctx, display.tx, `SELECT `+displayTaskColumns+`
        WHERE u.account_id = ANY($1::uuid[]) AND t.station_id = ANY($2::uuid[])
        ORDER BY u.account_id,t.released_at,t.id`, accounts, stations)
	if err != nil {
		return nil, err
	}
	headers, err := display.headers(accounts)
	if err != nil {
		return nil, err
	}
	releases := map[string]bool{}
	for _, accountTasks := range groupByAccount(tasks) {
		round := currentRound(accountTasks)
		status, visible := roundStatus(round, now)
		if !visible {
			continue
		}
		header, found := headers[round[0].AccountID]
		if !found {
			continue
		}
		sentAt := round[0].ReleasedAt
		card := DisplayCard{
			ID:                round[0].ID,
			DisplayCardHeader: cardHeader(header, round[0].Employee),
			SentAt:            sentAt,
			ElapsedSeconds:    max(0, int64(now.Sub(sentAt)/time.Second)),
			Status:            status,
			Items:             display.items(round, sentAt),
		}
		for _, task := range round {
			releases[task.ReleaseID] = true
		}
		result.Orders = append(result.Orders, card)
	}
	sort.SliceStable(result.Orders, func(first, second int) bool {
		left, right := result.Orders[first], result.Orders[second]
		if !left.SentAt.Equal(right.SentAt) {
			return left.SentAt.Before(right.SentAt)
		}
		return left.AccountID < right.AccountID
	})
	if len(result.Orders) > displayCardLimit {
		result.Orders = result.Orders[:displayCardLimit]
	}
	if len(releases) == 0 {
		return result, nil
	}
	// Las comandas de lo que se muestra que ninguna pantalla ha confirmado (la misma regla de «recepción pendiente» de
	// Comandas, reads.go): la pantalla las confirma cuando ya las pintó (K1-D12).
	result.ReceiptsPending, err = many[DisplayReceipt](display.ctx, display.tx, `SELECT jsonb_build_object(
            'ticket_id',t.id,'station_id',section->'station'->>'id')
        FROM app.kitchen_tickets t CROSS JOIN LATERAL jsonb_array_elements(t.document->'sections') section
        WHERE t.release_id = ANY($1::uuid[]) AND t.kind IN ('normal','modification','cancellation')
            AND section->'station'->>'reception' IN ('kds','both')
            AND section->'station'->>'id' = ANY($2::text[])
            AND NOT EXISTS (SELECT FROM app.kitchen_kds_receipts k WHERE k.ticket_id=t.id
                AND k.station_id=(section->'station'->>'id')::uuid)
        ORDER BY t.created_at,t.id LIMIT 500`, sortedKeys(releases), stations)
	return result, err
}

// groupByAccount separa las tareas (ordenadas por cuenta y llegada) por cuenta, en el mismo orden.
func groupByAccount(tasks []displayTask) [][]displayTask {
	groups := [][]displayTask{}
	for index := 0; index < len(tasks); {
		end := index
		for end < len(tasks) && tasks[end].AccountID == tasks[index].AccountID {
			end++
		}
		groups = append(groups, tasks[index:end])
		index = end
	}
	return groups
}

// headers lee el encabezado de las cuentas: folio, modo, nombre, cliente, mesa y su responsable actual (el que recibe
// el aviso de listo, K1-D5).
func (display *displayService) headers(accounts []string) (map[string]displayHeader, error) {
	rows, err := many[displayHeader](display.ctx, display.tx, `SELECT jsonb_build_object('id',a.id,'folio',a.folio,
            'mode',a.mode,'name',a.name,'customer_name',a.customer_name,'table_name',tb.name,
            'waiter_name',coalesce(person.display_name,''))
        FROM app.restaurant_accounts a LEFT JOIN app.restaurant_tables tb ON tb.id=a.table_id
        LEFT JOIN app.users person ON person.id=a.assigned_user_id WHERE a.id = ANY($1::uuid[])`, accounts)
	if err != nil {
		return nil, err
	}
	headers := map[string]displayHeader{}
	for _, row := range rows {
		headers[row.ID] = row
	}
	return headers, nil
}

// cardHeader arma el encabezado de la tarjeta: la mesa (o el nombre de la cuenta si no tiene) o «Para llevar» con el
// nombre del cliente (§4). El mesero es el responsable de la cuenta o, si no tiene nombre, quien envió (K1-D5).
func cardHeader(header displayHeader, sender string) DisplayCardHeader {
	result := DisplayCardHeader{AccountID: header.ID, Folio: header.Folio, Mode: header.Mode, TableName: header.TableName,
		AccountName: header.Name, CustomerName: header.CustomerName, WaiterName: header.WaiterName}
	if strings.TrimSpace(result.WaiterName) == "" {
		result.WaiterName = sender
	}
	switch {
	case header.Mode == "takeaway":
		result.Title = "Para llevar"
		result.Subtitle = header.Name
		if strings.TrimSpace(header.CustomerName) != "" {
			result.Subtitle = header.CustomerName
		}
	case header.TableName != nil && strings.TrimSpace(*header.TableName) != "":
		result.Title = *header.TableName
		if !sameDisplayName(header.Name, *header.TableName) {
			result.Subtitle = header.Name
		}
	default:
		result.Title = header.Name
	}
	return result
}

// sameDisplayName compara dos nombres sin distinguir mayúsculas ni espacios de los extremos (como el escritorio, para
// no repetir «Mesa 2 · Mesa 2»).
func sameDisplayName(first, second string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(first), " "), strings.Join(strings.Fields(second), " "))
}

// items agrupa las tareas de una ronda en platillos: las unidades iguales de una partida en una estación, del mismo
// envío, la misma versión y el mismo estado, en el orden en que llegaron. «NUEVO» es lo que llegó después del primer
// envío de la ronda.
func (display *displayService) items(round []displayTask, sentAt time.Time) []DisplayItem {
	items := []DisplayItem{}
	index := map[string]int{}
	for _, task := range round {
		status := "pending"
		switch {
		case task.cancelled():
			status = "cancelled"
		case task.ready():
			status = "ready"
		}
		isNew := task.ReleasedAt.After(sentAt)
		arrival := "first"
		if isNew {
			arrival = "new"
		}
		key := strings.Join([]string{task.ReleaseID, task.StationID, task.ItemID, task.VersionID, status, arrival}, "|")
		position, exists := index[key]
		if !exists {
			station := display.stations[task.StationID]
			item := DisplayItem{
				ID:            task.ID,
				StationID:     task.StationID,
				StationName:   station.Name,
				Name:          task.Instructions.Name,
				Status:        status,
				IsNew:         isNew,
				PendingChange: status == "pending" && task.pendingChange(),
				Tracking:      station.Tracking,
				Notes:         task.Instructions.Notes,
				Parts:         displayParts(task.Instructions),
				Tasks:         []DisplayTaskRef{},
				sortKey: displaySortKey{releasedAt: task.ReleasedAt, station: station.order, ordinal: task.Ordinal,
					id: task.ID},
			}
			items = append(items, item)
			position = len(items) - 1
			index[key] = position
		}
		item := &items[position]
		item.Quantity++
		item.Tasks = append(item.Tasks, DisplayTaskRef{ID: task.ID, VersionID: task.VersionID})
		if task.State == "preparing" && status == "pending" {
			item.Preparing = true
		}
		if status == "ready" && task.ReadyAt != nil && (item.ReadyAt == nil || task.ReadyAt.After(*item.ReadyAt)) {
			readyAt := *task.ReadyAt
			item.ReadyAt = &readyAt
		}
	}
	sort.SliceStable(items, func(first, second int) bool {
		return items[first].sortKey.less(items[second].sortKey)
	})
	return items
}

// displayParts son las partes de un platillo con los renglones de la comanda impresa (documents.KitchenPartDetails),
// siempre con las notas: en la pantalla se ven bien.
func displayParts(instructions Instructions) []DisplayPart {
	parts := []DisplayPart{}
	for _, part := range instructions.Parts {
		converted := kitchenPart(part)
		parts = append(parts, DisplayPart{
			Name:     documents.KitchenPartName(converted),
			Quantity: part.Quantity,
			ShowName: part.Path != "root" || part.Variant != "" || !sameDisplayName(part.Name, instructions.Name),
			Lines:    documents.KitchenPartDetails(converted, instructions.Notes, true),
		})
	}
	return parts
}

// --- Marcar listo -----------------------------------------------------------------------------------------------------

// displaySkip es una tarea que no se marcó o no se recuperó, con el motivo (lista cerrada del contrato).
type displaySkip struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// displayRefs lee {tasks: [{id, version_id}]}: entre 1 y 1000, sin repetir.
func (display *displayService) displayRefs() ([]DisplayTaskRef, error) {
	var input struct {
		Tasks []DisplayTaskRef `json:"tasks"`
	}
	if err := decode(display.request.Body, &input); err != nil {
		return nil, err
	}
	if len(input.Tasks) < 1 || len(input.Tasks) > displayTaskLimit {
		return nil, invalid("Selecciona entre 1 y 1000 unidades de preparación.")
	}
	seen := map[string]bool{}
	for _, reference := range input.Tasks {
		if err := validID(reference.ID); err != nil {
			return nil, err
		}
		if err := validID(reference.VersionID); err != nil {
			return nil, err
		}
		if seen[reference.ID] {
			return nil, invalid("La unidad está repetida.")
		}
		seen[reference.ID] = true
	}
	return input.Tasks, nil
}

// loadRefs lee las tareas de la petición y exige que todas sean de las estaciones de la pantalla (si una no lo es, no
// se toca ninguna: 403 STATION_SCOPE_DENIED).
func (display *displayService) loadRefs(references []DisplayTaskRef) ([]displayTask, error) {
	ids := make([]string, 0, len(references))
	for _, reference := range references {
		ids = append(ids, reference.ID)
	}
	found, err := many[displayTask](display.ctx, display.tx, `SELECT `+displayTaskColumns+` WHERE t.id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	byID := map[string]displayTask{}
	for _, task := range found {
		byID[task.ID] = task
	}
	tasks := make([]displayTask, 0, len(references))
	for _, reference := range references {
		task, exists := byID[reference.ID]
		if !exists {
			return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "Ese platillo ya no existe. Actualiza la pantalla."}
		}
		if !display.scope.Allows(task.StationID) {
			return nil, stationDenied()
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// markReady atiende items/ready (order false) y orders/ready (order true). Lo pendiente se marca listo con
// advanceTasks; con order, en una estación «por pedido» se marca el pedido completo de esa estación (como hoy) y las
// cancelaciones se dan por vistas. Lo que ya estaba listo, lo cancelado (en items/ready) y lo que cambió se informa en
// skipped y no impide marcar lo demás.
func (display *displayService) markReady(order bool) (any, error) {
	references, err := display.displayRefs()
	if err != nil {
		return nil, err
	}
	tasks, err := display.loadRefs(references)
	if err != nil {
		return nil, err
	}
	if !order {
		// K1-D14: en una estación «por pedido» no se marca un platillo suelto (la regla de POST kitchen/states).
		for _, task := range tasks {
			if display.stations[task.StationID].Tracking == "order" {
				return nil, invalid("Esta estación actualiza el pedido completo; utiliza su acción por pedido.")
			}
		}
	}
	skipped := []displaySkip{}
	toMark := []displayTask{}
	toDismiss := []displayTask{}
	for index, task := range tasks {
		reference := references[index]
		switch {
		case task.VersionID != reference.VersionID:
			skipped = append(skipped, displaySkip{ID: task.ID, Reason: "changed"})
		case task.cancelled():
			if order && task.ReadyAt == nil {
				if !task.dismissed() {
					toDismiss = append(toDismiss, task)
				}
				continue
			}
			if !order {
				skipped = append(skipped, displaySkip{ID: task.ID, Reason: "cancelled"})
			}
		case task.pendingChange():
			skipped = append(skipped, displaySkip{ID: task.ID, Reason: "changed"})
		case task.State == "ready":
			skipped = append(skipped, displaySkip{ID: task.ID, Reason: "already_ready"})
		default:
			toMark = append(toMark, task)
		}
	}
	if order {
		toMark, err = display.expandOrderTracking(toMark)
		if err != nil {
			return nil, err
		}
	}
	updated := 0
	if len(toMark) > 0 {
		marked, err := display.tasks(toMark)
		if err != nil {
			return nil, err
		}
		if err := display.advanceTasks(marked, "ready"); err != nil {
			return nil, err
		}
		updated = len(marked)
		result := map[string]any{"updated": updated, "state": "ready"}
		if err := display.history("states.changed", nil, marked, result); err != nil {
			return nil, err
		}
	}
	dismissed, err := display.dismiss(toDismiss)
	if err != nil {
		return nil, err
	}
	if !order {
		return map[string]any{"updated": updated, "skipped": skipped}, nil
	}
	return map[string]any{"updated": updated, "dismissed": dismissed, "skipped": skipped}, nil
}

// expandOrderTracking agrega, para cada tarea pendiente de una estación «por pedido», las demás tareas pendientes de su
// envío en esa estación: en esas estaciones se marca el pedido completo de la estación, como POST kitchen/states con
// release_id (states.go).
func (display *displayService) expandOrderTracking(tasks []displayTask) ([]displayTask, error) {
	result := []displayTask{}
	included := map[string]bool{}
	pairs := map[string]bool{}
	for _, task := range tasks {
		if !included[task.ID] {
			included[task.ID] = true
			result = append(result, task)
		}
		if display.stations[task.StationID].Tracking != "order" {
			continue
		}
		pair := task.ReleaseID + "|" + task.StationID
		if pairs[pair] {
			continue
		}
		pairs[pair] = true
		siblings, err := many[displayTask](display.ctx, display.tx, `SELECT `+displayTaskColumns+`
            WHERE t.release_id=$1 AND t.station_id=$2 AND t.state IN ('released','preparing') AND u.state='active'
                AND u.version_id=t.version_id ORDER BY t.id`, task.ReleaseID, task.StationID)
		if err != nil {
			return nil, err
		}
		for _, sibling := range siblings {
			if !included[sibling.ID] {
				included[sibling.ID] = true
				result = append(result, sibling)
			}
		}
	}
	return result, nil
}

// tasks lee las tareas completas (Task) de una lista, en el mismo orden, para advanceTasks.
func (display *displayService) tasks(selected []displayTask) ([]Task, error) {
	ids := make([]string, 0, len(selected))
	for _, task := range selected {
		ids = append(ids, task.ID)
	}
	found, err := many[Task](display.ctx, display.tx, "SELECT to_jsonb(t) FROM app.kitchen_tasks t WHERE id = ANY($1::uuid[])", ids)
	if err != nil {
		return nil, err
	}
	byID := map[string]Task{}
	for _, task := range found {
		byID[task.ID] = task
	}
	result := make([]Task, 0, len(selected))
	for _, task := range selected {
		result = append(result, byID[task.ID])
	}
	return result, nil
}

// dismiss da por vistas unas cancelaciones (el botón del pedido de una tarjeta, §4) y avisa a las demás pantallas de
// esas estaciones con kitchen.state.changed.
func (display *displayService) dismiss(tasks []displayTask) (int, error) {
	if len(tasks) == 0 {
		return 0, nil
	}
	stations := map[string]bool{}
	ids := []string{}
	for _, task := range tasks {
		ids = append(ids, task.ID)
		stations[task.StationID] = true
	}
	_, err := display.tx.Exec(display.ctx, `INSERT INTO app.kitchen_display_dismissals(task_id,device_id)
        SELECT unnest($1::uuid[]),$2 ON CONFLICT (task_id) DO UPDATE SET device_id=excluded.device_id,dismissed_at=now()`,
		ids, display.request.Actor.DeviceID)
	if err != nil {
		return 0, err
	}
	for _, id := range sortedKeys(stations) {
		station := id
		if err := events.Append(display.ctx, display.tx, "kitchen.state.changed", "kitchen", nil, &station); err != nil {
			return 0, err
		}
	}
	return len(tasks), display.history("cancellations.dismissed", nil, nil, map[string]any{"task_ids": ids})
}

// --- Recientes y recuperar (K1-D8) ------------------------------------------------------------------------------------

// recent es lo marcado listo en los últimos 30 minutos en las estaciones de la pantalla, por cuenta, de lo más reciente
// a lo más antiguo.
func (display *displayService) recent() (*DisplayRecent, error) {
	now, err := display.databaseNow()
	if err != nil {
		return nil, err
	}
	result := &DisplayRecent{ServerTime: now, WindowMinutes: int(DisplayRecentWindow / time.Minute), Orders: []DisplayRecentOrder{}}
	stations := display.scope.IDs()
	if len(stations) == 0 {
		return result, nil
	}
	tasks, err := many[displayTask](display.ctx, display.tx, `SELECT `+displayTaskColumns+`
        WHERE t.station_id = ANY($1::uuid[]) AND t.state='ready' AND u.state='active' AND u.version_id=t.version_id
            AND t.ready_at > now() - make_interval(secs => $2)
        ORDER BY u.account_id,t.released_at,t.id LIMIT 1000`, stations, DisplayRecentWindow.Seconds())
	if err != nil || len(tasks) == 0 {
		return result, err
	}
	accounts := []string{}
	for _, accountTasks := range groupByAccount(tasks) {
		accounts = append(accounts, accountTasks[0].AccountID)
	}
	headers, err := display.headers(accounts)
	if err != nil {
		return nil, err
	}
	for _, accountTasks := range groupByAccount(tasks) {
		header, found := headers[accountTasks[0].AccountID]
		if !found {
			continue
		}
		var markedAt time.Time
		for _, task := range accountTasks {
			if task.ReadyAt != nil && task.ReadyAt.After(markedAt) {
				markedAt = *task.ReadyAt
			}
		}
		result.Orders = append(result.Orders, DisplayRecentOrder{
			DisplayCardHeader: cardHeader(header, accountTasks[0].Employee),
			MarkedAt:          markedAt,
			Items:             display.items(accountTasks, accountTasks[0].ReleasedAt),
		})
	}
	sort.SliceStable(result.Orders, func(first, second int) bool {
		return result.Orders[first].MarkedAt.After(result.Orders[second].MarkedAt)
	})
	for index := range result.Orders {
		for item := range result.Orders[index].Items {
			result.Orders[index].Items[item].IsNew = false
		}
	}
	return result, nil
}

// recover regresa a la pantalla lo marcado listo por error (K1-D8): solo desde la pantalla, dentro de los 30 minutos de
// «Recientes», a su estado anterior («En preparación» si ya había empezado antes de marcarla; si no, «Por preparar»).
// Borra los avisos de listo de esas unidades: el que el mesero todavía no recibió ya no le llega, y si se vuelve a
// marcar, recordReadyNotices registra uno nuevo. El modo Cocina del escritorio sigue sin poder retroceder un «Listo»
// (states.go).
func (display *displayService) recover() (any, error) {
	references, err := display.displayRefs()
	if err != nil {
		return nil, err
	}
	tasks, err := display.loadRefs(references)
	if err != nil {
		return nil, err
	}
	now, err := display.databaseNow()
	if err != nil {
		return nil, err
	}
	skipped := []displaySkip{}
	selected := []displayTask{}
	alreadyBack := false
	for index, task := range tasks {
		reference := references[index]
		switch {
		case task.VersionID != reference.VersionID || task.pendingChange():
			skipped = append(skipped, displaySkip{ID: task.ID, Reason: "changed"})
		case task.cancelled():
			skipped = append(skipped, displaySkip{ID: task.ID, Reason: "cancelled"})
		case task.State != "ready":
			alreadyBack = true
			skipped = append(skipped, displaySkip{ID: task.ID, Reason: "not_ready"})
		case task.ReadyAt == nil || !task.ReadyAt.After(now.Add(-DisplayRecentWindow)):
			skipped = append(skipped, displaySkip{ID: task.ID, Reason: "expired"})
		default:
			selected = append(selected, task)
		}
	}
	if len(selected) == 0 {
		if alreadyBack {
			return map[string]any{"recovered": 0, "skipped": skipped}, nil
		}
		return nil, &Error{Status: 409, Code: "KITCHEN_NOT_RECOVERABLE", Message: "Ese platillo ya no se puede regresar a la pantalla."}
	}
	ids := []string{}
	units := []string{}
	versions := []string{}
	releases := map[string]bool{}
	stations := map[string]bool{}
	for _, task := range selected {
		ids = append(ids, task.ID)
		units = append(units, task.UnitID)
		versions = append(versions, task.VersionID)
		releases[task.ReleaseID], stations[task.StationID] = true, true
	}
	// Una tarea marcada lista directamente quedó con started_at igual a ready_at (states.go: las dos en la misma
	// sentencia); si started_at es anterior, ya estaba «En preparación».
	_, err = display.tx.Exec(display.ctx, `UPDATE app.kitchen_tasks SET
            state=CASE WHEN started_at IS NOT NULL AND started_at < ready_at THEN 'preparing' ELSE 'released' END,
            started_at=CASE WHEN started_at IS NOT NULL AND started_at < ready_at THEN started_at END,
            ready_at=NULL,revision=revision+1,updated_at=now()
        WHERE id = ANY($1::uuid[]) AND state='ready'`, ids)
	if err != nil {
		return nil, err
	}
	_, err = display.tx.Exec(display.ctx, `DELETE FROM app.ready_notices notice
        USING unnest($1::uuid[],$2::uuid[]) AS recovered(unit_id,version_id)
        WHERE notice.unit_id=recovered.unit_id AND notice.version_id=recovered.version_id`, units, versions)
	if err != nil {
		return nil, err
	}
	recovered, err := display.tasks(selected)
	if err != nil {
		return nil, err
	}
	if err := display.taskStatesChanged(recovered, releases, stations); err != nil {
		return nil, err
	}
	result := map[string]any{"recovered": len(selected), "skipped": skipped}
	if err := display.history("states.recovered", nil, recovered, result); err != nil {
		return nil, err
	}
	return result, nil
}

// --- Acuse de recepción (K1-D12) --------------------------------------------------------------------------------------

// receipts confirma que la pantalla ya muestra unas comandas de sus estaciones: la función de la 9F (recordReceipt),
// con el dispositivo y sin usuario. Idempotente: confirmar dos veces no repite nada ni avisa otra vez.
func (display *displayService) receipts() (any, error) {
	var input struct {
		Receipts []DisplayReceipt `json:"receipts"`
	}
	if err := decode(display.request.Body, &input); err != nil {
		return nil, err
	}
	if len(input.Receipts) < 1 || len(input.Receipts) > displayReceiptLimit {
		return nil, invalid("Confirma entre 1 y 500 comandas.")
	}
	seen := map[string]bool{}
	for _, receipt := range input.Receipts {
		if err := validID(receipt.TicketID); err != nil {
			return nil, err
		}
		if err := validID(receipt.StationID); err != nil {
			return nil, err
		}
		key := receipt.TicketID + "|" + receipt.StationID
		if seen[key] {
			return nil, invalid("La comanda está repetida.")
		}
		seen[key] = true
		if !display.scope.Allows(receipt.StationID) {
			return nil, stationDenied()
		}
	}
	acknowledged := 0
	for _, receipt := range input.Receipts {
		var deliverable bool
		err := display.tx.QueryRow(display.ctx, `SELECT EXISTS(SELECT FROM jsonb_array_elements(t.document->'sections') section
                WHERE section->'station'->>'id'=$2 AND section->'station'->>'reception' IN ('kds','both'))
            FROM app.kitchen_tickets t WHERE t.id=$1 AND t.kind IN ('normal','modification','cancellation')`,
			receipt.TicketID, receipt.StationID).Scan(&deliverable)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "La comanda no existe."}
		}
		if err != nil {
			return nil, err
		}
		if !deliverable {
			return nil, invalid("La comanda no tiene entrega en pantalla para esa estación.")
		}
		created, err := display.recordReceipt(receipt.TicketID, receipt.StationID, false)
		if err != nil {
			return nil, err
		}
		if !created {
			continue
		}
		acknowledged++
		ticketID := receipt.TicketID
		if err := display.history("kds.acknowledged", &ticketID, nil, map[string]string{"station_id": receipt.StationID}); err != nil {
			return nil, err
		}
	}
	return map[string]int{"acknowledged": acknowledged}, nil
}

// --- Eventos (K1-D7) --------------------------------------------------------------------------------------------------

// DisplayEvents es la lectura del canal de eventos de la pantalla de cocina (GET events y events/snapshot con su
// credencial), con el mismo cursor y la misma ventana que el de las terminales. Solo recibe, sin contenido:
//   - license.changed;
//   - kitchen_display.configuration.changed de esta pantalla o de todas;
//   - los eventos de cocina de sus estaciones (displayKitchenKinds);
//   - los cambios de encabezado (displayAccountKinds) de las cuentas con tareas en sus estaciones: las de sus unidades
//     de hoy o las de sus liberaciones (la cuenta de origen de una combinación o una transferencia ya no tiene las
//     unidades, pero sí la liberación).
func DisplayEvents(ctx context.Context, tx pgx.Tx, deviceID string, after int64) (*events.Batch, error) {
	if err := validID(deviceID); err != nil {
		return nil, err
	}
	scope, _, err := loadDisplayScope(ctx, tx, deviceID)
	if err != nil {
		return nil, err
	}
	stations := scope.IDs()
	return events.ReadWith(ctx, tx, after, func(candidates []events.Change) ([]events.Change, error) {
		mentioned := map[string]bool{}
		for _, event := range candidates {
			if displayAccountEvent(event) {
				mentioned[*event.AggregateID] = true
			}
		}
		relevant := map[string]bool{}
		if len(mentioned) > 0 && len(stations) > 0 {
			accounts, err := many[string](ctx, tx, `SELECT DISTINCT to_jsonb(related.account_id::text) FROM (
                    SELECT u.account_id FROM app.restaurant_item_units u JOIN app.kitchen_tasks t ON t.unit_id=u.id
                        WHERE u.account_id = ANY($1::uuid[]) AND t.station_id = ANY($2::uuid[])
                    UNION
                    SELECT r.account_id FROM app.kitchen_releases r
                        WHERE r.account_id = ANY($1::uuid[]) AND r.station_snapshot ?| $2::text[]) related`,
				sortedKeys(mentioned), stations)
			if err != nil {
				return nil, err
			}
			for _, account := range accounts {
				relevant[account] = true
			}
		}
		accepted := []events.Change{}
		for _, event := range candidates {
			switch {
			case events.IsLicenseChanged(event):
				accepted = append(accepted, event)
			case event.Kind == DisplayConfigurationChangedKind && event.Scope == "configuration" &&
				(event.AggregateID == nil || *event.AggregateID == deviceID):
				accepted = append(accepted, event)
			case event.Scope == "kitchen" && displayKitchenKinds[event.Kind] && event.StationID != nil &&
				scope.Allows(*event.StationID):
				accepted = append(accepted, event)
			case displayAccountEvent(event) && relevant[*event.AggregateID]:
				accepted = append(accepted, event)
			}
		}
		return accepted, nil
	})
}

// displayAccountEvent: un cambio de encabezado de una cuenta (con la cuenta como agregado).
func displayAccountEvent(event events.Change) bool {
	return (event.Scope == "restaurant" || event.Scope == "takeaway") && displayAccountKinds[event.Kind] &&
		event.AggregateID != nil
}
