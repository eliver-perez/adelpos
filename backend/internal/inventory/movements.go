package inventory

import (
	"time"
)

// Consulta del libro de movimientos (inventory.movements.read). Las cantidades son visibles con el
// permiso; valores, costos unitarios, base de costo, «costo pendiente» y valuación posterior solo
// con inventory.costs.read. Sin ese permiso tampoco se listan los ajustes de costo, que son solo
// valor (cantidad cero).

var movementKindLabels = map[string]string{
	KindInitial:             "Existencia inicial",
	KindReceipt:             "Entrada por compra",
	KindConsumption:         "Consumo por preparación",
	KindConsumptionReversal: "Reversión de consumo",
	KindWaste:               "Merma",
	KindAdjustmentIn:        "Ajuste positivo",
	KindAdjustmentOut:       "Ajuste negativo",
	KindTransferOut:         "Transferencia (salida)",
	KindTransferIn:          "Transferencia (entrada)",
	KindSupplierReturn:      "Devolución a proveedor",
	KindProductionIn:        "Elaboración (entrada)",
	KindProductionOut:       "Elaboración (consumo)",
	KindCountIn:             "Sobrante de conteo",
	KindCountOut:            "Faltante de conteo",
	KindCostAdjustment:      "Ajuste de costo",
}

var costBasisLabels = map[string]string{
	basisAverage:     "Costo promedio",
	basisReference:   "Último costo válido",
	basisConfigured:  "Costo de referencia del insumo",
	basisPending:     "Costo pendiente",
	basisDocument:    "Costo del documento",
	basisOriginal:    "Valor original",
	basisProvisional: "Valor provisional del faltante",
	basisDifference:  "Diferencia de costo",
}

// sourceKindLabels nombra el origen de un movimiento; las fases B–D agregan los suyos.
var sourceKindLabels = map[string]string{
	"initial":      "Existencia inicial",
	"consumption":  "Liberación a cocina",
	"amendment":    "Modificación de partida",
	"cancellation": "Cancelación de partida",
	"production":   "Elaboración",
	// Fase C: compras.
	"receipt":         "Recepción de mercancía",
	"supplier_return": "Devolución a proveedor",
}

// processLabels nombra los procesos de Go responsables de movimientos (fase B en adelante).
var processLabels = map[string]string{
	"inventory_worker": "Proceso de consumo de inventario",
}

// Movement es un movimiento del libro tal como lo entrega la API.
type Movement struct {
	ID                     string        `json:"id"`
	Seq                    int64         `json:"seq"`
	ItemID                 string        `json:"item_id"`
	ItemCode               string        `json:"item_code"`
	ItemName               string        `json:"item_name"`
	Unit                   UnitReference `json:"unit"`
	WarehouseID            *string       `json:"warehouse_id"`
	WarehouseName          *string       `json:"warehouse_name"`
	Kind                   string        `json:"kind"`
	KindLabel              string        `json:"kind_label"`
	Quantity               string        `json:"quantity"`
	StockAfter             *string       `json:"stock_after"`
	ValuationQuantityAfter string        `json:"valuation_quantity_after"`
	CostsVisible           bool          `json:"costs_visible"`
	Value                  *string       `json:"value"`
	ValueDisplay           *string       `json:"value_display"`
	UnitCost               *string       `json:"unit_cost"`
	UnitCostDisplay        *string       `json:"unit_cost_display"`
	CostBasis              *string       `json:"cost_basis"`
	CostBasisLabel         *string       `json:"cost_basis_label"`
	CostPending            *bool         `json:"cost_pending"`
	ValuationValueAfter    *string       `json:"valuation_value_after"`
	OccurredAt             time.Time     `json:"occurred_at"`
	RecordedAt             time.Time     `json:"recorded_at"`
	SourceKind             string        `json:"source_kind"`
	SourceKindLabel        string        `json:"source_kind_label"`
	SourceID               *string       `json:"source_id"`
	SourceLine             *int          `json:"source_line"`
	RestaurantUnitID       *string       `json:"restaurant_unit_id"`
	RelatedMovementID      *string       `json:"related_movement_id"`
	Reason                 *string       `json:"reason"`
	ActorName              *string       `json:"actor_name"`
	Process                *string       `json:"process"`
	ProcessLabel           *string       `json:"process_label"`
	OperationID            *string       `json:"operation_id"`
}

// MovementPage es una página del libro, del más reciente al más antiguo.
type MovementPage struct {
	Items        []Movement `json:"items"`
	Total        int        `json:"total"`
	Page         int        `json:"page"`
	PageSize     int        `json:"page_size"`
	CostsVisible bool       `json:"costs_visible"`
}

const movementColumns = `SELECT m.id,m.seq,m.item_id,i.code,i.name,u.id,u.code,u.name,u.dimension,m.warehouse_id,w.name,m.kind,
        m.quantity::text,m.stock_after::text,m.valuation_quantity_after::text,m.value::text,m.unit_cost::text,m.cost_basis,
        m.cost_pending,m.valuation_value_after::text,m.occurred_at,m.recorded_at,m.source_kind,m.source_id,m.source_line,
        m.restaurant_unit_id,m.related_movement_id,m.reason,actor.display_name,m.process,m.operation_id
        FROM app.inventory_movements m
        JOIN app.inventory_items i ON i.id=m.item_id
        JOIN app.inventory_units u ON u.id=i.base_unit_id
        LEFT JOIN app.inventory_warehouses w ON w.id=m.warehouse_id
        LEFT JOIN app.users actor ON actor.id=m.actor_id`

