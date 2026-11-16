//go:build integration

package reports_test

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/legacysynth"
	"local/ad-el-pos/server/internal/reports"
)

// Medición opcional (ADEL_REPORTS_MEASURE=1, sin -race para tiempos representativos): un historial sintético con un
// volumen parecido al real (≈ 230 mil ventas de 10/2019 a 09/2026, 720 mil renglones del libro, 100 mil complementos
// y 230 mil pagos) importado con el importador real, y los reportes del historial de un año completo (2025) y del
// periodo más largo que admiten (400 días, 2024–2025), con los tres valores de «Datos», en el ejecutor real de solo
// lectura. Imprime además el plan (EXPLAIN ANALYZE) de las consultas principales. No es una prueba de aceptación: solo
// registra tiempos. Usa su propia base desde cero.
func TestStage9HLegacyReportsVolume(t *testing.T) {
	if os.Getenv("ADEL_REPORTS_MEASURE") != "1" {
		t.Skip("Medición opt-in: ADEL_REPORTS_MEASURE=1")
	}
	fixture := freshFixture(t)
	ctx, pool := fixture.ctx, fixture.pool
	started := time.Now()
	tables := legacysynth.Dataset()
	byName := map[string]*legacysynth.Table{}
	for _, table := range tables {
		byName[table.Name] = table
	}
	const sales = 230000
	firstSale, firstLine, firstPayment := int64(100000), int64(1000000), int64(100000)
	lines := int64(0)
	base := time.Date(2019, 10, 1, 9, 0, 0, 0, time.UTC)
	for index := int64(0); index < sales; index++ {
		id := firstSale + index
		moment := base.Add(time.Duration(index) * 16 * time.Minute)
		stamp := moment.Format(legacyLayout)
		count := int64(3)
		if index%8 == 0 {
			count = 4
		}
		// Corte del año (los cortes sintéticos 1 a 6 cubren de 2019 a 2026); una de cada cinco, para llevar y sin corte.
		table, shift := int64(2+index%7), any(int64(min(max(moment.Year()-2020, 1), 6)))
		if index%5 == 0 {
			table, shift = 1, nil
		}
		byName["ventas"].Rows = append(byName["ventas"].Rows, []any{id, int64(1), table, nil, fmt.Sprint(id), stamp,
			int64(1), int64(0), fmt.Sprintf("%d.0000", 50*count), "0.0000", fmt.Sprintf("%d.0000", 50*count),
			fmt.Sprintf("%d.0000", 50*count), int64(1), nil, "0.0000", int64(0), shift, int64(0)})
		for line := int64(0); line < count; line++ {
			lineID := firstLine + lines
			byName["ventas_productos"].Rows = append(byName["ventas_productos"].Rows, []any{lineID, id, int64(1 + (index+line)%17),
				"1.000", "1.000", "50.0000", "0.0000", "50.0000", nil})
			lines++
			if lineID%20 < 9 {
				byName["ventas_complementos"].Rows = append(byName["ventas_complementos"].Rows, []any{lineID, int64(1), "1.000",
					"1.000", "0.0000", "0.0000", "0.0000"})
			}
		}
		byName["pagos"].Rows = append(byName["pagos"].Rows, []any{firstPayment + index, id, "", int64(1 + index%2),
			fmt.Sprintf("%d.0000", 50*count), int64(1), stamp, shift})
	}
	fixture.importLegacy(t, tables)
	tables, byName = nil, nil
	if _, err := pool.Exec(ctx, "ANALYZE"); err != nil {
		t.Fatal(err)
	}
	var ledger, payments, size string
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.legacy_sales_recognitions)::text,
            (SELECT count(*) FROM app.legacy_payments)::text,pg_size_pretty(pg_database_size(current_database()))`).
		Scan(&ledger, &payments, &size); err != nil {
		t.Fatal(err)
	}
	t.Logf("Historial sintético generado e importado en %s: %s renglones del libro, %s pagos; base de %s",
		time.Since(started).Round(time.Millisecond), ledger, payments, size)
	runner := reports.NewRunner(pool, reports.Slots)
	actor := reports.Actor{Permissions: []string{reports.PermissionRead, reports.PermissionSales, reports.PermissionPayments,
		reports.PermissionCash, reports.PermissionCashExpected, "cash.shifts.read", "cash.shifts.read_all", "cash.history.read"}}
	periods := []struct {
		label  string
		values url.Values
	}{
		{"año 2025", url.Values{"period": {"year"}, "year": {"2025"}}},
		{"400 días (2024–2025)", url.Values{"period": {"range"}, "from": {"2024-11-27"}, "until": {"2025-12-31"}}},
	}
	slowest := time.Duration(0)
	for _, period := range periods {
		for _, id := range legacyReportIDs {
			timings := []string{}
			rows := 0
			for _, source := range []string{reports.DataSourceAll, reports.DataSourceLegacy, reports.DataSourceNative} {
				values := url.Values{"data_source": {source}}
				for key, list := range period.values {
					values[key] = list
				}
				begin := time.Now()
				result, err := reports.Run(ctx, runner, actor, id, values)
				if err != nil {
					t.Fatalf("%s (%s, %s): %v", id, period.label, source, err)
				}
				elapsed := time.Since(begin)
				slowest = max(slowest, elapsed)
				if source == reports.DataSourceAll {
					rows = result.Page.TotalRows
				}
				timings = append(timings, source+" "+elapsed.Round(time.Millisecond).String())
			}
			t.Logf("%s (%s): %d renglones; %s", id, period.label, rows, strings.Join(timings, ", "))
		}
	}
	t.Logf("Reporte más lento: %s", slowest.Round(time.Millisecond))
	both := reports.LedgerFromForTest(true, true)
	queries := map[string]string{
		"ventas por día, año 2025 (los dos)": `SELECT ((r.recognized_at AT TIME ZONE 'America/Monterrey') - interval '05:00')::date,
                sum(r.net),sum(r.legacy_difference)
            FROM ` + both + ` WHERE r.recognized_at >= '2025-01-01 11:00+00' AND r.recognized_at < '2026-01-01 11:00+00' GROUP BY 1`,
		"ventas por producto, año 2025 (los dos)": `SELECT r.product_id,(array_agg(r.product_name ORDER BY r.sequence DESC))[1],
                sum(r.quantity),sum(r.net),count(DISTINCT r.account_id)
            FROM ` + both + ` WHERE r.recognized_at >= '2025-01-01 11:00+00' AND r.recognized_at < '2026-01-01 11:00+00'
                AND r.product_id IS NOT NULL GROUP BY r.product_id`,
		"cobros del anterior por método, año 2025": `SELECT p.method,sum(p.amount) FROM app.legacy_payments p
            WHERE p.paid_at >= '2025-01-01 11:00+00' AND p.paid_at < '2026-01-01 11:00+00' GROUP BY p.method`,
		"ventas canceladas del anterior, año 2025": `SELECT count(*),sum(v.amount_due) FROM app.legacy_sales v
            WHERE v.status='cancelled' AND v.cancelled_at >= '2025-01-01 11:00+00' AND v.cancelled_at < '2026-01-01 11:00+00'`,
		"complementos del anterior, año 2025": `SELECT c.legacy_complement_id,sum(l.quantity*c.requested)
            FROM app.legacy_sales_recognitions l
            JOIN app.legacy_line_complements c ON c.batch_id=l.batch_id AND c.legacy_line_id=l.legacy_line_id
            WHERE c.requested > 0 AND l.entry_kind='line' AND l.recognized_at >= '2025-01-01 11:00+00'
                AND l.recognized_at < '2026-01-01 11:00+00' GROUP BY 1`,
	}
	for name, query := range queries {
		rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query)
		if err != nil {
			t.Fatal(err)
		}
		plan := []string{}
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, line)
		}
		rows.Close()
		t.Logf("== %s\n%s", name, strings.Join(plan, "\n"))
	}
}
