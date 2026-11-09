//go:build integration

package operations_test

import (
	"encoding/json"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/features"
	"local/ad-el-pos/server/internal/payments"
)

type currentShiftResult struct {
	Shift           *payments.Shift `json:"shift"`
	Usable          bool            `json:"usable"`
	USDCash         bool            `json:"usd_cash"`
	USDRateRequired bool            `json:"usd_rate_required"`
}

type physicalRow struct {
	mxnIn, mxnOut, usdIn, usdOut, rate *string
}

func (fixture *paymentsFixture) usdFeature(t *testing.T) features.Feature {
	t.Helper()
	for _, feature := range decodeResponse[[]features.Feature](t, fixture.adminCall(t, "GET", "settings/features", nil, 200)) {
		if feature.Code == features.USDCash {
			return feature
		}
	}
	t.Fatal("La función de pago en dólares no aparece en la lista")
	return features.Feature{}
}

func (fixture *paymentsFixture) setUSDFeature(t *testing.T, enabled bool) features.Feature {
	t.Helper()
	current := fixture.usdFeature(t)
	return decodeResponse[features.Feature](t, fixture.adminCall(t, "PUT", "settings/features", map[string]any{
		"code": features.USDCash, "enabled": enabled, "revision": current.Revision,
	}, 200))
}

