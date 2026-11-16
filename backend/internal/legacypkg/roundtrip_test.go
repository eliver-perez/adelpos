package legacypkg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

// Textos que deben sobrevivir intactos: acentos, comillas, saltos de línea, «|», caracteres de control, barra
// invertida, emoji, separadores de línea Unicode y lo que encoding/json escaparía para HTML.
var difficultTexts = []string{
	"Ñandú «comillas» \"dobles\" y 'simples'",
	"línea 1\nlínea 2\r\nlínea 3\ttabulada",
	"barra | vertical || doble",
	"control \x01\x02\x1f y DEL \x7f",
	`barra \ invertida \\ doble`,
	"emoji 😀 y € y ½",
	"separadores    ",
	"<script>&amp;</script>",
	"",
	"   espacios al final   ",
}

func salesSpec() TableSpec {
	return TableSpec{
		Name: "ventas",
		Columns: []Column{
			{Name: "id", Type: "int", Identity: true},
			{Name: "folio", Type: "varchar(12)"},
			{Name: "fecha", Type: "datetime"},
			{Name: "nombre", Type: "varchar(50)", Nullable: true},
			{Name: "total", Type: "money"},
			{Name: "cantidad", Type: "numeric(18,3)"},
			{Name: "pagado", Type: "tinyint", Nullable: true},
			{Name: "password_present", Type: "bit", Replaces: "password"},
		},
		PrimaryKey: []string{"id"},
		OmittedColumns: []OmittedColumn{
			{Name: "portada", Type: "varbinary(max)", Reason: ReasonBinary, RowsWithValue: 3},
		},
	}
}

func salesRows(count int) [][]any {
	amounts := []string{"150.0000", "-0.0001", "922337203685477.5807", "0.0000", "86.2069", "-214748.3648"}
	rows := [][]any{}
	for index := 0; index < count; index++ {
		var name any = difficultTexts[index%len(difficultTexts)]
		if index%7 == 3 {
			name = nil
		}
		var paid any = int64(index % 2)
		if index%11 == 5 {
			paid = nil
		}
		rows = append(rows, []any{
			json.Number(fmt.Sprint(index + 1)),
			fmt.Sprintf("%d", 1000+index),
			fmt.Sprintf("2019-10-%02dT12:34:56.%03d", 1+index%28, []int{0, 3, 7, 997, 123}[index%5]),
			name,
			json.Number(amounts[index%len(amounts)]),
			json.Number("1.500"),
			paid,
			index%3 == 0,
		})
	}
	return rows
}

// Ida y vuelta con miles de filas, textos difíciles e importes exactos: lo leído es idéntico a lo escrito y el
// manifiesto trae conteos, huellas y columnas.
func TestRoundTripKeepsEveryValueExactly(t *testing.T) {
	sales := salesRows(5000)
	empty := testTable{spec: TableSpec{Name: "salidas", Columns: []Column{{Name: "monto", Type: "money"}}}}
	noColumns := testTable{spec: TableSpec{Name: "solo_binarios"}, rows: [][]any{{}, {}}}
	oddName := testTable{
		spec: TableSpec{Name: "otro esquema.tabla con espacios", Columns: []Column{{Name: "valor.con.puntos", Type: "int"}}},
		rows: [][]any{{int64(7)}, {int32(-8)}, {9}},
	}
	tables := []testTable{{spec: salesSpec(), rows: sales}, empty, noColumns, oddName}
	skipped := []SkippedTable{{Name: "sysdiagrams", Rows: 1, Reason: ReasonTool}}
	encoded, result := buildPackage(t, tables, skipped)
	if !bytes.HasPrefix(encoded, []byte(Signature)) {
		t.Fatal("el paquete no empieza con la firma")
	}
	manifest, read, err := readAll(encoded, testPassphrase, OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manifest, result.Manifest) {
		t.Fatal("el manifiesto leído no es el escrito")
	}
	if manifest.ContentFingerprint != ContentFingerprint(manifest.Tables) || len(manifest.Tables) != 4 {
		t.Fatalf("manifiesto inesperado: %+v", manifest)
	}
	if manifest.Tables[0].Rows != 5000 || manifest.Tables[1].Rows != 0 || manifest.Tables[2].Rows != 2 {
		t.Fatalf("conteos: %+v", manifest.Tables)
	}
	if manifest.Tables[3].File != "tables/_t0003.json" || manifest.Tables[0].File != "tables/ventas.json" {
		t.Fatalf("archivos de las tablas: %s, %s", manifest.Tables[0].File, manifest.Tables[3].File)
	}
	if !reflect.DeepEqual(manifest.SkippedTables, skipped) || manifest.Tables[0].OmittedColumns[0].RowsWithValue != 3 {
		t.Fatal("tablas o columnas omitidas")
	}
	got := read["ventas"]
	if len(got) != len(sales) {
		t.Fatalf("filas leídas: %d", len(got))
	}
	for index, row := range sales {
		expected := normalizeWritten(row)
		if !reflect.DeepEqual(got[index], expected) {
			t.Fatalf("fila %d: %#v, se esperaba %#v", index, got[index], expected)
		}
	}
	if len(read["salidas"]) != 0 || len(read["solo_binarios"]) != 2 {
		t.Fatal("tablas vacías o sin columnas")
	}
	if !reflect.DeepEqual(read["otro esquema.tabla con espacios"], [][]any{{json.Number("7")}, {json.Number("-8")}, {json.Number("9")}}) {
		t.Fatalf("tabla con nombre raro: %#v", read["otro esquema.tabla con espacios"])
	}
	verified, err := Verify(bytes.NewReader(encoded), testPassphrase, OpenOptions{})
	if err != nil || verified.ContentFingerprint != manifest.ContentFingerprint {
		t.Fatalf("Verify: %v", err)
	}
	header, err := Inspect(bytes.NewReader(encoded), OpenOptions{})
	if err != nil || header.Version != FormatVersion || header.ChunkSize != testChunkSize || header.KDF != testKDF {
		t.Fatalf("Inspect: %+v %v", header, err)
	}
}

