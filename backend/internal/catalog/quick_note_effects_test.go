package catalog

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// Identificadores de las indicaciones de prueba (Go exige UUID en la selección).
const (
	noteWithoutOnion = "00000000-0000-4000-8000-000000000301"
	noteWithOnion    = "00000000-0000-4000-8000-000000000302"
	noteExtraOnion   = "00000000-0000-4000-8000-000000000303"
	noteWellDone     = "00000000-0000-4000-8000-000000000304"
	noteInactive     = "00000000-0000-4000-8000-000000000305"
	noteNoIce        = "00000000-0000-4000-8000-000000000306"
	noteWithoutSalt  = "00000000-0000-4000-8000-000000000307"
	noteExtraCheese  = "00000000-0000-4000-8000-000000000308"
	noteWithCheese   = "00000000-0000-4000-8000-000000000309"
	noteExtraChili   = "00000000-0000-4000-8000-000000000310"
	noteUnknown      = "00000000-0000-4000-8000-000000000399"
)

func quickNote(id, category, text, effect, ingredient string, active bool) CategoryQuickNote {
	note := CategoryQuickNote{ID: id, CategoryID: category, Text: text, Effect: effect, Active: active}
	if ingredient != "" {
		note.IngredientID = &ingredient
	}
	return note
}

// quickNotesSnapshot parte del filete de referencia y agrega ingredientes con reglas distintas:
// cebolla incluida (removible y agregable hasta 3), queso no incluido (agregable hasta 2, $12.50),
// sal incluida fija y chile con máximo fraccionario; una ensalada sin cebolla, una bebida de otra
// categoría y un combo cuyo componente es el filete.
func quickNotesSnapshot() (Snapshot, PreviewRequest) {
	snapshot, request := referenceSnapshot()
	plate := snapshot.Products["plate"]
	plate.Ingredients = append(plate.Ingredients,
		ProductIngredient{IngredientID: "cheese", IncludedQuantity: "0", Addable: true, MaxQuantity: "2", ExtraPrice: "12.50"},
		ProductIngredient{IngredientID: "salt", IncludedQuantity: "1", MaxQuantity: "1", ExtraPrice: "0"},
		ProductIngredient{IngredientID: "chili", IncludedQuantity: "1", Removable: true, Addable: true, MaxQuantity: "1.5", ExtraPrice: "4.00"},
	)
	snapshot.Products["plate"] = plate
	snapshot.Products["salad"] = Product{
		Metadata:   Metadata{ID: "salad", Name: "Ensalada", Active: true, Revision: 1},
		CategoryID: "food", Kind: "product", BasePrice: "90.00",
	}
	snapshot.Products["water"] = Product{
		Metadata:   Metadata{ID: "water", Name: "Agua", Active: true, Revision: 1},
		CategoryID: "drinks", Kind: "product", BasePrice: "20.00",
	}
	snapshot.Products["combo"] = Product{
		Metadata:   Metadata{ID: "combo", Name: "Combo filete", Active: true, Revision: 1},
		CategoryID: "combos", Kind: "combo", BasePrice: "200.00",
		Slots: []ComboSlot{{
			ID: "main", Name: "Plato fuerte", MinQuantity: 1, MaxQuantity: 1, IncludedQuantity: 1, DefaultQuantity: 1,
			DefaultProductID: "plate", Options: []ComboOption{{ProductID: "plate", SubstitutionPrice: "0"}},
		}},
	}
	snapshot.Categories["drinks"] = Category{Metadata: Metadata{ID: "drinks", Active: true}}
	snapshot.Categories["combos"] = Category{Metadata: Metadata{ID: "combos", Active: true}}
	for id, name := range map[string]string{"cheese": "Queso", "salt": "Sal", "chili": "Chile"} {
		snapshot.Ingredients[id] = Ingredient{Metadata: Metadata{ID: id, Name: name, Active: true}}
	}
	snapshot.QuickNotes = map[string]CategoryQuickNote{}
	for _, note := range []CategoryQuickNote{
		quickNote(noteWithoutOnion, "food", "Sin cebolla", "omit", "onion", true),
		quickNote(noteWithOnion, "food", "Con cebolla", "normal", "onion", true),
		quickNote(noteExtraOnion, "food", "Cebolla extra", "extra", "onion", true),
		quickNote(noteWellDone, "food", "Bien cocido", "none", "", true),
		quickNote(noteInactive, "food", "Quitar cebolla", "omit", "onion", false),
		quickNote(noteNoIce, "drinks", "Sin hielo", "none", "", true),
		quickNote(noteWithoutSalt, "food", "Sin sal", "omit", "salt", true),
		quickNote(noteExtraCheese, "food", "Queso extra", "extra", "cheese", true),
		quickNote(noteWithCheese, "food", "Con queso", "normal", "cheese", true),
		quickNote(noteExtraChili, "food", "Chile extra", "extra", "chili", true),
	} {
		snapshot.QuickNotes[note.ID] = note
	}
	return snapshot, request
}

