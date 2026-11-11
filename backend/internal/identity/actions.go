package identity

import (
	"encoding/base64"

	"local/ad-el-pos/server/internal/logoraster"
)

// maxEncodedLogo es el largo máximo del base64 de un logotipo de 1 MiB: se rechaza antes de
// decodificar para no reservar memoria de más.
const maxEncodedLogo = (logoraster.MaxBytes + 2) / 3 * 4

// read atiende GET documents/identity: la identidad con las vistas previas del original y de cómo
// saldrá en papel de 58 y 80 mm.
func (handler *service) read() (any, error) {
	row, err := readIdentity(handler.ctx, handler.tx, false)
	if err != nil {
		return nil, err
	}
	return buildView(handler.ctx, handler.tx, row, true)
}

// result es la respuesta de una escritura: la identidad ya guardada, sin vistas previas.
func (handler *service) result() (any, error) {
	row, err := readIdentity(handler.ctx, handler.tx, false)
	if err != nil {
		return nil, err
	}
	return buildView(handler.ctx, handler.tx, row, false)
}

// checkRevision exige la revisión vigente: otra persona pudo guardar antes.
func (handler *service) checkRevision(revision int) error {
	if revision != handler.current.Revision {
		return revisionConflict()
	}
	return nil
}

// saveInput es el cuerpo de PUT documents/identity.
type saveInput struct {
	OperationID    string       `json:"operation_id"`
	Revision       int          `json:"revision"`
	AdditionalInfo string       `json:"additional_info"`
	HeaderText     string       `json:"header_text"`
	FooterText     string       `json:"footer_text"`
	Presentation   Presentation `json:"presentation"`
}

