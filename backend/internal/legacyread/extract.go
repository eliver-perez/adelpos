package legacyread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/legacypkg"
)

// DefaultPageRows es el número de filas por página de JSON (alrededor de 1 MB en las tablas grandes del respaldo).
const DefaultPageRows = 5000

// tableTimeout es el plazo de la lectura de una tabla: muy generoso (la tabla más grande del respaldo real tarda
// segundos), solo para que un SQL Server atascado no deje la preparación esperando para siempre. Es variable para que
// las pruebas lo acorten.
var tableTimeout = 2 * time.Hour

// sourceTable y sourceColumn son una tabla y una columna como las describe el catálogo de SQL Server.
type sourceTable struct {
	Schema  string         `json:"schema"`
	Name    string         `json:"name"`
	Columns []sourceColumn `json:"columns"`
}

type sourceColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	MaxLength  int    `json:"max_length"`
	Precision  int    `json:"precision"`
	Scale      int    `json:"scale"`
	Nullable   bool   `json:"nullable"`
	Identity   bool   `json:"identity"`
	PrimaryKey int    `json:"pk"`
}

// listTablesSQL enumera las tablas de usuario con sus columnas (tipo base, longitud, nulabilidad, identidad y posición
// en la llave primaria). sysdiagrams se incluye aunque SSMS la marque como suya: se registra como omitida.
const listTablesSQL = `SELECT (
  SELECT s.name AS [schema], t.name AS [name],
    (SELECT c.name AS [name],
        CASE WHEN c.system_type_id = 240 THEN TYPE_NAME(c.user_type_id) ELSE TYPE_NAME(c.system_type_id) END AS [type],
        c.max_length AS [max_length], c.precision AS [precision], c.scale AS [scale],
        c.is_nullable AS [nullable], c.is_identity AS [identity],
        ISNULL((SELECT ic.key_ordinal FROM sys.indexes i
          JOIN sys.index_columns ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id
          WHERE i.object_id = t.object_id AND i.is_primary_key = 1 AND ic.column_id = c.column_id), 0) AS [pk]
      FROM sys.columns c WHERE c.object_id = t.object_id ORDER BY c.column_id
      FOR JSON PATH, INCLUDE_NULL_VALUES) AS [columns]
  FROM sys.tables t JOIN sys.schemas s ON s.schema_id = t.schema_id
  WHERE t.is_ms_shipped = 0 OR t.name = N'sysdiagrams'
  FOR JSON PATH, INCLUDE_NULL_VALUES
) AS [j];`

func (box *container) listTables(ctx context.Context) ([]sourceTable, error) {
	var tables []sourceTable
	stderr, err := box.jsonQuery(ctx, databaseName, listTablesSQL, 10*time.Minute, &tables)
	if ctx.Err() != nil {
		return nil, fail(CodeCancelled, "", ctx.Err())
	}
	if err != nil {
		return nil, fail(CodeReadFailed, "catálogo: "+sqlErrorDetail(stderr), err)
	}
	sort.Slice(tables, func(left, right int) bool {
		return packageName(tables[left]) < packageName(tables[right])
	})
	return tables, nil
}

// packageName es el nombre de la tabla en el paquete: el de la tabla en dbo y «esquema.tabla» en otro esquema.
func packageName(table sourceTable) string {
	if strings.EqualFold(table.Schema, "dbo") {
		return table.Name
	}
	return table.Schema + "." + table.Name
}

// columnType es el tipo con su longitud o precisión, como lo escribe el manifiesto («varchar(50)», «numeric(18,3)»).
func columnType(column sourceColumn) string {
	base := strings.ToLower(column.Type)
	switch base {
	case "char", "varchar", "binary", "varbinary":
		if column.MaxLength < 0 {
			return base + "(max)"
		}
		return fmt.Sprintf("%s(%d)", base, column.MaxLength)
	case "nchar", "nvarchar":
		if column.MaxLength < 0 {
			return base + "(max)"
		}
		return fmt.Sprintf("%s(%d)", base, column.MaxLength/2)
	case "decimal", "numeric":
		return fmt.Sprintf("%s(%d,%d)", base, column.Precision, column.Scale)
	case "datetime2", "time", "datetimeoffset":
		return fmt.Sprintf("%s(%d)", base, column.Scale)
	}
	return base
}