func selections(ids ...string) []QuickNoteSelection {
	texts := map[string]string{
		noteWithoutOnion: "Sin cebolla", noteWithOnion: "Con cebolla", noteExtraOnion: "Cebolla extra",
		noteWellDone: "Bien cocido", noteInactive: "Quitar cebolla", noteNoIce: "Sin hielo", noteWithoutSalt: "Sin sal",
		noteExtraCheese: "Queso extra", noteWithCheese: "Con queso", noteExtraChili: "Chile extra", noteUnknown: "Otra",
	}
	result := []QuickNoteSelection{}
	for _, id := range ids {
		result = append(result, QuickNoteSelection{NoteID: id, Text: texts[id]})
	}
	return result
}

func plateLine(ingredients map[string]string, notes ...string) PreviewRequest {
	customization := Customization{QuickNotes: selections(notes...)}
	for id, quantity := range ingredients {
		customization.Ingredients = append(customization.Ingredients, IngredientSelection{IngredientID: id, Quantity: quantity})
	}
	return PreviewRequest{Lines: []LineRequest{{ProductID: "plate", Quantity: 1, Customization: customization}}}
}

func expectQuickNoteError(t *testing.T, label string, err error, code string) {
	t.Helper()
	var problem *Error
	if !errors.As(err, &problem) || problem.Status != 422 || problem.Code != code {
		t.Fatalf("%s: se esperaba 422 %s y se obtuvo %v", label, code, err)
	}
}

func TestQuickNoteTargetsMatchCaptureButtons(t *testing.T) {
	onion := ProductIngredient{IncludedQuantity: "1", Removable: true, Addable: true, MaxQuantity: "3"}
	cheese := ProductIngredient{IncludedQuantity: "0", Addable: true, MaxQuantity: "2"}
	salt := ProductIngredient{IncludedQuantity: "1", MaxQuantity: "1"}
	chili := ProductIngredient{IncludedQuantity: "1", Removable: true, Addable: true, MaxQuantity: "1.5"}
	tiny := ProductIngredient{IncludedQuantity: "0", Addable: true, MaxQuantity: "0.5"}
	cases := []struct {
		label      string
		effect     string
		rule       ProductIngredient
		target     string
		applicable bool
	}{
		{"sin cebolla", "omit", onion, "0", true},
		{"con cebolla incluida", "normal", onion, "1", true},
		{"cebolla extra", "extra", onion, "2", true},
		{"sin queso no incluido", "omit", cheese, "0", false},
		{"con queso no incluido", "normal", cheese, "1", true},
		{"queso extra no incluido", "extra", cheese, "1", true},
		{"sin sal fija", "omit", salt, "0", false},
		{"con sal fija", "normal", salt, "1", true},
		{"sal extra fija", "extra", salt, "1", false},
		{"chile extra con tope fraccionario", "extra", chili, "1.5", true},
		{"agregable con máximo menor que 1", "normal", tiny, "1", false},
		{"efecto desconocido", "double", onion, "0", false},
	}
	for _, entry := range cases {
		target, applicable := QuickNoteTarget(entry.effect, entry.rule)
		if applicable != entry.applicable || (applicable && target.String() != entry.target) {
			t.Fatalf("%s: se obtuvo %s/%v", entry.label, target.String(), applicable)
		}
	}
}

