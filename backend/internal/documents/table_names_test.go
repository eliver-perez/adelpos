package documents

import (
	"strings"
	"testing"
)

// «Mesa» sin repetir (etapa 9F, §4.6): el contexto de la precuenta y el ticket no antepone «Mesa » a un
// nombre que ya empieza con esa palabra, sin distinguir mayúsculas ni acentos.
func TestTableContextDoesNotRepeatWord(t *testing.T) {
	cases := map[string]string{
		"Mesa 1":     "Mesa 1",
		"MESA 2":     "MESA 2",
		"mesa 3":     "mesa 3",
		"Mésa 4":     "Mésa 4",
		"Mesa":       "Mesa",
		"Mesa-5":     "Mesa-5",
		"Mesa5":      "Mesa5",
		"  Mesa 6 ":  "  Mesa 6 ",
		"4":          "Mesa 4",
		"Terraza 2":  "Mesa Terraza 2",
		"Mesanine 1": "Mesa Mesanine 1",
		"Mesón 3":    "Mesa Mesón 3",
		"La mesa 7":  "Mesa La mesa 7",
	}
	for name, want := range cases {
		if got := TableContext(name); got != want {
			t.Fatalf("TableContext(%q) = %q, se esperaba %q", name, got, want)
		}
	}
}

// En una plantilla propia, el campo «Mesa:» no repite la palabra si el nombre ya la incluye; con otro
// nombre o con otra etiqueta no cambia.
func TestTableFieldDoesNotRepeatWord(t *testing.T) {
	render := func(label, table string) string {
		data := Data{Kitchen: &KitchenData{Table: table}}
		writer := &writer{family: FamilyKitchen, width: 42}
		renderField(KindKitchenOrder, &FieldOptions{Items: []FieldItem{{Key: "table", Label: label}}, Layout: FieldInline}, data, writer)
		texts := []string{}
		for _, line := range writer.lines {
			texts = append(texts, line.text)
		}
		return strings.TrimSpace(strings.Join(texts, "\n"))
	}
	cases := []struct{ label, table, want string }{
		{"Mesa:", "Mesa 1", "Mesa 1"},
		{"mesa", "MESA 2", "MESA 2"},
		{"Mesa:", "Terraza", "Mesa: Terraza"},
		{"Lugar:", "Mesa 1", "Lugar: Mesa 1"},
		{"", "Mesa 1", "Mesa 1"},
	}
	for _, testCase := range cases {
		if got := render(testCase.label, testCase.table); got != testCase.want {
			t.Fatalf("%q + %q = %q, se esperaba %q", testCase.label, testCase.table, got, testCase.want)
		}
	}
}
