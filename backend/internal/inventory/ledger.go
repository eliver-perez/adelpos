package inventory

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Libro de movimientos de inventario. Es la única vía para cambiar existencias y valuaciones, y la
// usan todas las fases (existencia inicial, consumos, recepciones, ajustes, transferencias,
// conteos y devoluciones). Cada aplicación:
//  1. comprueba la clave de idempotencia entry_key: si ya existe, devuelve lo registrado sin
//     aplicar nada (reprocesar una solicitud nunca duplica movimientos);
//  2. bloquea con FOR UPDATE la valuación del insumo y la existencia del almacén (las crea en cero
//     si no existían);
//  3. aplica el motor de costos (costing.go);
//  4. inserta el movimiento principal y, si corresponde, el ajuste de costo separado y el residuo
//     de valuación, cada uno con el estado resultante para auditar;
//  5. actualiza existencia y valuación.
//
// El llamador debe haber tomado el lock 7820148308 exclusivo (después de 7820148305 y 7820148304
// si también los usa). Una existencia negativa nunca bloquea: el resultado la informa como aviso.

// Tipos de movimiento con la operación del motor que les corresponde.
const (
	KindInitial             = "initial"
	KindReceipt             = "receipt"
	KindConsumption         = "consumption"
	KindConsumptionReversal = "consumption_reversal"
	KindWaste               = "waste"
	KindAdjustmentIn        = "adjustment_in"
	KindAdjustmentOut       = "adjustment_out"
	KindTransferOut         = "transfer_out"
	KindTransferIn          = "transfer_in"
	KindSupplierReturn      = "supplier_return"
	KindProductionIn        = "production_in"
	KindProductionOut       = "production_out"
	KindCountIn             = "count_in"
	KindCountOut            = "count_out"
	KindCostAdjustment      = "cost_adjustment"
)

// Una elaboración (production_in) es una entrada con documento: su valor es el de lo consumido y,
// con existencia negativa del insumo preparado, el déficit cubierto se realinea con un ajuste de
// costo separado como en una recepción (regla 4 de §0-B).
var (
	documentKinds = map[string]bool{KindInitial: true, KindReceipt: true, KindProductionIn: true}
	issueKinds    = map[string]bool{
		KindConsumption: true, KindWaste: true, KindAdjustmentOut: true, KindProductionOut: true, KindCountOut: true,
	}
	admitKinds   = map[string]bool{KindAdjustmentIn: true, KindCountIn: true}
	restoreKinds = map[string]bool{KindConsumptionReversal: true}
)

var processPattern = regexp.MustCompile(`^[a-z][a-z_]{1,39}$`)

// MovementInput es una entrada del libro. Quantity es siempre la magnitud positiva en la unidad
// base del insumo; el signo lo da el tipo.
type MovementInput struct {
	// EntryKey es la clave determinista de idempotencia, por ejemplo initial:<operación>:<renglón>.
	EntryKey    string
	Kind        string
	ItemID      string
	WarehouseID string
	Quantity    decimal.Decimal
	// Value es el valor del documento (initial, receipt; en production_in, el valor de lo consumido)
	// o el valor original exacto del consumo que se compensa (consumption_reversal). No negativo.
	Value *decimal.Decimal
	// OriginalCost es el costo unitario original de lo devuelto (supplier_return).
	OriginalCost      *decimal.Decimal
	OccurredAt        time.Time
	SourceKind        string
	SourceID          *string
	SourceLine        *int
	RestaurantUnitID  *string
	RelatedMovementID *string
	Reason            *string
	// Exactamente uno: el usuario de la operación o el proceso de Go (por ejemplo inventory_worker).
	ActorID     *string
	Process     *string
	OperationID *string
}

