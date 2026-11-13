package documents

import (
	"bytes"
	"testing"
)

// legacyFullCut es el corte de las impresoras y fotografías anteriores a la etapa 9F: total, después de los
// tres saltos de siempre, sin cerrar aparte el último renglón.
var legacyFullCut = PaperCut{Enabled: true, Legacy: true}

// Final de un documento ESC/POS (etapa 9F, §4.4; revisión 9F, M1): sin corte, los tres saltos de siempre;
// con corte, el salto que cierra el último renglón si quedó abierto, los renglones de avance y GS V 0
// (total) o GS V 1 (parcial); con el final anterior a la 9F, los tres saltos y el corte.
func TestPaperCutEnding(t *testing.T) {
	cases := []struct {
		cut      PaperCut
		lineOpen bool
		want     []byte
	}{
		{NoPaperCut, false, []byte{'\n', '\n', '\n'}},
		{NoPaperCut, true, []byte{'\n', '\n', '\n'}},
		{PaperCut{Partial: true, FeedLines: 8}, true, []byte{'\n', '\n', '\n'}},
		{legacyFullCut, false, []byte{'\n', '\n', '\n', 0x1d, 0x56, 0x00}},
		{legacyFullCut, true, []byte{'\n', '\n', '\n', 0x1d, 0x56, 0x00}},
		{PaperCut{Enabled: true, Legacy: true, Partial: true}, true, []byte{'\n', '\n', '\n', 0x1d, 0x56, 0x01}},
		// El último renglón abierto se cierra también con 0 renglones de avance: GS V llega al inicio de un renglón.
		{PaperCut{Enabled: true, FeedLines: 0}, false, []byte{0x1d, 0x56, 0x00}},
		{PaperCut{Enabled: true, FeedLines: 0}, true, []byte{'\n', 0x1d, 0x56, 0x00}},
		{PaperCut{Enabled: true, FeedLines: 4}, false, []byte{'\n', '\n', '\n', '\n', 0x1d, 0x56, 0x00}},
		{PaperCut{Enabled: true, FeedLines: 4}, true, []byte{'\n', '\n', '\n', '\n', '\n', 0x1d, 0x56, 0x00}},
		{PaperCut{Enabled: true, Partial: true, FeedLines: 0}, true, []byte{'\n', 0x1d, 0x56, 0x01}},
		{PaperCut{Enabled: true, Partial: true, FeedLines: 4}, false, []byte{'\n', '\n', '\n', '\n', 0x1d, 0x56, 0x01}},
		{PaperCut{Enabled: true, Partial: true, FeedLines: 8}, false, append(bytes.Repeat([]byte{'\n'}, 8), 0x1d, 0x56, 0x01)},
		{PaperCut{Enabled: true, Partial: true, FeedLines: 8}, true, append(bytes.Repeat([]byte{'\n'}, 9), 0x1d, 0x56, 0x01)},
		// Fuera de la lista cerrada (nunca llega de Go) se limita a 0–8.
		{PaperCut{Enabled: true, FeedLines: 20}, false, append(bytes.Repeat([]byte{'\n'}, 8), 0x1d, 0x56, 0x00)},
		{PaperCut{Enabled: true, FeedLines: -2}, true, []byte{'\n', 0x1d, 0x56, 0x00}},
	}
	for _, testCase := range cases {
		if got := testCase.cut.Ending(testCase.lineOpen); !bytes.Equal(got, testCase.want) {
			t.Fatalf("%+v (renglón abierto %v): % x, se esperaba % x", testCase.cut, testCase.lineOpen, got, testCase.want)
		}
	}
	if TextLineOpen("") || TextLineOpen("Total\n") || !TextLineOpen("Total") || !TextLineOpen("A\nB") {
		t.Fatal("TextLineOpen no reconoce el renglón abierto")
	}
}

