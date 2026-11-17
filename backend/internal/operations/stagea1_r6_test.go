//go:build integration

package operations_test

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/operations"
)

// Etapa A1, correcciones de la revisión independiente R6 en el servidor (decisión A1-D18: el servidor nunca repite lo
// que ya entró), contra PostgreSQL con la API real. Incluye las reproducciones del revisor (TestR6Review A1 y A3, que
// encontraban dos partidas con el mismo draft_item_id) convertidas en comprobaciones de lo correcto:
//   - un renglón del borrador con el mismo id que una partida del pedido se sustituye aunque no venga en
//     replaces_draft_items (igual o guardado por el mismo mesero y dispositivo: se quita; cambiado en otra terminal:
//     draft_item_changed);
//   - confirmar un draft_item_sent envía lo demás y nunca otra vez lo ya enviado (si no queda nada, se acepta sin
//     pedido nuevo);
//   - la misma clave que un «Enviar a cocina» ya aplicado en esa cuenta es el mismo pedido (OFFLINE_ORDER_ALREADY_ACCEPTED,
//     origin confirm_draft) y nunca abre un agujero (otra cuenta, mesa, usuario u operación: IDEMPOTENCY_CONFLICT);
//   - un pedido a una mesa cuyas partidas ya están en una cuenta abierta de esa mesa: items_in_account con la cuenta;
//   - lo mismo con la credencial de envío.
//
// Usa su propia base desde cero.

// alreadyAcceptedFrom es el 409 OFFLINE_ORDER_ALREADY_ACCEPTED con el origen de lo que ya había entrado.
type alreadyAcceptedFrom struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details struct {
			Status string                        `json:"status"`
			Origin string                        `json:"origin"`
			Result operations.OfflineOrderResult `json:"result"`
		} `json:"details"`
	} `json:"error"`
}

// confirmWithKey envía a cocina el borrador de la cuenta con la clave dada (la que el dispositivo usará también para el
// pedido de la cola si no recibe la respuesta, A1-D19) y devuelve el pedido que entró.
func (fixture *waiterModeFixture) confirmWithKey(t *testing.T, terminal testTerminal, accountID, key string) string {
	t.Helper()
	route := "restaurant/accounts/" + accountID
	current := fixture.refresh(t, accountID)
	preview := decodeResponse[operations.QuoteResult](t, fixture.call(t, terminal, "GET", route+"/preview", nil, 200))
	fixture.call(t, terminal, "POST", route+"/confirm", map[string]any{
		"operation_id": key, "revision": current.Account.Revision, "expected_fingerprint": preview.Fingerprint,
	}, 200)
	return fixture.orderOf(t, key)
}

// orderOf es el pedido registrado con esa clave de operación ("" si no hay).
func (fixture *waiterModeFixture) orderOf(t *testing.T, key string) string {
	t.Helper()
	var orderID string
	err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(max(id::text),'') FROM app.restaurant_orders WHERE operation_id=$1", key).
		Scan(&orderID)
	if err != nil {
		t.Fatal(err)
	}
	return orderID
}

// sentTimes cuenta las partidas confirmadas que conservan el id del renglón: más de una sería un envío repetido.
func (fixture *waiterModeFixture) sentTimes(t *testing.T, id string) int {
	t.Helper()
	return fixture.count(t, "SELECT count(*)::int FROM app.restaurant_items WHERE draft_item_id=$1", id)
}

func (fixture *waiterModeFixture) unitsOf(t *testing.T, key string) int {
	t.Helper()
	return fixture.count(t, `SELECT count(*)::int FROM app.restaurant_item_units u JOIN app.restaurant_items i ON i.id=u.item_id
        JOIN app.restaurant_orders o ON o.id=i.original_order_id WHERE o.operation_id=$1`, key)
}

