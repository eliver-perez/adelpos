package inventory

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"local/ad-el-pos/server/internal/plural"
)

// Conteos físicos por almacén (§23 de la especificación y §0-B «Conteos físicos»). Reglas:
//   - crear, editar, capturar y enviar a revisión: inventory.counts.perform; aprobar y aplicar:
//     inventory.counts.approve (quien tiene ese permiso puede aprobar su propio conteo; nadie sin él);
//   - se eligen los insumos al crear (todos los activos, los activos de una categoría o una lista) y
//     se pueden agregar o quitar mientras el conteo está abierto. Un insumo no puede estar en dos
//     conteos sin terminar del mismo almacén;
//   - cada captura es una transacción corta (la venta sigue: no se retienen locks entre capturas) y
//     guarda la hora del renglón (captured_at). Capturar otra vez el mismo insumo sustituye la captura;
//   - diferencia = contado − existencia teórica a captured_at (Σ del libro del almacén con
//     occurred_at ≤ captured_at). Los movimientos posteriores a la hora del renglón se conservan;
//   - antes de aplicar se procesan, dentro de la misma transacción, las solicitudes de consumo
//     pendientes liberadas hasta la hora máxima de los renglones (sin esperar al sondeo del
//     trabajador); si alguna no se puede calcular, el conteo no se aplica;
//   - cada renglón contado con diferencia genera un count_in (sobrante, regla 3 del motor) o un
//     count_out (faltante, regla 1) con occurred_at = captured_at y entry_key por renglón; un renglón
//     sin captura no se ajusta. Aplicar dos veces no genera otro ajuste (estado + entry_key);
//   - la existencia teórica y las diferencias solo las ve quien aprueba o consulta existencias
//     (inventory.counts.approve o inventory.stock.read): quien solo cuenta captura a ciegas.

const maximumCountLines = 2000

var countStateLabels = map[string]string{
	"open":      "Abierto",
	"submitted": "Enviado a revisión",
	"approved":  "Aprobado",
	"applied":   "Aplicado",
	"cancelled": "Cancelado",
}

var countScopeLabels = map[string]string{
	"all":      "Todos los insumos activos",
	"category": "Insumos de una categoría",
	"items":    "Insumos seleccionados",
}

// activeCountStates son los estados de un conteo sin terminar.
var activeCountStates = []string{"open", "submitted", "approved"}

