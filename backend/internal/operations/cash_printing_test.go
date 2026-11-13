//go:build integration

package operations_test

import (
	"fmt"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/moneytext"
	"local/ad-el-pos/server/internal/payments"
)

// recordedOperation es la respuesta de un ingreso, retiro o gasto con el estado de la impresión
// de su comprobante y del comando del cajón (fase C).
type recordedOperation struct {
	payments.CashOperation
	Printing *payments.PrintOutcome  `json:"printing"`
	Drawer   *payments.DrawerOutcome `json:"drawer"`
}

// printingFixture reúne la caja, los usuarios y las impresoras de la aceptación de la fase C. No
// hay hardware: las impresoras son destinos de prueba y el trabajador usa el adaptador controlado
// recordingTransport; ningún trabajo llega a CUPS ni a una conexión TCP.
type printingFixture struct {
	*cashFixture
	registerID string
	// cashier imprime y reimprime, autoriza sus salidas y abre el cajón, sin consultar el esperado;
	// supervisor imprime, reimprime y consulta el esperado; writer registra sin imprimir; observer
	// solo consulta.
	cashier       testTerminal
	supervisor    testTerminal
	writer        testTerminal
	observer      testTerminal
	tickets       kitchen.Printer
	drawerPrinter kitchen.Printer
	agent         kitchen.Printer
	shifts        map[string]string
	documents     map[string]payments.PrintOutcome
}

func (fixture *printingFixture) setReceiptPrinter(t *testing.T, printerID *string) {
	t.Helper()
	latest := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
	fixture.adminCall(t, "PUT", "payments/settings", map[string]any{
		"operation_id": randomID(), "revision": latest.Revision, "shared_shifts": latest.SharedShifts,
		"print_receipt_default": latest.PrintReceiptDefault, "receipt_printer_id": printerID,
		"refund_authorization_threshold": latest.RefundAuthorizationThreshold,
	}, 200)
}

// jobContent devuelve el texto guardado de un trabajo de impresión.
func (fixture *printingFixture) jobContent(t *testing.T, jobID string) string {
	t.Helper()
	var content string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT content FROM app.kitchen_print_jobs WHERE id=$1", jobID).Scan(&content); err != nil {
		t.Fatal(err)
	}
	return content
}

func (fixture *printingFixture) jobState(t *testing.T, jobID string) (string, int, int) {
	t.Helper()
	var state string
	var attempts, revision int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT state,attempts,revision FROM app.kitchen_print_jobs WHERE id=$1", jobID).Scan(
		&state, &attempts, &revision); err != nil {
		t.Fatal(err)
	}
	return state, attempts, revision
}

