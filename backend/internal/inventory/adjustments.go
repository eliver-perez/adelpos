package inventory

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Ajustes de inventario y mermas operativas (§22 de la especificación). Reglas:
//   - un ajuste de discrepancia (inventory.adjustments.record) corrige existencias con un motivo de
//     lista cerrada; quien además tiene inventory.adjustments.authorize lo aplica al registrarlo; si
//     no, queda pendiente de autorización hasta que otra persona con ese permiso lo autorice o lo
//     rechace (nadie autoriza ni rechaza un ajuste que registró);
//   - una merma operativa (inventory.waste.record) se aplica directamente y solo registra salidas
//     (movimiento waste); un ajuste negativo nunca se convierte en merma;
//   - los renglones positivos son entradas sin costo propio (adjustment_in, regla 3 del motor) y los
//     negativos salidas (adjustment_out o waste, regla 1); ninguno edita existencias directamente;
//   - pueden dejar existencias negativas: se avisa (NEGATIVE_STOCK) y no se bloquea;
//   - los movimientos llevan como hora de negocio la de su aplicación (al registrarlo o al
//     autorizarlo): un ajuste autorizado después de un conteo no altera la base de ese conteo.

const (
	adjustmentDiscrepancy = "discrepancy"
	adjustmentWaste       = "waste"
)

var adjustmentKindLabels = map[string]string{
	adjustmentDiscrepancy: "Ajuste de discrepancia",
	adjustmentWaste:       "Merma operativa",
}

var adjustmentReasonLabels = map[string]string{
	"breakage":          "Rotura",
	"spill":             "Derrame",
	"expiry":            "Caducidad",
	"physical_shortage": "Faltante físico",
	"surplus":           "Sobrante",
	"entry_error":       "Error de registro",
	"extraordinary_use": "Consumo extraordinario",
	"other":             "Otro",
}

// wasteReasons son los motivos que admite una merma operativa (una pérdida).
var wasteReasons = map[string]bool{"breakage": true, "spill": true, "expiry": true, "extraordinary_use": true, "other": true}

// adjustmentReasonDirection es el sentido que exige cada motivo: -1 solo salidas, 1 solo entradas
// y 0 ambos (un error de registro u otro motivo puede corregir en los dos sentidos).
var adjustmentReasonDirection = map[string]int{
	"breakage": -1, "spill": -1, "expiry": -1, "physical_shortage": -1, "surplus": 1, "entry_error": 0,
	"extraordinary_use": -1, "other": 0,
}

var adjustmentStateLabels = map[string]string{
	"pending_authorization": "Pendiente de autorización",
	"applied":               "Aplicado",
	"rejected":              "Rechazado",
}

// AdjustmentSummary es un ajuste o una merma en listados. Value (con inventory.costs.read) es el
// valor neto de sus movimientos: negativo si salió más de lo que entró.
type AdjustmentSummary struct {
	ID               string             `json:"id"`
	Folio            int64              `json:"folio"`
	Kind             string             `json:"kind"`
	KindLabel        string             `json:"kind_label"`
	Reason           string             `json:"reason"`
	ReasonLabel      string             `json:"reason_label"`
	ReasonDetail     string             `json:"reason_detail"`
	Warehouse        WarehouseReference `json:"warehouse"`
	State            string             `json:"state"`
	StateLabel       string             `json:"state_label"`
	Notes            string             `json:"notes"`
	LineCount        int                `json:"line_count"`
	RequestedByName  string             `json:"requested_by_name"`
	RequestedAt      time.Time          `json:"requested_at"`
	AuthorizedByName *string            `json:"authorized_by_name"`
	AuthorizedAt     *time.Time         `json:"authorized_at"`
	AppliedAt        *time.Time         `json:"applied_at"`
	RejectedByName   *string            `json:"rejected_by_name"`
	RejectedAt       *time.Time         `json:"rejected_at"`
	RejectionReason  *string            `json:"rejection_reason"`
	Revision         int                `json:"revision"`
	UpdatedAt        time.Time          `json:"updated_at"`
	CostsVisible     bool               `json:"costs_visible"`
	Value            *string            `json:"value"`
	ValueDisplay     *string            `json:"value_display"`
	requestedBy      string
}

