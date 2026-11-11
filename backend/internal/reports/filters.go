package reports

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Filtros reutilizables (§7). Cada reporte declara solo los filtros que tienen sentido para él; los
// demás parámetros se rechazan. Los filtros de tipo «id» reciben un UUID cuyas opciones se consultan
// en GET reports/filters/{source}; los de tipo «choice» tienen una lista cerrada; los de tipo «text»
// aceptan hasta 80 caracteres.

const (
	FilterID     = "id"
	FilterChoice = "choice"
	FilterText   = "text"
)

// NoReason es el valor del filtro de motivo para los descuentos sin motivo del catálogo.
const NoReason = "none"

// NoReasonLabel es la clasificación de los descuentos sin motivo del catálogo (§11).
const NoReasonLabel = "Sin motivo del catálogo"

// Option es una opción de un filtro.
type Option struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}

// Filter es un filtro admitido por un reporte.
type Filter struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"`
	Source  string   `json:"source,omitempty"`
	Options []Option `json:"options,omitempty"`
	Default string   `json:"default,omitempty"`
}

func idFilter(key, label, source string) Filter {
	return Filter{Key: key, Label: label, Type: FilterID, Source: source}
}

func filterRegister() Filter  { return idFilter("register_id", "Caja", "registers") }
func filterShift() Filter     { return idFilter("shift_id", "Turno", "shifts") }
func filterArea() Filter      { return idFilter("area_id", "Área", "areas") }
func filterTable() Filter     { return idFilter("table_id", "Mesa", "tables") }
func filterProduct() Filter   { return idFilter("product_id", "Producto", "products") }
func filterCategory() Filter  { return idFilter("category_id", "Categoría", "categories") }
func filterMethod() Filter    { return idFilter("method_id", "Método de pago", "payment_methods") }
func filterPriceList() Filter { return idFilter("price_list_id", "Lista de precios", "price_lists") }
func filterUser(label string) Filter {
	return idFilter("user_id", label, "users")
}

// filterReason admite además NoReason («Sin motivo del catálogo»).
func filterReason() Filter {
	return Filter{Key: "reason_id", Label: "Motivo", Type: FilterID, Source: "discount_reasons"}
}

func filterMode() Filter {
	return filterChoice("mode", "Modalidad", "",
		Option{Value: "dine_in", Label: "Salón"},
		Option{Value: "takeaway", Label: "Para llevar y ventanilla"})
}

func filterOrigin() Filter {
	return Filter{Key: "origin", Label: "Origen (para llevar)", Type: FilterText, Source: "origins"}
}

func filterChoice(key, label, defaultValue string, options ...Option) Filter {
	return Filter{Key: key, Label: label, Type: FilterChoice, Options: options, Default: defaultValue}
}

// filterSources es la lista cerrada de fuentes de opciones de GET reports/filters/{source}.
var filterSources = map[string]string{
	"registers": `SELECT id::text AS value,name AS label,CASE WHEN active THEN '' ELSE 'Inactiva' END AS detail
        FROM app.registers ORDER BY name`,
	// Solo los turnos que el usuario puede consultar en Caja, como los reportes que usan este filtro.
	"shifts": `SELECT s.id::text AS value,
            r.name||' · '||to_char(coalesce(s.business_date,(s.opened_at AT TIME ZONE coalesce(e.timezone,'UTC'))::date),'DD/MM/YYYY')
                ||' · '||u.display_name AS label,
            CASE s.state WHEN 'open' THEN 'Abierto' WHEN 'closing' THEN 'En arqueo' ELSE 'Cerrado' END AS detail
        FROM app.cash_shifts s JOIN app.registers r ON r.id=s.register_id JOIN app.users u ON u.id=s.opened_by
        LEFT JOIN app.establishment e ON true
        WHERE ` + cashScopeSQL + `
        ORDER BY s.opened_at DESC LIMIT 200`,
	"users": `SELECT id::text AS value,display_name AS label,CASE WHEN active THEN '' ELSE 'Inactivo' END AS detail
        FROM app.users WHERE NOT support ORDER BY display_name`,
	"areas": `SELECT id::text AS value,name AS label,CASE WHEN active THEN '' ELSE 'Inactiva' END AS detail
        FROM app.restaurant_areas ORDER BY sort_order,name`,
	"tables": `SELECT t.id::text AS value,t.name AS label,a.name AS detail
        FROM app.restaurant_tables t JOIN app.restaurant_areas a ON a.id=t.area_id ORDER BY a.sort_order,a.name,t.name`,
	"products": `SELECT id::text AS value,name AS label,code AS detail FROM app.catalog_products
        WHERE @search='' OR name ILIKE '%'||@search||'%' OR code ILIKE '%'||@search||'%'
        ORDER BY name LIMIT 100`,
	"categories": `SELECT id::text AS value,name AS label,CASE WHEN active THEN '' ELSE 'Inactiva' END AS detail
        FROM app.catalog_categories ORDER BY sort_order,name`,
	"payment_methods": `SELECT id::text AS value,name AS label,CASE WHEN active THEN '' ELSE 'Inactivo' END AS detail
        FROM app.payment_methods ORDER BY sort_order,name`,
	"price_lists": `SELECT id::text AS value,name AS label,CASE WHEN active THEN '' ELSE 'Inactiva' END AS detail
        FROM app.catalog_price_lists ORDER BY name`,
	"discount_reasons": `SELECT value,label,detail FROM (
            SELECT 0 AS position,'none' AS value,'Sin motivo del catálogo' AS label,'' AS detail,'' AS name
            UNION ALL
            SELECT 1,id::text,name,CASE WHEN active THEN '' ELSE 'Inactivo' END,name FROM app.discount_reasons
        ) reasons ORDER BY position,name`,
	"origins": `SELECT DISTINCT origin AS value,origin AS label,'' AS detail FROM app.restaurant_accounts
        WHERE mode='takeaway' ORDER BY origin LIMIT 100`,
}