// normalizeWritten es lo que la lectura entrega de un valor escrito: los enteros vuelven como json.Number.
func normalizeWritten(row []any) []any {
	normalized := make([]any, len(row))
	for index, value := range row {
		switch typed := value.(type) {
		case int64:
			normalized[index] = json.Number(fmt.Sprint(typed))
		default:
			normalized[index] = typed
		}
	}
	return normalized
}

// Los mismos datos dan la misma huella de contenido aunque el paquete se cifre con otra frase de paso (otra sal y otro
// nonce): la huella identifica el contenido, no el archivo.
func TestContentFingerprintDependsOnlyOnTheData(t *testing.T) {
	tables := []testTable{{spec: salesSpec(), rows: salesRows(40)}}
	first, firstResult := buildPackage(t, tables, nil)
	second, secondResult := buildPackage(t, tables, nil)
	if bytes.Equal(first, second) {
		t.Fatal("dos paquetes idénticos byte a byte: la sal o el nonce no son aleatorios")
	}
	if firstResult.Manifest.ContentFingerprint != secondResult.Manifest.ContentFingerprint {
		t.Fatal("la huella cambió con los mismos datos")
	}
	changed := salesRows(40)
	changed[10][4] = json.Number("150.0001")
	_, changedResult := buildPackage(t, []testTable{{spec: salesSpec(), rows: changed}}, nil)
	if changedResult.Manifest.ContentFingerprint == firstResult.Manifest.ContentFingerprint {
		t.Fatal("la huella no cambió al cambiar un importe")
	}
}

// La frase de paso se normaliza (NFC): la misma frase con «ñ» compuesta o descompuesta abre el paquete.
func TestPassphraseIsNormalized(t *testing.T) {
	composed := norm.NFC.String("contraseña del año")
	decomposed := norm.NFD.String(composed)
	if composed == decomposed {
		t.Fatal("la prueba necesita dos formas distintas")
	}
	writer := testWriter(t)
	var output bytes.Buffer
	if _, err := writer.Finish(&output, composed, testInfo()); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(bytes.NewReader(output.Bytes()), decomposed, OpenOptions{}); err != nil {
		t.Fatalf("la forma descompuesta no abrió el paquete: %v", err)
	}
}

func TestWrongPassphraseIsDistinguishedFromDamage(t *testing.T) {
	encoded, _ := buildPackage(t, []testTable{{spec: salesSpec(), rows: salesRows(3)}}, nil)
	_, err := Open(bytes.NewReader(encoded), "otra frase de paso", OpenOptions{})
	requireKind(t, err, KindWrongPassphrase)
	if strings.Contains(err.Error(), "otra frase") {
		t.Fatal("el mensaje repite la frase de paso")
	}
}

