//go:build integration

package operations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/licensing/licensingtest"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa L1, Fase C: la puerta de la licencia y de la hora contra PostgreSQL, con el servidor real por la API v1
// (httptest) y por Execute, estado por estado y fila por fila de la tabla de la §6 del contrato, con las reglas firmes
// (cobrar lo abierto en Vencida y Revocada; reportes, historial y respaldos en todos los estados), la hora con reloj
// simulado (bloqueo con su causa, también al cobrar, y desbloqueo al corregir el reloj), la §6.3 con un server_time
// simulado, el desbloqueo de soporte y la carpeta copiada o el respaldo restaurado («Sin activar con datos»).
//
// El reloj del gestor es simulado (licensingtest.Clock): nunca se cambia la hora de la computadora. Las licencias se
// firman con las llaves de prueba de vectors.json y la llave de soporte es una de prueba.

type licenseGateFixture struct {
	*restaurantFixture
	clock    *licensingtest.Clock
	licenses *licensingtest.Fixture
	manager  *licensing.Manager
	cash     string
	shiftID  string
}

// licenseReply es una respuesta con su estado y, si es un rechazo, su código y sus details.
type licenseReply struct {
	status  int
	code    string
	details map[string]any
	body    []byte
}

// send hace la petición por la API v1 sin exigir el estado.
func (fixture *licenseGateFixture) send(method, route string, body any) licenseReply {
	encoded := []byte(nil)
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:51000"
	request.Header.Set("X-Provisioning-Token", fixture.provisioning)
	request.Header.Set("X-Device-ID", fixture.admin.deviceID)
	request.Header.Set("X-Device-Token", fixture.admin.deviceToken)
	request.Header.Set("Authorization", "Bearer "+fixture.admin.sessionToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	reply := licenseReply{status: response.Code, body: append([]byte{}, response.Body.Bytes()...)}
	var problem struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(reply.body, &problem) == nil {
		reply.code, reply.details = problem.Error.Code, problem.Error.Details
	}
	return reply
}

// execute hace la misma petición por Execute (sin HTTP) y devuelve el código del rechazo, o "".
func (fixture *licenseGateFixture) execute(t *testing.T, method, route string, body any) string {
	t.Helper()
	encoded, _ := json.Marshal(body)
	if body == nil {
		encoded = nil
	}
	_, err := fixture.service.Execute(context.Background(), administration.Request{
		Method: method, Path: "/api/v1/" + route, Body: encoded, Local: true,
		DeviceID: fixture.admin.deviceID, DeviceToken: fixture.admin.deviceToken, SessionToken: fixture.admin.sessionToken,
	})
	if err == nil {
		return ""
	}
	var fault *administration.Fault
	if !asFault(err, &fault) {
		t.Fatalf("%s %s por Execute: %v", method, route, err)
	}
	return fault.Code
}

func asFault(err error, target **administration.Fault) bool {
	fault, ok := err.(*administration.Fault)
	if ok {
		*target = fault
	}
	return ok
}

// rejected exige el rechazo con su código por la API v1 y por Execute (un rechazo nunca escribe, así que repetirlo no
// cambia nada).
func (fixture *licenseGateFixture) rejected(t *testing.T, method, route string, body any, code string) licenseReply {
	t.Helper()
	reply := fixture.send(method, route, body)
	if reply.status != 409 || reply.code != code {
		t.Fatalf("%s %s: se esperaba 409 %s y resultó %d %s", method, route, code, reply.status, reply.body)
	}
	if got := fixture.execute(t, method, route, body); got != code {
		t.Fatalf("%s %s por Execute: se esperaba %s y resultó %q", method, route, code, got)
	}
	return reply
}

// allowed exige que la puerta deje pasar la operación: la respuesta es la del manejador (expected), nunca un rechazo de
// la licencia o de la hora.
func (fixture *licenseGateFixture) allowed(t *testing.T, method, route string, body any, expected int) licenseReply {
	t.Helper()
	reply := fixture.send(method, route, body)
	if reply.status != expected {
		t.Fatalf("%s %s: se esperaba %d y resultó %d %s", method, route, expected, reply.status, reply.body)
	}
	if strings.HasPrefix(reply.code, "LICENSE_") || reply.code == "SYSTEM_CLOCK_INVALID" {
		t.Fatalf("%s %s: la puerta no debía rechazarla: %s", method, route, reply.body)
	}
	return reply
}

// passesGate exige que la puerta deje pasar la operación, sea cual sea la respuesta del manejador.
func (fixture *licenseGateFixture) passesGate(t *testing.T, method, route string, body any) licenseReply {
	t.Helper()
	reply := fixture.send(method, route, body)
	if strings.HasPrefix(reply.code, "LICENSE_") || reply.code == "SYSTEM_CLOCK_INVALID" {
		t.Fatalf("%s %s: la puerta no debía rechazarla: %s", method, route, reply.body)
	}
	return reply
}

func withOperation(body map[string]any) map[string]any {
	body["operation_id"] = randomID()
	return body
}

// openAccountBody abre una cuenta con un producto (F1).
func (fixture *licenseGateFixture) openAccountBody(name string) map[string]any {
	return withOperation(map[string]any{"name": name, "mode": "dine_in"})
}

// pendingAccount abre una cuenta y le envía un producto: queda pendiente de cobro.
func (fixture *licenseGateFixture) pendingAccount(t *testing.T, name string) operations.Bill {
	t.Helper()
	bill := decodeResponse[operations.Bill](t, fixture.allowed(t, "POST", "restaurant/accounts", fixture.openAccountBody(name), 200).body)
	return fixture.capture(t, bill, []catalog.LineRequest{{ProductID: fixture.fries.ID, Quantity: 1}})
}

// payBody cobra el saldo completo de la cuenta en efectivo.
func (fixture *licenseGateFixture) payBody(t *testing.T, accountID string) map[string]any {
	t.Helper()
	finance := decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+accountID, nil, 200))
	return withOperation(map[string]any{"method_id": fixture.cash, "mode": "total", "expected_balance": finance.Balance})
}

