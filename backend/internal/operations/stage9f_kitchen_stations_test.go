//go:build integration

package operations_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/stationscope"
)

// Etapa 9F, fase C (§5 y §8) contra PostgreSQL con la API real:
//   - se ve la intersección de las estaciones de la terminal y las del usuario en Preparación, en el
//     tiempo real, en los cambios de estado, en los acuses de recepción y en las comandas; una tarea de
//     una estación que no se ve se rechaza sin cambiar nada;
//   - sin estaciones en común Go lo informa (station_scope.common = false, con los nombres) en lugar de
//     una pantalla vacía;
//   - quien tiene kitchen.stations.all atiende todas las estaciones de la terminal y sigue viendo todas
//     las comandas (la cola y el historial son administrativos);
//   - un combo con componentes en dos estaciones: cada una ve solo su parte, «Marcar producto listo» en
//     una marca solo esa parte y la partida queda lista (también en la cuenta) cuando ambas lo están;
//   - un complemento vinculado a un producto con otra ruta viaja con su producto (no se reparte);
//   - la configuración de la terminal (todas o selección, aviso sonoro, conflicto y validaciones).
//
// Usa su propia base desde cero (como las pruebas de la 9D y de la fase E).

type stage9FKitchenFixture struct {
	*restaurantFixture
	cook      testTerminal
	cookID    string
	chef      testTerminal
	plancha   kitchen.Station
	freidoras kitchen.Station
	barra     kitchen.Station
	postres   kitchen.Station
	bill      operations.Bill
}

// stage9FBoard es la forma de GET kitchen/board que esta prueba necesita.
type stage9FBoard struct {
	Tasks        []kitchen.Task       `json:"tasks"`
	Stations     []kitchen.Station    `json:"stations"`
	StationScope stationscope.Summary `json:"station_scope"`
	SoundAlert   bool                 `json:"sound_alert"`
	Releases     []map[string]any     `json:"releases"`
	Pending      []map[string]any     `json:"pending_changes"`
}

func (fixture *stage9FKitchenFixture) board(t *testing.T, terminal testTerminal, query string, status int) stage9FBoard {
	t.Helper()
	body := fixture.call(t, terminal, "GET", "kitchen/board?state=all&account_id="+fixture.bill.Account.ID+query, nil, status)
	if status != 200 {
		return stage9FBoard{}
	}
	return decodeResponse[stage9FBoard](t, body)
}

// stationTasks devuelve las tareas de la cuenta de la prueba en una estación (vistas por el administrador
// en su terminal, que muestra todas).
func (fixture *stage9FKitchenFixture) stationTasks(t *testing.T, station kitchen.Station) []kitchen.Task {
	t.Helper()
	result := []kitchen.Task{}
	for _, task := range fixture.board(t, fixture.admin, "", 200).Tasks {
		if task.StationID == station.ID {
			result = append(result, task)
		}
	}
	return result
}

// setState cambia el estado de tareas desde una terminal y devuelve la respuesta.
func (fixture *stage9FKitchenFixture) setState(t *testing.T, terminal testTerminal, tasks []kitchen.Task, state string, status int) []byte {
	t.Helper()
	references := []map[string]any{}
	for _, task := range tasks {
		references = append(references, map[string]any{"id": task.ID, "revision": task.Revision})
	}
	return fixture.call(t, terminal, "POST", "kitchen/states", map[string]any{
		"operation_id": randomID(), "state": state, "tasks": references,
	}, status)
}

func (fixture *stage9FKitchenFixture) saveTerminal(t *testing.T, deviceID string, body map[string]any, status int) []byte {
	t.Helper()
	body["operation_id"] = randomID()
	body["device_id"] = deviceID
	return fixture.adminCall(t, "PUT", "kitchen/access", body, status)
}

func (fixture *stage9FKitchenFixture) saveUser(t *testing.T, all bool, stations []string, expectedAll bool, expected []string) {
	t.Helper()
	fixture.adminCall(t, "PUT", "users/"+fixture.cookID+"/kitchen-stations", map[string]any{
		"all_stations": all, "station_ids": stations, "expected_all_stations": expectedAll, "expected_station_ids": expected,
	}, 200)
}

func stage9FNames(stations []kitchen.Station) []string {
	result := []string{}
	for _, station := range stations {
		result = append(result, station.Name)
	}
	return result
}

func stage9FPartNames(task kitchen.Task) []string {
	result := []string{}
	for _, part := range task.Instructions.Parts {
		result = append(result, fmt.Sprintf("%d × %s", part.Quantity, part.Name))
	}
	return result
}

