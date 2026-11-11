package documents

import (
	"strings"
	"unicode"

	"local/ad-el-pos/server/internal/moneytext"
)

// Funciones de texto de las tres plantillas anteriores a la 9B, copiadas sin cambios de
// comportamiento: cada familia conserva su propia forma de sanear y cortar líneas para que las
// plantillas predeterminadas reproduzcan exactamente los documentos de las etapas 5 a 7.1.
//   - Cocina: CleanText + corte por carácter que conserva los saltos de línea (wrapKitchen).
//   - Precuenta y ticket: Printable + corte por carácter (fit), controles convertidos en espacios.
//   - Caja: corte entre palabras (wrapWords) sobre el texto de Printable.

// CleanText elimina los caracteres de control e invisibles de un texto de usuario. Conserva los
// saltos de línea y convierte los tabuladores en espacios: nunca deja pasar ESC, GS, comandos de
// corte ni controles invisibles, porque solo el controlador del transporte genera comandos.
func CleanText(value string) string {
	return strings.Map(func(character rune) rune {
		if character == '\n' {
			return character
		}
		if character == '\t' {
			return ' '
		}
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return -1
		}
		return character
	}, value)
}

// Printable convierte en espacios los controles e invisibles (precuentas, tickets y caja).
func Printable(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return ' '
		}
		return character
	}, value)
}

// wrapKitchen corta por carácter cada línea del texto limpio (comandas).
func wrapKitchen(value string, width int) []string {
	result := []string{}
	for _, line := range strings.Split(CleanText(value), "\n") {
		remaining := []rune(line)
		for len(remaining) > width {
			result = append(result, string(remaining[:width]))
			remaining = remaining[width:]
		}
		result = append(result, string(remaining))
	}
	return result
}

// fit corta por carácter un texto imprimible (precuentas y tickets).
func fit(text string, width int) []string {
	runes := []rune(Printable(text))
	lines := []string{}
	for len(runes) > width {
		lines = append(lines, string(runes[:width]))
		runes = runes[width:]
	}
	return append(lines, string(runes))
}

// row alinea un concepto a la izquierda y un importe a la derecha dentro del ancho del papel.
func row(left, right string, width int) []string {
	left = Printable(left)
	right = Printable(right)
	space := width - len([]rune(right)) - 1
	if space < 4 {
		return append(fit(left, width), right)
	}
	parts := fit(left, space)
	last := parts[len(parts)-1]
	parts[len(parts)-1] = last + strings.Repeat(" ", width-len([]rune(last))-len([]rune(right))) + right
	return parts
}

// wrapWords reparte un texto en líneas del ancho del papel cortando entre palabras; una palabra
// más larga que el ancho se corta como en los documentos comerciales (documentos de caja).
func wrapWords(value string, width int) []string {
	words := strings.Fields(Printable(value))
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{}
	current := ""
	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if len([]rune(candidate)) <= width {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
		}
		pieces := fit(word, width)
		lines = append(lines, pieces[:len(pieces)-1]...)
		current = pieces[len(pieces)-1]
	}
	return append(lines, current)
}

// pesos expresa un importe en pesos con el formato único de moneytext («-$1,234.56»).
func pesos(amount string) string {
	return moneytext.Format(moneytext.CurrencyMXN, amount)
}

// dollars expresa un importe en dólares («US$20.00»).
func dollars(amount string) string {
	return moneytext.Format(moneytext.CurrencyUSD, amount)
}

// currencyAmount expresa un importe en su moneda.
func currencyAmount(currency, amount string) string {
	return moneytext.Format(currency, amount)
}

// rate expresa un tipo de cambio en pesos por dólar («$18.50/US$»).
func rate(value string) string {
	return pesos(value) + "/US$"
}

// runeCount es la longitud de un texto en caracteres.
func runeCount(value string) int {
	return len([]rune(value))
}
