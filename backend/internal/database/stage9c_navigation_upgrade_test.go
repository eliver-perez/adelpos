//go:build integration

package database

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/product"
	"local/ad-el-pos/server/migrations"
)

// Etapa 9C (fase A): actualización desde el esquema 19 con terminales e impresoras existentes. La
// 0020 agrega la pantalla de inicio de cada terminal (Mesas por omisión, lista cerrada con CHECK con
// nombre) y las columnas por impresora (NULL = el valor predeterminado del papel, de modo que las
// impresoras existentes imprimen igual). No cambia los demás datos de las terminales. Deja la base
// vacía al terminar.
func TestStage9CTerminalHomeUpgrade(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(u.Path, "/adel_pos_test_") {
		t.Fatal("integration requires an isolated adel_pos_test_ database")
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
	if last := expected[len(expected)-1].Version; last != product.Read().SchemaVersion {
		t.Fatalf("la última migración (%d) no coincide con el esquema del producto (%d)", last, product.Read().SchemaVersion)
	}
	schema19 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 19 {
			schema19[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema19); err != nil || n != 19 {
		t.Fatalf("no se preparó el esquema 19: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.registers(id,name) VALUES ('00000000-0000-4000-8000-000000000011','Caja 1');
        INSERT INTO app.devices(id,name,secret_hash,register_id,keyboard_mode) VALUES
            ('00000000-0000-4000-8000-000000000021','Terminal de barra','hash-1','00000000-0000-4000-8000-000000000011','always'),
            ('00000000-0000-4000-8000-000000000022','Terminal de cocina','hash-2',NULL,'disabled');
        INSERT INTO app.kitchen_printers(id,name,transport,destination,paper_width) VALUES
            ('00000000-0000-4000-8000-000000000031','Tickets','cups','COLA',80),
            ('00000000-0000-4000-8000-000000000032','Comandas','cups','COLA_58',58)`); err != nil {
		t.Fatal(err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-19 {
		t.Fatalf("la migración de la etapa 9C no se aplicó sobre el esquema 19: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la migración de la etapa 9C no es repetible")
	}
	checkStage9CExistingDevices(ctx, t, pool)
	checkStage9CHomeConstraint(ctx, t, pool)
	checkStage9CPrinterColumns(ctx, t, pool)
}

// Las terminales existentes quedan en Mesas y conservan su caja, su teclado y su nombre.
func checkStage9CExistingDevices(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var tables, preserved int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE home_screen='tables'),
        count(*) FILTER (WHERE (name='Terminal de barra' AND register_id='00000000-0000-4000-8000-000000000011' AND keyboard_mode='always')
            OR (name='Terminal de cocina' AND register_id IS NULL AND keyboard_mode='disabled'))
        FROM app.devices`).Scan(&tables, &preserved); err != nil {
		t.Fatal(err)
	}
	if tables != 2 || preserved != 2 {
		t.Fatalf("las terminales existentes cambiaron: %d en Mesas y %d intactas", tables, preserved)
	}
	var columnDefault string
	if err := pool.QueryRow(ctx, `SELECT column_default FROM information_schema.columns
        WHERE table_schema='app' AND table_name='devices' AND column_name='home_screen' AND is_nullable='NO'`).Scan(&columnDefault); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(columnDefault, "'tables'") {
		t.Fatalf("la pantalla de inicio debía ser Mesas por omisión: %s", columnDefault)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO app.devices(id,name,secret_hash) VALUES ('00000000-0000-4000-8000-000000000023','Terminal nueva','hash-3')"); err != nil {
		t.Fatal(err)
	}
	var created string
	if err := pool.QueryRow(ctx, "SELECT home_screen FROM app.devices WHERE id='00000000-0000-4000-8000-000000000023'").Scan(&created); err != nil || created != "tables" {
		t.Fatalf("una terminal nueva debía iniciar en Mesas: %q %v", created, err)
	}
}

// La lista cerrada vive en un CHECK con nombre que rechaza «Estado del sistema», la administración y
// el valor nulo.
func checkStage9CHomeConstraint(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var named int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint
        WHERE conname='devices_home_screen_check' AND conrelid='app.devices'::regclass AND contype='c'`).Scan(&named); err != nil || named != 1 {
		t.Fatalf("falta el CHECK con nombre de la pantalla de inicio: %d %v", named, err)
	}
	for _, accepted := range []string{"tables", "takeaway", "kitchen", "cash"} {
		if _, err := pool.Exec(ctx, "UPDATE app.devices SET home_screen=$1 WHERE id='00000000-0000-4000-8000-000000000021'", accepted); err != nil {
			t.Fatalf("no se aceptó %q: %v", accepted, err)
		}
	}
	for _, rejected := range []any{"status", "administration", "restaurant", "", nil} {
		_, err := pool.Exec(ctx, "UPDATE app.devices SET home_screen=$1 WHERE id='00000000-0000-4000-8000-000000000021'", rejected)
		if err == nil {
			t.Fatalf("la base aceptó %v como pantalla de inicio", rejected)
		}
	}
}

// Las impresoras existentes quedan con el valor predeterminado de su papel (NULL) y el CHECK con
// nombre solo admite la alternativa de cada papel.
func checkStage9CPrinterColumns(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var defaults, named int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.kitchen_printers WHERE columns IS NULL),
        (SELECT count(*) FROM pg_constraint WHERE conname='kitchen_printers_columns_check' AND conrelid='app.kitchen_printers'::regclass)`).Scan(&defaults, &named); err != nil {
		t.Fatal(err)
	}
	if defaults != 2 || named != 1 {
		t.Fatalf("las impresoras existentes cambiaron de columnas o falta el CHECK: %d %d", defaults, named)
	}
	cases := []struct {
		id      string
		columns int
		valid   bool
	}{
		{"00000000-0000-4000-8000-000000000031", 48, true},
		{"00000000-0000-4000-8000-000000000031", 42, true},
		{"00000000-0000-4000-8000-000000000031", 32, false},
		{"00000000-0000-4000-8000-000000000032", 32, true},
		{"00000000-0000-4000-8000-000000000032", 48, false},
	}
	for _, entry := range cases {
		_, err := pool.Exec(ctx, "UPDATE app.kitchen_printers SET columns=$1 WHERE id=$2", entry.columns, entry.id)
		if (err == nil) != entry.valid {
			t.Fatalf("columnas %d en %s: se esperaba válido=%v, error %v", entry.columns, entry.id, entry.valid, err)
		}
	}
}
