//go:build integration

package operations_test

import (
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa M1, revisor independiente de la concurrencia (§2.3 de la especificación, M1-D11). Es una prueba aparte de la de
// la Fase A (stagem1_concurrency_test.go): tres terminales (dos meseros con «Unir mesas» y la caja) mandan a la vez,
// desde una salida común (en un orden al azar) y cada petición con su propia transacción y su conexión del pool, una mezcla
// de 22 escrituras
// sobre las mismas mesas:
//   - abrir cuentas solas (con y sin la unión vista) y en una unión con seen_union_id;
//   - unir con el primer producto (creación diferida) a uniones distintas que comparten mesas, y el atajo de un grupo;
//   - crecer una mesa ocupada, agregar y quitar mesas de una unión abierta con la misma revisión;
//   - mover una cuenta a una mesa unida, combinar la única cuenta de la unión con otra y cobrarla a la vez;
//   - descartar la cuenta en borrador de una mesa que otra terminal quiere unir;
//   - dos pedidos de la cola sin conexión (uno a una mesa unida con la unión vista y otro a una mesa que se puede unir);
//   - cambiar de área, desactivar y poner fuera de servicio mesas que otras terminales quieren unir.
//
// Al final de cada ronda comprueba, además de las invariantes de la Fase A (expectStageM1Invariants):
//   - ninguna respuesta 5xx ni un error genérico de la base (VALIDATION_ERROR, ALREADY_EXISTS, SERVICE_UNAVAILABLE);
//   - las rutas de uniones solo rechazan con los códigos del contrato §5;
//   - la operación que responde 200 queda registrada y la que se rechaza no deja nada (ni su clave de operación, ni
//     cuentas, ni uniones): las cuentas y las uniones nuevas son exactamente las de las respuestas 200;
//   - ninguna cuenta perdida: todas siguen existiendo, ninguna abierta en una mesa desactivada o fuera de servicio, y
//     cada cuenta sin nombre propio se llama como el lugar en el que quedó (la unión o su mesa, M1-D14);
//   - ninguna mesa de una unión abierta desactivada, fuera de servicio o de otra área; una unión de grupo tiene
//     exactamente las secciones del grupo;
//   - el mapa lee ocupada cada mesa unida, con las cuentas de la unión, y la lista de cuentas de cualquier mesa unida son
//     las cuentas de toda la unión.
//
// Se repite ADEL_M1_REVIEW_ROUNDS veces (10 por omisión); con -count se repite la prueba completa.

// reviewAttempt es una escritura de una terminal en la ronda.
type reviewAttempt struct {
	name     string
	terminal testTerminal
	method   string
	route    string
	body     map[string]any
	// creates: con 200 abre una cuenta nueva; union: con 200 crea una unión nueva.
	creates bool
	union   bool
	// strict: sus rechazos deben ser 409 con un código del contrato de uniones (§5) o TABLE_HAS_ACCOUNT.
	strict bool
}

// reviewUnionCodes son los rechazos de las rutas de uniones que admite el contrato (§5), más TABLE_HAS_ACCOUNT (la
// creación diferida revisa también seen_account_ids).
var reviewUnionCodes = map[string]bool{
	"TABLE_UNION_AREA": true, "TABLE_UNION_OCCUPIED": true, "TABLE_UNION_BOTH_OCCUPIED": true,
	"TABLE_UNION_UNAVAILABLE": true, "TABLE_IN_UNION": true, "TABLE_GROUP_SECTION_OCCUPIED": true,
	"TABLE_UNION_NOT_OPEN": true, "TABLE_UNION_CONFLICT": true, "TABLE_UNION_DEFERRED": true, "TABLE_HAS_ACCOUNT": true,
}

// reviewGenericCodes son las traducciones genéricas de un error de la base (administration.publicFault): si aparecen,
// la base detuvo algo que Go no validó (un índice único, una comprobación o una llave) o la operación falló.
var reviewGenericCodes = map[string]bool{"VALIDATION_ERROR": true, "ALREADY_EXISTS": true, "SERVICE_UNAVAILABLE": true}

func TestStageM1ReviewConcurrency(t *testing.T) {
	fixture := newStageM1Fixture(t)
	rounds := 10
	if value, err := strconv.Atoi(os.Getenv("ADEL_M1_REVIEW_ROUNDS")); err == nil && value > 0 {
		rounds = value
	}
	second := inventoryUser(t, fixture.restaurantFixture, "mesero_m1_revisor", stageM1JoinerPermissions)
	group := decodeResponse[struct {
		Group struct {
			ID string `json:"id"`
		} `json:"group"`
	}](t, fixture.mutate(t, "POST", "restaurant/table-groups", map[string]any{"area_id": fixture.salon.ID, "name": "Mesa 3",
		"table_ids": fixture.ids("3A", "3B", "3C")}, 200)).Group
	tally := map[string]int{}
	t.Cleanup(func() { t.Logf("resultados en %d rondas: %v", rounds, tally) })
	for round := range rounds {
		// Estado de salida: la unión «Mesa 6 + 7» con una cuenta enviada (a1), cuentas solas en Mesa 9 y Mesa 10 y una
		// cuenta en borrador en «5».
		a1 := fixture.sent(t, "Mesa 6", draftLine(fixture.coca, 1))
		u1 := fixture.joinNow(t, "Mesa 6", "Mesa 7")
		a2 := fixture.sent(t, "Mesa 9", draftLine(fixture.coca, 1))
		a3 := fixture.sent(t, "Mesa 10", draftLine(fixture.coca, 1))
		a4 := fixture.openAt(t, fixture.joiner, "5", nil, draftLine(fixture.coca, 1))
		a1 = fixture.refresh(t, a1.Account.ID)
		a3 = fixture.refresh(t, a3.Account.ID)
		finance := decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+a1.Account.ID, nil, 200))
		eight, four, twelve := fixture.table(t, "Mesa 8"), fixture.table(t, "Mesa 4"), fixture.table(t, "Mesa 12")
		version := fixture.catalogVersion(t)
		previous := map[string]bool{a1.Account.ID: true, a2.Account.ID: true, a3.Account.ID: true, a4.Account.ID: true}
		accountsBefore := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts")
		unionsBefore := fixture.count(t, "SELECT count(*)::int FROM app.table_unions")

		open := func(table string, extra map[string]any) map[string]any {
			body := map[string]any{"operation_id": randomID(), "mode": "dine_in", "table_id": fixture.id(table),
				"seen_account_ids": []string{}, "seen_union_id": nil,
				"items": []operations.DraftItem{draftLine(fixture.coca, 1)}}
			for key, value := range extra {
				body[key] = value
			}
			return body
		}
		withOperation := func(body map[string]any) map[string]any {
			body["operation_id"] = randomID()
			return body
		}
		moved := eight
		moved.AreaID = fixture.terraza.ID
		deactivated := four
		deactivated.Active = false
		unionRoute := "restaurant/table-unions/" + u1.ID
		attempts := []reviewAttempt{
			{name: "abrir Mesa 1 sola", terminal: fixture.joiner, method: "POST", route: "restaurant/accounts",
				body: open("Mesa 1", nil), creates: true},
			{name: "unir Mesa 1 + Mesa 2", terminal: second, method: "POST", route: "restaurant/accounts",
				body: open("Mesa 1", map[string]any{"union_table_ids": fixture.ids("Mesa 2")}), creates: true, union: true, strict: true},
			{name: "unir Mesa 2 + Mesa 4", terminal: fixture.joiner, method: "POST", route: "restaurant/accounts",
				body: open("Mesa 2", map[string]any{"union_table_ids": fixture.ids("Mesa 4")}), creates: true, union: true, strict: true},
			{name: "atajo del grupo Mesa 3", terminal: second, method: "POST", route: "restaurant/accounts",
				body: open("3A", map[string]any{"union_group_id": group.ID}), creates: true, union: true, strict: true},
			{name: "abrir 3B sola", terminal: fixture.joiner, method: "POST", route: "restaurant/accounts",
				body: open("3B", nil), creates: true},
			{name: "unir 6 + 5 (5 en borrador)", terminal: second, method: "POST", route: "restaurant/accounts",
				body: open("6", map[string]any{"union_table_ids": fixture.ids("5")}), creates: true, union: true, strict: true},
			{name: "descartar la cuenta de 5", terminal: fixture.joiner, method: "POST",
				route: "restaurant/accounts/" + a4.Account.ID + "/discard", body: map[string]any{"operation_id": randomID(),
					"revision": a4.Account.Revision}},
			{name: "crecer Mesa 9 con Mesa 8", terminal: second, method: "POST", route: "restaurant/table-unions",
				body: map[string]any{"operation_id": randomID(), "start_table_id": fixture.id("Mesa 9"),
					"table_ids": fixture.ids("Mesa 8")}, union: true, strict: true},
			{name: "agregar Mesa 8 a Mesa 6 + 7", terminal: fixture.joiner, method: "POST", route: unionRoute + "/tables",
				body:   map[string]any{"operation_id": randomID(), "revision": u1.Revision, "table_id": fixture.id("Mesa 8")},
				strict: true},
			{name: "agregar Mesa 4 a Mesa 6 + 7", terminal: second, method: "POST", route: unionRoute + "/tables",
				body:   map[string]any{"operation_id": randomID(), "revision": u1.Revision, "table_id": fixture.id("Mesa 4")},
				strict: true},
			{name: "quitar Mesa 7 de Mesa 6 + 7", terminal: fixture.joiner, method: "POST",
				route: unionRoute + "/tables/" + fixture.id("Mesa 7") + "/remove",
				body:  map[string]any{"operation_id": randomID(), "revision": u1.Revision}, strict: true},
			{name: "mover la cuenta de Mesa 10 a Mesa 7", terminal: fixture.joiner, method: "POST",
				route: "restaurant/accounts/" + a3.Account.ID + "/move", body: map[string]any{"operation_id": randomID(),
					"revision": a3.Account.Revision, "table_id": fixture.id("Mesa 7")}},
			{name: "combinar la cuenta de la unión con la de Mesa 10", terminal: fixture.joiner, method: "POST",
				route: "restaurant/accounts/" + a1.Account.ID + "/merge", body: map[string]any{"operation_id": randomID(),
					"revision": a1.Account.Revision, "target_account_id": a3.Account.ID, "target_revision": a3.Account.Revision}},
			{name: "cobrar la única cuenta de la unión", terminal: fixture.admin, method: "POST",
				route: "payments/accounts/" + a1.Account.ID + "/payments", body: map[string]any{"operation_id": randomID(),
					"method_id": fixture.cashMethod, "mode": "total", "expected_balance": finance.Balance}},
			{name: "abrir en la unión desde Mesa 7 con la unión vista", terminal: second, method: "POST",
				route: "restaurant/accounts", body: open("Mesa 7", map[string]any{"seen_union_id": u1.ID,
					"seen_account_ids": []string{a1.Account.ID}}), creates: true},
			{name: "abrir Mesa 6 sin ver la unión", terminal: fixture.joiner, method: "POST", route: "restaurant/accounts",
				body: open("Mesa 6", nil), creates: true},
			{name: "cola sin conexión a Mesa 7 con la unión vista", terminal: fixture.joiner, method: "POST",
				route: "restaurant/offline-orders", body: offlineBody(randomID(), version, map[string]any{
					"table_id": fixture.id("Mesa 7"), "seen_account_ids": []string{a1.Account.ID}, "seen_union_id": u1.ID},
					offlineItem(fixture.coca.ID, 1, "35.00")), creates: true},
			{name: "cola sin conexión a Mesa 2", terminal: second, method: "POST", route: "restaurant/offline-orders",
				body: offlineBody(randomID(), version, map[string]any{"table_id": fixture.id("Mesa 2"),
					"seen_account_ids": []string{}}, offlineItem(fixture.coca.ID, 1, "35.00")), creates: true},
			{name: "Mesa 8 a la terraza", terminal: fixture.admin, method: "PATCH", route: "restaurant/tables/" + eight.ID,
				body: withOperation(structMap(t, moved))},
			{name: "desactivar Mesa 4", terminal: fixture.admin, method: "PATCH", route: "restaurant/tables/" + four.ID,
				body: withOperation(structMap(t, deactivated))},
			{name: "unir Mesa 11 + Mesa 12", terminal: fixture.joiner, method: "POST", route: "restaurant/accounts",
				body: open("Mesa 11", map[string]any{"union_table_ids": fixture.ids("Mesa 12")}), creates: true, union: true,
				strict: true},
			{name: "Mesa 12 fuera de servicio", terminal: fixture.admin, method: "POST",
				route: "restaurant/tables/" + twelve.ID + "/service", body: map[string]any{"operation_id": randomID(),
					"revision": twelve.Revision, "out_of_service": true}},
		}
		// Las peticiones salen juntas, en un orden distinto en cada ronda y con un desfase de 0 a 2 ms, para que cambie
		// quién gana cada carrera.
		start := make(chan struct{})
		var waiting sync.WaitGroup
		statuses := make([]int, len(attempts))
		bodies := make([][]byte, len(attempts))
		for _, index := range rand.Perm(len(attempts)) {
			waiting.Add(1)
			go func(index int, item reviewAttempt, delay time.Duration) {
				defer waiting.Done()
				<-start
				time.Sleep(delay)
				statuses[index], bodies[index] = fixture.raw(item.terminal, "", item.method, item.route, item.body)
			}(index, attempts[index], time.Duration(rand.IntN(3))*time.Millisecond)
		}
		close(start)
		waiting.Wait()

		created, unions := 0, 0
		for index, item := range attempts {
			status := statuses[index]
			result := strconv.Itoa(status)
			if status != 200 {
				code := errorCode(t, bodies[index])
				result += " " + code
				if status >= 500 || reviewGenericCodes[code] {
					t.Fatalf("ronda %d, %s: %d %s", round, item.name, status, bodies[index])
				}
				if item.strict && (status != 409 || !reviewUnionCodes[code]) {
					t.Fatalf("ronda %d, %s: rechazo fuera del contrato de uniones: %d %s", round, item.name, status, bodies[index])
				}
			}
			tally[item.name+" → "+result]++
			// La clave de operación queda solo si la operación se confirmó.
			recorded := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_operations WHERE id=$1",
				item.body["operation_id"]) == 1
			if recorded != (status == 200) {
				t.Fatalf("ronda %d, %s: respondió %d y su clave de operación registrada=%v", round, item.name, status, recorded)
			}
			if status == 200 && item.creates {
				created++
			}
			if status == 200 && item.union {
				unions++
			}
		}
		if count := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts"); count != accountsBefore+created {
			t.Fatalf("ronda %d: %d cuentas, se esperaban %d (+%d de las respuestas 200)", round, count, accountsBefore+created, created)
		}
		if count := fixture.count(t, "SELECT count(*)::int FROM app.table_unions"); count != unionsBefore+unions {
			t.Fatalf("ronda %d: %d uniones, se esperaban %d (+%d de las respuestas 200)", round, count, unionsBefore+unions, unions)
		}
		for id := range previous {
			if fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts WHERE id=$1", id) != 1 {
				t.Fatalf("ronda %d: se perdió la cuenta %s", round, id)
			}
		}
		expectStageM1Invariants(t, fixture.restaurantFixture)
		expectReviewInvariants(t, fixture, round)

		// Se cierra todo para la ronda siguiente: las cuentas en borrador se descartan y las enviadas se cobran (las
		// uniones se deshacen solas); las mesas vuelven a su área, activas y en servicio.
		listed := decodeResponse[struct {
			Items []operations.Account `json:"items"`
		}](t, fixture.adminCall(t, "GET", "restaurant/accounts?state=open&page_size=100", nil, 200))
		for _, account := range listed.Items {
			if account.ConfirmedQuantity == 0 {
				fixture.mutate(t, "POST", "restaurant/accounts/"+account.ID+"/discard",
					map[string]any{"revision": account.Revision}, 200)
				continue
			}
			if account.DraftQuantity > 0 {
				fixture.confirm(t, fixture.admin, account.ID)
			}
			fixture.pay(t, account.ID)
		}
		if fixture.openUnionCount(t) != 0 {
			t.Fatalf("ronda %d: quedaron uniones abiertas sin cuentas", round)
		}
		restored := fixture.table(t, "Mesa 8")
		if restored.AreaID != fixture.salon.ID {
			restored.AreaID = fixture.salon.ID
			fixture.mutate(t, "PATCH", "restaurant/tables/"+restored.ID, restored, 200)
		}
		restored = fixture.table(t, "Mesa 4")
		if !restored.Active {
			restored.Active = true
			fixture.mutate(t, "PATCH", "restaurant/tables/"+restored.ID, restored, 200)
		}
		restored = fixture.table(t, "Mesa 12")
		if restored.OutOfService {
			fixture.mutate(t, "POST", "restaurant/tables/"+restored.ID+"/service",
				map[string]any{"revision": restored.Revision, "out_of_service": false}, 200)
		}
	}
}

