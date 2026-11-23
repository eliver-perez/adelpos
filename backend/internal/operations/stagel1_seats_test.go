//go:build integration

package operations_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/licenseseats/seatstest"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa L1, Fase D (§5 de la especificación, §7 del contrato): los cupos de la licencia por la API real y PostgreSQL,
// con los límites y el reloj simulados (L1-D16):
//   - el plan básico (1, 0 y 0): solo se captura en la terminal con turno; ni tabletas, ni otra terminal de la misma
//     caja, ni Preparación; cobrar nunca pide cupo; el pedido sin conexión rechazado por cupo no guarda nada;
//   - comandero por terminal: los 15 minutos con las previas, la tableta compartida con tres PIN (volver a «Selecciona
//     tu usuario» no lo libera), cerrar sesión el último usuario, abrir turno y «Liberar cupo» desde Terminales;
//   - un reintento de una captura que ya entró no vuelve a pedir cupo;
//   - cocina: entrar, salir y cerrar sesión;
//   - la caja compartida de L1-D3: dos terminales con la misma caja física ocupan dos cupos y cerrar el turno desde otra
//     computadora libera los dos;
//   - una licencia nueva con menos cupos y varias capturas a la vez por el último cupo.
//
// Usa su propia base desde cero.

type seatsFixture struct {
	*paymentsFixture
	source  *seatstest.Source
	clock   *seatstest.Clock
	cash    string
	tables  []operations.Table
	waiters map[string]string
}

// seatsPermissions: cajero con captura, cocina (para Preparación), cobros y caja.
var seatsPermissions = append([]string{
	"accounts.open", "orders.capture", "orders.confirm", "orders.edit", "catalog.read", "catalog.prices.read",
	"restaurant.tables.read", "events.read", "kitchen.states.read", "kitchen.states.manage", "kitchen.stations.all",
}, cashierPermissions...)

// send envía la petición sin comprobar el estado: con la sesión si la hay y, para la cola del teléfono, con la
// credencial de envío en lugar de la sesión.
func (fixture *seatsFixture) send(terminal testTerminal, credential, method, route string, body any) (int, []byte) {
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:56900"
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

// openBody es abrir una cuenta sin mesa con su primer producto (crear un pedido).
func (fixture *seatsFixture) openBody() map[string]any {
	return map[string]any{"operation_id": randomID(), "mode": "dine_in", "items": []operations.DraftItem{draftLine(fixture.fries, 1)}}
}

// capture abre una cuenta con su primer producto y exige que proceda.
func (fixture *seatsFixture) capture(t *testing.T, terminal testTerminal) operations.Bill {
	t.Helper()
	code, body := fixture.send(terminal, "", "POST", "restaurant/accounts", fixture.openBody())
	if code != 200 {
		t.Fatalf("captura de %s: %d %s", terminal.deviceID, code, body)
	}
	return decodeResponse[operations.Bill](t, body)
}

type seatProblem struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details struct {
			Seat  string `json:"seat"`
			InUse int    `json:"in_use"`
			Limit int    `json:"limit"`
		} `json:"details"`
	} `json:"error"`
}

// noSeat exige 409 LICENSE_SEAT_UNAVAILABLE con el cupo, cuántos hay en uso y el límite; devuelve el mensaje.
func noSeat(t *testing.T, code int, body []byte, seat licenseseats.Seat, inUse, limit int) string {
	t.Helper()
	problem := decodeResponse[seatProblem](t, body)
	if code != 409 || problem.Error.Code != "LICENSE_SEAT_UNAVAILABLE" || problem.Error.Details.Seat != string(seat) ||
		problem.Error.Details.InUse != inUse || problem.Error.Details.Limit != limit {
		t.Fatalf("esperado sin cupo de %s (%d de %d): %d %s", seat, inUse, limit, code, body)
	}
	return problem.Error.Message
}

// confirmOrder envía a cocina el borrador de la cuenta («Enviar a cocina», también una captura).
func (fixture *seatsFixture) confirmOrder(t *testing.T, terminal testTerminal, accountID string) {
	t.Helper()
	route := "restaurant/accounts/" + accountID
	current := fixture.refresh(t, accountID)
	preview := decodeResponse[operations.QuoteResult](t, fixture.call(t, terminal, "GET", route+"/preview", nil, 200))
	fixture.call(t, terminal, "POST", route+"/confirm", map[string]any{
		"operation_id": randomID(), "revision": current.Account.Revision, "expected_fingerprint": preview.Fingerprint,
	}, 200)
}

