//go:build integration

package operations_test

import (
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/payments"
)

type adjustmentResult struct {
	AdjustmentID string           `json:"adjustment_id"`
	Finance      payments.Finance `json:"finance"`
	Closed       bool             `json:"closed"`
}

type refundResult struct {
	RefundIDs []string            `json:"refund_ids"`
	Plan      payments.RefundPlan `json:"plan"`
	Finance   payments.Finance    `json:"finance"`
	Reopened  bool                `json:"reopened"`
}

// post envía una operación financiera con identificador nuevo desde la terminal indicada.
func (fixture *paymentsFixture) post(t *testing.T, terminal testTerminal, accountID, action string, body map[string]any, status int) []byte {
	t.Helper()
	if _, found := body["operation_id"]; !found {
		body["operation_id"] = randomID()
	}
	return fixture.call(t, terminal, "POST", "payments/accounts/"+accountID+"/"+action, body, status)
}

func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	return decodeResponse[struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}](t, body).Error.Code
}

func (fixture *paymentsFixture) roleWithLimit(t *testing.T, name, limit string, permissions ...string) string {
	t.Helper()
	return decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "roles", map[string]any{"name": name, "permissions": permissions, "discount_limit": limit}, 200)).ID
}

// staffWithRole crea un usuario en una terminal propia con el rol indicado y la caja principal.
func (fixture *paymentsFixture) staffWithRole(t *testing.T, username, roleID string) testTerminal {
	t.Helper()
	password := "Clave temporal de " + username
	fixture.adminCall(t, "POST", "users", map[string]any{
		"username": username, "display_name": "Prueba " + username, "role_id": roleID, "active": true, "password": password,
	}, 200)
	invitation := decodeResponse[struct {
		Code string `json:"code"`
	}](t, fixture.adminCall(t, "POST", "devices/invitations", nil, 200))
	device := decodeResponse[struct {
		ID    string `json:"device_id"`
		Token string `json:"device_token"`
	}](t, fixture.call(t, testTerminal{}, "POST", "devices/enroll", map[string]any{"code": invitation.Code, "name": "Terminal " + username}, 200))
	fixture.adminCall(t, "PATCH", "devices/"+device.ID, map[string]any{"name": "Terminal " + username, "register_id": fixture.registers["primary"], "keyboard_mode": "auto"}, 200)
	terminal := testTerminal{deviceID: device.ID, deviceToken: device.Token}
	terminal.sessionToken = decodeResponse[struct {
		Token string `json:"session_token"`
	}](t, fixture.call(t, terminal, "POST", "auth/login", map[string]string{"username": username, "password": password}, 200)).Token
	return terminal
}

