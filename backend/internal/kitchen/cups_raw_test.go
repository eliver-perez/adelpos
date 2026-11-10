package kitchen

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Pruebas del cajón por CUPS (etapa 7.1) sin ejecutar programas del sistema: un ejecutor
// simulado registra cada llamada y responde con salidas grabadas en este Mac solo con consultas
// de lectura (lpstat -p y lpoptions -p; nunca lp). Ninguna prueba envía trabajos ni pulsos.

type recordedCall struct {
	program   string
	arguments []string
	// pulse es el contenido del archivo que lp recibiría, leído en el momento de la llamada.
	pulse []byte
}

type recordedReply struct {
	output []byte
	err    error
}

type recordedCommands struct {
	t         *testing.T
	forbidden bool
	mutex     sync.Mutex
	calls     []recordedCall
	replies   map[string]recordedReply
}

func (commands *recordedCommands) run(_ context.Context, _ bool, program string, arguments ...string) ([]byte, error) {
	commands.mutex.Lock()
	defer commands.mutex.Unlock()
	if commands.forbidden {
		commands.t.Fatalf("Se intentó ejecutar %s %v", program, arguments)
	}
	call := recordedCall{program: program, arguments: append([]string{}, arguments...)}
	if program == "/usr/bin/lp" && len(arguments) > 0 {
		content, err := os.ReadFile(arguments[len(arguments)-1])
		if err != nil {
			commands.t.Fatalf("lp recibiría un archivo inexistente: %v", err)
		}
		call.pulse = content
	}
	commands.calls = append(commands.calls, call)
	reply, found := commands.replies[program]
	if !found {
		commands.t.Fatalf("Llamada no preparada: %s", program)
	}
	return reply.output, reply.err
}

