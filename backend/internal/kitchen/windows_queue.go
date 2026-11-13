package kitchen

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"local/ad-el-pos/server/internal/plural"
)

// Cola de impresión de Windows (etapa 9E, §3.5). En Windows el transporte guardado como «cups» (la
// «cola de impresión del equipo») no usa CUPS: AD-EL POS entrega los bytes ESC/POS en modo RAW a la
// cola de la impresora instalada en Windows con su administrador de impresión (winspool.drv), sin
// migración ni cambios en la cola durable, el lease, la serialización por impresora ni las reglas de
// resultado incierto. ESC/POS por red (escpos_tcp) no cambia.
//
//   - Una cola de Windows recibe siempre ESC/POS sin convertir: no hay un camino de texto como el de
//     CUPS. Por eso, antes de cada envío (documentos y cajón) se consulta la impresora, solo con
//     lecturas, y solo se usa si es local y su puerto entrega los datos tal cual: USB, serie,
//     paralelo o un puerto TCP/IP estándar. Las impresoras de otro equipo, las virtuales (PDF, XPS,
//     OneNote, fax), los puertos de archivo y los de servicios web (WSD) se rechazan con su motivo.
//   - La marca «Esta impresora entiende ESC/POS» y la opción de corte de CUPS no aplican: el corte es
//     el comando ESC/POS GS V cuando la impresora lo admite y se solicita.
//   - Resultado: si algo falla antes de que Windows acepte iniciar el documento, nada se envió
//     (error); si falla después, o la escritura queda incompleta, el resultado es incierto y nunca
//     se reintenta solo; si Windows acepta el documento completo, se informa que la cola lo aceptó,
//     nunca que salió el papel.
//
// Este archivo no depende de Windows (las pruebas lo ejercitan con un administrador simulado en
// cualquier sistema); las llamadas reales a winspool.drv están en winspool_windows.go.

// spoolerAPI es la parte del administrador de impresión de Windows que usa AD-EL POS. La
// implementación real (winspool_windows.go) llama a winspool.drv; las pruebas usan una simulada
// que nunca toca impresoras reales.
// errPrinterNotInstalled es el error de spoolerAPI.printer cuando Windows no tiene instalada una impresora con
// ese nombre (ERROR_INVALID_PRINTER_NAME de OpenPrinterW; revisión 9F, B4): se distingue de un administrador
// de impresión que no responde.
var errPrinterNotInstalled = errors.New("la impresora no está instalada en Windows")

type spoolerAPI interface {
	// listPrinters enumera las impresoras locales y las conexiones (EnumPrintersW, nivel 2).
	listPrinters() ([]spoolerPrinter, error)
	// printer lee una impresora por su nombre (OpenPrinterW y GetPrinterW, nivel 2).
	printer(name string) (spoolerPrinter, error)
	// ports enumera los puertos de impresión con su monitor (EnumPortsW, nivel 2).
	ports() ([]spoolerPort, error)
	// open abre la impresora para enviarle un documento (OpenPrinterW).
	open(name string) (spoolerHandle, error)
}

// spoolerHandle es una impresora abierta para enviarle un documento RAW.
type spoolerHandle interface {
	// startDocument inicia un documento RAW con su título técnico (StartDocPrinterW) y devuelve
	// el número de trabajo que asignó Windows.
	startDocument(title string) (uint32, error)
	startPage() error
	// write entrega bytes al documento (WritePrinter) y devuelve cuántos aceptó Windows.
	write(data []byte) (int, error)
	endPage() error
	endDocument() error
	// abort descarta el documento en curso (AbortPrinter) para no imprimir uno incompleto.
	abort() error
	close() error
}

// spoolerPrinter son los datos de una impresora instalada en Windows (PRINTER_INFO_2).
type spoolerPrinter struct {
	Server     string
	Name       string
	Port       string
	Driver     string
	Attributes uint32
	Status     uint32
	Jobs       uint32
}

// spoolerPort es un puerto de impresión de Windows con su monitor (PORT_INFO_2).
type spoolerPort struct {
	Name    string
	Monitor string
}

// Atributos de PRINTER_INFO_2 que impiden asegurar a qué dispositivo llegan los bytes.
const (
	printerAttributeNetwork         = 0x00000010
	printerAttributeWorkOffline     = 0x00000400
	printerAttributeFax             = 0x00004000
	printerAttributeTerminalServer  = 0x00008000
	printerAttributeEnterpriseCloud = 0x00800000
)

