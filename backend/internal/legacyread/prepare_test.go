package legacyread

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/legacypkg"
)

type preparation struct {
	fake      *fakeDocker
	events    *collector
	preparer  *Preparer
	options   Options
	directory string
	tempDir   string
}

func newPreparation(t *testing.T) *preparation {
	t.Helper()
	directory := t.TempDir()
	tempDir := t.TempDir()
	backup, size := testBackup(t, directory)
	fake := newFakeDocker(t)
	fake.backupSize = size
	events := &collector{}
	return &preparation{
		fake:   fake,
		events: events,
		preparer: &Preparer{
			Runner:  fake,
			Events:  events,
			TempDir: tempDir,
			Sleep: func(ctx context.Context, duration time.Duration) error {
				return ctx.Err()
			},
			ProcessAlive: testProcessAlive,
		},
		options: Options{
			Backup:     backup,
			Output:     filepath.Join(directory, "migracion.adelmig"),
			Passphrase: testPassphrase,
			PageRows:   7,
			KDF:        testKDF,
		},
		directory: directory,
		tempDir:   tempDir,
	}
}

// livePID es el PID de una preparación «en curso» de otro proceso en las pruebas; los demás PID se dan por terminados.
const livePID = 111

func testProcessAlive(pid int) bool {
	return pid == livePID || pid == os.Getpid()
}

func (run *preparation) prepare(ctx context.Context) (Summary, error) {
	return run.preparer.Prepare(ctx, run.options)
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("se esperaba el error %s", code)
	}
	if failure := AsFailure(err); failure.Code != code {
		t.Fatalf("se esperaba %s y llegó %s", code, failure.Error())
	}
}

// containerName es el nombre del contenedor de la preparación (del docker run).
func containerName(t *testing.T, calls []recorded) (string, recorded) {
	t.Helper()
	for _, call := range calls {
		if len(call.Args) > 3 && call.Args[0] == "run" {
			return call.Args[3], call
		}
	}
	t.Fatal("no hubo docker run")
	return "", recorded{}
}

// requireCleanedUp comprueba que al final se eliminaron el contenedor, su volumen, la copia del respaldo, las tablas
// en preparación y el paquete a medias.
func (run *preparation) requireCleanedUp(t *testing.T, expectPackage bool) {
	t.Helper()
	calls := run.fake.commands()
	name, _ := containerName(t, calls)
	removedContainer, removedVolume := false, false
	for _, call := range calls {
		joined := strings.Join(call.Args, " ")
		if joined == "rm -f -v "+name {
			removedContainer = true
		}
		if joined == "volume rm -f "+name+"-data" {
			removedVolume = true
		}
	}
	if !removedContainer || !removedVolume {
		t.Fatalf("no se eliminó el contenedor (%v) o su volumen (%v)", removedContainer, removedVolume)
	}
	// En la carpeta temporal solo queda la carpeta de trabajo con su bloqueo.
	if entries, _ := os.ReadDir(run.tempDir); len(entries) != 1 || entries[0].Name() != workspaceName {
		t.Fatalf("quedó en la carpeta temporal: %v", entries)
	}
	if entries, _ := os.ReadDir(filepath.Join(run.tempDir, workspaceName)); len(entries) != 1 || entries[0].Name() != lockName {
		t.Fatalf("quedó en la carpeta de trabajo: %v", entries)
	}
	entries, _ := os.ReadDir(run.directory)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".partial") {
			t.Fatalf("quedó el paquete a medias: %s", entry.Name())
		}
	}
	_, err := os.Stat(run.options.Output)
	if expectPackage != (err == nil) {
		t.Fatalf("paquete esperado %v, existe %v", expectPackage, err == nil)
	}
}

