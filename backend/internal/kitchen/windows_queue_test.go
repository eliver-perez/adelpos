package kitchen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/documents"
)

// Cola de impresión de Windows (etapa 9E) con un administrador de impresión simulado: registra cada
// llamada y los bytes de cada documento, sin abrir impresoras reales ni enviar nada. Corre en
// cualquier sistema; la lectura del administrador real está en winspool_windows_test.go.

// fakeSpooler simula winspool.drv. Las fallas se configuran por llamada y block* detienen una
// llamada hasta cerrar el canal, para probar los límites de tiempo.
type fakeSpooler struct {
	mutex         sync.Mutex
	printers      []spoolerPrinter
	portList      []spoolerPort
	listError     error
	portsError    error
	openError     error
	startError    error
	pageError     error
	endPageError  error
	endDocError   error
	failWriteCopy int
	shortCopy     int
	jobID         uint32
	blockOpen     chan struct{}
	blockPrinter  chan struct{}
	blockWrite    chan struct{}
	calls         []string
	written       []byte
	titles        []string
	writes        int
}

func (spooler *fakeSpooler) record(call string) {
	spooler.mutex.Lock()
	defer spooler.mutex.Unlock()
	spooler.calls = append(spooler.calls, call)
}

func (spooler *fakeSpooler) recorded() []string {
	spooler.mutex.Lock()
	defer spooler.mutex.Unlock()
	return append([]string{}, spooler.calls...)
}

func (spooler *fakeSpooler) listPrinters() ([]spoolerPrinter, error) {
	spooler.record("list")
	return spooler.printers, spooler.listError
}

func (spooler *fakeSpooler) printer(name string) (spoolerPrinter, error) {
	spooler.record("printer " + name)
	if spooler.blockPrinter != nil {
		<-spooler.blockPrinter
	}
	for _, info := range spooler.printers {
		if info.Name == name {
			return info, nil
		}
	}
	return spoolerPrinter{}, errors.New("no existe")
}

func (spooler *fakeSpooler) ports() ([]spoolerPort, error) {
	spooler.record("ports")
	return spooler.portList, spooler.portsError
}

func (spooler *fakeSpooler) open(name string) (spoolerHandle, error) {
	spooler.record("open " + name)
	if spooler.blockOpen != nil {
		<-spooler.blockOpen
	}
	if spooler.openError != nil {
		return nil, spooler.openError
	}
	return &fakeHandle{spooler: spooler}, nil
}

type fakeHandle struct {
	spooler *fakeSpooler
}

func (handle *fakeHandle) startDocument(title string) (uint32, error) {
	spooler := handle.spooler
	spooler.record("start " + title)
	if spooler.startError != nil {
		return 0, spooler.startError
	}
	spooler.mutex.Lock()
	spooler.titles = append(spooler.titles, title)
	spooler.mutex.Unlock()
	return spooler.jobID, nil
}

func (handle *fakeHandle) startPage() error {
	handle.spooler.record("page")
	return handle.spooler.pageError
}

func (handle *fakeHandle) write(data []byte) (int, error) {
	spooler := handle.spooler
	spooler.record(fmt.Sprintf("write %d", len(data)))
	if spooler.blockWrite != nil {
		<-spooler.blockWrite
	}
	spooler.mutex.Lock()
	defer spooler.mutex.Unlock()
	spooler.writes++
	if spooler.writes == spooler.failWriteCopy {
		return 0, errors.New("se interrumpió")
	}
	if spooler.writes == spooler.shortCopy {
		spooler.written = append(spooler.written, data[:len(data)/2]...)
		return len(data) / 2, nil
	}
	spooler.written = append(spooler.written, data...)
	return len(data), nil
}

func (handle *fakeHandle) endPage() error {
	handle.spooler.record("endpage")
	return handle.spooler.endPageError
}

func (handle *fakeHandle) endDocument() error {
	handle.spooler.record("enddoc")
	return handle.spooler.endDocError
}

func (handle *fakeHandle) abort() error {
	handle.spooler.record("abort")
	return nil
}

func (handle *fakeHandle) close() error {
	handle.spooler.record("close")
	return nil
}

