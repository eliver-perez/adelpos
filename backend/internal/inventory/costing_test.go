package inventory

import (
	"testing"

	"github.com/shopspring/decimal"
)

// Pruebas del motor de costos (§0-B «Motor de costos» de la especificación y §5 del diseño). Cada
// caso anota el cálculo hecho a mano; valores a 6 decimales y costos a 10, mitad alejándose de cero.

func number(text string) decimal.Decimal {
	return decimal.RequireFromString(text)
}

func pointer(text string) *decimal.Decimal {
	value := number(text)
	return &value
}

func state(quantity, value string) Valuation {
	return Valuation{Quantity: number(quantity), Value: number(value), PendingQuantity: zero}
}

func expectDecimal(t *testing.T, label string, actual decimal.Decimal, expected string) {
	t.Helper()
	if !actual.Equal(number(expected)) {
		t.Fatalf("%s: esperado %s, obtenido %s", label, expected, actual.String())
	}
}

func expectOptional(t *testing.T, label string, actual *decimal.Decimal, expected string) {
	t.Helper()
	if expected == "" {
		if actual != nil {
			t.Fatalf("%s: esperado sin valor, obtenido %s", label, actual.String())
		}
		return
	}
	if actual == nil {
		t.Fatalf("%s: esperado %s, obtenido sin valor", label, expected)
	}
	expectDecimal(t, label, *actual, expected)
}

// expectState compara Q, V, L y la cantidad pendiente de costo.
func expectState(t *testing.T, label string, actual Valuation, quantity, value, last, pending string) {
	t.Helper()
	expectDecimal(t, label+" Q", actual.Quantity, quantity)
	expectDecimal(t, label+" V", actual.Value, value)
	expectOptional(t, label+" L", actual.LastValidCost, last)
	expectDecimal(t, label+" pendiente", actual.PendingQuantity, pending)
}

// mustEntry recibe directamente el resultado de una operación del motor:
// mustEntry(t)(Issue(estado, cantidad)).
func mustEntry(t *testing.T) func(Entry, error) Entry {
	return func(entry Entry, err error) Entry {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return entry
	}
}

// Ejemplo del §15: 10 u a $20 ($200) + 10 u a $30 ($300) = 20 u, $500, promedio $25.
func TestCostingWeightedAverageWithPositiveStock(t *testing.T) {
	first := mustEntry(t)(Receive(state("0", "0"), number("10"), number("200")))
	expectDecimal(t, "valor de la primera entrada", first.Value, "200")
	expectOptional(t, "costo de la primera entrada", first.UnitCost, "20")
	expectState(t, "tras 10 u a $20", first.After, "10", "200", "20", "0")
	second := mustEntry(t)(Receive(first.After, number("10"), number("300")))
	if second.Basis != basisDocument || second.Adjustment != nil {
		t.Fatalf("una entrada con existencia positiva no lleva ajuste: %+v", second)
	}
	// 200 + 300 = 500 con 20 u: promedio 500 / 20 = 25.
	expectState(t, "tras 10 u a $30", second.After, "20", "500", "25", "0")
}

// Salida que deja la existencia exactamente en cero: el valor es V completo, sin residuo.
func TestCostingIssueToExactZero(t *testing.T) {
	// 3 u por $10: la primera salida de 1 u vale 10/3 = 3.333333 (redondeo hacia abajo) y deja
	// 2 u por 6.666667; la segunda de 2 u es toda la existencia y vale exactamente 6.666667.
	start := state("3", "10")
	start.LastValidCost = pointer("3.3333333333")
	first := mustEntry(t)(Issue(start, number("1")))
	expectDecimal(t, "primera salida", first.Value, "-3.333333")
	expectState(t, "tras la primera salida", first.After, "2", "6.666667", "3.3333335", "0")
	last := mustEntry(t)(Issue(first.After, number("2")))
	expectDecimal(t, "salida final", last.Value, "-6.666667")
	if last.Residual != nil || last.Basis != basisAverage {
		t.Fatalf("la salida exacta no debe generar residuo: %+v", last)
	}
	// La cantidad queda en cero con valor cero; L conserva el último promedio válido.
	expectState(t, "existencia en cero", last.After, "0", "0", "3.3333335", "0")
}

