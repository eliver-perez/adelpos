//go:build integration

package operations_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/kitchen"
)

// Etapa 8, fase B: recetas, subrecetas, porciones habituales, costo teórico, elaboraciones y
// consumo por liberación a cocina. Se encadena al final del núcleo de inventario con su propio
// catálogo (categoría, productos, grupo, combo), estaciones, almacenes e insumos, para controlar
// cada cantidad.

type recipesFixture struct {
	*inventoryFixture
	items       map[string]inventory.Item
	warehouses  map[string]inventory.Warehouse
	stations    map[string]kitchen.Station
	printers    []kitchen.Printer
	category    catalog.Category
	ingredients map[string]catalog.Ingredient
	complements map[string]catalog.Complement
	options     map[string]catalog.GroupOption
	group       catalog.OptionGroup
	products    map[string]catalog.Product
	recipes     map[string]inventory.Recipe
	// recipeManager administra recetas de producto y complemento; subrecipeManager, preparaciones
	// y elaboraciones; wasteClerk clasifica cancelaciones hasta el corte.
	recipeManager    testTerminal
	subrecipeManager testTerminal
	wasteClerk       testTerminal
}

func testInventoryRecipesAndConsumption(t *testing.T, base *inventoryFixture) {
	fixture := &recipesFixture{
		inventoryFixture: base, items: map[string]inventory.Item{}, warehouses: map[string]inventory.Warehouse{},
		stations: map[string]kitchen.Station{}, ingredients: map[string]catalog.Ingredient{}, complements: map[string]catalog.Complement{},
		options: map[string]catalog.GroupOption{}, products: map[string]catalog.Product{}, recipes: map[string]inventory.Recipe{},
	}
	fixture.recipeManager = inventoryUser(t, base.restaurantFixture, "recetas_producto", []string{"inventory.read", "inventory.recipes.manage"})
	fixture.subrecipeManager = inventoryUser(t, base.restaurantFixture, "recetas_subrecetas", []string{
		"inventory.read", "inventory.subrecipes.manage", "inventory.production.record",
	})
	fixture.wasteClerk = inventoryUser(t, base.restaurantFixture, "inventario_mermas", []string{"inventory.read", "inventory.waste.record"})
	t.Run("inventory_b_setup", func(t *testing.T) {
		fixture.setupKitchenAndCatalog(t)
		fixture.setupItemsAndStock(t)
	})
	t.Run("inventory_b_portions", func(t *testing.T) {
		testIngredientPortions(t, fixture)
	})
	t.Run("inventory_b_recipes_versions_and_costs", func(t *testing.T) {
		testRecipesAndVersions(t, fixture)
	})
	t.Run("inventory_b_subrecipes_cycles_and_production", func(t *testing.T) {
		testSubrecipesAndProduction(t, fixture)
	})
	t.Run("inventory_b_recipe_targets", func(t *testing.T) {
		testRecipeTargets(t, fixture)
	})
	testConsumption(t, fixture)
}