// LedgerMovement es un movimiento registrado en el libro.
type LedgerMovement struct {
	ID                     string
	Seq                    int64
	ItemID                 string
	WarehouseID            *string
	Kind                   string
	Quantity               decimal.Decimal
	Value                  decimal.Decimal
	UnitCost               *decimal.Decimal
	CostBasis              string
	CostPending            bool
	OccurredAt             time.Time
	RecordedAt             time.Time
	SourceKind             string
	SourceID               *string
	SourceLine             *int
	RestaurantUnitID       *string
	RelatedMovementID      *string
	Reason                 *string
	ActorID                *string
	Process                *string
	OperationID            *string
	EntryKey               string
	StockAfter             *decimal.Decimal
	ValuationQuantityAfter decimal.Decimal
	ValuationValueAfter    decimal.Decimal
}

// LedgerResult es lo que produjo (o ya había producido) una entrada del libro.
type LedgerResult struct {
	Movement LedgerMovement
	// Adjustments son los movimientos de solo valor que la acompañan: ajuste de costo y residuo.
	Adjustments []LedgerMovement
	StockAfter  decimal.Decimal
	Valuation   Valuation
	// NegativeStock avisa que la existencia del almacén quedó por debajo de cero (nunca bloquea).
	NegativeStock bool
	// Unabsorbed y DocumentValue solo aplican a devoluciones a proveedor; el documento de la
	// devolución los guarda en su primera aplicación (un reintento devuelve cero).
	Unabsorbed    decimal.Decimal
	DocumentValue decimal.Decimal
	// Replayed indica que la entrada ya existía y no se aplicó otra vez.
	Replayed bool
}

// validateCommon revisa lo que comparten todas las entradas: clave, insumo, almacén, cantidad,
// origen, responsable, hora y motivo.
func (input MovementInput) validateCommon() error {
	if textLength(input.EntryKey) < 1 || textLength(input.EntryKey) > 190 {
		return fmt.Errorf("inventory: clave de movimiento inválida")
	}
	if !validID(input.ItemID) || !validID(input.WarehouseID) {
		return invalid("El insumo o el almacén del movimiento no son válidos.")
	}
	if !input.Quantity.IsPositive() || input.Quantity.Exponent() < -quantityScale {
		return invalid("La cantidad del movimiento debe ser positiva y con hasta seis decimales.")
	}
	if input.Quantity.GreaterThan(maximumQuantity) {
		return invalid("La cantidad del movimiento excede el máximo admitido.")
	}
	if !processPattern.MatchString(input.SourceKind) {
		return fmt.Errorf("inventory: origen de movimiento inválido %q", input.SourceKind)
	}
	if (input.ActorID == nil) == (input.Process == nil) {
		return fmt.Errorf("inventory: el movimiento requiere exactamente un responsable")
	}
	if input.Process != nil && !processPattern.MatchString(*input.Process) {
		return fmt.Errorf("inventory: proceso inválido %q", *input.Process)
	}
	if input.OccurredAt.IsZero() {
		return fmt.Errorf("inventory: el movimiento requiere su hora de negocio")
	}
	if input.Reason != nil && (textLength(*input.Reason) < 1 || textLength(*input.Reason) > 500) {
		return invalid("El motivo del movimiento admite de 1 a 500 caracteres.")
	}
	return nil
}

// validate agrega a validateCommon los datos que exige cada tipo de movimiento.
func (input MovementInput) validate() error {
	if err := input.validateCommon(); err != nil {
		return err
	}
	switch {
	case documentKinds[input.Kind], restoreKinds[input.Kind]:
		if input.Value == nil || input.Value.IsNegative() {
			return fmt.Errorf("inventory: %s requiere un valor no negativo", input.Kind)
		}
	case input.Kind == KindSupplierReturn:
		if input.OriginalCost == nil || input.OriginalCost.IsNegative() {
			return fmt.Errorf("inventory: la devolución requiere su costo original")
		}
	case issueKinds[input.Kind], admitKinds[input.Kind]:
	default:
		// Las transferencias se registran en pareja con ApplyTransfer y los ajustes de costo solo
		// los produce el motor.
		return fmt.Errorf("inventory: tipo de movimiento no admitido en Apply: %s", input.Kind)
	}
	return nil
}

