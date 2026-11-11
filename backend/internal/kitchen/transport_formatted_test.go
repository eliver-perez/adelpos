package kitchen

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/documents"
)

// Transportes de un documento con atributos o logotipo (etapa 9B), sin ejecutar programas: el
// ejecutor simulado registra lpstat, lpoptions y lp. Ningún trabajo llega a una impresora.

func formattedLayout() *documents.Layout {
	return &documents.Layout{
		PaperWidth: 80, Columns: 42, TextSize: documents.TextSizeNormal,
		Lines: []documents.Line{
			{Text: "RESTAURANTE", Bold: true, Size: documents.SizeDouble},
			{Size: documents.SizeNormal, Image: &documents.Image{RasterID: "logo-80", WidthDots: 16, HeightDots: 2}},
			{Text: "Café ñ", Size: documents.SizeNormal},
		},
	}
}

// formattedJob es un trabajo con atributos para una impresora del transporte indicado. En CUPS la
// impresora está marcada «Esta impresora entiende ESC/POS (formato y logotipo)».
func formattedJob(transport string) PrintJob {
	printer := Printer{Name: "Tickets", Transport: transport, Destination: "Printer_POS_80", PaperWidth: 80, Copies: 2, Port: 9100,
		Cut: true, SupportsCut: true, CutterOption: "", AcceptsESCPos: transport == "cups"}
	return PrintJob{ID: "00000000-0000-4000-8000-0000000000f1", PrinterSnapshot: printer, Content: "RESTAURANTE\nCafé ñ", Layout: formattedLayout()}
}

// Por CUPS con una cola que admite datos raw se envían los bytes ESC/POS con lp -o raw, las copias
// de la impresora y el título técnico; el logotipo sin bits se omite con aviso.
func TestFormattedDocumentThroughRawCUPS(t *testing.T) {
	job := formattedJob("cups")
	accepted := &recordedCommands{t: t, replies: map[string]recordedReply{
		"/usr/bin/lpstat":    {output: recorded(t, "lpstat-p-usb-pos80.txt")},
		"/usr/bin/lpoptions": {output: recorded(t, "lpoptions-usb-pos80.txt")},
		"/usr/bin/lp":        {output: []byte("la id. de petición es Printer_POS_80-301 (1 archivo(s))\n")},
	}}
	job.Rasters = map[string]*documents.Raster{"logo-80": {WidthDots: 16, HeightDots: 2, Bits: []byte{0xff, 0x00, 0x0f, 0xf0}}}
	outcome := HostTransport{commands: accepted}.Submit(context.Background(), job)
	if outcome.State != "submitted" || outcome.Reference != "Printer_POS_80-301" || strings.Contains(outcome.Message, "logotipo") {
		t.Fatalf("El documento raw quedó mal registrado: %+v", outcome)
	}
	lp := accepted.calls[len(accepted.calls)-1]
	expected := []string{"-d", "Printer_POS_80", "-o", "raw", "-n", "2", "-t", "ADEL-" + job.ID}
	if lp.program != "/usr/bin/lp" || strings.Join(lp.arguments[:8], "\x00") != strings.Join(expected, "\x00") {
		t.Fatalf("Argumentos de lp incorrectos: %q", lp.arguments)
	}
	if !bytes.Equal(lp.pulse, documents.EncodeESCPos(job.Layout, job.Rasters, true)) {
		t.Fatalf("El archivo raw no contiene los bytes ESC/POS del layout: % x", lp.pulse)
	}
	if !bytes.Contains(lp.pulse, []byte{0x1d, 0x76, 0x30, 0x00, 0x02, 0x00, 0x02, 0x00, 0xff, 0x00, 0x0f, 0xf0}) {
		t.Fatal("La imagen no viajó con GS v 0")
	}
	missing := &recordedCommands{t: t, replies: accepted.replies}
	job.Rasters = nil
	withoutLogo := HostTransport{commands: missing}.Submit(context.Background(), job)
	if withoutLogo.State != "submitted" || !strings.Contains(withoutLogo.Message, "logotipo") {
		t.Fatalf("Sin los bits del logotipo no se avisó o falló la impresión: %+v", withoutLogo)
	}
}

