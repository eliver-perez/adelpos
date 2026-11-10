package operations

import (
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/catalog"
)

// legacyDraft reproduce un borrador y su cotización tal como existían antes de la etapa 8 (sin
// indicaciones con identificador): ingredientes, un grupo, un componente de combo y observaciones.
func legacyDraft() (*catalog.Quote, []DraftItem) {
	variant := "00000000-0000-4000-8000-0000000000a9"
	items := []DraftItem{{
		ID: "00000000-0000-4000-8000-0000000000a1",
		LineRequest: catalog.LineRequest{
			ProductID: "00000000-0000-4000-8000-0000000000a2",
			Quantity:  2,
			Customization: catalog.Customization{
				Ingredients: []catalog.IngredientSelection{{IngredientID: "00000000-0000-4000-8000-0000000000a3", Quantity: "0"}},
				Groups: []catalog.GroupSelection{{
					GroupID: "00000000-0000-4000-8000-0000000000a4",
					Options: []catalog.IncludedOption{{OptionID: "00000000-0000-4000-8000-0000000000a5", Quantity: "2"}},
				}},
				Components: []catalog.ComponentSelection{{
					SlotID:    "00000000-0000-4000-8000-0000000000a6",
					ProductID: "00000000-0000-4000-8000-0000000000a7",
					VariantID: &variant,
					Quantity:  1,
					Customization: catalog.Customization{
						Ingredients: []catalog.IngredientSelection{},
						Groups:      []catalog.GroupSelection{},
						Components:  []catalog.ComponentSelection{},
						Notes:       "Sin sal",
					},
				}},
				Notes: "Sin cebolla, Bien cocida",
			},
		},
	}}
	child := &catalog.ProductBreakdown{
		ProductID: "00000000-0000-4000-8000-0000000000a7", Revision: 1, CategoryID: "00000000-0000-4000-8000-0000000000b1",
		VariantID: &variant, Name: "Papas", BasePrice: "30.00", PriceOrigin: "Precio base", IngredientExtras: "0.00",
		GroupExtras: "0.00", ComponentExtras: "0.00", UnitPrice: "30.00", Ingredients: []catalog.IngredientBreakdown{},
		Groups: []catalog.GroupBreakdown{}, Components: []catalog.ComponentBreakdown{}, Notes: "Sin sal",
	}
	quote := &catalog.Quote{
		CalculatedAt:      time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
		Timezone:          "America/Monterrey",
		PriceListID:       "00000000-0000-4000-8000-0000000000b2",
		PriceListRevision: 3,
		PriceListName:     "Restaurante",
		Lines: []catalog.QuoteLine{{
			ProductBreakdown: catalog.ProductBreakdown{
				ProductID: "00000000-0000-4000-8000-0000000000a2", Revision: 4, CategoryID: "00000000-0000-4000-8000-0000000000b1",
				Name: "Filete", BasePrice: "140.00", PriceOrigin: "Precio base", IngredientExtras: "0.00", GroupExtras: "10.00",
				ComponentExtras: "0.00", UnitPrice: "150.00",
				Ingredients: []catalog.IngredientBreakdown{{
					IngredientID: "00000000-0000-4000-8000-0000000000a3", Name: "Cebolla", Included: "1", Requested: "0",
					Extra: "0.00", RecipeQuantity: "0.000",
				}},
				Groups: []catalog.GroupBreakdown{{
					GroupID: "00000000-0000-4000-8000-0000000000a4", Revision: 2, Name: "Guarniciones", Mode: "value",
					OriginalValue: "55.00", RequestedValue: "65.00", Difference: "10.00", Extras: "0.00",
					Options: []catalog.OptionBreakdown{{
						OptionID: "00000000-0000-4000-8000-0000000000a5", ComplementID: "00000000-0000-4000-8000-0000000000b3",
						Name: "Papas", Included: "1", Requested: "2",
					}},
				}},
				Components: []catalog.ComponentBreakdown{{
					SlotID: "00000000-0000-4000-8000-0000000000a6", Name: "Acompañamiento", Quantity: 1, Charge: "0.00", Product: child,
				}},
				Notes: "Sin cebolla, Bien cocida",
			},
			Quantity: 2, Subtotal: "300.00", Discount: "0.00", Total: "300.00",
		}},
		Promotions: []catalog.PromotionResult{},
		Subtotal:   "300.00",
		Discount:   "0.00",
		Total:      "300.00",
	}
	return quote, items
}

// La huella de un borrador sin indicaciones con identificador y su serialización guardada son
// exactamente las de antes de la etapa 8: el valor esperado se calculó con el código anterior a
// customization.quick_notes. Así, un borrador guardado antes de actualizar se confirma sin
// QUOTE_CHANGED y las filas jsonb existentes no cambian de forma.
func TestDraftFingerprintWithoutQuickNotesIsUnchanged(t *testing.T) {
	quote, items := legacyDraft()
	fingerprint := quoteFingerprint(quote, map[string]any{"account_id": "00000000-0000-4000-8000-0000000000c1", "revision": 7, "draft": items})
	if fingerprint != legacyFingerprint {
		t.Fatalf("La huella de un borrador sin indicaciones cambió: %s", fingerprint)
	}
	if encoded := string(jsonValue(items)); strings.Contains(encoded, "quick_notes") {
		t.Fatalf("El borrador sin indicaciones serializa quick_notes: %s", encoded)
	}
	if encoded := string(jsonValue(quote.Lines[0])); strings.Contains(encoded, "quick_notes") {
		t.Fatalf("El desglose sin indicaciones serializa quick_notes: %s", encoded)
	}
}

const legacyFingerprint = "c939b03575416ee05016f56b3c9a91908ad669cc9fc9305925d4570b50aa1b3f"

// Las indicaciones rápidas elegidas son preparación, como las observaciones: cambiarlas sin tocar
// ingredientes, grupos ni componentes conserva la receta (camino «Preserve»). La copia del
// desglose no comparte los componentes con la versión anterior.
func TestQuickNotesArePreparationNotRecipe(t *testing.T) {
	quote, items := legacyDraft()
	previous := items[0].LineRequest
	changed := previous
	changed.Customization.QuickNotes = []catalog.QuickNoteSelection{{NoteID: "00000000-0000-4000-8000-0000000000d1", Text: "Bien cocida"}}
	changed.Customization.Notes = "Bien cocida"
	if !sameRecipe(previous, changed) {
		t.Fatal("Agregar una indicación rápida no debe cambiar la receta")
	}
	changed.Customization.Ingredients = []catalog.IngredientSelection{{IngredientID: "00000000-0000-4000-8000-0000000000a3", Quantity: "1"}}
	if sameRecipe(previous, changed) {
		t.Fatal("Cambiar un ingrediente sí cambia la receta")
	}
	line, err := cloneQuoteLine(quote.Lines[0])
	if err != nil {
		t.Fatal(err)
	}
	copyPreparationNotes(&line.ProductBreakdown, catalog.Customization{Components: []catalog.ComponentSelection{{
		SlotID: "00000000-0000-4000-8000-0000000000a6", Customization: catalog.Customization{Notes: "Con sal"},
	}}})
	if quote.Lines[0].Components[0].Product.Notes != "Sin sal" || line.Components[0].Product.Notes != "Con sal" {
		t.Fatal("La copia del desglose comparte los componentes con la versión anterior")
	}
}