// submitConfirming envía el pedido; si Go pide confirmar cambios aceptables, comprueba sus tipos y lo reenvía con la
// confirmación y la misma clave (lo que hace «Aceptar los cambios y enviar»).
func (fixture *waiterModeFixture) submitConfirming(t *testing.T, terminal testTerminal, credential string, body map[string]any,
	kinds ...string) operations.OfflineOrderResult {
	t.Helper()
	code, response := fixture.raw(terminal, credential, "POST", "restaurant/offline-orders", body)
	rejection := decodeResponse[offlineRejection](t, response)
	got := []string{}
	for _, change := range rejection.Error.Details.Changes {
		got = append(got, change.Kind)
		if !change.Acceptable {
			t.Fatalf("cambio que no se acepta: %+v", change)
		}
	}
	if code != 409 || !slices.Equal(got, kinds) {
		t.Fatalf("se esperaban los cambios %v: %d %s", kinds, code, response)
	}
	body["confirmation"] = map[string]any{"catalog_version": rejection.Error.Details.CatalogVersion, "changes": rejection.Error.Details.Changes}
	code, response = fixture.raw(terminal, credential, "POST", "restaurant/offline-orders", body)
	if code != 200 {
		t.Fatalf("pedido confirmado: %d %s", code, response)
	}
	return decodeResponse[operations.OfflineOrderResult](t, response)
}

func TestStageA1OfflineNeverRepeats(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &waiterModeFixture{restaurantFixture: newFixture(t)}
	fixture.seedCatalog(t)
	fixture.server = httptest.NewServer(fixture.service)
	defer fixture.server.Close()
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón A1-R6", Active: true}, 200))
	for index := range 3 {
		fixture.tables = append(fixture.tables, decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: fmt.Sprintf("Mesa A1-R6 %d", index+1), Shape: "round", X: 20 + index*130, Y: 30,
			Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200)))
	}
	fixture.waiter = inventoryUser(t, fixture.restaurantFixture, "mesero_a1r6", waiterPermissions)
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username='mesero_a1r6'").Scan(&fixture.waiterID); err != nil {
		t.Fatal(err)
	}
	t.Run("R6 A1: guardado aplicado sin respuesta y pedido sin replaces_draft_items: el renglón se sustituye", func(t *testing.T) {
		testR6SavedDraftWithoutReplaces(t, fixture)
	})
	t.Run("R6 A3: Enviar a cocina aplicado y pedido con otra clave: lo ya enviado nunca se repite", func(t *testing.T) {
		testR6AlreadySentNeverRepeats(t, fixture)
	})
	t.Run("misma clave que un Enviar a cocina aplicado: ya aceptado, sin agujeros", func(t *testing.T) {
		testR6SameKeyAsConfirm(t, fixture)
	})
	t.Run("pedido a una mesa con las partidas en una cuenta abierta de la mesa: items_in_account", func(t *testing.T) {
		testR6ItemsInTableAccount(t, fixture)
	})
	t.Run("lo mismo con la credencial de envío", func(t *testing.T) {
		testR6WithOfflineCredential(t, fixture)
	})
}

