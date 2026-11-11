package inventory

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Devoluciones a proveedor (§19 de la especificación y regla 5 del motor de costos). Una devolución:
//   - exige inventory.supplier_returns.record y está siempre ligada a una recepción (no se borra ni
//     se modifica la recepción original);
//   - devuelve por renglón de la recepción, en su misma presentación o unidad, nunca más de lo
//     recibido menos lo ya devuelto (RETURN_EXCEEDS_RECEIVED);
//   - sale con un movimiento supplier_return: salida normal al promedio vigente; la diferencia contra
//     el costo original de la recepción se registra como ajuste de costo ligado solo si la existencia
//     restante es positiva y sin dejar el valor negativo, y lo que no se absorbe queda en el
//     documento como «diferencia no absorbida» (sin movimiento);
//   - su valor asociado es cantidad × costo original (exacto cuando se devuelve todo lo que quedaba
//     del renglón);
//   - no crea ingresos de efectivo ni devoluciones bancarias, y es idempotente por operation_id
//     (repetirla devuelve el mismo documento; no hay devoluciones duplicadas). Una existencia
//     negativa no la bloquea: se avisa con NEGATIVE_STOCK.

// costPrecision es la precisión del costo original que se entrega al motor para que
// cantidad × costo reproduzca exactamente el valor del renglón (el documento guarda el costo a 10
// decimales).
const costPrecision = 20

type supplierReturnInput struct {
	OperationID string `json:"operation_id"`
	ReceiptID   string `json:"receipt_id"`
	// WarehouseID vacío es el almacén de la recepción.
	WarehouseID *string                   `json:"warehouse_id"`
	OccurredAt  *time.Time                `json:"occurred_at"`
	Reason      string                    `json:"reason"`
	Lines       []supplierReturnLineInput `json:"lines"`
}

type supplierReturnLineInput struct {
	ReceiptLineNo int `json:"receipt_line_no"`
	// Quantity está en la presentación o unidad del renglón de la recepción.
	Quantity string `json:"quantity"`
}

// SupplierReturnSummary es una devolución en listados e historiales. value es el valor asociado (a
// costo original); la diferencia no absorbida solo con inventory.costs.read.
type SupplierReturnSummary struct {
	ID                string             `json:"id"`
	Folio             int64              `json:"folio"`
	ReceiptID         string             `json:"receipt_id"`
	ReceiptFolio      int64              `json:"receipt_folio"`
	Supplier          SupplierReference  `json:"supplier"`
	Warehouse         WarehouseReference `json:"warehouse"`
	OccurredAt        time.Time          `json:"occurred_at"`
	Reason            string             `json:"reason"`
	ActorName         string             `json:"actor_name"`
	Value             string             `json:"value"`
	LineCount         int                `json:"line_count"`
	CostsVisible      bool               `json:"costs_visible"`
	Unabsorbed        *string            `json:"unabsorbed"`
	UnabsorbedDisplay *string            `json:"unabsorbed_display"`
	ValueExact        *string            `json:"value_exact"`
	CreatedAt         time.Time          `json:"created_at"`
	// ReceiptReference es la referencia del proveedor (factura o remisión) de la recepción.
	ReceiptReference string `json:"receipt_supplier_reference"`
}

// SupplierReturn es la devolución con sus renglones. Warnings solo viaja en la respuesta del registro.
type SupplierReturn struct {
	SupplierReturnSummary
	Lines    []SupplierReturnLine `json:"lines"`
	Warnings []Warning            `json:"warnings"`
}

// SupplierReturnLine es un renglón devuelto. Los datos de costeo (costo original, valor de salida
// al promedio, ajuste de costo y diferencia no absorbida) solo con inventory.costs.read.
type SupplierReturnLine struct {
	LineNo                 int           `json:"line_no"`
	ReceiptLineNo          int           `json:"receipt_line_no"`
	Item                   ItemReference `json:"item"`
	CaptureLabel           string        `json:"capture_label"`
	Quantity               string        `json:"quantity"`
	BaseQuantity           string        `json:"base_quantity"`
	Value                  string        `json:"value"`
	MovementID             string        `json:"movement_id"`
	StockAfter             string        `json:"stock_after"`
	CostsVisible           bool          `json:"costs_visible"`
	OriginalUnitCost       *string       `json:"original_unit_cost"`
	OriginalUnitCostShow   *string       `json:"original_unit_cost_display"`
	OutflowValue           *string       `json:"outflow_value"`
	OutflowValueDisplay    *string       `json:"outflow_value_display"`
	CostAdjustment         *string       `json:"cost_adjustment"`
	CostAdjustmentDisplay  *string       `json:"cost_adjustment_display"`
	Unabsorbed             *string       `json:"unabsorbed"`
	UnabsorbedDisplay      *string       `json:"unabsorbed_display"`
	CostAdjustmentMovement *string       `json:"cost_adjustment_movement_id"`
}

