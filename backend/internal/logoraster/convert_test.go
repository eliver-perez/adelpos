package logoraster

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/brand"
)

// Pruebas de la conversión: umbral y difuminado exactos en imágenes pequeñas conocidas, relleno
// por alineación, 58 frente a 80 mm, altura máxima, transparencia, opciones inválidas, determinismo
// y la marca de AD-EL POS (2048 × 682) como caso real.

// filled es una imagen width × height pintada con paint(x, y).
func filled(width, height int, paint func(x, y int) color.Color) *image.NRGBA {
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			picture.Set(x, y, paint(x, y))
		}
	}
	return picture
}

func grayLevel(level uint8) color.Color {
	return color.NRGBA{R: level, G: level, B: level, A: 255}
}

func options(paper, percent int, align, method string, threshold int) Options {
	return Options{PaperWidth: paper, WidthPercent: percent, Align: align, Method: method, Threshold: threshold}
}

// expectedRaster arma el raster esperado: black(x, y) en coordenadas del logotipo, desplazado offset
// puntos dentro del ancho imprimible.
func expectedRaster(printable, width, height, offset int, black func(x, y int) bool) []byte {
	raster := newBlankRaster(printable, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if black(x, y) {
				raster.set(x+offset, y)
			}
		}
	}
	return raster.bits
}

// allWhite indica si ningún bit del raster está en negro.
func allWhite(bits []byte) bool {
	for _, value := range bits {
		if value != 0 {
			return false
		}
	}
	return true
}

func convert(t *testing.T, source image.Image, settings Options) *Raster {
	t.Helper()
	raster, err := Convert(source, settings)
	if err != nil {
		t.Fatal(err)
	}
	return raster
}

// 96 puntos son el 25 % de 384: la imagen no se remuestrea y cada píxel decide un punto. Columnas
// de luminancia 0, 127, 128 y 255 en bloques de 24: con umbral 128 solo 0 y 127 son negras; con 200,
// también 128; con 1, solo el 0.
func TestConvertThresholdExact(t *testing.T) {
	levels := []uint8{0, 127, 128, 255}
	source := filled(96, 3, func(x, y int) color.Color {
		return grayLevel(levels[x/24])
	})
	cases := []struct {
		threshold int
		black     func(level uint8) bool
	}{
		{128, func(level uint8) bool { return level < 128 }},
		{200, func(level uint8) bool { return level < 200 }},
		{1, func(level uint8) bool { return level == 0 }},
		{255, func(level uint8) bool { return level < 255 }},
	}
	for _, testCase := range cases {
		raster := convert(t, source, options(Paper58, 25, AlignLeft, MethodThreshold, testCase.threshold))
		if raster.Width() != Dots58 || raster.Height() != 3 || raster.BytesPerRow() != 48 {
			t.Fatalf("Umbral %d: raster %d × %d", testCase.threshold, raster.Width(), raster.Height())
		}
		expected := expectedRaster(Dots58, 96, 3, 0, func(x, y int) bool {
			return testCase.black(levels[x/24])
		})
		if !bytes.Equal(raster.Bits(), expected) {
			t.Fatalf("Umbral %d: bits %x, esperados %x", testCase.threshold, raster.Bits()[:12], expected[:12])
		}
	}
}

// Floyd–Steinberg calculado a mano sobre 3 × 2 puntos de luminancia 100 (ver los repartos en el
// comentario de ditherFloydSteinberg): negro, blanco, negro en ambas filas.
func TestDitherFloydSteinbergExact(t *testing.T) {
	black := ditherFloydSteinberg([]int32{100, 100, 100, 100, 100, 100}, 3, 2)
	expected := []bool{true, false, true, true, false, true}
	if !slices.Equal(black, expected) {
		t.Fatalf("Difuminado de 3 × 2 a 100: %v, esperado %v", black, expected)
	}
	// Los extremos no se difuminan: negro sigue negro y blanco sigue blanco.
	if result := ditherFloydSteinberg([]int32{0, 0, 0, 0}, 2, 2); !slices.Equal(result, []bool{true, true, true, true}) {
		t.Fatalf("Negro: %v", result)
	}
	if result := ditherFloydSteinberg([]int32{255, 255, 255, 255}, 2, 2); !slices.Equal(result, []bool{false, false, false, false}) {
		t.Fatalf("Blanco: %v", result)
	}
	// Un gris del 50 % produce la mitad de puntos negros en un tablero alternado.
	half := make([]int32, 16)
	for index := range half {
		half[index] = 127
	}
	result := ditherFloydSteinberg(half, 4, 4)
	expectedHalf := []bool{
		true, false, true, false,
		false, true, false, true,
		true, false, true, false,
		false, true, false, true,
	}
	if !slices.Equal(result, expectedHalf) {
		t.Fatalf("Gris 127 de 4 × 4: %v", result)
	}
}

