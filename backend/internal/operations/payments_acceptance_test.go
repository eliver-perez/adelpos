//go:build integration

package operations_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// paymentsFixture reúne los datos de la aceptación financiera de la etapa 6.
type paymentsFixture struct {
	*restaurantFixture
	methods   map[string]payments.Method
	burger    catalog.Product
	soda      catalog.Product
	cashier   testTerminal
	observer  testTerminal
	registers map[string]string
}

func (fixture *paymentsFixture) finance(t *testing.T, accountID string) payments.Finance {
	t.Helper()
	return decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+accountID, nil, 200))
}

// pay registra un pago con un identificador nuevo y el saldo vigente como saldo esperado.
func (fixture *paymentsFixture) pay(t *testing.T, terminal testTerminal, accountID string, body map[string]any, status int) []byte {
	t.Helper()
	if _, found := body["operation_id"]; !found {
		body["operation_id"] = randomID()
	}
	if _, found := body["expected_balance"]; !found {
		body["expected_balance"] = fixture.finance(t, accountID).Balance
	}
	return fixture.call(t, terminal, "POST", "payments/accounts/"+accountID+"/payments", body, status)
}

type paymentResult struct {
	Payment   payments.PaymentRecord `json:"payment"`
	Finance   payments.Finance       `json:"finance"`
	Closed    bool                   `json:"closed"`
	Delivered bool                   `json:"delivered"`
}

func (fixture *paymentsFixture) account(t *testing.T, name string, lines ...catalog.LineRequest) operations.Bill {
	t.Helper()
	return fixture.capture(t, newAccount(t, fixture.restaurantFixture, name), lines)
}

func (fixture *paymentsFixture) countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func amountOf(t *testing.T, value string) decimal.Decimal {
	t.Helper()
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		t.Fatalf("Importe inválido en la respuesta: %q", value)
	}
	return parsed
}

// newCashier crea un usuario con permisos de cobro en una terminal propia asociada a la caja indicada.
func newCashier(t *testing.T, fixture *paymentsFixture, username string, permissions []string, registerID string) testTerminal {
	t.Helper()
	role := decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "roles", map[string]any{"name": "Rol " + username, "permissions": permissions}, 200))
	password := "Clave temporal de " + username
	fixture.adminCall(t, "POST", "users", map[string]any{
		"username": username, "display_name": "Prueba " + username, "role_id": role.ID, "active": true, "password": password,
	}, 200)
	invitation := decodeResponse[struct {
		Code string `json:"code"`
	}](t, fixture.adminCall(t, "POST", "devices/invitations", nil, 200))
	device := decodeResponse[struct {
		ID    string `json:"device_id"`
		Token string `json:"device_token"`
	}](t, fixture.call(t, testTerminal{}, "POST", "devices/enroll", map[string]any{"code": invitation.Code, "name": "Terminal " + username}, 200))
	if registerID != "" {
		fixture.adminCall(t, "PATCH", "devices/"+device.ID, map[string]any{"name": "Terminal " + username, "register_id": registerID, "keyboard_mode": "auto"}, 200)
	}
	terminal := testTerminal{deviceID: device.ID, deviceToken: device.Token}
	login := decodeResponse[struct {
		Token string `json:"session_token"`
	}](t, fixture.call(t, terminal, "POST", "auth/login", map[string]string{"username": username, "password": password}, 200))
	terminal.sessionToken = login.Token
	return terminal
}

// status envía una petición desde cualquier terminal y devuelve solo su código HTTP.
func (fixture *paymentsFixture) status(terminal testTerminal, method, route string, encoded []byte) int {
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:53000"
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response.Code
}

// lineOf busca la partida financiera de un producto por nombre: el orden no es significativo.
func lineOf(t *testing.T, finance payments.Finance, product catalog.Product) payments.FinanceLine {
	t.Helper()
	for _, item := range finance.Lines {
		if item.Name == product.Name {
			return item
		}
	}
	t.Fatalf("La cuenta no contiene %s", product.Name)
	return payments.FinanceLine{}
}

func line(product catalog.Product, quantity int) catalog.LineRequest {
	return catalog.LineRequest{ProductID: product.ID, Quantity: quantity}
}

