//go:build integration

package reports_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/legacyimport"
	"local/ad-el-pos/server/internal/legacypkg"
	"local/ad-el-pos/server/internal/legacysynth"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
	"local/ad-el-pos/server/internal/reports"
)

// Etapa 9H, fase C parte 2: el historial importado del sistema anterior en los reportes. El historial se carga con el
// importador real (legacyimport: lectura del paquete, correspondencia e importación en su transacción) a partir de
// un paquete sintético de legacysynth con ventas agregadas en fechas límite; después se crea operación propia de
// AD-EL POS por la API (la importación exige una instalación sin operación). Cada prueba usa su propia base desde
// cero y la deja vacía al terminar, como las de exportaciones.

const legacyReportsPassphrase = "frase de paso sintética de reportes 9H"

// legacyReportIDs son los reportes que incluyen el historial (filtro «Datos»).
var legacyReportIDs = []string{"sales.summary", "sales.by_day", "sales.by_week", "sales.by_month", "sales.by_hour",
	"sales.by_category", "sales.by_product", "sales.by_mode", "sales.dine_in", "sales.takeaway", "sales.accounts",
	"sales.by_shift", "sales.evolution", "products.top", "products.bottom", "products.trends", "products.complements",
	"payments.by_method", "payments.by_day", "payments.by_shift", "payments.by_user", "payments.by_register", "payments.list",
	"sales.voided_accounts", "sales.cancellations_by_reason", "products.quantity_changes", "cash.shifts"}

// legacyResult es la forma JSON de un reporte con lo que comparan estas pruebas.
type legacyResult struct {
	Meta struct {
		Period *struct {
			From  time.Time `json:"from"`
			Until time.Time `json:"until"`
		} `json:"period"`
		Filters []struct {
			Key     string `json:"key"`
			Display string `json:"display"`
		} `json:"filters"`
		Notes    []string `json:"notes"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	} `json:"meta"`
	Columns []struct {
		Key   string `json:"key"`
		Label string `json:"label"`
		Type  string `json:"type"`
		Total string `json:"total"`
	} `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	Totals  map[string]any   `json:"totals"`
	Summary []struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	} `json:"summary"`
	Page struct {
		TotalRows  int `json:"total_rows"`
		TotalPages int `json:"total_pages"`
	} `json:"page"`
}

// legacyRun consulta un reporte completo (todas sus páginas).
func (fixture *reportsFixture) legacyRun(t *testing.T, member terminal, id, query string) legacyResult {
	t.Helper()
	var result legacyResult
	for page := 1; ; page++ {
		current := decode[legacyResult](t, fixture.get(t, member, fmt.Sprintf("reports/run/%s?page_size=200&page=%d&%s", id, page, query), 200))
		if page == 1 {
			result = current
		} else {
			result.Rows = append(result.Rows, current.Rows...)
		}
		if page >= current.Page.TotalPages {
			return result
		}
	}
}

func (result legacyResult) figure(key string) any {
	for _, figure := range result.Summary {
		if figure.Key == key {
			return figure.Value
		}
	}
	return nil
}

func (result legacyResult) hasColumn(key string) bool {
	return slices.ContainsFunc(result.Columns, func(column struct {
		Key   string `json:"key"`
		Label string `json:"label"`
		Type  string `json:"type"`
		Total string `json:"total"`
	}) bool {
		return column.Key == key
	})
}

// number lee un importe, una cantidad o un entero del JSON.
func number(t *testing.T, value any) decimal.Decimal {
	t.Helper()
	switch typed := value.(type) {
	case nil:
		return decimal.Zero
	case string:
		parsed, err := decimal.NewFromString(typed)
		if err != nil {
			t.Fatalf("No es un número: %q", typed)
		}
		return parsed
	case float64:
		return decimal.NewFromFloat(typed)
	}
	t.Fatalf("Valor numérico inesperado %T", value)
	return decimal.Zero
}

