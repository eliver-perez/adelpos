package documents

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func renderSample(t *testing.T, kind Kind, raw string, options RenderOptions) *Layout {
	t.Helper()
	template := mustParse(t, kind, raw)
	layout, err := Render(kind, template, SampleData(kind, Identity{Name: "Restaurante"}), options)
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

// Alineación por relleno de espacios, doble ancho a la mitad de columnas, negrita, mayúsculas,
// espaciado e identificador de bloque en cada línea, sin cambiar Text().
func TestBlockPropertiesShapeLines(t *testing.T) {
	layout := renderSample(t, KindReceipt, `{"blocks":[
		{"id":"nombre","type":"establishment_name","align":"center","size":"double_width","bold":true,"uppercase":true,"spacing_before":1,"spacing_after":2},
		{"id":"total","type":"total","align":"right"},
		{"id":"texto","type":"fixed_text","align":"right","options":{"text":"Gracias\nVuelva pronto"}},
		{"id":"f","type":"fiscal_notice"}]}`, RenderOptions{PaperWidth: 80})
	if layout.Columns != 42 || layout.Plain() {
		t.Fatalf("Columnas o atributos incorrectos: %+v", layout)
	}
	name := layout.Lines[1]
	if layout.Lines[0].Text != "" || layout.Lines[0].BlockID != "nombre" || name.Text != strings.Repeat(" ", 5)+"RESTAURANTE" ||
		!name.Bold || name.Size != SizeDoubleWidth || name.BlockID != "nombre" {
		t.Fatalf("El nombre no se centró en 21 columnas en negrita y mayúsculas: %+v", layout.Lines[:2])
	}
	if layout.Lines[2].Text != "" || layout.Lines[3].Text != "" || layout.Lines[3].BlockID != "nombre" {
		t.Fatal("Falta el espaciado posterior del bloque")
	}
	total := layout.Lines[4]
	if !strings.HasPrefix(total.Text, "TOTAL ") || runeCount(total.Text) != 42 || total.BlockID != "total" {
		t.Fatalf("El total no quedó justificado a todo el ancho: %q", total.Text)
	}
	if layout.Lines[5].Text != strings.Repeat(" ", 35)+"Gracias" || layout.Lines[6].Text != strings.Repeat(" ", 29)+"Vuelva pronto" {
		t.Fatalf("El texto fijo no se alineó a la derecha por línea: %q %q", layout.Lines[5].Text, layout.Lines[6].Text)
	}
	text := layout.Text()
	if strings.Contains(text, "nombre") || !strings.HasPrefix(text, "\n     RESTAURANTE\n\n\nTOTAL") {
		t.Fatalf("Text() no es la proyección exacta de las líneas:\n%q", text)
	}
	stored, _ := json.Marshal(layout.ForQueue())
	if bytes.Contains(stored, []byte("block_id")) || bytes.Contains(stored, []byte("empty_blocks")) {
		t.Fatalf("La IR de la cola conserva datos del editor: %s", stored)
	}
}

// Un bloque visible sin líneas se informa con su motivo; el logotipo sin cargar avisa sin fallar.
func TestEmptyBlocksAreReported(t *testing.T) {
	layout := renderSample(t, KindPrecheck, `{"blocks":[
		{"id":"logo","type":"logo"},
		{"id":"dolares","type":"usd_total"},
		{"id":"saldo","type":"balance"},
		{"id":"oculto","type":"total","visible":false},
		{"id":"f","type":"fiscal_notice"}]}`, RenderOptions{PaperWidth: 58})
	data := SampleData(KindPrecheck, Identity{Name: "R"})
	data.Commercial.USDTotal = nil
	template := mustParse(t, KindPrecheck, `{"blocks":[{"id":"dolares","type":"usd_total"},{"id":"f","type":"fiscal_notice"}]}`)
	withoutDollars, _ := Render(KindPrecheck, template, data, RenderOptions{PaperWidth: 58})
	if len(withoutDollars.EmptyBlocks) != 1 || withoutDollars.EmptyBlocks[0].BlockID != "dolares" || !strings.Contains(withoutDollars.EmptyBlocks[0].Reason, "dólares") {
		t.Fatalf("El total en dólares vacío no se informó: %+v", withoutDollars.EmptyBlocks)
	}
	if len(layout.EmptyBlocks) != 1 || layout.EmptyBlocks[0].BlockID != "logo" || len(layout.Warnings) != 1 ||
		!strings.Contains(layout.Warnings[0], "No hay logotipo") {
		t.Fatalf("El logotipo sin cargar no se informó: %+v %+v", layout.EmptyBlocks, layout.Warnings)
	}
	for _, line := range layout.Lines {
		if line.BlockID == "oculto" || line.Image != nil {
			t.Fatal("Un bloque oculto o un logotipo inexistente imprimió algo")
		}
	}
}

// Con logotipo, la línea de imagen corresponde al ancho del papel y no aporta texto.
func TestLogoLineUsesRasterOfPaperWidth(t *testing.T) {
	identity := Identity{Name: "R", Logo: []LogoRaster{
		{PaperWidth: 58, RasterID: "raster-58", WidthDots: 384, HeightDots: 80},
		{PaperWidth: 80, RasterID: "raster-80", WidthDots: 576, HeightDots: 120},
	}}
	template := mustParse(t, KindKitchenOrder, `{"blocks":[{"id":"logo","type":"logo","spacing_after":1},{"id":"titulo","type":"document_title"}]}`)
	for _, paper := range []int{58, 80} {
		layout, err := Render(KindKitchenOrder, template, SampleData(KindKitchenOrder, identity), RenderOptions{PaperWidth: paper})
		if err != nil {
			t.Fatal(err)
		}
		image := layout.Lines[0].Image
		if image == nil || image.RasterID != "raster-"+map[int]string{58: "58", 80: "80"}[paper] || layout.Plain() || !layout.HasImages() {
			t.Fatalf("El logotipo no usó el raster de %d mm: %+v", paper, layout.Lines[0])
		}
		if layout.Text() != "\nCOMANDA\n" {
			t.Fatalf("La línea de imagen aportó texto: %q", layout.Text())
		}
	}
}

// El bloque protegido solo imprime el esperado con IncludeExpected; si no, la nota de siempre.
func TestProtectedBlockFollowsPermissionRule(t *testing.T) {
	raw := `{"blocks":[{"id":"esperado","type":"expected_differences"},{"id":"f","type":"fiscal_notice"}]}`
	hidden := renderSample(t, KindCashFinalCut, raw, RenderOptions{PaperWidth: 80})
	if !strings.Contains(hidden.Text(), "Esperado y diferencias: no incluidos.") || strings.Contains(hidden.Text(), "2,150.00") {
		t.Fatalf("El esperado se imprimió sin permiso:\n%s", hidden.Text())
	}
	shown := renderSample(t, KindCashFinalCut, raw, RenderOptions{PaperWidth: 80, IncludeExpected: true})
	for _, fragment := range []string{"EFECTIVO ESPERADO", "$2,150.00", "DIFERENCIAS (CONTADO - ESPERADO)", "Pesos · Faltante", "-$50.00"} {
		if !strings.Contains(shown.Text(), fragment) {
			t.Fatalf("Con permiso falta %q:\n%s", fragment, shown.Text())
		}
	}
}

// Las opciones heredables de la lista de cocina siguen la configuración copiada en la comanda o la
// sustituyen; el texto grande reduce las columnas a la mitad e ignora los tamaños por bloque.
func TestKitchenItemOptionsAndLargeText(t *testing.T) {
	inherit := renderSample(t, KindKitchenOrder, `{"blocks":[{"id":"lista","type":"kitchen_items"}]}`, RenderOptions{PaperWidth: 80})
	if strings.Contains(inherit.Text(), "Importe:") || !strings.Contains(inherit.Text(), "NOTA: Observación de muestra") {
		t.Fatalf("La lista no heredó la configuración de Cocina:\n%s", inherit.Text())
	}
	custom := renderSample(t, KindKitchenOrder, `{"blocks":[{"id":"lista","type":"kitchen_items","options":{"show_prices":"yes","show_notes":"no",
		"quantity_style":"multiplier","line_spacing":"0","show_item_reference":false,"show_station_headings":false,"station_divider":false}}]}`, RenderOptions{PaperWidth: 80})
	text := custom.Text()
	for _, absent := range []string{"NOTA:", "Partida:", "ESTACIÓN", "------"} {
		if strings.Contains(text, absent) {
			t.Fatalf("La lista personalizada imprimió %q:\n%s", absent, text)
		}
	}
	if !strings.Contains(text, "Platillo de muestra × 2") || !strings.Contains(text, "Importe: $250.00") {
		t.Fatalf("La lista personalizada no aplicó sus opciones:\n%s", text)
	}
	large := renderSample(t, KindKitchenOrder, `{"options":{"text_size":"large"},"blocks":[{"id":"titulo","type":"document_title","size":"double_width","bold":true}]}`, RenderOptions{PaperWidth: 58})
	if large.Columns != 15 || large.TextSize != TextSizeLarge || large.Lines[0].Size != SizeNormal || !large.Lines[0].Bold {
		t.Fatalf("El texto grande no ignoró el tamaño del bloque: %+v", large)
	}
}

// El campo de datos une sus datos, admite etiqueta y valor y omite el renglón sin datos.
func TestFieldBlockLayouts(t *testing.T) {
	layout := renderSample(t, KindCashPartialCut, `{"blocks":[
		{"id":"caja","type":"field","options":{"items":[{"key":"register","label":"Caja:"},{"key":"business_date","label":"Turno del"}]}},
		{"id":"corte","type":"field","options":{"items":[{"key":"cut_at","label":"Corte"}],"layout":"pair"}},
		{"id":"anterior","type":"field","options":{"items":[{"key":"previous_cut_at","label":"Corte anterior"}],"layout":"pair"}},
		{"id":"f","type":"fiscal_notice"}]}`, RenderOptions{PaperWidth: 80})
	// Los documentos de caja cortan entre palabras: el renglón de 44 caracteres ocupa dos líneas.
	if layout.Lines[0].Text != "Caja: Caja de muestra · Turno del" || layout.Lines[1].Text != "01/01/2026" || layout.Lines[1].BlockID != "caja" {
		t.Fatalf("Campo en línea incorrecto: %q %q", layout.Lines[0].Text, layout.Lines[1].Text)
	}
	if !strings.HasPrefix(layout.Lines[2].Text, "Corte ") || !strings.HasSuffix(layout.Lines[2].Text, "01/01/2026 17:00") || runeCount(layout.Lines[2].Text) != 42 {
		t.Fatalf("Campo etiqueta-valor incorrecto: %q", layout.Lines[2].Text)
	}
	if len(layout.EmptyBlocks) != 1 || layout.EmptyBlocks[0].BlockID != "anterior" {
		t.Fatalf("El campo sin datos no se omitió: %+v", layout.EmptyBlocks)
	}
}

// Las muestras impresas llevan la marca al inicio y al final, fuera de cualquier bloque.
func TestSampleMarkersAndFictitiousData(t *testing.T) {
	for _, info := range Kinds {
		layout, err := Render(info.Kind, Default(info.Kind), SampleData(info.Kind, Identity{Name: "Restaurante"}), RenderOptions{PaperWidth: 58})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(layout.Text(), "muestra") {
			t.Fatalf("Los datos de muestra de %s no se identifican como muestra:\n%s", info.Kind, layout.Text())
		}
		AddSampleMarkers(layout)
		first, last := layout.Lines[0], layout.Lines[len(layout.Lines)-1]
		if !strings.Contains(first.Text, "MUESTRA") || first.BlockID != "" || !strings.Contains(last.Text, "VÁLIDO ***") || last.BlockID != "" {
			t.Fatalf("Faltan las marcas de muestra en %s: %q … %q", info.Kind, first.Text, last.Text)
		}
		for _, line := range layout.Lines {
			if runeCount(line.Text) > layout.Columns {
				t.Fatalf("Una línea de %s excede el ancho: %q", info.Kind, line.Text)
			}
		}
	}
}

// Codificación ESC/POS exacta de un layout con atributos e imagen.
func TestEncodeESCPosWithAttributes(t *testing.T) {
	layout := &Layout{TextSize: TextSizeNormal, Lines: []Line{
		{Text: "Á", Bold: true, Size: SizeDouble},
		{Image: &Image{RasterID: "r", WidthDots: 9, HeightDots: 1}, Size: SizeNormal},
		{Text: "b\x1b", Size: SizeDoubleHeight},
	}}
	rasters := map[string]*Raster{"r": {WidthDots: 9, HeightDots: 1, Bits: []byte{0xff, 0x80}}}
	got := EncodeESCPos(layout, rasters, true)
	want := []byte{0x1b, 0x40, 0x1b, 0x74, 0x02,
		0x1d, 0x21, 0x11, 0x1b, 0x45, 0x01, 0xb5, 0x1b, 0x45, 0x00, 0x1d, 0x21, 0x00, '\n',
		0x1d, 0x76, 0x30, 0x00, 0x02, 0x00, 0x01, 0x00, 0xff, 0x80,
		0x1d, 0x21, 0x01, 0x1b, 0x45, 0x00, 'b', 0x1b, 0x45, 0x00, 0x1d, 0x21, 0x00,
		'\n', '\n', '\n', 0x1d, 0x56, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("Bytes ESC/POS incorrectos:\n% x\n% x", got, want)
	}
	layout.TextSize = TextSizeLarge
	layout.TrailingNewline = true
	large := EncodeESCPos(layout, nil, false)
	if !bytes.Contains(large, []byte{0x1d, 0x21, 0x11, 0x1b, 0x45, 0x00, 'b'}) || bytes.Contains(large, []byte{0x1d, 0x76}) ||
		!bytes.HasSuffix(large, []byte{0x1d, 0x21, 0x00, '\n', '\n', '\n', '\n'}) {
		t.Fatalf("El texto grande o la imagen sin bits no se codificaron como se espera: % x", large)
	}
	if len(MissingRasters(layout, nil)) != 1 || len(MissingRasters(layout, rasters)) != 0 {
		t.Fatal("MissingRasters no detecta las imágenes sin bits")
	}
	tall := &Raster{WidthDots: 8, HeightDots: 300, Bits: bytes.Repeat([]byte{0xaa}, 300)}
	command := imageCommand(&Image{RasterID: "t"}, map[string]*Raster{"t": tall})
	if bytes.Count(command, []byte{0x1d, 0x76, 0x30, 0x00}) != 3 || len(command) != 3*8+300 {
		t.Fatalf("La imagen alta no se dividió en franjas: %d bytes", len(command))
	}
}

func TestPlainDetection(t *testing.T) {
	layout := &Layout{Lines: []Line{{Text: "a", Size: SizeNormal}, {Text: "b"}}}
	if !layout.Plain() {
		t.Fatal("Un layout sin atributos no se reconoció como plano")
	}
	layout.Lines[1].Bold = true
	if layout.Plain() {
		t.Fatal("La negrita no hace el layout no plano")
	}
	layout.Lines[1].Bold = false
	layout.Lines[1].Size = SizeDoubleHeight
	if layout.Plain() {
		t.Fatal("El doble alto no hace el layout no plano")
	}
}

// Una observación con controles nunca llega a la impresora como comando: el texto se limpia al
// renderizar y otra vez al codificar; los únicos comandos son los del controlador.
func TestUserTextWithControlsIsSanitized(t *testing.T) {
	data := SampleData(KindReceipt, Identity{Name: "Restaurante\x1b@"})
	data.Commercial.Lines[0].Notes = "Sin sal\x1b@\x1dV\x00\x1bp\x00 por favor​"
	layout, err := Render(KindReceipt, Default(KindReceipt), data, RenderOptions{PaperWidth: 80})
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(layout.Text(), "\x1b\x1d\x00​") || !strings.Contains(layout.Text(), "Nota: Sin sal @ V  p  por favor") {
		t.Fatalf("La observación conservó controles:\n%q", layout.Text())
	}
	layout.Lines[0].Bold = true
	encoded := EncodeESCPos(layout, nil, true)
	if bytes.Count(encoded, []byte{0x1d, 0x56}) != 1 || bytes.Count(encoded, []byte{0x1b, 0x40}) != 1 || bytes.Contains(encoded, []byte{0x1b, 0x70}) {
		t.Fatal("Los bytes ESC/POS contienen comandos que no genera el controlador")
	}
}
