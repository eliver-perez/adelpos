//go:build integration

package operations_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa A2, parte B (Para llevar en el servidor) contra PostgreSQL con la API real, con un mesero que entra con PIN en
// un teléfono (sesión acotada a los permisos de mesero, A1-D17) y su credencial de envío:
//   - el pedido para llevar nuevo capturado sin conexión (destination.takeaway): abre la cuenta para llevar con su
//     origen, el cliente y el mesero que envía como responsable; la misma clave nunca abre dos pedidos (reintento con
//     la credencial o con la sesión, respuesta perdida y otro contenido) y, si las partidas ya están en un pedido para
//     llevar abierto (la apertura en línea entró sin respuesta), items_in_account con ese pedido (A1-D18);
//   - permisos (accounts.open + takeaway.manage) y forma del destino;
//   - nombre y teléfono después (A2-D1): ruta estrecha sin revisión, idempotente, con su evento en el ámbito takeaway,
//     solo en pedidos para llevar abiertos;
//   - la lista de pendientes y restaurant/state con preparación, pago y entrega, y «Entregado» solo con el pedido pagado
//     (A2-D2), sin exponer el cobro al mesero.
//
// Usa su propia base desde cero.

// takeawayWaiterPermissions es el rol Mesero de la guía para Para llevar: lo de la A1 más takeaway.*.
var takeawayWaiterPermissions = append(slices.Clone(waiterPermissions), "takeaway.read", "takeaway.manage", "takeaway.deliver")

type takeawayFixture struct {
	*waiterModeFixture
	// phone es el teléfono de meseros (solo su credencial de dispositivo); fixture.waiter es el mismo teléfono con la
	// sesión con PIN del mesero y credential, su credencial de envío.
	phone      testTerminal
	credential string
	// other es otro mesero en el mismo teléfono, con el rol de la A1 (sin Para llevar).
	other           testTerminal
	otherCredential string
	cashMethod      string
}

// pinLogin entra con PIN en el teléfono y devuelve la sesión y la credencial de envío.
func (fixture *takeawayFixture) pinLogin(t *testing.T, userID, pin string) (testTerminal, string) {
	t.Helper()
	code, response := fixture.raw(fixture.phone, "", "POST", "auth/pin-login", map[string]string{"user_id": userID, "pin": pin})
	login := decodeResponse[struct {
		Session    string `json:"session_token"`
		Credential string `json:"offline_credential"`
	}](t, response)
	if code != 200 || login.Session == "" || login.Credential == "" {
		t.Fatalf("entrada con PIN: %d %s", code, response)
	}
	session := fixture.phone
	session.sessionToken = login.Session
	return session, login.Credential
}

// userID es el identificador del usuario con ese nombre de acceso.
func (fixture *takeawayFixture) userID(t *testing.T, username string) string {
	t.Helper()
	var id string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username=$1", username).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// takeawayDestination es el destino de un pedido para llevar nuevo con los datos del cliente dados.
func takeawayDestination(customer map[string]any) map[string]any {
	return map[string]any{"takeaway": customer}
}

// eventsOf cuenta los eventos en tiempo real de la cuenta posteriores al cursor, por tipo y ámbito.
func (fixture *takeawayFixture) eventsOf(t *testing.T, after int64, accountID, kind, scope string) int {
	t.Helper()
	return fixture.count(t, `SELECT count(*)::int FROM app.realtime_events WHERE sequence>$1 AND aggregate_id=$2 AND kind=$3
        AND scope=$4`, after, accountID, kind, scope)
}

// pendingTakeaway es la cuenta de la lista de pendientes del mesero (listAccounts?mode=takeaway&state=open), o nil.
func (fixture *takeawayFixture) pendingTakeaway(t *testing.T, accountID string) *operations.Account {
	t.Helper()
	list := decodeResponse[struct {
		Items []operations.Account `json:"items"`
	}](t, fixture.call(t, fixture.waiter, "GET", "restaurant/accounts?mode=takeaway&state=open&page_size=100", nil, 200))
	for index := range list.Items {
		if list.Items[index].ID == accountID {
			return &list.Items[index]
		}
	}
	return nil
}

// payInFull cobra en la caja (la computadora principal) el saldo completo de la cuenta en efectivo.
func (fixture *takeawayFixture) payInFull(t *testing.T, accountID string) {
	t.Helper()
	finance := decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+accountID, nil, 200))
	fixture.adminCall(t, "POST", "payments/accounts/"+accountID+"/payments", map[string]any{
		"operation_id": randomID(), "method_id": fixture.cashMethod, "mode": "total", "expected_balance": finance.Balance,
	}, 200)
}