func (fixture *licenseGateFixture) openShift(t *testing.T) {
	t.Helper()
	reply := fixture.allowed(t, "POST", "payments/shifts", withOperation(map[string]any{"opening_float": "0"}), 200)
	fixture.shiftID = decodeResponse[payments.Shift](t, reply.body).ID
}

// closeShift hace el arqueo completo del turno abierto (F4): iniciar, contar y confirmar.
func (fixture *licenseGateFixture) closeShift(t *testing.T) {
	t.Helper()
	route := "payments/shifts/" + fixture.shiftID
	fixture.allowed(t, "POST", route+"/closing", withOperation(map[string]any{}), 200)
	closing := decodeResponse[payments.ClosingView](t, fixture.adminCall(t, "GET", route+"/closing", nil, 200))
	fixture.allowed(t, "PUT", route+"/closing/count", withOperation(map[string]any{
		"revision": closing.Count.Revision, "lines": []map[string]any{}, "extra_mxn": "50", "extra_note": "Prueba de la licencia",
		"notes": "",
	}), 200)
	closing = decodeResponse[payments.ClosingView](t, fixture.adminCall(t, "GET", route+"/closing", nil, 200))
	fixture.allowed(t, "POST", route+"/closing/confirm", withOperation(map[string]any{"count_revision": closing.Count.Revision}), 200)
	fixture.shiftID = ""
}

// alwaysAvailable son las filas que ningún estado bloquea: reportes, historial, respaldos, la sesión y la licencia.
func (fixture *licenseGateFixture) alwaysAvailable(t *testing.T, accountID string) {
	t.Helper()
	fixture.allowed(t, "GET", "reports/catalog", nil, 200)
	fixture.allowed(t, "GET", "restaurant/accounts", nil, 200)
	if accountID != "" {
		fixture.allowed(t, "GET", "restaurant/accounts/"+accountID+"/history", nil, 200)
	}
	fixture.allowed(t, "GET", "audit", nil, 200)
	// El tablero de Preparación es una lectura (F6); entrar en modo Cocina es otra operación (display-mode, F5).
	fixture.allowed(t, "GET", "kitchen/board", nil, 200)
	// Sin respaldos configurados en esta prueba la ruta responde 503, pero la puerta la deja pasar.
	fixture.passesGate(t, "GET", "maintenance/backups", nil)
	fixture.passesGate(t, "POST", "maintenance/backups/run", map[string]any{})
	fixture.allowed(t, "GET", "auth/me", nil, 200)
	fixture.allowed(t, "PUT", "preferences", map[string]any{"theme": "light", "home_screen": "status"}, 200)
	fixture.allowed(t, "POST", "license/support-request", map[string]any{}, 200)
}

