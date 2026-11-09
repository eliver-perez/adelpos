//go:build integration

package operations_test

import (
	"regexp"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/payments"
)

func testCashOperations(t *testing.T, fixture *cashFixture) {
	shiftID := func() string {
		return fixture.shifts["A"]
	}
	cash := fixture.methods["cash"].ID
	debit := fixture.methods["debit"].ID

	t.Run("C5_ingreso_con_permiso_sin_segunda_autorizacion_e_idempotente", func(t *testing.T) {
		before := fixture.countRows(t, "SELECT count(*) FROM app.cash_operations")
		fixture.shiftCall(t, fixture.observer, "POST", shiftID(), "income", map[string]any{"type": "other", "amount": "10"}, 403)
		for _, body := range []map[string]any{
			{"type": "other", "amount": "0"},
			{"type": "other", "amount": "-5"},
			{"type": "other", "amount": "1.234"},
			{"type": "venta", "amount": "10"},
			{"amount": "10"},
			{"type": "other", "amount": "10", "authorization": authorizationOf("supervisor_caja")},
			{"type": "other", "amount": "10", "category_id": fixture.categories["Insumos"]},
			{"type": "other", "amount": "10", "currency": "EUR"},
			{"type": "other", "amount": "10", "concept": strings.Repeat("x", 121)},
		} {
			fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", body, 400)
		}
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", map[string]any{
			"type": "other", "amount": "10", "currency": "USD",
		}, 409)) != "FEATURE_DISABLED" {
			t.Fatal("Se registró un ingreso en dólares con la función desactivada")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_operations") != before {
			t.Fatal("Un ingreso rechazado dejó registros")
		}
		body := map[string]any{"operation_id": randomID(), "type": "cash_return", "amount": "35.50", "concept": "Reintegro de vuelto", "notes": "Sobró de una compra"}
		first := fixture.operation(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", body, 200))
		repeated := fixture.operation(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", body, 200))
		if first.ID != repeated.ID || fixture.countRows(t, "SELECT count(*) FROM app.cash_operations WHERE operation_id=$1", body["operation_id"]) != 1 {
			t.Fatal("Un reintento del ingreso lo duplicó")
		}
		if first.Kind != "income" || first.Direction != "in" || first.Amount != "35.50" || first.Currency != "MXN" ||
			first.Concept != "Reintegro de vuelto" || first.TypeLabel == nil || *first.TypeLabel != "Reintegro de efectivo" ||
			first.RegisterName != "Caja de arqueo" || first.EquivalentMXN != "35.50" || first.Status != "recorded" {
			t.Fatalf("El ingreso no conservó sus datos: %+v", first)
		}
		body["amount"] = "36"
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", body, 409)) != "IDEMPOTENCY_CONFLICT" {
			t.Fatal("El mismo identificador con otro importe no se rechazó")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.operation_recorded' AND entity_id=$1", first.ID) != 1 ||
			fixture.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='cash.operation.recorded' AND scope='cash'") == 0 {
			t.Fatal("El ingreso no quedó auditado ni publicó su evento de caja")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.payments WHERE shift_id=$1", shiftID()) != 0 {
			t.Fatal("Un ingreso de efectivo se registró como venta")
		}
		fixture.operations["committed_income"] = first.ID
	})

	t.Run("C6_cobros_del_turno_y_efectivo_esperado_por_movimientos_reales", func(t *testing.T) {
		before := fixture.expected(t, shiftID())
		paid := fixture.account(t, "Cobro en caja de arqueo", line(fixture.soda, 4))
		cashPayment := decodeResponse[paymentResult](t, fixture.pay(t, fixture.cashier, paid.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "received": "200", "shift_id": shiftID(),
		}, 200))
		card := fixture.account(t, "Tarjeta en caja de arqueo", line(fixture.soda, 2))
		decodeResponse[paymentResult](t, fixture.pay(t, fixture.cashier, card.Account.ID, map[string]any{"method_id": debit, "mode": "total"}, 200))
		after := fixture.expected(t, shiftID())
		// Se reciben $200 y se entregan $100 de cambio: la caja aumenta $100, no $200; la tarjeta no
		// mueve efectivo.
		if !amountOf(t, after.MXN).Sub(amountOf(t, before.MXN)).Equal(amountOf(t, "100")) {
			t.Fatalf("El esperado no refleja el neto físico del cobro: antes %s, después %s", before.MXN, after.MXN)
		}
		if cashPayment.Payment.ShiftID != shiftID() {
			t.Fatal("El cobro no quedó en el turno de la caja")
		}
		fixture.accounts["paid"] = paid.Account.ID
		fixture.payments["cash"] = cashPayment.Payment.ID
		other := fixture.account(t, "Corrección pendiente en caja de arqueo", line(fixture.soda, 2))
		correctable := decodeResponse[paymentResult](t, fixture.pay(t, fixture.cashier, other.Account.ID, map[string]any{
			"method_id": cash, "mode": "partial", "amount": "20",
		}, 200))
		fixture.accounts["correctable"] = other.Account.ID
		fixture.payments["correctable"] = correctable.Payment.ID
		pending := fixture.account(t, "Cuenta abierta durante el arqueo", line(fixture.soda, 2))
		fixture.accounts["pending"] = pending.Account.ID
	})

	t.Run("C7_retiros_y_gastos_siempre_autorizados", func(t *testing.T) {
		before := fixture.countRows(t, "SELECT count(*) FROM app.cash_operations")
		withdrawal := map[string]any{"type": "safekeeping", "amount": "100", "concept": "Resguardo de billetes grandes"}
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "withdrawals", withdrawal, 403)) != "AUTHORIZATION_REQUIRED" {
			t.Fatal("Un retiro sin autorización no la pidió")
		}
		withdrawal["authorization"] = authorizationOf("cajero_caja")
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "withdrawals", withdrawal, 403)) != "SELF_AUTHORIZATION" {
			t.Fatal("El cajero se autorizó a sí mismo un retiro")
		}
		withdrawal["authorization"] = authorizationOf("cajero_dos")
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "withdrawals", withdrawal, 403)) != "AUTHORIZATION_INSUFFICIENT" {
			t.Fatal("Autorizó un usuario sin permiso de autorizar salidas")
		}
		withdrawal["authorization"] = map[string]any{"username": "supervisor_caja", "password": "contraseña equivocada"}
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "withdrawals", withdrawal, 401)) != "AUTHORIZATION_INVALID" {
			t.Fatal("Una contraseña de supervisor incorrecta no se rechazó")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_operations") != before {
			t.Fatal("Un retiro rechazado dejó registros")
		}
		withdrawal["authorization"] = authorizationOf("supervisor_caja")
		authorized := fixture.operation(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "withdrawals", withdrawal, 200))
		if authorized.Kind != "withdrawal" || authorized.Direction != "out" || text(authorized.AuthorizationMode) != "supervisor" ||
			text(authorized.AuthorizedByName) != "Prueba supervisor_caja" || authorized.ActorName != "Prueba cajero_caja" {
			t.Fatalf("El retiro no registró a quien ejecuta y a quien autoriza: %+v", authorized)
		}
		fixture.operations["withdrawal"] = authorized.ID
		// Quien tiene el permiso de autorizar no necesita segunda contraseña: queda autoautorizado.
		self := fixture.operation(t, fixture.shiftCall(t, fixture.manager, "POST", shiftID(), "withdrawals", map[string]any{
			"type": "admin_delivery", "amount": "50",
		}, 200))
		if text(self.AuthorizationMode) != "self" || text(self.AuthorizedByName) != self.ActorName {
			t.Fatalf("El retiro del encargado no quedó autoautorizado: %+v", self)
		}
		fixture.operations["self_withdrawal"] = self.ID
		expense := map[string]any{"category_id": fixture.categories["Insumos"], "amount": "45.90", "concept": "Hielo", "reference": "Nota 123"}
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "expenses", expense, 403)) != "AUTHORIZATION_REQUIRED" {
			t.Fatal("Un gasto sin autorización no la pidió")
		}
		expense["authorization"] = authorizationOf("supervisor_caja")
		spent := fixture.operation(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "expenses", expense, 200))
		if spent.Kind != "expense" || text(spent.CategoryName) != "Insumos" || spent.Reference != "Nota 123" || spent.Type != nil ||
			text(spent.AuthorizationMode) != "supervisor" {
			t.Fatalf("El gasto no quedó diferenciado de los retiros: %+v", spent)
		}
		selfExpense := fixture.operation(t, fixture.shiftCall(t, fixture.manager, "POST", shiftID(), "expenses", map[string]any{
			"category_id": fixture.categories["Servicios"], "amount": "30",
		}, 200))
		if text(selfExpense.AuthorizationMode) != "self" || selfExpense.Concept != "Servicios" {
			t.Fatalf("El gasto del encargado no quedó autoautorizado con su categoría: %+v", selfExpense)
		}
		fixture.operations["expense"] = spent.ID
		// Categorías configurables: una categoría desactivada ya no admite gastos.
		category := decodeResponse[payments.ExpenseCategory](t, fixture.adminCall(t, "POST", "payments/cash/expense-categories", map[string]any{
			"operation_id": randomID(), "name": "Temporal de prueba", "active": true,
		}, 200))
		fixture.call(t, fixture.cashier, "POST", "payments/cash/expense-categories", map[string]any{
			"operation_id": randomID(), "name": "Sin permiso", "active": true,
		}, 403)
		fixture.adminCall(t, "PATCH", "payments/cash/expense-categories/"+category.ID, map[string]any{
			"operation_id": randomID(), "revision": category.Revision, "name": category.Name, "active": false,
		}, 200)
		for _, body := range []map[string]any{
			{"category_id": category.ID, "amount": "10"},
			{"amount": "10"},
			{"category_id": fixture.categories["Insumos"], "amount": "10", "type": "other"},
		} {
			fixture.shiftCall(t, fixture.manager, "POST", shiftID(), "expenses", body, 400)
		}
		fixture.shiftCall(t, fixture.partner, "POST", shiftID(), "withdrawals", map[string]any{"type": "other", "amount": "1"}, 403)
		// Las contraseñas nunca se guardan en auditoría ni en las respuestas idempotentes.
		if fixture.countRows(t, `SELECT count(*) FROM app.audit_events WHERE coalesce(before_state::text,'') LIKE '%Clave temporal%'
            OR coalesce(after_state::text,'') LIKE '%Clave temporal%'`) != 0 ||
			fixture.countRows(t, "SELECT count(*) FROM app.restaurant_operations WHERE response::text LIKE '%Clave temporal%'") != 0 {
			t.Fatal("Una contraseña de autorización quedó registrada")
		}
	})

	t.Run("C8_salida_que_excede_el_disponible_sin_revelar_cifras_y_excepcional", func(t *testing.T) {
		expected := fixture.expected(t, shiftID())
		excess := amountOf(t, expected.MXN).Add(amountOf(t, "1000")).StringFixed(2)
		before := fixture.countRows(t, "SELECT count(*) FROM app.cash_operations")
		problem := fixture.shiftCall(t, fixture.manager, "POST", shiftID(), "withdrawals", map[string]any{"type": "other", "amount": excess}, 409)
		message := decodeResponse[struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}](t, problem).Error
		if message.Code != "CASH_INSUFFICIENT" || regexp.MustCompile(`[0-9]`).MatchString(message.Message) ||
			strings.Contains(string(problem), expected.MXN) || !strings.Contains(message.Message, "supervisor") {
			t.Fatalf("El rechazo por exceso reveló cifras o no orientó al cajero: %s", problem)
		}
		fixture.shiftCall(t, fixture.manager, "POST", shiftID(), "expenses", map[string]any{
			"category_id": fixture.categories["Otros"], "amount": excess,
		}, 409)
		exceptional := map[string]any{"type": "other", "amount": excess, "exceptional": true, "reason": "Pago urgente a proveedor autorizado"}
		// Sin consultar el esperado, ni el encargado ni otro supervisor sin ese permiso lo autorizan.
		if errorCode(t, fixture.shiftCall(t, fixture.manager, "POST", shiftID(), "withdrawals", exceptional, 403)) != "AUTHORIZATION_REQUIRED" {
			t.Fatal("Una salida excepcional se autoautorizó sin permiso de consultar el esperado")
		}
		exceptional["authorization"] = authorizationOf("encargado_caja")
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "withdrawals", exceptional, 403)) != "AUTHORIZATION_INSUFFICIENT" {
			t.Fatal("Autorizó una salida excepcional alguien sin permiso de consultar el esperado")
		}
		exceptional["reason"] = "No"
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "withdrawals", exceptional, 400)
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_operations") != before {
			t.Fatal("Una salida rechazada dejó registros")
		}
		exceptional["reason"] = "Pago urgente a proveedor autorizado"
		exceptional["authorization"] = authorizationOf("supervisor_caja")
		recorded := fixture.operation(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "withdrawals", exceptional, 200))
		if !recorded.Exceptional || recorded.Reason != "Pago urgente a proveedor autorizado" || text(recorded.AuthorizedByName) != "Prueba supervisor_caja" {
			t.Fatalf("La salida excepcional no quedó identificada: %+v", recorded)
		}
		// Se repone el efectivo para continuar: un ingreso identificado del encargado de turno.
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", map[string]any{
			"type": "float_contribution", "amount": amountOf(t, excess).Add(amountOf(t, "500")).StringFixed(2),
		}, 200)
		// Solicitada como excepcional sin exceder el disponible, se registra como ordinaria.
		ordinary := fixture.operation(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "withdrawals", map[string]any{
			"type": "other", "amount": "1", "exceptional": true, "reason": "Verificación de exceso", "authorization": authorizationOf("supervisor_caja"),
		}, 200))
		if ordinary.Exceptional {
			t.Fatal("Una salida dentro del disponible quedó marcada como excepcional")
		}
	})

	t.Run("C9_movimientos_sin_permiso_no_devuelven_esperado", func(t *testing.T) {
		route := "payments/shifts/" + shiftID() + "/movements"
		hidden := fixture.call(t, fixture.observer, "GET", route, nil, 200)
		blind(t, hidden, "Los movimientos sin permiso")
		if !strings.Contains(string(hidden), `"expected_visible":false`) || !strings.Contains(string(hidden), `"by_method"`) {
			t.Fatalf("Los movimientos sin permiso perdieron sus totales por método: %s", hidden)
		}
		visible := decodeResponse[struct {
			ExpectedVisible bool                     `json:"expected_visible"`
			ExpectedCash    string                   `json:"expected_cash"`
			Expected        payments.CashExpectation `json:"expected"`
			ByCurrency      map[string]string        `json:"by_currency"`
		}](t, fixture.adminCall(t, "GET", route, nil, 200))
		detail := fixture.expected(t, shiftID())
		if !visible.ExpectedVisible || visible.ExpectedCash != detail.MXN || visible.ByCurrency["expected_mxn"] != detail.MXN ||
			visible.Expected.MXN != detail.MXN {
			t.Fatalf("Los movimientos y el detalle no usan el mismo esperado: %+v / %+v", visible, detail)
		}
		operations := decodeResponse[struct {
			Operations []payments.CashOperation `json:"operations"`
		}](t, fixture.shiftCall(t, fixture.manager, "GET", shiftID(), "operations", nil, 200))
		if len(operations.Operations) < 8 {
			t.Fatalf("La consulta de operaciones del turno está incompleta: %d", len(operations.Operations))
		}
		blind(t, fixture.shiftCall(t, fixture.manager, "GET", shiftID(), "operations", nil, 200), "Las operaciones del turno")
		fixture.shiftCall(t, fixture.observer, "GET", shiftID(), "operations", nil, 403)
	})
}