func isTextType(base string) bool {
	switch base {
	case "char", "varchar", "nchar", "nvarchar", "text", "ntext", "xml":
		return true
	}
	return false
}

// outputColumn es una columna que viaja: su expresión en SQL Server y su tipo base para validar la salida.
type outputColumn struct {
	name       string
	baseType   string
	nullable   bool
	expression string
	order      []string
}

// omittedColumn es una columna que no viaja, con la expresión que cuenta sus filas con valor.
type omittedColumn struct {
	manifest legacypkg.OmittedColumn
	count    string
}

// tablePlan es cómo se lee una tabla.
type tablePlan struct {
	source  sourceTable
	name    string
	spec    legacypkg.TableSpec
	outputs []outputColumn
	omitted []omittedColumn
	skipped string
}

// planTable aplica las reglas de legacypkg: las contraseñas se sustituyen por <columna>_present (el valor nunca sale
// de SQL Server), los binarios y los tipos no admitidos se omiten y se cuentan, y las tablas excluidas solo se cuentan.
func planTable(source sourceTable) (tablePlan, error) {
	plan := tablePlan{source: source, name: packageName(source)}
	if reason := legacypkg.ExcludedTableReason(plan.name); reason != "" {
		plan.skipped = reason
		return plan, nil
	}
	plan.spec = legacypkg.TableSpec{Name: plan.name}
	names := map[string]bool{}
	keys := map[int]string{}
	keyColumns := 0
	for _, column := range source.Columns {
		names[column.Name] = true
		if column.PrimaryKey > 0 {
			keyColumns++
		}
	}
	for _, column := range source.Columns {
		quoted := sqlName(column.Name)
		base := strings.ToLower(column.Type)
		switch legacypkg.TreatmentFor(column.Name, base) {
		case legacypkg.TreatPassword:
			present := legacypkg.PresentColumn(column.Name)
			if names[present.Name] {
				return tablePlan{}, failf(CodeReadFailed, nil, "la tabla %s ya tiene una columna %s", plan.name, present.Name)
			}
			expression := "CAST(CASE WHEN DATALENGTH(" + quoted + ") > 0 THEN 1 ELSE 0 END AS bit)"
			plan.spec.Columns = append(plan.spec.Columns, present)
			plan.outputs = append(plan.outputs, outputColumn{
				name:       present.Name,
				baseType:   "bit",
				expression: expression,
				order:      []string{expression},
			})
		case legacypkg.TreatBinary, legacypkg.TreatUnsupported:
			reason := legacypkg.ReasonBinary
			if legacypkg.TreatmentFor(column.Name, base) == legacypkg.TreatUnsupported {
				reason = legacypkg.ReasonUnsupported
			}
			plan.omitted = append(plan.omitted, omittedColumn{
				manifest: legacypkg.OmittedColumn{Name: column.Name, Type: columnType(column), Reason: reason},
				count:    "ISNULL(SUM(CASE WHEN DATALENGTH(" + quoted + ") > 0 THEN CAST(1 AS bigint) ELSE 0 END), 0)",
			})
			continue
		default:
			expression := quoted
			switch base {
			case "text", "char", "varchar":
				if base == "text" {
					expression = "CONVERT(varchar(max), " + quoted + ")"
				}
			case "ntext", "xml":
				expression = "CONVERT(nvarchar(max), " + quoted + ")"
			}
			order := []string{expression}
			if isTextType(base) {
				// Orden determinista: bytes (BIN2) y, como SQL Server ignora los espacios del final al comparar, su
				// longitud.
				order = []string{expression + " COLLATE Latin1_General_BIN2", "DATALENGTH(" + expression + ")"}
			}
			plan.spec.Columns = append(plan.spec.Columns, legacypkg.Column{
				Name:     column.Name,
				Type:     columnType(column),
				Nullable: column.Nullable,
				Identity: column.Identity,
			})
			plan.outputs = append(plan.outputs, outputColumn{
				name:       column.Name,
				baseType:   base,
				nullable:   column.Nullable,
				expression: expression,
				order:      order,
			})
			if column.PrimaryKey > 0 {
				keys[column.PrimaryKey] = column.Name
			}
		}
	}
	// La llave primaria solo se usa si todas sus columnas viajan; si no (una llave binaria o de contraseña), las filas
	// se ordenan por todas las columnas, como una tabla sin llave.
	if len(keys) == keyColumns {
		for position := 1; position <= keyColumns; position++ {
			name, ok := keys[position]
			if !ok {
				return tablePlan{}, failf(CodeReadFailed, nil, "llave primaria incompleta en %s", plan.name)
			}
			plan.spec.PrimaryKey = append(plan.spec.PrimaryKey, name)
		}
	}
	for _, omitted := range plan.omitted {
		plan.spec.OmittedColumns = append(plan.spec.OmittedColumns, omitted.manifest)
	}
	return plan, nil
}

