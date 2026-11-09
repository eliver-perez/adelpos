// Package moneytext es el formato único de importes para textos que lee una persona: tickets,
// precuentas, comprobantes y cortes impresos, comandas y mensajes de error con importes.
//
// Las reglas son idénticas a las de la interfaz (desktop/src/services/money.ts) y ambas pruebas
// leen la misma tabla de casos (testdata/cases.json):
//   - separador de miles «,» y punto decimal;
//   - al menos dos decimales; si el texto trae más, se conservan sin redondear y solo se quitan
//     los ceros finales que sobran después del segundo decimal;
//   - los ceros a la izquierda de la parte entera se normalizan («0001234.5» → «$1,234.50»);
//   - «$» para pesos y «US$» para dólares;
//   - el signo negativo va antes del símbolo con el guion ASCII («-$1,234.56»); un cero negativo
//     se imprime sin signo;
//   - un texto vacío o que no es un decimal válido (signo «+», espacios, comas, notación
//     científica o puntos sin dígitos a ambos lados) se expresa como «—».
//
// El paquete no depende de ningún módulo de la aplicación: el formato es solo de presentación y
// nunca cambia los importes que Go calcula ni los que entrega en JSON.
package moneytext

import (
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

const (
	CurrencyMXN = "MXN"
	CurrencyUSD = "USD"
	// Empty es el texto de un importe vacío o inválido.
	Empty = "—"
)

var decimalPattern = regexp.MustCompile(`^(-?)([0-9]+)(?:\.([0-9]+))?$`)

type parsedAmount struct {
	negative bool
	integer  string
	fraction string
}

// parse descompone el texto decimal sin interpretarlo como número de punto flotante.
func parse(amount string) (parsedAmount, bool) {
	match := decimalPattern.FindStringSubmatch(amount)
	if match == nil {
		return parsedAmount{}, false
	}
	integer := strings.TrimLeft(match[2], "0")
	if integer == "" {
		integer = "0"
	}
	fraction := normalizeFraction(match[3])
	return parsedAmount{
		negative: match[1] == "-" && !isZero(integer, fraction),
		integer:  integer,
		fraction: fraction,
	}, true
}

// normalizeFraction completa a dos decimales; si hay más, solo quita los ceros finales a partir
// del tercero.
func normalizeFraction(fraction string) string {
	if len(fraction) < 2 {
		return fraction + strings.Repeat("0", 2-len(fraction))
	}
	end := len(fraction)
	for end > 2 && fraction[end-1] == '0' {
		end--
	}
	return fraction[:end]
}

func isZero(integer, fraction string) bool {
	return strings.Trim(integer, "0") == "" && strings.Trim(fraction, "0") == ""
}

// groupThousands separa la parte entera en grupos de tres dígitos con comas.
func groupThousands(integer string) string {
	head := len(integer) % 3
	if head == 0 {
		head = 3
	}
	var builder strings.Builder
	builder.WriteString(integer[:head])
	for position := head; position < len(integer); position += 3 {
		builder.WriteString(",")
		builder.WriteString(integer[position : position+3])
	}
	return builder.String()
}

func symbol(currency string) string {
	if currency == CurrencyUSD {
		return "US$"
	}
	return "$"
}

func compose(amount parsedAmount, currency, fraction, sign string) string {
	text := sign + symbol(currency) + groupThousands(amount.integer)
	if fraction != "" {
		text += "." + fraction
	}
	return text
}

// Format expresa un importe decimal con símbolo, miles y dos decimales: «$1,234.56»,
// «-US$20.00» o «—».
func Format(currency, amount string) string {
	parsed, valid := parse(amount)
	if !valid {
		return Empty
	}
	sign := ""
	if parsed.negative {
		sign = "-"
	}
	return compose(parsed, currency, parsed.fraction, sign)
}

// FormatDecimal expresa un decimal calculado por Go. El valor se fija a dos decimales con
// StringFixed(2), igual que los importes que la API entrega como cadenas, y después se le aplica
// Format.
func FormatDecimal(currency string, value decimal.Decimal) string {
	return Format(currency, value.StringFixed(2))
}

// FormatSigned expresa un ajuste o una diferencia con signo explícito: «+$50.00», «-$50.00» y
// «$0.00» para el cero.
func FormatSigned(currency, amount string) string {
	parsed, valid := parse(amount)
	if !valid {
		return Empty
	}
	sign := "+"
	if parsed.negative {
		sign = "-"
	} else if isZero(parsed.integer, parsed.fraction) {
		sign = ""
	}
	return compose(parsed, currency, parsed.fraction, sign)
}

// FormatDenomination expresa el valor de un billete o una moneda: sin decimales cuando es entero
// («$1,000», «US$100») y con dos cuando no lo es («$0.50»).
func FormatDenomination(currency, amount string) string {
	parsed, valid := parse(amount)
	if !valid {
		return Empty
	}
	fraction := parsed.fraction
	if strings.Trim(fraction, "0") == "" {
		fraction = ""
	}
	sign := ""
	if parsed.negative {
		sign = "-"
	}
	return compose(parsed, currency, fraction, sign)
}
