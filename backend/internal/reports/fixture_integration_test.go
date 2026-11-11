//go:build integration

package reports_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Fixture propio de reportes (justificación: una base temporal exclusiva del paquete permite
// comprobar totales exactos sin los datos de la aceptación encadenada de internal/operations, y
// iterar en segundos). Usa la API real (administration.Service) para crear catálogo, cuentas,
// cobros, descuentos, devoluciones y divisiones; los reportes se consultan por HTTP como la
// terminal.

type terminal struct {
	deviceID     string
	deviceToken  string
	sessionToken string
}

type reportsFixture struct {
	ctx          context.Context
	pool         *pgxpool.Pool
	service      *administration.Service
	provisioning string
	admin        terminal
	methods      map[string]string
	registerID   string
	category     catalog.Category
	drinks       catalog.Category
	burger       catalog.Product
	soda         catalog.Product
	salad        catalog.Product
	fries        catalog.Product
	combo        catalog.Product
	steak        catalog.Product
	group        catalog.OptionGroup
	area         operations.Area
	terrace      operations.Area
	table        operations.Table
	secondTable  operations.Table
	shiftID      string
}

func randomID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("La respuesta no cumple el contrato: %v: %s", err, body)
	}
	return result
}

func openPool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("No se pudo abrir el pool temporal")
	}
	t.Cleanup(pool.Close)
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return ctx, pool
}

func newReportsFixture(t *testing.T) *reportsFixture {
	t.Helper()
	ctx, pool := openPool(t)
	token := administration.NewToken()
	filename := filepath.Join(t.TempDir(), "provisioning")
	if err := os.WriteFile(filename, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := &reportsFixture{ctx: ctx, pool: pool, provisioning: token, methods: map[string]string{},
		service: &administration.Service{Pool: pool, ProvisioningFile: filename}}
	setup := decode[map[string]any](t, fixture.call(t, terminal{}, "POST", "setup", map[string]string{
		"name": "Restaurante de reportes", "timezone": "America/Monterrey", "currency": "MXN", "theme": "light",
		"terminal_name": "Caja principal",
	}, 200))
	fixture.admin.deviceID = setup["device_id"].(string)
	fixture.admin.deviceToken = setup["device_token"].(string)
	login := decode[map[string]any](t, fixture.call(t, fixture.admin, "POST", "auth/login",
		map[string]string{"username": "admin", "password": "admin"}, 200))
	fixture.admin.sessionToken = login["session_token"].(string)
	for _, method := range decode[[]payments.Method](t, fixture.get(t, fixture.admin, "payments/methods", 200)) {
		if method.Code != nil {
			fixture.methods[*method.Code] = method.ID
		}
	}
	if err := pool.QueryRow(ctx, "SELECT register_id FROM app.devices WHERE id=$1", fixture.admin.deviceID).Scan(&fixture.registerID); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// request ejecuta una petición HTTP real contra el servicio y devuelve código y cuerpo.
func (fixture *reportsFixture) request(terminal terminal, method, route string, body any) (int, []byte) {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, "/api/v1/"+route, reader)
	request.RemoteAddr = "127.0.0.1:52000"
	request.Header.Set("X-Provisioning-Token", fixture.provisioning)
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response.Code, append([]byte{}, response.Body.Bytes()...)
}

func (fixture *reportsFixture) call(t *testing.T, terminal terminal, method, route string, body any, status int) []byte {
	t.Helper()
	code, response := fixture.request(terminal, method, route, body)
	if code != status {
		t.Fatalf("%s %s: esperado %d recibido %d: %s", method, route, status, code, response)
	}
	return response
}

func (fixture *reportsFixture) get(t *testing.T, terminal terminal, route string, status int) []byte {
	t.Helper()
	return fixture.call(t, terminal, "GET", route, nil, status)
}

// mutate agrega un operation_id nuevo al cuerpo de una escritura de la administración.
func (fixture *reportsFixture) mutate(t *testing.T, method, route string, body map[string]any, status int) []byte {
	t.Helper()
	if body == nil {
		body = map[string]any{}
	}
	if identifier, _ := body["operation_id"].(string); identifier == "" {
		body["operation_id"] = randomID()
	}
	return fixture.call(t, fixture.admin, method, route, body, status)
}

func structToMap(value any) map[string]any {
	encoded, _ := json.Marshal(value)
	result := map[string]any{}
	_ = json.Unmarshal(encoded, &result)
	return result
}

func (fixture *reportsFixture) seedCatalog(t *testing.T) {
	t.Helper()
	create := func(route string, value any) []byte {
		return fixture.call(t, fixture.admin, "POST", route, value, 200)
	}
	fixture.category = decode[catalog.Category](t, create("catalog/categories", catalog.Category{Metadata: catalog.Metadata{Name: "Platillos", Active: true}}))
	fixture.drinks = decode[catalog.Category](t, create("catalog/categories", catalog.Category{Metadata: catalog.Metadata{Name: "Bebidas", Active: true}}))
	product := func(name, code string, category catalog.Category, price string, tax *int) catalog.Product {
		return decode[catalog.Product](t, create("catalog/products", catalog.Product{
			Metadata: catalog.Metadata{Name: name, Active: true}, CategoryID: category.ID, Code: code, Kind: "product",
			BasePrice: price, TaxRate: tax,
		}))
	}
	eight := 8
	fixture.burger = decode[catalog.Product](t, create("catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Hamburguesa", Active: true}, CategoryID: fixture.category.ID, Code: "HAMB", Kind: "product",
		BasePrice: "120", Variants: []catalog.Variant{{Name: "Doble", Code: "HAMB-D", Price: "150", Active: true}},
	}))
	fixture.soda = product("Refresco", "REFR", fixture.drinks, "25", nil)
	fixture.salad = product("Ensalada", "ENSA", fixture.category, "50", &eight)
	fixture.fries = product("Papas", "PAPA", fixture.category, "40", nil)
	fixture.combo = decode[catalog.Product](t, create("catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Combo hamburguesa", Active: true}, CategoryID: fixture.category.ID, Code: "COMBO", Kind: "combo",
		BasePrice: "90",
		Slots: []catalog.ComboSlot{{
			Name: "Papas del combo", MaxQuantity: 2, DefaultProductID: fixture.fries.ID,
			Options: []catalog.ComboOption{{ProductID: fixture.fries.ID, SubstitutionPrice: "0",
				Tiers: []catalog.QuantityTier{{From: 1, UnitPrice: "10"}, {From: 2, UnitPrice: "40"}}}},
		}},
	}))
	fixture.group = catalog.OptionGroup{Metadata: catalog.Metadata{Name: "Guarniciones", Active: true}, PricingMode: "value", MinQuantity: "0"}
	for index, name := range []string{"Arroz", "Frijoles"} {
		complement := decode[catalog.Complement](t, create("catalog/complements", catalog.Complement{Metadata: catalog.Metadata{Name: name, Active: true}}))
		fixture.group.Options = append(fixture.group.Options, catalog.GroupOption{
			ComplementID: complement.ID, ReferenceValue: []string{"20", "15"}[index], ExtraPrice: "0", Active: true,
		})
	}
	fixture.group = decode[catalog.OptionGroup](t, create("catalog/groups", fixture.group))
	byName := map[string]catalog.GroupOption{}
	for _, option := range fixture.group.Options {
		byName[option.Name] = option
	}
	fixture.group.Options = []catalog.GroupOption{byName["Arroz"], byName["Frijoles"]}
	fixture.steak = decode[catalog.Product](t, create("catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Filete", Active: true}, CategoryID: fixture.category.ID, Code: "FILE", Kind: "product",
		BasePrice: "150",
		Groups: []catalog.ProductGroup{{GroupID: fixture.group.ID,
			Included: []catalog.IncludedOption{{OptionID: fixture.group.Options[0].ID, Quantity: "1"}}}},
	}))
	fixture.area = decode[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", structToMap(operations.Area{Name: "Salón", Active: true}), 200))
	fixture.terrace = decode[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", structToMap(operations.Area{Name: "Terraza", Active: true}), 200))
	fixture.table = decode[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", structToMap(operations.Table{
		AreaID: fixture.area.ID, Name: "Mesa 1", Shape: "rectangle", X: 20, Y: 30, Width: 100, Height: 72, Seats: 4, Active: true,
	}), 200))
	fixture.secondTable = decode[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", structToMap(operations.Table{
		AreaID: fixture.terrace.ID, Name: "Mesa 2", Shape: "round", X: 140, Y: 30, Width: 90, Height: 90, Seats: 4, Active: true,
	}), 200))
}

