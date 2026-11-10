package inventory

import (
	"testing"

	"github.com/shopspring/decimal"
)

// Pruebas de la resolución del consumo (función pura, sin base de datos). Cada caso arma el
// desglose canónico de la partida tal como lo guarda Go en quote_line y comprueba el consumo por
// (insumo, almacén) con cantidades exactas.

type mapConfig struct {
	products     map[string]*recipeVersion
	complements  map[string]*recipeVersion
	preparations map[string]*recipeVersion
	portions     map[string]*ingredientPortion
}

func (config mapConfig) productRecipe(productID string, variantID *string) (*recipeVersion, error) {
	if variantID != nil {
		if version := config.products[productID+"/"+*variantID]; version != nil && version.Active {
			return version, nil
		}
	}
	return config.products[productID], nil
}

func (config mapConfig) complementRecipe(complementID string) (*recipeVersion, error) {
	return config.complements[complementID], nil
}

func (config mapConfig) preparation(recipeID string) (*recipeVersion, error) {
	return config.preparations[recipeID], nil
}

func (config mapConfig) portion(ingredientID string) (*ingredientPortion, error) {
	return config.portions[ingredientID], nil
}

func d(text string) decimal.Decimal {
	return decimal.RequireFromString(text)
}

func item(id, quantity string) recipeLine {
	return recipeLine{Kind: lineItem, ItemID: id, BaseQuantity: d(quantity)}
}

func takeaway(id, quantity string) recipeLine {
	return recipeLine{Kind: lineItem, ItemID: id, BaseQuantity: d(quantity), TakeawayOnly: true}
}

func prep(id, quantity string) recipeLine {
	return recipeLine{Kind: linePreparation, PreparationRecipeID: id, BaseQuantity: d(quantity)}
}

func override(ingredientID, itemID, quantity string) recipeLine {
	return recipeLine{Kind: lineIngredient, IngredientID: ingredientID, ItemID: itemID, BaseQuantity: d(quantity)}
}

func recipe(lines ...recipeLine) *recipeVersion {
	return &recipeVersion{Active: true, YieldBase: d("1"), WastePercent: zero, Lines: lines}
}

// burgerConfig es la hamburguesa del ejemplo de §7: 1 pan, 150 g de carne, 1 rebanada de queso y
// los ingredientes del catálogo cebolla (20 g por porción) y lechuga (15 g por porción).
func burgerConfig() mapConfig {
	return mapConfig{
		products: map[string]*recipeVersion{
			"hamburguesa": recipe(item("pan", "1"), item("carne", "150"), item("queso", "1")),
		},
		complements:  map[string]*recipeVersion{},
		preparations: map[string]*recipeVersion{},
		portions: map[string]*ingredientPortion{
			"cebolla": {ItemID: "cebolla-g", BaseQuantity: d("20")},
			"lechuga": {ItemID: "lechuga-g", BaseQuantity: d("15")},
		},
	}
}

func burger(onion, lettuce string) unitBreakdown {
	return unitBreakdown{ProductID: "hamburguesa", Ingredients: []ingredientBreakdown{
		{IngredientID: "cebolla", Requested: onion},
		{IngredientID: "lechuga", Requested: lettuce},
	}}
}

var generalRouting = unitRouting{DefaultWarehouse: "general"}

