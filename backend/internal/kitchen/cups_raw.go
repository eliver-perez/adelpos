package kitchen

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Cajón de dinero por CUPS (etapa 7.1, modo mixto). El cajón está conectado a la impresora, no a
// la computadora: con una impresora USB (o cualquier conexión que CUPS entregue tal cual) el pulso
// ESC/POS viaja en modo raw a su cola. Antes de usar una cola se consulta, solo con lpstat y
// lpoptions (nunca se imprime nada para averiguarlo), si CUPS puede entregar los bytes sin
// convertirlos. Todo el envío queda detrás de Transport: en Windows (etapa 9E) lo hace el
// administrador de impresión de Windows (windows_queue.go) sin cambiar la lógica de pagos ni de
// la cola.

// commandRunner ejecuta un programa del sistema con argumentos separados, nunca un shell
// construido con datos del cliente. combined devuelve también la salida de error (lp la usa para
// informar el trabajo). Las pruebas lo sustituyen para verificar argumentos sin ejecutar nada.
type commandRunner interface {
	run(ctx context.Context, combined bool, program string, arguments ...string) ([]byte, error)
}

// hostCommands ejecuta los programas de CUPS del equipo donde corre Go.
type hostCommands struct{}

func (hostCommands) run(ctx context.Context, combined bool, program string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, program, arguments...)
	command.Env = append(os.Environ(), "LC_ALL=C")
	if combined {
		return command.CombinedOutput()
	}
	return command.Output()
}

// QueueCapability es el resultado de consultar una cola CUPS para el pulso del cajón. Checked
// indica que CUPS respondió por la cola; Raw, que entrega los bytes sin convertirlos; Reason
// explica en español por qué no se puede usar.
type QueueCapability struct {
	Queue      string `json:"queue"`
	Checked    bool   `json:"checked"`
	Raw        bool   `json:"raw"`
	Connection string `json:"connection,omitempty"`
	Model      string `json:"model,omitempty"`
	Reason     string `json:"reason,omitempty"`
	// Missing indica que la cola (o la impresora de Windows) no existe en el equipo servidor, a diferencia
	// de una que no contestó: el problema del trabajo es ProblemNotFound (revisión 9F, B4).
	Missing bool `json:"-"`
}

// unavailableCode es la clave del problema de una cola que no se pudo consultar: inexistente o sin
// respuesta.
func (capability QueueCapability) unavailableCode() string {
	if capability.Missing {
		return ProblemNotFound
	}
	return ProblemUnreachable
}

// QueueInspector consulta si una cola CUPS admite datos raw. La implementación del equipo usa
// lpstat y lpoptions; las pruebas de aceptación inyectan una simulada.
type QueueInspector interface {
	InspectQueue(ctx context.Context, queue string) QueueCapability
}

// HostQueueInspector consulta las colas CUPS del equipo donde corre Go, solo con lecturas. En
// Windows (etapa 9E) consulta la impresora instalada en Windows y su puerto (windows_queue.go).
type HostQueueInspector struct {
	commands commandRunner
	// spooler sustituye en las pruebas el administrador de impresión de Windows.
	spooler spoolerAPI
}

// queueInspectionTimeout limita cada consulta: una cola que no responde no debe retener la
// transacción ni el lock operativo de quien configura el cajón.
const queueInspectionTimeout = 3 * time.Second

func (inspector HostQueueInspector) InspectQueue(ctx context.Context, queue string) QueueCapability {
	if spooler, windows := inspector.windowsSpooler(); windows {
		return inspectWindowsQueue(ctx, spooler, queue)
	}
	commands := inspector.commands
	if commands == nil {
		commands = hostCommands{}
	}
	return inspectQueue(ctx, commands, queue)
}