// Adjustment es el ajuste completo. Authorizable indica que la sesión puede autorizarlo o
// rechazarlo (pendiente, con el permiso y registrado por otra persona). Warnings solo viaja en la
// respuesta de la escritura que lo aplicó.
type Adjustment struct {
	AdjustmentSummary
	Authorizable bool             `json:"authorizable"`
	Lines        []AdjustmentLine `json:"lines"`
	Warnings     []Warning        `json:"warnings"`
}

// AdjustmentLine es un renglón con su cantidad con signo (capturada y en la unidad base) y, si ya se
// aplicó, su movimiento y la existencia del almacén después de él.
type AdjustmentLine struct {
	LineNo           int                    `json:"line_no"`
	Item             ItemReference          `json:"item"`
	Presentation     *PresentationReference `json:"presentation"`
	Unit             *UnitReference         `json:"unit"`
	CaptureLabel     string                 `json:"capture_label"`
	CapturedQuantity string                 `json:"captured_quantity"`
	Quantity         string                 `json:"quantity"`
	Direction        string                 `json:"direction"`
	MovementID       *string                `json:"movement_id"`
	MovementKind     *string                `json:"movement_kind"`
	MovementLabel    *string                `json:"movement_label"`
	StockAfter       *string                `json:"stock_after"`
	Value            *string                `json:"value"`
	ValueDisplay     *string                `json:"value_display"`
}

// AdjustmentPage es una página de ajustes (el más reciente primero).
type AdjustmentPage struct {
	Items        []AdjustmentSummary `json:"items"`
	Total        int                 `json:"total"`
	Page         int                 `json:"page"`
	PageSize     int                 `json:"page_size"`
	CostsVisible bool                `json:"costs_visible"`
}

type adjustmentInput struct {
	OperationID  string             `json:"operation_id"`
	Kind         string             `json:"kind"`
	Reason       string             `json:"reason"`
	ReasonDetail string             `json:"reason_detail"`
	WarehouseID  string             `json:"warehouse_id"`
	Notes        string             `json:"notes"`
	Lines        []controlLineInput `json:"lines"`
}

// preparedAdjustmentLine es un renglón validado con su cantidad base con signo.
type preparedAdjustmentLine struct {
	capture purchaseCapture
}

const adjustmentColumns = `SELECT a.id,a.folio,a.kind,a.reason,a.reason_detail,w.id,w.name,a.state,a.notes,a.line_count,a.requested_by,
        requester.display_name,a.requested_at,authorizer.display_name,a.authorized_at,a.applied_at,rejecter.display_name,a.rejected_at,
        a.rejection_reason,a.revision,a.updated_at
        FROM app.inventory_adjustments a
        JOIN app.inventory_warehouses w ON w.id=a.warehouse_id
        JOIN app.users requester ON requester.id=a.requested_by
        LEFT JOIN app.users authorizer ON authorizer.id=a.authorized_by
        LEFT JOIN app.users rejecter ON rejecter.id=a.rejected_by`

func (handler *service) scanAdjustment(row interface{ Scan(...any) error }) (*Adjustment, error) {
	var adjustment Adjustment
	summary := &adjustment.AdjustmentSummary
	err := row.Scan(&summary.ID, &summary.Folio, &summary.Kind, &summary.Reason, &summary.ReasonDetail, &summary.Warehouse.ID,
		&summary.Warehouse.Name, &summary.State, &summary.Notes, &summary.LineCount, &summary.requestedBy, &summary.RequestedByName,
		&summary.RequestedAt, &summary.AuthorizedByName, &summary.AuthorizedAt, &summary.AppliedAt, &summary.RejectedByName,
		&summary.RejectedAt, &summary.RejectionReason, &summary.Revision, &summary.UpdatedAt)
	if err != nil {
		return nil, err
	}
	summary.KindLabel = adjustmentKindLabels[summary.Kind]
	summary.ReasonLabel = adjustmentReasonLabels[summary.Reason]
	summary.StateLabel = adjustmentStateLabels[summary.State]
	summary.CostsVisible = handler.costsVisible()
	adjustment.Authorizable = summary.State == "pending_authorization" && handler.request.Actor.Can(permissionAdjustmentsAuthorize) &&
		summary.requestedBy != handler.request.Actor.UserID
	adjustment.Lines = []AdjustmentLine{}
	adjustment.Warnings = []Warning{}
	return &adjustment, nil
}

