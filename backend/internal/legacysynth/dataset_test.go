package legacysynth

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"golang.org/x/text/encoding/charmap"

	"local/ad-el-pos/server/internal/legacypkg"
)

var testKDF = legacypkg.KDFParams{Time: 1, MemoryKiB: 8 << 10, Threads: 1}

// Las 52 tablas del respaldo del sistema anterior (MIGRACION_ANALISIS.md §1.3).
var legacyTables = []string{
	"ajustes_inventario", "articulos", "articulos_imagenes", "articulos_tipos", "cajas", "cancelaciones",
	"categorias", "categorias_impresora", "ciudades", "colonias", "complementos", "complementos_precios", "cortes",
	"cortes_parciales", "descuentos", "divisas", "entradas", "entradas_detalles", "estados", "impresoras",
	"impresoras_poscommands", "info", "inventario", "mesas", "pagos", "pagos_productos", "paises", "personal",
	"personal_detalles", "productos", "productos_complementos", "productos_imagenes", "productos_precios",
	"promociones", "proveedores", "proveedores_detalles", "puestos", "recetas", "salidas", "servicios", "sysdiagrams",
	"tipos_descuentos", "tipos_pagos", "topes_caja", "unidades", "usuarios", "ventas", "ventas_complementos",
	"ventas_especial", "ventas_modificaciones", "ventas_productos", "ventas_servicios",
}