// usbSpooler tiene la impresora de tickets «POS_80» en el puerto USB001, una impresora PDF y los
// puertos locales que informa Windows 11.
func usbSpooler() *fakeSpooler {
	return &fakeSpooler{
		jobID: 17,
		printers: []spoolerPrinter{
			{Name: "POS_80", Port: "USB001", Driver: "Generic / Text Only", Attributes: 0x240},
			{Name: "Microsoft Print to PDF", Port: "PORTPROMPT:", Driver: "Microsoft Print To PDF", Attributes: 0x240},
		},
		portList: []spoolerPort{
			{Name: "USB001", Monitor: "USB Monitor"},
			{Name: "COM1:", Monitor: "Local Monitor"},
			{Name: "FILE:", Monitor: "Local Monitor"},
			{Name: "PORTPROMPT:", Monitor: "Local Monitor"},
		},
	}
}

func windowsTextJob(copies int) PrintJob {
	printer := Printer{Name: "Tickets", Transport: "cups", Destination: "POS_80", PaperWidth: 80, Copies: copies, Port: 9100,
		Cut: true, SupportsCut: true}
	return PrintJob{ID: "00000000-0000-4000-8000-0000000009e1", PrinterSnapshot: printer, Content: "COMANDA\nCafé ñ\n", TextSize: "large"}
}

// Un documento de texto viaja con los mismos bytes ESC/POS que la red (CP850, texto grande y corte
// GS V), una vez por copia dentro de un solo documento RAW con el título técnico del trabajo.
func TestWindowsQueueTextDocumentBytesAndCopies(t *testing.T) {
	spooler := usbSpooler()
	job := windowsTextJob(2)
	outcome := HostTransport{spooler: spooler}.Submit(context.Background(), job)
	if outcome.State != "submitted" || !outcome.Accepted || outcome.PhysicalConfirmed || outcome.Reference != "windows-17" {
		t.Fatalf("El documento aceptado quedó mal registrado: %+v", outcome)
	}
	if outcome.Message != "Aceptado por la cola de impresión de Windows; no confirma salida física de papel." {
		t.Fatalf("Mensaje de aceptación incorrecto: %q", outcome.Message)
	}
	single := ESCPosBytes(job.Content, job.PrinterSnapshot, "large")
	if !bytes.HasSuffix(single, []byte{0x1d, 0x56, 0x00}) || !bytes.Contains(single, []byte{0x1d, 0x21, 0x11}) {
		t.Fatalf("Los bytes de referencia no llevan el corte ni el texto grande: % x", single)
	}
	if !bytes.Equal(spooler.written, append(append([]byte{}, single...), single...)) {
		t.Fatalf("Los bytes RAW no son los de ESC/POS por red dos veces: % x", spooler.written)
	}
	expected := []string{
		"printer POS_80", "ports", "open POS_80", "start ADEL-" + job.ID, "page",
		fmt.Sprintf("write %d", len(single)), fmt.Sprintf("write %d", len(single)), "endpage", "enddoc", "close",
	}
	if strings.Join(spooler.recorded(), "|") != strings.Join(expected, "|") {
		t.Fatalf("Llamadas al administrador de impresión: %q", spooler.recorded())
	}
	// Sin corte autorizado no se envía GS V; la opción de corte de CUPS no interviene.
	plain := usbSpooler()
	job.PrinterSnapshot.Cut = false
	job.PrinterSnapshot.CutterOption = "CutMedia=1"
	job.PrinterSnapshot.Copies = 1
	if outcome := (HostTransport{spooler: plain}).Submit(context.Background(), job); outcome.State != "submitted" {
		t.Fatalf("El documento sin corte no se aceptó: %+v", outcome)
	}
	if bytes.Contains(plain.written, []byte{0x1d, 0x56}) {
		t.Fatal("Se envió el corte sin solicitarlo")
	}
}

