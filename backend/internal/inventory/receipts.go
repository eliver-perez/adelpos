package inventory

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Recepciones de mercancía (§18 de la especificación). Una recepción:
//   - exige inventory.purchases.receive; la recepción directa (sin orden) exige además
//     inventory.purchases.create, porque equivale a comprar sin orden previa;
//   - tiene proveedor obligatorio (el de la orden o, en la directa, uno activo), almacén (el previsto
//     por la orden o el predeterminado) y referencia del proveedor opcional (factura o remisión);
//   - es idempotente por operation_id: repetirla devuelve la misma respuesta y nunca recibe dos
//     veces; otro cuerpo con el mismo identificador responde 409;
//   - recibe total o parcialmente los renglones de una orden emitida: recibir más de lo pendiente
//     solo con allow_excess explícito (aviso RECEIPT_EXCESS);
//   - copia purchase_tax_in_cost de la configuración: el valor de inventario de cada renglón es su
//     total con IVA o su subtotal sin IVA, y el costo por unidad base es ese valor ÷ cantidad base;
//   - registra un movimiento receipt por renglón con el libro (regla 2 del motor: con existencia
//     negativa, el ajuste de costo va en un movimiento separado) y nunca modifica movimientos
//     anteriores;
//   - actualiza el estado de la orden (parcial o recibida) y el último precio del proveedor;
//   - no crea gastos de caja: el vínculo con un gasto es opcional y explícito (cash_links.go).

const maximumReceiptLines = 200

type receiptInput struct {
	OperationID string `json:"operation_id"`
	// PurchaseOrderID vacío es una recepción directa.
	PurchaseOrderID *string `json:"purchase_order_id"`
	// SupplierID es obligatorio en la directa; con orden, si se envía debe ser el de la orden.
	SupplierID        *string            `json:"supplier_id"`
	WarehouseID       *string            `json:"warehouse_id"`
	OccurredAt        *time.Time         `json:"occurred_at"`
	SupplierReference string             `json:"supplier_reference"`
	Notes             string             `json:"notes"`
	AllowExcess       bool               `json:"allow_excess"`
	Lines             []receiptLineInput `json:"lines"`
}

// receiptLineInput es un renglón recibido. Con orden: order_line_no y quantity (en la presentación o
// unidad del renglón de la orden); precio, descuento y tasa son opcionales y por omisión los de la
// orden (el descuento, prorrateado). Directa: insumo, presentación o unidad, cantidad, precio y tasa.
type receiptLineInput struct {
	OrderLineNo    *int    `json:"order_line_no"`
	ItemID         *string `json:"item_id"`
	PresentationID *string `json:"presentation_id"`
	UnitID         *string `json:"unit_id"`
	Quantity       string  `json:"quantity"`
	UnitPrice      *string `json:"unit_price"`
	Discount       *string `json:"discount"`
	TaxRate        *int    `json:"tax_rate"`
}

// ReceiptSummary es una recepción en listados e historiales. Los importes de compra los ve quien
// consulta compras; el valor de inventario solo con inventory.costs.read.
type ReceiptSummary struct {
	ID                 string             `json:"id"`
	Folio              int64              `json:"folio"`
	Supplier           SupplierReference  `json:"supplier"`
	PurchaseOrderID    *string            `json:"purchase_order_id"`
	PurchaseOrderFolio *int64             `json:"purchase_order_folio"`
	Direct             bool               `json:"direct"`
	Warehouse          WarehouseReference `json:"warehouse"`
	OccurredAt         time.Time          `json:"occurred_at"`
	ReceivedByName     string             `json:"received_by_name"`
	SupplierReference  string             `json:"supplier_reference"`
	TaxInCost          bool               `json:"tax_in_cost"`
	Subtotal           string             `json:"subtotal"`
	Discount           string             `json:"discount"`
	Tax                string             `json:"tax"`
	Total              string             `json:"total"`
	LineCount          int                `json:"line_count"`
	Excess             bool               `json:"excess"`
	// LinkedAmount es Σ vínculos vigentes con gastos de caja y LinkableAmount lo que aún se puede
	// vincular (total − vinculado).
	LinkedAmount   string `json:"linked_amount"`
	LinkableAmount string `json:"linkable_amount"`
	// CashLinkAttention indica que algún gasto vinculado se corrigió o regularizó después del vínculo.
	CashLinkAttention bool `json:"cash_link_attention"`
	// ReturnedValue es el valor asociado de las devoluciones a proveedor de la recepción.
	ReturnedValue         string    `json:"returned_value"`
	CostsVisible          bool      `json:"costs_visible"`
	InventoryValue        *string   `json:"inventory_value"`
	InventoryValueDisplay *string   `json:"inventory_value_display"`
	CreatedAt             time.Time `json:"created_at"`
}

