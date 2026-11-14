//go:build integration

package operations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
)

// Etapa 9G (fase D) contra PostgreSQL con la API real:
//   - §5.2 idempotencia: reintentos simultáneos con la misma clave (goroutines: un solo efecto y la misma respuesta),
//     reintento después de una respuesta perdida y la misma clave con otro contenido, otra terminal u otro usuario.
//   - §5.5 pedido capturado sin conexión: sin cambios (se acepta), doble envío (también simultáneo), cambio de
//     precio con confirmación del mesero, confirmación con un catálogo todavía viejo (se rechaza de nuevo), producto
//     agotado, producto eliminado, cuenta cerrada mientras tanto y mesa con una cuenta nueva de otra terminal; con su
//     Registro de movimientos (aceptado y rechazos).
//   - §5.6 recuperación del estado: mesas y cuentas con su revisión y preparación, permisos vigentes, permiso
//     revocado y terminal revocada.
//
// Usa su propia base desde cero.

type stage9GFixture struct {
	*restaurantFixture
	waiter   testTerminal
	waiterID string
	roleID   string
	third    operations.Table
}

func resetStage9GDatabase(t *testing.T) {
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

// raw envía la petición sin comprobar el estado (también desde goroutines).
func (fixture *stage9GFixture) raw(terminal testTerminal, method, route string, body any) (int, []byte) {
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:51900"
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response.Code, append([]byte{}, response.Body.Bytes()...)
}

func (fixture *stage9GFixture) scalar(t *testing.T, query string, args ...any) int {
	t.Helper()
	var value int
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// canonical normaliza un JSON (orden de claves) para comparar la primera respuesta con la repetida, que se guarda en
// jsonb.
func canonical(t *testing.T, body []byte) string {
	t.Helper()
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("respuesta inválida: %s", body)
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

type offlineRejection struct {
	Error struct {
		Code    string `json:"code"`
		Details struct {
			Status          string                     `json:"status"`
			CatalogVersion  int64                      `json:"catalog_version"`
			RequiresChanges bool                       `json:"requires_changes"`
			Changes         []operations.OfflineChange `json:"changes"`
		} `json:"details"`
	} `json:"error"`
}

func (fixture *stage9GFixture) catalogVersion(t *testing.T) int64 {
	t.Helper()
	return int64(fixture.scalar(t, "SELECT coalesce(max(version),0)::int FROM app.catalog_change_log"))
}

// offlineBody arma un pedido capturado sin conexión con su clave, la versión del catálogo y la hora de captura.
func offlineBody(key string, version int64, destination map[string]any, items ...map[string]any) map[string]any {
	return map[string]any{
		"operation_id":    key,
		"catalog_version": version,
		"captured_at":     time.Now().Add(-3 * time.Minute).UTC().Format(time.RFC3339Nano),
		"destination":     destination,
		"items":           items,
	}
}

func offlineItem(product string, quantity int, seen string) map[string]any {
	return map[string]any{
		"id": randomID(), "product_id": product, "variant_id": nil, "quantity": quantity, "seen_unit_price": seen,
		"customization": catalog.Customization{
			Ingredients: []catalog.IngredientSelection{}, Groups: []catalog.GroupSelection{}, Components: []catalog.ComponentSelection{},
		},
	}
}

func (fixture *stage9GFixture) setPrice(t *testing.T, product catalog.Product, price string) catalog.Product {
	t.Helper()
	current := decodeResponse[catalog.Product](t, fixture.adminCall(t, "GET", "catalog/products/"+product.ID, nil, 200))
	current.BasePrice = price
	return decodeResponse[catalog.Product](t, fixture.adminCall(t, "PATCH", "catalog/products/"+product.ID, current, 200))
}

func TestStage9GOfflineOrdersAndIdempotency(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &stage9GFixture{restaurantFixture: newFixture(t)}
	fixture.seedCatalog(t)
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Terraza 9G", Active: true}, 200))
	tables := []*operations.Table{&fixture.table, &fixture.secondTable, &fixture.third}
	for index, table := range tables {
		*table = decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: fmt.Sprintf("Mesa 9G-%d", index+1), Shape: "rectangle", X: 20 + index*130, Y: 30,
			Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200))
	}
	fixture.waiter = inventoryUser(t, fixture.restaurantFixture, "mesero_9g", []string{
		"accounts.read", "accounts.open", "orders.capture", "orders.confirm", "catalog.read", "catalog.prices.read",
		"restaurant.tables.read", "events.read",
	})
	var waiterID, roleID string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text,role_id::text FROM app.users WHERE username='mesero_9g'").Scan(&waiterID, &roleID); err != nil {
		t.Fatal(err)
	}
	fixture.waiterID, fixture.roleID = waiterID, roleID
	t.Run("idempotencia: reintentos simultáneos, respuesta perdida y otro contenido", func(t *testing.T) {
		testStage9GIdempotency(t, fixture)
	})
	t.Run("pedido sin conexión sin cambios y doble envío", func(t *testing.T) {
		testStage9GOfflineAccepted(t, fixture)
	})
	t.Run("pedido sin conexión con cambio de precio y confirmación con catálogo viejo", func(t *testing.T) {
		testStage9GOfflinePriceChange(t, fixture)
	})
	t.Run("pedido sin conexión con producto agotado o eliminado", func(t *testing.T) {
		testStage9GOfflineUnavailable(t, fixture)
	})
	t.Run("pedido sin conexión con la cuenta cerrada o la mesa ocupada mientras tanto", func(t *testing.T) {
		testStage9GOfflineDestination(t, fixture)
	})
	t.Run("forma, permisos y versión del pedido sin conexión", func(t *testing.T) {
		testStage9GOfflineValidation(t, fixture)
	})
	t.Run("revisión: reenvío confirmado después de una repetición tardía que ya entró", func(t *testing.T) {
		testStage9GOfflineLateRepeat(t, fixture)
	})
	t.Run("revisión: el borrador no acepta productos agotados", func(t *testing.T) {
		testStage9GDraftSoldOut(t, fixture)
	})
	t.Run("recuperación del estado con permiso y terminal revocados", func(t *testing.T) {
		testStage9GRecovery(t, fixture)
	})
}

