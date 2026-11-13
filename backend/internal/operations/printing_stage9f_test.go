//go:build integration

package operations_test

import (
	"encoding/json"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa 9F, fase B (§4.1, §4.2, §4.4–§4.6) contra PostgreSQL: impresora válida para precuentas, tickets y
// documentos de caja (decisión D4) sin encolar trabajos fallidos, reintento sin motivo con el permiso de
// imprimir (decisión D3), motivo obligatorio al reimprimir, corte de papel guardado y validado, nombre
// visible de la precuenta y «Mesa» sin repetir. Ningún trabajo llega a una impresora: las colas del
// equipo no existen y el trabajador usa el adaptador controlado.

type printerError struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Details map[string]string `json:"details"`
	} `json:"error"`
}

type receiptPrinting struct {
	Payment  payments.PaymentRecord `json:"payment"`
	Printing struct {
		Requested   bool   `json:"requested"`
		Status      string `json:"status"`
		Problem     string `json:"problem"`
		PrinterName string `json:"printer_name"`
		Message     string `json:"message"`
		JobID       string `json:"job_id"`
		TicketID    string `json:"ticket_id"`
	} `json:"printing"`
}

func (fixture *paymentsFixture) createPrinter(t *testing.T, printer kitchen.Printer) kitchen.Printer {
	t.Helper()
	return decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", printer, 200))
}

// setPrecheckName guarda el nombre visible de la precuenta en Administración › Documentos (revisión 9F, M3):
// PUT documents/identity con la identidad vigente y el nombre.
func (fixture *paymentsFixture) setPrecheckName(t *testing.T, name string, status int) {
	t.Helper()
	identity := decodeResponse[map[string]any](t, fixture.adminCall(t, "GET", "documents/identity", nil, 200))
	fixture.adminCall(t, "PUT", "documents/identity", map[string]any{
		"operation_id": randomID(), "revision": identity["revision"], "additional_info": identity["additional_info"],
		"header_text": identity["header_text"], "footer_text": identity["footer_text"], "presentation": identity["presentation"],
		"precheck_name": name,
	}, status)
}

// forceReceiptPrinter deja como impresora de tickets una que dejó de servir después de elegirla (Go ya no
// permite elegir una desactivada o de otra terminal, revisión 9F B3).
func (fixture *paymentsFixture) forceReceiptPrinter(t *testing.T, printerID any) {
	t.Helper()
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.payment_settings SET receipt_printer_id=$1", printerID); err != nil {
		t.Fatal(err)
	}
}

// runWorkerWith procesa solo el trabajo indicado con el adaptador controlado: los demás pendientes de
// otras pruebas se cancelan antes, como en la prueba 54.
func (fixture *paymentsFixture) runWorkerWith(t *testing.T, jobID, state string) {
	t.Helper()
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='cancelled' WHERE state IN ('pending','error') AND id<>$1", jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&kitchen.Worker{Pool: fixture.pool, Transport: &recordingTransport{state: state}}).PrintNext(fixture.ctx); err != nil {
		t.Fatal(err)
	}
}

func (fixture *paymentsFixture) jobState(t *testing.T, jobID string) (string, int) {
	t.Helper()
	var state string
	var attempts int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT state,attempts FROM app.kitchen_print_jobs WHERE id=$1", jobID).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	return state, attempts
}

