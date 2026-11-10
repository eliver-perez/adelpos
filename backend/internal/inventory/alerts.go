package inventory

import (
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"local/ad-el-pos/server/internal/plural"
)

// Alertas de inventario (§24 de la especificación). Se calculan en Go al consultarlas, a partir de
// los datos vigentes, y nunca bloquean ventas ni otras operaciones. Cada alerta tiene tipo,
// gravedad, texto en español sin identificadores crudos y una referencia navegable (tipo de destino,
// identificador y nombre). Visibilidad (además de inventory.read):
//   - las cantidades de existencia solo aparecen con inventory.stock.read;
//   - «costo pendiente» solo con inventory.costs.read;
//   - las alertas de compras (orden parcialmente recibida, gasto vinculado corregido) solo con algún
//     permiso de compras;
//   - «producto sin receta» respeta warn_products_without_recipe de la configuración.

// Tipos de alerta.
const (
	alertNegativeStock        = "negative_stock"
	alertBelowMinimum         = "below_minimum"
	alertReorderPoint         = "reorder_point"
	alertItemWithoutSupplier  = "item_without_supplier"
	alertProductWithoutRecipe = "product_without_recipe"
	alertCostPending          = "cost_pending"
	alertCountDifference      = "count_difference_pending"
	alertCountDue             = "count_due"
	alertPartialOrder         = "purchase_order_partial"
	alertUnclassified         = "cancellations_unclassified"
	alertConsumptionFailed    = "consumption_failed"
	alertCashLinkCorrected    = "cash_link_expense_corrected"
)

// alertTypeInfo es la etiqueta y la gravedad de un tipo de alerta.
type alertTypeInfo struct {
	code     string
	label    string
	severity string
}

// alertTypes enumera los tipos en el orden en que se muestran dentro de cada gravedad, con su
// etiqueta y su gravedad.
var alertTypes = []alertTypeInfo{
	{alertNegativeStock, "Existencia negativa", "critical"},
	{alertConsumptionFailed, "Consumo sin calcular", "critical"},
	{alertBelowMinimum, "Bajo mínimo", "warning"},
	{alertCostPending, "Costo pendiente", "warning"},
	{alertCountDifference, "Diferencia de conteo pendiente", "warning"},
	{alertUnclassified, "Cancelaciones sin clasificar", "warning"},
	{alertCashLinkCorrected, "Gasto vinculado corregido", "warning"},
	{alertReorderPoint, "Reabastecimiento", "info"},
	{alertCountDue, "Conteo pendiente", "info"},
	{alertPartialOrder, "Orden parcialmente recibida", "info"},
	{alertItemWithoutSupplier, "Insumo sin proveedor", "info"},
	{alertProductWithoutRecipe, "Producto sin receta", "info"},
}

var alertSeverityLabels = map[string]string{"critical": "Crítica", "warning": "Advertencia", "info": "Aviso"}

var alertSeverityRank = map[string]int{"critical": 0, "warning": 1, "info": 2}

// Alert es una alerta de inventario. Quantity y Threshold (en la unidad base del insumo) solo con
// inventory.stock.read. Count es el número de registros que resume (1 en las alertas de un solo
// registro). OccurredAt es el momento relevante cuando lo hay (último conteo aplicado, vencimiento
// más próximo de una cancelación, solicitud fallida más antigua).
type Alert struct {
	Type          string              `json:"type"`
	TypeLabel     string              `json:"type_label"`
	Severity      string              `json:"severity"`
	SeverityLabel string              `json:"severity_label"`
	Message       string              `json:"message"`
	Reference     AlertReference      `json:"reference"`
	Item          *ItemReference      `json:"item"`
	Warehouse     *WarehouseReference `json:"warehouse"`
	Quantity      *string             `json:"quantity"`
	Threshold     *string             `json:"threshold"`
	Count         int                 `json:"count"`
	OccurredAt    *time.Time          `json:"occurred_at"`
	order         int
}

