package inventory

import (
	"strings"
	"time"
)

// Rutas de cancelaciones por clasificar. El listado lo consulta quien tiene inventory.read; las
// cantidades y almacenes se muestran siempre y los valores solo con inventory.costs.read.
// Clasificar exige inventory.waste.record hasta el corte del día operativo y, después (o para
// corregir una clasificación vencida), inventory.adjustments.authorize. Ambos permisos se
// comprueban antes de escribir.

// Cancellation es una cancelación de una unidad consumida con su clasificación. Preclassified (etapa
// 9D): «Preparado / merma» que el trabajador asignó al cancelar la cuenta con un motivo que marca
// merma y que nadie ha corregido; ClassifiedByName es entonces quien canceló la cuenta, no quien la
// clasificó, y ResolutionLabel lo dice («Preclasificada al cancelar la cuenta»).
type Cancellation struct {
	ID                 string            `json:"id"`
	RestaurantUnitID   string            `json:"restaurant_unit_id"`
	ProductName        string            `json:"product_name"`
	AccountFolio       *int64            `json:"account_folio"`
	AccountName        *string           `json:"account_name"`
	CancelledAt        time.Time         `json:"cancelled_at"`
	OperatingDate      string            `json:"operating_date"`
	DeadlineAt         time.Time         `json:"deadline_at"`
	Expired            bool              `json:"expired"`
	State              string            `json:"state"`
	StateLabel         string            `json:"state_label"`
	Resolution         *string           `json:"resolution"`
	ResolutionLabel    *string           `json:"resolution_label"`
	Preclassified      bool              `json:"preclassified"`
	ClassifiedByName   *string           `json:"classified_by_name"`
	ClassifiedAt       *time.Time        `json:"classified_at"`
	Notes              *string           `json:"notes"`
	Revision           int               `json:"revision"`
	RequiredPermission string            `json:"required_permission"`
	CanClassify        bool              `json:"can_classify"`
	CostsVisible       bool              `json:"costs_visible"`
	Lines              []ConsumptionLine `json:"lines"`
}

// ConsumptionLine es el consumo de una unidad en un (insumo, almacén): lo consumido por la
// liberación y sus modificaciones y lo neto vigente (tras clasificaciones). Value solo con
// inventory.costs.read.
type ConsumptionLine struct {
	Item             ItemReference      `json:"item"`
	Warehouse        WarehouseReference `json:"warehouse"`
	ConsumedQuantity string             `json:"consumed_quantity"`
	NetQuantity      string             `json:"net_quantity"`
	Value            *string            `json:"value"`
	ValueDisplay     *string            `json:"value_display"`
}

// CancellationPage es una página del listado de cancelaciones.
type CancellationPage struct {
	Items        []*Cancellation `json:"items"`
	Total        int             `json:"total"`
	Pending      int             `json:"pending"`
	Page         int             `json:"page"`
	PageSize     int             `json:"page_size"`
	CostsVisible bool            `json:"costs_visible"`
}

const cancellationColumns = `SELECT c.id,c.restaurant_unit_id,coalesce(v.quote_line->>'name',''),a.folio,a.name,c.cancelled_at,
        to_char(c.operating_date,'YYYY-MM-DD'),c.deadline_at,c.deadline_at<=now() OR c.resolution IS NOT DISTINCT FROM 'automatic',c.state,c.resolution,author.display_name,
        c.classified_at,c.notes,c.revision,
        c.resolution IS NOT DISTINCT FROM 'manual' AND c.revision=1 AND EXISTS(SELECT FROM app.restaurant_events e
            WHERE e.id=q.source_id AND e.kind='account.cancelled')
        FROM app.inventory_cancellation_classifications c
        JOIN app.inventory_consumption_requests q ON q.id=c.request_id
        JOIN app.restaurant_item_versions v ON v.id=q.version_id
        JOIN app.restaurant_item_units u ON u.id=c.restaurant_unit_id
        LEFT JOIN app.restaurant_accounts a ON a.id=u.account_id
        LEFT JOIN app.users author ON author.id=c.classified_by`

