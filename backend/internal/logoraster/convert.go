package logoraster

import (
	"image"

	xdraw "golang.org/x/image/draw"
)

const (
	// Paper58 y Paper80 son los anchos de papel admitidos, en milímetros.
	Paper58 = 58
	Paper80 = 80
	// Dots58 y Dots80 son los anchos imprimibles en puntos (8 puntos por mm: 48 y 72 mm).
	Dots58 = 384
	Dots80 = 576

	// MaxHeightDots es la altura máxima del logotipo convertido: 320 puntos son 40 mm de papel y
	// acotan el raster a 15 360 bytes en 58 mm y 23 040 en 80 mm. Un logotipo más alto se reduce
	// en proporción (ancho incluido).
	MaxHeightDots = 320

	MinWidthPercent     = 25
	MaxWidthPercent     = 100
	DefaultWidthPercent = 60

	AlignLeft   = "left"
	AlignCenter = "center"
	AlignRight  = "right"

	MethodThreshold = "threshold"
	MethodDither    = "dither"

	MinThreshold     = 1
	MaxThreshold     = 255
	DefaultThreshold = 128

	// ditherMidpoint es el punto de decisión del difuminado (la mitad de la escala de 0 a 255).
	ditherMidpoint = 128
)

// Options es la presentación del logotipo en el papel.
type Options struct {
	// PaperWidth es el ancho del papel en milímetros: 58 u 80.
	PaperWidth int
	// WidthPercent es el ancho del logotipo respecto al ancho imprimible (25–100).
	WidthPercent int
	// Align es left, center o right.
	Align string
	// Method es threshold (umbral) o dither (difuminado Floyd–Steinberg).
	Method string
	// Threshold (1–255) solo se usa con threshold: un punto es negro si su luminancia es menor.
	Threshold int
}

// DefaultOptions es la presentación por omisión para un ancho de papel: 60 % del ancho imprimible,
// centrado y con umbral 128.
func DefaultOptions(paperWidth int) Options {
	return Options{
		PaperWidth:   paperWidth,
		WidthPercent: DefaultWidthPercent,
		Align:        AlignCenter,
		Method:       MethodThreshold,
		Threshold:    DefaultThreshold,
	}
}

// PrintableDots devuelve el ancho imprimible en puntos de un ancho de papel admitido.
func PrintableDots(paperWidth int) (int, bool) {
	switch paperWidth {
	case Paper58:
		return Dots58, true
	case Paper80:
		return Dots80, true
	}
	return 0, false
}

// Validate comprueba que todas las opciones estén dentro de su lista cerrada o de su rango. El
// umbral se valida también con el difuminado porque se conserva como parte de la presentación.
func (options Options) Validate() error {
	if _, ok := PrintableDots(options.PaperWidth); !ok {
		return invalid("El ancho de papel debe ser de 58 u 80 mm.")
	}
	if options.WidthPercent < MinWidthPercent || options.WidthPercent > MaxWidthPercent {
		return invalid("El ancho del logotipo debe estar entre 25 % y 100 % del ancho imprimible.")
	}
	switch options.Align {
	case AlignLeft, AlignCenter, AlignRight:
	default:
		return invalid("La alineación del logotipo debe ser izquierda, centro o derecha.")
	}
	switch options.Method {
	case MethodThreshold, MethodDither:
	default:
		return invalid("El método de conversión debe ser umbral o difuminado.")
	}
	if options.Threshold < MinThreshold || options.Threshold > MaxThreshold {
		return invalid("El umbral debe estar entre 1 y 255.")
	}
	return nil
}

// Convert convierte una imagen (normalmente Logo.Image) en el raster monocromático de un ancho de
// papel, rellenado con blanco hasta el ancho imprimible completo según la alineación.
func Convert(source image.Image, options Options) (*Raster, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if source == nil || source.Bounds().Empty() {
		return nil, invalid("El logotipo no tiene píxeles.")
	}
	printable, _ := PrintableDots(options.PaperWidth)
	bounds := source.Bounds()
	width, height := targetSize(bounds.Dx(), bounds.Dy(), printable, options.WidthPercent)
	flattened := Flatten(source, width, height)
	luminance := luminanceOf(flattened)
	var black []bool
	if options.Method == MethodDither {
		black = ditherFloydSteinberg(luminance, width, height)
	} else {
		black = applyThreshold(luminance, options.Threshold)
	}
	offset := alignmentOffset(printable, width, options.Align)
	raster := newBlankRaster(printable, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if black[y*width+x] {
				raster.set(x+offset, y)
			}
		}
	}
	return raster, nil
}

