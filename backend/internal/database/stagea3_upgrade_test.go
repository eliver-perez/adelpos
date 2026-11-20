//go:build integration

package database

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/product"
	"local/ad-el-pos/server/migrations"
)

// Notas de la versión 1 de las plantillas del sistema antes y después de la sección «Fase A» de la 0027 (A3-D12).
const (
	stageA3OldTemplateNote = "Formato predeterminado, igual al anterior a la etapa 9B"
	stageA3NewTemplateNote = "Formato predeterminado"
)

// Etapa A3: actualización desde el esquema 26 con roles, usuarios, terminales de los tres tipos con cada pantalla de
// inicio, sesiones, impresoras y una plantilla propia. La sección «Fase A» de la 0027 (A3-D12) reescribe la nota de la
// versión 1 de las diez plantillas del sistema, solo donde es exactamente la de la 0018, y deja otra vez activo el
// disparador que hace inmutables las versiones; la sección «Fase D» (A3-D7) pasa a Mesas los dispositivos android con
// «Cocina» o «Caja» y agrega devices_android_home_check. Ninguna otra fila de ninguna tabla cambia, ni otra columna de
// las terminales y las versiones. Deja la base vacía.
func TestStageA3Upgrade(t *testing.T) {
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
	if last := expected[len(expected)-1].Version; last < 27 || last != product.Read().SchemaVersion {
		t.Fatalf("la última migración (%d) no coincide con el esquema del producto (%d)", last, product.Read().SchemaVersion)
	}
	schema26 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 26 {
			schema26[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema26); err != nil || n != 26 {
		t.Fatalf("no se preparó el esquema 26: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stageA1ExistingData+stageA2ExistingData+stageA3ExistingData); err != nil {
		t.Fatal(err)
	}
	tables := stageA1Tables(ctx, t, pool)
	// Las dos columnas que la 0027 reescribe se comparan aparte, fila por fila.
	tables["devices"] = withoutColumn(tables["devices"], "home_screen")
	before := map[string]string{}
	for table, columns := range tables {
		before[table] = stageA1Snapshot(ctx, t, pool, table, columns)
	}
	homesBefore := stageA3Values(ctx, t, pool, "SELECT id::text,home_screen FROM app.devices")
	kindsBefore := stageA3Values(ctx, t, pool, "SELECT id::text,kind FROM app.devices")
	notesBefore := stageA3Values(ctx, t, pool, "SELECT id::text,note FROM app.document_template_versions")
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-26 {
		t.Fatalf("la 0027 no se aplicó sobre el esquema 26: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0027 no es repetible")
	}
	for table, columns := range tables {
		if after := stageA1Snapshot(ctx, t, pool, table, columns); after != before[table] {
			t.Fatalf("la 0027 cambió filas existentes de %s:\nantes   %s\ndespués %s", table, before[table], after)
		}
	}
	checkStageA3TemplateNotes(ctx, t, pool, notesBefore)
	checkStageA3AndroidHomes(ctx, t, pool, homesBefore, kindsBefore)
}

// Terminales y una plantilla propia de una instalación existente en el esquema 26: un dispositivo android con cada
// pantalla de inicio (las de Cocina y Caja pasan a Mesas), una terminal de escritorio con Caja y una de cocina con
// Cocina (se quedan), y una plantilla de ticket inactiva con dos versiones cuyas notas no son exactamente la de la 0018
// (se quedan).
const stageA3ExistingData = `
INSERT INTO app.devices(id,name,secret_hash,kind,home_screen,keyboard_mode) VALUES
    ('00000000-0000-4000-8000-0000000000d1','Tableta de cocina','hash-a3-1','android','kitchen','auto'),
    ('00000000-0000-4000-8000-0000000000d2','Teléfono de caja','hash-a3-2','android','cash','disabled'),
    ('00000000-0000-4000-8000-0000000000d3','Tableta para llevar','hash-a3-3','android','takeaway','always'),
    ('00000000-0000-4000-8000-0000000000d4','Caja con cobro','hash-a3-4','desktop','cash','always'),
    ('00000000-0000-4000-8000-0000000000d5','Pantalla de cocina','hash-a3-5','kitchen','kitchen','disabled');
INSERT INTO app.document_templates(id,kind,name,active,current_version_id,created_by) VALUES
    ('00000000-0000-4000-8000-0000000000e1','receipt','Ticket propio',false,'00000000-0000-4000-8000-0000000000e3',
     '00000000-0000-4000-8000-000000000010');
INSERT INTO app.document_template_versions(id,template_id,number,layout,note,created_by) VALUES
    ('00000000-0000-4000-8000-0000000000e2','00000000-0000-4000-8000-0000000000e1',1,'{}',
     'Formato predeterminado, igual al anterior a la etapa 9B (copia)','00000000-0000-4000-8000-000000000010'),
    ('00000000-0000-4000-8000-0000000000e3','00000000-0000-4000-8000-0000000000e1',2,'{}',
     'Copia de «Predeterminada», versión 1.','00000000-0000-4000-8000-000000000010');
`

// stageA3Values lee una consulta de dos columnas de texto como un mapa de la primera a la segunda.
func stageA3Values(ctx context.Context, t *testing.T, pool *pgxpool.Pool, query string) map[string]string {
	t.Helper()
	rows, err := pool.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatal(err)
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}

// withoutColumn quita una columna de la lista separada por comas que arma stageA1Tables.
func withoutColumn(columns, column string) string {
	kept := []string{}
	for _, name := range strings.Split(columns, ",") {
		if name != column {
			kept = append(kept, name)
		}
	}
	return strings.Join(kept, ",")
}

// checkStageA3TemplateNotes (Fase A, A3-D12): solo cambia la nota exacta de la 0018 (las diez versiones del sistema);
// las demás notas quedan igual, y las versiones siguen siendo inmutables con el disparador otra vez activo.
func checkStageA3TemplateNotes(ctx context.Context, t *testing.T, pool *pgxpool.Pool, before map[string]string) {
	t.Helper()
	after := stageA3Values(ctx, t, pool, "SELECT id::text,note FROM app.document_template_versions")
	if len(after) != len(before) {
		t.Fatalf("versiones de plantilla: %d antes y %d después", len(before), len(after))
	}
	rewritten := 0
	for id, note := range before {
		want := note
		if note == stageA3OldTemplateNote {
			want = stageA3NewTemplateNote
			rewritten++
		}
		if after[id] != want {
			t.Fatalf("nota de la versión %s: %q → %q (se esperaba %q)", id, note, after[id], want)
		}
	}
	if rewritten != 10 {
		t.Fatalf("se esperaban las diez versiones del sistema con la nota de la 0018, había %d", rewritten)
	}
	var system int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM app.document_template_versions v
        JOIN app.document_templates t ON t.id=v.template_id WHERE t.system AND v.number=1 AND v.note=$1`,
		stageA3NewTemplateNote).Scan(&system); err != nil || system != 10 {
		t.Fatalf("versiones del sistema con la nota nueva: %d %v", system, err)
	}
	var enabled string
	if err := pool.QueryRow(ctx, `SELECT tgenabled::text FROM pg_trigger
        WHERE tgname='document_template_versions_insert_only' AND tgrelid='app.document_template_versions'::regclass`).
		Scan(&enabled); err != nil || enabled != "O" {
		t.Fatalf("el disparador de inmutabilidad debía quedar activo: %q %v", enabled, err)
	}
	for _, statement := range []string{
		"UPDATE app.document_template_versions SET note='x' WHERE id='00000000-0000-4000-8000-0000000000e2'",
		"DELETE FROM app.document_template_versions WHERE id='00000000-0000-4000-8000-0000000000e2'",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil || !strings.Contains(err.Error(), "inmutables") {
			t.Fatalf("%s no se rechazó después de la 0027: %v", statement, err)
		}
	}
}

// checkStageA3AndroidHomes (Fase D, A3-D7): los android con Cocina o Caja quedan en Mesas, los demás conservan la suya,
// y devices_android_home_check rechaza Cocina y Caja en un android sin tocar las terminales de escritorio y de cocina.
func checkStageA3AndroidHomes(ctx context.Context, t *testing.T, pool *pgxpool.Pool, homes, kinds map[string]string) {
	t.Helper()
	after := stageA3Values(ctx, t, pool, "SELECT id::text,home_screen FROM app.devices")
	if len(after) != len(homes) {
		t.Fatalf("terminales: %d antes y %d después", len(homes), len(after))
	}
	moved := 0
	for id, home := range homes {
		want := home
		if kinds[id] == "android" && (home == "kitchen" || home == "cash") {
			want = "tables"
			moved++
		}
		if after[id] != want {
			t.Fatalf("pantalla de inicio de %s (%s): %q → %q (se esperaba %q)", id, kinds[id], home, after[id], want)
		}
	}
	if moved != 2 || after["00000000-0000-4000-8000-0000000000d4"] != "cash" || after["00000000-0000-4000-8000-0000000000d5"] != "kitchen" {
		t.Fatalf("se esperaban 2 android en Mesas y el escritorio y la cocina sin cambio: %d %v", moved, after)
	}
	var definition string
	if err := pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
        WHERE conname='devices_android_home_check' AND conrelid='app.devices'::regclass`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "'android'") || !strings.Contains(definition, "'tables'") || !strings.Contains(definition, "'takeaway'") {
		t.Fatalf("devices_android_home_check: %s", definition)
	}
	violates := func(err error) bool {
		var problem *pgconn.PgError
		return errors.As(err, &problem) && problem.Code == "23514" && problem.ConstraintName == "devices_android_home_check"
	}
	for _, rejected := range []string{"kitchen", "cash"} {
		_, err := pool.Exec(ctx, "UPDATE app.devices SET home_screen=$1 WHERE id='00000000-0000-4000-8000-0000000000d3'", rejected)
		if !violates(err) {
			t.Fatalf("un android con %q: %v", rejected, err)
		}
	}
	_, err := pool.Exec(ctx, `INSERT INTO app.devices(id,name,secret_hash,kind,home_screen)
        VALUES ('00000000-0000-4000-8000-0000000000d9','Tableta nueva','hash-a3-9','android','kitchen')`)
	if !violates(err) {
		t.Fatalf("un android nuevo con Cocina: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE app.devices SET home_screen='takeaway' WHERE id='00000000-0000-4000-8000-0000000000d1'"); err != nil {
		t.Fatalf("un android con Para llevar: %v", err)
	}
	for _, accepted := range []string{"kitchen", "cash", "takeaway", "tables"} {
		if _, err := pool.Exec(ctx, "UPDATE app.devices SET home_screen=$1 WHERE id IN ('00000000-0000-4000-8000-0000000000d4','00000000-0000-4000-8000-0000000000d5')", accepted); err != nil {
			t.Fatalf("una terminal de escritorio o de cocina con %q: %v", accepted, err)
		}
	}
}
