//go:build integration

package database

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/product"
	"local/ad-el-pos/server/migrations"
)

// Etapa 9H: actualización desde el esquema 23 con usuarios, terminales y sesiones existentes. La sección A de la 0024
// cambia el índice único de las sesiones vigentes de «uno por usuario» a «uno por usuario y terminal» (§2.4): ninguna
// sesión existente se cierra ni cambia, y desde entonces el mismo usuario puede tener una sesión vigente en cada
// terminal, pero nunca dos en la misma. La sección C crea el historial importado del sistema anterior (tablas
// legacy_* protegidas por su disparador) y las corridas de la migración, sin cambiar ninguna fila existente. Deja la
// base vacía.
func TestStage9HUpgrade(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(u.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("pool creation failed")
	}
	defer pool.Close()
	reset := func() {
		if _, err := pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
			t.Fatal(err)
		}
	}
	reset()
	defer reset()
	expected, err := ReadMigrations(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if last := expected[len(expected)-1].Version; last < 24 || last != product.Read().SchemaVersion {
		t.Fatalf("la última migración (%d) no coincide con el esquema del producto (%d)", last, product.Read().SchemaVersion)
	}
	schema23 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 23 {
			schema23[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema23); err != nil || n != 23 {
		t.Fatalf("no se preparó el esquema 23: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stage9HExistingSessions); err != nil {
		t.Fatal(err)
	}
	// Con el esquema 23, el mismo usuario no podía tener una segunda sesión vigente en otra terminal.
	if err := insertStage9HSession(ctx, pool, "cajera", "Terminal 2", "token-esquema-23"); !uniqueViolation(err) {
		t.Fatalf("el esquema 23 debía impedir dos sesiones vigentes del mismo usuario: %v", err)
	}
	before := stage9HSessionsSnapshot(ctx, t, pool)
	rowsBefore := stage9HRowCounts(ctx, t, pool)
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-23 {
		t.Fatalf("la 0024 no se aplicó sobre el esquema 23: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0024 no es repetible")
	}
	if after := stage9HSessionsSnapshot(ctx, t, pool); after != before {
		t.Fatalf("la 0024 cambió sesiones existentes: antes %s, después %s", before, after)
	}
	if after := stage9HRowCounts(ctx, t, pool); after != rowsBefore {
		t.Fatalf("la 0024 cambió filas existentes: antes %s, después %s", rowsBefore, after)
	}
	checkStage9HSessions(ctx, t, pool)
	checkStage9HLegacyHistory(ctx, t, pool)
}

// stage9HLegacyTables son las tablas de la sección C de la 0024 (historial importado del sistema anterior, protegidas
// por el disparador) y legacy_import_runs (las corridas, que no son historial).
var stage9HLegacyTables = []string{"legacy_import_batches", "legacy_id_map", "legacy_sales", "legacy_sale_lines",
	"legacy_line_complements", "legacy_quantity_changes", "legacy_discounts", "legacy_cancellations", "legacy_payments",
	"legacy_cash_shifts", "legacy_cash_withdrawals", "legacy_cash_expenses", "legacy_price_history",
	"legacy_sales_recognitions"}

// stage9HRowCounts cuenta las filas de todas las tablas del esquema app que existían en el esquema 23: la 0024 no debe
// cambiar ninguna. La prueba migra hasta la última, así que excluye lo que agrega la 0025 (etapa A1: sus tablas
// pin_attempts y offline_order_credentials, el permiso waiter_devices.use y sus concesiones), que se comprueba en
// stagea1_upgrade_test.go, la tabla ready_notices de la 0026 (etapa A2, parte D), que se comprueba en
// stagea2_upgrade_test.go, y lo de la 0028 (etapa L1: las tablas license_revisions y license_trusted_time de la sección
// «Fase A (revisión R1)», el permiso license.manage de la «Fase B» y la tabla license_seats de la «Fase D»), que se
// comprueba en stagel1_upgrade_test.go.
func stage9HRowCounts(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = 'app'
        AND table_type = 'BASE TABLE' AND table_name NOT LIKE 'legacy\_%'
        AND table_name NOT IN ('pin_attempts', 'offline_order_credentials', 'ready_notices', 'license_seats', 'license_revisions',
            'license_trusted_time')
        ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	rows.Close()
	parts := []string{}
	for _, name := range names {
		filter := ""
		switch name {
		case "permissions":
			filter = " WHERE key NOT IN ('waiter_devices.use','license.manage')"
		case "role_permissions":
			filter = " WHERE permission_key NOT IN ('waiter_devices.use','license.manage')"
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM app."+name+filter).Scan(&count); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, fmt.Sprintf("%s=%d", name, count))
	}
	return strings.Join(parts, ",")
}

// checkStage9HLegacyHistory comprueba la sección C de la 0024: las tablas del historial y de las corridas, el
// disparador por sentencia en cada tabla del historial que rechaza escribir fuera de la importación (también con una
// marca que no corresponde a un lote en curso), las restricciones de lotes y corridas y que ninguna tabla operativa
// tiene llaves hacia el historial.
func checkStage9HLegacyHistory(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, table := range append(append([]string{}, stage9HLegacyTables...), "legacy_import_runs") {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass('app.' || $1) IS NOT NULL", table).Scan(&exists); err != nil || !exists {
			t.Fatalf("falta la tabla %s: %v", table, err)
		}
	}
	for _, table := range stage9HLegacyTables {
		var triggers int
		err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_trigger tg JOIN pg_class c ON c.oid = tg.tgrelid
            JOIN pg_namespace n ON n.oid = c.relnamespace JOIN pg_proc p ON p.oid = tg.tgfoid
            WHERE n.nspname = 'app' AND c.relname = $1 AND p.proname = 'legacy_history_guard' AND NOT tg.tgisinternal`, table).Scan(&triggers)
		if err != nil || triggers != 1 {
			t.Fatalf("%s sin su disparador de protección: %d %v", table, triggers, err)
		}
	}
	var references int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint k JOIN pg_class target ON target.oid = k.confrelid
        JOIN pg_class source ON source.oid = k.conrelid
        WHERE k.contype = 'f' AND target.relname LIKE 'legacy\_%' AND source.relname NOT LIKE 'legacy\_%'`).Scan(&references); err != nil || references != 0 {
		t.Fatalf("tablas operativas con llaves hacia el historial: %d %v", references, err)
	}
	batch := "00000000-0000-4000-8000-0000000000c1"
	rejected := map[string]string{
		"lote sin marca": `INSERT INTO app.legacy_import_batches(id, status, package_fingerprint, format_version, timezone, started_at)
            VALUES ('` + batch + `', 'importing', repeat('a', 64), 1, 'UTC', now())`,
		"historial sin lote en curso": `SELECT set_config('adel.legacy_import', '` + batch + `', true);
            INSERT INTO app.legacy_cash_expenses(batch_id, position, amount) VALUES ('` + batch + `', 1, 1)`,
		"vaciar el libro":  `TRUNCATE app.legacy_sales_recognitions`,
		"marca inválida":   `SELECT set_config('adel.legacy_import', 'no-es-un-lote', true); DELETE FROM app.legacy_sales`,
		"corrida sin dato": `INSERT INTO app.legacy_import_runs(id, kind, status, upload_id, package_sha256, package_bytes, requested_by, requested_device) VALUES (gen_random_uuid(), 'analyze', 'succeeded', gen_random_uuid(), repeat('a', 64), 1, '00000000-0000-4000-8000-000000000010', '00000000-0000-4000-8000-000000000021')`,
	}
	for name, statement := range rejected {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, statement)
		_ = tx.Rollback(ctx)
		if err == nil {
			t.Fatalf("la 0024 aceptó: %s", name)
		}
	}
	// Dentro de una importación (marca del lote en curso) sí se escribe, y dos lotes en curso no pueden existir.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	statements := []string{
		`SELECT set_config('adel.legacy_import', '` + batch + `', true)`,
		`INSERT INTO app.legacy_import_batches(id, status, package_fingerprint, format_version, timezone, started_at)
            VALUES ('` + batch + `', 'importing', repeat('a', 64), 1, 'UTC', now())`,
		`INSERT INTO app.legacy_cash_expenses(batch_id, position, amount) VALUES ('` + batch + `', 1, 12.3456)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("la importación no pudo escribir: %v", err)
		}
	}
	var amount string
	if err := tx.QueryRow(ctx, "SELECT amount::text FROM app.legacy_cash_expenses").Scan(&amount); err != nil || amount != "12.3456" {
		t.Fatalf("importe exacto: %s %v", amount, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO app.legacy_import_batches(id, status, package_fingerprint, format_version, timezone, started_at)
        VALUES (gen_random_uuid(), 'importing', repeat('b', 64), 1, 'UTC', now())`); !uniqueViolation(err) {
		t.Fatalf("dos lotes en curso: %v", err)
	}
}

