//go:build integration

package operations_test

import (
	"encoding/json"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/payments"
)

func testCashClosing(t *testing.T, fixture *cashFixture) {
	shiftID := func() string {
		return fixture.shifts["A"]
	}
	cash := fixture.methods["cash"].ID
	debit := fixture.methods["debit"].ID
	// Operación incierta: preparada antes del arqueo y nunca confirmada.
	uncertain := map[string]any{"operation_id": randomID(), "type": "other", "amount": "10"}
	var committed map[string]any
	var committedResult payments.CashOperation

	t.Run("C12_iniciar_el_arqueo_pausa_la_caja_y_la_captura_es_ciega", func(t *testing.T) {
		committed = map[string]any{"operation_id": randomID(), "type": "other", "amount": "5", "concept": "Antes del arqueo"}
		committedResult = fixture.operation(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", committed, 200))
		started := fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "closing", map[string]any{}, 200)
		blind(t, started, "El inicio del arqueo")
		view := decodeResponse[payments.ClosingView](t, started)
		if view.Shift.State != "closing" || text(view.Shift.ClosingStartedByName) != "Prueba cajero_caja" || view.Shift.ClosingStartedAt == nil ||
			text(view.Shift.ClosingDeviceName) != "Terminal cajero_caja" || view.Count == nil || view.Count.State != "draft" || view.Count.CountedMXN != "0.00" {
			t.Fatalf("El arqueo no registró quién, cuándo y desde dónde: %+v", view.Shift)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='cash.shift.closing_started' AND aggregate_id=$1", shiftID()) != 1 {
			t.Fatal("El inicio del arqueo no publicó su evento")
		}
		current := decodeResponse[struct {
			Shift  *payments.Shift `json:"shift"`
			Usable bool            `json:"usable"`
			Reason string          `json:"reason"`
		}](t, fixture.call(t, fixture.partner, "GET", "payments/shifts/current", nil, 200))
		if current.Usable || current.Shift == nil || current.Shift.State != "closing" || !strings.Contains(current.Reason, "Caja de arqueo está en arqueo de cierre") {
			t.Fatalf("Las terminales de la caja no ven la pausa: %+v", current)
		}
		// Ningún endpoint devuelve esperado ni diferencias durante el arqueo, ni al administrador.
		for _, route := range []string{
			"payments/shifts/" + shiftID(),
			"payments/shifts/" + shiftID() + "/movements",
			"payments/shifts/" + shiftID() + "/closing",
			"payments/shifts/" + shiftID() + "/operations",
			"payments/cash-reports/" + fixture.reports["second"],
			"payments/shifts?register_id=" + fixture.registerID,
			"payments/shifts/current",
			"payments/cash/registers",
		} {
			blind(t, fixture.adminCall(t, "GET", route, nil, 200), "Durante el arqueo, "+route)
		}
		blind(t, fixture.call(t, fixture.supervisor, "GET", "payments/shifts/"+shiftID(), nil, 200), "Durante el arqueo, el supervisor")
		fixture.shiftCall(t, fixture.cashier, "GET", shiftID(), "closure", nil, 409)
	})

	t.Run("C13_cobro_devolucion_correccion_e_ingreso_rechazados_con_SHIFT_CLOSING", func(t *testing.T) {
		paymentsBefore := fixture.countRows(t, "SELECT count(*) FROM app.payments")
		refundsBefore := fixture.countRows(t, "SELECT count(*) FROM app.payment_refunds")
		operationsBefore := fixture.countRows(t, "SELECT count(*) FROM app.cash_operations")
		expectClosing := func(body []byte, context string) {
			t.Helper()
			if errorCode(t, body) != "SHIFT_CLOSING" || !strings.Contains(string(body), "Caja de arqueo está en arqueo de cierre") {
				t.Fatalf("%s no se rechazó con un mensaje claro de arqueo: %s", context, body)
			}
		}
		expectClosing(fixture.pay(t, fixture.cashier, fixture.accounts["pending"], map[string]any{"method_id": cash, "mode": "total"}, 409), "El cobro")
		expectClosing(fixture.pay(t, fixture.cashier, fixture.accounts["pending"], map[string]any{
			"method_id": debit, "mode": "total", "shift_id": shiftID(),
		}, 409), "El cobro con el turno mostrado")
		expectClosing(fixture.post(t, fixture.cashier, fixture.accounts["paid"], "refunds", map[string]any{
			"mode": "payment", "payment_id": fixture.payments["cash"], "amount": "10", "method_id": cash, "reason": "Devolución durante el arqueo",
		}, 409), "La devolución")
		expectClosing(fixture.post(t, fixture.admin, fixture.accounts["correctable"], "corrections", map[string]any{
			"payment_id": fixture.payments["correctable"], "method_id": debit, "reason": "Corrección durante el arqueo",
		}, 409), "La corrección de pago")
		expectClosing(fixture.shiftCall(t, fixture.partner, "POST", shiftID(), "income", map[string]any{"type": "other", "amount": "1"}, 409), "El ingreso")
		expectClosing(fixture.shiftCall(t, fixture.manager, "POST", shiftID(), "withdrawals", map[string]any{"type": "other", "amount": "1"}, 409), "El retiro")
		expectClosing(fixture.shiftCall(t, fixture.manager, "POST", shiftID(), "expenses", map[string]any{
			"category_id": fixture.categories["Otros"], "amount": "1",
		}, 409), "El gasto")
		expectClosing(fixture.shiftCall(t, fixture.manager, "POST", shiftID(), "corrections", map[string]any{
			"corrects_operation_id": fixture.operations["expense"], "reason": "Corrección durante el arqueo",
		}, 409), "La corrección de efectivo")
		expectClosing(fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "partial-cuts", map[string]any{}, 409), "El corte parcial")
		// Reintento de una operación incierta que nunca se confirmó: rechazado sin registrarse.
		expectClosing(fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", uncertain, 409), "El reintento incierto")
		// Reintento de una operación que sí se confirmó: el servidor devuelve su resultado sin duplicarla.
		replayed := fixture.operation(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", committed, 200))
		if replayed.ID != committedResult.ID {
			t.Fatal("El reintento de una operación confirmada no devolvió su resultado")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.payments") != paymentsBefore ||
			fixture.countRows(t, "SELECT count(*) FROM app.payment_refunds") != refundsBefore ||
			fixture.countRows(t, "SELECT count(*) FROM app.cash_operations") != operationsBefore {
			t.Fatal("Una operación rechazada durante el arqueo dejó registros")
		}
		if errorCode(t, fixture.openShift(t, fixture.cashier, map[string]any{"opening_float": "0"}, 409)) != "SHIFT_ALREADY_OPEN" {
			t.Fatal("Se abrió otro turno con la caja en arqueo")
		}
		fixture.adminCall(t, "PATCH", "registers/"+fixture.registerID, map[string]any{"name": "Caja de arqueo", "description": "", "active": false}, 409)
		fixture.adminCall(t, "PATCH", "devices/"+fixture.cashier.deviceID, map[string]any{"name": "Terminal cajero_caja", "register_id": nil, "keyboard_mode": "auto"}, 409)
	})

	t.Run("C14_borrador_del_conteo_en_el_servidor_y_cancelacion", func(t *testing.T) {
		count := fixture.pesosCount(t, "1234.50")
		saved := fixture.saveCount(t, fixture.cashier, shiftID(), count, "Primer conteo")
		if saved.Count.CountedMXN != "1234.50" || saved.Count.ExtraMXN != "4.50" || saved.Count.Notes != "Primer conteo" {
			t.Fatalf("El borrador no calculó el total contado: %+v", saved.Count)
		}
		stale := map[string]any{"revision": saved.Count.Revision - 1}
		for key, value := range count {
			stale[key] = value
		}
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "PUT", shiftID(), "closing/count", stale, 409)) != "COUNT_CHANGED" {
			t.Fatal("Un borrador obsoleto sobrescribió el conteo")
		}
		fixture.shiftCall(t, fixture.cashier, "PUT", shiftID(), "closing/count", map[string]any{
			"revision": saved.Count.Revision, "lines": []map[string]any{{"denomination_id": randomID(), "quantity": 1}},
		}, 400)
		// El borrador sobrevive a la navegación: otra consulta lo recupera tal cual.
		read := fixture.readClosing(t, fixture.partner, shiftID())
		if read.Count.Revision != saved.Count.Revision || read.Count.CountedMXN != "1234.50" || len(read.Count.Lines) != 3 {
			t.Fatal("El borrador del conteo no se conservó en el servidor")
		}
		// Cancelar un arqueo ajeno exige permiso.
		fixture.shiftCall(t, fixture.partner, "POST", shiftID(), "closing/cancel", map[string]any{}, 403)
		cancelBody := map[string]any{"operation_id": randomID(), "reason": "Llegó un cliente a pagar"}
		cancelled := decodeResponse[struct {
			Shift payments.Shift `json:"shift"`
		}](t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "closing/cancel", cancelBody, 200))
		if cancelled.Shift.State != "open" || cancelled.Shift.ClosingStartedAt != nil {
			t.Fatalf("Cancelar el arqueo no reanudó la caja: %+v", cancelled.Shift)
		}
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "closing/cancel", cancelBody, 200)
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "closing/cancel", map[string]any{}, 409)) != "SHIFT_NOT_CLOSING" {
			t.Fatal("Se canceló un arqueo que ya no estaba en curso")
		}
		// La caja reanudada vuelve a cobrar.
		fixture.pay(t, fixture.cashier, fixture.accounts["pending"], map[string]any{"method_id": cash, "mode": "partial", "amount": "10"}, 200)
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.closing_cancelled' AND entity_id=$1", shiftID()) != 1 {
			t.Fatal("La cancelación del arqueo no quedó auditada")
		}
	})

	t.Run("C15_movimiento_posterior_al_inicio_del_arqueo_detectado", func(t *testing.T) {
		restarted := decodeResponse[payments.ClosingView](t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "closing", map[string]any{}, 200))
		if restarted.Count.CountedMXN != "0.00" || len(restarted.Count.Lines) != 0 {
			t.Fatal("Un arqueo nuevo conservó el conteo del arqueo cancelado")
		}
		// Simula un registro que eludiera la pausa: la confirmación lo detecta en su transacción.
		if _, err := fixture.pool.Exec(fixture.ctx, `WITH operation AS (
                INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
                VALUES (gen_random_uuid(),$2,$3,'POST simulado','simulado','{}') RETURNING id)
            INSERT INTO app.cash_operations(id,shift_id,register_id,kind,currency,amount,direction,concept,subtype,actor_id,device_id,operation_id)
            SELECT gen_random_uuid(),s.id,s.register_id,'income','MXN',1,'in','Registro tardío','other',$2,$3,operation.id
            FROM app.cash_shifts s,operation WHERE s.id=$1`, shiftID(), fixture.cashierID, fixture.cashier.deviceID); err != nil {
			t.Fatal(err)
		}
		problem := fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "closing/confirm", map[string]any{"count_revision": restarted.Count.Revision}, 409)
		if errorCode(t, problem) != "CLOSING_MOVEMENTS_CHANGED" {
			t.Fatalf("La confirmación no detectó el movimiento posterior: %s", problem)
		}
		if fixture.detail(t, fixture.cashier, shiftID()).Shift.State != "closing" ||
			fixture.countRows(t, "SELECT count(*) FROM app.cash_shift_closures WHERE shift_id=$1", shiftID()) != 0 {
			t.Fatal("Un arqueo con datos antiguos cerró el turno")
		}
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "closing/cancel", map[string]any{}, 200)
	})

	t.Run("C16_cierre_ciego_con_faltante_cuentas_abiertas_y_confirmacion_unica", func(t *testing.T) {
		expected := fixture.expected(t, shiftID())
		start := decodeResponse[payments.ClosingView](t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "closing", map[string]any{}, 200))
		counted := amountOf(t, expected.MXN).Sub(amountOf(t, "150")).StringFixed(2)
		saved := fixture.saveCount(t, fixture.cashier, shiftID(), fixture.pesosCount(t, counted), "Faltan billetes")
		if saved.Count.Revision <= start.Count.Revision || saved.Count.CountedMXN != counted {
			t.Fatalf("El conteo no se guardó: %+v", saved.Count)
		}
		openAccounts := fixture.countRows(t, `SELECT count(*) FROM app.restaurant_accounts WHERE state='open'`)
		if openAccounts == 0 {
			t.Fatal("La prueba requiere cuentas abiertas al cerrar")
		}
		type outcome struct {
			status int
			body   []byte
		}
		// Dos confirmaciones simultáneas (por ejemplo, un doble toque con identificadores distintos).
		results := make(chan outcome, 2)
		for _, terminal := range []testTerminal{fixture.cashier, fixture.cashier} {
			go func(terminal testTerminal) {
				status, body := fixture.raw(terminal, "POST", "payments/shifts/"+shiftID()+"/closing/confirm", map[string]any{
					"operation_id": randomID(), "count_revision": saved.Count.Revision,
				})
				results <- outcome{status, body}
			}(terminal)
		}
		left, right := <-results, <-results
		assertOneWinner(t, []int{left.status, right.status})
		winner := left
		if right.status == 200 {
			winner = right
		}
		blind(t, winner.body, "La respuesta de la confirmación")
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_shift_closures WHERE shift_id=$1", shiftID()) != 1 ||
			fixture.countRows(t, "SELECT count(*) FROM app.cash_shift_reports WHERE shift_id=$1 AND kind='final'", shiftID()) != 1 {
			t.Fatal("Dos confirmaciones simultáneas cerraron dos veces")
		}
		if fixture.countRows(t, `SELECT count(*) FROM app.restaurant_accounts WHERE state='open'`) != openAccounts {
			t.Fatal("El cierre alteró cuentas abiertas")
		}
		var state, closedBy string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT s.state,c.state FROM app.cash_shifts s JOIN app.cash_counts c
            ON c.shift_id=s.id AND c.purpose='closing' WHERE s.id=$1`, shiftID()).Scan(&state, &closedBy); err != nil || state != "closed" || closedBy != "final" {
			t.Fatalf("El turno o su conteo no quedaron definitivos: %s %s %v", state, closedBy, err)
		}
		// Resultado según permisos: quien cerró sin permiso solo ve lo declarado y que se cerró.
		hidden := fixture.shiftCall(t, fixture.cashier, "GET", shiftID(), "closure", nil, 200)
		blind(t, hidden, "El resultado del cierre sin permiso")
		closure := decodeResponse[payments.ClosureView](t, hidden)
		if closure.Shift.State != "closed" || closure.Count.CountedMXN != counted || closure.CashierNotes != "Faltan billetes" || closure.ExpectedVisible {
			t.Fatalf("El cajero no ve lo que declaró: %+v", closure)
		}
		full := decodeResponse[payments.ClosureView](t, fixture.shiftCall(t, fixture.admin, "GET", shiftID(), "closure", nil, 200))
		if !full.ExpectedVisible || full.Expected == nil || full.Expected.MXN != expected.MXN || full.Differences == nil ||
			full.Differences.MXN != "-150.00" || full.Differences.ResultMXN != "short" || full.Differences.ResultUSD != "even" ||
			full.Differences.EquivalentMXN != nil {
			t.Fatalf("El cierre no registró el faltante: %+v %+v", full.Expected, full.Differences)
		}
		fixture.shiftCall(t, fixture.observer, "GET", shiftID(), "closure", nil, 403)
		var stored string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT after_state::text FROM app.audit_events WHERE action='cash.shift_closed' AND entity_id=$1",
			shiftID()).Scan(&stored); err != nil || strings.Contains(stored, "expected") || strings.Contains(stored, "difference") {
			t.Fatalf("La auditoría del cierre registró importes esperados: %s %v", stored, err)
		}
		fixture.shifts["closed_counted"] = counted
	})

	t.Run("C17_turno_cerrado_rechaza_operaciones_con_SHIFT_CLOSED", func(t *testing.T) {
		expectClosed := func(body []byte, context string) {
			t.Helper()
			if errorCode(t, body) != "SHIFT_CLOSED" {
				t.Fatalf("%s no se rechazó por turno cerrado: %s", context, body)
			}
		}
		expectClosed(fixture.pay(t, fixture.cashier, fixture.accounts["pending"], map[string]any{
			"method_id": cash, "mode": "total", "shift_id": shiftID(),
		}, 409), "El cobro sobre el turno cerrado")
		if errorCode(t, fixture.pay(t, fixture.cashier, fixture.accounts["pending"], map[string]any{"method_id": cash, "mode": "total"}, 409)) != "SHIFT_REQUIRED" {
			t.Fatal("Sin turno abierto la caja cobró")
		}
		expectClosed(fixture.post(t, fixture.cashier, fixture.accounts["paid"], "refunds", map[string]any{
			"mode": "payment", "payment_id": fixture.payments["cash"], "amount": "10", "method_id": cash, "reason": "Devolución tras el cierre",
			"shift_id": shiftID(),
		}, 409), "La devolución sobre el turno cerrado")
		expectClosed(fixture.post(t, fixture.admin, fixture.accounts["correctable"], "corrections", map[string]any{
			"payment_id": fixture.payments["correctable"], "method_id": debit, "reason": "Corrección tras el cierre",
		}, 409), "La corrección de pago")
		expectClosed(fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", uncertain, 409), "El reintento incierto")
		expectClosed(fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "closing", map[string]any{}, 409), "Un arqueo nuevo")
		replayed := fixture.operation(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", committed, 200))
		if replayed.ID != committedResult.ID ||
			fixture.countRows(t, "SELECT count(*) FROM app.cash_operations WHERE operation_id=$1", uncertain["operation_id"]) != 0 {
			t.Fatal("Un reintento sobre el turno cerrado se duplicó o se registró")
		}
		// Comentarios posteriores del supervisor: auditados y sin cambiar la diferencia.
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "closure/comments", map[string]any{"comment": "Sin permiso"}, 403)
		comment := decodeResponse[payments.ClosureComment](t, fixture.shiftCall(t, fixture.supervisor, "POST", shiftID(), "closure/comments", map[string]any{
			"comment": "Revisado con el cajero: faltante por cambio mal entregado",
		}, 200))
		withComments := decodeResponse[payments.ClosureView](t, fixture.shiftCall(t, fixture.admin, "GET", shiftID(), "closure", nil, 200))
		if len(withComments.Comments) != 1 || withComments.Comments[0].ID != comment.ID || withComments.Differences.MXN != "-150.00" {
			t.Fatal("El comentario del supervisor no se conservó o alteró la diferencia")
		}
		var cashierView map[string]json.RawMessage
		_ = json.Unmarshal(fixture.shiftCall(t, fixture.cashier, "GET", shiftID(), "closure", nil, 200), &cashierView)
		if _, found := cashierView["comments"]; found {
			t.Fatal("El cajero sin permiso vio los comentarios del supervisor")
		}
	})
}
