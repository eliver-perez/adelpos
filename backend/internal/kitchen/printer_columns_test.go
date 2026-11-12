package kitchen

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/documents"
)

// Columnas por impresora (etapa 9C, §5.3): validación en Go con la lista cerrada por papel, el
// predeterminado guardado como NULL, la fotografía anterior a la 9C con el predeterminado, el ancho
// del renderizador y la densidad de CUPS en proporción, sin cambiar nada con el predeterminado.

func columnsOf(value int) *int {
	return &value
}

func TestPrinterColumnsAreAClosedListPerPaper(t *testing.T) {
	base := func(paper int, columns *int) Printer {
		return Printer{Name: "Cocina", Transport: "cups", Destination: "COCINA", PaperWidth: paper, Copies: 1, Port: 9100, Columns: columns}
	}
	for _, accepted := range []struct {
		paper   int
		columns *int
		stored  *int
	}{
		{80, nil, nil}, {80, columnsOf(42), nil}, {80, columnsOf(48), columnsOf(48)},
		{58, nil, nil}, {58, columnsOf(30), nil}, {58, columnsOf(32), columnsOf(32)},
	} {
		printer := base(accepted.paper, accepted.columns)
		if err := validatePrinter(&printer); err != nil {
			t.Fatalf("%d mm con %v columnas debía aceptarse: %v", accepted.paper, accepted.columns, err)
		}
		if (printer.Columns == nil) != (accepted.stored == nil) || (printer.Columns != nil && *printer.Columns != *accepted.stored) {
			t.Fatalf("%d mm con %v columnas se guarda como %v", accepted.paper, accepted.columns, printer.Columns)
		}
	}
	for _, rejected := range []struct{ paper, columns int }{{80, 30}, {80, 32}, {80, 44}, {80, 0}, {58, 48}, {58, 42}, {58, -1}} {
		printer := base(rejected.paper, columnsOf(rejected.columns))
		err := validatePrinter(&printer)
		var problem *Error
		if err == nil || !errorAs(err, &problem) || problem.Status != 400 || !strings.Contains(problem.Message, "columnas de texto") {
			t.Fatalf("%d columnas en %d mm debían rechazarse con un mensaje claro: %v", rejected.columns, rejected.paper, err)
		}
	}
	// Cambiar el papel a 58 mm sin elegir de nuevo las columnas se rechaza explicando qué elegir.
	changed := base(58, columnsOf(48))
	err := validatePrinter(&changed)
	var problem *Error
	if !errorAs(err, &problem) || !strings.Contains(problem.Message, "Con papel de 58 mm las columnas de texto son 30 (predeterminado) o 32") ||
		!strings.Contains(problem.Message, "Si cambiaste el papel") {
		t.Fatalf("Mensaje al cambiar el papel: %v", err)
	}
}

