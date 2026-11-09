//go:build integration

package operations_test

import (
	"testing"

	"local/ad-el-pos/server/internal/payments"
)

func testCashUSD(t *testing.T, fixture *cashFixture) {
	cash := fixture.methods["cash"].ID
	var terminal testTerminal
	var shiftID string

	t.Run("C20_turno_con_fondo_en_pesos_y_dolares_y_tipo_de_cambio_del_turno", func(t *testing.T) {
		registerID := fixture.createRegister(t, "Caja bimoneda")
		permissions := append([]string{"payments.usd_rate.manage", "cash.outflows.authorize", "cash.corrections.authorize"}, cashierPermissions...)
		terminal = newCashier(t, fixture.paymentsFixture, "cajero_bimoneda", permissions, registerID)
		opening := map[string]any{
			"opening_float": "1000", "opening_float_usd": "50", "usd_rate": "18.50", "confirm_rate_difference": true,
			"count": map[string]any{"lines": []map[string]any{
				fixture.line(t, "MXN 500.00 bill", 2), fixture.line(t, "USD 20.00 bill", 2), fixture.line(t, "USD 10.00 bill", 1),
			}},
		}
		if errorCode(t, fixture.openShift(t, terminal, opening, 409)) != "FEATURE_DISABLED" {
			t.Fatal("Con la función desactivada se abrió un turno con dólares")
		}
		fixture.setUSDFeature(t, true)
		shift := decodeResponse[payments.Shift](t, fixture.openShift(t, terminal, opening, 200))
		if shift.OpeningFloat != "1000.00" || text(shift.OpeningFloatUSD) != "50.00" || text(shift.USDRate) != "18.50" {
			t.Fatalf("El turno no conservó sus fondos por moneda y su tipo de cambio: %+v", shift)
		}
		detail := fixture.detail(t, terminal, shift.ID)
		if detail.OpeningCount == nil || detail.OpeningCount.CountedMXN != "1000.00" || detail.OpeningCount.CountedUSD != "50.00" {
			t.Fatalf("El desglose de apertura no distinguió monedas: %+v", detail.OpeningCount)
		}
		shiftID = shift.ID
		fixture.shifts["USD"] = shift.ID
	})

	t.Run("C21_cobros_con_dolares_retiro_y_correccion_en_dolares", func(t *testing.T) {
		// Cobro de $100 con US$10 ($185): el cambio de $85 sale en pesos.
		first := fixture.account(t, "Dólares con cambio en pesos", line(fixture.soda, 4))
		paidUSD := decodeResponse[paymentResult](t, fixture.pay(t, terminal, first.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "received_usd": "10", "shift_id": shiftID,
		}, 200))
		if physical := fixture.movementPhysical(t, "payment_id", paidUSD.Payment.ID); !physical.is("0.00", "85.00", "10.00", "0.00") {
			t.Fatalf("El cobro con dólares no registró sus movimientos físicos: %+v", physical)
		}
		// Cobro de $50 con $100: el cambio de $50 sale en dólares (US$2.70).
		second := fixture.account(t, "Pesos con cambio en dólares", line(fixture.soda, 2))
		paidMXN := decodeResponse[paymentResult](t, fixture.pay(t, terminal, second.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "received": "100", "change_currency": "USD",
		}, 200))
		if physical := fixture.movementPhysical(t, "payment_id", paidMXN.Payment.ID); !physical.is("100.00", "0.00", "0.00", "2.70") {
			t.Fatalf("El cambio en dólares no salió de los dólares físicos: %+v", physical)
		}
		withdrawal := fixture.operation(t, fixture.shiftCall(t, terminal, "POST", shiftID, "withdrawals", map[string]any{
			"type": "safekeeping", "amount": "20", "currency": "USD",
		}, 200))
		if withdrawal.Currency != "USD" || text(withdrawal.USDRate) != "18.50" || withdrawal.EquivalentMXN != "370.00" {
			t.Fatalf("El retiro en dólares no conservó moneda y equivalencia: %+v", withdrawal)
		}
		if errorCode(t, fixture.shiftCall(t, terminal, "POST", shiftID, "withdrawals", map[string]any{
			"type": "other", "amount": "1000", "currency": "USD",
		}, 409)) != "CASH_INSUFFICIENT" {
			t.Fatal("Un retiro de dólares que excede los dólares disponibles se aceptó")
		}
		correction := decodeResponse[correctionResult](t, fixture.shiftCall(t, terminal, "POST", shiftID, "corrections", map[string]any{
			"corrects_operation_id": withdrawal.ID, "reason": "Retiro en dólares capturado por error",
		}, 200)).Correction
		if correction.Currency != "USD" || correction.Amount != "20.00" || correction.Direction != "in" || text(correction.USDRate) != "18.50" {
			t.Fatalf("La corrección en dólares no invirtió exactamente el retiro: %+v", correction)
		}
		fixture.shiftCall(t, terminal, "POST", shiftID, "withdrawals", map[string]any{"type": "transfer", "amount": "3.25", "currency": "USD"}, 200)
		fixture.shiftCall(t, terminal, "POST", shiftID, "income", map[string]any{"type": "change_supply", "amount": "3.25", "currency": "USD"}, 200)
		// Esperado por moneda: $1000 − $85 + $100 = $1015; US$50 + 10 − 2.70 − 20 + 20 − 3.25 + 3.25 = US$57.30.
		expected := fixture.expected(t, shiftID)
		if expected.MXN != "1015.00" || expected.USD != "57.30" || text(expected.USDRate) != "18.50" ||
			text(expected.USDEquivalentMXN) != "1060.05" || text(expected.TotalEquivalentMXN) != "2075.05" {
			t.Fatalf("El esperado por moneda o sus equivalencias son incorrectos: %+v", expected)
		}
		movements := decodeResponse[struct {
			ByCurrency map[string]string `json:"by_currency"`
		}](t, fixture.adminCall(t, "GET", "payments/shifts/"+shiftID+"/movements", nil, 200))
		if movements.ByCurrency["expected_mxn"] != "1015.00" || movements.ByCurrency["expected_usd"] != "57.30" ||
			movements.ByCurrency["usd_in"] != "10.00" || movements.ByCurrency["usd_out"] != "2.70" {
			t.Fatalf("Los movimientos del turno no cuentan cada moneda por separado: %+v", movements.ByCurrency)
		}
		cut := decodeResponse[payments.CashReport](t, fixture.shiftCall(t, terminal, "POST", shiftID, "partial-cuts", map[string]any{}, 200))
		usd := cashTotals(t, cut, "USD", false)
		if usd["withdrawals"] != "23.25" || usd["corrections_in"] != "20.00" || usd["income"] != "3.25" {
			t.Fatalf("El corte no separa las operaciones en dólares: %+v", usd)
		}
	})

	t.Run("C22_conteo_y_diferencias_por_moneda_con_sobrante_y_faltante", func(t *testing.T) {
		fixture.shiftCall(t, terminal, "POST", shiftID, "closing", map[string]any{}, 200)
		count := map[string]any{
			"lines": []map[string]any{
				fixture.line(t, "MXN 500.00 bill", 2), fixture.line(t, "MXN 10.00 coin", 3), fixture.line(t, "MXN 5.00 coin", 1),
				fixture.line(t, "USD 20.00 bill", 2), fixture.line(t, "USD 5.00 bill", 1),
			},
			"extra_mxn": "0.50", "extra_usd": "2.30", "extra_note": "Monedas sueltas sin desglosar",
		}
		saved := fixture.saveCount(t, terminal, shiftID, count, "Cierre bimoneda")
		if saved.Count.CountedMXN != "1035.50" || saved.Count.CountedUSD != "47.30" {
			t.Fatalf("El conteo no totalizó cada moneda: %+v", saved.Count)
		}
		fixture.shiftCall(t, terminal, "POST", shiftID, "closing/confirm", map[string]any{"count_revision": saved.Count.Revision}, 200)
		closure := decodeResponse[payments.ClosureView](t, fixture.shiftCall(t, fixture.admin, "GET", shiftID, "closure", nil, 200))
		differences := closure.Differences
		// Sobrante de $20.50 y faltante de US$10.00, que permanece en dólares; su equivalente
		// histórico es $20.50 − $185.00 = −$164.50 con el tipo de cambio del turno.
		if differences.MXN != "20.50" || differences.ResultMXN != "over" || differences.USD != "-10.00" || differences.ResultUSD != "short" ||
			text(differences.EquivalentMXN) != "-164.50" || closure.Expected.USD != "57.30" || text(closure.Expected.USDEquivalentMXN) != "1060.05" {
			t.Fatalf("Las diferencias por moneda no se conservaron: %+v %+v", closure.Expected, differences)
		}
		var rate string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT usd_rate::text FROM app.cash_shift_closures WHERE shift_id=$1", shiftID).Scan(&rate); err != nil ||
			rate != "18.50" {
			t.Fatalf("El cierre no conservó el tipo de cambio histórico del turno: %s %v", rate, err)
		}
		fixture.setUSDFeature(t, false)
		// Desactivar la función conserva los registros en dólares del cierre.
		again := decodeResponse[payments.ClosureView](t, fixture.shiftCall(t, fixture.admin, "GET", shiftID, "closure", nil, 200))
		if again.Differences.USD != "-10.00" || again.Count.CountedUSD != "47.30" {
			t.Fatal("Desactivar los dólares alteró un cierre histórico")
		}
	})
}
