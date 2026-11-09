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

// Simula una instalación existente en esquema 6 (con roles y datos ya creados por el asistente)
// y aplica la 0007: los permisos nuevos se conceden solo al administrador, las semillas son solo
// configuración y los registros anteriores conservan NULL en las columnas nuevas. Al terminar deja
// la base vacía para la prueba del ciclo de migraciones.
func TestStage61UpgradeOfExistingInstallation(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(u.Path, "/adel_pos_test_") {
		t.Fatal("integration requires an isolated adel_pos_test_ database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	// La base temporal es exclusiva de este paquete: se parte de un esquema vacío y se deja vacío.
	reset()
	defer reset()
	expected, err := ReadMigrations(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	stageSix := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 6 {
			stageSix[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, stageSix); err != nil || n != 6 {
		t.Fatalf("no se preparó el esquema 6: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.roles(code,name) VALUES ('administrator','Administrador'),('cashier','Cajero');
        INSERT INTO app.catalog_categories(name) VALUES ('Platillos');
        INSERT INTO app.catalog_products(category_id,name,code,kind,base_price)
        SELECT id,'Producto existente','EXISTENTE','product',100 FROM app.catalog_categories`); err != nil {
		t.Fatal(err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-6 {
		t.Fatalf("la 0007 no se aplicó sobre el esquema 6: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0007 no es repetible")
	}
	var granted, cashier int
	if err := pool.QueryRow(ctx, `SELECT
        count(*) FILTER (WHERE r.code='administrator'),count(*) FILTER (WHERE r.code<>'administrator')
        FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id
        WHERE p.permission_key IN ('settings.features.manage','payments.usd_rate.manage')`).Scan(&granted, &cashier); err != nil {
		t.Fatal(err)
	}
	if granted != 2 || cashier != 0 {
		t.Fatalf("los permisos nuevos no se concedieron solo al administrador: %d %d", granted, cashier)
	}
	var enabled bool
	var minimum, maximum, alert string
	var productRate *int
	if err := pool.QueryRow(ctx, `SELECT (SELECT enabled FROM app.establishment_features WHERE code='usd_cash'),
        s.usd_rate_min::text,s.usd_rate_max::text,s.usd_rate_alert_percent::text,
        (SELECT tax_rate FROM app.catalog_products WHERE code='EXISTENTE')
        FROM app.payment_settings s`).Scan(&enabled, &minimum, &maximum, &alert, &productRate); err != nil {
		t.Fatal(err)
	}
	if enabled || minimum != "10.00" || maximum != "40.00" || alert != "10.00" || productRate != nil {
		t.Fatalf("semillas o datos existentes inesperados: %v %s %s %s %v", enabled, minimum, maximum, alert, productRate)
	}
	for _, statement := range []string{
		"UPDATE app.catalog_products SET tax_rate=15",
		"UPDATE app.payment_settings SET usd_rate_min=50",
		"UPDATE app.payment_settings SET usd_rate_alert_percent=101",
		"INSERT INTO app.establishment_features(code) VALUES ('Funcion Invalida')",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó %q", statement)
		}
	}
	checkCurrencyConstraints(ctx, t, pool)
}

// checkCurrencyConstraints comprueba en PostgreSQL que los CHECK de dólares de la 0007 rechazan
// datos a medias: un CHECK que evalúa a NULL se acepta, por lo que cada columna exige IS NOT NULL.
// Las tablas temporales copian solo las restricciones CHECK (no las llaves foráneas), lo que
// permite probar las expresiones sin crear cuentas, turnos ni usuarios.
func checkCurrencyConstraints(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	setup := `CREATE TEMP TABLE payments_check (LIKE app.payments INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE movements_check (LIKE app.cash_movements INCLUDING CONSTRAINTS INCLUDING DEFAULTS)`
	if _, err := connection.Exec(ctx, setup); err != nil {
		t.Fatal(err)
	}
	payment := `INSERT INTO payments_check(id,account_id,shift_id,register_id,method_id,method_name,is_cash,kind,amount,
        received,change_given,received_mxn,received_usd,usd_rate,change_currency,change_usd,actor_id,device_id,operation_id)
        VALUES (gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),'Efectivo',true,'general',100,%s)`
	accepted := []string{
		// Sin dólares: columnas nuevas en NULL (pagos de la etapa 6).
		"100,0,NULL,NULL,NULL,NULL,NULL,gen_random_uuid(),gen_random_uuid(),gen_random_uuid()",
		// $7.50 + US$5 × 18.50 = $100.00, cambio en pesos.
		"100,0,7.50,5,18.50,'MXN',NULL,gen_random_uuid(),gen_random_uuid(),gen_random_uuid()",
		// $100 + US$5 × 18.50 = $192.50, cambio de $92.50 entregado como US$5.00.
		"192.50,92.50,100,5,18.50,'USD',5.00,gen_random_uuid(),gen_random_uuid(),gen_random_uuid()",
	}
	rejected := []string{
		// Dólares sin tipo de cambio ni pesos recibidos.
		"100,0,NULL,5,NULL,'MXN',NULL,gen_random_uuid(),gen_random_uuid(),gen_random_uuid()",
		// Tipo de cambio sin dólares recibidos ni moneda del cambio.
		"100,0,100,NULL,18.50,NULL,NULL,gen_random_uuid(),gen_random_uuid(),gen_random_uuid()",
		// Total recibido que no coincide con la conversión.
		"100,0,10,5,18.50,'MXN',NULL,gen_random_uuid(),gen_random_uuid(),gen_random_uuid()",
		// Cambio en dólares sin el importe en dólares entregado.
		"192.50,92.50,100,5,18.50,'USD',NULL,gen_random_uuid(),gen_random_uuid(),gen_random_uuid()",
	}
	for _, values := range accepted {
		statement := strings.Replace(payment, "%s", values, 1)
		if _, err := connection.Exec(ctx, statement); err != nil {
			t.Fatalf("PostgreSQL rechazó un pago válido (%s): %v", values, err)
		}
	}
	for _, values := range rejected {
		statement := strings.Replace(payment, "%s", values, 1)
		if _, err := connection.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó un pago con datos en dólares incompletos: %s", values)
		}
	}
	movement := `INSERT INTO movements_check(id,shift_id,register_id,kind,method_id,method_name,is_cash,sale_amount,net_amount,
        account_id,payment_id,actor_id,device_id,operation_id,mxn_in,mxn_out,usd_in,usd_out,usd_rate)
        VALUES (gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),'payment',gen_random_uuid(),'Efectivo',true,100,100,
        gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),%s)`
	if _, err := connection.Exec(ctx, strings.Replace(movement, "%s", "7.50,0,5,0,18.50", 1)); err != nil {
		t.Fatalf("PostgreSQL rechazó un movimiento válido por moneda: %v", err)
	}
	for _, values := range []string{"5,NULL,NULL,NULL,NULL", "7.50,0,5,NULL,18.50", "7.50,0,5,0,NULL"} {
		if _, err := connection.Exec(ctx, strings.Replace(movement, "%s", values, 1)); err == nil {
			t.Fatalf("PostgreSQL aceptó un movimiento con cantidades por moneda incompletas: %s", values)
		}
	}
	if _, err := connection.Exec(ctx, "DROP TABLE payments_check, movements_check"); err != nil {
		t.Fatal(err)
	}
}

// Simula una instalación existente en esquema 7 con un turno abierto y aplica la 0008: el turno
// conserva sus datos (las columnas nuevas quedan en NULL), los permisos de caja se conceden solo
// al administrador, las semillas son solo configuración (denominaciones, categorías de gasto y la
// función cash_limit desactivada) y las restricciones nuevas rechazan datos incoherentes. Deja la
// base vacía al terminar.
func TestStage7UpgradeOfExistingInstallation(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(u.Path, "/adel_pos_test_") {
		t.Fatal("integration requires an isolated adel_pos_test_ database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	stageSeven := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 7 {
			stageSeven[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, stageSeven); err != nil || n != 7 {
		t.Fatalf("no se preparó el esquema 7: %d %v", n, err)
	}
	// Datos de una instalación real en esquema 7: roles, una caja, un usuario, una terminal y un
	// turno abierto con tipo de cambio.
	if _, err := pool.Exec(ctx, `INSERT INTO app.roles(code,name) VALUES ('administrator','Administrador'),('cashier','Cajero');
        INSERT INTO app.registers(name) VALUES ('Caja 1');
        INSERT INTO app.users(username,display_name,role_id) SELECT 'cajera','Cajera',id FROM app.roles WHERE code='cashier';
        INSERT INTO app.devices(name,secret_hash,register_id) SELECT 'Terminal 1','hash-de-prueba',id FROM app.registers;
        INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        SELECT '00000000-0000-4000-8000-000000000071',u.id,d.id,'POST payments/shifts','huella','{}' FROM app.users u,app.devices d
        WHERE u.username='cajera';
        INSERT INTO app.cash_shifts(id,register_id,opening_float,opened_by,opened_device_id,operation_id,usd_rate,usd_rate_set_by,usd_rate_set_at)
        SELECT '00000000-0000-4000-8000-000000000072',d.register_id,350,u.id,d.id,'00000000-0000-4000-8000-000000000071',18.25,u.id,now()
        FROM app.users u,app.devices d WHERE u.username='cajera'`); err != nil {
		t.Fatal(err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-7 {
		t.Fatalf("la 0008 no se aplicó sobre el esquema 7: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0008 no es repetible")
	}
	var state, float, rate string
	var floatUSD, businessDate, closingStarted *string
	if err := pool.QueryRow(ctx, `SELECT state,opening_float::text,usd_rate::text,opening_float_usd::text,business_date::text,
        closing_started_at::text FROM app.cash_shifts WHERE id='00000000-0000-4000-8000-000000000072'`).Scan(
		&state, &float, &rate, &floatUSD, &businessDate, &closingStarted); err != nil {
		t.Fatal(err)
	}
	if state != "open" || float != "350.00" || rate != "18.25" || floatUSD != nil || businessDate != nil || closingStarted != nil {
		t.Fatalf("el turno existente cambió al migrar: %s %s %s %v %v %v", state, float, rate, floatUSD, businessDate, closingStarted)
	}
	var granted, others, denominations, categories int
	var cashLimit bool
	var limitMXN *string
	if err := pool.QueryRow(ctx, `SELECT
        (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id
            WHERE r.code='administrator' AND p.permission_key LIKE 'cash.%' AND p.permission_key NOT IN ('cash.shifts.open','cash.shifts.read')),
        (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id
            WHERE r.code<>'administrator' AND p.permission_key LIKE 'cash.%'),
        (SELECT count(*) FROM app.cash_denominations WHERE active),
        (SELECT count(*) FROM app.cash_expense_categories WHERE active),
        (SELECT enabled FROM app.establishment_features WHERE code='cash_limit'),
        (SELECT cash_limit_mxn::text FROM app.payment_settings)`).Scan(&granted, &others, &denominations, &categories, &cashLimit, &limitMXN); err != nil {
		t.Fatal(err)
	}
	if granted != 19 || others != 0 || denominations != 18 || categories != 4 || cashLimit || limitMXN != nil {
		t.Fatalf("permisos o semillas inesperados: %d %d %d %d %v %v", granted, others, denominations, categories, cashLimit, limitMXN)
	}
	var oldIndex, newIndex bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('app.cash_shifts_one_open_idx') IS NOT NULL,
        to_regclass('app.cash_shifts_one_active_idx') IS NOT NULL`).Scan(&oldIndex, &newIndex); err != nil || oldIndex || !newIndex {
		t.Fatalf("el índice de turno activo no sustituyó al de turno abierto: %v %v %v", oldIndex, newIndex, err)
	}
	for _, statement := range []string{
		// Cerrar sin responsable ni terminal, o entrar en arqueo sin quién, cuándo y dónde.
		"UPDATE app.cash_shifts SET state='closed',closed_at=now()",
		"UPDATE app.cash_shifts SET state='closing'",
		"UPDATE app.cash_shifts SET state='suspended'",
		"UPDATE app.cash_shifts SET opening_float_usd=-1",
		"INSERT INTO app.cash_denominations(currency,value,kind) VALUES ('MXN',500,'bill')",
		"INSERT INTO app.cash_denominations(currency,value,kind) VALUES ('EUR',5,'bill')",
		"UPDATE app.payment_settings SET cash_limit_mxn=0",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó %q", statement)
		}
	}
	for _, statement := range []string{
		"INSERT INTO app.realtime_events(id,kind,scope) VALUES (gen_random_uuid(),'cash.shift.opened','cash')",
		"INSERT INTO app.kitchen_tickets(id,kind,station_ids,document) VALUES (gen_random_uuid(),'cash_report','{}','{}')",
		"INSERT INTO app.kitchen_tickets(id,kind,station_ids,document) VALUES (gen_random_uuid(),'cash_drawer','{}','{}')",
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("PostgreSQL rechazó %q: %v", statement, err)
		}
	}
	checkCashConstraints(ctx, t, pool)
}

// checkCashConstraints prueba las expresiones CHECK de las operaciones de efectivo y de los
// cierres con tablas temporales que copian solo esas restricciones (sin llaves foráneas).
func checkCashConstraints(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `CREATE TEMP TABLE operations_check (LIKE app.cash_operations INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE closures_check (LIKE app.cash_shift_closures INCLUDING CONSTRAINTS INCLUDING DEFAULTS)`); err != nil {
		t.Fatal(err)
	}
	actor := "'00000000-0000-4000-8000-0000000000a1'"
	other := "'00000000-0000-4000-8000-0000000000a2'"
	operation := `INSERT INTO operations_check(id,shift_id,register_id,kind,currency,amount,direction,concept,subtype,category_id,reason,
        corrects_operation_id,regularization,exceptional,usd_rate,actor_id,device_id,authorized_by,authorization_mode,operation_id)
        VALUES (gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),%s,` + actor + `,gen_random_uuid(),%s,gen_random_uuid())`
	accepted := []struct{ values, authorization string }{
		{"'income','MXN',10,'in','Ingreso','other',NULL,'',NULL,false,false,NULL", "NULL,NULL"},
		{"'withdrawal','USD',10,'out','Retiro','safekeeping',NULL,'',NULL,false,false,18.50", other + ",'supervisor'"},
		{"'expense','MXN',10,'out','Gasto',NULL,gen_random_uuid(),'',NULL,false,false,NULL", actor + ",'self'"},
		{"'correction','MXN',10,'in','Corrección',NULL,NULL,'Motivo válido',gen_random_uuid(),true,false,NULL", actor + ",'self'"},
		{"'withdrawal','MXN',10,'out','Retiro','other',NULL,'Motivo excepcional',NULL,false,true,NULL", other + ",'supervisor'"},
	}
	rejected := []struct{ values, authorization string }{
		// Retiro sin autorización, autoautorización con otra persona o supervisor que es el actor.
		{"'withdrawal','MXN',10,'out','Retiro','other',NULL,'',NULL,false,false,NULL", "NULL,NULL"},
		{"'withdrawal','MXN',10,'out','Retiro','other',NULL,'',NULL,false,false,NULL", other + ",'self'"},
		{"'withdrawal','MXN',10,'out','Retiro','other',NULL,'',NULL,false,false,NULL", actor + ",'supervisor'"},
		// Ingreso que sale, gasto sin categoría, dólares sin tipo de cambio y pesos con tipo de cambio.
		{"'income','MXN',10,'out','Ingreso','other',NULL,'',NULL,false,false,NULL", "NULL,NULL"},
		{"'expense','MXN',10,'out','Gasto',NULL,NULL,'',NULL,false,false,NULL", actor + ",'self'"},
		{"'income','USD',10,'in','Ingreso','other',NULL,'',NULL,false,false,NULL", "NULL,NULL"},
		{"'income','MXN',10,'in','Ingreso','other',NULL,'',NULL,false,false,18.50", "NULL,NULL"},
		// Corrección sin motivo o sin original, regularización que no es corrección, excepcional sin motivo.
		{"'correction','MXN',10,'in','Corrección',NULL,NULL,'',gen_random_uuid(),false,false,NULL", actor + ",'self'"},
		{"'correction','MXN',10,'in','Corrección',NULL,NULL,'Motivo válido',NULL,false,false,NULL", actor + ",'self'"},
		{"'income','MXN',10,'in','Ingreso','other',NULL,'',NULL,true,false,NULL", "NULL,NULL"},
		{"'withdrawal','MXN',10,'out','Retiro','other',NULL,'',NULL,false,true,NULL", other + ",'supervisor'"},
		{"'income','MXN',0,'in','Ingreso','other',NULL,'',NULL,false,false,NULL", "NULL,NULL"},
	}
	build := func(values, authorization string) string {
		return strings.Replace(strings.Replace(operation, "%s", values, 1), "%s", authorization, 1)
	}
	for _, item := range accepted {
		if _, err := connection.Exec(ctx, build(item.values, item.authorization)); err != nil {
			t.Fatalf("PostgreSQL rechazó una operación válida (%s): %v", item.values, err)
		}
	}
	for _, item := range rejected {
		if _, err := connection.Exec(ctx, build(item.values, item.authorization)); err == nil {
			t.Fatalf("PostgreSQL aceptó una operación incoherente: %s / %s", item.values, item.authorization)
		}
	}
	closure := `INSERT INTO closures_check(shift_id,count_id,final_report_id,expected_mxn,expected_usd,counted_mxn,counted_usd,
        difference_mxn,difference_usd,usd_rate,difference_equivalent_mxn,result_mxn,result_usd,closed_by,device_id,operation_id)
        VALUES (gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),%s,gen_random_uuid(),gen_random_uuid(),gen_random_uuid())`
	validClosures := []string{
		"5000,0,4850,0,-150,0,NULL,NULL,'short','even'",
		"1015,57.30,1035.50,47.30,20.50,-10,18.50,-164.50,'over','short'",
	}
	invalidClosures := []string{
		// Diferencia que no es contado − esperado, resultado incoherente y equivalencia sin tipo de cambio.
		"5000,0,4850,0,0,0,NULL,NULL,'even','even'",
		"5000,0,4850,0,-150,0,NULL,NULL,'over','even'",
		"1015,57.30,1035.50,47.30,20.50,-10,NULL,-164.50,'over','short'",
		"1015,57.30,1035.50,47.30,20.50,-10,18.50,20.50,'over','short'",
	}
	for _, values := range validClosures {
		if _, err := connection.Exec(ctx, strings.Replace(closure, "%s", values, 1)); err != nil {
			t.Fatalf("PostgreSQL rechazó un cierre válido (%s): %v", values, err)
		}
	}
	for _, values := range invalidClosures {
		if _, err := connection.Exec(ctx, strings.Replace(closure, "%s", values, 1)); err == nil {
			t.Fatalf("PostgreSQL aceptó un cierre incoherente: %s", values)
		}
	}
	if _, err := connection.Exec(ctx, "DROP TABLE operations_check, closures_check"); err != nil {
		t.Fatal(err)
	}
	checkPrintingConstraints(ctx, t, connection)
}

