package kitchen

import (
	"context"
	"errors"
	"testing"
)

// Revisión 9F (B4): una impresora o cola que no existe en el equipo servidor (cola CUPS desconocida,
// impresora no instalada en Windows) se distingue de una que no responde: su problema es «not_found» y el
// aviso ofrece «Configurar impresora» además de «Reintentar»; sin respuesta sigue siendo «unreachable».
func TestMissingQueueIsNotFound(t *testing.T) {
	// CUPS con formato: lpstat no reconoce la cola.
	job := formattedJob("cups")
	missing := &recordedCommands{t: t, replies: map[string]recordedReply{
		"/usr/bin/lpstat": {err: errors.New("lpstat: Invalid destination name in list")},
	}}
	outcome := outcomeWithCode(HostTransport{commands: missing}.Submit(context.Background(), job))
	if outcome.State != "error" || outcome.Code != ProblemNotFound {
		t.Fatalf("La cola CUPS inexistente no se explicó como impresora no encontrada: %+v", outcome)
	}
	// Cajón por CUPS: igual.
	drawer := PrintJob{ID: "00000000-0000-4000-8000-0000000009b4", DrawerPulse: &DrawerPulse{Pin: 0},
		PrinterSnapshot: Printer{Name: "Caja", Transport: "cups", Destination: "CAJA_INEXISTENTE", PaperWidth: 80, Copies: 1, Port: 9100}}
	if got := submitDrawerPulseCUPS(context.Background(), drawer, missing); got.Code != ProblemNotFound {
		t.Fatalf("El cajón con una cola inexistente no se explicó: %+v", got)
	}
	// Windows: la impresora no está instalada (ERROR_INVALID_PRINTER_NAME en hostWinspool).
	text := windowsTextJob(1)
	text.PrinterSnapshot.Destination = "POS_RENOMBRADA"
	spooler := usbSpooler()
	outcome = outcomeWithCode(HostTransport{spooler: spooler}.Submit(context.Background(), text))
	if outcome.State != "error" || outcome.Code != ProblemNotFound || len(spooler.written) != 0 {
		t.Fatalf("La impresora de Windows no instalada no se explicó como no encontrada: %+v", outcome)
	}
	// Windows: el administrador de impresión no contestó (no se pudieron leer los puertos): no responde.
	unresponsive := usbSpooler()
	unresponsive.portsError = errors.New("RPC_S_SERVER_UNAVAILABLE")
	outcome = outcomeWithCode(HostTransport{spooler: unresponsive}.Submit(context.Background(), windowsTextJob(1)))
	if outcome.State != "error" || outcome.Code != ProblemUnreachable {
		t.Fatalf("Un administrador de impresión sin respuesta no quedó como «no responde»: %+v", outcome)
	}
}
