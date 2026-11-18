//go:build integration

package operations_test

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
)

// Revisión independiente del aviso de platillos listos (§4 de la etapa A2, parte D del servidor). Cada subprueba
// reproduce un hallazgo de $SP/doc-rev.md y comprueba el comportamiento que pide la especificación, así que FALLA con
// el código actual (el mensaje describe lo que pasó). No corrige nada: el coordinador decide si se conserva.
//
//   - R1: modificar una unidad quitándole la única estación que faltaba la deja «Listo» sin aviso.
//   - R2: un mesero con sesión con PIN vigente abierta antes de la «Hora de inicio del día»: active es true, el aviso
//     se fija a su dispositivo y ningún dispositivo lo recibe (el cursor lo deja atrás).
//   - R3: base restaurada: un aviso nuevo reutiliza el id de otro que el dispositivo ya avisó ese día.
//   - R4: el canal manda un mensaje cada 750 ms aunque no haya nada, e «interrupted» lleva id 0.
//
// Usa su propia base desde cero (como TestStageA2ReadyAlerts).

// reviewComboLine es el combo del catálogo de prueba con las papas indicadas en su espacio opcional.
func reviewComboLine(fixture *readyFixture, fries int) operations.DraftItem {
	return operations.DraftItem{ID: randomID(), LineRequest: catalog.LineRequest{
		ProductID: fixture.combo.ID, Quantity: 1,
		Customization: catalog.Customization{Components: []catalog.ComponentSelection{{
			SlotID: fixture.combo.Slots[0].ID, ProductID: fixture.fries.ID, Quantity: fries,
		}}},
	}}
}

// reviewTask es una tarea de cocina de una unidad de la cuenta.
type reviewTask struct {
	id, unitID, stationID, state string
	revision                     int
}

