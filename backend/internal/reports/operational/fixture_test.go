//go:build integration

package operational_test

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
	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Fixture de los reportes operativos: una base temporal propia, la API real (administration.Service)
// para crear catálogo, cuentas, cobros, turnos, operaciones de caja, inventario y compras, y los
// reportes consultados por HTTP como la terminal. Solo se escribe directamente en la base para
// simular el paso del tiempo (fechas de turnos y movimientos de caja de días anteriores), porque
// esas tablas no tienen disparadores de solo inserción y la API usa siempre la hora actual.

type terminal struct {
	deviceID     string
	deviceToken  string
	sessionToken string
}

type fixture struct {
	ctx          context.Context
	pool         *pgxpool.Pool
	service      *administration.Service
	provisioning string
	admin        terminal
	adminID      string
	methods      map[string]string
	registerID   string
	category     catalog.Category
	burger       catalog.Product
	soda         catalog.Product
	salad        catalog.Product
	table        operations.Table
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
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Second)
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

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx, pool := openPool(t)
	token := administration.NewToken()
	filename := filepath.Join(t.TempDir(), "provisioning")
	if err := os.WriteFile(filename, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	current := &fixture{ctx: ctx, pool: pool, provisioning: token, methods: map[string]string{},
		service: &administration.Service{Pool: pool, ProvisioningFile: filename}}
	setup := decode[map[string]any](t, current.call(t, terminal{}, "POST", "setup", map[string]string{
		"name": "Restaurante operativo", "timezone": "America/Monterrey", "currency": "MXN", "theme": "light",
		"terminal_name": "Caja principal",
	}, 200))
	current.admin.deviceID = setup["device_id"].(string)
	current.admin.deviceToken = setup["device_token"].(string)
	login := decode[map[string]any](t, current.call(t, current.admin, "POST", "auth/login",
		map[string]string{"username": "admin", "password": "admin"}, 200))
	current.admin.sessionToken = login["session_token"].(string)
	for _, method := range decode[[]payments.Method](t, current.get(t, current.admin, "payments/methods", 200)) {
		if method.Code != nil {
			current.methods[*method.Code] = method.ID
		}
	}
	current.registerID = current.scalar(t, "SELECT register_id::text FROM app.devices WHERE id=$1", current.admin.deviceID)
	current.adminID = current.scalar(t, "SELECT id::text FROM app.users WHERE username='admin'")
	return current
}

// shared conserva el fixture de TestOperationalReports para la medición opt-in de volumen, que
// corre después en la misma base (las pruebas del paquete son secuenciales).
var shared *fixture

// reopenFixture abre otro pool sobre la misma base con las credenciales de la aceptación.
func reopenFixture(t *testing.T) *fixture {
	t.Helper()
	if shared == nil {
		t.Skip("Requiere TestOperationalReports en la misma ejecución")
	}
	ctx, pool := openPool(t)
	copied := *shared
	copied.ctx, copied.pool = ctx, pool
	filename := filepath.Join(t.TempDir(), "provisioning")
	if err := os.WriteFile(filename, []byte(copied.provisioning), 0600); err != nil {
		t.Fatal(err)
	}
	copied.service = &administration.Service{Pool: pool, ProvisioningFile: filename}
	return &copied
}

// request ejecuta una petición HTTP real contra el servicio y devuelve código y cuerpo.
func (current *fixture) request(member terminal, method, route string, body any) (int, []byte) {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, "/api/v1/"+route, reader)
	request.RemoteAddr = "127.0.0.1:52100"
	request.Header.Set("X-Provisioning-Token", current.provisioning)
	request.Header.Set("X-Device-ID", member.deviceID)
	request.Header.Set("X-Device-Token", member.deviceToken)
	request.Header.Set("Authorization", "Bearer "+member.sessionToken)
	response := httptest.NewRecorder()
	current.service.ServeHTTP(response, request)
	return response.Code, append([]byte{}, response.Body.Bytes()...)
}

func (current *fixture) call(t *testing.T, member terminal, method, route string, body any, status int) []byte {
	t.Helper()
	code, response := current.request(member, method, route, body)
	if code != status {
		t.Fatalf("%s %s: esperado %d recibido %d: %s", method, route, status, code, response)
	}
	return response
}

func (current *fixture) get(t *testing.T, member terminal, route string, status int) []byte {
	t.Helper()
	return current.call(t, member, "GET", route, nil, status)
}