func recorded(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "cups", name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// La detección de datos raw usa los atributos que informa «lpoptions -p cola». Las salidas son
// las de las cuatro colas reales de este Mac (ubicación y UUID anonimizados).
func TestRawQueueDetectionFromRecordedOutputs(t *testing.T) {
	cases := []struct {
		file       string
		queue      string
		raw        bool
		connection string
		reason     string
	}{
		{"lpoptions-usb-pos80.txt", "Printer_POS_80", true, "usb", ""},
		{"lpoptions-socket-pos80.txt", "_192_168_1_80", true, "socket", ""},
		{"lpoptions-ipp-generic-postscript.txt", "HP_LaserJet_400_M401n", false, "ipp", "se conecta por ipp"},
		{"lpoptions-dnssd-airprint.txt", "HP_Smart_Tank_530_series", false, "dnssd", "AirPrint o IPP Everywhere"},
	}
	for _, item := range cases {
		options := ParseQueueOptions(string(recorded(t, item.file)))
		capability := EvaluateRawQueue(item.queue, options)
		if !capability.Checked || capability.Raw != item.raw || capability.Connection != item.connection {
			t.Fatalf("%s: detección incorrecta %+v", item.file, capability)
		}
		if item.raw && capability.Reason != "" {
			t.Fatalf("%s: una cola compatible no debe llevar motivo: %s", item.file, capability.Reason)
		}
		if !item.raw && !strings.Contains(capability.Reason, item.reason) {
			t.Fatalf("%s: el motivo no explica el rechazo: %s", item.file, capability.Reason)
		}
	}
	usb := ParseQueueOptions(string(recorded(t, "lpoptions-usb-pos80.txt")))
	if usb["printer-info"] != "Printer POS-80" || usb["printer-location"] != "Mostrador" || usb["printer-type"] != "2150404" {
		t.Fatalf("Los valores entre comillas no se interpretaron: %+v", usb)
	}
	airprint := ParseQueueOptions(string(recorded(t, "lpoptions-dnssd-airprint.txt")))
	if airprint["marker-names"] != "cyan cartridge,magenta cartridge,yellow cartridge,black cartridge" {
		t.Fatalf("Los escapes con barra invertida no se interpretaron: %q", airprint["marker-names"])
	}
	// lpoptions con una cola desconocida termina con éxito y escribe bytes sin sentido (grabado en
	// este Mac): sin device-uri nunca se considera compatible.
	unknown := EvaluateRawQueue("ADEL_NO_EXISTE", ParseQueueOptions(string(recorded(t, "lpoptions-unknown-queue.bin"))))
	if unknown.Raw || !strings.Contains(unknown.Reason, "no informa a qué dispositivo") {
		t.Fatalf("Una salida sin dispositivo se aceptó: %+v", unknown)
	}
}

// Clases, colas remotas y colas sin controlador nunca reciben el pulso, aunque su conexión
// fuera USB.
func TestRawQueueDetectionRejectsClassesRemoteAndDriverless(t *testing.T) {
	cases := []struct {
		options map[string]string
		reason  string
	}{
		{map[string]string{"device-uri": "usb://Printer/POS-80", "printer-type": "2150405"}, "clase de impresoras"},
		{map[string]string{"device-uri": "usb://Printer/POS-80", "printer-type": "2150406"}, "pertenece a otro equipo"},
		{map[string]string{"device-uri": "usb://Printer/POS-80", "printer-make-and-model": "Impresora - IPP Everywhere"}, "IPP Everywhere"},
		{map[string]string{"device-uri": "lpd://192.168.1.80/queue"}, "se conecta por lpd"},
		{map[string]string{"device-uri": "ipps://192.168.1.80/ipp/print"}, "se conecta por ipps"},
	}
	for _, item := range cases {
		capability := EvaluateRawQueue("COLA_PRUEBA", item.options)
		if capability.Raw || !strings.Contains(capability.Reason, item.reason) {
			t.Fatalf("%+v: detección incorrecta %+v", item.options, capability)
		}
	}
	serial := EvaluateRawQueue("COLA_SERIE", map[string]string{"device-uri": "serial:/dev/cu.usbserial?baud=9600", "printer-type": "4"})
	if !serial.Raw || serial.Connection != "serial" {
		t.Fatalf("Una cola por puerto serie se rechazó: %+v", serial)
	}
}

// La consulta de la cola solo usa lpstat -p (existencia) y lpoptions -p (atributos), con
// argumentos separados; una cola desconocida o un nombre inválido no ejecuta nada más.
func TestQueueInspectionUsesReadOnlyQueries(t *testing.T) {
	commands := &recordedCommands{t: t, replies: map[string]recordedReply{
		"/usr/bin/lpstat":    {output: recorded(t, "lpstat-p-usb-pos80.txt")},
		"/usr/bin/lpoptions": {output: recorded(t, "lpoptions-usb-pos80.txt")},
	}}
	capability := HostQueueInspector{commands: commands}.InspectQueue(context.Background(), "Printer_POS_80")
	if !capability.Checked || !capability.Raw {
		t.Fatalf("La cola USB grabada no se detectó compatible: %+v", capability)
	}
	if len(commands.calls) != 2 ||
		strings.Join(append([]string{commands.calls[0].program}, commands.calls[0].arguments...), " ") != "/usr/bin/lpstat -p Printer_POS_80" ||
		strings.Join(append([]string{commands.calls[1].program}, commands.calls[1].arguments...), " ") != "/usr/bin/lpoptions -p Printer_POS_80" {
		t.Fatalf("Consultas inesperadas: %+v", commands.calls)
	}
	missing := &recordedCommands{t: t, replies: map[string]recordedReply{
		"/usr/bin/lpstat": {output: recorded(t, "lpstat-p-unknown-queue.txt"), err: errors.New("exit status 1")},
	}}
	unknown := HostQueueInspector{commands: missing}.InspectQueue(context.Background(), "ADEL_NO_EXISTE")
	if unknown.Checked || unknown.Raw || len(missing.calls) != 1 || !strings.Contains(unknown.Reason, "No se pudo consultar") {
		t.Fatalf("Una cola desconocida no se explicó: %+v %+v", unknown, missing.calls)
	}
	invalid := HostQueueInspector{commands: &recordedCommands{t: t, forbidden: true}}.InspectQueue(context.Background(), "cola; rm -rf /")
	if invalid.Checked || invalid.Raw {
		t.Fatalf("Un nombre de cola inválido llegó a consultarse: %+v", invalid)
	}
}

// El pulso por CUPS se envía con /usr/bin/lp y argumentos separados: -d cola -o raw -n 1 -t
// ADEL-cajon-<trabajo> <archivo privado>, con exactamente los bytes ESC p del conector. El
// ejecutor simulado registra los argumentos y el contenido del archivo sin ejecutar lp.
func TestDrawerPulseThroughCUPSUsesRawArguments(t *testing.T) {
	job := PrintJob{
		ID:              "00000000-0000-4000-8000-0000000000d1",
		DrawerPulse:     &DrawerPulse{Pin: 1},
		PrinterSnapshot: Printer{Name: "Tickets USB", Transport: "cups", Destination: "Printer_POS_80", PaperWidth: 80, Copies: 2, Port: 9100},
	}
	accepted := &recordedCommands{t: t, replies: map[string]recordedReply{
		"/usr/bin/lpstat":    {output: recorded(t, "lpstat-p-usb-pos80.txt")},
		"/usr/bin/lpoptions": {output: recorded(t, "lpoptions-usb-pos80.txt")},
		"/usr/bin/lp":        {output: []byte("la id. de petición es Printer_POS_80-231 (1 archivo(s))\n")},
	}}
	outcome := HostTransport{commands: accepted}.Submit(context.Background(), job)
	if outcome.State != "submitted" || !outcome.Accepted || outcome.Reference != "Printer_POS_80-231" ||
		!strings.Contains(outcome.Message, "no confirma que el cajón se abrió") {
		t.Fatalf("El pulso aceptado por CUPS quedó mal registrado: %+v", outcome)
	}
	if len(accepted.calls) != 3 {
		t.Fatalf("Llamadas inesperadas: %+v", accepted.calls)
	}
	lp := accepted.calls[2]
	if lp.program != "/usr/bin/lp" || len(lp.arguments) != 9 {
		t.Fatalf("lp no recibió los argumentos esperados: %+v", lp)
	}
	expected := []string{"-d", "Printer_POS_80", "-o", "raw", "-n", "1", "-t", "ADEL-cajon-" + job.ID}
	if strings.Join(lp.arguments[:8], "\x00") != strings.Join(expected, "\x00") {
		t.Fatalf("Argumentos de lp incorrectos: %q", lp.arguments)
	}
	filename := lp.arguments[8]
	if !strings.HasPrefix(filepath.Base(filename), "adel-drawer-") || !bytes.Equal(lp.pulse, DrawerPulseBytes(1)) {
		t.Fatalf("El archivo del pulso no contiene ESC p del conector 5: %s % x", filename, lp.pulse)
	}
	if _, err := os.Stat(filename); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("El archivo temporal del pulso no se eliminó")
	}
	if !strings.Contains(strings.Join(drawerPulseArguments("Q", "J", "/tmp/f"), " "), "-o raw -n 1 -t ADEL-cajon-J /tmp/f") {
		t.Fatal("drawerPulseArguments no conserva el orden de argumentos")
	}
}

