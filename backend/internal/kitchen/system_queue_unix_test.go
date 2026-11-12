//go:build !windows

package kitchen

import (
	"context"
	"testing"
)

// Fuera de Windows la cola de impresión del equipo sigue siendo CUPS sin ningún cambio: el
// transporte y el inspector sin sustitutos usan CUPS y el destino se valida como cola CUPS.
func TestSystemQueueIsCUPSOutsideWindows(t *testing.T) {
	if _, windows := (HostTransport{}).windowsSpooler(); windows {
		t.Fatal("Fuera de Windows el transporte sin sustitutos debe usar CUPS")
	}
	if (HostQueueInspector{}).SystemQueue() != SystemQueueCUPS {
		t.Fatal("Fuera de Windows el inspector sin sustitutos consulta CUPS")
	}
	injection := Printer{Name: "Inyección", Transport: "cups", Destination: "cola; touch /tmp/inyección", PaperWidth: 80, Copies: 1, Port: 9100}
	if err := validatePrinter(&injection); err == nil {
		t.Fatal("Se aceptó un destino de ejecución arbitraria")
	}
	spaced := Printer{Name: "Tickets", Transport: "cups", Destination: "EPSON TM-T20II Receipt", PaperWidth: 80, Copies: 1, Port: 9100}
	if err := validatePrinter(&spaced); err == nil {
		t.Fatal("Fuera de Windows una cola CUPS no admite espacios")
	}
	// El nombre inválido nunca llega a lpstat (el ejecutor simulado falla si se le llama).
	capability := HostQueueInspector{commands: &recordedCommands{t: t, forbidden: true}}.InspectQueue(context.Background(), "cola; rm")
	if capability.Checked {
		t.Fatalf("Se consultó una cola con nombre inválido: %+v", capability)
	}
}
