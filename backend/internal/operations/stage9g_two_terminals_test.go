//go:build integration

package operations_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa 9G, fase C (§4.3): varias terminales a la vez contra la API real y PostgreSQL. Dos cajas («Caja Norte 9G-C»
// y «Caja Sur 9G-C»), cada una con su terminal, su usuario y su caja:
//   - los cambios de una llegan a la otra por el canal de eventos (SSE) y por la consulta del estado: mesa ocupada y
//     liberada, cuenta, envío a cocina, estado de preparación, cobro y turnos de caja;
//   - las dos modificando la misma cuenta: ninguna sobrescribe en silencio (409 ACCOUNT_CONFLICT con el aviso de Go)
//     y la segunda lee la cuenta actualizada; dos confirmaciones simultáneas dejan un solo envío a cocina;
//   - cada caja con su propio turno: el arqueo de cierre pausa solo la suya, el efectivo cuenta por caja y el tope se
//     compara con el esperado de cada caja por separado.
//
// Usa su propia base desde cero.

type twoTerminalsFixture struct {
	*paymentsFixture
	server  *httptest.Server
	north   testTerminal
	south   testTerminal
	regions map[string]string
	shifts  map[string]string
	tables  []operations.Table
	station kitchen.Station
}

// twoTerminalPermissions: cajero con mesas, captura, cocina, cobros, caja y eventos.
var twoTerminalPermissions = append([]string{
	"accounts.open", "orders.capture", "orders.confirm", "catalog.read", "catalog.prices.read", "restaurant.tables.read",
	"events.read", "kitchen.states.read", "kitchen.states.manage", "kitchen.stations.all", "cash.expected.read",
}, cashierPermissions...)

// rawCall envía una petición sin comprobar el estado (también desde goroutines).
func (fixture *twoTerminalsFixture) rawCall(terminal testTerminal, method, route string, body any) (int, []byte) {
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:55900"
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response.Code, append([]byte{}, response.Body.Bytes()...)
}

// write envía una escritura con un operation_id nuevo y comprueba el estado.
func (fixture *twoTerminalsFixture) write(t *testing.T, terminal testTerminal, method, route string, body map[string]any, status int) []byte {
	t.Helper()
	copied := map[string]any{"operation_id": randomID()}
	for key, value := range body {
		copied[key] = value
	}
	return fixture.call(t, terminal, method, route, copied, status)
}

func (fixture *twoTerminalsFixture) head(t *testing.T) int64 {
	t.Helper()
	var head int64
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(max(sequence),0) FROM app.realtime_events").Scan(&head); err != nil {
		t.Fatal(err)
	}
	return head
}

// streamedChange es un evento que recibió una terminal por su canal SSE.
type streamedChange struct {
	Kind        string  `json:"kind"`
	Scope       string  `json:"scope"`
	AggregateID *string `json:"aggregate_id"`
}

// waitEvents abre el canal de eventos de la terminal desde el cursor y lee hasta que llega un evento que cumple cada
// condición (o vence el plazo). Devuelve todo lo recibido.
func (fixture *twoTerminalsFixture) waitEvents(t *testing.T, terminal testTerminal, after int64, wanted ...func(streamedChange) bool) []streamedChange {
	t.Helper()
	request, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/events?after=%d", fixture.server.URL, after), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("canal de eventos: %d", response.StatusCode)
	}
	received := []streamedChange{}
	satisfied := func() bool {
		for _, condition := range wanted {
			found := false
			for _, change := range received {
				if condition(change) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	deadline := time.AfterFunc(8*time.Second, func() {
		response.Body.Close()
	})
	defer deadline.Stop()
	reader := bufio.NewReader(response.Body)
	for !satisfied() {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("la otra terminal no recibió los eventos esperados; recibió %+v", received)
		}
		data, found := strings.CutPrefix(strings.TrimSpace(line), "data: ")
		if !found {
			continue
		}
		var batch struct {
			Events []streamedChange `json:"events"`
		}
		if err := json.Unmarshal([]byte(data), &batch); err != nil {
			t.Fatalf("mensaje del canal inválido: %s", data)
		}
		received = append(received, batch.Events...)
	}
	return received
}

func aggregate(scope, id string) func(streamedChange) bool {
	return func(change streamedChange) bool {
		return change.Scope == scope && change.AggregateID != nil && *change.AggregateID == id
	}
}

func kind(name string) func(streamedChange) bool {
	return func(change streamedChange) bool {
		return change.Kind == name
	}
}

// recovery es el estado que consulta una terminal al reconectar (GET restaurant/state).
func (fixture *twoTerminalsFixture) recovery(t *testing.T, terminal testTerminal) operations.RecoveryView {
	t.Helper()
	return decodeResponse[operations.RecoveryView](t, fixture.call(t, terminal, "GET", "restaurant/state", nil, 200))
}

func tableState(view operations.RecoveryView, id string) string {
	for _, table := range view.Tables {
		if table.ID == id {
			return table.State
		}
	}
	return "no está"
}

func errorOf(t *testing.T, body []byte) (string, string) {
	t.Helper()
	var problem struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &problem); err != nil {
		t.Fatalf("error inválido: %s", body)
	}
	return problem.Error.Code, problem.Error.Message
}

