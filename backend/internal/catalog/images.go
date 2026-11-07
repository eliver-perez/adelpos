package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/jackc/pgx/v5"
)

func images(ctx context.Context, tx pgx.Tx, request Request, parts []string, audit Audit) (any, error) {
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
