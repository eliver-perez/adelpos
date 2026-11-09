//go:build integration

package operations_test

import (
	"encoding/json"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// positioned convierte un registro en cuerpo de escritura y le agrega la posición elegida.
func positioned(t *testing.T, value any, after string) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	fields["position"] = map[string]any{"after": after}
	return fields
}

// checkRenumbered exige múltiplos de 10 en el orden listado y una revisión nueva en cada
// registro anterior cuyo número cambió.
func checkRenumbered(
	t *testing.T,
	label string,
	ids []string,
	orders, revisions map[string]int,
	before map[string][2]int,
) {
	t.Helper()
	for index, id := range ids {
		if orders[id] != (index+1)*10 {
			t.Fatalf("%s: la renumeración no usa múltiplos de 10 en el orden listado: %v", label, orders)
		}
		old, existed := before[id]
		if existed && old[0] != orders[id] && revisions[id] != old[1]+1 {
			t.Fatalf("%s: un hermano movido no incrementó su revisión (%s)", label, id)
		}
		if existed && old[0] == orders[id] && revisions[id] != old[1] {
			t.Fatalf("%s: un hermano sin cambio de orden cambió de revisión (%s)", label, id)
		}
	}
}

func testOrderPositions(t *testing.T, fixture *restaurantFixture) {
	t.Run("etapa 6.1 posiciones de áreas estaciones y métodos de pago", func(t *testing.T) {
		readAreas := func() ([]string, map[string]int, map[string]int, map[string][2]int) {
			areas := decodeResponse[[]operations.Area](t, fixture.adminCall(t, "GET", "restaurant/areas", nil, 200))
			ids, orders, revisions, snapshot := []string{}, map[string]int{}, map[string]int{}, map[string][2]int{}
			for _, area := range areas {
				ids = append(ids, area.ID)
				orders[area.ID] = area.SortOrder
				revisions[area.ID] = area.Revision
				snapshot[area.ID] = [2]int{area.SortOrder, area.Revision}
			}
			return ids, orders, revisions, snapshot
		}
		_, _, _, before := readAreas()
		patio := decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", map[string]any{
			"name": "Patio", "description": "", "active": true, "position": map[string]any{"after": ""},
		}, 200))
		ids, orders, revisions, _ := readAreas()
		if ids[0] != patio.ID || patio.SortOrder != 10 || patio.Revision != 1 {
			t.Fatalf("el área al inicio no quedó primera: %+v %v", patio, ids)
		}
		checkRenumbered(t, "áreas", ids, orders, revisions, before)
		bar := decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", map[string]any{
			"name": "Barra", "description": "", "active": true, "position": map[string]any{"after": patio.ID},
		}, 200))
		if bar.SortOrder != 20 {
			t.Fatalf("«después de Patio» no quedó en segundo lugar: %+v", bar)
		}
		problem := string(fixture.mutate(t, "PATCH", "restaurant/areas/"+bar.ID, positioned(t, bar, bar.ID), 400))
		if !strings.Contains(problem, "sí mismo") {
			t.Fatalf("el área después de sí misma no se explicó: %s", problem)
		}
		// Sin posición se conserva el comportamiento anterior: se guarda el número recibido.
		bar.Description = "Junto a la entrada"
		saved := decodeResponse[operations.Area](t, fixture.mutate(t, "PATCH", "restaurant/areas/"+bar.ID, bar, 200))
		if saved.SortOrder != 20 || saved.Description != "Junto a la entrada" {
			t.Fatalf("guardar sin posición alteró el orden: %+v", saved)
		}

		stations := decodeResponse[[]kitchen.Station](t, fixture.adminCall(t, "GET", "kitchen/stations", nil, 200))
		if len(stations) < 2 {
			t.Fatal("se esperaban las estaciones de la aceptación de cocina")
		}
		stationBefore := map[string][2]int{}
		for _, station := range stations {
			stationBefore[station.ID] = [2]int{station.SortOrder, station.Revision}
		}
		dessertsBody := positioned(t, kitchen.Station{
			Name:      "Postres",
			Active:    true,
			Reception: "kds",
			Grouping:  "inherit",
			Tracking:  "inherit",
		}, stations[0].ID)
		desserts := decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", dessertsBody, 200))
		listed := decodeResponse[[]kitchen.Station](t, fixture.adminCall(t, "GET", "kitchen/stations", nil, 200))
		if listed[1].ID != desserts.ID || desserts.SortOrder != 20 {
			t.Fatalf("la estación no quedó después de la primera: %+v", listed)
		}
		stationIDs, stationOrders, stationRevisions := []string{}, map[string]int{}, map[string]int{}
		for _, station := range listed {
			stationIDs = append(stationIDs, station.ID)
			stationOrders[station.ID] = station.SortOrder
			stationRevisions[station.ID] = station.Revision
		}
		checkRenumbered(t, "estaciones", stationIDs, stationOrders, stationRevisions, stationBefore)
		// Un área no es hermana de una estación aunque su identificador exista.
		problem = string(fixture.mutate(t, "PATCH", "kitchen/stations/"+desserts.ID, positioned(t, desserts, patio.ID), 400))
		if !strings.Contains(problem, "misma lista") {
			t.Fatalf("el hermano de otra lista no se explicó: %s", problem)
		}
		movedBody := positioned(t, desserts, "")
		moved := decodeResponse[kitchen.Station](t, fixture.mutate(t, "PATCH", "kitchen/stations/"+desserts.ID, movedBody, 200))
		if moved.SortOrder != 10 || moved.Revision != desserts.Revision+1 {
			t.Fatalf("la estación movida al inicio no fue la esperada: %+v", moved)
		}

		methods := decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200))
		methodBefore := map[string][2]int{}
		for _, method := range methods {
			methodBefore[method.ID] = [2]int{method.SortOrder, method.Revision}
		}
		voucher := decodeResponse[payments.Method](t, fixture.mutate(t, "POST", "payments/methods", map[string]any{
			"name": "Vales de despensa", "icon": "voucher", "active": true, "sort_order": 0, "is_cash": false,
			"reference_mode": "optional", "allows_tip": false, "allows_refund": true,
			"position": map[string]any{"after": methods[0].ID},
		}, 200))
		ordered := decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200))
		if ordered[1].ID != voucher.ID || voucher.SortOrder != 20 {
			t.Fatalf("el método no quedó después del primero: %+v", ordered)
		}
		methodIDs, methodOrders, methodRevisions := []string{}, map[string]int{}, map[string]int{}
		for _, method := range ordered {
			methodIDs = append(methodIDs, method.ID)
			methodOrders[method.ID] = method.SortOrder
			methodRevisions[method.ID] = method.Revision
		}
		checkRenumbered(t, "métodos", methodIDs, methodOrders, methodRevisions, methodBefore)
		input := map[string]any{
			"revision": voucher.Revision, "name": voucher.Name, "icon": voucher.Icon, "active": voucher.Active,
			"sort_order": voucher.SortOrder, "is_cash": voucher.IsCash, "reference_mode": voucher.ReferenceMode,
			"allows_tip": voucher.AllowsTip, "allows_refund": voucher.AllowsRefund,
			"position": map[string]any{"after": desserts.ID},
		}
		fixture.mutate(t, "PATCH", "payments/methods/"+voucher.ID, input, 400)
		input["position"] = map[string]any{"after": voucher.ID}
		fixture.mutate(t, "PATCH", "payments/methods/"+voucher.ID, input, 400)
		input["position"] = map[string]any{}
		fixture.mutate(t, "PATCH", "payments/methods/"+voucher.ID, input, 400)

		var reorders int
		err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(DISTINCT action) FROM app.audit_events
            WHERE action IN ('restaurant.areas.reordered','kitchen.station.reordered','payments.methods_reordered')`).Scan(&reorders)
		if err != nil || reorders != 3 {
			t.Fatalf("la renumeración no quedó auditada en los tres módulos: %d %v", reorders, err)
		}
	})
}
