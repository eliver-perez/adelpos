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

// Etapa A1: actualización desde el esquema 24 con roles, usuarios, terminales y sesiones existentes. La sección B de la
// 0025 agrega el PIN de los usuarios (sin PIN, versión 0, sin nombre corto ni color), los intentos del PIN, las
// credenciales de envío, el bloqueo por inactividad de cada terminal (120 s) y el permiso waiter_devices.use para el
// administrador y el rol Mesero. No cambia ninguna fila existente de ninguna tabla: solo agrega el permiso y sus dos
// concesiones. Deja la base vacía.
func TestStageA1Upgrade(t *testing.T) {
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
	if last := expected[len(expected)-1].Version; last < 25 || last != product.Read().SchemaVersion {
		t.Fatalf("la última migración (%d) no coincide con el esquema del producto (%d)", last, product.Read().SchemaVersion)
	}
	schema24 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 24 {
			schema24[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema24); err != nil || n != 24 {
		t.Fatalf("no se preparó el esquema 24: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stageA1ExistingData); err != nil {
		t.Fatal(err)
	}
	tables := stageA1Tables(ctx, t, pool)
	before := map[string]string{}
	for table, columns := range tables {
		before[table] = stageA1Snapshot(ctx, t, pool, table, columns)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-24 {
		t.Fatalf("la 0025 no se aplicó sobre el esquema 24: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0025 no es repetible")
	}
	for table, columns := range tables {
		if after := stageA1Snapshot(ctx, t, pool, table, columns); after != before[table] {
			t.Fatalf("la 0025 cambió filas existentes de %s:\nantes   %s\ndespués %s", table, before[table], after)
		}
	}
	checkStageA1Permission(ctx, t, pool)
	checkStageA1Columns(ctx, t, pool)
	checkStageA1Credentials(ctx, t, pool)
}

// stageA1Tables son las tablas del esquema 24 con sus columnas: la comparación usa solo esas columnas (las nuevas de
// la 0025 tienen su valor por omisión y se comprueban aparte).
func stageA1Tables(ctx context.Context, t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT c.table_name,string_agg(quote_ident(c.column_name),',' ORDER BY c.ordinal_position)
        FROM information_schema.columns c JOIN information_schema.tables tb
            ON tb.table_schema=c.table_schema AND tb.table_name=c.table_name AND tb.table_type='BASE TABLE'
        WHERE c.table_schema='app' GROUP BY c.table_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	tables := map[string]string{}
	for rows.Next() {
		var table, columns string
		if err := rows.Scan(&table, &columns); err != nil {
			t.Fatal(err)
		}
		tables[table] = columns
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(tables) < 100 {
		t.Fatalf("el esquema 24 tiene solo %d tablas", len(tables))
	}
	return tables
}

// stageA1Snapshot resume las filas de una tabla con las columnas dadas. En permissions y role_permissions se excluye el
// permiso nuevo de la 0025, que se comprueba en checkStageA1Permission.
func stageA1Snapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool, table, columns string) string {
	t.Helper()
	filter := ""
	switch table {
	case "permissions":
		filter = " WHERE key<>'waiter_devices.use'"
	case "role_permissions":
		filter = " WHERE permission_key<>'waiter_devices.use'"
	}
	var snapshot string
	query := "SELECT coalesce(md5(string_agg(row_to_json(r)::text,'|' ORDER BY row_to_json(r)::text)),'') || ':' || count(*) " +
		"FROM (SELECT " + columns + " FROM app." + quoteTable(table) + filter + ") r"
	if err := pool.QueryRow(ctx, query).Scan(&snapshot); err != nil {
		t.Fatalf("%s: %v", table, err)
	}
	return snapshot
}

func quoteTable(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// checkStageA1Permission: el permiso con su descripción (la misma de Go) concedido solo al administrador y al rol
// Mesero; el supervisor no lo recibe.
func checkStageA1Permission(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var description, holders string
	err := pool.QueryRow(ctx, `SELECT p.description,
            coalesce((SELECT string_agg(r.code,',' ORDER BY r.code) FROM app.role_permissions rp JOIN app.roles r ON r.id=rp.role_id
                WHERE rp.permission_key='waiter_devices.use'),'')
        FROM app.permissions p WHERE p.key='waiter_devices.use'`).Scan(&description, &holders)
	if err != nil {
		t.Fatal(err)
	}
	if description != "Entrar con PIN en teléfonos y tabletas de meseros" || holders != "administrator,waiter" {
		t.Fatalf("permiso waiter_devices.use: %q concedido a %q", description, holders)
	}
}

// checkStageA1Columns: valores por omisión de las columnas nuevas y sus restricciones.
func checkStageA1Columns(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var withPIN, versions, badges, idle int
	err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.users WHERE pin_hash IS NOT NULL OR pin_set_at IS NOT NULL),
        (SELECT count(*) FROM app.users WHERE pin_version<>0),
        (SELECT count(*) FROM app.users WHERE short_name IS NOT NULL OR badge_color IS NOT NULL),
        (SELECT count(*) FROM app.devices WHERE idle_lock_seconds<>120)`).Scan(&withPIN, &versions, &badges, &idle)
	if err != nil {
		t.Fatal(err)
	}
	if withPIN != 0 || versions != 0 || badges != 0 || idle != 0 {
		t.Fatalf("valores por omisión: PIN %d, versiones %d, distintivos %d, bloqueo %d", withPIN, versions, badges, idle)
	}
	rejected := map[string]string{
		"bloqueo menor a 30 s":    "UPDATE app.devices SET idle_lock_seconds=29",
		"bloqueo mayor a 1 hora":  "UPDATE app.devices SET idle_lock_seconds=3601",
		"color fuera de formato":  "UPDATE app.users SET badge_color='#ABCDEF'",
		"color con nombre":        "UPDATE app.users SET badge_color='azul'",
		"nombre corto largo":      "UPDATE app.users SET short_name=repeat('a',21)",
		"nombre corto con blanco": "UPDATE app.users SET short_name=' Ana'",
		"PIN sin fecha":           "UPDATE app.users SET pin_hash='x'",
		"versión negativa":        "UPDATE app.users SET pin_version=-1",
		"intentos de más":         "INSERT INTO app.pin_attempts(user_id,failures) VALUES ('00000000-0000-4000-8000-0000000000a1',6)",
	}
	for name, statement := range rejected {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("la 0025 aceptó: %s", name)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE app.users SET pin_hash='$argon2id$prueba',pin_set_at=now(),pin_version=1,short_name='Ana',
        badge_color='#3591d4' WHERE id='00000000-0000-4000-8000-0000000000a1'`); err != nil {
		t.Fatalf("un PIN válido no se pudo guardar: %v", err)
	}
}

// checkStageA1Credentials: una sola credencial de envío vigente por usuario y dispositivo, solo su SHA-256 y la
// revocación con su motivo.
func checkStageA1Credentials(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	insert := func(hash string) error {
		_, err := pool.Exec(ctx, `INSERT INTO app.offline_order_credentials(user_id,device_id,secret_hash,pin_version)
            VALUES ('00000000-0000-4000-8000-0000000000a1','00000000-0000-4000-8000-0000000000b1',$1,1)`, hash)
		return err
	}
	if err := insert(strings.Repeat("a", 64)); err != nil {
		t.Fatalf("credencial válida: %v", err)
	}
	if err := insert(strings.Repeat("b", 64)); !uniqueViolation(err) {
		t.Fatalf("dos credenciales vigentes del mismo usuario y dispositivo: %v", err)
	}
	for name, statement := range map[string]string{
		"hash que no es SHA-256": `INSERT INTO app.offline_order_credentials(user_id,device_id,secret_hash,pin_version)
            VALUES ('00000000-0000-4000-8000-0000000000a1','00000000-0000-4000-8000-0000000000b2','secreto',1)`,
		"revocada sin motivo":  `UPDATE app.offline_order_credentials SET revoked_at=now()`,
		"motivo desconocido":   `UPDATE app.offline_order_credentials SET revoked_at=now(),revoke_reason='expired'`,
		"versión del PIN cero": `UPDATE app.offline_order_credentials SET pin_version=0`,
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("la 0025 aceptó: %s", name)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE app.offline_order_credentials SET revoked_at=now(),revoke_reason='replaced'"); err != nil {
		t.Fatal(err)
	}
	if err := insert(strings.Repeat("c", 64)); err != nil {
		t.Fatalf("una credencial nueva después de revocar la anterior: %v", err)
	}
}

// Roles (administrador, supervisor y mesero), usuarios, terminales, una sesión vigente y una cerrada, y un permiso del
// administrador: lo que tendría una instalación existente en el esquema 24.
const stageA1ExistingData = `
INSERT INTO app.roles(id,code,name) VALUES ('00000000-0000-4000-8000-000000000001','administrator','Administrador'),
    ('00000000-0000-4000-8000-000000000002','supervisor','Supervisor'),
    ('00000000-0000-4000-8000-000000000003','waiter','Mesero');
INSERT INTO app.permissions(key,description) VALUES ('users.manage','Crear, editar y desactivar usuarios') ON CONFLICT DO NOTHING;
INSERT INTO app.role_permissions(role_id,permission_key) VALUES ('00000000-0000-4000-8000-000000000001','users.manage');
INSERT INTO app.users(id,username,display_name,role_id,password_hash) VALUES
    ('00000000-0000-4000-8000-000000000010','admin','Administrador','00000000-0000-4000-8000-000000000001','hash-admin'),
    ('00000000-0000-4000-8000-0000000000a1','ana','Ana','00000000-0000-4000-8000-000000000003','hash-ana');
INSERT INTO app.devices(id,name,secret_hash,kind) VALUES
    ('00000000-0000-4000-8000-000000000021','Caja 1','hash-a1-1','desktop'),
    ('00000000-0000-4000-8000-0000000000b1','Tableta 1','hash-a1-2','android'),
    ('00000000-0000-4000-8000-0000000000b2','Teléfono 2','hash-a1-3','android');
INSERT INTO app.sessions(id,user_id,device_id,token_hash,expires_at,revoked_at,revoke_reason) VALUES
    ('00000000-0000-4000-8000-000000000031','00000000-0000-4000-8000-0000000000a1','00000000-0000-4000-8000-0000000000b1',
     'token-ana-1',now()+interval '12 hours',NULL,NULL),
    ('00000000-0000-4000-8000-000000000032','00000000-0000-4000-8000-000000000010','00000000-0000-4000-8000-000000000021',
     'token-admin-1',now()+interval '12 hours',now()-interval '1 hour','logout');
`