// refused abre una cuenta que debe rechazarse por cupo y comprueba que no se guardó nada: ni la cuenta ni su clave.
func (fixture *seatsFixture) refused(t *testing.T, terminal testTerminal, seat licenseseats.Seat, inUse, limit int) string {
	t.Helper()
	accounts := fixture.countRows(t, "SELECT count(*)::int FROM app.restaurant_accounts")
	request := fixture.openBody()
	code, body := fixture.send(terminal, "", "POST", "restaurant/accounts", request)
	message := noSeat(t, code, body, seat, inUse, limit)
	if fixture.countRows(t, "SELECT count(*)::int FROM app.restaurant_accounts") != accounts ||
		fixture.countRows(t, "SELECT count(*)::int FROM app.restaurant_operations WHERE id=$1", request["operation_id"]) != 0 {
		t.Fatal("la captura rechazada por cupo guardó la cuenta o su clave")
	}
	return message
}

// usage es seats de sessions-summary.
func (fixture *seatsFixture) usage(t *testing.T) map[string]licenseseats.Usage {
	t.Helper()
	summary := decodeResponse[struct {
		ByKind  map[string]int                `json:"by_kind"`
		Limited bool                          `json:"limited"`
		Seats   map[string]licenseseats.Usage `json:"seats"`
	}](t, fixture.adminCall(t, "GET", "devices/sessions-summary", nil, 200))
	if !summary.Limited || len(summary.ByKind) != 3 || len(summary.Seats) != 3 {
		t.Fatalf("sessions-summary: %+v", summary)
	}
	return summary.Seats
}

// inUse exige cuántos cupos de cada tipo hay en uso (cajas, comanderos y pantallas de cocina).
func (fixture *seatsFixture) inUse(t *testing.T, cash, orderTakers, kitchenDisplays int) {
	t.Helper()
	usage := fixture.usage(t)
	if usage["cash_registers"].InUse != cash || usage["order_takers"].InUse != orderTakers || usage["kitchen_displays"].InUse != kitchenDisplays {
		t.Fatalf("cupos en uso: %+v; esperados %d, %d y %d", usage, cash, orderTakers, kitchenDisplays)
	}
}

// seatsOf es lo que GET devices dice que ocupa la terminal.
func (fixture *seatsFixture) seatsOf(t *testing.T, terminal testTerminal) []string {
	t.Helper()
	for _, device := range decodeResponse[[]struct {
		ID    string   `json:"id"`
		Seats []string `json:"seats"`
	}](t, fixture.adminCall(t, "GET", "devices", nil, 200)) {
		if device.ID == terminal.deviceID {
			return device.Seats
		}
	}
	t.Fatalf("la terminal %s no está en Terminales", terminal.deviceID)
	return nil
}

func (fixture *seatsFixture) displayMode(terminal testTerminal, active bool) (int, []byte) {
	return fixture.send(terminal, "", "POST", "kitchen/display-mode", map[string]bool{"active": active})
}

// desktop es una terminal de escritorio con su usuario y su sesión; con caja física si se indica.
func (fixture *seatsFixture) desktop(t *testing.T, username, registerID string) testTerminal {
	t.Helper()
	return newCashier(t, fixture.paymentsFixture, username, seatsPermissions, registerID)
}