// orderBy es el orden de las filas: la llave primaria o, sin ella, todas las columnas que viajan.
func (plan tablePlan) orderBy() string {
	expressions := []string{}
	if len(plan.spec.PrimaryKey) > 0 {
		for _, key := range plan.spec.PrimaryKey {
			for _, output := range plan.outputs {
				if output.name == key {
					expressions = append(expressions, output.order...)
				}
			}
		}
	} else {
		for _, output := range plan.outputs {
			expressions = append(expressions, output.order...)
		}
	}
	if len(expressions) == 0 {
		return "(SELECT NULL)"
	}
	return strings.Join(expressions, ", ")
}

func (plan tablePlan) from() string {
	return sqlName(plan.source.Schema) + "." + sqlName(plan.source.Name)
}

// readSQL copia la tabla ordenada a una tabla temporal con su número de fila y la entrega por páginas: primero una
// línea con el total y los conteos de las columnas omitidas, y después una línea JSON por página. Cada página es un
// solo nvarchar(max) (FOR JSON dentro de una subconsulta), así SQL Server no la parte en trozos de 2033 caracteres.
// Las columnas se llaman c0, c1… en el JSON: así un nombre con puntos o espacios no cambia su forma.
func (plan tablePlan) readSQL(pageRows int) string {
	var sql strings.Builder
	selected := []string{"ROW_NUMBER() OVER (ORDER BY " + plan.orderBy() + ") AS [adel_row]"}
	aliases := []string{}
	for index, output := range plan.outputs {
		alias := fmt.Sprintf("[c%d]", index)
		selected = append(selected, output.expression+" AS "+alias)
		aliases = append(aliases, alias)
	}
	fmt.Fprintf(&sql, "SELECT %s INTO #adel_rows FROM %s;\n", strings.Join(selected, ", "), plan.from())
	sql.WriteString("CREATE UNIQUE CLUSTERED INDEX [adel_rows_order] ON #adel_rows ([adel_row]);\n")
	sql.WriteString("DECLARE @total bigint = (SELECT COUNT_BIG(*) FROM #adel_rows);\n")
	counts := []string{"@total AS [total]"}
	for index, omitted := range plan.omitted {
		counts = append(counts, fmt.Sprintf("(SELECT %s FROM %s) AS [o%d]", omitted.count, plan.from(), index))
	}
	fmt.Fprintf(&sql, "SELECT (SELECT %s FOR JSON PATH, WITHOUT_ARRAY_WRAPPER) AS [j];\n", strings.Join(counts, ", "))
	if len(aliases) == 0 {
		return sql.String()
	}
	sql.WriteString("DECLARE @from bigint = 0;\n")
	sql.WriteString("WHILE @from < @total\nBEGIN\n")
	fmt.Fprintf(&sql, "  SELECT (SELECT %s FROM #adel_rows\n", strings.Join(aliases, ", "))
	fmt.Fprintf(&sql, "    WHERE [adel_row] > @from AND [adel_row] <= @from + %d ORDER BY [adel_row]\n", pageRows)
	sql.WriteString("    FOR JSON PATH, INCLUDE_NULL_VALUES) AS [j];\n")
	fmt.Fprintf(&sql, "  SET @from = @from + %d;\nEND;\n", pageRows)
	return sql.String()
}

