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

// stage9AInventoryConstraints son las restricciones con nombre explícito de la 0016 (etapa 9A, A2).
var stage9AInventoryConstraints = []string{
	"inventory_suppliers_phone_secondary_check", "inventory_suppliers_mobile_phone_check", "inventory_suppliers_website_check",
	"inventory_suppliers_kind_check", "inventory_suppliers_counter_check", "inventory_receipts_counter_store_check",
}

// TestStage9AInventoryUpgradeFromSchema14 aplica las migraciones de la etapa 9A sobre una
// instalación en esquema 14 con compras de la etapa 8, incluido un proveedor formal llamado
// «Compras de mostrador» (con RFC, orden, recepción y último precio) y otro «Compras de mostrador
// (general)»: la 0016 no los convierte ni los toca, siembra un único proveedor especial con el primer
// nombre libre, deja las recepciones históricas formales y sin comercio, y sus restricciones validan.
func TestStage9AInventoryUpgradeFromSchema14(t *testing.T) {
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
	stageEight := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 14 {
			stageEight[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, stageEight); err != nil || n != 14 {
		t.Fatalf("no se preparó el esquema 14: %d %v", n, err)
	}
	seedStage8Purchasing(ctx, t, pool)
	before := purchasingSnapshot(ctx, t, pool)
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-14 {
		t.Fatalf("las migraciones de la etapa 9A (0015 en adelante) no se aplicaron sobre el esquema 14: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("las migraciones de la etapa 9A no son repetibles")
	}
	if after := purchasingSnapshot(ctx, t, pool); after != before {
		t.Fatalf("La 0016 alteró compras de la etapa 8:\nantes   %s\ndespués %s", before, after)
	}
	checkCounterSeed(ctx, t, pool)
	checkStage9AInventoryConstraints(ctx, t, pool)
}

// seedStage8Purchasing simula compras reales de la etapa 8: usuario, terminal, operaciones, un
// insumo, tres proveedores (dos con nombres que la siembra no debe usar), una orden en borrador, una
// recepción directa y el último precio del proveedor.
func seedStage8Purchasing(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO app.roles(code,name) VALUES ('administrator','Administrador');
        INSERT INTO app.users(username,display_name,role_id) SELECT 'compras','Compras',id FROM app.roles WHERE code='administrator';
        INSERT INTO app.devices(name,secret_hash) VALUES ('Terminal 1','hash-de-prueba');
        INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        SELECT op.id::uuid,u.id,d.id,op.route,'huella','{}' FROM app.users u,app.devices d,
        (VALUES ('00000000-0000-4000-8000-0000000009a1','POST inventory/purchase-orders'),
                ('00000000-0000-4000-8000-0000000009a2','POST inventory/receipts')) AS op(id,route)
        WHERE u.username='compras';
        INSERT INTO app.inventory_items(id,code,name,base_unit_id)
        SELECT '00000000-0000-4000-8000-0000000009a3','JIT-01','Jitomate',id FROM app.inventory_units WHERE code='kg';
        INSERT INTO app.inventory_suppliers(id,trade_name,legal_name,rfc,phone,contact,notes,revision,created_by,updated_by,
            created_at,updated_at)
        SELECT s.id::uuid,s.name,s.legal,s.rfc,s.phone,'','',s.revision,u.id,u.id,'2026-11-01 10:00:00-06','2026-11-02 11:00:00-06'
        FROM app.users u,
        (VALUES ('00000000-0000-4000-8000-0000000009b1','Compras de mostrador','Mercado Juárez S.A. de C.V.','MJU010203AB1',
                    '33 1234 5678',3),
                ('00000000-0000-4000-8000-0000000009b2','Compras de mostrador (general)','',NULL,'',1),
                ('00000000-0000-4000-8000-0000000009b3','Abarrotes del Norte','',NULL,'Pedir a Laura',2))
            AS s(id,name,legal,rfc,phone,revision)
        WHERE u.username='compras';
        INSERT INTO app.inventory_purchase_orders(id,supplier_id,order_date,expected_warehouse_id,created_by,updated_by,operation_id)
        SELECT '00000000-0000-4000-8000-0000000009c1','00000000-0000-4000-8000-0000000009b1','2026-11-03',w.id,u.id,u.id,
            '00000000-0000-4000-8000-0000000009a1'
        FROM app.inventory_warehouses w,app.users u WHERE w.is_default AND u.username='compras';
        INSERT INTO app.inventory_receipts(id,supplier_id,warehouse_id,occurred_at,received_by,supplier_reference,tax_in_cost,subtotal,
            discount,tax,total,inventory_value,line_count,operation_id)
        SELECT '00000000-0000-4000-8000-0000000009c2','00000000-0000-4000-8000-0000000009b1',w.id,'2026-11-04 09:30:00-06',u.id,'Nota 7',
            true,100,0,16,116,116,1,'00000000-0000-4000-8000-0000000009a2'
        FROM app.inventory_warehouses w,app.users u WHERE w.is_default AND u.username='compras';
        INSERT INTO app.inventory_supplier_items(supplier_id,item_id,preferred,last_unit_price,last_price_unit_id,last_price_at,
            last_receipt_id)
        SELECT '00000000-0000-4000-8000-0000000009b1','00000000-0000-4000-8000-0000000009a3',true,25,id,'2026-11-04 09:30:00-06',
            '00000000-0000-4000-8000-0000000009c2'
        FROM app.inventory_units WHERE code='kg'`); err != nil {
		t.Fatal(err)
	}
}

// purchasingSnapshot resume, con las columnas de la etapa 8, los proveedores, órdenes, recepciones e
// insumos surtidos existentes, para comprobar que la 0016 no los cambia.
func purchasingSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT concat_ws(' || ',
        (SELECT string_agg(concat_ws(';',id,trade_name,legal_name,rfc,phone,email,address,contact,notes,active,revision,created_by,
            updated_by,created_at,updated_at),'|' ORDER BY id) FROM app.inventory_suppliers
            WHERE id IN ('00000000-0000-4000-8000-0000000009b1','00000000-0000-4000-8000-0000000009b2',
                '00000000-0000-4000-8000-0000000009b3')),
        (SELECT string_agg(concat_ws(';',id,folio,supplier_id,state,order_date,total,revision),'|' ORDER BY id)
            FROM app.inventory_purchase_orders),
        (SELECT string_agg(concat_ws(';',id,folio,supplier_id,purchase_order_id,occurred_at,supplier_reference,total,inventory_value),'|'
            ORDER BY id) FROM app.inventory_receipts),
        (SELECT string_agg(concat_ws(';',supplier_id,item_id,preferred,last_unit_price,last_price_at,last_receipt_id,revision),'|'
            ORDER BY supplier_id,item_id) FROM app.inventory_supplier_items),
        (SELECT count(*) FROM app.inventory_movements))`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// checkCounterSeed comprueba la siembra: un solo proveedor de mostrador con el primer nombre libre,
// sin RFC, razón social, usuario, insumos ni compras; los existentes quedan formales y sin datos
// nuevos, y las recepciones históricas sin comercio.
func checkCounterSeed(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var name, legal string
	var rfc, createdBy *string
	var active bool
	var revision, counters, formal, emptyFields, items, orders, receipts, stores int
	err := pool.QueryRow(ctx, `SELECT s.trade_name,s.legal_name,s.rfc,s.created_by::text,s.active,s.revision,
            (SELECT count(*) FROM app.inventory_suppliers WHERE kind='counter'),
            (SELECT count(*) FROM app.inventory_suppliers WHERE kind='formal'),
            (SELECT count(*) FROM app.inventory_suppliers WHERE phone_secondary='' AND mobile_phone='' AND website=''),
            (SELECT count(*) FROM app.inventory_supplier_items WHERE supplier_id=s.id),
            (SELECT count(*) FROM app.inventory_purchase_orders WHERE supplier_id=s.id),
            (SELECT count(*) FROM app.inventory_receipts WHERE supplier_id=s.id),
            (SELECT count(*) FROM app.inventory_receipts WHERE counter_store<>'')
        FROM app.inventory_suppliers s WHERE s.kind='counter'`).
		Scan(&name, &legal, &rfc, &createdBy, &active, &revision, &counters, &formal, &emptyFields, &items, &orders, &receipts, &stores)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Compras de mostrador (general 2)" || legal != "" || rfc != nil || createdBy != nil || !active || revision != 1 {
		t.Fatalf("Proveedor de mostrador sembrado: %q %q %v %v %v %d", name, legal, rfc, createdBy, active, revision)
	}
	if counters != 1 || formal != 3 || emptyFields != 4 || items != 0 || orders != 0 || receipts != 0 || stores != 0 {
		t.Fatalf("Siembra de la 0016: mostrador %d, formales %d, sin datos nuevos %d, insumos %d, órdenes %d, recepciones %d, comercios %d",
			counters, formal, emptyFields, items, orders, receipts, stores)
	}
}

// checkStage9AInventoryConstraints comprueba nombres, CHECK (con una copia temporal de las tablas) y
// el índice único parcial del proveedor de mostrador (sobre la tabla real).
func checkStage9AInventoryConstraints(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var named, indexes int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM pg_constraint WHERE conname = ANY($1)),
        (SELECT count(*) FROM pg_indexes WHERE schemaname='app' AND indexname='inventory_suppliers_counter_key')`,
		stage9AInventoryConstraints).Scan(&named, &indexes); err != nil {
		t.Fatal(err)
	}
	if named != len(stage9AInventoryConstraints) || indexes != 1 {
		t.Fatalf("Faltan restricciones con nombre explícito en la 0016: %d de %d, índice %d", named, len(stage9AInventoryConstraints), indexes)
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	for copy, table := range map[string]string{"suppliers_9a_check": "inventory_suppliers", "receipts_9a_check": "inventory_receipts"} {
		statement := "CREATE TEMP TABLE " + copy + " (LIKE app." + table + " INCLUDING CONSTRAINTS INCLUDING DEFAULTS INCLUDING IDENTITY)"
		if _, err := connection.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	supplier := "INSERT INTO suppliers_9a_check(id,trade_name,legal_name,rfc,phone_secondary,mobile_phone,website,kind) VALUES " +
		"(gen_random_uuid(),'Proveedor',%s)"
	receipt := "INSERT INTO receipts_9a_check(id,supplier_id,warehouse_id,occurred_at,received_by,tax_in_cost,subtotal,discount,tax,total," +
		"inventory_value,line_count,operation_id,counter_store) VALUES (gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),now()," +
		"gen_random_uuid(),true,100,0,16,116,116,1,gen_random_uuid(),%s)"
	fill := func(template, values string) string {
		return strings.Replace(template, "%s", values, 1)
	}
	// Valores de proveedor: razón social, RFC, segundo teléfono, celular, página web y modalidad.
	accepted := []string{
		fill(supplier, "'',NULL,'','','','formal'"),
		fill(supplier, "'Mercado S.A.','MJU010203AB1','33 3615 0000 ext. 12','+52 1 33 1234 5678','https://mercado.mx/a','formal'"),
		fill(supplier, "'',NULL,'','','','counter'"),
		fill(receipt, "''"), fill(receipt, "'Central de abastos, bodega 12'"), fill(receipt, "'"+strings.Repeat("m", 100)+"'"),
	}
	rejected := []string{
		// Datos nuevos demasiado largos, con espacios en los extremos, vacíos como NULL o web con espacios.
		fill(supplier, "'',NULL,'"+strings.Repeat("5", 41)+"','','','formal'"),
		fill(supplier, "'',NULL,' 33 1234 5678','','','formal'"),
		fill(supplier, "'',NULL,'','"+strings.Repeat("5", 41)+"','','formal'"),
		fill(supplier, "'',NULL,'','','www.proveedor .mx','formal'"),
		fill(supplier, "'',NULL,'','','https://"+strings.Repeat("a", 190)+".mx','formal'"),
		fill(supplier, "'',NULL,NULL,'','','formal'"),
		// Modalidad desconocida y proveedor de mostrador con razón social o RFC.
		fill(supplier, "'',NULL,'','','','mostrador'"),
		fill(supplier, "'Mercado S.A.',NULL,'','','','counter'"),
		fill(supplier, "'','MJU010203AB1','','','','counter'"),
		// Comercio con espacios en los extremos, demasiado largo o NULL.
		fill(receipt, "' Central'"), fill(receipt, "'"+strings.Repeat("m", 101)+"'"), fill(receipt, "NULL"),
	}
	for _, statement := range accepted {
		if _, err := connection.Exec(ctx, statement); err != nil {
			t.Fatalf("PostgreSQL rechazó un registro válido de la 0016 (%s): %v", statement, err)
		}
	}
	for _, statement := range rejected {
		if _, err := connection.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó un registro incoherente de la 0016: %s", statement)
		}
	}
	for _, copy := range []string{"suppliers_9a_check", "receipts_9a_check"} {
		if _, err := connection.Exec(ctx, "DROP TABLE "+copy); err != nil {
			t.Fatal(err)
		}
	}
	// El índice único parcial impide un segundo proveedor de mostrador, aun con otro nombre.
	if _, err := pool.Exec(ctx, "INSERT INTO app.inventory_suppliers(id,trade_name,kind) VALUES (gen_random_uuid(),'Mercados','counter')"); err == nil {
		t.Fatal("PostgreSQL aceptó un segundo proveedor de mostrador")
	}
}
