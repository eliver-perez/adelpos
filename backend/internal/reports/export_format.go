package reports

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/internal/moneytext"
)

// Formato de presentación común a los archivos exportados. Los valores vienen tipados del Result
// (decimal.Decimal, int64, time.Time, string) y solo se convierten en texto para mostrarlos: los
// importes con moneytext (las mismas reglas que la pantalla y los tickets), los porcentajes con dos
// decimales, las fechas en dd/mm/aaaa y los instantes en la hora del restaurante.

const (
	exportDateLayout     = "02/01/2006"
	exportDateTimeLayout = "02/01/2006 15:04"
	// exportEmpty es el texto de una celda sin valor en el PDF.
	exportEmpty = "—"
)

// groupDigits separa con comas los miles de la parte entera de un número en texto («-12345.5» →
// «-12,345.5»), como moneytext.
func groupDigits(number string) string {
	sign := ""
	if strings.HasPrefix(number, "-") {
		sign, number = "-", number[1:]
	}
	integer, fraction, hasFraction := strings.Cut(number, ".")
	if len(integer) > 3 {
		var builder strings.Builder
		head := len(integer) % 3
		if head == 0 {
			head = 3
		}
		builder.WriteString(integer[:head])
		for position := head; position < len(integer); position += 3 {
			builder.WriteString(",")
			builder.WriteString(integer[position : position+3])
		}
		integer = builder.String()
	}
	if hasFraction {
		return sign + integer + "." + fraction
	}
	return sign + integer
}

// cleanText quita los caracteres de control de un texto para dibujarlo en una línea del PDF: los
// saltos y tabuladores se vuelven espacios y los demás controles se descartan.
func cleanText(text string) string {
	return strings.Map(func(character rune) rune {
		switch {
		case character == '\n' || character == '\r' || character == '\t':
			return ' '
		case unicode.IsControl(character):
			return -1
		}
		return character
	}, text)
}

// numericColumn indica si la columna se alinea a la derecha y nunca se parte en varias líneas.
func numericColumn(columnType string) bool {
	switch columnType {
	case TypeInteger, TypeQuantity, TypeMoneyMXN, TypeMoneyUSD, TypePercent:
		return true
	}
	return false
}

// displayValue es el texto de un valor tipado tal como lo ve una persona.
func displayValue(columnType string, value any, location *time.Location) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case decimal.Decimal:
		switch columnType {
		case TypeMoneyMXN:
			return moneytext.FormatDecimal(moneytext.CurrencyMXN, typed)
		case TypeMoneyUSD:
			return moneytext.FormatDecimal(moneytext.CurrencyUSD, typed)
		case TypePercent:
			return groupDigits(typed.StringFixed(2)) + " %"
		case TypeInteger:
			return groupDigits(typed.Truncate(0).String())
		}
		return groupDigits(typed.String())
	case int64:
		return groupDigits(strconv.FormatInt(typed, 10))
	case int:
		return groupDigits(strconv.Itoa(typed))
	case time.Time:
		if columnType == TypeDate {
			return typed.UTC().Format(exportDateLayout)
		}
		return typed.In(location).Format(exportDateTimeLayout)
	case bool:
		if typed {
			return "Sí"
		}
		return "No"
	case string:
		return typed
	}
	return ""
}

// localDateTime es un instante en la hora del restaurante.
func localDateTime(instant time.Time, location *time.Location) string {
	return instant.In(location).Format(exportDateTimeLayout)
}

// periodDescription describe el periodo resuelto por Go («Del 1 al 15 de noviembre de 2026»).
func periodDescription(period *businessday.Period) string {
	if period == nil {
		return "Situación al momento de generar el reporte"
	}
	return period.Label
}

// sortDescription describe el orden aplicado («Fecha, ascendente»).
func sortDescription(result *Result) string {
	if result.Sort.Key == "" {
		return "Orden del reporte"
	}
	label := result.Sort.Key
	for _, column := range result.Columns {
		if column.Key == result.Sort.Key {
			label = column.Label
		}
	}
	if result.Sort.Direction == "desc" {
		return label + ", descendente"
	}
	return label + ", ascendente"
}

// filterLines son los filtros aplicados en forma «Etiqueta: valor».
func filterLines(result *Result) []string {
	lines := []string{}
	for _, filter := range result.Meta.Filters {
		lines = append(lines, filter.Label+": "+filter.Display)
	}
	return lines
}

var transliteration = map[rune]string{
	'á': "a", 'é': "e", 'í': "i", 'ó': "o", 'ú': "u", 'ü': "u", 'ñ': "n",
	'Á': "a", 'É': "e", 'Í': "i", 'Ó': "o", 'Ú': "u", 'Ü': "u", 'Ñ': "n",
}

// slug convierte un título en un nombre de archivo ASCII en minúsculas con guiones
// («Ventas por día» → «ventas-por-dia»).
func slug(text string) string {
	var builder strings.Builder
	dash := false
	for _, character := range text {
		if replacement, found := transliteration[character]; found {
			builder.WriteString(replacement)
			dash = false
			continue
		}
		if character < unicode.MaxASCII && (unicode.IsLetter(character) || unicode.IsDigit(character)) {
			builder.WriteRune(unicode.ToLower(character))
			dash = false
			continue
		}
		if !dash && builder.Len() > 0 {
			builder.WriteString("-")
			dash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

// exportFileName es el nombre sugerido del archivo: el reporte y su periodo en ASCII
// («ventas-por-dia_2026-11-01_2026-11-30.pdf»); un reporte sin periodo lleva la fecha comercial de
// generación. Nunca supera 100 caracteres.
func exportFileName(result *Result, format string, location *time.Location) string {
	name := slug(result.Report.Title)
	if name == "" {
		name = "reporte"
	}
	if len(name) > 60 {
		name = strings.Trim(name[:60], "-")
	}
	if period := result.Meta.Period; period != nil {
		name += "_" + period.FirstDay
		if period.LastDay != period.FirstDay {
			name += "_" + period.LastDay
		}
	} else {
		name += "_" + result.Meta.GeneratedAt.In(location).Format("2006-01-02")
	}
	return name + "." + format
}