// filterAllowed indica si alguno de los reportes que el actor puede consultar usa la fuente.
func filterAllowed(actor Actor, source string) bool {
	for _, definition := range Definitions() {
		if !definition.Allowed(actor) {
			continue
		}
		for _, filter := range definition.Filters {
			if filter.Source == source {
				return true
			}
		}
	}
	return false
}

// filterOptions devuelve las opciones de una fuente de filtros. Solo nombres y estados, nunca
// importes, y solo si el actor puede consultar al menos un reporte que use la fuente (si no, 403).
// search (opcional) busca productos por nombre o código.
func filterOptions(ctx context.Context, tx pgx.Tx, actor Actor, source, search string) ([]Option, error) {
	query, found := filterSources[source]
	if !found {
		return nil, notFound("El filtro no existe.")
	}
	if !filterAllowed(actor, source) {
		return nil, denied()
	}
	search = strings.TrimSpace(search)
	if len([]rune(search)) > 80 {
		return nil, invalid("La búsqueda admite hasta 80 caracteres.")
	}
	rows, err := tx.Query(ctx, query, withCashScope(actor, pgx.NamedArgs{"search": search}))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	options := []Option{}
	for rows.Next() {
		var option Option
		if err := rows.Scan(&option.Value, &option.Label, &option.Detail); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	return options, rows.Err()
}

// filterNames obtiene el nombre legible del valor de un filtro de tipo «id» para los metadatos.
var filterNames = map[string]string{
	"registers": `SELECT name FROM app.registers WHERE id=@value::uuid`,
	"shifts": `SELECT r.name||' · '||to_char(coalesce(s.business_date,(s.opened_at AT TIME ZONE coalesce(e.timezone,'UTC'))::date),
            'DD/MM/YYYY')||' · '||u.display_name
        FROM app.cash_shifts s JOIN app.registers r ON r.id=s.register_id JOIN app.users u ON u.id=s.opened_by
        LEFT JOIN app.establishment e ON true WHERE s.id=@value::uuid AND ` + cashScopeSQL,
	"users":            `SELECT display_name FROM app.users WHERE id=@value::uuid`,
	"areas":            `SELECT name FROM app.restaurant_areas WHERE id=@value::uuid`,
	"tables":           `SELECT t.name||' · '||a.name FROM app.restaurant_tables t JOIN app.restaurant_areas a ON a.id=t.area_id WHERE t.id=@value::uuid`,
	"products":         `SELECT name FROM app.catalog_products WHERE id=@value::uuid`,
	"categories":       `SELECT name FROM app.catalog_categories WHERE id=@value::uuid`,
	"payment_methods":  `SELECT name FROM app.payment_methods WHERE id=@value::uuid`,
	"price_lists":      `SELECT name FROM app.catalog_price_lists WHERE id=@value::uuid`,
	"discount_reasons": `SELECT name FROM app.discount_reasons WHERE id=@value::uuid`,
}

// describeFilters arma la lista legible de filtros aplicados (pantalla y exportaciones). El nombre
// de un turno fuera del alcance de Caja del actor no se revela («Registro no encontrado»).
func describeFilters(ctx context.Context, tx pgx.Tx, actor Actor, definition *Definition, params *Params) ([]AppliedFilter, error) {
	applied := []AppliedFilter{}
	for _, filter := range definition.Filters {
		value := params.Values[filter.Key]
		if value == "" {
			continue
		}
		display := value
		switch {
		case filter.Type == FilterChoice:
			for _, option := range filter.Options {
				if option.Value == value {
					display = option.Label
				}
			}
		case filter.Key == "reason_id" && value == NoReason:
			display = NoReasonLabel
		case filter.Type == FilterID:
			display = "Registro no encontrado"
			if query, found := filterNames[filter.Source]; found {
				var name string
				err := tx.QueryRow(ctx, query, withCashScope(actor, pgx.NamedArgs{"value": value})).Scan(&name)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return nil, err
				}
				if err == nil {
					display = name
				}
			}
		}
		applied = append(applied, AppliedFilter{Key: filter.Key, Label: filter.Label, Value: value, Display: display})
	}
	return applied, nil
}
