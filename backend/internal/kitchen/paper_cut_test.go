package kitchen

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"slices"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/documents"
)

// Corte de papel por impresora (etapa 9F, §4.4, decisión D5): bytes exactos del final de cada documento
// en el texto plano y con formato, por cada transporte, sin enviar nada a una impresora real (conexión de
// loopback de la prueba, administrador de Windows simulado y programas de CUPS simulados).

// legacyFullCut es el corte de una fotografía anterior a la 9F con «cut» y «supports_cut»: total con los
// tres saltos de siempre.
var legacyFullCut = documents.PaperCut{Enabled: true, FeedLines: documents.LegacyCutFeedLines}

func feedLines(value int) *int {
	return &value
}

// cutPrinter es una impresora con corte, del tipo y el avance indicados.
func cutPrinter(mode string, feed int) Printer {
	return Printer{Name: "Tickets", Transport: "escpos_tcp", Destination: "127.0.0.1", Port: 9100, PaperWidth: 80, Copies: 1,
		Cut: true, SupportsCut: true, CutMode: mode, CutFeedLines: feedLines(feed)}
}

// expectedEnding son los bytes esperados escritos a mano: N saltos y GS V m.
func expectedEnding(feed int, mode byte) []byte {
	return append(bytes.Repeat([]byte{'\n'}, feed), 0x1d, 0x56, mode)
}

func TestPaperCutEndingBytes(t *testing.T) {
	cases := []struct {
		name    string
		printer Printer
		want    []byte
	}{
		{"sin corte", Printer{}, []byte{'\n', '\n', '\n'}},
		{"sin corte aunque tenga avance y tipo", Printer{CutMode: CutPartial, CutFeedLines: feedLines(8)}, []byte{'\n', '\n', '\n'}},
		{"total con avance 0", cutPrinter(CutFull, 0), []byte{0x1d, 0x56, 0x00}},
		{"total con avance 4", cutPrinter(CutFull, 4), expectedEnding(4, 0x00)},
		{"total con avance 8", cutPrinter(CutFull, 8), expectedEnding(8, 0x00)},
		{"parcial con avance 0", cutPrinter(CutPartial, 0), []byte{0x1d, 0x56, 0x01}},
		{"parcial con avance 4", cutPrinter(CutPartial, 4), expectedEnding(4, 0x01)},
		{"parcial con avance 8", cutPrinter(CutPartial, 8), expectedEnding(8, 0x01)},
		// Una fotografía anterior a la 9F (sin tipo ni avance) corta como entonces.
		{"fotografía anterior con corte", Printer{Cut: true, SupportsCut: true}, []byte{'\n', '\n', '\n', 0x1d, 0x56, 0x00}},
	}
	for _, testCase := range cases {
		if got := testCase.printer.PaperCut().Ending(); !bytes.Equal(got, testCase.want) {
			t.Fatalf("%s: % x, se esperaba % x", testCase.name, got, testCase.want)
		}
	}
}

// El texto plano (comandas, tickets, precuentas, cortes, pruebas) termina con el corte de la impresora.
// Sin corte, los bytes son idénticos a los de la etapa 9E: ESC @, ESC t 2, el texto en CP850 y tres saltos.
func TestPlainTextESCPosBytesWithPaperCut(t *testing.T) {
	legacy := append([]byte{0x1b, 0x40, 0x1b, 0x74, 0x02}, []byte("Caf\x82 \xa4\n")...)
	legacy = append(legacy, '\n', '\n', '\n')
	for _, printer := range []Printer{{}, {CutMode: CutPartial, CutFeedLines: feedLines(0)}, {Cut: false, SupportsCut: true}} {
		if got := ESCPosBytes("Café ñ\n", printer); !bytes.Equal(got, legacy) {
			t.Fatalf("Sin corte los bytes cambiaron: % x", got)
		}
	}
	body := legacy[:len(legacy)-3]
	for _, feed := range []int{0, 4, 8} {
		for mode, code := range map[string]byte{CutFull: 0x00, CutPartial: 0x01} {
			got := ESCPosBytes("Café ñ\n", cutPrinter(mode, feed))
			want := append(append([]byte{}, body...), expectedEnding(feed, code)...)
			if !bytes.Equal(got, want) {
				t.Fatalf("Corte %s con avance %d: % x", mode, feed, got)
			}
		}
	}
	// El texto grande de Cocina conserva GS ! 0x11 antes del texto.
	large := ESCPosBytes("Comanda", cutPrinter(CutPartial, 4), "large")
	if !bytes.HasPrefix(large, []byte{0x1b, 0x40, 0x1b, 0x74, 0x02, 0x1d, 0x21, 0x11}) || !bytes.HasSuffix(large, expectedEnding(4, 0x01)) {
		t.Fatalf("El texto grande con corte parcial quedó mal: % x", large)
	}
}

