package legacysynth

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"local/ad-el-pos/server/internal/legacypkg"
)

// insertBatch es el número de filas por INSERT (SQL Server admite hasta 1000 en un VALUES).
const insertBatch = 200

// TSQL es el script que crea la base sintética en un SQL Server (para respaldarla a un .bak sintético en la prueba con
// Docker). La base queda con el nivel de compatibilidad 110 (SQL Server 2012) y la intercalación del sistema anterior.
// Los lotes se separan con GO, como los ejecuta sqlcmd.
func TSQL(tables []*Table) string {
	var script strings.Builder
	script.WriteString("SET NOCOUNT ON;\n")
	fmt.Fprintf(&script, "CREATE DATABASE [%s] COLLATE SQL_Latin1_General_CP1_CI_AS;\nGO\n", DatabaseName)
	fmt.Fprintf(&script, "ALTER DATABASE [%s] SET COMPATIBILITY_LEVEL = 110;\nGO\n", DatabaseName)
	fmt.Fprintf(&script, "USE [%s];\nGO\n", DatabaseName)
	for _, table := range tables {
		writeCreateTable(&script, table)
	}
	for _, table := range tables {
		writeInserts(&script, table)
	}
	return script.String()
}

func writeCreateTable(script *strings.Builder, table *Table) {
	definitions := []string{}
	for _, column := range table.Columns {
		definition := quoteName(column.Name) + " " + column.Type
		if column.Identity {
			definition += " IDENTITY(1,1)"
		}
		if column.Nullable {
			definition += " NULL"
		} else {
			definition += " NOT NULL"
		}
		definitions = append(definitions, definition)
	}
	if len(table.PrimaryKey) > 0 {
		keys := []string{}
		for _, key := range table.PrimaryKey {
			keys = append(keys, quoteName(key))
		}
		definitions = append(definitions, "CONSTRAINT "+quoteName("PK_"+table.Name)+" PRIMARY KEY CLUSTERED ("+strings.Join(keys, ", ")+")")
	}
	fmt.Fprintf(script, "CREATE TABLE [dbo].%s (\n  %s\n);\nGO\n", quoteName(table.Name), strings.Join(definitions, ",\n  "))
}

func writeInserts(script *strings.Builder, table *Table) {
	if len(table.Rows) == 0 {
		return
	}
	hasIdentity := false
	names := []string{}
	for _, column := range table.Columns {
		names = append(names, quoteName(column.Name))
		if column.Identity {
			hasIdentity = true
		}
	}
	if hasIdentity {
		fmt.Fprintf(script, "SET IDENTITY_INSERT [dbo].%s ON;\n", quoteName(table.Name))
	}
	for start := 0; start < len(table.Rows); start += insertBatch {
		end := min(start+insertBatch, len(table.Rows))
		values := []string{}
		for _, row := range table.Rows[start:end] {
			literals := []string{}
			for index, value := range row {
				literals = append(literals, literal(table.Columns[index], value))
			}
			values = append(values, "("+strings.Join(literals, ", ")+")")
		}
		fmt.Fprintf(script, "INSERT INTO [dbo].%s (%s) VALUES\n%s;\n", quoteName(table.Name), strings.Join(names, ", "),
			strings.Join(values, ",\n"))
	}
	if hasIdentity {
		fmt.Fprintf(script, "SET IDENTITY_INSERT [dbo].%s OFF;\n", quoteName(table.Name))
	}
	script.WriteString("GO\n")
}

func quoteName(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

// literal es el valor como literal de T-SQL según el tipo de la columna.
func literal(column Column, value any) string {
	if value == nil {
		return "NULL"
	}
	switch typed := value.(type) {
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		if typed {
			return "1"
		}
		return "0"
	case []byte:
		return "0x" + hex.EncodeToString(typed)
	case string:
		switch legacypkg.BaseType(column.Type) {
		case "money", "smallmoney", "decimal", "numeric":
			return typed
		case "datetime", "smalldatetime", "date", "datetime2":
			return "'" + typed + "'"
		}
		return textLiteral(typed)
	}
	panic(fmt.Sprintf("legacysynth: valor %T en la columna %s", value, column.Name))
}

// textLiteral escribe un texto Unicode (N'…') con las comillas duplicadas y los caracteres de control como NCHAR(n),
// para que el script no dependa de cómo sqlcmd trate esos bytes.
func textLiteral(text string) string {
	if text == "" {
		return "N''"
	}
	parts := []string{}
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			parts = append(parts, "N'"+strings.ReplaceAll(current.String(), "'", "''")+"'")
			current.Reset()
		}
	}
	for _, character := range text {
		if character < 0x20 || character == 0x7f {
			flush()
			parts = append(parts, fmt.Sprintf("NCHAR(%d)", character))
			continue
		}
		current.WriteRune(character)
	}
	flush()
	return strings.Join(parts, " + ")
}
