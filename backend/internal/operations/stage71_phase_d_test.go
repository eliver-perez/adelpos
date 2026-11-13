//go:build integration

package operations_test

import (
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa 7.1, fase D (pendientes acordados): motivos de descuento, total equivalente en dólares en
// la precuenta y el ticket, e indicaciones rápidas por categoría. Se encadena al final de la
// aceptación con su propia caja, cajero y turno para no depender del estado de pruebas anteriores.

type phaseDFixture struct {
	*paymentsFixture
	cashier  testTerminal
	shiftID  string
	reasons  map[string]payments.DiscountReason
	printer  kitchen.Printer
	register string
}

func (fixture *phaseDFixture) loadReasons(t *testing.T) []payments.DiscountReason {
	t.Helper()
	list := decodeResponse[[]payments.DiscountReason](t, fixture.call(t, fixture.cashier, "GET", "payments/discount-reasons", nil, 200))
	fixture.reasons = map[string]payments.DiscountReason{}
	for _, reason := range list {
		fixture.reasons[reason.Name] = reason
	}
	return list
}

// saveReason guarda un motivo existente con su revisión vigente.
func (fixture *phaseDFixture) saveReason(t *testing.T, name string, changes map[string]any, status int) []byte {
	t.Helper()
	fixture.loadReasons(t)
	reason := fixture.reasons[name]
	body := map[string]any{
		"operation_id": randomID(), "revision": reason.Revision, "name": reason.Name,
		"requires_description": reason.RequiresDescription, "active": reason.Active,
	}
	for key, value := range changes {
		body[key] = value
	}
	return fixture.adminCall(t, "PATCH", "payments/discount-reasons/"+reason.ID, body, status)
}

// discount aplica un descuento general del 10 % (o una cortesía) desde la terminal del cajero.
func (fixture *phaseDFixture) discount(t *testing.T, accountID string, courtesy bool, reasonID any, reason string, status int) []byte {
	t.Helper()
	body := map[string]any{
		"scope": "account", "method": "percent", "value": "10", "courtesy": courtesy, "reason": reason,
		"expected_balance": fixture.finance(t, accountID).Balance,
	}
	if courtesy {
		body["value"] = "100"
	}
	if reasonID != nil {
		body["reason_id"] = reasonID
	}
	return fixture.post(t, fixture.cashier, accountID, "discounts", body, status)
}

func lastAdjustment(t *testing.T, finance payments.Finance) payments.Adjustment {
	t.Helper()
	if len(finance.Adjustments) == 0 {
		t.Fatal("La cuenta no tiene ajustes")
	}
	return finance.Adjustments[len(finance.Adjustments)-1]
}

func testStage71PhaseD(t *testing.T, base *restaurantFixture) {
	payment := &paymentsFixture{restaurantFixture: base, methods: map[string]payments.Method{}, registers: map[string]string{}}
	for _, method := range decodeResponse[[]payments.Method](t, base.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil {
			payment.methods[*method.Code] = method
		}
	}
	register := decodeResponse[struct {
		ID string `json:"id"`
	}](t, base.adminCall(t, "POST", "registers", map[string]any{"name": "Caja de motivos 7.1", "description": "", "active": true}, 200)).ID
	payment.registers["primary"] = register
	role := payment.roleWithLimit(t, "Cajero de motivos 7.1", "100",
		"accounts.read", "payments.read", "payments.collect", "payments.collect_partial", "payments.discounts.apply",
		"payments.courtesy", "payments.tickets.print", "cash.shifts.open", "cash.shifts.read", "payments.usd_rate.manage")
	fixture := &phaseDFixture{paymentsFixture: payment, register: register}
	fixture.cashier = payment.staffWithRole(t, "cajero_motivos", role)
	// La función de dólares queda desactivada al iniciar: el turno se abre sin tipo de cambio.
	if payment.usdFeature(t).Enabled {
		payment.setUSDFeature(t, false)
	}
	fixture.shiftID = decodeResponse[payments.Shift](t, base.call(t, fixture.cashier, "POST", "payments/shifts", map[string]any{
		"operation_id": randomID(), "opening_float": "0",
	}, 200)).ID
	// Impresora de tickets sin hardware (una cola del equipo que no existe; estas pruebas no ejecutan el
	// trabajador): los documentos quedan en la cola y nunca se envían a una impresora real. Desde la etapa
	// 9F una impresora de otra terminal («agent») ya no es válida para precuentas ni tickets.
	fixture.printer = decodeResponse[kitchen.Printer](t, base.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name: "Tickets de la fase D", Transport: "cups", Destination: "ADEL_TEST_PHASE_D", Active: true, PaperWidth: 80, Copies: 1,
	}, 200))
	payment.receiptSettings(t, fixture.printer.ID, false)

	testDiscountReasonCatalog(t, fixture)
	testDiscountsWithReasons(t, fixture)
	testDiscountReasonAdministration(t, fixture)
	testDollarTotals(t, fixture)
	testCategoryQuickNotes(t, base)
}