// Un documento con atributos o logotipo viaja siempre como ESC/POS (documents.EncodeESCPos), aunque
// la impresora no esté marcada «Esta impresora entiende ESC/POS»: en Windows la cola recibe RAW.
func TestWindowsQueueFormattedDocumentAlwaysESCPos(t *testing.T) {
	spooler := usbSpooler()
	job := formattedJob("cups")
	job.PrinterSnapshot.Destination = "POS_80"
	job.PrinterSnapshot.AcceptsESCPos = false
	job.Rasters = map[string]*documents.Raster{"logo-80": {WidthDots: 16, HeightDots: 2, Bits: []byte{0xff, 0x00, 0x0f, 0xf0}}}
	outcome := HostTransport{spooler: spooler}.Submit(context.Background(), job)
	if outcome.State != "submitted" || strings.Contains(outcome.Message, "logotipo") || strings.Contains(outcome.Message, "sin formato") {
		t.Fatalf("El documento con formato quedó mal registrado: %+v", outcome)
	}
	single := documents.EncodeESCPos(job.Layout, job.Rasters, legacyFullCut)
	if !bytes.Equal(spooler.written, append(append([]byte{}, single...), single...)) {
		t.Fatalf("No se entregaron los bytes ESC/POS del layout dos veces: % x", spooler.written)
	}
	if !bytes.Contains(spooler.written, []byte{0x1d, 0x76, 0x30, 0x00, 0x02, 0x00, 0x02, 0x00, 0xff, 0x00, 0x0f, 0xf0}) {
		t.Fatal("La imagen no viajó con GS v 0")
	}
	// Sin los bits del logotipo se omite con aviso y el documento se acepta igual.
	missing := usbSpooler()
	job.Rasters = nil
	withoutLogo := HostTransport{spooler: missing}.Submit(context.Background(), job)
	if withoutLogo.State != "submitted" || !strings.HasSuffix(withoutLogo.Message, missingLogoNote) {
		t.Fatalf("Sin los bits del logotipo no se avisó: %+v", withoutLogo)
	}
	if !bytes.Contains(missing.written, documents.EncodeESCPos(job.Layout, nil, legacyFullCut)) {
		t.Fatal("Sin logotipo no se entregó el resto del documento")
	}
}

// Resultado de cada falla: antes de que Windows acepte iniciar el documento nada se envió (error);
// después, cualquier falla o escritura incompleta es incierta y nunca se da por aceptada.
func TestWindowsQueueOutcomeMapping(t *testing.T) {
	cases := []struct {
		name      string
		configure func(*fakeSpooler)
		state     string
		message   string
		aborted   bool
		started   bool
	}{
		{"sin abrir la impresora", func(spooler *fakeSpooler) { spooler.openError = errors.New("acceso") },
			"error", "No se pudo abrir la impresora POS_80 en Windows; no se envió el documento.", false, false},
		{"sin iniciar el documento", func(spooler *fakeSpooler) { spooler.startError = errors.New("tipo de datos") },
			"error", "Windows no aceptó iniciar el documento en la impresora POS_80; no se envió el documento.", false, true},
		{"sin iniciar la página", func(spooler *fakeSpooler) { spooler.pageError = errors.New("página") },
			"uncertain", "no confirmó el documento completo", true, true},
		{"falla en la segunda copia", func(spooler *fakeSpooler) { spooler.failWriteCopy = 2 },
			"uncertain", "durante la copia 2", true, true},
		{"escritura incompleta", func(spooler *fakeSpooler) { spooler.shortCopy = 1 },
			"uncertain", "durante la copia 1", true, true},
		{"sin terminar la página", func(spooler *fakeSpooler) { spooler.endPageError = errors.New("página") },
			"uncertain", "no confirmó el documento completo", true, true},
		{"sin terminar el documento", func(spooler *fakeSpooler) { spooler.endDocError = errors.New("fin") },
			"uncertain", "no confirmó el documento completo", false, true},
	}
	for _, test := range cases {
		spooler := usbSpooler()
		test.configure(spooler)
		outcome := HostTransport{spooler: spooler}.Submit(context.Background(), windowsTextJob(2))
		calls := strings.Join(spooler.recorded(), "|")
		if outcome.State != test.state || outcome.Accepted || outcome.Reference != "" || !strings.Contains(outcome.Message, test.message) {
			t.Fatalf("%s: resultado %+v", test.name, outcome)
		}
		if strings.Contains(calls, "abort") != test.aborted || strings.Contains(calls, "start ") != test.started {
			t.Fatalf("%s: llamadas %s", test.name, calls)
		}
		if test.state == "error" && len(spooler.written) != 0 {
			t.Fatalf("%s: se escribieron bytes en un envío que no se inició", test.name)
		}
		// Una impresora que se abrió siempre se cierra, termine como termine el envío.
		if spooler.openError == nil && !strings.HasSuffix(calls, "close") {
			t.Fatalf("%s: la impresora quedó abierta: %s", test.name, calls)
		}
		if test.state == "uncertain" && !strings.Contains(outcome.Message, "antes de autorizar otra copia") {
			t.Fatalf("%s: el mensaje incierto no pide revisar antes de otra copia: %q", test.name, outcome.Message)
		}
	}
}