// El difuminado completo por Convert: un gris uniforme sin remuestreo produce aproximadamente la
// proporción de puntos negros de su luminancia, siempre igual en cada conversión y distinto del
// umbral (que lo dejaría todo blanco o todo negro).
func TestConvertDitherProportionAndDeterminism(t *testing.T) {
	source := filled(96, 40, func(x, y int) color.Color {
		return grayLevel(191)
	})
	settings := options(Paper58, 25, AlignCenter, MethodDither, DefaultThreshold)
	first := convert(t, source, settings)
	second := convert(t, source, settings)
	if !bytes.Equal(first.Bits(), second.Bits()) {
		t.Fatal("El difuminado debe ser determinista")
	}
	blackDots := 0
	for y := 0; y < first.Height(); y++ {
		for x := 0; x < first.Width(); x++ {
			if first.Black(x, y) {
				blackDots++
				if x < 144 || x >= 240 {
					t.Fatalf("Punto negro fuera del logotipo centrado: (%d, %d)", x, y)
				}
			}
		}
	}
	// Luminancia 191 ≈ 25 % de negro sobre 96 × 40 = 3 840 puntos.
	if blackDots < 900 || blackDots > 1020 {
		t.Fatalf("Proporción de negro del difuminado: %d de 3 840", blackDots)
	}
	threshold := convert(t, source, options(Paper58, 25, AlignCenter, MethodThreshold, DefaultThreshold))
	if !allWhite(threshold.Bits()) || bytes.Equal(threshold.Bits(), first.Bits()) {
		t.Fatal("Con umbral 128 un gris 191 queda en blanco")
	}
}

// El logotipo se coloca a la izquierda, al centro o a la derecha rellenando con blanco el resto del
// ancho imprimible: 96 puntos en 384 empiezan en 0, 144 o 288.
func TestConvertAlignmentPadding(t *testing.T) {
	source := filled(96, 2, func(x, y int) color.Color {
		return grayLevel(0)
	})
	for align, offset := range map[string]int{AlignLeft: 0, AlignCenter: 144, AlignRight: 288} {
		raster := convert(t, source, options(Paper58, 25, align, MethodThreshold, DefaultThreshold))
		expected := expectedRaster(Dots58, 96, 2, offset, func(x, y int) bool { return true })
		if !bytes.Equal(raster.Bits(), expected) {
			t.Fatalf("Alineación %s: los puntos negros no empiezan en %d", align, offset)
		}
		for y := 0; y < 2; y++ {
			if raster.Black(offset-1, y) || !raster.Black(offset, y) || !raster.Black(offset+95, y) || raster.Black(offset+96, y) {
				t.Fatalf("Alineación %s: bordes del logotipo en la fila %d", align, y)
			}
		}
	}
	// En 80 mm, 25 % son 144 puntos: centrado empieza en (576 − 144) / 2 = 216, a la derecha en 432.
	wide := filled(144, 1, func(x, y int) color.Color {
		return grayLevel(0)
	})
	for align, offset := range map[string]int{AlignLeft: 0, AlignCenter: 216, AlignRight: 432} {
		raster := convert(t, wide, options(Paper80, 25, align, MethodThreshold, DefaultThreshold))
		expected := expectedRaster(Dots80, 144, 1, offset, func(x, y int) bool { return true })
		if !bytes.Equal(raster.Bits(), expected) {
			t.Fatalf("80 mm, alineación %s: desplazamiento distinto de %d", align, offset)
		}
	}
}

