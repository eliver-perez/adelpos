//go:build integration

package reports_test

import (
	"context"
	"testing"

	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa M1 (Fase A, §1.4 y contrato §8; M1-D3 y M1-D14): los reportes con la unión «Mesa 1 + 3»:
//   - «En curso» muestra la unión como lugar de su cuenta abierta;
//   - «Ventas de salón» la muestra en un solo renglón con el nombre de la unión, sin repartir la venta entre sus mesas;
//   - el filtro de la Mesa 3 incluye la venta de la unión (en la que estuvo la mesa), aunque la cuenta estaba en la Mesa 1;
//   - «Cuentas canceladas» dice la unión que tenía la cuenta al cancelarse, aunque después se haya deshecho.
//
// Usa su propia base desde cero y la deja vacía al terminar.
func TestStageM1UnionReports(t *testing.T) {
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
	// La Mesa 3 en el Salón (la Mesa 2 de la base de reportes está en la Terraza): una unión «Mesa 1 + 3» y otra cuenta
	// sola en la Mesa 3 después.
	other := decode[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", structToMap(operations.Table{
		AreaID: fixture.area.ID, Name: "Mesa 3", Shape: "rectangle", X: 300, Y: 30, Width: 100, Height: 72, Seats: 4, Active: true,
	}), 200))
	union := func(start, other operations.Table) operations.Bill {
		t.Helper()
		bill := fixture.account(t, "", "dine_in", &start, line(fixture.burger, 1))
		fixture.mutate(t, "POST", "restaurant/table-unions", map[string]any{"start_table_id": start.ID,
			"table_ids": []string{other.ID}}, 200)
		return fixture.refresh(t, bill.Account.ID)
	}
	joined := union(fixture.table, other)
	if joined.Account.Name != "Mesa 1 + 3" {
		t.Fatalf("la cuenta de la unión: %s", joined.Account.Name)
	}
	progress := fixture.report(t, fixture.admin, "sales.in_progress", "")
	if row := find(t, progress, "folio", text(joined.Account.Folio)); row["place"] != "Mesa 1 + 3" {
		t.Fatalf("«En curso»: %v", row)
	}
	fixture.pay(t, joined.Account.ID, map[string]any{"method_id": fixture.methods["cash"], "mode": "total", "received": "500"})
	alone := fixture.account(t, "", "dine_in", &other, line(fixture.soda, 1))
	fixture.pay(t, alone.Account.ID, map[string]any{"method_id": fixture.methods["cash"], "mode": "total", "received": "500"})

	dineIn := fixture.report(t, fixture.admin, "sales.dine_in", "period=today")
	unionRow := find(t, dineIn, "table", "Mesa 1 + 3")
	single := find(t, dineIn, "table", "Mesa 3")
	if text(unionRow["accounts"]) != "1" || text(single["accounts"]) != "1" || len(dineIn.Rows) != 2 {
		t.Fatalf("«Ventas de salón»: %+v", dineIn.Rows)
	}
	if text(unionRow["net"]) != text(joined.Account.Totals.Total) {
		t.Fatalf("la venta de la unión se repartió: %v (total %s)", unionRow["net"], joined.Account.Totals.Total)
	}
	// Con el filtro de la Mesa 3: su venta sola y la de la unión en la que estuvo.
	filtered := fixture.report(t, fixture.admin, "sales.dine_in", "period=today&table_id="+other.ID)
	if len(filtered.Rows) != 2 {
		t.Fatalf("filtro de la Mesa 3: %+v", filtered.Rows)
	}
	// Con el de la Mesa 1, solo la unión.
	first := fixture.report(t, fixture.admin, "sales.dine_in", "period=today&table_id="+fixture.table.ID)
	if len(first.Rows) != 1 || first.Rows[0]["table"] != "Mesa 1 + 3" {
		t.Fatalf("filtro de la Mesa 1: %+v", first.Rows)
	}

	// Una cuenta cancelada en una unión: la unión se deshace, el reporte conserva su nombre.
	cancelled := union(other, fixture.table)
	reasons := decode[[]operations.CancelReason](t, fixture.get(t, fixture.admin, "restaurant/cancel-reasons", 200))
	fixture.mutate(t, "POST", "restaurant/accounts/"+cancelled.Account.ID+"/cancel-account", map[string]any{
		"revision": cancelled.Account.Revision, "reason_id": reasons[0].ID, "description": "Prueba M1"}, 200)
	if open := fixture.scalar(t, "SELECT count(*)::text FROM app.table_unions WHERE state='open'"); open != "0" {
		t.Fatalf("uniones abiertas después de cancelar: %s", open)
	}
	voided := fixture.report(t, fixture.admin, "sales.voided_accounts", "period=today")
	if row := find(t, voided, "folio", text(cancelled.Account.Folio)); row["place"] != "Mesa 1 + 3" {
		t.Fatalf("«Cuentas canceladas»: %v", row)
	}
}
