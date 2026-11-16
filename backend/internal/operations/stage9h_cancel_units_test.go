//go:build integration

package operations_test

import (
	"fmt"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/operations"
)

// Etapa 9H, fase A (§2.2) contra PostgreSQL con la API real: «Cancelar unidades» y la reducción de la
// cantidad desde «Modificar» no piden autorización de supervisor (decisión del propietario). Basta el
// permiso propio «Cancelar partidas o cantidades confirmadas» (orders.cancel) y queda registrado quién
// lo hizo:
//   - sin el permiso, Go rechaza cancelar y reducir con 403 (también si el cliente envía una
//     autorización, que ya no existe: no se valida ninguna contraseña ni se cuenta un intento), y la
//     cuenta ya no informa cancel_units_requires_authorization;
//   - con el permiso, el evento y la auditoría guardan el autor y el motivo, sin «Autorizó»;
//   - «Modificaciones de cantidades» y «Productos cancelados» conservan la columna «Autorizó»: vacía en
//     lo nuevo y con el supervisor en un registro de la 9F (simulado en la base de la prueba), que
//     «Actividad por empleado» sigue contando;
//   - el Registro de movimientos dice quién canceló.
//
// Usa los ayudantes de stage9f_cancel_units_test.go y su propia base desde cero.

type stage9HCancelFixture struct {
	*stage9FFixture
	cashier   testTerminal
	cashierID string
}

func TestStage9HCancelUnitsWithoutAuthorization(t *testing.T) {
	resetStage9DDatabase(t)
	base := &stage9FFixture{restaurantFixture: newFixture(t)}
	base.seedCatalog(t)
	// Mesero sin orders.cancel (abre cuentas, captura y modifica), cajero con orders.cancel y un
	// supervisor que también lo tiene (solo aparece en el registro simulado de la 9F).
	base.waiter = inventoryUser(t, base.restaurantFixture, "mesero_9h", []string{
		"accounts.read", "accounts.open", "orders.capture", "orders.edit", "catalog.read", "catalog.prices.read",
	})
	base.supervisor = inventoryUser(t, base.restaurantFixture, "supervisor_9h", []string{"accounts.read", "orders.cancel"})
	base.waiterID = base.scalar(t, "SELECT id::text FROM app.users WHERE username='mesero_9h'")
	base.supervisorID = base.scalar(t, "SELECT id::text FROM app.users WHERE username='supervisor_9h'")
	fixture := &stage9HCancelFixture{stage9FFixture: base}
	fixture.cashier = inventoryUser(t, base.restaurantFixture, "cajero_9h", []string{
		"accounts.read", "orders.edit", "orders.cancel", "catalog.read", "catalog.prices.read",
	})
	fixture.cashierID = base.scalar(t, "SELECT id::text FROM app.users WHERE username='cajero_9h'")
	t.Run("sin el permiso propio Go rechaza cancelar y reducir, sin camino de supervisor", func(t *testing.T) {
		testStage9HWithoutPermission(t, fixture)
	})
	t.Run("con el permiso propio se cancela y se reduce sin autorización y queda quién lo hizo", func(t *testing.T) {
		testStage9HWithPermission(t, fixture)
	})
	t.Run("«Autorizó» vacía en lo nuevo y con el supervisor en los registros de la 9F", func(t *testing.T) {
		testStage9HReports(t, fixture)
	})
}

