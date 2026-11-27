package tableunions

import "testing"

// Etapa M1 (M1-D2): el nombre de la unión en orden natural, con la palabra «Mesa» una sola vez cuando todas la llevan.
func TestUnionName(t *testing.T) {
	cases := []struct {
		names []string
		want  string
	}{
		{[]string{"6", "5"}, "5 + 6"},
		{[]string{"10", "2"}, "2 + 10"},
		{[]string{"Mesa 6", "Mesa 5"}, "Mesa 5 + 6"},
		{[]string{"mesa 10", "Mesa 2"}, "Mesa 2 + 10"},
		{[]string{"Mesa 3b", "Mesa 3a"}, "Mesa 3a + 3b"},
		{[]string{"3B", "3a", "3C"}, "3a + 3B + 3C"},
		{[]string{"Mesa 6", "5"}, "5 + Mesa 6"},
		{[]string{"Terraza 2", "Terraza 1"}, "Terraza 1 + Terraza 2"},
		{[]string{"Mesanine", "Mesa 1"}, "Mesa 1 + Mesanine"},
		{[]string{"Mésa 7", "Mesa 8"}, "Mesa 7 + 8"},
	}
	for _, check := range cases {
		tables := []NamedTable{}
		for index, name := range check.names {
			tables = append(tables, NamedTable{ID: string(rune('a' + index)), Name: name, Seats: 2})
		}
		SortTables(tables)
		if got := UnionName(tables); got != check.want {
			t.Errorf("%v: %q, esperado %q", check.names, got, check.want)
		}
		if seats := Seats(tables); seats != 2*len(check.names) {
			t.Errorf("%v: capacidad %d", check.names, seats)
		}
	}
}

func TestNaturalCompare(t *testing.T) {
	ordered := []string{"1", "2", "02", "10", "10a", "10B", "a", "A1", "a2", "b"}
	for index := 0; index+1 < len(ordered); index++ {
		if NaturalCompare(ordered[index], ordered[index+1]) >= 0 {
			t.Errorf("%q debe ir antes que %q", ordered[index], ordered[index+1])
		}
		if NaturalCompare(ordered[index+1], ordered[index]) <= 0 {
			t.Errorf("%q debe ir después que %q", ordered[index+1], ordered[index])
		}
	}
	if NaturalCompare("Mesa 3", "mesa 3") != 0 {
		t.Error("el orden no distingue mayúsculas")
	}
}
