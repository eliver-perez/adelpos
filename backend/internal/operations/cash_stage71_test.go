//go:build integration

package operations_test

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"

	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/payments"
)

// Aceptación de la fase C de la etapa 7.1: cajón por CUPS en modo raw (configuración, pulso con el
// adaptador controlado y «Probar apertura») y auditoría de los rechazos por efectivo insuficiente.
// Nada llega a CUPS ni a una impresora: la consulta de colas es simulada (controlledQueues, con la
// misma evaluación de atributos que usa el equipo) y el trabajador usa recordingTransport.

// controlledQueues sustituye las consultas de lpstat y lpoptions. Cada cola registrada responde con
// atributos simulados evaluados por kitchen.EvaluateRawQueue; una cola no registrada responde como
// no consultable, igual que una cola inexistente en el equipo.
type controlledQueues struct {
	mutex  sync.Mutex
	queues map[string]map[string]string
	calls  map[string]int
}

func newControlledQueues() *controlledQueues {
	return &controlledQueues{queues: map[string]map[string]string{}, calls: map[string]int{}}
}

func (queues *controlledQueues) set(queue string, options map[string]string) {
	queues.mutex.Lock()
	defer queues.mutex.Unlock()
	queues.queues[queue] = options
}

func (queues *controlledQueues) inspected(queue string) int {
	queues.mutex.Lock()
	defer queues.mutex.Unlock()
	return queues.calls[queue]
}

func (queues *controlledQueues) InspectQueue(_ context.Context, queue string) kitchen.QueueCapability {
	queues.mutex.Lock()
	defer queues.mutex.Unlock()
	queues.calls[queue]++
	options, found := queues.queues[queue]
	if !found {
		return kitchen.QueueCapability{Queue: queue, Reason: "No se pudo consultar la cola " + queue + " en CUPS del servidor: no existe o CUPS no responde."}
	}
	return kitchen.EvaluateRawQueue(queue, options)
}

// Atributos simulados de colas CUPS, con la forma que informa lpoptions -p.
var (
	usbQueue      = map[string]string{"device-uri": "usb://Printer/POS-80?location=2140000", "printer-make-and-model": "POS-80", "printer-type": "2150404"}
	airPrintQueue = map[string]string{
		"device-uri": "dnssd://Tickets._ipps._tcp.local./", "printer-make-and-model": "Impresora de pruebas-AirPrint", "printer-type": "69243084",
	}
)

// stage71Fixture agrega a la caja de impresión las colas CUPS simuladas del cajón.
type stage71Fixture struct {
	*printingFixture
	usb      kitchen.Printer
	airPrint kitchen.Printer
	offline  kitchen.Printer
	shiftID  string
}

func newStage71Fixture(t *testing.T, base *printingFixture) *stage71Fixture {
	t.Helper()
	fixture := &stage71Fixture{printingFixture: base}
	create := func(name, queue string) kitchen.Printer {
		return decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
			Name: name, Transport: "cups", Destination: queue, Active: true, PaperWidth: 80, Copies: 2,
		}, 200))
	}
	fixture.usb = create("Cajón USB controlado", "ADEL_TEST_DRAWER_USB")
	fixture.airPrint = create("Cola AirPrint controlada", "ADEL_TEST_DRAWER_AIRPRINT")
	// Cola sin atributos simulados: responde como una cola que CUPS no puede consultar.
	fixture.offline = create("Cola CUPS sin respuesta", "ADEL_TEST_DRAWER_OFFLINE")
	fixture.queues.set(fixture.usb.Destination, usbQueue)
	fixture.queues.set(fixture.airPrint.Destination, airPrintQueue)
	return fixture
}

