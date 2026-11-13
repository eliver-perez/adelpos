//go:build integration

package operations_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa 9B, fase A, encadenada al final de TestRestaurantAcceptance: la impresión real usa las
// plantillas. Cada documento guarda su versión; la reimpresión de una comanda, precuenta, ticket,
// comprobante y corte emitidos antes de editar la plantilla es idéntica al original; imprimir con la
// plantilla vigente es otra acción con su permiso y auditoría, que nunca crea ventas, pagos ni
// documentos comerciales o de caja; el esperado sigue la regla de caja (permiso y fuera del arqueo)
// y la vista previa con un documento real aplica los permisos de su origen. Ninguna prueba imprime:
// la cola usa el adaptador controlado y los destinos son colas de prueba.

type stage9BFixture struct {
	*paymentsFixture
	registerID string
	cashier    testTerminal
	limited    testTerminal
	noCurrent  testTerminal
	shiftID    string
	tickets    kitchen.Printer
	product    catalog.Product
}

// stage9BJob es lo que la cola guardó de un trabajo.
type stage9BJob struct {
	content    string
	layout     *string
	ticketID   string
	ticketKind string
	versionID  *string
}

func (fixture *stage9BFixture) job(t *testing.T, jobID string) stage9BJob {
	t.Helper()
	var job stage9BJob
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT j.content,j.layout::text,t.id,t.kind,t.template_version_id::text
        FROM app.kitchen_print_jobs j JOIN app.kitchen_tickets t ON t.id=j.ticket_id WHERE j.id=$1`, jobID).
		Scan(&job.content, &job.layout, &job.ticketID, &job.ticketKind, &job.versionID); err != nil {
		t.Fatal(err)
	}
	return job
}

// settle entrega los trabajos indicados al adaptador controlado como aceptados: los demás
// pendientes se cancelan para que el trabajador tome exactamente esos.
func (fixture *stage9BFixture) settle(t *testing.T, jobIDs ...string) {
	t.Helper()
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.kitchen_print_jobs SET state='cancelled',lease_until=NULL
        WHERE state IN ('pending','error','processing') AND NOT id::text=ANY($1)`, jobIDs); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingTransport{state: "submitted"}
	for range jobIDs {
		if worked, err := (&kitchen.Worker{Pool: fixture.pool, Transport: adapter}).PrintNext(fixture.ctx); err != nil || !worked {
			t.Fatalf("El trabajador no tomó los trabajos %v: %v", jobIDs, err)
		}
	}
}

// writeCounts resume los registros que nunca deben cambiar al reimprimir, imprimir con la plantilla
// vigente o imprimir muestras.
func (fixture *stage9BFixture) writeCounts(t *testing.T) string {
	t.Helper()
	values := []int{}
	for _, table := range []string{"restaurant_accounts", "payments", "payment_documents", "cash_documents", "cash_operations",
		"cash_movements", "cash_shift_reports", "kitchen_unit_releases"} {
		values = append(values, fixture.countRows(t, "SELECT count(*) FROM app."+table))
	}
	return fmt.Sprint(values)
}

// customize crea una plantilla a partir de la del sistema del tipo, le aplica edit, la guarda y la
// activa. Devuelve la versión vigente.
func (fixture *stage9BFixture) customize(t *testing.T, kind documents.Kind, name string, edit func(blocks []any) []any) string {
	t.Helper()
	type detail struct {
		ID             string          `json:"id"`
		Revision       int             `json:"revision"`
		Layout         json.RawMessage `json:"layout"`
		CurrentVersion struct {
			ID string `json:"id"`
		} `json:"current_version"`
	}
	created := decodeResponse[detail](t, fixture.mutate(t, "POST", "documents/templates", map[string]any{
		"kind": kind, "name": name, "source_version_id": documents.SystemVersionID(kind),
	}, 200))
	var template map[string]any
	if err := json.Unmarshal(created.Layout, &template); err != nil {
		t.Fatal(err)
	}
	template["blocks"] = edit(template["blocks"].([]any))
	saved := decodeResponse[detail](t, fixture.mutate(t, "PUT", "documents/templates/"+created.ID, map[string]any{
		"revision": created.Revision, "layout": template, "note": "Etapa 9B",
	}, 200))
	activated := decodeResponse[detail](t, fixture.mutate(t, "POST", "documents/templates/"+created.ID+"/activate", map[string]any{
		"revision": saved.Revision,
	}, 200))
	return activated.CurrentVersion.ID
}

// setTitle cambia el título del documento; boldName imprime el nombre siempre y en negrita.
func setTitle(title string, boldName bool) func(blocks []any) []any {
	return func(blocks []any) []any {
		for _, entry := range blocks {
			block := entry.(map[string]any)
			if block["id"] == "title" {
				block["options"] = map[string]any{"text": title}
			}
			if boldName && block["id"] == "name" {
				block["bold"] = true
				if options, ok := block["options"].(map[string]any); ok && options["when"] != nil {
					options["when"] = "always"
				}
			}
		}
		return blocks
	}
}

// addFooter agrega un texto fijo antes del aviso fiscal.
func addFooter(text string) func(blocks []any) []any {
	return func(blocks []any) []any {
		footer := map[string]any{"id": "gracias", "type": "fixed_text", "align": "center", "options": map[string]any{"text": text}}
		result := append([]any{}, blocks[:len(blocks)-1]...)
		return append(append(result, footer), blocks[len(blocks)-1])
	}
}

