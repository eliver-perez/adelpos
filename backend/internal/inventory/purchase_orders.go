package inventory

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Órdenes de compra (§17 de la especificación). Estados:
//
//	draft → issued → partially_received → received
//	draft | issued | partially_received → cancelled (con motivo; conserva lo ya recibido)
//	issued | partially_received → closed (cierre administrativo con motivo: lo pendiente ya no se espera)
//
// Crear y editar el borrador exige inventory.purchases.create; emitir, cancelar y cerrar,
// inventory.purchases.authorize. Solo el borrador se edita (los renglones de una orden emitida son de
// solo lectura también en la base). Crear o emitir no toca existencias ni caja: las existencias
// aumentan solo con la recepción física. Las recepciones cambian el estado a parcial o recibida y
// aumentan la revisión, de modo que una pantalla con la orden anterior debe actualizarse antes de
// cancelarla o cerrarla.

const maximumOrderLines = 200

var purchaseOrderStateLabels = map[string]string{
	"draft":              "Borrador",
	"issued":             "Emitida",
	"partially_received": "Parcialmente recibida",
	"received":           "Recibida",
	"cancelled":          "Cancelada",
	"closed":             "Cerrada administrativamente",
}

// PurchaseOrderSummary es una orden en listados e historiales.
type PurchaseOrderSummary struct {
	ID                string             `json:"id"`
	Folio             int64              `json:"folio"`
	Supplier          SupplierReference  `json:"supplier"`
	State             string             `json:"state"`
	StateLabel        string             `json:"state_label"`
	OrderDate         string             `json:"order_date"`
	ExpectedWarehouse WarehouseReference `json:"expected_warehouse"`
	Subtotal          string             `json:"subtotal"`
	Discount          string             `json:"discount"`
	Tax               string             `json:"tax"`
	Total             string             `json:"total"`
	LineCount         int                `json:"line_count"`
	ReceiptCount      int                `json:"receipt_count"`
	CreatedByName     string             `json:"created_by_name"`
	IssuedAt          *time.Time         `json:"issued_at"`
	Revision          int                `json:"revision"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

// PurchaseOrder es la orden completa con renglones (solicitado, recibido y pendiente) y sus
// recepciones. Las banderas de acción dependen solo del estado; los permisos los valida Go.
type PurchaseOrder struct {
	PurchaseOrderSummary
	Notes           string              `json:"notes"`
	UpdatedByName   string              `json:"updated_by_name"`
	IssuedByName    *string             `json:"issued_by_name"`
	CancelledAt     *time.Time          `json:"cancelled_at"`
	CancelledByName *string             `json:"cancelled_by_name"`
	CancelReason    *string             `json:"cancel_reason"`
	ClosedAt        *time.Time          `json:"closed_at"`
	ClosedByName    *string             `json:"closed_by_name"`
	CloseReason     *string             `json:"close_reason"`
	Editable        bool                `json:"editable"`
	Issuable        bool                `json:"issuable"`
	Cancellable     bool                `json:"cancellable"`
	Closable        bool                `json:"closable"`
	Receivable      bool                `json:"receivable"`
	Lines           []PurchaseOrderLine `json:"lines"`
	Receipts        []ReceiptSummary    `json:"receipts"`
}

// PurchaseOrderLine es un renglón de la orden con lo recibido y lo pendiente, en la presentación o
// unidad capturada y en la unidad base. Lo recibido de más (con allow_excess) aparece en excess.
type PurchaseOrderLine struct {
	PurchaseLine
	ReceivedQuantity     string `json:"received_quantity"`
	PendingQuantity      string `json:"pending_quantity"`
	ExcessQuantity       string `json:"excess_quantity"`
	ReceivedBaseQuantity string `json:"received_base_quantity"`
	PendingBaseQuantity  string `json:"pending_base_quantity"`
	FullyReceived        bool   `json:"fully_received"`
	// Datos para recibir: cantidades sin redondear, descuento total del renglón.
	ordered        decimal.Decimal
	orderedBase    decimal.Decimal
	received       decimal.Decimal
	unitPrice      decimal.Decimal
	discount       decimal.Decimal
	presentationID *string
	unitID         *string
}

// PurchaseOrderPage es una página del listado de órdenes (la más reciente primero).
type PurchaseOrderPage struct {
	Items    []PurchaseOrderSummary `json:"items"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

const purchaseOrderColumns = `SELECT o.id,o.folio,s.id,s.trade_name,s.active,o.state,to_char(o.order_date,'YYYY-MM-DD'),w.id,w.name,
        o.subtotal::text,o.discount::text,o.tax::text,o.total::text,o.line_count,
        (SELECT count(*) FROM app.inventory_receipts r WHERE r.purchase_order_id=o.id),creator.display_name,o.issued_at,o.revision,
        o.created_at,o.updated_at,o.notes,updater.display_name,issuer.display_name,o.cancelled_at,canceller.display_name,
        o.cancel_reason,o.closed_at,closer.display_name,o.close_reason
        FROM app.inventory_purchase_orders o
        JOIN app.inventory_suppliers s ON s.id=o.supplier_id
        JOIN app.inventory_warehouses w ON w.id=o.expected_warehouse_id
        JOIN app.users creator ON creator.id=o.created_by
        JOIN app.users updater ON updater.id=o.updated_by
        LEFT JOIN app.users issuer ON issuer.id=o.issued_by
        LEFT JOIN app.users canceller ON canceller.id=o.cancelled_by
        LEFT JOIN app.users closer ON closer.id=o.closed_by`

func scanPurchaseOrder(row interface{ Scan(...any) error }) (*PurchaseOrder, error) {
	var order PurchaseOrder
	var subtotal, discount, tax, total string
	summary := &order.PurchaseOrderSummary
	err := row.Scan(&summary.ID, &summary.Folio, &summary.Supplier.ID, &summary.Supplier.TradeName, &summary.Supplier.Active,
		&summary.State, &summary.OrderDate, &summary.ExpectedWarehouse.ID, &summary.ExpectedWarehouse.Name, &subtotal, &discount,
		&tax, &total, &summary.LineCount, &summary.ReceiptCount, &summary.CreatedByName, &summary.IssuedAt, &summary.Revision,
		&summary.CreatedAt, &summary.UpdatedAt, &order.Notes, &order.UpdatedByName, &order.IssuedByName, &order.CancelledAt,
		&order.CancelledByName, &order.CancelReason, &order.ClosedAt, &order.ClosedByName, &order.CloseReason)
	if err != nil {
		return nil, err
	}
	summary.StateLabel = purchaseOrderStateLabels[summary.State]
	summary.Subtotal = money(parseDatabaseDecimal(subtotal))
	summary.Discount = money(parseDatabaseDecimal(discount))
	summary.Tax = money(parseDatabaseDecimal(tax))
	summary.Total = money(parseDatabaseDecimal(total))
	order.Editable = summary.State == "draft"
	order.Issuable = summary.State == "draft"
	order.Cancellable = summary.State == "draft" || summary.State == "issued" || summary.State == "partially_received"
	order.Closable = summary.State == "issued" || summary.State == "partially_received"
	order.Receivable = order.Closable
	order.Lines = []PurchaseOrderLine{}
	order.Receipts = []ReceiptSummary{}
	return &order, nil
}

// readPurchaseOrderHeader lee la orden sin renglones.
func (handler *service) readPurchaseOrderHeader(id string) (*PurchaseOrder, error) {
	order, err := scanPurchaseOrder(handler.tx.QueryRow(handler.ctx, purchaseOrderColumns+" WHERE o.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("La orden de compra no existe.")
	}
	return order, err
}

// readPurchaseOrder lee la orden con renglones y recepciones.
func (handler *service) readPurchaseOrder(id string) (*PurchaseOrder, error) {
	order, err := handler.readPurchaseOrderHeader(id)
	if err != nil {
		return nil, err
	}
	if order.Lines, err = handler.purchaseOrderLines(order.ID); err != nil {
		return nil, err
	}
	if order.Receipts, err = handler.receiptSummaries("r.purchase_order_id=$1", maximumOrderLines, order.ID); err != nil {
		return nil, err
	}
	return order, nil
}

func (handler *service) readPurchaseOrderRoute(parts []string) (any, error) {
	return handler.readPurchaseOrder(parts[1])
}

// purchaseOrderLines lee los renglones de una orden con lo recibido por todas sus recepciones.
func (handler *service) purchaseOrderLines(orderID string) ([]PurchaseOrderLine, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT `+purchaseLineColumns+`,
            coalesce(received.quantity,0)::text,coalesce(received.base_quantity,0)::text
        FROM app.inventory_purchase_order_lines l`+purchaseLineJoins+`
        LEFT JOIN (SELECT order_line_no,sum(quantity) AS quantity,sum(base_quantity) AS base_quantity
            FROM app.inventory_receipt_lines WHERE order_id=$1 GROUP BY order_line_no) received ON received.order_line_no=l.line_no
        WHERE l.order_id=$1 ORDER BY l.line_no`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []PurchaseOrderLine{}
	for rows.Next() {
		var line PurchaseOrderLine
		var received, receivedBase string
		targets, finish := purchaseLineTargets(&line.PurchaseLine)
		if err := rows.Scan(append(targets, &received, &receivedBase)...); err != nil {
			return nil, err
		}
		finish()
		line.ordered = parseDatabaseDecimal(line.Quantity)
		line.orderedBase = parseDatabaseDecimal(line.BaseQuantity)
		line.received = parseDatabaseDecimal(received)
		line.unitPrice = parseDatabaseDecimal(line.UnitPrice)
		line.discount = parseDatabaseDecimal(line.Discount)
		if line.Presentation != nil {
			line.presentationID = &line.Presentation.ID
		}
		if line.Unit != nil {
			line.unitID = &line.Unit.ID
		}
		receivedBaseValue := parseDatabaseDecimal(receivedBase)
		line.ReceivedQuantity = quantityText(line.received)
		line.ReceivedBaseQuantity = quantityText(receivedBaseValue)
		line.PendingQuantity = quantityText(positivePart(line.ordered.Sub(line.received)))
		line.ExcessQuantity = quantityText(positivePart(line.received.Sub(line.ordered)))
		line.PendingBaseQuantity = quantityText(positivePart(line.orderedBase.Sub(receivedBaseValue)))
		line.FullyReceived = line.received.GreaterThanOrEqual(line.ordered)
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func positivePart(value decimal.Decimal) decimal.Decimal {
	if value.IsNegative() {
		return zero
	}
	return value
}

// purchaseOrderSummaries lee las órdenes que cumplen la condición (las más recientes primero).
func (handler *service) purchaseOrderSummaries(where string, limit int, arguments ...any) ([]PurchaseOrderSummary, error) {
	statement := purchaseOrderColumns + " WHERE " + where + " ORDER BY o.order_date DESC, o.folio DESC LIMIT " + itoa(limit)
	rows, err := handler.tx.Query(handler.ctx, statement, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	summaries := []PurchaseOrderSummary{}
	for rows.Next() {
		order, err := scanPurchaseOrder(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, order.PurchaseOrderSummary)
	}
	return summaries, rows.Err()
}

// listPurchaseOrders lista órdenes con búsqueda (folio o proveedor), filtros de estado, proveedor
// y fecha de la orden (AAAA-MM-DD), y paginación.
func (handler *service) listPurchaseOrders(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "search", "state", "supplier_id", "from", "until", "page", "page_size"); err != nil {
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
		// El folio se compara exacto y el proveedor por nombre comercial.
		set.arguments = append(set.arguments, search, pattern)
		count := len(set.arguments)
		set.clauses = append(set.clauses, fmt.Sprintf(`(o.folio::text=$%d OR EXISTS(SELECT FROM app.inventory_suppliers sp
            WHERE sp.id=o.supplier_id AND sp.trade_name ILIKE $%d))`, count-1, count))
	}
	if state := query.Get("state"); state != "" {
		if state == "open" {
			set.clauses = append(set.clauses, "o.state IN ('issued','partially_received')")
		} else {
			if _, known := purchaseOrderStateLabels[state]; !known {
				return nil, invalid("El filtro de estado de la orden no es válido.")
			}
			set.add("o.state=$%d", state)
		}
	}
	supplierID, err := optionalQueryID(query, "supplier_id", "proveedor")
	if err != nil {
		return nil, err
	}
	if supplierID != nil {
		set.add("o.supplier_id=$%d", *supplierID)
	}
	from, until, err := dateFilter(query)
	if err != nil {
		return nil, err
	}
	if from != nil {
		set.add("o.order_date>=$%d::date", *from)
	}
	if until != nil {
		set.add("o.order_date<=$%d::date", *until)
	}
	result := PurchaseOrderPage{Items: []PurchaseOrderSummary{}, Page: page, PageSize: size}
	err = handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_purchase_orders o"+set.where(), set.arguments...).
		Scan(&result.Total)
	if err != nil {
		return nil, err
	}
	statement := purchaseOrderColumns + set.where() + " ORDER BY o.order_date DESC, o.folio DESC LIMIT " + set.next(size) +
		" OFFSET " + set.next((page-1)*size)
	rows, err := handler.tx.Query(handler.ctx, statement, set.arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		order, err := scanPurchaseOrder(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, order.PurchaseOrderSummary)
	}
	return result, rows.Err()
}

// dateFilter lee from y until como fechas AAAA-MM-DD (sin zona: son fechas de documento).
func dateFilter(query interface{ Get(string) string }) (*string, *string, error) {
	var bounds [2]*string
	for index, key := range []string{"from", "until"} {
		text := query.Get(key)
		if text == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", text); err != nil {
			return nil, nil, invalid("Las fechas deben tener el formato AAAA-MM-DD.")
		}
		value := text
		bounds[index] = &value
	}
	if bounds[0] != nil && bounds[1] != nil && *bounds[0] > *bounds[1] {
		return nil, nil, invalid("La fecha inicial no puede ser posterior a la final.")
	}
	return bounds[0], bounds[1], nil
}

type purchaseOrderInput struct {
	OperationID string `json:"operation_id"`
	Revision    int    `json:"revision"`
	SupplierID  string `json:"supplier_id"`
	// OrderDate AAAA-MM-DD; vacía es hoy en la zona horaria del establecimiento. No puede ser futura.
	OrderDate *string `json:"order_date"`
	// ExpectedWarehouseID vacío es el almacén predeterminado.
	ExpectedWarehouseID *string             `json:"expected_warehouse_id"`
	Notes               string              `json:"notes"`
	Lines               []purchaseLineInput `json:"lines"`
}

// preparedOrder es una orden validada lista para guardar.
type preparedOrder struct {
	supplier  *Supplier
	date      string
	warehouse *Warehouse
	notes     string
	lines     []preparedPurchaseLine
	totals    PurchaseAmounts
}

// prepareOrder valida proveedor, fecha, almacén, observaciones y renglones. Un borrador puede no
// tener renglones todavía; emitirlo exige al menos uno.
func (handler *service) prepareOrder(input purchaseOrderInput) (preparedOrder, error) {
	if len(input.Lines) > maximumOrderLines {
		return preparedOrder{}, invalid("La orden de compra admite hasta 200 renglones.")
	}
	notes, err := purchaseText(input.Notes, "Las observaciones de la orden", 2000, false)
	if err != nil {
		return preparedOrder{}, err
	}
	supplier, err := handler.supplierForPurchase(input.SupplierID)
	if err != nil {
		return preparedOrder{}, err
	}
	date, err := handler.orderDate(input.OrderDate)
	if err != nil {
		return preparedOrder{}, err
	}
	warehouse, err := handler.purchaseWarehouse(input.ExpectedWarehouseID)
	if err != nil {
		return preparedOrder{}, err
	}
	lines, totals, err := handler.preparePurchaseLines(input.Lines)
	if err != nil {
		return preparedOrder{}, err
	}
	return preparedOrder{supplier: supplier, date: date, warehouse: warehouse, notes: notes, lines: lines, totals: totals}, nil
}

// orderDate valida la fecha de la orden (AAAA-MM-DD, no futura en la zona del establecimiento).
func (handler *service) orderDate(requested *string) (string, error) {
	location, err := handler.establishmentLocation()
	if err != nil {
		return "", err
	}
	var now time.Time
	if err := handler.tx.QueryRow(handler.ctx, "SELECT now()").Scan(&now); err != nil {
		return "", err
	}
	today := now.In(location).Format("2006-01-02")
	if requested == nil || strings.TrimSpace(*requested) == "" {
		return today, nil
	}
	date := strings.TrimSpace(*requested)
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return "", invalid("La fecha de la orden debe tener el formato AAAA-MM-DD.")
	}
	if date > today {
		return "", invalid("La fecha de la orden no puede ser futura.")
	}
	return date, nil
}

// purchaseWarehouse devuelve el almacén indicado (activo) o el predeterminado.
func (handler *service) purchaseWarehouse(requested *string) (*Warehouse, error) {
	warehouseID, err := validateOptionalID(requested)
	if err != nil {
		return nil, invalid("El almacén no es válido.")
	}
	var warehouse *Warehouse
	if warehouseID == nil {
		warehouse, err = handler.defaultWarehouse()
	} else {
		warehouse, err = handler.readWarehouse(*warehouseID)
	}
	if err != nil {
		return nil, err
	}
	if !warehouse.Active {
		return nil, conflict("WAREHOUSE_INACTIVE", "El almacén «"+warehouse.Name+"» está desactivado.")
	}
	return warehouse, nil
}

// insertOrderLines guarda los renglones de un borrador.
func (handler *service) insertOrderLines(orderID string, lines []preparedPurchaseLine) error {
	for index, line := range lines {
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_purchase_order_lines(order_id,line_no,item_id,presentation_id,
                unit_id,quantity,base_quantity,unit_price,discount,tax_rate,subtotal,tax,total)
            VALUES ($1,$2,$3,$4,$5,$6::numeric,$7::numeric,$8::numeric,$9::numeric,$10,$11::numeric,$12::numeric,$13::numeric)`,
			orderID, index+1, line.capture.item.ID, line.capture.presentationID, unitWhenNoPresentation(line.capture),
			line.capture.quantity.String(), line.capture.base.String(), line.unitPrice.String(), money(line.amounts.Discount),
			line.taxRate, money(line.amounts.Subtotal), money(line.amounts.Tax), money(line.amounts.Total))
		if err != nil {
			return err
		}
	}
	return nil
}

