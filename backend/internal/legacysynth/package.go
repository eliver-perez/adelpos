package legacysynth

import (
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/legacypkg"
	"local/ad-el-pos/server/internal/product"
)

// Datos del respaldo sintético en el manifiesto.
const (
	DatabaseName = "possys"
	BackupFile   = "sintetico.bak"
	// CreatedAt es la fecha fija de creación de los paquetes sintéticos (así dos paquetes sintéticos tienen el mismo
	// manifiesto salvo el cifrado).
	CreatedAt = "2026-11-15T12:00:00Z"
)

// projected es una tabla tal como la escribe el lector: columnas con contraseñas sustituidas y binarios omitidos, y
// filas con los valores del paquete en el orden de SQL Server.
type projected struct {
	spec    legacypkg.TableSpec
	rows    [][]any
	skipped *legacypkg.SkippedTable
}

type keptColumn struct {
	index     int
	source    Column
	treatment legacypkg.ColumnTreatment
}

// project aplica a una tabla las mismas reglas que el lector (legacypkg.TreatmentFor y ExcludedTableReason).
func project(table *Table) projected {
	if reason := legacypkg.ExcludedTableReason(table.Name); reason != "" {
		return projected{skipped: &legacypkg.SkippedTable{Name: table.Name, Rows: int64(len(table.Rows)), Reason: reason}}
	}
	spec := legacypkg.TableSpec{Name: table.Name, PrimaryKey: table.PrimaryKey}
	kept := []keptColumn{}
	for index, source := range table.Columns {
		treatment := legacypkg.TreatmentFor(source.Name, legacypkg.BaseType(source.Type))
		switch treatment {
		case legacypkg.TreatKeep:
			spec.Columns = append(spec.Columns, legacypkg.Column{
				Name:     source.Name,
				Type:     source.Type,
				Nullable: source.Nullable,
				Identity: source.Identity,
			})
			kept = append(kept, keptColumn{index: index, source: source, treatment: treatment})
		case legacypkg.TreatPassword:
			spec.Columns = append(spec.Columns, legacypkg.PresentColumn(source.Name))
			kept = append(kept, keptColumn{index: index, source: source, treatment: treatment})
		default:
			reason := legacypkg.ReasonBinary
			if treatment == legacypkg.TreatUnsupported {
				reason = legacypkg.ReasonUnsupported
			}
			spec.OmittedColumns = append(spec.OmittedColumns, legacypkg.OmittedColumn{
				Name:          source.Name,
				Type:          source.Type,
				Reason:        reason,
				RowsWithValue: rowsWithValue(table.Rows, index),
			})
		}
	}
	// Como el lector: la llave primaria solo ordena si todas sus columnas viajan con su valor.
	for _, key := range table.PrimaryKey {
		for _, source := range table.Columns {
			if source.Name == key && legacypkg.TreatmentFor(source.Name, legacypkg.BaseType(source.Type)) != legacypkg.TreatKeep {
				spec.PrimaryKey = nil
			}
		}
	}
	rows := make([][]any, 0, len(table.Rows))
	for _, raw := range table.Rows {
		row := make([]any, len(kept))
		for position, column := range kept {
			row[position] = packageValue(column, raw[column.index])
		}
		rows = append(rows, row)
	}
	keys := sortKeys(spec)
	sort.SliceStable(rows, func(left, right int) bool {
		return compareRows(spec.Columns, keys, rows[left], rows[right]) < 0
	})
	return projected{spec: spec, rows: rows}
}

// sortKeys son las columnas del orden: la llave primaria o, sin ella, todas.
func sortKeys(spec legacypkg.TableSpec) []int {
	keys := []int{}
	if len(spec.PrimaryKey) > 0 {
		for _, name := range spec.PrimaryKey {
			for index, column := range spec.Columns {
				if column.Name == name {
					keys = append(keys, index)
				}
			}
		}
		return keys
	}
	for index := range spec.Columns {
		keys = append(keys, index)
	}
	return keys
}

// rowsWithValue cuenta las filas con un valor no vacío (DATALENGTH > 0), como el lector.
func rowsWithValue(rows [][]any, index int) int64 {
	var count int64
	for _, row := range rows {
		switch value := row[index].(type) {
		case []byte:
			if len(value) > 0 {
				count++
			}
		case string:
			if value != "" {
				count++
			}
		}
	}
	return count
}

