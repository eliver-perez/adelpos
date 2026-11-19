//go:build integration

package operations_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"local/ad-el-pos/server/internal/kitchen"
)

// Etapa A2, parte C (impresora de precuentas por dispositivo, A2-D3) contra PostgreSQL con la API real. Un mesero
// entra con PIN en un teléfono e imprime la precuenta de su mesa:
//   - Terminales: la impresora de precuentas solo se asigna a un dispositivo android, activa y que puede imprimir; el
//     campo distingue «no enviado» de null; la configuración, auth/me y restaurant/state la informan con su terminal;
//   - sin impresora asignada: 409 PRINTER_NOT_CONFIGURED con details.scope «device», aunque exista la impresora de
//     tickets general (el escritorio sigue imprimiendo en ella), y sin el permiso, 403;
//   - con la impresora USB de una terminal: la respuesta dice agent_offline y la terminal cuando su agente no consulta,
//     y el agente simulado (print-agent/jobs/claim) recibe los bytes ESC/POS con el título según el nombre visible
//     (Precuenta, Ticket de venta, Cuenta), «Atendió: <mesero>», el folio y las partidas;
//   - con una impresora de red que no responde: documents/status lo dice y documents/retry la reintenta sin motivo en
//     la misma impresora del dispositivo (nunca en la general);
//   - documents/status: solo con payments.tickets.print, del documento de esa cuenta y sin importes.
//
// Usa su propia base desde cero.

// printWaiterPermissions es el rol Mesero de la guía con Para llevar e impresión de la precuenta.
var printWaiterPermissions = append(slices.Clone(takeawayWaiterPermissions), "payments.tickets.print")

type precheckFixture struct {
	*takeawayFixture
	// agent es la terminal de escritorio con la impresora USB; la prueba hace de su agente de impresión.
	agent     testTerminal
	agentName string
	usb       kitchen.Printer
	network   kitchen.Printer
	inactive  kitchen.Printer
	general   kitchen.Printer
	accountID string
	folio     int64
}

