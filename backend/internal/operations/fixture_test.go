//go:build integration

package operations_test

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
)

type testTerminal struct {
	deviceID     string
	deviceToken  string
	sessionToken string
}

type restaurantFixture struct {
	ctx           context.Context
	pool          *pgxpool.Pool
	service       *administration.Service
	provisioning  string
	admin         testTerminal
	waiter        testTerminal
	product       catalog.Product
	fries         catalog.Product
	combo         catalog.Product
	group         catalog.OptionGroup
	category      catalog.Category
	table         operations.Table
	secondTable   operations.Table
	area          operations.Area
	secondaryArea operations.Area
	account       operations.Bill
	secondAccount operations.Bill
	splitAccount  operations.Bill
	// queues sustituye las consultas de CUPS del cajón (etapa 7.1): ninguna prueba ejecuta lpstat,
	// lpoptions ni lp; una cola no registrada responde como no consultable.
	queues *controlledQueues
}

func randomID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

func decodeResponse[T any](t *testing.T, body []byte) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal("La respuesta no cumple el contrato", err)
	}
	return result
}

func newFixture(t *testing.T) *restaurantFixture {
	t.Helper()
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	// La aceptación encadenada (etapas 4 a 7.1) supera los dos minutos con -race en este equipo.
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
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal("Migración no repetible", err)
	}
	token := administration.NewToken()
	filename := filepath.Join(t.TempDir(), "provisioning")
	if err := os.WriteFile(filename, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	queues := newControlledQueues()
	fixture := &restaurantFixture{
		ctx: ctx, pool: pool, provisioning: token, queues: queues,
		service: &administration.Service{Pool: pool, ProvisioningFile: filename, Queues: queues},
	}
	setup := fixture.call(t, testTerminal{}, "POST", "setup", map[string]string{
		"name": "Restaurante temporal de etapa 4", "timezone": "America/Monterrey", "currency": "MXN", "theme": "light", "terminal_name": "Prueba principal",
	}, 200)
	credentials := decodeResponse[map[string]any](t, setup)
	fixture.admin.deviceID = credentials["device_id"].(string)
	fixture.admin.deviceToken = credentials["device_token"].(string)
	login := decodeResponse[map[string]any](t, fixture.call(t, fixture.admin, "POST", "auth/login", map[string]string{"username": "admin", "password": "admin"}, 200))
	fixture.admin.sessionToken = login["session_token"].(string)
	return fixture
}

func (fixture *restaurantFixture) call(t *testing.T, terminal testTerminal, method, route string, body any, status int) []byte {
	t.Helper()
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:51000"
	request.Header.Set("X-Provisioning-Token", fixture.provisioning)
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	if response.Code != status {
		if response.Code == 503 {
			_, problem := fixture.service.Execute(fixture.ctx, administration.Request{
				Method: method, Path: "/api/v1/" + route, Query: request.URL.Query(), Body: encoded,
				DeviceID: terminal.deviceID, DeviceToken: terminal.deviceToken, SessionToken: terminal.sessionToken, Local: true,
			})
			t.Fatalf("%s %s: esperado %d recibido %d; causa de prueba %v", method, route, status, response.Code, problem)
		}
		t.Fatalf("%s %s: esperado %d recibido %d: %s", method, route, status, response.Code, response.Body.String())
	}
	return append([]byte{}, response.Body.Bytes()...)
}

func (fixture *restaurantFixture) adminCall(t *testing.T, method, route string, body any, status int) []byte {
	t.Helper()
	return fixture.call(t, fixture.admin, method, route, body, status)
}

func (fixture *restaurantFixture) mutate(t *testing.T, method, route string, body any, status int) []byte {
	t.Helper()
	var fields map[string]any
	encoded, _ := json.Marshal(body)
	_ = json.Unmarshal(encoded, &fields)
	fields["operation_id"] = randomID()
	return fixture.adminCall(t, method, route, fields, status)
}

func (fixture *restaurantFixture) refresh(t *testing.T, id string) operations.Bill {
	t.Helper()
	return decodeResponse[operations.Bill](t, fixture.adminCall(t, "GET", "restaurant/accounts/"+id, nil, 200))
}