// packageValue es el valor que el lector escribe en el paquete para un valor de la base: números como json.Number con
// su texto exacto (los importes con sus 4 decimales, como FOR JSON de SQL Server), fechas como texto, char(n) con sus
// espacios y, en lugar de una contraseña, si tenía valor.
func packageValue(column keptColumn, value any) any {
	if column.treatment == legacypkg.TreatPassword {
		text, _ := value.(string)
		return text != ""
	}
	if value == nil {
		return nil
	}
	switch legacypkg.BaseType(column.source.Type) {
	case "bigint", "int", "smallint", "tinyint":
		return json.Number(strconv.FormatInt(value.(int64), 10))
	case "money", "smallmoney", "decimal", "numeric":
		return json.Number(value.(string))
	case "bit":
		return value.(bool)
	case "char", "nchar":
		return padded(value.(string), column.source.Type)
	}
	return value.(string)
}

// padded completa con espacios un char(n), como lo devuelve SQL Server.
func padded(text, sqlType string) string {
	open := strings.IndexByte(sqlType, '(')
	length, err := strconv.Atoi(strings.TrimSuffix(sqlType[open+1:], ")"))
	if err != nil {
		panic("legacysynth: longitud de " + sqlType)
	}
	for len([]rune(text)) < length {
		text += " "
	}
	return text
}

// sortedTables son las tablas en el orden en que el lector las escribe (por nombre).
func sortedTables(tables []*Table) []*Table {
	sorted := append([]*Table{}, tables...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left].Name < sorted[right].Name
	})
	return sorted
}

// WritePackage escribe en un paquete las tablas tal como legacy-prepare las extraería del respaldo sintético, y
// devuelve el resumen (rango de fechas de las ventas).
func WritePackage(writer *legacypkg.Writer, tables []*Table) (*legacypkg.Summary, error) {
	var summary *legacypkg.Summary
	for _, table := range sortedTables(tables) {
		result := project(table)
		if result.skipped != nil {
			if err := writer.SkipTable(*result.skipped); err != nil {
				return nil, err
			}
			continue
		}
		tableWriter, err := writer.BeginTable(result.spec)
		if err != nil {
			return nil, err
		}
		for _, row := range result.rows {
			if err := tableWriter.WriteRow(row); err != nil {
				return nil, err
			}
		}
		if err := tableWriter.Close(); err != nil {
			return nil, err
		}
		if table.Name == legacypkg.SalesTable {
			summary = salesSummary(result)
		}
	}
	return summary, nil
}

// salesSummary es el rango de fechas de las ventas.
func salesSummary(sales projected) *legacypkg.Summary {
	position := -1
	for index, column := range sales.spec.Columns {
		if column.Name == legacypkg.SalesDateColumn {
			position = index
		}
	}
	summary := &legacypkg.Summary{SalesTable: legacypkg.SalesTable}
	for _, row := range sales.rows {
		moment, ok := row[position].(string)
		if !ok {
			continue
		}
		if summary.SalesFirst == "" || moment < summary.SalesFirst {
			summary.SalesFirst = moment
		}
		if moment > summary.SalesLast {
			summary.SalesLast = moment
		}
	}
	return summary
}

// Build escribe en output un paquete sintético completo cifrado con la frase de paso.
func Build(output io.Writer, passphrase string, options legacypkg.WriterOptions) (legacypkg.Result, error) {
	writer, err := legacypkg.NewWriter(options)
	if err != nil {
		return legacypkg.Result{}, err
	}
	defer writer.Close()
	summary, err := WritePackage(writer, Dataset())
	if err != nil {
		return legacypkg.Result{}, err
	}
	created, _ := time.Parse(time.RFC3339, CreatedAt)
	return writer.Finish(output, passphrase, legacypkg.PackageInfo{
		CreatedAt: created,
		Creator:   legacypkg.Creator{Product: product.Read().Name, Version: product.Read().Version, Tool: "legacy-synthetic"},
		Source:    legacypkg.Source{Kind: legacypkg.SourceSynthetic, BackupFile: BackupFile, DatabaseName: DatabaseName},
		Summary:   summary,
	})
}
