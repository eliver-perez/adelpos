//go:build integration

package reports_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
	"local/ad-el-pos/server/internal/reports"
)

// Etapa 9C (§5.1) contra PostgreSQL con la API real: «Cuentas anuladas» (desde la etapa 9D
// «Cuentas canceladas»: la anulación anterior con motivo libre aparece con «Autorizó» vacío y la
// cuenta vacía liberada ya no aparece, porque no es una cancelación) y «Modificaciones de
// cantidades» (cancelaciones y modificaciones que reducen una partida ya confirmada) con sus
// totales, filtros, permisos y exportaciones PDF y Excel; y la referencia del comprobante como
// columna opcional de «Devoluciones» y «Correcciones de pagos». Desde la etapa 9F «Modificaciones de
// cantidades» tiene «Autorizó», vacía aquí porque el administrador cancela con su propio permiso (la
// autorización de un supervisor se prueba en operations, stage9f_cancel_units_test.go). La anulación
// anterior con motivo libre se retiró en la 9F: la cuenta anulada se deja como la dejaba esa ruta,
// directamente en la base (legacyVoid, dato anterior a la 9F). Con ADEL_REPORTS_MEASURE=1 mide además
// los dos reportes con un año de volumen sintético. Las cancelaciones de la 9D se prueban en
// stage9d_cancellations_integration_test.go.
//
// Como la prueba de exportaciones, usa su propia base desde cero y la deja vacía al terminar.
func TestStage9CCancellationReports(t *testing.T) {
	ctx, pool := openPool(t)
	if err := dropReportsSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := dropReportsSchema(context.Background(), pool); err != nil {
			t.Error(err)
		}
	})
	fixture := newReportsFixture(t)
	fixture.seedCatalog(t)
	shift := decode[payments.Shift](t, fixture.mutate(t, "POST", "payments/shifts", map[string]any{"opening_float": "500"}, 200))
	fixture.shiftID = shift.ID
	cash, debit := fixture.methods["cash"], fixture.methods["debit"]
	admin := fixture.scalar(t, "SELECT display_name FROM app.users WHERE username='admin'")
	route := func(bill operations.Bill, action string) string {
		return "restaurant/accounts/" + bill.Account.ID + "/" + action
	}
	cancel := func(bill operations.Bill, product catalog.Product, quantity int, reason string) operations.Bill {
		t.Helper()
		current := fixture.refresh(t, bill.Account.ID)
		return decode[operations.Bill](t, fixture.mutate(t, "POST", route(bill, "cancel"), map[string]any{
			"revision": current.Account.Revision, "version_id": fixture.versionOf(t, bill.Account.ID, product), "quantity": quantity,
			"reason": reason,
		}, 200))
	}

	// Cuenta anulada: 3 hamburguesas (120) y 2 refrescos (25); se cancela una hamburguesa, la partida
	// se modifica de 2 a 1, se cancelan los refrescos y la última hamburguesa y después se anula con la
	// forma anterior a la etapa 9F (dato heredado, insertado en la base de la prueba).
	voided := fixture.account(t, "Mesa que se fue", "dine_in", &fixture.table, line(fixture.burger, 3), line(fixture.soda, 2))
	cancel(voided, fixture.burger, 1, "Cliente cambió de opinión")
	current := fixture.refresh(t, voided.Account.ID)
	edit := map[string]any{
		"revision": current.Account.Revision, "version_id": fixture.versionOf(t, voided.Account.ID, fixture.burger),
		"line": line(fixture.burger, 1),
	}
	preview := decode[operations.QuoteResult](t, fixture.call(t, fixture.admin, "POST", route(voided, "edit-preview"), edit, 200))
	edit["expected_fingerprint"] = preview.Fingerprint
	edit["reason"] = "Solo quería una hamburguesa"
	fixture.mutate(t, "POST", route(voided, "edit"), edit, 200)
	cancel(voided, fixture.soda, 2, "Refrescos no servidos")
	cancel(voided, fixture.burger, 1, "Se retiró la orden")
	fixture.legacyVoid(t, voided.Account.ID, "Cliente se fue sin consumir")
	// Cuenta vacía liberada en la terraza.
	empty := decode[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
		"name": "Mesa liberada", "mode": "dine_in", "table_id": fixture.secondTable.ID,
	}, 200))
	fixture.mutate(t, "POST", route(empty, "release"), map[string]any{"revision": empty.Account.Revision}, 200)
	// Una cuenta cobrada con una partida reducida antes del cobro: no es anulada, pero su reducción sí
	// es una modificación de cantidad. Además lleva referencias de tarjeta en una devolución y en una
	// corrección.
	paid := fixture.account(t, "Con referencias", "dine_in", &fixture.table, line(fixture.salad, 2), line(fixture.burger, 1))
	cancel(paid, fixture.salad, 1, "Ensalada repetida por error")
	card := fixture.pay(t, paid.Account.ID, map[string]any{"method_id": debit, "mode": "partial", "amount": "20", "reference": "AUT-1001"})
	fixture.post(t, paid.Account.ID, "refunds", map[string]any{
		"mode": "payment", "payment_id": card.Payment.ID, "amount": "20", "method_id": debit, "reason": "Cobro duplicado",
		"reference": "DEV-2002",
	}, 200)
	wrong := fixture.pay(t, paid.Account.ID, map[string]any{"method_id": cash, "mode": "partial", "amount": "30"})
	fixture.post(t, paid.Account.ID, "corrections", map[string]any{
		"payment_id": wrong.Payment.ID, "method_id": debit, "reason": "Era tarjeta, no efectivo", "reference": "AUT-3003",
	}, 200)
	if !fixture.pay(t, paid.Account.ID, map[string]any{"method_id": debit, "mode": "total", "reference": "AUT-4004"}).Closed {
		t.Fatal("El cobro total debía cerrar la cuenta")
	}

	t.Run("cuentas anuladas con quién, terminal, motivo, lugar e importe cancelado", func(t *testing.T) {
		result := fixture.report(t, fixture.admin, "sales.voided_accounts", "")
		if len(result.Rows) != 1 || result.Meta.Title != "Cuentas canceladas" {
			t.Fatalf("Se esperaba solo la anulación (la liberación no es cancelación): %s %+v", result.Meta.Title, result.Rows)
		}
		row := find(t, result, "folio", text(voided.Account.Folio))
		expected := map[string]string{
			"kind": "Cancelación anterior con motivo libre", "cancelled_by": admin, "authorized_by": "", "terminal": "Caja principal",
			"reason": "Cliente se fue sin consumir", "description": "", "mode": "Salón", "place": "Mesa 1", "area": "Salón",
			"cancelled_units": "5", "cancelled_amount": "410.00", "account": "Mesa que se fue", "opened_by": admin,
		}
		for key, value := range expected {
			if text(row[key]) != value {
				t.Fatalf("Cuenta anulada: %s = %q, se esperaba %q (%+v)", key, text(row[key]), value, row)
			}
		}
		if text(result.Totals["cancelled_amount"]) != "410.00" || text(result.Totals["cancelled_units"]) != "5" {
			t.Fatalf("Totales de cuentas anuladas: %+v", result.Totals)
		}
		for query, count := range map[string]int{
			"reason_id=none": 1, "area_id=" + fixture.terrace.ID: 0, "area_id=" + fixture.area.ID: 1,
		} {
			if filtered := fixture.report(t, fixture.admin, "sales.voided_accounts", query); len(filtered.Rows) != count {
				t.Fatalf("Filtro %s: %+v", query, filtered.Rows)
			}
		}
		// El filtro anterior por tipo ya no existe.
		fixture.get(t, fixture.admin, "reports/run/sales.voided_accounts?kind=released", 400)
		if released := fixture.scalar(t, "SELECT state FROM app.restaurant_accounts WHERE id=$1", empty.Account.ID); released != "voided" {
			t.Fatalf("La cuenta vacía liberada debía seguir anulada: %s", released)
		}
		if takeaway := fixture.report(t, fixture.admin, "sales.voided_accounts", "mode=takeaway"); len(takeaway.Rows) != 0 {
			t.Fatalf("Ninguna cuenta para llevar se anuló: %+v", takeaway.Rows)
		}
		other := fixture.scalar(t, "SELECT gen_random_uuid()::text")
		if filtered := fixture.report(t, fixture.admin, "sales.voided_accounts", "user_id="+other); len(filtered.Rows) != 0 {
			t.Fatalf("Filtro por usuario: %+v", filtered.Rows)
		}
		// Un periodo sin anulaciones no muestra nada.
		yesterday := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
		if before := fixture.report(t, fixture.admin, "sales.voided_accounts", "period=day&date="+yesterday); len(before.Rows) != 0 {
			t.Fatalf("Periodo anterior: %+v", before.Rows)
		}
	})

	t.Run("modificaciones de cantidades con cantidad anterior y nueva, diferencia, quién y motivo", func(t *testing.T) {
		result := fixture.report(t, fixture.admin, "products.quantity_changes", "")
		if len(result.Rows) != 5 {
			t.Fatalf("Se esperaban cinco reducciones: %+v", result.Rows)
		}
		cases := map[string][]string{
			// motivo: producto, cambio, anterior, nueva, diferencia, importe, acción
			"Cliente cambió de opinión":   {"Hamburguesa", "Cantidad reducida", "3", "2", "-1", "-120.00", "Cancelación"},
			"Solo quería una hamburguesa": {"Hamburguesa", "Cantidad reducida", "2", "1", "-1", "-120.00", "Modificación de la partida"},
			"Refrescos no servidos":       {"Refresco", "Partida cancelada", "2", "0", "-2", "-50.00", "Cancelación"},
			"Se retiró la orden":          {"Hamburguesa", "Partida cancelada", "1", "0", "-1", "-120.00", "Cancelación"},
			"Ensalada repetida por error": {"Ensalada", "Cantidad reducida", "2", "1", "-1", "-50.00", "Cancelación"},
		}
		keys := []string{"product", "change", "previous_quantity", "new_quantity", "difference", "amount_difference", "action"}
		for reason, values := range cases {
			row := find(t, result, "reason", reason)
			for index, key := range keys {
				if text(row[key]) != values[index] {
					t.Fatalf("%s: %s = %q, se esperaba %q (%+v)", reason, key, text(row[key]), values[index], row)
				}
			}
			if text(row["changed_by"]) != admin || text(row["terminal"]) != "Caja principal" || text(row["mode"]) != "Salón" {
				t.Fatalf("%s: quién, terminal o modalidad: %+v", reason, row)
			}
		}
		if text(result.Totals["difference"]) != "-6" || text(result.Totals["amount_difference"]) != "-460.00" {
			t.Fatalf("Totales de modificaciones: %+v", result.Totals)
		}
		// Las cantidades anterior y nueva no se suman (no tendría sentido).
		if result.Totals["previous_quantity"] != nil || result.Totals["new_quantity"] != nil {
			t.Fatalf("La cantidad anterior no debe totalizarse: %+v", result.Totals)
		}
		// Los mismos importes que «Productos cancelados» (unidad por unidad) en el mismo periodo.
		cancelled := fixture.report(t, fixture.admin, "products.cancelled", "")
		if text(cancelled.Totals["amount"]) != "460.00" || text(cancelled.Totals["quantity"]) != "6" {
			t.Fatalf("Productos cancelados no coincide: %+v", cancelled.Totals)
		}
		sodas := fixture.report(t, fixture.admin, "products.quantity_changes", "product_id="+fixture.soda.ID)
		if len(sodas.Rows) != 1 || text(sodas.Rows[0]["product"]) != "Refresco" || len(sodas.Meta.Filters) != 1 ||
			sodas.Meta.Filters[0].Display != "Refresco" {
			t.Fatalf("Filtro por producto: %+v", sodas)
		}
		if takeaway := fixture.report(t, fixture.admin, "products.quantity_changes", "mode=takeaway"); len(takeaway.Rows) != 0 {
			t.Fatalf("Filtro por modalidad: %+v", takeaway.Rows)
		}
		adminID := fixture.scalar(t, "SELECT id::text FROM app.users WHERE username='admin'")
		if mine := fixture.report(t, fixture.admin, "products.quantity_changes", "user_id="+adminID); len(mine.Rows) != 5 {
			t.Fatalf("Filtro por usuario: %+v", mine.Rows)
		}
		sorted := fixture.report(t, fixture.admin, "products.quantity_changes", "sort=amount_difference&direction=asc&page_size=2")
		if len(sorted.Rows) != 2 || sorted.Page.TotalRows != 5 || text(sorted.Totals["amount_difference"]) != "-460.00" {
			t.Fatalf("Orden y página con los totales completos: %+v", sorted)
		}
		// Etapa 9F: «Autorizó» existe y queda vacía, porque el administrador canceló con su permiso.
		assertEmptyAuthorizationColumn(t, result)
	})

	t.Run("referencia del comprobante en devoluciones y correcciones", func(t *testing.T) {
		refunds := fixture.report(t, fixture.admin, "payments.refunds", "")
		row := find(t, refunds, "folio", text(paid.Account.Folio))
		if text(row["reference"]) != "DEV-2002" || text(row["original_reference"]) != "AUT-1001" {
			t.Fatalf("Referencias de la devolución: %+v", row)
		}
		corrections := fixture.report(t, fixture.admin, "payments.corrections", "")
		correction := find(t, corrections, "folio", text(paid.Account.Folio))
		if text(correction["original_reference"]) != "" || text(correction["new_reference"]) != "AUT-3003" {
			t.Fatalf("Referencias de la corrección: %+v", correction)
		}
		for _, id := range []string{"payments.refunds", "payments.corrections"} {
			definition, _ := reports.Lookup(id)
			for _, column := range definition.Columns {
				if strings.Contains(column.Key, "reference") && column.Visibility != reports.VisibilityOptional {
					t.Fatalf("%s.%s debe ser opcional", id, column.Key)
				}
			}
		}
		// «Detalle de cobros» ya tenía la referencia (9A) y «Compras recibidas por periodo» ya tiene «Recibió».
		list := fixture.report(t, fixture.admin, "payments.list", "")
		find(t, list, "reference", "AUT-4004")
		purchases, _ := reports.Lookup("purchases.received")
		receiver := false
		for _, column := range purchases.Columns {
			receiver = receiver || (column.Key == "received_by" && column.Label == "Recibió")
		}
		if !receiver {
			t.Fatal("«Compras recibidas por periodo» debe conservar la columna «Recibió»")
		}
	})

	t.Run("permisos de origen como Productos cancelados aunque se llame a la API directamente", func(t *testing.T) {
		sales := fixture.staff(t, "ventas_9c", "reports.read", "reports.sales.read")
		payer := fixture.staff(t, "cobros_9c", "reports.read", "reports.payments.read")
		nobody := fixture.staff(t, "modulo_9c", "reports.read")
		for _, id := range []string{"sales.voided_accounts", "products.quantity_changes"} {
			definition, _ := reports.Lookup(id)
			products, _ := reports.Lookup("products.cancelled")
			if len(definition.Permissions) != 1 || definition.Permissions[0] != products.Permissions[0] {
				t.Fatalf("%s debe exigir el permiso de «Productos cancelados»: %v", id, definition.Permissions)
			}
			fixture.get(t, sales, "reports/run/"+id, 200)
			fixture.get(t, payer, "reports/run/"+id, 403)
			fixture.get(t, nobody, "reports/run/"+id, 403)
			// Sin permiso de exportación no hay archivo.
			if response := fixture.export(t, sales, id, "format=pdf"); response.status != 403 {
				t.Fatalf("%s: exportar sin permiso respondió %d", id, response.status)
			}
		}
		fixture.get(t, sales, "reports/filters/users", 200)
		fixture.get(t, sales, "reports/filters/areas", 200)
	})

	t.Run("exportaciones PDF y Excel iguales a la pantalla", func(t *testing.T) {
		for id, names := range map[string][]string{
			// El PDF corta los textos largos en varias líneas de su celda: se buscan palabras sueltas.
			"sales.voided_accounts":     {"Cuentas canceladas", "Cancelación", "Administrador", "Mesa", "$410.00"},
			"products.quantity_changes": {"Modificaciones de cantidades", "Hamburguesa", "Refresco", "Ensalada", "-$460.00"},
		} {
			result := fixture.screen(t, fixture.admin, id, "")
			pdf := fixture.exportOK(t, fixture.admin, id, "format=pdf")
			if pdf.header.Get("Content-Type") != reports.ContentTypePDF || !strings.HasPrefix(string(pdf.body), "%PDF-") {
				t.Fatalf("%s: respuesta PDF %v", id, pdf.header)
			}
			pages := strings.Join(reports.PDFPagesForTest(t, pdf.body), "\n")
			for _, expected := range names {
				if !strings.Contains(pages, expected) {
					t.Fatalf("%s: el PDF no contiene «%s»:\n%s", id, expected, pages)
				}
			}
			xlsx := fixture.exportOK(t, fixture.admin, id, "format=xlsx")
			if xlsx.header.Get("Content-Type") != reports.ContentTypeXLSX {
				t.Fatalf("%s: respuesta XLSX %v", id, xlsx.header)
			}
			assertSameAsScreen(t, id, result, xlsx.body)
			if strings.Contains(workbookText(t, xlsx.body), "<f>") {
				t.Fatalf("%s: el libro contiene fórmulas", id)
			}
		}
		if count := fixture.scalar(t, `SELECT count(*)::text FROM app.audit_events WHERE action='report.exported'
            AND after_state->>'report' IN ('sales.voided_accounts','products.quantity_changes')`); count != "4" {
			t.Fatalf("Cada exportación debe auditarse: %s", count)
		}
	})

	if os.Getenv("ADEL_REPORTS_MEASURE") == "1" {
		t.Run("medición con un año de volumen sintético", func(t *testing.T) {
			measureCancellationReports(t, fixture)
		})
	}
}

