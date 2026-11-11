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

// Etapa 9A (B1): actualización desde el esquema 14 con cuentas cerradas existentes. La 0017 rellena
// el libro de reconocimientos con las cuentas hoy cerradas: fecha del primer cierre de la auditoría
// (o closed_at si no hay auditoría), turno y caja del pago registrado en ese cierre, importes y nombres
// del estado actual (unidades activas, descuentos vigentes por partida y generales, cargos y
// cortesías), con el IVA por cuenta y tasa de finance.go. Las cuentas abiertas no se rellenan, las
// unidades canceladas y los descuentos cancelados no cuentan, el libro es de solo inserción y los
// permisos reports.* se conceden solo al administrador. Deja la base vacía al terminar.
func TestStage9AReportsUpgradeBackfillsClosedAccounts(t *testing.T) {
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
	schema14 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 14 {
			schema14[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema14); err != nil || n != 14 {
		t.Fatalf("no se preparó el esquema 14: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stage9AClosedAccounts); err != nil {
		t.Fatal(err)
	}
	var cashierBefore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id
        WHERE r.code='cashier'`).Scan(&cashierBefore); err != nil {
		t.Fatal(err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-14 {
		t.Fatalf("las migraciones de la etapa 9A no se aplicaron sobre el esquema 14: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("las migraciones de la etapa 9A no son repetibles")
	}
	checkStage9ABackfill(ctx, t, pool)
	checkStage9AReportPermissions(ctx, t, pool, cashierBefore)
	// El libro es de solo inserción.
	for _, statement := range []string{"UPDATE app.sales_recognitions SET net=net", "DELETE FROM app.sales_recognitions",
		"TRUNCATE app.sales_recognitions"} {
		if _, err := pool.Exec(ctx, statement); err == nil || !strings.Contains(err.Error(), "solo inserción") {
			t.Fatalf("%s no se rechazó: %v", statement, err)
		}
	}
}

func checkStage9ABackfill(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	type accountTotals struct {
		rows                                                         int
		quantity                                                     int
		gross, promotion, discount, courtesy, charge, net, base, tax string
		recognizedAt                                                 time.Time
		shift                                                        *string
	}
	totals := map[string]accountTotals{}
	rows, err := pool.Query(ctx, `SELECT account_id::text,count(*),sum(quantity),sum(gross)::text,sum(promotion)::text,
            sum(discount)::text,sum(courtesy)::text,sum(charge)::text,sum(net)::text,sum(base)::text,sum(tax)::text,
            min(recognized_at),min(shift_id::text)
        FROM app.sales_recognitions WHERE recognition_kind='backfill' GROUP BY account_id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		var value accountTotals
		if err := rows.Scan(&id, &value.rows, &value.quantity, &value.gross, &value.promotion, &value.discount, &value.courtesy,
			&value.charge, &value.net, &value.base, &value.tax, &value.recognizedAt, &value.shift); err != nil {
			t.Fatal(err)
		}
		totals[id] = value
	}
	rows.Close()
	if len(totals) != 3 {
		t.Fatalf("Se esperaban tres cuentas cerradas rellenadas (no la abierta): %+v", totals)
	}
	// Cuenta A: 3 unidades activas (la cancelada no cuenta), descuento por partida, cargo y dos
	// descuentos generales repartidos entre 16 % y 8 %; el descuento cancelado no cuenta. La fecha es
	// el primer cierre de la auditoría, no el closed_at de la reliquidación.
	a := totals["00000000-0000-4000-8000-0000000000a1"]
	if a.rows != 9 || a.quantity != 3 || a.gross != "193.33" || a.promotion != "10.00" || a.discount != "22.78" ||
		a.courtesy != "0.00" || a.charge != "12.50" || a.net != "173.05" || a.tax != "20.90" || a.base != "152.15" {
		t.Fatalf("Relleno de la cuenta A: %+v", a)
	}
	if !a.recognizedAt.Equal(time.Date(2026, time.November, 4, 2, 15, 0, 0, time.UTC)) || a.shift == nil ||
		*a.shift != "00000000-0000-4000-8000-0000000000e1" {
		t.Fatalf("La cuenta A debe fecharse con su primer cierre y el turno de su pago: %s %v", a.recognizedAt, a.shift)
	}
	// Cuenta B: cortesía al 100 % = venta 0 con su importe en cortesías, sin turno (no hubo pago).
	b := totals["00000000-0000-4000-8000-0000000000a2"]
	if b.rows != 3 || b.net != "0.00" || b.courtesy != "50.00" || b.tax != "0.00" || b.shift != nil {
		t.Fatalf("Relleno de la cortesía: %+v", b)
	}
	// Cuenta D: sin auditoría de cierre, se usa closed_at.
	d := totals["00000000-0000-4000-8000-0000000000a4"]
	if d.rows != 1 || d.net != "120.00" || d.tax != "16.55" || !d.recognizedAt.Equal(time.Date(2026, time.November, 5, 18, 0, 0, 0, time.UTC)) {
		t.Fatalf("Relleno sin auditoría: %+v", d)
	}
	// IVA por tasa de la cuenta A (finance.go): 16 % sobre 126.49 y 8 % sobre 46.56.
	var tax16, tax8, part4, part5 string
	if err := pool.QueryRow(ctx, `SELECT
            (SELECT sum(tax)::text FROM app.sales_recognitions WHERE account_id='00000000-0000-4000-8000-0000000000a1' AND tax_rate=16),
            (SELECT sum(tax)::text FROM app.sales_recognitions WHERE account_id='00000000-0000-4000-8000-0000000000a1' AND tax_rate=8),
            (SELECT sum(discount)::text FROM app.sales_recognitions WHERE adjustment_id='00000000-0000-4000-8000-0000000000c4'),
            (SELECT sum(discount)::text FROM app.sales_recognitions WHERE adjustment_id='00000000-0000-4000-8000-0000000000c5')`).
		Scan(&tax16, &tax8, &part4, &part5); err != nil {
		t.Fatal(err)
	}
	if tax16 != "17.45" || tax8 != "3.45" || part4 != "7.77" || part5 != "5.01" {
		t.Fatalf("IVA por tasa o reparto de descuentos generales: %s %s %s %s", tax16, tax8, part4, part5)
	}
	// Datos históricos copiados: nombres del pedido, motivo con su nombre registrado, mesa y área.
	var product, code, variant, category, list, table, area, reason string
	if err := pool.QueryRow(ctx, `SELECT u.product_name,u.product_code,u.variant_name,u.category_name,u.price_list_name,
            u.table_name,u.area_name,a.reason_name
        FROM app.sales_recognitions u
        JOIN app.sales_recognitions a ON a.unit_id=u.unit_id AND a.source_kind='allocation'
        WHERE u.unit_id='00000000-0000-4000-8000-000000000301' AND u.source_kind='unit'`).
		Scan(&product, &code, &variant, &category, &list, &table, &area, &reason); err != nil {
		t.Fatal(err)
	}
	if product != "Hamburguesa" || code != "HAMB-01" || variant != "Doble" || category != "Platillos" || list != "Lista de prueba" ||
		table != "Mesa 7" || area != "Terraza" || reason != "Atención (nombre anterior)" {
		t.Fatalf("Datos históricos: %s %s %s %s %s %s %s %s", product, code, variant, category, list, table, area, reason)
	}
	var consistent bool
	if err := pool.QueryRow(ctx, `SELECT bool_and(net = gross - promotion - discount - courtesy + charge AND net = base + tax)
        FROM app.sales_recognitions`).Scan(&consistent); err != nil || !consistent {
		t.Fatalf("Renglones incoherentes: %v", err)
	}
}

func checkStage9AReportPermissions(ctx context.Context, t *testing.T, pool *pgxpool.Pool, cashierBefore int) {
	t.Helper()
	var reportPermissions, administrator, others, cashierAfter int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.permissions WHERE key LIKE 'reports.%'),
            (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id
                WHERE r.code='administrator' AND p.permission_key LIKE 'reports.%'),
            (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id
                WHERE r.code<>'administrator' AND p.permission_key LIKE 'reports.%'),
            (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE r.code='cashier')`).
		Scan(&reportPermissions, &administrator, &others, &cashierAfter); err != nil {
		t.Fatal(err)
	}
	if reportPermissions != 10 || administrator != 10 || others != 0 || cashierAfter != cashierBefore {
		t.Fatalf("Permisos de reportes: %d registrados, %d al administrador, %d a otros; cajera %d→%d", reportPermissions,
			administrator, others, cashierBefore, cashierAfter)
	}
}

// stage9AClosedAccounts son datos de una instalación en esquema 14: una cuenta reabierta y vuelta a
// cerrar (A), una cortesía para llevar entregada (B), una cuenta abierta (C) y una cerrada sin
// auditoría de cierre (D). La zona es America/Monterrey (UTC−6).
const stage9AClosedAccounts = `
INSERT INTO app.establishment(name,timezone) VALUES ('Restaurante de prueba','America/Monterrey');
INSERT INTO app.roles(id,code,name) VALUES ('00000000-0000-4000-8000-000000000001','administrator','Administrador'),
    ('00000000-0000-4000-8000-000000000002','cashier','Cajero');
INSERT INTO app.role_permissions(role_id,permission_key) SELECT '00000000-0000-4000-8000-000000000001',key FROM app.permissions;
INSERT INTO app.role_permissions(role_id,permission_key) VALUES ('00000000-0000-4000-8000-000000000002','payments.collect');
INSERT INTO app.users(id,username,display_name,role_id) VALUES
    ('00000000-0000-4000-8000-000000000011','cajera','Cajera','00000000-0000-4000-8000-000000000002');
INSERT INTO app.registers(id,name) VALUES ('00000000-0000-4000-8000-000000000021','Caja de prueba');
INSERT INTO app.devices(id,name,secret_hash,register_id) VALUES
    ('00000000-0000-4000-8000-000000000022','Terminal 1','hash-reportes','00000000-0000-4000-8000-000000000021');
INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
SELECT op::uuid,'00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022','POST prueba','huella','{}'
FROM unnest(ARRAY['00000000-0000-4000-8000-000000000d01','00000000-0000-4000-8000-000000000d02','00000000-0000-4000-8000-000000000d03',
    '00000000-0000-4000-8000-000000000d04','00000000-0000-4000-8000-000000000d05','00000000-0000-4000-8000-000000000d06',
    '00000000-0000-4000-8000-000000000d07','00000000-0000-4000-8000-000000000d08','00000000-0000-4000-8000-000000000d09',
    '00000000-0000-4000-8000-000000000d10','00000000-0000-4000-8000-000000000d11','00000000-0000-4000-8000-000000000d12',
    '00000000-0000-4000-8000-000000000d13','00000000-0000-4000-8000-000000000d14','00000000-0000-4000-8000-000000000d15']) op;
INSERT INTO app.cash_shifts(id,register_id,opening_float,opened_by,opened_device_id,operation_id,business_date) VALUES
    ('00000000-0000-4000-8000-0000000000e1','00000000-0000-4000-8000-000000000021',0,'00000000-0000-4000-8000-000000000011',
     '00000000-0000-4000-8000-000000000022','00000000-0000-4000-8000-000000000d01','2026-11-03');
INSERT INTO app.catalog_price_lists(id,name,fallback) VALUES ('00000000-0000-4000-8000-000000000031','Lista de prueba','base');
INSERT INTO app.catalog_categories(id,name) VALUES ('00000000-0000-4000-8000-000000000041','Platillos actuales'),
    ('00000000-0000-4000-8000-000000000042','Bebidas');
INSERT INTO app.catalog_products(id,category_id,name,code,kind,base_price) VALUES
    ('00000000-0000-4000-8000-000000000051','00000000-0000-4000-8000-000000000041','Hamburguesa renombrada','HAMB-01','product',110),
    ('00000000-0000-4000-8000-000000000052','00000000-0000-4000-8000-000000000042','Refresco','REF-01','product',25),
    ('00000000-0000-4000-8000-000000000053','00000000-0000-4000-8000-000000000041','Ensalada','ENS-01','product',50);
INSERT INTO app.restaurant_areas(id,name) VALUES ('00000000-0000-4000-8000-000000000061','Terraza');
INSERT INTO app.restaurant_tables(id,area_id,name) VALUES ('00000000-0000-4000-8000-000000000062','00000000-0000-4000-8000-000000000061','Mesa 7');
INSERT INTO app.restaurant_accounts(id,mode,name,table_id,price_list_id,origin,state,opened_by,assigned_user_id,device_id,closed_at) VALUES
    ('00000000-0000-4000-8000-0000000000a1','dine_in','Mesa 7','00000000-0000-4000-8000-000000000062','00000000-0000-4000-8000-000000000031',
     'Mostrador','closed','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011',
     '00000000-0000-4000-8000-000000000022','2026-11-04T06:00:00Z'),
    ('00000000-0000-4000-8000-0000000000a2','takeaway','Cortesía',NULL,'00000000-0000-4000-8000-000000000031','Ventanilla','closed',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011',
     '00000000-0000-4000-8000-000000000022','2026-11-04T19:00:00Z'),
    ('00000000-0000-4000-8000-0000000000a3','dine_in','Abierta',NULL,'00000000-0000-4000-8000-000000000031','Mostrador','open',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022',NULL),
    ('00000000-0000-4000-8000-0000000000a4','dine_in','Sin auditoría',NULL,'00000000-0000-4000-8000-000000000031','Mostrador','closed',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022',
     '2026-11-05T18:00:00Z');
INSERT INTO app.restaurant_orders(id,original_account_id,operation_id,kind,author_id,device_id,price_list_id,pricing_snapshot,quote)
SELECT o.id::uuid,o.account::uuid,o.operation::uuid,'confirmation','00000000-0000-4000-8000-000000000011',
    '00000000-0000-4000-8000-000000000022','00000000-0000-4000-8000-000000000031',
    '{"Products":{"00000000-0000-4000-8000-000000000051":{"code":"HAMB-01","kind":"product","name":"Hamburguesa",
        "variants":[{"id":"00000000-0000-4000-8000-000000000071","name":"Doble"}]},
      "00000000-0000-4000-8000-000000000052":{"code":"REF-01","kind":"product","name":"Refresco","variants":[]},
      "00000000-0000-4000-8000-000000000053":{"code":"ENS-01","kind":"product","name":"Ensalada","variants":[]}},
     "Categories":{"00000000-0000-4000-8000-000000000041":{"name":"Platillos"},"00000000-0000-4000-8000-000000000042":{"name":"Bebidas"}}}',
    '{"price_list_name":"Lista de prueba"}'