func testStage9GIdempotency(t *testing.T, fixture *stage9GFixture) {
	key := randomID()
	body := map[string]any{
		"operation_id": key, "mode": "dine_in", "table_id": fixture.table.ID, "seen_account_ids": []string{},
		"items": []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 2}}},
	}
	// Cinco reintentos simultáneos de la misma apertura con su primer producto.
	responses := make([]string, 5)
	codes := make([]int, 5)
	var wait sync.WaitGroup
	for index := range responses {
		wait.Add(1)
		go func() {
			defer wait.Done()
			code, response := fixture.raw(fixture.waiter, "POST", "restaurant/accounts", body)
			codes[index], responses[index] = code, string(response)
		}()
	}
	wait.Wait()
	for index := range responses {
		if codes[index] != 200 || canonical(t, []byte(responses[index])) != canonical(t, []byte(responses[0])) {
			t.Fatalf("reintento simultáneo %d: %d %s", index, codes[index], responses[index])
		}
	}
	if accounts := fixture.scalar(t, "SELECT count(*) FROM app.restaurant_accounts WHERE table_id=$1", fixture.table.ID); accounts != 1 {
		t.Fatalf("los reintentos simultáneos abrieron %d cuentas", accounts)
	}
	if keys := fixture.scalar(t, "SELECT count(*) FROM app.restaurant_operations WHERE id=$1", key); keys != 1 {
		t.Fatalf("la clave se guardó %d veces", keys)
	}
	// Reintento después de una respuesta perdida: la misma respuesta y ningún efecto nuevo.
	again := fixture.call(t, fixture.waiter, "POST", "restaurant/accounts", body, 200)
	if canonical(t, again) != canonical(t, []byte(responses[0])) {
		t.Fatal("el reintento no devolvió la respuesta guardada")
	}
	if drafts := fixture.scalar(t, `SELECT count(*) FROM app.restaurant_draft_items d JOIN app.restaurant_accounts a ON a.id=d.account_id
        WHERE a.table_id=$1`, fixture.table.ID); drafts != 1 {
		t.Fatalf("el reintento repitió el borrador: %d partidas", drafts)
	}
	// La misma clave con otro contenido, desde otra terminal o con otro usuario se rechaza sin efecto.
	changed := map[string]any{}
	for name, value := range body {
		changed[name] = value
	}
	changed["table_id"] = fixture.secondTable.ID
	for _, attempt := range []struct {
		terminal testTerminal
		body     map[string]any
	}{{fixture.waiter, changed}, {fixture.admin, body}} {
		code, response := fixture.raw(attempt.terminal, "POST", "restaurant/accounts", attempt.body)
		if code != 409 || !bytes.Contains(response, []byte("IDEMPOTENCY_CONFLICT")) {
			t.Fatalf("misma clave con otros datos: %d %s", code, response)
		}
	}
	if accounts := fixture.scalar(t, "SELECT count(*) FROM app.restaurant_accounts"); accounts != 1 {
		t.Fatalf("un conflicto abrió otra cuenta: %d", accounts)
	}
	// Confirmar el borrador con reintentos simultáneos: un solo pedido.
	bill := decodeResponse[operations.Bill](t, []byte(responses[0]))
	preview := decodeResponse[operations.QuoteResult](t, fixture.call(t, fixture.waiter, "GET", "restaurant/accounts/"+bill.Account.ID+"/preview", nil, 200))
	confirm := map[string]any{"operation_id": randomID(), "revision": bill.Account.Revision, "expected_fingerprint": preview.Fingerprint}
	for index := range codes {
		wait.Add(1)
		go func() {
			defer wait.Done()
			codes[index], _ = fixture.raw(fixture.waiter, "POST", "restaurant/accounts/"+bill.Account.ID+"/confirm", confirm)
		}()
	}
	wait.Wait()
	for _, code := range codes {
		if code != 200 {
			t.Fatalf("una confirmación repetida falló: %v", codes)
		}
	}
	if orders := fixture.scalar(t, "SELECT count(*) FROM app.restaurant_orders WHERE original_account_id=$1", bill.Account.ID); orders != 1 {
		t.Fatalf("las confirmaciones simultáneas registraron %d pedidos", orders)
	}
}

