//go:build integration

package operations_test

import (
	"encoding/json"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
)

// Etapa 8, fase B (Bq): indicaciones rápidas con efecto. Se encadena al final de la aceptación con
// sus propios ingredientes y productos en la categoría de la fixture (que ya tiene ruta de cocina
// e indicaciones solo de instrucción de la etapa 7.1: «Bien cocida» y «Sin cebolla»).

type quickEffectsFixture struct {
	*restaurantFixture
	onion, cheese catalog.Ingredient
	burger, combo catalog.Product
	// Indicaciones con efecto creadas aquí y las de la etapa 7.1 (sin efecto).
	withoutOnion, extraOnion, extraCheese catalog.CategoryQuickNote
	cooked, legacyOnion                   catalog.CategoryQuickNote
}

func effectNoteBody(categoryID, text string, effect any, ingredientID any) map[string]any {
	body := quickNoteBody(categoryID, text, true, nil)
	body["effect"] = effect
	body["ingredient_id"] = ingredientID
	return body
}

func quickSelection(notes ...catalog.CategoryQuickNote) []catalog.QuickNoteSelection {
	result := []catalog.QuickNoteSelection{}
	for _, note := range notes {
		result = append(result, catalog.QuickNoteSelection{NoteID: note.ID, Text: note.Text})
	}
	return result
}

func noteTexts(notes ...catalog.CategoryQuickNote) string {
	texts := []string{}
	for _, note := range notes {
		texts = append(texts, note.Text)
	}
	return strings.Join(texts, ", ")
}