FROM (VALUES ('00000000-0000-4000-8000-000000000201','00000000-0000-4000-8000-0000000000a1','00000000-0000-4000-8000-000000000d02'),
    ('00000000-0000-4000-8000-000000000202','00000000-0000-4000-8000-0000000000a2','00000000-0000-4000-8000-000000000d03'),
    ('00000000-0000-4000-8000-000000000203','00000000-0000-4000-8000-0000000000a4','00000000-0000-4000-8000-000000000d04'),
    ('00000000-0000-4000-8000-000000000204','00000000-0000-4000-8000-0000000000a3','00000000-0000-4000-8000-000000000d05')) o(id,account,operation);
INSERT INTO app.restaurant_items(id,original_order_id,product_id) VALUES
    ('00000000-0000-4000-8000-000000000281','00000000-0000-4000-8000-000000000201','00000000-0000-4000-8000-000000000051'),
    ('00000000-0000-4000-8000-000000000282','00000000-0000-4000-8000-000000000201','00000000-0000-4000-8000-000000000052'),
    ('00000000-0000-4000-8000-000000000283','00000000-0000-4000-8000-000000000201','00000000-0000-4000-8000-000000000053'),
    ('00000000-0000-4000-8000-000000000284','00000000-0000-4000-8000-000000000201','00000000-0000-4000-8000-000000000052'),
    ('00000000-0000-4000-8000-000000000285','00000000-0000-4000-8000-000000000202','00000000-0000-4000-8000-000000000052'),
    ('00000000-0000-4000-8000-000000000286','00000000-0000-4000-8000-000000000203','00000000-0000-4000-8000-000000000051'),
    ('00000000-0000-4000-8000-000000000287','00000000-0000-4000-8000-000000000204','00000000-0000-4000-8000-000000000052');
