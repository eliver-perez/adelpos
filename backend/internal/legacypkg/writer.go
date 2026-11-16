package legacypkg

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"time"

	"local/ad-el-pos/server/internal/fileacl"
)

// spoolChunkSize es el bloque del cifrado de las tablas en preparación.
const spoolChunkSize = 256 << 10

// WriterOptions configura la escritura de un paquete.
type WriterOptions struct {
	// SpoolDir es la carpeta donde se crea la carpeta privada de las tablas en preparación ("" = la temporal del
	// usuario). Las tablas se guardan ahí cifradas con una llave aleatoria que solo vive en memoria.
	SpoolDir string
	// KDF son los parámetros de Argon2id (cero = DefaultKDF). Las pruebas usan valores bajos.
	KDF KDFParams
	// ChunkSize es el tamaño de bloque del cifrado (cero = DefaultChunkSize).
	ChunkSize int
}

// TableSpec describe una tabla que se va a escribir: sus columnas en el orden de las filas, su llave primaria y las
// columnas que no viajan.
type TableSpec struct {
	Name           string
	Columns        []Column
	PrimaryKey     []string
	OmittedColumns []OmittedColumn
}

// PackageInfo son los datos del manifiesto que no salen de las tablas.
type PackageInfo struct {
	CreatedAt time.Time
	Creator   Creator
	Source    Source
	Summary   *Summary
}

// Result es lo que se escribió.
type Result struct {
	Manifest Manifest
	// Bytes es el tamaño del paquete escrito.
	Bytes int64
}

// Writer escribe un paquete en flujo. Como manifest.json va primero en el tar pero sus huellas y conteos solo se
// conocen al final, cada tabla se prepara primero en un archivo temporal cifrado (BeginTable, WriteRow, Close) y Finish
// arma el paquete: encabezado, manifiesto y tablas. Close borra siempre lo temporal.
type Writer struct {
	options  WriterOptions
	spoolDir string
	spoolKey []byte
	tables   []*spooledTable
	skipped  []SkippedTable
	names    map[string]bool
	current  *TableWriter
	finished bool
	closed   bool
}

type spooledTable struct {
	manifest TableManifest
	path     string
	prefix   []byte
	aad      []byte
}

// NewWriter prepara un paquete nuevo.
func NewWriter(options WriterOptions) (*Writer, error) {
	if options.KDF == (KDFParams{}) {
		options.KDF = DefaultKDF()
	}
	if options.ChunkSize == 0 {
		options.ChunkSize = DefaultChunkSize
	}
	limits := DefaultLimits()
	if options.ChunkSize < MinChunkSize || options.ChunkSize > limits.MaxChunkSize {
		return nil, invalid("Tamaño de bloque fuera de los límites.")
	}
	if options.KDF.Time < minArgonTime || options.KDF.MemoryKiB < minArgonMemoryKiB || options.KDF.Threads == 0 ||
		options.KDF.Time > limits.MaxArgonTime || options.KDF.MemoryKiB > limits.MaxArgonMemoryKiB ||
		options.KDF.Threads > limits.MaxArgonThreads {
		return nil, invalid("Parámetros de Argon2 fuera de los límites.")
	}
	base := options.SpoolDir
	if base == "" {
		base = os.TempDir()
	}
	// MkdirTemp crea la carpeta con 0700; en Windows, además, la ACL privada.
	directory, err := os.MkdirTemp(base, "adel-migracion-")
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear la carpeta temporal del paquete: %w", err)
	}
	if err := fileacl.Protect(directory); err != nil {
		_ = os.RemoveAll(directory)
		return nil, err
	}
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		_ = os.RemoveAll(directory)
		return nil, err
	}
	return &Writer{
		options:  options,
		spoolDir: directory,
		spoolKey: key,
		names:    map[string]bool{},
	}, nil
}

