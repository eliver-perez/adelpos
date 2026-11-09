package payments

import (
	"math/big"
	"regexp"
	"sort"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/moneytext"
)

var (
	zero        = decimal.Zero
	hundred     = decimal.NewFromInt(100)
	amountRegex = regexp.MustCompile(`^(0|[1-9][0-9]{0,9})(\.[0-9]{1,2})?$`)
	// Tasas de IVA permitidas: lista cerrada, nunca capturada a mano.
	allowedTaxRates = []int{16, 8, 0}
)

// parseAmount acepta únicamente importes no negativos con hasta diez dígitos enteros y dos
// decimales. Rechaza signos, notación científica, espacios, comas y dobles puntos.
func parseAmount(text, field string) (decimal.Decimal, error) {
	if !amountRegex.MatchString(text) {
		return zero, invalid("El importe de " + field + " no es válido: usa hasta dos decimales y sin signos.")
	}
	value, err := decimal.NewFromString(text)
	if err != nil {
		return zero, invalid("El importe de " + field + " no es válido.")
	}
	return value, nil
}

// parseOptionalAmount trata una cadena vacía como cero.
func parseOptionalAmount(text, field string) (decimal.Decimal, error) {
	if text == "" {
		return zero, nil
	}
	return parseAmount(text, field)
}

// money es la cadena decimal de un importe para JSON y para la base de datos: dos decimales, sin
// símbolo ni separadores. Los textos para personas usan formatPesos o formatDollars.
func money(value decimal.Decimal) string {
	return value.StringFixed(2)
}

// formatPesos y formatDollars expresan un importe calculado por Go en los mensajes con el formato
// único de moneytext («$1,234.50», «US$2.70»).
func formatPesos(value decimal.Decimal) string {
	return moneytext.FormatDecimal(currencyMXN, value)
}

func formatDollars(value decimal.Decimal) string {
	return moneytext.FormatDecimal(currencyUSD, value)
}

func cents(value decimal.Decimal) *big.Int {
	return value.Mul(hundred).Round(0).BigInt()
}

func fromCents(value *big.Int) decimal.Decimal {
	return decimal.NewFromBigInt(value, -2)
}

// roundMoney aplica el redondeo comercial del sistema: mitad alejándose de cero a centavos.
func roundMoney(value decimal.Decimal) decimal.Decimal {
	return value.Round(2)
}

type remainder struct {
	index int
	value *big.Int
}

// allocate reparte un importe entre partes en proporción a sus pesos, en centavos exactos.
// La suma de las partes es exactamente el total. Los centavos residuales se asignan a las partes
// con mayor residuo; los empates se resuelven por posición, de modo que el resultado es determinista.
// Con pesos nulos, el reparto es igualitario.
func allocate(total decimal.Decimal, weights []decimal.Decimal) []decimal.Decimal {
	result := make([]decimal.Decimal, len(weights))
	if len(weights) == 0 {
		return result
	}
	totalCents := cents(total)
	weightCents := make([]*big.Int, len(weights))
	weightSum := big.NewInt(0)
	for index, weight := range weights {
		weightCents[index] = cents(weight)
		weightSum.Add(weightSum, weightCents[index])
	}
	if weightSum.Sign() == 0 {
		for index := range weightCents {
			weightCents[index] = big.NewInt(1)
		}
		weightSum = big.NewInt(int64(len(weights)))
	}
	assigned := big.NewInt(0)
	remainders := make([]remainder, len(weights))
	for index := range weights {
		product := new(big.Int).Mul(totalCents, weightCents[index])
		share, rest := new(big.Int).QuoRem(product, weightSum, new(big.Int))
		result[index] = fromCents(share)
		assigned.Add(assigned, share)
		remainders[index] = remainder{index: index, value: rest}
	}
	sort.SliceStable(remainders, func(left, right int) bool {
		comparison := remainders[left].value.Cmp(remainders[right].value)
		if comparison != 0 {
			return comparison > 0
		}
		return remainders[left].index < remainders[right].index
	})
	pending := new(big.Int).Sub(totalCents, assigned)
	oneCent := decimal.New(1, -2)
	for position := 0; pending.Sign() > 0; position++ {
		target := remainders[position%len(remainders)].index
		result[target] = result[target].Add(oneCent)
		pending.Sub(pending, big.NewInt(1))
	}
	return result
}

// TaxLine es el desglose de IVA de un importe con precio que ya incluye el impuesto.
type TaxLine struct {
	Rate  int    `json:"rate"`
	Base  string `json:"base"`
	Tax   string `json:"tax"`
	Total string `json:"total"`
}

// taxBreakdown desglosa importes brutos agrupados por tasa. Base = total / (1 + tasa) redondeada
// a centavos y el IVA es la diferencia, por lo que base + IVA es exactamente el total de cada tasa.
// El desglose se calcula una vez por cuenta (o por devolución) sobre la suma por tasa; así no se
// acumulan errores de redondeo unidad por unidad.
func taxBreakdown(grossByRate map[int]decimal.Decimal) []TaxLine {
	lines := []TaxLine{}
	for _, rate := range allowedTaxRates {
		gross, found := grossByRate[rate]
		if !found {
			continue
		}
		divisor := decimal.NewFromInt(int64(100 + rate)).Div(hundred)
		base := roundMoney(gross.DivRound(divisor, 8))
		lines = append(lines, TaxLine{
			Rate:  rate,
			Base:  money(base),
			Tax:   money(gross.Sub(base)),
			Total: money(gross),
		})
	}
	return lines
}

func validTaxRate(rate int) bool {
	for _, allowed := range allowedTaxRates {
		if allowed == rate {
			return true
		}
	}
	return false
}

// percentOf calcula el porcentaje que representa un importe sobre una base, con dos decimales
// redondeados hacia arriba para que un descuento nunca se evalúe por debajo de su efecto real.
func percentOf(amount, base decimal.Decimal) decimal.Decimal {
	if base.Sign() <= 0 {
		return hundred
	}
	return amount.Mul(hundred).Div(base).RoundUp(2)
}
