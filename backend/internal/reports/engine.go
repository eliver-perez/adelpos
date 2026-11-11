package reports

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/businessday"
)

// Run ejecuta un reporte completo en el ejecutor de reportes: valida permisos y parámetros, consulta
// en la transacción de solo lectura y arma el resultado (columnas visibles, orden, totales de todo
// el conjunto filtrado y la página pedida). Las exportaciones de la fase C llaman a Run con los
// mismos parámetros y permisos que la pantalla y usan Result.AllRows.
func Run(ctx context.Context, runner *Runner, actor Actor, id string, values url.Values) (*Result, error) {
	definition, raw, err := Prepare(actor, id, values)
	if err != nil {
		return nil, err
	}
	var result *Result
	err = runner.Read(ctx, func(tx pgx.Tx) error {
		var readErr error
		result, readErr = Execute(ctx, tx, actor, definition, raw)
		return readErr
	})
	return result, err
}

// Prepare valida, sin consultar la base, que el reporte existe, que el actor puede verlo y que los
// parámetros tienen forma válida. Se llama dentro de la transacción de autenticación para que una
// denegación se audite como las demás.
func Prepare(actor Actor, id string, values url.Values) (*Definition, *rawParams, error) {
	if !actor.Can(PermissionRead) {
		return nil, nil, denied()
	}
	definition, found := Lookup(id)
	if !found {
		return nil, nil, notFound("El reporte no existe.")
	}
	if !definition.Allowed(actor) {
		return nil, nil, denied()
	}
	raw, err := parseParams(values, definition)
	if err != nil {
		return nil, nil, err
	}
	// Ordenar por una columna protegida revelaría su contenido aunque no se envíe.
	if raw.Sort != nil {
		for _, column := range definition.Columns {
			if column.Key == raw.Sort.Key && !actor.CanAll(column.Permissions) {
				return nil, nil, invalid("No se puede ordenar por " + raw.Sort.Key + ".")
			}
		}
	}
	return definition, raw, nil
}

// Execute consulta y arma el resultado dentro de una transacción ya abierta (de solo lectura).
func Execute(ctx context.Context, tx pgx.Tx, actor Actor, definition *Definition, raw *rawParams) (*Result, error) {
	config, now, err := snapshot(ctx, tx)
	if err != nil {
		return nil, err
	}
	params, err := raw.resolve(definition, now, config)
	if err != nil {
		return nil, err
	}
	query := &Query{Tx: tx, Config: config, Params: params, Actor: actor, Now: now, definition: definition}
	dataset, err := definition.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	return assemble(ctx, tx, query, definition, dataset)
}

// snapshot lee la configuración del día comercial y la hora de la fotografía en la transacción.
func snapshot(ctx context.Context, tx pgx.Tx) (businessday.Config, time.Time, error) {
	config, err := businessday.Load(ctx, tx)
	if err != nil {
		return config, time.Time{}, err
	}
	var now time.Time
	err = tx.QueryRow(ctx, "SELECT now()").Scan(&now)
	return config, now.UTC(), err
}

// visibleColumns quita las columnas que exigen permisos que el actor no tiene.
func visibleColumns(actor Actor, columns []Column) []Column {
	visible := make([]Column, 0, len(columns))
	for _, column := range columns {
		if actor.CanAll(column.Permissions) {
			visible = append(visible, column)
		}
	}
	return visible
}

// strip deja en el renglón solo las claves de las columnas visibles.
func strip(row Row, columns []Column) Row {
	result := make(Row, len(columns))
	for _, column := range columns {
		if value, found := row[column.Key]; found {
			result[column.Key] = value
		}
	}
	return result
}

func assemble(ctx context.Context, tx pgx.Tx, query *Query, definition *Definition, dataset *Dataset) (*Result, error) {
	params := query.Params
	allColumns := definition.Columns
	columns := visibleColumns(query.Actor, allColumns)
	totals := sumRows(allColumns, dataset.Rows)
	if definition.Totals != nil {
		definition.Totals(dataset.Rows, totals)
	}
	sortRows(dataset.Rows, allColumns, params.Sort)
	all := make([]Row, 0, len(dataset.Rows))
	for _, row := range dataset.Rows {
		all = append(all, strip(row, columns))
	}
	page := PageInfo{Page: params.Page, PageSize: params.PageSize, TotalRows: len(all)}
	page.TotalPages = (len(all) + params.PageSize - 1) / params.PageSize
	start := (params.Page - 1) * params.PageSize
	end := start + params.PageSize
	if start > len(all) {
		start = len(all)
	}
	if end > len(all) {
		end = len(all)
	}
	summary := []Figure{}
	for _, figure := range dataset.Summary {
		if query.Actor.CanAll(figure.Permissions) {
			summary = append(summary, figure)
		}
	}
	filters, err := describeFilters(ctx, tx, query.Actor, definition, params)
	if err != nil {
		return nil, err
	}
	direction := "asc"
	if params.Sort.Descending {
		direction = "desc"
	}
	result := &Result{
		Report: ReportInfo{ID: definition.ID, Group: definition.Group, Title: definition.Title, Description: definition.Description},
		Meta: Meta{
			Title: definition.Title, Period: params.Period, Comparison: params.Previous, Filters: filters,
			GeneratedAt: query.Now, Timezone: query.Config.Timezone, DayStart: query.Config.Cutoff(),
			DateBasis: definition.DateBasis, Warnings: dataset.Warnings, Notes: dataset.Notes,
		},
		Columns: columns,
		Rows:    all[start:end],
		AllRows: all,
		Totals:  strip(totals, columns),
		Summary: summary,
		Page:    page,
		Sort:    SortInfo{Key: params.Sort.Key, Direction: direction},
	}
	return result, nil
}

// sumRows suma en Go, sobre todo el conjunto filtrado, las columnas con Total «sum».
func sumRows(columns []Column, rows []Row) Row {
	totals := Row{}
	for _, column := range columns {
		if column.Total != TotalSum {
			continue
		}
		if column.Type == TypeInteger {
			var total int64
			for _, row := range rows {
				if value, ok := row[column.Key].(int64); ok {
					total += value
				}
			}
			totals[column.Key] = total
			continue
		}
		total := decimal.Zero
		for _, row := range rows {
			total = total.Add(decimalOf(row, column.Key))
		}
		totals[column.Key] = total
	}
	return totals
}

// compareValues compara dos valores del mismo tipo; los nulos van al final en ambos sentidos.
func compareValues(left, right any) int {
	switch typed := left.(type) {
	case decimal.Decimal:
		return typed.Cmp(right.(decimal.Decimal))
	case int64:
		other := right.(int64)
		if typed < other {
			return -1
		}
		if typed > other {
			return 1
		}
		return 0
	case time.Time:
		return typed.Compare(right.(time.Time))
	case string:
		return strings.Compare(strings.ToLower(typed), strings.ToLower(right.(string)))
	}
	return 0
}

// sortRows ordena de forma estable por la columna pedida (respetando el orden de la consulta como
// desempate).
func sortRows(rows []Row, columns []Column, order Sort) {
	if order.Key == "" {
		return
	}
	sort.SliceStable(rows, func(left, right int) bool {
		first, second := rows[left][order.Key], rows[right][order.Key]
		if first == nil || second == nil {
			return first != nil && second == nil
		}
		comparison := compareValues(first, second)
		if order.Descending {
			return comparison > 0
		}
		return comparison < 0
	})
}
