//go:build integration

package reports_test

import (
	"context"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
	"local/ad-el-pos/server/internal/reports"
)

// Etapa 9D (§5.3 y §5.4) contra PostgreSQL con la API real: «Cuentas canceladas» desde el registro de
// «Cancelar cuenta» con motivo, descripción, importe, quién canceló y quién autorizó (vacío si bastó el
// permiso propio y en las anulaciones anteriores), sin las cuentas liberadas ni descartadas; el total
// por motivo; la exclusión de las ventas; las unidades en «Productos cancelados», «Modificaciones de
// cantidades» y «Actividad por empleado»; filtros, permisos y exportaciones PDF y Excel. Usa su propia
// base desde cero.
func TestStage9DCancelledAccountsReports(t *testing.T) {
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
	shift := decode[payments.Shift](t, fixture.mutate(t, "POST", "payments/shifts", map[string]any{"opening_float": "0"}, 200))
	fixture.shiftID = shift.ID
	admin := fixture.scalar(t, "SELECT display_name FROM app.users WHERE username='admin'")
	cashier := fixture.staff(t, "cajero_rep9d", "accounts.read", "accounts.open", "takeaway.read", "takeaway.manage")
	fixture.staff(t, "supervisor_rep9d", "accounts.read", "accounts.cancel")
	reasons := map[string]operations.CancelReason{}
	for _, reason := range decode[[]operations.CancelReason](t, fixture.get(t, fixture.admin, "restaurant/cancel-reasons", 200)) {
		reasons[reason.Name] = reason
	}
	cancel := func(member terminal, bill operations.Bill, reason, description string, authorization map[string]any) operations.Bill {
		t.Helper()
		body := map[string]any{"operation_id": randomID(), "revision": fixture.refresh(t, bill.Account.ID).Account.Revision,
			"reason_id": reasons[reason].ID, "description": description}
		if authorization != nil {
			body["authorization"] = authorization
		}
		return decode[operations.Bill](t, fixture.call(t, member, "POST", "restaurant/accounts/"+bill.Account.ID+"/cancel-account", body, 200))
	}
	// A: mesa con dos hamburguesas y un refresco (265), cancelada por el administrador con su permiso.
	left := fixture.account(t, "Mesa que se fue", "dine_in", &fixture.table, line(fixture.burger, 2), line(fixture.soda, 1))
	cancel(fixture.admin, left, "Cliente se retiró sin pagar", "Se fueron sin avisar", nil)
	// B: para llevar con dos refrescos (50), cancelado por el cajero con la autorización del supervisor.
	takeaway := fixture.account(t, "Pedido abandonado", "takeaway", nil, line(fixture.soda, 2))
	cancel(cashier, takeaway, "Cliente se retiró sin pagar", "", map[string]any{
		"username": "supervisor_rep9d", "password": "Clave temporal de supervisor_rep9d",
	})
	// C: una ensalada (50) cancelada con «Otro» y su descripción.
	other := fixture.account(t, "Pedido de prueba", "dine_in", nil, line(fixture.salad, 1))
	cancel(fixture.admin, other, "Otro", "Capturado en la mesa equivocada", nil)
	// D: anulación con la forma anterior (motivo libre), retirada en la etapa 9F: dato heredado
	// insertado en la base de la prueba como lo dejaba esa ruta.
	legacy := decode[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": "Anulada antes", "mode": "dine_in"}, 200))
	fixture.legacyVoid(t, legacy.Account.ID, "Cliente se fue sin consumir")
	// E y F: una cuenta vacía liberada y una descartada con su borrador; no son cancelaciones.
	released := decode[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
		"mode": "dine_in", "table_id": fixture.secondTable.ID}, 200))
	fixture.mutate(t, "POST", "restaurant/accounts/"+released.Account.ID+"/release", map[string]any{"revision": released.Account.Revision}, 200)
	drafted := decode[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
		"mode": "dine_in", "items": []operations.DraftItem{{ID: randomID(), LineRequest: line(fixture.burger, 1)}},
	}, 200))
	fixture.mutate(t, "POST", "restaurant/accounts/"+drafted.Account.ID+"/discard", map[string]any{"revision": drafted.Account.Revision}, 200)
	// G: una cuenta cobrada, la única venta.
	sold := fixture.account(t, "Cobrada", "dine_in", nil, line(fixture.salad, 1))
	if !fixture.pay(t, sold.Account.ID, map[string]any{"method_id": fixture.methods["cash"], "mode": "total", "received": "50"}).Closed {
		t.Fatal("El cobro total debía cerrar la cuenta")
	}
	// Renombrar un motivo después no cambia lo registrado.
	waste := reasons["Cliente se retiró sin pagar"]
	fixture.mutate(t, "PATCH", "restaurant/cancel-reasons/"+waste.ID, map[string]any{
		"revision": waste.Revision, "name": "Se fue sin pagar", "requires_description": false, "prepared_is_waste": true, "active": true,
	}, 200)

	t.Run("cuentas canceladas con motivo, descripción, importe, quién canceló y quién autorizó", func(t *testing.T) {
		result := fixture.report(t, fixture.admin, "sales.voided_accounts", "")
		if result.Meta.Title != "Cuentas canceladas" || len(result.Rows) != 4 {
			t.Fatalf("Se esperaban las tres cancelaciones y la anulación anterior: %s %+v", result.Meta.Title, result.Rows)
		}
		for _, excluded := range []operations.Bill{released, drafted, sold} {
			for _, row := range result.Rows {
				if text(row["folio"]) == text(excluded.Account.Folio) {
					t.Fatalf("La cuenta %d no debía aparecer: %+v", excluded.Account.Folio, row)
				}
			}
		}
		cases := map[string]map[string]string{
			text(left.Account.Folio): {"reason": "Cliente se retiró sin pagar", "description": "Se fueron sin avisar",
				"cancelled_amount": "265.00", "cancelled_units": "3", "cancelled_by": admin, "authorized_by": "", "mode": "Salón",
				"place": "Mesa 1", "kind": "Cancelación"},
			text(takeaway.Account.Folio): {"reason": "Cliente se retiró sin pagar", "description": "", "cancelled_amount": "50.00",
				"cancelled_units": "2", "cancelled_by": "Prueba cajero_rep9d", "authorized_by": "Prueba supervisor_rep9d",
				"mode": "Para llevar", "place": "Ventanilla", "terminal": "Terminal cajero_rep9d"},
			text(other.Account.Folio): {"reason": "Otro", "description": "Capturado en la mesa equivocada", "cancelled_amount": "50.00",
				"cancelled_units": "1", "authorized_by": ""},
			text(legacy.Account.Folio): {"reason": "Cliente se fue sin consumir", "description": "", "cancelled_amount": "0.00",
				"cancelled_units": "0", "authorized_by": "", "kind": "Cancelación anterior con motivo libre"},
		}
		for folio, expected := range cases {
			row := find(t, result, "folio", folio)
			for key, value := range expected {
				if text(row[key]) != value {
					t.Fatalf("Folio %s: %s = %q, se esperaba %q (%+v)", folio, key, text(row[key]), value, row)
				}
			}
		}
		if text(result.Totals["cancelled_amount"]) != "365.00" || text(result.Totals["cancelled_units"]) != "6" {
			t.Fatalf("Totales: %+v", result.Totals)
		}
		definition, _ := reports.Lookup("sales.voided_accounts")
		authorized := false
		for _, column := range definition.Columns {
			authorized = authorized || (column.Key == "authorized_by" && column.Label == "Autorizó")
		}
		if !authorized {
			t.Fatal("Falta la columna «Autorizó»")
		}
		cashierID := fixture.scalar(t, "SELECT id::text FROM app.users WHERE username='cajero_rep9d'")
		for query, count := range map[string]int{
			"reason_id=" + waste.ID: 2, "reason_id=none": 1, "reason_id=" + reasons["Otro"].ID: 1,
			"user_id=" + cashierID: 1, "mode=takeaway": 1, "area_id=" + fixture.area.ID: 1,
		} {
			if filtered := fixture.report(t, fixture.admin, "sales.voided_accounts", query); len(filtered.Rows) != count {
				t.Fatalf("Filtro %s: %d renglones (%+v)", query, len(filtered.Rows), filtered.Rows)
			}
		}
		named := fixture.report(t, fixture.admin, "sales.voided_accounts", "reason_id="+waste.ID)
		if len(named.Meta.Filters) != 1 || named.Meta.Filters[0].Display != "Se fue sin pagar" {
			t.Fatalf("El filtro muestra el nombre vigente del motivo: %+v", named.Meta.Filters)
		}
	})

	t.Run("total por motivo", func(t *testing.T) {
		result := fixture.report(t, fixture.admin, "sales.cancellations_by_reason", "")
		if len(result.Rows) != 3 {
			t.Fatalf("Motivos: %+v", result.Rows)
		}
		cases := map[string][]string{
			// motivo: cuentas, unidades, importe, porcentaje
			"Cliente se retiró sin pagar": {"2", "5", "315.00", "86.30"},
			"Otro":                        {"1", "1", "50.00", "13.70"},
			"Cancelaciones anteriores con motivo libre": {"1", "0", "0.00", "0.00"},
		}
		for reason, values := range cases {
			row := find(t, result, "reason", reason)
			for index, key := range []string{"accounts", "cancelled_units", "cancelled_amount", "share"} {
				if text(row[key]) != values[index] {
					t.Fatalf("%s: %s = %q, se esperaba %q (%+v)", reason, key, text(row[key]), values[index], row)
				}
			}
		}
		if text(result.Totals["accounts"]) != "4" || text(result.Totals["cancelled_amount"]) != "365.00" ||
			text(result.Totals["share"]) != "100.00" {
			t.Fatalf("Totales por motivo: %+v", result.Totals)
		}
		if result.Rows[0]["reason"] != "Cliente se retiró sin pagar" {
			t.Fatalf("El orden predeterminado es por importe: %+v", result.Rows)
		}
		if takeaway := fixture.report(t, fixture.admin, "sales.cancellations_by_reason", "mode=takeaway"); len(takeaway.Rows) != 1 ||
			text(takeaway.Rows[0]["cancelled_amount"]) != "50.00" {
			t.Fatalf("Por motivo para llevar: %+v", takeaway.Rows)
		}
	})

	t.Run("excluidas de ventas y presentes en productos cancelados, cantidades y empleados", func(t *testing.T) {
		if sales := fixture.scalar(t, "SELECT string_agg(DISTINCT account_id::text,',') FROM app.sales_recognitions"); sales != sold.Account.ID {
			t.Fatalf("Solo la cuenta cobrada es venta: %s", sales)
		}
		byDay := fixture.report(t, fixture.admin, "sales.by_day", "")
		if text(byDay.Totals["net"]) != "50.00" {
			t.Fatalf("Venta neta del periodo: %+v", byDay.Totals)
		}
		cancelled := fixture.report(t, fixture.admin, "products.cancelled", "")
		if text(cancelled.Totals["quantity"]) != "6" || text(cancelled.Totals["amount"]) != "365.00" {
			t.Fatalf("Productos cancelados: %+v", cancelled.Totals)
		}
		row := find(t, cancelled, "folio", text(left.Account.Folio))
		if text(row["reason"]) != "Cuenta cancelada: Cliente se retiró sin pagar" || text(row["cancelled_by"]) != admin {
			t.Fatalf("Unidad de la cuenta cancelada: %+v", row)
		}
		changes := fixture.report(t, fixture.admin, "products.quantity_changes", "")
		if len(changes.Rows) != 4 || text(changes.Totals["difference"]) != "-6" || text(changes.Totals["amount_difference"]) != "-365.00" {
			t.Fatalf("Modificaciones de cantidades: %+v %+v", changes.Totals, changes.Rows)
		}
		for _, row := range changes.Rows {
			if text(row["action"]) != "Cancelación de la cuenta" || text(row["change"]) != "Partida cancelada" ||
				text(row["new_quantity"]) != "0" {
				t.Fatalf("Renglón de la cuenta cancelada: %+v", row)
			}
		}
		burgers := 0
		for _, row := range changes.Rows {
			if text(row["product"]) == "Hamburguesa" {
				burgers++
				if text(row["previous_quantity"]) != "2" || text(row["amount_difference"]) != "-240.00" {
					t.Fatalf("Partida de hamburguesas: %+v", row)
				}
			}
		}
		if burgers != 1 {
			t.Fatalf("Se esperaba una partida de hamburguesas: %d", burgers)
		}
		// Etapa 9F: «Autorizó» de cada partida es quien autorizó cancelar su cuenta (vacía con el permiso
		// propio), en «Modificaciones de cantidades» y en «Productos cancelados».
		for _, report := range []reportResult{changes, cancelled} {
			for _, row := range report.Rows {
				expected := ""
				if text(row["folio"]) == text(takeaway.Account.Folio) {
					expected = "Prueba supervisor_rep9d"
				}
				if text(row["authorized_by"]) != expected {
					t.Fatalf("%s: «Autorizó» de la cuenta %s = %q, se esperaba %q", report.Report.ID, text(row["folio"]),
						text(row["authorized_by"]), expected)
				}
			}
		}
		activity := fixture.report(t, fixture.admin, "employees.activity", "")
		supervisor := find(t, activity, "user", "Prueba supervisor_rep9d")
		cashierRow := find(t, activity, "user", "Prueba cajero_rep9d")
		if text(supervisor["authorizations"]) != "1" || text(cashierRow["cancelled_units"]) != "2" {
			t.Fatalf("Actividad: supervisor %+v, cajero %+v", supervisor, cashierRow)
		}
	})

	t.Run("permisos de origen y fuente de motivos", func(t *testing.T) {
		sales := fixture.staff(t, "ventas_9d", "reports.read", "reports.sales.read")
		payer := fixture.staff(t, "cobros_9d", "reports.read", "reports.payments.read")
		definition, _ := reports.Lookup("sales.cancellations_by_reason")
		if len(definition.Permissions) != 1 || definition.Permissions[0] != reports.PermissionSales {
			t.Fatalf("El total por motivo debe exigir el permiso de ventas: %v", definition.Permissions)
		}
		for _, id := range []string{"sales.voided_accounts", "sales.cancellations_by_reason"} {
			fixture.get(t, sales, "reports/run/"+id, 200)
			fixture.get(t, payer, "reports/run/"+id, 403)
		}
		options := decode[struct {
			Options []reports.Option `json:"options"`
		}](t, fixture.get(t, sales, "reports/filters/cancel_reasons", 200))
		labels := []string{}
		for _, option := range options.Options {
			labels = append(labels, option.Label)
		}
		if strings.Join(labels, "|") != "Sin motivo del catálogo|Se fue sin pagar|Cliente canceló el pedido|Error de captura|Otro" {
			t.Fatalf("Opciones del filtro de motivo: %v", labels)
		}
		fixture.get(t, payer, "reports/filters/cancel_reasons", 403)
	})

	t.Run("exportaciones PDF y Excel iguales a la pantalla", func(t *testing.T) {
		for id, names := range map[string][]string{
			"sales.voided_accounts":         {"Cuentas canceladas", "Autorizó", "supervisor_rep9d", "Ventanilla", "$365.00"},
			"sales.cancellations_by_reason": {"Cancelaciones por motivo", "Cliente se retiró", "$315.00", "$365.00"},
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
	})
}
