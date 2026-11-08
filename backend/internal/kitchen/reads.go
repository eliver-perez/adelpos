package kitchen

import (
	"strconv"
	"time"

	"local/ad-el-pos/server/internal/events"
)

func (handler *service) read(parts []string) (any, error) {
	switch parts[0] {
	case "stations":
		return many[Station](handler.ctx, handler.tx, "SELECT to_jsonb(s) FROM app.kitchen_stations s ORDER BY sort_order,name")
	case "outputs":
		return many[map[string]any](handler.ctx, handler.tx, "SELECT jsonb_build_object('id',id,'name',name,'active',active) FROM app.kitchen_printers ORDER BY name")
	case "printers":
		return many[Printer](handler.ctx, handler.tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p ORDER BY name")
	case "discovery":
		return DiscoverPrinters(handler.ctx)
	case "targets":
		products, err := many[map[string]any](handler.ctx, handler.tx,
			`SELECT jsonb_build_object('id',id,'name',name,'active',active)
			FROM app.catalog_products WHERE NOT archived ORDER BY name LIMIT 5000`)
		if err != nil {
			return nil, err
		}
		categories, err := many[map[string]any](handler.ctx, handler.tx,
			`SELECT jsonb_build_object('id',id,'name',name,'active',active)
			FROM app.catalog_categories ORDER BY sort_order,name`)
		return map[string]any{"products": products, "categories": categories}, err
	case "routes":
		return many[Route](handler.ctx, handler.tx, routeQuery+" ORDER BY r.id")
	case "settings":
		return handler.settings()
	case "access":
		return many[map[string]any](handler.ctx, handler.tx, `SELECT jsonb_build_object('device_id',d.id,'device_name',d.name,
            'station_ids',coalesce((SELECT jsonb_agg(station_id ORDER BY station_id) FROM app.kitchen_device_stations a WHERE a.device_id=d.id),'[]'))
            FROM app.devices d WHERE revoked_at IS NULL ORDER BY d.name`)
	case "releases":
		if err := handler.request.Actor.Require("kitchen.stations.all"); err != nil {
			return nil, err
		}
		if len(parts) == 2 {
			return readRelease(handler.ctx, handler.tx, parts[1])
		}
		accountID := handler.request.Query.Get("account_id")
		if accountID != "" {
			if err := validID(accountID); err != nil {
				return nil, err
			}
		}
		return many[Release](handler.ctx, handler.tx, `SELECT to_jsonb(r) FROM app.kitchen_releases r
			WHERE ($1='' OR r.account_id::text=$1 OR EXISTS(SELECT FROM app.restaurant_item_units u
			WHERE u.account_id::text=$1 AND u.id=ANY(r.unit_ids))) ORDER BY created_at DESC LIMIT 100`, accountID)
	case "board":
		return handler.board()
	case "tickets":
		return handler.readTickets(parts)
	case "jobs":
		return handler.readJobs(parts)
	case "history":
		if !handler.request.Actor.Can("kitchen.stations.all") {
			return nil, &Error{Status: 403, Code: "STATION_SCOPE_DENIED", Message: "El historial global requiere acceso a todas las estaciones."}
		}
		page, err := handler.page()
		if err != nil {
			return nil, err
		}
		return many[map[string]any](handler.ctx, handler.tx, `SELECT to_jsonb(h)||jsonb_build_object('actor_name',coalesce(u.display_name,'Servidor'),
            'device_name',coalesce(d.name,'Proceso Go')) FROM app.kitchen_history h LEFT JOIN app.users u ON u.id=h.actor_id
            LEFT JOIN app.devices d ON d.id=h.device_id ORDER BY h.created_at DESC,h.id DESC LIMIT 100 OFFSET $1`, (page-1)*100)
	}
	return nil, invalid("La consulta de cocina no existe.")
}

func (handler *service) page() (int, error) {
	value := handler.request.Query.Get("page")
	if value == "" {
		return 1, nil
	}
	page, err := strconv.Atoi(value)
	if err != nil || page < 1 || page > 10000 {
		return 0, invalid("La página debe estar entre 1 y 10000.")
	}
	return page, nil
}

func (handler *service) board() (any, error) {
	stationID := handler.request.Query.Get("station_id")
	if stationID != "" {
		if err := handler.stationAccess(stationID); err != nil {
			return nil, err
		}
	}
	page, err := handler.page()
	if err != nil {
		return nil, err
	}
	accountID := handler.request.Query.Get("account_id")
	if accountID != "" {
		if err := validID(accountID); err != nil {
			return nil, err
		}
	}
	all := handler.request.Actor.Can("kitchen.stations.all")
	tasks, err := many[map[string]any](handler.ctx, handler.tx, `SELECT to_jsonb(t)||jsonb_build_object(
        'state',CASE WHEN u.state='cancelled' THEN 'cancelled' ELSE t.state END,
        'folio',a.folio,'account_name',a.name,'table_name',p.name,'station_name',s.name,
        'warning_minutes',s.warning_minutes,'overdue_minutes',s.overdue_minutes,
        'target_minutes',s.target_minutes,'tracking',CASE WHEN s.tracking='inherit' THEN cfg.tracking ELSE s.tracking END,
        'elapsed_seconds',greatest(0,extract(epoch FROM now()-t.released_at)::bigint),'release_revision',r.revision)
        FROM app.kitchen_tasks t JOIN app.restaurant_item_units u ON u.id=t.unit_id
        JOIN app.restaurant_accounts a ON a.id=u.account_id LEFT JOIN app.restaurant_tables p ON p.id=a.table_id
        JOIN app.kitchen_stations s ON s.id=t.station_id JOIN app.kitchen_releases r ON r.id=t.release_id
        CROSS JOIN app.kitchen_settings cfg WHERE ($1='' OR t.station_id::text=$1)
        AND ($2 OR EXISTS(SELECT FROM app.kitchen_device_stations ds WHERE ds.device_id=$3 AND ds.station_id=t.station_id))
        AND ($6='' OR u.account_id::text=$6)
        AND ($4='all' OR ($4='ready' AND t.state='ready' AND u.state='active')
            OR ($4='' AND t.state IN ('released','preparing') AND u.state='active'))
        ORDER BY t.released_at,t.id LIMIT 250 OFFSET $5`, stationID, all, handler.request.Actor.DeviceID,
		handler.request.Query.Get("state"), (page-1)*250, accountID)
	if err != nil {
		return nil, err
	}
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	if !settings.ShowPrices || !handler.request.Actor.Can("catalog.prices.read") {
		for _, task := range tasks {
			if instruction, ok := task["instructions"].(map[string]any); ok {
				delete(instruction, "price")
			}
		}
	}
	releases := []map[string]any{}
	pendingChanges := []map[string]any{}
	if all {
		releases, err = many[map[string]any](handler.ctx, handler.tx, `SELECT (to_jsonb(r)-'unit_ids'-'routing_snapshot'-'station_snapshot')||
            jsonb_build_object('folio',a.folio,'account_name',a.name) FROM app.kitchen_releases r
            JOIN app.restaurant_accounts a ON a.id=r.account_id WHERE r.state IN ('pending','scheduled','blocked')
            ORDER BY coalesce(r.due_at,r.created_at),r.id LIMIT 100`)
		if err != nil {
			return nil, err
		}
		pendingChanges, err = many[map[string]any](handler.ctx, handler.tx, `SELECT to_jsonb(c)-'payload' FROM app.kitchen_pending_changes c ORDER BY created_at LIMIT 100`)
		if err != nil {
			return nil, err
		}
	}
	stations, err := many[Station](handler.ctx, handler.tx, `SELECT to_jsonb(s) FROM app.kitchen_stations s WHERE $1 OR EXISTS(
        SELECT FROM app.kitchen_device_stations d WHERE d.device_id=$2 AND d.station_id=s.id) ORDER BY sort_order,name`, all, handler.request.Actor.DeviceID)
	if err != nil {
		return nil, err
	}
	var timezone string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT timezone FROM app.establishment").Scan(&timezone); err != nil {
		return nil, err
	}
	return map[string]any{"tasks": tasks, "releases": releases, "pending_changes": pendingChanges,
		"stations": stations, "server_time": time.Now().UTC(), "timezone": timezone, "page": page}, nil
}

