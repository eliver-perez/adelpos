package documents

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Nombres de mesa sin «Mesa» repetido (etapa 9F, §4.6). El restaurante suele llamar a sus mesas «Mesa 1»,
// «Mesa 2»…; anteponer «Mesa » a ese nombre imprimía «Mesa Mesa 1». La palabra se reconoce sin distinguir
// mayúsculas ni acentos y solo como palabra completa: «Mesanine» o «Mesón 3» siguen llevando el prefijo.

// TableNameIncludesWord indica si el nombre de la mesa ya empieza con la palabra «Mesa».
func TableNameIncludesWord(name string) bool {
	folded := foldAccents(strings.ToLower(strings.TrimSpace(name)))
	rest, found := strings.CutPrefix(folded, "mesa")
	if !found {
		return false
	}
	for _, character := range rest {
		return !unicode.IsLetter(character)
	}
	return true
}

// TableContext es el texto de la mesa en la precuenta y el ticket («Mesa 4»), sin repetir «Mesa» si el
// nombre ya la incluye («Mesa 1» se imprime «Mesa 1», no «Mesa Mesa 1»).
func TableContext(name string) string {
	if TableNameIncludesWord(name) {
		return name
	}
	return "Mesa " + name
}

// foldAccents quita los acentos y la diéresis para comparar palabras («Mésa» = «mesa»).
func foldAccents(text string) string {
	folded, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), text)
	if err != nil {
		return text
	}
	return folded
}