// unitWhenNoPresentation devuelve la unidad de captura solo si no se capturó en una presentación
// (exactamente una de las dos, CHECK de la base).
func unitWhenNoPresentation(capture purchaseCapture) *string {
	if capture.presentationID != nil {
		return nil
	}
	return capture.unitID
}

// createPurchaseOrder registra un borrador de orden de compra. No toca existencias ni caja.
func (handler *service) createPurchaseOrder(_ []string) (any, error) {
	var input purchaseOrderInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	prepared, err := handler.prepareOrder(input)
	if err != nil {
		return nil, err
	}
	id := newID()
	actor := handler.request.Actor.UserID
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_purchase_orders(id,supplier_id,order_date,expected_warehouse_id,notes,
            subtotal,discount,tax,total,line_count,created_by,updated_by,operation_id)
        VALUES ($1,$2,$3::date,$4,$5,$6::numeric,$7::numeric,$8::numeric,$9::numeric,$10,$11,$11,$12)`,
		id, prepared.supplier.ID, prepared.date, prepared.warehouse.ID, prepared.notes, money(prepared.totals.Subtotal),
		money(prepared.totals.Discount), money(prepared.totals.Tax), money(prepared.totals.Total), len(prepared.lines), actor,
		handler.operationReference())
	if err != nil {
		return nil, err
	}
	if err := handler.insertOrderLines(id, prepared.lines); err != nil {
		return nil, err
	}
	saved, err := handler.readPurchaseOrder(id)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.purchases.changed", &id); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.purchase_order_created", "inventory_purchase_order", &id, nil, saved)
}

// updatePurchaseOrder reemplaza los datos y renglones de un borrador con revisión optimista.
func (handler *service) updatePurchaseOrder(parts []string) (any, error) {
	var input purchaseOrderInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	previous, err := handler.readPurchaseOrder(parts[1])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("La orden de compra")
	}
	if previous.State != "draft" {
		return nil, conflict("ORDER_NOT_EDITABLE", "Solo se edita una orden en borrador; esta orden está «"+previous.StateLabel+"».")
	}
	prepared, err := handler.prepareOrder(input)
	if err != nil {
		return nil, err
	}
	if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.inventory_purchase_order_lines WHERE order_id=$1", previous.ID); err != nil {
		return nil, err
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_purchase_orders SET supplier_id=$2,order_date=$3::date,
            expected_warehouse_id=$4,notes=$5,subtotal=$6::numeric,discount=$7::numeric,tax=$8::numeric,total=$9::numeric,line_count=$10,
            updated_by=$11,revision=revision+1,updated_at=now()
        WHERE id=$1`,
		previous.ID, prepared.supplier.ID, prepared.date, prepared.warehouse.ID, prepared.notes, money(prepared.totals.Subtotal),
		money(prepared.totals.Discount), money(prepared.totals.Tax), money(prepared.totals.Total), len(prepared.lines),
		handler.request.Actor.UserID)
	if err != nil {
		return nil, err
	}
	if err := handler.insertOrderLines(previous.ID, prepared.lines); err != nil {
		return nil, err
	}
	saved, err := handler.readPurchaseOrder(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.purchases.changed", &previous.ID); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.purchase_order_updated", "inventory_purchase_order", &previous.ID,
		previous, saved)
}