func line(product catalog.Product, quantity int) catalog.LineRequest {
	return catalog.LineRequest{ProductID: product.ID, Quantity: quantity}
}

// account abre una cuenta y confirma sus partidas.
func (fixture *reportsFixture) account(t *testing.T, name, mode string, table *operations.Table, lines ...catalog.LineRequest) operations.Bill {
	t.Helper()
	body := map[string]any{"name": name, "mode": mode}
	if table != nil {
		body["table_id"] = table.ID
	}
	if mode == "takeaway" {
		body["origin"] = "Ventanilla"
	}
	bill := decode[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", body, 200))
	return fixture.capture(t, bill, lines...)
}

func (fixture *reportsFixture) capture(t *testing.T, bill operations.Bill, lines ...catalog.LineRequest) operations.Bill {
	t.Helper()
	bill = fixture.refresh(t, bill.Account.ID)
	draft := make([]operations.DraftItem, 0, len(lines))
	for _, item := range lines {
		draft = append(draft, operations.DraftItem{ID: randomID(), LineRequest: item})
	}
	route := "restaurant/accounts/" + bill.Account.ID
	bill = decode[operations.Bill](t, fixture.mutate(t, "PUT", route+"/draft", map[string]any{"revision": bill.Account.Revision, "items": draft}, 200))
	preview := decode[operations.QuoteResult](t, fixture.get(t, fixture.admin, route+"/preview", 200))
	return decode[operations.Bill](t, fixture.mutate(t, "POST", route+"/confirm", map[string]any{
		"revision": bill.Account.Revision, "expected_fingerprint": preview.Fingerprint,
	}, 200))
}

func (fixture *reportsFixture) refresh(t *testing.T, accountID string) operations.Bill {
	t.Helper()
	return decode[operations.Bill](t, fixture.get(t, fixture.admin, "restaurant/accounts/"+accountID, 200))
}

func (fixture *reportsFixture) finance(t *testing.T, accountID string) payments.Finance {
	t.Helper()
	return decode[payments.Finance](t, fixture.get(t, fixture.admin, "payments/accounts/"+accountID, 200))
}

// post envía una operación financiera sobre una cuenta.
func (fixture *reportsFixture) post(t *testing.T, accountID, action string, body map[string]any, status int) []byte {
	t.Helper()
	if _, found := body["operation_id"]; !found {
		body["operation_id"] = randomID()
	}
	if _, found := body["expected_balance"]; !found && (action == "payments" || action == "discounts" || action == "charges") {
		body["expected_balance"] = fixture.finance(t, accountID).Balance
	}
	return fixture.call(t, fixture.admin, "POST", "payments/accounts/"+accountID+"/"+action, body, status)
}

type paymentResult struct {
	Payment payments.PaymentRecord `json:"payment"`
	Finance payments.Finance       `json:"finance"`
	Closed  bool                   `json:"closed"`
}

func (fixture *reportsFixture) pay(t *testing.T, accountID string, body map[string]any) paymentResult {
	t.Helper()
	return decode[paymentResult](t, fixture.post(t, accountID, "payments", body, 200))
}

func (fixture *reportsFixture) versionOf(t *testing.T, accountID string, product catalog.Product) string {
	t.Helper()
	for _, item := range fixture.finance(t, accountID).Lines {
		if item.Name == product.Name {
			return item.VersionID
		}
	}
	t.Fatalf("La cuenta no contiene %s", product.Name)
	return ""
}

// staff crea un usuario con un rol de permisos exactos en una terminal propia sin caja.
func (fixture *reportsFixture) staff(t *testing.T, username string, permissions ...string) terminal {
	t.Helper()
	role := decode[struct {
		ID string `json:"id"`
	}](t, fixture.call(t, fixture.admin, "POST", "roles", map[string]any{"name": "Rol " + username, "permissions": permissions}, 200))
	password := "Clave temporal de " + username
	fixture.call(t, fixture.admin, "POST", "users", map[string]any{
		"username": username, "display_name": "Prueba " + username, "role_id": role.ID, "active": true, "password": password,
	}, 200)
	invitation := decode[struct {
		Code string `json:"code"`
	}](t, fixture.call(t, fixture.admin, "POST", "devices/invitations", nil, 200))
	device := decode[struct {
		ID    string `json:"device_id"`
		Token string `json:"device_token"`
	}](t, fixture.call(t, terminal{}, "POST", "devices/enroll", map[string]any{"code": invitation.Code, "name": "Terminal " + username}, 200))
	member := terminal{deviceID: device.ID, deviceToken: device.Token}
	member.sessionToken = decode[struct {
		Token string `json:"session_token"`
	}](t, fixture.call(t, member, "POST", "auth/login", map[string]string{"username": username, "password": password}, 200)).Token
	return member
}

// reportResult es la forma JSON del resultado de un reporte.
type reportResult struct {
	Report struct {
		ID string `json:"id"`
	} `json:"report"`
	Meta struct {
		Title  string `json:"title"`
		Period *struct {
			Kind     string    `json:"kind"`
			Label    string    `json:"label"`
			FirstDay string    `json:"first_day"`
			LastDay  string    `json:"last_day"`
			From     time.Time `json:"from"`
			Until    time.Time `json:"until"`
		} `json:"period"`
		Filters []struct {
			Key     string `json:"key"`
			Display string `json:"display"`
		} `json:"filters"`
		DayStart string `json:"day_start"`
		Warnings []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"warnings"`
	} `json:"meta"`
	Columns []struct {
		Key  string `json:"key"`
		Type string `json:"type"`
	} `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	Totals  map[string]any   `json:"totals"`
	Summary []struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	} `json:"summary"`
	Page struct {
		Page       int `json:"page"`
		PageSize   int `json:"page_size"`
		TotalRows  int `json:"total_rows"`
		TotalPages int `json:"total_pages"`
	} `json:"page"`
}

func (fixture *reportsFixture) report(t *testing.T, member terminal, id, query string) reportResult {
	t.Helper()
	route := "reports/run/" + id
	if query != "" {
		route += "?" + query
	}
	return decode[reportResult](t, fixture.get(t, member, route, 200))
}

// find devuelve el renglón cuyo valor de key es value.
func find(t *testing.T, result reportResult, key, value string) map[string]any {
	t.Helper()
	for _, row := range result.Rows {
		if fmt.Sprint(row[key]) == value {
			return row
		}
	}
	t.Fatalf("%s: no hay renglón con %s=%s: %+v", result.Report.ID, key, value, result.Rows)
	return nil
}

func text(value any) string {
	if value == nil {
		return "<nil>"
	}
	return fmt.Sprint(value)
}

func (fixture *reportsFixture) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var value string
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
