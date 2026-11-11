package logoraster

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"runtime"
	"strings"
	"testing"
)

// Pruebas de la validación del logotipo: PNG y JPEG íntegros dentro de los límites; rechazo de
// archivos vacíos, demasiado grandes, de otros formatos (SVG, GIF, WebP, HTML) aunque se llamen
// «.png», de encabezados con dimensiones enormes sin decodificar los píxeles, y de PNG y JPEG
// truncados o dañados.

func encodePNG(t *testing.T, picture image.Image) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func encodeJPEG(t *testing.T, picture image.Image) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, picture, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// gradient es una imagen opaca con colores distintos en cada píxel.
func gradient(width, height int) *image.NRGBA {
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			picture.Set(x, y, color.NRGBA{R: uint8(x * 7), G: uint8(y * 11), B: 128, A: 255})
		}
	}
	return picture
}

// pngChunk arma un fragmento PNG con su CRC correcto.
func pngChunk(kind string, data []byte) []byte {
	var buffer bytes.Buffer
	_ = binary.Write(&buffer, binary.BigEndian, uint32(len(data)))
	buffer.WriteString(kind)
	buffer.Write(data)
	checksum := crc32.ChecksumIEEE(append([]byte(kind), data...))
	_ = binary.Write(&buffer, binary.BigEndian, checksum)
	return buffer.Bytes()
}

// pngWithDimensions es un PNG cuyo encabezado declara width × height (RGBA de 8 bits) con unos
// pocos bytes de datos comprimidos: decodificarlo de verdad reservaría width × height × 4 bytes.
func pngWithDimensions(width, height uint32) []byte {
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header[0:4], width)
	binary.BigEndian.PutUint32(header[4:8], height)
	header[8] = 8
	header[9] = 6
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, _ = writer.Write(make([]byte, 4096))
	_ = writer.Close()
	content := append([]byte{}, pngSignature...)
	content = append(content, pngChunk("IHDR", header)...)
	content = append(content, pngChunk("IDAT", compressed.Bytes())...)
	content = append(content, pngChunk("IEND", nil)...)
	return content
}

// assertRejected exige un *Error cuyo mensaje contenga fragment.
func assertRejected(t *testing.T, name string, content []byte, fragment string) {
	t.Helper()
	logo, err := Validate(content)
	if err == nil || logo != nil {
		t.Fatalf("%s: debía rechazarse", name)
	}
	var problem *Error
	if !errors.As(err, &problem) {
		t.Fatalf("%s: el rechazo debe ser *logoraster.Error: %T %v", name, err, err)
	}
	if !strings.Contains(problem.Message, fragment) {
		t.Fatalf("%s: mensaje %q sin «%s»", name, problem.Message, fragment)
	}
}

func TestValidateAcceptsPNGAndJPEG(t *testing.T) {
	pngContent := encodePNG(t, gradient(10, 5))
	logo, err := Validate(pngContent)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(pngContent)
	if logo.Format != FormatPNG || logo.MediaType != "image/png" || logo.Width != 10 || logo.Height != 5 ||
		logo.Size != len(pngContent) || logo.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("PNG validado: %+v", logo)
	}
	if bounds := logo.Image.Bounds(); bounds.Dx() != 10 || bounds.Dy() != 5 {
		t.Fatalf("Imagen decodificada: %v", bounds)
	}
	jpegContent := encodeJPEG(t, gradient(16, 8))
	logo, err = Validate(jpegContent)
	if err != nil {
		t.Fatal(err)
	}
	if logo.Format != FormatJPEG || logo.MediaType != "image/jpeg" || logo.Width != 16 || logo.Height != 8 {
		t.Fatalf("JPEG validado: %+v", logo)
	}
	// Otros modelos de color de PNG: paleta con transparencia y gris de 16 bits.
	paletted := image.NewPaletted(image.Rect(0, 0, 3, 3), color.Palette{color.Transparent, color.Black})
	gray := image.NewGray16(image.Rect(0, 0, 4, 2))
	for name, picture := range map[string]image.Image{"paleta": paletted, "gris de 16 bits": gray} {
		if _, err := Validate(encodePNG(t, picture)); err != nil {
			t.Fatalf("PNG con %s: %v", name, err)
		}
	}
	// Los límites exactos se aceptan: 1 × 1 y 2048 × 2048.
	if _, err := Validate(encodePNG(t, image.NewGray(image.Rect(0, 0, 1, 1)))); err != nil {
		t.Fatalf("1 × 1: %v", err)
	}
	largest := encodePNG(t, image.NewGray(image.Rect(0, 0, MaxDimension, MaxDimension)))
	if logo, err := Validate(largest); err != nil || logo.Width != MaxDimension || logo.Height != MaxDimension {
		t.Fatalf("2048 × 2048: %v", err)
	}
}

