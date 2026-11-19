//go:build integration

package operations_test

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
)

// Etapa A2, parte D (registro y canal de platillos listos; A2-D4 y A2-D9) contra PostgreSQL con la API real. Dos
// meseros (Ana y Beto) entran con PIN en dos tabletas; cocina marca «Listo» desde su terminal (KDS). Comprueba:
//   - un aviso por unidad que pasa a «Listo», en la misma operación de cocina, agrupado por cuenta y platillo, sin
//     precios; solo con la credencial del dispositivo y solo en android;
//   - llega al responsable de la cuenta en su dispositivo; otro mesero u otro dispositivo no lo reciben; nada se repite
//     (UNIQUE y cursor) y after recupera lo perdido;
//   - transferir la cuenta (assign) cambia el destinatario de lo que aún no estaba listo;
//   - con varias sesiones vigentes, el dispositivo de actividad más reciente; sin ninguna, todos donde entró hoy;
//     tras auth/logout los avisos ya no se fijan a ese dispositivo;
//   - el día comercial (active, day_ends_at y los meseros de días anteriores), el aviso apagado, sonido y vibración,
//     la poda de lo de más de 2 días, el dispositivo revocado y el canal SSE (Last-Event-ID; un canal nuevo del
//     dispositivo reemplaza al anterior);
//   - las correcciones de la revisión: entrada en el dispositivo, destinatario que puede recibir el aviso, unidades
//     canceladas, cuentas combinadas y last_seen_at sin escribir en cada lectura.
//
// Usa su propia base desde cero.

type readyFixture struct {
	*waiterModeFixture
	ana, beto     string
	tabletOne     testTerminal
	tabletTwo     testTerminal
	anaOne        testTerminal
	betoTwo       testTerminal
	variantFilete string
}

// readyTablet registra un teléfono o tableta de meseros con la aplicación 0.2.0 como si se hubiera emparejado.
func (fixture *readyFixture) readyTablet(t *testing.T, name string) testTerminal {
	t.Helper()
	return fixture.tabletWithVersion(t, name, "0.2.0")
}

// tabletWithVersion registra un teléfono o tableta de meseros con la versión de la aplicación que informó al emparejarse.
func (fixture *readyFixture) tabletWithVersion(t *testing.T, name, version string) testTerminal {
	t.Helper()
	secret := administration.NewToken()
	var id string
	if err := fixture.pool.QueryRow(fixture.ctx, `INSERT INTO app.devices(name,secret_hash,kind,os,app_version,paired_at)
        VALUES ($1,$2,'android','Android 15',$3,now()) RETURNING id`, name, administration.TokenHash(secret), version).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return testTerminal{deviceID: id, deviceToken: secret}
}

// enter es la entrada con PIN de un mesero en una tableta: la tableta con su sesión.
func (fixture *readyFixture) enter(t *testing.T, tablet testTerminal, userID, pin string) testTerminal {
	t.Helper()
	code, response := fixture.raw(tablet, "", "POST", "auth/pin-login", map[string]string{"user_id": userID, "pin": pin})
	login := decodeResponse[struct {
		Session string `json:"session_token"`
	}](t, response)
	if code != 200 || login.Session == "" {
		t.Fatalf("entrada con PIN: %d %s", code, response)
	}
	session := tablet
	session.sessionToken = login.Session
	return session
}

// alerts es GET ready-alerts con solo la credencial de la tableta.
func (fixture *readyFixture) alerts(t *testing.T, tablet testTerminal, after int64) administration.ReadyAlertsBatch {
	t.Helper()
	device := testTerminal{deviceID: tablet.deviceID, deviceToken: tablet.deviceToken}
	code, response := fixture.raw(device, "", "GET", fmt.Sprintf("ready-alerts?after=%d", after), nil)
	if code != 200 {
		t.Fatalf("avisos de la tableta: %d %s", code, response)
	}
	for _, money := range []string{"price", "total", "amount", "subtotal", "balance"} {
		if bytes.Contains(response, []byte(money)) {
			t.Fatalf("los avisos no deben traer importes: %s", response)
		}
	}
	return decodeResponse[administration.ReadyAlertsBatch](t, response)
}

// head es el último aviso registrado.
func (fixture *readyFixture) head(t *testing.T) int64 {
	t.Helper()
	var head int64
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(max(sequence),0) FROM app.ready_notices").Scan(&head); err != nil {
		t.Fatal(err)
	}
	return head
}

// order abre una cuenta en la mesa con sus partidas desde la sesión dada y la envía a cocina.
func (fixture *readyFixture) order(t *testing.T, session testTerminal, table int, items ...operations.DraftItem) string {
	t.Helper()
	opened := fixture.open(t, session, map[string]any{"table_id": fixture.tables[table].ID, "seen_account_ids": nil}, items...)
	fixture.confirm(t, session, opened.Account.ID)
	return opened.Account.ID
}