// printerStatusTexts traduce los bits de estado de PRINTER_INFO_2 que Windows informa de la
// impresora. Muchas impresoras USB no informan nada aunque estén desconectadas.
var printerStatusTexts = []struct {
	bit  uint32
	text string
}{
	{0x00000001, "en pausa"},
	{0x00000002, "con error"},
	{0x00000004, "pendiente de eliminar"},
	{0x00000008, "papel atascado"},
	{0x00000010, "sin papel"},
	{0x00000040, "problema con el papel"},
	{0x00000080, "sin conexión"},
	{0x00000200, "ocupada"},
	{0x00000400, "imprimiendo"},
	{0x00001000, "no disponible"},
	{0x00100000, "requiere atención"},
	{0x00400000, "puerta abierta"},
}

// virtualPrinterMarkers identifican, en el nombre o el controlador, las impresoras que no imprimen
// en papel: generan archivos (PDF, XPS), envían a otra aplicación (OneNote) o son de fax.
var virtualPrinterMarkers = []string{"pdf", "xps", "onenote", "fax", "microsoft software printer"}

// fileOrNullPorts son los puertos de Windows que escriben en un archivo, preguntan dónde guardarlo o
// descartan los datos: ninguno llega a una impresora.
var fileOrNullPorts = map[string]bool{"FILE:": true, "PORTPROMPT:": true, "NUL:": true, "NUL": true}

var (
	usbPortPattern      = regexp.MustCompile(`^USB[0-9]{1,4}$`)
	serialPortPattern   = regexp.MustCompile(`^COM[0-9]{1,3}:?$`)
	parallelPortPattern = regexp.MustCompile(`^LPT[0-9]{1,3}:?$`)
)

// Monitores de puerto de Windows que entregan un trabajo RAW tal cual al dispositivo. Un puerto USB
// lo atiende el monitor USB (o el monitor dinámico de Windows 8 y posteriores); COM y LPT, el
// monitor de puertos locales («Local Monitor» en Windows 11, «Local Port» en versiones
// anteriores); un puerto TCP/IP estándar envía los bytes a la impresora de red. Un monitor vacío
// significa que Windows no lo informó para ese puerto.
var (
	usbMonitors   = []string{"", "usb monitor", "dynamic print monitor"}
	localMonitors = []string{"", "local monitor", "local port"}
	tcpMonitor    = "standard tcp/ip port"
)

// spoolerTimeout limita una consulta de la lista de impresoras de Windows (la de una sola
// impresora usa queueInspectionTimeout, como CUPS).
const spoolerTimeout = 5 * time.Second

// maxDiscoveredPrinters acota la lista de impresoras de Windows que se muestra al configurar.
const maxDiscoveredPrinters = 64

// validateWindowsPrinterName valida el nombre de una impresora instalada en Windows. Se entrega como
// texto a OpenPrinterW (nunca a un shell), pero se excluyen los caracteres que cambian lo que abre:
// la barra invertida (una impresora de otro equipo, «\\servidor\impresora»), la coma (Windows la usa
// para abrir un trabajo o un puerto, «Impresora,Job 5») y las comillas, además de los controles y
// los espacios al inicio o al final.
func validateWindowsPrinterName(name string) error {
	message := "Selecciona el nombre de una impresora instalada en Windows en el equipo servidor " +
		"(sin barras invertidas, comas ni comillas)."
	count := utf8.RuneCountInString(name)
	if !utf8.ValidString(name) || count < 1 || count > 200 {
		return invalid(message)
	}
	if strings.HasPrefix(name, " ") || strings.HasSuffix(name, " ") {
		return invalid(message)
	}
	for _, character := range name {
		if unicode.IsControl(character) || character == '\\' || character == ',' || character == '"' {
			return invalid(message)
		}
	}
	return nil
}

// validateWindowsQueue valida el destino de una impresora «cola de impresión del equipo» cuando el
// equipo servidor es Windows. La opción de corte de CUPS no se usa en Windows; si viene de una
// configuración anterior, debe seguir siendo una opción válida de CUPS.
func validateWindowsQueue(printer *Printer) error {
	if err := validateWindowsPrinterName(printer.Destination); err != nil {
		return err
	}
	if printer.CutterOption != "" && !cutterPattern.MatchString(printer.CutterOption) {
		return invalid("La opción de corte CUPS debe corresponder a CutMedia, CutType o Cutter del controlador.")
	}
	return nil
}