// El lector completo contra el SQL Server simulado: el paquete tiene exactamente el contenido del sintético (misma
// huella), los argumentos de docker son los exactos y al terminar no queda nada.
func TestPrepareWritesTheExpectedPackageAndCleansUp(t *testing.T) {
	run := newPreparation(t)
	now := time.Now().Unix()
	old := time.Now().Add(-13 * time.Hour).Unix()
	// viejo1: de otra preparación en curso (su proceso vive): no se toca. viejo2: su proceso terminó. viejo3: sin PID
	// (versión anterior). viejo4: su PID existe, pero es de hace más de 12 horas (PID reutilizado).
	run.fake.leftoverIDs = fmt.Sprintf("viejo1|%d|%d\nviejo2|222|%d\nviejo3||\nviejo4|%d|%d\n", livePID, now, now, livePID, old)
	run.fake.leftoverVolumes = fmt.Sprintf("adel-legacy-vivo-data|%d|%d\nadel-legacy-viejo-data|222|%d\n", livePID, now, now)
	summary, err := run.prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.ContentFingerprint != run.fake.source.manifest.ContentFingerprint {
		t.Fatal("el paquete no tiene el contenido del sintético")
	}
	if !summary.Verified || summary.Backup.Sets != 1 || summary.PreviousLeftovers.Containers != 3 || summary.PreviousLeftovers.Volumes != 1 {
		t.Fatalf("resumen: verificado %v, juegos %d, restos %+v", summary.Verified, summary.Backup.Sets, summary.PreviousLeftovers)
	}
	file, err := os.Open(run.options.Output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	manifest, err := legacypkg.Verify(file, testPassphrase, legacypkg.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manifest.Tables, run.fake.source.manifest.Tables) || !reflect.DeepEqual(manifest.SkippedTables, run.fake.source.manifest.SkippedTables) {
		t.Fatal("las tablas del manifiesto no son las del sintético")
	}
	if !reflect.DeepEqual(manifest.Summary, run.fake.source.manifest.Summary) {
		t.Fatalf("resumen: %+v", manifest.Summary)
	}
	source := manifest.Source
	if source.Kind != legacypkg.SourceSQLServerBackup || source.DatabaseName != "possys" || source.ServerVersion != "11.0.7001" ||
		source.BackupFinishedAt != "2026-11-14T22:00:09.123" || source.CompatibilityLevel != 110 || source.ReaderImage != "mcr.microsoft.com/mssql/server@sha256:2222" ||
		source.ReaderImageID != "sha256:1111" || source.ReaderVersion != "16.0.4325.4" || source.BackupSHA256 != fileSHA256(t, run.options.Backup) ||
		source.BackupFile != "sintetico.bak" || source.BackupSets != 1 {
		t.Fatalf("origen: %+v", source)
	}
	expectedSteps := []string{"image:started", "image:finished", "start:started", "start:finished", "restore:started",
		"restore:finished", "read:started", "read:finished", "write:started", "write:finished", "cleanup:started", "cleanup:finished"}
	if steps := run.events.steps(); !reflect.DeepEqual(steps, expectedSteps) {
		t.Fatalf("pasos: %v", steps)
	}
	if summary.Sales == nil || summary.Sales.First != manifest.Summary.SalesFirst || len(summary.Tables) != 50 || len(summary.SkippedTables) != 2 {
		t.Fatalf("resumen: %+v", summary)
	}
	for _, step := range []string{StepImage, StepStart, StepRestore, StepRead, StepWrite, StepCleanup, "verify", "total"} {
		if _, ok := summary.Timings[step]; !ok {
			t.Fatalf("falta el tiempo de %s", step)
		}
	}
	calls := run.fake.commands()
	name, runCall := containerName(t, calls)
	started := ""
	for _, argument := range runCall.Args {
		if strings.HasPrefix(argument, ReaderStartedLabel+"=") {
			started = argument
		}
	}
	if seconds, err := strconv.ParseInt(strings.TrimPrefix(started, ReaderStartedLabel+"="), 10, 64); err != nil || time.Since(time.Unix(seconds, 0)) > time.Minute {
		t.Fatalf("etiqueta de la hora: %q", started)
	}
	pidLabel := ReaderPIDLabel + "=" + strconv.Itoa(os.Getpid())
	expectedRun := []string{
		"run", "-d", "--name", name, "--label", "local.adel.pos.legacy-reader=1", "--label", pidLabel, "--label", started,
		"--platform", "linux/amd64",
		"--pull", "never", "--network", "none", "-e", "ACCEPT_EULA=Y", "-e", "MSSQL_PID=Express", "-e", "MSSQL_SA_PASSWORD",
		"--mount", "type=bind,source=" + mustResolve(t, run.options.Backup) + ",target=/backup/respaldo.bak,readonly",
		"--mount", "type=volume,source=" + name + "-data,target=/var/opt/mssql",
		"sha256:1111",
	}
	if !reflect.DeepEqual(runCall.Args, expectedRun) {
		t.Fatalf("docker run:\n%v\n%v", runCall.Args, expectedRun)
	}
	if len(runCall.Env) != 1 || !strings.HasPrefix(runCall.Env[0], "MSSQL_SA_PASSWORD=Aa1!") || len(runCall.Env[0]) < 40 {
		t.Fatalf("la contraseña no llegó por el entorno: %v", runCall.Env)
	}
	password := strings.TrimPrefix(runCall.Env[0], "MSSQL_SA_PASSWORD=")
	for _, call := range calls {
		if strings.Contains(strings.Join(call.Args, " "), password) || strings.Contains(call.Stdin, password) {
			t.Fatalf("la contraseña apareció en una orden: %v", call.Args)
		}
		if call.Args[0] != "run" && len(call.Env) != 0 {
			t.Fatalf("variables de entorno en otra orden: %v", call.Args)
		}
	}
	// Limpieza de restos al empezar: solo lo que lleva la etiqueta del lector.
	joined := []string{}
	for _, call := range calls {
		joined = append(joined, strings.Join(call.Args, " "))
	}
	all := strings.Join(joined, "\n")
	for _, expected := range []string{
		"context inspect --format {{.Endpoints.docker.Host}}",
		"ps -a --filter label=local.adel.pos.legacy-reader=1 --format {{.ID}}" + labelFormat,
		"rm -f -v viejo2 viejo3 viejo4",
		"volume ls --filter label=local.adel.pos.legacy-reader=1 --format {{.Name}}" + labelFormat,
		"volume rm -f adel-legacy-viejo-data",
		"volume create --label local.adel.pos.legacy-reader=1 --label " + pidLabel + " --label " + started + " " + name + "-data",
	} {
		if !strings.Contains(all, expected) {
			t.Fatalf("falta la orden %q", expected)
		}
	}
	if strings.Contains(all, "viejo1") && strings.Contains(all, "rm -f -v viejo1") || strings.Contains(all, "adel-legacy-vivo-data\n") ||
		strings.Contains(all, "volume rm -f adel-legacy-vivo-data") {
		t.Fatal("se borró el lector de una preparación en curso")
	}
	if strings.Contains(all, "adel-legacy-analysis") {
		t.Fatal("se tocó un contenedor ajeno")
	}
	run.requireCleanedUp(t, true)
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func mustResolve(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// Las contraseñas del sistema anterior nunca se seleccionan: la consulta solo pide si había valor.
func TestPasswordColumnsNeverLeaveSQLServer(t *testing.T) {
	run := newPreparation(t)
	if _, err := run.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, call := range run.fake.commands() {
		if !strings.Contains(call.Stdin, "FROM [dbo].[usuarios]") {
			continue
		}
		found = true
		withoutCheck := strings.ReplaceAll(call.Stdin, "DATALENGTH([password])", "")
		if strings.Contains(withoutCheck, "[password]") {
			t.Fatalf("la consulta de usuarios lee la contraseña:\n%s", call.Stdin)
		}
		if !strings.Contains(call.Stdin, "CAST(CASE WHEN DATALENGTH([password]) > 0 THEN 1 ELSE 0 END AS bit) AS [c2]") {
			t.Fatalf("la consulta de usuarios no sustituye la contraseña:\n%s", call.Stdin)
		}
	}
	if !found {
		t.Fatal("no se leyó la tabla de usuarios")
	}
	for _, call := range run.fake.commands() {
		if strings.Contains(call.Stdin, "#adel_rows FROM [dbo].[personal_detalles]") || strings.Contains(call.Stdin, "#adel_rows FROM [dbo].[sysdiagrams]") {
			t.Fatal("se leyó una tabla excluida")
		}
		if strings.Contains(call.Stdin, "[portada] AS") {
			t.Fatal("se leyó una columna binaria")
		}
	}
}

func TestPrepareStopsEarlyOnBadInputs(t *testing.T) {
	t.Run("destino existente", func(t *testing.T) {
		run := newPreparation(t)
		if err := os.WriteFile(run.options.Output, []byte("del propietario"), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeOutputExists)
		if len(run.fake.commands()) != 0 {
			t.Fatal("se llamó a docker")
		}
		content, _ := os.ReadFile(run.options.Output)
		if string(content) != "del propietario" {
			t.Fatal("se tocó el archivo existente")
		}
	})
	t.Run("otra terminación", func(t *testing.T) {
		run := newPreparation(t)
		run.options.Output = filepath.Join(run.directory, "paquete.zip")
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeOutputInvalid)
	})
	t.Run("respaldo inexistente", func(t *testing.T) {
		run := newPreparation(t)
		run.options.Backup = filepath.Join(run.directory, "no-existe.bak")
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeBackupUnreadable)
	})
	t.Run("no es un respaldo", func(t *testing.T) {
		run := newPreparation(t)
		if err := os.WriteFile(run.options.Backup, []byte("texto cualquiera"), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeBackupInvalid)
	})
	t.Run("frase corta", func(t *testing.T) {
		run := newPreparation(t)
		run.options.Passphrase = "corta"
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodePassphraseInvalid)
	})
}

