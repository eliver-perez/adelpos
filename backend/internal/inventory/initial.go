package inventory

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Existencia inicial (§6 y regla 4 del motor). Es un movimiento «initial» por renglón (origen
// initial, source_id = operation_id) que se valora como una entrada con documento:
//   - exige el costo unitario capturado (por la unidad o presentación capturada) o confirmar
//     explícitamente el costo de referencia configurado del insumo (use_reference_cost: true);
//   - solo procede si el insumo no tiene movimientos en ese almacén: después, las diferencias se
//     registran con recepciones o ajustes (fases C y D), nunca con otra existencia inicial;
//   - exige inventory.adjustments.authorize e inventory.costs.manage (comprobados por la ruta);
//   - es idempotente por operation_id (restaurant_operations) y por renglón (entry_key).

const maximumInitialLines = 500

type initialBalanceInput struct {
	OperationID string             `json:"operation_id"`
	WarehouseID string             `json:"warehouse_id"`
	OccurredAt  *time.Time         `json:"occurred_at"`
	Reason      *string            `json:"reason"`
	Lines       []initialLineInput `json:"lines"`
}

type initialLineInput struct {
	ItemID string `json:"item_id"`
	// Quantity se captura en la unidad base, en otra unidad de su dimensión (unit_id) o en
	// presentaciones del insumo (presentation_id).
	Quantity       string  `json:"quantity"`
	UnitID         *string `json:"unit_id"`
	PresentationID *string `json:"presentation_id"`
	// UnitCost es el costo por unidad capturada (por caja si se captura en cajas).
	UnitCost         *string `json:"unit_cost"`
	UseReferenceCost bool    `json:"use_reference_cost"`
}

// InitialBalanceResult es la respuesta de una existencia inicial registrada.
type InitialBalanceResult struct {
	OperationID  string               `json:"operation_id"`
	Warehouse    WarehouseReference   `json:"warehouse"`
	OccurredAt   time.Time            `json:"occurred_at"`
	Reason       *string              `json:"reason"`
	CostsVisible bool                 `json:"costs_visible"`
	Lines        []InitialBalanceLine `json:"lines"`
	Warnings     []Warning            `json:"warnings"`
}

// InitialBalanceLine es un renglón aplicado. Los costos solo aparecen con inventory.costs.read.
type InitialBalanceLine struct {
	Line              int           `json:"line"`
	ItemID            string        `json:"item_id"`
	ItemCode          string        `json:"item_code"`
	ItemName          string        `json:"item_name"`
	Unit              UnitReference `json:"unit"`
	CapturedQuantity  string        `json:"captured_quantity"`
	CapturedUnitLabel string        `json:"captured_unit_label"`
	Quantity          string        `json:"quantity"`
	StockAfter        string        `json:"stock_after"`
	MovementID        string        `json:"movement_id"`
	UseReferenceCost  bool          `json:"use_reference_cost"`
	UnitCost          *string       `json:"unit_cost"`
	UnitCostDisplay   *string       `json:"unit_cost_display"`
	Value             *string       `json:"value"`
	ValueDisplay      *string       `json:"value_display"`
}

// Warning es un aviso que no bloquea la operación (por ejemplo, existencia negativa).
type Warning struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	ItemID  *string `json:"item_id"`
}

// preparedLine es un renglón validado con su cantidad base y su valor de documento.
type preparedLine struct {
	item         *Item
	captured     decimal.Decimal
	capturedName string
	quantity     decimal.Decimal
	value        decimal.Decimal
	reference    bool
}