func testPayments(t *testing.T, base *restaurantFixture) {
	fixture := &paymentsFixture{restaurantFixture: base, methods: map[string]payments.Method{}, registers: map[string]string{}}
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil {
			fixture.methods[*method.Code] = method
		}
	}
	if len(fixture.methods) != 5 {
		t.Fatal("No se sembraron los cinco métodos de pago iniciales")
	}
	fixture.burger = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Hamburguesa de cobro", Active: true}, CategoryID: fixture.category.ID, Code: "COBRO_HAMB", Kind: "product", BasePrice: "120",
	}, 200))
	fixture.soda = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Refresco de cobro", Active: true}, CategoryID: fixture.category.ID, Code: "COBRO_REFR", Kind: "product", BasePrice: "25",
	}, 200))
	var registerID string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT register_id FROM app.devices WHERE id=$1", fixture.admin.deviceID).Scan(&registerID); err != nil {
		t.Fatal(err)
	}
	fixture.registers["primary"] = registerID
	cash := fixture.methods["cash"].ID
	debit := fixture.methods["debit"].ID

	t.Run("43_45_46_turno_minimo_requerido_y_unico", func(t *testing.T) {
		bill := fixture.account(t, "Sin turno", line(fixture.soda, 1))
		problem := fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total"}, 409)
		if !json.Valid(problem) || fixture.countRows(t, "SELECT count(*) FROM app.payments") != 0 {
			t.Fatal("Se registró un cobro sin turno abierto")
		}
		fixture.adminCall(t, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "-1"}, 400)
		shift := decodeResponse[payments.Shift](t, fixture.adminCall(t, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0"}, 200))
		if shift.OpeningFloat != "0.00" || shift.State != "open" || shift.RegisterID != registerID {
			t.Fatal("No se abrió el turno con fondo inicial de $0")
		}
		fixture.adminCall(t, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "500"}, 409)
		if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.cash_shifts(id,register_id,opening_float,opened_by,opened_device_id,operation_id)
            SELECT gen_random_uuid(),register_id,0,opened_by,opened_device_id,gen_random_uuid() FROM app.cash_shifts LIMIT 1`); err == nil {
			t.Fatal("PostgreSQL permitió dos turnos abiertos en una caja")
		}
		fixture.registers["shift"] = shift.ID
	})

	t.Run("01_05_47_cobro_total_en_efectivo_con_cambio_y_cierre", func(t *testing.T) {
		bill := fixture.account(t, "Total efectivo", line(fixture.burger, 1), line(fixture.soda, 2))
		if fixture.finance(t, bill.Account.ID).Total != "170.00" {
			t.Fatal("El total financiero no reutilizó los importes confirmados")
		}
		result := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "received": "200",
		}, 200))
		if result.Payment.Amount != "170.00" || result.Payment.Change != "30.00" || *result.Payment.Received != "200.00" {
			t.Fatalf("Cambio incorrecto: %+v", result.Payment)
		}
		if !result.Closed || result.Finance.Balance != "0.00" || result.Finance.PaymentState != "paid" {
			t.Fatal("La cuenta liquidada de salón no se cerró")
		}
		var sale, net, received, change string
		var shiftID, actor string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT sale_amount::text,net_amount::text,cash_received::text,change_given::text,shift_id,actor_id
            FROM app.cash_movements WHERE payment_id=$1`, result.Payment.ID).Scan(&sale, &net, &received, &change, &shiftID, &actor); err != nil {
			t.Fatal(err)
		}
		if sale != "170.00" || net != "170.00" || received != "200.00" || change != "30.00" || shiftID != fixture.registers["shift"] {
			t.Fatal("El movimiento de caja no separó efectivo recibido, cambio e ingreso neto")
		}
		account := decodeResponse[operations.Bill](t, fixture.adminCall(t, "GET", "restaurant/accounts/"+bill.Account.ID, nil, 200))
		if account.Account.State != "closed" || account.Account.PaymentState != "paid" || account.Account.Finance.Balance != "0.00" {
			t.Fatal("La cuenta del restaurante no refleja el pago real")
		}
	})

	t.Run("02_07_08_tarjeta_referencias_opcional_y_obligatoria", func(t *testing.T) {
		bill := fixture.account(t, "Tarjeta", line(fixture.burger, 1))
		result := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": debit, "mode": "total", "reference": "AUT-123456",
		}, 200))
		if result.Payment.Reference != "AUT-123456" || result.Payment.Change != "0.00" || result.Payment.Received != nil {
			t.Fatal("El pago con tarjeta no conservó su referencia o generó cambio")
		}
		voucher := decodeResponse[payments.Method](t, fixture.adminCall(t, "POST", "payments/methods", map[string]any{
			"operation_id": randomID(), "name": "Vale con folio", "icon": "voucher", "active": true, "sort_order": 60,
			"is_cash": false, "reference_mode": "required", "allows_tip": false, "allows_refund": false,
		}, 200))
		other := fixture.account(t, "Referencia obligatoria", line(fixture.soda, 1))
		fixture.pay(t, fixture.admin, other.Account.ID, map[string]any{"method_id": voucher.ID, "mode": "total"}, 400)
		fixture.pay(t, fixture.admin, other.Account.ID, map[string]any{"method_id": voucher.ID, "mode": "total", "reference": "VALE-9"}, 200)
		fixture.adminCall(t, "POST", "payments/methods", map[string]any{
			"operation_id": randomID(), "name": "Efectivo con referencia", "icon": "cash", "active": true, "sort_order": 70,
			"is_cash": true, "reference_mode": "required", "allows_tip": true, "allows_refund": true,
		}, 400)
	})

	t.Run("03_04_06_parcial_y_mixto_sucesivo_con_cambio_del_abono", func(t *testing.T) {
		bill := fixture.account(t, "Mixto", line(fixture.burger, 1), line(fixture.soda, 2))
		first := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": cash, "mode": "partial", "amount": "70", "received": "100",
		}, 200))
		if first.Payment.Amount != "70.00" || first.Payment.Change != "30.00" || first.Finance.Balance != "100.00" || first.Closed {
			t.Fatalf("El abono en efectivo confundió recibido con abono: %+v", first.Payment)
		}
		if first.Finance.PaymentState != "partial" {
			t.Fatal("Una cuenta con saldo pendiente apareció liquidada")
		}
		second := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": debit, "mode": "partial", "amount": "60",
		}, 200))
		if second.Finance.Balance != "40.00" {
			t.Fatal("El segundo abono no actualizó el saldo")
		}
		third := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": fixture.methods["transfer"].ID, "mode": "total", "reference": "SPEI-77",
		}, 200))
		if third.Finance.Balance != "0.00" || len(third.Finance.Payments) != 3 || !third.Closed {
			t.Fatal("Los pagos sucesivos no se conservaron por separado o no liquidaron la cuenta")
		}
		if fixture.countRows(t, "SELECT count(DISTINCT method_id) FROM app.payments WHERE account_id=$1", bill.Account.ID) != 3 {
			t.Fatal("El pago mixto perdió la trazabilidad por método")
		}
	})

	t.Run("09_10_importes_invalidos_y_sobrepagos", func(t *testing.T) {
		bill := fixture.account(t, "Validaciones", line(fixture.soda, 2))
		for _, amount := range []string{"-1", "1.234", "abc", "0", "1..2", "1e2", "60"} {
			fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "partial", "amount": amount}, 400)
		}
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received": "10"}, 400)
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": debit, "mode": "total", "received": "60"}, 400)
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": debit, "mode": "total", "tip": "-5"}, 400)
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": fixture.methods["platforms"].ID, "mode": "total", "tip": "5"}, 400)
		if fixture.countRows(t, "SELECT count(*) FROM app.payments WHERE account_id=$1", bill.Account.ID) != 0 {
			t.Fatal("Un importe inválido produjo un pago")
		}
	})

	t.Run("31_32_33_propinas_separadas_de_la_venta", func(t *testing.T) {
		bill := fixture.account(t, "Propinas", line(fixture.soda, 2))
		first := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": cash, "mode": "partial", "amount": "20", "tip": "5", "received": "30",
		}, 200))
		if first.Payment.Change != "5.00" || first.Finance.Balance != "30.00" || first.Finance.Tips != "5.00" {
			t.Fatal("La propina en efectivo se mezcló con la venta")
		}
		second := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": debit, "mode": "total", "tip": "10",
		}, 200))
		if second.Finance.Paid != "50.00" || second.Finance.Tips != "15.00" || second.Finance.Total != "50.00" {
			t.Fatal("La propina electrónica alteró el total de la venta")
		}
		var tipNet string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT sum(tip_amount)::text FROM app.cash_movements WHERE account_id=$1", bill.Account.ID).Scan(&tipNet); err != nil || tipNet != "15.00" {
			t.Fatal("Los movimientos no conservaron las propinas por separado")
		}
		for _, tax := range second.Finance.Tax {
			if amountOf(t, tax.Total).GreaterThan(amountOf(t, second.Finance.Total)) {
				t.Fatal("La propina entró en la base gravable")
			}
		}
	})

	t.Run("11_12_13_18_P1_P2_cobro_por_seleccion_y_bloqueo_de_unidades_pagadas", func(t *testing.T) {
		bill := fixture.account(t, "Selección", line(fixture.burger, 2), line(fixture.soda, 3))
		finance := fixture.finance(t, bill.Account.ID)
		burgerLine, sodaLine := lineOf(t, finance, fixture.burger), lineOf(t, finance, fixture.soda)
		preview := decodeResponse[payments.PaymentPlan](t, fixture.adminCall(t, "POST", "payments/accounts/"+bill.Account.ID+"/preview", map[string]any{
			"method_id": cash, "mode": "total",
			"selections": []map[string]any{{"version_id": burgerLine.VersionID, "quantity": 1}, {"version_id": sodaLine.VersionID, "quantity": 1}},
		}, 200))
		if *preview.SelectionTotal != "145.00" || preview.Amount != "145.00" || preview.BalanceAfter != "170.00" {
			t.Fatalf("La selección no calculó solo los productos elegidos: %+v", preview)
		}
		result := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": cash, "mode": "total",
			"selections": []map[string]any{{"version_id": burgerLine.VersionID, "quantity": 1}, {"version_id": sodaLine.VersionID, "quantity": 1}},
		}, 200))
		if result.Payment.Kind != "selection" || len(result.Payment.UnitIDs) != 2 || result.Finance.Balance != "170.00" {
			t.Fatal("El pago por selección no quedó asignado a sus unidades")
		}
		if lineOf(t, result.Finance, fixture.burger).SettledQuantity != 1 || lineOf(t, result.Finance, fixture.soda).SettledQuantity != 1 {
			t.Fatal("No se identificaron las cantidades ya pagadas")
		}
		// Abono parcial de una selección: la unidad queda con pago parcial.
		partial := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": cash, "mode": "partial", "amount": "10",
			"selections": []map[string]any{{"version_id": sodaLine.VersionID, "quantity": 1}},
		}, 200))
		if lineOf(t, partial.Finance, fixture.soda).PartialQuantity != 1 {
			t.Fatal("La unidad con abono parcial no se identificó")
		}
		// Doble cobro imposible: no quedan suficientes unidades sin liquidar.
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "selections": []map[string]any{{"version_id": burgerLine.VersionID, "quantity": 2}},
		}, 400)
		current := fixture.refresh(t, bill.Account.ID)
		other := newAccount(t, fixture.restaurantFixture, "Destino de transferencia")
		// P1/P2: las dos hamburguesas no pueden transferirse porque una está liquidada.
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/transfer", map[string]any{
			"revision": current.Account.Revision, "target_account_id": other.Account.ID, "target_revision": other.Account.Revision,
			"selections": []map[string]any{{"version_id": burgerLine.VersionID, "quantity": 2}},
		}, 409)
		// Los refrescos tienen una unidad liquidada y otra con pago parcial: cancelar tres es imposible.
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/cancel", map[string]any{
			"revision": current.Account.Revision, "version_id": sodaLine.VersionID, "quantity": 3, "reason": "Prueba de unidades pagadas",
		}, 409)
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/edit-preview", map[string]any{
			"revision": current.Account.Revision, "version_id": burgerLine.VersionID, "reason": "Prueba de edición bloqueada",
			"line": catalog.LineRequest{ProductID: fixture.burger.ID, Quantity: 2, Customization: catalog.Customization{Notes: "sin cebolla"}},
		}, 409)
		// P3: la unidad libre restante sí se transfiere, y el pago permanece en su cuenta.
		moved := decodeResponse[struct {
			Source operations.Bill `json:"source"`
			Target operations.Bill `json:"target"`
		}](t, fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/transfer", map[string]any{
			"revision": current.Account.Revision, "target_account_id": other.Account.ID, "target_revision": other.Account.Revision,
			"selections": []map[string]any{{"version_id": burgerLine.VersionID, "quantity": 1}},
		}, 200))
		if moved.Target.Account.Finance.Total != "120.00" || moved.Source.Account.Finance.Paid != "155.00" {
			t.Fatal("Transferir una unidad libre alteró pagos o importes")
		}
		after := fixture.finance(t, bill.Account.ID)
		if lineOf(t, after, fixture.burger).Quantity != 1 || lineOf(t, after, fixture.burger).SettledQuantity != 1 {
			t.Fatal("La unidad pagada salió de su cuenta")
		}
		// Combo: la unidad comercial conserva su importe completo al cobrarse por selección.
		comboBill := fixture.account(t, "Combo cobrado", catalog.LineRequest{ProductID: fixture.combo.ID, Quantity: 2})
		comboFinance := fixture.finance(t, comboBill.Account.ID)
		unitTotal := comboFinance.Lines[0].Units[0].Total
		comboPay := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, comboBill.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "selections": []map[string]any{{"version_id": comboFinance.Lines[0].VersionID, "quantity": 1}},
		}, 200))
		if comboPay.Payment.Amount != unitTotal || comboPay.Finance.Lines[0].SettledQuantity != 1 {
			t.Fatal("Cobrar un combo parcial no respetó la unidad comercial")
		}
	})

	t.Run("P4_P5_15_16_20_saldos_divisiones_combinaciones_y_productos_nuevos", func(t *testing.T) {
		bill := fixture.account(t, "Abono general", line(fixture.burger, 1), line(fixture.soda, 2))
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "partial", "amount": "140"}, 200)
		current := fixture.refresh(t, bill.Account.ID)
		finance := fixture.finance(t, bill.Account.ID)
		// P4: dividir la hamburguesa dejaría $50 de total frente a $140 pagados.
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/split", map[string]any{
			"revision": current.Account.Revision, "new_account": map[string]string{"name": "Separada"},
			"selections": []map[string]any{{"version_id": lineOf(t, finance, fixture.burger).VersionID, "quantity": 1}},
		}, 409)
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/cancel", map[string]any{
			"revision": current.Account.Revision, "version_id": lineOf(t, finance, fixture.burger).VersionID, "quantity": 1, "reason": "Dejaría saldo negativo",
		}, 409)
		if fixture.countRows(t, "SELECT count(*) FROM app.restaurant_accounts WHERE name='Separada'") != 0 {
			t.Fatal("Una división rechazada dejó una cuenta creada")
		}
		// 15/16: dividir un refresco libre conserva el pago en la cuenta original.
		split := decodeResponse[struct {
			Source operations.Bill `json:"source"`
			Target operations.Bill `json:"target"`
		}](t, fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/split", map[string]any{
			"revision": current.Account.Revision, "new_account": map[string]string{"name": "Refresco separado"},
			"selections": []map[string]any{{"version_id": lineOf(t, finance, fixture.soda).VersionID, "quantity": 1}},
		}, 200))
		if split.Source.Account.Finance.Paid != "140.00" || split.Source.Account.Finance.Balance != "5.00" {
			t.Fatal("La división no conservó el pago en la cuenta original")
		}
		if split.Target.Account.Finance.Paid != "0.00" || split.Target.Account.Finance.Total != "25.00" {
			t.Fatal("La cuenta dividida heredó pagos o perdió importes")
		}
		// P5: la cuenta origen con pagos no se combina; la de destino sí puede tenerlos.
		source := fixture.refresh(t, bill.Account.ID)
		target := fixture.refresh(t, split.Target.Account.ID)
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/merge", map[string]any{
			"revision": source.Account.Revision, "target_account_id": target.Account.ID, "target_revision": target.Account.Revision,
		}, 409)
		merged := decodeResponse[struct {
			Target operations.Bill `json:"target"`
		}](t, fixture.mutate(t, "POST", "restaurant/accounts/"+target.Account.ID+"/merge", map[string]any{
			"revision": target.Account.Revision, "target_account_id": source.Account.ID, "target_revision": source.Account.Revision,
		}, 200))
		if merged.Target.Account.Finance.Total != "170.00" || merged.Target.Account.Finance.Paid != "140.00" {
			t.Fatal("La combinación hacia la cuenta con pagos alteró sus registros")
		}
		// 20: agregar productos tras un abono aumenta el saldo sin tocar pagos anteriores.
		grown := fixture.capture(t, merged.Target, []catalog.LineRequest{line(fixture.soda, 1)})
		if grown.Account.Finance.Total != "195.00" || grown.Account.Finance.Balance != "55.00" || grown.Account.Finance.Paid != "140.00" {
			t.Fatal("Agregar productos después de un pago alteró los pagos")
		}
		// Anular una cuenta con pagos vigentes se rechaza aunque no tuviera productos.
		fixture.mutate(t, "POST", "restaurant/accounts/"+grown.Account.ID+"/void", map[string]any{
			"revision": grown.Account.Revision, "reason": "No debe anularse con pagos",
		}, 400)
	})

	t.Run("61_62_63_idempotencia_y_doble_clic", func(t *testing.T) {
		bill := fixture.account(t, "Idempotencia", line(fixture.soda, 2))
		body := map[string]any{
			"operation_id": randomID(), "method_id": cash, "mode": "partial", "amount": "10",
			"expected_balance": fixture.finance(t, bill.Account.ID).Balance,
		}
		first := decodeResponse[paymentResult](t, fixture.call(t, fixture.admin, "POST", "payments/accounts/"+bill.Account.ID+"/payments", body, 200))
		repeated := decodeResponse[paymentResult](t, fixture.call(t, fixture.admin, "POST", "payments/accounts/"+bill.Account.ID+"/payments", body, 200))
		if first.Payment.ID != repeated.Payment.ID || fixture.countRows(t, "SELECT count(*) FROM app.payments WHERE account_id=$1", bill.Account.ID) != 1 {
			t.Fatal("Un reintento con el mismo identificador duplicó el pago")
		}
		body["amount"] = "11"
		fixture.call(t, fixture.admin, "POST", "payments/accounts/"+bill.Account.ID+"/payments", body, 409)
		// Saldo obsoleto: otra terminal ya cobró y la captura debe actualizarse.
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total", "expected_balance": "50.00"}, 409)
	})

	t.Run("48_64_65_66_concurrencia_permisos_y_sesion", func(t *testing.T) {
		fixture.adminCall(t, "PUT", "payments/settings", map[string]any{
			"operation_id": randomID(), "revision": decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200)).Revision,
			"shared_shifts": true, "print_receipt_default": true, "receipt_printer_id": nil, "refund_authorization_threshold": nil,
		}, 200)
		fixture.cashier = newCashier(t, fixture, "cajero_cobro", []string{
			"accounts.read", "payments.read", "payments.collect", "payments.collect_partial", "payments.collect_selection",
			"takeaway.read", "takeaway.deliver",
		}, registerID)
		fixture.observer = newCashier(t, fixture, "consulta_cobro", []string{"accounts.read", "payments.read"}, registerID)
		bill := fixture.account(t, "Concurrente", line(fixture.soda, 4))
		balance := fixture.finance(t, bill.Account.ID).Balance
		results := make(chan int, 2)
		for _, terminal := range []testTerminal{fixture.admin, fixture.cashier} {
			go func(terminal testTerminal) {
				body := map[string]any{"operation_id": randomID(), "method_id": cash, "mode": "partial", "amount": "60", "expected_balance": balance}
				encoded, _ := json.Marshal(body)
				results <- fixture.status(terminal, "POST", "payments/accounts/"+bill.Account.ID+"/payments", encoded)
			}(terminal)
		}
		assertOneWinner(t, []int{<-results, <-results})
		if fixture.finance(t, bill.Account.ID).Paid != "60.00" {
			t.Fatal("Dos terminales aplicaron cobros sobre el mismo saldo")
		}
		// 65 y S2: un 403 no deja escrituras financieras confirmadas.
		paymentsBefore := fixture.countRows(t, "SELECT count(*) FROM app.payments")
		fixture.pay(t, fixture.observer, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total"}, 403)
		fixture.pay(t, fixture.cashier, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total", "tip": "5"}, 403)
		if fixture.countRows(t, "SELECT count(*) FROM app.payments") != paymentsBefore || fixture.countRows(t, "SELECT count(*) FROM app.cash_movements WHERE account_id=$1", bill.Account.ID) != 1 {
			t.Fatal("Un cobro denegado dejó registros financieros")
		}
		// 66: una sesión revocada no cobra.
		var sessionID string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id FROM app.sessions WHERE token_hash=$1", administration.TokenHash(fixture.cashier.sessionToken)).Scan(&sessionID); err != nil {
			t.Fatal(err)
		}
		fixture.adminCall(t, "POST", "sessions/"+sessionID+"/revoke", map[string]any{}, 200)
		fixture.pay(t, fixture.cashier, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total"}, 401)
	})

	t.Run("A1_A2_A3_A4_iva_de_lista_cerrada_y_conservacion_historica", func(t *testing.T) {
		settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
		if settings.TaxRate != "16" {
			t.Fatal("La tasa inicial no es 16 %")
		}
		before := fixture.account(t, "IVA 16", line(fixture.burger, 1))
		for _, rate := range []string{"15", "0.16", "16.0", "-8", "abc", "08", "+8", ""} {
			fixture.adminCall(t, "PUT", "payments/tax", map[string]any{"operation_id": randomID(), "revision": settings.Revision, "tax_rate": rate}, 400)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.payment_settings SET tax_rate=15"); err == nil {
			t.Fatal("PostgreSQL aceptó una tasa fuera de la lista")
		}
		fixture.call(t, fixture.observer, "PUT", "payments/tax", map[string]any{"operation_id": randomID(), "revision": settings.Revision, "tax_rate": "8"}, 403)
		saved := decodeResponse[payments.Settings](t, fixture.adminCall(t, "PUT", "payments/tax", map[string]any{
			"operation_id": randomID(), "revision": settings.Revision, "tax_rate": "8",
		}, 200))
		if saved.TaxRate != "8" || fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='payments.tax_rate_changed'") != 1 {
			t.Fatal("El cambio de tasa no se guardó ni auditó")
		}
		after := fixture.account(t, "IVA 8", line(fixture.burger, 1))
		oldFinance := fixture.finance(t, before.Account.ID)
		newFinance := fixture.finance(t, after.Account.ID)
		if len(oldFinance.Tax) != 1 || oldFinance.Tax[0].Rate != 16 || oldFinance.Tax[0].Base != "103.45" || oldFinance.Tax[0].Tax != "16.55" {
			t.Fatalf("El cambio de tasa alteró una cuenta anterior: %+v", oldFinance.Tax)
		}
		if newFinance.Tax[0].Rate != 8 || newFinance.Tax[0].Base != "111.11" || newFinance.Tax[0].Tax != "8.89" {
			t.Fatalf("La nueva confirmación no tomó la tasa vigente: %+v", newFinance.Tax)
		}
		// Una cuenta con unidades de dos tasas desglosa cada una y la suma conserva el total.
		mixed := fixture.capture(t, before, []catalog.LineRequest{line(fixture.soda, 1)})
		mixedFinance := fixture.finance(t, mixed.Account.ID)
		sum := decimal.Zero
		for _, tax := range mixedFinance.Tax {
			sum = sum.Add(amountOf(t, tax.Base)).Add(amountOf(t, tax.Tax))
		}
		if len(mixedFinance.Tax) != 2 || !sum.Equal(amountOf(t, mixedFinance.Total)) {
			t.Fatal("El desglose de varias tasas no conserva exactamente el total")
		}
		fixture.adminCall(t, "PUT", "payments/tax", map[string]any{"operation_id": randomID(), "revision": saved.Revision, "tax_rate": "16"}, 200)
	})

	t.Run("50_56_57_58_59_entrega_y_persistencia_tras_reiniciar", func(t *testing.T) {
		takeaway := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "Para llevar cobrado", "mode": "takeaway", "origin": "Ventanilla",
		}, 200))
		takeaway = fixture.capture(t, takeaway, []catalog.LineRequest{line(fixture.soda, 2)})
		fixture.adminCall(t, "POST", "payments/accounts/"+takeaway.Account.ID+"/deliver", map[string]any{"operation_id": randomID()}, 409)
		partial := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, takeaway.Account.ID, map[string]any{
			"method_id": cash, "mode": "partial", "amount": "10", "deliver": true,
		}, 200))
		if partial.Delivered || partial.Finance.DeliveryState != "not_delivered" {
			t.Fatal("Un pedido con saldo pendiente se marcó como entregado")
		}
		paid := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, takeaway.Account.ID, map[string]any{"method_id": cash, "mode": "total"}, 200))
		if paid.Delivered || paid.Closed || paid.Finance.PaymentState != "paid" || paid.Finance.DeliveryState != "not_delivered" {
			t.Fatal("Un pedido pagado se consideró entregado automáticamente")
		}
		board := decodeResponse[struct {
			Items []operations.Account `json:"items"`
		}](t, fixture.adminCall(t, "GET", "restaurant/accounts?mode=takeaway", nil, 200))
		found := false
		for _, item := range board.Items {
			if item.ID == takeaway.Account.ID && item.PaymentState == "paid" && item.DeliveryState == "not_delivered" {
				found = true
			}
		}
		if !found {
			t.Fatal("El tablero para llevar no muestra el pedido pagado pendiente de entrega")
		}
		operationID := randomID()
		delivered := decodeResponse[struct {
			Delivered bool `json:"delivered"`
			Closed    bool `json:"closed"`
		}](t, fixture.adminCall(t, "POST", "payments/accounts/"+takeaway.Account.ID+"/deliver", map[string]any{"operation_id": operationID}, 200))
		if !delivered.Delivered || !delivered.Closed {
			t.Fatal("La entrega directa de un pedido liquidado no se registró")
		}
		fixture.adminCall(t, "POST", "payments/accounts/"+takeaway.Account.ID+"/deliver", map[string]any{"operation_id": operationID}, 200)
		fixture.adminCall(t, "POST", "payments/accounts/"+takeaway.Account.ID+"/deliver", map[string]any{"operation_id": randomID()}, 409)
		if fixture.countRows(t, "SELECT count(*) FROM app.takeaway_deliveries WHERE account_id=$1", takeaway.Account.ID) != 1 {
			t.Fatal("Se duplicó la entrega")
		}
		// Toggle de entrega dentro del cobro liquidador.
		quick := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": "Entrega al cobrar", "mode": "takeaway"}, 200))
		quick = fixture.capture(t, quick, []catalog.LineRequest{line(fixture.soda, 1)})
		toggled := decodeResponse[paymentResult](t, fixture.pay(t, fixture.admin, quick.Account.ID, map[string]any{"method_id": debit, "mode": "total", "deliver": true}, 200))
		if !toggled.Delivered || !toggled.Closed {
			t.Fatal("Marcar como entregado al liquidar no registró la entrega")
		}
		// 50: un servicio nuevo sobre la misma base conserva saldos y pagos.
		restarted := &administration.Service{Pool: fixture.pool, ProvisioningFile: fixture.service.ProvisioningFile}
		original := fixture.service
		fixture.service = restarted
		if fixture.finance(t, takeaway.Account.ID).Paid != "50.00" {
			t.Fatal("El saldo no se conservó al reiniciar el servicio")
		}
		fixture.service = original
	})

	t.Run("cajas_y_terminales_con_turno_abierto", func(t *testing.T) {
		fixture.adminCall(t, "PATCH", "devices/"+fixture.admin.deviceID, map[string]any{"name": "Prueba principal", "register_id": nil, "keyboard_mode": "auto"}, 409)
		fixture.adminCall(t, "PATCH", "devices/"+fixture.admin.deviceID, map[string]any{"name": "Terminal principal renombrada", "register_id": registerID, "keyboard_mode": "auto"}, 200)
		movements := decodeResponse[struct {
			Movements    []payments.Movement `json:"movements"`
			ExpectedCash string              `json:"expected_cash"`
		}](t, fixture.adminCall(t, "GET", "payments/shifts/"+fixture.registers["shift"]+"/movements", nil, 200))
		if len(movements.Movements) == 0 || fixture.countRows(t, "SELECT count(*) FROM app.cash_movements WHERE shift_id<>$1", fixture.registers["shift"]) != 0 {
			t.Fatal("Los movimientos no quedaron asociados al turno de su caja")
		}
	})

	testPaymentAdjustments(t, fixture)
	testPaymentDocuments(t, fixture)
	testProductTaxRates(t, fixture)
	testPaymentsUSD(t, fixture)
	testCashShifts(t, fixture)
}
