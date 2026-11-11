package reports

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// Libro de Excel (.xlsx real, §18) con github.com/xuri/excelize/v2:
//   - hoja «Datos»: encabezado distinguible y congelado, una columna por cada columna visible del
//     reporte, anchos según el contenido y, al final, los totales calculados en Go (números, nunca
//     fórmulas);
//   - hoja «Filtros»: encabezado de identidad (logotipo del restaurante o, si no tiene, el de AD-EL
//     POS, junto al nombre del establecimiento; export_xlsx_identity.go) y la tabla de metadatos:
//     establecimiento, dirección, teléfono y correo (solo los que existan), reporte, periodo,
//     comparación, filtros aplicados, fecha usada, hora de inicio del día, orden, renglones, fecha de
//     generación, cifras resumidas, avisos y notas.
//
// Tipos: importes, cantidades y porcentajes como números con formato por columna (pesos «$» y
// dólares «US$» en columnas separadas, nunca sumados); un importe solo se escribe como número si el
// valor que guardará Excel es exactamente el decimal de Go (ida y vuelta comprobada, a lo más 15
// dígitos significativos); si no, se escribe como texto y la hoja «Filtros» lo indica. Fechas
// comerciales como fecha y los instantes en la hora del restaurante. Códigos, folios, teléfonos y
// textos siempre como texto (formato «@»), con sus ceros iniciales.
//
// Seguridad: ningún valor se escribe como fórmula. Un texto que empieza por «=», «+», «-», «@»,
// tabulador, retorno o salto de línea (o por sus formas de ancho completo) se escribe con un
// apóstrofo inicial visible, para que tampoco se interprete como fórmula si alguien lo copia o lo
// guarda como CSV. Sin hipervínculos, nombres definidos, vínculos externos ni macros; la única
// imagen es el logotipo del encabezado de «Filtros» (PNG o JPEG preparado en Go, sin hipervínculo).

const (
	xlsxDataSheet = "Datos"
	xlsxInfoSheet = "Filtros"
	// excelSignificantDigits es la precisión con la que Excel conserva un número.
	excelSignificantDigits = 15
	xlsxMinWidth           = 8.0
	xlsxMaxWidth           = 60.0
)

// excelText antepone un apóstrofo a un texto que una hoja de cálculo podría interpretar como
// fórmula.
func excelText(text string) string {
	if text == "" {
		return text
	}
	first, _ := utf8.DecodeRuneInString(text)
	switch first {
	case '=', '+', '-', '@', '\t', '\r', '\n', '＝', '＋', '－', '＠':
		return "'" + text
	}
	return text
}

// significantDigits cuenta los dígitos significativos de un decimal (sin ceros finales).
func significantDigits(value decimal.Decimal) int {
	digits := strings.TrimLeft(value.Coefficient().String(), "-")
	digits = strings.TrimRight(digits, "0")
	if digits == "" {
		return 1
	}
	return len(digits)
}

// excelNumber devuelve el número que se escribirá en el libro si representa value exactamente:
// excelize escribe el double con su representación decimal más corta, así que se comprueba que esa
// representación sea el mismo decimal y que Excel no lo recorte (15 dígitos significativos).
func excelNumber(value decimal.Decimal) (float64, bool) {
	if significantDigits(value) > excelSignificantDigits {
		return 0, false
	}
	number := value.InexactFloat64()
	back, err := decimal.NewFromString(strconv.FormatFloat(number, 'f', -1, 64))
	if err != nil || !back.Equal(value) {
		return 0, false
	}
	return number, true
}

// xlsxFormats son los formatos numéricos por tipo de columna.
var xlsxFormats = map[string]string{
	TypeInteger:  "#,##0",
	TypeMoneyMXN: `"$"#,##0.00;-"$"#,##0.00`,
	TypeMoneyUSD: `"US$"#,##0.00;-"US$"#,##0.00`,
	TypePercent:  "0.00%",
	TypeDate:     "dd/mm/yyyy",
	TypeDateTime: "dd/mm/yyyy hh:mm",
}

type xlsxStyles struct {
	header      int
	headerRight int
	title       int
	label       int
	section     int
	wrapText    int
	byType      map[string]int
	totals      map[string]int
	textNumber  int
	totalText   int
}