func testPrintingStage9F(t *testing.T, fixture *paymentsFixture) {
	cash := fixture.methods["cash"].ID
	registerID := ""
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT register_id FROM app.devices WHERE id=$1", fixture.admin.deviceID).Scan(&registerID); err != nil {
		t.Fatal(err)
	}
	// Cajero que imprime y reimprime tickets y precuentas, sin acceso a la cola de cocina.
	printerClerk := newCashier(t, fixture, "cajero_impresion_9f", []string{
		"accounts.read", "payments.read", "payments.collect", "payments.tickets.print", "payments.tickets.reprint",
	}, registerID)
	valid := fixture.createPrinter(t, kitchen.Printer{
		Name: "Tickets 9F", Transport: "cups", Destination: "ADEL_TEST_9F_TICKETS", Active: true, PaperWidth: 80, Copies: 1,
	})
	agent := fixture.createPrinter(t, kitchen.Printer{
		Name: "Otra terminal 9F", Transport: "agent", Destination: "Caja 2", Active: true, PaperWidth: 80, Copies: 1,
	})
	inactive := fixture.createPrinter(t, kitchen.Printer{
		Name: "Tickets apagada 9F", Transport: "cups", Destination: "ADEL_TEST_9F_OFF", Active: false, PaperWidth: 80, Copies: 1,
	})
	jobs := func() int {
		return fixture.countRows(t, "SELECT count(*) FROM app.kitchen_print_jobs")
	}

	t.Run("9F_D4_sin_impresora_valida_avisa_sin_encolar", func(t *testing.T) {
		bill := fixture.account(t, "Precuenta 9F", line(fixture.soda, 1))
		cases := []struct {
			printer any
			code    string
			problem string
			message string
		}{
			{nil, "PRINTER_NOT_CONFIGURED", "missing", "No hay impresora configurada para precuentas."},
			{agent.ID, "PRINTER_UNAVAILABLE", "unsupported", "Esta impresora todavía no puede usarse para imprimir."},
			{inactive.ID, "PRINTER_UNAVAILABLE", "inactive", "La impresora Tickets apagada 9F está desactivada."},
		}
		for _, testCase := range cases {
			fixture.receiptSettings(t, nil, true)
			fixture.forceReceiptPrinter(t, testCase.printer)
			before, documents := jobs(), fixture.countRows(t, "SELECT count(*) FROM app.payment_documents")
			rejected := decodeResponse[printerError](t, fixture.post(t, printerClerk, bill.Account.ID, "precheck/print", map[string]any{}, 409))
			if rejected.Error.Code != testCase.code || rejected.Error.Message != testCase.message ||
				rejected.Error.Details["document"] != "precheck" || rejected.Error.Details["problem"] != testCase.problem {
				t.Fatalf("Rechazo de la precuenta sin impresora válida incorrecto: %+v", rejected.Error)
			}
			if jobs() != before || fixture.countRows(t, "SELECT count(*) FROM app.payment_documents") != documents {
				t.Fatal("Se encoló un trabajo o se registró una precuenta sin impresora válida")
			}
		}
		// El ticket de pago nunca hace fallar el pago: not_configured con el motivo, sin trabajo.
		before := jobs()
		paid := decodeResponse[receiptPrinting](t, fixture.pay(t, printerClerk, bill.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "received": "50", "print": true,
		}, 200))
		if paid.Printing.Status != "not_configured" || paid.Printing.Problem != "inactive" || paid.Printing.PrinterName != "Tickets apagada 9F" ||
			!strings.Contains(paid.Printing.Message, "el pago quedó registrado") || jobs() != before {
			t.Fatalf("El ticket sin impresora válida no se informó sin encolar: %+v", paid.Printing)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.payments WHERE id=$1", paid.Payment.ID) != 1 {
			t.Fatal("La falta de impresora revirtió el pago")
		}
		// «Reintentar» del aviso: una vez configurada, el ticket de ese pago se imprime; dos veces, no.
		fixture.post(t, printerClerk, bill.Account.ID, "documents/receipt", map[string]any{"payment_id": paid.Payment.ID}, 409)
		fixture.receiptSettings(t, valid.ID, true)
		printed := decodeResponse[struct {
			Status   string `json:"status"`
			JobID    string `json:"job_id"`
			TicketID string `json:"ticket_id"`
		}](t, fixture.post(t, printerClerk, bill.Account.ID, "documents/receipt", map[string]any{"payment_id": paid.Payment.ID}, 200))
		if printed.Status != "pending" || printed.JobID == "" {
			t.Fatalf("El ticket del pago no se imprimió al reintentar: %+v", printed)
		}
		again := fixture.post(t, printerClerk, bill.Account.ID, "documents/receipt", map[string]any{"payment_id": paid.Payment.ID}, 409)
		if errorCode(t, again) != "DOCUMENT_EXISTS" {
			t.Fatal("El ticket de un pago se imprimió dos veces sin pedir una reimpresión")
		}
		fixture.post(t, fixture.observer, bill.Account.ID, "documents/receipt", map[string]any{"payment_id": paid.Payment.ID}, 403)
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='payments.receipt_printed'") != 1 {
			t.Fatal("La impresión del ticket después del pago no se auditó")
		}
	})

	t.Run("9F_B1_ticket_solo_del_pago_que_quedo_sin_impresora", func(t *testing.T) {
		fixture.receiptSettings(t, valid.ID, false)
		receipts := func() int {
			return fixture.countRows(t, "SELECT count(*) FROM app.payment_documents WHERE kind='receipt'")
		}
		// Un pago registrado sin pedir ticket no recibe uno nuevo, sin motivo, por esta ruta.
		plain := fixture.account(t, "Sin ticket 9F", line(fixture.soda, 1))
		paidPlain := decodeResponse[receiptPrinting](t, fixture.pay(t, printerClerk, plain.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "received": "50", "print": false,
		}, 200))
		before := receipts()
		if code := errorCode(t, fixture.post(t, printerClerk, plain.Account.ID, "documents/receipt",
			map[string]any{"payment_id": paidPlain.Payment.ID}, 409)); code != "RECEIPT_NOT_PENDING" {
			t.Fatalf("Se aceptó el ticket de un pago que no lo tenía pendiente: %s", code)
		}
		// Un pago cuyo ticket no salió por falta de impresora, pero ya devuelto, tampoco.
		fixture.receiptSettings(t, nil, false)
		refunded := fixture.account(t, "Devuelto 9F", line(fixture.soda, 1))
		paidRefunded := decodeResponse[receiptPrinting](t, fixture.pay(t, printerClerk, refunded.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "received": "50", "print": true,
		}, 200))
		if paidRefunded.Printing.Status != "not_configured" {
			t.Fatalf("El ticket sin impresora no quedó pendiente: %+v", paidRefunded.Printing)
		}
		fixture.receiptSettings(t, valid.ID, false)
		fixture.post(t, fixture.admin, refunded.Account.ID, "refunds", map[string]any{
			"mode": "total", "method_id": cash, "reason": "Pedido devuelto por el cliente 9F",
		}, 200)
		if code := errorCode(t, fixture.post(t, printerClerk, refunded.Account.ID, "documents/receipt",
			map[string]any{"payment_id": paidRefunded.Payment.ID}, 409)); code != "PAYMENT_REFUNDED" {
			t.Fatalf("Se aceptó el ticket de un pago devuelto: %s", code)
		}
		if receipts() != before {
			t.Fatal("Se registró un ticket que no debía imprimirse")
		}
	})

	t.Run("9F_D3_reintento_sin_motivo_y_reimpresion_con_motivo", func(t *testing.T) {
		fixture.receiptSettings(t, valid.ID, false)
		bill := fixture.account(t, "Reintento 9F", line(fixture.soda, 1))
		precheck := decodeResponse[printingResult](t, fixture.post(t, printerClerk, bill.Account.ID, "precheck/print", map[string]any{}, 200))
		fixture.runWorkerWith(t, precheck.JobID, "error")
		finance := fixture.finance(t, bill.Account.ID)
		if len(finance.Documents) != 1 || *finance.Documents[0].JobState != "error" || finance.Documents[0].JobProblem == nil ||
			*finance.Documents[0].JobProblem != kitchen.ProblemUnreachable {
			t.Fatalf("La precuenta fallida no explica el problema: %+v", finance.Documents)
		}
		// Sin el permiso de la cola de cocina y sin motivo: un toque desde el aviso del documento.
		retried := decodeResponse[printingResult](t, fixture.post(t, printerClerk, bill.Account.ID, "documents/retry",
			map[string]any{"ticket_id": precheck.TicketID, "job_id": precheck.JobID}, 200))
		if state, attempts := fixture.jobState(t, precheck.JobID); retried.Status != "pending" || state != "pending" || attempts != 1 {
			t.Fatalf("El reintento no devolvió el trabajo a la cola: %+v %s %d", retried, state, attempts)
		}
		notFailed := fixture.post(t, printerClerk, bill.Account.ID, "documents/retry", map[string]any{"ticket_id": precheck.TicketID}, 409)
		if errorCode(t, notFailed) != "PRINT_NOT_FAILED" {
			t.Fatal("Se reintentó un trabajo que no falló")
		}
		// Un incierto (pudo haber salido) nunca se reintenta desde el aviso.
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='uncertain' WHERE id=$1", precheck.JobID); err != nil {
			t.Fatal(err)
		}
		uncertain := fixture.post(t, printerClerk, bill.Account.ID, "documents/retry", map[string]any{"ticket_id": precheck.TicketID}, 409)
		if errorCode(t, uncertain) != "PRINT_UNCERTAIN" {
			t.Fatal("Un resultado incierto se reintentó sin decisión")
		}
		// Con la impresora desactivada, el reintento avisa con el motivo en lugar de encolar.
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='error' WHERE id=$1", precheck.JobID); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_printers SET active=false WHERE id=$1", valid.ID); err != nil {
			t.Fatal(err)
		}
		unavailable := decodeResponse[printerError](t, fixture.post(t, printerClerk, bill.Account.ID, "documents/retry", map[string]any{"ticket_id": precheck.TicketID}, 409))
		if unavailable.Error.Code != "PRINTER_UNAVAILABLE" || unavailable.Error.Details["problem"] != kitchen.ProblemInactive {
			t.Fatalf("El reintento con la impresora desactivada no lo explicó: %+v", unavailable.Error)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_printers SET active=true WHERE id=$1", valid.ID); err != nil {
			t.Fatal(err)
		}
		// Otro documento u otra cuenta: no se reintenta por esta ruta.
		other := fixture.account(t, "Otra cuenta 9F", line(fixture.soda, 1))
		fixture.post(t, printerClerk, other.Account.ID, "documents/retry", map[string]any{"ticket_id": precheck.TicketID}, 404)
		fixture.post(t, fixture.observer, bill.Account.ID, "documents/retry", map[string]any{"ticket_id": precheck.TicketID}, 403)
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='payments.document_retried'") != 1 {
			t.Fatal("El reintento del documento no se auditó")
		}
		// La cola de cocina también reintenta sin motivo; resolver un incierto sigue exigiéndolo.
		detail := decodeResponse[struct {
			Job kitchen.PrintJob `json:"job"`
		}](t, fixture.adminCall(t, "GET", "kitchen/jobs/"+precheck.JobID, nil, 200))
		fixture.mutate(t, "POST", "kitchen/jobs/"+precheck.JobID+"/retry", map[string]any{"revision": detail.Job.Revision}, 200)
		if state, _ := fixture.jobState(t, precheck.JobID); state != "pending" {
			t.Fatal("El reintento sin motivo de la cola no devolvió el trabajo a la cola")
		}
		fixture.mutate(t, "POST", "kitchen/jobs/"+precheck.JobID+"/retry", map[string]any{"revision": detail.Job.Revision + 1, "reason": "mal"}, 400)
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='uncertain',revision=revision+1 WHERE id=$1", precheck.JobID); err != nil {
			t.Fatal(err)
		}
		detail = decodeResponse[struct {
			Job kitchen.PrintJob `json:"job"`
		}](t, fixture.adminCall(t, "GET", "kitchen/jobs/"+precheck.JobID, nil, 200))
		fixture.mutate(t, "POST", "kitchen/jobs/"+precheck.JobID+"/resolve", map[string]any{"revision": detail.Job.Revision, "decision": "accepted"}, 400)
		fixture.mutate(t, "POST", "kitchen/jobs/"+precheck.JobID+"/resolve", map[string]any{
			"revision": detail.Job.Revision, "decision": "accepted", "reason": "La precuenta salió en papel",
		}, 200)
		// Reimprimir lo que ya salió pide motivo, que queda en la auditoría y en el historial del trabajo.
		fixture.post(t, printerClerk, bill.Account.ID, "documents/reprint", map[string]any{"ticket_id": precheck.TicketID}, 400)
		fixture.post(t, printerClerk, bill.Account.ID, "documents/reprint", map[string]any{"ticket_id": precheck.TicketID, "reason": "otra"}, 400)
		copied := decodeResponse[printingResult](t, fixture.post(t, printerClerk, bill.Account.ID, "documents/reprint",
			map[string]any{"ticket_id": precheck.TicketID, "reason": "El cliente pidió otra copia"}, 200))
		var auditReason, historyReason string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT after_state->>'reason' FROM app.audit_events
            WHERE action='payments.document_reprinted' ORDER BY occurred_at DESC LIMIT 1`).Scan(&auditReason); err != nil {
			t.Fatal(err)
		}
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT after_value->>'reason' FROM app.kitchen_history
            WHERE kind='print.reprinted' AND entity_id=$1`, copied.JobID).Scan(&historyReason); err != nil {
			t.Fatal(err)
		}
		if auditReason != "El cliente pidió otra copia" || historyReason != auditReason {
			t.Fatalf("El motivo de la reimpresión no quedó registrado: %q %q", auditReason, historyReason)
		}
	})

	t.Run("9F_M2_reintento_y_reimpresion_con_la_impresora_vigente", func(t *testing.T) {
		fixed := fixture.createPrinter(t, kitchen.Printer{
			Name: "Revision 9F", Transport: "cups", Destination: "ADEL_REV_COLA_VIEJA", Active: true, PaperWidth: 80, Copies: 1,
		})
		fixture.receiptSettings(t, fixed.ID, false)
		bill := fixture.account(t, "Revision reintento 9F", line(fixture.soda, 1))
		precheck := decodeResponse[printingResult](t, fixture.post(t, printerClerk, bill.Account.ID, "precheck/print", map[string]any{}, 200))
		fixture.runWorkerWith(t, precheck.JobID, "error")
		snapshot := func(jobID string) (kitchen.Printer, string, string) {
			t.Helper()
			var raw []byte
			var content, printerID string
			if err := fixture.pool.QueryRow(fixture.ctx, "SELECT printer_snapshot,content,printer_id FROM app.kitchen_print_jobs WHERE id=$1",
				jobID).Scan(&raw, &content, &printerID); err != nil {
				t.Fatal(err)
			}
			var printer kitchen.Printer
			if err := json.Unmarshal(raw, &printer); err != nil {
				t.Fatal(err)
			}
			return printer, content, printerID
		}
		setJobState := func(state string) {
			t.Helper()
			if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state=$1 WHERE id=$2", state, precheck.JobID); err != nil {
				t.Fatal(err)
			}
		}
		updatePrinter := func(assignments string) {
			t.Helper()
			if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_printers SET "+assignments+",revision=revision+1 WHERE id=$1", fixed.ID); err != nil {
				t.Fatal(err)
			}
		}
		_, original, _ := snapshot(precheck.JobID)
		// 1. «Configurar impresora»: se corrige la cola de la misma impresora y se toca «Reintentar» en el aviso.
		updatePrinter("destination='ADEL_REV_COLA_NUEVA'")
		fixture.post(t, printerClerk, bill.Account.ID, "documents/retry", map[string]any{"ticket_id": precheck.TicketID, "job_id": precheck.JobID}, 200)
		if printer, content, _ := snapshot(precheck.JobID); printer.Destination != "ADEL_REV_COLA_NUEVA" || content != original {
			t.Fatalf("El reintento no tomó la cola corregida o cambió el documento con el mismo papel: %s", printer.Destination)
		}
		// 2. Desde la cola, con otro papel: la fotografía es la vigente y la precuenta se prepara para 58 mm con los
		// mismos datos (cada renglón cabe en 30 columnas y el total es el mismo).
		fixture.runWorkerWith(t, precheck.JobID, "error")
		updatePrinter("paper_width=58")
		detail := decodeResponse[struct {
			Job kitchen.PrintJob `json:"job"`
		}](t, fixture.adminCall(t, "GET", "kitchen/jobs/"+precheck.JobID, nil, 200))
		fixture.mutate(t, "POST", "kitchen/jobs/"+precheck.JobID+"/retry", map[string]any{"revision": detail.Job.Revision}, 200)
		total := ""
		for _, row := range strings.Split(original, "\n") {
			if strings.HasPrefix(strings.TrimSpace(row), "TOTAL") {
				fields := strings.Fields(row)
				total = fields[len(fields)-1]
				break
			}
		}
		printer, narrow, _ := snapshot(precheck.JobID)
		if printer.PaperWidth != 58 || narrow == original || total == "" || !strings.Contains(narrow, "CFDI") || !strings.Contains(narrow, total) {
			t.Fatalf("El reintento de la cola no preparó la precuenta para el papel vigente:\n%s", narrow)
		}
		for _, row := range strings.Split(narrow, "\n") {
			if len([]rune(row)) > 30 {
				t.Fatalf("Un renglón no cabe en 58 mm: %q", row)
			}
		}
		// 3. Si la impresora de tickets de Cobros cambió, el reintento del aviso va a la nueva («Configurar
		// impresora» lleva a elegirla allí), preparada otra vez para su papel de 80 mm.
		setJobState("error")
		fixture.receiptSettings(t, valid.ID, false)
		fixture.post(t, printerClerk, bill.Account.ID, "documents/retry", map[string]any{"ticket_id": precheck.TicketID, "job_id": precheck.JobID}, 200)
		if printer, content, printerID := snapshot(precheck.JobID); printerID != valid.ID || printer.Destination != "ADEL_TEST_9F_TICKETS" ||
			printer.PaperWidth != 80 || content != original {
			t.Fatalf("El reintento no fue a la impresora de tickets vigente: %s %s", printerID, printer.Destination)
		}
		// 4. Sin una impresora válida, el aviso sigue ofreciendo «Configurar impresora».
		setJobState("error")
		fixture.forceReceiptPrinter(t, agent.ID)
		unavailable := decodeResponse[printerError](t, fixture.post(t, printerClerk, bill.Account.ID, "documents/retry",
			map[string]any{"ticket_id": precheck.TicketID}, 409))
		if unavailable.Error.Code != "PRINTER_UNAVAILABLE" || unavailable.Error.Details["problem"] != kitchen.ProblemUnsupported ||
			unavailable.Error.Details["document"] != "precheck" {
			t.Fatalf("El reintento sin impresora válida no ofrece configurarla: %+v", unavailable.Error)
		}
		// 5. Reimprimir (con motivo) sale en la impresora de tickets vigente con su destino actual, pero idéntica
		// al original: la reimpresión exacta conserva el papel y las columnas con que se emitió (decisión de la 9C).
		setJobState("submitted")
		fixture.receiptSettings(t, fixed.ID, false)
		updatePrinter("destination='ADEL_REV_COLA_OTRA',paper_width=58")
		copied := decodeResponse[printingResult](t, fixture.post(t, printerClerk, bill.Account.ID, "documents/reprint",
			map[string]any{"ticket_id": precheck.TicketID, "reason": "Copia en la cola corregida"}, 200))
		copyPrinter, copyContent, copyPrinterID := snapshot(copied.JobID)
		if copyPrinterID != fixed.ID || copyPrinter.Destination != "ADEL_REV_COLA_OTRA" || copyPrinter.PaperWidth != 80 || copyContent != original {
			t.Fatalf("La reimpresión no salió con la impresora vigente e idéntica: %s %d\n%s", copyPrinter.Destination, copyPrinter.PaperWidth, copyContent)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='cancelled' WHERE ticket_id=$1", precheck.TicketID); err != nil {
			t.Fatal(err)
		}
		fixture.receiptSettings(t, valid.ID, false)
	})
	t.Run("9F_B3_impresora_de_tickets_que_puede_imprimir", func(t *testing.T) {
		save := func(printerID any, status int) []byte {
			t.Helper()
			settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
			return fixture.adminCall(t, "PUT", "payments/settings", map[string]any{
				"operation_id": randomID(), "revision": settings.Revision, "shared_shifts": settings.SharedShifts,
				"print_receipt_default": settings.PrintReceiptDefault, "receipt_printer_id": printerID,
				"refund_authorization_threshold": settings.RefundAuthorizationThreshold,
			}, status)
		}
		fixture.receiptSettings(t, nil, false)
		for _, testCase := range []struct {
			printer string
			problem string
		}{{agent.ID, kitchen.ProblemUnsupported}, {inactive.ID, kitchen.ProblemInactive}} {
			rejected := decodeResponse[printerError](t, save(testCase.printer, 422))
			if rejected.Error.Code != "RECEIPT_PRINTER_UNAVAILABLE" || rejected.Error.Details["problem"] != testCase.problem ||
				!strings.Contains(rejected.Error.Message, "Elige otra impresora de tickets.") {
				t.Fatalf("Se aceptó como impresora de tickets una que no puede imprimir: %+v", rejected.Error)
			}
		}
		if settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200)); settings.ReceiptPrinterID != nil {
			t.Fatal("El rechazo guardó la impresora")
		}
		// La que ya estaba elegida y dejó de servir no impide guardar las demás políticas.
		save(valid.ID, 200)
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_printers SET active=false WHERE id=$1", valid.ID); err != nil {
			t.Fatal(err)
		}
		save(valid.ID, 200)
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_printers SET active=true WHERE id=$1", valid.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("9F_nombre_visible_de_la_precuenta", func(t *testing.T) {
		fixture.receiptSettings(t, valid.ID, false)
		bill := fixture.account(t, "Nombre 9F", line(fixture.soda, 1))
		fixture.setPrecheckName(t, "ticket", 422)
		// Revisión 9F (M3): se elige en Administración › Documentos, con su permiso; Cobros ya no lo guarda.
		identityBody := decodeResponse[map[string]any](t, fixture.adminCall(t, "GET", "documents/identity", nil, 200))
		if identityBody["precheck_name"] != "precheck" {
			t.Fatalf("La identidad no informa el nombre de la precuenta: %v", identityBody["precheck_name"])
		}
		fixture.call(t, fixture.observer, "PUT", "documents/identity", map[string]any{
			"operation_id": randomID(), "revision": identityBody["revision"], "additional_info": identityBody["additional_info"],
			"header_text": identityBody["header_text"], "footer_text": identityBody["footer_text"],
			"presentation": identityBody["presentation"], "precheck_name": "bill",
		}, 403)
		settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
		fixture.adminCall(t, "PUT", "payments/settings", map[string]any{
			"operation_id": randomID(), "revision": settings.Revision, "shared_shifts": settings.SharedShifts,
			"print_receipt_default": settings.PrintReceiptDefault, "receipt_printer_id": settings.ReceiptPrinterID,
			"refund_authorization_threshold": settings.RefundAuthorizationThreshold, "precheck_name": "bill",
		}, 400)
		fixture.setPrecheckName(t, "sales_ticket", 200)
		document := decodeResponse[payments.CommercialDocument](t, fixture.adminCall(t, "GET", "payments/accounts/"+bill.Account.ID+"/precheck", nil, 200))
		if !strings.Contains(document.Content, "TICKET DE VENTA") || strings.Contains(document.Content, "PRECUENTA") ||
			!strings.Contains(document.Content, "CFDI") {
			t.Fatalf("El encabezado impreso no usa el nombre visible:\n%s", document.Content)
		}
		identity := decodeResponse[struct {
			Establishment map[string]any `json:"establishment"`
		}](t, fixture.call(t, fixture.observer, "GET", "auth/me", nil, 200))
		if identity.Establishment["precheck_name"] != "sales_ticket" {
			t.Fatalf("La sesión no informa el nombre visible: %+v", identity.Establishment)
		}
		fixture.receiptSettings(t, nil, false)
		missing := decodeResponse[printerError](t, fixture.post(t, printerClerk, bill.Account.ID, "precheck/print", map[string]any{}, 409))
		if missing.Error.Message != "No hay impresora configurada para tickets de venta." {
			t.Fatalf("El aviso no usa el nombre visible: %q", missing.Error.Message)
		}
		// Un cliente anterior que no envía el nombre lo conserva.
		fixture.receiptSettings(t, valid.ID, false)
		if settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200)); settings.PrecheckName != "sales_ticket" {
			t.Fatal("Guardar sin el nombre visible lo cambió")
		}
		var audited string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT after_state->>'precheck_name' FROM app.audit_events
            WHERE action='documents.precheck_name_changed' AND before_state->>'precheck_name'='precheck' ORDER BY occurred_at DESC LIMIT 1`).Scan(&audited); err != nil || audited != "sales_ticket" {
			t.Fatalf("El cambio del nombre visible no se auditó: %q %v", audited, err)
		}
		fixture.setPrecheckName(t, "precheck", 200)
		document = decodeResponse[payments.CommercialDocument](t, fixture.adminCall(t, "GET", "payments/accounts/"+bill.Account.ID+"/precheck", nil, 200))
		if !strings.Contains(document.Content, "PRECUENTA") || document.Title != "" {
			t.Fatal("Con «Precuenta» el documento no es el de siempre")
		}
	})

	t.Run("9F_mesa_sin_repetir", func(t *testing.T) {
		fixture.receiptSettings(t, valid.ID, false)
		for index, testCase := range []struct{ table, context string }{
			{"Mesa 9F", "Mesa 9F"},
			{"Terraza 9F", "Mesa Terraza 9F"},
		} {
			table := quickTable(t, fixture.restaurantFixture, testCase.table, 700+index*120)
			opened := decodeResponse[map[string]json.RawMessage](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
				"name": "", "mode": "dine_in", "table_id": table.ID,
			}, 200))
			var account struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(opened["account"], &account); err != nil {
				t.Fatal(err)
			}
			document := decodeResponse[payments.CommercialDocument](t, fixture.adminCall(t, "GET", "payments/accounts/"+account.ID+"/precheck", nil, 200))
			if document.Context != testCase.context || strings.Contains(document.Content, "Mesa Mesa") {
				t.Fatalf("Contexto de la mesa %q: %q\n%s", testCase.table, document.Context, document.Content)
			}
		}
	})

	t.Run("9F_D5_corte_de_papel_validado_y_auditado", func(t *testing.T) {
		created := decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", map[string]any{
			"name": "Corte 9F", "transport": "cups", "destination": "ADEL_TEST_9F_CUT", "port": 9100, "paper_width": 80, "copies": 1,
			"active": true, "fallback_id": nil, "accepts_escpos": false, "cutter_option": "",
		}, 200))
		if !created.Cut || !created.SupportsCut || created.CutMode != kitchen.CutFull || created.CutFeedLines == nil || *created.CutFeedLines != 4 {
			t.Fatalf("Una impresora nueva de 80 mm no corta con 4 renglones: %+v", created)
		}
		narrow := decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", map[string]any{
			"name": "Corte 58 9F", "transport": "cups", "destination": "ADEL_TEST_9F_CUT58", "port": 9100, "paper_width": 58, "copies": 1,
			"active": true, "fallback_id": nil,
		}, 200))
		if narrow.Cut || narrow.SupportsCut {
			t.Fatalf("Una impresora nueva de 58 mm empezó cortando: %+v", narrow)
		}
		update := func(changes map[string]any, status int) kitchen.Printer {
			current := decodeResponse[[]kitchen.Printer](t, fixture.adminCall(t, "GET", "kitchen/printers", nil, 200))
			for _, printer := range current {
				if printer.ID == created.ID {
					body := map[string]any{
						"id": printer.ID, "revision": printer.Revision, "name": printer.Name, "transport": printer.Transport,
						"destination": printer.Destination, "port": printer.Port, "paper_width": printer.PaperWidth, "copies": printer.Copies,
						"active": printer.Active, "fallback_id": nil, "accepts_escpos": false, "cutter_option": "",
					}
					for key, value := range changes {
						body[key] = value
					}
					response := fixture.mutate(t, "PATCH", "kitchen/printers/"+printer.ID, body, status)
					if status == 200 {
						return decodeResponse[kitchen.Printer](t, response)
					}
					return printer
				}
			}
			t.Fatal("No se encontró la impresora")
			return kitchen.Printer{}
		}
		partial := update(map[string]any{"cut_mode": "partial", "cut_feed_lines": 8}, 200)
		if partial.CutMode != kitchen.CutPartial || *partial.CutFeedLines != 8 || !partial.Cut {
			t.Fatalf("El corte parcial no se guardó: %+v", partial)
		}
		update(map[string]any{"cut_mode": "medio"}, 400)
		update(map[string]any{"cut_feed_lines": 9}, 400)
		// Un cliente anterior que no envía tipo ni avance los conserva; quitar el corte quita supports_cut.
		kept := update(map[string]any{}, 200)
		if kept.CutMode != kitchen.CutPartial || *kept.CutFeedLines != 8 {
			t.Fatalf("Guardar sin los campos nuevos cambió el corte: %+v", kept)
		}
		off := update(map[string]any{"cut": false, "supports_cut": true}, 200)
		if off.Cut || off.SupportsCut {
			t.Fatalf("Quitar el corte no lo quitó del todo: %+v", off)
		}
		// PostgreSQL también rechaza valores fuera de la lista cerrada.
		for _, statement := range []string{
			"UPDATE app.kitchen_printers SET cut_feed_lines=9 WHERE id=$1",
			"UPDATE app.kitchen_printers SET cut_feed_lines=-1 WHERE id=$1",
			"UPDATE app.kitchen_printers SET cut_mode='medio' WHERE id=$1",
		} {
			if _, err := fixture.pool.Exec(fixture.ctx, statement, created.ID); err == nil {
				t.Fatalf("PostgreSQL aceptó %s", statement)
			}
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='kitchen.printer.saved' AND entity_id=$1", created.ID) < 4 {
			t.Fatal("Los cambios del corte no quedaron en el Registro de movimientos")
		}
		fixture.call(t, fixture.observer, "PATCH", "kitchen/printers/"+created.ID, map[string]any{"operation_id": randomID()}, 403)
		// La prueba de impresión (como la de ancho, que cuenta printer_columns_9c_test.go) sale con el corte
		// de la impresora: su fotografía en el trabajo.
		update(map[string]any{"cut": true, "cut_mode": "partial", "cut_feed_lines": 2}, 200)
		width := decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/test-print", map[string]any{"printer_id": created.ID}, 200))
		var snapshot kitchen.Printer
		var raw []byte
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT printer_snapshot FROM app.kitchen_print_jobs WHERE id=$1", width.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			t.Fatal(err)
		}
		// La prueba técnica es una comanda: termina con su salto, así que el final son los 2 renglones y GS V 1.
		if ending := snapshot.PaperCut().Ending(false); string(ending) != "\n\n\x1dV\x01" {
			t.Fatalf("La prueba de impresión no corta como la impresora: % x", ending)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='cancelled' WHERE id=$1", width.ID); err != nil {
			t.Fatal(err)
		}
	})
}