// processOnly entrega exactamente un trabajo al adaptador controlado: los demás trabajos pendientes
// de pruebas anteriores se cancelan para que el trabajador no los tome primero.
func (fixture *printingFixture) processOnly(t *testing.T, jobID, state string) []kitchen.PrintJob {
	t.Helper()
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.kitchen_print_jobs SET state='cancelled',lease_until=NULL
        WHERE state IN ('pending','error','processing') AND id<>$1`, jobID); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingTransport{state: state}
	if _, err := (&kitchen.Worker{Pool: fixture.pool, Transport: adapter}).PrintNext(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	return adapter.jobs
}

// closeShift inicia el arqueo, cuenta $1,000.00 y confirma el cierre con el cuerpo indicado.
func (fixture *printingFixture) closeShift(t *testing.T, terminal testTerminal, shiftID string, body map[string]any, status int) []byte {
	t.Helper()
	fixture.shiftCall(t, terminal, "POST", shiftID, "closing", map[string]any{}, 200)
	saved := fixture.saveCount(t, terminal, shiftID, fixture.pesosCount(t, "1000"), "Cierre de la caja de impresión")
	body["count_revision"] = saved.Count.Revision
	return fixture.shiftCall(t, terminal, "POST", shiftID, "closing/confirm", body, status)
}

func (fixture *printingFixture) openFloat(t *testing.T, float string) string {
	t.Helper()
	return decodeResponse[payments.Shift](t, fixture.openShift(t, fixture.cashier, map[string]any{"opening_float": float}, 200)).ID
}

func newPrintingFixture(t *testing.T, base *cashFixture) *printingFixture {
	t.Helper()
	fixture := &printingFixture{cashFixture: base, shifts: map[string]string{}, documents: map[string]payments.PrintOutcome{}}
	fixture.registerID = fixture.createRegister(t, "Caja de impresión")
	fixture.cashier = newCashier(t, base.paymentsFixture, "cajero_impresion", append(append([]string{}, cashierPermissions...),
		"cash.reports.print", "cash.reports.reprint", "cash.outflows.authorize", "cash.corrections.authorize", "cash.drawer.open", "events.read"), fixture.registerID)
	fixture.supervisor = newCashier(t, base.paymentsFixture, "supervisor_impresion", []string{
		"payments.read", "cash.shifts.read", "cash.expected.read", "cash.reports.print", "cash.reports.reprint",
		"cash.partial_cuts.create", "cash.history.read", "cash.movements.read",
	}, fixture.registerID)
	fixture.writer = newCashier(t, base.paymentsFixture, "cajero_sin_impresion", cashierPermissions, fixture.registerID)
	fixture.observer = newCashier(t, base.paymentsFixture, "observador_impresion", []string{
		"payments.read", "cash.shifts.read", "cash.movements.read", "events.read",
	}, fixture.registerID)
	fixture.tickets = decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name: "Tickets de caja controlados", Transport: "cups", Destination: "ADEL_TEST_CASH_TICKETS", Active: true, PaperWidth: 80, Copies: 1,
	}, 200))
	// Destino ESC/POS de prueba en loopback: el adaptador controlado lo sustituye y nunca se conecta.
	fixture.drawerPrinter = decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name: "Cajón ESC/POS controlado", Transport: "escpos_tcp", Destination: "127.0.0.1", Port: 9109, Active: true, PaperWidth: 80, Copies: 2,
	}, 200))
	fixture.agent = decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name: "Agente de caja no disponible", Transport: "agent", Destination: "Agente futuro", Active: true, PaperWidth: 80, Copies: 1,
	}, 200))
	return fixture
}

// testCashPrinting prueba la fase C: documentos de caja, cajón de dinero, tope de caja y eventos.
func testCashPrinting(t *testing.T, base *cashFixture) {
	fixture := newPrintingFixture(t, base)
	original := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
	fixture.setReceiptPrinter(t, &fixture.tickets.ID)
	fixture.shifts["main"] = fixture.openFloat(t, "1000")
	testCashDocuments(t, fixture)
	testCashReprints(t, fixture)
	testCashDrawer(t, fixture)
	testCashLimit(t, fixture)
	testCashEvents(t, fixture)
	testFinalCutPrinting(t, fixture)
	testCashStage71(t, fixture)
	fixture.setReceiptPrinter(t, original.ReceiptPrinterID)
}

func testCashDocuments(t *testing.T, fixture *printingFixture) {
	shiftID := fixture.shifts["main"]
	operations := map[string]recordedOperation{}

	t.Run("C31_vista_previa_sin_escritura_y_comprobante_opcional", func(t *testing.T) {
		unprinted := decodeResponse[recordedOperation](t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "income", map[string]any{
			"type": "change_supply", "amount": "50", "concept": "Cambio sin comprobante",
		}, 200))
		if unprinted.Printing != nil || fixture.countRows(t, "SELECT count(*) FROM app.cash_documents WHERE cash_operation_id=$1", unprinted.ID) != 0 {
			t.Fatal("Sin la bandera print se imprimió un comprobante")
		}
		operations["unprinted"] = unprinted
		before := fixture.writeCounts(t)
		tickets := fixture.countRows(t, "SELECT count(*) FROM app.kitchen_tickets")
		preview := decodeResponse[payments.CashDocumentPreview](t, fixture.call(t, fixture.observer, "GET",
			"payments/cash-operations/"+unprinted.ID+"/preview", nil, 200))
		for _, fragment := range []string{"COMPROBANTE DE INGRESO", "Caja: Caja de impresión", "Cambio sin comprobante", "$50.00", "CFDI"} {
			if !strings.Contains(preview.Content, fragment) {
				t.Fatalf("La vista previa no muestra %q:\n%s", fragment, preview.Content)
			}
		}
		if preview.Document != nil || preview.IncludesExpected || preview.PaperWidth != 80 {
			t.Fatalf("La vista previa del comprobante es incoherente: %+v", preview)
		}
		if fixture.writeCounts(t) != before || fixture.countRows(t, "SELECT count(*) FROM app.kitchen_tickets") != tickets {
			t.Fatal("La vista previa escribió registros")
		}
		// Sin permiso de consultar movimientos no hay vista previa del comprobante.
		fixture.call(t, fixture.cashFixture.observer, "GET", "payments/cash-operations/"+unprinted.ID+"/preview", nil, 403)
		fixture.call(t, fixture.supervisor, "GET", "payments/cash-operations/"+randomID()+"/preview", nil, 404)
	})

	t.Run("C32_comprobantes_de_ingreso_retiro_y_gasto", func(t *testing.T) {
		comandas := fixture.countRows(t, "SELECT count(*) FROM app.kitchen_tickets WHERE kind NOT IN ('receipt','precheck','cash_report','cash_voucher','cash_drawer')")
		cases := []struct {
			key, action, title string
			body               map[string]any
			fragments          []string
		}{
			{"income", "income", "COMPROBANTE DE INGRESO", map[string]any{"type": "float_contribution", "amount": "100", "reference": "Sobre 1"},
				[]string{"Tipo: Aportación adicional al fondo", "$100.00", "Referencia: Sobre 1", "Registró: Prueba cajero_impresion"}},
			{"withdrawal", "withdrawals", "COMPROBANTE DE RETIRO", map[string]any{"type": "safekeeping", "amount": "200"},
				[]string{"Motivo: Resguardo", "$200.00", "Autorizó: Prueba cajero_impresion", "Con su propio permiso", "Firma"}},
			{"expense", "expenses", "COMPROBANTE DE GASTO", map[string]any{"category_id": fixture.categories["Insumos"], "amount": "30", "notes": "Hielo"},
				[]string{"Categoría: Insumos", "$30.00", "Observaciones: Hielo", "Firma"}},
		}
		for _, item := range cases {
			item.body["print"] = true
			recorded := decodeResponse[recordedOperation](t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID, item.action, item.body, 200))
			if recorded.Printing == nil || !recorded.Printing.Requested || recorded.Printing.Status != "pending" || recorded.Printing.IncludesExpected ||
				recorded.Printing.JobID == "" || recorded.Printing.DocumentID == "" {
				t.Fatalf("El comprobante de %s no se encoló: %+v", item.key, recorded.Printing)
			}
			content := fixture.jobContent(t, recorded.Printing.JobID)
			for _, fragment := range append([]string{item.title, "Caja: Caja de impresión", "Turno del"}, item.fragments...) {
				if !strings.Contains(content, fragment) {
					t.Fatalf("El comprobante de %s no muestra %q:\n%s", item.key, fragment, content)
				}
			}
			if strings.Contains(strings.ToLower(content), "esperado") {
				t.Fatalf("El comprobante de %s mencionó el efectivo esperado", item.key)
			}
			var kind, ticketKind string
			var includes bool
			if err := fixture.pool.QueryRow(fixture.ctx, `SELECT d.kind,t.kind,d.includes_expected FROM app.cash_documents d
                JOIN app.kitchen_tickets t ON t.id=d.ticket_id WHERE d.id=$1 AND d.cash_operation_id=$2`,
				recorded.Printing.DocumentID, recorded.ID).Scan(&kind, &ticketKind, &includes); err != nil {
				t.Fatal(err)
			}
			if kind != item.key || ticketKind != "cash_voucher" || includes {
				t.Fatalf("El documento de %s quedó mal registrado: %s %s %v", item.key, kind, ticketKind, includes)
			}
			operations[item.key] = recorded
			fixture.documents[item.key] = *recorded.Printing
		}
		// La bandera print exige cash.reports.print antes de escribir: nada se registra.
		before := fixture.countRows(t, "SELECT count(*) FROM app.cash_operations")
		fixture.shiftCall(t, fixture.writer, "POST", shiftID, "income", map[string]any{"type": "other", "amount": "5", "print": true}, 403)
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_operations") != before {
			t.Fatal("Una operación con impresión no autorizada quedó registrada")
		}
		// Los documentos de caja no aparecen entre las comandas.
		listed := string(fixture.adminCall(t, "GET", "kitchen/tickets", nil, 200))
		if strings.Contains(listed, fixture.documents["income"].TicketID) ||
			fixture.countRows(t, "SELECT count(*) FROM app.kitchen_tickets WHERE kind NOT IN ('receipt','precheck','cash_report','cash_voucher','cash_drawer')") != comandas {
			t.Fatal("Un comprobante de caja apareció entre las comandas")
		}
		// Las correcciones no tienen comprobante propio.
		correction := decodeResponse[struct {
			Correction payments.CashOperation `json:"correction"`
		}](t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "corrections",
			map[string]any{"corrects_operation_id": operations["unprinted"].ID, "reason": "Ingreso capturado de más"}, 200))
		if errorCode(t, fixture.call(t, fixture.cashier, "GET", "payments/cash-operations/"+correction.Correction.ID+"/preview", nil, 409)) != "CASH_DOCUMENT_UNSUPPORTED" {
			t.Fatal("Una corrección ofreció comprobante")
		}
	})

	t.Run("C33_corte_parcial_impreso_con_y_sin_permiso_de_esperado", func(t *testing.T) {
		expected := fixture.expected(t, shiftID)
		// El esperado se busca con el mismo formato impreso (miles y dos decimales).
		expectedText := moneytext.Format(moneytext.CurrencyMXN, expected.MXN)
		hidden := fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "partial-cuts", map[string]any{"print": true}, 200)
		blind(t, hidden, "El corte parcial impreso sin permiso")
		cut := decodeResponse[payments.CreatedReport](t, hidden)
		if cut.Printing == nil || cut.Printing.Status != "pending" || cut.Printing.IncludesExpected {
			t.Fatalf("El corte parcial sin permiso no se imprimió sin esperado: %+v", cut.Printing)
		}
		content := fixture.jobContent(t, cut.Printing.JobID)
		for _, fragment := range []string{"CORTE PARCIAL #1", "VENTAS Y PAGOS POR MÉTODO", "INGRESOS, RETIROS Y GASTOS", "Retiros", "$200.00",
			"Gastos", "$30.00", "Fondo inicial", "$1,000.00", "Esperado y diferencias: no incluidos."} {
			if !strings.Contains(content, fragment) {
				t.Fatalf("El corte parcial sin permiso no muestra %q:\n%s", fragment, content)
			}
		}
		if strings.Contains(content, "EFECTIVO ESPERADO") || strings.Contains(content, expectedText) {
			t.Fatalf("El corte parcial sin permiso imprimió el esperado (%s):\n%s", expected.MXN, content)
		}
		visible := fixture.shiftCall(t, fixture.supervisor, "POST", shiftID, "partial-cuts", map[string]any{"print": true}, 200)
		blind(t, visible, "La respuesta del corte impreso con permiso")
		withExpected := decodeResponse[payments.CreatedReport](t, visible)
		if withExpected.Printing == nil || !withExpected.Printing.IncludesExpected {
			t.Fatalf("El corte con permiso no incluyó el esperado: %+v", withExpected.Printing)
		}
		content = fixture.jobContent(t, withExpected.Printing.JobID)
		if !strings.Contains(content, "CORTE PARCIAL #2") || !strings.Contains(content, "EFECTIVO ESPERADO") || !strings.Contains(content, expectedText) {
			t.Fatalf("El corte con permiso no muestra el esperado %s:\n%s", expected.MXN, content)
		}
		// La vista previa respeta los mismos permisos sin escribir.
		before := fixture.countRows(t, "SELECT count(*) FROM app.cash_documents")
		previewHidden := decodeResponse[payments.CashDocumentPreview](t, fixture.call(t, fixture.cashier, "GET",
			"payments/cash-reports/"+withExpected.ID+"/preview", nil, 200))
		previewVisible := decodeResponse[payments.CashDocumentPreview](t, fixture.call(t, fixture.supervisor, "GET",
			"payments/cash-reports/"+withExpected.ID+"/preview", nil, 200))
		if previewHidden.IncludesExpected || strings.Contains(previewHidden.Content, "EFECTIVO ESPERADO") ||
			!previewVisible.IncludesExpected || !strings.Contains(previewVisible.Content, expectedText) ||
			previewVisible.Document == nil || previewVisible.Document.ID != withExpected.Printing.DocumentID {
			t.Fatal("La vista previa del corte no respetó los permisos de consulta del esperado")
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_documents") != before {
			t.Fatal("La vista previa del corte registró un documento")
		}
		fixture.documents["partial_hidden"] = *cut.Printing
		fixture.documents["partial_visible"] = *withExpected.Printing
		fixture.reports["printing_partial_visible"] = withExpected.ID
		fixture.reports["printing_partial_hidden"] = cut.ID
	})
}

func testCashReprints(t *testing.T, fixture *printingFixture) {
	shiftID := fixture.shifts["main"]

	t.Run("C34_reimpresion_idempotente_con_permiso_y_sin_movimientos", func(t *testing.T) {
		voucher := fixture.documents["withdrawal"]
		route := "payments/cash-documents/" + voucher.DocumentID + "/reprint"
		// Con el trabajo original pendiente no se generan copias descontroladas.
		fixture.call(t, fixture.cashier, "POST", route, map[string]any{"operation_id": randomID(), "reason": "Copia para el encargado"}, 400)
		if jobs := fixture.processOnly(t, voucher.JobID, "submitted"); len(jobs) != 1 || jobs[0].ID != voucher.JobID {
			t.Fatalf("El adaptador controlado no recibió el comprobante: %+v", jobs)
		}
		financial := []string{
			"SELECT count(*) FROM app.cash_operations", "SELECT count(*) FROM app.cash_movements",
			"SELECT count(*) FROM app.cash_shift_reports", "SELECT count(*) FROM app.payments",
		}
		before := []int{}
		for _, query := range financial {
			before = append(before, fixture.countRows(t, query))
		}
		var shiftRevision int
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT revision FROM app.cash_shifts WHERE id=$1", shiftID).Scan(&shiftRevision); err != nil {
			t.Fatal(err)
		}
		operationID := randomID()
		first := decodeResponse[payments.PrintOutcome](t, fixture.call(t, fixture.cashier, "POST", route, map[string]any{"operation_id": operationID, "reason": "Copia para el encargado"}, 200))
		again := decodeResponse[payments.PrintOutcome](t, fixture.call(t, fixture.cashier, "POST", route, map[string]any{"operation_id": operationID, "reason": "Copia para el encargado"}, 200))
		if first.JobID == "" || first.JobID != again.JobID || first.Status != "pending" {
			t.Fatalf("La reimpresión no es idempotente: %+v / %+v", first, again)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.kitchen_print_jobs WHERE ticket_id=$1", voucher.TicketID) != 2 {
			t.Fatal("El reintento de la reimpresión generó otra copia")
		}
		if fixture.jobContent(t, first.JobID) != fixture.jobContent(t, voucher.JobID) {
			t.Fatal("La copia no conserva el contenido del original")
		}
		if errorCode(t, fixture.call(t, fixture.cashier, "POST", route, map[string]any{"operation_id": randomID(), "reason": "Copia para el encargado"}, 409)) != "PRINT_COPY_PENDING" {
			t.Fatal("Se aceptó otra copia con una pendiente")
		}
		for index, query := range financial {
			if fixture.countRows(t, query) != before[index] {
				t.Fatalf("La reimpresión creó registros financieros (%s)", query)
			}
		}
		var revisionAfter int
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT revision FROM app.cash_shifts WHERE id=$1", shiftID).Scan(&revisionAfter); err != nil {
			t.Fatal(err)
		}
		if revisionAfter != shiftRevision {
			t.Fatal("La reimpresión modificó el turno")
		}
		// Sin el permiso de reimprimir se rechaza; desde la cola de cocina tampoco se copia.
		fixture.call(t, fixture.writer, "POST", route, map[string]any{"operation_id": randomID()}, 403)
		fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{"job_id": voucher.JobID, "reason": "Copia desde la cola de cocina"}, 400)
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.document_reprinted'") < 1 {
			t.Fatal("La reimpresión no quedó auditada")
		}
	})

	t.Run("C35_documento_con_esperado_solo_se_reimprime_con_su_permiso", func(t *testing.T) {
		visible := fixture.documents["partial_visible"]
		route := "payments/cash-documents/" + visible.DocumentID + "/reprint"
		fixture.processOnly(t, visible.JobID, "submitted")
		fixture.call(t, fixture.cashier, "POST", route, map[string]any{"operation_id": randomID(), "reason": "Copia para el encargado"}, 403)
		copy := decodeResponse[payments.PrintOutcome](t, fixture.call(t, fixture.supervisor, "POST", route, map[string]any{"operation_id": randomID(), "reason": "Copia para el encargado"}, 200))
		if !copy.IncludesExpected || copy.JobID == visible.JobID {
			t.Fatalf("La reimpresión del corte con esperado es incoherente: %+v", copy)
		}
		// La cola de cocina no muestra el contenido con esperado a quien no puede consultarlo.
		ticket := fixture.adminCall(t, "GET", "kitchen/tickets/"+visible.TicketID, nil, 200)
		if !strings.Contains(string(ticket), "EFECTIVO ESPERADO") {
			t.Fatal("La cola no muestra el corte al administrador")
		}
		hidden := fixture.documents["partial_hidden"]
		listed := decodeResponse[struct {
			Documents []payments.CashDocumentRecord `json:"documents"`
		}](t, fixture.shiftCall(t, fixture.observer, "GET", shiftID, "documents", nil, 200))
		found := map[string]payments.CashDocumentRecord{}
		for _, document := range listed.Documents {
			found[document.ID] = document
		}
		if found[visible.DocumentID].Copies != 2 || !found[visible.DocumentID].IncludesExpected || found[hidden.DocumentID].IncludesExpected ||
			found[fixture.documents["withdrawal"].DocumentID].Copies != 2 || len(listed.Documents) < 5 {
			t.Fatalf("La lista de documentos del turno es incoherente: %+v", listed.Documents)
		}
	})

	t.Run("C36_impresion_posterior_unica_por_operacion", func(t *testing.T) {
		unprinted := fixture.countRows(t, `SELECT count(*) FROM app.cash_operations o WHERE o.shift_id=$1 AND o.kind='income'
            AND NOT EXISTS(SELECT FROM app.cash_documents d WHERE d.cash_operation_id=o.id)`, shiftID)
		if unprinted == 0 {
			t.Fatal("No queda un ingreso sin comprobante para imprimir después")
		}
		var operationID string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT o.id FROM app.cash_operations o WHERE o.shift_id=$1 AND o.kind='income'
            AND NOT EXISTS(SELECT FROM app.cash_documents d WHERE d.cash_operation_id=o.id) ORDER BY o.created_at LIMIT 1`, shiftID).Scan(&operationID); err != nil {
			t.Fatal(err)
		}
		route := "payments/cash-operations/" + operationID + "/print"
		fixture.call(t, fixture.writer, "POST", route, map[string]any{"operation_id": randomID()}, 403)
		request := map[string]any{"operation_id": randomID()}
		printed := decodeResponse[payments.PrintOutcome](t, fixture.call(t, fixture.cashier, "POST", route, request, 200))
		replayed := decodeResponse[payments.PrintOutcome](t, fixture.call(t, fixture.cashier, "POST", route, request, 200))
		if printed.DocumentID == "" || printed.DocumentID != replayed.DocumentID ||
			fixture.countRows(t, "SELECT count(*) FROM app.cash_documents WHERE cash_operation_id=$1", operationID) != 1 {
			t.Fatal("La impresión posterior no es idempotente")
		}
		if errorCode(t, fixture.call(t, fixture.cashier, "POST", route, map[string]any{"operation_id": randomID()}, 409)) != "CASH_DOCUMENT_EXISTS" {
			t.Fatal("Una segunda impresión no se trató como reimpresión")
		}
	})
}