func newXLSXStyles(file *excelize.File) (*xlsxStyles, error) {
	styles := &xlsxStyles{byType: map[string]int{}, totals: map[string]int{}}
	border := func(position string, style int) []excelize.Border {
		return []excelize.Border{{Type: position, Color: "7A8794", Style: style}}
	}
	create := func(style *excelize.Style) (int, error) {
		return file.NewStyle(style)
	}
	var err error
	header := &excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "1C222A"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"DCE5EE"}},
		Border:    border("bottom", 2),
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "center"},
	}
	if styles.header, err = create(header); err != nil {
		return nil, err
	}
	header.Alignment = &excelize.Alignment{WrapText: true, Vertical: "center", Horizontal: "right"}
	if styles.headerRight, err = create(header); err != nil {
		return nil, err
	}
	if styles.label, err = create(&excelize.Style{Font: &excelize.Font{Bold: true}, Alignment: &excelize.Alignment{Vertical: "top"}}); err != nil {
		return nil, err
	}
	if styles.section, err = create(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 12},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"DCE5EE"}}}); err != nil {
		return nil, err
	}
	if styles.wrapText, err = create(&excelize.Style{NumFmt: 49, Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}}); err != nil {
		return nil, err
	}
	totalFill := excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EEF2F6"}}
	for _, columnType := range columnTypes {
		style := &excelize.Style{Alignment: &excelize.Alignment{Vertical: "top"}}
		switch columnType {
		case TypeText, TypeCode:
			// «@»: texto; conserva ceros iniciales aunque alguien edite la celda.
			style.NumFmt = 49
		case TypeQuantity:
			style.NumFmt = 0
		default:
			format := xlsxFormats[columnType]
			style.CustomNumFmt = &format
		}
		if styles.byType[columnType], err = create(style); err != nil {
			return nil, err
		}
		style.Font = &excelize.Font{Bold: true}
		style.Fill = totalFill
		style.Border = border("top", 2)
		if styles.totals[columnType], err = create(style); err != nil {
			return nil, err
		}
	}
	right := &excelize.Alignment{Horizontal: "right", Vertical: "top"}
	if styles.textNumber, err = create(&excelize.Style{NumFmt: 49, Alignment: right}); err != nil {
		return nil, err
	}
	if styles.totalText, err = create(&excelize.Style{NumFmt: 49, Alignment: right, Font: &excelize.Font{Bold: true},
		Fill: totalFill, Border: border("top", 2)}); err != nil {
		return nil, err
	}
	// El estilo del nombre en el encabezado de «Filtros» (9B) se crea al final para que los demás
	// conserven sus índices: la hoja «Datos» queda idéntica a la de la 9A.
	title := &excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 14, Color: "1C222A"},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	}
	if styles.title, err = create(title); err != nil {
		return nil, err
	}
	return styles, nil
}

// xlsxWriter convierte valores tipados en celdas.
type xlsxWriter struct {
	styles   *xlsxStyles
	location *time.Location
	// inexact cuenta los valores que no se podían representar exactamente como número de Excel y
	// se escribieron como texto.
	inexact int
}

// number escribe un decimal como número exacto o, si no es posible, como texto.
func (writer *xlsxWriter) number(value decimal.Decimal, style, textStyle int) excelize.Cell {
	if number, ok := excelNumber(value); ok {
		return excelize.Cell{StyleID: style, Value: number}
	}
	writer.inexact++
	return excelize.Cell{StyleID: textStyle, Value: value.String()}
}

func (writer *xlsxWriter) cell(columnType string, value any, total bool) excelize.Cell {
	style, textStyle := writer.styles.byType[columnType], writer.styles.textNumber
	if total {
		style, textStyle = writer.styles.totals[columnType], writer.styles.totalText
	}
	if value == nil {
		return excelize.Cell{StyleID: style}
	}
	if columnType == TypeText || columnType == TypeCode {
		return excelize.Cell{StyleID: style, Value: excelText(displayValue(columnType, value, writer.location))}
	}
	switch typed := value.(type) {
	case decimal.Decimal:
		switch columnType {
		case TypePercent:
			// Go entrega 12.50 para 12.5 %: Excel guarda la fracción exacta 0.125 con formato «0.00%».
			return writer.number(typed.Shift(-2), style, textStyle)
		case TypeMoneyMXN, TypeMoneyUSD, TypeQuantity, TypeInteger:
			return writer.number(typed, style, textStyle)
		}
	case int64:
		if typed > -1e15 && typed < 1e15 {
			return excelize.Cell{StyleID: style, Value: typed}
		}
		writer.inexact++
		return excelize.Cell{StyleID: textStyle, Value: strconv.FormatInt(typed, 10)}
	case time.Time:
		if columnType == TypeDate {
			// La fecha comercial ya es medianoche UTC: Excel muestra ese día.
			return excelize.Cell{StyleID: style, Value: typed.UTC()}
		}
		if columnType == TypeDateTime {
			// excelize convierte la hora de la zona del valor: se muestra la hora del restaurante.
			return excelize.Cell{StyleID: style, Value: typed.In(writer.location)}
		}
	}
	return excelize.Cell{StyleID: writer.styles.byType[TypeText], Value: excelText(displayValue(columnType, value, writer.location))}
}

