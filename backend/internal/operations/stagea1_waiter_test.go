//go:build integration

package operations_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
)

// Etapa A1, fase E (servidor del modo Mesero) contra PostgreSQL con la API real:
//   - A1-D11: restaurant.account.preparation_changed en el ámbito restaurant (takeaway para una cuenta para llevar), en la
//     misma transacción que el cambio de cocina, uno solo por cuenta y transacción, y que un mesero con accounts.read y
//     sin permisos de cocina recibe por el canal de eventos;
//   - §6.4: error.details.reason account_not_open (con el estado de la cuenta) en guardar el borrador, la vista previa,
//     enviar a cocina, cancelar y modificar unidades, sin cambiar el estado ni el código;
//   - A1-D6: el pedido sin conexión que sustituye renglones del borrador (replaces_draft_items): quita los propios que
//     siguen como los vio el dispositivo, no toca los de otra terminal, avisa como un guardado del borrador, convierte
//     en cambios confirmables el renglón cambiado, quitado o ya enviado (desde otra terminal o desde el mismo
//     dispositivo), se confirma con la misma clave, no se duplica al reintentar y funciona con la credencial de envío;
//   - event_cursor de restaurant/state.
//
// Usa su propia base desde cero.

type waiterModeFixture struct {
	*restaurantFixture
	server   *httptest.Server
	waiter   testTerminal
	waiterID string
	cook     testTerminal
	station  kitchen.Station
	tables   []operations.Table
}

// waiterPermissions: lo que la Fase 0 pide para el mesero (§4), sin ningún permiso de cocina ni de para llevar.
var waiterPermissions = []string{
	"accounts.read", "accounts.open", "orders.capture", "orders.confirm", "orders.edit", "orders.cancel",
	"catalog.read", "catalog.prices.read", "restaurant.tables.read", "restaurant.areas.read", "events.read",
	"waiter_devices.use",
}

// raw envía la petición con las cabeceras dadas, sin comprobar el estado: sesión solo si la hay y, para la cola del
// teléfono, la credencial de envío en lugar de la sesión.
func (fixture *waiterModeFixture) raw(terminal testTerminal, credential, method, route string, body any) (int, []byte) {
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:53900"
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	if terminal.sessionToken != "" {
		request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	}
	if credential != "" {
		request.Header.Set(administration.OfflineCredentialHeader, credential)
	}
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response.Code, append([]byte{}, response.Body.Bytes()...)
}

func (fixture *waiterModeFixture) head(t *testing.T) int64 {
	t.Helper()
	var head int64
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(max(sequence),0) FROM app.realtime_events").Scan(&head); err != nil {
		t.Fatal(err)
	}
	return head
}