// Receipt es la recepción completa con renglones, devoluciones y vínculos con gastos. Warnings solo
// viaja en la respuesta del registro.
type Receipt struct {
	ReceiptSummary
	Notes     string                  `json:"notes"`
	Lines     []ReceiptLine           `json:"lines"`
	Returns   []SupplierReturnSummary `json:"returns"`
	CashLinks []CashLink              `json:"cash_links"`
	Warnings  []Warning               `json:"warnings"`
}

// ReceiptLine es un renglón recibido. Con orden muestra lo solicitado, lo recibido antes por otras
// recepciones y lo pendiente después de esta; siempre, lo devuelto y lo que aún se puede devolver.
type ReceiptLine struct {
	PurchaseLine
	OrderLineNo            *int    `json:"order_line_no"`
	OrderedQuantity        *string `json:"ordered_quantity"`
	ReceivedBeforeQuantity *string `json:"received_before_quantity"`
	PendingAfterQuantity   *string `json:"pending_after_quantity"`
	ReturnedQuantity       string  `json:"returned_quantity"`
	ReturnableQuantity     string  `json:"returnable_quantity"`
	ReturnedBaseQuantity   string  `json:"returned_base_quantity"`
	ReturnableBaseQuantity string  `json:"returnable_base_quantity"`
	MovementID             string  `json:"movement_id"`
	StockAfter             string  `json:"stock_after"`
	CostsVisible           bool    `json:"costs_visible"`
	InventoryValue         *string `json:"inventory_value"`
	InventoryValueDisplay  *string `json:"inventory_value_display"`
	UnitCost               *string `json:"unit_cost"`
	UnitCostDisplay        *string `json:"unit_cost_display"`
	// Datos para devolver.
	quantity       decimal.Decimal
	base           decimal.Decimal
	inventoryValue decimal.Decimal
	unitCost       decimal.Decimal
	returned       decimal.Decimal
	returnedBase   decimal.Decimal
	returnedValue  decimal.Decimal
}

// ReceiptPage es una página del listado de recepciones (la más reciente primero).
type ReceiptPage struct {
	Items        []ReceiptSummary `json:"items"`
	Total        int              `json:"total"`
	Page         int              `json:"page"`
	PageSize     int              `json:"page_size"`
	CostsVisible bool             `json:"costs_visible"`
}

// recordReceipt registra una recepción de una orden o directa.
func (handler *service) recordReceipt(_ []string) (any, error) {
	var input receiptInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	direct := input.PurchaseOrderID == nil || strings.TrimSpace(*input.PurchaseOrderID) == ""
	if direct {
		// Recibir sin orden equivale a comprar: exige también crear órdenes de compra.
		if err := handler.request.Actor.Require(permissionPurchasesCreate); err != nil {
			return nil, err
		}
	}
	if len(input.Lines) == 0 || len(input.Lines) > maximumReceiptLines {
		return nil, invalid("La recepción requiere de 1 a 200 renglones.")
	}
	reference, err := purchaseText(input.SupplierReference, "La referencia del proveedor", 60, true)
	if err != nil {
		return nil, err
	}
	notes, err := purchaseText(input.Notes, "Las observaciones de la recepción", 2000, false)
	if err != nil {
		return nil, err
	}
	occurredAt, err := handler.initialTime(input.OccurredAt)
	if err != nil {
		return nil, invalid("La fecha de la recepción no puede ser futura.")
	}
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	var supplier *Supplier
	var order *PurchaseOrder
	var lines []preparedPurchaseLine
	warehouseID := input.WarehouseID
	if direct {
		if input.SupplierID == nil || strings.TrimSpace(*input.SupplierID) == "" {
			return nil, invalid("La recepción directa requiere el proveedor.")
		}
		if supplier, err = handler.supplierForPurchase(*input.SupplierID); err != nil {
			return nil, err
		}
		if lines, err = handler.prepareDirectReceipt(input.Lines); err != nil {
			return nil, err
		}
	} else {
		if order, lines, err = handler.prepareOrderReceipt(input); err != nil {
			return nil, err
		}
		if supplier, err = handler.readSupplier(order.Supplier.ID); err != nil {
			return nil, err
		}
		if warehouseID == nil || strings.TrimSpace(*warehouseID) == "" {
			warehouseID = &order.ExpectedWarehouse.ID
		}
	}
	warehouse, err := handler.purchaseWarehouse(warehouseID)
	if err != nil {
		return nil, err
	}
	return handler.saveReceipt(savedReceipt{
		supplier: supplier, order: order, warehouse: warehouse, occurredAt: occurredAt, reference: reference, notes: notes,
		taxInCost: settings.PurchaseTaxInCost, lines: lines,
	})
}