// agentCall es una petición del agente de impresión: solo con la credencial de la terminal.
func (fixture *precheckFixture) agentCall(t *testing.T, method, route string, body any) (int, []byte) {
	t.Helper()
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:52910"
	request.Header.Set("X-Device-ID", fixture.agent.deviceID)
	request.Header.Set("X-Device-Token", fixture.agent.deviceToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response.Code, append([]byte{}, response.Body.Bytes()...)
}

// setDevicePrinter guarda la impresora de precuentas del teléfono (con el resto de su configuración igual).
func (fixture *precheckFixture) setDevicePrinter(t *testing.T, printerID any) map[string]any {
	t.Helper()
	return decodeResponse[map[string]any](t, fixture.adminCall(t, "PATCH", "devices/"+fixture.phone.deviceID, map[string]any{
		"name": "Tableta A1-E", "register_id": nil, "keyboard_mode": "auto", "precheck_printer_id": printerID,
	}, 200))
}

// printedPrecheck es la respuesta de precheck/print y documents/retry.
type printedPrecheck struct {
	Status       string  `json:"status"`
	JobID        string  `json:"job_id"`
	TicketID     string  `json:"ticket_id"`
	Problem      *string `json:"problem"`
	PrinterName  *string `json:"printer_name"`
	TerminalName *string `json:"terminal_name"`
}

func (fixture *precheckFixture) printPrecheck(t *testing.T, terminal testTerminal) printedPrecheck {
	t.Helper()
	return decodeResponse[printedPrecheck](t, fixture.call(t, terminal, "POST", "payments/accounts/"+fixture.accountID+"/precheck/print",
		map[string]any{"operation_id": randomID()}, 200))
}

// documentStatus es la respuesta de GET payments/accounts/{id}/documents/status.
type documentStatus struct {
	TicketID     string  `json:"ticket_id"`
	Kind         string  `json:"kind"`
	JobID        *string `json:"job_id"`
	JobState     *string `json:"job_state"`
	JobProblem   *string `json:"job_problem"`
	PrinterName  *string `json:"printer_name"`
	TerminalName *string `json:"terminal_name"`
}

func (fixture *precheckFixture) status(t *testing.T, ticketID string) documentStatus {
	t.Helper()
	return decodeResponse[documentStatus](t, fixture.call(t, fixture.waiter, "GET",
		"payments/accounts/"+fixture.accountID+"/documents/status?ticket_id="+ticketID, nil, 200))
}

// claimedText reclama el siguiente trabajo de la terminal como su agente, lo confirma y devuelve sus bytes ESC/POS
// decodificados con la página de códigos del controlador de Go (CP850).
func (fixture *precheckFixture) claimedText(t *testing.T, jobID string) string {
	t.Helper()
	code, response := fixture.agentCall(t, "POST", "print-agent/jobs/claim", map[string]any{"claim_id": randomID()})
	claimed := decodeResponse[struct {
		Job *kitchen.AgentJob `json:"job"`
	}](t, response).Job
	if code != 200 || claimed == nil || claimed.ID != jobID || claimed.Queue != "POS_80_A2C" {
		t.Fatalf("reclamo del agente: %d %s", code, response)
	}
	data, err := base64.StdEncoding.DecodeString(claimed.Data)
	if err != nil || len(data) != claimed.Size {
		t.Fatalf("bytes del trabajo: %v", err)
	}
	if code, response := fixture.agentCall(t, "POST", "print-agent/jobs/"+claimed.ID+"/result", map[string]any{
		"attempt_id": claimed.AttemptID, "outcome": "submitted", "reference": "POS_80_A2C-1",
	}); code != 200 {
		t.Fatalf("resultado del agente: %d %s", code, response)
	}
	text, err := charmap.CodePage850.NewDecoder().Bytes(data)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

func (fixture *precheckFixture) createPrinter(t *testing.T, body map[string]any) kitchen.Printer {
	t.Helper()
	return decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", body, 200))
}

func (fixture *precheckFixture) jobs(t *testing.T) int {
	t.Helper()
	return fixture.count(t, "SELECT count(*)::int FROM app.kitchen_print_jobs")
}

func TestStageA2PrecheckPrinter(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &precheckFixture{takeawayFixture: &takeawayFixture{waiterModeFixture: &waiterModeFixture{restaurantFixture: newFixture(t)}}}
	fixture.seedCatalog(t)
	inventoryUser(t, fixture.restaurantFixture, "mesero_a2c", printWaiterPermissions)
	inventoryUser(t, fixture.restaurantFixture, "mesero_a2c_sin", waiterPermissions)
	fixture.agent = inventoryUser(t, fixture.restaurantFixture, "caja_sur_a2c", []string{"accounts.read"})
	fixture.agentName = "Terminal caja_sur_a2c"
	fixture.waiterID = fixture.userID(t, "mesero_a2c")
	otherID := fixture.userID(t, "mesero_a2c_sin")
	fixture.adminCall(t, "PUT", "users/"+fixture.waiterID+"/pin", map[string]any{"pin": "4829", "short_name": "Ana", "badge_color": ""}, 200)
	fixture.adminCall(t, "PUT", "users/"+otherID+"/pin", map[string]any{"pin": "6152", "short_name": "Beto", "badge_color": ""}, 200)
	fixture.phone = fixture.waiterPhone(t)
	fixture.waiter, fixture.credential = fixture.pinLogin(t, fixture.waiterID, "4829")
	fixture.other, fixture.otherCredential = fixture.pinLogin(t, otherID, "6152")
	// La Caja Sur informa su impresora USB y se registra como «Impresora USB de una terminal».
	if code, response := fixture.agentCall(t, "PUT", "print-agent/printers", map[string]any{"printers": []map[string]any{
		{"queue": "POS_80_A2C", "model": "POS-80 Series", "connection": "usb", "printer_type": 36876},
	}}); code != 200 {
		t.Fatalf("informe de impresoras: %d %s", code, response)
	}
	fixture.usb = fixture.createPrinter(t, map[string]any{"name": "USB Caja Sur A2-C", "transport": "agent",
		"destination": "POS_80_A2C", "paper_width": 80, "copies": 1, "active": true, "agent_device_id": fixture.agent.deviceID})
	fixture.network = fixture.createPrinter(t, map[string]any{"name": "Red A2-C", "transport": "escpos_tcp",
		"destination": "127.0.0.1", "port": 9109, "paper_width": 80, "copies": 1, "active": true})
	fixture.inactive = fixture.createPrinter(t, map[string]any{"name": "Apagada A2-C", "transport": "escpos_tcp",
		"destination": "127.0.0.1", "port": 9108, "paper_width": 80, "copies": 1, "active": false})
	// La impresora de tickets general de Cobros: la de la caja y el escritorio, nunca la del teléfono.
	fixture.general = fixture.createPrinter(t, map[string]any{"name": "General A2-C", "transport": "escpos_tcp",
		"destination": "127.0.0.1", "port": 9107, "paper_width": 80, "copies": 1, "active": true})
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.payment_settings SET receipt_printer_id=$1", fixture.general.ID); err != nil {
		t.Fatal(err)
	}
	// La mesa del mesero con dos partidas enviadas.
	opened := fixture.open(t, fixture.waiter, nil, draftLine(fixture.product, 1), draftLine(fixture.fries, 2))
	fixture.accountID, fixture.folio = opened.Account.ID, opened.Account.Folio
	fixture.confirm(t, fixture.waiter, fixture.accountID)

	t.Run("Terminales: impresora de precuentas solo en un android, activa y que imprime", func(t *testing.T) {
		testStageA2DevicePrinterSettings(t, fixture)
	})
	t.Run("sin impresora asignada: PRINTER_NOT_CONFIGURED del dispositivo aunque haya general", func(t *testing.T) {
		testStageA2PrecheckWithoutPrinter(t, fixture)
	})
	t.Run("impresora USB de una terminal: desconectada y bytes ESC/POS con el nombre visible", func(t *testing.T) {
		testStageA2PrecheckAgentBytes(t, fixture)
	})
	t.Run("impresora de red que no responde: estado y reintento en la misma impresora", func(t *testing.T) {
		testStageA2PrecheckRetry(t, fixture)
	})
	t.Run("documents/status: permiso de imprimir, documento de la cuenta y sin importes", func(t *testing.T) {
		testStageA2DocumentStatus(t, fixture)
	})
}

func testStageA2DevicePrinterSettings(t *testing.T, fixture *precheckFixture) {
	route := "devices/" + fixture.phone.deviceID
	saved := fixture.setDevicePrinter(t, fixture.usb.ID)
	if saved["precheck_printer_id"] != fixture.usb.ID || saved["precheck_printer_name"] != "USB Caja Sur A2-C" ||
		saved["precheck_printer_terminal_name"] != fixture.agentName {
		t.Fatalf("impresora de precuentas guardada: %v", saved)
	}
	// Sin el campo (un escritorio anterior) se conserva; con null se quita.
	kept := decodeResponse[map[string]any](t, fixture.adminCall(t, "PATCH", route, map[string]any{
		"name": "Tableta A1-E", "register_id": nil, "keyboard_mode": "auto"}, 200))
	if kept["precheck_printer_id"] != fixture.usb.ID {
		t.Fatalf("sin el campo debe conservarse: %v", kept)
	}
	if cleared := fixture.setDevicePrinter(t, nil); cleared["precheck_printer_id"] != nil || cleared["precheck_printer_name"] != nil {
		t.Fatalf("null debe quitarla: %v", cleared)
	}
	// Una impresora de red no tiene terminal.
	if network := fixture.setDevicePrinter(t, fixture.network.ID); network["precheck_printer_name"] != "Red A2-C" ||
		network["precheck_printer_terminal_name"] != nil {
		t.Fatalf("impresora de red: %v", network)
	}
	// Solo una impresora que existe y puede imprimir, y solo en un dispositivo android.
	for name, value := range map[string]any{
		"desactivada": fixture.inactive.ID, "que no existe": randomID(), "identificador inválido": "impresora", "número": 12,
	} {
		code, response := fixture.raw(fixture.admin, "", "PATCH", route, map[string]any{
			"name": "Tableta A1-E", "register_id": nil, "keyboard_mode": "auto", "precheck_printer_id": value})
		if code != 400 {
			t.Fatalf("impresora %s: %d %s", name, code, response)
		}
	}
	code, response := fixture.raw(fixture.admin, "", "PATCH", "devices/"+fixture.agent.deviceID, map[string]any{
		"name": "Terminal caja_sur_a2c", "register_id": nil, "keyboard_mode": "auto", "precheck_printer_id": fixture.general.ID})
	if code != 400 || !strings.Contains(string(response), "solo se asigna a teléfonos y tabletas") {
		t.Fatalf("impresora de precuentas en una terminal de escritorio: %d %s", code, response)
	}
	fixture.setDevicePrinter(t, fixture.usb.ID)
	// «Ver configuración»: la asignada con su problema y las que se le pueden elegir; en el escritorio, nada.
	type option struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		TerminalName string `json:"terminal_name"`
		Problem      string `json:"problem"`
	}
	configuration := decodeResponse[struct {
		PrecheckPrinter *option  `json:"precheck_printer"`
		Options         []option `json:"precheck_printer_options"`
	}](t, fixture.adminCall(t, "GET", route+"/configuration", nil, 200))
	problems := map[string]string{}
	for _, choice := range configuration.Options {
		problems[choice.Name] = choice.Problem
	}
	if configuration.PrecheckPrinter == nil || configuration.PrecheckPrinter.ID != fixture.usb.ID ||
		configuration.PrecheckPrinter.TerminalName != fixture.agentName || configuration.PrecheckPrinter.Problem != "" ||
		len(configuration.Options) != 4 || problems["Apagada A2-C"] != kitchen.ProblemInactive || problems["Red A2-C"] != "" {
		t.Fatalf("configuración del teléfono: %+v", configuration)
	}
	desktop := decodeResponse[struct {
		PrecheckPrinter *option  `json:"precheck_printer"`
		Options         []option `json:"precheck_printer_options"`
	}](t, fixture.adminCall(t, "GET", "devices/"+fixture.agent.deviceID+"/configuration", nil, 200))
	if desktop.PrecheckPrinter != nil || desktop.Options == nil || len(desktop.Options) != 0 {
		t.Fatalf("configuración de una terminal de escritorio: %+v", desktop)
	}
	// El teléfono la ve en su terminal (auth/me y restaurant/state), para decir a dónde se envió.
	for _, path := range []string{"auth/me", "restaurant/state"} {
		terminal := decodeResponse[struct {
			Terminal struct {
				PrecheckPrinterID *string `json:"precheck_printer_id"`
				PrecheckPrinter   *struct {
					ID           string  `json:"id"`
					Name         string  `json:"name"`
					TerminalName *string `json:"terminal_name"`
				} `json:"precheck_printer"`
			} `json:"terminal"`
		}](t, fixture.call(t, fixture.waiter, "GET", path, nil, 200)).Terminal
		if terminal.PrecheckPrinterID == nil || *terminal.PrecheckPrinterID != fixture.usb.ID || terminal.PrecheckPrinter == nil ||
			terminal.PrecheckPrinter.Name != "USB Caja Sur A2-C" || terminal.PrecheckPrinter.TerminalName == nil ||
			*terminal.PrecheckPrinter.TerminalName != fixture.agentName {
			t.Fatalf("terminal de %s: %+v", path, terminal)
		}
	}
	if fixture.count(t, `SELECT count(*)::int FROM app.audit_events WHERE action='device.updated' AND entity_id=$1
        AND after_state->>'precheck_printer_id'=$2`, fixture.phone.deviceID, fixture.usb.ID) < 2 {
		t.Fatal("el cambio de impresora de precuentas no quedó en el Registro de movimientos")
	}
}