// SupplierReturnPage es una página del listado de devoluciones (la más reciente primero).
type SupplierReturnPage struct {
	Items        []SupplierReturnSummary `json:"items"`
	Total        int                     `json:"total"`
	Page         int                     `json:"page"`
	PageSize     int                     `json:"page_size"`
	CostsVisible bool                    `json:"costs_visible"`
}

// preparedReturnLine es un renglón validado de una devolución.
type preparedReturnLine struct {
	receiptLine *ReceiptLine
	quantity    decimal.Decimal
	base        decimal.Decimal
	// cost es el costo original por unidad base con precisión suficiente para que base × cost sea
	// el valor exacto que se devuelve.
	cost decimal.Decimal
}

// recordSupplierReturn registra una devolución a proveedor de una recepción.
func (handler *service) recordSupplierReturn(_ []string) (any, error) {
	var input supplierReturnInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.ReceiptID); err != nil {
		return nil, invalid("Elige la recepción que se devuelve.")
	}
	reason, err := purchaseReason(input.Reason, "El motivo de la devolución")
	if err != nil {
		return nil, err
	}
	if len(input.Lines) == 0 || len(input.Lines) > maximumReceiptLines {
		return nil, invalid("La devolución requiere de 1 a 200 renglones.")
	}
	receipt, err := handler.readReceipt(strings.ToLower(input.ReceiptID))
	if err != nil {
		return nil, err
	}
	occurredAt, err := handler.initialTime(input.OccurredAt)
	if err != nil {
		return nil, invalid("La fecha de la devolución no puede ser futura.")
	}
	if occurredAt.Before(receipt.OccurredAt) {
		return nil, invalid("La devolución no puede ser anterior a la recepción.")
	}
	warehouseID := input.WarehouseID
	if warehouseID == nil || strings.TrimSpace(*warehouseID) == "" {
		warehouseID = &receipt.Warehouse.ID
	}
	warehouse, err := handler.purchaseWarehouse(warehouseID)
	if err != nil {
		return nil, err
	}
	lines, err := prepareReturnLines(receipt, input.Lines)
	if err != nil {
		return nil, err
	}
	return handler.saveSupplierReturn(receipt, warehouse, occurredAt, reason, lines)
}

// prepareReturnLines valida los renglones devueltos: un renglón de la recepción una sola vez y
// nunca más de lo recibido menos lo ya devuelto. Devolver todo lo que quedaba usa la cantidad base
// y el valor restantes exactos; una parte se convierte con la proporción del renglón recibido.
func prepareReturnLines(receipt *Receipt, inputs []supplierReturnLineInput) ([]preparedReturnLine, error) {
	byNumber := map[int]*ReceiptLine{}
	for index := range receipt.Lines {
		byNumber[receipt.Lines[index].LineNo] = &receipt.Lines[index]
	}
	seen := map[int]bool{}
	prepared := make([]preparedReturnLine, 0, len(inputs))
	for index, input := range inputs {
		prefix := "Renglón " + itoa(index+1) + ": "
		line := byNumber[input.ReceiptLineNo]
		if line == nil {
			return nil, invalid(prefix + "el renglón " + itoa(input.ReceiptLineNo) + " no existe en la recepción.")
		}
		if seen[line.LineNo] {
			return nil, invalid("El renglón " + itoa(line.LineNo) + " de la recepción aparece más de una vez.")
		}
		seen[line.LineNo] = true
		quantity, err := parseQuantity(input.Quantity, prefix+"la cantidad")
		if err != nil {
			return nil, err
		}
		returnable := positivePart(line.quantity.Sub(line.returned))
		if quantity.GreaterThan(returnable) {
			return nil, conflict("RETURN_EXCEEDS_RECEIVED", fmt.Sprintf(
				"%s«%s»: se recibieron %s %s y ya se devolvieron %s; solo puedes devolver %s.", prefix, line.Item.Name,
				quantityText(line.quantity), line.CaptureLabel, quantityText(line.returned), quantityText(returnable)))
		}
		remainingBase := positivePart(line.base.Sub(line.returnedBase))
		next := preparedReturnLine{receiptLine: line, quantity: quantity}
		if quantity.Equal(returnable) {
			// Todo lo que quedaba del renglón: cantidad y valor restantes exactos.
			remainingValue := positivePart(line.inventoryValue.Sub(line.returnedValue))
			next.base = remainingBase
			if next.base.IsPositive() {
				next.cost = remainingValue.DivRound(next.base, costPrecision)
			}
		} else {
			next.base = quantity.Mul(line.base).DivRound(line.quantity, quantityScale)
			if next.base.GreaterThan(remainingBase) {
				next.base = remainingBase
			}
			next.cost = line.inventoryValue.DivRound(line.base, costPrecision)
		}
		if !next.base.IsPositive() {
			return nil, invalid(prefix + "la cantidad devuelta es demasiado pequeña para la unidad base del insumo.")
		}
		prepared = append(prepared, next)
	}
	return prepared, nil
}

