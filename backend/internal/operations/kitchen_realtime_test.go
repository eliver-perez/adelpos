//go:build integration

package operations_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/events"
)

func openEventStream(t *testing.T, fixture *restaurantFixture, server *httptest.Server, terminal testTerminal, after int64, stationID string, status int) *http.Response {
	t.Helper()
	request, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/events?after=%d&station_id=%s", server.URL, after, stationID), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatal("No se abrió el canal autenticado", err)
	}
	if response.StatusCode != status {
		response.Body.Close()
		t.Fatalf("Estado de eventos esperado %d; recibido %d", status, response.StatusCode)
	}
	return response
}
func readEventData(t *testing.T, reader *bufio.Reader) map[string]json.RawMessage {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal("El canal no entregó el evento esperado", err)
		}
		if strings.HasPrefix(line, "data: ") {
			return decodeResponse[map[string]json.RawMessage](t, []byte(strings.TrimPrefix(line, "data: ")))
		}
	}
}

func testKitchenRealtime(t *testing.T, fixture *restaurantFixture) {
	t.Run("eventos autenticados replay ámbitos revocación y recuperación completa", func(t *testing.T) {
		server := httptest.NewServer(fixture.service)
		defer server.Close()
		denied := openEventStream(t, fixture, server, testTerminal{}, 0, "", 401)
		denied.Body.Close()
		var head int64
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(max(sequence),0) FROM app.realtime_events").Scan(&head); err != nil {
			t.Fatal(err)
		}
		response := openEventStream(t, fixture, server, fixture.admin, head, "", 200)
		reader := bufio.NewReader(response.Body)
		readEventData(t, reader)
		bill := newAccount(t, fixture, "Evento entre terminales")
		received := false
		for attempt := 0; attempt < 5; attempt++ {
			data := readEventData(t, reader)
			changes := decodeResponse[[]events.Change](t, data["events"])
			for _, event := range changes {
				if event.AggregateID != nil && *event.AggregateID == bill.Account.ID {
					received = true
				}
			}
			if received {
				break
			}
		}
		response.Body.Close()
		if !received {
			t.Fatal("Otra terminal no recibió el cambio confirmado")
		}
		replay := decodeResponse[events.Batch](t, fixture.adminCall(t, "GET", fmt.Sprintf("events/snapshot?after=%d", head), nil, 200))
		ids := map[string]bool{}
		for _, event := range replay.Events {
			if ids[event.ID] {
				t.Fatal("Un lote contiene eventos duplicados")
			}
			ids[event.ID] = true
		}
		after := decodeResponse[events.Batch](t, fixture.adminCall(t, "GET", fmt.Sprintf("events/snapshot?after=%d", replay.Cursor), nil, 200))
		if len(after.Events) != 0 {
			t.Fatal("El cursor repitió eventos ya consumidos")
		}

		roles := decodeResponse[[]struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Permissions []string `json:"permissions"`
		}](t, fixture.adminCall(t, "GET", "roles", nil, 200))
		for _, role := range roles {
			if role.Name == "Mesero autorizado de prueba" {
				permissions := append(role.Permissions, "events.read", "kitchen.states.read", "kitchen.tickets.read", "kitchen.kds.ack")
				fixture.adminCall(t, "PATCH", "roles/"+role.ID, map[string]any{"name": role.Name, "permissions": permissions}, 200)
			}
		}
		var stationID, otherStation string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id FROM app.kitchen_stations ORDER BY name LIMIT 1").Scan(&stationID); err != nil {
			t.Fatal(err)
		}
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id FROM app.kitchen_stations WHERE id<>$1 ORDER BY name LIMIT 1", stationID).Scan(&otherStation); err != nil {
			t.Fatal(err)
		}
		fixture.mutate(t, "PUT", "kitchen/access", map[string]any{"device_id": fixture.waiter.deviceID, "station_ids": []string{stationID}}, 200)
		fixture.call(t, fixture.waiter, "GET", "kitchen/board?station_id="+otherStation, nil, 403)
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT max(sequence) FROM app.realtime_events").Scan(&head); err != nil {
			t.Fatal(err)
		}
		scoped := openEventStream(t, fixture, server, fixture.waiter, head, stationID, 200)
		defer scoped.Body.Close()
		scopedReader := bufio.NewReader(scoped.Body)
		readEventData(t, scopedReader)
		for _, station := range []string{stationID, otherStation} {
			tx, err := fixture.pool.Begin(fixture.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := events.Append(fixture.ctx, tx, "kitchen.scope.acceptance", "kitchen", nil, &station); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(fixture.ctx); err != nil {
				t.Fatal(err)
			}
		}
		data := readEventData(t, scopedReader)
		changes := decodeResponse[[]events.Change](t, data["events"])
		if len(changes) != 1 || changes[0].StationID == nil || *changes[0].StationID != stationID {
			t.Fatal("El canal filtró información de otra estación")
		}
		identity := decodeResponse[struct {
			User struct {
				SessionID string `json:"session_id"`
			} `json:"user"`
		}](t, fixture.call(t, fixture.waiter, "GET", "auth/me", nil, 200))
		fixture.adminCall(t, "POST", "sessions/"+identity.User.SessionID+"/revoke", nil, 200)
		revoked := readEventData(t, scopedReader)
		if string(revoked["status"]) != "401" {
			t.Fatal("La sesión conectada no fue revocada")
		}
		scoped.Body.Close()
		if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.realtime_events(id,kind,scope)
            SELECT gen_random_uuid(),'acceptance.replay.window','restaurant' FROM generate_series(1,5002)`); err != nil {
			t.Fatal(err)
		}
		reset := decodeResponse[events.Batch](t, fixture.adminCall(t, "GET", "events/snapshot?after=0", nil, 200))
		if !reset.Reset || len(reset.Events) != 0 || reset.Cursor <= head {
			t.Fatal("No se indicó reconstrucción de estado fuera de la ventana de replay")
		}
	})
}