// Un documento con formato o logotipo termina igual: sin corte, idéntico al de la 9E; con corte, los
// renglones de avance y GS V 0/1.
func TestFormattedESCPosWithPaperCut(t *testing.T) {
	layout := formattedLayout()
	withoutCut := documents.EncodeESCPos(layout, nil, documents.NoPaperCut)
	legacyCut := documents.EncodeESCPos(layout, nil, legacyFullCut)
	if !bytes.Equal(legacyCut, append(append([]byte{}, withoutCut...), 0x1d, 0x56, 0x00)) || !bytes.HasSuffix(withoutCut, []byte{'\n', '\n', '\n'}) {
		t.Fatal("El corte de una fotografía anterior a la 9F cambió los bytes")
	}
	body := withoutCut[:len(withoutCut)-3]
	for _, feed := range []int{0, 4, 8} {
		for mode, code := range map[string]byte{CutFull: 0x00, CutPartial: 0x01} {
			got := documents.EncodeESCPos(layout, nil, cutPrinter(mode, feed).PaperCut())
			if !bytes.Equal(got, append(append([]byte{}, body...), expectedEnding(feed, code)...)) {
				t.Fatalf("Formato con corte %s y avance %d: % x", mode, feed, got)
			}
		}
	}
}

// Por ESC/POS de red viajan los bytes con el corte de la impresora, una vez por copia.
func TestPaperCutThroughNetworkESCPos(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan []byte, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			received <- nil
			return
		}
		defer connection.Close()
		buffer := bytes.Buffer{}
		_, _ = buffer.ReadFrom(connection)
		received <- buffer.Bytes()
	}()
	printer := cutPrinter(CutPartial, 8)
	printer.Port = listener.Addr().(*net.TCPAddr).Port
	printer.Copies = 2
	job := PrintJob{ID: "00000000-0000-4000-8000-0000000009f1", PrinterSnapshot: printer, Content: "PRECUENTA\nTotal $10.00"}
	outcome := submitESCPos(context.Background(), job)
	single := ESCPosBytes(job.Content, printer)
	data := <-received
	if outcome.State != "submitted" || !bytes.Equal(data, append(append([]byte{}, single...), single...)) ||
		!bytes.HasSuffix(single, expectedEnding(8, 0x01)) {
		t.Fatalf("El corte parcial no viajó por la red: %+v % x", outcome, data)
	}
}

// Por la cola de Windows (RAW) viajan los mismos bytes que por la red.
func TestPaperCutThroughWindowsQueue(t *testing.T) {
	spooler := usbSpooler()
	job := windowsTextJob(1)
	job.PrinterSnapshot.CutMode = CutPartial
	job.PrinterSnapshot.CutFeedLines = feedLines(4)
	outcome := HostTransport{spooler: spooler}.Submit(context.Background(), job)
	if outcome.State != "submitted" || !bytes.Equal(spooler.written, ESCPosBytes(job.Content, job.PrinterSnapshot, "large")) ||
		!bytes.HasSuffix(spooler.written, expectedEnding(4, 0x01)) {
		t.Fatalf("El corte parcial no viajó por la cola de Windows: %+v % x", outcome, spooler.written)
	}
	// Con formato, el documento RAW termina con el corte total sin avance.
	formatted := usbSpooler()
	document := formattedJob("cups")
	document.PrinterSnapshot.Destination = "POS_80"
	document.PrinterSnapshot.Copies = 1
	document.PrinterSnapshot.CutMode = CutFull
	document.PrinterSnapshot.CutFeedLines = feedLines(0)
	if outcome := (HostTransport{spooler: formatted}).Submit(context.Background(), document); outcome.State != "submitted" {
		t.Fatalf("El documento con formato no se aceptó: %+v", outcome)
	}
	if !bytes.HasSuffix(formatted.written, []byte{0x1d, 0x56, 0x00}) || bytes.HasSuffix(formatted.written, []byte{'\n', 0x1d, 0x56, 0x00}) {
		t.Fatalf("El corte total sin avance no terminó el documento RAW: % x", formatted.written)
	}
}