// Un administrador y una cajera, dos terminales y sus sesiones: la cajera con una vigente en la terminal 1 y una ya
// cerrada en la 2, y el administrador con una vigente en la terminal 1.
const stage9HExistingSessions = `
INSERT INTO app.roles(id,code,name) VALUES ('00000000-0000-4000-8000-000000000001','administrator','Administrador'),
    ('00000000-0000-4000-8000-000000000002','supervisor','Supervisor');
INSERT INTO app.users(id,username,display_name,role_id) VALUES
    ('00000000-0000-4000-8000-000000000010','admin','Administrador','00000000-0000-4000-8000-000000000001'),
    ('00000000-0000-4000-8000-000000000011','cajera','Cajera','00000000-0000-4000-8000-000000000002');
INSERT INTO app.devices(id,name,secret_hash) VALUES
    ('00000000-0000-4000-8000-000000000021','Terminal 1','hash-9h-1'),
    ('00000000-0000-4000-8000-000000000022','Terminal 2','hash-9h-2');
INSERT INTO app.sessions(id,user_id,device_id,token_hash,expires_at,revoked_at,revoke_reason) VALUES
    ('00000000-0000-4000-8000-000000000031','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000021',
     'token-cajera-1',now()+interval '12 hours',NULL,NULL),
    ('00000000-0000-4000-8000-000000000032','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022',
     'token-cajera-2',now()+interval '12 hours',now()-interval '1 hour','replaced'),
    ('00000000-0000-4000-8000-000000000033','00000000-0000-4000-8000-000000000010','00000000-0000-4000-8000-000000000021',
     'token-admin-1',now()+interval '12 hours',NULL,NULL);
`

