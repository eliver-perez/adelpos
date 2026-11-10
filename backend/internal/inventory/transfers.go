package inventory

import (
	"strings"
	"time"
)

// Transferencias entre almacenes (§21 de la especificación). Reglas:
//   - origen y destino distintos y activos; de 1 a 200 renglones, un insumo una vez;
//   - cada renglón registra con ApplyTransfer una salida y una entrada vinculadas con el mismo
//     valor: la cantidad y el valor del insumo no cambian (el costo es uno solo por insumo);
//   - transaccional e idempotente (operation_id y entry_key por renglón);
//   - no se bloquea por existencia negativa en el origen (aviso NEGATIVE_STOCK);
//   - no crea compras ni ventas; una transferencia equivocada se corrige con otra en sentido
//     contrario (el documento es de solo inserción).

// TransferSummary es una transferencia en listados. Value (con inventory.costs.read) es el valor
// trasladado.
type TransferSummary struct {
	ID            string             `json:"id"`
	Folio         int64              `json:"folio"`
	FromWarehouse WarehouseReference `json:"from_warehouse"`
	ToWarehouse   WarehouseReference `json:"to_warehouse"`
	OccurredAt    time.Time          `json:"occurred_at"`
	ActorName     string             `json:"actor_name"`
	Notes         string             `json:"notes"`
	LineCount     int                `json:"line_count"`
	CreatedAt     time.Time          `json:"created_at"`
	CostsVisible  bool               `json:"costs_visible"`
	Value         *string            `json:"value"`
	ValueDisplay  *string            `json:"value_display"`
}

// TransferDocument es la transferencia completa. Warnings solo viaja en la respuesta del registro.
type TransferDocument struct {
	TransferSummary
	Lines    []TransferLine `json:"lines"`
	Warnings []Warning      `json:"warnings"`
}

// TransferLine es un renglón con sus dos movimientos y la existencia de cada almacén después.
type TransferLine struct {
	LineNo           int                    `json:"line_no"`
	Item             ItemReference          `json:"item"`
	Presentation     *PresentationReference `json:"presentation"`
	Unit             *UnitReference         `json:"unit"`
	CaptureLabel     string                 `json:"capture_label"`
	CapturedQuantity string                 `json:"captured_quantity"`
	Quantity         string                 `json:"quantity"`
	OutMovementID    string                 `json:"out_movement_id"`
	InMovementID     string                 `json:"in_movement_id"`
	FromStockAfter   string                 `json:"from_stock_after"`
	ToStockAfter     string                 `json:"to_stock_after"`
	Value            *string                `json:"value"`
	ValueDisplay     *string                `json:"value_display"`
}

// TransferPage es una página de transferencias (la más reciente primero).
type TransferPage struct {
	Items        []TransferSummary `json:"items"`
	Total        int               `json:"total"`
	Page         int               `json:"page"`
	PageSize     int               `json:"page_size"`
	CostsVisible bool              `json:"costs_visible"`
}

type transferInput struct {
	OperationID     string             `json:"operation_id"`
	FromWarehouseID string             `json:"from_warehouse_id"`
	ToWarehouseID   string             `json:"to_warehouse_id"`
	Notes           string             `json:"notes"`
	Lines           []controlLineInput `json:"lines"`
}

const transferColumns = `SELECT t.id,t.folio,f.id,f.name,d.id,d.name,t.occurred_at,actor.display_name,t.notes,t.line_count,t.created_at,
        coalesce((SELECT sum(m.value) FROM app.inventory_movements m WHERE m.source_kind='transfer' AND m.source_id=t.id
            AND m.kind='transfer_in'),0)::text
        FROM app.inventory_transfers t
        JOIN app.inventory_warehouses f ON f.id=t.from_warehouse_id
        JOIN app.inventory_warehouses d ON d.id=t.to_warehouse_id
        JOIN app.users actor ON actor.id=t.actor_id`

func (handler *service) scanTransfer(row interface{ Scan(...any) error }) (*TransferDocument, error) {
	var transfer TransferDocument
	summary := &transfer.TransferSummary
	var value string
	err := row.Scan(&summary.ID, &summary.Folio, &summary.FromWarehouse.ID, &summary.FromWarehouse.Name, &summary.ToWarehouse.ID,
		&summary.ToWarehouse.Name, &summary.OccurredAt, &summary.ActorName, &summary.Notes, &summary.LineCount, &summary.CreatedAt, &value)
	if err != nil {
		return nil, err
	}
	summary.CostsVisible = handler.costsVisible()
	summary.Value, summary.ValueDisplay = handler.optionalMoneyPointers(parseDatabaseDecimal(value))
	transfer.Lines = []TransferLine{}
	transfer.Warnings = []Warning{}
	return &transfer, nil
}

