package kitchen

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"

	"local/ad-el-pos/server/internal/documents"
)

type Outcome struct {
	State             string `json:"state"`
	Reference         string `json:"reference"`
	Message           string `json:"message"`
	Accepted          bool   `json:"accepted"`
	PhysicalConfirmed bool   `json:"physical_confirmed"`
}
type Transport interface {
	Submit(context.Context, PrintJob) Outcome
}

// HostTransport entrega los trabajos con los medios del equipo donde corre Go: CUPS local
// (lpstat, lpoptions y lp con argumentos separados) o una conexión ESC/POS directa. commands
// queda vacío en producción; las pruebas unitarias lo sustituyen para verificar los argumentos
// de CUPS sin ejecutar nada.
type HostTransport struct {
	commands commandRunner
	// textCUPS sustituye en las pruebas el camino de texto de CUPS (el de antes de la 9B), que
	// ejecuta lpstat, lpoptions y lp directamente; nil usa submitCUPS.
	textCUPS func(context.Context, PrintJob) Outcome
}

func (transport HostTransport) runner() commandRunner {
	if transport.commands == nil {
		return hostCommands{}
	}
	return transport.commands
}

type DiscoveredPrinter struct {
	Queue   string `json:"queue"`
	Status  string `json:"status"`
	Options string `json:"options"`
}

func DiscoverPrinters(ctx context.Context) ([]DiscoveredPrinter, error) {
	command := exec.CommandContext(ctx, "/usr/bin/lpstat", "-a")
	command.Env = append(os.Environ(), "LC_ALL=C")
	output, err := command.Output()
	if err != nil {
		return nil, &Error{Status: 503, Code: "CUPS_UNAVAILABLE", Message: "No se pudieron consultar las colas CUPS del host del servidor."}
	}
	printers := []DiscoveredPrinter{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 1 || !queuePattern.MatchString(fields[0]) {
			continue
		}
		options, _ := exec.CommandContext(ctx, "/usr/bin/lpoptions", "-p", fields[0], "-l").Output()
		if len(options) > 32768 {
			options = options[:32768]
		}
		printers = append(printers, DiscoveredPrinter{Queue: fields[0], Status: cleanText(line), Options: cleanText(string(options))})
	}
	return printers, nil
}

func (transport HostTransport) Submit(ctx context.Context, job PrintJob) Outcome {
	if err := validatePrinter(&job.PrinterSnapshot); err != nil {
		return Outcome{State: "intervention", Message: "El destino guardado requiere revisión."}
	}
	if job.DrawerPulse != nil {
		return submitDrawerPulse(ctx, job, transport.runner())
	}
	// Etapa 9B: un documento con negrita, tamaños o logotipo viaja con sus atributos; un documento
	// plano (todas las plantillas predeterminadas) sigue exactamente el camino anterior.
	if job.Layout != nil && !job.Layout.Plain() {
		return transport.submitFormatted(ctx, job)
	}
	switch job.PrinterSnapshot.Transport {
	case "cups":
		return submitCUPS(ctx, job)
	case "escpos_tcp":
		return submitESCPos(ctx, job)
	default:
		return Outcome{State: "intervention", Message: "El destino requiere un agente de impresión futuro."}
	}
}

