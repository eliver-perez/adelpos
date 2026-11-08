//go:build integration

package operations_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
)

func createWaiter(t *testing.T, fixture *restaurantFixture) {
	t.Helper()
	permissions := []string{
		"accounts.read", "accounts.open", "accounts.manage", "orders.capture", "orders.confirm", "orders.edit", "orders.cancel",
		"orders.history.read", "restaurant.tables.read", "restaurant.areas.read", "catalog.read", "catalog.prices.read",
		"takeaway.read", "takeaway.manage",
	}
	role := decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "roles", map[string]any{"name": "Mesero autorizado de prueba", "permissions": permissions}, 200))
	fixture.adminCall(t, "POST", "users", map[string]any{
		"username": "mesero", "display_name": "Mesero temporal", "role_id": role.ID, "active": true, "password": "Solo para aceptación temporal 4",
	}, 200)
	invitation := decodeResponse[struct {
		Code string `json:"code"`
	}](t, fixture.adminCall(t, "POST", "devices/invitations", nil, 200))
	device := decodeResponse[struct {
		ID    string `json:"device_id"`
		Token string `json:"device_token"`
	}](t,
		fixture.call(t, testTerminal{}, "POST", "devices/enroll", map[string]any{"code": invitation.Code, "name": "Terminal secundaria temporal"}, 200))
	fixture.waiter = testTerminal{deviceID: device.ID, deviceToken: device.Token}
	login := decodeResponse[struct {
		Token string `json:"session_token"`
	}](t,
		fixture.call(t, fixture.waiter, "POST", "auth/login", map[string]string{"username": "mesero", "password": "Solo para aceptación temporal 4"}, 200))
	fixture.waiter.sessionToken = login.Token
}

func concurrentStatuses(fixture *restaurantFixture, method, route string, first, second any) []int {
	var wait sync.WaitGroup
	result := make(chan int, 2)
	cases := []struct {
		terminal testTerminal
		body     any
	}{
		{fixture.admin, first}, {fixture.waiter, second},
	}
	for _, scenario := range cases {
		wait.Add(1)
		go func(terminal testTerminal, body any) {
			defer wait.Done()
			encoded, _ := json.Marshal(body)
			request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
			request.RemoteAddr = "127.0.0.1:52000"
			request.Header.Set("X-Device-ID", terminal.deviceID)
			request.Header.Set("X-Device-Token", terminal.deviceToken)
			request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
			response := httptest.NewRecorder()
			fixture.service.ServeHTTP(response, request)
			result <- response.Code
		}(scenario.terminal, scenario.body)
	}
	wait.Wait()
	close(result)
	statuses := []int{}
	for status := range result {
		statuses = append(statuses, status)
	}
	return statuses
}

