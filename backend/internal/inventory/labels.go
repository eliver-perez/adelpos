package inventory

// Nombres visibles de los códigos del libro de movimientos y de los documentos de inventario, para
// los reportes de la etapa 9A (que leen las tablas por SQL en su transacción de solo lectura y deben
// nombrar cada código igual que el módulo de inventario). Devuelven el propio código si no existe.

func labelOf(labels map[string]string, code string) string {
	if label, found := labels[code]; found {
		return label
	}
	return code
}

// MovementKindLabel nombra un tipo de movimiento del libro («Consumo por preparación»).
func MovementKindLabel(kind string) string {
	return labelOf(movementKindLabels, kind)
}

// CostBasisLabel nombra la base de costo de un movimiento («Costo promedio»).
func CostBasisLabel(basis string) string {
	return labelOf(costBasisLabels, basis)
}

// SourceKindLabel nombra el origen de un movimiento («Liberación a cocina»).
func SourceKindLabel(source string) string {
	return labelOf(sourceKindLabels, source)
}

// AdjustmentReasonLabel nombra el motivo de un ajuste o merma («Caducidad»).
func AdjustmentReasonLabel(reason string) string {
	return labelOf(adjustmentReasonLabels, reason)
}

// AdjustmentStateLabel nombra el estado de un ajuste («Pendiente de autorización»).
func AdjustmentStateLabel(state string) string {
	return labelOf(adjustmentStateLabels, state)
}

// MovementKinds enumera los tipos de movimiento en el orden del libro.
var MovementKinds = []string{
	KindInitial, KindReceipt, KindConsumption, KindConsumptionReversal, KindWaste, KindAdjustmentIn, KindAdjustmentOut,
	KindTransferOut, KindTransferIn, KindSupplierReturn, KindProductionIn, KindProductionOut, KindCountIn, KindCountOut,
	KindCostAdjustment,
}

// CountStateLabel nombra el estado de un conteo físico («Aplicado»).
func CountStateLabel(state string) string {
	return labelOf(countStateLabels, state)
}

// CountScopeLabel nombra el alcance de un conteo físico («Todos los insumos»).
func CountScopeLabel(scope string) string {
	return labelOf(countScopeLabels, scope)
}

// PurchaseOrderStateLabel nombra el estado de una orden de compra («Recibida parcialmente»).
func PurchaseOrderStateLabel(state string) string {
	return labelOf(purchaseOrderStateLabels, state)
}