// Apply registra una entrada del libro. Es idempotente por EntryKey.
func Apply(ctx context.Context, tx pgx.Tx, input MovementInput) (LedgerResult, error) {
	if err := input.validate(); err != nil {
		return LedgerResult{}, err
	}
	if err := ensureLedgerLock(ctx, tx); err != nil {
		return LedgerResult{}, err
	}
	if recorded, found, err := replayEntry(ctx, tx, input.EntryKey); err != nil || found {
		return recorded, err
	}
	before, err := lockValuation(ctx, tx, input.ItemID)
	if err != nil {
		return LedgerResult{}, err
	}
	stockBefore, err := lockStock(ctx, tx, input.ItemID, input.WarehouseID)
	if err != nil {
		return LedgerResult{}, err
	}
	entry, extra, err := evaluate(before, input)
	if errors.Is(err, errCostRequired) {
		return LedgerResult{}, costRequired(ctx, tx, input.ItemID)
	}
	if err != nil {
		return LedgerResult{}, err
	}
	// Las salidas restan existencia; las entradas la suman.
	signed := input.Quantity
	if issueKinds[input.Kind] || input.Kind == KindSupplierReturn {
		signed = input.Quantity.Neg()
	}
	stockAfter := stockBefore.Add(signed)
	result := LedgerResult{StockAfter: stockAfter, Unabsorbed: extra.Unabsorbed, DocumentValue: extra.DocumentValue}
	main := movementRow(input, input.Kind, signed, entry.Value, entry.UnitCost, entry.Basis, entry.CostPending)
	main.WarehouseID = &input.WarehouseID
	main.StockAfter = &stockAfter
	main.ValuationQuantityAfter = before.Quantity.Add(signed)
	main.ValuationValueAfter = before.Value.Add(entry.Value)
	if err := insertMovement(ctx, tx, &main); err != nil {
		return LedgerResult{}, err
	}
	result.Movement = main
	running := main.ValuationValueAfter
	for _, adjustment := range []struct {
		suffix string
		value  *CostAdjustment
		reason string
	}{
		{":cost", entry.Adjustment, "Ajuste de costo por diferencia"},
		{":residual", entry.Residual, "Residuo de valuación"},
	} {
		if adjustment.value == nil {
			continue
		}
		running = running.Add(adjustment.value.Value)
		row := costAdjustmentRow(input, adjustment.suffix, adjustment.value, adjustment.reason, main.ID)
		row.ValuationQuantityAfter = main.ValuationQuantityAfter
		row.ValuationValueAfter = running
		if err := insertMovement(ctx, tx, &row); err != nil {
			return LedgerResult{}, err
		}
		result.Adjustments = append(result.Adjustments, row)
	}
	// El estado del motor y la suma de los movimientos deben coincidir exactamente: el valor de la
	// valuación es siempre la suma de los valores de sus movimientos.
	if !running.Equal(entry.After.Value) || !main.ValuationQuantityAfter.Equal(entry.After.Quantity) {
		return LedgerResult{}, fmt.Errorf("inventory: el libro no coincide con el motor de costos")
	}
	if err := saveStock(ctx, tx, input.ItemID, input.WarehouseID, stockAfter); err != nil {
		return LedgerResult{}, err
	}
	if err := saveValuation(ctx, tx, input.ItemID, entry.After); err != nil {
		return LedgerResult{}, err
	}
	result.Valuation = entry.After
	result.NegativeStock = stockAfter.IsNegative()
	return result, nil
}

// costRequired es el rechazo de una entrada sin documento que dejaría existencia positiva sin ningún
// costo: el usuario configura el costo de referencia del insumo y vuelve a intentarlo. La
// transacción completa se revierte (en un conteo no se aplica ningún renglón).
func costRequired(ctx context.Context, tx pgx.Tx, itemID string) error {
	var name string
	if err := tx.QueryRow(ctx, "SELECT name FROM app.inventory_items WHERE id=$1", itemID).Scan(&name); err != nil {
		return err
	}
	return conflict("COST_REQUIRED", "El insumo «"+name+"» no tiene costo: configura su costo de referencia antes de registrar "+
		"una entrada sin documento (ajuste positivo o sobrante de conteo).")
}

