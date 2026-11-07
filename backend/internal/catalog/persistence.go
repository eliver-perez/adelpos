package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func saveRecord(ctx context.Context, tx pgx.Tx, resource string, metadata Metadata, columns []string, values []any) error {
	fields := append([]string{
		"id",
		"name",
		"description",
		"active",
	}, columns...)
	arguments := append([]any{
		metadata.ID,
		metadata.Name,
		metadata.Description,
		metadata.Active,
	}, values...)
	placeholders := []string{}
	for index := range fields {
		placeholders = append(placeholders, fmt.Sprintf("$%d", index+1))
	}
	query := "INSERT INTO app." + resourceTables[resource] + " (" + strings.Join(fields, ",") +
		",revision) VALUES (" + strings.Join(placeholders, ",") + ",1)"
	if metadata.Revision > 0 {
		// UPDATE también admite cambios parciales sin intentar insertar campos obligatorios ausentes.
		assignments := []string{}
		for index, field := range fields {
			if field != "id" {
				assignments = append(assignments, fmt.Sprintf("%s=$%d", field, index+1))
			}
		}
		arguments = append(arguments, metadata.Revision)
		query = "UPDATE app." + resourceTables[resource] + " SET " + strings.Join(assignments, ",") +
			fmt.Sprintf(",revision=revision+1,updated_at=now() WHERE id=$1 AND revision=$%d", len(arguments))
	}
	result, err := tx.Exec(ctx, query, arguments...)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return conflict()
	}
	return nil
}

func saveDocument(ctx context.Context, tx pgx.Tx, resource string, document any, creating bool) error {
	switch value := document.(type) {
	case *Category:
		return saveRecord(

			ctx,
			tx,
			resource,
			value.Metadata,
			[]string{
				"sort_order",
				"color",
				"image_id",
				"availability",
			},
			[]any{
				value.SortOrder,
				value.Color,
				value.ImageID,
				jsonValue(value.Availability),
			},
		)
	case *Ingredient:
		return saveRecord(

			ctx,
			tx,
			resource,
			value.Metadata,
			[]string{"unit"},
			[]any{value.Unit},
		)
	case *Complement:
		return saveRecord(

			ctx,
			tx,
			resource,
			value.Metadata,
			[]string{"product_id", "ingredient_id"},
			[]any{value.ProductID, value.IngredientID},
		)
	case *Portion:
		return saveRecord(

			ctx,
			tx,
			resource,
			value.Metadata,
			[]string{"factor"},
			[]any{value.Factor},
		)
	case *OptionGroup:
		return saveGroup(ctx, tx, value)
	case *Product:
		return saveProduct(ctx, tx, value)
	case *PriceList:
		return savePriceList(ctx, tx, value, creating)
	case *Promotion:
		return savePromotion(ctx, tx, value)
	}
	return invalid("Recurso de catálogo no registrado.")
}

