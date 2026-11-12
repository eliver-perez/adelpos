package logoraster

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

	// MaxJPEGScans es el número máximo de barridos (marcadores SOS) de un JPEG. Decodificar cada
	// barrido de un JPEG progresivo recorre todos los bloques de la imagen, y el decodificador de Go
	// no limita su número: un archivo válido de 1 MiB con unos 61 000 barridos vacíos tardaba más de
	// un minuto (revisión de la etapa 9B). Los codificadores comunes usan alrededor de 10 barridos en
	// un JPEG progresivo y 1 a 3 en uno secuencial; 100 deja un margen amplio (como -maxscans de
	// libjpeg-turbo) y acota la validación de un logotipo de 2048 × 2048 a una fracción de segundo.
	MaxJPEGScans = 100
	// MaxJPEGSegments es el número máximo de segmentos con longitud (tablas, metadatos, encabezados y
	// barridos) de un JPEG; uno común tiene menos de 50.
	MaxJPEGSegments = 1024
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
// El orden importa: primero tamaño y firma; en un JPEG, el número de barridos y segmentos (un
// recorrido de los marcadores sin decodificar, para que un archivo con miles de barridos no ocupe
// el procesador durante minutos); luego las dimensiones del encabezado (sin decodificar los píxeles,
// para que un archivo que declara dimensiones enormes no reserve memoria) y al final la
// decodificación completa, que detecta archivos truncados o dañados.
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
	if format == FormatJPEG {
		if err := jpegMarkersProblem(content); err != nil {
			return nil, err
		}
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

// Marcadores JPEG que importan al recorrido: SOS (inicio de barrido), EOI (fin de imagen) y los
// reinicios RST0–RST7, que van dentro de los datos comprimidos o sueltos y no llevan longitud.
const (
	jpegMarkerSOS  = 0xDA
	jpegMarkerEOI  = 0xD9
	jpegMarkerRST0 = 0xD0
	jpegMarkerRST7 = 0xD7
)

// Errores neutros del recorrido de marcadores JPEG (CheckJPEGMarkers): cada módulo que lo usa (el
// logotipo de la identidad, las imágenes del catálogo) los traduce a su propio mensaje.
var (
	ErrJPEGTooManyScans    = errors.New("el JPEG excede el número máximo de barridos")
	ErrJPEGTooManySegments = errors.New("el JPEG excede el número máximo de segmentos")
)

// jpegMarkersProblem traduce el recorrido de marcadores a los mensajes del logotipo.
func jpegMarkersProblem(content []byte) error {
	err := CheckJPEGMarkers(content)
	switch {
	case errors.Is(err, ErrJPEGTooManySegments):
		return invalid("El logotipo JPEG tiene demasiados segmentos. Guárdalo de nuevo como PNG o como JPEG estándar.")
	case errors.Is(err, ErrJPEGTooManyScans):
		return invalid("El logotipo JPEG tiene demasiados barridos progresivos. Guárdalo de nuevo como PNG o como JPEG estándar.")
	}
	return nil
}

// CheckJPEGMarkers recorre los marcadores de un JPEG como lo hace el decodificador (sin decodificar
// nada ni reservar memoria) y devuelve ErrJPEGTooManyScans si tiene más de MaxJPEGScans barridos o
// ErrJPEGTooManySegments si tiene más de MaxJPEGSegments segmentos. Debe llamarse antes de
// decodificar (también antes de image.DecodeConfig). Un contenido que no empieza con la firma JPEG
// no se recorre, y un archivo mal formado o truncado no se rechaza aquí: lo detecta la
// decodificación.
func CheckJPEGMarkers(content []byte) error {
	if !bytes.HasPrefix(content, jpegSignature) {
		return nil
	}
	scans := 0
	segments := 0
	index := len(jpegSignature) - 1
	for {
		marker, next, found := nextJPEGMarker(content, index)
		if !found || marker == jpegMarkerEOI {
			return nil
		}
		index = next
		if marker >= jpegMarkerRST0 && marker <= jpegMarkerRST7 {
			continue
		}
		if index+2 > len(content) {
			return nil
		}
		// La longitud del segmento incluye sus dos bytes.
		length := int(content[index])<<8 | int(content[index+1])
		if length < 2 {
			return nil
		}
		segments++
		if segments > MaxJPEGSegments {
			return ErrJPEGTooManySegments
		}
		index += length
		if marker == jpegMarkerSOS {
			scans++
			if scans > MaxJPEGScans {
				return ErrJPEGTooManyScans
			}
			index = skipJPEGEntropyData(content, index)
		}
	}
}

// nextJPEGMarker devuelve el siguiente marcador desde index y la posición que lo sigue. Como el
// decodificador, ignora los bytes sueltos que no son 0xFF, los 0xFF de relleno y los pares FF 00.
func nextJPEGMarker(content []byte, index int) (byte, int, bool) {
	for index+1 < len(content) {
		if content[index] != 0xFF {
			index++
			continue
		}
		marker := content[index+1]
		switch marker {
		case 0xFF:
			index++
		case 0x00:
			index += 2
		default:
			return marker, index + 2, true
		}
	}
	return 0, index, false
}

// skipJPEGEntropyData avanza sobre los datos comprimidos de un barrido hasta el siguiente marcador
// que no sea un byte escapado (FF 00), un reinicio (RST0–RST7) ni un relleno (FF FF).
func skipJPEGEntropyData(content []byte, index int) int {
	for index+1 < len(content) {
		if content[index] != 0xFF {
			index++
			continue
		}
		marker := content[index+1]
		if marker == 0x00 || (marker >= jpegMarkerRST0 && marker <= jpegMarkerRST7) {
			index += 2
			continue
		}
		if marker == 0xFF {
			index++
			continue
		}
		return index
	}
	return len(content)
}
