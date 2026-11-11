package logoraster

import (
	"bytes"
	"testing"
	"time"
)

// Barridos y segmentos de un JPEG (revisión de la etapa 9B, hallazgo medio 2): el decodificador de
// Go no limita los barridos de un JPEG progresivo y cada uno recorre todos los bloques de la imagen;
// un archivo válido de 1 MiB con unos 61 000 barridos vacíos tardaba más de un minuto en validarse.
// La validación recorre antes los marcadores y rechaza más de MaxJPEGScans barridos o
// MaxJPEGSegments segmentos sin decodificar nada.

// progressiveJPEG arma un JPEG progresivo válido de 2048 × 2048 (un componente) con un barrido DC y
// scans barridos AC vacíos (corridas EOB), cada uno de 17 bytes. extra se inserta después de SOI.
func progressiveJPEG(scans int, extra []byte) []byte {
	var buffer bytes.Buffer
	buffer.Write([]byte{0xFF, 0xD8})
	buffer.Write(extra)
	// DQT: una tabla de cuantización con todos los valores en 1.
	buffer.Write([]byte{0xFF, 0xDB, 0x00, 67, 0x00})
	buffer.Write(bytes.Repeat([]byte{1}, 64))
	// SOF2: progresivo, 8 bits, 2048 × 2048, un componente.
	buffer.Write([]byte{0xFF, 0xC2, 0x00, 11, 8, 0x08, 0x00, 0x08, 0x00, 1, 1, 0x11, 0})
	// DHT DC 0: un código de longitud 1 para la categoría 0.
	dc := []byte{0x00, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x00}
	buffer.Write([]byte{0xFF, 0xC4, 0x00, byte(2 + len(dc))})
	buffer.Write(dc)
	// DHT AC 0: dos códigos de longitud 2 para EOB14 (0xE0) y EOB1 (0x10).
	ac := []byte{0x10, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xE0, 0x10}
	buffer.Write([]byte{0xFF, 0xC4, 0x00, byte(2 + len(ac))})
	buffer.Write(ac)
	// Barrido DC: 65 536 bloques con el código «0».
	buffer.Write([]byte{0xFF, 0xDA, 0x00, 8, 1, 1, 0x00, 0, 0, 0x00})
	buffer.Write(make([]byte, 65536/8))
	for index := 0; index < scans; index++ {
		buffer.Write([]byte{0xFF, 0xDA, 0x00, 8, 1, 1, 0x00, 1, 63, 0x00})
		buffer.Write([]byte{0x3F, 0xFF, 0x00, 0x3F, 0xFF, 0x00, 0x5F})
	}
	buffer.Write([]byte{0xFF, 0xD9})
	return buffer.Bytes()
}

// comment es un segmento COM con el texto indicado.
func comment(text []byte) []byte {
	length := len(text) + 2
	return append([]byte{0xFF, 0xFE, byte(length >> 8), byte(length)}, text...)
}

// Un JPEG progresivo con el máximo de barridos se acepta; con uno más se rechaza sin decodificar.
func TestValidateLimitsJPEGScans(t *testing.T) {
	// El barrido DC cuenta: MaxJPEGScans en total se aceptan.
	started := time.Now()
	logo, err := Validate(progressiveJPEG(MaxJPEGScans-1, nil))
	elapsed := time.Since(started)
	if err != nil || logo.Width != 2048 || logo.Height != 2048 || logo.Format != FormatJPEG {
		t.Fatalf("Un JPEG progresivo con %d barridos debía aceptarse: %v", MaxJPEGScans, err)
	}
	t.Logf("Validar un JPEG progresivo de 2048 × 2048 con %d barridos tomó %v", MaxJPEGScans, elapsed)
	assertRejected(t, "JPEG con un barrido de más", progressiveJPEG(MaxJPEGScans, nil), "demasiados barridos")
}

// El archivo de la revisión (1 MiB y unos 61 000 barridos) se rechaza al momento.
func TestValidateRejectsMegabyteOfScansQuickly(t *testing.T) {
	scans := (MaxBytes - len(progressiveJPEG(0, nil))) / 17
	content := progressiveJPEG(scans, nil)
	if len(content) > MaxBytes || scans < 60000 {
		t.Fatalf("El archivo de prueba no reproduce el caso: %d bytes y %d barridos", len(content), scans)
	}
	started := time.Now()
	assertRejected(t, "JPEG de 1 MiB con miles de barridos", content, "demasiados barridos")
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("El rechazo tardó %v: se decodificó antes de contar los barridos", elapsed)
	}
}

// Los segmentos con longitud tienen su propio límite; los bytes FF DA dentro de un segmento de
// metadatos no son barridos (el recorrido salta el contenido de cada segmento por su longitud).
func TestValidateLimitsJPEGSegments(t *testing.T) {
	fewComments := bytes.Repeat(comment([]byte("nota")), 10)
	if _, err := Validate(progressiveJPEG(5, fewComments)); err != nil {
		t.Fatalf("Un JPEG con algunos comentarios debía aceptarse: %v", err)
	}
	tooMany := bytes.Repeat(comment([]byte("x")), MaxJPEGSegments)
	assertRejected(t, "JPEG con demasiados segmentos", progressiveJPEG(5, tooMany), "demasiados segmentos")
	hidden := comment(bytes.Repeat([]byte{0xFF, 0xDA, 0x00, 0x08}, 1000))
	if _, err := Validate(progressiveJPEG(5, hidden)); err != nil {
		t.Fatalf("Los bytes FF DA dentro de un comentario se contaron como barridos: %v", err)
	}
	// Un JPEG secuencial común sigue aceptándose, y uno truncado lo sigue rechazando la decodificación.
	ordinary := encodeJPEG(t, gradient(64, 64))
	if _, err := Validate(ordinary); err != nil {
		t.Fatalf("Un JPEG secuencial común debía aceptarse: %v", err)
	}
	assertRejected(t, "JPEG truncado", ordinary[:len(ordinary)/2], "dañado")
}
