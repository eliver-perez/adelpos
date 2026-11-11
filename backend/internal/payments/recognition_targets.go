package payments

import (
	"math/big"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// Cálculo puro del reconocimiento de ventas (etapa 9A, §0-B «Cuándo cuenta una venta»). A partir del
// estado financiero vigente de una cuenta liquidada produce el «objetivo» de cada fuente: lo que la
// venta de esa fuente vale hoy. El libro guarda la diferencia entre el objetivo y lo ya reconocido
// (recognition.go). El relleno inicial de la migración 0017 reproduce este mismo cálculo en SQL; la
// prueba de integración de reportes compara ambos resultados.
//
// Fuentes (clave: clase, unidad, ajuste y tasa de IVA):
//   - unit: unidad comercial activa: cantidad 1, bruto (subtotal), promoción (descuento del
//     catálogo) y neto (total);
//   - allocation: asignación de un descuento o cortesía por partida vigente a una unidad activa;
//   - adjustment: cargo general (con su tasa) o descuento/cortesía general repartido entre tasas en
//     proporción al bruto de cada tasa, como finance.go: el total de descuentos generales se reparte
//     con allocate y cada descuento recibe su parte con los mismos pesos; el último registrado
//     recibe lo que falta en cada tasa, para que las partes sumen el reparto de la cuenta.
//
// IVA: el desglose de la cuenta por tasa es el de finance.go (taxBreakdown sobre el bruto por tasa
// después de descuentos). Cada renglón recibe un IVA provisional con la misma regla y el residuo de
// centavos de su cuenta y tasa se reparte uno a uno entre los renglones de mayor neto (desempate por
// clase, unidad y ajuste), de modo que la suma por cuenta y tasa es exactamente el desglose.

const (
	sourceUnit       = "unit"
	sourceAllocation = "allocation"
	sourceAdjustment = "adjustment"
)

// recognitionKey identifica una fuente del reconocimiento.
type recognitionKey struct {
	Kind         string
	UnitID       string
	AdjustmentID string
	TaxRate      int
}

// recognitionAmounts son los importes de una fuente. Net = Gross − Promotion − Discount − Courtesy
// + Charge y Net = Base + Tax.
type recognitionAmounts struct {
	Quantity  int
	Gross     decimal.Decimal
	Promotion decimal.Decimal
	Discount  decimal.Decimal
	Courtesy  decimal.Decimal
	Charge    decimal.Decimal
	Net       decimal.Decimal
	Base      decimal.Decimal
	Tax       decimal.Decimal
}

func (amounts recognitionAmounts) minus(other recognitionAmounts) recognitionAmounts {
	return recognitionAmounts{
		Quantity:  amounts.Quantity - other.Quantity,
		Gross:     amounts.Gross.Sub(other.Gross),
		Promotion: amounts.Promotion.Sub(other.Promotion),
		Discount:  amounts.Discount.Sub(other.Discount),
		Courtesy:  amounts.Courtesy.Sub(other.Courtesy),
		Charge:    amounts.Charge.Sub(other.Charge),
		Net:       amounts.Net.Sub(other.Net),
		Base:      amounts.Base.Sub(other.Base),
		Tax:       amounts.Tax.Sub(other.Tax),
	}
}

func (amounts recognitionAmounts) isZero() bool {
	return amounts.Quantity == 0 && amounts.Gross.IsZero() && amounts.Promotion.IsZero() && amounts.Discount.IsZero() &&
		amounts.Courtesy.IsZero() && amounts.Charge.IsZero() && amounts.Net.IsZero() && amounts.Base.IsZero() &&
		amounts.Tax.IsZero()
}

// recognitionAttributes son los datos históricos que acompañan a la fuente para reportar sin leer
// el snapshot del pedido.
type recognitionAttributes struct {
	VersionID              *string
	OrderID                *string
	ProductID              *string
	ProductName            *string
	ProductCode            *string
	ProductKind            *string
	VariantID              *string
	VariantName            *string
	CategoryID             *string
	CategoryName           *string
	PriceListID            *string
	PriceListName          *string
	AdjustmentKind         *string
	AdjustmentScope        *string
	IsCourtesy             bool
	ReasonID               *string
	ReasonName             *string
	Concept                *string
	AdjustmentActorID      *string
	AdjustmentAuthorizedBy *string
	AdjustmentCreatedAt    *time.Time
}

// recognitionLine es una fuente con sus importes y datos.
type recognitionLine struct {
	Key        recognitionKey
	Amounts    recognitionAmounts
	Attributes recognitionAttributes
}

// recognitionUnit es una unidad activa de la cuenta con sus importes confirmados.
type recognitionUnit struct {
	ID         string
	TaxRate    int
	Subtotal   decimal.Decimal
	Promotion  decimal.Decimal
	Total      decimal.Decimal
	Attributes recognitionAttributes
}

// recognitionAllocation es la asignación de un descuento por partida vigente a una unidad activa.
type recognitionAllocation struct {
	UnitID       string
	AdjustmentID string
	Amount       decimal.Decimal
	Adjustment   recognitionAttributes
}

// recognitionAdjustment es un descuento, cortesía o cargo general vigente de la cuenta, en el orden
// de registro (created_at, id).
type recognitionAdjustment struct {
	ID         string
	Kind       string
	TaxRate    *int
	Amount     decimal.Decimal
	Attributes recognitionAttributes
}

// buildRecognitionTargets calcula el objetivo de cada fuente de una cuenta liquidada.
func buildRecognitionTargets(units []recognitionUnit, allocations []recognitionAllocation,
	adjustments []recognitionAdjustment, accountList recognitionAttributes) []recognitionLine {
	lines := []recognitionLine{}
	unitRate := map[string]int{}
	unitAttributes := map[string]recognitionAttributes{}
	allocated := map[string]decimal.Decimal{}
	for _, allocation := range allocations {
		allocated[allocation.UnitID] = allocated[allocation.UnitID].Add(allocation.Amount)
	}
	grossByRate := map[int]decimal.Decimal{}
	for _, unit := range units {
		unitRate[unit.ID] = unit.TaxRate
		unitAttributes[unit.ID] = unit.Attributes
		grossByRate[unit.TaxRate] = grossByRate[unit.TaxRate].Add(unit.Total.Sub(allocated[unit.ID]))
		lines = append(lines, recognitionLine{
			Key: recognitionKey{Kind: sourceUnit, UnitID: unit.ID, TaxRate: unit.TaxRate},
			Amounts: recognitionAmounts{
				Quantity: 1, Gross: unit.Subtotal, Promotion: unit.Promotion, Net: unit.Total,
			},
			Attributes: unit.Attributes,
		})
	}
	for _, allocation := range allocations {
		attributes := mergeAdjustmentAttributes(unitAttributes[allocation.UnitID], allocation.Adjustment)
		amounts := recognitionAmounts{Net: allocation.Amount.Neg()}
		if attributes.IsCourtesy {
			amounts.Courtesy = allocation.Amount
		} else {
			amounts.Discount = allocation.Amount
		}
		lines = append(lines, recognitionLine{
			Key: recognitionKey{
				Kind: sourceAllocation, UnitID: allocation.UnitID, AdjustmentID: allocation.AdjustmentID,
				TaxRate: unitRate[allocation.UnitID],
			},
			Amounts:    amounts,
			Attributes: attributes,
		})
	}
	discounts := []recognitionAdjustment{}
	totalDiscount := zero
	for _, adjustment := range adjustments {
		if adjustment.Kind == "charge" {
			rate := *adjustment.TaxRate
			grossByRate[rate] = grossByRate[rate].Add(adjustment.Amount)
			lines = append(lines, recognitionLine{
				Key:        recognitionKey{Kind: sourceAdjustment, AdjustmentID: adjustment.ID, TaxRate: rate},
				Amounts:    recognitionAmounts{Charge: adjustment.Amount, Net: adjustment.Amount},
				Attributes: mergeAdjustmentAttributes(accountList, adjustment.Attributes),
			})
			continue
		}
		discounts = append(discounts, adjustment)
		totalDiscount = totalDiscount.Add(adjustment.Amount)
	}
	lines = append(lines, discountParts(discounts, totalDiscount, grossByRate, accountList)...)
	distributeTax(lines)
	return lines
}

// mergeAdjustmentAttributes combina los datos del producto (o de la lista de la cuenta) con los del
// ajuste que origina la fuente.
func mergeAdjustmentAttributes(base, adjustment recognitionAttributes) recognitionAttributes {
	base.AdjustmentKind = adjustment.AdjustmentKind
	base.AdjustmentScope = adjustment.AdjustmentScope
	base.IsCourtesy = adjustment.IsCourtesy
	base.ReasonID = adjustment.ReasonID
	base.ReasonName = adjustment.ReasonName
	base.Concept = adjustment.Concept
	base.AdjustmentActorID = adjustment.AdjustmentActorID
	base.AdjustmentAuthorizedBy = adjustment.AdjustmentAuthorizedBy
	base.AdjustmentCreatedAt = adjustment.AdjustmentCreatedAt
	return base
}

// discountParts reparte los descuentos generales entre tasas (ver el comentario del archivo). Las
// partes en cero no son fuentes.
func discountParts(discounts []recognitionAdjustment, total decimal.Decimal, grossByRate map[int]decimal.Decimal,
	accountList recognitionAttributes) []recognitionLine {
	if len(discounts) == 0 {
		return nil
	}
	rates := []int{}
	weights := []decimal.Decimal{}
	for _, rate := range allowedTaxRates {
		if gross, found := grossByRate[rate]; found {
			rates = append(rates, rate)
			weights = append(weights, gross)
		}
	}
	share := allocate(total, weights)
	assigned := make([]decimal.Decimal, len(rates))
	lines := []recognitionLine{}
	for index, discount := range discounts {
		parts := allocate(discount.Amount, weights)
		if index == len(discounts)-1 {
			for position := range rates {
				parts[position] = share[position].Sub(assigned[position])
			}
		}
		for position, rate := range rates {
			assigned[position] = assigned[position].Add(parts[position])
			if parts[position].IsZero() {
				continue
			}
			amounts := recognitionAmounts{Net: parts[position].Neg()}
			if discount.Attributes.IsCourtesy {
				amounts.Courtesy = parts[position]
			} else {
				amounts.Discount = parts[position]
			}
			lines = append(lines, recognitionLine{
				Key:        recognitionKey{Kind: sourceAdjustment, AdjustmentID: discount.ID, TaxRate: rate},
				Amounts:    amounts,
				Attributes: mergeAdjustmentAttributes(accountList, discount.Attributes),
			})
		}
	}
	return lines
}

// lineTax es el IVA incluido en un importe con la regla de taxBreakdown: base = importe / (1 + tasa)
// redondeada a centavos e IVA = importe − base.
func lineTax(amount decimal.Decimal, rate int) decimal.Decimal {
	if rate == 0 {
		return zero
	}
	divisor := decimal.NewFromInt(int64(100 + rate)).Div(hundred)
	return amount.Sub(roundMoney(amount.DivRound(divisor, 8)))
}

// recognitionOrder ordena los renglones de una tasa para repartir el residuo de centavos: mayor neto
// primero y desempate por clase, unidad y ajuste (el mismo orden que la migración 0017).
func recognitionOrder(left, right recognitionLine) bool {
	if comparison := left.Amounts.Net.Cmp(right.Amounts.Net); comparison != 0 {
		return comparison > 0
	}
	if left.Key.Kind != right.Key.Kind {
		return left.Key.Kind < right.Key.Kind
	}
	if left.Key.UnitID != right.Key.UnitID {
		return left.Key.UnitID < right.Key.UnitID
	}
	return left.Key.AdjustmentID < right.Key.AdjustmentID
}

// distributeTax asigna el IVA y la base de cada renglón (ver el comentario del archivo).
func distributeTax(lines []recognitionLine) {
	byRate := map[int][]int{}
	netByRate := map[int]decimal.Decimal{}
	for index, line := range lines {
		byRate[line.Key.TaxRate] = append(byRate[line.Key.TaxRate], index)
		netByRate[line.Key.TaxRate] = netByRate[line.Key.TaxRate].Add(line.Amounts.Net)
	}
	for rate, indexes := range byRate {
		target := lineTax(netByRate[rate], rate)
		provisional := zero
		for _, index := range indexes {
			lines[index].Amounts.Tax = lineTax(lines[index].Amounts.Net, rate)
			provisional = provisional.Add(lines[index].Amounts.Tax)
		}
		sort.SliceStable(indexes, func(left, right int) bool {
			return recognitionOrder(lines[indexes[left]], lines[indexes[right]])
		})
		residual := cents(target.Sub(provisional))
		count := big.NewInt(int64(len(indexes)))
		magnitude := new(big.Int).Abs(residual)
		quotient, remainder := new(big.Int).QuoRem(magnitude, count, new(big.Int))
		for rank, index := range indexes {
			extra := new(big.Int).Set(quotient)
			if big.NewInt(int64(rank+1)).Cmp(remainder) <= 0 {
				extra.Add(extra, big.NewInt(1))
			}
			if residual.Sign() < 0 {
				extra.Neg(extra)
			}
			lines[index].Amounts.Tax = lines[index].Amounts.Tax.Add(fromCents(extra))
		}
		for _, index := range indexes {
			lines[index].Amounts.Base = lines[index].Amounts.Net.Sub(lines[index].Amounts.Tax)
		}
	}
}
