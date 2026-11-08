//go:build integration

package operations_test

import (
	"testing"

	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
)

func testKitchenFrozenChanges(t *testing.T, fixture *restaurantFixture, bill operations.Bill, first, second kitchen.Station) {
	t.Run("recepción KDS persistida sin marcar impresión ni preparación", func(t *testing.T) {
		var ticketID string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT id FROM app.kitchen_tickets
			WHERE $1=ANY(station_ids) AND kind='normal' ORDER BY created_at LIMIT 1`, second.ID).Scan(&ticketID); err != nil {
			t.Fatal(err)
		}
		for attempt := 0; attempt < 2; attempt++ {
			fixture.mutate(t, "POST", "kitchen/tickets/"+ticketID+"/ack", map[string]string{
				"station_id": second.ID,
			}, 200)
		}
		if kitchenCount(t, fixture, "kitchen_kds_receipts") != 1 {
			t.Fatal("El reintento duplicó la recepción KDS")
		}
		fixture.mutate(t, "POST", "kitchen/tickets/"+ticketID+"/ack", map[string]string{
			"station_id": first.ID,
		}, 400)
	})
	t.Run("ruta histórica y modificación bloqueada conservan el aviso hasta resolver", func(t *testing.T) {
		bill = fixture.refresh(t, bill.Account.ID)
		releaseID := kitchenReleases(t, fixture, bill)[0].ID
		routes := decodeResponse[[]kitchen.Route](t, fixture.adminCall(t, "GET", "kitchen/routes", nil, 200))
		for _, route := range routes {
			if route.ProductID != nil && *route.ProductID == bill.Lines[0].Request.ProductID {
				route.StationIDs = []string{first.ID}
				fixture.mutate(t, "PATCH", "kitchen/routes/"+route.ID, route, 200)
			}
		}
		second.Active = false
		second = decodeResponse[kitchen.Station](t, fixture.mutate(t, "PATCH", "kitchen/stations/"+second.ID, second, 200))
		line := bill.Lines[0]
		line.Request.Customization.Notes = "Aviso pendiente: servir separado"
		body := map[string]any{
			"revision":   bill.Account.Revision,
			"version_id": line.VersionID,
			"line":       line.Request,
			"reason":     "Modificar mientras el destino está inactivo",
		}
		path := "restaurant/accounts/" + bill.Account.ID
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", path+"/edit-preview", body, 200))
		body["expected_fingerprint"] = preview.Fingerprint
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", path+"/edit", body, 200))
		if bill.Account.PreparationState != "change_pending" || kitchenCount(t, fixture, "kitchen_pending_changes") != 1 {
			t.Fatal("La modificación perdió su aviso o aparentó estar preparada")
		}
		for _, task := range kitchenTasks(t, fixture) {
			if task.ReleaseID == releaseID && task.State != "cancelled" {
				fixture.mutate(t, "POST", "kitchen/states", map[string]any{
					"state": "ready", "tasks": []map[string]any{{"id": task.ID, "revision": task.Revision}},
				}, 400)
				break
			}
		}
		second.Active = true
		fixture.mutate(t, "PATCH", "kitchen/stations/"+second.ID, second, 200)
		var eventID string
		var revision int
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT event_id,revision FROM app.kitchen_pending_changes WHERE account_id=$1", bill.Account.ID).Scan(&eventID, &revision); err != nil {
			t.Fatal(err)
		}
		fixture.mutate(t, "POST", "kitchen/changes/"+eventID+"/retry", map[string]any{"revision": revision}, 200)
		if kitchenCount(t, fixture, "kitchen_pending_changes") != 0 {
			t.Fatal("No se resolvió el aviso durable")
		}
		stations := map[string]bool{}
		for _, task := range kitchenTasks(t, fixture) {
			if task.ReleaseID == releaseID && task.State != "cancelled" {
				stations[task.StationID] = true
			}
		}
		if !stations[first.ID] || !stations[second.ID] {
			t.Fatal("El cambio de ruta actual alteró destinos del pedido ya liberado")
		}
		// Restaurar ambas estaciones para los siguientes escenarios de pedidos nuevos.
		currentRoutes := decodeResponse[[]kitchen.Route](t, fixture.adminCall(t, "GET", "kitchen/routes", nil, 200))
		for _, route := range currentRoutes {
			if route.ProductID != nil && *route.ProductID == bill.Lines[0].Request.ProductID {
				route.StationIDs = []string{first.ID, second.ID}
				fixture.mutate(t, "PATCH", "kitchen/routes/"+route.ID, route, 200)
			}
		}
	})
}