func reviewTasks(t *testing.T, fixture *readyFixture, accountID string) []reviewTask {
	t.Helper()
	rows, err := fixture.pool.Query(fixture.ctx, `SELECT t.id::text,t.unit_id::text,t.station_id::text,t.state,t.revision
        FROM app.kitchen_tasks t JOIN app.restaurant_item_units u ON u.id=t.unit_id WHERE u.account_id=$1 ORDER BY t.id`, accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	tasks := []reviewTask{}
	for rows.Next() {
		var task reviewTask
		if err := rows.Scan(&task.id, &task.unitID, &task.stationID, &task.state, &task.revision); err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func TestStageA2ReadyReview(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &readyFixture{waiterModeFixture: &waiterModeFixture{restaurantFixture: newFixture(t)}}
	fixture.seedCatalog(t)
	// Dos estaciones KDS: la categoría va a la parrilla y el producto «Papas producto» a la freidora (la ruta por
	// producto gana a la de la categoría). Así el combo con papas tiene una tarea en cada estación.
	grill := decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Parrilla revisión", Active: true, Reception: "kds", TargetMinutes: 12, WarningMinutes: 15, OverdueMinutes: 25,
		Grouping: "inherit", Tracking: "inherit",
	}, 200))
	fryer := decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Freidora revisión", Active: true, Reception: "kds", TargetMinutes: 12, WarningMinutes: 15, OverdueMinutes: 25,
		Grouping: "inherit", Tracking: "inherit",
	}, 200))
	fixture.station = grill
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare", StationIDs: []string{grill.ID}}, 200)
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &fixture.fries.ID, Mode: "prepare", StationIDs: []string{fryer.ID}}, 200)
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón revisión", Active: true}, 200))
	for index := range 6 {
		fixture.tables = append(fixture.tables, decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: fmt.Sprintf("Mesa %d", index+1), Shape: "round", X: 20 + index*110, Y: 30,
			Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200)))
	}
	fixture.cook = inventoryUser(t, fixture.restaurantFixture, "cocina_rev", []string{
		"kitchen.states.read", "kitchen.states.manage", "kitchen.stations.all", "events.read",
	})
	inventoryUser(t, fixture.restaurantFixture, "ana_rev", waiterPermissions)
	inventoryUser(t, fixture.restaurantFixture, "beto_rev", waiterPermissions)
	for username, target := range map[string]*string{"ana_rev": &fixture.ana, "beto_rev": &fixture.beto} {
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username=$1", username).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	fixture.adminCall(t, "PUT", "users/"+fixture.ana+"/pin", map[string]any{"pin": "4829", "short_name": "Ana", "badge_color": ""}, 200)
	fixture.adminCall(t, "PUT", "users/"+fixture.beto+"/pin", map[string]any{"pin": "6152", "short_name": "Beto", "badge_color": ""}, 200)
	fixture.tabletOne = fixture.readyTablet(t, "Tableta revisión 1")
	fixture.tabletTwo = fixture.readyTablet(t, "Tableta revisión 2")
	fixture.anaOne = fixture.enter(t, fixture.tabletOne, fixture.ana, "4829")
	fixture.betoTwo = fixture.enter(t, fixture.tabletTwo, fixture.beto, "6152")

	t.Run("R1 modificar quita la estación pendiente: la unidad queda Lista sin aviso", func(t *testing.T) {
		before := fixture.head(t)
		accountID := fixture.order(t, fixture.anaOne, 0, reviewComboLine(fixture, 1))
		tasks := reviewTasks(t, fixture, accountID)
		var grillTask *reviewTask
		stations := map[string]bool{}
		for index := range tasks {
			stations[tasks[index].stationID] = true
			if tasks[index].stationID == grill.ID {
				grillTask = &tasks[index]
			}
		}
		if len(tasks) != 2 || !stations[grill.ID] || !stations[fryer.ID] || grillTask == nil {
			t.Fatalf("el combo con papas debía tener una tarea en la parrilla y otra en la freidora: %+v", tasks)
		}
		// La parrilla termina su parte; la freidora todavía no.
		fixture.mutateAs(t, fixture.cook, "POST", "kitchen/states", map[string]any{"state": "ready",
			"tasks": []map[string]any{{"id": grillTask.id, "revision": grillTask.revision}}})
		if fixture.head(t) != before {
			t.Fatal("con la freidora pendiente la unidad no está lista y no debía registrarse aviso")
		}
		// El mesero quita las papas: la tarea de la freidora se cancela y la de la parrilla (sin cambio de receta) sigue
		// lista. La unidad queda «Listo».
		bill := fixture.refresh(t, accountID)
		line := bill.Lines[0]
		requested := line.Request
		requested.Customization.Components = []catalog.ComponentSelection{{
			SlotID: fixture.combo.Slots[0].ID, ProductID: fixture.fries.ID, Quantity: 0,
		}}
		input := operations.EditInput{Mutation: operations.Mutation{Revision: bill.Account.Revision}, VersionID: line.VersionID,
			Line: requested, Reason: "Sin papas"}
		route := "restaurant/accounts/" + accountID
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", route+"/edit-preview", input, 200))
		input.ExpectedFingerprint = preview.Fingerprint
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", route+"/edit", input, 200))
		after := reviewTasks(t, fixture, accountID)
		states := map[string]string{}
		for _, task := range after {
			states[task.stationID] = task.state
		}
		bill = fixture.refresh(t, accountID)
		if bill.Account.PreparationState != "ready" || states[grill.ID] != "ready" || states[fryer.ID] != "cancelled" {
			t.Fatalf("escenario no reproducido: preparación %q, tareas %v", bill.Account.PreparationState, states)
		}
		notices := fixture.count(t, "SELECT count(*)::int FROM app.ready_notices WHERE account_id=$1", accountID)
		alerts := ofAccount(fixture.alerts(t, fixture.tabletOne, before), accountID)
		if notices != 1 || len(alerts) != 1 {
			t.Errorf("la unidad quedó «Listo» (preparation_state %q; parrilla %s, freidora %s) al quitarle la estación "+
				"pendiente y no se registró su aviso: %d filas en ready_notices, %d avisos en la tableta de Ana",
				bill.Account.PreparationState, states[grill.ID], states[fryer.ID], notices, len(alerts))
		}
	})

	t.Run("R2 sesión vigente de antes del inicio del día: active true y el aviso no llega a nadie", func(t *testing.T) {
		config, err := businessday.Load(fixture.ctx, fixture.pool)
		if err != nil {
			t.Fatal(err)
		}
		dayStart := config.Start(config.DayOf(time.Now()).Date)
		// Beto entró en la tableta 2 una hora antes de la «Hora de inicio del día» y su sesión sigue vigente (12 h): por
		// ejemplo entró a las 22:00 con la hora de inicio a las 00:00 (o a las 18:00 y sigue a las 05:15 con la de 05:00).
		if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.sessions SET created_at=$3,last_seen_at=now(),
            expires_at=now()+interval '6 hours' WHERE user_id=$1 AND device_id=$2 AND revoked_at IS NULL`,
			fixture.beto, fixture.tabletTwo.deviceID, dayStart.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
		before := fixture.head(t)
		accountID := fixture.orderFor(t, fixture.beto, 1, draftLine(fixture.fries, 1))
		fixture.ready(t, accountID, "")
		pinned := fixture.count(t, `SELECT count(*)::int FROM app.ready_notices WHERE account_id=$1 AND recipient_user_id=$2
            AND recipient_device_id=$3`, accountID, fixture.beto, fixture.tabletTwo.deviceID)
		two := fixture.alerts(t, fixture.tabletTwo, before)
		one := fixture.alerts(t, fixture.tabletOne, before)
		if pinned != 1 || !two.Active || len(ofAccount(two, accountID)) != 1 {
			t.Errorf("el aviso de Beto quedó fijado a la tableta 2 (%d fila), que responde active %v, pero no lo entrega "+
				"(%d avisos; cursor %d, cabeza %d: ya lo dejó atrás) y la tableta 1 tampoco (%d): no llega a ningún "+
				"dispositivo aunque Beto tiene ahí su sesión abierta", pinned, two.Active, len(ofAccount(two, accountID)),
				two.Cursor, fixture.head(t), len(ofAccount(one, accountID)))
		}
		// Beto vuelve a entrar para lo que sigue.
		fixture.betoTwo = fixture.enter(t, fixture.tabletTwo, fixture.beto, "6152")
	})

	t.Run("R3 base restaurada: un aviso nuevo repite el id de otro que el dispositivo ya avisó", func(t *testing.T) {
		before := fixture.head(t)
		first := fixture.order(t, fixture.anaOne, 2, draftLine(fixture.fries, 1))
		fixture.ready(t, first, "")
		delivered := ofAccount(fixture.alerts(t, fixture.tabletOne, before), first)
		if len(delivered) != 1 {
			t.Fatalf("aviso de la primera cuenta: %+v", delivered)
		}
		seenID := delivered[0].ID
		cursor := fixture.head(t)
		// Se restaura un respaldo de justo antes de ese aviso: la tabla y su secuencia vuelven a ese punto (pg_dump
		// guarda el setval de la identidad). Solo se simula lo de ready_notices.
		if _, err := fixture.pool.Exec(fixture.ctx, "DELETE FROM app.ready_notices WHERE sequence>=$1", seenID); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, fmt.Sprintf(
			"ALTER TABLE app.ready_notices ALTER COLUMN sequence RESTART WITH %d", seenID)); err != nil {
			t.Fatal(err)
		}
		// El dispositivo vuelve con su cursor (mayor que la cabeza: Go relee el día) y cocina marca «Listo» otra cuenta.
		reread := fixture.alerts(t, fixture.tabletOne, cursor)
		second := fixture.order(t, fixture.anaOne, 3, draftLine(fixture.fries, 1))
		fixture.ready(t, second, "")
		fresh := ofAccount(fixture.alerts(t, fixture.tabletOne, reread.Cursor), second)
		if len(fresh) != 1 {
			t.Fatalf("aviso de la segunda cuenta: %+v", fresh)
		}
		if fresh[0].ID == seenID {
			t.Errorf("tras restaurar, el aviso nuevo de la cuenta %s llega con id %d, el mismo que el dispositivo ya avisó "+
				"hoy para la cuenta %s: con A2-D9 (descartar los id repetidos del día) el dispositivo lo descarta y se pierde",
				second, fresh[0].ID, first)
		}
	})

	t.Run("R4 canal: un mensaje cada 750 ms sin novedades e interrupted con id 0", func(t *testing.T) {
		tablet := fixture.readyTablet(t, "Tableta revisión 3")
		fixture.enter(t, tablet, fixture.beto, "6152")
		server := httptest.NewServer(fixture.service)
		defer server.Close()
		request, err := http.NewRequest("GET", server.URL+fmt.Sprintf("/api/v1/ready-alerts/stream?after=%d", fixture.head(t)), nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Device-ID", tablet.deviceID)
		request.Header.Set("X-Device-Token", tablet.deviceToken)
		response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		reader := bufio.NewReader(response.Body)
		type message struct {
			id, event string
			bytes     int
		}
		next := func() message {
			current := message{}
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					t.Fatal("el canal se cerró", err)
				}
				current.bytes += len(line)
				switch {
				case strings.HasPrefix(line, "id: "):
					current.id = strings.TrimSpace(strings.TrimPrefix(line, "id: "))
				case strings.HasPrefix(line, "event: "):
					current.event = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
				case line == "\n":
					return current
				}
			}
		}
		next()
		// Sin ningún «Listo», durante 6 s.
		started := time.Now()
		quiet, quietBytes := 0, 0
		for time.Since(started) < 6*time.Second {
			received := next()
			quiet++
			quietBytes += received.bytes
		}
		if quiet > 1 {
			t.Errorf("sin novedades el canal mandó %d mensajes (%d bytes) en %.1f s; en una hora con la pantalla apagada "+
				"serían unos %d mensajes y %d KB solo de datos, más una petición nueva cada 30 s",
				quiet, quietBytes, time.Since(started).Seconds(), quiet*600, quietBytes*600/1024)
		}
		// Retirar el dispositivo: la siguiente lectura manda «interrupted».
		fixture.mutate(t, "POST", "devices/"+tablet.deviceID+"/revoke", map[string]any{}, 200)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			received := next()
			if received.event == "interrupted" {
				if received.id == "0" {
					t.Errorf("«interrupted» llega con id: 0: el Last-Event-ID estándar del cliente vuelve a 0 y una " +
						"reconexión con él relee todo el día")
				}
				return
			}
		}
		t.Fatal("no llegó «interrupted» tras retirar el dispositivo")
	})
}