func (fixture *waiterModeFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var value int
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// preparationEvents cuenta los eventos de preparación de la cuenta posteriores al cursor (con su ámbito).
func (fixture *waiterModeFixture) preparationEvents(t *testing.T, after int64, accountID, scope string) int {
	t.Helper()
	return fixture.count(t, `SELECT count(*)::int FROM app.realtime_events WHERE sequence>$1 AND kind=$2
        AND aggregate_id=$3 AND scope=$4`, after, kitchen.PreparationChangedKind, accountID, scope)
}

// snapshot es lo que el canal de eventos entrega a la terminal desde el cursor (la misma lectura que el SSE).
func (fixture *waiterModeFixture) snapshot(t *testing.T, terminal testTerminal, after int64) []events.Change {
	t.Helper()
	return decodeResponse[events.Batch](t, fixture.call(t, terminal, "GET", fmt.Sprintf("events/snapshot?after=%d", after), nil, 200)).Events
}

// taskReferences son las tareas de cocina vigentes de la cuenta, para avanzarlas como lo haría la pantalla de cocina.
func (fixture *waiterModeFixture) taskReferences(t *testing.T, accountID string) []map[string]any {
	t.Helper()
	rows, err := fixture.pool.Query(fixture.ctx, `SELECT t.id::text,t.revision FROM app.kitchen_tasks t
        JOIN app.restaurant_item_units u ON u.id=t.unit_id WHERE u.account_id=$1 AND t.state<>'cancelled' ORDER BY t.id`, accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	references := []map[string]any{}
	for rows.Next() {
		var id string
		var revision int
		if err := rows.Scan(&id, &revision); err != nil {
			t.Fatal(err)
		}
		references = append(references, map[string]any{"id": id, "revision": revision})
	}
	return references
}

// open abre una cuenta con su primer producto (en una mesa o sin ella) y devuelve la cuenta.
func (fixture *waiterModeFixture) open(t *testing.T, terminal testTerminal, body map[string]any, items ...operations.DraftItem) operations.Bill {
	t.Helper()
	request := map[string]any{"operation_id": randomID(), "mode": "dine_in", "items": items}
	for key, value := range body {
		request[key] = value
	}
	return decodeResponse[operations.Bill](t, fixture.call(t, terminal, "POST", "restaurant/accounts", request, 200))
}

// save guarda el borrador completo con la revisión vigente de la cuenta.
func (fixture *waiterModeFixture) save(t *testing.T, terminal testTerminal, accountID string, items ...operations.DraftItem) operations.Bill {
	t.Helper()
	current := fixture.refresh(t, accountID)
	return decodeResponse[operations.Bill](t, fixture.call(t, terminal, "PUT", "restaurant/accounts/"+accountID+"/draft", map[string]any{
		"operation_id": randomID(), "revision": current.Account.Revision, "items": items,
	}, 200))
}

// confirm envía a cocina el borrador de la cuenta («Enviar a cocina»).
func (fixture *waiterModeFixture) confirm(t *testing.T, terminal testTerminal, accountID string) operations.Bill {
	t.Helper()
	route := "restaurant/accounts/" + accountID
	current := fixture.refresh(t, accountID)
	preview := decodeResponse[operations.QuoteResult](t, fixture.call(t, terminal, "GET", route+"/preview", nil, 200))
	return decodeResponse[operations.Bill](t, fixture.call(t, terminal, "POST", route+"/confirm", map[string]any{
		"operation_id": randomID(), "revision": current.Account.Revision, "expected_fingerprint": preview.Fingerprint,
	}, 200))
}

func TestStageA1WaiterMode(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &waiterModeFixture{restaurantFixture: newFixture(t)}
	fixture.seedCatalog(t)
	fixture.server = httptest.NewServer(fixture.service)
	defer fixture.server.Close()
	// Cocina en pantalla (sin impresora) para la categoría del catálogo de prueba: confirmar crea tareas.
	fixture.station = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina A1-E", Active: true, Reception: "kds", TargetMinutes: 12, WarningMinutes: 15, OverdueMinutes: 25,
		Grouping: "inherit", Tracking: "inherit",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare", StationIDs: []string{fixture.station.ID}}, 200)
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón A1-E", Active: true}, 200))
	for index := range 3 {
		fixture.tables = append(fixture.tables, decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: fmt.Sprintf("Mesa A1-E %d", index+1), Shape: "round", X: 20 + index*130, Y: 30,
			Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200)))
	}
	fixture.waiter = inventoryUser(t, fixture.restaurantFixture, "mesero_a1e", waiterPermissions)
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username='mesero_a1e'").Scan(&fixture.waiterID); err != nil {
		t.Fatal(err)
	}
	fixture.cook = inventoryUser(t, fixture.restaurantFixture, "cocina_a1e", []string{
		"kitchen.states.read", "kitchen.states.manage", "kitchen.stations.all", "events.read",
	})
	t.Run("evento de preparación: el mesero sin permisos de cocina lo recibe, uno por cuenta y transacción", func(t *testing.T) {
		testStageA1PreparationEvent(t, fixture)
	})
	t.Run("cuenta que ya no está abierta: details.reason sin cambiar estado ni código", func(t *testing.T) {
		testStageA1AccountNotOpen(t, fixture)
	})
	t.Run("pedido sin conexión que sustituye renglones del borrador", func(t *testing.T) {
		testStageA1ReplacedDraft(t, fixture)
	})
	t.Run("renglones del borrador cambiados, quitados o ya enviados: cambios que el mesero confirma", func(t *testing.T) {
		testStageA1ReplacedDraftChanges(t, fixture)
	})
	t.Run("pedido que sustituye el borrador con la credencial de envío y forma inválida", func(t *testing.T) {
		testStageA1ReplacedDraftCredential(t, fixture)
	})
	t.Run("cursor de eventos de restaurant/state", func(t *testing.T) {
		testStageA1EventCursor(t, fixture)
	})
}

