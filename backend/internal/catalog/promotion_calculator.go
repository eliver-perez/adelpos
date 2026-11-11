package catalog

import (
	"slices"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

func promotionScope(promotion Promotion, line QuoteLine) bool {
	return slices.Contains(promotion.ProductIDs, line.ProductID) || slices.Contains(promotion.CategoryIDs, line.CategoryID)
}

func promotionUnavailable(promotion Promotion, list PriceList, now time.Time, location *time.Location) string {
	if !promotion.Active {
		return "Promoción inactiva."
	}
	if !list.AllowPromotions {
		return "La lista de precios excluye promociones."
	}
	if (len(promotion.PriceListIDs) == 0 && !list.Default) || (len(promotion.PriceListIDs) > 0 && !slices.Contains(promotion.PriceListIDs, list.ID)) {
		return "La promoción no participa en esta lista de precios."
	}
	if !scheduleAvailable(promotion.Availability, now, location) {
		return "Fuera de vigencia, día u horario."
	}
	return ""
}

func applyPromotions(snapshot Snapshot, quote *Quote, now time.Time, location *time.Location) {
	promotions := slices.Clone(snapshot.Promotions)
	sort.Slice(promotions, func(first, second int) bool {
		if promotions[first].Priority != promotions[second].Priority {
			return promotions[first].Priority > promotions[second].Priority
		}
		return promotions[first].ID < promotions[second].ID
	})
	used := make([]bool, len(quote.Lines))
	stackable := make([]bool, len(quote.Lines))
	for _, promotion := range promotions {
		result := PromotionResult{
			ID:       promotion.ID,
			Name:     promotion.Name,
			Revision: promotion.Revision,
			Discount: "0.00",
		}
		result.Reason = promotionUnavailable(promotion, snapshot.PriceList, now, location)
		if result.Reason != "" {
			quote.Promotions = append(quote.Promotions, result)
			continue
		}
		eligible := map[int]decimal.Decimal{}
		units := 0
		for index, line := range quote.Lines {
			if !promotionScope(promotion, line) || (used[index] && !(stackable[index] && promotion.Stackable)) {
				continue
			}
			basis := amount(line.BasePrice).Mul(decimal.NewFromInt(int64(line.Quantity)))
			if line.ComplementAdjustment != "" {
				// Con la opción «El precio puede bajar al cambiar complementos», la base sin extras es
				// el precio base ya reducido por los complementos: la promoción nunca vuelve a
				// acreditar lo que ya se descontó (§3.1). Primero complementos, después promociones.
				reduced := decimal.Max(decimal.Zero, amount(line.BasePrice).Add(amount(line.ComplementAdjustment)))
				basis = reduced.Mul(decimal.NewFromInt(int64(line.Quantity)))
			}
			if promotion.IncludeExtras {
				basis = amount(line.Subtotal)
			}
			// La combinación explícita consume el importe restante, nunca el ya descontado.
			remaining := positiveDifference(basis, amount(line.Discount))
			if remaining.IsPositive() {
				eligible[index] = remaining
				units += line.Quantity
			}
		}
		if units < promotion.MinimumQuantity || len(eligible) == 0 {
			result.Reason = "No alcanza el mínimo o sus productos ya tienen una promoción incompatible."
			quote.Promotions = append(quote.Promotions, result)
			continue
		}
		allocations := promotionAllocations(promotion, quote.Lines, eligible, units)
		discount := decimal.Zero
		for index, value := range allocations {
			value = decimal.Min(value.Round(2), eligible[index])
			if !value.IsPositive() {
				continue
			}
			line := &quote.Lines[index]
			line.Discount = money(amount(line.Discount).Add(value))
			line.Total = money(amount(line.Subtotal).Sub(amount(line.Discount)))
			discount = discount.Add(value)
			used[index], stackable[index] = true, promotion.Stackable
		}
		result.Discount = money(discount)
		result.Applied = discount.IsPositive()
		result.Reason = "Aplicada según prioridad y alcance; descuento sobre importe elegible restante."
		if !result.Applied {
			result.Reason = "La regla no produce descuento con estas cantidades o precios."
		}
		quote.Promotions = append(quote.Promotions, result)
	}
}

func promotionAllocations(promotion Promotion, lines []QuoteLine, eligible map[int]decimal.Decimal, units int) map[int]decimal.Decimal {
	allocations := map[int]decimal.Decimal{}
	switch promotion.Kind {
	case "percent":
		for index, basis := range eligible {
			allocations[index] = basis.Mul(amount(promotion.Percent)).Div(decimal.NewFromInt(100)).Round(2)
		}
	case "special_price":
		for index, basis := range eligible {
			target := amount(promotion.SpecialPrice).Mul(decimal.NewFromInt(int64(lines[index].Quantity)))
			allocations[index] = positiveDifference(basis, target)
		}
	case "two_for_one", "quantity":
		buy, pay := promotion.BuyQuantity, promotion.PayQuantity
		if promotion.Kind == "two_for_one" {
			buy, pay = 2, 1
		}
		if buy < 1 {
			return allocations
		}
		freeUnits := (units / buy) * (buy - pay)
		type eligibleUnit struct {
			line  int
			price decimal.Decimal
		}
		candidates := []eligibleUnit{}
		for index, basis := range eligible {
			price := basis.DivRound(decimal.NewFromInt(int64(lines[index].Quantity)), 20)
			for count := 0; count < lines[index].Quantity; count++ {
				candidates = append(candidates, eligibleUnit{line: index, price: price})
			}
		}
		// En mezclas se bonifican las unidades de menor precio; empates por posición de línea.
		sort.SliceStable(candidates, func(first, second int) bool {
			if !candidates[first].price.Equal(candidates[second].price) {
				return candidates[first].price.LessThan(candidates[second].price)
			}
			return candidates[first].line < candidates[second].line
		})
		for _, candidate := range candidates[:min(freeUnits, len(candidates))] {
			allocations[candidate.line] = allocations[candidate.line].Add(candidate.price)
		}
	}
	return allocations
}
