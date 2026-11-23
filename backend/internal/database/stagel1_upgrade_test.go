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

// Etapa L1: actualización desde el esquema 27 con roles, usuarios, terminales, sesiones y turnos de caja abiertos, en
// arqueo y cerrados. La sección «Fase D» de la 0028 crea app.license_seats con sus restricciones e índices y ocupa el
// cupo de caja de la terminal que abrió cada turno activo (L1-D3), y ninguno más. Ninguna fila de ninguna tabla
// existente cambia. Deja la base vacía.
func TestStageL1Upgrade(t *testing.T) {
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
	if last := expected[len(expected)-1].Version; last < 28 || last != product.Read().SchemaVersion {
		t.Fatalf("la última migración (%d) no coincide con el esquema del producto (%d)", last, product.Read().SchemaVersion)
	}
	schema27 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 27 {
			schema27[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema27); err != nil || n != 27 {
		t.Fatalf("no se preparó el esquema 27: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stageA1ExistingData+stageL1ExistingData); err != nil {
		t.Fatal(err)
	}
	tables := stageA1Tables(ctx, t, pool)
	before := map[string]string{}
	for table, columns := range tables {
		before[table] = stageA1Snapshot(ctx, t, pool, table, columns)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-27 {
		t.Fatalf("la 0028 no se aplicó sobre el esquema 27: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0028 no es repetible")
	}
	for table, columns := range tables {
		if after := stageA1Snapshot(ctx, t, pool, table, columns); after != before[table] {
			t.Fatalf("la 0028 cambió filas existentes de %s:\nantes   %s\ndespués %s", table, before[table], after)
		}
	}
	checkStageL1Seats(ctx, t, pool)
}

// Una instalación en el esquema 27: dos cajas físicas con su terminal; el turno de la Caja 1 abierto por su terminal,
// el de la Caja 2 en arqueo de cierre y uno anterior de la Caja 2 ya cerrado (abierto por la tableta, que nunca abre
// turnos, para comprobar que un turno cerrado no ocupa nada).
const stageL1ExistingData = `
INSERT INTO app.registers(id,name) VALUES
    ('00000000-0000-4000-8000-0000000001a1','Caja 1 L1'),
    ('00000000-0000-4000-8000-0000000001a2','Caja 2 L1');
INSERT INTO app.devices(id,name,secret_hash,kind,register_id) VALUES
    ('00000000-0000-4000-8000-0000000001d1','Terminal caja 2','hash-l1-1','desktop','00000000-0000-4000-8000-0000000001a2');
UPDATE app.devices SET register_id='00000000-0000-4000-8000-0000000001a1' WHERE id='00000000-0000-4000-8000-000000000021';
INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response) VALUES
    ('00000000-0000-4000-8000-0000000001e1','00000000-0000-4000-8000-000000000010','00000000-0000-4000-8000-000000000021',
     'POST payments/shifts','l1-1','{}'),
    ('00000000-0000-4000-8000-0000000001e2','00000000-0000-4000-8000-000000000010','00000000-0000-4000-8000-0000000001d1',
     'POST payments/shifts','l1-2','{}'),
    ('00000000-0000-4000-8000-0000000001e3','00000000-0000-4000-8000-000000000010','00000000-0000-4000-8000-0000000000b1',
     'POST payments/shifts','l1-3','{}');
INSERT INTO app.cash_shifts(id,register_id,opening_float,opened_by,opened_device_id,operation_id,opened_at,state,
    closed_at,closed_by,closed_device_id,closing_started_by,closing_started_at,closing_device_id,closing_movement_count,
    closing_operation_count) VALUES
    ('00000000-0000-4000-8000-0000000001f3','00000000-0000-4000-8000-0000000001a2',0,'00000000-0000-4000-8000-000000000010',
     '00000000-0000-4000-8000-0000000000b1','00000000-0000-4000-8000-0000000001e3',now()-interval '2 days','closed',
     now()-interval '1 day','00000000-0000-4000-8000-000000000010','00000000-0000-4000-8000-0000000001d1',
     '00000000-0000-4000-8000-000000000010',now()-interval '25 hours','00000000-0000-4000-8000-0000000001d1',0,0),
    ('00000000-0000-4000-8000-0000000001f1','00000000-0000-4000-8000-0000000001a1',500,'00000000-0000-4000-8000-000000000010',
     '00000000-0000-4000-8000-000000000021','00000000-0000-4000-8000-0000000001e1',now()-interval '3 hours','open',
     NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL),
    ('00000000-0000-4000-8000-0000000001f2','00000000-0000-4000-8000-0000000001a2',0,'00000000-0000-4000-8000-000000000010',
     '00000000-0000-4000-8000-0000000001d1','00000000-0000-4000-8000-0000000001e2',now()-interval '5 hours','closing',
     NULL,NULL,NULL,'00000000-0000-4000-8000-000000000010',now()-interval '10 minutes','00000000-0000-4000-8000-0000000001d1',0,0);
`

// checkStageL1Seats comprueba la sección «Fase D»: las dos filas de caja de los turnos activos (con la hora de apertura)
// y ninguna del turno cerrado; la restricción de cada tipo con su vínculo; la llave primaria por terminal y tipo; los
// índices parciales; y el borrado en cascada con el turno.
func checkStageL1Seats(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT seat.device_id::text || '|' || seat.kind || '|' || coalesce(seat.shift_id::text,'') || '|' ||
            (seat.session_id IS NULL)::text || '|' || (seat.acquired_at = shift.opened_at AND seat.last_activity_at = shift.opened_at)::text
        FROM app.license_seats seat LEFT JOIN app.cash_shifts shift ON shift.id = seat.shift_id ORDER BY seat.device_id`)
	if err != nil {
		t.Fatal(err)
	}
	seats := []string{}
	for rows.Next() {
		var seat string
		if err := rows.Scan(&seat); err != nil {
			t.Fatal(err)
		}
		seats = append(seats, seat)
	}
	rows.Close()
	expected := []string{
		"00000000-0000-4000-8000-000000000021|cash_register|00000000-0000-4000-8000-0000000001f1|true|true",
		"00000000-0000-4000-8000-0000000001d1|cash_register|00000000-0000-4000-8000-0000000001f2|true|true",
	}
	if strings.Join(seats, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("cupos de caja de los turnos activos:\n%s", strings.Join(seats, "\n"))
	}
	for name, index := range map[string]string{
		"license_seats_shift_idx":   "(shift_id) WHERE (shift_id IS NOT NULL)",
		"license_seats_session_idx": "(session_id) WHERE (session_id IS NOT NULL)",
		"license_seats_pkey":        "(device_id, kind)",
	} {
		var definition string
		if err := pool.QueryRow(ctx, "SELECT coalesce((SELECT indexdef FROM pg_indexes WHERE schemaname='app' AND indexname=$1),'')",
			name).Scan(&definition); err != nil || !strings.Contains(definition, index) {
			t.Fatalf("índice %s: %q %v", name, definition, err)
		}
	}
	violates := func(code string, err error) bool {
		var problem *pgconn.PgError
		return errors.As(err, &problem) && problem.Code == code
	}
	tablet := "'00000000-0000-4000-8000-0000000000b1'"
	session := "'00000000-0000-4000-8000-000000000031'"
	shift := "'00000000-0000-4000-8000-0000000001f1'"
	for name, statement := range map[string]string{
		"tipo desconocido":             "INSERT INTO app.license_seats VALUES (" + tablet + ",'kds',NULL,NULL,now(),now())",
		"caja sin turno":               "INSERT INTO app.license_seats VALUES (" + tablet + ",'cash_register',NULL,NULL,now(),now())",
		"caja con sesión":              "INSERT INTO app.license_seats VALUES (" + tablet + ",'cash_register'," + shift + "," + session + ",now(),now())",
		"comandero con turno":          "INSERT INTO app.license_seats VALUES (" + tablet + ",'order_taker'," + shift + ",NULL,now(),now())",
		"comandero con sesión":         "INSERT INTO app.license_seats VALUES (" + tablet + ",'order_taker',NULL," + session + ",now(),now())",
		"cocina sin sesión":            "INSERT INTO app.license_seats VALUES (" + tablet + ",'kitchen_display',NULL,NULL,now(),now())",
		"cocina con turno":             "INSERT INTO app.license_seats VALUES (" + tablet + ",'kitchen_display'," + shift + "," + session + ",now(),now())",
		"sin hora de ocupación":        "INSERT INTO app.license_seats VALUES (" + tablet + ",'order_taker',NULL,NULL,NULL,now())",
		"sin hora de última actividad": "INSERT INTO app.license_seats VALUES (" + tablet + ",'order_taker',NULL,NULL,now(),NULL)",
	} {
		_, err := pool.Exec(ctx, statement)
		if !violates("23514", err) && !violates("23502", err) {
			t.Fatalf("%s: se aceptó o falló por otra causa: %v", name, err)
		}
	}
	// Los tres tipos válidos en la misma terminal; un segundo comandero de la misma terminal viola la llave primaria.
	_, err = pool.Exec(ctx, `INSERT INTO app.license_seats VALUES
        (`+tablet+`,'order_taker',NULL,NULL,now(),now()),
        (`+tablet+`,'kitchen_display',NULL,`+session+`,now(),now()),
        (`+tablet+`,'cash_register',`+shift+`,NULL,now(),now())`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO app.license_seats VALUES ("+tablet+",'order_taker',NULL,NULL,now(),now())"); !violates("23505", err) {
		t.Fatalf("dos comanderos en la misma terminal: %v", err)
	}
	// Las filas se van con su sesión o con su turno (también al quitar datos de prueba).
	if _, err := pool.Exec(ctx, "DELETE FROM app.sessions WHERE id="+session); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, "SELECT count(*)::int FROM app.license_seats WHERE device_id="+tablet).Scan(&remaining); err != nil || remaining != 2 {
		t.Fatalf("filas de la tableta después de borrar su sesión: %d %v", remaining, err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM app.license_seats WHERE device_id="+tablet); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM app.cash_shifts WHERE id="+shift); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*)::int FROM app.license_seats WHERE shift_id="+shift).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("filas del turno borrado: %d %v", remaining, err)
	}
}
