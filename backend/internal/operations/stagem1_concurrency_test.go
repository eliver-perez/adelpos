//go:build integration

package operations_test

import (
	"os"
	"strconv"
	"sync"
	"testing"

	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa M1 (Fase A, §2.3; M1-D11): varias terminales a la vez sobre las mismas mesas. En cada ronda, desde una salida
// común, diez peticiones concurrentes (cada una con su transacción y su conexión del pool, como dos terminales reales):
// abrir solas las mesas B, A y D; unir A+B, C+B y D+C con el primer producto (creación diferida); agregar G y B a la
// unión abierta E+F, quitarle F y cobrar su cuenta, todas con la misma revisión. Al final de cada ronda:
//   - ninguna mesa en dos uniones abiertas, ninguna unión abierta sin cuentas ni con menos de dos mesas;
//   - las cuentas abiertas son exactamente las que respondieron 200 al abrirse (más la de E+F si no se cobró): ninguna
//     se perdió ni se duplicó;
//   - cada perdedora recibió un 409 con un código del contrato (ninguna otra respuesta);
//   - toda unión abierta tiene sus mesas del mismo área y cada mesa unida se lee ocupada.
//
// Se repite ADEL_M1_CONCURRENCY_ROUNDS veces (25 por omisión); con -count se repite la prueba completa.
func TestStageM1Concurrency(t *testing.T) {
	fixture := newStageM1Fixture(t)
	rounds := 25
	if value, err := strconv.Atoi(os.Getenv("ADEL_M1_CONCURRENCY_ROUNDS")); err == nil && value > 0 {
		rounds = value
	}
	names := map[string]string{"A": "Mesa 1", "B": "Mesa 2", "C": "Mesa 4", "D": "Mesa 5", "E": "Mesa 6", "F": "Mesa 7",
		"G": "Mesa 8"}
	allowed := map[string]bool{"TABLE_UNION_OCCUPIED": true, "TABLE_IN_UNION": true, "TABLE_HAS_ACCOUNT": true,
		"TABLE_UNION_NOT_OPEN": true, "TABLE_UNION_CONFLICT": true, "TABLE_GROUP_SECTION_OCCUPIED": true}
	// tally cuenta los resultados de cada intento en todas las rondas, para ver en el registro que hubo competencia.
	tally := map[string]int{}
	t.Cleanup(func() { t.Logf("resultados en %d rondas: %v", rounds, tally) })
	for round := range rounds {
		// La unión E+F con su cuenta, antes de la salida.
		base := fixture.sent(t, names["E"], draftLine(fixture.coca, 1))
		union := fixture.joinNow(t, names["E"], names["F"])
		finance := decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+base.Account.ID, nil, 200))
		type attempt struct {
			name   string
			opens  bool
			pays   bool
			method string
			route  string
			body   map[string]any
		}
		// El cobro sale de la caja (la terminal principal, con su caja); lo demás, de la terminal del mesero.
		open := func(table string, extra map[string]any) map[string]any {
			body := map[string]any{"operation_id": randomID(), "mode": "dine_in", "table_id": fixture.id(names[table]),
				"seen_account_ids": []string{}, "seen_union_id": nil,
				"items": []operations.DraftItem{draftLine(fixture.coca, 1)}}
			for key, value := range extra {
				body[key] = value
			}
			return body
		}
		unionRoute := "restaurant/table-unions/" + union.ID
		attempts := []attempt{
			{"abrir B sola", true, false, "POST", "restaurant/accounts", open("B", nil)},
			{"abrir A sola", true, false, "POST", "restaurant/accounts", open("A", nil)},
			{"abrir D sola", true, false, "POST", "restaurant/accounts", open("D", nil)},
			{"unir A+B", true, false, "POST", "restaurant/accounts", open("A", map[string]any{"union_table_ids": fixture.ids(names["B"])})},
			{"unir C+B", true, false, "POST", "restaurant/accounts", open("C", map[string]any{"union_table_ids": fixture.ids(names["B"])})},
			{"unir D+C", true, false, "POST", "restaurant/accounts", open("D", map[string]any{"union_table_ids": fixture.ids(names["C"])})},
			{"agregar G", false, false, "POST", unionRoute + "/tables", map[string]any{"operation_id": randomID(),
				"revision": union.Revision, "table_id": fixture.id(names["G"])}},
			{"agregar B", false, false, "POST", unionRoute + "/tables", map[string]any{"operation_id": randomID(),
				"revision": union.Revision, "table_id": fixture.id(names["B"])}},
			{"quitar F", false, false, "POST", unionRoute + "/tables/" + fixture.id(names["F"]) + "/remove",
				map[string]any{"operation_id": randomID(), "revision": union.Revision}},
			{"cobrar E+F", false, true, "POST", "payments/accounts/" + base.Account.ID + "/payments", map[string]any{
				"operation_id": randomID(), "method_id": fixture.cashMethod, "mode": "total", "expected_balance": finance.Balance}},
		}
		start := make(chan struct{})
		var group sync.WaitGroup
		statuses := make([]int, len(attempts))
		bodies := make([][]byte, len(attempts))
		for index, item := range attempts {
			group.Add(1)
			go func(index int, item attempt) {
				defer group.Done()
				<-start
				terminal := fixture.joiner
				if item.pays {
					terminal = fixture.admin
				}
				statuses[index], bodies[index] = fixture.raw(terminal, "", item.method, item.route, item.body)
			}(index, item)
		}
		close(start)
		group.Wait()
		opened, paid := 0, false
		for index, item := range attempts {
			result := strconv.Itoa(statuses[index])
			if statuses[index] == 409 {
				result = errorCode(t, bodies[index])
			}
			tally[item.name+" → "+result]++
			switch statuses[index] {
			case 200:
				if item.opens {
					opened++
				}
				if item.pays {
					paid = true
				}
			case 409:
				code := errorCode(t, bodies[index])
				if !allowed[code] {
					t.Fatalf("ronda %d, %s: 409 %s fuera del contrato: %s", round, item.name, code, bodies[index])
				}
			default:
				t.Fatalf("ronda %d, %s: %d %s", round, item.name, statuses[index], bodies[index])
			}
		}
		expectStageM1Invariants(t, fixture.restaurantFixture)
		expected := opened
		if !paid {
			expected++
		}
		if count := fixture.count(t, "SELECT count(*)::int FROM app.restaurant_accounts WHERE state='open'"); count != expected {
			t.Fatalf("ronda %d: %d cuentas abiertas, %d esperadas (abiertas %d, cobrada %v)", round, count, expected, opened, paid)
		}
		// Cada mesa de una unión abierta se lee ocupada (M1-D10).
		view := fixture.overview(t)
		occupied := map[string]bool{}
		for _, table := range view.Tables {
			occupied[table.ID] = table.State != "free" && table.State != "out_of_service"
		}
		for _, open := range view.Unions {
			for _, id := range open.TableIDs {
				if !occupied[id] {
					t.Fatalf("ronda %d: mesa unida libre en el mapa: %+v", round, open)
				}
			}
		}
		// Se cobra todo para la ronda siguiente: las uniones se deshacen solas.
		listed := decodeResponse[struct {
			Items []operations.Account `json:"items"`
		}](t, fixture.adminCall(t, "GET", "restaurant/accounts?state=open&page_size=100", nil, 200))
		for _, account := range listed.Items {
			if account.DraftQuantity > 0 {
				fixture.confirm(t, fixture.admin, account.ID)
			}
			fixture.pay(t, account.ID)
		}
		if fixture.openUnionCount(t) != 0 {
			t.Fatalf("ronda %d: quedaron uniones abiertas sin cuentas", round)
		}
	}
}
