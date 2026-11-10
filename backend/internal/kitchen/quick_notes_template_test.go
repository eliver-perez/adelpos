package kitchen

import (
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/catalog"
)

// Comanda de etapa 7.1 (sin indicaciones resueltas): una partida modificada de combo con cambios
// de ingredientes en la raíz y en un componente, un complemento y notas en ambos niveles. El texto
// esperado se obtuvo con la plantilla anterior a la etapa 8.
const stage71Ticket = "MODIFICACIÓN\n10/11/2026 13:00:00\nCuenta #21 · Mesa 3\nSalón · Mesa 3\nCapturó: Laura\nPedido: pedido-1\n" +
	"------------------------------------------\nCOCINA\n1 x Combo hamburguesa\nANTES\nNOTA: Bien cocida\n  1 x Combo hamburguesa\n" +
	"AHORA\n  1 x Combo hamburguesa\n    SIN Cebolla\n    Queso: 2 (incluido 1)\n  2 x Papas · Grandes\n    Sal: 2 (incluido 1)\n" +
	"    SIN Cátsup\n    Salsas:\n      1 x Chipotle\n    NOTA: Extra sal, Sin cátsup\nNOTA: Sin cebolla, Bien cocida\n" +
	"Partida: partida-1\n------------------------------------------\nDocumento: documento-indicaciones\n"

func quickNotesTicket() Document {
	return Document{ID: "documento-indicaciones", Kind: "modification", CreatedAt: time.Date(2026, 11, 10, 19, 0, 0, 0, time.UTC),
		Settings: Settings{ShowNotes: true, QuantityStyle: "prefix"},
		Context: TicketContext{Timezone: "America/Monterrey", Folio: 21, AccountName: "Mesa 3", Area: "Salón", Table: "Mesa 3",
			Employee: "Laura", OrderID: "pedido-1"},
		Sections: []Section{{Station: Station{Name: "Cocina"}, Items: []Instructions{{
			ItemID: "partida-1", Name: "Combo hamburguesa", Quantity: 1, Notes: "Sin cebolla, Bien cocida",
			Before: &Instructions{Name: "Combo hamburguesa", Quantity: 1, Notes: "Bien cocida", Parts: []Part{
				{Path: "root", Name: "Combo hamburguesa", Quantity: 1, Ingredients: []catalog.IngredientBreakdown{
					{IngredientID: "cebolla", Name: "Cebolla", Included: "1", Requested: "1"}}},
			}},
			Parts: []Part{
				{Path: "root", Name: "Combo hamburguesa", Quantity: 1, Ingredients: []catalog.IngredientBreakdown{
					{IngredientID: "cebolla", Name: "Cebolla", Included: "1", Requested: "0"},
					{IngredientID: "queso", Name: "Queso", Included: "1", Requested: "2"}}},
				{Path: "root/0", Name: "Papas", Variant: "Grandes", Quantity: 2, Notes: "Extra sal, Sin cátsup",
					Ingredients: []catalog.IngredientBreakdown{
						{IngredientID: "sal", Name: "Sal", Included: "1", Requested: "2"},
						{IngredientID: "catsup", Name: "Cátsup", Included: "1", Requested: "0"}},
					Groups: []catalog.GroupBreakdown{{Name: "Salsas", Options: []catalog.OptionBreakdown{
						{Name: "Chipotle", Included: "0", Requested: "1"}}}}},
			},
		}}}}}
}

func resolvedNote(id, text, effect string, ingredientID string) catalog.QuickNoteBreakdown {
	note := catalog.QuickNoteBreakdown{NoteID: id, Text: text, Effect: effect}
	if ingredientID != "" {
		note.IngredientID = &ingredientID
	}
	return note
}

// Sin indicaciones con efecto la comanda es idéntica a la de la etapa 7.1, aunque la partida
// guarde los identificadores de indicaciones solo de instrucción.
func TestTicketWithoutQuickNoteEffectsIsUnchanged(t *testing.T) {
	if text := RenderText(quickNotesTicket(), 80); text != stage71Ticket {
		t.Fatalf("La comanda sin indicaciones cambió:\n%s", text)
	}
	document := quickNotesTicket()
	parts := document.Sections[0].Items[0].Parts
	parts[0].QuickNotes = []catalog.QuickNoteBreakdown{resolvedNote("n2", "Bien cocida", "none", "")}
	parts[1].QuickNotes = []catalog.QuickNoteBreakdown{resolvedNote("n5", "Sin cátsup", "none", "")}
	if text := RenderText(document, 80); text != stage71Ticket {
		t.Fatalf("Una indicación sin efecto alteró la comanda:\n%s", text)
	}
}

// Una indicación con efecto cuyo texto ya está en la nota cubre la línea estructurada de su
// ingrediente: se imprime una sola vez, en la raíz y en los componentes del combo. Las demás
// líneas se conservan.
func TestTicketPrintsEffectQuickNoteOnce(t *testing.T) {
	document := quickNotesTicket()
	parts := document.Sections[0].Items[0].Parts
	parts[0].QuickNotes = []catalog.QuickNoteBreakdown{
		resolvedNote("n1", "Sin cebolla", "omit", "cebolla"),
		resolvedNote("n2", "Bien cocida", "none", ""),
	}
	parts[1].QuickNotes = []catalog.QuickNoteBreakdown{
		resolvedNote("n4", "Extra sal", "extra", "sal"),
		resolvedNote("n5", "Sin cátsup", "omit", "catsup"),
	}
	text := RenderText(document, 80)
	for _, absent := range []string{"SIN Cebolla", "Sal: 2 (incluido 1)", "SIN Cátsup"} {
		if strings.Contains(text, absent) {
			t.Fatalf("La comanda repite la indicación como línea estructurada (%s):\n%s", absent, text)
		}
	}
	for _, present := range []string{"NOTA: Sin cebolla, Bien cocida", "Queso: 2 (incluido 1)", "NOTA: Extra sal, Sin cátsup", "1 x Chipotle"} {
		if strings.Count(text, present) != 1 {
			t.Fatalf("Falta o se repite %q:\n%s", present, text)
		}
	}
	if strings.Count(strings.ToLower(text), "sin cebolla") != 1 {
		t.Fatalf("«Sin cebolla» debe imprimirse una sola vez:\n%s", text)
	}
}

// Si la nota no se imprime (configuración sin notas) o ya no contiene el texto de la indicación,
// la línea estructurada se conserva para que cocina no pierda el cambio.
func TestTicketKeepsStructuredLineWhenNoteIsNotPrinted(t *testing.T) {
	document := quickNotesTicket()
	parts := document.Sections[0].Items[0].Parts
	parts[0].QuickNotes = []catalog.QuickNoteBreakdown{resolvedNote("n1", "Sin cebolla", "omit", "cebolla")}
	document.Settings.ShowNotes = false
	if text := RenderText(document, 80); !strings.Contains(text, "SIN Cebolla") || strings.Contains(text, "NOTA:") {
		t.Fatalf("Sin notas impresas debe quedar la línea estructurada:\n%s", text)
	}
	document = quickNotesTicket()
	document.Sections[0].Items[0].Notes = "Bien cocida"
	document.Sections[0].Items[0].Parts[0].QuickNotes = []catalog.QuickNoteBreakdown{resolvedNote("n1", "Sin cebolla", "omit", "cebolla")}
	if text := RenderText(document, 80); !strings.Contains(text, "SIN Cebolla") {
		t.Fatalf("Una nota sin el texto de la indicación debe conservar la línea estructurada:\n%s", text)
	}
}
