//go:build integration

package database

import (
	"context"
	"errors"
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
// terminal, pero nunca dos en la misma. Las fases siguientes agregan aquí sus propias comprobaciones. Deja la base vacía.
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
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-23 {
		t.Fatalf("la 0024 no se aplicó sobre el esquema 23: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0024 no es repetible")
	}
	if after := stage9HSessionsSnapshot(ctx, t, pool); after != before {
		t.Fatalf("la 0024 cambió sesiones existentes: antes %s, después %s", before, after)
	}
	checkStage9HSessions(ctx, t, pool)
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
