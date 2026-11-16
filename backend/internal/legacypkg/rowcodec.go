package legacypkg

import (
	"encoding/json"
	"errors"
	"strconv"
	"unicode/utf8"
)

// Valores de las filas. En el paquete cada fila es un objeto JSON con las columnas en el orden del manifiesto. Los
// importes y las cantidades viajan como números JSON con su texto decimal exacto (json.Number, nunca float64); las
// fechas, como texto; los nulos, como null.
//
// Al escribir se aceptan nil, bool, string, json.Number, int, int32 e int64. Al leer se entregan nil, bool, string y
// json.Number.

const hexDigits = "0123456789abcdef"

// appendValue agrega un valor JSON a la fila que se está escribiendo.
func appendValue(buffer []byte, value any) ([]byte, error) {
	switch typed := value.(type) {
	case nil:
		return append(buffer, "null"...), nil
	case bool:
		if typed {
			return append(buffer, "true"...), nil
		}
		return append(buffer, "false"...), nil
	case string:
		if !utf8.ValidString(typed) {
			return buffer, errors.New("texto con UTF-8 inválido")
		}
		return appendString(buffer, typed), nil
	case json.Number:
		if !isJSONNumber(string(typed)) {
			return buffer, errors.New("número inválido")
		}
		return append(buffer, typed...), nil
	case int:
		return strconv.AppendInt(buffer, int64(typed), 10), nil
	case int32:
		return strconv.AppendInt(buffer, int64(typed), 10), nil
	case int64:
		return strconv.AppendInt(buffer, typed, 10), nil
	}
	return buffer, errors.New("tipo de valor no admitido en una fila (los importes van como json.Number)")
}

// appendString escribe un texto JSON con el escape mínimo: comillas, barra invertida y caracteres de control. No
// escapa «<», «>» ni «&» (encoding/json lo hace para HTML): el texto queda tal como estaba.
func appendString(buffer []byte, text string) []byte {
	buffer = append(buffer, '"')
	start := 0
	for index := 0; index < len(text); index++ {
		character := text[index]
		if character >= 0x20 && character != '"' && character != '\\' {
			continue
		}
		buffer = append(buffer, text[start:index]...)
		switch character {
		case '"':
			buffer = append(buffer, '\\', '"')
		case '\\':
			buffer = append(buffer, '\\', '\\')
		case '\n':
			buffer = append(buffer, '\\', 'n')
		case '\r':
			buffer = append(buffer, '\\', 'r')
		case '\t':
			buffer = append(buffer, '\\', 't')
		default:
			buffer = append(buffer, '\\', 'u', '0', '0', hexDigits[character>>4], hexDigits[character&0x0f])
		}
		start = index + 1
	}
	buffer = append(buffer, text[start:]...)
	return append(buffer, '"')
}

// isJSONNumber comprueba la gramática de un número JSON: -?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?
func isJSONNumber(text string) bool {
	index := 0
	if index < len(text) && text[index] == '-' {
		index++
	}
	if index >= len(text) {
		return false
	}
	if text[index] == '0' {
		index++
	} else if isDigit(text[index]) {
		index = skipDigits(text, index)
	} else {
		return false
	}
	if index < len(text) && text[index] == '.' {
		index++
		if index >= len(text) || !isDigit(text[index]) {
			return false
		}
		index = skipDigits(text, index)
	}
	if index < len(text) && (text[index] == 'e' || text[index] == 'E') {
		index++
		if index < len(text) && (text[index] == '+' || text[index] == '-') {
			index++
		}
		if index >= len(text) || !isDigit(text[index]) {
			return false
		}
		index = skipDigits(text, index)
	}
	return index == len(text)
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}

func skipDigits(text string, index int) int {
	for index < len(text) && isDigit(text[index]) {
		index++
	}
	return index
}
