package legacypkg

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"io"
	"math"
)

// Flujo cifrado por bloques (STREAM, como en age o Tink): el texto claro se parte en bloques de chunkSize bytes y cada
// uno se cifra con AES-256-GCM. El nonce de 12 bytes es el prefijo aleatorio del paquete (7 bytes), el número de bloque
// (uint32) y una bandera de último bloque (1 byte); el AAD es el SHA-256 del encabezado. Así un bloque alterado,
// reordenado, duplicado o quitado, o un archivo truncado, no se descifra: el último bloque solo es válido al final.

const (
	tagSize       = 16
	nonceSize     = 12
	lastBlockFlag = 1
)

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func blockNonce(prefix []byte, counter uint32, last bool) []byte {
	nonce := make([]byte, nonceSize)
	copy(nonce, prefix)
	binary.BigEndian.PutUint32(nonce[noncePrefixSize:], counter)
	if last {
		nonce[nonceSize-1] = lastBlockFlag
	}
	return nonce
}

// chunkWriter cifra lo que se le escribe. Un bloque lleno solo se cifra cuando llega más texto (así se sabe que no es
// el último); Close cifra el último bloque, que puede ir incompleto o vacío.
type chunkWriter struct {
	aead      cipher.AEAD
	prefix    []byte
	aad       []byte
	chunkSize int
	counter   uint32
	buffer    []byte
	sealed    []byte
	output    io.Writer
	closed    bool
	err       error
}

func newChunkWriter(output io.Writer, key, prefix, aad []byte, chunkSize int) (*chunkWriter, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	return &chunkWriter{
		aead:      aead,
		prefix:    prefix,
		aad:       aad,
		chunkSize: chunkSize,
		buffer:    make([]byte, 0, chunkSize),
		sealed:    make([]byte, 0, chunkSize+tagSize),
		output:    output,
	}, nil
}

func (writer *chunkWriter) Write(data []byte) (int, error) {
	if writer.err != nil {
		return 0, writer.err
	}
	if writer.closed {
		return 0, errors.New("flujo cifrado cerrado")
	}
	written := 0
	for len(data) > 0 {
		if len(writer.buffer) == writer.chunkSize {
			if err := writer.seal(false); err != nil {
				return written, err
			}
		}
		count := copy(writer.buffer[len(writer.buffer):writer.chunkSize], data)
		writer.buffer = writer.buffer[:len(writer.buffer)+count]
		data = data[count:]
		written += count
	}
	return written, nil
}

func (writer *chunkWriter) seal(last bool) error {
	if writer.counter == math.MaxUint32 {
		writer.err = errors.New("demasiados bloques en el flujo cifrado")
		return writer.err
	}
	nonce := blockNonce(writer.prefix, writer.counter, last)
	writer.sealed = writer.aead.Seal(writer.sealed[:0], nonce, writer.buffer, writer.aad)
	if _, err := writer.output.Write(writer.sealed); err != nil {
		writer.err = err
		return err
	}
	writer.counter++
	writer.buffer = writer.buffer[:0]
	return nil
}

// Close cifra el último bloque. No cierra el destino.
func (writer *chunkWriter) Close() error {
	if writer.err != nil {
		return writer.err
	}
	if writer.closed {
		return nil
	}
	writer.closed = true
	return writer.seal(true)
}

// chunkReader descifra el flujo. Un bloque completo es el último solo si después ya no hay nada; uno incompleto
// siempre es el último. Implementa io.ByteReader para que gzip no lea por adelantado y se pueda comprobar que no
// sobra nada después del gzip.
type chunkReader struct {
	aead      cipher.AEAD
	prefix    []byte
	aad       []byte
	chunkSize int
	source    *bufio.Reader
	counter   uint32
	sealed    []byte
	plain     []byte
	position  int
	finished  bool
	err       error
}

func newChunkReader(source io.Reader, key, prefix, aad []byte, chunkSize int) (*chunkReader, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	return &chunkReader{
		aead:      aead,
		prefix:    prefix,
		aad:       aad,
		chunkSize: chunkSize,
		source:    bufio.NewReaderSize(source, 64<<10),
		sealed:    make([]byte, chunkSize+tagSize),
		plain:     make([]byte, 0, chunkSize),
	}, nil
}

func (reader *chunkReader) fill() error {
	if reader.err != nil {
		return reader.err
	}
	for reader.position == len(reader.plain) {
		if reader.finished {
			return io.EOF
		}
		if err := reader.open(); err != nil {
			reader.err = err
			return err
		}
	}
	return nil
}

// open lee y descifra el bloque siguiente.
func (reader *chunkReader) open() error {
	count, err := io.ReadFull(reader.source, reader.sealed)
	last := false
	switch {
	case err == nil:
		// Bloque completo: es el último solo si el archivo termina aquí.
		if _, peekErr := reader.source.Peek(1); peekErr == io.EOF {
			last = true
		} else if peekErr != nil {
			return damaged("lectura del bloque", peekErr)
		}
	case errors.Is(err, io.ErrUnexpectedEOF):
		last = true
	case errors.Is(err, io.EOF):
		return damaged("el paquete está truncado (falta el último bloque)", nil)
	default:
		return damaged("lectura del bloque", err)
	}
	if count < tagSize {
		return damaged("bloque truncado", nil)
	}
	nonce := blockNonce(reader.prefix, reader.counter, last)
	plain, openErr := reader.aead.Open(reader.plain[:0], nonce, reader.sealed[:count], reader.aad)
	if openErr != nil {
		return damaged("un bloque cifrado no es válido (alterado, reordenado, truncado o de más)", openErr)
	}
	if reader.counter == math.MaxUint32 {
		return damaged("demasiados bloques", nil)
	}
	reader.counter++
	reader.plain = plain
	reader.position = 0
	reader.finished = last
	return nil
}

func (reader *chunkReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if err := reader.fill(); err != nil {
		return 0, err
	}
	count := copy(buffer, reader.plain[reader.position:])
	reader.position += count
	return count, nil
}

func (reader *chunkReader) ReadByte() (byte, error) {
	if err := reader.fill(); err != nil {
		return 0, err
	}
	value := reader.plain[reader.position]
	reader.position++
	return value, nil
}

// expectEnd comprueba que el flujo terminó exactamente en el último bloque, sin texto claro de más.
func (reader *chunkReader) expectEnd() error {
	err := reader.fill()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	return damaged("datos de más después del contenido", nil)
}