// prepareDirectReceipt valida los renglones de una recepción sin orden.
func (handler *service) prepareDirectReceipt(inputs []receiptLineInput) ([]preparedPurchaseLine, error) {
	converted := make([]purchaseLineInput, 0, len(inputs))
	for index, line := range inputs {
		prefix := "Renglón " + itoa(index+1) + ": "
		if line.OrderLineNo != nil {
			return nil, invalid(prefix + "una recepción directa no recibe renglones de una orden.")
		}
		if line.ItemID == nil {
			return nil, invalid(prefix + "elige el insumo.")
		}
		if line.UnitPrice == nil {
			return nil, invalid(prefix + "captura el precio unitario sin IVA.")
		}
		converted = append(converted, purchaseLineInput{
			ItemID: *line.ItemID, PresentationID: line.PresentationID, UnitID: line.UnitID, Quantity: line.Quantity,
			UnitPrice: *line.UnitPrice, Discount: line.Discount, TaxRate: line.TaxRate,
		})
	}
	lines, _, err := handler.preparePurchaseLines(converted)
	return lines, err
}

// prepareOrderReceipt valida los renglones recibidos de una orden emitida o parcialmente recibida:
// cada renglón de la orden una sola vez, en su misma presentación o unidad, sin exceder lo pendiente
// salvo con allow_excess. Precio y tasa son los de la orden salvo que se capturen; el descuento de
// la orden se prorratea por lo recibido (proratedDiscount) salvo que se capture otro.
func (handler *service) prepareOrderReceipt(input receiptInput) (*PurchaseOrder, []preparedPurchaseLine, error) {
	if err := validateID(*input.PurchaseOrderID); err != nil {
		return nil, nil, invalid("La orden de compra no es válida.")
	}
	order, err := handler.readPurchaseOrder(strings.ToLower(*input.PurchaseOrderID))
	if err != nil {
		return nil, nil, err
	}
	if !order.Receivable {
		return nil, nil, conflict("ORDER_NOT_RECEIVABLE", "La orden de compra está «"+order.StateLabel+"»: no admite recepciones.")
	}
	if input.SupplierID != nil && strings.TrimSpace(*input.SupplierID) != "" &&
		!strings.EqualFold(strings.TrimSpace(*input.SupplierID), order.Supplier.ID) {
		return nil, nil, invalid("El proveedor de la recepción debe ser el de la orden de compra.")
	}
	byNumber := map[int]*PurchaseOrderLine{}
	for index := range order.Lines {
		byNumber[order.Lines[index].LineNo] = &order.Lines[index]
	}
	seen := map[int]bool{}
	prepared := make([]preparedPurchaseLine, 0, len(input.Lines))
	for index, line := range input.Lines {
		number := index + 1
		prefix := "Renglón " + itoa(number) + ": "
		if line.OrderLineNo == nil {
			return nil, nil, invalid(prefix + "indica el renglón de la orden que recibes.")
		}
		if line.ItemID != nil || line.PresentationID != nil || line.UnitID != nil {
			return nil, nil, invalid(prefix + "el insumo y su presentación o unidad son los del renglón de la orden.")
		}
		ordered := byNumber[*line.OrderLineNo]
		if ordered == nil {
			return nil, nil, invalid(prefix + "el renglón " + itoa(*line.OrderLineNo) + " no existe en la orden.")
		}
		if seen[ordered.LineNo] {
			return nil, nil, invalid("El renglón " + itoa(ordered.LineNo) + " de la orden aparece más de una vez.")
		}
		seen[ordered.LineNo] = true
		next, err := handler.prepareOrderReceiptLine(ordered, line, prefix, input.AllowExcess)
		if err != nil {
			return nil, nil, err
		}
		prepared = append(prepared, next)
	}
	return order, prepared, nil
}

