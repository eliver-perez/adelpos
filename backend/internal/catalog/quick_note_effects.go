package catalog

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Indicaciones rápidas con efecto (etapa 8, §9 de la especificación). Una indicación con efecto no
// es un segundo mecanismo: la captura aplica la misma selección estructurada del personalizador
// (customization.ingredients) y guarda el identificador de la indicación en
// customization.quick_notes. Go comprueba aquí que ambas cosas coincidan; el precio y el consumo
// de inventario salen solo de los ingredientes, así que nunca se acumulan multiplicadores ni se
// cobra un extra sin consumirlo. El texto libre de notes nunca se interpreta.

// maximumSelectedQuickNotes limita las indicaciones elegidas por nivel: una categoría admite 40.
const maximumSelectedQuickNotes = 40

// quickNoteRejected es el 422 de una indicación que no corresponde al producto o a su selección.
func quickNoteRejected(message string) error {
	return &Error{Status: 422, Code: "QUICK_NOTE_INVALID", Message: message}
}

// quickNoteConflict es el 422 de dos indicaciones con efecto sobre el mismo ingrediente.
func quickNoteConflict(message string) error {
	return &Error{Status: 422, Code: "QUICK_NOTE_CONFLICT", Message: message}
}

// CheckQuickNoteShape revisa la forma de las indicaciones elegidas en un nivel, sin consultar el
// catálogo: identificadores válidos, sin repetir y un texto informativo de 1 a 80 caracteres. La
// usan el borrador (que no ejecuta el motor) y el motor de cálculo.
func CheckQuickNoteShape(selections []QuickNoteSelection) error {
	if len(selections) > maximumSelectedQuickNotes {
		return invalid("Una partida admite hasta 40 indicaciones rápidas por producto.")
	}
	seen := map[string]bool{}
	for _, selection := range selections {
		if err := validateID(selection.NoteID); err != nil {
			return err
		}
		length := utf8.RuneCountInString(strings.TrimSpace(selection.Text))
		if length < 1 || length > 80 {
			return invalid("El texto de una indicación rápida elegida debe tener entre 1 y 80 caracteres.")
		}
		if seen[selection.NoteID] {
			return quickNoteRejected("La indicación «" + selection.Text + "» está elegida dos veces en el mismo producto.")
		}
		seen[selection.NoteID] = true
	}
	return nil
}

// QuickNoteTarget devuelve la cantidad de porciones del ingrediente que fija un efecto en un
// producto y si el efecto es aplicable a su regla. Es la misma regla de los botones de la captura:
//   - omit: aplica si el producto incluye el ingrediente y es removible; fija 0.
//   - normal: aplica si lo incluye (fija lo incluido: «Con cebolla» no cambia nada) o si es
//     agregable con máximo de al menos 1 (fija 1).
//   - extra: aplica si es agregable y su máximo supera lo incluido; fija min(incluido + 1, máximo),
//     igual que el botón «Extra».
func QuickNoteTarget(effect string, rule ProductIngredient) (decimal.Decimal, bool) {
	included := amount(rule.IncludedQuantity)
	maximum := amount(rule.MaxQuantity)
	one := decimal.NewFromInt(1)
	switch effect {
	case QuickNoteEffectOmit:
		return decimal.Zero, included.IsPositive() && rule.Removable
	case QuickNoteEffectNormal:
		if included.IsPositive() {
			return included, true
		}
		return one, rule.Addable && maximum.GreaterThanOrEqual(one)
	case QuickNoteEffectExtra:
		return decimal.Min(included.Add(one), maximum), rule.Addable && maximum.GreaterThan(included)
	}
	return decimal.Zero, false
}