// firstTicket devuelve el contenido del primer trabajo de impresión de la primera liberación de la
// cuenta (la comanda que recibe cocina). La plantilla corta las líneas al ancho del papel; las
// comprobaciones de texto usan unwrapped.
func firstTicket(t *testing.T, fixture *restaurantFixture, bill operations.Bill) string {
	t.Helper()
	releases := kitchenReleases(t, fixture, bill)
	if len(releases) == 0 {
		t.Fatal("La partida no se liberó a cocina")
	}
	var content string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT j.content FROM app.kitchen_print_jobs j JOIN app.kitchen_tickets t ON t.id=j.ticket_id
        WHERE t.release_id=$1 ORDER BY j.created_at LIMIT 1`, releases[0].ID).Scan(&content); err != nil {
		t.Fatal(err)
	}
	return content
}

// unwrapped une las líneas de la comanda para buscar una nota que la plantilla cortó al ancho.
func unwrapped(ticket string) string {
	return strings.ReplaceAll(ticket, "\n", "")
}

func storedQuoteLine(t *testing.T, fixture *restaurantFixture, versionID string) string {
	t.Helper()
	var encoded string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT quote_line::text FROM app.restaurant_item_versions WHERE id=$1", versionID).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	return encoded
}

func testQuickNoteEffects(t *testing.T, base *restaurantFixture) {
	fixture := &quickEffectsFixture{restaurantFixture: base}
	fixture.setupProducts(t)
	t.Run("8Bq_indicaciones_con_efecto_administradas_con_permiso", func(t *testing.T) {
		testQuickNoteEffectAdministration(t, fixture)
	})
	var confirmed operations.Bill
	t.Run("8Bq_confirmacion_con_indicaciones_con_efecto", func(t *testing.T) {
		confirmed = testQuickNoteEffectConfirmation(t, fixture)
	})
	t.Run("8Bq_editar_la_indicacion_no_cambia_la_partida_confirmada", func(t *testing.T) {
		testQuickNoteEditDoesNotChangeConfirmed(t, fixture, confirmed)
	})
	t.Run("8Bq_partida_antigua_y_borrador_sin_quick_notes", func(t *testing.T) {
		testLegacyTextOnlyLines(t, fixture)
	})
	t.Run("8Bq_modificacion_preserve_conserva_las_indicaciones", func(t *testing.T) {
		testPreservedQuickNotes(t, fixture, confirmed)
	})
	t.Run("8Bq_componente_de_combo_con_indicacion", func(t *testing.T) {
		testComboComponentQuickNote(t, fixture)
	})
}

func (fixture *quickEffectsFixture) setupProducts(t *testing.T) {
	t.Helper()
	fixture.onion = decodeResponse[catalog.Ingredient](t, fixture.adminCall(t, "POST", "catalog/ingredients", catalog.Ingredient{
		Metadata: catalog.Metadata{Name: "Cebolla blanca", Active: true}, Unit: "porción",
	}, 200))
	fixture.cheese = decodeResponse[catalog.Ingredient](t, fixture.adminCall(t, "POST", "catalog/ingredients", catalog.Ingredient{
		Metadata: catalog.Metadata{Name: "Queso manchego", Active: true}, Unit: "porción",
	}, 200))
	// Cebolla incluida (removible y agregable hasta 3, $5.00 por porción extra) y queso no incluido
	// (agregable hasta 2, $12.50).
	fixture.burger = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Hamburguesa con efectos", Active: true}, CategoryID: fixture.category.ID,
		Code: "HAMB-EFECTOS", Kind: "product", BasePrice: "120",
		Ingredients: []catalog.ProductIngredient{
			{IngredientID: fixture.onion.ID, IncludedQuantity: "1", Removable: true, Addable: true, MaxQuantity: "3", ExtraPrice: "5.00", RecipeQuantity: "0"},
			{IngredientID: fixture.cheese.ID, IncludedQuantity: "0", Addable: true, MaxQuantity: "2", ExtraPrice: "12.50", RecipeQuantity: "0"},
		},
	}, 200))
	fixture.combo = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Combo con efectos", Active: true}, CategoryID: fixture.category.ID,
		Code: "COMBO-EFECTOS", Kind: "combo", BasePrice: "160",
		Slots: []catalog.ComboSlot{{
			Name: "Hamburguesa", MinQuantity: 1, MaxQuantity: 1, IncludedQuantity: 1, DefaultQuantity: 1,
			DefaultProductID: fixture.burger.ID,
			Options:          []catalog.ComboOption{{ProductID: fixture.burger.ID, SubstitutionPrice: "0"}},
		}},
	}, 200))
	for _, note := range lookupQuickNotes(t, fixture.restaurantFixture, fixture.admin, fixture.category.ID) {
		switch note.Text {
		case "Bien cocida":
			fixture.cooked = note
		case "Sin cebolla":
			fixture.legacyOnion = note
		}
	}
	if fixture.cooked.ID == "" || fixture.legacyOnion.ID == "" {
		t.Fatal("Faltan las indicaciones de la etapa 7.1 de la fixture")
	}
}

func testQuickNoteEffectAdministration(t *testing.T, fixture *quickEffectsFixture) {
	categoryID := fixture.category.ID
	count := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.catalog_category_quick_notes")
	// Sin catalog.categories.manage: 403 sin registros.
	fixture.call(t, fixture.waiter, "POST", "catalog/quick-notes", effectNoteBody(categoryID, "Quitar cebolla blanca", "omit", fixture.onion.ID), 403)
	// Efecto sin ingrediente, efecto desconocido, instrucción con ingrediente, ingrediente
	// inexistente y texto con coma: rechazados como hoy, sin registros.
	for _, body := range []map[string]any{
		effectNoteBody(categoryID, "Quitar cebolla blanca", "omit", nil),
		effectNoteBody(categoryID, "Quitar cebolla blanca", "double", fixture.onion.ID),
		effectNoteBody(categoryID, "Quitar cebolla blanca", "none", fixture.onion.ID),
		effectNoteBody(categoryID, "Quitar cebolla blanca", "omit", randomID()),
		effectNoteBody(categoryID, "Quitar cebolla, blanca", "omit", fixture.onion.ID),
	} {
		fixture.adminCall(t, "POST", "catalog/quick-notes", body, 400)
	}
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.catalog_category_quick_notes") != count {
		t.Fatal("Una indicación rechazada dejó registros")
	}
	fixture.withoutOnion = decodeResponse[catalog.CategoryQuickNote](t, fixture.adminCall(t, "POST", "catalog/quick-notes",
		effectNoteBody(categoryID, "Quitar cebolla blanca", "omit", fixture.onion.ID), 200))
	fixture.extraOnion = decodeResponse[catalog.CategoryQuickNote](t, fixture.adminCall(t, "POST", "catalog/quick-notes",
		effectNoteBody(categoryID, "Cebolla blanca extra", "extra", fixture.onion.ID), 200))
	fixture.extraCheese = decodeResponse[catalog.CategoryQuickNote](t, fixture.adminCall(t, "POST", "catalog/quick-notes",
		effectNoteBody(categoryID, "Queso extra", "extra", fixture.cheese.ID), 200))
	if fixture.withoutOnion.Effect != "omit" || fixture.withoutOnion.IngredientID == nil || *fixture.withoutOnion.IngredientID != fixture.onion.ID ||
		fixture.withoutOnion.IngredientName == nil || *fixture.withoutOnion.IngredientName != "Cebolla blanca" {
		t.Fatalf("La indicación no guardó su efecto: %+v", fixture.withoutOnion)
	}
	if auditCount(t, fixture.restaurantFixture, "catalog.quick-notes.created", fixture.extraCheese.ID) != 1 {
		t.Fatal("La indicación con efecto no se auditó")
	}
	// Un cliente que no envía el efecto (etapa 7.1) conserva el de la indicación al editarla.
	fixture.extraCheese = decodeResponse[catalog.CategoryQuickNote](t, fixture.adminCall(t, "PATCH", "catalog/quick-notes/"+fixture.extraCheese.ID, map[string]any{
		"revision": fixture.extraCheese.Revision, "text": "Queso extra", "active": true,
	}, 200))
	if fixture.extraCheese.Effect != "extra" || fixture.extraCheese.IngredientID == nil {
		t.Fatalf("Editar sin efecto borró el efecto: %+v", fixture.extraCheese)
	}
	// Los lookups exponen efecto, ingrediente y su nombre; las indicaciones de la 7.1 siguen sin
	// efecto.
	for _, note := range lookupQuickNotes(t, fixture.restaurantFixture, fixture.waiter, categoryID) {
		switch note.ID {
		case fixture.extraCheese.ID:
			if note.Effect != "extra" || note.IngredientName == nil || *note.IngredientName != "Queso manchego" {
				t.Fatalf("Los lookups no exponen el efecto: %+v", note)
			}
		case fixture.cooked.ID, fixture.legacyOnion.ID:
			if note.Effect != "none" || note.IngredientID != nil || note.IngredientName != nil {
				t.Fatalf("Una indicación de la 7.1 cambió de efecto: %+v", note)
			}
		}
	}
}

func (fixture *quickEffectsFixture) burgerLine(quantity int, onion, cheese string, notes ...catalog.CategoryQuickNote) catalog.LineRequest {
	customization := catalog.Customization{Notes: noteTexts(notes...), QuickNotes: quickSelection(notes...)}
	if onion != "" {
		customization.Ingredients = append(customization.Ingredients, catalog.IngredientSelection{IngredientID: fixture.onion.ID, Quantity: onion})
	}
	if cheese != "" {
		customization.Ingredients = append(customization.Ingredients, catalog.IngredientSelection{IngredientID: fixture.cheese.ID, Quantity: cheese})
	}
	return catalog.LineRequest{ProductID: fixture.burger.ID, Quantity: quantity, Customization: customization}
}

func draftPreview(t *testing.T, fixture *restaurantFixture, bill operations.Bill, status int, lines ...catalog.LineRequest) []byte {
	t.Helper()
	items := []operations.DraftItem{}
	for _, line := range lines {
		items = append(items, operations.DraftItem{ID: randomID(), LineRequest: line})
	}
	return fixture.adminCall(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/draft-preview", map[string]any{
		"revision": bill.Account.Revision, "items": items,
	}, status)
}

func testQuickNoteEffectConfirmation(t *testing.T, fixture *quickEffectsFixture) operations.Bill {
	bill := newAccount(t, fixture.restaurantFixture, "Indicaciones con efecto")
	// La indicación cobra exactamente lo mismo que el botón «Extra»: la misma cantidad de queso.
	withNotes := decodeResponse[operations.QuoteResult](t, draftPreview(t, fixture.restaurantFixture, bill, 200,
		fixture.burgerLine(2, "0", "1", fixture.withoutOnion, fixture.extraCheese, fixture.cooked)))
	withoutNotes := decodeResponse[operations.QuoteResult](t, draftPreview(t, fixture.restaurantFixture, bill, 200,
		fixture.burgerLine(2, "0", "1")))
	if withNotes.Quote.Lines[0].UnitPrice != "132.50" || withoutNotes.Quote.Lines[0].UnitPrice != "132.50" ||
		withNotes.Quote.Total != withoutNotes.Quote.Total {
		t.Fatalf("El precio con indicación difiere del botón Extra: %s / %s", withNotes.Quote.Lines[0].UnitPrice, withoutNotes.Quote.Lines[0].UnitPrice)
	}
	// La cantidad del ingrediente debe coincidir con el efecto y dos efectos sobre la cebolla se
	// rechazan de forma explícita.
	mismatch := draftPreview(t, fixture.restaurantFixture, bill, 422, fixture.burgerLine(1, "", "", fixture.withoutOnion))
	conflict := draftPreview(t, fixture.restaurantFixture, bill, 422, fixture.burgerLine(1, "0", "", fixture.withoutOnion, fixture.extraOnion))
	if errorCode(t, mismatch) != "QUICK_NOTE_INVALID" || errorCode(t, conflict) != "QUICK_NOTE_CONFLICT" {
		t.Fatalf("Rechazos de indicaciones: %s / %s", mismatch, conflict)
	}
	items := []operations.DraftItem{{ID: randomID(), LineRequest: fixture.burgerLine(2, "0", "1", fixture.withoutOnion, fixture.extraCheese, fixture.cooked)}}
	route := "restaurant/accounts/" + bill.Account.ID
	bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", route+"/draft", map[string]any{"revision": bill.Account.Revision, "items": items}, 200))
	bill = confirmDraft(t, fixture.restaurantFixture, bill)
	if len(bill.Lines) != 1 || bill.Account.Totals.Total != "265.00" {
		t.Fatalf("Confirmación con indicaciones: %+v", bill.Account.Totals)
	}
	line := bill.Lines[0]
	resolved := line.QuoteLine.QuickNotes
	if len(resolved) != 3 || resolved[0].Effect != "omit" || *resolved[0].IngredientID != fixture.onion.ID ||
		resolved[1].Effect != "extra" || resolved[2].Effect != "none" || resolved[2].IngredientID != nil {
		t.Fatalf("La partida no congeló las indicaciones resueltas: %+v", resolved)
	}
	if len(line.Request.Customization.QuickNotes) != 3 || line.Request.Customization.Notes != "Quitar cebolla blanca, Queso extra, Bien cocida" {
		t.Fatalf("La solicitud no conserva identificadores y texto: %+v", line.Request.Customization)
	}
	// La comanda imprime cada indicación una sola vez: sin la línea estructurada duplicada.
	ticket := firstTicket(t, fixture.restaurantFixture, bill)
	if !strings.Contains(unwrapped(ticket), "NOTA: Quitar cebolla blanca, Queso extra, Bien cocida") ||
		strings.Contains(ticket, "SIN Cebolla blanca") || strings.Contains(ticket, "Queso manchego: 1 (incluido 0)") {
		t.Fatalf("La comanda repite las indicaciones con efecto:\n%s", ticket)
	}
	return bill
}

func testQuickNoteEditDoesNotChangeConfirmed(t *testing.T, fixture *quickEffectsFixture, bill operations.Bill) {
	if len(bill.Lines) == 0 {
		t.Fatal("Falta la partida confirmada")
	}
	before := storedQuoteLine(t, fixture.restaurantFixture, bill.Lines[0].VersionID)
	fixture.withoutOnion = decodeResponse[catalog.CategoryQuickNote](t, fixture.adminCall(t, "PATCH", "catalog/quick-notes/"+fixture.withoutOnion.ID, map[string]any{
		"revision": fixture.withoutOnion.Revision, "text": "Cebolla blanca aparte", "active": true, "effect": "none", "ingredient_id": nil,
	}, 200))
	if fixture.withoutOnion.Effect != "none" || fixture.withoutOnion.IngredientID != nil {
		t.Fatalf("La indicación no cambió de efecto: %+v", fixture.withoutOnion)
	}
	if after := storedQuoteLine(t, fixture.restaurantFixture, bill.Lines[0].VersionID); after != before {
		t.Fatal("Editar la indicación alteró la partida confirmada")
	}
	refreshed := fixture.refresh(t, bill.Account.ID)
	if refreshed.Lines[0].QuoteLine.QuickNotes[0].Text != "Quitar cebolla blanca" || refreshed.Lines[0].QuoteLine.QuickNotes[0].Effect != "omit" {
		t.Fatalf("La partida no conservó la indicación congelada: %+v", refreshed.Lines[0].QuoteLine.QuickNotes)
	}
}

func testLegacyTextOnlyLines(t *testing.T, fixture *quickEffectsFixture) {
	bill := newAccount(t, fixture.restaurantFixture, "Partida solo texto")
	route := "restaurant/accounts/" + bill.Account.ID
	// Como en la 7.1: «Sin cebolla» es solo texto (sin quick_notes) y no altera ingredientes.
	items := []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{
		ProductID: fixture.burger.ID, Quantity: 1, Customization: catalog.Customization{Notes: "Sin cebolla, Bien cocida"},
	}}}
	bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", route+"/draft", map[string]any{"revision": bill.Account.Revision, "items": items}, 200))
	var stored string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT customization::text FROM app.restaurant_draft_items WHERE id=$1", items[0].ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "quick_notes") {
		t.Fatalf("El borrador sin indicaciones guarda quick_notes: %s", stored)
	}
	// La huella del borrador es estable y confirma sin QUOTE_CHANGED.
	first := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "GET", route+"/preview", nil, 200))
	second := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "GET", route+"/preview", nil, 200))
	if first.Fingerprint == "" || first.Fingerprint != second.Fingerprint || first.Quote.Total != "120.00" {
		t.Fatalf("Huella del borrador sin indicaciones: %s / %s / %s", first.Fingerprint, second.Fingerprint, first.Quote.Total)
	}
	bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", route+"/confirm", map[string]any{
		"revision": bill.Account.Revision, "expected_fingerprint": first.Fingerprint,
	}, 200))
	quoteLine := storedQuoteLine(t, fixture.restaurantFixture, bill.Lines[0].VersionID)
	var request string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT request::text FROM app.restaurant_item_versions WHERE id=$1", bill.Lines[0].VersionID).Scan(&request); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(quoteLine, "quick_notes") || strings.Contains(request, "quick_notes") || strings.Contains(quoteLine, "QuickNotes") {
		t.Fatalf("Una partida solo con texto guarda indicaciones resueltas: %s %s", quoteLine, request)
	}
	if bill.Lines[0].QuoteLine.Ingredients[0].Requested != "1" {
		t.Fatal("El texto libre alteró los ingredientes")
	}
	var snapshot string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT pricing_snapshot::text FROM app.restaurant_orders WHERE id=$1", bill.Lines[0].OriginalOrderID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot, "QuickNotes") {
		t.Fatal("El snapshot de un pedido sin indicaciones cambió de forma")
	}
	if ticket := firstTicket(t, fixture.restaurantFixture, bill); !strings.Contains(unwrapped(ticket), "NOTA: Sin cebolla, Bien cocida") || strings.Contains(ticket, "SIN Cebolla") {
		t.Fatalf("La comanda de una partida solo con texto cambió:\n%s", ticket)
	}
}

func testPreservedQuickNotes(t *testing.T, fixture *quickEffectsFixture, bill operations.Bill) {
	bill = fixture.refresh(t, bill.Account.ID)
	if len(bill.Lines) == 0 {
		t.Fatal("Falta la partida confirmada")
	}
	line := bill.Lines[0]
	route := "restaurant/accounts/" + bill.Account.ID
	// Reducir la cantidad y agregar observaciones y una indicación sin efecto conserva receta e
	// importes: las indicaciones resueltas quedan congeladas (con el texto de la confirmación).
	request := line.Request
	request.Quantity = 1
	request.Customization.QuickNotes = append(append([]catalog.QuickNoteSelection{}, request.Customization.QuickNotes...),
		catalog.QuickNoteSelection{NoteID: fixture.legacyOnion.ID, Text: fixture.legacyOnion.Text})
	request.Customization.Notes += ", Sin cebolla, Salsa aparte"
	input := operations.EditInput{Mutation: operations.Mutation{Revision: bill.Account.Revision}, VersionID: line.VersionID, Line: request, Reason: "Cliente pidió una sola"}
	preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", route+"/edit-preview", input, 200))
	if !strings.Contains(preview.PricingPolicy, "Se conservan los precios") || preview.Quote.Total != "132.50" {
		t.Fatalf("La reducción con indicaciones no conservó importes: %s %s", preview.PricingPolicy, preview.Quote.Total)
	}
	input.ExpectedFingerprint = preview.Fingerprint
	bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", route+"/edit", input, 200))
	// La cuenta muestra también la unidad cancelada de la versión anterior; se toma la activa.
	var edited operations.ConfirmedLine
	for _, entry := range bill.Lines {
		if entry.State == "active" {
			edited = entry
		}
	}
	// Preserve conserva el pedido (y su snapshot) de la versión anterior.
	sameOrder := rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.restaurant_item_versions a
        JOIN app.restaurant_item_versions b ON b.order_id=a.order_id WHERE a.id=$1 AND b.id=$2`, line.VersionID, edited.VersionID)
	if sameOrder != 1 || edited.Version != line.Version+1 {
		t.Fatalf("La modificación no siguió el camino Preserve: %+v", edited)
	}
	resolved := edited.QuoteLine.QuickNotes
	if len(resolved) != 4 || resolved[0].Text != "Quitar cebolla blanca" || resolved[0].Effect != "omit" || resolved[1].Effect != "extra" ||
		resolved[3].NoteID != fixture.legacyOnion.ID || resolved[3].Effect != "none" {
		t.Fatalf("Preserve no conservó las indicaciones: %+v", resolved)
	}
	var stored catalog.QuoteLine
	if err := json.Unmarshal([]byte(storedQuoteLine(t, fixture.restaurantFixture, edited.VersionID)), &stored); err != nil || len(stored.QuickNotes) != 4 {
		t.Fatalf("La versión nueva no guardó las indicaciones: %v", err)
	}
	// Una indicación nueva con efecto sobre un ingrediente ya cubierto se rechaza con 422.
	request = edited.Request
	request.Customization.QuickNotes = append(append([]catalog.QuickNoteSelection{}, request.Customization.QuickNotes...),
		catalog.QuickNoteSelection{NoteID: fixture.extraOnion.ID, Text: fixture.extraOnion.Text})
	input = operations.EditInput{Mutation: operations.Mutation{Revision: bill.Account.Revision}, VersionID: edited.VersionID, Line: request, Reason: "Cliente cambió de idea"}
	problem := fixture.adminCall(t, "POST", route+"/edit-preview", input, 422)
	if errorCode(t, problem) != "QUICK_NOTE_CONFLICT" {
		t.Fatalf("Una indicación incompatible en Preserve no se rechazó: %s", problem)
	}
}

