//go:build integration

package catalog_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/migrations"
)

// La 0012 (etapa 8, fase B: indicaciones rápidas con efecto) se aplica sobre una instalación en
// esquema 11 con indicaciones de la etapa 7.1 sin alterarlas: quedan como solo instrucción
// ('none', sin ingrediente), con su texto, estado, orden y revisión. Como las pruebas de
// actualización de internal/database, parte de un esquema vacío en la base temporal exclusiva del
// paquete y la deja vacía al terminar.
func TestQuickNoteEffectsMigrationKeepsExistingNotes(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("No se pudo abrir el pool temporal")
	}
	defer pool.Close()
	reset := func() {
		if _, err := pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
			t.Fatal(err)
		}
	}
	reset()
	defer reset()
	expected, err := database.ReadMigrations(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	previous := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 11 {
			previous[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := database.Apply(ctx, pool, previous); err != nil || n != 11 {
		t.Fatalf("No se preparó el esquema 11: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.catalog_categories(id,name) VALUES ('00000000-0000-4000-8000-0000000000e1','Hamburguesas');
        INSERT INTO app.catalog_ingredients(id,name) VALUES ('00000000-0000-4000-8000-0000000000e2','Cebolla');
        INSERT INTO app.catalog_category_quick_notes(id,category_id,text,active,sort_order,revision) VALUES
            ('00000000-0000-4000-8000-0000000000e3','00000000-0000-4000-8000-0000000000e1','Sin cebolla',true,10,3),
            ('00000000-0000-4000-8000-0000000000e4','00000000-0000-4000-8000-0000000000e1','Bien cocida',false,20,1)`); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var rows string
		if err := pool.QueryRow(ctx, `SELECT string_agg(id||':'||category_id||':'||text||':'||active||':'||sort_order||':'||revision||':'||
            created_at||':'||updated_at, '|' ORDER BY id) FROM app.catalog_category_quick_notes`).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	before := snapshot()
	if n, err := database.Migrate(ctx, pool); err != nil || n != len(expected)-11 {
		t.Fatalf("La 0012 no se aplicó sobre el esquema 11: %d %v", n, err)
	}
	if n, err := database.Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("La 0012 no es repetible")
	}
	if after := snapshot(); after != before {
		t.Fatalf("La 0012 alteró las indicaciones existentes:\n%s\n%s", before, after)
	}
	var effects string
	if err := pool.QueryRow(ctx, `SELECT string_agg(effect||':'||coalesce(ingredient_id::text,'null'), '|' ORDER BY id)
        FROM app.catalog_category_quick_notes`).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if effects != "none:null|none:null" {
		t.Fatalf("Las indicaciones existentes no quedaron como solo instrucción: %s", effects)
	}
	var named int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname IN ('catalog_category_quick_notes_effect_check',
        'catalog_category_quick_notes_ingredient_id_fkey','catalog_category_quick_notes_effect_ingredient_check')`).Scan(&named); err != nil || named != 3 {
		t.Fatalf("Faltan restricciones con nombre explícito de la 0012: %d %v", named, err)
	}
	var indexed bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('app.catalog_category_quick_notes_ingredient_idx') IS NOT NULL").Scan(&indexed); err != nil || !indexed {
		t.Fatalf("Falta el índice del ingrediente vinculado: %v", err)
	}
	// Coherencia del efecto con su ingrediente y llave foránea, en PostgreSQL además de Go.
	insert := "INSERT INTO app.catalog_category_quick_notes(category_id,text,effect,ingredient_id) VALUES ('00000000-0000-4000-8000-0000000000e1',"
	for _, rejected := range []string{
		insert + "'Quitar cebolla','omit',NULL)",
		insert + "'Con cebolla','normal',NULL)",
		insert + "'Bien dorada','none','00000000-0000-4000-8000-0000000000e2')",
		insert + "'Doble cebolla','double','00000000-0000-4000-8000-0000000000e2')",
		insert + "'Cebolla extra','extra','00000000-0000-4000-8000-0000000000e9')",
	} {
		if _, err := pool.Exec(ctx, rejected); err == nil {
			t.Fatalf("PostgreSQL aceptó una indicación incoherente: %s", rejected)
		}
	}
	for _, accepted := range []string{
		insert + "'Quitar cebolla','omit','00000000-0000-4000-8000-0000000000e2')",
		insert + "'Cebolla extra','extra','00000000-0000-4000-8000-0000000000e2')",
		insert + "'Con cebolla','normal','00000000-0000-4000-8000-0000000000e2')",
		insert + "'Poco picante','none',NULL)",
	} {
		if _, err := pool.Exec(ctx, accepted); err != nil {
			t.Fatalf("PostgreSQL rechazó una indicación válida: %s %v", accepted, err)
		}
	}
}
