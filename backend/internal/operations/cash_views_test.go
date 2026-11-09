//go:build integration

package operations_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/payments"
)

// summaryView es el resumen en vivo del turno tal como lo recibe la interfaz.
type summaryView struct {
	Shift           payments.Shift              `json:"shift"`
	Blind           bool                        `json:"blind"`
	PartialCuts     int                         `json:"partial_cuts"`
	LastCut         *payments.CashReportSummary `json:"last_cut"`
	Accumulated     *json.RawMessage            `json:"accumulated"`
	SinceLastCut    *json.RawMessage            `json:"since_last_cut"`
	ExpectedVisible bool                        `json:"expected_visible"`
	Expected        *payments.CashExpectation   `json:"expected"`
	Physical        *payments.PhysicalCash      `json:"physical"`
	Closure         *payments.ClosureSummary    `json:"closure"`
}

// totalsOf decodifica los totales de un resumen o de un corte.
func totalsOf(t *testing.T, raw *json.RawMessage) payments.CashReport {
	t.Helper()
	if raw == nil {
		t.Fatal("El resumen no incluyó totales")
	}
	var report payments.CashReport
	if err := json.Unmarshal(*raw, &report.Snapshot.Accumulated); err != nil {
		t.Fatal(err)
	}
	return report
}

// sameJSON compara dos documentos JSON por su contenido, sin depender del formato.
func sameJSON(t *testing.T, first, second []byte) bool {
	t.Helper()
	var left, right any
	if err := json.Unmarshal(first, &left); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second, &right); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(left, right)
}

// writeCounts cuenta los registros que una consulta de solo lectura nunca debe crear.
func (fixture *cashFixture) writeCounts(t *testing.T) [4]int {
	t.Helper()
	return [4]int{
		fixture.countRows(t, "SELECT count(*) FROM app.cash_counts"),
		fixture.countRows(t, "SELECT count(*) FROM app.cash_count_lines"),
		fixture.countRows(t, "SELECT count(*) FROM app.restaurant_operations"),
		// Un 403 audita su denegación (access.denied); eso no es una escritura de la consulta.
		fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action<>'access.denied'"),
	}
}

