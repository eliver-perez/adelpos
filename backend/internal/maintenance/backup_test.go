//go:build !windows

package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/config"
)

// Programas sustitutos de pg_dump y pg_restore: registran argumentos y entorno, y fallan a pedido.
const fakeDump = `#!/bin/sh
printf '%s\n' "$@" > "$FAKE_LOG.args"
env > "$FAKE_LOG.env"
if [ -n "$FAKE_DUMP_FAIL" ]; then
  echo "fallo simulado de pg_dump" >&2
  exit 1
fi
if [ -n "$FAKE_DUMP_EMPTY" ]; then
  exit 0
fi
printf 'PGDMP-respaldo-de-prueba'
`

const fakeRestore = `#!/bin/sh
if [ -n "$FAKE_RESTORE_FAIL" ]; then
  echo "archivo dañado" >&2
  exit 1
fi
head -c 5 "$2" | grep -q PGDMP
`

func fakeTools(t *testing.T) (string, string) {
	t.Helper()
	bin := t.TempDir()
	for name, body := range map[string]string{"pg_dump": fakeDump, "pg_restore": fakeRestore} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(t.TempDir(), "llamada")
	t.Setenv("FAKE_LOG", log)
	return bin, log
}

func testBackup(bin, directory string) *DumpBackup {
	return &DumpBackup{
		Database: config.Database{
			Host:     "127.0.0.1",
			Port:     55499,
			Name:     "adel_pos_test_respaldo",
			User:     "adel_app",
			Password: "secreto-solo-en-el-entorno",
			SSLMode:  "disable",
		},
		PGBin:     bin,
		Directory: directory,
		now: func() time.Time {
			return time.Date(2026, 11, 13, 18, 30, 5, 0, time.UTC)
		},
	}
}

