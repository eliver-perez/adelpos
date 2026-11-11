package logoraster

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// Pruebas del raster: bytes GS v 0 exactos (con franjas de 128 filas), reconstrucción validada de
// un raster guardado y vista previa PNG de 1 bit decodificable e idéntica a los bits.

// patternBits llena rows filas de rowBytes bytes con un patrón reconocible por fila y columna.
func patternBits(rowBytes, rows int) []byte {
	bits := make([]byte, rowBytes*rows)
	for index := range bits {
		bits[index] = byte(index*31 + index/rowBytes)
	}
	return bits
}

func TestESCPosExactBytes(t *testing.T) {
	bits := make([]byte, 48*2)
	bits[0] = 0x80
	bits[47] = 0x01
	bits[48] = 0xFF
	raster, err := NewRaster(Dots58, 2, bits)
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte{0x1D, 0x76, 0x30, 0x00, 48, 0, 2, 0}
	expected = append(expected, bits...)
	if got := raster.ESCPos(); !bytes.Equal(got, expected) {
		t.Fatalf("GS v 0 de 384 × 2:\n%x\nesperado\n%x", got, expected)
	}
	if !raster.Black(0, 0) || raster.Black(1, 0) || !raster.Black(383, 0) || !raster.Black(7, 1) || raster.Black(8, 1) {
		t.Fatal("Orden de bits: MSB a la izquierda y 1 = negro")
	}
	if raster.Black(-1, 0) || raster.Black(0, 2) || raster.Black(384, 0) {
		t.Fatal("Fuera del raster todo es blanco")
	}
	// 80 mm: x = 72 bytes por fila.
	wide, err := NewRaster(Dots80, 1, make([]byte, 72))
	if err != nil {
		t.Fatal(err)
	}
	if header := wide.ESCPos()[:8]; !bytes.Equal(header, []byte{0x1D, 0x76, 0x30, 0x00, 72, 0, 1, 0}) {
		t.Fatalf("Encabezado GS v 0 de 80 mm: %x", header)
	}
}

// Un raster de 300 filas viaja en tres bloques consecutivos de 128, 128 y 44 filas, cada uno con su
// encabezado (yH = 0) y exactamente sus filas.
func TestESCPosBands(t *testing.T) {
	bits := patternBits(72, 300)
	raster, err := NewRaster(Dots80, 300, bits)
	if err != nil {
		t.Fatal(err)
	}
	output := raster.ESCPos()
	if len(output) != len(bits)+3*8 {
		t.Fatalf("Tamaño del envío: %d", len(output))
	}
	position := 0
	row := 0
	for _, rows := range []int{128, 128, 44} {
		header := output[position : position+8]
		if !bytes.Equal(header[:4], []byte{0x1D, 0x76, 0x30, 0x00}) {
			t.Fatalf("Franja desde la fila %d sin GS v 0: %x", row, header)
		}
		if width := binary.LittleEndian.Uint16(header[4:6]); width != 72 {
			t.Fatalf("Bytes por fila: %d", width)
		}
		if height := binary.LittleEndian.Uint16(header[6:8]); int(height) != rows || header[7] != 0 {
			t.Fatalf("Filas de la franja: %d (yH = %d)", height, header[7])
		}
		position += 8
		data := output[position : position+rows*72]
		if !bytes.Equal(data, bits[row*72:(row+rows)*72]) {
			t.Fatalf("Datos de la franja desde la fila %d", row)
		}
		position += rows * 72
		row += rows
	}
	// La altura máxima (320) son tres franjas; 128 filas exactas, una sola.
	exact, _ := NewRaster(Dots58, 128, make([]byte, 48*128))
	if got := exact.ESCPos(); len(got) != 8+48*128 || got[6] != 128 {
		t.Fatalf("128 filas deben ir en un solo bloque: %d bytes", len(got))
	}
}

func TestNewRasterValidatesStoredData(t *testing.T) {
	cases := map[string]struct {
		width, height, length int
	}{
		"ancho de 512 puntos": {512, 10, 64 * 10},
		"alto cero":           {Dots58, 0, 0},
		"alto de 321":         {Dots58, 321, 48 * 321},
		"bytes de menos":      {Dots58, 10, 48*10 - 1},
		"bytes de más":        {Dots80, 10, 72*10 + 1},
	}
	for name, testCase := range cases {
		if _, err := NewRaster(testCase.width, testCase.height, make([]byte, testCase.length)); err == nil {
			t.Fatalf("%s: debía rechazarse", name)
		}
	}
	bits := patternBits(48, 3)
	raster, err := NewRaster(Dots58, 3, bits)
	if err != nil {
		t.Fatal(err)
	}
	// NewRaster copia los datos y Bits entrega copias: el raster es inmutable.
	bits[0] ^= 0xFF
	copied := raster.Bits()
	copied[1] ^= 0xFF
	if again := raster.Bits(); again[0] == bits[0] || again[1] == copied[1] {
		t.Fatal("El raster debe conservar sus propios bits")
	}
}

// La vista previa es un PNG de paleta con profundidad de 1 bit, del tamaño exacto del raster
// (relleno incluido) y con los mismos puntos negros sobre blanco.
func TestPreviewPNG(t *testing.T) {
	source := filled(96, 10, func(x, y int) color.Color {
		if (x+y)%3 == 0 {
			return grayLevel(0)
		}
		return grayLevel(255)
	})
	raster := convert(t, source, options(Paper58, 25, AlignRight, MethodThreshold, DefaultThreshold))
	content, err := raster.PNG()
	if err != nil {
		t.Fatal(err)
	}
	// IHDR: profundidad de bits en el byte 24 y tipo de color en el 25 (3 = paleta).
	if !bytes.HasPrefix(content, pngSignature) || content[24] != 1 || content[25] != 3 {
		t.Fatalf("La vista previa debe ser PNG de paleta de 1 bit: profundidad %d, tipo %d", content[24], content[25])
	}
	decoded, err := png.Decode(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if bounds := decoded.Bounds(); bounds != image.Rect(0, 0, Dots58, 10) {
		t.Fatalf("Tamaño de la vista previa: %v", bounds)
	}
	blackDots := 0
	for y := 0; y < 10; y++ {
		for x := 0; x < Dots58; x++ {
			red, green, blue, _ := decoded.At(x, y).RGBA()
			black := red == 0 && green == 0 && blue == 0
			white := red == 0xFFFF && green == 0xFFFF && blue == 0xFFFF
			if black != raster.Black(x, y) || black == white {
				t.Fatalf("Vista previa distinta del raster en (%d, %d)", x, y)
			}
			if black {
				blackDots++
				if x < 288 {
					t.Fatalf("Punto negro fuera del logotipo alineado a la derecha: (%d, %d)", x, y)
				}
			}
		}
	}
	if blackDots == 0 {
		t.Fatal("La vista previa no tiene puntos negros")
	}
	again, _ := raster.PNG()
	if !bytes.Equal(content, again) {
		t.Fatal("La vista previa debe ser determinista")
	}
}
