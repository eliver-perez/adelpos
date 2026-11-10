package inventory

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Insumos inventariables (§3 de la especificación). Son independientes de los productos de venta y
// de los ingredientes del catálogo: las recetas de la fase B los relacionan. Reglas:
//   - código y nombre únicos sin distinguir mayúsculas; código de barras opcional y único entre
//     insumos y presentaciones;
//   - la unidad base no cambia si el insumo tiene movimientos o presentaciones (sus cantidades
//     están en esa unidad);
//   - el costo de referencia (por unidad base) solo se entrega con inventory.costs.read y solo se
//     envía o cambia con inventory.costs.manage;
//   - existencia mínima y punto de reabastecimiento en la unidad base (vacíos = sin alerta).

var itemKindLabels = map[string]string{
	"purchased": "Comprado",
	"prepared":  "Preparación almacenable",
}

// Item es un insumo tal como lo entrega la API. Con costs_visible falso, reference_cost y
// reference_cost_display son null.
type Item struct {
	ID                   string         `json:"id"`
	Code                 string         `json:"code"`
	Barcode              *string        `json:"barcode"`
	Name                 string         `json:"name"`
	Description          string         `json:"description"`
	CategoryID           *string        `json:"category_id"`
	CategoryName         *string        `json:"category_name"`
	BaseUnit             UnitReference  `json:"base_unit"`
	Kind                 string         `json:"kind"`
	KindLabel            string         `json:"kind_label"`
	Active               bool           `json:"active"`
	CostsVisible         bool           `json:"costs_visible"`
	ReferenceCost        *string        `json:"reference_cost"`
	ReferenceCostDisplay *string        `json:"reference_cost_display"`
	MinStock             *string        `json:"min_stock"`
	ReorderPoint         *string        `json:"reorder_point"`
	HasMovements         bool           `json:"has_movements"`
	Presentations        []Presentation `json:"presentations"`
	Revision             int            `json:"revision"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	UpdatedByName        *string        `json:"updated_by_name"`
}

// Presentation es una presentación de compra; base_quantity está en la unidad base del insumo.
type Presentation struct {
	ID              string    `json:"id"`
	ItemID          string    `json:"item_id"`
	Name            string    `json:"name"`
	BaseQuantity    string    `json:"base_quantity"`
	Barcode         *string   `json:"barcode"`
	Active          bool      `json:"active"`
	PurchaseDefault bool      `json:"purchase_default"`
	Revision        int       `json:"revision"`
	UpdatedAt       time.Time `json:"updated_at"`
}

const itemColumns = `SELECT i.id,i.code,i.barcode,i.name,i.description,i.category_id,c.name,
        u.id,u.code,u.name,u.dimension,i.kind,i.active,i.reference_cost::text,i.min_stock::text,i.reorder_point::text,
        EXISTS(SELECT FROM app.inventory_movements m WHERE m.item_id=i.id) AS has_movements,
        i.revision,i.created_at,i.updated_at,updater.display_name
        FROM app.inventory_items i
        JOIN app.inventory_units u ON u.id=i.base_unit_id
        LEFT JOIN app.inventory_categories c ON c.id=i.category_id
        LEFT JOIN app.users updater ON updater.id=i.updated_by`

func scanItem(row interface{ Scan(...any) error }) (*Item, error) {
	var item Item
	var reference, minimumStock, reorder *string
	err := row.Scan(&item.ID, &item.Code, &item.Barcode, &item.Name, &item.Description, &item.CategoryID, &item.CategoryName,
		&item.BaseUnit.ID, &item.BaseUnit.Code, &item.BaseUnit.Name, &item.BaseUnit.Dimension, &item.Kind, &item.Active,
		&reference, &minimumStock, &reorder, &item.HasMovements, &item.Revision, &item.CreatedAt, &item.UpdatedAt,
		&item.UpdatedByName)
	if err != nil {
		return nil, err
	}
	item.KindLabel = itemKindLabels[item.Kind]
	item.CostsVisible = true
	if cost := parseDatabaseOptional(reference); cost != nil {
		exact := exactMoneyText(*cost)
		display := displayUnitCostText(*cost)
		item.ReferenceCost, item.ReferenceCostDisplay = &exact, &display
	}
	item.MinStock = optionalQuantityText(parseDatabaseOptional(minimumStock))
	item.ReorderPoint = optionalQuantityText(parseDatabaseOptional(reorder))
	item.Presentations = []Presentation{}
	return &item, nil
}

// present quita los costos de una respuesta si el actor no puede consultarlos. La auditoría
// conserva el registro completo.
func (handler *service) present(item *Item) *Item {
	if handler.costsVisible() {
		return item
	}
	copied := *item
	copied.CostsVisible = false
	copied.ReferenceCost = nil
	copied.ReferenceCostDisplay = nil
	return &copied
}

const presentationColumns = `SELECT id,item_id,name,base_quantity::text,barcode,active,purchase_default,revision,updated_at
        FROM app.inventory_presentations`

func scanPresentation(row interface{ Scan(...any) error }) (*Presentation, error) {
	var presentation Presentation
	var quantity string
	err := row.Scan(&presentation.ID, &presentation.ItemID, &presentation.Name, &quantity, &presentation.Barcode,
		&presentation.Active, &presentation.PurchaseDefault, &presentation.Revision, &presentation.UpdatedAt)
	if err != nil {
		return nil, err
	}
	presentation.BaseQuantity = quantityText(parseDatabaseDecimal(quantity))
	return &presentation, nil
}

// attachPresentations carga las presentaciones de los insumos de una página en una sola consulta.
func (handler *service) attachPresentations(items []*Item) error {
	if len(items) == 0 {
		return nil
	}
	identifiers := make([]string, 0, len(items))
	byID := map[string]*Item{}
	for _, item := range items {
		identifiers = append(identifiers, item.ID)
		byID[item.ID] = item
	}
	rows, err := handler.tx.Query(handler.ctx, presentationColumns+` WHERE item_id = ANY($1::uuid[])
        ORDER BY purchase_default DESC, active DESC, base_quantity, name, id`, identifiers)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		presentation, err := scanPresentation(rows)
		if err != nil {
			return err
		}
		if item := byID[presentation.ItemID]; item != nil {
			item.Presentations = append(item.Presentations, *presentation)
		}
	}
	return rows.Err()
}

// readItem devuelve el insumo completo (con costo) y sus presentaciones.
func (handler *service) readItem(id string) (*Item, error) {
	item, err := scanItem(handler.tx.QueryRow(handler.ctx, itemColumns+" WHERE i.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("El insumo no existe.")
	}
	if err != nil {
		return nil, err
	}
	if err := handler.attachPresentations([]*Item{item}); err != nil {
		return nil, err
	}
	return item, nil
}

func (handler *service) readItemRoute(parts []string) (any, error) {
	item, err := handler.readItem(parts[1])
	if err != nil {
		return nil, err
	}
	return handler.present(item), nil
}

// ItemPage es una página del listado de insumos.
type ItemPage struct {
	Items        []*Item `json:"items"`
	Total        int     `json:"total"`
	Page         int     `json:"page"`
	PageSize     int     `json:"page_size"`
	CostsVisible bool    `json:"costs_visible"`
}

// listItems lista insumos con búsqueda (código, nombre o código de barras, también de sus
// presentaciones), filtros de estado, categoría y tipo, y paginación.
func (handler *service) listItems(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "search", "active", "category_id", "kind", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	pattern, err := searchPattern(query)
	if err != nil {
		return nil, err
	}
	if pattern != "" {
		set.add(`(i.name ILIKE $%[1]d OR i.code ILIKE $%[1]d OR i.barcode ILIKE $%[1]d OR EXISTS(
            SELECT FROM app.inventory_presentations p WHERE p.item_id=i.id AND p.barcode ILIKE $%[1]d))`, pattern)
	}
	active, err := optionalBool(query, "active", "estado")
	if err != nil {
		return nil, err
	}
	if active != nil {
		set.add("i.active=$%d", *active)
	}
	if category := query.Get("category_id"); category == "none" {
		set.clauses = append(set.clauses, "i.category_id IS NULL")
	} else {
		categoryID, err := optionalQueryID(query, "category_id", "categoría")
		if err != nil {
			return nil, err
		}
		if categoryID != nil {
			set.add("i.category_id=$%d", *categoryID)
		}
	}
	if kind := query.Get("kind"); kind != "" {
		if _, known := itemKindLabels[kind]; !known {
			return nil, invalid("El filtro de tipo de insumo no es válido.")
		}
		set.add("i.kind=$%d", kind)
	}
	var total int
	err = handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_items i"+set.where(), set.arguments...).Scan(&total)
	if err != nil {
		return nil, err
	}
	statement := itemColumns + set.where() + " ORDER BY i.active DESC, i.name, i.id LIMIT " + set.next(size) +
		" OFFSET " + set.next((page-1)*size)
	rows, err := handler.tx.Query(handler.ctx, statement, set.arguments...)
	if err != nil {
		return nil, err
	}
	items := []*Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := handler.attachPresentations(items); err != nil {
		return nil, err
	}
	for index, item := range items {
		items[index] = handler.present(item)
	}
	return ItemPage{Items: items, Total: total, Page: page, PageSize: size, CostsVisible: handler.costsVisible()}, nil
}

// optionalText distingue un campo ausente (Set falso: no se cambia) de un valor o de null.
type optionalText struct {
	Set   bool
	Value *string
}

func (field *optionalText) UnmarshalJSON(data []byte) error {
	field.Set = true
	if string(data) == "null" {
		field.Value = nil
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	field.Value = &text
	return nil
}

type itemInput struct {
	OperationID string  `json:"operation_id"`
	Revision    int     `json:"revision"`
	Code        string  `json:"code"`
	Barcode     *string `json:"barcode"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	CategoryID  *string `json:"category_id"`
	BaseUnitID  string  `json:"base_unit_id"`
	Kind        string  `json:"kind"`
	Active      bool    `json:"active"`
	// ReferenceCost ausente conserva el costo vigente; enviarlo (valor o null) exige
	// inventory.costs.manage.
	ReferenceCost optionalText `json:"reference_cost"`
	MinStock      *string      `json:"min_stock"`
	ReorderPoint  *string      `json:"reorder_point"`
}

