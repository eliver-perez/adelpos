package documents

import (
	"strings"
	"testing"
)

// Columnas por impresora (etapa 9C, §5.3): lista cerrada por papel, salida idéntica con el valor
// predeterminado y renglones recalculados al nuevo ancho con 48 y 32 columnas (el texto grande de
// Cocina usa la mitad).

func TestColumnChoicesAreClosedPerPaper(t *testing.T) {
	cases := map[int][]int{80: {42, 48}, 58: {30, 32}}
	for paper, choices := range cases {
		got := ColumnChoices(paper)
		if len(got) != 2 || got[0] != choices[0] || got[1] != choices[1] || got[0] != ColumnsFor(paper) {
			t.Fatalf("Columnas de %d mm: %v", paper, got)
		}
		for _, columns := range choices {
			if !ValidColumns(paper, columns) {
				t.Fatalf("%d columnas deben admitirse en %d mm", columns, paper)
			}
		}
	}
	for _, rejected := range [][2]int{{80, 30}, {80, 32}, {80, 40}, {80, 0}, {58, 42}, {58, 48}, {58, 31}, {76, 42}} {
		if ValidColumns(rejected[0], rejected[1]) {
			t.Fatalf("%d columnas no deben admitirse en %d mm", rejected[1], rejected[0])
		}
	}
}

// Con las columnas predeterminadas explícitas la salida de los diez tipos es idéntica a la de la 9B
// (Columns en cero): la impresora con el valor predeterminado imprime exactamente igual.
func TestDefaultColumnsKeepEveryDocumentIdentical(t *testing.T) {
	identity := Identity{Name: "Restaurante con un nombre comercial larguísimo que corta en dos renglones"}
	for _, info := range Kinds {
		for _, paper := range []int{58, 80} {
			data := SampleData(info.Kind, identity)
			before, err := Render(info.Kind, Default(info.Kind), data, RenderOptions{PaperWidth: paper, IncludeExpected: true})
			if err != nil {
				t.Fatal(err)
			}
			explicit, err := Render(info.Kind, Default(info.Kind), data, RenderOptions{PaperWidth: paper, Columns: ColumnsFor(paper), IncludeExpected: true})
			if err != nil {
				t.Fatal(err)
			}
			if before.Text() != explicit.Text() || before.Columns != explicit.Columns {
				t.Fatalf("%s a %d mm cambió con las columnas predeterminadas explícitas", info.Kind, paper)
			}
		}
	}
}

// Con 48 y 32 columnas cada renglón cabe en el ancho, los separadores y los renglones «concepto …
// importe» ocupan exactamente el ancho nuevo y el documento es distinto del predeterminado; el
// texto grande de Cocina usa la mitad de las columnas.
func TestWideColumnsRecalculateRowsAndCuts(t *testing.T) {
	identity := Identity{Name: "Restaurante con un nombre comercial larguísimo que corta en dos renglones"}
	for _, info := range Kinds {
		for paper, wide := range map[int]int{80: 48, 58: 32} {
			data := SampleData(info.Kind, identity)
			layout, err := Render(info.Kind, Default(info.Kind), data, RenderOptions{PaperWidth: paper, Columns: wide, IncludeExpected: true})
			if err != nil {
				t.Fatal(err)
			}
			if layout.Columns != wide || layout.PaperWidth != paper {
				t.Fatalf("%s a %d mm: columnas %d", info.Kind, paper, layout.Columns)
			}
			full := 0
			for _, line := range layout.Lines {
				width := runeCount(line.Text)
				if width > wide {
					t.Fatalf("%s a %d columnas: renglón de %d caracteres %q", info.Kind, wide, width, line.Text)
				}
				if width == wide {
					full++
				}
			}
			if full == 0 {
				t.Fatalf("%s a %d columnas: ningún separador o renglón justificado ocupa el ancho nuevo:\n%s", info.Kind, wide, layout.Text())
			}
			normal, _ := Render(info.Kind, Default(info.Kind), data, RenderOptions{PaperWidth: paper, IncludeExpected: true})
			if normal.Text() == layout.Text() {
				t.Fatalf("%s a %d columnas debía recalcular los renglones", info.Kind, wide)
			}
		}
	}
	// Renglón «concepto … importe»: el importe termina en la última columna.
	receipt, err := Render(KindReceipt, Default(KindReceipt), SampleData(KindReceipt, identity), RenderOptions{PaperWidth: 80, Columns: 48})
	if err != nil {
		t.Fatal(err)
	}
	total := ""
	for _, line := range receipt.Lines {
		if strings.HasPrefix(line.Text, "TOTAL ") {
			total = line.Text
		}
	}
	if runeCount(total) != 48 || strings.HasSuffix(total, " ") {
		t.Fatalf("El total no quedó justificado a 48 columnas: %q", total)
	}
	// Texto grande de Cocina: la mitad de las columnas elegidas.
	for paper, wide := range map[int]int{80: 48, 58: 32} {
		data := SampleData(KindKitchenOrder, identity)
		data.Kitchen.Settings.TextSize = TextSizeLarge
		layout, err := Render(KindKitchenOrder, Default(KindKitchenOrder), data, RenderOptions{PaperWidth: paper, Columns: wide})
		if err != nil {
			t.Fatal(err)
		}
		if layout.Columns != wide/2 || layout.TextSize != TextSizeLarge {
			t.Fatalf("Texto grande a %d columnas: %d", wide, layout.Columns)
		}
		for _, line := range layout.Lines {
			if runeCount(line.Text) > wide/2 {
				t.Fatalf("Texto grande a %d columnas: renglón de más de %d: %q", wide, wide/2, line.Text)
			}
		}
	}
}

