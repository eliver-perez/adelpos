package stationscope

import (
	"slices"
	"testing"
)

// Estaciones de ejemplo en el orden de Cocina: la terminal muestra Barra y Postres; el usuario atiende
// Cocina caliente y Barra.
func exampleStations() []Station {
	return []Station{
		{ID: "a", Name: "Cocina caliente", InTerminal: false, InUser: true},
		{ID: "b", Name: "Barra", InTerminal: true, InUser: true},
		{ID: "c", Name: "Postres", InTerminal: true, InUser: false},
	}
}

func TestComputeIntersectsTerminalAndUser(t *testing.T) {
	scope := Compute(false, false, false, exampleStations())
	if scope.All || !scope.Common() || !slices.Equal(scope.IDs(), []string{"b"}) {
		t.Fatalf("intersección: all=%v ids=%v", scope.All, scope.IDs())
	}
	if !scope.Allows("b") || scope.Allows("a") || scope.Allows("c") || scope.Allows("otra") {
		t.Fatal("solo Barra está en la terminal y en el usuario")
	}
	summary := scope.Summary()
	if !slices.Equal(summary.TerminalStations, []string{"Barra", "Postres"}) ||
		!slices.Equal(summary.UserStations, []string{"Cocina caliente", "Barra"}) ||
		summary.TerminalAll || summary.UserAll || summary.UserAllByPermission {
		t.Fatalf("resumen: %+v", summary)
	}
}

func TestComputeAllFlags(t *testing.T) {
	// Terminal y usuario con «todas»: no se filtra, también las estaciones que se creen después.
	everything := Compute(true, true, false, exampleStations())
	if !everything.All || !everything.Allows("nueva") || !everything.Common() || len(everything.IDs()) != 0 {
		t.Fatalf("todas: %+v", everything.Summary())
	}
	// Terminal con «todas» y usuario con una selección: se ve la del usuario.
	userOnly := Compute(true, false, false, exampleStations())
	if userOnly.All || !slices.Equal(userOnly.IDs(), []string{"a", "b"}) {
		t.Fatalf("selección del usuario: %v", userOnly.IDs())
	}
	if len(userOnly.Summary().TerminalStations) != 0 || !userOnly.Summary().TerminalAll {
		t.Fatalf("la terminal con «todas» no lista estaciones: %+v", userOnly.Summary())
	}
	// Con kitchen.stations.all el usuario atiende todas aunque tenga una selección guardada; la terminal
	// sigue decidiendo lo que muestra.
	permission := Compute(false, false, true, exampleStations())
	if permission.All || !slices.Equal(permission.IDs(), []string{"b", "c"}) {
		t.Fatalf("permiso global en una terminal con selección: %v", permission.IDs())
	}
	if summary := permission.Summary(); !summary.UserAll || !summary.UserAllByPermission || len(summary.UserStations) != 0 {
		t.Fatalf("resumen con permiso global: %+v", summary)
	}
}

func TestComputeWithoutCommonStations(t *testing.T) {
	stations := []Station{
		{ID: "a", Name: "Cocina caliente", InTerminal: false, InUser: true},
		{ID: "b", Name: "Barra", InTerminal: true, InUser: false},
	}
	scope := Compute(false, false, false, stations)
	if scope.All || scope.Common() || len(scope.IDs()) != 0 || scope.Allows("a") || scope.Allows("b") {
		t.Fatalf("sin estaciones en común: %+v %v", scope.Summary(), scope.IDs())
	}
	if summary := scope.Summary(); summary.Common || summary.All {
		t.Fatalf("el resumen debe informar que no hay estaciones en común: %+v", summary)
	}
	// Una terminal sin «todas» y sin selección no muestra nada (nunca se amplía por falta de datos).
	unassigned := []Station{
		{ID: "a", Name: "Cocina caliente", InTerminal: false, InUser: false},
		{ID: "b", Name: "Barra", InTerminal: false, InUser: false},
	}
	empty := Compute(false, true, false, unassigned)
	if empty.Common() || len(empty.Summary().TerminalStations) != 0 {
		t.Fatalf("terminal sin estaciones: %+v", empty.Summary())
	}
}

func TestEverything(t *testing.T) {
	scope := Everything()
	if !scope.All || !scope.Common() || !scope.Allows("cualquiera") {
		t.Fatal("el alcance sin filtro permite todas las estaciones")
	}
}
