package kitchen

import (
	"encoding/json"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
)

// Etapa 9A, §3.1: con la opción «El precio puede bajar al cambiar complementos» la diferencia del
// grupo lleva signo y el desglose del producto agrega su ajuste. La parte que recibe la cocina no
// contiene ninguno de esos importes, igual que los demás.
func TestPreparationPartHidesSignedComplementAmounts(t *testing.T) {
	product := catalog.ProductBreakdown{
		ProductID: "plate", Name: "Filete", BasePrice: "150.00", UnitPrice: "140.00", GroupExtras: "-10.00",
		PriceCanDecrease: true, ComplementAdjustment: "-10.00",
		Groups: []catalog.GroupBreakdown{{
			GroupID: "sides", Name: "Guarniciones", Mode: "value", OriginalValue: "35.00", RequestedValue: "25.00",
			Difference: "-10.00", Extras: "0.00",
			Options: []catalog.OptionBreakdown{{OptionID: "beans", Name: "Frijoles", Included: "0", Requested: "1"}},
		}},
	}
	part := Part{
		Path: "root", ProductID: product.ProductID, Name: product.Name, Quantity: 1,
		Ingredients: preparationIngredients(product.Ingredients), Groups: preparationGroups(product.Groups, catalog.Snapshot{}),
	}
	encoded, err := json.Marshal(part)
	if err != nil {
		t.Fatal(err)
	}
	for _, amount := range []string{"-10.00", "35.00", "25.00", "150.00", "140.00", "complement_adjustment", "price_can_decrease"} {
		if strings.Contains(string(encoded), amount) {
			t.Fatalf("La parte de cocina contiene un importe (%s): %s", amount, encoded)
		}
	}
	if product.Groups[0].Difference != "-10.00" {
		t.Fatal("Limpiar la parte de cocina alteró el desglose comercial")
	}
	if !strings.Contains(string(encoded), "Frijoles") {
		t.Fatalf("La parte perdió la opción elegida: %s", encoded)
	}
}