var (
	lengthPattern   = regexp.MustCompile(`^(var)?char\((\d+)\)$`)
	decimalsPattern = regexp.MustCompile(`^numeric\(\d+,(\d+)\)$`)
	datetimePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{2}[037]$`)
	datePattern     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

func byName(tables []*Table) map[string]*Table {
	result := map[string]*Table{}
	for _, table := range tables {
		result[table.Name] = table
	}
	return result
}

func TestSchemaHasTheLegacyTables(t *testing.T) {
	tables := Dataset()
	if len(tables) != len(legacyTables) {
		t.Fatalf("%d tablas", len(tables))
	}
	names := byName(tables)
	for _, name := range legacyTables {
		if names[name] == nil {
			t.Errorf("falta la tabla %s", name)
		}
	}
}

// Cada valor cabe en su columna como lo guardaría SQL Server: textos de la página 1252 dentro de su longitud,
// importes con sus decimales exactos, fechas con milisegundos que datetime no redondea y nulos solo donde se admiten.
func TestDatasetValuesFitTheirColumns(t *testing.T) {
	for _, table := range Dataset() {
		for rowIndex, row := range table.Rows {
			for index, value := range row {
				column := table.Columns[index]
				if err := checkValue(column, value); err != "" {
					t.Errorf("%s fila %d columna %s: %s", table.Name, rowIndex, column.Name, err)
				}
			}
		}
	}
}

func checkValue(column Column, value any) string {
	if value == nil {
		if !column.Nullable {
			return "nulo en una columna NOT NULL"
		}
		return ""
	}
	base := legacypkg.BaseType(column.Type)
	switch base {
	case "int", "smallint", "tinyint", "bigint":
		number, ok := value.(int64)
		if !ok || (base == "tinyint" && (number < 0 || number > 255)) || (base == "smallint" && (number < -32768 || number > 32767)) {
			return "entero fuera de su tipo"
		}
	case "money":
		if text, ok := value.(string); !ok || !strings.Contains(text, ".") || len(text)-strings.Index(text, ".") != 5 {
			return "importe sin 4 decimales"
		}
	case "numeric":
		scale, _ := strconv.Atoi(decimalsPattern.FindStringSubmatch(column.Type)[1])
		if text, ok := value.(string); !ok || len(text)-strings.Index(text, ".") != scale+1 {
			return "cantidad sin sus decimales"
		}
	case "datetime":
		if text, ok := value.(string); !ok || !datetimePattern.MatchString(text) {
			return "fecha y hora inválida"
		}
	case "date":
		if text, ok := value.(string); !ok || !datePattern.MatchString(text) {
			return "fecha inválida"
		}
	case "varbinary":
		if _, ok := value.([]byte); !ok {
			return "binario"
		}
	case "char", "varchar":
		text, ok := value.(string)
		if !ok {
			return "texto"
		}
		encoded, err := charmap.Windows1252.NewEncoder().Bytes([]byte(text))
		if err != nil {
			return "texto fuera de la página de códigos 1252"
		}
		limit, _ := strconv.Atoi(lengthPattern.FindStringSubmatch(column.Type)[2])
		if len(encoded) > limit {
			return "texto más largo que la columna"
		}
	}
	return ""
}

// Los casos que la Fase C necesita importar y validar están en los datos.
func TestDatasetCoversTheLegacyCases(t *testing.T) {
	tables := byName(Dataset())
	if len(tables["ventas"].Rows) != salesCount || len(tables["cancelaciones"].Rows) == 0 || len(tables["descuentos"].Rows) == 0 {
		t.Fatal("ventas, cancelaciones o descuentos")
	}
	negativeComplements := 0
	for _, row := range tables["ventas_complementos"].Rows {
		if strings.HasPrefix(row[6].(string), "-") {
			negativeComplements++
		}
	}
	taxedLines := 0
	for _, row := range tables["ventas_productos"].Rows {
		if row[6].(string) != "0.0000" {
			taxedLines++
		}
	}
	zeroOrNegativePayments := 0
	cardPayments := 0
	for _, row := range tables["pagos"].Rows {
		amount := decimal.RequireFromString(row[4].(string))
		if !amount.IsPositive() {
			zeroOrNegativePayments++
		}
		if row[3].(int64) == 2 {
			cardPayments++
		}
	}
	if negativeComplements == 0 || taxedLines == 0 || zeroOrNegativePayments < 2 || cardPayments == 0 {
		t.Fatalf("complementos negativos %d, renglones con IVA %d, pagos en cero o negativos %d, tarjeta %d",
			negativeComplements, taxedLines, zeroOrNegativePayments, cardPayments)
	}
	if len(tables["cortes"].Rows) != 8 || len(tables["cortes_parciales"].Rows) == 0 || len(tables["salidas"].Rows) == 0 {
		t.Fatal("cortes, retiros o salidas")
	}
	if len(tables["divisas"].Rows) != 2 || len(tables["usuarios"].Rows) != len(SyntheticPasswords) {
		t.Fatal("divisas o usuarios")
	}
}

// El paquete sintético se abre, trae las 50 tablas extraídas y las 2 omitidas, y no contiene ninguna contraseña ni
// datos personales de los empleados.
func TestSyntheticPackage(t *testing.T) {
	var output bytes.Buffer
	result, err := Build(&output, "frase de paso sintética", legacypkg.WriterOptions{SpoolDir: t.TempDir(), KDF: testKDF})
	if err != nil {
		t.Fatal(err)
	}
	manifest := result.Manifest
	if len(manifest.Tables) != 50 || len(manifest.SkippedTables) != 2 {
		t.Fatalf("%d tablas y %d omitidas", len(manifest.Tables), len(manifest.SkippedTables))
	}
	if manifest.Summary == nil || manifest.Summary.SalesFirst == "" || manifest.Summary.SalesFirst >= manifest.Summary.SalesLast {
		t.Fatalf("resumen: %+v", manifest.Summary)
	}
	users, _ := manifest.Table("usuarios")
	if users.Columns[2].Name != "password_present" || users.Columns[2].Replaces != "password" {
		t.Fatalf("columnas de usuarios: %+v", users.Columns)
	}
	products, _ := manifest.Table("productos")
	if len(products.OmittedColumns) != 1 || products.OmittedColumns[0].Name != "portada" || products.OmittedColumns[0].RowsWithValue != 3 {
		t.Fatalf("columnas omitidas de productos: %+v", products.OmittedColumns)
	}
	reader, err := legacypkg.Open(bytes.NewReader(output.Bytes()), "frase de paso sintética", legacypkg.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var plain bytes.Buffer
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
			plain.Write(encoded)
			if table.Info().Name == "usuarios" {
				user := row[1].(string)
				for _, password := range SyntheticPasswords {
					if password.User == user && row[2].(bool) != (password.Password != "") {
						t.Fatalf("password_present de %s", user)
					}
				}
			}
		}
	}
	for _, password := range SyntheticPasswords {
		if password.Password != "" && bytes.Contains(plain.Bytes(), []byte(password.Password)) {
			t.Fatal("una contraseña llegó al paquete")
		}
	}
	for _, personal := range []string{"uno@ejemplo.invalid", "Calle Falsa", "0000000001"} {
		if bytes.Contains(plain.Bytes(), []byte(personal)) {
			t.Fatalf("un dato personal llegó al paquete: %s", personal)
		}
	}
}

// El orden de las tablas sin llave primaria es el de SQL Server: espacios del final ignorados y DATALENGTH al empatar,
// bytes de la página 1252 (BIN2) y nulos primero.
func TestOrderingFollowsSQLServer(t *testing.T) {
	cases := []struct {
		sqlType     string
		left, right any
		expected    int
	}{
		{"varchar(10)", "a", "a ", -1},
		{"varchar(10)", "B", "a", -1},
		{"varchar(10)", "z", "é", -1},
		{"varchar(10)", "€", "é", -1},
		{"varchar(10)", nil, "", -1},
		{"money", json.Number("-1.0000"), json.Number("0.5000"), -1},
		{"money", json.Number("10.0000"), json.Number("9.0000"), 1},
		{"datetime", "2019-10-01T00:00:00.003", "2019-10-01T00:00:00.007", -1},
		{"bit", false, true, -1},
		{"nvarchar(10)", "a", "a", 0},
		{"varchar(10)", "a\x01", "a", -1},
	}
	for _, testCase := range cases {
		if result := compareValues(testCase.sqlType, testCase.left, testCase.right); result != testCase.expected {
			t.Errorf("%s %v %v: %d", testCase.sqlType, testCase.left, testCase.right, result)
		}
	}
	// El orden completo que da SQL Server 2022 con Latin1_General_BIN2 y DATALENGTH (comprobado por el revisor en un
	// SQL Server propio): un carácter menor que el espacio después de un prefijo ordena antes que el prefijo, porque
	// SQL Server rellena el más corto con espacios.
	expected := []any{nil, "", "A", "a\x01", "a\t", "a\x1f", "a", "a ", "a!"}
	for _, sqlType := range []string{"varchar(20)", "nvarchar(20)"} {
		values := []any{"a", "a\x01", "a\t", "a ", "a!", "A", "", nil, "a\x1f"}
		sort.SliceStable(values, func(left, right int) bool {
			return compareValues(sqlType, values[left], values[right]) < 0
		})
		if !reflect.DeepEqual(values, expected) {
			t.Errorf("%s: %q, SQL Server: %q", sqlType, values, expected)
		}
	}
}

func TestTSQLScript(t *testing.T) {
	script := TSQL(Dataset())
	for _, expected := range []string{
		"CREATE DATABASE [possys] COLLATE SQL_Latin1_General_CP1_CI_AS",
		"SET COMPATIBILITY_LEVEL = 110",
		"CREATE TABLE [dbo].[ventas] (",
		"SET IDENTITY_INSERT [dbo].[ventas] ON;",
		"N'línea 1' + NCHAR(10) + N'línea 2' + NCHAR(13) + NCHAR(10) + N'fin'",
		"N'Ñandú «comillas» \"dobles\" y ''simples'''",
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("el script no contiene %q", expected)
		}
	}
	if strings.Count(script, "CREATE TABLE") != 52 {
		t.Fatal("el script no crea las 52 tablas")
	}
}