// expectReviewInvariants son las comprobaciones del revisor además de las de la Fase A.
func expectReviewInvariants(t *testing.T, fixture *stageM1Fixture, round int) {
	t.Helper()
	checks := map[string]string{
		"cuenta abierta en una mesa desactivada, fuera de servicio o de un área inactiva": `SELECT count(*)::int
            FROM app.restaurant_accounts a JOIN app.restaurant_tables t ON t.id=a.table_id
            JOIN app.restaurant_areas area ON area.id=t.area_id
            WHERE a.state='open' AND (NOT t.active OR t.out_of_service OR NOT area.active)`,
		"mesa de una unión abierta desactivada o fuera de servicio": `SELECT count(*)::int FROM app.table_union_members m
            JOIN app.table_unions u ON u.id=m.union_id JOIN app.restaurant_tables t ON t.id=m.table_id
            WHERE u.state='open' AND m.left_at IS NULL AND (NOT t.active OR t.out_of_service)`,
		// Todas las cuentas de la prueba se abren sin nombre propio: cada una se llama como su lugar (M1-D14).
		"cuenta sin nombre propio que no se llama como su lugar": `SELECT count(*)::int FROM app.restaurant_accounts a
            JOIN app.restaurant_tables t ON t.id=a.table_id
            WHERE a.state='open' AND a.name <> coalesce((SELECT u.name FROM app.table_union_members m
                JOIN app.table_unions u ON u.id=m.union_id WHERE m.table_id=a.table_id AND m.left_at IS NULL
                AND u.state='open'), t.name)`,
		"unión de grupo sin exactamente las secciones del grupo": `SELECT count(*)::int FROM app.table_unions u
            JOIN app.table_groups g ON g.id=u.group_id WHERE u.state='open' AND (u.name<>g.name OR NOT g.active
                OR (SELECT array_agg(m.table_id ORDER BY m.table_id) FROM app.table_union_members m
                    WHERE m.union_id=u.id AND m.left_at IS NULL) IS DISTINCT FROM
                (SELECT array_agg(s.table_id ORDER BY s.table_id) FROM app.table_group_sections s WHERE s.group_id=g.id))`,
		"historial de la unión sin su «joined» o sin su «dissolved»": `SELECT count(*)::int FROM app.table_unions u
            WHERE NOT EXISTS(SELECT FROM app.table_union_events e WHERE e.union_id=u.id AND e.kind='joined')
                OR (u.state='dissolved') <> EXISTS(SELECT FROM app.table_union_events e WHERE e.union_id=u.id
                    AND e.kind='dissolved')`,
	}
	for name, query := range checks {
		if count := fixture.count(t, query); count != 0 {
			t.Fatalf("ronda %d: %s (%d)", round, name, count)
		}
	}
	// El mapa: cada mesa unida se lee ocupada con las cuentas de su unión, y la lista de cuentas de cualquiera de sus
	// mesas son las de toda la unión.
	view := fixture.overview(t)
	tables := map[string]operations.Table{}
	for _, table := range view.Tables {
		tables[table.ID] = table
	}
	for _, union := range view.Unions {
		expected := fixture.count(t, `SELECT count(*)::int FROM app.restaurant_accounts a JOIN app.table_union_members m
            ON m.table_id=a.table_id AND m.left_at IS NULL WHERE m.union_id=$1 AND a.state='open'`, union.ID)
		if union.AccountCount != expected || expected == 0 {
			t.Fatalf("ronda %d: la unión %s dice %d cuentas y tiene %d", round, union.Name, union.AccountCount, expected)
		}
		for _, id := range union.TableIDs {
			table := tables[id]
			if table.UnionID == nil || *table.UnionID != union.ID || table.State == "free" || table.AccountCount != expected {
				t.Fatalf("ronda %d: la mesa %s de la unión %s se lee %s con %d cuentas", round, table.Name, union.Name,
					table.State, table.AccountCount)
			}
		}
		last := union.TableIDs[len(union.TableIDs)-1]
		listed := decodeResponse[struct {
			Items []operations.Account `json:"items"`
		}](t, fixture.adminCall(t, "GET", "restaurant/accounts?state=open&page_size=100&table_id="+last, nil, 200))
		if len(listed.Items) != expected {
			t.Fatalf("ronda %d: la lista de la mesa %s de la unión %s trae %d cuentas de %d", round, tables[last].Name,
				union.Name, len(listed.Items), expected)
		}
		for _, account := range listed.Items {
			if account.TableUnion == nil || account.TableUnion.ID != union.ID {
				t.Fatalf("ronda %d: la cuenta %s de la unión %s no dice su unión: %+v", round, account.ID, union.Name,
					account.TableUnion)
			}
		}
	}
}
