package kitchen

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/documents"
)

// Etapa 9G (fase E): reglas puras del agente de impresión de las terminales. El flujo completo (reclamo con lease,
// resultados y terminales desconectadas) se prueba contra PostgreSQL en operations/stage9g_print_agent_test.go.

func agentPrinter() Printer {
	device := "11111111-2222-4333-8444-555555555555"
	feed := 4
	return Printer{
		ID: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", Name: "USB de la Caja 2", Transport: "agent", Destination: "POS_80",
		Port: 9100, PaperWidth: 80, Copies: 1, Active: true, CutMode: CutFull, CutFeedLines: &feed, AgentDeviceID: &device,
	}
}

func TestAgentPrinterNeedsAQueueOfItsTerminal(t *testing.T) {
	printer := agentPrinter()
	if err := validatePrinter(&printer); err != nil || !printer.AgentPrinter() {
		t.Fatalf("impresora de una terminal válida rechazada: %v", err)
	}
	for name, change := range map[string]func(*Printer){
		"una ruta como cola":    func(p *Printer) { p.Destination = "/tmp/archivo" },
		"argumentos en la cola": func(p *Printer) { p.Destination = "POS_80 -o x" },
		"terminal inválida":     func(p *Printer) { invalidID := "Caja 2"; p.AgentDeviceID = &invalidID },
		"opción de corte CUPS":  func(p *Printer) { p.CutterOption = "CutMedia=Auto" },
		"marca ESC/POS de CUPS": func(p *Printer) { p.AcceptsESCPos = true },
	} {
		candidate := agentPrinter()
		change(&candidate)
		if err := validatePrinter(&candidate); err == nil {
			t.Errorf("%s: debió rechazarse", name)
		}
	}
	// Sin terminal (impresoras anteriores a la 9G y destinos de aceptación) el destino sigue siendo texto libre y la
	// impresora no puede imprimir.
	legacy := agentPrinter()
	legacy.AgentDeviceID = nil
	legacy.Destination = "Agente de aceptación no disponible"
	if err := validatePrinter(&legacy); err != nil || legacy.AgentPrinter() {
		t.Fatalf("impresora de agente anterior a la 9G: %v", err)
	}
	if PrinterProblem(&legacy) != ProblemUnsupported {
		t.Fatal("una impresora de otra terminal sin terminal no sirve para tickets")
	}
	withTerminal := agentPrinter()
	if PrinterProblem(&withTerminal) != "" {
		t.Fatal("la impresora USB de una terminal sirve para tickets (D10)")
	}
	// Solo una impresora «agent» pertenece a una terminal.
	network := agentPrinter()
	network.Transport = "escpos_tcp"
	network.Destination = "192.168.1.80"
	if err := validatePrinter(&network); err != nil || network.AgentDeviceID != nil {
		t.Fatalf("una impresora de red no lleva terminal: %v %v", err, network.AgentDeviceID)
	}
	if !CompatibleDrawerPrinter("agent", withTerminal.AgentDeviceID) || CompatibleDrawerPrinter("agent", nil) {
		t.Fatal("el cajón admite la impresora de una terminal, no la de agente sin terminal")
	}
}

func TestAgentJobBytesAreTheNetworkESCPosBytes(t *testing.T) {
	printer := agentPrinter()
	printer.Copies = 2
	job := PrintJob{ID: "job", PrinterSnapshot: printer, Content: "COMANDA\nPapas con acentos ñ"}
	single := ESCPosBytes(job.Content, printer)
	data := AgentJobBytes(context.Background(), job, nil)
	if !bytes.Equal(data, append(append([]byte{}, single...), single...)) {
		t.Fatal("las copias deben ir repetidas en los mismos bytes del controlador ESC/POS")
	}
	// El pulso del cajón viaja una sola vez, sin texto ni corte.
	job.DrawerPulse = &DrawerPulse{Pin: 1}
	if pulse := AgentJobBytes(context.Background(), job, nil); !bytes.Equal(pulse, DrawerPulseBytes(1)) {
		t.Fatalf("pulso del cajón: %x", pulse)
	}
	// Un documento con formato usa el codificador ESC/POS de la IR, como una impresora de red.
	job.DrawerPulse = nil
	printer.Copies = 1
	job.PrinterSnapshot = printer
	job.Layout = &documents.Layout{PaperWidth: 80, Lines: []documents.Line{{Text: "TOTAL", Bold: true}}}
	formatted := AgentJobBytes(context.Background(), job, nil)
	if !bytes.Equal(formatted, documents.EncodeESCPos(job.Layout, map[string]*documents.Raster{}, printer.PaperCut())) {
		t.Fatal("el documento con formato debe llevar los bytes de documents.EncodeESCPos")
	}
}

