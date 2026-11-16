package administration

import (
	"errors"
	"strings"
	"testing"
)

// Reglas del PIN (etapa A1, fase B): de 4 a 6 dígitos y nunca uno trivial. La prueba recorre la lista completa de muy
// usados y ejemplos de cada regla general, y comprueba que un PIN ordinario pasa.
func TestPINRules(t *testing.T) {
	code := func(err error) string {
		var fault *Fault
		if errors.As(err, &fault) {
			return fault.Code
		}
		return ""
	}
	for _, pin := range []string{"", "1", "123", "1234567", "12a4", "4829 ", " 482", "48.29", "-482", "４８２９", "٤٨٢٩"} {
		if code(validatePIN(pin)) != "VALIDATION_ERROR" {
			t.Fatalf("%q: debía rechazarse por su forma", pin)
		}
	}
	trivial := []string{
		// Todos iguales.
		"0000", "1111", "99999", "777777",
		// Secuencias ascendentes y descendentes, también las que dan la vuelta.
		"1234", "0123", "6789", "7890", "8901", "12345", "123456", "4567", "4321", "3210", "9876", "0987", "54321", "654321",
		"098765", "210987",
		// Bloques repetidos de 2 o 3 dígitos.
		"1212", "6969", "2525", "0101", "121212", "696969", "123123", "520520", "789789", "012012",
	}
	trivial = append(trivial, commonPINs...)
	for _, pin := range trivial {
		if code(validatePIN(pin)) != "PIN_TOO_SIMPLE" {
			t.Fatalf("%q: debía rechazarse por trivial", pin)
		}
	}
	for _, pin := range commonPINs {
		if len(pin) < pinMinimumLength || len(pin) > pinMaximumLength || strings.Trim(pin, "0123456789") != "" {
			t.Fatalf("la lista de muy usados tiene un PIN imposible: %q", pin)
		}
	}
	for _, pin := range []string{"4829", "7305", "6152", "3958", "5071", "8061", "48291", "730519", "1235", "9182", "1213", "123124"} {
		if err := validatePIN(pin); err != nil {
			t.Fatalf("%q: debía aceptarse: %v", pin, err)
		}
	}
}

// El distintivo: nombre corto de hasta 20 caracteres (vacío = el nombre visible) y un color de la paleta de Go.
func TestBadgeValidation(t *testing.T) {
	name, color, err := normalizeBadge("  Ana  ", strings.ToUpper(BadgePalette[3].Hex))
	if err != nil || name == nil || *name != "Ana" || color == nil || *color != BadgePalette[3].Hex {
		t.Fatalf("distintivo válido: %v %v %v", name, color, err)
	}
	name, color, err = normalizeBadge("", "")
	if err != nil || name != nil || color != nil {
		t.Fatalf("sin distintivo: %v %v %v", name, color, err)
	}
	if _, _, err := normalizeBadge("Ñandú Martínez López", ""); err != nil {
		t.Fatalf("20 caracteres con acentos: %v", err)
	}
	for _, input := range [][2]string{{strings.Repeat("a", 21), ""}, {"Ana", "#123456"}, {"Ana", "azul"}, {"Ana", "#3591d4ff"}} {
		if _, _, err := normalizeBadge(input[0], input[1]); err == nil {
			t.Fatalf("distintivo inválido aceptado: %v", input)
		}
	}
	for name, initial := range map[string]string{"Ana": "A", "ángel": "Á", "  ñoño": "Ñ", "3er turno": "3", "": "", "··": ""} {
		if badgeInitial(name) != initial {
			t.Fatalf("inicial de %q: %q", name, badgeInitial(name))
		}
	}
}