// prepareOrderReceiptLine valida un renglón recibido de una orden y calcula sus importes.
func (handler *service) prepareOrderReceiptLine(ordered *PurchaseOrderLine, line receiptLineInput, prefix string,
	allowExcess bool) (preparedPurchaseLine, error) {
	capture, err := handler.resolveCapture(ordered.Item.ID, ordered.presentationID, ordered.unitID, line.Quantity, prefix, true)
	if err != nil {
		return preparedPurchaseLine{}, err
	}
	pending := positivePart(ordered.ordered.Sub(ordered.received))
	excess := positivePart(capture.quantity.Sub(pending))
	if excess.IsPositive() && !allowExcess {
		return preparedPurchaseLine{}, conflict("RECEIPT_EXCEEDS_PENDING", fmt.Sprintf(
			"%s«%s» tiene pendiente %s %s y se capturó %s. Para recibir de más confirma explícitamente el excedente.",
			prefix, ordered.Item.Name, quantityText(pending), capture.label, quantityText(capture.quantity)))
	}
	price := ordered.unitPrice
	if line.UnitPrice != nil {
		if price, err = parsePurchasePrice(*line.UnitPrice, prefix+"el precio unitario sin IVA"); err != nil {
			return preparedPurchaseLine{}, err
		}
	}
	rate := ordered.TaxRate
	if line.TaxRate != nil {
		if rate, err = validTaxRate(line.TaxRate, strings.TrimSuffix(prefix, ": ")); err != nil {
			return preparedPurchaseLine{}, err
		}
	}
	var discount decimal.Decimal
	if line.Discount != nil && strings.TrimSpace(*line.Discount) != "" {
		if discount, err = parsePurchaseAmount(*line.Discount, prefix+"el descuento"); err != nil {
			return preparedPurchaseLine{}, err
		}
	} else {
		// El prorrateo nunca supera el importe de lo recibido (por ejemplo, con un precio capturado menor).
		discount = proratedDiscount(ordered.discount, ordered.ordered, ordered.received, capture.quantity)
		gross := roundMoney(capture.quantity.Mul(price))
		if discount.GreaterThan(gross) {
			discount = gross
		}
	}
	amounts, err := purchaseLineAmounts(capture.quantity, price, discount, rate)
	if err != nil {
		return preparedPurchaseLine{}, prefixed(prefix, err)
	}
	number := ordered.LineNo
	return preparedPurchaseLine{
		capture: capture, unitPrice: price, taxRate: rate, amounts: amounts, orderLineNo: &number, excess: excess,
	}, nil
}

// savedReceipt son los datos validados de una recepción listos para registrar.
type savedReceipt struct {
	supplier   *Supplier
	order      *PurchaseOrder
	warehouse  *Warehouse
	occurredAt time.Time
	reference  string
	notes      string
	taxInCost  bool
	lines      []preparedPurchaseLine
}

// inventoryValueOf es el valor de inventario de un renglón: el total con IVA o el subtotal sin IVA.
func inventoryValueOf(amounts PurchaseAmounts, taxInCost bool) decimal.Decimal {
	if taxInCost {
		return amounts.Total
	}
	return amounts.Subtotal
}

