package reports

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/internal/moneytext"
)

// Pruebas de los generadores de exportación (fase C1) sobre un Result armado por el mismo motor que
// la pantalla (assemble): PDF válido con texto extraíble, encabezados repetidos, «Página n de m»,
// totales y monedas separadas; XLSX válido con tipos, fechas, importes exactos, códigos como texto,
// fórmulas neutralizadas, totales, hoja de filtros y encabezado congelado; y columnas protegidas
// ausentes del archivo para quien no tiene su permiso.

const accentedText = "Año señal Ñandú ¿Cómo? ¡Sí! «crème brûlée» — Pingüino"

var formulaTexts = []string{`=HYPERLINK("http://example.invalid","x")`, "+1+1", "-2+3", "@SUM(A1)", "\tTabulado",
	"\rRetorno", "\nSalto", "＝全角", "＋1", "－1", "＠SUM(A1)"}

func exportTestDefinition() *Definition {
	definition := &Definition{
		ID: "test.export", Group: "sales", Title: "Ventas de prueba: año, señal y Ñandú",
		Description: "Descripción ¿con acentos? ¡Sí!", DateBasis: "Fecha de cierre de la cuenta, con la hora de inicio del día.",
		Columns: []Column{
			textColumn("name", "Producto"),
			codeColumn("code", "Código"),
			countColumn("quantity", "Cantidad"),
			{Key: "weight", Label: "Peso", Type: TypeQuantity, Total: TotalSum, Sortable: true},
			moneyColumn("net", "Venta neta"),
			dollarColumn("usd", "Dólares recibidos"),
			percentColumn("share", "% de la venta"),
			dateColumn("day", "Día comercial"),
			dateTimeColumn("at", "Cierre"),
			requiring(moneyColumn("cost", "Costo del consumo"), PermissionCostsRead),
			requiring(moneyColumn("expected", "Efectivo esperado"), PermissionCashExpected),
			requiring(moneyColumn("difference", "Diferencia de caja"), PermissionCashExpected),
		},
		DefaultSort: Sort{Key: "quantity"},
		Totals:      shareTotals,
	}
	for index := range definition.Columns {
		if definition.Columns[index].Total == "" {
			definition.Columns[index].Total = TotalNone
		}
		if definition.Columns[index].Visibility == "" {
			definition.Columns[index].Visibility = VisibilityDefault
		}
	}
	return definition
}

// exportTestRows genera count renglones; los primeros llevan textos con acentos y textos que
// parecen fórmulas.
func exportTestRows(count int) []Row {
	rows := make([]Row, 0, count)
	day := time.Date(2026, time.November, 10, 0, 0, 0, 0, time.UTC)
	// 01:30 del 11 de noviembre en Monterrey (UTC-6) = 07:30 UTC.
	closing := time.Date(2026, time.November, 11, 7, 30, 0, 0, time.UTC)
	for index := 0; index < count; index++ {
		name := "Producto " + strconv.Itoa(index+1)
		switch {
		case index == 0:
			name = accentedText
		case index <= len(formulaTexts):
			name = formulaTexts[index-1]
		case index == len(formulaTexts)+1:
			name = strings.Repeat("Hamburguesa doble con queso y tocino ", 6)
		}
		rows = append(rows, Row{
			"name": name, "code": []string{"007", "000123", "A-01"}[index%3], "quantity": int64(index + 1),
			"weight": decimal.RequireFromString("1.250"), "net": decimal.RequireFromString("1234.56"),
			"usd": decimal.RequireFromString("20.05"), "day": day, "at": closing,
			"cost": decimal.RequireFromString("400.10"), "expected": decimal.RequireFromString("9876.54"),
			"difference": decimal.RequireFromString("-3.21"),
		})
	}
	return rows
}

func exportLocationForTest(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("America/Monterrey")
	if err != nil {
		t.Fatal(err)
	}
	return location
}