// AlertReference es el destino navegable de una alerta: kind es stock (insumo en un almacén o en
// total), item, product, count, warehouse, purchase_order, receipt, cancellations o
// consumption_requests; id es null en las alertas que resumen una lista.
type AlertReference struct {
	Kind  string  `json:"kind"`
	ID    *string `json:"id"`
	Label string  `json:"label"`
}

// AlertCounts cuenta las alertas visibles por gravedad y por tipo.
type AlertCounts struct {
	Total    int            `json:"total"`
	Critical int            `json:"critical"`
	Warning  int            `json:"warning"`
	Info     int            `json:"info"`
	ByType   map[string]int `json:"by_type"`
}

// AlertPage es una página de alertas con los conteos de todas las visibles.
type AlertPage struct {
	Items             []Alert     `json:"items"`
	Total             int         `json:"total"`
	Page              int         `json:"page"`
	PageSize          int         `json:"page_size"`
	Counts            AlertCounts `json:"counts"`
	CostsVisible      bool        `json:"costs_visible"`
	StockVisible      bool        `json:"stock_visible"`
	PurchasingVisible bool        `json:"purchasing_visible"`
	GeneratedAt       time.Time   `json:"generated_at"`
}

// alertCollector arma las alertas con su visibilidad.
type alertCollector struct {
	handler    *service
	stock      bool
	costs      bool
	purchasing bool
	alerts     []Alert
}

func (handler *service) purchasingVisible() bool {
	return slices.ContainsFunc(purchasingPermissions, handler.request.Actor.Can)
}

// add agrega una alerta con la etiqueta, la gravedad y el orden de su tipo.
func (collector *alertCollector) add(alert Alert) {
	for index, entry := range alertTypes {
		if entry.code == alert.Type {
			alert.TypeLabel, alert.Severity, alert.order = entry.label, entry.severity, index
		}
	}
	alert.SeverityLabel = alertSeverityLabels[alert.Severity]
	if alert.Count == 0 {
		alert.Count = 1
	}
	collector.alerts = append(collector.alerts, alert)
}

