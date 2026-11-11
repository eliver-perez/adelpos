package identity

import (
	"context"
	"encoding/base64"
	"image"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/logoraster"
)

// Conversiones del logotipo a puntos para papel de 58 y 80 mm. Se generan al cargar el logotipo o
// al cambiar su presentación (nunca al imprimir) y se guardan inmutables; una combinación que ya
// existe se reutiliza sin volver a convertir.

// imageLoader entrega la imagen decodificada del original solo si hace falta convertir.
type imageLoader func() (image.Image, error)

// storedImage reutiliza la imagen ya decodificada al validar una carga.
func storedImage(picture image.Image) imageLoader {
	return func() (image.Image, error) {
		return picture, nil
	}
}

// originalImage decodifica el original guardado (ya validado al cargarlo) una sola vez.
func originalImage(ctx context.Context, db querier, logoID string) imageLoader {
	var picture image.Image
	return func() (image.Image, error) {
		if picture != nil {
			return picture, nil
		}
		_, content, err := readLogo(ctx, db, logoID, true)
		if err != nil {
			return nil, err
		}
		logo, err := logoraster.Validate(content)
		if err != nil {
			return nil, publicError(err)
		}
		picture = logo.Image
		return picture, nil
	}
}

// ensureRasters devuelve las conversiones del logotipo con la presentación para cada ancho de papel,
// guardando las que falten, en el orden de paperWidths.
func ensureRasters(ctx context.Context, tx pgx.Tx, logoID string, presentation Presentation, load imageLoader) ([]*rasterRow, error) {
	rows := []*rasterRow{}
	for _, paperWidth := range paperWidths {
		stored, err := findRaster(ctx, tx, logoID, paperWidth, presentation)
		if err != nil {
			return nil, err
		}
		if stored == nil {
			picture, err := load()
			if err != nil {
				return nil, err
			}
			raster, err := logoraster.Convert(picture, presentation.options(paperWidth))
			if err != nil {
				return nil, publicError(err)
			}
			stored, err = storeRaster(ctx, tx, logoID, paperWidth, presentation, raster)
			if err != nil {
				return nil, err
			}
		}
		rows = append(rows, stored)
	}
	return rows, nil
}

// assignRasters deja en la fila de la identidad las conversiones vigentes (58 y 80 mm).
func assignRasters(row *identityRow, rasters []*rasterRow) {
	for _, raster := range rasters {
		id := raster.ID
		switch raster.PaperWidth {
		case logoraster.Paper58:
			row.Raster58ID = &id
		case logoraster.Paper80:
			row.Raster80ID = &id
		}
	}
}

// rasterPNG reconstruye una conversión guardada y la codifica como PNG de 1 bit: exactamente los
// puntos que se imprimen, incluido el margen blanco de la alineación.
func rasterPNG(widthDots, heightDots int, bits []byte) ([]byte, error) {
	raster, err := logoraster.NewRaster(widthDots, heightDots, bits)
	if err != nil {
		return nil, err
	}
	return raster.PNG()
}

// pngDataURI es la vista previa de una conversión para mostrarla en un <img>.
func pngDataURI(content []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(content)
}

// rasterView describe una conversión guardada, con su vista previa si withPreview.
func rasterView(row *rasterRow, withPreview bool) (RasterView, error) {
	view := RasterView{
		PaperWidth: row.PaperWidth,
		RasterID:   row.ID,
		WidthDots:  row.WidthDots,
		HeightDots: row.HeightDots,
	}
	if !withPreview {
		return view, nil
	}
	content, err := rasterPNG(row.WidthDots, row.HeightDots, row.Bits)
	if err != nil {
		return view, err
	}
	view.DataURI = pngDataURI(content)
	return view, nil
}
