package catalog

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Etapa 9A, §3.1: «El precio puede bajar al cambiar complementos». Las pruebas usan el filete de
// $150 de referenceSnapshot (arroz $20 + ensalada $15 incluidos; papas $25 y frijoles $10).

func sidesSelection(options ...IncludedOption) []GroupSelection {
	return []GroupSelection{{GroupID: "sides", Options: options}}
}

func option(id, quantity string) IncludedOption {
	return IncludedOption{OptionID: id, Quantity: quantity}
}

// withDecrease activa la opción en el producto indicado del snapshot.
func withDecrease(snapshot Snapshot, productID string) Snapshot {
	product := snapshot.Products[productID]
	product.PriceCanDecrease = true
	snapshot.Products[productID] = product
	return snapshot
}

func calculate(t *testing.T, snapshot Snapshot, request PreviewRequest) *Quote {
	t.Helper()
	quote, err := Calculate(snapshot, request, calculationTime, time.UTC)
	if err != nil {
		t.Fatalf("La cotización falló: %v", err)
	}
	return quote
}

// comboSnapshot agrega un combo de $160 cuyo único componente es el filete (con sus guarniciones).
func comboSnapshot() Snapshot {
	snapshot, _ := referenceSnapshot()
	snapshot.Products["combo"] = Product{
		Metadata:   Metadata{ID: "combo", Name: "Combo filete", Active: true, Revision: 1},
		CategoryID: "food",
		Kind:       "combo",
		BasePrice:  "160",
		Slots: []ComboSlot{
			{
				ID:               "main",
				Name:             "Plato",
				MinQuantity:      1,
				MaxQuantity:      1,
				IncludedQuantity: 1,
				DefaultQuantity:  1,
				DefaultProductID: "plate",
				Options:          []ComboOption{{ProductID: "plate", SubstitutionPrice: "2.50"}},
			},
		},
	}
	return snapshot
}

// comboRequest pide el combo con el filete sustituyendo la ensalada por frijoles (−$5 de
// equivalencia) y con la personalización de ingredientes indicada.
func comboRequest(ingredients []IngredientSelection) PreviewRequest {
	return PreviewRequest{Lines: []LineRequest{{
		ProductID: "combo",
		Quantity:  1,
		Customization: Customization{Components: []ComponentSelection{{
			SlotID:    "main",
			ProductID: "plate",
			Quantity:  1,
			Customization: Customization{
				Ingredients: ingredients,
				Groups:      sidesSelection(option("rice", "1"), option("beans", "1")),
			},
		}}},
	}}}
}

// quantitySnapshot convierte las guarniciones al modo cantidad: cada porción adicional cuesta $15
// (un tramo desde la unidad 1). reference, si no está vacío, reemplaza el valor de referencia de
// todas las opciones.
func quantitySnapshot(reference string) (Snapshot, PreviewRequest) {
	snapshot, request := referenceSnapshot()
	group := snapshot.Groups["sides"]
	group.PricingMode = "quantity"
	for index := range group.Options {
		group.Options[index].Tiers = []QuantityTier{{From: 1, UnitPrice: "15"}}
		if reference != "" {
			group.Options[index].ReferenceValue = reference
		}
	}
	snapshot.Groups["sides"] = group
	return snapshot, request
}

// legacyScenario es una cotización que solo usa datos anteriores a la etapa 9A.
type legacyScenario struct {
	name     string
	snapshot Snapshot
	request  PreviewRequest
}

