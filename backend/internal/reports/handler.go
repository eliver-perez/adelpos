package reports

import (
	"context"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/businessday"
)

// Rutas del módulo (todas GET y con reports.read):
//   - reports/catalog: reportes y filtros visibles para el usuario;
//   - reports/run/{id}: ejecuta un reporte del registro cerrado;
//   - reports/export/{id}?format=pdf|xlsx: el mismo reporte como archivo (export.go), con
//     reports.export.pdf o reports.export.excel;
//   - reports/dashboard: indicadores del resumen ejecutivo, cada uno con su permiso;
//   - reports/filters/{source}: opciones de un filtro (cajas, turnos, usuarios, áreas, mesas,
//     productos, categorías, métodos de pago, listas de precios, motivos y orígenes).
//
// Handle se llama dentro de la transacción de autenticación de administration: valida la ruta, los
// permisos y la forma de los parámetros (una denegación se audita y se confirma como en el resto de
// la API). El catálogo y las opciones de filtros son lecturas pequeñas sin locks y se resuelven ahí.
// La ejecución de un reporte o del dashboard queda en un Plan que administration ejecuta después de
// confirmar esa transacción, en el Runner de solo lectura.

// Request es una petición al módulo.
type Request struct {
	Method string
	Route  string
	Query  url.Values
	Actor  Actor
}

// Plan es el resultado de la fase de autenticación: un resultado inmediato o una consulta diferida.
type Plan struct {
	immediate any
	deferred  func(ctx context.Context, runner *Runner) (any, error)
}

// Deferred indica si el plan necesita el ejecutor de reportes.
func (plan *Plan) Deferred() bool {
	return plan.deferred != nil
}

// Execute devuelve el resultado inmediato o ejecuta la consulta diferida.
func (plan *Plan) Execute(ctx context.Context, runner *Runner) (any, error) {
	if plan.deferred == nil {
		return plan.immediate, nil
	}
	return plan.deferred(ctx, runner)
}

// Handle atiende reports/* en la transacción de autenticación (ver el comentario del archivo).
func Handle(ctx context.Context, tx pgx.Tx, request Request) (*Plan, error) {
	if request.Method != "GET" {
		return nil, notFound("La ruta de reportes no existe.")
	}
	if !request.Actor.Can(PermissionRead) {
		return nil, denied()
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(request.Route, "reports/"), "/"), "/")
	switch {
	case len(parts) == 1 && parts[0] == "catalog":
		if len(request.Query) > 0 {
			return nil, invalid("El catálogo de reportes no admite parámetros.")
		}
		catalog, err := buildCatalog(ctx, tx, request.Actor)
		return &Plan{immediate: catalog}, err
	case len(parts) == 2 && parts[0] == "filters":
		for key := range request.Query {
			if key != "search" {
				return nil, invalid("Las opciones de filtros solo admiten search.")
			}
		}
		options, err := filterOptions(ctx, tx, parts[1], request.Query.Get("search"))
		if err != nil {
			return nil, err
		}
		return &Plan{immediate: map[string]any{"source": parts[1], "options": options}}, nil
	case len(parts) == 2 && parts[0] == "run":
		definition, raw, err := Prepare(request.Actor, parts[1], request.Query)
		if err != nil {
			return nil, err
		}
		actor := request.Actor
		return &Plan{deferred: func(ctx context.Context, runner *Runner) (any, error) {
			var result *Result
			err := runner.Read(ctx, func(tx pgx.Tx) error {
				var readErr error
				result, readErr = Execute(ctx, tx, actor, definition, raw)
				return readErr
			})
			return result, err
		}}, nil
	case len(parts) == 2 && parts[0] == "export":
		// Exportación a PDF o Excel (fase C1, export.go): misma validación que run más el formato y
		// su permiso; el archivo se genera después, en el Runner.
		return prepareExport(request.Actor, parts[1], request.Query)
	case len(parts) == 1 && parts[0] == "dashboard":
		raw, err := parseDashboardParams(request.Query)
		if err != nil {
			return nil, err
		}
		actor := request.Actor
		return &Plan{deferred: func(ctx context.Context, runner *Runner) (any, error) {
			var result *Dashboard
			err := runner.Read(ctx, func(tx pgx.Tx) error {
				var readErr error
				result, readErr = buildDashboard(ctx, tx, actor, raw)
				return readErr
			})
			return result, err
		}}, nil
	}
	return nil, notFound("La ruta de reportes no existe.")
}

