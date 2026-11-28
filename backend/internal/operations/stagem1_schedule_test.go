//go:build integration

package operations_test

import (
	"testing"
	"time"

	"local/ad-el-pos/server/internal/operations"
)

// Etapa M1 (Fase A, §5.3; decisión del propietario M1-D17, reemplaza K11-D9): quitar la hora a un pedido programado lo
// envía a cocina en ese momento, en la misma transacción, como un pedido sin hora. Antes quedaba «Por enviar a cocina»
// hasta tocar «Enviar a cocina ahora». Cambiar la hora a otra sigue reprogramando sin enviar. Usa su propia base desde
// cero.
func TestStageM1RemovingScheduleSends(t *testing.T) {
	fixture := newStageM1Fixture(t)
	location, err := time.LoadLocation(fixture.timezone(t))
	if err != nil {
		t.Fatal(err)
	}
	scheduled := time.Now().In(location).Add(3 * time.Hour).Truncate(time.Minute)
	opened := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
		"name": "Programado M1", "mode": "takeaway", "origin": "Teléfono",
		"scheduled_local": scheduled.Format("2006-01-02T15:04"), "schedule_kind": "preparation",
		"items": []operations.DraftItem{draftLine(fixture.hamburguesa, 1)},
	}, 200))
	bill := fixture.confirm(t, fixture.admin, opened.Account.ID)
	releaseID, state, _ := fixture.release(t, bill.Account.ID)
	if state != "scheduled" || card(t, fixture.orders(t, fixture.display), bill.Account.ID) != nil {
		t.Fatalf("el pedido programado ya llegó a cocina: %s", state)
	}
	// Otra hora: sigue programado.
	account := fixture.refresh(t, bill.Account.ID).Account
	update := accountUpdate(account)
	update["scheduled_local"] = scheduled.Add(time.Hour).Format("2006-01-02T15:04")
	fixture.mutate(t, "PATCH", "restaurant/accounts/"+account.ID, update, 200)
	if _, state, _ := fixture.release(t, bill.Account.ID); state != "scheduled" {
		t.Fatalf("reprogramar lo envió: %s", state)
	}
	// Sin hora: se envía en ese momento.
	account = fixture.refresh(t, bill.Account.ID).Account
	update = accountUpdate(account)
	update["scheduled_local"] = ""
	update["schedule_kind"] = ""
	saved := decodeResponse[operations.Bill](t, fixture.mutate(t, "PATCH", "restaurant/accounts/"+account.ID, update, 200))
	if id, state, _ := fixture.release(t, bill.Account.ID); id != releaseID || state != "released" {
		t.Fatalf("quitar la hora no envió el pedido: %s", state)
	}
	if saved.Account.PreparationState != "released" || saved.Account.ScheduledAt != nil {
		t.Fatalf("la cuenta sin hora: %s %v", saved.Account.PreparationState, saved.Account.ScheduledAt)
	}
	if card(t, fixture.orders(t, fixture.display), bill.Account.ID) == nil {
		t.Fatal("la pantalla de cocina no recibió el pedido")
	}
	if releases, _ := fixture.onBoard(t, bill.Account.ID); releases != 0 {
		t.Fatal("el pedido sigue en «Por enviar a cocina»")
	}
}