func draftLine(product catalog.Product, quantity int) operations.DraftItem {
	return operations.DraftItem{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: product.ID, Quantity: quantity}}
}

func TestStage9GTwoTerminals(t *testing.T) {
	resetStage9GDatabase(t)
	base := newFixture(t)
	base.seedCatalog(t)
	fixture := &twoTerminalsFixture{
		paymentsFixture: &paymentsFixture{restaurantFixture: base, methods: map[string]payments.Method{}, registers: map[string]string{}},
		regions:         map[string]string{}, shifts: map[string]string{},
	}
	fixture.server = httptest.NewServer(fixture.service)
	defer fixture.server.Close()
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil {
			fixture.methods[*method.Code] = method
		}
	}
	// Cocina en pantalla (sin impresora) para la categoría del catálogo de prueba.
	fixture.station = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina 9G-C", Active: true, Reception: "kds", TargetMinutes: 12, WarningMinutes: 15, OverdueMinutes: 25,
		Grouping: "inherit", Tracking: "inherit",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare", StationIDs: []string{fixture.station.ID}}, 200)
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón 9G-C", Active: true}, 200))
	for index := range 2 {
		fixture.tables = append(fixture.tables, decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: fmt.Sprintf("Mesa 9G-C %d", index+1), Shape: "rectangle", X: 20 + index*130, Y: 30,
			Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200)))
	}
	for _, name := range []string{"Caja Norte 9G-C", "Caja Sur 9G-C"} {
		fixture.regions[name] = decodeResponse[struct {
			ID string `json:"id"`
		}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": name, "description": "", "active": true}, 200)).ID
	}
	fixture.north = newCashier(t, fixture.paymentsFixture, "cajera_norte_9gc", twoTerminalPermissions, fixture.regions["Caja Norte 9G-C"])
	fixture.south = newCashier(t, fixture.paymentsFixture, "cajero_sur_9gc", twoTerminalPermissions, fixture.regions["Caja Sur 9G-C"])

	var accountID string
	t.Run("cada caja abre su propio turno y la otra lo ve", func(t *testing.T) {
		after := fixture.head(t)
		for name, terminal := range map[string]testTerminal{"Caja Norte 9G-C": fixture.north, "Caja Sur 9G-C": fixture.south} {
			shift := decodeResponse[payments.Shift](t, fixture.write(t, terminal, "POST", "payments/shifts", map[string]any{"opening_float": "0"}, 200))
			if shift.State != "open" || shift.RegisterID != fixture.regions[name] {
				t.Fatalf("turno de %s: %+v", name, shift)
			}
			fixture.shifts[name] = shift.ID
		}
		// Cada terminal opera el turno de su caja.
		for name, terminal := range map[string]testTerminal{"Caja Norte 9G-C": fixture.north, "Caja Sur 9G-C": fixture.south} {
			current := decodeResponse[struct {
				Usable bool            `json:"usable"`
				Shift  *payments.Shift `json:"shift"`
			}](t, fixture.call(t, terminal, "GET", "payments/shifts/current", nil, 200))
			if !current.Usable || current.Shift == nil || current.Shift.ID != fixture.shifts[name] {
				t.Fatalf("turno vigente de %s: %+v", name, current)
			}
		}
		// La apertura de la otra caja llega por el canal de eventos (ámbito de caja).
		fixture.waitEvents(t, fixture.south, after, aggregate("cash", fixture.shifts["Caja Norte 9G-C"]))
		fixture.waitEvents(t, fixture.north, after, aggregate("cash", fixture.shifts["Caja Sur 9G-C"]))
		// Una terminal no inicia el arqueo de cierre del turno de otra caja.
		if status, _ := fixture.rawCall(fixture.south, "POST", "payments/shifts/"+fixture.shifts["Caja Norte 9G-C"]+"/closing",
			map[string]any{"operation_id": randomID()}); status == 200 {
			t.Fatal("la Caja Sur inició el arqueo del turno de la Caja Norte")
		}
	})

	t.Run("la mesa que ocupa una terminal aparece ocupada en la otra", func(t *testing.T) {
		after := fixture.head(t)
		opened := decodeResponse[operations.Bill](t, fixture.write(t, fixture.north, "POST", "restaurant/accounts", map[string]any{
			"mode": "dine_in", "table_id": fixture.tables[0].ID, "seen_account_ids": []string{},
			"items": []operations.DraftItem{draftLine(fixture.fries, 1)},
		}, 200))
		accountID = opened.Account.ID
		fixture.waitEvents(t, fixture.south, after, aggregate("restaurant", accountID))
		if state := tableState(fixture.recovery(t, fixture.south), fixture.tables[0].ID); state != "occupied" {
			t.Fatalf("la otra terminal ve la mesa %s", state)
		}
		// La otra terminal que intenta abrir otra cuenta en la misma mesa recibe el aviso con la cuenta existente.
		code, body := fixture.rawCall(fixture.south, "POST", "restaurant/accounts", map[string]any{
			"operation_id": randomID(), "mode": "dine_in", "table_id": fixture.tables[0].ID, "seen_account_ids": []string{},
			"items": []operations.DraftItem{draftLine(fixture.fries, 1)},
		})
		if problem, _ := errorOf(t, body); code != 409 || problem != "TABLE_HAS_ACCOUNT" || !strings.Contains(string(body), accountID) {
			t.Fatalf("mesa ya ocupada por la otra terminal: %d %s", code, body)
		}
	})

	t.Run("dos terminales en la misma cuenta no se sobrescriben en silencio", func(t *testing.T) {
		route := "restaurant/accounts/" + accountID
		northView := decodeResponse[operations.Bill](t, fixture.call(t, fixture.north, "GET", route, nil, 200))
		southView := decodeResponse[operations.Bill](t, fixture.call(t, fixture.south, "GET", route, nil, 200))
		if northView.Account.Revision != southView.Account.Revision {
			t.Fatal("las dos terminales deben partir de la misma revisión")
		}
		saved := decodeResponse[operations.Bill](t, fixture.write(t, fixture.north, "PUT", route+"/draft", map[string]any{
			"revision": northView.Account.Revision, "items": []operations.DraftItem{draftLine(fixture.fries, 2), draftLine(fixture.product, 1)},
		}, 200))
		// La segunda terminal guarda con la revisión que vio: Go la rechaza con un aviso claro y nada cambia.
		code, body := fixture.rawCall(fixture.south, "PUT", route+"/draft", map[string]any{
			"operation_id": randomID(), "revision": southView.Account.Revision, "items": []operations.DraftItem{draftLine(fixture.fries, 5)},
		})
		problem, message := errorOf(t, body)
		if code != 409 || problem != "ACCOUNT_CONFLICT" || !strings.Contains(message, "otra terminal") {
			t.Fatalf("la segunda terminal sobrescribió o no recibió el aviso: %d %s", code, body)
		}
		current := decodeResponse[operations.Bill](t, fixture.call(t, fixture.south, "GET", route, nil, 200))
		if current.Account.Revision != saved.Account.Revision || len(current.Draft) != 2 {
			t.Fatalf("la segunda terminal debe ver la cuenta actualizada: revisión %d, %d partidas", current.Account.Revision, len(current.Draft))
		}
		// Dos confirmaciones simultáneas de la misma revisión (una por terminal): un solo envío a cocina.
		preview := decodeResponse[operations.QuoteResult](t, fixture.call(t, fixture.north, "GET", route+"/preview", nil, 200))
		codes := make([]int, 2)
		bodies := make([][]byte, 2)
		var wait sync.WaitGroup
		for index, terminal := range []testTerminal{fixture.north, fixture.south} {
			wait.Add(1)
			go func() {
				defer wait.Done()
				codes[index], bodies[index] = fixture.rawCall(terminal, "POST", route+"/confirm", map[string]any{
					"operation_id": randomID(), "revision": current.Account.Revision, "expected_fingerprint": preview.Fingerprint,
				})
			}()
		}
		wait.Wait()
		accepted := 0
		for index := range codes {
			if codes[index] == 200 {
				accepted++
				continue
			}
			if problem, _ := errorOf(t, bodies[index]); codes[index] != 409 || (problem != "ACCOUNT_CONFLICT" && problem != "QUOTE_CHANGED") {
				t.Fatalf("la confirmación rechazada debe avisar el conflicto: %d %s", codes[index], bodies[index])
			}
		}
		if accepted != 1 {
			t.Fatalf("confirmaciones aceptadas: %d", accepted)
		}
		confirmed := 0
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*)::int FROM app.restaurant_item_units WHERE account_id=$1", accountID).Scan(&confirmed); err != nil {
			t.Fatal(err)
		}
		if confirmed != 3 {
			t.Fatalf("se confirmaron %d unidades; se esperaban 3 (sin duplicar el envío)", confirmed)
		}
	})

	t.Run("el envío a cocina y la preparación se ven en las dos terminales", func(t *testing.T) {
		after := fixture.head(t)
		board := decodeResponse[struct {
			Tasks []kitchen.Task `json:"tasks"`
		}](t, fixture.call(t, fixture.south, "GET", "kitchen/board?state=all", nil, 200))
		references := []map[string]any{}
		for _, task := range board.Tasks {
			if task.StationID == fixture.station.ID && task.State != "cancelled" {
				references = append(references, map[string]any{"id": task.ID, "revision": task.Revision})
			}
		}
		if len(references) == 0 {
			t.Fatal("la otra terminal no ve el envío a cocina")
		}
		fixture.write(t, fixture.south, "POST", "kitchen/states", map[string]any{"state": "ready", "tasks": references}, 200)
		fixture.waitEvents(t, fixture.north, after, kind("kitchen.states.changed"))
		bill := decodeResponse[operations.Bill](t, fixture.call(t, fixture.north, "GET", "restaurant/accounts/"+accountID, nil, 200))
		if bill.Account.PreparationState != "ready" {
			t.Fatalf("la terminal que capturó debe ver la cuenta lista: %s", bill.Account.PreparationState)
		}
		// Una terminal con la vista vieja de la tarea no la cambia en silencio.
		if status, _ := fixture.rawCall(fixture.north, "POST", "kitchen/states", map[string]any{
			"operation_id": randomID(), "state": "preparing", "tasks": references,
		}); status == 200 {
			t.Fatal("se aceptó un cambio de preparación con la revisión vieja")
		}
	})

	t.Run("el cobro de una caja libera la mesa en la otra y el efectivo cuenta en su caja", func(t *testing.T) {
		after := fixture.head(t)
		before := fixture.expected(t, fixture.shifts["Caja Sur 9G-C"]).MXN
		result := decodeResponse[paymentResult](t, fixture.pay(t, fixture.north, accountID, map[string]any{
			"method_id": fixture.methods["cash"].ID, "mode": "total",
		}, 200))
		if !result.Closed {
			t.Fatal("el cobro total debe cerrar la cuenta")
		}
		fixture.waitEvents(t, fixture.south, after, kind("restaurant.account.closed"), aggregate("cash", fixture.shifts["Caja Norte 9G-C"]))
		if state := tableState(fixture.recovery(t, fixture.south), fixture.tables[0].ID); state != "free" {
			t.Fatalf("la otra terminal debe ver la mesa libre: %s", state)
		}
		if after := fixture.expected(t, fixture.shifts["Caja Sur 9G-C"]).MXN; after != before {
			t.Fatalf("el cobro de la Caja Norte cambió el esperado de la Caja Sur: %s → %s", before, after)
		}
		if north := fixture.expected(t, fixture.shifts["Caja Norte 9G-C"]).MXN; amountOf(t, north).IsZero() {
			t.Fatal("el efectivo del cobro debe contar en la Caja Norte")
		}
	})

	t.Run("el arqueo de cierre pausa solo su caja y el tope se compara por caja", func(t *testing.T) {
		north, south := fixture.shifts["Caja Norte 9G-C"], fixture.shifts["Caja Sur 9G-C"]
		fixture.write(t, fixture.north, "POST", "payments/shifts/"+north+"/closing", map[string]any{}, 200)
		// La Caja Sur sigue cobrando mientras la Norte cuenta su efectivo.
		opened := decodeResponse[operations.Bill](t, fixture.write(t, fixture.south, "POST", "restaurant/accounts", map[string]any{
			"mode": "dine_in", "table_id": fixture.tables[1].ID, "seen_account_ids": []string{},
			"items": []operations.DraftItem{draftLine(fixture.fries, 3)},
		}, 200))
		route := "restaurant/accounts/" + opened.Account.ID
		preview := decodeResponse[operations.QuoteResult](t, fixture.call(t, fixture.south, "GET", route+"/preview", nil, 200))
		fixture.write(t, fixture.south, "POST", route+"/confirm", map[string]any{
			"revision": opened.Account.Revision, "expected_fingerprint": preview.Fingerprint,
		}, 200)
		// Tope de caja de 100 pesos: la Caja Sur lo supera con su propio efectivo; la Norte, en arqueo, no recibe aviso.
		features := decodeResponse[[]struct {
			Code     string `json:"code"`
			Revision int    `json:"revision"`
		}](t, fixture.adminCall(t, "GET", "settings/features", nil, 200))
		for _, feature := range features {
			if feature.Code == "cash_limit" {
				fixture.adminCall(t, "PUT", "settings/features", map[string]any{"code": "cash_limit", "enabled": true, "revision": feature.Revision}, 200)
			}
		}
		limit := decodeResponse[payments.CashLimitSettings](t, fixture.adminCall(t, "GET", "payments/cash/limit", nil, 200))
		fixture.adminCall(t, "PUT", "payments/cash/limit", map[string]any{"operation_id": randomID(), "revision": limit.Revision, "cash_limit_mxn": "100"}, 200)
		fixture.pay(t, fixture.south, opened.Account.ID, map[string]any{"method_id": fixture.methods["cash"].ID, "mode": "total"}, 200)
		// La Norte, pausada por su arqueo, no acepta cobros.
		other := decodeResponse[operations.Bill](t, fixture.write(t, fixture.north, "POST", "restaurant/accounts", map[string]any{
			"mode": "dine_in", "table_id": fixture.tables[0].ID, "seen_account_ids": []string{},
			"items": []operations.DraftItem{draftLine(fixture.fries, 1)},
		}, 200))
		otherRoute := "restaurant/accounts/" + other.Account.ID
		otherPreview := decodeResponse[operations.QuoteResult](t, fixture.call(t, fixture.north, "GET", otherRoute+"/preview", nil, 200))
		fixture.write(t, fixture.north, "POST", otherRoute+"/confirm", map[string]any{
			"revision": other.Account.Revision, "expected_fingerprint": otherPreview.Fingerprint,
		}, 200)
		if code, body := fixture.rawCall(fixture.north, "POST", "payments/accounts/"+other.Account.ID+"/payments", map[string]any{
			"operation_id": randomID(), "method_id": fixture.methods["cash"].ID, "mode": "total",
			"expected_balance": fixture.finance(t, other.Account.ID).Balance,
		}); code != 409 {
			t.Fatalf("la caja en arqueo cobró: %d %s", code, body)
		}
		southDetail := fixture.call(t, fixture.south, "GET", "payments/shifts/"+south, nil, 200)
		northDetail := fixture.call(t, fixture.north, "GET", "payments/shifts/"+north, nil, 200)
		if !strings.Contains(string(southDetail), `"cash_limit_warning"`) {
			t.Fatalf("la Caja Sur supera el tope con su efectivo y debe recibir el aviso: %s", southDetail)
		}
		if strings.Contains(string(northDetail), `"cash_limit_warning"`) {
			t.Fatal("la Caja Norte no supera el tope (y está en arqueo): no recibe el aviso")
		}
		// La Caja Sur sigue abierta; la Norte, en arqueo.
		if detail := fixture.detail(t, fixture.south, south); detail.Shift.State != "open" {
			t.Fatalf("turno de la Caja Sur: %s", detail.Shift.State)
		}
	})
}

// detail lee el detalle de un turno desde la terminal dada.
func (fixture *twoTerminalsFixture) detail(t *testing.T, terminal testTerminal, shiftID string) payments.ShiftDetail {
	t.Helper()
	return decodeResponse[payments.ShiftDetail](t, fixture.call(t, terminal, "GET", "payments/shifts/"+shiftID, nil, 200))
}

// expected es el efectivo esperado de un turno (lo consulta el administrador en el detalle del turno).
func (fixture *twoTerminalsFixture) expected(t *testing.T, shiftID string) payments.CashExpectation {
	t.Helper()
	detail := fixture.detail(t, fixture.admin, shiftID)
	if !detail.ExpectedVisible || detail.Expected == nil {
		t.Fatal("el administrador no recibió el efectivo esperado")
	}
	return *detail.Expected
}
