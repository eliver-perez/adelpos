package reports

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/businessday"
)

// Query es el contexto de la consulta de un reporte: la transacción de solo lectura, la
// configuración del día comercial, los parámetros resueltos, quien consulta y la hora de la
// fotografía (now() de la transacción).
type Query struct {
	Tx         pgx.Tx
	Config     businessday.Config
	Params     *Params
	Actor      Actor
	Now        time.Time
	definition *Definition
}

// Args devuelve los argumentos con nombre comunes: @from y @until (instantes del periodo), @tz (zona
// horaria) y @cutoff (hora de inicio como intervalo), más un argumento por filtro indicado.
func (query *Query) Args() pgx.NamedArgs {
	args := pgx.NamedArgs{"tz": query.Config.Timezone, "cutoff": query.Config.Interval()}
	if query.Params.Period != nil {
		args["from"] = query.Params.Period.From
		args["until"] = query.Params.Period.Until
	}
	for key, value := range query.Params.Values {
		args[key] = value
	}
	return args
}

// PeriodArgs son los argumentos para otro periodo (por ejemplo, el anterior equivalente).
func (query *Query) PeriodArgs(period *businessday.Period) pgx.NamedArgs {
	args := query.Args()
	args["from"] = period.From
	args["until"] = period.Until
	return args
}

// Has indica si se indicó el filtro.
func (query *Query) Has(key string) bool {
	return query.Params.Values[key] != ""
}

// businessDate es la expresión SQL del día comercial de un instante, coherente con los límites que
// calcula Go (difiere solo en la hora repetida de un atraso de horario junto a la hora de inicio).
func businessDate(column string) string {
	return "((" + column + " AT TIME ZONE @tz) - @cutoff::interval)::date"
}

// localHour es la hora local (0 a 23) de un instante en la zona del establecimiento.
func localHour(column string) string {
	return "extract(hour FROM " + column + " AT TIME ZONE @tz)::integer"
}

// inPeriod es el filtro [desde, hasta) de una columna de instante.
func inPeriod(column string) string {
	return column + " >= @from AND " + column + " < @until"
}

// conditions acumula condiciones SQL opcionales unidas con AND.
type conditions []string

func (list *conditions) add(condition string) {
	*list = append(*list, condition)
}

// when agrega la condición si se indicó el filtro.
func (list *conditions) when(query *Query, key, condition string) {
	if query.Has(key) {
		list.add(condition)
	}
}

func (list conditions) sql() string {
	if len(list) == 0 {
		return ""
	}
	return " AND " + strings.Join(list, " AND ")
}

// collect ejecuta una consulta cuyas columnas se llaman como las claves de las columnas del reporte
// y convierte cada valor al tipo de Go de su columna. Las columnas numéricas se pueden leer como
// numeric o como texto decimal.
func (query *Query) collect(ctx context.Context, sql string, args pgx.NamedArgs) ([]Row, error) {
	types := map[string]string{}
	for _, column := range query.definition.Columns {
		types[column.Key] = column.Type
	}
	return collectRows(ctx, query.Tx, sql, args, types)
}

func collectRows(ctx context.Context, tx pgx.Tx, sql string, args pgx.NamedArgs, types map[string]string) ([]Row, error) {
	rows, err := tx.Query(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fields := rows.FieldDescriptions()
	result := []Row{}
	for rows.Next() {
		if len(result) >= MaxRows {
			return nil, tooLarge()
		}
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(Row, len(values))
		for index, field := range fields {
			converted, err := convertValue(types[field.Name], values[index])
			if err != nil {
				return nil, fmt.Errorf("columna %s: %w", field.Name, err)
			}
			row[field.Name] = converted
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// convertValue lleva un valor de pgx al tipo de Go de la columna.
func convertValue(columnType string, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch columnType {
	case TypeMoneyMXN, TypeMoneyUSD, TypePercent, TypeQuantity:
		return toDecimal(value)
	case TypeInteger:
		switch typed := value.(type) {
		case int64:
			return typed, nil
		case int32:
			return int64(typed), nil
		case int16:
			return int64(typed), nil
		}
		number, err := toDecimal(value)
		if err != nil {
			return nil, err
		}
		return number.IntPart(), nil
	case TypeDate, TypeDateTime:
		if typed, ok := value.(time.Time); ok {
			return typed, nil
		}
		return nil, fmt.Errorf("se esperaba una fecha")
	}
	// Columnas auxiliares (no declaradas) y de texto: se conservan los tipos nativos útiles.
	switch typed := value.(type) {
	case [16]byte:
		return pgtype.UUID{Bytes: typed, Valid: true}.String(), nil
	case string, bool, int64, time.Time:
		return typed, nil
	case int32:
		return int64(typed), nil
	case int16:
		return int64(typed), nil
	case pgtype.Numeric:
		return toDecimal(typed)
	}
	return fmt.Sprint(value), nil
}

func toDecimal(value any) (decimal.Decimal, error) {
	switch typed := value.(type) {
	case string:
		return decimal.NewFromString(typed)
	case pgtype.Numeric:
		text, err := typed.Value()
		if err != nil {
			return decimal.Zero, err
		}
		if text == nil {
			return decimal.Zero, nil
		}
		return decimal.NewFromString(text.(string))
	case int64:
		return decimal.NewFromInt(typed), nil
	case int32:
		return decimal.NewFromInt(int64(typed)), nil
	case float64:
		return decimal.NewFromFloat(typed), nil
	}
	return decimal.Zero, fmt.Errorf("valor numérico no admitido %T", value)
}

// decimalOf lee un valor decimal de un renglón (cero si falta).
func decimalOf(row Row, key string) decimal.Decimal {
	if value, ok := row[key].(decimal.Decimal); ok {
		return value
	}
	if value, ok := row[key].(int64); ok {
		return decimal.NewFromInt(value)
	}
	return decimal.Zero
}

// percentOf es part / whole × 100 con dos decimales (redondeo comercial); nil si whole es cero.
func percentOf(part, whole decimal.Decimal) any {
	if whole.IsZero() {
		return nil
	}
	return part.Mul(decimal.NewFromInt(100)).DivRound(whole, 2)
}
