package inventory

import (
	"errors"

	"github.com/shopspring/decimal"
)

// Motor de costo promedio ponderado móvil (etapa 8, §0-B «Motor de costos» y §15 de la
// especificación; reglas 1–8 del §5 del diseño). Es una función pura sobre decimales: no lee ni
// escribe la base de datos. El libro (ledger.go) le entrega el estado del insumo bloqueado y
// registra lo que devuelve.
//
// El costo promedio es uno solo por insumo (no por almacén): el estado guarda la cantidad total Q
// y el valor total V, y el promedio es V/Q cuando Q > 0. No se guarda solo el costo unitario
// porque el redondeo acumulado lo desviaría. Ningún movimiento histórico se reescribe: toda
// diferencia es un movimiento nuevo (ajuste de costo o residuo).
//
// Redondeo: valores a 6 decimales y costos unitarios a 10, mitad alejándose de cero
// (decimal.DivRound redondea el cociente exacto, sin redondeo doble).
//
// Invariantes que mantiene toda operación: Q = 0 ⇒ V = 0; Q > 0 ⇒ V ≥ 0; L es nulo o mayor que
// cero; 0 ≤ cantidad pendiente ≤ déficit.

// errCostRequired rechaza una entrada sin documento (ajuste positivo o sobrante de conteo) que
// dejaría existencia positiva sin ningún costo disponible (§0-B.6: «nunca a un costo inventado»). El
// libro la convierte en el rechazo público COST_REQUIRED con el nombre del insumo.
var errCostRequired = errors.New("inventory: entrada sin costo disponible")

// Bases de costo de un movimiento (inventory_movements.cost_basis).
const (
	basisAverage     = "average"     // promedio vigente V/Q
	basisReference   = "reference"   // último costo promedio válido (L)
	basisConfigured  = "configured"  // costo de referencia configurado en el insumo (R)
	basisPending     = "pending"     // sin ningún costo: valor cero y «costo pendiente»
	basisDocument    = "document"    // costo del documento (recepción, existencia inicial)
	basisOriginal    = "original"    // valor exacto del movimiento que se compensa
	basisProvisional = "provisional" // valor provisional del déficit acumulado
	basisDifference  = "difference"  // ajuste de costo por diferencia o residuo de valuación
)

// Valuation es el estado de costo de un insumo, común a todos sus almacenes.
type Valuation struct {
	// Quantity (Q) es la cantidad total en la unidad base: Σ existencias del insumo.
	Quantity decimal.Decimal
	// Value (V) es el valor total; puede ser negativo junto con la cantidad.
	Value decimal.Decimal
	// LastValidCost (L) es el último promedio V/Q con existencia y valor positivos.
	LastValidCost *decimal.Decimal
	// ReferenceCost (R) es el costo de referencia configurado en el insumo (no lo cambia el motor).
	ReferenceCost *decimal.Decimal
	// PendingQuantity es la parte del déficit (Q < 0) valorada en cero por falta de costo.
	PendingQuantity decimal.Decimal
}

// CostAdjustment es un movimiento de solo valor (cantidad 0) que acompaña a otro.
type CostAdjustment struct {
	Value decimal.Decimal
	Basis string
}

// Entry es el resultado de aplicar una operación al estado del insumo.
type Entry struct {
	// Value es el valor con signo del movimiento principal: positivo en entradas y negativo en
	// salidas.
	Value decimal.Decimal
	// UnitCost es |Value| / cantidad (10 decimales); nil cuando la salida no tiene costo.
	UnitCost *decimal.Decimal
	Basis    string
	// CostPending marca que una parte del movimiento se valoró en cero por falta de costo.
	CostPending bool
	// Adjustment es el ajuste de costo separado de una entrada con documento o de una reversión de
	// consumo que cubren un déficit (regla 2) o de una devolución a proveedor (regla 5).
	Adjustment *CostAdjustment
	// Residual es el residuo de valuación que mantiene el invariante Q = 0 ⇒ V = 0 (regla 6).
	Residual *CostAdjustment
	// After es el estado final del insumo, con los ajustes incluidos.
	After Valuation
}

// ReturnEntry agrega a la salida de una devolución a proveedor el valor del documento y la parte
// de la diferencia que no pudo absorber la existencia restante.
type ReturnEntry struct {
	Entry
	// DocumentValue es q × costo original: el valor que acredita el proveedor.
	DocumentValue decimal.Decimal
	// Unabsorbed es la diferencia (con signo) que se guarda en la devolución sin movimiento.
	Unabsorbed decimal.Decimal
}

// TransferEntry describe una transferencia entre almacenes: el mismo valor sale de uno y entra al
// otro, por lo que Q y V del insumo no cambian.
type TransferEntry struct {
	// Value es el valor positivo que se traslada (salida −Value, entrada +Value).
	Value       decimal.Decimal
	UnitCost    *decimal.Decimal
	Basis       string
	CostPending bool
	After       Valuation
}