func testStageA2PrecheckWithoutPrinter(t *testing.T, fixture *precheckFixture) {
	fixture.setDevicePrinter(t, nil)
	jobs := fixture.jobs(t)
	code, response := fixture.raw(fixture.waiter, "", "POST", "payments/accounts/"+fixture.accountID+"/precheck/print",
		map[string]any{"operation_id": randomID()})
	problem := decodeResponse[printerError](t, response)
	if code != 409 || problem.Error.Code != "PRINTER_NOT_CONFIGURED" || problem.Error.Details["scope"] != "device" ||
		problem.Error.Details["document"] != "precheck" || problem.Error.Details["problem"] != "missing" ||
		problem.Error.Message != "Este dispositivo no tiene impresora asignada. Pide al administrador que la elija en Terminales." {
		t.Fatalf("precuenta sin impresora del dispositivo: %d %s", code, response)
	}
	if fixture.jobs(t) != jobs || fixture.count(t, "SELECT count(*)::int FROM app.payment_documents WHERE account_id=$1", fixture.accountID) != 0 {
		t.Fatal("sin impresora no debe encolarse nada")
	}
	// Sin el permiso de imprimir: 403, también con la impresora asignada.
	fixture.setDevicePrinter(t, fixture.network.ID)
	if code, response := fixture.raw(fixture.other, "", "POST", "payments/accounts/"+fixture.accountID+"/precheck/print",
		map[string]any{"operation_id": randomID()}); code != 403 || errorCode(t, response) != "PERMISSION_DENIED" {
		t.Fatalf("precuenta sin payments.tickets.print: %d %s", code, response)
	}
	// Una impresora asignada que dejó de servir: PRINTER_UNAVAILABLE del dispositivo (no la general).
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_printers SET active=false WHERE id=$1", fixture.network.ID); err != nil {
		t.Fatal(err)
	}
	code, response = fixture.raw(fixture.waiter, "", "POST", "payments/accounts/"+fixture.accountID+"/precheck/print",
		map[string]any{"operation_id": randomID()})
	problem = decodeResponse[printerError](t, response)
	if code != 409 || problem.Error.Code != "PRINTER_UNAVAILABLE" || problem.Error.Details["scope"] != "device" ||
		problem.Error.Details["printer_name"] != "Red A2-C" || problem.Error.Details["problem"] != kitchen.ProblemInactive {
		t.Fatalf("precuenta con la impresora del dispositivo desactivada: %d %s", code, response)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_printers SET active=true WHERE id=$1", fixture.network.ID); err != nil {
		t.Fatal(err)
	}
	// El escritorio (sin impresora de dispositivo) sigue imprimiendo en la general, sin details.scope.
	desktop := fixture.printPrecheck(t, fixture.admin)
	if desktop.PrinterName == nil || *desktop.PrinterName != "General A2-C" || desktop.TerminalName != nil {
		t.Fatalf("precuenta desde el escritorio: %+v", desktop)
	}
}

