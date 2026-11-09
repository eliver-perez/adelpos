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