// exportTestDocument arma el documento con el mismo motor de la pantalla.
func exportTestDocument(t *testing.T, definition *Definition, rows []Row, permissions ...string) *exportDocument {
	t.Helper()
	location := exportLocationForTest(t)
	config := businessday.Config{Timezone: "America/Monterrey", Location: location, Hour: 5}
	now := time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC)
	period, err := businessday.Resolve(businessday.Spec{Kind: businessday.KindRange, From: "2026-11-01", Until: "2026-11-30"}, now, config)
	if err != nil {
		t.Fatal(err)
	}
	actor := Actor{Permissions: append([]string{PermissionRead}, permissions...)}
	query := &Query{Params: &Params{Period: &period, Page: 1, PageSize: DefaultPageSize, Sort: definition.DefaultSort},
		Actor: actor, Config: config, Now: now}
	dataset := &Dataset{Rows: rows,
		Summary: []Figure{
			{Key: "net", Label: "Venta neta del periodo", Type: TypeMoneyMXN, Value: decimal.RequireFromString("1234.56")},
			{Key: "expected", Label: "Esperado total", Type: TypeMoneyMXN, Value: decimal.RequireFromString("9876.54"),
				Permissions: []string{PermissionCashExpected}},
		},
		Warnings: []Warning{{Code: "SHIFT_CLOSING", Message: "Turnos en arqueo excluidos: 1."}},
		Notes:    []string{"Las propinas no son venta."},
	}
	result, err := assemble(context.Background(), nil, query, definition, dataset)
	if err != nil {
		t.Fatal(err)
	}
	result.Meta.Filters = []AppliedFilter{{Key: "product_id", Label: "Producto", Value: "0f8e3c2a-0000-4000-8000-000000000001",
		Display: "Hamburguesa «doble»"}}
	return &exportDocument{Establishment: "Restaurante El Peñón", Result: result, Location: location, Paper: PaperLetter}
}

// pdfPages extrae el texto de cada página: descomprime los flujos de contenido y decodifica las
// cadenas UTF-16BE que fpdf escribe para las fuentes UTF-8 (CID = código Unicode).
func pdfPages(t *testing.T, content []byte) []string {
	t.Helper()
	pages := []string{}
	// Cada flujo se lee con su /Length exacto (los datos comprimidos pueden terminar en cualquier byte).
	streamPattern := regexp.MustCompile(`/Length (\d+)[^>]*>>\s*stream\r?\n`)
	for _, match := range streamPattern.FindAllSubmatchIndex(content, -1) {
		length, err := strconv.Atoi(string(content[match[2]:match[3]]))
		if err != nil || match[1]+length > len(content) {
			continue
		}
		reader, err := zlib.NewReader(bytes.NewReader(content[match[1] : match[1]+length]))
		if err != nil {
			continue
		}
		data, err := io.ReadAll(reader)
		if err != nil || !bytes.Contains(data, []byte(" Td (")) || bytes.Contains(data, []byte("begincmap")) {
			continue
		}
		pages = append(pages, strings.Join(pdfStrings(data), "\n"))
	}
	return pages
}

// pdfStrings lee las cadenas literales «(…)Tj» de un flujo de contenido.
func pdfStrings(data []byte) []string {
	texts := []string{}
	for index := 0; index < len(data); index++ {
		if data[index] != '(' || !bytes.HasSuffix(data[:index], []byte("Td ")) {
			continue
		}
		var raw []byte
		position := index + 1
		for ; position < len(data) && data[position] != ')'; position++ {
			character := data[position]
			if character == '\\' && position+1 < len(data) {
				position++
				switch escaped := data[position]; escaped {
				case 'n':
					raw = append(raw, '\n')
				case 'r':
					raw = append(raw, '\r')
				case 't':
					raw = append(raw, '\t')
				case 'b':
					raw = append(raw, '\b')
				case 'f':
					raw = append(raw, '\f')
				default:
					if escaped >= '0' && escaped <= '7' {
						end := position
						for end < len(data) && end < position+3 && data[end] >= '0' && data[end] <= '7' {
							end++
						}
						value, _ := strconv.ParseUint(string(data[position:end]), 8, 8)
						raw = append(raw, byte(value))
						position = end - 1
					} else {
						raw = append(raw, escaped)
					}
				}
				continue
			}
			raw = append(raw, character)
		}
		units := make([]uint16, 0, len(raw)/2)
		for offset := 0; offset+1 < len(raw); offset += 2 {
			units = append(units, uint16(raw[offset])<<8|uint16(raw[offset+1]))
		}
		texts = append(texts, string(utf16.Decode(units)))
		index = position
	}
	return texts
}