// CategoryReference identifica una categoría de insumos por nombre.
type CategoryReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CountSummary es un conteo en listados. DifferenceLines solo se entrega en un conteo aplicado y a
// quien ve la existencia teórica.
type CountSummary struct {
	ID                 string             `json:"id"`
	Folio              int64              `json:"folio"`
	Warehouse          WarehouseReference `json:"warehouse"`
	Scope              string             `json:"scope"`
	ScopeLabel         string             `json:"scope_label"`
	Category           *CategoryReference `json:"category"`
	State              string             `json:"state"`
	StateLabel         string             `json:"state_label"`
	Notes              string             `json:"notes"`
	LineCount          int                `json:"line_count"`
	CountedLines       int                `json:"counted_lines"`
	DifferenceLines    *int               `json:"difference_lines"`
	CreatedByName      string             `json:"created_by_name"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedByName      string             `json:"updated_by_name"`
	UpdatedAt          time.Time          `json:"updated_at"`
	SubmittedByName    *string            `json:"submitted_by_name"`
	SubmittedAt        *time.Time         `json:"submitted_at"`
	ApprovedByName     *string            `json:"approved_by_name"`
	ApprovedAt         *time.Time         `json:"approved_at"`
	AppliedByName      *string            `json:"applied_by_name"`
	AppliedAt          *time.Time         `json:"applied_at"`
	ReferenceAt        *time.Time         `json:"reference_at"`
	CancelledByName    *string            `json:"cancelled_by_name"`
	CancelledAt        *time.Time         `json:"cancelled_at"`
	CancelReason       *string            `json:"cancel_reason"`
	Revision           int                `json:"revision"`
	TheoreticalVisible bool               `json:"theoretical_visible"`
	differences        int
}

// Count es el conteo completo con sus renglones. Las banderas de acción dependen solo del estado;
// los permisos los valida Go. Warnings son avisos vigentes (renglones sin contar, consumos o
// cancelaciones pendientes, ajustes por autorizar) y los de existencia negativa al aplicarlo.
type Count struct {
	CountSummary
	Editable     bool        `json:"editable"`
	Submittable  bool        `json:"submittable"`
	Approvable   bool        `json:"approvable"`
	Applicable   bool        `json:"applicable"`
	Cancellable  bool        `json:"cancellable"`
	CostsVisible bool        `json:"costs_visible"`
	Value        *string     `json:"value"`
	ValueDisplay *string     `json:"value_display"`
	Lines        []CountLine `json:"lines"`
	Warnings     []Warning   `json:"warnings"`
}

// CountLine es un renglón del conteo. Sin captura, counted es false y los datos de captura son null.
// La existencia teórica (a la hora del renglón), la diferencia y la existencia actual del almacén
// solo con theoretical_visible: en un conteo sin aplicar se calculan al consultar; en uno aplicado
// son las que se aplicaron.
type CountLine struct {
	LineNo              int                    `json:"line_no"`
	Item                ItemReference          `json:"item"`
	CategoryName        *string                `json:"category_name"`
	Counted             bool                   `json:"counted"`
	CountedQuantity     *string                `json:"counted_quantity"`
	CapturedQuantity    *string                `json:"captured_quantity"`
	Presentation        *PresentationReference `json:"presentation"`
	Unit                *UnitReference         `json:"unit"`
	CaptureLabel        *string                `json:"capture_label"`
	CapturedAt          *time.Time             `json:"captured_at"`
	CapturedByName      *string                `json:"captured_by_name"`
	TheoreticalQuantity *string                `json:"theoretical_quantity"`
	Difference          *string                `json:"difference"`
	DifferenceKind      *string                `json:"difference_kind"`
	CurrentQuantity     *string                `json:"current_quantity"`
	MovementID          *string                `json:"movement_id"`
	MovementKind        *string                `json:"movement_kind"`
	MovementLabel       *string                `json:"movement_label"`
	Value               *string                `json:"value"`
	ValueDisplay        *string                `json:"value_display"`
}

// CountPage es una página de conteos (el más reciente primero).
type CountPage struct {
	Items    []CountSummary `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

const countColumns = `SELECT c.id,c.folio,w.id,w.name,c.scope,cat.id,cat.name,c.state,c.notes,c.line_count,
        (SELECT count(*) FROM app.inventory_count_lines l WHERE l.count_id=c.id AND l.counted_quantity IS NOT NULL)::int,
        (SELECT count(*) FROM app.inventory_count_lines l WHERE l.count_id=c.id AND l.difference<>0)::int,
        creator.display_name,c.created_at,updater.display_name,c.updated_at,submitter.display_name,c.submitted_at,
        approver.display_name,c.approved_at,applier.display_name,c.applied_at,c.reference_at,canceller.display_name,c.cancelled_at,
        c.cancel_reason,c.revision
        FROM app.inventory_counts c
        JOIN app.inventory_warehouses w ON w.id=c.warehouse_id
        LEFT JOIN app.inventory_categories cat ON cat.id=c.category_id
        JOIN app.users creator ON creator.id=c.created_by
        JOIN app.users updater ON updater.id=c.updated_by
        LEFT JOIN app.users submitter ON submitter.id=c.submitted_by
        LEFT JOIN app.users approver ON approver.id=c.approved_by
        LEFT JOIN app.users applier ON applier.id=c.applied_by
        LEFT JOIN app.users canceller ON canceller.id=c.cancelled_by`

// theoreticalVisible indica si la sesión ve la existencia teórica y las diferencias de un conteo.
func (handler *service) theoreticalVisible() bool {
	return handler.request.Actor.Can(permissionCountsApprove) || handler.request.Actor.Can(permissionStockRead)
}

func (handler *service) scanCount(row interface{ Scan(...any) error }) (*Count, error) {
	var count Count
	summary := &count.CountSummary
	var categoryID, categoryName *string
	err := row.Scan(&summary.ID, &summary.Folio, &summary.Warehouse.ID, &summary.Warehouse.Name, &summary.Scope, &categoryID,
		&categoryName, &summary.State, &summary.Notes, &summary.LineCount, &summary.CountedLines, &summary.differences,
		&summary.CreatedByName, &summary.CreatedAt, &summary.UpdatedByName, &summary.UpdatedAt, &summary.SubmittedByName,
		&summary.SubmittedAt, &summary.ApprovedByName, &summary.ApprovedAt, &summary.AppliedByName, &summary.AppliedAt,
		&summary.ReferenceAt, &summary.CancelledByName, &summary.CancelledAt, &summary.CancelReason, &summary.Revision)
	if err != nil {
		return nil, err
	}
	if categoryID != nil {
		summary.Category = &CategoryReference{ID: *categoryID, Name: *categoryName}
	}
	summary.ScopeLabel = countScopeLabels[summary.Scope]
	summary.StateLabel = countStateLabels[summary.State]
	summary.TheoreticalVisible = handler.theoreticalVisible()
	if summary.State == "applied" && summary.TheoreticalVisible {
		differences := summary.differences
		summary.DifferenceLines = &differences
	}
	count.Editable = summary.State == "open"
	count.Submittable = summary.State == "open"
	count.Approvable = summary.State == "submitted"
	count.Applicable = summary.State == "approved"
	count.Cancellable = summary.State == "open" || summary.State == "submitted" || summary.State == "approved"
	count.CostsVisible = handler.costsVisible() && summary.TheoreticalVisible
	count.Lines = []CountLine{}
	count.Warnings = []Warning{}
	return &count, nil
}

// readCountHeader lee el conteo sin renglones.
func (handler *service) readCountHeader(id string) (*Count, error) {
	count, err := handler.scanCount(handler.tx.QueryRow(handler.ctx, countColumns+" WHERE c.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("El conteo no existe.")
	}
	return count, err
}

// readCount lee el conteo con sus renglones y, si no está terminado, sus avisos vigentes.
func (handler *service) readCount(id string) (*Count, error) {
	count, err := handler.readCountHeader(id)
	if err != nil {
		return nil, err
	}
	if err := handler.attachCountLines(count); err != nil {
		return nil, err
	}
	if count.State == "open" || count.State == "submitted" || count.State == "approved" {
		warnings, err := handler.countWarnings(count)
		if err != nil {
			return nil, err
		}
		count.Warnings = warnings
	}
	return count, nil
}

func (handler *service) readCountRoute(parts []string) (any, error) {
	return handler.readCount(parts[1])
}

// attachCountLines agrega los renglones con su captura y, a quien la ve, la existencia teórica: la
// aplicada en un conteo aplicado o la calculada ahora a la hora de cada renglón en uno sin aplicar.
func (handler *service) attachCountLines(count *Count) error {
	values := map[int]decimal.Decimal{}
	if count.State == "applied" && count.CostsVisible {
		var err error
		values, err = handler.movementValues([]string{"count"}, count.ID)
		if err != nil {
			return err
		}
		count.Value, count.ValueDisplay = moneyPointers(values[0])
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT l.line_no,`+controlLineColumns+`,cat.name,l.counted_quantity::text,
            l.captured_quantity::text,l.captured_at,capturer.display_name,l.theoretical_quantity::text,l.movement_id,m.kind,
            coalesce(s.quantity,0)::text,
            CASE WHEN l.captured_at IS NOT NULL THEN (SELECT coalesce(sum(x.quantity),0) FROM app.inventory_movements x
                WHERE x.item_id=l.item_id AND x.warehouse_id=$2 AND x.occurred_at<=l.captured_at) END::text
        FROM app.inventory_count_lines l`+controlLineJoins+`
        LEFT JOIN app.inventory_categories cat ON cat.id=i.category_id
        LEFT JOIN app.users capturer ON capturer.id=l.captured_by
        LEFT JOIN app.inventory_movements m ON m.id=l.movement_id
        LEFT JOIN app.inventory_stock s ON s.item_id=l.item_id AND s.warehouse_id=$2
        WHERE l.count_id=$1 ORDER BY l.line_no`, count.ID, count.Warehouse.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	applied := count.State == "applied"
	for rows.Next() {
		var line CountLine
		var counted, captured, stored, live *string
		var current string
		targets, finish := controlLineTargets(&line.Item)
		scan := append(append([]any{&line.LineNo}, targets...), &line.CategoryName, &counted, &captured, &line.CapturedAt,
			&line.CapturedByName, &stored, &line.MovementID, &line.MovementKind, &current, &live)
		if err := rows.Scan(scan...); err != nil {
			return err
		}
		presentation, unit, label := finish()
		line.Counted = counted != nil
		if line.Counted {
			line.CountedQuantity = optionalQuantityText(parseDatabaseOptional(counted))
			line.CapturedQuantity = optionalQuantityText(parseDatabaseOptional(captured))
			line.Presentation, line.Unit, line.CaptureLabel = presentation, unit, &label
		}
		if line.MovementKind != nil {
			movementLabel := movementKindLabels[*line.MovementKind]
			line.MovementLabel = &movementLabel
		}
		if count.TheoreticalVisible {
			currentValue := parseDatabaseDecimal(current)
			line.CurrentQuantity = optionalQuantityText(&currentValue)
			theoretical := live
			if applied {
				theoretical = stored
			}
			if line.Counted && theoretical != nil {
				theoreticalValue := parseDatabaseDecimal(*theoretical)
				difference := parseDatabaseDecimal(*counted).Sub(theoreticalValue)
				line.TheoreticalQuantity = optionalQuantityText(&theoreticalValue)
				line.Difference = optionalQuantityText(&difference)
				kind := differenceKind(difference)
				line.DifferenceKind = &kind
			}
			if applied && line.MovementID != nil && count.CostsVisible {
				line.Value, line.ValueDisplay = moneyPointers(values[line.LineNo])
			}
		}
		count.Lines = append(count.Lines, line)
	}
	return rows.Err()
}

// differenceKind clasifica una diferencia: sobrante, faltante o sin diferencia.
func differenceKind(difference decimal.Decimal) string {
	switch {
	case difference.IsPositive():
		return "surplus"
	case difference.IsNegative():
		return "shortage"
	}
	return "none"
}

// countWarnings son los avisos vigentes de un conteo sin terminar: renglones sin contar, consumos de
// ventas anteriores a la hora del conteo sin calcular, cancelaciones sin clasificar y ajustes por
// autorizar del almacén que podrían contarse dos veces.
func (handler *service) countWarnings(count *Count) ([]Warning, error) {
	warnings := []Warning{}
	uncounted := count.LineCount - count.CountedLines
	if uncounted > 0 {
		warnings = append(warnings, Warning{Code: "COUNT_UNCOUNTED_LINES",
			Message: plural.Count(uncounted, "insumo no se ha contado", "insumos no se han contado") + ": no se ajustará" +
				plural.Word(uncounted, "", "n") + " al aplicar el conteo."})
	}
	var reference *time.Time
	if err := handler.tx.QueryRow(handler.ctx, "SELECT max(captured_at) FROM app.inventory_count_lines WHERE count_id=$1",
		count.ID).Scan(&reference); err != nil {
		return nil, err
	}
	if reference == nil {
		return warnings, nil
	}
	var consumption, cancellations, adjustments int
	err := handler.tx.QueryRow(handler.ctx, `SELECT
            (SELECT count(*) FROM app.inventory_consumption_requests WHERE state<>'processed' AND released_at<=$3)::int,
            (SELECT count(*) FROM app.inventory_cancellation_classifications k WHERE k.state='pending' AND k.cancelled_at<=$3
                AND EXISTS(SELECT FROM app.inventory_movements m WHERE m.restaurant_unit_id=k.restaurant_unit_id
                    AND m.warehouse_id=$2))::int,
            (SELECT count(*) FROM app.inventory_adjustments a WHERE a.state='pending_authorization' AND a.warehouse_id=$2
                AND EXISTS(SELECT FROM app.inventory_adjustment_lines al JOIN app.inventory_count_lines cl
                    ON cl.item_id=al.item_id AND cl.count_id=$1 AND cl.counted_quantity IS NOT NULL
                    WHERE al.adjustment_id=a.id))::int`, count.ID, count.Warehouse.ID, *reference).
		Scan(&consumption, &cancellations, &adjustments)
	if err != nil {
		return nil, err
	}
	if consumption > 0 {
		warnings = append(warnings, Warning{Code: "COUNT_PENDING_CONSUMPTION",
			Message: plural.Count(consumption, "consumo de ventas anterior al conteo está", "consumos de ventas anteriores al conteo están") +
				" pendiente" + plural.Word(consumption, "", "s") + " de calcular; se calculará" + plural.Word(consumption, "", "n") +
				" antes de aplicar."})
	}
	if cancellations > 0 {
		warnings = append(warnings, Warning{Code: "COUNT_PENDING_CANCELLATIONS",
			Message: plural.Count(cancellations, "cancelación sin clasificar es anterior", "cancelaciones sin clasificar son anteriores") +
				" al conteo en este almacén: clasifícala" + plural.Word(cancellations, "", "s") +
				" antes de aplicar para no contar dos veces su consumo."})
	}
	if adjustments > 0 {
		warnings = append(warnings, Warning{Code: "COUNT_PENDING_ADJUSTMENTS",
			Message: plural.Count(adjustments, "ajuste pendiente de autorización afecta", "ajustes pendientes de autorización afectan") +
				" a insumos contados en este almacén: si se autoriza" + plural.Word(adjustments, "", "n") +
				" después de aplicar el conteo, se sumará" + plural.Word(adjustments, "", "n") + " a sus diferencias."})
	}
	return warnings, nil
}

// listCounts lista conteos con filtros de almacén, estado y fecha de creación (días de negocio),
// búsqueda por folio y paginación.
func (handler *service) listCounts(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "warehouse_id", "state", "from", "until", "search", "page", "page_size"); err != nil {
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
		set.add("c.warehouse_id=$%d", *warehouseID)
	}
	if state := query.Get("state"); state != "" {
		if state == "active" {
			set.clauses = append(set.clauses, "c.state IN ('open','submitted','approved')")
		} else {
			if _, known := countStateLabels[state]; !known {
				return nil, invalid("El filtro de estado del conteo no es válido.")
			}
			set.add("c.state=$%d", state)
		}
	}
	if search := strings.TrimSpace(query.Get("search")); search != "" {
		set.add("c.folio::text=$%d", search)
	}
	from, until, err := handler.dateRange(query)
	if err != nil {
		return nil, err
	}
	if from != nil {
		set.add("c.created_at>=$%d", *from)
	}
	if until != nil {
		set.add("c.created_at<$%d", *until)
	}
	result := CountPage{Items: []CountSummary{}, Page: page, PageSize: size}
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_counts c"+set.where(), set.arguments...).
		Scan(&result.Total); err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, countColumns+set.where()+" ORDER BY c.created_at DESC, c.folio DESC LIMIT "+
		set.next(size)+" OFFSET "+set.next((page-1)*size), set.arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		count, err := handler.scanCount(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, count.CountSummary)
	}
	return result, rows.Err()
}

type countInput struct {
	OperationID string   `json:"operation_id"`
	WarehouseID string   `json:"warehouse_id"`
	Scope       string   `json:"scope"`
	CategoryID  *string  `json:"category_id"`
	ItemIDs     []string `json:"item_ids"`
	Notes       string   `json:"notes"`
}

// countItemOrder ordena los insumos de un conteo como en una hoja de conteo: por categoría (en su
// orden) y nombre.
const countItemOrder = ` ORDER BY cat.sort_order NULLS LAST, lower(cat.name) NULLS LAST, lower(i.name), i.id`

// selectedItems valida una lista de insumos (existentes, sin repetir) y la devuelve ordenada.
func (handler *service) selectedItems(identifiers []string) ([]string, error) {
	if len(identifiers) == 0 {
		return nil, invalid("Elige al menos un insumo.")
	}
	if len(identifiers) > maximumCountLines {
		return nil, invalid("Un conteo admite hasta 2000 insumos.")
	}
	normalized := make([]string, 0, len(identifiers))
	seen := map[string]bool{}
	for _, identifier := range identifiers {
		if !validID(identifier) {
			return nil, invalid("Un insumo elegido no es válido.")
		}
		lower := strings.ToLower(identifier)
		if seen[lower] {
			return nil, invalid("Un insumo aparece más de una vez en la selección.")
		}
		seen[lower] = true
		normalized = append(normalized, lower)
	}
	ordered, err := handler.orderedItems("i.id = ANY($1::uuid[])", normalized)
	if err != nil {
		return nil, err
	}
	if len(ordered) != len(normalized) {
		return nil, notFound("Un insumo elegido no existe.")
	}
	return ordered, nil
}

// orderedItems devuelve los insumos que cumplen la condición en el orden de la hoja de conteo.
func (handler *service) orderedItems(where string, arguments ...any) ([]string, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT i.id FROM app.inventory_items i
        LEFT JOIN app.inventory_categories cat ON cat.id=i.category_id WHERE `+where+countItemOrder, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	identifiers := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		identifiers = append(identifiers, id)
	}
	return identifiers, rows.Err()
}

// checkCountConflicts rechaza insumos que ya están en otro conteo sin terminar del mismo almacén: dos
// conteos simultáneos del mismo insumo ajustarían dos veces la misma diferencia.
func (handler *service) checkCountConflicts(warehouseID string, items []string, countID string) error {
	var itemName, warehouseName string
	var folio int64
	err := handler.tx.QueryRow(handler.ctx, `SELECT i.name,c.folio,w.name FROM app.inventory_count_lines l
        JOIN app.inventory_counts c ON c.id=l.count_id JOIN app.inventory_items i ON i.id=l.item_id
        JOIN app.inventory_warehouses w ON w.id=c.warehouse_id
        WHERE c.warehouse_id=$1 AND c.state = ANY($2) AND l.item_id = ANY($3::uuid[]) AND c.id<>$4::uuid
        ORDER BY c.folio, lower(i.name) LIMIT 1`, warehouseID, activeCountStates, items, countID).Scan(&itemName, &folio, &warehouseName)
	if isNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return conflict("COUNT_ITEM_IN_PROGRESS", "«"+itemName+"» ya está en el conteo "+itoa(int(folio))+" de «"+warehouseName+
		"», que no se ha aplicado ni cancelado.")
}

// insertCountLines agrega renglones sin captura después del último número de renglón.
func (handler *service) insertCountLines(countID string, items []string, after int) error {
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_count_lines(count_id,line_no,item_id)
        SELECT $1,(t.ordinal+$3)::int,t.item FROM unnest($2::uuid[]) WITH ORDINALITY AS t(item,ordinal)`, countID, items, after)
	return err
}

// createCount crea un conteo abierto con los insumos elegidos.
func (handler *service) createCount(_ []string) (any, error) {
	var input countInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	warehouse, err := handler.activeWarehouse(input.WarehouseID, "el almacén del conteo")
	if err != nil {
		return nil, err
	}
	notes, err := controlNotes(input.Notes, 1000)
	if err != nil {
		return nil, err
	}
	var categoryID *string
	var items []string
	switch input.Scope {
	case "all":
		if input.CategoryID != nil || len(input.ItemIDs) > 0 {
			return nil, invalid("Un conteo de todos los insumos no lleva categoría ni lista de insumos.")
		}
		items, err = handler.orderedItems("i.active")
	case "category":
		if len(input.ItemIDs) > 0 {
			return nil, invalid("Un conteo por categoría no lleva lista de insumos.")
		}
		categoryID, err = validateOptionalID(input.CategoryID)
		if err != nil || categoryID == nil {
			return nil, invalid("Elige la categoría del conteo.")
		}
		if _, err := handler.readCategory(*categoryID); err != nil {
			return nil, err
		}
		items, err = handler.orderedItems("i.active AND i.category_id=$1", *categoryID)
	case "items":
		if input.CategoryID != nil {
			return nil, invalid("Un conteo de insumos seleccionados no lleva categoría.")
		}
		items, err = handler.selectedItems(input.ItemIDs)
	default:
		return nil, invalid("El alcance del conteo debe ser todos, una categoría o insumos seleccionados.")
	}
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, conflict("COUNT_EMPTY", "No hay insumos activos que contar con esa selección.")
	}
	if len(items) > maximumCountLines {
		return nil, invalid("Un conteo admite hasta 2000 insumos: divide la selección por categorías.")
	}
	if err := handler.checkCountConflicts(warehouse.ID, items, "00000000-0000-0000-0000-000000000000"); err != nil {
		return nil, err
	}
	id := newID()
	actor := handler.request.Actor.UserID
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_counts(id,warehouse_id,scope,category_id,notes,line_count,created_by,
            updated_by,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8)`, id, warehouse.ID, input.Scope, categoryID, notes, len(items), actor,
		handler.operationReference())
	if err != nil {
		return nil, err
	}
	if err := handler.insertCountLines(id, items, 0); err != nil {
		return nil, err
	}
	saved, err := handler.readCount(id)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.counts.changed", &id); err != nil {
		return nil, err
	}
	after := map[string]any{"folio": saved.Folio, "warehouse_id": warehouse.ID, "warehouse_name": warehouse.Name, "scope": saved.Scope,
		"category_id": categoryID, "notes": notes, "line_count": saved.LineCount, "item_ids": items}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.count_created", "inventory_count", &id, nil, after)
}

