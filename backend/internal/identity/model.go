package identity

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"local/ad-el-pos/server/internal/logoraster"
)

// Límites y claves de la identidad documental.
const (
	// MaxTextCharacters y MaxTextLines acotan la información adicional, el encabezado y el pie
	// (los mismos límites que el texto fijo de las plantillas; también los exige la 0019).
	MaxTextCharacters = 300
	MaxTextLines      = 8

	// ConverterVersion identifica el algoritmo de conversión de internal/logoraster con el que se
	// guardan las conversiones. Si un día cambia, se incrementa: las conversiones nuevas no se
	// confunden con las ya guardadas, que siguen sirviendo a los documentos emitidos.
	ConverterVersion = 1

	// ditherThreshold es el umbral que se guarda con el difuminado, cuyo punto de decisión es fijo.
	ditherThreshold = 128

	// EventChanged se publica en el ámbito SSE documents al cambiar la identidad o el logotipo.
	EventChanged = "documents.identity.changed"

	PermissionManage        = "documents.identity.manage"
	PermissionTemplatesRead = "documents.templates.read"
	PermissionSettingsRead  = "settings.read"
)

// paperWidths son los anchos de papel con conversión guardada, en el orden en que se presentan.
var paperWidths = []int{logoraster.Paper58, logoraster.Paper80}

// Presentation es cómo se imprime el logotipo en los documentos térmicos.
type Presentation struct {
	WidthPercent int    `json:"width_percent"`
	Align        string `json:"align"`
	Method       string `json:"method"`
	Threshold    int    `json:"threshold"`
}

// DefaultPresentation es la presentación inicial (la de la 0019): 60 %, centrado, umbral 128.
func DefaultPresentation() Presentation {
	options := logoraster.DefaultOptions(logoraster.Paper58)
	return Presentation{
		WidthPercent: options.WidthPercent,
		Align:        options.Align,
		Method:       options.Method,
		Threshold:    options.Threshold,
	}
}

// options devuelve las opciones de conversión para un ancho de papel.
func (presentation Presentation) options(paperWidth int) logoraster.Options {
	return logoraster.Options{
		PaperWidth:   paperWidth,
		WidthPercent: presentation.WidthPercent,
		Align:        presentation.Align,
		Method:       presentation.Method,
		Threshold:    presentation.Threshold,
	}
}

// validate comprueba la presentación con las mismas reglas que la conversión.
func (presentation Presentation) validate() error {
	return publicError(presentation.options(logoraster.Paper58).Validate())
}

// rasterThreshold es el umbral con el que se identifica la conversión guardada: el del usuario con
// umbral y el punto de decisión fijo con difuminado (el umbral no interviene en el resultado).
func (presentation Presentation) rasterThreshold() int {
	if presentation.Method == logoraster.MethodDither {
		return ditherThreshold
	}
	return presentation.Threshold
}

// Person identifica a quien cargó o guardó, con su nombre visible.
type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Establishment son los datos de Configuración general que la identidad reutiliza (solo lectura).
type Establishment struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
}

// RasterView describe una conversión del logotipo para un ancho de papel. DataURI (PNG de 1 bit con
// exactamente los puntos que se imprimen) solo viaja en las lecturas, no en las escrituras.
type RasterView struct {
	PaperWidth int    `json:"paper_width"`
	RasterID   string `json:"raster_id,omitempty"`
	WidthDots  int    `json:"width_dots"`
	HeightDots int    `json:"height_dots"`
	DataURI    string `json:"data_uri,omitempty"`
}

// LogoView describe el logotipo vigente. DataURI es el original y solo viaja en las lecturas.
type LogoView struct {
	ID         string       `json:"id"`
	MediaType  string       `json:"media_type"`
	Width      int          `json:"width"`
	Height     int          `json:"height"`
	SizeBytes  int          `json:"size_bytes"`
	SHA256     string       `json:"sha256"`
	UploadedAt time.Time    `json:"uploaded_at"`
	UploadedBy *Person      `json:"uploaded_by"`
	DataURI    string       `json:"data_uri,omitempty"`
	Rasters    []RasterView `json:"rasters"`
}