func TestAgentOutcomeFollowsTheQueueRules(t *testing.T) {
	printer := agentPrinter()
	job := &PrintJob{ID: "job", PrinterSnapshot: printer}
	submitted, err := agentOutcome("submitted", "", "POS_80-17", job, "Caja 2")
	if err != nil || submitted.State != "submitted" || !submitted.Accepted || submitted.PhysicalConfirmed ||
		!strings.Contains(submitted.Message, "no confirma salida física de papel") || !strings.Contains(submitted.Message, "Caja 2") {
		t.Fatalf("aceptado por la cola: %+v %v", submitted, err)
	}
	job.DrawerPulse = &DrawerPulse{}
	drawer, err := agentOutcome("submitted", "", "POS_80-18", job, "Caja 2")
	if err != nil || !strings.Contains(drawer.Message, "no confirma que el cajón se abrió") {
		t.Fatalf("pulso aceptado: %+v %v", drawer, err)
	}
	failed, err := agentOutcome("error", ProblemNotFound, "", job, "Caja 2")
	if err != nil || failed.State != "error" || failed.Code != ProblemNotFound {
		t.Fatalf("error confirmado: %+v %v", failed, err)
	}
	uncertain, err := agentOutcome("uncertain", "", "", job, "Caja 2")
	if err != nil || uncertain.State != "uncertain" || uncertain.Code != ProblemUncertain {
		t.Fatalf("incierto: %+v %v", uncertain, err)
	}
	for name, values := range map[string][3]string{
		"aceptado sin referencia":         {"submitted", "", ""},
		"referencia de otra cola":         {"submitted", "", "OTRA-3"},
		"error con una clave desconocida": {"error", "papel", ""},
		"error con referencia":            {"error", ProblemUnreachable, "POS_80-4"},
		"incierto con clave":              {"uncertain", ProblemSetup, ""},
		"estado desconocido":              {"printed", "", ""},
		"intervención desde el agente":    {"intervention", "", ""},
	} {
		if _, err := agentOutcome(values[0], values[1], values[2], job, "Caja 2"); err == nil {
			t.Errorf("%s: debió rechazarse", name)
		}
	}
}

func TestTerminalQueuesUseTheRawRule(t *testing.T) {
	remote := int64(cupsPrinterRemote)
	class := int64(cupsPrinterClass)
	local := int64(0x801c)
	cases := []struct {
		name       string
		connection string
		model      string
		kind       *int64
		usable     bool
	}{
		{"USB con controlador", "usb", "POS-80", &local, true},
		{"AppSocket", "socket", "Generic", nil, true},
		{"red de documentos", "ipp", "Generic PostScript Printer", nil, false},
		{"AirPrint", "dnssd", "HP Smart Tank-AirPrint", nil, false},
		{"compartida", "usb", "POS-80", &remote, false},
		{"grupo", "usb", "POS-80", &class, false},
		{"sin conexión informada", "", "POS-80", nil, false},
	}
	for _, item := range cases {
		queue := EvaluateTerminalQueue("POS_80", item.connection, item.model, item.kind)
		if queue.Usable != item.usable || (!item.usable && queue.Reason == "") {
			t.Errorf("%s: %+v", item.name, queue)
		}
		for _, technical := range []string{"CUPS", "raw", "lpstat"} {
			if strings.Contains(queue.Reason, technical) {
				t.Errorf("%s: motivo con términos técnicos: %s", item.name, queue.Reason)
			}
		}
	}
	// La regla es la misma que la del cajón por CUPS del equipo servidor.
	if !EvaluateRawQueue("POS_80", map[string]string{"device-uri": "usb://Printer/POS-80"}).Raw ||
		EvaluateRawQueue("HP", map[string]string{"device-uri": "ipp://hp.local/ipp/print"}).Raw {
		t.Fatal("la regla del cajón por CUPS cambió")
	}
}

func TestJobProblemKnowsDisconnectedTerminals(t *testing.T) {
	sql := JobProblemSQL("j")
	for _, expected := range []string{"'agent_offline'", "print_agent_seen_at", "revoked_at IS NULL", "j.attempts<3"} {
		if !strings.Contains(sql, expected) {
			t.Errorf("JobProblemSQL no considera %s", expected)
		}
	}
	if !strings.Contains(JobTerminalSQL("j"), "agent_device_id") {
		t.Fatal("JobTerminalSQL debe leer la terminal de la fotografía")
	}
}
