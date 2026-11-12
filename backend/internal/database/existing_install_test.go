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

// Simula una instalación existente en esquema 8 (0.7.0) con un turno abierto, un cajón configurado
// y un comando de apertura manual ya registrado, y aplica la 0009 (etapa 7.1): el comando anterior
// se conserva, el CHECK de motivos del cajón conserva su nombre y admite test, y el índice parcial
// de rechazos por efectivo insuficiente existe. Deja la base vacía al terminar.
func TestStage71UpgradeOfExistingInstallation(t *testing.T) {
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
	stageEight := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 8 {
			stageEight[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, stageEight); err != nil || n != 8 {
		t.Fatalf("no se preparó el esquema 8: %d %v", n, err)
	}
	// Datos de una instalación real en esquema 8: caja, cajera, terminal, turno abierto, cajón
	// configurado con una impresora ESC/POS y un comando de apertura manual en la cola.
	if _, err := pool.Exec(ctx, `INSERT INTO app.roles(code,name) VALUES ('administrator','Administrador'),('cashier','Cajero');
        INSERT INTO app.registers(name) VALUES ('Caja 1');
        INSERT INTO app.users(username,display_name,role_id) SELECT 'cajera','Cajera',id FROM app.roles WHERE code='cashier';
        INSERT INTO app.devices(name,secret_hash,register_id) SELECT 'Terminal 1','hash-de-prueba',id FROM app.registers;
        INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        SELECT op.id::uuid,u.id,d.id,op.route,'huella','{}' FROM app.users u,app.devices d,
        (VALUES ('00000000-0000-4000-8000-000000000081','POST payments/shifts'),
                ('00000000-0000-4000-8000-000000000082','POST payments/cash/open-drawer'),
                ('00000000-0000-4000-8000-000000000088','POST payments/accounts/descuento')) AS op(id,route)
        WHERE u.username='cajera';
        INSERT INTO app.cash_shifts(id,register_id,opening_float,opened_by,opened_device_id,operation_id,business_date)
        SELECT '00000000-0000-4000-8000-000000000083',d.register_id,500,u.id,d.id,'00000000-0000-4000-8000-000000000081',current_date
        FROM app.users u,app.devices d WHERE u.username='cajera';
        INSERT INTO app.kitchen_printers(id,name,transport,destination,port)
        VALUES ('00000000-0000-4000-8000-000000000084','Tickets de caja','escpos_tcp','192.168.1.80',9100);
        INSERT INTO app.cash_drawer_settings(register_id,printer_id,pulse_pin,enabled)
        SELECT id,'00000000-0000-4000-8000-000000000084',0,true FROM app.registers;
        INSERT INTO app.kitchen_tickets(id,kind,station_ids,document)
        VALUES ('00000000-0000-4000-8000-000000000085','cash_drawer','{}','{"drawer_pulse":{"pin":0}}');
        INSERT INTO app.kitchen_print_jobs(id,ticket_id,printer_id,printer_snapshot,content,state)
        VALUES ('00000000-0000-4000-8000-000000000086','00000000-0000-4000-8000-000000000085',
            '00000000-0000-4000-8000-000000000084','{}','Comando de apertura del cajón de Caja 1','submitted');
        INSERT INTO app.cash_drawer_events(id,register_id,shift_id,reason,notes,actor_id,device_id,job_id,operation_id)
        SELECT '00000000-0000-4000-8000-000000000087',d.register_id,'00000000-0000-4000-8000-000000000083','manual',
            'Cambio de billetes para el fondo',u.id,d.id,'00000000-0000-4000-8000-000000000086','00000000-0000-4000-8000-000000000082'
        FROM app.users u,app.devices d WHERE u.username='cajera';
        INSERT INTO app.catalog_price_lists(name,fallback) VALUES ('Lista de la etapa 7','base');
        INSERT INTO app.catalog_categories(id,name) VALUES ('00000000-0000-4000-8000-00000000008b','Hamburguesas');
        INSERT INTO app.restaurant_accounts(id,mode,name,price_list_id,opened_by,assigned_user_id,device_id)
        SELECT '00000000-0000-4000-8000-000000000089','dine_in','Mesa 4',l.id,u.id,u.id,d.id
        FROM app.catalog_price_lists l,app.users u,app.devices d WHERE u.username='cajera' AND l.name='Lista de la etapa 7';
        INSERT INTO app.account_adjustments(id,account_id,kind,scope,method,value,amount,concept,reason,actor_id,device_id,operation_id)
        SELECT '00000000-0000-4000-8000-00000000008a','00000000-0000-4000-8000-000000000089','discount','account','percent',10,27,
            'Descuento manual','Cliente frecuente de la etapa 6',u.id,d.id,'00000000-0000-4000-8000-000000000088'
        FROM app.users u,app.devices d WHERE u.username='cajera'`); err != nil {
		t.Fatal(err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-8 {
		t.Fatalf("la 0009 no se aplicó sobre el esquema 8: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0009 no es repetible")
	}
	var reason, notes, shiftState string
	if err := pool.QueryRow(ctx, `SELECT e.reason,e.notes,s.state FROM app.cash_drawer_events e JOIN app.cash_shifts s ON s.id=e.shift_id
        WHERE e.id='00000000-0000-4000-8000-000000000087'`).Scan(&reason, &notes, &shiftState); err != nil {
		t.Fatal(err)
	}
	if reason != "manual" || notes != "Cambio de billetes para el fondo" || shiftState != "open" {
		t.Fatalf("el comando o el turno existentes cambiaron al migrar: %s %s %s", reason, notes, shiftState)
	}
	var definitions []string
	rows, err := pool.Query(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
        WHERE conrelid='app.cash_drawer_events'::regclass AND conname='cash_drawer_events_reason_check'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var definition string
		if err := rows.Scan(&definition); err != nil {
			t.Fatal(err)
		}
		definitions = append(definitions, definition)
	}
	rows.Close()
	if len(definitions) != 1 || !strings.Contains(definitions[0], "'test'") || !strings.Contains(definitions[0], "'manual'") {
		t.Fatalf("el CHECK de motivos del cajón no conserva su nombre con test: %v", definitions)
	}
	var indexDefinition string
	if err := pool.QueryRow(ctx, `SELECT pg_get_indexdef('app.audit_events_cash_outflow_rejected_idx'::regclass)`).Scan(&indexDefinition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(indexDefinition, "WHERE") || !strings.Contains(indexDefinition, "cash.outflow_rejected") {
		t.Fatalf("el índice de rechazos no es parcial: %s", indexDefinition)
	}
	// La restricción nueva admite test y sigue rechazando motivos desconocidos o una apertura
	// manual sin explicación (tabla temporal que copia solo los CHECK).
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, "CREATE TEMP TABLE drawer_reason_check (LIKE app.cash_drawer_events INCLUDING CONSTRAINTS INCLUDING DEFAULTS)"); err != nil {
		t.Fatal(err)
	}
	drawer := `INSERT INTO drawer_reason_check(id,register_id,reason,notes,actor_id,device_id,job_id,operation_id)
        VALUES (gen_random_uuid(),gen_random_uuid(),%s,gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),gen_random_uuid())`
	for _, values := range []string{"'test','Prueba de apertura desde la configuración del cajón'", "'test',''", "'payment',''"} {
		if _, err := connection.Exec(ctx, strings.Replace(drawer, "%s", values, 1)); err != nil {
			t.Fatalf("PostgreSQL rechazó un comando válido (%s): %v", values, err)
		}
	}
	for _, values := range []string{"'unknown',''", "'manual','No'", "'Test',''"} {
		if _, err := connection.Exec(ctx, strings.Replace(drawer, "%s", values, 1)); err == nil {
			t.Fatalf("PostgreSQL aceptó un comando incoherente: %s", values)
		}
	}
	if _, err := connection.Exec(ctx, "DROP TABLE drawer_reason_check"); err != nil {
		t.Fatal(err)
	}
	checkStage71PhaseD(ctx, t, pool, connection)
}

// checkStage71PhaseD comprueba la fase D de la 0009 sobre la instalación de esquema 8: el descuento
// histórico queda sin motivo y sin cambios, el catálogo de motivos se siembra, las restricciones
// tienen nombre explícito y los CHECK no evalúan a NULL.
func checkStage71PhaseD(ctx context.Context, t *testing.T, pool *pgxpool.Pool, connection *pgxpool.Conn) {
	t.Helper()
	var reasonID *string
	var reason, amount string
	if err := pool.QueryRow(ctx, `SELECT reason_id::text,reason,amount::text FROM app.account_adjustments
        WHERE id='00000000-0000-4000-8000-00000000008a'`).Scan(&reasonID, &reason, &amount); err != nil {
		t.Fatal(err)
	}
	if reasonID != nil || reason != "Cliente frecuente de la etapa 6" || amount != "27.00" {
		t.Fatalf("El descuento histórico cambió al migrar: %v %s %s", reasonID, reason, amount)
	}
	var seeded string
	if err := pool.QueryRow(ctx, `SELECT string_agg(name||':'||requires_description||':'||active||':'||sort_order,'|' ORDER BY sort_order)
        FROM app.discount_reasons`).Scan(&seeded); err != nil {
		t.Fatal(err)
	}
	if seeded != "Atención:false:true:10|Promoción:false:true:20|Demora:false:true:30|Cortesía de la casa:false:true:40|Otro:true:true:50" {
		t.Fatalf("Siembra de motivos de descuento: %s", seeded)
	}
	var named int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname IN (
            'discount_reasons_name_check','discount_reasons_name_key','discount_reasons_sort_order_check','discount_reasons_revision_check',
            'account_adjustments_reason_id_fkey','account_adjustments_reason_kind_check','account_adjustments_reason_name_check',
            'catalog_category_quick_notes_category_id_fkey','catalog_category_quick_notes_text_check',
            'catalog_category_quick_notes_sort_order_check','catalog_category_quick_notes_revision_check')`).Scan(&named); err != nil {
		t.Fatal(err)
	}
	if named != 11 {
		t.Fatalf("Faltan restricciones con nombre explícito en la fase D: %d de 11", named)
	}
	var uniqueIndex bool
	if err := pool.QueryRow(ctx, `SELECT indisunique FROM pg_index WHERE indexrelid='app.catalog_category_quick_notes_text_key'::regclass`).Scan(&uniqueIndex); err != nil || !uniqueIndex {
		t.Fatalf("El índice único de indicaciones no existe: %v", err)
	}
	// CHECK con tablas temporales que copian solo las restricciones (sin llaves foráneas).
	if _, err := connection.Exec(ctx, `CREATE TEMP TABLE reasons_check (LIKE app.discount_reasons INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE quick_notes_check (LIKE app.catalog_category_quick_notes INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE adjustments_check (LIKE app.account_adjustments INCLUDING CONSTRAINTS INCLUDING DEFAULTS)`); err != nil {
		t.Fatal(err)
	}
	adjustment := `INSERT INTO adjustments_check(id,account_id,kind,scope,method,value,amount,tax_rate,concept,actor_id,device_id,operation_id,reason_id,reason_name)
        VALUES (gen_random_uuid(),gen_random_uuid(),%s,gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),%s,%s)`
	build := func(statement string, values ...string) string {
		for _, value := range values {
			statement = strings.Replace(statement, "%s", value, 1)
		}
		return statement
	}
	accepted := []string{
		"INSERT INTO reasons_check(name) VALUES ('Atención')",
		"INSERT INTO reasons_check(name,requires_description) VALUES ('Otro motivo',true)",
		"INSERT INTO quick_notes_check(category_id,text) VALUES (gen_random_uuid(),'Sin cebolla')",
		"INSERT INTO quick_notes_check(category_id,text) VALUES (gen_random_uuid(),'Término ½ · sin sal')",
		build(adjustment, "'discount','account','percent',10,5,NULL,'Descuento'", "gen_random_uuid()", "'Atención'"),
		build(adjustment, "'discount','account','percent',10,5,NULL,'Descuento'", "NULL", "NULL"),
		build(adjustment, "'charge','account','amount',30,30,16,'Servicio'", "NULL", "NULL"),
	}
	rejected := []string{
		"INSERT INTO reasons_check(name) VALUES ('')",
		"INSERT INTO reasons_check(name) VALUES (' Atención')",
		"INSERT INTO reasons_check(name) VALUES ('" + strings.Repeat("m", 61) + "')",
		"INSERT INTO reasons_check(name,sort_order) VALUES ('Orden negativo',-10)",
		"INSERT INTO quick_notes_check(category_id,text) VALUES (gen_random_uuid(),'Sin cebolla, sin sal')",
		"INSERT INTO quick_notes_check(category_id,text) VALUES (gen_random_uuid(),'')",
		"INSERT INTO quick_notes_check(category_id,text) VALUES (gen_random_uuid(),'Sin sal ')",
		"INSERT INTO quick_notes_check(category_id,text) VALUES (gen_random_uuid(),'" + strings.Repeat("a", 81) + "')",
		"INSERT INTO quick_notes_check(category_id,text,revision) VALUES (gen_random_uuid(),'Sin hielo',0)",
		// Un cargo nunca lleva motivo de descuento.
		build(adjustment, "'charge','account','amount',30,30,16,'Servicio'", "gen_random_uuid()", "'Atención'"),
		// El nombre guardado acompaña siempre al motivo, y solo a él.
		build(adjustment, "'discount','account','percent',10,5,NULL,'Descuento'", "gen_random_uuid()", "NULL"),
		build(adjustment, "'discount','account','percent',10,5,NULL,'Descuento'", "NULL", "'Atención'"),
		build(adjustment, "'discount','account','percent',10,5,NULL,'Descuento'", "gen_random_uuid()", "''"),
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
	if _, err := connection.Exec(ctx, "DROP TABLE reasons_check, quick_notes_check, adjustments_check"); err != nil {
		t.Fatal(err)
	}
	// Llaves foráneas e índice único sobre las tablas reales: categoría inexistente y texto que solo
	// difiere en mayúsculas dentro de la misma categoría.
	category := "'00000000-0000-4000-8000-00000000008b'"
	if _, err := pool.Exec(ctx, "INSERT INTO app.catalog_category_quick_notes(category_id,text) VALUES ("+category+",'Sin cebolla')"); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"INSERT INTO app.catalog_category_quick_notes(category_id,text) VALUES (" + category + ",'SIN CEBOLLA')",
		"INSERT INTO app.catalog_category_quick_notes(category_id,text) VALUES (gen_random_uuid(),'Sin hielo')",
		"UPDATE app.account_adjustments SET reason_id=gen_random_uuid() WHERE id='00000000-0000-4000-8000-00000000008a'",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó una referencia o duplicado inválido: %s", statement)
		}
	}
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

// inventoryConstraints son las restricciones con nombre explícito de la 0010 (etapa 8, fase A).
var inventoryConstraints = []string{
	"inventory_settings_singleton_check", "inventory_settings_count_frequency_check", "inventory_settings_revision_check",
	"inventory_settings_updated_by_fkey", "inventory_settings_count_interval_check",
	"inventory_units_code_check", "inventory_units_name_check", "inventory_units_dimension_check", "inventory_units_factor_check",
	"inventory_units_revision_check", "inventory_units_code_key", "inventory_units_system_active_check",
	"inventory_categories_name_check", "inventory_categories_sort_order_check", "inventory_categories_revision_check",
	"inventory_items_code_check", "inventory_items_barcode_check", "inventory_items_name_check", "inventory_items_description_check",
	"inventory_items_category_id_fkey", "inventory_items_base_unit_id_fkey", "inventory_items_kind_check",
	"inventory_items_reference_cost_check", "inventory_items_min_stock_check", "inventory_items_reorder_point_check",
	"inventory_items_revision_check", "inventory_items_created_by_fkey", "inventory_items_updated_by_fkey",
	"inventory_presentations_item_id_fkey", "inventory_presentations_name_check", "inventory_presentations_base_quantity_check",
	"inventory_presentations_barcode_check", "inventory_presentations_revision_check", "inventory_presentations_default_active_check",
	"inventory_warehouses_name_check", "inventory_warehouses_description_check", "inventory_warehouses_revision_check",
	"inventory_warehouses_default_active_check",
	"inventory_station_warehouses_station_id_fkey", "inventory_station_warehouses_warehouse_id_fkey",
	"inventory_station_warehouses_actor_id_fkey", "inventory_station_warehouses_operation_id_fkey", "inventory_station_warehouses_seq_key",
	"inventory_stock_item_id_fkey", "inventory_stock_warehouse_id_fkey", "inventory_stock_pkey",
	"inventory_valuations_item_id_fkey", "inventory_valuations_last_valid_cost_check", "inventory_valuations_pending_cost_quantity_check",
	"inventory_valuations_zero_check",
	"inventory_movements_item_id_fkey", "inventory_movements_warehouse_id_fkey", "inventory_movements_kind_check",
	"inventory_movements_unit_cost_check", "inventory_movements_cost_basis_check", "inventory_movements_source_kind_check",
	"inventory_movements_source_line_check", "inventory_movements_restaurant_unit_id_fkey",
	"inventory_movements_related_movement_id_fkey", "inventory_movements_reason_check", "inventory_movements_actor_id_fkey",
	"inventory_movements_process_check", "inventory_movements_operation_id_fkey", "inventory_movements_entry_key_check",
	"inventory_movements_seq_key", "inventory_movements_entry_key_key", "inventory_movements_responsible_check",
	"inventory_movements_shape_check", "inventory_movements_sign_check", "inventory_movements_pending_check",
}

// Simula una instalación existente en esquema 9 (0.7.1) con datos de las etapas anteriores y aplica
// las migraciones de la etapa 8 que existan (0010 en adelante; fases A, B, C y D): nada existente
// cambia (funciones, permisos de otros roles, gastos de caja con y sin corrección, eventos y
// estaciones), las migraciones son repetibles según el migrador (una segunda ejecución no aplica
// nada), las siembras son solo de referencia (la 0011, la 0013 y la 0014 no siembran nada), los permisos de
// inventario se conceden solo al administrador y las restricciones nuevas tienen nombre y rechazan
// datos incoherentes. Deja la base vacía al terminar.
func TestStage8UpgradeOfExistingInstallation(t *testing.T) {
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
	stageNine := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 9 {
			stageNine[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, stageNine); err != nil || n != 9 {
		t.Fatalf("no se preparó el esquema 9: %d %v", n, err)
	}
	// Datos de una instalación real en esquema 9: roles (incluido «Inventarios» sin permisos), el
	// administrador con todos los permisos, una cajera con los suyos, caja, terminal, estación de
	// cocina, pago en dólares activo, un turno abierto con un gasto corregido y otro vigente y un
	// evento de caja en el outbox.
	if _, err := pool.Exec(ctx, `INSERT INTO app.roles(code,name) VALUES ('administrator','Administrador'),('cashier','Cajero'),
            ('inventory','Inventarios');
        INSERT INTO app.role_permissions(role_id,permission_key) SELECT r.id,p.key FROM app.roles r,app.permissions p
            WHERE r.code='administrator';
        INSERT INTO app.role_permissions(role_id,permission_key) SELECT r.id,p.key FROM app.roles r,app.permissions p
            WHERE r.code='cashier' AND p.key IN ('cash.shifts.open','cash.expenses.record','payments.collect');
        INSERT INTO app.registers(name) VALUES ('Caja 1');
        INSERT INTO app.users(username,display_name,role_id) SELECT 'cajera','Cajera',id FROM app.roles WHERE code='cashier';
        INSERT INTO app.devices(name,secret_hash,register_id) SELECT 'Terminal 1','hash-de-prueba',id FROM app.registers;
        INSERT INTO app.kitchen_stations(id,name,reception) VALUES ('00000000-0000-4000-8000-000000000090','Cocina caliente','kds');
        UPDATE app.establishment_features SET enabled=true,revision=2 WHERE code='usd_cash';
        INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        SELECT op.id::uuid,u.id,d.id,op.route,'huella','{}' FROM app.users u,app.devices d,
        (VALUES ('00000000-0000-4000-8000-000000000091','POST payments/shifts'),
                ('00000000-0000-4000-8000-000000000092','POST payments/shifts/gasto'),
                ('00000000-0000-4000-8000-000000000093','POST payments/shifts/correccion'),
                ('00000000-0000-4000-8000-000000000094','POST payments/shifts/gasto')) AS op(id,route)
        WHERE u.username='cajera';
        INSERT INTO app.cash_shifts(id,register_id,opening_float,opened_by,opened_device_id,operation_id,business_date)
        SELECT '00000000-0000-4000-8000-000000000095',d.register_id,500,u.id,d.id,'00000000-0000-4000-8000-000000000091',current_date
        FROM app.users u,app.devices d WHERE u.username='cajera';
        INSERT INTO app.cash_operations(id,shift_id,register_id,kind,currency,amount,direction,concept,category_id,actor_id,device_id,
            authorized_by,authorization_mode,operation_id)
        SELECT op.id::uuid,'00000000-0000-4000-8000-000000000095',d.register_id,'expense','MXN',op.amount,'out','Insumos',c.id,u.id,d.id,
            u.id,'self',op.operation::uuid
        FROM app.users u,app.devices d,app.cash_expense_categories c,
        (VALUES ('00000000-0000-4000-8000-000000000096',250.00,'00000000-0000-4000-8000-000000000092'),
                ('00000000-0000-4000-8000-000000000097',180.50,'00000000-0000-4000-8000-000000000094')) AS op(id,amount,operation)
        WHERE u.username='cajera' AND c.name='Insumos';
        INSERT INTO app.cash_operations(id,shift_id,register_id,kind,currency,amount,direction,concept,reason,corrects_operation_id,
            actor_id,device_id,authorized_by,authorization_mode,operation_id)
        SELECT '00000000-0000-4000-8000-000000000098','00000000-0000-4000-8000-000000000095',d.register_id,'correction','MXN',250,'in',
            'Corrección','Gasto capturado dos veces','00000000-0000-4000-8000-000000000096',u.id,d.id,u.id,'self',
            '00000000-0000-4000-8000-000000000093'
        FROM app.users u,app.devices d WHERE u.username='cajera';
        INSERT INTO app.realtime_events(id,kind,scope,aggregate_id)
        VALUES ('00000000-0000-4000-8000-000000000099','cash.operation.recorded','cash','00000000-0000-4000-8000-000000000095')`); err != nil {
		t.Fatal(err)
	}
	var permissionsBefore, cashierBefore int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.permissions),
        (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE r.code='cashier')`).
		Scan(&permissionsBefore, &cashierBefore); err != nil {
		t.Fatal(err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-9 {
		t.Fatalf("las migraciones de la etapa 8 (0010 en adelante) no se aplicaron sobre el esquema 9: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("las migraciones de la etapa 8 no son repetibles")
	}
	checkStage8ExistingData(ctx, t, pool, permissionsBefore, cashierBefore)
	checkStage8Seeds(ctx, t, pool)
	checkStage8Constraints(ctx, t, pool)
	checkStage8PhaseB(ctx, t, pool)
	checkStage8PhaseC(ctx, t, pool)
	checkStage8PhaseD(ctx, t, pool)
}

// checkStage8ExistingData comprueba que la 0010 no alteró los datos de las etapas anteriores.
func checkStage8ExistingData(ctx context.Context, t *testing.T, pool *pgxpool.Pool, permissionsBefore, cashierBefore int) {
	t.Helper()
	var usdEnabled bool
	var usdRevision int
	var usdEnabledAt *time.Time
	if err := pool.QueryRow(ctx, "SELECT enabled,revision,enabled_at FROM app.establishment_features WHERE code='usd_cash'").
		Scan(&usdEnabled, &usdRevision, &usdEnabledAt); err != nil {
		t.Fatal(err)
	}
	if !usdEnabled || usdRevision != 2 || usdEnabledAt != nil {
		t.Fatalf("La función existente cambió al migrar: %v %d %v", usdEnabled, usdRevision, usdEnabledAt)
	}
	var operations string
	if err := pool.QueryRow(ctx, `SELECT string_agg(kind||':'||amount::text||':'||coalesce(corrects_operation_id::text,'-'),'|' ORDER BY id)
        FROM app.cash_operations`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != "expense:250.00:-|expense:180.50:-|correction:250.00:00000000-0000-4000-8000-000000000096" {
		t.Fatalf("Los gastos de caja cambiaron al migrar: %s", operations)
	}
	var permissionsAfter, cashierAfter, inventoryRole, events int
	// Los permisos reports.* de la 0017 (etapa 9A) se comprueban en stage9a_reports_upgrade_test.go, los
	// documents.* de la 0018 (etapa 9B), en stage9b_templates_upgrade_test.go y accounts.cancel de la
	// 0021 (etapa 9D), en stage9d_cancellation_upgrade_test.go.
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.permissions WHERE key NOT LIKE 'reports.%' AND key NOT LIKE 'documents.%'
            AND key<>'accounts.cancel'),
        (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE r.code='cashier'),
        (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE r.code='inventory'),
        (SELECT count(*) FROM app.realtime_events WHERE scope='cash')`).Scan(&permissionsAfter, &cashierAfter, &inventoryRole, &events); err != nil {
		t.Fatal(err)
	}
	// 10 permisos de la 0010 (fase A), 4 de la 0011 (fase B), 6 de la 0013 (fase C) y 4 de la 0014 (fase D).
	if permissionsAfter != permissionsBefore+24 || cashierAfter != cashierBefore || inventoryRole != 0 || events != 1 {
		t.Fatalf("Permisos o eventos existentes: %d→%d, cajera %d→%d, inventarios %d, eventos %d",
			permissionsBefore, permissionsAfter, cashierBefore, cashierAfter, inventoryRole, events)
	}
}

// checkStage8Seeds comprueba las siembras de referencia y la concesión de permisos de la 0010.
func checkStage8Seeds(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var enabled bool
	var enabledAt *time.Time
	var units, warehouses, items, movements, settings, administrator, others int
	err := pool.QueryRow(ctx, `SELECT (SELECT enabled FROM app.establishment_features WHERE code='inventory_control'),
            (SELECT enabled_at FROM app.establishment_features WHERE code='inventory_control'),
            (SELECT count(*) FROM app.inventory_units WHERE system AND active),
            (SELECT count(*) FROM app.inventory_warehouses WHERE is_default AND active AND name='Almacén general'),
            (SELECT count(*) FROM app.inventory_items),
            (SELECT count(*) FROM app.inventory_movements),
            (SELECT count(*) FROM app.inventory_settings WHERE purchase_tax_in_cost AND operating_day_cutoff='05:00'
                AND warn_products_without_recipe AND count_frequency='none' AND count_interval_days IS NULL),
            (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id
                WHERE r.code='administrator' AND p.permission_key LIKE 'inventory.%'),
            (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id
                WHERE r.code<>'administrator' AND p.permission_key LIKE 'inventory.%')`).
		Scan(&enabled, &enabledAt, &units, &warehouses, &items, &movements, &settings, &administrator, &others)
	if err != nil {
		t.Fatal(err)
	}
	if enabled || enabledAt != nil || units != 6 || warehouses != 1 || items != 0 || movements != 0 || settings != 1 ||
		administrator != 24 || others != 0 {
		t.Fatalf("Siembras de la 0010: %v %v %d %d %d %d %d %d %d", enabled, enabledAt, units, warehouses, items, movements, settings,
			administrator, others)
	}
	// El ámbito de eventos inventory se agregó conservando los anteriores.
	for _, scope := range []string{"restaurant", "takeaway", "kitchen", "printing", "configuration", "cash", "inventory"} {
		if _, err := pool.Exec(ctx, "INSERT INTO app.realtime_events(id,kind,scope) VALUES (gen_random_uuid(),'prueba.ambito',$1)", scope); err != nil {
			t.Fatalf("El ámbito %s no se admite: %v", scope, err)
		}
	}
	if _, err := pool.Exec(ctx, "INSERT INTO app.realtime_events(id,kind,scope) VALUES (gen_random_uuid(),'prueba.ambito','reports')"); err == nil {
		t.Fatal("Se aceptó un ámbito de eventos desconocido")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM app.realtime_events WHERE kind='prueba.ambito'"); err != nil {
		t.Fatal(err)
	}
}

// checkStage8Constraints comprueba nombres, CHECK seguros ante NULL y el libro de solo inserción.
func checkStage8Constraints(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var named int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_constraint WHERE conname = ANY($1)", inventoryConstraints).Scan(&named); err != nil {
		t.Fatal(err)
	}
	if named != len(inventoryConstraints) {
		t.Fatalf("Faltan restricciones con nombre explícito en la 0010: %d de %d", named, len(inventoryConstraints))
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `CREATE TEMP TABLE movements_check (LIKE app.inventory_movements INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE settings_check (LIKE app.inventory_settings INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE valuations_check (LIKE app.inventory_valuations INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE units_check (LIKE app.inventory_units INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE items_check (LIKE app.inventory_items INCLUDING CONSTRAINTS INCLUDING DEFAULTS)`); err != nil {
		t.Fatal(err)
	}
	movement := `INSERT INTO movements_check(id,seq,item_id,warehouse_id,kind,quantity,value,unit_cost,cost_basis,cost_pending,occurred_at,
        source_kind,actor_id,process,entry_key,stock_after,valuation_quantity_after,valuation_value_after)
        VALUES (gen_random_uuid(),1,gen_random_uuid(),%s,now(),'initial',%s,'clave',%s,0,0)`
	build := func(values ...string) string {
		statement := movement
		for _, value := range values {
			statement = strings.Replace(statement, "%s", value, 1)
		}
		return statement
	}
	warehouse := "gen_random_uuid()"
	actor := "gen_random_uuid(),NULL"
	accepted := []string{
		build(warehouse+",'initial',10,200,20,'document',false", actor, "10"),
		build(warehouse+",'consumption',-2,-40,20,'average',false", "NULL,'inventory_worker'", "8"),
		build(warehouse+",'consumption',-1,0,NULL,'pending',true", "NULL,'inventory_worker'", "-1"),
		build("NULL,'cost_adjustment',0,-10,NULL,'difference',false", actor, "NULL"),
	}
	rejected := []string{
		// Una entrada con cantidad negativa o una salida positiva.
		build(warehouse+",'initial',-10,200,20,'document',false", actor, "10"),
		build(warehouse+",'consumption',2,-40,20,'average',false", actor, "8"),
		// Un movimiento de existencias sin almacén, en cero o sin existencia posterior.
		build("NULL,'initial',10,200,20,'document',false", actor, "10"),
		build(warehouse+",'adjustment_in',0,0,NULL,'average',false", actor, "0"),
		build(warehouse+",'initial',10,200,20,'document',false", actor, "NULL"),
		// Un ajuste de costo con almacén o con cantidad.
		build(warehouse+",'cost_adjustment',0,-10,NULL,'difference',false", actor, "NULL"),
		build("NULL,'cost_adjustment',1,-10,NULL,'difference',false", actor, "NULL"),
		// Sin responsable o con dos responsables.
		build(warehouse+",'initial',10,200,20,'document',false", "NULL,NULL", "10"),
		build(warehouse+",'initial',10,200,20,'document',false", "gen_random_uuid(),'inventory_worker'", "10"),
		// Costo pendiente sin la marca, tipo o base desconocidos y costo negativo.
		build(warehouse+",'consumption',-1,0,NULL,'pending',false", actor, "-1"),
		build(warehouse+",'robo',-1,0,NULL,'average',false", actor, "-1"),
		build(warehouse+",'initial',10,200,20,'inventado',false", actor, "10"),
		build(warehouse+",'initial',10,200,-20,'document',false", actor, "10"),
	}
	for _, statement := range accepted {
		if _, err := connection.Exec(ctx, statement); err != nil {
			t.Fatalf("PostgreSQL rechazó un movimiento válido (%s): %v", statement, err)
		}
	}
	for _, statement := range rejected {
		if _, err := connection.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó un movimiento incoherente: %s", statement)
		}
	}
	for _, statement := range []string{
		"INSERT INTO settings_check(count_frequency) VALUES ('custom')",
		"INSERT INTO settings_check(count_frequency,count_interval_days) VALUES ('weekly',7)",
		"INSERT INTO settings_check(count_frequency,count_interval_days) VALUES ('custom',0)",
		"INSERT INTO settings_check(singleton) VALUES (false)",
		"INSERT INTO valuations_check(item_id,quantity,value) VALUES (gen_random_uuid(),0,1)",
		"INSERT INTO valuations_check(item_id,pending_cost_quantity) VALUES (gen_random_uuid(),-1)",
		"INSERT INTO units_check(code,name,dimension,factor) VALUES ('oz','Onza','weight',28.35)",
		"INSERT INTO units_check(code,name,dimension,factor) VALUES ('oz','Onza','mass',0)",
		"INSERT INTO units_check(code,name,dimension,factor) VALUES ('Oz','Onza','mass',28.35)",
		"INSERT INTO units_check(code,name,dimension,factor,system,active) VALUES ('oz','Onza','mass',28.35,true,false)",
		"INSERT INTO items_check(code,name,base_unit_id,reference_cost) VALUES ('A','Arroz',gen_random_uuid(),0)",
		"INSERT INTO items_check(code,name,base_unit_id,kind) VALUES ('A','Arroz',gen_random_uuid(),'servicio')",
		"INSERT INTO items_check(code,name,base_unit_id,min_stock) VALUES ('A','Arroz',gen_random_uuid(),-1)",
		"INSERT INTO items_check(code,name,base_unit_id,barcode) VALUES ('A','Arroz',gen_random_uuid(),'con espacio')",
		"INSERT INTO items_check(code,name,base_unit_id) VALUES (' A','Arroz',gen_random_uuid())",
	} {
		if _, err := connection.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó un registro incoherente: %s", statement)
		}
	}
	for _, statement := range []string{
		"INSERT INTO settings_check(singleton,count_frequency,count_interval_days) VALUES (true,'custom',14)",
		"INSERT INTO valuations_check(item_id,quantity,value,last_valid_cost) VALUES (gen_random_uuid(),-5,-100,20)",
		"INSERT INTO units_check(code,name,dimension,factor) VALUES ('oz','Onza','mass',28.349523125)",
		"INSERT INTO items_check(code,name,base_unit_id,reference_cost,barcode) VALUES ('A','Arroz',gen_random_uuid(),18.5,'7501')",
	} {
		if _, err := connection.Exec(ctx, statement); err != nil {
			t.Fatalf("PostgreSQL rechazó un registro válido (%s): %v", statement, err)
		}
	}
	if _, err := connection.Exec(ctx, "DROP TABLE movements_check, settings_check, valuations_check, units_check, items_check"); err != nil {
		t.Fatal(err)
	}
	// El libro real es de solo inserción: un movimiento se registra pero no se modifica ni se borra.
	if _, err := pool.Exec(ctx, `INSERT INTO app.inventory_items(id,code,name,base_unit_id)
            SELECT '00000000-0000-4000-8000-0000000000a0','ARZ','Arroz',id FROM app.inventory_units WHERE code='kg';
        INSERT INTO app.inventory_movements(id,item_id,warehouse_id,kind,quantity,value,unit_cost,cost_basis,occurred_at,source_kind,
            process,entry_key,stock_after,valuation_quantity_after,valuation_value_after)
        SELECT gen_random_uuid(),'00000000-0000-4000-8000-0000000000a0',w.id,'initial',10,200,20,'document',now(),'initial',
            'migration_check','prueba:1',10,10,200 FROM app.inventory_warehouses w WHERE w.is_default`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"UPDATE app.inventory_movements SET value=0",
		"DELETE FROM app.inventory_movements",
		"TRUNCATE app.inventory_movements",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó %s sobre el libro", statement)
		}
	}
}

// recipeConstraints son las restricciones con nombre explícito de la 0011 (etapa 8, fase B).
var recipeConstraints = []string{
	"inventory_ingredient_portions_ingredient_id_fkey", "inventory_ingredient_portions_item_id_fkey",
	"inventory_ingredient_portions_unit_id_fkey", "inventory_ingredient_portions_actor_id_fkey",
	"inventory_ingredient_portions_operation_id_fkey", "inventory_ingredient_portions_seq_key",
	"inventory_ingredient_portions_shape_check", "inventory_recipes_owner_kind_check", "inventory_recipes_product_id_fkey",
	"inventory_recipes_complement_id_fkey", "inventory_recipes_preparation_mode_check",
	"inventory_recipes_preparation_item_id_fkey", "inventory_recipes_name_check", "inventory_recipes_revision_check",
	"inventory_recipes_created_by_fkey", "inventory_recipes_updated_by_fkey", "inventory_recipes_variant_id_fkey",
	"inventory_recipes_owner_check", "inventory_recipe_versions_recipe_id_fkey", "inventory_recipe_versions_version_check",
	"inventory_recipe_versions_name_check", "inventory_recipe_versions_yield_quantity_check",
	"inventory_recipe_versions_yield_unit_id_fkey", "inventory_recipe_versions_yield_base_quantity_check",
	"inventory_recipe_versions_waste_check", "inventory_recipe_versions_notes_check",
	"inventory_recipe_versions_created_by_fkey", "inventory_recipe_versions_operation_id_fkey",
	"inventory_recipe_versions_recipe_version_key", "inventory_recipe_lines_version_id_fkey",
	"inventory_recipe_lines_line_no_check", "inventory_recipe_lines_component_kind_check",
	"inventory_recipe_lines_item_id_fkey", "inventory_recipe_lines_preparation_recipe_id_fkey",
	"inventory_recipe_lines_ingredient_id_fkey", "inventory_recipe_lines_quantity_check", "inventory_recipe_lines_unit_id_fkey",
	"inventory_recipe_lines_base_quantity_check", "inventory_recipe_lines_pkey", "inventory_recipe_lines_component_check",
	"inventory_consumption_requests_kind_check", "inventory_consumption_requests_restaurant_unit_id_fkey",
	"inventory_consumption_requests_version_id_fkey", "inventory_consumption_requests_service_mode_check",
	"inventory_consumption_requests_state_check", "inventory_consumption_requests_attempts_check",
	"inventory_consumption_requests_last_error_check", "inventory_consumption_requests_diagnostics_check",
	"inventory_consumption_requests_seq_key", "inventory_consumption_requests_source_key",
	"inventory_consumption_requests_processed_check", "inventory_consumption_requests_failed_check",
	"inventory_unit_consumptions_restaurant_unit_id_fkey", "inventory_unit_consumptions_release_request_id_fkey",
	"inventory_unit_consumptions_last_request_id_fkey", "inventory_unit_consumptions_last_version_id_fkey",
	"inventory_unit_consumptions_service_mode_check", "inventory_unit_consumptions_state_check",
	"inventory_unit_consumptions_release_request_key", "inventory_cancellation_classifications_restaurant_unit_id_fkey",
	"inventory_cancellation_classifications_request_id_fkey", "inventory_cancellation_classifications_state_check",
	"inventory_cancellation_classifications_resolution_check", "inventory_cancellation_classifications_classified_by_fkey",
	"inventory_cancellation_classifications_notes_check", "inventory_cancellation_classifications_revision_check",
	"inventory_cancellation_classifications_unit_key", "inventory_cancellation_classifications_request_key",
	"inventory_cancellation_classifications_deadline_check", "inventory_cancellation_classifications_resolution_shape_check",
	"inventory_productions_recipe_id_fkey", "inventory_productions_recipe_version_id_fkey",
	"inventory_productions_item_id_fkey", "inventory_productions_warehouse_id_fkey", "inventory_productions_batches_check",
	"inventory_productions_expected_quantity_check", "inventory_productions_produced_quantity_check",
	"inventory_productions_waste_quantity_check", "inventory_productions_consumed_value_check",
	"inventory_productions_notes_check", "inventory_productions_actor_id_fkey", "inventory_productions_operation_id_fkey",
	"inventory_productions_movement_id_fkey", "inventory_productions_operation_id_key",
}

// checkStage8PhaseB comprueba la 0011: sin siembras, permisos de la fase B solo del administrador,
// restricciones con nombre, CHECK seguros ante NULL (con tablas temporales) y versiones, renglones
// y porciones de solo inserción.
func checkStage8PhaseB(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var named, rows, granted, others int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_constraint WHERE conname = ANY($1)", recipeConstraints).Scan(&named); err != nil {
		t.Fatal(err)
	}
	if named != len(recipeConstraints) {
		t.Fatalf("Faltan restricciones con nombre explícito en la 0011: %d de %d", named, len(recipeConstraints))
	}
	err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.inventory_recipes)+(SELECT count(*) FROM app.inventory_recipe_versions)
            +(SELECT count(*) FROM app.inventory_recipe_lines)+(SELECT count(*) FROM app.inventory_ingredient_portions)
            +(SELECT count(*) FROM app.inventory_consumption_requests)+(SELECT count(*) FROM app.inventory_unit_consumptions)
            +(SELECT count(*) FROM app.inventory_cancellation_classifications)+(SELECT count(*) FROM app.inventory_productions),
        (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE r.code='administrator'
            AND p.permission_key IN ('inventory.recipes.manage','inventory.subrecipes.manage','inventory.production.record',
                'inventory.waste.record')),
        (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE r.code<>'administrator'
            AND p.permission_key IN ('inventory.recipes.manage','inventory.subrecipes.manage','inventory.production.record',
                'inventory.waste.record'))`).Scan(&rows, &granted, &others)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 0 || granted != 4 || others != 0 {
		t.Fatalf("La 0011 no debe sembrar datos y solo concede al administrador: filas %d, administrador %d, otros %d", rows, granted, others)
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `CREATE TEMP TABLE recipes_check (LIKE app.inventory_recipes INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE lines_check (LIKE app.inventory_recipe_lines INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE portions_check (LIKE app.inventory_ingredient_portions INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE requests_check (LIKE app.inventory_consumption_requests INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE classifications_check (LIKE app.inventory_cancellation_classifications INCLUDING CONSTRAINTS INCLUDING DEFAULTS);
        CREATE TEMP TABLE versions_check (LIKE app.inventory_recipe_versions INCLUDING CONSTRAINTS INCLUDING DEFAULTS)`); err != nil {
		t.Fatal(err)
	}
	recipe := "INSERT INTO recipes_check(owner_kind,product_id,variant_id,complement_id,preparation_mode,preparation_item_id,name," +
		"created_by,updated_by) VALUES (%s,'Receta',gen_random_uuid(),gen_random_uuid())"
	line := "INSERT INTO lines_check(version_id,line_no,component_kind,item_id,preparation_recipe_id,ingredient_id,quantity,unit_id," +
		"base_quantity,takeaway_only) VALUES (gen_random_uuid(),1,%s)"
	request := "INSERT INTO requests_check(kind,restaurant_unit_id,source_id,version_id,released_at,occurred_at,service_mode,state," +
		"attempts,last_error,processed_at) VALUES (%s)"
	classification := "INSERT INTO classifications_check(restaurant_unit_id,request_id,cancelled_at,operating_date,deadline_at,state," +
		"resolution,classified_by,classified_at) VALUES (gen_random_uuid(),gen_random_uuid(),now(),current_date,%s)"
	fill := func(template, values string) string {
		return strings.Replace(template, "%s", values, 1)
	}
	accepted := []string{
		fill(recipe, "'product',gen_random_uuid(),NULL,NULL,NULL,NULL"),
		fill(recipe, "'complement',NULL,NULL,gen_random_uuid(),NULL,NULL"),
		fill(recipe, "'preparation',NULL,NULL,NULL,'immediate',NULL"),
		fill(recipe, "'preparation',NULL,NULL,NULL,'stored',gen_random_uuid()"),
		fill(line, "'item',gen_random_uuid(),NULL,NULL,150,gen_random_uuid(),150,false"),
		fill(line, "'preparation',NULL,gen_random_uuid(),NULL,50,gen_random_uuid(),50,true"),
		fill(line, "'ingredient',gen_random_uuid(),NULL,gen_random_uuid(),20,gen_random_uuid(),20,false"),
		"INSERT INTO portions_check(ingredient_id,actor_id) VALUES (gen_random_uuid(),gen_random_uuid())",
		"INSERT INTO portions_check(ingredient_id,item_id,quantity,unit_id,base_quantity,actor_id) VALUES " +
			"(gen_random_uuid(),gen_random_uuid(),0.02,gen_random_uuid(),20,gen_random_uuid())",
		fill(request, "'release',gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),now(),now(),'takeaway','pending',0,NULL,NULL"),
		fill(request, "'amend',gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),now(),now(),'dine_in','processed',0,NULL,now()"),
		fill(request, "'cancel',gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),now(),now(),'dine_in','failed',1,'Error genérico',NULL"),
		fill(classification, "now()+interval '1 day','pending',NULL,NULL,NULL"),
		fill(classification, "now()+interval '1 day','not_prepared','automatic',NULL,now()"),
		fill(classification, "now()+interval '1 day','prepared_waste','manual',gen_random_uuid(),now()"),
		fill(classification, "now()+interval '1 day','prepared_waste','correction',gen_random_uuid(),now()"),
	}
	rejected := []string{
		// Recetas incoherentes con su tipo (incluido el NULL en la rama que valida).
		fill(recipe, "'product',NULL,NULL,NULL,NULL,NULL"),
		fill(recipe, "'product',gen_random_uuid(),NULL,gen_random_uuid(),NULL,NULL"),
		fill(recipe, "'complement',NULL,gen_random_uuid(),gen_random_uuid(),NULL,NULL"),
		fill(recipe, "'preparation',NULL,NULL,NULL,NULL,NULL"),
		fill(recipe, "'preparation',NULL,NULL,NULL,'stored',NULL"),
		fill(recipe, "'preparation',NULL,NULL,NULL,'immediate',gen_random_uuid()"),
		fill(recipe, "'menu',gen_random_uuid(),NULL,NULL,NULL,NULL"),
		// Renglones sin su componente, con dos o con cantidad no positiva; «solo para llevar» en un
		// renglón de ingrediente.
		fill(line, "'item',NULL,NULL,NULL,1,gen_random_uuid(),1,false"),
		fill(line, "'item',gen_random_uuid(),gen_random_uuid(),NULL,1,gen_random_uuid(),1,false"),
		fill(line, "'ingredient',NULL,NULL,gen_random_uuid(),1,gen_random_uuid(),1,false"),
		fill(line, "'ingredient',gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),1,gen_random_uuid(),1,false"),
		fill(line, "'ingredient',gen_random_uuid(),NULL,gen_random_uuid(),1,gen_random_uuid(),1,true"),
		fill(line, "'item',gen_random_uuid(),NULL,NULL,0,gen_random_uuid(),0,false"),
		// Porción a medias.
		"INSERT INTO portions_check(ingredient_id,item_id,actor_id) VALUES (gen_random_uuid(),gen_random_uuid(),gen_random_uuid())",
		"INSERT INTO portions_check(ingredient_id,item_id,quantity,unit_id,base_quantity,actor_id) VALUES " +
			"(gen_random_uuid(),gen_random_uuid(),0,gen_random_uuid(),0,gen_random_uuid())",
		// Solicitudes procesadas sin fecha, fallidas sin error, tipo o modalidad desconocidos.
		fill(request, "'release',gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),now(),now(),'dine_in','processed',0,NULL,NULL"),
		fill(request, "'release',gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),now(),now(),'dine_in','pending',0,NULL,now()"),
		fill(request, "'release',gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),now(),now(),'dine_in','failed',0,NULL,NULL"),
		fill(request, "'refund',gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),now(),now(),'dine_in','pending',0,NULL,NULL"),
		fill(request, "'release',gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),now(),now(),'delivery','pending',0,NULL,NULL"),
		// Clasificaciones: pendiente con resolución, automática como merma o con usuario, manual sin
		// usuario y corte anterior a la cancelación.
		fill(classification, "now()+interval '1 day','pending','manual',gen_random_uuid(),now()"),
		fill(classification, "now()+interval '1 day','prepared_waste','automatic',NULL,now()"),
		fill(classification, "now()+interval '1 day','not_prepared','automatic',gen_random_uuid(),now()"),
		fill(classification, "now()+interval '1 day','not_prepared','manual',NULL,now()"),
		fill(classification, "now()+interval '1 day','not_prepared',NULL,NULL,now()"),
		fill(classification, "now()-interval '1 day','pending',NULL,NULL,NULL"),
		// Versión con merma de 100 % o rendimiento cero.
		"INSERT INTO versions_check(recipe_id,version,name,active,yield_quantity,yield_unit_id,yield_base_quantity," +
			"production_waste_percent,created_by) VALUES (gen_random_uuid(),1,'V',true,1,gen_random_uuid(),1,100,gen_random_uuid())",
		"INSERT INTO versions_check(recipe_id,version,name,active,yield_quantity,yield_unit_id,yield_base_quantity,created_by) " +
			"VALUES (gen_random_uuid(),1,'V',true,0,gen_random_uuid(),0,gen_random_uuid())",
	}
	for _, statement := range accepted {
		if _, err := connection.Exec(ctx, statement); err != nil {
			t.Fatalf("PostgreSQL rechazó un registro válido de la 0011 (%s): %v", statement, err)
		}
	}
	for _, statement := range rejected {
		if _, err := connection.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó un registro incoherente de la 0011: %s", statement)
		}
	}
	if _, err := connection.Exec(ctx, "DROP TABLE recipes_check, lines_check, portions_check, requests_check, classifications_check, "+
		"versions_check"); err != nil {
		t.Fatal(err)
	}
	// Versiones, renglones y porciones reales son de solo inserción.
	if _, err := pool.Exec(ctx, `INSERT INTO app.catalog_ingredients(id,name) VALUES ('00000000-0000-4000-8000-0000000000b1','Cebolla');
        INSERT INTO app.inventory_recipes(id,owner_kind,preparation_mode,name,created_by,updated_by)
            SELECT '00000000-0000-4000-8000-0000000000b2','preparation','immediate','Salsa',id,id FROM app.users WHERE username='cajera';
        INSERT INTO app.inventory_recipe_versions(id,recipe_id,version,name,active,yield_quantity,yield_unit_id,yield_base_quantity,created_by)
            SELECT '00000000-0000-4000-8000-0000000000b3','00000000-0000-4000-8000-0000000000b2',1,'Salsa',true,500,un.id,500,u.id
            FROM app.users u, app.inventory_units un WHERE u.username='cajera' AND un.code='ml';
        INSERT INTO app.inventory_recipe_lines(version_id,line_no,component_kind,item_id,quantity,unit_id,base_quantity)
            SELECT '00000000-0000-4000-8000-0000000000b3',1,'item','00000000-0000-4000-8000-0000000000a0',0.4,un.id,0.4
            FROM app.inventory_units un WHERE un.code='kg';
        INSERT INTO app.inventory_ingredient_portions(ingredient_id,item_id,quantity,unit_id,base_quantity,actor_id)
            SELECT '00000000-0000-4000-8000-0000000000b1','00000000-0000-4000-8000-0000000000a0',0.02,un.id,0.02,u.id
            FROM app.users u, app.inventory_units un WHERE u.username='cajera' AND un.code='kg'`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"UPDATE app.inventory_recipe_versions SET active=false",
		"DELETE FROM app.inventory_recipe_versions",
		"UPDATE app.inventory_recipe_lines SET quantity=1",
		"DELETE FROM app.inventory_recipe_lines",
		"TRUNCATE app.inventory_recipe_lines",
		"UPDATE app.inventory_ingredient_portions SET item_id=NULL,quantity=NULL,unit_id=NULL,base_quantity=NULL",
		"DELETE FROM app.inventory_ingredient_portions",
		"TRUNCATE app.inventory_ingredient_portions",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó %s sobre un registro de solo inserción", statement)
		}
	}
	// Una variante debe pertenecer al producto de la receta (llave compuesta).
	if _, err := pool.Exec(ctx, `INSERT INTO app.inventory_recipes(owner_kind,product_id,variant_id,name,created_by,updated_by)
        SELECT 'product',gen_random_uuid(),gen_random_uuid(),'Ajena',id,id FROM app.users WHERE username='cajera'`); err == nil {
		t.Fatal("PostgreSQL aceptó una receta de una variante y un producto inexistentes")
	}
}
