package catalog

import (
	"regexp"
	"testing"
)

func TestPaletteHasSixteenDistinctNamedColors(t *testing.T) {
	if len(Palette) != 16 {
		t.Fatalf("la paleta debe tener 16 colores, tiene %d", len(Palette))
	}
	pattern := regexp.MustCompile(`^#[0-9a-f]{6}$`)
	hexes := map[string]bool{}
	names := map[string]bool{}
	for _, color := range Palette {
		if !pattern.MatchString(color.Hex) {
			t.Fatalf("el color %q no está en forma canónica #rrggbb", color.Hex)
		}
		if color.Name == "" || hexes[color.Hex] || names[color.Name] {
			t.Fatalf("color repetido o sin nombre: %+v", color)
		}
		hexes[color.Hex] = true
		names[color.Name] = true
	}
}

func TestCategoryColorAcceptsPaletteEmptyAndInheritedOnly(t *testing.T) {
	// Sin color y colores de la paleta (en cualquier capitalización) se aceptan y normalizan.
	empty := Category{}
	if err := validateCategoryColor(&empty, nil); err != nil {
		t.Fatalf("sin color debe ser válido: %v", err)
	}
	red := Category{Color: "#D64545"}
	if err := validateCategoryColor(&red, nil); err != nil || red.Color != "#d64545" {
		t.Fatalf("un color de la paleta debe aceptarse en forma canónica: %q %v", red.Color, err)
	}
	// Un color nuevo fuera de la paleta se rechaza, también al editar un registro sin color.
	custom := Category{Color: "#14665c"}
	if err := validateCategoryColor(&custom, nil); err == nil {
		t.Fatal("se aceptó un color nuevo fuera de la paleta")
	}
	if err := validateCategoryColor(&custom, &Category{}); err == nil {
		t.Fatal("se aceptó un color fuera de la paleta al editar una categoría sin color")
	}
	// El color heredado (guardado antes de la paleta) se conserva tal cual al editar.
	inherited := Category{Color: "#14665c"}
	if err := validateCategoryColor(&inherited, &Category{Color: "#14665c"}); err != nil || inherited.Color != "#14665c" {
		t.Fatalf("el color heredado no se conservó: %q %v", inherited.Color, err)
	}
	// Cambiar el heredado por otro color fuera de la paleta no está permitido.
	other := Category{Color: "#123456"}
	if err := validateCategoryColor(&other, &Category{Color: "#14665c"}); err == nil {
		t.Fatal("se aceptó sustituir el color heredado por otro fuera de la paleta")
	}
	for _, value := range []string{"rojo", "#d6454", "d64545", "#d64545ff"} {
		candidate := Category{Color: value}
		if err := validateCategoryColor(&candidate, nil); err == nil {
			t.Fatalf("se aceptó el valor de color inválido %q", value)
		}
	}
}
