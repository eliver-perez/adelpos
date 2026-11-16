package legacypkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
)

// OpenOptions configura la lectura. Con Limits en cero se usan DefaultLimits.
type OpenOptions struct {
	Limits Limits
}

// Reader lee un paquete en flujo, sin cargarlo completo en memoria: el manifiesto al abrir y después cada tabla en
// orden con NextTable y sus filas con TableReader.Next.
//
// La integridad completa (bloques cifrados, huellas, conteos y final del paquete) se confirma al recorrerlo:
// NextTable devuelve io.EOF solo cuando todo cuadró. Quien importe no debe confirmar su transacción antes de eso.
type Reader struct {
	header     Header
	manifest   Manifest
	limits     Limits
	encrypted  *chunkReader
	compressed *gzip.Reader
	archive    *tar.Reader
	next       int
	current    *TableReader
	done       bool
	err        error
}

func resolveLimits(options OpenOptions) Limits {
	if options.Limits == (Limits{}) {
		return DefaultLimits()
	}
	return options.Limits
}

// Inspect lee solo el encabezado en claro (sin la frase de paso): dice si el archivo es un paquete y de qué versión.
func Inspect(source io.Reader, options OpenOptions) (Header, error) {
	_, params, err := readHeader(source, resolveLimits(options))
	if err != nil {
		return Header{}, err
	}
	return headerInfo(params), nil
}

func headerInfo(params headerParams) Header {
	return Header{
		Version: FormatVersion,
		KDF: KDFParams{
			Time:      params.KDF.Time,
			MemoryKiB: params.KDF.MemoryKiB,
			Threads:   params.KDF.Threads,
		},
		ChunkSize: params.ChunkSize,
	}
}

// Open abre un paquete: valida el encabezado, comprueba la frase de paso y lee el manifiesto.
func Open(source io.Reader, passphrase string, options OpenOptions) (*Reader, error) {
	limits := resolveLimits(options)
	raw, params, err := readHeader(source, limits)
	if err != nil {
		return nil, err
	}
	keys, err := checkPassphrase(passphrase, params)
	if err != nil {
		return nil, err
	}
	headerHash := sha256.Sum256(raw)
	encrypted, err := newChunkReader(source, keys.encryption, params.NoncePrefix, headerHash[:], params.ChunkSize)
	if err != nil {
		return nil, damaged("cifrado", err)
	}
	compressed, err := gzip.NewReader(encrypted)
	if err != nil {
		return nil, asPackageError(err, "gzip")
	}
	// Un solo miembro gzip: lo que siga después es un paquete alterado (se comprueba al final).
	compressed.Multistream(false)
	limited := &limitedReader{source: compressed, remaining: limits.MaxUncompressedBytes}
	reader := &Reader{
		header:     headerInfo(params),
		limits:     limits,
		encrypted:  encrypted,
		compressed: compressed,
		archive:    tar.NewReader(limited),
	}
	if err := reader.readManifest(); err != nil {
		return nil, err
	}
	return reader, nil
}