// BeginTable empieza una tabla; solo puede haber una abierta a la vez.
func (writer *Writer) BeginTable(spec TableSpec) (*TableWriter, error) {
	if writer.closed || writer.finished {
		return nil, errors.New("el paquete ya se terminó")
	}
	if writer.current != nil {
		return nil, errors.New("hay otra tabla abierta")
	}
	if err := writer.claimName(spec.Name); err != nil {
		return nil, err
	}
	index := len(writer.tables)
	table := &spooledTable{
		manifest: TableManifest{
			Name:           spec.Name,
			File:           tableFile(spec.Name, index),
			PrimaryKey:     copied(spec.PrimaryKey),
			Columns:        copied(spec.Columns),
			OmittedColumns: copied(spec.OmittedColumns),
		},
		path:   filepath.Join(writer.spoolDir, fmt.Sprintf("tabla-%04d.bin", index)),
		prefix: make([]byte, noncePrefixSize),
		aad:    []byte(fmt.Sprintf("adelmig spool %d", index)),
	}
	for _, existing := range writer.tables {
		if existing.manifest.File == table.manifest.File {
			return nil, fmt.Errorf("dos tablas usarían el mismo archivo %s", table.manifest.File)
		}
	}
	if err := validateTableManifest(withPlaceholderHash(table.manifest), DefaultLimits()); err != nil {
		return nil, err
	}
	if _, err := rand.Read(table.prefix); err != nil {
		return nil, err
	}
	tableWriter, err := newTableWriter(writer, table)
	if err != nil {
		return nil, err
	}
	writer.tables = append(writer.tables, table)
	writer.current = tableWriter
	return tableWriter, nil
}

// SkipTable registra una tabla que no se extrajo, con su conteo y el motivo.
func (writer *Writer) SkipTable(skipped SkippedTable) error {
	if writer.closed || writer.finished {
		return errors.New("el paquete ya se terminó")
	}
	if skipped.Rows < 0 || skipped.Reason == "" {
		return errors.New("tabla omitida sin conteo o sin motivo")
	}
	if err := writer.claimName(skipped.Name); err != nil {
		return err
	}
	writer.skipped = append(writer.skipped, skipped)
	return nil
}

func (writer *Writer) claimName(name string) error {
	if !validName(name) {
		return fmt.Errorf("nombre de tabla inválido")
	}
	if writer.names[name] {
		return fmt.Errorf("la tabla %s ya está en el paquete", name)
	}
	writer.names[name] = true
	return nil
}

// Finish escribe el paquete completo en output: encabezado, flujo cifrado con el gzip del tar (manifiesto primero y
// después las tablas en el orden en que se escribieron). No cierra output.
func (writer *Writer) Finish(output io.Writer, passphrase string, info PackageInfo) (Result, error) {
	if writer.closed || writer.finished {
		return Result{}, errors.New("el paquete ya se terminó")
	}
	if writer.current != nil {
		return Result{}, errors.New("hay una tabla abierta")
	}
	if err := ValidatePassphrase(passphrase); err != nil {
		return Result{}, err
	}
	writer.finished = true
	manifest, encodedManifest, err := writer.buildManifest(info)
	if err != nil {
		return Result{}, err
	}
	params, keys, err := newHeaderParams(passphrase, writer.options.KDF, writer.options.ChunkSize)
	if err != nil {
		return Result{}, err
	}
	header, err := encodeHeader(params)
	if err != nil {
		return Result{}, err
	}
	counter := &countingWriter{target: output}
	if _, err := counter.Write(header); err != nil {
		return Result{}, err
	}
	headerHash := sha256.Sum256(header)
	encrypted, err := newChunkWriter(counter, keys.encryption, params.NoncePrefix, headerHash[:], params.ChunkSize)
	if err != nil {
		return Result{}, err
	}
	compressed := gzip.NewWriter(encrypted)
	archive := tar.NewWriter(compressed)
	modified := info.CreatedAt.UTC().Truncate(time.Second)
	if err := writeEntry(archive, ManifestFile, int64(len(encodedManifest)), modified); err != nil {
		return Result{}, err
	}
	if _, err := archive.Write(encodedManifest); err != nil {
		return Result{}, err
	}
	for _, table := range writer.tables {
		if err := writer.copyTable(archive, table, modified); err != nil {
			return Result{}, err
		}
	}
	if err := archive.Close(); err != nil {
		return Result{}, err
	}
	if err := compressed.Close(); err != nil {
		return Result{}, err
	}
	if err := encrypted.Close(); err != nil {
		return Result{}, err
	}
	return Result{Manifest: manifest, Bytes: counter.count}, nil
}

