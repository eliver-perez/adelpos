package catalog

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func restaurantLocation(ctx context.Context, tx pgx.Tx) (*time.Location, error) {
	var name string
	if err := tx.QueryRow(ctx, "SELECT timezone FROM app.establishment").Scan(&name); err != nil {
		return nil, err
	}
	return time.LoadLocation(name)
}

func productAvailabilityReason(product Product, category Category, now time.Time, location *time.Location, override bool) string {
	if !product.Active || product.Archived {
		return "El producto está inactivo o archivado."
	}
	if !category.Active {
		return "La categoría está inactiva."
	}
	if !override {
		if product.TemporaryUnavailable {
			return "El producto no está disponible temporalmente."
		}
		if !scheduleAvailable(product.Availability, now, location) || !scheduleAvailable(category.Availability, now, location) {
			return "El producto o su categoría está fuera de su horario de disponibilidad."
		}
	}
	return ""
}

// Las referencias se entregan por separado para reutilizar grupos sin copiar sus reglas.
func lookups(ctx context.Context, tx pgx.Tx, actor Actor) (any, error) {
	result := map[string]any{}
	for _, resource := range []string{
		"categories",
		"ingredients",
		"complements",
		"portions",
		"groups",
		"price-lists",
	} {
		records, err := readDocuments(ctx, tx, resource)
		if err != nil {
			return nil, err
		}
		result[resource] = RedactPrices(records, actor)
	}
	rows, err := tx.Query(ctx, `SELECT id,name,code,kind,active,archived,
        COALESCE((SELECT jsonb_agg(jsonb_build_object('id',v.id,'name',v.name,'active',v.active) ORDER BY v.name)
        FROM app.catalog_variants v WHERE v.product_id=p.id),'[]')
        FROM app.catalog_products p ORDER BY name,id LIMIT 5001`)
	if err != nil {
		return nil, err
	}
	products := []any{}
	for rows.Next() {
		var id, name, code, kind string
		var active, archived bool
		var variants any
		if err := rows.Scan(

			&id,
			&name,
			&code,
			&kind,
			&active,
			&archived,
			&variants,
		); err != nil {
			rows.Close()
			return nil, err
		}
		products = append(
			products,
			map[string]any{
				"id":       id,
				"name":     name,
				"code":     code,
				"kind":     kind,
				"active":   active,
				"archived": archived,
				"variants": variants,
			},
		)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(products) > 5000 {
		return nil, invalid("El selector admite hasta 5000 productos; reduce el catálogo activo antes de usar este editor.")
	}
	result["products"] = products
	return result, nil
}

func configuration(ctx context.Context, tx pgx.Tx, id string, actor Actor) (any, error) {
	document, err := readDocument(ctx, tx, "products", id)
	if err != nil {
		return nil, err
	}
	product := document.(*Product)
	groups := []any{}
	for _, binding := range product.Groups {
		group, err := readDocument(ctx, tx, "groups", binding.GroupID)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	category, err := readDocument(ctx, tx, "categories", product.CategoryID)
	if err != nil {
		return nil, err
	}
	location, err := restaurantLocation(ctx, tx)
	if err != nil {
		return nil, err
	}
	return RedactPrices(map[string]any{
		"product":  product,
		"groups":   groups,
		"category": category,
		"availability_reason": productAvailabilityReason(

			*product,
			*category.(*Category),
			time.Now(),
			location,
			false,
		),
		"timezone": location.String(),
	}, actor), nil
}

func productState(ctx context.Context, tx pgx.Tx, request Request, id string, audit Audit) (any, error) {
	if err := request.Actor.Require("catalog.products.state"); err != nil {
		return nil, err
	}
	var input struct {
		Revision int  `json:"revision"`
		Active   bool `json:"active"`
		Archived bool `json:"archived"`
	}
	if request.Method != "PATCH" {
		return nil, invalid("El cambio de estado requiere PATCH.")
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	old, err := readDocument(ctx, tx, "products", id)
	if err != nil {
		return nil, err
	}
	product := *old.(*Product)
	if product.Revision != input.Revision {
		return nil, conflict()
	}
	product.Active, product.Archived = input.Active, input.Archived
	if err := saveRecord(

		ctx,
		tx,
		"products",
		product.Metadata,
		[]string{"archived"},
		[]any{product.Archived},
	); err != nil {
		return nil, err
	}
	saved, err := readDocument(ctx, tx, "products", id)
	if err != nil {
		return nil, err
	}
	return RedactPrices(saved, request.Actor), audit(

		ctx,
		tx,
		"catalog.product.state",
		"products",
		&id,
		old,
		saved,
	)
}

func productAvailability(ctx context.Context, tx pgx.Tx, request Request, id string, audit Audit) (any, error) {
	if err := request.Actor.Require("catalog.availability.manage"); err != nil {
		return nil, err
	}
	var input struct {
		Revision             int      `json:"revision"`
		TemporaryUnavailable bool     `json:"temporary_unavailable"`
		Availability         Schedule `json:"availability"`
	}
	if request.Method != "PATCH" {
		return nil, invalid("El cambio de disponibilidad requiere PATCH.")
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateSchedule(input.Availability); err != nil {
		return nil, err
	}
	old, err := readDocument(ctx, tx, "products", id)
	if err != nil {
		return nil, err
	}
	product := *old.(*Product)
	if product.Revision != input.Revision {
		return nil, conflict()
	}
	product.Availability, product.TemporaryUnavailable = input.Availability, input.TemporaryUnavailable
	err = saveRecord(

		ctx,
		tx,
		"products",
		product.Metadata,
		[]string{"availability", "temporary_unavailable"},
		[]any{jsonValue(product.Availability), product.TemporaryUnavailable},
	)
	if err != nil {
		return nil, err
	}
	saved, err := readDocument(ctx, tx, "products", id)
	if err != nil {
		return nil, err
	}
	return RedactPrices(saved, request.Actor), audit(

		ctx,
		tx,
		"catalog.product.availability",
		"products",
		&id,
		old,
		saved,
	)
}

func duplicateProduct(ctx context.Context, tx pgx.Tx, request Request, id string, audit Audit) (any, error) {
	if err := request.Actor.Require("catalog.products.manage"); err != nil {
		return nil, err
	}
	var input struct {
		Revision int    `json:"revision"`
		Name     string `json:"name"`
		Code     string `json:"code"`
	}
	if request.Method != "POST" {
		return nil, invalid("Duplicar requiere POST.")
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	document, err := readDocument(ctx, tx, "products", id)
	if err != nil {
		return nil, err
	}
	product := document.(*Product)
	if product.Revision != input.Revision {
		return nil, conflict()
	}
	product.ID, product.Name, product.Code, product.Barcode, product.Revision = newID(), input.Name, input.Code, nil, 0
	product.Archived = false
	for index := range product.Variants {
		variant := &product.Variants[index]
		variant.ID = newID()
		variant.Code = "VAR-" + variant.ID
		variant.Barcode = nil
	}
	for index := range product.Slots {
		product.Slots[index].ID = newID()
	}
	if err := validateDocument(

		ctx,
		tx,
		product,
		nil,
		request.Actor,
	); err != nil {
		return nil, err
	}
	if err := saveDocument(

		ctx,
		tx,
		"products",
		product,
		true,
	); err != nil {
		return nil, err
	}
	saved, err := readDocument(ctx, tx, "products", product.ID)
	if err != nil {
		return nil, err
	}
	return RedactPrices(saved, request.Actor), audit(

		ctx,
		tx,
		"catalog.product.duplicated",
		"products",
		&product.ID,
		map[string]string{"source_id": id},
		saved,
	)
}

func reorderCategories(ctx context.Context, tx pgx.Tx, request Request, audit Audit) (any, error) {
	if err := request.Actor.Require("catalog.categories.manage"); err != nil {
		return nil, err
	}
	var input struct {
		Items []struct {
			ID        string `json:"id"`
			Revision  int    `json:"revision"`
			SortOrder int    `json:"sort_order"`
		} `json:"items"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if len(input.Items) == 0 || len(input.Items) > 100 {
		return nil, invalid("Reordena entre 1 y 100 categorías a la vez.")
	}
	seen := map[string]bool{}
	for _, item := range input.Items {
		if err := validateID(item.ID); err != nil {
			return nil, err
		}
		if seen[item.ID] {
			return nil, invalid("La categoría aparece dos veces en el orden.")
		}
		seen[item.ID] = true
		result, err := tx.Exec(

			ctx,
			"UPDATE app.catalog_categories SET sort_order=$1,revision=revision+1 WHERE id=$2 AND revision=$3",
			item.SortOrder,
			item.ID,
			item.Revision,
		)
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() != 1 {
			return nil, conflict()
		}
	}
	return map[string]bool{"ok": true}, audit(

		ctx,
		tx,
		"catalog.categories.reordered",
		"categories",
		nil,
		nil,
		input,
	)
}
