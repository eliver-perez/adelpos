//go:build integration

package legacyimport

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// previousLedgerCheckSQL es la consulta de la validación del libro antes de la corrección de la Fase E (D-1), solo como
// control de que la prueba reproduce las estadísticas del defecto: una subconsulta agregada del lote dentro de la unión.
const previousLedgerCheckSQL = `SELECT count(*) FILTER (WHERE coalesce(l.net, 0) <> s.amount_due),
            count(*) FILTER (WHERE l.legacy_sale_id IS NULL)
        FROM app.legacy_sales s
        LEFT JOIN (SELECT legacy_sale_id, sum(net) AS net FROM app.legacy_sales_recognitions WHERE batch_id = $1
            GROUP BY legacy_sale_id) l ON l.legacy_sale_id = s.legacy_id
        WHERE s.batch_id = $1 AND s.status = 'paid'`

// planNode es un nodo de EXPLAIN (FORMAT JSON).
type planNode map[string]any

// explainPlan es el plan (sin ejecutar) de una consulta con sus argumentos.
func explainPlan(t *testing.T, querier Querier, query string, args ...any) planNode {
	t.Helper()
	var encoded []byte
	if err := querier.QueryRow(context.Background(), "EXPLAIN (FORMAT JSON) "+query, args...).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"`
	}
	if err := json.Unmarshal(encoded, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("plan: %v %s", err, encoded)
	}
	return plans[0].Plan
}

// walk recorre un nodo y sus hijos.
func (node planNode) walk(visit func(planNode)) {
	visit(node)
	children, _ := node["Plans"].([]any)
	for _, child := range children {
		if typed, ok := child.(map[string]any); ok {
			planNode(typed).walk(visit)
		}
	}
}

// joinFilters son las condiciones de unión que se evalúan fila por fila fuera de un índice, un hash o un merge (en el
// plan del defecto, «Join Filter: (legacy_sale_id = s.legacy_id)»: vuelve a recorrer el lado de adentro por cada venta).
func (node planNode) joinFilters() []string {
	filters := []string{}
	node.walk(func(current planNode) {
		if filter, found := current["Join Filter"].(string); found {
			filters = append(filters, filter)
		}
	})
	return filters
}

// filtersJoinBy dice si alguna condición de unión fuera del índice usa la columna.
func (node planNode) filtersJoinBy(column string) bool {
	for _, filter := range node.joinFilters() {
		if strings.Contains(filter, column) {
			return true
		}
	}
	return false
}

// indexConditions son las condiciones de índice de los recorridos de una tabla.
func (node planNode) indexConditions(relation string) []string {
	conditions := []string{}
	node.walk(func(current planNode) {
		if current["Relation Name"] == relation {
			if condition, found := current["Index Cond"].(string); found {
				conditions = append(conditions, condition)
			}
		}
	})
	return conditions
}

// looksUpBy dice si algún recorrido de la tabla busca por la columna en el índice.
func (node planNode) looksUpBy(relation, column string) bool {
	for _, condition := range node.indexConditions(relation) {
		if strings.Contains(condition, column) {
			return true
		}
	}
	return false
}

// estimatedRows es la estimación de filas del recorrido de una tabla (la mayor, si la recorre varias veces).
func (node planNode) estimatedRows(relation string) float64 {
	rows := 0.0
	node.walk(func(current planNode) {
		if current["Relation Name"] == relation {
			if value, _ := current["Plan Rows"].(float64); value > rows {
				rows = value
			}
		}
	})
	return rows
}