func TestStage9FKitchenStations(t *testing.T) {
	resetStage9DDatabase(t)
	fixture := &stage9FKitchenFixture{restaurantFixture: newFixture(t)}
	fixture.seedCatalog(t)
	station := func(name string) kitchen.Station {
		return decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", map[string]any{
			"name": name, "description": "", "active": true, "sort_order": 0, "reception": "kds", "printer_id": nil,
		}, 200))
	}
	fixture.barra = station("Barra 9F")
	fixture.freidoras = station("Freidoras 9F")
	fixture.plancha = station("Plancha 9F")
	fixture.postres = station("Postres 9F")
	// Platillos (también el combo) a la plancha; las papas, con ruta propia, a las freidoras; el filete,
	// con ruta propia, a la barra.
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare", StationIDs: []string{fixture.plancha.ID}}, 200)
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &fixture.fries.ID, Mode: "prepare", StationIDs: []string{fixture.freidoras.ID}}, 200)
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &fixture.product.ID, Mode: "prepare", StationIDs: []string{fixture.barra.ID}}, 200)
	// Usuario de cocina con solo permisos de cocina y un jefe de cocina con kitchen.stations.all, cada uno
	// en su propia terminal (un usuario solo tiene una sesión a la vez).
	kitchenPermissions := []string{"events.read", "kitchen.states.read", "kitchen.states.manage", "kitchen.tickets.read", "kitchen.kds.ack"}
	fixture.cook = inventoryUser(t, fixture.restaurantFixture, "cocina_9f", kitchenPermissions)
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username='cocina_9f'").Scan(&fixture.cookID); err != nil {
		t.Fatal(err)
	}
	fixture.chef = inventoryUser(t, fixture.restaurantFixture, "jefe_cocina_9f", append(kitchenPermissions, "kitchen.stations.all"))

	// Una cuenta con el combo (con papas) y un filete, enviada a cocina.
	bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": "Estaciones 9F", "mode": "dine_in"}, 200))
	fixture.bill = fixture.capture(t, bill, []catalog.LineRequest{
		{
			ProductID: fixture.combo.ID,
			Quantity:  1,
			Customization: catalog.Customization{Components: []catalog.ComponentSelection{{
				SlotID: fixture.combo.Slots[0].ID, ProductID: fixture.fries.ID, Quantity: 1,
			}}},
		},
		{ProductID: fixture.product.ID, Quantity: 1},
	})

	t.Run("un combo en dos estaciones: cada una ve solo su parte", func(t *testing.T) {
		testStage9FComboParts(t, fixture)
	})
	t.Run("la terminal y el usuario: se ve la intersección", func(t *testing.T) {
		testStage9FIntersection(t, fixture)
	})
	t.Run("el tiempo real solo entrega las estaciones visibles", func(t *testing.T) {
		testStage9FRealtime(t, fixture)
	})
	t.Run("la partida queda lista cuando ambas estaciones lo están", func(t *testing.T) {
		testStage9FReadyWhenAllParts(t, fixture)
	})
	t.Run("sin estaciones en común Go lo informa", func(t *testing.T) {
		testStage9FNoCommonStations(t, fixture)
	})
	t.Run("un complemento vinculado a otro producto viaja con su producto", func(t *testing.T) {
		testStage9FComplementStaysWithProduct(t, fixture)
	})
	t.Run("configuración de la terminal: validaciones, conflicto y aviso sonoro", func(t *testing.T) {
		testStage9FTerminalConfiguration(t, fixture)
	})
}

