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
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/product"
	"local/ad-el-pos/server/migrations"
)

// Etapa 9F, fase C: actualización desde el esquema 21 con terminales, usuarios y estaciones existentes. La
// 0022 convierte kitchen_device_stations en «Estaciones que muestra esta terminal» con la marca «todas»
// (una terminal que ya tenía estaciones conserva su selección; las demás muestran todas), agrega el aviso
// sonoro por terminal (desactivado), «Estaciones que puede atender» por usuario (todas por omisión, tabla
// kitchen_user_stations con restricciones con nombre) y corrige las descripciones de accounts.manage (encargo
// de la fase E) y de kitchen.stations.manage (las estaciones por terminal ya no son un ámbito de
// autorización). No cambia los demás datos de terminales, usuarios ni estaciones. Deja la base vacía.
func TestStage9FKitchenStationsUpgrade(t *testing.T) {
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
	if last := expected[len(expected)-1].Version; last != 22 || product.Read().SchemaVersion != 22 {
		t.Fatalf("se esperaba el esquema 22 en la última migración (%d) y en el producto (%d)", last, product.Read().SchemaVersion)
	}
	schema21 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 21 {
			schema21[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema21); err != nil || n != 21 {
		t.Fatalf("no se preparó el esquema 21: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stage9FExistingKitchen); err != nil {
		t.Fatal(err)
	}
	var manageBefore string
	if err := pool.QueryRow(ctx, "SELECT description FROM app.permissions WHERE key='accounts.manage'").Scan(&manageBefore); err != nil {
		t.Fatal(err)
	}
	if manageBefore != "Editar datos de cuentas y cancelar cuentas vacías con motivo libre" {
		t.Fatalf("descripción de accounts.manage en el esquema 21: %q", manageBefore)
	}
	if stations := permissionDescription(ctx, t, pool, "kitchen.stations.manage"); stations != "Administrar estaciones y ámbitos de terminal" {
		t.Fatalf("descripción de kitchen.stations.manage en el esquema 21: %q", stations)
	}
	before := stage9FKitchenSnapshot(ctx, t, pool)
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-21 {
		t.Fatalf("la 0022 no se aplicó sobre el esquema 21: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0022 no es repetible")
	}
	if after := stage9FKitchenSnapshot(ctx, t, pool); after != before {
		t.Fatalf("la 0022 cambió datos existentes: antes %s, después %s", before, after)
	}
	checkStage9FTerminalStations(ctx, t, pool)
	checkStage9FUserStations(ctx, t, pool)
	var manage string
	if err := pool.QueryRow(ctx, "SELECT description FROM app.permissions WHERE key='accounts.manage'").Scan(&manage); err != nil {
		t.Fatal(err)
	}
	if manage != "Editar datos de cuentas" {
		t.Fatalf("descripción de accounts.manage: %q", manage)
	}
	// La descripción guardada coincide con la del registro de permisos de Go.
	stations := permissionDescription(ctx, t, pool, "kitchen.stations.manage")
	if stations != "Administrar estaciones y las estaciones que muestra cada terminal" {
		t.Fatalf("descripción de kitchen.stations.manage: %q", stations)
	}
	for _, permission := range kitchen.Permissions {
		if permission.Key == "kitchen.stations.manage" && permission.Description != stations {
			t.Fatalf("la descripción de Go (%q) no coincide con la de la base (%q)", permission.Description, stations)
		}
	}
}

// permissionDescription lee la descripción guardada de un permiso.
func permissionDescription(ctx context.Context, t *testing.T, pool *pgxpool.Pool, key string) string {
	t.Helper()
	var description string
	if err := pool.QueryRow(ctx, "SELECT description FROM app.permissions WHERE key=$1", key).Scan(&description); err != nil {
		t.Fatal(err)
	}
	return description
}

// stage9FKitchenSnapshot resume lo que la 0022 no debe tocar: terminales, usuarios, estaciones y la
// selección de estaciones de cada terminal.
func stage9FKitchenSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT concat_ws(' | ',
            (SELECT string_agg(name||':'||home_screen||':'||keyboard_mode||':'||coalesce(register_id::text,'-'),',' ORDER BY name)
                FROM app.devices),
            (SELECT string_agg(username||':'||role_id||':'||active,',' ORDER BY username) FROM app.users),
            (SELECT string_agg(name||':'||active||':'||revision,',' ORDER BY name) FROM app.kitchen_stations),
            (SELECT string_agg(d.name||'='||s.name,',' ORDER BY d.name,s.name) FROM app.kitchen_device_stations ds
                JOIN app.devices d ON d.id=ds.device_id JOIN app.kitchen_stations s ON s.id=ds.station_id))`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// La terminal que ya tenía estaciones conserva su selección (marca en falso); las demás muestran todas.
// El aviso sonoro nace desactivado y las columnas no admiten NULL.
func checkStage9FTerminalStations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var terminals string
	if err := pool.QueryRow(ctx, `SELECT string_agg(name||':'||kitchen_all_stations||':'||kitchen_sound_alert,',' ORDER BY name)
        FROM app.devices`).Scan(&terminals); err != nil {
		t.Fatal(err)
	}
	if terminals != "Caja principal:true:false,Pantalla de barra:false:false,Terminal revocada:true:false" {
		t.Fatalf("estaciones de las terminales después de la 0022: %s", terminals)
	}
	for _, statement := range []string{
		"UPDATE app.devices SET kitchen_all_stations=NULL",
		"UPDATE app.devices SET kitchen_sound_alert=NULL",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("la base aceptó %s", statement)
		}
	}
}

// Los usuarios existentes atienden todas las estaciones; la tabla de selección tiene llaves con nombre
// hacia usuarios y estaciones y no admite repetidos.
func checkStage9FUserStations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var restricted, rows, named int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.users WHERE NOT kitchen_all_stations),
            (SELECT count(*) FROM app.kitchen_user_stations),
            (SELECT count(*) FROM pg_constraint WHERE conrelid='app.kitchen_user_stations'::regclass
                AND conname IN ('kitchen_user_stations_pkey','kitchen_user_stations_user_id_fkey','kitchen_user_stations_station_id_fkey'))`).
		Scan(&restricted, &rows, &named); err != nil {
		t.Fatal(err)
	}
	if restricted != 0 || rows != 0 || named != 3 {
		t.Fatalf("usuarios con selección %d, filas %d, restricciones con nombre %d", restricted, rows, named)
	}
	insert := "INSERT INTO app.kitchen_user_stations(user_id,station_id) VALUES ($1,$2)"
	cook, bar := "00000000-0000-4000-8000-000000000011", "00000000-0000-4000-8000-000000000032"
	if _, err := pool.Exec(ctx, insert, cook, bar); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, insert, cook, bar); err == nil {
		t.Fatal("la base aceptó la misma estación dos veces para el usuario")
	}
	if _, err := pool.Exec(ctx, insert, cook, "00000000-0000-4000-8000-0000000000ff"); err == nil {
		t.Fatal("la base aceptó una estación inexistente")
	}
	if _, err := pool.Exec(ctx, insert, "00000000-0000-4000-8000-0000000000fe", bar); err == nil {
		t.Fatal("la base aceptó un usuario inexistente")
	}
	if _, err := pool.Exec(ctx, "UPDATE app.users SET kitchen_all_stations=NULL"); err == nil {
		t.Fatal("la base aceptó una marca de estaciones vacía en el usuario")
	}
}

