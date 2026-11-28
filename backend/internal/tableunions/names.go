package tableunions

import (
	"sort"
	"strings"
	"unicode"

	"local/ad-el-pos/server/internal/documents"
)

// NamedTable es una mesa de una unión o de un grupo con lo que hace falta para nombrarla y sumar su capacidad.
type NamedTable struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Seats int    `json:"seats"`
}

// SortTables ordena las mesas en orden natural de su nombre (M1-D2): los números como números y sin distinguir
// mayúsculas, así que «2» va antes que «10» y «3a» antes que «3B». La palabra «Mesa» del principio no cuenta para el
// orden (con «5» y «Mesa 6», primero «5»); con nombres iguales decide el identificador, para que el orden sea estable.
func SortTables(tables []NamedTable) {
	sort.SliceStable(tables, func(left, right int) bool {
		first, second := sortKey(tables[left].Name), sortKey(tables[right].Name)
		if comparison := NaturalCompare(first, second); comparison != 0 {
			return comparison < 0
		}
		if comparison := NaturalCompare(tables[left].Name, tables[right].Name); comparison != 0 {
			return comparison < 0
		}
		return tables[left].ID < tables[right].ID
	})
}

// UnionName es el nombre de una unión con sus mesas ya en orden (SortTables), según M1-D2:
//   - si todas empiezan con la palabra «Mesa», «Mesa 5 + 6» (la palabra una sola vez);
//   - si no, los nombres tal cual: «5 + 6».
//
// El nombre de un grupo (una unión con todas sus secciones) lo decide quien llama, porque depende de los grupos.
func UnionName(tables []NamedTable) string {
	allWithWord := len(tables) > 0
	for _, table := range tables {
		if !documents.TableNameIncludesWord(table.Name) {
			allWithWord = false
			break
		}
	}
	parts := make([]string, 0, len(tables))
	for _, table := range tables {
		name := strings.TrimSpace(table.Name)
		if allWithWord {
			name = withoutWord(name)
		}
		parts = append(parts, name)
	}
	joined := strings.Join(parts, " + ")
	if allWithWord {
		return "Mesa " + joined
	}
	return joined
}

// AccountNameLimit es el largo máximo del nombre de una cuenta (app.restaurant_accounts.name y validateAccountInput).
const AccountNameLimit = 100

// accountReferenceTail es lo que cierra una referencia recortada.
const accountReferenceTail = " + …"

// AccountReference es la referencia neutra de una cuenta sin nombre propio en una unión (M1-D14): el nombre de la unión
// tal cual si cabe en el nombre de una cuenta. Si no cabe (M1-D29: una unión de muchas mesas puede pasar de 100
// caracteres), se recorta en el último « + » que quepa, sin partir el nombre de una mesa, y termina en « + …»:
// «Terraza 1 + Terraza 2 + … + Terraza 8 + …». La unión conserva su nombre completo en el mapa, la lista, los
// documentos, la comanda y el KDS; solo la cuenta lleva la referencia recortada. Se mide en bytes, como la validación de
// Go, que es más estricta que la de la base (caracteres).
func AccountReference(unionName string) string {
	if len(unionName) <= AccountNameLimit {
		return unionName
	}
	limit := AccountNameLimit - len(accountReferenceTail)
	cut := -1
	for index := strings.Index(unionName, " + "); index >= 0 && index <= limit; {
		cut = index
		next := strings.Index(unionName[index+1:], " + ")
		if next < 0 {
			break
		}
		index += 1 + next
	}
	if cut > 0 {
		return unionName[:cut] + accountReferenceTail
	}
	// Sin un « + » que quepa (una primera mesa de nombre larguísimo): se corta en un carácter completo.
	runes := []rune(unionName)
	for len(string(runes))+len("…") > AccountNameLimit {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// Seats es la capacidad de la unión: la suma de sus mesas.
func Seats(tables []NamedTable) int {
	total := 0
	for _, table := range tables {
		total += table.Seats
	}
	return total
}

// sortKey es el nombre sin la palabra «Mesa» del principio, para ordenar.
func sortKey(name string) string {
	trimmed := strings.TrimSpace(name)
	if documents.TableNameIncludesWord(trimmed) {
		return withoutWord(trimmed)
	}
	return trimmed
}

// withoutWord quita la palabra «Mesa» (con o sin acento, en cualquier mayúscula) y lo que la separa del resto: «Mesa
// 5» → «5», «mesa-3a» → «3a». Si no queda nada, devuelve el nombre completo.
func withoutWord(name string) string {
	runes := []rune(name)
	if len(runes) < 4 {
		return name
	}
	rest := strings.TrimLeftFunc(string(runes[4:]), func(character rune) bool {
		return unicode.IsSpace(character) || unicode.IsPunct(character)
	})
	if rest == "" {
		return name
	}
	return rest
}

// NaturalCompare compara dos textos en orden natural sin distinguir mayúsculas: los tramos de dígitos se comparan como
// números (sin ceros a la izquierda; a igual valor, el más corto primero) y el resto por su texto en minúsculas.
// Devuelve -1, 0 o 1.
func NaturalCompare(first, second string) int {
	left, right := []rune(strings.ToLower(first)), []rune(strings.ToLower(second))
	i, j := 0, 0
	for i < len(left) && j < len(right) {
		if unicode.IsDigit(left[i]) && unicode.IsDigit(right[j]) {
			startLeft, startRight := i, j
			for i < len(left) && unicode.IsDigit(left[i]) {
				i++
			}
			for j < len(right) && unicode.IsDigit(right[j]) {
				j++
			}
			numberLeft := strings.TrimLeft(string(left[startLeft:i]), "0")
			numberRight := strings.TrimLeft(string(right[startRight:j]), "0")
			if len(numberLeft) != len(numberRight) {
				if len(numberLeft) < len(numberRight) {
					return -1
				}
				return 1
			}
			if numberLeft != numberRight {
				if numberLeft < numberRight {
					return -1
				}
				return 1
			}
			if lengthLeft, lengthRight := i-startLeft, j-startRight; lengthLeft != lengthRight {
				if lengthLeft < lengthRight {
					return -1
				}
				return 1
			}
			continue
		}
		if left[i] != right[j] {
			if left[i] < right[j] {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	switch {
	case len(left)-i < len(right)-j:
		return -1
	case len(left)-i > len(right)-j:
		return 1
	}
	return 0
}
