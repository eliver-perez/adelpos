//go:build integration

package operations_test

import (
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
)

// Etapa 9F, fase E (§7.1) contra PostgreSQL con la API real: la ruta antigua de anulación (POST
// restaurant/accounts/{id}/void) responde 410 ROUTE_RETIRED sin escribir nada, con y sin permisos.
//
// La 9F también hizo que «Cancelar unidades» y la reducción de cantidades desde «Modificar»
// procedieran sin orders.cancel con la autorización de un supervisor; la etapa 9H lo retiró
// (decisión del propietario) y sus pruebas viven ahora en stage9h_cancel_units_test.go, que usa los
// ayudantes de este archivo.
//
// Usa su propia base desde cero (como la prueba de la 9D, corre después de la aceptación encadenada).

const stage9FPassword = "Clave temporal de "

type stage9FFixture struct {
	*restaurantFixture
	waiter       testTerminal
	supervisor   testTerminal
	weak         testTerminal
	viewer       testTerminal
	waiterID     string
	supervisorID string
}

// stage9FReport es la forma JSON de un reporte que esta prueba necesita.
type stage9FReport struct {
	Columns []struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	} `json:"columns"`
	Rows []map[string]any `json:"rows"`
}

func (fixture *stage9FFixture) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var value string
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// send envía una operación desde la terminal indicada con un operation_id nuevo si no trae uno.
func (fixture *stage9FFixture) send(t *testing.T, terminal testTerminal, method, route string, body map[string]any, status int) []byte {
	t.Helper()
	if _, found := body["operation_id"]; !found {
		body["operation_id"] = randomID()
	}
	return fixture.call(t, terminal, method, route, body, status)
}

// sentAccount abre una cuenta sin mesa con las papas confirmadas (enviadas a cocina).
func (fixture *stage9FFixture) sentAccount(t *testing.T, name string, quantity int) operations.Bill {
	t.Helper()
	bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": name, "mode": "dine_in"}, 200))
	return fixture.capture(t, bill, []catalog.LineRequest{{ProductID: fixture.fries.ID, Quantity: quantity}})
}

func (fixture *stage9FFixture) report(t *testing.T, id string) stage9FReport {
	t.Helper()
	return decodeResponse[stage9FReport](t, fixture.adminCall(t, "GET", "reports/run/"+id, nil, 200))
}

func stage9FAuthorization(username string) map[string]any {
	return map[string]any{"username": username, "password": stage9FPassword + username}
}

// stage9FRow devuelve el renglón del reporte cuyo valor de key es value.
func stage9FRow(t *testing.T, result stage9FReport, key, value string) map[string]any {
	t.Helper()
	for _, row := range result.Rows {
		if row[key] == value {
			return row
		}
	}
	t.Fatalf("No hay renglón con %s=%s: %+v", key, value, result.Rows)
	return nil
}

func TestStage9FRetiredVoid(t *testing.T) {
	resetStage9DDatabase(t)
	fixture := &stage9FFixture{restaurantFixture: newFixture(t)}
	fixture.seedCatalog(t)
	// Un usuario que solo consulta cuentas: tampoco a él se le audita una denegación.
	fixture.viewer = inventoryUser(t, fixture.restaurantFixture, "consulta_9f", []string{"accounts.read"})
	testStage9FRetiredVoid(t, fixture)
}

func testStage9FRetiredVoid(t *testing.T, fixture *stage9FFixture) {
	empty := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": "Vacía 9F", "mode": "dine_in"}, 200))
	counts := `SELECT (SELECT count(*) FROM app.restaurant_operations)||'/'||(SELECT count(*) FROM app.audit_events)||'/'||
        (SELECT count(*) FROM app.restaurant_events)||'/'||(SELECT count(*) FROM app.realtime_events)`
	before := fixture.scalar(t, counts)
	// Con todos los permisos, sin accounts.manage y con una cuenta inexistente: siempre 410, antes de
	// comprobar permisos (tampoco se audita una denegación).
	attempts := []struct {
		terminal testTerminal
		account  string
	}{
		{fixture.admin, empty.Account.ID},
		{fixture.viewer, empty.Account.ID},
		{fixture.admin, randomID()},
	}
	for _, attempt := range attempts {
		response := fixture.send(t, attempt.terminal, "POST", "restaurant/accounts/"+attempt.account+"/void", map[string]any{
			"revision": empty.Account.Revision, "reason": "Cliente se fue sin consumir",
		}, 410)
		problem := decodeResponse[struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}](t, response).Error
		if problem.Code != "ROUTE_RETIRED" || !strings.Contains(problem.Message, "«Cancelar cuenta»") ||
			!strings.Contains(problem.Message, "«Descartar cuenta vacía»") {
			t.Fatalf("Respuesta de la ruta retirada: %+v", problem)
		}
	}
	if after := fixture.scalar(t, counts); after != before {
		t.Fatalf("La ruta retirada escribió registros (operaciones/auditoría/eventos/tiempo real): %s → %s", before, after)
	}
	unchanged := fixture.refresh(t, empty.Account.ID)
	if unchanged.Account.State != "open" || unchanged.Account.Revision != empty.Account.Revision {
		t.Fatalf("La ruta retirada modificó la cuenta: %+v", unchanged.Account)
	}
	// La cuenta vacía sigue el camino vigente: se descarta sin motivo.
	fixture.mutate(t, "POST", "restaurant/accounts/"+empty.Account.ID+"/discard", map[string]any{"revision": empty.Account.Revision}, 200)
}