// La prueba de ancho imprime la regla numerada del ancho elegido, una línea completa de guiones y un
// renglón «producto … importe» justificado, marcada como prueba al inicio y al final, en texto plano.
func TestWidthTestRuler(t *testing.T) {
	for _, test := range []struct {
		paper, columns, expected int
	}{{80, 48, 48}, {80, 42, 42}, {58, 32, 32}, {58, 30, 30}, {80, 30, 42}, {58, 48, 30}} {
		layout := RenderWidthTest(WidthTest{PaperWidth: test.paper, Columns: test.columns, PrinterName: "Cocina caliente",
			LocalDateTime: "11/11/2026 18:30"})
		if layout.Columns != test.expected || !layout.Plain() || layout.PaperWidth != test.paper {
			t.Fatalf("Prueba de %d mm a %d columnas: %+v", test.paper, test.columns, layout)
		}
		lines := strings.Split(strings.TrimSuffix(layout.Text(), "\n"), "\n")
		if strings.TrimSpace(lines[0]) != WidthTestMarker || strings.TrimSpace(lines[len(lines)-1]) != WidthTestMarker {
			t.Fatalf("La prueba de ancho debe marcarse al inicio y al final:\n%s", layout.Text())
		}
		ruler := strings.Repeat("1234567890", 5)[:test.expected]
		dashes := strings.Repeat("-", test.expected)
		found := map[string]bool{}
		for index, line := range lines {
			if runeCount(line) > test.expected {
				t.Fatalf("Renglón más ancho que %d: %q", test.expected, line)
			}
			if line == ruler {
				found["ruler"] = true
				if index == 0 || !strings.HasPrefix(lines[index-1], strings.Repeat(" ", 9)+"1") {
					t.Fatalf("Falta la línea de decenas sobre la regla:\n%s", layout.Text())
				}
			}
			if line == dashes {
				found["dashes"] = true
			}
			if strings.HasPrefix(line, "Producto de ejemplo") && strings.HasSuffix(line, "$1,234.50") && runeCount(line) == test.expected {
				found["row"] = true
			}
		}
		if len(found) != 3 {
			t.Fatalf("Faltan la regla, los guiones o el renglón de ejemplo a %d columnas (%v):\n%s", test.expected, found, layout.Text())
		}
		if !strings.Contains(layout.Text(), "Papel de "+map[int]string{80: "80", 58: "58"}[test.paper]+" mm") {
			t.Fatalf("La prueba debe indicar el papel:\n%s", layout.Text())
		}
	}
}