func testDiscountReasonCatalog(t *testing.T, fixture *phaseDFixture) {
	t.Run("71D_motivos_sembrados_en_orden_y_solo_otro_exige_descripcion", func(t *testing.T) {
		list := fixture.loadReasons(t)
		names := []string{}
		for _, reason := range list {
			names = append(names, reason.Name)
			if !reason.Active || reason.RequiresDescription != (reason.Name == "Otro") {
				t.Fatalf("Motivo sembrado incorrecto: %+v", reason)
			}
		}
		if strings.Join(names, "|") != "Atención|Promoción|Demora|Cortesía de la casa|Otro" {
			t.Fatalf("Siembra o orden de los motivos: %v", names)
		}
		// Sin payments.read no se consultan.
		fixture.call(t, fixture.waiter, "GET", "payments/discount-reasons", nil, 403)
	})
}

func testDiscountsWithReasons(t *testing.T, fixture *phaseDFixture) {
	fries := line(fixture.fries, 2)
	t.Run("71D_descuento_y_cortesia_con_motivo_guardan_reason_id", func(t *testing.T) {
		bill := fixture.account(t, "Descuento con motivo", fries)
		attention := fixture.reasons["Atención"]
		applied := decodeResponse[adjustmentResult](t, fixture.discount(t, bill.Account.ID, false, attention.ID, "", 200))
		adjustment := lastAdjustment(t, applied.Finance)
		if adjustment.ReasonID == nil || *adjustment.ReasonID != attention.ID || text(adjustment.ReasonName) != "Atención" || adjustment.Reason != "" {
			t.Fatalf("El descuento no conservó su motivo: %+v", adjustment)
		}
		var stored *string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT reason_id::text FROM app.account_adjustments WHERE id=$1", applied.AdjustmentID).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if text(stored) != attention.ID {
			t.Fatal("El reason_id no quedó guardado para los reportes")
		}
		// La auditoría del descuento incluye el motivo.
		if fixture.countRows(t, `SELECT count(*) FROM app.audit_events WHERE action='payments.discount_applied' AND entity_id=$1
            AND after_state->'detail'->>'reason_id'=$2 AND after_state->'detail'->>'reason_name'='Atención'`, bill.Account.ID, attention.ID) != 1 {
			t.Fatal("La auditoría del descuento no registró su motivo")
		}
		fixture.registers["attention_account"] = bill.Account.ID
		courtesy := fixture.account(t, "Cortesía con motivo", line(fixture.fries, 1))
		house := fixture.reasons["Cortesía de la casa"]
		granted := decodeResponse[adjustmentResult](t, fixture.discount(t, courtesy.Account.ID, true, house.ID, "Cliente de la casa", 200))
		given := lastAdjustment(t, granted.Finance)
		if !granted.Closed || !given.Courtesy || text(given.ReasonName) != "Cortesía de la casa" || given.Reason != "Cliente de la casa" {
			t.Fatalf("La cortesía no conservó su motivo y descripción: %+v", given)
		}
	})

	t.Run("71D_otro_exige_descripcion_y_el_motivo_debe_existir", func(t *testing.T) {
		bill := fixture.account(t, "Descuento por otro motivo", fries)
		other := fixture.reasons["Otro"]
		before := fixture.countRows(t, "SELECT count(*) FROM app.account_adjustments")
		for _, description := range []string{"", "Mal", strings.Repeat("x", 501)} {
			fixture.discount(t, bill.Account.ID, false, other.ID, description, 400)
		}
		fixture.discount(t, bill.Account.ID, false, randomID(), "Motivo inexistente", 404)
		fixture.discount(t, bill.Account.ID, false, "no-es-un-uuid", "Motivo inválido", 400)
		// Un motivo sin descripción obligatoria admite hasta 500 caracteres.
		fixture.discount(t, bill.Account.ID, false, fixture.reasons["Demora"].ID, strings.Repeat("y", 501), 400)
		if fixture.countRows(t, "SELECT count(*) FROM app.account_adjustments") != before {
			t.Fatal("Un descuento rechazado dejó registros")
		}
		applied := decodeResponse[adjustmentResult](t, fixture.discount(t, bill.Account.ID, false, other.ID, "  Platillo frío  ", 200))
		if adjustment := lastAdjustment(t, applied.Finance); adjustment.Reason != "Platillo frío" || text(adjustment.ReasonName) != "Otro" {
			t.Fatalf("«Otro» no guardó su descripción: %+v", adjustment)
		}
	})

	t.Run("71D_sin_reason_id_conserva_la_regla_de_la_etapa_6", func(t *testing.T) {
		bill := fixture.account(t, "Descuento sin motivo del catálogo", fries)
		fixture.discount(t, bill.Account.ID, false, nil, "", 400)
		fixture.discount(t, bill.Account.ID, false, nil, "Corto", 200)
		finance := fixture.finance(t, bill.Account.ID)
		if adjustment := lastAdjustment(t, finance); adjustment.ReasonID != nil || adjustment.ReasonName != nil || adjustment.Reason != "Corto" {
			t.Fatalf("El descuento sin reason_id no se registró como en la etapa 6: %+v", adjustment)
		}
	})

	t.Run("71D_motivo_inactivo_rechazado_y_desactivar_no_altera_lo_registrado", func(t *testing.T) {
		accountID := fixture.registers["attention_account"]
		before := fixture.finance(t, accountID)
		fixture.saveReason(t, "Atención", map[string]any{"active": false}, 200)
		bill := fixture.account(t, "Motivo desactivado", fries)
		count := fixture.countRows(t, "SELECT count(*) FROM app.account_adjustments")
		problem := fixture.discount(t, bill.Account.ID, false, fixture.reasons["Atención"].ID, "", 409)
		if errorCode(t, problem) != "DISCOUNT_REASON_INACTIVE" || fixture.countRows(t, "SELECT count(*) FROM app.account_adjustments") != count {
			t.Fatalf("Se aceptó un motivo desactivado: %s", problem)
		}
		after := fixture.finance(t, accountID)
		if text(lastAdjustment(t, after).ReasonName) != "Atención" || *lastAdjustment(t, after).ReasonID != *lastAdjustment(t, before).ReasonID ||
			after.Total != before.Total {
			t.Fatal("Desactivar el motivo alteró el descuento registrado")
		}
		// El formulario de descuento solo ofrece activos; la lista conserva el inactivo.
		fixture.loadReasons(t)
		if fixture.reasons["Atención"].Active {
			t.Fatal("El motivo no quedó desactivado")
		}
		fixture.saveReason(t, "Atención", map[string]any{"active": true}, 200)
	})

	t.Run("71D_motivo_en_la_precuenta_el_ticket_y_el_historial", func(t *testing.T) {
		accountID := fixture.registers["attention_account"]
		printed := decodeResponse[printingResult](t, fixture.post(t, fixture.cashier, accountID, "precheck/print", map[string]any{}, 200))
		var content string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT content FROM app.kitchen_print_jobs WHERE id=$1", printed.JobID).Scan(&content); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(content, "Descuento manual · Atención") || !strings.Contains(content, "PRECUENTA") {
			t.Fatalf("La precuenta no muestra el motivo:\n%s", content)
		}
		receipt := decodeResponse[struct {
			Printing printingResult `json:"printing"`
		}](t, fixture.pay(t, fixture.cashier, accountID, map[string]any{
			"method_id": fixture.methods["cash"].ID, "mode": "total", "print": true,
		}, 200))
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT content FROM app.kitchen_print_jobs WHERE id=$1", receipt.Printing.JobID).Scan(&content); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(content, "TICKET DE PAGO") || !strings.Contains(content, "Descuento manual · Atención") {
			t.Fatalf("El ticket no muestra el motivo:\n%s", content)
		}
		// El historial de la cuenta (ajustes registrados) conserva motivo y descripción.
		history := fixture.finance(t, accountID)
		if text(lastAdjustment(t, history).ReasonName) != "Atención" {
			t.Fatal("El historial de la cuenta perdió el motivo")
		}
	})

	t.Run("71D_renombrar_un_motivo_no_altera_lo_registrado", func(t *testing.T) {
		accountID := fixture.registers["attention_account"]
		fixture.saveReason(t, "Atención", map[string]any{"name": "Atención al cliente"}, 200)
		fixture.loadReasons(t)
		renamed := lastAdjustment(t, fixture.finance(t, accountID))
		if text(renamed.ReasonName) != "Atención" || renamed.ReasonID == nil || *renamed.ReasonID != fixture.reasons["Atención al cliente"].ID {
			t.Fatalf("Renombrar el motivo alteró el descuento registrado: %v", text(renamed.ReasonName))
		}
		// Un descuento nuevo guarda el nombre vigente.
		bill := fixture.account(t, "Motivo renombrado", fries)
		fixture.discount(t, bill.Account.ID, false, fixture.reasons["Atención al cliente"].ID, "", 200)
		if text(lastAdjustment(t, fixture.finance(t, bill.Account.ID)).ReasonName) != "Atención al cliente" {
			t.Fatal("El descuento nuevo no guardó el nombre vigente del motivo")
		}
		fixture.saveReason(t, "Atención al cliente", map[string]any{"name": "Atención"}, 200)
		fixture.loadReasons(t)
	})
}