func TestDockerProblems(t *testing.T) {
	t.Run("detenido", func(t *testing.T) {
		run := newPreparation(t)
		run.fake.infoFails = true
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeDockerNotRunning)
		if !strings.Contains(AsFailure(err).Message, "Ábrelo") {
			t.Fatalf("mensaje: %s", AsFailure(err).Message)
		}
	})
	t.Run("contenedores de Windows", func(t *testing.T) {
		run := newPreparation(t)
		run.fake.infoOS = "windows"
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeDockerUnsupported)
	})
	t.Run("sin memoria", func(t *testing.T) {
		run := newPreparation(t)
		run.fake.stoppedOnStart = true
		run.fake.startLogs = "sqlservr: This program requires a machine with at least 2000 megabytes of memory.\n"
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeReaderMemory)
		run.requireCleanedUp(t, false)
	})
	t.Run("arranque lento", func(t *testing.T) {
		run := newPreparation(t)
		run.fake.notReadyProbes = 3
		if _, err := run.prepare(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("imagen ausente", func(t *testing.T) {
		run := newPreparation(t)
		run.fake.imageMissing = true
		if _, err := run.prepare(context.Background()); err != nil {
			t.Fatal(err)
		}
		pulled := false
		for _, call := range run.fake.commands() {
			if strings.Join(call.Args, " ") == "pull --platform linux/amd64 mcr.microsoft.com/mssql/server:2022-latest" {
				pulled = true
			}
		}
		progress := 0
		for _, event := range run.events.events {
			if update, ok := event.(ProgressEvent); ok && update.Step == StepImage {
				progress++
			}
		}
		if !pulled || progress == 0 {
			t.Fatalf("descarga %v, avance %d", pulled, progress)
		}
	})
}

// Si Docker no puede montar la carpeta del respaldo, se monta una copia privada que se borra al terminar.
func TestMountFallbackUsesAPrivateCopy(t *testing.T) {
	run := newPreparation(t)
	run.fake.mountFailures = 1
	if _, err := run.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	runs := [][]string{}
	for _, call := range run.fake.commands() {
		if call.Args[0] == "run" {
			runs = append(runs, call.Args)
		}
	}
	if len(runs) != 2 {
		t.Fatalf("docker run: %d veces", len(runs))
	}
	second := strings.Join(runs[1], " ")
	if !strings.Contains(second, "source="+mustResolve(t, run.tempDir)) && !strings.Contains(second, "source="+run.tempDir) {
		t.Fatalf("la segunda vez no montó la copia: %s", second)
	}
	if !strings.Contains(second, "respaldo.bak,target=/backup/respaldo.bak,readonly") {
		t.Fatalf("montaje de la copia: %s", second)
	}
	run.requireCleanedUp(t, true)
}

func TestFailuresCleanUpEverything(t *testing.T) {
	t.Run("restauración", func(t *testing.T) {
		run := newPreparation(t)
		run.fake.restoreFails = true
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeRestoreFailed)
		failure := AsFailure(err)
		if strings.Contains(failure.Error(), "valor-que-no-debe-salir") || !strings.Contains(failure.Detail, "3241") {
			t.Fatalf("detalle: %s", failure.Error())
		}
		run.requireCleanedUp(t, false)
	})
	t.Run("página dañada", func(t *testing.T) {
		run := newPreparation(t)
		run.fake.brokenTable = "pagos"
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeReadFailed)
		run.requireCleanedUp(t, false)
	})
	t.Run("pánico", func(t *testing.T) {
		run := newPreparation(t)
		run.fake.restorePanics = true
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeInternal)
		run.requireCleanedUp(t, false)
	})
	t.Run("cancelación a media lectura", func(t *testing.T) {
		run := newPreparation(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		run.fake.onTable = func(name string) {
			if name == "pagos" {
				cancel()
			}
		}
		_, err := run.prepare(ctx)
		requireCode(t, err, CodeCancelled)
		run.requireCleanedUp(t, false)
	})
}