func (fixture *restaurantFixture) capture(t *testing.T, bill operations.Bill, lines []catalog.LineRequest) operations.Bill {
	t.Helper()
	draft := make([]operations.DraftItem, 0, len(lines))
	for _, line := range lines {
		draft = append(draft, operations.DraftItem{ID: randomID(), LineRequest: line})
	}
	route := "restaurant/accounts/" + bill.Account.ID
	bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", route+"/draft", map[string]any{"revision": bill.Account.Revision, "items": draft}, 200))
	preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "GET", route+"/preview", nil, 200))
	return decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", route+"/confirm", map[string]any{
		"revision": bill.Account.Revision, "expected_fingerprint": preview.Fingerprint,
	}, 200))
}

func (fixture *restaurantFixture) seedCatalog(t *testing.T) {
	t.Helper()
	fixture.category = decodeResponse[catalog.Category](t, fixture.adminCall(t, "POST", "catalog/categories", catalog.Category{Metadata: catalog.Metadata{Name: "Platillos", Active: true}}, 200))
	fixture.group = catalog.OptionGroup{Metadata: catalog.Metadata{Name: "Guarniciones", Active: true}, PricingMode: "value", MinQuantity: "0"}
	for index, name := range []string{"Arroz", "Ensalada", "Papas", "Frijoles"} {
		complement := decodeResponse[catalog.Complement](t, fixture.adminCall(t, "POST", "catalog/complements", catalog.Complement{Metadata: catalog.Metadata{Name: name, Active: true}}, 200))
		prices := []string{"20", "15", "25", "10"}
		fixture.group.Options = append(fixture.group.Options, catalog.GroupOption{
			ComplementID: complement.ID, ReferenceValue: prices[index], ExtraPrice: "0", Active: true,
		})
	}
	fixture.group = decodeResponse[catalog.OptionGroup](t, fixture.adminCall(t, "POST", "catalog/groups", fixture.group, 200))
	byName := map[string]catalog.GroupOption{}
	for _, option := range fixture.group.Options {
		byName[option.Name] = option
	}
	fixture.group.Options = []catalog.GroupOption{byName["Arroz"], byName["Ensalada"], byName["Papas"], byName["Frijoles"]}
	ingredient := decodeResponse[catalog.Ingredient](t, fixture.adminCall(t, "POST", "catalog/ingredients", catalog.Ingredient{Metadata: catalog.Metadata{Name: "Cebolla", Active: true}, Unit: "porción"}, 200))
	fixture.product = catalog.Product{
		Metadata: catalog.Metadata{Name: "Filete", Active: true}, CategoryID: fixture.category.ID, Code: "FILETE", Kind: "product", BasePrice: "150",
		Ingredients: []catalog.ProductIngredient{{
			IngredientID:     ingredient.ID,
			IncludedQuantity: "1",
			Removable:        true,
			Addable:          true,
			MaxQuantity:      "3",
			ExtraPrice:       "2.50",
			RecipeQuantity:   "0.05",
		}},
		Groups: []catalog.ProductGroup{{
			GroupID: fixture.group.ID,
			Included: []catalog.IncludedOption{
				{OptionID: fixture.group.Options[0].ID, Quantity: "1"},
				{OptionID: fixture.group.Options[1].ID, Quantity: "1"},
			},
		}},
		Variants: []catalog.Variant{{Name: "Grande", Code: "FILETE-G", Price: "180", Active: true}},
	}
	fixture.product = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", fixture.product, 200))
	fixture.fries = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Papas producto", Active: true}, CategoryID: fixture.category.ID, Code: "PAPAS", Kind: "product", BasePrice: "40",
	}, 200))
	fixture.combo = catalog.Product{
		Metadata: catalog.Metadata{Name: "Hamburguesa y papas opcionales", Active: true}, CategoryID: fixture.category.ID, Code: "COMBO", Kind: "combo", BasePrice: "90",
		Slots: []catalog.ComboSlot{{
			Name:             "Papas opcionales",
			MaxQuantity:      2,
			DefaultProductID: fixture.fries.ID,
			Options: []catalog.ComboOption{{
				ProductID:         fixture.fries.ID,
				SubstitutionPrice: "0",
				Tiers: []catalog.QuantityTier{
					{From: 1, UnitPrice: "10"},
					{From: 2, UnitPrice: "40"},
				},
			}},
		}},
	}
	fixture.combo = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", fixture.combo, 200))
}
