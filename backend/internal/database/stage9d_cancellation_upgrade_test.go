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

// Etapa 9D: actualización desde el esquema 20 con cuentas existentes (una anulada con la forma
// anterior, una abierta con borrador y otra abierta vacía). La 0021 agrega el catálogo de motivos de
// cancelación con sus cuatro valores iniciales, el registro de cuentas canceladas (de solo inserción,
// con restricciones con nombre seguras ante NULL), el umbral de cuenta abierta (12 h, de 1 a 72) y el
// permiso «Cancelar cuentas» solo para el administrador, sin modificar las cuentas, eventos ni permisos
// existentes de los demás roles. Deja la base vacía al terminar.
func TestStage9DAccountCancellationUpgrade(t *testing.T) {
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
	if last := expected[len(expected)-1].Version; last != 21 || product.Read().SchemaVersion != 21 {
		t.Fatalf("se esperaba el esquema 21 en la última migración (%d) y en el producto (%d)", last, product.Read().SchemaVersion)
	}
	schema20 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 20 {
			schema20[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema20); err != nil || n != 20 {
		t.Fatalf("no se preparó el esquema 20: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stage9DExistingAccounts); err != nil {
		t.Fatal(err)
	}
	before := stage9DSnapshot(ctx, t, pool)
	if n, err := Migrate(ctx, pool); err != nil || n != 1 {
		t.Fatalf("la 0021 no se aplicó sobre el esquema 20: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0021 no es repetible")
	}
	if after := stage9DSnapshot(ctx, t, pool); after != before {
		t.Fatalf("la 0021 cambió datos existentes: antes %s, después %s", before, after)
	}
	checkStage9DReasons(ctx, t, pool)
	checkStage9DPermission(ctx, t, pool)
	checkStage9DLongOpenHours(ctx, t, pool)
	checkStage9DCancellations(ctx, t, pool)
}

// stage9DSnapshot resume lo que la 0021 no debe tocar: cuentas, eventos, borradores y permisos de los
// demás roles.
func stage9DSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT concat_ws(' | ',
            (SELECT string_agg(folio||':'||state||':'||revision,',' ORDER BY folio) FROM app.restaurant_accounts),
            (SELECT string_agg(kind,',' ORDER BY sequence) FROM app.restaurant_events),
            (SELECT count(*) FROM app.restaurant_draft_items),
            (SELECT string_agg(r.code||'='||p.permission_key,',' ORDER BY r.code,p.permission_key)
                FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE r.code<>'administrator'),
            (SELECT warning_minutes||'/'||overdue_minutes||'/'||revision FROM app.restaurant_settings))`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// Los cuatro motivos iniciales en su orden: solo «Otro» exige descripción y solo «Cliente se retiró
// sin pagar» marca lo enviado como merma; el nombre es único y no admite espacios sobrantes.
func checkStage9DReasons(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var reasons string
	err := pool.QueryRow(ctx, `SELECT string_agg(name||':'||requires_description||':'||prepared_is_waste||':'||active,
        ',' ORDER BY sort_order) FROM app.account_cancel_reasons`).Scan(&reasons)
	if err != nil {
		t.Fatal(err)
	}
	want := "Cliente se retiró sin pagar:false:true:true,Cliente canceló el pedido:false:false:true," +
		"Error de captura:false:false:true,Otro:true:false:true"
	if reasons != want {
		t.Fatalf("motivos iniciales: %s", reasons)
	}
	for _, statement := range []string{
		"INSERT INTO app.account_cancel_reasons(name) VALUES ('Otro')",
		"INSERT INTO app.account_cancel_reasons(name) VALUES (' Con espacios ')",
		"INSERT INTO app.account_cancel_reasons(name) VALUES ('')",
		"INSERT INTO app.account_cancel_reasons(name) VALUES (NULL)",
		"INSERT INTO app.account_cancel_reasons(name,sort_order) VALUES ('Negativo',-1)",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("la base aceptó %s", statement)
		}
	}
}

// accounts.cancel existe con su descripción, solo el administrador lo recibe y accounts.manage
// queda con la descripción que muestra Go.
func checkStage9DPermission(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var description, holders, manage string
	err := pool.QueryRow(ctx, `SELECT p.description,
            coalesce((SELECT string_agg(r.code,',' ORDER BY r.code) FROM app.role_permissions rp JOIN app.roles r ON r.id=rp.role_id
                WHERE rp.permission_key='accounts.cancel'),''),
            (SELECT description FROM app.permissions WHERE key='accounts.manage')
        FROM app.permissions p WHERE p.key='accounts.cancel'`).Scan(&description, &holders, &manage)
	if err != nil {
		t.Fatal(err)
	}
	if description != "Cancelar cuentas" || holders != "administrator" ||
		manage != "Editar datos de cuentas y cancelar cuentas vacías con motivo libre" {
		t.Fatalf("permiso: %q a %q; accounts.manage %q", description, holders, manage)
	}
}

// El umbral nace en 12 horas y su CHECK con nombre solo admite de 1 a 72 (nunca NULL).
func checkStage9DLongOpenHours(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var hours, named int
	err := pool.QueryRow(ctx, `SELECT (SELECT long_open_hours FROM app.restaurant_settings),
        (SELECT count(*) FROM pg_constraint WHERE conname='restaurant_settings_long_open_hours_check'
            AND conrelid='app.restaurant_settings'::regclass)`).Scan(&hours, &named)
	if err != nil || hours != 12 || named != 1 {
		t.Fatalf("umbral %d, CHECK %d, %v", hours, named, err)
	}
	for _, value := range []any{1, 72} {
		if _, err := pool.Exec(ctx, "UPDATE app.restaurant_settings SET long_open_hours=$1", value); err != nil {
			t.Fatalf("no se aceptó %v: %v", value, err)
		}
	}
	for _, value := range []any{0, 73, nil} {
		if _, err := pool.Exec(ctx, "UPDATE app.restaurant_settings SET long_open_hours=$1", value); err == nil {
			t.Fatalf("la base aceptó %v horas", value)
		}
	}
}

// El registro de cancelaciones admite una cancelación válida (con o sin autorizador) y rechaza la
// autoautorización, descripciones vacías o con espacios sobrantes, importes negativos y una segunda
// fila por cuenta u operación; es de solo inserción.
func checkStage9DCancellations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var named int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conrelid='app.account_cancellations'::regclass
        AND conname IN ('account_cancellations_reason_name_check','account_cancellations_description_check',
            'account_cancellations_units_check','account_cancellations_amount_check','account_cancellations_operation_key',
            'account_cancellations_authorizer_check','account_cancellations_account_id_fkey','account_cancellations_reason_id_fkey')`).
		Scan(&named); err != nil || named != 8 {
		t.Fatalf("restricciones con nombre del registro: %d %v", named, err)
	}
	insert := `INSERT INTO app.account_cancellations(account_id,operation_id,reason_id,reason_name,description,prepared_is_waste,
            cancelled_by,authorized_by,device_id,cancelled_units,cancelled_amount)
        SELECT $1::uuid,$2::uuid,r.id,r.name,$3::text,r.prepared_is_waste,'00000000-0000-4000-8000-000000000011',$4::uuid,
            '00000000-0000-4000-8000-000000000022',$5::integer,$6::numeric
        FROM app.account_cancel_reasons r WHERE r.name='Cliente se retiró sin pagar'`
	account, operation := "00000000-0000-4000-8000-0000000000a2", "00000000-0000-4000-8000-000000000d05"
	supervisor := "00000000-0000-4000-8000-000000000012"
	canceller := "00000000-0000-4000-8000-000000000011"
	rejected := []struct {
		name        string
		description any
		authorized  any
		units       int
		amount      string
	}{
		{"autoautorización", nil, canceller, 1, "10"},
		{"descripción vacía", "", nil, 1, "10"},
		{"descripción con espacios", " Se fue ", nil, 1, "10"},
		{"unidades negativas", nil, nil, -1, "10"},
		{"importe negativo", nil, nil, 1, "-0.01"},
	}
	for _, entry := range rejected {
		if _, err := pool.Exec(ctx, insert, account, operation, entry.description, entry.authorized, entry.units, entry.amount); err == nil {
			t.Fatalf("la base aceptó %s", entry.name)
		}
	}
	if _, err := pool.Exec(ctx, insert, account, operation, "Se fue sin pagar", supervisor, 2, "70.00"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, insert, account, "00000000-0000-4000-8000-000000000d06", nil, nil, 0, "0"); err == nil {
		t.Fatal("la base aceptó dos cancelaciones de la misma cuenta")
	}
	if _, err := pool.Exec(ctx, insert, "00000000-0000-4000-8000-0000000000a3", operation, nil, nil, 0, "0"); err == nil {
		t.Fatal("la base aceptó dos cancelaciones con la misma operación")
	}
	if _, err := pool.Exec(ctx, insert, "00000000-0000-4000-8000-0000000000a3", "00000000-0000-4000-8000-000000000d06", nil, nil, 0, "0"); err != nil {
		t.Fatalf("una cancelación sin descripción ni autorizador debía aceptarse: %v", err)
	}
	for _, statement := range []string{"UPDATE app.account_cancellations SET cancelled_units=cancelled_units",
		"DELETE FROM app.account_cancellations", "TRUNCATE app.account_cancellations"} {
		if _, err := pool.Exec(ctx, statement); err == nil || !strings.Contains(err.Error(), "solo inserción") {
			t.Fatalf("%s no se rechazó: %v", statement, err)
		}
	}
}