func dumps(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// D2: el respaldo usa la conexión de la configuración con la contraseña en el entorno del proceso hijo
// (nunca en argumentos), el nombre de scripts/database.py backup, permisos 0600 y la comprobación con
// pg_restore --list.
func TestDumpBackupCreatesACheckedPrivateFile(t *testing.T) {
	bin, log := fakeTools(t)
	directory := filepath.Join(t.TempDir(), "backups")
	t.Setenv("PGSERVICE", "otro-servicio")
	t.Setenv("PGPASSWORD", "heredada")
	name, err := testBackup(bin, directory).Backup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^adel_pos_test_respaldo-20261113T183005Z-[0-9a-f]{6}\.dump$`).MatchString(name) {
		t.Fatalf("nombre del respaldo: %q", name)
	}
	if strings.ContainsAny(name, `/\`) {
		t.Fatal("el nombre incluye una carpeta")
	}
	info, err := os.Stat(filepath.Join(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permisos del respaldo: %v", info.Mode().Perm())
	}
	folder, err := os.Stat(directory)
	if err != nil || folder.Mode().Perm() != 0700 {
		t.Fatalf("permisos de la carpeta de respaldos: %v", err)
	}
	arguments, err := os.ReadFile(log + ".args")
	if err != nil {
		t.Fatal(err)
	}
	if string(arguments) != "--format=custom\n--no-owner\n--no-acl\n" {
		t.Fatalf("argumentos de pg_dump: %q", arguments)
	}
	environment, err := os.ReadFile(log + ".env")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"PGPASSWORD=secreto-solo-en-el-entorno",
		"PGHOST=127.0.0.1",
		"PGPORT=55499",
		"PGUSER=adel_app",
		"PGDATABASE=adel_pos_test_respaldo",
		"PGSSLMODE=disable",
	} {
		if !strings.Contains(string(environment), expected+"\n") {
			t.Fatalf("falta %s en el entorno de pg_dump", expected)
		}
	}
	if strings.Contains(string(environment), "PGSERVICE") || strings.Contains(string(environment), "heredada") {
		t.Fatal("el respaldo heredó variables de PostgreSQL del proceso")
	}
}

// Si pg_dump falla, queda vacío o el archivo no se puede leer, no queda ningún archivo y hay error.
func TestDumpBackupFailuresLeaveNoFile(t *testing.T) {
	for _, variable := range []string{"FAKE_DUMP_FAIL", "FAKE_DUMP_EMPTY", "FAKE_RESTORE_FAIL"} {
		t.Run(variable, func(t *testing.T) {
			bin, _ := fakeTools(t)
			directory := filepath.Join(t.TempDir(), "backups")
			t.Setenv(variable, "1")
			name, err := testBackup(bin, directory).Backup(context.Background())
			if err == nil || name != "" {
				t.Fatalf("el respaldo debía fallar: %q, %v", name, err)
			}
			if left := dumps(t, directory); len(left) != 0 {
				t.Fatalf("quedaron archivos: %v", left)
			}
		})
	}
	// Sin los programas tampoco hay respaldo.
	missing := testBackup(filepath.Join(t.TempDir(), "sin-postgres"), filepath.Join(t.TempDir(), "backups"))
	if _, err := missing.Backup(context.Background()); err == nil {
		t.Fatal("se creó un respaldo sin pg_dump")
	}
}

// Binarios: postgres_bin de la configuración, después ADEL_PG_BIN y por último Homebrew. Carpeta:
// backup_directory o «backups» junto a la carpeta de la configuración.
func TestDumpBackupLocations(t *testing.T) {
	t.Setenv("ADEL_PG_BIN", "/opt/postgresql/variable")
	cfg := config.Config{PostgresBin: "/opt/postgresql/configuracion"}
	if backup := NewDumpBackup(cfg, "/repositorio/.local/server.json"); backup.PGBin != "/opt/postgresql/configuracion" ||
		backup.Directory != "/repositorio/backups" {
		t.Fatalf("ubicaciones: %+v", backup)
	}
	if backup := NewDumpBackup(config.Config{}, "/repositorio/.local/server.json"); backup.PGBin != "/opt/postgresql/variable" {
		t.Fatalf("ADEL_PG_BIN no se usó: %s", backup.PGBin)
	}
	t.Setenv("ADEL_PG_BIN", "")
	homebrew := "/usr/local/opt/postgresql@18/bin"
	if runtime.GOARCH == "arm64" {
		homebrew = "/opt/homebrew/opt/postgresql@18/bin"
	}
	if backup := NewDumpBackup(config.Config{}, "/repositorio/.local/server.json"); backup.PGBin != homebrew {
		t.Fatalf("ruta de Homebrew: %s", backup.PGBin)
	}
	configured := config.Config{BackupDirectory: "/respaldos/del/restaurante"}
	if backup := NewDumpBackup(configured, "/repositorio/.local/server.json"); backup.Directory != "/respaldos/del/restaurante" {
		t.Fatalf("carpeta configurada: %s", backup.Directory)
	}
}

func TestOperationsReasonUsesRestaurantWords(t *testing.T) {
	reason := operationsReason([]string{"restaurant_items", "restaurant_item_versions", "kitchen_tasks", "tabla_futura"})
	if reason != "Ya tiene registros: ventas, tareas de cocina y otros registros." {
		t.Fatalf("motivo: %q", reason)
	}
	if strings.Contains(reason, "_") {
		t.Fatal("el motivo contiene nombres técnicos")
	}
}

// Etapa 9G, fase F: los respaldos programados crean el archivo en la ruta que se indica (Dump, privado y sin restos
// si falla) y lo verifican aparte (Verify): pg_restore --list debe leerlo y su catálogo debe traer los datos de la
// tabla de migraciones; un archivo legible de otra base no se acepta.
func TestDumpAndVerifyForScheduledBackups(t *testing.T) {
	bin, _ := fakeTools(t)
	listing := `#!/bin/sh
if [ -n "$FAKE_RESTORE_FAIL" ]; then
  echo "archivo dañado" >&2
  exit 1
fi
head -c 5 "$2" | grep -q PGDMP || exit 1
echo ";"
echo "; Archive created at 2026-11-14 05:00:00 CST"
if [ -z "$FAKE_RESTORE_FOREIGN" ]; then
  echo "3848; 0 817680 TABLE DATA public schema_migrations adel_app"
fi
echo "3849; 0 817689 TABLE DATA app accounts adel_app"
`
	if err := os.WriteFile(filepath.Join(bin, "pg_restore"), []byte(listing), 0700); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	backup := testBackup(bin, filepath.Join(directory, "predeterminada"))
	var scheduled ScheduledBackuper = backup
	if scheduled.DefaultDirectory() != filepath.Join(directory, "predeterminada") || scheduled.DatabaseName() != "adel_pos_test_respaldo" {
		t.Fatal("carpeta o base del respaldo programado")
	}
	filename := filepath.Join(directory, "otra-carpeta.dump.partial")
	if err := scheduled.Dump(context.Background(), filename); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filename)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("respaldo privado: %v", err)
	}
	if err := scheduled.Verify(context.Background(), filename); err != nil {
		t.Fatalf("verificación: %v", err)
	}
	t.Setenv("FAKE_RESTORE_FOREIGN", "1")
	if err := scheduled.Verify(context.Background(), filename); err == nil {
		t.Fatal("se aceptó un respaldo sin los datos de AD-EL POS")
	}
	t.Setenv("FAKE_RESTORE_FOREIGN", "")
	t.Setenv("FAKE_RESTORE_FAIL", "1")
	if err := scheduled.Verify(context.Background(), filename); err == nil {
		t.Fatal("se aceptó un respaldo ilegible")
	}
	// Verify no borra el archivo: lo decide quien verifica.
	if _, err := os.Stat(filename); err != nil {
		t.Fatal("la verificación borró el archivo")
	}
	t.Setenv("FAKE_DUMP_FAIL", "1")
	failed := filepath.Join(directory, "fallido.dump.partial")
	if err := scheduled.Dump(context.Background(), failed); err == nil {
		t.Fatal("pg_dump fallido no dio error")
	}
	if _, err := os.Stat(failed); err == nil {
		t.Fatal("quedó el archivo del pg_dump fallido")
	}
	// Un archivo que ya existe no se sobrescribe.
	if err := scheduled.Dump(context.Background(), filename); err == nil {
		t.Fatal("se sobrescribió un archivo existente")
	}
}

func TestCheckListing(t *testing.T) {
	if err := CheckListing("3848; 0 817680 TABLE DATA public schema_migrations adel_app\n"); err != nil {
		t.Fatal(err)
	}
	if err := CheckListing("3848; 0 817680 TABLE DATA public schema_migrations"); err != nil {
		t.Fatal("sin dueño al final del renglón")
	}
	for _, foreign := range []string{"", "3848; 0 1 TABLE DATA public schema_migrations_old x", "TABLE public schema_migrations x"} {
		if err := CheckListing(foreign); err == nil {
			t.Fatalf("se aceptó %q", foreign)
		}
	}
}