func testStageA1PreparationEvent(t *testing.T, fixture *waiterModeFixture) {
	// El mesero no ve la cocina.
	fixture.call(t, fixture.waiter, "GET", "kitchen/board?state=all", nil, 403)
	// Enviar a cocina dos partidas (tres unidades): un solo evento de preparación, en la transacción de la confirmación.
	first := fixture.open(t, fixture.waiter, map[string]any{"table_id": fixture.tables[0].ID, "seen_account_ids": []string{}},
		draftLine(fixture.fries, 2), draftLine(fixture.product, 1))
	before := fixture.head(t)
	fixture.confirm(t, fixture.waiter, first.Account.ID)
	if count := fixture.preparationEvents(t, before, first.Account.ID, "restaurant"); count != 1 {
		t.Fatalf("eventos de preparación al confirmar: %d", count)
	}
	sameTransaction := fixture.count(t, `SELECT count(*)::int FROM app.realtime_events p JOIN app.realtime_events c
        ON c.created_at=p.created_at AND c.aggregate_id=p.aggregate_id AND c.kind='restaurant.order.confirmed'
        WHERE p.sequence>$1 AND p.kind=$2 AND p.aggregate_id=$3`, before, kitchen.PreparationChangedKind, first.Account.ID)
	if sameTransaction != 1 {
		t.Fatal("el evento de preparación no nació en la transacción de la confirmación")
	}
	second := fixture.open(t, fixture.waiter, map[string]any{"table_id": fixture.tables[1].ID, "seen_account_ids": []string{}},
		draftLine(fixture.fries, 1))
	fixture.confirm(t, fixture.waiter, second.Account.ID)

	// Cocina pasa a «En preparación» las cuatro tareas de las dos cuentas en una sola operación: un evento por cuenta.
	references := append(fixture.taskReferences(t, first.Account.ID), fixture.taskReferences(t, second.Account.ID)...)
	if len(references) != 4 {
		t.Fatalf("tareas de cocina: %d", len(references))
	}
	after := fixture.head(t)
	stream := openEventStream(t, fixture.restaurantFixture, fixture.server, fixture.waiter, after, "", 200)
	defer stream.Body.Close()
	reader := bufio.NewReader(stream.Body)
	readEventData(t, reader)
	fixture.mutateAs(t, fixture.cook, "POST", "kitchen/states", map[string]any{"state": "preparing", "tasks": references})
	for _, account := range []string{first.Account.ID, second.Account.ID} {
		if count := fixture.preparationEvents(t, after, account, "restaurant"); count != 1 {
			t.Fatalf("eventos de preparación de la cuenta %s: %d", account, count)
		}
	}
	// El mesero lo recibe por el canal (SSE), en el ámbito restaurant y sin ningún evento de cocina.
	received := []events.Change{}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) && !slices.ContainsFunc(received, preparationOf(first.Account.ID)) {
		data := readEventData(t, reader)
		received = append(received, decodeResponse[[]events.Change](t, data["events"])...)
	}
	if !slices.ContainsFunc(received, preparationOf(first.Account.ID)) {
		t.Fatalf("el mesero no recibió el evento de preparación por el canal: %+v", received)
	}
	seen := fixture.snapshot(t, fixture.waiter, after)
	preparation := 0
	for _, change := range seen {
		if change.Scope == "kitchen" || change.Scope == "printing" {
			t.Fatalf("el mesero recibió un evento de cocina: %+v", change)
		}
		if change.Kind == kitchen.PreparationChangedKind {
			preparation++
		}
	}
	if preparation != 2 {
		t.Fatalf("eventos de preparación que ve el mesero: %d (%+v)", preparation, seen)
	}
	bill := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "GET", "restaurant/accounts/"+first.Account.ID, nil, 200))
	if bill.Account.PreparationState != "preparing" || bill.Lines[0].PreparationState != "preparing" {
		t.Fatalf("estado que ve el mesero: %s / %s", bill.Account.PreparationState, bill.Lines[0].PreparationState)
	}
	// «Listo» solo de la primera cuenta: un evento suyo y ninguno de la otra.
	ready := fixture.head(t)
	fixture.mutateAs(t, fixture.cook, "POST", "kitchen/states", map[string]any{
		"state": "ready", "tasks": fixture.taskReferences(t, first.Account.ID),
	})
	if fixture.preparationEvents(t, ready, first.Account.ID, "restaurant") != 1 ||
		fixture.preparationEvents(t, ready, second.Account.ID, "restaurant") != 0 {
		t.Fatal("«Listo» debe avisar solo a la cuenta que cambió, una vez")
	}
	if state := fixture.refresh(t, first.Account.ID).Account.PreparationState; state != "ready" {
		t.Fatalf("la cuenta debe estar lista: %s", state)
	}

	// Una cuenta para llevar avisa en el ámbito takeaway: el mesero sin takeaway.read no la recibe.
	takeaway := fixture.open(t, fixture.admin, map[string]any{"mode": "takeaway", "name": "Para llevar A1-E"}, draftLine(fixture.fries, 1))
	beforeTakeaway := fixture.head(t)
	fixture.confirm(t, fixture.admin, takeaway.Account.ID)
	if fixture.preparationEvents(t, beforeTakeaway, takeaway.Account.ID, "takeaway") != 1 ||
		fixture.preparationEvents(t, beforeTakeaway, takeaway.Account.ID, "restaurant") != 0 {
		t.Fatal("la cuenta para llevar debe avisar su preparación en el ámbito takeaway")
	}
	if slices.ContainsFunc(fixture.snapshot(t, fixture.waiter, beforeTakeaway), func(change events.Change) bool {
		return change.AggregateID != nil && *change.AggregateID == takeaway.Account.ID
	}) {
		t.Fatal("el mesero sin takeaway.read recibió un evento de la cuenta para llevar")
	}
}