func TestPDFExportContent(t *testing.T) {
	document := exportTestDocument(t, exportTestDefinition(), exportTestRows(40), PermissionCostsRead, PermissionCashExpected)
	content, orientation, err := renderPDF(context.Background(), document)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) {
		t.Fatal("El archivo no empieza con la firma %PDF-")
	}
	if orientation != "landscape" {
		t.Fatalf("Un reporte de doce columnas debe ir en horizontal: %s", orientation)
	}
	pages := pdfPages(t, content)
	declared := len(regexp.MustCompile(`/Type /Page[^s]`).FindAll(content, -1))
	if len(pages) != declared || declared < 2 {
		t.Fatalf("Páginas: %d flujos de texto y %d páginas declaradas", len(pages), declared)
	}
	all := strings.Join(pages, "\n")
	for _, expected := range []string{"Restaurante El Peñón", "Ventas de prueba: año, señal y Ñandú", "Del 1 al 30 de noviembre de 2026",
		"Producto: Hamburguesa «doble»", "Fecha de cierre de la cuenta", "05:00", "Ñandú", "¿Cómo?", "¡Sí!", "«crème", "brûlée»",
		"Pingüino", "Generado", "12/11/2026 12:00",
		"Venta neta del periodo", "Esperado total", "Aviso: Turnos en arqueo excluidos: 1.", "Las propinas no son venta.",
		"US$20.05", "$1,234.56", "1.25", "2.50 %", "10/11/2026", "11/11/2026 01:30", "007", "000123", "Costo"} {
		if !strings.Contains(all, expected) {
			t.Fatalf("El PDF no contiene «%s»", expected)
		}
	}
	// Encabezados de columna repetidos y «Página n de m» en cada página.
	for number, page := range pages {
		for _, label := range []string{"Producto", "Venta neta", "Dólares", "Cierre"} {
			if !strings.Contains(page, label) {
				t.Fatalf("La página %d no repite el encabezado «%s»", number+1, label)
			}
		}
		footer := "Página " + strconv.Itoa(number+1) + " de " + strconv.Itoa(len(pages))
		if !strings.Contains(page, footer) {
			t.Fatalf("La página %d no dice «%s»", number+1, footer)
		}
	}
	// Totales iguales a los del resultado de la pantalla, cada moneda en su columna.
	last := pages[len(pages)-1]
	totals := document.Result.Totals
	for _, expected := range []string{
		"Total", moneytext.FormatDecimal("MXN", totals["net"].(decimal.Decimal)), moneytext.FormatDecimal("USD", totals["usd"].(decimal.Decimal)),
		"820", "50", "100.00 %",
	} {
		if !strings.Contains(last, expected) {
			t.Fatalf("La última página no contiene el total «%s»: %s", expected, last)
		}
	}
	if moneytext.FormatDecimal("MXN", totals["net"].(decimal.Decimal)) != "$49,382.40" ||
		moneytext.FormatDecimal("USD", totals["usd"].(decimal.Decimal)) != "US$802.00" {
		t.Fatalf("Totales del motor: %+v", totals)
	}
}

// Sin inventory.costs.read ni cash.expected.read el PDF no contiene esas columnas, sus valores, sus
// totales ni sus cifras resumidas.
func TestPDFExportOmitsProtectedColumns(t *testing.T) {
	document := exportTestDocument(t, exportTestDefinition(), exportTestRows(5))
	content, _, err := renderPDF(context.Background(), document)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(pdfPages(t, content), "\n")
	for _, forbidden := range []string{"Costo", "$400.10", "esperado", "Esperado", "$9,876.54", "Diferencia", "-$3.21"} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("El PDF de un usuario sin permiso contiene «%s»", forbidden)
		}
	}
	if !strings.Contains(all, "Venta neta") {
		t.Fatal("El PDF debe conservar las columnas autorizadas")
	}
}

