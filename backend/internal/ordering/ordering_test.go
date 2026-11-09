package ordering

import (
	"errors"
	"slices"
	"testing"
)

const (
	breakfast = "00000000-0000-4000-8000-000000000001"
	drinks    = "00000000-0000-4000-8000-000000000002"
	desserts  = "00000000-0000-4000-8000-000000000003"
	soups     = "00000000-0000-4000-8000-000000000004"
)

func text(value string) *string {
	return &value
}

func TestArrangeMovesToStartAndAfterSibling(t *testing.T) {
	current := []string{breakfast, drinks, desserts}
	cases := []struct {
		name  string
		id    string
		after string
		want  []string
	}{
		{"al inicio", desserts, "", []string{desserts, breakfast, drinks}},
		{"después del primero", desserts, breakfast, []string{breakfast, desserts, drinks}},
		{"al final", breakfast, desserts, []string{drinks, desserts, breakfast}},
		{"sin moverse", drinks, breakfast, []string{breakfast, drinks, desserts}},
		{"registro nuevo", soups, drinks, []string{breakfast, drinks, soups, desserts}},
		{"registro nuevo en lista vacía", soups, "", []string{soups}},
	}
	for _, scenario := range cases {
		siblings := current
		if scenario.name == "registro nuevo en lista vacía" {
			siblings = []string{}
		}
		got, err := Arrange(siblings, scenario.id, scenario.after)
		if err != nil {
			t.Fatalf("%s: %v", scenario.name, err)
		}
		if !slices.Equal(got, scenario.want) {
			t.Fatalf("%s: esperado %v, recibido %v", scenario.name, scenario.want, got)
		}
	}
	if !slices.Equal(current, []string{breakfast, drinks, desserts}) {
		t.Fatal("Arrange no debe modificar la lista recibida")
	}
}

func TestArrangeRejectsSelfAndForeignSiblings(t *testing.T) {
	current := []string{breakfast, drinks}
	var problem *Error
	if _, err := Arrange(current, drinks, drinks); !errors.As(err, &problem) || problem != errSelf {
		t.Fatalf("se aceptó colocar un registro después de sí mismo: %v", err)
	}
	if _, err := Arrange(current, drinks, soups); !errors.As(err, &problem) || problem != errNotSibling {
		t.Fatalf("se aceptó un hermano ajeno a la lista: %v", err)
	}
}

func TestPlanRenumbersInStepsAndReportsOnlyChanges(t *testing.T) {
	siblings := []Sibling{
		{ID: breakfast, SortOrder: 10},
		{ID: drinks, SortOrder: 20},
		{ID: desserts, SortOrder: 30},
	}
	changes, err := Plan(siblings, desserts, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{ID: desserts, From: 30, To: 10},
		{ID: breakfast, From: 10, To: 20},
		{ID: drinks, From: 20, To: 30},
	}
	if !slices.Equal(changes, want) {
		t.Fatalf("renumeración inesperada: %+v", changes)
	}
	// Un registro nuevo al final solo recibe su número: los hermanos conservan el suyo.
	changes, err = Plan(siblings, soups, desserts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changes, []Change{{ID: soups, From: 0, To: 40}}) {
		t.Fatalf("el registro nuevo alteró a sus hermanos: %+v", changes)
	}
	// Un registro nuevo en medio renumera solo a quienes cambian de número.
	changes, err = Plan(siblings, soups, breakfast)
	if err != nil {
		t.Fatal(err)
	}
	want = []Change{
		{ID: soups, From: 0, To: 20},
		{ID: drinks, From: 20, To: 30},
		{ID: desserts, From: 30, To: 40},
	}
	if !slices.Equal(changes, want) {
		t.Fatalf("inserción intermedia inesperada: %+v", changes)
	}
	// La lista vacía empieza en el primer múltiplo.
	changes, err = Plan([]Sibling{}, soups, "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changes, []Change{{ID: soups, From: 0, To: 10}}) {
		t.Fatalf("el primer registro no recibió 10: %+v", changes)
	}
}

func TestPlanKeepsStrictOrderAndNormalizesTies(t *testing.T) {
	strict := []Sibling{
		{ID: breakfast, SortOrder: 5},
		{ID: drinks, SortOrder: 7},
		{ID: desserts, SortOrder: 40},
	}
	changes, err := Plan(strict, drinks, breakfast)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("guardar sin mover no debe renumerar: %+v", changes)
	}
	// Datos anteriores con empates (todos en 0): el lugar dependería del nombre, así que la
	// posición elegida se hace explícita renumerando la lista.
	ties := []Sibling{
		{ID: breakfast, SortOrder: 0},
		{ID: drinks, SortOrder: 0},
		{ID: desserts, SortOrder: 0},
	}
	changes, err = Plan(ties, drinks, breakfast)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{ID: breakfast, From: 0, To: 10},
		{ID: drinks, From: 0, To: 20},
		{ID: desserts, From: 0, To: 30},
	}
	if !slices.Equal(changes, want) {
		t.Fatalf("la normalización de empates no fue la esperada: %+v", changes)
	}
	// Un registro nuevo al final de una lista con empates no toca a los hermanos.
	changes, err = Plan(ties, soups, desserts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changes, []Change{{ID: soups, From: 0, To: 10}}) {
		t.Fatalf("el registro nuevo al final alteró a sus hermanos: %+v", changes)
	}
}

func TestValidateRequiresExplicitAfter(t *testing.T) {
	if _, err := Validate(nil, drinks); err == nil {
		t.Fatal("se aceptó una posición ausente")
	}
	if _, err := Validate(&Position{}, drinks); err == nil {
		t.Fatal("se aceptó una posición sin «after»")
	}
	if after, err := Validate(&Position{After: text("")}, drinks); err != nil || after != "" {
		t.Fatalf("«al inicio» no fue válido: %q %v", after, err)
	}
	if after, err := Validate(&Position{After: text(breakfast)}, drinks); err != nil || after != breakfast {
		t.Fatalf("un hermano válido fue rechazado: %q %v", after, err)
	}
	for _, value := range []string{"Desayunos", "00000000-0000-4000-8000-00000000000Z", drinks} {
		if _, err := Validate(&Position{After: text(value)}, drinks); err == nil {
			t.Fatalf("se aceptó la posición inválida %q", value)
		}
	}
}

func TestScopesAreClosedAndComplete(t *testing.T) {
	for _, scope := range []Scope{
		CatalogCategories,
		CatalogProducts,
		RestaurantAreas,
		KitchenStations,
		PaymentMethods,
		CashExpenseCategories,
	} {
		if _, known := scopes[scope]; !known {
			t.Fatalf("falta la definición del alcance %d", scope)
		}
	}
	if _, known := scopes[Scope(99)]; known {
		t.Fatal("un alcance desconocido no debe tener tabla")
	}
	if scopes[CatalogProducts].group != "category_id" {
		t.Fatal("los productos deben ordenarse dentro de su categoría")
	}
}