func testComboComponentQuickNote(t *testing.T, fixture *quickEffectsFixture) {
	bill := newAccount(t, fixture.restaurantFixture, "Combo con indicación")
	component := catalog.Customization{
		Ingredients: []catalog.IngredientSelection{{IngredientID: fixture.cheese.ID, Quantity: "1"}},
		Notes:       fixture.extraCheese.Text,
		QuickNotes:  quickSelection(fixture.extraCheese),
	}
	line := catalog.LineRequest{ProductID: fixture.combo.ID, Quantity: 1, Customization: catalog.Customization{
		Components: []catalog.ComponentSelection{{SlotID: fixture.combo.Slots[0].ID, ProductID: fixture.burger.ID, Quantity: 1, Customization: component}},
	}}
	// En la raíz del combo la indicación del queso no aplica: el combo no tiene ese ingrediente.
	rootLine := line
	rootLine.Customization.QuickNotes = quickSelection(fixture.extraCheese)
	if problem := draftPreview(t, fixture.restaurantFixture, bill, 422, rootLine); errorCode(t, problem) != "QUICK_NOTE_INVALID" {
		t.Fatalf("Una indicación inaplicable en la raíz del combo no se rechazó: %s", problem)
	}
	bill = fixture.capture(t, bill, []catalog.LineRequest{line})
	child := bill.Lines[0].QuoteLine.Components[0].Product
	if bill.Account.Totals.Total != "172.50" || child == nil || len(child.QuickNotes) != 1 || child.QuickNotes[0].Effect != "extra" {
		t.Fatalf("El componente no congeló su indicación: %s %+v", bill.Account.Totals.Total, child)
	}
	ticket := firstTicket(t, fixture.restaurantFixture, bill)
	if !strings.Contains(unwrapped(ticket), "NOTA: Queso extra") || strings.Contains(ticket, "Queso manchego: 1 (incluido 0)") {
		t.Fatalf("La comanda del componente repite la indicación:\n%s", ticket)
	}
}