func resolveFor(t *testing.T, breakdown unitBreakdown, snapshot pricingSnapshot, mode string, config consumptionConfig,
	routing unitRouting) *resolution {
	t.Helper()
	result, err := resolveUnit(breakdown, snapshot, mode, config, routing)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// expectConsumption compara el consumo redondeado con lo esperado (insumo@almacén → cantidad).
func expectConsumption(t *testing.T, result *resolution, expected map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, entry := range result.entries() {
		got[entry.Key.ItemID+"@"+entry.Key.WarehouseID] = entry.Quantity.String()
	}
	if len(got) != len(expected) {
		t.Fatalf("consumo %v, esperado %v", got, expected)
	}
	for key, quantity := range expected {
		if got[key] != quantity {
			t.Fatalf("consumo de %s: %s, esperado %s (todo: %v)", key, got[key], quantity, got)
		}
	}
}

func hasDiagnostic(result *resolution, code string) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func TestResolutionNormalProductAndPortions(t *testing.T) {
	config := burgerConfig()
	cases := []struct {
		name    string
		onion   string
		onionG  string
		lettuce string
	}{
		// Producto normal: lo incluido (1 porción de cada ingrediente).
		{"normal", "1", "20", "1"},
		// «Sin cebolla» → 0 porciones: no se consume cebolla.
		{"sin cebolla", "0", "", "1"},
		// «Con cebolla» en un producto que ya la incluye → 1 porción, no 2.
		{"con cebolla", "1", "20", "1"},
		// «Cebolla extra» con 1 incluida → 2 porciones (el botón «Extra» fija incluido + 1).
		{"cebolla extra", "2", "40", "1"},
		// Cantidades del catálogo: 3 porciones y media porción.
		{"tres porciones", "3", "60", "0.5"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := resolveFor(t, burger(testCase.onion, testCase.lettuce), pricingSnapshot{}, "dine_in", config, generalRouting)
			expected := map[string]string{"pan@general": "1", "carne@general": "150", "queso@general": "1"}
			if testCase.onionG != "" {
				expected["cebolla-g@general"] = testCase.onionG
			}
			lettuce := d(testCase.lettuce).Mul(d("15"))
			if lettuce.IsPositive() {
				expected["lechuga-g@general"] = lettuce.String()
			}
			expectConsumption(t, result, expected)
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnóstico inesperado: %+v", result.Diagnostics)
			}
		})
	}
}

func TestResolutionIgnoresFreeTextAndNeverInventsPortions(t *testing.T) {
	config := burgerConfig()
	// La observación libre «Sin cebolla» viaja en notes, que la resolución ni siquiera lee: con la
	// cebolla pedida (1) el consumo es el normal.
	encoded := []byte(`{"product_id":"hamburguesa","notes":"Sin cebolla","ingredients":[` +
		`{"ingredient_id":"cebolla","included":"1","requested":"1"},{"ingredient_id":"lechuga","included":"1","requested":"1"}],` +
		`"groups":null,"components":null}`)
	breakdown, err := decodeBreakdown(encoded)
	if err != nil {
		t.Fatal(err)
	}
	result := resolveFor(t, breakdown, pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{
		"pan@general": "1", "carne@general": "150", "queso@general": "1", "cebolla-g@general": "20", "lechuga-g@general": "15",
	})
	// Un ingrediente agregado sin porción de referencia no inventa consumo: se señala.
	added := burger("1", "1")
	added.Ingredients = append(added.Ingredients, ingredientBreakdown{IngredientID: "jalapeno", Requested: "1"})
	result = resolveFor(t, added, pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{
		"pan@general": "1", "carne@general": "150", "queso@general": "1", "cebolla-g@general": "20", "lechuga-g@general": "15",
	})
	if !hasDiagnostic(result, diagnosticIngredientNoPortion) {
		t.Fatalf("falta el diagnóstico de ingrediente sin porción: %+v", result.Diagnostics)
	}
	// Un producto sin receta ni porciones no consume nada y queda señalado.
	empty := resolveFor(t, unitBreakdown{ProductID: "refresco"}, pricingSnapshot{}, "dine_in", config, generalRouting)
	if len(empty.entries()) != 0 || !hasDiagnostic(empty, diagnosticNoConfiguration) {
		t.Fatalf("producto sin configuración: %v %+v", empty.entries(), empty.Diagnostics)
	}
}

func TestResolutionRecipeIngredientOverride(t *testing.T) {
	config := burgerConfig()
	// La receta de este producto usa 30 g de cebolla morada por porción en lugar de la habitual.
	config.products["hamburguesa"] = recipe(item("pan", "1"), override("cebolla", "cebolla-morada-g", "30"))
	result := resolveFor(t, burger("2", "0"), pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{"pan@general": "1", "cebolla-morada-g@general": "60"})
}

