package catalog

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func preview(ctx context.Context, tx pgx.Tx, request Request, audit Audit) (any, error) {
	var input PreviewRequest
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if len(input.Lines) < 1 || len(input.Lines) > 20 {
		return nil, invalid("La previsualización admite entre 1 y 20 productos.")
	}
	if input.OverrideAvailability {
		if err := request.Actor.Require("catalog.availability.override"); err != nil {
			return nil, err
		}
		if len(strings.TrimSpace(input.OverrideReason)) < 5 || len(input.OverrideReason) > 500 {
			return nil, invalid("Explica la excepción de disponibilidad, entre 5 y 500 caracteres.")
		}
	}
	if err := validateOptionalID(input.PriceListID); err != nil {
		return nil, err
	}
	snapshot, err := loadSnapshot(ctx, tx, input)
	if err != nil {
		return nil, err
	}
	location, err := restaurantLocation(ctx, tx)
	if err != nil {
		return nil, err
	}
	quote, err := Calculate(snapshot, input, time.Now(), location)
	if err != nil {
		return nil, err
	}
	if input.OverrideAvailability {
		if err := audit(

			ctx,
			tx,
			"catalog.availability.overridden",
			"preview",
			nil,
			nil,
			map[string]any{"reason": input.OverrideReason, "lines": input.Lines},
		); err != nil {
			return nil, err
		}
	}
	return quote, nil
}

func loadSnapshot(ctx context.Context, tx pgx.Tx, input PreviewRequest) (Snapshot, error) {
	snapshot := Snapshot{
		Products:    map[string]Product{},
		Categories:  map[string]Category{},
		Ingredients: map[string]Ingredient{},
		Complements: map[string]Complement{},
		Portions:    map[string]Portion{},
		Groups:      map[string]OptionGroup{},
	}
	var listID string
	if input.PriceListID != nil {
		listID = *input.PriceListID
	} else if err := tx.QueryRow(ctx, "SELECT id FROM app.catalog_price_lists WHERE is_default").Scan(&listID); err != nil {
		return snapshot, err
	}
	list, err := readDocument(ctx, tx, "price-lists", listID)
	if err != nil {
		return snapshot, err
	}
	snapshot.PriceList = *list.(*PriceList)
	for _, resource := range []string{
		"categories",
		"ingredients",
		"complements",
		"portions",
		"groups",
		"promotions",
	} {
		documents, err := readDocuments(ctx, tx, resource)
		if err != nil {
			return snapshot, err
		}
		for _, document := range documents {
			switch value := document.(type) {
			case *Category:
				snapshot.Categories[value.ID] = *value
			case *Ingredient:
				snapshot.Ingredients[value.ID] = *value
			case *Complement:
				snapshot.Complements[value.ID] = *value
			case *Portion:
				snapshot.Portions[value.ID] = *value
			case *OptionGroup:
				snapshot.Groups[value.ID] = *value
			case *Promotion:
				snapshot.Promotions = append(snapshot.Promotions, *value)
			}
		}
	}
	for _, complement := range snapshot.Complements {
		if complement.ProductID != nil {
			document, err := readDocument(ctx, tx, "products", *complement.ProductID)
			if err != nil {
				return snapshot, err
			}
			snapshot.Products[*complement.ProductID] = *document.(*Product)
		}
	}
	remaining := 1000
	for _, line := range input.Lines {
		if err := loadSelectedProduct(

			ctx,
			tx,
			&snapshot,
			line.ProductID,
			line.Customization,
			0,
			&remaining,
		); err != nil {
			return snapshot, err
		}
	}
	if err := loadQuickNotes(ctx, tx, &snapshot, input.Lines); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func loadSelectedProduct(
	ctx context.Context,
	tx pgx.Tx,
	snapshot *Snapshot,
	id string,
	customization Customization,
	depth int,
	remaining *int,
) error {
	*remaining--
	if *remaining < 0 {
		return invalid("La configuración expande demasiados componentes para una previsualización.")
	}
	if depth > 8 || len(snapshot.Products) > 200 {
		return invalid("La personalización supera ocho niveles o 200 productos referenciados.")
	}
	if err := validateID(id); err != nil {
		return err
	}
	product, exists := snapshot.Products[id]
	if !exists {
		document, err := readDocument(ctx, tx, "products", id)
		if err != nil {
			return err
		}
		product = *document.(*Product)
		snapshot.Products[id] = product
	}
	for _, slot := range product.Slots {
		selected := ComponentSelection{ProductID: slot.DefaultProductID, Quantity: slot.DefaultQuantity}
		for _, component := range customization.Components {
			if component.SlotID == slot.ID {
				selected = component
			}
		}
		if selected.Quantity > 0 {
			if err := loadSelectedProduct(

				ctx,
				tx,
				snapshot,
				selected.ProductID,
				selected.Customization,
				depth+1,
				remaining,
			); err != nil {
				return err
			}
		}
	}
	return nil
}
