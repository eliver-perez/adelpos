//go:build integration

package operations_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa 9G, fase E (§6, decisiones D10 y D13): impresión con varias terminales contra la API real y PostgreSQL. Dos
// cajas con su terminal: la Caja Sur tiene una impresora USB que imprime su agente de impresión (la aplicación de esa
// terminal), la Caja Norte no. Comprueba:
//   - la terminal informa sus impresoras y solo las suyas; la impresora de la terminal se elige de esa lista;
//   - el trabajador del servidor nunca envía los trabajos de una impresora de terminal (solo recoge la lease vencida);
//   - con la terminal desconectada el trabajo espera con el problema «desconectada» y el nombre de la terminal; al
//     volver, su agente lo reclama con lease y lo confirma (aceptado, error confirmado con reintento, incierto);
//   - repetir el reclamo o el resultado no duplica nada; un resultado con la lease vencida deja el trabajo incierto;
//   - otra terminal no reclama ni confirma trabajos ajenos;
//   - el cajón de la Caja Sur se abre por su agente con el pulso exacto y nunca se reintenta solo;
//   - cada caja usa su impresora de tickets y, sin una propia, la general.
//
// Usa su propia base desde cero.

type printAgentFixture struct {
	*paymentsFixture
	regions map[string]string
	north   testTerminal
	south   testTerminal
	usb     kitchen.Printer
	network kitchen.Printer
	worker  *kitchen.Worker
	sent    *agentWatchTransport
}

// agentWatchTransport cuenta los envíos del trabajador del servidor sin imprimir nada.
type agentWatchTransport struct {
	mutex sync.Mutex
	jobs  []string
}

func (transport *agentWatchTransport) Submit(_ context.Context, job kitchen.PrintJob) kitchen.Outcome {
	transport.mutex.Lock()
	defer transport.mutex.Unlock()
	transport.jobs = append(transport.jobs, job.ID)
	return kitchen.Outcome{State: "submitted", Accepted: true, Reference: "controlado-1", Message: "Destino controlado de la prueba."}
}

func (transport *agentWatchTransport) sentJob(id string) bool {
	transport.mutex.Lock()
	defer transport.mutex.Unlock()
	for _, job := range transport.jobs {
		if job == id {
			return true
		}
	}
	return false
}

// agentCall es una petición del agente de impresión: solo con la credencial de la terminal, sin sesión de usuario.
func (fixture *printAgentFixture) agentCall(t *testing.T, terminal testTerminal, method, route string, body any) (int, []byte) {
	t.Helper()
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:52900"
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response.Code, append([]byte{}, response.Body.Bytes()...)
}

type claimedJob struct {
	Job *kitchen.AgentJob `json:"job"`
}

func (fixture *printAgentFixture) claim(t *testing.T, terminal testTerminal, claimID string) *kitchen.AgentJob {
	t.Helper()
	status, body := fixture.agentCall(t, terminal, "POST", "print-agent/jobs/claim", map[string]any{"claim_id": claimID})
	if status != 200 {
		t.Fatalf("reclamo del agente: %d %s", status, body)
	}
	return decodeResponse[claimedJob](t, body).Job
}

type agentResult struct {
	State    string `json:"state"`
	Recorded bool   `json:"recorded"`
	Reason   string `json:"reason"`
}

func (fixture *printAgentFixture) report(t *testing.T, terminal testTerminal, job *kitchen.AgentJob, body map[string]any, status int) agentResult {
	t.Helper()
	body["attempt_id"] = job.AttemptID
	code, response := fixture.agentCall(t, terminal, "POST", "print-agent/jobs/"+job.ID+"/result", body)
	if code != status {
		t.Fatalf("resultado del agente: esperado %d recibido %d: %s", status, code, response)
	}
	if status != 200 {
		return agentResult{}
	}
	return decodeResponse[agentResult](t, response)
}