// filete es el ejemplo del propietario: el filete incluye arroz y ensalada; se puede cambiar por
// papas, frijoles, coditos o pasta.
func fileteConfig() (mapConfig, pricingSnapshot) {
	config := mapConfig{
		products: map[string]*recipeVersion{"filete": recipe(item("filete-g", "180"))},
		complements: map[string]*recipeVersion{
			"c-arroz":    recipe(item("arroz-g", "90")),
			"c-ensalada": recipe(item("lechuga-g", "40"), item("tomate-g", "30")),
			"c-pasta":    recipe(item("pasta-g", "100")),
		},
		preparations: map[string]*recipeVersion{},
		portions: map[string]*ingredientPortion{
			"papa": {ItemID: "papa-g", BaseQuantity: d("150")},
		},
	}
	snapshot := pricingSnapshot{
		Complements: map[string]snapshotComplement{
			"c-papas": {IngredientID: reference("papa")},
		},
		Portions: map[string]snapshotPortion{"media": {Factor: "0.500"}},
	}
	return config, snapshot
}

func filete(options ...optionSnapshot) unitBreakdown {
	return unitBreakdown{ProductID: "filete", Groups: []groupBreakdown{{GroupID: "guarniciones", Options: options}}}
}

func TestResolutionSubstitutedComplements(t *testing.T) {
	config, snapshot := fileteConfig()
	// Quitar arroz y poner pasta: consume pasta y ensalada aunque el precio no cambie.
	result := resolveFor(t, filete(
		optionSnapshot{OptionID: "o-arroz", ComplementID: "c-arroz", Requested: "0"},
		optionSnapshot{OptionID: "o-ensalada", ComplementID: "c-ensalada", Requested: "1"},
		optionSnapshot{OptionID: "o-pasta", ComplementID: "c-pasta", Requested: "1"},
	), snapshot, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{
		"filete-g@general": "180", "lechuga-g@general": "40", "tomate-g@general": "30", "pasta-g@general": "100",
	})
	// «2 porciones de papas» (complemento ligado al ingrediente papa): 2 × porción habitual.
	result = resolveFor(t, filete(
		optionSnapshot{OptionID: "o-arroz", ComplementID: "c-arroz", Requested: "0"},
		optionSnapshot{OptionID: "o-ensalada", ComplementID: "c-ensalada", Requested: "0"},
		optionSnapshot{OptionID: "o-papas", ComplementID: "c-papas", Requested: "2"},
	), snapshot, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{"filete-g@general": "180", "papa-g@general": "300"})
	// Factor de porción del catálogo: media porción de arroz × 3 = 1.5 recetas de arroz.
	media := "media"
	result = resolveFor(t, filete(optionSnapshot{OptionID: "o-arroz-media", ComplementID: "c-arroz", PortionID: &media, Requested: "3"}),
		snapshot, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{"filete-g@general": "180", "arroz-g@general": "135"})
	// Un complemento sin receta ni vínculo no consume nada y queda señalado.
	result = resolveFor(t, filete(optionSnapshot{OptionID: "o-otro", ComplementID: "c-sin-config", Requested: "1"}), snapshot,
		"dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{"filete-g@general": "180"})
	if !hasDiagnostic(result, diagnosticComplementNoConfig) {
		t.Fatalf("falta el diagnóstico del complemento: %+v", result.Diagnostics)
	}
}

func TestResolutionComplementLinkedToProduct(t *testing.T) {
	config := mapConfig{
		products: map[string]*recipeVersion{
			"filete":        recipe(item("filete-g", "180")),
			"papas-francia": recipe(item("papa-congelada-g", "120"), item("aceite-ml", "10")),
		},
		complements:  map[string]*recipeVersion{},
		preparations: map[string]*recipeVersion{},
		portions:     map[string]*ingredientPortion{"sal": {ItemID: "sal-g", BaseQuantity: d("1.5")}},
	}
	snapshot := pricingSnapshot{
		Complements: map[string]snapshotComplement{"c-papas": {ProductID: reference("papas-francia")}},
		Products: map[string]snapshotProduct{"papas-francia": {Ingredients: []struct {
			IngredientID     string `json:"ingredient_id"`
			IncludedQuantity string `json:"included_quantity"`
		}{{IngredientID: "sal", IncludedQuantity: "1"}}}},
	}
	// Sin receta propia del complemento: la configuración estándar del producto vinculado × 2.
	result := resolveFor(t, filete(optionSnapshot{OptionID: "o-papas", ComplementID: "c-papas", Requested: "2"}), snapshot, "dine_in",
		config, generalRouting)
	expectConsumption(t, result, map[string]string{
		"filete-g@general": "180", "papa-congelada-g@general": "240", "aceite-ml@general": "20", "sal-g@general": "3",
	})
	// La receta propia del complemento prevalece sobre la del producto vinculado.
	config.complements["c-papas"] = recipe(item("papa-congelada-g", "100"))
	result = resolveFor(t, filete(optionSnapshot{OptionID: "o-papas", ComplementID: "c-papas", Requested: "1"}), snapshot, "dine_in",
		config, generalRouting)
	expectConsumption(t, result, map[string]string{"filete-g@general": "180", "papa-congelada-g@general": "100"})
}

func TestResolutionTakeawayOnlyComponents(t *testing.T) {
	config := burgerConfig()
	config.products["hamburguesa"] = recipe(item("pan", "1"), takeaway("bolsa", "1"), takeaway("contenedor", "1"))
	dineIn := resolveFor(t, burger("0", "0"), pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, dineIn, map[string]string{"pan@general": "1"})
	// Para llevar (incluye ventanilla, que es takeaway con otro origen).
	toGo := resolveFor(t, burger("0", "0"), pricingSnapshot{}, "takeaway", config, generalRouting)
	expectConsumption(t, toGo, map[string]string{"pan@general": "1", "bolsa@general": "1", "contenedor@general": "1"})
}

func TestResolutionPreparationsAndYields(t *testing.T) {
	config := burgerConfig()
	// Salsa de la casa (inmediata): 400 g de tomate y 50 g de chile rinden 500 ml.
	salsa := recipe(item("tomate-g", "400"), item("chile-g", "50"))
	salsa.PreparationMode, salsa.YieldBase, salsa.RecipeID = preparationNow, d("500"), "salsa"
	// Arroz preparado (almacenable): se consume el insumo «arroz preparado», no sus ingredientes.
	rice := recipe(item("arroz-crudo-g", "1000"))
	rice.PreparationMode, rice.YieldBase, rice.RecipeID, rice.PreparationItemID = preparationStored, d("2500"), "arroz", "arroz-preparado-g"
	config.preparations["salsa"], config.preparations["arroz"] = salsa, rice
	config.products["hamburguesa"] = recipe(item("pan", "1"), prep("salsa", "50"), prep("arroz", "200"))
	result := resolveFor(t, burger("0", "0"), pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{
		"pan@general": "1", "tomate-g@general": "40", "chile-g@general": "5", "arroz-preparado-g@general": "200",
	})
	// Con 20 % de merma de elaboración, la salsa rinde 400 ml efectivos: 50 ml = 1/8 del lote.
	salsa.WastePercent = d("20")
	result = resolveFor(t, burger("0", "0"), pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{
		"pan@general": "1", "tomate-g@general": "50", "chile-g@general": "6.25", "arroz-preparado-g@general": "200",
	})
	// Rendimiento de la receta del producto: una receta de 3 porciones consume un tercio por unidad,
	// redondeado a seis decimales.
	pot := recipe(item("caldo-ml", "1000"))
	pot.YieldBase = d("3")
	config.products["sopa"] = pot
	result = resolveFor(t, unitBreakdown{ProductID: "sopa"}, pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{"caldo-ml@general": "333.333333"})
	// Una preparación desactivada no inventa consumo: se señala.
	salsa.Active = false
	result = resolveFor(t, burger("0", "0"), pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{"pan@general": "1", "arroz-preparado-g@general": "200"})
	if !hasDiagnostic(result, diagnosticPreparationNotActive) {
		t.Fatalf("falta el diagnóstico de la preparación: %+v", result.Diagnostics)
	}
	// Una preparación que se usa a sí misma (defensa: Go la rechaza al guardar) se corta.
	loop := recipe(item("agua-ml", "1"), prep("bucle", "1"))
	loop.PreparationMode, loop.YieldBase, loop.RecipeID = preparationNow, d("1"), "bucle"
	config.preparations["bucle"] = loop
	config.products["raro"] = recipe(prep("bucle", "1"))
	result = resolveFor(t, unitBreakdown{ProductID: "raro"}, pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{"agua-ml@general": "1"})
	if !hasDiagnostic(result, diagnosticPreparationCycle) {
		t.Fatalf("falta el diagnóstico del ciclo: %+v", result.Diagnostics)
	}
}

func TestResolutionVariantRecipePrevails(t *testing.T) {
	config := burgerConfig()
	grande := "grande"
	config.products["hamburguesa/grande"] = recipe(item("pan", "1"), item("carne", "250"))
	breakdown := burger("0", "0")
	breakdown.VariantID = &grande
	result := resolveFor(t, breakdown, pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{"pan@general": "1", "carne@general": "250"})
	// Desactivada la receta de la variante, se usa la del producto.
	config.products["hamburguesa/grande"].Active = false
	result = resolveFor(t, breakdown, pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{"pan@general": "1", "carne@general": "150", "queso@general": "1"})
}

func TestResolutionCombosAndWarehouses(t *testing.T) {
	config := burgerConfig()
	config.products["combo"] = recipe(item("charola", "1"))
	config.products["papas"] = recipe(item("papa-g", "150"))
	config.products["refresco"] = recipe(item("lata", "1"))
	// Combo con hamburguesa sin cebolla, 2 porciones de papas y un refresco. La raíz y la
	// hamburguesa van a Cocina caliente (almacén Cocina); las papas a Freidoras (almacén Freidoras);
	// el refresco no tiene estación (Sin preparación) y hereda la del combo.
	combo := unitBreakdown{ProductID: "combo", Components: []componentBreakdown{
		{Quantity: 1, Product: &unitBreakdown{ProductID: "hamburguesa", Ingredients: []ingredientBreakdown{
			{IngredientID: "cebolla", Requested: "0"}, {IngredientID: "lechuga", Requested: "1"},
		}}},
		{Quantity: 2, Product: &unitBreakdown{ProductID: "papas"}},
		{Quantity: 1, Product: &unitBreakdown{ProductID: "refresco"}},
		{Quantity: 0, Product: nil},
	}}
	kitchen := stationRef{ID: "s-cocina", Name: "Cocina caliente"}
	fryer := stationRef{ID: "s-freidoras", Name: "Freidoras"}
	routing := unitRouting{
		DefaultWarehouse: "general",
		StationsByPath: map[string][]stationRef{
			"root": {kitchen}, "root/0": {kitchen}, "root/1": {fryer},
		},
		Assignments: map[string]string{"s-cocina": "cocina", "s-freidoras": "freidoras"},
	}
	result := resolveFor(t, combo, pricingSnapshot{}, "dine_in", config, routing)
	expectConsumption(t, result, map[string]string{
		"charola@cocina": "1", "pan@cocina": "1", "carne@cocina": "150", "queso@cocina": "1", "lechuga-g@cocina": "15",
		"papa-g@freidoras": "300", "lata@cocina": "1",
	})
	if names := result.Stations["freidoras"]; len(names) != 1 || names[0] != "Freidoras" {
		t.Fatalf("estación resuelta del almacén de freidoras: %v", names)
	}
	// Dos combos en la misma unidad no existen (una unidad por cantidad), pero un componente con
	// cantidad 3 multiplica toda su receta.
	triple := unitBreakdown{ProductID: "combo", Components: []componentBreakdown{{Quantity: 3, Product: &unitBreakdown{ProductID: "papas"}}}}
	result = resolveFor(t, triple, pricingSnapshot{}, "dine_in", config, generalRouting)
	expectConsumption(t, result, map[string]string{"charola@general": "1", "papa-g@general": "450"})
}

func TestResolutionStationWarehouseChoice(t *testing.T) {
	config := mapConfig{products: map[string]*recipeVersion{"platillo": recipe(item("sal-g", "1"))}}
	barra := stationRef{ID: "s-barra", Name: "Barra"}
	cocina := stationRef{ID: "s-cocina", Name: "Cocina"}
	cases := []struct {
		name        string
		stations    []stationRef
		assignments map[string]string
		warehouse   string
	}{
		// Varias estaciones con almacenes distintos: la primera por nombre con asignación.
		{"primera por nombre", []stationRef{cocina, barra}, map[string]string{"s-barra": "w-barra", "s-cocina": "w-cocina"}, "w-barra"},
		// La primera por nombre sin asignación cede a la siguiente que sí la tiene.
		{"primera con asignación", []stationRef{barra, cocina}, map[string]string{"s-cocina": "w-cocina"}, "w-cocina"},
		// Sin asignación: Almacén general.
		{"sin asignación", []stationRef{barra}, map[string]string{}, "general"},
		// Sin estación: Almacén general.
		{"sin estación", nil, map[string]string{}, "general"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			routing := unitRouting{DefaultWarehouse: "general", StationsByPath: map[string][]stationRef{}, Assignments: testCase.assignments}
			if testCase.stations != nil {
				routing.StationsByPath["root"] = testCase.stations
			}
			result := resolveFor(t, unitBreakdown{ProductID: "platillo"}, pricingSnapshot{}, "dine_in", config, routing)
			expectConsumption(t, result, map[string]string{"sal-g@" + testCase.warehouse: "1"})
		})
	}
}