func (fixture *stage71Fixture) drawerSettings(t *testing.T) (payments.DrawerSettings, map[string]payments.DrawerPrinter) {
	t.Helper()
	listed := decodeResponse[struct {
		Drawers  []payments.DrawerSettings `json:"drawers"`
		Printers []payments.DrawerPrinter  `json:"printers"`
	}](t, fixture.adminCall(t, "GET", "payments/cash/drawers", nil, 200))
	printers := map[string]payments.DrawerPrinter{}
	for _, printer := range listed.Printers {
		printers[printer.ID] = printer
	}
	for _, drawer := range listed.Drawers {
		if drawer.RegisterID == fixture.registerID {
			return drawer, printers
		}
	}
	t.Fatal("La caja de impresión no aparece en la lista de cajones")
	return payments.DrawerSettings{}, printers
}

func (fixture *stage71Fixture) saveDrawer(t *testing.T, printerID string, enabled bool, status int) []byte {
	t.Helper()
	drawer, _ := fixture.drawerSettings(t)
	return fixture.adminCall(t, "PUT", "payments/cash/drawers/"+fixture.registerID, map[string]any{
		"operation_id": randomID(), "revision": drawer.Revision, "printer_id": printerID, "pulse_pin": 1, "enabled": enabled,
	}, status)
}

// testCashStage71 se encadena al final de la aceptación de impresión de caja (testCashPrinting).
func testCashStage71(t *testing.T, base *printingFixture) {
	fixture := newStage71Fixture(t, base)
	testCUPSDrawerSettings(t, fixture)
	testCUPSDrawerPulses(t, fixture)
	testDrawerTest(t, fixture)
	testRejectedOutflows(t, fixture)
	fixture.closeShift(t, fixture.cashier, fixture.shiftID, map[string]any{}, 200)
	fixture.saveDrawer(t, fixture.drawerPrinter.ID, false, 200)
}

func testCUPSDrawerSettings(t *testing.T, fixture *stage71Fixture) {
	t.Run("C48_cajon_con_cola_cups_compatible_e_incompatible", func(t *testing.T) {
		_, printers := fixture.drawerSettings(t)
		usb, airPrint, offline := printers[fixture.usb.ID], printers[fixture.airPrint.ID], printers[fixture.offline.ID]
		if !usb.Compatible || usb.Connection != "usb" || usb.Reason != "" {
			t.Fatalf("La cola USB con datos raw no se informó compatible: %+v", usb)
		}
		if airPrint.Compatible || airPrint.Connection != "dnssd" || !strings.Contains(airPrint.Reason, "AirPrint") {
			t.Fatalf("La cola AirPrint no explicó por qué no sirve: %+v", airPrint)
		}
		if offline.Compatible || !strings.Contains(offline.Reason, "No se pudo consultar") {
			t.Fatalf("La cola sin respuesta no se explicó: %+v", offline)
		}
		before, _ := fixture.drawerSettings(t)
		audits := fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.drawer_settings_changed'")
		problem := fixture.saveDrawer(t, fixture.airPrint.ID, true, 400)
		if errorCode(t, problem) != "DRAWER_QUEUE_NOT_RAW" || !strings.Contains(string(problem), "AirPrint") ||
			!strings.Contains(string(problem), "ESC/POS por red") {
			t.Fatalf("La cola sin datos raw no se rechazó con su motivo: %s", problem)
		}
		problem = fixture.saveDrawer(t, fixture.offline.ID, true, 409)
		if errorCode(t, problem) != "DRAWER_QUEUE_UNAVAILABLE" || !strings.Contains(string(problem), "no existe o CUPS no responde") {
			t.Fatalf("La cola sin respuesta no se rechazó con su motivo: %s", problem)
		}
		after, _ := fixture.drawerSettings(t)
		if after.Revision != before.Revision || text(after.PrinterID) != text(before.PrinterID) ||
			fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.drawer_settings_changed'") != audits {
			t.Fatal("Una configuración rechazada cambió el cajón o quedó auditada")
		}
		// Desactivar el cajón nunca depende de que CUPS responda: la cola no se consulta.
		calls := fixture.queues.inspected(fixture.offline.Destination)
		fixture.saveDrawer(t, fixture.offline.ID, false, 200)
		if fixture.queues.inspected(fixture.offline.Destination) != calls+1 {
			// La lista de cajones que lee saveDrawer consulta una vez; el guardado no vuelve a hacerlo.
			t.Fatalf("Se consultó la cola al desactivar el cajón: %d consultas", fixture.queues.inspected(fixture.offline.Destination)-calls)
		}
		saved := decodeResponse[map[string]any](t, fixture.saveDrawer(t, fixture.usb.ID, true, 200))
		if saved["enabled"] != true || saved["printer_name"] != "Cajón USB controlado" || saved["pulse_pin"].(float64) != 1 {
			t.Fatalf("El cajón no se configuró con la cola USB: %+v", saved)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.drawer_settings_changed'") != audits+2 {
			t.Fatal("Las configuraciones aceptadas no quedaron auditadas")
		}
	})
}