// Una cola que no admite datos raw imprime el texto por el camino anterior y lo dice, sin fallar;
// las reglas de resultado incierto no cambian.
func TestFormattedDocumentThroughTextOnlyCUPS(t *testing.T) {
	job := formattedJob("cups")
	queries := &recordedCommands{t: t, replies: map[string]recordedReply{
		"/usr/bin/lpstat":    {output: recorded(t, "lpstat-p-usb-pos80.txt")},
		"/usr/bin/lpoptions": {output: recorded(t, "lpoptions-dnssd-airprint.txt")},
	}}
	var received *PrintJob
	transport := HostTransport{commands: queries, textCUPS: func(_ context.Context, sent PrintJob) Outcome {
		received = &sent
		return Outcome{State: "submitted", Accepted: true, Reference: "Q-1", Message: "Aceptado por CUPS; no confirma salida física de papel."}
	}}
	outcome := transport.Submit(context.Background(), job)
	if received == nil || received.Content != "RESTAURANTE\nCafé ñ" || outcome.State != "submitted" ||
		!strings.Contains(outcome.Message, "no admite datos raw") || !strings.Contains(outcome.Message, "no confirma salida física") {
		t.Fatalf("La cola sin datos raw no imprimió el texto con aviso: %+v", outcome)
	}
	for _, call := range queries.calls {
		if call.program == "/usr/bin/lp" {
			t.Fatal("Se enviaron bytes raw a una cola que no los admite")
		}
	}
	uncertain := HostTransport{commands: queries, textCUPS: func(context.Context, PrintJob) Outcome {
		return Outcome{State: "uncertain", Message: "CUPS no devolvió un resultado inequívoco."}
	}}.Submit(context.Background(), job)
	if uncertain.State != "uncertain" || uncertain.Accepted {
		t.Fatalf("El resultado incierto cambió en el camino de texto: %+v", uncertain)
	}
	unreachable := &recordedCommands{t: t, replies: map[string]recordedReply{
		"/usr/bin/lpstat": {output: nil, err: &net.OpError{Op: "dial"}},
	}}
	failed := HostTransport{commands: unreachable}.Submit(context.Background(), job)
	if failed.State != "error" || failed.Accepted {
		t.Fatalf("Una cola que no responde no quedó como error confirmado: %+v", failed)
	}
}

// Por ESC/POS de red el documento viaja con GS !, ESC E y el corte, una vez por copia.
func TestFormattedDocumentThroughESCPos(t *testing.T) {
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
	job := formattedJob("escpos_tcp")
	job.PrinterSnapshot.Destination = "127.0.0.1"
	job.PrinterSnapshot.Port = listener.Addr().(*net.TCPAddr).Port
	outcome := submitESCPosFormatted(t, job)
	data := <-received
	single := documents.EncodeESCPos(job.Layout, nil, true)
	if outcome.State != "submitted" || !bytes.Equal(data, append(append([]byte{}, single...), single...)) {
		t.Fatalf("Los bytes ESC/POS con atributos no se entregaron dos veces: %+v % x", outcome, data)
	}
	if !strings.Contains(outcome.Message, "logotipo") {
		t.Fatal("La imagen sin bits no se avisó")
	}
}

// submitESCPosFormatted usa el camino con atributos sin validar el puerto 9100–9109 del destino:
// la prueba escucha en un puerto libre de loopback.
func submitESCPosFormatted(t *testing.T, job PrintJob) Outcome {
	t.Helper()
	return HostTransport{commands: &recordedCommands{t: t, forbidden: true}}.submitFormatted(context.Background(), job)
}