// Antes de enviar se consulta la impresora: una que no existe queda en error y una que no entrega
// los datos sin alterar (virtual, archivo, red…) queda en intervención, sin abrirla en ambos casos.
func TestWindowsQueueInspectsBeforeSending(t *testing.T) {
	missing := usbSpooler()
	job := windowsTextJob(1)
	job.PrinterSnapshot.Destination = "NO_EXISTE"
	outcome := HostTransport{spooler: missing}.Submit(context.Background(), job)
	if outcome.State != "error" || !strings.Contains(outcome.Message, "no está instalada") ||
		!strings.HasSuffix(outcome.Message, "No se envió el documento.") {
		t.Fatalf("Una impresora inexistente debe quedar en error sin envío: %+v", outcome)
	}
	if strings.Contains(strings.Join(missing.recorded(), "|"), "open") {
		t.Fatal("Se abrió una impresora inexistente")
	}
	virtual := usbSpooler()
	virtual.printers = append(virtual.printers, spoolerPrinter{Name: "Oficina", Port: "PORTPROMPT:", Driver: "Microsoft XPS Document Writer v4"})
	job.PrinterSnapshot.Destination = "Oficina"
	outcome = HostTransport{spooler: virtual}.Submit(context.Background(), job)
	if outcome.State != "intervention" || !strings.Contains(outcome.Message, "virtual") ||
		!strings.HasSuffix(outcome.Message, "No se envió el documento.") {
		t.Fatalf("Una impresora virtual debe quedar en intervención sin envío: %+v", outcome)
	}
	if strings.Contains(strings.Join(virtual.recorded(), "|"), "open") {
		t.Fatal("Se abrió una impresora virtual para enviarle ESC/POS")
	}
	// También con un documento con formato.
	formatted := formattedJob("cups")
	formatted.PrinterSnapshot.Destination = "Oficina"
	if outcome := (HostTransport{spooler: virtual}).Submit(context.Background(), formatted); outcome.State != "intervention" {
		t.Fatalf("Un documento con formato llegó a una impresora virtual: %+v", outcome)
	}
}

// Los límites de tiempo: si vencen antes de iniciar el documento, el envío se abandona y no llega a
// iniciarse (error, nada enviado); si vencen después de iniciarlo, el resultado es incierto.
func TestWindowsQueueTimeouts(t *testing.T) {
	before := usbSpooler()
	before.blockOpen = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	outcome := spoolRaw(ctx, before, rawDocument{queue: "POS_80", title: "ADEL-1", data: []byte("x"), copies: 1, texts: documentTexts("POS_80")})
	cancel()
	if outcome.State != "error" || !strings.Contains(outcome.Message, "no respondió a tiempo") {
		t.Fatalf("Un límite vencido antes de iniciar debe ser un error sin envío: %+v", outcome)
	}
	close(before.blockOpen)
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(strings.Join(before.recorded(), "|"), "close") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if calls := strings.Join(before.recorded(), "|"); strings.Contains(calls, "start") || !strings.Contains(calls, "close") {
		t.Fatalf("El envío abandonado llegó a iniciar el documento: %s", calls)
	}

	after := usbSpooler()
	after.blockWrite = make(chan struct{})
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	outcome = spoolRaw(ctx, after, rawDocument{queue: "POS_80", title: "ADEL-2", data: []byte("x"), copies: 1, texts: documentTexts("POS_80")})
	cancel()
	close(after.blockWrite)
	if outcome.State != "uncertain" || outcome.Accepted {
		t.Fatalf("Un límite vencido después de iniciar debe ser incierto: %+v", outcome)
	}

	// Un contexto ya vencido no abre la impresora.
	expired, stop := context.WithCancel(context.Background())
	stop()
	untouched := usbSpooler()
	if outcome := spoolRaw(expired, untouched, rawDocument{queue: "POS_80", title: "ADEL-3", data: []byte("x"), copies: 1,
		texts: documentTexts("POS_80")}); outcome.State != "error" || len(untouched.recorded()) != 0 {
		t.Fatalf("Con el contexto vencido no debe intentarse el envío: %+v %v", outcome, untouched.recorded())
	}

	// La consulta de una impresora que no responde termina con el contexto, sin enviar nada.
	slow := usbSpooler()
	slow.blockPrinter = make(chan struct{})
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	capability := inspectWindowsQueue(ctx, slow, "POS_80")
	cancel()
	close(slow.blockPrinter)
	if capability.Checked || capability.Raw || !strings.Contains(capability.Reason, "no responde") {
		t.Fatalf("Una impresora que no responde debe quedar sin consultar: %+v", capability)
	}
}