func TestStageA2Takeaway(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &takeawayFixture{waiterModeFixture: &waiterModeFixture{restaurantFixture: newFixture(t)}}
	fixture.seedCatalog(t)
	// Cocina en pantalla (sin impresora) para la categoría del catálogo de prueba: confirmar crea tareas.
	fixture.station = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina A2-B", Active: true, Reception: "kds", TargetMinutes: 12, WarningMinutes: 15, OverdueMinutes: 25,
		Grouping: "inherit", Tracking: "inherit",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare", StationIDs: []string{fixture.station.ID}}, 200)
	fixture.cook = inventoryUser(t, fixture.restaurantFixture, "cocina_a2b", []string{
		"kitchen.states.read", "kitchen.states.manage", "kitchen.stations.all", "events.read",
	})
	// El mesero con el rol de la guía (con Para llevar) y otro con el de la A1, los dos con PIN en el mismo teléfono.
	inventoryUser(t, fixture.restaurantFixture, "mesero_a2b", takeawayWaiterPermissions)
	inventoryUser(t, fixture.restaurantFixture, "mesero_a2b_sin", waiterPermissions)
	fixture.waiterID = fixture.userID(t, "mesero_a2b")
	otherID := fixture.userID(t, "mesero_a2b_sin")
	fixture.adminCall(t, "PUT", "users/"+fixture.waiterID+"/pin", map[string]any{"pin": "4829", "short_name": "Ana", "badge_color": ""}, 200)
	fixture.adminCall(t, "PUT", "users/"+otherID+"/pin", map[string]any{"pin": "6152", "short_name": "Beto", "badge_color": ""}, 200)
	fixture.phone = fixture.waiterPhone(t)
	fixture.waiter, fixture.credential = fixture.pinLogin(t, fixture.waiterID, "4829")
	fixture.other, fixture.otherCredential = fixture.pinLogin(t, otherID, "6152")
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil && *method.Code == "cash" {
			fixture.cashMethod = method.ID
		}
	}
	fixture.adminCall(t, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0"}, 200)

	t.Run("pedido para llevar sin conexión: abre la cuenta una sola vez con su cliente y su responsable", func(t *testing.T) {
		testStageA2OfflineTakeaway(t, fixture)
	})
	t.Run("apertura en línea sin respuesta: items_in_account con el pedido para llevar", func(t *testing.T) {
		testStageA2TakeawayItemsInAccount(t, fixture)
	})
	t.Run("permisos y forma del destino para llevar", func(t *testing.T) {
		testStageA2TakeawayPermissionsAndShape(t, fixture)
	})
	t.Run("nombre y teléfono después (A2-D1)", func(t *testing.T) {
		testStageA2TakeawayCustomer(t, fixture)
	})
	t.Run("pendientes con preparación, pago y entrega; Entregado solo pagado (A2-D2)", func(t *testing.T) {
		testStageA2TakeawayPendingAndDelivery(t, fixture)
	})
}