// collectAlerts calcula todas las alertas visibles para la sesión, ordenadas por gravedad, tipo y
// nombre.
func (handler *service) collectAlerts() ([]Alert, error) {
	collector := &alertCollector{handler: handler, stock: handler.request.Actor.Can(permissionStockRead),
		costs: handler.costsVisible(), purchasing: handler.purchasingVisible()}
	steps := []func() error{
		collector.negativeStock, collector.stockLevels, collector.itemsWithoutSupplier, collector.productsWithoutRecipe,
		collector.pendingCosts, collector.countDifferences, collector.countsDue, collector.partialOrders,
		collector.unclassifiedCancellations, collector.failedConsumption, collector.correctedCashLinks,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	slices.SortStableFunc(collector.alerts, func(left, right Alert) int {
		if rank := alertSeverityRank[left.Severity] - alertSeverityRank[right.Severity]; rank != 0 {
			return rank
		}
		if left.order != right.order {
			return left.order - right.order
		}
		return strings.Compare(strings.ToLower(left.Reference.Label), strings.ToLower(right.Reference.Label))
	})
	return collector.alerts, nil
}

// countAlerts cuenta las alertas por gravedad y tipo.
func countAlerts(alerts []Alert) AlertCounts {
	counts := AlertCounts{ByType: map[string]int{}}
	for _, entry := range alertTypes {
		counts.ByType[entry.code] = 0
	}
	for _, alert := range alerts {
		counts.Total++
		counts.ByType[alert.Type]++
		switch alert.Severity {
		case "critical":
			counts.Critical++
		case "warning":
			counts.Warning++
		default:
			counts.Info++
		}
	}
	return counts
}

// listAlerts devuelve las alertas visibles con filtros de tipo y gravedad y paginación; los conteos
// son de todas las alertas visibles (sin filtros).
func (handler *service) listAlerts(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "type", "severity", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	kind, severity := query.Get("type"), query.Get("severity")
	if kind != "" && !slices.ContainsFunc(alertTypes, func(entry alertTypeInfo) bool {
		return entry.code == kind
	}) {
		return nil, invalid("El filtro de tipo de alerta no es válido.")
	}
	if _, known := alertSeverityLabels[severity]; severity != "" && !known {
		return nil, invalid("El filtro de gravedad de la alerta no es válido.")
	}
	alerts, err := handler.collectAlerts()
	if err != nil {
		return nil, err
	}
	now, err := handler.transactionTime()
	if err != nil {
		return nil, err
	}
	result := AlertPage{Items: []Alert{}, Page: page, PageSize: size, Counts: countAlerts(alerts), CostsVisible: handler.costsVisible(),
		StockVisible: handler.request.Actor.Can(permissionStockRead), PurchasingVisible: handler.purchasingVisible(), GeneratedAt: now}
	filtered := []Alert{}
	for _, alert := range alerts {
		if (kind == "" || alert.Type == kind) && (severity == "" || alert.Severity == severity) {
			filtered = append(filtered, alert)
		}
	}
	result.Total = len(filtered)
	start := min((page-1)*size, len(filtered))
	end := min(start+size, len(filtered))
	result.Items = append(result.Items, filtered[start:end]...)
	return result, nil
}

// quantityWithUnit escribe una cantidad con la abreviatura de la unidad base («-3 pza»).
func quantityWithUnit(value decimal.Decimal, unit UnitReference) string {
	return quantityText(value) + " " + unit.Code
}

// itemColumns de las alertas: insumo con su unidad base (alias i y u).
const alertItemColumns = `i.id,i.code,i.name,i.kind,i.active,u.id,u.code,u.name,u.dimension`

func alertItemTargets(item *ItemReference) []any {
	return []any{&item.ID, &item.Code, &item.Name, &item.Kind, &item.Active, &item.Unit.ID, &item.Unit.Code, &item.Unit.Name,
		&item.Unit.Dimension}
}

// negativeStock: existencia negativa por insumo activo y almacén.
func (collector *alertCollector) negativeStock() error {
	rows, err := collector.handler.tx.Query(collector.handler.ctx, `SELECT `+alertItemColumns+`,w.id,w.name,s.quantity::text
        FROM app.inventory_stock s JOIN app.inventory_items i ON i.id=s.item_id JOIN app.inventory_units u ON u.id=i.base_unit_id
        JOIN app.inventory_warehouses w ON w.id=s.warehouse_id
        WHERE i.active AND s.quantity<0`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item ItemReference
		var warehouse WarehouseReference
		var quantity string
		if err := rows.Scan(append(alertItemTargets(&item), &warehouse.ID, &warehouse.Name, &quantity)...); err != nil {
			return err
		}
		alert := Alert{Type: alertNegativeStock, Item: &item, Warehouse: &warehouse,
			Reference: AlertReference{Kind: "stock", ID: &item.ID, Label: item.Name},
			Message:   "Existencia negativa de «" + item.Name + "» en «" + warehouse.Name + "»"}
		if collector.stock {
			value := parseDatabaseDecimal(quantity)
			alert.Quantity = optionalQuantityText(&value)
			alert.Message += ": " + quantityWithUnit(value, item.Unit)
		}
		alert.Message += ". Registra la entrada o el ajuste que falta; la venta no se bloquea."
		collector.add(alert)
	}
	return rows.Err()
}

// stockLevels: bajo mínimo y reabastecimiento por almacén (niveles del almacén contra su
// existencia) y en total (niveles del insumo contra la existencia total), solo de insumos activos.
func (collector *alertCollector) stockLevels() error {
	rows, err := collector.handler.tx.Query(collector.handler.ctx, `SELECT `+alertItemColumns+`,w.id,w.name,coalesce(s.quantity,0)::text,
            l.min_stock::text,l.reorder_point::text
        FROM app.inventory_stock_levels l JOIN app.inventory_items i ON i.id=l.item_id
        JOIN app.inventory_units u ON u.id=i.base_unit_id JOIN app.inventory_warehouses w ON w.id=l.warehouse_id
        LEFT JOIN app.inventory_stock s ON s.item_id=l.item_id AND s.warehouse_id=l.warehouse_id
        WHERE i.active AND w.active
        UNION ALL
        SELECT `+alertItemColumns+`,NULL,NULL,coalesce(v.quantity,0)::text,i.min_stock::text,i.reorder_point::text
        FROM app.inventory_items i JOIN app.inventory_units u ON u.id=i.base_unit_id
        LEFT JOIN app.inventory_valuations v ON v.item_id=i.id
        WHERE i.active AND (i.min_stock IS NOT NULL OR i.reorder_point IS NOT NULL)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item ItemReference
		var warehouseID, warehouseName, minimum, reorder *string
		var quantity string
		if err := rows.Scan(append(alertItemTargets(&item), &warehouseID, &warehouseName, &quantity, &minimum, &reorder)...); err != nil {
			return err
		}
		stock := parseDatabaseDecimal(quantity)
		var warehouse *WarehouseReference
		place := " en total"
		if warehouseID != nil {
			warehouse = &WarehouseReference{ID: *warehouseID, Name: *warehouseName}
			place = " en «" + warehouse.Name + "»"
		}
		for _, level := range []struct {
			kind      string
			threshold *string
			reached   func(threshold decimal.Decimal) bool
			text      string
			action    string
		}{
			{alertBelowMinimum, minimum, stock.LessThan, "«" + item.Name + "» está bajo el mínimo" + place,
				"repón la existencia."},
			{alertReorderPoint, reorder, stock.LessThanOrEqual, "Reabastecer «" + item.Name + "»" + place,
				"llegó al punto de reabastecimiento."},
		} {
			if level.threshold == nil {
				continue
			}
			threshold := parseDatabaseDecimal(*level.threshold)
			if !level.reached(threshold) {
				continue
			}
			copied := item
			alert := Alert{Type: level.kind, Item: &copied, Warehouse: warehouse,
				Reference: AlertReference{Kind: "stock", ID: &copied.ID, Label: copied.Name}, Message: level.text}
			if collector.stock {
				alert.Quantity = optionalQuantityText(&stock)
				alert.Threshold = optionalQuantityText(&threshold)
				alert.Message += ": " + quantityWithUnit(stock, item.Unit) + " de " + quantityWithUnit(threshold, item.Unit)
			}
			alert.Message += "; " + level.action
			collector.add(alert)
		}
	}
	return rows.Err()
}

// itemsWithoutSupplier: insumos comprados activos sin ningún proveedor activo que los surta.
func (collector *alertCollector) itemsWithoutSupplier() error {
	rows, err := collector.handler.tx.Query(collector.handler.ctx, `SELECT `+alertItemColumns+`
        FROM app.inventory_items i JOIN app.inventory_units u ON u.id=i.base_unit_id
        WHERE i.active AND i.kind='purchased' AND NOT EXISTS(SELECT FROM app.inventory_supplier_items si
            JOIN app.inventory_suppliers sp ON sp.id=si.supplier_id WHERE si.item_id=i.id AND sp.active)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item ItemReference
		if err := rows.Scan(alertItemTargets(&item)...); err != nil {
			return err
		}
		collector.add(Alert{Type: alertItemWithoutSupplier, Item: &item,
			Reference: AlertReference{Kind: "item", ID: &item.ID, Label: item.Name},
			Message:   "«" + item.Name + "» no tiene un proveedor activo que lo surta."})
	}
	return rows.Err()
}

// productsWithoutRecipe: productos activos y vendibles (no archivados, de una categoría activa; los
// combos consumen por sus componentes) sin receta activa, si la configuración lo pide.
func (collector *alertCollector) productsWithoutRecipe() error {
	settings, err := collector.handler.settings()
	if err != nil {
		return err
	}
	if !settings.WarnProductsWithoutRecipe {
		return nil
	}
	rows, err := collector.handler.tx.Query(collector.handler.ctx, `SELECT listed.product_id,listed.name,listed.configured
        FROM (`+targetsQuery+`) listed(kind,product_id,variant_id,complement_id,name,variant_name,category_name,product_kind,active,
            recipe_id,recipe_name,recipe_active,inherited_recipe_id,status,route_mode,ingredients_total,ingredients_with_portion,
            linked_product_name,linked_ingredient_name,configured)
        JOIN app.catalog_products p ON p.id=listed.product_id JOIN app.catalog_categories c ON c.id=p.category_id
        WHERE listed.kind='product' AND listed.product_kind='product' AND listed.active AND listed.status<>'with_recipe'
            AND c.active AND NOT p.archived`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		var configured bool
		if err := rows.Scan(&id, &name, &configured); err != nil {
			return err
		}
		message := "El producto «" + name + "» no tiene receta: sus ventas no descuentan insumos."
		if configured {
			message = "El producto «" + name + "» no tiene receta: sus ventas solo descuentan las porciones de sus ingredientes."
		}
		productID := id
		collector.add(Alert{Type: alertProductWithoutRecipe, Reference: AlertReference{Kind: "product", ID: &productID, Label: name},
			Message: message})
	}
	return rows.Err()
}

// pendingCosts: insumos con salidas valoradas en cero por falta de costo (solo con costos).
func (collector *alertCollector) pendingCosts() error {
	if !collector.costs {
		return nil
	}
	rows, err := collector.handler.tx.Query(collector.handler.ctx, `SELECT `+alertItemColumns+`,v.pending_cost_quantity::text
        FROM app.inventory_valuations v JOIN app.inventory_items i ON i.id=v.item_id JOIN app.inventory_units u ON u.id=i.base_unit_id
        WHERE v.pending_cost_quantity>0`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item ItemReference
		var pending string
		if err := rows.Scan(append(alertItemTargets(&item), &pending)...); err != nil {
			return err
		}
		alert := Alert{Type: alertCostPending, Item: &item, Reference: AlertReference{Kind: "item", ID: &item.ID, Label: item.Name},
			Message: "«" + item.Name + "» tiene salidas con costo pendiente: se valorarán con la próxima recepción."}
		if collector.stock {
			value := parseDatabaseDecimal(pending)
			alert.Quantity = optionalQuantityText(&value)
		}
		collector.add(alert)
	}
	return rows.Err()
}

// countDifferences: conteos enviados o aprobados sin aplicar, con sus diferencias calculadas ahora
// (contado − teórico a la hora de cada renglón).
func (collector *alertCollector) countDifferences() error {
	rows, err := collector.handler.tx.Query(collector.handler.ctx, `SELECT c.id,c.folio,c.state,w.id,w.name,
            (SELECT count(*) FROM app.inventory_count_lines l WHERE l.count_id=c.id AND l.counted_quantity IS NOT NULL
                AND l.counted_quantity <> (SELECT coalesce(sum(m.quantity),0) FROM app.inventory_movements m
                    WHERE m.item_id=l.item_id AND m.warehouse_id=c.warehouse_id AND m.occurred_at<=l.captured_at))::int,
            coalesce(c.approved_at,c.submitted_at)
        FROM app.inventory_counts c JOIN app.inventory_warehouses w ON w.id=c.warehouse_id
        WHERE c.state IN ('submitted','approved')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, state string
		var folio int64
		var warehouse WarehouseReference
		var differences int
		var since *time.Time
		if err := rows.Scan(&id, &folio, &state, &warehouse.ID, &warehouse.Name, &differences, &since); err != nil {
			return err
		}
		label := "Conteo " + itoa(int(folio))
		pending := "pendiente de aprobar"
		if state == "approved" {
			pending = "aprobado y pendiente de aplicar"
		}
		countID := id
		copied := warehouse
		collector.add(Alert{Type: alertCountDifference, Warehouse: &copied,
			Reference: AlertReference{Kind: "count", ID: &countID, Label: label}, Count: max(differences, 1), OccurredAt: since,
			Message: "El conteo " + itoa(int(folio)) + " de «" + warehouse.Name + "» está " + pending + " con " +
				plural.Count(differences, "diferencia", "diferencias") + "."})
	}
	return rows.Err()
}

// countsDue: almacenes activos con existencias registradas cuyo último conteo aplicado venció según
// la frecuencia de conteo de la configuración (días operativos con la hora de corte).
func (collector *alertCollector) countsDue() error {
	settings, err := collector.handler.settings()
	if err != nil {
		return err
	}
	if settings.CountFrequency == "none" {
		return nil
	}
	now, err := collector.handler.transactionTime()
	if err != nil {
		return err
	}
	today, err := operatingDayFor(collector.handler.ctx, collector.handler.tx, now)
	if err != nil {
		return err
	}
	rows, err := collector.handler.tx.Query(collector.handler.ctx, `SELECT w.id,w.name,
            (SELECT max(c.reference_at) FROM app.inventory_counts c WHERE c.warehouse_id=w.id AND c.state='applied')
        FROM app.inventory_warehouses w
        WHERE w.active AND EXISTS(SELECT FROM app.inventory_stock s WHERE s.warehouse_id=w.id)`)
	if err != nil {
		return err
	}
	type warehouseCount struct {
		warehouse WarehouseReference
		last      *time.Time
	}
	entries := []warehouseCount{}
	for rows.Next() {
		var entry warehouseCount
		if err := rows.Scan(&entry.warehouse.ID, &entry.warehouse.Name, &entry.last); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, entry)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	frequency := strings.ToLower(settings.CountFrequencyLabel)
	for _, entry := range entries {
		var lastDay *time.Time
		if entry.last != nil {
			day, err := operatingDayFor(collector.handler.ctx, collector.handler.tx, *entry.last)
			if err != nil {
				return err
			}
			lastDay = &day.Date
		}
		if !countDue(settings.CountFrequency, settings.CountIntervalDays, lastDay, today.Date) {
			continue
		}
		message := "Conteo pendiente en «" + entry.warehouse.Name + "»: aún no se ha aplicado ningún conteo (frecuencia " + frequency + ")."
		if entry.last != nil {
			message = "Conteo pendiente en «" + entry.warehouse.Name + "»: venció la frecuencia de conteo (" + frequency + ")."
		}
		warehouse := entry.warehouse
		warehouseID := warehouse.ID
		collector.add(Alert{Type: alertCountDue, Warehouse: &warehouse, OccurredAt: entry.last,
			Reference: AlertReference{Kind: "warehouse", ID: &warehouseID, Label: warehouse.Name}, Message: message})
	}
	return nil
}

// countDue indica si un almacén debe contarse: lastDay y today son días operativos (fechas a
// medianoche UTC). Diaria: si el último conteo es de un día anterior; semanal: si tiene 7 días o
// más; mensual: si es del mismo día del mes anterior o antes; cada N días: si tiene N días o más.
// Sin ningún conteo aplicado, siempre. Es una función pura.
func countDue(frequency string, interval *int, lastDay *time.Time, today time.Time) bool {
	if frequency == "none" {
		return false
	}
	if lastDay == nil {
		return true
	}
	switch frequency {
	case "daily":
		return lastDay.Before(today)
	case "weekly":
		return !lastDay.After(today.AddDate(0, 0, -7))
	case "monthly":
		return !lastDay.After(today.AddDate(0, -1, 0))
	case "custom":
		if interval == nil || *interval < 1 {
			return false
		}
		return !lastDay.After(today.AddDate(0, 0, -*interval))
	}
	return false
}

// partialOrders: órdenes de compra parcialmente recibidas (solo con permisos de compras).
func (collector *alertCollector) partialOrders() error {
	if !collector.purchasing {
		return nil
	}
	rows, err := collector.handler.tx.Query(collector.handler.ctx, `SELECT o.id,o.folio,s.trade_name,
            (SELECT max(r.occurred_at) FROM app.inventory_receipts r WHERE r.purchase_order_id=o.id)
        FROM app.inventory_purchase_orders o JOIN app.inventory_suppliers s ON s.id=o.supplier_id
        WHERE o.state='partially_received'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, supplier string
		var folio int64
		var last *time.Time
		if err := rows.Scan(&id, &folio, &supplier, &last); err != nil {
			return err
		}
		orderID := id
		collector.add(Alert{Type: alertPartialOrder, OccurredAt: last,
			Reference: AlertReference{Kind: "purchase_order", ID: &orderID, Label: "Orden de compra " + itoa(int(folio))},
			Message: "La orden de compra " + itoa(int(folio)) + " de «" + supplier + "» está parcialmente recibida: " +
				"recibe lo pendiente o ciérrala."})
	}
	return rows.Err()
}

// unclassifiedCancellations: cancelaciones de unidades ya consumidas pendientes de clasificar, en
// una sola alerta con el vencimiento más próximo.
func (collector *alertCollector) unclassifiedCancellations() error {
	var pending int
	var deadline *time.Time
	err := collector.handler.tx.QueryRow(collector.handler.ctx, `SELECT count(*)::int,min(deadline_at)
        FROM app.inventory_cancellation_classifications WHERE state='pending'`).Scan(&pending, &deadline)
	if err != nil || pending == 0 {
		return err
	}
	collector.add(Alert{Type: alertUnclassified, Count: pending, OccurredAt: deadline,
		Reference: AlertReference{Kind: "cancellations", Label: "Cancelaciones pendientes"},
		Message: plural.Count(pending, "cancelación de una partida liberada está", "cancelaciones de partidas liberadas están") +
			" sin clasificar: al cierre del día operativo se tomará" + plural.Word(pending, "", "n") + " como «No preparado»."})
	return nil
}

// failedConsumption: solicitudes de consumo que no se pudieron calcular, en una sola alerta.
func (collector *alertCollector) failedConsumption() error {
	var failed int
	var oldest *time.Time
	err := collector.handler.tx.QueryRow(collector.handler.ctx, `SELECT count(*)::int,min(created_at)
        FROM app.inventory_consumption_requests WHERE state='failed'`).Scan(&failed, &oldest)
	if err != nil || failed == 0 {
		return err
	}
	collector.add(Alert{Type: alertConsumptionFailed, Count: failed, OccurredAt: oldest,
		Reference: AlertReference{Kind: "consumption_requests", Label: "Solicitudes de consumo"},
		Message: plural.Count(failed, "consumo de ventas no se pudo calcular", "consumos de ventas no se pudieron calcular") +
			"; se reintentará" + plural.Word(failed, "", "n") + " automáticamente. Revisa las recetas y porciones."})
	return nil
}

// correctedCashLinks: recepciones con un vínculo vigente a un gasto de caja que se corrigió o
// regularizó después (solo con permisos de compras). Una sola sentencia sobre cash_operations, sin
// datos del gasto.
func (collector *alertCollector) correctedCashLinks() error {
	if !collector.purchasing {
		return nil
	}
	rows, err := collector.handler.tx.Query(collector.handler.ctx, `SELECT r.id,r.folio,s.trade_name,count(*)::int,max(fix.created_at)
        FROM app.inventory_receipt_cash_links cl
        JOIN app.cash_operations fix ON fix.corrects_operation_id=cl.cash_operation_id
        JOIN app.inventory_receipts r ON r.id=cl.receipt_id JOIN app.inventory_suppliers s ON s.id=r.supplier_id
        WHERE cl.unlinked_at IS NULL GROUP BY r.id,r.folio,s.trade_name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, supplier string
		var folio int64
		var links int
		var corrected *time.Time
		if err := rows.Scan(&id, &folio, &supplier, &links, &corrected); err != nil {
			return err
		}
		receiptID := id
		collector.add(Alert{Type: alertCashLinkCorrected, Count: links, OccurredAt: corrected,
			Reference: AlertReference{Kind: "receipt", ID: &receiptID, Label: "Recepción " + itoa(int(folio))},
			Message: "La recepción " + itoa(int(folio)) + " de «" + supplier + "» tiene " +
				plural.Count(links, "gasto de caja vinculado que se corrigió", "gastos de caja vinculados que se corrigieron") +
				" después del vínculo: revísalo y desvincúlalo si ya no corresponde."})
	}
	return rows.Err()
}