func testDiscountReasonAdministration(t *testing.T, fixture *phaseDFixture) {
	t.Run("71D_administracion_de_motivos_con_permiso_auditada_e_idempotente", func(t *testing.T) {
		before := fixture.countRows(t, "SELECT count(*) FROM app.discount_reasons")
		body := map[string]any{"operation_id": randomID(), "name": "Cumpleaños", "requires_description": false, "active": true, "position": map[string]any{"after": ""}}
		// Sin payments.settings.manage: 403 sin registros.
		fixture.call(t, fixture.cashier, "POST", "payments/discount-reasons", body, 403)
		if fixture.countRows(t, "SELECT count(*) FROM app.discount_reasons") != before {
			t.Fatal("Un usuario sin permiso creó un motivo")
		}
		created := decodeResponse[payments.DiscountReason](t, fixture.adminCall(t, "POST", "payments/discount-reasons", body, 200))
		replay := decodeResponse[payments.DiscountReason](t, fixture.adminCall(t, "POST", "payments/discount-reasons", body, 200))
		if replay.ID != created.ID || fixture.countRows(t, "SELECT count(*) FROM app.discount_reasons") != before+1 {
			t.Fatal("El reintento con el mismo operation_id duplicó el motivo")
		}
		if auditCount(t, fixture.restaurantFixture, "payments.discount_reason_created", created.ID) != 1 ||
			auditCount(t, fixture.restaurantFixture, "payments.discount_reasons_reordered", created.ID) != 1 {
			t.Fatal("La creación o el orden del motivo no se auditaron una vez")
		}
		list := fixture.loadReasons(t)
		if list[0].ID != created.ID {
			t.Fatal("El motivo creado «al inicio» no quedó primero")
		}
		// Nombre duplicado, revisión obsoleta, nombre vacío y sin permiso.
		fixture.adminCall(t, "POST", "payments/discount-reasons", map[string]any{"operation_id": randomID(), "name": "Demora", "active": true}, 409)
		fixture.adminCall(t, "PATCH", "payments/discount-reasons/"+created.ID, map[string]any{
			"operation_id": randomID(), "revision": created.Revision + 5, "name": "Cumpleaños", "active": true,
		}, 409)
		fixture.adminCall(t, "PATCH", "payments/discount-reasons/"+created.ID, map[string]any{
			"operation_id": randomID(), "revision": fixture.reasons["Cumpleaños"].Revision, "name": "  ", "active": true,
		}, 400)
		fixture.call(t, fixture.cashier, "PATCH", "payments/discount-reasons/"+created.ID, map[string]any{
			"operation_id": randomID(), "revision": fixture.reasons["Cumpleaños"].Revision, "name": "Cumpleaños", "active": false,
		}, 403)
		saved := decodeResponse[payments.DiscountReason](t, fixture.saveReason(t, "Cumpleaños", map[string]any{
			"name": "Cumpleaños del cliente", "requires_description": true, "position": map[string]any{"after": fixture.reasons["Otro"].ID},
		}, 200))
		if saved.Name != "Cumpleaños del cliente" || !saved.RequiresDescription || saved.Revision <= created.Revision ||
			auditCount(t, fixture.restaurantFixture, "payments.discount_reason_updated", created.ID) != 1 {
			t.Fatalf("La edición del motivo no se guardó ni auditó: %+v", saved)
		}
		list = fixture.loadReasons(t)
		if list[len(list)-1].ID != created.ID {
			t.Fatal("El motivo no quedó después de «Otro»")
		}
		// Debe quedar al menos un motivo activo.
		for _, reason := range list {
			if reason.ID != created.ID {
				fixture.saveReason(t, reason.Name, map[string]any{"active": false}, 200)
			}
		}
		if errorCode(t, fixture.saveReason(t, "Cumpleaños del cliente", map[string]any{"active": false}, 409)) != "LAST_ACTIVE_REASON" {
			t.Fatal("Se desactivaron todos los motivos")
		}
		for _, reason := range list {
			if reason.ID != created.ID {
				fixture.saveReason(t, reason.Name, map[string]any{"active": true}, 200)
			}
		}
		fixture.saveReason(t, "Cumpleaños del cliente", map[string]any{"active": false}, 200)
	})
}