// importLegacy escribe un paquete sintético con las tablas y lo importa como el asistente (lectura completa,
// correspondencia e importación con el importador real). La validación debe cuadrar.
func (fixture *reportsFixture) importLegacy(t *testing.T, tables []*legacysynth.Table) {
	t.Helper()
	ctx := fixture.ctx
	path := filepath.Join(t.TempDir(), "historial.adelmig")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := legacypkg.NewWriter(legacypkg.WriterOptions{SpoolDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := legacysynth.WritePackage(writer, tables)
	if err != nil {
		t.Fatal(err)
	}
	written, err := writer.Finish(file, legacyReportsPassphrase, legacypkg.PackageInfo{CreatedAt: time.Now(),
		Creator: legacypkg.Creator{Product: "AD-EL POS", Version: "0.11.0", Tool: "reportes-9h"},
		Source:  legacypkg.Source{Kind: legacypkg.SourceSynthetic}, Summary: summary})
	writer.Close()
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	environment, err := legacyimport.ReadEnvironment(ctx, fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	source, closer, err := legacyimport.OpenPackage(path, legacyReportsPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	model, err := legacyimport.Load(ctx, source, environment.Config.Location, nil)
	closer.Close()
	if err != nil {
		t.Fatal(err)
	}
	plan, preview := legacyimport.Analyze(model, environment, written.Bytes, "")
	if !preview.Clean {
		t.Fatalf("La instalación de la prueba debe estar limpia: %+v", preview.Blockers)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	var admin string
	if err := fixture.pool.QueryRow(ctx, "SELECT id::text FROM app.users WHERE username='admin'").Scan(&admin); err != nil {
		t.Fatal(err)
	}
	report, err := legacyimport.Import(ctx, fixture.pool, plan, environment.Config, legacyimport.ImportRequest{UserID: admin,
		Username: "admin", DeviceID: fixture.admin.deviceID, PackageSHA256: hex.EncodeToString(digest[:]), PackageBytes: written.Bytes},
		nil)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Validation.OK {
		t.Fatalf("La validación del lote sintético no cuadra: %+v", report.Validation)
	}
}

// Ventas agregadas al conjunto sintético en fechas límite, con importes conocidos (hora local del anterior).
type legacyLine struct {
	product  int64
	quantity string
	total    string
}

type legacyPayment struct {
	kind   int64
	amount string
	at     string
	shift  any
}

type legacySale struct {
	id        int64
	table     int64
	opened    string
	total     string
	extra     string
	shift     any
	lines     []legacyLine
	payments  []legacyPayment
	discount  string
	cancelled string
}

const legacyLayout = "2006-01-02T15:04:05.000"

func addLegacySales(tables []*legacysynth.Table, sales []legacySale) {
	byName := map[string]*legacysynth.Table{}
	for _, table := range tables {
		byName[table.Name] = table
	}
	line, payment := int64(90000), int64(90000)
	for _, sale := range sales {
		paid := decimal.Zero
		for _, entry := range sale.payments {
			payment++
			byName["pagos"].Rows = append(byName["pagos"].Rows, []any{payment, sale.id, "", entry.kind, entry.amount, int64(1),
				entry.at, entry.shift})
			paid = paid.Add(decimal.RequireFromString(entry.amount))
		}
		for _, entry := range sale.lines {
			line++
			byName["ventas_productos"].Rows = append(byName["ventas_productos"].Rows, []any{line, sale.id, entry.product,
				entry.quantity, entry.quantity, entry.total, "0.0000", entry.total, nil})
		}
		var concept any
		extra := "0.0000"
		if sale.extra != "" {
			concept, extra = "Descorche", sale.extra
		}
		if sale.discount != "" {
			byName["descuentos"].Rows = append(byName["descuentos"].Rows, []any{int64(1), sale.id, "Cortesía de la casa", sale.discount})
		}
		if sale.cancelled != "" {
			byName["cancelaciones"].Rows = append(byName["cancelaciones"].Rows, []any{sale.id, sale.id, int64(1), sale.cancelled})
		}
		byName["ventas"].Rows = append(byName["ventas"].Rows, []any{sale.id, int64(1), sale.table, nil, fmt.Sprint(sale.id),
			sale.opened, int64(1), int64(0), sale.total, "0.0000", sale.total, paid.StringFixed(4), int64(1), concept, extra,
			int64(0), sale.shift, int64(0)})
	}
}

// legacyBoundarySales son las ventas con importes y fechas conocidos: el inicio del día a las 05:00 (04:59 es el día
// anterior), los dos cambios de horario de 2021 en Monterrey, importes de cuatro decimales, una venta cuyo total no
// cuadra con sus renglones (con descuento y cargo), una cancelada con un pago, una sin corte y dos recientes para
// mezclar con AD-EL POS.
func legacyBoundarySales(now time.Time) []legacySale {
	local := func(moment time.Time) string {
		return moment.Format(legacyLayout)
	}
	return []legacySale{
		{id: 9001, table: 2, opened: "2025-03-18T04:10:00.000", total: "20.2468", shift: int64(5),
			lines:    []legacyLine{{6, "1.000", "10.1234"}, {7, "1.000", "10.1234"}},
			payments: []legacyPayment{{1, "20.2468", "2025-03-18T04:59:00.000", int64(5)}}},
		{id: 9002, table: 3, opened: "2025-03-18T05:00:00.000", total: "110.0000", extra: "3.0000", discount: "5.0000",
			shift: int64(5), lines: []legacyLine{{10, "1.000", "100.0000"}},
			payments: []legacyPayment{{2, "108.0000", "2025-03-18T05:00:00.000", int64(5)}}},
		{id: 9003, table: 1, opened: "2025-03-18T13:00:00.000", total: "30.0000", shift: int64(5), cancelled: "2025-03-18T13:30:00.000",
			lines:    []legacyLine{{6, "3.000", "30.0000"}},
			payments: []legacyPayment{{1, "30.0000", "2025-03-18T13:20:00.000", int64(5)}}},
		{id: 9007, table: 1, opened: "2025-03-19T10:00:00.000", total: "70.0000", lines: []legacyLine{{11, "1.000", "70.0000"}},
			payments: []legacyPayment{{1, "70.0000", "2025-03-19T10:30:00.000", nil}}},
		{id: 9004, table: 4, opened: "2021-04-04T04:00:00.000", total: "40.0000", shift: int64(1),
			lines:    []legacyLine{{8, "1.000", "40.0000"}},
			payments: []legacyPayment{{1, "40.0000", "2021-04-04T04:30:00.000", int64(1)}}},
		{id: 9005, table: 4, opened: "2021-10-31T04:00:00.000", total: "50.0000", shift: int64(1),
			lines:    []legacyLine{{9, "1.000", "50.0000"}},
			payments: []legacyPayment{{1, "50.0000", "2021-10-31T04:59:00.000", int64(1)}}},
		{id: 9006, table: 4, opened: "2021-10-31T05:00:00.000", total: "60.0000", shift: int64(1),
			lines:    []legacyLine{{9, "1.000", "60.0000"}},
			payments: []legacyPayment{{1, "60.0000", "2021-10-31T05:00:00.000", int64(1)}}},
		{id: 9008, table: 1, opened: local(now.AddDate(0, 0, -1).Add(-time.Hour)), total: "15.0000", shift: int64(6),
			lines:    []legacyLine{{6, "1.000", "15.0000"}},
			payments: []legacyPayment{{1, "15.0000", local(now.AddDate(0, 0, -1)), int64(6)}}},
		{id: 9009, table: 1, opened: local(now.Add(-3 * time.Minute)), total: "25.0000", shift: int64(6),
			lines:    []legacyLine{{7, "1.000", "25.0000"}},
			payments: []legacyPayment{{1, "25.0000", local(now.Add(-2 * time.Minute)), int64(6)}}},
	}
}

// legacyUndatedShiftSale es la venta del corte 9100, que no tiene fecha_inicio ni fecha_fin (columna con nulos en el
// respaldo; revisión de la Fase C, M-8): el corte toma las fechas de su venta y su pago del 12/07/2023.
var legacyUndatedShiftSale = legacySale{id: 9010, table: 2, opened: "2023-07-12T13:00:00.000", total: "33.0000",
	shift: int64(9100), lines: []legacyLine{{6, "1.000", "33.0000"}},
	payments: []legacyPayment{{1, "33.0000", "2023-07-12T13:45:00.000", int64(9100)}}}

// addLegacyUndatedShift agrega el corte 9100 cerrado, sin fecha de inicio ni de fin.
func addLegacyUndatedShift(tables []*legacysynth.Table) {
	for _, table := range tables {
		if table.Name == "cortes" {
			table.Rows = append(table.Rows, []any{int64(9100), int64(1), "100.0000", "133.0000", "33.0000", int64(1), "C",
				int64(1), nil, nil, "0.0000", "0.0000"})
		}
	}
}

// freshFixture prepara una base nueva para la prueba y la deja vacía al terminar.
func freshFixture(t *testing.T) *reportsFixture {
	t.Helper()
	ctx, pool := openPool(t)
	if err := dropReportsSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := dropReportsSchema(ctx, pool); err != nil {
			t.Error(err)
		}
	})
	return newReportsFixture(t)
}

// normalizedReport es el JSON de un reporte sin la hora de la fotografía.
func (fixture *reportsFixture) normalizedReport(t *testing.T, id, query string) string {
	t.Helper()
	body := fixture.get(t, fixture.admin, "reports/run/"+id+"?page_size=200&"+query, 200)
	value := map[string]any{}
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	delete(value["meta"].(map[string]any), "generated_at")
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// Sin lote importado los reportes quedan exactamente como antes: el catálogo no ofrece «Datos» ni sus columnas en
// ningún reporte, y el parámetro, si llega, se ignora (mismo JSON con cualquiera de sus valores y sin él).
func TestStage9HReportsWithoutLegacyBatch(t *testing.T) {
	fixture := freshFixture(t)
	fixture.seedCatalog(t)
	shift := decode[payments.Shift](t, fixture.mutate(t, "POST", "payments/shifts", map[string]any{"opening_float": "500"}, 200))
	fixture.shiftID = shift.ID
	first := fixture.account(t, "Mesa uno", "dine_in", &fixture.table, line(fixture.burger, 2), line(fixture.soda, 1))
	fixture.pay(t, first.Account.ID, map[string]any{"method_id": fixture.methods["cash"], "mode": "total", "received": "300"})
	second := fixture.account(t, "Para llevar", "takeaway", nil, line(fixture.salad, 1))
	fixture.pay(t, second.Account.ID, map[string]any{"method_id": fixture.methods["debit"], "mode": "total"})

	catalogBody := decode[struct {
		Groups []struct {
			Reports []struct {
				ID      string `json:"id"`
				Filters []struct {
					Key string `json:"key"`
				} `json:"filters"`
				Columns []struct {
					Key string `json:"key"`
				} `json:"columns"`
			} `json:"reports"`
		} `json:"groups"`
	}](t, fixture.get(t, fixture.admin, "reports/catalog", 200))
	for _, group := range catalogBody.Groups {
		for _, report := range group.Reports {
			for _, filter := range report.Filters {
				if filter.Key == "data_source" {
					t.Fatalf("%s ofrece «Datos» sin lote importado", report.ID)
				}
			}
			for _, column := range report.Columns {
				if column.Key == "data_source" || column.Key == "legacy_difference" || column.Key == "legacy_sales_amount" {
					t.Fatalf("%s ofrece la columna %s sin lote importado", report.ID, column.Key)
				}
			}
		}
	}
	for _, id := range legacyReportIDs {
		base := fixture.normalizedReport(t, id, "period=current_year")
		if strings.Contains(base, "data_source") || strings.Contains(base, "legacy_difference") || strings.Contains(base, "Sistema anterior") {
			t.Fatalf("%s sin lote menciona el historial: %s", id, base)
		}
		for _, value := range []string{reports.DataSourceAll, reports.DataSourceNative, reports.DataSourceLegacy} {
			if other := fixture.normalizedReport(t, id, "period=current_year&data_source="+value); other != base {
				t.Fatalf("%s sin lote cambia con data_source=%s:\n%s\n%s", id, value, base, other)
			}
		}
	}
}

func TestStage9HLegacyHistoryInReports(t *testing.T) {
	fixture := freshFixture(t)
	location, err := time.LoadLocation("America/Monterrey")
	if err != nil {
		t.Fatal(err)
	}
	// Los cambios de horario de 2021 que usan las ventas de prueba (Monterrey dejó el horario de verano en 2022).
	if _, offset := time.Date(2021, 4, 4, 4, 30, 0, 0, location).Zone(); offset != -5*3600 {
		t.Fatalf("Se esperaba horario de verano el 04/04/2021 a las 04:30: %d", offset)
	}
	if _, offset := time.Date(2021, 10, 31, 4, 59, 0, 0, location).Zone(); offset != -6*3600 {
		t.Fatalf("Se esperaba horario normal el 31/10/2021 a las 04:59: %d", offset)
	}
	now := time.Now().In(location)
	tables := legacysynth.Dataset()
	addLegacySales(tables, append(legacyBoundarySales(now), legacyUndatedShiftSale))
	addLegacyUndatedShift(tables)
	fixture.importLegacy(t, tables)
	// Las fechas límite no comparten día comercial con las ventas del conjunto sintético.
	for _, day := range []string{"2025-03-17", "2025-03-18", "2025-03-19", "2021-04-03", "2021-04-04", "2021-10-30", "2021-10-31"} {
		others := fixture.scalar(t, `SELECT count(*)::text FROM app.legacy_sales
            WHERE legacy_id < 9000 AND ((coalesce(recognized_at,opened_at) AT TIME ZONE 'America/Monterrey') - interval '05:00')::date = $1::date`, day)
		if others != "0" {
			t.Fatalf("El conjunto sintético ya tiene ventas el %s: elige otra fecha límite", day)
		}
	}

	// Operación propia de AD-EL POS después de importar: un producto importado en una mesa importada y uno nuevo.
	var importedProduct, importedTable string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT
            (SELECT new_id::text FROM app.legacy_id_map WHERE entity='product' AND legacy_key='6'),
            (SELECT new_id::text FROM app.legacy_id_map WHERE entity='table' AND legacy_key='2')`).
		Scan(&importedProduct, &importedTable); err != nil {
		t.Fatal(err)
	}
	category := decode[catalog.Category](t, fixture.call(t, fixture.admin, "POST", "catalog/categories",
		catalog.Category{Metadata: catalog.Metadata{Name: "Novedades AD-EL POS", Active: true}}, 200))
	burger := decode[catalog.Product](t, fixture.call(t, fixture.admin, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Hamburguesa nueva", Active: true}, CategoryID: category.ID, Code: "NUEVA-1", Kind: "product",
		BasePrice: "120"}, 200))
	shift := decode[payments.Shift](t, fixture.mutate(t, "POST", "payments/shifts", map[string]any{"opening_float": "500"}, 200))
	fixture.shiftID = shift.ID
	table := operations.Table{ID: importedTable}
	dineIn := fixture.account(t, "Mesa importada", "dine_in", &table, line(catalog.Product{Metadata: catalog.Metadata{ID: importedProduct}}, 2))
	fixture.pay(t, dineIn.Account.ID, map[string]any{"method_id": fixture.methods["cash"], "mode": "total", "received": "500"})
	// En salón (un para llevar se reconoce al entregarse).
	second := fixture.account(t, "Otra cuenta", "dine_in", &table, line(burger, 1))
	fixture.pay(t, second.Account.ID, map[string]any{"method_id": fixture.methods["debit"], "mode": "total"})
	var nativeSale time.Time
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT max(recognized_at) FROM app.sales_recognitions").Scan(&nativeSale); err != nil {
		t.Fatal(err)
	}

	year2025 := "period=year&year=2025"
	mixed := "period=range&from=" + now.AddDate(0, 0, -35).Format("2006-01-02") + "&until=" + now.Format("2006-01-02")

	t.Run("catálogo y metadatos con el lote vigente", func(t *testing.T) {
		catalogBody := decode[struct {
			Groups []struct {
				Reports []struct {
					ID      string `json:"id"`
					Filters []struct {
						Key     string           `json:"key"`
						Default string           `json:"default"`
						Options []map[string]any `json:"options"`
					} `json:"filters"`
					Columns []struct {
						Key string `json:"key"`
					} `json:"columns"`
				} `json:"reports"`
			} `json:"groups"`
		}](t, fixture.get(t, fixture.admin, "reports/catalog", 200))
		seen := map[string]bool{}
		for _, group := range catalogBody.Groups {
			for _, report := range group.Reports {
				declared := slices.Contains(legacyReportIDs, report.ID)
				hasFilter := false
				for _, filter := range report.Filters {
					if filter.Key == "data_source" {
						hasFilter = true
						if filter.Default != "all" || len(filter.Options) != 3 {
							t.Fatalf("%s: «Datos» con otra forma: %+v", report.ID, filter)
						}
					}
				}
				hasColumn := slices.ContainsFunc(report.Columns, func(column struct {
					Key string `json:"key"`
				}) bool {
					return column.Key == "data_source"
				})
				// «Evolución de ventas» compara cubetas de tiempo de dos periodos: tiene el filtro pero no la columna.
				if hasFilter != declared || hasColumn != (declared && report.ID != "sales.evolution") {
					t.Fatalf("%s: filtro «Datos» %v y columna %v; se esperaba %v", report.ID, hasFilter, hasColumn, declared)
				}
				seen[report.ID] = true
			}
		}
		for _, id := range legacyReportIDs {
			if !seen[id] {
				t.Fatalf("%s no está en el catálogo", id)
			}
		}
		result := fixture.legacyRun(t, fixture.admin, "sales.by_day", year2025)
		if len(result.Meta.Filters) != 1 || result.Meta.Filters[0].Key != "data_source" ||
			result.Meta.Filters[0].Display != reports.BothSourcesLabel {
			t.Fatalf("Por omisión «Datos» es los dos y se describe en los metadatos: %+v", result.Meta.Filters)
		}
	})

	t.Run("cada reporte con los tres valores del filtro", func(t *testing.T) {
		for _, id := range legacyReportIDs {
			all := fixture.legacyRun(t, fixture.admin, id, year2025)
			legacy := fixture.legacyRun(t, fixture.admin, id, year2025+"&data_source=legacy")
			native := fixture.legacyRun(t, fixture.admin, id, year2025+"&data_source=native")
			// En 2025 solo hay historial: los dos y «Solo sistema anterior» coinciden; «Solo AD-EL POS» no tiene nada.
			if !reflect.DeepEqual(all.Rows, legacy.Rows) || !reflect.DeepEqual(all.Totals, legacy.Totals) ||
				!reflect.DeepEqual(all.Summary, legacy.Summary) {
				t.Fatalf("%s 2025: los dos ≠ solo sistema anterior:\n%+v\n%+v", id, all.Totals, legacy.Totals)
			}
			legacyRows := 0
			for _, row := range all.Rows {
				if row["data_source"] == reports.LegacySourceLabel {
					legacyRows++
				}
			}
			// «Evolución de ventas» compara cubetas de tiempo (sin «Datos»); «menos vendidos» puede no tener renglones.
			if legacyRows == 0 && id != "products.bottom" && id != "sales.evolution" {
				t.Fatalf("%s 2025: ningún renglón marcado «Sistema anterior»: %+v", id, all.Rows)
			}
			for _, row := range native.Rows {
				if source := row["data_source"]; source != nil && source != reports.NativeSourceLabel {
					t.Fatalf("%s 2025 solo AD-EL POS: renglón del historial %+v", id, row)
				}
			}
			for _, column := range native.Columns {
				if column.Total == "sum" && !number(t, native.Totals[column.Key]).IsZero() {
					t.Fatalf("%s 2025 solo AD-EL POS: total de %s %v", id, column.Key, native.Totals[column.Key])
				}
			}
			// Con AD-EL POS y el historial en el mismo periodo, cada total es la suma de los dos.
			mixedAll := fixture.legacyRun(t, fixture.admin, id, mixed)
			mixedNative := fixture.legacyRun(t, fixture.admin, id, mixed+"&data_source=native")
			mixedLegacy := fixture.legacyRun(t, fixture.admin, id, mixed+"&data_source=legacy")
			for _, column := range mixedAll.Columns {
				if column.Total != "sum" {
					continue
				}
				got := number(t, mixedAll.Totals[column.Key])
				want := number(t, mixedNative.Totals[column.Key]).Add(number(t, mixedLegacy.Totals[column.Key]))
				if !got.Equal(want) {
					t.Fatalf("%s periodo mixto: total de %s %s ≠ AD-EL POS %v + sistema anterior %v", id, column.Key, got,
						mixedNative.Totals[column.Key], mixedLegacy.Totals[column.Key])
				}
			}
		}
	})

	t.Run("totales exactos contra el lote", func(t *testing.T) {
		byDay := fixture.legacyRun(t, fixture.admin, "sales.by_day", year2025+"&data_source=legacy")
		from, until := byDay.Meta.Period.From, byDay.Meta.Period.Until
		sales := func(expression string) string {
			return fixture.scalar(t, `SELECT coalesce(sum(`+expression+`),0)::numeric(19,2)::text FROM app.legacy_sales
                WHERE status='paid' AND recognized_at >= $1 AND recognized_at < $2`, from, until)
		}
		accounts := fixture.scalar(t, `SELECT count(*)::text FROM app.legacy_sales WHERE status='paid' AND recognized_at >= $1 AND recognized_at < $2`,
			from, until)
		net, tax, difference := sales("amount_due"), sales("tax"), sales("total-lines_amount")
		gross, discounts, charges := sales("lines_amount"), sales("discount_amount"), sales("coalesce(charge_amount,0)")
		checkSales := func(id string, result legacyResult) {
			t.Helper()
			expected := map[string]string{"net": net, "tax": tax, "legacy_difference": difference, "gross": gross,
				"discount": discounts, "charge": charges, "accounts": accounts}
			for key, want := range expected {
				if !result.hasColumn(key) {
					continue
				}
				if got := number(t, result.Totals[key]); !got.Equal(decimal.RequireFromString(want)) {
					t.Fatalf("%s 2025: %s %s ≠ lote %s", id, key, got, want)
				}
			}
			if got := number(t, result.figure("net")); !got.Equal(decimal.RequireFromString(net)) {
				t.Fatalf("%s 2025: venta neta %s ≠ lote %s", id, got, net)
			}
		}
		if decimal.RequireFromString(difference).IsZero() || decimal.RequireFromString(discounts).IsZero() {
			t.Fatalf("El lote de 2025 debe tener diferencias y descuentos para la prueba: %s %s", difference, discounts)
		}
		for _, id := range []string{"sales.summary", "sales.by_day", "sales.by_week", "sales.by_month", "sales.by_hour", "sales.by_mode",
			"sales.accounts", "sales.by_shift"} {
			checkSales(id, fixture.legacyRun(t, fixture.admin, id, year2025+"&data_source=legacy"))
		}
		salon := fixture.legacyRun(t, fixture.admin, "sales.dine_in", year2025+"&data_source=legacy")
		counter := fixture.legacyRun(t, fixture.admin, "sales.takeaway", year2025+"&data_source=legacy")
		if got := number(t, salon.Totals["net"]).Add(number(t, counter.Totals["net"])); !got.Equal(decimal.RequireFromString(net)) {
			t.Fatalf("Salón + para llevar 2025 %s ≠ lote %s", got, net)
		}
		accountsReport := fixture.legacyRun(t, fixture.admin, "sales.accounts", year2025+"&data_source=legacy")
		if fmt.Sprint(len(accountsReport.Rows)) != accounts {
			t.Fatalf("Cuentas cerradas 2025: %d renglones ≠ %s ventas pagadas", len(accountsReport.Rows), accounts)
		}
		for _, row := range accountsReport.Rows {
			if !strings.HasPrefix(text(row["folio"]), "SA-") || row["kind"] != "Venta del sistema anterior" {
				t.Fatalf("Una venta del anterior se distingue por su folio y su tipo: %+v", row)
			}
			due := fixture.scalar(t, "SELECT amount_due::numeric(19,2)::text FROM app.legacy_sales WHERE 'SA-'||folio=$1 AND status='paid' LIMIT 1",
				text(row["folio"]))
			if text(row["net"]) != due && fixture.scalar(t, "SELECT count(*)::text FROM app.legacy_sales WHERE 'SA-'||folio=$1",
				text(row["folio"])) == "1" {
				t.Fatalf("La venta %s suma %s y se debía %s", row["folio"], row["net"], due)
			}
		}
		// Productos: los renglones con sus complementos; la diferencia, los descuentos y los cargos aparte.
		lines := func(expression string) string {
			return fixture.scalar(t, `SELECT coalesce(sum(`+expression+`),0)::numeric(19,2)::text FROM app.legacy_sale_lines x
                JOIN app.legacy_sales v ON v.batch_id=x.batch_id AND v.legacy_id=x.legacy_sale_id
                WHERE v.status='paid' AND v.recognized_at >= $1 AND v.recognized_at < $2`, from, until)
		}
		productNet, quantity := lines("x.total+x.complements_total"), lines("x.quantity")
		for _, id := range []string{"sales.by_product", "sales.by_category", "products.top", "products.bottom"} {
			result := fixture.legacyRun(t, fixture.admin, id, year2025+"&data_source=legacy")
			if got := number(t, result.Totals["net"]); !got.Equal(decimal.RequireFromString(productNet)) {
				t.Fatalf("%s 2025: venta de productos %s ≠ renglones del lote %s", id, got, productNet)
			}
			if got := number(t, result.Totals["quantity"]); !got.Equal(decimal.RequireFromString(quantity)) {
				t.Fatalf("%s 2025: cantidad %s ≠ lote %s", id, got, quantity)
			}
			for key, want := range map[string]string{"legacy_difference": difference, "account_discounts": discounts, "account_charges": charges} {
				if got := number(t, result.figure(key)); !got.Equal(decimal.RequireFromString(want)) {
					t.Fatalf("%s 2025: cifra %s %s ≠ lote %s", id, key, got, want)
				}
			}
		}
		trends := fixture.legacyRun(t, fixture.admin, "products.trends", year2025+"&data_source=legacy")
		previousQuantity := fixture.scalar(t, `SELECT coalesce(sum(x.quantity),0)::bigint::text FROM app.legacy_sale_lines x
            JOIN app.legacy_sales v ON v.batch_id=x.batch_id AND v.legacy_id=x.legacy_sale_id
            WHERE v.status='paid' AND v.recognized_at >= $1 AND v.recognized_at < $2`, from.AddDate(-1, 0, 0), from)
		if text(trends.Totals["quantity"]) != strings.TrimSuffix(quantity, ".00") || text(trends.Totals["previous_quantity"]) != previousQuantity {
			t.Fatalf("Tendencia 2025 contra 2024: %+v (lote %s y %s)", trends.Totals, quantity, previousQuantity)
		}
		evolution := fixture.legacyRun(t, fixture.admin, "sales.evolution", year2025+"&group_by=month&data_source=legacy")
		previousNet := fixture.scalar(t, `SELECT coalesce(sum(amount_due),0)::numeric(19,2)::text FROM app.legacy_sales
            WHERE status='paid' AND recognized_at >= $1 AND recognized_at < $2`, from.AddDate(-1, 0, 0), from)
		if text(evolution.Totals["net"]) != net || text(evolution.Totals["previous_net"]) != previousNet {
			t.Fatalf("Evolución 2025 contra 2024: %+v (lote %s y %s)", evolution.Totals, net, previousNet)
		}
		// Cobros: todos los pagos del anterior en su fecha, por forma de pago (como la validación §4.4).
		collected := fixture.scalar(t, `SELECT coalesce(sum(amount),0)::numeric(19,2)::text FROM app.legacy_payments
            WHERE paid_at >= $1 AND paid_at < $2`, from, until)
		count := fixture.scalar(t, "SELECT count(*)::text FROM app.legacy_payments WHERE paid_at >= $1 AND paid_at < $2", from, until)
		for _, id := range []string{"payments.by_method", "payments.by_day", "payments.by_shift", "payments.by_user", "payments.by_register"} {
			result := fixture.legacyRun(t, fixture.admin, id, year2025+"&data_source=legacy")
			if text(result.Totals["net"]) != collected || text(result.Totals["payments"]) != count {
				t.Fatalf("%s 2025: %+v ≠ pagos del lote %s (%s)", id, result.Totals, collected, count)
			}
		}
		methods := fixture.legacyRun(t, fixture.admin, "payments.by_method", year2025+"&data_source=legacy")
		for _, row := range methods.Rows {
			want := fixture.scalar(t, `SELECT coalesce(sum(amount),0)::numeric(19,2)::text FROM app.legacy_payments
                WHERE paid_at >= $1 AND paid_at < $2 AND method_name=$3`, from, until, text(row["method"]))
			if text(row["net"]) != want {
				t.Fatalf("Cobros 2025 de %s: %s ≠ lote %s", row["method"], row["net"], want)
			}
		}
		list := fixture.legacyRun(t, fixture.admin, "payments.list", year2025+"&data_source=legacy")
		if fmt.Sprint(len(list.Rows)) != count {
			t.Fatalf("Detalle de cobros 2025: %d ≠ %s pagos", len(list.Rows), count)
		}
		// Cancelaciones, cambios de cantidad, complementos y cortes.
		cancelled := fixture.legacyRun(t, fixture.admin, "sales.voided_accounts", year2025+"&data_source=legacy")
		cancelledAmount := fixture.scalar(t, `SELECT coalesce(sum(amount_due),0)::numeric(19,2)::text FROM app.legacy_sales
            WHERE status='cancelled' AND cancelled_at >= $1 AND cancelled_at < $2`, from, until)
		cancelledCount := fixture.scalar(t, `SELECT count(*)::text FROM app.legacy_sales
            WHERE status='cancelled' AND cancelled_at >= $1 AND cancelled_at < $2`, from, until)
		if text(cancelled.Totals["cancelled_amount"]) != cancelledAmount || fmt.Sprint(len(cancelled.Rows)) != cancelledCount {
			t.Fatalf("Cuentas canceladas 2025: %+v (%d) ≠ lote %s (%s)", cancelled.Totals, len(cancelled.Rows), cancelledAmount, cancelledCount)
		}
		byReason := fixture.legacyRun(t, fixture.admin, "sales.cancellations_by_reason", year2025+"&data_source=legacy")
		if text(byReason.Totals["accounts"]) != cancelledCount || text(byReason.Totals["cancelled_amount"]) != cancelledAmount {
			t.Fatalf("Cancelaciones por motivo 2025: %+v", byReason.Totals)
		}
		changes := fixture.legacyRun(t, fixture.admin, "products.quantity_changes", year2025+"&data_source=legacy")
		changesCount := fixture.scalar(t, `SELECT count(*)::text FROM app.legacy_quantity_changes
            WHERE changed_at >= $1 AND changed_at < $2 AND new_quantity IS DISTINCT FROM original_quantity`, from, until)
		if fmt.Sprint(len(changes.Rows)) != changesCount {
			t.Fatalf("Modificaciones de cantidades 2025: %d ≠ lote %s", len(changes.Rows), changesCount)
		}
		complements := fixture.legacyRun(t, fixture.admin, "products.complements", year2025+"&data_source=legacy")
		portions := fixture.scalar(t, `SELECT coalesce(sum(x.quantity*c.requested),0)::text FROM app.legacy_line_complements c
            JOIN app.legacy_sale_lines x ON x.batch_id=c.batch_id AND x.legacy_id=c.legacy_line_id
            JOIN app.legacy_sales v ON v.batch_id=x.batch_id AND v.legacy_id=x.legacy_sale_id
            WHERE v.status='paid' AND v.recognized_at >= $1 AND v.recognized_at < $2 AND c.requested > 0`, from, until)
		if !number(t, complements.Totals["portions"]).Equal(decimal.RequireFromString(portions)) {
			t.Fatalf("Complementos 2025: porciones %v ≠ lote %s", complements.Totals["portions"], portions)
		}
		shifts := fixture.legacyRun(t, fixture.admin, "cash.shifts", year2025+"&data_source=legacy")
		shiftCount := fixture.scalar(t, `SELECT count(*)::text FROM app.legacy_cash_shifts
            WHERE (opened_at AT TIME ZONE 'America/Monterrey')::date BETWEEN '2025-01-01' AND '2025-12-31'`)
		if fmt.Sprint(len(shifts.Rows)) != shiftCount || shiftCount == "0" {
			t.Fatalf("Cortes del anterior en 2025: %d ≠ lote %s", len(shifts.Rows), shiftCount)
		}
	})

	t.Run("día comercial, cambio de horario, cuatro decimales y diferencia", func(t *testing.T) {
		day := func(date string) legacyResult {
			return fixture.legacyRun(t, fixture.admin, "sales.by_day", "period=day&date="+date+"&data_source=legacy")
		}
		expectDay := func(date, net string) {
			t.Helper()
			if got := text(day(date).Totals["net"]); got != net {
				t.Fatalf("Venta del %s: %s; se esperaba %s", date, got, net)
			}
		}
		// 04:59 pertenece al día anterior con la hora de inicio de las 05:00; 05:00 ya es el día siguiente. Los dos
		// renglones de 10.1234 suman 20.2468, que se muestra 20.25 (redondeo solo al mostrar).
		expectDay("2025-03-17", "20.25")
		march := day("2025-03-18")
		if text(march.Totals["net"]) != "108.00" || text(march.Totals["gross"]) != "100.00" || text(march.Totals["discount"]) != "5.00" ||
			text(march.Totals["charge"]) != "3.00" || text(march.Totals["legacy_difference"]) != "10.00" || text(march.Totals["accounts"]) != "1" {
			t.Fatalf("Venta del 18/03/2025 (la cancelada no es venta; total guardado 110 con renglones 100): %+v", march.Totals)
		}
		products := fixture.legacyRun(t, fixture.admin, "sales.by_product", "period=day&date=2025-03-17&data_source=legacy")
		if len(products.Rows) != 2 || text(products.Rows[0]["net"]) != "10.12" || text(products.Totals["net"]) != "20.25" {
			t.Fatalf("Cuatro decimales por producto: %+v %+v", products.Rows, products.Totals)
		}
		difference := fixture.legacyRun(t, fixture.admin, "sales.by_product", "period=day&date=2025-03-18&data_source=legacy")
		if len(difference.Rows) != 1 || text(difference.Totals["net"]) != "100.00" || text(difference.figure("legacy_difference")) != "10.00" ||
			text(difference.figure("account_discounts")) != "5.00" || text(difference.figure("account_charges")) != "3.00" ||
			text(difference.figure("net")) != "108.00" {
			t.Fatalf("La diferencia no se reparte entre productos: %+v %+v", difference.Rows, difference.Summary)
		}
		accounts := fixture.legacyRun(t, fixture.admin, "sales.accounts", "period=day&date=2025-03-18&data_source=legacy")
		if len(accounts.Rows) != 1 || accounts.Rows[0]["folio"] != "SA-9002" || text(accounts.Rows[0]["net"]) != "108.00" ||
			text(accounts.Rows[0]["legacy_difference"]) != "10.00" || accounts.Rows[0]["data_source"] != reports.LegacySourceLabel {
			t.Fatalf("La venta del anterior con su diferencia: %+v", accounts.Rows)
		}
		summary := fixture.legacyRun(t, fixture.admin, "sales.summary", "period=day&date=2025-03-18&data_source=legacy")
		noRate := map[string]any{}
		for _, row := range summary.Rows {
			if row["rate"] == "Sin tasa (sistema anterior)" {
				noRate = row
			}
		}
		if text(noRate["net"]) != "8.00" || text(noRate["legacy_difference"]) != "10.00" {
			t.Fatalf("Descuento, cargo y diferencia sin tasa: %+v", summary.Rows)
		}
		// Cambios de horario de 2021: el día del adelanto dura 23 horas y el del atraso 25.
		expectDay("2021-04-03", "40.00")
		expectDay("2021-04-04", "0.00")
		expectDay("2021-10-30", "50.00")
		expectDay("2021-10-31", "60.00")
		hours := fixture.legacyRun(t, fixture.admin, "sales.by_hour", "period=day&date=2021-10-30&data_source=legacy")
		if row := find(t, reportResult{Rows: hours.Rows}, "hour", "04:00 – 04:59"); text(row["net"]) != "50.00" {
			t.Fatalf("Hora local de la venta de las 04:59 después del atraso: %+v", row)
		}
		// La hora de inicio del día reagrupa los mismos instantes sin reescribir nada.
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.inventory_settings SET operating_day_cutoff='04:00'"); err != nil {
			t.Fatal(err)
		}
		expectDay("2025-03-17", "0.00")
		expectDay("2025-03-18", "128.25")
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.inventory_settings SET operating_day_cutoff='05:00'"); err != nil {
			t.Fatal(err)
		}
		expectDay("2025-03-17", "20.25")
		// Una venta pagada sin corte queda «Sin turno»; los cortes del anterior son turnos propios.
		shifts := fixture.legacyRun(t, fixture.admin, "sales.by_shift", "period=day&date=2025-03-19&data_source=legacy")
		if len(shifts.Rows) != 1 || shifts.Rows[0]["register"] != "Sin turno" || text(shifts.Rows[0]["net"]) != "70.00" {
			t.Fatalf("Venta del anterior sin corte: %+v", shifts.Rows)
		}
	})

	t.Run("corte del anterior sin fecha de inicio", func(t *testing.T) {
		// Sin fecha_inicio en el respaldo, el corte 9100 toma la de su venta: aparece en «Turnos de caja» del día de su
		// venta y su pago, y en «Ventas por turno» con esa fecha.
		if got := fixture.scalar(t, `SELECT array_to_string(flags, ',') FROM app.legacy_cash_shifts WHERE legacy_id = 9100`); got !=
			"start_estimated,end_estimated" {
			t.Fatalf("Marcas del corte sin fechas: %s", got)
		}
		shifts := fixture.legacyRun(t, fixture.admin, "cash.shifts", "period=day&date=2023-07-12&data_source=legacy")
		// 13:00 y 13:45 en Monterrey (UTC−6 desde 2022): la venta y el pago.
		if len(shifts.Rows) != 1 || shifts.Rows[0]["data_source"] != reports.LegacySourceLabel ||
			!strings.HasPrefix(text(shifts.Rows[0]["business_date"]), "2023-07-12") ||
			text(shifts.Rows[0]["opened_at"]) != "2023-07-12T19:00:00Z" || text(shifts.Rows[0]["closed_at"]) != "2023-07-12T19:45:00Z" ||
			!number(t, shifts.Rows[0]["sales"]).Equal(decimal.RequireFromString("33")) {
			t.Fatalf("Turnos de caja del 12/07/2023: %+v", shifts.Rows)
		}
		byShift := fixture.legacyRun(t, fixture.admin, "sales.by_shift", "period=day&date=2023-07-12&data_source=legacy")
		if !slices.ContainsFunc(byShift.Rows, func(row map[string]any) bool {
			return strings.HasPrefix(text(row["business_date"]), "2023-07-12") && text(row["net"]) == "33.00"
		}) {
			t.Fatalf("Ventas por turno del 12/07/2023: %+v", byShift.Rows)
		}
	})

	t.Run("canceladas fuera de la venta y cobros del anterior", func(t *testing.T) {
		cancelled := fixture.legacyRun(t, fixture.admin, "sales.voided_accounts", "period=day&date=2025-03-18&data_source=legacy")
		authorizer := fixture.scalar(t, "SELECT authorized_name FROM app.legacy_cancellations WHERE legacy_sale_id=9003")
		if len(cancelled.Rows) != 1 || cancelled.Rows[0]["folio"] != "SA-9003" || text(cancelled.Rows[0]["cancelled_amount"]) != "30.00" ||
			text(cancelled.Rows[0]["cancelled_units"]) != "3" || cancelled.Rows[0]["authorized_by"] != authorizer ||
			cancelled.Rows[0]["reason"] != "Sin motivo (sistema anterior)" || cancelled.Rows[0]["data_source"] != reports.LegacySourceLabel {
			t.Fatalf("Venta cancelada del anterior: %+v", cancelled.Rows)
		}
		withoutCatalogReason := fixture.legacyRun(t, fixture.admin, "sales.voided_accounts", "period=day&date=2025-03-18&reason_id=none")
		if len(withoutCatalogReason.Rows) != 1 {
			t.Fatalf("«Sin motivo del catálogo» incluye las cancelaciones del anterior: %+v", withoutCatalogReason.Rows)
		}
		list := fixture.legacyRun(t, fixture.admin, "payments.list", "period=day&date=2025-03-18&data_source=legacy")
		kinds := map[string]string{}
		for _, row := range list.Rows {
			kinds[text(row["kind"])+" · "+text(row["method"])] = text(row["amount"])
		}
		if len(list.Rows) != 2 || kinds["Cobro de una venta cancelada · Efectivo"] != "30.00" ||
			kinds["Cobro · Tarjeta (sistema anterior)"] != "108.00" {
			t.Fatalf("Cobros del 18/03/2025: %+v", list.Rows)
		}
		methods := fixture.legacyRun(t, fixture.admin, "payments.by_method", "period=day&date=2025-03-18&data_source=legacy")
		if text(methods.Totals["net"]) != "138.00" || len(methods.Rows) != 2 {
			t.Fatalf("Cobros por método del 18/03/2025: %+v", methods.Rows)
		}
		// Un usuario con el alcance de Caja limitado ve los cobros del anterior (no son turnos de AD-EL POS), pero no
		// sus cortes.
		member := fixture.staff(t, "reportes9h", reports.PermissionRead, reports.PermissionPayments, reports.PermissionCash,
			"cash.shifts.read")
		scoped := fixture.legacyRun(t, member, "payments.by_method", "period=day&date=2025-03-18&data_source=legacy")
		if text(scoped.Totals["net"]) != "138.00" {
			t.Fatalf("Cobros del anterior fuera del alcance de Caja: %+v", scoped.Totals)
		}
		cortes := fixture.legacyRun(t, member, "cash.shifts", year2025+"&data_source=legacy")
		if len(cortes.Rows) != 0 || !slices.ContainsFunc(cortes.Meta.Notes, func(note string) bool {
			return strings.Contains(note, "cortes del sistema anterior solo se muestran")
		}) {
			t.Fatalf("Los cortes del anterior exigen todas las cajas y el historial: %+v %v", cortes.Rows, cortes.Meta.Notes)
		}
		protected := fixture.legacyRun(t, fixture.admin, "cash.shifts", year2025+"&data_source=legacy")
		if !protected.hasColumn("difference_mxn") || protected.Rows[0]["difference_mxn"] == nil ||
			protected.Rows[0]["legacy_sales_amount"] == nil || protected.Rows[0]["register"] == nil {
			t.Fatalf("Corte del anterior con su diferencia y su venta guardadas: %+v", protected.Rows)
		}
	})

	t.Run("AD-EL POS y el historial juntos", func(t *testing.T) {
		products := fixture.legacyRun(t, fixture.admin, "sales.by_product", mixed)
		merged := find(t, reportResult{Rows: products.Rows}, "code", "SA-6")
		legacyOnly := find(t, reportResult{Rows: fixture.legacyRun(t, fixture.admin, "sales.by_product", mixed+"&data_source=legacy").Rows},
			"code", "SA-6")
		if merged["data_source"] != reports.BothSourcesLabel ||
			!number(t, merged["quantity"]).Equal(number(t, legacyOnly["quantity"]).Add(decimal.NewFromInt(2))) {
			t.Fatalf("Un producto importado vendido en los dos sistemas es un solo renglón: %+v (historial %+v)", merged, legacyOnly)
		}
		burgerRow := find(t, reportResult{Rows: products.Rows}, "code", "NUEVA-1")
		if burgerRow["data_source"] != reports.NativeSourceLabel {
			t.Fatalf("Producto de AD-EL POS: %+v", burgerRow)
		}
		accounts := fixture.legacyRun(t, fixture.admin, "sales.accounts", mixed)
		marks := map[string]int{}
		for _, row := range accounts.Rows {
			marks[text(row["data_source"])]++
		}
		if marks[reports.LegacySourceLabel] < 2 || marks[reports.NativeSourceLabel] != 2 {
			t.Fatalf("Cuentas de los dos sistemas: %v", marks)
		}
		// La venta del anterior de hace dos minutos y la de AD-EL POS caen en el mismo día comercial (salvo justo en la
		// hora de inicio): un solo renglón por día y en cobros por día.
		recognized := nativeSale
		legacyDay := businessday.DayOf(now.Add(-2*time.Minute), location, 5, 0).Date
		if businessday.DayOf(recognized, location, 5, 0).Date.Equal(legacyDay) {
			for _, id := range []string{"sales.by_day", "payments.by_day"} {
				result := fixture.legacyRun(t, fixture.admin, id, mixed)
				row := find(t, reportResult{Rows: result.Rows}, "day", legacyDay.Format("2006-01-02"))
				if row["data_source"] != reports.BothSourcesLabel {
					t.Fatalf("%s: el día con ventas de los dos sistemas es un solo renglón: %+v", id, row)
				}
			}
		}
		// El resumen ejecutivo es la operación de AD-EL POS: no incluye el historial.
		dashboard := decode[struct {
			Sections []struct {
				ID         string `json:"id"`
				Indicators []struct {
					Key   string `json:"key"`
					Value any    `json:"value"`
				} `json:"indicators"`
			} `json:"sections"`
		}](t, fixture.get(t, fixture.admin, "reports/dashboard?period=year&year=2025", 200))
		for _, section := range dashboard.Sections {
			for _, indicator := range section.Indicators {
				if section.ID == "sales" && indicator.Key == "net_sales" && text(indicator.Value) != "0.00" {
					t.Fatalf("El resumen ejecutivo de 2025 no incluye el historial: %v", indicator.Value)
				}
			}
		}
	})

	t.Run("exportación con el filtro y la marca", func(t *testing.T) {
		query := "period=day&date=2025-03-18&data_source=legacy"
		pdf := fixture.exportOK(t, fixture.admin, "sales.by_day", "format=pdf&"+query)
		content := strings.Join(reports.PDFPagesForTest(t, pdf.body), "\n")
		for _, expected := range []string{"Datos: Solo sistema anterior", "Diferencia del sistema anterior", "Sistema anterior", "108.00"} {
			if !strings.Contains(content, expected) {
				t.Fatalf("El PDF no contiene %q:\n%s", expected, content)
			}
		}
		excel := fixture.exportOK(t, fixture.admin, "sales.by_day", "format=xlsx&"+query)
		assertSameAsScreen(t, "sales.by_day", fixture.screen(t, fixture.admin, "sales.by_day", query), excel.body)
		filters := strings.Builder{}
		for _, row := range sheetRows(t, workbook(t, excel.body), "Filtros", false) {
			filters.WriteString(strings.Join(row, " | ") + "\n")
		}
		if !strings.Contains(filters.String(), "Datos") || !strings.Contains(filters.String(), "Solo sistema anterior") {
			t.Fatalf("La hoja de filtros no tiene «Datos»: %s", filters.String())
		}
		payments := fixture.exportOK(t, fixture.admin, "payments.list", "format=xlsx&"+query)
		assertSameAsScreen(t, "payments.list", fixture.screen(t, fixture.admin, "payments.list", query), payments.body)
		if body := workbookText(t, payments.body); !strings.Contains(body, "Tarjeta (sistema anterior)") || !strings.Contains(body, "SA-9002") {
			t.Fatal("El libro de cobros no distingue los del anterior")
		}
	})

	t.Run("filtros de lo que solo existe en AD-EL POS excluyen el historial", func(t *testing.T) {
		values := url.Values{"period": {"year"}, "year": {"2025"}, "register_id": {fixture.registerID}}
		for _, id := range []string{"payments.by_method", "sales.by_shift"} {
			result := fixture.legacyRun(t, fixture.admin, id, values.Encode())
			if len(result.Rows) != 0 {
				t.Fatalf("%s con una caja de AD-EL POS no incluye el historial: %+v", id, result.Rows)
			}
		}
		product := url.Values{"period": {"year"}, "year": {"2025"}, "product_id": {importedProduct}, "data_source": {"legacy"}}
		result := fixture.legacyRun(t, fixture.admin, "sales.by_product", product.Encode())
		if len(result.Rows) != 1 || result.Rows[0]["code"] != "SA-6" {
			t.Fatalf("El filtro de un producto importado filtra el historial por su identificador nuevo: %+v", result.Rows)
		}
	})
}