type orderTransitionInput struct {
	OperationID string `json:"operation_id"`
	Revision    int    `json:"revision"`
	Reason      string `json:"reason"`
}

// orderTransition valida revisión y estado de origen, aplica el cambio y lo audita.
func (handler *service) orderTransition(orderID string, input orderTransitionInput, allowed []string, refusal string,
	action, update string, arguments ...any) (any, error) {
	previous, err := handler.readPurchaseOrder(orderID)
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("La orden de compra")
	}
	permitted := false
	for _, state := range allowed {
		permitted = permitted || previous.State == state
	}
	if !permitted {
		return nil, conflict(refusal, "La orden de compra está «"+previous.StateLabel+"»: no admite esta acción.")
	}
	values := append([]any{previous.ID, handler.request.Actor.UserID}, arguments...)
	if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.inventory_purchase_orders SET `+update+`,updated_by=$2,
            revision=revision+1,updated_at=now() WHERE id=$1`, values...); err != nil {
		return nil, err
	}
	saved, err := handler.readPurchaseOrder(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.purchases.changed", &previous.ID); err != nil {
		return nil, err
	}
	before := map[string]any{"state": previous.State, "revision": previous.Revision}
	after := map[string]any{"state": saved.State, "revision": saved.Revision, "reason": input.Reason}
	return saved, handler.audit(handler.ctx, handler.tx, action, "inventory_purchase_order", &previous.ID, before, after)
}

// issuePurchaseOrder emite (autoriza) un borrador con al menos un renglón, de un proveedor activo y
// con insumos activos. No toca existencias ni caja.
func (handler *service) issuePurchaseOrder(parts []string) (any, error) {
	var input orderTransitionInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Reason) != "" {
		return nil, invalid("Emitir una orden no lleva motivo.")
	}
	order, err := handler.readPurchaseOrder(parts[1])
	if err != nil {
		return nil, err
	}
	if order.State == "draft" {
		if len(order.Lines) == 0 {
			return nil, conflict("ORDER_EMPTY", "Agrega al menos un renglón antes de emitir la orden.")
		}
		if !order.Supplier.Active {
			return nil, conflict("SUPPLIER_INACTIVE", "El proveedor «"+order.Supplier.TradeName+"» está desactivado.")
		}
		for _, line := range order.Lines {
			if !line.Item.Active {
				return nil, conflict("ITEM_INACTIVE", "El insumo «"+line.Item.Name+"» está desactivado.")
			}
		}
	}
	return handler.orderTransition(order.ID, input, []string{"draft"}, "ORDER_NOT_DRAFT", "inventory.purchase_order_issued",
		"state='issued',issued_by=$2,issued_at=now()")
}

// cancelPurchaseOrder cancela una orden en borrador, emitida o parcialmente recibida, con motivo.
// Las recepciones ya registradas se conservan: solo deja de esperarse lo pendiente.
func (handler *service) cancelPurchaseOrder(parts []string) (any, error) {
	var input orderTransitionInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	reason, err := purchaseReason(input.Reason, "El motivo de la cancelación")
	if err != nil {
		return nil, err
	}
	input.Reason = reason
	return handler.orderTransition(parts[1], input, []string{"draft", "issued", "partially_received"}, "ORDER_NOT_CANCELLABLE",
		"inventory.purchase_order_cancelled", "state='cancelled',cancelled_by=$2,cancelled_at=now(),cancel_reason=$3", reason)
}

// closePurchaseOrder cierra administrativamente una orden emitida o parcialmente recibida, con
// motivo: lo recibido se conserva y lo pendiente ya no se espera.
func (handler *service) closePurchaseOrder(parts []string) (any, error) {
	var input orderTransitionInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	reason, err := purchaseReason(input.Reason, "El motivo del cierre")
	if err != nil {
		return nil, err
	}
	input.Reason = reason
	return handler.orderTransition(parts[1], input, []string{"issued", "partially_received"}, "ORDER_NOT_CLOSABLE",
		"inventory.purchase_order_closed", "state='closed',closed_by=$2,closed_at=now(),close_reason=$3", reason)
}
