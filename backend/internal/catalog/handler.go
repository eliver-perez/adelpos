package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var writePermissions = map[string]string{
	"categories":  "catalog.categories.manage",
	"products":    "catalog.products.manage",
	"ingredients": "catalog.ingredients.manage",
	"complements": "catalog.complements.manage",
	"portions":    "catalog.complements.manage",
	"groups":      "catalog.groups.manage",
	"price-lists": "catalog.price_lists.manage",
	"promotions":  "catalog.promotions.manage",
}

func Handle(ctx context.Context, tx pgx.Tx, request Request, audit Audit) (any, error) {
	if err := request.Actor.Require("catalog.read"); err != nil {
		return nil, err
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(request.Route, "catalog/"), "/"), "/")
	readOnly := request.Method == "GET" || (len(parts) == 1 && parts[0] == "preview")
	lock := "SELECT pg_advisory_xact_lock(7820148304)"
	if readOnly {
		lock = "SELECT pg_advisory_xact_lock_shared(7820148304)"
	}
	// Un snapshot de cálculo no mezcla revisiones mientras otra terminal edita relaciones.
	if _, err := tx.Exec(ctx, lock); err != nil {
		return nil, err
	}
	if len(parts) == 1 && parts[0] == "preview" && request.Method == "POST" {
		if err := request.Actor.Require("catalog.prices.read"); err != nil {
			return nil, err
		}
		return preview(ctx, tx, request, audit)
	}
	if parts[0] == "images" {
		return images(

			ctx,
			tx,
			request,
			parts,
			audit,
		)
	}
	if len(parts) == 1 && parts[0] == "lookups" && request.Method == "GET" {
		return lookups(ctx, tx, request.Actor)
	}
	resource := parts[0]
	if _, exists := documentQueries[resource]; !exists {
		return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "Ruta de catálogo no disponible."}
	}
	if len(parts) == 1 && request.Method == "GET" {
		return listPage(ctx, tx, resource, request)
	}
	if resource == "categories" && len(parts) == 2 && parts[1] == "reorder" && request.Method == "POST" {
		return reorderCategories(ctx, tx, request, audit)
	}
	if len(parts) > 1 {
		if err := validateID(parts[1]); err != nil {
			return nil, err
		}
	}
	if len(parts) == 2 && request.Method == "GET" {
		document, err := readDocument(ctx, tx, resource, parts[1])
		return RedactPrices(document, request.Actor), err
	}
	if resource == "products" && len(parts) == 3 {
		switch parts[2] {
		case "configuration":
			if request.Method == "GET" {
				return configuration(ctx, tx, parts[1], request.Actor)
			}
		case "state":
			return productState(

				ctx,
				tx,
				request,
				parts[1],
				audit,
			)
		case "availability":
			return productAvailability(

				ctx,
				tx,
				request,
				parts[1],
				audit,
			)
		case "duplicate":
			return duplicateProduct(

				ctx,
				tx,
				request,
				parts[1],
				audit,
			)
		}
	}
	if err := request.Actor.Require(writePermissions[resource]); err != nil {
		return nil, err
	}
	if (request.Method != "POST" || len(parts) != 1) && (request.Method != "PATCH" || len(parts) != 2) {
		return nil, &Error{Status: 405, Code: "METHOD_NOT_ALLOWED", Message: "Método no permitido para este recurso."}
	}
	document := newDocument(resource)
	if err := decode(request.Body, document); err != nil {
		return nil, err
	}
	metadata := metadataOf(document)
	var previous any
	action := "catalog." + resource + ".created"
	if request.Method == "PATCH" {
		current, err := readDocument(ctx, tx, resource, parts[1])
		if err != nil {
			return nil, err
		}
		if metadata.Revision != metadataOf(current).Revision || (metadata.ID != "" && metadata.ID != parts[1]) {
			return nil, conflict()
		}
		previous = current
		metadata.ID = parts[1]
		action = "catalog." + resource + ".updated"
	} else {
		if metadata.ID != "" || metadata.Revision != 0 {
			return nil, invalid("Los registros nuevos no deben reutilizar identificadores ni revisiones.")
		}
		metadata.ID = newID()
	}
	if err := validateDocument(

		ctx,
		tx,
		document,
		previous,
		request.Actor,
	); err != nil {
		return nil, err
	}
	var displacedDefault *PriceList
	if list, ok := document.(*PriceList); ok && list.Default {
		lists, err := readDocuments(ctx, tx, "price-lists")
		if err != nil {
			return nil, err
		}
		for _, record := range lists {
			current := record.(*PriceList)
			if current.Default && current.ID != list.ID {
				displacedDefault = current
			}
		}
	}
	if err := saveDocument(

		ctx,
		tx,
		resource,
		document,
		previous == nil,
	); err != nil {
		return nil, err
	}
	saved, err := readDocument(ctx, tx, resource, metadata.ID)
	if err != nil {
		return nil, err
	}
	if err := audit(

		ctx,
		tx,
		action,
		resource,
		&metadata.ID,
		previous,
		saved,
	); err != nil {
		return nil, err
	}
	if displacedDefault != nil {
		current, err := readDocument(ctx, tx, "price-lists", displacedDefault.ID)
		if err != nil {
			return nil, err
		}
		if err := audit(

			ctx,
			tx,
			"catalog.price-list.default_replaced",
			"price-lists",
			&displacedDefault.ID,
			displacedDefault,
			current,
		); err != nil {
			return nil, err
		}
	}
	return RedactPrices(saved, request.Actor), nil
}