// saveReceipt guarda la recepción, registra un movimiento receipt por renglón, actualiza el último
// precio del proveedor y el estado de la orden, y publica y audita en la misma transacción.
func (handler *service) saveReceipt(data savedReceipt) (any, error) {
	totals := PurchaseAmounts{}
	inventoryValue := zero
	excess := false
	for _, line := range data.lines {
		totals.add(line.amounts)
		inventoryValue = inventoryValue.Add(inventoryValueOf(line.amounts, data.taxInCost))
		excess = excess || line.excess.IsPositive()
	}
	id := newID()
	var orderID *string
	if data.order != nil {
		orderID = &data.order.ID
	}
	var folio int64
	err := handler.tx.QueryRow(handler.ctx, `INSERT INTO app.inventory_receipts(id,supplier_id,purchase_order_id,warehouse_id,occurred_at,
            received_by,supplier_reference,tax_in_cost,subtotal,discount,tax,total,inventory_value,line_count,excess,notes,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::numeric,$10::numeric,$11::numeric,$12::numeric,$13::numeric,$14,$15,$16,$17)
        RETURNING folio`,
		id, data.supplier.ID, orderID, data.warehouse.ID, data.occurredAt, handler.request.Actor.UserID, data.reference, data.taxInCost,
		money(totals.Subtotal), money(totals.Discount), money(totals.Tax), money(totals.Total), inventoryValue.String(),
		len(data.lines), excess, data.notes, handler.operationReference()).Scan(&folio)
	if err != nil {
		return nil, err
	}
	reason := "Recepción " + fmt.Sprint(folio) + " · " + data.supplier.TradeName
	warnings := []Warning{}
	audited := []map[string]any{}
	for index, line := range data.lines {
		number := index + 1
		value := inventoryValueOf(line.amounts, data.taxInCost)
		applied, err := Apply(handler.ctx, handler.tx, MovementInput{
			EntryKey:    "receipt:" + id + ":" + itoa(number),
			Kind:        KindReceipt,
			ItemID:      line.capture.item.ID,
			WarehouseID: data.warehouse.ID,
			Quantity:    line.capture.base,
			Value:       &value,
			OccurredAt:  data.occurredAt,
			SourceKind:  "receipt",
			SourceID:    &id,
			SourceLine:  &number,
			Reason:      &reason,
			ActorID:     handler.actorReference(),
			OperationID: handler.operationReference(),
		})
		if err != nil {
			return nil, err
		}
		unitCost := zero
		if applied.Movement.UnitCost != nil {
			unitCost = *applied.Movement.UnitCost
		}
		_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_receipt_lines(receipt_id,line_no,order_id,order_line_no,item_id,
                presentation_id,unit_id,quantity,base_quantity,unit_price,discount,tax_rate,subtotal,tax,total,inventory_value,unit_cost,
                movement_id)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8::numeric,$9::numeric,$10::numeric,$11::numeric,$12,$13::numeric,$14::numeric,$15::numeric,
                $16::numeric,$17::numeric,$18)`,
			id, number, orderID, line.orderLineNo, line.capture.item.ID, line.capture.presentationID, unitWhenNoPresentation(line.capture),
			line.capture.quantity.String(), line.capture.base.String(), line.unitPrice.String(), money(line.amounts.Discount),
			line.taxRate, money(line.amounts.Subtotal), money(line.amounts.Tax), money(line.amounts.Total), value.String(),
			unitCost.String(), applied.Movement.ID)
		if err != nil {
			return nil, err
		}
		if err := handler.recordSupplierPrice(data.supplier.ID, id, line, data.occurredAt); err != nil {
			return nil, err
		}
		if line.excess.IsPositive() {
			itemID := line.capture.item.ID
			warnings = append(warnings, Warning{
				Code: "RECEIPT_EXCESS",
				Message: "Se recibió más de lo pendiente de «" + line.capture.item.Name + "»: excedente de " +
					quantityText(line.excess) + " " + line.capture.label + ".",
				ItemID: &itemID,
			})
		}
		audited = append(audited, map[string]any{
			"line": number, "order_line_no": line.orderLineNo, "item_id": line.capture.item.ID, "item_name": line.capture.item.Name,
			"quantity": line.capture.quantity.String(), "capture": line.capture.label, "base_quantity": line.capture.base.String(),
			"unit_price": line.unitPrice.String(), "discount": money(line.amounts.Discount), "tax_rate": line.taxRate,
			"subtotal": money(line.amounts.Subtotal), "tax": money(line.amounts.Tax), "total": money(line.amounts.Total),
			"inventory_value": value.String(), "movement_id": applied.Movement.ID, "excess": line.excess.String(),
		})
	}
	if data.order != nil {
		if err := handler.refreshOrderState(data.order.ID); err != nil {
			return nil, err
		}
	}
	if err := handler.publishReceipt(id, orderID, data.warehouse.ID); err != nil {
		return nil, err
	}
	after := map[string]any{
		"folio": folio, "supplier_id": data.supplier.ID, "supplier_name": data.supplier.TradeName, "purchase_order_id": orderID,
		"warehouse_id": data.warehouse.ID, "occurred_at": data.occurredAt, "supplier_reference": data.reference,
		"tax_in_cost": data.taxInCost, "subtotal": money(totals.Subtotal), "discount": money(totals.Discount),
		"tax": money(totals.Tax), "total": money(totals.Total), "inventory_value": inventoryValue.String(), "lines": audited,
	}
	if err := handler.audit(handler.ctx, handler.tx, "inventory.receipt_recorded", "inventory_receipt", &id, nil, after); err != nil {
		return nil, err
	}
	receipt, err := handler.readReceipt(id)
	if err != nil {
		return nil, err
	}
	receipt.Warnings = warnings
	return receipt, nil
}

// publishReceipt publica los cambios de una recepción: el documento, la orden (si la hay), las
// existencias del almacén y los movimientos del libro.
func (handler *service) publishReceipt(receiptID string, orderID *string, warehouseID string) error {
	if err := handler.publish("inventory.receipts.changed", &receiptID); err != nil {
		return err
	}
	if orderID != nil {
		if err := handler.publish("inventory.purchases.changed", orderID); err != nil {
			return err
		}
	}
	if err := handler.publish("inventory.stock.changed", &warehouseID); err != nil {
		return err
	}
	return handler.publish("inventory.movements.recorded", &receiptID)
}

// refreshOrderState deja la orden «recibida» si todos sus renglones alcanzaron lo ordenado y
// «parcialmente recibida» en otro caso; aumenta la revisión (otra pantalla debe actualizarse).
func (handler *service) refreshOrderState(orderID string) error {
	var complete bool
	err := handler.tx.QueryRow(handler.ctx, `SELECT bool_and(coalesce(received.quantity,0)>=l.quantity)
        FROM app.inventory_purchase_order_lines l
        LEFT JOIN (SELECT order_line_no,sum(quantity) AS quantity FROM app.inventory_receipt_lines WHERE order_id=$1
            GROUP BY order_line_no) received ON received.order_line_no=l.line_no
        WHERE l.order_id=$1`, orderID).Scan(&complete)
	if err != nil {
		return err
	}
	state := "partially_received"
	if complete {
		state = "received"
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_purchase_orders SET state=$2,revision=revision+1,updated_at=now()
        WHERE id=$1`, orderID, state)
	return err
}