func testStageA2OfflineTakeaway(t *testing.T, fixture *takeawayFixture) {
	accounts := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts")
	key := randomID()
	line := draftLine(fixture.fries, 2)
	body := offlineBody(key, fixture.catalogVersion(t), takeawayDestination(map[string]any{
		"customer_name": "  Juan  ", "phone": "555 123 4567",
	}), seenItem(line, 2, "40.00"))
	before := fixture.head(t)
	code, response := fixture.raw(fixture.phone, fixture.credential, "POST", "restaurant/offline-orders", body)
	if code != 200 {
		t.Fatalf("pedido para llevar con la credencial de envío: %d %s", code, response)
	}
	result := decodeResponse[operations.OfflineOrderResult](t, response)
	account := result.Bill.Account
	if result.Status != "accepted" || account.ID != result.AccountID || account.Mode != "takeaway" || account.TableID != nil ||
		account.Origin != operations.TakeawayOrigin || account.CustomerName != "Juan" || account.Phone != "555 123 4567" ||
		account.Name != "Pedido para llevar" || account.AssignedUserID != fixture.waiterID || len(result.Bill.Lines) != 1 ||
		result.Bill.Lines[0].Quantity != 2 || len(result.Bill.Draft) != 0 {
		t.Fatalf("pedido para llevar aceptado: %+v", result.Bill)
	}
	var openedBy, deviceID string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT opened_by::text,device_id::text FROM app.restaurant_accounts WHERE id=$1",
		account.ID).Scan(&openedBy, &deviceID); err != nil {
		t.Fatal(err)
	}
	if openedBy != fixture.waiterID || deviceID != fixture.phone.deviceID {
		t.Fatalf("quién y desde dónde abrió el pedido: %s %s", openedBy, deviceID)
	}
	// Todo en el ámbito takeaway, y cocina lo recibió (dos unidades, una estación).
	if fixture.eventsOf(t, before, account.ID, "restaurant.account.opened", "takeaway") != 1 ||
		fixture.eventsOf(t, before, account.ID, "restaurant.order.confirmed", "takeaway") != 1 ||
		fixture.eventsOf(t, before, account.ID, kitchen.PreparationChangedKind, "takeaway") != 1 ||
		fixture.count(t, "SELECT count(*)::int FROM app.realtime_events WHERE sequence>$1 AND aggregate_id=$2 AND scope='restaurant'",
			before, account.ID) != 0 {
		t.Fatal("los eventos del pedido para llevar deben ir en el ámbito takeaway")
	}
	if len(fixture.taskReferences(t, account.ID)) != 2 || fixture.sentTimes(t, line.ID) != 1 {
		t.Fatal("cocina no recibió el pedido para llevar")
	}
	// El reintento con la misma clave (respuesta perdida) devuelve lo mismo, con la credencial o con la sesión.
	for name, sender := range map[string][2]any{"credencial": {fixture.phone, fixture.credential}, "sesión": {fixture.waiter, ""}} {
		code, again := fixture.raw(sender[0].(testTerminal), sender[1].(string), "POST", "restaurant/offline-orders", body)
		if code != 200 || canonical(t, again) != canonical(t, response) {
			t.Fatalf("reintento con la %s: %d %s", name, code, again)
		}
	}
	// La misma clave con otro contenido (por ejemplo, el cliente corregido sin conexión): el pedido que ya entró, nunca
	// otro.
	changed := offlineBody(key, fixture.catalogVersion(t), takeawayDestination(map[string]any{"customer_name": "Pedro"}),
		seenItem(line, 2, "40.00"))
	code, conflict := fixture.raw(fixture.phone, fixture.credential, "POST", "restaurant/offline-orders", changed)
	accepted := decodeResponse[alreadyAcceptedFrom](t, conflict)
	if code != 409 || accepted.Error.Code != "OFFLINE_ORDER_ALREADY_ACCEPTED" || accepted.Error.Details.Origin != "offline_order" ||
		accepted.Error.Details.Result.AccountID != account.ID {
		t.Fatalf("misma clave con otro contenido: %d %s", code, conflict)
	}
	// Desde otro mesero la misma clave es de otra operación.
	if code, response := fixture.raw(fixture.other, "", "POST", "restaurant/offline-orders", body); code != 409 ||
		errorCode(t, response) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("misma clave desde otro mesero: %d %s", code, response)
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts") != accounts+1 || fixture.sentTimes(t, line.ID) != 1 ||
		fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='restaurant.offline_order.accepted' AND entity_id=$1", key) != 1 {
		t.Fatal("la misma clave abrió o registró más de un pedido")
	}
	// Sin datos del cliente también se acepta (son opcionales).
	plain := decodeResponse[operations.OfflineOrderResult](t, fixture.raw2xx(t, fixture.waiter, "", offlineBody(randomID(),
		fixture.catalogVersion(t), takeawayDestination(map[string]any{}), offlineItem(fixture.product.ID, 1, "150.00"))))
	if plain.Bill.Account.Mode != "takeaway" || plain.Bill.Account.CustomerName != "" || plain.Bill.Account.Phone != "" ||
		plain.Bill.Account.Origin != operations.TakeawayOrigin {
		t.Fatalf("pedido para llevar sin datos del cliente: %+v", plain.Bill.Account)
	}
}

