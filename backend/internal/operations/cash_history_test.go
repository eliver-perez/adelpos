//go:build integration

package operations_test

import (
	"testing"
	"time"

	"local/ad-el-pos/server/internal/payments"
)

type shiftHistory struct {
	Items []payments.ShiftHistoryItem `json:"items"`
	Total int                         `json:"total"`
}

func historyContains(history shiftHistory, shiftID string) *payments.ShiftHistoryItem {
	for index := range history.Items {
		if history.Items[index].Shift.ID == shiftID {
			return &history.Items[index]
		}
	}
	return nil
}

func testCashHistory(t *testing.T, fixture *cashFixture) {
	t.Run("C23_historial_por_caja_y_fecha_comercial_con_permisos", func(t *testing.T) {
		fixture.call(t, fixture.cashier, "GET", "payments/shifts", nil, 403)
		route := "payments/shifts?register_id=" + fixture.registerID
		full := decodeResponse[shiftHistory](t, fixture.adminCall(t, "GET", route, nil, 200))
		closed := historyContains(full, fixture.shifts["A"])
		if closed == nil || historyContains(full, fixture.shifts["B"]) == nil || closed.Closure == nil ||
			text(closed.Closure.DifferenceMXN) != "-150.00" || closed.Closure.CountedMXN != fixture.shifts["closed_counted"] {
			t.Fatalf("El historial no muestra los turnos cerrados con su diferencia: %+v", full.Items)
		}
		historian := newCashier(t, fixture.paymentsFixture, "historial_caja", []string{"payments.read", "cash.shifts.read", "cash.history.read"},
			fixture.registerID)
		own := fixture.call(t, historian, "GET", "payments/shifts", nil, 200)
		blind(t, own, "El historial sin permiso de consultar el esperado")
		limited := decodeResponse[shiftHistory](t, own)
		if historyContains(limited, fixture.shifts["A"]) == nil || historyContains(limited, fixture.shifts["USD"]) != nil {
			t.Fatal("Sin permiso para otras cajas el historial mostró turnos ajenos o perdió los propios")
		}
		fixture.call(t, historian, "GET", "payments/shifts?register_id="+fixture.registers["primary"], nil, 403)
		blind(t, fixture.shiftCall(t, historian, "GET", fixture.shifts["A"], "closure", nil, 200), "El cierre visto con historial sin permiso")
		fixture.call(t, historian, "GET", "payments/shifts?from=2026-13-01", nil, 400)
		fixture.call(t, historian, "GET", "payments/shifts?state=perdido", nil, 400)
		withAll := decodeResponse[shiftHistory](t, fixture.call(t, fixture.supervisor, "GET", "payments/shifts?state=closed", nil, 200))
		if historyContains(withAll, fixture.shifts["USD"]) == nil || historyContains(withAll, fixture.shifts["A"]) == nil {
			t.Fatal("Con permiso para otras cajas el historial no incluye todas")
		}
	})

	t.Run("C24_fecha_comercial_de_un_turno_que_cruza_medianoche", func(t *testing.T) {
		shiftID := fixture.shifts["B"]
		// Simula un turno abierto a las 23:30 del 8 de noviembre en Monterrey (05:30 UTC del 9) y un
		// movimiento a las 00:30 del día siguiente; la fecha comercial es la de apertura.
		if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.cash_shifts SET opened_at='2026-11-09 05:30:00+00',business_date=NULL
            WHERE id=$1`, shiftID); err != nil {
			t.Fatal(err)
		}
		var operationID string
		if err := fixture.pool.QueryRow(fixture.ctx, `UPDATE app.cash_operations SET created_at='2026-11-09 06:30:00+00'
            WHERE id=(SELECT id FROM app.cash_operations WHERE shift_id=$1 ORDER BY created_at LIMIT 1) RETURNING id`, shiftID).Scan(&operationID); err != nil {
			t.Fatal(err)
		}
		detail := fixture.detail(t, fixture.admin, shiftID)
		if detail.Shift.BusinessDate != "2026-11-08" {
			t.Fatalf("La fecha comercial no es la de apertura en la zona del establecimiento: %s", detail.Shift.BusinessDate)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.cash_shifts SET business_date='2026-11-08' WHERE id=$1", shiftID); err != nil {
			t.Fatal(err)
		}
		base := "payments/shifts?register_id=" + fixture.registerID
		previous := decodeResponse[shiftHistory](t, fixture.adminCall(t, "GET", base+"&from=2026-11-08&until=2026-11-08", nil, 200))
		next := decodeResponse[shiftHistory](t, fixture.adminCall(t, "GET", base+"&from=2026-11-09&until=2026-11-09", nil, 200))
		if historyContains(previous, shiftID) == nil || historyContains(next, shiftID) != nil {
			t.Fatal("El filtro por fecha comercial no usó la fecha de apertura")
		}
		operations := decodeResponse[struct {
			Operations []payments.CashOperation `json:"operations"`
		}](t, fixture.shiftCall(t, fixture.admin, "GET", shiftID, "operations", nil, 200))
		var moved *payments.CashOperation
		for index := range operations.Operations {
			if operations.Operations[index].ID == operationID {
				moved = &operations.Operations[index]
			}
		}
		location, _ := time.LoadLocation("America/Monterrey")
		if moved == nil || moved.CreatedAt.In(location).Format("2006-01-02 15:04") != "2026-11-09 00:30" {
			t.Fatal("El movimiento no conservó su fecha y hora reales")
		}
	})

	t.Run("C25_cajas_con_su_turno_y_catalogo_de_caja", func(t *testing.T) {
		own := decodeResponse[[]payments.RegisterShift](t, fixture.call(t, fixture.cashier, "GET", "payments/cash/registers", nil, 200))
		if len(own) != 1 || own[0].Register.ID != fixture.registerID || !own[0].Own || own[0].Shift != nil {
			t.Fatalf("El cajero no ve solo su caja sin turno activo: %+v", own)
		}
		all := decodeResponse[[]payments.RegisterShift](t, fixture.adminCall(t, "GET", "payments/cash/registers", nil, 200))
		if len(all) < 4 {
			t.Fatalf("El administrador no ve las demás cajas: %d", len(all))
		}
		catalog := decodeResponse[struct {
			Denominations   []payments.Denomination    `json:"denominations"`
			Categories      []payments.ExpenseCategory `json:"expense_categories"`
			IncomeTypes     []map[string]string        `json:"income_types"`
			WithdrawalTypes []map[string]string        `json:"withdrawal_types"`
		}](t, fixture.call(t, fixture.cashier, "GET", "payments/cash/catalog", nil, 200))
		mexican, dollars := 0, 0
		for _, denomination := range catalog.Denominations {
			if denomination.Currency == "MXN" {
				mexican++
			} else {
				dollars++
			}
		}
		if mexican != 12 || dollars != 6 || len(catalog.IncomeTypes) != 4 || len(catalog.WithdrawalTypes) != 5 || len(catalog.Categories) < 4 {
			t.Fatalf("El catálogo sembrado no es el esperado: %d MXN, %d USD", mexican, dollars)
		}
		fixture.call(t, fixture.cashier, "POST", "payments/cash/denominations", map[string]any{
			"operation_id": randomID(), "currency": "USD", "value": "0.25", "kind": "coin", "active": true,
		}, 403)
		coin := decodeResponse[payments.Denomination](t, fixture.adminCall(t, "POST", "payments/cash/denominations", map[string]any{
			"operation_id": randomID(), "currency": "USD", "value": "0.25", "kind": "coin", "active": true,
		}, 200))
		if errorCode(t, fixture.adminCall(t, "POST", "payments/cash/denominations", map[string]any{
			"operation_id": randomID(), "currency": "USD", "value": "0.25", "kind": "coin", "active": true,
		}, 409)) != "DENOMINATION_EXISTS" {
			t.Fatal("Se duplicó una denominación")
		}
		fixture.adminCall(t, "POST", "payments/cash/denominations", map[string]any{
			"operation_id": randomID(), "currency": "EUR", "value": "1", "kind": "coin", "active": true,
		}, 400)
		fixture.adminCall(t, "PATCH", "payments/cash/denominations/"+coin.ID, map[string]any{
			"operation_id": randomID(), "revision": coin.Revision + 1, "active": false,
		}, 409)
		saved := decodeResponse[payments.Denomination](t, fixture.adminCall(t, "PATCH", "payments/cash/denominations/"+coin.ID, map[string]any{
			"operation_id": randomID(), "revision": coin.Revision, "active": false,
		}, 200))
		if saved.Active || saved.Revision != coin.Revision+1 ||
			fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action IN ('cash.denomination_created','cash.denomination_updated')") != 2 {
			t.Fatal("La denominación no se desactivó con revisión y auditoría")
		}
	})
}