// evaluateWindowsPrinter decide, con los datos que informa Windows, si la impresora entrega los
// comandos ESC/POS sin convertirlos. El criterio es conservador, como el de CUPS: ante cualquier
// duda la impresora no se usa y se explica por qué.
func evaluateWindowsPrinter(queue string, info spoolerPrinter, ports []spoolerPort) QueueCapability {
	capability := QueueCapability{Queue: queue, Checked: true, Model: cleanText(info.Driver)}
	if info.Attributes&(printerAttributeNetwork|printerAttributeTerminalServer|printerAttributeEnterpriseCloud) != 0 ||
		strings.HasPrefix(info.Name, `\\`) || strings.TrimSpace(info.Server) != "" {
		capability.Reason = "La impresora " + queue + " pertenece a otro equipo de la red: ese equipo podría convertir los datos y " +
			"no se puede asegurar que entregue los comandos ESC/POS sin alterar. Instálala en el equipo servidor o usa la conexión ESC/POS por red."
		return capability
	}
	if info.Attributes&printerAttributeFax != 0 || virtualPrinter(info) {
		capability.Reason = "La impresora " + queue + " es una impresora virtual o de fax (" + capability.Model + "): no imprime tickets en papel."
		return capability
	}
	port := strings.TrimSpace(info.Port)
	upper := strings.ToUpper(port)
	switch {
	case port == "":
		capability.Reason = "Windows no informa el puerto de la impresora " + queue +
			"; no se puede asegurar que entregue los comandos ESC/POS sin alterar."
		return capability
	case strings.Contains(port, ","):
		capability.Reason = "La impresora " + queue + " usa varios puertos a la vez (grupo de impresoras): " +
			"no se puede asegurar a qué impresora llegan los datos."
		return capability
	case fileOrNullPorts[upper]:
		capability.Reason = "La impresora " + queue + " imprime en un archivo o en ningún dispositivo (puerto " +
			cleanText(port) + "), no en papel."
		return capability
	}
	monitorName := portMonitor(ports, upper)
	monitor := strings.ToLower(monitorName)
	switch {
	case strings.Contains(monitor, "wsd") || strings.HasPrefix(upper, "WSD"):
		capability.Reason = "La impresora " + queue + " se conecta por servicios web (WSD), que reciben documentos y no garantizan " +
			"datos sin alterar. Para una impresora de red usa la conexión ESC/POS por red."
		return capability
	case monitor == tcpMonitor:
		capability.Connection = "socket"
	case usbPortPattern.MatchString(upper) && slices.Contains(usbMonitors, monitor):
		capability.Connection = "usb"
	case serialPortPattern.MatchString(upper) && slices.Contains(localMonitors, monitor):
		capability.Connection = "serial"
	case parallelPortPattern.MatchString(upper) && slices.Contains(localMonitors, monitor):
		capability.Connection = "parallel"
	default:
		described := cleanText(port)
		if monitorName != "" {
			described += ", " + cleanText(monitorName)
		}
		capability.Reason = "La impresora " + queue + " usa el puerto " + described + ", del que no se puede asegurar que entregue " +
			"los comandos ESC/POS sin alterar. Usa un puerto USB (USB001), serie (COM), paralelo (LPT) o TCP/IP estándar, " +
			"o la conexión ESC/POS por red."
		return capability
	}
	capability.Raw = true
	return capability
}

// virtualPrinter indica si el nombre o el controlador corresponde a una impresora que no imprime en
// papel.
func virtualPrinter(info spoolerPrinter) bool {
	described := strings.ToLower(info.Name + " " + info.Driver)
	for _, marker := range virtualPrinterMarkers {
		if strings.Contains(described, marker) {
			return true
		}
	}
	return false
}

// portMonitor devuelve el monitor del puerto (vacío si Windows no lo informa).
func portMonitor(ports []spoolerPort, upperName string) string {
	for _, port := range ports {
		if strings.ToUpper(strings.TrimSpace(port.Name)) == upperName {
			return strings.TrimSpace(port.Monitor)
		}
	}
	return ""
}