func testStageA2TakeawayItemsInAccount(t *testing.T, fixture *takeawayFixture) {
	// En línea, «Nuevo pedido» abrió el pedido con su primer producto, pero el teléfono no recibió la respuesta; la red se
	// cortó y el pedido sale de la cola como pedido para llevar nuevo.
	line := draftLine(fixture.fries, 1)
	opened := fixture.open(t, fixture.waiter, map[string]any{"mode": "takeaway", "origin": operations.TakeawayOrigin,
		"customer_name": "Lucía"}, line)
	takeaways := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts WHERE mode='takeaway'")
	key := randomID()
	body := offlineBody(key, fixture.catalogVersion(t), takeawayDestination(map[string]any{"customer_name": "Lucía"}),
		seenItem(line, 1, "40.00"))
	code, response := fixture.raw(fixture.phone, fixture.credential, "POST", "restaurant/offline-orders", body)
	rejection := decodeResponse[offlineRejection](t, response)
	changes := rejection.Error.Details.Changes
	folio := fmt.Sprintf("folio %d", opened.Account.Folio)
	if code != 409 || !rejection.Error.Details.RequiresChanges || len(changes) != 1 || changes[0].Kind != operations.OfflineItemsInAccount ||
		changes[0].AccountID == nil || *changes[0].AccountID != opened.Account.ID || changes[0].Acceptable || changes[0].Now == nil ||
		*changes[0].Now != folio || changes[0].Message != "Lo que capturaste ya está en el pedido para llevar de Lucía ("+folio+
		"). Envía el pedido a esa cuenta." {
		t.Fatalf("partidas ya en un pedido para llevar: %d %s", code, response)
	}
	// No se confirma: hay que dirigirlo a esa cuenta, con la misma clave; entra una sola vez y quita el renglón.
	body["confirmation"] = map[string]any{"catalog_version": rejection.Error.Details.CatalogVersion, "changes": changes}
	if code, _ := fixture.raw(fixture.phone, fixture.credential, "POST", "restaurant/offline-orders", body); code != 409 {
		t.Fatalf("items_in_account no se acepta con la confirmación: %d", code)
	}
	delete(body, "confirmation")
	body["destination"] = map[string]any{"account_id": opened.Account.ID}
	result := decodeResponse[operations.OfflineOrderResult](t, fixture.raw2xx(t, fixture.phone, fixture.credential, body))
	if result.AccountID != opened.Account.ID || !slices.Equal(result.RemovedDraftItemIDs, []string{line.ID}) ||
		len(result.Bill.Draft) != 0 || fixture.unitsOf(t, key) != 1 || fixture.sentTimes(t, line.ID) != 1 {
		t.Fatalf("pedido dirigido al pedido para llevar: %+v", result)
	}
	// Ya enviadas también se reconocen: otro pedido para llevar con esas partidas vuelve a decir la cuenta.
	code, response = fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", offlineBody(randomID(),
		fixture.catalogVersion(t), takeawayDestination(map[string]any{}), seenItem(line, 1, "40.00")))
	rejection = decodeResponse[offlineRejection](t, response)
	if code != 409 || len(rejection.Error.Details.Changes) != 1 || rejection.Error.Details.Changes[0].Kind != operations.OfflineItemsInAccount ||
		*rejection.Error.Details.Changes[0].AccountID != opened.Account.ID {
		t.Fatalf("partidas ya enviadas en un pedido para llevar: %d %s", code, response)
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts WHERE mode='takeaway'") != takeaways ||
		fixture.sentTimes(t, line.ID) != 1 {
		t.Fatal("se abrió otro pedido para llevar o se repitió la partida")
	}
}