// Bytes exactos del final de cada tipo de documento con las plantillas predeterminadas (revisión 9F, M1):
// los renglones de avance son los mismos en todos (los renglones en blanco entre el último renglón impreso
// y el corte), también con 0; sin corte y con el final anterior a la 9F, los bytes de la etapa 9E.
func TestPaperCutEndsEveryDocumentKind(t *testing.T) {
	for _, info := range Kinds {
		layout, err := Render(info.Kind, Default(info.Kind), SampleData(info.Kind, Identity{Name: "Restaurante"}), RenderOptions{PaperWidth: 80})
		if err != nil {
			t.Fatalf("%s: %v", info.Kind, err)
		}
		// Comandas y avisos terminan con salto; precuentas, tickets y documentos de caja, sin él.
		if layout.TrailingNewline != (info.Family == FamilyKitchen) {
			t.Fatalf("%s: cambió el final del documento (TrailingNewline=%v)", info.Kind, layout.TrailingNewline)
		}
		withoutCut := EncodeESCPos(layout, nil, NoPaperCut)
		if !bytes.HasSuffix(withoutCut, []byte{'\n', '\n', '\n'}) {
			t.Fatalf("%s: sin corte no termina con los tres saltos de siempre: % x", info.Kind, withoutCut)
		}
		body := withoutCut[:len(withoutCut)-3]
		// Lo impreso hasta el último renglón, cerrado: el texto del documento termina igual en todos.
		closed := append([]byte{}, body...)
		if !layout.TrailingNewline {
			closed = append(closed, '\n')
		}
		if !bytes.HasSuffix(closed, []byte{0x1b, 0x45, 0x00, 0x1d, 0x21, 0x00, '\n'}) {
			t.Fatalf("%s: el último renglón no se cierra con su salto: % x", info.Kind, closed[max(0, len(closed)-12):])
		}
		for _, feed := range []int{0, 1, 4, 8} {
			for _, partial := range []bool{false, true} {
				mode := byte(0x00)
				if partial {
					mode = 0x01
				}
				got := EncodeESCPos(layout, nil, PaperCut{Enabled: true, Partial: partial, FeedLines: feed})
				want := append(append(append([]byte{}, closed...), bytes.Repeat([]byte{'\n'}, feed)...), 0x1d, 0x56, mode)
				if !bytes.Equal(got, want) {
					t.Fatalf("%s con corte (parcial %v) y %d renglones: termina en % x, se esperaba % x", info.Kind, partial, feed,
						got[max(0, len(got)-16):], want[max(0, len(want)-16):])
				}
			}
		}
		// Una impresora que ya cortaba antes de la 9F conserva el final de la 9E: los tres saltos y GS V 0.
		legacy := EncodeESCPos(layout, nil, legacyFullCut)
		if !bytes.Equal(legacy, append(append([]byte{}, withoutCut...), 0x1d, 0x56, 0x00)) {
			t.Fatalf("%s: el corte anterior a la 9F cambió los bytes", info.Kind)
		}
	}
}

// Caso de la revisión: con corte y 0 renglones, la leyenda fiscal del ticket se imprime (su salto llega
// antes de GS V) en lugar de quedar en el búfer de la impresora.
func TestZeroFeedPrintsTheLastLineOfTheReceipt(t *testing.T) {
	layout, err := Render(KindReceipt, Default(KindReceipt), SampleData(KindReceipt, Identity{Name: "Restaurante"}), RenderOptions{PaperWidth: 80})
	if err != nil {
		t.Fatal(err)
	}
	encoded := EncodeESCPos(layout, nil, PaperCut{Enabled: true, FeedLines: 0})
	if !bytes.HasSuffix(encoded, []byte("(CFDI).\x1b\x45\x00\x1d\x21\x00\n\x1d\x56\x00")) {
		t.Fatalf("La leyenda fiscal no se cierra antes del corte: % x", encoded[max(0, len(encoded)-24):])
	}
}

// Un documento cuya última línea es una imagen no deja renglón abierto: el corte sigue a su avance.
func TestPaperCutAfterTrailingImage(t *testing.T) {
	layout := &Layout{Lines: []Line{{Text: "TICKET", Size: SizeDouble}, {Image: &Image{RasterID: "r"}}}}
	rasters := map[string]*Raster{"r": {WidthDots: 8, HeightDots: 1, Bits: []byte{0xff}}}
	encoded := EncodeESCPos(layout, rasters, PaperCut{Enabled: true, FeedLines: 2})
	if !bytes.HasSuffix(encoded, []byte{0x01, 0x00, 0xff, '\n', '\n', 0x1d, 0x56, 0x00}) {
		t.Fatalf("El corte después de una imagen agregó un salto de más: % x", encoded)
	}
}