// setupKitchenAndCatalog crea las estaciones con sus almacenes, las rutas y el catálogo propio.
func (fixture *recipesFixture) setupKitchenAndCatalog(t *testing.T) {
	for _, name := range []string{"Impresora inventario", "Impresora inventario respaldo"} {
		fixture.printers = append(fixture.printers, decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
			Name: name, Transport: "cups", Destination: "ADEL_TEST_ONLY", Active: true, PaperWidth: 80, Copies: 1, Port: 9100,
		}, 200)))
	}
	fixture.stations["cocina"] = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Inv Cocina", Active: true, Reception: "printer", PrinterID: &fixture.printers[0].ID, TargetMinutes: 12, WarningMinutes: 15,
		OverdueMinutes: 25, Grouping: "inherit", Tracking: "inherit",
	}, 200))
	fixture.stations["freidora"] = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Inv Freidora", Active: true, Reception: "kds", TargetMinutes: 8, WarningMinutes: 10, OverdueMinutes: 20,
		Grouping: "inherit", Tracking: "inherit",
	}, 200))
	for key, name := range map[string]string{"cocina": "Cocina B", "freidora": "Freidora B"} {
		fixture.warehouses[key] = decodeResponse[inventory.Warehouse](t, fixture.adminWrite(t, "POST", "inventory/warehouses",
			map[string]any{"name": name, "active": true}, 200))
		station := fixture.stations[key]
		current := decodeResponse[[]inventory.StationWarehouse](t, fixture.adminCall(t, "GET", "inventory/station-warehouses", nil, 200))
		revision := 0
		for _, entry := range current {
			if entry.StationID == station.ID {
				revision = entry.Revision
			}
		}
		fixture.adminWrite(t, "PUT", "inventory/station-warehouses/"+station.ID, map[string]any{
			"revision": revision, "warehouse_id": fixture.warehouses[key].ID,
		}, 200)
	}
	fixture.category = decodeResponse[catalog.Category](t, fixture.adminCall(t, "POST", "catalog/categories",
		catalog.Category{Metadata: catalog.Metadata{Name: "Inventario B", Active: true}}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare",
		StationIDs: []string{fixture.stations["cocina"].ID}}, 200)
	for _, name := range []string{"Lechuga", "Jalapeño"} {
		fixture.ingredients[name] = decodeResponse[catalog.Ingredient](t, fixture.adminCall(t, "POST", "catalog/ingredients",
			catalog.Ingredient{Metadata: catalog.Metadata{Name: name, Active: true}, Unit: "porción"}, 200))
	}
	// «Cebolla» ya existe en el catálogo de la aceptación.
	fixture.ingredients["Cebolla"] = catalog.Ingredient{Metadata: catalog.Metadata{ID: fixture.product.Ingredients[0].IngredientID, Name: "Cebolla"}}
	product := func(name, code, price string, ingredients []catalog.ProductIngredient, groups []catalog.ProductGroup) catalog.Product {
		return decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
			Metadata: catalog.Metadata{Name: name, Active: true}, CategoryID: fixture.category.ID, Code: code, Kind: "product",
			BasePrice: price, Ingredients: ingredients, Groups: groups,
		}, 200))
	}
	fixture.products["hamburguesa"] = product("Hamburguesa B", "INV-HAMB", "100", []catalog.ProductIngredient{
		{IngredientID: fixture.ingredients["Cebolla"].ID, IncludedQuantity: "1", Removable: true, Addable: true, MaxQuantity: "3",
			ExtraPrice: "5", RecipeQuantity: "0"},
		{IngredientID: fixture.ingredients["Lechuga"].ID, IncludedQuantity: "1", Removable: true, Addable: true, MaxQuantity: "2",
			ExtraPrice: "0", RecipeQuantity: "0"},
		{IngredientID: fixture.ingredients["Jalapeño"].ID, IncludedQuantity: "0", Removable: false, Addable: true, MaxQuantity: "2",
			ExtraPrice: "0", RecipeQuantity: "0"},
	}, nil)
	fixture.products["papas"] = product("Papas producto B", "INV-PAPAS", "40", nil, nil)
	fixture.products["refresco"] = product("Refresco B", "INV-REF", "25", nil, nil)
	fixture.products["enchiladas"] = product("Enchiladas B", "INV-ENCH", "90", nil, nil)
	friesID, sodaID := fixture.products["papas"].ID, fixture.products["refresco"].ID
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &friesID, Mode: "prepare",
		StationIDs: []string{fixture.stations["freidora"].ID}}, 200)
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &sodaID, Mode: "none"}, 200)
	// Guarniciones del ejemplo del propietario (equivalencia por valor) y una porción «Media».
	half := decodeResponse[catalog.Portion](t, fixture.adminCall(t, "POST", "catalog/portions",
		catalog.Portion{Metadata: catalog.Metadata{Name: "Media B", Active: true}, Factor: "0.5"}, 200))
	fixture.group = catalog.OptionGroup{Metadata: catalog.Metadata{Name: "Guarniciones B", Active: true}, PricingMode: "value", MinQuantity: "0"}
	for _, entry := range []struct{ name, value string }{{"Arroz B", "30"}, {"Ensalada B", "25"}, {"Papas B", "35"}, {"Pasta B", "20"}} {
		fixture.complements[entry.name] = decodeResponse[catalog.Complement](t, fixture.adminCall(t, "POST", "catalog/complements",
			catalog.Complement{Metadata: catalog.Metadata{Name: entry.name, Active: true}}, 200))
		fixture.group.Options = append(fixture.group.Options, catalog.GroupOption{
			ComplementID: fixture.complements[entry.name].ID, ReferenceValue: entry.value, ExtraPrice: "0", Active: true,
		})
	}
	fixture.group.Options = append(fixture.group.Options, catalog.GroupOption{
		ComplementID: fixture.complements["Arroz B"].ID, PortionID: &half.ID, ReferenceValue: "15", ExtraPrice: "0", Active: true,
	})
	fixture.group = decodeResponse[catalog.OptionGroup](t, fixture.adminCall(t, "POST", "catalog/groups", fixture.group, 200))
	for _, option := range fixture.group.Options {
		key := option.Name
		if option.PortionID != nil {
			key += " media"
		}
		fixture.options[key] = option
	}
	fixture.products["filete"] = product("Filete B", "INV-FILETE", "140", nil, []catalog.ProductGroup{{
		GroupID: fixture.group.ID,
		Included: []catalog.IncludedOption{
			{OptionID: fixture.options["Arroz B"].ID, Quantity: "1"}, {OptionID: fixture.options["Ensalada B"].ID, Quantity: "1"},
		},
	}})
	// El filete se prepara en dos estaciones: su consumo se calcula una sola vez.
	fileteID := fixture.products["filete"].ID
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &fileteID, Mode: "prepare",
		StationIDs: []string{fixture.stations["cocina"].ID, fixture.stations["freidora"].ID}}, 200)
	slot := func(name, productID string) catalog.ComboSlot {
		return catalog.ComboSlot{Name: name, MinQuantity: 1, MaxQuantity: 1, IncludedQuantity: 1, DefaultQuantity: 1, DefaultProductID: productID}
	}
	fixture.products["combo"] = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Combo B", Active: true}, CategoryID: fixture.category.ID, Code: "INV-COMBO", Kind: "combo",
		BasePrice: "150", Slots: []catalog.ComboSlot{
			slot("Principal", fixture.products["hamburguesa"].ID), slot("Papas", fixture.products["papas"].ID),
			slot("Bebida", fixture.products["refresco"].ID),
		},
	}, 200))
}