func testCashDrawer(t *testing.T, fixture *printingFixture) {
	shiftID := fixture.shifts["main"]
	route := "payments/cash/drawers/" + fixture.registerID
	cash := fixture.methods["cash"].ID
	debit := fixture.methods["debit"].ID
	var paymentDrawer payments.DrawerOutcome

	t.Run("C37_configuracion_del_cajon_con_protocolo_incompatible_rechazada", func(t *testing.T) {
		fixture.call(t, fixture.cashier, "PUT", route, map[string]any{
			"operation_id": randomID(), "revision": 0, "printer_id": fixture.drawerPrinter.ID, "pulse_pin": 0, "enabled": true,
		}, 403)
		// Etapa 7.1: el agente sigue sin entregar el pulso; una cola CUPS se acepta solo si admite
		// datos raw (aquí, una cola AirPrint simulada que no los admite).
		problem := fixture.adminCall(t, "PUT", route, map[string]any{
			"operation_id": randomID(), "revision": 0, "printer_id": fixture.agent.ID, "pulse_pin": 0, "enabled": true,
		}, 400)
		if errorCode(t, problem) != "DRAWER_PROTOCOL_UNSUPPORTED" || !strings.Contains(string(problem), "ESC/POS") {
			t.Fatalf("Se aceptó el cajón con el agente: %s", problem)
		}
		fixture.queues.set(fixture.tickets.Destination, map[string]string{
			"device-uri": "dnssd://Tickets._ipps._tcp.local./", "printer-make-and-model": "Tickets de caja-AirPrint", "printer-type": "69243084",
		})
		problem = fixture.adminCall(t, "PUT", route, map[string]any{
			"operation_id": randomID(), "revision": 0, "printer_id": fixture.tickets.ID, "pulse_pin": 0, "enabled": true,
		}, 400)
		if errorCode(t, problem) != "DRAWER_QUEUE_NOT_RAW" || !strings.Contains(string(problem), "AirPrint") {
			t.Fatalf("Se aceptó el cajón en una cola CUPS sin datos raw: %s", problem)
		}
		fixture.adminCall(t, "PUT", route, map[string]any{"operation_id": randomID(), "revision": 0, "pulse_pin": 0, "enabled": true}, 400)
		fixture.adminCall(t, "PUT", route, map[string]any{
			"operation_id": randomID(), "revision": 0, "printer_id": fixture.drawerPrinter.ID, "pulse_pin": 3, "enabled": true,
		}, 400)
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_settings WHERE register_id=$1", fixture.registerID) != 0 {
			t.Fatal("Una configuración rechazada quedó guardada")
		}
		saved := decodeResponse[map[string]any](t, fixture.adminCall(t, "PUT", route, map[string]any{
			"operation_id": randomID(), "revision": 0, "printer_id": fixture.drawerPrinter.ID, "pulse_pin": 0, "enabled": true,
		}, 200))
		if saved["enabled"] != true || saved["revision"].(float64) != 1 {
			t.Fatalf("El cajón no se configuró: %+v", saved)
		}
		if errorCode(t, fixture.adminCall(t, "PUT", route, map[string]any{
			"operation_id": randomID(), "revision": 0, "printer_id": fixture.drawerPrinter.ID, "pulse_pin": 1, "enabled": true,
		}, 409)) != "REVISION_CONFLICT" {
			t.Fatal("Se sobrescribió la configuración del cajón con una revisión obsoleta")
		}
		listed := decodeResponse[struct {
			Drawers  []payments.DrawerSettings `json:"drawers"`
			Printers []payments.DrawerPrinter  `json:"printers"`
		}](t, fixture.adminCall(t, "GET", "payments/cash/drawers", nil, 200))
		configured, compatible := false, map[string]bool{}
		for _, drawer := range listed.Drawers {
			configured = configured || (drawer.RegisterID == fixture.registerID && drawer.Enabled && text(drawer.PrinterName) == "Cajón ESC/POS controlado")
		}
		for _, printer := range listed.Printers {
			compatible[printer.ID] = printer.Compatible
		}
		if !configured || !compatible[fixture.drawerPrinter.ID] || compatible[fixture.tickets.ID] || compatible[fixture.agent.ID] {
			t.Fatalf("La lista de cajones no refleja la configuración: %+v", listed)
		}
		registers := decodeResponse[[]payments.RegisterShift](t, fixture.call(t, fixture.cashier, "GET", "payments/cash/registers", nil, 200))
		if len(registers) != 1 || !registers[0].DrawerEnabled {
			t.Fatalf("La caja no informa su cajón activado: %+v", registers)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.drawer_settings_changed'") != 1 {
			t.Fatal("La configuración del cajón no quedó auditada")
		}
	})

	t.Run("C38_apertura_automatica_con_efectivo_y_nunca_con_tarjeta", func(t *testing.T) {
		before := fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events")
		paid := fixture.account(t, "Cobro con cajón", line(fixture.soda, 1))
		request := map[string]any{"operation_id": randomID(), "method_id": cash, "mode": "total", "shift_id": shiftID}
		body := fixture.pay(t, fixture.cashier, paid.Account.ID, request, 200)
		result := decodeResponse[struct {
			Drawer payments.DrawerOutcome `json:"drawer"`
		}](t, body)
		if !result.Drawer.Requested || result.Drawer.Status != "pending" || result.Drawer.JobID == "" {
			t.Fatalf("El cobro en efectivo no envió el comando del cajón: %+v", result.Drawer)
		}
		// El reintento con el mismo identificador recupera el resultado sin otro comando.
		replayed := decodeResponse[struct {
			Drawer payments.DrawerOutcome `json:"drawer"`
		}](t, fixture.pay(t, fixture.cashier, paid.Account.ID, request, 200))
		if replayed.Drawer.JobID != result.Drawer.JobID {
			t.Fatal("El reintento del cobro emitió otro comando del cajón")
		}
		var reason, jobPrinter, ticketKind string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT e.reason,j.printer_id,t.kind FROM app.cash_drawer_events e
            JOIN app.kitchen_print_jobs j ON j.id=e.job_id JOIN app.kitchen_tickets t ON t.id=j.ticket_id WHERE e.job_id=$1 AND e.shift_id=$2`,
			result.Drawer.JobID, shiftID).Scan(&reason, &jobPrinter, &ticketKind); err != nil {
			t.Fatal(err)
		}
		if reason != "payment" || jobPrinter != fixture.drawerPrinter.ID || ticketKind != "cash_drawer" {
			t.Fatalf("El comando del cajón quedó mal registrado: %s %s %s", reason, jobPrinter, ticketKind)
		}
		card := fixture.account(t, "Cobro con tarjeta", line(fixture.soda, 1))
		electronic := decodeResponse[struct {
			Drawer payments.DrawerOutcome `json:"drawer"`
		}](t, fixture.pay(t, fixture.cashier, card.Account.ID, map[string]any{"method_id": debit, "mode": "total", "reference": "1234"}, 200))
		if electronic.Drawer.Requested {
			t.Fatal("Un pago con tarjeta abrió el cajón")
		}
		refund := decodeResponse[struct {
			Drawer payments.DrawerOutcome `json:"drawer"`
		}](t, fixture.post(t, fixture.cashier, paid.Account.ID, "refunds", map[string]any{
			"mode": "total", "method_id": cash, "reason": "Devolución en efectivo con cajón",
		}, 200))
		if !refund.Drawer.Requested || refund.Drawer.Status != "pending" {
			t.Fatalf("La devolución en efectivo no envió el comando del cajón: %+v", refund.Drawer)
		}
		income := decodeResponse[recordedOperation](t, fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "income",
			map[string]any{"type": "change_supply", "amount": "20"}, 200))
		if income.Drawer == nil || !income.Drawer.Requested {
			t.Fatal("El ingreso no envió el comando del cajón")
		}
		reasons := fixture.countRows(t, `SELECT count(DISTINCT reason) FROM app.cash_drawer_events WHERE register_id=$1
            AND reason IN ('payment','refund','operation')`, fixture.registerID)
		if fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events") != before+3 || reasons != 3 {
			t.Fatal("Los comandos del cajón no corresponden a cobro, devolución e ingreso en efectivo")
		}
		// El comando solo describe la apertura y el adaptador recibe el pulso, nunca texto.
		jobs := fixture.processOnly(t, result.Drawer.JobID, "submitted")
		if len(jobs) != 1 || jobs[0].DrawerPulse == nil || jobs[0].DrawerPulse.Pin != 0 || jobs[0].TicketKind != "cash_drawer" ||
			jobs[0].PrinterSnapshot.Copies != 1 {
			t.Fatalf("El adaptador no recibió el pulso del cajón: %+v", jobs)
		}
		if state, _, _ := fixture.jobState(t, result.Drawer.JobID); state != "submitted" ||
			fixture.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='cash.print.updated' AND aggregate_id=$1", result.Drawer.JobID) != 1 {
			t.Fatal("El envío del comando no quedó registrado ni avisó en el ámbito de caja")
		}
		paymentDrawer = result.Drawer
	})

	t.Run("C39_apertura_manual_autorizada_auditada_y_rechazada_sin_permiso", func(t *testing.T) {
		before := fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events WHERE reason='manual'")
		fixture.call(t, fixture.observer, "POST", "payments/cash/open-drawer", map[string]any{"operation_id": randomID(), "reason": "Sin permiso"}, 403)
		fixture.call(t, fixture.cashier, "POST", "payments/cash/open-drawer", map[string]any{"operation_id": randomID(), "reason": "No"}, 400)
		if errorCode(t, fixture.adminCall(t, "POST", "payments/cash/open-drawer", map[string]any{
			"operation_id": randomID(), "reason": "Caja sin cajón configurado",
		}, 409)) != "DRAWER_DISABLED" {
			t.Fatal("Se abrió un cajón no activado")
		}
		request := map[string]any{"operation_id": randomID(), "reason": "Cambio de billetes para el fondo"}
		opened := decodeResponse[payments.DrawerOutcome](t, fixture.call(t, fixture.cashier, "POST", "payments/cash/open-drawer", request, 200))
		again := decodeResponse[payments.DrawerOutcome](t, fixture.call(t, fixture.cashier, "POST", "payments/cash/open-drawer", request, 200))
		if !opened.Requested || opened.Status != "pending" || opened.JobID != again.JobID {
			t.Fatalf("La apertura manual no es idempotente: %+v / %+v", opened, again)
		}
		var notes string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT notes FROM app.cash_drawer_events WHERE job_id=$1 AND reason='manual'", opened.JobID).Scan(&notes); err != nil {
			t.Fatal(err)
		}
		if notes != "Cambio de billetes para el fondo" || fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events WHERE reason='manual'") != before+1 ||
			fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.drawer_opened_manually'") != 1 {
			t.Fatal("La apertura manual no quedó registrada y auditada con su motivo")
		}
		fixture.registers["manual_drawer_job"] = opened.JobID
	})

	t.Run("C40_resultado_incierto_o_error_del_comando_sin_reintento_automatico", func(t *testing.T) {
		uncertain := fixture.registers["manual_drawer_job"]
		fixture.processOnly(t, uncertain, "uncertain")
		state, attempts, revision := fixture.jobState(t, uncertain)
		if state != "uncertain" || attempts != 1 {
			t.Fatalf("El comando incierto quedó en %s con %d intentos", state, attempts)
		}
		// El trabajador no vuelve a tomarlo, ni la cola permite reenviarlo.
		adapter := &recordingTransport{state: "submitted"}
		if _, err := (&kitchen.Worker{Pool: fixture.pool, Transport: adapter}).PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if len(adapter.jobs) != 0 {
			t.Fatal("El trabajador reenvió un comando del cajón")
		}
		fixture.mutate(t, "POST", "kitchen/jobs/"+uncertain+"/resolve", map[string]any{
			"revision": revision, "decision": "send", "reason": "Reenviar el comando incierto",
		}, 400)
		fixture.mutate(t, "POST", "kitchen/jobs/"+uncertain+"/resolve", map[string]any{
			"revision": revision, "decision": "accepted", "reason": "El cajero confirmó que el cajón abrió",
		}, 200)
		// Un error confirmado tampoco se reintenta solo: abrir el cajón más tarde sería un riesgo.
		failed := decodeResponse[payments.DrawerOutcome](t, fixture.call(t, fixture.cashier, "POST", "payments/cash/open-drawer", map[string]any{
			"operation_id": randomID(), "reason": "Segunda apertura de prueba",
		}, 200))
		fixture.processOnly(t, failed.JobID, "error")
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET next_attempt_at=now()-interval '1 minute' WHERE id=$1", failed.JobID); err != nil {
			t.Fatal(err)
		}
		retry := &recordingTransport{state: "submitted"}
		if _, err := (&kitchen.Worker{Pool: fixture.pool, Transport: retry}).PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		state, attempts, revision = fixture.jobState(t, failed.JobID)
		if len(retry.jobs) != 0 || state != "error" || attempts != 1 {
			t.Fatalf("Un comando con error se reintentó solo: %s, %d intentos", state, attempts)
		}
		fixture.mutate(t, "POST", "kitchen/jobs/"+failed.JobID+"/retry", map[string]any{"revision": revision, "reason": "Reintento manual"}, 400)
		// El cobro con su comando enviado sigue registrado.
		if paymentDrawer.JobID == "" || fixture.countRows(t, "SELECT count(*) FROM app.cash_drawer_events WHERE job_id=$1", paymentDrawer.JobID) != 1 {
			t.Fatal("El comando del cobro desapareció")
		}
	})

	t.Run("C41_cajon_con_impresora_desactivada_no_bloquea_el_cobro", func(t *testing.T) {
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_printers SET active=false WHERE id=$1", fixture.drawerPrinter.ID); err != nil {
			t.Fatal(err)
		}
		bill := fixture.account(t, "Cobro con cajón sin impresora", line(fixture.soda, 1))
		result := decodeResponse[struct {
			Payment payments.PaymentRecord `json:"payment"`
			Drawer  payments.DrawerOutcome `json:"drawer"`
		}](t, fixture.pay(t, fixture.cashier, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total"}, 200))
		if result.Payment.ID == "" || result.Drawer.Status != "unavailable" || result.Drawer.JobID != "" {
			t.Fatalf("La impresora desactivada del cajón bloqueó o alteró el cobro: %+v", result.Drawer)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_printers SET active=true WHERE id=$1", fixture.drawerPrinter.ID); err != nil {
			t.Fatal(err)
		}
	})
}

func testCashLimit(t *testing.T, fixture *printingFixture) {
	shiftID := fixture.shifts["main"]
	shiftRoute := "payments/shifts/" + shiftID
	hasWarning := func(body []byte) bool {
		return strings.Contains(string(body), `"cash_limit_warning"`)
	}
	routes := []string{shiftRoute, shiftRoute + "/summary", "payments/shifts/current"}

	t.Run("C42_tope_desactivado_sin_efecto", func(t *testing.T) {
		fixture.call(t, fixture.cashier, "PUT", "payments/cash/limit", map[string]any{"operation_id": randomID(), "revision": 0}, 403)
		limit := decodeResponse[payments.CashLimitSettings](t, fixture.adminCall(t, "GET", "payments/cash/limit", nil, 200))
		if limit.Enabled || limit.CashLimitMXN != nil {
			t.Fatalf("El tope no está desactivado por defecto: %+v", limit)
		}
		if errorCode(t, fixture.adminCall(t, "PUT", "payments/cash/limit", map[string]any{
			"operation_id": randomID(), "revision": limit.Revision, "cash_limit_mxn": "100",
		}, 409)) != "FEATURE_DISABLED" {
			t.Fatal("Se configuró el tope con la función desactivada")
		}
		// Aun con un importe guardado, la función desactivada no produce avisos.
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.payment_settings SET cash_limit_mxn=1"); err != nil {
			t.Fatal(err)
		}
		for _, route := range routes {
			if hasWarning(fixture.call(t, fixture.supervisor, "GET", route, nil, 200)) {
				t.Fatalf("El tope desactivado produjo un aviso en %s", route)
			}
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.payment_settings SET cash_limit_mxn=NULL"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("C43_tope_activado_aviso_solo_con_permiso_y_sin_bloquear_cobros", func(t *testing.T) {
		features := decodeResponse[[]struct {
			Code     string `json:"code"`
			Revision int    `json:"revision"`
		}](t, fixture.adminCall(t, "GET", "settings/features", nil, 200))
		revision := -1
		for _, feature := range features {
			if feature.Code == "cash_limit" {
				revision = feature.Revision
			}
		}
		if revision < 0 {
			t.Fatal("El tope de caja no aparece entre las funciones activables")
		}
		fixture.adminCall(t, "PUT", "settings/features", map[string]any{"code": "cash_limit", "enabled": true, "revision": revision}, 200)
		limit := decodeResponse[payments.CashLimitSettings](t, fixture.adminCall(t, "GET", "payments/cash/limit", nil, 200))
		if !limit.USDCash {
			problem := fixture.adminCall(t, "PUT", "payments/cash/limit", map[string]any{
				"operation_id": randomID(), "revision": limit.Revision, "cash_limit_mxn": "500", "cash_limit_usd": "10",
			}, 409)
			if errorCode(t, problem) != "FEATURE_DISABLED" {
				t.Fatal("Se aceptó un tope en dólares sin la función usd_cash")
			}
		}
		fixture.adminCall(t, "PUT", "payments/cash/limit", map[string]any{
			"operation_id": randomID(), "revision": limit.Revision, "cash_limit_mxn": "0",
		}, 400)
		saved := decodeResponse[payments.CashLimitSettings](t, fixture.adminCall(t, "PUT", "payments/cash/limit", map[string]any{
			"operation_id": randomID(), "revision": limit.Revision, "cash_limit_mxn": "500",
		}, 200))
		if !saved.Enabled || text(saved.CashLimitMXN) != "500.00" ||
			fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.limit_changed'") != 1 {
			t.Fatalf("El tope no se guardó auditado: %+v", saved)
		}
		if amountOf(t, fixture.expected(t, shiftID).MXN).LessThanOrEqual(amountOf(t, "500")) {
			t.Fatal("La prueba necesita un esperado mayor que el tope")
		}
		for _, route := range routes {
			body := fixture.call(t, fixture.supervisor, "GET", route, nil, 200)
			if !hasWarning(body) || !strings.Contains(string(body), "considera un retiro") || !strings.Contains(string(body), `"MXN"`) {
				t.Fatalf("Quien consulta el esperado no recibió el aviso del tope en %s: %s", route, body)
			}
			for _, terminal := range []testTerminal{fixture.cashier, fixture.observer} {
				if hasWarning(fixture.call(t, terminal, "GET", route, nil, 200)) {
					t.Fatalf("Un usuario sin permiso recibió el aviso del tope en %s", route)
				}
			}
		}
		// El aviso nunca bloquea cobros.
		bill := fixture.account(t, "Cobro con tope superado", line(fixture.soda, 1))
		fixture.pay(t, fixture.cashier, bill.Account.ID, map[string]any{"method_id": fixture.methods["cash"].ID, "mode": "total", "shift_id": shiftID}, 200)
	})

	t.Run("C44_tope_ausente_durante_el_arqueo", func(t *testing.T) {
		started := fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "closing", map[string]any{}, 200)
		blind(t, started, "El inicio del arqueo con tope")
		for _, route := range routes {
			body := fixture.call(t, fixture.supervisor, "GET", route, nil, 200)
			blind(t, body, "Durante el arqueo, "+route)
			if hasWarning(body) {
				t.Fatalf("El aviso del tope apareció durante el arqueo en %s", route)
			}
		}
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "closing/cancel", map[string]any{}, 200)
		if !hasWarning(fixture.call(t, fixture.supervisor, "GET", shiftRoute, nil, 200)) {
			t.Fatal("El aviso no volvió al reanudar la caja")
		}
	})
}

func testCashEvents(t *testing.T, fixture *printingFixture) {
	shiftID := fixture.shifts["main"]

	t.Run("C45_eventos_de_caja_publicados_sin_importes_y_filtrados_por_permiso", func(t *testing.T) {
		outsider := newCashier(t, fixture.paymentsFixture, "eventos_sin_caja", []string{"events.read", "accounts.read"}, fixture.registerID)
		var head int64
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(max(sequence),0) FROM app.realtime_events").Scan(&head); err != nil {
			t.Fatal(err)
		}
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "income", map[string]any{"type": "other", "amount": "15", "print": true}, 200)
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "partial-cuts", map[string]any{}, 200)
		fixture.call(t, fixture.cashier, "POST", "payments/cash/open-drawer", map[string]any{"operation_id": randomID(), "reason": "Apertura para eventos"}, 200)
		limit := decodeResponse[payments.CashLimitSettings](t, fixture.adminCall(t, "GET", "payments/cash/limit", nil, 200))
		fixture.adminCall(t, "PUT", "payments/cash/limit", map[string]any{"operation_id": randomID(), "revision": limit.Revision, "cash_limit_mxn": ""}, 200)
		bill := fixture.account(t, "Cobro para eventos", line(fixture.soda, 1))
		fixture.pay(t, fixture.cashier, bill.Account.ID, map[string]any{"method_id": fixture.methods["cash"].ID, "mode": "total"}, 200)
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "closing", map[string]any{}, 200)
		fixture.shiftCall(t, fixture.cashier, "POST", shiftID, "closing/cancel", map[string]any{}, 200)
		read := func(terminal testTerminal) []events.Change {
			t.Helper()
			result := []events.Change{}
			cursor := head
			for {
				batch := decodeResponse[events.Batch](t, fixture.call(t, terminal, "GET", fmt.Sprintf("events/snapshot?after=%d", cursor), nil, 200))
				if batch.Reset {
					t.Fatal("El cursor de eventos se reinició durante la prueba")
				}
				result = append(result, batch.Events...)
				if batch.Cursor == cursor {
					return result
				}
				cursor = batch.Cursor
			}
		}
		kinds := map[string]int{}
		for _, event := range read(fixture.cashier) {
			if event.Scope != "cash" {
				continue
			}
			kinds[event.Kind]++
			if event.AggregateID != nil && *event.AggregateID != shiftID && event.Kind != "cash.settings.changed" && event.Kind != "cash.print.updated" {
				t.Fatalf("El evento %s no identifica el turno: %v", event.Kind, *event.AggregateID)
			}
		}
		for _, kind := range []string{
			"cash.operation.recorded", "cash.document.printed", "cash.report.created", "cash.drawer.command_queued", "cash.settings.changed",
			"cash.movement.recorded", "cash.shift.closing_started", "cash.shift.closing_cancelled",
		} {
			if kinds[kind] == 0 {
				t.Fatalf("La escritura de caja no publicó %s: %v", kind, kinds)
			}
		}
		for _, event := range read(outsider) {
			if event.Scope == "cash" {
				t.Fatalf("Un usuario sin permisos de caja recibió %s", event.Kind)
			}
		}
		// Los eventos solo llevan referencias de invalidación, nunca importes.
		var payload string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT coalesce(string_agg(to_jsonb(e)::text,' '),'') FROM app.realtime_events e
            WHERE sequence>$1 AND scope='cash'`, head).Scan(&payload); err != nil {
			t.Fatal(err)
		}
		for _, fragment := range []string{"amount", "expected", "mxn"} {
			if strings.Contains(payload, fragment) {
				t.Fatalf("Un evento de caja contiene %q", fragment)
			}
		}
	})
}

