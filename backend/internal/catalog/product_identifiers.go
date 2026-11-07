package catalog

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

func validateProductIdentifiers(ctx context.Context, tx pgx.Tx, product *Product) error {
	product.Code = strings.TrimSpace(product.Code)
	codes := map[string]string{}
	barcodes := map[string]string{}
	register := func(code string, barcode *string, ownerID string) error {
		if code == "" || len(code) > 60 || codes[code] != "" {
			return invalid("Cada producto y presentación necesita un código interno único de hasta 60 caracteres.")
		}
		codes[code] = ownerID
		if barcode != nil {
			if strings.TrimSpace(*barcode) == "" || len(*barcode) > 100 || barcodes[*barcode] != "" {
				return invalid("El código de barras debe ser único y tener hasta 100 caracteres.")
			}
			barcodes[*barcode] = ownerID
		}
		return nil
	}
	if err := register(product.Code, product.Barcode, product.ID); err != nil {
		return err
	}
	seenIDs := map[string]bool{}
	for index := range product.Variants {
		variant := &product.Variants[index]
		if variant.ID == "" {
			variant.ID = newID()
		}
		if err := validateID(variant.ID); err != nil {
			return err
		}
		variant.Name, variant.Code = strings.TrimSpace(variant.Name), strings.TrimSpace(variant.Code)
		if seenIDs[variant.ID] || variant.Name == "" || len(variant.Name) > 100 {
			return invalid("Las presentaciones requieren nombres y no pueden repetir su identidad.")
		}
		seenIDs[variant.ID] = true
		if err := register(variant.Code, variant.Barcode, variant.ID); err != nil {
			return err
		}
		if _, err := parseMoney(variant.Price); err != nil {
			return err
		}
		if err := validateSchedule(variant.Availability); err != nil {
			return err
		}
	}
	// Los códigos de registros inactivos siguen reservados para conservar su identidad histórica.
	for field, owners := range map[string]map[string]string{"code": codes, "barcode": barcodes} {
		for value, ownerID := range owners {
			var used bool
			query := "SELECT EXISTS(SELECT FROM app.catalog_products WHERE " + field + "=$1 AND id<>$2) " +
				"OR EXISTS(SELECT FROM app.catalog_variants WHERE " + field + "=$1 AND id<>$2)"
			if err := tx.QueryRow(ctx, query, value, ownerID).Scan(&used); err != nil {
				return err
			}
			if used {
				return invalid("El código interno o de barras ya está reservado por otro producto o presentación.")
			}
		}
	}
	return nil
}