// licenseOf es el `license` de auth/me.
func (fixture *licenseGateFixture) licenseOf(t *testing.T) administration.LicenseSummary {
	t.Helper()
	var me struct {
		License *administration.LicenseSummary `json:"license"`
	}
	if err := json.Unmarshal(fixture.allowed(t, "GET", "auth/me", nil, 200).body, &me); err != nil || me.License == nil {
		t.Fatalf("auth/me sin el estado de la licencia: %v", err)
	}
	return *me.License
}

func (fixture *licenseGateFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var value int
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// use pone el gestor de licencias en el servidor, con los cupos de esa licencia (el gestor real, como en main.go).
func (fixture *licenseGateFixture) use(manager *licensing.Manager) {
	fixture.service.License = manager
	fixture.service.Seats = administration.LicenseSeats(manager)
}

// serverClock es el servidor de licencias simulado de la §6.3.
type serverClock struct {
	instant time.Time
	err     error
}

func (clock *serverClock) ServerTime(context.Context) (time.Time, error) {
	return clock.instant, clock.err
}

func TestStageL1LicenseGate(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &licenseGateFixture{restaurantFixture: newFixture(t)}
	fixture.seedCatalog(t)
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil && *method.Code == "cash" {
			fixture.cash = method.ID
		}
	}
	// El reloj simulado empieza con la hora real: la base registra con now() y la última hora confiable se inicia con
	// la última actividad de la auditoría.
	fixture.clock = licensingtest.NewClock(time.Now().UTC().Truncate(time.Second))
	fixture.licenses = licensingtest.New(t, fixture.clock)
	fixture.licenses.Database = licensing.PoolProbe{Pool: fixture.pool}
	fixture.manager = fixture.licenses.Start()
	fixture.use(fixture.manager)

	t.Run("Sin activar: solo reportes, la sesión y la licencia", func(t *testing.T) {
		if state := fixture.licenseOf(t).State; state != licensing.StateNotActivated {
			t.Fatalf("estado: %s", state)
		}
		reply := fixture.rejected(t, "POST", "restaurant/accounts", fixture.openAccountBody("Sin activar"), "LICENSE_NOT_ACTIVATED")
		if reply.details["state"] != "not_activated" || reply.details["copied"] != false || reply.details["operation"] != "capture" {
			t.Fatalf("details: %v", reply.details)
		}
		fixture.rejected(t, "POST", "payments/shifts", withOperation(map[string]any{"opening_float": "0"}), "LICENSE_NOT_ACTIVATED")
		fixture.rejected(t, "POST", "catalog/categories", map[string]any{"name": "Sin activar", "active": true}, "LICENSE_NOT_ACTIVATED")
		fixture.rejected(t, "POST", "kitchen/states", map[string]any{}, "LICENSE_NOT_ACTIVATED")
		fixture.alwaysAvailable(t, "")
	})

	var accounts []operations.Bill
	expires := fixture.clock.Now().Add(72 * time.Hour)
	t.Run("Vigente: todo, dentro de los cupos", func(t *testing.T) {
		fixture.licenses.Accept(fixture.manager, licensingtest.Subscription(expires, 3, 3, 3))
		summary := fixture.licenseOf(t)
		if summary.State != licensing.StateActive || summary.DaysUntilExpiry == nil || *summary.DaysUntilExpiry != 3 ||
			summary.GraceDays != 5 || summary.Clock.Blocked || !summary.Clock.LicenseOK || summary.Environment != "test" {
			t.Fatalf("estado de la licencia: %+v", summary)
		}
		fixture.openShift(t)
		for index := range 4 {
			accounts = append(accounts, fixture.pendingAccount(t, fmt.Sprintf("Cuenta %d", index+1)))
		}
		route := "payments/shifts/" + fixture.shiftID
		fixture.allowed(t, "POST", route+"/closing", withOperation(map[string]any{}), 200)
		fixture.allowed(t, "POST", route+"/closing/cancel", withOperation(map[string]any{"reason": "Prueba de la licencia"}), 200)
		fixture.allowed(t, "GET", "kitchen/board", nil, 200)
		fixture.allowed(t, "POST", "catalog/categories", map[string]any{"name": "Vigente", "active": true}, 200)
		fixture.alwaysAvailable(t, accounts[0].Account.ID)
	})

	t.Run("Tolerancia: todo, con los días que quedan", func(t *testing.T) {
		fixture.clock.Set(expires.Add(26 * time.Hour))
		summary := fixture.licenseOf(t)
		if summary.State != licensing.StateGrace || summary.GraceDaysRemaining == nil || *summary.GraceDaysRemaining != 4 {
			t.Fatalf("estado de la licencia en tolerancia: %+v", summary)
		}
		accounts = append(accounts, fixture.pendingAccount(t, "Cuenta en tolerancia"))
		fixture.allowed(t, "POST", "catalog/categories", map[string]any{"name": "Tolerancia", "active": true}, 200)
		fixture.alwaysAvailable(t, accounts[0].Account.ID)
	})

	t.Run("Vencida: se cobra lo abierto, no se captura; turno solo con cuentas pendientes", func(t *testing.T) {
		fixture.clock.Set(expires.Add(6 * 24 * time.Hour))
		if state := fixture.licenseOf(t).State; state != licensing.StateExpired {
			t.Fatalf("estado: %s", state)
		}
		reply := fixture.rejected(t, "POST", "restaurant/accounts", fixture.openAccountBody("Vencida"), "LICENSE_EXPIRED")
		if reply.details["operation"] != "capture" || reply.details["expires_at"] == nil || reply.details["grace_ended_at"] == nil {
			t.Fatalf("details: %v", reply.details)
		}
		accountRoute := "restaurant/accounts/" + accounts[0].Account.ID
		fixture.rejected(t, "PUT", accountRoute+"/draft", withOperation(map[string]any{"revision": accounts[0].Account.Revision,
			"items": []operations.DraftItem{draftLine(fixture.fries, 1)}}), "LICENSE_EXPIRED")
		// Regla firme: cobrar lo ya abierto.
		fixture.allowed(t, "POST", "payments/accounts/"+accounts[0].Account.ID+"/payments", fixture.payBody(t, accounts[0].Account.ID), 200)
		fixture.allowed(t, "GET", "kitchen/board", nil, 200)
		fixture.allowed(t, "POST", "catalog/categories", map[string]any{"name": "Vencida", "active": true}, 200)
		fixture.alwaysAvailable(t, accounts[0].Account.ID)
		// Cerrar el turno y hacer el corte sí; abrir otro solo porque quedan cuentas pendientes de cobro.
		fixture.closeShift(t)
		fixture.openShift(t)
		for _, bill := range accounts[1:] {
			fixture.allowed(t, "POST", "payments/accounts/"+bill.Account.ID+"/payments", fixture.payBody(t, bill.Account.ID), 200)
		}
		fixture.closeShift(t)
		reply = fixture.rejected(t, "POST", "payments/shifts", withOperation(map[string]any{"opening_float": "0"}), "LICENSE_EXPIRED")
		if reply.details["operation"] != "open_shift" || reply.details["pending_accounts"] != false {
			t.Fatalf("abrir turno sin cuentas pendientes: %v", reply.details)
		}
	})

	accounts = nil
	t.Run("Revocada: igual que Vencida, con su motivo", func(t *testing.T) {
		// Una licencia nueva vigente para capturar cuentas pendientes y después la revocación.
		fixture.licenses.Accept(fixture.manager, licensingtest.Subscription(fixture.clock.Now().Add(30*24*time.Hour), 3, 3, 3))
		fixture.openShift(t)
		accounts = append(accounts, fixture.pendingAccount(t, "Revocada 1"), fixture.pendingAccount(t, "Revocada 2"))
		fixture.licenses.Accept(fixture.manager, licensingtest.Revoked(licensing.ReasonTransferred))
		summary := fixture.licenseOf(t)
		if summary.State != licensing.StateRevoked || summary.RevocationReason == nil || *summary.RevocationReason != "transferred" ||
			summary.Clock.LicenseOK {
			t.Fatalf("estado revocado: %+v", summary)
		}
		reply := fixture.rejected(t, "POST", "restaurant/accounts", fixture.openAccountBody("Revocada"), "LICENSE_REVOKED")
		if reply.details["reason"] != "transferred" || !strings.Contains(fixture.send("POST", "restaurant/accounts",
			fixture.openAccountBody("Revocada")).String(), "transfirió") {
			t.Fatalf("details: %v", reply.details)
		}
		fixture.allowed(t, "POST", "payments/accounts/"+accounts[0].Account.ID+"/payments", fixture.payBody(t, accounts[0].Account.ID), 200)
		// Administración (el inventario es administración, L1-D6): la puerta la deja pasar.
		fixture.passesGate(t, "POST", "inventory/units", withOperation(map[string]any{"code": "ozr", "name": "Onza revocada",
			"dimension": "mass", "factor": "28.349523125", "active": true}))
		fixture.alwaysAvailable(t, accounts[0].Account.ID)
		fixture.closeShift(t)
		fixture.openShift(t)
		fixture.allowed(t, "POST", "payments/accounts/"+accounts[1].Account.ID+"/payments", fixture.payBody(t, accounts[1].Account.ID), 200)
		fixture.closeShift(t)
		reply = fixture.rejected(t, "POST", "payments/shifts", withOperation(map[string]any{"opening_float": "0"}), "LICENSE_REVOKED")
		if reply.details["pending_accounts"] != false {
			t.Fatalf("details: %v", reply.details)
		}
	})

	t.Run("Sin activar con datos: carpeta copiada a otra computadora y respaldo restaurado", func(t *testing.T) {
		original := fixture.manager
		// La misma carpeta de datos en otra computadora: la licencia guardada se ignora.
		fixture.licenses.SetMachine(licensingtest.OtherMachineID)
		copied := fixture.licenses.Start()
		fixture.use(copied)
		summary := fixture.licenseOf(t)
		if summary.State != licensing.StateNotActivatedWithData || !summary.Copied {
			t.Fatalf("copia: %+v", summary)
		}
		reply := fixture.rejected(t, "POST", "catalog/categories", map[string]any{"name": "Copia", "active": true}, "LICENSE_NOT_ACTIVATED")
		if reply.details["state"] != "not_activated_with_data" || reply.details["copied"] != true {
			t.Fatalf("details de la copia: %v", reply.details)
		}
		if !strings.Contains(string(reply.body), "se copió de otra computadora") {
			t.Fatalf("mensaje de la copia: %s", reply.body)
		}
		fixture.rejected(t, "POST", "payments/shifts", withOperation(map[string]any{"opening_float": "0"}), "LICENSE_NOT_ACTIVATED")
		fixture.alwaysAvailable(t, accounts[0].Account.ID)
		// Un respaldo restaurado en otra computadora: identidad sin licencia y la base con operaciones.
		restored := licensingtest.New(t, fixture.clock)
		restored.Database = licensing.PoolProbe{Pool: fixture.pool}
		fixture.use(restored.Start())
		summary = fixture.licenseOf(t)
		if summary.State != licensing.StateNotActivatedWithData || summary.Copied {
			t.Fatalf("respaldo restaurado: %+v", summary)
		}
		fixture.rejected(t, "POST", "restaurant/accounts", fixture.openAccountBody("Restaurado"), "LICENSE_NOT_ACTIVATED")
		fixture.alwaysAvailable(t, accounts[0].Account.ID)
		fixture.licenses.SetMachine(licensingtest.MachineID)
		fixture.use(original)
	})

	t.Run("Hora del equipo incorrecta: se bloquea todo lo que registra con fecha, también cobrar", func(t *testing.T) {
		fixture.licenses.Accept(fixture.manager, licensingtest.Perpetual(3, 3, 3))
		fixture.openShift(t)
		bill := fixture.pendingAccount(t, "Hora incorrecta")
		if err := fixture.service.CheckLicense(context.Background()); err != nil {
			t.Fatal(err)
		}
		lastActivity := fixture.manager.TrustedTime()
		cursor := fixture.count(t, "SELECT coalesce(max(sequence),0)::int FROM app.realtime_events")
		// El reloj se atrasa 3 horas.
		fixture.clock.Set(lastActivity.Add(-3 * time.Hour))
		reply := fixture.rejected(t, "POST", "payments/accounts/"+bill.Account.ID+"/payments", fixture.payBody(t, bill.Account.ID),
			"SYSTEM_CLOCK_INVALID")
		details := reply.details
		if details["margin_minutes"] != float64(10) || details["license_ok"] != true || details["license_state"] != "active" ||
			details["timezone"] != "America/Monterrey" || details["blocked"] != true {
			t.Fatalf("details: %v", details)
		}
		if local, _ := time.Parse(time.RFC3339Nano, fmt.Sprint(details["local_time"])); !local.Equal(fixture.clock.Now()) {
			t.Fatalf("local_time: %v", details["local_time"])
		}
		if last, _ := time.Parse(time.RFC3339Nano, fmt.Sprint(details["last_activity_time"])); !last.Equal(lastActivity) {
			t.Fatalf("last_activity_time: %v", details["last_activity_time"])
		}
		location, _ := time.LoadLocation("America/Monterrey")
		if details["local_time_text"] != licensing.ScreenTime(fixture.clock.Now(), location) ||
			details["last_activity_time_text"] != licensing.ScreenTime(lastActivity, location) {
			t.Fatalf("textos de la pantalla: %v", details)
		}
		if !strings.Contains(string(reply.body), "la computadora principal tiene la fecha y hora incorrectas") {
			t.Fatalf("mensaje: %s", reply.body)
		}
		fixture.rejected(t, "POST", "restaurant/accounts", fixture.openAccountBody("Hora"), "SYSTEM_CLOCK_INVALID")
		fixture.rejected(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/cancel-account", withOperation(map[string]any{}), "SYSTEM_CLOCK_INVALID")
		fixture.rejected(t, "POST", "payments/shifts/"+fixture.shiftID+"/closing", withOperation(map[string]any{}), "SYSTEM_CLOCK_INVALID")
		fixture.rejected(t, "POST", "inventory/units", map[string]any{"name": "Kilo hora", "abbreviation": "kgh", "active": true}, "SYSTEM_CLOCK_INVALID")
		fixture.rejected(t, "POST", "catalog/categories", map[string]any{"name": "Hora", "active": true}, "SYSTEM_CLOCK_INVALID")
		fixture.rejected(t, "POST", "kitchen/states", map[string]any{}, "SYSTEM_CLOCK_INVALID")
		// Siguen: reportes, historial, respaldos, la licencia, la sesión y las preferencias.
		fixture.alwaysAvailable(t, bill.Account.ID)
		fixture.passesGate(t, "POST", "payments/accounts/"+bill.Account.ID+"/preview", map[string]any{"method_id": fixture.cash,
			"mode": "total"})
		summary := fixture.licenseOf(t)
		if !summary.Clock.Blocked || summary.Clock.LastActivityTime == nil || !summary.Clock.LastActivityTime.Equal(lastActivity) {
			t.Fatalf("auth/me con el bloqueo: %+v", summary.Clock)
		}
		// La auditoría del bloqueo, con las dos fechas, y el aviso a las terminales.
		if err := fixture.service.CheckLicense(context.Background()); err != nil {
			t.Fatal(err)
		}
		if fixture.count(t, `SELECT count(*)::int FROM app.audit_events WHERE action='license.clock_blocked'
            AND after_state->>'local_time' IS NOT NULL AND after_state->>'last_activity_time' IS NOT NULL`) != 1 {
			t.Fatal("el bloqueo no quedó en la auditoría con las dos fechas")
		}
		if fixture.count(t, `SELECT count(*)::int FROM app.realtime_events WHERE sequence>$1 AND kind='license.changed'`, cursor) != 1 {
			t.Fatal("el bloqueo no se avisó a las terminales")
		}
		// Al corregir el reloj el bloqueo se quita solo.
		fixture.clock.Set(lastActivity.Add(time.Minute))
		fixture.allowed(t, "POST", "payments/accounts/"+bill.Account.ID+"/payments", fixture.payBody(t, bill.Account.ID), 200)
		if err := fixture.service.CheckLicense(context.Background()); err != nil {
			t.Fatal(err)
		}
		if fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='license.clock_unblocked'") != 1 ||
			fixture.count(t, `SELECT count(*)::int FROM app.realtime_events WHERE sequence>$1 AND kind='license.changed'`, cursor) != 2 {
			t.Fatal("el desbloqueo no quedó en la auditoría ni se avisó")
		}
	})

	t.Run("Comprobar la hora con internet (§6.3)", func(t *testing.T) {
		// Sin el cliente del servidor de licencias, 503 claro.
		if reply := fixture.send("POST", "license/time-check", map[string]any{}); reply.status != 503 || reply.code != "LICENSE_SERVER_UNAVAILABLE" {
			t.Fatalf("sin cliente: %d %s", reply.status, reply.body)
		}
		// La fecha saltó dos días al futuro, se registró actividad con esa fecha y después se corrigió el reloj.
		real := fixture.clock.Now()
		fixture.clock.Set(real.Add(48 * time.Hour))
		fixture.allowed(t, "POST", "catalog/categories", map[string]any{"name": "En el futuro", "active": true}, 200)
		future := fixture.manager.TrustedTime()
		// Un pedido registrado con esa fecha (la base registra con su propio reloj: aquí se prepara a mano).
		futureInstant := fixture.clock.Now().Add(-time.Hour)
		var orderID string
		if err := fixture.pool.QueryRow(fixture.ctx, `UPDATE app.restaurant_orders SET created_at=$1
            WHERE id=(SELECT id FROM app.restaurant_orders ORDER BY created_at DESC LIMIT 1) RETURNING id::text`, futureInstant).Scan(&orderID); err != nil {
			t.Fatal(err)
		}
		fixture.clock.Set(real)
		fixture.rejected(t, "POST", "catalog/categories", map[string]any{"name": "Bloqueada", "active": true}, "SYSTEM_CLOCK_INVALID")

		// No coincide: responde la hora del servidor y no cambia nada.
		server := &serverClock{instant: real.Add(3 * time.Hour)}
		fixture.service.LicenseServerClock = server
		mismatch := fixture.send("POST", "license/time-check", map[string]any{})
		if mismatch.status != 409 || mismatch.code != "LICENSE_TIME_MISMATCH" || mismatch.details["server_time"] == nil ||
			mismatch.details["server_time_text"] == "" || !fixture.manager.TrustedTime().Equal(future) {
			t.Fatalf("no coincide: %d %s", mismatch.status, mismatch.body)
		}
		if fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='license.clock_internet_check' AND NOT success") != 1 {
			t.Fatal("la comprobación que no coincide queda en la auditoría")
		}

		// Una hora del servidor anterior a la emisión de la licencia vigente (firmada) no se usa, aunque coincida con el
		// reloj.
		fixture.clock.Set(fixture.manager.Status().IssuedAt.Add(-48 * time.Hour))
		server.instant = fixture.clock.Now()
		if reply := fixture.send("POST", "license/time-check", map[string]any{}); reply.status != 502 ||
			reply.code != "LICENSE_RESPONSE_INVALID" || !fixture.manager.TrustedTime().Equal(future) {
			t.Fatalf("hora del servidor anterior a la licencia: %d %s", reply.status, reply.body)
		}
		fixture.clock.Set(real)

		// Coincide: la última hora confiable baja al server_time, se audita con los dos valores y las operaciones con
		// fecha futura se listan sin modificarlas.
		server.instant = real.Add(2 * time.Minute)
		corrected := fixture.allowed(t, "POST", "license/time-check", map[string]any{}, 200)
		var result struct {
			Matches  bool                        `json:"matches"`
			Replaced bool                        `json:"replaced"`
			Clock    administration.ClockDetails `json:"clock"`
		}
		_ = json.Unmarshal(corrected.body, &result)
		if !result.Matches || !result.Replaced || result.Clock.Blocked || !fixture.manager.TrustedTime().Equal(server.instant.UTC()) {
			t.Fatalf("coincide: %s", corrected.body)
		}
		var audited []byte
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT after_state::text FROM app.audit_events WHERE action='license.clock_corrected'
            ORDER BY occurred_at DESC LIMIT 1`).Scan(&audited); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(audited), orderID) || !strings.Contains(string(audited), `"previous_last_activity_time"`) ||
			!strings.Contains(string(audited), `"server_time"`) {
			t.Fatalf("auditoría de la corrección: %s", audited)
		}
		if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_orders WHERE id=$1 AND created_at=$2", orderID, futureInstant) != 1 {
			t.Fatal("la operación con fecha futura no se modifica")
		}
		fixture.allowed(t, "POST", "catalog/categories", map[string]any{"name": "Corregida", "active": true}, 200)
		fixture.service.LicenseServerClock = nil
	})

	t.Run("Desbloqueo de soporte (§6.4) por la API", func(t *testing.T) {
		key := licensingtest.NewSupportKey(t)
		fixture.service.SupportTrust = &key.Trust
		lastActivity := fixture.manager.TrustedTime()
		fixture.clock.Set(lastActivity.Add(-5 * time.Hour))
		fixture.rejected(t, "POST", "catalog/categories", map[string]any{"name": "Soporte", "active": true}, "SYSTEM_CLOCK_INVALID")
		var request struct {
			Code string `json:"request_code"`
		}
		_ = json.Unmarshal(fixture.allowed(t, "POST", "license/support-request", map[string]any{"renew": true}, 200).body, &request)
		if len(request.Code) != 29 {
			t.Fatalf("código de solicitud: %q", request.Code)
		}
		// Alterada: 400 con el motivo, auditada y sin cambiar nada.
		response := key.Respond(t, request.Code, fixture.clock.Now(), fixture.clock.Now())
		altered := fixture.send("POST", "license/support-unlock", map[string]any{"response": response[:len(response)-4] + "AAAA"})
		if altered.status != 400 || altered.code != "SUPPORT_UNLOCK_INVALID" || altered.details["reason"] != "altered" {
			t.Fatalf("alterada: %d %s", altered.status, altered.body)
		}
		// Válida, importada como archivo.
		unlocked := fixture.allowed(t, "POST", "license/support-unlock", map[string]any{"response": response + "\n", "source": "file"}, 200)
		if !strings.Contains(string(unlocked.body), `"accepted":true`) || fixture.manager.Clock().Blocked {
			t.Fatalf("desbloqueo: %s", unlocked.body)
		}
		fixture.allowed(t, "POST", "catalog/categories", map[string]any{"name": "Desbloqueada", "active": true}, 200)
		// Repetida.
		repeated := fixture.send("POST", "license/support-unlock", map[string]any{"response": response})
		if repeated.status != 400 || repeated.details["reason"] != "used" {
			t.Fatalf("repetida: %d %s", repeated.status, repeated.body)
		}
		if fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='license.support_unlocked'") != 1 ||
			fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='license.support_unlock_rejected' AND NOT success") != 2 {
			t.Fatal("cada intento queda en la auditoría")
		}
		fixture.service.SupportTrust = nil
	})

	t.Run("Cupos con los límites reales de la licencia (plan básico y licencia nueva con más cupos)", func(t *testing.T) {
		// Plan básico: 1 caja, 0 comanderos y 0 pantallas de cocina. La puerta deja pasar; el cupo lo decide la Fase D
		// con los cupos del gestor real.
		fixture.licenses.Accept(fixture.manager, licensingtest.Perpetual(1, 0, 0))
		if fixture.shiftID == "" {
			fixture.openShift(t)
		}
		bill := fixture.pendingAccount(t, "Plan básico con turno")
		mode := fixture.send("POST", "kitchen/display-mode", map[string]any{"active": true})
		if mode.status != 409 || mode.code != "LICENSE_SEAT_UNAVAILABLE" || mode.details["seat"] != "kitchen_displays" ||
			mode.details["limit"] != float64(0) {
			t.Fatalf("Preparación en el plan básico: %d %s", mode.status, mode.body)
		}
		fixture.allowed(t, "POST", "payments/accounts/"+bill.Account.ID+"/payments", fixture.payBody(t, bill.Account.ID), 200)
		fixture.closeShift(t)
		// Sin turno, la terminal sería comandero: no hay cupo.
		captured := fixture.send("POST", "restaurant/accounts", fixture.openAccountBody("Plan básico sin turno"))
		if captured.status != 409 || captured.code != "LICENSE_SEAT_UNAVAILABLE" || captured.details["seat"] != "order_takers" {
			t.Fatalf("captura sin turno en el plan básico: %d %s", captured.status, captured.body)
		}
		// Una licencia nueva con un comandero y una pantalla de cocina vale desde la siguiente operación.
		fixture.licenses.Accept(fixture.manager, licensingtest.Perpetual(1, 1, 1))
		fixture.allowed(t, "POST", "restaurant/accounts", fixture.openAccountBody("Comandero"), 200)
		fixture.allowed(t, "POST", "kitchen/display-mode", map[string]any{"active": true}, 200)
		fixture.allowed(t, "POST", "kitchen/display-mode", map[string]any{"active": false}, 200)
	})
}

// String es el cuerpo de la respuesta, para buscar textos.
func (reply licenseReply) String() string {
	return string(reply.body)
}
