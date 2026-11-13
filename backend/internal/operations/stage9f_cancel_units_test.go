//go:build integration

package operations_test

import (
	"fmt"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
)

// Etapa 9F, fase E (§7.1 y §7.2) contra PostgreSQL con la API real:
//   - la ruta antigua de anulación (POST restaurant/accounts/{id}/void) responde 410 ROUTE_RETIRED sin
//     escribir nada, con y sin permisos;
//   - «Cancelar unidades» y la reducción de cantidades desde «Modificar» siguen el criterio de
//     «Cancelar cuenta»: con orders.cancel basta el permiso propio; sin él, la autorización de un
//     supervisor que lo tenga (sin autorización, autoautorización, contraseña incorrecta y supervisor
//     sin el permiso se rechazan antes de escribir), Go informa si hará falta
//     (cancel_units_requires_authorization en la cuenta y en edit-preview), el evento y la auditoría
//     guardan quién autorizó, la contraseña no queda en ningún registro y un reintento con la misma
//     operación recupera el resultado sin cancelar dos veces;
//   - «Autorizó» en «Modificaciones de cantidades» y «Productos cancelados» y las autorizaciones en
//     «Actividad por empleado».
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

func TestStage9FCancelUnitsAndRetiredVoid(t *testing.T) {
	resetStage9DDatabase(t)
	fixture := &stage9FFixture{restaurantFixture: newFixture(t)}
	fixture.seedCatalog(t)
	// Mesero sin orders.cancel (sí puede abrir cuentas y modificar), supervisor con orders.cancel,
	// encargado con «Cancelar cuentas» pero sin orders.cancel y un usuario que solo consulta cuentas.
	fixture.waiter = inventoryUser(t, fixture.restaurantFixture, "mesero_9f", []string{
		"accounts.read", "accounts.open", "orders.capture", "orders.edit", "catalog.read", "catalog.prices.read",
	})
	fixture.supervisor = inventoryUser(t, fixture.restaurantFixture, "supervisor_9f", []string{"accounts.read", "orders.cancel"})
	fixture.weak = inventoryUser(t, fixture.restaurantFixture, "encargado_9f", []string{"accounts.read", "accounts.cancel"})
	fixture.viewer = inventoryUser(t, fixture.restaurantFixture, "consulta_9f", []string{"accounts.read"})
	fixture.waiterID = fixture.scalar(t, "SELECT id::text FROM app.users WHERE username='mesero_9f'")
	fixture.supervisorID = fixture.scalar(t, "SELECT id::text FROM app.users WHERE username='supervisor_9f'")
	t.Run("la ruta antigua de anulación responde 410 sin escribir nada", func(t *testing.T) {
		testStage9FRetiredVoid(t, fixture)
	})
	t.Run("cancelar unidades con el permiso propio", func(t *testing.T) {
		testStage9FCancelWithPermission(t, fixture)
	})
	t.Run("cancelar unidades sin permiso: rechazos, supervisor e idempotencia", func(t *testing.T) {
		testStage9FCancelWithSupervisor(t, fixture)
	})
	t.Run("reducir la cantidad desde «Modificar» con y sin supervisor", func(t *testing.T) {
		testStage9FEditReduction(t, fixture)
	})
	t.Run("«Autorizó» en los reportes y las autorizaciones del supervisor", func(t *testing.T) {
		testStage9FReports(t, fixture)
	})
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

func testStage9FCancelWithPermission(t *testing.T, fixture *stage9FFixture) {
	bill := fixture.sentAccount(t, "Propio 9F", 3)
	if bill.Account.CancelUnitsRequiresAuthorization {
		t.Fatal("El administrador tiene «Cancelar partidas»: no debía necesitar supervisor")
	}
	route := "restaurant/accounts/" + bill.Account.ID + "/cancel"
	// Con el permiso propio, una autorización enviada se ignora (no se valida ni se registra).
	cancelled := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", route, map[string]any{
		"revision": bill.Account.Revision, "version_id": bill.Lines[0].VersionID, "quantity": 1,
		"reason": "Cliente cambió de opinión", "authorization": map[string]any{"username": "supervisor_9f", "password": "incorrecta"},
	}, 200))
	if cancelled.Account.ConfirmedQuantity != 2 || cancelled.Account.Totals.Total != "80.00" {
		t.Fatalf("Cancelación con el permiso propio: %+v", cancelled.Account)
	}
	if stored := fixture.scalar(t, `SELECT (payload->'after' ? 'authorized_by')::text||'/'||coalesce(payload->'after'->>'authorized_by','nulo')
        FROM app.restaurant_events WHERE account_id=$1 AND kind='item.cancelled'`, bill.Account.ID); stored != "true/nulo" {
		t.Fatalf("Con el permiso propio el evento guarda «Autorizó» vacío: %s", stored)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.audit_events
        WHERE action='restaurant.supervisor_authorized' AND entity_id=$1`, bill.Account.ID); count != 0 {
		t.Fatalf("Sin supervisor no debía auditarse una autorización: %d", count)
	}
	if count := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.login_attempts WHERE username='supervisor_9f'"); count != 0 {
		t.Fatalf("La autorización ignorada no debía contar un intento: %d", count)
	}
}

func testStage9FCancelWithSupervisor(t *testing.T, fixture *stage9FFixture) {
	bill := fixture.sentAccount(t, "Mesero 9F", 3)
	asWaiter := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "GET", "restaurant/accounts/"+bill.Account.ID, nil, 200))
	if !asWaiter.Account.CancelUnitsRequiresAuthorization {
		t.Fatalf("Go debía informar al mesero que hace falta un supervisor: %+v", asWaiter.Account)
	}
	route := "restaurant/accounts/" + bill.Account.ID + "/cancel"
	versionID := bill.Lines[0].VersionID
	body := func(quantity int, authorization map[string]any) map[string]any {
		fields := map[string]any{
			"revision": bill.Account.Revision, "version_id": versionID, "quantity": quantity, "reason": "Se enfrió en la mesa",
		}
		if authorization != nil {
			fields["authorization"] = authorization
		}
		return fields
	}
	// Quien no puede abrir cuentas ni cancelar partidas no llega a pedir la autorización.
	if code := errorCode(t, fixture.send(t, fixture.viewer, "POST", route, body(1, stage9FAuthorization("supervisor_9f")), 403)); code != "PERMISSION_DENIED" {
		t.Fatalf("Usuario de consulta: %s", code)
	}
	// Una cantidad imposible se informa antes de pedir la contraseña (no cuenta un intento).
	if code := errorCode(t, fixture.send(t, fixture.waiter, "POST", route, body(4, map[string]any{"username": "supervisor_9f", "password": "incorrecta"}), 400)); code != "OPERATION_VALIDATION" {
		t.Fatalf("Cantidad imposible: %s", code)
	}
	denied := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action='access.denied'")
	cases := []struct {
		label         string
		authorization map[string]any
		status        int
		code          string
	}{
		{"sin autorización", nil, 403, "AUTHORIZATION_REQUIRED"},
		{"autoautorización", stage9FAuthorization("mesero_9f"), 403, "SELF_AUTHORIZATION"},
		{"contraseña incorrecta", map[string]any{"username": "supervisor_9f", "password": "incorrecta"}, 401, "AUTHORIZATION_INVALID"},
		{"supervisor sin «Cancelar partidas»", stage9FAuthorization("encargado_9f"), 403, "AUTHORIZATION_INSUFFICIENT"},
	}
	for _, entry := range cases {
		if code := errorCode(t, fixture.send(t, fixture.waiter, "POST", route, body(2, entry.authorization), entry.status)); code != entry.code {
			t.Fatalf("%s: %s", entry.label, code)
		}
	}
	unchanged := fixture.refresh(t, bill.Account.ID)
	if unchanged.Account.Revision != bill.Account.Revision || unchanged.Account.ConfirmedQuantity != 3 {
		t.Fatalf("Un rechazo modificó la cuenta: %+v", unchanged.Account)
	}
	if count := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action='access.denied'"); count != denied+3 {
		t.Fatalf("Denegaciones auditadas: %d → %d", denied, count)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT failures FROM app.login_attempts WHERE device_id=$1 AND username='supervisor_9f'`,
		fixture.waiter.deviceID); count != 1 {
		t.Fatalf("El intento fallido debía contarse en el límite de intentos: %d", count)
	}
	// Con la autorización del supervisor procede y registra quién canceló y quién autorizó.
	authorized := body(2, stage9FAuthorization("supervisor_9f"))
	authorized["operation_id"] = randomID()
	cancelled := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "POST", route, authorized, 200))
	if cancelled.Account.ConfirmedQuantity != 1 || cancelled.Account.Totals.Total != "40.00" {
		t.Fatalf("Cancelación autorizada: %+v", cancelled.Account)
	}
	stored := fixture.scalar(t, `SELECT author_id::text||'/'||(payload->'after'->>'authorized_by')||'/'||(payload->'after'->>'authorized_by_username')||'/'||
            (payload->'after'->>'authorized_by_name')||'/'||(payload->'after'->>'reason')
        FROM app.restaurant_events WHERE account_id=$1 AND kind='item.cancelled'`, bill.Account.ID)
	if stored != fixture.waiterID+"/"+fixture.supervisorID+"/supervisor_9f/Prueba supervisor_9f/Se enfrió en la mesa" {
		t.Fatalf("Quién canceló y quién autorizó en el evento: %s", stored)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.audit_events WHERE action='restaurant.supervisor_authorized'
        AND entity_id=$1 AND after_state->>'scope'='item_cancel' AND after_state->>'supervisor'='supervisor_9f'
        AND after_state->>'amount'='80.00'`, bill.Account.ID); count != 1 {
		t.Fatalf("La autorización no quedó auditada: %d", count)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.audit_events WHERE action='restaurant.item.cancelled'
        AND entity_id=$1 AND after_state->>'authorized_by'=$2`, bill.Account.ID, fixture.supervisorID); count != 1 {
		t.Fatalf("La auditoría de la cancelación debía decir quién autorizó: %d", count)
	}
	// Un reintento con la misma operación y sin contraseña recupera el resultado sin cancelar dos veces;
	// con otros datos se rechaza.
	retry := map[string]any{}
	for key, value := range authorized {
		retry[key] = value
	}
	retry["authorization"] = map[string]any{"username": "supervisor_9f"}
	again := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "POST", route, retry, 200))
	if again.Account.Revision != cancelled.Account.Revision || again.Account.ConfirmedQuantity != 1 {
		t.Fatalf("El reintento no recuperó el mismo resultado: %+v", again.Account)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.restaurant_item_units WHERE account_id=$1 AND state='cancelled'`,
		bill.Account.ID); count != 2 {
		t.Fatalf("El reintento canceló otra vez: %d unidades canceladas", count)
	}
	retry["quantity"] = 1
	if code := errorCode(t, fixture.call(t, fixture.waiter, "POST", route, retry, 409)); code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("La misma operación con otros datos: %s", code)
	}
	secret := stage9FPassword + "supervisor_9f"
	if count := rowCount(t, fixture.restaurantFixture, `SELECT (SELECT count(*) FROM app.audit_events WHERE strpos(coalesce(after_state::text,'')||
            coalesce(before_state::text,''),$1)>0)
        +(SELECT count(*) FROM app.restaurant_operations WHERE strpos(response::text,$1)>0 OR strpos(fingerprint,$1)>0)
        +(SELECT count(*) FROM app.restaurant_events WHERE strpos(payload::text,$1)>0)`, secret); count != 0 {
		t.Fatalf("La contraseña del supervisor quedó guardada en %d registros", count)
	}
}

func testStage9FEditReduction(t *testing.T, fixture *stage9FFixture) {
	bill := fixture.sentAccount(t, "Modificar 9F", 3)
	route := "restaurant/accounts/" + bill.Account.ID
	line := bill.Lines[0]
	edit := func(quantity int, notes string) map[string]any {
		request := line.Request
		request.Quantity = quantity
		request.Customization.Notes = notes
		return map[string]any{"revision": bill.Account.Revision, "version_id": line.VersionID, "line": request}
	}
	preview := func(terminal testTerminal, body map[string]any) operations.QuoteResult {
		t.Helper()
		return decodeResponse[operations.QuoteResult](t, fixture.call(t, terminal, "POST", route+"/edit-preview", body, 200))
	}
	// Go informa si la modificación pedirá supervisor: solo al reducir y sin orders.cancel.
	if !preview(fixture.waiter, edit(1, "")).CancelUnitsRequiresAuthorization {
		t.Fatal("Reducir sin «Cancelar partidas» debía pedir supervisor")
	}
	if preview(fixture.admin, edit(1, "")).CancelUnitsRequiresAuthorization {
		t.Fatal("Con «Cancelar partidas» no hace falta supervisor")
	}
	if preview(fixture.waiter, edit(3, "Sin sal")).CancelUnitsRequiresAuthorization {
		t.Fatal("Sin reducir no hace falta supervisor")
	}
	reduce := edit(2, "")
	reduce["expected_fingerprint"] = preview(fixture.waiter, edit(2, "")).Fingerprint
	reduce["reason"] = "Solo quería dos"
	if code := errorCode(t, fixture.send(t, fixture.waiter, "POST", route+"/edit", reduce, 403)); code != "AUTHORIZATION_REQUIRED" {
		t.Fatalf("Reducir sin autorización: %s", code)
	}
	reduce["authorization"] = stage9FAuthorization("mesero_9f")
	if code := errorCode(t, fixture.send(t, fixture.waiter, "POST", route+"/edit", reduce, 403)); code != "SELF_AUTHORIZATION" {
		t.Fatalf("Reducir con autoautorización: %s", code)
	}
	if current := fixture.refresh(t, bill.Account.ID); current.Account.Revision != bill.Account.Revision || current.Account.ConfirmedQuantity != 3 {
		t.Fatalf("Un rechazo modificó la cuenta: %+v", current.Account)
	}
	reduce["authorization"] = stage9FAuthorization("supervisor_9f")
	reduced := decodeResponse[operations.Bill](t, fixture.send(t, fixture.waiter, "POST", route+"/edit", reduce, 200))
	if reduced.Account.ConfirmedQuantity != 2 {
		t.Fatalf("La reducción autorizada debía dejar dos unidades: %+v", reduced.Account)
	}
	stored := fixture.scalar(t, `SELECT author_id::text||'/'||(payload->'after'->>'authorized_by')||'/'||(payload->'after'->>'authorized_by_name')||'/'||
            jsonb_array_length(payload->'after'->'cancelled_unit_ids')
        FROM app.restaurant_events WHERE account_id=$1 AND kind='item.modified'`, bill.Account.ID)
	if stored != fixture.waiterID+"/"+fixture.supervisorID+"/Prueba supervisor_9f/1" {
		t.Fatalf("Quién redujo y quién autorizó en el evento: %s", stored)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.audit_events WHERE action='restaurant.supervisor_authorized'
        AND entity_id=$1 AND after_state->>'scope'='item_modify' AND after_state->>'quantity'='1'`, bill.Account.ID); count != 1 {
		t.Fatalf("La autorización de la reducción no quedó auditada: %d", count)
	}
	// Modificar sin reducir no pide ni registra autorización (sobre la partida activa, ya con 2).
	for _, current := range reduced.Lines {
		if current.State == "active" {
			line = current
		}
	}
	bill = reduced
	notes := edit(2, "Sin sal")
	notes["expected_fingerprint"] = preview(fixture.waiter, edit(2, "Sin sal")).Fingerprint
	notes["reason"] = "Indicación del cliente"
	fixture.send(t, fixture.waiter, "POST", route+"/edit", notes, 200)
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.restaurant_events WHERE account_id=$1 AND kind='item.modified'
        AND NOT payload->'after' ? 'authorized_by'`, bill.Account.ID); count != 1 {
		t.Fatalf("Una modificación sin reducción no lleva «Autorizó»: %d", count)
	}
}

func testStage9FReports(t *testing.T, fixture *stage9FFixture) {
	admin := fixture.scalar(t, "SELECT display_name FROM app.users WHERE username='admin'")
	changes := fixture.report(t, "products.quantity_changes")
	found := false
	for _, column := range changes.Columns {
		found = found || (column.Key == "authorized_by" && column.Label == "Autorizó")
	}
	if !found {
		t.Fatalf("«Modificaciones de cantidades» debe tener la columna «Autorizó»: %+v", changes.Columns)
	}
	cases := map[string][]string{
		// motivo: modificó, autorizó, acción
		"Cliente cambió de opinión": {admin, "", "Cancelación"},
		"Se enfrió en la mesa":      {"Prueba mesero_9f", "Prueba supervisor_9f", "Cancelación"},
		"Solo quería dos":           {"Prueba mesero_9f", "Prueba supervisor_9f", "Modificación de la partida"},
	}
	for reason, values := range cases {
		row := stage9FRow(t, changes, "reason", reason)
		if row["changed_by"] != values[0] || row["authorized_by"] != values[1] || row["action"] != values[2] {
			t.Fatalf("%s: %+v", reason, row)
		}
	}
	cancelled := fixture.report(t, "products.cancelled")
	authorized := 0
	for _, row := range cancelled.Rows {
		if row["authorized_by"] == "Prueba supervisor_9f" {
			authorized++
			if row["cancelled_by"] != "Prueba mesero_9f" {
				t.Fatalf("Unidad cancelada con supervisor: %+v", row)
			}
		} else if row["authorized_by"] != "" {
			t.Fatalf("«Autorizó» inesperado en «Productos cancelados»: %+v", row)
		}
	}
	// Dos unidades canceladas por el mesero y una retirada al reducir, todas con el supervisor.
	if authorized != 3 {
		t.Fatalf("Unidades con «Autorizó» en «Productos cancelados»: %d (%+v)", authorized, cancelled.Rows)
	}
	activity := fixture.report(t, "employees.activity")
	supervisor := stage9FRow(t, activity, "user", "Prueba supervisor_9f")
	if fmt.Sprint(supervisor["authorizations"]) != "2" {
		t.Fatalf("Autorizaciones del supervisor (cancelación y reducción): %+v", supervisor)
	}
}
