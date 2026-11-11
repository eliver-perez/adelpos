package designer

import (
	"context"
	"encoding/json"
	"strings"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/payments"
)

// Vista previa: la IR exacta de Go para una plantilla (guardada o en edición) con datos de muestra
// o con un documento real. No escribe nada. Los bloques protegidos siguen la regla de caja: solo
// con cash.expected.read y, en un documento real, fuera del arqueo de su turno.

// PreviewImage es la imagen de una línea de logotipo con su vista previa (fase B).
type PreviewImage struct {
	RasterID   string `json:"raster_id"`
	WidthDots  int    `json:"width_dots"`
	HeightDots int    `json:"height_dots"`
	DataURI    string `json:"data_uri"`
}

// PreviewLine es una línea de la IR tal como la muestra el editor.
type PreviewLine struct {
	Text    string        `json:"text"`
	Bold    bool          `json:"bold"`
	Size    string        `json:"size"`
	BlockID string        `json:"block_id"`
	Image   *PreviewImage `json:"image"`
}

// Preview es la respuesta de POST documents/preview.
type Preview struct {
	Kind             documents.Kind         `json:"kind"`
	PaperWidth       int                    `json:"paper_width"`
	Columns          int                    `json:"columns"`
	TextSize         string                 `json:"text_size"`
	Sample           bool                   `json:"sample"`
	IncludesExpected bool                   `json:"includes_expected"`
	Lines            []PreviewLine          `json:"lines"`
	Text             string                 `json:"text"`
	Warnings         []string               `json:"warnings"`
	EmptyBlocks      []documents.EmptyBlock `json:"empty_blocks"`
}

// RasterPreviewer es la parte opcional de la fuente de rasters que entrega la vista previa PNG de
// una imagen convertida como data URI (fase B, identity.Rasters).
type RasterPreviewer interface {
	DataURI(ctx context.Context, id string) (string, error)
}

type previewSource struct {
	Sample   bool    `json:"sample"`
	TicketID *string `json:"ticket_id"`
}