// Salida que cruza a negativo: la parte existente vale V y el resto se valora a L.
func TestCostingIssueCrossingToNegative(t *testing.T) {
	// 2 u por $50 (L = 25); salida de 5 u: 50 + 3 × 25 = 125, base «reference». Queda −3 u y −75.
	start := state("2", "50")
	start.LastValidCost = pointer("25")
	entry := mustEntry(t)(Issue(start, number("5")))
	expectDecimal(t, "valor de la salida", entry.Value, "-125")
	expectOptional(t, "costo de la salida", entry.UnitCost, "25")
	if entry.Basis != basisReference || entry.CostPending {
		t.Fatalf("la parte negativa debió valorarse a L: %+v", entry)
	}
	expectState(t, "tras cruzar", entry.After, "-3", "-75", "25", "0")
}

// Salida con existencia negativa: al último costo válido L.
func TestCostingIssueWithNegativeStockAtLastValidCost(t *testing.T) {
	// −3 u por −75 con L = 25; salida de 2 u: 2 × 25 = 50. Queda −5 u y −125.
	start := state("-3", "-75")
	start.LastValidCost = pointer("25")
	start.ReferenceCost = pointer("40")
	entry := mustEntry(t)(Issue(start, number("2")))
	expectDecimal(t, "valor de la salida", entry.Value, "-50")
	if entry.Basis != basisReference {
		t.Fatalf("con L, el costo configurado no interviene: %s", entry.Basis)
	}
	expectState(t, "tras la salida", entry.After, "-5", "-125", "25", "0")
}

// Salida sin existencia y sin L: al costo de referencia configurado R.
func TestCostingIssueWithConfiguredReferenceOnly(t *testing.T) {
	// 0 u, sin L, R = 18; salida de 4 u: 4 × 18 = 72, base «configured». Queda −4 u y −72.
	start := state("0", "0")
	start.ReferenceCost = pointer("18")
	entry := mustEntry(t)(Issue(start, number("4")))
	expectDecimal(t, "valor de la salida", entry.Value, "-72")
	if entry.Basis != basisConfigured || entry.CostPending {
		t.Fatalf("debió valorarse al costo configurado: %+v", entry)
	}
	// Sin existencia positiva no hay último costo válido.
	expectState(t, "tras la salida", entry.After, "-4", "-72", "", "0")
}

// Varias salidas negativas con costos de referencia distintos antes de la recepción.
func TestCostingSeveralNegativeIssuesBeforeReceipt(t *testing.T) {
	// R = 18: salida de 4 u = 72 → −4 u, −72.
	start := state("0", "0")
	start.ReferenceCost = pointer("18")
	first := mustEntry(t)(Issue(start, number("4")))
	// El costo configurado cambia a 20: salida de 2 u = 40 → −6 u, −112.
	changed := first.After
	changed.ReferenceCost = pointer("20")
	second := mustEntry(t)(Issue(changed, number("2")))
	expectDecimal(t, "segunda salida", second.Value, "-40")
	expectState(t, "antes de la recepción", second.After, "-6", "-112", "", "0")
	// Recepción de 10 u a $25 ($250): cubre el déficit completo (d = 6), provisional = −V = 112;
	// real = 250 × 6 / 10 = 150; ajuste = 112 − 150 = −38. V = −112 + 250 − 38 = 100 con 4 u.
	receipt := mustEntry(t)(Receive(second.After, number("10"), number("250")))
	expectDecimal(t, "valor de la recepción", receipt.Value, "250")
	if receipt.Adjustment == nil {
		t.Fatal("la recepción con déficit debe llevar un ajuste de costo")
	}
	expectDecimal(t, "ajuste de costo", receipt.Adjustment.Value, "-38")
	expectState(t, "tras la recepción", receipt.After, "4", "100", "25", "0")
}