// itemValues son los datos validados de un insumo listos para guardar.
type itemValues struct {
	barcode       *string
	categoryID    *string
	baseUnitID    string
	referenceCost *decimal.Decimal
	minStock      *decimal.Decimal
	reorderPoint  *decimal.Decimal
}

// validateItem recorta y valida los campos simples. Los permisos de costo se comprueban aquí, antes
// de cualquier escritura.
func (handler *service) validateItem(input *itemInput) (itemValues, error) {
	values := itemValues{}
	if input.ReferenceCost.Set {
		if err := handler.request.Actor.Require(permissionCostsManage); err != nil {
			return values, err
		}
	}
	if err := validateName(&input.Code, "El código interno del insumo", 40); err != nil {
		return values, err
	}
	if err := validateName(&input.Name, "El nombre del insumo", 100); err != nil {
		return values, err
	}
	if err := validateDescription(&input.Description, 2000); err != nil {
		return values, err
	}
	if input.Kind == "" {
		input.Kind = "purchased"
	}
	if _, known := itemKindLabels[input.Kind]; !known {
		return values, invalid("El tipo de insumo no es válido.")
	}
	barcode, err := normalizeBarcode(input.Barcode)
	if err != nil {
		return values, err
	}
	values.barcode = barcode
	category, err := validateOptionalID(input.CategoryID)
	if err != nil {
		return values, err
	}
	values.categoryID = category
	if err := validateID(input.BaseUnitID); err != nil {
		return values, invalid("Elige la unidad base del insumo.")
	}
	values.baseUnitID = strings.ToLower(input.BaseUnitID)
	if input.ReferenceCost.Set && input.ReferenceCost.Value != nil && strings.TrimSpace(*input.ReferenceCost.Value) != "" {
		cost, err := parseCost(*input.ReferenceCost.Value, "El costo de referencia")
		if err != nil {
			return values, err
		}
		if !cost.IsPositive() {
			return values, invalid("El costo de referencia debe ser mayor que cero; déjalo vacío si no hay costo.")
		}
		values.referenceCost = &cost
	}
	if values.minStock, err = parseOptionalQuantity(input.MinStock, "La existencia mínima"); err != nil {
		return values, err
	}
	if values.reorderPoint, err = parseOptionalQuantity(input.ReorderPoint, "El punto de reabastecimiento"); err != nil {
		return values, err
	}
	return values, nil
}