func (handler *service) preview(_ []string) (any, error) {
	var input struct {
		Kind       string          `json:"kind"`
		Layout     json.RawMessage `json:"layout"`
		PaperWidth int             `json:"paper_width"`
		Source     previewSource   `json:"source"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	kind, found := documents.ParseKind(input.Kind)
	if !found {
		return nil, invalid("El tipo de documento no existe.")
	}
	if input.PaperWidth != 58 && input.PaperWidth != 80 {
		return nil, invalid("El ancho de papel debe ser 58 u 80 mm.")
	}
	if input.Source.Sample == (input.Source.TicketID != nil) {
		return nil, invalid("Indica una vista previa con datos de muestra o con un documento real.")
	}
	template, err := documents.ParseTemplate(kind, input.Layout)
	if err != nil {
		return nil, templateInvalid(err)
	}
	source, err := handler.previewData(kind, input.Source)
	if err != nil {
		return nil, err
	}
	layout, err := documents.Render(kind, template, source.data, documents.RenderOptions{
		PaperWidth: input.PaperWidth, IncludeExpected: source.includeExpected,
	})
	if err != nil {
		return nil, err
	}
	// Los avisos del origen (p. ej. un documento de caja anterior a la 9B) van antes que los de los
	// bloques.
	layout.Warnings = append(source.notices, layout.Warnings...)
	return handler.previewResponse(kind, layout, input.Source.Sample, source.includeExpected), nil
}

// previewInputData son los datos con que se renderiza una vista previa: includeExpected decide el
// bloque protegido de los cortes y notices son avisos del origen para la vista previa.
type previewInputData struct {
	data            documents.Data
	includeExpected bool
	notices         []string
}

// previewData obtiene los datos de muestra (con la identidad real) o los de un documento real con
// los permisos de su origen.
func (handler *service) previewData(kind documents.Kind, source previewSource) (previewInputData, error) {
	actor := handler.request.Actor
	if source.Sample {
		data, err := handler.sampleData(kind)
		if err != nil {
			return previewInputData{}, err
		}
		// Los datos de muestra son ficticios; aun así, el bloque protegido sigue la regla del permiso.
		return previewInputData{data: data, includeExpected: actor.Can("cash.expected.read"), notices: []string{}}, nil
	}
	if err := actor.Require(documents.PermissionPreviewReal); err != nil {
		return previewInputData{}, err
	}
	ticketID := *source.TicketID
	if err := validID(ticketID); err != nil {
		return previewInputData{}, err
	}
	var found documents.Kind
	result := previewInputData{notices: []string{}}
	switch kind.Family() {
	case documents.FamilyKitchen:
		kitchenData, ticketKind, err := kitchen.PreviewSource(handler.ctx, handler.tx, handler.kitchenActor(), ticketID)
		if err != nil {
			return result, err
		}
		result.data.Kitchen, found = kitchenData, ticketKind
	case documents.FamilyCommercial:
		commercial, ticketKind, err := payments.CommercialPreviewSource(handler.ctx, handler.tx, handler.paymentsActor(), ticketID)
		if err != nil {
			return result, err
		}
		result.data.Commercial, found = commercial, ticketKind
	case documents.FamilyCash:
		// Con los datos que guardó el documento (los mismos que imprime «Imprimir con la plantilla
		// vigente»); solo un documento anterior a la 9B se lee de su corte u operación, con aviso.
		cash, err := payments.CashPreviewSource(handler.ctx, handler.tx, handler.paymentsActor(), ticketID)
		if err != nil {
			return result, err
		}
		result.data.Cash, found, result.includeExpected = cash.Data, cash.Kind, cash.IncludeExpected
		if cash.Notice != "" {
			result.notices = append(result.notices, cash.Notice)
		}
	}
	// Como al imprimir con la plantilla vigente, la vista previa con un documento real usa sus
	// datos históricos y la identidad vigente del establecimiento.
	identity, err := documents.LoadIdentity(handler.ctx, handler.tx)
	if err != nil {
		return result, err
	}
	switch {
	case result.data.Kitchen != nil:
		result.data.Kitchen.Identity = identity
	case result.data.Commercial != nil:
		result.data.Commercial.Identity = identity
	case result.data.Cash != nil:
		result.data.Cash.Identity = identity
	}
	if found != kind {
		return previewInputData{}, conflict("SOURCE_KIND_MISMATCH", "El documento elegido es de otro tipo; elige uno de "+strings.ToLower(kind.Name())+".")
	}
	return result, nil
}

// sampleData son los datos de muestra de un tipo con la identidad real del establecimiento. En
// cocina aplican además la configuración vigente de Cocina con la regla de una comanda real (sin
// «Mostrar precios», sin importes), para que la vista previa y la muestra impresa anticipen el papel.
func (handler *service) sampleData(kind documents.Kind) (documents.Data, error) {
	identity, err := documents.LoadIdentity(handler.ctx, handler.tx)
	if err != nil {
		return documents.Data{}, err
	}
	data := documents.SampleData(kind, identity)
	if data.Kitchen != nil {
		settings, err := kitchen.SampleSettings(handler.ctx, handler.tx)
		if err != nil {
			return documents.Data{}, err
		}
		data.Kitchen.UseSettings(settings)
	}
	return data, nil
}

func (handler *service) kitchenActor() kitchen.Actor {
	actor := handler.request.Actor
	return kitchen.Actor{UserID: actor.UserID, DeviceID: actor.DeviceID, Permissions: actor.Permissions}
}

func (handler *service) paymentsActor() payments.Actor {
	actor := handler.request.Actor
	return payments.Actor{UserID: actor.UserID, DeviceID: actor.DeviceID, Permissions: actor.Permissions}
}

// previewResponse expresa la IR para el editor, con la vista previa de las imágenes si existe.
func (handler *service) previewResponse(kind documents.Kind, layout *documents.Layout, sample, includeExpected bool) Preview {
	response := Preview{
		Kind: kind, PaperWidth: layout.PaperWidth, Columns: layout.Columns, TextSize: layout.TextSize, Sample: sample,
		IncludesExpected: includeExpected, Lines: make([]PreviewLine, 0, len(layout.Lines)), Text: layout.Text(),
		Warnings: layout.Warnings, EmptyBlocks: layout.EmptyBlocks,
	}
	if response.Warnings == nil {
		response.Warnings = []string{}
	}
	if response.EmptyBlocks == nil {
		response.EmptyBlocks = []documents.EmptyBlock{}
	}
	previewer, _ := handler.request.Rasters.(RasterPreviewer)
	for _, line := range layout.Lines {
		item := PreviewLine{Text: line.Text, Bold: line.Bold, Size: line.Size, BlockID: line.BlockID}
		if line.Image != nil {
			item.Image = &PreviewImage{RasterID: line.Image.RasterID, WidthDots: line.Image.WidthDots, HeightDots: line.Image.HeightDots}
			if previewer != nil {
				if uri, err := previewer.DataURI(handler.ctx, line.Image.RasterID); err == nil {
					item.Image.DataURI = uri
				}
			}
		}
		response.Lines = append(response.Lines, item)
	}
	return response
}

// recent lista los documentos reales recientes del tipo que el usuario puede consultar en su origen.
func (handler *service) recent(_ []string) (any, error) {
	kind, found := documents.ParseKind(handler.request.Query.Get("kind"))
	if !found {
		return nil, invalid("Indica un tipo de documento válido.")
	}
	switch kind {
	case documents.KindKitchenOrder:
		return kitchen.RecentDocuments(handler.ctx, handler.tx, handler.kitchenActor(), "normal")
	case documents.KindKitchenModification:
		return kitchen.RecentDocuments(handler.ctx, handler.tx, handler.kitchenActor(), "modification")
	case documents.KindKitchenCancellation:
		return kitchen.RecentDocuments(handler.ctx, handler.tx, handler.kitchenActor(), "cancellation")
	case documents.KindReceipt, documents.KindPrecheck:
		return payments.RecentCommercialDocuments(handler.ctx, handler.tx, handler.paymentsActor(), string(kind))
	}
	documentKind := map[documents.Kind]string{
		documents.KindCashIncome: "income", documents.KindCashWithdrawal: "withdrawal", documents.KindCashExpense: "expense",
		documents.KindCashPartialCut: "partial_cut", documents.KindCashFinalCut: "final_cut",
	}[kind]
	return payments.RecentCashDocuments(handler.ctx, handler.tx, handler.paymentsActor(), documentKind)
}