// returnExtras son los datos de una devolución que no son movimientos.
type returnExtras struct {
	Unabsorbed    decimal.Decimal
	DocumentValue decimal.Decimal
}

// evaluate elige la operación del motor según el tipo de movimiento.
func evaluate(before Valuation, input MovementInput) (Entry, returnExtras, error) {
	switch {
	case documentKinds[input.Kind]:
		entry, err := Receive(before, input.Quantity, *input.Value)
		return entry, returnExtras{}, err
	case issueKinds[input.Kind]:
		entry, err := Issue(before, input.Quantity)
		return entry, returnExtras{}, err
	case admitKinds[input.Kind]:
		entry, err := Admit(before, input.Quantity)
		return entry, returnExtras{}, err
	case restoreKinds[input.Kind]:
		entry, err := Restore(before, input.Quantity, *input.Value)
		return entry, returnExtras{}, err
	case input.Kind == KindSupplierReturn:
		entry, err := ReturnToSupplier(before, input.Quantity, *input.OriginalCost)
		return entry.Entry, returnExtras{Unabsorbed: entry.Unabsorbed, DocumentValue: entry.DocumentValue}, err
	}
	return Entry{}, returnExtras{}, fmt.Errorf("inventory: tipo de movimiento sin operación: %s", input.Kind)
}

// TransferInput es una transferencia de un insumo entre dos almacenes (fase D).
type TransferInput struct {
	EntryKey        string
	ItemID          string
	FromWarehouseID string
	ToWarehouseID   string
	Quantity        decimal.Decimal
	OccurredAt      time.Time
	SourceKind      string
	SourceID        *string
	SourceLine      *int
	Reason          *string
	ActorID         *string
	Process         *string
	OperationID     *string
}

// TransferResult son los dos movimientos de una transferencia.
type TransferResult struct {
	Out       LedgerMovement
	In        LedgerMovement
	FromStock decimal.Decimal
	ToStock   decimal.Decimal
	// NegativeStock avisa que el almacén de origen quedó por debajo de cero (nunca bloquea).
	NegativeStock bool
	Replayed      bool
}