func (handler *service) authorizeResource(parts []string) error {
	if len(parts) < 2 {
		return nil
	}
	if parts[0] == "tickets" {
		_, err := handler.filteredDocument(parts[1])
		return err
	}
	if parts[0] == "jobs" || parts[0] == "releases" || parts[0] == "changes" {
		if !handler.request.Actor.Can("kitchen.stations.all") {
			return &Error{Status: 403, Code: "STATION_SCOPE_DENIED", Message: "Esta operación requiere acceso a todas las estaciones afectadas."}
		}
	}
	return nil
}

func (handler *service) filteredDocument(id string) (*Document, error) {
	var kind string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT kind FROM app.kitchen_tickets WHERE id=$1", id).Scan(&kind); err != nil {
		return nil, err
	}
	if isCommercialKind(kind) {
		return handler.commercialDocument(id, kind)
	}
	document, err := one[Document](handler.ctx, handler.tx, "SELECT document FROM app.kitchen_tickets WHERE id=$1", id)
	if err != nil {
		return nil, err
	}
	visible := []Section{}
	for _, section := range document.Sections {
		if err := handler.stationAccess(section.Station.ID); err == nil {
			if !handler.request.Actor.Can("catalog.prices.read") {
				for index := range section.Items {
					clearInstructionPrices(&section.Items[index])
				}
			}
			visible = append(visible, section)
		}
	}
	if len(visible) == 0 {
		return nil, &Error{Status: 403, Code: "STATION_SCOPE_DENIED", Message: "No tienes acceso a las estaciones de esta comanda."}
	}
	document.Sections = visible
	document.Preview = RenderText(*document, 80)
	return document, nil
}