// 58 y 80 mm: el mismo logotipo al 50 % mide 192 o 288 puntos de ancho (con la altura
// proporcional) dentro de un raster de 384 o 576 puntos.
func TestConvertPaperWidths(t *testing.T) {
	source := filled(50, 25, func(x, y int) color.Color {
		return grayLevel(0)
	})
	cases := []struct {
		paper, printable, rowBytes, width, height int
	}{
		{Paper58, Dots58, 48, 192, 96},
		{Paper80, Dots80, 72, 288, 144},
	}
	for _, testCase := range cases {
		raster := convert(t, source, options(testCase.paper, 50, AlignLeft, MethodThreshold, DefaultThreshold))
		if raster.Width() != testCase.printable || raster.BytesPerRow() != testCase.rowBytes || raster.Height() != testCase.height {
			t.Fatalf("%d mm: raster %d × %d (%d bytes por fila)", testCase.paper, raster.Width(), raster.Height(), raster.BytesPerRow())
		}
		// Un negro uniforme sigue negro tras el escalado: exactamente width puntos por fila.
		for y := 0; y < raster.Height(); y++ {
			for x := 0; x < raster.Width(); x++ {
				if raster.Black(x, y) != (x < testCase.width) {
					t.Fatalf("%d mm: punto (%d, %d)", testCase.paper, x, y)
				}
			}
		}
	}
}

// Un logotipo alto se reduce en proporción para no exceder 320 puntos: 100 × 1000 al 100 % de 58 mm
// pediría 384 × 3 840; queda en 32 × 320, centrado en (384 − 32) / 2 = 176.
func TestConvertMaximumHeight(t *testing.T) {
	source := filled(100, 1000, func(x, y int) color.Color {
		return grayLevel(0)
	})
	raster := convert(t, source, options(Paper58, 100, AlignCenter, MethodThreshold, DefaultThreshold))
	if raster.Height() != MaxHeightDots || raster.Width() != Dots58 {
		t.Fatalf("Altura máxima: %d × %d", raster.Width(), raster.Height())
	}
	for y := 0; y < raster.Height(); y++ {
		for x := 0; x < raster.Width(); x++ {
			if raster.Black(x, y) != (x >= 176 && x < 208) {
				t.Fatalf("Punto (%d, %d) fuera del logotipo reducido a 32 puntos", x, y)
			}
		}
	}
	if size := len(raster.Bits()); size != 48*320 {
		t.Fatalf("Tamaño del raster a la altura máxima: %d bytes", size)
	}
	width, height := targetSize(2048, 2048, Dots80, 100)
	if width != 320 || height != 320 {
		t.Fatalf("Un cuadrado de 2048 al 100 %% de 80 mm: %d × %d", width, height)
	}
	width, height = targetSize(3, 1, Dots58, 25)
	if width != 96 || height != 32 {
		t.Fatalf("Proporción 3:1 al 25 %%: %d × %d", width, height)
	}
	width, height = targetSize(2048, 1, Dots58, 25)
	if width != 96 || height != 1 {
		t.Fatalf("Una franja de un píxel conserva al menos un punto de alto: %d × %d", width, height)
	}
}

// La transparencia se imprime en blanco: un negro totalmente transparente no deja puntos y uno
// semitransparente se compone sobre blanco (gris de luminancia 127).
func TestConvertTransparencyBecomesWhite(t *testing.T) {
	transparent := filled(96, 2, func(x, y int) color.Color {
		if x < 48 {
			return color.NRGBA{A: 0}
		}
		return color.NRGBA{A: 255}
	})
	raster := convert(t, transparent, options(Paper58, 25, AlignLeft, MethodThreshold, DefaultThreshold))
	expected := expectedRaster(Dots58, 96, 2, 0, func(x, y int) bool { return x >= 48 })
	if !bytes.Equal(raster.Bits(), expected) {
		t.Fatal("Los píxeles transparentes deben quedar en blanco y los opacos en negro")
	}
	halfway := filled(96, 1, func(x, y int) color.Color {
		return color.NRGBA{A: 128}
	})
	flattened := Flatten(halfway, 96, 1)
	if luminance := luminanceOf(flattened)[0]; luminance != 127 {
		t.Fatalf("Negro al 50 %% sobre blanco: luminancia %d", luminance)
	}
	light := convert(t, halfway, options(Paper58, 25, AlignLeft, MethodThreshold, 100))
	if !allWhite(light.Bits()) {
		t.Fatal("Con umbral 100 el gris 127 queda en blanco")
	}
	dark := convert(t, halfway, options(Paper58, 25, AlignLeft, MethodThreshold, 200))
	if !dark.Black(0, 0) || !dark.Black(95, 0) || dark.Black(96, 0) {
		t.Fatal("Con umbral 200 el gris 127 queda en negro")
	}
}