// ApplyTransfer registra la salida y la entrada vinculadas de una transferencia con el mismo valor:
// la cantidad y el valor del insumo no cambian (el costo es uno solo por insumo). Idempotente por
// EntryKey (claves <clave>:out y <clave>:in).
func ApplyTransfer(ctx context.Context, tx pgx.Tx, input TransferInput) (TransferResult, error) {
	if input.FromWarehouseID == input.ToWarehouseID {
		return TransferResult{}, invalid("El almacén de origen y el de destino deben ser distintos.")
	}
	outbound := MovementInput{
		EntryKey: input.EntryKey + ":out", Kind: KindTransferOut, ItemID: input.ItemID, WarehouseID: input.FromWarehouseID,
		Quantity: input.Quantity, OccurredAt: input.OccurredAt, SourceKind: input.SourceKind, SourceID: input.SourceID,
		SourceLine: input.SourceLine, Reason: input.Reason, ActorID: input.ActorID, Process: input.Process,
		OperationID: input.OperationID,
	}
	if err := outbound.validateCommon(); err != nil {
		return TransferResult{}, err
	}
	if !validID(input.ToWarehouseID) {
		return TransferResult{}, invalid("El almacén de destino no es válido.")
	}
	if err := ensureLedgerLock(ctx, tx); err != nil {
		return TransferResult{}, err
	}
	out, foundOut, err := replayEntry(ctx, tx, input.EntryKey+":out")
	if err != nil {
		return TransferResult{}, err
	}
	in, foundIn, err := replayEntry(ctx, tx, input.EntryKey+":in")
	if err != nil {
		return TransferResult{}, err
	}
	if foundOut && foundIn {
		return TransferResult{
			Out: out.Movement, In: in.Movement, FromStock: out.StockAfter, ToStock: in.StockAfter, Replayed: true,
			NegativeStock: out.StockAfter.IsNegative(),
		}, nil
	}
	if foundOut != foundIn {
		return TransferResult{}, fmt.Errorf("inventory: transferencia registrada a medias: %s", input.EntryKey)
	}
	before, err := lockValuation(ctx, tx, input.ItemID)
	if err != nil {
		return TransferResult{}, err
	}
	fromBefore, err := lockStock(ctx, tx, input.ItemID, input.FromWarehouseID)
	if err != nil {
		return TransferResult{}, err
	}
	toBefore, err := lockStock(ctx, tx, input.ItemID, input.ToWarehouseID)
	if err != nil {
		return TransferResult{}, err
	}
	entry, err := Transfer(before, input.Quantity)
	if err != nil {
		return TransferResult{}, err
	}
	fromAfter := fromBefore.Sub(input.Quantity)
	toAfter := toBefore.Add(input.Quantity)
	outRow := movementRow(outbound, KindTransferOut, input.Quantity.Neg(), entry.Value.Neg(), entry.UnitCost, entry.Basis, entry.CostPending)
	outRow.WarehouseID = &input.FromWarehouseID
	outRow.StockAfter = &fromAfter
	outRow.ValuationQuantityAfter = before.Quantity.Sub(input.Quantity)
	outRow.ValuationValueAfter = before.Value.Sub(entry.Value)
	if err := insertMovement(ctx, tx, &outRow); err != nil {
		return TransferResult{}, err
	}
	inbound := outbound
	inbound.EntryKey = input.EntryKey + ":in"
	inbound.WarehouseID = input.ToWarehouseID
	inbound.RelatedMovementID = &outRow.ID
	inRow := movementRow(inbound, KindTransferIn, input.Quantity, entry.Value, entry.UnitCost, entry.Basis, entry.CostPending)
	inRow.WarehouseID = &input.ToWarehouseID
	inRow.StockAfter = &toAfter
	inRow.ValuationQuantityAfter = before.Quantity
	inRow.ValuationValueAfter = before.Value
	if err := insertMovement(ctx, tx, &inRow); err != nil {
		return TransferResult{}, err
	}
	if err := saveStock(ctx, tx, input.ItemID, input.FromWarehouseID, fromAfter); err != nil {
		return TransferResult{}, err
	}
	if err := saveStock(ctx, tx, input.ItemID, input.ToWarehouseID, toAfter); err != nil {
		return TransferResult{}, err
	}
	return TransferResult{Out: outRow, In: inRow, FromStock: fromAfter, ToStock: toAfter, NegativeStock: fromAfter.IsNegative()}, nil
}

// ensureLedgerLock vuelve a pedir el lock de inventario exclusivo. Los locks consultivos son
// reentrantes dentro de la transacción: si el llamador ya lo tiene (lo correcto), no espera.
func ensureLedgerLock(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, fmt.Sprintf("SELECT pg_advisory_xact_lock(%d)", lockKey))
	return err
}

// lockValuation bloquea (o crea en cero) la valuación del insumo y la combina con su costo de
// referencia configurado.
func lockValuation(ctx context.Context, tx pgx.Tx, itemID string) (Valuation, error) {
	var reference *string
	err := tx.QueryRow(ctx, "SELECT reference_cost::text FROM app.inventory_items WHERE id=$1", itemID).Scan(&reference)
	if isNoRows(err) {
		return Valuation{}, notFound("El insumo no existe.")
	}
	if err != nil {
		return Valuation{}, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO app.inventory_valuations(item_id) VALUES ($1) ON CONFLICT DO NOTHING", itemID); err != nil {
		return Valuation{}, err
	}
	var quantity, value, pending string
	var last *string
	err = tx.QueryRow(ctx, `SELECT quantity::text,value::text,last_valid_cost::text,pending_cost_quantity::text
        FROM app.inventory_valuations WHERE item_id=$1 FOR UPDATE`, itemID).Scan(&quantity, &value, &last, &pending)
	if err != nil {
		return Valuation{}, err
	}
	return Valuation{
		Quantity:        parseDatabaseDecimal(quantity),
		Value:           parseDatabaseDecimal(value),
		LastValidCost:   parseDatabaseOptional(last),
		ReferenceCost:   parseDatabaseOptional(reference),
		PendingQuantity: parseDatabaseDecimal(pending),
	}, nil
}