func testPaymentAdjustments(t *testing.T, fixture *paymentsFixture) {
	cash := fixture.methods["cash"].ID
	debit := fixture.methods["debit"].ID
	cashierRole := fixture.roleWithLimit(t, "Cajero con límite", "10",
		"accounts.read", "payments.read", "payments.collect", "payments.collect_partial", "payments.collect_selection",
		"payments.discounts.apply", "payments.charges.create")
	supervisorRole := fixture.roleWithLimit(t, "Supervisor de cobro", "30",
		"accounts.read", "payments.read", "payments.discounts.authorize", "payments.refunds.authorize", "payments.corrections")
	cashier := fixture.staffWithRole(t, "cajera_limite", cashierRole)
	supervisor := fixture.staffWithRole(t, "supervisora", supervisorRole)
	fixture.staffWithRole(t, "supervisor_bloqueo", supervisorRole)
	authorization := map[string]string{"username": "supervisora", "password": "Clave temporal de supervisora"}

	t.Run("21_22_A5_descuentos_por_partida_y_general_reducen_base_e_iva", func(t *testing.T) {
		bill := fixture.account(t, "Descuentos", line(fixture.burger, 2), line(fixture.soda, 1))
		finance := fixture.finance(t, bill.Account.ID)
		burger := lineOf(t, finance, fixture.burger)
		preview := decodeResponse[payments.DiscountPlan](t, fixture.post(t, fixture.admin, bill.Account.ID, "discounts/preview", map[string]any{
			"scope": "items", "method": "percent", "value": "10", "selections": []map[string]any{{"version_id": burger.VersionID, "quantity": 1}},
		}, 200))
		if preview.Amount != "12.00" || preview.Base != "120.00" || preview.Percent != "10.00" {
			t.Fatalf("El descuento por partida no se calculó sobre la unidad elegida: %+v", preview)
		}
		applied := decodeResponse[adjustmentResult](t, fixture.post(t, fixture.admin, bill.Account.ID, "discounts", map[string]any{
			"scope": "items", "method": "percent", "value": "10", "reason": "Cliente frecuente",
			"selections":       []map[string]any{{"version_id": burger.VersionID, "quantity": 1}},
			"expected_balance": finance.Balance,
		}, 200))
		if applied.Finance.ItemDiscounts != "12.00" || applied.Finance.Total != "253.00" {
			t.Fatal("El descuento por partida no redujo el total")
		}
		general := decodeResponse[adjustmentResult](t, fixture.post(t, fixture.admin, bill.Account.ID, "discounts", map[string]any{
			"scope": "account", "method": "amount", "value": "21", "reason": "Ajuste comercial", "expected_balance": applied.Finance.Balance,
		}, 200))
		if general.Finance.Total != "232.00" || general.Finance.AccountDiscounts != "21.00" {
			t.Fatal("El descuento general no redujo el total")
		}
		if general.Finance.Tax[0].Base != "200.00" || general.Finance.Tax[0].Tax != "32.00" {
			t.Fatalf("El descuento no redujo base e IVA: %+v", general.Finance.Tax)
		}
		// La unidad con descuento manual no se transfiere ni cancela hasta cancelar ese descuento.
		current := fixture.refresh(t, bill.Account.ID)
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/cancel", map[string]any{
			"revision": current.Account.Revision, "version_id": burger.VersionID, "quantity": 2, "reason": "Unidad con descuento",
		}, 409)
	})

	t.Run("23_24_cortesia_total_sin_pago_ficticio", func(t *testing.T) {
		bill := fixture.account(t, "Cortesía", line(fixture.soda, 2))
		before := fixture.countRows(t, "SELECT count(*) FROM app.payments")
		result := decodeResponse[adjustmentResult](t, fixture.post(t, fixture.admin, bill.Account.ID, "discounts", map[string]any{
			"scope": "account", "method": "percent", "value": "100", "courtesy": true, "reason": "Cortesía de la casa",
			"expected_balance": fixture.finance(t, bill.Account.ID).Balance,
		}, 200))
		if !result.Closed || result.Finance.PaymentState != "paid" || result.Finance.Total != "0.00" || result.Finance.ItemsTotal != "50.00" {
			t.Fatal("La cortesía no liquidó la cuenta conservando los productos vendidos")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.payments") != before {
			t.Fatal("La cortesía registró un pago ficticio")
		}
		fixture.post(t, fixture.admin, fixture.account(t, "Cortesía inválida", line(fixture.soda, 1)).Account.ID, "discounts", map[string]any{
			"scope": "account", "method": "percent", "value": "50", "courtesy": true, "reason": "No es total", "expected_balance": "25.00",
		}, 400)
	})

	t.Run("25_26_27_S4_S5_limites_y_autorizacion_puntual_de_supervisor", func(t *testing.T) {
		bill := fixture.account(t, "Límites", line(fixture.burger, 1))
		body := func(value string) map[string]any {
			return map[string]any{
				"scope": "account", "method": "percent", "value": value, "reason": "Prueba de límites",
				"expected_balance": fixture.finance(t, bill.Account.ID).Balance,
			}
		}
		required := fixture.post(t, cashier, bill.Account.ID, "discounts", body("15"), 403)
		if errorCode(t, required) != "AUTHORIZATION_REQUIRED" || fixture.countRows(t, "SELECT count(*) FROM app.account_adjustments WHERE account_id=$1", bill.Account.ID) != 0 {
			t.Fatal("Un descuento sobre el límite no exigió autorización o dejó escrituras")
		}
		withSelf := body("15")
		withSelf["authorization"] = map[string]string{"username": "cajera_limite", "password": "Clave temporal de cajera_limite"}
		if errorCode(t, fixture.post(t, cashier, bill.Account.ID, "discounts", withSelf, 403)) != "SELF_AUTHORIZATION" {
			t.Fatal("Un usuario se autorizó a sí mismo")
		}
		// Una autorización incompleta (como el reintento incierto que ya no lleva la contraseña)
		// responde el mismo 403 que una autorización ausente y no escribe nada.
		incomplete := body("15")
		incomplete["authorization"] = map[string]string{"username": "supervisora", "password": ""}
		if errorCode(t, fixture.post(t, cashier, bill.Account.ID, "discounts", incomplete, 403)) != "AUTHORIZATION_REQUIRED" {
			t.Fatal("Una autorización sin contraseña no se trató como autorización requerida")
		}
		courtesy := fixture.post(t, cashier, bill.Account.ID, "discounts", map[string]any{
			"scope": "account", "method": "percent", "value": "100", "courtesy": true, "reason": "Cortesía sin permiso",
			"expected_balance": fixture.finance(t, bill.Account.ID).Balance,
		}, 403)
		if errorCode(t, courtesy) != "AUTHORIZATION_REQUIRED" || !strings.Contains(string(courtesy), "cortesía") {
			t.Fatalf("La cortesía sin permiso no explicó la autorización requerida: %s", courtesy)
		}
		tooHigh := body("40")
		tooHigh["authorization"] = authorization
		if errorCode(t, fixture.post(t, cashier, bill.Account.ID, "discounts", tooHigh, 403)) != "AUTHORIZATION_INSUFFICIENT" {
			t.Fatal("Un supervisor autorizó por encima de su propio límite")
		}
		authorized := body("25")
		authorized["authorization"] = authorization
		result := decodeResponse[adjustmentResult](t, fixture.post(t, cashier, bill.Account.ID, "discounts", authorized, 200))
		var authorizedBy string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT u.username FROM app.account_adjustments j JOIN app.users u ON u.id=j.authorized_by
            WHERE j.id=$1`, result.AdjustmentID).Scan(&authorizedBy); err != nil || authorizedBy != "supervisora" {
			t.Fatal("No se registró quién autorizó el descuento")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.payment_authorizations WHERE scope='discount' AND account_id=$1", bill.Account.ID) != 1 {
			t.Fatal("La autorización no quedó ligada a la operación")
		}
		// La autorización no inicia sesión: la sesión del supervisor en su terminal sigue vigente.
		fixture.call(t, supervisor, "GET", "auth/me", nil, 200)
		within := decodeResponse[adjustmentResult](t, fixture.post(t, cashier, fixture.account(t, "Dentro del límite", line(fixture.burger, 1)).Account.ID, "discounts", map[string]any{
			"scope": "account", "method": "percent", "value": "10", "reason": "Dentro del límite", "expected_balance": "120.00",
		}, 200))
		if within.Finance.Total != "108.00" {
			t.Fatal("Un descuento dentro del límite exigió una autorización innecesaria")
		}
		// S4: cinco contraseñas incorrectas bloquean nuevas autorizaciones de ese supervisor en la terminal.
		throttled := body("15")
		for attempt := 0; attempt < 5; attempt++ {
			throttled["operation_id"] = randomID()
			throttled["authorization"] = map[string]string{"username": "supervisor_bloqueo", "password": "incorrecta"}
			fixture.post(t, cashier, bill.Account.ID, "discounts", throttled, 401)
		}
		throttled["operation_id"] = randomID()
		throttled["authorization"] = map[string]string{"username": "supervisor_bloqueo", "password": "Clave temporal de supervisor_bloqueo"}
		fixture.post(t, cashier, bill.Account.ID, "discounts", throttled, 429)
		var audited int
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM app.audit_events WHERE action='authorization.failed'").Scan(&audited); err != nil || audited < 6 {
			t.Fatal("Los intentos fallidos de autorización no quedaron auditados")
		}
		var history string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(jsonb_agg(to_jsonb(a)),'[]')::text FROM app.audit_events a").Scan(&history); err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"Clave temporal de supervisora", "Clave temporal de supervisor_bloqueo", "incorrecta"} {
			if strings.Contains(history, secret) {
				t.Fatal("La auditoría contiene una contraseña de autorización")
			}
		}
	})

	t.Run("S3_limite_de_descuento_sujeto_a_delegacion_segura", func(t *testing.T) {
		managerRole := fixture.roleWithLimit(t, "Gestor con límite", "10", "roles.read", "roles.manage", "permissions.read", "permissions.manage", "users.read", "users.manage")
		manager := fixture.staffWithRole(t, "gestor_limites", managerRole)
		fixture.call(t, manager, "POST", "roles", map[string]any{"name": "Límite mayor", "permissions": []string{"users.read"}, "discount_limit": "20"}, 403)
		lower := decodeResponse[struct {
			ID string `json:"id"`
		}](t, fixture.call(t, manager, "POST", "roles", map[string]any{"name": "Límite igual", "permissions": []string{"users.read"}, "discount_limit": "10"}, 200))
		fixture.call(t, manager, "PATCH", "roles/"+lower.ID, map[string]any{"name": "Límite igual", "permissions": []string{"users.read"}, "discount_limit": "50"}, 403)
		fixture.call(t, manager, "PATCH", "roles/"+managerRole, map[string]any{"name": "Gestor con límite", "permissions": []string{"users.read"}, "discount_limit": "5"}, 403)
		fixture.call(t, manager, "POST", "users", map[string]any{
			"username": "con_supervision", "display_name": "Con límite ajeno", "role_id": supervisorRole, "active": true, "password": "Clave temporal larga",
		}, 403)
		fixture.adminCall(t, "POST", "roles", map[string]any{"name": "Límite inválido", "permissions": []string{}, "discount_limit": "101"}, 400)
	})

	t.Run("28_29_30_cargos_generales_independientes_y_auditados", func(t *testing.T) {
		bill := fixture.account(t, "Cargos", line(fixture.soda, 1))
		fixture.post(t, cashier, bill.Account.ID, "charges", map[string]any{"concept": "Servicio", "amount": "-5", "expected_balance": "25.00"}, 400)
		charge := decodeResponse[adjustmentResult](t, fixture.post(t, cashier, bill.Account.ID, "charges", map[string]any{
			"concept": "Servicio a domicilio", "amount": "15", "reason": "Entrega especial", "expected_balance": "25.00",
		}, 200))
		if charge.Finance.Charges != "15.00" || charge.Finance.Total != "40.00" || charge.Finance.ItemsTotal != "25.00" {
			t.Fatal("El cargo general no se reflejó en el total o alteró los productos")
		}
		if *charge.Finance.Adjustments[0].TaxRate != 16 {
			t.Fatal("El cargo no conservó la tasa de IVA vigente")
		}
		void := map[string]any{"adjustment_id": charge.AdjustmentID, "reason": "Cargo capturado por error"}
		if errorCode(t, fixture.post(t, cashier, bill.Account.ID, "adjustments/void", void, 403)) != "AUTHORIZATION_REQUIRED" {
			t.Fatal("Un usuario sin permiso canceló un cargo")
		}
		voided := decodeResponse[adjustmentResult](t, fixture.post(t, fixture.admin, bill.Account.ID, "adjustments/void", map[string]any{
			"adjustment_id": charge.AdjustmentID, "reason": "Cargo capturado por error",
		}, 200))
		if voided.Finance.Total != "25.00" || !voided.Finance.Adjustments[0].Voided {
			t.Fatal("La cancelación del cargo no conservó el registro original")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action IN ('payments.charge_added','payments.adjustment_voided')") < 2 {
			t.Fatal("Los cambios del cargo no se auditaron")
		}
	})

	t.Run("34_37_A6_devolucion_total_con_otro_metodo_y_tasa_original", func(t *testing.T) {
		bill := fixture.account(t, "Devolución total", line(fixture.burger, 1))
		paid := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": debit, "mode": "total"}, 200))
		settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
		fixture.adminCall(t, "PUT", "payments/tax", map[string]any{"operation_id": randomID(), "revision": settings.Revision, "tax_rate": "0"}, 200)
		result := decodeResponse[refundResult](t, fixture.post(t, fixture.admin, bill.Account.ID, "refunds", map[string]any{
			"mode": "total", "method_id": cash, "reason": "Pedido devuelto por el cliente",
		}, 200))
		if !result.Reopened || result.Finance.Paid != "0.00" || result.Finance.Balance != "120.00" || result.Finance.State != "open" {
			t.Fatal("La devolución total no reabrió la cuenta con su saldo")
		}
		refund := result.Finance.Refunds[0]
		if refund.MethodName != "Efectivo" || refund.Tax[0].Rate != 16 || refund.Tax[0].Tax != "16.55" {
			t.Fatalf("La devolución no usó el método real o la tasa original: %+v", refund)
		}
		var originalAmount, movement string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT amount::text FROM app.payments WHERE id=$1", paid.Payment.ID).Scan(&originalAmount); err != nil || originalAmount != "120.00" {
			t.Fatal("La devolución modificó el pago original")
		}
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT net_amount::text FROM app.cash_movements WHERE refund_id=$1", refund.ID).Scan(&movement); err != nil || movement != "-120.00" {
			t.Fatal("La devolución no generó su movimiento compensatorio en caja")
		}
		latest := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
		fixture.adminCall(t, "PUT", "payments/tax", map[string]any{"operation_id": randomID(), "revision": latest.Revision, "tax_rate": "16"}, 200)
		fixture.post(t, fixture.admin, bill.Account.ID, "refunds", map[string]any{"mode": "total", "method_id": cash, "reason": "Segunda devolución"}, 409)
	})

	t.Run("35_36_38_P6_devolucion_parcial_por_productos_y_sin_duplicados", func(t *testing.T) {
		bill := fixture.account(t, "Devolución parcial", line(fixture.burger, 1), line(fixture.soda, 2))
		finance := fixture.finance(t, bill.Account.ID)
		soda := lineOf(t, finance, fixture.soda)
		general := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "partial", "amount": "100"}, 200))
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": debit, "mode": "total", "selections": []map[string]any{{"version_id": soda.VersionID, "quantity": 2}},
		}, 200)
		partial := decodeResponse[refundResult](t, fixture.post(t, fixture.admin, bill.Account.ID, "refunds", map[string]any{
			"mode": "payment", "payment_id": general.Payment.ID, "amount": "30", "method_id": cash, "reason": "Ajuste por queja",
		}, 200))
		if partial.Finance.Paid != "120.00" || partial.Finance.Balance != "50.00" {
			t.Fatal("La devolución parcial no actualizó el saldo")
		}
		fixture.post(t, fixture.admin, bill.Account.ID, "refunds", map[string]any{
			"mode": "payment", "payment_id": general.Payment.ID, "amount": "80", "method_id": cash, "reason": "Excede lo devolvible",
		}, 400)
		fixture.post(t, fixture.admin, bill.Account.ID, "refunds/preview", map[string]any{
			"mode": "payment", "payment_id": general.Payment.ID, "method_id": cash, "reason": "Sin importe",
		}, 400)
		items := decodeResponse[refundResult](t, fixture.post(t, fixture.admin, bill.Account.ID, "refunds", map[string]any{
			"mode": "items", "selections": []map[string]any{{"version_id": soda.VersionID, "quantity": 1}}, "method_id": cash, "reason": "Refresco derramado",
		}, 200))
		if items.Plan.Amount != "25.00" || lineOf(t, items.Finance, fixture.soda).SettledQuantity != 1 {
			t.Fatal("La devolución por productos no liberó exactamente la unidad elegida")
		}
		fixture.post(t, fixture.admin, bill.Account.ID, "refunds", map[string]any{
			"mode": "items", "selections": []map[string]any{{"version_id": soda.VersionID, "quantity": 2}}, "method_id": cash, "reason": "Duplicada",
		}, 400)
		// P6: la unidad devuelta vuelve a ser cancelable con las reglas normales.
		current := fixture.refresh(t, bill.Account.ID)
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/cancel", map[string]any{
			"revision": current.Account.Revision, "version_id": soda.VersionID, "quantity": 1, "reason": "Refresco devuelto",
		}, 200)
	})

	t.Run("39_40_devoluciones_con_permiso_directo_umbral_y_supervisor", func(t *testing.T) {
		bill := fixture.account(t, "Autorización de devolución", line(fixture.burger, 1))
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total"}, 200)
		body := map[string]any{"mode": "total", "method_id": cash, "reason": "Devolución autorizada"}
		if errorCode(t, fixture.post(t, cashier, bill.Account.ID, "refunds", body, 403)) != "AUTHORIZATION_REQUIRED" {
			t.Fatal("Una devolución sin permiso no pidió autorización")
		}
		settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
		fixture.adminCall(t, "PUT", "payments/settings", map[string]any{
			"operation_id": randomID(), "revision": settings.Revision, "shared_shifts": true, "print_receipt_default": true,
			"receipt_printer_id": nil, "refund_authorization_threshold": "50",
		}, 200)
		if errorCode(t, fixture.post(t, fixture.admin, bill.Account.ID, "refunds", map[string]any{"mode": "total", "method_id": cash, "reason": "Sobre el umbral"}, 403)) != "AUTHORIZATION_REQUIRED" {
			t.Fatal("El umbral configurado no exigió autorización")
		}
		body["operation_id"] = randomID()
		body["authorization"] = authorization
		result := decodeResponse[refundResult](t, fixture.post(t, cashier, bill.Account.ID, "refunds", body, 200))
		if *result.Finance.Refunds[0].AuthorizedBy != "Prueba supervisora" {
			t.Fatal("La devolución no registró al supervisor que autorizó")
		}
		latest := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
		fixture.adminCall(t, "PUT", "payments/settings", map[string]any{
			"operation_id": randomID(), "revision": latest.Revision, "shared_shifts": true, "print_receipt_default": true,
			"receipt_printer_id": nil, "refund_authorization_threshold": nil,
		}, 200)
	})

	t.Run("41_42_correccion_de_metodo_en_turno_abierto", func(t *testing.T) {
		bill := fixture.account(t, "Corrección", line(fixture.soda, 2))
		wrong := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "partial", "amount": "30", "received": "50"}, 200))
		fixture.post(t, cashier, bill.Account.ID, "corrections", map[string]any{
			"payment_id": wrong.Payment.ID, "method_id": debit, "reason": "Era tarjeta",
		}, 403)
		shiftID := wrong.Payment.ShiftID
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.cash_shifts SET state='closed',closed_at=now() WHERE id=$1", shiftID); err != nil {
			t.Fatal(err)
		}
		if errorCode(t, fixture.post(t, fixture.admin, bill.Account.ID, "corrections", map[string]any{
			"payment_id": wrong.Payment.ID, "method_id": debit, "reason": "Turno cerrado",
		}, 409)) != "SHIFT_CLOSED" {
			t.Fatal("Se corrigió un pago de un turno cerrado")
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.cash_shifts SET state='open',closed_at=NULL WHERE id=$1", shiftID); err != nil {
			t.Fatal(err)
		}
		corrected := decodeResponse[struct {
			PaymentID string           `json:"payment_id"`
			Finance   payments.Finance `json:"finance"`
		}](t, fixture.post(t, fixture.admin, bill.Account.ID, "corrections", map[string]any{
			"payment_id": wrong.Payment.ID, "method_id": debit, "reference": "AUT-CORR", "reason": "Era tarjeta, no efectivo",
		}, 200))
		if corrected.Finance.Paid != "30.00" || corrected.Finance.Balance != "20.00" || len(corrected.Finance.Payments) != 2 {
			t.Fatal("La corrección duplicó o perdió el ingreso")
		}
		var cashNet, debitNet string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT coalesce(sum(net_amount) FILTER (WHERE is_cash),0)::text,
            coalesce(sum(net_amount) FILTER (WHERE NOT is_cash),0)::text FROM app.cash_movements WHERE account_id=$1`, bill.Account.ID).Scan(&cashNet, &debitNet); err != nil {
			t.Fatal(err)
		}
		if cashNet != "0.00" || debitNet != "30.00" {
			t.Fatalf("Los movimientos de la corrección no compensan el método original: efectivo %s, tarjeta %s", cashNet, debitNet)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.payments WHERE id=$1 AND method_name='Efectivo'", wrong.Payment.ID) != 1 {
			t.Fatal("La corrección modificó el registro original")
		}
		fixture.post(t, fixture.admin, bill.Account.ID, "corrections", map[string]any{
			"payment_id": wrong.Payment.ID, "method_id": fixture.methods["transfer"].ID, "reason": "Segunda corrección",
		}, 409)
	})
}
