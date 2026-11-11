package reports

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
)

// Registro cerrado de reportes (§29: ninguna ruta acepta SQL). Cada reporte se declara con
// register(...) en el init de su archivo y queda identificado por un id estable («sales.by_day»),
// que usan la pantalla, el puente nativo y las exportaciones. Ver doc.go, «Cómo agregar un reporte».

// Period indica si el reporte se filtra por periodo o es una fotografía del momento («En curso»).
const (
	PeriodRequired = "required"
	PeriodNone     = "none"
)

// Group es un grupo de navegación del módulo Reportes (§4).
type Group struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Groups es la lista cerrada de grupos en su orden de navegación. Los grupos sin reportes visibles
// para el usuario no se envían en el catálogo.
var Groups = []Group{
	{"summary", "Resumen ejecutivo"},
	{"sales", "Ventas"},
	{"products", "Productos"},
	{"categories", "Categorías"},
	{"payments", "Pagos"},
	{"discounts", "Descuentos y cortesías"},
	{"cash", "Caja"},
	{"kitchen", "Cocina y operación"},
	{"inventory", "Inventarios"},
	{"purchases", "Compras"},
	{"suppliers", "Proveedores"},
	{"costs", "Costos y márgenes"},
	{"employees", "Empleados"},
}

// Sort es un orden del detalle.
type Sort struct {
	Key        string
	Descending bool
}

// Dataset es lo que devuelve la consulta de un reporte: todos los renglones del conjunto filtrado
// (el motor ordena, pagina y totaliza en Go), cifras resumidas, avisos y notas.
type Dataset struct {
	Rows     []Row
	Summary  []Figure
	Warnings []Warning
	Notes    []string
}

// Definition es la declaración de un reporte.
type Definition struct {
	ID          string
	Group       string
	Title       string
	Description string
	// Permissions se exigen todos (además de reports.read) para ver el reporte.
	Permissions []string
	// Period es PeriodRequired (por omisión) o PeriodNone.
	Period string
	// Comparison indica si admite compare=previous (periodo anterior equivalente).
	Comparison bool
	// DateBasis explica qué fecha usa el reporte (§6, «Fechas»).
	DateBasis   string
	Filters     []Filter
	Columns     []Column
	DefaultSort Sort
	Query       func(ctx context.Context, query *Query) (*Dataset, error)
	// Totals completa las columnas con Total «custom» a partir de todos los renglones y de los
	// totales sumados.
	Totals func(rows []Row, totals Row)
}

var (
	reportIDPattern = regexp.MustCompile(`^[a-z]+(\.[a-z_]+)+$`)
	registry        = map[string]*Definition{}
	registryOrder   = []string{}
)

var columnTypes = []string{TypeText, TypeCode, TypeInteger, TypeQuantity, TypeMoneyMXN, TypeMoneyUSD, TypePercent, TypeDate, TypeDateTime}

// register agrega un reporte al registro. Un error de declaración detiene el arranque (es un error
// de programación, no de datos).
func register(definition Definition) {
	if !reportIDPattern.MatchString(definition.ID) || len(definition.ID) > 64 {
		panic("id de reporte inválido: " + definition.ID)
	}
	if _, exists := registry[definition.ID]; exists {
		panic("reporte duplicado: " + definition.ID)
	}
	if !slices.ContainsFunc(Groups, func(group Group) bool { return group.ID == definition.Group }) {
		panic("grupo desconocido en " + definition.ID)
	}
	if definition.Period == "" {
		definition.Period = PeriodRequired
	}
	if definition.Query == nil || definition.Title == "" || definition.DateBasis == "" || len(definition.Columns) == 0 {
		panic("reporte incompleto: " + definition.ID)
	}
	keys := map[string]bool{}
	for index, column := range definition.Columns {
		if keys[column.Key] || !slices.Contains(columnTypes, column.Type) || column.Label == "" {
			panic(fmt.Sprintf("columna inválida %q en %s", column.Key, definition.ID))
		}
		keys[column.Key] = true
		if column.Visibility == "" {
			definition.Columns[index].Visibility = VisibilityDefault
		}
		if column.Total == "" {
			definition.Columns[index].Total = TotalNone
		}
	}
	if definition.DefaultSort.Key != "" && !keys[definition.DefaultSort.Key] {
		panic("orden por omisión desconocido en " + definition.ID)
	}
	filterKeys := map[string]bool{}
	for _, filter := range definition.Filters {
		if filterKeys[filter.Key] || reservedParameters[filter.Key] {
			panic("filtro duplicado o reservado " + filter.Key + " en " + definition.ID)
		}
		filterKeys[filter.Key] = true
	}
	stored := definition
	registry[definition.ID] = &stored
	registryOrder = append(registryOrder, definition.ID)
}

// Lookup devuelve la definición de un reporte.
func Lookup(id string) (*Definition, bool) {
	definition, found := registry[id]
	return definition, found
}

// Definitions devuelve los reportes en el orden de sus grupos y de registro.
func Definitions() []*Definition {
	position := map[string]int{}
	for index, group := range Groups {
		position[group.ID] = index
	}
	result := make([]*Definition, 0, len(registryOrder))
	for _, id := range registryOrder {
		result = append(result, registry[id])
	}
	sort.SliceStable(result, func(left, right int) bool {
		return position[result[left].Group] < position[result[right].Group]
	})
	return result
}

// Actor es quien consulta, con sus permisos efectivos leídos en la transacción de autenticación.
type Actor struct {
	UserID      string
	DeviceID    string
	Permissions []string
}

// Can indica si el actor tiene el permiso.
func (actor Actor) Can(permission string) bool {
	return slices.Contains(actor.Permissions, permission)
}

// CanAll indica si el actor tiene todos los permisos.
func (actor Actor) CanAll(permissions []string) bool {
	for _, permission := range permissions {
		if !actor.Can(permission) {
			return false
		}
	}
	return true
}

// Allowed indica si el actor puede consultar el reporte.
func (definition *Definition) Allowed(actor Actor) bool {
	return actor.Can(PermissionRead) && actor.CanAll(definition.Permissions)
}