// El pulso del cajón: una sola copia de ESC p, con su título técnico y el mensaje que nunca afirma
// que el cajón se abrió; una impresora que no entrega los datos sin alterar no recibe nada.
func TestWindowsQueueDrawerPulse(t *testing.T) {
	spooler := usbSpooler()
	job := PrintJob{
		ID:              "00000000-0000-4000-8000-0000000009e2",
		DrawerPulse:     &DrawerPulse{Pin: 1},
		PrinterSnapshot: Printer{Name: "Caja", Transport: "cups", Destination: "POS_80", PaperWidth: 80, Copies: 2, Port: 9100},
	}
	outcome := HostTransport{spooler: spooler}.Submit(context.Background(), job)
	if outcome.State != "submitted" || !outcome.Accepted || outcome.Reference != "windows-17" ||
		outcome.Message != "Comando de apertura aceptado por la cola de impresión de Windows; no confirma que el cajón se abrió." {
		t.Fatalf("El pulso aceptado quedó mal registrado: %+v", outcome)
	}
	if !bytes.Equal(spooler.written, DrawerPulseBytes(1)) || spooler.titles[0] != "ADEL-cajon-"+job.ID {
		t.Fatalf("El pulso no es una sola copia de ESC p: % x %v", spooler.written, spooler.titles)
	}
	if strings.Contains(outcome.Message, "cajón abierto") || strings.Contains(outcome.Message, "se abrió el cajón") {
		t.Fatal("El mensaje afirma que el cajón se abrió")
	}

	virtual := usbSpooler()
	job.PrinterSnapshot.Destination = "Microsoft Print to PDF"
	outcome = HostTransport{spooler: virtual}.submitDrawerPulse(context.Background(), job)
	if outcome.State != "intervention" || !strings.HasSuffix(outcome.Message, "No se envió el comando de apertura.") {
		t.Fatalf("El pulso a una impresora virtual debe quedar en intervención: %+v", outcome)
	}
	if strings.Contains(strings.Join(virtual.recorded(), "|"), "open") {
		t.Fatal("Se abrió una impresora virtual para el pulso")
	}

	missing := usbSpooler()
	job.PrinterSnapshot.Destination = "NO_EXISTE"
	outcome = HostTransport{spooler: missing}.submitDrawerPulse(context.Background(), job)
	if outcome.State != "error" || !strings.HasSuffix(outcome.Message, "No se envió el comando de apertura.") {
		t.Fatalf("El pulso a una impresora inexistente debe quedar en error: %+v", outcome)
	}

	interrupted := usbSpooler()
	interrupted.failWriteCopy = 1
	job.PrinterSnapshot.Destination = "POS_80"
	outcome = HostTransport{spooler: interrupted}.submitDrawerPulse(context.Background(), job)
	if outcome.State != "uncertain" || !strings.Contains(outcome.Message, "revisa el cajón") {
		t.Fatalf("Un pulso interrumpido debe ser incierto: %+v", outcome)
	}
}