func submitCUPS(ctx context.Context, job PrintJob) Outcome {
	printer := job.PrinterSnapshot
	probe := exec.CommandContext(ctx, "/usr/bin/lpstat", "-p", printer.Destination)
	probe.Env = append(os.Environ(), "LC_ALL=C")
	if err := probe.Run(); err != nil {
		return Outcome{State: "error", Message: "No se pudo consultar la cola CUPS del servidor; no se envió el documento."}
	}
	if printer.Cut {
		if printer.CutterOption == "" {
			return Outcome{State: "intervention", Message: "Configura la opción de corte que anuncia el controlador CUPS."}
		}
		key, value, _ := strings.Cut(printer.CutterOption, "=")
		output, err := exec.CommandContext(ctx, "/usr/bin/lpoptions", "-p", printer.Destination, "-l").Output()
		supported := false
		for _, line := range strings.Split(string(output), "\n") {
			label, choices, found := strings.Cut(line, ":")
			optionName, _, _ := strings.Cut(label, "/")
			if found && optionName == key {
				for _, choice := range strings.Fields(choices) {
					if strings.TrimPrefix(choice, "*") == value {
						supported = true
					}
				}
			}
		}
		if err != nil || !supported {
			return Outcome{State: "intervention", Message: "El controlador no confirma esa opción de corte; no se envió el documento."}
		}
	}
	file, err := os.CreateTemp("", "adel-print-*.txt")
	if err != nil {
		return Outcome{State: "error", Message: "No se pudo preparar el archivo privado; no se envió el documento."}
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(job.Content); err != nil {
		file.Close()
		return Outcome{State: "error", Message: "No se pudo escribir el documento local."}
	}
	if err := file.Close(); err != nil {
		return Outcome{State: "error", Message: "No se pudo finalizar el documento local."}
	}
	command := exec.CommandContext(ctx, "/usr/bin/lp", cupsTextArguments(job, file.Name())...)
	command.Env = append(os.Environ(), "LC_ALL=C", "CHARSET=utf-8")
	// Una vez iniciado lp, un timeout o error genérico puede ocultar aceptación del spooler.
	output, err := command.CombinedOutput()
	reference := cupsJobReference(printer.Destination, output)
	if err == nil && reference != "" {
		return Outcome{State: "submitted", Reference: reference, Accepted: true, Message: "Aceptado por CUPS; no confirma salida física de papel."}
	}
	if _, notStarted := err.(*exec.Error); notStarted {
		return Outcome{State: "error", Message: "No se pudo ejecutar el transporte CUPS; no se envió el documento."}
	}
	return Outcome{State: "uncertain", Message: "CUPS no devolvió un resultado inequívoco. Revisar su cola antes de autorizar otra copia."}
}

// cupsTextArguments son los argumentos separados de lp para un documento de texto: cola, copias,
// título técnico, texto plano sin portada a una cara, densidad (cpi 12, o 6 con el texto grande de
// Cocina), papel calculado por líneas, márgenes en cero y la opción de corte del controlador.
func cupsTextArguments(job PrintJob, filename string) []string {
	printer := job.PrinterSnapshot
	arguments := []string{"-d", printer.Destination, "-n", strconv.Itoa(printer.Copies), "-t", "ADEL-" + job.ID,
		"-o", "document-format=text/plain", "-o", "job-sheets=none", "-o", "sides=one-sided"}
	cpi := 12
	lineMillimeters := 4
	if job.TextSize == "large" {
		cpi = 6
		lineMillimeters = 8
	}
	height := max(60, min(2000, (strings.Count(job.Content, "\n")+4)*lineMillimeters))
	arguments = append(arguments, "-o", "cpi="+textDensity(printer, cpi), "-o", fmt.Sprintf("media=Custom.%dx%dmm", printer.PaperWidth, height),
		"-o", "page-left=0", "-o", "page-right=0", "-o", "page-top=0", "-o", "page-bottom=0")
	if printer.Cut {
		arguments = append(arguments, "-o", printer.CutterOption)
	}
	return append(arguments, filename)
}

// textDensity es la densidad (cpi) de CUPS en texto para la impresora del trabajo. Con las columnas
// predeterminadas de su papel es exactamente la de siempre (12, o 6 con el texto grande de Cocina),
// de modo que los argumentos de lp no cambian. Con otras columnas (48 en 80 mm o 32 en 58 mm, etapa
// 9C) se ajusta en proporción para que el renglón quepa en el mismo papel: cpi × columnas ÷
// predeterminado (13.71 para 48 y 12.8 para 32). Que una impresora concreta respete esa densidad es
// una prueba física pendiente (la prueba de ancho existe para comprobarlo antes de elegir).
func textDensity(printer Printer, cpi int) string {
	if !printer.customColumns() {
		return strconv.Itoa(cpi)
	}
	density := float64(cpi) * float64(printer.TextColumns()) / float64(documents.ColumnsFor(printer.PaperWidth))
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(density, 'f', 2, 64), "0"), ".")
}