// insertStage9HSession agrega una sesión vigente del usuario en la terminal indicada.
func insertStage9HSession(ctx context.Context, pool *pgxpool.Pool, username, device, token string) error {
	_, err := pool.Exec(ctx, `INSERT INTO app.sessions(user_id,device_id,token_hash,expires_at)
        SELECT u.id,d.id,$3,now()+interval '12 hours' FROM app.users u, app.devices d WHERE u.username=$1 AND d.name=$2`,
		username, device, token)
	return err
}

func uniqueViolation(err error) bool {
	var problem *pgconn.PgError
	return errors.As(err, &problem) && problem.Code == "23505"
}

// stage9HSessionsSnapshot resume las sesiones existentes, que la 0024 no debe tocar.
func stage9HSessionsSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT coalesce(string_agg(concat_ws('/',id,user_id,device_id,token_hash,expires_at,revoked_at,
            coalesce(revoke_reason,'')),' | ' ORDER BY id),'') FROM app.sessions`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// checkStage9HSessions comprueba la sección A de la 0024: el índice nuevo por usuario y terminal en lugar del anterior,
// una sesión vigente del mismo usuario en otra terminal permitida y una segunda en la misma terminal rechazada.
func checkStage9HSessions(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var old int
	var definition string
	err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM pg_indexes WHERE schemaname='app' AND indexname='sessions_one_active_user_idx'),
        coalesce((SELECT indexdef FROM pg_indexes WHERE schemaname='app' AND indexname='sessions_one_active_user_device_idx'),'')`).
		Scan(&old, &definition)
	if err != nil {
		t.Fatal(err)
	}
	if old != 0 || !strings.Contains(definition, "CREATE UNIQUE INDEX") || !strings.Contains(definition, "(user_id, device_id)") ||
		!strings.Contains(definition, "WHERE (revoked_at IS NULL)") {
		t.Fatalf("índices de sesiones después de la 0024: anterior %d, nuevo %q", old, definition)
	}
	if err := insertStage9HSession(ctx, pool, "cajera", "Terminal 2", "token-cajera-2b"); err != nil {
		t.Fatalf("el mismo usuario debía poder tener una sesión vigente en otra terminal: %v", err)
	}
	if err := insertStage9HSession(ctx, pool, "cajera", "Terminal 1", "token-cajera-1b"); !uniqueViolation(err) {
		t.Fatalf("dos sesiones vigentes del mismo usuario en la misma terminal debían rechazarse: %v", err)
	}
	// Una sesión cerrada no cuenta: la terminal 2 ya tenía la de antes, cerrada, y la nueva vigente.
	var active, total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE revoked_at IS NULL),count(*) FROM app.sessions s JOIN app.users u ON u.id=s.user_id
        WHERE u.username='cajera'`).Scan(&active, &total); err != nil {
		t.Fatal(err)
	}
	if active != 2 || total != 3 {
		t.Fatalf("sesiones de la cajera: %d vigentes de %d", active, total)
	}
}
