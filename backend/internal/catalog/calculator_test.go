package catalog

import (
	"strings"
	"testing"
	"time"
)

func referenceSnapshot() (Snapshot, PreviewRequest) {
	snapshot := Snapshot{
		Products: map[string]Product{"plate": {
			Metadata: Metadata{
				ID:       "plate",
				Name:     "Filete",
				Active:   true,
				Revision: 1,
			},
			CategoryID: "food",
			Kind:       "product",
			BasePrice:  "150.00",
			Groups: []ProductGroup{
				{
					GroupID:  "sides",
					Included: []IncludedOption{{OptionID: "rice", Quantity: "1"}, {OptionID: "salad", Quantity: "1"}},
				},
			},
			Ingredients: []ProductIngredient{
				{
					IngredientID:     "onion",
					IncludedQuantity: "1",
					Removable:        true,
					Addable:          true,
					MaxQuantity:      "3",
					ExtraPrice:       "0.10",
					RecipeQuantity:   "0.05",
				},
			},
		}},
		Categories:  map[string]Category{"food": {Metadata: Metadata{ID: "food", Active: true}}},
		Ingredients: map[string]Ingredient{"onion": {Metadata: Metadata{ID: "onion", Name: "Cebolla", Active: true}}},
		Complements: map[string]Complement{},
		Portions:    map[string]Portion{},
		Groups:      map[string]OptionGroup{},
		PriceList: PriceList{
			Metadata: Metadata{
				ID:       "normal",
				Name:     "Restaurante",
				Active:   true,
				Revision: 1,
			},
			Default:         true,
			Fallback:        "base",
			AllowPromotions: true,
		},
	}
	group := OptionGroup{
		Metadata: Metadata{
			ID:       "sides",
			Name:     "Guarniciones",
			Active:   true,
			Revision: 1,
		},
		PricingMode: "value",
		MinQuantity: "0",
	}
	for index, id := range []string{
		"rice",
		"salad",
		"fries",
		"beans",
	} {
		values := []string{
			"20",
			"15",
			"25",
			"10",
		}
		snapshot.Complements[id] = Complement{Metadata: Metadata{ID: id, Name: id, Active: true}}
		group.Options = append(
			group.Options,
			GroupOption{
				ID:             id,
				ComplementID:   id,
				ReferenceValue: values[index],
				ExtraPrice:     "0",
				Active:         true,
				Name:           id,
			},
		)
	}
	snapshot.Groups["sides"] = group
	return snapshot, PreviewRequest{Lines: []LineRequest{{ProductID: "plate", Quantity: 1}}}
}

var calculationTime = time.Date(

	2026,
	11,
	7,
	18,
	0,
	0,
	0,
	time.UTC,
)

func TestComplementEquivalenceAndIngredients(t *testing.T) {
	tests := []struct {
		name        string
		options     []IncludedOption
		ingredients []IngredientSelection
		want        string
	}{
		{name: "original", want: "150.00"},
		{name: "retirar todos", options: []IncludedOption{}, want: "150.00"},
		{
			name:    "papas y ensalada",
			options: []IncludedOption{{OptionID: "fries", Quantity: "1"}, {OptionID: "salad", Quantity: "1"}},
			want:    "155.00",
		},
		{
			name:    "reemplazar todos",
			options: []IncludedOption{{OptionID: "fries", Quantity: "1"}, {OptionID: "beans", Quantity: "1"}},
			want:    "150.00",
		},
		{name: "doble papas", options: []IncludedOption{{OptionID: "fries", Quantity: "2"}}, want: "165.00"},
		{
			name:        "sin cebolla",
			ingredients: []IngredientSelection{{IngredientID: "onion", Quantity: "0"}},
			want:        "150.00",
		},
		{
			name:        "cebolla adicional exacta",
			ingredients: []IngredientSelection{{IngredientID: "onion", Quantity: "3"}},
			want:        "150.20",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, request := referenceSnapshot()
			request.Lines[0].Customization.Ingredients = test.ingredients
			if test.options != nil {
				request.Lines[0].Customization.Groups = []GroupSelection{{GroupID: "sides", Options: test.options}}
			}
			quote, err := Calculate(snapshot, request, calculationTime, time.UTC)
			if err != nil || quote.Total != test.want {
				t.Fatalf("resultado=%+v error=%v; esperado %s", quote, err, test.want)
			}
		})
	}
}