// Las tablas en preparación nunca quedan en claro en el disco y se borran al cerrar.
func TestSpoolIsEncryptedPrivateAndRemoved(t *testing.T) {
	directory := t.TempDir()
	writer, err := NewWriter(WriterOptions{SpoolDir: directory, KDF: testKDF})
	if err != nil {
		t.Fatal(err)
	}
	table, err := writer.BeginTable(TableSpec{Name: "usuarios", Columns: []Column{{Name: "usuario", Type: "varchar(30)"}}})
	if err != nil {
		t.Fatal(err)
	}
	marker := "MARCADOR-QUE-NO-DEBE-VERSE-EN-CLARO"
	for index := 0; index < 2000; index++ {
		if err := table.WriteRow([]any{marker}); err != nil {
			t.Fatal(err)
		}
	}
	if err := table.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("carpeta temporal: %v %v", entries, err)
	}
	spool := filepath.Join(directory, entries[0].Name())
	info, err := os.Stat(spool)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 && !isWindows() {
		t.Fatalf("carpeta temporal con permisos %v", info.Mode().Perm())
	}
	err = filepath.WalkDir(spool, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(content, []byte(marker)) || bytes.Contains(content, []byte("usuario")) {
			t.Fatalf("texto claro en %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(spool); !os.IsNotExist(err) {
		t.Fatal("la carpeta temporal no se borró")
	}
}

func TestWriterRejectsInvalidInput(t *testing.T) {
	writer := testWriter(t)
	spec := TableSpec{Name: "pagos", Columns: []Column{{Name: "monto", Type: "money"}, {Name: "folio", Type: "varchar(20)", Nullable: true}}}
	table, err := writer.BeginTable(spec)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]any{
		"float64":           {150.5, nil},
		"número inválido":   {json.Number("1e"), nil},
		"nulo no permitido": {nil, nil},
		"faltan valores":    {json.Number("1")},
		"UTF-8 inválido":    {json.Number("1"), "\xff"},
	}
	for name, row := range cases {
		probe := testWriter(t)
		probeTable, err := probe.BeginTable(spec)
		if err != nil {
			t.Fatal(err)
		}
		if err := probeTable.WriteRow(row); err == nil {
			t.Fatalf("%s: se aceptó", name)
		}
	}
	if err := table.WriteRow([]any{json.Number("1.0000"), nil}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.BeginTable(TableSpec{Name: "otra"}); err == nil {
		t.Fatal("dos tablas abiertas a la vez")
	}
	if err := table.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.BeginTable(spec); err == nil {
		t.Fatal("tabla repetida")
	}
	if err := writer.SkipTable(SkippedTable{Name: "pagos", Rows: 1, Reason: ReasonTool}); err == nil {
		t.Fatal("tabla omitida con el nombre de una extraída")
	}
	var output bytes.Buffer
	_, err = writer.Finish(&output, "corta", testInfo())
	requireKind(t, err, KindInvalid)
	if output.Len() != 0 {
		t.Fatal("se escribió un paquete con una frase de paso corta")
	}
}

func TestWriterRejectsRowsTheReaderWouldRefuse(t *testing.T) {
	writer := testWriter(t)
	table, err := writer.BeginTable(TableSpec{Name: "notas", Columns: []Column{{Name: "texto", Type: "varchar(max)"}}})
	if err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("x", int(DefaultLimits().MaxRowBytes))
	requireKind(t, table.WriteRow([]any{huge}), KindLimit)
	var output bytes.Buffer
	if _, err := writer.Finish(&output, testPassphrase, testInfo()); err == nil {
		t.Fatal("se terminó un paquete con una tabla fallida")
	}
}

// Una tabla que no se lee completa se comprueba igual al pasar a la siguiente.
func TestNextTableChecksSkippedRows(t *testing.T) {
	encoded, _ := buildPackage(t, []testTable{
		{spec: salesSpec(), rows: salesRows(30)},
		{spec: TableSpec{Name: "cortes", Columns: []Column{{Name: "id", Type: "int"}}}, rows: [][]any{{1}, {2}}},
	}, nil)
	reader, err := Open(bytes.NewReader(encoded), testPassphrase, OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := reader.NextTable()
	if err != nil || first.Info().Name != "ventas" || first.ColumnIndex("total") != 4 || first.ColumnIndex("no") != -1 {
		t.Fatalf("primera tabla: %v", err)
	}
	if _, err := first.Next(); err != nil {
		t.Fatal(err)
	}
	second, err := reader.NextTable()
	if err != nil || second.Info().Name != "cortes" {
		t.Fatalf("segunda tabla: %v", err)
	}
	if _, err := first.Next(); err != io.EOF {
		t.Fatalf("la primera tabla siguió entregando filas: %v", err)
	}
	if _, err := reader.NextTable(); err != io.EOF {
		t.Fatalf("final: %v", err)
	}
	if _, err := reader.NextTable(); err != io.EOF {
		t.Fatal("NextTable después del final")
	}
}

func isWindows() bool {
	return os.PathSeparator == '\\'
}