// inspectQueue consulta primero que la cola exista (lpstat -p: lpoptions no falla con una cola
// desconocida) y después sus atributos (lpoptions -p sin -o, que solo informa).
func inspectQueue(ctx context.Context, commands commandRunner, queue string) QueueCapability {
	if !queuePattern.MatchString(queue) {
		return QueueCapability{Queue: queue, Reason: "El nombre de la cola CUPS no es válido."}
	}
	unavailable := QueueCapability{
		Queue:  queue,
		Reason: "No se pudo consultar la cola " + queue + " en CUPS del servidor: no existe o CUPS no responde.",
	}
	limited, cancel := context.WithTimeout(ctx, queueInspectionTimeout)
	defer cancel()
	if _, err := commands.run(limited, false, "/usr/bin/lpstat", "-p", queue); err != nil {
		// lpstat -p falla con una cola desconocida (en macOS el planificador de CUPS se inicia a demanda):
		// se trata como cola inexistente, que se resuelve configurando la impresora.
		missing := unavailable
		missing.Missing = limited.Err() == nil
		return missing
	}
	output, err := commands.run(limited, false, "/usr/bin/lpoptions", "-p", queue)
	if err != nil {
		return unavailable
	}
	if len(output) > 32768 {
		output = output[:32768]
	}
	return EvaluateRawQueue(queue, ParseQueueOptions(string(output)))
}

// ParseQueueOptions interpreta la salida de «lpoptions -p cola»: pares nombre=valor separados por
// espacios, con valores entre comillas simples o dobles y escapes con barra invertida. Un nombre
// sin valor se conserva vacío.
func ParseQueueOptions(output string) map[string]string {
	options := map[string]string{}
	var name, value strings.Builder
	inValue := false
	escaped := false
	quote := rune(0)
	write := func(character rune) {
		if inValue {
			value.WriteRune(character)
			return
		}
		name.WriteRune(character)
	}
	flush := func() {
		if name.Len() > 0 {
			options[name.String()] = value.String()
		}
		name.Reset()
		value.Reset()
		inValue = false
	}
	for _, character := range output {
		switch {
		case escaped:
			write(character)
			escaped = false
		case character == '\\':
			escaped = true
		case quote != 0:
			if character == quote {
				quote = 0
			} else {
				write(character)
			}
		case character == '\'' || character == '"':
			quote = character
		case unicode.IsSpace(character):
			flush()
		case character == '=' && !inValue:
			inValue = true
		default:
			write(character)
		}
	}
	flush()
	return options
}

// Bits de printer-type de CUPS que impiden asegurar a qué impresora llegan los bytes.
const (
	cupsPrinterClass  = 0x1
	cupsPrinterRemote = 0x2
)

// rawConnections son las conexiones en las que CUPS entrega un trabajo raw tal cual al
// dispositivo: USB, puerto serie o paralelo y AppSocket (socket://, el puerto 9100 de la
// impresora). IPP, IPPS, Bonjour (dnssd), LPD y HTTP reciben documentos que la impresora o CUPS
// interpretan: no garantizan que el pulso llegue sin alterar.
var rawConnections = map[string]bool{"usb": true, "serial": true, "parallel": true, "socket": true}

// driverlessMarkers identifican las colas sin controlador (AirPrint o IPP Everywhere), en las que
// CUPS convierte cada trabajo al formato que anuncia la impresora.
var driverlessMarkers = []string{"airprint", "ipp everywhere", "driverless"}

// Veredictos de la regla única de datos raw (rawQueueVerdict): la usan el cajón por CUPS del equipo servidor
// (EvaluateRawQueue, etapa 7.1) y las impresoras que informa cada terminal para su agente de impresión
// (EvaluateTerminalQueue, etapa 9G), cada uno con sus textos.
const (
	rawUsable     = ""
	rawNoDevice   = "no_device"
	rawClass      = "class"
	rawRemote     = "remote"
	rawDriverless = "driverless"
	rawConnection = "connection"
)