// orderFor abre desde la caja una cuenta a nombre del mesero dado y la envía a cocina: así no cambia la actividad de
// las sesiones del mesero en sus tabletas.
func (fixture *readyFixture) orderFor(t *testing.T, waiterID string, table int, items ...operations.DraftItem) string {
	t.Helper()
	opened := fixture.open(t, fixture.admin, map[string]any{"table_id": fixture.tables[table].ID, "seen_account_ids": nil,
		"assigned_user_id": waiterID}, items...)
	fixture.confirm(t, fixture.admin, opened.Account.ID)
	return opened.Account.ID
}

// ready marca «Listo» desde la cocina (KDS) las tareas vivas de la cuenta, o solo las del producto indicado.
func (fixture *readyFixture) ready(t *testing.T, accountID, productID string) {
	t.Helper()
	rows, err := fixture.pool.Query(fixture.ctx, `SELECT t.id::text,t.revision FROM app.kitchen_tasks t
        JOIN app.restaurant_item_units u ON u.id=t.unit_id JOIN app.restaurant_item_versions v ON v.id=u.version_id
        WHERE u.account_id=$1 AND t.state NOT IN ('cancelled','ready') AND ($2='' OR v.quote_line->>'product_id'=$2)
        ORDER BY t.id`, accountID, productID)
	if err != nil {
		t.Fatal(err)
	}
	references := []map[string]any{}
	for rows.Next() {
		var id string
		var revision int
		if err := rows.Scan(&id, &revision); err != nil {
			t.Fatal(err)
		}
		references = append(references, map[string]any{"id": id, "revision": revision})
	}
	rows.Close()
	if len(references) == 0 {
		t.Fatal("no hay tareas que marcar")
	}
	fixture.mutateAs(t, fixture.cook, "POST", "kitchen/states", map[string]any{"state": "ready", "tasks": references})
}

