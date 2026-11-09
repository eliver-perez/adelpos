package catalog

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ProductTaxRates es la lista cerrada de tasas propias de un producto; coincide con la tasa
// general del establecimiento (CHECK en PostgreSQL y validación en payments).
var ProductTaxRates = []int{16, 8, 0}

// validateProductTaxRate acepta nil (hereda la tasa general) o una tasa de la lista cerrada.
func validateProductTaxRate(rate *int) error {
	if rate == nil {
		return nil
	}
	for _, allowed := range ProductTaxRates {
		if *rate == allowed {
			return nil
		}
	}
	return invalid("La tasa de IVA del producto debe ser la tasa general del establecimiento, 16 %, 8 % o 0 %.")
}

// ProductTaxRate expone la tasa propia de un producto para que pedidos calcule la tasa efectiva
// sin consultar tablas del catálogo. nil significa que hereda la tasa general vigente. Toma el
// lock compartido del catálogo, como QuoteForOrder, para leer la misma versión que la cotización.
func ProductTaxRate(ctx context.Context, tx pgx.Tx, productID string) (*int, error) {
	if err := validateID(productID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7820148304)"); err != nil {
		return nil, err
	}
	var rate *int
	err := tx.QueryRow(ctx, "SELECT tax_rate FROM app.catalog_products WHERE id=$1", productID).Scan(&rate)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, invalid("El producto no existe en el catálogo.")
	}
	return rate, err
}
