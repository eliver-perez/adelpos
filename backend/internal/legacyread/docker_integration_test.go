//go:build integration && legacydocker

package legacyread

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/legacypkg"
	"local/ad-el-pos/server/internal/legacysynth"
)

// Prueba con Docker real y datos sintéticos (opcional: etiquetas integration y legacydocker, y ADEL_TEST_DOCKER=1;
// nunca en la tupla de scripts/database.py). Crea la base sintética de internal/legacysynth (las 52 tablas del sistema
// anterior, compatibilidad 110) en un SQL Server de prueba, la respalda a un .bak sintético y corre legacy-prepare
// completo contra ese .bak. El paquete debe tener exactamente el contenido del paquete sintético (misma huella: importes,
// milisegundos, textos difíciles y orden de las filas), ninguna contraseña ni dato personal, y al terminar no debe
// quedar ningún contenedor ni volumen. Nunca usa el respaldo real.
//
//	ADEL_TEST_DOCKER=1 go test -tags 'integration legacydocker' -run TestLegacyPrepareWithDocker -v ./internal/legacyread

const testLabel = "local.adel.pos.legacy-test=1"

func dockerForTest(t *testing.T) string {
	t.Helper()
	if os.Getenv("ADEL_TEST_DOCKER") != "1" {
		t.Skip("prueba con Docker: ADEL_TEST_DOCKER=1")
	}
	docker, err := SystemLocator().Find()
	if err != nil {
		t.Skip("sin docker")
	}
	runner := ExecRunner{Docker: docker}
	output := newBoundedBuffer(1 << 20)
	if err := runner.Run(context.Background(), Command{Args: []string{"image", "inspect", "--format", "{{.Architecture}}", Image}, Stdout: output}); err != nil {
		t.Skip("sin la imagen " + Image + " (descárgala con docker pull --platform linux/amd64)")
	}
	return docker
}

func dockerRun(t *testing.T, docker string, stdin string, arguments ...string) string {
	t.Helper()
	stdout := newBoundedBuffer(16 << 20)
	stderr := newBoundedBuffer(1 << 20)
	command := Command{Args: arguments, Stdout: stdout, Stderr: stderr}
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	if err := (ExecRunner{Docker: docker}).Run(context.Background(), command); err != nil {
		t.Fatalf("docker %s: %v\n%s", arguments[0], err, stderr.String())
	}
	return stdout.String()
}

// syntheticBackup crea la base sintética en un SQL Server de prueba (con su propia etiqueta, que el lector no toca) y
// la respalda a un .bak en una carpeta temporal.
func syntheticBackup(t *testing.T, docker string) string {
	t.Helper()
	name := "adel-legacy-test-" + randomHex(6)
	runner := ExecRunner{Docker: docker}
	remove := func() {
		_ = runner.Run(context.Background(), Command{Args: []string{"rm", "-f", "-v", name}, Stdout: io.Discard, Stderr: io.Discard})
	}
	t.Cleanup(remove)
	err := runner.Run(context.Background(), Command{
		Args: []string{"run", "-d", "--name", name, "--label", testLabel, "--platform", Platform, "--pull", "never",
			"--network", "none", "-e", "ACCEPT_EULA=Y", "-e", "MSSQL_PID=Express", "-e", "MSSQL_SA_PASSWORD", Image},
		Env:    []string{"MSSQL_SA_PASSWORD=" + newPassword()},
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Minute)
	for {
		probe := runner.Run(context.Background(), Command{
			Args:   execArguments(name, "", jsonArguments),
			Stdin:  strings.NewReader("SELECT 1;"),
			Stdout: io.Discard,
			Stderr: io.Discard,
		})
		if probe == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("el SQL Server de prueba no arrancó")
		}
		time.Sleep(2 * time.Second)
	}
	dockerRun(t, docker, legacysynth.TSQL(legacysynth.Dataset()), execArguments(name, "", jsonArguments)...)
	backup := "BACKUP DATABASE [" + legacysynth.DatabaseName + "] TO DISK = N'/var/opt/mssql/data/sintetico.bak' WITH INIT, FORMAT;"
	dockerRun(t, docker, backup, execArguments(name, "", jsonArguments)...)
	path := filepath.Join(t.TempDir(), legacysynth.BackupFile)
	dockerRun(t, docker, "", "cp", name+":/var/opt/mssql/data/sintetico.bak", path)
	remove()
	return path
}