func testStage9BDocuments(t *testing.T, base *restaurantFixture) {
	fixture := &stage9BFixture{paymentsFixture: &paymentsFixture{restaurantFixture: base, methods: map[string]payments.Method{}, registers: map[string]string{}}}
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil {
			fixture.methods[*method.Code] = method
		}
	}
	fixture.registerID = decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": "Caja de documentos", "description": "", "active": true}, 200)).ID
	documentsPermissions := []string{documents.PermissionTemplatesRead, documents.PermissionPreviewReal, documents.PermissionRenderCurrent}
	fixture.cashier = newCashier(t, fixture.paymentsFixture, "cajero_documentos", append(append(append([]string{}, cashierPermissions...),
		"cash.reports.print", "cash.reports.reprint", "cash.expected.read", "cash.history.read", "payments.tickets.print",
		"payments.tickets.reprint", "events.read"), documentsPermissions...), fixture.registerID)
	fixture.limited = newCashier(t, fixture.paymentsFixture, "cajero_documentos_sin_esperado", append(append(append([]string{}, cashierPermissions...),
		"cash.reports.print", "cash.reports.reprint", "payments.tickets.reprint"), documentsPermissions...), fixture.registerID)
	fixture.noCurrent = newCashier(t, fixture.paymentsFixture, "reimpresion_exacta", []string{
		"accounts.read", "payments.read", "payments.tickets.reprint", "cash.shifts.read", "cash.reports.reprint", "cash.expected.read",
		"kitchen.reprint", "kitchen.stations.all", "kitchen.queue.read",
	}, fixture.registerID)
	fixture.shiftID = decodeResponse[payments.Shift](t, fixture.call(t, fixture.cashier, "POST", "payments/shifts", map[string]any{
		"operation_id": randomID(), "opening_float": "500",
	}, 200)).ID
	fixture.tickets = decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name: "Tickets de documentos 9B", Transport: "cups", Destination: "ADEL_TEST_9B_TICKETS", Active: true, PaperWidth: 58, Copies: 1,
	}, 200))
	settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
	fixture.adminCall(t, "PUT", "payments/settings", map[string]any{
		"operation_id": randomID(), "revision": settings.Revision, "shared_shifts": true, "print_receipt_default": false,
		"receipt_printer_id": fixture.tickets.ID, "refund_authorization_threshold": settings.RefundAuthorizationThreshold,
	}, 200)
	testStage9BKitchen(t, fixture)
	testStage9BCommercial(t, fixture)
	testStage9BCash(t, fixture)
	// Fase B2: logotipo del restaurante en una comanda real (documents_logo_9b_test.go).
	testStage9BLogo(t, fixture)
	t.Run("9B_restaurar_predeterminadas", func(t *testing.T) {
		for _, kind := range []documents.Kind{documents.KindKitchenOrder, documents.KindReceipt, documents.KindPrecheck,
			documents.KindCashIncome, documents.KindCashPartialCut} {
			fixture.mutate(t, "POST", "documents/restore-default", map[string]any{"kind": kind}, 200)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.document_templates WHERE active AND NOT system") != 0 {
			t.Fatal("Restaurar no dejó activas las plantillas del sistema")
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='cancelled' WHERE state IN ('pending','error','uncertain')"); err != nil {
			t.Fatal(err)
		}
	})
	// Etapa 9C (§5.3): columnas por impresora con las plantillas del sistema (printer_columns_9c_test.go).
	testStage9CPrinterColumns(t, fixture)
}