// stage9FExistingKitchen son datos de una instalación en esquema 21: dos estaciones, una terminal de barra
// con su estación asignada (el ámbito anterior), una caja sin estaciones, una terminal revocada y dos
// usuarios.
const stage9FExistingKitchen = `
INSERT INTO app.establishment(name,timezone) VALUES ('Restaurante de prueba','America/Monterrey');
INSERT INTO app.roles(id,code,name) VALUES ('00000000-0000-4000-8000-000000000001','administrator','Administrador'),
    ('00000000-0000-4000-8000-000000000002','kitchen_staff','Cocina de prueba');
INSERT INTO app.users(id,username,display_name,role_id) VALUES
    ('00000000-0000-4000-8000-000000000011','cocinera','Cocinera','00000000-0000-4000-8000-000000000002'),
    ('00000000-0000-4000-8000-000000000012','gerente','Gerente','00000000-0000-4000-8000-000000000001');
INSERT INTO app.registers(id,name) VALUES ('00000000-0000-4000-8000-000000000021','Caja de prueba');
INSERT INTO app.devices(id,name,secret_hash,register_id,home_screen,keyboard_mode) VALUES
    ('00000000-0000-4000-8000-000000000022','Caja principal','hash-1','00000000-0000-4000-8000-000000000021','tables','auto'),
    ('00000000-0000-4000-8000-000000000023','Pantalla de barra','hash-2',NULL,'kitchen','disabled'),
    ('00000000-0000-4000-8000-000000000024','Terminal revocada','hash-3',NULL,'tables','auto');
UPDATE app.devices SET revoked_at=now() WHERE id='00000000-0000-4000-8000-000000000024';
INSERT INTO app.kitchen_stations(id,name,reception) VALUES
    ('00000000-0000-4000-8000-000000000031','Cocina caliente','kds'),
    ('00000000-0000-4000-8000-000000000032','Barra','kds');
INSERT INTO app.kitchen_device_stations(device_id,station_id) VALUES
    ('00000000-0000-4000-8000-000000000023','00000000-0000-4000-8000-000000000032');
`