func TestSQLServerOutputParsing(t *testing.T) {
	output := "BackupName" + columnSeparator + "Position\n------" + columnSeparator + "--\nuno | dos" + columnSeparator + "2\n\n"
	rows, err := parseTable(output)
	if err != nil || len(rows) != 1 || rows[0]["BackupName"] != "uno | dos" || rows[0]["Position"] != "2" {
		t.Fatalf("%v %v", rows, err)
	}
	if _, err := parseTable("A" + columnSeparator + "B\n---\nsolo una\n"); err == nil {
		t.Fatal("se aceptó una fila incompleta")
	}
	if detail := sqlErrorDetail("Msg 245, Level 16, State 1\nConversion failed when converting the varchar value 'Juan' to int."); detail != "error 245 de SQL Server" {
		t.Fatalf("detalle: %s", detail)
	}
	outputs := []outputColumn{
		{name: "fecha", baseType: "datetime"},
		{name: "total", baseType: "money"},
		{name: "nombre", baseType: "varchar", nullable: true},
		{name: "password_present", baseType: "bit"},
	}
	page, err := decodePage([]byte(`[{"c0":"2019-10-01T12:34:56","c1":150.0000,"c2":null,"c3":true},{"c0":"2019-10-01T12:34:56.997","c1":-0.0001,"c2":"a|b","c3":false}]`), outputs)
	if err != nil {
		t.Fatal(err)
	}
	if page[0][0] != "2019-10-01T12:34:56.000" || page[1][0] != "2019-10-01T12:34:56.997" || string(page[0][1].(interface{ String() string }).String()) != "150.0000" {
		t.Fatalf("página: %#v", page)
	}
	broken := map[string]string{
		"JSON truncado":      `[{"c0":"2019-10-01T12:34:56"`,
		"columna faltante":   `[{"c0":"2019-10-01T12:34:56","c1":1,"c2":null}]`,
		"importe como texto": `[{"c0":"2019-10-01T12:34:56","c1":"150","c2":null,"c3":true}]`,
		"fecha inválida":     `[{"c0":"ayer","c1":1,"c2":null,"c3":true}]`,
		"nulo no permitido":  `[{"c0":null,"c1":1,"c2":null,"c3":true}]`,
		"datos de más":       `[] []`,
	}
	for name, line := range broken {
		if _, err := decodePage([]byte(line), outputs); err == nil {
			t.Errorf("%s: se aceptó", name)
		}
	}
	if page, err := decodePage([]byte("NULL"), outputs); err != nil || len(page) != 0 {
		t.Fatal("página vacía")
	}
}

