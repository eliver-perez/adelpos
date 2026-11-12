//go:build integration

package operations_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/features"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa 9D (§4.1, §4.2, §3.4 y §5) contra PostgreSQL con la API real: «Cancelar cuenta» con permiso
// propio y con autorización de supervisor (sin permiso ni autorización, autoautorización, contraseña
// incorrecta y supervisor sin permiso), con productos liberados a cocina y sin liberar, su aviso de
// cancelación a la estación, el inventario con la preclasificación de merma del motivo, la mesa libre,
// la idempotencia sin contraseña en la huella, los rechazos por pagos parciales y por descuentos, el
// motivo «Otro» sin descripción, el catálogo de motivos, «Descartar» cuentas vacías y con borrador, la
// creación de cuentas con su primer producto sin mesa y para llevar, y el umbral de cuenta abierta.
//
// Usa su propia base desde cero (corre después de la aceptación encadenada, en otro archivo).

const stage9DPassword = "Clave temporal de "

type stage9DFixture struct {
	*restaurantFixture
	waiter       testTerminal
	supervisor   testTerminal
	weak         testTerminal
	adminID      string
	waiterID     string
	supervisorID string
	station      kitchen.Station
	dessert      catalog.Product
	cash         string
	reasons      map[string]operations.CancelReason
}

func resetStage9DDatabase(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("No se pudo abrir el pool temporal")
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatal(err)
	}
}

func (fixture *stage9DFixture) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var value string
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// operate envía una operación con un operation_id nuevo desde la terminal indicada.
func (fixture *stage9DFixture) operate(t *testing.T, terminal testTerminal, method, route string, body map[string]any, status int) []byte {
	t.Helper()
	if _, found := body["operation_id"]; !found {
		body["operation_id"] = randomID()
	}
	return fixture.call(t, terminal, method, route, body, status)
}

// cancelBody arma el cuerpo de «Cancelar cuenta» con la revisión vigente de la cuenta.
func (fixture *stage9DFixture) cancelBody(t *testing.T, accountID, reason, description string, authorization map[string]any) map[string]any {
	t.Helper()
	body := map[string]any{
		"revision":    fixture.refresh(t, accountID).Account.Revision,
		"reason_id":   fixture.reasons[reason].ID,
		"description": description,
	}
	if authorization != nil {
		body["authorization"] = authorization
	}
	return body
}

func stage9DAuthorization(username string) map[string]any {
	return map[string]any{"username": username, "password": stage9DPassword + username}
}

// order abre una cuenta de mesa y confirma sus partidas (con la cocina configurada, se liberan).
func (fixture *stage9DFixture) order(t *testing.T, name string, table *operations.Table, lines ...catalog.LineRequest) operations.Bill {
	t.Helper()
	body := map[string]any{"name": name, "mode": "dine_in"}
	if table != nil {
		body["table_id"] = table.ID
	}
	bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", body, 200))
	return fixture.capture(t, bill, lines)
}

func (fixture *stage9DFixture) processInventory(t *testing.T) {
	t.Helper()
	if _, err := inventory.ProcessPending(fixture.ctx, fixture.pool, 1000); err != nil {
		t.Fatal(err)
	}
}

func TestStage9DAccountCancellation(t *testing.T) {
	resetStage9DDatabase(t)
	fixture := &stage9DFixture{restaurantFixture: newFixture(t), reasons: map[string]operations.CancelReason{}}
	fixture.seedCatalog(t)
	setupStage9D(t, fixture)
	t.Run("catálogo de motivos de cancelación", func(t *testing.T) {
		testStage9DReasons(t, fixture)
	})
	t.Run("cancelar con permiso propio: cocina, inventario con merma, mesa libre e idempotencia", func(t *testing.T) {
		testStage9DCancelWithPermission(t, fixture)
	})
	t.Run("cancelar con autorización de supervisor y sus rechazos", func(t *testing.T) {
		testStage9DCancelWithSupervisor(t, fixture)
	})
	t.Run("pagos parciales, descuentos y motivos inválidos", func(t *testing.T) {
		testStage9DCancelRejections(t, fixture)
	})
	t.Run("descartar cuentas vacías y con borrador", func(t *testing.T) {
		testStage9DDiscard(t, fixture)
	})
	t.Run("creación con el primer producto sin mesa y para llevar", func(t *testing.T) {
		testStage9DDeferredCreation(t, fixture)
	})
	t.Run("umbral de cuenta abierta por mucho tiempo", func(t *testing.T) {
		testStage9DLongOpen(t, fixture)
	})
}