// assertEmptyAuthorizationColumn comprueba que el reporte tiene la columna «Autorizó» (etapa 9F) y que
// está vacía en todos sus renglones: quien canceló o redujo tenía su propio permiso.
func assertEmptyAuthorizationColumn(t *testing.T, result reportResult) {
	t.Helper()
	found := false
	for _, column := range result.Columns {
		found = found || (column.Key == "authorized_by" && column.Label == "Autorizó")
	}
	if !found {
		t.Fatalf("%s debe tener la columna «Autorizó»", result.Report.ID)
	}
	for _, row := range result.Rows {
		if text(row["authorized_by"]) != "" {
			t.Fatalf("%s: «Autorizó» debía quedar vacía con el permiso propio: %+v", result.Report.ID, row)
		}
	}
}

// measureCancellationReports genera un año de volumen sintético sobre los registros de la prueba
// (60 000 cuentas, 600 000 eventos de restaurante, 1 200 cuentas anuladas y 6 000 unidades canceladas
// con su evento) y mide los dos reportes de un año con el ejecutor de solo lectura, sin y con un
// índice parcial candidato sobre restaurant_events, e imprime sus planes. Solo registra tiempos.
func measureCancellationReports(t *testing.T, fixture *reportsFixture) {
	ctx, pool := fixture.ctx, fixture.pool
	started := time.Now()
	if _, err := pool.Exec(ctx, `
        CREATE TEMP TABLE volume_account AS SELECT * FROM app.restaurant_accounts WHERE state='voided' ORDER BY folio LIMIT 1;
        CREATE TEMP TABLE volume_cancel AS SELECT * FROM app.restaurant_events WHERE kind='item.cancelled' ORDER BY sequence LIMIT 1;
        CREATE TEMP TABLE volume_unit AS SELECT * FROM app.restaurant_item_units WHERE state='cancelled' ORDER BY cancelled_at LIMIT 1;
        INSERT INTO app.restaurant_accounts(id,mode,name,table_id,price_list_id,origin,state,opened_by,assigned_user_id,device_id,created_at)
        SELECT md5('cuenta'||n)::uuid,a.mode,'Volumen '||n,a.table_id,a.price_list_id,a.origin,
            CASE WHEN n % 50 = 0 THEN 'voided' ELSE 'open' END,a.opened_by,a.assigned_user_id,a.device_id,
            date_trunc('day',now()) - make_interval(days => (n % 365)) + make_interval(mins => (n % 900))
        FROM generate_series(1,60000) n CROSS JOIN volume_account a;
        INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        SELECT md5('operacion'||n)::uuid,e.author_id,e.device_id,'POST medición','huella',
            jsonb_build_object('lines',jsonb_build_array(jsonb_build_object('version_id',e.payload->'after'->>'version_id',
                'state','active','quantity',2)))
        FROM generate_series(1,607200) n CROSS JOIN volume_cancel e;
        -- Ruido: diez eventos por cuenta con una carga de unos 600 bytes.
        INSERT INTO app.restaurant_events(id,account_id,operation_id,kind,payload,author_id,device_id,created_at)
        SELECT md5('evento'||n)::uuid,md5('cuenta'||((n % 60000) + 1))::uuid,md5('operacion'||n)::uuid,'draft.changed',
            jsonb_build_object('before',jsonb_build_object('nota',repeat('x',300)),'after',jsonb_build_object('nota',repeat('y',300))),
            e.author_id,e.device_id,date_trunc('day',now()) - make_interval(days => (n % 365)) + make_interval(mins => (n % 900))
        FROM generate_series(1,600000) n CROSS JOIN volume_cancel e;
        -- Anulación de una de cada 50 cuentas.
        INSERT INTO app.restaurant_events(id,account_id,operation_id,kind,payload,author_id,device_id,created_at)
        SELECT md5('anulacion'||n)::uuid,md5('cuenta'||n)::uuid,md5('operacion'||(600000+n/50))::uuid,'account.void',
            jsonb_build_object('before','{}'::jsonb,'after',jsonb_build_object('reason','Medición de volumen')),
            e.author_id,e.device_id,date_trunc('day',now()) - make_interval(days => (n % 365)) + make_interval(mins => (n % 900) + 1)
        FROM generate_series(50,60000,50) n CROSS JOIN volume_cancel e;
        -- 6 000 unidades canceladas, cada una con su evento item.cancelled en su cuenta.
        INSERT INTO app.restaurant_item_units(id,item_id,version_id,ordinal,account_id,state,subtotal,discount,total,cancelled_at)
        SELECT md5('unidad'||n)::uuid,u.item_id,u.version_id,1000+n,md5('cuenta'||(n*10))::uuid,'cancelled',25,0,25,
            date_trunc('day',now()) - make_interval(days => (n % 365)) + make_interval(mins => (n % 900) + 2)
        FROM generate_series(1,6000) n CROSS JOIN volume_unit u;
        INSERT INTO app.restaurant_events(id,account_id,operation_id,kind,payload,author_id,device_id,created_at)
        SELECT md5('cancelacion'||n)::uuid,md5('cuenta'||(n*10))::uuid,md5('operacion'||(601200+n))::uuid,'item.cancelled',
            jsonb_set(jsonb_set(e.payload,'{after,unit_ids}',jsonb_build_array(md5('unidad'||n)::uuid::text)),'{before,units}',
                jsonb_build_array(jsonb_build_object('id',md5('unidad'||n)::uuid::text,'total','25.00'))),
            e.author_id,e.device_id,date_trunc('day',now()) - make_interval(days => (n % 365)) + make_interval(mins => (n % 900) + 2)
        FROM generate_series(1,6000) n CROSS JOIN volume_cancel e;
        ANALYZE app.restaurant_accounts; ANALYZE app.restaurant_events; ANALYZE app.restaurant_item_units;
        ANALYZE app.restaurant_operations`); err != nil {
		t.Fatal(err)
	}
	t.Logf("Volumen sintético generado en %s", time.Since(started).Round(time.Millisecond))
	var events, size string
	if err := pool.QueryRow(ctx, `SELECT count(*)::text,pg_size_pretty(pg_total_relation_size('app.restaurant_events'))
        FROM app.restaurant_events`).Scan(&events, &size); err != nil {
		t.Fatal(err)
	}
	t.Logf("restaurant_events: %s renglones, %s", events, size)
	runner := reports.NewRunner(pool, reports.Slots)
	actor := reports.Actor{Permissions: []string{reports.PermissionRead, reports.PermissionSales}}
	measure := func(label string) {
		for _, id := range []string{"sales.voided_accounts", "products.quantity_changes"} {
			best := time.Duration(0)
			rows := 0
			for attempt := 0; attempt < 3; attempt++ {
				begin := time.Now()
				result, err := reports.Run(ctx, runner, actor, id, url.Values{"period": {"current_year"}})
				if err != nil {
					t.Fatal(err)
				}
				elapsed := time.Since(begin)
				if best == 0 || elapsed < best {
					best = elapsed
				}
				rows = result.Page.TotalRows
			}
			t.Logf("%s · %s (año actual): %d renglones, mejor de 3: %s", label, id, rows, best.Round(time.Millisecond))
		}
	}
	measure("sin índice nuevo")
	// Índice candidato: los eventos de anulación, cancelación y modificación por fecha.
	if _, err := pool.Exec(ctx, `CREATE INDEX measure_restaurant_events_reductions_idx ON app.restaurant_events(created_at)
        WHERE kind IN ('account.void','account.released','item.cancelled','item.modified'); ANALYZE app.restaurant_events`); err != nil {
		t.Fatal(err)
	}
	measure("con índice parcial candidato")
	if _, err := pool.Exec(ctx, "DROP INDEX app.measure_restaurant_events_reductions_idx"); err != nil {
		t.Fatal(err)
	}
	// La alternativa sin partir de las cuentas ni de las unidades recorre todos los eventos del año.
	for name, query := range map[string]string{
		"recorrido directo de eventos (sin índice)": `SELECT count(*) FROM app.restaurant_events
            WHERE kind IN ('account.void','account.released','item.cancelled','item.modified')
            AND created_at >= date_trunc('year',now()) AND created_at < date_trunc('year',now()) + interval '1 year'`,
		"cuentas anuladas (consulta del reporte)": `SELECT count(*) FROM app.restaurant_accounts a
            JOIN LATERAL (SELECT ev.created_at FROM app.restaurant_events ev WHERE ev.account_id=a.id
                AND ev.kind IN ('account.void','account.released') ORDER BY ev.sequence DESC LIMIT 1) e ON true
            WHERE a.state='voided' AND e.created_at >= date_trunc('year',now())`,
		"unidades canceladas → evento (consulta del reporte)": `SELECT count(DISTINCT e.id) FROM app.restaurant_item_units u
            JOIN LATERAL (SELECT ev.id FROM app.restaurant_events ev WHERE ev.account_id=u.account_id
                AND ev.kind IN ('item.cancelled','item.modified')
                AND (coalesce(ev.payload->'after'->'unit_ids','[]'::jsonb) ? u.id::text
                    OR coalesce(ev.payload->'after'->'cancelled_unit_ids','[]'::jsonb) ? u.id::text)
                ORDER BY ev.sequence DESC LIMIT 1) e ON true
            WHERE u.state='cancelled' AND u.cancelled_at >= date_trunc('year',now())`,
	} {
		rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query)
		if err != nil {
			t.Fatal(err)
		}
		plan := []string{}
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, line)
		}
		rows.Close()
		t.Logf("== %s\n%s", name, strings.Join(plan, "\n"))
	}
}