// resolveQuickNotes valida las indicaciones elegidas en un nivel (producto raíz o componente de
// combo) contra el producto calculado y devuelve su resolución congelada. requested es la
// cantidad efectiva de cada ingrediente del producto ya calculada por el motor (explícita o la
// incluida). Reglas, en orden:
//  1. la indicación existe, está activa y pertenece a la categoría del producto;
//  2. con efecto, su ingrediente está en las reglas del producto y el efecto es aplicable;
//  3. dos indicaciones con efecto sobre el mismo ingrediente se rechazan de forma explícita (un
//     solo efecto efectivo por ingrediente);
//  4. la cantidad solicitada del ingrediente coincide exactamente con la que fija el efecto.
//
// Todos los rechazos son 422 en español; ninguno altera el precio, que sigue saliendo de los
// ingredientes.
func resolveQuickNotes(
	definitions map[string]CategoryQuickNote,
	product Product,
	selections []QuickNoteSelection,
	requested []IngredientBreakdown,
) ([]QuickNoteBreakdown, error) {
	if len(selections) == 0 {
		return nil, nil
	}
	if err := CheckQuickNoteShape(selections); err != nil {
		return nil, err
	}
	rules := map[string]ProductIngredient{}
	for _, rule := range product.Ingredients {
		rules[rule.IngredientID] = rule
	}
	quantities := map[string]IngredientBreakdown{}
	for _, ingredient := range requested {
		quantities[ingredient.IngredientID] = ingredient
	}
	resolved := make([]QuickNoteBreakdown, 0, len(selections))
	effects := []CategoryQuickNote{}
	byIngredient := map[string]CategoryQuickNote{}
	// Primera pasada: existencia, categoría, aplicabilidad y un solo efecto por ingrediente, para
	// que dos efectos sobre el mismo ingrediente se informen como tales.
	for _, selection := range selections {
		definition, exists := definitions[selection.NoteID]
		if !exists || !definition.Active {
			return nil, quickNoteRejected("La indicación «" + selection.Text + "» ya no está disponible; quítala de " + product.Name + ".")
		}
		if definition.CategoryID != product.CategoryID {
			return nil, quickNoteRejected("La indicación «" + definition.Text + "» no pertenece a la categoría de " + product.Name + ".")
		}
		entry := QuickNoteBreakdown{
			NoteID: definition.ID,
			Text:   definition.Text,
			Effect: QuickNoteEffectNone,
		}
		if definition.Effect != "" && definition.Effect != QuickNoteEffectNone {
			if err := checkQuickNoteApplicable(definition, product, rules); err != nil {
				return nil, err
			}
			ingredientID := *definition.IngredientID
			if other, repeated := byIngredient[ingredientID]; repeated {
				return nil, quickNoteConflict(
					"Las indicaciones «" + other.Text + "» y «" + definition.Text + "» cambian el mismo ingrediente (" +
						quantities[ingredientID].Name + "); elige solo una.",
				)
			}
			byIngredient[ingredientID] = definition
			effects = append(effects, definition)
			entry.Effect = definition.Effect
			entry.IngredientID = &ingredientID
		}
		resolved = append(resolved, entry)
	}
	// Segunda pasada: la cantidad elegida de cada ingrediente coincide con la que fija su efecto.
	for _, definition := range effects {
		ingredient := quantities[*definition.IngredientID]
		target, _ := QuickNoteTarget(definition.Effect, rules[*definition.IngredientID])
		current, err := parseQuantity(ingredient.Requested)
		if err != nil || !current.Equal(target) {
			return nil, quickNoteRejected(
				"La indicación «" + definition.Text + "» no coincide con la cantidad elegida de " + ingredient.Name +
					"; vuelve a elegirla.",
			)
		}
	}
	return resolved, nil
}

// checkQuickNoteApplicable comprueba que el ingrediente de una indicación con efecto esté en las
// reglas del producto y que el efecto le sea aplicable (QuickNoteTarget).
func checkQuickNoteApplicable(definition CategoryQuickNote, product Product, rules map[string]ProductIngredient) error {
	if definition.IngredientID == nil {
		return quickNoteRejected("La indicación «" + definition.Text + "» no tiene ingrediente vinculado.")
	}
	notApplicable := quickNoteRejected("La indicación «" + definition.Text + "» no aplica a " + product.Name + ".")
	rule, configured := rules[*definition.IngredientID]
	if !configured {
		return notApplicable
	}
	if _, applicable := QuickNoteTarget(definition.Effect, rule); !applicable {
		return notApplicable
	}
	return nil
}

// quickNoteIDs reúne los identificadores elegidos en una selección y en sus componentes.
func quickNoteIDs(customization Customization, depth int, result map[string]bool) {
	if depth > 8 {
		return
	}
	for _, selection := range customization.QuickNotes {
		result[selection.NoteID] = true
	}
	for _, component := range customization.Components {
		quickNoteIDs(component.Customization, depth+1, result)
	}
}

