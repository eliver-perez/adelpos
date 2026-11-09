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
	"local/ad-el-pos/server/internal/ordering"
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

	category := catalog.Category{Metadata: catalog.Metadata{Name: "Platillos", Active: true}, Color: "#1f9a78"}
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
	run("combo con producto original implícito y sin autorreferencia", func(t *testing.T) {
		// Etapa 6.1 §1.4: «Productos permitidos y sustituciones» es opcional; el original siempre está
		// permitido. §1.5: un combo no puede contenerse a sí mismo y el original debe elegirse.
		drink := catalog.Product{
			Metadata:   catalog.Metadata{Name: "Refresco", Active: true},
			CategoryID: category.ID,
			Code:       "REFRESCO-61",
			Kind:       "product",
			BasePrice:  "25",
		}
		_ = json.Unmarshal(call("POST", "catalog/products", drink, 200), &drink)
		combo := catalog.Product{
			Metadata:   catalog.Metadata{Name: "Combo sin alternativas", Active: true},
			CategoryID: category.ID,
			Code:       "COMBO-61",
			Kind:       "combo",
			BasePrice:  "120",
			Slots: []catalog.ComboSlot{
				{
					Name:             "Bebida",
					MinQuantity:      1,
					MaxQuantity:      1,
					IncludedQuantity: 1,
					DefaultQuantity:  1,
					DefaultProductID: drink.ID,
				},
			},
		}
		_ = json.Unmarshal(call("POST", "catalog/products", combo, 200), &combo)
		if len(combo.Slots) != 1 || len(combo.Slots[0].Options) != 1 {
			t.Fatalf("el original no quedó como alternativa implícita: %+v", combo.Slots)
		}
		original := combo.Slots[0].Options[0]
		if original.ProductID != drink.ID || original.SubstitutionPrice != "0.00" || len(original.Tiers) != 0 {
			t.Fatalf("alternativa implícita inesperada: %+v", original)
		}
		request = catalog.PreviewRequest{Lines: []catalog.LineRequest{{ProductID: combo.ID, Quantity: 1}}}
		checkTotal("120.00")
		var saved catalog.Product
		_ = json.Unmarshal(call("GET", "catalog/products/"+combo.ID, nil, 200), &saved)
		if len(saved.Slots) != 1 || len(saved.Slots[0].Options) != 1 {
			t.Fatalf("la consulta no conserva el original: %+v", saved.Slots)
		}
		selfReference := saved
		selfReference.Slots = []catalog.ComboSlot{saved.Slots[0]}
		selfReference.Slots[0].DefaultProductID = saved.ID
		selfReference.Slots[0].Options = nil
		problem := string(call("PATCH", "catalog/products/"+saved.ID, selfReference, 400))
		if !strings.Contains(problem, "sí mismo") {
			t.Fatalf("la autorreferencia del original no se explicó: %s", problem)
		}
		missing := saved
		missing.Slots = []catalog.ComboSlot{saved.Slots[0]}
		missing.Slots[0].DefaultProductID = ""
		missing.Slots[0].Options = nil
		problem = string(call("PATCH", "catalog/products/"+saved.ID, missing, 400))
		if !strings.Contains(problem, "producto original") {
			t.Fatalf("la falta de original no se explicó: %s", problem)
		}
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
	run("paleta de colores con conservación del color heredado", func(t *testing.T) {
		// Etapa 6.1 §2.3: Go acepta sin color o un color de la paleta; un color capturado antes de
		// la paleta se conserva al editar, pero no se puede capturar uno nuevo fuera de ella.
		var lookups struct {
			Palette  []catalog.PaletteColor `json:"palette"`
			Products []map[string]any       `json:"products"`
		}
		_ = json.Unmarshal(call("GET", "catalog/lookups", nil, 200), &lookups)
		if len(lookups.Palette) != 16 || lookups.Palette[0].Hex != catalog.Palette[0].Hex {
			t.Fatalf("los lookups no exponen la paleta: %+v", lookups.Palette)
		}
		for _, entry := range lookups.Products {
			if _, ok := entry["category_id"].(string); !ok {
				t.Fatalf("un producto de los lookups no expone su categoría: %+v", entry)
			}
			if _, ok := entry["sort_order"].(float64); !ok {
				t.Fatalf("un producto de los lookups no expone su orden: %+v", entry)
			}
		}
		outside := catalog.Category{Metadata: catalog.Metadata{Name: "Color libre", Active: true}, Color: "#14665c"}
		if problem := string(call("POST", "catalog/categories", outside, 400)); !strings.Contains(problem, "paleta") {
			t.Fatalf("el rechazo del color no se explicó: %s", problem)
		}
		legacy := catalog.Category{Metadata: catalog.Metadata{Name: "Color heredado", Active: true}, Color: "#d64545"}
		_ = json.Unmarshal(call("POST", "catalog/categories", legacy, 200), &legacy)
		if _, err := pool.Exec(ctx, "UPDATE app.catalog_categories SET color='#14665c' WHERE id=$1", legacy.ID); err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal(call("GET", "catalog/categories/"+legacy.ID, nil, 200), &legacy)
		legacy.Description = "Conserva su color anterior"
		_ = json.Unmarshal(call("PATCH", "catalog/categories/"+legacy.ID, legacy, 200), &legacy)
		if legacy.Color != "#14665c" {
			t.Fatalf("no se conservó el color heredado: %q", legacy.Color)
		}
		changed := legacy
		changed.Color = "#123456"
		call("PATCH", "catalog/categories/"+legacy.ID, changed, 400)
		changed.Color = "#3591D4"
		_ = json.Unmarshal(call("PATCH", "catalog/categories/"+legacy.ID, changed, 200), &legacy)
		if legacy.Color != "#3591d4" {
			t.Fatalf("el color de la paleta no quedó en forma canónica: %q", legacy.Color)
		}
		legacy.Color = ""
		_ = json.Unmarshal(call("PATCH", "catalog/categories/"+legacy.ID, legacy, 200), &legacy)
		if legacy.Color != "" {
			t.Fatal("no se pudo quitar el color")
		}
	})
	run("orden visual por posiciones dentro de cada lista", func(t *testing.T) {
		// Etapa 6.1 §2.2: el usuario elige «al inicio» o «después de» y Go renumera en múltiplos de
		// 10 dentro de la misma transacción, incrementando la revisión de los hermanos movidos.
		at := func(after string) *ordering.Position {
			return &ordering.Position{After: &after}
		}
		readCategory := func(id string) catalog.Category {
			var value catalog.Category
			_ = json.Unmarshal(call("GET", "catalog/categories/"+id, nil, 200), &value)
			return value
		}
		readProduct := func(id string) catalog.Product {
			var value catalog.Product
			_ = json.Unmarshal(call("GET", "catalog/products/"+id, nil, 200), &value)
			return value
		}
		var categories struct {
			Items []catalog.Category `json:"items"`
		}
		_ = json.Unmarshal(call("GET", "catalog/categories?page_size=100", nil, 200), &categories)
		previous := map[string]catalog.Category{}
		for _, item := range categories.Items {
			previous[item.ID] = item
		}
		breakfast := catalog.Category{Metadata: catalog.Metadata{Name: "Desayunos", Active: true}, Position: at("")}
		_ = json.Unmarshal(call("POST", "catalog/categories", breakfast, 200), &breakfast)
		if breakfast.SortOrder != 10 || breakfast.Revision != 1 {
			t.Fatalf("la categoría al inicio no recibió el primer lugar: %+v", breakfast)
		}
		_ = json.Unmarshal(call("GET", "catalog/categories?page_size=100", nil, 200), &categories)
		if categories.Items[0].ID != breakfast.ID {
			t.Fatalf("el listado no respeta la nueva posición: %+v", categories.Items)
		}
		for index, item := range categories.Items {
			if item.SortOrder != (index+1)*10 {
				t.Fatalf("la renumeración no usa múltiplos de 10: %+v", categories.Items)
			}
			if old, exists := previous[item.ID]; exists && old.SortOrder != item.SortOrder && item.Revision != old.Revision+1 {
				t.Fatalf("un hermano movido no incrementó su revisión: antes %+v después %+v", old, item)
			}
		}
		drinks := catalog.Category{Metadata: catalog.Metadata{Name: "Bebidas", Active: true}, Position: at(breakfast.ID)}
		_ = json.Unmarshal(call("POST", "catalog/categories", drinks, 200), &drinks)
		if drinks.SortOrder != 20 {
			t.Fatalf("«después de Desayunos» no quedó en segundo lugar: %+v", drinks)
		}
		// Guardar sin moverse no renumera ni cambia la revisión de los hermanos.
		platillos := readCategory(category.ID)
		drinks.Position = at(breakfast.ID)
		drinks.Description = "Frías y calientes"
		_ = json.Unmarshal(call("PATCH", "catalog/categories/"+drinks.ID, drinks, 200), &drinks)
		if readCategory(category.ID).Revision != platillos.Revision || drinks.SortOrder != 20 {
			t.Fatal("guardar en la misma posición alteró a los hermanos")
		}
		drinks.Position = at(drinks.ID)
		problem := string(call("PATCH", "catalog/categories/"+drinks.ID, drinks, 400))
		if !strings.Contains(problem, "sí mismo") {
			t.Fatalf("colocarse después de sí mismo no se explicó: %s", problem)
		}
		drinks.Position = at("00000000-0000-4000-8000-000000000061")
		call("PATCH", "catalog/categories/"+drinks.ID, drinks, 400)
		drinks.Position = &ordering.Position{}
		call("PATCH", "catalog/categories/"+drinks.ID, drinks, 400)
		drinks.Position = nil

		coffee := catalog.Product{
			Metadata:   catalog.Metadata{Name: "Café americano", Active: true},
			CategoryID: drinks.ID,
			Code:       "CAFE-61",
			Kind:       "product",
			BasePrice:  "30",
			Position:   at(""),
		}
		_ = json.Unmarshal(call("POST", "catalog/products", coffee, 200), &coffee)
		juice := catalog.Product{
			Metadata:   catalog.Metadata{Name: "Jugo de naranja", Active: true},
			CategoryID: drinks.ID,
			Code:       "JUGO-61",
			Kind:       "product",
			BasePrice:  "35",
			Position:   at(coffee.ID),
		}
		_ = json.Unmarshal(call("POST", "catalog/products", juice, 200), &juice)
		if coffee.SortOrder != 10 || juice.SortOrder != 20 {
			t.Fatalf("los productos no se numeraron dentro de su categoría: %d %d", coffee.SortOrder, juice.SortOrder)
		}
		// Los combos comparten la lista de su categoría.
		combo := catalog.Product{
			Metadata:   catalog.Metadata{Name: "Desayuno con bebida", Active: true},
			CategoryID: drinks.ID,
			Code:       "COMBO-DESAYUNO-61",
			Kind:       "combo",
			BasePrice:  "60",
			Position:   at(""),
			Slots: []catalog.ComboSlot{
				{
					Name:             "Bebida",
					MinQuantity:      1,
					MaxQuantity:      1,
					IncludedQuantity: 1,
					DefaultQuantity:  1,
					DefaultProductID: coffee.ID,
				},
			},
		}
		_ = json.Unmarshal(call("POST", "catalog/products", combo, 200), &combo)
		coffeeAfter := readProduct(coffee.ID)
		juiceAfter := readProduct(juice.ID)
		if combo.SortOrder != 10 || coffeeAfter.SortOrder != 20 || juiceAfter.SortOrder != 30 {
			t.Fatalf(

				"el combo no se ordenó con los productos: %d %d %d",
				combo.SortOrder,
				coffeeAfter.SortOrder,
				juiceAfter.SortOrder,
			)
		}
		if coffeeAfter.Revision != coffee.Revision+1 || juiceAfter.Revision != juice.Revision+1 {
			t.Fatal("los productos renumerados no incrementaron su revisión")
		}
		if readProduct(product.ID).SortOrder != product.SortOrder {
			t.Fatal("la renumeración alcanzó a productos de otra categoría")
		}
		var listed struct {
			Items []catalog.Product `json:"items"`
		}
		_ = json.Unmarshal(call("GET", "catalog/products?archived=all&category_id="+drinks.ID, nil, 200), &listed)
		if len(listed.Items) != 3 || listed.Items[0].ID != combo.ID || listed.Items[2].ID != juice.ID {
			t.Fatalf("el listado no respeta el orden de la categoría: %+v", listed.Items)
		}
		// Un hermano de otra categoría se rechaza y no deja cambios parciales.
		juiceAfter.Position = at(product.ID)
		problem = string(call("PATCH", "catalog/products/"+juice.ID, juiceAfter, 400))
		if !strings.Contains(problem, "misma lista") {
			t.Fatalf("el hermano de otra categoría no se explicó: %s", problem)
		}
		if readProduct(juice.ID).Revision != juiceAfter.Revision {
			t.Fatal("el rechazo dejó cambios parciales")
		}
		// Al cambiar de categoría con posición, el producto se coloca entre sus nuevos hermanos.
		molletes := catalog.Product{
			Metadata:   catalog.Metadata{Name: "Molletes", Active: true},
			CategoryID: breakfast.ID,
			Code:       "MOLLETES-61",
			Kind:       "product",
			BasePrice:  "70",
			Position:   at(""),
		}
		_ = json.Unmarshal(call("POST", "catalog/products", molletes, 200), &molletes)
		juiceAfter.CategoryID = breakfast.ID
		juiceAfter.Position = at("")
		var moved catalog.Product
		_ = json.Unmarshal(call("PATCH", "catalog/products/"+juice.ID, juiceAfter, 200), &moved)
		if moved.CategoryID != breakfast.ID || moved.SortOrder != 10 || moved.Revision != juiceAfter.Revision+1 {
			t.Fatalf("el cambio de categoría con posición no fue el esperado: %+v", moved)
		}
		if shifted := readProduct(molletes.ID); shifted.SortOrder != 20 || shifted.Revision != molletes.Revision+1 {
			t.Fatalf("el hermano de la nueva categoría no se renumeró: %+v", shifted)
		}
		if remaining := readProduct(coffee.ID); remaining.SortOrder != 20 || remaining.Revision != coffeeAfter.Revision {
			t.Fatal("la categoría anterior no debía renumerarse al retirar el producto")
		}
		var reorders int
		if err := pool.QueryRow(
			ctx,
			`SELECT count(*) FROM app.audit_events
            WHERE action IN ('catalog.categories.reordered','catalog.products.reordered')`,
		).Scan(&reorders); err != nil || reorders < 3 {
			t.Fatalf("la renumeración no quedó auditada: %d %v", reorders, err)
		}
	})
	run("tasa de IVA propia de lista cerrada, protegida y auditada", func(t *testing.T) {
		eight := 8
		taxed := catalog.Product{
			Metadata:   catalog.Metadata{Name: "Agua fronteriza", Active: true},
			CategoryID: category.ID, Code: "AGUA-8", Kind: "product", BasePrice: "20", TaxRate: &eight,
		}
		var saved catalog.Product
		_ = json.Unmarshal(call("POST", "catalog/products", taxed, 200), &saved)
		if saved.TaxRate == nil || *saved.TaxRate != 8 {
			t.Fatalf("La tasa propia no se guardó: %+v", saved.TaxRate)
		}
		for _, rate := range []int{15, -8, 1, 160} {
			invalidRate := rate
			candidate := taxed
			candidate.Code = "AGUA-INVALIDA"
			candidate.TaxRate = &invalidRate
			call("POST", "catalog/products", candidate, 400)
		}
		if _, err := pool.Exec(ctx, "UPDATE app.catalog_products SET tax_rate=15 WHERE id=$1", saved.ID); err == nil {
			t.Fatal("PostgreSQL aceptó una tasa fuera de la lista cerrada")
		}
		// Un duplicado conserva la tasa propia del original.
		var copied catalog.Product
		_ = json.Unmarshal(call("POST", "catalog/products/"+saved.ID+"/duplicate", map[string]any{
			"name": "Agua fronteriza copia", "code": "AGUA-8-C", "revision": saved.Revision,
		}, 200), &copied)
		if copied.TaxRate == nil || *copied.TaxRate != 8 {
			t.Fatal("El duplicado perdió la tasa propia")
		}
		// Cambiar la tasa exige el permiso de precios, como el precio; otros cambios no.
		var userID string
		if err := pool.QueryRow(ctx, "SELECT id FROM app.users WHERE username='admin'").Scan(&userID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO app.user_permissions(user_id,permission_key,effect) VALUES ($1,'catalog.prices.manage','deny')", userID); err != nil {
			t.Fatal(err)
		}
		inherit := saved
		inherit.TaxRate = nil
		call("PATCH", "catalog/products/"+saved.ID, inherit, 403)
		described := saved
		described.Description = "Sin cambio de tasa"
		_ = json.Unmarshal(call("PATCH", "catalog/products/"+saved.ID, described, 200), &saved)
		if _, err := pool.Exec(ctx, "DELETE FROM app.user_permissions WHERE user_id=$1 AND permission_key='catalog.prices.manage'", userID); err != nil {
			t.Fatal(err)
		}
		inherit = saved
		inherit.TaxRate = nil
		_ = json.Unmarshal(call("PATCH", "catalog/products/"+saved.ID, inherit, 200), &saved)
		if saved.TaxRate != nil {
			t.Fatal("El producto no volvió a heredar la tasa general")
		}
		var audited int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM app.audit_events WHERE action='catalog.products.updated'
            AND entity_id=$1 AND before_state->>'tax_rate'='8' AND after_state->'tax_rate'='null'::jsonb`, saved.ID).Scan(&audited); err != nil || audited != 1 {
			t.Fatalf("El cambio de tasa no quedó auditado con el guardado: %d %v", audited, err)
		}
	})
	run("captura táctil: categorías disponibles y búsqueda por nombre o código", func(t *testing.T) {
		// Etapa 6.1 fase D: los lookups indican qué categorías están disponibles ahora (activas y
		// dentro de su horario, con el reloj de Go) y el listado de productos busca también por
		// código interno.
		future := catalog.Category{
			Metadata:     catalog.Metadata{Name: "Temporada futura", Active: true},
			Availability: catalog.Schedule{StartsOn: "2999-01-01", Days: []int{}, Windows: []catalog.TimeWindow{}},
		}
		_ = json.Unmarshal(call("POST", "catalog/categories", future, 200), &future)
		paused := catalog.Category{Metadata: catalog.Metadata{Name: "Categoría inactiva", Active: false}}
		_ = json.Unmarshal(call("POST", "catalog/categories", paused, 200), &paused)
		var lookups struct {
			Categories []map[string]any `json:"categories"`
		}
		_ = json.Unmarshal(call("GET", "catalog/lookups", nil, 200), &lookups)
		availability := map[string]any{}
		for _, entry := range lookups.Categories {
			id, _ := entry["id"].(string)
			availability[id] = entry["available_now"]
		}
		if availability[category.ID] != true {
			t.Fatalf("una categoría sin restricciones no aparece disponible: %v", availability[category.ID])
		}
		if availability[future.ID] != false || availability[paused.ID] != false {
			t.Fatalf("una categoría fuera de horario o inactiva aparece disponible: %v %v", availability[future.ID], availability[paused.ID])
		}
		coded := catalog.Product{
			Metadata:   catalog.Metadata{Name: "Machacado norteño", Active: true},
			CategoryID: category.ID, Code: "MACH-77", Kind: "product", BasePrice: "95",
		}
		_ = json.Unmarshal(call("POST", "catalog/products", coded, 200), &coded)
		for _, search := range []string{"mach-77", "Machacado"} {
			var page struct {
				Items []catalog.Product `json:"items"`
				Total int               `json:"total"`
			}
			_ = json.Unmarshal(call("GET", "catalog/products?search="+search, nil, 200), &page)
			if page.Total != 1 || page.Items[0].ID != coded.ID {
				t.Fatalf("la búsqueda %q no encontró el producto por nombre o código: %+v", search, page)
			}
		}
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