INSERT INTO app.restaurant_item_versions(id,item_id,order_id,version,request,quote_line,author_id,device_id,tax_rate)
SELECT v.id::uuid,v.item::uuid,v.order_id::uuid,1,'{}',v.line::jsonb,'00000000-0000-4000-8000-000000000011',
    '00000000-0000-4000-8000-000000000022',v.rate
FROM (VALUES
    ('00000000-0000-4000-8000-000000000291','00000000-0000-4000-8000-000000000281','00000000-0000-4000-8000-000000000201',
     '{"name":"Hamburguesa","product_id":"00000000-0000-4000-8000-000000000051","category_id":"00000000-0000-4000-8000-000000000041","variant_id":"00000000-0000-4000-8000-000000000071"}',16),
    ('00000000-0000-4000-8000-000000000292','00000000-0000-4000-8000-000000000282','00000000-0000-4000-8000-000000000201',
     '{"name":"Refresco","product_id":"00000000-0000-4000-8000-000000000052","category_id":"00000000-0000-4000-8000-000000000042","variant_id":null}',16),
    ('00000000-0000-4000-8000-000000000293','00000000-0000-4000-8000-000000000283','00000000-0000-4000-8000-000000000201',
     '{"name":"Ensalada","product_id":"00000000-0000-4000-8000-000000000053","category_id":"00000000-0000-4000-8000-000000000041","variant_id":null}',8),
    ('00000000-0000-4000-8000-000000000294','00000000-0000-4000-8000-000000000284','00000000-0000-4000-8000-000000000201',
     '{"name":"Refresco","product_id":"00000000-0000-4000-8000-000000000052","category_id":"00000000-0000-4000-8000-000000000042","variant_id":null}',16),
    ('00000000-0000-4000-8000-000000000295','00000000-0000-4000-8000-000000000285','00000000-0000-4000-8000-000000000202',
     '{"name":"Refresco","product_id":"00000000-0000-4000-8000-000000000052","category_id":"00000000-0000-4000-8000-000000000042","variant_id":null}',16),
    ('00000000-0000-4000-8000-000000000296','00000000-0000-4000-8000-000000000286','00000000-0000-4000-8000-000000000203',
     '{"name":"Hamburguesa","product_id":"00000000-0000-4000-8000-000000000051","category_id":"00000000-0000-4000-8000-000000000041","variant_id":null}',16),
    ('00000000-0000-4000-8000-000000000297','00000000-0000-4000-8000-000000000287','00000000-0000-4000-8000-000000000204',
     '{"name":"Refresco","product_id":"00000000-0000-4000-8000-000000000052","category_id":"00000000-0000-4000-8000-000000000042","variant_id":null}',16)
) v(id,item,order_id,line,rate);
INSERT INTO app.restaurant_item_units(id,item_id,version_id,ordinal,account_id,state,subtotal,discount,total,cancelled_at) VALUES
    ('00000000-0000-4000-8000-000000000301','00000000-0000-4000-8000-000000000281','00000000-0000-4000-8000-000000000291',1,
     '00000000-0000-4000-8000-0000000000a1','active',110,10,100,NULL),
    ('00000000-0000-4000-8000-000000000302','00000000-0000-4000-8000-000000000282','00000000-0000-4000-8000-000000000292',1,
     '00000000-0000-4000-8000-0000000000a1','active',33.33,0,33.33,NULL),
    ('00000000-0000-4000-8000-000000000303','00000000-0000-4000-8000-000000000283','00000000-0000-4000-8000-000000000293',1,
     '00000000-0000-4000-8000-0000000000a1','active',50,0,50,NULL),
    ('00000000-0000-4000-8000-000000000304','00000000-0000-4000-8000-000000000284','00000000-0000-4000-8000-000000000294',1,
     '00000000-0000-4000-8000-0000000000a1','cancelled',20,0,20,'2026-11-04T01:30:00Z'),
    ('00000000-0000-4000-8000-000000000305','00000000-0000-4000-8000-000000000285','00000000-0000-4000-8000-000000000295',1,
     '00000000-0000-4000-8000-0000000000a2','active',25,0,25,NULL),
    ('00000000-0000-4000-8000-000000000306','00000000-0000-4000-8000-000000000285','00000000-0000-4000-8000-000000000295',2,
     '00000000-0000-4000-8000-0000000000a2','active',25,0,25,NULL),
    ('00000000-0000-4000-8000-000000000307','00000000-0000-4000-8000-000000000286','00000000-0000-4000-8000-000000000296',1,
     '00000000-0000-4000-8000-0000000000a4','active',120,0,120,NULL),
    ('00000000-0000-4000-8000-000000000308','00000000-0000-4000-8000-000000000287','00000000-0000-4000-8000-000000000297',1,
     '00000000-0000-4000-8000-0000000000a3','active',40,0,40,NULL);