// countSQL cuenta las filas de una tabla excluida.
func (plan tablePlan) countSQL() string {
	return fmt.Sprintf("SELECT (SELECT COUNT_BIG(*) AS [total] FROM %s FOR JSON PATH, WITHOUT_ARRAY_WRAPPER) AS [j];", plan.from())
}

var datetimeText = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}$`)

// normalizeValue valida un valor de FOR JSON según su tipo y lo deja como lo escribe el paquete. datetime y
// smalldatetime llevan siempre sus milisegundos (FOR JSON los omite cuando son cero).
func normalizeValue(output outputColumn, value any) (any, error) {
	if value == nil {
		if !output.nullable && output.baseType != "bit" {
			return nil, fmt.Errorf("nulo en la columna %s, que no admite nulos", output.name)
		}
		return nil, nil
	}
	switch output.baseType {
	case "bigint", "int", "smallint", "tinyint", "decimal", "numeric", "money", "smallmoney", "float", "real":
		if _, ok := value.(json.Number); !ok {
			return nil, fmt.Errorf("la columna %s no trae un número", output.name)
		}
		return value, nil
	case "bit":
		if _, ok := value.(bool); !ok {
			return nil, fmt.Errorf("la columna %s no trae un booleano", output.name)
		}
		return value, nil
	case "datetime", "smalldatetime":
		text, ok := value.(string)
		if ok && len(text) == len("2006-01-02T15:04:05") {
			text += ".000"
		}
		if !ok || !datetimeText.MatchString(text) {
			return nil, fmt.Errorf("la columna %s no trae una fecha y hora", output.name)
		}
		return text, nil
	}
	if _, ok := value.(string); !ok {
		return nil, fmt.Errorf("la columna %s no trae un texto", output.name)
	}
	return value, nil
}

// tableReading es el avance de la lectura de todas las tablas.
type tableReading struct {
	done  int
	total int
}

// readTable lee una tabla en el paquete y devuelve su conteo.
func (box *container) readTable(
	ctx context.Context,
	plan tablePlan,
	writer *legacypkg.Writer,
	pageRows int,
	progress tableReading,
	sales *SalesRange,
) (int64, error) {
	tableWriter, err := writer.BeginTable(plan.spec)
	if err != nil {
		return 0, fail(CodeWriteFailed, "", err)
	}
	var total int64 = -1
	var rows int64
	salesColumn := -1
	if plan.name == legacypkg.SalesTable {
		for index, output := range plan.outputs {
			if output.name == legacypkg.SalesDateColumn && (output.baseType == "datetime" || output.baseType == "smalldatetime") {
				salesColumn = index
			}
		}
	}
	stderr := newBoundedBuffer(64 << 10)
	onLine := func(line []byte) error {
		if total < 0 {
			counts := map[string]int64{}
			if err := json.Unmarshal(line, &counts); err != nil {
				return failf(CodeReadFailed, err, "conteos de %s", plan.name)
			}
			total = counts["total"]
			// Los conteos de las columnas omitidas llegan con el total: el manifiesto los lleva.
			for index, omitted := range plan.omitted {
				if err := tableWriter.SetOmittedRows(omitted.manifest.Name, counts[fmt.Sprintf("o%d", index)]); err != nil {
					return fail(CodeWriteFailed, "", err)
				}
			}
			return nil
		}
		page, err := decodePage(line, plan.outputs)
		if err != nil {
			return failf(CodeReadFailed, err, "página de %s", plan.name)
		}
		for _, values := range page {
			if salesColumn >= 0 {
				if moment, ok := values[salesColumn].(string); ok {
					sales.include(moment)
				}
			}
			if err := tableWriter.WriteRow(values); err != nil {
				return failf(CodeWriteFailed, err, "fila de %s", plan.name)
			}
		}
		rows += int64(len(page))
		if rows > total {
			return failf(CodeReadFailed, nil, "%s entregó más filas que su conteo", plan.name)
		}
		box.events.Emit(TableEvent{
			Event:     "table",
			Table:     plan.name,
			Status:    "reading",
			Done:      progress.done,
			Total:     progress.total,
			Rows:      rows,
			TableRows: total,
		})
		return nil
	}
	tableContext, cancel := context.WithTimeout(ctx, tableTimeout)
	defer cancel()
	err = streamLines(tableContext, box.runner, Command{
		Args:   execArguments(box.name, databaseName, jsonArguments),
		Stdin:  strings.NewReader("SET NOCOUNT ON;\nSET TEXTSIZE 2147483647;\n" + plan.readSQL(pageRows)),
		Stderr: stderr,
	}, onLine)
	if ctx.Err() != nil {
		return 0, fail(CodeCancelled, "", ctx.Err())
	}
	if errors.Is(tableContext.Err(), context.DeadlineExceeded) {
		return 0, failf(CodeReadFailed, err, "se agotó el plazo de lectura de %s", plan.name)
	}
	if err != nil {
		var failure *Failure
		if errors.As(err, &failure) {
			return 0, failure
		}
		return 0, failf(CodeReadFailed, err, "%s: %s", plan.name, sqlErrorDetail(stderr.String()))
	}
	if total < 0 {
		return 0, failf(CodeReadFailed, nil, "%s no entregó su conteo", plan.name)
	}
	if len(plan.outputs) == 0 {
		// Sin columnas que viajen (todas binarias): filas vacías, tantas como tenga la tabla.
		for ; rows < total; rows++ {
			if err := tableWriter.WriteRow([]any{}); err != nil {
				return 0, fail(CodeWriteFailed, "", err)
			}
		}
	}
	if rows != total {
		return 0, failf(CodeReadFailed, nil, "%s entregó %d filas de %d", plan.name, rows, total)
	}
	if err := tableWriter.Close(); err != nil {
		return 0, fail(CodeWriteFailed, "", err)
	}
	return rows, nil
}

// decodePage decodifica una página de FOR JSON (o «NULL» si no tuvo filas) en filas con un valor por columna.
func decodePage(line []byte, outputs []outputColumn) ([][]any, error) {
	if bytes.Equal(bytes.TrimSpace(line), []byte("NULL")) {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var objects []map[string]any
	if err := decoder.Decode(&objects); err != nil {
		return nil, err
	}
	if decoder.More() {
		return nil, errors.New("datos de más después de la página")
	}
	rows := make([][]any, 0, len(objects))
	for _, object := range objects {
		if len(object) != len(outputs) {
			return nil, errors.New("una fila no tiene las columnas esperadas")
		}
		values := make([]any, len(outputs))
		for index, output := range outputs {
			raw, ok := object[fmt.Sprintf("c%d", index)]
			if !ok {
				return nil, errors.New("falta una columna en una fila")
			}
			value, err := normalizeValue(output, raw)
			if err != nil {
				return nil, err
			}
			values[index] = value
		}
		rows = append(rows, values)
	}
	return rows, nil
}

// countTable cuenta las filas de una tabla excluida.
func (box *container) countTable(ctx context.Context, plan tablePlan) (int64, error) {
	var counts struct {
		Total int64 `json:"total"`
	}
	stderr, err := box.jsonQuery(ctx, databaseName, plan.countSQL(), time.Hour, &counts)
	if ctx.Err() != nil {
		return 0, fail(CodeCancelled, "", ctx.Err())
	}
	if err != nil {
		return 0, failf(CodeReadFailed, err, "conteo de %s: %s", plan.name, sqlErrorDetail(stderr))
	}
	return counts.Total, nil
}

// include amplía el rango de fechas de las ventas.
func (sales *SalesRange) include(moment string) {
	if sales.First == "" || moment < sales.First {
		sales.First = moment
	}
	if moment > sales.Last {
		sales.Last = moment
	}
}
