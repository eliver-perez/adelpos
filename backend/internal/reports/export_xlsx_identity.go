package reports

import (
	"github.com/xuri/excelize/v2"
	"local/ad-el-pos/server/internal/logoraster"
)

// Encabezado de identidad de la hoja «Filtros» del Excel (etapa 9B): el renglón 1 lleva el logotipo
// en la celda A1 (a lo más 200 × 56 píxeles con su proporción, anclado a una celda para que no se
// deforme) y el nombre del establecimiento en B1; el renglón 2 queda vacío y la tabla «Concepto |
// Valor» empieza en el renglón 3 con el establecimiento y, debajo, la dirección, el teléfono y el
// correo que existan. La imagen no lleva hipervínculo ni macro: es la misma imagen preparada para el
// PDF.

const (
	xlsxInfoLabelWidth = 30.0
	// xlsxInfoTableRow es el renglón del encabezado «Concepto | Valor».
	xlsxInfoTableRow = 3
	// Medidas del logotipo en pantalla (píxeles de Excel a 96 ppp: 56 px ≈ 15 mm).
	xlsxLogoMaxWidth  = 200.0
	xlsxLogoMaxHeight = 56.0
	xlsxLogoOffset    = 4
	// xlsxTitleRowHeight es la altura del renglón 1 sin logotipo, en puntos.
	xlsxTitleRowHeight = 24.0
)

// contactRows son los renglones de dirección, teléfono y correo, solo los que existan.
func contactRows(document *exportDocument, text func(string) excelize.Cell) []infoRow {
	rows := []infoRow{}
	if document.Address != "" {
		rows = append(rows, infoRow{label: "Dirección", value: text(document.Address)})
	}
	if document.Phone != "" {
		rows = append(rows, infoRow{label: "Teléfono", value: text(document.Phone)})
	}
	if document.Email != "" {
		rows = append(rows, infoRow{label: "Correo electrónico", value: text(document.Email)})
	}
	return rows
}

// logoDisplayPixels es el tamaño en pantalla del logotipo dentro del máximo, en píxeles enteros.
func logoDisplayPixels(logo *exportLogo) (int, int) {
	width, height := logo.fit(xlsxLogoMaxWidth, xlsxLogoMaxHeight)
	return max(1, int(width+0.5)), max(1, int(height+0.5))
}

// addInfoLogo inserta el logotipo en A1 de la hoja «Filtros». excelize multiplica el tamaño de la
// imagen por la escala y trunca: se suma medio píxel para obtener exactamente el tamaño calculado.
func addInfoLogo(file *excelize.File, logo *exportLogo) error {
	if logo == nil {
		return nil
	}
	width, height := logoDisplayPixels(logo)
	extension := ".png"
	if logo.format == logoraster.FormatJPEG {
		extension = ".jpeg"
	}
	alternative := "Logotipo de AD-EL POS"
	if logo.restaurant {
		alternative = "Logotipo del restaurante"
	}
	return file.AddPictureFromBytes(xlsxInfoSheet, "A1", &excelize.Picture{
		Extension: extension,
		File:      logo.content,
		Format: &excelize.GraphicOptions{
			AltText:         alternative,
			LockAspectRatio: true,
			OffsetX:         xlsxLogoOffset,
			OffsetY:         xlsxLogoOffset,
			ScaleX:          (float64(width) + 0.5) / float64(logo.width),
			ScaleY:          (float64(height) + 0.5) / float64(logo.height),
			Positioning:     "oneCell",
		},
	})
}

// writeInfoIdentity escribe el renglón 1 de «Filtros»: el nombre del establecimiento en B1, con la
// altura necesaria para el logotipo que ocupa A1.
func writeInfoIdentity(stream *excelize.StreamWriter, styles *xlsxStyles, document *exportDocument) error {
	height := xlsxTitleRowHeight
	if document.Logo != nil {
		_, logoHeight := logoDisplayPixels(document.Logo)
		// Píxeles a puntos (96 ppp → 72 ppp), con el margen del logotipo arriba y abajo.
		height = max(height, float64(logoHeight+2*xlsxLogoOffset)*0.75)
	}
	var name any
	if document.Establishment != "" {
		name = excelize.Cell{StyleID: styles.title, Value: excelText(cleanText(document.Establishment))}
	}
	return stream.SetRow("A1", []any{nil, name}, excelize.RowOpts{Height: height})
}
