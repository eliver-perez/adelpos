//go:build integration

package catalog_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/database"
)

// Etapa 9G (fase D) contra PostgreSQL con la API real: «Agotado» (§5.4: permiso propio, idempotencia con reintentos
// simultáneos, fin con el inicio del día comercial, «hasta nuevo aviso», rechazo al capturar el producto o un
// complemento agotado, evento en tiempo real, Registro de movimientos y reinicio automático), el catálogo
// descargable por versión (§5.3: completo, solo cambios con bajas y archivados, versión mayor que la vigente) y la
// imagen binaria con caché (ETag e If-None-Match). Usa su propia base desde cero.

type stage9GTerminal struct {
	deviceID, deviceToken, sessionToken string
}

type stage9GCatalog struct {
	ctx          context.Context
	pool         *pgxpool.Pool
	service      *administration.Service
	provisioning string
	admin        stage9GTerminal
	cashier      stage9GTerminal
	category     catalog.Category
	burger       catalog.Product
	fries        catalog.Product
	plate        catalog.Product
	group        catalog.OptionGroup
}

func stage9GID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	raw[6] = raw[6]&15 | 64
	raw[8] = raw[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

// send hace la petición y devuelve la respuesta sin comprobarla (se usa también desde goroutines).
func (fixture *stage9GCatalog) send(terminal stage9GTerminal, method, route string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, "/api/v1/"+route, reader)
	request.RemoteAddr = "127.0.0.1:50900"
	request.Header.Set("X-Provisioning-Token", fixture.provisioning)
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response
}

func (fixture *stage9GCatalog) call(t *testing.T, terminal stage9GTerminal, method, route string, body any, status int) []byte {
	t.Helper()
	response := fixture.send(terminal, method, route, body, nil)
	if response.Code != status {
		t.Fatalf("%s %s: esperado %d, recibido %d: %s", method, route, status, response.Code, response.Body.String())
	}
	return response.Body.Bytes()
}

func stage9GDecode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("respuesta inválida: %v: %s", err, body)
	}
	return value
}

func (fixture *stage9GCatalog) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var value int
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func newStage9GCatalog(t *testing.T) *stage9GCatalog {
	t.Helper()
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("No se pudo abrir el pool temporal")
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	provisioning := administration.NewToken()
	filename := filepath.Join(t.TempDir(), "provisioning")
	if err := os.WriteFile(filename, []byte(provisioning), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := &stage9GCatalog{ctx: ctx, pool: pool, provisioning: provisioning,
		service: &administration.Service{Pool: pool, ProvisioningFile: filename}}
	setup := stage9GDecode[map[string]any](t, fixture.call(t, stage9GTerminal{}, "POST", "setup", map[string]string{
		"name": "Catálogo 9G", "timezone": "America/Monterrey", "currency": "MXN", "theme": "light", "terminal_name": "Caja 9G",
	}, 200))
	fixture.admin = stage9GTerminal{deviceID: setup["device_id"].(string), deviceToken: setup["device_token"].(string)}
	login := stage9GDecode[map[string]any](t, fixture.call(t, fixture.admin, "POST", "auth/login",
		map[string]string{"username": "admin", "password": "admin"}, 200))
	fixture.admin.sessionToken = login["session_token"].(string)
	fixture.cashier = fixture.user(t, "cajero_9g", []string{"catalog.read", "catalog.prices.read", "events.read"})
	fixture.seed(t)
	return fixture
}

// user crea un usuario con un rol propio, una terminal enrolada y su sesión.
func (fixture *stage9GCatalog) user(t *testing.T, username string, permissions []string) stage9GTerminal {
	t.Helper()
	role := stage9GDecode[map[string]any](t, fixture.call(t, fixture.admin, "POST", "roles",
		map[string]any{"name": "Rol " + username, "permissions": permissions}, 200))
	password := "Clave temporal de " + username
	fixture.call(t, fixture.admin, "POST", "users", map[string]any{
		"username": username, "display_name": "Prueba " + username, "role_id": role["id"], "active": true, "password": password,
	}, 200)
	invitation := stage9GDecode[map[string]any](t, fixture.call(t, fixture.admin, "POST", "devices/invitations", nil, 200))
	device := stage9GDecode[map[string]any](t, fixture.call(t, stage9GTerminal{}, "POST", "devices/enroll",
		map[string]any{"code": invitation["code"], "name": "Terminal " + username}, 200))
	terminal := stage9GTerminal{deviceID: device["device_id"].(string), deviceToken: device["device_token"].(string)}
	login := stage9GDecode[map[string]any](t, fixture.call(t, terminal, "POST", "auth/login",
		map[string]string{"username": username, "password": password}, 200))
	terminal.sessionToken = login["session_token"].(string)
	return terminal
}

// seed crea una categoría, papas (que también son complemento), un plato con un grupo de guarniciones que ofrece las
// papas y una hamburguesa con una presentación.
func (fixture *stage9GCatalog) seed(t *testing.T) {
	t.Helper()
	fixture.category = stage9GDecode[catalog.Category](t, fixture.call(t, fixture.admin, "POST", "catalog/categories",
		catalog.Category{Metadata: catalog.Metadata{Name: "Comida 9G", Active: true}}, 200))
	fixture.fries = stage9GDecode[catalog.Product](t, fixture.call(t, fixture.admin, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Papas 9G", Active: true}, CategoryID: fixture.category.ID, Code: "PAPAS-9G", Kind: "product",
		BasePrice: "35",
	}, 200))
	complement := stage9GDecode[catalog.Complement](t, fixture.call(t, fixture.admin, "POST", "catalog/complements",
		catalog.Complement{Metadata: catalog.Metadata{Name: "Papas como guarnición", Active: true}, ProductID: &fixture.fries.ID}, 200))
	fixture.group = stage9GDecode[catalog.OptionGroup](t, fixture.call(t, fixture.admin, "POST", "catalog/groups", catalog.OptionGroup{
		Metadata: catalog.Metadata{Name: "Guarnición 9G", Active: true}, PricingMode: "value", MinQuantity: "0",
		Options: []catalog.GroupOption{{ComplementID: complement.ID, ReferenceValue: "10", ExtraPrice: "0", Active: true}},
	}, 200))
	fixture.plate = stage9GDecode[catalog.Product](t, fixture.call(t, fixture.admin, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Plato 9G", Active: true}, CategoryID: fixture.category.ID, Code: "PLATO-9G", Kind: "product",
		BasePrice: "90", Groups: []catalog.ProductGroup{{GroupID: fixture.group.ID}},
	}, 200))
	fixture.burger = stage9GDecode[catalog.Product](t, fixture.call(t, fixture.admin, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Hamburguesa 9G", Active: true}, CategoryID: fixture.category.ID, Code: "HAMB-9G", Kind: "product",
		BasePrice: "120", Variants: []catalog.Variant{{Name: "Doble", Code: "HAMB-9G-D", Price: "160", Active: true}},
	}, 200))
}

