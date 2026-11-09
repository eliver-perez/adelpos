package features

import (
	"regexp"
	"testing"
)

// El registro debe cumplir la misma forma de código que valida PostgreSQL y no repetir funciones.
func TestKnownFeaturesAreWellFormed(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)
	seen := map[string]bool{}
	for _, definition := range Known {
		if !pattern.MatchString(definition.Code) {
			t.Fatalf("Código de función inválido: %q", definition.Code)
		}
		if seen[definition.Code] {
			t.Fatalf("Función repetida: %q", definition.Code)
		}
		seen[definition.Code] = true
		if definition.Name == "" || len(definition.Description) < 20 {
			t.Fatalf("La función %q necesita nombre y explicación", definition.Code)
		}
	}
	if !seen[USDCash] {
		t.Fatal("El pago en dólares no está registrado")
	}
	if _, found := Lookup("licencias"); found {
		t.Fatal("Se aceptó una función desconocida")
	}
}
