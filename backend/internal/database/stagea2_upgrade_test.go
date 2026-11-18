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

// Etapa A2: actualización desde el esquema 25 con roles, usuarios, terminales (de escritorio y android), sesiones e
// impresoras existentes. La parte C de la 0026 agrega la impresora de precuentas de cada dispositivo
// (devices.precheck_printer_id, vacía en todos), con su llave foránea, su índice y la restricción de que solo un
// dispositivo android la tenga (A2-D3); la parte D, el registro de platillos listos (app.ready_notices, vacío, con
// UNIQUE (unit_id, version_id)) y «Avisar platillos listos», «Sonido» y «Vibración» de cada terminal (activados). No
// cambia ninguna fila existente de ninguna tabla. Deja la base vacía.
func TestStageA2Upgrade(t *testing.T) {
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
	if last := expected[len(expected)-1].Version; last < 26 || last != product.Read().SchemaVersion {
		t.Fatalf("la última migración (%d) no coincide con el esquema del producto (%d)", last, product.Read().SchemaVersion)
	}
	schema25 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 25 {
			schema25[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema25); err != nil || n != 25 {
		t.Fatalf("no se preparó el esquema 25: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stageA1ExistingData+stageA2ExistingData); err != nil {
		t.Fatal(err)
	}
	tables := stageA1Tables(ctx, t, pool)
	before := map[string]string{}
	for table, columns := range tables {
		before[table] = stageA1Snapshot(ctx, t, pool, table, columns)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-25 {
		t.Fatalf("la 0026 no se aplicó sobre el esquema 25: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0026 no es repetible")
	}
	for table, columns := range tables {
		if after := stageA1Snapshot(ctx, t, pool, table, columns); after != before[table] {
			t.Fatalf("la 0026 cambió filas existentes de %s:\nantes   %s\ndespués %s", table, before[table], after)
		}
	}
	checkStageA2PrecheckPrinter(ctx, t, pool)
	checkStageA2ReadyNotices(ctx, t, pool)
}

// checkStageA2ReadyNotices (parte D): la tabla vacía con su identidad, sus índices y su restricción única, y los tres
// ajustes de avisos activados en todas las terminales existentes.
func checkStageA2ReadyNotices(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var notices, disabled int
	var identity, unique, recipient string
	err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.ready_notices),
            (SELECT count(*) FROM app.devices WHERE NOT (ready_alerts_enabled AND ready_alerts_sound AND ready_alerts_vibration)),
            (SELECT identity_generation FROM information_schema.columns WHERE table_schema='app' AND table_name='ready_notices'
                AND column_name='sequence'),
            coalesce((SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname='ready_notices_unit_version_key'),''),
            coalesce((SELECT indexdef FROM pg_indexes WHERE schemaname='app' AND indexname='ready_notices_recipient_idx'),'')`).
		Scan(&notices, &disabled, &identity, &unique, &recipient)
	if err != nil {
		t.Fatal(err)
	}
	if notices != 0 || disabled != 0 || identity != "ALWAYS" || unique != "UNIQUE (unit_id, version_id)" ||
		!strings.Contains(recipient, "(recipient_user_id, sequence)") {
		t.Fatalf("ready_notices: %d avisos, %d terminales con avisos apagados, identidad %q, %q, %q", notices, disabled,
			identity, unique, recipient)
	}
	// Un aviso de una unidad que no existe (23503: llave foránea).
	_, err = pool.Exec(ctx, `INSERT INTO app.ready_notices(unit_id,version_id,account_id,recipient_user_id)
        VALUES (gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),'00000000-0000-4000-8000-0000000000a1')`)
	var problem *pgconn.PgError
	if !errors.As(err, &problem) || problem.Code != "23503" {
		t.Fatalf("aviso de una unidad que no existe: %v", err)
	}
}

// Impresoras de una instalación existente en el esquema 25: una de red y la USB de una terminal con agente.
const stageA2ExistingData = `
INSERT INTO app.kitchen_printers(id,name,transport,destination,agent_device_id) VALUES
    ('00000000-0000-4000-8000-0000000000c1','Tickets de red','escpos_tcp','192.168.1.80',NULL),
    ('00000000-0000-4000-8000-0000000000c2','USB Caja 1','agent','POS_80','00000000-0000-4000-8000-000000000021');
`

// checkStageA2PrecheckPrinter (parte C): la columna vacía en todas las terminales, su índice parcial, la llave foránea
// a kitchen_printers y la restricción de que solo un dispositivo android tenga impresora de precuentas.
func checkStageA2PrecheckPrinter(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var dataType, nullable, index string
	var filled int
	err := pool.QueryRow(ctx, `SELECT c.data_type,c.is_nullable,
            coalesce((SELECT indexdef FROM pg_indexes WHERE schemaname='app' AND indexname='devices_precheck_printer_idx'),''),
            (SELECT count(*) FROM app.devices WHERE precheck_printer_id IS NOT NULL)
        FROM information_schema.columns c
        WHERE c.table_schema='app' AND c.table_name='devices' AND c.column_name='precheck_printer_id'`).
		Scan(&dataType, &nullable, &index, &filled)
	if err != nil {
		t.Fatal(err)
	}
	if dataType != "uuid" || nullable != "YES" || filled != 0 ||
		!strings.Contains(index, "(precheck_printer_id)") || !strings.Contains(index, "WHERE (precheck_printer_id IS NOT NULL)") {
		t.Fatalf("devices.precheck_printer_id: %s %s, %d con valor, índice %q", dataType, nullable, filled, index)
	}
	violates := func(code string, err error) bool {
		var problem *pgconn.PgError
		return errors.As(err, &problem) && problem.Code == code
	}
	// Una terminal de escritorio no tiene impresora de precuentas (23514: CHECK).
	_, err = pool.Exec(ctx, `UPDATE app.devices SET precheck_printer_id='00000000-0000-4000-8000-0000000000c1'
        WHERE id='00000000-0000-4000-8000-000000000021'`)
	if !violates("23514", err) {
		t.Fatalf("una terminal de escritorio con impresora de precuentas: %v", err)
	}
	// Una impresora que no existe (23503: llave foránea).
	_, err = pool.Exec(ctx, `UPDATE app.devices SET precheck_printer_id='00000000-0000-4000-8000-0000000000c9'
        WHERE id='00000000-0000-4000-8000-0000000000b1'`)
	if !violates("23503", err) {
		t.Fatalf("una impresora de precuentas que no existe: %v", err)
	}
	// Un teléfono o tableta con la impresora de red o la de una terminal.
	for _, printer := range []string{"00000000-0000-4000-8000-0000000000c1", "00000000-0000-4000-8000-0000000000c2"} {
		if _, err := pool.Exec(ctx, `UPDATE app.devices SET precheck_printer_id=$1 WHERE id='00000000-0000-4000-8000-0000000000b1'`,
			printer); err != nil {
			t.Fatalf("impresora de precuentas de un dispositivo android: %v", err)
		}
	}
	// La impresora asignada no se borra mientras un dispositivo la tenga (se desvincula primero).
	_, err = pool.Exec(ctx, "DELETE FROM app.kitchen_printers WHERE id='00000000-0000-4000-8000-0000000000c2'")
	if !violates("23503", err) {
		t.Fatalf("borrar la impresora de precuentas de un dispositivo: %v", err)
	}
}