// inspectWindowsQueue consulta la impresora y los puertos de Windows (solo lecturas) con un tiempo
// acotado: una impresora que no responde no debe retener la transacción ni el lock operativo de
// quien configura el cajón. La consulta que no termina a tiempo sigue en segundo plano y su
// resultado se descarta; nunca envía nada.
func inspectWindowsQueue(ctx context.Context, api spoolerAPI, queue string) QueueCapability {
	if validateWindowsPrinterName(queue) != nil {
		return QueueCapability{Queue: queue, Reason: "El nombre de la impresora instalada en Windows no es válido."}
	}
	unavailable := QueueCapability{
		Queue: queue,
		Reason: "No se pudo consultar la impresora " + queue + " en Windows: no está instalada en el equipo servidor " +
			"o el administrador de impresión no responde.",
	}
	limited, cancel := context.WithTimeout(ctx, queueInspectionTimeout)
	defer cancel()
	type reading struct {
		info  spoolerPrinter
		ports []spoolerPort
		err   error
	}
	result := make(chan reading, 1)
	go func() {
		info, err := api.printer(queue)
		if err != nil {
			result <- reading{err: err}
			return
		}
		ports, err := api.ports()
		result <- reading{info: info, ports: ports, err: err}
	}()
	select {
	case read := <-result:
		// Revisión 9F (B4): una impresora que Windows no tiene instalada (o se renombró) se configura; un
		// administrador de impresión que no responde se reintenta.
		if errors.Is(read.err, errPrinterNotInstalled) {
			return QueueCapability{Queue: queue, Missing: true,
				Reason: "La impresora " + queue + " no está instalada en Windows en el equipo servidor."}
		}
		if read.err != nil {
			return unavailable
		}
		return evaluateWindowsPrinter(queue, read.info, read.ports)
	case <-limited.Done():
		return unavailable
	}
}

// discoverWindowsPrinters lista las impresoras instaladas en Windows para elegirlas al configurar,
// con un resumen en español de su puerto, controlador y estado y si AD-EL POS puede usarlas. Solo se
// listan las que tienen un nombre válido (las conexiones a impresoras de otro equipo no).
func discoverWindowsPrinters(ctx context.Context, api spoolerAPI) ([]DiscoveredPrinter, error) {
	unavailable := &Error{Status: 503, Code: "SPOOLER_UNAVAILABLE", Message: "No se pudieron consultar las impresoras instaladas en Windows en el equipo servidor."}
	limited, cancel := context.WithTimeout(ctx, spoolerTimeout)
	defer cancel()
	type reading struct {
		printers []spoolerPrinter
		ports    []spoolerPort
		err      error
	}
	result := make(chan reading, 1)
	go func() {
		printers, err := api.listPrinters()
		if err != nil {
			result <- reading{err: err}
			return
		}
		ports, err := api.ports()
		result <- reading{printers: printers, ports: ports, err: err}
	}()
	var read reading
	select {
	case read = <-result:
	case <-limited.Done():
		return nil, unavailable
	}
	if read.err != nil {
		return nil, unavailable
	}
	installed := slices.Clone(read.printers)
	slices.SortStableFunc(installed, func(left, right spoolerPrinter) int {
		return strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	})
	discovered := []DiscoveredPrinter{}
	for _, info := range installed {
		if len(discovered) >= maxDiscoveredPrinters {
			break
		}
		if validateWindowsPrinterName(info.Name) != nil {
			continue
		}
		capability := evaluateWindowsPrinter(info.Name, info, read.ports)
		discovered = append(discovered, DiscoveredPrinter{Queue: info.Name, Status: windowsPrinterSummary(info, capability)})
	}
	return discovered, nil
}

// windowsPrinterSummary resume una impresora de Windows para la configuración: puerto, controlador,
// estado que informa Windows y si AD-EL POS puede enviarle los comandos ESC/POS sin alterar.
func windowsPrinterSummary(info spoolerPrinter, capability QueueCapability) string {
	connections := map[string]string{"usb": "USB", "serial": "puerto serie", "parallel": "puerto paralelo", "socket": "TCP/IP estándar"}
	port := "Puerto " + strings.TrimSpace(info.Port)
	if label := connections[capability.Connection]; label != "" {
		port += " (" + label + ")"
	}
	parts := []string{port}
	if driver := strings.TrimSpace(info.Driver); driver != "" {
		parts = append(parts, "Controlador "+driver)
	}
	parts = append(parts, windowsPrinterState(info))
	summary := strings.Join(parts, " · ") + "."
	if capability.Raw {
		return cleanText(summary + " Recibe los comandos ESC/POS sin alterar.")
	}
	return cleanText(summary + " No se puede usar: " + capability.Reason)
}

