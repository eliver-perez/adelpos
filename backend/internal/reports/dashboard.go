package reports

import (
	"context"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/businessday"
)

// Resumen ejecutivo (§5). Cada sección agrupa indicadores con el mismo permiso y los calcula para el
// periodo y para el anterior equivalente en la misma fotografía. Una sección sin permiso no se
// calcula ni se envía. El dashboard no se recalcula con cada evento SSE: la pantalla lo actualiza a
// pedido o con un temporizador moderado. La fase B2 agrega caja, compras, consumo, mermas, costos y
// margen con registerDashboard.

// IndicatorDefinition describe un indicador.
type IndicatorDefinition struct {
	Key   string
	Label string
	Type  string
	// ReportID es el reporte detallado al que se navega desde el indicador.
	ReportID string
	Hint     string
}

// DashboardSection es una sección del resumen.
type DashboardSection struct {
	ID          string
	Title       string
	Permissions []string
	Indicators  []IndicatorDefinition
	// Compute devuelve el valor de cada indicador en un periodo (decimal.Decimal o int64; nil si no
	// hay datos suficientes).
	Compute func(ctx context.Context, query *Query, period *businessday.Period) (map[string]any, error)
	// Series devuelve series opcionales del periodo actual (tendencias).
	Series func(ctx context.Context, query *Query) ([]Series, error)
	// CashScoped indica que la sección se limita a los turnos que el usuario puede consultar en
	// Caja (cashScopeSQL); el resumen avisa CASH_SCOPE una vez si el alcance es limitado.
	CashScoped bool
}

// Point es un punto de una serie por día comercial.
type Point struct {
	Date  time.Time
	Value decimal.Decimal
}

// Series es una serie de tendencia.
type Series struct {
	Key      string
	Label    string
	Type     string
	ReportID string
	Points   []Point
}

var dashboardSections = []DashboardSection{}

func registerDashboard(section DashboardSection) {
	for _, existing := range dashboardSections {
		if existing.ID == section.ID {
			panic("sección duplicada del dashboard: " + section.ID)
		}
	}
	dashboardSections = append(dashboardSections, section)
}

// Indicator es un indicador calculado.
type Indicator struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	Type          string `json:"type"`
	Value         any    `json:"value"`
	Previous      any    `json:"previous"`
	Change        any    `json:"change"`
	ChangePercent any    `json:"change_percent"`
	ReportID      string `json:"report_id,omitempty"`
	Hint          string `json:"hint,omitempty"`
}

// DashboardSectionResult es una sección calculada.
type DashboardSectionResult struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Indicators []Indicator `json:"indicators"`
}

// SeriesResult es una serie con sus valores formateados.
type SeriesResult struct {
	Key      string           `json:"key"`
	Label    string           `json:"label"`
	Type     string           `json:"type"`
	ReportID string           `json:"report_id,omitempty"`
	Points   []map[string]any `json:"points"`
}

// Dashboard es la respuesta de GET reports/dashboard.
type Dashboard struct {
	Period      *businessday.Period      `json:"period"`
	Comparison  *businessday.Period      `json:"comparison"`
	GeneratedAt time.Time                `json:"generated_at"`
	Timezone    string                   `json:"timezone"`
	DayStart    string                   `json:"day_start"`
	Sections    []DashboardSectionResult `json:"sections"`
	Series      []SeriesResult           `json:"series"`
	Warnings    []Warning                `json:"warnings"`
}

// parseDashboardParams admite el periodo y compare (por omisión, el periodo anterior equivalente).
func parseDashboardParams(values url.Values) (*rawParams, error) {
	for key := range values {
		switch key {
		case "period", "date", "week", "month", "year", "from", "until", "compare":
		default:
			return nil, invalid("El parámetro " + key + " no se admite en el resumen ejecutivo.")
		}
	}
	raw, err := parseParams(values, nil)
	if err != nil {
		return nil, err
	}
	if values.Get("compare") == "" {
		raw.Compare = true
	}
	return raw, nil
}