// CatalogReport es un reporte visible en el catálogo.
type CatalogReport struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	DateBasis   string   `json:"date_basis"`
	Period      string   `json:"period"`
	Comparison  bool     `json:"comparison"`
	Filters     []Filter `json:"filters"`
	Columns     []Column `json:"columns"`
	DefaultSort SortInfo `json:"default_sort"`
	Export      struct {
		PDF   bool `json:"pdf"`
		Excel bool `json:"excel"`
	} `json:"export"`
}

// CatalogGroup agrupa los reportes visibles de un grupo.
type CatalogGroup struct {
	ID      string          `json:"id"`
	Title   string          `json:"title"`
	Reports []CatalogReport `json:"reports"`
}

// Catalog es la respuesta de GET reports/catalog.
type Catalog struct {
	Groups    []CatalogGroup `json:"groups"`
	Periods   []Option       `json:"periods"`
	Dashboard bool           `json:"dashboard"`
	Timezone  string         `json:"timezone"`
	DayStart  string         `json:"day_start"`
}

var periodOptions = []Option{
	{Value: businessday.KindToday, Label: "Hoy"},
	{Value: businessday.KindYesterday, Label: "Ayer"},
	{Value: businessday.KindCurrentWeek, Label: "Semana actual"},
	{Value: businessday.KindCurrentMonth, Label: "Mes actual"},
	{Value: businessday.KindCurrentYear, Label: "Año actual"},
	{Value: businessday.KindDay, Label: "Día"},
	{Value: businessday.KindISOWeek, Label: "Semana del año (ISO 8601)"},
	{Value: businessday.KindMonth, Label: "Mes"},
	{Value: businessday.KindYear, Label: "Año"},
	{Value: businessday.KindRange, Label: "Periodo personalizado"},
}

func buildCatalog(ctx context.Context, tx pgx.Tx, actor Actor) (*Catalog, error) {
	config, err := businessday.Load(ctx, tx)
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{Groups: []CatalogGroup{}, Periods: periodOptions, Dashboard: true, Timezone: config.Timezone,
		DayStart: config.Cutoff()}
	byGroup := map[string][]CatalogReport{}
	for _, definition := range Definitions() {
		if !definition.Allowed(actor) {
			continue
		}
		entry := CatalogReport{
			ID: definition.ID, Title: definition.Title, Description: definition.Description, DateBasis: definition.DateBasis,
			Period: definition.Period, Comparison: definition.Comparison, Filters: definition.Filters,
			Columns: visibleColumns(actor, definition.Columns),
		}
		if entry.Filters == nil {
			entry.Filters = []Filter{}
		}
		entry.DefaultSort = SortInfo{Key: definition.DefaultSort.Key, Direction: "asc"}
		if definition.DefaultSort.Descending {
			entry.DefaultSort.Direction = "desc"
		}
		entry.Export.PDF = actor.Can(PermissionExportPDF)
		entry.Export.Excel = actor.Can(PermissionExportExcel)
		byGroup[definition.Group] = append(byGroup[definition.Group], entry)
	}
	for _, group := range Groups {
		if reports := byGroup[group.ID]; len(reports) > 0 {
			catalog.Groups = append(catalog.Groups, CatalogGroup{ID: group.ID, Title: group.Title, Reports: reports})
		}
	}
	return catalog, nil
}
