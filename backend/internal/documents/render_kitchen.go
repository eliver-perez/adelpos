package documents

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// Bloques de las comandas y avisos de cocina. La lógica es la de kitchen.RenderText de las etapas
// 5 a 8 (renderParts y quickNoteCoverage incluidos), con las opciones heredables de la lista.

func renderKitchenBlock(block Block, data *KitchenData, w *writer) {
	switch block.Type {
	case BlockKitchenLocation:
		options, _ := block.parsed.(*KitchenLocationOptions)
		showCustomer := options == nil || options.ShowCustomer
		if data.Mode == "takeaway" {
			w.text("PARA LLEVAR · " + data.Origin)
			if showCustomer && data.Customer != "" {
				w.text(data.Customer)
			}
			return
		}
		w.text(data.Area + " · " + data.Table)
	case BlockKitchenItems:
		if options, ok := block.parsed.(*KitchenItemsOptions); ok {
			renderKitchenItems(options, data, w)
		}
	}
}

// resolveTristate resuelve una opción «según Cocina / sí / no».
func resolveTristate(value string, inherited bool) bool {
	switch value {
	case OptionYes:
		return true
	case OptionNo:
		return false
	}
	return inherited
}

// renderKitchenItems imprime cada estación con su separador y nombre, y cada partida con sus
// partes, antes/ahora, observaciones, importe, motivo, referencia y espacio entre partidas.
func renderKitchenItems(options *KitchenItemsOptions, data *KitchenData, w *writer) {
	showNotes := resolveTristate(options.ShowNotes, data.Settings.ShowNotes)
	// Los importes solo existen si Cocina los muestra (la comanda no guarda precios en otro caso);
	// la plantilla puede ocultarlos, nunca forzarlos.
	showPrices := options.ShowPrices != OptionNo && data.Settings.ShowPrices
	quantityStyle := options.QuantityStyle
	if quantityStyle == Inherit {
		quantityStyle = data.Settings.QuantityStyle
	}
	lineSpacing := data.Settings.LineSpacing
	if options.LineSpacing != SpacingInherit {
		lineSpacing, _ = strconv.Atoi(options.LineSpacing)
	}
	for _, section := range data.Sections {
		if options.StationDivider {
			w.rule("-")
		}
		if options.StationHeadings {
			w.text(strings.ToUpper(section.StationName))
		}
		for _, item := range section.Items {
			heading := fmt.Sprintf("%d x %s", item.Quantity, item.Name)
			if quantityStyle == QuantityMulti {
				heading = fmt.Sprintf("%s × %d", item.Name, item.Quantity)
			}
			w.text(heading)
			if item.Quantity > 1 && len(item.Parts) > 0 {
				w.text("Detalle por unidad:")
			}
			if item.Before != nil {
				w.text("ANTES")
				if showNotes && item.Before.Notes != "" {
					w.text("NOTA: " + item.Before.Notes)
				}
				renderKitchenParts(item.Before.Parts, item.Before.Notes, showNotes, w)
				w.text("AHORA")
			}
			renderKitchenParts(item.Parts, item.Notes, showNotes, w)
			if showNotes && item.Notes != "" {
				w.text("NOTA: " + item.Notes)
			}
			if showPrices && item.Price != "" {
				w.text("Importe: " + pesos(item.Price))
			}
			if item.Reason != "" {
				w.text("Motivo: " + item.Reason)
			}
			if options.ItemReference {
				w.text("Partida: " + item.ItemID)
			}
			for spacing := 0; spacing < lineSpacing; spacing++ {
				w.text("")
			}
		}
	}
}

// renderKitchenParts imprime cada parte con sus cambios de ingredientes y complementos. itemNotes
// es la nota de la partida, que la comanda imprime para la parte raíz; las de componentes van en
// su parte. Las reglas de qué se muestra están en KitchenPartDetails (etapa K1: las usa también la
// pantalla de cocina para Android, para que la pantalla y el papel digan lo mismo).
func renderKitchenParts(parts []KitchenPart, itemNotes string, showNotes bool, w *writer) {
	for _, part := range parts {
		w.text(fmt.Sprintf("  %d x %s", part.Quantity, KitchenPartName(part)))
		for _, detail := range KitchenPartDetails(part, itemNotes, showNotes) {
			indent := "    "
			if detail.Level > 0 {
				indent = "      "
			}
			w.text(indent + detail.Text)
		}
	}
}

