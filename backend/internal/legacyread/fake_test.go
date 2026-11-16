package legacyread

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"local/ad-el-pos/server/internal/legacypkg"
	"local/ad-el-pos/server/internal/legacysynth"
)

// Docker simulado: responde como docker y como el SQL Server del contenedor, con los datos del paquete sintético
// (internal/legacysynth). Así se prueba todo el lector sin Docker: argumentos, limpieza, contraseñas, páginas y el
// paquete resultante, que debe tener la misma huella de contenido que el sintético.

const testPassphrase = "frase de paso de prueba"

var testKDF = legacypkg.KDFParams{Time: 1, MemoryKiB: 8 << 10, Threads: 1}

// syntheticSource son las tablas del paquete sintético con sus filas, para servirlas como las serviría SQL Server.
type syntheticSource struct {
	manifest legacypkg.Manifest
	rows     map[string][][]any
	schema   map[string]*legacysynth.Table
}

var (
	syntheticOnce   sync.Once
	syntheticCached syntheticSource
)

func loadSynthetic(t *testing.T) syntheticSource {
	t.Helper()
	syntheticOnce.Do(func() {
		var encoded bytes.Buffer
		if _, err := legacysynth.Build(&encoded, testPassphrase, legacypkg.WriterOptions{SpoolDir: os.TempDir(), KDF: testKDF}); err != nil {
			panic(err)
		}
		reader, err := legacypkg.Open(bytes.NewReader(encoded.Bytes()), testPassphrase, legacypkg.OpenOptions{})
		if err != nil {
			panic(err)
		}
		source := syntheticSource{manifest: reader.Manifest(), rows: map[string][][]any{}, schema: map[string]*legacysynth.Table{}}
		for {
			table, err := reader.NextTable()
			if err == io.EOF {
				break
			}
			if err != nil {
				panic(err)
			}
			rows := [][]any{}
			for {
				row, err := table.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					panic(err)
				}
				rows = append(rows, row)
			}
			source.rows[table.Info().Name] = rows
		}
		for _, table := range legacysynth.Dataset() {
			source.schema[table.Name] = table
		}
		syntheticCached = source
	})
	return syntheticCached
}

// recorded es una orden recibida.
type recorded struct {
	Args  []string
	Env   []string
	Stdin string
}

// fakeDocker simula docker y el SQL Server del contenedor.
type fakeDocker struct {
	t      *testing.T
	source syntheticSource
	mutex  sync.Mutex
	calls  []recorded
	// Comportamientos configurables.
	infoOS          string
	infoFails       bool
	imageMissing    bool
	leftoverIDs     string
	leftoverVolumes string
	mountFailures   int
	restoreFails    bool
	restorePanics   bool
	brokenTable     string
	onTable         func(name string)
	notReadyProbes  int
	stoppedOnStart  bool
	startLogs       string
	backupSize      int64
}

func newFakeDocker(t *testing.T) *fakeDocker {
	return &fakeDocker{t: t, source: loadSynthetic(t), infoOS: "linux"}
}

func (fake *fakeDocker) record(command Command) recorded {
	stdin := ""
	if command.Stdin != nil {
		content, _ := io.ReadAll(command.Stdin)
		stdin = string(content)
	}
	call := recorded{Args: append([]string{}, command.Args...), Env: append([]string{}, command.Env...), Stdin: stdin}
	fake.mutex.Lock()
	fake.calls = append(fake.calls, call)
	fake.mutex.Unlock()
	return call
}

func (fake *fakeDocker) commands() []recorded {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]recorded{}, fake.calls...)
}

func write(writer io.Writer, text string) {
	if writer != nil {
		_, _ = io.WriteString(writer, text)
	}
}

