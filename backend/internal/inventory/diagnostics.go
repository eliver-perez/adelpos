package inventory

import (
	"encoding/json"
	"time"
)

// Lecturas de diagnóstico del consumo (inventory.movements.read): solicitudes pendientes o
// fallidas y la traza de consumo de una unidad comercial (solicitudes, estado, clasificación y
// movimientos por insumo y almacén; los valores solo con inventory.costs.read).

var requestKindLabels = map[string]string{
	"release": "Liberación a cocina",
	"amend":   "Modificación de partida liberada",
	"cancel":  "Cancelación de partida liberada",
}

var requestStateLabels = map[string]string{
	"pending":   "Pendiente",
	"processed": "Procesada",
	"failed":    "Con error (se reintenta)",
}

// ConsumptionRequest es una solicitud de consumo tal como la entrega la API.
type ConsumptionRequest struct {
	ID               string       `json:"id"`
	Seq              int64        `json:"seq"`
	Kind             string       `json:"kind"`
	KindLabel        string       `json:"kind_label"`
	RestaurantUnitID string       `json:"restaurant_unit_id"`
	ProductName      string       `json:"product_name"`
	AccountFolio     *int64       `json:"account_folio"`
	ServiceMode      string       `json:"service_mode"`
	ReleasedAt       time.Time    `json:"released_at"`
	OccurredAt       time.Time    `json:"occurred_at"`
	State            string       `json:"state"`
	StateLabel       string       `json:"state_label"`
	Attempts         int          `json:"attempts"`
	LastError        *string      `json:"last_error"`
	NextAttemptAt    time.Time    `json:"next_attempt_at"`
	ProcessedAt      *time.Time   `json:"processed_at"`
	CreatedAt        time.Time    `json:"created_at"`
	Diagnostics      []Diagnostic `json:"diagnostics"`
}