func testStage9HWithoutPermission(t *testing.T, fixture *stage9HCancelFixture) {
	bill := fixture.sentAccount(t, "Mesero 9H", 3)
	route := "restaurant/accounts/" + bill.Account.ID
	// La cuenta ya no dice si hace falta un supervisor: el campo de la 9F se retiró.
	raw := string(fixture.call(t, fixture.waiter, "GET", route, nil, 200))
	if strings.Contains(raw, "cancel_units_requires_authorization") {
		t.Fatalf("La cuenta no debe informar cancel_units_requires_authorization: %s", raw)
	}
	counts := `SELECT (SELECT count(*) FROM app.restaurant_operations)||'/'||(SELECT count(*) FROM app.restaurant_events)||'/'||
        (SELECT count(*) FROM app.restaurant_item_units WHERE state='cancelled')`
	before := fixture.scalar(t, counts)
	body := func(authorization map[string]any) map[string]any {
		fields := map[string]any{
			"revision": bill.Account.Revision, "version_id": bill.Lines[0].VersionID, "quantity": 1, "reason": "Se enfrió en la mesa",
		}
		if authorization != nil {
			fields["authorization"] = authorization
		}
		return fields
	}
	// Sin el permiso, 403 antes de leer el cuerpo: con o sin la autorización de un supervisor que sí
	// lo tiene, que ya no se valida (ni cuenta un intento en el límite del inicio de sesión).
	for _, authorization := range []map[string]any{nil, stage9FAuthorization("supervisor_9h")} {
		if code := errorCode(t, fixture.send(t, fixture.waiter, "POST", route+"/cancel", body(authorization), 403)); code != "PERMISSION_DENIED" {
			t.Fatalf("Cancelar sin el permiso propio: %s", code)
		}
	}
	if count := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.login_attempts WHERE username='supervisor_9h'"); count != 0 {
		t.Fatalf("No debía validarse la contraseña de ningún supervisor: %d intentos", count)
	}
	// Reducir desde «Modificar» también exige el permiso propio, ya en la previsualización.
	line := bill.Lines[0]
	edit := func(quantity int, notes string) map[string]any {
		request := line.Request
		request.Quantity = quantity
		request.Customization.Notes = notes
		return map[string]any{"revision": bill.Account.Revision, "version_id": line.VersionID, "line": request}
	}
	denied := decodeResponse[struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}](t, fixture.call(t, fixture.waiter, "POST", route+"/edit-preview", edit(1, ""), 403)).Error
	if denied.Code != "PERMISSION_DENIED" || !strings.Contains(denied.Message, "«Cancelar partidas o cantidades confirmadas»") {
		t.Fatalf("Reducir sin el permiso propio (previsualización): %+v", denied)
	}
	// Al confirmar (con la huella de una previsualización del administrador) Go lo vuelve a rechazar.
	reduce := edit(2, "")
	reduce["expected_fingerprint"] = decodeResponse[operations.QuoteResult](t, fixture.call(t, fixture.admin, "POST", route+"/edit-preview", edit(2, ""), 200)).Fingerprint
	reduce["reason"] = "Solo quería dos"
	if code := errorCode(t, fixture.send(t, fixture.waiter, "POST", route+"/edit", reduce, 403)); code != "PERMISSION_DENIED" {
		t.Fatalf("Reducir sin el permiso propio (confirmación): %s", code)
	}
	reduce["operation_id"] = randomID()
	reduce["authorization"] = stage9FAuthorization("supervisor_9h")
	if code := errorCode(t, fixture.call(t, fixture.waiter, "POST", route+"/edit", reduce, 400)); code != "OPERATION_VALIDATION" {
		t.Fatalf("La autorización de supervisor ya no es un campo de «Modificar»: %s", code)
	}
	if after := fixture.scalar(t, counts); after != before {
		t.Fatalf("Un rechazo escribió operaciones, eventos o cancelaciones: %s → %s", before, after)
	}
	// Modificar sin reducir sigue sin pedir orders.cancel.
	notes := edit(3, "Sin sal")
	notes["expected_fingerprint"] = decodeResponse[operations.QuoteResult](t, fixture.call(t, fixture.waiter, "POST", route+"/edit-preview", edit(3, "Sin sal"), 200)).Fingerprint
	notes["reason"] = "Indicación del cliente"
	modified := decodeResponse[operations.Bill](t, fixture.send(t, fixture.waiter, "POST", route+"/edit", notes, 200))
	if modified.Account.ConfirmedQuantity != 3 {
		t.Fatalf("Modificar sin reducir: %+v", modified.Account)
	}
}