// Reproducción del revisor (A1): la apertura con el primer producto entró en Go y el teléfono no recibió la respuesta;
// su pedido lleva el renglón como partida sin nombrarlo en replaces_draft_items. Antes el renglón se quedaba en el
// borrador y el siguiente «Enviar a cocina» lo mandaba otra vez.
func testR6SavedDraftWithoutReplaces(t *testing.T, fixture *waiterModeFixture) {
	mine := draftLine(fixture.fries, 2)
	accountID := fixture.open(t, fixture.waiter, map[string]any{"table_id": fixture.tables[0].ID, "seen_account_ids": []string{}}, mine).Account.ID
	key := randomID()
	body := offlineBody(key, fixture.catalogVersion(t), map[string]any{"account_id": accountID}, seenItem(mine, 2, "40.00"))
	result := decodeResponse[operations.OfflineOrderResult](t, fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders", body, 200))
	if !slices.Equal(result.RemovedDraftItemIDs, []string{mine.ID}) || len(result.Bill.Draft) != 0 || len(result.AlreadySentItemIDs) != 0 ||
		fixture.unitsOf(t, key) != 2 {
		t.Fatalf("pedido con el renglón guardado: quitados %v, borrador %+v", result.RemovedDraftItemIDs, result.Bill.Draft)
	}
	// La cuenta ya no tiene nada por enviar: otra terminal que toque «Enviar a cocina» no repite el renglón.
	current := fixture.refresh(t, accountID)
	code, response := fixture.raw(fixture.admin, "", "POST", "restaurant/accounts/"+accountID+"/confirm", map[string]any{
		"operation_id": randomID(), "revision": current.Account.Revision, "expected_fingerprint": "",
	})
	if code != 400 || !bytes.Contains(response, []byte("No hay productos pendientes")) || fixture.sentTimes(t, mine.ID) != 1 {
		t.Fatalf("el renglón debe haber entrado una sola vez: %d %s, %d partidas", code, response, fixture.sentTimes(t, mine.ID))
	}

	// El mismo mesero lo cambió sin conexión después de guardarlo (2 → 3): lo guardado es suyo y se sustituye.
	own := draftLine(fixture.fries, 2)
	ownAccount := fixture.open(t, fixture.waiter, map[string]any{"table_id": fixture.tables[1].ID, "seen_account_ids": []string{}}, own).Account.ID
	ownKey := randomID()
	result = decodeResponse[operations.OfflineOrderResult](t, fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders",
		offlineBody(ownKey, fixture.catalogVersion(t), map[string]any{"account_id": ownAccount}, seenItem(own, 3, "40.00")), 200))
	if !slices.Equal(result.RemovedDraftItemIDs, []string{own.ID}) || len(result.Bill.Draft) != 0 || fixture.unitsOf(t, ownKey) != 3 {
		t.Fatalf("renglón propio cambiado sin conexión: quitados %v, %d unidades", result.RemovedDraftItemIDs, fixture.unitsOf(t, ownKey))
	}

	// Otra terminal lo cambió (sin sal): no se quita en silencio; es draft_item_changed y, confirmado, se queda.
	shared := draftLine(fixture.fries, 1)
	sharedAccount := fixture.open(t, fixture.waiter, nil, shared).Account.ID
	theirs := shared
	theirs.Customization.Notes = "Sin sal"
	fixture.save(t, fixture.admin, sharedAccount, theirs)
	sharedKey := randomID()
	result = fixture.submitConfirming(t, fixture.waiter, "", offlineBody(sharedKey, fixture.catalogVersion(t),
		map[string]any{"account_id": sharedAccount}, seenItem(shared, 1, "40.00")), operations.OfflineDraftItemChanged)
	if len(result.RemovedDraftItemIDs) != 0 || len(result.Bill.Draft) != 1 || result.Bill.Draft[0].Customization.Notes != "Sin sal" ||
		fixture.unitsOf(t, sharedKey) != 1 {
		t.Fatalf("renglón cambiado en otra terminal: %+v", result.Bill.Draft)
	}
}

