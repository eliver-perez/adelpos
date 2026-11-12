//go:build windows

package main

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/standalone"
	"local/ad-el-pos/server/migrations"
)

// TestMigrateWithBackupProtectsExistingData simula la actualización de una instalación de Windows a una
// versión con una migración nueva: la base tiene todas las migraciones menos la última y el servidor, al
// arrancar, primero crea un respaldo comprobado («antes-de-migrar») y después migra. Una segunda llamada
// sin pendientes no respalda otra vez. Crea un clúster temporal con los binarios reales de PostgreSQL, así
// que solo corre con ADEL_STANDALONE_PG_BIN (por ejemplo «.tools\pgsql\bin»); usa el puerto 55496.
func TestMigrateWithBackupProtectsExistingData(t *testing.T) {
	pgBin := os.Getenv("ADEL_STANDALONE_PG_BIN")
	if pgBin == "" {
		t.Skip("define ADEL_STANDALONE_PG_BIN para crear un clúster temporal")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	layout, err := standalone.NewLayout(filepath.Join(t.TempDir(), "Datos de prueba"), pgBin, "AD-EL POS")
	if err != nil {
		t.Fatal(err)
	}
	layout.Port = 55496
	cluster := standalone.NewCluster(layout, logger)
	defer func() {
		_ = cluster.Stop(context.Background())
	}()
	cfg, err := standalone.Prepare(ctx, cluster, "adel_pos")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// Base de la versión anterior: todas las migraciones menos la última.
	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil || len(names) < 2 {
		t.Fatal("no se encontraron las migraciones embebidas")
	}
	older := fstest.MapFS{}
	for _, name := range names[:len(names)-1] {
		body, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		older[name] = &fstest.MapFile{Data: body}
	}
	if _, err := database.Apply(ctx, pool, older); err != nil {
		t.Fatal(err)
	}
	applied, pending, err := database.MigrationStatus(ctx, pool)
	if err != nil || applied != len(names)-1 || pending != 1 {
		t.Fatalf("estado inicial inesperado: %d aplicadas, %d pendientes, %v", applied, pending, err)
	}
	if err := migrateWithBackup(ctx, logger, layout, cfg, pool); err != nil {
		t.Fatal(err)
	}
	applied, pending, err = database.MigrationStatus(ctx, pool)
	if err != nil || applied != len(names) || pending != 0 {
		t.Fatalf("la migración pendiente no se aplicó: %d aplicadas, %d pendientes, %v", applied, pending, err)
	}
	backups := standalone.LatestBackups(layout)
	if len(backups) != 1 || !strings.HasSuffix(backups[0], "-antes-de-migrar.dump") {
		t.Fatalf("se esperaba un respaldo antes de migrar: %v", backups)
	}
	// El respaldo es de la base anterior a la migración y se puede restaurar.
	report, err := standalone.RestoreCheck(ctx, layout, cfg.Database, filepath.Join(layout.Backups(), backups[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tables) == 0 {
		t.Fatal("el respaldo restaurado no tiene tablas")
	}
	// Sin migraciones pendientes no se crea otro respaldo.
	if err := migrateWithBackup(ctx, logger, layout, cfg, pool); err != nil {
		t.Fatal(err)
	}
	if again := standalone.LatestBackups(layout); len(again) != 1 {
		t.Fatalf("no debe respaldar sin migraciones pendientes: %v", again)
	}
}