func testStageA2TakeawayPermissionsAndShape(t *testing.T, fixture *takeawayFixture) {
	accounts := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts")
	version := fixture.catalogVersion(t)
	order := func(destination map[string]any) map[string]any {
		return offlineBody(randomID(), version, destination, offlineItem(fixture.fries.ID, 1, "40.00"))
	}
	// El mesero con el rol de la A1 (sin takeaway.manage): 403, con la sesión o con la credencial, sin dejar nada.
	for name, sender := range map[string][2]any{"sesión": {fixture.other, ""}, "credencial": {fixture.phone, fixture.otherCredential}} {
		code, response := fixture.raw(sender[0].(testTerminal), sender[1].(string), "POST", "restaurant/offline-orders",
			order(takeawayDestination(map[string]any{"customer_name": "Sin permiso"})))
		if code != 403 || errorCode(t, response) != "PERMISSION_DENIED" {
			t.Fatalf("pedido para llevar sin takeaway.manage (%s): %d %s", name, code, response)
		}
	}
	// Forma: un solo destino; cuentas vistas y nombre solo con una mesa; los límites del cliente de AccountInput.
	for name, body := range map[string]map[string]any{
		"con mesa":           order(map[string]any{"takeaway": map[string]any{}, "table_id": randomID()}),
		"con cuenta":         order(map[string]any{"takeaway": map[string]any{}, "account_id": randomID()}),
		"con cuentas vistas": order(map[string]any{"takeaway": map[string]any{}, "seen_account_ids": []string{randomID()}}),
		"con nombre":         order(map[string]any{"takeaway": map[string]any{}, "name": "Mesa ficticia"}),
		"cliente largo":      order(takeawayDestination(map[string]any{"customer_name": strings.Repeat("J", 101)})),
		"teléfono largo":     order(takeawayDestination(map[string]any{"phone": strings.Repeat("5", 41)})),
		"campo desconocido":  order(takeawayDestination(map[string]any{"table_id": randomID()})),
		"sin destino":        order(map[string]any{}),
		"destino nulo":       order(map[string]any{"takeaway": nil}),
	} {
		if code, response := fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", body); code != 400 {
			t.Fatalf("destino para llevar inválido (%s): %d %s", name, code, response)
		}
	}
	replaces := order(takeawayDestination(map[string]any{}))
	replaces["replaces_draft_items"] = []operations.DraftItem{draftLine(fixture.fries, 1)}
	if code, response := fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", replaces); code != 400 {
		t.Fatalf("replaces_draft_items en un pedido para llevar nuevo: %d %s", code, response)
	}
	// En el límite exacto sí entra (100 y 40 caracteres, recortados).
	exact := decodeResponse[operations.OfflineOrderResult](t, fixture.raw2xx(t, fixture.waiter, "", order(takeawayDestination(map[string]any{
		"customer_name": " " + strings.Repeat("J", 100) + " ", "phone": strings.Repeat("5", 40),
	}))))
	if len(exact.Bill.Account.CustomerName) != 100 || len(exact.Bill.Account.Phone) != 40 {
		t.Fatalf("límites del cliente: %q %q", exact.Bill.Account.CustomerName, exact.Bill.Account.Phone)
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts") != accounts+1 {
		t.Fatal("un pedido rechazado dejó una cuenta")
	}
}