func syntheticManifest(t *testing.T) legacypkg.Manifest {
	t.Helper()
	result, err := legacysynth.Build(io.Discard, testPassphrase, legacypkg.WriterOptions{SpoolDir: t.TempDir(), KDF: testKDF})
	if err != nil {
		t.Fatal(err)
	}
	return result.Manifest
}

// requireSyntheticContent abre el paquete y lo compara con el sintético; también busca contraseñas y datos personales
// en todo su contenido.
func requireSyntheticContent(t *testing.T, path string, expected legacypkg.Manifest) legacypkg.Manifest {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := legacypkg.Open(file, testPassphrase, legacypkg.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var content bytes.Buffer
	for {
		table, err := reader.NextTable()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		for {
			row, err := table.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(row)
			content.Write(encoded)
		}
	}
	manifest := reader.Manifest()
	if manifest.ContentFingerprint != expected.ContentFingerprint {
		for index, table := range manifest.Tables {
			if index < len(expected.Tables) && !reflect.DeepEqual(table, expected.Tables[index]) {
				t.Errorf("la tabla %s no coincide:\n%+v\n%+v", table.Name, table, expected.Tables[index])
			}
		}
		t.Fatal("el paquete de Docker no tiene el contenido del sintético")
	}
	if !reflect.DeepEqual(manifest.SkippedTables, expected.SkippedTables) || !reflect.DeepEqual(manifest.Summary, expected.Summary) {
		t.Fatalf("omitidas o resumen: %+v %+v", manifest.SkippedTables, manifest.Summary)
	}
	for _, password := range legacysynth.SyntheticPasswords {
		if password.Password != "" && bytes.Contains(content.Bytes(), []byte(password.Password)) {
			t.Fatal("una contraseña llegó al paquete")
		}
	}
	for _, personal := range []string{"uno@ejemplo.invalid", "Calle Falsa", "Avenida Sintética"} {
		if bytes.Contains(content.Bytes(), []byte(personal)) {
			t.Fatalf("un dato personal llegó al paquete: %s", personal)
		}
	}
	source := manifest.Source
	if source.DatabaseName != legacysynth.DatabaseName || source.CompatibilityLevel != 110 || !strings.HasPrefix(source.ServerVersion, "16.") ||
		!strings.HasPrefix(source.ReaderImage, "mcr.microsoft.com/mssql/server@sha256:") || source.ReaderVersion == "" {
		t.Fatalf("origen: %+v", source)
	}
	return manifest
}

// requireNoLeftovers comprueba que no quedó ningún contenedor ni volumen de los procesos de esta prueba (por la
// etiqueta con su PID: un lector de otra ejecución viva no cuenta).
func requireNoLeftovers(t *testing.T, docker string, pids ...int) {
	t.Helper()
	ours := map[string]bool{}
	for _, pid := range pids {
		ours[strconv.Itoa(pid)] = true
	}
	format := `{{.Label "` + ReaderPIDLabel + `"}}`
	containers := dockerRun(t, docker, "", "ps", "-a", "--filter", "label="+ReaderLabel, "--format", "{{.ID}}|"+format)
	volumes := dockerRun(t, docker, "", "volume", "ls", "--filter", "label="+ReaderLabel, "--format", "{{.Name}}|"+format)
	for _, line := range strings.Split(strings.TrimSpace(containers+"\n"+volumes), "\n") {
		name, pid, _ := strings.Cut(strings.TrimSpace(line), "|")
		if name != "" && ours[pid] {
			t.Fatalf("quedó %s del proceso %s", name, pid)
		}
	}
}

// preparation es una ejecución del subcomando real.
type binaryRun struct {
	command *exec.Cmd
	stdout  *bytes.Buffer
}

func startBinary(t *testing.T, binary, backup, output, tempDir string) *binaryRun {
	t.Helper()
	command := exec.Command(binary, "legacy-prepare", "--backup", backup, "--output", output)
	command.Stdin = strings.NewReader(testPassphrase + "\n")
	command.Env = append(os.Environ(), "TMPDIR="+tempDir, "TMP="+tempDir, "TEMP="+tempDir)
	stdout := &bytes.Buffer{}
	command.Stdout = stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	return &binaryRun{command: command, stdout: stdout}
}

// finish espera el fin y devuelve el último evento.
func (run *binaryRun) finish(t *testing.T) (map[string]any, []byte) {
	t.Helper()
	_ = run.command.Wait()
	var last map[string]any
	var lastLine []byte
	scanner := bufio.NewScanner(bytes.NewReader(run.stdout.Bytes()))
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	for scanner.Scan() {
		var event map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("línea que no es JSON: %s", scanner.Text())
		}
		last = event
		lastLine = append([]byte{}, scanner.Bytes()...)
	}
	return last, lastLine
}

