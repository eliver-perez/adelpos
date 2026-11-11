package inventory

import (
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Sugerencias de compra (§24 «Preparar sugerencias de compra basadas en existencias, pero no emitir
// órdenes automáticamente sin intervención autorizada»). Reglas:
//   - por insumo comprado activo con punto de reabastecimiento: el del insumo o, si no tiene, la suma
//     de los puntos de reabastecimiento de sus almacenes (niveles por almacén);
//   - necesidad = punto de reabastecimiento − (existencia total + pendiente de recibir de órdenes
//     emitidas o parcialmente recibidas); solo se sugiere si es mayor que cero. Los borradores no
//     cuentan como pendiente (se informan aparte en draft_quantity);
//   - se expresa en la presentación de compra: la habitual del proveedor (preferido en la consulta,
//     elegido en el borrador) si está activa; si no, la presentación predeterminada de compra del
//     insumo; si no, la unidad base. Se redondea hacia arriba a presentaciones completas (o a
//     unidades base enteras): ninguna sugerencia queda por debajo de la necesidad;
//   - la consulta no escribe nada; crear un borrador es una acción explícita
//     (POST purchase-suggestions/draft-order, inventory.purchases.create) que nunca emite la orden;
//   - el proveedor y el último precio solo con algún permiso de compras; las cantidades de
//     existencia y pendiente solo con inventory.stock.read o inventory.purchases.create;
//   - sin esos permisos (stock_visible false) tampoco viajan el punto de reabastecimiento ni la
//     cantidad sugerida (en presentaciones y en la unidad base): con el punto y lo sugerido se
//     deduciría la existencia. La sugerencia sigue listando el insumo, igual que la alerta de
//     reabastecimiento sin cantidades. Crear el borrador exige inventory.purchases.create, que sí
//     los ve.

var one = decimal.NewFromInt(1)

// defaultSuggestionTaxRate es la tasa de IVA de un renglón sugerido sin recepciones anteriores del
// mismo proveedor e insumo (la tasa general; el borrador se revisa antes de emitirlo).
const defaultSuggestionTaxRate = 16

// PurchaseSuggestion es la sugerencia de compra de un insumo.
type PurchaseSuggestion struct {
	Item                  ItemReference          `json:"item"`
	CategoryName          *string                `json:"category_name"`
	ReorderPoint          *string                `json:"reorder_point"`
	ReorderSource         string                 `json:"reorder_source"`
	MinStock              *string                `json:"min_stock"`
	StockVisible          bool                   `json:"stock_visible"`
	Stock                 *string                `json:"stock"`
	PendingQuantity       *string                `json:"pending_quantity"`
	DraftQuantity         *string                `json:"draft_quantity"`
	NeedQuantity          *string                `json:"need_quantity"`
	BelowMinimum          *bool                  `json:"below_minimum"`
	PurchasingVisible     bool                   `json:"purchasing_visible"`
	Supplier              *SupplierReference     `json:"supplier"`
	SupplierCode          *string                `json:"supplier_code"`
	Presentation          *PresentationReference `json:"presentation"`
	Unit                  *UnitReference         `json:"unit"`
	CaptureLabel          string                 `json:"capture_label"`
	SuggestedQuantity     *string                `json:"suggested_quantity"`
	SuggestedBaseQuantity *string                `json:"suggested_base_quantity"`
	LastUnitPrice         *string                `json:"last_unit_price"`
	LastPriceLabel        *string                `json:"last_price_label"`
	LastPriceAt           *time.Time             `json:"last_price_at"`
	// Datos para el borrador.
	suggested      decimal.Decimal
	presentationID *string
	lastPrice      *decimal.Decimal
	lastPriceMatch bool
}

// PurchaseSuggestionPage es una página de sugerencias (por proveedor preferido e insumo).
type PurchaseSuggestionPage struct {
	Items             []PurchaseSuggestion `json:"items"`
	Total             int                  `json:"total"`
	Page              int                  `json:"page"`
	PageSize          int                  `json:"page_size"`
	StockVisible      bool                 `json:"stock_visible"`
	PurchasingVisible bool                 `json:"purchasing_visible"`
}

// suggestionNeed es lo que se calcula por insumo antes de elegir proveedor y presentación.
type suggestionNeed struct {
	item           ItemReference
	categoryName   *string
	reorder        decimal.Decimal
	fromWarehouses bool
	minimum        *decimal.Decimal
	stock          decimal.Decimal
	pending        decimal.Decimal
	drafted        decimal.Decimal
	need           decimal.Decimal
}

// supplierOffer es la relación de un proveedor con un insumo.
type supplierOffer struct {
	supplier     SupplierReference
	preferred    bool
	code         string
	presentation *PresentationReference
	lastPrice    *decimal.Decimal
	lastByPres   *string
	lastByUnit   *string
	lastAt       *time.Time
	lastLabel    *string
}

// suggestedQuantity redondea la necesidad hacia arriba a presentaciones completas (cantidad de
// presentaciones y su equivalente en la unidad base) o, sin presentación, a unidades base enteras.
// Es una función pura.
func suggestedQuantity(need decimal.Decimal, presentationBase *decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	if presentationBase == nil || !presentationBase.IsPositive() {
		units := need.Ceil()
		return units, units
	}
	quotient, remainder := need.QuoRem(*presentationBase, 0)
	if remainder.IsPositive() {
		quotient = quotient.Add(one)
	}
	return quotient, quotient.Mul(*presentationBase).Round(quantityScale)
}

// suggestionNeeds calcula la necesidad de los insumos comprados activos con punto de
// reabastecimiento; con itemIDs, solo de esos insumos.
func (handler *service) suggestionNeeds(itemIDs []string) ([]suggestionNeed, error) {
	filter := ""
	arguments := []any{}
	if itemIDs != nil {
		filter = " AND i.id = ANY($1::uuid[])"
		arguments = append(arguments, itemIDs)
	}
	rows, err := handler.tx.Query(handler.ctx, `WITH stock AS (
            SELECT item_id, sum(quantity) AS quantity FROM app.inventory_stock GROUP BY item_id
        ), levels AS (
            SELECT item_id, sum(reorder_point) AS reorder_point FROM app.inventory_stock_levels
            WHERE reorder_point IS NOT NULL GROUP BY item_id
        ), received AS (
            SELECT order_id, order_line_no, sum(base_quantity) AS base_quantity FROM app.inventory_receipt_lines
            WHERE order_id IS NOT NULL GROUP BY order_id, order_line_no
        ), ordered AS (
            SELECT l.item_id,
                sum(CASE WHEN o.state IN ('issued','partially_received')
                    THEN greatest(l.base_quantity - coalesce(r.base_quantity,0), 0) ELSE 0 END) AS pending,
                sum(CASE WHEN o.state='draft' THEN l.base_quantity ELSE 0 END) AS drafted
            FROM app.inventory_purchase_order_lines l JOIN app.inventory_purchase_orders o ON o.id=l.order_id
            LEFT JOIN received r ON r.order_id=l.order_id AND r.order_line_no=l.line_no
            WHERE o.state IN ('draft','issued','partially_received') GROUP BY l.item_id
        )
        SELECT `+alertItemColumns+`,cat.name,coalesce(i.reorder_point,lv.reorder_point)::text,i.reorder_point IS NULL,
            i.min_stock::text,coalesce(st.quantity,0)::text,coalesce(od.pending,0)::text,coalesce(od.drafted,0)::text
        FROM app.inventory_items i JOIN app.inventory_units u ON u.id=i.base_unit_id
        LEFT JOIN app.inventory_categories cat ON cat.id=i.category_id
        LEFT JOIN stock st ON st.item_id=i.id LEFT JOIN levels lv ON lv.item_id=i.id LEFT JOIN ordered od ON od.item_id=i.id
        WHERE i.active AND i.kind='purchased' AND coalesce(i.reorder_point,lv.reorder_point) IS NOT NULL`+filter+`
        ORDER BY lower(i.name), i.id`, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	needs := []suggestionNeed{}
	for rows.Next() {
		var entry suggestionNeed
		var reorder, stock, pending, drafted string
		var minimum *string
		if err := rows.Scan(append(alertItemTargets(&entry.item), &entry.categoryName, &reorder, &entry.fromWarehouses, &minimum, &stock,
			&pending, &drafted)...); err != nil {
			return nil, err
		}
		entry.reorder = parseDatabaseDecimal(reorder)
		entry.minimum = parseDatabaseOptional(minimum)
		entry.stock = parseDatabaseDecimal(stock)
		entry.pending = parseDatabaseDecimal(pending)
		entry.drafted = parseDatabaseDecimal(drafted)
		entry.need = entry.reorder.Sub(entry.stock.Add(entry.pending))
		if entry.need.IsPositive() {
			needs = append(needs, entry)
		}
	}
	return needs, rows.Err()
}

// supplierOffers lee, por insumo, la relación con el proveedor preferido (supplierID vacío) o con el
// proveedor indicado. Un proveedor desactivado no cuenta como preferido.
func (handler *service) supplierOffers(itemIDs []string, supplierID string) (map[string]*supplierOffer, error) {
	condition := "si.preferred AND sp.active"
	arguments := []any{itemIDs}
	if supplierID != "" {
		condition = "si.supplier_id=$2"
		arguments = append(arguments, supplierID)
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT si.item_id,sp.id,sp.trade_name,sp.kind,sp.active,si.preferred,si.supplier_code,
            p.id,p.name,p.base_quantity::text,p.active,si.last_unit_price::text,si.last_price_presentation_id,si.last_price_unit_id,
            si.last_price_at,coalesce(lp.name,lu.name)
        FROM app.inventory_supplier_items si JOIN app.inventory_suppliers sp ON sp.id=si.supplier_id
        LEFT JOIN app.inventory_presentations p ON p.id=si.presentation_id
        LEFT JOIN app.inventory_presentations lp ON lp.id=si.last_price_presentation_id
        LEFT JOIN app.inventory_units lu ON lu.id=si.last_price_unit_id
        WHERE si.item_id = ANY($1::uuid[]) AND sp.kind='formal' AND `+condition, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	offers := map[string]*supplierOffer{}
	for rows.Next() {
		var itemID string
		var offer supplierOffer
		var presentationID, presentationName, presentationBase, lastPrice *string
		var presentationActive *bool
		if err := rows.Scan(&itemID, &offer.supplier.ID, &offer.supplier.TradeName, &offer.supplier.Kind, &offer.supplier.Active,
			&offer.preferred, &offer.code,
			&presentationID, &presentationName, &presentationBase, &presentationActive, &lastPrice, &offer.lastByPres, &offer.lastByUnit,
			&offer.lastAt, &offer.lastLabel); err != nil {
			return nil, err
		}
		if presentationID != nil && *presentationActive {
			offer.presentation = &PresentationReference{ID: *presentationID, Name: *presentationName,
				BaseQuantity: quantityText(parseDatabaseDecimal(*presentationBase)), Active: true}
		}
		offer.lastPrice = parseDatabaseOptional(lastPrice)
		copied := offer
		offers[itemID] = &copied
	}
	return offers, rows.Err()
}

// defaultPresentations lee la presentación predeterminada de compra activa de cada insumo.
func (handler *service) defaultPresentations(itemIDs []string) (map[string]*PresentationReference, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT item_id,id,name,base_quantity::text FROM app.inventory_presentations
        WHERE item_id = ANY($1::uuid[]) AND purchase_default AND active`, itemIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	presentations := map[string]*PresentationReference{}
	for rows.Next() {
		var itemID, base string
		presentation := &PresentationReference{Active: true}
		if err := rows.Scan(&itemID, &presentation.ID, &presentation.Name, &base); err != nil {
			return nil, err
		}
		presentation.BaseQuantity = quantityText(parseDatabaseDecimal(base))
		presentations[itemID] = presentation
	}
	return presentations, rows.Err()
}

// buildSuggestions arma las sugerencias de los insumos con necesidad, con el proveedor preferido
// (supplierID vacío) o con el indicado.
func (handler *service) buildSuggestions(itemIDs []string, supplierID string) ([]PurchaseSuggestion, error) {
	needs, err := handler.suggestionNeeds(itemIDs)
	if err != nil {
		return nil, err
	}
	if len(needs) == 0 {
		return []PurchaseSuggestion{}, nil
	}
	identifiers := make([]string, 0, len(needs))
	for _, entry := range needs {
		identifiers = append(identifiers, entry.item.ID)
	}
	offers, err := handler.supplierOffers(identifiers, supplierID)
	if err != nil {
		return nil, err
	}
	defaults, err := handler.defaultPresentations(identifiers)
	if err != nil {
		return nil, err
	}
	stockVisible := handler.request.Actor.Can(permissionStockRead) || handler.request.Actor.Can(permissionPurchasesCreate)
	purchasing := handler.purchasingVisible()
	suggestions := make([]PurchaseSuggestion, 0, len(needs))
	for _, entry := range needs {
		item := entry.item
		suggestion := PurchaseSuggestion{Item: item, CategoryName: entry.categoryName, ReorderSource: "item",
			MinStock: optionalQuantityText(entry.minimum), StockVisible: stockVisible, PurchasingVisible: purchasing}
		if entry.fromWarehouses {
			suggestion.ReorderSource = "warehouses"
		}
		if stockVisible {
			stock, pending, drafted, need, reorder := entry.stock, entry.pending, entry.drafted, entry.need, entry.reorder
			suggestion.ReorderPoint = optionalQuantityText(&reorder)
			suggestion.Stock = optionalQuantityText(&stock)
			suggestion.PendingQuantity = optionalQuantityText(&pending)
			suggestion.DraftQuantity = optionalQuantityText(&drafted)
			suggestion.NeedQuantity = optionalQuantityText(&need)
			below := entry.minimum != nil && stock.LessThan(*entry.minimum)
			suggestion.BelowMinimum = &below
		}
		offer := offers[item.ID]
		presentation := defaults[item.ID]
		if offer != nil && offer.presentation != nil {
			presentation = offer.presentation
		}
		var presentationBase *decimal.Decimal
		if presentation != nil {
			base := decimal.RequireFromString(presentation.BaseQuantity)
			presentationBase = &base
			suggestion.Presentation = presentation
			suggestion.presentationID = &presentation.ID
			suggestion.CaptureLabel = presentation.Name
		} else {
			unit := item.Unit
			suggestion.Unit = &unit
			suggestion.CaptureLabel = unit.Name
		}
		quantity, base := suggestedQuantity(entry.need, presentationBase)
		suggestion.suggested = quantity
		if stockVisible {
			suggestion.SuggestedQuantity = optionalQuantityText(&quantity)
			suggestion.SuggestedBaseQuantity = optionalQuantityText(&base)
		}
		if offer != nil {
			// El último precio solo sirve para el borrador si se capturó en la misma presentación o
			// unidad que la sugerencia.
			suggestion.lastPrice = offer.lastPrice
			if presentation != nil {
				suggestion.lastPriceMatch = offer.lastByPres != nil && *offer.lastByPres == presentation.ID
			} else {
				suggestion.lastPriceMatch = offer.lastByUnit != nil && *offer.lastByUnit == item.Unit.ID
			}
			if purchasing {
				supplier := offer.supplier
				code := offer.code
				suggestion.Supplier, suggestion.SupplierCode = &supplier, &code
				if offer.lastPrice != nil {
					price := exactMoneyText(*offer.lastPrice)
					suggestion.LastUnitPrice, suggestion.LastPriceLabel, suggestion.LastPriceAt = &price, offer.lastLabel, offer.lastAt
				}
			}
		}
		suggestions = append(suggestions, suggestion)
	}
	return suggestions, nil
}

// listPurchaseSuggestions lista las sugerencias con filtros de proveedor preferido (none = sin
// proveedor preferido), categoría y búsqueda, ordenadas por proveedor e insumo.
func (handler *service) listPurchaseSuggestions(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "supplier_id", "category_id", "search", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	supplierFilter := query.Get("supplier_id")
	if supplierFilter != "" && supplierFilter != "none" {
		if !validID(supplierFilter) {
			return nil, invalid("El filtro de proveedor no es válido.")
		}
		supplierFilter = strings.ToLower(supplierFilter)
	}
	if supplierFilter != "" && !handler.purchasingVisible() {
		return nil, denied()
	}
	categoryID, err := optionalQueryID(query, "category_id", "categoría")
	if err != nil {
		return nil, err
	}
	search := strings.ToLower(strings.TrimSpace(query.Get("search")))
	if textLength(search) > 100 {
		return nil, invalid("La búsqueda admite hasta 100 caracteres.")
	}
	var itemIDs []string
	if categoryID != nil {
		if itemIDs, err = handler.orderedItems("i.category_id=$1", *categoryID); err != nil {
			return nil, err
		}
	}
	suggestions, err := handler.buildSuggestions(itemIDs, "")
	if err != nil {
		return nil, err
	}
	filtered := []PurchaseSuggestion{}
	for _, suggestion := range suggestions {
		switch {
		case supplierFilter == "none" && suggestion.Supplier != nil:
			continue
		case supplierFilter != "" && supplierFilter != "none" && (suggestion.Supplier == nil || suggestion.Supplier.ID != supplierFilter):
			continue
		case search != "" && !strings.Contains(strings.ToLower(suggestion.Item.Name), search) &&
			!strings.Contains(strings.ToLower(suggestion.Item.Code), search):
			continue
		}
		filtered = append(filtered, suggestion)
	}
	// Por proveedor preferido (los que no tienen, al final) y nombre del insumo; el orden por nombre
	// ya viene de la consulta.
	ordered := make([]PurchaseSuggestion, 0, len(filtered))
	for _, withSupplier := range []bool{true, false} {
		groups := map[string][]PurchaseSuggestion{}
		names := []string{}
		for _, suggestion := range filtered {
			if (suggestion.Supplier != nil) != withSupplier {
				continue
			}
			key := ""
			if suggestion.Supplier != nil {
				key = strings.ToLower(suggestion.Supplier.TradeName) + "\x00" + suggestion.Supplier.ID
			}
			if _, found := groups[key]; !found {
				names = append(names, key)
			}
			groups[key] = append(groups[key], suggestion)
		}
		slices.Sort(names)
		for _, name := range names {
			ordered = append(ordered, groups[name]...)
		}
	}
	result := PurchaseSuggestionPage{Items: []PurchaseSuggestion{}, Total: len(ordered), Page: page, PageSize: size,
		StockVisible:      handler.request.Actor.Can(permissionStockRead) || handler.request.Actor.Can(permissionPurchasesCreate),
		PurchasingVisible: handler.purchasingVisible()}
	start := min((page-1)*size, len(ordered))
	end := min(start+size, len(ordered))
	result.Items = append(result.Items, ordered[start:end]...)
	return result, nil
}

type draftOrderInput struct {
	OperationID         string   `json:"operation_id"`
	SupplierID          string   `json:"supplier_id"`
	ItemIDs             []string `json:"item_ids"`
	ExpectedWarehouseID *string  `json:"expected_warehouse_id"`
	Notes               string   `json:"notes"`
}

// draftOrderFromSuggestions crea un borrador de orden de compra para un proveedor con las
// sugerencias vigentes de los insumos elegidos: cantidad sugerida en su presentación, último precio
// del proveedor si se capturó en la misma presentación o unidad (si no, 0) y la tasa de IVA de su
// última recepción (si no, 16 %). Nunca emite la orden: el borrador se revisa y se emite aparte con
// inventory.purchases.authorize. Si una sugerencia ya no aplica, no crea nada.
func (handler *service) draftOrderFromSuggestions(_ []string) (any, error) {
	var input draftOrderInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	supplier, err := handler.supplierForPurchase(input.SupplierID, false)
	if err != nil {
		return nil, err
	}
	if len(input.ItemIDs) == 0 || len(input.ItemIDs) > maximumOrderLines {
		return nil, invalid("Elige de 1 a 200 insumos de las sugerencias.")
	}
	identifiers := make([]string, 0, len(input.ItemIDs))
	seen := map[string]bool{}
	for _, identifier := range input.ItemIDs {
		lower := strings.ToLower(identifier)
		if !validID(lower) {
			return nil, invalid("Un insumo elegido no es válido.")
		}
		if seen[lower] {
			return nil, invalid("Un insumo aparece más de una vez en la selección.")
		}
		seen[lower] = true
		identifiers = append(identifiers, lower)
	}
	suggestions, err := handler.buildSuggestions(identifiers, supplier.ID)
	if err != nil {
		return nil, err
	}
	bySuggestion := map[string]PurchaseSuggestion{}
	for _, suggestion := range suggestions {
		bySuggestion[suggestion.Item.ID] = suggestion
	}
	lines := make([]purchaseLineInput, 0, len(identifiers))
	for _, identifier := range identifiers {
		suggestion, found := bySuggestion[identifier]
		if !found {
			name := "Un insumo elegido"
			if item, err := handler.itemReference(identifier); err == nil {
				name = "«" + item.Name + "»"
			}
			return nil, conflict("SUGGESTION_CHANGED", name+" ya no necesita reabastecerse: actualiza las sugerencias.")
		}
		price := "0"
		if suggestion.lastPriceMatch && suggestion.lastPrice != nil {
			price = suggestion.lastPrice.String()
		}
		rate, err := handler.lastTaxRate(supplier.ID, identifier)
		if err != nil {
			return nil, err
		}
		line := purchaseLineInput{ItemID: identifier, Quantity: suggestion.suggested.String(), UnitPrice: price, TaxRate: &rate}
		if suggestion.presentationID != nil {
			line.PresentationID = suggestion.presentationID
		} else {
			unitID := suggestion.Item.Unit.ID
			line.UnitID = &unitID
		}
		lines = append(lines, line)
	}
	prepared, err := handler.prepareOrder(purchaseOrderInput{SupplierID: supplier.ID, ExpectedWarehouseID: input.ExpectedWarehouseID,
		Notes: input.Notes, Lines: lines})
	if err != nil {
		return nil, err
	}
	saved, err := handler.insertDraftOrder(prepared)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.purchases.changed", &saved.ID); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.purchase_order_drafted_from_suggestions", "inventory_purchase_order",
		&saved.ID, nil, saved)
}

// lastTaxRate es la tasa de IVA de la recepción más reciente del insumo con ese proveedor o, si no
// hay, la tasa general.
func (handler *service) lastTaxRate(supplierID, itemID string) (int, error) {
	var rate int
	err := handler.tx.QueryRow(handler.ctx, `SELECT l.tax_rate FROM app.inventory_receipt_lines l
        JOIN app.inventory_receipts r ON r.id=l.receipt_id WHERE r.supplier_id=$1 AND l.item_id=$2
        ORDER BY r.occurred_at DESC, r.folio DESC LIMIT 1`, supplierID, itemID).Scan(&rate)
	if isNoRows(err) {
		return defaultSuggestionTaxRate, nil
	}
	return rate, err
}