func (fixture *stage9GCatalog) preview(t *testing.T, terminal stage9GTerminal, line catalog.LineRequest, status int) []byte {
	t.Helper()
	return fixture.call(t, terminal, "POST", "catalog/preview", map[string]any{"lines": []catalog.LineRequest{line}}, status)
}

func TestStage9GCatalog(t *testing.T) {
	fixture := newStage9GCatalog(t)
	t.Run("Agotado: permiso, idempotencia, fin del día, captura rechazada y disponible otra vez", func(t *testing.T) {
		testStage9GSoldOut(t, fixture)
	})
	t.Run("Agotado hasta nuevo aviso, complemento agotado y reinicio automático", func(t *testing.T) {
		testStage9GSoldOutReset(t, fixture)
	})
	t.Run("catálogo descargable completo y por cambios", func(t *testing.T) {
		testStage9GSync(t, fixture)
	})
	t.Run("imagen binaria con ETag", func(t *testing.T) {
		testStage9GImageContent(t, fixture)
	})
	t.Run("revisión: época, archivados con lista o IVA, combos y complementos con un componente agotado", func(t *testing.T) {
		testStage9GSyncReview(t, fixture)
	})
}

func testStage9GSoldOut(t *testing.T, fixture *stage9GCatalog) {
	route := "catalog/products/" + fixture.burger.ID + "/sold-out"
	key := stage9GID()
	body := map[string]any{"operation_id": key, "sold_out": true}
	// Sin el permiso propio (catalog.availability.manage tampoco basta).
	fixture.call(t, fixture.cashier, "POST", route, body, 403)
	fixture.call(t, fixture.admin, "POST", route, map[string]any{"sold_out": true}, 400)
	eventsBefore := fixture.count(t, "SELECT count(*) FROM app.realtime_events WHERE scope='catalog' AND kind='catalog.product.sold_out'")
	// Reintentos simultáneos con la misma clave: un solo efecto y la misma respuesta.
	responses := make([]string, 4)
	var wait sync.WaitGroup
	for index := range responses {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response := fixture.send(fixture.admin, "POST", route, body, nil)
			responses[index] = fmt.Sprintf("%d %s", response.Code, response.Body.String())
		}()
	}
	wait.Wait()
	for _, response := range responses[1:] {
		if response != responses[0] || !strings.HasPrefix(response, "200 ") {
			t.Fatalf("los reintentos simultáneos no devolvieron la misma respuesta: %q", responses)
		}
	}
	state := stage9GDecode[catalog.SoldOutState](t, []byte(strings.TrimPrefix(responses[0], "200 ")))
	if !state.SoldOut || state.Indefinite || state.SoldOutUntil == nil || state.CatalogVersion <= 0 {
		t.Fatalf("estado de agotado: %+v", state)
	}
	// El fin es el inicio del día comercial siguiente («Hora de inicio del día»).
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := businessday.Load(fixture.ctx, tx)
	_ = tx.Rollback(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if expected := configuration.DayOf(*state.SoldOutAt).Deadline; !state.SoldOutUntil.Equal(expected) {
		t.Fatalf("el agotado vence %v y no al inicio del día siguiente %v", state.SoldOutUntil, expected)
	}
	if audits := fixture.count(t, "SELECT count(*) FROM app.audit_events WHERE action='catalog.product.sold_out' AND entity_id=$1",
		fixture.burger.ID); audits != 1 {
		t.Fatalf("se auditaron %d marcas en lugar de una", audits)
	}
	if published := fixture.count(t, "SELECT count(*) FROM app.realtime_events WHERE scope='catalog' AND kind='catalog.product.sold_out'"); published != eventsBefore+1 {
		t.Fatalf("se publicaron %d eventos de agotado en lugar de uno", published-eventsBefore)
	}
	// Reintento después de una respuesta perdida: la misma respuesta, sin repetir.
	if again := fixture.call(t, fixture.admin, "POST", route, body, 200); string(again) != strings.TrimPrefix(responses[0], "200 ") {
		t.Fatalf("el reintento no devolvió la respuesta guardada: %s", again)
	}
	// La misma clave con otro contenido se rechaza.
	conflict := fixture.call(t, fixture.admin, "POST", route, map[string]any{"operation_id": key, "sold_out": true, "indefinite": true}, 409)
	if !bytes.Contains(conflict, []byte("IDEMPOTENCY_CONFLICT")) {
		t.Fatalf("conflicto: %s", conflict)
	}
	// Un producto agotado no se puede capturar (tampoco su presentación) y la captura lo ve marcado.
	rejected := fixture.preview(t, fixture.admin, catalog.LineRequest{ProductID: fixture.burger.ID, Quantity: 1}, 409)
	if !bytes.Contains(rejected, []byte("PRODUCT_SOLD_OUT")) || !bytes.Contains(rejected, []byte("Hamburguesa 9G está agotado")) {
		t.Fatalf("rechazo del producto agotado: %s", rejected)
	}
	variant := fixture.burger.Variants[0].ID
	fixture.preview(t, fixture.admin, catalog.LineRequest{ProductID: fixture.burger.ID, VariantID: &variant, Quantity: 1}, 409)
	listing := stage9GDecode[struct {
		Items []map[string]any `json:"items"`
	}](t, fixture.call(t, fixture.admin, "GET", "catalog/products?search=HAMB-9G", nil, 200))
	if len(listing.Items) != 1 || listing.Items[0]["sold_out"] != true || listing.Items[0]["available_now"] != false ||
		listing.Items[0]["availability_reason"] != "Agotado." || listing.Items[0]["sold_out_indefinite"] != false {
		t.Fatalf("la captura no ve el producto agotado: %v", listing.Items)
	}
	// Marcarlo no cambia la revisión del producto (no interrumpe a quien lo edita en Catálogo).
	current := stage9GDecode[catalog.Product](t, fixture.call(t, fixture.admin, "GET", "catalog/products/"+fixture.burger.ID, nil, 200))
	if current.Revision != fixture.burger.Revision {
		t.Fatalf("la revisión cambió de %d a %d", fixture.burger.Revision, current.Revision)
	}
	// «Disponible otra vez» con otra clave.
	available := stage9GDecode[catalog.SoldOutState](t, fixture.call(t, fixture.admin, "POST", route,
		map[string]any{"operation_id": stage9GID(), "sold_out": false}, 200))
	if available.SoldOut || available.SoldOutUntil != nil || available.CatalogVersion <= state.CatalogVersion {
		t.Fatalf("disponible otra vez: %+v", available)
	}
	fixture.preview(t, fixture.admin, catalog.LineRequest{ProductID: fixture.burger.ID, Quantity: 1}, 200)
	if audits := fixture.count(t, "SELECT count(*) FROM app.audit_events WHERE action='catalog.product.available_again' AND entity_id=$1",
		fixture.burger.ID); audits != 1 {
		t.Fatalf("«Disponible otra vez» se auditó %d veces", audits)
	}
	// «Disponible otra vez» de un producto que no estaba agotado no audita ni avisa nada.
	fixture.call(t, fixture.admin, "POST", route, map[string]any{"operation_id": stage9GID(), "sold_out": false}, 200)
	if audits := fixture.count(t, "SELECT count(*) FROM app.audit_events WHERE action='catalog.product.available_again'"); audits != 1 {
		t.Fatalf("se auditó un cambio que no ocurrió: %d", audits)
	}
}

