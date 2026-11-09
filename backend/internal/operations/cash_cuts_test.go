//go:build integration

package operations_test

import (
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/payments"
)

func cashTotals(t *testing.T, report payments.CashReport, currency string, since bool) map[string]string {
	t.Helper()
	totals := report.Snapshot.Accumulated
	if since {
		totals = report.Snapshot.SincePrevious
	}
	for _, item := range totals.Cash {
		if item.Currency == currency {
			return map[string]string{
				"income": item.Income, "withdrawals": item.Withdrawals, "expenses": item.Expenses,
				"corrections_in": item.CorrectionsIn, "corrections_out": item.CorrectionsOut,
			}
		}
	}
	t.Fatalf("El corte no contiene operaciones en %s", currency)
	return nil
}

func testCashPartialCuts(t *testing.T, fixture *cashFixture) {
	shiftID := func() string {
		return fixture.shifts["A"]
	}

	t.Run("C11_varios_cortes_parciales_informativos_e_idempotentes", func(t *testing.T) {
		expectedBefore := fixture.expected(t, shiftID())
		operationsBefore := fixture.countRows(t, "SELECT count(*) FROM app.cash_operations WHERE shift_id=$1", shiftID())
		movementsBefore := fixture.countRows(t, "SELECT count(*) FROM app.cash_movements WHERE shift_id=$1", shiftID())
		fixture.shiftCall(t, fixture.observer, "POST", shiftID(), "partial-cuts", map[string]any{}, 403)
		body := map[string]any{"operation_id": randomID()}
		created := fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "partial-cuts", body, 200)
		blind(t, created, "La creación del corte parcial")
		first := decodeResponse[payments.CashReport](t, created)
		repeated := decodeResponse[payments.CashReport](t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "partial-cuts", body, 200))
		if first.ID != repeated.ID || first.Kind != "partial" || first.Sequence != 1 ||
			fixture.countRows(t, "SELECT count(*) FROM app.cash_shift_reports WHERE shift_id=$1", shiftID()) != 1 {
			t.Fatal("Un reintento del corte parcial generó otro corte")
		}
		// El corte no mueve dinero, no cierra el turno y no genera movimientos.
		if fixture.expected(t, shiftID()).MXN != expectedBefore.MXN ||
			fixture.countRows(t, "SELECT count(*) FROM app.cash_operations WHERE shift_id=$1", shiftID()) != operationsBefore ||
			fixture.countRows(t, "SELECT count(*) FROM app.cash_movements WHERE shift_id=$1", shiftID()) != movementsBefore ||
			fixture.detail(t, fixture.cashier, shiftID()).Shift.State != "open" {
			t.Fatal("El corte parcial modificó el saldo, los movimientos o el estado del turno")
		}
		accumulated := first.Snapshot.Accumulated
		if accumulated.Payments < 3 || accumulated.CashOperations != operationsBefore || accumulated.Sales == "0.00" ||
			len(accumulated.Methods) < 2 || first.Snapshot.Control.RegisterName != "Caja de arqueo" {
			t.Fatalf("El snapshot del corte no contiene los totales del turno: %+v", accumulated)
		}
		totals := cashTotals(t, first, "MXN", false)
		if totals["withdrawals"] == "0.00" || totals["expenses"] != "75.90" || totals["corrections_out"] != "200.00" || totals["corrections_in"] != "50.00" {
			t.Fatalf("El corte no distingue retiros, gastos y correcciones: %+v", totals)
		}
		fixture.reports["first"] = first.ID
		// Un movimiento posterior aparece solo en lo ocurrido desde el corte anterior.
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "income", map[string]any{"type": "other", "amount": "75", "concept": "Venta de cartón"}, 200)
		second := decodeResponse[payments.CashReport](t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID(), "partial-cuts", map[string]any{}, 200))
		since := cashTotals(t, second, "MXN", true)
		if second.Sequence != 2 || text(second.PreviousReportID) != first.ID || !second.CoveredFrom.Equal(first.CoveredTo) ||
			since["income"] != "75.00" || since["withdrawals"] != "0.00" || second.Snapshot.SincePrevious.CashOperations != 1 ||
			second.Snapshot.SincePrevious.Payments != 0 {
			t.Fatalf("El segundo corte no distingue lo ocurrido desde el anterior: %+v", second.Snapshot.SincePrevious)
		}
		accumulatedIncome := amountOf(t, cashTotals(t, second, "MXN", false)["income"])
		if !accumulatedIncome.Sub(amountOf(t, totals["income"])).Equal(amountOf(t, "75")) {
			t.Fatal("El acumulado del segundo corte no incluye exactamente el ingreso nuevo")
		}
		fixture.reports["second"] = second.ID
		// La consulta oculta el esperado sin permiso; el snapshot lo conserva.
		route := "payments/cash-reports/" + second.ID
		hidden := fixture.call(t, fixture.cashier, "GET", route, nil, 200)
		blind(t, hidden, "El corte parcial consultado sin permiso")
		blind(t, fixture.call(t, fixture.observer, "GET", route, nil, 200), "El corte parcial consultado por un observador")
		visible := decodeResponse[payments.CashReport](t, fixture.adminCall(t, "GET", route, nil, 200))
		current := fixture.expected(t, shiftID())
		if !visible.ExpectedVisible || visible.Snapshot.Sensitive == nil || visible.Snapshot.Sensitive.Expected.MXN != current.MXN {
			t.Fatalf("El corte con permiso no muestra el esperado del snapshot: %+v", visible.Snapshot.Sensitive)
		}
		var stored string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT snapshot->'sensitive'->'expected'->>'mxn' FROM app.cash_shift_reports WHERE id=$1",
			first.ID).Scan(&stored); err != nil || stored != expectedBefore.MXN {
			t.Fatalf("El snapshot no guardó el esperado del momento del corte: %s %v", stored, err)
		}
		detail := fixture.detail(t, fixture.cashier, shiftID())
		if len(detail.Reports) != 2 || detail.Summary.PartialCuts != 2 {
			t.Fatalf("El detalle del turno no lista los cortes: %+v", detail.Reports)
		}
		if !strings.Contains(string(fixture.call(t, fixture.cashier, "GET", route, nil, 200)), `"expected_visible":false`) {
			t.Fatal("El corte no indica que el esperado se omitió")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.partial_cut_created'") != 2 ||
			fixture.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='cash.report.created' AND scope='cash'") != 2 {
			t.Fatal("Los cortes no quedaron auditados ni publicaron su evento")
		}
	})
}