// KitchenPartName es el nombre de la parte con su variante («Tacos · Orden de 3»).
func KitchenPartName(part KitchenPart) string {
	name := part.Name
	if part.Variant != "" {
		name += " · " + part.Variant
	}
	return name
}

// KitchenDetail es un renglón de los cambios de una parte: nivel 0 para un ingrediente, el nombre de un grupo o la nota
// de la parte, y nivel 1 para las opciones de un grupo.
type KitchenDetail struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// KitchenPartDetails son los renglones que la comanda imprime debajo de una parte (etapa K1, extraído de
// renderKitchenParts sin cambiar ninguna regla):
//   - un ingrediente solo si lo solicitado difiere de lo incluido: «SIN …» si se quitó o «…: 2 (incluido 1)»;
//   - el ingrediente que ya representa una indicación rápida impresa en la nota no se repite (quickNoteCoverage);
//   - cada grupo de complementos con sus opciones solicitadas, o «Sin complementos»;
//   - la nota de la parte («NOTA: …») si se muestran notas.
//
// itemNotes es la nota de la partida, que cuenta para la parte raíz.
func KitchenPartDetails(part KitchenPart, itemNotes string, showNotes bool) []KitchenDetail {
	details := []KitchenDetail{}
	printedNotes := part.Notes
	if part.Path == "root" {
		printedNotes = itemNotes
	}
	covered := quickNoteCoverage(part.QuickNotes, printedNotes, showNotes)
	for _, ingredient := range part.Ingredients {
		requested, _ := decimal.NewFromString(ingredient.Requested)
		included, _ := decimal.NewFromString(ingredient.Included)
		if requested.Equal(included) {
			continue
		}
		// La indicación con efecto («Sin cebolla») ya se imprime en la nota: no se repite la
		// línea estructurada («SIN Cebolla») del mismo ingrediente.
		if covered[ingredient.IngredientID] {
			continue
		}
		if requested.IsZero() {
			details = append(details, KitchenDetail{Text: "SIN " + ingredient.Name})
		} else {
			details = append(details, KitchenDetail{Text: ingredient.Name + ": " + ingredient.Requested + " (incluido " + ingredient.Included + ")"})
		}
	}
	for _, group := range part.Groups {
		details = append(details, KitchenDetail{Text: group.Name + ":"})
		anyOption := false
		for _, option := range group.Options {
			quantity, _ := decimal.NewFromString(option.Requested)
			if quantity.IsPositive() {
				anyOption = true
				details = append(details, KitchenDetail{Level: 1, Text: option.Requested + " x " + option.Name})
			}
		}
		if !anyOption {
			details = append(details, KitchenDetail{Level: 1, Text: "Sin complementos"})
		}
	}
	if showNotes && part.Notes != "" {
		details = append(details, KitchenDetail{Text: "NOTA: " + part.Notes})
	}
	return details
}

// quickNoteCoverage devuelve los ingredientes cuya línea estructurada ya representa una indicación
// rápida con efecto impresa en la nota (etapa 8): la comanda imprime la indicación una sola vez.
// Solo cuenta si la comanda muestra notas y el texto resuelto de la indicación está entre las
// indicaciones de la nota (separadas por comas, como las une la captura).
func quickNoteCoverage(notes []KitchenQuickNote, printedNotes string, showNotes bool) map[string]bool {
	covered := map[string]bool{}
	if !showNotes || strings.TrimSpace(printedNotes) == "" {
		return covered
	}
	entries := strings.Split(printedNotes, ",")
	for _, note := range notes {
		if note.Effect == "" || note.Effect == quickNoteNoEffect || note.IngredientID == nil {
			continue
		}
		for _, entry := range entries {
			if strings.EqualFold(strings.TrimSpace(entry), strings.TrimSpace(note.Text)) {
				covered[*note.IngredientID] = true
			}
		}
	}
	return covered
}