// setupItemsAndStock da de alta los insumos de la fase B con su existencia inicial.
func (fixture *recipesFixture) setupItemsAndStock(t *testing.T) {
	units := fixture.units
	items := []struct {
		key, code, name, unit, kind string
		reference                   any
	}{
		{"pan", "B-PAN", "Pan B", "pza", "purchased", nil},
		{"carne", "B-CARNE", "Carne B", "g", "purchased", nil},
		{"queso", "B-QUESO", "Queso B", "pza", "purchased", nil},
		{"cebolla", "B-CEBOLLA", "Cebolla B", "g", "purchased", nil},
		{"lechuga", "B-LECHUGA", "Lechuga B", "g", "purchased", nil},
		{"bolsa", "B-BOLSA", "Bolsa para llevar B", "pza", "purchased", "2"},
		{"filete", "B-FILETE", "Filete de res B", "g", "purchased", nil},
		{"arroz", "B-ARROZ", "Arroz crudo B", "g", "purchased", nil},
		{"pasta", "B-PASTA", "Pasta B", "g", "purchased", nil},
		{"papa", "B-PAPA", "Papa B", "g", "purchased", nil},
		{"lata", "B-LATA", "Refresco en lata B", "pza", "purchased", nil},
		{"charola", "B-CHAROLA", "Charola B", "pza", "purchased", nil},
		{"tomate", "B-TOMATE", "Tomate B", "g", "purchased", nil},
		{"chile", "B-CHILE", "Chile B", "g", "purchased", nil},
		{"tortilla", "B-TORTILLA", "Tortilla B", "pza", "purchased", nil},
		{"arroz_rojo", "B-ARROZROJO", "Arroz rojo preparado B", "g", "prepared", nil},
	}
	for _, entry := range items {
		body := map[string]any{"code": entry.code, "name": entry.name, "base_unit_id": units[entry.unit].ID, "kind": entry.kind, "active": true}
		if entry.reference != nil {
			body["reference_cost"] = entry.reference
		}
		fixture.items[entry.key] = fixture.createItem(t, body)
	}
	stock := func(warehouse string, lines ...map[string]any) {
		fixture.adminWrite(t, "POST", "inventory/initial-balances", map[string]any{
			"warehouse_id": fixture.warehouses[warehouse].ID, "lines": lines,
		}, 200)
	}
	line := func(key, quantity, cost string) map[string]any {
		return map[string]any{"item_id": fixture.items[key].ID, "quantity": quantity, "unit_cost": cost}
	}
	// Valores: pan 300, carne 2000, queso 10 (2 piezas para probar saldo positivo, cero y negativo),
	// cebolla 60, lechuga 100, filete 1250, arroz 250, pasta 120, lata 288, charola 50, tomate 150,
	// chile 100 y tortilla 300.
	stock("cocina", line("pan", "100", "3"), line("carne", "10000", "0.2"), line("queso", "2", "5"), line("cebolla", "2000", "0.03"),
		line("lechuga", "2000", "0.05"), line("filete", "5000", "0.25"), line("arroz", "10000", "0.025"), line("pasta", "3000", "0.04"),
		line("lata", "24", "12"), line("charola", "50", "1"), line("tomate", "5000", "0.03"), line("chile", "1000", "0.1"),
		line("tortilla", "300", "1"))
	stock("freidora", line("papa", "10000", "0.02"))
}

// recipeLine arma un renglón de receta para el cuerpo JSON.
func recipeItemLine(item inventory.Item, quantity string, unit inventory.Unit, takeawayOnly bool) map[string]any {
	return map[string]any{"component_kind": "item", "item_id": item.ID, "quantity": quantity, "unit_id": unit.ID, "takeaway_only": takeawayOnly}
}

func (fixture *recipesFixture) saveRecipe(t *testing.T, terminal testTerminal, key string, body map[string]any, status int) inventory.Recipe {
	t.Helper()
	response := fixture.write(t, terminal, "POST", "inventory/recipes", body, status)
	if status != 200 {
		return inventory.Recipe{}
	}
	recipe := decodeResponse[inventory.Recipe](t, response)
	fixture.recipes[key] = recipe
	return recipe
}

func (fixture *recipesFixture) readRecipe(t *testing.T, terminal testTerminal, id string) inventory.Recipe {
	t.Helper()
	return decodeResponse[inventory.Recipe](t, fixture.call(t, terminal, "GET", "inventory/recipes/"+id, nil, 200))
}