type queuedJob struct {
	ID           string  `json:"id"`
	State        string  `json:"state"`
	Attempts     int     `json:"attempts"`
	PrinterID    string  `json:"printer_id"`
	Problem      *string `json:"problem"`
	TerminalName *string `json:"terminal_name"`
	LastError    string  `json:"last_error"`
}

func (fixture *printAgentFixture) job(t *testing.T, id string) queuedJob {
	t.Helper()
	return decodeResponse[struct {
		Job queuedJob `json:"job"`
	}](t, fixture.adminCall(t, "GET", "kitchen/jobs/"+id, nil, 200)).Job
}

func (fixture *printAgentFixture) testPrint(t *testing.T, printerID string) string {
	t.Helper()
	return decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/test-print", map[string]any{"printer_id": printerID}, 200)).ID
}

func (fixture *printAgentFixture) exec(t *testing.T, statement string, args ...any) {
	t.Helper()
	if _, err := fixture.pool.Exec(fixture.ctx, statement, args...); err != nil {
		t.Fatal(err)
	}
}

func problemOf(job queuedJob) string {
	if job.Problem == nil {
		return ""
	}
	return *job.Problem
}

// usbPrinters son las impresoras que informa la computadora de la Caja Sur: una térmica USB y una de oficina por
// la red (que no sirve para tickets).
var usbPrinters = []map[string]any{
	{"queue": "POS_80_9GE", "model": "POS-80 Series", "connection": "usb", "printer_type": 36876},
	{"queue": "Oficina_9GE", "model": "Generic PostScript Printer", "connection": "ipp", "printer_type": nil},
}