func testStage9GSoldOutReset(t *testing.T, fixture *stage9GCatalog) {
	friesRoute := "catalog/products/" + fixture.fries.ID + "/sold-out"
	indefinite := stage9GDecode[catalog.SoldOutState](t, fixture.call(t, fixture.admin, "POST", friesRoute,
		map[string]any{"operation_id": stage9GID(), "sold_out": true, "indefinite": true}, 200))
	if !indefinite.SoldOut || !indefinite.Indefinite || indefinite.SoldOutUntil != nil {
		t.Fatalf("agotado hasta nuevo aviso: %+v", indefinite)
	}
	// Las papas agotadas también se rechazan como complemento del plato.
	option := fixture.group.Options[0].ID
	line := catalog.LineRequest{ProductID: fixture.plate.ID, Quantity: 1, Customization: catalog.Customization{
		Groups: []catalog.GroupSelection{{GroupID: fixture.group.ID, Options: []catalog.IncludedOption{{OptionID: option, Quantity: "1"}}}},
	}}
	rejected := fixture.preview(t, fixture.admin, line, 409)
	if !bytes.Contains(rejected, []byte("PRODUCT_SOLD_OUT")) || !bytes.Contains(rejected, []byte("complemento")) {
		t.Fatalf("complemento agotado: %s", rejected)
	}
	fixture.preview(t, fixture.admin, catalog.LineRequest{ProductID: fixture.plate.ID, Quantity: 1}, 200)
	// La hamburguesa agotada hoy: su marca vence; la lectura ya la ignora antes de que pase el trabajador.
	burgerRoute := "catalog/products/" + fixture.burger.ID + "/sold-out"
	fixture.call(t, fixture.admin, "POST", burgerRoute, map[string]any{"operation_id": stage9GID(), "sold_out": true}, 200)
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.catalog_products SET sold_out_at=now()-interval '2 days',
        sold_out_until=now()-interval '1 hour' WHERE id=$1`, fixture.burger.ID); err != nil {
		t.Fatal(err)
	}
	fixture.preview(t, fixture.admin, catalog.LineRequest{ProductID: fixture.burger.ID, Quantity: 1}, 200)
	versionBefore := fixture.count(t, "SELECT coalesce(max(version),0) FROM app.catalog_change_log")
	reset, err := catalog.ResetExpiredSoldOut(fixture.ctx, fixture.pool, time.Now())
	if err != nil || reset != 1 {
		t.Fatalf("el reinicio limpió %d productos: %v", reset, err)
	}
	if again, err := catalog.ResetExpiredSoldOut(fixture.ctx, fixture.pool, time.Now()); err != nil || again != 0 {
		t.Fatalf("el reinicio no es repetible: %d %v", again, err)
	}
	if marked := fixture.count(t, "SELECT count(*) FROM app.catalog_products WHERE sold_out_at IS NOT NULL"); marked != 1 {
		t.Fatalf("quedaron %d productos agotados (solo las papas, hasta nuevo aviso)", marked)
	}
	if audits := fixture.count(t, `SELECT count(*) FROM app.audit_events WHERE action='catalog.product.sold_out_reset'
        AND entity_id=$1 AND actor_kind='process'`, fixture.burger.ID); audits != 1 {
		t.Fatalf("el reinicio se auditó %d veces", audits)
	}
	if events := fixture.count(t, `SELECT count(*) FROM app.realtime_events WHERE scope='catalog' AND kind='catalog.product.available'
        AND aggregate_id=$1`, fixture.burger.ID); events < 2 {
		rows, _ := fixture.pool.Query(fixture.ctx, "SELECT kind||' '||coalesce(aggregate_id::text,'') FROM app.realtime_events WHERE scope='catalog' ORDER BY sequence")
		listed := []string{}
		for rows.Next() {
			var entry string
			_ = rows.Scan(&entry)
			listed = append(listed, entry)
		}
		rows.Close()
		t.Fatalf("el reinicio no avisó a las terminales: %d %v", events, listed)
	}
	if after := fixture.count(t, "SELECT coalesce(max(version),0) FROM app.catalog_change_log"); after <= versionBefore {
		t.Fatal("el reinicio no subió la versión del catálogo")
	}
	fixture.call(t, fixture.admin, "POST", friesRoute, map[string]any{"operation_id": stage9GID(), "sold_out": false}, 200)
}

type stage9GSync struct {
	CatalogVersion int64               `json:"catalog_version"`
	CatalogEpoch   string              `json:"catalog_epoch"`
	PriceList      map[string]any      `json:"price_list"`
	Full           bool                `json:"full"`
	Reset          bool                `json:"reset"`
	TaxRate        int                 `json:"tax_rate"`
	Products       []map[string]any    `json:"products"`
	Categories     []map[string]any    `json:"categories"`
	Groups         []map[string]any    `json:"groups"`
	Images         []map[string]any    `json:"images"`
	Deleted        []map[string]string `json:"deleted"`
}

func productNames(products []map[string]any) []string {
	names := []string{}
	for _, product := range products {
		names = append(names, fmt.Sprint(product["name"]))
	}
	return names
}

func testStage9GSync(t *testing.T, fixture *stage9GCatalog) {
	// Sin catalog.prices.read no se descarga (los precios son parte del catálogo descargable).
	reader := fixture.user(t, "lector_9g", []string{"catalog.read"})
	fixture.call(t, reader, "GET", "catalog/sync", nil, 403)
	full := stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET", "catalog/sync", nil, 200))
	if !full.Full || full.CatalogVersion <= 0 || len(full.Products) != 3 || len(full.Categories) != 1 || full.TaxRate != 16 {
		t.Fatalf("descarga completa: versión %d, productos %v", full.CatalogVersion, productNames(full.Products))
	}
	var burger map[string]any
	for _, product := range full.Products {
		if product["id"] == fixture.burger.ID {
			burger = product
		}
	}
	variants, _ := burger["variants"].([]any)
	if burger["price"] != "120.00" || burger["effective_tax_rate"] != float64(16) || burger["available_now"] != true ||
		burger["sold_out"] != false || len(variants) != 1 || variants[0].(map[string]any)["effective_price"] != "160.00" {
		t.Fatalf("producto de la descarga: %v", burger)
	}
	// Sin cambios: nada que descargar.
	empty := stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET",
		fmt.Sprintf("catalog/sync?since=%d&epoch=%s", full.CatalogVersion, full.CatalogEpoch), nil, 200))
	if empty.Full || empty.CatalogVersion != full.CatalogVersion || len(empty.Products) != 0 || len(empty.Deleted) != 0 {
		t.Fatalf("descarga sin cambios: %+v", empty)
	}
	// Un cambio de precio llega solo con su producto y sube la versión.
	burgerDocument := stage9GDecode[catalog.Product](t, fixture.call(t, fixture.admin, "GET", "catalog/products/"+fixture.burger.ID, nil, 200))
	burgerDocument.BasePrice = "130"
	fixture.call(t, fixture.admin, "PATCH", "catalog/products/"+fixture.burger.ID, burgerDocument, 200)
	priced := stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET",
		fmt.Sprintf("catalog/sync?since=%d&epoch=%s", full.CatalogVersion, full.CatalogEpoch), nil, 200))
	if priced.Full || priced.CatalogVersion <= full.CatalogVersion || len(priced.Products) != 1 || priced.Products[0]["price"] != "130.00" ||
		len(priced.Categories) != 0 {
		t.Fatalf("descarga del cambio de precio: versión %d, productos %v", priced.CatalogVersion, priced.Products)
	}
	// «Agotado» también cambia la versión y llega en los cambios.
	fixture.call(t, fixture.admin, "POST", "catalog/products/"+fixture.plate.ID+"/sold-out",
		map[string]any{"operation_id": stage9GID(), "sold_out": true}, 200)
	soldOut := stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET",
		fmt.Sprintf("catalog/sync?since=%d&epoch=%s", priced.CatalogVersion, priced.CatalogEpoch), nil, 200))
	if len(soldOut.Products) != 1 || soldOut.Products[0]["sold_out"] != true || soldOut.Products[0]["available_now"] != false {
		t.Fatalf("descarga del agotado: %v", soldOut.Products)
	}
	// Un archivado llega en los cambios con archived; un borrado, en deleted; y ya no están en la descarga completa.
	plateDocument := stage9GDecode[catalog.Product](t, fixture.call(t, fixture.admin, "GET", "catalog/products/"+fixture.plate.ID, nil, 200))
	fixture.call(t, fixture.admin, "PATCH", "catalog/products/"+fixture.plate.ID+"/state",
		map[string]any{"revision": plateDocument.Revision, "active": false, "archived": true}, 200)
	temporary := stage9GDecode[catalog.Product](t, fixture.call(t, fixture.admin, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Temporal 9G", Active: true}, CategoryID: fixture.category.ID, Code: "TEMP-9G", Kind: "product",
		BasePrice: "10",
	}, 200))
	if _, err := fixture.pool.Exec(fixture.ctx, "DELETE FROM app.catalog_products WHERE id=$1", temporary.ID); err != nil {
		t.Fatal(err)
	}
	removals := stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET",
		fmt.Sprintf("catalog/sync?since=%d&epoch=%s", soldOut.CatalogVersion, soldOut.CatalogEpoch), nil, 200))
	if len(removals.Products) != 1 || removals.Products[0]["archived"] != true || len(removals.Deleted) != 1 ||
		removals.Deleted[0]["id"] != temporary.ID || removals.Deleted[0]["entity"] != "product" {
		t.Fatalf("bajas: productos %v, borrados %v", productNames(removals.Products), removals.Deleted)
	}
	again := stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET", "catalog/sync", nil, 200))
	if len(again.Products) != 2 {
		t.Fatalf("la descarga completa incluye archivados o borrados: %v", productNames(again.Products))
	}
	// Un cambio de categoría trae sus productos; la tasa general trae todos.
	category := stage9GDecode[catalog.Category](t, fixture.call(t, fixture.admin, "GET", "catalog/categories/"+fixture.category.ID, nil, 200))
	category.Description = "Cambio 9G"
	fixture.call(t, fixture.admin, "PATCH", "catalog/categories/"+fixture.category.ID, category, 200)
	byCategory := stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET",
		fmt.Sprintf("catalog/sync?since=%d&epoch=%s", again.CatalogVersion, again.CatalogEpoch), nil, 200))
	if len(byCategory.Categories) != 1 || len(byCategory.Products) != 3 {
		t.Fatalf("cambio de categoría: categorías %d, productos %v", len(byCategory.Categories), productNames(byCategory.Products))
	}
	// Una versión mayor que la vigente (otro servidor o base restaurada) entrega todo; una inválida se rechaza.
	future := stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET",
		fmt.Sprintf("catalog/sync?since=%d&epoch=%s", byCategory.CatalogVersion+100, byCategory.CatalogEpoch), nil, 200))
	if !future.Full || !future.Reset || len(future.Products) != 2 {
		t.Fatalf("versión mayor que la vigente: %+v", future)
	}
	fixture.call(t, fixture.cashier, "GET", "catalog/sync?since=-1", nil, 400)
	fixture.call(t, fixture.cashier, "GET", "catalog/sync?since=x", nil, 400)
	// Las escrituras del catálogo publican su evento en tiempo real (ámbito catalog).
	if published := fixture.count(t, "SELECT count(*) FROM app.realtime_events WHERE scope='catalog' AND kind='catalog.changed'"); published == 0 {
		t.Fatal("las escrituras del catálogo no publican eventos")
	}
}

func testStage9GImageContent(t *testing.T, fixture *stage9GCatalog) {
	picture := image.NewRGBA(image.Rect(0, 0, 4, 3))
	picture.Set(1, 1, color.RGBA{R: 200, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	uploaded := stage9GDecode[map[string]any](t, fixture.call(t, fixture.admin, "POST", "catalog/images",
		map[string]string{"content_base64": base64.StdEncoding.EncodeToString(encoded.Bytes())}, 200))
	id := uploaded["id"].(string)
	response := fixture.send(fixture.cashier, "GET", "catalog/images/"+id+"/content", nil, nil)
	etag := response.Header().Get("ETag")
	if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), encoded.Bytes()) || response.Header().Get("Content-Type") != "image/png" ||
		!strings.HasPrefix(etag, `"`) || !strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("imagen binaria: %d %q %q", response.Code, response.Header().Get("Content-Type"), etag)
	}
	cached := fixture.send(fixture.cashier, "GET", "catalog/images/"+id+"/content", nil, map[string]string{"If-None-Match": etag})
	if cached.Code != 304 || cached.Body.Len() != 0 {
		t.Fatalf("la imagen en caché no respondió 304: %d", cached.Code)
	}
	stale := fixture.send(fixture.cashier, "GET", "catalog/images/"+id+"/content", nil, map[string]string{"If-None-Match": `"otra"`})
	if stale.Code != 200 {
		t.Fatalf("otra etiqueta no descarga la imagen: %d", stale.Code)
	}
	if missing := fixture.send(fixture.cashier, "GET", "catalog/images/"+stage9GID()+"/content", nil, nil); missing.Code != 404 {
		t.Fatalf("imagen inexistente: %d", missing.Code)
	}
	// El JSON base64 que usa el escritorio sigue igual.
	legacy := stage9GDecode[map[string]any](t, fixture.call(t, fixture.cashier, "GET", "catalog/images/"+id, nil, 200))
	if !strings.HasPrefix(fmt.Sprint(legacy["data_uri"]), "data:image/png;base64,") {
		t.Fatalf("imagen en JSON: %v", legacy)
	}
	// La descarga del catálogo lleva solo la referencia de la imagen de un producto.
	burger := stage9GDecode[catalog.Product](t, fixture.call(t, fixture.admin, "GET", "catalog/products/"+fixture.burger.ID, nil, 200))
	burger.ImageID = &id
	fixture.call(t, fixture.admin, "PATCH", "catalog/products/"+fixture.burger.ID, burger, 200)
	full := stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET", "catalog/sync", nil, 200))
	if len(full.Images) != 1 || full.Images[0]["id"] != id || full.Images[0]["checksum"] == "" || `"`+fmt.Sprint(full.Images[0]["checksum"])+`"` != etag {
		t.Fatalf("referencias de imágenes: %v", full.Images)
	}
}