func testStage9HWithPermission(t *testing.T, fixture *stage9HCancelFixture) {
	bill := fixture.sentAccount(t, "Cajero 9H", 3)
	route := "restaurant/accounts/" + bill.Account.ID
	cancelled := decodeResponse[operations.Bill](t, fixture.send(t, fixture.cashier, "POST", route+"/cancel", map[string]any{
		"revision": bill.Account.Revision, "version_id": bill.Lines[0].VersionID, "quantity": 1, "reason": "Cliente cambió de opinión 9H",
	}, 200))
	if cancelled.Account.ConfirmedQuantity != 2 || cancelled.Account.Totals.Total != "80.00" {
		t.Fatalf("Cancelación con el permiso propio: %+v", cancelled.Account)
	}
	stored := fixture.scalar(t, `SELECT author_id::text||'/'||(payload->'after'->>'reason')||'/'||(payload->'after' ? 'authorized_by')::text
        FROM app.restaurant_events WHERE account_id=$1 AND kind='item.cancelled'`, bill.Account.ID)
	if stored != fixture.cashierID+"/Cliente cambió de opinión 9H/false" {
		t.Fatalf("El evento debe guardar quién canceló y el motivo, sin «Autorizó»: %s", stored)
	}
	// La autorización enviada por un cliente anterior ya no es un campo de la operación.
	if code := errorCode(t, fixture.send(t, fixture.cashier, "POST", route+"/cancel", map[string]any{
		"revision": cancelled.Account.Revision, "version_id": cancelled.Lines[0].VersionID, "quantity": 1, "reason": "Con autorización",
		"authorization": stage9FAuthorization("supervisor_9h"),
	}, 400)); code != "OPERATION_VALIDATION" {
		t.Fatalf("Cancelar con una autorización de supervisor: %s", code)
	}
	// Reducir desde «Modificar» con el permiso propio, sin autorización.
	var line operations.ConfirmedLine
	for _, current := range cancelled.Lines {
		if current.State == "active" {
			line = current
		}
	}
	request := line.Request
	request.Quantity = 1
	body := map[string]any{"revision": cancelled.Account.Revision, "version_id": line.VersionID, "line": request}
	body["expected_fingerprint"] = decodeResponse[operations.QuoteResult](t, fixture.call(t, fixture.cashier, "POST", route+"/edit-preview", body, 200)).Fingerprint
	body["reason"] = "Solo quería una 9H"
	reduced := decodeResponse[operations.Bill](t, fixture.send(t, fixture.cashier, "POST", route+"/edit", body, 200))
	if reduced.Account.ConfirmedQuantity != 1 {
		t.Fatalf("La reducción con el permiso propio debía dejar una unidad: %+v", reduced.Account)
	}
	stored = fixture.scalar(t, `SELECT author_id::text||'/'||jsonb_array_length(payload->'after'->'cancelled_unit_ids')||'/'||
            (payload->'after' ? 'authorized_by')::text
        FROM app.restaurant_events WHERE account_id=$1 AND kind='item.modified'`, bill.Account.ID)
	if stored != fixture.cashierID+"/1/false" {
		t.Fatalf("El evento de la reducción debe guardar quién la hizo, sin «Autorizó»: %s", stored)
	}
	// Ninguna autorización de supervisor auditada y el Registro de movimientos dice quién canceló.
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.audit_events
        WHERE action='restaurant.supervisor_authorized' AND entity_id=$1`, bill.Account.ID); count != 0 {
		t.Fatalf("No debía auditarse una autorización de supervisor: %d", count)
	}
	audit := decodeResponse[struct {
		Items []struct {
			ActorUsername string         `json:"actor_username"`
			EntityID      string         `json:"entity_id"`
			AfterState    map[string]any `json:"after_state"`
		} `json:"items"`
	}](t, fixture.adminCall(t, "GET", "audit?action=restaurant.item.cancelled&page_size=100", nil, 200))
	found := false
	for _, item := range audit.Items {
		if item.EntityID == bill.Account.ID {
			found = true
			if item.ActorUsername != "cajero_9h" || item.AfterState["reason"] != "Cliente cambió de opinión 9H" {
				t.Fatalf("Registro de movimientos de la cancelación: %+v", item)
			}
			if _, present := item.AfterState["authorized_by"]; present {
				t.Fatalf("La auditoría nueva no lleva «Autorizó»: %+v", item.AfterState)
			}
		}
	}
	if !found {
		t.Fatal("La cancelación no aparece en el Registro de movimientos")
	}
}

// testStage9HReports comprueba los reportes con lo nuevo (sin «Autorizó») y con un registro de la 9F:
// una cancelación cuyo evento guardó quién autorizó, simulada en la base de la prueba a partir de una
// cancelación real (los eventos de la 9F tienen exactamente esa forma).
func testStage9HReports(t *testing.T, fixture *stage9HCancelFixture) {
	legacy := fixture.sentAccount(t, "Registro 9F", 2)
	route := "restaurant/accounts/" + legacy.Account.ID
	fixture.send(t, fixture.admin, "POST", route+"/cancel", map[string]any{
		"revision": legacy.Account.Revision, "version_id": legacy.Lines[0].VersionID, "quantity": 1, "reason": "Registro anterior de la 9F",
	}, 200)
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.restaurant_events SET author_id=$2,
            payload=jsonb_set(payload,'{after}',payload->'after'||jsonb_build_object('authorized_by',$3::text,
                'authorized_by_username','supervisor_9h','authorized_by_name','Prueba supervisor_9h'))
        WHERE account_id=$1 AND kind='item.cancelled'`, legacy.Account.ID, fixture.waiterID, fixture.supervisorID); err != nil {
		t.Fatal(err)
	}
	changes := fixture.report(t, "products.quantity_changes")
	found := false
	for _, column := range changes.Columns {
		found = found || (column.Key == "authorized_by" && column.Label == "Autorizó")
	}
	if !found {
		t.Fatalf("«Modificaciones de cantidades» conserva la columna «Autorizó»: %+v", changes.Columns)
	}
	cases := map[string][]string{
		// motivo: modificó, autorizó, acción
		"Cliente cambió de opinión 9H": {"Prueba cajero_9h", "", "Cancelación"},
		"Solo quería una 9H":           {"Prueba cajero_9h", "", "Modificación de la partida"},
		"Registro anterior de la 9F":   {"Prueba mesero_9h", "Prueba supervisor_9h", "Cancelación"},
	}
	for reason, values := range cases {
		row := stage9FRow(t, changes, "reason", reason)
		if row["changed_by"] != values[0] || row["authorized_by"] != values[1] || row["action"] != values[2] {
			t.Fatalf("%s: %+v", reason, row)
		}
	}
	cancelled := fixture.report(t, "products.cancelled")
	byCashier, authorized := 0, 0
	for _, row := range cancelled.Rows {
		switch {
		case row["cancelled_by"] == "Prueba cajero_9h":
			byCashier++
			if row["authorized_by"] != "" {
				t.Fatalf("Lo nuevo no tiene «Autorizó»: %+v", row)
			}
		case row["authorized_by"] == "Prueba supervisor_9h":
			authorized++
			if row["cancelled_by"] != "Prueba mesero_9h" {
				t.Fatalf("Registro de la 9F en «Productos cancelados»: %+v", row)
			}
		}
	}
	// El cajero canceló una unidad y retiró otra al reducir; el registro de la 9F, una.
	if byCashier != 2 || authorized != 1 {
		t.Fatalf("«Productos cancelados»: %d del cajero y %d autorizadas (%+v)", byCashier, authorized, cancelled.Rows)
	}
	activity := fixture.report(t, "employees.activity")
	supervisor := stage9FRow(t, activity, "user", "Prueba supervisor_9h")
	if fmt.Sprint(supervisor["authorizations"]) != "1" {
		t.Fatalf("«Actividad por empleado» sigue contando la autorización de la 9F: %+v", supervisor)
	}
}