func (handler *service) scanMovement(row interface{ Scan(...any) error }) (Movement, error) {
	var movement Movement
	var quantity, valuationQuantity, value, basis, valuationValue string
	var stockAfter, unitCost *string
	var pending bool
	err := row.Scan(&movement.ID, &movement.Seq, &movement.ItemID, &movement.ItemCode, &movement.ItemName, &movement.Unit.ID,
		&movement.Unit.Code, &movement.Unit.Name, &movement.Unit.Dimension, &movement.WarehouseID, &movement.WarehouseName,
		&movement.Kind, &quantity, &stockAfter, &valuationQuantity, &value, &unitCost, &basis, &pending, &valuationValue,
		&movement.OccurredAt, &movement.RecordedAt, &movement.SourceKind, &movement.SourceID, &movement.SourceLine,
		&movement.RestaurantUnitID, &movement.RelatedMovementID, &movement.Reason, &movement.ActorName, &movement.Process,
		&movement.OperationID)
	if err != nil {
		return Movement{}, err
	}
	movement.KindLabel = movementKindLabels[movement.Kind]
	movement.Quantity = quantityText(parseDatabaseDecimal(quantity))
	movement.StockAfter = optionalQuantityText(parseDatabaseOptional(stockAfter))
	movement.ValuationQuantityAfter = quantityText(parseDatabaseDecimal(valuationQuantity))
	movement.SourceKindLabel = sourceKindLabels[movement.SourceKind]
	if movement.SourceKindLabel == "" {
		movement.SourceKindLabel = movement.SourceKind
	}
	if movement.Process != nil {
		label := processLabels[*movement.Process]
		if label == "" {
			label = *movement.Process
		}
		movement.ProcessLabel = &label
	}
	movement.CostsVisible = handler.costsVisible()
	if movement.CostsVisible {
		exact, display := exactMoneyText(parseDatabaseDecimal(value)), displayMoneyText(parseDatabaseDecimal(value))
		movement.Value, movement.ValueDisplay = &exact, &display
		if cost := parseDatabaseOptional(unitCost); cost != nil {
			exactCost, displayCost := exactMoneyText(*cost), displayUnitCostText(*cost)
			movement.UnitCost, movement.UnitCostDisplay = &exactCost, &displayCost
		}
		label := costBasisLabels[basis]
		movement.CostBasis, movement.CostBasisLabel = &basis, &label
		movement.CostPending = &pending
		valuation := exactMoneyText(parseDatabaseDecimal(valuationValue))
		movement.ValuationValueAfter = &valuation
	}
	return movement, nil
}

// listMovements consulta el libro con filtros de insumo, almacén, tipo, origen y fechas de negocio
// (AAAA-MM-DD en la zona horaria del establecimiento), con paginación.
func (handler *service) listMovements(_ []string) (any, error) {
	query := handler.request.Query
	allowed := []string{"item_id", "warehouse_id", "kind", "source_kind", "source_id", "from", "until", "page", "page_size"}
	if err := allowQuery(query, allowed...); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	if !handler.costsVisible() {
		set.clauses = append(set.clauses, "m.kind<>'cost_adjustment'")
	}
	for key, column := range map[string]string{"item_id": "m.item_id", "warehouse_id": "m.warehouse_id", "source_id": "m.source_id"} {
		identifier, err := optionalQueryID(query, key, "identificador")
		if err != nil {
			return nil, err
		}
		if identifier != nil {
			set.add(column+"=$%d", *identifier)
		}
	}
	if kind := query.Get("kind"); kind != "" {
		if _, known := movementKindLabels[kind]; !known {
			return nil, invalid("El filtro de tipo de movimiento no es válido.")
		}
		set.add("m.kind=$%d", kind)
	}
	if source := query.Get("source_kind"); source != "" {
		if !processPattern.MatchString(source) {
			return nil, invalid("El filtro de origen no es válido.")
		}
		set.add("m.source_kind=$%d", source)
	}
	from, until, err := handler.dateRange(query)
	if err != nil {
		return nil, err
	}
	if from != nil {
		set.add("m.occurred_at>=$%d", *from)
	}
	if until != nil {
		set.add("m.occurred_at<$%d", *until)
	}
	result := MovementPage{Items: []Movement{}, Page: page, PageSize: size, CostsVisible: handler.costsVisible()}
	err = handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_movements m"+set.where(), set.arguments...).Scan(&result.Total)
	if err != nil {
		return nil, err
	}
	statement := movementColumns + set.where() + " ORDER BY m.occurred_at DESC, m.seq DESC LIMIT " + set.next(size) +
		" OFFSET " + set.next((page-1)*size)
	rows, err := handler.tx.Query(handler.ctx, statement, set.arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		movement, err := handler.scanMovement(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, movement)
	}
	return result, rows.Err()
}