func testCUPSDrawerPulses(t *testing.T, fixture *stage71Fixture) {
	cash := fixture.methods["cash"].ID
	debit := fixture.methods["debit"].ID
	fixture.shiftID = fixture.openFloat(t, "1000")

	t.Run("C49_pulso_por_cups_con_efectivo_y_nunca_con_tarjeta", func(t *testing.T) {
		before := fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events")
		paid := fixture.account(t, "Cobro con cajón USB", line(fixture.soda, 1))
		result := decodeResponse[struct {
			Drawer payments.DrawerOutcome `json:"drawer"`
		}](t, fixture.pay(t, fixture.cashier, paid.Account.ID, map[string]any{"method_id": cash, "mode": "total", "shift_id": fixture.shiftID}, 200))
		if !result.Drawer.Requested || result.Drawer.Status != "pending" || result.Drawer.JobID == "" {
			t.Fatalf("El cobro en efectivo no envió el comando por CUPS: %+v", result.Drawer)
		}
		var reason, printerID string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT e.reason,j.printer_id FROM app.cash_drawer_events e
            JOIN app.kitchen_print_jobs j ON j.id=e.job_id WHERE e.job_id=$1`, result.Drawer.JobID).Scan(&reason, &printerID); err != nil {
			t.Fatal(err)
		}
		if reason != "payment" || printerID != fixture.usb.ID {
			t.Fatalf("El comando no quedó en la cola CUPS del cajón: %s %s", reason, printerID)
		}
		card := fixture.account(t, "Cobro con tarjeta y cajón USB", line(fixture.soda, 1))
		electronic := decodeResponse[struct {
			Drawer payments.DrawerOutcome `json:"drawer"`
		}](t, fixture.pay(t, fixture.cashier, card.Account.ID, map[string]any{"method_id": debit, "mode": "total", "reference": "4321"}, 200))
		if electronic.Drawer.Requested || fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events") != before+1 {
			t.Fatal("Un pago con tarjeta abrió el cajón por CUPS")
		}
		// El adaptador controlado recibe el pulso del conector 5 con el destino CUPS y una copia.
		jobs := fixture.processOnly(t, result.Drawer.JobID, "submitted")
		if len(jobs) != 1 || jobs[0].DrawerPulse == nil || jobs[0].DrawerPulse.Pin != 1 || jobs[0].PrinterSnapshot.Transport != "cups" ||
			jobs[0].PrinterSnapshot.Destination != fixture.usb.Destination || jobs[0].PrinterSnapshot.Copies != 1 {
			t.Fatalf("El adaptador no recibió el pulso por CUPS: %+v", jobs)
		}
		if state, _, _ := fixture.jobState(t, result.Drawer.JobID); state != "submitted" {
			t.Fatalf("El comando aceptado quedó en %s", state)
		}
	})

	t.Run("C50_apertura_manual_por_cups_con_permiso", func(t *testing.T) {
		fixture.call(t, fixture.observer, "POST", "payments/cash/open-drawer", map[string]any{"operation_id": randomID(), "reason": "Sin permiso"}, 403)
		opened := decodeResponse[payments.DrawerOutcome](t, fixture.call(t, fixture.cashier, "POST", "payments/cash/open-drawer", map[string]any{
			"operation_id": randomID(), "reason": "Cambio para el fondo por USB",
		}, 200))
		var reason, printerID, notes string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT e.reason,j.printer_id,e.notes FROM app.cash_drawer_events e
            JOIN app.kitchen_print_jobs j ON j.id=e.job_id WHERE e.job_id=$1`, opened.JobID).Scan(&reason, &printerID, &notes); err != nil {
			t.Fatal(err)
		}
		if !opened.Requested || reason != "manual" || printerID != fixture.usb.ID || notes != "Cambio para el fondo por USB" {
			t.Fatalf("La apertura manual por CUPS quedó mal registrada: %+v %s %s", opened, reason, printerID)
		}
	})
}