// reference devuelve el costo de referencia vigente: el último promedio válido o, si nunca hubo,
// el costo configurado en el insumo. Sin ninguno, nil y «pendiente».
func (state Valuation) reference() (*decimal.Decimal, string) {
	if state.LastValidCost != nil {
		return state.LastValidCost, basisReference
	}
	if state.ReferenceCost != nil {
		return state.ReferenceCost, basisConfigured
	}
	return nil, basisPending
}

// deficit es la cantidad faltante cuando la existencia total es negativa (cero si no lo es).
func (state Valuation) deficit() decimal.Decimal {
	if state.Quantity.IsNegative() {
		return state.Quantity.Neg()
	}
	return zero
}

// proportion es la parte proporcional del valor total de una existencia positiva: q × V / Q,
// redondeada a 6 decimales. Si q es toda la existencia, es exactamente V (sin residuo).
func (state Valuation) proportion(quantity decimal.Decimal) decimal.Decimal {
	if quantity.Equal(state.Quantity) {
		return state.Value
	}
	return quantity.Mul(state.Value).DivRound(state.Quantity, valueScale)
}

// copyDecimal devuelve un puntero independiente para que el estado nuevo no comparta memoria con
// el anterior.
func copyDecimal(value *decimal.Decimal) *decimal.Decimal {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func (state Valuation) clone() Valuation {
	return Valuation{
		Quantity:        state.Quantity,
		Value:           state.Value,
		LastValidCost:   copyDecimal(state.LastValidCost),
		ReferenceCost:   copyDecimal(state.ReferenceCost),
		PendingQuantity: state.PendingQuantity,
	}
}

// unitCostOf es |valor| / cantidad con 10 decimales.
func unitCostOf(value, quantity decimal.Decimal) *decimal.Decimal {
	cost := value.Abs().DivRound(quantity, costScale)
	return &cost
}

func minimum(first, second decimal.Decimal) decimal.Decimal {
	if first.LessThan(second) {
		return first
	}
	return second
}

func requirePositive(quantity decimal.Decimal) error {
	if !quantity.IsPositive() {
		return invalid("La cantidad del movimiento debe ser mayor que cero.")
	}
	return nil
}

// settle aplica las reglas que siguen a toda operación:
//   - regla 6: si la cantidad quedó en cero y el valor no, emite el residuo −V;
//   - la cantidad pendiente de costo nunca excede el déficit vigente;
//   - regla 7: el último costo válido pasa a ser V/Q cuando Q > 0 y V > 0, solo si el promedio
//     redondeado a 10 decimales es mayor que cero (un valor diminuto sobre una cantidad grande
//     redondea a cero, que no es un costo válido y violaría last_valid_cost > 0). Un promedio de
//     cero no reemplaza a L.
//
// Las reglas 2 y 3 ya dejan el valor exactamente en cero cuando una entrada cubre todo el déficit,
// por lo que el residuo es una red de seguridad del invariante.
func settle(entry *Entry, after Valuation) {
	if after.Quantity.IsZero() && !after.Value.IsZero() {
		entry.Residual = &CostAdjustment{Value: after.Value.Neg(), Basis: basisDifference}
		after.Value = zero
	}
	if after.PendingQuantity.GreaterThan(after.deficit()) {
		after.PendingQuantity = after.deficit()
	}
	if after.PendingQuantity.IsNegative() {
		after.PendingQuantity = zero
	}
	if after.Quantity.IsPositive() && after.Value.IsPositive() {
		if average := after.Value.DivRound(after.Quantity, costScale); average.IsPositive() {
			after.LastValidCost = &average
		}
	}
	entry.After = after
}

// Issue registra una salida de q > 0 (consumo, merma, ajuste negativo, faltante de conteo,
// consumo de una elaboración y la salida de una devolución). Regla 1:
//   - Q ≥ q: valor q × V/Q (exactamente V si q = Q), base «average»;
//   - 0 < Q < q (cruza a negativo): la parte Q vale V exacto y el resto se valora al costo de
//     referencia vigente (base «reference» o «configured») o queda pendiente (valor cero);
//   - Q ≤ 0: q × referencia; sin referencia, valor cero, «costo pendiente» y la cantidad se suma
//     a la pendiente de costo.
func Issue(state Valuation, quantity decimal.Decimal) (Entry, error) {
	if err := requirePositive(quantity); err != nil {
		return Entry{}, err
	}
	after := state.clone()
	entry := Entry{}
	var value decimal.Decimal
	switch {
	case state.Quantity.GreaterThanOrEqual(quantity):
		value = state.proportion(quantity)
		entry.Basis = basisAverage
	case state.Quantity.IsPositive():
		remainder := quantity.Sub(state.Quantity)
		value = state.Value
		reference, basis := state.reference()
		if reference != nil {
			value = value.Add(remainder.Mul(*reference).Round(valueScale))
			entry.Basis = basis
		} else {
			entry.Basis = basisAverage
			entry.CostPending = true
			after.PendingQuantity = after.PendingQuantity.Add(remainder)
		}
	default:
		reference, basis := state.reference()
		entry.Basis = basis
		if reference != nil {
			value = quantity.Mul(*reference).Round(valueScale)
		} else {
			value = zero
			entry.CostPending = true
			after.PendingQuantity = after.PendingQuantity.Add(quantity)
		}
	}
	entry.Value = value.Neg()
	if entry.Basis != basisPending {
		entry.UnitCost = unitCostOf(value, quantity)
	}
	after.Quantity = state.Quantity.Sub(quantity)
	after.Value = state.Value.Sub(value)
	settle(&entry, after)
	return entry, nil
}

// Receive registra una entrada con documento (recepción, existencia inicial o entrada de una
// elaboración, regla 4) de q > 0 con el valor total del documento (q × costo; en una elaboración,
// el valor de lo consumido). Regla 2:
//   - Q ≥ 0: V += valor, Q += q (promedio ponderado normal);
//   - Q < 0: las d = min(q, −Q) unidades que cubren el déficit se valoran al costo real del
//     documento. Su valor provisional era d × V/Q (exactamente −V si d = −Q). La diferencia
//     provisional − real se registra como ajuste de costo separado (negativo si el costo real
//     fue mayor) y la cantidad pendiente de costo baja en min(pendiente, d).
func Receive(state Valuation, quantity, documentValue decimal.Decimal) (Entry, error) {
	if err := requirePositive(quantity); err != nil {
		return Entry{}, err
	}
	if documentValue.IsNegative() {
		return Entry{}, invalid("El valor de la entrada no puede ser negativo.")
	}
	value := documentValue.Round(valueScale)
	after := state.clone()
	entry := Entry{Value: value, UnitCost: unitCostOf(value, quantity), Basis: basisDocument}
	after.Quantity = state.Quantity.Add(quantity)
	after.Value = state.Value.Add(value)
	if state.Quantity.IsNegative() {
		deficit := state.deficit()
		covered := minimum(quantity, deficit)
		provisional := provisionalValue(state, covered)
		actual := value
		if !covered.Equal(quantity) {
			actual = value.Mul(covered).DivRound(quantity, valueScale)
		}
		difference := provisional.Sub(actual)
		if !difference.IsZero() {
			entry.Adjustment = &CostAdjustment{Value: difference, Basis: basisDifference}
			after.Value = after.Value.Add(difference)
		}
		after.PendingQuantity = after.PendingQuantity.Sub(minimum(after.PendingQuantity, covered))
	}
	settle(&entry, after)
	return entry, nil
}

// provisionalValue es el valor provisional (positivo) de d unidades del déficit: d × V/Q, o
// exactamente −V cuando d cubre todo el déficit.
func provisionalValue(state Valuation, covered decimal.Decimal) decimal.Decimal {
	if covered.Equal(state.deficit()) {
		return state.Value.Neg()
	}
	return covered.Mul(state.Value).DivRound(state.Quantity, valueScale)
}

// Admit registra una entrada sin costo propio: ajuste positivo o sobrante de conteo (regla 3).
//   - Q > 0: al promedio vigente V/Q;
//   - Q ≤ 0: el déficit cubierto d = min(q, −Q) se valora a su valor provisional d × V/Q (exacto
//     si cubre todo) y el resto al costo de referencia vigente (L o, si nunca hubo, R).
//
// Si el resto (la parte que queda en existencia positiva) no tiene ningún costo disponible, la
// entrada se rechaza con errCostRequired: nunca se inventa un costo ni entra existencia positiva a
// valor cero sin marca (§0-B.6). Cubrir solo el déficit sigue permitido, al provisional.
//
// Interpretación de §0-B.6 que conserva exacto el valor: cubrir el déficit al provisional deja el
// promedio del déficit restante sin cambio y, si lo cubre todo, el valor vuelve exactamente a cero.
func Admit(state Valuation, quantity decimal.Decimal) (Entry, error) {
	if err := requirePositive(quantity); err != nil {
		return Entry{}, err
	}
	after := state.clone()
	entry := Entry{}
	var value decimal.Decimal
	if state.Quantity.IsPositive() {
		value = quantity.Mul(state.Value).DivRound(state.Quantity, valueScale)
		entry.Basis = basisAverage
	} else {
		covered := minimum(quantity, state.deficit())
		remainder := quantity.Sub(covered)
		value = zero
		if covered.IsPositive() {
			value = provisionalValue(state, covered)
			entry.Basis = basisProvisional
		}
		if remainder.IsPositive() {
			reference, basis := state.reference()
			if reference == nil {
				return Entry{}, errCostRequired
			}
			value = value.Add(remainder.Mul(*reference).Round(valueScale))
			if covered.IsZero() {
				entry.Basis = basis
			}
		}
		after.PendingQuantity = after.PendingQuantity.Sub(minimum(after.PendingQuantity, covered))
	}
	entry.Value = value
	entry.UnitCost = unitCostOf(value, quantity)
	after.Quantity = state.Quantity.Add(quantity)
	after.Value = state.Value.Add(value)
	settle(&entry, after)
	return entry, nil
}

// Restore registra la reversión de un consumo (consumption_reversal). El movimiento conserva
// exactamente el valor original del consumo que compensa (así el consumo neto de la partida sigue
// siendo exacto) y se aplica como una entrada con documento (regla 2, Receive): con Q < 0, las
// d = min(q, −Q) unidades que cubren el déficit se realinean con un ajuste de costo separado
// (provisional de d − valor original × d / q) y la cantidad pendiente de costo baja en
// min(pendiente, d). Sin ese ajuste, sumar el valor original a un valor negativo acumulado a otro
// costo distorsionaría el promedio o dejaría V < 0 con Q > 0. Base «original».
func Restore(state Valuation, quantity, value decimal.Decimal) (Entry, error) {
	if err := requirePositive(quantity); err != nil {
		return Entry{}, err
	}
	if value.IsNegative() {
		return Entry{}, invalid("El valor de la reversión no puede ser negativo.")
	}
	entry, err := Receive(state, quantity, value)
	if err != nil {
		return Entry{}, err
	}
	entry.Basis = basisOriginal
	return entry, nil
}

// Transfer valora una transferencia de q > 0 entre almacenes. La salida y la entrada llevan el
// mismo valor (al promedio si Q > 0; si no, al costo de referencia; sin costo, cero pendiente), de
// modo que la cantidad, el valor, el promedio y la cantidad pendiente del insumo no cambian.
func Transfer(state Valuation, quantity decimal.Decimal) (TransferEntry, error) {
	if err := requirePositive(quantity); err != nil {
		return TransferEntry{}, err
	}
	entry := TransferEntry{After: state.clone()}
	if state.Quantity.IsPositive() {
		entry.Value = quantity.Mul(state.Value).DivRound(state.Quantity, valueScale)
		entry.Basis = basisAverage
	} else {
		reference, basis := state.reference()
		entry.Basis = basis
		if reference != nil {
			entry.Value = quantity.Mul(*reference).Round(valueScale)
		} else {
			entry.Value = zero
			entry.CostPending = true
		}
	}
	if entry.Basis != basisPending {
		entry.UnitCost = unitCostOf(entry.Value, quantity)
	}
	return entry, nil
}

// ReturnToSupplier registra una devolución de q > 0 unidades con su costo unitario original c0
// (regla 5). La salida es una salida normal (regla 1) con valor va. La diferencia q × c0 − va se
// registra como ajuste de costo ligado a la devolución solo si la existencia restante es positiva,
// limitado para que el valor no quede negativo; lo que no se absorbe se devuelve como diferencia no
// absorbida, que la devolución guarda sin movimiento. El valor de la devolución es q × c0.
func ReturnToSupplier(state Valuation, quantity, originalCost decimal.Decimal) (ReturnEntry, error) {
	if originalCost.IsNegative() {
		return ReturnEntry{}, invalid("El costo original de la devolución no puede ser negativo.")
	}
	issued, err := Issue(state, quantity)
	if err != nil {
		return ReturnEntry{}, err
	}
	result := ReturnEntry{Entry: issued, DocumentValue: quantity.Mul(originalCost).Round(valueScale)}
	outflow := issued.Value.Neg()
	difference := result.DocumentValue.Sub(outflow)
	after := issued.After
	if !after.Quantity.IsPositive() || difference.IsZero() {
		result.Unabsorbed = difference
		return result, nil
	}
	// La existencia restante absorbe la diferencia: si se devolvió a un costo mayor que el promedio,
	// lo que queda pierde valor (ajuste negativo), sin llegar a un valor negativo.
	adjustment := difference.Neg()
	if after.Value.Add(adjustment).IsNegative() {
		adjustment = after.Value.Neg()
	}
	result.Unabsorbed = difference.Add(adjustment)
	if !adjustment.IsZero() {
		result.Adjustment = &CostAdjustment{Value: adjustment, Basis: basisDifference}
		after.Value = after.Value.Add(adjustment)
		settle(&result.Entry, after)
	}
	return result, nil
}