// stage9DExistingAccounts son datos de una instalación en esquema 20: una cuenta anulada con la
// forma anterior (A1), una abierta con borrador (A2) y otra abierta vacía (A3), con sus eventos.
const stage9DExistingAccounts = `
INSERT INTO app.establishment(name,timezone) VALUES ('Restaurante de prueba','America/Monterrey');
INSERT INTO app.roles(id,code,name) VALUES ('00000000-0000-4000-8000-000000000001','administrator','Administrador'),
    ('00000000-0000-4000-8000-000000000002','supervisor','Supervisor');
INSERT INTO app.role_permissions(role_id,permission_key) SELECT '00000000-0000-4000-8000-000000000001',key FROM app.permissions;
INSERT INTO app.role_permissions(role_id,permission_key) VALUES ('00000000-0000-4000-8000-000000000002','accounts.read'),
    ('00000000-0000-4000-8000-000000000002','accounts.manage');
INSERT INTO app.users(id,username,display_name,role_id) VALUES
    ('00000000-0000-4000-8000-000000000011','cajera','Cajera','00000000-0000-4000-8000-000000000002'),
    ('00000000-0000-4000-8000-000000000012','gerente','Gerente','00000000-0000-4000-8000-000000000001');
INSERT INTO app.registers(id,name) VALUES ('00000000-0000-4000-8000-000000000021','Caja de prueba');
INSERT INTO app.devices(id,name,secret_hash,register_id) VALUES
    ('00000000-0000-4000-8000-000000000022','Terminal 1','hash-cancelaciones','00000000-0000-4000-8000-000000000021');
INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
SELECT op::uuid,'00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022','POST prueba','huella','{}'
FROM unnest(ARRAY['00000000-0000-4000-8000-000000000d01','00000000-0000-4000-8000-000000000d02','00000000-0000-4000-8000-000000000d03',
    '00000000-0000-4000-8000-000000000d04','00000000-0000-4000-8000-000000000d05','00000000-0000-4000-8000-000000000d06']) op;
INSERT INTO app.catalog_price_lists(id,name,fallback) VALUES ('00000000-0000-4000-8000-000000000031','Lista de prueba','base');
INSERT INTO app.catalog_categories(id,name) VALUES ('00000000-0000-4000-8000-000000000041','Platillos');
INSERT INTO app.catalog_products(id,category_id,name,code,kind,base_price) VALUES
    ('00000000-0000-4000-8000-000000000051','00000000-0000-4000-8000-000000000041','Hamburguesa','HAMB-01','product',110);
INSERT INTO app.restaurant_accounts(id,mode,name,price_list_id,origin,state,opened_by,assigned_user_id,device_id) VALUES
    ('00000000-0000-4000-8000-0000000000a1','dine_in','Anulada antes','00000000-0000-4000-8000-000000000031','Mostrador','voided',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022'),
    ('00000000-0000-4000-8000-0000000000a2','dine_in','Con borrador','00000000-0000-4000-8000-000000000031','Mostrador','open',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022'),
    ('00000000-0000-4000-8000-0000000000a3','takeaway','Pedido para llevar','00000000-0000-4000-8000-000000000031','Para llevar','open',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022');
INSERT INTO app.restaurant_draft_items(id,account_id,product_id,quantity,customization,position,author_id,device_id) VALUES
    ('00000000-0000-4000-8000-000000000061','00000000-0000-4000-8000-0000000000a2','00000000-0000-4000-8000-000000000051',1,
     '{}',0,'00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022');
INSERT INTO app.restaurant_events(id,account_id,operation_id,kind,payload,author_id,device_id)
SELECT e.id::uuid,e.account::uuid,e.operation::uuid,e.kind,e.payload::jsonb,'00000000-0000-4000-8000-000000000011',
    '00000000-0000-4000-8000-000000000022'
FROM (VALUES
    ('00000000-0000-4000-8000-000000000071','00000000-0000-4000-8000-0000000000a1','00000000-0000-4000-8000-000000000d01',
     'account.opened','{"before":null,"after":null}'),
    ('00000000-0000-4000-8000-000000000072','00000000-0000-4000-8000-0000000000a1','00000000-0000-4000-8000-000000000d02',
     'account.void','{"before":{},"after":{"reason":"Cliente se fue sin consumir"}}'),
    ('00000000-0000-4000-8000-000000000073','00000000-0000-4000-8000-0000000000a2','00000000-0000-4000-8000-000000000d03',
     'account.opened','{"before":null,"after":null}'),
    ('00000000-0000-4000-8000-000000000074','00000000-0000-4000-8000-0000000000a2','00000000-0000-4000-8000-000000000d03',
     'draft.saved','{"before":[],"after":[]}'),
    ('00000000-0000-4000-8000-000000000075','00000000-0000-4000-8000-0000000000a3','00000000-0000-4000-8000-000000000d04',
     'account.opened','{"before":null,"after":null}')
) e(id,account,operation,kind,payload);
`