// windowsPrinterState describe el estado que informa Windows, sin afirmar que haya papel ni que la
// impresora esté conectada.
func windowsPrinterState(info spoolerPrinter) string {
	states := []string{}
	if info.Attributes&printerAttributeWorkOffline != 0 {
		states = append(states, "en uso sin conexión")
	}
	for _, status := range printerStatusTexts {
		if info.Status&status.bit != 0 {
			states = append(states, status.text)
		}
	}
	state := "Windows no informa problemas"
	if len(states) > 0 {
		state = "Windows informa: " + strings.Join(states, ", ")
	}
	if info.Jobs > 0 {
		state += " · " + plural.Count(int(info.Jobs), "trabajo en espera", "trabajos en espera")
	}
	return state
}

// rawTexts son los mensajes de un envío RAW: de un documento o del comando del cajón.
type rawTexts struct {
	openFailed  string
	startFailed string
	timedOut    string
	uncertain   string
	copyFailed  string // con el número de copia
	accepted    string
}

func documentTexts(queue string) rawTexts {
	return rawTexts{
		openFailed:  "No se pudo abrir la impresora " + queue + " en Windows; no se envió el documento.",
		startFailed: "Windows no aceptó iniciar el documento en la impresora " + queue + "; no se envió el documento.",
		timedOut:    "La impresora " + queue + " no respondió a tiempo en Windows; no se envió el documento.",
		uncertain: "La cola de impresión de Windows no confirmó el documento completo. " +
			"Revisa la impresora y su cola antes de autorizar otra copia.",
		copyFailed: "La cola de impresión de Windows se interrumpió durante la copia %d; " +
			"revisa la impresora y su cola antes de autorizar otra copia.",
		accepted: "Aceptado por la cola de impresión de Windows; no confirma salida física de papel.",
	}
}

func drawerTexts(queue string) rawTexts {
	return rawTexts{
		openFailed:  "No se pudo abrir la impresora " + queue + " del cajón en Windows; no se envió el comando de apertura.",
		startFailed: "Windows no aceptó iniciar el comando de apertura en la impresora " + queue + "; no se envió.",
		timedOut:    "La impresora " + queue + " del cajón no respondió a tiempo en Windows; no se envió el comando de apertura.",
		uncertain: "La cola de impresión de Windows no confirmó el comando de apertura; " +
			"revisa el cajón y la cola antes de volver a abrirlo.",
		copyFailed: "La cola de impresión de Windows se interrumpió al recibir el comando de apertura (copia %d); " +
			"revisa el cajón y la cola antes de volver a abrirlo.",
		accepted: "Comando de apertura aceptado por la cola de impresión de Windows; no confirma que el cajón se abrió.",
	}
}

// rawDocument es lo que se entrega a una cola de Windows como un solo documento RAW: los bytes se
// escriben una vez por copia.
type rawDocument struct {
	queue  string
	title  string
	data   []byte
	copies int
	texts  rawTexts
}

// spoolGuard coordina el envío con el límite de tiempo: si el límite vence antes de iniciar el
// documento, el envío se abandona y no llega a iniciarse (nada se envió); si vence después, el
// resultado es incierto.
type spoolGuard struct {
	mutex     sync.Mutex
	started   bool
	abandoned bool
}

// begin marca que el documento va a iniciarse; devuelve false si el envío ya se abandonó.
func (guard *spoolGuard) begin() bool {
	guard.mutex.Lock()
	defer guard.mutex.Unlock()
	if guard.abandoned {
		return false
	}
	guard.started = true
	return true
}

// abandon abandona el envío si el documento aún no se inició; devuelve false si ya se inició.
func (guard *spoolGuard) abandon() bool {
	guard.mutex.Lock()
	defer guard.mutex.Unlock()
	if guard.started {
		return false
	}
	guard.abandoned = true
	return true
}

// spoolRaw entrega un documento RAW a la cola de Windows dentro del tiempo del contexto. Las
// llamadas a winspool.drv no admiten cancelación: el envío corre aparte y, si el tiempo vence, el
// resultado depende de si el documento ya se había iniciado.
func spoolRaw(ctx context.Context, api spoolerAPI, document rawDocument) Outcome {
	if ctx.Err() != nil {
		return Outcome{State: "error", Message: document.texts.timedOut}
	}
	guard := &spoolGuard{}
	result := make(chan Outcome, 1)
	go func() {
		result <- spoolRawNow(api, document, guard)
	}()
	select {
	case outcome := <-result:
		return outcome
	case <-ctx.Done():
		// Si el envío terminó al mismo tiempo que el límite, vale su resultado.
		select {
		case outcome := <-result:
			return outcome
		default:
		}
		if guard.abandon() {
			return Outcome{State: "error", Message: document.texts.timedOut}
		}
		return Outcome{State: "uncertain", Message: document.texts.uncertain}
	}
}