func (handler *service) scanCancellation(row interface{ Scan(...any) error }) (*Cancellation, error) {
	var item Cancellation
	err := row.Scan(&item.ID, &item.RestaurantUnitID, &item.ProductName, &item.AccountFolio, &item.AccountName, &item.CancelledAt,
		&item.OperatingDate, &item.DeadlineAt, &item.Expired, &item.State, &item.Resolution, &item.ClassifiedByName, &item.ClassifiedAt,
		&item.Notes, &item.Revision, &item.Preclassified)
	if err != nil {
		return nil, err
	}
	item.StateLabel = classificationLabels[item.State]
	if item.Resolution != nil {
		label := resolutionLabels[*item.Resolution]
		// Una clasificación manual en su primera revisión que viene de account.cancelled solo puede
		// ser la preclasificación del trabajador: toda clasificación del encargado sube la revisión.
		if item.Preclassified {
			label = preclassifiedLabel
		}
		item.ResolutionLabel = &label
	}
	item.RequiredPermission = permissionWasteRecord
	if item.Expired {
		item.RequiredPermission = permissionAdjustmentsAuthorize
	}
	item.CanClassify = handler.request.Actor.Can(item.RequiredPermission)
	item.CostsVisible = handler.costsVisible()
	item.Lines = []ConsumptionLine{}
	return &item, nil
}

