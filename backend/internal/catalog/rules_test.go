package catalog

import (
	"testing"
	"time"
)

func TestQuantityGroupsRequireExplicitIncrementalPrices(t *testing.T) {
	snapshot, request := referenceSnapshot()
	group := snapshot.Groups["sides"]
	group.PricingMode = "quantity"
	group.Options[0].Tiers = []QuantityTier{{From: 1, UnitPrice: "10"}, {From: 2, UnitPrice: "40"}}
	snapshot.Groups[group.ID] = group
	request.Lines[0].Customization.Groups = []GroupSelection{{GroupID: group.ID, Options: []IncludedOption{{OptionID: "rice", Quantity: "3"}}}}
	quote, err := Calculate(snapshot, request, calculationTime, time.UTC)
	if err != nil || quote.Total != "200.00" {
		t.Fatalf("Dos adicionales deben cobrar 10+40: %+v %v", quote, err)
	}
	request.Lines[0].Customization.Groups[0].Options[0].Quantity = "1.5"
	if _, err := Calculate(snapshot, request, calculationTime, time.UTC); err == nil {
		t.Fatal("Se aceptaron fracciones para tramos por unidad")
	}
}

func TestComboChildCustomizationIsChargedOnce(t *testing.T) {
	snapshot, _ := referenceSnapshot()
	plate := snapshot.Products["plate"]
	snapshot.Products["combo"] = Product{
		Metadata:   Metadata{ID: "combo", Name: "Combo", Active: true},
		Kind:       "combo",
		CategoryID: "food",
		BasePrice:  "90",
		Slots: []ComboSlot{
			{
				ID:               "main",
				Name:             "Principal",
				MinQuantity:      1,
				MaxQuantity:      1,
				IncludedQuantity: 1,
				DefaultQuantity:  1,
				DefaultProductID: plate.ID,
				Options:          []ComboOption{{ProductID: plate.ID, SubstitutionPrice: "0"}},
			},
		},
	}
	customization := Customization{Ingredients: []IngredientSelection{{IngredientID: "onion", Quantity: "3"}}}
	request := PreviewRequest{
		Lines: []LineRequest{
			{
				ProductID: "combo",
				Quantity:  2,
				Customization: Customization{
					Components: []ComponentSelection{{
						SlotID:        "main",
						ProductID:     plate.ID,
						Quantity:      1,
						Customization: customization,
					}},
				},
			},
		},
	}
	quote, err := Calculate(snapshot, request, calculationTime, time.UTC)
	if err != nil || quote.Total != "180.40" || quote.Lines[0].ComponentExtras != "0.20" {
		t.Fatalf("Se duplicó precio base o personalización: %+v %v", quote, err)
	}
	plate.Active = false
	snapshot.Products[plate.ID] = plate
	if _, err := Calculate(snapshot, request, calculationTime, time.UTC); err == nil {
		t.Fatal("Un componente inactivo dejó disponible su combo")
	}
}

func TestVariantPriceOriginAndFallback(t *testing.T) {
	snapshot, request := referenceSnapshot()
	product := snapshot.Products["plate"]
	product.Variants = []Variant{{
		ID:     "large",
		Name:   "Grande",
		Price:  "180",
		Active: true,
	}}
	snapshot.Products[product.ID] = product
	variantID := "large"
	request.Lines[0].VariantID = &variantID
	snapshot.PriceList.Entries = []PriceEntry{{ProductID: product.ID, Price: "115"}}
	quote, err := Calculate(snapshot, request, calculationTime, time.UTC)
	if err != nil || quote.Total != "180.00" {
		t.Fatal("El respaldo de presentación no conservó su precio propio")
	}
	snapshot.PriceList.Entries = append(snapshot.PriceList.Entries, PriceEntry{ProductID: product.ID, VariantID: &variantID, Price: "120"})
	snapshot.PriceList.Fallback = "reject"
	quote, err = Calculate(snapshot, request, calculationTime, time.UTC)
	if err != nil || quote.Total != "120.00" {
		t.Fatal("No se usó precio específico de presentación")
	}
}

func TestPromotionWindowsAndCentRounding(t *testing.T) {
	snapshot, request := referenceSnapshot()
	product := snapshot.Products["plate"]
	product.BasePrice = "0.05"
	snapshot.Products[product.ID] = product
	snapshot.Promotions = []Promotion{{
		Metadata:        Metadata{ID: "promo", Active: true},
		Kind:            "percent",
		Percent:         "10",
		MinimumQuantity: 1,
		CategoryIDs:     []string{"food"},
		Availability:    Schedule{Windows: []TimeWindow{{Start: "18:00", End: "19:00"}}},
	}}
	quote, err := Calculate(snapshot, request, calculationTime, time.UTC)
	if err != nil || quote.Total != "0.04" || quote.Discount != "0.01" {
		t.Fatal("El descuento de medio centavo no usó redondeo comercial")
	}
	quote, err = Calculate(snapshot, request, calculationTime.Add(time.Hour), time.UTC)
	if err != nil || quote.Total != "0.05" || quote.Promotions[0].Applied {
		t.Fatal("La promoción ignoró el fin exclusivo de horario")
	}
	fullDay := Schedule{Windows: []TimeWindow{{Start: "00:00", End: "24:00"}}}
	if err := validateSchedule(fullDay); err != nil || !scheduleAvailable(fullDay, calculationTime, time.UTC) {
		t.Fatal("No se admitió cierre a las 24:00")
	}
}

func TestGranularProductPermissions(t *testing.T) {
	snapshot, _ := referenceSnapshot()
	original := snapshot.Products["plate"]
	original.Groups = nil
	original.Variants = []Variant{{ID: "variant", Active: true}}
	actor := Actor{Permissions: []string{"catalog.products.manage", "catalog.prices.manage"}}
	changed := original
	changed.Groups = []ProductGroup{{GroupID: "sides"}}
	restricted := Actor{Permissions: []string{"catalog.products.manage"}}
	if err := validateProductPermissions(&changed, &original, restricted); err == nil {
		t.Fatal("Se cambiaron condiciones económicas sin permiso de precios")
	}
	changed = original
	changed.Variants = []Variant{{ID: "variant", Active: true, Availability: Schedule{Days: []int{1}}}}
	if err := validateProductPermissions(&changed, &original, actor); err == nil {
		t.Fatal("Se cambiaron horarios de presentación sin permiso")
	}
	changed = original
	changed.Variants = nil
	if err := validateProductPermissions(&changed, &original, actor); err == nil {
		t.Fatal("Se desactivó una presentación por omisión sin permiso de estado")
	}
}
