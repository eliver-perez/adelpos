package documents

import (
	"strconv"
	"strings"
)

// Prueba de ancho de una impresora (etapa 9C, §5.3): una regla numerada con el ancho elegido, una
// línea completa de guiones y un renglón «producto … importe» justificado, entre dos marcas de
// prueba. El personal la imprime antes de elegir 48 (80 mm) o 32 (58 mm) columnas para ver si el
// final del renglón se corta o pasa al siguiente. No usa datos de ventas: el importe es fijo.

// WidthTestMarker identifica la prueba de ancho al inicio y al final del papel.
const WidthTestMarker = "*** PRUEBA DE ANCHO ***"

// WidthTestSampleAmount es el importe fijo del renglón de ejemplo.
const WidthTestSampleAmount = "1234.50"

// WidthTest son los datos de una prueba de ancho.
type WidthTest struct {
	// PaperWidth es el papel de la impresora (58 u 80 mm).
	PaperWidth int
	// Columns son las columnas que se prueban; fuera de ColumnChoices se usa el predeterminado.
	Columns int
	// PrinterName y LocalDateTime identifican la hoja impresa.
	PrinterName   string
	LocalDateTime string
}

// RenderWidthTest produce la IR (texto plano) de una prueba de ancho.
func RenderWidthTest(test WidthTest) *Layout {
	paper := test.PaperWidth
	if paper != 58 {
		paper = 80
	}
	columns := test.Columns
	if !ValidColumns(paper, columns) {
		columns = ColumnsFor(paper)
	}
	texts := []string{alignText(WidthTestMarker, AlignCenter, columns)}
	texts = append(texts, wrapWords("Impresora: "+test.PrinterName, columns)...)
	texts = append(texts, wrapWords("Papel de "+strconv.Itoa(paper)+" mm · "+strconv.Itoa(columns)+" columnas", columns)...)
	if test.LocalDateTime != "" {
		texts = append(texts, test.LocalDateTime)
	}
	texts = append(texts, "", rulerTens(columns), rulerUnits(columns), strings.Repeat("-", columns))
	texts = append(texts, row("Producto de ejemplo", pesos(WidthTestSampleAmount), columns)...)
	texts = append(texts, strings.Repeat("-", columns), "")
	texts = append(texts, wrapWords("Si el último número, la línea de guiones o el importe se cortan o pasan al renglón siguiente, "+
		"esta impresora no imprime "+strconv.Itoa(columns)+" columnas: elige menos.", columns)...)
	texts = append(texts, alignText(WidthTestMarker, AlignCenter, columns))
	layout := &Layout{PaperWidth: paper, Columns: columns, TextSize: TextSizeNormal, TrailingNewline: true, Lines: []Line{}}
	for _, text := range texts {
		layout.Lines = append(layout.Lines, Line{Text: text, Size: SizeNormal})
	}
	return layout
}

// rulerTens marca las decenas sobre la regla («         1         2…»), sin espacios al final.
func rulerTens(columns int) string {
	var builder strings.Builder
	for position := 1; position <= columns; position++ {
		if position%10 == 0 {
			builder.WriteString(strconv.Itoa((position / 10) % 10))
			continue
		}
		builder.WriteByte(' ')
	}
	return strings.TrimRight(builder.String(), " ")
}

// rulerUnits es la regla numerada «123456789012…» del ancho completo.
func rulerUnits(columns int) string {
	var builder strings.Builder
	for position := 1; position <= columns; position++ {
		builder.WriteString(strconv.Itoa(position % 10))
	}
	return builder.String()
}
