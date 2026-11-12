package payments

import (
	"context"
	"testing"

	"local/ad-el-pos/server/internal/kitchen"
)

// Compatibilidad de la impresora del cajón con la cola de impresión del equipo (etapa 9E): con un
// inspector de CUPS los textos no cambian; con uno de Windows, la impresora desactivada no habla de
// CUPS. Ninguna impresora desactivada se consulta.

// queueInspector responde con una capacidad fija y cuenta las consultas; system declara su cola
// (vacía = no la declara, como los inspectores simulados de CUPS).
type queueInspector struct {
	system   string
	queried  int
	response kitchen.QueueCapability
}

func (inspector *queueInspector) InspectQueue(_ context.Context, queue string) kitchen.QueueCapability {
	inspector.queried++
	response := inspector.response
	response.Queue = queue
	return response
}

type windowsQueueInspector struct {
	queueInspector
}

func (*windowsQueueInspector) SystemQueue() string {
	return kitchen.SystemQueueWindows
}

func TestDrawerPrinterCompatibilityBySystemQueue(t *testing.T) {
	cups := &queueInspector{}
	inactive := DrawerPrinter{Name: "Caja", Transport: "cups", Active: false}
	drawerPrinterCompatibility(context.Background(), cups, &inactive, "Printer_POS_80")
	if inactive.Compatible || inactive.Reason != "Impresora desactivada; su cola CUPS no se consultó." || cups.queried != 0 {
		t.Fatalf("Con CUPS la impresora desactivada conserva su motivo y no se consulta: %+v", inactive)
	}
	windows := &windowsQueueInspector{queueInspector{response: kitchen.QueueCapability{Checked: true, Raw: true, Connection: "usb"}}}
	inactive = DrawerPrinter{Name: "Caja", Transport: "cups", Active: false}
	drawerPrinterCompatibility(context.Background(), windows, &inactive, "POS-80C")
	if inactive.Compatible || inactive.Reason != "Impresora desactivada; no se consultó en Windows." || windows.queried != 0 {
		t.Fatalf("En Windows la impresora desactivada no habla de CUPS ni se consulta: %+v", inactive)
	}
	active := DrawerPrinter{Name: "Caja", Transport: "cups", Active: true}
	drawerPrinterCompatibility(context.Background(), windows, &active, "POS-80C")
	if !active.Compatible || active.Connection != "usb" || windows.queried != 1 {
		t.Fatalf("La impresora de Windows que entrega los datos sin alterar sirve para el cajón: %+v", active)
	}
}