// lockStock bloquea (o crea en cero) la existencia del insumo en el almacén.
func lockStock(ctx context.Context, tx pgx.Tx, itemID, warehouseID string) (decimal.Decimal, error) {
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.inventory_warehouses WHERE id=$1)", warehouseID).Scan(&exists); err != nil {
		return zero, err
	}
	if !exists {
		return zero, notFound("El almacén no existe.")
	}
	_, err := tx.Exec(ctx, "INSERT INTO app.inventory_stock(item_id,warehouse_id) VALUES ($1,$2) ON CONFLICT DO NOTHING", itemID, warehouseID)
	if err != nil {
		return zero, err
	}
	var quantity string
	err = tx.QueryRow(ctx, "SELECT quantity::text FROM app.inventory_stock WHERE item_id=$1 AND warehouse_id=$2 FOR UPDATE",
		itemID, warehouseID).Scan(&quantity)
	if err != nil {
		return zero, err
	}
	return parseDatabaseDecimal(quantity), nil
}

func saveStock(ctx context.Context, tx pgx.Tx, itemID, warehouseID string, quantity decimal.Decimal) error {
	_, err := tx.Exec(ctx, `UPDATE app.inventory_stock SET quantity=$3::numeric,updated_at=now()
        WHERE item_id=$1 AND warehouse_id=$2`, itemID, warehouseID, quantity.String())
	return err
}

func saveValuation(ctx context.Context, tx pgx.Tx, itemID string, state Valuation) error {
	var last *string
	if state.LastValidCost != nil {
		text := state.LastValidCost.String()
		last = &text
	}
	_, err := tx.Exec(ctx, `UPDATE app.inventory_valuations SET quantity=$2::numeric,value=$3::numeric,
        last_valid_cost=$4::numeric,pending_cost_quantity=$5::numeric,updated_at=now() WHERE item_id=$1`,
		itemID, state.Quantity.String(), state.Value.String(), last, state.PendingQuantity.String())
	return err
}

// movementRow arma un movimiento con los datos de origen y responsable de la entrada.
func movementRow(input MovementInput, kind string, quantity, value decimal.Decimal, unitCost *decimal.Decimal, basis string,
	pending bool) LedgerMovement {
	return LedgerMovement{
		ID:                newID(),
		ItemID:            input.ItemID,
		Kind:              kind,
		Quantity:          quantity,
		Value:             value,
		UnitCost:          unitCost,
		CostBasis:         basis,
		CostPending:       pending,
		OccurredAt:        input.OccurredAt,
		SourceKind:        input.SourceKind,
		SourceID:          input.SourceID,
		SourceLine:        input.SourceLine,
		RestaurantUnitID:  input.RestaurantUnitID,
		RelatedMovementID: input.RelatedMovementID,
		Reason:            input.Reason,
		ActorID:           input.ActorID,
		Process:           input.Process,
		OperationID:       input.OperationID,
		EntryKey:          input.EntryKey,
	}
}

// costAdjustmentRow arma el movimiento de solo valor (sin almacén ni cantidad) ligado al principal.
func costAdjustmentRow(input MovementInput, suffix string, adjustment *CostAdjustment, reason, relatedID string) LedgerMovement {
	row := movementRow(input, KindCostAdjustment, zero, adjustment.Value, nil, adjustment.Basis, false)
	row.EntryKey = input.EntryKey + suffix
	row.RelatedMovementID = &relatedID
	row.RestaurantUnitID = input.RestaurantUnitID
	text := reason
	row.Reason = &text
	return row
}