// readyRelease marca «Listo» el pedido completo desde la cocina (la acción por pedido del KDS, con su liberación).
func (fixture *readyFixture) readyRelease(t *testing.T, accountID string) {
	t.Helper()
	var releaseID string
	var revision int
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT id::text,revision FROM app.kitchen_releases WHERE account_id=$1
        ORDER BY created_at DESC,id LIMIT 1`, accountID).Scan(&releaseID, &revision); err != nil {
		t.Fatal(err)
	}
	fixture.mutateAs(t, fixture.cook, "POST", "kitchen/states", map[string]any{"state": "ready", "release_id": releaseID, "revision": revision})
}

// ofAccount son los avisos de una cuenta en el lote.
func ofAccount(batch administration.ReadyAlertsBatch, accountID string) []administration.ReadyAlert {
	found := []administration.ReadyAlert{}
	for _, alert := range batch.Alerts {
		if alert.AccountID == accountID {
			found = append(found, alert)
		}
	}
	return found
}

// itemsText resume los platillos de un aviso en orden alfabético («2 × Filete · Grande, 1 × Papas producto»).
func itemsText(alert administration.ReadyAlert) string {
	parts := []string{}
	for _, item := range alert.Items {
		parts = append(parts, fmt.Sprintf("%d × %s", item.Quantity, item.ProductName))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func TestStageA2ReadyAlerts(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &readyFixture{waiterModeFixture: &waiterModeFixture{restaurantFixture: newFixture(t)}}
	fixture.seedCatalog(t)
	fixture.station = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina A2-D", Active: true, Reception: "kds", TargetMinutes: 12, WarningMinutes: 15, OverdueMinutes: 25,
		Grouping: "inherit", Tracking: "inherit",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare", StationIDs: []string{fixture.station.ID}}, 200)
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón A2-D", Active: true}, 200))
	for index := range 8 {
		fixture.tables = append(fixture.tables, decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: fmt.Sprintf("Mesa %d", index+1), Shape: "round", X: 20 + index*110, Y: 30,
			Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200)))
	}
	fixture.cook = inventoryUser(t, fixture.restaurantFixture, "cocina_a2d", []string{
		"kitchen.states.read", "kitchen.states.manage", "kitchen.stations.all", "events.read",
	})
	inventoryUser(t, fixture.restaurantFixture, "ana_a2d", waiterPermissions)
	inventoryUser(t, fixture.restaurantFixture, "beto_a2d", waiterPermissions)
	for username, target := range map[string]*string{"ana_a2d": &fixture.ana, "beto_a2d": &fixture.beto} {
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username=$1", username).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	fixture.adminCall(t, "PUT", "users/"+fixture.ana+"/pin", map[string]any{"pin": "4829", "short_name": "Ana", "badge_color": ""}, 200)
	fixture.adminCall(t, "PUT", "users/"+fixture.beto+"/pin", map[string]any{"pin": "6152", "short_name": "Beto", "badge_color": ""}, 200)
	fixture.tabletOne = fixture.readyTablet(t, "Tableta A2-D 1")
	fixture.tabletTwo = fixture.readyTablet(t, "Tableta A2-D 2")
	fixture.variantFilete = fixture.product.Variants[0].ID

	t.Run("credencial del dispositivo: sin mesero que haya entrado hoy no hay servicio; solo android", func(t *testing.T) {
		testStageA2ReadyAccess(t, fixture)
	})
	t.Run("el aviso llega una vez al responsable en su dispositivo, agrupado y sin precios", func(t *testing.T) {
		testStageA2ReadyRecipient(t, fixture)
	})
	t.Run("transferir la cuenta cambia el destinatario de lo que aún no estaba listo", func(t *testing.T) {
		testStageA2ReadyTransfer(t, fixture)
	})
	t.Run("varias sesiones, ninguna vigente y auth/logout (A2-D4)", func(t *testing.T) {
		testStageA2ReadySessions(t, fixture)
	})
	t.Run("after recupera lo perdido y nada se repite", func(t *testing.T) {
		testStageA2ReadyRecovery(t, fixture)
	})
	t.Run("canal SSE con Last-Event-ID; un canal nuevo reemplaza al anterior", func(t *testing.T) {
		testStageA2ReadyStream(t, fixture)
	})
	t.Run("Terminales: apagado, sonido y vibración", func(t *testing.T) {
		testStageA2ReadySettings(t, fixture)
	})
	t.Run("correcciones de la revisión: entrada en el dispositivo, destinatario, cancelaciones y combinaciones", func(t *testing.T) {
		testStageA2ReadyCorrections(t, fixture)
	})
	t.Run("día comercial, sesiones de antes del inicio del día (A2-D12), poda y dispositivo revocado", func(t *testing.T) {
		testStageA2ReadyDayAndRevocation(t, fixture)
	})
}

func testStageA2ReadyAccess(t *testing.T, fixture *readyFixture) {
	config, err := businessday.Load(fixture.ctx, fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	batch := fixture.alerts(t, fixture.tabletOne, 0)
	if batch.Active || batch.Cursor != 0 || len(batch.Alerts) != 0 || !batch.Settings.Enabled || !batch.Settings.Sound ||
		!batch.Settings.Vibration || !batch.DayEndsAt.Equal(config.DayOf(time.Now()).Deadline) {
		t.Fatalf("tableta sin meseros: %+v", batch)
	}
	// Una terminal de escritorio no recibe avisos; tampoco sin credencial, con un parámetro desconocido o un cursor
	// inválido, ni con la credencial de envío de un mesero.
	desktop := testTerminal{deviceID: fixture.admin.deviceID, deviceToken: fixture.admin.deviceToken}
	device := testTerminal{deviceID: fixture.tabletOne.deviceID, deviceToken: fixture.tabletOne.deviceToken}
	for name, check := range map[string]struct {
		terminal   testTerminal
		credential string
		route      string
		status     int
		code       string
	}{
		"escritorio":          {desktop, "", "ready-alerts", 403, "READY_ALERTS_DEVICE_NOT_ALLOWED"},
		"sin credencial":      {testTerminal{}, "", "ready-alerts", 401, "DEVICE_REQUIRED"},
		"parámetro":           {device, "", "ready-alerts?after=0&user_id=x", 400, ""},
		"cursor":              {device, "", "ready-alerts?after=-1", 400, ""},
		"credencial de envío": {device, administration.NewToken(), "ready-alerts", 401, "OFFLINE_CREDENTIAL_NOT_ALLOWED"},
	} {
		code, response := fixture.raw(check.terminal, check.credential, "GET", check.route, nil)
		if code != check.status || (check.code != "" && errorCode(t, response) != check.code) {
			t.Fatalf("%s: %d %s", name, code, response)
		}
	}
	// Con un mesero que entró con PIN hoy, el servicio debe estar activo.
	fixture.anaOne = fixture.enter(t, fixture.tabletOne, fixture.ana, "4829")
	fixture.betoTwo = fixture.enter(t, fixture.tabletTwo, fixture.beto, "6152")
	if !fixture.alerts(t, fixture.tabletOne, 0).Active || !fixture.alerts(t, fixture.tabletTwo, 0).Active {
		t.Fatal("con un mesero que entró hoy el servicio debe estar activo")
	}
}

func testStageA2ReadyRecipient(t *testing.T, fixture *readyFixture) {
	before := fixture.head(t)
	filete := draftLine(fixture.product, 2)
	filete.VariantID = &fixture.variantFilete
	accountID := fixture.order(t, fixture.anaOne, 0, filete, draftLine(fixture.fries, 1))
	if fixture.head(t) != before {
		t.Fatal("enviar a cocina no es «Listo»")
	}
	fixture.ready(t, accountID, "")
	// Un aviso por unidad, con el responsable y su dispositivo, en la misma operación de cocina que el evento.
	if fixture.count(t, `SELECT count(*)::int FROM app.ready_notices WHERE account_id=$1 AND recipient_user_id=$2
        AND recipient_device_id=$3`, accountID, fixture.ana, fixture.tabletOne.deviceID) != 3 {
		t.Fatal("se esperaban tres avisos, uno por unidad")
	}
	batch := fixture.alerts(t, fixture.tabletOne, before)
	alerts := ofAccount(batch, accountID)
	if len(alerts) != 1 || batch.Cursor != fixture.head(t) || alerts[0].ID != batch.Cursor || alerts[0].Mode != "dine_in" ||
		alerts[0].TableName == nil || *alerts[0].TableName != "Mesa 1" || alerts[0].WaiterUserID != fixture.ana ||
		alerts[0].WaiterShortName != "Ana" || alerts[0].CustomerName != "" || alerts[0].Folio < 1 ||
		itemsText(alerts[0]) != "1 × Papas producto, 2 × Filete · Grande" {
		t.Fatalf("aviso de la mesa: %+v", batch)
	}
	// Desde el cursor no se repite; marcar «Listo» otra vez tampoco registra nada.
	if again := fixture.alerts(t, fixture.tabletOne, batch.Cursor); len(again.Alerts) != 0 || again.Cursor != batch.Cursor {
		t.Fatalf("aviso repetido: %+v", again)
	}
	// Otro mesero en otro dispositivo no lo recibe (su cursor avanza igual).
	other := fixture.alerts(t, fixture.tabletTwo, before)
	if len(other.Alerts) != 0 || other.Cursor != batch.Cursor {
		t.Fatalf("la tableta de Beto recibió el aviso de Ana: %+v", other)
	}
}

func testStageA2ReadyTransfer(t *testing.T, fixture *readyFixture) {
	before := fixture.head(t)
	accountID := fixture.order(t, fixture.anaOne, 1, draftLine(fixture.product, 1), draftLine(fixture.fries, 1))
	fixture.ready(t, accountID, fixture.product.ID)
	current := fixture.refresh(t, accountID)
	fixture.mutate(t, "POST", "restaurant/accounts/"+accountID+"/assign", map[string]any{
		"revision": current.Account.Revision, "assigned_user_id": fixture.beto,
	}, 200)
	fixture.ready(t, accountID, fixture.fries.ID)
	ana := ofAccount(fixture.alerts(t, fixture.tabletOne, before), accountID)
	beto := ofAccount(fixture.alerts(t, fixture.tabletTwo, before), accountID)
	if len(ana) != 1 || itemsText(ana[0]) != "1 × Filete" || ana[0].WaiterUserID != fixture.ana ||
		len(beto) != 1 || itemsText(beto[0]) != "1 × Papas producto" || beto[0].WaiterUserID != fixture.beto ||
		beto[0].WaiterShortName != "Beto" {
		t.Fatalf("destinatarios tras transferir: Ana %+v, Beto %+v", ana, beto)
	}
}

func testStageA2ReadySessions(t *testing.T, fixture *readyFixture) {
	// Ana entra también en la tableta 2: su sesión más reciente es esa.
	anaTwo := fixture.enter(t, fixture.tabletTwo, fixture.ana, "4829")
	expect := func(name string, wantOne, wantTwo bool) {
		t.Helper()
		before := fixture.head(t)
		accountID := fixture.orderFor(t, fixture.ana, 2, draftLine(fixture.fries, 1))
		fixture.ready(t, accountID, "")
		one := len(ofAccount(fixture.alerts(t, fixture.tabletOne, before), accountID)) == 1
		two := len(ofAccount(fixture.alerts(t, fixture.tabletTwo, before), accountID)) == 1
		if one != wantOne || two != wantTwo {
			t.Fatalf("%s: tableta 1 %v, tableta 2 %v", name, one, two)
		}
	}
	expect("sesión más reciente en la tableta 2", false, true)
	// Ana usa la tableta 1: ahora esa es la de actividad más reciente.
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.sessions SET last_seen_at=now()-interval '1 minute' WHERE user_id=$1 AND device_id=$2",
		fixture.ana, fixture.tabletTwo.deviceID); err != nil {
		t.Fatal(err)
	}
	fixture.call(t, fixture.anaOne, "GET", "auth/me", nil, 200)
	expect("actividad más reciente en la tableta 1", true, false)
	// Cerrar la sesión con auth/logout deja de contar: la vigente es la de la tableta 2.
	fixture.call(t, fixture.anaOne, "POST", "auth/logout", nil, 200)
	expect("después de cerrar la sesión de la tableta 1", false, true)
	// Sin ninguna vigente, llega a todos los dispositivos donde entró hoy.
	fixture.call(t, anaTwo, "POST", "auth/logout", nil, 200)
	expect("sin sesiones vigentes", true, true)
	// Vuelve a entrar en la tableta 1 para lo que sigue.
	fixture.anaOne = fixture.enter(t, fixture.tabletOne, fixture.ana, "4829")
}

func testStageA2ReadyRecovery(t *testing.T, fixture *readyFixture) {
	// La tableta 1 se quedó sin red con su cursor; mientras, cocina marca «Listo» tres cuentas de Ana (la última con la
	// acción «Listo» del pedido completo).
	cursor := fixture.alerts(t, fixture.tabletOne, fixture.head(t)).Cursor
	accounts := []string{}
	for table := 3; table < 6; table++ {
		accountID := fixture.order(t, fixture.anaOne, table, draftLine(fixture.fries, 2))
		if table == 5 {
			fixture.readyRelease(t, accountID)
		} else {
			fixture.ready(t, accountID, "")
		}
		accounts = append(accounts, accountID)
	}
	recovered := fixture.alerts(t, fixture.tabletOne, cursor)
	if len(recovered.Alerts) != 3 || recovered.Cursor != fixture.head(t) {
		t.Fatalf("avisos recuperados: %+v", recovered)
	}
	for index, alert := range recovered.Alerts {
		if alert.AccountID != accounts[index] || itemsText(alert) != "2 × Papas producto" ||
			(index > 0 && alert.ID <= recovered.Alerts[index-1].ID) {
			t.Fatalf("aviso recuperado %d: %+v", index, alert)
		}
	}
	// Desde el cursor nuevo, nada; con el anterior, los mismos (mismos id: el dispositivo descarta los que ya avisó).
	if again := fixture.alerts(t, fixture.tabletOne, recovered.Cursor); len(again.Alerts) != 0 {
		t.Fatalf("aviso repetido: %+v", again)
	}
	repeated := fixture.alerts(t, fixture.tabletOne, cursor)
	if len(repeated.Alerts) != 3 || repeated.Alerts[2].ID != recovered.Alerts[2].ID {
		t.Fatalf("relectura desde el cursor anterior: %+v", repeated)
	}
	// Cada aviso tiene su clave (la que el dispositivo usa para descartar repetidos): distinta entre avisos e igual al
	// releerlo.
	keys := map[string]bool{}
	for index, alert := range recovered.Alerts {
		if alert.Key == "" || keys[alert.Key] || repeated.Alerts[index].Key != alert.Key {
			t.Fatalf("claves de los avisos: %+v / %+v", recovered.Alerts, repeated.Alerts)
		}
		keys[alert.Key] = true
	}
	// UNIQUE (unit_id, version_id): la misma preparación no se registra dos veces.
	if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.ready_notices(unit_id,version_id,account_id,recipient_user_id,alert_key)
        SELECT unit_id,version_id,account_id,recipient_user_id,gen_random_uuid() FROM app.ready_notices LIMIT 1`); err == nil {
		t.Fatal("se registró dos veces la misma preparación")
	}
}