func assertOneWinner(t *testing.T, statuses []int) {
	t.Helper()
	successes, conflicts := 0, 0
	for _, status := range statuses {
		if status == 200 {
			successes++
		}
		if status == 409 {
			conflicts++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("Se esperó una operación y un conflicto, se obtuvo %v", statuses)
	}
}

func testSecurityAndConcurrency(t *testing.T, fixture *restaurantFixture) {
	createWaiter(t, fixture)
	bill := newAccount(t, fixture, "Concurrencia entre terminales")
	route := "restaurant/accounts/" + bill.Account.ID
	t.Run("dos terminales no sobrescriben borradores ni duplican confirmaciones", func(t *testing.T) {
		first := map[string]any{"operation_id": randomID(), "revision": bill.Account.Revision, "items": []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.product.ID, Quantity: 1}}}}
		second := map[string]any{"operation_id": randomID(), "revision": bill.Account.Revision, "items": []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.product.ID, Quantity: 1}}}}
		assertOneWinner(t, concurrentStatuses(fixture, "PUT", route+"/draft", first, second))
		bill = fixture.refresh(t, bill.Account.ID)
		if len(bill.Draft) != 1 {
			t.Fatal("La concurrencia perdió o duplicó partidas del borrador")
		}
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "GET", route+"/preview", nil, 200))
		first = map[string]any{"operation_id": randomID(), "revision": bill.Account.Revision, "expected_fingerprint": preview.Fingerprint}
		second = map[string]any{"operation_id": randomID(), "revision": bill.Account.Revision, "expected_fingerprint": preview.Fingerprint}
		assertOneWinner(t, concurrentStatuses(fixture, "POST", route+"/confirm", first, second))
		bill = fixture.refresh(t, bill.Account.ID)
		if bill.Account.ConfirmedQuantity != 1 {
			t.Fatal("Se duplicó una confirmación concurrente")
		}
	})
	t.Run("mesero autorizado edita y cancela sin supervisor", func(t *testing.T) {
		line := bill.Lines[0]
		input := operations.EditInput{Mutation: operations.Mutation{Revision: bill.Account.Revision}, VersionID: line.VersionID, Line: line.Request, Reason: "Cambio ordinario autorizado"}
		input.Line.Customization.Notes = "Salsa aparte por favor"
		preview := decodeResponse[operations.QuoteResult](t, fixture.call(t, fixture.waiter, "POST", route+"/edit-preview", input, 200))
		input.OperationID = randomID()
		input.ExpectedFingerprint = preview.Fingerprint
		bill = decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "POST", route+"/edit", input, 200))
		line = bill.Lines[0]
		fixture.call(t, fixture.waiter, "POST", route+"/cancel", map[string]any{
			"operation_id": randomID(), "revision": bill.Account.Revision, "version_id": line.VersionID, "quantity": 1, "reason": "Cancelación ordinaria autorizada",
		}, 200)
		bill = fixture.refresh(t, bill.Account.ID)
		if bill.Account.ConfirmedQuantity != 0 {
			t.Fatal("La cancelación autorizada no se guardó")
		}
	})
	t.Run("permisos efectivos y rollback de transferencias", func(t *testing.T) {
		fixture.call(t, fixture.waiter, "POST", "restaurant/areas", map[string]any{"operation_id": randomID(), "name": "No autorizada", "active": true}, 403)
		target := newAccount(t, fixture, "Destino de rollback")
		source := fixture.capture(t, newAccount(t, fixture, "Origen de rollback"), []catalog.LineRequest{{ProductID: fixture.product.ID, Quantity: 1}, {ProductID: fixture.fries.ID, Quantity: 1}})
		fixture.mutate(t, "POST", "restaurant/accounts/"+source.Account.ID+"/transfer", operations.TransferInput{
			Mutation: operations.Mutation{Revision: source.Account.Revision}, TargetAccountID: &target.Account.ID, TargetRevision: target.Account.Revision,
			Selections: []operations.Selection{{VersionID: source.Lines[0].VersionID, Quantity: 1}, {VersionID: source.Lines[1].VersionID, Quantity: 999}},
		}, 400)
		refreshed := fixture.refresh(t, source.Account.ID)
		if refreshed.Account.Revision != source.Account.Revision || refreshed.Account.ConfirmedQuantity != 2 || fixture.refresh(t, target.Account.ID).Account.ConfirmedQuantity != 0 {
			t.Fatal("Se confirmó una transferencia parcial pese a fallar el lote")
		}
		if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.user_permissions(user_id,permission_key,effect)
            SELECT id,'orders.cancel','deny' FROM app.users WHERE username='mesero'`); err != nil {
			t.Fatal(err)
		}
		fixture.call(t, fixture.waiter, "POST", "restaurant/accounts/"+source.Account.ID+"/cancel", map[string]any{
			"operation_id": randomID(), "revision": source.Account.Revision, "version_id": source.Lines[0].VersionID, "quantity": 1, "reason": "Debe quedar denegada",
		}, 403)
		var denials, events int
		err := fixture.pool.QueryRow(fixture.ctx, `SELECT
            (SELECT count(*) FROM app.audit_events WHERE action='access.denied'),
            (SELECT count(*) FROM app.restaurant_events WHERE kind IN ('order.confirmed','item.modified','item.cancelled'))`).Scan(&denials, &events)
		if err != nil || denials < 2 || events < 10 {
			t.Fatal("Falta auditoría o diario durable de pedidos", err)
		}
	})
}