// dollarLine es la línea informativa esperada con el total y el tipo de cambio del turno.
func dollarLine(t *testing.T, total, rate string) (string, string) {
	t.Helper()
	usd := amountOf(t, total).DivRound(amountOf(t, rate), 2).StringFixed(2)
	return "Total en dólares (T.C. $" + rate + ")", "US$" + usd
}

func testDollarTotals(t *testing.T, fixture *phaseDFixture) {
	precheck := func(t *testing.T, terminal testTerminal, accountID string) payments.CommercialDocument {
		t.Helper()
		return decodeResponse[payments.CommercialDocument](t, fixture.call(t, terminal, "GET", "payments/accounts/"+accountID+"/precheck", nil, 200))
	}
	bill := fixture.account(t, "Equivalente USD 7.1", line(fixture.fries, 3))

	t.Run("71D_sin_la_funcion_o_sin_tipo_de_cambio_no_aparece_el_total_en_dolares", func(t *testing.T) {
		document := precheck(t, fixture.cashier, bill.Account.ID)
		if document.USDTotal != nil || strings.Contains(document.Content, "dólares") {
			t.Fatalf("Con la función desactivada apareció el total en dólares:\n%s", document.Content)
		}
		fixture.setUSDFeature(t, true)
		// El turno de la caja se abrió sin tipo de cambio: tampoco aparece.
		document = precheck(t, fixture.cashier, bill.Account.ID)
		if document.USDTotal != nil || strings.Contains(document.Content, "Total en dólares") {
			t.Fatalf("Sin tipo de cambio apareció el total en dólares:\n%s", document.Content)
		}
	})

	t.Run("71D_precuenta_y_ticket_con_el_tipo_de_cambio_del_turno", func(t *testing.T) {
		fixture.call(t, fixture.cashier, "POST", "payments/shifts/"+fixture.shiftID+"/usd-rate", map[string]any{
			"operation_id": randomID(), "usd_rate": "18.50", "confirm_rate_difference": true,
		}, 200)
		document := precheck(t, fixture.cashier, bill.Account.ID)
		label, amount := dollarLine(t, document.Total, "18.50")
		if text(document.USDRate) != "18.50" || "US$"+text(document.USDTotal) != amount ||
			!strings.Contains(document.Content, label) || !strings.Contains(document.Content, amount) {
			t.Fatalf("La precuenta no muestra el total en dólares %s %s:\n%s", label, amount, document.Content)
		}
		// Es informativo: los importes en pesos no cambian.
		finance := fixture.finance(t, bill.Account.ID)
		if document.Total != finance.Total || document.Balance != finance.Balance {
			t.Fatal("El total en dólares alteró los importes del documento")
		}
		// Una terminal cuya caja no tiene turno activo no muestra la línea en su precuenta.
		other := decodeResponse[struct {
			ID string `json:"id"`
		}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": "Caja sin turno 7.1", "description": "", "active": true}, 200)).ID
		idle := newCashier(t, fixture.paymentsFixture, "precuenta_sin_turno", []string{"accounts.read", "payments.read"}, other)
		if idleDocument := precheck(t, idle, bill.Account.ID); idleDocument.USDTotal != nil || strings.Contains(idleDocument.Content, "Total en dólares") {
			t.Fatalf("La precuenta sin turno activo mostró el total en dólares:\n%s", idleDocument.Content)
		}
		// El ticket usa el tipo de cambio del turno del pago.
		receipt := decodeResponse[struct {
			Payment  payments.PaymentRecord `json:"payment"`
			Printing printingResult         `json:"printing"`
		}](t, fixture.pay(t, fixture.cashier, bill.Account.ID, map[string]any{
			"method_id": fixture.methods["debit"].ID, "mode": "total", "print": true,
		}, 200))
		if receipt.Payment.ShiftID != fixture.shiftID {
			t.Fatal("El pago no quedó en el turno de la caja del cajero")
		}
		var content, rate string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT j.content,t.document->>'usd_rate' FROM app.kitchen_print_jobs j
            JOIN app.kitchen_tickets t ON t.id=j.ticket_id WHERE j.id=$1`, receipt.Printing.JobID).Scan(&content, &rate); err != nil {
			t.Fatal(err)
		}
		if rate != "18.50" || !strings.Contains(content, label) || !strings.Contains(content, amount) || !strings.Contains(content, "TICKET DE PAGO") {
			t.Fatalf("El ticket no muestra el total en dólares del turno del pago:\n%s", content)
		}
	})

	t.Run("71D_al_desactivar_la_funcion_desaparece_del_ticket_y_la_precuenta", func(t *testing.T) {
		fixture.setUSDFeature(t, false)
		next := fixture.account(t, "Sin equivalente USD 7.1", line(fixture.fries, 1))
		if document := precheck(t, fixture.cashier, next.Account.ID); document.USDTotal != nil || strings.Contains(document.Content, "dólares") {
			t.Fatalf("La precuenta conservó el total en dólares con la función desactivada:\n%s", document.Content)
		}
		receipt := decodeResponse[struct {
			Printing printingResult `json:"printing"`
		}](t, fixture.pay(t, fixture.cashier, next.Account.ID, map[string]any{
			"method_id": fixture.methods["debit"].ID, "mode": "total", "print": true,
		}, 200))
		var content string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT content FROM app.kitchen_print_jobs WHERE id=$1", receipt.Printing.JobID).Scan(&content); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(content, "dólares") {
			t.Fatalf("El ticket mostró el total en dólares con la función desactivada:\n%s", content)
		}
	})
}

func quickNoteBody(categoryID, text string, active bool, after *string) map[string]any {
	body := map[string]any{"category_id": categoryID, "text": text, "active": active, "revision": 0}
	if after != nil {
		body["position"] = map[string]any{"after": *after}
	}
	return body
}

func lookupQuickNotes(t *testing.T, fixture *restaurantFixture, terminal testTerminal, categoryID string) []catalog.CategoryQuickNote {
	t.Helper()
	lookups := decodeResponse[struct {
		Notes []catalog.CategoryQuickNote `json:"category_quick_notes"`
	}](t, fixture.call(t, terminal, "GET", "catalog/lookups", nil, 200))
	result := []catalog.CategoryQuickNote{}
	for _, note := range lookups.Notes {
		if note.CategoryID == categoryID {
			result = append(result, note)
		}
	}
	return result
}

// rowCount cuenta filas con la conexión de la aceptación.
func rowCount(t *testing.T, fixture *restaurantFixture, query string, args ...any) int {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func testCategoryQuickNotes(t *testing.T, fixture *restaurantFixture) {
	categoryID := fixture.category.ID
	var onion, cooked catalog.CategoryQuickNote

	t.Run("71D_indicaciones_rapidas_administradas_con_permiso_y_auditadas", func(t *testing.T) {
		before := rowCount(t, fixture, "SELECT count(*) FROM app.catalog_category_quick_notes")
		// El mesero consulta el catálogo pero no administra categorías: 403 sin registros.
		fixture.call(t, fixture.waiter, "POST", "catalog/quick-notes", quickNoteBody(categoryID, "Sin cebolla", true, nil), 403)
		if rowCount(t, fixture, "SELECT count(*) FROM app.catalog_category_quick_notes") != before {
			t.Fatal("Un usuario sin permiso creó una indicación rápida")
		}
		onion = decodeResponse[catalog.CategoryQuickNote](t, fixture.adminCall(t, "POST", "catalog/quick-notes", quickNoteBody(categoryID, "  Sin cebolla ", true, nil), 200))
		cooked = decodeResponse[catalog.CategoryQuickNote](t, fixture.adminCall(t, "POST", "catalog/quick-notes", quickNoteBody(categoryID, "Bien cocida", true, &onion.ID), 200))
		if onion.Text != "Sin cebolla" || onion.CategoryID != categoryID || onion.Revision != 1 || cooked.SortOrder <= onion.SortOrder {
			t.Fatalf("Indicaciones creadas: %+v %+v", onion, cooked)
		}
		if auditCount(t, fixture, "catalog.quick-notes.created", onion.ID) != 1 || auditCount(t, fixture, "catalog.quick-notes.created", cooked.ID) != 1 {
			t.Fatal("La creación de indicaciones no se auditó")
		}
		// Comas, vacías, más de 80 caracteres, saltos de línea, duplicadas, categoría inexistente.
		count := rowCount(t, fixture, "SELECT count(*) FROM app.catalog_category_quick_notes")
		for _, invalid := range []string{"Sin cebolla, sin sal", "", "   ", strings.Repeat("a", 81), "Sin\nsal"} {
			fixture.adminCall(t, "POST", "catalog/quick-notes", quickNoteBody(categoryID, invalid, true, nil), 400)
		}
		problem := fixture.adminCall(t, "POST", "catalog/quick-notes", quickNoteBody(categoryID, "SIN CEBOLLA", true, nil), 409)
		if errorCode(t, problem) != "DUPLICATE_QUICK_NOTE" {
			t.Fatalf("Una indicación repetida no se rechazó: %s", problem)
		}
		fixture.adminCall(t, "POST", "catalog/quick-notes", quickNoteBody(randomID(), "Sin sal", true, nil), 400)
		if rowCount(t, fixture, "SELECT count(*) FROM app.catalog_category_quick_notes") != count {
			t.Fatal("Una indicación inválida dejó registros")
		}
		// Edición con revisión optimista, sin comas, sin permiso, y orden visual dentro de la categoría.
		fixture.adminCall(t, "PATCH", "catalog/quick-notes/"+cooked.ID, map[string]any{"revision": cooked.Revision + 1, "text": "Bien cocida", "active": true}, 409)
		fixture.adminCall(t, "PATCH", "catalog/quick-notes/"+cooked.ID, map[string]any{"revision": cooked.Revision, "text": "Bien, cocida", "active": true}, 400)
		fixture.call(t, fixture.waiter, "PATCH", "catalog/quick-notes/"+cooked.ID, map[string]any{"revision": cooked.Revision, "text": "Bien cocida", "active": false}, 403)
		fixture.adminCall(t, "PATCH", "catalog/quick-notes/"+cooked.ID, map[string]any{
			"revision": cooked.Revision, "text": "Bien cocida", "active": true, "category_id": randomID(),
		}, 400)
		cooked = decodeResponse[catalog.CategoryQuickNote](t, fixture.adminCall(t, "PATCH", "catalog/quick-notes/"+cooked.ID, map[string]any{
			"revision": cooked.Revision, "text": "Bien cocida", "active": true, "position": map[string]any{"after": ""},
		}, 200))
		if auditCount(t, fixture, "catalog.quick-notes.updated", cooked.ID) != 1 || auditCount(t, fixture, "catalog.quick-notes.reordered", cooked.ID) != 1 {
			t.Fatal("La edición o el orden de la indicación no se auditaron")
		}
		inactive := decodeResponse[catalog.CategoryQuickNote](t, fixture.adminCall(t, "POST", "catalog/quick-notes", quickNoteBody(categoryID, "Sin sal", false, nil), 200))
		notes := lookupQuickNotes(t, fixture, fixture.waiter, categoryID)
		if len(notes) != 3 || notes[0].ID != cooked.ID || notes[1].ID != onion.ID || notes[2].ID != inactive.ID || notes[2].Active {
			t.Fatalf("Los lookups no exponen las indicaciones en su orden con su estado: %+v", notes)
		}
	})

	t.Run("71D_indicacion_rapida_llega_a_la_comanda_como_la_escrita", func(t *testing.T) {
		bill := newAccount(t, fixture, "Indicaciones rápidas 7.1")
		route := "restaurant/accounts/" + bill.Account.ID
		// Dos indicaciones rápidas y una escrita, unidas por comas como en la captura (la línea cabe en
		// el ancho de 80 mm: la plantilla ajusta las más largas a varias líneas).
		notes := cooked.Text + ", " + onion.Text + ", Sin sal"
		items := []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{
			ProductID: fixture.fries.ID, Quantity: 1, Customization: catalog.Customization{Notes: notes},
		}}}
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", route+"/draft", map[string]any{"revision": bill.Account.Revision, "items": items}, 200))
		bill = confirmDraft(t, fixture, bill)
		if len(bill.Lines) != 1 || bill.Lines[0].Request.Customization.Notes != notes {
			t.Fatalf("La partida no conservó las indicaciones como texto: %+v", bill.Lines)
		}
		releases := kitchenReleases(t, fixture, bill)
		if len(releases) == 0 {
			t.Fatal("La partida no se liberó a cocina")
		}
		var content string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT j.content FROM app.kitchen_print_jobs j JOIN app.kitchen_tickets t ON t.id=j.ticket_id
            WHERE t.release_id=$1 ORDER BY j.created_at LIMIT 1`, releases[0].ID).Scan(&content); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(content, "NOTA: "+notes) {
			t.Fatalf("La comanda no muestra las indicaciones rápidas como nota:\n%s", content)
		}
	})
}