// Un reporte angosto va en vertical; A4 si se pide; un reporte vacío lo indica.
func TestPDFExportOrientationPaperAndEmpty(t *testing.T) {
	definition := &Definition{ID: "test.narrow", Title: "Cobros por método", DateBasis: "Fecha del cobro.",
		Columns: []Column{textColumn("method", "Método"), moneyColumn("net", "Cobrado neto")}}
	for index := range definition.Columns {
		definition.Columns[index].Visibility = VisibilityDefault
	}
	document := exportTestDocument(t, definition, []Row{{"method": "Efectivo", "net": decimal.RequireFromString("10")}})
	content, orientation, err := renderPDF(context.Background(), document)
	if err != nil || orientation != "portrait" {
		t.Fatalf("Un reporte de dos columnas va en vertical: %s %v", orientation, err)
	}
	if !bytes.Contains(content, []byte("/MediaBox [0 0 612.00 792.00]")) {
		t.Fatal("Carta vertical: 612 × 792 puntos")
	}
	document.Paper = PaperA4
	content, _, err = renderPDF(context.Background(), document)
	if err != nil || !bytes.Contains(content, []byte("/MediaBox [0 0 595.28 841.89]")) {
		t.Fatalf("A4 vertical: %v", err)
	}
	empty := exportTestDocument(t, definition, []Row{})
	content, _, err = renderPDF(context.Background(), empty)
	if err != nil || !strings.Contains(strings.Join(pdfPages(t, content), "\n"), "No hay datos para el periodo") {
		t.Fatalf("Un reporte vacío debe decirlo: %v", err)
	}
}

