package catalog

import (
	"context"
	"reflect"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

var resourceTables = map[string]string{
	"categories":  "catalog_categories",
	"products":    "catalog_products",
	"ingredients": "catalog_ingredients",
	"complements": "catalog_complements",
	"portions":    "catalog_portions",
	"groups":      "catalog_option_groups",
	"price-lists": "catalog_price_lists",
	"promotions":  "catalog_promotions",
	"images":      "catalog_images",
}

func metadataOf(document any) *Metadata {
	switch value := document.(type) {
	case *Category:
		return &value.Metadata
	case *Product:
		return &value.Metadata
	case *Ingredient:
		return &value.Metadata
	case *Complement:
		return &value.Metadata
	case *Portion:
		return &value.Metadata
	case *OptionGroup:
		return &value.Metadata
	case *PriceList:
		return &value.Metadata
	case *Promotion:
		return &value.Metadata
	}
	panic("tipo de catálogo no registrado")
}

func reference(ctx context.Context, tx pgx.Tx, resource, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app."+resourceTables[resource]+" WHERE id=$1)", id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return invalid("El registro relacionado de " + resource + " no existe.")
	}
	return nil
}

func optionalReference(ctx context.Context, tx pgx.Tx, resource string, id *string) error {
	if id != nil {
		return reference(ctx, tx, resource, *id)
	}
	return nil
}

func validateBounds(minimum string, maximum *string) error {
	min, err := parseQuantity(minimum)
	if err != nil {
		return err
	}
	if maximum != nil {
		max, err := parseQuantity(*maximum)
		if err != nil {
			return err
		}
		if max.LessThan(min) {
			return invalid("La cantidad máxima no puede ser menor que la mínima.")
		}
	}
	return nil
}

func validateTiers(tiers []QuantityTier) error {
	if len(tiers) > 100 {
		return invalid("Se permiten hasta 100 tramos de precio.")
	}
	sort.Slice(tiers, func(first, second int) bool {
		return tiers[first].From < tiers[second].From
	})
	previous := 0
	for _, tier := range tiers {
		if tier.From <= previous || tier.From > 1000 || (previous == 0 && tier.From != 1) {
			return invalid("Los tramos deben empezar en la unidad 1, ser únicos y estar ordenados.")
		}
		if _, err := parseMoney(tier.UnitPrice); err != nil {
			return err
		}
		previous = tier.From
	}
	return nil
}

func validateDocument(ctx context.Context, tx pgx.Tx, document any, previous any, actor Actor) error {
	if err := validateMetadata(metadataOf(document)); err != nil {
		return err
	}
	switch value := document.(type) {
	case *Category:
		old, _ := previous.(*Category)
		if err := validateCategoryColor(value, old); err != nil {
			return err
		}
		if err := optionalReference(ctx, tx, "images", value.ImageID); err != nil {
			return err
		}
		var oldSchedule Schedule
		var oldImage *string
		if old != nil {
			oldSchedule = old.Availability
			oldImage = old.ImageID
		}
		if !reflect.DeepEqual(oldImage, value.ImageID) {
			if err := actor.Require("catalog.images.manage"); err != nil {
				return err
			}
		}
		if !reflect.DeepEqual(jsonValue(oldSchedule), jsonValue(value.Availability)) && (hasSchedule(oldSchedule) || hasSchedule(value.Availability)) {
			if err := actor.Require("catalog.availability.manage"); err != nil {
				return err
			}
		}
		return validateSchedule(value.Availability)
	case *Ingredient:
		value.Unit = strings.TrimSpace(value.Unit)
		if value.Unit == "" || len(value.Unit) > 30 {
			return invalid("Indica la unidad del ingrediente, hasta 30 caracteres.")
		}
	case *Complement:
		if value.ProductID != nil && value.IngredientID != nil {
			return invalid("Relaciona el complemento con un producto o un ingrediente, no ambos.")
		}
		if err := optionalReference(ctx, tx, "products", value.ProductID); err != nil {
			return err
		}
		return optionalReference(ctx, tx, "ingredients", value.IngredientID)
	case *Portion:
		factor, err := parseQuantity(value.Factor)
		if err != nil {
			return err
		}
		if !factor.IsPositive() {
			return invalid("El factor de porción debe ser mayor que cero.")
		}
	case *OptionGroup:
		return validateGroup(ctx, tx, value, actor)
	case *Product:
		return validateProduct(

			ctx,
			tx,
			value,
			previous,
			actor,
		)
	case *PriceList:
		return validatePriceList(ctx, tx, value, actor)
	case *Promotion:
		return validatePromotion(ctx, tx, value, actor)
	}
	return nil
}

