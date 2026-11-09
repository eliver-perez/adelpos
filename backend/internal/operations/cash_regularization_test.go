//go:build integration

package operations_test

import (
	"testing"

	"local/ad-el-pos/server/internal/payments"
)

func testCashRegularization(t *testing.T, fixture *cashFixture) {
	closedShift := func() string {
		return fixture.shifts["A"]
	}

	t.Run("C18_regularizacion_posterior_al_cierre_en_el_turno_vigente", func(t *testing.T) {
		before := decodeResponse[payments.ClosureView](t, fixture.shiftCall(t, fixture.admin, "GET", closedShift(), "closure", nil, 200))
		var storedBefore string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT to_jsonb(c)::text FROM app.cash_shift_closures c WHERE shift_id=$1",
			closedShift()).Scan(&storedBefore); err != nil {
			t.Fatal(err)
		}
		body := map[string]any{"corrects_operation_id": fixture.operations["withdrawal"], "reason": "El retiro de resguardo fue de otra caja"}
		// Sin turno vigente en la caja no hay dónde registrar la regularización.
		if errorCode(t, fixture.shiftCall(t, fixture.manager, "POST", closedShift(), "corrections", body, 409)) != "SHIFT_CLOSED" {
			t.Fatal("Se registró una corrección dentro de un turno cerrado")
		}
		shift := decodeResponse[payments.Shift](t, fixture.openShift(t, fixture.cashier, map[string]any{"opening_float": "500"}, 200))
		fixture.shifts["B"] = shift.ID
		expectedBefore := fixture.expected(t, shift.ID)
		regularization := decodeResponse[correctionResult](t, fixture.shiftCall(t, fixture.manager, "POST", shift.ID, "corrections", body, 200)).Correction
		if !regularization.Regularization || regularization.ShiftID != shift.ID || regularization.Status != "regularization" ||
			text(regularization.CorrectsOperationID) != fixture.operations["withdrawal"] || regularization.Direction != "in" ||
			text(regularization.AuthorizationMode) != "self" {
			t.Fatalf("La regularización no quedó enlazada en el turno vigente: %+v", regularization)
		}
		// No cambia el efectivo esperado del turno vigente ni la diferencia del turno cerrado.
		if fixture.expected(t, shift.ID).MXN != expectedBefore.MXN {
			t.Fatal("La regularización alteró el efectivo esperado del turno vigente")
		}
		after := decodeResponse[payments.ClosureView](t, fixture.shiftCall(t, fixture.admin, "GET", closedShift(), "closure", nil, 200))
		var storedAfter string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT to_jsonb(c)::text FROM app.cash_shift_closures c WHERE shift_id=$1",
			closedShift()).Scan(&storedAfter); err != nil {
			t.Fatal(err)
		}
		if storedAfter != storedBefore || after.Differences.MXN != before.Differences.MXN || len(after.Regularizations) != 1 ||
			after.Regularizations[0].ID != regularization.ID {
			t.Fatalf("La regularización cambió el cierre o no aparece enlazada: %+v", after.Regularizations)
		}
		// El cajero que cerró también ve las regularizaciones enlazadas, sin importes esperados.
		cashierView := fixture.shiftCall(t, fixture.cashier, "GET", closedShift(), "closure", nil, 200)
		blind(t, cashierView, "La consulta del cierre regularizado")
		if len(decodeResponse[payments.ClosureView](t, cashierView).Regularizations) != 1 {
			t.Fatal("El cierre no muestra la regularización a quien lo cerró")
		}
		// Con otro identificador de operación: el mismo identificador solo devolvería el resultado anterior.
		delete(body, "operation_id")
		if errorCode(t, fixture.shiftCall(t, fixture.manager, "POST", shift.ID, "corrections", body, 409)) != "OPERATION_ALREADY_CORRECTED" {
			t.Fatal("Una operación se regularizó dos veces")
		}
		// Una regularización solo se registra en el turno vigente de la misma caja.
		var primaryShift string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT s.id FROM app.cash_shifts s JOIN app.devices d ON d.register_id=s.register_id
            WHERE d.id=$1 AND s.state='open'`, fixture.admin.deviceID).Scan(&primaryShift); err != nil {
			t.Fatal(err)
		}
		if errorCode(t, fixture.shiftCall(t, fixture.admin, "POST", primaryShift, "corrections", map[string]any{
			"corrects_operation_id": fixture.operations["expense"], "reason": "Regularizar desde otra caja",
		}, 409)) != "REGULARIZATION_REGISTER" {
			t.Fatal("Se regularizó una operación desde el turno de otra caja")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.operation_regularized'") != 1 {
			t.Fatal("La regularización no quedó auditada")
		}
	})

	t.Run("C19_cierre_sin_diferencia", func(t *testing.T) {
		shiftID := fixture.shifts["B"]
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "income", map[string]any{"type": "change_supply", "amount": "250.50"}, 200)
		bill := fixture.account(t, "Cobro del segundo turno", line(fixture.burger, 1))
		fixture.pay(t, fixture.cashier, bill.Account.ID, map[string]any{"method_id": fixture.methods["cash"].ID, "mode": "total", "received": "500"}, 200)
		expected := fixture.expected(t, shiftID)
		start := decodeResponse[payments.ClosingView](t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "closing", map[string]any{}, 200))
		saved := fixture.saveCount(t, fixture.cashier, shiftID, fixture.pesosCount(t, expected.MXN), "")
		if saved.Count.Revision <= start.Count.Revision {
			t.Fatal("El borrador del segundo arqueo no se guardó")
		}
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "closing/confirm", map[string]any{"count_revision": saved.Count.Revision}, 200)
		closure := decodeResponse[payments.ClosureView](t, fixture.shiftCall(t, fixture.supervisor, "GET", shiftID, "closure", nil, 200))
		if closure.Differences == nil || closure.Differences.MXN != "0.00" || closure.Differences.ResultMXN != "even" ||
			closure.Expected.MXN != expected.MXN || closure.Count.CountedMXN != expected.MXN {
			t.Fatalf("El cierre sin diferencia no se registró: %+v %+v", closure.Expected, closure.Differences)
		}
	})
}