// Por CUPS con «Esta impresora entiende ESC/POS» y una cola raw, el archivo lleva el corte de la impresora.
func TestPaperCutThroughRawCUPS(t *testing.T) {
	job := formattedJob("cups")
	job.PrinterSnapshot.CutMode = CutPartial
	job.PrinterSnapshot.CutFeedLines = feedLines(8)
	raw := &recordedCommands{t: t, replies: map[string]recordedReply{
		"/usr/bin/lpstat":    {output: recorded(t, "lpstat-p-usb-pos80.txt")},
		"/usr/bin/lpoptions": {output: recorded(t, "lpoptions-usb-pos80.txt")},
		"/usr/bin/lp":        {output: []byte("la id. de petición es Printer_POS_80-401 (1 archivo(s))\n")},
	}}
	outcome := HostTransport{commands: raw}.Submit(context.Background(), job)
	sent := raw.calls[len(raw.calls)-1].pulse
	if outcome.State != "submitted" || !bytes.Equal(sent, documents.EncodeESCPos(job.Layout, nil, job.PrinterSnapshot.PaperCut())) ||
		!bytes.HasSuffix(sent, expectedEnding(8, 0x01)) {
		t.Fatalf("El corte parcial no viajó por CUPS raw: %+v % x", outcome, sent)
	}
}

// Por CUPS de texto no se envían comandos ESC/POS: el corte es la opción del controlador si está
// configurada; sin ella se imprime sin corte (ya no queda en «Requiere intervención»).
func TestPaperCutThroughCUPSText(t *testing.T) {
	job := PrintJob{ID: "00000000-0000-4000-8000-0000000009f2", Content: "PRECUENTA\n",
		PrinterSnapshot: Printer{Name: "Tickets", Transport: "cups", Destination: "TICKETS", PaperWidth: 80, Copies: 1, Port: 9100,
			Cut: true, SupportsCut: true, CutMode: CutPartial, CutFeedLines: feedLines(4)}}
	withoutOption := cupsTextArguments(job, "/tmp/archivo")
	if slices.Contains(withoutOption, "") || strings.Contains(strings.Join(withoutOption, " "), "Cut") {
		t.Fatalf("Sin opción del controlador no debe pedirse corte: %q", withoutOption)
	}
	job.PrinterSnapshot.CutterOption = "CutMedia=1"
	withOption := cupsTextArguments(job, "/tmp/archivo")
	if !slices.Contains(withOption, "CutMedia=1") {
		t.Fatalf("La opción del controlador no se envió: %q", withOption)
	}
	job.PrinterSnapshot.Cut = false
	if slices.Contains(cupsTextArguments(job, "/tmp/archivo"), "CutMedia=1") {
		t.Fatal("Sin corte se pidió la opción del controlador")
	}
}

// Una fotografía guardada antes de la 9F (sin cut_mode ni cut_feed_lines) se reimprime idéntica.
func TestPrinterSnapshotBeforeStage9F(t *testing.T) {
	var snapshot Printer
	if err := json.Unmarshal([]byte(`{"id":"00000000-0000-4000-8000-000000000031","name":"Tickets","transport":"escpos_tcp",
        "destination":"192.168.1.50","port":9100,"paper_width":80,"copies":1,"supports_cut":true,"cut":true,"cutter_option":"",
        "accepts_escpos":false,"columns":null,"active":true,"fallback_id":null,"revision":3}`), &snapshot); err != nil {
		t.Fatal(err)
	}
	if err := validatePrinter(&snapshot); err != nil {
		t.Fatalf("La fotografía anterior dejó de ser válida: %v", err)
	}
	if snapshot.PaperCut() != legacyFullCut {
		t.Fatalf("La fotografía anterior cambió su corte: %+v", snapshot.PaperCut())
	}
}