func validateGroup(ctx context.Context, tx pgx.Tx, group *OptionGroup, actor Actor) error {
	if err := actor.Require("catalog.prices.manage"); err != nil {
		return err
	}
	if group.PricingMode != "value" && group.PricingMode != "quantity" {
		return invalid("El grupo debe usar equivalencia por valor o precio por cantidad.")
	}
	if err := validateBounds(group.MinQuantity, group.MaxQuantity); err != nil {
		return err
	}
	if len(group.Options) == 0 || len(group.Options) > 100 {
		return invalid("Un grupo necesita de 1 a 100 opciones.")
	}
	seen := map[string]bool{}
	for index := range group.Options {
		option := &group.Options[index]
		if option.ID == "" {
			option.ID = newID()
		}
		if err := validateID(option.ID); err != nil {
			return err
		}
		if seen[option.ID] {
			return invalid("Una opción está repetida.")
		}
		seen[option.ID] = true
		if err := reference(ctx, tx, "complements", option.ComplementID); err != nil {
			return err
		}
		if err := optionalReference(ctx, tx, "portions", option.PortionID); err != nil {
			return err
		}
		for _, price := range []string{option.ReferenceValue, option.ExtraPrice} {
			if _, err := parseMoney(price); err != nil {
				return err
			}
		}
		if err := validateBounds("0", option.MaxQuantity); err != nil {
			return err
		}
		if err := validateTiers(option.Tiers); err != nil {
			return err
		}
	}
	return nil
}

func validatePriceList(ctx context.Context, tx pgx.Tx, value *PriceList, actor Actor) error {
	if err := actor.Require("catalog.prices.manage"); err != nil {
		return err
	}
	if value.Fallback != "base" && value.Fallback != "reject" {
		return invalid("Selecciona respaldo al precio base o rechazo cuando no exista precio en la lista.")
	}
	if value.Default && !value.Active {
		return invalid("La lista predeterminada debe permanecer activa.")
	}
	if len(value.Entries) > 5000 {
		return invalid("La lista supera las 5000 entradas por edición.")
	}
	for _, entry := range value.Entries {
		if err := reference(ctx, tx, "products", entry.ProductID); err != nil {
			return err
		}
		if _, err := parseMoney(entry.Price); err != nil {
			return err
		}
		if entry.VariantID != nil {
			if err := validateID(*entry.VariantID); err != nil {
				return err
			}
			var belongs bool
			if err := tx.QueryRow(
				ctx,
				"SELECT EXISTS(SELECT FROM app.catalog_variants WHERE id=$1 AND product_id=$2)",
				*entry.VariantID,
				entry.ProductID,
			).Scan(&belongs); err != nil {
				return err
			}
			if !belongs {
				return invalid("La presentación no pertenece al producto de esta entrada de precio.")
			}
		}
	}
	return nil
}

func validatePromotion(ctx context.Context, tx pgx.Tx, value *Promotion, actor Actor) error {
	if err := actor.Require("catalog.prices.manage"); err != nil {
		return err
	}
	if value.Kind != "two_for_one" && value.Kind != "percent" && value.Kind != "quantity" && value.Kind != "special_price" {
		return invalid("Tipo de promoción desconocido.")
	}
	percent, err := parseMoney(value.Percent)
	if err != nil {
		return err
	}
	if percent.GreaterThan(decimal.NewFromInt(100)) || (value.Kind == "percent" && !percent.IsPositive()) {
		return invalid("El porcentaje debe ser mayor que cero y hasta 100.")
	}
	if _, err := parseMoney(value.SpecialPrice); err != nil {
		return err
	}
	if value.Kind == "two_for_one" {
		value.BuyQuantity, value.PayQuantity = 2, 1
	}
	invalidBuy := value.BuyQuantity < 1 || value.BuyQuantity > 1000
	invalidPay := value.PayQuantity < 0 || value.PayQuantity >= value.BuyQuantity
	invalidMinimum := value.MinimumQuantity < 1 || value.MinimumQuantity > 1000
	if invalidBuy || invalidPay || invalidMinimum {
		return invalid("Las cantidades de la promoción no son válidas.")
	}
	if len(value.ProductIDs)+len(value.CategoryIDs) == 0 {
		return invalid("Selecciona al menos un producto o una categoría participante.")
	}
	if err := validateSchedule(value.Availability); err != nil {
		return err
	}
	references := map[string][]string{"products": value.ProductIDs, "categories": value.CategoryIDs, "price-lists": value.PriceListIDs}
	for resource, ids := range references {
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				return invalid("Un participante de la promoción está repetido.")
			}
			seen[id] = true
			if err := reference(ctx, tx, resource, id); err != nil {
				return err
			}
		}
	}
	return nil
}