func (reader *Reader) readManifest() error {
	entry, err := reader.archive.Next()
	if err != nil {
		return asPackageError(err, "primera entrada del tar")
	}
	if entry.Name != ManifestFile || entry.Typeflag != tar.TypeReg {
		return damaged("la primera entrada no es el manifiesto", nil)
	}
	if entry.Size < 2 || entry.Size > reader.limits.MaxManifestBytes {
		return limitExceeded("tamaño del manifiesto")
	}
	encoded, err := io.ReadAll(io.LimitReader(reader.archive, entry.Size))
	if err != nil {
		return asPackageError(err, "manifiesto")
	}
	if int64(len(encoded)) != entry.Size {
		return damaged("manifiesto truncado", nil)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	if err := decoder.Decode(&reader.manifest); err != nil {
		return damaged("JSON del manifiesto", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return damaged("datos de más en el manifiesto", nil)
	}
	return validateManifest(&reader.manifest, reader.header.Version, reader.limits)
}

// Header son los parámetros del encabezado.
func (reader *Reader) Header() Header {
	return reader.header
}

// Manifest es el manifiesto del paquete (ya validado contra su huella de contenido).
func (reader *Reader) Manifest() Manifest {
	return reader.manifest
}

// NextTable entrega la tabla siguiente en el orden del manifiesto. Si la anterior no se leyó completa, la recorre y la
// comprueba. Al terminar comprueba el final del paquete y devuelve io.EOF.
func (reader *Reader) NextTable() (*TableReader, error) {
	if reader.err != nil {
		return nil, reader.err
	}
	if reader.current != nil {
		if err := reader.current.drain(); err != nil {
			return nil, reader.fail(err)
		}
		reader.current = nil
	}
	if reader.done {
		return nil, io.EOF
	}
	if reader.next == len(reader.manifest.Tables) {
		if err := reader.expectEnd(); err != nil {
			return nil, reader.fail(err)
		}
		reader.done = true
		return nil, io.EOF
	}
	info := reader.manifest.Tables[reader.next]
	entry, err := reader.archive.Next()
	if err != nil {
		return nil, reader.fail(asPackageError(err, "entrada de tabla"))
	}
	if entry.Name != info.File || entry.Typeflag != tar.TypeReg || entry.Size != info.Bytes {
		return nil, reader.fail(damaged("la entrada del tar no corresponde al manifiesto", nil))
	}
	reader.next++
	reader.current = newTableReader(reader, info)
	return reader.current, nil
}

// expectEnd comprueba que después de la última tabla no hay nada: fin del tar, fin del gzip (con su CRC) y último
// bloque cifrado.
func (reader *Reader) expectEnd() error {
	if _, err := reader.archive.Next(); err != io.EOF {
		if err == nil {
			return damaged("entradas de más en el tar", nil)
		}
		return asPackageError(err, "final del tar")
	}
	var extra [1]byte
	count, err := reader.compressed.Read(extra[:])
	if count != 0 {
		return damaged("datos de más después del tar", nil)
	}
	if err != io.EOF {
		return asPackageError(err, "final del gzip")
	}
	return reader.encrypted.expectEnd()
}

func (reader *Reader) fail(err error) error {
	if reader.err == nil {
		reader.err = err
	}
	return reader.err
}

// Verify recorre un paquete completo y comprueba todo; devuelve su manifiesto.
func Verify(source io.Reader, passphrase string, options OpenOptions) (Manifest, error) {
	reader, err := Open(source, passphrase, options)
	if err != nil {
		return Manifest{}, err
	}
	for {
		table, err := reader.NextTable()
		if err == io.EOF {
			return reader.Manifest(), nil
		}
		if err != nil {
			return Manifest{}, err
		}
		if err := table.drain(); err != nil {
			return Manifest{}, err
		}
	}
}

// TableReader entrega las filas de una tabla, cada una con un valor por columna en el orden de Columns: nil, bool,
// string o json.Number (texto decimal exacto).
type TableReader struct {
	reader   *Reader
	info     TableManifest
	hash     hash.Hash
	entry    io.Reader
	limit    *rowLimitReader
	decoder  *json.Decoder
	rows     int64
	started  bool
	finished bool
	err      error
}

func newTableReader(reader *Reader, info TableManifest) *TableReader {
	digest := sha256.New()
	entry := io.TeeReader(reader.archive, digest)
	limit := &rowLimitReader{source: entry}
	decoder := json.NewDecoder(limit)
	decoder.UseNumber()
	return &TableReader{
		reader:  reader,
		info:    info,
		hash:    digest,
		entry:   entry,
		limit:   limit,
		decoder: decoder,
	}
}

// Info es la tabla en el manifiesto.
func (table *TableReader) Info() TableManifest {
	return table.info
}

// Columns son las columnas de cada fila.
func (table *TableReader) Columns() []Column {
	return table.info.Columns
}

// ColumnIndex es la posición de una columna o -1.
func (table *TableReader) ColumnIndex(name string) int {
	for index, column := range table.info.Columns {
		if column.Name == name {
			return index
		}
	}
	return -1
}

// Next entrega la fila siguiente. Al final comprueba el conteo y el SHA-256 de la tabla y devuelve io.EOF.
func (table *TableReader) Next() ([]any, error) {
	if table.err != nil {
		return nil, table.err
	}
	if table.finished {
		return nil, io.EOF
	}
	values, err := table.next()
	if err != nil && err != io.EOF {
		table.err = err
		table.reader.fail(err)
	}
	return values, err
}

func (table *TableReader) next() ([]any, error) {
	maxRow := table.reader.limits.MaxRowBytes
	table.limit.allowUntil(table.decoder.InputOffset() + maxRow + rowLimitSlack)
	if !table.started {
		table.started = true
		if err := expectDelim(table.decoder, '['); err != nil {
			return nil, err
		}
	}
	if table.decoder.More() {
		values, err := table.readRow()
		if err != nil {
			return nil, err
		}
		table.rows++
		if table.rows > table.info.Rows {
			return nil, damaged("la tabla tiene más filas que el manifiesto", nil)
		}
		return values, nil
	}
	if err := expectDelim(table.decoder, ']'); err != nil {
		return nil, err
	}
	if _, err := table.decoder.Token(); err != io.EOF {
		return nil, decodeError(err, "datos de más después de la tabla")
	}
	// Lo que el decodificador no haya leído también cuenta para el SHA-256 (no debería quedar nada).
	if _, err := io.Copy(io.Discard, table.entry); err != nil {
		return nil, asPackageError(err, "final de la tabla")
	}
	if table.rows != table.info.Rows {
		return nil, damaged("el conteo de filas no coincide con el manifiesto", nil)
	}
	if hex.EncodeToString(table.hash.Sum(nil)) != table.info.SHA256 {
		return nil, damaged("el SHA-256 de la tabla no coincide con el manifiesto", nil)
	}
	table.finished = true
	return nil, io.EOF
}

func (table *TableReader) readRow() ([]any, error) {
	if err := expectDelim(table.decoder, '{'); err != nil {
		return nil, err
	}
	values := make([]any, len(table.info.Columns))
	for index, column := range table.info.Columns {
		key, err := table.decoder.Token()
		if err != nil {
			return nil, decodeError(err, "fila")
		}
		if name, ok := key.(string); !ok || name != column.Name {
			return nil, damaged("las columnas de una fila no corresponden al manifiesto", nil)
		}
		value, err := table.decoder.Token()
		if err != nil {
			return nil, decodeError(err, "fila")
		}
		switch typed := value.(type) {
		case nil, bool, string, json.Number:
			values[index] = typed
		default:
			return nil, damaged("valor anidado en una fila", nil)
		}
	}
	if err := expectDelim(table.decoder, '}'); err != nil {
		return nil, err
	}
	return values, nil
}

// drain recorre lo que falte de la tabla para comprobarla.
func (table *TableReader) drain() error {
	for {
		_, err := table.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func expectDelim(decoder *json.Decoder, delim json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return decodeError(err, "estructura de la tabla")
	}
	if found, ok := token.(json.Delim); !ok || found != delim {
		return damaged("estructura de la tabla", nil)
	}
	return nil
}

func decodeError(err error, detail string) error {
	if errors.Is(err, errRowTooLarge) {
		return limitExceeded("tamaño de una fila")
	}
	return asPackageError(err, detail)
}

// asPackageError conserva un error del paquete (bloque alterado, límite) y convierte cualquier otro en «dañado».
func asPackageError(err error, detail string) error {
	if err == nil {
		return nil
	}
	if KindOf(err) != "" {
		return err
	}
	if errors.Is(err, errUncompressedTooLarge) {
		return limitExceeded("tamaño descomprimido")
	}
	return damaged(detail, err)
}

var (
	errUncompressedTooLarge = errors.New("tamaño descomprimido excedido")
	errRowTooLarge          = errors.New("fila demasiado grande")
)

// rowLimitSlack es el margen de lectura adelantada del decodificador sobre el tamaño máximo de una fila.
const rowLimitSlack = 64 << 10

// limitedReader corta el flujo descomprimido al llegar al límite (bomba de descompresión).
type limitedReader struct {
	source    io.Reader
	remaining int64
}

func (reader *limitedReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if reader.remaining <= 0 {
		// Ya se entregó el máximo: si el contenido sigue, el paquete excede el límite.
		var probe [1]byte
		count, err := reader.source.Read(probe[:])
		if count > 0 {
			return 0, errUncompressedTooLarge
		}
		if err == nil {
			err = io.ErrNoProgress
		}
		return 0, err
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	count, err := reader.source.Read(buffer)
	reader.remaining -= int64(count)
	return count, err
}

// rowLimitReader impide que el decodificador JSON acumule en memoria una fila más grande que el límite: solo le
// entrega bytes hasta cierta posición, que avanza al empezar cada fila.
type rowLimitReader struct {
	source   io.Reader
	consumed int64
	until    int64
}

func (reader *rowLimitReader) allowUntil(position int64) {
	reader.until = position
}

func (reader *rowLimitReader) Read(buffer []byte) (int, error) {
	available := reader.until - reader.consumed
	if available <= 0 {
		return 0, errRowTooLarge
	}
	if int64(len(buffer)) > available {
		buffer = buffer[:available]
	}
	count, err := reader.source.Read(buffer)
	reader.consumed += int64(count)
	return count, err
}