func testStage9GOfflineAccepted(t *testing.T, fixture *stage9GFixture) {
	version := fixture.catalogVersion(t)
	key := randomID()
	body := offlineBody(key, version, map[string]any{"table_id": fixture.secondTable.ID, "seen_account_ids": []string{}},
		offlineItem(fixture.product.ID, 2, "150.00"), offlineItem(fixture.fries.ID, 1, "40"))
	// Doble envío simultáneo (el teléfono reintentó): un solo pedido y la misma respuesta.
	responses := make([][]byte, 3)
	codes := make([]int, 3)
	var wait sync.WaitGroup
	for index := range responses {
		wait.Add(1)
		go func() {
			defer wait.Done()
			codes[index], responses[index] = fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", body)
		}()
	}
	wait.Wait()
	for index := range responses {
		if codes[index] != 200 || canonical(t, responses[index]) != canonical(t, responses[0]) {
			t.Fatalf("envío %d del pedido sin conexión: %d %s", index, codes[index], responses[index])
		}
	}
	result := decodeResponse[operations.OfflineOrderResult](t, responses[0])
	if result.Status != "accepted" || result.CatalogVersion != version || len(result.AcceptedChanges) != 0 || result.Bill == nil ||
		result.Bill.Account.TableID == nil || *result.Bill.Account.TableID != fixture.secondTable.ID || len(result.Bill.Lines) != 2 ||
		result.Bill.Account.Totals.Total != "340.00" || len(result.Bill.Draft) != 0 {
		t.Fatalf("pedido aceptado: %s", responses[0])
	}
	if orders := fixture.scalar(t, "SELECT count(*) FROM app.restaurant_orders WHERE operation_id=$1", key); orders != 1 {
		t.Fatalf("el doble envío registró %d pedidos", orders)
	}
	// Envío repetido más tarde (respuesta perdida): la misma respuesta.
	if again := fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders", body, 200); canonical(t, again) != canonical(t, responses[0]) {
		t.Fatal("el reenvío del pedido aceptado no devolvió la respuesta guardada")
	}
	// Registro de movimientos: capturado sin conexión, con la hora de captura y de llegada.
	var after map[string]any
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT after_state FROM app.audit_events
        WHERE action='restaurant.offline_order.accepted' AND entity_id=$1`, key).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after["captured_at"] == nil || after["received_at"] == nil || after["order_id"] != result.OrderID {
		t.Fatalf("auditoría del pedido sin conexión: %v", after)
	}
	if offline := fixture.scalar(t, `SELECT count(*) FROM app.restaurant_events WHERE kind='order.confirmed'
        AND account_id=$1 AND payload->'after'->'offline'->>'captured_at' IS NOT NULL`, result.AccountID); offline != 1 {
		t.Fatalf("la historia de la cuenta no marca el pedido sin conexión: %d", offline)
	}
}

func testStage9GOfflinePriceChange(t *testing.T, fixture *stage9GFixture) {
	accountID := decodeResponse[operations.OfflineOrderResult](t, fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders",
		offlineBody(randomID(), fixture.catalogVersion(t), map[string]any{"table_id": fixture.third.ID},
			offlineItem(fixture.fries.ID, 1, "40.00")), 200)).AccountID
	captured := fixture.catalogVersion(t)
	fixture.product = fixture.setPrice(t, fixture.product, "165")
	key := randomID()
	body := offlineBody(key, captured, map[string]any{"account_id": accountID}, offlineItem(fixture.product.ID, 1, "150.00"))
	unitsBefore := fixture.scalar(t, "SELECT count(*) FROM app.restaurant_item_units WHERE account_id=$1", accountID)
	code, response := fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", body)
	rejection := decodeResponse[offlineRejection](t, response)
	changes := rejection.Error.Details.Changes
	if code != 409 || rejection.Error.Code != "ORDER_REQUIRES_CONFIRMATION" || rejection.Error.Details.RequiresChanges ||
		len(changes) != 1 || changes[0].Kind != operations.OfflinePriceChanged || *changes[0].Before != "150.00" ||
		*changes[0].Now != "165.00" || !changes[0].Acceptable || !strings.Contains(changes[0].Message, "$150.00 → $165.00") {
		t.Fatalf("cambio de precio: %d %s", code, response)
	}
	current := rejection.Error.Details.CatalogVersion
	if current <= captured || fixture.scalar(t, "SELECT count(*) FROM app.restaurant_item_units WHERE account_id=$1", accountID) != unitsBefore {
		t.Fatal("el pedido con cambios entró o la versión no subió")
	}
	if fixture.scalar(t, `SELECT count(*) FROM app.audit_events WHERE action='restaurant.offline_order.requires_confirmation'
        AND entity_id=$1 AND NOT success`, key) != 1 {
		t.Fatal("el rechazo no quedó en el Registro de movimientos")
	}
	// Mientras el mesero revisa, el precio vuelve a cambiar: su confirmación con el catálogo anterior se rechaza de
	// nuevo con los cambios vigentes.
	fixture.product = fixture.setPrice(t, fixture.product, "170")
	body["confirmation"] = map[string]any{"catalog_version": current, "changes": changes}
	code, response = fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", body)
	again := decodeResponse[offlineRejection](t, response)
	if code != 409 || again.Error.Details.CatalogVersion <= current || len(again.Error.Details.Changes) != 1 ||
		*again.Error.Details.Changes[0].Now != "170.00" {
		t.Fatalf("confirmación con catálogo viejo: %d %s", code, response)
	}
	// Una confirmación que no cubre el cambio vigente (vio otro precio) tampoco se acepta.
	body["confirmation"] = map[string]any{"catalog_version": again.Error.Details.CatalogVersion, "changes": changes}
	if code, _ := fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", body); code != 409 {
		t.Fatalf("se aceptó una confirmación de otro precio: %d", code)
	}
	// Con la versión vigente y los cambios que devolvió el último rechazo, se acepta con el precio del servidor.
	body["confirmation"] = map[string]any{"catalog_version": again.Error.Details.CatalogVersion, "changes": again.Error.Details.Changes}
	accepted := decodeResponse[operations.OfflineOrderResult](t, fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders", body, 200))
	if len(accepted.AcceptedChanges) != 1 || *accepted.AcceptedChanges[0].Now != "170.00" {
		t.Fatalf("cambios aceptados: %+v", accepted.AcceptedChanges)
	}
	var unitPrice string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT v.quote_line->>'unit_price' FROM app.restaurant_item_versions v
        JOIN app.restaurant_orders o ON o.id=v.order_id WHERE o.operation_id=$1`, key).Scan(&unitPrice); err != nil || unitPrice != "170.00" {
		t.Fatalf("el pedido no entró con el precio del servidor: %q %v", unitPrice, err)
	}
	var audited map[string]any
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT after_state FROM app.audit_events
        WHERE action='restaurant.offline_order.accepted' AND entity_id=$1`, key).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if accepted, _ := audited["accepted_changes"].([]any); len(accepted) != 1 {
		t.Fatalf("la auditoría no guarda los cambios aceptados: %v", audited)
	}
	// El primer envío sin confirmación, si llega tarde con la misma clave, ya no crea otro pedido: recibe el resultado
	// que sí entró (revisión de la fase D, hallazgo 4).
	delete(body, "confirmation")
	code, response = fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", body)
	late := decodeResponse[alreadyAccepted](t, response)
	if code != 409 || late.Error.Code != "OFFLINE_ORDER_ALREADY_ACCEPTED" || late.Error.Details.Result.OrderID != accepted.OrderID {
		t.Fatalf("el envío tardío sin confirmación: %d %s", code, response)
	}
	fixture.product = fixture.setPrice(t, fixture.product, "150")
}

// alreadyAccepted es el rechazo de un reenvío con la clave de un pedido sin conexión que ya entró con otro contenido.
type alreadyAccepted struct {
	Error struct {
		Code    string `json:"code"`
		Details struct {
			Status string                        `json:"status"`
			Result operations.OfflineOrderResult `json:"result"`
		} `json:"details"`
	} `json:"error"`
}

// Revisión de la fase D, hallazgo 4: el original se rechaza por precio (sin guardar la clave), el precio vuelve, entra
// una repetición tardía del original y después llega el reenvío con la confirmación del mesero: no se registra otra
// vez y recibe el pedido que sí entró. Otro usuario con la misma clave no ve ese resultado.
func testStage9GOfflineLateRepeat(t *testing.T, fixture *stage9GFixture) {
	accountID := decodeResponse[operations.OfflineOrderResult](t, fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders",
		offlineBody(randomID(), fixture.catalogVersion(t), map[string]any{"table_id": fixture.third.ID},
			offlineItem(fixture.fries.ID, 1, "40.00")), 200)).AccountID
	fixture.product = fixture.setPrice(t, fixture.product, "175")
	key := randomID()
	body := offlineBody(key, fixture.catalogVersion(t), map[string]any{"account_id": accountID}, offlineItem(fixture.product.ID, 1, "150.00"))
	code, response := fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", body)
	rejection := decodeResponse[offlineRejection](t, response)
	if code != 409 || rejection.Error.Code != "ORDER_REQUIRES_CONFIRMATION" {
		t.Fatalf("original con otro precio: %d %s", code, response)
	}
	fixture.product = fixture.setPrice(t, fixture.product, "150")
	accepted := decodeResponse[operations.OfflineOrderResult](t, fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders", body, 200))
	confirmed := map[string]any{}
	for name, value := range body {
		confirmed[name] = value
	}
	confirmed["confirmation"] = map[string]any{"catalog_version": rejection.Error.Details.CatalogVersion, "changes": rejection.Error.Details.Changes}
	code, response = fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", confirmed)
	late := decodeResponse[alreadyAccepted](t, response)
	result := late.Error.Details.Result
	if code != 409 || late.Error.Code != "OFFLINE_ORDER_ALREADY_ACCEPTED" || late.Error.Details.Status != "already_accepted" ||
		result.OrderID != accepted.OrderID || result.AccountID != accountID || result.Status != "accepted" {
		t.Fatalf("reenvío confirmado después de la repetición tardía: %d %s", code, response)
	}
	if orders := fixture.scalar(t, "SELECT count(*) FROM app.restaurant_orders WHERE operation_id=$1", key); orders != 1 {
		t.Fatalf("el pedido entró %d veces", orders)
	}
	// La misma clave desde otro usuario y otra terminal: el conflicto de siempre, sin el pedido de nadie.
	code, response = fixture.raw(fixture.admin, "POST", "restaurant/offline-orders", confirmed)
	if code != 409 || !bytes.Contains(response, []byte("IDEMPOTENCY_CONFLICT")) || bytes.Contains(response, []byte(accepted.OrderID)) {
		t.Fatalf("la misma clave desde otra terminal: %d %s", code, response)
	}
}

// Revisión de la fase D, hallazgo 3: un producto agotado no se puede capturar, tampoco al guardar el borrador o al
// abrir una cuenta con su primer producto (ni como componente de un combo); una partida ya guardada cuyo producto se
// agotó después se puede conservar o reducir, pero no aumentar.
func testStage9GDraftSoldOut(t *testing.T, fixture *stage9GFixture) {
	empty := catalog.Customization{Ingredients: []catalog.IngredientSelection{}, Groups: []catalog.GroupSelection{}, Components: []catalog.ComponentSelection{}}
	fries := operations.DraftItem{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 2, Customization: empty}}
	opened := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "POST", "restaurant/accounts", map[string]any{
		"operation_id": randomID(), "mode": "dine_in", "items": []operations.DraftItem{fries},
	}, 200))
	route := "restaurant/accounts/" + opened.Account.ID + "/draft"
	fixture.adminCall(t, "POST", "catalog/products/"+fixture.fries.ID+"/sold-out", map[string]any{"operation_id": randomID(), "sold_out": true}, 200)
	steak := operations.DraftItem{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.product.ID, Quantity: 1, Customization: empty}}
	accounts := fixture.scalar(t, "SELECT count(*) FROM app.restaurant_accounts")
	// Abrir otra cuenta con las papas agotadas: nada se crea.
	code, response := fixture.raw(fixture.waiter, "POST", "restaurant/accounts", map[string]any{
		"operation_id": randomID(), "mode": "dine_in",
		"items": []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1, Customization: empty}}},
	})
	if code != 409 || !bytes.Contains(response, []byte("PRODUCT_SOLD_OUT")) || fixture.scalar(t, "SELECT count(*) FROM app.restaurant_accounts") != accounts {
		t.Fatalf("abrir una cuenta con un producto agotado: %d %s", code, response)
	}
	// La partida guardada se conserva igual al agregar otro producto, y se puede reducir.
	bill := fixture.refresh(t, opened.Account.ID)
	bill = decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "PUT", route, map[string]any{
		"operation_id": randomID(), "revision": bill.Account.Revision, "items": []operations.DraftItem{fries, steak},
	}, 200))
	reduced := fries
	reduced.Quantity = 1
	bill = decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "PUT", route, map[string]any{
		"operation_id": randomID(), "revision": bill.Account.Revision, "items": []operations.DraftItem{reduced, steak},
	}, 200))
	// Aumentarla, agregar otra partida del producto agotado o un combo con las papas: 409 con el mismo error claro.
	more := fries
	more.Quantity = 3
	another := operations.DraftItem{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1, Customization: empty}}
	combo := operations.DraftItem{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.combo.ID, Quantity: 1,
		Customization: catalog.Customization{Components: []catalog.ComponentSelection{{
			SlotID: fixture.combo.Slots[0].ID, ProductID: fixture.fries.ID, Quantity: 1,
		}}}}}
	for name, items := range map[string][]operations.DraftItem{
		"aumentar":     {more, steak},
		"otra partida": {reduced, steak, another},
		"combo":        {reduced, steak, combo},
	} {
		code, response := fixture.raw(fixture.waiter, "PUT", route, map[string]any{
			"operation_id": randomID(), "revision": bill.Account.Revision, "items": items,
		})
		if code != 409 || !bytes.Contains(response, []byte("PRODUCT_SOLD_OUT")) || !bytes.Contains(response, []byte("Papas producto está agotado")) {
			t.Fatalf("%s con el producto agotado: %d %s", name, code, response)
		}
	}
	if current := fixture.refresh(t, opened.Account.ID); current.Account.Revision != bill.Account.Revision || len(current.Draft) != 2 {
		t.Fatal("un borrador rechazado cambió la cuenta")
	}
	// Con las papas disponibles otra vez, se guarda.
	fixture.adminCall(t, "POST", "catalog/products/"+fixture.fries.ID+"/sold-out", map[string]any{"operation_id": randomID(), "sold_out": false}, 200)
	fixture.call(t, fixture.waiter, "PUT", route, map[string]any{
		"operation_id": randomID(), "revision": bill.Account.Revision, "items": []operations.DraftItem{more, steak, combo},
	}, 200)
}

func testStage9GOfflineUnavailable(t *testing.T, fixture *stage9GFixture) {
	version := fixture.catalogVersion(t)
	fixture.adminCall(t, "POST", "catalog/products/"+fixture.fries.ID+"/sold-out", map[string]any{"operation_id": randomID(), "sold_out": true}, 200)
	temporary := decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Temporal 9G", Active: true}, CategoryID: fixture.category.ID, Code: "TEMP-9G", Kind: "product",
		BasePrice: "10",
	}, 200))
	if _, err := fixture.pool.Exec(fixture.ctx, "DELETE FROM app.catalog_products WHERE id=$1", temporary.ID); err != nil {
		t.Fatal(err)
	}
	key := randomID()
	fries, removed, steak := offlineItem(fixture.fries.ID, 1, "40.00"), offlineItem(temporary.ID, 1, "10.00"), offlineItem(fixture.product.ID, 1, "150.00")
	body := offlineBody(key, version, map[string]any{"table_id": fixture.table.ID, "seen_account_ids": fixture.openAccounts(t, fixture.table.ID)},
		fries, removed, steak)
	code, response := fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", body)
	rejection := decodeResponse[offlineRejection](t, response)
	kinds := map[string]operations.OfflineChange{}
	for _, change := range rejection.Error.Details.Changes {
		kinds[change.Kind] = change
	}
	soldOut, gone := kinds[operations.OfflineProductSoldOut], kinds[operations.OfflineProductRemoved]
	if code != 409 || !rejection.Error.Details.RequiresChanges || len(kinds) != 2 || soldOut.Acceptable || gone.Acceptable ||
		*soldOut.ItemID != fries["id"] || *gone.ItemID != removed["id"] || !strings.Contains(soldOut.Message, "agotado") {
		t.Fatalf("producto agotado y eliminado: %d %s", code, response)
	}
	// Ni con la confirmación del mesero entra un producto agotado: hay que quitarlo.
	body["confirmation"] = map[string]any{"catalog_version": rejection.Error.Details.CatalogVersion, "changes": rejection.Error.Details.Changes}
	if code, _ := fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", body); code != 409 {
		t.Fatalf("se aceptó un producto agotado con confirmación: %d", code)
	}
	// Sin los productos que ya no existen o están agotados, se acepta.
	body["items"] = []map[string]any{steak}
	delete(body, "confirmation")
	accepted := decodeResponse[operations.OfflineOrderResult](t, fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders", body, 200))
	if len(accepted.Bill.Lines) != 1 || accepted.Bill.Lines[0].QuoteLine.UnitPrice != "150.00" {
		t.Fatalf("pedido corregido: %+v", accepted.Bill.Lines)
	}
	fixture.adminCall(t, "POST", "catalog/products/"+fixture.fries.ID+"/sold-out", map[string]any{"operation_id": randomID(), "sold_out": false}, 200)
}

// openAccounts devuelve las cuentas abiertas de una mesa (las que el mesero veía).
func (fixture *stage9GFixture) openAccounts(t *testing.T, tableID string) []string {
	t.Helper()
	rows, err := fixture.pool.Query(fixture.ctx, "SELECT id::text FROM app.restaurant_accounts WHERE table_id=$1 AND state='open'", tableID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func testStage9GOfflineDestination(t *testing.T, fixture *stage9GFixture) {
	// Mesa cerrada mientras tanto: la cuenta del pedido se descarta (otra terminal) antes de que llegue.
	opened := decodeResponse[operations.Bill](t, fixture.call(t, fixture.admin, "POST", "restaurant/accounts", map[string]any{
		"operation_id": randomID(), "mode": "dine_in", "table_id": fixture.third.ID,
		"items": []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1}}},
	}, 200))
	fixture.call(t, fixture.admin, "POST", "restaurant/accounts/"+opened.Account.ID+"/discard",
		map[string]any{"operation_id": randomID(), "revision": opened.Account.Revision}, 200)
	body := offlineBody(randomID(), fixture.catalogVersion(t), map[string]any{"account_id": opened.Account.ID},
		offlineItem(fixture.fries.ID, 1, "40.00"))
	code, response := fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", body)
	rejection := decodeResponse[offlineRejection](t, response)
	if code != 409 || len(rejection.Error.Details.Changes) != 1 || rejection.Error.Details.Changes[0].Kind != operations.OfflineAccountClosed ||
		rejection.Error.Details.Changes[0].Acceptable {
		t.Fatalf("cuenta cerrada: %d %s", code, response)
	}
	// Mesa ocupada mientras tanto por otra terminal: el mesero lo confirma y se abre otra cuenta en la mesa.
	seen := fixture.openAccounts(t, fixture.secondTable.ID)
	decodeResponse[operations.Bill](t, fixture.call(t, fixture.admin, "POST", "restaurant/accounts", map[string]any{
		"operation_id": randomID(), "mode": "dine_in", "table_id": fixture.secondTable.ID, "name": "Otra terminal",
		"items": []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1}}},
	}, 200))
	body = offlineBody(randomID(), fixture.catalogVersion(t), map[string]any{"table_id": fixture.secondTable.ID, "seen_account_ids": seen},
		offlineItem(fixture.fries.ID, 1, "40.00"))
	code, response = fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", body)
	rejection = decodeResponse[offlineRejection](t, response)
	if code != 409 || len(rejection.Error.Details.Changes) != 1 || rejection.Error.Details.Changes[0].Kind != operations.OfflineTableHasAccount ||
		!rejection.Error.Details.Changes[0].Acceptable {
		t.Fatalf("mesa ocupada: %d %s", code, response)
	}
	accountsBefore := len(fixture.openAccounts(t, fixture.secondTable.ID))
	body["confirmation"] = map[string]any{"catalog_version": rejection.Error.Details.CatalogVersion, "changes": rejection.Error.Details.Changes}
	accepted := decodeResponse[operations.OfflineOrderResult](t, fixture.call(t, fixture.waiter, "POST", "restaurant/offline-orders", body, 200))
	if len(fixture.openAccounts(t, fixture.secondTable.ID)) != accountsBefore+1 || accepted.Bill.Account.State != "open" {
		t.Fatal("la confirmación no abrió otra cuenta en la mesa")
	}
}

func testStage9GOfflineValidation(t *testing.T, fixture *stage9GFixture) {
	version := fixture.catalogVersion(t)
	item := offlineItem(fixture.fries.ID, 1, "40.00")
	cases := []map[string]any{
		offlineBody(randomID(), version, map[string]any{}, item),
		offlineBody(randomID(), version, map[string]any{"table_id": fixture.table.ID, "account_id": randomID()}, item),
		offlineBody(randomID(), -1, map[string]any{"table_id": fixture.table.ID}, item),
		offlineBody(randomID(), version, map[string]any{"table_id": fixture.table.ID}),
		offlineBody(randomID(), version, map[string]any{"table_id": fixture.table.ID}, offlineItem(fixture.fries.ID, 1, "40.001")),
		offlineBody(randomID(), version, map[string]any{"table_id": fixture.table.ID}, offlineItem(fixture.fries.ID, 1, "")),
	}
	future := offlineBody(randomID(), version, map[string]any{"table_id": fixture.table.ID}, item)
	future["captured_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	cases = append(cases, future)
	for index, body := range cases {
		if code, response := fixture.raw(fixture.waiter, "POST", "restaurant/offline-orders", body); code != 400 {
			t.Fatalf("caso inválido %d: %d %s", index, code, response)
		}
	}
	// Sin orders.confirm no se envía un pedido sin conexión.
	capture := inventoryUser(t, fixture.restaurantFixture, "capturista_9g", []string{
		"accounts.read", "accounts.open", "orders.capture", "catalog.read", "catalog.prices.read",
	})
	if code, _ := fixture.raw(capture, "POST", "restaurant/offline-orders", offlineBody(randomID(), version,
		map[string]any{"table_id": fixture.table.ID}, item)); code != 403 {
		t.Fatalf("sin orders.confirm: %d", code)
	}
}

func testStage9GRecovery(t *testing.T, fixture *stage9GFixture) {
	type state struct {
		CatalogVersion int64                `json:"catalog_version"`
		User           map[string]any       `json:"user"`
		Terminal       map[string]any       `json:"terminal"`
		Tables         []operations.Table   `json:"tables"`
		Accounts       []operations.Account `json:"accounts"`
	}
	current := decodeResponse[state](t, fixture.call(t, fixture.waiter, "GET", "restaurant/state", nil, 200))
	permissions, _ := current.User["permissions"].([]any)
	if current.CatalogVersion != fixture.catalogVersion(t) || len(current.Tables) != 3 || len(current.Accounts) == 0 ||
		current.Terminal["id"] != fixture.waiter.deviceID || len(permissions) != 8 {
		t.Fatalf("estado: versión %d, mesas %d, cuentas %d, permisos %v", current.CatalogVersion, len(current.Tables), len(current.Accounts), permissions)
	}
	for _, account := range current.Accounts {
		if account.State != "open" || account.Revision < 1 || account.PreparationState == "" || account.PaymentState == "" {
			t.Fatalf("cuenta del estado: %+v", account)
		}
	}
	// Se le revoca accounts.read: al reconectar recibe sus permisos vigentes, sin cuentas.
	if _, err := fixture.pool.Exec(fixture.ctx, "DELETE FROM app.role_permissions WHERE role_id=$1 AND permission_key='accounts.read'",
		fixture.roleID); err != nil {
		t.Fatal(err)
	}
	reduced := decodeResponse[map[string]any](t, fixture.call(t, fixture.waiter, "GET", "restaurant/state", nil, 200))
	user, _ := reduced["user"].(map[string]any)
	if reduced["accounts"] != nil || reduced["tables"] == nil || strings.Contains(fmt.Sprint(user["permissions"]), "accounts.read") {
		t.Fatalf("estado con el permiso revocado: %v", reduced)
	}
	// La terminal revocada se entera al reconectar.
	fixture.adminCall(t, "POST", "devices/"+fixture.waiter.deviceID+"/revoke", nil, 200)
	code, response := fixture.raw(fixture.waiter, "GET", "restaurant/state", nil)
	if code != 401 || !bytes.Contains(response, []byte("DEVICE_REVOKED")) {
		t.Fatalf("terminal revocada: %d %s", code, response)
	}
}
