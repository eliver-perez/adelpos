package logoraster

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	"image/png"
)

const (
	// MaxBytes es el tamaño máximo del archivo original del logotipo (1 MiB).
	MaxBytes = 1 << 20
	// MaxDimension es el ancho y el alto máximos del original, en píxeles.
	MaxDimension = 2048

	FormatPNG  = "png"
	FormatJPEG = "jpeg"
)

// Firmas de los únicos formatos aceptados. La de JPEG es el marcador SOI (FF D8) seguido del inicio
// de cualquier otro marcador (FF).
var (
	pngSignature  = []byte("\x89PNG\r\n\x1a\n")
	jpegSignature = []byte{0xFF, 0xD8, 0xFF}
)

// Error es un rechazo de validación o de opciones con un mensaje en español listo para mostrarse.
type Error struct {
	Message string
}

func (problem *Error) Error() string {
	return problem.Message
}

func invalid(message string) error {
	return &Error{Message: message}
}

// Logo es un logotipo validado: el formato y las dimensiones del original, su tamaño, su huella
// SHA-256 en hexadecimal y la imagen ya decodificada para convertirla.
type Logo struct {
	Format    string
	MediaType string
	Width     int
	Height    int
	Size      int
	SHA256    string
	Image     image.Image
}

// Validate comprueba que content sea un PNG o JPEG íntegro dentro de los límites y lo decodifica.
// El orden importa: primero tamaño y firma, luego las dimensiones del encabezado (sin decodificar
// los píxeles, para que un archivo que declara dimensiones enormes no reserve memoria) y al final
// la decodificación completa, que detecta archivos truncados o dañados.
func Validate(content []byte) (*Logo, error) {
	if len(content) == 0 {
		return nil, invalid("El archivo del logotipo está vacío.")
	}
	if len(content) > MaxBytes {
		return nil, invalid("El logotipo debe ocupar como máximo 1 MiB.")
	}
	format := detectFormat(content)
	if format == "" {
		return nil, invalid("El logotipo debe ser una imagen PNG o JPEG; no se aceptan SVG, GIF, WebP ni otros formatos.")
	}
	configuration, decodedFormat, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return nil, invalid("El logotipo está dañado o incompleto.")
	}
	// La firma y el decodificador deben coincidir: un archivo que empieza como PNG pero que otro
	// decodificador registrado reconoce de otra forma no se acepta.
	if decodedFormat != format {
		return nil, invalid("El logotipo debe ser una imagen PNG o JPEG; no se aceptan SVG, GIF, WebP ni otros formatos.")
	}
	if err := validateDimensions(configuration.Width, configuration.Height); err != nil {
		return nil, err
	}
	decoded, err := decodeFull(content, format)
	if err != nil {
		return nil, invalid("El logotipo está dañado o incompleto.")
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != configuration.Width || bounds.Dy() != configuration.Height {
		return nil, invalid("El logotipo está dañado o incompleto.")
	}
	digest := sha256.Sum256(content)
	logo := &Logo{
		Format:    format,
		MediaType: "image/" + format,
		Width:     configuration.Width,
		Height:    configuration.Height,
		Size:      len(content),
		SHA256:    hex.EncodeToString(digest[:]),
		Image:     decoded,
	}
	return logo, nil
}

// detectFormat reconoce el formato solo por la firma del inicio del archivo.
func detectFormat(content []byte) string {
	if bytes.HasPrefix(content, pngSignature) {
		return FormatPNG
	}
	if bytes.HasPrefix(content, jpegSignature) {
		return FormatJPEG
	}
	return ""
}

// validateDimensions aplica los límites de 1 × 1 a 2048 × 2048 píxeles.
func validateDimensions(width, height int) error {
	validWidth := width >= 1 && width <= MaxDimension
	validHeight := height >= 1 && height <= MaxDimension
	if !validWidth || !validHeight {
		return invalid("El logotipo debe medir entre 1 × 1 y 2048 × 2048 píxeles.")
	}
	return nil
}

// decodeFull decodifica todos los píxeles con el decodificador del formato detectado.
func decodeFull(content []byte, format string) (image.Image, error) {
	if format == FormatPNG {
		return png.Decode(bytes.NewReader(content))
	}
	return jpeg.Decode(bytes.NewReader(content))
}