func saveGroup(ctx context.Context, tx pgx.Tx, group *OptionGroup) error {
	err := saveRecord(

		ctx,
		tx,
		"groups",
		group.Metadata,
		[]string{"pricing_mode", "min_quantity", "max_quantity"},
		[]any{group.PricingMode, group.MinQuantity, group.MaxQuantity},
	)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE app.catalog_group_options SET active=false WHERE group_id=$1", group.ID); err != nil {
		return err
	}
	for _, option := range group.Options {
		var belongsElsewhere bool
		if err := tx.QueryRow(
			ctx,
			"SELECT EXISTS(SELECT FROM app.catalog_group_options WHERE id=$1 AND group_id<>$2)",
			option.ID,
			group.ID,
		).Scan(&belongsElsewhere); err != nil {
			return err
		}
		if belongsElsewhere {
			return invalid("Una opción pertenece a otro grupo y no puede cambiar de identidad.")
		}
		_, err := tx.Exec(

			ctx,
			`INSERT INTO app.catalog_group_options(id,group_id,complement_id,portion_id,reference_value,extra_price,max_quantity,active)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(id) DO UPDATE SET complement_id=excluded.complement_id,
            portion_id=excluded.portion_id,reference_value=excluded.reference_value,extra_price=excluded.extra_price,
            max_quantity=excluded.max_quantity,active=excluded.active`,
			option.ID,
			group.ID,
			option.ComplementID,
			option.PortionID,
			option.ReferenceValue,
			option.ExtraPrice,
			option.MaxQuantity,
			option.Active,
		)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM app.catalog_option_tiers WHERE option_id=$1", option.ID); err != nil {
			return err
		}
		for _, tier := range option.Tiers {
			if _, err := tx.Exec(

				ctx,
				"INSERT INTO app.catalog_option_tiers(option_id,from_quantity,unit_price) VALUES ($1,$2,$3)",
				option.ID,
				tier.From,
				tier.UnitPrice,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func savePriceList(ctx context.Context, tx pgx.Tx, value *PriceList, creating bool) error {
	if !creating && !value.Default {
		var currentlyDefault bool
		if err := tx.QueryRow(ctx, "SELECT is_default FROM app.catalog_price_lists WHERE id=$1", value.ID).Scan(&currentlyDefault); err != nil {
			return err
		}
		if currentlyDefault {
			return invalid(
				"Selecciona primero otra lista como predeterminada; siempre debe existir una lista activa predeterminada.",
			)
		}
	}
	if value.Default {
		// También cambia su revisión: otro editor no puede restablecer un predeterminado obsoleto.
		if _, err := tx.Exec(
			ctx,
			"UPDATE app.catalog_price_lists SET is_default=false,revision=revision+1 WHERE is_default AND id<>$1",
			value.ID,
		); err != nil {
			return err
		}
	}
	err := saveRecord(

		ctx,
		tx,
		"price-lists",
		value.Metadata,
		[]string{"is_default", "fallback", "allow_promotions"},
		[]any{value.Default, value.Fallback, value.AllowPromotions},
	)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM app.catalog_price_entries WHERE list_id=$1", value.ID); err != nil {
		return err
	}
	for _, entry := range value.Entries {
		if _, err := tx.Exec(

			ctx,
			"INSERT INTO app.catalog_price_entries(list_id,product_id,variant_id,price) VALUES ($1,$2,$3,$4)",
			value.ID,
			entry.ProductID,
			entry.VariantID,
			entry.Price,
		); err != nil {
			return err
		}
	}
	return nil
}

func savePromotion(ctx context.Context, tx pgx.Tx, value *Promotion) error {
	err := saveRecord(

		ctx,
		tx,
		"promotions",
		value.Metadata,
		[]string{
			"kind",
			"percent",
			"special_price",
			"buy_quantity",
			"pay_quantity",
			"minimum_quantity",
			"priority",
			"stackable",
			"include_extras",
			"availability",
		},
		[]any{
			value.Kind,
			value.Percent,
			value.SpecialPrice,
			value.BuyQuantity,
			value.PayQuantity,
			value.MinimumQuantity,
			value.Priority,
			value.Stackable,
			value.IncludeExtras,
			jsonValue(value.Availability),
		},
	)
	if err != nil {
		return err
	}
	tables := []struct {
		name, column string
		ids          []string
	}{
		{"catalog_promotion_products", "product_id", value.ProductIDs},
		{"catalog_promotion_categories", "category_id", value.CategoryIDs},
		{"catalog_promotion_lists", "list_id", value.PriceListIDs},
	}
	for _, relation := range tables {
		if _, err := tx.Exec(ctx, "DELETE FROM app."+relation.name+" WHERE promotion_id=$1", value.ID); err != nil {
			return err
		}
		for _, id := range relation.ids {
			if _, err := tx.Exec(
				ctx,
				"INSERT INTO app."+relation.name+"(promotion_id,"+relation.column+") VALUES ($1,$2)",
				value.ID,
				id,
			); err != nil {
				return err
			}
		}
	}
	return nil
}