INSERT INTO app.account_adjustments(id,account_id,kind,scope,method,value,amount,courtesy,tax_rate,concept,reason,reason_id,reason_name,
    actor_id,device_id,operation_id,created_at)
SELECT j.id::uuid,j.account::uuid,j.kind,j.scope,j.method,j.value,j.amount,j.courtesy,j.rate,j.concept,'',
    (SELECT id FROM app.discount_reasons WHERE name=j.reason),j.reason_name,'00000000-0000-4000-8000-000000000011',
    '00000000-0000-4000-8000-000000000022',j.operation::uuid,j.created::timestamptz
FROM (VALUES
    ('00000000-0000-4000-8000-0000000000c1','00000000-0000-4000-8000-0000000000a1','discount','items','amount',10.00,10.00,false,NULL::integer,
     'Descuento manual','Atención','Atención (nombre anterior)','00000000-0000-4000-8000-000000000d06','2026-11-04T01:00:00Z'),
    ('00000000-0000-4000-8000-0000000000c2','00000000-0000-4000-8000-0000000000a1','discount','items','amount',5.00,5.00,false,NULL,
     'Descuento manual',NULL,NULL,'00000000-0000-4000-8000-000000000d07','2026-11-04T01:05:00Z'),
    ('00000000-0000-4000-8000-0000000000c3','00000000-0000-4000-8000-0000000000a1','charge','account','amount',12.50,12.50,false,16,
     'Servicio especial',NULL,NULL,'00000000-0000-4000-8000-000000000d09','2026-11-04T01:06:00Z'),
    ('00000000-0000-4000-8000-0000000000c4','00000000-0000-4000-8000-0000000000a1','discount','account','amount',7.77,7.77,false,NULL,
     'Descuento manual',NULL,NULL,'00000000-0000-4000-8000-000000000d10','2026-11-04T01:10:00Z'),
    ('00000000-0000-4000-8000-0000000000c5','00000000-0000-4000-8000-0000000000a1','discount','account','amount',5.01,5.01,false,NULL,
     'Descuento manual',NULL,NULL,'00000000-0000-4000-8000-000000000d11','2026-11-04T01:20:00Z'),
    ('00000000-0000-4000-8000-0000000000c6','00000000-0000-4000-8000-0000000000a2','discount','account','percent',100,50.00,true,NULL,
     'Cortesía','Cortesía de la casa','Cortesía de la casa','00000000-0000-4000-8000-000000000d12','2026-11-04T18:50:00Z')
) j(id,account,kind,scope,method,value,amount,courtesy,rate,concept,reason,reason_name,operation,created);
INSERT INTO app.account_adjustment_allocations(adjustment_id,unit_id,amount) VALUES
    ('00000000-0000-4000-8000-0000000000c1','00000000-0000-4000-8000-000000000301',10.00),
    ('00000000-0000-4000-8000-0000000000c2','00000000-0000-4000-8000-000000000302',5.00);
