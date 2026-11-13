package maintenance

import (
	"strings"
	"testing"
)

// Revisión independiente de la 9F: los motivos de la vista previa nombran las promociones que no cambian
// (M1) y explican qué pasa con la ruta de un producto o una categoría del restaurante (B1), sin términos
// técnicos.
func TestPromotionSentence(t *testing.T) {
	cases := map[string][]string{
		"Participa en la promoción «2×1 de martes», que no cambia.":        {"2×1 de martes"},
		"Participa en las promociones «A» y «B», que no cambian.":          {"A", "B"},
		"Participa en las promociones «A», «B» y «Combo», que no cambian.": {"A", "B", "Combo"},
	}
	for expected, names := range cases {
		if got := promotionSentence(names); got != expected {
			t.Fatalf("promotionSentence(%v) = %q, se esperaba %q", names, got, expected)
		}
	}
}

func TestRestaurantRouteReason(t *testing.T) {
	category := restaurantRouteReason(routeRow{
		ProductRoute: false,
		Owner:        "Platillos",
		StationNames: []string{"Cocina caliente", "Freidoras"},
	})
	expected := "Solo enviaba a estaciones de prueba («Cocina caliente» y «Freidoras»); se quita: sus productos sin " +
		"ruta propia quedarán sin ruta de preparación hasta que le asignes otra."
	if category != expected {
		t.Fatalf("motivo de la ruta de una categoría: %q", category)
	}
	product := restaurantRouteReason(routeRow{ProductRoute: true, Owner: "Tacos", StationNames: []string{"Freidoras"}})
	if !strings.HasPrefix(product, "Solo enviaba a estaciones de prueba («Freidoras»); se quita: el producto usará la ruta de su categoría") {
		t.Fatalf("motivo de la ruta de un producto: %q", product)
	}
	for _, reason := range []string{category, product} {
		if strings.Contains(reason, "_") || strings.Contains(strings.ToLower(reason), "route") {
			t.Fatalf("motivo con términos técnicos: %q", reason)
		}
	}
}