// readTransfer lee la transferencia con sus renglones.
func (handler *service) readTransfer(id string) (*TransferDocument, error) {
	transfer, err := handler.scanTransfer(handler.tx.QueryRow(handler.ctx, transferColumns+" WHERE t.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("La transferencia no existe.")
	}
	if err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT l.line_no,`+controlLineColumns+`,l.captured_quantity::text,l.quantity::text,
            l.out_movement_id,l.in_movement_id,o.stock_after::text,n.stock_after::text,n.value::text
        FROM app.inventory_transfer_lines l`+controlLineJoins+`
        JOIN app.inventory_movements o ON o.id=l.out_movement_id
        JOIN app.inventory_movements n ON n.id=l.in_movement_id
        WHERE l.transfer_id=$1 ORDER BY l.line_no`, transfer.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var line TransferLine
		var captured, quantity, fromAfter, toAfter, value string
		targets, finish := controlLineTargets(&line.Item)
		if err := rows.Scan(append(append([]any{&line.LineNo}, targets...), &captured, &quantity, &line.OutMovementID,
			&line.InMovementID, &fromAfter, &toAfter, &value)...); err != nil {
			return nil, err
		}
		line.Presentation, line.Unit, line.CaptureLabel = finish()
		line.CapturedQuantity = quantityText(parseDatabaseDecimal(captured))
		line.Quantity = quantityText(parseDatabaseDecimal(quantity))
		line.FromStockAfter = quantityText(parseDatabaseDecimal(fromAfter))
		line.ToStockAfter = quantityText(parseDatabaseDecimal(toAfter))
		line.Value, line.ValueDisplay = handler.optionalMoneyPointers(parseDatabaseDecimal(value))
		transfer.Lines = append(transfer.Lines, line)
	}
	return transfer, rows.Err()
}

func (handler *service) readTransferRoute(parts []string) (any, error) {
	return handler.readTransfer(parts[1])
}

// listTransfers lista transferencias con filtros de almacén (origen o destino), insumo y fecha
// (días de negocio), búsqueda por folio y paginación.
func (handler *service) listTransfers(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "warehouse_id", "item_id", "from", "until", "search", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	warehouseID, err := optionalQueryID(query, "warehouse_id", "almacén")
	if err != nil {
		return nil, err
	}
	if warehouseID != nil {
		set.add("(t.from_warehouse_id=$%[1]d OR t.to_warehouse_id=$%[1]d)", *warehouseID)
	}
	itemID, err := optionalQueryID(query, "item_id", "insumo")
	if err != nil {
		return nil, err
	}
	if itemID != nil {
		set.add("EXISTS(SELECT FROM app.inventory_transfer_lines l WHERE l.transfer_id=t.id AND l.item_id=$%d)", *itemID)
	}
	if search := strings.TrimSpace(query.Get("search")); search != "" {
		set.add("t.folio::text=$%d", search)
	}
	from, until, err := handler.dateRange(query)
	if err != nil {
		return nil, err
	}
	if from != nil {
		set.add("t.occurred_at>=$%d", *from)
	}
	if until != nil {
		set.add("t.occurred_at<$%d", *until)
	}
	result := TransferPage{Items: []TransferSummary{}, Page: page, PageSize: size, CostsVisible: handler.costsVisible()}
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_transfers t"+set.where(), set.arguments...).
		Scan(&result.Total); err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, transferColumns+set.where()+" ORDER BY t.occurred_at DESC, t.folio DESC LIMIT "+
		set.next(size)+" OFFSET "+set.next((page-1)*size), set.arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		transfer, err := handler.scanTransfer(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, transfer.TransferSummary)
	}
	return result, rows.Err()
}

// recordTransfer registra una transferencia entre dos almacenes.
func (handler *service) recordTransfer(_ []string) (any, error) {
	var input transferInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	origin, err := handler.activeWarehouse(input.FromWarehouseID, "el almacén de origen")
	if err != nil {
		return nil, err
	}
	destination, err := handler.activeWarehouse(input.ToWarehouseID, "el almacén de destino")
	if err != nil {
		return nil, err
	}
	if origin.ID == destination.ID {
		return nil, invalid("El almacén de origen y el de destino deben ser distintos.")
	}
	notes, err := controlNotes(input.Notes, 500)
	if err != nil {
		return nil, err
	}
	if len(input.Lines) == 0 || len(input.Lines) > maximumControlLines {
		return nil, invalid("La transferencia requiere de 1 a 200 renglones.")
	}
	captures := make([]purchaseCapture, 0, len(input.Lines))
	seen := map[string]bool{}
	for index, line := range input.Lines {
		capture, err := handler.controlCapture(line.ItemID, line.PresentationID, line.UnitID, line.Quantity,
			"Renglón "+itoa(index+1)+": ", false, false)
		if err != nil {
			return nil, err
		}
		if seen[capture.item.ID] {
			return nil, invalid("El insumo «" + capture.item.Name + "» aparece en más de un renglón.")
		}
		seen[capture.item.ID] = true
		captures = append(captures, capture)
	}
	now, err := handler.transactionTime()
	if err != nil {
		return nil, err
	}
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_transfers(id,from_warehouse_id,to_warehouse_id,occurred_at,actor_id,
            notes,line_count,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, id, origin.ID, destination.ID, now, handler.request.Actor.UserID, notes, len(captures),
		handler.operationReference())
	if err != nil {
		return nil, err
	}
	warnings := []Warning{}
	reason := "Transferencia de «" + origin.Name + "» a «" + destination.Name + "»"
	if textLength(reason) > 500 {
		reason = "Transferencia entre almacenes"
	}
	for index, capture := range captures {
		number := index + 1
		applied, err := ApplyTransfer(handler.ctx, handler.tx, TransferInput{
			EntryKey: "transfer:" + id + ":" + itoa(number), ItemID: capture.item.ID, FromWarehouseID: origin.ID,
			ToWarehouseID: destination.ID, Quantity: capture.base, OccurredAt: now, SourceKind: "transfer", SourceID: &id,
			SourceLine: &number, Reason: &reason, ActorID: handler.actorReference(), OperationID: handler.operationReference(),
		})
		if err != nil {
			return nil, err
		}
		presentationID, unitID := captureColumns(capture)
		_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_transfer_lines(transfer_id,line_no,item_id,presentation_id,unit_id,
                captured_quantity,quantity,out_movement_id,in_movement_id)
            VALUES ($1,$2,$3,$4,$5,$6::numeric,$7::numeric,$8,$9)`, id, number, capture.item.ID, presentationID, unitID,
			capture.quantity.String(), capture.base.String(), applied.Out.ID, applied.In.ID)
		if err != nil {
			return nil, err
		}
		if applied.NegativeStock {
			warnings = append(warnings, negativeStockWarning(itemReferenceOf(capture.item), origin.Name))
		}
	}
	saved, err := handler.readTransfer(id)
	if err != nil {
		return nil, err
	}
	saved.Warnings = warnings
	for _, event := range []struct {
		kind      string
		aggregate string
	}{
		{"inventory.transfers.changed", id}, {"inventory.stock.changed", origin.ID}, {"inventory.stock.changed", destination.ID},
		{"inventory.movements.recorded", id}, {"inventory.alerts.changed", id},
	} {
		aggregate := event.aggregate
		if err := handler.publish(event.kind, &aggregate); err != nil {
			return nil, err
		}
	}
	lines := []map[string]any{}
	for _, line := range saved.Lines {
		lines = append(lines, map[string]any{
			"line": line.LineNo, "item_id": line.Item.ID, "item_name": line.Item.Name, "captured_quantity": line.CapturedQuantity,
			"capture_label": line.CaptureLabel, "quantity": line.Quantity, "out_movement_id": line.OutMovementID,
			"in_movement_id": line.InMovementID,
		})
	}
	after := map[string]any{
		"folio": saved.Folio, "from_warehouse_id": origin.ID, "from_warehouse_name": origin.Name, "to_warehouse_id": destination.ID,
		"to_warehouse_name": destination.Name, "occurred_at": now, "notes": notes, "lines": lines,
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.transfer_recorded", "inventory_transfer", &id, nil, after)
}
