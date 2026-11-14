//go:build integration

package database

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/product"
	"local/ad-el-pos/server/migrations"
)

// Etapa 9G (fase D): actualización desde el esquema 22 con un catálogo, cuentas y claves de operación existentes.
// La sección D de la 0023 agrega «Agotado» a los productos (sin marcar ninguno), el permiso
// catalog.sold_out.manage solo al administrador, el ámbito «catalog» de los eventos y la bitácora de versiones del
// catálogo con sus disparadores. No cambia productos (ni su revisión), cuentas ni claves de operación. Las fases B, E
// y F agregan aquí sus propias comprobaciones. Deja la base vacía.
func TestStage9GAndroidUpgrade(t *testing.T) {
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
	if last := expected[len(expected)-1].Version; last != 23 || product.Read().SchemaVersion != 23 {
		t.Fatalf("se esperaba el esquema 23 en la última migración (%d) y en el producto (%d)", last, product.Read().SchemaVersion)
	}
	schema22 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 22 {
			schema22[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema22); err != nil || n != 22 {
		t.Fatalf("no se preparó el esquema 22: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stage9GExistingCatalog); err != nil {
		t.Fatal(err)
	}
	before := stage9GSnapshot(ctx, t, pool)
	if n, err := Migrate(ctx, pool); err != nil || n != 1 {
		t.Fatalf("la 0023 no se aplicó sobre el esquema 22: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0023 no es repetible")
	}
	if after := stage9GSnapshot(ctx, t, pool); after != before {
		t.Fatalf("la 0023 cambió datos existentes: antes %s, después %s", before, after)
	}
	checkStage9GSoldOut(ctx, t, pool)
	checkStage9GPermission(ctx, t, pool)
	checkStage9GCatalogScope(ctx, t, pool)
	checkStage9GCatalogVersion(ctx, t, pool)
	// Fase F: respaldos programados (stage9g_backups_upgrade_test.go).
	checkStage9GBackups(ctx, t, pool)
}

// Un catálogo con una presentación, la configuración de cobro, el administrador y un supervisor, una cuenta abierta
// y una clave de operación.
const stage9GExistingCatalog = `
INSERT INTO app.establishment(name,timezone) VALUES ('Restaurante de prueba','America/Monterrey');
INSERT INTO app.roles(id,code,name) VALUES ('00000000-0000-4000-8000-000000000001','administrator','Administrador'),
    ('00000000-0000-4000-8000-000000000002','supervisor','Supervisor');
INSERT INTO app.role_permissions(role_id,permission_key) SELECT '00000000-0000-4000-8000-000000000001',key FROM app.permissions;
INSERT INTO app.users(id,username,display_name,role_id) VALUES
    ('00000000-0000-4000-8000-000000000011','cajera','Cajera','00000000-0000-4000-8000-000000000002');
INSERT INTO app.registers(id,name) VALUES ('00000000-0000-4000-8000-000000000021','Caja de prueba');
INSERT INTO app.devices(id,name,secret_hash,register_id) VALUES
    ('00000000-0000-4000-8000-000000000022','Terminal 1','hash-9g','00000000-0000-4000-8000-000000000021');
INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response) VALUES
    ('00000000-0000-4000-8000-000000000d01','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022',
     'POST prueba','huella','{"ok":true}');
INSERT INTO app.catalog_price_lists(id,name,fallback) VALUES ('00000000-0000-4000-8000-000000000031','Lista de prueba','base');
INSERT INTO app.catalog_categories(id,name) VALUES ('00000000-0000-4000-8000-000000000041','Platillos');
INSERT INTO app.catalog_products(id,category_id,name,code,kind,base_price,temporary_unavailable) VALUES
    ('00000000-0000-4000-8000-000000000051','00000000-0000-4000-8000-000000000041','Hamburguesa','HAMB-01','product',110,false),
    ('00000000-0000-4000-8000-000000000052','00000000-0000-4000-8000-000000000041','Malteada','MALT-01','product',65,true);
INSERT INTO app.catalog_variants(id,product_id,name,code,price) VALUES
    ('00000000-0000-4000-8000-000000000053','00000000-0000-4000-8000-000000000051','Doble','HAMB-02',150);
INSERT INTO app.restaurant_accounts(id,mode,name,price_list_id,origin,state,opened_by,assigned_user_id,device_id) VALUES
    ('00000000-0000-4000-8000-0000000000a1','dine_in','Mesa 1','00000000-0000-4000-8000-000000000031','Mostrador','open',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022');
`

// stage9GSnapshot resume lo que la 0023 no debe tocar: productos con su revisión y su pausa, presentaciones, cuentas,
// claves de operación y la configuración de cobro.
func stage9GSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT concat_ws(' | ',
            (SELECT string_agg(name||':'||base_price||':'||revision||':'||temporary_unavailable||':'||updated_at,',' ORDER BY name)
                FROM app.catalog_products),
            (SELECT string_agg(name||':'||price,',' ORDER BY name) FROM app.catalog_variants),
            (SELECT string_agg(name||':'||state||':'||revision,',' ORDER BY name) FROM app.restaurant_accounts),
            (SELECT string_agg(id||':'||fingerprint||':'||response::text,',' ORDER BY id) FROM app.restaurant_operations),
            (SELECT tax_rate||':'||revision FROM app.payment_settings))`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// Ningún producto queda agotado y el CHECK con nombre solo admite marcas completas: fin posterior al inicio o nulo
// (hasta nuevo aviso), y nada sin la hora en que se marcó.
func checkStage9GSoldOut(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var marked, named int
	err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.catalog_products WHERE sold_out_at IS NOT NULL),
        (SELECT count(*) FROM pg_constraint WHERE conname='catalog_products_sold_out_check'
            AND conrelid='app.catalog_products'::regclass)`).Scan(&marked, &named)
	if err != nil || marked != 0 || named != 1 {
		t.Fatalf("agotados %d, CHECK %d, %v", marked, named, err)
	}
	valid := []string{
		`UPDATE app.catalog_products SET sold_out_at=now(),sold_out_until=NULL,sold_out_by='00000000-0000-4000-8000-000000000011'
            WHERE code='HAMB-01'`,
		`UPDATE app.catalog_products SET sold_out_at=now(),sold_out_until=now()+interval '5 hours' WHERE code='HAMB-01'`,
		`UPDATE app.catalog_products SET sold_out_at=NULL,sold_out_until=NULL,sold_out_by=NULL WHERE code='HAMB-01'`,
	}
	for _, statement := range valid {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("no se aceptó una marca válida: %v", err)
		}
	}
	invalid := []string{
		`UPDATE app.catalog_products SET sold_out_until=now() WHERE code='HAMB-01'`,
		`UPDATE app.catalog_products SET sold_out_by='00000000-0000-4000-8000-000000000011' WHERE code='HAMB-01'`,
		`UPDATE app.catalog_products SET sold_out_at=now(),sold_out_until=now()-interval '1 hour' WHERE code='HAMB-01'`,
	}
	for _, statement := range invalid {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("se aceptó una marca inválida: %s", statement)
		}
	}
}

