//go:build integration

package legacyimport

import (
	"context"
	"testing"
)

// Etapa M1 (Fase A, §3 y §7; M1-D5): la importación crea las mesas divisibles del anterior en lo vivo («Mesa 3» con 3a,
// 3b y 3c, que toma el nombre de la mesa «Mesa 3»; «Mesa 7» con 7a a 7d), deja las dudosas como mesas normales, la
// validación cuadra (la mesa que se volvió grupo no se cuenta) y la venta de la «Mesa 3» conserva su nombre en el
// historial. Una segunda importación vuelve a calcular los grupos del anterior y conserva los del propietario.
func TestStageM1LegacyImportTableGroups(t *testing.T) {
	pool := testDatabase(t)
	admin, device := installation(t, pool)
	ctx := context.Background()
	report, err := importSynthetic(t, pool, stageM1Package(), admin, device, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Validation.OK {
		t.Fatalf("la validación no cuadra: %+v", report.Validation)
	}
	if report.Summary.Counts["table_groups"] != 2 {
		t.Fatalf("conteo de grupos: %v", report.Summary.Counts)
	}
	groups := `SELECT coalesce(string_agg(g.name || ':' || (SELECT string_agg(t.name, ',' ORDER BY s.position)
            FROM app.table_group_sections s JOIN app.restaurant_tables t ON t.id = s.table_id WHERE s.group_id = g.id), ';'
            ORDER BY g.name), '') FROM app.table_groups g WHERE g.active AND g.source = 'legacy'`
	if got := queryText(t, pool, groups); got != "Mesa 3:3a,3b,3c;Mesa 7:Mesa 7a,Mesa 7b,Mesa 7c,Mesa 7d" {
		t.Fatalf("grupos importados: %s", got)
	}
	if got := queryText(t, pool, `SELECT count(*)::text FROM app.restaurant_tables WHERE name IN ('Mesa 3', 'Mesa 7')`); got != "0" {
		t.Fatalf("la mesa que se volvió grupo existe como mesa: %s", got)
	}
	if got := queryText(t, pool, `SELECT count(*)::text FROM app.restaurant_tables WHERE name IN ('Mesa 12a','Mesa 12c','Mesa 15b',
        'Mesa 20a','Mesa 20b','Mesa 20','20')`); got != "7" {
		t.Fatalf("las dudosas no quedaron como mesas: %s", got)
	}
	if got := queryText(t, pool, `SELECT count(*)::text FROM app.table_groups WHERE legacy_batch_id IS NULL AND source='legacy'`); got != "0" {
		t.Fatalf("grupos sin lote: %s", got)
	}
	if got := queryText(t, pool, `SELECT coalesce(string_agg(DISTINCT coalesce(table_name, '') || '/' || (table_id IS NULL)::text, ','), '')
        FROM app.legacy_sales WHERE legacy_table_id = 12`); got != "Mesa 3/true" {
		t.Fatalf("la venta de la «Mesa 3»: %s", got)
	}

	// El propietario arma un grupo a mano con dos mesas importadas y vuelve a importar el mismo paquete.
	var manual string
	if err := pool.QueryRow(ctx, `WITH area AS (SELECT area_id FROM app.restaurant_tables WHERE name = 'Mesa 12a'),
            created AS (INSERT INTO app.table_groups(id, area_id, name) SELECT gen_random_uuid(), area_id, 'Mesa 12' FROM area
                RETURNING id)
        SELECT id::text FROM created`).Scan(&manual); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.table_group_sections(group_id, table_id, position)
        SELECT $1, id, row_number() OVER (ORDER BY name) - 1 FROM app.restaurant_tables WHERE name IN ('Mesa 12a', 'Mesa 12c')`,
		manual); err != nil {
		t.Fatal(err)
	}
	second, err := importSynthetic(t, pool, stageM1Package(), admin, device, report.BatchID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Validation.OK {
		t.Fatalf("la segunda importación no cuadra: %+v", second.Validation)
	}
	if got := queryText(t, pool, groups); got != "Mesa 3:3a,3b,3c;Mesa 7:Mesa 7a,Mesa 7b,Mesa 7c,Mesa 7d" {
		t.Fatalf("grupos de la segunda importación: %s", got)
	}
	if got := queryText(t, pool, `SELECT count(*)::text FROM app.table_groups WHERE source='legacy' AND NOT active`); got != "2" {
		t.Fatalf("los grupos del lote anterior: %s", got)
	}
	if got := queryText(t, pool, `SELECT active::text || '/' || (SELECT count(*) FROM app.table_group_sections WHERE group_id = $1)::text
        FROM app.table_groups WHERE id = $1`, manual); got != "true/2" {
		t.Fatalf("el grupo del propietario: %s", got)
	}
}
