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

// Etapa 9F (fase B): actualización desde el esquema 21 con impresoras, trabajos de impresión y la
// configuración de cobro existentes. La 0022 agrega el tipo de corte y los renglones de avance por
// impresora (las existentes conservan su papel: corte total y, si ya cortaban, sin renglones de avance,
// que es el final de siempre; las que no cortaban y las nuevas reciben 4) y el nombre visible de la
// precuenta («Precuenta» por omisión), con CHECK con nombre.
// No cambia impresoras, trabajos ni sus fotografías, ni la configuración de cobro. Deja la base vacía.
func TestStage9FPrintingUpgrade(t *testing.T) {
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
	if last := expected[len(expected)-1].Version; last != 22 || product.Read().SchemaVersion != 22 {
		t.Fatalf("se esperaba el esquema 22 en la última migración (%d) y en el producto (%d)", last, product.Read().SchemaVersion)
	}
	schema21 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 21 {
			schema21[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema21); err != nil || n != 21 {
		t.Fatalf("no se preparó el esquema 21: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stage9FExistingPrinting); err != nil {
		t.Fatal(err)
	}
	before := stage9FSnapshot(ctx, t, pool)
	if n, err := Migrate(ctx, pool); err != nil || n != 1 {
		t.Fatalf("la 0022 no se aplicó sobre el esquema 21: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0022 no es repetible")
	}
	if after := stage9FSnapshot(ctx, t, pool); after != before {
		t.Fatalf("la 0022 cambió datos existentes: antes %s, después %s", before, after)
	}
	checkStage9FPaperCut(ctx, t, pool)
	checkStage9FPrecheckName(ctx, t, pool)
}

// Dos impresoras de 80 y 58 mm (una con corte), un trabajo con su fotografía anterior a la 9F y la
// configuración de cobro con la impresora de tickets.
const stage9FExistingPrinting = `
INSERT INTO app.kitchen_printers(id,name,transport,destination,paper_width,supports_cut,cut) VALUES
    ('00000000-0000-4000-8000-000000000041','Tickets','cups','COLA_TICKETS',80,true,true),
    ('00000000-0000-4000-8000-000000000042','Comandas 58','escpos_tcp','192.168.1.60',58,false,false);
INSERT INTO app.kitchen_tickets(id,kind,station_ids,document) VALUES
    ('00000000-0000-4000-8000-000000000051','test','{}','{"kind":"test"}');
INSERT INTO app.kitchen_print_jobs(id,ticket_id,printer_id,printer_snapshot,content,state) VALUES
    ('00000000-0000-4000-8000-000000000061','00000000-0000-4000-8000-000000000051','00000000-0000-4000-8000-000000000041',
     '{"id":"00000000-0000-4000-8000-000000000041","name":"Tickets","transport":"cups","destination":"COLA_TICKETS","port":9100,"paper_width":80,"copies":1,"supports_cut":true,"cut":true,"cutter_option":"","active":true,"fallback_id":null,"revision":1}',
     'PRUEBA','submitted');
UPDATE app.payment_settings SET receipt_printer_id='00000000-0000-4000-8000-000000000041';
`

// stage9FSnapshot resume lo que la 0022 no debe tocar: impresoras, trabajos con sus fotografías y la
// configuración de cobro.
func stage9FSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT concat_ws(' | ',
            (SELECT string_agg(name||':'||transport||':'||paper_width||':'||cut||':'||supports_cut||':'||revision,',' ORDER BY name)
                FROM app.kitchen_printers),
            (SELECT string_agg(id||':'||state||':'||md5(printer_snapshot::text)||':'||md5(content),',' ORDER BY id)
                FROM app.kitchen_print_jobs),
            (SELECT tax_rate||':'||print_receipt_default||':'||coalesce(receipt_printer_id::text,'')||':'||revision
                FROM app.payment_settings))`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// Las impresoras existentes conservan corte total; la que ya cortaba queda sin renglones de avance (NULL: los
// tres saltos de siempre y el corte, revisión 9F M1) y la que no cortaba recibe 4, como una impresora nueva;
// la lista cerrada se valida con CHECK con nombre.
func checkStage9FPaperCut(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var existing string
	if err := pool.QueryRow(ctx, `SELECT string_agg(name||':'||cut_mode||':'||coalesce(cut_feed_lines::text,'NULL'),',' ORDER BY name)
        FROM app.kitchen_printers`).Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing != "Comandas 58:full:4,Tickets:full:NULL" {
		t.Fatalf("las impresoras existentes cambiaron su corte: %s", existing)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.kitchen_printers(id,name,transport,destination)
        VALUES ('00000000-0000-4000-8000-000000000043','Nueva','cups','COLA_NUEVA')`); err != nil {
		t.Fatal(err)
	}
	var mode string
	var feed int
	if err := pool.QueryRow(ctx, "SELECT cut_mode,cut_feed_lines FROM app.kitchen_printers WHERE name='Nueva'").Scan(&mode, &feed); err != nil {
		t.Fatal(err)
	}
	if mode != "full" || feed != 4 {
		t.Fatalf("una impresora nueva no recibe corte total con 4 renglones: %s %d", mode, feed)
	}
	var named int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conrelid='app.kitchen_printers'::regclass
        AND conname IN ('kitchen_printers_cut_mode_check','kitchen_printers_cut_feed_lines_check')`).Scan(&named); err != nil || named != 2 {
		t.Fatalf("faltan los CHECK del corte de papel: %d %v", named, err)
	}
	cases := []struct {
		statement string
		valid     bool
	}{
		{"UPDATE app.kitchen_printers SET cut_mode='partial',cut_feed_lines=0 WHERE name='Nueva'", true},
		{"UPDATE app.kitchen_printers SET cut_feed_lines=8 WHERE name='Nueva'", true},
		{"UPDATE app.kitchen_printers SET cut_feed_lines=9 WHERE name='Nueva'", false},
		{"UPDATE app.kitchen_printers SET cut_feed_lines=-1 WHERE name='Nueva'", false},
		// NULL es el final anterior a la 9F; Go nunca lo escribe en una impresora nueva ni lo pone al editar.
		{"UPDATE app.kitchen_printers SET cut_feed_lines=NULL WHERE name='Nueva'", true},
		{"UPDATE app.kitchen_printers SET cut_mode='medio' WHERE name='Nueva'", false},
		{"UPDATE app.kitchen_printers SET cut_mode=NULL WHERE name='Nueva'", false},
		// El CHECK de la 0005 sigue vigente: cortar exige supports_cut (Go guarda supports_cut = cut).
		{"UPDATE app.kitchen_printers SET cut=true,supports_cut=false WHERE name='Nueva'", false},
	}
	for _, entry := range cases {
		_, err := pool.Exec(ctx, entry.statement)
		if (err == nil) != entry.valid {
			t.Fatalf("%s: se esperaba válido=%v, error %v", entry.statement, entry.valid, err)
		}
	}
}

// El nombre visible de la precuenta es «Precuenta» por omisión y una lista cerrada con CHECK con nombre.
func checkStage9FPrecheckName(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var name string
	if err := pool.QueryRow(ctx, "SELECT precheck_name FROM app.payment_settings").Scan(&name); err != nil || name != "precheck" {
		t.Fatalf("el nombre de la precuenta no es «Precuenta» por omisión: %q %v", name, err)
	}
	for _, value := range []string{"sales_ticket", "bill", "precheck"} {
		if _, err := pool.Exec(ctx, "UPDATE app.payment_settings SET precheck_name=$1", value); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
	}
	for _, value := range []any{"Ticket de venta", "", nil} {
		if _, err := pool.Exec(ctx, "UPDATE app.payment_settings SET precheck_name=$1", value); err == nil {
			t.Fatalf("se aceptó el nombre %v fuera de la lista cerrada", value)
		}
	}
	var named int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conrelid='app.payment_settings'::regclass
        AND conname='payment_settings_precheck_name_check'`).Scan(&named); err != nil || named != 1 {
		t.Fatalf("falta el CHECK del nombre de la precuenta: %d %v", named, err)
	}
}