func TestPlanTable(t *testing.T) {
	plan, err := planTable(sourceTable{Schema: "dbo", Name: "usuarios", Columns: []sourceColumn{
		{Name: "id", Type: "smallint", Identity: true, PrimaryKey: 1},
		{Name: "usuario", Type: "varchar", MaxLength: 30},
		{Name: "Contraseña_Hash", Type: "varbinary", MaxLength: 64},
		{Name: "foto", Type: "image", MaxLength: 16, Nullable: true},
		{Name: "zona", Type: "geography", MaxLength: -1, Nullable: true},
		{Name: "nota", Type: "ntext", MaxLength: 16, Nullable: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, column := range plan.spec.Columns {
		names = append(names, column.Name+":"+column.Type)
	}
	if !reflect.DeepEqual(names, []string{"id:smallint", "usuario:varchar(30)", "Contraseña_Hash_present:bit", "nota:ntext"}) {
		t.Fatalf("columnas: %v", names)
	}
	if len(plan.spec.OmittedColumns) != 2 || plan.spec.OmittedColumns[0].Reason != legacypkg.ReasonBinary || plan.spec.OmittedColumns[1].Reason != legacypkg.ReasonUnsupported {
		t.Fatalf("omitidas: %+v", plan.spec.OmittedColumns)
	}
	sql := plan.readSQL(100)
	for _, expected := range []string{
		"ROW_NUMBER() OVER (ORDER BY [id]) AS [adel_row]",
		"CONVERT(nvarchar(max), [nota]) AS [c3]",
		"ISNULL(SUM(CASE WHEN DATALENGTH([foto]) > 0 THEN CAST(1 AS bigint) ELSE 0 END), 0)",
		"FOR JSON PATH, INCLUDE_NULL_VALUES",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("el SQL no contiene %q:\n%s", expected, sql)
		}
	}
	noKey, err := planTable(sourceTable{Schema: "ventas", Name: "complementos", Columns: []sourceColumn{
		{Name: "producto", Type: "int"}, {Name: "descripcion", Type: "varchar", MaxLength: 80, Nullable: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if noKey.name != "ventas.complementos" || noKey.orderBy() != "[producto], [descripcion] COLLATE Latin1_General_BIN2, DATALENGTH([descripcion])" {
		t.Fatalf("orden sin llave: %s %s", noKey.name, noKey.orderBy())
	}
	skipped, _ := planTable(sourceTable{Schema: "dbo", Name: "personal_detalles"})
	if skipped.skipped != legacypkg.ReasonPersonalData {
		t.Fatal("tabla excluida")
	}
}

func TestLocatorSearchesTheUsualPlaces(t *testing.T) {
	missing := func(string) (string, error) {
		return "", errors.New("no está en el PATH")
	}
	directory := t.TempDir()
	docker := filepath.Join(directory, ".docker", "bin", "docker")
	if err := os.MkdirAll(filepath.Dir(docker), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docker, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	locator := Locator{GOOS: "darwin", Home: directory, Getenv: os.Getenv, LookPath: missing, Stat: os.Stat}
	found, err := locator.Find()
	if err != nil || found != docker {
		t.Fatalf("encontrado %q, %v", found, err)
	}
	candidates := locator.Candidates()
	if !reflect.DeepEqual(candidates[1:], []string{"/usr/local/bin/docker", "/opt/homebrew/bin/docker", "/Applications/Docker.app/Contents/Resources/bin/docker"}) {
		t.Fatalf("candidatos: %v", candidates)
	}
	windows := Locator{GOOS: "windows", Getenv: func(name string) string {
		if name == "ProgramFiles" {
			return `D:\Programas`
		}
		return ""
	}}
	if candidates := windows.Candidates(); len(candidates) != 1 || !strings.HasSuffix(candidates[0], filepath.Join("Docker", "Docker", "resources", "bin", "docker.exe")) {
		t.Fatalf("Windows: %v", candidates)
	}
	nothing := Locator{GOOS: "darwin", Home: t.TempDir(), Getenv: os.Getenv, LookPath: missing, Stat: func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}}
	_, err = nothing.Find()
	requireCode(t, err, CodeDockerMissing)
	if !strings.Contains(AsFailure(err).Message, "docker.com") {
		t.Fatal("el mensaje no dice cómo instalar Docker Desktop")
	}
}

func TestMountOptionQuotesOddPaths(t *testing.T) {
	if value := mountOption("type=bind", "source=/Volumes/USB, copia/respaldo.bak", "readonly"); value != `type=bind,"source=/Volumes/USB, copia/respaldo.bak",readonly` {
		t.Fatalf("montaje: %s", value)
	}
}

// Una sola preparación a la vez: con el bloqueo tomado por otra, ALREADY_RUNNING sin tocar Docker ni la carpeta.
func TestOnlyOnePreparationAtATime(t *testing.T) {
	run := newPreparation(t)
	other, err := openWorkspace(run.tempDir)
	if err != nil {
		t.Fatal(err)
	}
	busyRun, err := other.newRun()
	if err != nil {
		t.Fatal(err)
	}
	_, err = run.prepare(context.Background())
	requireCode(t, err, CodeAlreadyRunning)
	if len(run.fake.commands()) != 0 {
		t.Fatal("se llamó a docker con otra preparación en curso")
	}
	if _, err := os.Stat(busyRun); err != nil {
		t.Fatal("se borró la carpeta de la preparación en curso")
	}
	_, _, err = run.preparer.Cleanup(context.Background())
	requireCode(t, err, CodeAlreadyRunning)
	if err := other.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := run.prepare(context.Background()); err != nil {
		t.Fatalf("después de soltar el bloqueo: %v", err)
	}
}

// staleRun deja en la carpeta de trabajo lo que deja una preparación terminada a la fuerza: su carpeta con tablas en
// preparación y la copia del respaldo, y el paquete a medias junto al destino (registrado en la carpeta).
func staleRun(t *testing.T, run *preparation) (string, string) {
	t.Helper()
	root := filepath.Join(run.tempDir, workspaceName)
	stale := filepath.Join(root, "run-viejo")
	for _, directory := range []string{filepath.Join(stale, "adel-migracion-1"), filepath.Join(stale, "adel-respaldo-1")} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(stale, "adel-respaldo-1", "respaldo.bak"), []byte("TAPE copia"), 0600); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(run.directory, ".migracion-anterior.adelmig.0123456789ab.partial")
	if err := os.WriteFile(partial, []byte("ADELMIG\x00a medias"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, partialRecord), []byte(partial+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return stale, partial
}

func TestStaleFilesFromAKilledPreparationAreRemoved(t *testing.T) {
	t.Run("al preparar", func(t *testing.T) {
		run := newPreparation(t)
		stale, partial := staleRun(t, run)
		summary, err := run.prepare(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Fatal("quedó la carpeta de la preparación anterior")
		}
		if _, err := os.Stat(partial); !os.IsNotExist(err) {
			t.Fatal("quedó el paquete a medias")
		}
		if summary.PreviousLeftovers.Folders != 1 || summary.PreviousLeftovers.Partials != 1 {
			t.Fatalf("restos: %+v", summary.PreviousLeftovers)
		}
		run.requireCleanedUp(t, true)
	})
	t.Run("con --cleanup", func(t *testing.T) {
		run := newPreparation(t)
		stale, partial := staleRun(t, run)
		run.fake.leftoverIDs = fmt.Sprintf("muerto|222|%d\nvivo|%d|%d\n", time.Now().Unix(), livePID, time.Now().Unix())
		report, checked, err := run.preparer.Cleanup(context.Background())
		if err != nil || !checked {
			t.Fatalf("limpieza: %v %v", checked, err)
		}
		if report.Folders != 1 || report.Partials != 1 || report.Containers != 1 {
			t.Fatalf("informe: %+v", report)
		}
		for _, path := range []string{stale, partial} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("quedó %s", path)
			}
		}
		removed := false
		for _, call := range run.fake.commands() {
			joined := strings.Join(call.Args, " ")
			if joined == "rm -f -v muerto" {
				removed = true
			}
			if strings.Contains(joined, "vivo") || call.Args[0] == "run" {
				t.Fatalf("orden inesperada: %s", joined)
			}
		}
		if !removed {
			t.Fatal("no se borró el lector cuyo proceso terminó")
		}
	})
	t.Run("sin Docker", func(t *testing.T) {
		run := newPreparation(t)
		stale, _ := staleRun(t, run)
		run.fake.infoFails = true
		report, checked, err := run.preparer.Cleanup(context.Background())
		if err != nil || checked || report.Folders != 1 {
			t.Fatalf("limpieza sin Docker: %+v %v %v", report, checked, err)
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Fatal("quedó la carpeta")
		}
	})
}

// Docker debe ser el de esta computadora: un DOCKER_HOST o un contexto remoto se rechazan antes de contactarlo.
func TestRemoteDockerIsRefused(t *testing.T) {
	for _, endpoint := range []string{"tcp://192.0.2.1:2375", "ssh://usuario@192.0.2.1", ""} {
		run := newPreparation(t)
		run.fake.endpoint = endpoint
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeDockerRemote)
		for _, call := range run.fake.commands() {
			if call.Args[0] != "context" {
				t.Fatalf("se contactó al Docker remoto: %v", call.Args)
			}
		}
	}
	local := newPreparation(t)
	local.fake.endpoint = "npipe:////./pipe/docker_engine"
	if _, err := local.prepare(context.Background()); err != nil {
		t.Fatalf("Docker de Windows: %v", err)
	}
}

// Si el respaldo cambia mientras se lee (se seguía copiando), el paquete no se escribe.
func TestBackupChangedDuringThePreparation(t *testing.T) {
	run := newPreparation(t)
	run.fake.onRun = func() {
		file, err := os.OpenFile(run.options.Backup, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = file.Write([]byte("más datos"))
		_ = file.Close()
	}
	_, err := run.prepare(context.Background())
	requireCode(t, err, CodeBackupChanged)
	run.requireCleanedUp(t, false)
	// Si el contenedor ya ve el respaldo con otro tamaño, el error también es BACKUP_CHANGED (no «no se pudo iniciar»).
	seen := newPreparation(t)
	seen.fake.onRun = func() {
		_ = os.WriteFile(seen.options.Backup, append([]byte("TAPE"), make([]byte, 9000)...), 0600)
	}
	seen.fake.backupSize = 9004
	_, err = seen.prepare(context.Background())
	requireCode(t, err, CodeBackupChanged)
	seen.requireCleanedUp(t, false)
}

func TestBackupSets(t *testing.T) {
	t.Run("varios juegos de la misma base", func(t *testing.T) {
		run := newPreparation(t)
		run.fake.headerRows = []string{headerRow("1", "1", "possys"), headerRow("5", "2", "possys"), headerRow("1", "3", "POSSYS")}
		summary, err := run.prepare(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if summary.Backup.Sets != 3 {
			t.Fatalf("juegos: %d", summary.Backup.Sets)
		}
		restored := false
		for _, call := range run.fake.commands() {
			if strings.Contains(call.Stdin, "RESTORE DATABASE") && strings.Contains(call.Stdin, "WITH FILE = 3,") {
				restored = true
			}
		}
		if !restored {
			t.Fatal("no se restauró el juego completo más reciente")
		}
	})
	t.Run("varias bases", func(t *testing.T) {
		run := newPreparation(t)
		run.fake.headerRows = []string{headerRow("1", "1", "possys"), headerRow("1", "2", "contabilidad")}
		_, err := run.prepare(context.Background())
		requireCode(t, err, CodeBackupInvalid)
		if !strings.Contains(AsFailure(err).Message, "varias bases") {
			t.Fatalf("mensaje: %s", AsFailure(err).Message)
		}
		run.requireCleanedUp(t, false)
	})
}

// Un SQL Server atascado en una tabla agota el plazo de esa tabla (READ_FAILED) en lugar de esperar para siempre.
func TestTableTimeout(t *testing.T) {
	previous := tableTimeout
	tableTimeout = 200 * time.Millisecond
	defer func() {
		tableTimeout = previous
	}()
	run := newPreparation(t)
	run.fake.blockTable = "pagos"
	_, err := run.prepare(context.Background())
	requireCode(t, err, CodeReadFailed)
	if !strings.Contains(AsFailure(err).Detail, "plazo") || !strings.Contains(AsFailure(err).Detail, "pagos") {
		t.Fatalf("detalle: %s", AsFailure(err).Detail)
	}
	run.requireCleanedUp(t, false)
}