func TestQuickNoteEffectsMustMatchTheStructuredSelection(t *testing.T) {
	snapshot, _ := quickNotesSnapshot()
	accepted := []struct {
		label       string
		ingredients map[string]string
		notes       []string
		price       string
	}{
		{"sin cebolla fija 0", map[string]string{"onion": "0"}, []string{noteWithoutOnion}, "150.00"},
		{"con cebolla incluida sin cambio", nil, []string{noteWithOnion}, "150.00"},
		{"con cebolla explícita igual a lo incluido", map[string]string{"onion": "1"}, []string{noteWithOnion}, "150.00"},
		{"cebolla extra fija incluido + 1", map[string]string{"onion": "2"}, []string{noteExtraOnion}, "150.10"},
		{"con queso no incluido fija 1", map[string]string{"cheese": "1"}, []string{noteWithCheese}, "162.50"},
		{"queso extra no incluido fija 1", map[string]string{"cheese": "1"}, []string{noteExtraCheese}, "162.50"},
		{"chile extra respeta el tope", map[string]string{"chili": "1.5"}, []string{noteExtraChili}, "152.00"},
		{"solo instrucción y efecto juntos", map[string]string{"onion": "0"}, []string{noteWellDone, noteWithoutOnion}, "150.00"},
		{"efectos sobre ingredientes distintos", map[string]string{"onion": "0", "cheese": "1"}, []string{noteWithoutOnion, noteExtraCheese}, "162.50"},
	}
	for _, entry := range accepted {
		quote, err := Calculate(snapshot, plateLine(entry.ingredients, entry.notes...), calculationTime, time.UTC)
		if err != nil {
			t.Fatalf("%s: %v", entry.label, err)
		}
		if quote.Lines[0].UnitPrice != entry.price {
			t.Fatalf("%s: precio %s, esperado %s", entry.label, quote.Lines[0].UnitPrice, entry.price)
		}
		if len(quote.Lines[0].QuickNotes) != len(entry.notes) {
			t.Fatalf("%s: indicaciones resueltas %+v", entry.label, quote.Lines[0].QuickNotes)
		}
	}
	rejected := []struct {
		label       string
		ingredients map[string]string
		notes       []string
		code        string
	}{
		{"sin cebolla con la cebolla incluida", nil, []string{noteWithoutOnion}, "QUICK_NOTE_INVALID"},
		{"con cebolla con doble porción", map[string]string{"onion": "2"}, []string{noteWithOnion}, "QUICK_NOTE_INVALID"},
		{"cebolla extra sin el extra", nil, []string{noteExtraOnion}, "QUICK_NOTE_INVALID"},
		{"cebolla extra con tres porciones", map[string]string{"onion": "3"}, []string{noteExtraOnion}, "QUICK_NOTE_INVALID"},
		{"con queso sin el queso", nil, []string{noteWithCheese}, "QUICK_NOTE_INVALID"},
		{"sin sal en sal fija", nil, []string{noteWithoutSalt}, "QUICK_NOTE_INVALID"},
		{"indicación inactiva", map[string]string{"onion": "0"}, []string{noteInactive}, "QUICK_NOTE_INVALID"},
		{"indicación de otra categoría", nil, []string{noteNoIce}, "QUICK_NOTE_INVALID"},
		{"indicación inexistente", nil, []string{noteUnknown}, "QUICK_NOTE_INVALID"},
		{"misma indicación dos veces", map[string]string{"onion": "0"}, []string{noteWithoutOnion, noteWithoutOnion}, "QUICK_NOTE_INVALID"},
		{"dos efectos sobre la cebolla", map[string]string{"onion": "0"}, []string{noteWithoutOnion, noteExtraOnion}, "QUICK_NOTE_CONFLICT"},
		{"sin y con cebolla", map[string]string{"onion": "0"}, []string{noteWithOnion, noteWithoutOnion}, "QUICK_NOTE_CONFLICT"},
	}
	for _, entry := range rejected {
		_, err := Calculate(snapshot, plateLine(entry.ingredients, entry.notes...), calculationTime, time.UTC)
		expectQuickNoteError(t, entry.label, err, entry.code)
	}
	// Una indicación con efecto no aparece ni se acepta en un producto sin ese ingrediente.
	salad := PreviewRequest{Lines: []LineRequest{{ProductID: "salad", Quantity: 1, Customization: Customization{
		QuickNotes: selections(noteWithoutOnion),
	}}}}
	_, err := Calculate(snapshot, salad, calculationTime, time.UTC)
	expectQuickNoteError(t, "sin cebolla en un producto sin cebolla", err, "QUICK_NOTE_INVALID")
	if err == nil || !strings.Contains(err.(*Error).Message, "no aplica a Ensalada") {
		t.Fatalf("El rechazo debe explicar que no aplica: %v", err)
	}
	// La forma se valida también: un identificador inválido es un 400 de validación.
	bad := plateLine(nil)
	bad.Lines[0].Customization.QuickNotes = []QuickNoteSelection{{NoteID: "sin-uuid", Text: "Sin cebolla"}}
	if _, err := Calculate(snapshot, bad, calculationTime, time.UTC); err == nil || err.(*Error).Status != 400 {
		t.Fatalf("Un identificador inválido debe rechazarse: %v", err)
	}
}