func requireResult(t *testing.T, name string, event map[string]any, line []byte, expected legacypkg.Manifest) Summary {
	t.Helper()
	if event["event"] != "result" {
		t.Fatalf("%s: %v", name, event)
	}
	var result ResultEvent
	if err := json.Unmarshal(line, &result); err != nil {
		t.Fatal(err)
	}
	summary := result.Summary
	if summary.ContentFingerprint != expected.ContentFingerprint || !summary.Verified || summary.Sales == nil || len(summary.Tables) != 50 {
		t.Fatalf("%s: resumen %+v", name, summary)
	}
	return summary
}

func TestLegacyPrepareWithDocker(t *testing.T) {
	docker := dockerForTest(t)
	started := time.Now()
	backup := syntheticBackup(t, docker)
	t.Logf("respaldo sintético creado en %.1f s", time.Since(started).Seconds())
	expected := syntheticManifest(t)
	directory := t.TempDir()
	binary := filepath.Join(t.TempDir(), "ad-el-pos-server")
	build := exec.Command("go", "build", "-o", binary, "./cmd/ad-el-pos-server")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, output)
	}
	// 1. El subcomando real (binario compilado, frase por la entrada estándar, avance en líneas JSON). Mientras corre:
	//    otra preparación con la misma carpeta temporal responde ALREADY_RUNNING, y una con otra carpeta (otro bloqueo)
	//    corre a la vez sin que ninguna destruya el lector de la otra.
	shared := t.TempDir()
	first := startBinary(t, binary, backup, filepath.Join(directory, "primero.adelmig"), shared)
	time.Sleep(2 * time.Second)
	blocked := startBinary(t, binary, backup, filepath.Join(directory, "bloqueado.adelmig"), shared)
	event, _ := blocked.finish(t)
	if event["code"] != CodeAlreadyRunning {
		t.Fatalf("la segunda preparación con la misma carpeta: %v", event)
	}
	parallelStart := time.Now()
	parallel := startBinary(t, binary, backup, filepath.Join(directory, "paralelo.adelmig"), t.TempDir())
	firstEvent, firstLine := first.finish(t)
	parallelEvent, parallelLine := parallel.finish(t)
	summary := requireResult(t, "la primera", firstEvent, firstLine, expected)
	requireResult(t, "la que corrió a la vez", parallelEvent, parallelLine, expected)
	t.Logf("legacy-prepare: %.1f s en total; pasos %v; paquete de %d bytes; la paralela terminó %.1f s después de empezar",
		summary.Timings["total"], summary.Timings, summary.PackageBytes, time.Since(parallelStart).Seconds())
	requireSyntheticContent(t, filepath.Join(directory, "primero.adelmig"), expected)
	requireSyntheticContent(t, filepath.Join(directory, "paralelo.adelmig"), expected)
	requireNoLeftovers(t, docker, first.command.Process.Pid, blocked.command.Process.Pid, parallel.command.Process.Pid)
	if _, err := os.Stat(filepath.Join(directory, "bloqueado.adelmig")); !os.IsNotExist(err) {
		t.Fatal("la preparación bloqueada escribió un paquete")
	}

	// 2. Páginas de 7 filas (muchos límites de página, también en las tablas sin llave primaria), en este proceso.
	paged := filepath.Join(directory, "paginas.adelmig")
	var events bytes.Buffer
	cli := CLI{
		Stdin:      strings.NewReader(testPassphrase + "\n"),
		Stdout:     &events,
		FindDocker: SystemLocator().Find,
		NewRunner: func(path string) Runner {
			return ExecRunner{Docker: path}
		},
		TempDir: t.TempDir(),
		Adjust: func(options *Options) {
			options.PageRows = 7
			options.KDF = testKDF
		},
	}
	pagedStart := time.Now()
	if code := cli.Run(context.Background(), []string{"--backup", backup, "--output", paged}); code != 0 {
		t.Fatalf("con páginas de 7 filas: %s", events.String())
	}
	t.Logf("con páginas de 7 filas: %.1f s", time.Since(pagedStart).Seconds())
	requireSyntheticContent(t, paged, expected)
	requireNoLeftovers(t, docker, os.Getpid())
}