// buildManifest arma y valida el manifiesto con los mismos límites que la lectura: nunca se escribe un paquete que
// AD-EL POS no pueda abrir.
func (writer *Writer) buildManifest(info PackageInfo) (Manifest, []byte, error) {
	manifest := Manifest{
		Format:        FormatName,
		FormatVersion: FormatVersion,
		CreatedAt:     info.CreatedAt.UTC().Format(time.RFC3339),
		CreatedBy:     info.Creator,
		Source:        info.Source,
		Summary:       info.Summary,
		Tables:        []TableManifest{},
		SkippedTables: copied(writer.skipped),
	}
	var total int64
	for _, table := range writer.tables {
		manifest.Tables = append(manifest.Tables, table.manifest)
		total += table.manifest.Bytes
	}
	manifest.ContentFingerprint = ContentFingerprint(manifest.Tables)
	limits := DefaultLimits()
	if err := validateManifest(&manifest, FormatVersion, limits); err != nil {
		return Manifest{}, nil, err
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, nil, err
	}
	if int64(len(encoded)) > limits.MaxManifestBytes || total+int64(len(encoded)) > limits.MaxUncompressedBytes {
		return Manifest{}, nil, limitExceeded("tamaño del contenido")
	}
	return manifest, encoded, nil
}

// copyTable pasa una tabla preparada al tar, descifrándola en flujo.
func (writer *Writer) copyTable(archive *tar.Writer, table *spooledTable, modified time.Time) error {
	file, err := os.Open(table.path)
	if err != nil {
		return err
	}
	defer file.Close()
	decrypted, err := newChunkReader(file, writer.spoolKey, table.prefix, table.aad, spoolChunkSize)
	if err != nil {
		return err
	}
	decompressed, err := gzip.NewReader(decrypted)
	if err != nil {
		return fmt.Errorf("tabla temporal ilegible: %w", err)
	}
	if err := writeEntry(archive, table.manifest.File, table.manifest.Bytes, modified); err != nil {
		return err
	}
	if _, err := io.CopyN(archive, decompressed, table.manifest.Bytes); err != nil {
		return fmt.Errorf("tabla temporal incompleta: %w", err)
	}
	var extra [1]byte
	if count, _ := decompressed.Read(extra[:]); count != 0 {
		return errors.New("tabla temporal con datos de más")
	}
	return nil
}

func writeEntry(archive *tar.Writer, name string, size int64, modified time.Time) error {
	return archive.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Size:     size,
		Mode:     0600,
		ModTime:  modified,
	})
}

// Close borra las tablas en preparación. Se llama siempre (también si algo falló).
func (writer *Writer) Close() error {
	if writer.closed {
		return nil
	}
	writer.closed = true
	if writer.current != nil {
		writer.current.abort()
	}
	return os.RemoveAll(writer.spoolDir)
}

// TableWriter escribe las filas de una tabla en preparación.
type TableWriter struct {
	writer     *Writer
	table      *spooledTable
	file       *os.File
	encrypted  *chunkWriter
	compressed *gzip.Writer
	hash       hash.Hash
	keys       [][]byte
	buffer     []byte
	rows       int64
	bytes      int64
	maxRow     int64
	done       bool
	err        error
}

func newTableWriter(writer *Writer, table *spooledTable) (*TableWriter, error) {
	file, err := fileacl.CreatePrivate(table.path)
	if err != nil {
		return nil, err
	}
	encrypted, err := newChunkWriter(file, writer.spoolKey, table.prefix, table.aad, spoolChunkSize)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	compressed, err := gzip.NewWriterLevel(encrypted, gzip.BestSpeed)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	tableWriter := &TableWriter{
		writer:     writer,
		table:      table,
		file:       file,
		encrypted:  encrypted,
		compressed: compressed,
		hash:       sha256.New(),
		maxRow:     DefaultLimits().MaxRowBytes,
	}
	// Prefijos de cada columna ya codificados: «{"nombre":» para la primera y «,"nombre":» para las demás.
	for index, column := range table.manifest.Columns {
		prefix := []byte{','}
		if index == 0 {
			prefix = []byte{'{'}
		}
		prefix = appendString(prefix, column.Name)
		prefix = append(prefix, ':')
		tableWriter.keys = append(tableWriter.keys, prefix)
	}
	if err := tableWriter.emit([]byte{'['}); err != nil {
		tableWriter.abort()
		return nil, err
	}
	return tableWriter, nil
}

