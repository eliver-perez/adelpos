package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/logoraster"
)

// Binary es una respuesta binaria del catálogo: el contenido de una imagen con su etiqueta de caché (etapa 9G,
// §5.3). administration la escribe tal cual, sin JSON. NotModified indica que el cliente ya tiene esa versión
// (If-None-Match coincide) y se responde 304 sin contenido.
type Binary struct {
	ContentType  string
	ETag         string
	CacheControl string
	Content      []byte
	NotModified  bool
}

// imageContent atiende GET catalog/images/{id}/content: la imagen tal cual, para que los clientes la guarden en su
// caché en lugar de recibirla en base64 dentro del JSON. La identidad de una imagen nunca cambia de contenido (una
// imagen nueva tiene otro id), así que la etiqueta es su checksum y la caché puede ser larga.
func imageContent(ctx context.Context, tx pgx.Tx, request Request, id string) (any, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}
	var checksum, mime string
	if err := tx.QueryRow(ctx, "SELECT checksum,media_type FROM app.catalog_images WHERE id=$1", id).Scan(&checksum, &mime); err != nil {
		return nil, err
	}
	result := &Binary{
		ContentType:  mime,
		ETag:         `"` + checksum + `"`,
		CacheControl: "private, max-age=31536000, immutable",
	}
	if etagMatches(request.IfNoneMatch, result.ETag) {
		result.NotModified = true
		return result, nil
	}
	if err := tx.QueryRow(ctx, "SELECT content FROM app.catalog_images WHERE id=$1", id).Scan(&result.Content); err != nil {
		return nil, err
	}
	return result, nil
}

// etagMatches compara If-None-Match (una lista separada por comas, «*» o etiquetas débiles W/"…") con la etiqueta.
func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}

func images(ctx context.Context, tx pgx.Tx, request Request, parts []string, audit Audit) (any, error) {
	if request.Method == "GET" && len(parts) == 3 && parts[2] == "content" {
		return imageContent(ctx, tx, request, parts[1])
	}
	if request.Method == "GET" && len(parts) == 2 {
		if err := validateID(parts[1]); err != nil {
			return nil, err
		}
		var content []byte
		var mime string
		var width, height int
		err := tx.QueryRow(ctx, "SELECT content,media_type,width,height FROM app.catalog_images WHERE id=$1", parts[1]).Scan(&content, &mime, &width, &height)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"id":       parts[1],
			"data_uri": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(content),
			"width":    width,
			"height":   height,
		}, nil
	}
	if err := request.Actor.Require("catalog.images.manage"); err != nil {
		return nil, err
	}
	if request.Method != "POST" || len(parts) != 1 {
		return nil, invalid("Carga una imagen mediante POST.")
	}
	var input struct {
		Content string `json:"content_base64"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	content, err := base64.StdEncoding.Strict().DecodeString(input.Content)
	if err != nil || len(content) == 0 || len(content) > 1024*1024 {
		return nil, invalid("La imagen debe ser PNG o JPEG y ocupar como máximo 1 MiB.")
	}
	// Etapa 9C (§5.2): el decodificador de Go no limita los barridos de un JPEG progresivo y cada uno
	// recorre todos los bloques de la imagen (un archivo de 1 MiB con miles de barridos vacíos tardaba
	// más de un minuto). Antes de leer el encabezado o decodificar se recorren sus marcadores con la
	// misma validación del logotipo (como máximo 100 barridos y 1024 segmentos).
	if err := checkImageMarkers(content); err != nil {
		return nil, err
	}
	configuration, format, err := image.DecodeConfig(bytes.NewReader(content))
	validFormat := format == "png" || format == "jpeg"
	validWidth := configuration.Width >= 1 && configuration.Width <= 2048
	validHeight := configuration.Height >= 1 && configuration.Height <= 2048
	if err != nil || !validFormat || !validWidth || !validHeight {
		return nil, invalid("La imagen debe ser PNG/JPEG válido de hasta 2048 × 2048 píxeles.")
	}
	// Decodificar completa impide aceptar archivos truncados con una cabecera aparentemente válida.
	if _, _, err := image.Decode(bytes.NewReader(content)); err != nil {
		return nil, invalid("El archivo de imagen está dañado o incompleto.")
	}
	digest := sha256.Sum256(content)
	checksum := fmt.Sprintf("%x", digest)
	mime := "image/" + format
	id := newID()
	err = tx.QueryRow(

		ctx,
		`INSERT INTO app.catalog_images(id,content,media_type,width,height,checksum)
        VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT(checksum) DO UPDATE SET checksum=excluded.checksum RETURNING id`,
		id,
		content,
		mime,
		configuration.Width,
		configuration.Height,
		checksum,
	).Scan(&id)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"id":         id,
		"media_type": mime,
		"width":      configuration.Width,
		"height":     configuration.Height,
		"size":       len(content),
	}
	// La auditoría conserva metadatos, nunca los bytes ni el base64 de las fotografías.
	return result, audit(

		ctx,
		tx,
		"catalog.image.uploaded",
		"images",
		&id,
		nil,
		result,
	)
}

// checkImageMarkers traduce el recorrido de marcadores JPEG de logoraster a los mensajes del
// catálogo. Un PNG no se recorre.
func checkImageMarkers(content []byte) error {
	err := logoraster.CheckJPEGMarkers(content)
	switch {
	case errors.Is(err, logoraster.ErrJPEGTooManyScans):
		return invalid(fmt.Sprintf("La imagen JPEG tiene demasiados barridos progresivos (máximo %d). "+
			"Guárdala de nuevo como PNG o como JPEG estándar.", logoraster.MaxJPEGScans))
	case errors.Is(err, logoraster.ErrJPEGTooManySegments):
		return invalid(fmt.Sprintf("La imagen JPEG tiene demasiados segmentos (máximo %d). "+
			"Guárdala de nuevo como PNG o como JPEG estándar.", logoraster.MaxJPEGSegments))
	}
	return nil
}
