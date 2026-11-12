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
)

// Etapa 9C (§5.3), encadenada en testStage9BDocuments con las plantillas del sistema restauradas:
// columnas por impresora en Cocina → Impresoras (lista cerrada en Go y en PostgreSQL, auditada),
// comandas renderizadas con las columnas de la impresora destino, reimpresión exacta con la
// fotografía original tras cambiarlas, impresión con la plantilla vigente con la impresora actual,
// documentos comerciales con las columnas de la impresora de tickets y la prueba de ancho por la
// cola, sin ventas ni movimientos. Ninguna prueba imprime: el trabajador usa el adaptador controlado
// y los destinos son colas de prueba.

// widestLine es el renglón más largo de un texto, en caracteres.
func widestLine(content string) int {
	widest := 0
	for _, line := range strings.Split(content, "\n") {
		widest = max(widest, len([]rune(line)))
	}
	return widest
}

// stage9CCounts resume los registros que la prueba de ancho y las copias nunca deben cambiar.
func (fixture *stage9BFixture) stage9CCounts(t *testing.T) string {
	t.Helper()
	values := []int{}
	for _, table := range []string{"restaurant_events", "restaurant_item_units", "sales_recognitions", "inventory_movements",
		"inventory_consumption_requests", "payment_refunds"} {
		values = append(values, fixture.countRows(t, "SELECT count(*) FROM app."+table))
	}
	return fixture.writeCounts(t) + fmt.Sprint(values)
}

func (fixture *stage9BFixture) printerNamed(t *testing.T, name string) kitchen.Printer {
	t.Helper()
	for _, printer := range decodeResponse[[]kitchen.Printer](t, fixture.adminCall(t, "GET", "kitchen/printers", nil, 200)) {
		if printer.Name == name {
			return printer
		}
	}
	t.Fatalf("No existe la impresora %s", name)
	return kitchen.Printer{}
}

// saveColumns guarda la impresora con otras columnas (nil es el predeterminado) y papel.
func (fixture *stage9BFixture) saveColumns(t *testing.T, printer kitchen.Printer, paper int, columns *int, status int) kitchen.Printer {
	t.Helper()
	printer.PaperWidth = paper
	printer.Columns = columns
	body := fixture.mutate(t, "PATCH", "kitchen/printers/"+printer.ID, printer, status)
	if status != 200 {
		return printer
	}
	return decodeResponse[kitchen.Printer](t, body)
}

func (fixture *stage9BFixture) snapshotColumns(t *testing.T, jobID string) string {
	t.Helper()
	var columns string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(printer_snapshot->>'columns','sin dato') FROM app.kitchen_print_jobs WHERE id=$1",
		jobID).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	return columns
}