// Reemplazo con las estadísticas del lote anterior (revisión de la Fase E, D-1). Después de la primera importación el
// autovacuum analiza el historial y sus estadísticas solo conocen ese lote; dentro del reemplazo el planificador
// estimaba el lote nuevo en una fila y la validación del libro volvía a sumar el libro completo por cada venta (horas con
// el volumen real). Comprueba, sin depender del tiempo: que con esas estadísticas el plan de la validación ya no es el
// del defecto (y que el de la consulta anterior sí lo era), que dentro de la transacción del reemplazo las estadísticas
// ya son las del lote nuevo (ANALYZE ve las filas escritas y no las borradas), que la validación da OK y que, al
// confirmar, los reportes quedan con las estadísticas del lote nuevo.
func TestLegacyImportReplaceWithStaleStatistics(t *testing.T) {
	pool := testDatabase(t)
	admin, device := installation(t, pool)
	ctx := context.Background()
	pkg := standardSynthetic()
	pkg.addVolumeSales(3000)
	first, err := importSynthetic(t, pool, pkg, admin, device, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Validation.OK {
		t.Fatalf("primera importación: %+v", first.Validation)
	}
	if _, found := first.TimingsMS["analyze_history"]; !found {
		t.Fatalf("la importación debía analizar el historial: %v", first.TimingsMS)
	}
	var paid float64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM app.legacy_sales WHERE status = 'paid'").Scan(&paid); err != nil {
		t.Fatal(err)
	}
	// Las tablas con muchas filas del mismo lote (en las chicas, ANALYZE no guarda valores frecuentes).
	large := []string{"legacy_sales", "legacy_sale_lines", "legacy_line_complements", "legacy_payments", "legacy_sales_recognitions"}
	// Las estadísticas que deja el autovacuum después de la primera importación (aquí, a mano y completas: las tablas
	// son chicas, así que ANALYZE las lee enteras y el plan no depende del muestreo).
	if _, err := pool.Exec(ctx, "ANALYZE app."+strings.Join(historyTables, ", app.")); err != nil {
		t.Fatal(err)
	}
	other := uuidText(newUUID())
	if previous := explainPlan(t, pool, previousLedgerCheckSQL, other); !previous.filtersJoinBy("legacy_sale_id") {
		t.Fatalf("control: con estas estadísticas la consulta anterior debía tener el plan del defecto: %v", previous)
	}
	if current := explainPlan(t, pool, ledgerCheckSQL, other); len(current.joinFilters()) > 0 ||
		!current.looksUpBy("legacy_sales_recognitions", "legacy_sale_id") {
		t.Fatalf("la validación del libro de un lote nuevo con las estadísticas del vigente no busca el libro de cada venta por el índice: %v", current)
	}
	// Lo mismo en la lista de datos incoherentes del informe: la venta de cada fila marcada se busca por su llave.
	for _, query := range incoherentQueries {
		plan := explainPlan(t, pool, query.sql, other, listedFlags(query.entity))
		if len(plan.joinFilters()) > 0 || (strings.Contains(query.sql, "LATERAL") && !plan.looksUpBy("legacy_sales", "legacy_id")) {
			t.Fatalf("informe (%s) con las estadísticas del vigente: %v", query.entity, plan)
		}
	}

	// Dentro de la transacción del reemplazo, después de escribir el historial, las estadísticas ya son las del lote
	// nuevo: solo él en batch_id (las filas del lote borrado no cuentan) y la estimación de sus ventas pagadas cuadra.
	hooks := &Hooks{AfterHistory: func(ctx context.Context, tx pgx.Tx) error {
		var batch string
		if err := tx.QueryRow(ctx, "SELECT id::text FROM app.legacy_import_batches WHERE status = 'importing'").Scan(&batch); err != nil {
			return err
		}
		for _, table := range large {
			var values string
			if err := tx.QueryRow(ctx, `SELECT coalesce(most_common_vals::text, '') FROM pg_stats
                WHERE schemaname = 'app' AND tablename = $1 AND attname = 'batch_id'`, table).Scan(&values); err != nil {
				return fmt.Errorf("%s: %w", table, err)
			}
			if values != "{"+batch+"}" {
				return fmt.Errorf("estadísticas de %s dentro del reemplazo: %s (lote nuevo %s)", table, values, batch)
			}
		}
		plan := explainPlan(t, tx, ledgerCheckSQL, batch)
		if rows := plan.estimatedRows("legacy_sales"); rows < paid*0.9 || rows > paid*1.1 || len(plan.joinFilters()) > 0 {
			return fmt.Errorf("plan de la validación dentro del reemplazo (estima %.0f de %.0f ventas pagadas): %v", rows, paid, plan)
		}
		return nil
	}}
	second, err := importSynthetic(t, pool, pkg, admin, device, first.BatchID, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Validation.OK || second.ReplacedBatchID != first.BatchID {
		t.Fatalf("reemplazo: %+v", second.Validation)
	}
	// Al confirmar, los reportes (otra sesión) ven las estadísticas del lote nuevo, también en la correspondencia.
	for _, table := range append([]string{"legacy_id_map"}, large...) {
		got := queryText(t, pool, `SELECT coalesce(most_common_vals::text, '') FROM pg_stats
            WHERE schemaname = 'app' AND tablename = $1 AND attname = 'batch_id'`, table)
		if got != "{"+second.BatchID+"}" {
			t.Fatalf("estadísticas de %s después del reemplazo: %s (lote nuevo %s)", table, got, second.BatchID)
		}
	}
	if got := queryText(t, pool, `SELECT (reltuples::bigint = (SELECT count(*) FROM app.legacy_sales))::text FROM pg_class
        WHERE oid = 'app.legacy_sales'::regclass`); got != "true" {
		t.Fatal("el número de filas de legacy_sales en las estadísticas no es el del lote nuevo")
	}
}