func (fake *fakeDocker) Run(ctx context.Context, command Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	call := fake.record(command)
	arguments := strings.Join(call.Args, " ")
	switch {
	case arguments == "info --format {{.OSType}}":
		if fake.infoFails {
			write(command.Stderr, "Cannot connect to the Docker daemon")
			return &ExitError{Code: 1}
		}
		write(command.Stdout, fake.infoOS+"\n")
	case strings.HasPrefix(arguments, "image inspect"):
		if fake.imageMissing {
			fake.imageMissing = false
			return &ExitError{Code: 1}
		}
		write(command.Stdout, `sha256:1111|amd64|["mcr.microsoft.com/mssql/server@sha256:2222"]`+"\n")
	case strings.HasPrefix(arguments, "pull "):
		write(command.Stdout, "2022-latest: Pulling from mssql/server\naaa: Pulling fs layer\nbbb: Pulling fs layer\naaa: Pull complete\nbbb: Already exists\n")
	case strings.HasPrefix(arguments, "ps -a -q"):
		write(command.Stdout, fake.leftoverIDs)
	case strings.HasPrefix(arguments, "volume ls -q"):
		write(command.Stdout, fake.leftoverVolumes)
	case strings.HasPrefix(arguments, "rm -f -v"), strings.HasPrefix(arguments, "volume rm"), strings.HasPrefix(arguments, "volume create"):
	case strings.HasPrefix(arguments, "run -d"):
		if fake.mountFailures > 0 {
			fake.mountFailures--
			write(command.Stderr, `docker: Error response from daemon: invalid mount config for type "bind": bind source path does not exist`)
			return &ExitError{Code: 125}
		}
	case strings.Contains(arguments, " stat -c %s "):
		write(command.Stdout, strconv.FormatInt(fake.backupSize, 10)+"\n")
	case strings.HasPrefix(arguments, "inspect --format {{.State.Running}}"):
		if fake.stoppedOnStart {
			write(command.Stdout, "false\n")
		} else {
			write(command.Stdout, "true\n")
		}
	case strings.HasPrefix(arguments, "logs "):
		write(command.Stdout, fake.startLogs)
	case strings.HasPrefix(arguments, "exec -i "):
		return fake.sql(ctx, command, call)
	default:
		fake.t.Errorf("orden de docker inesperada: %s", arguments)
		return &ExitError{Code: 1}
	}
	return nil
}

var (
	tableFrom  = regexp.MustCompile(`INTO #adel_rows FROM \[dbo\]\.\[([^\]]+)\]`)
	countFrom  = regexp.MustCompile(`COUNT_BIG\(\*\) AS \[total\] FROM \[dbo\]\.\[([^\]]+)\]`)
	pageRowsRe = regexp.MustCompile(`@from \+ (\d+) ORDER BY`)
)

// sql responde como sqlcmd dentro del contenedor.
func (fake *fakeDocker) sql(ctx context.Context, command Command, call recorded) error {
	sql := call.Stdin
	switch {
	case strings.Contains(sql, "SELECT 1;"):
		if fake.stoppedOnStart {
			return &ExitError{Code: 1}
		}
		if fake.notReadyProbes > 0 {
			fake.notReadyProbes--
			write(command.Stderr, "Sqlcmd: Error: Microsoft ODBC Driver 18 for SQL Server : Login timeout expired.")
			return &ExitError{Code: 1}
		}
		write(command.Stdout, "1\n")
	case strings.Contains(sql, "RESTORE HEADERONLY"):
		header := strings.Join([]string{"BackupName", "BackupType", "Position", "DatabaseName", "DatabaseVersion", "BackupStartDate",
			"BackupFinishDate", "CompatibilityLevel", "SoftwareVersionMajor", "SoftwareVersionMinor", "SoftwareVersionBuild", "Collation"}, columnSeparator)
		row := strings.Join([]string{"nombre | con barra", "1", "1", "possys", "706", "2026-11-14 22:00:01.000", "2026-11-14 22:00:09.123",
			"110", "11", "0", "7001", "SQL_Latin1_General_CP1_CI_AS"}, columnSeparator)
		dashes := strings.Repeat("-", 5)
		write(command.Stdout, header+"\n"+dashes+"\n"+row+"\n\n")
	case strings.Contains(sql, "RESTORE FILELISTONLY"):
		header := strings.Join([]string{"LogicalName", "PhysicalName", "Type", "FileId"}, columnSeparator)
		rows := strings.Join([]string{"possys", `C:\datos\possys.mdf`, "D", "1"}, columnSeparator) + "\n" +
			strings.Join([]string{"possys_log", `C:\datos\possys_log.ldf`, "L", "2"}, columnSeparator)
		write(command.Stdout, header+"\n-----\n"+rows+"\n")
	case strings.Contains(sql, "RESTORE DATABASE"):
		if fake.restorePanics {
			panic("pánico simulado")
		}
		if fake.restoreFails {
			write(command.Stderr, "Msg 3241, Level 16, State 0, Server x, Line 1\nThe media family on device '/backup/respaldo.bak' is incorrectly formed. valor-que-no-debe-salir\n")
			return &ExitError{Code: 1}
		}
		write(command.Stderr, "50 percent processed.\n100 percent processed.\n")
	case strings.Contains(sql, "SERVERPROPERTY('ProductVersion')"):
		write(command.Stdout, `{"version":"16.0.4325.4"}`+"\n")
	case strings.Contains(sql, "FROM sys.tables"):
		write(command.Stdout, fake.catalogJSON()+"\n")
	case countFrom.MatchString(sql):
		name := countFrom.FindStringSubmatch(sql)[1]
		write(command.Stdout, fmt.Sprintf(`{"total":%d}`+"\n", len(fake.source.schema[name].Rows)))
	case tableFrom.MatchString(sql):
		name := tableFrom.FindStringSubmatch(sql)[1]
		if fake.onTable != nil {
			fake.onTable(name)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		pageRows := 1 << 30
		if match := pageRowsRe.FindStringSubmatch(sql); match != nil {
			pageRows, _ = strconv.Atoi(match[1])
		}
		return fake.serveTable(command.Stdout, name, pageRows)
	default:
		fake.t.Errorf("SQL inesperado: %s", sql)
		return &ExitError{Code: 1}
	}
	return nil
}

// catalogJSON describe las tablas sintéticas como lo haría el catálogo de SQL Server.
func (fake *fakeDocker) catalogJSON() string {
	type column struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		MaxLength int    `json:"max_length"`
		Precision int    `json:"precision"`
		Scale     int    `json:"scale"`
		Nullable  bool   `json:"nullable"`
		Identity  bool   `json:"identity"`
		Key       int    `json:"pk"`
	}
	type table struct {
		Schema  string   `json:"schema"`
		Name    string   `json:"name"`
		Columns []column `json:"columns"`
	}
	tables := []table{}
	for _, source := range legacysynth.Dataset() {
		entry := table{Schema: "dbo", Name: source.Name}
		for _, sourceColumn := range source.Columns {
			described := column{Name: sourceColumn.Name, Type: legacypkg.BaseType(sourceColumn.Type), Nullable: sourceColumn.Nullable, Identity: sourceColumn.Identity}
			if open := strings.IndexByte(sourceColumn.Type, '('); open >= 0 {
				inside := strings.TrimSuffix(sourceColumn.Type[open+1:], ")")
				first, second, hasScale := strings.Cut(inside, ",")
				switch {
				case inside == "max":
					described.MaxLength = -1
				case hasScale:
					described.Precision, _ = strconv.Atoi(first)
					described.Scale, _ = strconv.Atoi(second)
				default:
					described.MaxLength, _ = strconv.Atoi(first)
					if strings.HasPrefix(described.Type, "n") {
						described.MaxLength *= 2
					}
				}
			}
			for position, key := range source.PrimaryKey {
				if key == sourceColumn.Name {
					described.Key = position + 1
				}
			}
			entry.Columns = append(entry.Columns, described)
		}
		tables = append(tables, entry)
	}
	encoded, _ := json.Marshal(tables)
	return string(encoded)
}