// checkPrintingConstraints prueba los CHECK de los documentos de caja y de los comandos del cajón
// (fase C) con tablas temporales que copian solo esas restricciones.
func checkPrintingConstraints(ctx context.Context, t *testing.T, connection *pgxpool.Conn) {
	t.Helper()
	if _, err := connection.Exec(ctx, `CREATE TEMP TABLE documents_check (LIKE app.cash_documents INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE drawer_check (LIKE app.cash_drawer_events INCLUDING CONSTRAINTS INCLUDING DEFAULTS)`); err != nil {
		t.Fatal(err)
	}
	document := `INSERT INTO documents_check(id,ticket_id,shift_id,report_id,cash_operation_id,kind,includes_expected,created_by,device_id,operation_id)
        VALUES (gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),%s,gen_random_uuid(),gen_random_uuid(),gen_random_uuid())`
	drawer := `INSERT INTO drawer_check(id,register_id,reason,notes,actor_id,device_id,job_id,operation_id)
        VALUES (gen_random_uuid(),gen_random_uuid(),%s,gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),gen_random_uuid())`
	accepted := []string{
		strings.Replace(document, "%s", "gen_random_uuid(),NULL,'partial_cut',true", 1),
		strings.Replace(document, "%s", "NULL,gen_random_uuid(),'withdrawal',false", 1),
		strings.Replace(drawer, "%s", "'refund',''", 1),
		strings.Replace(drawer, "%s", "'manual','Cambio para el fondo'", 1),
	}
	rejected := []string{
		// Un comprobante nunca contiene el esperado; un corte exige su referencia.
		strings.Replace(document, "%s", "NULL,gen_random_uuid(),'income',true", 1),
		strings.Replace(document, "%s", "NULL,gen_random_uuid(),'final_cut',false", 1),
		// Motivo desconocido y apertura manual sin explicación.
		strings.Replace(drawer, "%s", "'unknown',''", 1),
		strings.Replace(drawer, "%s", "'manual','No'", 1),
	}
	for _, statement := range accepted {
		if _, err := connection.Exec(ctx, statement); err != nil {
			t.Fatalf("PostgreSQL rechazó un registro válido (%s): %v", statement, err)
		}
	}
	for _, statement := range rejected {
		if _, err := connection.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó un registro incoherente: %s", statement)
		}
	}
	if _, err := connection.Exec(ctx, "DROP TABLE documents_check, drawer_check"); err != nil {
		t.Fatal(err)
	}
}