// Un layout plano viaja por ESC/POS con exactamente los bytes del texto anterior a la 9B.
func TestPlainLayoutKeepsESCPosBytesOverTCP(t *testing.T) {
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
	job := formattedJob("escpos_tcp")
	job.PrinterSnapshot.Destination = "127.0.0.1"
	job.PrinterSnapshot.Port = listener.Addr().(*net.TCPAddr).Port
	job.PrinterSnapshot.Copies = 1
	job.Layout = &documents.Layout{Lines: []documents.Line{{Text: "Solo texto", Size: documents.SizeNormal}}}
	job.Content = "Solo texto"
	job.TextSize = "large"
	// submitESCPos es el camino de texto que Submit usa con un layout plano.
	outcome := submitESCPos(context.Background(), job)
	if !job.Layout.Plain() || outcome.State != "submitted" || !bytes.Equal(<-received, ESCPosBytes("Solo texto", job.PrinterSnapshot, "large")) {
		t.Fatalf("Un layout plano no conservó los bytes del texto: %+v", outcome)
	}
}

// Una cola CUPS que admite datos raw no basta (revisión de la 9B, hallazgo medio 4): sin la marca
// explícita de la impresora el documento se imprime como texto por el camino anterior, con aviso y
// sin consultar la cola ni fallar. Con la marca y una cola raw viaja como ESC/POS.
func TestCUPSWithoutESCPosOptionPrintsText(t *testing.T) {
	job := formattedJob("cups")
	job.PrinterSnapshot.AcceptsESCPos = false
	job.Rasters = map[string]*documents.Raster{"logo-80": {WidthDots: 16, HeightDots: 2, Bits: []byte{0xff, 0x00, 0x0f, 0xf0}}}
	var received *PrintJob
	transport := HostTransport{commands: &recordedCommands{t: t, forbidden: true}, textCUPS: func(_ context.Context, sent PrintJob) Outcome {
		received = &sent
		return Outcome{State: "submitted", Accepted: true, Reference: "Q-3", Message: "Aceptado por CUPS; no confirma salida física de papel."}
	}}
	outcome := transport.Submit(context.Background(), job)
	if received == nil || received.Content != job.Content || outcome.State != "submitted" || !outcome.Accepted ||
		!strings.Contains(outcome.Message, "no está marcada como «Esta impresora entiende ESC/POS") {
		t.Fatalf("Sin la marca ESC/POS no se imprimió el texto con aviso: %+v", outcome)
	}
	// El resultado incierto del camino de texto se conserva.
	uncertain := HostTransport{commands: &recordedCommands{t: t, forbidden: true}, textCUPS: func(context.Context, PrintJob) Outcome {
		return Outcome{State: "uncertain", Message: "CUPS no devolvió un resultado inequívoco."}
	}}.Submit(context.Background(), job)
	if uncertain.State != "uncertain" || uncertain.Accepted {
		t.Fatalf("El resultado incierto cambió sin la marca ESC/POS: %+v", uncertain)
	}
	// Con la marca y la misma cola raw, los bytes ESC/POS viajan con lp -o raw.
	job.PrinterSnapshot.AcceptsESCPos = true
	raw := &recordedCommands{t: t, replies: map[string]recordedReply{
		"/usr/bin/lpstat":    {output: recorded(t, "lpstat-p-usb-pos80.txt")},
		"/usr/bin/lpoptions": {output: recorded(t, "lpoptions-usb-pos80.txt")},
		"/usr/bin/lp":        {output: []byte("la id. de petición es Printer_POS_80-302 (1 archivo(s))\n")},
	}}
	marked := HostTransport{commands: raw}.Submit(context.Background(), job)
	if marked.State != "submitted" || raw.calls[len(raw.calls)-1].program != "/usr/bin/lp" {
		t.Fatalf("Con la marca y la cola raw el documento no viajó como ESC/POS: %+v", marked)
	}
	// La marca solo existe en CUPS: ESC/POS por red y el agente la rechazan.
	for _, transportName := range []string{"escpos_tcp", "agent"} {
		printer := formattedJob(transportName).PrinterSnapshot
		printer.Destination = "192.168.1.50"
		printer.AcceptsESCPos = true
		err := validatePrinter(&printer)
		problem, isPublic := err.(*Error)
		if !isPublic || !strings.Contains(problem.Message, "solo se configura en colas CUPS") {
			t.Fatalf("La marca ESC/POS se aceptó en %s: %v", transportName, err)
		}
	}
}