// Clasificación de impresoras y puertos de Windows: solo las locales con puerto USB, serie,
// paralelo o TCP/IP estándar entregan los datos sin alterar; las demás, con su motivo en español.
func TestWindowsPrinterClassification(t *testing.T) {
	ports := []spoolerPort{
		{Name: "USB001", Monitor: "USB Monitor"},
		{Name: "COM1:", Monitor: "Local Monitor"},
		{Name: "LPT1:", Monitor: "Local Port"},
		{Name: "IP_192.168.1.80", Monitor: "Standard TCP/IP Port"},
		{Name: "WSD-7c1a", Monitor: "WSD Port"},
		{Name: "ESDPRT001", Monitor: "EPSON Port Monitor"},
		{Name: `C:\tickets\salida.prn`, Monitor: "Local Monitor"},
		{Name: "FILE:", Monitor: "Local Monitor"},
		{Name: "COM2:", Monitor: "Otro monitor"},
	}
	cases := []struct {
		info       spoolerPrinter
		raw        bool
		connection string
		reason     string
	}{
		{spoolerPrinter{Name: "Tickets", Port: "USB001", Driver: "Generic / Text Only"}, true, "usb", ""},
		{spoolerPrinter{Name: "Tickets", Port: "USB002", Driver: "POS-80C"}, true, "usb", ""},
		{spoolerPrinter{Name: "Tickets", Port: "COM1:", Driver: "Generic / Text Only"}, true, "serial", ""},
		{spoolerPrinter{Name: "Tickets", Port: "LPT1:", Driver: "Generic / Text Only"}, true, "parallel", ""},
		{spoolerPrinter{Name: "Tickets", Port: "IP_192.168.1.80", Driver: "Generic / Text Only"}, true, "socket", ""},
		{spoolerPrinter{Name: "Tickets", Port: "FILE:", Driver: "Generic / Text Only"}, false, "", "archivo"},
		{spoolerPrinter{Name: "Tickets", Port: "nul:", Driver: "Generic / Text Only"}, false, "", "archivo"},
		{spoolerPrinter{Name: "Microsoft Print to PDF", Port: "PORTPROMPT:", Driver: "Microsoft Print To PDF"}, false, "", "virtual"},
		{spoolerPrinter{Name: "Documentos", Port: "PORTPROMPT:", Driver: "Microsoft XPS Document Writer v4"}, false, "", "virtual"},
		{spoolerPrinter{Name: "OneNote (Desktop)", Port: "nul:", Driver: "Send to Microsoft OneNote 16 Driver"}, false, "", "virtual"},
		{spoolerPrinter{Name: "Fax", Port: "SHRFAX:", Driver: "Microsoft Shared Fax Driver", Attributes: printerAttributeFax}, false, "", "fax"},
		{spoolerPrinter{Name: "Tickets", Port: "WSD-7c1a", Driver: "Generic / Text Only"}, false, "", "servicios web"},
		{spoolerPrinter{Name: "Tickets", Port: "USB001", Driver: "Generic / Text Only", Attributes: printerAttributeNetwork}, false, "", "otro equipo"},
		{spoolerPrinter{Name: `\\caja\Tickets`, Port: "USB001", Driver: "Generic / Text Only"}, false, "", "otro equipo"},
		{spoolerPrinter{Name: "Tickets", Server: `\\caja`, Port: "USB001", Driver: "Generic / Text Only"}, false, "", "otro equipo"},
		{spoolerPrinter{Name: "Tickets", Port: "USB001", Driver: "Generic / Text Only", Attributes: printerAttributeTerminalServer}, false, "", "otro equipo"},
		{spoolerPrinter{Name: "Tickets", Port: "USB001,USB002", Driver: "Generic / Text Only"}, false, "", "varios puertos"},
		{spoolerPrinter{Name: "Tickets", Port: "ESDPRT001", Driver: "EPSON TM-T20II Receipt5"}, false, "", "puerto ESDPRT001, EPSON Port Monitor"},
		{spoolerPrinter{Name: "Tickets", Port: `C:\tickets\salida.prn`, Driver: "Generic / Text Only"}, false, "", `puerto C:\tickets\salida.prn, Local Monitor`},
		{spoolerPrinter{Name: "Tickets", Port: "COM2:", Driver: "Generic / Text Only"}, false, "", "puerto COM2:, Otro monitor"},
		{spoolerPrinter{Name: "Tickets", Port: "", Driver: "Generic / Text Only"}, false, "", "no informa el puerto"},
	}
	for _, test := range cases {
		capability := evaluateWindowsPrinter("Tickets", test.info, ports)
		if !capability.Checked || capability.Raw != test.raw || capability.Connection != test.connection ||
			!strings.Contains(capability.Reason, test.reason) || capability.Model != test.info.Driver {
			t.Fatalf("%s en %q: %+v", test.info.Name, test.info.Port, capability)
		}
		if test.raw != (capability.Reason == "") {
			t.Fatalf("%q: el motivo debe existir solo cuando no se puede usar: %+v", test.info.Port, capability)
		}
	}
	// Sin la información del puerto (por ejemplo, un USB que Windows aún no enumera) basta su nombre.
	if capability := evaluateWindowsPrinter("Tickets", spoolerPrinter{Name: "Tickets", Port: "USB003"}, nil); !capability.Raw {
		t.Fatalf("Un puerto USB sin monitor informado debe admitirse: %+v", capability)
	}
	// Un inspector que no responde con los puertos queda sin consultar.
	broken := usbSpooler()
	broken.portsError = errors.New("puertos")
	if capability := inspectWindowsQueue(context.Background(), broken, "POS_80"); capability.Checked {
		t.Fatalf("Sin los puertos no se puede clasificar la impresora: %+v", capability)
	}
	// Un nombre inválido no llega al administrador de impresión.
	invalidName := usbSpooler()
	if capability := inspectWindowsQueue(context.Background(), invalidName, `\\caja\Tickets`); capability.Checked ||
		len(invalidName.recorded()) != 0 {
		t.Fatalf("Un nombre inválido no debe consultarse: %+v", capability)
	}
}