func testStageA2ReadyStream(t *testing.T, fixture *readyFixture) {
	server := httptest.NewServer(fixture.service)
	defer server.Close()
	open := func(after string, lastEventID string) *http.Response {
		t.Helper()
		request, err := http.NewRequest("GET", server.URL+"/api/v1/ready-alerts/stream"+after, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Device-ID", fixture.tabletOne.deviceID)
		request.Header.Set("X-Device-Token", fixture.tabletOne.deviceToken)
		if lastEventID != "" {
			request.Header.Set("Last-Event-ID", lastEventID)
		}
		response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	type message struct {
		id    string
		event string
		batch administration.ReadyAlertsBatch
	}
	next := func(reader *bufio.Reader) message {
		t.Helper()
		current := message{}
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal("el canal no entregó el mensaje", err)
			}
			switch {
			case strings.HasPrefix(line, "id: "):
				current.id = strings.TrimSpace(strings.TrimPrefix(line, "id: "))
			case strings.HasPrefix(line, "event: "):
				current.event = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			case strings.HasPrefix(line, "data: "):
				current.batch = decodeResponse[administration.ReadyAlertsBatch](t, []byte(strings.TrimPrefix(line, "data: ")))
				return current
			}
		}
	}
	head := fixture.head(t)
	stream := open(fmt.Sprintf("?after=%d", head), "")
	defer stream.Body.Close()
	if stream.StatusCode != 200 || !strings.HasPrefix(stream.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("canal de avisos: %d", stream.StatusCode)
	}
	reader := bufio.NewReader(stream.Body)
	first := next(reader)
	if first.event != "alerts" || first.id != fmt.Sprint(head) || !first.batch.Active || len(first.batch.Alerts) != 0 {
		t.Fatalf("primer mensaje: %+v", first)
	}
	// Cocina marca «Listo»: llega por el canal con su id (sin novedades el canal no manda mensajes).
	accountID := fixture.order(t, fixture.anaOne, 6, draftLine(fixture.fries, 1))
	fixture.ready(t, accountID, "")
	received := next(reader)
	alerts := ofAccount(received.batch, accountID)
	if received.event != "alerts" || len(alerts) != 1 || alerts[0].Key == "" || received.id != fmt.Sprint(fixture.head(t)) {
		t.Fatalf("aviso por el canal: %+v", received)
	}
	// Un segundo canal del mismo dispositivo (tras un cambio de red) reemplaza al primero, que se cierra (revisión R12);
	// con Last-Event-ID anterior, el aviso vuelve a llegar con la misma clave (el dispositivo descarta el repetido).
	resumed := open("", first.id)
	defer resumed.Body.Close()
	if resumed.StatusCode != 200 {
		t.Fatalf("segundo canal del dispositivo: %d", resumed.StatusCode)
	}
	again := next(bufio.NewReader(resumed.Body))
	if repeated := ofAccount(again.batch, accountID); len(repeated) != 1 || repeated[0].Key != alerts[0].Key {
		t.Fatalf("reconexión con Last-Event-ID: %+v", again)
	}
	closed := make(chan error, 1)
	go func() {
		for {
			if _, err := reader.ReadString('\n'); err != nil {
				closed <- err
				return
			}
		}
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("el canal anterior debía cerrarse al abrir otro del mismo dispositivo")
	}
	stream.Body.Close()
}

