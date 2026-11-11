package kitchen

import (
	"time"

	"local/ad-el-pos/server/internal/documents"
)

// Etapa 9B: las comandas, avisos de cocina y la prueba técnica de impresora se renderizan con el
// renderizador único de internal/documents. Cada comanda emitida guarda su versión de plantilla
// (Document.TemplateVersionID): reimprimirla desde la cola vuelve a renderizar sus datos con esa
// misma versión, de modo que la copia es idéntica aunque la plantilla haya cambiado después. Una
// comanda anterior a la 9B (sin versión) usa la versión 1 del sistema, idéntica al formato
// anterior; la prueba técnica de impresora también.

// cleanText elimina controles e invisibles de un texto de usuario (conserva saltos de línea).
func cleanText(value string) string {
	return documents.CleanText(value)
}

// documentKind es el tipo de documento de un ticket de cocina; la prueba técnica usa la comanda.
func documentKind(ticketKind string) documents.Kind {
	kind, found := documents.KitchenKind(ticketKind)
	if !found {
		return documents.KindKitchenOrder
	}
	return kind
}

// DocumentData convierte una comanda guardada en los datos tipados del renderizador. La fecha se
// expresa en la zona horaria copiada en el contexto (UTC si no es válida, como antes) y el nombre
// del establecimiento es el del contexto copiado al liberar.
func DocumentData(document Document) documents.KitchenData {
	location, err := time.LoadLocation(document.Context.Timezone)
	if err != nil {
		location = time.UTC
	}
	local := document.CreatedAt.In(location)
	identity := documents.Identity{}
	if document.Identity != nil {
		identity = *document.Identity
	}
	identity.Name = document.Context.Establishment
	data := documents.KitchenData{
		TicketKind:    document.Kind,
		DocumentID:    document.ID,
		LocalDateTime: local.Format("02/01/2006 15:04:05"),
		LocalDate:     local.Format("02/01/2006"),
		LocalTime:     local.Format("15:04"),
		Folio:         document.Context.Folio,
		AccountName:   document.Context.AccountName,
		Table:         document.Context.Table,
		Area:          document.Context.Area,
		Mode:          document.Context.Mode,
		Customer:      document.Context.Customer,
		Origin:        document.Context.Origin,
		Employee:      document.Context.Employee,
		OrderID:       document.Context.OrderID,
		Settings: documents.KitchenSettings{
			ShowPrices:        document.Settings.ShowPrices,
			ShowNotes:         document.Settings.ShowNotes,
			ShowEstablishment: document.Settings.ShowEstablishment,
			TextSize:          document.Settings.TextSize,
			LineSpacing:       document.Settings.LineSpacing,
			QuantityStyle:     document.Settings.QuantityStyle,
		},
		Sections: make([]documents.KitchenSection, 0, len(document.Sections)),
		Identity: identity,
	}
	for _, section := range document.Sections {
		converted := documents.KitchenSection{StationName: section.Station.Name, Items: make([]documents.KitchenItem, 0, len(section.Items))}
		for _, item := range section.Items {
			converted.Items = append(converted.Items, kitchenItem(item))
		}
		data.Sections = append(data.Sections, converted)
	}
	return data
}

func kitchenItem(item Instructions) documents.KitchenItem {
	converted := documents.KitchenItem{
		ItemID: item.ItemID, Name: item.Name, Quantity: item.Quantity, Price: item.Price, Reason: item.Reason, Notes: item.Notes,
		Parts: make([]documents.KitchenPart, 0, len(item.Parts)),
	}
	if item.Before != nil {
		before := kitchenItem(*item.Before)
		converted.Before = &before
	}
	for _, part := range item.Parts {
		converted.Parts = append(converted.Parts, kitchenPart(part))
	}
	return converted
}

func kitchenPart(part Part) documents.KitchenPart {
	converted := documents.KitchenPart{
		Path: part.Path, Name: part.Name, Variant: part.Variant, Quantity: part.Quantity, Notes: part.Notes,
		Ingredients: make([]documents.KitchenIngredient, 0, len(part.Ingredients)),
		Groups:      make([]documents.KitchenGroup, 0, len(part.Groups)),
	}
	for _, ingredient := range part.Ingredients {
		converted.Ingredients = append(converted.Ingredients, documents.KitchenIngredient{
			IngredientID: ingredient.IngredientID, Name: ingredient.Name, Included: ingredient.Included, Requested: ingredient.Requested,
		})
	}
	for _, group := range part.Groups {
		options := make([]documents.KitchenOption, 0, len(group.Options))
		for _, option := range group.Options {
			options = append(options, documents.KitchenOption{Name: option.Name, Requested: option.Requested})
		}
		converted.Groups = append(converted.Groups, documents.KitchenGroup{Name: group.Name, Options: options})
	}
	for _, note := range part.QuickNotes {
		converted.QuickNotes = append(converted.QuickNotes, documents.KitchenQuickNote{
			Text: note.Text, Effect: note.Effect, IngredientID: note.IngredientID,
		})
	}
	return converted
}

// RenderDocument renderiza una comanda con una versión de plantilla al ancho de papel indicado.
func RenderDocument(version *documents.Version, document Document, paperWidth int) (*documents.Layout, error) {
	data := DocumentData(document)
	return documents.Render(version.Kind, version.Template, documents.Data{Kitchen: &data}, documents.RenderOptions{PaperWidth: paperWidth})
}

// renderStored renderiza una comanda guardada con la versión de plantilla que guardó al emitirse.
func (handler *service) renderStored(document Document, paperWidth int) (*documents.Layout, error) {
	version, err := documents.VersionByID(handler.ctx, handler.tx, documentKind(document.Kind), document.TemplateVersionID)
	if err != nil {
		return nil, err
	}
	return RenderDocument(version, document, paperWidth)
}