// La fotografía de una impresora guardada antes de la 9C (sin el campo) usa el predeterminado; la
// de una con 48 columnas las conserva para la reimpresión.
func TestPrinterSnapshotColumns(t *testing.T) {
	var legacy Printer
	if err := json.Unmarshal([]byte(`{"id":"x","paper_width":80,"transport":"cups"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Columns != nil || legacy.TextColumns() != 42 || legacy.RenderOptions().Columns != 0 || legacy.RenderOptions().PaperWidth != 80 {
		t.Fatalf("Una fotografía anterior a la 9C debe usar el predeterminado: %+v", legacy.RenderOptions())
	}
	wide := Printer{PaperWidth: 80, Columns: columnsOf(48)}
	encoded, _ := json.Marshal(wide)
	var copied Printer
	if err := json.Unmarshal(encoded, &copied); err != nil {
		t.Fatal(err)
	}
	if copied.TextColumns() != 48 || copied.RenderOptions().Columns != 48 {
		t.Fatalf("La fotografía conserva las columnas: %s", encoded)
	}
	narrow := Printer{PaperWidth: 58, Columns: columnsOf(32)}
	if narrow.RenderOptions().Columns != 32 || narrow.RenderOptions().PaperWidth != 58 {
		t.Fatalf("58 mm a 32 columnas: %+v", narrow.RenderOptions())
	}
	// Un valor guardado que ya no corresponde al papel (no debería existir: lo impide la 0020) cae al
	// predeterminado en lugar de imprimir fuera del papel.
	stale := Printer{PaperWidth: 58, Columns: columnsOf(48)}
	if stale.TextColumns() != 30 || stale.RenderOptions().Columns != 0 {
		t.Fatalf("Columnas incoherentes con el papel: %+v", stale.RenderOptions())
	}
}

// Una comanda a 48 o 32 columnas recalcula sus renglones; con el predeterminado es idéntica.
func TestKitchenOrderUsesPrinterColumns(t *testing.T) {
	version := &documents.Version{Kind: documents.KindKitchenOrder, Template: documents.Default(documents.KindKitchenOrder)}
	for _, document := range equivalenceDocuments()[:2] {
		for paper, wide := range map[int]int{80: 48, 58: 32} {
			defaultLayout, err := renderDocumentWith(version, document, Printer{PaperWidth: paper}.RenderOptions())
			if err != nil {
				t.Fatal(err)
			}
			if defaultLayout.Text() != legacyRenderText(document, paper) {
				t.Fatalf("Con el predeterminado la comanda de %d mm debe ser la de siempre", paper)
			}
			layout, err := renderDocumentWith(version, document, Printer{PaperWidth: paper, Columns: columnsOf(wide)}.RenderOptions())
			if err != nil {
				t.Fatal(err)
			}
			if layout.Columns != wide || layout.Text() == defaultLayout.Text() {
				t.Fatalf("La comanda a %d columnas no se recalculó", wide)
			}
			for _, line := range strings.Split(layout.Text(), "\n") {
				if len([]rune(line)) > wide {
					t.Fatalf("Renglón de más de %d columnas: %q", wide, line)
				}
			}
		}
	}
}

// CUPS en texto: con el predeterminado los argumentos de lp no cambian (12 o 6 cpi); con 48 o 32
// columnas la densidad se ajusta en proporción (12 × columnas ÷ predeterminado).
func TestCUPSTextDensityFollowsColumns(t *testing.T) {
	density := func(arguments []string) string {
		for _, argument := range arguments {
			if strings.HasPrefix(argument, "cpi=") {
				return strings.TrimPrefix(argument, "cpi=")
			}
		}
		return ""
	}
	cases := []struct {
		paper   int
		columns *int
		size    string
		want    string
	}{
		{80, nil, "normal", "12"}, {80, columnsOf(42), "normal", "12"}, {80, nil, "large", "6"},
		{58, nil, "normal", "12"}, {58, columnsOf(30), "large", "6"},
		{80, columnsOf(48), "normal", "13.71"}, {80, columnsOf(48), "large", "6.86"},
		{58, columnsOf(32), "normal", "12.8"}, {58, columnsOf(32), "large", "6.4"},
	}
	for _, test := range cases {
		printer := Printer{Name: "Cocina", Transport: "cups", Destination: "COCINA", PaperWidth: test.paper, Copies: 1, Columns: test.columns}
		job := PrintJob{ID: "trabajo", PrinterSnapshot: printer, Content: "uno\ndos\n", TextSize: test.size}
		arguments := cupsTextArguments(job, "/tmp/archivo")
		if got := density(arguments); got != test.want {
			t.Fatalf("%d mm, %v columnas, %s: cpi=%s, se esperaba %s", test.paper, test.columns, test.size, got, test.want)
		}
		if test.want == "12" || test.want == "6" {
			if !slices.Equal(arguments, legacyCUPSArguments(job, "/tmp/archivo")) {
				t.Fatalf("Con el predeterminado los argumentos de lp no deben cambiar: %v", arguments)
			}
		}
	}
}

func errorAs(err error, target **Error) bool {
	problem, ok := err.(*Error)
	if ok {
		*target = problem
	}
	return ok
}