// rawQueueVerdict decide, con los atributos que informa CUPS, si la cola entrega los bytes sin convertirlos, y
// devuelve la conexión (el esquema de device-uri). El criterio es conservador: ante cualquier duda la cola no se
// usa.
func rawQueueVerdict(options map[string]string) (string, string) {
	uri := strings.TrimSpace(options["device-uri"])
	if uri == "" {
		return "", rawNoDevice
	}
	scheme, _, _ := strings.Cut(uri, ":")
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	connection := cleanText(scheme)
	if kind, err := strconv.ParseInt(strings.TrimSpace(options["printer-type"]), 10, 64); err == nil {
		if kind&cupsPrinterClass != 0 {
			return connection, rawClass
		}
		if kind&cupsPrinterRemote != 0 {
			return connection, rawRemote
		}
	}
	model := strings.ToLower(options["printer-make-and-model"])
	for _, marker := range driverlessMarkers {
		if strings.Contains(model, marker) {
			return connection, rawDriverless
		}
	}
	if !rawConnections[scheme] {
		return connection, rawConnection
	}
	return connection, rawUsable
}

// EvaluateRawQueue decide, con los atributos que informa CUPS, si la cola entrega el pulso del
// cajón sin convertirlo. El criterio es conservador: ante cualquier duda la cola no se usa y se
// explica por qué.
func EvaluateRawQueue(queue string, options map[string]string) QueueCapability {
	capability := QueueCapability{Queue: queue, Checked: true, Model: cleanText(options["printer-make-and-model"])}
	connection, verdict := rawQueueVerdict(options)
	capability.Connection = connection
	switch verdict {
	case rawNoDevice:
		capability.Reason = "CUPS no informa a qué dispositivo envía la cola " + queue +
			"; no se puede asegurar que entregue el comando del cajón sin alterar."
	case rawClass:
		capability.Reason = "La cola " + queue + " es una clase de impresoras: el cajón está conectado a una impresora concreta."
	case rawRemote:
		capability.Reason = "La cola " + queue + " pertenece a otro equipo: su CUPS podría convertir el comando del cajón."
	case rawDriverless:
		capability.Reason = "La cola " + queue + " es una impresora sin controlador (AirPrint o IPP Everywhere): " +
			"CUPS convierte cada trabajo al formato de la impresora y no garantiza entregar el comando ESC/POS sin alterar."
	case rawConnection:
		capability.Reason = "La cola " + queue + " se conecta por " + capability.Connection +
			", que recibe documentos y no garantiza datos raw. Para una impresora de red usa la conexión ESC/POS por red."
	default:
		capability.Raw = true
	}
	return capability
}

// TerminalQueue es una impresora que informó una terminal para su agente de impresión (etapa 9G, fase E), con si
// AD-EL POS puede usarla para tickets, comandas y el cajón (Usable) y, si no, por qué en palabras del
// restaurante.
type TerminalQueue struct {
	Queue      string `json:"queue"`
	Model      string `json:"model"`
	Connection string `json:"connection"`
	Usable     bool   `json:"usable"`
	Reason     string `json:"reason,omitempty"`
}

// EvaluateTerminalQueue aplica a una impresora informada por una terminal la misma regla del cajón por CUPS: su
// agente le envía los bytes ESC/POS en modo raw, así que solo sirve una impresora concreta de esa computadora
// conectada por USB, puerto serie o paralelo o AppSocket, con controlador.
func EvaluateTerminalQueue(queue, connection, model string, printerType *int64) TerminalQueue {
	options := map[string]string{"printer-make-and-model": model}
	if connection != "" {
		options["device-uri"] = connection + ":"
	}
	if printerType != nil {
		options["printer-type"] = strconv.FormatInt(*printerType, 10)
	}
	_, verdict := rawQueueVerdict(options)
	result := TerminalQueue{Queue: queue, Model: cleanText(model), Connection: connection, Usable: verdict == rawUsable}
	switch verdict {
	case rawNoDevice:
		result.Reason = "La computadora no informa cómo está conectada esta impresora."
	case rawClass:
		result.Reason = "Es un grupo de impresoras, no una impresora concreta."
	case rawRemote:
		result.Reason = "Es una impresora compartida desde otra computadora."
	case rawDriverless:
		result.Reason = "Es una impresora de documentos (AirPrint): no recibe tickets."
	case rawConnection:
		result.Reason = "Está conectada por la red como impresora de documentos; para una impresora de tickets en la red usa «ESC/POS por red»."
	}
	return result
}