func TestValidateRejectsSizeAndOtherFormats(t *testing.T) {
	assertRejected(t, "vacío", nil, "vacío")
	assertRejected(t, "vacío", []byte{}, "vacío")
	oversized := append(append([]byte{}, pngSignature...), make([]byte, MaxBytes)...)
	assertRejected(t, "más de 1 MiB", oversized, "1 MiB")
	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">` +
		`<script>alert(1)</script><rect width="10" height="10"/></svg>`)
	assertRejected(t, "SVG", svg, "PNG o JPEG")
	assertRejected(t, "SVG sin declaración", []byte(`<svg onload="alert(1)"></svg>`), "PNG o JPEG")
	var animated bytes.Buffer
	if err := gif.Encode(&animated, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.White, color.Black}), nil); err != nil {
		t.Fatal(err)
	}
	// Un GIF llamado «logo.png» sigue siendo GIF: la extensión nunca se consulta.
	assertRejected(t, "GIF con extensión engañosa", animated.Bytes(), "PNG o JPEG")
	webp := append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 24)...)
	assertRejected(t, "WebP", webp, "PNG o JPEG")
	assertRejected(t, "HTML", []byte("<html><body><img src=x onerror=alert(1)></body></html>"), "PNG o JPEG")
	assertRejected(t, "BMP", append([]byte("BM"), make([]byte, 60)...), "PNG o JPEG")
	// La firma de PNG con basura detrás no es un PNG.
	assertRejected(t, "firma PNG sin encabezado", append(append([]byte{}, pngSignature...), []byte("no es un png")...), "dañado")
	// La firma de JPEG con basura detrás tampoco.
	assertRejected(t, "firma JPEG sin marcadores", []byte{0xFF, 0xD8, 0xFF, 0x00, 0x01, 0x02}, "dañado")
}

// Un encabezado que declara dimensiones enormes se rechaza antes de decodificar: sin reservar la
// memoria que pediría la imagen (60 000 × 60 000 × 4 bytes ≈ 13 GiB).
func TestValidateRejectsHugeDimensionsWithoutDecoding(t *testing.T) {
	enormous := pngWithDimensions(60000, 60000)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	assertRejected(t, "PNG de 60 000 × 60 000", enormous, "2048 × 2048")
	runtime.ReadMemStats(&after)
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
		t.Fatalf("La validación reservó %d bytes: decodificó un encabezado enorme", allocated)
	}
	assertRejected(t, "PNG de 2049 × 1", pngWithDimensions(2049, 1), "2048 × 2048")
	assertRejected(t, "PNG de 1 × 2049", pngWithDimensions(1, 2049), "2048 × 2048")
	// JPEG con el SOF0 alterado para declarar 65 535 × 65 535.
	content := encodeJPEG(t, gradient(8, 8))
	marker := bytes.Index(content, []byte{0xFF, 0xC0})
	if marker < 0 {
		t.Fatal("El JPEG de prueba no tiene SOF0")
	}
	huge := append([]byte{}, content...)
	binary.BigEndian.PutUint16(huge[marker+5:], 65535)
	binary.BigEndian.PutUint16(huge[marker+7:], 65535)
	assertRejected(t, "JPEG de 65 535 × 65 535", huge, "2048 × 2048")
}

func TestValidateRejectsTruncatedAndDamagedFiles(t *testing.T) {
	pngContent := encodePNG(t, gradient(40, 30))
	// Sin el fragmento IEND (los últimos 12 bytes) y cortado a la mitad: el encabezado sigue
	// siendo válido, solo la decodificación completa lo detecta.
	if _, _, err := image.DecodeConfig(bytes.NewReader(pngContent[:len(pngContent)/2])); err != nil {
		t.Fatalf("El PNG cortado conserva un encabezado legible: %v", err)
	}
	assertRejected(t, "PNG sin IEND", pngContent[:len(pngContent)-12], "dañado")
	assertRejected(t, "PNG truncado", pngContent[:len(pngContent)/2], "dañado")
	// Un byte alterado dentro de IDAT invalida su CRC.
	damaged := append([]byte{}, pngContent...)
	idat := bytes.Index(damaged, []byte("IDAT"))
	damaged[idat+10] ^= 0xFF
	assertRejected(t, "PNG con CRC inválido", damaged, "dañado")

	jpegContent := encodeJPEG(t, gradient(64, 64))
	assertRejected(t, "JPEG sin EOI", jpegContent[:len(jpegContent)-2], "dañado")
	assertRejected(t, "JPEG truncado", jpegContent[:len(jpegContent)-10], "dañado")
	broken := append([]byte{}, jpegContent...)
	table := bytes.Index(broken, []byte{0xFF, 0xC4})
	for index := table + 5; index < table+25; index++ {
		broken[index] = 0xFF
	}
	assertRejected(t, "JPEG con tabla de Huffman dañada", broken, "dañado")
}
