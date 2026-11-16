package legacyread

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// backupSet es el juego de respaldo que se restaura (RESTORE HEADERONLY).
type backupSet struct {
	Position           int
	DatabaseName       string
	StartedAt          string
	FinishedAt         string
	ServerVersion      string
	DatabaseVersion    int
	CompatibilityLevel int
	Collation          string
	Sets               int
}

// backupFile es un archivo de la base dentro del respaldo (RESTORE FILELISTONLY).
type backupFile struct {
	LogicalName string
	Type        string
	FileID      int
}

var restorePercent = regexp.MustCompile(`^(\d{1,3}) percent processed\.`)

// tableQuery ejecuta un SQL cuya salida es una tabla con encabezados.
func (box *container) tableQuery(ctx context.Context, sql string, timeout time.Duration) ([]map[string]string, string, error) {
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout := newBoundedBuffer(4 << 20)
	stderr := newBoundedBuffer(64 << 10)
	err := box.runner.Run(commandContext, Command{
		Args:   execArguments(box.name, "", tableArguments),
		Stdin:  strings.NewReader("SET NOCOUNT ON;\n" + sql + "\n"),
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return nil, stderr.String(), err
	}
	rows, err := parseTable(stdout.String())
	return rows, stderr.String(), err
}

// jsonQuery ejecuta un SQL que devuelve un solo valor JSON y lo decodifica.
func (box *container) jsonQuery(ctx context.Context, database, sql string, timeout time.Duration, target any) (string, error) {
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout := newBoundedBuffer(maxLineBytes)
	stderr := newBoundedBuffer(64 << 10)
	err := box.runner.Run(commandContext, Command{
		Args:   execArguments(box.name, database, jsonArguments),
		Stdin:  strings.NewReader("SET NOCOUNT ON;\nSET TEXTSIZE 2147483647;\n" + sql + "\n"),
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return stderr.String(), err
	}
	decoder := json.NewDecoder(strings.NewReader(stdout.String()))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return stderr.String(), fmt.Errorf("respuesta JSON inválida: %w", err)
	}
	return stderr.String(), nil
}

// readHeader lee RESTORE HEADERONLY y elige el respaldo completo más reciente del archivo.
func (box *container) readHeader(ctx context.Context) (backupSet, error) {
	rows, stderr, err := box.tableQuery(ctx, "RESTORE HEADERONLY FROM DISK = "+sqlString(backupTarget)+";", 10*time.Minute)
	if ctx.Err() != nil {
		return backupSet{}, fail(CodeCancelled, "", ctx.Err())
	}
	if err != nil {
		return backupSet{}, fail(CodeBackupInvalid, sqlErrorDetail(stderr), err)
	}
	return chooseBackupSet(rows)
}

// chooseBackupSet elige el respaldo completo de la base (BackupType 1) más reciente de los que tenga el archivo. Si el
// archivo tiene respaldos completos de varias bases, no adivina cuál es la del sistema anterior: BACKUP_INVALID.
func chooseBackupSet(rows []map[string]string) (backupSet, error) {
	chosen := backupSet{}
	databases := map[string]bool{}
	for _, row := range rows {
		if atoi(row["BackupType"]) != 1 {
			continue
		}
		databases[strings.ToLower(strings.TrimSpace(row["DatabaseName"]))] = true
		position := atoi(row["Position"])
		if position <= chosen.Position {
			continue
		}
		chosen = backupSet{
			Position:     position,
			DatabaseName: row["DatabaseName"],
			StartedAt:    isoDatetime(row["BackupStartDate"]),
			FinishedAt:   isoDatetime(row["BackupFinishDate"]),
			ServerVersion: fmt.Sprintf("%d.%d.%d",
				atoi(row["SoftwareVersionMajor"]), atoi(row["SoftwareVersionMinor"]), atoi(row["SoftwareVersionBuild"])),
			DatabaseVersion:    atoi(row["DatabaseVersion"]),
			CompatibilityLevel: atoi(row["CompatibilityLevel"]),
			Collation:          row["Collation"],
		}
	}
	if chosen.Position == 0 {
		return backupSet{}, fail(CodeBackupInvalid, "el archivo no tiene un respaldo completo de la base", nil)
	}
	if len(databases) > 1 {
		failure := fail(CodeBackupInvalid, fmt.Sprintf("respaldos completos de %d bases distintas", len(databases)), nil)
		failure.Message = "El archivo tiene respaldos de varias bases de datos. Usa un respaldo que solo tenga la del sistema anterior."
		return backupSet{}, failure
	}
	chosen.Sets = len(rows)
	return chosen, nil
}

// isoDatetime pasa «AAAA-MM-DD HH:MM:SS.mmm» de sqlcmd a «AAAA-MM-DDTHH:MM:SS.mmm».
func isoDatetime(text string) string {
	text = strings.TrimSpace(text)
	if text == "NULL" {
		return ""
	}
	return strings.Replace(text, " ", "T", 1)
}

// readFiles lee RESTORE FILELISTONLY del juego elegido.
func (box *container) readFiles(ctx context.Context, position int) ([]backupFile, error) {
	sql := fmt.Sprintf("RESTORE FILELISTONLY FROM DISK = %s WITH FILE = %d;", sqlString(backupTarget), position)
	rows, stderr, err := box.tableQuery(ctx, sql, 10*time.Minute)
	if ctx.Err() != nil {
		return nil, fail(CodeCancelled, "", ctx.Err())
	}
	if err != nil {
		return nil, fail(CodeBackupInvalid, sqlErrorDetail(stderr), err)
	}
	files := []backupFile{}
	for _, row := range rows {
		files = append(files, backupFile{LogicalName: row["LogicalName"], Type: strings.TrimSpace(row["Type"]), FileID: atoi(row["FileId"])})
	}
	if len(files) == 0 {
		return nil, fail(CodeBackupInvalid, "el respaldo no tiene archivos", nil)
	}
	return files, nil
}

// restoreSQL es el RESTORE DATABASE con un MOVE por archivo a la carpeta de datos del contenedor.
func restoreSQL(position int, files []backupFile) (string, error) {
	moves := []string{}
	for _, file := range files {
		target := ""
		switch file.Type {
		case "D":
			extension := ".ndf"
			if file.FileID == 1 {
				extension = ".mdf"
			}
			target = fmt.Sprintf("%s/data/%s_%d%s", dataTarget, databaseName, file.FileID, extension)
		case "L":
			target = fmt.Sprintf("%s/data/%s_%d.ldf", dataTarget, databaseName, file.FileID)
		case "S", "F":
			target = fmt.Sprintf("%s/data/%s_%d", dataTarget, databaseName, file.FileID)
		default:
			return "", failf(CodeBackupInvalid, nil, "tipo de archivo %q en el respaldo", file.Type)
		}
		moves = append(moves, "MOVE "+sqlString(file.LogicalName)+" TO "+sqlString(target))
	}
	return fmt.Sprintf("RESTORE DATABASE %s FROM DISK = %s WITH FILE = %d, %s, RECOVERY, STATS = 5;",
		sqlName(databaseName), sqlString(backupTarget), position, strings.Join(moves, ", ")), nil
}

// restore restaura la base (con el avance de STATS) y la deja de solo lectura: solo se lee.
func (box *container) restore(ctx context.Context, position int, files []backupFile) error {
	sql, err := restoreSQL(position, files)
	if err != nil {
		return err
	}
	sql += "\nALTER DATABASE " + sqlName(databaseName) + " SET READ_ONLY WITH NO_WAIT;"
	stderr := newLineWriter(func(line string) {
		if match := restorePercent.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			percent, _ := strconv.ParseInt(match[1], 10, 64)
			box.events.Emit(ProgressEvent{Event: "progress", Step: StepRestore, Message: "Restaurando el respaldo", Current: percent, Total: 100})
		}
	})
	commandContext, cancel := context.WithTimeout(ctx, 4*time.Hour)
	defer cancel()
	err = box.runner.Run(commandContext, Command{
		Args:   execArguments(box.name, "", jsonArguments),
		Stdin:  strings.NewReader("SET NOCOUNT ON;\n" + sql + "\n"),
		Stdout: io.Discard,
		Stderr: stderr,
	})
	if ctx.Err() != nil {
		return fail(CodeCancelled, "", ctx.Err())
	}
	if err != nil {
		return fail(CodeRestoreFailed, sqlErrorDetail(stderr.String()), err)
	}
	return nil
}

// readerVersion es la versión del SQL Server temporal.
func (box *container) readerVersion(ctx context.Context) string {
	var result struct {
		Version string `json:"version"`
	}
	sql := "SELECT (SELECT CAST(SERVERPROPERTY('ProductVersion') AS nvarchar(128)) AS [version] FOR JSON PATH, WITHOUT_ARRAY_WRAPPER) AS [j];"
	if _, err := box.jsonQuery(ctx, "", sql, shortTimeout, &result); err != nil {
		return ""
	}
	return result.Version
}
