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
	"local/ad-el-pos/server/migrations"
)

// La 0015 (etapa 9A, parte A1) se aplica sobre una instalación en esquema 14 con productos, un
// combo y precuentas y tickets ya impresos: los productos quedan con la opción «El precio puede
// bajar al cambiar complementos» desactivada y sin ningún otro cambio (ni revisión ni fecha), y los
// documentos históricos conservan NULL en la revisión de la cuenta y el número de ajustes (siguen
// la regla anterior por marcas de tiempo). Como las demás pruebas de actualización, parte de un
// esquema vacío en la base temporal exclusiva del paquete y la deja vacía al terminar.
func TestStage9ACatalogAndPrecheckUpgrade(t *testing.T) {
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
	stageFourteen := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 14 {
			stageFourteen[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, stageFourteen); err != nil || n != 14 {
		t.Fatalf("no se preparó el esquema 14: %d %v", n, err)
	}
	// Instalación en esquema 14: un producto con tasa propia y revisión 3, un combo y una cuenta con
	// una precuenta y un ticket impresos.
	if _, err := pool.Exec(ctx, `INSERT INTO app.roles(code,name) VALUES ('administrator','Administrador');
        INSERT INTO app.users(id,username,display_name,role_id)
            SELECT '00000000-0000-4000-8000-0000000001a1','cajera','Cajera',id FROM app.roles WHERE code='administrator';
        INSERT INTO app.devices(id,name,secret_hash) VALUES ('00000000-0000-4000-8000-0000000001a2','Terminal 1','hash-de-prueba');
        INSERT INTO app.catalog_categories(id,name) VALUES ('00000000-0000-4000-8000-0000000001a3','Platillos');
        INSERT INTO app.catalog_products(id,category_id,name,code,kind,base_price,tax_rate,revision,updated_at) VALUES
            ('00000000-0000-4000-8000-0000000001a4','00000000-0000-4000-8000-0000000001a3','Filete','FILETE','product',150,8,3,
                '2026-11-01 12:00:00+00'),
            ('00000000-0000-4000-8000-0000000001a5','00000000-0000-4000-8000-0000000001a3','Combo','COMBO','combo',160,NULL,1,
                '2026-11-02 12:00:00+00');
        INSERT INTO app.restaurant_accounts(id,mode,name,price_list_id,opened_by,assigned_user_id,device_id,revision)
            SELECT '00000000-0000-4000-8000-0000000001a6','takeaway','Pedido',l.id,'00000000-0000-4000-8000-0000000001a1',
                '00000000-0000-4000-8000-0000000001a1','00000000-0000-4000-8000-0000000001a2',4
            FROM app.catalog_price_lists l WHERE l.is_default;
        INSERT INTO app.kitchen_tickets(id,kind,station_ids,document) VALUES
            ('00000000-0000-4000-8000-0000000001a7','precheck','{}','{}'),
            ('00000000-0000-4000-8000-0000000001a8','receipt','{}','{}');
        INSERT INTO app.payment_documents(ticket_id,account_id,kind,actor_id,device_id) VALUES
            ('00000000-0000-4000-8000-0000000001a7','00000000-0000-4000-8000-0000000001a6','precheck',
                '00000000-0000-4000-8000-0000000001a1','00000000-0000-4000-8000-0000000001a2'),
            ('00000000-0000-4000-8000-0000000001a8','00000000-0000-4000-8000-0000000001a6','receipt',
                '00000000-0000-4000-8000-0000000001a1','00000000-0000-4000-8000-0000000001a2')`); err != nil {
		t.Fatal(err)
	}
	// Foto de productos y documentos sin las columnas de la 0015 (antes no existen; después se
	// excluyen para comparar todo lo demás). Desde la etapa 9G también sin las de «Agotado» de la 0023, que
	// comprueba stage9g_android_upgrade_test.go.
	snapshot := func() string {
		t.Helper()
		var products, documents string
		err := pool.QueryRow(ctx, `SELECT
            (SELECT string_agg((to_jsonb(p) - 'price_can_decrease' - 'sold_out_at' - 'sold_out_until' - 'sold_out_by')::text,'|'
                ORDER BY p.id) FROM app.catalog_products p),
            (SELECT string_agg((to_jsonb(d) - 'account_revision' - 'adjustment_count')::text,'|' ORDER BY d.ticket_id)
                FROM app.payment_documents d)`).Scan(&products, &documents)
		if err != nil {
			t.Fatal(err)
		}
		return products + "#" + documents
	}
	before := snapshot()
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-14 {
		t.Fatalf("las migraciones desde la 0015 no se aplicaron sobre el esquema 14: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("las migraciones desde la 0015 no son repetibles")
	}
	// Ningún producto ni documento cambió, salvo por las columnas nuevas.
	if after := snapshot(); after != before {
		t.Fatalf("La 0015 alteró productos o documentos existentes:\nantes   %s\ndespués %s", before, after)
	}
	var enabled, historic int
	var nullable, columnDefault string
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.catalog_products WHERE price_can_decrease),
            (SELECT count(*) FROM app.payment_documents WHERE account_revision IS NULL AND adjustment_count IS NULL),
            (SELECT is_nullable FROM information_schema.columns WHERE table_schema='app' AND table_name='catalog_products'
                AND column_name='price_can_decrease'),
            (SELECT column_default FROM information_schema.columns WHERE table_schema='app' AND table_name='catalog_products'
                AND column_name='price_can_decrease')`).Scan(&enabled, &historic, &nullable, &columnDefault); err != nil {
		t.Fatal(err)
	}
	if enabled != 0 || historic != 2 || nullable != "NO" || columnDefault != "false" {
		t.Fatalf("Columnas nuevas de la 0015: activados %d, históricos %d, nulo %s, omisión %s", enabled, historic, nullable, columnDefault)
	}
	checkPrecheckVersionConstraint(ctx, t, pool)
}

// checkPrecheckVersionConstraint prueba el CHECK con nombre de la 0015 con una copia temporal de
// payment_documents: solo una precuenta guarda revisión y número de ajustes, siempre juntos y
// válidos; un ticket de pago o una fila histórica los dejan en NULL.
func checkPrecheckVersionConstraint(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var named int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_constraint WHERE conname='payment_documents_precheck_version_check'").Scan(&named); err != nil || named != 1 {
		t.Fatalf("Falta la restricción con nombre de la 0015: %d %v", named, err)
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, "CREATE TEMP TABLE documents_check (LIKE app.payment_documents INCLUDING CONSTRAINTS INCLUDING DEFAULTS)"); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO documents_check(ticket_id,account_id,kind,actor_id,device_id,account_revision,adjustment_count)
        VALUES (gen_random_uuid(),gen_random_uuid(),%s,gen_random_uuid(),gen_random_uuid(),%s,%s)`
	statement := func(kind, revision, adjustments string) string {
		result := strings.Replace(insert, "%s", kind, 1)
		result = strings.Replace(result, "%s", revision, 1)
		return strings.Replace(result, "%s", adjustments, 1)
	}
	valid := [][3]string{
		{"'precheck'", "4", "0"},
		{"'precheck'", "1", "7"},
		{"'precheck'", "NULL", "NULL"},
		{"'receipt'", "NULL", "NULL"},
	}
	for _, values := range valid {
		if _, err := connection.Exec(ctx, statement(values[0], values[1], values[2])); err != nil {
			t.Fatalf("PostgreSQL rechazó un documento válido %v: %v", values, err)
		}
	}
	invalid := [][3]string{
		{"'receipt'", "4", "0"},
		{"'precheck'", "4", "NULL"},
		{"'precheck'", "NULL", "0"},
		{"'precheck'", "0", "0"},
		{"'precheck'", "4", "-1"},
	}
	for _, values := range invalid {
		if _, err := connection.Exec(ctx, statement(values[0], values[1], values[2])); err == nil {
			t.Fatalf("PostgreSQL aceptó un documento incoherente %v", values)
		}
	}
}