func testDrawerTest(t *testing.T, fixture *stage71Fixture) {
	route := "payments/cash/drawers/" + fixture.registerID + "/test"

	t.Run("C51_probar_apertura_con_y_sin_permiso", func(t *testing.T) {
		events := fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events")
		denied := fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='access.denied'")
		// El cajero abre el cajón manualmente, pero probarlo es parte de la configuración.
		fixture.call(t, fixture.cashier, "POST", route, map[string]any{"operation_id": randomID()}, 403)
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events") != events ||
			fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='access.denied'") != denied+1 {
			t.Fatal("La prueba sin permiso emitió un comando o no quedó auditada como denegación")
		}
		queued := fixture.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='cash.drawer.command_queued' AND aggregate_id=$1", fixture.shiftID)
		request := map[string]any{"operation_id": randomID()}
		tested := decodeResponse[payments.DrawerOutcome](t, fixture.adminCall(t, "POST", route, request, 200))
		again := decodeResponse[payments.DrawerOutcome](t, fixture.adminCall(t, "POST", route, request, 200))
		if !tested.Requested || tested.Status != "pending" || tested.JobID == "" || again.JobID != tested.JobID {
			t.Fatalf("La prueba de apertura no es idempotente: %+v / %+v", tested, again)
		}
		var reason, notes, shiftID, printerID string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT e.reason,e.notes,e.shift_id,j.printer_id FROM app.cash_drawer_events e
            JOIN app.kitchen_print_jobs j ON j.id=e.job_id WHERE e.job_id=$1`, tested.JobID).Scan(&reason, &notes, &shiftID, &printerID); err != nil {
			t.Fatal(err)
		}
		if reason != "test" || !strings.Contains(notes, "Prueba de apertura") || shiftID != fixture.shiftID || printerID != fixture.usb.ID {
			t.Fatalf("La prueba de apertura quedó mal registrada: %s %s %s %s", reason, notes, shiftID, printerID)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events") != events+1 ||
			fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.drawer_tested' AND entity_id=$1", fixture.registerID) != 1 ||
			fixture.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='cash.drawer.command_queued' AND aggregate_id=$1", fixture.shiftID) != queued+1 {
			t.Fatal("La prueba de apertura no quedó registrada una vez, auditada y publicada")
		}
		// Una caja sin impresora del cajón no se prueba; una cola que dejó de admitir datos raw
		// tampoco, sin registrar ningún comando.
		empty := fixture.createRegister(t, "Caja sin cajón")
		if errorCode(t, fixture.adminCall(t, "POST", "payments/cash/drawers/"+empty+"/test", map[string]any{"operation_id": randomID()}, 409)) != "DRAWER_NOT_CONFIGURED" {
			t.Fatal("Se probó el cajón de una caja sin impresora")
		}
		fixture.queues.set(fixture.usb.Destination, airPrintQueue)
		problem := fixture.adminCall(t, "POST", route, map[string]any{"operation_id": randomID()}, 400)
		fixture.queues.set(fixture.usb.Destination, usbQueue)
		if errorCode(t, problem) != "DRAWER_QUEUE_NOT_RAW" || fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events") != events+1 {
			t.Fatalf("Se probó el cajón en una cola sin datos raw: %s", problem)
		}
		fixture.registers["stage71_test_job"] = tested.JobID
	})

	t.Run("C52_resultado_incierto_por_cups_sin_reintento_automatico", func(t *testing.T) {
		jobID := fixture.registers["stage71_test_job"]
		fixture.processOnly(t, jobID, "uncertain")
		state, attempts, revision := fixture.jobState(t, jobID)
		if state != "uncertain" || attempts != 1 {
			t.Fatalf("El comando incierto quedó en %s con %d intentos", state, attempts)
		}
		adapter := &recordingTransport{state: "submitted"}
		if _, err := (&kitchen.Worker{Pool: fixture.pool, Transport: adapter}).PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if len(adapter.jobs) != 0 {
			t.Fatal("El trabajador reenvió un comando incierto del cajón por CUPS")
		}
		fixture.mutate(t, "POST", "kitchen/jobs/"+jobID+"/resolve", map[string]any{
			"revision": revision, "decision": "send", "reason": "Reenviar la prueba incierta",
		}, 400)
		if state, attempts, _ := fixture.jobState(t, jobID); state != "uncertain" || attempts != 1 {
			t.Fatalf("El comando incierto cambió sin decisión humana: %s, %d intentos", state, attempts)
		}
	})
}

func testRejectedOutflows(t *testing.T, fixture *stage71Fixture) {
	rejectedRoute := "payments/shifts/" + fixture.shiftID + "/rejected-outflows"
	var excess, expectedMXN string

	t.Run("C53_rechazo_por_efectivo_insuficiente_auditado_sin_el_esperado", func(t *testing.T) {
		expectedMXN = fixture.expected(t, fixture.shiftID).MXN
		excess = amountOf(t, expectedMXN).Add(amountOf(t, "1234.50")).StringFixed(2)
		operations := fixture.countRows(t, "SELECT count(*) FROM app.cash_operations WHERE shift_id=$1", fixture.shiftID)
		drawers := fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events")
		recorded := fixture.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='cash.operation.recorded'")
		operationID := randomID()
		problem := fixture.shiftCall(t, fixture.cashier, "POST", fixture.shiftID, "withdrawals", map[string]any{
			"operation_id": operationID, "type": "safekeeping", "amount": excess,
		}, 409)
		message := decodeResponse[struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}](t, problem).Error
		// Hacia el cajero nada cambia: el rechazo sigue sin cifras.
		if message.Code != "CASH_INSUFFICIENT" || regexp.MustCompile(`[0-9]`).MatchString(message.Message) || strings.Contains(string(problem), expectedMXN) {
			t.Fatalf("El rechazo reveló cifras: %s", problem)
		}
		// La operación se revirtió completa y solo sobrevivió su auditoría.
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_operations WHERE shift_id=$1", fixture.shiftID) != operations ||
			fixture.countRows(t, "SELECT count(*) FROM app.restaurant_operations WHERE id=$1", operationID) != 0 ||
			fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events") != drawers ||
			fixture.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='cash.operation.recorded'") != recorded {
			t.Fatal("El retiro rechazado dejó registros")
		}
		var actorID, deviceID, entityID, details string
		var success bool
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT actor_id,device_id,entity_id,success,after_state::text FROM app.audit_events
            WHERE action='cash.outflow_rejected' AND after_state->>'operation_id'=$1`, operationID).Scan(
			&actorID, &deviceID, &entityID, &success, &details); err != nil {
			t.Fatal(err)
		}
		if success || actorID != fixture.userID(t, "cajero_impresion") || deviceID != fixture.cashier.deviceID || entityID != fixture.shiftID {
			t.Fatalf("La auditoría del rechazo no identifica usuario, terminal y turno: %s %s %s %v", actorID, deviceID, entityID, success)
		}
		for _, fragment := range []string{`"kind": "withdrawal"`, `"currency": "MXN"`, `"attempted_amount": "` + excess + `"`,
			`"register_id": "` + fixture.registerID + `"`, `"shift_id": "` + fixture.shiftID + `"`, `"authorization_mode": "self"`} {
			if !strings.Contains(details, fragment) {
				t.Fatalf("La auditoría del rechazo no contiene %s: %s", fragment, details)
			}
		}
		if strings.Contains(details, "expected") || strings.Contains(details, `"`+expectedMXN+`"`) {
			t.Fatalf("La auditoría del rechazo contiene el esperado: %s", details)
		}
		// Un gasto autorizado por otra persona también queda auditado con su autorizador.
		fixture.shiftCall(t, fixture.writer, "POST", fixture.shiftID, "expenses", map[string]any{
			"category_id": fixture.categories["Otros"], "amount": excess, "authorization": authorizationOf("encargado_caja"),
		}, 409)
		if fixture.countRows(t, `SELECT count(*) FROM app.audit_events WHERE action='cash.outflow_rejected' AND NOT success
            AND entity_id=$1 AND after_state->>'kind'='expense' AND after_state->>'authorization_mode'='supervisor'
            AND after_state->>'authorized_by'=$2`, fixture.shiftID, fixture.userID(t, "encargado_caja")) != 1 {
			t.Fatal("El gasto rechazado no quedó auditado con su autorizador")
		}
	})

	t.Run("C54_consulta_de_intentos_rechazados_solo_con_permiso_de_esperado", func(t *testing.T) {
		fixture.call(t, fixture.cashier, "GET", rejectedRoute, nil, 403)
		fixture.call(t, fixture.observer, "GET", rejectedRoute, nil, 403)
		body := fixture.call(t, fixture.supervisor, "GET", rejectedRoute, nil, 200)
		listed := decodeResponse[struct {
			Visible    bool                       `json:"visible"`
			Rejections []payments.RejectedOutflow `json:"rejections"`
		}](t, body)
		if !listed.Visible || len(listed.Rejections) != 2 {
			t.Fatalf("La consulta no devolvió los dos intentos rechazados: %s", body)
		}
		withdrawal, expense := listed.Rejections[0], listed.Rejections[1]
		if withdrawal.Kind != "withdrawal" || withdrawal.AttemptedAmount != excess || withdrawal.Currency != "MXN" ||
			withdrawal.ActorName != "Prueba cajero_impresion" || withdrawal.DeviceName != "Terminal cajero_impresion" ||
			withdrawal.RegisterName != "Caja de impresión" || text(withdrawal.AuthorizationMode) != "self" {
			t.Fatalf("El retiro rechazado se informó mal: %+v", withdrawal)
		}
		if expense.Kind != "expense" || expense.ActorName != "Prueba cajero_sin_impresion" || text(expense.AuthorizedByName) != "Prueba encargado_caja" {
			t.Fatalf("El gasto rechazado se informó mal: %+v", expense)
		}
		if strings.Contains(string(body), "expected") || strings.Contains(string(body), `"`+expectedMXN+`"`) {
			t.Fatalf("La consulta de rechazos reveló el esperado: %s", body)
		}
		// Durante el arqueo de cierre la consulta es ciega, como el esperado.
		fixture.shiftCall(t, fixture.cashier, "POST", fixture.shiftID, "closing", map[string]any{}, 200)
		blindBody := fixture.call(t, fixture.supervisor, "GET", rejectedRoute, nil, 200)
		hidden := decodeResponse[struct {
			Visible    bool                       `json:"visible"`
			Rejections []payments.RejectedOutflow `json:"rejections"`
		}](t, blindBody)
		if hidden.Visible || len(hidden.Rejections) != 0 || strings.Contains(string(blindBody), excess) {
			t.Fatalf("La consulta de rechazos no fue ciega durante el arqueo: %s", blindBody)
		}
		fixture.shiftCall(t, fixture.cashier, "POST", fixture.shiftID, "closing/cancel", map[string]any{}, 200)
	})
}