func preparationOf(accountID string) func(events.Change) bool {
	return func(change events.Change) bool {
		return change.Kind == kitchen.PreparationChangedKind && change.Scope == "restaurant" &&
			change.AggregateID != nil && *change.AggregateID == accountID
	}
}

// mutateAs envía una escritura con un operation_id nuevo desde la terminal dada.
func (fixture *waiterModeFixture) mutateAs(t *testing.T, terminal testTerminal, method, route string, body map[string]any) []byte {
	t.Helper()
	request := map[string]any{"operation_id": randomID()}
	for key, value := range body {
		request[key] = value
	}
	return fixture.call(t, terminal, method, route, request, 200)
}

// accountNotOpenProblem es el rechazo de una operación sobre una cuenta que ya no está abierta.
type accountNotOpenProblem struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details struct {
			Reason       string  `json:"reason"`
			AccountID    string  `json:"account_id"`
			AccountState string  `json:"account_state"`
			MergedIntoID *string `json:"merged_into_id"`
		} `json:"details"`
	} `json:"error"`
}

// expectNotOpen comprueba el rechazo: el 400 OPERATION_VALIDATION de siempre, con el motivo y el estado de la cuenta.
func (fixture *waiterModeFixture) expectNotOpen(t *testing.T, name, method, route string, body any, accountID, state, message string) accountNotOpenProblem {
	t.Helper()
	code, response := fixture.raw(fixture.waiter, "", method, route, body)
	problem := decodeResponse[accountNotOpenProblem](t, response)
	if code != 400 || problem.Error.Code != "OPERATION_VALIDATION" || problem.Error.Details.Reason != operations.AccountNotOpenReason ||
		problem.Error.Details.AccountID != accountID || problem.Error.Details.AccountState != state ||
		!strings.Contains(problem.Error.Message, message) {
		t.Fatalf("%s sobre una cuenta %s: %d %s", name, state, code, response)
	}
	return problem
}