// Sin identificador de trabajo el resultado es incierto (nunca se reintenta solo); un lp que no
// pudo iniciarse es un error confirmado; una cola que dejó de admitir datos raw queda en
// intervención sin llamar a lp.
func TestDrawerPulseThroughCUPSOutcomes(t *testing.T) {
	job := PrintJob{
		ID:              "00000000-0000-4000-8000-0000000000d2",
		DrawerPulse:     &DrawerPulse{Pin: 0},
		PrinterSnapshot: Printer{Name: "Tickets USB", Transport: "cups", Destination: "Printer_POS_80", PaperWidth: 80, Copies: 1, Port: 9100},
	}
	queries := func(lp recordedReply, options string) *recordedCommands {
		return &recordedCommands{t: t, replies: map[string]recordedReply{
			"/usr/bin/lpstat":    {output: recorded(t, "lpstat-p-usb-pos80.txt")},
			"/usr/bin/lpoptions": {output: recorded(t, options)},
			"/usr/bin/lp":        lp,
		}}
	}
	uncertain := HostTransport{commands: queries(recordedReply{output: []byte("")}, "lpoptions-usb-pos80.txt")}.Submit(context.Background(), job)
	if uncertain.State != "uncertain" || uncertain.Accepted {
		t.Fatalf("Una respuesta sin trabajo no quedó incierta: %+v", uncertain)
	}
	failed := HostTransport{commands: queries(recordedReply{err: &exec.Error{Name: "/usr/bin/lp", Err: exec.ErrNotFound}}, "lpoptions-usb-pos80.txt")}.Submit(context.Background(), job)
	if failed.State != "error" || failed.Accepted {
		t.Fatalf("Un lp que no se ejecutó no quedó como error confirmado: %+v", failed)
	}
	changed := queries(recordedReply{}, "lpoptions-dnssd-airprint.txt")
	airprint := HostTransport{commands: changed}.Submit(context.Background(), job)
	if airprint.State != "intervention" || len(changed.calls) != 2 || !strings.Contains(airprint.Message, "No se envió") {
		t.Fatalf("Una cola sin datos raw recibió el pulso: %+v %+v", airprint, changed.calls)
	}
}