// write agrega un operation_id nuevo al cuerpo de una escritura.
func (current *fixture) write(t *testing.T, member terminal, method, route string, body map[string]any, status int) []byte {
	t.Helper()
	if body == nil {
		body = map[string]any{}
	}
	if identifier, _ := body["operation_id"].(string); identifier == "" {
		body["operation_id"] = randomID()
	}
	return current.call(t, member, method, route, body, status)
}

func (current *fixture) mutate(t *testing.T, method, route string, body map[string]any, status int) []byte {
	t.Helper()
	return current.write(t, current.admin, method, route, body, status)
}

func structToMap(value any) map[string]any {
	encoded, _ := json.Marshal(value)
	result := map[string]any{}
	_ = json.Unmarshal(encoded, &result)
	return result
}

func (current *fixture) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var value string
	if err := current.pool.QueryRow(current.ctx, query, args...).Scan(&value); err != nil {
		t.Fatalf("%v: %s", err, query)
	}
	return value
}

func (current *fixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := current.pool.Exec(current.ctx, query, args...); err != nil {
		t.Fatalf("%v: %s", err, query)
	}
}

func (current *fixture) seedCatalog(t *testing.T) {
	t.Helper()
	create := func(value any) catalog.Product {
		return decode[catalog.Product](t, current.call(t, current.admin, "POST", "catalog/products", value, 200))
	}
	current.category = decode[catalog.Category](t, current.call(t, current.admin, "POST", "catalog/categories",
		catalog.Category{Metadata: catalog.Metadata{Name: "Platillos", Active: true}}, 200))
	product := func(name, code, price string) catalog.Product {
		return create(catalog.Product{Metadata: catalog.Metadata{Name: name, Active: true}, CategoryID: current.category.ID, Code: code,
			Kind: "product", BasePrice: price})
	}
	current.burger = product("Hamburguesa", "HAMB", "120")
	current.soda = product("Refresco", "REFR", "25")
	current.salad = product("Ensalada", "ENSA", "50")
	// Una estación KDS con la ruta de la categoría: la confirmación libera a cocina (y con el
	// inventario activado registra el consumo) sin impresoras.
	station := decode[kitchen.Station](t, current.mutate(t, "POST", "kitchen/stations", structToMap(kitchen.Station{
		Name: "Cocina", Active: true, Reception: "kds", TargetMinutes: 12, WarningMinutes: 15, OverdueMinutes: 25, Grouping: "inherit",
		Tracking: "inherit",
	}), 200))
	current.mutate(t, "POST", "kitchen/routes", structToMap(kitchen.Route{CategoryID: &current.category.ID, Mode: "prepare",
		StationIDs: []string{station.ID}}), 200)
	area := decode[operations.Area](t, current.mutate(t, "POST", "restaurant/areas", structToMap(operations.Area{Name: "Salón", Active: true}), 200))
	current.table = decode[operations.Table](t, current.mutate(t, "POST", "restaurant/tables", structToMap(operations.Table{
		AreaID: area.ID, Name: "Mesa 1", Shape: "rectangle", X: 20, Y: 30, Width: 100, Height: 72, Seats: 4, Active: true,
	}), 200))
}

func line(product catalog.Product, quantity int) catalog.LineRequest {
	return catalog.LineRequest{ProductID: product.ID, Quantity: quantity}
}