func TestResolutionDepthLimit(t *testing.T) {
	config := mapConfig{products: map[string]*recipeVersion{"nivel": recipe(item("sal-g", "1"))}}
	root := &unitBreakdown{ProductID: "nivel"}
	current := root
	for level := 0; level < maximumDepth+2; level++ {
		child := &unitBreakdown{ProductID: "nivel"}
		current.Components = []componentBreakdown{{Quantity: 1, Product: child}}
		current = child
	}
	result := resolveFor(t, *root, pricingSnapshot{}, "dine_in", config, generalRouting)
	// La raíz y ocho niveles de componentes consumen; el resto se corta y se señala.
	expectConsumption(t, result, map[string]string{"sal-g@general": "9"})
	if !hasDiagnostic(result, diagnosticDepthExceeded) {
		t.Fatalf("falta el diagnóstico de profundidad: %+v", result.Diagnostics)
	}
}

func TestResolutionBatch(t *testing.T) {
	salsa := recipe(item("tomate-g", "400"), item("chile-g", "50"))
	salsa.PreparationMode, salsa.YieldBase, salsa.RecipeID = preparationNow, d("500"), "salsa"
	stored := recipe(item("arroz-crudo-g", "1000"), prep("salsa", "100"))
	stored.PreparationMode, stored.YieldBase, stored.RecipeID, stored.PreparationItemID = preparationStored, d("2500"), "arroz", "arroz-preparado-g"
	config := mapConfig{preparations: map[string]*recipeVersion{"salsa": salsa, "arroz": stored}}
	result, err := resolveBatch(stored, d("2.5"), config, "cocina")
	if err != nil {
		t.Fatal(err)
	}
	expectConsumption(t, result, map[string]string{"arroz-crudo-g@cocina": "2500", "tomate-g@cocina": "200", "chile-g@cocina": "25"})
}

func TestRecipeCycleDetection(t *testing.T) {
	edges := map[string][]string{"a": {"b"}, "b": {"c"}, "c": {}}
	if pathReturns(edges, "a") {
		t.Fatal("no hay ciclo en a → b → c")
	}
	edges["c"] = []string{"a"}
	if !pathReturns(edges, "a") {
		t.Fatal("a → b → c → a es un ciclo")
	}
	// A través del insumo producido: la almacenable s produce x y la inmediata p consume x; si s
	// usa p, el ciclo s → p → (productor de x = s) se detecta.
	viaItem := map[string][]string{"s": {"p"}, "p": {"s"}}
	if !pathReturns(viaItem, "s") {
		t.Fatal("el ciclo a través del insumo producido no se detectó")
	}
	if pathReturns(map[string][]string{"s": {"p"}, "p": {"q"}, "q": {"p"}}, "s") {
		t.Fatal("un ciclo ajeno no involucra a la receta guardada")
	}
}
