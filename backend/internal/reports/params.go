package reports

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/businessday"
)

// Parámetros de GET reports/run/{id} y GET reports/dashboard:
//   - periodo: period (today, yesterday, current_week, current_month, current_year, day, iso_week,
//     month, year, range) con date, week, month, year o from/until según la clase;
//   - compare=previous: periodo anterior equivalente (si el reporte lo admite);
//   - page (desde 1), page_size (1 a MaxPageSize), sort (clave de una columna ordenable) y direction
//     (asc o desc);
//   - los filtros que declara el reporte. Cualquier otro parámetro se rechaza.

const (
	DefaultPageSize = 50
	MaxPageSize     = 200
	// MaxRows es el máximo de renglones que un reporte procesa en memoria (pantalla o exportación).
	MaxRows = 100000
)

var reservedParameters = map[string]bool{
	"period": true, "date": true, "week": true, "month": true, "year": true, "from": true, "until": true,
	"compare": true, "page": true, "page_size": true, "sort": true, "direction": true,
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// rawParams son los parámetros validados de forma (sin resolver el periodo, que necesita la zona
// horaria, la hora de inicio y la hora de la fotografía).
type rawParams struct {
	Spec     businessday.Spec
	Compare  bool
	Page     int
	PageSize int
	Sort     *Sort
	Values   map[string]string
}

// Params son los parámetros resueltos que recibe la consulta.
type Params struct {
	Period   *businessday.Period
	Previous *businessday.Period
	Page     int
	PageSize int
	Sort     Sort
	Values   map[string]string
}

// Value devuelve el valor de un filtro («» si no se indicó y no tiene valor por omisión).
func (params Params) Value(key string) string {
	return params.Values[key]
}

func single(values url.Values, key string) (string, error) {
	list := values[key]
	if len(list) > 1 {
		return "", invalid("El parámetro " + key + " se indicó más de una vez.")
	}
	if len(list) == 0 {
		return "", nil
	}
	return strings.TrimSpace(list[0]), nil
}

func positive(values url.Values, key string, fallback, maximum int) (int, error) {
	text, err := single(values, key)
	if err != nil || text == "" {
		return fallback, err
	}
	number, err := strconv.Atoi(text)
	if err != nil || number < 1 || number > maximum {
		return 0, invalid("El parámetro " + key + " no es válido.")
	}
	return number, nil
}

// parseParams valida los parámetros de un reporte (o del dashboard si definition es nil).
func parseParams(values url.Values, definition *Definition) (*rawParams, error) {
	allowed := map[string]Filter{}
	if definition != nil {
		for _, filter := range definition.Filters {
			allowed[filter.Key] = filter
		}
	}
	for key := range values {
		if reservedParameters[key] {
			continue
		}
		if _, found := allowed[key]; !found {
			return nil, invalid("El parámetro " + key + " no se admite en este reporte.")
		}
	}
	params := &rawParams{Values: map[string]string{}}
	var err error
	fields := []struct {
		key    string
		target *string
	}{
		{"period", &params.Spec.Kind}, {"date", &params.Spec.Date}, {"week", &params.Spec.Week}, {"month", &params.Spec.Month},
		{"year", &params.Spec.Year}, {"from", &params.Spec.From}, {"until", &params.Spec.Until},
	}
	for _, field := range fields {
		if *field.target, err = single(values, field.key); err != nil {
			return nil, err
		}
	}
	compare, err := single(values, "compare")
	if err != nil {
		return nil, err
	}
	switch compare {
	case "", "none":
	case "previous":
		if definition != nil && !definition.Comparison {
			return nil, invalid("Este reporte no admite comparación con el periodo anterior.")
		}
		params.Compare = true
	default:
		return nil, invalid("La comparación debe ser previous o none.")
	}
	if definition != nil && definition.Period == PeriodNone && params.Spec.Kind != "" {
		return nil, invalid("Este reporte muestra la situación actual y no admite periodo.")
	}
	if params.Page, err = positive(values, "page", 1, 1000000); err != nil {
		return nil, err
	}
	if params.PageSize, err = positive(values, "page_size", DefaultPageSize, MaxPageSize); err != nil {
		return nil, err
	}
	sortKey, err := single(values, "sort")
	if err != nil {
		return nil, err
	}
	direction, err := single(values, "direction")
	if err != nil {
		return nil, err
	}
	if direction != "" && direction != "asc" && direction != "desc" {
		return nil, invalid("La dirección del orden debe ser asc o desc.")
	}
	if sortKey != "" && definition != nil {
		sortable := false
		for _, column := range definition.Columns {
			if column.Key == sortKey && column.Sortable {
				sortable = true
			}
		}
		if !sortable {
			return nil, invalid("No se puede ordenar por " + sortKey + ".")
		}
		params.Sort = &Sort{Key: sortKey, Descending: direction == "desc"}
	} else if direction != "" && definition != nil {
		params.Sort = &Sort{Key: definition.DefaultSort.Key, Descending: direction == "desc"}
	}
	if definition != nil {
		for _, filter := range definition.Filters {
			value, err := single(values, filter.Key)
			if err != nil {
				return nil, err
			}
			if value == "" {
				value = filter.Default
			}
			if value == "" {
				continue
			}
			if err := validateFilter(filter, value); err != nil {
				return nil, err
			}
			params.Values[filter.Key] = value
		}
	}
	return params, nil
}

func validateFilter(filter Filter, value string) error {
	switch filter.Type {
	case FilterID:
		if filter.Key == "reason_id" && value == NoReason {
			return nil
		}
		if !uuidPattern.MatchString(value) {
			return invalid("El filtro «" + filter.Label + "» no es válido.")
		}
	case FilterChoice:
		for _, option := range filter.Options {
			if option.Value == value {
				return nil
			}
		}
		return invalid("El filtro «" + filter.Label + "» no admite ese valor.")
	case FilterText:
		if len([]rune(value)) > 80 {
			return invalid("El filtro «" + filter.Label + "» admite hasta 80 caracteres.")
		}
	}
	return nil
}

// resolve convierte los parámetros en definitivos con la configuración y la hora de la fotografía.
func (raw *rawParams) resolve(definition *Definition, now time.Time, config businessday.Config) (*Params, error) {
	params := &Params{Page: raw.Page, PageSize: raw.PageSize, Values: raw.Values}
	if definition != nil {
		params.Sort = definition.DefaultSort
		if raw.Sort != nil {
			params.Sort = *raw.Sort
		}
	}
	if definition == nil || definition.Period != PeriodNone {
		period, err := businessday.Resolve(raw.Spec, now, config)
		if err != nil {
			return nil, invalid(err.Error())
		}
		params.Period = &period
		if raw.Compare {
			previous := period.Previous(now, config)
			params.Previous = &previous
		}
	}
	return params, nil
}
