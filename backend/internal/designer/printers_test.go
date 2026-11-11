package designer

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/kitchen"
)

// Compatibilidad de impresoras de GET documents/printers (revisión de la etapa 9B, hallazgos medio 4
// y bajo 6): una cola CUPS solo recibe formato y logotipo si la impresora está marcada «Esta
// impresora entiende ESC/POS (formato y logotipo)» y su cola admite raw; sin la marca la cola no se
// consulta, y todas las consultas a CUPS de una petición comparten un límite total de tiempo.

// recordingInspector responde con atributos fijos por cola y registra cada consulta. Una cola sin
// atributos «no responde»: espera a que venza el contexto, como un CUPS colgado.
type recordingInspector struct {
	mutex   sync.Mutex
	options map[string]map[string]string
	queried []string
}

func (inspector *recordingInspector) InspectQueue(ctx context.Context, queue string) kitchen.QueueCapability {
	inspector.mutex.Lock()
	inspector.queried = append(inspector.queried, queue)
	options, found := inspector.options[queue]
	inspector.mutex.Unlock()
	if !found {
		<-ctx.Done()
		return kitchen.QueueCapability{Queue: queue, Reason: "No se pudo consultar la cola " + queue + "."}
	}
	return kitchen.EvaluateRawQueue(queue, options)
}

func cupsPrinter(name, queue string, escpos bool) kitchen.Printer {
	return kitchen.Printer{ID: name, Name: name, Transport: "cups", Destination: queue, PaperWidth: 80, AcceptsESCPos: escpos}
}

func TestPrinterCompatibilityNeedsExplicitESCPosOption(t *testing.T) {
	inspector := &recordingInspector{options: map[string]map[string]string{
		"USB": {"device-uri": "usb://Printer/POS-80", "printer-make-and-model": "POS-80", "printer-type": "2150404"},
		"IPP": {"device-uri": "ipp://192.168.1.20/ipp/print", "printer-make-and-model": "Generic PostScript", "printer-type": "2150404"},
	}}
	printers := []kitchen.Printer{
		cupsPrinter("sin-marca", "USB", false),
		cupsPrinter("marcada", "USB", true),
		cupsPrinter("sin-raw", "IPP", true),
		{ID: "red", Name: "red", Transport: "escpos_tcp", Destination: "192.168.1.30", PaperWidth: 58},
		{ID: "agente", Name: "agente", Transport: "agent", Destination: "otra", PaperWidth: 80},
	}
	result := printerCompatibilities(context.Background(), inspector, printers, false)
	expected := map[string][2]string{
		"sin-marca": {"unsupported", compatibilityDisabled},
		"marcada":   {"supported", compatibilityRawQueue},
		"sin-raw":   {"unsupported", compatibilityNotRaw},
		"red":       {"supported", compatibilityNetwork},
		"agente":    {"unsupported", compatibilityAgent},
	}
	for _, item := range result {
		want := expected[item.ID]
		if item.Format.Status != want[0] || item.Format.Code != want[1] || item.Logo != item.Format {
			t.Fatalf("%s: compatibilidad %+v, se esperaba %v", item.ID, item.Format, want)
		}
	}
	if !strings.HasPrefix(result[0].Format.Reason, "Desactivado en la impresora") || !strings.HasPrefix(result[2].Format.Reason, "Cola sin raw") {
		t.Fatalf("Los motivos no distinguen la marca desactivada de la cola sin raw: %q / %q", result[0].Format.Reason, result[2].Format.Reason)
	}
	// La impresora sin la marca no se consulta: solo las dos marcadas.
	if strings.Join(inspector.queried, ",") != "USB,IPP" {
		t.Fatalf("Se consultaron colas de impresoras sin la marca ESC/POS: %v", inspector.queried)
	}
	// Sin logotipo cargado el motivo del logotipo lo dice.
	withoutLogo := printerCompatibilities(context.Background(), inspector, printers[1:2], true)
	if !strings.HasPrefix(withoutLogo[0].Logo.Reason, "No hay logotipo cargado") || withoutLogo[0].Format.Reason == withoutLogo[0].Logo.Reason {
		t.Fatalf("El motivo del logotipo no avisa que no hay logotipo: %+v", withoutLogo[0])
	}
}

// Tres colas que no responden no retienen la petición más que el límite total: las que ya no
// alcanzan a consultarse quedan «Sin confirmar» sin consultarse.
func TestPrinterCompatibilityHasTotalTimeBudget(t *testing.T) {
	previous := printerProbeBudget
	printerProbeBudget = 300 * time.Millisecond
	t.Cleanup(func() { printerProbeBudget = previous })
	inspector := &recordingInspector{options: map[string]map[string]string{}}
	printers := []kitchen.Printer{
		cupsPrinter("a", "COLGADA_A", true),
		cupsPrinter("b", "COLGADA_B", true),
		cupsPrinter("c", "COLGADA_C", true),
	}
	started := time.Now()
	result := printerCompatibilities(context.Background(), inspector, printers, false)
	elapsed := time.Since(started)
	if elapsed > time.Second {
		t.Fatalf("Las consultas a CUPS excedieron el límite total: %v", elapsed)
	}
	if len(inspector.queried) != 1 {
		t.Fatalf("Después de vencer el límite no deben consultarse más colas: %v", inspector.queried)
	}
	if result[0].Format.Status != "unknown" || result[0].Format.Code != compatibilityUnavailable {
		t.Fatalf("La cola que no respondió debe quedar sin confirmar: %+v", result[0].Format)
	}
	for _, item := range result[1:] {
		if item.Format.Status != "unknown" || item.Format.Code != compatibilityTimeout || !strings.Contains(item.Format.Reason, "no respondió a tiempo") {
			t.Fatalf("Una cola sin tiempo para consultarse debe quedar sin confirmar: %+v", item.Format)
		}
	}
	if printerProbeBudget != 300*time.Millisecond || previous != 4*time.Second {
		t.Fatalf("El límite total de CUPS debe ser de 4 s como el del cajón: %v", previous)
	}
	// La ruta no toma locks consultivos mientras consulta CUPS.
	if route := resolve("GET", []string{"printers"}); route == nil || route.lock != lockNone || route.write {
		t.Fatalf("GET documents/printers no debe tomar locks consultivos: %+v", route)
	}
}
