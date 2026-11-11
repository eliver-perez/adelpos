//go:build integration

package operational_test

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
	"local/ad-el-pos/server/internal/reports"
)

// Aceptación de los reportes de la fase B2 contra PostgreSQL con la API real: caja por turno y por
// día con un turno permanente de varios días, cortes, cierres y diferencias por moneda con el tipo
// de cambio histórico, esperado protegido (comprobado sobre el JSON completo), turno en arqueo
// excluido para todos, alcance de Caja y reportes denegados; existencias por almacén, kardex,
// costos y valuación con y sin permisos, consumos, mermas clasificadas, conteos, inventario
// negativo, costos «Sin costo», «Estimado» y firme y ajustes de costo como concepto; compras por
// periodo con recepciones parciales, mostrador y gasto vinculado sin duplicar; empleados sin doble
// conteo; totales del detalle iguales a los resumidos; dashboard con sus permisos.
func TestOperationalReports(t *testing.T) {
	fx := newFixture(t)
	fx.seedCatalog(t)
	defer func() {
		shared = fx
	}()
	cash, debit := fx.methods["cash"], fx.methods["debit"]
	inv := setupInventory(t, fx)
	// Día comercial de hoy (hora de inicio 05:00 en America/Monterrey) y los dos anteriores.
	today := fx.scalar(t, "SELECT ((now() AT TIME ZONE 'America/Monterrey') - interval '05:00')::date::text")
	dayMinus1 := fx.scalar(t, "SELECT ($1::date - 1)::text", today)
	dayMinus2 := fx.scalar(t, "SELECT ($1::date - 2)::text", today)
	threeDays := "period=range&from=" + dayMinus2 + "&until=" + today
	noon := func(day string) string {
		return fx.scalar(t, "SELECT (($1::date + time '12:00') AT TIME ZONE 'America/Monterrey')::text", day)
	}

	// Turno permanente de la caja 1 con fondo distintivo, tipo de cambio y operaciones de varios días.
	shift := decode[payments.Shift](t, fx.mutate(t, "POST", "payments/shifts", map[string]any{"opening_float": "512.34"}, 200))
	fx.setFeature(t, "usd_cash", true)
	fx.mutate(t, "POST", "payments/shifts/"+shift.ID+"/usd-rate", map[string]any{"usd_rate": "18.00"}, 200)
	catalog := decode[struct {
		Categories []payments.ExpenseCategory `json:"expense_categories"`
	}](t, fx.get(t, fx.admin, "payments/cash/catalog", 200))
	expenseCategory := catalog.Categories[0].ID
	// Antes de activar el inventario: estas unidades quedan «Sin costo» (sin consumo registrado).
	first := fx.account(t, "Primer día", line(fx.burger, 1))
	firstPayment := fx.pay(t, fx.admin, first.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received": "150"})
	second := fx.account(t, "Con tarjeta", line(fx.soda, 2))
	fx.pay(t, fx.admin, second.Account.ID, map[string]any{"method_id": debit, "mode": "total"})
	income := decode[payments.CashOperation](t, fx.mutate(t, "POST", "payments/shifts/"+shift.ID+"/income",
		map[string]any{"type": "other", "amount": "40", "concept": "Aportación del encargado"}, 200))
	withdrawal := decode[payments.CashOperation](t, fx.mutate(t, "POST", "payments/shifts/"+shift.ID+"/withdrawals",
		map[string]any{"type": "safekeeping", "amount": "100", "concept": "Resguardo"}, 200))
	fx.mutate(t, "POST", "payments/shifts/"+shift.ID+"/expenses",
		map[string]any{"category_id": expenseCategory, "amount": "30", "concept": "Hielo"}, 200)
	dollars := fx.account(t, "En dólares", line(fx.burger, 1))
	fx.pay(t, fx.admin, dollars.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received_usd": "10"})
	fx.mutate(t, "POST", "payments/shifts/"+shift.ID+"/partial-cuts", map[string]any{}, 200)
	// El paso del tiempo: el turno se abrió hace dos días comerciales, el primer cobro fue ese día y el
	// ingreso y el retiro, ayer.
	fx.exec(t, `UPDATE app.cash_shifts SET opened_at=(($2::date + time '10:00') AT TIME ZONE 'America/Monterrey'),business_date=$2::date
        WHERE id=$1`, shift.ID, dayMinus2)
	paymentID := firstPayment["payment"].(map[string]any)["id"].(string)
	fx.exec(t, "UPDATE app.payments SET created_at=$2::timestamptz WHERE id=$1", paymentID, noon(dayMinus2))
	fx.exec(t, "UPDATE app.cash_movements SET created_at=$2::timestamptz WHERE payment_id=$1", paymentID, noon(dayMinus2))
	fx.exec(t, "UPDATE app.cash_operations SET created_at=$2::timestamptz WHERE id IN ($1,$3)", income.ID, noon(dayMinus1), withdrawal.ID)

	limited := fx.staff(t, "caja_sin_esperado", "", reports.PermissionRead, reports.PermissionCash, reports.PermissionPayments,
		reports.PermissionSales, reports.PermissionEmployees, "cash.shifts.read", "cash.shifts.read_all", "cash.history.read")

	t.Run("caja por turno con el esperado del cálculo único", func(t *testing.T) {
		summary := decode[payments.ShiftSummaryView](t, fx.get(t, fx.admin, "payments/shifts/"+shift.ID+"/summary", 200))
		// 512.34 + (150 − 30) + (0 − 60 de cambio del cobro en dólares) + 40 − 100 − 30 = 482.34; 10 dólares.
		if summary.Expected == nil || summary.Expected.MXN != "482.34" || summary.Expected.USD != "10.00" {
			t.Fatalf("Esperado de Caja: %+v", summary.Expected)
		}
		result := fx.report(t, fx.admin, "cash.shifts", threeDays)
		row := find(t, result, "business_date", dayMinus2)
		checks := map[string]string{
			"opening_float": "512.34", "payments": "3", "sales": "290", "net": "290", "cash_net": "240", "income_mxn": "40",
			"withdrawals_mxn": "100", "expenses_mxn": "30", "partial_cuts": "1", "expected_mxn": summary.Expected.MXN,
			"expected_usd": summary.Expected.USD, "mxn_in": "150", "mxn_out": "90", "usd_in": "10", "usd_rate": "18",
		}
		for key, expected := range checks {
			if !equal(t, row[key], expected) {
				t.Fatalf("Turno permanente: %s = %v, esperado %s: %+v", key, row[key], expected, row)
			}
		}
		if text(row["state"]) != "Abierto" || row["difference_mxn"] != nil {
			t.Fatalf("Un turno abierto no tiene diferencia: %+v", row)
		}
		// Un turno permanente aparece en la fecha de su apertura (vista por turno), no hoy.
		if len(fx.report(t, fx.admin, "cash.shifts", "").Rows) != 0 {
			t.Fatal("La vista por turno de hoy no debe incluir un turno abierto hace dos días")
		}
		// Ventas y cobros por turno: la venta reconocida al cerrar en el turno y lo cobrado en él.
		sales := find(t, fx.report(t, fx.admin, "cash.sales_collections_by_shift", threeDays), "business_date", dayMinus2)
		if !equal(t, sales["recognized_net"], "290") || !equal(t, sales["collected"], "290") || text(sales["accounts"]) != "3" {
			t.Fatalf("Ventas y cobros por turno: %+v", sales)
		}
		byShift := fx.report(t, fx.admin, "payments.by_shift", threeDays)
		if !equal(t, byShift.Totals["net"], "290") {
			t.Fatalf("El cobrado por turno de Caja coincide con el de cobros: %+v", byShift.Totals)
		}
	})

	t.Run("caja por día con un turno permanente de varios días", func(t *testing.T) {
		result := fx.report(t, fx.admin, "cash.by_day", threeDays)
		if len(result.Rows) != 3 {
			t.Fatalf("Se esperaban tres días: %+v", result.Rows)
		}
		opened := find(t, result, "day", dayMinus2)
		yesterday := find(t, result, "day", dayMinus1)
		current := find(t, result, "day", today)
		if text(opened["openings"]) != "1" || !equal(t, opened["opening_float"], "512.34") || !equal(t, opened["sales"], "120") ||
			!equal(t, opened["cash_net"], "120") || !equal(t, opened["mxn_in"], "150") {
			t.Fatalf("Día de la apertura: %+v", opened)
		}
		if !equal(t, yesterday["income_mxn"], "40") || !equal(t, yesterday["withdrawals_mxn"], "100") || text(yesterday["payments"]) != "0" {
			t.Fatalf("Día intermedio: %+v", yesterday)
		}
		if text(current["payments"]) != "2" || !equal(t, current["sales"], "170") || !equal(t, current["expenses_mxn"], "30") ||
			text(current["partial_cuts"]) != "1" || !equal(t, current["usd_in"], "10") {
			t.Fatalf("Día de hoy: %+v", current)
		}
		// Totales del detalle = resumen y = los del turno.
		if !equal(t, result.Totals["sales"], "290") || !sum(t, result, "net").Equal(amount(t, result.Totals["net"])) {
			t.Fatalf("Totales por día: %+v", result.Totals)
		}
		// Con la hora de inicio en 00:00 y el turno en otra fecha, la vista por día sigue sin depender del turno.
		paged := fx.report(t, fx.admin, "cash.by_day", threeDays+"&page_size=1&page=2")
		if len(paged.Rows) != 1 || !equal(t, paged.Totals["sales"], "290") {
			t.Fatalf("La paginación no altera los totales: %+v", paged.Totals)
		}
	})

	// Caja 2 con su cajero: un cobro y el arqueo de cierre iniciado (turno en arqueo).
	register := decode[struct {
		ID string `json:"id"`
	}](t, fx.call(t, fx.admin, "POST", "registers", map[string]any{"name": "Caja 2", "description": "", "active": true}, 200)).ID
	cashier := fx.staff(t, "cajero_dos", register, "accounts.read", "payments.read", "payments.collect", "cash.shifts.open",
		"cash.shifts.read", "cash.counts.perform", "cash.shifts.close")
	closingShift := decode[payments.Shift](t, fx.write(t, cashier, "POST", "payments/shifts", map[string]any{"opening_float": "200"}, 200))
	blindAccount := fx.account(t, "Caja dos", line(fx.soda, 1))
	fx.pay(t, cashier, blindAccount.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received": "25"})
	fx.write(t, cashier, "POST", "payments/shifts/"+closingShift.ID+"/closing", map[string]any{}, 200)
	// Supervisores de cobros que consultan en Caja solo los turnos de su caja (sin todas las cajas) y,
	// salvo el último, sin el historial de turnos cerrados.
	scopePermissions := []string{reports.PermissionRead, reports.PermissionPayments, reports.PermissionEmployees,
		reports.PermissionCashExpected, reports.PermissionExportExcel, "cash.shifts.read"}
	supervisorOne := fx.staff(t, "supervisor_caja_uno", fx.registerID, scopePermissions...)
	supervisorTwo := fx.staff(t, "supervisor_caja_dos", register, scopePermissions...)
	historian := fx.staff(t, "supervisor_historial", register, append(append([]string{}, scopePermissions...), "cash.history.read")...)

	t.Run("turno en arqueo excluido de todo agregado para todos", func(t *testing.T) {
		shifts := fx.report(t, fx.admin, "cash.shifts", threeDays)
		blindRow := find(t, shifts, "register", "Caja 2")
		for key, value := range blindRow {
			if value != nil && !map[string]bool{"register": true, "business_date": true, "state": true, "opened_at": true,
				"opened_by": true, "closed_by": true}[key] {
				t.Fatalf("El turno en arqueo muestra %s=%v", key, value)
			}
		}
		if !shifts.warning("SHIFT_CLOSING") || strings.Contains(string(shifts.raw), `"200.00"`) {
			t.Fatalf("Aviso y fondo del turno en arqueo: %s", shifts.raw)
		}
		byRegister := find(t, fx.report(t, fx.admin, "cash.by_register", threeDays), "register", "Caja 2")
		if text(byRegister["closing_shifts"]) != "1" || text(byRegister["payments"]) != "0" || !equal(t, byRegister["sales"], "0") {
			t.Fatalf("Historial por caja con el turno en arqueo: %+v", byRegister)
		}
		day := find(t, fx.report(t, fx.admin, "cash.by_day", threeDays), "day", today)
		if text(day["payments"]) != "2" || !equal(t, day["sales"], "170") {
			t.Fatalf("La vista por día incluyó el turno en arqueo: %+v", day)
		}
		methods := fx.report(t, fx.admin, "payments.by_method", "")
		if !equal(t, methods.Totals["net"], "170") || !methods.warning("SHIFT_CLOSING") {
			t.Fatalf("Cobros sin el turno en arqueo: %+v", methods.Totals)
		}
		// Ventas por turno: las cifras resumidas excluyen el turno en arqueo igual que los renglones.
		salesByShift := fx.report(t, fx.admin, "sales.by_shift", "")
		if !salesByShift.warning("SHIFT_CLOSING") || !equal(t, salesByShift.figure("net"), text(salesByShift.Totals["net"])) ||
			text(salesByShift.figure("quantity")) != text(salesByShift.Totals["quantity"]) ||
			text(salesByShift.figure("accounts")) != text(salesByShift.Totals["accounts"]) {
			t.Fatalf("Cifras de ventas por turno con el turno en arqueo: %+v %+v", salesByShift.Summary, salesByShift.Totals)
		}
		// El aviso de turnos en arqueo de los cobros solo cuenta los del alcance de Caja del usuario.
		if scoped := fx.report(t, supervisorOne, "payments.by_method", ""); scoped.warning("SHIFT_CLOSING") || !scoped.warning("CASH_SCOPE") ||
			!equal(t, scoped.Totals["net"], "170") {
			t.Fatalf("Alcance de la caja 1 con el turno en arqueo de otra caja: %s", scoped.raw)
		}
		if own := fx.report(t, supervisorTwo, "payments.by_method", ""); !own.warning("SHIFT_CLOSING") || len(own.Rows) != 0 {
			t.Fatalf("Alcance de la caja 2 con su turno en arqueo: %s", own.raw)
		}
		dashboard := fx.get(t, fx.admin, "reports/dashboard", 200)
		if !strings.Contains(string(dashboard), `"closing_shifts","label":"Turnos en arqueo","type":"integer","value":1`) {
			t.Fatalf("El dashboard debe indicar el turno en arqueo sin importes: %s", dashboard)
		}
		activity := fx.report(t, fx.admin, "employees.activity", "")
		for _, row := range activity.Rows {
			if text(row["user"]) == "Prueba cajero_dos" && (text(row["payments"]) != "0" || !equal(t, row["collected"], "0")) {
				t.Fatalf("El cobro de un turno en arqueo no cuenta para su cajero: %+v", row)
			}
		}
	})

	// Cierre de la caja 2: contado 230 contra un esperado de 225 (sobrante de $5).
	closing := decode[payments.ClosingView](t, fx.get(t, cashier, "payments/shifts/"+closingShift.ID+"/closing", 200))
	saved := decode[payments.ClosingView](t, fx.write(t, cashier, "PUT", "payments/shifts/"+closingShift.ID+"/closing/count", map[string]any{
		"revision": closing.Count.Revision, "notes": "", "lines": []any{}, "extra_mxn": "230.00", "extra_note": "Conteo sin desglosar",
	}, 200))
	fx.write(t, cashier, "POST", "payments/shifts/"+closingShift.ID+"/closing/confirm", map[string]any{"count_revision": saved.Count.Revision}, 200)

	t.Run("cierre definitivo con sobrante y cortes", func(t *testing.T) {
		row := find(t, fx.report(t, fx.admin, "cash.shifts", threeDays), "register", "Caja 2")
		if text(row["state"]) != "Cerrado" || !equal(t, row["counted_mxn"], "230") || !equal(t, row["expected_mxn"], "225") ||
			!equal(t, row["difference_mxn"], "5") || text(row["result"]) != "Sobrante en pesos" {
			t.Fatalf("Turno cerrado con sobrante: %+v", row)
		}
		byRegister := find(t, fx.report(t, fx.admin, "cash.by_register", threeDays), "register", "Caja 2")
		if !equal(t, byRegister["over_mxn"], "5") || !equal(t, byRegister["short_mxn"], "0") || text(byRegister["closed_shifts"]) != "1" ||
			!equal(t, byRegister["sales"], "25") {
			t.Fatalf("Sobrantes por caja: %+v", byRegister)
		}
		day := find(t, fx.report(t, fx.admin, "cash.by_day", threeDays), "day", today)
		if text(day["closings"]) != "1" || !equal(t, day["counted_mxn"], "230") || !equal(t, day["closing_expected_mxn"], "225") ||
			!equal(t, day["over_mxn"], "5") || !equal(t, day["sales"], "195") {
			t.Fatalf("Cierres del día: %+v", day)
		}
		cuts := fx.report(t, fx.admin, "cash.cuts", "")
		final := find(t, cuts, "cut", "Corte definitivo")
		partial := find(t, cuts, "cut", "Corte parcial 1")
		if !equal(t, final["expected_mxn"], "225") || !equal(t, final["difference_mxn"], "5") || !equal(t, final["sales"], "25") ||
			!equal(t, partial["expected_mxn"], "482.34") {
			t.Fatalf("Cortes: %+v %+v", final, partial)
		}
	})

	t.Run("cobros con el alcance de Caja aunque se llame a la API", func(t *testing.T) {
		if all := fx.report(t, fx.admin, "payments.by_method", ""); !equal(t, all.Totals["net"], "195") || all.warning("CASH_SCOPE") {
			t.Fatalf("El administrador ve los cobros de ambas cajas: %s", all.raw)
		}
		// Supervisor de la caja 1 sin consultar todas las cajas: no ve la caja 2 ni su turno cerrado.
		one := fx.report(t, supervisorOne, "payments.by_method", "")
		if !equal(t, one.Totals["net"], "170") || !one.warning("CASH_SCOPE") {
			t.Fatalf("Cobros de la caja 1: %s", one.raw)
		}
		for _, id := range []string{"payments.by_method", "payments.list", "payments.by_shift", "payments.by_register", "payments.tips",
			"payments.currency"} {
			for _, query := range []string{"shift_id=" + closingShift.ID, threeDays + "&shift_id=" + closingShift.ID, threeDays} {
				result := fx.report(t, supervisorOne, id, query)
				if strings.Contains(string(result.raw), "Caja 2") || strings.Contains(string(result.raw), "cajero_dos") {
					t.Fatalf("%s (%s) reveló la caja 2: %s", id, query, result.raw)
				}
				if strings.Contains(query, "shift_id") && len(result.Rows) != 0 {
					t.Fatalf("%s con el turno cerrado de otra caja: %s", id, result.raw)
				}
			}
		}
		// Ni en las exportaciones ni en las opciones de filtros ni en el resumen ejecutivo.
		for _, query := range []string{"shift_id=" + closingShift.ID, threeDays} {
			workbook := fx.get(t, supervisorOne, "reports/export/payments.list?format=xlsx&"+query, 200)
			if content := workbookText(t, workbook); strings.Contains(content, "Caja 2") || strings.Contains(content, "cajero_dos") {
				t.Fatalf("La exportación (%s) reveló la caja 2", query)
			}
		}
		shifts := string(fx.get(t, supervisorOne, "reports/filters/shifts", 200))
		if strings.Contains(shifts, closingShift.ID) || !strings.Contains(shifts, shift.ID) {
			t.Fatalf("Opciones de turnos de la caja 1: %s", shifts)
		}
		dashboard := decode[reports.Dashboard](t, fx.get(t, supervisorOne, "reports/dashboard", 200))
		collected, warned := "", false
		for _, section := range dashboard.Sections {
			for _, indicator := range section.Indicators {
				if section.ID == "payments" && indicator.Key == "collected" {
					collected = text(indicator.Value)
				}
			}
		}
		for _, warning := range dashboard.Warnings {
			warned = warned || warning.Code == "CASH_SCOPE"
		}
		if collected != "170.00" || !warned {
			t.Fatalf("Cobros del resumen ejecutivo de la caja 1: %q %+v", collected, dashboard.Warnings)
		}
		// Supervisor de la caja 2 sin historial: su turno cerrado (por otro) no aparece; con historial, sí.
		if own := fx.report(t, supervisorTwo, "payments.list", threeDays); len(own.Rows) != 0 || !own.warning("CASH_SCOPE") {
			t.Fatalf("Sin historial no se ven los cobros de un turno cerrado: %s", own.raw)
		}
		if strings.Contains(string(fx.get(t, supervisorTwo, "reports/filters/shifts", 200)), closingShift.ID) {
			t.Fatal("Sin historial el turno cerrado no aparece en las opciones de turnos")
		}
		// Actividad por empleado: los cobros y la caja con el mismo alcance; la venta no se limita.
		adminActivity := find(t, fx.report(t, fx.admin, "employees.activity", threeDays), "user", "Administrador")
		cashierAll := find(t, fx.report(t, fx.admin, "employees.activity", threeDays), "user", "Prueba cajero_dos")
		if !equal(t, cashierAll["collected"], "25") || text(cashierAll["payments"]) != "1" {
			t.Fatalf("El administrador ve los cobros del cajero de la caja 2: %+v", cashierAll)
		}
		for _, member := range []terminal{supervisorOne, supervisorTwo} {
			activity := fx.report(t, member, "employees.activity", threeDays)
			if strings.Contains(string(activity.raw), "cajero_dos") || !activity.warning("CASH_SCOPE") {
				t.Fatalf("Sin todas las cajas o sin historial no se ven los cobros del turno cerrado de la caja 2: %s", activity.raw)
			}
			if member == supervisorOne {
				row := find(t, activity, "user", "Administrador")
				for _, key := range []string{"payments", "collected", "cash_operations", "accounts", "sales", "confirmations"} {
					if text(row[key]) != text(adminActivity[key]) {
						t.Fatalf("Actividad del administrador en la caja 1 (%s): %+v frente a %+v", key, row, adminActivity)
					}
				}
			}
		}
		if row := find(t, fx.report(t, historian, "employees.activity", threeDays), "user", "Prueba cajero_dos"); !equal(t, row["collected"], "25") {
			t.Fatalf("Con historial se ven los cobros del turno cerrado de su caja: %+v", row)
		}
		closed := fx.report(t, historian, "payments.by_method", "shift_id="+closingShift.ID)
		if !equal(t, closed.Totals["net"], "25") || !strings.Contains(string(fx.get(t, historian, "reports/filters/shifts", 200)), closingShift.ID) {
			t.Fatalf("Con historial se ve el turno cerrado de su caja: %s", closed.raw)
		}
	})

	t.Run("pesos físicos de un cobro anterior a la 6.1 con la regla de Caja", func(t *testing.T) {
		// Un cobro en efectivo anterior a la 0007 no tiene columnas por moneda: entró lo recibido (150)
		// y salió el cambio (30).
		fx.exec(t, `UPDATE app.cash_movements SET mxn_in=NULL,mxn_out=NULL,usd_in=NULL,usd_out=NULL,usd_rate=NULL WHERE payment_id=$1`,
			paymentID)
		summary := decode[payments.ShiftSummaryView](t, fx.get(t, fx.admin, "payments/shifts/"+shift.ID+"/summary", 200))
		if summary.Physical == nil {
			t.Fatal("El resumen de Caja debía traer el efectivo físico")
		}
		currency := fx.report(t, fx.admin, "payments.currency", threeDays+"&group_by=shift")
		var row map[string]any
		for _, candidate := range currency.Rows {
			if strings.HasPrefix(text(candidate["group_label"]), "Caja 1") {
				row = candidate
			}
		}
		if row == nil || !equal(t, row["mxn_in"], summary.Physical.MXNIn) || !equal(t, row["mxn_out"], summary.Physical.MXNOut) ||
			!equal(t, row["usd_in"], summary.Physical.USDIn) {
			t.Fatalf("Efectivo por moneda frente a Caja %+v: %+v", summary.Physical, currency.Rows)
		}
		cashRow := find(t, fx.report(t, fx.admin, "cash.shifts", threeDays), "register", "Caja 1")
		if !equal(t, row["mxn_in"], text(cashRow["mxn_in"])) || !equal(t, row["mxn_out"], text(cashRow["mxn_out"])) {
			t.Fatalf("Efectivo por moneda frente a la caja por turno: %+v %+v", row, cashRow)
		}
	})

	t.Run("esperado y diferencias protegidos en todo el JSON", func(t *testing.T) {
		forbidden := []string{`"expected_mxn"`, `"expected_usd"`, `"difference_mxn"`, `"difference_usd"`, `"difference_equivalent_mxn"`,
			`"mxn_in"`, `"mxn_out"`, `"usd_in"`, `"usd_out"`, `"over_mxn"`, `"short_mxn"`, `"result"`, `"closing_expected_mxn"`,
			`"cash_flow_mxn"`, `"flow_mxn"`, `"482.34"`, `"225.00"`, "cash_differences"}
		bodies := map[string][]byte{"catálogo": fx.get(t, limited, "reports/catalog", 200), "dashboard": fx.get(t, limited, "reports/dashboard", 200)}
		for _, definition := range reports.Definitions() {
			if definition.Group != "cash" {
				continue
			}
			for _, query := range []string{"", threeDays} {
				bodies[definition.ID+" "+query] = fx.get(t, limited, "reports/run/"+definition.ID+"?page_size=200&"+query, 200)
			}
		}
		bodies["cobros por turno"] = fx.get(t, limited, "reports/run/payments.by_shift?"+threeDays, 200)
		for name, body := range bodies {
			for _, fragment := range forbidden {
				if strings.Contains(string(body), fragment) {
					t.Fatalf("%s reveló %s sin cash.expected.read: %s", name, fragment, body)
				}
			}
		}
		// Lo que Caja sí muestra sin el permiso: fondo, cobros por método y lo contado.
		row := find(t, fx.report(t, limited, "cash.shifts", threeDays), "register", "Caja 2")
		if !equal(t, row["counted_mxn"], "230") || !equal(t, row["opening_float"], "200") {
			t.Fatalf("Paridad con Caja sin el permiso: %+v", row)
		}
		// Con permiso, el administrador sí recibe el esperado.
		if !strings.Contains(string(fx.get(t, fx.admin, "reports/run/cash.shifts?"+threeDays, 200)), `"482.34"`) {
			t.Fatal("El administrador debía recibir el esperado")
		}
	})

	t.Run("alcance de Caja y reportes denegados aunque se llame a la API", func(t *testing.T) {
		scoped := fx.staff(t, "caja_propia", "", reports.PermissionRead, reports.PermissionCash, "cash.shifts.read")
		result := fx.report(t, scoped, "cash.shifts", threeDays)
		if len(result.Rows) != 0 || !result.warning("CASH_SCOPE") {
			t.Fatalf("Sin consultar todas las cajas ni el historial no se ven turnos de otras cajas: %+v", result)
		}
		denied := fx.staff(t, "sin_caja", "", reports.PermissionRead, reports.PermissionSales)
		for _, id := range []string{"cash.shifts", "cash.by_day", "cash.operations", "cash.cuts", "purchases.received", "inventory.stock",
			"inventory.kardex", "costs.margin_by_product", "employees.activity", "suppliers.activity"} {
			fx.get(t, denied, "reports/run/"+id, 403)
		}
		dashboard := string(fx.get(t, denied, "reports/dashboard", 200))
		for _, section := range []string{`"id":"cash"`, `"id":"purchases"`, `"id":"inventory"`, `"id":"margin"`} {
			if strings.Contains(dashboard, section) {
				t.Fatalf("El dashboard incluyó %s sin su permiso", section)
			}
		}
	})

	// Inventario activado: consumos reales (firmes, estimados por existencia negativa y sin receta).
	fx.setFeature(t, "inventory_control", true)
	stocked := fx.account(t, "Con inventario", line(fx.burger, 2), line(fx.soda, 3), line(fx.salad, 1))
	fx.process(t)
	fx.pay(t, fx.admin, stocked.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received": "365"})
	pending := fx.account(t, "Consumo pendiente", line(fx.burger, 1))
	fx.pay(t, fx.admin, pending.Account.ID, map[string]any{"method_id": debit, "mode": "total"})

	t.Run("márgenes con «Sin costo», «Estimado» y firme", func(t *testing.T) {
		if fx.scalar(t, "SELECT count(*)::text FROM app.inventory_consumption_requests WHERE state='pending'") != "1" {
			t.Fatal("La prueba necesita una solicitud de consumo pendiente")
		}
		result := fx.report(t, fx.admin, "costs.margin_by_product", "")
		if !result.warning("CONSUMPTION_PENDING") {
			t.Fatal("Con consumo pendiente el margen debe avisar que los datos pueden estar incompletos")
		}
		burger := find(t, result, "product", "Hamburguesa")
		soda := find(t, result, "product", "Refresco")
		salad := find(t, result, "product", "Ensalada")
		// Hamburguesas: 2 antes de activar el inventario y 1 pendiente (sin costo) y 2 firmes a $33.
		if text(burger["units"]) != "5" || text(burger["firm_units"]) != "2" || text(burger["uncosted_units"]) != "3" ||
			!equal(t, burger["cost"], "66") || !strings.HasPrefix(text(burger["status"]), "Firme parcial") {
			t.Fatalf("Margen de la hamburguesa: %+v", burger)
		}
		// Refrescos: 2 antes de activar y 1 de la caja 2 (sin costo), 2 firmes a $12 y 1 estimado (existencia
		// en cero: último costo válido).
		if text(soda["firm_units"]) != "2" || text(soda["estimated_units"]) != "1" || text(soda["uncosted_units"]) != "3" ||
			!equal(t, soda["cost"], "36") || !strings.HasPrefix(text(soda["status"]), "Estimado") {
			t.Fatalf("Margen del refresco: %+v", soda)
		}
		if text(salad["status"]) != "Sin costo" || !equal(t, salad["cost"], "0") || salad["margin_percent"] != nil {
			t.Fatalf("Producto sin receta: %+v", salad)
		}
		// Margen = venta sin IVA de las unidades con costo − su costo; % con costo firme sobre la venta sin IVA.
		costedBase := decimal.RequireFromString(fx.scalar(t, `SELECT sum(base)::text FROM app.sales_recognitions WHERE account_id=$1
            AND product_id=$2`, stocked.Account.ID, fx.burger.ID))
		if !amount(t, burger["margin"]).Equal(costedBase.Sub(decimal.NewFromInt(66))) || !amount(t, burger["costed_base"]).Equal(costedBase) {
			t.Fatalf("Margen estimado de la hamburguesa: %+v (base con costo %s)", burger, costedBase)
		}
		if !amount(t, result.Totals["cost"]).Equal(sum(t, result, "cost")) || !amount(t, result.Totals["base"]).Equal(sum(t, result, "base")) {
			t.Fatalf("Totales del margen: %+v", result.Totals)
		}
		status := fx.report(t, fx.admin, "costs.cost_status", "state=uncosted")
		reasons := map[string]bool{}
		for _, row := range status.Rows {
			reasons[text(row["reason"])] = true
		}
		if len(reasons) != 3 {
			t.Fatalf("Qué falta para el costo (sin consumo, pendiente y sin receta): %+v", status.Rows)
		}
		fx.process(t)
		after := fx.report(t, fx.admin, "costs.margin_by_product", "")
		if after.warning("CONSUMPTION_PENDING") || text(find(t, after, "product", "Hamburguesa")["firm_units"]) != "3" {
			t.Fatalf("Procesado el consumo, la unidad pasa a costo firme: %+v", after.Rows)
		}
		period := fx.report(t, fx.admin, "costs.margin_by_period", "")
		if !amount(t, period.Totals["margin"]).Equal(amount(t, after.Totals["margin"])) {
			t.Fatalf("El margen por periodo coincide con el margen por producto: %+v %+v", period.Totals, after.Totals)
		}
	})

	t.Run("costo teórico por el motor de recetas", func(t *testing.T) {
		products := fx.report(t, fx.admin, "costs.theoretical_products", "")
		burger := find(t, products, "product", "Hamburguesa")
		recipe := decode[inventory.Recipe](t, fx.get(t, fx.admin, "inventory/recipes/"+inv.burgerRecipe, 200))
		if recipe.Cost.UnitCost == nil || !equal(t, burger["unit_cost"], *recipe.Cost.UnitCost) || !equal(t, burger["unit_cost"], "33") ||
			!equal(t, burger["price_base"], "103.45") {
			t.Fatalf("Costo teórico de la hamburguesa: %+v (motor %v)", burger, recipe.Cost.UnitCost)
		}
		recipes := fx.report(t, fx.admin, "costs.theoretical_recipes", "kind=product")
		if len(recipes.Rows) != 2 {
			t.Fatalf("Recetas de producto: %+v", recipes.Rows)
		}
	})

	// Merma operativa, cancelación «Preparado / merma», transferencia y conteo con diferencia.
	fx.mutate(t, "POST", "inventory/adjustments", map[string]any{"kind": "waste", "reason": "breakage", "warehouse_id": inv.general,
		"lines": []map[string]any{{"item_id": inv.items["PAN"], "quantity": "-1"}}}, 200)
	cancelled := fx.account(t, "Cancelada preparada", line(fx.burger, 1))
	fx.process(t)
	current := decode[operations.Bill](t, fx.get(t, fx.admin, "restaurant/accounts/"+cancelled.Account.ID, 200))
	unitID := fx.scalar(t, "SELECT id::text FROM app.restaurant_item_units WHERE account_id=$1", cancelled.Account.ID)
	fx.mutate(t, "POST", "restaurant/accounts/"+cancelled.Account.ID+"/cancel", map[string]any{
		"revision": current.Account.Revision, "version_id": current.Lines[0].VersionID, "quantity": 1, "reason": "Se quemó",
	}, 200)
	fx.process(t)
	trace := decode[inventory.UnitConsumptionTrace](t, fx.get(t, fx.admin, "inventory/consumption/"+unitID, 200))
	fx.mutate(t, "POST", "inventory/cancellations/"+trace.Cancellation.ID+"/classify",
		map[string]any{"revision": trace.Cancellation.Revision, "state": "prepared_waste"}, 200)
	fx.mutate(t, "POST", "inventory/transfers", map[string]any{"from_warehouse_id": inv.general, "to_warehouse_id": inv.barra,
		"lines": []map[string]any{{"item_id": inv.items["CARNE"], "quantity": "100"}}}, 200)
	count := decode[inventory.Count](t, fx.mutate(t, "POST", "inventory/counts", map[string]any{"warehouse_id": inv.barra, "scope": "items",
		"item_ids": []string{inv.items["CARNE"]}}, 200))
	route := "inventory/counts/" + count.ID
	captured := decode[inventory.Count](t, fx.mutate(t, "PUT", route+"/lines", map[string]any{"lines": []map[string]any{
		{"item_id": inv.items["CARNE"], "counted_quantity": "90"}}}, 200))
	submitted := decode[inventory.Count](t, fx.mutate(t, "POST", route+"/submit", map[string]any{"revision": captured.Revision}, 200))
	approved := decode[inventory.Count](t, fx.mutate(t, "POST", route+"/approve", map[string]any{"revision": submitted.Revision}, 200))
	fx.mutate(t, "POST", route+"/apply", map[string]any{"revision": approved.Revision}, 200)

	t.Run("existencias, kardex, mermas, conteos y consumos con sus permisos", func(t *testing.T) {
		stock := fx.report(t, fx.admin, "inventory.stock", "")
		lata := find(t, stock, "code", "LATA")
		pan := find(t, stock, "code", "PAN")
		if !equal(t, lata["quantity"], "-1") || text(lata["status"]) != "Negativa" || text(pan["status"]) != "Bajo mínimo" ||
			!equal(t, pan["average_cost"], "3") || !equal(t, pan["quantity"], "5") {
			t.Fatalf("Existencias: %+v %+v", lata, pan)
		}
		negative := fx.report(t, fx.admin, "inventory.negative", "")
		if len(negative.Rows) != 1 || text(negative.Rows[0]["code"]) != "LATA" {
			t.Fatalf("Insumos negativos: %+v", negative.Rows)
		}
		if find(t, fx.report(t, fx.admin, "inventory.below_minimum", ""), "code", "PAN")["missing"] == nil {
			t.Fatal("PAN está bajo su mínimo")
		}
		barra := fx.report(t, fx.admin, "inventory.stock_by_warehouse", "warehouse_id="+inv.barra)
		if len(barra.Rows) != 1 || !equal(t, barra.Rows[0]["quantity"], "90") {
			t.Fatalf("Existencias de la barra tras transferir 100 y contar 90: %+v", barra.Rows)
		}
		kardex := fx.report(t, fx.admin, "inventory.kardex", "item_id="+inv.items["PAN"])
		opening, entries, outflows, closingQuantity := kardex.figure("opening"), kardex.figure("entries"), kardex.figure("outflows"),
			kardex.figure("closing")
		if !equal(t, opening, "0") || !equal(t, entries, "10") || !equal(t, outflows, "5") || !equal(t, closingQuantity, "5") ||
			!sum(t, kardex, "quantity").Equal(decimal.NewFromInt(5)) {
			t.Fatalf("Kardex del pan: %v %v %v %v %+v", opening, entries, outflows, closingQuantity, kardex.Rows)
		}
		waste := fx.report(t, fx.admin, "inventory.waste", "")
		operational := find(t, waste, "origin", "Merma operativa")
		if text(operational["classification"]) != "Rotura" || !equal(t, operational["value"], "3") || !equal(t, waste.Totals["value"], "36") {
			t.Fatalf("Mermas clasificadas (rotura $3 y cancelación preparada $33): %+v", waste.Rows)
		}
		for _, row := range waste.Rows {
			if text(row["origin"]) == "Cancelación de partida" && text(row["classification"]) != "Preparado / merma" {
				t.Fatalf("Clasificación de la cancelación: %+v", row)
			}
		}
		if !equal(t, find(t, fx.report(t, fx.admin, "costs.waste", ""), "classification", "Preparado / merma")["value"], "33") {
			t.Fatal("Costo de la merma por cancelación")
		}
		differences := fx.report(t, fx.admin, "inventory.count_differences", "")
		if len(differences.Rows) != 1 || !equal(t, differences.Rows[0]["difference"], "-10") || !equal(t, differences.Rows[0]["theoretical"], "100") {
			t.Fatalf("Diferencias de conteo: %+v", differences.Rows)
		}
		counts := fx.report(t, fx.admin, "inventory.counts", "")
		if len(counts.Rows) != 1 || text(counts.Rows[0]["difference_lines"]) != "1" || text(counts.Rows[0]["state"]) != "Aplicado" {
			t.Fatalf("Conteos: %+v", counts.Rows)
		}
		if len(fx.report(t, fx.admin, "inventory.transfers", "").Rows) != 1 {
			t.Fatal("Transferencias")
		}
		consumption := fx.report(t, fx.admin, "inventory.consumption_by_item", "")
		// Pan: 2 + 1 + 1 hamburguesas con consumo (la cancelada «Preparado / merma» conserva su consumo); 7
		// unidades con consumo: 4 hamburguesas y 3 refrescos (la ensalada no tiene receta).
		if !equal(t, find(t, consumption, "code", "PAN")["quantity"], "4") || !equal(t, consumption.figure("units"), "7") {
			t.Fatalf("Consumo por ingrediente: %+v %v", consumption.Rows, consumption.figure("units"))
		}
		kitchen := fx.report(t, fx.admin, "inventory.consumption_by_kitchen", "")
		if !equal(t, kitchen.figure("units"), "7") || !equal(t, kitchen.Totals["units"], "7") {
			t.Fatalf("Consumo asociado a cocina sin contar una unidad dos veces: %+v", kitchen.Rows)
		}
		byProduct := fx.report(t, fx.admin, "inventory.consumption_by_product", "")
		if !amount(t, byProduct.Totals["value"]).Equal(amount(t, consumption.Totals["value"])) {
			t.Fatalf("Consumo por producto e ingrediente: %+v %+v", byProduct.Totals, consumption.Totals)
		}
		// Con cantidades y sin costos: ni columnas ni importes de costo en el JSON.
		quantities := fx.staff(t, "inventario_cantidades", "", reports.PermissionRead, reports.PermissionInventory, "inventory.stock.read",
			"inventory.movements.read")
		for _, id := range []string{"inventory.stock", "inventory.kardex", "inventory.waste", "inventory.consumption_by_item",
			"inventory.count_differences", "inventory.transfers"} {
			result := fx.report(t, quantities, id, "")
			for _, key := range []string{"average_cost", "value", "unit_cost", "cost_basis", "valuation_after", "cost_pending", "item_value"} {
				_, inTotals := result.Totals[key]
				if result.has(key) || inTotals {
					t.Fatalf("%s reveló %s sin inventory.costs.read: %s", id, key, result.raw)
				}
				for _, row := range result.Rows {
					if _, found := row[key]; found {
						t.Fatalf("%s reveló %s en un renglón sin inventory.costs.read", id, key)
					}
				}
				if key != "value" && strings.Contains(string(result.raw), `"`+key+`"`) {
					t.Fatalf("%s reveló %s sin inventory.costs.read: %s", id, key, result.raw)
				}
			}
			for _, row := range result.Rows {
				if text(row["kind"]) == "Ajuste de costo" {
					t.Fatalf("%s listó un ajuste de costo sin permiso de costos", id)
				}
			}
		}
		fx.get(t, quantities, "reports/run/inventory.valuation", 403)
		fx.get(t, quantities, "reports/run/inventory.average_costs", 403)
		// Valuación sin costos unitarios: solo el valor total del inventario.
		valuation := fx.staff(t, "inventario_valuacion", "", reports.PermissionRead, reports.PermissionInventory, "inventory.valuation.read")
		valued := fx.report(t, valuation, "inventory.valuation", "")
		total := fx.scalar(t, "SELECT round(sum(value),2)::text FROM app.inventory_valuations")
		if valued.has("value") || valued.has("quantity") || !equal(t, valued.figure("total_value"), total) {
			t.Fatalf("Valuación sin costos ni existencias: %s", valued.raw)
		}
		full := fx.report(t, fx.admin, "inventory.valuation", "")
		if !amount(t, full.Totals["value"]).Round(2).Equal(decimal.RequireFromString(total)) {
			t.Fatalf("Valuación por insumo = valor total: %+v %s", full.Totals, total)
		}
	})

	// Compras: orden con recepción parcial, recepción directa, compra de mostrador pagada desde caja y
	// devolución a proveedor.
	supplier := decode[map[string]any](t, fx.mutate(t, "POST", "inventory/suppliers", map[string]any{"trade_name": "Distribuidora Norte"}, 200))
	supplierID := supplier["id"].(string)
	counterID := fx.scalar(t, "SELECT id::text FROM app.inventory_suppliers WHERE kind='counter'")
	purchaseLine := func(item, quantity, price string, rate int) map[string]any {
		return map[string]any{"item_id": inv.items[item], "quantity": quantity, "unit_price": price, "discount": "0", "tax_rate": rate,
			"unit_id": inv.units["pza"]}
	}
	draft := decode[map[string]any](t, fx.mutate(t, "POST", "inventory/purchase-orders", map[string]any{"supplier_id": supplierID,
		"lines": []map[string]any{purchaseLine("LATA", "24", "12.50", 16)}}, 200))
	order := decode[map[string]any](t, fx.mutate(t, "POST", "inventory/purchase-orders/"+draft["id"].(string)+"/issue",
		map[string]any{"revision": draft["revision"]}, 200))
	orderReceipt := decode[map[string]any](t, fx.mutate(t, "POST", "inventory/receipts", map[string]any{"purchase_order_id": order["id"],
		"lines": []map[string]any{{"order_line_no": 1, "quantity": "12"}}}, 200))
	fx.mutate(t, "POST", "inventory/receipts", map[string]any{"supplier_id": supplierID,
		"lines": []map[string]any{purchaseLine("PAN", "20", "2.50", 0)}}, 200)
	counterReceipt := decode[map[string]any](t, fx.mutate(t, "POST", "inventory/receipts", map[string]any{"supplier_id": counterID,
		"counter_store": "Mercado Juárez", "lines": []map[string]any{purchaseLine("LATA", "6", "14", 16)}}, 200))
	expense := decode[payments.CashOperation](t, fx.mutate(t, "POST", "payments/shifts/"+shift.ID+"/expenses",
		map[string]any{"category_id": expenseCategory, "amount": "97.44", "concept": "Refrescos del mercado"}, 200))
	fx.mutate(t, "POST", "inventory/receipts/"+counterReceipt["id"].(string)+"/cash-links",
		map[string]any{"cash_operation_id": expense.ID, "amount": "97.44"}, 200)
	fx.mutate(t, "POST", "inventory/supplier-returns", map[string]any{"receipt_id": orderReceipt["id"], "reason": "Latas abolladas",
		"lines": []map[string]any{{"receipt_line_no": 1, "quantity": "2"}}}, 200)

	t.Run("compras por periodo sin duplicar el gasto vinculado", func(t *testing.T) {
		received := fx.report(t, fx.admin, "purchases.received", "")
		// 12 × 12.50 + 16 % = 174.00; 20 × 2.50 = 50.00; mostrador 6 × 14 + 16 % = 97.44.
		if len(received.Rows) != 3 || !equal(t, received.Totals["total"], "321.44") || !equal(t, received.figure("total"), "321.44") ||
			!equal(t, received.figure("cash_linked"), "97.44") || !equal(t, received.figure("returned_value"), "29") {
			t.Fatalf("Compras recibidas: %+v %+v", received.Totals, received.Summary)
		}
		// Las devoluciones y lo pagado desde caja de las cifras respetan los filtros: la devolución es de
		// latas de la orden de Distribuidora Norte y el gasto vinculado, de la compra de mostrador.
		for query, expected := range map[string][2]string{
			"modality=counter":                            {"0", "97.44"},
			"modality=order":                              {"29", "0"},
			"supplier_id=" + supplierID:                   {"29", "0"},
			"state=partially_received":                    {"29", "0"},
			"state=received":                              {"0", "0"},
			"item_id=" + inv.items["PAN"]:                 {"0", "0"},
			"item_id=" + inv.items["LATA"]:                {"29", "97.44"},
			"warehouse_id=" + inv.barra:                   {"0", "0"},
			"warehouse_id=" + inv.general:                 {"29", "97.44"},
			"supplier_id=" + counterID:                    {"0", "97.44"},
			"modality=direct&item_id=" + inv.items["PAN"]: {"0", "0"},
		} {
			filtered := fx.report(t, fx.admin, "purchases.received", query)
			if !equal(t, filtered.figure("returned_value"), expected[0]) || !equal(t, filtered.figure("cash_linked"), expected[1]) {
				t.Fatalf("Cifras de compras con %s: %+v", query, filtered.Summary)
			}
			// Un vínculo no se reparte por insumo: con filtro de insumo se rotula como de las recepciones completas.
			complete := strings.Contains(string(filtered.raw), "Pagado desde caja (recepciones completas)")
			if complete != strings.Contains(query, "item_id") {
				t.Fatalf("Rótulo de lo pagado desde caja con %s: %s", query, filtered.raw)
			}
		}
		if total := fx.scalar(t, "SELECT sum(total)::text FROM app.inventory_receipts"); !equal(t, received.Totals["total"], total) {
			t.Fatalf("Cada recepción se suma una sola vez: %s", total)
		}
		counter := fx.report(t, fx.admin, "purchases.received", "modality=counter")
		if len(counter.Rows) != 1 || text(counter.Rows[0]["supplier"]) != "Compras de mostrador" ||
			text(counter.Rows[0]["counter_store"]) != "Mercado Juárez" || !equal(t, counter.Totals["total"], "97.44") {
			t.Fatalf("Compras de mostrador: %+v", counter.Rows)
		}
		if direct := fx.report(t, fx.admin, "purchases.received", "modality=direct"); len(direct.Rows) != 1 || text(direct.Rows[0]["code"]) != "PAN" {
			t.Fatalf("Recepción directa: %+v", direct.Rows)
		}
		ordered := fx.report(t, fx.admin, "purchases.ordered", "")
		if len(ordered.Rows) != 1 || !equal(t, ordered.Rows[0]["received_quantity"], "12") || !equal(t, ordered.Rows[0]["pending_quantity"], "12") ||
			!equal(t, ordered.Rows[0]["total"], "348") || text(ordered.Rows[0]["state"]) != "Parcialmente recibida" {
			t.Fatalf("Ordenado contra recibido: %+v", ordered.Rows)
		}
		pendingOrders := fx.report(t, fx.admin, "purchases.pending", "")
		if len(pendingOrders.Rows) != 1 || !equal(t, pendingOrders.Rows[0]["pending_total"], "174") {
			t.Fatalf("Pendiente de recepción: %+v", pendingOrders.Rows)
		}
		bySupplier := fx.report(t, fx.admin, "purchases.by_supplier", "")
		north := find(t, bySupplier, "supplier", "Distribuidora Norte")
		market := find(t, bySupplier, "supplier", "Compras de mostrador")
		if !equal(t, north["total"], "224") || !equal(t, north["ordered_total"], "348") || !equal(t, north["returned_value"], "29") ||
			!equal(t, market["total"], "97.44") || !equal(t, market["cash_linked"], "97.44") || !equal(t, bySupplier.Totals["total"], "321.44") {
			t.Fatalf("Compras por proveedor: %+v", bySupplier.Rows)
		}
		returns := fx.report(t, fx.admin, "purchases.supplier_returns", "")
		if len(returns.Rows) != 1 || !equal(t, returns.Totals["value"], "29") {
			t.Fatalf("Devoluciones a proveedor: %+v", returns.Rows)
		}
		links := fx.report(t, fx.admin, "purchases.cash_links", "")
		if len(links.Rows) != 1 || !equal(t, links.Totals["linked"], "97.44") || text(links.Rows[0]["concept"]) != "Refrescos del mercado" {
			t.Fatalf("Compras pagadas desde caja: %+v", links.Rows)
		}
		// El gasto existe en caja una vez y no cambia el total de compras.
		operations := fx.report(t, fx.admin, "cash.operations", "")
		if !equal(t, operations.Totals["out_mxn"], "127.44") {
			t.Fatalf("Gastos de caja de hoy: %+v", operations.Totals)
		}
		activity := find(t, fx.report(t, fx.admin, "suppliers.activity", ""), "supplier", "Distribuidora Norte")
		if text(activity["receipts"]) != "2" || text(activity["open_orders"]) != "1" {
			t.Fatalf("Proveedores: %+v", activity)
		}
		// Quien consulta compras sin costos no ve el valor de inventario de una recepción.
		buyer := fx.staff(t, "compras_reporte", "", reports.PermissionRead, reports.PermissionPurchases)
		plain := fx.report(t, buyer, "purchases.received", "")
		if plain.has("inventory_value") || plain.has("unit_cost") || !plain.has("total") || plain.has("concept") {
			t.Fatalf("Columnas de compras sin costos: %+v", plain.Columns)
		}
		if len(fx.report(t, fx.admin, "costs.cost_adjustments", "").Rows) == 0 {
			t.Fatal("La recepción que cubrió el déficit de latas debe registrar un ajuste de costo")
		}
	})

	t.Run("empleados sin doble conteo", func(t *testing.T) {
		summary := fx.report(t, fx.admin, "sales.summary", "")
		responsible := fx.report(t, fx.admin, "employees.responsible_sales", "")
		if !amount(t, responsible.Totals["net"]).Equal(amount(t, summary.Totals["net"])) {
			t.Fatalf("Cada venta se atribuye una vez a su responsable: %+v %+v", responsible.Totals, summary.Totals)
		}
		activity := fx.report(t, fx.admin, "employees.activity", "")
		byUser := fx.report(t, fx.admin, "payments.by_user", "")
		if !amount(t, activity.Totals["collected"]).Equal(amount(t, byUser.Totals["net"])) {
			t.Fatalf("Lo cobrado por usuario coincide con cobros: %+v %+v", activity.Totals, byUser.Totals)
		}
		admin := find(t, activity, "user", "Administrador")
		confirmations := fx.scalar(t, `SELECT count(*)::text FROM app.restaurant_orders WHERE kind='confirmation'
            AND created_at >= $1::date - interval '1 day'`, today)
		if text(admin["confirmations"]) != confirmations || text(admin["cash_operations"]) == "0" {
			t.Fatalf("Actividad del administrador: %+v (confirmaciones %s)", admin, confirmations)
		}
		units := fx.report(t, fx.admin, "employees.confirmations", "")
		created := fx.scalar(t, `SELECT count(*)::text FROM app.restaurant_item_units u JOIN app.restaurant_items i ON i.id=u.item_id
            JOIN app.restaurant_orders o ON o.id=i.original_order_id WHERE u.created_at=o.created_at`)
		if text(units.Totals["units"]) != created {
			t.Fatalf("Unidades confirmadas: %+v (%s)", units.Totals, created)
		}
		if strings.Contains(string(activity.raw), "Capturó") || strings.Contains(string(activity.raw), "captured") {
			t.Fatal("No se reporta «capturó»: no se conserva")
		}
	})

	t.Run("diferencias por moneda con el tipo de cambio histórico", func(t *testing.T) {
		summary := decode[payments.ShiftSummaryView](t, fx.get(t, fx.admin, "payments/shifts/"+shift.ID+"/summary", 200))
		row := find(t, fx.report(t, fx.admin, "cash.shifts", threeDays), "register", "Caja 1")
		if !equal(t, row["expected_mxn"], summary.Expected.MXN) || !equal(t, row["expected_usd"], summary.Expected.USD) {
			t.Fatalf("El esperado del reporte es el de Caja: %+v %+v", row, summary.Expected)
		}
		// Se cuentan $2.50 menos en pesos y US$1 más: diferencia equivalente −2.50 + 1 × 18.00 = 15.50.
		counted := decimal.RequireFromString(summary.Expected.MXN).Sub(decimal.RequireFromString("2.50")).StringFixed(2)
		countedUSD := decimal.RequireFromString(summary.Expected.USD).Add(decimal.NewFromInt(1)).StringFixed(2)
		fx.mutate(t, "POST", "payments/shifts/"+shift.ID+"/closing", map[string]any{}, 200)
		view := decode[payments.ClosingView](t, fx.get(t, fx.admin, "payments/shifts/"+shift.ID+"/closing", 200))
		stored := decode[payments.ClosingView](t, fx.mutate(t, "PUT", "payments/shifts/"+shift.ID+"/closing/count", map[string]any{
			"revision": view.Count.Revision, "notes": "", "lines": []any{}, "extra_mxn": counted, "extra_usd": countedUSD,
			"extra_note": "Conteo de prueba",
		}, 200))
		fx.mutate(t, "POST", "payments/shifts/"+shift.ID+"/closing/confirm", map[string]any{"count_revision": stored.Count.Revision}, 200)
		closed := find(t, fx.report(t, fx.admin, "cash.shifts", threeDays), "register", "Caja 1")
		if !equal(t, closed["difference_mxn"], "-2.50") || !equal(t, closed["difference_usd"], "1") ||
			!equal(t, closed["difference_equivalent_mxn"], "15.50") || text(closed["result"]) != "Faltante en pesos · Sobrante en dólares" ||
			!equal(t, closed["usd_rate"], "18") {
			t.Fatalf("Diferencias por moneda: %+v", closed)
		}
		byRegister := find(t, fx.report(t, fx.admin, "cash.by_register", threeDays), "register", "Caja 1")
		if !equal(t, byRegister["short_mxn"], "2.50") || !equal(t, byRegister["over_usd"], "1") {
			t.Fatalf("Faltantes y sobrantes por moneda: %+v", byRegister)
		}
		raw := string(fx.get(t, limited, "reports/run/cash.shifts?page_size=200&"+threeDays, 200))
		for _, fragment := range []string{`"-2.50"`, `"15.50"`, `"difference_usd"`, `"result"`} {
			if strings.Contains(raw, fragment) {
				t.Fatalf("Sin cash.expected.read se reveló %s: %s", fragment, raw)
			}
		}
	})

	t.Run("dashboard con caja, compras, inventario, costos y margen bajo sus permisos", func(t *testing.T) {
		admin := decode[map[string]any](t, fx.get(t, fx.admin, "reports/dashboard?period=today", 200))
		sections := map[string]map[string]any{}
		for _, raw := range admin["sections"].([]any) {
			section := raw.(map[string]any)
			indicators := map[string]any{}
			for _, indicator := range section["indicators"].([]any) {
				entry := indicator.(map[string]any)
				indicators[entry["key"].(string)] = entry
			}
			sections[section["id"].(string)] = indicators
		}
		for _, id := range []string{"cash", "cash_differences", "purchases", "inventory", "inventory_costs", "margin"} {
			if sections[id] == nil {
				t.Fatalf("Falta la sección %s del dashboard: %+v", id, admin)
			}
		}
		margin := sections["margin"]["margin"].(map[string]any)
		if margin["value"] == nil || margin["previous"] != nil {
			t.Fatalf("El margen solo tiene valor con datos suficientes (ayer no hubo ventas): %+v", margin)
		}
		if !equal(t, sections["purchases"]["received"].(map[string]any)["value"], "321.44") {
			t.Fatalf("Compras del dashboard: %+v", sections["purchases"])
		}
		noCosts := fx.staff(t, "ventas_sin_costos", "", reports.PermissionRead, reports.PermissionSales)
		dashboard := string(fx.get(t, noCosts, "reports/dashboard", 200))
		if strings.Contains(dashboard, `"id":"margin"`) || strings.Contains(dashboard, "inventory_costs") {
			t.Fatalf("Sin inventory.costs.read no hay margen ni costos: %s", dashboard)
		}
	})

	t.Run("totales del detalle iguales a los resumidos en todos los reportes", func(t *testing.T) {
		for _, definition := range reports.Definitions() {
			query := ""
			if definition.Period == reports.PeriodRequired {
				query = threeDays
			}
			full := fx.report(t, fx.admin, definition.ID, query)
			paged := fx.report(t, fx.admin, definition.ID, query+"&page_size=1&page=1")
			for _, column := range definition.Columns {
				if column.Total != reports.TotalSum || !full.has(column.Key) {
					continue
				}
				if !sum(t, full, column.Key).Equal(amount(t, full.Totals[column.Key])) ||
					!amount(t, paged.Totals[column.Key]).Equal(amount(t, full.Totals[column.Key])) {
					t.Fatalf("%s.%s: detalle %s, total %v, total paginado %v", definition.ID, column.Key, sum(t, full, column.Key),
						full.Totals[column.Key], paged.Totals[column.Key])
				}
			}
		}
	})
}

// inventoryFixture son los insumos, almacenes y recetas del escenario.
type inventoryFixture struct {
	units        map[string]string
	items        map[string]string
	general      string
	barra        string
	burgerRecipe string
}

// setupInventory da de alta (con el inventario aún desactivado) pan, carne y latas con existencia
// inicial en el almacén general, el almacén «Barra» y las recetas de la hamburguesa (1 pan + 150 g
// de carne = $33) y del refresco (1 lata = $12). La ensalada no tiene receta.
// workbookText concatena las partes descomprimidas de un libro de Excel exportado.
func workbookText(t *testing.T, content []byte) string {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	var builder strings.Builder
	for _, entry := range archive.File {
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		builder.Write(data)
	}
	return builder.String()
}

func setupInventory(t *testing.T, fx *fixture) inventoryFixture {
	t.Helper()
	result := inventoryFixture{units: map[string]string{}, items: map[string]string{}}
	for _, unit := range decode[[]inventory.Unit](t, fx.get(t, fx.admin, "inventory/units", 200)) {
		result.units[unit.Code] = unit.ID
	}
	result.general = fx.scalar(t, "SELECT id::text FROM app.inventory_warehouses WHERE is_default")
	result.barra = decode[map[string]any](t, fx.mutate(t, "POST", "inventory/warehouses", map[string]any{"name": "Barra", "active": true}, 200))["id"].(string)
	for _, item := range []struct{ code, name, unit, minimum string }{
		{"PAN", "Pan", "pza", "20"}, {"CARNE", "Carne molida", "g", ""}, {"LATA", "Refresco en lata", "pza", ""},
	} {
		body := map[string]any{"code": item.code, "name": item.name, "base_unit_id": result.units[item.unit], "kind": "purchased", "active": true}
		if item.minimum != "" {
			body["min_stock"] = item.minimum
		}
		result.items[item.code] = decode[map[string]any](t, fx.mutate(t, "POST", "inventory/items", body, 200))["id"].(string)
	}
	fx.mutate(t, "POST", "inventory/initial-balances", map[string]any{"warehouse_id": result.general, "lines": []map[string]any{
		{"item_id": result.items["PAN"], "quantity": "10", "unit_cost": "3"},
		{"item_id": result.items["CARNE"], "quantity": "2000", "unit_cost": "0.2"},
		{"item_id": result.items["LATA"], "quantity": "2", "unit_cost": "12"},
	}}, 200)
	recipe := func(product string, lines ...map[string]any) string {
		return decode[map[string]any](t, fx.mutate(t, "POST", "inventory/recipes", map[string]any{"owner_kind": "product", "product_id": product,
			"active": true, "lines": lines}, 200))["id"].(string)
	}
	item := func(code, quantity, unit string) map[string]any {
		return map[string]any{"component_kind": "item", "item_id": result.items[code], "quantity": quantity, "unit_id": result.units[unit],
			"takeaway_only": false}
	}
	result.burgerRecipe = recipe(fx.burger.ID, item("PAN", "1", "pza"), item("CARNE", "150", "g"))
	recipe(fx.soda.ID, item("LATA", "1", "pza"))
	return result
}