func testStageA1AccountNotOpen(t *testing.T, fixture *waiterModeFixture) {
	// Cancelada (descartada desde otra terminal) mientras el mesero capturaba: guardar el borrador y la vista previa.
	voided := fixture.open(t, fixture.waiter, nil, draftLine(fixture.fries, 1))
	fixture.mutate(t, "POST", "restaurant/accounts/"+voided.Account.ID+"/discard", map[string]any{"revision": voided.Account.Revision}, 200)
	route := "restaurant/accounts/" + voided.Account.ID
	problem := fixture.expectNotOpen(t, "guardar el borrador", "PUT", route+"/draft", map[string]any{
		"operation_id": randomID(), "revision": voided.Account.Revision, "items": []operations.DraftItem{draftLine(fixture.fries, 2)},
	}, voided.Account.ID, "voided", "ya se canceló o se liberó")
	if problem.Error.Details.MergedIntoID != nil {
		t.Fatalf("una cuenta cancelada no se combinó: %v", *problem.Error.Details.MergedIntoID)
	}
	fixture.expectNotOpen(t, "la vista previa", "GET", route+"/preview", nil, voided.Account.ID, "voided", "ya se canceló o se liberó")

	// Combinada en otra desde otra terminal: enviar a cocina, cancelar y modificar unidades.
	source := fixture.open(t, fixture.waiter, nil, draftLine(fixture.fries, 2))
	source = fixture.confirm(t, fixture.waiter, source.Account.ID)
	source = fixture.save(t, fixture.waiter, source.Account.ID, draftLine(fixture.product, 1))
	target := fixture.open(t, fixture.waiter, nil, draftLine(fixture.fries, 1))
	fixture.mutate(t, "POST", "restaurant/accounts/"+source.Account.ID+"/merge", map[string]any{
		"revision": source.Account.Revision, "target_account_id": target.Account.ID, "target_revision": target.Account.Revision,
	}, 200)
	route = "restaurant/accounts/" + source.Account.ID
	version := source.Lines[0].VersionID
	line := catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 2, Customization: catalog.Customization{Notes: "Sin sal"}}
	merged := fixture.expectNotOpen(t, "enviar a cocina", "POST", route+"/confirm", map[string]any{
		"operation_id": randomID(), "revision": source.Account.Revision, "expected_fingerprint": "x",
	}, source.Account.ID, "merged", "se combinó con otra cuenta")
	if merged.Error.Details.MergedIntoID == nil || *merged.Error.Details.MergedIntoID != target.Account.ID {
		t.Fatalf("la cuenta combinada debe decir en cuál: %+v", merged.Error.Details)
	}
	fixture.expectNotOpen(t, "cancelar unidades", "POST", route+"/cancel", map[string]any{
		"operation_id": randomID(), "revision": source.Account.Revision, "version_id": version, "quantity": 1,
		"reason": "El cliente ya no las quiso",
	}, source.Account.ID, "merged", "se combinó con otra cuenta")
	fixture.expectNotOpen(t, "la vista previa de modificar", "POST", route+"/edit-preview", map[string]any{
		"revision": source.Account.Revision, "version_id": version, "line": line,
	}, source.Account.ID, "merged", "se combinó con otra cuenta")
	fixture.expectNotOpen(t, "modificar unidades", "POST", route+"/edit", map[string]any{
		"operation_id": randomID(), "revision": source.Account.Revision, "version_id": version, "line": line,
		"expected_fingerprint": "x", "reason": "El cliente las pidió sin sal",
	}, source.Account.ID, "merged", "se combinó con otra cuenta")

	// Cobrada mientras tanto (el cierre por el cobro se simula: aquí importa el rechazo, no el cobro).
	closed := fixture.open(t, fixture.waiter, nil, draftLine(fixture.fries, 1))
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.restaurant_accounts SET state='closed',closed_at=now(),revision=revision+1
        WHERE id=$1`, closed.Account.ID); err != nil {
		t.Fatal(err)
	}
	fixture.expectNotOpen(t, "guardar el borrador", "PUT", "restaurant/accounts/"+closed.Account.ID+"/draft", map[string]any{
		"operation_id": randomID(), "revision": closed.Account.Revision + 1, "items": []operations.DraftItem{draftLine(fixture.fries, 1)},
	}, closed.Account.ID, "closed", "«"+closed.Account.Name+"» ya se cobró")
	// En el pedido sin conexión el mismo caso es account_closed, con el estado en now.
	code, response := fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", offlineBody(randomID(), fixture.catalogVersion(t),
		map[string]any{"account_id": closed.Account.ID}, offlineItem(fixture.fries.ID, 1, "40.00")))
	rejection := decodeResponse[offlineRejection](t, response)
	changes := rejection.Error.Details.Changes
	if code != 409 || len(changes) != 1 || changes[0].Kind != operations.OfflineAccountClosed || changes[0].Now == nil ||
		*changes[0].Now != "closed" || !strings.Contains(changes[0].Message, "ya se cobró") {
		t.Fatalf("pedido sin conexión a una cuenta cobrada: %d %s", code, response)
	}
}

// catalogVersion es la versión vigente del catálogo.
func (fixture *waiterModeFixture) catalogVersion(t *testing.T) int64 {
	t.Helper()
	return int64(fixture.count(t, "SELECT coalesce(max(version),0)::int FROM app.catalog_change_log"))
}

// seenItem es la partida del pedido sin conexión con el mismo id que el renglón del borrador que sustituye.
func seenItem(item operations.DraftItem, quantity int, price string) map[string]any {
	return map[string]any{
		"id": item.ID, "product_id": item.ProductID, "variant_id": item.VariantID, "quantity": quantity,
		"customization": item.Customization, "seen_unit_price": price,
	}
}

func draftIDs(bill *operations.Bill) []string {
	ids := []string{}
	for _, item := range bill.Draft {
		ids = append(ids, item.ID)
	}
	slices.Sort(ids)
	return ids
}

func sortedIDs(ids ...string) []string {
	sorted := append([]string{}, ids...)
	slices.Sort(sorted)
	return sorted
}

func testStageA1ReplacedDraft(t *testing.T, fixture *waiterModeFixture) {
	// El mesero abre la cuenta y guarda en línea dos renglones; otra terminal agrega un tercero.
	mine, mineToo := draftLine(fixture.fries, 2), draftLine(fixture.product, 1)
	opened := fixture.open(t, fixture.waiter, map[string]any{"table_id": fixture.tables[2].ID, "seen_account_ids": []string{}}, mine)
	accountID := opened.Account.ID
	fixture.save(t, fixture.waiter, accountID, mine, mineToo)
	theirs := draftLine(fixture.fries, 1)
	theirs.Customization.Notes = "Para compartir"
	fixture.save(t, fixture.admin, accountID, mine, mineToo, theirs)
	// Se corta la red: el mesero sube a 3 las papas, quita el filete y agrega otro producto; el pedido sustituye los dos
	// renglones que había guardado, tal como los vio.
	extra := operations.DraftItem{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.product.ID, Quantity: 1}}
	key := randomID()
	body := offlineBody(key, fixture.catalogVersion(t), map[string]any{"account_id": accountID},
		seenItem(mine, 3, "40.00"), seenItem(extra, 1, "150.00"))
	body["replaces_draft_items"] = []operations.DraftItem{mine, mineToo}
	before := fixture.head(t)
	audits := fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='restaurant.draft.saved' AND entity_id=$1", accountID)
	response := fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders", body, 200)
	result := decodeResponse[operations.OfflineOrderResult](t, response)
	if !slices.Equal(result.RemovedDraftItemIDs, sortedIDs(mine.ID, mineToo.ID)) || !slices.Equal(draftIDs(result.Bill), []string{theirs.ID}) ||
		result.Bill.Draft[0].Customization.Notes != "Para compartir" || len(result.AcceptedChanges) != 0 {
		t.Fatalf("pedido que sustituye el borrador: quitados %v, borrador %+v", result.RemovedDraftItemIDs, result.Bill.Draft)
	}
	// Cada partida confirmada conserva el id con que se capturó.
	if fixture.count(t, `SELECT count(*)::int FROM app.restaurant_items i JOIN app.restaurant_orders o ON o.id=i.original_order_id
        WHERE o.operation_id=$1 AND i.draft_item_id = ANY($2::uuid[])`, key, []string{mine.ID, extra.ID}) != 2 {
		t.Fatal("las partidas confirmadas no conservan el id del renglón")
	}
	// Las demás terminales se enteran como de un guardado del borrador: historia, evento y Registro de movimientos.
	if fixture.count(t, `SELECT count(*)::int FROM app.restaurant_events WHERE account_id=$1 AND kind='draft.saved'
        AND operation_id=$2`, accountID, key) != 1 ||
		fixture.count(t, `SELECT count(*)::int FROM app.realtime_events WHERE sequence>$1 AND kind='restaurant.draft.saved'
        AND aggregate_id=$2`, before, accountID) != 1 ||
		fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='restaurant.draft.saved' AND entity_id=$1", accountID) != audits+1 {
		t.Fatal("quitar los renglones del borrador no se publicó como un guardado del borrador")
	}
	var audited map[string]any
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT after_state FROM app.audit_events
        WHERE action='restaurant.offline_order.accepted' AND entity_id=$1`, key).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if removed, _ := audited["removed_draft_item_ids"].([]any); len(removed) != 2 {
		t.Fatalf("la auditoría del pedido no dice qué renglones quitó: %v", audited)
	}
	// Reintento (respuesta perdida): la misma respuesta, sin otro pedido ni otro guardado del borrador.
	again := fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders", body, 200)
	if canonical(t, again) != canonical(t, response) ||
		fixture.count(t, "SELECT count(*)::int FROM app.restaurant_orders WHERE operation_id=$1", key) != 1 ||
		fixture.count(t, "SELECT count(*)::int FROM app.restaurant_events WHERE kind='draft.saved' AND operation_id=$1", key) != 1 ||
		!slices.Equal(draftIDs(&operations.Bill{Draft: fixture.refresh(t, accountID).Draft}), []string{theirs.ID}) {
		t.Fatal("el reintento repitió el pedido o tocó el borrador")
	}
	// La misma clave con otro contenido después de aceptado: el pedido que sí entró, sin repetir nada.
	changed := map[string]any{}
	for name, value := range body {
		changed[name] = value
	}
	changed["replaces_draft_items"] = []operations.DraftItem{mine}
	code, late := fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", changed)
	if problem := decodeResponse[alreadyAccepted](t, late); code != 409 || problem.Error.Code != "OFFLINE_ORDER_ALREADY_ACCEPTED" ||
		problem.Error.Details.Result.OrderID != result.OrderID {
		t.Fatalf("misma clave con otro contenido: %d %s", code, late)
	}
}

