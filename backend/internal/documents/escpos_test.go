package documents

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Pruebas de la posición de las imágenes en los bytes ESC/POS (revisión de la etapa 9B, hallazgo
// medio 1): GS v 0 se ejecuta al inicio de una línea, con el búfer de impresión vacío. Una línea de
// texto que quedó sin salto (la última de un ticket, precuenta o documento de caja) recibe el salto
// antes de la imagen que la sigue.

var rasterCommand = []byte{0x1d, 0x76, 0x30, 0x00}

// Plantilla predeterminada del ticket con el logotipo al final, después del aviso fiscal.
func TestLogoAfterLastTextLineStartsOnEmptyLine(t *testing.T) {
	template := Default(KindReceipt)
	template.Blocks = append(template.Blocks, newBlock("logo-final", BlockLogo, &EmptyOptions{}))
	raw, err := json.Marshal(template)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseTemplate(KindReceipt, raw)
	if err != nil {
		t.Fatalf("La plantilla con el logotipo al final se rechazó: %v", err)
	}
	identity := Identity{Name: "Restaurante", Logo: []LogoRaster{{PaperWidth: 80, RasterID: "r80", WidthDots: 8, HeightDots: 2}}}
	layout, err := Render(KindReceipt, parsed, SampleData(KindReceipt, identity), RenderOptions{PaperWidth: 80})
	if err != nil {
		t.Fatal(err)
	}
	if layout.Lines[len(layout.Lines)-1].Image == nil || layout.TrailingNewline {
		t.Fatal("La última línea del ticket debía ser el logotipo, sin salto final del documento")
	}
	rasters := map[string]*Raster{"r80": {WidthDots: 8, HeightDots: 2, Bits: []byte{0xff, 0x81}}}
	encoded := EncodeESCPos(layout, rasters, legacyFullCut)
	index := bytes.Index(encoded, rasterCommand)
	if index < 0 {
		t.Fatal("Los bytes no contienen GS v 0")
	}
	// El aviso fiscal termina con el reinicio de atributos y su salto; la imagen empieza después.
	if before := encoded[:index]; !bytes.HasSuffix(before, []byte("CFDI).\x1bE\x00\x1d!\x00\n")) {
		t.Fatalf("GS v 0 llega sin salto de línea previo: % x", before[max(0, len(before)-12):])
	}
	image := []byte{0x1d, 0x76, 0x30, 0x00, 0x01, 0x00, 0x02, 0x00, 0xff, 0x81}
	if !bytes.HasSuffix(encoded, append(image, '\n', '\n', '\n', 0x1d, 0x56, 0x00)) {
		t.Fatalf("Tras el logotipo deben seguir los tres saltos y el corte: % x", encoded[index:])
	}
	if bytes.Count(encoded, []byte("CFDI).\x1bE\x00\x1d!\x00\n\n")) != 0 {
		t.Fatal("El aviso fiscal recibió dos saltos")
	}
}

// Secuencia exacta: una imagen inicial no lleva salto previo (el búfer está vacío tras ESC @), una
// imagen después de una línea intermedia no duplica el salto, y la imagen que sigue a la última
// línea de texto recibe el suyo. Con el salto final del documento (comandas) tampoco se duplica.
func TestImageLinesAlwaysStartOnEmptyLine(t *testing.T) {
	image := &Image{RasterID: "r", WidthDots: 8, HeightDots: 1}
	layout := &Layout{TextSize: TextSizeNormal, Lines: []Line{
		{Image: image},
		{Text: "a", Size: SizeNormal},
		{Image: image},
		{Text: "b", Size: SizeNormal},
		{Image: image},
	}}
	rasters := map[string]*Raster{"r": {WidthDots: 8, HeightDots: 1, Bits: []byte{0xf0}}}
	command := []byte{0x1d, 0x76, 0x30, 0x00, 0x01, 0x00, 0x01, 0x00, 0xf0}
	text := func(value byte) []byte {
		return []byte{0x1d, 0x21, 0x00, 0x1b, 0x45, 0x00, value, 0x1b, 0x45, 0x00, 0x1d, 0x21, 0x00}
	}
	want := []byte{0x1b, 0x40, 0x1b, 0x74, 0x02}
	want = append(want, command...)
	want = append(append(want, text('a')...), '\n')
	want = append(want, command...)
	want = append(append(want, text('b')...), '\n')
	want = append(want, command...)
	want = append(want, '\n', '\n', '\n')
	if got := EncodeESCPos(layout, rasters, NoPaperCut); !bytes.Equal(got, want) {
		t.Fatalf("Bytes ESC/POS incorrectos:\n% x\n% x", got, want)
	}
	layout.TrailingNewline = true
	if got := EncodeESCPos(layout, rasters, NoPaperCut); !bytes.Equal(got, want) {
		t.Fatalf("Con el salto final del documento, la imagen no debe duplicar el salto:\n% x\n% x", got, want)
	}
	// Sin imagen al final, la última línea conserva la regla anterior (sin salto propio).
	layout.TrailingNewline = false
	layout.Lines = layout.Lines[:4]
	plainEnd := EncodeESCPos(layout, rasters, NoPaperCut)
	if !bytes.HasSuffix(plainEnd, append(text('b'), '\n', '\n', '\n')) {
		t.Fatalf("La última línea de texto cambió: % x", plainEnd)
	}
}