func pagination(query map[string][]string) (int, int, error) {
	page, size := 1, 25
	for key, target := range map[string]*int{"page": &page, "page_size": &size} {
		if len(query[key]) > 0 && query[key][0] != "" {
			value, err := strconv.Atoi(query[key][0])
			if err != nil || value < 1 {
				return 0, 0, invalid("La página y su tamaño deben ser enteros positivos.")
			}
			*target = value
		}
	}
	if page > 10000 || size > 100 {
		return 0, 0, invalid("La página máxima es 10000 y el tamaño máximo es 100.")
	}
	return page, size, nil
}

func listPage(ctx context.Context, tx pgx.Tx, resource string, request Request) (any, error) {
	page, size, err := pagination(request.Query)
	if err != nil {
		return nil, err
	}
	aliases := map[string]string{
		"categories":  "c",
		"products":    "p",
		"ingredients": "i",
		"complements": "c",
		"portions":    "p",
		"groups":      "g",
		"price-lists": "l",
		"promotions":  "p",
	}
	alias := aliases[resource]
	clauses := []string{"true"}
	arguments := []any{}
	add := func(expression string, value any) {
		arguments = append(arguments, value)
		clauses = append(clauses, fmt.Sprintf(expression, len(arguments)))
	}
	if search := strings.TrimSpace(request.Query.Get("search")); search != "" {
		if len(search) > 100 {
			return nil, invalid("La búsqueda admite hasta 100 caracteres.")
		}
		add(alias+".name ILIKE $%d", "%"+search+"%")
	}
	if active := request.Query.Get("active"); active != "" {
		if active != "true" && active != "false" {
			return nil, invalid("El filtro de estado no es válido.")
		}
		add(alias+".active=$%d", active == "true")
	}
	if resource == "products" {
		if category := request.Query.Get("category_id"); category != "" {
			if err := validateID(category); err != nil {
				return nil, err
			}
			add("p.category_id=$%d", category)
		}
		if kind := request.Query.Get("kind"); kind != "" {
			if kind != "product" && kind != "combo" {
				return nil, invalid("Tipo de producto inválido.")
			}
			add("p.kind=$%d", kind)
		}
		if request.Query.Get("archived") != "all" {
			add("p.archived=$%d", request.Query.Get("archived") == "true")
		}
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM app."+resourceTables[resource]+" "+alias+where, arguments...).Scan(&total); err != nil {
		return nil, err
	}
	order := " ORDER BY " + alias + ".name," + alias + ".id"
	if resource == "products" || resource == "categories" {
		order = " ORDER BY " + alias + ".sort_order," + alias + ".name," + alias + ".id"
	}
	query := documentQueries[resource] + where + order + fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(arguments)+1, len(arguments)+2)
	arguments = append(arguments, size, (page-1)*size)
	rows, err := tx.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	items := []any{}
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			rows.Close()
			return nil, err
		}
		document := newDocument(resource)
		if err := json.Unmarshal(encoded, document); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, RedactPrices(document, request.Actor))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if resource == "products" {
		for index, item := range items {
			value := item.(map[string]any)
			id, _ := value["id"].(string)
			productDoc, err := readDocument(ctx, tx, "products", id)
			if err != nil {
				return nil, err
			}
			product := productDoc.(*Product)
			categoryDoc, err := readDocument(ctx, tx, "categories", product.CategoryID)
			if err != nil {
				return nil, err
			}
			location, err := restaurantLocation(ctx, tx)
			if err != nil {
				return nil, err
			}
			reason := productAvailabilityReason(

				*product,
				*categoryDoc.(*Category),
				time.Now(),
				location,
				false,
			)
			value["available_now"] = reason == ""
			value["availability_reason"] = reason
			items[index] = value
		}
	}
	return map[string]any{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": size,
	}, nil
}

// RedactPrices aplica la misma restricción monetaria al catálogo y a sus snapshots de auditoría.
func RedactPrices(document any, actor Actor) any {
	if document == nil {
		return nil
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(jsonValue(document))))
	decoder.UseNumber()
	_ = decoder.Decode(&value)
	if !actor.Can("catalog.prices.read") {
		removePrices(value)
	}
	return value
}

func removePrices(value any) {
	switch record := value.(type) {
	case map[string]any:
		for key, child := range record {
			switch key {
			case "base_price", "price", "reference_value", "extra_price", "substitution_price", "unit_price", "special_price":
				delete(record, key)
			default:
				removePrices(child)
			}
		}
	case []any:
		for _, child := range record {
			removePrices(child)
		}
	}
}