// checkItemReferences valida categoría y unidad base. Una categoría o unidad inactiva solo se
// conserva si ya era la del insumo; no se asigna de nuevo.
func (handler *service) checkItemReferences(values itemValues, previous *Item) error {
	if values.categoryID != nil && (previous == nil || previous.CategoryID == nil || *previous.CategoryID != *values.categoryID) {
		var active bool
		err := handler.tx.QueryRow(handler.ctx, "SELECT active FROM app.inventory_categories WHERE id=$1", *values.categoryID).Scan(&active)
		if isNoRows(err) {
			return notFound("La categoría de insumos no existe.")
		}
		if err != nil {
			return err
		}
		if !active {
			return conflict("CATEGORY_INACTIVE", "La categoría elegida está desactivada.")
		}
	}
	inactiveAllowed := previous != nil && previous.BaseUnit.ID == values.baseUnitID
	if _, _, err := handler.unitMeasure(values.baseUnitID, inactiveAllowed); err != nil {
		return err
	}
	return nil
}

// barcodeTaken comprueba que un código de barras no exista ya en otro insumo ni en otra
// presentación (el lector debe identificar un solo registro).
func (handler *service) barcodeTaken(barcode *string, itemID, presentationID string) error {
	if barcode == nil {
		return nil
	}
	var taken bool
	err := handler.tx.QueryRow(handler.ctx, `SELECT EXISTS(SELECT FROM app.inventory_items WHERE barcode=$1 AND id::text<>$2)
        OR EXISTS(SELECT FROM app.inventory_presentations WHERE barcode=$1 AND id::text<>$3)`,
		*barcode, itemID, presentationID).Scan(&taken)
	if err != nil {
		return err
	}
	if taken {
		return conflict("DUPLICATE_BARCODE", "El código de barras ya pertenece a otro insumo o presentación.")
	}
	return nil
}