// tablet registra un teléfono o tableta de meseros como si se hubiera emparejado.
func (fixture *seatsFixture) tablet(t *testing.T, name string) testTerminal {
	t.Helper()
	secret := administration.NewToken()
	var id string
	if err := fixture.pool.QueryRow(fixture.ctx, `INSERT INTO app.devices(name,secret_hash,kind,os,app_version,paired_at)
        VALUES ($1,$2,'android','Android 15','0.3.0',now()) RETURNING id`, name, administration.TokenHash(secret)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return testTerminal{deviceID: id, deviceToken: secret}
}

// enter es la entrada con PIN de un mesero en una tableta: la tableta con su sesión y la credencial de envío.
func (fixture *seatsFixture) enter(t *testing.T, tablet testTerminal, waiter string) (testTerminal, string) {
	t.Helper()
	code, response := fixture.send(tablet, "", "POST", "auth/pin-login", map[string]string{"user_id": fixture.waiters[waiter], "pin": "4829"})
	login := decodeResponse[struct {
		Session    string `json:"session_token"`
		Credential string `json:"offline_credential"`
	}](t, response)
	if code != 200 || login.Session == "" || login.Credential == "" {
		t.Fatalf("entrada con PIN de %s: %d %s", waiter, code, response)
	}
	session := tablet
	session.sessionToken = login.Session
	return session, login.Credential
}

// closeShift cierra el turno desde la terminal indicada: inicia el arqueo, guarda el conteo y lo confirma.
func (fixture *seatsFixture) closeShift(t *testing.T, terminal testTerminal, shiftID string) {
	t.Helper()
	route := "payments/shifts/" + shiftID
	fixture.call(t, terminal, "POST", route+"/closing", map[string]any{"operation_id": randomID()}, 200)
	closing := decodeResponse[payments.ClosingView](t, fixture.call(t, terminal, "GET", route+"/closing", nil, 200))
	fixture.call(t, terminal, "PUT", route+"/closing/count", map[string]any{
		"operation_id": randomID(), "revision": closing.Count.Revision, "lines": []map[string]any{}, "extra_mxn": "50",
		"extra_note": "Conteo de la prueba de cupos", "notes": "",
	}, 200)
	closing = decodeResponse[payments.ClosingView](t, fixture.call(t, terminal, "GET", route+"/closing", nil, 200))
	fixture.call(t, terminal, "POST", route+"/closing/confirm", map[string]any{"operation_id": randomID(), "count_revision": closing.Count.Revision}, 200)
}

func TestStageL1LicenseSeats(t *testing.T) {
	resetStage9GDatabase(t)
	base := newFixture(t)
	base.seedCatalog(t)
	fixture := &seatsFixture{
		paymentsFixture: &paymentsFixture{restaurantFixture: base, methods: map[string]payments.Method{}, registers: map[string]string{}},
		source:          seatstest.NewSource(1, 0, 0),
		clock:           seatstest.NewClock(time.Now()),
		waiters:         map[string]string{},
	}
	base.service.Seats = licenseseats.NewManager(licenseseats.Options{Limits: fixture.source, Now: fixture.clock.Now})
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil && *method.Code == "cash" {
			fixture.cash = method.ID
		}
	}
	// La otra terminal de la caja compartida (L1-D3) cobra en el turno de su caja: el establecimiento permite compartirlo.
	settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
	fixture.adminCall(t, "PUT", "payments/settings", map[string]any{
		"operation_id": randomID(), "revision": settings.Revision, "shared_shifts": true,
		"print_receipt_default": settings.PrintReceiptDefault, "receipt_printer_id": settings.ReceiptPrinterID,
		"refund_authorization_threshold": settings.RefundAuthorizationThreshold,
	}, 200)
	station := decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina L1-D", Active: true, Reception: "kds", TargetMinutes: 12, WarningMinutes: 15, OverdueMinutes: 25,
		Grouping: "inherit", Tracking: "inherit",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare", StationIDs: []string{station.ID}}, 200)
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón L1-D", Active: true}, 200))
	for index := range 2 {
		fixture.tables = append(fixture.tables, decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: fmt.Sprintf("Mesa L1-D %d", index+1), Shape: "round", X: 20 + index*130, Y: 30,
			Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200)))
	}
	registerOne := decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": "Caja L1-D 1", "description": "", "active": true}, 200)).ID
	registerTwo := decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": "Caja L1-D 2", "description": "", "active": true}, 200)).ID
	// La caja de la caja 1, otra terminal con la misma caja física, dos terminales sin caja y una de la caja 2.
	cashier := fixture.desktop(t, "caja_uno_l1d", registerOne)
	partner := fixture.desktop(t, "caja_compartida_l1d", registerOne)
	captain := fixture.desktop(t, "captura_l1d", "")
	screen := fixture.desktop(t, "cocina_l1d", "")
	second := fixture.desktop(t, "caja_dos_l1d", registerTwo)
	// Tres meseros con PIN y dos tabletas.
	role := decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "roles", map[string]any{"name": "Meseros L1-D", "permissions": waiterPermissions}, 200)).ID
	for index, name := range []string{"ana", "beto", "carla"} {
		fixture.waiters[name] = decodeResponse[struct {
			ID string `json:"id"`
		}](t, fixture.adminCall(t, "POST", "users", map[string]any{"username": name + "_l1d", "display_name": "Mesero " + name,
			"role_id": role, "active": true, "password": "Clave temporal de " + name}, 200)).ID
		fixture.adminCall(t, "PUT", "users/"+fixture.waiters[name]+"/pin", map[string]any{"pin": "4829",
			"short_name": fmt.Sprintf("M%d", index+1), "badge_color": "#3591d4"}, 200)
	}
	shared := fixture.tablet(t, "Tableta compartida L1-D")
	other := fixture.tablet(t, "Tableta dos L1-D")
	anaShared, anaCredential := fixture.enter(t, shared, "ana")
	var shiftOne string
	var accountOne operations.Bill
	var captainFirst map[string]any

	t.Run("plan básico: solo captura la terminal con turno, cobrar nunca pide cupo", func(t *testing.T) {
		fixture.inUse(t, 0, 0, 0)
		usage := fixture.usage(t)
		if usage["cash_registers"].Limit != 1 || usage["order_takers"].Limit != 0 || usage["kitchen_displays"].Limit != 0 {
			t.Fatalf("límites del plan básico: %+v", usage)
		}
		// Sin turno abierto, la caja también sería comandero: no hay ninguno.
		message := fixture.refused(t, cashier, licenseseats.OrderTakers, 0, 0)
		if !strings.Contains(message, "no incluye comanderos") {
			t.Fatalf("mensaje sin comanderos: %q", message)
		}
		shiftOne = decodeResponse[payments.Shift](t, fixture.call(t, cashier, "POST", "payments/shifts",
			map[string]any{"operation_id": randomID(), "opening_float": "0"}, 200)).ID
		fixture.inUse(t, 1, 0, 0)
		accountOne = fixture.capture(t, cashier)
		fixture.confirmOrder(t, cashier, accountOne.Account.ID)
		fixture.refused(t, captain, licenseseats.OrderTakers, 0, 0)
		fixture.refused(t, anaShared, licenseseats.OrderTakers, 0, 0)
		// La otra terminal de la misma caja física: con 1 caja no captura gratis (L1-D3).
		message = fixture.refused(t, partner, licenseseats.CashRegisters, 1, 1)
		if message != "No hay cupo de caja libre (1 de 1 en uso). Se libera al cerrar un turno de caja." {
			t.Fatalf("mensaje sin caja: %q", message)
		}
		// Pero cobra y registra efectivo en el turno de su caja sin cupo.
		finance := fixture.finance(t, accountOne.Account.ID)
		fixture.pay(t, partner, accountOne.Account.ID, map[string]any{"method_id": fixture.cash, "mode": "total",
			"received": finance.Balance}, 200)
		fixture.call(t, partner, "POST", "payments/shifts/"+shiftOne+"/income", map[string]any{"operation_id": randomID(),
			"type": "other", "amount": "10", "concept": "Cambio de la prueba de cupos"}, 200)
		// Ni Preparación.
		code, body := fixture.displayMode(cashier, true)
		if message := noSeat(t, code, body, licenseseats.KitchenDisplays, 0, 0); !strings.Contains(message, "Preparación") {
			t.Fatalf("mensaje sin pantallas de cocina: %q", message)
		}
		fixture.inUse(t, 1, 0, 0)
		if seats := fixture.seatsOf(t, cashier); !slices.Equal(seats, []string{"cash_registers"}) {
			t.Fatalf("cupos de la caja en Terminales: %v", seats)
		}
		if seats := fixture.seatsOf(t, partner); len(seats) != 0 {
			t.Fatalf("cupos de la terminal rechazada: %v", seats)
		}
	})

	t.Run("pedido sin conexión rechazado por cupo: definitivo, sin guardar nada", func(t *testing.T) {
		var version int64
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(max(version),0) FROM app.catalog_change_log").Scan(&version); err != nil {
			t.Fatal(err)
		}
		order := offlineBody(randomID(), version, map[string]any{"table_id": fixture.tables[0].ID, "seen_account_ids": []string{}},
			offlineItem(fixture.fries.ID, 1, "40.00"))
		device := testTerminal{deviceID: shared.deviceID, deviceToken: shared.deviceToken}
		for attempt := range 2 {
			code, body := fixture.send(device, anaCredential, "POST", "restaurant/offline-orders", order)
			noSeat(t, code, body, licenseseats.OrderTakers, 0, 0)
			if fixture.countRows(t, "SELECT count(*)::int FROM app.restaurant_operations WHERE id=$1", order["operation_id"]) != 0 ||
				fixture.countRows(t, "SELECT count(*)::int FROM app.restaurant_accounts WHERE table_id=$1", fixture.tables[0].ID) != 0 ||
				fixture.countRows(t, "SELECT count(*)::int FROM app.audit_events WHERE action LIKE 'restaurant.offline_order.%' AND entity_id=$1",
					order["operation_id"]) != 0 {
				t.Fatalf("envío %d: el pedido rechazado por cupo dejó rastro", attempt+1)
			}
		}
	})

	t.Run("comandero por terminal: 15 minutos con las previas y el reintento de lo que ya entró", func(t *testing.T) {
		fixture.source.Set(3, 1, 1)
		captainFirst = fixture.openBody()
		code, body := fixture.send(captain, "", "POST", "restaurant/accounts", captainFirst)
		if code != 200 {
			t.Fatalf("captura con cupo: %d %s", code, body)
		}
		opened := decodeResponse[operations.Bill](t, body)
		fixture.inUse(t, 1, 1, 0)
		message := fixture.refused(t, anaShared, licenseseats.OrderTakers, 1, 1)
		if message != "No hay cupo de comandero libre (1 de 1 en uso). Pide que se cierre la sesión de otra terminal." {
			t.Fatalf("mensaje sin comandero: %q", message)
		}
		// Entrar a la mesa para capturar (la previa) renueva la actividad.
		fixture.clock.Advance(10 * time.Minute)
		current := fixture.refresh(t, opened.Account.ID)
		fixture.call(t, captain, "POST", "restaurant/accounts/"+opened.Account.ID+"/draft-preview", map[string]any{
			"revision": current.Account.Revision, "items": []operations.DraftItem{draftLine(fixture.fries, 2)},
		}, 200)
		fixture.clock.Advance(14 * time.Minute)
		fixture.refused(t, anaShared, licenseseats.OrderTakers, 1, 1)
		fixture.clock.Advance(time.Minute)
		// 15 minutos sin captura ni previa: se libera solo.
		fixture.inUse(t, 1, 0, 0)
		if seats := fixture.seatsOf(t, captain); len(seats) != 0 {
			t.Fatalf("el comandero vencido sigue en Terminales: %v", seats)
		}
		fixture.capture(t, anaShared)
		fixture.refused(t, captain, licenseseats.OrderTakers, 1, 1)
		// El reintento de la captura que ya entró (respuesta perdida) recibe su resultado, sin pedir cupo otra vez.
		code, body = fixture.send(captain, "", "POST", "restaurant/accounts", captainFirst)
		if code != 200 || decodeResponse[operations.Bill](t, body).Account.ID != opened.Account.ID {
			t.Fatalf("reintento de una captura que ya entró: %d %s", code, body)
		}
		fixture.inUse(t, 1, 1, 0)
	})

	t.Run("tableta compartida: tres PIN, un cupo; volver a «Selecciona tu usuario» no lo libera", func(t *testing.T) {
		betoShared, _ := fixture.enter(t, shared, "beto")
		carlaShared, _ := fixture.enter(t, shared, "carla")
		for _, session := range []testTerminal{anaShared, betoShared, carlaShared} {
			fixture.capture(t, session)
		}
		fixture.inUse(t, 1, 1, 0)
		// «Selecciona tu usuario» consulta los meseros con la credencial del dispositivo y no cierra ninguna sesión.
		fixture.clock.Advance(2 * time.Minute)
		code, body := fixture.send(testTerminal{deviceID: shared.deviceID, deviceToken: shared.deviceToken}, "", "GET", "auth/pin-users", nil)
		if code != 200 {
			t.Fatalf("«Selecciona tu usuario»: %d %s", code, body)
		}
		fixture.enter(t, shared, "beto")
		fixture.inUse(t, 1, 1, 0)
		anaOther, _ := fixture.enter(t, other, "ana")
		fixture.refused(t, anaOther, licenseseats.OrderTakers, 1, 1)
		if seats := fixture.seatsOf(t, shared); !slices.Equal(seats, []string{"order_takers"}) {
			t.Fatalf("cupos de la tableta compartida: %v", seats)
		}
	})

	t.Run("liberar el cupo desde Terminales cierra las sesiones de esa terminal", func(t *testing.T) {
		released := decodeResponse[struct {
			Released       []string `json:"released"`
			SessionsClosed int      `json:"sessions_closed"`
		}](t, fixture.adminCall(t, "POST", "devices/"+shared.deviceID+"/seat/release", map[string]any{}, 200))
		if !slices.Equal(released.Released, []string{"order_takers"}) || released.SessionsClosed != 3 {
			t.Fatalf("liberar el cupo de la tableta: %+v", released)
		}
		if fixture.countRows(t, `SELECT count(*)::int FROM app.audit_events WHERE action='license.seat_released' AND entity_id=$1`,
			shared.deviceID) != 1 {
			t.Fatal("sin auditoría license.seat_released")
		}
		fixture.inUse(t, 1, 0, 0)
		if code, _ := fixture.send(anaShared, "", "GET", "auth/me", nil); code != 401 {
			t.Fatalf("la sesión de la tableta liberada sigue vigente: %d", code)
		}
		anaOther, _ := fixture.enter(t, other, "ana")
		fixture.capture(t, anaOther)
		// Sin cupo de comandero o de cocina no hay nada que liberar; una caja se libera cerrando su turno.
		code, body := fixture.send(fixture.admin, "", "POST", "devices/"+shared.deviceID+"/seat/release", map[string]any{})
		if problem, _ := errorOf(t, body); code != 409 || problem != "LICENSE_SEAT_NOT_HELD" {
			t.Fatalf("liberar una terminal sin cupo: %d %s", code, body)
		}
		code, body = fixture.send(fixture.admin, "", "POST", "devices/"+cashier.deviceID+"/seat/release", map[string]any{})
		if problem, message := errorOf(t, body); code != 409 || problem != "LICENSE_SEAT_NOT_HELD" || !strings.Contains(message, "cerrar su turno") {
			t.Fatalf("liberar una caja: %d %s", code, body)
		}
		// Sin devices.manage no se libera.
		if code, _ := fixture.send(anaOther, "", "POST", "devices/"+other.deviceID+"/seat/release", map[string]any{}); code != 403 {
			t.Fatalf("liberar sin permiso: %d", code)
		}
		fixture.inUse(t, 1, 1, 0)
	})

	t.Run("cocina: entrar a Preparación ocupa el cupo, salir y cerrar sesión lo liberan", func(t *testing.T) {
		if code, body := fixture.displayMode(cashier, true); code != 200 || !strings.Contains(string(body), `"active":true`) {
			t.Fatalf("entrar a Preparación: %d %s", code, body)
		}
		code, body := fixture.displayMode(screen, true)
		noSeat(t, code, body, licenseseats.KitchenDisplays, 1, 1)
		if code, body := fixture.displayMode(cashier, false); code != 200 {
			t.Fatalf("salir de Preparación: %d %s", code, body)
		}
		if code, body := fixture.displayMode(screen, true); code != 200 {
			t.Fatalf("entrar a Preparación con el cupo libre: %d %s", code, body)
		}
		fixture.inUse(t, 1, 1, 1)
		fixture.call(t, screen, "POST", "auth/logout", nil, 200)
		fixture.inUse(t, 1, 1, 0)
		if code, body := fixture.displayMode(cashier, true); code != 200 {
			t.Fatalf("entrar a Preparación después del cierre de sesión: %d %s", code, body)
		}
		// Un teléfono o tableta de meseros no tiene Preparación (sin permiso de cocina en la sesión con PIN).
		anaOther, _ := fixture.enter(t, other, "ana")
		if code, _ := fixture.displayMode(anaOther, true); code != 403 {
			t.Fatalf("Preparación en una tableta: %d", code)
		}
		if code, body := fixture.send(cashier, "", "POST", "kitchen/display-mode", map[string]any{}); code != 400 {
			t.Fatalf("modo Cocina sin active: %d %s", code, body)
		}
		fixture.inUse(t, 1, 1, 1)
	})

	t.Run("cerrar sesión el último usuario y abrir turno liberan el comandero", func(t *testing.T) {
		fixture.source.Set(3, 2, 1)
		fixture.capture(t, captain)
		fixture.inUse(t, 1, 2, 1)
		fixture.call(t, captain, "POST", "auth/logout", nil, 200)
		fixture.inUse(t, 1, 1, 1)
		// La sesión siguiente en esa terminal no recupera el cupo sin capturar.
		login := decodeResponse[struct {
			Token string `json:"session_token"`
		}](t, fixture.call(t, testTerminal{deviceID: captain.deviceID, deviceToken: captain.deviceToken}, "POST", "auth/login",
			map[string]string{"username": "captura_l1d", "password": "Clave temporal de captura_l1d"}, 200))
		captain.sessionToken = login.Token
		fixture.inUse(t, 1, 1, 1)
		// La terminal de la caja 2 captura como comandero y deja de serlo al abrir el turno de su caja.
		fixture.capture(t, second)
		fixture.inUse(t, 1, 2, 1)
		fixture.call(t, second, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0"}, 200)
		fixture.inUse(t, 2, 1, 1)
		if seats := fixture.seatsOf(t, second); !slices.Equal(seats, []string{"cash_registers"}) {
			t.Fatalf("cupos de la caja 2 después de abrir turno: %v", seats)
		}
	})

	t.Run("caja compartida: dos terminales, dos cupos; cerrar el turno desde otra computadora libera los dos", func(t *testing.T) {
		fixture.capture(t, partner)
		fixture.inUse(t, 3, 1, 1)
		for _, terminal := range []testTerminal{cashier, partner} {
			if seats := fixture.seatsOf(t, terminal); !slices.Contains(seats, "cash_registers") {
				t.Fatalf("cupos de una terminal de la caja compartida: %v", seats)
			}
		}
		// La computadora principal (la del administrador) cierra el turno de la caja 1.
		fixture.closeShift(t, fixture.admin, shiftOne)
		fixture.inUse(t, 1, 1, 1)
		if seats := fixture.seatsOf(t, partner); len(seats) != 0 {
			t.Fatalf("la terminal compartida sigue con su caja: %v", seats)
		}
		// Sin turno en su caja, la terminal captura como comandero.
		fixture.capture(t, partner)
		fixture.inUse(t, 1, 2, 1)
	})

	t.Run("licencia nueva con menos cupos: nadie se interrumpe y no se ocupan nuevos", func(t *testing.T) {
		fixture.source.Set(1, 1, 1)
		anaOther, _ := fixture.enter(t, other, "ana")
		// Los dos comanderos y la caja 2 siguen capturando.
		fixture.capture(t, anaOther)
		fixture.capture(t, partner)
		fixture.capture(t, second)
		fixture.refused(t, captain, licenseseats.OrderTakers, 2, 1)
		// La caja 1 no abre turno: la caja 2 ya ocupa la única caja.
		shifts := fixture.countRows(t, "SELECT count(*)::int FROM app.cash_shifts")
		code, body := fixture.send(cashier, "", "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0"})
		noSeat(t, code, body, licenseseats.CashRegisters, 1, 1)
		if fixture.countRows(t, "SELECT count(*)::int FROM app.cash_shifts") != shifts {
			t.Fatal("el turno rechazado por cupo se abrió")
		}
		// Hasta quedar dentro del límite: con un comandero menos sigue sin haber cupo nuevo.
		fixture.call(t, partner, "POST", "auth/logout", nil, 200)
		fixture.refused(t, captain, licenseseats.OrderTakers, 1, 1)
	})

	t.Run("varias terminales a la vez por el último cupo: solo una lo obtiene", func(t *testing.T) {
		// Los comanderos anteriores vencen; en cada vuelta queda un solo cupo libre para cuatro terminales.
		fixture.clock.Advance(16 * time.Minute)
		for round := range 3 {
			holders := fixture.usage(t)["order_takers"].InUse
			fixture.source.Set(1, holders+1, 1)
			contenders := []testTerminal{}
			for index := range 4 {
				contenders = append(contenders, fixture.desktop(t, fmt.Sprintf("carrera_%d_%d_l1d", round, index), ""))
			}
			codes := make([]int, len(contenders))
			bodies := make([][]byte, len(contenders))
			var wait sync.WaitGroup
			for index, terminal := range contenders {
				wait.Add(1)
				go func() {
					defer wait.Done()
					codes[index], bodies[index] = fixture.send(terminal, "", "POST", "restaurant/accounts", fixture.openBody())
				}()
			}
			wait.Wait()
			accepted := 0
			for index, code := range codes {
				if code == 200 {
					accepted++
					continue
				}
				noSeat(t, code, bodies[index], licenseseats.OrderTakers, holders+1, holders+1)
			}
			if accepted != 1 {
				t.Fatalf("vuelta %d: %d capturas obtuvieron el último cupo", round+1, accepted)
			}
		}
	})
}