func testStageA2PrecheckAgentBytes(t *testing.T, fixture *precheckFixture) {
	fixture.setDevicePrinter(t, fixture.usb.ID)
	// La Caja Sur lleva más de 30 s sin consultar: la respuesta ya dice que está desconectada.
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.devices SET print_agent_seen_at=now()-interval '5 minutes' WHERE id=$1",
		fixture.agent.deviceID); err != nil {
		t.Fatal(err)
	}
	offline := fixture.printPrecheck(t, fixture.waiter)
	if offline.Status != "pending" || offline.Problem == nil || *offline.Problem != kitchen.ProblemAgentOffline ||
		offline.PrinterName == nil || *offline.PrinterName != "USB Caja Sur A2-C" || offline.TerminalName == nil ||
		*offline.TerminalName != fixture.agentName {
		t.Fatalf("precuenta con la terminal desconectada: %+v", offline)
	}
	status := fixture.status(t, offline.TicketID)
	if status.Kind != "precheck" || status.JobState == nil || *status.JobState != "pending" || status.JobProblem == nil ||
		*status.JobProblem != kitchen.ProblemAgentOffline || status.TerminalName == nil || *status.TerminalName != fixture.agentName {
		t.Fatalf("estado con la terminal desconectada: %+v", status)
	}
	// La terminal vuelve: su agente recibe los bytes ESC/POS de la precuenta con «Precuenta» y el mesero que la pidió.
	text := fixture.claimedText(t, offline.JobID)
	folio := strconv.FormatInt(fixture.folio, 10)
	for _, expected := range []string{"PRECUENTA", "Atendió:", "Prueba mesero_a2c", "Folio", folio, "Filete", "Papas producto"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("la precuenta impresa no dice %q:\n%s", expected, text)
		}
	}
	if status := fixture.status(t, offline.TicketID); status.JobState == nil || *status.JobState != "submitted" || status.JobProblem != nil {
		t.Fatalf("estado después de que el agente la envió: %+v", status)
	}
	// Con otro nombre visible, el impreso lo dice (el botón del teléfono también, con establishment.precheck_name).
	for key, title := range map[string]string{"sales_ticket": "TICKET DE VENTA", "bill": "CUENTA"} {
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.payment_settings SET precheck_name=$1", key); err != nil {
			t.Fatal(err)
		}
		printed := fixture.printPrecheck(t, fixture.waiter)
		if printed.Problem != nil {
			t.Fatalf("la terminal ya consultó: %+v", printed)
		}
		text := fixture.claimedText(t, printed.JobID)
		if !strings.Contains(text, title) || strings.Contains(text, "PRECUENTA") || !strings.Contains(text, "Prueba mesero_a2c") {
			t.Fatalf("título impreso con %s: %s", key, text)
		}
	}
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.payment_settings SET precheck_name='precheck'"); err != nil {
		t.Fatal(err)
	}
	// Ninguno salió por la impresora de tickets general.
	if fixture.count(t, `SELECT count(*)::int FROM app.kitchen_print_jobs j JOIN app.payment_documents d ON d.ticket_id=j.ticket_id
        WHERE d.device_id=$1 AND j.printer_id<>$2`, fixture.phone.deviceID, fixture.usb.ID) != 0 {
		t.Fatal("una precuenta del teléfono salió en otra impresora")
	}
}

