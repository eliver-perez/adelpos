//go:build integration

package operations_test

import (
	"testing"

	"local/ad-el-pos/server/internal/payments"
)

type correctionResult struct {
	Correction payments.CashOperation `json:"correction"`
	Original   payments.CashOperation `json:"original"`
}

func testCashCorrections(t *testing.T, fixture *cashFixture) {
	shiftID := func() string {
		return fixture.shifts["A"]
	}

	t.Run("C10_correccion_de_ingreso_y_retiro_con_autorizacion_conservando_el_original", func(t *testing.T) {
		incomeID := fixture.operations["income"]
		before := fixture.expected(t, shiftID())
		body := map[string]any{"corrects_operation_id": incomeID, "reason": "Ingreso capturado dos veces"}
		// Tener permiso de registrar ingresos no implica autorización para corregirlos.
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "corrections", body, 403)) != "AUTHORIZATION_REQUIRED" {
			t.Fatal("Una corrección sin autorización no la pidió")
		}
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "corrections", map[string]any{
			"corrects_operation_id": incomeID, "reason": "No", "authorization": authorizationOf("supervisor_caja"),
		}, 400)
		body["authorization"] = authorizationOf("supervisor_caja")
		result := decodeResponse[correctionResult](t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "corrections", body, 200))
		correction := result.Correction
		if correction.Kind != "correction" || correction.Direction != "out" || correction.Amount != "200.00" || correction.Currency != "MXN" ||
			text(correction.CorrectsOperationID) != incomeID || correction.Regularization || correction.Status != "correction" ||
			text(correction.AuthorizationMode) != "supervisor" || correction.Reason != "Ingreso capturado dos veces" {
			t.Fatalf("La corrección no invierte exactamente el ingreso: %+v", correction)
		}
		if result.Original.Status != "corrected" || result.Original.Amount != "200.00" || result.Original.Kind != "income" {
			t.Fatalf("El ingreso original no se conservó: %+v", result.Original)
		}
		var amount, kind string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT amount::text,kind FROM app.cash_operations WHERE id=$1", incomeID).Scan(&amount, &kind); err != nil ||
			amount != "200.00" || kind != "income" {
			t.Fatal("La corrección modificó el registro original")
		}
		after := fixture.expected(t, shiftID())
		if !amountOf(t, before.MXN).Sub(amountOf(t, after.MXN)).Equal(amountOf(t, "200")) {
			t.Fatalf("La corrección del ingreso no actualizó el esperado: %s → %s", before.MXN, after.MXN)
		}
		if errorCode(t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "corrections", map[string]any{
			"corrects_operation_id": incomeID, "reason": "Segunda corrección", "authorization": authorizationOf("supervisor_caja"),
		}, 409)) != "OPERATION_ALREADY_CORRECTED" {
			t.Fatal("Un ingreso se corrigió dos veces")
		}
		fixture.shiftCall(t, fixture.manager, "POST", shiftID(), "corrections", map[string]any{
			"corrects_operation_id": correction.ID, "reason": "Corregir una corrección",
		}, 400)
		// El encargado autoriza con su propio permiso la corrección de un retiro.
		withdrawal := decodeResponse[correctionResult](t, fixture.shiftCall(t, fixture.manager, "POST", shiftID(), "corrections", map[string]any{
			"corrects_operation_id": fixture.operations["self_withdrawal"], "reason": "El retiro no se realizó",
		}, 200)).Correction
		if withdrawal.Direction != "in" || withdrawal.Amount != "50.00" || text(withdrawal.AuthorizationMode) != "self" {
			t.Fatalf("La corrección del retiro no lo invirtió: %+v", withdrawal)
		}
		restored := fixture.expected(t, shiftID())
		if !amountOf(t, restored.MXN).Sub(amountOf(t, after.MXN)).Equal(amountOf(t, "50")) {
			t.Fatal("La corrección del retiro no devolvió el efectivo esperado")
		}
		fixture.shiftCall(t, fixture.partner, "POST", shiftID(), "corrections", map[string]any{
			"corrects_operation_id": fixture.operations["expense"], "reason": "Sin permiso de corregir",
		}, 403)
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.operation_corrected'") != 2 {
			t.Fatal("Las correcciones no quedaron auditadas")
		}
	})
}