func buildDashboard(ctx context.Context, tx pgx.Tx, actor Actor, raw *rawParams) (*Dashboard, error) {
	config, now, err := snapshot(ctx, tx)
	if err != nil {
		return nil, err
	}
	params, err := raw.resolve(nil, now, config)
	if err != nil {
		return nil, err
	}
	query := &Query{Tx: tx, Config: config, Params: params, Actor: actor, Now: now}
	result := &Dashboard{
		Period: params.Period, Comparison: params.Previous, GeneratedAt: now, Timezone: config.Timezone,
		DayStart: config.Cutoff(), Sections: []DashboardSectionResult{}, Series: []SeriesResult{}, Warnings: []Warning{},
	}
	scopeWarned := false
	for _, section := range dashboardSections {
		if !actor.CanAll(section.Permissions) {
			continue
		}
		current, err := section.Compute(ctx, query, params.Period)
		if err != nil {
			return nil, err
		}
		previous := map[string]any{}
		if params.Previous != nil {
			if previous, err = section.Compute(ctx, query, params.Previous); err != nil {
				return nil, err
			}
		}
		computed := DashboardSectionResult{ID: section.ID, Title: section.Title, Indicators: []Indicator{}}
		for _, definition := range section.Indicators {
			indicator := Indicator{
				Key: definition.Key, Label: definition.Label, Type: definition.Type, Hint: definition.Hint,
				Value: FormatValue(definition.Type, current[definition.Key]),
			}
			if report, found := Lookup(definition.ReportID); found && report.Allowed(actor) {
				indicator.ReportID = definition.ReportID
			}
			if params.Previous != nil {
				indicator.Previous = FormatValue(definition.Type, previous[definition.Key])
				change, percent := compareIndicator(current[definition.Key], previous[definition.Key])
				indicator.Change = FormatValue(definition.Type, change)
				indicator.ChangePercent = FormatValue(TypePercent, percent)
			}
			computed.Indicators = append(computed.Indicators, indicator)
		}
		result.Sections = append(result.Sections, computed)
		if section.CashScoped && cashScopeLimited(actor) && !scopeWarned {
			result.Warnings = append(result.Warnings, cashScopeWarning())
			scopeWarned = true
		}
		if section.Series == nil {
			continue
		}
		series, err := section.Series(ctx, query)
		if err != nil {
			return nil, err
		}
		for _, entry := range series {
			formatted := SeriesResult{Key: entry.Key, Label: entry.Label, Type: entry.Type, Points: []map[string]any{}}
			if report, found := Lookup(entry.ReportID); found && report.Allowed(actor) {
				formatted.ReportID = entry.ReportID
			}
			for _, point := range entry.Points {
				formatted.Points = append(formatted.Points, map[string]any{
					"date": point.Date.Format("2006-01-02"), "value": FormatValue(entry.Type, point.Value),
				})
			}
			result.Series = append(result.Series, formatted)
		}
	}
	return result, nil
}

// compareIndicator devuelve la variación absoluta y porcentual (nil si falta un valor o el anterior
// es cero).
func compareIndicator(current, previous any) (any, any) {
	if current == nil || previous == nil {
		return nil, nil
	}
	now := decimalValue(current)
	before := decimalValue(previous)
	change := now.Sub(before)
	var percent any
	if !before.IsZero() {
		percent = change.Mul(decimal.NewFromInt(100)).DivRound(before.Abs(), 2)
	}
	if _, isInteger := current.(int64); isInteger {
		return change.IntPart(), percent
	}
	return change, percent
}

func decimalValue(value any) decimal.Decimal {
	switch typed := value.(type) {
	case decimal.Decimal:
		return typed
	case int64:
		return decimal.NewFromInt(typed)
	}
	return decimal.Zero
}