// spoolRawNow hace el envío: abrir la impresora, iniciar el documento RAW y su página, escribir los
// bytes una vez por copia y terminar. Antes de que Windows acepte iniciar el documento nada se
// envió (error); después, cualquier falla o escritura incompleta es incierta, y el documento en
// curso se descarta para no imprimir uno incompleto.
func spoolRawNow(api spoolerAPI, document rawDocument, guard *spoolGuard) Outcome {
	handle, err := api.open(document.queue)
	if err != nil {
		return Outcome{State: "error", Message: document.texts.openFailed}
	}
	defer handle.close()
	if !guard.begin() {
		return Outcome{State: "error", Message: document.texts.timedOut}
	}
	jobID, err := handle.startDocument(document.title)
	if err != nil {
		return Outcome{State: "error", Message: document.texts.startFailed}
	}
	if err := handle.startPage(); err != nil {
		_ = handle.abort()
		return Outcome{State: "uncertain", Message: document.texts.uncertain}
	}
	for copyNumber := 1; copyNumber <= document.copies; copyNumber++ {
		written, err := handle.write(document.data)
		if err != nil || written != len(document.data) {
			_ = handle.abort()
			return Outcome{State: "uncertain", Message: fmt.Sprintf(document.texts.copyFailed, copyNumber)}
		}
	}
	if err := handle.endPage(); err != nil {
		_ = handle.abort()
		return Outcome{State: "uncertain", Message: document.texts.uncertain}
	}
	if err := handle.endDocument(); err != nil {
		return Outcome{State: "uncertain", Message: document.texts.uncertain}
	}
	return Outcome{
		State:     "submitted",
		Reference: "windows-" + strconv.FormatUint(uint64(jobID), 10),
		Accepted:  true,
		Message:   document.texts.accepted,
	}
}

// submitWindowsDocument entrega los bytes ESC/POS de un documento a la impresora de Windows del
// trabajo, una vez por copia, después de comprobar que entrega los datos sin alterar. Una impresora
// que no los entrega queda en intervención sin enviar nada: en Windows no hay un camino de texto.
func submitWindowsDocument(ctx context.Context, api spoolerAPI, job PrintJob, data []byte) Outcome {
	queue := job.PrinterSnapshot.Destination
	capability := inspectWindowsQueue(ctx, api, queue)
	if !capability.Checked {
		return Outcome{State: "error", Code: capability.unavailableCode(), Message: capability.Reason + " No se envió el documento."}
	}
	if !capability.Raw {
		return Outcome{State: "intervention", Message: capability.Reason + " No se envió el documento."}
	}
	return spoolRaw(ctx, api, rawDocument{
		queue:  queue,
		title:  "ADEL-" + job.ID,
		data:   data,
		copies: job.PrinterSnapshot.Copies,
		texts:  documentTexts(queue),
	})
}

// submitWindowsText entrega un documento de texto (todas las plantillas predeterminadas) con los
// mismos bytes ESC/POS que la conexión por red: CP850, texto grande de Cocina y corte GS V si la
// impresora lo admite y se solicita.
func submitWindowsText(ctx context.Context, api spoolerAPI, job PrintJob) Outcome {
	return submitWindowsDocument(ctx, api, job, ESCPosBytes(job.Content, job.PrinterSnapshot, job.TextSize))
}

// submitDrawerPulseWindows envía el pulso del cajón a la impresora de Windows. Antes vuelve a
// consultarla (su instalación puede haber cambiado desde que se guardó el cajón): si ya no entrega
// los datos sin alterar, el trabajo queda en intervención sin enviar nada. Un resultado incierto
// nunca se reintenta solo.
func submitDrawerPulseWindows(ctx context.Context, api spoolerAPI, job PrintJob) Outcome {
	queue := job.PrinterSnapshot.Destination
	capability := inspectWindowsQueue(ctx, api, queue)
	if !capability.Checked {
		return Outcome{State: "error", Code: capability.unavailableCode(), Message: capability.Reason + " No se envió el comando de apertura."}
	}
	if !capability.Raw {
		return Outcome{State: "intervention", Message: capability.Reason + " No se envió el comando de apertura."}
	}
	return spoolRaw(ctx, api, rawDocument{
		queue:  queue,
		title:  "ADEL-cajon-" + job.ID,
		data:   DrawerPulseBytes(job.DrawerPulse.Pin),
		copies: 1,
		texts:  drawerTexts(queue),
	})
}
