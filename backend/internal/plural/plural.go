// Package plural da la concordancia de los contadores en textos impresos y mensajes («1 cuenta»,
// «0 cuentas», «2 cuentas»). Equivale a desktop/src/services/plural.ts y ambas pruebas leen la
// misma tabla de casos (testdata/cases.json). Solo el uno es singular en español.
package plural

import "strconv"

// Count antepone el número a la forma que le corresponde.
func Count(count int, singular, plural string) string {
	return strconv.Itoa(count) + " " + Word(count, singular, plural)
}

// Word devuelve solo la forma que corresponde al número, para concordar otras palabras de la
// frase (por ejemplo, el verbo).
func Word(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
