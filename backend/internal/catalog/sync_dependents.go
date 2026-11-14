package catalog

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// Qué entra en una descarga de cambios y con qué disponibilidad (revisión independiente de la fase D, hallazgos 1 y
// 5). La bitácora anota cada registro que cambió; aquí se agrega lo que depende de él:
//   - una categoría que cambió trae sus productos (su disponibilidad depende de ella);
//   - un producto que cambió (agotado, disponible otra vez, inactivo, archivado, de otra categoría…) trae los combos
//     que lo ofrecen en una ranura (también como predeterminado, y combos de combos) y los grupos cuyos complementos
//     son ese producto;
//   - un complemento, una porción o un ingrediente que cambió trae los grupos que los usan;
//   - si cambió la lista predeterminada o la tasa general de IVA, todos los productos vigentes y, además, los
//     archivados que cambiaron, para que el cliente los quite (antes se filtraban y el cliente los conservaba).
//
// Las dependencias se resuelven al leer, con las relaciones vigentes, a partir de lo que anotó la bitácora: así
// cubren cualquier causa (agotado, desactivación, categoría, complemento) sin que los disparadores de la 0023 tengan
// que conocer las relaciones, y la versión ya subió con el registro que cambió.

type syncPlan struct {
	full        bool
	allProducts bool
	changed     map[string][]string
}

// selection son los identificadores de una entidad que van en la descarga: nil = todos (descarga completa).
func (plan *syncPlan) selection(entity string) []string {
	if plan.full {
		return nil
	}
	ids := plan.changed[entity]
	if ids == nil {
		return []string{}
	}
	return ids
}

// add agrega identificadores sin repetir y dice cuántos eran nuevos.
func (plan *syncPlan) add(entity string, ids ...string) int {
	added := 0
	for _, id := range ids {
		if !slices.Contains(plan.changed[entity], id) {
			plan.changed[entity] = append(plan.changed[entity], id)
			added++
		}
	}
	return added
}

// products lee los productos de la descarga.
func (plan *syncPlan) products(ctx context.Context, tx pgx.Tx) ([]any, error) {
	if plan.full {
		return syncDocuments(ctx, tx, "products", nil, "NOT p.archived")
	}
	if !plan.allProducts {
		return syncDocuments(ctx, tx, "products", plan.selection("product"), "")
	}
	current, err := syncDocuments(ctx, tx, "products", nil, "NOT p.archived")
	if err != nil {
		return nil, err
	}
	archived, err := syncDocuments(ctx, tx, "products", plan.selection("product"), "p.archived")
	if err != nil {
		return nil, err
	}
	return append(current, archived...), nil
}

// planDelta arma la descarga de cambios a partir de la bitácora.
func planDelta(ctx context.Context, tx pgx.Tx, changes syncChanges) (syncPlan, error) {
	plan := syncPlan{changed: map[string][]string{}}
	for entity, ids := range changes.changed {
		plan.changed[entity] = slices.Clone(ids)
	}
	plan.allProducts = len(changes.changed["price_list"]) > 0 || len(changes.changed["tax"]) > 0
	if ids := changes.changed["category"]; len(ids) > 0 {
		extra, err := productsOfCategories(ctx, tx, ids)
		if err != nil {
			return plan, err
		}
		plan.add("product", extra...)
	}
	// Combos que ofrecen un producto que cambió; un combo puede ser componente de otro (hasta ocho niveles).
	for depth := 0; depth < 8; depth++ {
		combos, err := identifiers(ctx, tx, `SELECT DISTINCT s.combo_id::text FROM app.catalog_combo_slots s
            LEFT JOIN app.catalog_combo_options o ON o.slot_id=s.id
            WHERE o.product_id::text = ANY($1::text[]) OR s.default_product_id::text = ANY($1::text[])`,
			plan.changed["product"])
		if err != nil {
			return plan, err
		}
		if plan.add("product", combos...) == 0 {
			break
		}
	}
	groups, err := identifiers(ctx, tx, `SELECT DISTINCT o.group_id::text FROM app.catalog_group_options o
        JOIN app.catalog_complements c ON c.id=o.complement_id
        WHERE c.product_id::text = ANY($1::text[]) OR c.id::text = ANY($2::text[]) OR o.portion_id::text = ANY($3::text[])
            OR c.ingredient_id::text = ANY($4::text[])`,
		emptyIfNil(plan.changed["product"]), emptyIfNil(changes.changed["complement"]), emptyIfNil(changes.changed["portion"]),
		emptyIfNil(changes.changed["ingredient"]))
	if err != nil {
		return plan, err
	}
	plan.add("group", groups...)
	return plan, nil
}

func emptyIfNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

