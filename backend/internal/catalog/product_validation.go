package catalog

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

func hasSchedule(schedule Schedule) bool {
	return schedule.StartsOn != "" || schedule.EndsOn != "" || len(schedule.Days) > 0 || len(schedule.Windows) > 0
}

func validateProduct(ctx context.Context, tx pgx.Tx, product *Product, previous any, actor Actor) error {
	if product.Kind != "product" && product.Kind != "combo" {
		return invalid("Selecciona producto independiente o combo.")
	}
	old, _ := previous.(*Product)
	if err := validateProductPermissions(product, old, actor); err != nil {
		return err
	}
	if product.Kind != "combo" && len(product.Slots) > 0 {
		return invalid("Solo un combo puede tener componentes de combo.")
	}
	if err := reference(ctx, tx, "categories", product.CategoryID); err != nil {
		return err
	}
	if err := optionalReference(ctx, tx, "images", product.ImageID); err != nil {
		return err
	}
	if _, err := parseMoney(product.BasePrice); err != nil {
		return err
	}
	if err := validateSchedule(product.Availability); err != nil {
		return err
	}
	if len(product.Variants) > 100 || len(product.Ingredients) > 100 || len(product.Groups) > 30 || len(product.Slots) > 30 {
		return invalid(
			"Se excedió el límite de configuraciones por producto: 100 presentaciones/ingredientes y 30 grupos/componentes.",
		)
	}
	if err := validateProductIdentifiers(ctx, tx, product); err != nil {
		return err
	}
	seenIngredients := map[string]bool{}
	for _, ingredient := range product.Ingredients {
		if seenIngredients[ingredient.IngredientID] {
			return invalid("Un ingrediente está repetido en el producto.")
		}
		seenIngredients[ingredient.IngredientID] = true
		if err := reference(ctx, tx, "ingredients", ingredient.IngredientID); err != nil {
			return err
		}
		maximum := ingredient.MaxQuantity
		if err := validateBounds(ingredient.IncludedQuantity, &maximum); err != nil {
			return err
		}
		if _, err := parseQuantity(ingredient.RecipeQuantity); err != nil {
			return err
		}
		if _, err := parseMoney(ingredient.ExtraPrice); err != nil {
			return err
		}
	}
	if err := validateBindings(ctx, tx, product.Groups); err != nil {
		return err
	}
	return validateSlots(ctx, tx, product)
}

func validateBindings(ctx context.Context, tx pgx.Tx, bindings []ProductGroup) error {
	seen := map[string]bool{}
	for _, binding := range bindings {
		if seen[binding.GroupID] {
			return invalid("Un grupo está asignado dos veces al mismo producto.")
		}
		seen[binding.GroupID] = true
		if err := reference(ctx, tx, "groups", binding.GroupID); err != nil {
			return err
		}
		document, err := readDocument(ctx, tx, "groups", binding.GroupID)
		if err != nil {
			return err
		}
		group := document.(*OptionGroup)
		minimum, maximum := group.MinQuantity, group.MaxQuantity
		if binding.MinQuantity != nil {
			minimum = *binding.MinQuantity
		}
		if binding.MaxQuantity != nil {
			maximum = binding.MaxQuantity
		}
		if err := validateBounds(minimum, maximum); err != nil {
			return err
		}
		total := decimal.Zero
		defaults := map[string]bool{}
		for _, included := range binding.Included {
			quantity, err := parseQuantity(included.Quantity)
			if err != nil {
				return err
			}
			belongs := false
			for _, option := range group.Options {
				if option.ID == included.OptionID {
					belongs = true
					if option.MaxQuantity != nil && quantity.GreaterThan(amount(*option.MaxQuantity)) {
						return invalid("Una cantidad incluida supera el máximo de su opción.")
					}
				}
			}
			if !belongs || defaults[included.OptionID] {
				return invalid("Una opción incluida está repetida o no pertenece al grupo.")
			}
			defaults[included.OptionID] = true
			total = total.Add(quantity)
		}
		if total.LessThan(amount(minimum)) || (maximum != nil && total.GreaterThan(amount(*maximum))) {
			return invalid("La configuración original del grupo debe respetar sus límites mínimo y máximo.")
		}
	}
	return nil
}

func validateSlots(ctx context.Context, tx pgx.Tx, product *Product) error {
	if product.Kind == "combo" && len(product.Slots) == 0 {
		return invalid("Un combo necesita al menos un componente que reutilice un producto existente.")
	}
	seen := map[string]bool{}
	for index := range product.Slots {
		slot := &product.Slots[index]
		if slot.ID == "" {
			slot.ID = newID()
		}
		if err := validateID(slot.ID); err != nil {
			return err
		}
		invalidIdentity := seen[slot.ID] || slot.Name == "" || len(slot.Name) > 100
		invalidLimits := slot.MinQuantity < 0 || slot.MaxQuantity < 1 || slot.MaxQuantity > 100 || slot.MinQuantity > slot.MaxQuantity
		invalidIncluded := slot.IncludedQuantity < 0 || slot.IncludedQuantity > slot.MaxQuantity
		invalidDefault := slot.DefaultQuantity < slot.MinQuantity || slot.DefaultQuantity > slot.MaxQuantity
		if invalidIdentity || invalidLimits || invalidIncluded || invalidDefault {
			return invalid("Revisa el nombre, identidad y límites del componente de combo.")
		}
		seen[slot.ID] = true
		if len(slot.Options) < 1 || len(slot.Options) > 30 {
			return invalid("Cada componente requiere de 1 a 30 productos permitidos.")
		}
		defaultAllowed := false
		allowed := map[string]bool{}
		for _, option := range slot.Options {
			if option.ProductID == product.ID || allowed[option.ProductID] {
				return invalid("Un combo no puede referenciarse a sí mismo ni repetir una alternativa.")
			}
			allowed[option.ProductID] = true
			if err := reference(ctx, tx, "products", option.ProductID); err != nil {
				return err
			}
			defaultAllowed = defaultAllowed || option.ProductID == slot.DefaultProductID
			if _, err := parseMoney(option.SubstitutionPrice); err != nil {
				return err
			}
			if err := validateTiers(option.Tiers); err != nil {
				return err
			}
			if slot.DefaultQuantity > slot.IncludedQuantity && len(option.Tiers) == 0 {
				return invalid("Las cantidades adicionales originales necesitan tramos de precio explícitos.")
			}
			var cycle bool
			err := tx.QueryRow(ctx, `WITH RECURSIVE children(id,path) AS (
                SELECT $1::uuid,ARRAY[$1::uuid]
                UNION ALL
                SELECT o.product_id,c.path||o.product_id FROM children c
                JOIN app.catalog_combo_slots s ON s.combo_id=c.id
                JOIN app.catalog_combo_options o ON o.slot_id=s.id
                WHERE NOT o.product_id=ANY(c.path) AND array_length(c.path,1)<10
            ) SELECT EXISTS(SELECT FROM children WHERE id=$2 OR array_length(path,1)>=9)`, option.ProductID, product.ID).Scan(&cycle)
			if err != nil {
				return err
			}
			if cycle {
				return invalid("Los combos no pueden formar ciclos ni superar ocho niveles de componentes.")
			}
		}
		if !defaultAllowed {
			return invalid("El producto original del componente debe ser una alternativa permitida.")
		}
	}
	return nil
}