// Un archivo grande: miles de renglones en muchas páginas, todas con encabezado y número.
func TestPDFExportLargeFile(t *testing.T) {
	document := exportTestDocument(t, exportTestDefinition(), exportTestRows(3000), PermissionCostsRead)
	started := time.Now()
	content, _, err := renderPDF(context.Background(), document)
	if err != nil {
		t.Fatal(err)
	}
	pages := pdfPages(t, content)
	if len(pages) < 40 {
		t.Fatalf("3 000 renglones deben ocupar muchas páginas: %d", len(pages))
	}
	for number, page := range pages {
		if !strings.Contains(page, "Página "+strconv.Itoa(number+1)+" de "+strconv.Itoa(len(pages))) || !strings.Contains(page, "Producto") {
			t.Fatalf("Página %d sin encabezado o número", number+1)
		}
	}
	if !strings.Contains(pages[len(pages)-1], "Producto 3000") {
		t.Fatal("Falta el último renglón")
	}
	t.Logf("PDF de 3 000 renglones: %d páginas, %d bytes, %s", len(pages), len(content), time.Since(started).Round(time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := renderPDF(ctx, document); err == nil {
		t.Fatal("Una exportación cancelada no debe terminar")
	}
}

func openWorkbook(t *testing.T, content []byte) *excelize.File {
	t.Helper()
	if !bytes.HasPrefix(content, []byte("PK\x03\x04")) {
		t.Fatal("El archivo no empieza con la firma PK\\x03\\x04")
	}
	file, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func raw(t *testing.T, file *excelize.File, sheet, cell string) string {
	t.Helper()
	value, err := file.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func shown(t *testing.T, file *excelize.File, sheet, cell string) string {
	t.Helper()
	value, err := file.GetCellValue(sheet, cell)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func cellType(t *testing.T, file *excelize.File, sheet, cell string) excelize.CellType {
	t.Helper()
	kind, err := file.GetCellType(sheet, cell)
	if err != nil {
		t.Fatal(err)
	}
	return kind
}

// isNumber indica si la celda es numérica (excelize escribe los números sin atributo t, como Excel).
func isNumber(t *testing.T, file *excelize.File, sheet, cell string) bool {
	t.Helper()
	kind := cellType(t, file, sheet, cell)
	if kind != excelize.CellTypeUnset && kind != excelize.CellTypeNumber {
		return false
	}
	_, err := strconv.ParseFloat(raw(t, file, sheet, cell), 64)
	return err == nil
}

func numberFormat(t *testing.T, file *excelize.File, sheet, cell string) (int, string) {
	t.Helper()
	index, err := file.GetCellStyle(sheet, cell)
	if err != nil {
		t.Fatal(err)
	}
	style, err := file.GetStyle(index)
	if err != nil {
		t.Fatal(err)
	}
	custom := ""
	if style.CustomNumFmt != nil {
		custom = *style.CustomNumFmt
	}
	return style.NumFmt, custom
}

// assertSafeWorkbook descomprime el libro y comprueba que no hay fórmulas, hipervínculos, nombres
// definidos, vínculos externos ni macros.
func assertSafeWorkbook(t *testing.T, content []byte) {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range archive.File {
		name := strings.ToLower(entry.Name)
		if strings.Contains(name, "externallink") || strings.Contains(name, "vbaproject") || strings.HasSuffix(name, ".bin") ||
			strings.Contains(name, "media/") || strings.Contains(name, "drawings/") {
			t.Fatalf("El libro contiene una parte no permitida: %s", entry.Name)
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if regexp.MustCompile(`<f[ >]|<f/>`).MatchString(text) {
			t.Fatalf("%s contiene una fórmula", entry.Name)
		}
		for _, forbidden := range []string{"hyperlink", `TargetMode="External"`, "definedName", "macroEnabled", "vbaProject"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contiene «%s»", entry.Name, forbidden)
			}
		}
	}
}

func TestXLSXExportContent(t *testing.T) {
	document := exportTestDocument(t, exportTestDefinition(), exportTestRows(40), PermissionCostsRead, PermissionCashExpected)
	content, err := renderXLSX(context.Background(), document)
	if err != nil {
		t.Fatal(err)
	}
	assertSafeWorkbook(t, content)
	file := openWorkbook(t, content)
	if sheets := file.GetSheetList(); !slices.Equal(sheets, []string{xlsxDataSheet, xlsxInfoSheet}) {
		t.Fatalf("Hojas: %v", sheets)
	}
	header, err := file.GetRows(xlsxDataSheet)
	if err != nil {
		t.Fatal(err)
	}
	labels := []string{"Producto", "Código", "Cantidad", "Peso", "Venta neta", "Dólares recibidos", "% de la venta", "Día comercial",
		"Cierre", "Costo del consumo", "Efectivo esperado", "Diferencia de caja"}
	if !slices.Equal(header[0], labels) {
		t.Fatalf("Encabezados: %v", header[0])
	}
	panes, err := file.GetPanes(xlsxDataSheet)
	if err != nil || !panes.Freeze || panes.YSplit != 1 || panes.TopLeftCell != "A2" {
		t.Fatalf("El encabezado debe quedar congelado: %+v %v", panes, err)
	}
	if index, _ := file.GetCellStyle(xlsxDataSheet, "A1"); index == 0 {
		t.Fatal("El encabezado debe tener un estilo distinguible")
	}
	if width, _ := file.GetColWidth(xlsxDataSheet, "A"); width < 20 || width > xlsxMaxWidth {
		t.Fatalf("Ancho de la columna de texto: %v", width)
	}
	// Renglón 2: acentos; los renglones 3 a 13 tienen textos que parecen fórmulas.
	if shown(t, file, xlsxDataSheet, "A2") != accentedText {
		t.Fatal("El texto con acentos cambió")
	}
	for index, text := range formulaTexts {
		cell := "A" + strconv.Itoa(index+3)
		if got := raw(t, file, xlsxDataSheet, cell); got != "'"+text {
			t.Fatalf("%s: el texto que parece fórmula debe llevar apóstrofo: %q", cell, got)
		}
		if formula, _ := file.GetCellFormula(xlsxDataSheet, cell); formula != "" {
			t.Fatalf("%s quedó como fórmula: %s", cell, formula)
		}
	}
	// Códigos con ceros iniciales como texto «@»; importes, cantidades y porcentajes como números exactos.
	if raw(t, file, xlsxDataSheet, "B2") != "007" || raw(t, file, xlsxDataSheet, "B3") != "000123" ||
		cellType(t, file, xlsxDataSheet, "B2") != excelize.CellTypeInlineString {
		t.Fatal("Los códigos deben conservar sus ceros iniciales como texto")
	}
	if format, _ := numberFormat(t, file, xlsxDataSheet, "B2"); format != 49 {
		t.Fatalf("Formato de código: %d", format)
	}
	checks := map[string]string{"C2": "1", "D2": "1.25", "E2": "1234.56", "F2": "20.05"}
	for cell, expected := range checks {
		if !isNumber(t, file, xlsxDataSheet, cell) || raw(t, file, xlsxDataSheet, cell) != expected {
			t.Fatalf("%s debe ser el número %s: %q", cell, expected, raw(t, file, xlsxDataSheet, cell))
		}
	}
	if _, format := numberFormat(t, file, xlsxDataSheet, "E2"); format != `"$"#,##0.00;-"$"#,##0.00` {
		t.Fatalf("Formato de pesos: %s", format)
	}
	if _, format := numberFormat(t, file, xlsxDataSheet, "F2"); !strings.Contains(format, "US$") {
		t.Fatalf("Formato de dólares: %s", format)
	}
	if shown(t, file, xlsxDataSheet, "E2") != "$1,234.56" || shown(t, file, xlsxDataSheet, "F2") != "US$20.05" {
		t.Fatalf("Importes mostrados: %s %s", shown(t, file, xlsxDataSheet, "E2"), shown(t, file, xlsxDataSheet, "F2"))
	}
	// 2.5 % de participación (1234.56 de 49 382.40) se guarda como la fracción exacta 0.025.
	if raw(t, file, xlsxDataSheet, "G2") != "0.025" {
		t.Fatalf("Porcentaje: %s", raw(t, file, xlsxDataSheet, "G2"))
	}
	// Fecha comercial y hora del restaurante (07:30 UTC = 01:30 en Monterrey).
	if shown(t, file, xlsxDataSheet, "H2") != "10/11/2026" || shown(t, file, xlsxDataSheet, "I2") != "11/11/2026 01:30" {
		t.Fatalf("Fechas: %s %s", shown(t, file, xlsxDataSheet, "H2"), shown(t, file, xlsxDataSheet, "I2"))
	}
	if !isNumber(t, file, xlsxDataSheet, "H2") || !isNumber(t, file, xlsxDataSheet, "I2") {
		t.Fatalf("Las fechas deben ser números de serie de Excel")
	}
	// Totales de Go como números, sin fórmulas, en el renglón siguiente al último.
	totals := document.Result.Totals
	if raw(t, file, xlsxDataSheet, "A42") != "Total" || raw(t, file, xlsxDataSheet, "C42") != "820" ||
		raw(t, file, xlsxDataSheet, "E42") != totals["net"].(decimal.Decimal).String() ||
		raw(t, file, xlsxDataSheet, "F42") != totals["usd"].(decimal.Decimal).String() || raw(t, file, xlsxDataSheet, "G42") != "1" {
		t.Fatalf("Totales: %s %s %s %s", raw(t, file, xlsxDataSheet, "C42"), raw(t, file, xlsxDataSheet, "E42"),
			raw(t, file, xlsxDataSheet, "F42"), raw(t, file, xlsxDataSheet, "G42"))
	}
	// Hoja de filtros y metadatos.
	info, err := file.GetRows(xlsxInfoSheet)
	if err != nil {
		t.Fatal(err)
	}
	flat := map[string]string{}
	for _, row := range info {
		if len(row) > 1 {
			flat[row[0]] = row[1]
		}
	}
	expected := map[string]string{
		"Establecimiento": "Restaurante El Peñón", "Reporte": "Ventas de prueba: año, señal y Ñandú",
		"Periodo": "Del 1 al 30 de noviembre de 2026", "Primer día": "01/11/2026", "Último día": "30/11/2026",
		"Hora de inicio del día": "05:00", "Zona horaria": "America/Monterrey", "Renglones": "40",
		"Generado": "12/11/2026 12:00", "Producto": "Hamburguesa «doble»", "Venta neta del periodo": "$1,234.56",
		"Esperado total": "$9,876.54", "Aviso": "Turnos en arqueo excluidos: 1.", "Nota": "Las propinas no son venta.",
	}
	for label, value := range expected {
		if flat[label] != value {
			t.Fatalf("Hoja de filtros, %s: %q (esperado %q)", label, flat[label], value)
		}
	}
	properties, err := file.GetDocProps()
	if err != nil || properties.Creator != "AD-EL POS" || properties.Title != document.Result.Meta.Title {
		t.Fatalf("Propiedades del libro: %+v %v", properties, err)
	}
}

// Sin los permisos de costos y de esperado de caja, el libro no contiene esas columnas, sus valores
// ni sus cifras (se revisa el contenido descomprimido completo).
func TestXLSXExportOmitsProtectedColumns(t *testing.T) {
	document := exportTestDocument(t, exportTestDefinition(), exportTestRows(5))
	content, err := renderXLSX(context.Background(), document)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range archive.File {
		reader, _ := entry.Open()
		data, _ := io.ReadAll(reader)
		_ = reader.Close()
		for _, forbidden := range []string{"Costo", "400.1", "Esperado", "esperado", "9876.54", "Diferencia", "-3.21"} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("%s contiene «%s» sin permiso", entry.Name, forbidden)
			}
		}
	}
	file := openWorkbook(t, content)
	header, _ := file.GetRows(xlsxDataSheet)
	if len(header[0]) != 9 {
		t.Fatalf("Columnas autorizadas: %v", header[0])
	}
}

func TestExcelTextAndNumbers(t *testing.T) {
	for _, text := range formulaTexts {
		if escaped := excelText(text); escaped != "'"+text {
			t.Fatalf("%q debe llevar apóstrofo: %q", text, escaped)
		}
	}
	for _, text := range []string{"", "Hamburguesa", "1+1", "Total = 3", "'ya citado", " =espacio"} {
		if excelText(text) != text {
			t.Fatalf("%q no debe cambiar", text)
		}
	}
	for _, text := range []string{"0", "0.1", "0.01", "1234.56", "-3.21", "99999999999.99", "0.000001", "123456789012345",
		"1.25", "100000000000000000000"} {
		number, ok := excelNumber(decimal.RequireFromString(text))
		if !ok {
			t.Fatalf("%s debe escribirse como número exacto", text)
		}
		if back := strconv.FormatFloat(number, 'f', -1, 64); !decimal.RequireFromString(back).Equal(decimal.RequireFromString(text)) {
			t.Fatalf("Ida y vuelta de %s: %s", text, back)
		}
	}
	for _, text := range []string{"1234567890123456", "0.1234567890123456", "12345678901234.56"} {
		if _, ok := excelNumber(decimal.RequireFromString(text)); ok {
			t.Fatalf("%s excede la precisión de Excel y debe ir como texto", text)
		}
	}
	// Un importe que Excel no puede guardar exacto se escribe como texto y se avisa en la hoja de filtros.
	definition := &Definition{ID: "test.precision", Title: "Precisión", DateBasis: "Prueba.",
		Columns: []Column{textColumn("name", "Nombre"), moneyColumn("net", "Importe")}}
	for index := range definition.Columns {
		definition.Columns[index].Visibility = VisibilityDefault
	}
	document := exportTestDocument(t, definition, []Row{{"name": "Grande", "net": decimal.RequireFromString("12345678901234.56")}})
	content, err := renderXLSX(context.Background(), document)
	if err != nil {
		t.Fatal(err)
	}
	file := openWorkbook(t, content)
	if isNumber(t, file, xlsxDataSheet, "B2") && cellType(t, file, xlsxDataSheet, "B2") != excelize.CellTypeInlineString ||
		raw(t, file, xlsxDataSheet, "B2") != "12345678901234.56" {
		t.Fatalf("El importe largo debe conservarse como texto: %s", raw(t, file, xlsxDataSheet, "B2"))
	}
	info, _ := file.GetRows(xlsxInfoSheet)
	found := false
	for _, row := range info {
		if len(row) > 1 && row[0] == "Precisión" && strings.Contains(row[1], "15 dígitos") {
			found = true
		}
	}
	if !found {
		t.Fatal("La hoja de filtros debe avisar de los valores escritos como texto")
	}
}

func TestExportFileNamesAndOptions(t *testing.T) {
	location := exportLocationForTest(t)
	period := &businessday.Period{FirstDay: "2026-11-01", LastDay: "2026-11-30"}
	result := &Result{Report: ReportInfo{Title: "Ventas por día"}, Meta: Meta{Period: period}}
	if name := exportFileName(result, FormatPDF, location); name != "ventas-por-dia_2026-11-01_2026-11-30.pdf" {
		t.Fatalf("Nombre: %s", name)
	}
	result.Meta.Period = &businessday.Period{FirstDay: "2026-11-10", LastDay: "2026-11-10"}
	result.Report.Title = "Descuentos y cortesías / Año ñ «x»"
	if name := exportFileName(result, FormatXLSX, location); name != "descuentos-y-cortesias-ano-n-x_2026-11-10.xlsx" {
		t.Fatalf("Nombre: %s", name)
	}
	result.Meta.Period = nil
	result.Meta.GeneratedAt = time.Date(2026, time.November, 11, 3, 0, 0, 0, time.UTC)
	result.Report.Title = strings.Repeat("Reporte muy largo ", 10)
	name := exportFileName(result, FormatPDF, location)
	if !strings.HasSuffix(name, "_2026-11-10.pdf") || len(name) > 100 || strings.ContainsAny(name, " /\\") {
		t.Fatalf("Nombre de un reporte sin periodo: %s", name)
	}
	cases := []struct {
		query string
		code  string
	}{
		{"", "VALIDATION_ERROR"},
		{"format=csv", "VALIDATION_ERROR"},
		{"format=pdf&format=xlsx", "VALIDATION_ERROR"},
		{"format=xlsx&paper=a4", "VALIDATION_ERROR"},
		{"format=pdf&paper=legal", "VALIDATION_ERROR"},
		{"format=pdf&page=2", "VALIDATION_ERROR"},
		{"format=xlsx&page_size=10", "VALIDATION_ERROR"},
	}
	for _, testCase := range cases {
		values, _ := url.ParseQuery(testCase.query)
		if _, _, err := parseExportOptions(values); !isCode(err, testCase.code) {
			t.Fatalf("%q: %v", testCase.query, err)
		}
	}
	values, _ := url.ParseQuery("format=pdf&period=month&month=2026-11&sort=net")
	options, rest, err := parseExportOptions(values)
	if err != nil || options.Paper != PaperLetter || rest.Has("format") || rest.Get("month") != "2026-11" {
		t.Fatalf("Parámetros de exportación: %+v %v %v", options, rest, err)
	}
}

// El permiso de exportación es por formato y se exige además de los permisos del reporte.
func TestPrepareExportRequiresFormatPermission(t *testing.T) {
	sales := []string{PermissionRead, PermissionSales}
	cases := []struct {
		permissions []string
		format      string
		status      int
	}{
		{sales, FormatPDF, 403},
		{sales, FormatXLSX, 403},
		{append(slices.Clone(sales), PermissionExportExcel), FormatPDF, 403},
		{append(slices.Clone(sales), PermissionExportExcel), FormatXLSX, 0},
		{append(slices.Clone(sales), PermissionExportPDF), FormatPDF, 0},
		{[]string{PermissionRead, PermissionExportPDF}, FormatPDF, 403},
	}
	for _, testCase := range cases {
		_, err := prepareExport(Actor{Permissions: testCase.permissions}, "sales.by_day", url.Values{"format": {testCase.format}})
		status := 0
		var problem *Error
		if err != nil && errors.As(err, &problem) {
			status = problem.Status
		}
		if status != testCase.status {
			t.Fatalf("%v %s: %d (%v)", testCase.permissions, testCase.format, status, err)
		}
	}
	if _, err := prepareExport(Actor{Permissions: []string{PermissionRead, PermissionExportPDF}}, "no.existe",
		url.Values{"format": {"pdf"}}); !isCode(err, "NOT_FOUND") {
		t.Fatalf("Un reporte inexistente: %v", err)
	}
	if !strings.Contains(exportTooLarge(61234).(*Error).Message, "61,234 renglones") || exportTooLarge(1).(*Error).Status != 422 {
		t.Fatal("El rechazo por tamaño debe explicar el límite con 422")
	}
}

// La lista cerrada de reportes exportables del puente nativo (desktop/src-tauri/src/export.rs)
// coincide exactamente con el registro de Go: un reporte nuevo debe agregarse también ahí.
func TestExportBridgeListsEveryReport(t *testing.T) {
	content, err := os.ReadFile("../../../desktop/src-tauri/src/export.rs")
	if err != nil {
		t.Fatalf("No se encontró el puente de exportación: %v", err)
	}
	source := string(content)
	start := strings.Index(source, "const EXPORTABLE_REPORTS: &[&str] = &[")
	if start < 0 {
		t.Fatal("export.rs no declara EXPORTABLE_REPORTS")
	}
	end := strings.Index(source[start:], "];")
	listed := regexp.MustCompile(`"([a-z_.]+)"`).FindAllStringSubmatch(source[start:start+end], -1)
	bridge := []string{}
	for _, match := range listed {
		bridge = append(bridge, match[1])
	}
	registered := []string{}
	for _, definition := range Definitions() {
		registered = append(registered, definition.ID)
	}
	slices.Sort(bridge)
	slices.Sort(registered)
	if !slices.Equal(bridge, registered) {
		missing, extra := []string{}, []string{}
		for _, id := range registered {
			if !slices.Contains(bridge, id) {
				missing = append(missing, id)
			}
		}
		for _, id := range bridge {
			if !slices.Contains(registered, id) {
				extra = append(extra, id)
			}
		}
		t.Fatalf("Agrega a EXPORTABLE_REPORTS de desktop/src-tauri/src/export.rs: %v; quita: %v", missing, extra)
	}
}

// PDFPagesForTest expone el extractor de texto a las pruebas de integración (paquete reports_test).
func PDFPagesForTest(t *testing.T, content []byte) []string {
	return pdfPages(t, content)
}