// Reproducción del revisor (A3): «Enviar a cocina» entró y su respuesta se perdió; el pedido sale con otra clave. Antes,
// aceptar draft_item_sent lo mandaba dos veces.
func testR6AlreadySentNeverRepeats(t *testing.T, fixture *waiterModeFixture) {
	mine, extra := draftLine(fixture.fries, 2), draftLine(fixture.product, 1)
	accountID := fixture.open(t, fixture.waiter, nil, mine).Account.ID
	confirmed := fixture.confirmWithKey(t, fixture.waiter, accountID, randomID())
	// Con otra partida nueva: se confirma y entra solo lo demás.
	key := randomID()
	body := offlineBody(key, fixture.catalogVersion(t), map[string]any{"account_id": accountID}, seenItem(mine, 2, "40.00"),
		seenItem(extra, 1, "150.00"))
	body["replaces_draft_items"] = []operations.DraftItem{mine}
	code, response := fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", body)
	_, byItem := replacedChanges(t, response)
	if code != 409 || byItem[mine.ID].Kind != operations.OfflineDraftItemSent || !byItem[mine.ID].Acceptable ||
		byItem[mine.ID].Message != "Papas producto: ya se había enviado a cocina desde este dispositivo y no se vuelve a enviar. "+
			"Si confirmas, se envía lo demás del pedido." {
		t.Fatalf("partida ya enviada desde este dispositivo: %d %s", code, response)
	}
	result := fixture.submitConfirming(t, fixture.waiter, "", body, operations.OfflineDraftItemSent)
	if !slices.Equal(result.AlreadySentItemIDs, []string{mine.ID}) || result.OrderID == confirmed || fixture.unitsOf(t, key) != 1 ||
		fixture.sentTimes(t, mine.ID) != 1 || fixture.sentTimes(t, extra.ID) != 1 {
		t.Fatalf("debe entrar solo lo demás: ya enviadas %v, %d unidades, %d envíos", result.AlreadySentItemIDs, fixture.unitsOf(t, key),
			fixture.sentTimes(t, mine.ID))
	}
	// Todo ya enviado (sin nombrarlo en replaces_draft_items): se acepta sin pedido nuevo y la cuenta no cambia.
	revision := fixture.refresh(t, accountID).Account.Revision
	again := randomID()
	repeat := offlineBody(again, fixture.catalogVersion(t), map[string]any{"account_id": accountID}, seenItem(mine, 2, "40.00"))
	result = fixture.submitConfirming(t, fixture.waiter, "", repeat, operations.OfflineDraftItemSent)
	if result.Status != "accepted" || !slices.Equal(result.AlreadySentItemIDs, []string{mine.ID}) || result.OrderID != confirmed ||
		fixture.orderOf(t, again) != "" || fixture.sentTimes(t, mine.ID) != 1 || result.Bill.Account.Revision != revision {
		t.Fatalf("pedido ya enviado completo: %+v (pedido %s)", result, fixture.orderOf(t, again))
	}
	// El reintento (respuesta perdida) devuelve lo mismo, sin escribir nada.
	if code, response := fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", repeat); code != 200 ||
		decodeResponse[operations.OfflineOrderResult](t, response).OrderID != confirmed || fixture.sentTimes(t, mine.ID) != 1 {
		t.Fatalf("reintento del pedido ya enviado: %d %s", code, response)
	}
}

func testR6SameKeyAsConfirm(t *testing.T, fixture *waiterModeFixture) {
	mine := draftLine(fixture.fries, 1)
	accountID := fixture.open(t, fixture.waiter, nil, mine).Account.ID
	key := randomID()
	orderID := fixture.confirmWithKey(t, fixture.waiter, accountID, key)
	orders := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_orders")
	body := offlineBody(key, fixture.catalogVersion(t), map[string]any{"account_id": accountID}, seenItem(mine, 1, "40.00"))
	code, response := fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", body)
	accepted := decodeResponse[alreadyAcceptedFrom](t, response)
	result := accepted.Error.Details.Result
	if code != 409 || accepted.Error.Code != "OFFLINE_ORDER_ALREADY_ACCEPTED" || accepted.Error.Details.Status != "already_accepted" ||
		accepted.Error.Details.Origin != "confirm_draft" || result.Status != "accepted" || result.OperationID != key ||
		result.OrderID != orderID || result.AccountID != accountID || !slices.Equal(result.AlreadySentItemIDs, []string{mine.ID}) ||
		result.Bill == nil || result.Bill.Account.ID != accountID || len(result.Bill.Lines) != 1 || len(result.AcceptedChanges) != 0 {
		t.Fatalf("pedido con la clave de un Enviar a cocina aplicado: %d %s", code, response)
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_orders") != orders || fixture.sentTimes(t, mine.ID) != 1 {
		t.Fatal("el pedido con la clave de la confirmación no debe mandar nada")
	}
	// La misma clave con otra cuenta, a una mesa, desde otro usuario o de otra operación: el conflicto de siempre, sin
	// el pedido de nadie.
	other := fixture.open(t, fixture.waiter, nil, draftLine(fixture.fries, 1)).Account.ID
	saveKey := randomID()
	current := fixture.refresh(t, other)
	fixture.call(t, fixture.waiter, "PUT", "restaurant/accounts/"+other+"/draft", map[string]any{
		"operation_id": saveKey, "revision": current.Account.Revision, "items": current.Draft,
	}, 200)
	cases := map[string]struct {
		terminal testTerminal
		body     map[string]any
	}{
		"otra cuenta":    {fixture.waiter, offlineBody(key, fixture.catalogVersion(t), map[string]any{"account_id": other}, seenItem(mine, 1, "40.00"))},
		"una mesa":       {fixture.waiter, offlineBody(key, fixture.catalogVersion(t), map[string]any{"table_id": fixture.tables[2].ID}, seenItem(mine, 1, "40.00"))},
		"otro usuario":   {fixture.admin, body},
		"otra operación": {fixture.waiter, offlineBody(saveKey, fixture.catalogVersion(t), map[string]any{"account_id": other}, offlineItem(fixture.fries.ID, 1, "40.00"))},
	}
	for name, attempt := range cases {
		code, response := fixture.raw(attempt.terminal, "", "POST", "restaurant/offline-orders", attempt.body)
		if code != 409 || !bytes.Contains(response, []byte("IDEMPOTENCY_CONFLICT")) || bytes.Contains(response, []byte(orderID)) {
			t.Fatalf("misma clave (%s): %d %s", name, code, response)
		}
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_orders") != orders {
		t.Fatal("un conflicto de clave registró un pedido")
	}
}