// returnLineResult es lo que el libro registró para un renglón devuelto.
type returnLineResult struct {
	value        decimal.Decimal
	outflow      decimal.Decimal
	adjustment   decimal.Decimal
	adjustmentID *string
	unabsorbed   decimal.Decimal
	movement     LedgerMovement
}

// saveSupplierReturn registra los movimientos y el documento de la devolución.
func (handler *service) saveSupplierReturn(receipt *Receipt, warehouse *Warehouse, occurredAt time.Time, reason string,
	lines []preparedReturnLine) (any, error) {
	id := newID()
	movementReason := "Devolución a proveedor · " + receipt.Supplier.TradeName + " · Recepción " + fmt.Sprint(receipt.Folio)
	results := make([]returnLineResult, 0, len(lines))
	warnings := []Warning{}
	totalValue, totalUnabsorbed := zero, zero
	for index, line := range lines {
		number := index + 1
		cost := line.cost
		applied, err := Apply(handler.ctx, handler.tx, MovementInput{
			EntryKey:     "supplier_return:" + id + ":" + itoa(number),
			Kind:         KindSupplierReturn,
			ItemID:       line.receiptLine.Item.ID,
			WarehouseID:  warehouse.ID,
			Quantity:     line.base,
			OriginalCost: &cost,
			OccurredAt:   occurredAt,
			SourceKind:   "supplier_return",
			SourceID:     &id,
			SourceLine:   &number,
			Reason:       &movementReason,
			ActorID:      handler.actorReference(),
			OperationID:  handler.operationReference(),
		})
		if err != nil {
			return nil, err
		}
		result := returnLineResult{
			value: applied.DocumentValue, outflow: applied.Movement.Value.Neg(), unabsorbed: applied.Unabsorbed,
			movement: applied.Movement,
		}
		for _, adjustment := range applied.Adjustments {
			if strings.HasSuffix(adjustment.EntryKey, ":cost") {
				movementID := adjustment.ID
				result.adjustment, result.adjustmentID = adjustment.Value, &movementID
			}
		}
		if applied.NegativeStock {
			itemID := line.receiptLine.Item.ID
			warnings = append(warnings, Warning{
				Code: "NEGATIVE_STOCK", ItemID: &itemID,
				Message: "La existencia de «" + line.receiptLine.Item.Name + "» en «" + warehouse.Name + "» quedó negativa.",
			})
		}
		totalValue = totalValue.Add(result.value)
		totalUnabsorbed = totalUnabsorbed.Add(result.unabsorbed)
		results = append(results, result)
	}
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_supplier_returns(id,receipt_id,supplier_id,warehouse_id,occurred_at,
            reason,actor_id,value,unabsorbed,line_count,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8::numeric,$9::numeric,$10,$11)`,
		id, receipt.ID, receipt.Supplier.ID, warehouse.ID, occurredAt, reason, handler.request.Actor.UserID, totalValue.String(),
		totalUnabsorbed.String(), len(lines), handler.operationReference())
	if err != nil {
		return nil, err
	}
	audited := []map[string]any{}
	for index, line := range lines {
		result := results[index]
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_supplier_return_lines(return_id,line_no,receipt_id,
                receipt_line_no,item_id,quantity,base_quantity,original_unit_cost,value,outflow_value,cost_adjustment,unabsorbed,
                movement_id,cost_adjustment_movement_id)
            VALUES ($1,$2,$3,$4,$5,$6::numeric,$7::numeric,$8::numeric,$9::numeric,$10::numeric,$11::numeric,$12::numeric,$13,$14)`,
			id, index+1, receipt.ID, line.receiptLine.LineNo, line.receiptLine.Item.ID, line.quantity.String(), line.base.String(),
			line.receiptLine.unitCost.String(), result.value.String(), result.outflow.String(), result.adjustment.String(),
			result.unabsorbed.String(), result.movement.ID, result.adjustmentID)
		if err != nil {
			return nil, err
		}
		audited = append(audited, map[string]any{
			"line": index + 1, "receipt_line_no": line.receiptLine.LineNo, "item_id": line.receiptLine.Item.ID,
			"item_name": line.receiptLine.Item.Name, "quantity": line.quantity.String(), "base_quantity": line.base.String(),
			"value": result.value.String(), "outflow_value": result.outflow.String(), "cost_adjustment": result.adjustment.String(),
			"unabsorbed": result.unabsorbed.String(), "movement_id": result.movement.ID,
		})
	}
	if err := handler.publish("inventory.receipts.changed", &receipt.ID); err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.stock.changed", &warehouse.ID); err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.movements.recorded", &id); err != nil {
		return nil, err
	}
	after := map[string]any{
		"receipt_id": receipt.ID, "receipt_folio": receipt.Folio, "supplier_id": receipt.Supplier.ID,
		"warehouse_id": warehouse.ID, "occurred_at": occurredAt, "reason": reason, "value": totalValue.String(),
		"unabsorbed": totalUnabsorbed.String(), "lines": audited,
	}
	err = handler.audit(handler.ctx, handler.tx, "inventory.supplier_return_recorded", "inventory_supplier_return", &id, nil, after)
	if err != nil {
		return nil, err
	}
	saved, err := handler.readSupplierReturn(id)
	if err != nil {
		return nil, err
	}
	saved.Warnings = warnings
	return saved, nil
}

