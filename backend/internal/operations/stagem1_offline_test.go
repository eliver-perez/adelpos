//go:build integration

package operations_test

import (
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/operations"
)

// Etapa M1 (Fase A, M1-D13 y la tabla del contrato §7): un pedido de la cola sin conexión de Android que se capturó en
// una mesa y llega cuando la mesa ya está en una unión va a la unión; si la unión que vio el mesero ya no existe y la
// mesa está libre, va a la mesa. Sin tipos de cambio nuevos: table_has_account e items_in_account, como en la 0.4.1.
// Usa su propia base desde cero.
func TestStageM1OfflineQueue(t *testing.T) {
	fixture := newStageM1Fixture(t)
	t.Cleanup(func() { expectStageM1Invariants(t, fixture.restaurantFixture) })
	submit := func(t *testing.T, destination map[string]any, status int, items ...map[string]any) (operations.OfflineOrderResult,
		offlineRejection, map[string]any) {
		t.Helper()
		if len(items) == 0 {
			items = []map[string]any{offlineItem(fixture.coca.ID, 1, "35.00")}
		}
		body := offlineBody(randomID(), fixture.catalogVersion(t), destination, items...)
		response := fixture.call(t, fixture.joiner, "POST", "restaurant/offline-orders", body, status)
		if status == 200 {
			return decodeResponse[operations.OfflineOrderResult](t, response), offlineRejection{}, body
		}
		return operations.OfflineOrderResult{}, decodeResponse[offlineRejection](t, response), body
	}
	confirmWith := func(t *testing.T, body map[string]any, rejection offlineRejection) operations.OfflineOrderResult {
		t.Helper()
		body["confirmation"] = map[string]any{"catalog_version": rejection.Error.Details.CatalogVersion,
			"changes": rejection.Error.Details.Changes}
		return decodeResponse[operations.OfflineOrderResult](t, fixture.call(t, fixture.joiner, "POST", "restaurant/offline-orders",
			body, 200))
	}
	settle := func(t *testing.T, accountIDs ...string) {
		t.Helper()
		for _, id := range accountIDs {
			if fixture.refresh(t, id).Account.DraftQuantity > 0 {
				fixture.confirm(t, fixture.joiner, id)
			}
			fixture.pay(t, id)
		}
	}

	t.Run("la mesa ahora está en una unión con cuentas que el mesero no veía", func(t *testing.T) {
		bill := fixture.deferred(t, "Mesa 5", "Mesa 6")
		_, rejection, body := submit(t, map[string]any{"table_id": fixture.id("Mesa 6"), "seen_account_ids": []string{}}, 409)
		changes := rejection.Error.Details.Changes
		if rejection.Error.Code != "ORDER_REQUIRES_CONFIRMATION" || len(changes) != 1 || changes[0].Kind != "table_has_account" ||
			!changes[0].Acceptable || changes[0].Message != "La Mesa 6 ahora está unida con otras mesas (Mesa 5 + 6) y tiene "+
			"cuentas abiertas. Confirma para abrir otra cuenta en la unión o envía el pedido a esa cuenta." {
			t.Fatalf("pedido a una mesa unida: %+v", rejection.Error)
		}
		accepted := confirmWith(t, body, rejection)
		if accepted.Bill.Account.TableUnion == nil || accepted.Bill.Account.TableUnion.ID != bill.Account.TableUnion.ID ||
			accepted.Bill.Account.Name != "Mesa 5 + 6" {
			t.Fatalf("la cuenta nueva no quedó en la unión: %+v", accepted.Bill.Account)
		}
		// Lo ya recibido en una cuenta de la unión: items_in_account con esa cuenta, aunque esté en la otra mesa.
		item := offlineItem(fixture.coca.ID, 1, "35.00")
		first, _, _ := submit(t, map[string]any{"account_id": bill.Account.ID}, 200, item)
		_, rejection, _ = submit(t, map[string]any{"table_id": fixture.id("Mesa 6"), "seen_account_ids": []string{
			bill.Account.ID, accepted.AccountID}, "seen_union_id": bill.Account.TableUnion.ID}, 409, item)
		changes = rejection.Error.Details.Changes
		if len(changes) != 1 || changes[0].Kind != "items_in_account" || changes[0].AccountID == nil ||
			*changes[0].AccountID != first.AccountID {
			t.Fatalf("partidas ya recibidas en la unión: %+v", changes)
		}
		// Con la unión y todas sus cuentas vistas: cuenta nueva en la unión sin pedir confirmación.
		third, _, _ := submit(t, map[string]any{"table_id": fixture.id("Mesa 5"), "seen_account_ids": []string{bill.Account.ID,
			accepted.AccountID}, "seen_union_id": bill.Account.TableUnion.ID}, 200)
		if third.Bill.Account.TableUnion == nil || third.Bill.Account.TableUnion.ID != bill.Account.TableUnion.ID {
			t.Fatalf("la tercera cuenta: %+v", third.Bill.Account)
		}
		settle(t, bill.Account.ID, accepted.AccountID, third.AccountID)
	})

	t.Run("la unión que vio el mesero ya no existe y la mesa está libre", func(t *testing.T) {
		bill := fixture.deferred(t, "Mesa 5", "Mesa 6")
		seen := bill.Account.TableUnion.ID
		settle(t, bill.Account.ID)
		result, _, _ := submit(t, map[string]any{"table_id": fixture.id("Mesa 6"), "seen_account_ids": []string{bill.Account.ID},
			"seen_union_id": seen}, 200)
		if result.Bill.Account.TableUnion != nil || *result.Bill.Account.TableID != fixture.id("Mesa 6") ||
			result.Bill.Account.Name != "Mesa 6" {
			t.Fatalf("cuenta nueva en la mesa: %+v", result.Bill.Account)
		}
		settle(t, result.AccountID)
	})

	t.Run("se deshizo y otra cuenta ocupó la mesa", func(t *testing.T) {
		bill := fixture.deferred(t, "Mesa 5", "Mesa 6")
		seen := bill.Account.TableUnion.ID
		settle(t, bill.Account.ID)
		other := fixture.sent(t, "Mesa 6", draftLine(fixture.coca, 1))
		_, rejection, _ := submit(t, map[string]any{"table_id": fixture.id("Mesa 6"), "seen_account_ids": []string{bill.Account.ID},
			"seen_union_id": seen}, 409)
		changes := rejection.Error.Details.Changes
		if len(changes) != 1 || changes[0].Kind != "table_has_account" || !strings.HasPrefix(changes[0].Message, "La mesa «Mesa 6» ya tiene una cuenta abierta en otra terminal") {
			t.Fatalf("mesa ocupada después de deshacerse: %+v", changes)
		}
		settle(t, other.Account.ID)
	})

	t.Run("la mesa está en otra unión distinta de la que vio", func(t *testing.T) {
		first := fixture.deferred(t, "Mesa 5", "Mesa 6")
		seen := first.Account.TableUnion.ID
		settle(t, first.Account.ID)
		second := fixture.deferred(t, "Mesa 6", "Mesa 7")
		_, rejection, _ := submit(t, map[string]any{"table_id": fixture.id("Mesa 6"), "seen_account_ids": []string{},
			"seen_union_id": seen}, 409)
		changes := rejection.Error.Details.Changes
		if len(changes) != 1 || changes[0].Kind != "table_has_account" || changes[0].Message != "La Mesa 6 ahora está unida "+
			"con otras mesas (Mesa 6 + 7) y tiene cuentas abiertas. Confirma para abrir otra cuenta en la unión o envía el pedido a esa cuenta." {
			t.Fatalf("otra unión: %+v", changes)
		}
		settle(t, second.Account.ID)
	})

	t.Run("la mesa salió de la unión y quedó libre", func(t *testing.T) {
		bill := fixture.deferred(t, "Mesa 5", "Mesa 6", "Mesa 7")
		union := fixture.union(t, "Mesa 5")
		fixture.call(t, fixture.joiner, "POST", "restaurant/table-unions/"+union.ID+"/tables/"+fixture.id("Mesa 7")+"/remove",
			map[string]any{"operation_id": randomID(), "revision": union.Revision}, 200)
		result, _, _ := submit(t, map[string]any{"table_id": fixture.id("Mesa 7"), "seen_account_ids": []string{bill.Account.ID},
			"seen_union_id": union.ID}, 200)
		if result.Bill.Account.TableUnion != nil || *result.Bill.Account.TableID != fixture.id("Mesa 7") {
			t.Fatalf("cuenta nueva en la mesa que salió: %+v", result.Bill.Account)
		}
		settle(t, bill.Account.ID, result.AccountID)
	})

	t.Run("destino account_id: a esa cuenta esté donde esté, o account_closed", func(t *testing.T) {
		opened := fixture.sent(t, "Mesa 9", draftLine(fixture.coca, 1))
		fixture.joinNow(t, "Mesa 9", "Mesa 10")
		result, _, _ := submit(t, map[string]any{"account_id": opened.Account.ID}, 200)
		if result.AccountID != opened.Account.ID {
			t.Fatalf("al destino de la cuenta: %+v", result)
		}
		settle(t, opened.Account.ID)
		_, rejection, _ := submit(t, map[string]any{"account_id": opened.Account.ID}, 409)
		if len(rejection.Error.Details.Changes) != 1 || rejection.Error.Details.Changes[0].Kind != "account_closed" {
			t.Fatalf("cuenta cobrada: %+v", rejection.Error)
		}
	})
}