// adjustmentSources son los orígenes del libro de un ajuste (discrepancia) y de una merma.
var adjustmentSources = []string{"adjustment", "waste"}

// readAdjustment lee el ajuste con sus renglones y, con costos, sus valores.
func (handler *service) readAdjustment(id string) (*Adjustment, error) {
	adjustment, err := handler.scanAdjustment(handler.tx.QueryRow(handler.ctx, adjustmentColumns+" WHERE a.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("El ajuste de inventario no existe.")
	}
	if err != nil {
		return nil, err
	}
	values, err := handler.movementValues(adjustmentSources, adjustment.ID)
	if err != nil {
		return nil, err
	}
	if adjustment.State == "applied" {
		adjustment.Value, adjustment.ValueDisplay = handler.optionalMoneyPointers(values[0])
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT l.line_no,`+controlLineColumns+`,l.captured_quantity::text,l.quantity::text,
            l.movement_id,m.kind,m.stock_after::text
        FROM app.inventory_adjustment_lines l`+controlLineJoins+`
        LEFT JOIN app.inventory_movements m ON m.id=l.movement_id
        WHERE l.adjustment_id=$1 ORDER BY l.line_no`, adjustment.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var line AdjustmentLine
		var captured, quantity string
		var stockAfter *string
		targets, finish := controlLineTargets(&line.Item)
		if err := rows.Scan(append(append([]any{&line.LineNo}, targets...), &captured, &quantity, &line.MovementID,
			&line.MovementKind, &stockAfter)...); err != nil {
			return nil, err
		}
		line.Presentation, line.Unit, line.CaptureLabel = finish()
		base := parseDatabaseDecimal(quantity)
		line.CapturedQuantity = signedQuantityText(parseDatabaseDecimal(captured))
		line.Quantity = signedQuantityText(base)
		line.Direction = "in"
		if base.IsNegative() {
			line.Direction = "out"
		}
		if line.MovementKind != nil {
			label := movementKindLabels[*line.MovementKind]
			line.MovementLabel = &label
			line.StockAfter = optionalQuantityText(parseDatabaseOptional(stockAfter))
			line.Value, line.ValueDisplay = handler.optionalMoneyPointers(values[line.LineNo])
		}
		adjustment.Lines = append(adjustment.Lines, line)
	}
	return adjustment, rows.Err()
}

func (handler *service) readAdjustmentRoute(parts []string) (any, error) {
	return handler.readAdjustment(parts[1])
}

// listAdjustments lista ajustes y mermas con filtros de tipo, estado, motivo, almacén, insumo y
// fecha de registro (días de negocio), búsqueda por folio y paginación.
func (handler *service) listAdjustments(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "kind", "state", "reason", "warehouse_id", "item_id", "from", "until", "search", "page",
		"page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	for key, labels := range map[string]map[string]string{
		"kind": adjustmentKindLabels, "state": adjustmentStateLabels, "reason": adjustmentReasonLabels,
	} {
		value := query.Get(key)
		if value == "" {
			continue
		}
		if _, known := labels[value]; !known {
			return nil, invalid("El filtro " + key + " del ajuste no es válido.")
		}
		set.add("a."+key+"=$%d", value)
	}
	warehouseID, err := optionalQueryID(query, "warehouse_id", "almacén")
	if err != nil {
		return nil, err
	}
	if warehouseID != nil {
		set.add("a.warehouse_id=$%d", *warehouseID)
	}
	itemID, err := optionalQueryID(query, "item_id", "insumo")
	if err != nil {
		return nil, err
	}
	if itemID != nil {
		set.add("EXISTS(SELECT FROM app.inventory_adjustment_lines l WHERE l.adjustment_id=a.id AND l.item_id=$%d)", *itemID)
	}
	if search := strings.TrimSpace(query.Get("search")); search != "" {
		set.add("a.folio::text=$%d", search)
	}
	from, until, err := handler.dateRange(query)
	if err != nil {
		return nil, err
	}
	if from != nil {
		set.add("a.requested_at>=$%d", *from)
	}
	if until != nil {
		set.add("a.requested_at<$%d", *until)
	}
	result := AdjustmentPage{Items: []AdjustmentSummary{}, Page: page, PageSize: size, CostsVisible: handler.costsVisible()}
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_adjustments a"+set.where(), set.arguments...).
		Scan(&result.Total); err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, adjustmentColumns+set.where()+" ORDER BY a.requested_at DESC, a.folio DESC LIMIT "+
		set.next(size)+" OFFSET "+set.next((page-1)*size), set.arguments...)
	if err != nil {
		return nil, err
	}
	adjustments := []*Adjustment{}
	for rows.Next() {
		adjustment, err := handler.scanAdjustment(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		adjustments = append(adjustments, adjustment)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, adjustment := range adjustments {
		if adjustment.State == "applied" && handler.costsVisible() {
			values, err := handler.movementValues(adjustmentSources, adjustment.ID)
			if err != nil {
				return nil, err
			}
			adjustment.Value, adjustment.ValueDisplay = moneyPointers(values[0])
		}
		result.Items = append(result.Items, adjustment.AdjustmentSummary)
	}
	return result, nil
}

// validateAdjustmentReason valida tipo, motivo y texto del motivo. El permiso depende del tipo y se
// comprueba aquí, antes de cualquier escritura.
func (handler *service) validateAdjustmentReason(input *adjustmentInput) error {
	switch input.Kind {
	case adjustmentDiscrepancy:
		if err := handler.request.Actor.Require(permissionAdjustmentsRecord); err != nil {
			return err
		}
	case adjustmentWaste:
		if err := handler.request.Actor.Require(permissionWasteRecord); err != nil {
			return err
		}
	default:
		return invalid("El tipo de ajuste debe ser discrepancia o merma.")
	}
	if _, known := adjustmentReasonLabels[input.Reason]; !known {
		return invalid("Elige el motivo del ajuste.")
	}
	if input.Kind == adjustmentWaste && !wasteReasons[input.Reason] {
		return invalid("Una merma solo admite rotura, derrame, caducidad, consumo extraordinario u otro motivo.")
	}
	input.ReasonDetail = strings.TrimSpace(input.ReasonDetail)
	if textLength(input.ReasonDetail) > 200 || hasControlCharacters(input.ReasonDetail) {
		return invalid("El detalle del motivo admite hasta 200 caracteres en una sola línea.")
	}
	if input.Reason == "other" && textLength(input.ReasonDetail) < 3 {
		return invalid("Describe el motivo del ajuste (al menos 3 caracteres).")
	}
	return nil
}

// prepareAdjustmentLines valida los renglones: de 1 a 200, un insumo una vez, cantidad con signo
// coherente con el motivo y, en una merma, solo salidas.
func (handler *service) prepareAdjustmentLines(input adjustmentInput) ([]preparedAdjustmentLine, error) {
	if len(input.Lines) == 0 || len(input.Lines) > maximumControlLines {
		return nil, invalid("El ajuste requiere de 1 a 200 renglones.")
	}
	direction := adjustmentReasonDirection[input.Reason]
	prepared := make([]preparedAdjustmentLine, 0, len(input.Lines))
	seen := map[string]bool{}
	for index, line := range input.Lines {
		prefix := "Renglón " + itoa(index+1) + ": "
		capture, err := handler.controlCapture(line.ItemID, line.PresentationID, line.UnitID, line.Quantity, prefix, false, true)
		if err != nil {
			return nil, err
		}
		if seen[capture.item.ID] {
			return nil, invalid("El insumo «" + capture.item.Name + "» aparece en más de un renglón.")
		}
		seen[capture.item.ID] = true
		negative := capture.base.IsNegative()
		if input.Kind == adjustmentWaste && !negative {
			return nil, invalid(prefix + "una merma solo registra salidas: captura la cantidad con signo negativo.")
		}
		if direction < 0 && !negative {
			return nil, invalid(prefix + "el motivo «" + adjustmentReasonLabels[input.Reason] + "» solo admite salidas (cantidad negativa).")
		}
		if direction > 0 && negative {
			return nil, invalid(prefix + "el motivo «Sobrante» solo admite entradas (cantidad positiva).")
		}
		prepared = append(prepared, preparedAdjustmentLine{capture: capture})
	}
	return prepared, nil
}

// adjustmentMovementReason es el motivo que llevan los movimientos del ajuste.
func adjustmentMovementReason(reason, detail string) *string {
	text := adjustmentReasonLabels[reason]
	if detail != "" {
		text += ": " + detail
	}
	if textLength(text) > 500 {
		text = string([]rune(text)[:500])
	}
	return &text
}

// recordAdjustment registra un ajuste de discrepancia o una merma.
func (handler *service) recordAdjustment(_ []string) (any, error) {
	var input adjustmentInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := handler.validateAdjustmentReason(&input); err != nil {
		return nil, err
	}
	notes, err := controlNotes(input.Notes, 500)
	if err != nil {
		return nil, err
	}
	warehouse, err := handler.activeWarehouse(input.WarehouseID, "el almacén del ajuste")
	if err != nil {
		return nil, err
	}
	lines, err := handler.prepareAdjustmentLines(input)
	if err != nil {
		return nil, err
	}
	now, err := handler.transactionTime()
	if err != nil {
		return nil, err
	}
	// Una merma se aplica siempre; una discrepancia, solo si quien la registra puede autorizarla.
	apply := input.Kind == adjustmentWaste || handler.request.Actor.Can(permissionAdjustmentsAuthorize)
	state := "pending_authorization"
	var appliedAt *time.Time
	if apply {
		state = "applied"
		appliedAt = &now
	}
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_adjustments(id,kind,reason,reason_detail,warehouse_id,state,notes,
            line_count,requested_by,requested_at,applied_at,operation_id,updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$10)`, id, input.Kind, input.Reason, input.ReasonDetail, warehouse.ID, state,
		notes, len(lines), handler.request.Actor.UserID, now, appliedAt, handler.operationReference())
	if err != nil {
		return nil, err
	}
	warnings := []Warning{}
	movements := map[int]string{}
	if apply {
		header := adjustmentHeader{id: id, kind: input.Kind, reason: input.Reason, detail: input.ReasonDetail, warehouse: warehouse}
		movements, warnings, err = handler.applyAdjustmentLines(header, lines, now)
		if err != nil {
			return nil, err
		}
	}
	for index, line := range lines {
		number := index + 1
		presentationID, unitID := captureColumns(line.capture)
		var movementID *string
		if movement, found := movements[number]; found {
			movementID = &movement
		}
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_adjustment_lines(adjustment_id,line_no,item_id,presentation_id,
                unit_id,captured_quantity,quantity,movement_id)
            VALUES ($1,$2,$3,$4,$5,$6::numeric,$7::numeric,$8)`, id, number, line.capture.item.ID, presentationID, unitID,
			line.capture.quantity.String(), line.capture.base.String(), movementID)
		if err != nil {
			return nil, err
		}
	}
	saved, err := handler.readAdjustment(id)
	if err != nil {
		return nil, err
	}
	saved.Warnings = warnings
	if err := handler.publishAdjustment(id, warehouse.ID, apply); err != nil {
		return nil, err
	}
	action := "inventory.adjustment_requested"
	switch {
	case input.Kind == adjustmentWaste:
		action = "inventory.waste_recorded"
	case apply:
		action = "inventory.adjustment_recorded"
	}
	return saved, handler.audit(handler.ctx, handler.tx, action, "inventory_adjustment", &id, nil, handler.auditedAdjustment(saved))
}

// adjustmentHeader son los datos del ajuste que necesita su aplicación.
type adjustmentHeader struct {
	id        string
	kind      string
	reason    string
	detail    string
	warehouse *Warehouse
}

// applyAdjustmentLines registra un movimiento por renglón con el motor de costos y devuelve el
// movimiento de cada renglón y los avisos de existencia negativa.
func (handler *service) applyAdjustmentLines(header adjustmentHeader, lines []preparedAdjustmentLine, occurredAt time.Time) (
	map[int]string, []Warning, error) {
	movements := map[int]string{}
	warnings := []Warning{}
	sourceKind := "adjustment"
	if header.kind == adjustmentWaste {
		sourceKind = "waste"
	}
	reason := adjustmentMovementReason(header.reason, header.detail)
	for index, line := range lines {
		number := index + 1
		kind := KindAdjustmentIn
		switch {
		case header.kind == adjustmentWaste:
			kind = KindWaste
		case line.capture.base.IsNegative():
			kind = KindAdjustmentOut
		}
		applied, err := Apply(handler.ctx, handler.tx, MovementInput{
			EntryKey: "adjustment:" + header.id + ":" + itoa(number), Kind: kind, ItemID: line.capture.item.ID,
			WarehouseID: header.warehouse.ID, Quantity: line.capture.base.Abs(), OccurredAt: occurredAt, SourceKind: sourceKind,
			SourceID: &header.id, SourceLine: &number, Reason: reason, ActorID: handler.actorReference(),
			OperationID: handler.operationReference(),
		})
		if err != nil {
			return nil, nil, err
		}
		movements[number] = applied.Movement.ID
		if applied.NegativeStock {
			warnings = append(warnings, negativeStockWarning(itemReferenceOf(line.capture.item), header.warehouse.Name))
		}
	}
	return movements, warnings, nil
}

// publishAdjustment publica el cambio del ajuste y, si se aplicó, el de existencias y alertas.
func (handler *service) publishAdjustment(id, warehouseID string, applied bool) error {
	if err := handler.publish("inventory.adjustments.changed", &id); err != nil {
		return err
	}
	if !applied {
		return nil
	}
	for _, kind := range []string{"inventory.stock.changed", "inventory.movements.recorded", "inventory.alerts.changed"} {
		aggregate := &id
		if kind == "inventory.stock.changed" {
			aggregate = &warehouseID
		}
		if err := handler.publish(kind, aggregate); err != nil {
			return err
		}
	}
	return nil
}

// auditedAdjustment es el registro de auditoría del ajuste (con valores: la auditoría es de
// administración).
func (handler *service) auditedAdjustment(adjustment *Adjustment) map[string]any {
	lines := []map[string]any{}
	for _, line := range adjustment.Lines {
		lines = append(lines, map[string]any{
			"line": line.LineNo, "item_id": line.Item.ID, "item_name": line.Item.Name, "captured_quantity": line.CapturedQuantity,
			"capture_label": line.CaptureLabel, "quantity": line.Quantity, "movement_id": line.MovementID,
		})
	}
	return map[string]any{
		"folio": adjustment.Folio, "kind": adjustment.Kind, "reason": adjustment.Reason, "reason_detail": adjustment.ReasonDetail,
		"warehouse_id": adjustment.Warehouse.ID, "warehouse_name": adjustment.Warehouse.Name, "state": adjustment.State,
		"notes": adjustment.Notes, "revision": adjustment.Revision, "lines": lines,
	}
}

type adjustmentDecisionInput struct {
	OperationID string `json:"operation_id"`
	Revision    int    `json:"revision"`
	Reason      string `json:"reason"`
}

// lockPendingAdjustment bloquea el ajuste y valida revisión, estado y que quien decide no sea quien
// lo registró. El permiso inventory.adjustments.authorize ya lo exigió la ruta.
func (handler *service) lockPendingAdjustment(id string, revision int) (*Adjustment, error) {
	if _, err := handler.tx.Exec(handler.ctx, "SELECT 1 FROM app.inventory_adjustments WHERE id=$1 FOR UPDATE", id); err != nil {
		return nil, err
	}
	adjustment, err := handler.readAdjustment(id)
	if err != nil {
		return nil, err
	}
	if adjustment.Revision != revision {
		return nil, revisionConflict("El ajuste de inventario")
	}
	if adjustment.State != "pending_authorization" {
		return nil, conflict("ADJUSTMENT_NOT_PENDING", "El ajuste ya está «"+adjustment.StateLabel+"»: no admite esta acción.")
	}
	if adjustment.requestedBy == handler.request.Actor.UserID {
		return nil, &Error{Status: 403, Code: "SELF_AUTHORIZATION_FORBIDDEN",
			Message: "No puedes autorizar ni rechazar un ajuste que tú registraste: debe decidirlo otra persona."}
	}
	return adjustment, nil
}

// authorizeAdjustment aplica un ajuste pendiente registrado por otra persona: escribe sus
// movimientos con la hora de la autorización.
func (handler *service) authorizeAdjustment(parts []string) (any, error) {
	var input adjustmentDecisionInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Reason) != "" {
		return nil, invalid("Autorizar un ajuste no lleva motivo.")
	}
	previous, err := handler.lockPendingAdjustment(parts[1], input.Revision)
	if err != nil {
		return nil, err
	}
	warehouse, err := handler.readWarehouse(previous.Warehouse.ID)
	if err != nil {
		return nil, err
	}
	lines, err := handler.pendingAdjustmentLines(previous)
	if err != nil {
		return nil, err
	}
	now, err := handler.transactionTime()
	if err != nil {
		return nil, err
	}
	header := adjustmentHeader{id: previous.ID, kind: previous.Kind, reason: previous.Reason, detail: previous.ReasonDetail,
		warehouse: warehouse}
	movements, warnings, err := handler.applyAdjustmentLines(header, lines, now)
	if err != nil {
		return nil, err
	}
	for number, movementID := range movements {
		_, err := handler.tx.Exec(handler.ctx, `UPDATE app.inventory_adjustment_lines SET movement_id=$3
            WHERE adjustment_id=$1 AND line_no=$2`, previous.ID, number, movementID)
		if err != nil {
			return nil, err
		}
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_adjustments SET state='applied',authorized_by=$2,authorized_at=$3,
            applied_at=$3,decision_operation_id=$4,revision=revision+1,updated_at=$3 WHERE id=$1`, previous.ID,
		handler.request.Actor.UserID, now, handler.operationReference())
	if err != nil {
		return nil, err
	}
	saved, err := handler.readAdjustment(previous.ID)
	if err != nil {
		return nil, err
	}
	saved.Warnings = warnings
	if err := handler.publishAdjustment(saved.ID, warehouse.ID, true); err != nil {
		return nil, err
	}
	before := map[string]any{"state": previous.State, "revision": previous.Revision}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.adjustment_authorized", "inventory_adjustment", &saved.ID, before,
		handler.auditedAdjustment(saved))
}