func testIngredientPortions(t *testing.T, fixture *recipesFixture) {
	units := fixture.units
	portion := func(name string) inventory.IngredientPortion {
		page := decodeResponse[inventory.IngredientPortionPage](t, fixture.adminCall(t, "GET",
			"inventory/ingredient-portions?search="+strings.ReplaceAll(name, " ", "%20"), nil, 200))
		for _, entry := range page.Items {
			if entry.IngredientName == name {
				return *entry
			}
		}
		t.Fatalf("No se listó el ingrediente %s", name)
		return inventory.IngredientPortion{}
	}
	onion := portion("Cebolla")
	if onion.Configured || onion.Revision != 0 || len(onion.History) != 0 {
		t.Fatalf("Porción inicial de la cebolla: %+v", onion)
	}
	route := "inventory/ingredient-portions/" + onion.IngredientID
	// 0.02 kg = 20 g de cebolla por porción (conversión a la unidad base del insumo).
	saved := decodeResponse[inventory.IngredientPortion](t, fixture.write(t, fixture.recipeManager, "PUT", route, map[string]any{
		"revision": 0, "item_id": fixture.items["cebolla"].ID, "quantity": "0.02", "unit_id": units["kg"].ID,
	}, 200))
	if !saved.Configured || saved.BaseQuantity == nil || *saved.BaseQuantity != "20" || saved.Revision != 1 || saved.Item.ID != fixture.items["cebolla"].ID {
		t.Fatalf("Porción guardada: %+v", saved)
	}
	// Guardar lo mismo no agrega historial; una revisión obsoleta se rechaza.
	same := decodeResponse[inventory.IngredientPortion](t, fixture.write(t, fixture.recipeManager, "PUT", route, map[string]any{
		"revision": 1, "item_id": fixture.items["cebolla"].ID, "quantity": "0.02", "unit_id": units["kg"].ID,
	}, 200))
	if same.Revision != 1 || len(same.History) != 1 {
		t.Fatalf("Repetir la porción vigente agregó historial: %+v", same)
	}
	fixture.write(t, fixture.recipeManager, "PUT", route, map[string]any{"revision": 0, "item_id": fixture.items["cebolla"].ID, "quantity": "25"}, 409)
	// Otra dimensión, cantidad no positiva e insumo con unidad incompatible se rechazan.
	fixture.write(t, fixture.recipeManager, "PUT", route, map[string]any{"revision": 1, "item_id": fixture.items["cebolla"].ID, "quantity": "1",
		"unit_id": units["ml"].ID}, 400)
	fixture.write(t, fixture.recipeManager, "PUT", route, map[string]any{"revision": 1, "item_id": fixture.items["cebolla"].ID, "quantity": "0"}, 400)
	// Sin inventory.recipes.manage no se escribe.
	fixture.write(t, fixture.subrecipeManager, "PUT", route, map[string]any{"revision": 1, "item_id": fixture.items["cebolla"].ID,
		"quantity": "30"}, 403)
	// Retirar y volver a configurar deja historial de solo inserción.
	removed := decodeResponse[inventory.IngredientPortion](t, fixture.write(t, fixture.recipeManager, "PUT", route,
		map[string]any{"revision": 1, "item_id": nil}, 200))
	if removed.Configured || removed.Revision != 2 {
		t.Fatalf("Retirar la porción: %+v", removed)
	}
	restored := decodeResponse[inventory.IngredientPortion](t, fixture.write(t, fixture.recipeManager, "PUT", route, map[string]any{
		"revision": 2, "item_id": fixture.items["cebolla"].ID, "quantity": "20",
	}, 200))
	if restored.Revision != 3 || len(restored.History) != 3 || *restored.BaseQuantity != "20" {
		t.Fatalf("Historial de la porción: %+v", restored)
	}
	lettuce := portion("Lechuga")
	fixture.write(t, fixture.recipeManager, "PUT", "inventory/ingredient-portions/"+lettuce.IngredientID, map[string]any{
		"revision": 0, "item_id": fixture.items["lechuga"].ID, "quantity": "15",
	}, 200)
	configured := decodeResponse[inventory.IngredientPortionPage](t, fixture.adminCall(t, "GET",
		"inventory/ingredient-portions?configured=true", nil, 200))
	names := []string{}
	for _, entry := range configured.Items {
		names = append(names, entry.IngredientName)
	}
	if !slices.Contains(names, "Cebolla") || !slices.Contains(names, "Lechuga") || slices.Contains(names, "Jalapeño") {
		t.Fatalf("Ingredientes con porción: %v", names)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.inventory_ingredient_portions SET base_quantity=1"); err == nil {
		t.Fatal("El historial de porciones aceptó una modificación")
	}
}

