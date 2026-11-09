//go:build integration

package operations_test

import (
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/payments"
)

func testCashOpening(t *testing.T, fixture *cashFixture) {
	t.Run("C1_apertura_por_denominaciones_validada_en_go", func(t *testing.T) {
		count := map[string]any{
			"lines":      []map[string]any{fixture.line(t, "MXN 500.00 bill", 2), fixture.line(t, "MXN 100.00 bill", 3)},
			"extra_mxn":  "40",
			"extra_note": "Monedas sueltas",
		}
		problem := fixture.openShift(t, fixture.cashier, map[string]any{"opening_float": "1350", "count": count}, 400)
		if errorCode(t, problem) != "COUNT_TOTAL_MISMATCH" {
			t.Fatalf("Un desglose que no coincide con el fondo no se rechazó: %s", problem)
		}
		if errorCode(t, fixture.openShift(t, fixture.cashier, map[string]any{"opening_float": "1350", "opening_float_usd": "10"}, 409)) != "FEATURE_DISABLED" {
			t.Fatal("Se aceptó un fondo en dólares con la función desactivada")
		}
		usdCount := map[string]any{"lines": []map[string]any{fixture.line(t, "USD 20.00 bill", 1)}}
		if errorCode(t, fixture.openShift(t, fixture.cashier, map[string]any{"opening_float": "0", "count": usdCount}, 409)) != "FEATURE_DISABLED" {
			t.Fatal("Se aceptó un desglose en dólares con la función desactivada")
		}
		fixture.openShift(t, fixture.cashier, map[string]any{"opening_float": "50", "count": map[string]any{"extra_mxn": "50"}}, 400)
		fixture.openShift(t, fixture.cashier, map[string]any{"opening_float": "100", "count": map[string]any{
			"lines": []map[string]any{fixture.line(t, "MXN 100.00 bill", -1)},
		}}, 400)
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_shifts WHERE register_id=$1", fixture.registerID) != 0 {
			t.Fatal("Una apertura rechazada dejó un turno")
		}
		count["extra_mxn"] = "50"
		shift := decodeResponse[payments.Shift](t, fixture.openShift(t, fixture.cashier, map[string]any{
			"opening_float": "1350", "count": count, "opening_notes": "Fondo entregado por gerencia",
		}, 200))
		if shift.State != "open" || shift.OpeningFloat != "1350.00" || shift.OpeningFloatUSD != nil || shift.OpeningNotes != "Fondo entregado por gerencia" {
			t.Fatalf("La apertura no conservó fondo y observaciones: %+v", shift)
		}
		var businessDate string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT (opened_at AT TIME ZONE 'America/Monterrey')::date::text FROM app.cash_shifts
            WHERE id=$1 AND business_date=(opened_at AT TIME ZONE 'America/Monterrey')::date`, shift.ID).Scan(&businessDate); err != nil {
			t.Fatal("La fecha comercial no es la fecha de apertura en la zona del establecimiento", err)
		}
		if shift.BusinessDate != businessDate {
			t.Fatalf("La fecha comercial expuesta (%s) difiere de la guardada (%s)", shift.BusinessDate, businessDate)
		}
		detail := fixture.detail(t, fixture.cashier, shift.ID)
		opening := detail.OpeningCount
		if opening == nil || opening.State != "final" || opening.CountedMXN != "1350.00" || opening.ExtraMXN != "50.00" || len(opening.Lines) != 2 {
			t.Fatalf("El desglose de apertura no quedó registrado: %+v", opening)
		}
		if opening.Lines[0].Amount != "1000.00" || opening.Lines[1].Amount != "300.00" {
			t.Fatalf("Go no calculó el importe de cada denominación: %+v", opening.Lines)
		}
		if detail.ExpectedVisible || detail.Expected != nil {
			t.Fatal("El cajero sin permiso recibió el efectivo esperado")
		}
		if errorCode(t, fixture.openShift(t, fixture.cashier, map[string]any{"opening_float": "0"}, 409)) != "SHIFT_ALREADY_OPEN" {
			t.Fatal("Se abrió un segundo turno en la misma caja")
		}
		fixture.openShift(t, fixture.partner, map[string]any{"opening_float": "0"}, 403)
		if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.cash_shifts(id,register_id,state,opening_float,opened_by,opened_device_id,
            operation_id,closing_started_by,closing_started_at,closing_device_id,closing_movement_count,closing_operation_count)
            SELECT gen_random_uuid(),register_id,'closing',0,opened_by,opened_device_id,gen_random_uuid(),opened_by,now(),opened_device_id,0,0
            FROM app.cash_shifts WHERE id=$1`, shift.ID); err == nil || !strings.Contains(err.Error(), "cash_shifts_one_active_idx") {
			t.Fatalf("PostgreSQL permitió un turno en arqueo junto a un turno abierto en la misma caja: %v", err)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='cash.shift.opened' AND scope='cash' AND aggregate_id=$1", shift.ID) != 1 {
			t.Fatal("La apertura no publicó su evento de caja")
		}
		fixture.shifts["A"] = shift.ID
	})

	t.Run("C2_dos_aperturas_simultaneas_y_fondo_cero_sin_desglose", func(t *testing.T) {
		registerID := fixture.createRegister(t, "Caja de carrera")
		first := newCashier(t, fixture.paymentsFixture, "carrera_uno", []string{"payments.read", "cash.shifts.open"}, registerID)
		second := newCashier(t, fixture.paymentsFixture, "carrera_dos", []string{"payments.read", "cash.shifts.open"}, registerID)
		type outcome struct {
			status int
			body   []byte
		}
		results := make(chan outcome, 2)
		for _, terminal := range []testTerminal{first, second} {
			go func(terminal testTerminal) {
				status, body := fixture.raw(terminal, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0"})
				results <- outcome{status, body}
			}(terminal)
		}
		left, right := <-results, <-results
		assertOneWinner(t, []int{left.status, right.status})
		loser := left
		if left.status == 200 {
			loser = right
		}
		if errorCode(t, loser.body) != "SHIFT_ALREADY_OPEN" {
			t.Fatalf("La apertura concurrente no explicó el conflicto: %s", loser.body)
		}
		var floats string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT string_agg(opening_float::text,',') FROM app.cash_shifts WHERE register_id=$1`,
			registerID).Scan(&floats); err != nil || floats != "0.00" {
			t.Fatalf("La carrera dejó turnos inesperados: %s %v", floats, err)
		}
		if fixture.countRows(t, `SELECT count(*) FROM app.cash_counts c JOIN app.cash_shifts s ON s.id=c.shift_id WHERE s.register_id=$1`, registerID) != 0 {
			t.Fatal("Un fondo de $0 sin desglose registró un conteo")
		}
	})

	t.Run("C3_el_turno_persiste_al_reiniciar_go", func(t *testing.T) {
		restarted := &administration.Service{Pool: fixture.pool, ProvisioningFile: fixture.service.ProvisioningFile}
		original := fixture.service
		fixture.service = restarted
		current := decodeResponse[struct {
			Shift  *payments.Shift `json:"shift"`
			Usable bool            `json:"usable"`
		}](t, fixture.call(t, fixture.cashier, "GET", "payments/shifts/current", nil, 200))
		fixture.service = original
		if current.Shift == nil || current.Shift.ID != fixture.shifts["A"] || !current.Usable {
			t.Fatal("El turno abierto no se recuperó tras reiniciar el servicio")
		}
	})

	t.Run("C4_turno_compartido_y_responsables_individuales", func(t *testing.T) {
		settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
		save := func(shared bool) {
			latest := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
			fixture.adminCall(t, "PUT", "payments/settings", map[string]any{
				"operation_id": randomID(), "revision": latest.Revision, "shared_shifts": shared,
				"print_receipt_default": settings.PrintReceiptDefault, "receipt_printer_id": settings.ReceiptPrinterID,
				"refund_authorization_threshold": settings.RefundAuthorizationThreshold,
			}, 200)
		}
		save(false)
		income := map[string]any{"type": "change_supply", "amount": "100"}
		problem := fixture.shiftCall(t, fixture.partner, "POST", fixture.shifts["A"], "income", income, 409)
		if errorCode(t, problem) != "SHIFT_NOT_SHARED" {
			t.Fatalf("Sin turno compartido otro cajero registró un ingreso: %s", problem)
		}
		own := fixture.operation(t, fixture.shiftCall(t, fixture.cashier, "POST", fixture.shifts["A"], "income", map[string]any{
			"type": "float_contribution", "amount": "200",
		}, 200))
		save(true)
		shared := fixture.operation(t, fixture.shiftCall(t, fixture.partner, "POST", fixture.shifts["A"], "income", income, 200))
		if own.ActorName != "Prueba cajero_caja" || shared.ActorName != "Prueba cajero_dos" || own.ShiftID != shared.ShiftID {
			t.Fatalf("El turno compartido no conservó el responsable de cada movimiento: %s / %s", own.ActorName, shared.ActorName)
		}
		fixture.operations["income"] = own.ID
		if !strings.Contains(own.Concept, "Aportación") || own.AuthorizationMode != nil || own.AuthorizedByName != nil {
			t.Fatalf("El ingreso no tomó su concepto o pidió autorización: %+v", own)
		}
	})
}
