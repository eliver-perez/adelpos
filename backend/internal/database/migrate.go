package database

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/migrations"
)

type Migration struct {
	Version             int
	Name, SQL, Checksum string
}

func ReadMigrations(files fs.FS) ([]Migration, error) {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	result := make([]Migration, 0, len(names))
	for index, name := range names {
		prefix, _, found := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if !found || len(prefix) != 4 || err != nil || version != index+1 {
			return nil, errors.New("las migraciones deben ser consecutivas desde 0001")
		}
		body, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, err
		}
		result = append(result, Migration{version, name, string(body), fmt.Sprintf("%x", sha256.Sum256(body))})
	}
	if len(result) == 0 {
		return nil, errors.New("no hay migraciones incluidas")
	}
	return result, nil
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func appliedCount(ctx context.Context, db queryer, expected []Migration) (int, error) {
	rows, err := db.Query(ctx, "SELECT version, name, checksum FROM public.schema_migrations ORDER BY version")
	if err != nil {
		return 0, errors.New("registro de migraciones no disponible")
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version int
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return 0, errors.New("registro de migraciones inválido")
		}
		if count >= len(expected) || version != expected[count].Version || name != expected[count].Name || checksum != expected[count].Checksum {
			return 0, errors.New("historial incompatible, discontinuo o migración modificada")
		}
		count++
	}
	return count, rows.Err()
}

func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	expected, err := ReadMigrations(migrations.Files)
	if err != nil {
		return err
	}
	n, err := appliedCount(ctx, pool, expected)
	if err != nil {
		return err
	}
	if n != len(expected) {
		return errors.New("hay migraciones pendientes")
	}
	return nil
}

// MigrationStatus informa cuántas migraciones embebidas ya están aplicadas y cuántas faltan (etapa 9E:
// la instalación de Windows respalda la base antes de aplicar una migración sobre datos existentes).
// Una base sin registro de migraciones informa cero aplicadas.
func MigrationStatus(ctx context.Context, pool *pgxpool.Pool) (applied int, pending int, err error) {
	expected, err := ReadMigrations(migrations.Files)
	if err != nil {
		return 0, 0, err
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
		return 0, 0, errors.New("no se pudo consultar el registro de migraciones")
	}
	if !exists {
		return 0, len(expected), nil
	}
	applied, err = appliedCount(ctx, pool, expected)
	if err != nil {
		return 0, 0, err
	}
	return applied, len(expected) - applied, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	return Apply(ctx, pool, migrations.Files)
}

// Apply serializa migradores con un lock transaccional. Un fallo revierte el lote completo.
func Apply(ctx context.Context, pool *pgxpool.Pool, files fs.FS) (int, error) {
	expected, err := ReadMigrations(files)
	if err != nil {
		return 0, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, errors.New("no se pudo iniciar la transacción de migración")
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7820148301)"); err != nil {
		return 0, errors.New("no se pudo bloquear la migración")
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
		version integer PRIMARY KEY CHECK (version > 0),
		name text NOT NULL UNIQUE,
		checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return 0, errors.New("no se pudo preparar el registro de migraciones")
	}
	n, err := appliedCount(ctx, tx, expected)
	if err != nil {
		return 0, err
	}
	for _, m := range expected[n:] {
		if _, err := tx.Exec(ctx, m.SQL); err != nil {
			return 0, fmt.Errorf("falló la migración %04d; transacción revertida", m.Version)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO public.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)", m.Version, m.Name, m.Checksum); err != nil {
			return 0, errors.New("no se pudo registrar la migración")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, errors.New("no se pudo confirmar la migración")
	}
	return len(expected) - n, nil
}
