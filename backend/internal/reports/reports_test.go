package reports

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/migrations"
)

// Cada permiso de reportes registrado en Go existe en la 0017 con la misma descripción y se concede
// solo al administrador; la 0017 no registra permisos que Go no conozca.
func TestReportPermissionsAreMigratedAndGrantedToAdministrator(t *testing.T) {
	content, err := fs.ReadFile(migrations.Files, "0017_reports.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	grant := sql[strings.LastIndex(sql, "INSERT INTO app.role_permissions"):]
	if !strings.Contains(grant, "r.code = 'administrator'") || strings.Count(sql, "INSERT INTO app.role_permissions") != 1 {
		t.Fatal("La 0017 debe conceder sus permisos solo al administrador")
	}
	if found := strings.Count(sql, "('reports."); found != len(Permissions) || len(Permissions) != 10 {
		t.Fatalf("La 0017 registra %d permisos de reportes y Go %d", found, len(Permissions))
	}
	for _, permission := range Permissions {
		if !strings.Contains(sql, "('"+permission.Key+"', '"+permission.Description+"')") {
			t.Fatalf("La 0017 no registra %s con la misma descripción", permission.Key)
		}
		if !strings.Contains(grant, "'"+permission.Key+"'") {
			t.Fatalf("La 0017 no concede %s al administrador", permission.Key)
		}
	}
}

// El registro es cerrado y coherente: ids estables, permisos de grupo, fecha usada declarada,
// columnas tipadas con orden válido, sin columnas protegidas como orden por omisión y sin filtros
// reservados.
func TestReportRegistryIsClosedAndConsistent(t *testing.T) {
	groupPermissions := []string{PermissionSales, PermissionPayments, PermissionDiscounts, PermissionCash, PermissionInventory,
		PermissionPurchases, PermissionEmployees}
	definitions := Definitions()
	if len(definitions) < 40 {
		t.Fatalf("Se esperaban los reportes de ventas, productos, cocina, pagos y descuentos: %d", len(definitions))
	}
	for _, definition := range definitions {
		if len(definition.Permissions) == 0 || !slices.Contains(groupPermissions, definition.Permissions[0]) {
			t.Fatalf("%s no exige un permiso de grupo", definition.ID)
		}
		if definition.DateBasis == "" || definition.Title == "" || definition.Description == "" {
			t.Fatalf("%s no explica su fecha o su contenido", definition.ID)
		}
		for _, column := range definition.Columns {
			if column.Key == definition.DefaultSort.Key && len(column.Permissions) > 0 {
				t.Fatalf("%s ordena por omisión por una columna protegida", definition.ID)
			}
			if (column.Type == TypeMoneyMXN || column.Type == TypeMoneyUSD) && column.Total == TotalNone && column.Key != "average_ticket" {
				// Los importes sin total deben declararlo con intención (ninguno en la fase B1).
				t.Fatalf("%s.%s es un importe sin total", definition.ID, column.Key)
			}
		}
		if _, found := Lookup(definition.ID); !found {
			t.Fatalf("%s no se encuentra en el registro", definition.ID)
		}
	}
	// El efectivo físico exige cash.expected.read (paridad con Caja).
	currency, _ := Lookup("payments.currency")
	if !slices.Contains(currency.Permissions, PermissionCashExpected) {
		t.Fatal("El efectivo por moneda debe exigir cash.expected.read")
	}
	list, _ := Lookup("payments.list")
	for _, column := range list.Columns {
		if (column.Key == "received" || column.Key == "change" || column.Key == "received_usd") && !slices.Contains(column.Permissions, PermissionCashExpected) {
			t.Fatalf("La columna %s debe exigir cash.expected.read", column.Key)
		}
	}
}

func TestPermissionsAndParameters(t *testing.T) {
	sales := Actor{Permissions: []string{PermissionRead, PermissionSales}}
	if _, _, err := Prepare(sales, "payments.by_method", nil); !isCode(err, "PERMISSION_DENIED") {
		t.Fatalf("Un reporte de cobros sin su permiso debe denegarse: %v", err)
	}
	if _, _, err := Prepare(Actor{Permissions: []string{PermissionSales}}, "sales.by_day", nil); !isCode(err, "PERMISSION_DENIED") {
		t.Fatal("Sin reports.read ningún reporte se consulta")
	}
	if _, _, err := Prepare(sales, "no.existe", nil); !isCode(err, "NOT_FOUND") {
		t.Fatal("Un reporte fuera del registro no existe")
	}
	cases := []url.Values{
		{"sql": {"SELECT 1"}},
		{"page": {"0"}},
		{"page_size": {"201"}},
		{"sort": {"desconocida"}},
		{"direction": {"arriba"}},
		{"compare": {"siempre"}},
		{"mode": {"delivery"}},
		{"category_id": {"no-es-uuid"}},
		{"origin": {strings.Repeat("x", 81)}},
		{"period": {"day", "week"}},
	}
	for _, values := range cases {
		if _, _, err := Prepare(sales, "sales.by_day", values); !isCode(err, "VALIDATION_ERROR") {
			t.Fatalf("Se aceptaron parámetros inválidos %v: %v", values, err)
		}
	}
	if _, _, err := Prepare(sales, "sales.in_progress", url.Values{"period": {"today"}}); !isCode(err, "VALIDATION_ERROR") {
		t.Fatal("«En curso» no admite periodo")
	}
	if _, _, err := Prepare(sales, "sales.by_day", url.Values{"compare": {"previous"}}); !isCode(err, "VALIDATION_ERROR") {
		t.Fatal("Un reporte sin comparación no admite compare=previous")
	}
	_, raw, err := Prepare(sales, "sales.evolution", url.Values{"compare": {"previous"}, "period": {"iso_week"}, "week": {"2026-W53"},
		"sort": {"net"}, "direction": {"desc"}})
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := Lookup("sales.evolution")
	config := businessday.Config{Timezone: "America/Monterrey", Location: mustLocation(t), Hour: 5}
	params, err := raw.resolve(definition, time.Date(2026, time.November, 10, 18, 0, 0, 0, time.UTC), config)
	if err != nil || params.Period.FirstDay != "2026-12-28" || params.Previous == nil || params.Previous.ISOWeek != 52 ||
		params.Sort.Key != "net" || !params.Sort.Descending || params.Value("group_by") != "day" {
		t.Fatalf("Parámetros resueltos: %+v %v", params, err)
	}
	payer := Actor{Permissions: []string{PermissionRead, PermissionPayments}}
	if _, _, err := Prepare(payer, "payments.list", url.Values{"sort": {"received"}}); !isCode(err, "VALIDATION_ERROR") {
		t.Fatal("No se puede ordenar por una columna protegida sin su permiso")
	}
	if columns := visibleColumns(payer, definitionOf(t, "payments.list").Columns); slices.ContainsFunc(columns, func(column Column) bool {
		return column.Key == "received"
	}) {
		t.Fatal("Sin cash.expected.read no se envía el efectivo recibido")
	}
}

func definitionOf(t *testing.T, id string) *Definition {
	t.Helper()
	definition, found := Lookup(id)
	if !found {
		t.Fatal(id)
	}
	return definition
}

func mustLocation(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("America/Monterrey")
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func isCode(err error, code string) bool {
	var problem *Error
	return errors.As(err, &problem) && problem.Code == code
}

// El motor totaliza todo el conjunto filtrado en Go, ordena con su tipo y pagina sin cambiar los
// totales; el JSON lleva el dinero como cadena con dos decimales y quita las columnas protegidas.
func TestTotalsSortingPagingAndFormatting(t *testing.T) {
	definition := &Definition{
		ID: "test.engine", Columns: []Column{
			textColumn("name", "Nombre"),
			countColumn("quantity", "Cantidad"),
			moneyColumn("net", "Venta"),
			requiring(moneyColumn("cost", "Costo"), PermissionCostsRead),
			percentColumn("share", "%"),
			dateColumn("day", "Día"),
		},
		DefaultSort: Sort{Key: "net", Descending: true},
		Totals:      shareTotals,
	}
	for index := range definition.Columns {
		if definition.Columns[index].Total == "" {
			definition.Columns[index].Total = TotalNone
		}
	}
	day := time.Date(2026, time.November, 10, 0, 0, 0, 0, time.UTC)
	rows := []Row{
		{"name": "b", "quantity": int64(1), "net": decimal.RequireFromString("10.10"), "cost": decimal.RequireFromString("4"), "day": day},
		{"name": "a", "quantity": int64(3), "net": decimal.RequireFromString("30.30"), "cost": decimal.RequireFromString("9"), "day": day},
		{"name": "c", "quantity": int64(2), "net": decimal.RequireFromString("20.20"), "cost": decimal.RequireFromString("5"), "day": day},
	}
	actor := Actor{Permissions: []string{PermissionRead}}
	for page, expected := range []string{"a", "c", "b"} {
		copied := make([]Row, len(rows))
		for index, row := range rows {
			copied[index] = Row{}
			for key, value := range row {
				copied[index][key] = value
			}
		}
		query := &Query{Params: &Params{Page: page + 1, PageSize: 1, Sort: definition.DefaultSort}, Actor: actor,
			Config: businessday.DefaultConfig()}
		result, err := assemble(nil, nil, query, definition, &Dataset{Rows: copied})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Rows) != 1 || result.Rows[0]["name"] != expected || result.Page.TotalRows != 3 || result.Page.TotalPages != 3 {
			t.Fatalf("Página %d: %+v", page+1, result.Rows)
		}
		if result.Totals["net"].(decimal.Decimal).StringFixed(2) != "60.60" || result.Totals["quantity"].(int64) != 6 {
			t.Fatalf("La paginación alteró los totales: %+v", result.Totals)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		text := string(encoded)
		if strings.Contains(text, "cost") || !strings.Contains(text, `"net":"60.60"`) || !strings.Contains(text, `"share":"100.00"`) ||
			!strings.Contains(text, `"day":"2026-11-10"`) || !strings.Contains(text, `"quantity":6`) {
			t.Fatalf("JSON del resultado: %s", text)
		}
	}
}

func TestFormatValue(t *testing.T) {
	cases := []struct {
		kind     string
		value    any
		expected any
	}{
		{TypeMoneyMXN, decimal.RequireFromString("1234.5"), "1234.50"},
		{TypeMoneyUSD, decimal.RequireFromString("0.65"), "0.65"},
		{TypePercent, decimal.RequireFromString("12.345"), "12.35"},
		{TypeQuantity, decimal.RequireFromString("1.250000"), "1.25"},
		{TypeInteger, int64(7), int64(7)},
		{TypeDate, time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), "2026-12-31"},
		{TypeDateTime, time.Date(2026, 11, 11, 7, 30, 0, 0, time.UTC), "2026-11-11T07:30:00Z"},
		{TypeCode, "0012", "0012"},
		{TypeText, nil, nil},
	}
	for _, testCase := range cases {
		if got := FormatValue(testCase.kind, testCase.value); got != testCase.expected {
			t.Fatalf("%s: %v ≠ %v", testCase.kind, got, testCase.expected)
		}
	}
}