// pendingAdjustmentLines vuelve a preparar los renglones guardados de un ajuste pendiente con su
// cantidad base registrada (las conversiones no se recalculan al autorizar).
func (handler *service) pendingAdjustmentLines(adjustment *Adjustment) ([]preparedAdjustmentLine, error) {
	lines := make([]preparedAdjustmentLine, 0, len(adjustment.Lines))
	for index, line := range adjustment.Lines {
		if line.LineNo != index+1 {
			return nil, fmt.Errorf("inventory: renglones del ajuste fuera de orden")
		}
		item, err := handler.readItem(line.Item.ID)
		if err != nil {
			return nil, err
		}
		base, err := decimal.NewFromString(line.Quantity)
		if err != nil {
			return nil, err
		}
		lines = append(lines, preparedAdjustmentLine{capture: purchaseCapture{item: item, base: base}})
	}
	return lines, nil
}

// rejectAdjustment rechaza con motivo un ajuste pendiente registrado por otra persona; no escribe
// movimientos.
func (handler *service) rejectAdjustment(parts []string) (any, error) {
	var input adjustmentDecisionInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	reason, err := purchaseReason(input.Reason, "El motivo del rechazo")
	if err != nil {
		return nil, err
	}
	previous, err := handler.lockPendingAdjustment(parts[1], input.Revision)
	if err != nil {
		return nil, err
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_adjustments SET state='rejected',rejected_by=$2,rejected_at=now(),
            rejection_reason=$3,decision_operation_id=$4,revision=revision+1,updated_at=now() WHERE id=$1`, previous.ID,
		handler.request.Actor.UserID, reason, handler.operationReference())
	if err != nil {
		return nil, err
	}
	saved, err := handler.readAdjustment(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publishAdjustment(saved.ID, saved.Warehouse.ID, false); err != nil {
		return nil, err
	}
	before := map[string]any{"state": previous.State, "revision": previous.Revision}
	after := map[string]any{"state": saved.State, "revision": saved.Revision, "reason": reason}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.adjustment_rejected", "inventory_adjustment", &saved.ID, before,
		after)
}