// commercialDocument muestra en la cola un ticket o precuenta con su contenido impreso. Contiene
// importes, por lo que exige el alcance global de la cola y permiso de precios o pagos.
func (handler *service) commercialDocument(id, kind string) (*Document, error) {
	if err := handler.request.Actor.Require("kitchen.stations.all"); err != nil {
		return nil, err
	}
	if !handler.request.Actor.Can("catalog.prices.read") && !handler.request.Actor.Can("payments.read") {
		return nil, &Error{Status: 403, Code: "PERMISSION_DENIED", Message: "No tienes permiso para consultar documentos con importes."}
	}
	var document Document
	err := handler.tx.QueryRow(handler.ctx, `SELECT t.created_at,coalesce((SELECT j.content FROM app.kitchen_print_jobs j
        WHERE j.ticket_id=t.id ORDER BY j.created_at DESC LIMIT 1),'') FROM app.kitchen_tickets t WHERE t.id=$1`, id).Scan(&document.CreatedAt, &document.Preview)
	if err != nil {
		return nil, err
	}
	document.ID = id
	document.Kind = kind
	document.Sections = []Section{}
	return &document, nil
}

func (handler *service) readTickets(parts []string) (any, error) {
	if len(parts) == 2 {
		return handler.filteredDocument(parts[1])
	}
	page, err := handler.page()
	if err != nil {
		return nil, err
	}
	station := handler.request.Query.Get("station_id")
	if station != "" {
		if err := handler.stationAccess(station); err != nil {
			return nil, err
		}
	}
	rows, err := many[map[string]any](handler.ctx, handler.tx, `SELECT jsonb_build_object('id',t.id,'kind',t.kind,
        'created_at',t.created_at,'release_id',t.release_id,'folio',t.document->'context'->'folio',
        'pending_kds',EXISTS(SELECT FROM jsonb_array_elements(t.document->'sections') section
            WHERE section->'station'->>'reception' IN ('kds','both')
            AND ($2 OR EXISTS(SELECT FROM app.kitchen_device_stations a WHERE a.device_id=$3 AND a.station_id=(section->'station'->>'id')::uuid))
            AND NOT EXISTS(SELECT FROM app.kitchen_kds_receipts k WHERE k.ticket_id=t.id AND k.station_id=(section->'station'->>'id')::uuid)))
        FROM app.kitchen_tickets t WHERE t.kind NOT IN ('receipt','precheck') AND ($1='' OR NULLIF($1,'')::uuid=ANY(station_ids))
        AND ($2 OR EXISTS(SELECT FROM app.kitchen_device_stations d WHERE d.device_id=$3 AND d.station_id=ANY(t.station_ids)))
        ORDER BY created_at DESC,id DESC LIMIT 100 OFFSET $4`, station, handler.request.Actor.Can("kitchen.stations.all"), handler.request.Actor.DeviceID, (page-1)*100)
	return rows, err
}

func (handler *service) readJobs(parts []string) (any, error) {
	if err := handler.request.Actor.Require("kitchen.stations.all"); err != nil {
		return nil, err
	}
	if len(parts) == 2 {
		attempts, err := many[map[string]any](handler.ctx, handler.tx, "SELECT to_jsonb(a) FROM app.kitchen_print_attempts a WHERE job_id=$1 ORDER BY attempt", parts[1])
		if err != nil {
			return nil, err
		}
		job, err := one[map[string]any](handler.ctx, handler.tx, "SELECT to_jsonb(j)-'content' FROM app.kitchen_print_jobs j WHERE id=$1", parts[1])
		return map[string]any{"job": job, "attempts": attempts}, err
	}
	page, err := handler.page()
	if err != nil {
		return nil, err
	}
	return many[map[string]any](handler.ctx, handler.tx, `SELECT (to_jsonb(j)-'content')||jsonb_build_object('printer_name',p.name)
        FROM app.kitchen_print_jobs j JOIN app.kitchen_printers p ON p.id=j.printer_id
        WHERE ($1='' OR j.state=$1) ORDER BY j.created_at DESC,j.id DESC LIMIT 100 OFFSET $2`, handler.request.Query.Get("state"), (page-1)*100)
}

func (handler *service) ackTicket(id string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		StationID   string `json:"station_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := handler.stationAccess(input.StationID); err != nil {
		return nil, err
	}
	document, err := handler.filteredDocument(id)
	if err != nil {
		return nil, err
	}
	found := false
	for _, section := range document.Sections {
		if section.Station.ID == input.StationID && section.Station.Reception != "printer" {
			found = true
		}
	}
	if !found {
		return nil, invalid("La comanda no tiene entrega KDS para esa estación.")
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_kds_receipts(ticket_id,station_id,device_id,user_id)
        VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, id, input.StationID, handler.request.Actor.DeviceID, handler.request.Actor.UserID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "kitchen.kds.received", "kitchen", &id, &input.StationID); err != nil {
		return nil, err
	}
	return map[string]bool{"acknowledged": true}, handler.history("kds.acknowledged", &id, nil, map[string]string{"station_id": input.StationID})
}