func TestConvertRejectsInvalidOptions(t *testing.T) {
	source := filled(10, 10, func(x, y int) color.Color {
		return grayLevel(0)
	})
	valid := DefaultOptions(Paper80)
	if err := valid.Validate(); err != nil {
		t.Fatalf("Las opciones por omisión deben ser válidas: %v", err)
	}
	cases := map[string]struct {
		change   func(*Options)
		fragment string
	}{
		"papel de 72 mm":     {func(settings *Options) { settings.PaperWidth = 72 }, "58 u 80"},
		"ancho de 24 %":      {func(settings *Options) { settings.WidthPercent = 24 }, "25 %"},
		"ancho de 101 %":     {func(settings *Options) { settings.WidthPercent = 101 }, "100 %"},
		"alineación vacía":   {func(settings *Options) { settings.Align = "" }, "alineación"},
		"alineación libre":   {func(settings *Options) { settings.Align = "justify" }, "alineación"},
		"método desconocido": {func(settings *Options) { settings.Method = "halftone" }, "umbral o difuminado"},
		"umbral 0":           {func(settings *Options) { settings.Threshold = 0 }, "1 y 255"},
		"umbral 256":         {func(settings *Options) { settings.Threshold = 256 }, "1 y 255"},
		"umbral con difuminado": {func(settings *Options) {
			settings.Method = MethodDither
			settings.Threshold = -1
		}, "1 y 255"},
	}
	for name, testCase := range cases {
		settings := DefaultOptions(Paper58)
		testCase.change(&settings)
		_, err := Convert(source, settings)
		var problem *Error
		if !errors.As(err, &problem) || !strings.Contains(problem.Message, testCase.fragment) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := Convert(nil, valid); err == nil {
		t.Fatal("Una imagen nula debe rechazarse")
	}
	if _, err := Convert(image.NewRGBA(image.Rectangle{}), valid); err == nil {
		t.Fatal("Una imagen vacía debe rechazarse")
	}
}

// La marca de AD-EL POS (2048 × 682, RGBA transparente) como logotipo real: dimensiones esperadas,
// puntos negros solo dentro del logotipo y la misma conversión en cada ejecución.
func TestConvertBrandLogo(t *testing.T) {
	logo, err := Validate(brand.LightLogoPNG())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		settings      Options
		width, height int
	}{
		{options(Paper58, 60, AlignCenter, MethodThreshold, DefaultThreshold), 230, 77},
		{options(Paper80, 100, AlignCenter, MethodDither, DefaultThreshold), 576, 192},
	}
	for _, testCase := range cases {
		started := time.Now()
		raster := convert(t, logo.Image, testCase.settings)
		elapsed := time.Since(started)
		again := convert(t, logo.Image, testCase.settings)
		if !bytes.Equal(raster.Bits(), again.Bits()) {
			t.Fatal("La conversión de la marca debe ser determinista")
		}
		if raster.Height() != testCase.height {
			t.Fatalf("%d mm: alto %d, esperado %d", testCase.settings.PaperWidth, raster.Height(), testCase.height)
		}
		printable, _ := PrintableDots(testCase.settings.PaperWidth)
		offset := (printable - testCase.width) / 2
		blackDots := 0
		for y := 0; y < raster.Height(); y++ {
			for x := 0; x < raster.Width(); x++ {
				if !raster.Black(x, y) {
					continue
				}
				blackDots++
				if x < offset || x >= offset+testCase.width {
					t.Fatalf("Punto negro fuera del logotipo: (%d, %d)", x, y)
				}
			}
		}
		total := testCase.width * testCase.height
		if blackDots < total/10 || blackDots > total*7/10 {
			t.Fatalf("Proporción de negro de la marca: %d de %d", blackDots, total)
		}
		t.Logf("Marca a %d mm (%s): %d × %d puntos, %d bytes de raster, %d bytes GS v 0, %s",
			testCase.settings.PaperWidth, testCase.settings.Method, raster.Width(), raster.Height(), len(raster.Bits()),
			len(raster.ESCPos()), elapsed.Round(time.Millisecond))
	}
}