func TestSeparateExplicitExtrasAndRounding(t *testing.T) {
	snapshot, request := referenceSnapshot()
	group := snapshot.Groups["sides"]
	group.Options[0].ExtraPrice = "3.00"
	group.Options[2].ReferenceValue = "0.01"
	snapshot.Groups["sides"] = group
	request.Lines[0].Customization.Groups = []GroupSelection{
		{
			GroupID: "sides",
			Options: []IncludedOption{{OptionID: "rice", Quantity: "2"}, {OptionID: "salad", Quantity: "1"}},
		},
	}
	quote, err := Calculate(snapshot, request, calculationTime, time.UTC)
	if err != nil || quote.Total != "173.00" || quote.Lines[0].Groups[0].Extras != "3.00" {
		t.Fatalf("No se separó el extra explícito: %+v %v", quote, err)
	}
	product := snapshot.Products["plate"]
	product.Groups[0].Included = nil
	snapshot.Products["plate"] = product
	request.Lines[0].Customization.Groups = []GroupSelection{{GroupID: "sides", Options: []IncludedOption{{OptionID: "fries", Quantity: "0.5"}}}}
	quote, err = Calculate(snapshot, request, calculationTime, time.UTC)
	if err != nil || quote.Total != "150.01" {
		t.Fatalf("0.005 debe redondear a 0.01: %+v %v", quote, err)
	}
	if _, err := parseMoney("0.005"); err == nil {
		t.Fatal("Se aceptó precisión monetaria de entrada excesiva")
	}
}

func TestSelectionValidationAndAvailability(t *testing.T) {
	snapshot, request := referenceSnapshot()
	for _, quantity := range []string{
		"-1",
		"1001",
		"0.0001",
		"NaN",
	} {
		request.Lines[0].Customization.Groups = []GroupSelection{{GroupID: "sides", Options: []IncludedOption{{OptionID: "rice", Quantity: quantity}}}}
		if _, err := Calculate(snapshot, request, calculationTime, time.UTC); err == nil {
			t.Fatalf("Se aceptó cantidad %s", quantity)
		}
	}
	request.Lines[0].Customization = Customization{}
	group := snapshot.Groups["sides"]
	maximum := "1"
	group.MaxQuantity = &maximum
	snapshot.Groups["sides"] = group
	if _, err := Calculate(snapshot, request, calculationTime, time.UTC); err == nil {
		t.Fatal("Se ignoró máximo del grupo")
	}
	group.MaxQuantity = nil
	snapshot.Groups["sides"] = group
	product := snapshot.Products["plate"]
	product.TemporaryUnavailable = true
	snapshot.Products["plate"] = product
	if _, err := Calculate(snapshot, request, calculationTime, time.UTC); err == nil {
		t.Fatal("Se ignoró indisponibilidad")
	}
	request.OverrideAvailability = true
	if _, err := Calculate(snapshot, request, calculationTime, time.UTC); err != nil {
		t.Fatal(err)
	}
	product.Active = false
	snapshot.Products["plate"] = product
	if _, err := Calculate(snapshot, request, calculationTime, time.UTC); err == nil {
		t.Fatal("Una excepción permitió producto inactivo")
	}
}

func TestComboIncrementalPricingAndCustomization(t *testing.T) {
	snapshot, _ := referenceSnapshot()
	burger := Product{
		Metadata:   Metadata{ID: "burger", Name: "Hamburguesa", Active: true},
		CategoryID: "food",
		Kind:       "product",
		BasePrice:  "90",
	}
	fries := Product{
		Metadata:   Metadata{ID: "fries-product", Name: "Papas", Active: true},
		CategoryID: "food",
		Kind:       "product",
		BasePrice:  "40",
	}
	combo := Product{
		Metadata:   Metadata{ID: "combo", Name: "Hamburguesa con opción papas", Active: true},
		CategoryID: "food",
		Kind:       "combo",
		BasePrice:  "90",
		Slots: []ComboSlot{
			{
				ID:               "main",
				Name:             "Hamburguesa",
				MinQuantity:      1,
				MaxQuantity:      1,
				IncludedQuantity: 1,
				DefaultQuantity:  1,
				DefaultProductID: "burger",
				Options:          []ComboOption{{ProductID: "burger", SubstitutionPrice: "0"}},
			},
			{
				ID:               "fries-slot",
				Name:             "Papas",
				MaxQuantity:      3,
				DefaultProductID: "fries-product",
				Options: []ComboOption{
					{
						ProductID:         "fries-product",
						SubstitutionPrice: "0",
						Tiers:             []QuantityTier{{From: 1, UnitPrice: "10"}, {From: 2, UnitPrice: "40"}},
					},
				},
			},
		},
	}
	snapshot.Products[burger.ID], snapshot.Products[fries.ID], snapshot.Products[combo.ID] = burger, fries, combo
	for quantity, want := range []string{"90.00", "100.00", "140.00"} {
		request := PreviewRequest{
			Lines: []LineRequest{
				{
					ProductID:     "combo",
					Quantity:      1,
					Customization: Customization{Components: []ComponentSelection{{SlotID: "fries-slot", ProductID: fries.ID, Quantity: quantity}}},
				},
			},
		}
		quote, err := Calculate(snapshot, request, calculationTime, time.UTC)
		if err != nil || quote.Total != want {
			t.Fatalf("papas=%d: %+v %v", quantity, quote, err)
		}
	}
	combo.Slots[1].Options[0].Tiers = nil
	snapshot.Products[combo.ID] = combo
	request := PreviewRequest{
		Lines: []LineRequest{
			{
				ProductID:     "combo",
				Quantity:      1,
				Customization: Customization{Components: []ComponentSelection{{SlotID: "fries-slot", ProductID: fries.ID, Quantity: 1}}},
			},
		},
	}
	if _, err := Calculate(snapshot, request, calculationTime, time.UTC); err == nil {
		t.Fatal("Se inventó precio para una unidad adicional sin tramos")
	}
}

