package inventory

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Ayudantes de consulta para listados con filtros, búsqueda y paginación. Los parámetros se
// validan contra una lista cerrada por ruta (el puente nativo también los limita).

const (
	defaultPageSize = 25
	maximumPageSize = 100
	maximumPage     = 10000
)

// allowQuery rechaza parámetros que la ruta no admite.
func allowQuery(query url.Values, keys ...string) error {
	for key := range query {
		if !slices.Contains(keys, key) {
			return invalid("Parámetro de consulta no permitido: " + key + ".")
		}
	}
	return nil
}

// pagination lee page (desde 1) y page_size (1–100, 25 por omisión).
func pagination(query url.Values) (int, int, error) {
	page, size := 1, defaultPageSize
	for key, target := range map[string]*int{"page": &page, "page_size": &size} {
		text := query.Get(key)
		if text == "" {
			continue
		}
		value, err := strconv.Atoi(text)
		if err != nil || value < 1 {
			return 0, 0, invalid("La página y su tamaño deben ser enteros positivos.")
		}
		*target = value
	}
	if page > maximumPage || size > maximumPageSize {
		return 0, 0, invalid("La página máxima es 10000 y el tamaño máximo es 100.")
	}
	return page, size, nil
}

// searchPattern prepara un patrón ILIKE con los comodines del texto escapados.
func searchPattern(query url.Values) (string, error) {
	search := strings.TrimSpace(query.Get("search"))
	if search == "" {
		return "", nil
	}
	if textLength(search) > 100 {
		return "", invalid("La búsqueda admite hasta 100 caracteres.")
	}
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(search) + "%", nil
}

// optionalBool lee un filtro true/false.
func optionalBool(query url.Values, key, subject string) (*bool, error) {
	text := query.Get(key)
	if text == "" {
		return nil, nil
	}
	if text != "true" && text != "false" {
		return nil, invalid("El filtro de " + subject + " no es válido.")
	}
	value := text == "true"
	return &value, nil
}

// optionalQueryID lee un filtro de identificador.
func optionalQueryID(query url.Values, key, subject string) (*string, error) {
	text := query.Get(key)
	if text == "" {
		return nil, nil
	}
	if !validID(text) {
		return nil, invalid("El filtro de " + subject + " no es válido.")
	}
	normalized := strings.ToLower(text)
	return &normalized, nil
}

// filters arma una cláusula WHERE con argumentos numerados.
type filters struct {
	clauses   []string
	arguments []any
}

func (set *filters) add(expression string, value any) {
	set.arguments = append(set.arguments, value)
	set.clauses = append(set.clauses, fmt.Sprintf(expression, len(set.arguments)))
}

func (set *filters) where() string {
	if len(set.clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(set.clauses, " AND ")
}

// next devuelve el número del siguiente argumento para LIMIT/OFFSET.
func (set *filters) next(value any) string {
	set.arguments = append(set.arguments, value)
	return "$" + strconv.Itoa(len(set.arguments))
}

// establishmentLocation es la zona horaria IANA del establecimiento (UTC si falta o es inválida).
func (handler *service) establishmentLocation() (*time.Location, error) {
	var name string
	err := handler.tx.QueryRow(handler.ctx, "SELECT timezone FROM app.establishment").Scan(&name)
	if isNoRows(err) {
		return time.UTC, nil
	}
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC, nil
	}
	return location, nil
}

// dateRange interpreta from y until (AAAA-MM-DD) como días completos en la zona horaria del
// establecimiento: from desde el inicio de su día y until hasta el final del suyo.
func (handler *service) dateRange(query url.Values) (*time.Time, *time.Time, error) {
	fromText, untilText := query.Get("from"), query.Get("until")
	if fromText == "" && untilText == "" {
		return nil, nil, nil
	}
	location, err := handler.establishmentLocation()
	if err != nil {
		return nil, nil, err
	}
	var from, until *time.Time
	if fromText != "" {
		day, err := time.ParseInLocation("2006-01-02", fromText, location)
		if err != nil {
			return nil, nil, invalid("La fecha inicial debe tener el formato AAAA-MM-DD.")
		}
		from = &day
	}
	if untilText != "" {
		day, err := time.ParseInLocation("2006-01-02", untilText, location)
		if err != nil {
			return nil, nil, invalid("La fecha final debe tener el formato AAAA-MM-DD.")
		}
		end := day.AddDate(0, 0, 1)
		until = &end
	}
	if from != nil && until != nil && !from.Before(*until) {
		return nil, nil, invalid("La fecha inicial no puede ser posterior a la final.")
	}
	return from, until, nil
}
