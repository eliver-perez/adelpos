//go:build windows

package kitchen

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Pruebas del administrador de impresión real de Windows, SOLO con lecturas: enumeran impresoras y
// puertos y consultan una impresora, pero nunca abren un documento ni envían bytes. Las pruebas del
// envío usan el administrador simulado (windows_queue_test.go).

func TestHostWinspoolReadsInstalledPrintersWithoutPrinting(t *testing.T) {
	spooler := hostWinspool{}
	printers, err := spooler.listPrinters()
	if err != nil {
		t.Fatalf("No se pudieron enumerar las impresoras de Windows: %v", err)
	}
	ports, err := spooler.ports()
	if err != nil {
		t.Fatalf("No se pudieron enumerar los puertos de Windows: %v", err)
	}
	if len(ports) == 0 {
		t.Fatal("Windows siempre informa al menos los puertos locales (FILE:, PORTPROMPT:)")
	}
	for _, port := range ports {
		t.Logf("Puerto %q · monitor %q", port.Name, port.Monitor)
	}
	for _, info := range printers {
		capability := evaluateWindowsPrinter(info.Name, info, ports)
		t.Logf("Impresora %q · puerto %q · controlador %q · atributos %#x · estado %#x · raw %v · %s",
			info.Name, info.Port, info.Driver, info.Attributes, info.Status, capability.Raw, capability.Reason)
		if info.Name == "" {
			t.Fatal("Windows informó una impresora sin nombre")
		}
		if validateWindowsPrinterName(info.Name) != nil {
			continue
		}
		// La lectura de una sola impresora coincide con la enumeración.
		single, err := spooler.printer(info.Name)
		if err != nil {
			t.Fatalf("No se pudo leer la impresora %q: %v", info.Name, err)
		}
		if single.Name != info.Name || single.Port != info.Port || single.Driver != info.Driver {
			t.Fatalf("La impresora %q leída sola no coincide con la enumeración: %+v / %+v", info.Name, single, info)
		}
		inspected := (HostQueueInspector{}).InspectQueue(context.Background(), info.Name)
		if !inspected.Checked || inspected.Raw != capability.Raw {
			t.Fatalf("La consulta de %q no coincide con su clasificación: %+v / %+v", info.Name, inspected, capability)
		}
	}
	discovered, err := DiscoverPrinters(context.Background())
	if err != nil {
		t.Fatalf("La lista de impresoras para configurar falló: %v", err)
	}
	for _, item := range discovered {
		t.Logf("Lista para configurar: %q · %s", item.Queue, item.Status)
		if item.Options != "" || item.Status == "" || !strings.Contains(item.Status, "Puerto") {
			t.Fatalf("Resumen incompleto de %q: %+v", item.Queue, item)
		}
	}
}

func TestHostWinspoolUnknownPrinterIsNotChecked(t *testing.T) {
	capability := (HostQueueInspector{}).InspectQueue(context.Background(), "ADEL POS impresora que no existe 9E")
	if capability.Checked || capability.Raw || !strings.Contains(capability.Reason, "no está instalada") {
		t.Fatalf("Una impresora inexistente debe quedar sin consultar: %+v", capability)
	}
	if _, err := (hostWinspool{}).printer("ADEL POS impresora que no existe 9E"); err == nil {
		t.Fatal("Windows abrió una impresora que no existe")
	}
}

// En Windows el transporte «cups» valida el nombre de una impresora instalada en Windows (con
// espacios y acentos), no el de una cola CUPS.
func TestWindowsValidatesInstalledPrinterNames(t *testing.T) {
	accepted := []string{"EPSON TM-T20II Receipt", "Impresora de cocina ñandú", "POS-80C", "Printer_POS_80"}
	for _, name := range accepted {
		printer := Printer{Name: "Tickets", Transport: "cups", Destination: name, PaperWidth: 80, Copies: 1}
		if err := validatePrinter(&printer); err != nil {
			t.Fatalf("Se rechazó la impresora de Windows %q: %v", name, err)
		}
	}
	rejected := []string{`\\servidor\Tickets`, "Tickets,Job 5", `Tickets "USB"`, "Tickets\x00", "Tickets\nUSB"}
	for _, name := range rejected {
		printer := Printer{Name: "Tickets", Transport: "cups", Destination: name, PaperWidth: 80, Copies: 1}
		var problem *Error
		if err := validatePrinter(&printer); !errors.As(err, &problem) || !strings.Contains(problem.Message, "impresora instalada en Windows") {
			t.Fatalf("Se aceptó el destino %q: %v", name, err)
		}
	}
	if spooler, windows := (HostTransport{}).windowsSpooler(); !windows || spooler == nil {
		t.Fatal("En Windows el transporte sin sustitutos debe usar la cola de Windows")
	}
	if (HostQueueInspector{}).SystemQueue() != SystemQueueWindows {
		t.Fatal("En Windows el inspector sin sustitutos consulta la cola de Windows")
	}
}