// account abre una cuenta sin mesa y confirma sus partidas (la confirmación libera a cocina).
func (current *fixture) account(t *testing.T, name string, lines ...catalog.LineRequest) operations.Bill {
	t.Helper()
	bill := decode[operations.Bill](t, current.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": name, "mode": "dine_in"}, 200))
	draft := make([]operations.DraftItem, 0, len(lines))
	for _, item := range lines {
		draft = append(draft, operations.DraftItem{ID: randomID(), LineRequest: item})
	}
	route := "restaurant/accounts/" + bill.Account.ID
	bill = decode[operations.Bill](t, current.get(t, current.admin, route, 200))
	bill = decode[operations.Bill](t, current.mutate(t, "PUT", route+"/draft", map[string]any{"revision": bill.Account.Revision, "items": draft}, 200))
	preview := decode[operations.QuoteResult](t, current.get(t, current.admin, route+"/preview", 200))
	return decode[operations.Bill](t, current.mutate(t, "POST", route+"/confirm", map[string]any{
		"revision": bill.Account.Revision, "expected_fingerprint": preview.Fingerprint,
	}, 200))
}

func (current *fixture) finance(t *testing.T, accountID string) payments.Finance {
	t.Helper()
	return decode[payments.Finance](t, current.get(t, current.admin, "payments/accounts/"+accountID, 200))
}

// pay registra un pago desde la terminal indicada (que cobra en el turno de su caja).
func (current *fixture) pay(t *testing.T, member terminal, accountID string, body map[string]any) map[string]any {
	t.Helper()
	body["operation_id"] = randomID()
	body["expected_balance"] = current.finance(t, accountID).Balance
	return decode[map[string]any](t, current.call(t, member, "POST", "payments/accounts/"+accountID+"/payments", body, 200))
}

// staff crea un usuario con un rol de permisos exactos en una terminal propia (con caja si se indica).
func (current *fixture) staff(t *testing.T, username, registerID string, permissions ...string) terminal {
	t.Helper()
	role := decode[struct {
		ID string `json:"id"`
	}](t, current.call(t, current.admin, "POST", "roles", map[string]any{"name": "Rol " + username, "permissions": permissions}, 200))
	password := "Clave temporal de " + username
	current.call(t, current.admin, "POST", "users", map[string]any{
		"username": username, "display_name": "Prueba " + username, "role_id": role.ID, "active": true, "password": password,
	}, 200)
	invitation := decode[struct {
		Code string `json:"code"`
	}](t, current.call(t, current.admin, "POST", "devices/invitations", nil, 200))
	device := decode[struct {
		ID    string `json:"device_id"`
		Token string `json:"device_token"`
	}](t, current.call(t, terminal{}, "POST", "devices/enroll", map[string]any{"code": invitation.Code, "name": "Terminal " + username}, 200))
	if registerID != "" {
		current.call(t, current.admin, "PATCH", "devices/"+device.ID, map[string]any{"name": "Terminal " + username, "register_id": registerID,
			"keyboard_mode": "auto"}, 200)
	}
	member := terminal{deviceID: device.ID, deviceToken: device.Token}
	member.sessionToken = decode[struct {
		Token string `json:"session_token"`
	}](t, current.call(t, member, "POST", "auth/login", map[string]string{"username": username, "password": password}, 200)).Token
	return member
}

// setFeature activa o desactiva una función del establecimiento.
func (current *fixture) setFeature(t *testing.T, code string, enabled bool) {
	t.Helper()
	features := decode[[]struct {
		Code     string `json:"code"`
		Enabled  bool   `json:"enabled"`
		Revision int    `json:"revision"`
	}](t, current.get(t, current.admin, "settings/features", 200))
	for _, feature := range features {
		if feature.Code == code && feature.Enabled != enabled {
			current.call(t, current.admin, "PUT", "settings/features", map[string]any{"code": code, "enabled": enabled,
				"revision": feature.Revision}, 200)
		}
	}
}

func (current *fixture) process(t *testing.T) {
	t.Helper()
	if _, err := inventory.ProcessPending(current.ctx, current.pool, 1000); err != nil {
		t.Fatal(err)
	}
}

// reportResult es la forma JSON del resultado de un reporte.
type reportResult struct {
	Report struct {
		ID string `json:"id"`
	} `json:"report"`
	Meta struct {
		Warnings []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"warnings"`
		Notes []string `json:"notes"`
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
		TotalRows int `json:"total_rows"`
	} `json:"page"`
	raw []byte
}

func (current *fixture) report(t *testing.T, member terminal, id, query string) reportResult {
	t.Helper()
	route := "reports/run/" + id + "?"
	if !strings.Contains(query, "page_size=") {
		route += "page_size=200&"
	}
	route += query
	body := current.get(t, member, route, 200)
	result := decode[reportResult](t, body)
	result.raw = body
	return result
}

func (result reportResult) has(key string) bool {
	for _, column := range result.Columns {
		if column.Key == key {
			return true
		}
	}
	return false
}

func (result reportResult) warning(code string) bool {
	for _, warning := range result.Meta.Warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}

func (result reportResult) figure(key string) any {
	for _, figure := range result.Summary {
		if figure.Key == key {
			return figure.Value
		}
	}
	return nil
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

func amount(t *testing.T, value any) decimal.Decimal {
	t.Helper()
	parsed, err := decimal.NewFromString(text(value))
	if err != nil {
		t.Fatalf("Importe inválido %v", value)
	}
	return parsed
}

// sum suma una columna de todos los renglones (para comparar con los totales de Go).
func sum(t *testing.T, result reportResult, key string) decimal.Decimal {
	t.Helper()
	total := decimal.Zero
	for _, row := range result.Rows {
		if row[key] != nil {
			total = total.Add(amount(t, row[key]))
		}
	}
	return total
}

// equal compara importes como decimales («10» y «10.00» son iguales).
func equal(t *testing.T, value any, expected string) bool {
	t.Helper()
	return amount(t, value).Equal(decimal.RequireFromString(expected))
}
