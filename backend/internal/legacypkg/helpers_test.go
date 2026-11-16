package legacypkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

// Parámetros de Argon2 mínimos y bloques chicos: las pruebas van rápido y recorren muchos bloques.
var testKDF = KDFParams{Time: 1, MemoryKiB: 8 << 10, Threads: 1}

const (
	testPassphrase = "frase de paso de prueba"
	testChunkSize  = MinChunkSize
)

func testWriter(t *testing.T) *Writer {
	t.Helper()
	writer, err := NewWriter(WriterOptions{SpoolDir: t.TempDir(), KDF: testKDF, ChunkSize: testChunkSize})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = writer.Close()
	})
	return writer
}

func testInfo() PackageInfo {
	return PackageInfo{
		CreatedAt: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC),
		Creator:   Creator{Product: "AD-EL POS", Version: "prueba", Tool: "legacypkg_test"},
		Source:    Source{Kind: SourceSynthetic, DatabaseName: "sintetica"},
	}
}

// testTable describe una tabla de prueba con sus filas.
type testTable struct {
	spec TableSpec
	rows [][]any
}

// buildPackage escribe las tablas y devuelve los bytes del paquete.
func buildPackage(t *testing.T, tables []testTable, skipped []SkippedTable) ([]byte, Result) {
	t.Helper()
	writer := testWriter(t)
	for _, table := range tables {
		tableWriter, err := writer.BeginTable(table.spec)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range table.rows {
			if err := tableWriter.WriteRow(row); err != nil {
				t.Fatal(err)
			}
		}
		if err := tableWriter.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range skipped {
		if err := writer.SkipTable(table); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	result, err := writer.Finish(&output, testPassphrase, testInfo())
	if err != nil {
		t.Fatal(err)
	}
	if result.Bytes != int64(output.Len()) {
		t.Fatalf("tamaño informado %d y escrito %d", result.Bytes, output.Len())
	}
	return output.Bytes(), result
}

// readAll lee un paquete completo: tablas y filas en orden.
func readAll(source []byte, passphrase string, options OpenOptions) (Manifest, map[string][][]any, error) {
	reader, err := Open(bytes.NewReader(source), passphrase, options)
	if err != nil {
		return Manifest{}, nil, err
	}
	tables := map[string][][]any{}
	for {
		table, err := reader.NextTable()
		if err == io.EOF {
			return reader.Manifest(), tables, nil
		}
		if err != nil {
			return Manifest{}, nil, err
		}
		rows := [][]any{}
		for {
			row, err := table.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return Manifest{}, nil, err
			}
			rows = append(rows, row)
		}
		tables[table.Info().Name] = rows
	}
}

func requireKind(t *testing.T, err error, kind ErrorKind) {
	t.Helper()
	if err == nil {
		t.Fatalf("se esperaba un error %s", kind)
	}
	if KindOf(err) != kind {
		t.Fatalf("se esperaba %s y llegó %q: %v", kind, KindOf(err), err)
	}
	if !errors.Is(err, &Error{Kind: kind}) {
		t.Fatalf("errors.Is no reconoce el tipo %s", kind)
	}
}

// craftedPackage arma un paquete «a mano» con un tar propio (para probar entradas manipuladas que el escritor nunca
// produciría), cifrado como uno legítimo con testPassphrase.
func craftedPackage(t *testing.T, params headerParams, keys packageKeys, fill func(archive *tar.Writer)) []byte {
	t.Helper()
	header, err := encodeHeader(params)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	output.Write(header)
	headerHash := sha256.Sum256(header)
	encrypted, err := newChunkWriter(&output, keys.encryption, params.NoncePrefix, headerHash[:], params.ChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(encrypted)
	archive := tar.NewWriter(compressed)
	fill(archive)
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := encrypted.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func testParams(t *testing.T) (headerParams, packageKeys) {
	t.Helper()
	params, keys, err := newHeaderParams(testPassphrase, testKDF, testChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	return params, keys
}

func addEntry(t *testing.T, archive *tar.Writer, header *tar.Header, content []byte) {
	t.Helper()
	if header.Typeflag == 0 {
		header.Typeflag = tar.TypeReg
	}
	if header.Typeflag == tar.TypeReg {
		header.Size = int64(len(content))
	}
	header.Mode = 0600
	if err := archive.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if len(content) > 0 {
		if _, err := archive.Write(content); err != nil {
			t.Fatal(err)
		}
	}
}

// manifestFor arma un manifiesto válido para tablas con contenido dado (para los paquetes armados a mano).
func manifestFor(tables map[string][]byte, order []string, rows map[string]int64) Manifest {
	manifest := Manifest{
		Format:        FormatName,
		FormatVersion: FormatVersion,
		CreatedAt:     "2026-11-15T12:00:00Z",
		CreatedBy:     Creator{Product: "AD-EL POS", Version: "prueba", Tool: "craft"},
		Source:        Source{Kind: SourceSynthetic},
		Tables:        []TableManifest{},
		SkippedTables: []SkippedTable{},
	}
	for index, name := range order {
		digest := sha256.Sum256(tables[name])
		manifest.Tables = append(manifest.Tables, TableManifest{
			Name:           name,
			File:           tableFile(name, index),
			Rows:           rows[name],
			Bytes:          int64(len(tables[name])),
			SHA256:         fmt.Sprintf("%x", digest),
			PrimaryKey:     []string{},
			Columns:        []Column{{Name: "valor", Type: "varchar(max)", Nullable: true}},
			OmittedColumns: []OmittedColumn{},
		})
	}
	manifest.ContentFingerprint = ContentFingerprint(manifest.Tables)
	return manifest
}

func encodeJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
