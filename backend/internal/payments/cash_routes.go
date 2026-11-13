package payments

import (
	"strings"
)

// cashRoute resuelve las rutas de administración de caja (etapa 7) bajo /api/v1/payments/. Cada
// ruta lleva como máximo un identificador, como exige el puente nativo: los registros
// relacionados (la operación que se corrige, por ejemplo) viajan en el cuerpo. El permiso de la
// ruta se comprueba antes de cualquier lectura con bloqueo o escritura; los permisos que dependen
// del caso (autorizaciones, otras cajas, historial) se resuelven dentro de la acción, también
// antes de escribir.
func cashRoute(method string, parts []string) *route {
	length := len(parts)
	switch {
	case length == 1 && parts[0] == "shifts" && method == "GET":
		return &route{permissionHistoryRead, true, (*service).listShifts}
	case length == 2 && parts[0] == "shifts" && parts[1] != "current" && method == "GET":
		return &route{permissionShiftsRead, true, (*service).readShiftDetail}
	case length == 2 && parts[0] == "cash-reports" && method == "GET":
		return &route{permissionShiftsRead, true, (*service).readReport}
	case length == 3 && (parts[0] == "cash-reports" || parts[0] == "cash-operations" || parts[0] == "cash-documents"):
		return cashDocumentRoute(method, parts[0], parts[2])
	case length >= 2 && parts[0] == "cash":
		return cashSettingsRoute(method, parts)
	case length >= 3 && parts[0] == "shifts":
		return shiftActionRoute(method, strings.Join(parts[2:], "/"))
	}
	return nil
}

// cashSettingsRoute atiende el catálogo de caja y las cajas con su turno activo.
func cashSettingsRoute(method string, parts []string) *route {
	resource := parts[1]
	if len(parts) == 2 {
		switch {
		case resource == "catalog" && method == "GET":
			return &route{"payments.read", true, (*service).readCashCatalog}
		case resource == "registers" && method == "GET":
			return &route{permissionShiftsRead, true, (*service).listCashRegisters}
		case resource == "count-preview" && method == "POST":
			// Vista previa del conteo (fase B): solo calcula. El permiso propio de la apertura o del
			// arqueo se comprueba dentro de la acción, según el conteo.
			return &route{"payments.read", true, (*service).previewCount}
		case resource == "denominations" && method == "POST":
			return &route{permissionCashSettings, false, (*service).createDenomination}
		case resource == "expense-categories" && method == "POST":
			return &route{permissionCashSettings, false, (*service).createExpenseCategory}
		// Fase C: tope de caja, cajón de dinero por caja y apertura manual del cajón de la terminal.
		case resource == "limit" && method == "GET":
			return &route{permissionCashSettings, true, (*service).readCashLimitSettings}
		case resource == "limit" && method == "PUT":
			return &route{permissionCashSettings, false, (*service).saveCashLimitSettings}
		case resource == "drawers" && method == "GET":
			return &route{permissionCashSettings, true, (*service).listDrawerSettings}
		case resource == "open-drawer" && method == "POST":
			return &route{permissionDrawerOpen, false, (*service).openDrawerManually}
		}
		return nil
	}
	if len(parts) == 3 && method == "PATCH" {
		switch resource {
		case "denominations":
			return &route{permissionCashSettings, false, (*service).updateDenomination}
		case "expense-categories":
			return &route{permissionCashSettings, false, (*service).updateExpenseCategory}
		}
	}
	if len(parts) == 3 && method == "PUT" && resource == "drawers" {
		return &route{permissionCashSettings, false, (*service).saveDrawerSettings}
	}
	// «Probar apertura» del cajón de una caja (etapa 7.1): payments/cash/drawers/{caja}/test.
	if len(parts) == 4 && method == "POST" && resource == "drawers" && parts[3] == "test" {
		return &route{permissionCashSettings, false, (*service).testDrawer}
	}
	return nil
}

