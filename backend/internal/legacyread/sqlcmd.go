package legacyread

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// sqlcmdScript corre sqlcmd dentro del contenedor. La contraseña se lee ahí, del entorno del propio contenedor
// (nunca pasa por la línea de órdenes de este equipo); -S 127.0.0.1 porque con --network none el nombre localhost no
// resuelve; -C confía en el certificado propio de SQL Server; -b termina con error ante cualquier error de SQL; -f 65001
// es UTF-8; -r 1 manda todos los mensajes (también los informativos) a la salida de errores, y el SQL llega por la
// entrada estándar. Los demás indicadores llegan como argumentos posicionales ("$@"), nunca concatenados al script.
const sqlcmdScript = `tool=/opt/mssql-tools18/bin/sqlcmd
[ -x "$tool" ] || tool=/opt/mssql-tools/bin/sqlcmd
SQLCMDPASSWORD="$MSSQL_SA_PASSWORD" exec "$tool" -S 127.0.0.1 -U sa -C -b -f 65001 -r 1 -i /dev/stdin "$@"`

// columnSeparator separa las columnas de las salidas en forma de tabla (RESTORE HEADERONLY y FILELISTONLY): el
// separador de unidades ASCII, que no aparece en los datos.
const columnSeparator = "\x1f"

// maxLineBytes acota una línea de la salida de sqlcmd (una página de filas en JSON).
const maxLineBytes = 256 << 20

// jsonArguments: con -y 0 sqlcmd no recorta los valores largos (una página de JSON en un solo nvarchar(max)) y no
// imprime encabezados (-y 0 y -h son incompatibles).
var jsonArguments = []string{"-y", "0"}

// tableArguments: con encabezados, sin espacios de relleno (-W), los caracteres de control de los datos como espacios
// (-k2) y el separador propio.
var tableArguments = []string{"-W", "-k2", "-s", columnSeparator}

var sqlErrorNumber = regexp.MustCompile(`Msg (\d+), Level (\d+)`)

// sqlErrorDetail resume un error de sqlcmd sin su texto (puede citar valores del respaldo): solo el número del error.
func sqlErrorDetail(stderr string) string {
	if match := sqlErrorNumber.FindStringSubmatch(stderr); match != nil {
		return "error " + match[1] + " de SQL Server"
	}
	if strings.Contains(stderr, "Login timeout") || strings.Contains(stderr, "Login failed") {
		return "SQL Server no aceptó la conexión"
	}
	return ""
}

func execArguments(container string, database string, extra []string) []string {
	arguments := []string{"exec", "-i", container, "bash", "-c", sqlcmdScript, "bash"}
	if database != "" {
		arguments = append(arguments, "-d", database)
	}
	return append(arguments, extra...)
}

// streamLines ejecuta una orden y entrega cada línea no vacía de su salida estándar (sin el salto de línea) mientras
// la orden corre. Si onLine falla, la orden se cancela.
func streamLines(ctx context.Context, runner Runner, command Command, onLine func(line []byte) error) error {
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	command.Stdout = writer
	done := make(chan error, 1)
	go func() {
		// Un pánico aquí no debe saltarse la limpieza del contenedor: se convierte en error.
		defer func() {
			if recovered := recover(); recovered != nil {
				_ = writer.CloseWithError(errPanic)
				done <- errPanic
			}
		}()
		err := runner.Run(runContext, command)
		_ = writer.CloseWithError(io.EOF)
		done <- err
	}()
	buffered := bufio.NewReaderSize(reader, 1<<20)
	var lineErr error
	for lineErr == nil {
		line, err := readLine(buffered)
		if len(line) > 0 {
			lineErr = onLine(line)
		}
		if err != nil {
			if err != io.EOF {
				lineErr = err
			}
			break
		}
	}
	if lineErr != nil {
		cancel()
		_ = reader.CloseWithError(lineErr)
		<-done
		return lineErr
	}
	return <-done
}

var (
	errLineTooLong = errors.New("línea demasiado larga en la salida de sqlcmd")
	errPanic       = errors.New("falla interna al leer la salida de sqlcmd")
)

// readLine lee una línea completa (sin «\n» ni «\r» final), con el límite de maxLineBytes.
func readLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		line = append(line, fragment...)
		if len(line) > maxLineBytes {
			return nil, errLineTooLong
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		line = bytes.TrimRight(line, "\r\n")
		return line, err
	}
}

// parseTable interpreta una salida de sqlcmd con encabezados (tableArguments): la primera línea son los nombres, la
// segunda los guiones y después una fila por línea hasta una línea vacía.
func parseTable(output string) ([]map[string]string, error) {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start+1 >= len(lines) {
		return nil, errors.New("salida sin encabezados")
	}
	headers := strings.Split(lines[start], columnSeparator)
	if !strings.HasPrefix(lines[start+1], "-") {
		return nil, errors.New("salida sin la línea de guiones")
	}
	rows := []map[string]string{}
	for _, line := range lines[start+2:] {
		if strings.TrimSpace(line) == "" {
			break
		}
		fields := strings.Split(line, columnSeparator)
		if len(fields) != len(headers) {
			return nil, errors.New("una fila no tiene el número de columnas de los encabezados")
		}
		row := map[string]string{}
		for index, header := range headers {
			row[strings.TrimSpace(header)] = fields[index]
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func atoi(text string) int {
	value, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0
	}
	return value
}

// sqlString es un literal N'…' de T-SQL con las comillas duplicadas.
func sqlString(text string) string {
	return "N'" + strings.ReplaceAll(text, "'", "''") + "'"
}

// sqlName es un identificador entre corchetes.
func sqlName(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

// lineWriter entrega a onLine cada línea completa que se le escribe (para seguir el avance en la salida de errores
// de sqlcmd o en la de docker pull). Guarda además las últimas líneas para examinarlas al fallar.
type lineWriter struct {
	onLine  func(line string)
	pending []byte
	tail    *boundedBuffer
}

func newLineWriter(onLine func(line string)) *lineWriter {
	return &lineWriter{onLine: onLine, tail: newBoundedBuffer(64 << 10)}
}

func (writer *lineWriter) Write(data []byte) (int, error) {
	_, _ = writer.tail.Write(data)
	writer.pending = append(writer.pending, data...)
	for {
		index := bytes.IndexByte(writer.pending, '\n')
		if index < 0 {
			break
		}
		line := strings.TrimRight(string(writer.pending[:index]), "\r")
		writer.pending = writer.pending[index+1:]
		if writer.onLine != nil {
			writer.onLine(line)
		}
	}
	if len(writer.pending) > 64<<10 {
		writer.pending = writer.pending[:0]
	}
	return len(data), nil
}

func (writer *lineWriter) String() string {
	return writer.tail.String()
}