// loadQuickNotes agrega al snapshot solo las indicaciones que usan las partidas; sin indicaciones
// el snapshot queda idéntico al de antes de la etapa 8.
func loadQuickNotes(ctx context.Context, tx pgx.Tx, snapshot *Snapshot, lines []LineRequest) error {
	selected := map[string]bool{}
	for _, line := range lines {
		quickNoteIDs(line.Customization, 0, selected)
	}
	if len(selected) == 0 {
		return nil
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		if err := validateID(id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	notes, err := readQuickNotesByID(ctx, tx, ids)
	if err != nil {
		return err
	}
	snapshot.QuickNotes = notes
	return nil
}

// PreserveQuickNotes resuelve las indicaciones de una modificación que conserva receta e importes
// (camino «Preserve» de operations: cambios solo de observaciones o reducciones de cantidad). Las
// indicaciones ya resueltas en la versión anterior se conservan tal como se congelaron, aunque la
// indicación haya cambiado después; las nuevas se leen del catálogo vigente y se validan contra
// las reglas del producto del snapshot del pedido y las cantidades congeladas del desglose (que no
// cambian, porque la receta es la misma). breakdown debe ser una copia propia del desglose.
func PreserveQuickNotes(ctx context.Context, tx pgx.Tx, snapshot Snapshot, breakdown *ProductBreakdown, customization Customization) error {
	missing := map[string]bool{}
	newQuickNoteIDs(breakdown, customization, 0, missing)
	current := map[string]CategoryQuickNote{}
	if len(missing) > 0 {
		ids := make([]string, 0, len(missing))
		for id := range missing {
			if err := validateID(id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7820148304)"); err != nil {
			return err
		}
		loaded, err := readQuickNotesByID(ctx, tx, ids)
		if err != nil {
			return err
		}
		current = loaded
	}
	return preserveLevel(snapshot, current, breakdown, customization, 0)
}

// newQuickNoteIDs reúne las indicaciones elegidas que no estaban resueltas en el mismo nivel del
// desglose anterior: son las únicas que se leen del catálogo vigente.
func newQuickNoteIDs(breakdown *ProductBreakdown, customization Customization, depth int, result map[string]bool) {
	if breakdown == nil || depth > 8 {
		return
	}
	frozen := map[string]bool{}
	for _, note := range breakdown.QuickNotes {
		frozen[note.NoteID] = true
	}
	for _, selection := range customization.QuickNotes {
		if !frozen[selection.NoteID] {
			result[selection.NoteID] = true
		}
	}
	for _, component := range breakdown.Components {
		for _, selection := range customization.Components {
			if selection.SlotID == component.SlotID {
				newQuickNoteIDs(component.Product, selection.Customization, depth+1, result)
			}
		}
	}
}

func preserveLevel(snapshot Snapshot, current map[string]CategoryQuickNote, breakdown *ProductBreakdown, customization Customization, depth int) error {
	if depth > 8 {
		return invalid("La personalización supera ocho niveles.")
	}
	if len(customization.QuickNotes) > 0 {
		product, exists := snapshot.Products[breakdown.ProductID]
		if !exists {
			return invalid("No se pudo verificar el producto de la partida para sus indicaciones.")
		}
		// Las indicaciones congeladas de este nivel valen como estaban: activas, de la categoría
		// del producto y con su efecto de entonces.
		definitions := map[string]CategoryQuickNote{}
		for id, note := range current {
			definitions[id] = note
		}
		for _, note := range breakdown.QuickNotes {
			definitions[note.NoteID] = CategoryQuickNote{
				ID:           note.NoteID,
				CategoryID:   product.CategoryID,
				Text:         note.Text,
				Effect:       note.Effect,
				IngredientID: note.IngredientID,
				Active:       true,
			}
		}
		resolved, err := resolveQuickNotes(definitions, product, customization.QuickNotes, breakdown.Ingredients)
		if err != nil {
			return err
		}
		breakdown.QuickNotes = resolved
	} else {
		breakdown.QuickNotes = nil
	}
	for index := range breakdown.Components {
		component := &breakdown.Components[index]
		if component.Product == nil {
			continue
		}
		child := Customization{}
		for _, selection := range customization.Components {
			if selection.SlotID == component.SlotID {
				child = selection.Customization
			}
		}
		if err := preserveLevel(snapshot, current, component.Product, child, depth+1); err != nil {
			return err
		}
	}
	return nil
}
