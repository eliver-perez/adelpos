//go:build integration

package database

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// checkStage9GBackups comprueba la sección F de la 0023 (respaldos programados): una sola fila de configuración con
// los valores de una instalación nueva (activos, a la «Hora de inicio del día», 30 conservados, carpeta
// predeterminada), el registro de ejecuciones vacío y las restricciones que impiden un registro incoherente.
func checkStage9GBackups(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var rows, keep int
	var enabled, timeIsNull, directoryIsNull bool
	err := pool.QueryRow(ctx, `SELECT count(*) OVER (), enabled, scheduled_time IS NULL, keep_count, directory IS NULL
        FROM app.backup_settings`).Scan(&rows, &enabled, &timeIsNull, &keep, &directoryIsNull)
	if err != nil || rows != 1 || !enabled || !timeIsNull || keep != 30 || !directoryIsNull {
		t.Fatalf("configuración inicial de respaldos: %d %v %v %d %v %v", rows, enabled, timeIsNull, keep, directoryIsNull, err)
	}
	var hasInstallation, hasEnabledAt bool
	if err := pool.QueryRow(ctx, `SELECT installation_id IS NOT NULL, enabled_at <= now() FROM app.backup_settings`).
		Scan(&hasInstallation, &hasEnabledAt); err != nil || !hasInstallation || !hasEnabledAt {
		t.Fatalf("identificador de la instalación y fecha de activación: %v %v %v", hasInstallation, hasEnabledAt, err)
	}
	var runs int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM app.backup_runs").Scan(&runs); err != nil || runs != 0 {
		t.Fatalf("registro de respaldos inicial: %d %v", runs, err)
	}
	rejected := map[string]string{
		"segunda fila de configuración": `INSERT INTO app.backup_settings(singleton) VALUES (false)`,
		"conservar cero":                `UPDATE app.backup_settings SET keep_count = 0`,
		"hora con segundos":             `UPDATE app.backup_settings SET scheduled_time = '05:00:30'`,
		"carpeta con salto de renglón":  `UPDATE app.backup_settings SET directory = E'/Volumes/USB\nx'`,
		"automático sin instante": `INSERT INTO app.backup_runs(id, kind, status, started_at)
            VALUES (gen_random_uuid(), 'automatic', 'running', now())`,
		"manual sin clave": `INSERT INTO app.backup_runs(id, kind, status, started_at, requested_by, requested_device)
            VALUES (gen_random_uuid(), 'manual', 'running', now(), '00000000-0000-4000-8000-000000000011',
                '00000000-0000-4000-8000-000000000022')`,
		"correcto sin verificar": `INSERT INTO app.backup_runs(id, kind, status, scheduled_for, started_at, finished_at,
                directory, file_name, size_bytes)
            VALUES (gen_random_uuid(), 'automatic', 'succeeded', now(), now(), now(), '/r', 'a.dump', 10)`,
		"fallido con archivo": `INSERT INTO app.backup_runs(id, kind, status, scheduled_for, started_at, finished_at,
                failure_reason, file_name)
            VALUES (gen_random_uuid(), 'automatic', 'failed', now(), now(), now(), 'dump_failed', 'a.dump')`,
		"predeterminada sin carpeta elegida": `INSERT INTO app.backup_runs(id, kind, status, scheduled_for, started_at,
                finished_at, directory, file_name, size_bytes, verified, used_default_directory, fallback_reason)
            VALUES (gen_random_uuid(), 'automatic', 'succeeded', now(), now(), now(), '/r', 'a.dump', 10, true, true,
                'missing')`,
		"fin antes del inicio": `INSERT INTO app.backup_runs(id, kind, status, scheduled_for, started_at, finished_at,
                failure_reason)
            VALUES (gen_random_uuid(), 'automatic', 'failed', now(), now(), now() - interval '1 minute', 'dump_failed')`,
		"temporal en un fallido": `INSERT INTO app.backup_runs(id, kind, status, scheduled_for, started_at, finished_at,
                failure_reason, directory, partial_file)
            VALUES (gen_random_uuid(), 'automatic', 'failed', now(), now(), now(), 'interrupted', '/r', 'a.dump.partial')`,
		"temporal con carpeta": `INSERT INTO app.backup_runs(id, kind, status, scheduled_for, started_at, directory, partial_file)
            VALUES (gen_random_uuid(), 'automatic', 'running', now(), now(), '/r', '../a.dump.partial')`,
		"temporal sin carpeta": `INSERT INTO app.backup_runs(id, kind, status, scheduled_for, started_at, partial_file)
            VALUES (gen_random_uuid(), 'automatic', 'running', now(), now(), 'a.dump.partial')`,
		"nombre con carpeta": `INSERT INTO app.backup_runs(id, kind, status, scheduled_for, started_at, finished_at,
                directory, file_name, size_bytes, verified)
            VALUES (gen_random_uuid(), 'automatic', 'succeeded', now(), now(), now(), '/r', '../a.dump', 10, true)`,
	}
	for name, statement := range rejected {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, statement)
		_ = tx.Rollback(ctx)
		if err == nil {
			t.Fatalf("la 0023 aceptó un registro de respaldos incoherente: %s", name)
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO app.backup_runs(id, kind, status, started_at, finished_at, requested_directory,
            directory, used_default_directory, fallback_reason, file_name, size_bytes, verified, operation_id,
            requested_by, requested_device)
        VALUES (gen_random_uuid(), 'manual', 'succeeded', now(), now(), '/Volumes/USB/Respaldos', '/r', true, 'missing',
            'adel_pos-20261114T110000Z-a1b2c3-manual.dump', 10, true, gen_random_uuid(),
            '00000000-0000-4000-8000-000000000011', '00000000-0000-4000-8000-000000000022')`)
	if err != nil {
		t.Fatalf("respaldo manual coherente rechazado: %v", err)
	}
}