// replacedChanges lee el 409 con los cambios del pedido y los ordena por renglón.
func replacedChanges(t *testing.T, response []byte) (offlineRejection, map[string]operations.OfflineChange) {
	t.Helper()
	rejection := decodeResponse[offlineRejection](t, response)
	byItem := map[string]operations.OfflineChange{}
	for _, change := range rejection.Error.Details.Changes {
		if change.ItemID != nil {
			byItem[*change.ItemID] = change
		}
	}
	return rejection, byItem
}

func testStageA1ReplacedDraftChanges(t *testing.T, fixture *waiterModeFixture) {
	accountID := fixture.open(t, fixture.waiter, nil, draftLine(fixture.fries, 1)).Account.ID
	// El mesero guarda en línea tres renglones; mientras no tiene conexión, otra terminal quita uno y envía a cocina el
	// borrador (con otro); después el mesero había guardado un cuarto que la otra terminal cambia.
	removed, sent := draftLine(fixture.product, 1), draftLine(fixture.fries, 2)
	opening := fixture.refresh(t, accountID).Draft[0]
	fixture.save(t, fixture.waiter, accountID, opening, removed, sent)
	fixture.save(t, fixture.admin, accountID, opening, sent)
	fixture.confirm(t, fixture.admin, accountID)
	changedLine := draftLine(fixture.fries, 1)
	fixture.save(t, fixture.waiter, accountID, changedLine)
	theirs := changedLine
	theirs.Customization = catalog.Customization{Notes: "Sin sal"}
	fixture.save(t, fixture.admin, accountID, theirs)
	key := randomID()
	body := offlineBody(key, fixture.catalogVersion(t), map[string]any{"account_id": accountID},
		seenItem(changedLine, 1, "40.00"), seenItem(removed, 1, "150.00"), seenItem(sent, 2, "40.00"))
	body["replaces_draft_items"] = []operations.DraftItem{changedLine, removed, sent}
	orders := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_orders WHERE original_account_id=$1", accountID)
	code, response := fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", body)
	rejection, byItem := replacedChanges(t, response)
	expected := map[string]struct{ kind, message string }{
		changedLine.ID: {operations.OfflineDraftItemChanged, "Papas producto: lo cambiaron en otra terminal"},
		removed.ID:     {operations.OfflineDraftItemRemoved, "Filete: lo quitaron de la cuenta en otra terminal"},
		sent.ID:        {operations.OfflineDraftItemSent, "Papas producto: ya se envió desde otra terminal"},
	}
	if code != 409 || rejection.Error.Code != "ORDER_REQUIRES_CONFIRMATION" || rejection.Error.Details.RequiresChanges ||
		len(rejection.Error.Details.Changes) != 3 {
		t.Fatalf("renglones que ya no están como los vio el dispositivo: %d %s", code, response)
	}
	for id, want := range expected {
		change := byItem[id]
		if change.Kind != want.kind || !change.Acceptable || !strings.HasPrefix(change.Message, want.message) ||
			change.ProductID == nil || change.Before != nil || change.Now != nil {
			t.Fatalf("cambio del renglón %s: %+v", id, change)
		}
	}
	// No se aceptó en silencio: nada cambió, tampoco el renglón de la otra terminal.
	current := fixture.refresh(t, accountID)
	if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_orders WHERE original_account_id=$1", accountID) != orders ||
		len(current.Draft) != 1 || current.Draft[0].Customization.Notes != "Sin sal" {
		t.Fatalf("el rechazo cambió la cuenta: %+v", current.Draft)
	}
	// El mesero acepta los cambios: con la misma clave entra su pedido completo y el renglón de la otra terminal queda.
	body["confirmation"] = map[string]any{"catalog_version": rejection.Error.Details.CatalogVersion, "changes": rejection.Error.Details.Changes}
	result := decodeResponse[operations.OfflineOrderResult](t, fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders", body, 200))
	if len(result.RemovedDraftItemIDs) != 0 || len(result.AcceptedChanges) != 3 || len(result.Bill.Draft) != 1 ||
		result.Bill.Draft[0].Customization.Notes != "Sin sal" ||
		fixture.count(t, "SELECT count(*)::int FROM app.restaurant_events WHERE kind='draft.saved' AND operation_id=$1", key) != 0 {
		t.Fatalf("pedido confirmado: quitados %v, cambios %d, borrador %+v", result.RemovedDraftItemIDs, len(result.AcceptedChanges), result.Bill.Draft)
	}
	if fixture.count(t, `SELECT count(*)::int FROM app.restaurant_item_units u JOIN app.restaurant_items i ON i.id=u.item_id
        JOIN app.restaurant_orders o ON o.id=i.original_order_id WHERE o.operation_id=$1`, key) != 4 {
		t.Fatal("el pedido confirmado no entró completo")
	}

	// Ya enviado desde el mismo dispositivo (por ejemplo, «Enviar a cocina» en línea antes de perder la señal).
	own := draftLine(fixture.fries, 1)
	fixture.save(t, fixture.waiter, accountID, own)
	fixture.confirm(t, fixture.waiter, accountID)
	repeat := offlineBody(randomID(), fixture.catalogVersion(t), map[string]any{"account_id": accountID}, seenItem(own, 1, "40.00"))
	repeat["replaces_draft_items"] = []operations.DraftItem{own}
	code, response = fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", repeat)
	_, byItem = replacedChanges(t, response)
	if code != 409 || byItem[own.ID].Kind != operations.OfflineDraftItemSent ||
		!strings.HasPrefix(byItem[own.ID].Message, "Papas producto: ya se había enviado desde este dispositivo") {
		t.Fatalf("renglón ya enviado desde el mismo dispositivo: %d %s", code, response)
	}
}

// waiterPhone registra un teléfono de meseros como si se hubiera emparejado (el emparejamiento se prueba aparte).
func (fixture *waiterModeFixture) waiterPhone(t *testing.T) testTerminal {
	t.Helper()
	secret := administration.NewToken()
	var id string
	err := fixture.pool.QueryRow(fixture.ctx, `INSERT INTO app.devices(name,secret_hash,kind,os,app_version,paired_at)
        VALUES ('Tableta A1-E',$1,'android','Android 15','0.1.0',now()) RETURNING id`, administration.TokenHash(secret)).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return testTerminal{deviceID: id, deviceToken: secret}
}

func testStageA1ReplacedDraftCredential(t *testing.T, fixture *waiterModeFixture) {
	// El mesero entra con PIN en una tableta: sesión y credencial de envío.
	fixture.adminCall(t, "PUT", "users/"+fixture.waiterID+"/pin", map[string]any{"pin": "4829", "short_name": "Ana", "badge_color": "#3591d4"}, 200)
	phone := fixture.waiterPhone(t)
	code, response := fixture.raw(phone, "", "POST", "auth/pin-login", map[string]string{"user_id": fixture.waiterID, "pin": "4829"})
	login := decodeResponse[struct {
		Session    string `json:"session_token"`
		Credential string `json:"offline_credential"`
	}](t, response)
	if code != 200 || login.Session == "" || login.Credential == "" {
		t.Fatalf("entrada con PIN: %d %s", code, response)
	}
	tablet := phone
	tablet.sessionToken = login.Session
	saved := draftLine(fixture.fries, 1)
	accountID := fixture.open(t, tablet, nil, saved).Account.ID
	// Sin señal la cola envía el pedido con la credencial (sin sesión); sustituye el renglón guardado en línea.
	key := randomID()
	body := offlineBody(key, fixture.catalogVersion(t), map[string]any{"account_id": accountID}, seenItem(saved, 2, "40.00"))
	body["replaces_draft_items"] = []operations.DraftItem{saved}
	for attempt := range 2 {
		code, response = fixture.raw(phone, login.Credential, "POST", "restaurant/offline-orders", body)
		result := decodeResponse[operations.OfflineOrderResult](t, response)
		if code != 200 || !slices.Equal(result.RemovedDraftItemIDs, []string{saved.ID}) || len(result.Bill.Draft) != 0 {
			t.Fatalf("envío %d con la credencial de envío: %d %s", attempt+1, code, response)
		}
	}
	if fixture.count(t, `SELECT count(*)::int FROM app.restaurant_orders WHERE operation_id=$1 AND author_id=$2 AND device_id=$3`,
		key, fixture.waiterID, phone.deviceID) != 1 {
		t.Fatal("el pedido con la credencial debe entrar una vez, a nombre del mesero y de la tableta")
	}

	// Forma inválida (400 sin escribir nada): con una mesa como destino, renglones repetidos, de más o sin cantidad.
	version := fixture.catalogVersion(t)
	item := offlineItem(fixture.fries.ID, 1, "40.00")
	line := draftLine(fixture.fries, 1)
	zero := line
	zero.Quantity = 0
	many := []operations.DraftItem{}
	for range 21 {
		many = append(many, draftLine(fixture.fries, 1))
	}
	cases := map[string]struct {
		destination map[string]any
		replaced    []operations.DraftItem
	}{
		"mesa":      {map[string]any{"table_id": fixture.tables[0].ID}, []operations.DraftItem{line}},
		"repetidos": {map[string]any{"account_id": accountID}, []operations.DraftItem{line, line}},
		"de más":    {map[string]any{"account_id": accountID}, many},
		"cantidad":  {map[string]any{"account_id": accountID}, []operations.DraftItem{zero}},
	}
	for name, option := range cases {
		body := offlineBody(randomID(), version, option.destination, item)
		body["replaces_draft_items"] = option.replaced
		if code, response := fixture.raw(fixture.waiter, "", "POST", "restaurant/offline-orders", body); code != 400 {
			t.Fatalf("renglones del borrador inválidos (%s): %d %s", name, code, response)
		}
	}
}

func testStageA1EventCursor(t *testing.T, fixture *waiterModeFixture) {
	state := decodeResponse[struct {
		EventCursor *int64 `json:"event_cursor"`
	}](t, fixture.call(t, fixture.waiter, "GET", "restaurant/state", nil, 200))
	if state.EventCursor == nil || *state.EventCursor != fixture.head(t) {
		t.Fatalf("cursor de eventos del estado: %v (último %d)", state.EventCursor, fixture.head(t))
	}
	// Desde ese cursor el canal entrega lo que pasó después de leer el estado, y nada de antes.
	if pending := fixture.snapshot(t, fixture.waiter, *state.EventCursor); len(pending) != 0 {
		t.Fatalf("eventos anteriores al estado: %+v", pending)
	}
	opened := fixture.open(t, fixture.waiter, nil, draftLine(fixture.fries, 1))
	if !slices.ContainsFunc(fixture.snapshot(t, fixture.waiter, *state.EventCursor), func(change events.Change) bool {
		return change.AggregateID != nil && *change.AggregateID == opened.Account.ID
	}) {
		t.Fatal("el canal no entregó desde el cursor lo ocurrido después del estado")
	}
}