func testRecipesAndVersions(t *testing.T, fixture *recipesFixture) {
	units, items, products := fixture.units, fixture.items, fixture.products
	cursor := eventCursor(t, fixture.restaurantFixture)
	burger := map[string]any{
		"owner_kind": "product", "product_id": products["hamburguesa"].ID, "active": true,
		"lines": []map[string]any{
			recipeItemLine(items["pan"], "1", units["pza"], false),
			// 0.15 kg = 150 g de carne.
			recipeItemLine(items["carne"], "0.15", units["kg"], false),
			recipeItemLine(items["queso"], "1", units["pza"], false),
			recipeItemLine(items["bolsa"], "1", units["pza"], true),
		},
	}
	// Una receta de producto no la crea quien solo administra subrecetas, ni nadie sin permiso.
	fixture.saveRecipe(t, fixture.subrecipeManager, "", burger, 403)
	fixture.saveRecipe(t, fixture.stock, "", burger, 403)
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_recipes") != 0 {
		t.Fatal("Una receta rechazada por permisos quedó registrada")
	}
	recipe := fixture.saveRecipe(t, fixture.recipeManager, "hamburguesa", burger, 200)
	if recipe.Name != "Hamburguesa B" || recipe.Revision != 1 || recipe.Version.Version != 1 || len(recipe.Version.Lines) != 4 ||
		recipe.Version.YieldQuantity != "1" || recipe.Version.YieldUnit.Code != "porcion" || recipe.TargetName != "Hamburguesa B" {
		t.Fatalf("Receta creada: %+v", recipe.RecipeSummary)
	}
	if line := recipe.Version.Lines[1]; line.Quantity != "0.15" || line.Unit.Code != "kg" || line.BaseQuantity != "150" ||
		line.BaseUnit.Code != "g" || !recipe.Version.Lines[3].TakeawayOnly {
		t.Fatalf("Conversión del renglón: %+v", line)
	}
	if len(recipe.Ingredients) != 3 || recipe.Ingredients[0].Name != "Cebolla" || recipe.Ingredients[0].PortionSource != "habitual" ||
		*recipe.Ingredients[0].IncludedQuantity != "20" || recipe.Ingredients[1].PortionSource != "none" {
		t.Fatalf("Ingredientes del producto en la receta: %+v", recipe.Ingredients)
	}
	// Quien administra recetas sin consultar costos no los ve.
	if recipe.Cost.CostsVisible || recipe.Cost.UnitCost != nil || recipe.Version.Lines[0].Cost != nil {
		t.Fatalf("Costos visibles sin inventory.costs.read: %+v", recipe.Cost)
	}
	// Costo teórico (con costs.read): pan 3 + carne 150 × 0.2 + queso 5 + cebolla 20 × 0.03 + lechuga
	// 15 × 0.05 = 39.35; la bolsa (solo para llevar) a su costo de referencia de $2 aparte.
	withCosts := fixture.readRecipe(t, fixture.admin, recipe.ID)
	if !withCosts.Cost.CostsVisible || *withCosts.Cost.UnitCost != "39.35" || *withCosts.Cost.TakeawayCost != "2.00" ||
		*withCosts.Cost.CostPending || *withCosts.Version.Lines[1].Cost != "30.00" || *withCosts.Ingredients[0].Cost != "0.60" {
		t.Fatalf("Costo teórico: %+v %+v", withCosts.Cost, withCosts.Ingredients)
	}
	stockView := fixture.readRecipe(t, fixture.costs, recipe.ID)
	if !stockView.Cost.CostsVisible || *stockView.Cost.UnitCost != "39.35" {
		t.Fatalf("Costo teórico con inventory.costs.read: %+v", stockView.Cost)
	}
	// Rechazos: destino duplicado, producto o variante ajenos, unidad de otra dimensión, renglón de
	// ingrediente fuera de una receta de producto y «solo para llevar» en un ingrediente.
	fixture.saveRecipe(t, fixture.recipeManager, "", burger, 409)
	missing := map[string]any{"owner_kind": "product", "product_id": randomID(), "active": true, "lines": burger["lines"]}
	fixture.saveRecipe(t, fixture.recipeManager, "", missing, 404)
	foreign := map[string]any{"owner_kind": "product", "product_id": products["hamburguesa"].ID, "variant_id": fixture.product.Variants[0].ID,
		"active": true, "lines": burger["lines"]}
	fixture.saveRecipe(t, fixture.recipeManager, "", foreign, 404)
	wrongUnit := map[string]any{"owner_kind": "product", "product_id": products["filete"].ID, "active": true,
		"lines": []map[string]any{recipeItemLine(items["filete"], "180", units["ml"], false)}}
	fixture.saveRecipe(t, fixture.recipeManager, "", wrongUnit, 400)
	complementWithIngredient := map[string]any{"owner_kind": "complement", "complement_id": fixture.complements["Arroz B"].ID, "active": true,
		"lines": []map[string]any{{"component_kind": "ingredient", "ingredient_id": fixture.ingredients["Cebolla"].ID,
			"item_id": items["cebolla"].ID, "quantity": "20"}}}
	fixture.saveRecipe(t, fixture.recipeManager, "", complementWithIngredient, 400)
	takeawayIngredient := map[string]any{"owner_kind": "product", "product_id": products["filete"].ID, "active": true,
		"lines": []map[string]any{{"component_kind": "ingredient", "ingredient_id": fixture.ingredients["Cebolla"].ID,
			"item_id": items["cebolla"].ID, "quantity": "20", "takeaway_only": true}}}
	fixture.saveRecipe(t, fixture.recipeManager, "", takeawayIngredient, 400)
	fixture.saveRecipe(t, fixture.recipeManager, "", map[string]any{"owner_kind": "product", "product_id": products["filete"].ID,
		"active": true, "lines": []map[string]any{}}, 400)
	// Modificar crea la versión 2 y conserva la 1 (versionado obligatorio, §7).
	update := map[string]any{"revision": 1, "name": "Hamburguesa sencilla B", "active": true, "lines": []map[string]any{
		recipeItemLine(items["pan"], "1", units["pza"], false),
		recipeItemLine(items["carne"], "180", units["g"], false),
		recipeItemLine(items["queso"], "1", units["pza"], false),
		recipeItemLine(items["bolsa"], "1", units["pza"], true),
	}}
	fixture.write(t, fixture.stock, "PUT", "inventory/recipes/"+recipe.ID, update, 403)
	second := decodeResponse[inventory.Recipe](t, fixture.write(t, fixture.recipeManager, "PUT", "inventory/recipes/"+recipe.ID, update, 200))
	if second.Revision != 2 || second.Version.Version != 2 || second.Name != "Hamburguesa sencilla B" || len(second.Versions) != 2 ||
		second.Version.Lines[1].BaseQuantity != "180" {
		t.Fatalf("Versión 2: %+v", second.RecipeSummary)
	}
	fixture.write(t, fixture.recipeManager, "PUT", "inventory/recipes/"+recipe.ID, update, 409)
	changed := map[string]any{"revision": 2, "owner_kind": "complement", "active": true, "lines": update["lines"]}
	fixture.write(t, fixture.recipeManager, "PUT", "inventory/recipes/"+recipe.ID, changed, 409)
	first := decodeResponse[inventory.RecipeVersionDetail](t, fixture.adminCall(t, "GET", "inventory/recipe-versions/"+recipe.Version.ID, nil, 200))
	if first.Version != 1 || first.Lines[1].BaseQuantity != "150" || first.Name != "Hamburguesa B" {
		t.Fatalf("La versión 1 no se conservó: %+v", first)
	}
	// Se regresa a 150 g en una versión 3 (los consumos usan la vigente a cada liberación).
	update["revision"] = 2
	update["lines"].([]map[string]any)[1] = recipeItemLine(items["carne"], "150", units["g"], false)
	third := decodeResponse[inventory.Recipe](t, fixture.write(t, fixture.recipeManager, "PUT", "inventory/recipes/"+recipe.ID, update, 200))
	fixture.recipes["hamburguesa"] = third
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.inventory_recipe_lines SET quantity=1"); err == nil {
		t.Fatal("Los renglones de una versión aceptaron una modificación")
	}
	// Recetas de los demás productos y complementos.
	productRecipe := func(key string, lines ...map[string]any) {
		fixture.saveRecipe(t, fixture.recipeManager, key, map[string]any{"owner_kind": "product", "product_id": products[key].ID,
			"active": true, "lines": lines}, 200)
	}
	productRecipe("filete", recipeItemLine(items["filete"], "180", units["g"], false))
	productRecipe("papas", recipeItemLine(items["papa"], "120", units["g"], false))
	productRecipe("refresco", recipeItemLine(items["lata"], "1", units["pza"], false))
	productRecipe("combo", recipeItemLine(items["charola"], "1", units["pza"], false))
	for name, entry := range map[string]map[string]any{
		"Arroz B":    recipeItemLine(items["arroz"], "90", units["g"], false),
		"Ensalada B": recipeItemLine(items["lechuga"], "40", units["g"], false),
		"Pasta B":    recipeItemLine(items["pasta"], "100", units["g"], false),
		"Papas B":    recipeItemLine(items["papa"], "150", units["g"], false),
	} {
		fixture.saveRecipe(t, fixture.recipeManager, name, map[string]any{"owner_kind": "complement",
			"complement_id": fixture.complements[name].ID, "active": true, "lines": []map[string]any{entry}}, 200)
	}
	kinds := eventKinds(fixture.inventoryEvents(t, fixture.stock, cursor))
	if !slices.Contains(kinds, "inventory.recipes.changed") {
		t.Fatalf("Eventos de recetas: %v", kinds)
	}
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action IN ('inventory.recipe_created','inventory.recipe_versioned')") < 11 {
		t.Fatal("Las recetas no se auditaron")
	}
	listed := decodeResponse[inventory.RecipePage](t, fixture.adminCall(t, "GET", "inventory/recipes?owner_kind=complement", nil, 200))
	if listed.Total != 4 {
		t.Fatalf("Listado de recetas de complemento: %d", listed.Total)
	}
}

