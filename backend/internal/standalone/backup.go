package standalone

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/fileacl"
	"local/ad-el-pos/server/internal/maintenance"
)

// databaseEnvironment son las variables de libpq para los programas de PostgreSQL: la contraseña viaja
// por PGPASSWORD, nunca como argumento visible.
func databaseEnvironment(port int, user, password, database string) []string {
	return []string{
		"PGHOST=127.0.0.1",
		"PGPORT=" + strconv.Itoa(port),
		"PGUSER=" + user,
		"PGPASSWORD=" + password,
		"PGDATABASE=" + database,
		"PGSSLMODE=disable",
		"PGCONNECT_TIMEOUT=5",
		"PGCLIENTENCODING=UTF8",
		"LC_MESSAGES=C",
	}
}

// Backup crea un respaldo pg_dump en formato custom (el mismo de macOS) en la carpeta de respaldos y
// comprueba que su catálogo se pueda leer. label distingue los respaldos automáticos (antes de migrar).
func Backup(ctx context.Context, layout Layout, database config.Database, label string) (string, error) {
	if err := os.MkdirAll(layout.Backups(), 0700); err != nil {
		return "", errors.New("no se pudo crear la carpeta de respaldos")
	}
	suffix, err := randomHex(3)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s-%s", database.Name, time.Now().UTC().Format("20060102T150405Z"), suffix)
	if label != "" {
		name += "-" + label
	}
	filename := filepath.Join(layout.Backups(), name+".dump")
	if err := DumpFile(ctx, layout, database, filename); err != nil {
		return "", err
	}
	environment := databaseEnvironment(layout.Port, database.User, database.Password, database.Name)
	if _, err := runCaptured(ctx, layout.tool("pg_restore"), []string{"--list", filename}, environment); err != nil {
		_ = os.Remove(filename)
		return "", errors.New("el respaldo no se pudo leer; se descartó")
	}
	return filename, nil
}

// DumpFile crea el respaldo pg_dump (formato custom) de la base de la instalación en filename. Lo usan Backup (manual,
// antes de migrar y el respaldo de seguridad de restore) y los respaldos programados (etapa 9G, fase F), que eligen la
// carpeta (la predeterminada, otro disco o una USB). Si algo falla, no deja el archivo.
func DumpFile(ctx context.Context, layout Layout, database config.Database, filename string) error {
	environment := databaseEnvironment(layout.Port, database.User, database.Password, database.Name)
	return dumpToFile(ctx, layout, environment, filename)
}

// dumpToFile ejecuta pg_dump con la salida en un archivo nuevo y privado desde el primer byte (revisión de la fase F):
// se crea antes con fileacl.CreatePrivate (0600 sin importar la máscara del proceso, sin seguir enlaces ni reemplazar
// un archivo existente; en Windows con la ACL privada antes de escribir) y pg_dump escribe en su salida estándar. Con
// --file, pg_dump lo creaba con la máscara del proceso (0644 desde una consola), seguía enlaces y truncaba lo que
// hubiera, y solo al final se dejaba privado. Si pg_dump falla o el archivo queda vacío, lo borra.
func dumpToFile(ctx context.Context, layout Layout, environment []string, filename string) error {
	output, err := fileacl.CreatePrivate(filename)
	if err != nil {
		return errors.New("no se pudo crear el archivo de respaldo")
	}
	dump := command(ctx, layout.tool("pg_dump"), []string{"--format=custom", "--no-owner", "--no-acl"}, environment)
	dump.Stdout = output
	var dumpErrors bytes.Buffer
	dump.Stderr = &dumpErrors
	runError := dump.Run()
	closeError := output.Close()
	if runError != nil || closeError != nil {
		_ = os.Remove(filename)
		return fmt.Errorf("no se pudo crear el respaldo: %s", tail(dumpErrors.Bytes()))
	}
	info, err := os.Lstat(filename)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(filename)
		return errors.New("el respaldo quedó vacío; se descartó")
	}
	return nil
}

// VerifyFile lee el catálogo de un respaldo con el pg_restore de la instalación y exige los datos de la tabla de
// migraciones (maintenance.CheckListing), como los respaldos programados de serve. No borra el archivo.
func VerifyFile(ctx context.Context, layout Layout, database config.Database, filename string) error {
	environment := databaseEnvironment(layout.Port, database.User, database.Password, database.Name)
	output, err := runCaptured(ctx, layout.tool("pg_restore"), []string{"--list", filename}, environment)
	if err != nil {
		return fmt.Errorf("el respaldo no se pudo leer: %s", tail(output))
	}
	return maintenance.CheckListing(string(output))
}

// RestoreReport es el resultado del ensayo de restauración: tablas del esquema app con su conteo.
type RestoreReport struct {
	Backup string         `json:"backup"`
	Equal  bool           `json:"counts_equal"`
	Tables map[string]int `json:"tables"`
}

// tableCounts cuenta las filas de todas las tablas del esquema app de una base.
func tableCounts(ctx context.Context, layout Layout, database string) (map[string]int, error) {
	connection, err := superuserConnection(ctx, layout, database)
	if err != nil {
		return nil, err
	}
	defer connection.Close(context.Background())
	return countTables(ctx, connection)
}

