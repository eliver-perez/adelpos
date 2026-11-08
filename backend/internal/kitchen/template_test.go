package kitchen

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
)

func TestThermalDocumentPreservesPreparationAndExcludesCommands(t *testing.T) {
	document := Document{ID: "document-test", Kind: "normal", CreatedAt: time.Date(2026, 11, 7, 19, 0, 0, 0, time.UTC),
		Settings: Settings{ShowNotes: true, ShowEstablishment: true, LineSpacing: 1, QuantityStyle: "prefix"},
		Context:  TicketContext{Establishment: "ADEL POS", Timezone: "America/Monterrey", Folio: 12, Table: "Mesa 1"},
		Sections: []Section{{Station: Station{Name: "Cocina caliente"}, Items: []Instructions{{Name: "Hamburguesa", Quantity: 2,
			Price: "100.00", Notes: "Sin cebolla\x1b\x40\x1d\x56\x00; café, ñ y á", Parts: []Part{}, ItemID: "partida-estable"}}}}}
	text := RenderText(document, 80)
	for _, expected := range []string{"2 x Hamburguesa", "café, ñ y á", "Cocina caliente", "partida-estable"} {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(expected)) {
			t.Fatalf("Falta preparación: %s", expected)
		}
	}
	if strings.Contains(text, "100.00") || strings.Contains(text, "ADICIÓN") || strings.Contains(text, "REIMPRESIÓN") || strings.ContainsAny(text, "\x1b\x1d\x00") {
		t.Fatal("El documento filtró importes, etiquetas o controles no autorizados")
	}
	document.Kind = "modification"
	if !strings.Contains(RenderText(document, 58), "MODIFICACIÓN") {
		t.Fatal("Falta aviso de modificación")
	}
	document.Kind = "cancellation"
	if !strings.Contains(RenderText(document, 58), "CANCELACIÓN") {
		t.Fatal("Falta aviso de cancelación")
	}
}

func TestESCPosEncodingAndTrustedCut(t *testing.T) {
	plain := ESCPosBytes("Café ñ á\n", Printer{})
	if bytes.Contains(plain, []byte{0x1d, 0x56}) {
		t.Fatal("No debe cortar sin capacidad explícita")
	}
	decoded, err := charmap.CodePage850.NewDecoder().Bytes(plain[5:])
	if err != nil || !strings.Contains(string(decoded), "Café ñ á") {
		t.Fatal("Se perdieron los acentos de CP850")
	}
	cut := ESCPosBytes("Prueba", Printer{Cut: true, SupportsCut: true})
	if !bytes.HasSuffix(cut, []byte{0x1d, 0x56, 0x00}) {
		t.Fatal("El controlador no generó el corte autorizado")
	}
	if err := validatePrinter(&Printer{Name: "Inyección", Transport: "cups", Destination: "cola; touch /tmp/inyección", PaperWidth: 80, Copies: 1, Port: 9100}); err == nil {
		t.Fatal("Se aceptó un destino de ejecución arbitraria")
	}
	if !queuePattern.MatchString("_192_168_1_80") {
		t.Fatal("Se rechazó una cola local válida de CUPS")
	}
}