func testSubrecipesAndProduction(t *testing.T, fixture *recipesFixture) {
	units, items := fixture.units, fixture.items
	// Salsa (inmediata): 400 g de tomate y 50 g de chile rinden 500 ml.
	salsaBody := map[string]any{"owner_kind": "preparation", "preparation_mode": "immediate", "name": "Salsa de la casa B", "active": true,
		"yield_quantity": "0.5", "yield_unit_id": units["l"].ID, "lines": []map[string]any{
			recipeItemLine(items["tomate"], "400", units["g"], false), recipeItemLine(items["chile"], "50", units["g"], false),
		}}
	fixture.saveRecipe(t, fixture.recipeManager, "", salsaBody, 403)
	salsa := fixture.saveRecipe(t, fixture.subrecipeManager, "salsa", salsaBody, 200)
	if salsa.Version.YieldBaseQuantity != "500" || salsa.Version.EffectiveYield != "500" {
		t.Fatalf("Rendimiento de la salsa: %+v", salsa.Version)
	}
	salsaCost := fixture.readRecipe(t, fixture.admin, salsa.ID)
	if *salsaCost.Cost.BatchCost != "17.00" || *salsaCost.Cost.YieldUnitCost != "0.034" {
		t.Fatalf("Costo de la salsa: %+v", salsaCost.Cost)
	}
	// Una almacenable exige un insumo de tipo «Preparación almacenable».
	stored := map[string]any{"owner_kind": "preparation", "preparation_mode": "stored", "preparation_item_id": items["arroz"].ID,
		"active": true, "lines": []map[string]any{recipeItemLine(items["tomate"], "1", units["g"], false)}}
	fixture.saveRecipe(t, fixture.subrecipeManager, "", stored, 409)
	// Arroz rojo (almacenable): 1 kg de arroz + 100 ml de salsa rinden 2.5 kg con 4 % de merma.
	riceBody := map[string]any{"owner_kind": "preparation", "preparation_mode": "stored", "preparation_item_id": items["arroz_rojo"].ID,
		"active": true, "yield_quantity": "2.5", "yield_unit_id": units["kg"].ID, "production_waste_percent": "4", "lines": []map[string]any{
			recipeItemLine(items["arroz"], "1", units["kg"], false),
			{"component_kind": "preparation", "preparation_recipe_id": salsa.ID, "quantity": "100", "unit_id": units["ml"].ID},
		}}
	rice := fixture.saveRecipe(t, fixture.subrecipeManager, "arroz_rojo", riceBody, 200)
	if rice.Name != "Arroz rojo preparado B" || rice.Version.YieldBaseQuantity != "2500" || rice.Version.EffectiveYield != "2400" ||
		rice.Version.Lines[1].BaseQuantity != "100" || rice.Version.Lines[1].BaseUnit.Code != "ml" {
		t.Fatalf("Preparación almacenable: %+v", rice.Version)
	}
	// Ciclos: la salsa no puede usar el insumo que produce el arroz rojo (que usa la salsa), ni a sí
	// misma; el arroz rojo no puede consumir su propio insumo.
	cycle := map[string]any{"revision": 1, "active": true, "yield_quantity": "500", "yield_unit_id": units["ml"].ID, "lines": []map[string]any{
		recipeItemLine(items["tomate"], "400", units["g"], false), recipeItemLine(items["arroz_rojo"], "10", units["g"], false),
	}}
	if code := errorCode(t, fixture.write(t, fixture.subrecipeManager, "PUT", "inventory/recipes/"+salsa.ID, cycle, 409)); code != "RECIPE_CYCLE" {
		t.Fatalf("Ciclo a través del insumo producido: %s", code)
	}
	self := map[string]any{"revision": 1, "active": true, "yield_quantity": "500", "yield_unit_id": units["ml"].ID, "lines": []map[string]any{
		{"component_kind": "preparation", "preparation_recipe_id": salsa.ID, "quantity": "1"},
	}}
	if code := errorCode(t, fixture.write(t, fixture.subrecipeManager, "PUT", "inventory/recipes/"+salsa.ID, self, 409)); code != "RECIPE_CYCLE" {
		t.Fatalf("Preparación que se usa a sí misma: %s", code)
	}
	ownItem := map[string]any{"revision": 1, "active": true, "yield_quantity": "2.5", "yield_unit_id": units["kg"].ID, "lines": []map[string]any{
		recipeItemLine(items["arroz_rojo"], "1", units["g"], false),
	}}
	fixture.write(t, fixture.subrecipeManager, "PUT", "inventory/recipes/"+rice.ID, ownItem, 409)
	// La dimensión del rendimiento de una preparación no cambia.
	dimension := map[string]any{"revision": 1, "active": true, "yield_quantity": "500", "yield_unit_id": units["g"].ID,
		"lines": salsaBody["lines"]}
	fixture.write(t, fixture.subrecipeManager, "PUT", "inventory/recipes/"+salsa.ID, dimension, 409)
	// Una preparación usada por una receta activa no se desactiva.
	deactivate := map[string]any{"revision": 1, "active": false, "yield_quantity": "0.5", "yield_unit_id": units["l"].ID,
		"lines": salsaBody["lines"]}
	if code := errorCode(t, fixture.write(t, fixture.subrecipeManager, "PUT", "inventory/recipes/"+salsa.ID, deactivate, 409)); code != "PREPARATION_IN_USE" {
		t.Fatalf("Desactivar una preparación en uso: %s", code)
	}
	// Enchiladas: 3 tortillas, 100 ml de salsa (inmediata) y 200 g de arroz rojo (almacenable).
	fixture.saveRecipe(t, fixture.recipeManager, "enchiladas", map[string]any{"owner_kind": "product",
		"product_id": fixture.products["enchiladas"].ID, "active": true, "lines": []map[string]any{
			recipeItemLine(items["tortilla"], "3", units["pza"], false),
			{"component_kind": "preparation", "preparation_recipe_id": salsa.ID, "quantity": "100", "unit_id": units["ml"].ID},
			{"component_kind": "preparation", "preparation_recipe_id": rice.ID, "quantity": "200", "unit_id": units["g"].ID},
		}}, 200)
	// Costo teórico antes de elaborar: tortilla 3 + salsa 3.40; el arroz rojo aún no tiene costo.
	enchiladas := fixture.readRecipe(t, fixture.admin, fixture.recipes["enchiladas"].ID)
	if *enchiladas.Cost.UnitCost != "6.40" || !*enchiladas.Cost.CostPending {
		t.Fatalf("Costo teórico con costo pendiente: %+v", enchiladas.Cost)
	}
	salsaUses := fixture.readRecipe(t, fixture.admin, salsa.ID)
	if len(salsaUses.UsedBy) != 2 {
		t.Fatalf("Recetas que usan la salsa: %+v", salsaUses.UsedBy)
	}
	// Elaboración: 2 lotes de arroz rojo consumen 2000 g de arroz y 200 ml de salsa (160 g de
	// tomate y 20 g de chile): 50 + 4.80 + 2 = $56.80. Se esperan 4800 g (2 × 2400) y se producen
	// 4750 g: merma 250 g (5000 − 4750). El insumo preparado entra con el valor consumido.
	production := map[string]any{"recipe_id": rice.ID, "warehouse_id": fixture.warehouses["cocina"].ID, "batches": "2",
		"produced_quantity": "4750", "notes": "Lote de la mañana"}
	fixture.write(t, fixture.recipeManager, "POST", "inventory/productions", production, 403)
	operationID := randomID()
	production["operation_id"] = operationID
	result := decodeResponse[inventory.Production](t, fixture.call(t, fixture.subrecipeManager, "POST", "inventory/productions", production, 200))
	if result.ExpectedQuantity != "4800" || result.ProducedQuantity != "4750" || result.WasteQuantity != "250" || len(result.Lines) != 3 ||
		result.ConsumedValue != nil || result.Item.ID != items["arroz_rojo"].ID {
		t.Fatalf("Elaboración: %+v", result)
	}
	consumed := map[string]string{}
	for _, line := range result.Lines {
		consumed[line.Item.Code] = line.Quantity
	}
	if consumed["B-ARROZ"] != "2000" || consumed["B-TOMATE"] != "160" || consumed["B-CHILE"] != "20" {
		t.Fatalf("Consumo de la elaboración: %v", consumed)
	}
	// Reintento idempotente.
	again := decodeResponse[inventory.Production](t, fixture.call(t, fixture.subrecipeManager, "POST", "inventory/productions", production, 200))
	if again.ID != result.ID || rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_productions") != 1 {
		t.Fatal("La elaboración repetida se registró dos veces")
	}
	var value, quantity string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT value::text,quantity::text FROM app.inventory_movements
        WHERE source_kind='production' AND kind='production_in'`).Scan(&value, &quantity); err != nil {
		t.Fatal(err)
	}
	if !decimal.RequireFromString(value).Equal(decimal.RequireFromString("56.8")) || !decimal.RequireFromString(quantity).Equal(decimal.RequireFromString("4750")) {
		t.Fatalf("Entrada de la elaboración: %s por %s", quantity, value)
	}
	listed := decodeResponse[inventory.ProductionPage](t, fixture.adminCall(t, "GET", "inventory/productions", nil, 200))
	if listed.Total != 1 || listed.Items[0].ConsumedValue == nil || *listed.Items[0].ConsumedValue != "56.80" {
		t.Fatalf("Listado de elaboraciones: %+v", listed)
	}
	fixture.call(t, fixture.wasteClerk, "GET", "inventory/productions", nil, 403)
	checkLedgerConsistency(t, fixture.inventoryFixture)
}

func testRecipeTargets(t *testing.T, fixture *recipesFixture) {
	page := decodeResponse[inventory.RecipeTargetPage](t, fixture.adminCall(t, "GET",
		"inventory/recipe-targets?search=Inventario%20B&page_size=100", nil, 200))
	byName := map[string]inventory.RecipeTarget{}
	for _, target := range page.Items {
		byName[target.Name] = target
	}
	if byName["Hamburguesa B"].Status != "with_recipe" || byName["Refresco B"].Status != "with_recipe" ||
		byName["Hamburguesa B"].IngredientsWithPortion != 2 || byName["Hamburguesa B"].IngredientsTotal != 3 {
		t.Fatalf("Destinos con receta: %+v", byName)
	}
	without := decodeResponse[inventory.RecipeTargetPage](t, fixture.adminCall(t, "GET", "inventory/recipe-targets?state=without_recipe&kind=product",
		nil, 200))
	for _, target := range without.Items {
		if target.Status != "without_recipe" || target.Kind != "product" || target.RecipeID != nil && *target.RecipeActive {
			t.Fatalf("Filtro sin receta: %+v", target)
		}
	}
	complements := decodeResponse[inventory.RecipeTargetPage](t, fixture.adminCall(t, "GET", "inventory/recipe-targets?kind=complement&search=B",
		nil, 200))
	found := false
	for _, target := range complements.Items {
		if target.Name == "Pasta B" {
			found = target.Status == "with_recipe" && target.Configured
		}
	}
	if !found {
		t.Fatalf("Complemento con receta: %+v", complements.Items)
	}
}
