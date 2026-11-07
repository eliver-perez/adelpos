package catalog

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func saveProduct(ctx context.Context, tx pgx.Tx, product *Product) error {
	err := saveRecord(

		ctx,
		tx,
		"products",
		product.Metadata,
		[]string{
			"category_id",
			"code",
			"barcode",
			"kind",
			"base_price",
			"image_id",
			"sort_order",
			"archived",
			"temporary_unavailable",
			"availability",
		},
		[]any{
			product.CategoryID,
			product.Code,
			product.Barcode,
			product.Kind,
			product.BasePrice,
			product.ImageID,
			product.SortOrder,
			product.Archived,
			product.TemporaryUnavailable,
			jsonValue(product.Availability),
		},
	)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE app.catalog_variants SET active=false WHERE product_id=$1", product.ID); err != nil {
		return err
	}
	for _, variant := range product.Variants {
		var belongsElsewhere bool
		if err := tx.QueryRow(
			ctx,
			"SELECT EXISTS(SELECT FROM app.catalog_variants WHERE id=$1 AND product_id<>$2)",
			variant.ID,
			product.ID,
		).Scan(&belongsElsewhere); err != nil {
			return err
		}
		if belongsElsewhere {
			return invalid("Una presentación pertenece a otro producto.")
		}
		_, err := tx.Exec(

			ctx,
			`INSERT INTO app.catalog_variants(id,product_id,name,code,barcode,price,active,availability)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(id) DO UPDATE SET name=excluded.name,code=excluded.code,
            barcode=excluded.barcode,price=excluded.price,active=excluded.active,availability=excluded.availability`,
			variant.ID,
			product.ID,
			variant.Name,
			variant.Code,
			variant.Barcode,
			variant.Price,
			variant.Active,
			jsonValue(variant.Availability),
		)
		if err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, "DELETE FROM app.catalog_product_ingredients WHERE product_id=$1", product.ID); err != nil {
		return err
	}
	for _, ingredient := range product.Ingredients {
		_, err := tx.Exec(

			ctx,
			`INSERT INTO app.catalog_product_ingredients(product_id,ingredient_id,included_quantity,removable,addable,max_quantity,extra_price,recipe_quantity)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			product.ID,
			ingredient.IngredientID,
			ingredient.IncludedQuantity,
			ingredient.Removable,
			ingredient.Addable,
			ingredient.MaxQuantity,
			ingredient.ExtraPrice,
			ingredient.RecipeQuantity,
		)
		if err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, "DELETE FROM app.catalog_group_defaults WHERE product_id=$1", product.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM app.catalog_product_groups WHERE product_id=$1", product.ID); err != nil {
		return err
	}
	for _, binding := range product.Groups {
		if _, err := tx.Exec(

			ctx,
			"INSERT INTO app.catalog_product_groups(product_id,group_id,min_quantity,max_quantity) VALUES ($1,$2,$3,$4)",
			product.ID,
			binding.GroupID,
			binding.MinQuantity,
			binding.MaxQuantity,
		); err != nil {
			return err
		}
		for _, included := range binding.Included {
			if _, err := tx.Exec(

				ctx,
				"INSERT INTO app.catalog_group_defaults(product_id,group_id,option_id,quantity) VALUES ($1,$2,$3,$4)",
				product.ID,
				binding.GroupID,
				included.OptionID,
				included.Quantity,
			); err != nil {
				return err
			}
		}
	}
	return saveSlots(ctx, tx, product)
}

func saveSlots(ctx context.Context, tx pgx.Tx, product *Product) error {
	deletes := []string{
		"DELETE FROM app.catalog_combo_tiers t USING app.catalog_combo_slots s WHERE t.slot_id=s.id AND s.combo_id=$1",
		"DELETE FROM app.catalog_combo_options o USING app.catalog_combo_slots s WHERE o.slot_id=s.id AND s.combo_id=$1",
		"DELETE FROM app.catalog_combo_slots WHERE combo_id=$1",
	}
	for _, query := range deletes {
		if _, err := tx.Exec(ctx, query, product.ID); err != nil {
			return err
		}
	}
	for _, slot := range product.Slots {
		_, err := tx.Exec(

			ctx,
			`INSERT INTO app.catalog_combo_slots(id,combo_id,name,min_quantity,max_quantity,included_quantity,default_quantity,default_product_id)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			slot.ID,
			product.ID,
			slot.Name,
			slot.MinQuantity,
			slot.MaxQuantity,
			slot.IncludedQuantity,
			slot.DefaultQuantity,
			slot.DefaultProductID,
		)
		if err != nil {
			return err
		}
		for _, option := range slot.Options {
			if _, err := tx.Exec(

				ctx,
				"INSERT INTO app.catalog_combo_options(slot_id,product_id,substitution_price) VALUES ($1,$2,$3)",
				slot.ID,
				option.ProductID,
				option.SubstitutionPrice,
			); err != nil {
				return err
			}
			for _, tier := range option.Tiers {
				if _, err := tx.Exec(

					ctx,
					"INSERT INTO app.catalog_combo_tiers(slot_id,product_id,from_quantity,unit_price) VALUES ($1,$2,$3,$4)",
					slot.ID,
					option.ProductID,
					tier.From,
					tier.UnitPrice,
				); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