func TestPromotionsAndPriceLists(t *testing.T) {
	tests := []struct {
		name, kind string
		quantity   int
		want       string
	}{
		{
			"dos 2x1",
			"two_for_one",
			2,
			"150.00",
		},
		{
			"tres 2x1",
			"two_for_one",
			3,
			"300.00",
		},
		{
			"compra tres paga dos",
			"quantity",
			3,
			"300.00",
		},
		{
			"porcentaje",
			"percent",
			1,
			"135.00",
		},
		{
			"precio especial",
			"special_price",
			2,
			"200.00",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, request := referenceSnapshot()
			snapshot.Promotions = []Promotion{
				{
					Metadata:        Metadata{ID: "promo", Name: "Oferta", Active: true},
					Kind:            test.kind,
					Percent:         "10",
					SpecialPrice:    "100",
					BuyQuantity:     3,
					PayQuantity:     2,
					MinimumQuantity: 1,
					CategoryIDs:     []string{"food"},
				},
			}
			request.Lines[0].Quantity = test.quantity
			quote, err := Calculate(snapshot, request, calculationTime, time.UTC)
			if err != nil || quote.Total != test.want || !quote.Promotions[0].Applied {
				t.Fatalf("%+v %v", quote, err)
			}
		})
	}
	snapshot, request := referenceSnapshot()
	snapshot.Promotions = []Promotion{
		{
			Metadata:        Metadata{ID: "p1", Active: true},
			Kind:            "percent",
			Percent:         "10",
			MinimumQuantity: 1,
			Priority:        10,
			ProductIDs:      []string{"plate"},
		},
		{
			Metadata:        Metadata{ID: "p2", Active: true},
			Kind:            "percent",
			Percent:         "20",
			MinimumQuantity: 1,
			Priority:        1,
			CategoryIDs:     []string{"food"},
		},
	}
	quote, _ := Calculate(snapshot, request, calculationTime, time.UTC)
	if quote.Total != "135.00" || quote.Promotions[1].Applied {
		t.Fatal("Se acumularon promociones incompatibles")
	}
	snapshot.Promotions[0].Stackable, snapshot.Promotions[1].Stackable = true, true
	quote, _ = Calculate(snapshot, request, calculationTime, time.UTC)
	if quote.Total != "108.00" {
		t.Fatalf("Combinación restante incorrecta: %s", quote.Total)
	}
	snapshot.PriceList.Default = false
	snapshot.PriceList.Entries = []PriceEntry{{ProductID: "plate", Price: "115"}}
	quote, _ = Calculate(snapshot, request, calculationTime, time.UTC)
	if quote.Total != "115.00" || !strings.HasPrefix(quote.Lines[0].PriceOrigin, "Lista:") {
		t.Fatal("La plataforma heredó una promoción no autorizada")
	}
	snapshot.Promotions[0].PriceListIDs = []string{"normal"}
	quote, _ = Calculate(snapshot, request, calculationTime, time.UTC)
	if quote.Total != "103.50" {
		t.Fatal("No se aplicó promoción explícita para la plataforma")
	}
	snapshot.PriceList.AllowPromotions = false
	quote, _ = Calculate(snapshot, request, calculationTime, time.UTC)
	if quote.Total != "115.00" {
		t.Fatal("Se ignoró exclusión de promociones de lista")
	}
}

func TestServerTimezoneSchedules(t *testing.T) {
	location, err := time.LoadLocation("America/Monterrey")
	if err != nil {
		t.Fatal(err)
	}
	schedule := Schedule{
		StartsOn: "2026-11-07",
		EndsOn:   "2026-11-07",
		Days:     []int{6},
		Windows:  []TimeWindow{{Start: "12:00", End: "13:00"}},
	}
	if !scheduleAvailable(schedule, calculationTime, location) || scheduleAvailable(schedule, calculationTime.Add(time.Hour), location) || scheduleAvailable(schedule, calculationTime.AddDate(0, 0, 1), location) {
		t.Fatal("Los horarios deben usar la zona del restaurante y fin exclusivo")
	}
	if err := validateSchedule(Schedule{Windows: []TimeWindow{{Start: "23:00", End: "01:00"}}}); err == nil {
		t.Fatal("No se rechazó un intervalo cruzando medianoche")
	}
}
