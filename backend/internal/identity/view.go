package identity

import (
	"context"
	"encoding/base64"

	"local/ad-el-pos/server/internal/logoraster"
)

// buildView arma la identidad completa. withPreviews agrega el original y las conversiones como
// data URI (solo en GET: las escrituras devuelven la vista sin imágenes y se guardan así en
// restaurant_operations).
func buildView(ctx context.Context, db querier, row *identityRow, withPreviews bool) (*View, error) {
	establishment, err := readEstablishment(ctx, db)
	if err != nil {
		return nil, err
	}
	view := &View{
		Revision:       row.Revision,
		Establishment:  establishment,
		AdditionalInfo: row.AdditionalInfo,
		HeaderText:     row.HeaderText,
		FooterText:     row.FooterText,
		Presentation:   row.Presentation,
		UpdatedAt:      row.UpdatedAt,
		UpdatedBy:      row.UpdatedBy,
		Limits:         currentLimits(),
		Warnings:       []string{},
	}
	if row.LogoID == nil {
		return view, nil
	}
	logo, err := buildLogoView(ctx, db, row, withPreviews)
	if err != nil {
		return nil, err
	}
	view.Logo = logo
	if len(logo.Rasters) != len(paperWidths) {
		view.Warnings = append(view.Warnings,
			"Falta una conversión del logotipo: vuelve a guardar su presentación para generarla.")
	}
	return view, nil
}

// buildLogoView describe el logotipo vigente con sus conversiones de 58 y 80 mm.
func buildLogoView(ctx context.Context, db querier, row *identityRow, withPreviews bool) (*LogoView, error) {
	logo, content, err := readLogo(ctx, db, *row.LogoID, withPreviews)
	if err != nil {
		return nil, err
	}
	view := &LogoView{
		ID:         logo.ID,
		MediaType:  logo.MediaType,
		Width:      logo.Width,
		Height:     logo.Height,
		SizeBytes:  logo.SizeBytes,
		SHA256:     logo.SHA256,
		UploadedAt: logo.UploadedAt,
		UploadedBy: logo.UploadedBy,
		Rasters:    []RasterView{},
	}
	if withPreviews {
		view.DataURI = "data:" + logo.MediaType + ";base64," + base64.StdEncoding.EncodeToString(content)
	}
	for _, id := range []*string{row.Raster58ID, row.Raster80ID} {
		if id == nil {
			continue
		}
		raster, err := readRaster(ctx, db, *id)
		if err != nil {
			return nil, err
		}
		if raster == nil {
			continue
		}
		rasterInfo, err := rasterView(raster, withPreviews)
		if err != nil {
			return nil, err
		}
		view.Rasters = append(view.Rasters, rasterInfo)
	}
	return view, nil
}

// presentationSummary es la presentación para la auditoría.
func presentationSummary(presentation Presentation) map[string]any {
	return map[string]any{
		"width_percent": presentation.WidthPercent,
		"align":         presentation.Align,
		"method":        presentation.Method,
		"threshold":     presentation.Threshold,
	}
}

// identitySummary es lo que se audita de la identidad: textos, presentación, logotipo y revisión.
// Nunca lleva bytes ni base64 del logotipo.
func identitySummary(row *identityRow) map[string]any {
	return map[string]any{
		"additional_info": row.AdditionalInfo,
		"header_text":     row.HeaderText,
		"footer_text":     row.FooterText,
		"presentation":    presentationSummary(row.Presentation),
		"logo_id":         row.LogoID,
		"revision":        row.Revision,
	}
}

// logoSummary es lo que se audita de un logotipo: su descripción y sus conversiones, sin bytes.
func logoSummary(logo *logoraster.Logo, logoID string, rasters []*rasterRow) map[string]any {
	conversions := []map[string]any{}
	for _, raster := range rasters {
		conversions = append(conversions, map[string]any{
			"paper_width": raster.PaperWidth,
			"raster_id":   raster.ID,
			"width_dots":  raster.WidthDots,
			"height_dots": raster.HeightDots,
		})
	}
	return map[string]any{
		"logo_id":    logoID,
		"media_type": logo.MediaType,
		"width":      logo.Width,
		"height":     logo.Height,
		"size_bytes": logo.Size,
		"sha256":     logo.SHA256,
		"rasters":    conversions,
	}
}
