package reports

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/businessday"
)

// Modelo de resultado único (§28): la pantalla y las exportaciones (PDF y Excel, fase C) usan el
// mismo Result. Los valores se guardan con su tipo de Go (string, int64, decimal.Decimal,
// time.Time o nil) y solo se formatean al serializar: el dinero y las cantidades viajan como
// cadenas decimales calculadas en Go, nunca como number para que React no haga aritmética.

// Tipos de columna.
const (
	TypeText     = "text"      // texto libre
	TypeCode     = "code"      // código o folio: se exporta como texto (conserva ceros iniciales)
	TypeInteger  = "integer"   // entero (int64)
	TypeQuantity = "quantity"  // cantidad decimal exacta (decimal.Decimal)
	TypeMoneyMXN = "money_mxn" // pesos con dos decimales (decimal.Decimal)
	TypeMoneyUSD = "money_usd" // dólares con dos decimales (decimal.Decimal); nunca se suma con pesos
	TypePercent  = "percent"   // porcentaje con dos decimales (decimal.Decimal, 12.50 = 12.5 %)
	TypeDate     = "date"      // fecha comercial (time.Time a medianoche UTC), AAAA-MM-DD
	TypeDateTime = "datetime"  // instante (time.Time), RFC 3339 en UTC; se muestra en la zona del restaurante
)

// Visibilidad de una columna en la pantalla: las «optional» se ofrecen pero no se muestran de
// inicio. Las columnas que el usuario no puede ver por permisos no se envían (ver Column.Permissions).
const (
	VisibilityDefault  = "default"
	VisibilityOptional = "optional"
)

// Totales de una columna: «sum» suma todo el conjunto filtrado en Go; «custom» lo calcula la función
// Totals del reporte (porcentajes, promedios); «none» deja la celda vacía.
const (
	TotalNone   = "none"
	TotalSum    = "sum"
	TotalCustom = "custom"
)

// Column describe una columna tipada del detalle.
type Column struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Visibility  string `json:"visibility"`
	Total       string `json:"total"`
	Sortable    bool   `json:"sortable"`
	Description string `json:"description,omitempty"`
	// Permissions son permisos adicionales que exige la columna (costos, efectivo físico). Sin
	// ellos la columna, sus valores, sus totales y sus cifras resumidas no se envían ni se exportan.
	Permissions []string `json:"-"`
}

// Row es un renglón con valores tipados por clave de columna.
type Row map[string]any

// Figure es una cifra resumida del reporte (tarjetas de la pantalla y encabezado de exportación).
type Figure struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Value       any      `json:"value"`
	Hint        string   `json:"hint,omitempty"`
	Permissions []string `json:"-"`
}

// Warning es un aviso del reporte (datos incompletos, turnos en arqueo, relleno histórico).
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AppliedFilter es un filtro aplicado, legible para la pantalla y las exportaciones.
type AppliedFilter struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Value   string `json:"value"`
	Display string `json:"display"`
}

// ReportInfo identifica el reporte.
type ReportInfo struct {
	ID          string `json:"id"`
	Group       string `json:"group"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Meta son los metadatos del resultado.
type Meta struct {
	Title       string              `json:"title"`
	Period      *businessday.Period `json:"period"`
	Comparison  *businessday.Period `json:"comparison"`
	Filters     []AppliedFilter     `json:"filters"`
	GeneratedAt time.Time           `json:"generated_at"`
	Timezone    string              `json:"timezone"`
	DayStart    string              `json:"day_start"`
	DateBasis   string              `json:"date_basis"`
	Warnings    []Warning           `json:"warnings"`
	Notes       []string            `json:"notes"`
}

// PageInfo describe la página del detalle.
type PageInfo struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalRows  int `json:"total_rows"`
	TotalPages int `json:"total_pages"`
}

// SortInfo es el orden aplicado.
type SortInfo struct {
	Key       string `json:"key"`
	Direction string `json:"direction"`
}

// Result es el resultado de un reporte: metadatos, columnas visibles, la página del detalle,
// los totales de todo el conjunto filtrado y las cifras resumidas.
type Result struct {
	Report  ReportInfo
	Meta    Meta
	Columns []Column
	// Rows es la página solicitada; AllRows, todo el conjunto filtrado y ordenado (para exportar).
	Rows    []Row
	AllRows []Row
	Totals  Row
	Summary []Figure
	Page    PageInfo
	Sort    SortInfo
}

// MarshalJSON formatea cada valor según el tipo de su columna. AllRows nunca se serializa.
func (result *Result) MarshalJSON() ([]byte, error) {
	rows := make([]map[string]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		rows = append(rows, formatRow(result.Columns, row))
	}
	var totals map[string]any
	if result.Totals != nil {
		totals = formatRow(result.Columns, result.Totals)
	}
	summary := make([]map[string]any, 0, len(result.Summary))
	for _, figure := range result.Summary {
		entry := map[string]any{
			"key": figure.Key, "label": figure.Label, "type": figure.Type, "value": FormatValue(figure.Type, figure.Value),
		}
		if figure.Hint != "" {
			entry["hint"] = figure.Hint
		}
		summary = append(summary, entry)
	}
	meta := result.Meta
	if meta.Filters == nil {
		meta.Filters = []AppliedFilter{}
	}
	if meta.Warnings == nil {
		meta.Warnings = []Warning{}
	}
	if meta.Notes == nil {
		meta.Notes = []string{}
	}
	columns := result.Columns
	if columns == nil {
		columns = []Column{}
	}
	return json.Marshal(map[string]any{
		"report":  result.Report,
		"meta":    meta,
		"columns": columns,
		"rows":    rows,
		"totals":  totals,
		"summary": summary,
		"page":    result.Page,
		"sort":    result.Sort,
	})
}

func formatRow(columns []Column, row Row) map[string]any {
	formatted := make(map[string]any, len(columns))
	for _, column := range columns {
		formatted[column.Key] = FormatValue(column.Type, row[column.Key])
	}
	return formatted
}

// FormatValue convierte un valor tipado en su forma JSON: dinero y porcentajes con dos decimales
// como cadena, cantidades como cadena decimal exacta, enteros como número, fechas como AAAA-MM-DD e
// instantes en RFC 3339 (UTC). nil sigue siendo nil.
func FormatValue(columnType string, value any) any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case decimal.Decimal:
		switch columnType {
		case TypeMoneyMXN, TypeMoneyUSD, TypePercent:
			return typed.StringFixed(2)
		case TypeInteger:
			return typed.IntPart()
		}
		return typed.String()
	case time.Time:
		if columnType == TypeDate {
			return typed.Format("2006-01-02")
		}
		return typed.UTC().Format(time.RFC3339)
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	}
	return value
}