func TestStage9GPrintAgent(t *testing.T) {
	resetStage9GDatabase(t)
	base := newFixture(t)
	base.seedCatalog(t)
	fixture := &printAgentFixture{
		paymentsFixture: &paymentsFixture{restaurantFixture: base, methods: map[string]payments.Method{}, registers: map[string]string{}},
		regions:         map[string]string{},
		sent:            &agentWatchTransport{},
	}
	fixture.worker = &kitchen.Worker{Pool: fixture.pool, Transport: fixture.sent}
	for _, name := range []string{"Caja Norte 9G-E", "Caja Sur 9G-E"} {
		fixture.regions[name] = decodeResponse[struct {
			ID string `json:"id"`
		}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": name, "description": "", "active": true}, 200)).ID
	}
	permissions := append([]string{"cash.drawer.open", "payments.tickets.print", "accounts.open", "orders.capture",
		"orders.confirm", "catalog.read", "restaurant.tables.read"}, cashierPermissions...)
	fixture.north = newCashier(t, fixture.paymentsFixture, "cajera_norte_9ge", permissions, fixture.regions["Caja Norte 9G-E"])
	fixture.south = newCashier(t, fixture.paymentsFixture, "cajero_sur_9ge", permissions, fixture.regions["Caja Sur 9G-E"])
	southName := "Terminal cajero_sur_9ge"

	t.Run("la terminal informa sus impresoras y solo las suyas", func(t *testing.T) {
		status, body := fixture.agentCall(t, fixture.south, "PUT", "print-agent/printers", map[string]any{"printers": usbPrinters})
		if status != 200 {
			t.Fatalf("informe de impresoras: %d %s", status, body)
		}
		reported := decodeResponse[struct {
			Printers []kitchen.TerminalQueue `json:"printers"`
		}](t, body).Printers
		if len(reported) != 2 || reported[0].Queue != "Oficina_9GE" || reported[0].Usable || !reported[1].Usable {
			t.Fatalf("impresoras informadas: %+v", reported)
		}
		// La Caja Norte informa las suyas: la lista de la Caja Sur no cambia (no hay forma de informar las de otra).
		if status, body := fixture.agentCall(t, fixture.north, "PUT", "print-agent/printers", map[string]any{"printers": []any{}}); status != 200 {
			t.Fatalf("informe vacío: %d %s", status, body)
		}
		if count := fixture.countRows(t, "SELECT count(*) FROM app.device_local_printers WHERE device_id=$1", fixture.south.deviceID); count != 2 {
			t.Fatalf("la lista de la Caja Sur cambió: %d", count)
		}
		// Sin credencial de terminal no se informa nada; una lista inválida se rechaza.
		if status, _ := fixture.agentCall(t, testTerminal{}, "PUT", "print-agent/printers", map[string]any{"printers": usbPrinters}); status != 401 {
			t.Fatalf("informe sin credencial: %d", status)
		}
		for _, invalid := range []map[string]any{
			{"queue": "POS 80; rm -rf", "model": "", "connection": "usb"},
			{"queue": "POS_80", "model": "", "connection": "usb://Printer/POS-80?serial=123"},
		} {
			if status, _ := fixture.agentCall(t, fixture.south, "PUT", "print-agent/printers", map[string]any{"printers": []any{invalid}}); status != 400 {
				t.Fatalf("impresora inválida aceptada: %v (%d)", invalid, status)
			}
		}
		// El informe queda en el Registro de movimientos con la terminal como autora, solo cuando la lista cambia.
		if count := fixture.countRows(t, `SELECT count(*) FROM app.audit_events WHERE action='device.local_printers_reported'
            AND device_id=$1 AND actor_kind='device'`, fixture.south.deviceID); count != 1 {
			t.Fatalf("auditoría del informe: %d", count)
		}
		fixture.agentCall(t, fixture.south, "PUT", "print-agent/printers", map[string]any{"printers": usbPrinters})
		if count := fixture.countRows(t, `SELECT count(*) FROM app.audit_events WHERE action='device.local_printers_reported'
            AND device_id=$1`, fixture.south.deviceID); count != 1 {
			t.Fatalf("un informe igual no se audita otra vez: %d", count)
		}
	})

	t.Run("la impresora de una terminal se elige de lo que informó", func(t *testing.T) {
		printer := func(destination string) map[string]any {
			return map[string]any{"name": "USB Caja Sur 9G-E", "transport": "agent", "destination": destination, "paper_width": 80,
				"copies": 1, "active": true, "agent_device_id": fixture.south.deviceID}
		}
		for destination, reason := range map[string]string{
			"Oficina_9GE": "no sirve para tickets",
			"Otra_9GE":    "no informó esa impresora",
		} {
			body := fixture.mutate(t, "POST", "kitchen/printers", printer(destination), 400)
			if !strings.Contains(string(body), reason) {
				t.Fatalf("%s: %s", destination, body)
			}
		}
		fixture.usb = decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", printer("POS_80_9GE"), 200))
		if !fixture.usb.AgentPrinter() || fixture.usb.TerminalName != southName {
			t.Fatalf("impresora de la terminal: %+v", fixture.usb)
		}
		// En Cocina › Impresoras y en Terminales se ve en qué terminal está.
		listed := decodeResponse[[]kitchen.Printer](t, fixture.adminCall(t, "GET", "kitchen/printers", nil, 200))
		if len(listed) != 1 || listed[0].TerminalName != southName {
			t.Fatalf("lista de impresoras: %+v", listed)
		}
		terminals := decodeResponse[[]kitchen.TerminalPrinterSet](t, fixture.adminCall(t, "GET", "kitchen/terminal-printers", nil, 200))
		found := false
		for _, set := range terminals {
			if set.DeviceID == fixture.south.deviceID {
				found = len(set.Queues) == 2 && len(set.Printers) == 1 && set.Printers[0].Name == "USB Caja Sur 9G-E"
			}
		}
		if !found {
			t.Fatalf("impresoras por terminal: %+v", terminals)
		}
		configuration := decodeResponse[struct {
			Printers      []kitchen.AssignedOutput `json:"printers"`
			LocalPrinters struct {
				Queues []kitchen.TerminalQueue `json:"queues"`
			} `json:"local_printers"`
		}](t, fixture.adminCall(t, "GET", "devices/"+fixture.south.deviceID+"/configuration", nil, 200))
		if len(configuration.Printers) != 1 || len(configuration.LocalPrinters.Queues) != 2 {
			t.Fatalf("configuración de la terminal: %+v", configuration)
		}
		fixture.network = decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", map[string]any{
			"name": "Tickets generales 9G-E", "transport": "escpos_tcp", "destination": "127.0.0.1", "port": 9109, "paper_width": 80,
			"copies": 1, "active": true,
		}, 200))
	})

	t.Run("con la terminal desconectada el trabajo espera y lo dice", func(t *testing.T) {
		jobID := fixture.testPrint(t, fixture.usb.ID)
		job := fixture.job(t, jobID)
		if job.State != "pending" || problemOf(job) != kitchen.ProblemAgentOffline || job.TerminalName == nil || *job.TerminalName != southName {
			t.Fatalf("trabajo para una terminal desconectada: %+v", job)
		}
		// El trabajador del servidor nunca envía un trabajo de la impresora de una terminal.
		for range 3 {
			if _, err := fixture.worker.PrintNext(fixture.ctx); err != nil {
				t.Fatal(err)
			}
		}
		if fixture.sent.sentJob(jobID) || fixture.job(t, jobID).State != "pending" {
			t.Fatal("el servidor tomó el trabajo de la impresora de una terminal")
		}
		// Otra terminal no reclama ni confirma trabajos ajenos.
		if other := fixture.claim(t, fixture.north, randomID()); other != nil {
			t.Fatalf("la Caja Norte reclamó un trabajo de la Caja Sur: %+v", other)
		}
		// Al volver, el agente de la Caja Sur lo reclama con su lease y los bytes del controlador ESC/POS.
		claimID := randomID()
		claimed := fixture.claim(t, fixture.south, claimID)
		if claimed == nil || claimed.ID != jobID || claimed.Queue != "POS_80_9GE" || claimed.Title != "ADEL-"+jobID || claimed.Drawer {
			t.Fatalf("trabajo reclamado: %+v", claimed)
		}
		data, err := base64.StdEncoding.DecodeString(claimed.Data)
		var content string
		if scan := fixture.pool.QueryRow(fixture.ctx, "SELECT content FROM app.kitchen_print_jobs WHERE id=$1", jobID).Scan(&content); err != nil || scan != nil {
			t.Fatal(err, scan)
		}
		usb := fixture.usb
		usb.TerminalName = ""
		if !bytes.Equal(data, kitchen.ESCPosBytes(content, usb)) || claimed.Size != len(data) {
			t.Fatal("los bytes del agente deben ser los del controlador ESC/POS de Go")
		}
		job = fixture.job(t, jobID)
		if job.State != "processing" || job.Attempts != 1 || problemOf(job) != "" {
			t.Fatalf("trabajo en proceso con la terminal conectada: %+v", job)
		}
		// Repetir el reclamo (respuesta perdida) devuelve el mismo intento; otro reclamo no toma otro trabajo de la
		// misma impresora mientras este no termine.
		again := fixture.claim(t, fixture.south, claimID)
		if again == nil || again.ID != jobID || again.AttemptID != claimed.AttemptID {
			t.Fatalf("reclamo repetido: %+v", again)
		}
		second := fixture.testPrint(t, fixture.usb.ID)
		if busy := fixture.claim(t, fixture.south, randomID()); busy != nil {
			t.Fatalf("dos trabajos de la misma impresora a la vez: %+v", busy)
		}
		if count := fixture.countRows(t, "SELECT count(*) FROM app.kitchen_print_attempts WHERE job_id=$1", jobID); count != 1 {
			t.Fatalf("intentos del trabajo: %d", count)
		}
		// La Caja Norte no confirma un trabajo ajeno (ni sabe si existe).
		if code, _ := fixture.agentCall(t, fixture.north, "POST", "print-agent/jobs/"+jobID+"/result", map[string]any{
			"attempt_id": claimed.AttemptID, "outcome": "submitted", "reference": "POS_80_9GE-1"}); code != 404 {
			t.Fatalf("resultado de otra terminal: %d", code)
		}
		// Aceptado por la cola de la terminal (no es papel); repetirlo responde lo guardado; otro resultado, 409.
		result := fixture.report(t, fixture.south, claimed, map[string]any{"outcome": "submitted", "reference": "POS_80_9GE-41"}, 200)
		if result.State != "submitted" || !result.Recorded {
			t.Fatalf("resultado aceptado: %+v", result)
		}
		repeated := fixture.report(t, fixture.south, claimed, map[string]any{"outcome": "submitted", "reference": "POS_80_9GE-41"}, 200)
		if repeated.State != "submitted" || !repeated.Recorded {
			t.Fatalf("resultado repetido: %+v", repeated)
		}
		fixture.report(t, fixture.south, claimed, map[string]any{"outcome": "error", "code": "unreachable"}, 409)
		job = fixture.job(t, jobID)
		if job.State != "submitted" || !strings.Contains(job.LastError, "no confirma salida física de papel") {
			t.Fatalf("trabajo aceptado por la cola de la terminal: %+v", job)
		}
		if count := fixture.countRows(t, `SELECT count(*) FROM app.kitchen_history WHERE entity_id=$1 AND kind='print.submitted'
            AND device_id=$2`, jobID, fixture.south.deviceID); count != 1 {
			t.Fatalf("historial del resultado con la terminal: %d", count)
		}
		// El siguiente trabajo de la impresora ya se puede reclamar.
		next := fixture.claim(t, fixture.south, randomID())
		if next == nil || next.ID != second {
			t.Fatalf("siguiente trabajo: %+v", next)
		}
		fixture.report(t, fixture.south, next, map[string]any{"outcome": "submitted", "reference": "POS_80_9GE-42"}, 200)
	})

	t.Run("error confirmado con reintento e incierto sin reintento", func(t *testing.T) {
		jobID := fixture.testPrint(t, fixture.usb.ID)
		claimed := fixture.claim(t, fixture.south, randomID())
		failed := fixture.report(t, fixture.south, claimed, map[string]any{"outcome": "error", "code": "not_found",
			"detail": "lpstat: la cola no existe"}, 200)
		if failed.State != "error" {
			t.Fatalf("error confirmado: %+v", failed)
		}
		job := fixture.job(t, jobID)
		if problemOf(job) != kitchen.ProblemNotFound || job.Attempts != 1 {
			t.Fatalf("problema del error: %+v", job)
		}
		// El reintento automático espera su turno (espera creciente) y después lo reclama el mismo agente.
		if early := fixture.claim(t, fixture.south, randomID()); early != nil {
			t.Fatalf("reintento antes de su turno: %+v", early)
		}
		fixture.exec(t, "UPDATE app.kitchen_print_jobs SET next_attempt_at=now() WHERE id=$1", jobID)
		retried := fixture.claim(t, fixture.south, randomID())
		if retried == nil || retried.ID != jobID || retried.AttemptID == claimed.AttemptID {
			t.Fatalf("reintento automático: %+v", retried)
		}
		uncertain := fixture.report(t, fixture.south, retried, map[string]any{"outcome": "uncertain"}, 200)
		if uncertain.State != "uncertain" {
			t.Fatalf("resultado incierto: %+v", uncertain)
		}
		// Un incierto nunca se reenvía solo: ni el agente ni el servidor lo toman.
		if again := fixture.claim(t, fixture.south, randomID()); again != nil {
			t.Fatalf("incierto reenviado: %+v", again)
		}
		if _, err := fixture.worker.PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if job := fixture.job(t, jobID); job.State != "uncertain" || problemOf(job) != kitchen.ProblemUncertain || fixture.sent.sentJob(jobID) {
			t.Fatalf("incierto: %+v", job)
		}
		if count := fixture.countRows(t, "SELECT count(*) FROM app.kitchen_history WHERE entity_id=$1 AND kind='print.agent_detail'", jobID); count != 1 {
			t.Fatalf("detalle técnico para soporte: %d", count)
		}
	})

	t.Run("la lease vencida deja el trabajo incierto", func(t *testing.T) {
		// El agente informa tarde: el resultado no se acepta y el trabajo queda para revisión.
		late := fixture.testPrint(t, fixture.usb.ID)
		claimed := fixture.claim(t, fixture.south, randomID())
		fixture.exec(t, "UPDATE app.kitchen_print_jobs SET lease_until=now()-interval '1 second' WHERE id=$1", late)
		result := fixture.report(t, fixture.south, claimed, map[string]any{"outcome": "submitted", "reference": "POS_80_9GE-50"}, 200)
		if result.Recorded || result.Reason != "lease_expired" || result.State != "uncertain" {
			t.Fatalf("resultado tardío: %+v", result)
		}
		if job := fixture.job(t, late); job.State != "uncertain" {
			t.Fatalf("trabajo con resultado tardío: %+v", job)
		}
		if again := fixture.report(t, fixture.south, claimed, map[string]any{"outcome": "submitted", "reference": "POS_80_9GE-50"}, 200); again.Recorded {
			t.Fatalf("resultado tardío repetido: %+v", again)
		}
		if count := fixture.countRows(t, "SELECT count(*) FROM app.kitchen_history WHERE entity_id=$1 AND kind='print.agent_late_result'", late); count < 1 {
			t.Fatal("el informe tardío debe quedar en el historial")
		}
		// El agente se cierra sin informar: el trabajador del servidor recoge la lease vencida, sin enviarlo.
		lost := fixture.testPrint(t, fixture.usb.ID)
		if claimed := fixture.claim(t, fixture.south, randomID()); claimed == nil || claimed.ID != lost {
			t.Fatalf("reclamo: %+v", claimed)
		}
		fixture.exec(t, "UPDATE app.kitchen_print_jobs SET lease_until=now()-interval '1 second' WHERE id=$1", lost)
		if _, err := fixture.worker.PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		job := fixture.job(t, lost)
		if job.State != "uncertain" || fixture.sent.sentJob(lost) || !strings.Contains(job.LastError, "La terminal no informó") {
			t.Fatalf("lease vencida recogida por el servidor: %+v", job)
		}
	})

	t.Run("terminal desconectada otra vez y de vuelta", func(t *testing.T) {
		fixture.exec(t, "UPDATE app.devices SET print_agent_seen_at=now()-interval '2 minutes' WHERE id=$1", fixture.south.deviceID)
		jobID := fixture.testPrint(t, fixture.usb.ID)
		if job := fixture.job(t, jobID); problemOf(job) != kitchen.ProblemAgentOffline {
			t.Fatalf("problema con la terminal callada: %+v", job)
		}
		claimed := fixture.claim(t, fixture.south, randomID())
		if claimed == nil || claimed.ID != jobID {
			t.Fatalf("lo pendiente se imprime al volver: %+v", claimed)
		}
		fixture.report(t, fixture.south, claimed, map[string]any{"outcome": "submitted", "reference": "POS_80_9GE-60"}, 200)
	})

	t.Run("el cajón de la caja se abre por su agente", func(t *testing.T) {
		south := fixture.regions["Caja Sur 9G-E"]
		fixture.call(t, fixture.admin, "PUT", "payments/cash/drawers/"+south, map[string]any{
			"operation_id": randomID(), "revision": 0, "printer_id": fixture.usb.ID, "pulse_pin": 1, "enabled": true,
		}, 200)
		drawers := decodeResponse[struct {
			Printers []payments.DrawerPrinter `json:"printers"`
		}](t, fixture.adminCall(t, "GET", "payments/cash/drawers", nil, 200))
		compatible := false
		for _, printer := range drawers.Printers {
			if printer.ID == fixture.usb.ID {
				compatible = printer.Compatible && printer.TerminalName == southName
			}
		}
		if !compatible {
			t.Fatalf("la impresora de la terminal debe ser compatible con el cajón: %+v", drawers.Printers)
		}
		opened := decodeResponse[payments.DrawerOutcome](t, fixture.call(t, fixture.south, "POST", "payments/cash/open-drawer", map[string]any{
			"operation_id": randomID(), "reason": "Cambio para el cliente",
		}, 200))
		if opened.Status != "pending" || opened.JobID == "" {
			t.Fatalf("apertura por el agente: %+v", opened)
		}
		claimed := fixture.claim(t, fixture.south, randomID())
		if claimed == nil {
			t.Fatal("el agente no recibió el pulso del cajón")
		}
		pulse, _ := base64.StdEncoding.DecodeString(claimed.Data)
		if claimed.ID != opened.JobID || !claimed.Drawer || !bytes.Equal(pulse, kitchen.DrawerPulseBytes(1)) ||
			claimed.Title != "ADEL-cajon-"+opened.JobID {
			t.Fatalf("pulso del cajón por el agente: %+v %x", claimed, pulse)
		}
		failed := fixture.report(t, fixture.south, claimed, map[string]any{"outcome": "error", "code": "unreachable"}, 200)
		if failed.State != "error" {
			t.Fatalf("pulso con error: %+v", failed)
		}
		// Un comando del cajón con error confirmado nunca se reintenta solo (riesgo físico).
		fixture.exec(t, "UPDATE app.kitchen_print_jobs SET next_attempt_at=now() WHERE id=$1", opened.JobID)
		if again := fixture.claim(t, fixture.south, randomID()); again != nil {
			t.Fatalf("pulso reintentado solo: %+v", again)
		}
		// La segunda apertura se acepta en la cola de la terminal.
		reopened := decodeResponse[payments.DrawerOutcome](t, fixture.call(t, fixture.south, "POST", "payments/cash/open-drawer", map[string]any{
			"operation_id": randomID(), "reason": "Cambio para el cliente",
		}, 200))
		next := fixture.claim(t, fixture.south, randomID())
		if next == nil || next.ID != reopened.JobID {
			t.Fatalf("segunda apertura: %+v", next)
		}
		if accepted := fixture.report(t, fixture.south, next, map[string]any{"outcome": "submitted", "reference": "POS_80_9GE-70"}, 200); accepted.State != "submitted" {
			t.Fatalf("pulso aceptado: %+v", accepted)
		}
		if job := fixture.job(t, reopened.JobID); !strings.Contains(job.LastError, "no confirma que el cajón se abrió") {
			t.Fatalf("mensaje del pulso: %+v", job)
		}
	})

	t.Run("cada caja usa su impresora de tickets y si no la general", func(t *testing.T) {
		settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
		if len(settings.RegisterPrinters) < 2 {
			t.Fatalf("cajas en las políticas de cobro: %+v", settings.RegisterPrinters)
		}
		// Una impresora de agente sin terminal no sirve para tickets de una caja (como la general, revisión 9F B3).
		legacy := decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", map[string]any{
			"name": "Agente sin terminal 9G-E", "transport": "agent", "destination": "Sin terminal", "paper_width": 80, "copies": 1, "active": true,
		}, 200))
		body := fixture.call(t, fixture.admin, "PUT", "payments/settings", map[string]any{
			"operation_id": randomID(), "revision": settings.Revision, "shared_shifts": true, "print_receipt_default": false,
			"receipt_printer_id": fixture.network.ID, "refund_authorization_threshold": nil,
			"register_printers": []map[string]any{{"register_id": fixture.regions["Caja Sur 9G-E"], "printer_id": legacy.ID}},
		}, 422)
		if errorCode(t, body) != "RECEIPT_PRINTER_UNAVAILABLE" {
			t.Fatalf("impresora sin terminal para una caja: %s", body)
		}
		saved := decodeResponse[payments.Settings](t, fixture.call(t, fixture.admin, "PUT", "payments/settings", map[string]any{
			"operation_id": randomID(), "revision": settings.Revision, "shared_shifts": true, "print_receipt_default": false,
			"receipt_printer_id": fixture.network.ID, "refund_authorization_threshold": nil,
			"register_printers": []map[string]any{{"register_id": fixture.regions["Caja Sur 9G-E"], "printer_id": fixture.usb.ID}},
		}, 200))
		for _, item := range saved.RegisterPrinters {
			if item.RegisterID == fixture.regions["Caja Sur 9G-E"] && (item.PrinterID == nil || *item.PrinterID != fixture.usb.ID ||
				item.TerminalName == nil || *item.TerminalName != southName) {
				t.Fatalf("impresora de la Caja Sur: %+v", item)
			}
			if item.RegisterID == fixture.regions["Caja Norte 9G-E"] && item.PrinterID != nil {
				t.Fatalf("la Caja Norte usa la general: %+v", item)
			}
		}
		if count := fixture.countRows(t, `SELECT count(*) FROM app.audit_events WHERE action='payments.settings_changed'
            AND after_state::text LIKE '%register_printers%'`); count < 1 {
			t.Fatal("el cambio de impresora por caja debe auditarse")
		}
		precheckFrom := func(terminal testTerminal, name string) string {
			bill := fixture.capture(t, newAccount(t, fixture.restaurantFixture, name), []catalog.LineRequest{{ProductID: fixture.fries.ID, Quantity: 1}})
			printed := decodeResponse[printingResult](t, fixture.post(t, terminal, bill.Account.ID, "precheck/print", map[string]any{}, 200))
			var printerID string
			if err := fixture.pool.QueryRow(fixture.ctx, "SELECT printer_id::text FROM app.kitchen_print_jobs WHERE id=$1", printed.JobID).Scan(&printerID); err != nil {
				t.Fatal(err)
			}
			return printerID
		}
		if printer := precheckFrom(fixture.south, "Cuenta Sur 9G-E"); printer != fixture.usb.ID {
			t.Fatalf("la precuenta de la Caja Sur sale en su impresora USB: %s", printer)
		}
		if printer := precheckFrom(fixture.north, "Cuenta Norte 9G-E"); printer != fixture.network.ID {
			t.Fatalf("la precuenta de la Caja Norte sale en la general: %s", printer)
		}
		// El documento de la cuenta dice en qué terminal está la impresora (el aviso «desconectada» lo usa).
		fixture.exec(t, "UPDATE app.devices SET print_agent_seen_at=now()-interval '2 minutes' WHERE id=$1", fixture.south.deviceID)
		bill := fixture.capture(t, newAccount(t, fixture.restaurantFixture, "Cuenta Sur 2 9G-E"), []catalog.LineRequest{{ProductID: fixture.fries.ID, Quantity: 1}})
		fixture.post(t, fixture.south, bill.Account.ID, "precheck/print", map[string]any{}, 200)
		finance := fixture.finance(t, bill.Account.ID)
		if len(finance.Documents) != 1 || finance.Documents[0].JobProblem == nil || *finance.Documents[0].JobProblem != kitchen.ProblemAgentOffline ||
			finance.Documents[0].TerminalName == nil || *finance.Documents[0].TerminalName != southName {
			t.Fatalf("documento de una impresora desconectada: %+v", finance.Documents)
		}
	})
}
