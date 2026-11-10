package inventory

import (
	"testing"
)

// Conversiones de unidades y presentaciones (§4 de la especificación). Resultados exactos a mano.

var (
	gram       = measure{Dimension: "mass", Factor: number("1")}
	kilogram   = measure{Dimension: "mass", Factor: number("1000")}
	ounce      = measure{Dimension: "mass", Factor: number("28.349523125")}
	milliliter = measure{Dimension: "volume", Factor: number("1")}
	liter      = measure{Dimension: "volume", Factor: number("1000")}
	piece      = measure{Dimension: "count", Factor: number("1")}
	portion    = measure{Dimension: "portion", Factor: number("1")}
)

func mustConvert(t *testing.T, quantity string, from, to measure) string {
	t.Helper()
	converted, err := convertQuantity(number(quantity), from, to)
	if err != nil {
		t.Fatal(err)
	}
	return converted.String()
}

func TestConversionsBetweenMassUnits(t *testing.T) {
	// 1 kg = 1,000 g; 2.5 kg = 2,500 g; 250 g = 0.25 kg; 1 g = 0.001 kg.
	cases := []struct {
		quantity string
		from, to measure
		expected string
	}{
		{"1", kilogram, gram, "1000"},
		{"2.5", kilogram, gram, "2500"},
		{"250", gram, kilogram, "0.25"},
		{"1", gram, kilogram, "0.001"},
		{"0.123456", kilogram, gram, "123.456"},
	}
	for _, testCase := range cases {
		if actual := mustConvert(t, testCase.quantity, testCase.from, testCase.to); actual != testCase.expected {
			t.Fatalf("%s: esperado %s, obtenido %s", testCase.quantity, testCase.expected, actual)
		}
	}
}

func TestConversionsBetweenVolumeUnits(t *testing.T) {
	// 1 l = 1,000 ml; 0.75 l = 750 ml; 355 ml = 0.355 l.
	if actual := mustConvert(t, "1", liter, milliliter); actual != "1000" {
		t.Fatalf("1 l: %s", actual)
	}
	if actual := mustConvert(t, "0.75", liter, milliliter); actual != "750" {
		t.Fatalf("0.75 l: %s", actual)
	}
	if actual := mustConvert(t, "355", milliliter, liter); actual != "0.355" {
		t.Fatalf("355 ml: %s", actual)
	}
}

func TestConversionPrecisionKeepsSixDecimals(t *testing.T) {
	// 1 oz = 28.349523125 g → 28.349523 (el séptimo decimal es 1: se redondea hacia abajo).
	if actual := mustConvert(t, "1", ounce, gram); actual != "28.349523" {
		t.Fatalf("1 oz: %s", actual)
	}
	// 3 oz = 85.048569375 g → 85.048569 (séptimo decimal 3).
	if actual := mustConvert(t, "3", ounce, gram); actual != "85.048569" {
		t.Fatalf("3 oz: %s", actual)
	}
	// 0.000002 oz = 0.00005669904625 g → 0.000057 (mitad alejándose de cero).
	if actual := mustConvert(t, "0.000002", ounce, gram); actual != "0.000057" {
		t.Fatalf("0.000002 oz: %s", actual)
	}
	// 1 g = 1 / 28.349523125 oz = 0.035273961… → 0.035274.
	if actual := mustConvert(t, "1", gram, ounce); actual != "0.035274" {
		t.Fatalf("1 g en oz: %s", actual)
	}
	// 0.0004 g = 0.0000004 kg: se redondea a cero y se rechaza en lugar de registrar nada.
	if _, err := convertQuantity(number("0.0004"), gram, kilogram); err == nil {
		t.Fatal("se aceptó una conversión que se redondea a cero")
	}
}

func TestConversionsRejectIncompatibleDimensions(t *testing.T) {
	for _, pair := range [][2]measure{{kilogram, piece}, {liter, gram}, {piece, portion}, {portion, milliliter}} {
		if _, err := convertQuantity(number("1"), pair[0], pair[1]); err == nil {
			t.Fatalf("se convirtió entre %s y %s", pair[0].Dimension, pair[1].Dimension)
		}
	}
}

func TestPresentationConversions(t *testing.T) {
	// Caja de refrescos con 24 botellas: 3 cajas = 72 piezas; 0.5 caja = 12 piezas.
	box := number("24")
	for quantity, expected := range map[string]string{"3": "72", "0.5": "12", "1": "24"} {
		converted, err := presentationQuantity(number(quantity), box)
		if err != nil {
			t.Fatal(err)
		}
		if converted.String() != expected {
			t.Fatalf("%s cajas: esperado %s, obtenido %s", quantity, expected, converted.String())
		}
	}
	// Paquete de pan con 12 piezas: 2.5 paquetes = 30 piezas.
	converted, err := presentationQuantity(number("2.5"), number("12"))
	if err != nil || converted.String() != "30" {
		t.Fatalf("paquetes: %v %v", converted, err)
	}
	// Costal de 25 kg de un insumo en gramos (25,000 g): 1.5 costales = 37,500 g.
	sack, err := convertQuantity(number("25"), kilogram, gram)
	if err != nil {
		t.Fatal(err)
	}
	converted, err = presentationQuantity(number("1.5"), sack)
	if err != nil || converted.String() != "37500" {
		t.Fatalf("costales: %v %v", converted, err)
	}
	if _, err := presentationQuantity(number("1"), zero); err == nil {
		t.Fatal("se aceptó una presentación sin equivalencia")
	}
}

func TestQuantityParsingIsStrict(t *testing.T) {
	for _, text := range []string{"1", "0.5", "12.345678", "1000000000"} {
		if _, err := parseQuantity(text, "La cantidad"); err != nil {
			t.Fatalf("se rechazó %q: %v", text, err)
		}
	}
	for _, text := range []string{"0", "-1", "1.2345678", "1e3", "1,000", " ", "+1", ".5", "1000000001"} {
		if _, err := parseQuantity(text, "La cantidad"); err == nil {
			t.Fatalf("se aceptó %q", text)
		}
	}
	if value, err := parseCost("33.3333333333", "El costo"); err != nil || value.String() != "33.3333333333" {
		t.Fatalf("costo con diez decimales: %v %v", value, err)
	}
	if _, err := parseCost("1.12345678901", "El costo"); err == nil {
		t.Fatal("se aceptó un costo con once decimales")
	}
}

func TestDecimalTexts(t *testing.T) {
	cases := map[string][3]string{
		// valor exacto, a centavos, costo a cuatro decimales
		"500.000000":    {"500.00", "500.00", "500.00"},
		"33.3333333333": {"33.3333333333", "33.33", "33.3333"},
		"-50":           {"-50.00", "-50.00", "-50.00"},
		"0.0125":        {"0.0125", "0.01", "0.0125"},
		"0.000009":      {"0.000009", "0.00", "0.00"},
	}
	for text, expected := range cases {
		value := number(text)
		if exactMoneyText(value) != expected[0] || displayMoneyText(value) != expected[1] || displayUnitCostText(value) != expected[2] {
			t.Fatalf("%s: %s %s %s", text, exactMoneyText(value), displayMoneyText(value), displayUnitCostText(value))
		}
	}
	if quantityText(number("20.000000")) != "20" || quantityText(number("-1.500000")) != "-1.5" {
		t.Fatal("las cantidades deben omitir ceros finales")
	}
}
