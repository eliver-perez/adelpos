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
type HostTransport struct{}

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

func (HostTransport) Submit(ctx context.Context, job PrintJob) Outcome {
	if err := validatePrinter(&job.PrinterSnapshot); err != nil {
		return Outcome{State: "intervention", Message: "El destino guardado requiere revisión."}
	}
	if job.DrawerPulse != nil {
		return submitDrawerPulse(ctx, job)
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
	arguments := []string{"-d", printer.Destination, "-n", strconv.Itoa(printer.Copies), "-t", "ADEL-" + job.ID,
		"-o", "document-format=text/plain", "-o", "job-sheets=none", "-o", "sides=one-sided"}
	cpi := 12
	lineMillimeters := 4
	if job.TextSize == "large" {
		cpi = 6
		lineMillimeters = 8
	}
	height := max(60, min(2000, (strings.Count(job.Content, "\n")+4)*lineMillimeters))
	arguments = append(arguments, "-o", fmt.Sprintf("cpi=%d", cpi), "-o", fmt.Sprintf("media=Custom.%dx%dmm", printer.PaperWidth, height),
		"-o", "page-left=0", "-o", "page-right=0", "-o", "page-top=0", "-o", "page-bottom=0")
	if printer.Cut {
		arguments = append(arguments, "-o", printer.CutterOption)
	}
	arguments = append(arguments, file.Name())
	command := exec.CommandContext(ctx, "/usr/bin/lp", arguments...)
	command.Env = append(os.Environ(), "LC_ALL=C", "CHARSET=utf-8")
	// Una vez iniciado lp, un timeout o error genérico puede ocultar aceptación del spooler.
	output, err := command.CombinedOutput()
	match := regexp.MustCompile(`(?:^|\s)(` + regexp.QuoteMeta(printer.Destination) + `-[0-9]+)(?:\s|$)`).FindStringSubmatch(string(output))
	if err == nil && len(match) == 2 {
		return Outcome{State: "submitted", Reference: match[1], Accepted: true, Message: "Aceptado por CUPS; no confirma salida física de papel."}
	}
	if _, notStarted := err.(*exec.Error); notStarted {
		return Outcome{State: "error", Message: "No se pudo ejecutar el transporte CUPS; no se envió el documento."}
	}
	return Outcome{State: "uncertain", Message: "CUPS no devolvió un resultado inequívoco. Revisar su cola antes de autorizar otra copia."}
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
	printer := job.PrinterSnapshot
	dialer := net.Dialer{Timeout: 4 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(printer.Destination, strconv.Itoa(printer.Port)))
	if err != nil {
		return Outcome{State: "error", Message: "No se abrió la conexión con la impresora; no se enviaron bytes."}
	}
	defer connection.Close()
	_ = connection.SetWriteDeadline(time.Now().Add(8 * time.Second))
	data := ESCPosBytes(job.Content, printer, job.TextSize)
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

// submitDrawerPulse entrega el pulso del cajón por una conexión ESC/POS directa. Otros transportes
// alterarían los bytes (CUPS los filtra como texto) o no existen aún (agente): el trabajo queda en
// intervención sin enviar nada. La aceptación TCP no confirma que el cajón se abrió físicamente.
func submitDrawerPulse(ctx context.Context, job PrintJob) Outcome {
	printer := job.PrinterSnapshot
	if !CompatibleDrawerTransport(printer.Transport) {
		return Outcome{State: "intervention", Message: "El transporte de la impresora no entrega el pulso ESC/POS del cajón sin alterar; no se envió el comando."}
	}
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