// Un trabajo redirigido, con respaldo o copiado a una impresora de otro ancho conserva su IR
// (revisión de la 9B, hallazgo bajo 5): el logotipo convertido para el ancho original no se envía;
// se omite con aviso y el resto del documento se imprime.
func TestLogoOfOtherPaperWidthIsOmitted(t *testing.T) {
	logo := []byte{0xff, 0x00, 0x0f, 0xf0}
	job := formattedJob("cups")
	job.PrinterSnapshot.PaperWidth = 58
	job.Rasters = map[string]*documents.Raster{"logo-80": {WidthDots: 16, HeightDots: 2, Bits: logo}}
	replies := map[string]recordedReply{
		"/usr/bin/lpstat":    {output: recorded(t, "lpstat-p-usb-pos80.txt")},
		"/usr/bin/lpoptions": {output: recorded(t, "lpoptions-usb-pos80.txt")},
		"/usr/bin/lp":        {output: []byte("la id. de petición es Printer_POS_80-303 (1 archivo(s))\n")},
	}
	raw := &recordedCommands{t: t, replies: replies}
	outcome := HostTransport{commands: raw}.Submit(context.Background(), job)
	sent := raw.calls[len(raw.calls)-1].pulse
	if outcome.State != "submitted" || bytes.Contains(sent, []byte{0x1d, 0x76, 0x30, 0x00}) || !bytes.Contains(sent, []byte("RESTAURANTE")) {
		t.Fatalf("El logotipo de otro ancho de papel viajó o el documento no se imprimió: %+v % x", outcome, sent)
	}
	if !strings.Contains(outcome.Message, "otro ancho de papel") || strings.Contains(outcome.Message, "No se encontró") {
		t.Fatalf("El aviso no explica el logotipo omitido: %q", outcome.Message)
	}
	// Un raster más ancho que el área imprimible de la impresora tampoco se envía.
	wide := formattedJob("cups")
	wide.Layout.PaperWidth = 58
	wide.PrinterSnapshot.PaperWidth = 58
	wide.Rasters = map[string]*documents.Raster{"logo-80": {WidthDots: 576, HeightDots: 1, Bits: make([]byte, 72)}}
	raw = &recordedCommands{t: t, replies: replies}
	outcome = HostTransport{commands: raw}.Submit(context.Background(), wide)
	if bytes.Contains(raw.calls[len(raw.calls)-1].pulse, []byte{0x1d, 0x76, 0x30, 0x00}) || !strings.Contains(outcome.Message, "otro ancho de papel") {
		t.Fatalf("Un raster de 576 puntos se envió a una impresora de 58 mm: %+v", outcome)
	}
	// Con el mismo ancho el logotipo sí viaja.
	same := formattedJob("cups")
	same.Rasters = map[string]*documents.Raster{"logo-80": {WidthDots: 16, HeightDots: 2, Bits: logo}}
	raw = &recordedCommands{t: t, replies: replies}
	outcome = HostTransport{commands: raw}.Submit(context.Background(), same)
	if !bytes.Contains(raw.calls[len(raw.calls)-1].pulse, append([]byte{0x1d, 0x76, 0x30, 0x00, 0x02, 0x00, 0x02, 0x00}, logo...)) ||
		strings.Contains(outcome.Message, "logotipo") {
		t.Fatalf("El logotipo del mismo ancho no viajó: %+v", outcome)
	}
}
