package kitchen

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/moneytext"
)

func cleanText(value string) string {
	return strings.Map(func(character rune) rune {
		if character == '\n' {
			return character
		}
		if character == '\t' {
			return ' '
		}
		// Nunca permitir ESC, GS, comandos de corte ni controles invisibles de usuario.
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return -1
		}
		return character
	}, value)
}

func wrapText(value string, width int) []string {
	result := []string{}
	for _, line := range strings.Split(cleanText(value), "\n") {
		remaining := []rune(line)
		for len(remaining) > width {
			result = append(result, string(remaining[:width]))
			remaining = remaining[width:]
		}
		result = append(result, string(remaining))
	}
	return result
}

func RenderText(document Document, paperWidth int) string {
	width := 42
	if paperWidth == 58 {
		width = 30
	}
	if document.Settings.TextSize == "large" {
		width /= 2
	}
	lines := []string{}
	add := func(text string) { lines = append(lines, wrapText(text, width)...) }
	if document.Settings.ShowEstablishment {
		add(document.Context.Establishment)
	}
	switch document.Kind {
	case "modification":
		add("MODIFICACIÓN")
	case "cancellation":
		add("CANCELACIÓN")
	case "test":
		add("PRUEBA DE IMPRESIÓN")
	default:
		add("COMANDA")
	}
	location, err := time.LoadLocation(document.Context.Timezone)
	if err != nil {
		location = time.UTC
	}
	add(document.CreatedAt.In(location).Format("02/01/2006 15:04:05"))
	add(fmt.Sprintf("Cuenta #%d · %s", document.Context.Folio, document.Context.AccountName))
	if document.Context.Mode == "takeaway" {
		add("PARA LLEVAR · " + document.Context.Origin)
		if document.Context.Customer != "" {
			add(document.Context.Customer)
		}
	} else {
		add(document.Context.Area + " · " + document.Context.Table)
	}
	add("Capturó: " + document.Context.Employee)
	add("Pedido: " + document.Context.OrderID)
	for _, section := range document.Sections {
		add(strings.Repeat("-", width))
		add(strings.ToUpper(section.Station.Name))
		for _, item := range section.Items {
			heading := fmt.Sprintf("%d x %s", item.Quantity, item.Name)
			if document.Settings.QuantityStyle == "multiplier" {
				heading = fmt.Sprintf("%s × %d", item.Name, item.Quantity)
			}
			add(heading)
			if item.Quantity > 1 && len(item.Parts) > 0 {
				add("Detalle por unidad:")
			}
			if item.Before != nil {
				add("ANTES")
				if document.Settings.ShowNotes && item.Before.Notes != "" {
					add("NOTA: " + item.Before.Notes)
				}
				renderParts(item.Before.Parts, item.Before.Notes, document.Settings.ShowNotes, add)
				add("AHORA")
			}
			renderParts(item.Parts, item.Notes, document.Settings.ShowNotes, add)
			if document.Settings.ShowNotes && item.Notes != "" {
				add("NOTA: " + item.Notes)
			}
			if document.Settings.ShowPrices && item.Price != "" {
				add("Importe: " + moneytext.Format(moneytext.CurrencyMXN, item.Price))
			}
			if item.Reason != "" {
				add("Motivo: " + item.Reason)
			}
			add("Partida: " + item.ItemID)
			for spacing := 0; spacing < document.Settings.LineSpacing; spacing++ {
				add("")
			}
		}
	}
	add(strings.Repeat("-", width))
	add("Documento: " + document.ID)
	return strings.Join(lines, "\n") + "\n"
}

// renderParts imprime cada parte con sus cambios de ingredientes y complementos. itemNotes es la
// nota de la partida, que la comanda imprime para la parte raíz; las de componentes van en su
// parte.
func renderParts(parts []Part, itemNotes string, showNotes bool, add func(string)) {
	for _, part := range parts {
		name := part.Name
		if part.Variant != "" {
			name += " · " + part.Variant
		}
		add(fmt.Sprintf("  %d x %s", part.Quantity, name))
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
				add("    SIN " + ingredient.Name)
			} else {
				add("    " + ingredient.Name + ": " + ingredient.Requested + " (incluido " + ingredient.Included + ")")
			}
		}
		for _, group := range part.Groups {
			add("    " + group.Name + ":")
			anyOption := false
			for _, option := range group.Options {
				quantity, _ := decimal.NewFromString(option.Requested)
				if quantity.IsPositive() {
					anyOption = true
					add("      " + option.Requested + " x " + option.Name)
				}
			}
			if !anyOption {
				add("      Sin complementos")
			}
		}
		if showNotes && part.Notes != "" {
			add("    NOTA: " + part.Notes)
		}
	}
}

// quickNoteCoverage devuelve los ingredientes cuya línea estructurada ya representa una indicación
// rápida con efecto impresa en la nota (etapa 8): la comanda imprime la indicación una sola vez.
// Solo cuenta si la comanda muestra notas y el texto resuelto de la indicación está entre las
// indicaciones de la nota (separadas por comas, como las une la captura). Sin indicaciones con
// efecto el mapa queda vacío y la comanda es idéntica a la de la etapa 7.1.
func quickNoteCoverage(notes []catalog.QuickNoteBreakdown, printedNotes string, showNotes bool) map[string]bool {
	covered := map[string]bool{}
	if !showNotes || strings.TrimSpace(printedNotes) == "" {
		return covered
	}
	entries := strings.Split(printedNotes, ",")
	for _, note := range notes {
		if note.Effect == "" || note.Effect == catalog.QuickNoteEffectNone || note.IngredientID == nil {
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
