package logoraster

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// MaxBandRows es la altura máxima de cada bloque GS v 0 que genera ESCPos (ver doc.go).
const MaxBandRows = 128

// Raster es un logotipo monocromático listo para imprimir: ancho imprimible completo (384 o 576
// puntos), alto en puntos y bits empaquetados por fila, el más significativo a la izquierda y
// 1 = negro. Es inmutable: se crea con Convert o con NewRaster y sus métodos nunca lo modifican.
type Raster struct {
	width  int
	height int
	bits   []byte
}

// newBlankRaster crea un raster en blanco (todos los bits en 0).
func newBlankRaster(width, height int) *Raster {
	rowBytes := (width + 7) / 8
	return &Raster{width: width, height: height, bits: make([]byte, rowBytes*height)}
}

// NewRaster reconstruye un raster guardado (por ejemplo, la conversión almacenada de un logotipo).
// Exige uno de los anchos imprimibles, una altura de 1 a MaxHeightDots y exactamente
// BytesPerRow × alto bytes; copia los bits recibidos.
func NewRaster(width, height int, bits []byte) (*Raster, error) {
	if width != Dots58 && width != Dots80 {
		return nil, invalid("El ancho del raster debe ser de 384 o 576 puntos.")
	}
	if height < 1 || height > MaxHeightDots {
		return nil, invalid("La altura del raster debe estar entre 1 y 320 puntos.")
	}
	raster := newBlankRaster(width, height)
	if len(bits) != len(raster.bits) {
		return nil, invalid("Los datos del raster no corresponden a su ancho y su altura.")
	}
	copy(raster.bits, bits)
	return raster, nil
}

// Width es el ancho en puntos (el ancho imprimible del papel).
func (raster *Raster) Width() int {
	return raster.width
}

// Height es el alto en puntos.
func (raster *Raster) Height() int {
	return raster.height
}

// BytesPerRow es el número de bytes de cada fila (48 en 58 mm y 72 en 80 mm).
func (raster *Raster) BytesPerRow() int {
	return (raster.width + 7) / 8
}

// Bits devuelve una copia de los bits empaquetados, fila por fila de arriba abajo.
func (raster *Raster) Bits() []byte {
	copied := make([]byte, len(raster.bits))
	copy(copied, raster.bits)
	return copied
}

// Black indica si el punto (x, y) es negro; fuera del raster siempre es blanco.
func (raster *Raster) Black(x, y int) bool {
	if x < 0 || y < 0 || x >= raster.width || y >= raster.height {
		return false
	}
	return raster.bits[y*raster.BytesPerRow()+x/8]&(0x80>>(x%8)) != 0
}

// set pinta de negro el punto (x, y); solo lo usa la conversión.
func (raster *Raster) set(x, y int) {
	raster.bits[y*raster.BytesPerRow()+x/8] |= 0x80 >> (x % 8)
}

// PNG es la vista previa de cómo saldrá en papel: un PNG de 1 bit por punto (paleta de dos colores,
// índice 0 blanco y 1 negro) del tamaño exacto del raster, con su relleno de alineación.
func (raster *Raster) PNG() ([]byte, error) {
	palette := color.Palette{color.White, color.Black}
	preview := image.NewPaletted(image.Rect(0, 0, raster.width, raster.height), palette)
	for y := 0; y < raster.height; y++ {
		for x := 0; x < raster.width; x++ {
			if raster.Black(x, y) {
				preview.Pix[y*preview.Stride+x] = 1
			}
		}
	}
	var buffer bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&buffer, preview); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// ESCPos devuelve los bytes del comando de imagen raster GS v 0 (1D 76 30 00 xL xH yL yH datos) en
// franjas consecutivas de a lo más MaxBandRows filas. x es BytesPerRow e y las filas de la franja,
// ambos en little endian. No incluye inicialización, justificación, saltos ni corte: el transporte
// lo envía al inicio de una línea y continúa con el texto.
func (raster *Raster) ESCPos() []byte {
	rowBytes := raster.BytesPerRow()
	bands := (raster.height + MaxBandRows - 1) / MaxBandRows
	output := make([]byte, 0, len(raster.bits)+8*bands)
	for start := 0; start < raster.height; start += MaxBandRows {
		rows := min(MaxBandRows, raster.height-start)
		output = append(output,
			0x1D, 0x76, 0x30, 0x00,
			byte(rowBytes&0xFF), byte(rowBytes>>8),
			byte(rows&0xFF), byte(rows>>8),
		)
		output = append(output, raster.bits[start*rowBytes:(start+rows)*rowBytes]...)
	}
	return output
}