// columnWidth estima el ancho (en caracteres) de una columna por su encabezado y su contenido.
func columnWidth(column Column, rows []Row, totals Row, location *time.Location) float64 {
	width := 0
	for _, word := range strings.Fields(column.Label) {
		width = max(width, utf8.RuneCountInString(word))
	}
	measure := func(value any) {
		if value != nil {
			width = max(width, utf8.RuneCountInString(displayValue(column.Type, value, location)))
		}
	}
	for _, row := range rows {
		measure(row[column.Key])
	}
	measure(totals[column.Key])
	return min(max(float64(width)+2, xlsxMinWidth), xlsxMaxWidth)
}

func renderXLSX(ctx context.Context, document *exportDocument) ([]byte, error) {
	result := document.Result
	file := excelize.NewFile()
	defer file.Close()
	if err := file.SetSheetName("Sheet1", xlsxDataSheet); err != nil {
		return nil, err
	}
	if _, err := file.NewSheet(xlsxInfoSheet); err != nil {
		return nil, err
	}
	styles, err := newXLSXStyles(file)
	if err != nil {
		return nil, err
	}
	writer := &xlsxWriter{styles: styles, location: document.Location}
	if err := writeDataSheet(ctx, file, writer, document); err != nil {
		return nil, err
	}
	if err := writeInfoSheet(file, writer, document); err != nil {
		return nil, err
	}
	generated := result.Meta.GeneratedAt.UTC().Format(time.RFC3339)
	if err := file.SetDocProps(&excelize.DocProperties{
		Title: result.Meta.Title, Subject: periodDescription(result.Meta.Period), Description: result.Report.Description,
		Creator: "AD-EL POS", LastModifiedBy: "AD-EL POS", Category: "Reporte", Language: "es-MX",
		Created: generated, Modified: generated,
	}); err != nil {
		return nil, err
	}
	if err := file.SetAppProps(&excelize.AppProperties{Application: "AD-EL POS", Company: document.Establishment}); err != nil {
		return nil, err
	}
	file.SetActiveSheet(0)
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeDataSheet(ctx context.Context, file *excelize.File, writer *xlsxWriter, document *exportDocument) error {
	result := document.Result
	stream, err := file.NewStreamWriter(xlsxDataSheet)
	if err != nil {
		return err
	}
	headerLines := 1
	for index, column := range result.Columns {
		width := columnWidth(column, result.AllRows, result.Totals, document.Location)
		if err := stream.SetColWidth(index+1, index+1, width); err != nil {
			return err
		}
		lines := (utf8.RuneCountInString(column.Label) + int(width) - 3) / int(width-2)
		headerLines = max(headerLines, lines)
	}
	if err := stream.SetPanes(&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
		Selection: []excelize.Selection{{SQRef: "A2", ActiveCell: "A2", Pane: "bottomLeft"}}}); err != nil {
		return err
	}
	header := make([]any, len(result.Columns))
	for index, column := range result.Columns {
		style := writer.styles.header
		if numericColumn(column.Type) {
			style = writer.styles.headerRight
		}
		header[index] = excelize.Cell{StyleID: style, Value: excelText(column.Label)}
	}
	if err := stream.SetRow("A1", header, excelize.RowOpts{Height: 15.5 * float64(headerLines)}); err != nil {
		return err
	}
	for index, row := range result.AllRows {
		if index%1000 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		cells := make([]any, len(result.Columns))
		for position, column := range result.Columns {
			cells[position] = writer.cell(column.Type, row[column.Key], false)
		}
		cell, _ := excelize.CoordinatesToCellName(1, index+2)
		if err := stream.SetRow(cell, cells); err != nil {
			return err
		}
	}
	if len(result.AllRows) > 0 {
		cells := make([]any, len(result.Columns))
		labelled := false
		for position, column := range result.Columns {
			value, found := result.Totals[column.Key]
			if (!found || value == nil) && !labelled && !numericColumn(column.Type) {
				cells[position] = excelize.Cell{StyleID: writer.styles.totals[TypeText], Value: "Total"}
				labelled = true
				continue
			}
			cells[position] = writer.cell(column.Type, value, true)
		}
		cell, _ := excelize.CoordinatesToCellName(1, len(result.AllRows)+2)
		if err := stream.SetRow(cell, cells); err != nil {
			return err
		}
	}
	return stream.Flush()
}