func testStage9BKitchen(t *testing.T, fixture *stage9BFixture) {
	var original kitchen.PrintJob
	var originalDocument kitchen.Document
	t.Run("9B_comanda_guarda_su_version_e_identidad", func(t *testing.T) {
		printer := decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
			Name: "Comandas 9B", Transport: "cups", Destination: "ADEL_TEST_9B_KITCHEN", Active: true, PaperWidth: 80, Copies: 1,
		}, 200))
		station := decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
			Name: "Estación de documentos", Active: true, Reception: "printer", PrinterID: &printer.ID, TargetMinutes: 10, WarningMinutes: 12,
			OverdueMinutes: 20, Grouping: "inherit", Tracking: "inherit",
		}, 200))
		category := decodeResponse[catalog.Category](t, fixture.adminCall(t, "POST", "catalog/categories", catalog.Category{
			Metadata: catalog.Metadata{Name: "Platillos 9B", Active: true}}, 200))
		fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &category.ID, Mode: "prepare", StationIDs: []string{station.ID}}, 200)
		fixture.product = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
			Metadata: catalog.Metadata{Name: "Platillo de documentos", Active: true}, CategoryID: category.ID, Code: "DOC9B", Kind: "product", BasePrice: "80",
		}, 200))
		original = fixture.releasedJob(t, "Comanda 9B antes")
		job := fixture.job(t, original.ID)
		if job.ticketKind != "normal" || job.versionID == nil || *job.versionID != documents.SystemVersionID(documents.KindKitchenOrder) || job.layout != nil {
			t.Fatalf("La comanda no guardó la versión del sistema o su trabajo no es de texto plano: %+v", job)
		}
		originalDocument = decodeResponse[kitchen.Document](t, fixture.ticketDocument(t, job.ticketID))
		if originalDocument.TemplateVersionID == nil || originalDocument.Identity == nil || originalDocument.Identity.Name == "" {
			t.Fatal("La comanda no copió su versión de plantilla ni la identidad")
		}
		layout, err := kitchen.RenderDocument(&documents.Version{Kind: documents.KindKitchenOrder, Template: documents.Default(documents.KindKitchenOrder)},
			originalDocument, 80)
		if err != nil || layout.Text() != job.content || !strings.Contains(job.content, "COMANDA") {
			t.Fatalf("El trabajo de la comanda no es el texto de la plantilla predeterminada:\n%s", job.content)
		}
		fixture.settle(t, original.ID)
	})
	t.Run("9B_reimpresion_identica_y_plantilla_vigente_como_accion_distinta", func(t *testing.T) {
		versionID := fixture.customize(t, documents.KindKitchenOrder, "Comanda con título", setTitle("COMANDA 9B", true))
		before := fixture.writeCounts(t)
		copied := decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{
			"job_id": original.ID, "reason": "Copia tras editar la plantilla",
		}, 200))
		copy := fixture.job(t, copied.ID)
		if copy.content != fixture.job(t, original.ID).content || copy.layout != nil || strings.Contains(copy.content, "COMANDA 9B") {
			t.Fatalf("La reimpresión no es idéntica a la comanda original:\n%s", copy.content)
		}
		fixture.settle(t, copied.ID)
		// Sin documents.history.render_current no se imprime con la plantilla vigente.
		fixture.call(t, fixture.noCurrent, "POST", "kitchen/print-jobs/rerender", map[string]any{
			"operation_id": randomID(), "job_id": original.ID, "reason": "Sin permiso propio",
		}, 403)
		result := decodeResponse[struct {
			Job               kitchen.PrintJob `json:"job"`
			TicketID          string           `json:"ticket_id"`
			TemplateVersionID string           `json:"template_version_id"`
		}](t, fixture.mutate(t, "POST", "kitchen/print-jobs/rerender", map[string]any{
			"job_id": original.ID, "reason": "Ver la comanda con la plantilla nueva",
		}, 200))
		rerendered := fixture.job(t, result.Job.ID)
		if rerendered.ticketKind != documents.TicketKindRerender || result.TemplateVersionID != versionID || rerendered.versionID == nil ||
			*rerendered.versionID != versionID || !strings.Contains(rerendered.content, "COMANDA 9B") || rerendered.layout == nil {
			t.Fatalf("La impresión con la plantilla vigente no usó la versión activa con atributos: %+v", rerendered)
		}
		if fixture.writeCounts(t) != before || fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='print.rerendered'") != 1 {
			t.Fatal("La impresión con la plantilla vigente creó registros operativos o no se auditó")
		}
		fixture.mutate(t, "POST", "kitchen/print-jobs/rerender", map[string]any{"job_id": original.ID, "reason": "Otra copia pendiente"}, 409)
		// Revisión de la 9B (hallazgo bajo 4): con la impresión con la plantilla vigente sin resolver
		// tampoco se acepta una reimpresión exacta del mismo documento.
		blocked := fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{"job_id": original.ID, "reason": "Copia con otra pendiente"}, 409)
		if errorCode(t, blocked) != "PRINT_COPY_PENDING" {
			t.Fatal("La reimpresión exacta no se bloqueó con una impresión con la plantilla vigente pendiente")
		}
		listed := string(fixture.adminCall(t, "GET", "kitchen/tickets", nil, 200))
		if strings.Contains(listed, result.TicketID) {
			t.Fatal("La impresión con la plantilla vigente apareció entre las comandas")
		}
		fixture.settle(t, result.Job.ID)
		// Una copia desde la cola de la impresión con la plantilla vigente es de la misma familia:
		// mientras siga sin resolver, el original no admite otra copia exacta ni con la plantilla vigente.
		rerenderCopy := decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{
			"job_id": result.Job.ID, "reason": "Copia de la impresión con la plantilla vigente",
		}, 200))
		for _, route := range []string{"kitchen/reprint", "kitchen/print-jobs/rerender"} {
			blocked := fixture.mutate(t, "POST", route, map[string]any{"job_id": original.ID, "reason": "Otra copia del mismo documento"}, 409)
			if errorCode(t, blocked) != "PRINT_COPY_PENDING" {
				t.Fatalf("%s no se bloqueó con una copia de la impresión con la plantilla vigente pendiente", route)
			}
		}
		fixture.settle(t, rerenderCopy.ID)
		// Una comanda nueva usa la versión activa; la prueba técnica sigue con el formato de siempre.
		fresh := fixture.job(t, fixture.releasedJob(t, "Comanda 9B después").ID)
		if fresh.versionID == nil || *fresh.versionID != versionID || !strings.Contains(fresh.content, "COMANDA 9B") || fresh.layout == nil {
			t.Fatalf("La comanda nueva no usó la plantilla activa: %+v", fresh)
		}
		test := decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/test-print", map[string]string{"printer_id": fixture.tickets.ID}, 200))
		testJob := fixture.job(t, test.ID)
		if !strings.Contains(testJob.content, "PRUEBA DE IMPRESIÓN") || strings.Contains(testJob.content, "COMANDA 9B") || testJob.layout != nil || testJob.versionID != nil {
			t.Fatalf("La prueba técnica cambió con la plantilla de la comanda:\n%s", testJob.content)
		}
	})
	t.Run("9B_vista_previa_con_comanda_real", func(t *testing.T) {
		job := fixture.job(t, original.ID)
		preview := decodeResponse[struct {
			Sample bool   `json:"sample"`
			Text   string `json:"text"`
		}](t, fixture.adminCall(t, "POST", "documents/preview", map[string]any{
			"kind": "kitchen_order", "layout": json.RawMessage(documents.Default(documents.KindKitchenOrder).Encode()), "paper_width": 80,
			"source": map[string]any{"ticket_id": job.ticketID},
		}, 200))
		if preview.Sample || preview.Text != job.content {
			t.Fatalf("La vista previa con la comanda real no reproduce su texto:\n%s", preview.Text)
		}
		mismatch := fixture.adminCall(t, "POST", "documents/preview", map[string]any{
			"kind": "kitchen_cancellation", "layout": json.RawMessage(documents.Default(documents.KindKitchenCancellation).Encode()), "paper_width": 80,
			"source": map[string]any{"ticket_id": job.ticketID},
		}, 409)
		if errorCode(t, mismatch) != "SOURCE_KIND_MISMATCH" {
			t.Fatal("La vista previa aceptó un documento de otro tipo")
		}
	})
}

