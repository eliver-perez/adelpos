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
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/migrations"
)

// Etapa 9B (fase A): actualización desde el esquema 17 con documentos impresos existentes. La 0018
// siembra las diez plantillas del sistema activas con identificadores fijos y el layout exacto de
// documents.Default (el formato anterior), deja los tickets y trabajos existentes sin versión ni
// layout, agrega los dos tipos de ticket nuevos, hace inmutables las versiones y concede los seis
// permisos documents.* solo al administrador. Deja la base vacía al terminar.
func TestStage9BTemplatesUpgrade(t *testing.T) {
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
	schema17 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 17 {
			schema17[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema17); err != nil || n != 17 {
		t.Fatalf("no se preparó el esquema 17: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.roles(id,code,name) VALUES ('00000000-0000-4000-8000-000000000001','administrator','Administrador'),
            ('00000000-0000-4000-8000-000000000002','cashier','Cajero');
        INSERT INTO app.role_permissions(role_id,permission_key) SELECT '00000000-0000-4000-8000-000000000001',key FROM app.permissions;
        INSERT INTO app.role_permissions(role_id,permission_key) VALUES ('00000000-0000-4000-8000-000000000002','payments.collect');
        INSERT INTO app.kitchen_printers(id,name,transport,destination) VALUES ('00000000-0000-4000-8000-000000000031','Tickets','cups','COLA');
        INSERT INTO app.kitchen_tickets(id,kind,station_ids,document) VALUES ('00000000-0000-4000-8000-000000000041','receipt','{}','{"content":"TICKET"}');
        INSERT INTO app.kitchen_print_jobs(id,ticket_id,printer_id,printer_snapshot,content) VALUES
            ('00000000-0000-4000-8000-000000000051','00000000-0000-4000-8000-000000000041','00000000-0000-4000-8000-000000000031','{}','TICKET')`); err != nil {
		t.Fatal(err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-17 {
		t.Fatalf("las migraciones de la etapa 9B no se aplicaron sobre el esquema 17: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("las migraciones de la etapa 9B no son repetibles")
	}
	checkStage9BSeed(ctx, t, pool)
	checkStage9BImmutability(ctx, t, pool)
	checkStage9BExistingDocuments(ctx, t, pool)
	checkStage9BPermissions(ctx, t, pool)
}

// La semilla es idéntica a las plantillas predeterminadas de Go.
func checkStage9BSeed(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var templates, versions int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.document_templates WHERE system AND active AND revision=1),
        (SELECT count(*) FROM app.document_template_versions WHERE number=1 AND created_by IS NULL)`).Scan(&templates, &versions); err != nil {
		t.Fatal(err)
	}
	if templates != 10 || versions != 10 {
		t.Fatalf("Semilla incompleta: %d plantillas y %d versiones", templates, versions)
	}
	for _, info := range documents.Kinds {
		var equal bool
		var kind, current string
		err := pool.QueryRow(ctx, `SELECT t.kind,t.current_version_id::text,v.layout=$3::jsonb FROM app.document_templates t
            JOIN app.document_template_versions v ON v.template_id=t.id WHERE t.id=$1 AND v.id=$2`,
			documents.SystemTemplateID(info.Kind), documents.SystemVersionID(info.Kind), string(documents.Default(info.Kind).Encode())).
			Scan(&kind, &current, &equal)
		if err != nil {
			t.Fatalf("Falta la plantilla del sistema de %s: %v", info.Kind, err)
		}
		if kind != string(info.Kind) || current != documents.SystemVersionID(info.Kind) || !equal {
			t.Fatalf("La plantilla sembrada de %s no es la predeterminada de Go", info.Kind)
		}
		var stored []byte
		if err := pool.QueryRow(ctx, "SELECT layout FROM app.document_template_versions WHERE id=$1", documents.SystemVersionID(info.Kind)).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		parsed, err := documents.ParseTemplate(info.Kind, stored)
		if err != nil || string(parsed.Encode()) != string(documents.Default(info.Kind).Encode()) {
			t.Fatalf("El layout sembrado de %s no es válido o difiere al normalizarlo: %v", info.Kind, err)
		}
	}
}

// Las versiones son inmutables, las plantillas no se borran y las del sistema son de solo lectura;
// hay una sola plantilla activa por tipo.
func checkStage9BImmutability(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	version := documents.SystemVersionID(documents.KindReceipt)
	for _, statement := range []string{
		"UPDATE app.document_template_versions SET note='x' WHERE id='" + version + "'",
		"DELETE FROM app.document_template_versions WHERE id='" + version + "'",
		"TRUNCATE app.document_template_versions CASCADE",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil || !strings.Contains(err.Error(), "inmutables") {
			t.Fatalf("%s no se rechazó: %v", statement, err)
		}
	}
	system := documents.SystemTemplateID(documents.KindReceipt)
	for statement, fragment := range map[string]string{
		"UPDATE app.document_templates SET name='Otra' WHERE id='" + system + "'":     "solo lectura",
		"UPDATE app.document_templates SET kind='precheck' WHERE id='" + system + "'": "tipo",
		"DELETE FROM app.document_templates WHERE id='" + system + "'":                "no se eliminan",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil || !strings.Contains(err.Error(), fragment) {
			t.Fatalf("%s no se rechazó: %v", statement, err)
		}
	}
	// Una segunda plantilla activa del mismo tipo viola el índice único parcial.
	_, err := pool.Exec(ctx, `INSERT INTO app.users(id,username,display_name,role_id) VALUES
        ('00000000-0000-4000-8000-000000000061','autor','Autor','00000000-0000-4000-8000-000000000001');
        INSERT INTO app.document_templates(id,kind,name,active,current_version_id,created_by) VALUES
        ('00000000-0000-4000-8000-000000000062','receipt','Segunda',true,'00000000-0000-4000-8000-000000000063','00000000-0000-4000-8000-000000000061');
        INSERT INTO app.document_template_versions(id,template_id,number,layout,created_by) VALUES
        ('00000000-0000-4000-8000-000000000063','00000000-0000-4000-8000-000000000062',1,'{}','00000000-0000-4000-8000-000000000061')`)
	if err == nil || !strings.Contains(err.Error(), "document_templates_active_key") {
		t.Fatalf("Se aceptaron dos plantillas activas del mismo tipo: %v", err)
	}
	// La versión vigente debe pertenecer a la misma plantilla (llave compuesta diferida).
	_, err = pool.Exec(ctx, `BEGIN;
        INSERT INTO app.users(id,username,display_name,role_id) VALUES
        ('00000000-0000-4000-8000-000000000064','autora','Autora','00000000-0000-4000-8000-000000000001');
        INSERT INTO app.document_templates(id,kind,name,current_version_id,created_by) VALUES
        ('00000000-0000-4000-8000-000000000065','receipt','Ajena','`+version+`','00000000-0000-4000-8000-000000000064');
        COMMIT`)
	if err == nil || !strings.Contains(err.Error(), "document_templates_current_version_fkey") {
		t.Fatalf("Se aceptó una versión vigente de otra plantilla: %v", err)
	}
	_, _ = pool.Exec(ctx, "ROLLBACK")
}

// Los documentos y trabajos anteriores conservan versión y layout NULL; la cola acepta los tipos
// nuevos y rechaza un layout que no es un objeto.
func checkStage9BExistingDocuments(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var version, layout *string
	if err := pool.QueryRow(ctx, `SELECT t.template_version_id::text,j.layout::text FROM app.kitchen_tickets t
        JOIN app.kitchen_print_jobs j ON j.ticket_id=t.id WHERE t.id='00000000-0000-4000-8000-000000000041'`).Scan(&version, &layout); err != nil {
		t.Fatal(err)
	}
	if version != nil || layout != nil {
		t.Fatal("La migración alteró un documento existente")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.kitchen_tickets(id,kind,station_ids,document,template_version_id) VALUES
        ('00000000-0000-4000-8000-000000000042','document_sample','{}','{}',NULL),
        ('00000000-0000-4000-8000-000000000043','document_rerender','{}','{}','`+documents.SystemVersionID(documents.KindPrecheck)+`')`); err != nil {
		t.Fatalf("La cola no admite los tipos de ticket nuevos: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.kitchen_print_jobs(id,ticket_id,printer_id,printer_snapshot,content,layout) VALUES
        ('00000000-0000-4000-8000-000000000052','00000000-0000-4000-8000-000000000042','00000000-0000-4000-8000-000000000031','{}','X','[1]')`); err == nil {
		t.Fatal("Se aceptó un layout que no es un objeto")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.kitchen_tickets(id,kind,station_ids,document) VALUES
        ('00000000-0000-4000-8000-000000000044','plantilla','{}','{}')`); err == nil {
		t.Fatal("La cola aceptó un tipo de ticket desconocido")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.realtime_events(id,kind,scope) VALUES
        ('00000000-0000-4000-8000-000000000071','documents.templates.changed','documents')`); err != nil {
		t.Fatalf("El ámbito SSE documents no se admite: %v", err)
	}
}

// Los seis permisos del diseñador existen con la descripción de Go y solo los recibe el administrador.
func checkStage9BPermissions(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, permission := range documents.Permissions {
		var description string
		if err := pool.QueryRow(ctx, "SELECT description FROM app.permissions WHERE key=$1", permission.Key).Scan(&description); err != nil {
			t.Fatalf("Falta el permiso %s: %v", permission.Key, err)
		}
		if description != permission.Description {
			t.Fatalf("La descripción de %s difiere de Go: %q", permission.Key, description)
		}
	}
	var administrator, others int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE r.code='administrator'),count(*) FILTER (WHERE r.code<>'administrator')
        FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE p.permission_key LIKE 'documents.%'`).Scan(&administrator, &others); err != nil {
		t.Fatal(err)
	}
	if administrator != 6 || others != 0 {
		t.Fatalf("Permisos documents.*: %d al administrador y %d a otros roles", administrator, others)
	}
}