INSERT INTO app.account_adjustment_voids(adjustment_id,reason,actor_id,device_id,operation_id) VALUES
    ('00000000-0000-4000-8000-0000000000c2','Error de captura','00000000-0000-4000-8000-000000000011',
     '00000000-0000-4000-8000-000000000022','00000000-0000-4000-8000-000000000d08');
INSERT INTO app.payments(id,account_id,shift_id,register_id,method_id,method_name,is_cash,kind,amount,tip,received,change_given,actor_id,
    device_id,operation_id,created_at)
SELECT p.id::uuid,p.account::uuid,'00000000-0000-4000-8000-0000000000e1','00000000-0000-4000-8000-000000000021',m.id,m.name,m.is_cash,
    'general',p.amount,0,p.received,p.change,'00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022',
    p.operation::uuid,p.created::timestamptz
FROM (VALUES ('00000000-0000-4000-8000-000000000401','00000000-0000-4000-8000-0000000000a1','cash',173.05,200.00,26.95,
        '00000000-0000-4000-8000-000000000d13','2026-11-04T02:15:00Z'),
    ('00000000-0000-4000-8000-000000000402','00000000-0000-4000-8000-0000000000a4','debit',120.00,NULL,0,
        '00000000-0000-4000-8000-000000000d14','2026-11-05T18:00:00Z')) p(id,account,method,amount,received,change,operation,created)
JOIN app.payment_methods m ON m.code=p.method;
INSERT INTO app.takeaway_deliveries(account_id,actor_id,device_id,operation_id) VALUES
    ('00000000-0000-4000-8000-0000000000a2','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022',
     '00000000-0000-4000-8000-000000000d15');
INSERT INTO app.audit_events(occurred_at,actor_id,actor_username,actor_kind,device_id,action,entity_type,entity_id,success)
SELECT e.at::timestamptz,'00000000-0000-4000-8000-000000000011','cajera','user','00000000-0000-4000-8000-000000000022',e.action,
    'account',e.account::uuid,true
FROM (VALUES ('2026-11-04T02:15:00Z','payments.account_closed','00000000-0000-4000-8000-0000000000a1'),
    ('2026-11-04T03:00:00Z','payments.account_reopened','00000000-0000-4000-8000-0000000000a1'),
    ('2026-11-04T06:00:00Z','payments.account_closed','00000000-0000-4000-8000-0000000000a1'),
    ('2026-11-04T19:00:00Z','payments.account_closed','00000000-0000-4000-8000-0000000000a2')) e(at,action,account);
`
