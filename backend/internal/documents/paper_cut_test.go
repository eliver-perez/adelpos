package documents

import (
	"bytes"
	"testing"
)

// legacyFullCut es el corte de antes de la etapa 9F: total, después de los tres saltos de siempre.
var legacyFullCut = PaperCut{Enabled: true, FeedLines: LegacyCutFeedLines}

// Final de un documento ESC/POS con formato (etapa 9F, §4.4): sin corte, los tres saltos de siempre;
// con corte, el avance de la impresora y GS V 0 (total) o GS V 1 (parcial).
func TestPaperCutEnding(t *testing.T) {
	cases := []struct {
		cut  PaperCut
		want []byte
	}{
		{NoPaperCut, []byte{'\n', '\n', '\n'}},
		{PaperCut{Partial: true, FeedLines: 8}, []byte{'\n', '\n', '\n'}},
		{legacyFullCut, []byte{'\n', '\n', '\n', 0x1d, 0x56, 0x00}},
		{PaperCut{Enabled: true, FeedLines: 0}, []byte{0x1d, 0x56, 0x00}},
		{PaperCut{Enabled: true, FeedLines: 4}, []byte{'\n', '\n', '\n', '\n', 0x1d, 0x56, 0x00}},
		{PaperCut{Enabled: true, Partial: true, FeedLines: 0}, []byte{0x1d, 0x56, 0x01}},
		{PaperCut{Enabled: true, Partial: true, FeedLines: 4}, []byte{'\n', '\n', '\n', '\n', 0x1d, 0x56, 0x01}},
		{PaperCut{Enabled: true, Partial: true, FeedLines: 8}, append(bytes.Repeat([]byte{'\n'}, 8), 0x1d, 0x56, 0x01)},
		// Fuera de la lista cerrada (nunca llega de Go) se limita a 0–8.
		{PaperCut{Enabled: true, FeedLines: 20}, append(bytes.Repeat([]byte{'\n'}, 8), 0x1d, 0x56, 0x00)},
		{PaperCut{Enabled: true, FeedLines: -2}, []byte{0x1d, 0x56, 0x00}},
	}
	for _, testCase := range cases {
		if got := testCase.cut.Ending(); !bytes.Equal(got, testCase.want) {
			t.Fatalf("%+v: % x, se esperaba % x", testCase.cut, got, testCase.want)
		}
	}
	layout := &Layout{Lines: []Line{{Text: "TICKET", Bold: true, Size: SizeDouble}, {Text: "Total", Size: SizeNormal}}}
	plain := EncodeESCPos(layout, nil, NoPaperCut)
	partial := EncodeESCPos(layout, nil, PaperCut{Enabled: true, Partial: true, FeedLines: 4})
	if !bytes.Equal(partial, append(append([]byte{}, plain[:len(plain)-3]...), '\n', '\n', '\n', '\n', 0x1d, 0x56, 0x01)) {
		t.Fatalf("El layout con corte parcial no termina con su avance: % x", partial)
	}
}