func checkStage9GPermission(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var description, holders string
	err := pool.QueryRow(ctx, `SELECT p.description,
            coalesce((SELECT string_agg(r.code,',' ORDER BY r.code) FROM app.role_permissions rp JOIN app.roles r ON r.id=rp.role_id
                WHERE rp.permission_key='catalog.sold_out.manage'),'')
        FROM app.permissions p WHERE p.key='catalog.sold_out.manage'`).Scan(&description, &holders)
	if err != nil {
		t.Fatal(err)
	}
	if description != "Marcar productos como agotados y disponibles otra vez desde la captura" || holders != "administrator" {
		t.Fatalf("permiso: %q a %q", description, holders)
	}
}

func checkStage9GCatalogScope(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	// Época del catálogo (revisión de la fase D, hallazgo 2): una sola fila con su identificador.
	var epochs int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM app.catalog_epoch WHERE epoch IS NOT NULL").Scan(&epochs); err != nil || epochs != 1 {
		t.Fatalf("época del catálogo: %d %v", epochs, err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO app.catalog_epoch(singleton) VALUES (false)"); err == nil {
		t.Fatal("la época del catálogo admitió una segunda fila")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.realtime_events(id,kind,scope)
        VALUES ('00000000-0000-4000-8000-0000000000e1','catalog.changed','catalog')`); err != nil {
		t.Fatalf("el ámbito catalog no se admite: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.realtime_events(id,kind,scope)
        VALUES ('00000000-0000-4000-8000-0000000000e2','prueba','desconocido')`); err == nil {
		t.Fatal("se admitió un ámbito desconocido")
	}
}

// La bitácora de versiones: un número por transacción, el renglón hijo marca a su producto, el borrado es una baja,
// el cambio de la tasa general se anota y los demás cambios de la configuración de cobro no.
func checkStage9GCatalogVersion(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	logged := func() map[string]string {
		rows, err := pool.Query(ctx, "SELECT entity||':'||entity_id,version||':'||deleted FROM app.catalog_change_log")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		result := map[string]string{}
		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				t.Fatal(err)
			}
			result[key] = value
		}
		return result
	}
	// Las marcas de checkStage9GSoldOut ya anotaron la hamburguesa; se empieza desde una bitácora vacía.
	if _, err := pool.Exec(ctx, "DELETE FROM app.catalog_change_log"); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`UPDATE app.catalog_products SET description='Con queso' WHERE code='HAMB-01'`,
		`UPDATE app.catalog_variants SET price=155 WHERE code='HAMB-02'`,
		`UPDATE app.catalog_categories SET description='Del día' WHERE name='Platillos'`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	first := logged()
	hamburger := first["product:00000000-0000-4000-8000-000000000051"]
	category := first["category:00000000-0000-4000-8000-000000000041"]
	if len(first) != 2 || hamburger == "" || hamburger != category || !strings.HasSuffix(hamburger, ":false") {
		t.Fatalf("una transacción no comparte un solo número o la presentación no marcó a su producto: %v", first)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM app.catalog_products WHERE code='MALT-01'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE app.payment_settings SET print_receipt_default=NOT print_receipt_default`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE app.payment_settings SET tax_rate=8`); err != nil {
		t.Fatal(err)
	}
	second := logged()
	removed := second["product:00000000-0000-4000-8000-000000000052"]
	tax := second["tax:00000000-0000-0000-0000-000000000000"]
	if !strings.HasSuffix(removed, ":true") || tax == "" || len(second) != 4 {
		t.Fatalf("la baja o la tasa general no se anotaron (o se anotó otro cambio de cobro): %v", second)
	}
	if versionOf(t, removed) >= versionOf(t, tax) || versionOf(t, hamburger) >= versionOf(t, removed) {
		t.Fatalf("las versiones no aumentan con cada transacción: %v", second)
	}
}

// versionOf toma el número de versión de un renglón «versión:borrado» de la bitácora.
func versionOf(t *testing.T, entry string) int {
	t.Helper()
	number, _, _ := strings.Cut(entry, ":")
	value, err := strconv.Atoi(number)
	if err != nil {
		t.Fatalf("versión inválida en la bitácora: %q", entry)
	}
	return value
}