// lockCount bloquea el conteo y valida su revisión.
func (handler *service) lockCount(id string, revision int) (*Count, error) {
	if _, err := handler.tx.Exec(handler.ctx, "SELECT 1 FROM app.inventory_counts WHERE id=$1 FOR UPDATE", id); err != nil {
		return nil, err
	}
	count, err := handler.readCountHeader(id)
	if err != nil {
		return nil, err
	}
	if count.Revision != revision {
		return nil, revisionConflict("El conteo")
	}
	return count, nil
}

// requireCountState exige un estado de origen.
func requireCountState(count *Count, state, code string) error {
	if count.State != state {
		return conflict(code, "El conteo está «"+count.StateLabel+"»: no admite esta acción.")
	}
	return nil
}

type countUpdateInput struct {
	OperationID   string   `json:"operation_id"`
	Revision      int      `json:"revision"`
	Notes         *string  `json:"notes"`
	AddItemIDs    []string `json:"add_item_ids"`
	RemoveItemIDs []string `json:"remove_item_ids"`
}

// updateCount cambia las observaciones y agrega o quita insumos de un conteo abierto.
func (handler *service) updateCount(parts []string) (any, error) {
	var input countUpdateInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	previous, err := handler.lockCount(parts[1], input.Revision)
	if err != nil {
		return nil, err
	}
	if err := requireCountState(previous, "open", "COUNT_NOT_OPEN"); err != nil {
		return nil, err
	}
	notes := previous.Notes
	if input.Notes != nil {
		if notes, err = controlNotes(*input.Notes, 1000); err != nil {
			return nil, err
		}
	}
	present := map[string]bool{}
	last := 0
	rows, err := handler.tx.Query(handler.ctx, "SELECT item_id,line_no FROM app.inventory_count_lines WHERE count_id=$1", previous.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var itemID string
		var lineNo int
		if err := rows.Scan(&itemID, &lineNo); err != nil {
			rows.Close()
			return nil, err
		}
		present[itemID] = true
		last = max(last, lineNo)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	removed := []string{}
	for _, identifier := range input.RemoveItemIDs {
		lower := strings.ToLower(identifier)
		if !validID(lower) || !present[lower] {
			return nil, invalid("Un insumo que quieres quitar no forma parte del conteo.")
		}
		removed = append(removed, lower)
		delete(present, lower)
	}
	added := []string{}
	if len(input.AddItemIDs) > 0 {
		added, err = handler.selectedItems(input.AddItemIDs)
		if err != nil {
			return nil, err
		}
		for _, identifier := range added {
			if present[identifier] {
				return nil, invalid("Un insumo que quieres agregar ya forma parte del conteo.")
			}
			present[identifier] = true
		}
		if err := handler.checkCountConflicts(previous.Warehouse.ID, added, previous.ID); err != nil {
			return nil, err
		}
	}
	if len(present) == 0 {
		return nil, invalid("El conteo debe conservar al menos un insumo.")
	}
	if len(present) > maximumCountLines {
		return nil, invalid("Un conteo admite hasta 2000 insumos.")
	}
	if len(removed) > 0 {
		if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.inventory_count_lines WHERE count_id=$1 AND item_id = ANY($2::uuid[])",
			previous.ID, removed); err != nil {
			return nil, err
		}
	}
	if len(added) > 0 {
		if err := handler.insertCountLines(previous.ID, added, last); err != nil {
			return nil, err
		}
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_counts SET notes=$2,line_count=$3,updated_by=$4,updated_at=now(),
            revision=revision+1 WHERE id=$1`, previous.ID, notes, len(present), handler.request.Actor.UserID)
	if err != nil {
		return nil, err
	}
	saved, err := handler.readCount(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.counts.changed", &saved.ID); err != nil {
		return nil, err
	}
	before := map[string]any{"notes": previous.Notes, "line_count": previous.LineCount, "revision": previous.Revision}
	after := map[string]any{"notes": saved.Notes, "line_count": saved.LineCount, "revision": saved.Revision, "added_item_ids": added,
		"removed_item_ids": removed}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.count_updated", "inventory_count", &saved.ID, before, after)
}

type countCaptureInput struct {
	OperationID string             `json:"operation_id"`
	Lines       []countCaptureLine `json:"lines"`
}

// countCaptureLine es la captura de un insumo del conteo: la cantidad física (cero admitido) en una
// presentación, en una unidad de su dimensión o en su unidad base; null borra la captura.
type countCaptureLine struct {
	ItemID          string  `json:"item_id"`
	CountedQuantity *string `json:"counted_quantity"`
	PresentationID  *string `json:"presentation_id"`
	UnitID          *string `json:"unit_id"`
}

// theoreticalAt es la existencia teórica de un insumo en un almacén a un instante: la suma de los
// movimientos del libro con occurred_at ≤ instante.
func (handler *service) theoreticalAt(itemID, warehouseID string, instant time.Time) (decimal.Decimal, error) {
	var total string
	err := handler.tx.QueryRow(handler.ctx, `SELECT coalesce(sum(quantity),0)::text FROM app.inventory_movements
        WHERE item_id=$1 AND warehouse_id=$2 AND occurred_at<=$3`, itemID, warehouseID, instant).Scan(&total)
	if err != nil {
		return zero, err
	}
	return parseDatabaseDecimal(total), nil
}

// captureCountLines guarda en una transacción corta las cantidades contadas de uno o más insumos del
// conteo abierto, con la hora de la captura. No exige revisión: varias terminales pueden capturar a
// la vez insumos distintos (la última captura de un insumo prevalece).
func (handler *service) captureCountLines(parts []string) (any, error) {
	var input countCaptureInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if len(input.Lines) == 0 || len(input.Lines) > maximumControlLines {
		return nil, invalid("Captura de 1 a 200 insumos por guardado.")
	}
	if _, err := handler.tx.Exec(handler.ctx, "SELECT 1 FROM app.inventory_counts WHERE id=$1 FOR UPDATE", parts[1]); err != nil {
		return nil, err
	}
	count, err := handler.readCountHeader(parts[1])
	if err != nil {
		return nil, err
	}
	if err := requireCountState(count, "open", "COUNT_NOT_OPEN"); err != nil {
		return nil, err
	}
	lineNumbers := map[string]int{}
	rows, err := handler.tx.Query(handler.ctx, "SELECT item_id,line_no FROM app.inventory_count_lines WHERE count_id=$1", count.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var itemID string
		var lineNo int
		if err := rows.Scan(&itemID, &lineNo); err != nil {
			rows.Close()
			return nil, err
		}
		lineNumbers[itemID] = lineNo
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	now, err := handler.transactionTime()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	audited := []map[string]any{}
	for index, line := range input.Lines {
		prefix := "Renglón " + itoa(index+1) + ": "
		lower := strings.ToLower(line.ItemID)
		lineNo, found := lineNumbers[lower]
		if !validID(lower) || !found {
			return nil, invalid(prefix + "el insumo no forma parte del conteo.")
		}
		if seen[lower] {
			return nil, invalid(prefix + "el insumo aparece más de una vez en el guardado.")
		}
		seen[lower] = true
		if line.CountedQuantity == nil {
			if line.PresentationID != nil || line.UnitID != nil {
				return nil, invalid(prefix + "para borrar la captura no envíes unidad ni presentación.")
			}
			_, err := handler.tx.Exec(handler.ctx, `UPDATE app.inventory_count_lines SET counted_quantity=NULL,captured_quantity=NULL,
                    presentation_id=NULL,unit_id=NULL,captured_at=NULL,captured_by=NULL,theoretical_quantity=NULL,difference=NULL
                WHERE count_id=$1 AND line_no=$2`, count.ID, lineNo)
			if err != nil {
				return nil, err
			}
			audited = append(audited, map[string]any{"item_id": lower, "counted_quantity": nil})
			continue
		}
		capture, err := handler.controlCapture(lower, line.PresentationID, line.UnitID, *line.CountedQuantity, prefix, true, false)
		if err != nil {
			return nil, err
		}
		theoretical, err := handler.theoreticalAt(capture.item.ID, count.Warehouse.ID, now)
		if err != nil {
			return nil, err
		}
		presentationID, unitID := captureColumns(capture)
		_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_count_lines SET counted_quantity=$3::numeric,
                captured_quantity=$4::numeric,presentation_id=$5,unit_id=$6,captured_at=$7,captured_by=$8,
                theoretical_quantity=$9::numeric,difference=$10::numeric
            WHERE count_id=$1 AND line_no=$2`, count.ID, lineNo, capture.base.String(), capture.quantity.String(), presentationID, unitID,
			now, handler.request.Actor.UserID, theoretical.String(), capture.base.Sub(theoretical).String())
		if err != nil {
			return nil, err
		}
		audited = append(audited, map[string]any{"item_id": capture.item.ID, "item_name": capture.item.Name,
			"counted_quantity": quantityText(capture.base), "captured_quantity": quantityText(capture.quantity),
			"capture_label": capture.label})
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_counts SET updated_by=$2,updated_at=now(),revision=revision+1
        WHERE id=$1`, count.ID, handler.request.Actor.UserID)
	if err != nil {
		return nil, err
	}
	saved, err := handler.readCount(count.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.counts.changed", &saved.ID); err != nil {
		return nil, err
	}
	after := map[string]any{"captured_at": now, "revision": saved.Revision, "lines": audited}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.count_lines_captured", "inventory_count", &saved.ID, nil, after)
}

type countTransitionInput struct {
	OperationID string `json:"operation_id"`
	Revision    int    `json:"revision"`
	Reason      string `json:"reason"`
}

// countTransition aplica un cambio de estado simple (enviar o aprobar) con revisión y auditoría.
func (handler *service) countTransition(parts []string, from, code, update, action string, prepare func(*Count) error) (any, error) {
	var input countTransitionInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Reason) != "" {
		return nil, invalid("Esta acción no lleva motivo.")
	}
	previous, err := handler.lockCount(parts[1], input.Revision)
	if err != nil {
		return nil, err
	}
	if err := requireCountState(previous, from, code); err != nil {
		return nil, err
	}
	if prepare != nil {
		if err := prepare(previous); err != nil {
			return nil, err
		}
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_counts SET `+update+`,updated_by=$2,updated_at=now(),
            revision=revision+1 WHERE id=$1`, previous.ID, handler.request.Actor.UserID)
	if err != nil {
		return nil, err
	}
	saved, err := handler.readCount(previous.ID)
	if err != nil {
		return nil, err
	}
	for _, kind := range []string{"inventory.counts.changed", "inventory.alerts.changed"} {
		if err := handler.publish(kind, &saved.ID); err != nil {
			return nil, err
		}
	}
	before := map[string]any{"state": previous.State, "revision": previous.Revision}
	after := map[string]any{"state": saved.State, "revision": saved.Revision}
	return saved, handler.audit(handler.ctx, handler.tx, action, "inventory_count", &saved.ID, before, after)
}

