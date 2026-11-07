//go:build integration

package database

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/migrations"
)

func TestPostgresMigrationLifecycle(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(u.Path, "/adel_pos_test_") {
		t.Fatal("integration requires an isolated adel_pos_test_ database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("pool creation failed")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal("PostgreSQL unavailable")
	}
	if h := Probe(ctx, pool); h.Postgres != "available" || h.Schema != "incompatible" {
		t.Fatalf("unexpected initial state: %+v", h)
	}
	var wg sync.WaitGroup
	counts := make(chan int, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := Migrate(ctx, pool)
			counts <- n
			errs <- err
		}()
	}
	wg.Wait()
	close(counts)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	total := 0
	for n := range counts {
		total += n
	}
	expected, _ := ReadMigrations(migrations.Files)
	if total != len(expected) {
		t.Fatalf("concurrent apply count = %d", total)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("repeat migration was not a no-op")
	}
	if h := Probe(ctx, pool); h.Postgres != "available" || h.Schema != "compatible" {
		t.Fatalf("unexpected migrated state: %+v", h)
	}
	broken := fstest.MapFS{}
	for _, migration := range expected {
		broken[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
	}
	broken[fmt.Sprintf("%04d_failure.sql", len(expected)+1)] = &fstest.MapFile{Data: []byte("CREATE TABLE app.rollback_probe (id integer); SELECT missing_function();")}
	if _, err := Apply(ctx, pool, broken); err == nil {
		t.Fatal("broken migration succeeded")
	}
	var absent bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('app.rollback_probe') IS NULL").Scan(&absent); err != nil || !absent {
		t.Fatal("failed migration left partial data")
	}
	if err := CheckSchema(ctx, pool); err != nil {
		t.Fatal("failed migration changed history")
	}
	if _, err := pool.Exec(ctx, "UPDATE public.schema_migrations SET checksum='tampered' WHERE version=1"); err != nil {
		t.Fatal(err)
	}
	if _, err := Migrate(ctx, pool); err == nil {
		t.Fatal("tampered migration accepted")
	}
	if err := CheckSchema(ctx, pool); err == nil {
		t.Fatal("tampered schema marked ready")
	}
	_, _ = pool.Exec(ctx, "UPDATE public.schema_migrations SET checksum=$1 WHERE version=1", expected[0].Checksum)
	_, _ = pool.Exec(ctx, "INSERT INTO public.schema_migrations (version,name,checksum) VALUES ($1,$2,'future')", len(expected)+1, fmt.Sprintf("%04d_future.sql", len(expected)+1))
	if err := CheckSchema(ctx, pool); err == nil {
		t.Fatal("newer schema accepted")
	}
}