func itemConflict(err error) error {
	if !uniqueViolation(err) {
		return err
	}
	switch name := constraintName(err); {
	case strings.Contains(name, "code"):
		return conflict("DUPLICATE_CODE", "Ya existe un insumo con ese código interno.")
	case strings.Contains(name, "barcode"):
		return conflict("DUPLICATE_BARCODE", "El código de barras ya pertenece a otro insumo o presentación.")
	}
	return conflict("DUPLICATE_NAME", "Ya existe un insumo con ese nombre.")
}

// createItem da de alta un insumo. Sin reference_cost queda sin costo de referencia.
func (handler *service) createItem(_ []string) (any, error) {
	var input itemInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	values, err := handler.validateItem(&input)
	if err != nil {
		return nil, err
	}
	if err := handler.checkItemReferences(values, nil); err != nil {
		return nil, err
	}
	if err := handler.barcodeTaken(values.barcode, "", ""); err != nil {
		return nil, err
	}
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_items(id,code,barcode,name,description,category_id,base_unit_id,
            kind,active,reference_cost,min_stock,reorder_point,created_by,updated_by)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::numeric,$11::numeric,$12::numeric,$13,$13)`,
		id, input.Code, values.barcode, input.Name, input.Description, values.categoryID, values.baseUnitID, input.Kind,
		input.Active, optionalDecimalText(values.referenceCost), optionalDecimalText(values.minStock),
		optionalDecimalText(values.reorderPoint), handler.actorReference())
	if err != nil {
		return nil, itemConflict(err)
	}
	saved, err := handler.readItem(id)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.items.changed", &id); err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, "inventory.item_created", "inventory_item", &id, nil, saved); err != nil {
		return nil, err
	}
	return handler.present(saved), nil
}

// updateItem guarda todos los datos del insumo con revisión optimista.
func (handler *service) updateItem(parts []string) (any, error) {
	var input itemInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	values, err := handler.validateItem(&input)
	if err != nil {
		return nil, err
	}
	previous, err := handler.readItem(parts[1])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("El insumo")
	}
	if err := handler.checkItemReferences(values, previous); err != nil {
		return nil, err
	}
	if values.baseUnitID != previous.BaseUnit.ID {
		if err := handler.checkBaseUnitChange(previous, input.ReferenceCost.Set); err != nil {
			return nil, err
		}
	}
	if err := handler.barcodeTaken(values.barcode, previous.ID, ""); err != nil {
		return nil, err
	}
	reference := parseOptionalCostText(previous.ReferenceCost)
	if input.ReferenceCost.Set {
		reference = values.referenceCost
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_items SET code=$2,barcode=$3,name=$4,description=$5,category_id=$6,
            base_unit_id=$7,kind=$8,active=$9,reference_cost=$10::numeric,min_stock=$11::numeric,reorder_point=$12::numeric,
            updated_by=$13,revision=revision+1,updated_at=now()
        WHERE id=$1`,
		previous.ID, input.Code, values.barcode, input.Name, input.Description, values.categoryID, values.baseUnitID, input.Kind,
		input.Active, optionalDecimalText(reference), optionalDecimalText(values.minStock), optionalDecimalText(values.reorderPoint),
		handler.actorReference())
	if err != nil {
		return nil, itemConflict(err)
	}
	saved, err := handler.readItem(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.items.changed", &saved.ID); err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, "inventory.item_updated", "inventory_item", &saved.ID, previous, saved); err != nil {
		return nil, err
	}
	return handler.present(saved), nil
}

// checkBaseUnitChange aplica las reglas de cambio de unidad base: nunca con movimientos (las
// cantidades registradas están en la unidad anterior), nunca con presentaciones (su equivalencia
// también) y, si hay costo de referencia, solo capturándolo otra vez por la unidad nueva.
func (handler *service) checkBaseUnitChange(previous *Item, referenceSent bool) error {
	if previous.HasMovements {
		return conflict("BASE_UNIT_LOCKED", "El insumo ya tiene movimientos: su unidad base no puede cambiar.")
	}
	if len(previous.Presentations) > 0 {
		return conflict("BASE_UNIT_HAS_PRESENTATIONS",
			"El insumo tiene presentaciones expresadas en su unidad base: no se puede cambiar la unidad.")
	}
	if previous.ReferenceCost != nil && !referenceSent {
		return conflict("REFERENCE_COST_REQUIRED",
			"Cambiar la unidad base exige capturar de nuevo el costo de referencia por la unidad nueva.")
	}
	return nil
}

func parseOptionalCostText(text *string) *decimal.Decimal {
	if text == nil {
		return nil
	}
	value := decimal.RequireFromString(*text)
	return &value
}
