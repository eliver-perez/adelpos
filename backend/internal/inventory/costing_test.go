package inventory

import (
	"errors"
	"math/rand/v2"
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

// expectState compara Q, V, L y la cantidad pendiente de costo, y comprueba los invariantes del
// motor en cada estado que verifica una prueba.
func expectState(t *testing.T, label string, actual Valuation, quantity, value, last, pending string) {
	t.Helper()
	expectDecimal(t, label+" Q", actual.Quantity, quantity)
	expectDecimal(t, label+" V", actual.Value, value)
	expectOptional(t, label+" L", actual.LastValidCost, last)
	expectDecimal(t, label+" pendiente", actual.PendingQuantity, pending)
	expectInvariants(t, label, actual)
}

// expectInvariants: Q = 0 ⇒ V = 0; Q > 0 ⇒ V ≥ 0 (nunca valor negativo con existencia positiva);
// L nulo o mayor que cero (CHECK last_valid_cost > 0); 0 ≤ pendiente ≤ déficit.
func expectInvariants(t *testing.T, label string, actual Valuation) {
	t.Helper()
	if actual.Quantity.IsZero() && !actual.Value.IsZero() {
		t.Fatalf("%s: Q = 0 con V = %s", label, actual.Value)
	}
	if actual.Quantity.IsPositive() && actual.Value.IsNegative() {
		t.Fatalf("%s: V = %s negativo con Q = %s positiva", label, actual.Value, actual.Quantity)
	}
	if actual.LastValidCost != nil && !actual.LastValidCost.IsPositive() {
		t.Fatalf("%s: último costo válido no positivo %s", label, actual.LastValidCost)
	}
	if actual.PendingQuantity.IsNegative() || actual.PendingQuantity.GreaterThan(actual.deficit()) {
		t.Fatalf("%s: cantidad pendiente %s fuera de [0, %s]", label, actual.PendingQuantity, actual.deficit())
	}
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
	// Sin existencia, sin L y con R = 12: 2 u al costo configurado = 24 (base «configured»).
	configured := state("0", "0")
	configured.ReferenceCost = pointer("12")
	reference := mustEntry(t)(Admit(configured, number("2")))
	expectDecimal(t, "ajuste al costo configurado", reference.Value, "24")
	if reference.Basis != basisConfigured || reference.CostPending {
		t.Fatalf("base del ajuste al costo configurado: %+v", reference)
	}
	expectState(t, "existencia al costo configurado", reference.After, "2", "24", "12", "0")
	// Sin existencia ni costo alguno el ajuste se rechaza (TestCostingAdmitWithoutAnyCost): antes
	// entraba a valor cero sin marca, que es un costo inventado.
}

// Entrada sin documento sin ningún costo (§0-B.6 «nunca a un costo inventado»): la parte que dejaría
// existencia positiva se rechaza con errCostRequired (COST_REQUIRED en la API); la parte que solo
// cubre déficit sigue permitida a su valor provisional.
func TestCostingAdmitWithoutAnyCost(t *testing.T) {
	// 0 u, sin L ni R: un ajuste de +2 u dejaría 2 u sin costo → rechazo.
	if _, err := Admit(state("0", "0"), number("2")); !errors.Is(err, errCostRequired) {
		t.Fatalf("esperado errCostRequired, obtenido %v", err)
	}
	// −3 u pendientes de costo (V = 0, sin L ni R). Sobrante de 3 u: cubre exactamente el déficit
	// al provisional −V = 0 → 0 u, valor 0, nada pendiente. Permitido.
	pending := state("-3", "0")
	pending.PendingQuantity = number("3")
	covered := mustEntry(t)(Admit(pending, number("3")))
	expectDecimal(t, "sobrante que cubre el déficit", covered.Value, "0")
	if covered.Basis != basisProvisional || covered.CostPending {
		t.Fatalf("base del sobrante que cubre el déficit: %+v", covered)
	}
	expectState(t, "déficit cubierto sin costo", covered.After, "0", "0", "", "0")
	// Sobrante de 2 u: cubre 2 de 3 u al provisional 2 × 0 / −3 = 0 → −1 u, 1 u pendiente.
	part := mustEntry(t)(Admit(pending, number("2")))
	expectState(t, "déficit parcial sin costo", part.After, "-1", "0", "", "1")
	// Sobrante de 5 u: cubriría 3 u y dejaría 2 u positivas sin costo → rechazo, estado intacto.
	if _, err := Admit(pending, number("5")); !errors.Is(err, errCostRequired) {
		t.Fatalf("esperado errCostRequired al cruzar a positivo sin costo, obtenido %v", err)
	}
	expectState(t, "estado sin cambios", pending, "-3", "0", "", "3")
	// Con un último costo válido (L = 4) el mismo sobrante sí entra: 3 u al provisional 0 y 2 u a
	// 4 = 8 → 2 u por 8.
	pending.LastValidCost = pointer("4")
	crossed := mustEntry(t)(Admit(pending, number("5")))
	expectDecimal(t, "sobrante con L", crossed.Value, "8")
	expectState(t, "sobrante con L", crossed.After, "2", "8", "4", "0")
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

// Reversión que deja la cantidad exactamente en cero: el ajuste de costo realinea el déficit y el
// valor queda en cero sin residuo (antes, el residuo −50 hacía ese trabajo).
func TestCostingZeroQuantityInvariant(t *testing.T) {
	// −5 u por −100 (L = 20). Se revierte un consumo de 5 u registrado en $150: el movimiento lleva
	// 150 (valor original); cubre todo el déficit: provisional = −V = 100, real = 150, ajuste
	// 100 − 150 = −50. V = −100 + 150 − 50 = 0 con 0 u.
	start := state("-5", "-100")
	start.LastValidCost = pointer("20")
	entry := mustEntry(t)(Restore(start, number("5"), number("150")))
	expectDecimal(t, "reversión", entry.Value, "150")
	if entry.Basis != basisOriginal || entry.Residual != nil || entry.Adjustment == nil {
		t.Fatalf("la reversión debió producir un ajuste de costo y ningún residuo: %+v", entry)
	}
	expectDecimal(t, "ajuste de costo", entry.Adjustment.Value, "-50")
	expectState(t, "invariante", entry.After, "0", "0", "20", "0")
	// Revertir el consumo exacto que dejó el déficit no produce ajuste ni residuo.
	exact := mustEntry(t)(Restore(start, number("5"), number("100")))
	if exact.Residual != nil || exact.Adjustment != nil {
		t.Fatal("una reversión exacta no deja ajuste ni residuo")
	}
	expectState(t, "reversión exacta", exact.After, "0", "0", "20", "0")
}

// El residuo es la red de seguridad del invariante Q = 0 ⇒ V = 0: si un estado llegara a cero con
// valor, settle emite −V.
func TestCostingResidualSafetyNet(t *testing.T) {
	entry := Entry{}
	settle(&entry, Valuation{Quantity: zero, Value: number("0.000004"), PendingQuantity: zero})
	if entry.Residual == nil {
		t.Fatal("settle debió emitir el residuo")
	}
	expectDecimal(t, "residuo", entry.Residual.Value, "-0.000004")
	expectState(t, "tras el residuo", entry.After, "0", "0", "", "0")
}

// Entrada de una elaboración (production_in) con existencia negativa del insumo preparado: es una
// entrada con documento (regla 4), no una reversión.
func TestCostingProductionCoversDeficitAtRealCost(t *testing.T) {
	// Salsa con R = 0.1/g, sin existencia: se venden 500 g antes de elaborarla → 500 × 0.1 = 50;
	// Q = −500, V = −50 (base «configured»).
	start := state("0", "0")
	start.ReferenceCost = pointer("0.1")
	sold := mustEntry(t)(Issue(start, number("500")))
	expectDecimal(t, "venta antes de elaborar", sold.Value, "-50")
	expectState(t, "tras vender", sold.After, "-500", "-50", "", "0")
	// Se elaboran 1000 g con valor consumido 300 (0.3/g): d = 500 = déficit, provisional = −V = 50;
	// real = 300 × 500 / 1000 = 150; ajuste = 50 − 150 = −100. V = −50 + 300 − 100 = 150 con
	// 500 g: promedio 0.3/g, el costo real de la elaboración.
	produced := mustEntry(t)(Receive(sold.After, number("1000"), number("300")))
	expectDecimal(t, "entrada de la elaboración", produced.Value, "300")
	expectOptional(t, "costo de la elaboración", produced.UnitCost, "0.3")
	if produced.Basis != basisDocument || produced.Adjustment == nil {
		t.Fatalf("la elaboración con déficit debe llevar un ajuste de costo: %+v", produced)
	}
	expectDecimal(t, "ajuste de costo", produced.Adjustment.Value, "-100")
	expectState(t, "tras elaborar", produced.After, "500", "150", "0.3", "0")
	// Como reversión (comportamiento anterior) habría quedado V = −50 + 300 = 250 con 500 g: un
	// promedio de 0.5/g que no corresponde a ningún costo real.
}

// Reversión de consumo con existencia negativa: el movimiento conserva el valor original y el
// déficit cubierto se realinea con un ajuste de costo separado.
func TestCostingConsumptionReversalWithNegativeStock(t *testing.T) {
	// (a) Entran 10 u a $20: Q = 10, V = 200, L = 20.
	first := mustEntry(t)(Receive(state("0", "0"), number("10"), number("200")))
	// El consumo A de 10 u vale V exacto 200: Q = 0, V = 0.
	consumedA := mustEntry(t)(Issue(first.After, number("10")))
	expectDecimal(t, "consumo A", consumedA.Value, "-200")
	// Entran 10 u a $30: Q = 10, V = 300, L = 30.
	second := mustEntry(t)(Receive(consumedA.After, number("10"), number("300")))
	// Se consumen 15 u: 10 u por V = 300 y 5 u a L = 30 = 150 → 450. Q = −5, V = −150.
	consumedB := mustEntry(t)(Issue(second.After, number("15")))
	expectDecimal(t, "consumo B", consumedB.Value, "-450")
	expectState(t, "antes de revertir", consumedB.After, "-5", "-150", "30", "0")
	// Se revierte A (10 u, valor original 200): el movimiento lleva 200 (base «original», costo 20).
	// d = 5 = déficit: provisional = −V = 150; real = 200 × 5 / 10 = 100; ajuste = 150 − 100 = +50.
	// V = −150 + 200 + 50 = 100 con 5 u: promedio 20 (sin el ajuste, V = 50 y promedio 10).
	reversed := mustEntry(t)(Restore(consumedB.After, number("10"), number("200")))
	expectDecimal(t, "reversión de A", reversed.Value, "200")
	expectOptional(t, "costo de la reversión", reversed.UnitCost, "20")
	if reversed.Basis != basisOriginal || reversed.Adjustment == nil || reversed.Adjustment.Basis != basisDifference {
		t.Fatalf("la reversión con déficit debe llevar un ajuste de costo: %+v", reversed)
	}
	expectDecimal(t, "ajuste de la reversión", reversed.Adjustment.Value, "50")
	expectState(t, "tras revertir A", reversed.After, "5", "100", "20", "0")

	// (b) Q = −5, V = −500 (L = 100). Se revierten 6 u de valor original 6: d = 5, provisional = 500;
	// real = 6 × 5 / 6 = 5; ajuste = 500 − 5 = +495. V = −500 + 6 + 495 = 1 con 1 u (sin el ajuste,
	// V = −494 con Q = 1).
	deep := state("-5", "-500")
	deep.LastValidCost = pointer("100")
	small := mustEntry(t)(Restore(deep, number("6"), number("6")))
	expectDecimal(t, "reversión pequeña", small.Value, "6")
	expectDecimal(t, "ajuste de la reversión pequeña", small.Adjustment.Value, "495")
	expectState(t, "tras la reversión pequeña", small.After, "1", "1", "1", "0")

	// (c) Reversión que cubre solo parte del déficit: Q = −10, V = −200 (provisional 20/u). Se
	// revierten 4 u consumidas a $30 (120): provisional = 4 × −200 / −10 = 80; ajuste = 80 − 120 =
	// −40. V = −200 + 120 − 40 = −120 con −6 u: el promedio provisional del déficit sigue en 20.
	partial := state("-10", "-200")
	partial.LastValidCost = pointer("20")
	within := mustEntry(t)(Restore(partial, number("4"), number("120")))
	expectDecimal(t, "ajuste parcial", within.Adjustment.Value, "-40")
	expectState(t, "déficit restante", within.After, "-6", "-120", "20", "0")

	// (d) Con existencia positiva no hay ajuste: Q = 10, V = 300 + reversión de 5 u por 100 → 15 u
	// por 400 (promedio 26.6666666667).
	positive := mustEntry(t)(Restore(state("10", "300"), number("5"), number("100")))
	if positive.Adjustment != nil {
		t.Fatal("la reversión con existencia positiva no lleva ajuste")
	}
	expectState(t, "reversión con existencia", positive.After, "15", "400", "26.6666666667", "0")

	// (e) Reversión de un consumo pendiente de costo (valor original 0) sobre un déficit pendiente:
	// −5 u, V = 0, 5 u pendientes; se revierten 2 u: provisional 0, real 0, sin ajuste; pendiente 3.
	unpriced := state("-5", "0")
	unpriced.PendingQuantity = number("5")
	pendingReversal := mustEntry(t)(Restore(unpriced, number("2"), number("0")))
	if pendingReversal.Adjustment != nil {
		t.Fatal("revertir un consumo pendiente no produce ajuste")
	}
	expectState(t, "pendiente revertido", pendingReversal.After, "-3", "0", "", "3")
}

// Regla 7: un promedio que redondea a cero en 10 decimales no es un último costo válido (y violaría
// el CHECK last_valid_cost > 0).
func TestCostingLastValidCostNeverRoundsToZero(t *testing.T) {
	// 1 000 000 000 u por 0.000001: promedio 10^-15 → 0.0000000000 a 10 decimales. L sigue sin valor.
	tiny := mustEntry(t)(Receive(state("0", "0"), number("1000000000"), number("0.000001")))
	expectOptional(t, "costo unitario redondeado", tiny.UnitCost, "0")
	expectState(t, "promedio diminuto", tiny.After, "1000000000", "0.000001", "", "0")
	// Con un L anterior (5), se conserva.
	previous := state("0", "0")
	previous.LastValidCost = pointer("5")
	kept := mustEntry(t)(Receive(previous, number("1000000000"), number("0.000001")))
	expectState(t, "L conservado", kept.After, "1000000000", "0.000001", "5", "0")
	// El promedio más pequeño representable sí cuenta: 1 u por 0.000001 → L = 0.000001.
	smallest := mustEntry(t)(Receive(state("0", "0"), number("1"), number("0.000001")))
	expectState(t, "promedio mínimo", smallest.After, "1", "0.000001", "0.000001", "0")
}

// Secuencias pseudoaleatorias (semilla fija) de todas las operaciones del motor: tras cada una se
// comprueban los invariantes y que el valor total cambie exactamente en la suma de los movimientos.
func TestCostingInvariantsOverRandomSequences(t *testing.T) {
	random := rand.New(rand.NewPCG(8, 2026))
	scales := []int32{0, 1, 2, 3, 6}
	amount := func(limit int64) decimal.Decimal {
		return decimal.New(random.Int64N(limit)+1, -scales[random.IntN(len(scales))])
	}
	for sequence := 0; sequence < 200; sequence++ {
		current := state("0", "0")
		if random.IntN(2) == 0 {
			current.ReferenceCost = pointer("0.75")
		}
		for step := 0; step < 60; step++ {
			quantity := amount(5000)
			value := amount(100000)
			before := current
			var entry Entry
			var err error
			switch random.IntN(7) {
			case 0, 1:
				entry, err = Issue(before, quantity)
			case 2:
				entry, err = Receive(before, quantity, value)
			case 3:
				entry, err = Admit(before, quantity)
				if errors.Is(err, errCostRequired) {
					_, basis := before.reference()
					if basis != basisPending || !quantity.GreaterThan(before.deficit()) {
						t.Fatalf("rechazo sin motivo: %+v, %s", before, quantity)
					}
					continue
				}
			case 4:
				entry, err = Restore(before, quantity, value)
			case 5:
				// Una transferencia no cambia el estado del insumo.
				transfer, err := Transfer(before, quantity)
				if err != nil || !transfer.After.Quantity.Equal(before.Quantity) || !transfer.After.Value.Equal(before.Value) {
					t.Fatalf("secuencia %d paso %d: transferencia alteró el estado: %+v %v", sequence, step, transfer, err)
				}
				continue
			default:
				var returned ReturnEntry
				returned, err = ReturnToSupplier(before, quantity, value.DivRound(quantity, costScale))
				entry = returned.Entry
			}
			if err != nil {
				t.Fatalf("secuencia %d paso %d: %v", sequence, step, err)
			}
			moved := entry.Value
			for _, extra := range []*CostAdjustment{entry.Adjustment, entry.Residual} {
				if extra != nil {
					moved = moved.Add(extra.Value)
				}
			}
			if !entry.After.Value.Equal(before.Value.Add(moved)) {
				t.Fatalf("secuencia %d paso %d: V %s + movimientos %s ≠ %s", sequence, step, before.Value, moved, entry.After.Value)
			}
			if entry.After.Value.Exponent() < -valueScale {
				t.Fatalf("secuencia %d paso %d: valor con más de 6 decimales %s", sequence, step, entry.After.Value)
			}
			expectInvariants(t, "secuencia", entry.After)
			current = entry.After
		}
	}
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