// cashDocumentRoute atiende la vista previa, la impresión y la reimpresión de documentos de caja
// (fase C): payments/cash-reports/{corte}/preview|print, payments/cash-operations/{operación}/
// preview|print y payments/cash-documents/{documento}/reprint. La vista previa no escribe.
func cashDocumentRoute(method, resource, action string) *route {
	switch {
	case resource == "cash-reports" && action == "preview" && method == "GET":
		return &route{permissionShiftsRead, true, (*service).previewReportDocument}
	case resource == "cash-reports" && action == "print" && method == "POST":
		return &route{permissionReportsPrint, false, (*service).printReportDocument}
	case resource == "cash-operations" && action == "preview" && method == "GET":
		return &route{permissionMovementsRead, true, (*service).previewOperationDocument}
	case resource == "cash-operations" && action == "print" && method == "POST":
		return &route{permissionReportsPrint, false, (*service).printOperationDocument}
	case resource == "cash-documents" && action == "reprint" && method == "POST":
		return &route{permissionReportsReprint, false, (*service).reprintCashDocument}
	case resource == "cash-documents" && action == "retry" && method == "POST":
		// Etapa 9F (§4.2): reintentar sin motivo un documento que nunca salió en papel.
		return &route{permissionReportsPrint, false, (*service).retryCashDocument}
	case resource == "cash-documents" && action == "rerender" && method == "POST":
		// Etapa 9B: impresión con la plantilla vigente; exige además documents.history.render_current.
		return &route{permissionReportsReprint, false, (*service).rerenderCashDocument}
	}
	return nil
}

// shiftActionRoute atiende las acciones sobre un turno: payments/shifts/{id}/<acción>.
func shiftActionRoute(method, action string) *route {
	if method == "GET" {
		switch action {
		case "operations":
			return &route{permissionMovementsRead, true, (*service).shiftOperations}
		case "closing":
			return &route{permissionCounts, true, (*service).readClosing}
		case "closure":
			return &route{permissionShiftsRead, true, (*service).readClosure}
		case "summary":
			return &route{permissionShiftsRead, true, (*service).shiftSummary}
		case "documents":
			return &route{permissionShiftsRead, true, (*service).shiftDocuments}
		case "rejected-outflows":
			// Retiros y gastos rechazados por efectivo insuficiente (etapa 7.1): solo para quien
			// puede consultar el esperado.
			return &route{permissionExpectedRead, true, (*service).shiftRejectedOutflows}
		}
		return nil
	}
	if method == "PUT" && action == "closing/count" {
		return &route{permissionCounts, false, (*service).saveClosingCount}
	}
	if method != "POST" {
		return nil
	}
	switch action {
	case "income":
		return &route{permissionIncome, false, (*service).recordIncome}
	case "withdrawals":
		return &route{permissionWithdrawals, false, (*service).recordWithdrawal}
	case "expenses":
		return &route{permissionExpenses, false, (*service).recordExpense}
	case "corrections":
		return &route{permissionCorrections, false, (*service).correctCashOperation}
	case "partial-cuts":
		return &route{permissionPartialCuts, false, (*service).createPartialCut}
	case "closing":
		return &route{permissionCounts, false, (*service).startClosing}
	case "closing/cancel":
		return &route{permissionCounts, false, (*service).cancelClosing}
	case "closing/confirm":
		return &route{permissionClose, false, (*service).confirmClosing}
	case "closure/comments":
		return &route{permissionClosuresReview, false, (*service).commentClosure}
	}
	return nil
}

// cashIdentifier devuelve el identificador de la ruta que debe validarse como UUID, si existe.
func cashIdentifier(parts []string) (string, bool) {
	switch {
	case len(parts) > 1 && (parts[0] == "cash-reports" || parts[0] == "cash-operations" || parts[0] == "cash-documents"):
		return parts[1], true
	case len(parts) > 2 && parts[0] == "cash":
		return parts[2], true
	}
	return "", false
}
