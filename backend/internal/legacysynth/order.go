package legacysynth

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf16"

	"github.com/shopspring/decimal"
	"golang.org/x/text/encoding/charmap"

	"local/ad-el-pos/server/internal/legacypkg"
)

// El lector ordena las filas de cada tabla en SQL Server (internal/legacyread): por la llave primaria o, sin ella, por
// todas las columnas en su orden, y las de texto con la intercalación binaria Latin1_General_BIN2 seguida de su
// DATALENGTH. compareRows reproduce ese orden para que el paquete sintético salga idéntico, fila por fila, al que
// legacy-prepare produce del respaldo sintético (lo comprueba la prueba con Docker).

// compareRows compara dos filas ya proyectadas por las columnas indicadas.
func compareRows(columns []legacypkg.Column, keys []int, left, right []any) int {
	for _, index := range keys {
		if result := compareValues(columns[index].Type, left[index], right[index]); result != 0 {
			return result
		}
	}
	return 0
}

// compareValues compara como SQL Server en orden ascendente: NULL primero.
func compareValues(sqlType string, left, right any) int {
	if left == nil || right == nil {
		switch {
		case left == nil && right == nil:
			return 0
		case left == nil:
			return -1
		}
		return 1
	}
	switch base := legacypkg.BaseType(sqlType); base {
	case "bigint", "int", "smallint", "tinyint", "money", "smallmoney", "decimal", "numeric":
		return numberOf(left).Cmp(numberOf(right))
	case "bit":
		return compareBools(left.(bool), right.(bool))
	case "char", "varchar", "text":
		return compareText(codePageBytes(left.(string)), codePageBytes(right.(string)), []byte(" "))
	case "nchar", "nvarchar", "ntext":
		return compareText(utf16Bytes(left.(string)), utf16Bytes(right.(string)), []byte("\x00 "))
	}
	// Fechas («AAAA-MM-DDTHH:MM:SS.mmm») y lo demás: el texto ya ordena.
	return strings.Compare(left.(string), right.(string))
}

func numberOf(value any) decimal.Decimal {
	return decimal.RequireFromString(string(value.(json.Number)))
}

func compareBools(left, right bool) int {
	switch {
	case left == right:
		return 0
	case !left:
		return -1
	}
	return 1
}

// compareText compara como BIN2: SQL Server rellena el texto más corto con espacios hasta la longitud del otro y
// compara los bytes (así «a» + U+0001 < «a» < «a!», porque U+0001 es menor que el espacio). Si empatan, por
// DATALENGTH (con los espacios del final). space es el espacio en la codificación del texto.
func compareText(left, right []byte, space []byte) int {
	length := max(len(left), len(right))
	if result := bytes.Compare(padded(left, length, space), padded(right, length, space)); result != 0 {
		return result
	}
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	}
	return 0
}

// padded completa con espacios hasta length bytes.
func padded(value []byte, length int, space []byte) []byte {
	result := append([]byte{}, value...)
	for len(result) < length {
		result = append(result, space...)
	}
	return result
}

// codePageBytes es el texto en la página de códigos 1252 (la de la intercalación SQL_Latin1_General_CP1_CI_AS).
func codePageBytes(text string) []byte {
	encoded, err := charmap.Windows1252.NewEncoder().Bytes([]byte(text))
	if err != nil {
		panic("legacysynth: texto fuera de la página de códigos 1252: " + text)
	}
	return encoded
}

// utf16Bytes es el texto en UTF-16, como lo guarda nvarchar: dos bytes por unidad (DATALENGTH), con el byte mayor
// primero para que bytes.Compare ordene como BIN2 compara las unidades.
func utf16Bytes(text string) []byte {
	units := utf16.Encode([]rune(text))
	encoded := make([]byte, 0, len(units)*2)
	for _, unit := range units {
		encoded = append(encoded, byte(unit>>8), byte(unit))
	}
	return encoded
}