func testStage9CPrinterColumns(t *testing.T, fixture *stage9BFixture) {
	wide, narrow := 48, 32
	printer := fixture.printerNamed(t, "Comandas 9B")
	var before, after kitchen.PrintJob
	t.Run("9C_columnas_por_impresora_lista_cerrada_en_Go_y_PostgreSQL", func(t *testing.T) {
		if printer.Columns != nil || printer.PaperWidth != 80 {
			t.Fatalf("Una impresora existente usa el predeterminado: %+v", printer)
		}
		// La comanda con el predeterminado es la de siempre.
		before = fixture.releasedJob(t, "Comanda 9C con el predeterminado")
		job := fixture.job(t, before.ID)
		document := decodeResponse[kitchen.Document](t, fixture.ticketDocument(t, job.ticketID))
		legacy, err := kitchen.RenderDocument(&documents.Version{Kind: documents.KindKitchenOrder, Template: documents.Default(documents.KindKitchenOrder)}, document, 80)
		if err != nil || job.content != legacy.Text() || widestLine(job.content) != 42 || fixture.snapshotColumns(t, before.ID) != "sin dato" {
			t.Fatalf("Con el predeterminado la comanda debe ser idéntica a la de la 9B:\n%s", job.content)
		}
		fixture.settle(t, before.ID)
		for _, rejected := range []struct {
			paper   int
			columns int
		}{{80, 30}, {80, 32}, {80, 44}, {58, 48}, {58, 42}} {
			fixture.saveColumns(t, printer, rejected.paper, &rejected.columns, 400)
		}
		problem := fixture.mutate(t, "PATCH", "kitchen/printers/"+printer.ID, func() map[string]any {
			changed := printer
			changed.PaperWidth = 58
			changed.Columns = &wide
			encoded, _ := json.Marshal(changed)
			fields := map[string]any{}
			_ = json.Unmarshal(encoded, &fields)
			return fields
		}(), 400)
		if !strings.Contains(string(problem), "Con papel de 58 mm las columnas de texto son 30 (predeterminado) o 32") {
			t.Fatalf("Cambiar el papel sin ajustar las columnas debe explicarse: %s", problem)
		}
		// Sin kitchen.printers.manage no se cambian (la terminal de reimpresión no lo tiene).
		denied := printer
		denied.Columns = &wide
		fixture.call(t, fixture.noCurrent, "PATCH", "kitchen/printers/"+printer.ID, func() map[string]any {
			encoded, _ := json.Marshal(denied)
			fields := map[string]any{"operation_id": randomID()}
			_ = json.Unmarshal(encoded, &fields)
			return fields
		}(), 403)
		// PostgreSQL también las rechaza (CHECK de la 0020), aunque se escriba sin pasar por Go.
		for _, statement := range []string{
			`UPDATE app.kitchen_printers SET "columns"=44 WHERE id=$1`,
			`UPDATE app.kitchen_printers SET "columns"=32 WHERE id=$1`,
			`UPDATE app.kitchen_printers SET "columns"=48,paper_width=58 WHERE id=$1`,
		} {
			if _, err := fixture.pool.Exec(fixture.ctx, statement, printer.ID); err == nil || !strings.Contains(err.Error(), "kitchen_printers_columns_check") {
				t.Fatalf("PostgreSQL aceptó columnas fuera de la lista (%s): %v", statement, err)
			}
		}
		saved := fixture.saveColumns(t, printer, 80, &wide, 200)
		if saved.Columns == nil || *saved.Columns != wide || saved.Revision != printer.Revision+1 {
			t.Fatalf("No se guardaron 48 columnas: %+v", saved)
		}
		printer = saved
		// Elegir el predeterminado lo guarda como NULL y queda auditado como cualquier cambio.
		reset := fixture.saveColumns(t, printer, 80, func() *int { value := 42; return &value }(), 200)
		if reset.Columns != nil {
			t.Fatalf("El predeterminado se guarda como NULL: %+v", reset)
		}
		printer = fixture.saveColumns(t, reset, 80, &wide, 200)
		if fixture.countRows(t, `SELECT count(*) FROM app.kitchen_history WHERE kind='printer.saved' AND entity_id=$1
            AND after_value->>'columns'='48'`, printer.ID) != 2 ||
			fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='kitchen.printer.saved' AND entity_id=$1", printer.ID) < 3 {
			t.Fatal("El cambio de columnas no quedó auditado como los cambios de impresora")
		}
	})
	t.Run("9C_comanda_con_las_columnas_de_la_impresora_destino_y_reimpresion_exacta", func(t *testing.T) {
		after = fixture.releasedJob(t, "Comanda 9C a 48 columnas")
		wideJob := fixture.job(t, after.ID)
		if widestLine(wideJob.content) != wide || fixture.snapshotColumns(t, after.ID) != "48" || wideJob.layout != nil {
			t.Fatalf("La comanda nueva debe salir a 48 columnas en texto plano:\n%s", wideJob.content)
		}
		document := decodeResponse[kitchen.Document](t, fixture.ticketDocument(t, wideJob.ticketID))
		normal, _ := kitchen.RenderDocument(&documents.Version{Kind: documents.KindKitchenOrder, Template: documents.Default(documents.KindKitchenOrder)}, document, 80)
		if wideJob.content == normal.Text() {
			t.Fatal("La comanda a 48 columnas no recalculó sus renglones")
		}
		fixture.settle(t, after.ID)
		counts := fixture.stage9CCounts(t)
		// La comanda emitida con el predeterminado se reimprime idéntica aunque la impresora ya tenga 48.
		copied := decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{
			"job_id": before.ID, "reason": "Copia tras cambiar las columnas",
		}, 200))
		if fixture.job(t, copied.ID).content != fixture.job(t, before.ID).content {
			t.Fatal("La reimpresión de una comanda anterior al cambio no es idéntica")
		}
		fixture.settle(t, copied.ID)
		// Y la emitida a 48 sale a 48 aunque la impresora vuelva al predeterminado.
		printer = fixture.saveColumns(t, printer, 80, nil, 200)
		copied = decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{
			"job_id": after.ID, "reason": "Copia tras volver al predeterminado",
		}, 200))
		if fixture.job(t, copied.ID).content != wideJob.content || fixture.snapshotColumns(t, copied.ID) != "48" {
			t.Fatal("La reimpresión de la comanda de 48 columnas no conservó su ancho")
		}
		fixture.settle(t, copied.ID)
		// Imprimir con la plantilla vigente usa la impresora actual (ya en el predeterminado).
		result := decodeResponse[struct {
			Job kitchen.PrintJob `json:"job"`
		}](t, fixture.mutate(t, "POST", "kitchen/print-jobs/rerender", map[string]any{
			"job_id": after.ID, "reason": "Con la plantilla y la impresora vigentes",
		}, 200))
		if content := fixture.job(t, result.Job.ID).content; widestLine(content) != 42 {
			t.Fatalf("La plantilla vigente debía usar la impresora actual (42 columnas):\n%s", content)
		}
		fixture.settle(t, result.Job.ID)
		if fixture.stage9CCounts(t) != counts {
			t.Fatal("Las copias crearon ventas, pagos o movimientos")
		}
	})
	t.Run("9C_documentos_comerciales_con_las_columnas_de_la_impresora_de_tickets", func(t *testing.T) {
		tickets := fixture.printerNamed(t, "Tickets de documentos 9B")
		tickets = fixture.saveColumns(t, tickets, 58, &narrow, 200)
		bill := fixture.capture(t, newAccount(t, fixture.restaurantFixture, "Precuenta 9C a 32 columnas"),
			[]catalog.LineRequest{{ProductID: fixture.product.ID, Quantity: 2}})
		precheck := decodeResponse[printingResult](t, fixture.call(t, fixture.cashier, "POST", "payments/accounts/"+bill.Account.ID+"/precheck/print",
			map[string]any{"operation_id": randomID()}, 200))
		original := fixture.job(t, precheck.JobID)
		if widestLine(original.content) != narrow || !strings.Contains(original.content, "PRECUENTA") {
			t.Fatalf("La precuenta debe salir a 32 columnas:\n%s", original.content)
		}
		fixture.settle(t, precheck.JobID)
		tickets = fixture.saveColumns(t, tickets, 58, nil, 200)
		copied := decodeResponse[printingResult](t, fixture.call(t, fixture.cashier, "POST", "payments/accounts/"+bill.Account.ID+"/documents/reprint",
			map[string]any{"operation_id": randomID(), "ticket_id": precheck.TicketID}, 200))
		if fixture.job(t, copied.JobID).content != original.content {
			t.Fatal("La reimpresión de la precuenta no conservó sus 32 columnas")
		}
		fixture.settle(t, copied.JobID)
		fresh := decodeResponse[printingResult](t, fixture.call(t, fixture.cashier, "POST", "payments/accounts/"+bill.Account.ID+"/precheck/print",
			map[string]any{"operation_id": randomID()}, 200))
		if widestLine(fixture.job(t, fresh.JobID).content) != 30 {
			t.Fatal("Con el predeterminado la precuenta vuelve a 30 columnas")
		}
		fixture.settle(t, fresh.JobID)
	})
	t.Run("9C_prueba_de_ancho_por_la_cola_sin_ventas_ni_movimientos", func(t *testing.T) {
		counts := fixture.stage9CCounts(t)
		route := "kitchen/printers/" + printer.ID + "/width-test"
		fixture.call(t, fixture.noCurrent, "POST", route, map[string]any{"operation_id": randomID(), "columns": wide}, 403)
		fixture.mutate(t, "POST", route, map[string]any{"columns": narrow}, 400)
		// Prueba de 48 columnas antes de guardarlas (la impresora sigue en el predeterminado).
		result := decodeResponse[kitchen.WidthTestResult](t, fixture.mutate(t, "POST", route, map[string]any{"columns": wide}, 200))
		job := fixture.job(t, result.Job.ID)
		ruler := strings.Repeat("1234567890", 5)[:wide]
		if job.ticketKind != "test" || result.Columns != wide || result.PaperWidth != 80 || job.layout != nil || job.versionID != nil ||
			!strings.Contains(job.content, documents.WidthTestMarker) || !strings.Contains(job.content, "\n"+ruler+"\n") ||
			!strings.Contains(job.content, "\n"+strings.Repeat("-", wide)+"\n") || !strings.Contains(job.content, "$1,234.50") ||
			widestLine(job.content) != wide || fixture.snapshotColumns(t, result.Job.ID) != "48" {
			t.Fatalf("La prueba de ancho no es la regla de 48 columnas en texto plano: %+v\n%s", result, job.content)
		}
		if saved := fixture.printerNamed(t, "Comandas 9B"); saved.Columns != nil || saved.Revision != printer.Revision {
			t.Fatal("Probar un ancho no debe guardar las columnas de la impresora")
		}
		// Sin columnas indicadas usa las de la impresora.
		current := decodeResponse[kitchen.WidthTestResult](t, fixture.mutate(t, "POST", route, map[string]any{}, 200))
		if current.Columns != 42 || widestLine(fixture.job(t, current.Job.ID).content) != 42 {
			t.Fatalf("La prueba sin columnas debe usar las de la impresora: %+v", current)
		}
		// Va por la cola de siempre y se muestra y se copia con su mismo texto.
		fixture.settle(t, result.Job.ID)
		ticket := decodeResponse[kitchen.Document](t, fixture.adminCall(t, "GET", "kitchen/tickets/"+job.ticketID, nil, 200))
		if ticket.Preview != job.content {
			t.Fatalf("La cola no muestra el texto de la prueba de ancho:\n%s", ticket.Preview)
		}
		copied := decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{
			"job_id": result.Job.ID, "reason": "Otra hoja de la prueba de ancho",
		}, 200))
		if fixture.job(t, copied.ID).content != job.content {
			t.Fatal("La copia de la prueba de ancho no repite su texto")
		}
		fixture.mutate(t, "POST", "kitchen/print-jobs/rerender", map[string]any{"job_id": result.Job.ID, "reason": "No es una comanda"}, 400)
		if fixture.countRows(t, "SELECT count(*) FROM app.kitchen_history WHERE kind='print.width_test_requested'") != 2 ||
			fixture.countRows(t, "SELECT count(*) FROM app.audit_events WHERE action='kitchen.print.width_test_requested'") != 2 {
			t.Fatal("La prueba de ancho no quedó registrada")
		}
		if fixture.stage9CCounts(t) != counts {
			t.Fatal("La prueba de ancho creó ventas, pagos o movimientos")
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='cancelled' WHERE state IN ('pending','error','uncertain')"); err != nil {
			t.Fatal(err)
		}
	})
}
