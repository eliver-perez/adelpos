package reports

import (
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// legacyReports son los reportes que incluyen el historial importado del sistema anterior (etapa 9H, fase C parte 2):
// los de ventas, productos, formas de pago, por día y por turno de la nota técnica 3, más las cuentas canceladas, las
// cancelaciones por motivo, los cambios de cantidad, los turnos de caja y la evolución de ventas.
var legacyReports = []string{"sales.summary", "sales.by_day", "sales.by_week", "sales.by_month", "sales.by_hour",
	"sales.by_category", "sales.by_product", "sales.by_mode", "sales.dine_in", "sales.takeaway", "sales.accounts",
	"sales.by_shift", "sales.evolution", "products.top", "products.bottom", "products.trends", "products.complements",
	"payments.by_method", "payments.by_day", "payments.by_shift", "payments.by_user", "payments.by_register", "payments.list",
	"sales.voided_accounts", "sales.cancellations_by_reason", "products.quantity_changes", "cash.shifts"}

// Solo esos reportes declaran «Datos» (choice, por omisión los dos, con sus tres opciones y la marca Legacy) y las
// columnas del historial; ninguno más tiene columnas ni filtros del historial.
func TestLegacyReportsDeclareDataSource(t *testing.T) {
	for _, definition := range Definitions() {
		declared := slices.Contains(legacyReports, definition.ID)
		hasFilter := false
		for _, filter := range definition.Filters {
			if filter.Legacy != (filter.Key == dataSourceKey) {
				t.Fatalf("%s: el filtro %s tiene la marca del historial equivocada", definition.ID, filter.Key)
			}
			if filter.Key != dataSourceKey {
				continue
			}
			hasFilter = true
			values := []string{}
			for _, option := range filter.Options {
				values = append(values, option.Value)
			}
			if filter.Type != FilterChoice || filter.Default != DataSourceAll || filter.Label != "Datos" ||
				!slices.Equal(values, []string{DataSourceAll, DataSourceNative, DataSourceLegacy}) {
				t.Fatalf("%s: «Datos» con otra forma: %+v", definition.ID, filter)
			}
		}
		if hasFilter != declared {
			t.Fatalf("%s: filtro «Datos» %v; se esperaba %v", definition.ID, hasFilter, declared)
		}
		hasColumn := false
		for _, column := range definition.Columns {
			if column.Legacy {
				hasColumn = true
			}
			if (column.Key == "data_source" || column.Key == "legacy_difference") && !column.Legacy {
				t.Fatalf("%s: la columna %s debe ser del historial", definition.ID, column.Key)
			}
		}
		if hasColumn != (declared && definition.ID != "sales.evolution") {
			t.Fatalf("%s: columnas del historial %v; se esperaba %v", definition.ID, hasColumn, declared)
		}
	}
}

// Sin lote vigente, o en un reporte sin «Datos», solo AD-EL POS sin importar el valor recibido.
func TestResolveLegacy(t *testing.T) {
	byDay, _ := Lookup("sales.by_day")
	variants, _ := Lookup("sales.by_variant")
	cases := []struct {
		definition *Definition
		available  bool
		value      string
		want       legacyScope
	}{
		{byDay, false, DataSourceLegacy, legacyScope{Native: true}},
		{byDay, false, "", legacyScope{Native: true}},
		{byDay, true, DataSourceAll, legacyScope{Available: true, Native: true, History: true}},
		{byDay, true, DataSourceNative, legacyScope{Available: true, Native: true}},
		{byDay, true, DataSourceLegacy, legacyScope{Available: true, History: true}},
		{variants, true, DataSourceLegacy, legacyScope{Available: true, Native: true}},
	}
	for _, item := range cases {
		params := &Params{Values: map[string]string{dataSourceKey: item.value}}
		if got := resolveLegacy(item.definition, params, item.available); got != item.want {
			t.Fatalf("%s con lote %v y %q: %+v; se esperaba %+v", item.definition.ID, item.available, item.value, got, item.want)
		}
	}
	scope := legacyScope{Native: true}
	query := &Query{legacy: scope}
	if query.ledgerFrom() != "app.sales_recognitions r" || query.ledgerExtras() != "" {
		t.Fatal("Sin lote la fuente y las medidas de los reportes de ventas son las de antes")
	}
	columns := visibleColumns(Actor{}, withLegacyColumns([]Column{textColumn("day", "Día")}, true), false)
	if len(columns) != 1 || columns[0].Key != "day" {
		t.Fatalf("Sin lote no se envían las columnas del historial: %+v", columns)
	}
}

// Los renglones de AD-EL POS y del historial con la misma clave se juntan: se suman sus medidas y se marcan con los dos.
func TestMergeRows(t *testing.T) {
	day := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	columns := []Column{dateColumn("day", "Día"), countColumn("payments", "Cobros"), moneyColumn("net", "Neto"), columnDataSource}
	rows := mergeRows([]Row{
		{"day": day, "payments": int64(2), "net": decimal.RequireFromString("10.50"), "data_source": NativeSourceLabel},
		{"day": day.AddDate(0, 0, 1), "payments": int64(1), "net": decimal.RequireFromString("1"), "data_source": NativeSourceLabel},
		{"day": day, "payments": int64(3), "net": decimal.RequireFromString("0.1234"), "data_source": LegacySourceLabel},
	}, "day", columns)
	if len(rows) != 2 || rows[0]["payments"] != int64(5) || !decimalOf(rows[0], "net").Equal(decimal.RequireFromString("10.6234")) ||
		rows[0]["data_source"] != BothSourcesLabel || rows[1]["data_source"] != NativeSourceLabel {
		t.Fatalf("Renglones juntados: %+v", rows)
	}
}

// LedgerFromForTest expone a la medición de volumen (paquete reports_test) la fuente del libro con o sin AD-EL POS y
// el historial, para imprimir sus planes.
func LedgerFromForTest(native, history bool) string {
	query := &Query{legacy: legacyScope{Available: true, Native: native, History: history}}
	return query.ledgerFrom()
}