func testStageA2TakeawayCustomer(t *testing.T, fixture *takeawayFixture) {
	opened := fixture.open(t, fixture.waiter, map[string]any{"mode": "takeaway", "origin": operations.TakeawayOrigin}, draftLine(fixture.product, 1))
	id := opened.Account.ID
	route := "restaurant/accounts/" + id + "/customer"
	before := fixture.head(t)
	key := randomID()
	body := map[string]any{"operation_id": key, "customer_name": " Juan Pérez ", "phone": " 555 987 6543 "}
	first := fixture.call(t, fixture.waiter, "POST", route, body, 200)
	saved := decodeResponse[operations.Bill](t, first)
	if saved.Account.CustomerName != "Juan Pérez" || saved.Account.Phone != "555 987 6543" || saved.Account.Revision != opened.Account.Revision ||
		len(saved.Draft) != 1 {
		t.Fatalf("datos del cliente: %+v", saved.Account)
	}
	// Su evento en el ámbito takeaway, su historia y su Registro de movimientos, en la misma operación.
	if fixture.eventsOf(t, before, id, "restaurant.account.updated", "takeaway") != 1 ||
		fixture.count(t, "SELECT count(*)::int FROM app.restaurant_events WHERE account_id=$1 AND kind='account.updated' AND operation_id=$2", id, key) != 1 ||
		fixture.count(t, `SELECT count(*)::int FROM app.audit_events WHERE action='restaurant.account.updated' AND entity_id=$1
            AND after_state->>'customer_name'='Juan Pérez'`, id) != 1 {
		t.Fatal("el cambio de datos del cliente no quedó registrado ni se avisó")
	}
	// Repetir con la misma clave devuelve lo mismo sin registrar otra vez; con otro contenido, IDEMPOTENCY_CONFLICT.
	if again := fixture.call(t, fixture.waiter, "POST", route, body, 200); canonical(t, again) != canonical(t, first) ||
		fixture.eventsOf(t, before, id, "restaurant.account.updated", "takeaway") != 1 {
		t.Fatal("el reintento no devolvió lo mismo o se registró dos veces")
	}
	if code, response := fixture.raw(fixture.waiter, "", "POST", route, map[string]any{"operation_id": key, "customer_name": "Otro",
		"phone": ""}); code != 409 || errorCode(t, response) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("misma clave con otro contenido: %d %s", code, response)
	}
	// Sin revisión: el borrador se sigue guardando con la revisión que el teléfono tenía antes de cambiar los datos.
	fixture.call(t, fixture.waiter, "PUT", "restaurant/accounts/"+id+"/draft", map[string]any{
		"operation_id": randomID(), "revision": opened.Account.Revision,
		"items": []operations.DraftItem{opened.Draft[0], draftLine(fixture.fries, 1)},
	}, 200)
	// Lo mismo otra vez no registra nada; vacío quita los datos.
	fixture.call(t, fixture.waiter, "POST", route, map[string]any{"operation_id": randomID(), "customer_name": "Juan Pérez",
		"phone": "555 987 6543"}, 200)
	if fixture.eventsOf(t, before, id, "restaurant.account.updated", "takeaway") != 1 {
		t.Fatal("sin cambios no debe registrarse nada")
	}
	cleared := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "POST", route, map[string]any{
		"operation_id": randomID(), "customer_name": "", "phone": "",
	}, 200))
	if cleared.Account.CustomerName != "" || cleared.Account.Phone != "" || cleared.Account.Name != "Pedido para llevar" ||
		len(cleared.Draft) != 2 {
		t.Fatalf("quitar los datos del cliente: %+v", cleared.Account)
	}
	// Forma: los dos campos, con los límites de AccountInput.
	for name, invalid := range map[string]map[string]any{
		"sin teléfono":   {"customer_name": "Juan"},
		"sin cliente":    {"phone": "555"},
		"cliente largo":  {"customer_name": strings.Repeat("J", 101), "phone": ""},
		"teléfono largo": {"customer_name": "", "phone": strings.Repeat("5", 41)},
		"con revisión":   {"customer_name": "Juan", "phone": "", "revision": opened.Account.Revision},
	} {
		invalid["operation_id"] = randomID()
		if code, response := fixture.raw(fixture.waiter, "", "POST", route, invalid); code != 400 {
			t.Fatalf("datos del cliente inválidos (%s): %d %s", name, code, response)
		}
	}
	// Solo pedidos para llevar: una cuenta de mesa responde 400 sin cambiar nada.
	dineIn := fixture.open(t, fixture.waiter, nil, draftLine(fixture.fries, 1))
	code, response := fixture.raw(fixture.waiter, "", "POST", "restaurant/accounts/"+dineIn.Account.ID+"/customer",
		map[string]any{"operation_id": randomID(), "customer_name": "Juan", "phone": ""})
	if code != 400 || errorCode(t, response) != "OPERATION_VALIDATION" || fixture.refresh(t, dineIn.Account.ID).Account.CustomerName != "" {
		t.Fatalf("datos del cliente en una cuenta de mesa: %d %s", code, response)
	}
	// Sin takeaway.manage: 403.
	if code, response := fixture.raw(fixture.other, "", "POST", route, map[string]any{"operation_id": randomID(),
		"customer_name": "Beto", "phone": ""}); code != 403 || errorCode(t, response) != "PERMISSION_DENIED" {
		t.Fatalf("datos del cliente sin takeaway.manage: %d %s", code, response)
	}
	// Un pedido que ya no está abierto (descartado en la caja): el rechazo de siempre con details.reason.
	discarded := fixture.open(t, fixture.waiter, map[string]any{"mode": "takeaway", "origin": operations.TakeawayOrigin}, draftLine(fixture.fries, 1))
	fixture.mutate(t, "POST", "restaurant/accounts/"+discarded.Account.ID+"/discard", map[string]any{"revision": discarded.Account.Revision}, 200)
	fixture.expectNotOpen(t, "cambiar los datos del cliente", "POST", "restaurant/accounts/"+discarded.Account.ID+"/customer",
		map[string]any{"operation_id": randomID(), "customer_name": "Juan", "phone": ""}, discarded.Account.ID, "voided",
		"ya se canceló o se liberó")
}