func optionalDecimalText(value *decimal.Decimal) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func insertMovement(ctx context.Context, tx pgx.Tx, row *LedgerMovement) error {
	return tx.QueryRow(ctx, `INSERT INTO app.inventory_movements(
            id,item_id,warehouse_id,kind,quantity,value,unit_cost,cost_basis,cost_pending,occurred_at,source_kind,source_id,
            source_line,restaurant_unit_id,related_movement_id,reason,actor_id,process,operation_id,entry_key,stock_after,
            valuation_quantity_after,valuation_value_after)
        VALUES ($1,$2,$3,$4,$5::numeric,$6::numeric,$7::numeric,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21::numeric,
            $22::numeric,$23::numeric)
        RETURNING seq,recorded_at`,
		row.ID, row.ItemID, row.WarehouseID, row.Kind, row.Quantity.String(), row.Value.String(), optionalDecimalText(row.UnitCost),
		row.CostBasis, row.CostPending, row.OccurredAt, row.SourceKind, row.SourceID, row.SourceLine, row.RestaurantUnitID,
		row.RelatedMovementID, row.Reason, row.ActorID, row.Process, row.OperationID, row.EntryKey,
		optionalDecimalText(row.StockAfter), row.ValuationQuantityAfter.String(), row.ValuationValueAfter.String(),
	).Scan(&row.Seq, &row.RecordedAt)
}

const ledgerColumns = `SELECT id,seq,item_id,warehouse_id,kind,quantity::text,value::text,unit_cost::text,cost_basis,cost_pending,
        occurred_at,recorded_at,source_kind,source_id,source_line,restaurant_unit_id,related_movement_id,reason,actor_id,process,
        operation_id,entry_key,stock_after::text,valuation_quantity_after::text,valuation_value_after::text
        FROM app.inventory_movements`

func scanLedgerMovement(row interface{ Scan(...any) error }) (LedgerMovement, error) {
	var movement LedgerMovement
	var quantity, value, valuationQuantity, valuationValue string
	var unitCost, stockAfter *string
	err := row.Scan(&movement.ID, &movement.Seq, &movement.ItemID, &movement.WarehouseID, &movement.Kind, &quantity, &value,
		&unitCost, &movement.CostBasis, &movement.CostPending, &movement.OccurredAt, &movement.RecordedAt, &movement.SourceKind,
		&movement.SourceID, &movement.SourceLine, &movement.RestaurantUnitID, &movement.RelatedMovementID, &movement.Reason,
		&movement.ActorID, &movement.Process, &movement.OperationID, &movement.EntryKey, &stockAfter, &valuationQuantity,
		&valuationValue)
	if err != nil {
		return LedgerMovement{}, err
	}
	movement.Quantity = parseDatabaseDecimal(quantity)
	movement.Value = parseDatabaseDecimal(value)
	movement.UnitCost = parseDatabaseOptional(unitCost)
	movement.StockAfter = parseDatabaseOptional(stockAfter)
	movement.ValuationQuantityAfter = parseDatabaseDecimal(valuationQuantity)
	movement.ValuationValueAfter = parseDatabaseDecimal(valuationValue)
	return movement, nil
}

// replayEntry devuelve una entrada ya registrada con sus ajustes, sin aplicar nada.
func replayEntry(ctx context.Context, tx pgx.Tx, key string) (LedgerResult, bool, error) {
	main, err := scanLedgerMovement(tx.QueryRow(ctx, ledgerColumns+" WHERE entry_key=$1", key))
	if isNoRows(err) {
		return LedgerResult{}, false, nil
	}
	if err != nil {
		return LedgerResult{}, false, err
	}
	result := LedgerResult{Movement: main, Replayed: true}
	if main.StockAfter != nil {
		result.StockAfter = *main.StockAfter
		result.NegativeStock = main.StockAfter.IsNegative()
	}
	rows, err := tx.Query(ctx, ledgerColumns+" WHERE entry_key IN ($1,$2) ORDER BY seq", key+":cost", key+":residual")
	if err != nil {
		return LedgerResult{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		adjustment, err := scanLedgerMovement(rows)
		if err != nil {
			return LedgerResult{}, false, err
		}
		result.Adjustments = append(result.Adjustments, adjustment)
	}
	if err := rows.Err(); err != nil {
		return LedgerResult{}, false, err
	}
	last := main
	if len(result.Adjustments) > 0 {
		last = result.Adjustments[len(result.Adjustments)-1]
	}
	result.Valuation = Valuation{Quantity: last.ValuationQuantityAfter, Value: last.ValuationValueAfter}
	return result, true, nil
}