// PaperWidth es un ancho de papel admitido con su ancho imprimible.
type PaperWidth struct {
	PaperWidth int `json:"paper_width"`
	WidthDots  int `json:"width_dots"`
}

// Limits son los límites que la interfaz muestra y respeta (Go los vuelve a validar).
type Limits struct {
	TextMaxCharacters int          `json:"text_max_characters"`
	TextMaxLines      int          `json:"text_max_lines"`
	LogoMaxBytes      int          `json:"logo_max_bytes"`
	LogoMaxDimension  int          `json:"logo_max_dimension"`
	WidthPercentMin   int          `json:"width_percent_min"`
	WidthPercentMax   int          `json:"width_percent_max"`
	ThresholdMin      int          `json:"threshold_min"`
	ThresholdMax      int          `json:"threshold_max"`
	MaxHeightDots     int          `json:"max_height_dots"`
	PaperWidths       []PaperWidth `json:"paper_widths"`
}

func currentLimits() Limits {
	widths := []PaperWidth{}
	for _, paperWidth := range paperWidths {
		dots, _ := logoraster.PrintableDots(paperWidth)
		widths = append(widths, PaperWidth{PaperWidth: paperWidth, WidthDots: dots})
	}
	return Limits{
		TextMaxCharacters: MaxTextCharacters,
		TextMaxLines:      MaxTextLines,
		LogoMaxBytes:      logoraster.MaxBytes,
		LogoMaxDimension:  logoraster.MaxDimension,
		WidthPercentMin:   logoraster.MinWidthPercent,
		WidthPercentMax:   logoraster.MaxWidthPercent,
		ThresholdMin:      logoraster.MinThreshold,
		ThresholdMax:      logoraster.MaxThreshold,
		MaxHeightDots:     logoraster.MaxHeightDots,
		PaperWidths:       widths,
	}
}

// View es la identidad documental completa: la respuesta de GET documents/identity y, sin vistas
// previas, la de cada escritura.
type View struct {
	Revision       int           `json:"revision"`
	Establishment  Establishment `json:"establishment"`
	AdditionalInfo string        `json:"additional_info"`
	HeaderText     string        `json:"header_text"`
	FooterText     string        `json:"footer_text"`
	Presentation   Presentation  `json:"presentation"`
	Logo           *LogoView     `json:"logo"`
	UpdatedAt      time.Time     `json:"updated_at"`
	UpdatedBy      *Person       `json:"updated_by"`
	Limits         Limits        `json:"limits"`
	Warnings       []string      `json:"warnings"`
}

// normalizeText prepara un texto de la identidad para guardarlo: unifica los saltos de línea,
// convierte los tabuladores en espacios, elimina los controles e invisibles (a la impresora nunca
// llegan comandos), quita los espacios al final de cada línea y las líneas vacías al inicio y al
// final, y aplica los límites de caracteres y de líneas. label nombra el campo en el mensaje.
func normalizeText(label, value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", invalid(label + " contiene caracteres no válidos.")
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	cleaned := strings.Map(func(character rune) rune {
		if character == '\n' {
			return character
		}
		if character == '\t' {
			return ' '
		}
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return -1
		}
		return character
	}, value)
	lines := strings.Split(cleaned, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	cleaned = strings.Trim(strings.Join(lines, "\n"), "\n")
	if utf8.RuneCountInString(cleaned) > MaxTextCharacters {
		return "", invalid(label + " admite como máximo " + strconv.Itoa(MaxTextCharacters) + " caracteres.")
	}
	if strings.Count(cleaned, "\n")+1 > MaxTextLines {
		return "", invalid(label + " admite como máximo " + strconv.Itoa(MaxTextLines) + " líneas.")
	}
	return cleaned, nil
}

// publicError traduce un rechazo de logoraster (mensaje en español) a un 422 de la identidad.
func publicError(err error) error {
	if err == nil {
		return nil
	}
	var problem *logoraster.Error
	if errors.As(err, &problem) {
		return invalid(problem.Message)
	}
	return err
}