const receiptColumns = `SELECT r.id,r.folio,s.id,s.trade_name,s.active,r.purchase_order_id,o.folio,w.id,w.name,r.occurred_at,
        receiver.display_name,r.supplier_reference,r.tax_in_cost,r.subtotal::text,r.discount::text,r.tax::text,r.total::text,
        r.inventory_value::text,r.line_count,r.excess,r.notes,r.created_at,
        coalesce((SELECT sum(cl.amount) FROM app.inventory_receipt_cash_links cl
            WHERE cl.receipt_id=r.id AND cl.unlinked_at IS NULL),0)::text,
        EXISTS(SELECT FROM app.inventory_receipt_cash_links cl
            JOIN app.cash_operations fix ON fix.corrects_operation_id=cl.cash_operation_id
            WHERE cl.receipt_id=r.id AND cl.unlinked_at IS NULL),
        coalesce((SELECT sum(sr.value) FROM app.inventory_supplier_returns sr WHERE sr.receipt_id=r.id),0)::text
        FROM app.inventory_receipts r
        JOIN app.inventory_suppliers s ON s.id=r.supplier_id
        JOIN app.inventory_warehouses w ON w.id=r.warehouse_id
        JOIN app.users receiver ON receiver.id=r.received_by
        LEFT JOIN app.inventory_purchase_orders o ON o.id=r.purchase_order_id`

func (handler *service) scanReceipt(row interface{ Scan(...any) error }) (*Receipt, error) {
	var receipt Receipt
	summary := &receipt.ReceiptSummary
	var subtotal, discount, tax, total, value, linked, returned string
	err := row.Scan(&summary.ID, &summary.Folio, &summary.Supplier.ID, &summary.Supplier.TradeName, &summary.Supplier.Active,
		&summary.PurchaseOrderID, &summary.PurchaseOrderFolio, &summary.Warehouse.ID, &summary.Warehouse.Name, &summary.OccurredAt,
		&summary.ReceivedByName, &summary.SupplierReference, &summary.TaxInCost, &subtotal, &discount, &tax, &total, &value,
		&summary.LineCount, &summary.Excess, &receipt.Notes, &summary.CreatedAt, &linked, &summary.CashLinkAttention, &returned)
	if err != nil {
		return nil, err
	}
	totalValue := parseDatabaseDecimal(total)
	linkedValue := parseDatabaseDecimal(linked)
	summary.Direct = summary.PurchaseOrderID == nil
	summary.Subtotal = money(parseDatabaseDecimal(subtotal))
	summary.Discount = money(parseDatabaseDecimal(discount))
	summary.Tax = money(parseDatabaseDecimal(tax))
	summary.Total = money(totalValue)
	summary.LinkedAmount = money(linkedValue)
	summary.LinkableAmount = money(positivePart(totalValue.Sub(linkedValue)))
	summary.ReturnedValue = money(roundMoney(parseDatabaseDecimal(returned)))
	summary.CostsVisible = handler.costsVisible()
	if summary.CostsVisible {
		summary.InventoryValue, summary.InventoryValueDisplay = moneyPointers(parseDatabaseDecimal(value))
	}
	receipt.Lines = []ReceiptLine{}
	receipt.Returns = []SupplierReturnSummary{}
	receipt.CashLinks = []CashLink{}
	receipt.Warnings = []Warning{}
	return &receipt, nil
}