func (handler *service) recordInitialBalances(_ []string) (any, error) {
	var input initialBalanceInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.WarehouseID); err != nil {
		return nil, invalid("Elige el almacén de la existencia inicial.")
	}
	if len(input.Lines) == 0 || len(input.Lines) > maximumInitialLines {
		return nil, invalid("La existencia inicial requiere de 1 a 500 renglones.")
	}
	reason, err := initialReason(input.Reason)
	if err != nil {
		return nil, err
	}
	warehouse, err := handler.readWarehouse(strings.ToLower(input.WarehouseID))
	if err != nil {
		return nil, err
	}
	if !warehouse.Active {
		return nil, conflict("WAREHOUSE_INACTIVE", "El almacén «"+warehouse.Name+"» está desactivado.")
	}
	occurredAt, err := handler.initialTime(input.OccurredAt)
	if err != nil {
		return nil, err
	}
	prepared := make([]preparedLine, 0, len(input.Lines))
	seen := map[string]bool{}
	for index := range input.Lines {
		line, err := handler.prepareInitialLine(input.Lines[index], index+1, warehouse)
		if err != nil {
			return nil, err
		}
		if seen[line.item.ID] {
			return nil, invalid("El insumo «" + line.item.Name + "» aparece en más de un renglón.")
		}
		seen[line.item.ID] = true
		prepared = append(prepared, line)
	}
	result := InitialBalanceResult{
		OperationID: handler.operationID, Warehouse: WarehouseReference{ID: warehouse.ID, Name: warehouse.Name},
		OccurredAt: occurredAt, Reason: reason, CostsVisible: handler.costsVisible(), Lines: []InitialBalanceLine{},
		Warnings: []Warning{},
	}
	audited := []map[string]any{}
	for index, line := range prepared {
		number := index + 1
		value := line.value
		applied, err := Apply(handler.ctx, handler.tx, MovementInput{
			EntryKey:    "initial:" + handler.operationID + ":" + itoa(number),
			Kind:        KindInitial,
			ItemID:      line.item.ID,
			WarehouseID: warehouse.ID,
			Quantity:    line.quantity,
			Value:       &value,
			OccurredAt:  occurredAt,
			SourceKind:  "initial",
			SourceID:    handler.operationReference(),
			SourceLine:  &number,
			Reason:      reason,
			ActorID:     handler.actorReference(),
			OperationID: handler.operationReference(),
		})
		if err != nil {
			return nil, err
		}
		result.Lines = append(result.Lines, handler.initialLineResult(number, line, applied))
		audited = append(audited, map[string]any{
			"line": number, "item_id": line.item.ID, "item_name": line.item.Name, "captured_quantity": line.captured.String(),
			"captured_unit": line.capturedName, "quantity": line.quantity.String(), "value": value.String(),
			"use_reference_cost": line.reference, "movement_id": applied.Movement.ID,
		})
	}
	if err := handler.publish("inventory.stock.changed", &warehouse.ID); err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.movements.recorded", handler.operationReference()); err != nil {
		return nil, err
	}
	after := map[string]any{"warehouse_id": warehouse.ID, "warehouse_name": warehouse.Name, "occurred_at": occurredAt,
		"reason": reason, "lines": audited}
	err = handler.audit(handler.ctx, handler.tx, "inventory.initial_balance_recorded", "inventory_initial_balance",
		handler.operationReference(), nil, after)
	return result, err
}

// initialReason recorta la nota opcional de la existencia inicial (hasta 500 caracteres).
func initialReason(text *string) (*string, error) {
	if text == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*text)
	if trimmed == "" {
		return nil, nil
	}
	if textLength(trimmed) > 500 {
		return nil, invalid("La nota de la existencia inicial admite hasta 500 caracteres.")
	}
	return &trimmed, nil
}

// initialTime usa la hora indicada (no futura) o la de la transacción.
func (handler *service) initialTime(requested *time.Time) (time.Time, error) {
	var now time.Time
	if err := handler.tx.QueryRow(handler.ctx, "SELECT now()").Scan(&now); err != nil {
		return time.Time{}, err
	}
	if requested == nil {
		return now, nil
	}
	if requested.After(now.Add(5 * time.Minute)) {
		return time.Time{}, invalid("La fecha de la existencia inicial no puede ser futura.")
	}
	return requested.UTC(), nil
}

// prepareInitialLine valida un renglón: insumo activo sin movimientos en el almacén, cantidad
// convertida a la unidad base y valor del documento.
func (handler *service) prepareInitialLine(input initialLineInput, number int, warehouse *Warehouse) (preparedLine, error) {
	prefix := "Renglón " + itoa(number) + ": "
	if err := validateID(input.ItemID); err != nil {
		return preparedLine{}, invalid(prefix + "elige el insumo.")
	}
	item, err := handler.readItem(strings.ToLower(input.ItemID))
	if err != nil {
		return preparedLine{}, err
	}
	if !item.Active {
		return preparedLine{}, conflict("ITEM_INACTIVE", "El insumo «"+item.Name+"» está desactivado.")
	}
	var recorded bool
	err = handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.inventory_movements WHERE item_id=$1 AND warehouse_id=$2)",
		item.ID, warehouse.ID).Scan(&recorded)
	if err != nil {
		return preparedLine{}, err
	}
	if recorded {
		return preparedLine{}, conflict("INITIAL_BALANCE_EXISTS", "El insumo «"+item.Name+"» ya tiene movimientos en «"+
			warehouse.Name+"»: registra la diferencia con una recepción o un ajuste.")
	}
	captured, err := parseQuantity(input.Quantity, prefix+"la cantidad")
	if err != nil {
		return preparedLine{}, err
	}
	line := preparedLine{item: item, captured: captured, reference: input.UseReferenceCost}
	if err := handler.convertInitialQuantity(&line, input, prefix); err != nil {
		return preparedLine{}, err
	}
	if err := initialValue(&line, input, prefix); err != nil {
		return preparedLine{}, err
	}
	return line, nil
}

