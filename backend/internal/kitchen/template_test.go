package kitchen

import (
	"bytes"
	"context"
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

// Con la opción de mostrar importes, la comanda usa el formato único de importes (etapa 7.1).
func TestThermalDocumentShowsPriceWithThousands(t *testing.T) {
	document := Document{ID: "document-price", Kind: "normal", CreatedAt: time.Date(2026, 11, 7, 19, 0, 0, 0, time.UTC),
		Settings: Settings{ShowPrices: true, QuantityStyle: "prefix"},
		Context:  TicketContext{Timezone: "America/Monterrey", Folio: 7, Table: "Mesa 2"},
		Sections: []Section{{Station: Station{Name: "Barra"}, Items: []Instructions{{Name: "Botella", Quantity: 1,
			Price: "1250.00", Parts: []Part{}, ItemID: "partida-precio"}}}}}
	text := RenderText(document, 80)
	if !strings.Contains(text, "Importe: $1,250.00") || strings.Contains(text, "$1250.00") {
		t.Fatalf("La comanda no usa el formato de importes con miles:\n%s", text)
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

// El pulso del cajón es exactamente ESC p m t1 t2: sin reinicio (ESC @), sin texto ni corte, y
// solo el transporte ESC/POS directo lo entrega sin alterar.
func TestDrawerPulseBytesAndCompatibleTransport(t *testing.T) {
	if !bytes.Equal(DrawerPulseBytes(0), []byte{0x1b, 0x70, 0x00, 0x19, 0xfa}) {
		t.Fatalf("Pulso del pin 2 incorrecto: % x", DrawerPulseBytes(0))
	}
	if !bytes.Equal(DrawerPulseBytes(1), []byte{0x1b, 0x70, 0x01, 0x19, 0xfa}) {
		t.Fatalf("Pulso del pin 5 incorrecto: % x", DrawerPulseBytes(1))
	}
	// Un pin fuera de la lista cerrada nunca produce otro comando.
	if !bytes.Equal(DrawerPulseBytes(7), DrawerPulseBytes(0)) {
		t.Fatal("Un pin desconocido generó un comando distinto del pin 2")
	}
	if !CompatibleDrawerTransport("escpos_tcp") || !CompatibleDrawerTransport("cups") || CompatibleDrawerTransport("agent") {
		t.Fatal("La compatibilidad del cajón no se limita a ESC/POS por red y CUPS")
	}
	// Con un transporte incompatible el comando queda en intervención sin abrir conexiones ni
	// ejecutar programas (el ejecutor simulado falla la prueba si se le llama).
	commands := &recordedCommands{t: t, forbidden: true}
	outcome := HostTransport{commands: commands}.Submit(context.Background(), PrintJob{
		DrawerPulse:     &DrawerPulse{Pin: 0},
		PrinterSnapshot: Printer{Name: "Agente", Transport: "agent", Destination: "Agente futuro", PaperWidth: 80, Copies: 1, Port: 9100},
	})
	if outcome.State != "intervention" || outcome.Accepted || !strings.Contains(outcome.Message, "pulso") {
		t.Fatalf("Un pulso por el agente no quedó en intervención: %+v", outcome)
	}
}
