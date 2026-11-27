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

// Etapa M1 (Fase A): actualización desde el esquema 30 con lo que dejó la 0.17.0 (los datos de la prueba de la K1.1:
// cuentas abiertas, cobradas, canceladas y combinadas, pedidos, unidades, eventos y cocina), una venta reconocida en
// una mesa y los roles del propietario. La sección «Fase A» de la 0031 agrega las tablas de uniones y grupos (vacías),
// las tres columnas de la unión en sales_recognitions (NULL en las filas que ya existían) y el permiso «Unir mesas»,
// que concede solo a los roles Administrador, Mesero, Cajero y Supervisor que ya abren cuentas y ven las mesas (M1-D8).
// Ninguna otra fila cambia: cuentas, ventas, roles ni lo demás. Es repetible y deja la base vacía.
func TestStageM1Upgrade(t *testing.T) {
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
	if last := expected[len(expected)-1].Version; last < 31 || last != product.Read().SchemaVersion {
		t.Fatalf("la última migración (%d) no coincide con el esquema del producto (%d)", last, product.Read().SchemaVersion)
	}
	schema30 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 30 {
			schema30[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema30); err != nil || n != 30 {
		t.Fatalf("no se preparó el esquema 30: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stageK11ExistingData); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, stageM1ExistingData); err != nil {
		t.Fatal(err)
	}
	tables := stageA1Tables(ctx, t, pool)
	before := map[string]string{}
	for table, columns := range tables {
		before[table] = stageA1Snapshot(ctx, t, pool, table, columns)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-30 {
		t.Fatalf("la 0031 no se aplicó sobre el esquema 30: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0031 no es repetible")
	}
	// stageA1Snapshot deja fuera el permiso nuevo y su concesión (se comprueban abajo); las columnas nuevas no cuentan
	// (la comparación usa las del esquema 30).
	for table, columns := range tables {
		if after := stageA1Snapshot(ctx, t, pool, table, columns); after != before[table] {
			t.Fatalf("la 0031 cambió filas existentes de %s:\nantes   %s\ndespués %s", table, before[table], after)
		}
	}
	var granted string
	if err := pool.QueryRow(ctx, `SELECT coalesce(string_agg(coalesce(r.code, r.name), ',' ORDER BY coalesce(r.code, r.name)), '')
        FROM app.role_permissions p JOIN app.roles r ON r.id = p.role_id WHERE p.permission_key = 'restaurant.tables.join'`).
		Scan(&granted); err != nil {
		t.Fatal(err)
	}
	if granted != "administrator,cashier,waiter" {
		t.Fatalf("«Unir mesas» concedido a: %s", granted)
	}
	var description string
	if err := pool.QueryRow(ctx, "SELECT description FROM app.permissions WHERE key = 'restaurant.tables.join'").Scan(&description); err != nil ||
		description != "Unir mesas" {
		t.Fatalf("el permiso: %q %v", description, err)
	}
	var unions, groups, snapshots int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.table_unions), (SELECT count(*) FROM app.table_groups),
            (SELECT count(*) FROM app.sales_recognitions WHERE table_union_id IS NOT NULL OR table_union_name IS NOT NULL
                OR table_union_tables IS NOT NULL)`).Scan(&unions, &groups, &snapshots); err != nil {
		t.Fatal(err)
	}
	if unions != 0 || groups != 0 || snapshots != 0 {
		t.Fatalf("la 0031 escribió uniones, grupos o instantáneas: %d %d %d", unions, groups, snapshots)
	}
	// La defensa en la base: una mesa en dos uniones abiertas se rechaza.
	if _, err := pool.Exec(ctx, stageM1TwoUnions); !stageK1Violates("23505", err) {
		t.Fatalf("una mesa en dos uniones abiertas: %v", err)
	}
}

// stageM1ExistingData agrega a los datos de la K1.1 los roles del propietario (con y sin lo que necesita «Unir mesas»,
// y un rol propio sin código), un área con dos mesas, una cuenta abierta en una de ellas y una venta reconocida.
const stageM1ExistingData = `
INSERT INTO app.roles(id,code,name) VALUES
    ('00000000-0000-4000-8000-000000000002','waiter','Mesero'),
    ('00000000-0000-4000-8000-000000000003','cashier','Cajero'),
    ('00000000-0000-4000-8000-000000000004','supervisor','Supervisor'),
    ('00000000-0000-4000-8000-000000000005','kitchen','Cocina'),
    ('00000000-0000-4000-8000-000000000006',NULL,'Mesero de terraza');
INSERT INTO app.role_permissions(role_id,permission_key)
SELECT g.role::uuid,g.permission FROM (VALUES
    ('00000000-0000-4000-8000-000000000001','accounts.open'),
    ('00000000-0000-4000-8000-000000000001','restaurant.tables.read'),
    ('00000000-0000-4000-8000-000000000002','accounts.open'),
    ('00000000-0000-4000-8000-000000000002','restaurant.tables.read'),
    ('00000000-0000-4000-8000-000000000003','accounts.open'),
    ('00000000-0000-4000-8000-000000000003','restaurant.tables.read'),
    ('00000000-0000-4000-8000-000000000004','accounts.open'),
    ('00000000-0000-4000-8000-000000000005','kitchen.states.read'),
    ('00000000-0000-4000-8000-000000000006','accounts.open'),
    ('00000000-0000-4000-8000-000000000006','restaurant.tables.read')) g(role,permission);
INSERT INTO app.restaurant_areas(id,name) VALUES ('00000000-0000-4000-8000-000000000071','Salón M1');
INSERT INTO app.restaurant_tables(id,area_id,name,seats) VALUES
    ('00000000-0000-4000-8000-000000000072','00000000-0000-4000-8000-000000000071','Mesa 5',4),
    ('00000000-0000-4000-8000-000000000073','00000000-0000-4000-8000-000000000071','Mesa 6',4);
UPDATE app.restaurant_accounts SET table_id='00000000-0000-4000-8000-000000000072' WHERE id='00000000-0000-4000-8000-0000000000a4';
INSERT INTO app.sales_recognitions(closing_id,recognition_kind,recognized_at,account_id,account_folio,mode,origin,table_id,
    table_name,source_kind,unit_id,tax_rate,quantity,gross,promotion,discount,courtesy,charge,net,base,tax)
SELECT gen_random_uuid(),'initial','2026-11-25T20:00:00Z',a.id,a.folio,'dine_in','Mostrador',
    '00000000-0000-4000-8000-000000000073','Mesa 6','unit','00000000-0000-4000-8000-000000000311',16,1,35,0,0,0,0,35,30.17,4.83
FROM app.restaurant_accounts a WHERE a.id='00000000-0000-4000-8000-0000000000a1';
`

// stageM1TwoUnions intenta dejar la Mesa 5 en dos uniones abiertas.
const stageM1TwoUnions = `
INSERT INTO app.table_unions(id,area_id,name,opened_by,opened_device_id) VALUES
    ('00000000-0000-4000-8000-000000000081','00000000-0000-4000-8000-000000000071','Mesa 5 + 6',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022'),
    ('00000000-0000-4000-8000-000000000082','00000000-0000-4000-8000-000000000071','Mesa 5 + 7',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022');
INSERT INTO app.table_union_members(id,union_id,table_id,joined_by) VALUES
    ('00000000-0000-4000-8000-000000000091','00000000-0000-4000-8000-000000000081','00000000-0000-4000-8000-000000000072',
     '00000000-0000-4000-8000-000000000011'),
    ('00000000-0000-4000-8000-000000000092','00000000-0000-4000-8000-000000000082','00000000-0000-4000-8000-000000000072',
     '00000000-0000-4000-8000-000000000011');
`