// legacyScenarios reúne cotizaciones sin la opción: piso, extras, ingredientes, modo cantidad,
// presentación, lista de precios, combo y promociones. Solo usa campos que existían antes de la
// 9A, así que la misma función produjo con el código anterior las huellas de legacyQuoteDigests.
func legacyScenarios() []legacyScenario {
	scenarios := []legacyScenario{}
	add := func(name string, snapshot Snapshot, request PreviewRequest) {
		scenarios = append(scenarios, legacyScenario{name: name, snapshot: snapshot, request: request})
	}
	snapshot, request := referenceSnapshot()
	request.Lines[0].Customization.Groups = sidesSelection()
	add("retirar todo", snapshot, request)
	snapshot, request = referenceSnapshot()
	request.Lines[0].Customization.Groups = sidesSelection(option("salad", "1"), option("beans", "1"))
	add("sustituir por uno más barato", snapshot, request)
	snapshot, request = referenceSnapshot()
	group := snapshot.Groups["sides"]
	group.Options[3].ExtraPrice = "4.00"
	snapshot.Groups["sides"] = group
	request.Lines[0].Customization.Groups = sidesSelection(option("beans", "1"))
	request.Lines[0].Customization.Ingredients = []IngredientSelection{{IngredientID: "onion", Quantity: "3"}}
	add("extras e ingredientes", snapshot, request)
	snapshot, request = quantitySnapshot("")
	request.Lines[0].Customization.Groups = sidesSelection(option("salad", "1"), option("fries", "1"))
	add("modo cantidad", snapshot, request)
	snapshot, request = referenceSnapshot()
	product := snapshot.Products["plate"]
	product.Variants = []Variant{{ID: "large", Name: "Grande", Price: "180", Active: true}}
	snapshot.Products["plate"] = product
	variant := "large"
	request.Lines[0].VariantID = &variant
	request.Lines[0].Customization.Groups = sidesSelection(option("rice", "1"), option("beans", "1"))
	add("presentación", snapshot, request)
	snapshot, request = referenceSnapshot()
	snapshot.PriceList.Entries = []PriceEntry{{ProductID: "plate", Price: "115"}}
	request.Lines[0].Customization.Groups = sidesSelection(option("rice", "1"))
	add("lista de precios", snapshot, request)
	add("combo", comboSnapshot(), comboRequest([]IngredientSelection{{IngredientID: "onion", Quantity: "2"}}))
	for _, kind := range []string{"two_for_one", "percent", "special_price"} {
		snapshot, request = referenceSnapshot()
		snapshot.Promotions = []Promotion{{
			Metadata:        Metadata{ID: "promo", Name: "Oferta", Active: true},
			Kind:            kind,
			Percent:         "10",
			SpecialPrice:    "100",
			MinimumQuantity: 1,
			CategoryIDs:     []string{"food"},
		}}
		request.Lines[0].Quantity = 2
		request.Lines[0].Customization.Groups = sidesSelection(option("rice", "1"), option("beans", "1"))
		add("promoción "+kind, snapshot, request)
	}
	return scenarios
}