// readReceipt lee la recepción con renglones, devoluciones y vínculos con gastos.
func (handler *service) readReceipt(id string) (*Receipt, error) {
	receipt, err := handler.scanReceipt(handler.tx.QueryRow(handler.ctx, receiptColumns+" WHERE r.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("La recepción no existe.")
	}
	if err != nil {
		return nil, err
	}
	if receipt.Lines, err = handler.receiptLines(receipt.ID, receipt.Folio); err != nil {
		return nil, err
	}
	if receipt.Returns, err = handler.supplierReturnSummaries("sr.receipt_id=$1", maximumReceiptLines, receipt.ID); err != nil {
		return nil, err
	}
	if receipt.CashLinks, err = handler.cashLinks("cl.receipt_id=$1", receipt.ID); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (handler *service) readReceiptRoute(parts []string) (any, error) {
	return handler.readReceipt(parts[1])
}

// receiptLines lee los renglones de una recepción con el contexto de su orden (solicitado, recibido
// por recepciones anteriores y pendiente después de esta) y lo devuelto.
func (handler *service) receiptLines(receiptID string, folio int64) ([]ReceiptLine, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT `+purchaseLineColumns+`,l.order_line_no,ol.quantity::text,
            (SELECT coalesce(sum(prior.quantity),0) FROM app.inventory_receipt_lines prior
                JOIN app.inventory_receipts pr ON pr.id=prior.receipt_id
                WHERE prior.order_id=l.order_id AND prior.order_line_no=l.order_line_no AND pr.folio<$2)::text,
            l.inventory_value::text,l.unit_cost::text,l.movement_id,m.stock_after::text,
            coalesce(returned.quantity,0)::text,coalesce(returned.base_quantity,0)::text,coalesce(returned.value,0)::text
        FROM app.inventory_receipt_lines l`+purchaseLineJoins+`
        JOIN app.inventory_movements m ON m.id=l.movement_id
        LEFT JOIN app.inventory_purchase_order_lines ol ON ol.order_id=l.order_id AND ol.line_no=l.order_line_no
        LEFT JOIN (SELECT receipt_line_no,sum(quantity) AS quantity,sum(base_quantity) AS base_quantity,sum(value) AS value
            FROM app.inventory_supplier_return_lines WHERE receipt_id=$1 GROUP BY receipt_line_no) returned
            ON returned.receipt_line_no=l.line_no
        WHERE l.receipt_id=$1 ORDER BY l.line_no`, receiptID, folio)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []ReceiptLine{}
	for rows.Next() {
		var line ReceiptLine
		var ordered *string
		var before, value, cost, stockAfter, returned, returnedBase, returnedValue string
		targets, finish := purchaseLineTargets(&line.PurchaseLine)
		err := rows.Scan(append(targets, &line.OrderLineNo, &ordered, &before, &value, &cost, &line.MovementID, &stockAfter,
			&returned, &returnedBase, &returnedValue)...)
		if err != nil {
			return nil, err
		}
		finish()
		line.quantity = parseDatabaseDecimal(line.Quantity)
		line.base = parseDatabaseDecimal(line.BaseQuantity)
		line.inventoryValue = parseDatabaseDecimal(value)
		line.unitCost = parseDatabaseDecimal(cost)
		line.returned = parseDatabaseDecimal(returned)
		line.returnedBase = parseDatabaseDecimal(returnedBase)
		line.returnedValue = parseDatabaseDecimal(returnedValue)
		if ordered != nil {
			orderedValue := parseDatabaseDecimal(*ordered)
			beforeValue := parseDatabaseDecimal(before)
			orderedText := quantityText(orderedValue)
			beforeText := quantityText(beforeValue)
			pendingText := quantityText(positivePart(orderedValue.Sub(beforeValue).Sub(line.quantity)))
			line.OrderedQuantity, line.ReceivedBeforeQuantity, line.PendingAfterQuantity = &orderedText, &beforeText, &pendingText
		}
		line.StockAfter = quantityText(parseDatabaseDecimal(stockAfter))
		line.ReturnedQuantity = quantityText(line.returned)
		line.ReturnableQuantity = quantityText(positivePart(line.quantity.Sub(line.returned)))
		line.ReturnedBaseQuantity = quantityText(line.returnedBase)
		line.ReturnableBaseQuantity = quantityText(positivePart(line.base.Sub(line.returnedBase)))
		line.CostsVisible = handler.costsVisible()
		if line.CostsVisible {
			line.InventoryValue, line.InventoryValueDisplay = moneyPointers(line.inventoryValue)
			line.UnitCost, line.UnitCostDisplay = costPointers(line.unitCost)
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

// receiptSummaries lee las recepciones que cumplen la condición (las más recientes primero).
func (handler *service) receiptSummaries(where string, limit int, arguments ...any) ([]ReceiptSummary, error) {
	statement := receiptColumns + " WHERE " + where + " ORDER BY r.occurred_at DESC, r.folio DESC LIMIT " + itoa(limit)
	rows, err := handler.tx.Query(handler.ctx, statement, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	summaries := []ReceiptSummary{}
	for rows.Next() {
		receipt, err := handler.scanReceipt(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, receipt.ReceiptSummary)
	}
	return summaries, rows.Err()
}

// listReceipts lista recepciones con búsqueda (folio, referencia del proveedor o proveedor),
// filtros de proveedor, orden, almacén, tipo (directa o con orden) y días de negocio (AAAA-MM-DD en
// la zona del establecimiento), y paginación.
func (handler *service) listReceipts(_ []string) (any, error) {
	query := handler.request.Query
	allowed := []string{"search", "supplier_id", "purchase_order_id", "warehouse_id", "direct", "from", "until", "page", "page_size"}
	if err := allowQuery(query, allowed...); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	if search := strings.TrimSpace(query.Get("search")); search != "" {
		pattern, err := searchPattern(query)
		if err != nil {
			return nil, err
		}
		set.arguments = append(set.arguments, search, pattern)
		count := len(set.arguments)
		set.clauses = append(set.clauses, fmt.Sprintf(`(r.folio::text=$%d OR r.supplier_reference ILIKE $%d
            OR EXISTS(SELECT FROM app.inventory_suppliers sp WHERE sp.id=r.supplier_id AND sp.trade_name ILIKE $%d))`,
			count-1, count, count))
	}
	for key, column := range map[string]string{
		"supplier_id": "r.supplier_id", "purchase_order_id": "r.purchase_order_id", "warehouse_id": "r.warehouse_id",
	} {
		identifier, err := optionalQueryID(query, key, "identificador")
		if err != nil {
			return nil, err
		}
		if identifier != nil {
			set.add(column+"=$%d", *identifier)
		}
	}
	direct, err := optionalBool(query, "direct", "tipo de recepción")
	if err != nil {
		return nil, err
	}
	if direct != nil {
		if *direct {
			set.clauses = append(set.clauses, "r.purchase_order_id IS NULL")
		} else {
			set.clauses = append(set.clauses, "r.purchase_order_id IS NOT NULL")
		}
	}
	from, until, err := handler.dateRange(query)
	if err != nil {
		return nil, err
	}
	if from != nil {
		set.add("r.occurred_at>=$%d", *from)
	}
	if until != nil {
		set.add("r.occurred_at<$%d", *until)
	}
	result := ReceiptPage{Items: []ReceiptSummary{}, Page: page, PageSize: size, CostsVisible: handler.costsVisible()}
	count := "SELECT count(*) FROM app.inventory_receipts r" + set.where()
	err = handler.tx.QueryRow(handler.ctx, count, set.arguments...).Scan(&result.Total)
	if err != nil {
		return nil, err
	}
	statement := receiptColumns + set.where() + " ORDER BY r.occurred_at DESC, r.folio DESC LIMIT " + set.next(size) +
		" OFFSET " + set.next((page-1)*size)
	rows, err := handler.tx.Query(handler.ctx, statement, set.arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		receipt, err := handler.scanReceipt(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, receipt.ReceiptSummary)
	}
	return result, rows.Err()
}