// listCancellations lista cancelaciones por estado (pending, not_prepared, prepared_waste) y día
// operativo (from/until AAAA-MM-DD), las pendientes primero por vencimiento.
func (handler *service) listCancellations(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "state", "from", "until", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	if state := query.Get("state"); state != "" {
		if _, known := classificationLabels[state]; !known {
			return nil, invalid("El filtro de estado de la cancelación no es válido.")
		}
		set.add("c.state=$%d", state)
	}
	for key, operator := range map[string]string{"from": ">=", "until": "<="} {
		text := query.Get(key)
		if text == "" {
			continue
		}
		day, err := time.Parse("2006-01-02", text)
		if err != nil {
			return nil, invalid("Las fechas del filtro deben tener el formato AAAA-MM-DD.")
		}
		set.add("c.operating_date"+operator+"$%d", day)
	}
	result := CancellationPage{Items: []*Cancellation{}, Page: page, PageSize: size, CostsVisible: handler.costsVisible()}
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_cancellation_classifications c"+set.where(),
		set.arguments...).Scan(&result.Total); err != nil {
		return nil, err
	}
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_cancellation_classifications WHERE state='pending'").
		Scan(&result.Pending); err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, cancellationColumns+set.where()+` ORDER BY (c.state='pending') DESC, c.deadline_at,
        c.cancelled_at DESC, c.id LIMIT `+set.next(size)+" OFFSET "+set.next((page-1)*size), set.arguments...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		item, err := handler.scanCancellation(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		result.Items = append(result.Items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, item := range result.Items {
		lines, err := handler.consumptionLines(item.RestaurantUnitID)
		if err != nil {
			return nil, err
		}
		item.Lines = lines
	}
	return result, nil
}

// readCancellation lee una cancelación con su consumo.
func (handler *service) readCancellation(id string) (*Cancellation, error) {
	item, err := handler.scanCancellation(handler.tx.QueryRow(handler.ctx, cancellationColumns+" WHERE c.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("La cancelación no existe.")
	}
	if err != nil {
		return nil, err
	}
	lines, err := handler.consumptionLines(item.RestaurantUnitID)
	if err != nil {
		return nil, err
	}
	item.Lines = lines
	return item, nil
}

// consumptionLines describe el consumo de una unidad por (insumo, almacén).
func (handler *service) consumptionLines(unitID string) ([]ConsumptionLine, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT i.id,i.code,i.name,i.kind,i.active,u.id,u.code,u.name,u.dimension,w.id,w.name,
            (-sum(m.quantity) FILTER (WHERE m.source_kind<>'cancellation'))::text,(-sum(m.quantity))::text,(-sum(m.value))::text
        FROM app.inventory_movements m
        JOIN app.inventory_items i ON i.id=m.item_id JOIN app.inventory_units u ON u.id=i.base_unit_id
        JOIN app.inventory_warehouses w ON w.id=m.warehouse_id
        WHERE m.restaurant_unit_id=$1 AND m.kind IN ('consumption','consumption_reversal')
        GROUP BY i.id,i.code,i.name,i.kind,i.active,u.id,u.code,u.name,u.dimension,w.id,w.name ORDER BY i.name,w.name`, unitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []ConsumptionLine{}
	for rows.Next() {
		var line ConsumptionLine
		var consumed *string
		var net, value string
		if err := rows.Scan(&line.Item.ID, &line.Item.Code, &line.Item.Name, &line.Item.Kind, &line.Item.Active, &line.Item.Unit.ID,
			&line.Item.Unit.Code, &line.Item.Unit.Name, &line.Item.Unit.Dimension, &line.Warehouse.ID, &line.Warehouse.Name, &consumed,
			&net, &value); err != nil {
			return nil, err
		}
		line.ConsumedQuantity = "0"
		if consumed != nil {
			line.ConsumedQuantity = quantityText(parseDatabaseDecimal(*consumed))
		}
		line.NetQuantity = quantityText(parseDatabaseDecimal(net))
		if handler.costsVisible() {
			line.Value, line.ValueDisplay = moneyPointers(parseDatabaseDecimal(value))
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

type classifyInput struct {
	OperationID string  `json:"operation_id"`
	Revision    int     `json:"revision"`
	State       string  `json:"state"`
	Notes       *string `json:"notes"`
}

// classifyCancellation clasifica (o corrige) una cancelación. Repetir el estado vigente no escribe
// nada.
func (handler *service) classifyCancellation(parts []string) (any, error) {
	var input classifyInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	input.State = normalizeKind(input.State)
	if input.State != "not_prepared" && input.State != "prepared_waste" {
		return nil, invalid("Clasifica la cancelación como «No preparado» o «Preparado / merma».")
	}
	var notes *string
	if input.Notes != nil {
		trimmed := strings.TrimSpace(*input.Notes)
		if textLength(trimmed) > 500 {
			return nil, invalid("La nota de la clasificación admite hasta 500 caracteres.")
		}
		if trimmed != "" {
			notes = &trimmed
		}
	}
	row, err := lockClassification(handler.ctx, handler.tx, parts[1])
	if err != nil {
		return nil, err
	}
	// Vencida: pasó el corte o el reconciliador ya la resolvió (eso solo ocurre pasado el corte).
	var expired bool
	if err := handler.tx.QueryRow(handler.ctx, "SELECT $1::timestamptz<=now()", row.Deadline).Scan(&expired); err != nil {
		return nil, err
	}
	expired = expired || row.Resolution == "automatic"
	resolution := "manual"
	permission := permissionWasteRecord
	if expired {
		resolution, permission = "correction", permissionAdjustmentsAuthorize
	}
	if err := handler.request.Actor.Require(permission); err != nil {
		return nil, err
	}
	if row.Revision != input.Revision {
		return nil, revisionConflict("La cancelación")
	}
	before, err := handler.readCancellation(row.ID)
	if err != nil {
		return nil, err
	}
	if row.State == input.State {
		return before, nil
	}
	written, err := applyClassification(handler.ctx, handler.tx, row, classificationChange{
		state: input.State, resolution: resolution, actorID: handler.actorReference(), operationID: handler.operationReference(),
		notes: notes,
	})
	if err != nil {
		return nil, err
	}
	saved, err := handler.readCancellation(row.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.alerts.changed", &saved.ID); err != nil {
		return nil, err
	}
	if written > 0 {
		if err := handler.publish("inventory.stock.changed", &saved.RestaurantUnitID); err != nil {
			return nil, err
		}
		if err := handler.publish("inventory.movements.recorded", &saved.ID); err != nil {
			return nil, err
		}
	}
	action := "inventory.cancellation_classified"
	if resolution == "correction" {
		action = "inventory.cancellation_corrected"
	}
	auditBefore := map[string]any{"state": before.State, "resolution": before.Resolution, "revision": before.Revision}
	auditAfter := map[string]any{"state": saved.State, "resolution": saved.Resolution, "revision": saved.Revision, "notes": saved.Notes,
		"movements": written, "lines": saved.Lines}
	return saved, handler.audit(handler.ctx, handler.tx, action, "inventory_cancellation", &saved.ID, auditBefore, auditAfter)
}
