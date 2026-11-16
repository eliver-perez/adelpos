package legacyread

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

type cliRun struct {
	run    *preparation
	cli    CLI
	output *bytes.Buffer
}

func newCLIRun(t *testing.T, stdin io.Reader) *cliRun {
	t.Helper()
	run := newPreparation(t)
	output := &bytes.Buffer{}
	return &cliRun{
		run:    run,
		output: output,
		cli: CLI{
			Stdin:  stdin,
			Stdout: output,
			FindDocker: func() (string, error) {
				return "/usr/local/bin/docker", nil
			},
			NewRunner: func(string) Runner {
				return run.fake
			},
			TempDir: run.tempDir,
			Adjust: func(options *Options) {
				options.KDF = testKDF
				options.PageRows = 50
			},
		},
	}
}

func (current *cliRun) arguments() []string {
	return []string{"--backup", current.run.options.Backup, "--output", current.run.options.Output}
}

// events decodifica las líneas JSON de la salida.
func (current *cliRun) events(t *testing.T) []map[string]any {
	t.Helper()
	events := []map[string]any{}
	scanner := bufio.NewScanner(bytes.NewReader(current.output.Bytes()))
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	for scanner.Scan() {
		event := map[string]any{}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("línea que no es JSON: %s", scanner.Text())
		}
		events = append(events, event)
	}
	return events
}

func lastEvent(t *testing.T, events []map[string]any) map[string]any {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("sin eventos")
	}
	return events[len(events)-1]
}

// La frase de paso llega por la primera línea de la entrada estándar; el avance son líneas JSON y la última es el
// resumen.
func TestCLIPreparesWithThePassphraseFromStdin(t *testing.T) {
	current := newCLIRun(t, strings.NewReader(testPassphrase+"\r\nlo que sigue no se lee\n"))
	if code := current.cli.Run(context.Background(), current.arguments()); code != 0 {
		t.Fatalf("código %d: %s", code, current.output.String())
	}
	events := current.events(t)
	result := lastEvent(t, events)
	if result["event"] != "result" {
		t.Fatalf("último evento: %v", result)
	}
	summary := result["summary"].(map[string]any)
	if summary["content_fingerprint"] != current.run.fake.source.manifest.ContentFingerprint || summary["package_bytes"].(float64) <= 0 {
		t.Fatalf("resumen: %v", summary)
	}
	tables := 0
	for _, event := range events {
		if event["event"] == "table" {
			tables++
			if event["total"].(float64) != 52 {
				t.Fatalf("total de tablas: %v", event)
			}
		}
	}
	if tables < 52 {
		t.Fatalf("eventos de tabla: %d", tables)
	}
	if strings.Contains(current.output.String(), testPassphrase) {
		t.Fatal("la frase de paso apareció en la salida")
	}
	current.run.requireCleanedUp(t, true)
}

func TestCLIRejectsBadInvocations(t *testing.T) {
	cases := map[string]struct {
		stdin     string
		arguments func(current *cliRun) []string
		code      string
	}{
		"frase corta":    {"corta\n", (*cliRun).arguments, CodePassphraseInvalid},
		"sin frase":      {"", (*cliRun).arguments, CodePassphraseInvalid},
		"sin argumentos": {testPassphrase + "\n", func(*cliRun) []string { return nil }, CodeUsage},
		"argumento de más": {testPassphrase + "\n", func(current *cliRun) []string {
			return append(current.arguments(), "extra")
		}, CodeUsage},
		"frase en los argumentos": {testPassphrase + "\n", func(current *cliRun) []string {
			return append(current.arguments(), "--passphrase", testPassphrase)
		}, CodeUsage},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			current := newCLIRun(t, strings.NewReader(testCase.stdin))
			if code := current.cli.Run(context.Background(), testCase.arguments(current)); code != 1 {
				t.Fatalf("código %d", code)
			}
			event := lastEvent(t, current.events(t))
			if event["event"] != "error" || event["code"] != testCase.code || event["message"] == "" {
				t.Fatalf("evento: %v", event)
			}
			if len(current.run.fake.commands()) != 0 {
				t.Fatal("se llamó a docker")
			}
		})
	}
}

func TestCLIWithoutDocker(t *testing.T) {
	current := newCLIRun(t, strings.NewReader(testPassphrase+"\n"))
	current.cli.FindDocker = func() (string, error) {
		return "", fail(CodeDockerMissing, "", nil)
	}
	if code := current.cli.Run(context.Background(), current.arguments()); code != 1 {
		t.Fatal("código")
	}
	event := lastEvent(t, current.events(t))
	if event["code"] != CodeDockerMissing || !strings.Contains(event["message"].(string), "Docker Desktop") {
		t.Fatalf("evento: %v", event)
	}
}

// En una terminal la frase se pide dos veces y deben coincidir.
func TestCLIAsksTwiceOnATerminal(t *testing.T) {
	current := newCLIRun(t, strings.NewReader(""))
	answers := []string{testPassphrase, "otra cosa distinta"}
	current.cli.Terminal = func(string) (string, error) {
		answer := answers[0]
		answers = answers[1:]
		return answer, nil
	}
	if code := current.cli.Run(context.Background(), current.arguments()); code != 1 {
		t.Fatal("código")
	}
	if event := lastEvent(t, current.events(t)); event["code"] != CodePassphraseInvalid {
		t.Fatalf("evento: %v", event)
	}
}

// Con --watch-stdin, cerrar la entrada estándar cancela la preparación y no queda nada.
func TestCLIWatchStdinCancels(t *testing.T) {
	reader, writer := io.Pipe()
	current := newCLIRun(t, reader)
	var once sync.Once
	current.run.fake.onTable = func(name string) {
		if name == "pagos" {
			once.Do(func() {
				_ = writer.Close()
			})
		}
	}
	go func() {
		_, _ = writer.Write([]byte(testPassphrase + "\n"))
	}()
	code := current.cli.Run(context.Background(), append(current.arguments(), "--watch-stdin"))
	if code != 1 {
		t.Fatal("código")
	}
	if event := lastEvent(t, current.events(t)); event["code"] != CodeCancelled {
		t.Fatalf("evento: %v", event)
	}
	current.run.requireCleanedUp(t, false)
}

// Si la salida se cierra (la aplicación que lo lanzó terminó), la preparación se cancela y limpia.
func TestCLIBrokenOutputCancels(t *testing.T) {
	current := newCLIRun(t, strings.NewReader(testPassphrase+"\n"))
	broken := &failingWriter{remaining: 12}
	current.cli.Stdout = broken
	if code := current.cli.Run(context.Background(), current.arguments()); code != 1 {
		t.Fatal("código")
	}
	current.run.requireCleanedUp(t, false)
}

type failingWriter struct {
	mutex     sync.Mutex
	remaining int
}

func (writer *failingWriter) Write(data []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	if writer.remaining <= 0 {
		return 0, errors.New("tubería cerrada")
	}
	writer.remaining--
	return len(data), nil
}