// save atiende PUT documents/identity: textos y presentación del logotipo. Si hay logotipo y la
// presentación cambia, se generan (o reutilizan) sus conversiones en la misma transacción. Guardar
// sin cambios no crea revisión, auditoría ni evento.
func (handler *service) save() (any, error) {
	var input saveInput
	if err := decodeStrict(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := handler.checkRevision(input.Revision); err != nil {
		return nil, err
	}
	next := *handler.current
	var err error
	if next.AdditionalInfo, err = normalizeText("La información adicional", input.AdditionalInfo); err != nil {
		return nil, err
	}
	if next.HeaderText, err = normalizeText("El texto de encabezado", input.HeaderText); err != nil {
		return nil, err
	}
	if next.FooterText, err = normalizeText("El texto de pie", input.FooterText); err != nil {
		return nil, err
	}
	if err := input.Presentation.validate(); err != nil {
		return nil, err
	}
	next.Presentation = input.Presentation
	unchanged := next.AdditionalInfo == handler.current.AdditionalInfo &&
		next.HeaderText == handler.current.HeaderText &&
		next.FooterText == handler.current.FooterText &&
		next.Presentation == handler.current.Presentation
	if unchanged {
		return handler.result()
	}
	if next.LogoID != nil && next.Presentation != handler.current.Presentation {
		loader := originalImage(handler.ctx, handler.tx, *next.LogoID)
		rasters, err := ensureRasters(handler.ctx, handler.tx, *next.LogoID, next.Presentation, loader)
		if err != nil {
			return nil, err
		}
		assignRasters(&next, rasters)
	}
	if err := saveIdentity(handler.ctx, handler.tx, &next, handler.request.Actor.UserID); err != nil {
		return nil, err
	}
	next.Revision = handler.current.Revision + 1
	err = handler.changed(
		"documents.identity_updated",
		"document_identity",
		nil,
		identitySummary(handler.current),
		identitySummary(&next),
	)
	if err != nil {
		return nil, err
	}
	return handler.result()
}

// uploadInput es el cuerpo de POST documents/identity/logo.
type uploadInput struct {
	OperationID string `json:"operation_id"`
	Revision    int    `json:"revision"`
	Content     string `json:"content_base64"`
}

// uploadLogo atiende POST documents/identity/logo: carga o reemplaza el logotipo. El archivo se
// valida con logoraster (PNG o JPEG, ≤ 1 MiB, ≤ 2048 × 2048, dimensiones antes de decodificar y
// decodificación completa), se guarda el original inmutable y sus conversiones de 58 y 80 mm con la
// presentación vigente. El logotipo anterior y sus conversiones se conservan para los documentos ya
// emitidos.
func (handler *service) uploadLogo() (any, error) {
	var input uploadInput
	if err := decodeStrict(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := handler.checkRevision(input.Revision); err != nil {
		return nil, err
	}
	if len(input.Content) > maxEncodedLogo {
		return nil, invalid("El logotipo debe ocupar como máximo 1 MiB.")
	}
	content, err := base64.StdEncoding.Strict().DecodeString(input.Content)
	if err != nil {
		return nil, invalid("El archivo del logotipo no se recibió completo. Vuelve a elegirlo.")
	}
	logo, err := logoraster.Validate(content)
	if err != nil {
		return nil, publicError(err)
	}
	logoID, _, err := storeLogo(handler.ctx, handler.tx, logo, content, handler.request.Actor.UserID)
	if err != nil {
		return nil, err
	}
	next := *handler.current
	rasters, err := ensureRasters(handler.ctx, handler.tx, logoID, next.Presentation, storedImage(logo.Image))
	if err != nil {
		return nil, err
	}
	next.LogoID = &logoID
	assignRasters(&next, rasters)
	if err := saveIdentity(handler.ctx, handler.tx, &next, handler.request.Actor.UserID); err != nil {
		return nil, err
	}
	after := logoSummary(logo, logoID, rasters)
	after["replaced_logo_id"] = handler.current.LogoID
	after["revision"] = handler.current.Revision + 1
	before := map[string]any{"logo_id": handler.current.LogoID, "revision": handler.current.Revision}
	if err := handler.changed("documents.logo_uploaded", "document_logo", &logoID, before, after); err != nil {
		return nil, err
	}
	return handler.result()
}

// revisionInput es el cuerpo de POST documents/identity/logo/remove.
type revisionInput struct {
	OperationID string `json:"operation_id"`
	Revision    int    `json:"revision"`
}

// removeLogo atiende POST documents/identity/logo/remove: desvincula el logotipo. El original y sus
// conversiones se conservan (los documentos ya emitidos los siguen usando) y la presentación queda
// guardada para el próximo logotipo.
func (handler *service) removeLogo() (any, error) {
	var input revisionInput
	if err := decodeStrict(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := handler.checkRevision(input.Revision); err != nil {
		return nil, err
	}
	if handler.current.LogoID == nil {
		return nil, noLogo()
	}
	removedID := *handler.current.LogoID
	next := *handler.current
	next.LogoID = nil
	next.Raster58ID = nil
	next.Raster80ID = nil
	if err := saveIdentity(handler.ctx, handler.tx, &next, handler.request.Actor.UserID); err != nil {
		return nil, err
	}
	before := map[string]any{"logo_id": removedID, "revision": handler.current.Revision}
	after := map[string]any{"logo_id": nil, "revision": handler.current.Revision + 1}
	if err := handler.changed("documents.logo_removed", "document_logo", &removedID, before, after); err != nil {
		return nil, err
	}
	return handler.result()
}

// previewInput es el cuerpo de POST documents/identity/logo/preview.
type previewInput struct {
	Presentation Presentation `json:"presentation"`
}

// PreviewResult son las conversiones del logotipo vigente con otra presentación, sin guardar.
type PreviewResult struct {
	Presentation Presentation `json:"presentation"`
	Rasters      []RasterView `json:"rasters"`
}

// previewLogo atiende POST documents/identity/logo/preview: muestra cómo saldría el logotipo con
// otra presentación antes de guardarla. No escribe nada: usa la conversión guardada si existe y, si
// no, convierte en memoria (la conversión solo se guarda al guardar la presentación).
func (handler *service) previewLogo() (any, error) {
	var input previewInput
	if err := decodeStrict(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.Presentation.validate(); err != nil {
		return nil, err
	}
	row, err := readIdentity(handler.ctx, handler.tx, false)
	if err != nil {
		return nil, err
	}
	if row.LogoID == nil {
		return nil, noLogo()
	}
	loader := originalImage(handler.ctx, handler.tx, *row.LogoID)
	result := PreviewResult{Presentation: input.Presentation, Rasters: []RasterView{}}
	for _, paperWidth := range paperWidths {
		stored, err := findRaster(handler.ctx, handler.tx, *row.LogoID, paperWidth, input.Presentation)
		if err != nil {
			return nil, err
		}
		if stored != nil {
			view, err := rasterView(stored, true)
			if err != nil {
				return nil, err
			}
			result.Rasters = append(result.Rasters, view)
			continue
		}
		picture, err := loader()
		if err != nil {
			return nil, err
		}
		raster, err := logoraster.Convert(picture, input.Presentation.options(paperWidth))
		if err != nil {
			return nil, publicError(err)
		}
		content, err := raster.PNG()
		if err != nil {
			return nil, err
		}
		result.Rasters = append(result.Rasters, RasterView{
			PaperWidth: paperWidth,
			WidthDots:  raster.Width(),
			HeightDots: raster.Height(),
			DataURI:    pngDataURI(content),
		})
	}
	return result, nil
}