// «Cebolla extra» por indicación cobra exactamente lo mismo que el botón «Extra» del
// personalizador, porque fija la misma cantidad; el precio sale solo de los ingredientes.
func TestQuickNoteExtraChargesExactlyTheExtraButton(t *testing.T) {
	snapshot, _ := quickNotesSnapshot()
	for _, pair := range []struct {
		ingredient string
		quantity   string
		note       string
	}{
		{"onion", "2", noteExtraOnion},
		{"cheese", "1", noteExtraCheese},
		{"chili", "1.5", noteExtraChili},
	} {
		button, err := Calculate(snapshot, plateLine(map[string]string{pair.ingredient: pair.quantity}), calculationTime, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		withNote, err := Calculate(snapshot, plateLine(map[string]string{pair.ingredient: pair.quantity}, pair.note), calculationTime, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		first, second := button.Lines[0], withNote.Lines[0]
		if first.UnitPrice != second.UnitPrice || first.IngredientExtras != second.IngredientExtras || first.Total != second.Total {
			t.Fatalf("%s: botón %s/%s, indicación %s/%s", pair.ingredient, first.UnitPrice, first.IngredientExtras, second.UnitPrice, second.IngredientExtras)
		}
		if strings.Contains(string(jsonValue(first)), "quick_notes") {
			t.Fatal("Sin indicaciones el desglose no debe llevar quick_notes")
		}
	}
}

// El desglose congela texto, efecto e ingrediente de cada indicación, en la raíz y en los
// componentes de un combo; cada nivel se valida con la categoría de su propio producto.
func TestQuickNotesAreFrozenInTheBreakdown(t *testing.T) {
	snapshot, _ := quickNotesSnapshot()
	quote, err := Calculate(snapshot, plateLine(map[string]string{"onion": "0"}, noteWithoutOnion, noteWellDone), calculationTime, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(jsonValue(quote.Lines[0].QuickNotes))
	expected := `[{"note_id":"` + noteWithoutOnion + `","text":"Sin cebolla","effect":"omit","ingredient_id":"onion"},` +
		`{"note_id":"` + noteWellDone + `","text":"Bien cocido","effect":"none","ingredient_id":null}]`
	if encoded != expected {
		t.Fatalf("Indicaciones congeladas: %s", encoded)
	}
	component := Customization{
		Ingredients: []IngredientSelection{{IngredientID: "onion", Quantity: "0"}},
		QuickNotes:  selections(noteWithoutOnion),
	}
	combo := PreviewRequest{Lines: []LineRequest{{ProductID: "combo", Quantity: 1, Customization: Customization{
		Components: []ComponentSelection{{SlotID: "main", ProductID: "plate", Quantity: 1, Customization: component}},
	}}}}
	quote, err = Calculate(snapshot, combo, calculationTime, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	child := quote.Lines[0].Components[0].Product
	if len(quote.Lines[0].QuickNotes) != 0 || len(child.QuickNotes) != 1 || child.QuickNotes[0].Effect != "omit" {
		t.Fatalf("El componente no conservó su indicación: %+v", child.QuickNotes)
	}
	// En la raíz del combo la indicación del platillo no pertenece a su categoría.
	combo.Lines[0].Customization.QuickNotes = selections(noteWithoutOnion)
	_, err = Calculate(snapshot, combo, calculationTime, time.UTC)
	expectQuickNoteError(t, "indicación del platillo en la raíz del combo", err, "QUICK_NOTE_INVALID")
	// El componente sin la cebolla retirada no coincide con su indicación.
	combo.Lines[0].Customization.QuickNotes = nil
	combo.Lines[0].Customization.Components[0].Customization.Ingredients = nil
	_, err = Calculate(snapshot, combo, calculationTime, time.UTC)
	expectQuickNoteError(t, "componente sin aplicar la indicación", err, "QUICK_NOTE_INVALID")
}

// El camino «Preserve» conserva las indicaciones resueltas aunque la indicación cambie después y
// valida las nuevas con las reglas del snapshot del pedido y las cantidades congeladas.
func TestPreservedQuickNotesKeepTheFrozenResolution(t *testing.T) {
	snapshot, _ := quickNotesSnapshot()
	quote, err := Calculate(snapshot, plateLine(map[string]string{"onion": "0"}, noteWithoutOnion), calculationTime, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	var breakdown ProductBreakdown
	if err := json.Unmarshal(jsonValue(quote.Lines[0].ProductBreakdown), &breakdown); err != nil {
		t.Fatal(err)
	}
	// Después de confirmar, el administrador cambia el texto y el efecto de «Sin cebolla».
	edited := map[string]CategoryQuickNote{
		noteWithoutOnion: quickNote(noteWithoutOnion, "food", "Cebolla aparte", "none", "", true),
		noteWellDone:     snapshot.QuickNotes[noteWellDone],
		noteExtraCheese:  snapshot.QuickNotes[noteExtraCheese],
	}
	customization := Customization{
		Ingredients: []IngredientSelection{{IngredientID: "onion", Quantity: "0"}},
		QuickNotes:  selections(noteWithoutOnion, noteWellDone),
	}
	if err := preserveLevel(snapshot, edited, &breakdown, customization, 0); err != nil {
		t.Fatal(err)
	}
	if len(breakdown.QuickNotes) != 2 || breakdown.QuickNotes[0].Text != "Sin cebolla" || breakdown.QuickNotes[0].Effect != "omit" ||
		breakdown.QuickNotes[1].Text != "Bien cocido" {
		t.Fatalf("La resolución congelada cambió: %+v", breakdown.QuickNotes)
	}
	// Una indicación nueva con efecto que no coincide con la receta conservada se rechaza.
	customization.QuickNotes = selections(noteWithoutOnion, noteExtraCheese)
	err = preserveLevel(snapshot, edited, &breakdown, customization, 0)
	expectQuickNoteError(t, "indicación nueva sin su cantidad", err, "QUICK_NOTE_INVALID")
	// Quitar todas las indicaciones las retira del desglose.
	if err := preserveLevel(snapshot, edited, &breakdown, Customization{}, 0); err != nil || breakdown.QuickNotes != nil {
		t.Fatalf("No se retiraron las indicaciones: %v %+v", err, breakdown.QuickNotes)
	}
}