// targetSize calcula el tamaño del logotipo en puntos: el porcentaje del ancho imprimible con la
// altura proporcional (redondeo al entero más cercano, mínimo un punto) y, si la altura excede
// MaxHeightDots, ambas medidas reducidas en proporción.
func targetSize(sourceWidth, sourceHeight, printable, percent int) (int, int) {
	width := max(1, divideRounded(printable*percent, 100))
	height := max(1, divideRounded(sourceHeight*width, sourceWidth))
	if height > MaxHeightDots {
		height = MaxHeightDots
		width = max(1, divideRounded(sourceWidth*MaxHeightDots, sourceHeight))
	}
	return width, height
}

// divideRounded divide enteros positivos redondeando a la mitad hacia arriba.
func divideRounded(numerator, denominator int) int {
	return (2*numerator + denominator) / (2 * denominator)
}

// alignmentOffset es la columna (en puntos) donde empieza el logotipo dentro del ancho imprimible.
func alignmentOffset(printable, width int, align string) int {
	switch align {
	case AlignLeft:
		return 0
	case AlignRight:
		return printable - width
	}
	return (printable - width) / 2
}

// Flatten escala source a width × height con Catmull-Rom y lo compone sobre un fondo blanco: el
// resultado es opaco y la transparencia queda en blanco. Si el tamaño ya coincide, solo compone
// (sin remuestrear), de modo que una imagen del tamaño exacto conserva sus píxeles.
func Flatten(source image.Image, width, height int) *image.RGBA {
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.Draw(target, target.Bounds(), image.White, image.Point{}, xdraw.Src)
	bounds := source.Bounds()
	if bounds.Dx() == width && bounds.Dy() == height {
		xdraw.Draw(target, target.Bounds(), source, bounds.Min, xdraw.Over)
		return target
	}
	xdraw.CatmullRom.Scale(target, target.Bounds(), source, bounds, xdraw.Over, nil)
	return target
}

// luminanceOf calcula la luminancia (0–255) de cada píxel de una imagen opaca con los coeficientes
// enteros de BT.601 que usa color.GrayModel (19595 + 38470 + 7471 = 65536).
func luminanceOf(picture *image.RGBA) []int32 {
	bounds := picture.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	values := make([]int32, width*height)
	for y := 0; y < height; y++ {
		row := picture.Pix[y*picture.Stride:]
		for x := 0; x < width; x++ {
			red := int32(row[4*x])
			green := int32(row[4*x+1])
			blue := int32(row[4*x+2])
			values[y*width+x] = (19595*red + 38470*green + 7471*blue + 1<<15) >> 16
		}
	}
	return values
}

// applyThreshold marca como negro todo punto con luminancia menor que el umbral.
func applyThreshold(luminance []int32, threshold int) []bool {
	black := make([]bool, len(luminance))
	for index, value := range luminance {
		black[index] = value < int32(threshold)
	}
	return black
}

// ditherFloydSteinberg difumina la luminancia con Floyd–Steinberg (de izquierda a derecha y de
// arriba abajo): cada punto se decide contra la mitad de la escala y su error se reparte 7/16 a la
// derecha, 3/16 abajo a la izquierda, 5/16 abajo y 1/16 abajo a la derecha. Todo es aritmética
// entera; la última fracción recibe el resto para no perder error por el truncamiento, y la parte
// que caería fuera de la imagen se descarta.
func ditherFloydSteinberg(luminance []int32, width, height int) []bool {
	values := make([]int32, len(luminance))
	copy(values, luminance)
	black := make([]bool, len(values))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*width + x
			current := values[index]
			var printed int32 = 255
			if current < ditherMidpoint {
				printed = 0
				black[index] = true
			}
			spread := current - printed
			right := spread * 7 / 16
			belowLeft := spread * 3 / 16
			below := spread * 5 / 16
			belowRight := spread - right - belowLeft - below
			if x+1 < width {
				values[index+1] += right
			}
			if y+1 < height {
				if x > 0 {
					values[index+width-1] += belowLeft
				}
				values[index+width] += below
				if x+1 < width {
					values[index+width+1] += belowRight
				}
			}
		}
	}
	return black
}