const supplierReturnColumns = `SELECT sr.id,sr.folio,sr.receipt_id,r.folio,s.id,s.trade_name,s.kind,s.active,w.id,w.name,sr.occurred_at,sr.reason,
        actor.display_name,sr.value::text,sr.line_count,sr.unabsorbed::text,sr.created_at,r.supplier_reference
        FROM app.inventory_supplier_returns sr
        JOIN app.inventory_receipts r ON r.id=sr.receipt_id
        JOIN app.inventory_suppliers s ON s.id=sr.supplier_id
        JOIN app.inventory_warehouses w ON w.id=sr.warehouse_id
        JOIN app.users actor ON actor.id=sr.actor_id`

func (handler *service) scanSupplierReturn(row interface{ Scan(...any) error }) (*SupplierReturn, error) {
	var document SupplierReturn
	summary := &document.SupplierReturnSummary
	var value, unabsorbed string
	err := row.Scan(&summary.ID, &summary.Folio, &summary.ReceiptID, &summary.ReceiptFolio, &summary.Supplier.ID,
		&summary.Supplier.TradeName, &summary.Supplier.Kind, &summary.Supplier.Active, &summary.Warehouse.ID, &summary.Warehouse.Name, &summary.OccurredAt,
		&summary.Reason, &summary.ActorName, &value, &summary.LineCount, &unabsorbed, &summary.CreatedAt, &summary.ReceiptReference)
	if err != nil {
		return nil, err
	}
	exact := parseDatabaseDecimal(value)
	summary.Value = money(roundMoney(exact))
	summary.CostsVisible = handler.costsVisible()
	if summary.CostsVisible {
		summary.Unabsorbed, summary.UnabsorbedDisplay = moneyPointers(parseDatabaseDecimal(unabsorbed))
		valueExact := exactMoneyText(exact)
		summary.ValueExact = &valueExact
	}
	document.Lines = []SupplierReturnLine{}
	document.Warnings = []Warning{}
	return &document, nil
}

