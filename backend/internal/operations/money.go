package operations

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/catalog"
)

func value(amount string) decimal.Decimal {
	// Solo se convierten valores numeric del servidor o resultados del motor de catálogo.
	return decimal.RequireFromString(amount)
}

func money(amount decimal.Decimal) string {
	return amount.Round(2).StringFixed(2)
}

func addTotals(first, second Totals) Totals {
	return Totals{
		Subtotal: money(value(first.Subtotal).Add(value(second.Subtotal))),
		Discount: money(value(first.Discount).Add(value(second.Discount))),
		Total:    money(value(first.Total).Add(value(second.Total))),
	}
}

func quoteTotals(quote *catalog.Quote) Totals {
	return Totals{Subtotal: quote.Subtotal, Discount: quote.Discount, Total: quote.Total}
}

func quoteFingerprint(quote *catalog.Quote, context any) string {
	stable := *quote
	stable.CalculatedAt = time.Time{}
	digest := sha256.Sum256(jsonValue(map[string]any{"quote": stable, "context": context}))
	return fmt.Sprintf("%x", digest)
}

func unitMoney(amount string, quantity, index int) string {
	// Prorrateo entero: los centavos restantes pertenecen a las primeras unidades estables.
	// Transferir una unidad conserva esos mismos centavos y no vuelve a evaluar promociones.
	quotient, remainder := value(amount).Shift(2).QuoRem(decimal.NewFromInt(int64(quantity)), 0)
	if int64(index) < remainder.IntPart() {
		quotient = quotient.Add(decimal.NewFromInt(1))
	}
	return money(quotient.Shift(-2))
}

func validateStorageAmount(quote *catalog.Quote) error {
	maximum := decimal.RequireFromString("9999999999999999.99")
	if value(quote.Subtotal).GreaterThan(maximum) || value(quote.Total).GreaterThan(maximum) {
		return invalid("El importe excede la capacidad monetaria admitida por una operación.")
	}
	return nil
}