// WriteRow escribe una fila con un valor por columna, en el orden de TableSpec.Columns.
func (tableWriter *TableWriter) WriteRow(values []any) error {
	if tableWriter.err != nil {
		return tableWriter.err
	}
	if tableWriter.done {
		return errors.New("la tabla ya se cerró")
	}
	columns := tableWriter.table.manifest.Columns
	if len(values) != len(columns) {
		return tableWriter.fail(fmt.Errorf("la fila tiene %d valores y la tabla %d columnas", len(values), len(columns)))
	}
	row := tableWriter.buffer[:0]
	if tableWriter.rows > 0 {
		row = append(row, ',', '\n')
	}
	if len(columns) == 0 {
		row = append(row, '{')
	}
	for index, value := range values {
		if value == nil && !columns[index].Nullable {
			return tableWriter.fail(fmt.Errorf("valor nulo en la columna %s, que no admite nulos", columns[index].Name))
		}
		row = append(row, tableWriter.keys[index]...)
		var err error
		row, err = appendValue(row, value)
		if err != nil {
			return tableWriter.fail(fmt.Errorf("columna %s: %w", columns[index].Name, err))
		}
	}
	row = append(row, '}')
	if int64(len(row)) > tableWriter.maxRow {
		return tableWriter.fail(limitExceeded("tamaño de una fila"))
	}
	tableWriter.buffer = row
	if err := tableWriter.emit(row); err != nil {
		return err
	}
	tableWriter.rows++
	return nil
}

func (tableWriter *TableWriter) emit(data []byte) error {
	tableWriter.hash.Write(data)
	written, err := tableWriter.compressed.Write(data)
	tableWriter.bytes += int64(written)
	if err != nil {
		return tableWriter.fail(err)
	}
	return nil
}

func (tableWriter *TableWriter) fail(err error) error {
	if tableWriter.err == nil {
		tableWriter.err = err
	}
	return tableWriter.err
}

// SetOmittedRows fija cuántas filas tenían valor en una columna omitida (el lector lo sabe al empezar a leer la tabla).
func (tableWriter *TableWriter) SetOmittedRows(name string, rows int64) error {
	if tableWriter.done {
		return errors.New("la tabla ya se cerró")
	}
	if rows < 0 {
		return errors.New("conteo negativo")
	}
	omitted := tableWriter.table.manifest.OmittedColumns
	for index := range omitted {
		if omitted[index].Name == name {
			omitted[index].RowsWithValue = rows
			return nil
		}
	}
	return fmt.Errorf("la columna %s no está entre las omitidas", name)
}

// Rows es el número de filas escritas hasta ahora.
func (tableWriter *TableWriter) Rows() int64 {
	return tableWriter.rows
}

// Close termina la tabla y registra su conteo, su tamaño y su SHA-256 para el manifiesto.
func (tableWriter *TableWriter) Close() error {
	if tableWriter.done {
		return tableWriter.err
	}
	if tableWriter.err != nil {
		tableWriter.abort()
		return tableWriter.err
	}
	if err := tableWriter.emit([]byte{']'}); err != nil {
		tableWriter.abort()
		return err
	}
	tableWriter.done = true
	tableWriter.writer.current = nil
	closeErr := errors.Join(tableWriter.compressed.Close(), tableWriter.encrypted.Close(), tableWriter.file.Close())
	if closeErr != nil {
		tableWriter.err = closeErr
		return closeErr
	}
	manifest := &tableWriter.table.manifest
	manifest.Rows = tableWriter.rows
	manifest.Bytes = tableWriter.bytes
	manifest.SHA256 = hex.EncodeToString(tableWriter.hash.Sum(nil))
	return nil
}

// abort cierra el archivo temporal sin terminar la tabla; el paquete ya no se puede terminar.
func (tableWriter *TableWriter) abort() {
	if !tableWriter.done {
		tableWriter.done = true
		_ = tableWriter.file.Close()
		tableWriter.writer.current = nil
		tableWriter.writer.finished = true
	}
	if tableWriter.err == nil {
		tableWriter.err = errors.New("la tabla no se terminó")
	}
}

// countingWriter cuenta lo que se escribe en el destino.
type countingWriter struct {
	target io.Writer
	count  int64
}

func (writer *countingWriter) Write(data []byte) (int, error) {
	written, err := writer.target.Write(data)
	writer.count += int64(written)
	return written, err
}

// copied copia una lista (nunca nil): el escritor no comparte sus listas con quien lo llama.
func copied[T any](values []T) []T {
	return append([]T{}, values...)
}

// withPlaceholderHash valida la estructura de una tabla antes de tener su SHA-256 y su tamaño.
func withPlaceholderHash(table TableManifest) TableManifest {
	table.SHA256 = hex.EncodeToString(make([]byte, sha256.Size))
	table.Bytes = 2
	return table
}