// readSupplierReturn lee la devolución con sus renglones.
func (handler *service) readSupplierReturn(id string) (*SupplierReturn, error) {
	document, err := handler.scanSupplierReturn(handler.tx.QueryRow(handler.ctx, supplierReturnColumns+" WHERE sr.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("La devolución a proveedor no existe.")
	}
	if err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT l.line_no,l.receipt_line_no,i.id,i.code,i.name,i.kind,i.active,bu.id,bu.code,
            bu.name,bu.dimension,coalesce(p.name,cu.name),l.quantity::text,l.base_quantity::text,l.value::text,l.movement_id,
            m.stock_after::text,l.original_unit_cost::text,l.outflow_value::text,l.cost_adjustment::text,l.unabsorbed::text,
            l.cost_adjustment_movement_id
        FROM app.inventory_supplier_return_lines l
        JOIN app.inventory_receipt_lines rl ON rl.receipt_id=l.receipt_id AND rl.line_no=l.receipt_line_no
        JOIN app.inventory_items i ON i.id=l.item_id
        JOIN app.inventory_units bu ON bu.id=i.base_unit_id
        LEFT JOIN app.inventory_presentations p ON p.id=rl.presentation_id
        LEFT JOIN app.inventory_units cu ON cu.id=rl.unit_id
        JOIN app.inventory_movements m ON m.id=l.movement_id
        WHERE l.return_id=$1 ORDER BY l.line_no`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var line SupplierReturnLine
		var quantity, base, value, stockAfter, cost, outflow, adjustment, unabsorbed string
		err := rows.Scan(&line.LineNo, &line.ReceiptLineNo, &line.Item.ID, &line.Item.Code, &line.Item.Name, &line.Item.Kind,
			&line.Item.Active, &line.Item.Unit.ID, &line.Item.Unit.Code, &line.Item.Unit.Name, &line.Item.Unit.Dimension,
			&line.CaptureLabel, &quantity, &base, &value, &line.MovementID, &stockAfter, &cost, &outflow, &adjustment, &unabsorbed,
			&line.CostAdjustmentMovement)
		if err != nil {
			return nil, err
		}
		line.Quantity = quantityText(parseDatabaseDecimal(quantity))
		line.BaseQuantity = quantityText(parseDatabaseDecimal(base))
		line.Value = money(roundMoney(parseDatabaseDecimal(value)))
		line.StockAfter = quantityText(parseDatabaseDecimal(stockAfter))
		line.CostsVisible = handler.costsVisible()
		if line.CostsVisible {
			line.OriginalUnitCost, line.OriginalUnitCostShow = costPointers(parseDatabaseDecimal(cost))
			line.OutflowValue, line.OutflowValueDisplay = moneyPointers(parseDatabaseDecimal(outflow))
			line.CostAdjustment, line.CostAdjustmentDisplay = moneyPointers(parseDatabaseDecimal(adjustment))
			line.Unabsorbed, line.UnabsorbedDisplay = moneyPointers(parseDatabaseDecimal(unabsorbed))
		} else {
			line.CostAdjustmentMovement = nil
		}
		document.Lines = append(document.Lines, line)
	}
	return document, rows.Err()
}

func (handler *service) readSupplierReturnRoute(parts []string) (any, error) {
	return handler.readSupplierReturn(parts[1])
}

// supplierReturnSummaries lee las devoluciones que cumplen la condición (las más recientes primero).
func (handler *service) supplierReturnSummaries(where string, limit int, arguments ...any) ([]SupplierReturnSummary, error) {
	statement := supplierReturnColumns + " WHERE " + where + " ORDER BY sr.occurred_at DESC, sr.folio DESC LIMIT " + itoa(limit)
	rows, err := handler.tx.Query(handler.ctx, statement, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	summaries := []SupplierReturnSummary{}
	for rows.Next() {
		document, err := handler.scanSupplierReturn(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, document.SupplierReturnSummary)
	}
	return summaries, rows.Err()
}

// listSupplierReturns lista devoluciones con filtros de recepción, proveedor y días de negocio
// (AAAA-MM-DD en la zona del establecimiento), y paginación.
func (handler *service) listSupplierReturns(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "receipt_id", "supplier_id", "from", "until", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	for key, column := range map[string]string{"receipt_id": "sr.receipt_id", "supplier_id": "sr.supplier_id"} {
		identifier, err := optionalQueryID(query, key, "identificador")
		if err != nil {
			return nil, err
		}
		if identifier != nil {
			set.add(column+"=$%d", *identifier)
		}
	}
	from, until, err := handler.dateRange(query)
	if err != nil {
		return nil, err
	}
	if from != nil {
		set.add("sr.occurred_at>=$%d", *from)
	}
	if until != nil {
		set.add("sr.occurred_at<$%d", *until)
	}
	result := SupplierReturnPage{Items: []SupplierReturnSummary{}, Page: page, PageSize: size, CostsVisible: handler.costsVisible()}
	err = handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_supplier_returns sr"+set.where(), set.arguments...).
		Scan(&result.Total)
	if err != nil {
		return nil, err
	}
	statement := supplierReturnColumns + set.where() + " ORDER BY sr.occurred_at DESC, sr.folio DESC LIMIT " + set.next(size) +
		" OFFSET " + set.next((page-1)*size)
	rows, err := handler.tx.Query(handler.ctx, statement, set.arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		document, err := handler.scanSupplierReturn(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, document.SupplierReturnSummary)
	}
	return result, rows.Err()
}