// serveTable escribe la línea de conteos y las páginas de una tabla como FOR JSON: claves c0, c1… y fechas sin
// «.000» cuando los milisegundos son cero.
func (fake *fakeDocker) serveTable(output io.Writer, name string, pageRows int) error {
	info, ok := fake.source.manifest.Table(name)
	if !ok {
		fake.t.Errorf("tabla desconocida %s", name)
		return &ExitError{Code: 1}
	}
	rows := fake.source.rows[name]
	counts := map[string]int64{"total": int64(len(rows))}
	for index, omitted := range info.OmittedColumns {
		counts[fmt.Sprintf("o%d", index)] = omitted.RowsWithValue
	}
	encoded, _ := json.Marshal(counts)
	write(output, string(encoded)+"\n")
	if len(info.Columns) == 0 {
		return nil
	}
	for start := 0; start < len(rows); start += pageRows {
		end := min(start+pageRows, len(rows))
		page := []map[string]any{}
		for _, row := range rows[start:end] {
			object := map[string]any{}
			for index, value := range row {
				if text, ok := value.(string); ok && legacypkg.BaseType(info.Columns[index].Type) == "datetime" {
					value = strings.TrimSuffix(text, ".000")
				}
				object[fmt.Sprintf("c%d", index)] = value
			}
			page = append(page, object)
		}
		encoded, _ := json.Marshal(page)
		if name == fake.brokenTable {
			encoded = encoded[:len(encoded)/2]
		}
		write(output, string(encoded)+"\n")
	}
	return nil
}

// testBackup crea un respaldo «sintético» (solo la firma MTF y relleno; el SQL Server simulado no lo lee).
func testBackup(t *testing.T, directory string) (string, int64) {
	t.Helper()
	path := filepath.Join(directory, "sintetico.bak")
	content := append([]byte("TAPE"), make([]byte, 4092)...)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	return path, int64(len(content))
}

// collector guarda los eventos.
type collector struct {
	mutex  sync.Mutex
	events []any
}

func (events *collector) Emit(event any) {
	events.mutex.Lock()
	defer events.mutex.Unlock()
	events.events = append(events.events, event)
}

func (events *collector) steps() []string {
	events.mutex.Lock()
	defer events.mutex.Unlock()
	steps := []string{}
	for _, event := range events.events {
		if step, ok := event.(StepEvent); ok {
			steps = append(steps, step.Step+":"+step.Status)
		}
	}
	return steps
}