// Valores predeterminados al guardar: una impresora nueva de 80 mm corta (total, 4 renglones) y una de
// 58 mm no; lo que el cliente envía se respeta; al editar, lo que no envía se conserva; supports_cut sigue
// a cut.
func TestPaperCutDefaultsWhenSaving(t *testing.T) {
	none := map[string]bool{}
	eighty := Printer{PaperWidth: 80}
	applyCutDefaults(&eighty, nil, none)
	if !eighty.Cut || !eighty.SupportsCut || eighty.CutMode != CutFull || eighty.CutFeedLines == nil || *eighty.CutFeedLines != 4 {
		t.Fatalf("Impresora nueva de 80 mm: %+v", eighty)
	}
	fiftyEight := Printer{PaperWidth: 58}
	applyCutDefaults(&fiftyEight, nil, none)
	if fiftyEight.Cut || fiftyEight.SupportsCut || *fiftyEight.CutFeedLines != 4 {
		t.Fatalf("Impresora nueva de 58 mm: %+v", fiftyEight)
	}
	explicit := Printer{PaperWidth: 80, CutMode: CutPartial, CutFeedLines: feedLines(0)}
	applyCutDefaults(&explicit, nil, map[string]bool{"cut": true})
	if explicit.Cut || explicit.SupportsCut || explicit.CutMode != CutPartial || *explicit.CutFeedLines != 0 {
		t.Fatalf("Los valores enviados no se respetaron: %+v", explicit)
	}
	before := cutPrinter(CutPartial, 6)
	edited := Printer{PaperWidth: 80}
	applyCutDefaults(&edited, &before, none)
	if !edited.Cut || !edited.SupportsCut || edited.CutMode != CutPartial || *edited.CutFeedLines != 6 {
		t.Fatalf("Un cliente anterior a la 9F cambió el corte guardado: %+v", edited)
	}
	single := Printer{PaperWidth: 58, Cut: true}
	applyCutDefaults(&single, nil, map[string]bool{"cut": true})
	if !single.SupportsCut {
		t.Fatal("«Cortar el papel al terminar» exige todavía una segunda casilla")
	}
}

func TestPaperCutValidation(t *testing.T) {
	valid := Printer{Name: "Tickets", Transport: "escpos_tcp", Destination: "192.168.1.50", Port: 9100, PaperWidth: 80, Copies: 1,
		Cut: true, SupportsCut: true, CutMode: CutFull, CutFeedLines: feedLines(8)}
	if err := validatePrinter(&valid); err != nil {
		t.Fatalf("Se rechazó un corte válido: %v", err)
	}
	for _, change := range []func(*Printer){
		func(printer *Printer) { printer.CutMode = "medio" },
		func(printer *Printer) { printer.CutFeedLines = feedLines(9) },
		func(printer *Printer) { printer.CutFeedLines = feedLines(-1) },
	} {
		printer := valid
		change(&printer)
		if err := validatePrinter(&printer); err == nil {
			t.Fatalf("Se aceptó un corte fuera de la lista cerrada: %+v", printer)
		}
	}
}

// Claves de los problemas de impresión: explícitas o derivadas del estado del resultado.
func TestPrintProblemCodes(t *testing.T) {
	cases := map[string]Outcome{
		ProblemUnreachable: {State: "error"},
		ProblemSetup:       {State: "intervention"},
		ProblemUncertain:   {State: "uncertain"},
		ProblemInactive:    {State: "intervention", Code: ProblemInactive},
	}
	for want, outcome := range cases {
		if got := outcomeWithCode(outcome).Code; got != want {
			t.Fatalf("%+v: %q, se esperaba %q", outcome, got, want)
		}
	}
	if outcomeWithCode(Outcome{State: "submitted"}).Code != "" {
		t.Fatal("Un envío aceptado no tiene problema")
	}
	agent := HostTransport{commands: &recordedCommands{t: t, forbidden: true}}.Submit(context.Background(), PrintJob{
		PrinterSnapshot: Printer{Name: "Otra terminal", Transport: "agent", Destination: "Caja 2", PaperWidth: 80, Copies: 1, Port: 9100},
		Content:         "PRUEBA",
	})
	if agent.State != "intervention" || agent.Code != ProblemUnsupported || agent.Message != "Esta impresora todavía no puede usarse para imprimir." {
		t.Fatalf("La impresora de otra terminal no explica el problema: %+v", agent)
	}
	if PrinterProblem(&Printer{Active: true, Transport: "cups"}) != "" || PrinterProblem(&Printer{Active: false, Transport: "cups"}) != ProblemInactive ||
		PrinterProblem(&Printer{Active: true, Transport: "agent"}) != ProblemUnsupported {
		t.Fatal("La impresora válida para precuentas no sigue la decisión D4")
	}
	if inactivePrinterMessage("Tickets") != "La impresora Tickets está desactivada." {
		t.Fatal("El texto de la impresora desactivada cambió")
	}
}
