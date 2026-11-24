//go:build integration

package licensing

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/database"
)

// testDatabase abre la base temporal (exige un nombre adel_pos_test_*), la deja vacía y la migra.
func testDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

// actorRows crea un usuario y una terminal: configuración, que no cuenta como operación.
func actorRows(t *testing.T, ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (string, string) {
	t.Helper()
	var role, user, device string
	if err := querier.QueryRow(ctx, `INSERT INTO app.roles(name) VALUES ('Prueba de licencias') RETURNING id`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if err := querier.QueryRow(ctx, `INSERT INTO app.users(username, display_name, role_id)
        VALUES ('licencias', 'Licencias', $1) RETURNING id`, role).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := querier.QueryRow(ctx, `INSERT INTO app.devices(name, secret_hash)
        VALUES ('Caja de prueba', 'hash-de-prueba-licencias') RETURNING id`).Scan(&device); err != nil {
		t.Fatal(err)
	}
	return user, device
}

// «Qué cuenta como operación» (datapresence.go): la configuración y la auditoría no; una cuenta o un turno
// sí. La consulta no toma ni espera los locks del negocio.
func TestDatabaseProbeCountsOnlyOperations(t *testing.T) {
	pool := testDatabase(t)
	ctx := context.Background()
	probe := PoolProbe{Pool: pool}

	found, err := probe.HasOperations(ctx)
	if err != nil || found {
		t.Fatalf("una base recién migrada no tiene operaciones: %v %v", found, err)
	}
	// La migración deja su propio registro de auditoría (0002): esa es la primera actividad.
	if _, present, err := probe.LatestActivity(ctx); err != nil || !present {
		t.Fatalf("la migración deja la primera actividad: %v %v", present, err)
	}

	// Configuración y auditoría: no son operaciones.
	if _, err := pool.Exec(ctx, `INSERT INTO app.establishment(name, timezone) VALUES ('Licencias', 'America/Monterrey')`); err != nil {
		t.Fatal(err)
	}
	user, device := actorRows(t, ctx, pool)
	occurred := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if _, err := pool.Exec(ctx, `INSERT INTO app.audit_events(occurred_at, actor_id, actor_username, actor_kind, action, entity_type, success)
        VALUES ($1, $2, 'licencias', 'user', 'setup.completed', 'establishment', true)`, occurred, user); err != nil {
		t.Fatal(err)
	}
	if found, err := probe.HasOperations(ctx); err != nil || found {
		t.Fatalf("la configuración y la auditoría no son operaciones: %v %v", found, err)
	}
	latest, present, err := probe.LatestActivity(ctx)
	if err != nil || !present || !latest.Equal(occurred) {
		t.Fatalf("la última actividad es la más reciente de la auditoría: %v %v %v", latest, present, err)
	}

	// Cada tipo de operación, en una transacción que se deshace.
	operations := map[string]func(pgx.Tx) error{
		"cuenta": func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO app.restaurant_accounts(mode, name, price_list_id, opened_by, assigned_user_id, device_id)
                VALUES ('takeaway', 'Para llevar', (SELECT id FROM app.catalog_price_lists WHERE is_default), $1, $1, $2)`, user, device)
			return err
		},
		"turno de caja": func(tx pgx.Tx) error {
			var register, operation string
			if err := tx.QueryRow(ctx, `INSERT INTO app.registers(name) VALUES ('Caja de licencias') RETURNING id`).Scan(&register); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO app.restaurant_operations(id, actor_id, device_id, route, fingerprint, response)
                VALUES (gen_random_uuid(), $1, $2, 'payments/shifts', 'prueba', '{}') RETURNING id`, user, device).Scan(&operation); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO app.cash_shifts(id, register_id, opening_float, opened_by, opened_device_id, operation_id)
                VALUES (gen_random_uuid(), $1, 0, $2, $3, $4)`, register, user, device, operation)
			return err
		},
	}
	for name, insert := range operations {
		transaction, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := insert(transaction); err != nil {
			_ = transaction.Rollback(ctx)
			t.Fatalf("%s: %v", name, err)
		}
		found, err := hasOperations(ctx, transaction)
		_ = transaction.Rollback(ctx)
		if err != nil || !found {
			t.Fatalf("%s cuenta como operación: %v %v", name, found, err)
		}
	}
	if found, err := probe.HasOperations(ctx); err != nil || found {
		t.Fatal("lo deshecho no queda")
	}

	// Una cuenta confirmada: la base tiene operaciones, también mientras otra conexión tiene el lock exclusivo
	// de la operación del restaurante (7820148305) y la fila bloqueada.
	var account string
	if err := pool.QueryRow(ctx, `INSERT INTO app.restaurant_accounts(mode, name, price_list_id, opened_by, assigned_user_id, device_id)
        VALUES ('takeaway', 'Para llevar', (SELECT id FROM app.catalog_price_lists WHERE is_default), $1, $1, $2) RETURNING id`, user, device).Scan(&account); err != nil {
		t.Fatal(err)
	}
	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(ctx)
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_xact_lock(7820148305)`); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, `SELECT 1 FROM app.restaurant_accounts WHERE id = $1 FOR UPDATE`, account); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	found, err = probe.HasOperations(ctx)
	if err != nil || !found {
		t.Fatalf("con operaciones: %v %v", found, err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("la consulta esperó a los locks del negocio (%s)", elapsed)
	}
}

// Revisión R1 (R1-3): la copia del historial de revisiones en la base sube y nunca baja.
func TestDatabaseRevisionCopyNeverGoesBack(t *testing.T) {
	pool := testDatabase(t)
	ctx := context.Background()
	probe := PoolProbe{Pool: pool}
	revisions, err := probe.AcceptedRevisions(ctx)
	if err != nil || len(revisions) != 0 {
		t.Fatalf("una base nueva no tiene revisiones: %v %v", revisions, err)
	}
	licenseID := "11111111-1111-4111-8111-111111111111"
	first, second := strings.Repeat("a", 64), strings.Repeat("b", 64)
	if err := probe.RecordAcceptedRevision(ctx, licenseID, 2, second); err != nil {
		t.Fatal(err)
	}
	// Una menor, o la misma con otro contenido, no la cambia.
	if err := probe.RecordAcceptedRevision(ctx, licenseID, 1, first); err != nil {
		t.Fatal(err)
	}
	if err := probe.RecordAcceptedRevision(ctx, licenseID, 2, first); err != nil {
		t.Fatal(err)
	}
	revisions, err = probe.AcceptedRevisions(ctx)
	if err != nil || revisions[licenseID] != (AcceptedRevision{Revision: 2, SHA256: second}) {
		t.Fatalf("la copia no baja: %v %v", revisions, err)
	}
	if err := probe.RecordAcceptedRevision(ctx, licenseID, 5, first); err != nil {
		t.Fatal(err)
	}
	if revisions, _ = probe.AcceptedRevisions(ctx); revisions[licenseID].Revision != 5 {
		t.Fatalf("la copia sube: %v", revisions)
	}
	// Los formatos inválidos los rechaza la base.
	if err := probe.RecordAcceptedRevision(ctx, licenseID, 9, "no es un sha256"); err == nil {
		t.Fatal("un sha256 inválido se rechaza")
	}
}

// Revisión R2 (R2-3): la copia de la última hora confiable en la base sube y nunca baja con RaiseTrustedTimeCopy; solo
// ReplaceTrustedTimeCopy (§6.3 y §6.4) la baja. Un gestor que arranca con estado.json editado hacia atrás toma el máximo
// de las dos copias.
func TestDatabaseTrustedTimeCopy(t *testing.T) {
	pool := testDatabase(t)
	ctx := context.Background()
	probe := PoolProbe{Pool: pool}
	if _, found, err := probe.TrustedTimeCopy(ctx); err != nil || found {
		t.Fatalf("una base nueva no tiene copia: %v %v", found, err)
	}
	later := time.Date(2026, 11, 25, 12, 0, 0, 123456789, time.UTC)
	earlier := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	if err := probe.RaiseTrustedTimeCopy(ctx, TrustedCopy{Time: later, Source: string(TimeFromOperation)}); err != nil {
		t.Fatal(err)
	}
	if err := probe.RaiseTrustedTimeCopy(ctx, TrustedCopy{Time: earlier, Source: string(TimeFromClock)}); err != nil {
		t.Fatal(err)
	}
	copied, found, err := probe.TrustedTimeCopy(ctx)
	if err != nil || !found || !copied.Time.Equal(databaseInstant(later)) || copied.Source != string(TimeFromOperation) {
		t.Fatalf("subir con una hora anterior no la baja: %+v %v %v", copied, found, err)
	}
	if err := probe.ReplaceTrustedTimeCopy(ctx, TrustedCopy{Time: earlier, Source: string(ReplaceByInternetCheck)}); err != nil {
		t.Fatal(err)
	}
	if copied, _, _ = probe.TrustedTimeCopy(ctx); !copied.Time.Equal(earlier) || copied.Source != string(ReplaceByInternetCheck) {
		t.Fatalf("la §6.3 la baja: %+v", copied)
	}
	if err := probe.RaiseTrustedTimeCopy(ctx, TrustedCopy{Time: later, Source: "a_mano"}); err == nil {
		t.Fatal("una fuente desconocida la rechaza la base")
	}
	var rows int
	if err := pool.QueryRow(ctx, "SELECT count(*)::int FROM app.license_trusted_time").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("una sola fila: %d %v", rows, err)
	}

	// Con el gestor: la copia de la base se llena al arrancar y sube cada minuto; editar estado.json hacia atrás no
	// baja la hora confiable al reiniciar.
	if _, err := pool.Exec(ctx, "DELETE FROM app.license_trusted_time"); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t)
	h.database = nil
	start := func() *Manager {
		installation := h.fixtures.installations["installation-fixture-a"]
		manager := NewManager(Options{
			Directory: h.directory, Installation: &installation, Trust: h.fixtures.testTrust(), Now: h.clock.now,
			MachineID: func() (string, error) { return h.machine, nil }, Database: probe,
		})
		if err := manager.Load(ctx); err != nil {
			t.Fatal(err)
		}
		return manager
	}
	manager := start()
	h.clock.set("2026-11-25T12:00:00Z")
	manager.tick(ctx)
	if copied, _, _ = probe.TrustedTimeCopy(ctx); !copied.Time.Equal(h.clock.now()) || copied.Source != string(TimeFromClock) {
		t.Fatalf("cada minuto la copia sube con el reloj: %+v", copied)
	}
	h.clock.set("2026-11-21T12:00:00Z")
	state := h.readState()
	state.TrustedTime = &earlier
	h.writeState(state)
	if restarted := start(); !restarted.TrustedTime().Equal(time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)) ||
		!restarted.Clock().Blocked {
		t.Fatalf("editar estado.json no baja la hora confiable: %s", restarted.TrustedTime())
	}
}