// countTables cuenta las filas de todas las tablas del esquema app con una conexión abierta (la del
// superusuario de la instalación o, al pasar los datos de una instalación anterior, la de su rol).
func countTables(ctx context.Context, connection *pgx.Conn) (map[string]int, error) {
	rows, err := connection.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'app' ORDER BY tablename")
	if err != nil {
		return nil, errors.New("no se pudieron listar las tablas")
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, errors.New("no se pudieron listar las tablas")
	}
	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		var count int
		query := "SELECT count(*) FROM app." + pgx.Identifier{table}.Sanitize()
		if err := connection.QueryRow(ctx, query).Scan(&count); err != nil {
			return nil, errors.New("no se pudo contar la tabla " + table)
		}
		counts[table] = count
	}
	return counts, nil
}

// checkBackupFile exige un respaldo existente dentro de la carpeta de respaldos de la instalación.
func checkBackupFile(layout Layout, filename string) (string, error) {
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return "", errors.New("ruta de respaldo inválida")
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() || !samePath(filepath.Dir(absolute), layout.Backups()) {
		return "", errors.New("elige un respaldo existente de la carpeta de respaldos de AD-EL POS")
	}
	return absolute, nil
}

// restoreInto restaura un respaldo en una base vacía del rol de la aplicación.
func restoreInto(ctx context.Context, layout Layout, database config.Database, target, dump string) error {
	admin, err := secretFile(layout.AdminPassword())
	if err != nil {
		return err
	}
	environment := databaseEnvironment(layout.Port, superuser, admin, "postgres")
	output, err := runCaptured(ctx, layout.tool("pg_restore"), []string{
		"--exit-on-error", "--no-owner", "--no-acl", "--role=" + database.User, "--dbname", target, dump,
	}, environment)
	if err != nil {
		return fmt.Errorf("no se pudo restaurar el respaldo: %s", tail(output))
	}
	return nil
}

// RestoreCheck restaura el respaldo en una base temporal y compara los conteos de todas las tablas con la
// base actual, como scripts/restore-check.py en macOS. La base temporal siempre se elimina.
func RestoreCheck(ctx context.Context, layout Layout, database config.Database, filename string) (RestoreReport, error) {
	dump, err := checkBackupFile(layout, filename)
	if err != nil {
		return RestoreReport{}, err
	}
	before, err := tableCounts(ctx, layout, database.Name)
	if err != nil {
		return RestoreReport{}, err
	}
	suffix, err := randomHex(6)
	if err != nil {
		return RestoreReport{}, err
	}
	temporary := "adel_pos_test_restore_" + suffix
	connection, err := superuserConnection(ctx, layout, "postgres")
	if err != nil {
		return RestoreReport{}, err
	}
	defer connection.Close(context.Background())
	if _, err := connection.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{temporary}.Sanitize()+" OWNER "+pgx.Identifier{database.User}.Sanitize()); err != nil {
		return RestoreReport{}, errors.New("no se pudo crear la base temporal del ensayo")
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{temporary}.Sanitize()+" WITH (FORCE)")
	}()
	if err := restoreInto(ctx, layout, database, temporary, dump); err != nil {
		return RestoreReport{}, err
	}
	restored, err := tableCounts(ctx, layout, temporary)
	if err != nil {
		return RestoreReport{}, err
	}
	return RestoreReport{Backup: filepath.Base(dump), Equal: reflect.DeepEqual(before, restored), Tables: restored}, nil
}

// Restore reemplaza la base de la aplicación con un respaldo. Solo procede con el servidor detenido (lo
// garantiza el bloqueo de la carpeta que toma quien llama) y antes crea un respaldo de seguridad del
// estado actual, de modo que la restauración se puede deshacer.
func Restore(ctx context.Context, layout Layout, database config.Database, filename string) (string, map[string]int, error) {
	dump, err := checkBackupFile(layout, filename)
	if err != nil {
		return "", nil, err
	}
	safety, err := Backup(ctx, layout, database, "antes-de-restaurar")
	if err != nil {
		return "", nil, err
	}
	connection, err := superuserConnection(ctx, layout, "postgres")
	if err != nil {
		return safety, nil, err
	}
	defer connection.Close(context.Background())
	name := pgx.Identifier{database.Name}.Sanitize()
	if _, err := connection.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
		return safety, nil, errors.New("no se pudo reemplazar la base; quedó como estaba")
	}
	if _, err := connection.Exec(ctx, "CREATE DATABASE "+name+" OWNER "+pgx.Identifier{database.User}.Sanitize()); err != nil {
		return safety, nil, errors.New("no se pudo crear la base; restaura el respaldo de seguridad " + filepath.Base(safety))
	}
	if err := restoreInto(ctx, layout, database, database.Name, dump); err != nil {
		return safety, nil, fmt.Errorf("%w; restaura el respaldo de seguridad %s", err, filepath.Base(safety))
	}
	counts, err := tableCounts(ctx, layout, database.Name)
	if err != nil {
		return safety, nil, err
	}
	return safety, counts, nil
}

// LatestBackups lista los respaldos de la carpeta, del más reciente al más antiguo.
func LatestBackups(layout Layout) []string {
	entries, err := os.ReadDir(layout.Backups())
	if err != nil {
		return nil
	}
	names := []string{}
	for index := len(entries) - 1; index >= 0; index-- {
		if strings.HasSuffix(entries[index].Name(), ".dump") {
			names = append(names, entries[index].Name())
		}
	}
	return names
}
