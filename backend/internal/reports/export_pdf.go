package reports

import (
	"bytes"
	"context"
	"math"
	"slices"
	"strconv"
	"strings"

	"codeberg.org/go-pdf/fpdf"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// PDF de un reporte (§17) con codeberg.org/go-pdf/fpdf y las fuentes Go (Go Regular y Go Bold,
// golang.org/x/image/font/gofont) incrustadas como UTF-8: el texto con acentos, «ñ», «¿¡» y «€» se
// puede copiar y buscar. Estructura:
//   - primera página: encabezado de identidad (logotipo del restaurante o, si no tiene, el de
//     AD-EL POS, con el nombre, la dirección, el teléfono y el correo del establecimiento;
//     export_pdf_identity.go), nombre y descripción del reporte, periodo, comparación, filtros
//     aplicados, fecha usada, hora de inicio del día, orden, renglones y fecha de generación;
//     después las cifras resumidas y los avisos;
//   - páginas siguientes: un encabezado breve (reporte, establecimiento y periodo);
//   - tabla con los encabezados de columna repetidos en cada página, renglones alternados, totales
//     de Go al final y las notas del reporte;
//   - pie con «Página n de m».
//
// Carta (o A4 si se pide) vertical si la tabla cabe con letra de 8 pt; si no, horizontal con la letra
// necesaria (hasta 5.5 pt). Las columnas de importes, cantidades, fechas y códigos nunca se parten:
// su ancho es el de su valor más largo; solo las columnas de texto se ajustan en varias líneas.

const (
	pdfFamily      = "go"
	pdfMargin      = 12.0
	pdfFooterSpace = 14.0
	pdfCellMargin  = 1.2
	pdfRowPadding  = 0.9
	pdfMaxFontSize = 8.0
	pdfMinFontSize = 5.5
	// pdfTextCap es el ancho máximo preferido (en mm) de una columna de texto antes de ajustarse en
	// varias líneas; pdfWordCap limita la palabra más larga que se considera como mínimo.
	pdfTextCap = 70.0
	pdfWordCap = 40.0
)

// Colores (RGB) de la presentación.
var (
	pdfInk       = [3]int{28, 34, 42}
	pdfMuted     = [3]int{88, 98, 110}
	pdfRule      = [3]int{176, 186, 196}
	pdfHeaderBG  = [3]int{224, 231, 238}
	pdfStripeBG  = [3]int{246, 248, 250}
	pdfTotalsBG  = [3]int{236, 241, 245}
	pdfWarningBG = [3]int{253, 243, 222}
)

// pdfColumn guarda las medidas de una columna con letra de 1 pt (en mm), para escalar a cualquier
// tamaño sin volver a medir.
type pdfColumn struct {
	column     Column
	numeric    bool
	wraps      bool
	align      string
	valueWidth float64
	// typicalWidth es el ancho del percentil 90 de los valores: un texto excepcionalmente largo se
	// parte en varias líneas en lugar de ensanchar la columna para todos los renglones.
	typicalWidth float64
	longestWord  float64
	headerWord   float64
	width        float64
}

type pdfLayout struct {
	orientation string
	fontSize    float64
	columns     []pdfColumn
}

func newPDF(orientation, paper string) *fpdf.Fpdf {
	size := "Letter"
	if paper == PaperA4 {
		size = "A4"
	}
	pdf := fpdf.New(orientation, "mm", size, "")
	pdf.AddUTF8FontFromBytes(pdfFamily, "", goregular.TTF)
	pdf.AddUTF8FontFromBytes(pdfFamily, "B", gobold.TTF)
	pdf.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	pdf.SetCellMargin(pdfCellMargin)
	pdf.SetAutoPageBreak(true, pdfFooterSpace)
	return pdf
}

// lineHeight es el alto de una línea de texto (mm) para un tamaño de letra en puntos.
func lineHeight(size float64) float64 {
	return size * 0.3528 * 1.25
}

// pdfCellTexts convierte todos los renglones en el texto de cada celda (una sola vez, para medir y
// dibujar).
func pdfCellTexts(document *exportDocument) [][]string {
	result := document.Result
	cells := make([][]string, len(result.AllRows))
	for index, row := range result.AllRows {
		texts := make([]string, len(result.Columns))
		for position, column := range result.Columns {
			value, found := row[column.Key]
			if !found || value == nil {
				texts[position] = exportEmpty
				continue
			}
			texts[position] = cleanText(displayValue(column.Type, value, document.Location))
		}
		cells[index] = texts
	}
	return cells
}

func pdfTotalTexts(document *exportDocument) []string {
	result := document.Result
	texts := make([]string, len(result.Columns))
	labelled := false
	for position, column := range result.Columns {
		value, found := result.Totals[column.Key]
		if found && value != nil {
			texts[position] = cleanText(displayValue(column.Type, value, document.Location))
			continue
		}
		if !labelled && !numericColumn(column.Type) {
			texts[position] = "Total"
			labelled = true
		}
	}
	return texts
}

// measureColumns mide encabezados y valores con letra de 1 pt.
func measureColumns(pdf *fpdf.Fpdf, columns []Column, cells [][]string, totals []string) []pdfColumn {
	const reference = 10.0
	measured := make([]pdfColumn, len(columns))
	pdf.SetFont(pdfFamily, "B", reference)
	for index, column := range columns {
		entry := pdfColumn{column: column, numeric: numericColumn(column.Type), align: "L"}
		if entry.numeric {
			entry.align = "R"
		}
		entry.wraps = column.Type == TypeText
		for _, word := range strings.Fields(column.Label) {
			entry.headerWord = math.Max(entry.headerWord, pdf.GetStringWidth(word)/reference)
		}
		entry.valueWidth = pdf.GetStringWidth(totals[index]) / reference
		measured[index] = entry
	}
	pdf.SetFont(pdfFamily, "", reference)
	widths := make([][]float64, len(columns))
	for _, texts := range cells {
		for index, text := range texts {
			width := pdf.GetStringWidth(text) / reference
			entry := &measured[index]
			if width > entry.valueWidth {
				entry.valueWidth = width
			}
			if entry.wraps {
				widths[index] = append(widths[index], width)
				if width > entry.longestWord {
					for _, word := range strings.Fields(text) {
						entry.longestWord = math.Max(entry.longestWord, pdf.GetStringWidth(word)/reference)
					}
				}
			}
		}
	}
	for index := range measured {
		measured[index].typicalWidth = measured[index].valueWidth
		if values := widths[index]; len(values) > 0 {
			slices.Sort(values)
			measured[index].typicalWidth = values[(len(values)-1)*9/10]
		}
	}
	return measured
}

// fitColumns calcula los anchos para un tamaño de letra; ok es false si las columnas que no se
// parten no caben.
func fitColumns(columns []pdfColumn, size, available float64) ([]pdfColumn, bool) {
	padding := 2 * pdfCellMargin
	fitted := make([]pdfColumn, len(columns))
	copy(fitted, columns)
	minimums := make([]float64, len(columns))
	preferred := make([]float64, len(columns))
	minimumTotal, preferredTotal := 0.0, 0.0
	for index, column := range fitted {
		header := column.headerWord*size + padding
		if column.wraps {
			word := math.Min(column.longestWord*size+padding, pdfWordCap)
			minimums[index] = math.Max(header, math.Max(word, 14))
			preferred[index] = math.Max(minimums[index], math.Min(column.typicalWidth*size+padding, pdfTextCap))
		} else {
			minimums[index] = math.Max(header, column.valueWidth*size+padding)
			preferred[index] = minimums[index]
		}
		minimumTotal += minimums[index]
		preferredTotal += preferred[index]
	}
	if minimumTotal > available {
		return nil, false
	}
	if preferredTotal <= available {
		// Sobra espacio: la tabla ocupa todo el ancho repartiendo el sobrante en proporción.
		scale := available / preferredTotal
		for index := range fitted {
			fitted[index].width = preferred[index] * scale
		}
		return fitted, true
	}
	// Las columnas de texto ceden en proporción a lo que les falta para su ancho preferido.
	extra := available - minimumTotal
	flexible := preferredTotal - minimumTotal
	for index := range fitted {
		fitted[index].width = minimums[index] + (preferred[index]-minimums[index])*extra/flexible
	}
	return fitted, true
}

// chooseLayout decide orientación, letra y anchos: vertical con 8 pt si cabe; si no, horizontal con
// la letra mayor que permita que importes, cantidades, fechas y códigos quepan completos.
func chooseLayout(document *exportDocument, cells [][]string, totals []string) pdfLayout {
	measure := newPDF("P", document.Paper)
	measured := measureColumns(measure, document.Result.Columns, cells, totals)
	width, height := measure.GetPageSize()
	portrait := width - 2*pdfMargin
	landscape := height - 2*pdfMargin
	if fitted, ok := fitColumns(measured, pdfMaxFontSize, portrait); ok {
		return pdfLayout{orientation: "P", fontSize: pdfMaxFontSize, columns: fitted}
	}
	for size := pdfMaxFontSize; size >= pdfMinFontSize; size -= 0.5 {
		if fitted, ok := fitColumns(measured, size, landscape); ok {
			return pdfLayout{orientation: "L", fontSize: size, columns: fitted}
		}
	}
	// Último recurso (decenas de columnas): letra mínima y anchos proporcionales; un valor que no
	// quepa se parte en varias líneas, nunca se recorta.
	fitted := make([]pdfColumn, len(measured))
	copy(fitted, measured)
	total := 0.0
	for _, column := range fitted {
		total += math.Max(column.headerWord, column.valueWidth)*pdfMinFontSize + 2*pdfCellMargin
	}
	for index, column := range fitted {
		fitted[index].width = (math.Max(column.headerWord, column.valueWidth)*pdfMinFontSize + 2*pdfCellMargin) * landscape / total
		fitted[index].wraps = true
	}
	return pdfLayout{orientation: "L", fontSize: pdfMinFontSize, columns: fitted}
}

// pdfWriter dibuja el documento con un layout ya decidido.
type pdfWriter struct {
	pdf       *fpdf.Fpdf
	document  *exportDocument
	layout    pdfLayout
	left      float64
	width     float64
	bottom    float64
	tableOpen bool
}

func (writer *pdfWriter) color(rgb [3]int) {
	writer.pdf.SetTextColor(rgb[0], rgb[1], rgb[2])
}

func (writer *pdfWriter) fill(rgb [3]int) {
	writer.pdf.SetFillColor(rgb[0], rgb[1], rgb[2])
}

func (writer *pdfWriter) rule(rgb [3]int, thickness float64) {
	writer.pdf.SetDrawColor(rgb[0], rgb[1], rgb[2])
	writer.pdf.SetLineWidth(thickness)
}

// pageHeader es el encabezado breve de las páginas siguientes a la primera.
func (writer *pdfWriter) pageHeader() {
	pdf := writer.pdf
	result := writer.document.Result
	pdf.SetXY(writer.left, pdfMargin)
	pdf.SetFont(pdfFamily, "B", 8.5)
	writer.color(pdfInk)
	pdf.CellFormat(writer.width/2, 4.5, cleanText(result.Meta.Title), "", 0, "L", false, 0, "")
	pdf.SetFont(pdfFamily, "", 7.5)
	writer.color(pdfMuted)
	detail := periodDescription(result.Meta.Period)
	if writer.document.Establishment != "" {
		detail = writer.document.Establishment + " · " + detail
	}
	pdf.CellFormat(writer.width/2, 4.5, cleanText(detail), "", 1, "R", false, 0, "")
	writer.rule(pdfRule, 0.2)
	pdf.Line(writer.left, pdf.GetY()+0.8, writer.left+writer.width, pdf.GetY()+0.8)
	pdf.SetXY(writer.left, pdf.GetY()+2.5)
}

func (writer *pdfWriter) footer() {
	pdf := writer.pdf
	result := writer.document.Result
	pdf.SetXY(writer.left, -pdfFooterSpace+3)
	writer.rule(pdfRule, 0.2)
	pdf.Line(writer.left, pdf.GetY(), writer.left+writer.width, pdf.GetY())
	pdf.SetXY(writer.left, pdf.GetY()+1.2)
	pdf.SetFont(pdfFamily, "", 7)
	writer.color(pdfMuted)
	generated := "AD-EL POS · Generado el " + localDateTime(result.Meta.GeneratedAt, writer.document.Location)
	pdf.CellFormat(writer.width*0.7, 4, generated, "", 0, "L", false, 0, "")
	pdf.CellFormat(writer.width*0.3, 4, "Página "+strconv.Itoa(pdf.PageNo())+" de {nb}", "", 0, "R", false, 0, "")
}

// keyValue dibuja «Etiqueta: valor» con el valor ajustado en varias líneas.
func (writer *pdfWriter) keyValue(label, value string) {
	pdf := writer.pdf
	const labelWidth = 40.0
	pdf.SetX(writer.left)
	pdf.SetFont(pdfFamily, "B", 8)
	writer.color(pdfMuted)
	pdf.CellFormat(labelWidth, 4.2, label, "", 0, "L", false, 0, "")
	pdf.SetFont(pdfFamily, "", 8)
	writer.color(pdfInk)
	pdf.MultiCell(writer.width-labelWidth, 4.2, cleanText(value), "", "L", false)
}

// firstPageHeader es el encabezado completo de la primera página.
func (writer *pdfWriter) firstPageHeader() {
	pdf := writer.pdf
	document := writer.document
	result := document.Result
	meta := result.Meta
	pdf.SetXY(writer.left, pdfMargin)
	writer.identityHeader()
	pdf.SetFont(pdfFamily, "B", 15)
	writer.color(pdfInk)
	pdf.MultiCell(writer.width, 7, cleanText(meta.Title), "", "L", false)
	if result.Report.Description != "" {
		pdf.SetFont(pdfFamily, "", 8.5)
		writer.color(pdfMuted)
		pdf.MultiCell(writer.width, 4.2, cleanText(result.Report.Description), "", "L", false)
	}
	pdf.Ln(2)
	writer.keyValue("Periodo", periodDescription(meta.Period))
	if meta.Comparison != nil {
		writer.keyValue("Comparación", meta.Comparison.Label)
	}
	filters := filterLines(result)
	if len(filters) == 0 {
		writer.keyValue("Filtros", "Sin filtros")
	} else {
		writer.keyValue("Filtros", strings.Join(filters, "; "))
	}
	writer.keyValue("Fecha usada", meta.DateBasis)
	writer.keyValue("Hora de inicio del día", meta.DayStart+" (zona "+meta.Timezone+")")
	writer.keyValue("Orden", sortDescription(result))
	writer.keyValue("Renglones", groupDigits(strconv.Itoa(len(result.AllRows))))
	writer.keyValue("Generado", localDateTime(meta.GeneratedAt, document.Location))
	writer.summary()
	writer.warnings()
	pdf.Ln(3)
}

// summary dibuja las cifras resumidas en una rejilla de hasta cuatro por línea.
func (writer *pdfWriter) summary() {
	figures := writer.document.Result.Summary
	if len(figures) == 0 {
		return
	}
	pdf := writer.pdf
	pdf.Ln(2)
	perLine := 4
	cellWidth := writer.width / float64(perLine)
	for start := 0; start < len(figures); start += perLine {
		end := min(start+perLine, len(figures))
		y := pdf.GetY()
		if y+11 > writer.bottom {
			pdf.AddPage()
			y = pdf.GetY()
		}
		for index, figure := range figures[start:end] {
			x := writer.left + float64(index)*cellWidth
			writer.fill(pdfTotalsBG)
			pdf.Rect(x+0.5, y, cellWidth-1, 10, "F")
			pdf.SetXY(x+0.5, y+0.6)
			pdf.SetFont(pdfFamily, "", 7)
			writer.color(pdfMuted)
			pdf.CellFormat(cellWidth-1, 3.6, cleanText(figure.Label), "", 0, "L", false, 0, "")
			pdf.SetXY(x+0.5, y+4.5)
			pdf.SetFont(pdfFamily, "B", 9.5)
			writer.color(pdfInk)
			value := exportEmpty
			if figure.Value != nil {
				value = cleanText(displayValue(figure.Type, figure.Value, writer.document.Location))
			}
			pdf.CellFormat(cellWidth-1, 4.8, value, "", 0, "L", false, 0, "")
		}
		pdf.SetXY(writer.left, y+11)
	}
}

func (writer *pdfWriter) warnings() {
	list := writer.document.Result.Meta.Warnings
	if len(list) == 0 {
		return
	}
	pdf := writer.pdf
	pdf.Ln(1.5)
	pdf.SetFont(pdfFamily, "", 8)
	writer.color(pdfInk)
	writer.fill(pdfWarningBG)
	for _, warning := range list {
		pdf.SetX(writer.left)
		pdf.MultiCell(writer.width, 4.4, "Aviso: "+cleanText(warning.Message), "", "L", true)
	}
}

// splitCell parte el texto de una celda en líneas del ancho de su columna.
func (writer *pdfWriter) splitCell(text string, column pdfColumn, maximumLines int) []string {
	if text == "" {
		return []string{""}
	}
	if !column.wraps && writer.pdf.GetStringWidth(text) <= column.width-2*pdfCellMargin {
		return []string{text}
	}
	lines := writer.pdf.SplitText(text, column.width)
	if len(lines) == 0 {
		return []string{""}
	}
	if len(lines) > maximumLines {
		lines = lines[:maximumLines]
		lines[maximumLines-1] = strings.TrimRight(lines[maximumLines-1], " ") + "…"
	}
	return lines
}

// tableHeader dibuja los encabezados de columna (también al inicio de cada página nueva).
func (writer *pdfWriter) tableHeader() {
	pdf := writer.pdf
	size := writer.layout.fontSize
	line := lineHeight(size)
	pdf.SetFont(pdfFamily, "B", size)
	split := make([][]string, len(writer.layout.columns))
	lines := 1
	for index, column := range writer.layout.columns {
		split[index] = pdf.SplitText(cleanText(column.column.Label), column.width)
		if len(split[index]) == 0 {
			split[index] = []string{""}
		}
		lines = max(lines, len(split[index]))
	}
	height := float64(lines)*line + 2*pdfRowPadding
	y := pdf.GetY()
	writer.fill(pdfHeaderBG)
	pdf.Rect(writer.left, y, writer.width, height, "F")
	writer.color(pdfInk)
	x := writer.left
	for index, column := range writer.layout.columns {
		for number, text := range split[index] {
			pdf.SetXY(x, y+pdfRowPadding+float64(number)*line)
			pdf.CellFormat(column.width, line, text, "", 0, column.align, false, 0, "")
		}
		x += column.width
	}
	writer.rule(pdfRule, 0.3)
	pdf.Line(writer.left, y+height, writer.left+writer.width, y+height)
	pdf.SetXY(writer.left, y+height)
}

// row dibuja un renglón; si no cabe en la página, abre otra (con sus encabezados).
func (writer *pdfWriter) row(texts []string, style string, stripe bool) {
	pdf := writer.pdf
	size := writer.layout.fontSize
	line := lineHeight(size)
	fontStyle := ""
	if style == "total" {
		fontStyle = "B"
	}
	pdf.SetFont(pdfFamily, fontStyle, size)
	maximumLines := max(1, int((writer.bottom-pdfMargin-30)/line))
	split := make([][]string, len(texts))
	lines := 1
	for index, text := range texts {
		split[index] = writer.splitCell(text, writer.layout.columns[index], maximumLines)
		lines = max(lines, len(split[index]))
	}
	height := float64(lines)*line + 2*pdfRowPadding
	if pdf.GetY()+height > writer.bottom {
		pdf.AddPage()
		pdf.SetFont(pdfFamily, fontStyle, size)
	}
	y := pdf.GetY()
	switch {
	case style == "total":
		writer.fill(pdfTotalsBG)
		pdf.Rect(writer.left, y, writer.width, height, "F")
		writer.rule(pdfInk, 0.35)
		pdf.Line(writer.left, y, writer.left+writer.width, y)
	case stripe:
		writer.fill(pdfStripeBG)
		pdf.Rect(writer.left, y, writer.width, height, "F")
	}
	writer.color(pdfInk)
	x := writer.left
	for index, column := range writer.layout.columns {
		for number, text := range split[index] {
			pdf.SetXY(x, y+pdfRowPadding+float64(number)*line)
			pdf.CellFormat(column.width, line, text, "", 0, column.align, false, 0, "")
		}
		x += column.width
	}
	pdf.SetXY(writer.left, y+height)
}

func (writer *pdfWriter) notes() {
	notes := writer.document.Result.Meta.Notes
	if len(notes) == 0 {
		return
	}
	pdf := writer.pdf
	pdf.Ln(3)
	pdf.SetX(writer.left)
	pdf.SetFont(pdfFamily, "B", 8)
	writer.color(pdfInk)
	pdf.CellFormat(writer.width, 4.4, "Notas", "", 1, "L", false, 0, "")
	pdf.SetFont(pdfFamily, "", 7.5)
	writer.color(pdfMuted)
	for _, note := range notes {
		pdf.SetX(writer.left)
		pdf.MultiCell(writer.width, 4, "• "+cleanText(note), "", "L", false)
	}
}

// renderPDF genera el PDF y devuelve también la orientación elegida (portrait o landscape).
func renderPDF(ctx context.Context, document *exportDocument) ([]byte, string, error) {
	result := document.Result
	cells := pdfCellTexts(document)
	totals := pdfTotalTexts(document)
	layout := chooseLayout(document, cells, totals)
	pdf := newPDF(layout.orientation, document.Paper)
	width, height := pdf.GetPageSize()
	writer := &pdfWriter{pdf: pdf, document: document, layout: layout, left: pdfMargin, width: width - 2*pdfMargin,
		bottom: height - pdfFooterSpace}
	generated := result.Meta.GeneratedAt
	pdf.SetCreationDate(generated)
	pdf.SetModificationDate(generated)
	pdf.SetCatalogSort(true)
	pdf.SetTitle(result.Meta.Title, true)
	pdf.SetSubject(periodDescription(result.Meta.Period), true)
	pdf.SetAuthor(document.Establishment, true)
	pdf.SetCreator("AD-EL POS", true)
	pdf.SetProducer("AD-EL POS", true)
	pdf.SetLang("es-MX")
	pdf.AliasNbPages("{nb}")
	pdf.SetHeaderFunc(func() {
		if pdf.PageNo() > 1 {
			writer.pageHeader()
		}
		if writer.tableOpen {
			writer.tableHeader()
		}
	})
	pdf.SetFooterFunc(writer.footer)
	pdf.AddPage()
	writer.firstPageHeader()
	writer.tableOpen = true
	if pdf.GetY()+30 > writer.bottom {
		pdf.AddPage()
	} else {
		writer.tableHeader()
	}
	if len(cells) == 0 {
		pdf.SetFont(pdfFamily, "", layout.fontSize+0.5)
		writer.color(pdfMuted)
		pdf.SetX(writer.left)
		pdf.CellFormat(writer.width, 8, "No hay datos para el periodo y los filtros seleccionados.", "", 1, "L", false, 0, "")
	}
	for index, texts := range cells {
		if index%500 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, "", err
			}
		}
		writer.row(texts, "", index%2 == 1)
	}
	if len(cells) > 0 {
		writer.row(totals, "total", false)
	}
	writer.tableOpen = false
	writer.notes()
	var buffer bytes.Buffer
	if err := pdf.Output(&buffer); err != nil {
		return nil, "", err
	}
	orientation := "portrait"
	if layout.orientation == "L" {
		orientation = "landscape"
	}
	return buffer.Bytes(), orientation, nil
}