// Recepción que cubre solo parte del déficit: el promedio del déficit restante no cambia.
func TestCostingReceiptCoveringPartOfDeficit(t *testing.T) {
	// −6 u por −112 (promedio provisional 18.666…). Recepción de 4 u a $25 ($100): d = 4;
	// provisional = 4 × (−112) / (−6) = 74.6666666… → 74.666667; real = 100 (d = q);
	// ajuste = 74.666667 − 100 = −25.333333. V = −112 + 100 − 25.333333 = −37.333333 con −2 u.
	start := state("-6", "-112")
	start.ReferenceCost = pointer("20")
	partial := mustEntry(t)(Receive(start, number("4"), number("100")))
	expectDecimal(t, "ajuste parcial", partial.Adjustment.Value, "-25.333333")
	expectState(t, "déficit restante", partial.After, "-2", "-37.333333", "", "0")
	// Segunda recepción de 2 u a $25 ($50): cubre el resto exacto, provisional = −V = 37.333333;
	// ajuste = 37.333333 − 50 = −12.666667. V = 0 con 0 u. Total de ajustes −38, igual que una sola
	// recepción.
	rest := mustEntry(t)(Receive(partial.After, number("2"), number("50")))
	expectDecimal(t, "ajuste final", rest.Adjustment.Value, "-12.666667")
	if rest.Residual != nil {
		t.Fatal("cubrir el déficit exacto no deja residuo")
	}
	expectState(t, "déficit cubierto", rest.After, "0", "0", "", "0")
}

// Ejemplo del §15: −5 u (último costo $20) + recepción de 8 u a $30 → 3 u a $30, ajuste −$50.
func TestCostingReceiptCrossingToPositive(t *testing.T) {
	// −5 u por −100 (L = 20). Recepción de 8 u por $240: d = 5 = déficit, provisional = 100;
	// real = 240 × 5 / 8 = 150; ajuste = 100 − 150 = −50. V = −100 + 240 − 50 = 90 con 3 u = $30.
	start := state("-5", "-100")
	start.LastValidCost = pointer("20")
	entry := mustEntry(t)(Receive(start, number("8"), number("240")))
	expectDecimal(t, "valor de la recepción", entry.Value, "240")
	expectOptional(t, "costo de la recepción", entry.UnitCost, "30")
	expectDecimal(t, "ajuste de costo", entry.Adjustment.Value, "-50")
	if entry.Adjustment.Basis != basisDifference {
		t.Fatalf("base del ajuste: %s", entry.Adjustment.Basis)
	}
	expectState(t, "tras cruzar a positivo", entry.After, "3", "90", "30", "0")
}

// Costo pendiente: sin L ni R la salida vale cero y se resuelve por diferencia al recibir.
func TestCostingPendingCostResolvedByDifference(t *testing.T) {
	// Sin costo: salida de 5 u con valor 0, «costo pendiente», 5 u pendientes.
	pending := mustEntry(t)(Issue(state("0", "0"), number("5")))
	expectDecimal(t, "salida pendiente", pending.Value, "0")
	if !pending.CostPending || pending.Basis != basisPending || pending.UnitCost != nil {
		t.Fatalf("la salida sin costo debe quedar pendiente: %+v", pending)
	}
	expectState(t, "pendiente", pending.After, "-5", "0", "", "5")
	// Recepción de 8 u por $240: d = 5, provisional = −V = 0; real = 240 × 5 / 8 = 150;
	// ajuste = 0 − 150 = −150 (el consumo pendiente se valora por diferencia sin reescribirlo).
	// V = 0 + 240 − 150 = 90 con 3 u ($30) y ya nada pendiente.
	receipt := mustEntry(t)(Receive(pending.After, number("8"), number("240")))
	expectDecimal(t, "ajuste por diferencia", receipt.Adjustment.Value, "-150")
	expectState(t, "resuelto", receipt.After, "3", "90", "30", "0")
}

