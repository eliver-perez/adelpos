//go:build integration

package catalog_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
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
)

func TestStageThreeAcceptance(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("No se pudo abrir el pool temporal")
	}
	defer pool.Close()
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal("La repetición de migraciones no fue segura", err)
	}
	provisioning := administration.NewToken()
	filename := filepath.Join(t.TempDir(), "provisioning")
	if err := os.WriteFile(filename, []byte(provisioning), 0600); err != nil {
		t.Fatal(err)
	}
	service := &administration.Service{Pool: pool, ProvisioningFile: filename}
	deviceID, deviceToken, sessionToken := "", "", ""
	currentTest := t
	run := func(name string, execute func(*testing.T)) {
		t.Run(name, func(child *testing.T) {
			currentTest = child
			defer func() { currentTest = t }()
			execute(child)
		})
	}
	call := func(method, route string, body any, expected int) []byte {
		currentTest.Helper()
		encoded, _ := json.Marshal(body)
		request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
		request.RemoteAddr = "127.0.0.1:50000"
		request.Header.Set("X-Provisioning-Token", provisioning)
		request.Header.Set("X-Device-ID", deviceID)
		request.Header.Set("X-Device-Token", deviceToken)
		request.Header.Set("Authorization", "Bearer "+sessionToken)
		response := httptest.NewRecorder()
		service.ServeHTTP(response, request)
		if response.Code != expected {
			if response.Code == 503 {
				_, problem := service.Execute(
					ctx,
					administration.Request{
						Method:       method,
						Path:         "/api/v1/" + route,
						Query:        request.URL.Query(),
						Body:         encoded,
						DeviceID:     deviceID,
						DeviceToken:  deviceToken,
						SessionToken: sessionToken,
						Local:        true,
					},
				)
				currentTest.Fatalf(

					"%s %s: %d; causa de prueba: %v",
					method,
					route,
					response.Code,
					problem,
				)
			}
			currentTest.Fatalf(

				"%s %s: esperado %d, recibido %d: %s",
				method,
				route,
				expected,
				response.Code,
				response.Body.String(),
			)
		}
		return response.Body.Bytes()
	}
	var setup struct {
		DeviceID    string `json:"device_id"`
		DeviceToken string `json:"device_token"`
	}
	_ = json.Unmarshal(
		call(
			"POST",
			"setup",
			map[string]string{
				"name":          "Catálogo temporal",
				"timezone":      "America/Monterrey",
				"currency":      "MXN",
				"theme":         "light",
				"terminal_name": "Prueba",
			},
			200,
		),
		&setup,
	)
	deviceID, deviceToken = setup.DeviceID, setup.DeviceToken
	var login struct {
		SessionToken string `json:"session_token"`
	}
	_ = json.Unmarshal(
		call("POST", "auth/login", map[string]string{"username": "admin", "password": "admin"}, 200),
		&login,
	)
	sessionToken = login.SessionToken

	category := catalog.Category{Metadata: catalog.Metadata{Name: "Platillos", Active: true}, Color: "#14665c"}
	run("categorías edición concurrencia y filtros", func(t *testing.T) {
		_ = json.Unmarshal(call("POST", "catalog/categories", category, 200), &category)
		stale := category
		category.Name = "Platillos de prueba"
		_ = json.Unmarshal(call("PATCH", "catalog/categories/"+category.ID, category, 200), &category)
		call("PATCH", "catalog/categories/"+category.ID, stale, 409)
		call(
			"POST",
			"catalog/categories/reorder",
			map[string]any{"items": []any{map[string]any{"id": category.ID, "revision": category.Revision, "sort_order": 2}}},
			200,
		)
		_ = json.Unmarshal(call("GET", "catalog/categories/"+category.ID, nil, 200), &category)
		response := call("GET", "catalog/categories?search=Platillos&active=true&page_size=1", nil, 200)
		if !bytes.Contains(response, []byte(`"total":1`)) {
			t.Fatal("No se aplicaron filtros/paginación")
		}
	})
	ingredient := catalog.Ingredient{Metadata: catalog.Metadata{Name: "Cebolla", Active: true}, Unit: "porción"}
	_ = json.Unmarshal(call("POST", "catalog/ingredients", ingredient, 200), &ingredient)
	portion := catalog.Portion{Metadata: catalog.Metadata{Name: "Entera", Active: true}, Factor: "1"}
	_ = json.Unmarshal(call("POST", "catalog/portions", portion, 200), &portion)
	group := catalog.OptionGroup{
		Metadata:    catalog.Metadata{Name: "Guarniciones", Active: true},
		PricingMode: "value",
		MinQuantity: "0",
	}
	for index, name := range []string{
		"Arroz",
		"Ensalada",
		"Papas",
		"Frijoles",
	} {
		complement := catalog.Complement{Metadata: catalog.Metadata{Name: name, Active: true}}
		_ = json.Unmarshal(call("POST", "catalog/complements", complement, 200), &complement)
		values := []string{
			"20",
			"15",
			"25",
			"10",
		}
		group.Options = append(
			group.Options,
			catalog.GroupOption{
				ComplementID:   complement.ID,
				PortionID:      &portion.ID,
				ReferenceValue: values[index],
				ExtraPrice:     "0",
				Active:         true,
			},
		)
	}
	_ = json.Unmarshal(call("POST", "catalog/groups", group, 200), &group)
	optionsByName := map[string]catalog.GroupOption{}
	for _, option := range group.Options {
		optionsByName[option.Name] = option
	}
	group.Options = []catalog.GroupOption{
		optionsByName["Arroz"],
		optionsByName["Ensalada"],
		optionsByName["Papas"],
		optionsByName["Frijoles"],
	}

	product := catalog.Product{
		Metadata:   catalog.Metadata{Name: "Filete", Active: true},
		CategoryID: category.ID,
		Kind:       "product",
		Code:       "FILETE",
		BasePrice:  "150",
		Ingredients: []catalog.ProductIngredient{
			{
				IngredientID:     ingredient.ID,
				IncludedQuantity: "1",
				Removable:        true,
				Addable:          true,
				MaxQuantity:      "3",
				ExtraPrice:       "2.50",
				RecipeQuantity:   "0.05",
			},
		},
		Groups: []catalog.ProductGroup{
			{
				GroupID:  group.ID,
				Included: []catalog.IncludedOption{{OptionID: group.Options[0].ID, Quantity: "1"}, {OptionID: group.Options[1].ID, Quantity: "1"}},
			},
		},
		Variants: []catalog.Variant{{
			Name:   "Grande",
			Code:   "FILETE-G",
			Price:  "180",
			Active: true,
		}},
	}
	run("producto variantes configuración y persistencia", func(t *testing.T) {
		_ = json.Unmarshal(call("POST", "catalog/products", product, 200), &product)
		call("GET", "catalog/products/"+product.ID+"/configuration", nil, 200)
		call("GET", "catalog/lookups", nil, 200)
		product.Description = "Configuración persistida"
		_ = json.Unmarshal(call("PATCH", "catalog/products/"+product.ID, product, 200), &product)
		call(
			"POST",
			"catalog/products/"+product.ID+"/duplicate",
			map[string]any{"name": "Filete duplicado", "code": "FILETE-D", "revision": product.Revision},
			200,
		)
		call("GET", "catalog/products?search=Filete&category_id="+category.ID+"&kind=product", nil, 200)
	})
	request := catalog.PreviewRequest{Lines: []catalog.LineRequest{{ProductID: product.ID, Quantity: 1}}}
	checkTotal := func(want string) {
		currentTest.Helper()
		var quote catalog.Quote
		_ = json.Unmarshal(call("POST", "catalog/preview", request, 200), &quote)
		if quote.Total != want {
			currentTest.Fatalf("Total esperado %s recibido %s", want, quote.Total)
		}
	}
	run("equivalencia sustituciones eliminación y extras", func(t *testing.T) {
		checkTotal("150.00")
		request.Lines[0].Customization.Groups = []catalog.GroupSelection{{GroupID: group.ID, Options: []catalog.IncludedOption{}}}
		checkTotal("150.00")
		request.Lines[0].Customization.Groups[0].Options = []catalog.IncludedOption{{OptionID: group.Options[2].ID, Quantity: "1"}, {OptionID: group.Options[1].ID, Quantity: "1"}}
		checkTotal("155.00")
		request.Lines[0].Customization.Groups[0].Options = []catalog.IncludedOption{{OptionID: group.Options[2].ID, Quantity: "1"}, {OptionID: group.Options[3].ID, Quantity: "1"}}
		checkTotal("150.00")
		request.Lines[0].Customization.Ingredients = []catalog.IngredientSelection{{IngredientID: ingredient.ID, Quantity: "2"}}
		checkTotal("152.50")
		request.Lines[0].Customization.Ingredients[0].Quantity = "0"
		checkTotal("150.00")
	})
	run("imágenes seguras y metadatos auditados", func(t *testing.T) {
		var buffer bytes.Buffer
		_ = png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2)))
		var result struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(
			call(
				"POST",
				"catalog/images",
				map[string]string{"content_base64": base64.StdEncoding.EncodeToString(buffer.Bytes())},
				200,
			),
			&result,
		)
		call("GET", "catalog/images/"+result.ID, nil, 200)
		call(
			"POST",
			"catalog/images",
			map[string]string{"content_base64": base64.StdEncoding.EncodeToString([]byte("archivo inválido"))},
			400,
		)
		call(
			"POST",
			"catalog/images",
			map[string]string{"content_base64": base64.StdEncoding.EncodeToString(buffer.Bytes()[:24])},
			400,
		)
		product.ImageID = &result.ID
		_ = json.Unmarshal(call("PATCH", "catalog/products/"+product.ID, product, 200), &product)
	})
	run("combos referencias escalonadas sustituciones y ciclos", func(t *testing.T) {
		burger := catalog.Product{
			Metadata:   catalog.Metadata{Name: "Hamburguesa", Active: true},
			CategoryID: category.ID,
			Code:       "BURGER",
			Kind:       "product",
			BasePrice:  "90",
		}
		fries := catalog.Product{
			Metadata:   catalog.Metadata{Name: "Papas producto", Active: true},
			CategoryID: category.ID,
			Code:       "PAPAS",
			Kind:       "product",
			BasePrice:  "40",
		}
		_ = json.Unmarshal(call("POST", "catalog/products", burger, 200), &burger)
		_ = json.Unmarshal(call("POST", "catalog/products", fries, 200), &fries)
		combo := catalog.Product{
			Metadata:   catalog.Metadata{Name: "Combo", Active: true},
			CategoryID: category.ID,
			Code:       "COMBO",
			Kind:       "combo",
			BasePrice:  "90",
			Slots: []catalog.ComboSlot{
				{
					Name:             "Principal",
					MinQuantity:      1,
					MaxQuantity:      1,
					IncludedQuantity: 1,
					DefaultQuantity:  1,
					DefaultProductID: burger.ID,
					Options:          []catalog.ComboOption{{ProductID: burger.ID, SubstitutionPrice: "0"}, {ProductID: product.ID, SubstitutionPrice: "5"}},
				},
				{
					Name:             "Papas opcionales",
					MaxQuantity:      2,
					DefaultProductID: fries.ID,
					Options: []catalog.ComboOption{
						{
							ProductID:         fries.ID,
							SubstitutionPrice: "0",
							Tiers:             []catalog.QuantityTier{{From: 1, UnitPrice: "10"}, {From: 2, UnitPrice: "40"}},
						},
					},
				},
			},
		}
		_ = json.Unmarshal(call("POST", "catalog/products", combo, 200), &combo)
		slotsByName := map[string]catalog.ComboSlot{}
		for _, slot := range combo.Slots {
			slotsByName[slot.Name] = slot
		}
		combo.Slots = []catalog.ComboSlot{slotsByName["Principal"], slotsByName["Papas opcionales"]}

		for quantity, want := range []string{"90.00", "100.00", "140.00"} {
			request.Lines = []catalog.LineRequest{
				{
					ProductID: combo.ID,
					Quantity:  1,
					Customization: catalog.Customization{
						Components: []catalog.ComponentSelection{{SlotID: combo.Slots[1].ID, ProductID: fries.ID, Quantity: quantity}},
					},
				},
			}
			checkTotal(want)
		}
		burger.Kind = "combo"
		burger.Slots = []catalog.ComboSlot{
			{
				Name:             "Ciclo",
				MaxQuantity:      1,
				DefaultProductID: combo.ID,
				Options:          []catalog.ComboOption{{ProductID: combo.ID, SubstitutionPrice: "0"}},
			},
		}
		call("PATCH", "catalog/products/"+burger.ID, burger, 400)
		request.Lines[0].Customization.Components = append(
			request.Lines[0].Customization.Components,
			catalog.ComponentSelection{SlotID: combo.Slots[0].ID, ProductID: product.ID, Quantity: 1},
		)
		checkTotal("145.00")
	})
	run("listas promociones vigencia y exclusiones", func(t *testing.T) {
		request = catalog.PreviewRequest{Lines: []catalog.LineRequest{{ProductID: product.ID, Quantity: 2}}}
		promotion := catalog.Promotion{
			Metadata:        catalog.Metadata{Name: "2 por 1", Active: true},
			Kind:            "two_for_one",
			Percent:         "0",
			SpecialPrice:    "0",
			BuyQuantity:     2,
			PayQuantity:     1,
			MinimumQuantity: 2,
			ProductIDs:      []string{product.ID},
		}
		_ = json.Unmarshal(call("POST", "catalog/promotions", promotion, 200), &promotion)
		checkTotal("150.00")
		request.Lines[0].Quantity = 3
		checkTotal("300.00")
		list := catalog.PriceList{
			Metadata:        catalog.Metadata{Name: "Plataforma", Active: true},
			Fallback:        "reject",
			AllowPromotions: true,
			Entries:         []catalog.PriceEntry{{ProductID: product.ID, Price: "175"}},
		}
		_ = json.Unmarshal(call("POST", "catalog/price-lists", list, 200), &list)
		request.PriceListID = &list.ID
		checkTotal("525.00")
		promotion.PriceListIDs = []string{list.ID}
		_ = json.Unmarshal(call("PATCH", "catalog/promotions/"+promotion.ID, promotion, 200), &promotion)
		checkTotal("350.00")
		promotion.Availability.EndsOn = "2000-01-01"
		_ = json.Unmarshal(call("PATCH", "catalog/promotions/"+promotion.ID, promotion, 200), &promotion)
		checkTotal("525.00")
		request.Lines[0].VariantID = &product.Variants[0].ID
		call("POST", "catalog/preview", request, 400)
	})
	run("disponibilidad desactivación referencias archivado y excepción", func(t *testing.T) {
		request = catalog.PreviewRequest{Lines: []catalog.LineRequest{{ProductID: product.ID, Quantity: 1}}}
		category.Active = false
		_ = json.Unmarshal(call("PATCH", "catalog/categories/"+category.ID, category, 200), &category)
		call("POST", "catalog/preview", request, 409)
		category.Active = true
		_ = json.Unmarshal(call("PATCH", "catalog/categories/"+category.ID, category, 200), &category)
		_ = json.Unmarshal(
			call(
				"PATCH",
				"catalog/products/"+product.ID+"/availability",
				map[string]any{"revision": product.Revision, "temporary_unavailable": true, "availability": catalog.Schedule{}},
				200,
			),
			&product,
		)
		call("POST", "catalog/preview", request, 409)
		request.OverrideAvailability = true
		request.OverrideReason = "Validación autorizada en base temporal"
		checkTotal("150.00")
		_ = json.Unmarshal(
			call(
				"PATCH",
				"catalog/products/"+product.ID+"/state",
				map[string]any{"revision": product.Revision, "active": false, "archived": true},
				200,
			),
			&product,
		)
		call("POST", "catalog/preview", request, 409)
		call("GET", "catalog/products?archived=true", nil, 200)
	})
	run("edición de referencias y cambio de predeterminado auditado", func(t *testing.T) {
		ingredient.Description = "Ingrediente editado"
		_ = json.Unmarshal(call("PATCH", "catalog/ingredients/"+ingredient.ID, ingredient, 200), &ingredient)
		portion.Factor = "0.5"
		_ = json.Unmarshal(call("PATCH", "catalog/portions/"+portion.ID, portion, 200), &portion)
		group.Description = "Grupo editado sin duplicar opciones"
		_ = json.Unmarshal(call("PATCH", "catalog/groups/"+group.ID, group, 200), &group)
		var complement catalog.Complement
		_ = json.Unmarshal(call("GET", "catalog/complements/"+group.Options[0].ComplementID, nil, 200), &complement)
		complement.Active = false
		_ = json.Unmarshal(call("PATCH", "catalog/complements/"+complement.ID, complement, 200), &complement)
		complement.Active = true
		call("PATCH", "catalog/complements/"+complement.ID, complement, 200)
		var lists struct {
			Items []catalog.PriceList `json:"items"`
		}
		_ = json.Unmarshal(call("GET", "catalog/price-lists", nil, 200), &lists)
		var original, alternative catalog.PriceList
		for _, list := range lists.Items {
			if list.Default {
				original = list
			} else {
				alternative = list
			}
		}
		alternative.Default = true
		call("PATCH", "catalog/price-lists/"+alternative.ID, alternative, 200)
		_ = json.Unmarshal(call("GET", "catalog/price-lists/"+original.ID, nil, 200), &original)
		if original.Default || original.Revision != 2 {
			t.Fatal("No cambió revisión de la lista desplazada")
		}
		original.Default = true
		call("PATCH", "catalog/price-lists/"+original.ID, original, 200)
	})
	run("permisos vigentes precios protegidos y auditoría", func(t *testing.T) {
		var userID string
		if err := pool.QueryRow(ctx, "SELECT id FROM app.users WHERE username='admin'").Scan(&userID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(
			ctx,
			"INSERT INTO app.user_permissions(user_id,permission_key,effect) VALUES ($1,'catalog.prices.read','deny'),($1,'catalog.products.manage','deny')",
			userID,
		); err != nil {
			t.Fatal(err)
		}
		response := call("GET", "catalog/products/"+product.ID, nil, 200)
		if bytes.Contains(response, []byte("base_price")) || bytes.Contains(response, []byte("extra_price")) {
			t.Fatal("Se filtraron precios sin permiso")
		}
		auditResponse := call("GET", "audit?action=catalog.products.created", nil, 200)
		if bytes.Contains(auditResponse, []byte("base_price")) || bytes.Contains(auditResponse, []byte("extra_price")) {
			t.Fatal("La auditoría permitió eludir el permiso de precios")
		}
		call("POST", "catalog/products", product, 403)
		call("POST", "catalog/preview", request, 403)
		var changes, denials, overrides int
		err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE action LIKE 'catalog.%'),count(*) FILTER(WHERE action='access.denied'),
            count(*) FILTER(WHERE action='catalog.availability.overridden') FROM app.audit_events`).Scan(&changes, &denials, &overrides)
		if err != nil || changes < 20 || denials < 2 || overrides < 1 {
			t.Fatalf(

				"Auditoría incompleta: %d %d %d %v",
				changes,
				denials,
				overrides,
				err,
			)
		}
	})
}