// ConsumptionRequestPage es una página de solicitudes.
type ConsumptionRequestPage struct {
	Items    []ConsumptionRequest `json:"items"`
	Total    int                  `json:"total"`
	Pending  int                  `json:"pending"`
	Failed   int                  `json:"failed"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

const requestColumns = `SELECT q.id,q.seq,q.kind,q.restaurant_unit_id,coalesce(v.quote_line->>'name',''),a.folio,q.service_mode,
        q.released_at,q.occurred_at,q.state,q.attempts,q.last_error,q.next_attempt_at,q.processed_at,q.created_at,q.diagnostics
        FROM app.inventory_consumption_requests q
        JOIN app.restaurant_item_versions v ON v.id=q.version_id
        JOIN app.restaurant_item_units u ON u.id=q.restaurant_unit_id
        LEFT JOIN app.restaurant_accounts a ON a.id=u.account_id`

func scanConsumptionRequest(row interface{ Scan(...any) error }) (ConsumptionRequest, error) {
	var request ConsumptionRequest
	var diagnostics []byte
	err := row.Scan(&request.ID, &request.Seq, &request.Kind, &request.RestaurantUnitID, &request.ProductName, &request.AccountFolio,
		&request.ServiceMode, &request.ReleasedAt, &request.OccurredAt, &request.State, &request.Attempts, &request.LastError,
		&request.NextAttemptAt, &request.ProcessedAt, &request.CreatedAt, &diagnostics)
	if err != nil {
		return ConsumptionRequest{}, err
	}
	request.KindLabel = requestKindLabels[request.Kind]
	request.StateLabel = requestStateLabels[request.State]
	request.Diagnostics = []Diagnostic{}
	if err := json.Unmarshal(diagnostics, &request.Diagnostics); err != nil {
		return ConsumptionRequest{}, err
	}
	return request, nil
}

// listConsumptionRequests lista solicitudes por estado (por omisión, las no procesadas), la más
// antigua primero.
func (handler *service) listConsumptionRequests(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "state", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	switch state := query.Get("state"); state {
	case "":
		set.clauses = append(set.clauses, "q.state<>'processed'")
	case "all":
	default:
		if _, known := requestStateLabels[state]; !known {
			return nil, invalid("El filtro de estado de la solicitud no es válido.")
		}
		set.add("q.state=$%d", state)
	}
	result := ConsumptionRequestPage{Items: []ConsumptionRequest{}, Page: page, PageSize: size}
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_consumption_requests q"+set.where(), set.arguments...).
		Scan(&result.Total); err != nil {
		return nil, err
	}
	if err := handler.tx.QueryRow(handler.ctx, `SELECT count(*) FILTER (WHERE state='pending'),count(*) FILTER (WHERE state='failed')
        FROM app.inventory_consumption_requests WHERE state<>'processed'`).Scan(&result.Pending, &result.Failed); err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, requestColumns+set.where()+" ORDER BY q.seq LIMIT "+set.next(size)+" OFFSET "+
		set.next((page-1)*size), set.arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		request, err := scanConsumptionRequest(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, request)
	}
	return result, rows.Err()
}

// UnitConsumptionTrace es la traza de consumo de una unidad comercial.
type UnitConsumptionTrace struct {
	RestaurantUnitID string               `json:"restaurant_unit_id"`
	Consumed         bool                 `json:"consumed"`
	State            *string              `json:"state"`
	ReleasedAt       *time.Time           `json:"released_at"`
	ServiceMode      *string              `json:"service_mode"`
	CostsVisible     bool                 `json:"costs_visible"`
	Requests         []ConsumptionRequest `json:"requests"`
	Lines            []ConsumptionLine    `json:"lines"`
	Movements        []Movement           `json:"movements"`
	Cancellation     *Cancellation        `json:"cancellation"`
}

// readUnitConsumption devuelve la traza de consumo de la unidad de la ruta.
func (handler *service) readUnitConsumption(parts []string) (any, error) {
	unitID := parts[1]
	var exists bool
	if err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.restaurant_item_units WHERE id=$1)", unitID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, notFound("La unidad comercial no existe.")
	}
	trace := &UnitConsumptionTrace{RestaurantUnitID: unitID, CostsVisible: handler.costsVisible(), Requests: []ConsumptionRequest{},
		Movements: []Movement{}}
	err := handler.tx.QueryRow(handler.ctx, `SELECT state,released_at,service_mode FROM app.inventory_unit_consumptions
        WHERE restaurant_unit_id=$1`, unitID).Scan(&trace.State, &trace.ReleasedAt, &trace.ServiceMode)
	if err != nil && !isNoRows(err) {
		return nil, err
	}
	trace.Consumed = trace.State != nil
	rows, err := handler.tx.Query(handler.ctx, requestColumns+" WHERE q.restaurant_unit_id=$1 ORDER BY q.seq", unitID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		request, err := scanConsumptionRequest(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		trace.Requests = append(trace.Requests, request)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if trace.Lines, err = handler.consumptionLines(unitID); err != nil {
		return nil, err
	}
	statement := movementColumns + " WHERE m.restaurant_unit_id=$1"
	if !handler.costsVisible() {
		statement += " AND m.kind<>'cost_adjustment'"
	}
	movements, err := handler.tx.Query(handler.ctx, statement+" ORDER BY m.seq", unitID)
	if err != nil {
		return nil, err
	}
	for movements.Next() {
		movement, err := handler.scanMovement(movements)
		if err != nil {
			movements.Close()
			return nil, err
		}
		trace.Movements = append(trace.Movements, movement)
	}
	movements.Close()
	if err := movements.Err(); err != nil {
		return nil, err
	}
	var classification string
	err = handler.tx.QueryRow(handler.ctx, "SELECT id FROM app.inventory_cancellation_classifications WHERE restaurant_unit_id=$1", unitID).
		Scan(&classification)
	if err != nil && !isNoRows(err) {
		return nil, err
	}
	if classification != "" {
		if trace.Cancellation, err = handler.readCancellation(classification); err != nil {
			return nil, err
		}
	}
	return trace, nil
}