// Una recepción que cubre solo parte de un déficit pendiente reduce la cantidad pendiente.
func TestCostingPartialReceiptReducesPendingQuantity(t *testing.T) {
	// −5 u pendientes (V = 0). Recepción de 3 u a $10 ($30): d = 3, provisional = 3 × 0 / −5 = 0;
	// ajuste = 0 − 30 = −30; V = 0 con −2 u; pendiente 5 − 3 = 2.
	start := state("-5", "0")
	start.PendingQuantity = number("5")
	entry := mustEntry(t)(Receive(start, number("3"), number("30")))
	expectDecimal(t, "ajuste", entry.Adjustment.Value, "-30")
	expectState(t, "pendiente parcial", entry.After, "-2", "0", "", "2")
	// La siguiente salida sigue sin costo (Q < 0 y sin L ni R): suma al pendiente.
	next := mustEntry(t)(Issue(entry.After, number("1")))
	expectState(t, "nueva salida pendiente", next.After, "-3", "0", "", "3")
}

// Ajuste positivo con existencia positiva y con existencia negativa.
func TestCostingPositiveAdjustments(t *testing.T) {
	// Positiva: 20 u por $500 (promedio 25); ajuste de +4 u = 4 × 25 = 100 → 24 u, $600.
	positive := mustEntry(t)(Admit(state("20", "500"), number("4")))
	expectDecimal(t, "ajuste con existencia positiva", positive.Value, "100")
	if positive.Basis != basisAverage {
		t.Fatalf("base: %s", positive.Basis)
	}
	expectState(t, "tras el ajuste", positive.After, "24", "600", "25", "0")
	// Negativa: −5 u por −100 (L = 20); ajuste de +8 u: el déficit d = 5 vale su provisional exacto
	// −V = 100 y las 3 u restantes a L: 3 × 20 = 60. Valor 160 (base «provisional») → 3 u por $60.
	start := state("-5", "-100")
	start.LastValidCost = pointer("20")
	negative := mustEntry(t)(Admit(start, number("8")))
	expectDecimal(t, "ajuste con existencia negativa", negative.Value, "160")
	if negative.Basis != basisProvisional || negative.Adjustment != nil {
		t.Fatalf("el ajuste positivo no lleva ajuste de costo separado: %+v", negative)
	}
	expectState(t, "tras cubrir el déficit", negative.After, "3", "60", "20", "0")
	// Sin existencia ni costo alguno: valor cero y «costo pendiente»; nunca se inventa un costo.
	blind := mustEntry(t)(Admit(state("0", "0"), number("2")))
	expectDecimal(t, "ajuste sin costo", blind.Value, "0")
	if !blind.CostPending || blind.Basis != basisPending {
		t.Fatalf("el ajuste sin costo debe quedar pendiente: %+v", blind)
	}
	// Con Q > 0 y V = 0 el promedio de cero no es un costo válido: L sigue sin valor.
	expectState(t, "existencia sin costo", blind.After, "2", "0", "", "0")
}

// Transferencia entre almacenes: el mismo valor sale y entra; el costo del insumo no cambia.
func TestCostingTransferKeepsCost(t *testing.T) {
	// 20 u por $500: transferir 5 u traslada 5 × 25 = 125; Q, V y L quedan igual.
	start := state("20", "500")
	start.LastValidCost = pointer("25")
	transfer, err := Transfer(start, number("5"))
	if err != nil {
		t.Fatal(err)
	}
	expectDecimal(t, "valor trasladado", transfer.Value, "125")
	expectOptional(t, "costo trasladado", transfer.UnitCost, "25")
	expectState(t, "tras transferir", transfer.After, "20", "500", "25", "0")
	// Sin existencia total positiva se traslada al costo de referencia, también sin cambios.
	negative := state("-2", "-50")
	negative.LastValidCost = pointer("25")
	shifted, err := Transfer(negative, number("1"))
	if err != nil {
		t.Fatal(err)
	}
	expectDecimal(t, "valor trasladado en negativo", shifted.Value, "25")
	expectState(t, "existencia negativa intacta", shifted.After, "-2", "-50", "25", "0")
}