// movementPhysical lee el efecto físico por moneda del movimiento de un pago o devolución.
func (fixture *paymentsFixture) movementPhysical(t *testing.T, column, id string) physicalRow {
	t.Helper()
	var row physicalRow
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT mxn_in::text,mxn_out::text,usd_in::text,usd_out::text,usd_rate::text
        FROM app.cash_movements WHERE `+column+`=$1`, id).Scan(&row.mxnIn, &row.mxnOut, &row.usdIn, &row.usdOut, &row.rate); err != nil {
		t.Fatal(err)
	}
	return row
}

func text(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}

func (row physicalRow) is(mxnIn, mxnOut, usdIn, usdOut string) bool {
	return text(row.mxnIn) == mxnIn && text(row.mxnOut) == mxnOut && text(row.usdIn) == usdIn && text(row.usdOut) == usdOut
}

func testPaymentsUSD(t *testing.T, fixture *paymentsFixture) {
	cash := fixture.methods["cash"].ID
	debit := fixture.methods["debit"].ID
	shiftID := fixture.registers["shift"]

	t.Run("U1_funcion_desactivada_rechaza_dolares_aunque_se_llame_a_la_api", func(t *testing.T) {
		if fixture.usdFeature(t).Enabled {
			t.Fatal("El pago en dólares no inicia desactivado")
		}
		// Cualquier sesión autenticada consulta la lista para decidir qué mostrar.
		fixture.call(t, fixture.observer, "GET", "settings/features", nil, 200)
		bill := fixture.account(t, "Dólares desactivados", line(fixture.soda, 2))
		before := fixture.countRows(t, "SELECT count(*) FROM app.payments")
		for _, body := range []map[string]any{
			{"method_id": cash, "mode": "total", "received_usd": "10"},
			{"method_id": cash, "mode": "total", "received_usd": "0"},
			{"method_id": cash, "mode": "total", "change_currency": "USD"},
			{"method_id": debit, "mode": "total", "received_usd": "10"},
		} {
			preview := fixture.adminCall(t, "POST", "payments/accounts/"+bill.Account.ID+"/preview", body, 409)
			if errorCode(t, preview) != "FEATURE_DISABLED" {
				t.Fatalf("La vista previa no rechazó dólares con la función desactivada: %s", preview)
			}
			if errorCode(t, fixture.pay(t, fixture.admin, bill.Account.ID, body, 409)) != "FEATURE_DISABLED" {
				t.Fatal("El cobro no rechazó dólares con la función desactivada")
			}
		}
		if errorCode(t, fixture.adminCall(t, "POST", "payments/shifts/"+shiftID+"/usd-rate", map[string]any{
			"operation_id": randomID(), "usd_rate": "18.50",
		}, 409)) != "FEATURE_DISABLED" {
			t.Fatal("Se capturó un tipo de cambio con la función desactivada")
		}
		if errorCode(t, fixture.adminCall(t, "POST", "payments/shifts", map[string]any{
			"operation_id": randomID(), "opening_float": "0", "usd_rate": "18.50",
		}, 409)) != "FEATURE_DISABLED" {
			t.Fatal("La apertura de turno aceptó tipo de cambio con la función desactivada")
		}
		settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
		if errorCode(t, fixture.adminCall(t, "PUT", "payments/settings/usd", map[string]any{
			"operation_id": randomID(), "revision": settings.Revision, "usd_rate_min": "15", "usd_rate_max": "25", "usd_rate_alert_percent": "5",
		}, 409)) != "FEATURE_DISABLED" {
			t.Fatal("Se configuró el rango del dólar con la función desactivada")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.payments") != before {
			t.Fatal("Un dato en dólares rechazado dejó pagos")
		}
		// Sin la función, la vista previa y el turno quedan exactamente como en la etapa 6.
		preview := fixture.adminCall(t, "POST", "payments/accounts/"+bill.Account.ID+"/preview", map[string]any{"method_id": cash, "mode": "total"}, 200)
		if strings.Contains(string(preview), `"usd"`) {
			t.Fatalf("La vista previa en pesos mostró cifras en dólares: %s", preview)
		}
		current := decodeResponse[currentShiftResult](t, fixture.adminCall(t, "GET", "payments/shifts/current", nil, 200))
		if current.USDCash || current.USDRateRequired || current.Shift.USDRate != nil {
			t.Fatal("El turno mostró datos en dólares con la función desactivada")
		}
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received": "100"}, 200)
		fixture.registers["pesos_payment_account"] = bill.Account.ID
	})

	t.Run("U2_activar_con_turno_abierto_exige_capturar_el_tipo_de_cambio_una_vez", func(t *testing.T) {
		current := fixture.usdFeature(t)
		fixture.call(t, fixture.observer, "PUT", "settings/features", map[string]any{"code": features.USDCash, "enabled": true, "revision": current.Revision}, 403)
		fixture.adminCall(t, "PUT", "settings/features", map[string]any{"code": features.USDCash, "enabled": true, "revision": current.Revision + 1}, 409)
		fixture.adminCall(t, "PUT", "settings/features", map[string]any{"code": "licencias", "enabled": true, "revision": 0}, 404)
		saved := fixture.setUSDFeature(t, true)
		if !saved.Enabled || saved.Revision != current.Revision+1 || saved.UpdatedByName == nil {
			t.Fatalf("La función no se activó con revisión y responsable: %+v", saved)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='settings.feature_changed'") != 1 ||
			fixture.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='settings.features.changed'") != 1 {
			t.Fatal("Activar la función no quedó auditado ni publicó su evento")
		}
		state := decodeResponse[currentShiftResult](t, fixture.adminCall(t, "GET", "payments/shifts/current", nil, 200))
		if !state.USDCash || !state.USDRateRequired {
			t.Fatal("El turno abierto sin tipo de cambio no lo exige")
		}
		bill := fixture.account(t, "Turno sin tipo de cambio", line(fixture.soda, 2))
		if errorCode(t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received_usd": "5"}, 409)) != "USD_RATE_REQUIRED" {
			t.Fatal("Un turno sin tipo de cambio aceptó dólares")
		}
		// Sin dólares, el turno sin tipo de cambio sigue cobrando en pesos.
		pesos := fixture.adminCall(t, "POST", "payments/accounts/"+bill.Account.ID+"/preview", map[string]any{"method_id": cash, "mode": "total"}, 200)
		if strings.Contains(string(pesos), `"usd"`) {
			t.Fatal("Sin tipo de cambio la vista previa no puede expresar dólares")
		}
		route := "payments/shifts/" + shiftID + "/usd-rate"
		for _, rate := range []string{"1.95", "45", "abc", "19.505", "0", ""} {
			fixture.adminCall(t, "POST", route, map[string]any{"operation_id": randomID(), "usd_rate": rate}, 400)
		}
		fixture.call(t, fixture.observer, "POST", route, map[string]any{"operation_id": randomID(), "usd_rate": "18.50"}, 403)
		shift := decodeResponse[payments.Shift](t, fixture.adminCall(t, "POST", route, map[string]any{"operation_id": randomID(), "usd_rate": "18.50"}, 200))
		if text(shift.USDRate) != "18.50" || shift.USDRateSetByName == nil || shift.USDRateSetAt == nil {
			t.Fatalf("El tipo de cambio no quedó fijo en el turno: %+v", shift)
		}
		if errorCode(t, fixture.adminCall(t, "POST", route, map[string]any{"operation_id": randomID(), "usd_rate": "19"}, 409)) != "USD_RATE_ALREADY_SET" {
			t.Fatal("Se cambió el tipo de cambio de un turno que ya lo tenía")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.shift_usd_rate_set'") != 1 {
			t.Fatal("La captura del tipo de cambio no quedó auditada")
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.cash_shifts SET usd_rate_set_by=NULL WHERE id=$1", shiftID); err == nil {
			t.Fatal("PostgreSQL aceptó un tipo de cambio sin responsable")
		}
		fixture.registers["usd_account"] = bill.Account.ID
	})

	t.Run("U3_pesos_y_dolares_en_la_misma_operacion_con_cambio_en_ambas_monedas", func(t *testing.T) {
		bill := fixture.account(t, "Pesos y dólares", line(fixture.burger, 2), line(fixture.soda, 2))
		body := map[string]any{"method_id": cash, "mode": "total", "received": "100", "received_usd": "20"}
		plan := decodeResponse[payments.PaymentPlan](t, fixture.adminCall(t, "POST", "payments/accounts/"+bill.Account.ID+"/preview", body, 200))
		usd := plan.USD
		if usd == nil || usd.Rate != "18.50" || usd.BalanceUSD != "15.68" || usd.ReceivedUSDMXN != "370.00" || usd.ReceivedTotal != "470.00" {
			t.Fatalf("La vista previa no convirtió lo recibido: %+v", usd)
		}
		if plan.Change != "180.00" || usd.ChangeMXN != "180.00" || usd.ChangeUSD != "9.73" || usd.ChangeCurrency != "MXN" || !usd.Covered {
			t.Fatalf("El cambio no se expresó en pesos y dólares: %s %+v", plan.Change, usd)
		}
		result := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, body, 200))
		payment := result.Payment
		if payment.Amount != "290.00" || text(payment.Received) != "470.00" || payment.Change != "180.00" ||
			text(payment.ReceivedMXN) != "100.00" || text(payment.ReceivedUSD) != "20.00" || text(payment.USDRate) != "18.50" ||
			text(payment.ChangeCurrency) != "MXN" || payment.ChangeUSD != nil {
			t.Fatalf("El pago no guardó pesos, dólares, tipo de cambio y moneda del cambio: %+v", payment)
		}
		if !result.Closed {
			t.Fatal("El pago en dólares no liquidó la cuenta")
		}
		physical := fixture.movementPhysical(t, "payment_id", payment.ID)
		if !physical.is("100.00", "180.00", "20.00", "0.00") || text(physical.rate) != "18.50" {
			t.Fatalf("El movimiento no distinguió pesos y dólares físicos: %+v", physical)
		}
		var net, received, change string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT net_amount::text,cash_received::text,change_given::text FROM app.cash_movements WHERE payment_id=$1",
			payment.ID).Scan(&net, &received, &change); err != nil || net != "290.00" || received != "470.00" || change != "180.00" {
			t.Fatal("El movimiento en pesos equivalentes cambió de significado")
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.payments SET received_usd=21 WHERE id=$1", payment.ID); err == nil {
			t.Fatal("PostgreSQL aceptó dólares que no corresponden al total recibido")
		}
	})

	t.Run("U4_cambio_entregado_en_dolares_faltante_y_validaciones", func(t *testing.T) {
		bill := fixture.account(t, "Cambio en dólares", line(fixture.soda, 2))
		short := decodeResponse[payments.PaymentPlan](t, fixture.adminCall(t, "POST", "payments/accounts/"+bill.Account.ID+"/preview", map[string]any{
			"method_id": cash, "mode": "total", "received_usd": "2",
		}, 200))
		if short.USD == nil || short.USD.Covered || short.USD.MissingMXN != "13.00" || short.USD.MissingUSD != "0.71" {
			t.Fatalf("La vista previa no informó el faltante en ambas monedas: %+v", short.USD)
		}
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received_usd": "2"}, 400)
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": debit, "mode": "total", "received_usd": "5"}, 400)
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received_usd": "5", "change_currency": "EUR"}, 400)
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received_usd": "-5"}, 400)
		result := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "received_usd": "5", "change_currency": "USD",
		}, 200))
		payment := result.Payment
		if text(payment.Received) != "92.50" || payment.Change != "42.50" || text(payment.ChangeCurrency) != "USD" || text(payment.ChangeUSD) != "2.30" {
			t.Fatalf("El cambio en dólares no se registró: %+v", payment)
		}
		if physical := fixture.movementPhysical(t, "payment_id", payment.ID); !physical.is("0.00", "0.00", "5.00", "2.30") {
			t.Fatalf("El cambio en dólares no salió de los dólares físicos: %+v", physical)
		}
		// Un efectivo en pesos con la función activa también separa lo físico por moneda.
		pesos := fixture.account(t, "Solo pesos con función activa", line(fixture.soda, 1))
		paid := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, pesos.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received": "100"}, 200))
		if paid.Payment.ReceivedUSD != nil || !fixture.movementPhysical(t, "payment_id", paid.Payment.ID).is("100.00", "75.00", "0.00", "0.00") {
			t.Fatal("El efectivo en pesos no registró su efecto físico")
		}
	})

	t.Run("U5_idempotencia_y_concurrencia_de_cobros_con_dolares", func(t *testing.T) {
		bill := fixture.account(t, "Dólares idempotentes", line(fixture.soda, 4))
		body := map[string]any{
			"operation_id": randomID(), "method_id": cash, "mode": "partial", "amount": "40", "received_usd": "5",
			"expected_balance": fixture.finance(t, bill.Account.ID).Balance,
		}
		first := decodeResponse[paymentResult](t, fixture.call(t, fixture.admin, "POST", "payments/accounts/"+bill.Account.ID+"/payments", body, 200))
		repeated := decodeResponse[paymentResult](t, fixture.call(t, fixture.admin, "POST", "payments/accounts/"+bill.Account.ID+"/payments", body, 200))
		if first.Payment.ID != repeated.Payment.ID || text(repeated.Payment.ReceivedUSD) != "5.00" ||
			fixture.countRows(t, "SELECT count(*) FROM app.payments WHERE account_id=$1", bill.Account.ID) != 1 ||
			fixture.countRows(t, "SELECT count(*) FROM app.cash_movements WHERE account_id=$1", bill.Account.ID) != 1 {
			t.Fatal("Un reintento con el mismo identificador duplicó el pago en dólares")
		}
		body["received_usd"] = "6"
		if errorCode(t, fixture.call(t, fixture.admin, "POST", "payments/accounts/"+bill.Account.ID+"/payments", body, 409)) != "IDEMPOTENCY_CONFLICT" {
			t.Fatal("El mismo identificador con otros dólares no se rechazó")
		}
		cashier := newCashier(t, fixture, "cajero_dolares", []string{"accounts.read", "payments.read", "payments.collect", "payments.collect_partial"},
			fixture.registers["primary"])
		balance := fixture.finance(t, bill.Account.ID).Balance
		results := make(chan int, 2)
		for _, terminal := range []testTerminal{fixture.admin, cashier} {
			go func(terminal testTerminal) {
				encoded, _ := json.Marshal(map[string]any{
					"operation_id": randomID(), "method_id": cash, "mode": "partial", "amount": "30", "received_usd": "2",
					"expected_balance": balance,
				})
				results <- fixture.status(terminal, "POST", "payments/accounts/"+bill.Account.ID+"/payments", encoded)
			}(terminal)
		}
		assertOneWinner(t, []int{<-results, <-results})
		if fixture.finance(t, bill.Account.ID).Paid != "70.00" {
			t.Fatal("Dos cobros con dólares se aplicaron sobre el mismo saldo")
		}
	})

	t.Run("U6_correccion_de_metodo_invierte_los_movimientos_fisicos_por_moneda", func(t *testing.T) {
		bill := fixture.account(t, "Corrección en dólares", line(fixture.soda, 2))
		wrong := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": cash, "mode": "partial", "amount": "30", "received_usd": "5",
		}, 200))
		corrected := decodeResponse[struct {
			ReversalID string           `json:"reversal_id"`
			PaymentID  string           `json:"payment_id"`
			Finance    payments.Finance `json:"finance"`
		}](t, fixture.post(t, fixture.admin, bill.Account.ID, "corrections", map[string]any{
			"payment_id": wrong.Payment.ID, "method_id": debit, "reason": "Era tarjeta, no dólares",
		}, 200))
		if corrected.Finance.Paid != "30.00" || corrected.Finance.Balance != "20.00" {
			t.Fatal("La corrección de un pago en dólares duplicó o perdió el ingreso")
		}
		if reversal := fixture.movementPhysical(t, "refund_id", corrected.ReversalID); !reversal.is("62.50", "0.00", "0.00", "5.00") {
			t.Fatalf("El reverso no invirtió pesos y dólares físicos: %+v", reversal)
		}
		var mxn, usd string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT coalesce(sum(mxn_in-mxn_out),0)::text,coalesce(sum(usd_in-usd_out),0)::text
            FROM app.cash_movements WHERE account_id=$1 AND is_cash`, bill.Account.ID).Scan(&mxn, &usd); err != nil || mxn != "0.00" || usd != "0.00" {
			t.Fatalf("El cajón no quedó como antes del pago erróneo: pesos %s, dólares %s", mxn, usd)
		}
	})

	t.Run("U7_ticket_con_dolares_tipo_de_cambio_y_moneda_del_cambio", func(t *testing.T) {
		bill := fixture.account(t, "Ticket en dólares", line(fixture.burger, 1))
		printed := decodeResponse[struct {
			Printing printingResult `json:"printing"`
		}](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "received": "50", "received_usd": "5", "change_currency": "USD", "print": true,
		}, 200))
		var content string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT content FROM app.kitchen_print_jobs WHERE id=$1", printed.Printing.JobID).Scan(&content); err != nil {
			t.Fatal(err)
		}
		for _, fragment := range []string{"Pesos recibidos", "$50.00", "Dólares recibidos", "US$5.00", "Tipo de cambio", "$18.50/US$", "Total recibido", "$142.50", "Cambio en dólares", "US$1.22", "$22.50"} {
			if !strings.Contains(content, fragment) {
				t.Fatalf("El ticket no muestra %q:\n%s", fragment, content)
			}
		}
	})

	t.Run("U8_tipo_de_cambio_al_abrir_turno_rango_y_confirmacion_por_diferencia", func(t *testing.T) {
		register := decodeResponse[struct {
			ID string `json:"id"`
		}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": "Caja de dólares", "description": "", "active": true}, 200))
		opener := newCashier(t, fixture, "apertura_dolares", []string{"payments.read", "cash.shifts.open", "payments.usd_rate.manage"}, register.ID)
		withoutRate := newCashier(t, fixture, "apertura_sin_tipo", []string{"payments.read", "cash.shifts.open"}, register.ID)
		fixture.call(t, withoutRate, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0", "usd_rate": "18.50"}, 403)
		fixture.call(t, opener, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0", "usd_rate": "45"}, 400)
		problem := fixture.call(t, opener, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0", "usd_rate": "16"}, 409)
		if errorCode(t, problem) != "USD_RATE_CONFIRMATION_REQUIRED" || !strings.Contains(string(problem), "18.50") {
			t.Fatalf("Una diferencia grande contra el turno anterior no pidió confirmación: %s", problem)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_shifts WHERE register_id=$1", register.ID) != 0 {
			t.Fatal("Se abrió un turno sin confirmar la diferencia del tipo de cambio")
		}
		shift := decodeResponse[payments.Shift](t, fixture.call(t, opener, "POST", "payments/shifts", map[string]any{
			"operation_id": randomID(), "opening_float": "200", "usd_rate": "16", "confirm_rate_difference": true,
		}, 200))
		if text(shift.USDRate) != "16.00" || shift.OpeningFloat != "200.00" {
			t.Fatalf("El turno no conservó su tipo de cambio: %+v", shift)
		}
		settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
		if settings.USDRateMin != "10.00" || settings.USDRateMax != "40.00" || settings.USDRateAlertPercent != "10.00" {
			t.Fatalf("El rango inicial del tipo de cambio no es el sembrado: %+v", settings)
		}
		fixture.adminCall(t, "PUT", "payments/settings/usd", map[string]any{
			"operation_id": randomID(), "revision": settings.Revision, "usd_rate_min": "25", "usd_rate_max": "15", "usd_rate_alert_percent": "5",
		}, 400)
		fixture.adminCall(t, "PUT", "payments/settings/usd", map[string]any{
			"operation_id": randomID(), "revision": settings.Revision, "usd_rate_min": "15", "usd_rate_max": "25", "usd_rate_alert_percent": "101",
		}, 400)
		fixture.call(t, fixture.observer, "PUT", "payments/settings/usd", map[string]any{
			"operation_id": randomID(), "revision": settings.Revision, "usd_rate_min": "15", "usd_rate_max": "25", "usd_rate_alert_percent": "5",
		}, 403)
		saved := decodeResponse[payments.Settings](t, fixture.adminCall(t, "PUT", "payments/settings/usd", map[string]any{
			"operation_id": randomID(), "revision": settings.Revision, "usd_rate_min": "15", "usd_rate_max": "25", "usd_rate_alert_percent": "5",
		}, 200))
		if saved.USDRateMin != "15.00" || saved.USDRateMax != "25.00" || saved.USDRateAlertPercent != "5.00" ||
			fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='payments.usd_settings_changed'") != 1 {
			t.Fatal("El rango y el porcentaje de alerta no se guardaron ni auditaron")
		}
		movements := decodeResponse[struct {
			ByCurrency map[string]string `json:"by_currency"`
		}](t, fixture.adminCall(t, "GET", "payments/shifts/"+shiftID+"/movements", nil, 200))
		if movements.ByCurrency["usd_in"] == "0.00" || movements.ByCurrency["expected_usd"] == "" {
			t.Fatalf("Los movimientos del turno no cuentan los dólares por separado: %+v", movements.ByCurrency)
		}
	})

	t.Run("U9_desactivar_conserva_registros_y_devoluciones_solo_en_pesos", func(t *testing.T) {
		paymentsBefore := fixture.countRows(t, "SELECT count(*) FROM app.payments WHERE received_usd IS NOT NULL")
		fixture.setUSDFeature(t, false)
		if fixture.countRows(t, "SELECT count(*) FROM app.payments WHERE received_usd IS NOT NULL") != paymentsBefore || paymentsBefore == 0 {
			t.Fatal("Desactivar la función alteró los pagos en dólares registrados")
		}
		bill := fixture.account(t, "Dólares tras desactivar", line(fixture.soda, 1))
		if errorCode(t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received_usd": "5"}, 409)) != "FEATURE_DISABLED" {
			t.Fatal("La función desactivada aceptó dólares")
		}
		current := decodeResponse[currentShiftResult](t, fixture.adminCall(t, "GET", "payments/shifts/current", nil, 200))
		if current.USDCash || current.USDRateRequired {
			t.Fatal("El turno siguió ofreciendo dólares con la función desactivada")
		}
		// La devolución de un pago recibido en dólares sale en pesos.
		var paymentID, accountID string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT p.id,p.account_id FROM app.payments p WHERE p.received_usd IS NOT NULL
            AND p.corrects_payment_id IS NULL AND NOT EXISTS(SELECT FROM app.payment_refunds r WHERE r.payment_id=p.id)
            AND p.kind='general' ORDER BY p.created_at LIMIT 1`).Scan(&paymentID, &accountID); err != nil {
			t.Fatal(err)
		}
		refund := decodeResponse[refundResult](t, fixture.post(t, fixture.admin, accountID, "refunds", map[string]any{
			"mode": "payment", "payment_id": paymentID, "amount": "10", "method_id": cash, "reason": "Devolución de un pago en dólares",
		}, 200))
		if physical := fixture.movementPhysical(t, "refund_id", refund.RefundIDs[0]); !physical.is("0.00", "10.00", "0.00", "0.00") {
			t.Fatalf("La devolución no salió en pesos: %+v", physical)
		}
		finance := fixture.finance(t, accountID)
		for _, payment := range finance.Payments {
			if payment.ID == paymentID && payment.ReceivedUSD == nil {
				t.Fatal("La consulta perdió los dólares de un pago histórico")
			}
		}
	})
}