func testStageA2PrecheckRetry(t *testing.T, fixture *precheckFixture) {
	fixture.setDevicePrinter(t, fixture.network.ID)
	printed := fixture.printPrecheck(t, fixture.waiter)
	if printed.Problem != nil || printed.PrinterName == nil || *printed.PrinterName != "Red A2-C" || printed.TerminalName != nil {
		t.Fatalf("precuenta en la impresora de red: %+v", printed)
	}
	// La impresora no responde (el adaptador controlado de la prueba responde error, nada sale).
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='cancelled' WHERE state IN ('pending','error') AND id<>$1",
		printed.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&kitchen.Worker{Pool: fixture.pool, Transport: &recordingTransport{state: "error"}}).PrintNext(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	status := fixture.status(t, printed.TicketID)
	if status.JobState == nil || *status.JobState != "error" || status.JobProblem == nil || *status.JobProblem != kitchen.ProblemUnreachable ||
		status.PrinterName == nil || *status.PrinterName != "Red A2-C" {
		t.Fatalf("estado con la impresora que no responde: %+v", status)
	}
	// «Reintentar» sin motivo sale en la misma impresora del dispositivo, aunque la general sea otra.
	retried := decodeResponse[printedPrecheck](t, fixture.call(t, fixture.waiter, "POST",
		"payments/accounts/"+fixture.accountID+"/documents/retry", map[string]any{"operation_id": randomID(), "ticket_id": printed.TicketID}, 200))
	if retried.TicketID != printed.TicketID || retried.PrinterName == nil || *retried.PrinterName != "Red A2-C" || retried.Problem != nil {
		t.Fatalf("reintento: %+v", retried)
	}
	var printerID string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT printer_id::text FROM app.kitchen_print_jobs WHERE id=$1", retried.JobID).
		Scan(&printerID); err != nil || printerID != fixture.network.ID {
		t.Fatalf("impresora del reintento: %s %v", printerID, err)
	}
	// Sin impresora del dispositivo el reintento tampoco pasa a la general.
	if _, err := (&kitchen.Worker{Pool: fixture.pool, Transport: &recordingTransport{state: "error"}}).PrintNext(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	fixture.setDevicePrinter(t, nil)
	code, response := fixture.raw(fixture.waiter, "", "POST", "payments/accounts/"+fixture.accountID+"/documents/retry",
		map[string]any{"operation_id": randomID(), "ticket_id": printed.TicketID})
	problem := decodeResponse[printerError](t, response)
	if code != 409 || problem.Error.Code != "PRINTER_NOT_CONFIGURED" || problem.Error.Details["scope"] != "device" {
		t.Fatalf("reintento sin impresora del dispositivo: %d %s", code, response)
	}
}

func testStageA2DocumentStatus(t *testing.T, fixture *precheckFixture) {
	var ticket string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT ticket_id::text FROM app.payment_documents WHERE account_id=$1 LIMIT 1",
		fixture.accountID).Scan(&ticket); err != nil {
		t.Fatal(err)
	}
	route := "payments/accounts/" + fixture.accountID + "/documents/status?ticket_id="
	raw := fixture.call(t, fixture.waiter, "GET", route+ticket, nil, 200)
	for _, money := range []string{"total", "paid", "balance", "amount", "payments"} {
		if bytes.Contains(raw, []byte(`"`+money+`"`)) {
			t.Fatalf("el estado del documento no debe traer importes: %s", raw)
		}
	}
	if code, response := fixture.raw(fixture.other, "", "GET", route+ticket, nil); code != 403 || errorCode(t, response) != "PERMISSION_DENIED" {
		t.Fatalf("estado sin payments.tickets.print: %d %s", code, response)
	}
	if code, _ := fixture.raw(fixture.waiter, "", "GET", route+"documento", nil); code != 400 {
		t.Fatalf("estado con un identificador inválido: %d", code)
	}
	if code, _ := fixture.raw(fixture.waiter, "", "GET", route+randomID(), nil); code != 404 {
		t.Fatalf("estado de un documento que no es de la cuenta: %d", code)
	}
	other := fixture.open(t, fixture.admin, nil, draftLine(fixture.fries, 1))
	if code, _ := fixture.raw(fixture.waiter, "", "GET", "payments/accounts/"+other.Account.ID+"/documents/status?ticket_id="+ticket, nil); code != 404 {
		t.Fatalf("estado de un documento de otra cuenta: %d", code)
	}
}