// Nombres de impresoras de Windows: con espacios, acentos y guiones; sin barras invertidas, comas,
// comillas, controles ni espacios en los extremos, y de 1 a 200 caracteres.
func TestWindowsPrinterNameValidation(t *testing.T) {
	accepted := []string{"EPSON TM-T20II Receipt", "Impresora de cocina ñandú", "POS-80C (copia 1)", "a", strings.Repeat("é", 200)}
	for _, name := range accepted {
		if err := validateWindowsPrinterName(name); err != nil {
			t.Fatalf("Se rechazó %q: %v", name, err)
		}
	}
	rejected := []string{"", " Tickets", "Tickets ", `\\caja\Tickets`, "Tickets,Job 5", `Tickets "USB"`, "Tickets\tUSB",
		"Tickets\x00", strings.Repeat("a", 201), "\xff\xfe"}
	for _, name := range rejected {
		var problem *Error
		err := validateWindowsPrinterName(name)
		if !errors.As(err, &problem) || problem.Status != 400 || !strings.Contains(problem.Message, "impresora instalada en Windows") {
			t.Fatalf("Se aceptó %q: %v", name, err)
		}
	}
	// La opción de corte de CUPS que llegue de una configuración anterior se sigue validando.
	if err := validateWindowsQueue(&Printer{Destination: "Tickets", CutterOption: "CutMedia=1"}); err != nil {
		t.Fatalf("Se rechazó una opción de corte válida: %v", err)
	}
	if err := validateWindowsQueue(&Printer{Destination: "Tickets", CutterOption: "-o raw"}); err == nil {
		t.Fatal("Se aceptó una opción de corte inválida")
	}
}

