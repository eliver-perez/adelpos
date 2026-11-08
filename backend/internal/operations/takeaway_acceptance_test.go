//go:build integration

package operations_test

import (
	"net/url"
	"strconv"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
)

func testTakeaway(t *testing.T, fixture *restaurantFixture) {
	t.Run("para llevar ventanilla programación estados independientes y antigüedad", func(t *testing.T) {
		location, _ := time.LoadLocation("America/Monterrey")
		scheduled := time.Now().In(location).Add(2 * time.Hour).Format("2006-01-02T15:04")
		bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "Pedido de prueba", "mode": "takeaway", "customer_name": "Cliente temporal", "phone": "5550001234",
			"origin": "Ventanilla", "notes": "Recoger por ventanilla", "scheduled_local": scheduled, "schedule_kind": "delivery",
		}, 200))
		if bill.Account.TableID != nil || bill.Account.ScheduledLocal != scheduled {
			t.Fatal("Para llevar utilizó una mesa o perdió la programación local")
		}
		bill = fixture.capture(t, bill, []catalog.LineRequest{{ProductID: fixture.product.ID, Quantity: 1}})
		if bill.Account.PreparationState != "scheduled" || bill.Account.PaymentState != "not_recorded" || bill.Account.DeliveryState != "not_delivered" {
			t.Fatal("Se simularon estados de cocina, cobro o entrega")
		}
		version := bill.Lines[0]
		version.Request.Customization.Notes = "Empacar por separado"
		edit := map[string]any{"revision": bill.Account.Revision, "version_id": version.VersionID, "line": version.Request, "reason": "Indicaciones de empaque"}
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/edit-preview", edit, 200))
		edit["expected_fingerprint"] = preview.Fingerprint
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/edit", edit, 200))
		if bill.Lines[0].Request.Customization.Notes != "Empacar por separado" {
			t.Fatal("No se conservó la modificación del pedido para llevar")
		}
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.restaurant_accounts SET created_at=now()-interval '60 minutes' WHERE id=$1", bill.Account.ID); err != nil {
			t.Fatal(err)
		}
		fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": "Más reciente", "mode": "takeaway", "origin": "Mostrador"}, 200)
		settings := decodeResponse[operations.Settings](t, fixture.adminCall(t, "GET", "restaurant/settings", nil, 200))
		settings.WarningMinutes, settings.OverdueMinutes = 10, 20
		fixture.mutate(t, "PUT", "restaurant/settings", settings, 200)
		board := decodeResponse[struct {
			Items []operations.Account `json:"items"`
		}](t, fixture.adminCall(t, "GET", "restaurant/accounts?mode=takeaway", nil, 200))
		if len(board.Items) < 2 || board.Items[0].ID != bill.Account.ID || board.Items[0].ElapsedSeconds < 3600 || board.Items[0].Urgency != "overdue" {
			t.Fatal("El tablero no priorizó antigüedad y atraso calculados por servidor")
		}
		for _, search := range []string{"Cliente temporal", "5550001234", strconv.FormatInt(bill.Account.Folio, 10)} {
			result := decodeResponse[struct {
				Items []operations.Account `json:"items"`
			}](t, fixture.adminCall(t, "GET", "restaurant/accounts?mode=takeaway&search="+url.QueryEscape(search), nil, 200))
			if len(result.Items) != 1 || result.Items[0].ID != bill.Account.ID {
				t.Fatal("No se localizó el pedido por cliente o teléfono")
			}
		}
		fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/deliver", map[string]any{"revision": bill.Account.Revision}, 404)
		updated := fixture.refresh(t, bill.Account.ID)
		if updated.Account.DeliveryState != "not_delivered" {
			t.Fatal("Se habilitó una entrega sin liquidación")
		}
	})
}