func testStageA2TakeawayPendingAndDelivery(t *testing.T, fixture *takeawayFixture) {
	order := decodeResponse[operations.OfflineOrderResult](t, fixture.raw2xx(t, fixture.phone, fixture.credential, offlineBody(randomID(),
		fixture.catalogVersion(t), takeawayDestination(map[string]any{"customer_name": "Marta"}), offlineItem(fixture.fries.ID, 2, "40.00"))))
	id := order.AccountID
	pending := fixture.pendingTakeaway(t, id)
	if pending == nil || pending.CustomerName != "Marta" || pending.Origin != operations.TakeawayOrigin || pending.Folio < 1 ||
		pending.PreparationState != "released" || pending.PaymentState != "not_recorded" || pending.DeliveryState != "not_delivered" {
		t.Fatalf("pendiente recién enviado: %+v", pending)
	}
	// restaurant/state también lo trae, con los mismos estados.
	state := decodeResponse[struct {
		User struct {
			Permissions []string `json:"permissions"`
		} `json:"user"`
		Accounts []operations.Account `json:"accounts"`
	}](t, fixture.call(t, fixture.waiter, "GET", "restaurant/state", nil, 200))
	found := slices.ContainsFunc(state.Accounts, func(account operations.Account) bool {
		return account.ID == id && account.Mode == "takeaway" && account.PaymentState == "not_recorded" && account.DeliveryState == "not_delivered"
	})
	if !found || !slices.Contains(state.User.Permissions, "takeaway.deliver") || slices.Contains(state.User.Permissions, "payments.read") {
		t.Fatalf("restaurant/state del mesero: %v", state.User.Permissions)
	}
	// Cocina avanza: «En preparación» y «Listo» se ven en la lista y llegan por el canal en el ámbito takeaway.
	after := fixture.head(t)
	fixture.mutateAs(t, fixture.cook, "POST", "kitchen/states", map[string]any{"state": "preparing", "tasks": fixture.taskReferences(t, id)})
	if pending := fixture.pendingTakeaway(t, id); pending == nil || pending.PreparationState != "preparing" {
		t.Fatalf("pendiente en preparación: %+v", pending)
	}
	fixture.mutateAs(t, fixture.cook, "POST", "kitchen/states", map[string]any{"state": "ready", "tasks": fixture.taskReferences(t, id)})
	if pending := fixture.pendingTakeaway(t, id); pending == nil || pending.PreparationState != "ready" {
		t.Fatalf("pendiente listo: %+v", pending)
	}
	preparation := 0
	for _, change := range fixture.snapshot(t, fixture.waiter, after) {
		if change.AggregateID != nil && *change.AggregateID == id && change.Kind == kitchen.PreparationChangedKind && change.Scope == "takeaway" {
			preparation++
		}
		if change.Scope == "kitchen" || change.Scope == "printing" {
			t.Fatalf("el mesero recibió un evento de cocina: %+v", change)
		}
	}
	if preparation != 2 {
		t.Fatalf("eventos de preparación del pedido para llevar que ve el mesero: %d", preparation)
	}
	// Sin pagar no se entrega (lo decide Go): BALANCE_PENDING y nada registrado.
	route := "payments/accounts/" + id + "/deliver"
	code, response := fixture.raw(fixture.waiter, "", "POST", route, map[string]any{"operation_id": randomID()})
	if code != 409 || errorCode(t, response) != "BALANCE_PENDING" ||
		fixture.count(t, "SELECT count(*)::int FROM app.takeaway_deliveries WHERE account_id=$1", id) != 0 {
		t.Fatalf("entregar sin pagar: %d %s", code, response)
	}
	// El mesero sin takeaway.deliver tampoco entrega (403), pagado o no.
	if code, response := fixture.raw(fixture.other, "", "POST", route, map[string]any{"operation_id": randomID()}); code != 403 ||
		errorCode(t, response) != "PERMISSION_DENIED" {
		t.Fatalf("entregar sin takeaway.deliver: %d %s", code, response)
	}
	// Se cobra en la caja: pagado y pendiente de entrega.
	fixture.payInFull(t, id)
	if pending := fixture.pendingTakeaway(t, id); pending == nil || pending.PaymentState != "paid" || pending.DeliveryState != "not_delivered" {
		t.Fatalf("pendiente pagado: %+v", pending)
	}
	// «Entregado» desde el teléfono: se registra y el pedido se cierra; el mesero no recibe el cobro (finance null).
	before := fixture.head(t)
	key := randomID()
	first := fixture.call(t, fixture.waiter, "POST", route, map[string]any{"operation_id": key}, 200)
	delivered := decodeResponse[map[string]any](t, first)
	if delivered["delivered"] != true || delivered["closed"] != true || delivered["finance"] != nil {
		t.Fatalf("entrega desde el teléfono: %s", first)
	}
	if again := fixture.call(t, fixture.waiter, "POST", route, map[string]any{"operation_id": key}, 200); canonical(t, again) != canonical(t, first) {
		t.Fatalf("reintento de la entrega: %s", again)
	}
	if code, response := fixture.raw(fixture.waiter, "", "POST", route, map[string]any{"operation_id": randomID()}); code != 409 ||
		errorCode(t, response) != "ALREADY_DELIVERED" {
		t.Fatalf("entregar dos veces: %d %s", code, response)
	}
	if fixture.count(t, "SELECT count(*)::int FROM app.takeaway_deliveries WHERE account_id=$1 AND actor_id=$2 AND device_id=$3",
		id, fixture.waiterID, fixture.phone.deviceID) != 1 || fixture.refresh(t, id).Account.State != "closed" ||
		fixture.pendingTakeaway(t, id) != nil || fixture.eventsOf(t, before, id, "takeaway.delivered", "takeaway") != 1 {
		t.Fatal("la entrega no quedó registrada una sola vez o el pedido no se cerró")
	}
	// La caja (con payments.read) sigue recibiendo el estado financiero completo.
	other := decodeResponse[operations.OfflineOrderResult](t, fixture.raw2xx(t, fixture.waiter, "", offlineBody(randomID(),
		fixture.catalogVersion(t), takeawayDestination(map[string]any{}), offlineItem(fixture.fries.ID, 1, "40.00"))))
	fixture.payInFull(t, other.AccountID)
	cashier := decodeResponse[struct {
		Finance *payments.Finance `json:"finance"`
	}](t, fixture.mutate(t, "POST", "payments/accounts/"+other.AccountID+"/deliver", map[string]any{}, 200))
	if cashier.Finance == nil || cashier.Finance.DeliveryState != "delivered" || cashier.Finance.PaymentState != "paid" {
		t.Fatalf("entrega desde la caja: %+v", cashier.Finance)
	}
	if !slices.ContainsFunc(fixture.snapshot(t, fixture.waiter, before), func(change events.Change) bool {
		return change.Kind == "takeaway.delivered" && change.AggregateID != nil && *change.AggregateID == id
	}) {
		t.Fatal("el mesero no recibe la entrega por el canal")
	}
}