// La apertura de la cuenta con su primer producto entró en Go y su respuesta se perdió: el pedido sale a la mesa.
// Antes, aceptar table_has_account terminaba en un 400 genérico («la partida pertenece a otra cuenta»).
func testR6ItemsInTableAccount(t *testing.T, fixture *waiterModeFixture) {
	table := fixture.tables[2]
	mine := draftLine(fixture.fries, 2)
	opened := fixture.open(t, fixture.waiter, map[string]any{"table_id": table.ID, "seen_account_ids": []string{}}, mine)
	key := randomID()
	body := offlineBody(key, fixture.catalogVersion(t), map[string]any{"table_id": table.ID, "seen_account_ids": []string{}},
		seenItem(mine, 2, "40.00"))
	code, response := fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", body)
	rejection := decodeResponse[offlineRejection](t, response)
	changes := rejection.Error.Details.Changes
	folio := fmt.Sprintf("folio %d", opened.Account.Folio)
	if code != 409 || !rejection.Error.Details.RequiresChanges || len(changes) != 1 || changes[0].Kind != operations.OfflineItemsInAccount ||
		changes[0].AccountID == nil || *changes[0].AccountID != opened.Account.ID || changes[0].Acceptable || changes[0].Now == nil ||
		*changes[0].Now != folio || !strings.Contains(changes[0].Message, "Envía el pedido a esa cuenta") {
		t.Fatalf("partidas ya en una cuenta de la mesa: %d %s", code, response)
	}
	// No se confirma: hay que dirigirlo a esa cuenta. Con la misma clave, entra una sola vez y quita el renglón.
	body["confirmation"] = map[string]any{"catalog_version": rejection.Error.Details.CatalogVersion, "changes": changes}
	if code, _ := fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", body); code != 409 {
		t.Fatalf("items_in_account no se acepta con la confirmación: %d", code)
	}
	delete(body, "confirmation")
	body["destination"] = map[string]any{"account_id": opened.Account.ID}
	result := decodeResponse[operations.OfflineOrderResult](t, fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders", body, 200))
	if result.AccountID != opened.Account.ID || !slices.Equal(result.RemovedDraftItemIDs, []string{mine.ID}) || fixture.unitsOf(t, key) != 2 ||
		len(fixture.openAccounts(t, table.ID)) != 1 {
		t.Fatalf("pedido dirigido a la cuenta de la mesa: %+v", result)
	}
	// Ya enviadas, también se reconocen: el pedido a la mesa vuelve a decir la cuenta (y ahí será draft_item_sent).
	code, response = fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", offlineBody(randomID(), fixture.catalogVersion(t),
		map[string]any{"table_id": table.ID, "seen_account_ids": []string{opened.Account.ID}}, seenItem(mine, 2, "40.00")))
	rejection = decodeResponse[offlineRejection](t, response)
	if code != 409 || len(rejection.Error.Details.Changes) != 1 || rejection.Error.Details.Changes[0].Kind != operations.OfflineItemsInAccount ||
		*rejection.Error.Details.Changes[0].AccountID != opened.Account.ID {
		t.Fatalf("partidas ya enviadas en una cuenta de la mesa: %d %s", code, response)
	}
}

