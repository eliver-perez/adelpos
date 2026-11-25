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

// Etapa K1: actualización desde el esquema 28 con roles, usuarios, terminales (también una kitchen reservada y nunca
// usada), sesiones, turnos, el cupo de cocina de una terminal en Preparación, una estación «Solo pantalla de cocina» con
// una comanda y su acuse. La sección «Fase A» de la 0029 agrega los límites de color generales (10 y 20 minutos), los
// ajustes de cada pantalla de cocina y su «cupo liberado», relaja el vínculo del cupo de cocina y el usuario del acuse
// solo para un dispositivo kitchen (con su disparador), crea kitchen_display_dismissals y dos índices. Ninguna fila de
// ninguna tabla existente cambia. Deja la base vacía.
func TestStageK1Upgrade(t *testing.T) {
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
	if last := expected[len(expected)-1].Version; last < 29 || last != product.Read().SchemaVersion {
		t.Fatalf("la última migración (%d) no coincide con el esquema del producto (%d)", last, product.Read().SchemaVersion)
	}
	schema28 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 28 {
			schema28[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema28); err != nil || n != 28 {
		t.Fatalf("no se preparó el esquema 28: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stageA1ExistingData+stageL1ExistingData+stageK1ExistingData); err != nil {
		t.Fatal(err)
	}
	tables := stageA1Tables(ctx, t, pool)
	before := map[string]string{}
	for table, columns := range tables {
		before[table] = stageA1Snapshot(ctx, t, pool, table, columns)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-28 {
		t.Fatalf("la 0029 no se aplicó sobre el esquema 28: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0029 no es repetible")
	}
	for table, columns := range tables {
		if after := stageA1Snapshot(ctx, t, pool, table, columns); after != before[table] {
			t.Fatalf("la 0029 cambió filas existentes de %s:\nantes   %s\ndespués %s", table, before[table], after)
		}
	}
	checkStageK1KitchenSettings(ctx, t, pool)
	checkStageK1DisplaySettings(ctx, t, pool)
	checkStageK1Seats(ctx, t, pool)
	checkStageK1Receipts(ctx, t, pool)
	checkStageK1Dismissals(ctx, t, pool)
}

// Una instalación en el esquema 28: además de lo de la A1 y la L1, una terminal de tipo kitchen (el tipo reservado de
// la 0023), la sesión de la caja en Preparación con su cupo de cocina (L1-D7), una estación «Solo pantalla de cocina»
// y una comanda de esa estación con su acuse de la caja.
const stageK1ExistingData = `
INSERT INTO app.devices(id,name,secret_hash,kind) VALUES
    ('00000000-0000-4000-8000-0000000002d1','Pantalla cocina','hash-k1-1','kitchen');
INSERT INTO app.sessions(id,user_id,device_id,token_hash,expires_at) VALUES
    ('00000000-0000-4000-8000-0000000002a1','00000000-0000-4000-8000-000000000010','00000000-0000-4000-8000-000000000021',
     'token-admin-k1',now()+interval '12 hours');
INSERT INTO app.license_seats(device_id,kind,session_id,acquired_at,last_activity_at) VALUES
    ('00000000-0000-4000-8000-000000000021','kitchen_display','00000000-0000-4000-8000-0000000002a1',now(),now());
INSERT INTO app.kitchen_stations(id,name,reception) VALUES
    ('00000000-0000-4000-8000-0000000002e1','Barra K1','kds');
INSERT INTO app.kitchen_tickets(id,kind,station_ids,document) VALUES
    ('00000000-0000-4000-8000-0000000002f1','normal',ARRAY['00000000-0000-4000-8000-0000000002e1']::uuid[],'{}');
INSERT INTO app.kitchen_kds_receipts(ticket_id,station_id,device_id,user_id) VALUES
    ('00000000-0000-4000-8000-0000000002f1','00000000-0000-4000-8000-0000000002e1','00000000-0000-4000-8000-000000000021',
     '00000000-0000-4000-8000-000000000010');
`

func stageK1Violates(code string, err error) bool {
	var problem *pgconn.PgError
	return errors.As(err, &problem) && problem.Code == code
}

// checkStageK1KitchenSettings: los límites de color generales nacen en 10 y 20 minutos, con sus restricciones.
func checkStageK1KitchenSettings(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var warning, late int
	if err := pool.QueryRow(ctx, "SELECT display_warning_minutes,display_late_minutes FROM app.kitchen_settings").
		Scan(&warning, &late); err != nil || warning != 10 || late != 20 {
		t.Fatalf("límites de color por omisión: %d %d %v", warning, late, err)
	}
	for name, statement := range map[string]string{
		"atención en cero":           "UPDATE app.kitchen_settings SET display_warning_minutes=0",
		"atención de más de un día":  "UPDATE app.kitchen_settings SET display_warning_minutes=1441,display_late_minutes=2000",
		"retraso igual a atención":   "UPDATE app.kitchen_settings SET display_late_minutes=10",
		"retraso antes de atención":  "UPDATE app.kitchen_settings SET display_late_minutes=5",
		"retraso de más de dos días": "UPDATE app.kitchen_settings SET display_late_minutes=2881",
	} {
		if _, err := pool.Exec(ctx, statement); !stageK1Violates("23514", err) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE app.kitchen_settings SET display_warning_minutes=1,display_late_minutes=2880"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE app.kitchen_settings SET display_warning_minutes=10,display_late_minutes=20"); err != nil {
		t.Fatal(err)
	}
}

// checkStageK1DisplaySettings: los ajustes de la pantalla nacen con sus valores por omisión en todas las terminales
// (letra normal, tema oscuro, sonido activado, volumen medio y sin «cupo liberado»), con sus listas cerradas, y solo un
// dispositivo kitchen puede quedar en «cupo liberado».
func checkStageK1DisplaySettings(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var different int
	if err := pool.QueryRow(ctx, `SELECT count(*)::int FROM app.devices WHERE kitchen_display_text_size<>'normal'
        OR kitchen_display_theme<>'dark' OR NOT kitchen_display_sound OR kitchen_display_volume<>'medium'
        OR kitchen_display_seat_released_at IS NOT NULL`).Scan(&different); err != nil || different != 0 {
		t.Fatalf("ajustes de pantalla por omisión: %d %v", different, err)
	}
	kitchenDevice := "'00000000-0000-4000-8000-0000000002d1'"
	for name, statement := range map[string]string{
		"letra desconocida":            "UPDATE app.devices SET kitchen_display_text_size='huge' WHERE id=" + kitchenDevice,
		"tema desconocido":             "UPDATE app.devices SET kitchen_display_theme='blue' WHERE id=" + kitchenDevice,
		"volumen desconocido":          "UPDATE app.devices SET kitchen_display_volume='max' WHERE id=" + kitchenDevice,
		"cupo liberado en una caja":    "UPDATE app.devices SET kitchen_display_seat_released_at=now() WHERE kind='desktop'",
		"cupo liberado en una tableta": "UPDATE app.devices SET kitchen_display_seat_released_at=now() WHERE kind='android'",
	} {
		if _, err := pool.Exec(ctx, statement); !stageK1Violates("23514", err) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE app.devices SET kitchen_display_text_size='large',kitchen_display_theme='light',
        kitchen_display_sound=false,kitchen_display_volume='high',kitchen_display_seat_released_at=now() WHERE id=`+kitchenDevice); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE app.devices SET kitchen_display_text_size='normal',kitchen_display_theme='dark',
        kitchen_display_sound=true,kitchen_display_volume='medium',kitchen_display_seat_released_at=NULL WHERE id=`+kitchenDevice); err != nil {
		t.Fatal(err)
	}
}

// checkStageK1Seats: el cupo de cocina de la caja en Preparación sigue con su sesión; una pantalla kitchen puede ocupar
// el suyo sin sesión, y ningún otro dispositivo (el disparador responde como una restricción CHECK). Los demás
// vínculos de la 0028 siguen igual.
func checkStageK1Seats(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var kept string
	if err := pool.QueryRow(ctx, `SELECT session_id::text FROM app.license_seats
        WHERE device_id='00000000-0000-4000-8000-000000000021' AND kind='kitchen_display'`).Scan(&kept); err != nil ||
		kept != "00000000-0000-4000-8000-0000000002a1" {
		t.Fatalf("cupo de cocina de la caja: %q %v", kept, err)
	}
	tablet := "'00000000-0000-4000-8000-0000000000b1'"
	desktop := "'00000000-0000-4000-8000-0000000001d1'"
	display := "'00000000-0000-4000-8000-0000000002d1'"
	session := "'00000000-0000-4000-8000-000000000031'"
	shift := "'00000000-0000-4000-8000-0000000001f1'"
	for name, statement := range map[string]string{
		"cocina sin sesión en una tableta": "INSERT INTO app.license_seats VALUES (" + tablet + ",'kitchen_display',NULL,NULL,now(),now())",
		"cocina sin sesión en una caja":    "INSERT INTO app.license_seats VALUES (" + desktop + ",'kitchen_display',NULL,NULL,now(),now())",
		"cocina con turno":                 "INSERT INTO app.license_seats VALUES (" + display + ",'kitchen_display'," + shift + ",NULL,now(),now())",
		"caja sin turno":                   "INSERT INTO app.license_seats VALUES (" + display + ",'cash_register',NULL,NULL,now(),now())",
		"comandero con sesión":             "INSERT INTO app.license_seats VALUES (" + display + ",'order_taker',NULL," + session + ",now(),now())",
	} {
		if _, err := pool.Exec(ctx, statement); !stageK1Violates("23514", err) {
			t.Fatalf("%s: se aceptó o falló por otra causa: %v", name, err)
		}
	}
	if _, err := pool.Exec(ctx, "INSERT INTO app.license_seats VALUES ("+display+",'kitchen_display',NULL,NULL,now(),now())"); err != nil {
		t.Fatalf("la pantalla de cocina sin sesión: %v", err)
	}
	// Pasar la fila de la pantalla a otro dispositivo también lo revisa el disparador.
	if _, err := pool.Exec(ctx, "UPDATE app.license_seats SET device_id="+tablet+" WHERE device_id="+display); !stageK1Violates("23514", err) {
		t.Fatalf("mover el cupo sin sesión a una tableta: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM app.license_seats WHERE device_id="+display); err != nil {
		t.Fatal(err)
	}
}

// checkStageK1Receipts: el acuse de la caja conserva su usuario; una pantalla kitchen puede confirmar sin usuario y
// ningún otro dispositivo.
func checkStageK1Receipts(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ticket := "'00000000-0000-4000-8000-0000000002f1'"
	station := "'00000000-0000-4000-8000-0000000002e1'"
	for name, device := range map[string]string{
		"una tableta": "'00000000-0000-4000-8000-0000000000b1'",
		"una caja":    "'00000000-0000-4000-8000-0000000001d1'",
	} {
		_, err := pool.Exec(ctx, "INSERT INTO app.kitchen_kds_receipts(ticket_id,station_id,device_id,user_id) VALUES ("+
			ticket+","+station+","+device+",NULL)")
		if !stageK1Violates("23514", err) {
			t.Fatalf("acuse sin usuario de %s: %v", name, err)
		}
	}
	display := "'00000000-0000-4000-8000-0000000002d1'"
	if _, err := pool.Exec(ctx, "INSERT INTO app.kitchen_kds_receipts(ticket_id,station_id,device_id,user_id) VALUES ("+
		ticket+","+station+","+display+",NULL)"); err != nil {
		t.Fatalf("acuse de la pantalla sin usuario: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM app.kitchen_kds_receipts WHERE device_id="+display); err != nil {
		t.Fatal(err)
	}
}

// checkStageK1Dismissals: la tabla de las cancelaciones vistas nace vacía, con su llave por tarea y sus llaves foráneas,
// y los dos índices nuevos existen.
func checkStageK1Dismissals(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var rows int
	if err := pool.QueryRow(ctx, "SELECT count(*)::int FROM app.kitchen_display_dismissals").Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("kitchen_display_dismissals nace vacía: %d %v", rows, err)
	}
	var foreignKeys string
	if err := pool.QueryRow(ctx, `SELECT string_agg(target.relname::text,',' ORDER BY target.relname)
        FROM pg_constraint c JOIN pg_class target ON target.oid=c.confrelid
        WHERE c.conrelid='app.kitchen_display_dismissals'::regclass AND c.contype='f'`).Scan(&foreignKeys); err != nil ||
		foreignKeys != "devices,kitchen_tasks" {
		t.Fatalf("llaves foráneas de kitchen_display_dismissals: %q %v", foreignKeys, err)
	}
	for name, index := range map[string]string{
		"kitchen_display_dismissals_pkey": "(task_id)",
		"kitchen_tasks_ready_idx":         "(station_id, ready_at) WHERE (state = 'ready'::text)",
		"kitchen_tickets_release_idx":     "(release_id) WHERE (release_id IS NOT NULL)",
	} {
		var definition string
		if err := pool.QueryRow(ctx, "SELECT coalesce((SELECT indexdef FROM pg_indexes WHERE schemaname='app' AND indexname=$1),'')",
			name).Scan(&definition); err != nil || !strings.Contains(definition, index) {
			t.Fatalf("índice %s: %q %v", name, definition, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.kitchen_display_dismissals(task_id,device_id)
        VALUES ('00000000-0000-4000-8000-0000000002ff','00000000-0000-4000-8000-0000000002d1')`); !stageK1Violates("23503", err) {
		t.Fatalf("una cancelación vista sin tarea: %v", err)
	}
}