// convertInitialQuantity calcula la cantidad base del renglón según lo capturado.
func (handler *service) convertInitialQuantity(line *preparedLine, input initialLineInput, prefix string) error {
	if input.UnitID != nil && input.PresentationID != nil {
		return invalid(prefix + "captura la cantidad en una unidad o en una presentación, no en ambas.")
	}
	if input.PresentationID != nil {
		presentationID, err := validateOptionalID(input.PresentationID)
		if err != nil || presentationID == nil {
			return invalid(prefix + "la presentación no es válida.")
		}
		for _, presentation := range line.item.Presentations {
			if presentation.ID != *presentationID {
				continue
			}
			if !presentation.Active {
				return conflict("PRESENTATION_INACTIVE", "La presentación «"+presentation.Name+"» está desactivada.")
			}
			quantity, err := presentationQuantity(line.captured, parseDatabaseDecimal(presentation.BaseQuantity))
			if err != nil {
				return err
			}
			line.quantity, line.capturedName = quantity, presentation.Name
			return nil
		}
		return invalid(prefix + "la presentación no pertenece al insumo «" + line.item.Name + "».")
	}
	unitID, err := validateOptionalID(input.UnitID)
	if err != nil {
		return invalid(prefix + "la unidad no es válida.")
	}
	quantity, err := handler.toBaseQuantity(line.item, line.captured, unitID)
	if err != nil {
		return err
	}
	line.quantity = quantity
	line.capturedName = line.item.BaseUnit.Name
	if unitID != nil && *unitID != line.item.BaseUnit.ID {
		_, name, err := handler.unitMeasure(*unitID, false)
		if err != nil {
			return err
		}
		line.capturedName = name
	}
	return nil
}

// initialValue calcula el valor del documento: cantidad capturada × costo por unidad capturada, o
// cantidad base × costo de referencia configurado si el usuario lo confirmó. Nunca se inventa.
func initialValue(line *preparedLine, input initialLineInput, prefix string) error {
	if input.UseReferenceCost {
		if input.UnitCost != nil && strings.TrimSpace(*input.UnitCost) != "" {
			return invalid(prefix + "captura el costo unitario o usa el costo de referencia, no ambos.")
		}
		if line.item.ReferenceCost == nil {
			return conflict("REFERENCE_COST_MISSING", "El insumo «"+line.item.Name+"» no tiene costo de referencia configurado.")
		}
		reference := decimal.RequireFromString(*line.item.ReferenceCost)
		line.value = line.quantity.Mul(reference).Round(valueScale)
		return nil
	}
	if input.UnitCost == nil || strings.TrimSpace(*input.UnitCost) == "" {
		return invalid(prefix + "captura el costo unitario o confirma el costo de referencia del insumo.")
	}
	cost, err := parseCost(*input.UnitCost, prefix+"el costo unitario")
	if err != nil {
		return err
	}
	line.value = line.captured.Mul(cost).Round(valueScale)
	return nil
}

func (handler *service) initialLineResult(number int, line preparedLine, applied LedgerResult) InitialBalanceLine {
	result := InitialBalanceLine{
		Line: number, ItemID: line.item.ID, ItemCode: line.item.Code, ItemName: line.item.Name,
		Unit: line.item.BaseUnit, CapturedQuantity: quantityText(line.captured), CapturedUnitLabel: line.capturedName,
		Quantity: quantityText(line.quantity), StockAfter: quantityText(applied.StockAfter), MovementID: applied.Movement.ID,
		UseReferenceCost: line.reference,
	}
	if handler.costsVisible() {
		value := applied.Movement.Value
		exact, display := exactMoneyText(value), displayMoneyText(value)
		result.Value, result.ValueDisplay = &exact, &display
		if applied.Movement.UnitCost != nil {
			cost, costDisplay := exactMoneyText(*applied.Movement.UnitCost), displayUnitCostText(*applied.Movement.UnitCost)
			result.UnitCost, result.UnitCostDisplay = &cost, &costDisplay
		}
	}
	return result
}
