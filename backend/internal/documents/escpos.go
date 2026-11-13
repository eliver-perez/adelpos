package documents

import (
	"bytes"
	"context"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

// Codificación ESC/POS de un layout con atributos o imágenes. Es el único lugar, junto con el
// controlador de kitchen, que genera bytes de control: el texto de la IR ya está saneado y aquí se
// vuelve a limpiar antes de codificarlo en CP850.

// Raster es una imagen monocromática ya convertida al ancho imprimible (fase B, internal/logoraster):
// HeightDots filas de (WidthDots+7)/8 bytes, el bit más significativo primero y 1 = negro.
type Raster struct {
	WidthDots  int
	HeightDots int
	Bits       []byte
}

// RasterSource carga los bits de una imagen por su identificador al enviar un trabajo (la tabla la
// crea la fase B). Devuelve nil sin error si la imagen ya no existe: el bloque se omite con aviso.
type RasterSource interface {
	Raster(ctx context.Context, id string) (*Raster, error)
}

// rasterBand es la altura máxima de cada orden GS v 0: las impresoras térmicas comunes aceptan
// bloques grandes, pero un búfer pequeño puede descartar uno de cientos de filas.
const rasterBand = 128

// sizeCommand es el argumento de GS ! para cada tamaño (ancho en los bits 4–6, alto en 0–2).
func sizeCommand(size string) byte {
	switch size {
	case SizeDoubleHeight:
		return 0x01
	case SizeDoubleWidth:
		return 0x10
	case SizeDouble:
		return 0x11
	}
	return 0x00
}

// EncodeESCPos codifica un layout para una impresora ESC/POS: ESC @ y ESC t 2 (CP850); por línea de
// texto, GS ! n y ESC E n con el reinicio al final de la línea, el texto y el salto; por línea de
// imagen, GS v 0 con los bits que entregue rasters (una imagen sin bits se omite). Con el texto
// grande de Cocina cada línea va a doble tamaño, como antes de la 9B. Al final, el cierre del
// documento según el corte de la impresora (PaperCut.Ending, que sabe si el último renglón quedó
// abierto): sin corte, los tres saltos de siempre.
//
// GS v 0 se ejecuta al inicio de una línea, con el búfer de impresión vacío (muchas impresoras
// ignoran la imagen o la mezclan con el texto si no). Por eso, si una imagen sigue a la última
// línea de texto y esa línea quedó sin salto (tickets, precuentas y documentos de caja no terminan
// con salto), el salto se emite antes de la imagen. El texto del documento (Text) no cambia.
func EncodeESCPos(layout *Layout, rasters map[string]*Raster, cut PaperCut) []byte {
	result := []byte{0x1b, 0x40, 0x1b, 0x74, 0x02}
	lastText := -1
	for index, line := range layout.Lines {
		if line.Image == nil {
			lastText = index
		}
	}
	// lineOpen indica que la última línea de texto se escribió sin su salto.
	lineOpen := false
	for index, line := range layout.Lines {
		if line.Image != nil {
			command := imageCommand(line.Image, rasters)
			if lineOpen && len(command) > 0 {
				result = append(result, '\n')
				lineOpen = false
			}
			result = append(result, command...)
			continue
		}
		size := sizeCommand(line.Size)
		if layout.TextSize == TextSizeLarge {
			size = 0x11
		}
		bold := byte(0)
		if line.Bold {
			bold = 1
		}
		result = append(result, 0x1d, 0x21, size, 0x1b, 0x45, bold)
		result = append(result, encodeCP850(line.Text)...)
		result = append(result, 0x1b, 0x45, 0x00, 0x1d, 0x21, 0x00)
		// Las líneas se separan con un salto; la última solo lo lleva si el documento termina con
		// él (comandas), como el texto plano.
		if index != lastText || layout.TrailingNewline {
			result = append(result, '\n')
		} else {
			lineOpen = true
		}
	}
	return append(result, cut.Ending(lineOpen)...)
}

// Corte de papel (etapa 9F, §4.4, decisión D5). Cada impresora decide si corta al terminar, el tipo de
// corte y cuántos renglones avanza antes de cortar, para que la cuchilla no caiga sobre el texto.
const (
	// LegacyCutFeedLines son los saltos que llevaba siempre el final de un documento hasta la etapa 9E: los
	// de un documento sin corte y los del corte de las impresoras y fotografías anteriores a la 9F
	// (PaperCut.Legacy), de modo que su papel no cambia.
	LegacyCutFeedLines = 3
	// DefaultCutFeedLines son los renglones de avance de una impresora nueva.
	DefaultCutFeedLines = 4
	// MaxCutFeedLines es el máximo de la lista cerrada de renglones de avance (0 a 8).
	MaxCutFeedLines = 8
)

// PaperCut es el final de un documento ESC/POS.
//
//   - Sin corte (Enabled falso): los tres saltos de siempre, byte a byte iguales a los de la etapa 9E.
//   - Con corte: se cierra el último renglón si quedó abierto (precuentas, tickets y documentos de caja
//     terminan sin salto; comandas, avisos y pruebas, con él), después FeedLines renglones en blanco y el
//     corte GS V 0 (total) o GS V 1 (parcial, Partial). Así GS V llega siempre al inicio de un renglón,
//     también con 0 renglones de avance (una impresora solo imprime un renglón al recibir su salto y solo
//     corta al inicio de uno), y los renglones de avance son los mismos en todos los documentos.
//   - Legacy (revisión 9F, M1): el corte de las impresoras que ya cortaban antes de la 9F y de las
//     fotografías de esa época (sin renglones de avance guardados): los tres saltos de siempre y el corte,
//     sin cerrar aparte el renglón, como hasta la 9E. Con el avance uniforme, una impresora migrada con 3
//     renglones daría un renglón más en precuentas, tickets y documentos de caja (que terminan sin salto) que
//     en comandas: ningún número reproduce a la vez el papel de unos y otros, así que estas impresoras
//     conservan el final anterior hasta que el propietario elija sus renglones de avance.
//
// La mayoría de las impresoras sin cortador ignoran GS V.
type PaperCut struct {
	Enabled   bool
	Partial   bool
	FeedLines int
	Legacy    bool
}

// NoPaperCut es el final de un documento sin corte.
var NoPaperCut = PaperCut{}

// Ending devuelve los bytes que cierran el documento; lineOpen indica que el último renglón se escribió
// sin su salto. Nunca forma parte del pulso del cajón, que no imprime ni corta.
func (cut PaperCut) Ending(lineOpen bool) []byte {
	if !cut.Enabled {
		return []byte{'\n', '\n', '\n'}
	}
	mode := byte(0x00)
	if cut.Partial {
		mode = 0x01
	}
	if cut.Legacy {
		return []byte{'\n', '\n', '\n', 0x1d, 0x56, mode}
	}
	ending := []byte{}
	if lineOpen {
		ending = append(ending, '\n')
	}
	feed := min(max(cut.FeedLines, 0), MaxCutFeedLines)
	ending = append(ending, bytes.Repeat([]byte{'\n'}, feed)...)
	return append(ending, 0x1d, 0x56, mode)
}

// TextLineOpen indica si un texto plano termina con un renglón sin su salto (vacío no deja ninguno).
func TextLineOpen(text string) bool {
	return text != "" && !strings.HasSuffix(text, "\n")
}

// encodeCP850 limpia el texto y lo codifica en CP850; un carácter sin equivalente se imprime «?».
func encodeCP850(text string) []byte {
	encoder := charmap.CodePage850.NewEncoder()
	encoded := []byte{}
	for _, character := range CleanText(text) {
		if character == '\n' {
			continue
		}
		value, err := encoder.String(string(character))
		if err != nil {
			value = "?"
		}
		encoded = append(encoded, []byte(value)...)
	}
	return encoded
}

// imageCommand es la orden GS v 0 (modo normal) de una imagen, en franjas de hasta rasterBand
// filas. Sin bits o con un tamaño que no coincide con lo declarado, la imagen se omite.
func imageCommand(image *Image, rasters map[string]*Raster) []byte {
	raster := rasters[image.RasterID]
	if raster == nil || raster.WidthDots <= 0 || raster.HeightDots <= 0 {
		return nil
	}
	rowBytes := (raster.WidthDots + 7) / 8
	if len(raster.Bits) != rowBytes*raster.HeightDots {
		return nil
	}
	command := []byte{}
	for start := 0; start < raster.HeightDots; start += rasterBand {
		rows := min(rasterBand, raster.HeightDots-start)
		command = append(command, 0x1d, 0x76, 0x30, 0x00,
			byte(rowBytes&0xff), byte(rowBytes>>8), byte(rows&0xff), byte(rows>>8))
		command = append(command, raster.Bits[start*rowBytes:(start+rows)*rowBytes]...)
	}
	return command
}

// MissingRasters devuelve los identificadores de las imágenes del layout sin bits disponibles.
func MissingRasters(layout *Layout, rasters map[string]*Raster) []string {
	missing := []string{}
	for _, line := range layout.Lines {
		if line.Image == nil {
			continue
		}
		if imageCommand(line.Image, rasters) == nil {
			missing = append(missing, line.Image.RasterID)
		}
	}
	return missing
}