func quoteDigest(t *testing.T, quote *Quote) string {
	t.Helper()
	encoded, err := json.Marshal(quote)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

// legacyQuoteDigests son las huellas SHA-256 del JSON de cada cotización de legacyScenarios
// calculadas con el código anterior a la etapa 9A (commit da182ee).
var legacyQuoteDigests = map[string]string{
	"retirar todo":                 "99c3d00bebe7d676377625151f9c9f5b655cad1baf10b07af56cc7083cffcdbd",
	"sustituir por uno más barato": "c6340f37fff36b23b053f3e0523ceb66727f8a051eb8e2b5ec9ac397906de95d",
	"extras e ingredientes":        "e45423a2266128eac416dc74102b520b276671029918ad3a185dfb6108cfd6e5",
	"modo cantidad":                "25d59b498c220984bcdeb06661136ec6d6a1a4f577618c7d787c9ed9570fb984",
	"presentación":                 "1c692cc13d068fd5b602e481e300020debc37f0cc1287d6b09913e08731770f5",
	"lista de precios":             "224cb6f15ffa7f0483656faf5fde40067fc85ca6fdd703a801b4a1efdeea8c0b",
	"combo":                        "084706cd1f45dead01ae5b7384569b50419e2f2a76d5ec37a606c51cb3acae23",
	"promoción two_for_one":        "61f099a0c014197d0026ff6ffc19e8b405ce11311f8de6949b181158966a1762",
	"promoción percent":            "d4dd2ea3a9fa4deed3e5f857d0e1d2a80f07df8fa6a75b4294a19d38cc59a418",
	"promoción special_price":      "20a1422a2c8cda71c23e811d341873056b21ed542634118e5dff0974a9d0541a",
}

// Con la opción desactivada el camino es idéntico al anterior: mismos importes y el mismo JSON,
// byte a byte, sin los campos nuevos (las huellas de borradores y cotizaciones no cambian).
func TestPriceDecreaseDisabledKeepsLegacyQuotes(t *testing.T) {
	for _, scenario := range legacyScenarios() {
		quote := calculate(t, scenario.snapshot, scenario.request)
		encoded, _ := json.Marshal(quote)
		if strings.Contains(string(encoded), "price_can_decrease") || strings.Contains(string(encoded), "complement_adjustment") {
			t.Fatalf("%s: la cotización sin la opción serializa los campos nuevos: %s", scenario.name, encoded)
		}
		if want := legacyQuoteDigests[scenario.name]; quoteDigest(t, quote) != want {
			t.Fatalf("%s: la cotización sin la opción cambió: %s", scenario.name, encoded)
		}
	}
}

// Opción desactivada: el piso actual sigue intacto (quitar o sustituir nunca baja).
func TestPriceDecreaseDisabledKeepsFloor(t *testing.T) {
	tests := []struct {
		name    string
		options []IncludedOption
		want    string
	}{
		{name: "quitar todo sin reemplazo", options: []IncludedOption{}, want: "150.00"},
		{name: "quitar la ensalada", options: []IncludedOption{option("rice", "1")}, want: "150.00"},
		{name: "ensalada por frijoles", options: []IncludedOption{option("rice", "1"), option("beans", "1")}, want: "150.00"},
		{name: "papas por arroz", options: []IncludedOption{option("fries", "1"), option("salad", "1")}, want: "155.00"},
	}
	for _, test := range tests {
		snapshot, request := referenceSnapshot()
		request.Lines[0].Customization.Groups = sidesSelection(test.options...)
		quote := calculate(t, snapshot, request)
		line := quote.Lines[0]
		if quote.Total != test.want || line.PriceCanDecrease || line.ComplementAdjustment != "" || strings.HasPrefix(line.Groups[0].Difference, "-") {
			t.Fatalf("%s: %s %+v", test.name, quote.Total, line.Groups[0])
		}
	}
}

// Opción activada: diferencia con signo por grupo, quitar sin reemplazo y el ejemplo del
// propietario ($150, incluidos $35, seleccionados $25 → $140).
func TestPriceDecreaseEnabledSignedDifference(t *testing.T) {
	tests := []struct {
		name       string
		options    []IncludedOption
		total      string
		difference string
		adjustment string
	}{
		{name: "sin cambio", options: []IncludedOption{option("rice", "1"), option("salad", "1")}, total: "150.00", difference: "0.00", adjustment: "0.00"},
		{name: "diferencia positiva", options: []IncludedOption{option("fries", "1"), option("salad", "1")}, total: "155.00", difference: "5.00", adjustment: "0.00"},
		{name: "ejemplo del propietario", options: []IncludedOption{option("salad", "1"), option("beans", "1")}, total: "140.00", difference: "-10.00", adjustment: "-10.00"},
		{name: "quitar la ensalada sin reemplazo", options: []IncludedOption{option("rice", "1")}, total: "135.00", difference: "-15.00", adjustment: "-15.00"},
		{name: "quitar todo sin reemplazo", options: []IncludedOption{}, total: "115.00", difference: "-35.00", adjustment: "-35.00"},
		{name: "doble papas", options: []IncludedOption{option("fries", "2")}, total: "165.00", difference: "15.00", adjustment: "0.00"},
	}
	for _, test := range tests {
		snapshot, request := referenceSnapshot()
		snapshot = withDecrease(snapshot, "plate")
		request.Lines[0].Customization.Groups = sidesSelection(test.options...)
		quote := calculate(t, snapshot, request)
		line := quote.Lines[0]
		if quote.Total != test.total || line.Groups[0].Difference != test.difference {
			t.Fatalf("%s: total %s, diferencia %s", test.name, quote.Total, line.Groups[0].Difference)
		}
		if !line.PriceCanDecrease || line.ComplementAdjustment != test.adjustment {
			t.Fatalf("%s: el desglose no registra la regla y su efecto: %v %q", test.name, line.PriceCanDecrease, line.ComplementAdjustment)
		}
		// El desglose conserva unit = base + ingredientes + complementos + componentes.
		if line.UnitPrice != test.total || line.GroupExtras != money(amount(test.total).Sub(amount("150"))) {
			t.Fatalf("%s: desglose incoherente %+v", test.name, line.ProductBreakdown)
		}
	}
}

// Límites: nunca negativo y nunca por debajo del valor de los complementos que permanecen.
func TestPriceDecreaseLimits(t *testing.T) {
	// Producto mal configurado: base $20 con $35 incluidos. Quitar todo deja el precio en cero, no
	// en −$15.
	snapshot, request := referenceSnapshot()
	product := snapshot.Products["plate"]
	product.BasePrice = "20"
	snapshot.Products["plate"] = product
	snapshot = withDecrease(snapshot, "plate")
	request.Lines[0].Customization.Groups = sidesSelection()
	quote := calculate(t, snapshot, request)
	if quote.Total != "0.00" || quote.Lines[0].ComplementAdjustment != "-20.00" {
		t.Fatalf("El precio debía quedar en cero: %s %s", quote.Total, quote.Lines[0].ComplementAdjustment)
	}
	// Base $30: conservar solo el arroz ($20) daría $15, pero el precio no baja del valor del
	// complemento que permanece.
	product = snapshot.Products["plate"]
	product.BasePrice = "30"
	snapshot.Products["plate"] = product
	request.Lines[0].Customization.Groups = sidesSelection(option("rice", "1"))
	quote = calculate(t, snapshot, request)
	if quote.Total != "20.00" || quote.Lines[0].ComplementAdjustment != "-10.00" || quote.Lines[0].Groups[0].Difference != "-15.00" {
		t.Fatalf("El precio bajó del valor de los complementos que permanecen: %s %+v", quote.Total, quote.Lines[0].ProductBreakdown)
	}
	// La opción nunca sube el precio: con $35 incluidos en una base de $20, conservar lo incluido
	// cuesta lo mismo que sin la opción.
	product.BasePrice = "20"
	snapshot.Products["plate"] = product
	request.Lines[0].Customization.Groups = sidesSelection(option("rice", "1"), option("salad", "1"))
	quote = calculate(t, snapshot, request)
	if quote.Total != "20.00" || quote.Lines[0].ComplementAdjustment != "0.00" {
		t.Fatalf("La opción subió el precio: %s", quote.Total)
	}
}

// Los extras explícitos y los ingredientes adicionales son cargos independientes: se suman
// después del ajuste y nunca se descuentan, ni siquiera cuando actúa el límite.
func TestPriceDecreaseKeepsIndependentCharges(t *testing.T) {
	snapshot, request := referenceSnapshot()
	group := snapshot.Groups["sides"]
	group.Options[3].ExtraPrice = "4.00"
	snapshot.Groups["sides"] = group
	snapshot = withDecrease(snapshot, "plate")
	request.Lines[0].Customization.Groups = sidesSelection(option("beans", "1"))
	request.Lines[0].Customization.Ingredients = []IngredientSelection{{IngredientID: "onion", Quantity: "3"}}
	quote := calculate(t, snapshot, request)
	// 150 − 25 (equivalencia) + 4 (extra de frijoles) + 0.20 (cebolla) = 129.20.
	line := quote.Lines[0]
	if quote.Total != "129.20" || line.Groups[0].Extras != "4.00" || line.IngredientExtras != "0.20" || line.ComplementAdjustment != "-25.00" {
		t.Fatalf("Se descontó un cargo independiente: %s %+v", quote.Total, line.ProductBreakdown)
	}
	// Base $20 con el límite del valor que permanece ($10): el núcleo queda en $10 y los cargos
	// independientes se suman completos.
	product := snapshot.Products["plate"]
	product.BasePrice = "20"
	snapshot.Products["plate"] = product
	quote = calculate(t, snapshot, request)
	if quote.Total != "14.20" {
		t.Fatalf("El límite absorbió cargos independientes: %s", quote.Total)
	}
}

// Modo cantidad: tramos de las porciones adicionales menos el valor de referencia de las incluidas
// que se retiraron; sin valores de referencia no cambia nada.
func TestPriceDecreaseQuantityMode(t *testing.T) {
	selection := sidesSelection(option("salad", "1"), option("fries", "1"))
	snapshot, request := quantitySnapshot("")
	request.Lines[0].Customization.Groups = selection
	if quote := calculate(t, snapshot, request); quote.Total != "165.00" {
		t.Fatalf("Sin la opción el modo cantidad cobra el tramo: %s", quote.Total)
	}
	snapshot = withDecrease(snapshot, "plate")
	quote := calculate(t, snapshot, request)
	// Tramo de papas $15 − arroz retirado $20 = −$5.
	if quote.Total != "145.00" || quote.Lines[0].Groups[0].Difference != "-5.00" {
		t.Fatalf("Modo cantidad con la opción: %s %s", quote.Total, quote.Lines[0].Groups[0].Difference)
	}
	snapshot, request = quantitySnapshot("0")
	snapshot = withDecrease(snapshot, "plate")
	request.Lines[0].Customization.Groups = selection
	quote = calculate(t, snapshot, request)
	if quote.Total != "165.00" || quote.Lines[0].ComplementAdjustment != "0.00" {
		t.Fatalf("Sin valores de referencia la opción no cambia el modo cantidad: %s", quote.Total)
	}
}

// Presentaciones y listas de precios: el ajuste parte del precio efectivo y respeta sus límites.
func TestPriceDecreaseVariantsAndPriceLists(t *testing.T) {
	snapshot, request := referenceSnapshot()
	product := snapshot.Products["plate"]
	product.Variants = []Variant{{ID: "large", Name: "Grande", Price: "180", Active: true}}
	snapshot.Products["plate"] = product
	snapshot = withDecrease(snapshot, "plate")
	variant := "large"
	request.Lines[0].VariantID = &variant
	request.Lines[0].Customization.Groups = sidesSelection(option("salad", "1"), option("beans", "1"))
	if quote := calculate(t, snapshot, request); quote.Total != "170.00" {
		t.Fatalf("Presentación con la opción: %s", quote.Total)
	}
	snapshot, request = referenceSnapshot()
	snapshot = withDecrease(snapshot, "plate")
	snapshot.PriceList.Entries = []PriceEntry{{ProductID: "plate", Price: "115"}}
	request.Lines[0].Customization.Groups = sidesSelection(option("salad", "1"), option("beans", "1"))
	quote := calculate(t, snapshot, request)
	if quote.Total != "105.00" || !strings.HasPrefix(quote.Lines[0].PriceOrigin, "Lista:") {
		t.Fatalf("Lista de precios con la opción: %s", quote.Total)
	}
	// Una lista con precio bajo ($30) queda protegida por el valor de lo que permanece ($25).
	snapshot.PriceList.Entries = []PriceEntry{{ProductID: "plate", Price: "30"}}
	if quote := calculate(t, snapshot, request); quote.Total != "25.00" {
		t.Fatalf("Lista baja con la opción: %s", quote.Total)
	}
}

// Combos: el ajuste negativo de un componente solo aplica si el combo también tiene la opción, y
// nunca rebaja el cargo de sustitución ni los extras del componente.
func TestPriceDecreaseCombos(t *testing.T) {
	onion := []IngredientSelection{{IngredientID: "onion", Quantity: "2"}}
	tests := []struct {
		name               string
		combo, child       bool
		total, charge      string
		comboAdjustment    string
		childHasAdjustment bool
	}{
		{name: "sin la opción", total: "162.60", charge: "2.60"},
		{name: "solo el componente", child: true, total: "162.60", charge: "2.60"},
		{name: "solo el combo", combo: true, total: "162.60", charge: "2.60", comboAdjustment: "0.00"},
		{name: "combo y componente", combo: true, child: true, total: "157.60", charge: "2.60", comboAdjustment: "-5.00", childHasAdjustment: true},
	}
	for _, test := range tests {
		snapshot := comboSnapshot()
		if test.combo {
			snapshot = withDecrease(snapshot, "combo")
		}
		if test.child {
			snapshot = withDecrease(snapshot, "plate")
		}
		quote := calculate(t, snapshot, comboRequest(onion))
		line := quote.Lines[0]
		component := line.Components[0]
		if quote.Total != test.total || component.Charge != test.charge || line.ComplementAdjustment != test.comboAdjustment {
			t.Fatalf("%s: total %s, cargo %s, ajuste %q", test.name, quote.Total, component.Charge, line.ComplementAdjustment)
		}
		if (component.Product.ComplementAdjustment != "") != test.childHasAdjustment {
			t.Fatalf("%s: el componente aplicó la opción sin el combo: %+v", test.name, component.Product)
		}
	}
	// El combo con la opción tampoco baja del valor de los complementos que permanecen en su
	// componente: combo de $20 con el filete sin ensalada (queda el arroz, $20).
	snapshot := withDecrease(withDecrease(comboSnapshot(), "combo"), "plate")
	combo := snapshot.Products["combo"]
	combo.BasePrice = "20"
	snapshot.Products["combo"] = combo
	request := comboRequest(nil)
	request.Lines[0].Customization.Components[0].Customization.Groups = sidesSelection(option("rice", "1"))
	if quote := calculate(t, snapshot, request); quote.Total != "22.50" || quote.Lines[0].ComplementAdjustment != "0.00" {
		t.Fatalf("El combo bajó del valor de sus complementos: %s %s", quote.Total, quote.Lines[0].ComplementAdjustment)
	}
}

// Promociones: se calculan sobre el precio ya reducido, sin descontar dos veces la diferencia.
func TestPriceDecreasePromotionsWithoutDoubleDiscount(t *testing.T) {
	tests := []struct {
		kind      string
		discount  string
		total     string
		extras    bool
		quantity  int
		selection []IncludedOption
	}{
		// 2 × $140: el 2×1 bonifica una unidad de $140, no la base de $150.
		{kind: "two_for_one", quantity: 2, discount: "140.00", total: "140.00"},
		// 10 % de $140, no de $150.
		{kind: "percent", quantity: 1, discount: "14.00", total: "126.00"},
		{kind: "percent", quantity: 1, discount: "14.00", total: "126.00", extras: true},
		// Precio especial de $100 sobre $140: descuento de $40, no de $50.
		{kind: "special_price", quantity: 1, discount: "40.00", total: "100.00"},
	}
	for _, test := range tests {
		snapshot, request := referenceSnapshot()
		snapshot = withDecrease(snapshot, "plate")
		snapshot.Promotions = []Promotion{{
			Metadata:        Metadata{ID: "promo", Name: "Oferta", Active: true},
			Kind:            test.kind,
			Percent:         "10",
			SpecialPrice:    "100",
			MinimumQuantity: 1,
			IncludeExtras:   test.extras,
			CategoryIDs:     []string{"food"},
		}}
		request.Lines[0].Quantity = test.quantity
		request.Lines[0].Customization.Groups = sidesSelection(option("salad", "1"), option("beans", "1"))
		quote := calculate(t, snapshot, request)
		if quote.Discount != test.discount || quote.Total != test.total {
			t.Fatalf("%s: descuento %s total %s", test.kind, quote.Discount, quote.Total)
		}
		line := quote.Lines[0]
		if amount(line.Discount).GreaterThan(amount(line.Subtotal)) {
			t.Fatalf("%s: la promoción supera el subtotal", test.kind)
		}
	}
	// Una reducción mayor que la base (base de $20, se retiran $35 incluidos y la porción adicional
	// cuesta $200 por tramo) deja la base de la promoción en cero, nunca negativa: núcleo $185 =
	// 20 + 200 − 35, con base reducida 185 − 200 < 0.
	snapshot, request := quantitySnapshot("")
	group := snapshot.Groups["sides"]
	for index := range group.Options {
		group.Options[index].Tiers = []QuantityTier{{From: 1, UnitPrice: "200"}}
	}
	snapshot.Groups["sides"] = group
	product := snapshot.Products["plate"]
	product.BasePrice = "20"
	snapshot.Products["plate"] = product
	snapshot = withDecrease(snapshot, "plate")
	snapshot.Promotions = []Promotion{{
		Metadata:        Metadata{ID: "promo", Name: "Oferta", Active: true},
		Kind:            "percent",
		Percent:         "50",
		MinimumQuantity: 1,
		CategoryIDs:     []string{"food"},
	}}
	request.Lines[0].Customization.Groups = sidesSelection(option("fries", "1"))
	quote := calculate(t, snapshot, request)
	if quote.Discount != "0.00" || quote.Total != "185.00" {
		t.Fatalf("La base de la promoción quedó negativa: %+v", quote)
	}
}

// Cambiar la opción exige el permiso de precios, como el precio y la tasa de IVA.
func TestPriceDecreaseRequiresPricePermission(t *testing.T) {
	previous := Product{Metadata: Metadata{ID: "plate", Active: true}, Kind: "product", BasePrice: "150"}
	changed := previous
	changed.PriceCanDecrease = true
	withoutPrices := Actor{Permissions: []string{"catalog.products.manage"}}
	if err := validateProductPermissions(&changed, &previous, withoutPrices); err == nil {
		t.Fatal("Se cambió la opción sin catalog.prices.manage")
	}
	if err := validateProductPermissions(&previous, &previous, withoutPrices); err != nil {
		t.Fatalf("Guardar sin cambiar la opción no exige el permiso de precios: %v", err)
	}
	withPrices := Actor{Permissions: []string{"catalog.products.manage", "catalog.prices.manage"}}
	if err := validateProductPermissions(&changed, &previous, withPrices); err != nil {
		t.Fatalf("Con el permiso de precios la opción se puede cambiar: %v", err)
	}
}
