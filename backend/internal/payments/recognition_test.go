package payments

import "testing"

// recognizedUnit es lo ya reconocido de una unidad de 25.00 (IVA 16 %) con el estado actual de la
// unidad y si se reconoció en la cuenta que cierra.
func recognizedUnit(unitID, unitAccount, state string, recognizedHere bool) recognizedSource {
	return recognizedSource{
		Line: recognitionLine{
			Key: recognitionKey{Kind: sourceUnit, UnitID: unitID, TaxRate: 16},
			Amounts: recognitionAmounts{Quantity: 1, Gross: amount("25.00"), Promotion: zero, Discount: zero, Courtesy: zero,
				Charge: zero, Net: amount("25.00"), Base: amount("21.55"), Tax: amount("3.45")},
		},
		UnitAccountID: pointer(unitAccount), UnitState: pointer(state), RecognizedInThere: recognizedHere,
	}
}

// Una unidad reconocida en la cuenta A, transferida después de una reapertura a la cuenta B y
// cancelada en B: al cerrar B se reconoce su diferencia negativa (aunque se reconoció en A), una sola
// vez; una unidad activa en otra cuenta no se resta; una cancelada que hoy está en otra cuenta la
// revierte esa cuenta o la que la reconoció.
func TestRecognitionDeltasReverseUnitCancelledAfterTransfer(t *testing.T) {
	const accountA, accountB, accountC = "00000000-0000-4000-8000-0000000000a1", "00000000-0000-4000-8000-0000000000b1",
		"00000000-0000-4000-8000-0000000000c1"
	const unit = "00000000-0000-4000-8000-0000000000e1"
	key := recognitionKey{Kind: sourceUnit, UnitID: unit, TaxRate: 16}

	// Cierre de B: la unidad hoy pertenece a B, cancelada, reconocida en A.
	deltas := recognitionDeltas(accountB, nil, map[recognitionKey]recognizedSource{key: recognizedUnit(unit, accountB, "cancelled", false)})
	if len(deltas) != 1 || deltas[0].Key != key || deltas[0].Amounts.Quantity != -1 || !deltas[0].Amounts.Net.Equal(amount("-25.00")) ||
		!deltas[0].Amounts.Tax.Equal(amount("-3.45")) {
		t.Fatalf("Al cerrar la cuenta nueva se revierte la unidad cancelada: %+v", deltas)
	}
	// Ya revertida (la suma global es cero): no se escribe nada otra vez.
	reversed := recognizedUnit(unit, accountB, "cancelled", true)
	reversed.Line.Amounts = recognitionAmounts{Gross: zero, Promotion: zero, Discount: zero, Courtesy: zero, Charge: zero, Net: zero,
		Base: zero, Tax: zero}
	if deltas := recognitionDeltas(accountA, nil, map[recognitionKey]recognizedSource{key: reversed}); len(deltas) != 0 {
		t.Fatalf("Una unidad ya revertida no se revierte dos veces: %+v", deltas)
	}
	// Cierre de A con la unidad activa en B: no se resta aquí (la reconoce B).
	if deltas := recognitionDeltas(accountA, nil, map[recognitionKey]recognizedSource{key: recognizedUnit(unit, accountB, "active", true)}); len(deltas) != 0 {
		t.Fatalf("Una unidad activa en otra cuenta no se resta: %+v", deltas)
	}
	// Cierre de A con la unidad cancelada en B (B aún no cierra): A la revierte, como antes.
	if deltas := recognitionDeltas(accountA, nil, map[recognitionKey]recognizedSource{key: recognizedUnit(unit, accountB, "cancelled", true)}); len(deltas) != 1 {
		t.Fatalf("La cuenta que la reconoció revierte una unidad cancelada: %+v", deltas)
	}
	// Una cuenta ajena (ni la reconoció ni la tiene) no la toca.
	if deltas := recognitionDeltas(accountC, nil, map[recognitionKey]recognizedSource{key: recognizedUnit(unit, accountB, "cancelled", false)}); len(deltas) != 0 {
		t.Fatalf("Una cuenta ajena no revierte la unidad: %+v", deltas)
	}
}
