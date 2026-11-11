package reports

import (
	"bytes"

	"codeberg.org/go-pdf/fpdf"
)

// Encabezado de identidad de la primera página del PDF (etapa 9B): el logotipo a la izquierda, a lo
// más de 50 × 14 mm con su proporción, y a su derecha el nombre del establecimiento y, debajo, su
// dirección y una línea con teléfono y correo (solo los que existan), centrados verticalmente
// respecto al logotipo. Una línea divisoria lo separa del título del reporte. Las páginas siguientes
// conservan su encabezado breve (pageHeader).

const (
	pdfLogoMaxWidth  = 50.0
	pdfLogoMaxHeight = 14.0
	pdfLogoGap       = 4.0
	// pdfLogoName es el nombre con el que fpdf registra la imagen (una sola por documento).
	pdfLogoName = "identity-logo"
)

// identityBlock es un texto del encabezado con su letra; lines es el texto ya partido al ancho
// disponible.
type identityBlock struct {
	text       string
	style      string
	size       float64
	lineHeight float64
	ink        [3]int
	lines      []string
}

// identityBlocks parte nombre, dirección y contacto en líneas del ancho disponible y omite los
// vacíos.
func (writer *pdfWriter) identityBlocks(width float64) []identityBlock {
	document := writer.document
	candidates := []identityBlock{
		{text: document.Establishment, style: "B", size: 11, lineHeight: 5.2, ink: pdfInk},
		{text: document.Address, style: "", size: 8, lineHeight: 4, ink: pdfMuted},
		{text: document.contactLine(), style: "", size: 8, lineHeight: 4, ink: pdfMuted},
	}
	blocks := []identityBlock{}
	for _, block := range candidates {
		text := cleanText(block.text)
		if text == "" {
			continue
		}
		writer.pdf.SetFont(pdfFamily, block.style, block.size)
		block.lines = writer.pdf.SplitText(text, width)
		if len(block.lines) == 0 {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// identityHeader dibuja el encabezado de identidad desde la posición actual y deja el cursor al
// margen izquierdo, debajo de la línea divisoria. Sin logotipo ni datos no dibuja nada.
func (writer *pdfWriter) identityHeader() {
	pdf := writer.pdf
	top := pdf.GetY()
	textLeft := writer.left
	logoHeight := 0.0
	if logo := writer.document.Logo; logo != nil {
		logoWidth, height := logo.fit(pdfLogoMaxWidth, pdfLogoMaxHeight)
		options := fpdf.ImageOptions{ImageType: logo.format}
		pdf.RegisterImageOptionsReader(pdfLogoName, options, bytes.NewReader(logo.content))
		pdf.ImageOptions(pdfLogoName, writer.left, top, logoWidth, height, false, options, 0, "")
		textLeft += logoWidth + pdfLogoGap
		logoHeight = height
	}
	blocks := writer.identityBlocks(writer.left + writer.width - textLeft)
	if logoHeight == 0 && len(blocks) == 0 {
		return
	}
	textHeight := 0.0
	for _, block := range blocks {
		textHeight += float64(len(block.lines)) * block.lineHeight
	}
	y := top
	if textHeight < logoHeight {
		y += (logoHeight - textHeight) / 2
	}
	for _, block := range blocks {
		pdf.SetFont(pdfFamily, block.style, block.size)
		writer.color(block.ink)
		for _, line := range block.lines {
			pdf.SetXY(textLeft, y)
			pdf.CellFormat(writer.left+writer.width-textLeft, block.lineHeight, line, "", 0, "L", false, 0, "")
			y += block.lineHeight
		}
	}
	bottom := max(top+logoHeight, y)
	writer.rule(pdfRule, 0.2)
	pdf.Line(writer.left, bottom+1.5, writer.left+writer.width, bottom+1.5)
	pdf.SetXY(writer.left, bottom+4)
}