// cupsJobReference extrae de la respuesta de lp el identificador del trabajo («cola-123»). La
// búsqueda no depende del idioma del mensaje: macOS lo traduce aunque se pida LC_ALL=C.
func cupsJobReference(queue string, output []byte) string {
	match := regexp.MustCompile(`(?:^|\s)(` + regexp.QuoteMeta(queue) + `-[0-9]+)(?:\s|$)`).FindStringSubmatch(string(output))
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func ESCPosBytes(content string, printer Printer, textSize ...string) []byte {
	// El único origen de comandos binarios es este controlador; el texto ya está saneado.
	encoder := charmap.CodePage850.NewEncoder()
	text := cleanText(content)
	characters := []byte{}
	for _, character := range text {
		encoded, err := encoder.String(string(character))
		if err != nil {
			encoded = "?"
		}
		characters = append(characters, []byte(encoded)...)
	}
	result := []byte{0x1b, 0x40, 0x1b, 0x74, 0x02}
	if len(textSize) > 0 && textSize[0] == "large" {
		result = append(result, 0x1d, 0x21, 0x11)
	}
	result = append(result, characters...)
	result = append(result, '\n', '\n', '\n')
	if printer.Cut && printer.SupportsCut {
		result = append(result, 0x1d, 0x56, 0x00)
	}
	return result
}

func submitESCPos(ctx context.Context, job PrintJob) Outcome {
	return writeESCPos(ctx, job, ESCPosBytes(job.Content, job.PrinterSnapshot, job.TextSize))
}

// writeESCPos entrega los bytes de un documento por una conexión ESC/POS directa, una vez por copia.
func writeESCPos(ctx context.Context, job PrintJob, data []byte) Outcome {
	printer := job.PrinterSnapshot
	dialer := net.Dialer{Timeout: 4 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(printer.Destination, strconv.Itoa(printer.Port)))
	if err != nil {
		return Outcome{State: "error", Message: "No se abrió la conexión con la impresora; no se enviaron bytes."}
	}
	defer connection.Close()
	_ = connection.SetWriteDeadline(time.Now().Add(8 * time.Second))
	for copyNumber := 0; copyNumber < printer.Copies; copyNumber++ {
		written, err := connection.Write(data)
		if err != nil || written != len(data) {
			return Outcome{State: "uncertain", Message: fmt.Sprintf("La conexión se interrumpió durante la copia %d; no repetir sin revisar.", copyNumber+1)}
		}
	}
	return Outcome{State: "submitted", Accepted: true, Message: "Bytes entregados a la conexión de red; no confirma impresión física."}
}

// DrawerPulseBytes es el comando ESC/POS de apertura del cajón: ESC p m t1 t2, con m el pin del
// conector (0 = pin 2, 1 = pin 5), t1 = 25 (50 ms de pulso) y t2 = 250 (500 ms de pausa). No se
// envía ESC @: reiniciar la impresora podría descartar lo que aún tenga en su búfer.
func DrawerPulseBytes(pin int) []byte {
	if pin != 1 {
		pin = 0
	}
	return []byte{0x1b, 0x70, byte(pin), 0x19, 0xfa}
}

// submitDrawerPulse entrega el pulso del cajón con el transporte de su impresora: una conexión
// ESC/POS directa o una cola CUPS en modo raw (etapa 7.1). Otros transportes (el agente futuro)
// quedan en intervención sin enviar nada. Ninguna aceptación confirma que el cajón se abrió.
func submitDrawerPulse(ctx context.Context, job PrintJob, commands commandRunner) Outcome {
	switch job.PrinterSnapshot.Transport {
	case "escpos_tcp":
		return submitDrawerPulseTCP(ctx, job)
	case "cups":
		return submitDrawerPulseCUPS(ctx, job, commands)
	}
	return Outcome{State: "intervention", Message: "El transporte de la impresora no entrega el pulso ESC/POS del cajón sin alterar; no se envió el comando."}
}

// submitDrawerPulseTCP escribe el pulso en una conexión ESC/POS directa por red.
func submitDrawerPulseTCP(ctx context.Context, job PrintJob) Outcome {
	printer := job.PrinterSnapshot
	dialer := net.Dialer{Timeout: 4 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(printer.Destination, strconv.Itoa(printer.Port)))
	if err != nil {
		return Outcome{State: "error", Message: "No se abrió la conexión con la impresora del cajón; no se envió el comando de apertura."}
	}
	defer connection.Close()
	_ = connection.SetWriteDeadline(time.Now().Add(8 * time.Second))
	data := DrawerPulseBytes(job.DrawerPulse.Pin)
	written, err := connection.Write(data)
	if err != nil || written != len(data) {
		return Outcome{State: "uncertain", Message: "La conexión se interrumpió al enviar el comando de apertura; revisa el cajón antes de volver a abrirlo."}
	}
	return Outcome{State: "submitted", Accepted: true, Message: "Comando de apertura entregado a la conexión de red; no confirma que el cajón se abrió."}
}

// drawerPulseArguments son los argumentos separados de lp para el pulso: la cola validada, modo
// raw (CUPS no filtra los bytes), una sola copia y un título técnico con el trabajo de la cola.
func drawerPulseArguments(queue, jobID, filename string) []string {
	return []string{"-d", queue, "-o", "raw", "-n", "1", "-t", "ADEL-cajon-" + jobID, filename}
}

// submitDrawerPulseCUPS envía el pulso en modo raw a la cola CUPS de la impresora del cajón. Antes
// vuelve a consultar la cola (la configuración de CUPS puede haber cambiado desde que se guardó el
// cajón): si ya no admite datos raw, el trabajo queda en intervención sin enviar nada. El pulso se
// escribe en un archivo temporal privado que se elimina al terminar. Una vez iniciado lp, un
// resultado sin identificador de trabajo es incierto y nunca se reintenta solo.
func submitDrawerPulseCUPS(ctx context.Context, job PrintJob, commands commandRunner) Outcome {
	queue := job.PrinterSnapshot.Destination
	capability := inspectQueue(ctx, commands, queue)
	if !capability.Checked {
		return Outcome{State: "error", Message: "No se pudo consultar la cola CUPS del cajón; no se envió el comando de apertura."}
	}
	if !capability.Raw {
		return Outcome{State: "intervention", Message: capability.Reason + " No se envió el comando de apertura."}
	}
	file, err := os.CreateTemp("", "adel-drawer-*.bin")
	if err != nil {
		return Outcome{State: "error", Message: "No se pudo preparar el archivo privado; no se envió el comando de apertura."}
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(DrawerPulseBytes(job.DrawerPulse.Pin)); err != nil {
		file.Close()
		return Outcome{State: "error", Message: "No se pudo escribir el comando de apertura local; no se envió."}
	}
	if err := file.Close(); err != nil {
		return Outcome{State: "error", Message: "No se pudo finalizar el comando de apertura local; no se envió."}
	}
	output, err := commands.run(ctx, true, "/usr/bin/lp", drawerPulseArguments(queue, job.ID, file.Name())...)
	reference := cupsJobReference(queue, output)
	if err == nil && reference != "" {
		return Outcome{State: "submitted", Reference: reference, Accepted: true, Message: "Comando de apertura aceptado por CUPS; no confirma que el cajón se abrió."}
	}
	if _, notStarted := err.(*exec.Error); notStarted {
		return Outcome{State: "error", Message: "No se pudo ejecutar el transporte CUPS; no se envió el comando de apertura."}
	}
	return Outcome{State: "uncertain", Message: "CUPS no devolvió un resultado inequívoco para el comando de apertura; revisa el cajón y la cola antes de volver a abrirlo."}
}