// openAccounts devuelve las cuentas abiertas de una mesa.
func (fixture *waiterModeFixture) openAccounts(t *testing.T, tableID string) []string {
	t.Helper()
	rows, err := fixture.pool.Query(fixture.ctx, "SELECT id::text FROM app.restaurant_accounts WHERE table_id=$1 AND state='open'", tableID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func testR6WithOfflineCredential(t *testing.T, fixture *waiterModeFixture) {
	fixture.adminCall(t, "PUT", "users/"+fixture.waiterID+"/pin", map[string]any{"pin": "4829", "short_name": "Ana", "badge_color": "#3591d4"}, 200)
	phone := fixture.waiterPhone(t)
	code, response := fixture.raw(phone, "", "POST", "auth/pin-login", map[string]string{"user_id": fixture.waiterID, "pin": "4829"})
	login := decodeResponse[struct {
		Session    string `json:"session_token"`
		Credential string `json:"offline_credential"`
	}](t, response)
	if code != 200 || login.Credential == "" {
		t.Fatalf("entrada con PIN: %d %s", code, response)
	}
	tablet := phone
	tablet.sessionToken = login.Session
	// «Enviar a cocina» en la tableta con la clave K; la respuesta se pierde y la cola manda el pedido con la misma clave.
	mine := draftLine(fixture.fries, 1)
	accountID := fixture.open(t, tablet, nil, mine).Account.ID
	key := randomID()
	orderID := fixture.confirmWithKey(t, tablet, accountID, key)
	code, response = fixture.raw(phone, login.Credential, "POST", "restaurant/offline-orders",
		offlineBody(key, fixture.catalogVersion(t), map[string]any{"account_id": accountID}, seenItem(mine, 1, "40.00")))
	accepted := decodeResponse[alreadyAcceptedFrom](t, response)
	if code != 409 || accepted.Error.Code != "OFFLINE_ORDER_ALREADY_ACCEPTED" || accepted.Error.Details.Origin != "confirm_draft" ||
		accepted.Error.Details.Result.OrderID != orderID || fixture.sentTimes(t, mine.ID) != 1 {
		t.Fatalf("misma clave con la credencial de envío: %d %s", code, response)
	}
	// Con otra clave: draft_item_sent y, confirmado, nada se repite.
	result := fixture.submitConfirming(t, phone, login.Credential, offlineBody(randomID(), fixture.catalogVersion(t),
		map[string]any{"account_id": accountID}, seenItem(mine, 1, "40.00")), operations.OfflineDraftItemSent)
	if result.OrderID != orderID || fixture.sentTimes(t, mine.ID) != 1 {
		t.Fatalf("ya enviado con la credencial de envío: %+v", result)
	}
	// Un renglón guardado desde la tableta que el pedido no nombra: se sustituye.
	saved := draftLine(fixture.product, 1)
	fixture.save(t, tablet, accountID, saved)
	result = decodeResponse[operations.OfflineOrderResult](t, fixture.raw2xx(t, phone, login.Credential, offlineBody(randomID(),
		fixture.catalogVersion(t), map[string]any{"account_id": accountID}, seenItem(saved, 1, "150.00"))))
	if !slices.Equal(result.RemovedDraftItemIDs, []string{saved.ID}) || len(result.Bill.Draft) != 0 || fixture.sentTimes(t, saved.ID) != 1 {
		t.Fatalf("renglón guardado y pedido con la credencial: %+v", result)
	}
}

// raw2xx envía un pedido sin conexión y exige 200.
func (fixture *waiterModeFixture) raw2xx(t *testing.T, terminal testTerminal, credential string, body map[string]any) []byte {
	t.Helper()
	code, response := fixture.raw(terminal, credential, "POST", "restaurant/offline-orders", body)
	if code != 200 {
		t.Fatalf("pedido sin conexión: %d %s", code, response)
	}
	return response
}