func testStageA2ReadySettings(t *testing.T, fixture *readyFixture) {
	route := "devices/" + fixture.tabletOne.deviceID
	saved := decodeResponse[map[string]any](t, fixture.adminCall(t, "PATCH", route, map[string]any{
		"name": "Tableta A2-D 1", "register_id": nil, "keyboard_mode": "auto",
		"ready_alerts_sound": false, "ready_alerts_vibration": false,
	}, 200))
	if saved["ready_alerts_enabled"] != true || saved["ready_alerts_sound"] != false || saved["ready_alerts_vibration"] != false {
		t.Fatalf("sonido y vibración: %v", saved)
	}
	batch := fixture.alerts(t, fixture.tabletOne, fixture.head(t))
	if !batch.Active || batch.Settings != (administration.ReadyAlertSettings{Enabled: true, Sound: false, Vibration: false}) {
		t.Fatalf("ajustes en los avisos: %+v", batch.Settings)
	}
	terminal := decodeResponse[struct {
		Terminal map[string]any `json:"terminal"`
	}](t, fixture.call(t, fixture.anaOne, "GET", "auth/me", nil, 200)).Terminal
	if terminal["ready_alerts_enabled"] != true || terminal["ready_alerts_sound"] != false || terminal["ready_alerts_vibration"] != false {
		t.Fatalf("terminal de auth/me: %v", terminal)
	}
	// Apagado: active false y sin avisos (no es un error), aunque cocina siga marcando «Listo»; el cursor avanza.
	fixture.adminCall(t, "PATCH", route, map[string]any{"name": "Tableta A2-D 1", "register_id": nil, "keyboard_mode": "auto",
		"ready_alerts_enabled": false}, 200)
	before := fixture.head(t)
	accountID := fixture.order(t, fixture.anaOne, 7, draftLine(fixture.fries, 1))
	fixture.ready(t, accountID, "")
	off := fixture.alerts(t, fixture.tabletOne, before)
	if off.Active || off.Settings.Enabled || len(off.Alerts) != 0 || off.Cursor != fixture.head(t) {
		t.Fatalf("avisos apagados: %+v", off)
	}
	configuration := decodeResponse[struct {
		Device map[string]any `json:"device"`
	}](t, fixture.adminCall(t, "GET", route+"/configuration", nil, 200))
	if configuration.Device["ready_alerts_enabled"] != false || configuration.Device["ready_alerts_sound"] != false {
		t.Fatalf("configuración de la tableta: %v", configuration.Device)
	}
	// Sin los campos se conserva lo guardado; en una terminal de escritorio no se configuran.
	kept := decodeResponse[map[string]any](t, fixture.adminCall(t, "PATCH", route, map[string]any{
		"name": "Tableta A2-D 1", "register_id": nil, "keyboard_mode": "auto"}, 200))
	if kept["ready_alerts_enabled"] != false || kept["ready_alerts_vibration"] != false {
		t.Fatalf("sin los campos debe conservarse: %v", kept)
	}
	code, response := fixture.raw(fixture.admin, "", "PATCH", "devices/"+fixture.admin.deviceID, map[string]any{
		"name": "Prueba principal", "register_id": nil, "keyboard_mode": "auto", "ready_alerts_enabled": false})
	if code != 400 || !strings.Contains(string(response), "solo se configuran en teléfonos y tabletas") {
		t.Fatalf("avisos en una terminal de escritorio: %d %s", code, response)
	}
	fixture.adminCall(t, "PATCH", route, map[string]any{"name": "Tableta A2-D 1", "register_id": nil, "keyboard_mode": "auto",
		"ready_alerts_enabled": true, "ready_alerts_sound": true, "ready_alerts_vibration": true}, 200)
}

