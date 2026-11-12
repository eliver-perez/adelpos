//go:build windows

package standalone

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestClusterLifecycleWithBundledPostgreSQL recorre el ciclo de la instalación con los binarios reales de
// PostgreSQL: crear el clúster en una carpeta con espacios y acentos, arrancarlo, preparar rol, base,
// configuración y token, respaldar, ensayar la restauración, detenerlo y recuperarse de un postmaster.pid
// huérfano. Crea un clúster temporal (unos 20 s), así que solo corre con ADEL_STANDALONE_PG_BIN, por
// ejemplo «.tools\pgsql\bin». Usa el puerto 55497 para no chocar con el clúster de desarrollo.
func TestClusterLifecycleWithBundledPostgreSQL(t *testing.T) {
	pgBin := os.Getenv("ADEL_STANDALONE_PG_BIN")
	if pgBin == "" {
		t.Skip("define ADEL_STANDALONE_PG_BIN para crear un clúster temporal")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	layout, err := NewLayout(filepath.Join(t.TempDir(), "Datos José Ñandú"), pgBin, "AD-EL POS")
	if err != nil {
		t.Fatal(err)
	}
	layout.Port = 55497
	cluster := NewCluster(layout, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer func() {
		_ = cluster.Stop(context.Background())
	}()
	cfg, err := Prepare(ctx, cluster, "adel_pos")
	if err != nil {
		t.Fatal(err)
	}
	if !cluster.Running() {
		t.Fatal("PostgreSQL debe quedar activo tras preparar")
	}
	if cfg.Database.Port != 55497 || cfg.Database.User != "adel_app" {
		t.Fatalf("configuración inesperada: %+v", cfg.Database)
	}
	// Preparar dos veces no repite la inicialización ni cambia la configuración.
	again, err := Prepare(ctx, cluster, "adel_pos")
	if err != nil || again.Database.Password != cfg.Database.Password {
		t.Fatalf("la segunda preparación debe conservar todo: %v", err)
	}
	backup, err := Backup(ctx, layout, cfg.Database, "")
	if err != nil {
		t.Fatal(err)
	}
	report, err := RestoreCheck(ctx, layout, cfg.Database, backup)
	if err != nil || !report.Equal {
		t.Fatalf("el ensayo de restauración debe coincidir: %v %+v", err, report)
	}
	if err := cluster.Stop(ctx); err != nil || cluster.Running() {
		t.Fatalf("PostgreSQL debe detenerse: %v", err)
	}
	// Un postmaster.pid de un proceso que ya no existe (corte de luz) se limpia al arrancar.
	stale := filepath.Join(layout.Cluster(), "postmaster.pid")
	if err := os.WriteFile(stale, []byte("999999\n"+layout.Cluster()+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cluster.Start(ctx); err != nil || !cluster.Running() {
		t.Fatalf("debe arrancar tras limpiar el pid huérfano: %v", err)
	}
}
