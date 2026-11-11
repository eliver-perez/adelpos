//go:build integration

package reports_test

import (
	"context"
	"io/fs"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
	"local/ad-el-pos/server/internal/reports"
	"local/ad-el-pos/server/migrations"
)

// Aceptación de reportes de la fase B1 contra PostgreSQL con la API real: reconocimiento de ventas
// al cerrar, «En curso», devoluciones en su día, reliquidaciones, cortesías, motivos históricos,
// combos, divisiones y combinaciones, pagos parciales, mixtos, propinas, dólares, correcciones,
// permisos, totales y paginación, relleno de la 0017 y hora de inicio del día.
func TestReportsAcceptance(t *testing.T) {
	fixture := newReportsFixture(t)
	fixture.seedCatalog(t)
	accounts := map[string]operations.Bill{}

	t.Run("reportes sin ningún turno", func(t *testing.T) {
		// Una cortesía cierra la cuenta sin cobro y sin turno: la venta se reconoce igual.
		bill := fixture.account(t, "Cortesía sin turno", "dine_in", &fixture.table, line(fixture.soda, 2))
		closed := decode[struct {
			Closed bool `json:"closed"`
		}](t, fixture.post(t, bill.Account.ID, "discounts", map[string]any{
			"scope": "account", "method": "percent", "value": "100", "courtesy": true, "reason": "Cortesía de la casa",
		}, 200))
		if !closed.Closed {
			t.Fatal("La cortesía al 100 % no cerró la cuenta")
		}
		accounts["courtesy"] = bill
		if count := fixture.scalar(t, "SELECT count(*)::text FROM app.cash_shifts"); count != "0" {
			t.Fatalf("La prueba debe correr sin turnos: %s", count)
		}
		// Todos los reportes registrados responden sin ningún turno abierto ni cerrado.
		for _, definition := range reports.Definitions() {
			fixture.report(t, fixture.admin, definition.ID, "")
		}
		summary := fixture.report(t, fixture.admin, "sales.summary", "")
		if text(summary.Totals["net"]) != "0.00" || text(summary.Totals["courtesy"]) != "50.00" || text(summary.Totals["gross"]) != "50.00" ||
			text(summary.Totals["quantity"]) != "2" {
			t.Fatalf("Cortesía al 100 %%: venta 0 y su importe en cortesías: %+v", summary.Totals)
		}
		if shift := fixture.scalar(t, "SELECT coalesce(min(shift_id::text),'sin turno') FROM app.sales_recognitions"); shift != "sin turno" {
			t.Fatalf("La cortesía sin turno quedó atribuida a un turno: %s", shift)
		}
		courtesies := fixture.report(t, fixture.admin, "discounts.courtesies", "")
		if len(courtesies.Rows) != 1 || text(courtesies.Rows[0]["total_discount"]) != "50.00" || text(courtesies.Rows[0]["reason"]) != "Sin motivo del catálogo" {
			t.Fatalf("Reporte de cortesías: %+v", courtesies.Rows)
		}
	})

	shift := decode[payments.Shift](t, fixture.mutate(t, "POST", "payments/shifts", map[string]any{"opening_float": "500"}, 200))
	fixture.shiftID = shift.ID
	cash, debit := fixture.methods["cash"], fixture.methods["debit"]

	t.Run("venta contada al cerrar y cuentas abiertas en En curso", func(t *testing.T) {
		bill := fixture.account(t, "Mesa uno", "dine_in", &fixture.table, line(fixture.burger, 2), line(fixture.soda, 1))
		before := fixture.report(t, fixture.admin, "sales.summary", "")
		if text(before.Totals["net"]) != "0.00" {
			t.Fatalf("Una cuenta abierta no debe contar como venta: %+v", before.Totals)
		}
		progress := fixture.report(t, fixture.admin, "sales.in_progress", "")
		row := find(t, progress, "folio", text(bill.Account.Folio))
		if text(row["total"]) != "265.00" || text(row["balance"]) != "265.00" || text(row["status"]) != "Con saldo pendiente" {
			t.Fatalf("La cuenta abierta no aparece en «En curso» con su total y saldo: %+v", row)
		}
		result := fixture.pay(t, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received": "300"})
		if !result.Closed {
			t.Fatal("El cobro total no cerró la cuenta")
		}
		accounts["first"] = bill
		after := fixture.report(t, fixture.admin, "sales.summary", "")
		if text(after.Totals["net"]) != "265.00" || text(after.Totals["quantity"]) != "5" {
			t.Fatalf("La venta debe contarse al cerrar la cuenta: %+v", after.Totals)
		}
		var recognized, closedAt time.Time
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT min(r.recognized_at),a.closed_at FROM app.sales_recognitions r
            JOIN app.restaurant_accounts a ON a.id=r.account_id WHERE r.account_id=$1 GROUP BY a.closed_at`, bill.Account.ID).
			Scan(&recognized, &closedAt); err != nil || !recognized.Equal(closedAt) {
			t.Fatalf("El reconocimiento debe tener la fecha del cierre: %s %s %v", recognized, closedAt, err)
		}
		if found := fixture.scalar(t, "SELECT count(*)::text FROM app.sales_recognitions WHERE account_id=$1 AND shift_id=$2", bill.Account.ID, shift.ID); found != "3" {
			t.Fatalf("La venta pertenece al turno de la operación que cerró la cuenta: %s", found)
		}
		open := fixture.account(t, "Sigue abierta", "dine_in", &fixture.secondTable, line(fixture.soda, 1))
		accounts["open"] = open
		progress = fixture.report(t, fixture.admin, "sales.in_progress", "state=with_balance")
		find(t, progress, "folio", text(open.Account.Folio))
	})

	t.Run("pagos parciales mixtos propinas y correcciones", func(t *testing.T) {
		mixed := fixture.account(t, "Mixta", "dine_in", &fixture.table, line(fixture.burger, 1))
		fixture.pay(t, mixed.Account.ID, map[string]any{"method_id": cash, "mode": "partial", "amount": "50"})
		fixture.pay(t, mixed.Account.ID, map[string]any{"method_id": debit, "mode": "total"})
		accounts["mixed"] = mixed
		partial := fixture.account(t, "Parcial", "dine_in", &fixture.table, line(fixture.soda, 2))
		fixture.pay(t, partial.Account.ID, map[string]any{"method_id": cash, "mode": "partial", "amount": "20"})
		fixture.pay(t, partial.Account.ID, map[string]any{"method_id": cash, "mode": "total", "tip": "10", "received": "40"})
		accounts["partial"] = partial
		corrected := fixture.account(t, "Corregida", "dine_in", &fixture.table, line(fixture.soda, 2))
		wrong := fixture.pay(t, corrected.Account.ID, map[string]any{"method_id": cash, "mode": "partial", "amount": "30"})
		fixture.post(t, corrected.Account.ID, "corrections", map[string]any{
			"payment_id": wrong.Payment.ID, "method_id": debit, "reason": "Era tarjeta, no efectivo",
		}, 200)
		fixture.pay(t, corrected.Account.ID, map[string]any{"method_id": debit, "mode": "total"})
		accounts["corrected"] = corrected

		mixedReport := fixture.report(t, fixture.admin, "payments.mixed_partial", "")
		if row := find(t, mixedReport, "folio", text(mixed.Account.Folio)); text(row["classification"]) != "Mixto (varios métodos)" ||
			text(row["collected"]) != "120.00" {
			t.Fatalf("Pago mixto derivado: %+v", row)
		}
		if row := find(t, mixedReport, "folio", text(partial.Account.Folio)); text(row["classification"]) != "Parcial (varios pagos)" ||
			text(row["tips"]) != "10.00" {
			t.Fatalf("Pago parcial derivado: %+v", row)
		}
		byMethod := fixture.report(t, fixture.admin, "payments.by_method", "")
		cashRow := find(t, byMethod, "method", "Efectivo")
		debitRow := find(t, byMethod, "method", "Tarjeta de débito")
		// Efectivo: 265 + 50 + 20 + 30 + 30 = 395 cobrados, 30 corregidos; débito: 70 + 30 + 20 = 120.
		if text(cashRow["collected"]) != "395.00" || text(cashRow["corrections"]) != "30.00" || text(cashRow["net"]) != "365.00" ||
			text(cashRow["tips"]) != "10.00" {
			t.Fatalf("Cobros en efectivo: %+v", cashRow)
		}
		if text(debitRow["collected"]) != "120.00" || text(debitRow["net"]) != "120.00" {
			t.Fatalf("Cobros con tarjeta: %+v", debitRow)
		}
		if text(byMethod.Totals["net"]) != "485.00" {
			t.Fatalf("Una corrección no debe duplicar lo cobrado: %+v", byMethod.Totals)
		}
		// Propinas separadas: la venta del periodo no incluye los 10 de propina.
		summary := fixture.report(t, fixture.admin, "sales.summary", "")
		if text(summary.Totals["net"]) != "485.00" {
			t.Fatalf("Venta neta = 265 + 120 + 50 + 50 (sin propinas): %+v", summary.Totals)
		}
		tips := fixture.report(t, fixture.admin, "payments.tips", "group_by=method")
		if row := find(t, tips, "group_label", "Efectivo"); text(row["tips"]) != "10.00" {
			t.Fatalf("Propinas por método: %+v", tips.Rows)
		}
		corrections := fixture.report(t, fixture.admin, "payments.corrections", "")
		if len(corrections.Rows) != 1 || text(corrections.Rows[0]["new_method"]) != "Tarjeta de débito" ||
			text(corrections.Rows[0]["original_amount"]) != "30.00" {
			t.Fatalf("Correcciones: %+v", corrections.Rows)
		}
	})

	t.Run("devolución en su propio día y cuenta reabierta con saldo cero", func(t *testing.T) {
		bill := fixture.account(t, "Devuelta", "dine_in", &fixture.table, line(fixture.burger, 1), line(fixture.soda, 1))
		soda := fixture.versionOf(t, bill.Account.ID, fixture.soda)
		// El refresco se paga por selección para poder devolverlo por producto.
		fixture.pay(t, bill.Account.ID, map[string]any{"method_id": debit, "mode": "total",
			"selections": []map[string]any{{"version_id": soda, "quantity": 1}}})
		fixture.pay(t, bill.Account.ID, map[string]any{"method_id": debit, "mode": "total"})
		refund := decode[struct {
			Reopened bool `json:"reopened"`
		}](t, fixture.post(t, bill.Account.ID, "refunds", map[string]any{
			"mode": "items", "selections": []map[string]any{{"version_id": soda, "quantity": 1}}, "method_id": cash, "reason": "Refresco derramado",
		}, 200))
		if !refund.Reopened {
			t.Fatal("La devolución debía reabrir la cuenta")
		}
		current := fixture.refresh(t, bill.Account.ID)
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/cancel", map[string]any{
			"revision": current.Account.Revision, "version_id": soda, "quantity": 1, "reason": "Refresco devuelto",
		}, 200)
		accounts["refunded"] = bill
		// La devolución se mueve al día siguiente para comprobar que se refleja en su propio día.
		if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.payment_refunds SET created_at=created_at+interval '1 day'
            WHERE account_id=$1`, bill.Account.ID); err != nil {
			t.Fatal(err)
		}
		today := fixture.report(t, fixture.admin, "sales.by_day", "").Meta.Period.FirstDay
		tomorrow := mustDate(t, today).AddDate(0, 0, 1).Format("2006-01-02")
		days := fixture.report(t, fixture.admin, "sales.by_day", "period=range&from="+today+"&until="+tomorrow)
		if text(find(t, days, "day", today)["net"]) != "630.00" || text(find(t, days, "day", tomorrow)["net"]) != "0.00" {
			t.Fatalf("La devolución no debe alterar la venta del día original: %+v", days.Rows)
		}
		cobros := fixture.report(t, fixture.admin, "payments.by_day", "period=range&from="+today+"&until="+tomorrow)
		if text(find(t, cobros, "day", tomorrow)["refunds"]) != "25.00" || text(find(t, cobros, "day", today)["refunds"]) != "0.00" {
			t.Fatalf("La devolución debe reflejarse en su día: %+v", cobros.Rows)
		}
		refunds := fixture.report(t, fixture.admin, "payments.refunds", "period=day&date="+tomorrow)
		if len(refunds.Rows) != 1 || text(refunds.Rows[0]["amount"]) != "25.00" || text(refunds.Rows[0]["reason"]) != "Refresco derramado" {
			t.Fatalf("Reporte de devoluciones: %+v", refunds.Rows)
		}
		progress := fixture.report(t, fixture.admin, "sales.in_progress", "state=reopened")
		row := find(t, progress, "folio", text(bill.Account.Folio))
		if text(row["status"]) != "Reabierta" || text(row["balance"]) != "0.00" {
			t.Fatalf("La cuenta reabierta con saldo 0 debe aparecer como reabierta: %+v", row)
		}
		cancelled := fixture.report(t, fixture.admin, "products.cancelled", "")
		if len(cancelled.Rows) != 1 || text(cancelled.Rows[0]["reason"]) != "Refresco devuelto" || text(cancelled.Rows[0]["amount"]) != "25.00" {
			t.Fatalf("Productos cancelados con fecha y motivo: %+v", cancelled.Rows)
		}
	})

	t.Run("reliquidación que reconoce solo diferencias", func(t *testing.T) {
		bill := fixture.account(t, "Reliquidada", "dine_in", &fixture.table, line(fixture.burger, 1), line(fixture.fries, 1))
		payment := fixture.pay(t, bill.Account.ID, map[string]any{"method_id": debit, "mode": "total"})
		fixture.post(t, bill.Account.ID, "refunds", map[string]any{
			"mode": "payment", "payment_id": payment.Payment.ID, "amount": "40", "method_id": cash, "reason": "Papas no servidas",
		}, 200)
		fries := fixture.versionOf(t, bill.Account.ID, fixture.fries)
		current := fixture.refresh(t, bill.Account.ID)
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/cancel", map[string]any{
			"revision": current.Account.Revision, "version_id": fries, "quantity": 1, "reason": "Papas no servidas",
		}, 200)
		fixture.capture(t, bill, line(fixture.soda, 1))
		if !fixture.pay(t, bill.Account.ID, map[string]any{"method_id": cash, "mode": "total"}).Closed {
			t.Fatal("La reliquidación debía volver a cerrar la cuenta")
		}
		accounts["resettled"] = bill
		rows := fixture.scalar(t, `SELECT string_agg(recognition_kind||':'||source_kind||':'||quantity||':'||net::text,','
                ORDER BY recognition_kind,net)
            FROM app.sales_recognitions WHERE account_id=$1`, bill.Account.ID)
		if rows != "initial:unit:1:40.00,initial:unit:1:120.00,resettlement:unit:-1:-40.00,resettlement:unit:1:25.00" {
			t.Fatalf("La reliquidación debe reconocer solo las diferencias: %s", rows)
		}
		total := fixture.scalar(t, "SELECT sum(net)::text FROM app.sales_recognitions WHERE account_id=$1", bill.Account.ID)
		if total != fixture.finance(t, bill.Account.ID).Total {
			t.Fatalf("Lo reconocido debe ser el total vigente: %s", total)
		}
		// Un nuevo cierre sin cambios no reconoce nada.
		if count := fixture.scalar(t, "SELECT count(DISTINCT closing_id)::text FROM app.sales_recognitions WHERE account_id=$1", bill.Account.ID); count != "2" {
			t.Fatalf("Cierres reconocidos: %s", count)
		}
		closings := fixture.report(t, fixture.admin, "sales.accounts", "")
		resettlement := 0
		for _, row := range closings.Rows {
			if text(row["folio"]) == text(bill.Account.Folio) && text(row["kind"]) == "Reliquidación" && text(row["net"]) == "-15.00" {
				resettlement++
			}
		}
		if resettlement != 1 {
			t.Fatalf("La reliquidación debe aparecer como cierre propio: %+v", closings.Rows)
		}
	})

	t.Run("descuentos con motivo histórico y sin motivo del catálogo", func(t *testing.T) {
		reasons := decode[[]struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Revision int    `json:"revision"`
		}](t, fixture.get(t, fixture.admin, "payments/discount-reasons", 200))
		var attention string
		var revision int
		for _, reason := range reasons {
			if reason.Name == "Atención" {
				attention, revision = reason.ID, reason.Revision
			}
		}
		bill := fixture.account(t, "Con descuentos", "dine_in", &fixture.secondTable, line(fixture.burger, 2))
		burger := fixture.versionOf(t, bill.Account.ID, fixture.burger)
		fixture.post(t, bill.Account.ID, "discounts", map[string]any{
			"scope": "items", "method": "percent", "value": "10", "reason_id": attention, "reason": "",
			"selections": []map[string]any{{"version_id": burger, "quantity": 1}},
		}, 200)
		fixture.post(t, bill.Account.ID, "discounts", map[string]any{"scope": "account", "method": "amount", "value": "20", "reason": "Ajuste"}, 200)
		fixture.pay(t, bill.Account.ID, map[string]any{"method_id": debit, "mode": "total"})
		accounts["discounted"] = bill
		fixture.call(t, fixture.admin, "PATCH", "payments/discount-reasons/"+attention, map[string]any{
			"operation_id": randomID(), "name": "Atención especial", "requires_description": false, "active": true, "revision": revision,
		}, 200)
		byReason := fixture.report(t, fixture.admin, "discounts.by_reason", "kind=discount")
		if text(find(t, byReason, "reason", "Atención")["total_discount"]) != "12.00" ||
			text(find(t, byReason, "reason", "Sin motivo del catálogo")["total_discount"]) != "20.00" {
			t.Fatalf("Descuentos por motivo histórico: %+v", byReason.Rows)
		}
		for _, row := range byReason.Rows {
			if text(row["reason"]) == "Atención especial" {
				t.Fatal("Renombrar el motivo cambió el reporte")
			}
		}
		withoutReason := fixture.report(t, fixture.admin, "discounts.by_reason", "reason_id=none")
		if len(withoutReason.Rows) != 2 || text(withoutReason.Totals["total_discount"]) != "70.00" {
			t.Fatalf("Filtro «Sin motivo del catálogo» (el descuento general y la cortesía): %+v", withoutReason.Rows)
		}
		general := fixture.report(t, fixture.admin, "discounts.general", "")
		if text(general.Totals["total_discount"]) != "70.00" {
			t.Fatalf("Descuentos generales de cuenta (incluida la cortesía general): %+v", general.Rows)
		}
		products := fixture.report(t, fixture.admin, "discounts.by_product", "")
		if row := find(t, products, "product", "Hamburguesa"); text(row["discount"]) != "12.00" || text(row["units"]) != "1" {
			t.Fatalf("Descuentos por producto: %+v", row)
		}
		summary := fixture.report(t, fixture.admin, "discounts.summary", "")
		if text(summary.Totals["item_discounts"]) != "12.00" || text(summary.Totals["general_discounts"]) != "20.00" ||
			text(summary.Totals["courtesy"]) != "50.00" || text(summary.Totals["total_discount"]) != "82.00" {
			t.Fatalf("Resumen de descuentos: %+v", summary.Totals)
		}
	})

	t.Run("combos sin duplicar componentes y cuentas divididas y combinadas", func(t *testing.T) {
		comboLine := line(fixture.combo, 1)
		comboLine.Customization.Components = []catalog.ComponentSelection{{
			SlotID: fixture.combo.Slots[0].ID, ProductID: fixture.fries.ID, Quantity: 1,
		}}
		comboBill := fixture.account(t, "Combo", "dine_in", &fixture.table, comboLine)
		comboTotal := fixture.finance(t, comboBill.Account.ID).Total
		fixture.pay(t, comboBill.Account.ID, map[string]any{"method_id": debit, "mode": "total"})
		accounts["combo"] = comboBill
		combos := fixture.report(t, fixture.admin, "products.combos", "")
		if row := find(t, combos, "product", "Combo hamburguesa"); text(row["quantity"]) != "1" || text(row["net"]) != comboTotal {
			t.Fatalf("El combo es la venta: %+v", row)
		}
		components := fixture.report(t, fixture.admin, "products.combo_components", "")
		if row := find(t, components, "component", "Papas"); text(row["quantity"]) != "1" {
			t.Fatalf("Componente preparado: %+v", row)
		}
		for _, column := range components.Columns {
			if strings.Contains(column.Type, "money") {
				t.Fatal("Los componentes no deben tener importe propio")
			}
		}
		byProduct := fixture.report(t, fixture.admin, "sales.by_product", "")
		// Papas: solo la unidad vendida sola (40) que después se canceló (−40); el combo no la duplica.
		if row := find(t, byProduct, "product", "Papas"); text(row["quantity"]) != "0" || text(row["net"]) != "0.00" {
			t.Fatalf("Los componentes del combo no son ventas de su producto: %+v", row)
		}

		split := fixture.account(t, "Para dividir", "dine_in", &fixture.secondTable, line(fixture.burger, 2), line(fixture.soda, 2))
		burger := fixture.versionOf(t, split.Account.ID, fixture.burger)
		soda := fixture.versionOf(t, split.Account.ID, fixture.soda)
		split = fixture.refresh(t, split.Account.ID)
		pair := decode[struct {
			Target operations.Bill `json:"target"`
		}](t, fixture.mutate(t, "POST", "restaurant/accounts/"+split.Account.ID+"/split", structToMap(operations.TransferInput{
			Mutation:   operations.Mutation{Revision: split.Account.Revision},
			NewAccount: &operations.AccountInput{Name: "División"},
			Selections: []operations.Selection{{VersionID: burger, Quantity: 1}, {VersionID: soda, Quantity: 1}},
		}), 200))
		fixture.pay(t, split.Account.ID, map[string]any{"method_id": cash, "mode": "total"})
		fixture.pay(t, pair.Target.Account.ID, map[string]any{"method_id": debit, "mode": "total"})
		accounts["split"], accounts["split_target"] = split, pair.Target
		first := fixture.account(t, "Combinada A", "dine_in", &fixture.table, line(fixture.soda, 1))
		second := fixture.account(t, "Combinada B", "dine_in", &fixture.table, line(fixture.soda, 1))
		first, second = fixture.refresh(t, first.Account.ID), fixture.refresh(t, second.Account.ID)
		fixture.mutate(t, "POST", "restaurant/accounts/"+first.Account.ID+"/merge", map[string]any{
			"revision": first.Account.Revision, "target_account_id": second.Account.ID, "target_revision": second.Account.Revision,
		}, 200)
		fixture.pay(t, second.Account.ID, map[string]any{"method_id": cash, "mode": "total"})
		accounts["merged_target"] = second
		units := fixture.scalar(t, `SELECT count(*)::text FROM app.sales_recognitions r JOIN app.restaurant_item_units u ON u.id=r.unit_id
            JOIN app.restaurant_items i ON i.id=u.item_id JOIN app.restaurant_orders o ON o.id=i.original_order_id
            WHERE r.source_kind='unit' AND o.original_account_id IN ($1,$2,$3)`, split.Account.ID, first.Account.ID, second.Account.ID)
		if units != "6" {
			t.Fatalf("Cada unidad dividida o combinada se reconoce una sola vez: %s", units)
		}
		if merged := fixture.scalar(t, "SELECT count(*)::text FROM app.sales_recognitions WHERE account_id=$1", first.Account.ID); merged != "0" {
			t.Fatalf("La cuenta combinada (origen) no tiene venta propia: %s", merged)
		}
	})

	t.Run("dólares físicos y cambio por moneda con el tipo de cambio histórico", func(t *testing.T) {
		features := decode[[]struct {
			Code     string `json:"code"`
			Revision int    `json:"revision"`
		}](t, fixture.get(t, fixture.admin, "settings/features", 200))
		for _, feature := range features {
			if feature.Code == "usd_cash" {
				fixture.call(t, fixture.admin, "PUT", "settings/features", map[string]any{"code": "usd_cash", "enabled": true, "revision": feature.Revision}, 200)
			}
		}
		fixture.call(t, fixture.admin, "POST", "payments/shifts/"+fixture.shiftID+"/usd-rate", map[string]any{
			"operation_id": randomID(), "usd_rate": "18.50",
		}, 200)
		mixed := fixture.account(t, "Pesos y dólares", "dine_in", &fixture.table, line(fixture.burger, 2), line(fixture.soda, 2))
		fixture.pay(t, mixed.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received": "100", "received_usd": "20"})
		dollars := fixture.account(t, "Cambio en dólares", "dine_in", &fixture.table, line(fixture.soda, 1))
		fixture.pay(t, dollars.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received_usd": "2", "change_currency": "USD"})
		accounts["usd"], accounts["usd_change"] = mixed, dollars
		currency := fixture.report(t, fixture.admin, "payments.currency", "group_by=shift")
		if len(currency.Rows) != 1 {
			t.Fatalf("Efectivo por moneda por turno: %+v", currency.Rows)
		}
		row := currency.Rows[0]
		if text(row["usd_in"]) != "22.00" || text(row["usd_out"]) != "0.65" || text(row["usd_in_equivalent"]) != "407.00" ||
			text(row["rates"]) != "18.50" {
			t.Fatalf("Dólares físicos, cambio y equivalencia histórica: %+v", row)
		}
		for _, column := range currency.Columns {
			if column.Key == "usd_in" && column.Type != "money_usd" {
				t.Fatal("Los dólares deben ser una columna en USD")
			}
		}
	})

	t.Run("para llevar pagado sin entregar", func(t *testing.T) {
		bill := fixture.account(t, "Para llevar", "takeaway", nil, line(fixture.salad, 1))
		fixture.pay(t, bill.Account.ID, map[string]any{"method_id": debit, "mode": "total"})
		progress := fixture.report(t, fixture.admin, "sales.in_progress", "state=paid_undelivered")
		if row := find(t, progress, "folio", text(bill.Account.Folio)); text(row["status"]) != "Pagada sin entregar" {
			t.Fatalf("Pagada sin entregar: %+v", row)
		}
		if count := fixture.scalar(t, "SELECT count(*)::text FROM app.sales_recognitions WHERE account_id=$1", bill.Account.ID); count != "0" {
			t.Fatal("Un pedido pagado sin entregar no es venta todavía")
		}
		fixture.post(t, bill.Account.ID, "deliver", map[string]any{}, 200)
		takeaway := fixture.report(t, fixture.admin, "sales.takeaway", "")
		if row := find(t, takeaway, "origin", "Ventanilla"); text(row["net"]) != "50.00" || text(row["tax"]) != "3.70" {
			t.Fatalf("Venta para llevar con IVA de 8 %%: %+v", row)
		}
		accounts["takeaway"] = bill
	})

	t.Run("totales del detalle iguales a los resumidos y paginación", func(t *testing.T) {
		summary := fixture.report(t, fixture.admin, "sales.summary", "")
		net := text(summary.Totals["net"])
		for _, id := range []string{"sales.by_day", "sales.accounts", "sales.by_mode", "sales.by_price_list", "sales.by_hour", "sales.by_shift"} {
			if total := text(fixture.report(t, fixture.admin, id, "").Totals["net"]); total != net {
				t.Fatalf("%s totaliza %s y el resumen %s", id, total, net)
			}
		}
		full := fixture.report(t, fixture.admin, "sales.by_product", "page_size=200")
		pages := []reportResult{}
		for page := 1; page <= full.Page.TotalRows; page++ {
			result := fixture.report(t, fixture.admin, "sales.by_product", "page_size=1&page="+text(page))
			if len(result.Rows) != 1 || text(result.Totals["net"]) != text(full.Totals["net"]) || result.Page.TotalRows != full.Page.TotalRows {
				t.Fatalf("La paginación alteró los totales: %+v", result)
			}
			pages = append(pages, result)
		}
		names := []string{}
		for _, page := range pages {
			names = append(names, text(page.Rows[0]["product"]))
		}
		if len(names) != len(full.Rows) || full.Page.TotalRows < 5 {
			t.Fatalf("Páginas incompletas: %v", names)
		}
		// Las categorías suman lo mismo que los productos (cada unidad pertenece a una categoría).
		categories := fixture.report(t, fixture.admin, "sales.by_category", "")
		if text(categories.Totals["net"]) != text(full.Totals["net"]) || text(find(t, categories, "category", "Bebidas")["quantity"]) == "0" {
			t.Fatalf("Ventas por categoría: %+v %+v", categories.Totals, categories.Rows)
		}
		// Productos + ajustes generales de cuenta = venta neta del periodo.
		figures := map[string]string{}
		for _, figure := range full.Summary {
			figures[figure.Key] = text(figure.Value)
		}
		products := amount(t, text(full.Totals["net"]))
		reconciled := products.Sub(amount(t, figures["account_discounts"])).Add(amount(t, figures["account_charges"]))
		if reconciled.StringFixed(2) != net {
			t.Fatalf("Productos %s − descuentos generales %s + cargos %s ≠ venta %s", products, figures["account_discounts"],
				figures["account_charges"], net)
		}
		sorted := fixture.report(t, fixture.admin, "sales.by_product", "sort=quantity&direction=asc")
		for index := 1; index < len(sorted.Rows); index++ {
			if amount(t, text(sorted.Rows[index-1]["quantity"])).GreaterThan(amount(t, text(sorted.Rows[index]["quantity"]))) {
				t.Fatal("El orden ascendente no se respetó")
			}
		}
		fixture.get(t, fixture.admin, "reports/run/sales.by_product?sort=desconocida", 400)
		fixture.get(t, fixture.admin, "reports/run/sales.by_product?sql=SELECT", 400)
		fixture.get(t, fixture.admin, "reports/run/no.existe", 404)
		// El IVA de los reportes es el desglose por cuenta y tasa de finance.go.
		var expected string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT coalesce(sum(tax),0)::text FROM app.sales_recognitions`).Scan(&expected); err != nil {
			t.Fatal(err)
		}
		finances := amount(t, "0")
		for _, key := range []string{"first", "mixed", "partial", "corrected", "resettled", "discounted", "combo", "split", "split_target",
			"merged_target", "usd", "usd_change", "takeaway", "courtesy"} {
			if _, found := accounts[key]; !found {
				t.Fatalf("Falta la cuenta %s", key)
			}
			finances = finances.Add(amount(t, fixture.finance(t, accounts[key].Account.ID).TaxTotal))
		}
		refunded := fixture.finance(t, accounts["refunded"].Account.ID)
		// La cuenta reabierta conserva su reconocimiento original (hamburguesa y refresco).
		finances = finances.Add(amount(t, refunded.TaxTotal)).Add(amount(t, "3.45"))
		if finances.StringFixed(2) != expected || text(summary.Totals["tax"]) != expected {
			t.Fatalf("IVA reconocido %s, reporte %s, cuentas %s", expected, text(summary.Totals["tax"]), finances.StringFixed(2))
		}
	})

	t.Run("permisos de cada reporte aunque se llame a la API directamente", func(t *testing.T) {
		nobody := fixture.staff(t, "sin_reportes", "accounts.read")
		fixture.get(t, nobody, "reports/catalog", 403)
		fixture.get(t, nobody, "reports/dashboard", 403)
		fixture.get(t, nobody, "reports/filters/registers", 403)
		readOnly := fixture.staff(t, "solo_modulo", "reports.read")
		for _, definition := range reports.Definitions() {
			fixture.get(t, readOnly, "reports/run/"+definition.ID, 403)
		}
		catalogResult := decode[reports.Catalog](t, fixture.get(t, readOnly, "reports/catalog", 200))
		if len(catalogResult.Groups) != 0 {
			t.Fatalf("Sin permisos de grupo el catálogo no muestra reportes: %+v", catalogResult.Groups)
		}
		dashboard := decode[reports.Dashboard](t, fixture.get(t, readOnly, "reports/dashboard", 200))
		if len(dashboard.Sections) != 0 {
			t.Fatalf("Sin permisos de grupo el dashboard no calcula indicadores: %+v", dashboard.Sections)
		}
		sales := fixture.staff(t, "ventas", "reports.read", "reports.sales.read")
		payer := fixture.staff(t, "cobros", "reports.read", "reports.payments.read")
		discounter := fixture.staff(t, "descuentos", "reports.read", "reports.discounts.read")
		members := map[string]terminal{
			reports.PermissionSales: sales, reports.PermissionPayments: payer, reports.PermissionDiscounts: discounter,
		}
		for _, definition := range reports.Definitions() {
			for permission, member := range members {
				status := 403
				if len(definition.Permissions) == 1 && definition.Permissions[0] == permission {
					status = 200
				}
				fixture.get(t, member, "reports/run/"+definition.ID, status)
			}
		}
		// Sin cash.expected.read no se envían el efectivo físico ni sus totales, ni se puede ordenar por él.
		list := fixture.report(t, payer, "payments.list", "")
		for _, column := range list.Columns {
			if column.Key == "received" || column.Key == "change" || column.Key == "received_usd" || column.Key == "usd_rate" {
				t.Fatalf("Columna protegida enviada sin cash.expected.read: %s", column.Key)
			}
		}
		for _, row := range append(list.Rows, list.Totals) {
			if _, found := row["received"]; found {
				t.Fatal("Valor protegido enviado sin cash.expected.read")
			}
		}
		fixture.get(t, payer, "reports/run/payments.list?sort=received", 400)
		if full := fixture.report(t, fixture.admin, "payments.list", ""); !hasColumn(full, "received") {
			t.Fatal("El administrador sí ve el efectivo físico")
		}
		salesDashboard := decode[reports.Dashboard](t, fixture.get(t, sales, "reports/dashboard", 200))
		if len(salesDashboard.Sections) != 1 || salesDashboard.Sections[0].ID != "sales" {
			t.Fatalf("El dashboard solo muestra las secciones permitidas: %+v", salesDashboard.Sections)
		}
		for _, indicator := range salesDashboard.Sections[0].Indicators {
			if indicator.ReportID != "" && !strings.HasPrefix(indicator.ReportID, "sales.") && !strings.HasPrefix(indicator.ReportID, "products.") {
				t.Fatalf("Indicador con vínculo a un reporte no permitido: %+v", indicator)
			}
		}
		salesCatalog := decode[reports.Catalog](t, fixture.get(t, sales, "reports/catalog", 200))
		for _, group := range salesCatalog.Groups {
			if group.ID == "payments" || group.ID == "discounts" {
				t.Fatalf("El catálogo mostró un grupo sin permiso: %s", group.ID)
			}
			for _, report := range group.Reports {
				if report.Export.PDF || report.Export.Excel {
					t.Fatal("Sin permisos de exportación el catálogo no ofrece exportar")
				}
			}
		}
		// Las denegaciones quedan auditadas.
		if count := fixture.scalar(t, "SELECT count(*)::text FROM app.audit_events WHERE action='access.denied'"); count == "0" {
			t.Fatal("Las denegaciones de reportes no se auditaron")
		}
	})

	t.Run("dashboard con comparación y filtros", func(t *testing.T) {
		dashboard := decode[reports.Dashboard](t, fixture.get(t, fixture.admin, "reports/dashboard?period=today", 200))
		values := map[string]reports.Indicator{}
		for _, section := range dashboard.Sections {
			for _, indicator := range section.Indicators {
				values[indicator.Key] = indicator
			}
		}
		net := text(fixture.report(t, fixture.admin, "sales.summary", "").Totals["net"])
		if text(values["net_sales"].Value) != net || text(values["net_sales"].Previous) != "0.00" || values["net_sales"].ChangePercent != nil {
			t.Fatalf("Ventas netas del dashboard: %+v", values["net_sales"])
		}
		if text(values["tips"].Value) != "10.00" || text(values["courtesies"].Value) != "50.00" || text(values["open_accounts"].Value) != "2" {
			t.Fatalf("Indicadores del dashboard: %+v", values)
		}
		if dashboard.Comparison == nil || dashboard.Comparison.FirstDay == dashboard.Period.FirstDay || len(dashboard.Series) != 1 {
			t.Fatalf("Comparación con el día anterior y serie de tendencia: %+v", dashboard)
		}
		options := decode[struct {
			Options []reports.Option `json:"options"`
		}](t, fixture.get(t, fixture.admin, "reports/filters/discount_reasons", 200))
		if len(options.Options) < 2 || options.Options[0].Value != "none" || options.Options[0].Label != "Sin motivo del catálogo" {
			t.Fatalf("Opciones del filtro de motivos: %+v", options.Options)
		}
		fixture.get(t, fixture.admin, "reports/filters/inexistente", 404)
		products := decode[struct {
			Options []reports.Option `json:"options"`
		}](t, fixture.get(t, fixture.admin, "reports/filters/products?search=refr", 200))
		if len(products.Options) != 1 || products.Options[0].Label != "Refresco" {
			t.Fatalf("Búsqueda de productos: %+v", products.Options)
		}
		filtered := fixture.report(t, fixture.admin, "sales.by_product", "category_id="+fixture.drinks.ID)
		if len(filtered.Rows) != 1 || text(filtered.Rows[0]["product"]) != "Refresco" || len(filtered.Meta.Filters) != 1 ||
			filtered.Meta.Filters[0].Display != "Bebidas" {
			t.Fatalf("Filtro por categoría legible: %+v", filtered)
		}
		week := fixture.report(t, fixture.admin, "sales.by_week", "period=iso_week&week=2026-W53")
		if week.Meta.Period == nil || week.Meta.Period.FirstDay != "2026-12-28" || week.Meta.Period.LastDay != "2027-01-03" {
			t.Fatalf("Semana ISO 53 de 2026: %+v", week.Meta.Period)
		}
	})

	t.Run("REPORTS_BUSY con el semáforo lleno y cobro sin esperar a un reporte", func(t *testing.T) {
		runner := reports.NewRunner(fixture.pool, reports.Slots)
		fixture.service.Reports = runner
		first, ok := runner.Acquire()
		second, okSecond := runner.Acquire()
		if !ok || !okSecond {
			t.Fatal("El semáforo debía admitir dos reportes")
		}
		code, body := fixture.request(fixture.admin, "GET", "reports/run/sales.summary", nil)
		problem := decode[struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}](t, body)
		if code != 503 || problem.Error.Code != "REPORTS_BUSY" || !strings.Contains(problem.Error.Message, "Hay otros reportes en curso") {
			t.Fatalf("Con el semáforo lleno se esperaba 503 REPORTS_BUSY: %d %s", code, body)
		}
		fixture.get(t, fixture.admin, "reports/dashboard", 503)
		first()
		second()
		fixture.get(t, fixture.admin, "reports/run/sales.summary", 200)
		// Un reporte en curso (transacción de solo lectura abierta) no frena un cobro y no ve lo
		// que se confirma después de su fotografía.
		started, finish := make(chan string, 1), make(chan struct{})
		done := make(chan error, 1)
		finished := false
		release := func() {
			if !finished {
				finished = true
				close(finish)
			}
		}
		// Si la prueba falla antes, el reporte en curso termina igualmente.
		defer release()
		go func() {
			done <- runner.Read(context.Background(), func(tx pgx.Tx) error {
				var before string
				if err := tx.QueryRow(context.Background(), "SELECT count(*)::text FROM app.sales_recognitions").Scan(&before); err != nil {
					return err
				}
				started <- before
				<-finish
				var after string
				if err := tx.QueryRow(context.Background(), "SELECT count(*)::text FROM app.sales_recognitions").Scan(&after); err != nil {
					return err
				}
				if after != before {
					return &reports.Error{Code: "SNAPSHOT", Message: "La fotografía cambió: " + before + " → " + after}
				}
				return nil
			})
		}()
		before := <-started
		begin := time.Now()
		bill := fixture.account(t, "Durante un reporte", "dine_in", &fixture.table, line(fixture.soda, 1))
		if !fixture.pay(t, bill.Account.ID, map[string]any{"method_id": debit, "mode": "total"}).Closed {
			t.Fatal("El cobro concurrente no cerró la cuenta")
		}
		accounts["concurrent"] = bill
		if elapsed := time.Since(begin); elapsed > 5*time.Second {
			t.Fatalf("El cobro esperó al reporte: %s", elapsed)
		}
		if now := fixture.scalar(t, "SELECT count(*)::text FROM app.sales_recognitions"); now == before {
			t.Fatal("El cobro debía reconocer su venta")
		}
		release()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})

	t.Run("relleno de la 0017 equivalente al cálculo de Go", func(t *testing.T) {
		compareBackfill(t, fixture)
	})

	t.Run("hora de inicio del día: 00:00 y 05:00 con una venta de la 01:30", func(t *testing.T) {
		// Venta sintética de la 01:30 del 15 de enero de 2025 (hora de Monterrey) sobre una unidad existente.
		unit := fixture.scalar(t, `SELECT unit_id::text FROM app.sales_recognitions WHERE account_id=$1 AND source_kind='unit' LIMIT 1`,
			accounts["first"].Account.ID)
		if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.sales_recognitions(closing_id,recognition_kind,recognized_at,account_id,
                account_folio,mode,origin,source_kind,unit_id,tax_rate,quantity,gross,promotion,discount,courtesy,charge,net,base,tax)
            SELECT gen_random_uuid(),'resettlement','2025-01-15T01:30:00-06:00',account_id,account_folio,mode,origin,'unit',unit_id,16,
                0,100,0,0,0,0,100,86.21,13.79
            FROM app.sales_recognitions WHERE unit_id=$1 LIMIT 1`, unit); err != nil {
			t.Fatal(err)
		}
		netOn := func(date string) string {
			return text(fixture.report(t, fixture.admin, "sales.summary", "period=day&date="+date).Totals["net"])
		}
		if netOn("2025-01-14") != "100.00" || netOn("2025-01-15") != "0.00" {
			t.Fatal("Con inicio a las 05:00 la venta de la 01:30 es del día anterior")
		}
		settings := decode[map[string]any](t, fixture.get(t, fixture.admin, "settings/business-day", 200))
		if text(settings["operating_day_cutoff"]) != "05:00" {
			t.Fatalf("Hora de inicio por omisión: %+v", settings)
		}
		viewer := fixture.staff(t, "configura_no", "settings.read", "inventory.read", "inventory.settings.manage")
		fixture.call(t, viewer, "PUT", "settings/business-day", map[string]any{"operating_day_cutoff": "00:00", "revision": settings["revision"]}, 403)
		inventory := decode[map[string]any](t, fixture.get(t, fixture.admin, "inventory/settings", 200))
		fixture.call(t, fixture.admin, "PUT", "inventory/settings", map[string]any{
			"operation_id": randomID(), "revision": inventory["revision"], "purchase_tax_in_cost": true, "operating_day_cutoff": "00:00",
			"warn_products_without_recipe": true, "count_frequency": "none",
		}, 400)
		fixture.call(t, fixture.admin, "PUT", "settings/business-day", map[string]any{"operating_day_cutoff": "00:00", "revision": settings["revision"]}, 200)
		if netOn("2025-01-14") != "0.00" || netOn("2025-01-15") != "100.00" {
			t.Fatal("Con inicio a las 00:00 la venta de la 01:30 es del mismo día")
		}
		if recorded := fixture.scalar(t, `SELECT recognized_at::text FROM app.sales_recognitions WHERE recognized_at='2025-01-15T07:30:00Z'`); recorded == "" {
			t.Fatal("Cambiar la hora de inicio no debe reescribir registros")
		}
		if fixture.scalar(t, "SELECT count(*)::text FROM app.audit_events WHERE action='settings.business_day_changed'") != "1" {
			t.Fatal("El cambio de la hora de inicio no quedó auditado")
		}
		fixture.call(t, fixture.admin, "PUT", "settings/business-day", map[string]any{"operating_day_cutoff": "05:00", "revision": settings["revision"]}, 409)
		latest := decode[map[string]any](t, fixture.get(t, fixture.admin, "settings/business-day", 200))
		fixture.call(t, fixture.admin, "PUT", "settings/business-day", map[string]any{"operating_day_cutoff": "25:00", "revision": latest["revision"]}, 400)
		fixture.call(t, fixture.admin, "PUT", "settings/business-day", map[string]any{"operating_day_cutoff": "05:00", "revision": latest["revision"]}, 200)
	})
}