func testStageA2ReadyDayAndRevocation(t *testing.T, fixture *readyFixture) {
	// La poda: lo de más de 2 días desaparece al registrar el siguiente aviso.
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.ready_notices SET ready_at=now()-interval '3 days'
        WHERE sequence=(SELECT min(sequence) FROM app.ready_notices)`); err != nil {
		t.Fatal(err)
	}
	oldest := fixture.count(t, "SELECT min(sequence)::int FROM app.ready_notices")
	accountID := fixture.order(t, fixture.anaOne, 0, draftLine(fixture.fries, 1))
	fixture.ready(t, accountID, "")
	if fixture.count(t, "SELECT count(*)::int FROM app.ready_notices WHERE sequence=$1", oldest) != 0 {
		t.Fatal("no se podó el aviso de hace 3 días")
	}
	// Un aviso de antes del inicio del día comercial no llega, aunque el cursor lo deje atrás.
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.ready_notices SET ready_at=now()-interval '30 hours'
        WHERE account_id=$1`, accountID); err != nil {
		t.Fatal(err)
	}
	if yesterday := ofAccount(fixture.alerts(t, fixture.tabletOne, 0), accountID); len(yesterday) != 0 {
		t.Fatalf("aviso de otro día comercial: %+v", yesterday)
	}
	// A2-D12 (revisión RD2): una sesión con PIN de antes del inicio del día comercial, aún vigente (la última entrada de
	// la noche dura 12 h), solo cuenta si se usó en los últimos 30 minutos; si no, el servicio se detiene de noche.
	config, err := businessday.Load(fixture.ctx, fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	dayStart := config.Start(config.DayOf(time.Now()).Date)
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.sessions SET created_at=$2,
        last_seen_at=now()-interval '31 minutes',expires_at=now()+interval '6 hours' WHERE device_id=$1`,
		fixture.tabletOne.deviceID, dayStart.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if night := fixture.alerts(t, fixture.tabletOne, 0); night.Active || len(night.Alerts) != 0 {
		t.Fatalf("sesión de antes del inicio del día sin usar desde hace 31 minutos: %+v", night)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.sessions SET last_seen_at=now()-interval '5 minutes'
        WHERE device_id=$1 AND revoked_at IS NULL`, fixture.tabletOne.deviceID); err != nil {
		t.Fatal(err)
	}
	if working := fixture.alerts(t, fixture.tabletOne, 0); !working.Active {
		t.Fatalf("sesión de antes del inicio del día usada hace 5 minutos: %+v", working)
	}
	// Los meseros que entraron en días anteriores ya no cuentan: sin nadie de hoy, el servicio se detiene.
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.sessions SET created_at=now()-interval '2 days',
        last_seen_at=now()-interval '2 days',expires_at=now()-interval '1 day' WHERE device_id=$1`, fixture.tabletOne.deviceID); err != nil {
		t.Fatal(err)
	}
	if stale := fixture.alerts(t, fixture.tabletOne, 0); stale.Active || len(stale.Alerts) != 0 {
		t.Fatalf("tableta sin meseros de hoy: %+v", stale)
	}
	// Retirar el dispositivo: 401 DEVICE_REVOKED.
	fixture.mutate(t, "POST", "devices/"+fixture.tabletTwo.deviceID+"/revoke", map[string]any{}, 200)
	device := testTerminal{deviceID: fixture.tabletTwo.deviceID, deviceToken: fixture.tabletTwo.deviceToken}
	if code, response := fixture.raw(device, "", "GET", "ready-alerts", nil); code != 401 || errorCode(t, response) != "DEVICE_REVOKED" {
		t.Fatalf("dispositivo revocado: %d %s", code, response)
	}
}

// testStageA2ReadyCorrections cubre las correcciones de la revisión independiente que no reproduce
// TestStageA2ReadyReview: R11 (un dispositivo no recibe los avisos de antes de que el mesero entrara en él), R7 (el
// dispositivo destinatario se elige solo entre los que pueden recibir el aviso), R10 (la cuenta vigente si se combinó y
// sin unidades canceladas después) y R4 (la lectura no escribe «última conexión» cada vez).
func testStageA2ReadyCorrections(t *testing.T, fixture *readyFixture) {
	// R11: Beto no tiene ninguna sesión vigente; su aviso va a todos donde entró hoy, pero no a la tableta en la que
	// entra después.
	before := fixture.head(t)
	fixture.call(t, fixture.betoTwo, "POST", "auth/logout", nil, 200)
	noticeFor := fixture.orderFor(t, fixture.beto, 3, draftLine(fixture.fries, 1))
	fixture.ready(t, noticeFor, "")
	tabletThree := fixture.readyTablet(t, "Tableta A2-D 3")
	fixture.enter(t, tabletThree, fixture.beto, "6152")
	if late := ofAccount(fixture.alerts(t, tabletThree, 0), noticeFor); len(late) != 0 {
		t.Fatalf("la tableta en la que Beto entró después recibió el aviso de antes: %+v", late)
	}
	if earlier := ofAccount(fixture.alerts(t, fixture.tabletTwo, before), noticeFor); len(earlier) != 1 {
		t.Fatalf("la tableta donde Beto ya había entrado hoy debía recibirlo: %+v", earlier)
	}
	fixture.betoTwo = fixture.enter(t, fixture.tabletTwo, fixture.beto, "6152")

	// R7: la sesión más reciente de Ana está en una tableta con la aplicación 0.1.0 (no lee avisos): el aviso se fija a
	// la tableta 1; con los avisos de la tableta 1 apagados, no queda ninguna que pueda recibirlo y va a todas.
	old := fixture.tabletWithVersion(t, "Tableta A2-D 0.1.0", "0.1.0")
	fixture.enter(t, old, fixture.ana, "4829")
	pinned := fixture.orderFor(t, fixture.ana, 4, draftLine(fixture.fries, 1))
	fixture.ready(t, pinned, "")
	if fixture.count(t, "SELECT count(*)::int FROM app.ready_notices WHERE account_id=$1 AND recipient_device_id=$2",
		pinned, fixture.tabletOne.deviceID) != 1 {
		t.Fatal("el aviso debía fijarse a la tableta 1, la más reciente que puede recibirlo")
	}
	route := "devices/" + fixture.tabletOne.deviceID
	fixture.adminCall(t, "PATCH", route, map[string]any{"name": "Tableta A2-D 1", "register_id": nil, "keyboard_mode": "auto",
		"ready_alerts_enabled": false}, 200)
	unpinned := fixture.orderFor(t, fixture.ana, 5, draftLine(fixture.fries, 1))
	fixture.ready(t, unpinned, "")
	if fixture.count(t, "SELECT count(*)::int FROM app.ready_notices WHERE account_id=$1 AND recipient_device_id IS NULL",
		unpinned) != 1 {
		t.Fatal("sin un dispositivo que pueda recibirlo, el aviso debía quedar para todos")
	}
	fixture.adminCall(t, "PATCH", route, map[string]any{"name": "Tableta A2-D 1", "register_id": nil, "keyboard_mode": "auto",
		"ready_alerts_enabled": true}, 200)
	fixture.mutate(t, "POST", "devices/"+old.deviceID+"/revoke", map[string]any{}, 200)
	fixture.call(t, fixture.anaOne, "GET", "auth/me", nil, 200)

	// R10: una unidad cancelada después del «Listo» no se avisa; si se cancelan todas, no hay aviso (el cursor avanza);
	// una cuenta combinada después se avisa con la cuenta en que se combinó.
	before = fixture.head(t)
	partly := fixture.order(t, fixture.anaOne, 6, draftLine(fixture.fries, 2), draftLine(fixture.product, 1))
	whole := fixture.order(t, fixture.anaOne, 7, draftLine(fixture.fries, 1))
	merged := fixture.order(t, fixture.anaOne, 0, draftLine(fixture.fries, 1))
	target := fixture.order(t, fixture.anaOne, 1, draftLine(fixture.product, 1))
	for _, accountID := range []string{partly, whole, merged} {
		fixture.ready(t, accountID, "")
	}
	cancel := func(accountID, productID string) {
		t.Helper()
		bill := fixture.refresh(t, accountID)
		for _, line := range bill.Lines {
			if line.Request.ProductID == productID && line.State == "active" {
				fixture.mutate(t, "POST", "restaurant/accounts/"+accountID+"/cancel", map[string]any{
					"revision": bill.Account.Revision, "version_id": line.VersionID, "quantity": line.Quantity,
					"reason": "El cliente ya no lo quiso",
				}, 200)
				return
			}
		}
		t.Fatalf("la cuenta no tiene %s activo", productID)
	}
	cancel(partly, fixture.product.ID)
	cancel(whole, fixture.fries.ID)
	source := fixture.refresh(t, merged)
	destination := fixture.refresh(t, target)
	fixture.mutate(t, "POST", "restaurant/accounts/"+merged+"/merge", map[string]any{
		"revision": source.Account.Revision, "target_account_id": target, "target_revision": destination.Account.Revision,
	}, 200)
	batch := fixture.alerts(t, fixture.tabletOne, before)
	if alerts := ofAccount(batch, partly); len(alerts) != 1 || itemsText(alerts[0]) != "2 × Papas producto" {
		t.Fatalf("aviso con una unidad cancelada después: %+v", alerts)
	}
	if alerts := ofAccount(batch, whole); len(alerts) != 0 {
		t.Fatalf("aviso con todas sus unidades canceladas: %+v", alerts)
	}
	if alerts := ofAccount(batch, target); len(alerts) != 1 || len(ofAccount(batch, merged)) != 0 || itemsText(alerts[0]) != "1 × Papas producto" {
		t.Fatalf("aviso de una cuenta combinada después: %+v", batch.Alerts)
	}
	if batch.Cursor != fixture.head(t) {
		t.Fatalf("el cursor debía avanzar hasta el último aviso: %d de %d", batch.Cursor, fixture.head(t))
	}

	// R4: leer seguido no vuelve a escribir «última conexión» del dispositivo (como mucho una vez por minuto).
	fixture.alerts(t, fixture.tabletOne, fixture.head(t))
	var seen time.Time
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT last_seen_at FROM app.devices WHERE id=$1", fixture.tabletOne.deviceID).
		Scan(&seen); err != nil {
		t.Fatal(err)
	}
	fixture.alerts(t, fixture.tabletOne, fixture.head(t))
	var again time.Time
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT last_seen_at FROM app.devices WHERE id=$1", fixture.tabletOne.deviceID).
		Scan(&again); err != nil || !again.Equal(seen) {
		t.Fatalf("la lectura de avisos volvió a escribir la última conexión: %v → %v (%v)", seen, again, err)
	}
}