func testStage9FComboParts(t *testing.T, fixture *stage9FKitchenFixture) {
	grill := fixture.stationTasks(t, fixture.plancha)
	fryer := fixture.stationTasks(t, fixture.freidoras)
	bar := fixture.stationTasks(t, fixture.barra)
	if len(grill) != 1 || len(fryer) != 1 || len(bar) != 1 {
		t.Fatalf("tareas por estación: plancha %d, freidoras %d, barra %d", len(grill), len(fryer), len(bar))
	}
	// Las dos tareas del combo son de la misma unidad, pero cada estación recibe solo su parte.
	if grill[0].UnitID != fryer[0].UnitID || grill[0].Instructions.Name != fixture.combo.Name {
		t.Fatal("las partes del combo debían pertenecer a la misma unidad comercial")
	}
	if got := stage9FPartNames(grill[0]); !slices.Equal(got, []string{"1 × " + fixture.combo.Name}) {
		t.Fatalf("la plancha debía ver solo el combo, sin las papas: %v", got)
	}
	if got := stage9FPartNames(fryer[0]); !slices.Equal(got, []string{"1 × " + fixture.fries.Name}) {
		t.Fatalf("las freidoras debían ver solo las papas: %v", got)
	}
	if got := stage9FPartNames(bar[0]); !slices.Equal(got, []string{"1 × " + fixture.product.Name}) {
		t.Fatalf("la barra debía ver solo el filete: %v", got)
	}
	// Una unidad comercial por producto: el combo no crea otra liberación por sus dos estaciones.
	var units int
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FROM app.kitchen_unit_releases k
        JOIN app.restaurant_item_units u ON u.id=k.unit_id WHERE u.account_id=$1`, fixture.bill.Account.ID).Scan(&units); err != nil {
		t.Fatal(err)
	}
	if units != 2 {
		t.Fatalf("liberaciones canónicas: %d", units)
	}
}

func testStage9FIntersection(t *testing.T, fixture *stage9FKitchenFixture) {
	// Terminales: plancha y freidoras. Usuario de cocina: freidoras. Se ve solo freidoras.
	for _, terminal := range []testTerminal{fixture.cook, fixture.chef} {
		fixture.saveTerminal(t, terminal.deviceID, map[string]any{
			"all_stations": false, "station_ids": []string{fixture.plancha.ID, fixture.freidoras.ID},
			"expected_all_stations": true, "expected_station_ids": []string{},
		}, 200)
	}
	fixture.saveUser(t, false, []string{fixture.freidoras.ID}, true, []string{})
	board := fixture.board(t, fixture.cook, "", 200)
	if len(board.Tasks) != 1 || board.Tasks[0].StationID != fixture.freidoras.ID {
		t.Fatalf("el usuario de cocina debía ver solo las freidoras: %d tareas", len(board.Tasks))
	}
	if !slices.Equal(stage9FNames(board.Stations), []string{"Freidoras 9F"}) {
		t.Fatalf("estaciones visibles: %v", stage9FNames(board.Stations))
	}
	scope := board.StationScope
	if !scope.Common || scope.All || scope.TerminalAll || scope.UserAll ||
		!slices.Equal(scope.TerminalStations, []string{"Freidoras 9F", "Plancha 9F"}) ||
		!slices.Equal(scope.UserStations, []string{"Freidoras 9F"}) {
		t.Fatalf("intersección informada: %+v", scope)
	}
	if len(board.Releases) != 0 || len(board.Pending) != 0 {
		t.Fatal("las liberaciones y avisos pendientes siguen exigiendo kitchen.stations.all")
	}
	fixture.board(t, fixture.cook, "&station_id="+fixture.plancha.ID, 403)
	fixture.board(t, fixture.cook, "&station_id="+fixture.barra.ID, 403)

	// Una tarea de una estación que no se ve se rechaza sin cambiar nada.
	grill := fixture.stationTasks(t, fixture.plancha)
	rejected := string(fixture.setState(t, fixture.cook, grill, "preparing", 403))
	if !strings.Contains(rejected, "STATION_SCOPE_DENIED") {
		t.Fatalf("rechazo: %s", rejected)
	}
	if after := fixture.stationTasks(t, fixture.plancha); after[0].Revision != grill[0].Revision || after[0].State != "released" {
		t.Fatal("el rechazo cambió la tarea de la plancha")
	}
	// Un pedido completo por estación también exige verla.
	release := grill[0].ReleaseID
	var revision int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT revision FROM app.kitchen_releases WHERE id=$1", release).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	fixture.call(t, fixture.cook, "POST", "kitchen/states", map[string]any{
		"operation_id": randomID(), "state": "preparing", "release_id": release, "revision": revision, "station_id": fixture.plancha.ID,
	}, 403)
	fryer := fixture.stationTasks(t, fixture.freidoras)
	fixture.setState(t, fixture.cook, fryer, "preparing", 200)

	// El jefe de cocina (kitchen.stations.all) atiende todas las estaciones de su terminal (no la barra) y
	// Go lo dice.
	chef := fixture.board(t, fixture.chef, "", 200)
	chefStations := []string{}
	for _, task := range chef.Tasks {
		chefStations = append(chefStations, task.StationID)
	}
	slices.Sort(chefStations)
	expected := []string{fixture.plancha.ID, fixture.freidoras.ID}
	slices.Sort(expected)
	if !slices.Equal(chefStations, expected) || !chef.StationScope.UserAllByPermission || !chef.StationScope.UserAll {
		t.Fatalf("jefe de cocina en su terminal: %v %+v", chefStations, chef.StationScope)
	}
	fixture.setState(t, fixture.chef, fixture.stationTasks(t, fixture.barra), "preparing", 403)

	// Comandas: el usuario de cocina solo las de sus estaciones; con kitchen.stations.all, todas (son
	// administrativas, como la cola de impresión).
	tickets := func(terminal testTerminal) []string {
		rows := decodeResponse[[]struct {
			ID string `json:"id"`
		}](t, fixture.call(t, terminal, "GET", "kitchen/tickets", nil, 200))
		ids := []string{}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		return ids
	}
	ticketOf := func(station kitchen.Station) string {
		var id string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.kitchen_tickets WHERE release_id=$1 AND $2::uuid=ANY(station_ids)",
			release, station.ID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	fryerTicket, grillTicket, barTicket := ticketOf(fixture.freidoras), ticketOf(fixture.plancha), ticketOf(fixture.barra)
	cookTickets := tickets(fixture.cook)
	if !slices.Contains(cookTickets, fryerTicket) || slices.Contains(cookTickets, grillTicket) || slices.Contains(cookTickets, barTicket) {
		t.Fatalf("comandas del usuario de cocina: %v", cookTickets)
	}
	adminTickets := tickets(fixture.chef)
	if !slices.Contains(adminTickets, grillTicket) || !slices.Contains(adminTickets, barTicket) {
		t.Fatal("con kitchen.stations.all se ven todas las comandas")
	}
	fixture.call(t, fixture.cook, "GET", "kitchen/tickets/"+grillTicket, nil, 403)
	fixture.call(t, fixture.chef, "GET", "kitchen/tickets/"+barTicket, nil, 200)
	// Acuse de recepción: solo de una estación visible.
	fixture.call(t, fixture.cook, "POST", "kitchen/tickets/"+fryerTicket+"/ack", map[string]any{
		"operation_id": randomID(), "station_id": fixture.freidoras.ID,
	}, 200)
	fixture.call(t, fixture.cook, "POST", "kitchen/tickets/"+grillTicket+"/ack", map[string]any{
		"operation_id": randomID(), "station_id": fixture.plancha.ID,
	}, 403)
}

func testStage9FRealtime(t *testing.T, fixture *stage9FKitchenFixture) {
	var head int64
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT max(sequence) FROM app.realtime_events").Scan(&head); err != nil {
		t.Fatal(err)
	}
	for _, station := range []kitchen.Station{fixture.plancha, fixture.freidoras, fixture.barra, fixture.postres} {
		tx, err := fixture.pool.Begin(fixture.ctx)
		if err != nil {
			t.Fatal(err)
		}
		id := station.ID
		if err := events.Append(fixture.ctx, tx, "kitchen.scope.stage9f", "kitchen", nil, &id); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(fixture.ctx); err != nil {
			t.Fatal(err)
		}
	}
	received := func(terminal testTerminal, query string) []string {
		batch := decodeResponse[events.Batch](t, fixture.call(t, terminal, "GET", fmt.Sprintf("events/snapshot?after=%d%s", head, query), nil, 200))
		stations := []string{}
		for _, event := range batch.Events {
			if event.Kind == "kitchen.scope.stage9f" && event.StationID != nil {
				stations = append(stations, *event.StationID)
			}
		}
		return stations
	}
	if got := received(fixture.cook, ""); !slices.Equal(got, []string{fixture.freidoras.ID}) {
		t.Fatalf("el usuario de cocina recibió eventos de otras estaciones: %v", got)
	}
	if got := received(fixture.cook, "&station_id="+fixture.freidoras.ID); !slices.Equal(got, []string{fixture.freidoras.ID}) {
		t.Fatalf("filtro por estación visible: %v", got)
	}
	fixture.call(t, fixture.cook, "GET", fmt.Sprintf("events/snapshot?after=%d&station_id=%s", head, fixture.plancha.ID), nil, 403)
	// Con kitchen.stations.all llegan todas, como la cola y el historial.
	if got := received(fixture.chef, ""); len(got) != 4 {
		t.Fatalf("con kitchen.stations.all debían llegar las cuatro estaciones: %v", got)
	}
}

func testStage9FReadyWhenAllParts(t *testing.T, fixture *stage9FKitchenFixture) {
	comboLine := func() operations.ConfirmedLine {
		for _, line := range fixture.refresh(t, fixture.bill.Account.ID).Lines {
			if line.Request.ProductID == fixture.combo.ID {
				return line
			}
		}
		t.Fatal("no se encontró la partida del combo")
		return operations.ConfirmedLine{}
	}
	// «Marcar producto listo» en las freidoras marca solo esa parte.
	fixture.setState(t, fixture.cook, fixture.stationTasks(t, fixture.freidoras), "ready", 200)
	if state := fixture.stationTasks(t, fixture.plancha)[0].State; state != "released" {
		t.Fatalf("la parte de la plancha cambió a %s", state)
	}
	if state := comboLine().PreparationState; state != "preparing" {
		t.Fatalf("con una sola parte lista, la partida debía seguir en preparación: %s", state)
	}
	// Al terminar la plancha la partida queda lista, también en la cuenta (cuando la barra termina el filete).
	fixture.setState(t, fixture.chef, fixture.stationTasks(t, fixture.plancha), "ready", 200)
	if state := comboLine().PreparationState; state != "ready" {
		t.Fatalf("con sus dos partes listas, la partida del combo debía quedar lista: %s", state)
	}
	if state := fixture.refresh(t, fixture.bill.Account.ID).Account.PreparationState; state != "preparing" {
		t.Fatalf("la cuenta tiene el filete pendiente: %s", state)
	}
	fixture.setState(t, fixture.admin, fixture.stationTasks(t, fixture.barra), "ready", 200)
	if state := fixture.refresh(t, fixture.bill.Account.ID).Account.PreparationState; state != "ready" {
		t.Fatalf("con todas sus partes listas la cuenta debía quedar lista: %s", state)
	}
}

func testStage9FNoCommonStations(t *testing.T, fixture *stage9FKitchenFixture) {
	fixture.saveUser(t, false, []string{fixture.postres.ID}, false, []string{fixture.freidoras.ID})
	board := fixture.board(t, fixture.cook, "", 200)
	scope := board.StationScope
	if scope.Common || len(board.Tasks) != 0 || len(board.Stations) != 0 ||
		!slices.Equal(scope.TerminalStations, []string{"Freidoras 9F", "Plancha 9F"}) ||
		!slices.Equal(scope.UserStations, []string{"Postres 9F"}) {
		t.Fatalf("sin estaciones en común: %+v, %d tareas", scope, len(board.Tasks))
	}
	fixture.setState(t, fixture.cook, fixture.stationTasks(t, fixture.freidoras), "ready", 403)
	// El usuario vuelve a «todas»: ve las de la terminal.
	fixture.saveUser(t, true, []string{}, false, []string{fixture.postres.ID})
	board = fixture.board(t, fixture.cook, "", 200)
	if !board.StationScope.Common || !board.StationScope.UserAll || !slices.Equal(stage9FNames(board.Stations), []string{"Freidoras 9F", "Plancha 9F"}) {
		t.Fatalf("usuario con todas: %+v %v", board.StationScope, stage9FNames(board.Stations))
	}
}

func testStage9FComplementStaysWithProduct(t *testing.T, fixture *stage9FKitchenFixture) {
	// «Papas fritas 9F» es un complemento vinculado al producto de papas (ruta a las freidoras).
	complement := decodeResponse[catalog.Complement](t, fixture.adminCall(t, "POST", "catalog/complements", catalog.Complement{
		Metadata: catalog.Metadata{Name: "Papas fritas 9F", Active: true}, ProductID: &fixture.fries.ID,
	}, 200))
	group := decodeResponse[catalog.OptionGroup](t, fixture.adminCall(t, "POST", "catalog/groups", catalog.OptionGroup{
		Metadata: catalog.Metadata{Name: "Guarnición 9F", Active: true}, PricingMode: "value", MinQuantity: "0",
		Options: []catalog.GroupOption{{ComplementID: complement.ID, ReferenceValue: "20", ExtraPrice: "0", Active: true}},
	}, 200))
	burger := decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Hamburguesa 9F", Active: true}, CategoryID: fixture.category.ID, Code: "HAMB-9F",
		Kind: "product", BasePrice: "120",
		Groups: []catalog.ProductGroup{{GroupID: group.ID, Included: []catalog.IncludedOption{{OptionID: group.Options[0].ID, Quantity: "1"}}}},
	}, 200))
	bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": "Complemento 9F", "mode": "dine_in"}, 200))
	bill = fixture.capture(t, bill, []catalog.LineRequest{{ProductID: burger.ID, Quantity: 1}})
	tasks := decodeResponse[stage9FBoard](t, fixture.adminCall(t, "GET", "kitchen/board?state=all&account_id="+bill.Account.ID, nil, 200)).Tasks
	if len(tasks) != 1 || tasks[0].StationID != fixture.plancha.ID {
		t.Fatalf("la hamburguesa debía llegar solo a la plancha: %d tareas", len(tasks))
	}
	found := false
	for _, part := range tasks[0].Instructions.Parts {
		for _, chosen := range part.Groups {
			for _, option := range chosen.Options {
				if option.Name == "Papas fritas 9F" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("el complemento debía viajar con la hamburguesa a la plancha")
	}
}

func testStage9FTerminalConfiguration(t *testing.T, fixture *stage9FKitchenFixture) {
	current := []string{fixture.plancha.ID, fixture.freidoras.ID}
	slices.Sort(current)
	invalid := []map[string]any{
		{"all_stations": true, "station_ids": []string{fixture.barra.ID}, "expected_station_ids": current},
		{"all_stations": false, "station_ids": []string{}, "expected_station_ids": current},
		{"all_stations": false, "station_ids": []string{fixture.barra.ID, fixture.barra.ID}, "expected_station_ids": current},
		{"all_stations": false, "station_ids": []string{randomID()}, "expected_station_ids": current},
	}
	for _, body := range invalid {
		fixture.saveTerminal(t, fixture.cook.deviceID, body, 400)
	}
	// Otra terminal cambió la selección: conflicto, sin escribir.
	fixture.saveTerminal(t, fixture.cook.deviceID, map[string]any{"all_stations": true, "station_ids": []string{}, "expected_station_ids": []string{fixture.barra.ID}}, 409)
	fixture.saveTerminal(t, fixture.cook.deviceID, map[string]any{"all_stations": true, "station_ids": []string{}, "expected_all_stations": true, "expected_station_ids": current}, 409)
	if board := fixture.board(t, fixture.cook, "", 200); board.SoundAlert || board.StationScope.TerminalAll {
		t.Fatal("los rechazos cambiaron la terminal")
	}
	// Aviso sonoro activado y la terminal vuelve a «todas»; queda en el historial de cocina auditado.
	saved := decodeResponse[map[string]any](t, fixture.saveTerminal(t, fixture.cook.deviceID, map[string]any{
		"all_stations": true, "station_ids": []string{}, "sound_alert": true,
		"expected_all_stations": false, "expected_station_ids": current,
	}, 200))
	if saved["all_stations"] != true || saved["sound_alert"] != true {
		t.Fatalf("respuesta: %v", saved)
	}
	board := fixture.board(t, fixture.cook, "", 200)
	if !board.SoundAlert || !board.StationScope.All || len(board.Stations) != 4 {
		t.Fatalf("terminal con todas y aviso sonoro: %+v %v", board.StationScope, board.SoundAlert)
	}
	if other := fixture.board(t, fixture.admin, "", 200); other.SoundAlert {
		t.Fatal("el aviso sonoro es de cada terminal")
	}
	var audited int
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FROM app.audit_events WHERE action='kitchen.station.access.saved'
        AND entity_id=$1 AND (after_state->>'sound_alert')::boolean`, fixture.cook.deviceID).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Fatalf("auditoría de la terminal: %d", audited)
	}
	// Compatibilidad: sin all_stations, una lista con estaciones es una selección (como antes de la 9F).
	fixture.saveTerminal(t, fixture.cook.deviceID, map[string]any{"station_ids": []string{fixture.postres.ID}, "expected_station_ids": []string{}}, 200)
	if board := fixture.board(t, fixture.cook, "", 200); !slices.Equal(stage9FNames(board.Stations), []string{"Postres 9F"}) || !board.SoundAlert {
		t.Fatalf("selección sin la marca: %v (el aviso sonoro se conserva: %v)", stage9FNames(board.Stations), board.SoundAlert)
	}
	access := decodeResponse[[]map[string]any](t, fixture.adminCall(t, "GET", "kitchen/access", nil, 200))
	for _, terminal := range access {
		if terminal["device_id"] == fixture.cook.deviceID {
			if terminal["all_stations"] != false || terminal["sound_alert"] != true || terminal["home_screen"] != "tables" {
				t.Fatalf("lectura de la terminal: %v", terminal)
			}
		}
	}
}