// setupStage9D configura mesas, una estación de pantalla con la ruta de la categoría del catálogo,
// un postre sin ruta (su liberación queda bloqueada), el control de inventario con una receta para
// las papas, los usuarios y el turno de caja.
func setupStage9D(t *testing.T, fixture *stage9DFixture) {
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón", Active: true}, 200))
	fixture.table = decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
		AreaID: fixture.area.ID, Name: "Mesa 1", Shape: "rectangle", X: 20, Y: 30, Width: 100, Height: 72, Seats: 4, Active: true,
	}, 200))
	fixture.secondTable = decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
		AreaID: fixture.area.ID, Name: "Mesa 2", Shape: "round", X: 140, Y: 30, Width: 90, Height: 90, Seats: 4, Active: true,
	}, 200))
	fixture.station = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina 9D", Active: true, Reception: "kds", TargetMinutes: 12, WarningMinutes: 15, OverdueMinutes: 25,
		Grouping: "inherit", Tracking: "inherit",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare",
		StationIDs: []string{fixture.station.ID}}, 200)
	desserts := decodeResponse[catalog.Category](t, fixture.adminCall(t, "POST", "catalog/categories",
		catalog.Category{Metadata: catalog.Metadata{Name: "Postres sin ruta", Active: true}}, 200))
	fixture.dessert = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Flan 9D", Active: true}, CategoryID: desserts.ID, Code: "FLAN-9D", Kind: "product", BasePrice: "35",
	}, 200))
	// Control de inventario con una receta para las papas: su liberación consume una pieza.
	var control features.Feature
	for _, feature := range decodeResponse[[]features.Feature](t, fixture.adminCall(t, "GET", "settings/features", nil, 200)) {
		if feature.Code == features.InventoryControl {
			control = feature
		}
	}
	fixture.adminCall(t, "PUT", "settings/features", map[string]any{"code": features.InventoryControl, "enabled": true,
		"revision": control.Revision}, 200)
	units := map[string]inventory.Unit{}
	for _, unit := range decodeResponse[[]inventory.Unit](t, fixture.adminCall(t, "GET", "inventory/units", nil, 200)) {
		units[unit.Code] = unit
	}
	potato := decodeResponse[inventory.Item](t, fixture.mutate(t, "POST", "inventory/items", map[string]any{
		"code": "PAPA-9D", "name": "Papa 9D", "base_unit_id": units["pza"].ID, "kind": "purchased", "active": true, "reference_cost": "2",
	}, 200))
	fixture.mutate(t, "POST", "inventory/recipes", map[string]any{"owner_kind": "product", "product_id": fixture.fries.ID, "active": true,
		"lines": []map[string]any{recipeItemLine(potato, "1", units["pza"], false)}}, 200)
	fixture.waiter = inventoryUser(t, fixture.restaurantFixture, "mesero_9d", []string{
		"accounts.read", "accounts.open", "takeaway.read", "takeaway.manage", "orders.capture", "catalog.read", "catalog.prices.read",
	})
	fixture.supervisor = inventoryUser(t, fixture.restaurantFixture, "supervisor_9d", []string{"accounts.read", "accounts.cancel"})
	fixture.weak = inventoryUser(t, fixture.restaurantFixture, "encargado_9d", []string{"accounts.read", "accounts.manage"})
	fixture.adminID = fixture.scalar(t, "SELECT id::text FROM app.users WHERE username='admin'")
	fixture.waiterID = fixture.scalar(t, "SELECT id::text FROM app.users WHERE username='mesero_9d'")
	fixture.supervisorID = fixture.scalar(t, "SELECT id::text FROM app.users WHERE username='supervisor_9d'")
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil && *method.Code == "cash" {
			fixture.cash = method.ID
		}
	}
	fixture.mutate(t, "POST", "payments/shifts", map[string]any{"opening_float": "0"}, 200)
	for _, reason := range decodeResponse[[]operations.CancelReason](t, fixture.adminCall(t, "GET", "restaurant/cancel-reasons", nil, 200)) {
		fixture.reasons[reason.Name] = reason
	}
}

