package catalog

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/logoraster"
)

// Imágenes del catálogo (etapa 9C, §5.2): antes de leer el encabezado o decodificar, un JPEG se
// recorre con la validación de marcadores del logotipo (logoraster.CheckJPEGMarkers), de modo que
// un JPEG progresivo de 1 MiB con miles de barridos vacíos se rechaza al instante en lugar de
// ocupar el procesador más de un minuto.

// progressiveJPEG arma un JPEG progresivo válido de 2048 × 2048 (un componente) con un barrido DC y
// scans barridos AC vacíos (corridas EOB) de 17 bytes cada uno; extra se inserta después de SOI.
// Es el mismo generador de las pruebas de logoraster (no exportado allí).
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

// ProgressiveJPEG expone el generador a la aceptación del catálogo con PostgreSQL (paquete
// catalog_test); solo existe al compilar las pruebas.
var ProgressiveJPEG = progressiveJPEG

// jpegComment es un segmento COM con el texto indicado.
func jpegComment(text []byte) []byte {
	length := len(text) + 2
	return append([]byte{0xFF, 0xFE, byte(length >> 8), byte(length)}, text...)
}

// uploadImage llama a la carga de imágenes sin base de datos: los rechazos ocurren antes de
// escribir, por lo que la transacción no se usa.
func uploadImage(t *testing.T, content []byte) error {
	t.Helper()
	body, err := json.Marshal(map[string]string{"content_base64": base64.StdEncoding.EncodeToString(content)})
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Method: "POST", Route: "catalog/images", Body: body, Actor: Actor{Permissions: []string{"catalog.images.manage"}}}
	_, err = images(context.Background(), nil, request, []string{"images"}, nil)
	return err
}

func assertImageRejected(t *testing.T, name string, content []byte, fragment string) {
	t.Helper()
	err := uploadImage(t, content)
	var problem *Error
	if !errors.As(err, &problem) || problem.Status != 400 || !strings.Contains(problem.Message, fragment) {
		t.Fatalf("%s: se esperaba un rechazo 400 con «%s», se obtuvo %v", name, fragment, err)
	}
}

// El JPEG progresivo de 1 MiB con unos 61 000 barridos se rechaza al instante con el mensaje del
// catálogo, y uno con un barrido de más que el máximo también.
func TestCatalogImageRejectsProgressiveJPEGWithTooManyScans(t *testing.T) {
	scans := (1024*1024 - len(progressiveJPEG(0, nil))) / 17
	content := progressiveJPEG(scans, nil)
	if len(content) > 1024*1024 || scans < 60000 {
		t.Fatalf("El archivo de prueba no reproduce el caso: %d bytes y %d barridos", len(content), scans)
	}
	started := time.Now()
	assertImageRejected(t, "JPEG de 1 MiB con miles de barridos", content, "La imagen JPEG tiene demasiados barridos progresivos (máximo 100)")
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("El rechazo tardó %v: se decodificó antes de contar los barridos", elapsed)
	}
	assertImageRejected(t, "JPEG con un barrido de más", progressiveJPEG(logoraster.MaxJPEGScans, nil), "demasiados barridos progresivos")
	// El máximo exacto (el barrido DC y 99 AC) pasa el recorrido; la carga completa se prueba con
	// PostgreSQL en la aceptación del catálogo.
	if err := checkImageMarkers(progressiveJPEG(logoraster.MaxJPEGScans-1, nil)); err != nil {
		t.Fatalf("Un JPEG con el máximo de barridos debía pasar el recorrido: %v", err)
	}
}

// Revisión de la etapa 9C: un byte suelto después de SOI (FF D8 00 FF DB …) no debe evitar el
// recorrido, porque el decodificador de Go acepta ese archivo y lo decodifica igual de despacio.
func TestCatalogImageCountsScansAfterStrayByteAfterSOI(t *testing.T) {
	scans := (1024*1024 - len(progressiveJPEG(0, []byte{0x00}))) / 17
	content := progressiveJPEG(scans, []byte{0x00})
	if len(content) > 1024*1024 || scans < 60000 || content[2] != 0x00 {
		t.Fatalf("El archivo de prueba no reproduce el caso: %d bytes, %d barridos, prefijo % X", len(content), scans, content[:4])
	}
	started := time.Now()
	assertImageRejected(t, "JPEG con un byte suelto tras SOI", content, "La imagen JPEG tiene demasiados barridos progresivos (máximo 100)")
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("El rechazo tardó %v: se decodificó antes de contar los barridos", elapsed)
	}
}

// Los segmentos tienen su propio límite y su propio mensaje; un JPEG secuencial común y un PNG no
// se ven afectados, y el mensaje nunca menciona el logotipo.
func TestCatalogImageLimitsJPEGSegments(t *testing.T) {
	tooMany := bytes.Repeat(jpegComment([]byte("x")), logoraster.MaxJPEGSegments)
	assertImageRejected(t, "JPEG con demasiados segmentos", progressiveJPEG(5, tooMany), "La imagen JPEG tiene demasiados segmentos (máximo 1024)")
	picture := image.NewGray(image.Rect(0, 0, 32, 32))
	for x := 0; x < 32; x++ {
		picture.SetGray(x, x, color.Gray{Y: uint8(x * 8)})
	}
	var ordinary bytes.Buffer
	if err := jpeg.Encode(&ordinary, picture, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	if err := checkImageMarkers(ordinary.Bytes()); err != nil {
		t.Fatalf("Un JPEG secuencial común no debía rechazarse: %v", err)
	}
	if err := checkImageMarkers([]byte("\x89PNG\r\n\x1a\n")); err != nil {
		t.Fatalf("Un PNG no se recorre como JPEG: %v", err)
	}
	var problem *Error
	err := uploadImage(t, progressiveJPEG(logoraster.MaxJPEGScans, nil))
	if !errors.As(err, &problem) || strings.Contains(problem.Message, "logotipo") {
		t.Fatalf("El mensaje del catálogo no debe hablar del logotipo: %v", err)
	}
}