func testFinalCutPrinting(t *testing.T, fixture *printingFixture) {
	t.Run("C46_cierre_sin_impresora_se_confirma_y_el_corte_se_imprime_despues", func(t *testing.T) {
		shiftID := fixture.shifts["main"]
		fixture.setReceiptPrinter(t, nil)
		closed := decodeResponse[struct {
			Closed        bool                  `json:"closed"`
			FinalReportID string                `json:"final_report_id"`
			Printing      payments.PrintOutcome `json:"printing"`
		}](t, fixture.closeShift(t, fixture.cashier, shiftID, map[string]any{"print": true}, 200))
		if !closed.Closed || closed.Printing.Status != "not_configured" {
			t.Fatalf("El cierre sin impresora no se confirmó o no informó la impresión: %+v", closed)
		}
		route := "payments/cash-reports/" + closed.FinalReportID + "/print"
		if errorCode(t, fixture.call(t, fixture.supervisor, "POST", route, map[string]any{"operation_id": randomID()}, 409)) != "PRINTER_NOT_CONFIGURED" {
			t.Fatal("Se imprimió sin impresora configurada")
		}
		fixture.setReceiptPrinter(t, &fixture.tickets.ID)
		closures := fixture.countRows(t, "SELECT count(*) FROM app.cash_shift_closures")
		printed := decodeResponse[payments.PrintOutcome](t, fixture.call(t, fixture.supervisor, "POST", route, map[string]any{"operation_id": randomID()}, 200))
		content := fixture.jobContent(t, printed.JobID)
		for _, fragment := range []string{"CORTE DEFINITIVO", "Cierre", "Cerró: Prueba cajero_impresion", "EFECTIVO DECLARADO", "$1,000.00",
			"EFECTIVO ESPERADO", "DIFERENCIAS (CONTADO - ESPERADO)", "Observaciones", "Cierre de la caja de impresión"} {
			if !strings.Contains(strings.ToLower(content), strings.ToLower(fragment)) {
				t.Fatalf("El corte definitivo con permiso no muestra %q:\n%s", fragment, content)
			}
		}
		if !printed.IncludesExpected || fixture.countRows(t, "SELECT count(*) FROM app.cash_shift_closures") != closures {
			t.Fatal("Imprimir el corte definitivo cambió el cierre o no incluyó el esperado")
		}
		// Quien cerró sin el permiso ve la vista previa sin esperado y no reimprime la copia con él.
		preview := decodeResponse[payments.CashDocumentPreview](t, fixture.call(t, fixture.cashier, "GET",
			"payments/cash-reports/"+closed.FinalReportID+"/preview", nil, 200))
		if preview.IncludesExpected || strings.Contains(preview.Content, "EFECTIVO ESPERADO") || !strings.Contains(preview.Content, "EFECTIVO DECLARADO") {
			t.Fatalf("La vista previa del corte definitivo sin permiso es incorrecta:\n%s", preview.Content)
		}
		fixture.processOnly(t, printed.JobID, "submitted")
		fixture.call(t, fixture.cashier, "POST", "payments/cash-documents/"+printed.DocumentID+"/reprint", map[string]any{"operation_id": randomID(), "reason": "Copia para el encargado"}, 403)
		// Apertura, conteo, cierre y comentario del turno también publicaron su evento de caja.
		fixture.shiftCall(t, fixture.admin, "POST", shiftID, "closure/comments", map[string]any{"comment": "Revisado sin observaciones"}, 200)
		for _, kind := range []string{"cash.shift.opened", "cash.closing.count_saved", "cash.shift.closed", "cash.closure.commented"} {
			if fixture.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE scope='cash' AND kind=$1 AND aggregate_id=$2", kind, shiftID) == 0 {
				t.Fatalf("El turno no publicó %s", kind)
			}
		}
	})

	t.Run("C47_corte_definitivo_impreso_sin_esperado_y_fallo_de_impresora_sin_revertir", func(t *testing.T) {
		shiftID := fixture.openFloat(t, "1000")
		closed := decodeResponse[struct {
			Closed   bool                  `json:"closed"`
			Printing payments.PrintOutcome `json:"printing"`
		}](t, fixture.closeShift(t, fixture.cashier, shiftID, map[string]any{"print": true}, 200))
		if !closed.Closed || closed.Printing.Status != "pending" || closed.Printing.IncludesExpected {
			t.Fatalf("El corte definitivo sin permiso no se encoló sin esperado: %+v", closed.Printing)
		}
		content := fixture.jobContent(t, closed.Printing.JobID)
		if !strings.Contains(content, "CORTE DEFINITIVO") || !strings.Contains(content, "EFECTIVO DECLARADO") ||
			strings.Contains(content, "EFECTIVO ESPERADO") || strings.Contains(content, "DIFERENCIAS") {
			t.Fatalf("El corte definitivo sin permiso mostró el esperado:\n%s", content)
		}
		// La impresora falla después del cierre: el turno sigue cerrado con su resultado.
		fixture.processOnly(t, closed.Printing.JobID, "error")
		var state string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT state FROM app.cash_shifts WHERE id=$1", shiftID).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != "closed" || fixture.countRows(t, "SELECT count(*) FROM app.cash_shift_closures WHERE shift_id=$1", shiftID) != 1 {
			t.Fatal("Un error de la impresora revirtió el cierre")
		}
		// Si el documento ni siquiera puede encolarse, el punto de guardado lo deshace y el cierre
		// se confirma igual. Un disparador temporal de esta base de prueba simula la falla.
		if _, err := fixture.pool.Exec(fixture.ctx, `CREATE FUNCTION app.test_fail_cash_document() RETURNS trigger LANGUAGE plpgsql AS
            $$ BEGIN RAISE EXCEPTION 'falla simulada de impresión'; END $$;
            CREATE TRIGGER test_fail_cash_document BEFORE INSERT ON app.cash_documents FOR EACH ROW EXECUTE FUNCTION app.test_fail_cash_document()`); err != nil {
			t.Fatal(err)
		}
		third := fixture.openFloat(t, "1000")
		tickets := fixture.countRows(t, "SELECT count(*) FROM app.kitchen_tickets")
		failed := decodeResponse[struct {
			Closed   bool                  `json:"closed"`
			Printing payments.PrintOutcome `json:"printing"`
		}](t, fixture.closeShift(t, fixture.cashier, third, map[string]any{"print": true}, 200))
		if _, err := fixture.pool.Exec(fixture.ctx, `DROP TRIGGER test_fail_cash_document ON app.cash_documents;
            DROP FUNCTION app.test_fail_cash_document()`); err != nil {
			t.Fatal(err)
		}
		if !failed.Closed || failed.Printing.Status != "failed" || !strings.Contains(failed.Printing.Message, "quedó registrado") {
			t.Fatalf("La falla al encolar no se informó sin revertir el cierre: %+v", failed.Printing)
		}
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT state FROM app.cash_shifts WHERE id=$1", third).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != "closed" || fixture.countRows(t, "SELECT count(*) FROM app.kitchen_tickets") != tickets ||
			fixture.countRows(t, "SELECT count(*) FROM app.cash_documents WHERE shift_id=$1", third) != 0 {
			t.Fatal("La falla de impresión dejó registros a medias o revirtió el cierre")
		}
	})
}