// testCashViews prueba las consultas de solo lectura de la fase B: la vista previa del conteo y
// el resumen en vivo del turno. Usa una caja propia para no alterar los turnos anteriores.
func testCashViews(t *testing.T, fixture *cashFixture) {
	registerID := fixture.createRegister(t, "Caja de consultas")
	cashier := newCashier(t, fixture.paymentsFixture, "cajero_consultas", cashierPermissions, registerID)
	viewer := newCashier(t, fixture.paymentsFixture, "observador_consultas", []string{"payments.read", "cash.shifts.read"}, registerID)
	preview := func(terminal testTerminal, body map[string]any, status int) []byte {
		t.Helper()
		return fixture.call(t, terminal, "POST", "payments/cash/count-preview", body, status)
	}
	var shiftID string

	t.Run("C26_catalogo_de_denominaciones_en_orden_numerico", func(t *testing.T) {
		// El catálogo llega por moneda, billetes antes que monedas y valor descendente (numérico).
		catalog := decodeResponse[struct {
			Denominations []payments.Denomination `json:"denominations"`
		}](t, fixture.call(t, cashier, "GET", "payments/cash/catalog", nil, 200))
		order := []string{}
		for _, denomination := range catalog.Denominations {
			if denomination.Currency == "MXN" {
				order = append(order, denomination.Kind+" "+denomination.Value)
			}
		}
		expected := "bill 1000.00,bill 500.00,bill 200.00,bill 100.00,bill 50.00,bill 20.00," +
			"coin 20.00,coin 10.00,coin 5.00,coin 2.00,coin 1.00,coin 0.50"
		if strings.Join(order, ",") != expected {
			t.Fatalf("Las denominaciones no están en orden numérico: %v", order)
		}
	})

	t.Run("C27_vista_previa_del_conteo_valida_y_totaliza_sin_escribir", func(t *testing.T) {
		before := fixture.writeCounts(t)
		count := fixture.pesosCount(t, "1234.50")
		result := decodeResponse[payments.CountPreview](t, preview(cashier, count, 200))
		if result.CountedMXN != "1234.50" || result.CountedUSD != "0.00" || result.ExtraMXN != "4.50" || len(result.Lines) != 3 {
			t.Fatalf("La vista previa no totalizó el conteo: %+v", result)
		}
		if result.Lines[0].Value != "500.00" || result.Lines[0].Quantity != 2 || result.Lines[0].Amount != "1000.00" {
			t.Fatalf("La vista previa no calculó el importe de cada denominación: %+v", result.Lines[0])
		}
		// Sin permiso de abrir turnos ni de realizar arqueos no se calcula nada.
		preview(viewer, count, 403)
		// La misma validación del arqueo: denominación repetida, adicional sin identificar y dólares
		// con la función desactivada.
		repeated := fixture.line(t, "MXN 500.00 bill", 1)
		preview(cashier, map[string]any{"lines": []map[string]any{repeated, repeated}}, 400)
		preview(cashier, map[string]any{"lines": []map[string]any{}, "extra_mxn": "3"}, 400)
		if errorCode(t, preview(cashier, map[string]any{"lines": []map[string]any{fixture.line(t, "USD 20.00 bill", 1)}}, 409)) != "FEATURE_DISABLED" {
			t.Fatal("La vista previa aceptó dólares con la función desactivada")
		}
		if fixture.writeCounts(t) != before {
			t.Fatal("La vista previa del conteo escribió registros")
		}
	})

	t.Run("C28_resumen_del_turno_con_y_sin_permiso", func(t *testing.T) {
		opening := map[string]any{"opening_float": "300", "count": fixture.pesosCount(t, "300")}
		shiftID = decodeResponse[payments.Shift](t, fixture.openShift(t, cashier, opening, 200)).ID
		fixture.shiftCall(t, cashier, "POST", shiftID, "income", map[string]any{"type": "change_supply", "amount": "100"}, 200)
		route := "payments/shifts/" + shiftID + "/summary"
		// Sin el permiso: totales del corte, sin esperado ni efectivo físico por moneda.
		hidden := fixture.call(t, cashier, "GET", route, nil, 200)
		blind(t, hidden, "El resumen sin permiso de consultar el esperado")
		if strings.Contains(string(hidden), `"physical"`) {
			t.Fatal("El resumen sin permiso incluyó el efectivo físico por moneda")
		}
		own := decodeResponse[summaryView](t, hidden)
		totals := totalsOf(t, own.Accumulated)
		if own.Blind || own.ExpectedVisible || own.PartialCuts != 0 || own.LastCut != nil ||
			cashTotals(t, totals, "MXN", false)["income"] != "100.00" || totals.Snapshot.Accumulated.CashOperations != 1 {
			t.Fatalf("El resumen sin permiso no contiene los totales del turno: %s", hidden)
		}
		blind(t, fixture.call(t, viewer, "GET", route, nil, 200), "El resumen visto por un observador")
		// Otra caja sin permiso para consultarla.
		fixture.call(t, fixture.cashier, "GET", route, nil, 403)
		// Con el permiso: el esperado de la misma función que cortes y cierre.
		visible := decodeResponse[summaryView](t, fixture.adminCall(t, "GET", route, nil, 200))
		if !visible.ExpectedVisible || visible.Expected == nil || visible.Expected.MXN != "400.00" || visible.Physical == nil ||
			visible.Physical.MXNIn != "0.00" || visible.Expected.MXN != fixture.expected(t, shiftID).MXN {
			t.Fatalf("El resumen con permiso no muestra el esperado: %+v", visible)
		}
		// El resumen coincide con el corte que se registraría en ese momento.
		cut := decodeResponse[payments.CashReport](t, fixture.shiftCall(t, cashier, "POST", shiftID, "partial-cuts", map[string]any{}, 200))
		stored := decodeResponse[struct {
			Snapshot struct {
				Accumulated json.RawMessage `json:"accumulated"`
			} `json:"snapshot"`
		}](t, fixture.adminCall(t, "GET", "payments/cash-reports/"+cut.ID, nil, 200))
		if !sameJSON(t, *visible.Accumulated, stored.Snapshot.Accumulated) {
			t.Fatalf("El resumen no usa el cálculo del corte:\n%s\n%s", *visible.Accumulated, stored.Snapshot.Accumulated)
		}
		fixture.shiftCall(t, cashier, "POST", shiftID, "income", map[string]any{"type": "other", "amount": "50", "concept": "Venta de cartón"}, 200)
		after := decodeResponse[summaryView](t, fixture.call(t, cashier, "GET", route, nil, 200))
		since := totalsOf(t, after.SinceLastCut)
		if after.PartialCuts != 1 || after.LastCut == nil || after.LastCut.ID != cut.ID ||
			cashTotals(t, since, "MXN", false)["income"] != "50.00" ||
			cashTotals(t, totalsOf(t, after.Accumulated), "MXN", false)["income"] != "150.00" {
			t.Fatalf("El resumen no distingue lo ocurrido desde el último corte: %+v", after)
		}
	})

	t.Run("C29_resumen_y_vista_previa_ciegos_durante_el_arqueo", func(t *testing.T) {
		started := decodeResponse[payments.ClosingView](t, fixture.shiftCall(t, cashier, "POST", shiftID, "closing", map[string]any{}, 200))
		route := "payments/shifts/" + shiftID + "/summary"
		for _, terminal := range []testTerminal{cashier, fixture.admin} {
			body := fixture.call(t, terminal, "GET", route, nil, 200)
			blind(t, body, "El resumen durante el arqueo")
			for _, fragment := range []string{`"accumulated"`, `"since_last_cut"`, `"physical"`} {
				if strings.Contains(string(body), fragment) {
					t.Fatalf("El resumen durante el arqueo reveló %s: %s", fragment, body)
				}
			}
			if !decodeResponse[summaryView](t, body).Blind {
				t.Fatal("El resumen no indica que el arqueo es ciego")
			}
		}
		before := fixture.writeCounts(t)
		count := fixture.pesosCount(t, "380")
		count["shift_id"] = shiftID
		result := preview(cashier, count, 200)
		blind(t, result, "La vista previa del arqueo")
		if decodeResponse[payments.CountPreview](t, result).CountedMXN != "380.00" {
			t.Fatalf("La vista previa del arqueo no totalizó el conteo: %s", result)
		}
		// Quien no tiene acceso a la caja no puede calcular conteos de su turno.
		fixture.call(t, fixture.cashier, "POST", "payments/cash/count-preview", count, 403)
		closing := fixture.readClosing(t, cashier, shiftID)
		if fixture.writeCounts(t) != before || closing.Count.Revision != started.Count.Revision || closing.Count.CountedMXN != "0.00" {
			t.Fatal("La vista previa del arqueo modificó el borrador o escribió registros")
		}
		delete(count, "shift_id")
		saved := fixture.saveCount(t, cashier, shiftID, count, "Cierre de consultas")
		fixture.shiftCall(t, cashier, "POST", shiftID, "closing/confirm", map[string]any{"count_revision": saved.Count.Revision}, 200)
	})

	t.Run("C30_resumen_de_un_turno_cerrado_segun_permisos", func(t *testing.T) {
		route := "payments/shifts/" + shiftID + "/summary"
		closer := fixture.call(t, cashier, "GET", route, nil, 200)
		blind(t, closer, "El resumen del turno cerrado sin permiso")
		own := decodeResponse[summaryView](t, closer)
		if own.Shift.State != "closed" || own.Closure == nil || own.Closure.CountedMXN != "380.00" || own.Accumulated == nil {
			t.Fatalf("Quien cerró no ve lo que declaró: %s", closer)
		}
		full := decodeResponse[summaryView](t, fixture.adminCall(t, "GET", route, nil, 200))
		if !full.ExpectedVisible || full.Closure == nil || text(full.Closure.DifferenceMXN) != "-70.00" || text(full.Closure.ResultMXN) != "short" ||
			full.Expected == nil || full.Expected.MXN != "450.00" {
			t.Fatalf("El resumen del turno cerrado no muestra el resultado del cierre: %+v", full.Closure)
		}
		// Otro usuario de la caja sin historial no consulta turnos cerrados.
		fixture.call(t, viewer, "GET", route, nil, 403)
	})
}