func testStage9DReasons(t *testing.T, fixture *stage9DFixture) {
	listed := decodeResponse[[]operations.CancelReason](t, fixture.call(t, fixture.waiter, "GET", "restaurant/cancel-reasons", nil, 200))
	names := []string{}
	for _, reason := range listed {
		names = append(names, reason.Name)
	}
	if strings.Join(names, "|") != "Cliente se retiró sin pagar|Cliente canceló el pedido|Error de captura|Otro" {
		t.Fatalf("Motivos iniciales: %v", names)
	}
	if !fixture.reasons["Otro"].RequiresDescription || !fixture.reasons["Cliente se retiró sin pagar"].PreparedIsWaste ||
		fixture.reasons["Cliente canceló el pedido"].PreparedIsWaste || fixture.reasons["Error de captura"].RequiresDescription {
		t.Fatalf("Marcas de los motivos iniciales: %+v", fixture.reasons)
	}
	// Administrarlos exige el permiso de los motivos de descuento.
	fixture.operate(t, fixture.waiter, "POST", "restaurant/cancel-reasons", map[string]any{"name": "Prueba", "active": true}, 403)
	created := decodeResponse[operations.CancelReason](t, fixture.mutate(t, "POST", "restaurant/cancel-reasons", map[string]any{
		"name": "Pedido duplicado", "requires_description": false, "prepared_is_waste": false, "active": true,
		"position": map[string]any{"after": fixture.reasons["Error de captura"].ID},
	}, 200))
	if created.SortOrder <= fixture.reasons["Error de captura"].SortOrder {
		t.Fatalf("La posición elegida no se aplicó: %+v", created)
	}
	fixture.mutate(t, "POST", "restaurant/cancel-reasons", map[string]any{"name": "Pedido duplicado", "active": true}, 409)
	fixture.mutate(t, "POST", "restaurant/cancel-reasons", map[string]any{"name": "Con\nsalto", "active": true}, 400)
	fixture.mutate(t, "POST", "restaurant/cancel-reasons", map[string]any{"name": "  ", "active": true}, 400)
	renamed := decodeResponse[operations.CancelReason](t, fixture.mutate(t, "PATCH", "restaurant/cancel-reasons/"+created.ID, map[string]any{
		"revision": created.Revision, "name": "Pedido repetido", "requires_description": true, "prepared_is_waste": true, "active": false,
	}, 200))
	if renamed.Name != "Pedido repetido" || !renamed.RequiresDescription || !renamed.PreparedIsWaste || renamed.Active ||
		renamed.Revision != created.Revision+1 {
		t.Fatalf("Cambio del motivo: %+v", renamed)
	}
	fixture.mutate(t, "PATCH", "restaurant/cancel-reasons/"+created.ID, map[string]any{
		"revision": created.Revision, "name": "Obsoleto", "active": true,
	}, 409)
	// Debe quedar al menos un motivo activo. Colocar un motivo renumera a sus hermanos (con revisión
	// nueva), así que se leen otra vez.
	seeded := []string{"Cliente se retiró sin pagar", "Cliente canceló el pedido", "Error de captura", "Otro"}
	for index, name := range seeded {
		var reason operations.CancelReason
		for _, entry := range decodeResponse[[]operations.CancelReason](t, fixture.adminCall(t, "GET", "restaurant/cancel-reasons", nil, 200)) {
			if entry.Name == name {
				reason = entry
			}
		}
		status := 200
		if index == len(seeded)-1 {
			status = 409
		}
		body := fixture.mutate(t, "PATCH", "restaurant/cancel-reasons/"+reason.ID, map[string]any{
			"revision": reason.Revision, "name": reason.Name, "requires_description": reason.RequiresDescription,
			"prepared_is_waste": reason.PreparedIsWaste, "active": false,
		}, status)
		if status == 409 && errorCode(t, body) != "LAST_ACTIVE_REASON" {
			t.Fatalf("Se esperaba LAST_ACTIVE_REASON: %s", body)
		}
	}
	for _, name := range seeded[:len(seeded)-1] {
		current := decodeResponse[[]operations.CancelReason](t, fixture.adminCall(t, "GET", "restaurant/cancel-reasons", nil, 200))
		for _, reason := range current {
			if reason.Name == name {
				fixture.mutate(t, "PATCH", "restaurant/cancel-reasons/"+reason.ID, map[string]any{
					"revision": reason.Revision, "name": reason.Name, "requires_description": reason.RequiresDescription,
					"prepared_is_waste": reason.PreparedIsWaste, "active": true,
				}, 200)
			}
		}
	}
	for _, reason := range decodeResponse[[]operations.CancelReason](t, fixture.adminCall(t, "GET", "restaurant/cancel-reasons", nil, 200)) {
		fixture.reasons[reason.Name] = reason
	}
	if count := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.account_cancel_reasons WHERE active"); count != 4 {
		t.Fatalf("Motivos activos al terminar: %d", count)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.audit_events
        WHERE action IN ('restaurant.cancel_reason_created','restaurant.cancel_reason_updated')`); count < 8 {
		t.Fatalf("Los cambios del catálogo no se auditaron: %d", count)
	}
}

func testStage9DCancelWithPermission(t *testing.T, fixture *stage9DFixture) {
	bill := fixture.order(t, "Se fue sin pagar", &fixture.table, catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 2})
	bill = fixture.capture(t, bill, []catalog.LineRequest{{ProductID: fixture.dessert.ID, Quantity: 1}})
	route := "restaurant/accounts/" + bill.Account.ID
	// Un borrador sin enviar que la cancelación descarta.
	bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", route+"/draft", map[string]any{
		"revision": bill.Account.Revision, "items": []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{
			ProductID: fixture.fries.ID, Quantity: 1}}},
	}, 200))
	fixture.processInventory(t)
	released := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.kitchen_unit_releases k
        JOIN app.restaurant_item_units u ON u.id=k.unit_id WHERE u.account_id=$1`, bill.Account.ID)
	blocked := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.kitchen_releases WHERE account_id=$1 AND state='blocked'", bill.Account.ID)
	consumed := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.inventory_unit_consumptions c
        JOIN app.restaurant_item_units u ON u.id=c.restaurant_unit_id WHERE u.account_id=$1`, bill.Account.ID)
	if released != 2 || blocked != 1 || consumed != 2 || bill.Account.DraftQuantity != 1 {
		t.Fatalf("Preparación: %d liberadas, %d bloqueadas, %d consumidas, borrador %d", released, blocked, consumed, bill.Account.DraftQuantity)
	}
	if bill.Account.CancelBlocker != "" || bill.Account.CancelRequiresAuthorization || bill.Account.Discardable {
		t.Fatalf("Marcas de la cuenta antes de cancelar: %+v", bill.Account)
	}
	tables := decodeResponse[[]operations.Table](t, fixture.adminCall(t, "GET", "restaurant/tables", nil, 200))
	for _, table := range tables {
		if table.ID == fixture.table.ID && table.State != "occupied" {
			t.Fatalf("La mesa debía estar ocupada: %s", table.State)
		}
	}
	body := fixture.cancelBody(t, bill.Account.ID, "Cliente se retiró sin pagar", "Mesa de la ventana", nil)
	body["operation_id"] = randomID()
	first := fixture.adminCall(t, "POST", route+"/cancel-account", body, 200)
	cancelled := decodeResponse[operations.Bill](t, first)
	if cancelled.Account.State != "voided" || cancelled.Account.ConfirmedQuantity != 0 || len(cancelled.Draft) != 0 ||
		cancelled.Account.Releasable || cancelled.Account.Discardable {
		t.Fatalf("Cuenta cancelada: %+v", cancelled.Account)
	}
	record := cancelled.Cancellation
	if record == nil || record.ReasonName != "Cliente se retiró sin pagar" || record.Description == nil || *record.Description != "Mesa de la ventana" ||
		!record.PreparedIsWaste || record.CancelledUnits != 3 || record.CancelledAmount != "115.00" || record.AuthorizedByName != nil ||
		record.CancelledByName != "Administrador" {
		t.Fatalf("Registro de la cancelación: %+v", record)
	}
	// Reintento idempotente: misma respuesta; con otro contenido, conflicto.
	repeated := fixture.adminCall(t, "POST", route+"/cancel-account", body, 200)
	var left, right any
	_ = json.Unmarshal(first, &left)
	_ = json.Unmarshal(repeated, &right)
	if string(mustJSON(left)) != string(mustJSON(right)) {
		t.Fatal("El reintento no devolvió el mismo resultado")
	}
	body["description"] = "Otra descripción"
	if errorCode(t, fixture.adminCall(t, "POST", route+"/cancel-account", body, 409)) != "IDEMPOTENCY_CONFLICT" {
		t.Fatal("Un reintento con otro contenido debía rechazarse")
	}
	// Una cuenta cancelada no se vuelve a cancelar.
	fixture.mutate(t, "POST", route+"/cancel-account", map[string]any{"revision": cancelled.Account.Revision,
		"reason_id": fixture.reasons["Error de captura"].ID}, 400)
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.restaurant_item_units
        WHERE account_id=$1 AND state='cancelled' AND cancelled_at IS NOT NULL`, bill.Account.ID); count != 3 {
		t.Fatalf("Unidades canceladas: %d", count)
	}
	if count := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.restaurant_draft_items WHERE account_id=$1", bill.Account.ID); count != 0 {
		t.Fatalf("El borrador debía descartarse: %d", count)
	}
	eventID := fixture.scalar(t, "SELECT id::text FROM app.restaurant_events WHERE account_id=$1 AND kind='account.cancelled'", bill.Account.ID)
	// Cocina: las tareas liberadas se cancelan con su aviso de cancelación a la estación; la liberación
	// bloqueada (sin ruta) se cancela sin papel.
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.kitchen_tasks k JOIN app.restaurant_item_units u ON u.id=k.unit_id
        WHERE u.account_id=$1 AND k.state<>'cancelled'`, bill.Account.ID); count != 0 {
		t.Fatalf("Quedaron %d tareas de cocina activas", count)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.kitchen_tickets
        WHERE source_event_id=$1 AND kind='cancellation' AND $2::uuid=ANY(station_ids)`, eventID, fixture.station.ID); count != 1 {
		t.Fatalf("Avisos de cancelación a la estación: %d", count)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.kitchen_releases
        WHERE account_id=$1 AND state IN ('pending','scheduled','blocked')`, bill.Account.ID); count != 0 {
		t.Fatalf("Quedaron %d liberaciones sin cancelar", count)
	}
	// Mesa libre, auditoría sin contraseñas y evento en tiempo real.
	tables = decodeResponse[[]operations.Table](t, fixture.adminCall(t, "GET", "restaurant/tables", nil, 200))
	for _, table := range tables {
		if table.ID == fixture.table.ID && (table.State != "free" || table.AccountCount != 0) {
			t.Fatalf("La mesa debía quedar libre: %+v", table)
		}
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.audit_events
        WHERE action='restaurant.account.cancelled' AND entity_id=$1 AND success`, bill.Account.ID); count != 1 {
		t.Fatalf("Auditoría de la cancelación: %d", count)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.realtime_events
        WHERE kind='restaurant.account.cancelled' AND aggregate_id=$1`, bill.Account.ID); count != 1 {
		t.Fatalf("Evento en tiempo real: %d", count)
	}
	// Nunca es venta.
	if count := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.sales_recognitions WHERE account_id=$1", bill.Account.ID); count != 0 {
		t.Fatalf("La cuenta cancelada se reconoció como venta: %d", count)
	}
	// Inventario: dos solicitudes «cancel» del evento y, con el motivo que marca merma, «Preparado / merma».
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.inventory_consumption_requests
        WHERE kind='cancel' AND source_id=$1`, eventID); count != 2 {
		t.Fatalf("Solicitudes de cancelación al inventario: %d", count)
	}
	fixture.processInventory(t)
	rows, err := fixture.pool.Query(fixture.ctx, `SELECT c.id::text,c.state,coalesce(c.resolution,''),coalesce(c.classified_by::text,''),
            coalesce(c.notes,''),c.revision
        FROM app.inventory_cancellation_classifications c JOIN app.restaurant_item_units u ON u.id=c.restaurant_unit_id
        WHERE u.account_id=$1 ORDER BY c.id`, bill.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	type classification struct {
		id, state, resolution, by, notes string
		revision                         int
	}
	found := []classification{}
	for rows.Next() {
		var entry classification
		if err := rows.Scan(&entry.id, &entry.state, &entry.resolution, &entry.by, &entry.notes, &entry.revision); err != nil {
			t.Fatal(err)
		}
		found = append(found, entry)
	}
	rows.Close()
	if len(found) != 2 {
		t.Fatalf("Clasificaciones: %+v", found)
	}
	for _, entry := range found {
		if entry.state != "prepared_waste" || entry.resolution != "manual" || entry.by != fixture.adminID ||
			!strings.Contains(entry.notes, "Cliente se retiró sin pagar") {
			t.Fatalf("Preclasificación de merma: %+v", entry)
		}
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.audit_events
        WHERE action='inventory.cancellation_preclassified'`); count != 2 {
		t.Fatalf("Auditoría de la preclasificación: %d", count)
	}
	// El consumo se conserva (merma) y el encargado puede corregirlo antes del corte.
	if net := fixture.scalar(t, `SELECT coalesce(-sum(m.quantity),0)::text FROM app.inventory_movements m
        JOIN app.restaurant_item_units u ON u.id=m.restaurant_unit_id WHERE u.account_id=$1`, bill.Account.ID); net != "2.000000" {
		t.Fatalf("Consumo neto de la cuenta cancelada: %s", net)
	}
	corrected := decodeResponse[inventory.Cancellation](t, fixture.mutate(t, "POST", "inventory/cancellations/"+found[0].id+"/classify",
		map[string]any{"revision": found[0].revision, "state": "not_prepared"}, 200))
	if corrected.State != "not_prepared" || corrected.Resolution == nil || *corrected.Resolution != "manual" {
		t.Fatalf("Corrección del encargado: %+v", corrected)
	}
}

func testStage9DCancelWithSupervisor(t *testing.T, fixture *stage9DFixture) {
	bill := fixture.order(t, "Cambió de opinión", &fixture.secondTable, catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1})
	fixture.processInventory(t)
	route := "restaurant/accounts/" + bill.Account.ID + "/cancel-account"
	asWaiter := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "GET", "restaurant/accounts/"+bill.Account.ID, nil, 200))
	if !asWaiter.Account.CancelRequiresAuthorization || asWaiter.Account.CancelBlocker != "" {
		t.Fatalf("El mesero debía necesitar autorización: %+v", asWaiter.Account)
	}
	revision := asWaiter.Account.Revision
	denied := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action='access.denied'")
	cases := []struct {
		label         string
		authorization map[string]any
		status        int
		code          string
	}{
		{"sin permiso ni autorización", nil, 403, "AUTHORIZATION_REQUIRED"},
		{"autoautorización", stage9DAuthorization("mesero_9d"), 403, "SELF_AUTHORIZATION"},
		{"contraseña incorrecta", map[string]any{"username": "supervisor_9d", "password": "incorrecta"}, 401, "AUTHORIZATION_INVALID"},
		{"supervisor sin permiso", stage9DAuthorization("encargado_9d"), 403, "AUTHORIZATION_INSUFFICIENT"},
	}
	for _, entry := range cases {
		body := fixture.cancelBody(t, bill.Account.ID, "Cliente canceló el pedido", "", entry.authorization)
		if code := errorCode(t, fixture.operate(t, fixture.waiter, "POST", route, body, entry.status)); code != entry.code {
			t.Fatalf("%s: %s", entry.label, code)
		}
	}
	unchanged := fixture.refresh(t, bill.Account.ID)
	if unchanged.Account.State != "open" || unchanged.Account.Revision != revision || unchanged.Account.ConfirmedQuantity != 1 {
		t.Fatalf("Un rechazo modificó la cuenta: %+v", unchanged.Account)
	}
	if count := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action='access.denied'"); count != denied+3 {
		t.Fatalf("Denegaciones auditadas: %d → %d", denied, count)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT failures FROM app.login_attempts WHERE device_id=$1 AND username='supervisor_9d'`,
		fixture.waiter.deviceID); count != 1 {
		t.Fatalf("El intento fallido debía contarse en el límite de intentos: %d", count)
	}
	authorization := stage9DAuthorization("supervisor_9d")
	body := fixture.cancelBody(t, bill.Account.ID, "Cliente canceló el pedido", "", authorization)
	body["operation_id"] = randomID()
	cancelled := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "POST", route, body, 200))
	record := cancelled.Cancellation
	if cancelled.Account.State != "voided" || record == nil || record.AuthorizedByName == nil || *record.AuthorizedByName != "Prueba supervisor_9d" ||
		record.CancelledByName != "Prueba mesero_9d" || record.Description != nil || record.PreparedIsWaste || record.CancelledAmount != "40.00" {
		t.Fatalf("Cancelación autorizada: %+v %+v", cancelled.Account, record)
	}
	stored := fixture.scalar(t, "SELECT cancelled_by::text||'/'||authorized_by::text FROM app.account_cancellations WHERE account_id=$1", bill.Account.ID)
	if stored != fixture.waiterID+"/"+fixture.supervisorID {
		t.Fatalf("Quién canceló y quién autorizó: %s", stored)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.audit_events WHERE action='restaurant.supervisor_authorized'
        AND entity_id=$1 AND after_state->>'supervisor'='supervisor_9d'`, bill.Account.ID); count != 1 {
		t.Fatalf("La autorización no quedó auditada: %d", count)
	}
	// La contraseña no queda en la huella, la respuesta, los eventos ni la auditoría: un reintento sin
	// ella recupera el mismo resultado.
	retry := map[string]any{}
	for key, value := range body {
		retry[key] = value
	}
	retry["authorization"] = map[string]any{"username": "supervisor_9d"}
	again := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "POST", route, retry, 200))
	if again.Account.Revision != cancelled.Account.Revision || again.Cancellation == nil {
		t.Fatal("El reintento sin contraseña no recuperó el resultado")
	}
	secret := stage9DPassword + "supervisor_9d"
	if count := rowCount(t, fixture.restaurantFixture, `SELECT (SELECT count(*) FROM app.audit_events WHERE strpos(coalesce(after_state::text,'')||
            coalesce(before_state::text,''),$1)>0)
        +(SELECT count(*) FROM app.restaurant_operations WHERE strpos(response::text,$1)>0)
        +(SELECT count(*) FROM app.restaurant_events WHERE strpos(payload::text,$1)>0)`, secret); count != 0 {
		t.Fatalf("La contraseña del supervisor quedó guardada en %d registros", count)
	}
	// Sin la marca de merma, la cancelación queda «por clasificar».
	fixture.processInventory(t)
	if state := fixture.scalar(t, `SELECT string_agg(c.state,',') FROM app.inventory_cancellation_classifications c
        JOIN app.restaurant_item_units u ON u.id=c.restaurant_unit_id WHERE u.account_id=$1`, bill.Account.ID); state != "pending" {
		t.Fatalf("La cancelación sin merma debía quedar por clasificar: %s", state)
	}
}

func testStage9DCancelRejections(t *testing.T, fixture *stage9DFixture) {
	bill := fixture.order(t, "Pago parcial", nil, catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 2})
	finance := decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+bill.Account.ID, nil, 200))
	paid := decodeResponse[struct {
		Payment payments.PaymentRecord `json:"payment"`
	}](t, fixture.mutate(t, "POST", "payments/accounts/"+bill.Account.ID+"/payments", map[string]any{
		"method_id": fixture.cash, "mode": "partial", "amount": "30", "expected_balance": finance.Balance,
	}, 200))
	if blocker := fixture.refresh(t, bill.Account.ID).Account.CancelBlocker; blocker != "payments" {
		t.Fatalf("Con un pago parcial, el impedimento debía ser «payments»: %q", blocker)
	}
	route := "restaurant/accounts/" + bill.Account.ID + "/cancel-account"
	body := fixture.cancelBody(t, bill.Account.ID, "Error de captura", "", nil)
	if code := errorCode(t, fixture.mutate(t, "POST", route, body, 409)); code != "ACCOUNT_HAS_PAYMENTS" {
		t.Fatalf("Pago parcial: %s", code)
	}
	// Sin permiso, el pago se informa antes de pedir al supervisor.
	if code := errorCode(t, fixture.operate(t, fixture.waiter, "POST", route, fixture.cancelBody(t, bill.Account.ID, "Error de captura", "", nil), 409)); code != "ACCOUNT_HAS_PAYMENTS" {
		t.Fatalf("Pago parcial sin permiso: %s", code)
	}
	// Primero la devolución; después procede.
	fixture.mutate(t, "POST", "payments/accounts/"+bill.Account.ID+"/refunds", map[string]any{
		"mode": "payment", "payment_id": paid.Payment.ID, "amount": "30", "method_id": fixture.cash, "reason": "Cliente se retiró",
	}, 200)
	if blocker := fixture.refresh(t, bill.Account.ID).Account.CancelBlocker; blocker != "" {
		t.Fatalf("Tras la devolución no debía haber impedimento: %q", blocker)
	}
	// Con un descuento vigente: primero quitarlo.
	finance = decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+bill.Account.ID, nil, 200))
	fixture.mutate(t, "POST", "payments/accounts/"+bill.Account.ID+"/discounts", map[string]any{
		"scope": "account", "method": "amount", "value": "5", "reason": "Ajuste comercial", "expected_balance": finance.Balance,
	}, 200)
	if blocker := fixture.refresh(t, bill.Account.ID).Account.CancelBlocker; blocker != "adjustments" {
		t.Fatalf("Con un descuento, el impedimento debía ser «adjustments»: %q", blocker)
	}
	if code := errorCode(t, fixture.mutate(t, "POST", route, fixture.cancelBody(t, bill.Account.ID, "Error de captura", "", nil), 409)); code != "ACCOUNT_HAS_ADJUSTMENTS" {
		t.Fatalf("Descuento vigente: %s", code)
	}
	// «Otro» sin descripción, descripción demasiado larga, sin motivo y motivo inexistente.
	other := fixture.order(t, "Motivos inválidos", nil, catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1})
	otherRoute := "restaurant/accounts/" + other.Account.ID + "/cancel-account"
	if code := errorCode(t, fixture.mutate(t, "POST", otherRoute, fixture.cancelBody(t, other.Account.ID, "Otro", "  ", nil), 422)); code != "CANCEL_DESCRIPTION_REQUIRED" {
		t.Fatalf("«Otro» sin descripción: %s", code)
	}
	fixture.mutate(t, "POST", otherRoute, fixture.cancelBody(t, other.Account.ID, "Otro", "Corto", nil), 200)
	long := fixture.order(t, "Descripción larga", nil, catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1})
	longRoute := "restaurant/accounts/" + long.Account.ID + "/cancel-account"
	fixture.mutate(t, "POST", longRoute, fixture.cancelBody(t, long.Account.ID, "Error de captura", strings.Repeat("x", 501), nil), 422)
	fixture.mutate(t, "POST", longRoute, map[string]any{"revision": long.Account.Revision}, 400)
	fixture.mutate(t, "POST", longRoute, map[string]any{"revision": long.Account.Revision, "reason_id": randomID()}, 404)
	fixture.mutate(t, "POST", longRoute, map[string]any{"revision": long.Account.Revision - 1,
		"reason_id": fixture.reasons["Error de captura"].ID}, 409)
	if state := fixture.refresh(t, long.Account.ID).Account.State; state != "open" {
		t.Fatalf("Los rechazos no debían cancelar la cuenta: %s", state)
	}
}

func testStage9DDiscard(t *testing.T, fixture *stage9DFixture) {
	// Para llevar vacía: se descarta sin motivo con accounts.open.
	takeaway := decodeResponse[operations.Bill](t, fixture.operate(t, fixture.waiter, "POST", "restaurant/accounts", map[string]any{"mode": "takeaway"}, 200))
	if !takeaway.Account.Discardable || !takeaway.Account.Releasable {
		t.Fatalf("Una cuenta vacía debía poder descartarse: %+v", takeaway.Account)
	}
	discarded := decodeResponse[operations.Bill](t, fixture.operate(t, fixture.waiter, "POST",
		"restaurant/accounts/"+takeaway.Account.ID+"/discard", map[string]any{"revision": takeaway.Account.Revision}, 200))
	if discarded.Account.State != "voided" || discarded.Cancellation != nil {
		t.Fatalf("Descarte: %+v", discarded)
	}
	// Mesa con solo un borrador (como #1 y #3 de la instalación real): «Liberar mesa» no procede y
	// «Descartar» sí, y borra el borrador.
	draft := decodeResponse[operations.Bill](t, fixture.operate(t, fixture.waiter, "POST", "restaurant/accounts", map[string]any{
		"mode": "dine_in", "table_id": fixture.table.ID, "seen_account_ids": []string{},
		"items": []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1}}},
	}, 200))
	if !draft.Account.Discardable || draft.Account.Releasable || draft.Account.DraftQuantity != 1 {
		t.Fatalf("Cuenta con borrador: %+v", draft.Account)
	}
	route := "restaurant/accounts/" + draft.Account.ID
	if code := errorCode(t, fixture.operate(t, fixture.waiter, "POST", route+"/release", map[string]any{"revision": draft.Account.Revision}, 409)); code != "ACCOUNT_NOT_EMPTY" {
		t.Fatalf("Liberar con borrador: %s", code)
	}
	// Sin accounts.open no se descarta.
	fixture.operate(t, fixture.supervisor, "POST", route+"/discard", map[string]any{"revision": draft.Account.Revision}, 403)
	gone := decodeResponse[operations.Bill](t, fixture.operate(t, fixture.waiter, "POST", route+"/discard",
		map[string]any{"revision": draft.Account.Revision}, 200))
	if gone.Account.State != "voided" || len(gone.Draft) != 0 {
		t.Fatalf("Descarte con borrador: %+v", gone)
	}
	if count := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.restaurant_draft_items WHERE account_id=$1", draft.Account.ID); count != 0 {
		t.Fatalf("El borrador no se borró: %d", count)
	}
	tables := decodeResponse[[]operations.Table](t, fixture.adminCall(t, "GET", "restaurant/tables", nil, 200))
	for _, table := range tables {
		if table.ID == fixture.table.ID && table.State != "free" {
			t.Fatalf("La mesa debía quedar libre tras descartar: %s", table.State)
		}
	}
	// No es cancelación: no tiene registro y queda auditada como descarte.
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.account_cancellations WHERE account_id IN ($1,$2)`,
		takeaway.Account.ID, draft.Account.ID); count != 0 {
		t.Fatalf("Un descarte creó un registro de cancelación: %d", count)
	}
	if count := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.audit_events
        WHERE action='restaurant.account.discarded' AND entity_id IN ($1,$2)`, takeaway.Account.ID, draft.Account.ID); count != 2 {
		t.Fatalf("Auditoría de los descartes: %d", count)
	}
	// Con productos enviados no se descarta: se cancela con motivo.
	sent := fixture.order(t, "Con productos", nil, catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1})
	if sent.Account.Discardable {
		t.Fatal("Una cuenta con productos enviados no se descarta")
	}
	if code := errorCode(t, fixture.mutate(t, "POST", "restaurant/accounts/"+sent.Account.ID+"/discard",
		map[string]any{"revision": sent.Account.Revision}, 409)); code != "ACCOUNT_NOT_EMPTY" {
		t.Fatalf("Descartar con productos: %s", code)
	}
}

func testStage9DDeferredCreation(t *testing.T, fixture *stage9DFixture) {
	items := func() []operations.DraftItem {
		return []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 2}}}
	}
	for _, body := range []map[string]any{
		{"mode": "takeaway", "origin": "Para llevar", "items": items()},
		{"mode": "dine_in", "seen_account_ids": []string{}, "items": items()},
	} {
		before := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.restaurant_accounts")
		bill := decodeResponse[operations.Bill](t, fixture.operate(t, fixture.waiter, "POST", "restaurant/accounts", body, 200))
		if bill.Account.State != "open" || bill.Account.DraftQuantity != 2 || len(bill.Draft) != 1 || bill.Account.TableID != nil ||
			bill.Account.Mode != body["mode"] {
			t.Fatalf("Cuenta creada con su primer producto: %+v", bill.Account)
		}
		if after := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.restaurant_accounts"); after != before+1 {
			t.Fatalf("Se esperaba una sola cuenta nueva: %d → %d", before, after)
		}
		// Se confirma normalmente y queda como cualquier cuenta.
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "GET", "restaurant/accounts/"+bill.Account.ID+"/preview", nil, 200))
		confirmed := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/confirm", map[string]any{
			"revision": bill.Account.Revision, "expected_fingerprint": preview.Fingerprint,
		}, 200))
		if confirmed.Account.ConfirmedQuantity != 2 {
			t.Fatalf("Confirmación de la cuenta diferida: %+v", confirmed.Account)
		}
	}
	// Sin accounts.open no se crea ninguna cuenta, tampoco con su primer producto.
	fixture.operate(t, fixture.supervisor, "POST", "restaurant/accounts", map[string]any{"mode": "takeaway", "items": items()}, 403)
}

func testStage9DLongOpen(t *testing.T, fixture *stage9DFixture) {
	overview := decodeResponse[struct {
		Settings operations.Settings `json:"settings"`
	}](t, fixture.adminCall(t, "GET", "restaurant/overview", nil, 200))
	if overview.Settings.LongOpenHours != 12 {
		t.Fatalf("Umbral predeterminado: %+v", overview.Settings)
	}
	bill := fixture.order(t, "Cuenta antigua", &fixture.table, catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1})
	if bill.Account.LongOpen {
		t.Fatal("Una cuenta recién abierta no lleva mucho tiempo")
	}
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.restaurant_accounts SET created_at=now()-interval '13 hours' WHERE id=$1", bill.Account.ID); err != nil {
		t.Fatal(err)
	}
	page := decodeResponse[operations.Bill](t, fixture.adminCall(t, "GET", "restaurant/accounts/"+bill.Account.ID, nil, 200))
	if !page.Account.LongOpen {
		t.Fatal("A las 13 horas la cuenta debía marcarse como abierta por mucho tiempo")
	}
	list := decodeResponse[struct {
		Items    []operations.Account `json:"items"`
		Settings operations.Settings  `json:"settings"`
	}](t, fixture.adminCall(t, "GET", "restaurant/accounts?mode=dine_in", nil, 200))
	marked := false
	for _, account := range list.Items {
		marked = marked || (account.ID == bill.Account.ID && account.LongOpen)
	}
	if !marked || list.Settings.LongOpenHours != 12 {
		t.Fatalf("La lista de cuentas no informa la marca o el umbral: %+v", list.Settings)
	}
	tableMarked := func() bool {
		for _, table := range decodeResponse[[]operations.Table](t, fixture.adminCall(t, "GET", "restaurant/tables", nil, 200)) {
			if table.ID == fixture.table.ID {
				return table.LongOpen
			}
		}
		return false
	}
	if !tableMarked() {
		t.Fatal("La mesa con la cuenta antigua debía marcarse")
	}
	// Con 24 horas de umbral ya no se marca (lo cambia la ruta de «Funciones del establecimiento»).
	current := decodeResponse[map[string]any](t, fixture.adminCall(t, "GET", "settings/long-open-hours", nil, 200))
	fixture.adminCall(t, "PUT", "settings/long-open-hours", map[string]any{"long_open_hours": 24, "revision": current["revision"]}, 200)
	if fixture.refresh(t, bill.Account.ID).Account.LongOpen || tableMarked() {
		t.Fatal("Con 24 horas de umbral la cuenta de 13 horas no debía marcarse")
	}
}