func hasColumn(result reportResult, key string) bool {
	for _, column := range result.Columns {
		if column.Key == key {
			return true
		}
	}
	return false
}

func mustDate(t *testing.T, text string) time.Time {
	t.Helper()
	date, err := time.Parse("2006-01-02", text)
	if err != nil {
		t.Fatal(err)
	}
	return date
}

// compareBackfill ejecuta el bloque de relleno de la migración 0017 sobre una tabla temporal y lo
// compara con lo que escribió Go para las mismas cuentas cerradas: la suma por fuente (clase,
// unidad, ajuste y tasa) de cada importe y los datos históricos deben coincidir.
func compareBackfill(t *testing.T, fixture *reportsFixture) {
	t.Helper()
	content, err := fs.ReadFile(migrations.Files, "0017_reports.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	start := strings.Index(sql, "-- reconocimiento:relleno:inicio")
	end := strings.Index(sql, "-- reconocimiento:relleno:fin")
	if start < 0 || end < start {
		t.Fatal("La migración no delimita su relleno")
	}
	block := strings.Replace(sql[start:end], "INSERT INTO app.sales_recognitions(", "INSERT INTO pg_temp.backfill_probe(", 1)
	connection, err := fixture.pool.Acquire(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err := connection.Exec(fixture.ctx, `CREATE TEMP TABLE backfill_probe (LIKE app.sales_recognitions INCLUDING DEFAULTS INCLUDING IDENTITY)`); err != nil {
		t.Fatal(err)
	}
	defer connection.Exec(fixture.ctx, "DROP TABLE IF EXISTS pg_temp.backfill_probe")
	if _, err := connection.Exec(fixture.ctx, block); err != nil {
		t.Fatal(err)
	}
	summarize := func(table string) map[string]string {
		rows, err := connection.Query(fixture.ctx, `SELECT source_kind||'|'||coalesce(unit_id::text,'')||'|'||coalesce(adjustment_id::text,'')||'|'||tax_rate,
                sum(quantity)||'|'||sum(gross)||'|'||sum(promotion)||'|'||sum(discount)||'|'||sum(courtesy)||'|'||sum(charge)||'|'||
                sum(net)||'|'||sum(base)||'|'||sum(tax)||'|'||coalesce(max(product_name),'')||'|'||coalesce(max(category_name),'')||'|'||
                coalesce(max(variant_name),'')||'|'||coalesce(max(price_list_name),'')||'|'||coalesce(max(reason_name),'')
            FROM `+table+` r
            WHERE r.account_id IN (SELECT id FROM app.restaurant_accounts WHERE state='closed')
            GROUP BY 1 HAVING sum(quantity) <> 0 OR sum(gross) <> 0 OR sum(discount) <> 0 OR sum(courtesy) <> 0 OR sum(charge) <> 0
                OR sum(net) <> 0`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		result := map[string]string{}
		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				t.Fatal(err)
			}
			result[key] = value
		}
		return result
	}
	written := summarize("app.sales_recognitions")
	backfilled := summarize("pg_temp.backfill_probe")
	for key, value := range written {
		if _, found := backfilled[key]; !found {
			t.Errorf("Solo Go reconoció %s = %s", key, value)
		}
	}
	for key, value := range backfilled {
		if _, found := written[key]; !found {
			t.Errorf("Solo el relleno reconoció %s = %s", key, value)
		}
	}
	if len(written) < 20 || len(written) != len(backfilled) {
		t.Fatalf("Fuentes: Go %d, relleno %d", len(written), len(backfilled))
	}
	keys := []string{}
	for key := range written {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if written[key] != backfilled[key] {
			t.Fatalf("La fuente %s difiere: Go %s, relleno %s", key, written[key], backfilled[key])
		}
	}
}

func amount(t *testing.T, value string) decimal.Decimal {
	t.Helper()
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		t.Fatalf("Importe inválido %q", value)
	}
	return parsed
}