// infoRow es un renglón «Concepto | Valor» de la hoja de filtros.
type infoRow struct {
	label   string
	value   excelize.Cell
	section bool
}

func writeInfoSheet(file *excelize.File, writer *xlsxWriter, document *exportDocument) error {
	result := document.Result
	meta := result.Meta
	styles := writer.styles
	text := func(value string) excelize.Cell {
		return excelize.Cell{StyleID: styles.wrapText, Value: excelText(value)}
	}
	rows := []infoRow{{label: "Establecimiento", value: text(document.Establishment)}}
	rows = append(rows, contactRows(document, text)...)
	rows = append(rows,
		infoRow{label: "Reporte", value: text(meta.Title)},
		infoRow{label: "Descripción", value: text(result.Report.Description)},
		infoRow{label: "Periodo", value: text(periodDescription(meta.Period))})
	if period := meta.Period; period != nil {
		rows = append(rows,
			infoRow{label: "Primer día", value: writer.cell(TypeDate, period.First(), false)},
			infoRow{label: "Último día", value: writer.cell(TypeDate, period.Last(), false)})
	}
	if meta.Comparison != nil {
		rows = append(rows, infoRow{label: "Comparación", value: text(meta.Comparison.Label)})
	}
	rows = append(rows,
		infoRow{label: "Fecha usada", value: text(meta.DateBasis)},
		infoRow{label: "Hora de inicio del día", value: text(meta.DayStart)},
		infoRow{label: "Zona horaria", value: text(meta.Timezone)},
		infoRow{label: "Orden", value: text(sortDescription(result))},
		infoRow{label: "Renglones", value: writer.cell(TypeInteger, int64(len(result.AllRows)), false)},
		infoRow{label: "Generado", value: writer.cell(TypeDateTime, meta.GeneratedAt, false)},
		infoRow{label: "Filtros aplicados", section: true})
	if len(meta.Filters) == 0 {
		rows = append(rows, infoRow{label: "Sin filtros"})
	}
	for _, filter := range meta.Filters {
		rows = append(rows, infoRow{label: filter.Label, value: text(filter.Display)})
	}
	if len(result.Summary) > 0 {
		rows = append(rows, infoRow{label: "Resumen", section: true})
		for _, figure := range result.Summary {
			rows = append(rows, infoRow{label: figure.Label, value: writer.cell(figure.Type, figure.Value, false)})
		}
	}
	if len(meta.Warnings) > 0 {
		rows = append(rows, infoRow{label: "Avisos", section: true})
		for _, warning := range meta.Warnings {
			rows = append(rows, infoRow{label: "Aviso", value: text(warning.Message)})
		}
	}
	if len(meta.Notes) > 0 {
		rows = append(rows, infoRow{label: "Notas", section: true})
		for _, note := range meta.Notes {
			rows = append(rows, infoRow{label: "Nota", value: text(note)})
		}
	}
	if writer.inexact > 0 {
		rows = append(rows, infoRow{label: "Precisión", value: text(strconv.Itoa(writer.inexact) +
			" valores con más de 15 dígitos significativos se escribieron como texto para conservarlos exactos.")})
	}
	// El logotipo se agrega antes de abrir el flujo de la hoja: el flujo conserva el dibujo del
	// encabezado al escribir la hoja (excelize no admite imágenes después de iniciarlo).
	if err := addInfoLogo(file, document.Logo); err != nil {
		return err
	}
	stream, err := file.NewStreamWriter(xlsxInfoSheet)
	if err != nil {
		return err
	}
	if err := stream.SetColWidth(1, 1, xlsxInfoLabelWidth); err != nil {
		return err
	}
	if err := stream.SetColWidth(2, 2, 90); err != nil {
		return err
	}
	if err := writeInfoIdentity(stream, styles, document); err != nil {
		return err
	}
	headerCell, _ := excelize.CoordinatesToCellName(1, xlsxInfoTableRow)
	if err := stream.SetRow(headerCell, []any{
		excelize.Cell{StyleID: styles.header, Value: "Concepto"}, excelize.Cell{StyleID: styles.header, Value: "Valor"},
	}); err != nil {
		return err
	}
	for index, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, xlsxInfoTableRow+1+index)
		values := []any{excelize.Cell{StyleID: styles.label, Value: excelText(row.label)}, row.value}
		if row.section {
			values = []any{excelize.Cell{StyleID: styles.section, Value: row.label}, excelize.Cell{StyleID: styles.section}}
		}
		if err := stream.SetRow(cell, values); err != nil {
			return err
		}
	}
	return stream.Flush()
}