// Devolución a proveedor: la diferencia contra el costo original la absorbe la existencia restante.
func TestCostingSupplierReturnAbsorbedDifference(t *testing.T) {
	// 20 u por $500 (promedio 25). Devolución de 4 u con costo original $30: salida 4 × 25 = 100;
	// valor de la devolución 4 × 30 = 120; diferencia 20. Quedan 16 u por 400 y el ajuste −20 las
	// deja en 380 (promedio 23.75). Nada sin absorber.
	start := state("20", "500")
	start.LastValidCost = pointer("25")
	entry, err := ReturnToSupplier(start, number("4"), number("30"))
	if err != nil {
		t.Fatal(err)
	}
	expectDecimal(t, "salida de la devolución", entry.Value, "-100")
	expectDecimal(t, "valor de la devolución", entry.DocumentValue, "120")
	expectDecimal(t, "ajuste de la devolución", entry.Adjustment.Value, "-20")
	expectDecimal(t, "sin absorber", entry.Unabsorbed, "0")
	expectState(t, "tras la devolución", entry.After, "16", "380", "23.75", "0")
	// Costo original menor que el promedio: la diferencia (−20) aumenta el valor restante.
	cheaper, err := ReturnToSupplier(start, number("4"), number("20"))
	if err != nil {
		t.Fatal(err)
	}
	// Salida 100, devolución 4 × 20 = 80, diferencia −20 → ajuste +20: 16 u por 420 (26.25).
	expectDecimal(t, "ajuste positivo", cheaper.Adjustment.Value, "20")
	expectState(t, "devolución barata", cheaper.After, "16", "420", "26.25", "0")
}

// Devolución cuya diferencia no se puede absorber (sin existencia restante o con valor insuficiente).
func TestCostingSupplierReturnUnabsorbedDifference(t *testing.T) {
	// 4 u por $100; se devuelven las 4 a $30: salida = V exacto 100, devolución 120, diferencia 20.
	// La existencia queda en cero: no hay ajuste y los 20 quedan sin absorber.
	start := state("4", "100")
	start.LastValidCost = pointer("25")
	all, err := ReturnToSupplier(start, number("4"), number("30"))
	if err != nil {
		t.Fatal(err)
	}
	if all.Adjustment != nil {
		t.Fatal("sin existencia restante no hay ajuste de costo")
	}
	expectDecimal(t, "sin absorber total", all.Unabsorbed, "20")
	expectState(t, "existencia agotada", all.After, "0", "0", "25", "0")
	// 5 u por $10 (promedio 2); se devuelven 4 a $30: salida 4 × 2 = 8, devolución 120,
	// diferencia 112. Queda 1 u por 2: el ajuste se limita a −2 para no dejar valor negativo y
	// 110 quedan sin absorber. Con V = 0 el último costo válido no cambia.
	small := state("5", "10")
	small.LastValidCost = pointer("2")
	limited, err := ReturnToSupplier(small, number("4"), number("30"))
	if err != nil {
		t.Fatal(err)
	}
	expectDecimal(t, "salida limitada", limited.Value, "-8")
	expectDecimal(t, "ajuste limitado", limited.Adjustment.Value, "-2")
	expectDecimal(t, "sin absorber limitado", limited.Unabsorbed, "110")
	expectState(t, "valor agotado", limited.After, "1", "0", "2", "0")
}

// Sobrante y faltante de conteo se aplican como ajustes positivo y negativo.
func TestCostingCountSurplusAndShortage(t *testing.T) {
	// 10 u por $250 (promedio 25). Faltante de 2 u: salida 2 × 25 = 50 → 8 u por 200.
	start := state("10", "250")
	start.LastValidCost = pointer("25")
	shortage := mustEntry(t)(Issue(start, number("2")))
	expectDecimal(t, "faltante", shortage.Value, "-50")
	expectState(t, "tras el faltante", shortage.After, "8", "200", "25", "0")
	// Sobrante de 3 u: entrada al promedio 3 × 25 = 75 → 11 u por 275.
	surplus := mustEntry(t)(Admit(shortage.After, number("3")))
	expectDecimal(t, "sobrante", surplus.Value, "75")
	expectState(t, "tras el sobrante", surplus.After, "11", "275", "25", "0")
}