func identifiers(ctx context.Context, tx pgx.Tx, query string, arguments ...any) ([]string, error) {
	for index, argument := range arguments {
		if ids, ok := argument.([]string); ok && ids == nil {
			arguments[index] = []string{}
		}
	}
	rows, err := tx.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

// syncAvailability evalúa la disponibilidad de cualquier producto, complemento, porción e ingrediente con las mismas
// reglas del cálculo (productAvailabilityReason, selectedOptionAvailable), al momento de la descarga.
type syncAvailability struct {
	now         time.Time
	location    *time.Location
	categories  map[string]Category
	products    map[string]Product
	complements map[string]Complement
	portions    map[string]Portion
	ingredients map[string]Ingredient
}

func loadSyncAvailability(ctx context.Context, tx pgx.Tx, now time.Time, location *time.Location) (*syncAvailability, error) {
	availability := &syncAvailability{
		now:         now,
		location:    location,
		categories:  map[string]Category{},
		products:    map[string]Product{},
		complements: map[string]Complement{},
		portions:    map[string]Portion{},
		ingredients: map[string]Ingredient{},
	}
	for _, resource := range []string{"categories", "complements", "portions", "ingredients"} {
		documents, err := readDocuments(ctx, tx, resource)
		if err != nil {
			return nil, err
		}
		for _, document := range documents {
			switch value := document.(type) {
			case *Category:
				availability.categories[value.ID] = *value
			case *Complement:
				availability.complements[value.ID] = *value
			case *Portion:
				availability.portions[value.ID] = *value
			case *Ingredient:
				availability.ingredients[value.ID] = *value
			}
		}
	}
	// De los productos solo hace falta lo que decide su disponibilidad (también de los archivados).
	rows, err := tx.Query(ctx, `SELECT id::text,name,category_id::text,active,archived,temporary_unavailable,availability,
            sold_out_at,sold_out_until
        FROM app.catalog_products`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var product Product
		var schedule []byte
		if err := rows.Scan(&product.ID, &product.Name, &product.CategoryID, &product.Active, &product.Archived,
			&product.TemporaryUnavailable, &schedule, &product.SoldOutAt, &product.SoldOutUntil); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(schedule, &product.Availability); err != nil {
			return nil, err
		}
		availability.products[product.ID] = product
	}
	return availability, rows.Err()
}

func (availability *syncAvailability) category(id string) Category {
	return availability.categories[id]
}

// productState es el motivo por el que un producto no está disponible ahora («» si lo está) y si está agotado.
func (availability *syncAvailability) productState(id string) (string, bool) {
	product, exists := availability.products[id]
	if !exists {
		return "El producto ya no existe.", false
	}
	reason := productAvailabilityReason(product, availability.categories[product.CategoryID], availability.now,
		availability.location, false)
	return reason, soldOutNow(product, availability.now)
}

// annotateCombo agrega a cada opción de las ranuras de un combo su disponibilidad y si está agotada; si una ranura
// obligatoria no tiene ninguna opción disponible, el combo tampoco lo está.
func (availability *syncAvailability) annotateCombo(view map[string]any, product Product) {
	if product.Kind != "combo" {
		return
	}
	slots, _ := view["slots"].([]any)
	blocked := ""
	for index, entry := range slots {
		slotView, isMap := entry.(map[string]any)
		if !isMap || index >= len(product.Slots) {
			continue
		}
		slot := product.Slots[index]
		options, _ := slotView["options"].([]any)
		available := false
		for position, optionEntry := range options {
			optionView, isOption := optionEntry.(map[string]any)
			if !isOption || position >= len(slot.Options) {
				continue
			}
			reason, soldOut := availability.productState(slot.Options[position].ProductID)
			optionView["available_now"] = reason == ""
			optionView["sold_out"] = soldOut
			available = available || reason == ""
		}
		if slot.MinQuantity > 0 && !available && blocked == "" {
			blocked = slot.Name
		}
	}
	if blocked != "" && view["available_now"] == true {
		view["available_now"] = false
		view["availability_reason"] = "El componente «" + blocked + "» del combo no está disponible."
	}
}

// annotateGroups agrega a cada opción de los grupos su disponibilidad y si su complemento es un producto agotado.
func (availability *syncAvailability) annotateGroups(views any) {
	groups, _ := views.([]any)
	for _, entry := range groups {
		group, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		options, _ := group["options"].([]any)
		for _, optionEntry := range options {
			option, isOption := optionEntry.(map[string]any)
			if !isOption {
				continue
			}
			available, soldOut := availability.optionState(option)
			option["available_now"] = available
			option["sold_out"] = soldOut
		}
	}
}

// optionState aplica a una opción de grupo las reglas de selectedOptionAvailable.
func (availability *syncAvailability) optionState(option map[string]any) (bool, bool) {
	active, _ := option["active"].(bool)
	complementID, _ := option["complement_id"].(string)
	complement, exists := availability.complements[complementID]
	if !active || !exists || !complement.Active {
		return false, false
	}
	if portionID, ok := option["portion_id"].(string); ok && portionID != "" {
		if portion, exists := availability.portions[portionID]; !exists || !portion.Active {
			return false, false
		}
	}
	if complement.IngredientID != nil {
		if ingredient, exists := availability.ingredients[*complement.IngredientID]; !exists || !ingredient.Active {
			return false, false
		}
	}
	if complement.ProductID != nil {
		reason, soldOut := availability.productState(*complement.ProductID)
		return reason == "", soldOut
	}
	return true, false
}