// syncSince descarga los cambios desde una descarga anterior, con su versión y su época.
func (fixture *stage9GCatalog) syncSince(t *testing.T, previous stage9GSync) stage9GSync {
	t.Helper()
	return stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET",
		fmt.Sprintf("catalog/sync?since=%d&epoch=%s", previous.CatalogVersion, previous.CatalogEpoch), nil, 200))
}

func findSynced(t *testing.T, records []map[string]any, id string) map[string]any {
	t.Helper()
	for _, record := range records {
		if record["id"] == id {
			return record
		}
	}
	return nil
}

// Revisión independiente de la fase D: hallazgo 2 (época), hallazgo 5 (un producto agotado que es componente de un
// combo o complemento de un grupo trae al combo y al grupo con su disponibilidad) y hallazgo 1 (un producto archivado
// después de la versión X llega aunque también cambie la lista predeterminada o la tasa general de IVA).
func testStage9GSyncReview(t *testing.T, fixture *stage9GCatalog) {
	combo := stage9GDecode[catalog.Product](t, fixture.call(t, fixture.admin, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Combo 9G", Active: true}, CategoryID: fixture.category.ID, Code: "COMBO-9G", Kind: "combo",
		BasePrice: "150",
		Slots: []catalog.ComboSlot{{
			Name: "Papas del combo", MinQuantity: 1, MaxQuantity: 1, IncludedQuantity: 1, DefaultQuantity: 1,
			DefaultProductID: fixture.fries.ID,
			Options:          []catalog.ComboOption{{ProductID: fixture.fries.ID, SubstitutionPrice: "0"}},
		}},
	}, 200))
	base := stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET", "catalog/sync", nil, 200))
	if len(base.CatalogEpoch) != 36 {
		t.Fatalf("época del catálogo: %q", base.CatalogEpoch)
	}
	syncedCombo := findSynced(t, base.Products, combo.ID)
	if syncedCombo == nil || syncedCombo["available_now"] != true {
		t.Fatalf("combo en la descarga completa: %v", syncedCombo)
	}

	// Hallazgo 2: la descarga de cambios exige la época; con otra época (otra base o una restaurada) se entrega todo.
	fixture.call(t, fixture.cashier, "GET", fmt.Sprintf("catalog/sync?since=%d", base.CatalogVersion), nil, 400)
	fixture.call(t, fixture.cashier, "GET", fmt.Sprintf("catalog/sync?since=%d&epoch=otra", base.CatalogVersion), nil, 400)
	foreign := stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET",
		fmt.Sprintf("catalog/sync?since=%d&epoch=%s", base.CatalogVersion, stage9GID()), nil, 200))
	if !foreign.Full || !foreign.Reset || len(foreign.Products) != len(base.Products) || foreign.CatalogEpoch != base.CatalogEpoch {
		t.Fatalf("otra época: full %v reset %v productos %d", foreign.Full, foreign.Reset, len(foreign.Products))
	}
	if same := fixture.syncSince(t, base); same.Full || same.Reset || len(same.Products) != 0 {
		t.Fatalf("misma época sin cambios: %+v", same)
	}

	// Hallazgo 5: las papas agotadas llegan con el combo que las ofrece (no disponible: su ranura obligatoria no
	// tiene otra opción) y con el grupo cuyo complemento son las papas.
	fixture.call(t, fixture.admin, "POST", "catalog/products/"+fixture.fries.ID+"/sold-out",
		map[string]any{"operation_id": stage9GID(), "sold_out": true}, 200)
	soldOut := fixture.syncSince(t, base)
	syncedCombo = findSynced(t, soldOut.Products, combo.ID)
	if findSynced(t, soldOut.Products, fixture.fries.ID) == nil || syncedCombo == nil || len(soldOut.Products) != 2 {
		t.Fatalf("el agotado de un componente no trae al combo: %v", productNames(soldOut.Products))
	}
	slots, _ := syncedCombo["slots"].([]any)
	option := slots[0].(map[string]any)["options"].([]any)[0].(map[string]any)
	if syncedCombo["available_now"] != false || !strings.Contains(fmt.Sprint(syncedCombo["availability_reason"]), "Papas del combo") ||
		option["sold_out"] != true || option["available_now"] != false {
		t.Fatalf("combo con su componente agotado: %v", syncedCombo)
	}
	group := findSynced(t, soldOut.Groups, fixture.group.ID)
	if group == nil {
		t.Fatalf("el agotado de un complemento no trae a su grupo: %v", soldOut.Groups)
	}
	groupOption := group["options"].([]any)[0].(map[string]any)
	if groupOption["sold_out"] != true || groupOption["available_now"] != false {
		t.Fatalf("opción del grupo con el complemento agotado: %v", groupOption)
	}
	if fullCombo := findSynced(t, stage9GDecode[stage9GSync](t, fixture.call(t, fixture.cashier, "GET", "catalog/sync", nil, 200)).Products,
		combo.ID); fullCombo == nil || fullCombo["available_now"] != false {
		t.Fatalf("la descarga completa no marca el combo: %v", fullCombo)
	}
	fixture.call(t, fixture.admin, "POST", "catalog/products/"+fixture.fries.ID+"/sold-out",
		map[string]any{"operation_id": stage9GID(), "sold_out": false}, 200)
	available := fixture.syncSince(t, soldOut)
	if syncedCombo = findSynced(t, available.Products, combo.ID); syncedCombo == nil || syncedCombo["available_now"] != true {
		t.Fatalf("el combo no vuelve a estar disponible: %v", syncedCombo)
	}

	// Hallazgo 1: se archiva la hamburguesa y después se guarda la lista predeterminada; la descarga de cambios trae
	// todos los productos vigentes y también la hamburguesa archivada (antes se perdía).
	burger := stage9GDecode[catalog.Product](t, fixture.call(t, fixture.admin, "GET", "catalog/products/"+fixture.burger.ID, nil, 200))
	fixture.call(t, fixture.admin, "PATCH", "catalog/products/"+fixture.burger.ID+"/state",
		map[string]any{"revision": burger.Revision, "active": false, "archived": true}, 200)
	listID := fmt.Sprint(available.PriceList["id"])
	list := stage9GDecode[catalog.PriceList](t, fixture.call(t, fixture.admin, "GET", "catalog/price-lists/"+listID, nil, 200))
	fixture.call(t, fixture.admin, "PATCH", "catalog/price-lists/"+listID, list, 200)
	priced := fixture.syncSince(t, available)
	if archived := findSynced(t, priced.Products, fixture.burger.ID); archived == nil || archived["archived"] != true || priced.Full {
		t.Fatalf("lista guardada después de archivar: %v", productNames(priced.Products))
	}
	if findSynced(t, priced.Products, combo.ID) == nil || findSynced(t, priced.Products, fixture.fries.ID) == nil {
		t.Fatalf("con la lista cambiada deben llegar todos los productos vigentes: %v", productNames(priced.Products))
	}
	// Lo mismo con la tasa general de IVA, en el otro orden: primero el IVA y después el archivado.
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.payment_settings SET tax_rate=8"); err != nil {
		t.Fatal(err)
	}
	comboDocument := stage9GDecode[catalog.Product](t, fixture.call(t, fixture.admin, "GET", "catalog/products/"+combo.ID, nil, 200))
	fixture.call(t, fixture.admin, "PATCH", "catalog/products/"+combo.ID+"/state",
		map[string]any{"revision": comboDocument.Revision, "active": false, "archived": true}, 200)
	taxed := fixture.syncSince(t, priced)
	archivedCombo := findSynced(t, taxed.Products, combo.ID)
	if taxed.TaxRate != 8 || archivedCombo == nil || archivedCombo["archived"] != true ||
		findSynced(t, taxed.Products, fixture.fries.ID)["effective_tax_rate"] != float64(8) {
		t.Fatalf("IVA cambiado y combo archivado: %d %v", taxed.TaxRate, productNames(taxed.Products))
	}
	if findSynced(t, taxed.Products, fixture.burger.ID) != nil {
		t.Fatal("un archivado que no cambió después de la versión pedida no debe volver a llegar")
	}
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.payment_settings SET tax_rate=16"); err != nil {
		t.Fatal(err)
	}
}