// Invariante Q = 0 ⇒ V = 0: una reversión a valor original que deja la cantidad en cero con valor.
func TestCostingZeroQuantityInvariant(t *testing.T) {
	// −5 u por −100. Se revierte un consumo de 5 u registrado en $150: Q = 0 y V = 50, así que el
	// motor emite un residuo de −50 y el valor queda en cero.
	start := state("-5", "-100")
	start.LastValidCost = pointer("20")
	entry := mustEntry(t)(Restore(start, number("5"), number("150")))
	expectDecimal(t, "reversión", entry.Value, "150")
	if entry.Basis != basisOriginal || entry.Residual == nil {
		t.Fatalf("la reversión debió producir un residuo: %+v", entry)
	}
	expectDecimal(t, "residuo", entry.Residual.Value, "-50")
	expectState(t, "invariante", entry.After, "0", "0", "20", "0")
	// Revertir el consumo exacto que dejó el déficit no produce residuo.
	exact := mustEntry(t)(Restore(start, number("5"), number("100")))
	if exact.Residual != nil {
		t.Fatal("una reversión exacta no deja residuo")
	}
	expectState(t, "reversión exacta", exact.After, "0", "0", "20", "0")
}

// Redondeo con cantidades fraccionarias de 4 a 6 decimales.
func TestCostingRoundsFractionalQuantities(t *testing.T) {
	// 1250.5 g por $87.53: costo 87.53 / 1250.5 = 0.06999600159… → 0.0699960016.
	receipt := mustEntry(t)(Receive(state("0", "0"), number("1250.5"), number("87.53")))
	expectOptional(t, "costo por gramo", receipt.UnitCost, "0.0699960016")
	// Salida de 12.3456 g: 12.3456 × 87.53 / 1250.5 = 1080.610368 / 1250.5 = 0.86414263… → 0.864143.
	first := mustEntry(t)(Issue(receipt.After, number("12.3456")))
	expectDecimal(t, "salida de 12.3456 g", first.Value, "-0.864143")
	// Quedan 1238.1544 g por 86.665857 (promedio 0.06999600131… → 0.0699960013).
	expectState(t, "tras la primera salida", first.After, "1238.1544", "86.665857", "0.0699960013", "0")
	// Salida de 0.000125 g: 0.000125 × 86.665857 / 1238.1544 = 0.0000087495… → 0.000009.
	tiny := mustEntry(t)(Issue(first.After, number("0.000125")))
	expectDecimal(t, "salida mínima", tiny.Value, "-0.000009")
	expectState(t, "tras la salida mínima", tiny.After, "1238.154275", "86.665848", "0.0699960011", "0")
	// Toda la existencia restante sale por su valor exacto: sin residuo.
	rest := mustEntry(t)(Issue(tiny.After, number("1238.154275")))
	expectDecimal(t, "salida final", rest.Value, "-86.665848")
	expectState(t, "existencia agotada", rest.After, "0", "0", "0.0699960011", "0")
	// 3 u por $10, salida de 0.5 u tras 1 u: 0.5 × 6.666667 / 2 = 1.66666675 → 1.666667.
	third := mustEntry(t)(Issue(state("3", "10"), number("1")))
	half := mustEntry(t)(Issue(third.After, number("0.5")))
	expectDecimal(t, "media unidad", half.Value, "-1.666667")
	expectState(t, "tras media unidad", half.After, "1.5", "5", "3.3333333333", "0")
}

// El motor rechaza cantidades no positivas y valores negativos.
func TestCostingRejectsInvalidInputs(t *testing.T) {
	if _, err := Issue(state("1", "1"), number("0")); err == nil {
		t.Fatal("se aceptó una salida de cero")
	}
	if _, err := Receive(state("0", "0"), number("1"), number("-1")); err == nil {
		t.Fatal("se aceptó una entrada con valor negativo")
	}
	if _, err := Restore(state("0", "0"), number("-1"), number("1")); err == nil {
		t.Fatal("se aceptó una reversión negativa")
	}
	if _, err := ReturnToSupplier(state("1", "1"), number("1"), number("-1")); err == nil {
		t.Fatal("se aceptó un costo original negativo")
	}
}