// La lista para configurar: ordenada por nombre, sin conexiones a impresoras de otro equipo, con un
// resumen en español, sin opciones de controlador y acotada.
func TestWindowsPrinterDiscovery(t *testing.T) {
	spooler := &fakeSpooler{
		printers: []spoolerPrinter{
			{Name: "Zeta tickets", Port: "USB001", Driver: "Generic / Text Only", Status: 0x10, Jobs: 2},
			{Name: `\\caja\Compartida`, Port: "USB001", Driver: "Generic / Text Only", Attributes: printerAttributeNetwork},
			{Name: "Microsoft Print to PDF", Port: "PORTPROMPT:", Driver: "Microsoft Print To PDF"},
			{Name: "alfa serie", Port: "COM1:", Driver: "Generic / Text Only", Attributes: printerAttributeWorkOffline},
		},
		portList: []spoolerPort{{Name: "USB001", Monitor: "USB Monitor"}, {Name: "COM1:", Monitor: "Local Monitor"}},
	}
	discovered, err := discoverWindowsPrinters(context.Background(), spooler)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, item := range discovered {
		names = append(names, item.Queue)
		if item.Options != "" {
			t.Fatalf("Windows no anuncia opciones de controlador: %+v", item)
		}
	}
	if strings.Join(names, "|") != "alfa serie|Microsoft Print to PDF|Zeta tickets" {
		t.Fatalf("Lista de impresoras: %q", names)
	}
	if discovered[0].Status != "Puerto COM1: (puerto serie) · Controlador Generic / Text Only · Windows informa: en uso sin conexión. "+
		"Recibe los comandos ESC/POS sin alterar." {
		t.Fatalf("Resumen de la impresora serie: %q", discovered[0].Status)
	}
	if !strings.HasPrefix(discovered[1].Status, "Puerto PORTPROMPT: · Controlador Microsoft Print To PDF · Windows no informa problemas. No se puede usar: ") {
		t.Fatalf("Resumen de la impresora PDF: %q", discovered[1].Status)
	}
	if discovered[2].Status != "Puerto USB001 (USB) · Controlador Generic / Text Only · Windows informa: sin papel · 2 trabajos en espera. "+
		"Recibe los comandos ESC/POS sin alterar." {
		t.Fatalf("Resumen de la impresora USB: %q", discovered[2].Status)
	}

	many := &fakeSpooler{portList: spooler.portList}
	for index := 0; index < 70; index++ {
		many.printers = append(many.printers, spoolerPrinter{Name: fmt.Sprintf("Tickets %02d", index), Port: "USB001"})
	}
	if discovered, err := discoverWindowsPrinters(context.Background(), many); err != nil || len(discovered) != maxDiscoveredPrinters {
		t.Fatalf("La lista no se acotó a %d: %d %v", maxDiscoveredPrinters, len(discovered), err)
	}

	for _, broken := range []*fakeSpooler{{listError: errors.New("sin servicio")}, {portsError: errors.New("sin puertos")}} {
		_, err := discoverWindowsPrinters(context.Background(), broken)
		var problem *Error
		if !errors.As(err, &problem) || problem.Status != 503 || problem.Code != "SPOOLER_UNAVAILABLE" ||
			!strings.Contains(problem.Message, "impresoras instaladas en Windows") {
			t.Fatalf("Sin el administrador de impresión debe responder 503: %v", err)
		}
	}
}

// Qué cola usa cada transporte e inspector: los sustitutos de CUPS ejercitan siempre CUPS y el
// administrador simulado, siempre Windows, en cualquier sistema.
func TestSystemQueueSelection(t *testing.T) {
	spooler := usbSpooler()
	if _, windows := (HostTransport{commands: &recordedCommands{t: t, forbidden: true}}).windowsSpooler(); windows {
		t.Fatal("Con los programas de CUPS sustituidos el transporte debe usar CUPS")
	}
	if _, windows := (HostTransport{textCUPS: func(context.Context, PrintJob) Outcome { return Outcome{} }}).windowsSpooler(); windows {
		t.Fatal("Con el texto de CUPS sustituido el transporte debe usar CUPS")
	}
	if selected, windows := (HostTransport{spooler: spooler}).windowsSpooler(); !windows || selected != spooler {
		t.Fatal("Con el administrador simulado el transporte debe usar Windows")
	}
	inspector := HostQueueInspector{spooler: spooler}
	if inspector.SystemQueue() != SystemQueueWindows || SystemQueueOf(inspector) != SystemQueueWindows {
		t.Fatal("El inspector con el administrador simulado consulta Windows")
	}
	if capability := inspector.InspectQueue(context.Background(), "POS_80"); !capability.Raw || capability.Connection != "usb" {
		t.Fatalf("El inspector no consultó la cola de Windows: %+v", capability)
	}
	cups := HostQueueInspector{commands: &recordedCommands{t: t, forbidden: true}}
	if cups.SystemQueue() != SystemQueueCUPS || SystemQueueOf(cups) != SystemQueueCUPS {
		t.Fatal("El inspector con los programas de CUPS sustituidos consulta CUPS")
	}
	if SystemQueueOf(&designerLikeInspector{}) != SystemQueueCUPS {
		t.Fatal("Un inspector que no declara su cola se trata como CUPS")
	}
}

// designerLikeInspector es un inspector simulado como los de otros paquetes: no declara su cola.
type designerLikeInspector struct{}

func (*designerLikeInspector) InspectQueue(_ context.Context, queue string) QueueCapability {
	return QueueCapability{Queue: queue}
}
