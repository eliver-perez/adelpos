//go:build integration

package legacyimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/legacypkg"
	"local/ad-el-pos/server/internal/legacysynth"
)

// TestLegacyImportVolume mide, con un volumen parecido al real (≈ 230 mil ventas, 720 mil renglones, 330 mil
// complementos y 230 mil pagos, todo sintético), el tamaño del paquete, el tiempo de lectura (análisis), la memoria
// del modelo, el tiempo de la importación y cuánto quedan tomados los locks de la operación. Opcional
// (ADEL_LEGACY_MEASURE=1), como las demás mediciones de volumen: las suites ordinarias la omiten. Con -race los
// tiempos no son representativos.
func TestLegacyImportVolume(t *testing.T) {
	if os.Getenv("ADEL_LEGACY_MEASURE") != "1" {
		t.Skip("medición opcional: ADEL_LEGACY_MEASURE=1")
	}
	pool := testDatabase(t)
	admin, device := installation(t, pool)
	ctx := context.Background()
	start := time.Now()
	tables := legacysynth.Dataset()
	byName := map[string]*legacysynth.Table{}
	for _, table := range tables {
		byName[table.Name] = table
	}
	const sales = 230000
	firstSale, firstLine, firstPayment := int64(100000), int64(1000000), int64(100000)
	lines, complements := 0, 0
	base := time.Date(2019, 10, 1, 9, 0, 0, 0, time.UTC)
	for index := int64(0); index < sales; index++ {
		id := firstSale + index
		moment := base.Add(time.Duration(index) * 16 * time.Minute)
		stamp := moment.Format("2006-01-02T15:04:05.000")
		count := int64(3)
		if index%8 == 0 {
			count = 4
		}
		byName["ventas"].Rows = append(byName["ventas"].Rows, []any{id, int64(1), int64(1 + index%5), nil, fmt.Sprint(id), stamp,
			int64(1), int64(0), fmt.Sprintf("%d.0000", 50*count), "0.0000", fmt.Sprintf("%d.0000", 50*count),
			fmt.Sprintf("%d.0000", 50*count), int64(1), nil, "0.0000", int64(0), int64(4), int64(0)})
		for line := int64(0); line < count; line++ {
			lineID := firstLine + int64(lines)
			byName["ventas_productos"].Rows = append(byName["ventas_productos"].Rows, []any{lineID, id, int64(1 + line%3),
				"1.000", "1.000", "50.0000", "0.0000", "50.0000", nil})
			lines++
			if (lineID % 20) < 9 {
				byName["ventas_complementos"].Rows = append(byName["ventas_complementos"].Rows, []any{lineID, int64(1), "1.000",
					"1.000", "0.0000", "0.0000", "0.0000"})
				complements++
			}
		}
		byName["pagos"].Rows = append(byName["pagos"].Rows, []any{firstPayment + index, id, "", int64(1 + index%2),
			fmt.Sprintf("%d.0000", 50*count), int64(1), stamp, int64(4)})
	}
	generated := time.Since(start)
	path := filepath.Join(t.TempDir(), "volumen.adelmig")
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
	written, err := writer.Finish(file, syntheticPassphrase, legacypkg.PackageInfo{CreatedAt: time.Now(),
		Creator: legacypkg.Creator{Product: "AD-EL POS", Version: "0.10.0", Tool: "medicion-9h-c"},
		Source:  legacypkg.Source{Kind: legacypkg.SourceSynthetic}, Summary: summary})
	writer.Close()
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	packaged := time.Since(start) - generated
	tables, byName = nil, nil
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	// Lectura completa (lo que hace el análisis) y correspondencia.
	readStart := time.Now()
	environment, err := ReadEnvironment(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	source, closer, err := OpenPackage(path, syntheticPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	model, err := Load(ctx, source, environment.Config.Location, nil)
	closer.Close()
	if err != nil {
		t.Fatal(err)
	}
	read := time.Since(readStart)
	plan, preview := Analyze(model, environment, written.Bytes, "")
	analyzed := time.Since(readStart)
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	content, _ := os.ReadFile(path)
	digest := sha256.Sum256(content)
	importStart := time.Now()
	report, err := Import(ctx, pool, plan, environment.Config, ImportRequest{UserID: admin, Username: "admin", DeviceID: device,
		PackageSHA256: hex.EncodeToString(digest[:]), PackageBytes: written.Bytes}, nil)
	if err != nil {
		t.Fatal(err)
	}
	imported := time.Since(importStart)
	if !report.Validation.OK {
		t.Fatalf("validación con volumen: %+v", report.Validation)
	}
	var database string
	if err := pool.QueryRow(ctx, "SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&database); err != nil {
		t.Fatal(err)
	}
	t.Logf("volumen sintético: %d ventas, %d renglones, %d complementos, %d pagos", preview.Counts[CountSales],
		preview.Counts[CountLines], preview.Counts[CountLineComplements], preview.Counts[CountPayments])
	t.Logf("generar: %s; paquete: %s, %.1f MB", generated.Round(time.Millisecond), packaged.Round(time.Millisecond),
		float64(written.Bytes)/1e6)
	t.Logf("análisis: lectura %s, con la correspondencia %s; memoria del modelo ≈ %.0f MB (montón en uso)",
		read.Round(time.Millisecond), analyzed.Round(time.Millisecond), float64(after.HeapInuse-min(after.HeapInuse, before.HeapInuse))/1e6)
	t.Logf("importación: %s; tiempos (ms) %v; base después de importar %s", imported.Round(time.Millisecond), report.TimingsMS, database)

	// Reemplazo con las estadísticas del lote vigente (revisión de la Fase E, D-1): después de la primera importación el
	// autovacuum analiza las tablas del historial y solo conocen ese lote; aquí se fuerza con ANALYZE. El reemplazo debe
	// quedar en el mismo orden que la primera importación (si vuelve el plan cuadrático, el plazo lo corta).
	if _, err := pool.Exec(ctx, "ANALYZE app."+strings.Join(historyTables, ", app.")); err != nil {
		t.Fatal(err)
	}
	t.Logf("plan de la validación del libro para un lote nuevo con las estadísticas del vigente:\n%s",
		explainText(t, pool, ledgerCheckSQL, uuidText(newUUID())))
	environment, err = ReadEnvironment(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	replacePlan, replacePreview := Analyze(model, environment, written.Bytes, "")
	if !replacePreview.WillReplace {
		t.Fatal("el segundo análisis debía ofrecer reemplazar el lote")
	}
	limit := 3*imported + time.Minute
	replaceCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	replaceStart := time.Now()
	replaced, err := Import(replaceCtx, pool, replacePlan, environment.Config, ImportRequest{UserID: admin, Username: "admin",
		DeviceID: device, ExpectedBatchID: report.BatchID, PackageSHA256: hex.EncodeToString(digest[:]), PackageBytes: written.Bytes}, nil)
	if err != nil {
		t.Fatalf("el reemplazo no terminó en %s (3 × la primera importación + 1 min): %v", limit.Round(time.Second), err)
	}
	replacedIn := time.Since(replaceStart)
	if !replaced.Validation.OK || replaced.ReplacedBatchID != report.BatchID {
		t.Fatalf("validación del reemplazo con volumen: %+v", replaced.Validation)
	}
	if err := pool.QueryRow(ctx, "SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&database); err != nil {
		t.Fatal(err)
	}
	t.Logf("reemplazo: %s; tiempos (ms) %v; base después del reemplazo %s", replacedIn.Round(time.Millisecond),
		replaced.TimingsMS, database)
}

// explainText es el plan (EXPLAIN, sin ejecutar) de una consulta con sus argumentos, para la bitácora de la medición.
func explainText(t *testing.T, pool *pgxpool.Pool, query string, args ...any) string {
	t.Helper()
	rows, err := pool.Query(context.Background(), "EXPLAIN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	lines := []string{}
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}