// submitCount envía a revisión un conteo abierto con al menos un insumo contado.
func (handler *service) submitCount(parts []string) (any, error) {
	return handler.countTransition(parts, "open", "COUNT_NOT_OPEN", "state='submitted',submitted_by=$2,submitted_at=now()",
		"inventory.count_submitted", func(count *Count) error {
			if count.CountedLines == 0 {
				return conflict("COUNT_NOTHING_COUNTED", "Captura al menos un insumo antes de enviar el conteo a revisión.")
			}
			return nil
		})
}

// approveCount aprueba un conteo enviado (inventory.counts.approve).
func (handler *service) approveCount(parts []string) (any, error) {
	return handler.countTransition(parts, "submitted", "COUNT_NOT_SUBMITTED", "state='approved',approved_by=$2,approved_at=now()",
		"inventory.count_approved", nil)
}

// cancelCount cancela con motivo un conteo sin terminar. Uno abierto lo cancela quien cuenta o
// quien aprueba; uno enviado o aprobado, solo quien aprueba.
func (handler *service) cancelCount(parts []string) (any, error) {
	var input countTransitionInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	reason, err := purchaseReason(input.Reason, "El motivo de la cancelación")
	if err != nil {
		return nil, err
	}
	previous, err := handler.lockCount(parts[1], input.Revision)
	if err != nil {
		return nil, err
	}
	if !previous.Cancellable {
		return nil, conflict("COUNT_NOT_CANCELLABLE", "El conteo está «"+previous.StateLabel+"»: ya no se cancela.")
	}
	if previous.State != "open" && !handler.request.Actor.Can(permissionCountsApprove) {
		return nil, denied()
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_counts SET state='cancelled',cancelled_by=$2,cancelled_at=now(),
            cancel_reason=$3,updated_by=$2,updated_at=now(),revision=revision+1 WHERE id=$1`, previous.ID, handler.request.Actor.UserID,
		reason)
	if err != nil {
		return nil, err
	}
	saved, err := handler.readCount(previous.ID)
	if err != nil {
		return nil, err
	}
	for _, kind := range []string{"inventory.counts.changed", "inventory.alerts.changed"} {
		if err := handler.publish(kind, &saved.ID); err != nil {
			return nil, err
		}
	}
	before := map[string]any{"state": previous.State, "revision": previous.Revision}
	after := map[string]any{"state": saved.State, "revision": saved.Revision, "reason": reason}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.count_cancelled", "inventory_count", &saved.ID, before, after)
}

// countedLine es un renglón contado que se aplica.
type countedLine struct {
	lineNo     int
	itemID     string
	counted    decimal.Decimal
	capturedAt time.Time
}

// applyCount aplica un conteo aprobado: procesa los consumos pendientes anteriores a su hora de
// referencia y registra un count_in o count_out por cada renglón contado con diferencia.
func (handler *service) applyCount(parts []string) (any, error) {
	var input countTransitionInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Reason) != "" {
		return nil, invalid("Aplicar un conteo no lleva motivo.")
	}
	previous, err := handler.lockCount(parts[1], input.Revision)
	if err != nil {
		return nil, err
	}
	if previous.State == "applied" {
		return nil, conflict("COUNT_ALREADY_APPLIED", "El conteo ya se aplicó: aplicarlo otra vez no genera otro ajuste.")
	}
	if err := requireCountState(previous, "approved", "COUNT_NOT_APPROVED"); err != nil {
		return nil, err
	}
	lines, err := handler.countedLines(previous.ID)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, conflict("COUNT_NOTHING_COUNTED", "El conteo no tiene insumos contados.")
	}
	reference := lines[0].capturedAt
	for _, line := range lines {
		if line.capturedAt.After(reference) {
			reference = line.capturedAt
		}
	}
	processed, err := processConsumptionThrough(handler.ctx, handler.tx, reference)
	if err != nil {
		return nil, err
	}
	warehouse, err := handler.readWarehouse(previous.Warehouse.ID)
	if err != nil {
		return nil, err
	}
	warnings := []Warning{}
	movements := 0
	reason := "Conteo físico " + itoa(int(previous.Folio))
	audited := []map[string]any{}
	for _, line := range lines {
		theoretical, err := handler.theoreticalAt(line.itemID, warehouse.ID, line.capturedAt)
		if err != nil {
			return nil, err
		}
		difference := line.counted.Sub(theoretical)
		var movementID *string
		if !difference.IsZero() {
			kind := KindCountIn
			if difference.IsNegative() {
				kind = KindCountOut
			}
			number := line.lineNo
			applied, err := Apply(handler.ctx, handler.tx, MovementInput{
				EntryKey: "count:" + previous.ID + ":" + line.itemID, Kind: kind, ItemID: line.itemID, WarehouseID: warehouse.ID,
				Quantity: difference.Abs(), OccurredAt: line.capturedAt, SourceKind: "count", SourceID: &previous.ID,
				SourceLine: &number, Reason: &reason, ActorID: handler.actorReference(), OperationID: handler.operationReference(),
			})
			if err != nil {
				return nil, err
			}
			movementID = &applied.Movement.ID
			movements++
			if applied.NegativeStock {
				item, err := handler.itemReference(line.itemID)
				if err != nil {
					return nil, err
				}
				warnings = append(warnings, negativeStockWarning(*item, warehouse.Name))
			}
		}
		_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_count_lines SET theoretical_quantity=$3::numeric,
                difference=$4::numeric,movement_id=$5 WHERE count_id=$1 AND line_no=$2`, previous.ID, line.lineNo, theoretical.String(),
			difference.String(), movementID)
		if err != nil {
			return nil, err
		}
		audited = append(audited, map[string]any{"line": line.lineNo, "item_id": line.itemID, "counted_quantity": quantityText(line.counted),
			"captured_at": line.capturedAt, "theoretical_quantity": quantityText(theoretical), "difference": quantityText(difference),
			"movement_id": movementID})
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_counts SET state='applied',applied_by=$2,applied_at=now(),reference_at=$3,
            updated_by=$2,updated_at=now(),revision=revision+1 WHERE id=$1`, previous.ID, handler.request.Actor.UserID, reference)
	if err != nil {
		return nil, err
	}
	saved, err := handler.readCount(previous.ID)
	if err != nil {
		return nil, err
	}
	saved.Warnings = warnings
	// Los eventos se publican al final (lock del outbox): primero los de los consumos procesados.
	for _, entry := range processed {
		if err := publishRequestOutcome(handler.ctx, handler.tx, entry.request, entry.outcome); err != nil {
			return nil, err
		}
	}
	kinds := []string{"inventory.counts.changed", "inventory.alerts.changed"}
	if movements > 0 {
		kinds = append(kinds, "inventory.stock.changed", "inventory.movements.recorded")
	}
	for _, kind := range kinds {
		aggregate := saved.ID
		if kind == "inventory.stock.changed" {
			aggregate = warehouse.ID
		}
		if err := handler.publish(kind, &aggregate); err != nil {
			return nil, err
		}
	}
	before := map[string]any{"state": previous.State, "revision": previous.Revision}
	after := map[string]any{"state": saved.State, "revision": saved.Revision, "reference_at": reference,
		"processed_consumption_requests": len(processed), "movements": movements, "lines": audited}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.count_applied", "inventory_count", &saved.ID, before, after)
}

// countedLines lee los renglones contados de un conteo en orden.
func (handler *service) countedLines(countID string) ([]countedLine, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT line_no,item_id,counted_quantity::text,captured_at FROM app.inventory_count_lines
        WHERE count_id=$1 AND counted_quantity IS NOT NULL ORDER BY line_no`, countID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []countedLine{}
	for rows.Next() {
		var line countedLine
		var counted string
		if err := rows.Scan(&line.lineNo, &line.itemID, &counted, &line.capturedAt); err != nil {
			return nil, err
		}
		line.counted = parseDatabaseDecimal(counted)
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

// processedRequest es una solicitud de consumo procesada dentro de la aplicación de un conteo, con
// lo que produjo (para publicar sus eventos al final).
type processedRequest struct {
	request *consumptionRequest
	outcome requestOutcome
}

// processConsumptionThrough procesa en la transacción del llamador, que tiene el lock 7820148308
// exclusivo (el trabajador no puede estar a la mitad de una solicitud), las solicitudes de consumo
// sin procesar liberadas hasta reference, en orden de seq, con el mismo procesador del trabajador.
// Si una falla, revierte su intento y rechaza la operación: aplicar el conteo sin ese consumo
// contaría dos veces la diferencia cuando el trabajador lo registre después.
func processConsumptionThrough(ctx context.Context, tx pgx.Tx, reference time.Time) ([]processedRequest, error) {
	rows, err := tx.Query(ctx, `SELECT r.id,r.kind,r.restaurant_unit_id,r.source_id,r.version_id,r.released_at,r.occurred_at,
            r.service_mode,r.state,r.attempts
        FROM app.inventory_consumption_requests r
        WHERE r.state<>'processed' AND r.released_at<=$1 ORDER BY r.seq FOR UPDATE`, reference)
	if err != nil {
		return nil, err
	}
	pending := []*consumptionRequest{}
	for rows.Next() {
		var request consumptionRequest
		if err := rows.Scan(&request.ID, &request.Kind, &request.UnitID, &request.SourceID, &request.VersionID, &request.ReleasedAt,
			&request.OccurredAt, &request.ServiceMode, &request.State, &request.Attempts); err != nil {
			rows.Close()
			return nil, err
		}
		pending = append(pending, &request)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	processed := make([]processedRequest, 0, len(pending))
	for _, request := range pending {
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			return nil, err
		}
		outcome, err := applyRequest(ctx, savepoint, request)
		if err != nil {
			_ = savepoint.Rollback(ctx)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, conflict("COUNT_CONSUMPTION_PENDING", "Hay consumos de ventas anteriores al conteo que no se pudieron calcular: "+
				"revisa las solicitudes de consumo y vuelve a aplicar el conteo.")
		}
		if err := savepoint.Commit(ctx); err != nil {
			return nil, err
		}
		processed = append(processed, processedRequest{request: request, outcome: outcome})
	}
	return processed, nil
}