// releasedJob captura una cuenta con el producto de la estación de documentos y devuelve el trabajo
// de su comanda.
func (fixture *stage9BFixture) releasedJob(t *testing.T, name string) kitchen.PrintJob {
	t.Helper()
	bill := fixture.capture(t, newAccount(t, fixture.restaurantFixture, name), []catalog.LineRequest{{ProductID: fixture.product.ID, Quantity: 1}})
	for _, release := range kitchenReleases(t, fixture.restaurantFixture, bill) {
		if release.State != "released" {
			fixture.mutate(t, "POST", "kitchen/releases/"+release.ID+"/release", map[string]any{"revision": release.Revision}, 200)
		}
	}
	var job kitchen.PrintJob
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT j.id FROM app.kitchen_print_jobs j JOIN app.kitchen_tickets t ON t.id=j.ticket_id
        JOIN app.kitchen_releases r ON r.id=t.release_id WHERE r.account_id=$1 AND t.kind='normal' AND j.original_job_id IS NULL`,
		bill.Account.ID).Scan(&job.ID); err != nil {
		t.Fatalf("La cuenta %s no generó comanda: %v", name, err)
	}
	return job
}

func (fixture *stage9BFixture) ticketDocument(t *testing.T, ticketID string) []byte {
	t.Helper()
	var document []byte
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT document FROM app.kitchen_tickets WHERE id=$1", ticketID).Scan(&document); err != nil {
		t.Fatal(err)
	}
	return document
}

func testStage9BCommercial(t *testing.T, fixture *stage9BFixture) {
	var bill operations.Bill
	var receipt, precheck printingResult
	t.Run("9B_precuenta_y_ticket_con_la_version_activa", func(t *testing.T) {
		bill = fixture.capture(t, newAccount(t, fixture.restaurantFixture, "Documentos comerciales 9B"), []catalog.LineRequest{{ProductID: fixture.product.ID, Quantity: 2}})
		precheck = decodeResponse[printingResult](t, fixture.call(t, fixture.cashier, "POST", "payments/accounts/"+bill.Account.ID+"/precheck/print",
			map[string]any{"operation_id": randomID()}, 200))
		paid := decodeResponse[struct {
			Printing printingResult `json:"printing"`
		}](t, fixture.pay(t, fixture.cashier, bill.Account.ID, map[string]any{
			"method_id": fixture.methods["cash"].ID, "mode": "total", "received": "200", "print": true,
		}, 200))
		receipt = paid.Printing
		for _, item := range []struct {
			result printingResult
			kind   documents.Kind
			title  string
		}{{precheck, documents.KindPrecheck, "PRECUENTA"}, {receipt, documents.KindReceipt, "TICKET DE PAGO"}} {
			job := fixture.job(t, item.result.JobID)
			if job.versionID == nil || *job.versionID != documents.SystemVersionID(item.kind) || job.layout != nil ||
				!strings.Contains(job.content, item.title) || !strings.Contains(job.content, "CFDI") {
				t.Fatalf("El documento %s no usó la versión del sistema en texto plano: %+v", item.kind, job)
			}
			for _, line := range strings.Split(job.content, "\n") {
				if len([]rune(line)) > 30 {
					t.Fatalf("El documento de 58 mm excede 30 columnas: %q", line)
				}
			}
		}
		fixture.settle(t, precheck.JobID, receipt.JobID)
	})
	t.Run("9B_reimpresion_exacta_y_plantilla_vigente_de_tickets", func(t *testing.T) {
		fixture.customize(t, documents.KindReceipt, "Ticket con agradecimiento", addFooter("GRACIAS 9B"))
		precheckVersion := fixture.customize(t, documents.KindPrecheck, "Precuenta con aviso", addFooter("PRECUENTA 9B"))
		before := fixture.writeCounts(t)
		copied := decodeResponse[printingResult](t, fixture.call(t, fixture.cashier, "POST", "payments/accounts/"+bill.Account.ID+"/documents/reprint",
			map[string]any{"operation_id": randomID(), "ticket_id": receipt.TicketID, "reason": "Copia para el cliente"}, 200))
		if fixture.job(t, copied.JobID).content != fixture.job(t, receipt.JobID).content {
			t.Fatal("La reimpresión del ticket no es idéntica al original")
		}
		precheckCopy := decodeResponse[printingResult](t, fixture.call(t, fixture.cashier, "POST", "payments/accounts/"+bill.Account.ID+"/documents/reprint",
			map[string]any{"operation_id": randomID(), "ticket_id": precheck.TicketID, "reason": "Copia para el cliente"}, 200))
		if copy := fixture.job(t, precheckCopy.JobID); copy.content != fixture.job(t, precheck.JobID).content || strings.Contains(copy.content, "PRECUENTA 9B") {
			t.Fatal("La reimpresión de la precuenta no es idéntica al original")
		}
		// Revisión de la 9B (hallazgo bajo 4): con la copia exacta sin resolver no se imprime con la
		// plantilla vigente.
		blocked := fixture.call(t, fixture.cashier, "POST", "payments/accounts/"+bill.Account.ID+"/documents/rerender",
			map[string]any{"operation_id": randomID(), "ticket_id": precheck.TicketID, "reason": "Copia con la plantilla vigente"}, 409)
		if errorCode(t, blocked) != "PRINT_COPY_PENDING" {
			t.Fatal("La impresión con la plantilla vigente no se bloqueó con una copia exacta pendiente")
		}
		fixture.settle(t, copied.JobID, precheckCopy.JobID)
		// Revisión 9F (B2): otra copia en papel de un documento ya emitido pide motivo, como reimprimir.
		fixture.call(t, fixture.cashier, "POST", "payments/accounts/"+bill.Account.ID+"/documents/rerender",
			map[string]any{"operation_id": randomID(), "ticket_id": precheck.TicketID}, 400)
		fixture.call(t, fixture.cashier, "POST", "payments/accounts/"+bill.Account.ID+"/documents/rerender",
			map[string]any{"operation_id": randomID(), "ticket_id": precheck.TicketID, "reason": "otra"}, 400)
		fixture.call(t, fixture.noCurrent, "POST", "payments/accounts/"+bill.Account.ID+"/documents/rerender",
			map[string]any{"operation_id": randomID(), "ticket_id": precheck.TicketID, "reason": "Copia con la plantilla vigente"}, 403)
		result := decodeResponse[struct {
			Status            string `json:"status"`
			JobID             string `json:"job_id"`
			TicketID          string `json:"ticket_id"`
			TemplateVersionID string `json:"template_version_id"`
		}](t, fixture.call(t, fixture.cashier, "POST", "payments/accounts/"+bill.Account.ID+"/documents/rerender",
			map[string]any{"operation_id": randomID(), "ticket_id": precheck.TicketID, "reason": "Copia con la plantilla vigente"}, 200))
		rerendered := fixture.job(t, result.JobID)
		if result.TemplateVersionID != precheckVersion || rerendered.ticketKind != documents.TicketKindRerender ||
			!strings.Contains(rerendered.content, "PRECUENTA 9B") || !strings.Contains(rerendered.content, "PRECUENTA") {
			t.Fatalf("La precuenta no se imprimió con la plantilla vigente: %+v", rerendered)
		}
		if fixture.writeCounts(t) != before || fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='payments.document_rerendered'") != 1 {
			t.Fatal("Imprimir con la plantilla vigente creó pagos o documentos comerciales, o no se auditó")
		}
		var auditReason, historyReason string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT after_state->>'reason' FROM app.audit_events
            WHERE action='payments.document_rerendered' ORDER BY occurred_at DESC LIMIT 1`).Scan(&auditReason); err != nil {
			t.Fatal(err)
		}
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT after_value->>'reason' FROM app.kitchen_history
            WHERE kind='print.rerendered' AND entity_id=$1`, result.JobID).Scan(&historyReason); err != nil {
			t.Fatal(err)
		}
		if auditReason != "Copia con la plantilla vigente" || historyReason != auditReason {
			t.Fatalf("El motivo de la impresión con la plantilla vigente no quedó registrado: %q %q", auditReason, historyReason)
		}
		// Y con la impresión con la plantilla vigente sin resolver no se acepta la copia exacta.
		blocked = fixture.call(t, fixture.cashier, "POST", "payments/accounts/"+bill.Account.ID+"/documents/reprint",
			map[string]any{"operation_id": randomID(), "ticket_id": precheck.TicketID, "reason": "Copia para el cliente"}, 409)
		if errorCode(t, blocked) != "PRINT_COPY_PENDING" {
			t.Fatal("La copia exacta no se bloqueó con una impresión con la plantilla vigente pendiente")
		}
		// La vista previa de la precuenta usa la plantilla activa sin registrar nada.
		document := decodeResponse[payments.CommercialDocument](t, fixture.call(t, fixture.cashier, "GET", "payments/accounts/"+bill.Account.ID+"/precheck", nil, 200))
		if !strings.Contains(document.Content, "PRECUENTA 9B") || document.TemplateVersionID == nil || *document.TemplateVersionID != precheckVersion {
			t.Fatalf("La vista previa de la precuenta no usó la plantilla activa:\n%s", document.Content)
		}
		recent := string(fixture.call(t, fixture.cashier, "GET", "documents/recent?kind=receipt", nil, 200))
		if !strings.Contains(recent, receipt.TicketID) {
			t.Fatal("Los documentos recientes no incluyen el ticket emitido")
		}
		preview := decodeResponse[struct {
			Text string `json:"text"`
		}](t, fixture.call(t, fixture.cashier, "POST", "documents/preview", map[string]any{
			"kind": "receipt", "layout": json.RawMessage(documents.Default(documents.KindReceipt).Encode()), "paper_width": 58,
			"source": map[string]any{"ticket_id": receipt.TicketID},
		}, 200))
		if preview.Text != fixture.job(t, receipt.JobID).content {
			t.Fatalf("La vista previa del ticket real no reproduce su texto:\n%s", preview.Text)
		}
		fixture.call(t, fixture.noCurrent, "GET", "documents/recent?kind=receipt", nil, 403)
		fixture.settle(t, result.JobID)
	})
}

func testStage9BCash(t *testing.T, fixture *stage9BFixture) {
	var voucher, hiddenCut, visibleCut payments.PrintOutcome
	t.Run("9B_comprobante_y_cortes_guardan_datos_estructurados", func(t *testing.T) {
		recorded := decodeResponse[recordedOperation](t, fixture.call(t, fixture.cashier, "POST", "payments/shifts/"+fixture.shiftID+"/income",
			map[string]any{"operation_id": randomID(), "type": "float_contribution", "amount": "100", "reference": "Sobre 9B", "print": true}, 200))
		voucher = *recorded.Printing
		hidden := decodeResponse[payments.CreatedReport](t, fixture.call(t, fixture.limited, "POST", "payments/shifts/"+fixture.shiftID+"/partial-cuts",
			map[string]any{"operation_id": randomID(), "print": true}, 200))
		visible := decodeResponse[payments.CreatedReport](t, fixture.call(t, fixture.cashier, "POST", "payments/shifts/"+fixture.shiftID+"/partial-cuts",
			map[string]any{"operation_id": randomID(), "print": true}, 200))
		hiddenCut, visibleCut = *hidden.Printing, *visible.Printing
		if hiddenCut.IncludesExpected || !visibleCut.IncludesExpected {
			t.Fatal("Los cortes no siguieron el permiso de consultar el esperado")
		}
		for _, item := range []struct {
			outcome payments.PrintOutcome
			kind    documents.Kind
		}{{voucher, documents.KindCashIncome}, {hiddenCut, documents.KindCashPartialCut}, {visibleCut, documents.KindCashPartialCut}} {
			job := fixture.job(t, item.outcome.JobID)
			if job.versionID == nil || *job.versionID != documents.SystemVersionID(item.kind) || job.layout != nil {
				t.Fatalf("El documento de caja no guardó la versión del sistema: %+v", job)
			}
		}
		fixture.settle(t, voucher.JobID, hiddenCut.JobID, visibleCut.JobID)
		var withData, sensitiveHidden, sensitiveVisible bool
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT (SELECT document ? 'data' FROM app.kitchen_tickets WHERE id=$1),
            (SELECT document->'data'->'cut' ? 'sensitive' FROM app.kitchen_tickets WHERE id=$2),
            (SELECT document->'data'->'cut' ? 'sensitive' FROM app.kitchen_tickets WHERE id=$3)`,
			voucher.TicketID, hiddenCut.TicketID, visibleCut.TicketID).Scan(&withData, &sensitiveHidden, &sensitiveVisible); err != nil {
			t.Fatal(err)
		}
		if !withData || sensitiveHidden || !sensitiveVisible {
			t.Fatalf("Los datos estructurados no respetan el esperado: datos %v, oculto %v, visible %v", withData, sensitiveHidden, sensitiveVisible)
		}
	})
	t.Run("9B_reimpresion_exacta_y_plantilla_vigente_de_caja", func(t *testing.T) {
		fixture.customize(t, documents.KindCashIncome, "Ingreso con título", setTitle("INGRESO 9B", false))
		fixture.customize(t, documents.KindCashPartialCut, "Corte con título", setTitle("CORTE 9B", false))
		before := fixture.writeCounts(t)
		copied := decodeResponse[payments.PrintOutcome](t, fixture.call(t, fixture.cashier, "POST", "payments/cash-documents/"+voucher.DocumentID+"/reprint",
			map[string]any{"operation_id": randomID(), "reason": "Copia para el encargado"}, 200))
		if fixture.job(t, copied.JobID).content != fixture.job(t, voucher.JobID).content {
			t.Fatal("La reimpresión del comprobante no es idéntica al original")
		}
		cutCopy := decodeResponse[payments.PrintOutcome](t, fixture.call(t, fixture.cashier, "POST", "payments/cash-documents/"+visibleCut.DocumentID+"/reprint",
			map[string]any{"operation_id": randomID(), "reason": "Copia para el encargado"}, 200))
		if fixture.job(t, cutCopy.JobID).content != fixture.job(t, visibleCut.JobID).content {
			t.Fatal("La reimpresión del corte no es idéntica al original")
		}
		// Revisión de la 9B (hallazgo bajo 4): con la copia exacta sin resolver no se imprime con la
		// plantilla vigente; al resolverla, sí.
		blocked := fixture.call(t, fixture.cashier, "POST", "payments/cash-documents/"+voucher.DocumentID+"/rerender",
			map[string]any{"operation_id": randomID(), "reason": "Copia con la plantilla vigente"}, 409)
		if errorCode(t, blocked) != "PRINT_COPY_PENDING" {
			t.Fatal("La impresión con la plantilla vigente de caja no se bloqueó con una copia exacta pendiente")
		}
		fixture.settle(t, copied.JobID, cutCopy.JobID)
		fixture.call(t, fixture.cashier, "POST", "payments/cash-documents/"+voucher.DocumentID+"/rerender", map[string]any{"operation_id": randomID()}, 400)
		fixture.call(t, fixture.noCurrent, "POST", "payments/cash-documents/"+voucher.DocumentID+"/rerender", map[string]any{"operation_id": randomID(), "reason": "Copia con la plantilla vigente"}, 403)
		rerendered := decodeResponse[payments.PrintOutcome](t, fixture.call(t, fixture.cashier, "POST", "payments/cash-documents/"+voucher.DocumentID+"/rerender",
			map[string]any{"operation_id": randomID(), "reason": "Copia con la plantilla vigente"}, 200))
		if job := fixture.job(t, rerendered.JobID); job.ticketKind != documents.TicketKindRerender || !strings.Contains(job.content, "INGRESO 9B") ||
			!strings.Contains(job.content, "Sobre 9B") {
			t.Fatalf("El comprobante no se imprimió con la plantilla vigente:\n%s", job.content)
		}
		blocked = fixture.call(t, fixture.cashier, "POST", "payments/cash-documents/"+voucher.DocumentID+"/reprint",
			map[string]any{"operation_id": randomID(), "reason": "Copia para el encargado"}, 409)
		if errorCode(t, blocked) != "PRINT_COPY_PENDING" {
			t.Fatal("La copia exacta de caja no se bloqueó con una impresión con la plantilla vigente pendiente")
		}
		cut := decodeResponse[payments.PrintOutcome](t, fixture.call(t, fixture.cashier, "POST", "payments/cash-documents/"+visibleCut.DocumentID+"/rerender",
			map[string]any{"operation_id": randomID(), "reason": "Copia con la plantilla vigente"}, 200))
		cutJob := fixture.job(t, cut.JobID)
		if !cut.IncludesExpected || !strings.Contains(cutJob.content, "CORTE 9B #") || !strings.Contains(cutJob.content, "EFECTIVO ESPERADO") {
			t.Fatalf("El corte con la plantilla vigente perdió el esperado permitido:\n%s", cutJob.content)
		}
		// Sin cash.expected.read no se imprime un documento que contiene el esperado.
		fixture.call(t, fixture.limited, "POST", "payments/cash-documents/"+visibleCut.DocumentID+"/rerender", map[string]any{"operation_id": randomID(), "reason": "Copia con la plantilla vigente"}, 403)
		if fixture.writeCounts(t) != before || fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='cash.document_rerendered'") != 2 {
			t.Fatal("Imprimir con la plantilla vigente creó documentos de caja o movimientos, o no se auditó")
		}
		// La cola aplica las reglas de caja a los documentos impresos con la plantilla vigente.
		fixture.call(t, fixture.limited, "GET", "kitchen/tickets/"+cutJob.ticketID, nil, 403)
		fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{"job_id": cut.JobID, "reason": "No se copia desde la cola"}, 400)
		// Las copias de esta prueba no se envían: quedan canceladas sin transporte.
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='cancelled' WHERE state='pending'"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("9B_esperado_ausente_sin_permiso_y_durante_el_arqueo", func(t *testing.T) {
		layout := json.RawMessage(documents.Default(documents.KindCashPartialCut).Encode())
		previewOf := func(client testTerminal, ticketID string) (bool, string) {
			result := decodeResponse[struct {
				IncludesExpected bool   `json:"includes_expected"`
				Text             string `json:"text"`
			}](t, fixture.call(t, client, "POST", "documents/preview", map[string]any{
				"kind": "cash_partial_cut", "layout": layout, "paper_width": 80, "source": map[string]any{"ticket_id": ticketID},
			}, 200))
			return result.IncludesExpected, result.Text
		}
		preview := func(client testTerminal) (bool, string) {
			return previewOf(client, visibleCut.TicketID)
		}
		if include, text := preview(fixture.cashier); !include || !strings.Contains(text, "EFECTIVO ESPERADO") {
			t.Fatalf("Con permiso la vista previa real no mostró el esperado:\n%s", text)
		}
		// Revisión de la 9B (hallazgo bajo 7): la vista previa usa los datos guardados del documento,
		// como «Imprimir con la plantilla vigente». El corte que imprimió alguien sin
		// cash.expected.read no guardó el esperado: tampoco lo muestra a quien sí puede verlo.
		if include, text := previewOf(fixture.cashier, hiddenCut.TicketID); include || strings.Contains(text, "EFECTIVO ESPERADO") ||
			!strings.Contains(text, "no incluidos") {
			t.Fatalf("La vista previa del corte impreso sin el esperado no anticipa lo que se imprime:\n%s", text)
		}
		if include, text := preview(fixture.limited); include || strings.Contains(text, "EFECTIVO ESPERADO") || !strings.Contains(text, "no incluidos") {
			t.Fatalf("Sin permiso la vista previa real mostró el esperado:\n%s", text)
		}
		fixture.call(t, fixture.cashier, "POST", "payments/shifts/"+fixture.shiftID+"/closing", map[string]any{"operation_id": randomID()}, 200)
		if include, text := preview(fixture.cashier); include || strings.Contains(text, "EFECTIVO ESPERADO") {
			t.Fatalf("Durante el arqueo la vista previa real mostró el esperado:\n%s", text)
		}
		closing := fixture.call(t, fixture.cashier, "POST", "payments/cash-documents/"+visibleCut.DocumentID+"/rerender", map[string]any{"operation_id": randomID(), "reason": "Copia con la plantilla vigente"}, 409)
		if errorCode(t, closing) != "SHIFT_CLOSING" {
			t.Fatal("Durante el arqueo se imprimió un corte con el esperado")
		}
		fixture.call(t, fixture.cashier, "POST", "payments/shifts/"+fixture.shiftID+"/closing/cancel", map[string]any{
			"operation_id": randomID(), "reason": "Fin de la prueba de documentos",
		}, 200)
	})
	t.Run("9B_vista_previa_y_recientes_de_caja_con_los_permisos_de_origen", func(t *testing.T) {
		// La vista previa del comprobante reproduce el texto que se imprimió (datos guardados).
		voucherPreview := decodeResponse[struct {
			Text     string   `json:"text"`
			Warnings []string `json:"warnings"`
		}](t, fixture.call(t, fixture.cashier, "POST", "documents/preview", map[string]any{
			"kind": "cash_income", "layout": json.RawMessage(documents.Default(documents.KindCashIncome).Encode()), "paper_width": 58,
			"source": map[string]any{"ticket_id": voucher.TicketID},
		}, 200))
		if voucherPreview.Text != fixture.job(t, voucher.JobID).content || len(voucherPreview.Warnings) != 0 {
			t.Fatalf("La vista previa del comprobante no reproduce lo impreso: %v\n%s", voucherPreview.Warnings, voucherPreview.Text)
		}
		// Revisión de la 9B (hallazgo bajo 8): los comprobantes recientes exigen cash.movements.read
		// (como su vista previa); los cortes, consultar el turno. El alcance de los turnos se aplica.
		documentsOnly := []string{documents.PermissionTemplatesRead, documents.PermissionPreviewReal, "cash.shifts.read"}
		shiftsReader := newCashier(t, fixture.paymentsFixture, "recientes_sin_movimientos", documentsOnly, fixture.registerID)
		denied := fixture.call(t, shiftsReader, "GET", "documents/recent?kind=cash_income", nil, 403)
		if errorCode(t, denied) != "PERMISSION_DENIED" {
			t.Fatal("Los comprobantes recientes se listaron sin cash.movements.read")
		}
		if cuts := string(fixture.call(t, shiftsReader, "GET", "documents/recent?kind=cash_partial_cut", nil, 200)); !strings.Contains(cuts, visibleCut.TicketID) {
			t.Fatal("Los cortes recientes no se listaron con cash.shifts.read")
		}
		recent := decodeResponse[[]documents.RecentDocument](t, fixture.call(t, fixture.cashier, "GET", "documents/recent?kind=cash_income", nil, 200))
		if len(recent) == 0 || recent[0].TicketID != voucher.TicketID || !strings.HasPrefix(recent[0].Label, "Caja de documentos · ") {
			t.Fatalf("Los comprobantes recientes no incluyen el ingreso con su caja: %+v", recent)
		}
		otherRegister := decodeResponse[struct {
			ID string `json:"id"`
		}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": "Caja ajena 9B", "description": "", "active": true}, 200)).ID
		foreign := newCashier(t, fixture.paymentsFixture, "recientes_de_otra_caja",
			append(append([]string{}, documentsOnly...), "cash.movements.read"), otherRegister)
		for _, kind := range []string{"cash_income", "cash_partial_cut"} {
			if listed := decodeResponse[[]documents.RecentDocument](t, fixture.call(t, foreign, "GET", "documents/recent?kind="+kind, nil, 200)); len(listed) != 0 {
				t.Fatalf("Una terminal de otra caja sin cash.shifts.read_all vio los documentos de %s: %+v", kind, listed)
			}
		}
		withAll := newCashier(t, fixture.paymentsFixture, "recientes_de_todas_las_cajas",
			append(append([]string{}, documentsOnly...), "cash.movements.read", "cash.shifts.read_all"), otherRegister)
		if listed := string(fixture.call(t, withAll, "GET", "documents/recent?kind=cash_income", nil, 200)); !strings.Contains(listed, voucher.TicketID) {
			t.Fatal("Con cash.shifts.read_all no se listaron los comprobantes de otra caja")
		}
	})
	t.Run("9B_documento_anterior_solo_admite_reimpresion_exacta", func(t *testing.T) {
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_tickets SET document=document-'data'-'template_version_id' WHERE id=$1", hiddenCut.TicketID); err != nil {
			t.Fatal(err)
		}
		// La vista previa de un documento anterior a la 9B lee el corte actual y lo avisa.
		legacy := decodeResponse[struct {
			Warnings []string `json:"warnings"`
		}](t, fixture.call(t, fixture.cashier, "POST", "documents/preview", map[string]any{
			"kind": "cash_partial_cut", "layout": json.RawMessage(documents.Default(documents.KindCashPartialCut).Encode()), "paper_width": 80,
			"source": map[string]any{"ticket_id": hiddenCut.TicketID},
		}, 200))
		if len(legacy.Warnings) == 0 || !strings.Contains(legacy.Warnings[0], "se emitió antes de las plantillas de documentos") {
			t.Fatalf("La vista previa de un documento anterior no lo avisó: %v", legacy.Warnings)
		}
		unavailable := fixture.call(t, fixture.cashier, "POST", "payments/cash-documents/"+hiddenCut.DocumentID+"/rerender", map[string]any{"operation_id": randomID(), "reason": "Copia con la plantilla vigente"}, 409)
		if errorCode(t, unavailable) != "DOCUMENT_DATA_UNAVAILABLE" {
			t.Fatal("Un documento sin datos estructurados se imprimió con la plantilla vigente")
		}
		copied := decodeResponse[payments.PrintOutcome](t, fixture.call(t, fixture.cashier, "POST", "payments/cash-documents/"+hiddenCut.DocumentID+"/reprint",
			map[string]any{"operation_id": randomID(), "reason": "Copia para el encargado"}, 200))
		if fixture.job(t, copied.JobID).content != fixture.job(t, hiddenCut.JobID).content {
			t.Fatal("La reimpresión exacta del documento anterior cambió")
		}
	})
}
