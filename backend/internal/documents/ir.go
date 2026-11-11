package documents

import "strings"

// Representación intermedia (IR) de un documento: la producen el renderizador único y la consumen
// por igual la vista previa del editor y la impresión. Los transportes la traducen: un layout plano
// viaja exactamente como los documentos anteriores a la 9B (su texto); uno con atributos o imágenes
// se codifica en ESC/POS (EncodeESCPos) cuando el transporte lo admite.

// Tamaños de línea admitidos por ESC/POS (GS !).
const (
	SizeNormal       = "normal"
	SizeDoubleHeight = "double_height"
	SizeDoubleWidth  = "double_width"
	SizeDouble       = "double"
)

// Tamaños de texto del documento completo (comportamiento de Cocina desde la etapa 5).
const (
	TextSizeInherit = "inherit"
	TextSizeNormal  = "normal"
	TextSizeLarge   = "large"
)

// Layout es un documento renderizado. Columns es el ancho en caracteres de una línea de tamaño
// normal (30 en 58 mm y 42 en 80 mm; la mitad con el texto grande de Cocina). TrailingNewline
// conserva el salto final que siempre tuvieron las comandas y que no tienen los demás documentos.
type Layout struct {
	PaperWidth      int    `json:"paper_width"`
	Columns         int    `json:"columns"`
	TextSize        string `json:"text_size"`
	TrailingNewline bool   `json:"trailing_newline,omitempty"`
	Lines           []Line `json:"lines"`
	// Warnings y EmptyBlocks solo informan a la vista previa; la cola no los guarda.
	Warnings    []string     `json:"warnings,omitempty"`
	EmptyBlocks []EmptyBlock `json:"empty_blocks,omitempty"`
}

// Line es una línea ya cortada, alineada y saneada. Una línea de imagen (logotipo) no lleva texto.
// BlockID identifica el bloque que la produjo para que el editor seleccione y resalte sus líneas;
// es vacío en las líneas que no salen de un bloque (las marcas de MUESTRA).
type Line struct {
	Text    string `json:"text"`
	Bold    bool   `json:"bold,omitempty"`
	Size    string `json:"size"`
	Image   *Image `json:"image,omitempty"`
	BlockID string `json:"block_id,omitempty"`
}

// Image es una línea de imagen monocromática ya convertida al ancho imprimible (fase B). Los bits
// no viajan en la IR: el transporte los carga por RasterID al enviar.
type Image struct {
	RasterID   string `json:"raster_id"`
	WidthDots  int    `json:"width_dots"`
	HeightDots int    `json:"height_dots"`
}

// EmptyBlock informa un bloque visible que no imprimió ninguna línea y por qué (por ejemplo «Total
// en dólares» sin dólares o «Logotipo» sin logotipo), para que el editor lo explique.
type EmptyBlock struct {
	BlockID string `json:"block_id"`
	Reason  string `json:"reason"`
}

// Text es la proyección de texto del documento: las líneas de texto unidas por saltos de línea (las
// de imagen no aportan nada). Para las plantillas predeterminadas es idéntica al texto anterior.
func (layout *Layout) Text() string {
	parts := []string{}
	for _, line := range layout.Lines {
		if line.Image != nil {
			continue
		}
		parts = append(parts, line.Text)
	}
	text := strings.Join(parts, "\n")
	if layout.TrailingNewline {
		text += "\n"
	}
	return text
}

// Plain indica que ninguna línea lleva negrita, un tamaño distinto del normal ni una imagen: el
// documento se imprime por el camino anterior a la 9B, con los mismos bytes y argumentos.
func (layout *Layout) Plain() bool {
	for _, line := range layout.Lines {
		if line.Bold || line.Image != nil {
			return false
		}
		if line.Size != "" && line.Size != SizeNormal {
			return false
		}
	}
	return true
}

// HasImages indica si el documento lleva al menos una línea de imagen.
func (layout *Layout) HasImages() bool {
	for _, line := range layout.Lines {
		if line.Image != nil {
			return true
		}
	}
	return false
}

// ForQueue devuelve la copia que se guarda en kitchen_print_jobs.layout: las mismas líneas sin
// identificadores de bloque, avisos ni bloques vacíos, que solo sirven al editor.
func (layout *Layout) ForQueue() *Layout {
	stored := &Layout{
		PaperWidth:      layout.PaperWidth,
		Columns:         layout.Columns,
		TextSize:        layout.TextSize,
		TrailingNewline: layout.TrailingNewline,
		Lines:           make([]Line, 0, len(layout.Lines)),
	}
	for _, line := range layout.Lines {
		line.BlockID = ""
		stored.Lines = append(stored.Lines, line)
	}
	return stored
}
