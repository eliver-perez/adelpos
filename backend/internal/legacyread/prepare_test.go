package legacyread

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	if entries, _ := os.ReadDir(run.tempDir); len(entries) != 0 {
		t.Fatalf("quedó en la carpeta temporal: %v", entries)
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
	run.fake.leftoverIDs = "viejo1\nviejo2\n"
	run.fake.leftoverVolumes = "adel-legacy-viejo-data\n"
	summary, err := run.prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.ContentFingerprint != run.fake.source.manifest.ContentFingerprint {
		t.Fatal("el paquete no tiene el contenido del sintético")
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
		source.ReaderImageID != "sha256:1111" || source.ReaderVersion != "16.0.4325.4" || len(source.BackupSHA256) != 64 || source.BackupFile != "sintetico.bak" {
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
	for _, step := range []string{StepImage, StepStart, StepRestore, StepRead, StepWrite, StepCleanup, "total"} {
		if _, ok := summary.Timings[step]; !ok {
			t.Fatalf("falta el tiempo de %s", step)
		}
	}
	calls := run.fake.commands()
	name, runCall := containerName(t, calls)
	expectedRun := []string{
		"run", "-d", "--name", name, "--label", "local.adel.pos.legacy-reader=1", "--platform", "linux/amd64",
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
		"ps -a -q --filter label=local.adel.pos.legacy-reader=1",
		"rm -f -v viejo1 viejo2",
		"volume ls -q --filter label=local.adel.pos.legacy-reader=1",
		"volume rm -f adel-legacy-viejo-data",
		"volume create --label local.adel.pos.legacy-reader=1 " + name + "-data",
	} {
		if !strings.Contains(all, expected) {
			t.Fatalf("falta la orden %q", expected)
		}
	}
	if strings.Contains(all, "adel-legacy-analysis") {
		t.Fatal("se tocó un contenedor ajeno")
	}
	run.requireCleanedUp(t, true)
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
