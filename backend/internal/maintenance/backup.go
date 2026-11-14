package maintenance

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/fileacl"
)

// Backuper crea el respaldo que precede a «Quitar datos de prueba» (decisión D2 de la etapa 9F) y devuelve
// solo el nombre del archivo, nunca su carpeta: la interfaz lo muestra sin rutas técnicas. Si el
// respaldo no queda bien (creado y legible), devuelve un error y no se borra nada.
type Backuper interface {
	Backup(ctx context.Context) (string, error)
}

// ScheduledBackuper es el respaldo de la instalación que usan los respaldos programados y «Respaldar ahora» (etapa
// 9G, fase F; internal/backups). Además del respaldo con nombre propio de Backuper, crea el archivo en la ruta que se
// le indica (la carpeta elegida, que puede estar en otro disco o en una USB, o la predeterminada) y lo verifica aparte,
// para registrar por separado si se creó y si se pudo leer. DumpBackup lo implementa con serve y el respaldo de la
// instalación (internal/standalone) en el host de macOS y de Windows.
type ScheduledBackuper interface {
	Backuper
	// DefaultDirectory es la carpeta predeterminada de respaldos, la misma de Backup.
	DefaultDirectory() string
	// DatabaseName es el nombre de la base, con el que empieza el nombre de cada respaldo.
	DatabaseName() string
	// Dump crea el respaldo (pg_dump en formato custom) en filename, que no debe existir, con permisos privados. Si
	// falla no deja el archivo.
	Dump(ctx context.Context, filename string) error
	// Verify lee el catálogo del respaldo con pg_restore --list y comprueba que sea de una base de AD-EL POS.
	Verify(ctx context.Context, filename string) error
	// ProtectedDirectories son las carpetas del servidor donde nunca se guardan respaldos elegidos (la carpeta de datos
	// del servicio o la de la configuración con serve: secretos, identidad, bitácoras y la base).
	ProtectedDirectories() []string
}

// verificationMarker es la entrada que todo respaldo de una base de AD-EL POS tiene en su catálogo: los datos de la
// tabla de migraciones. Un archivo que pg_restore puede leer pero sin ella (otra base, una base vacía) no se acepta.
const verificationMarker = "TABLE DATA public schema_migrations "

// listingLimit acota la salida de pg_restore --list que se conserva para buscar la entrada (el catálogo de la base
// del restaurante ocupa unos cuantos KB).
const listingLimit = 8 * 1024 * 1024

// CheckListing comprueba la salida de pg_restore --list de un respaldo: debe incluir los datos de la tabla de
// migraciones. Lo usan DumpBackup y el respaldo de la instalación (internal/standalone).
func CheckListing(listing string) error {
	for _, line := range strings.Split(listing, "\n") {
		if strings.Contains(line+" ", verificationMarker) {
			return nil
		}
	}
	return errors.New("el respaldo no contiene los datos de AD-EL POS")
}

// limitedBuffer conserva hasta listingLimit bytes de una salida y descarta el resto sin fallar.
type limitedBuffer struct {
	bytes.Buffer
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	room := listingLimit - buffer.Len()
	if room > 0 {
		if len(data) > room {
			buffer.Buffer.Write(data[:room])
		} else {
			buffer.Buffer.Write(data)
		}
	}
	return len(data), nil
}

// DumpBackup respalda con pg_dump en formato custom, con el mismo nombre y la misma comprobación que
// scripts/database.py backup (macOS y desarrollo). La instalación de Windows usa en su lugar el respaldo
// de internal/standalone (host_windows.go).
type DumpBackup struct {
	Database config.Database
	// PGBin es la carpeta de pg_dump y pg_restore.
	PGBin string
	// Directory es la carpeta de los respaldos.
	Directory string
	// ConfigDirectory es la carpeta de la configuración (.local en desarrollo): nunca se acepta como carpeta elegida.
	ConfigDirectory string
	// now fija la hora del nombre en las pruebas.
	now func() time.Time
}

// NewDumpBackup arma el respaldo con la conexión de la configuración: binarios de postgres_bin, de
// ADEL_PG_BIN o de Homebrew (como scripts/database.py), y carpeta de backup_directory o «backups» junto a
// la carpeta de la configuración (.local/server.json → backups/).
func NewDumpBackup(cfg config.Config, configFile string) *DumpBackup {
	configDirectory, err := filepath.Abs(filepath.Dir(configFile))
	if err != nil {
		configDirectory = filepath.Dir(configFile)
	}
	return &DumpBackup{
		Database:        cfg.Database,
		PGBin:           postgresBin(cfg.PostgresBin),
		Directory:       backupDirectory(cfg.BackupDirectory, configFile),
		ConfigDirectory: configDirectory,
	}
}

func postgresBin(configured string) string {
	if configured != "" {
		return configured
	}
	if value := os.Getenv("ADEL_PG_BIN"); value != "" {
		return value
	}
	if runtime.GOARCH == "arm64" {
		return "/opt/homebrew/opt/postgresql@18/bin"
	}
	return "/usr/local/opt/postgresql@18/bin"
}

func backupDirectory(configured, configFile string) string {
	if configured != "" {
		return configured
	}
	absolute, err := filepath.Abs(configFile)
	if err != nil {
		absolute = configFile
	}
	return filepath.Join(filepath.Dir(filepath.Dir(absolute)), "backups")
}

// tool es la ruta de un programa de PostgreSQL (con .exe en Windows).
func (backup *DumpBackup) tool(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(backup.PGBin, name)
}

// environment son las variables de libpq del proceso hijo: la contraseña viaja por PGPASSWORD, nunca como
// argumento visible. Las variables PG* heredadas se descartan para que no cambien el destino.
func (backup *DumpBackup) environment() []string {
	environment := []string{}
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(variable), "PG") {
			environment = append(environment, variable)
		}
	}
	database := backup.Database
	environment = append(environment,
		"PGHOST="+database.Host,
		"PGPORT="+strconv.Itoa(database.Port),
		"PGUSER="+database.User,
		"PGDATABASE="+database.Name,
		"PGPASSWORD="+database.Password,
		"PGSSLMODE="+database.SSLMode,
		"PGCONNECT_TIMEOUT=5",
		"PGAPPNAME=ad-el-pos-respaldo",
	)
	if database.SSLRootCert != "" {
		environment = append(environment, "PGSSLROOTCERT="+database.SSLRootCert)
	}
	return environment
}

// DefaultDirectory es la carpeta de los respaldos (backup_directory o «backups» junto a la configuración).
func (backup *DumpBackup) DefaultDirectory() string {
	return backup.Directory
}

// DatabaseName es el nombre de la base de la configuración.
func (backup *DumpBackup) DatabaseName() string {
	return backup.Database.Name
}

// ProtectedDirectories es la carpeta de la configuración.
func (backup *DumpBackup) ProtectedDirectories() []string {
	if backup.ConfigDirectory == "" {
		return nil
	}
	return []string{backup.ConfigDirectory}
}

// Dump crea el respaldo en filename (que no debe existir, ni como enlace) con permisos 0600 desde el primer byte (ACL
// privada en Windows, fijada antes de escribir). Si pg_dump falla o el archivo queda vacío, lo borra.
func (backup *DumpBackup) Dump(ctx context.Context, filename string) error {
	output, err := fileacl.CreatePrivate(filename)
	if err != nil {
		return errors.New("no se pudo crear el archivo de respaldo")
	}
	dump := exec.CommandContext(ctx, backup.tool("pg_dump"), "--format=custom", "--no-owner", "--no-acl")
	dump.Env = backup.environment()
	dump.Stdout = output
	var dumpErrors bytes.Buffer
	dump.Stderr = &dumpErrors
	runError := dump.Run()
	closeError := output.Close()
	if runError != nil || closeError != nil {
		_ = os.Remove(filename)
		return fmt.Errorf("pg_dump no pudo crear el respaldo: %s", lastLines(dumpErrors.String()))
	}
	info, err := os.Lstat(filename)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(filename)
		return errors.New("el respaldo quedó vacío; se descartó")
	}
	return nil
}

// Verify lee el catálogo del respaldo con pg_restore --list y exige la entrada de la tabla de migraciones. No borra
// el archivo: lo decide quien verifica.
func (backup *DumpBackup) Verify(ctx context.Context, filename string) error {
	list := exec.CommandContext(ctx, backup.tool("pg_restore"), "--list", filename)
	list.Env = backup.environment()
	var listing limitedBuffer
	list.Stdout = &listing
	var listErrors bytes.Buffer
	list.Stderr = &listErrors
	if err := list.Run(); err != nil {
		return fmt.Errorf("el respaldo no se pudo leer: %s", lastLines(listErrors.String()))
	}
	return CheckListing(listing.String())
}

// Backup crea <base>-<AAAAMMDDTHHMMSSZ>-<6 hex>.dump con permisos 0600 en la carpeta de respaldos y
// comprueba que su catálogo se pueda leer con pg_restore --list. Si algo falla, borra el archivo.
func (backup *DumpBackup) Backup(ctx context.Context) (string, error) {
	if err := os.MkdirAll(backup.Directory, 0700); err != nil {
		return "", errors.New("no se pudo crear la carpeta de respaldos")
	}
	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		return "", errors.New("no se pudo nombrar el respaldo")
	}
	now := time.Now
	if backup.now != nil {
		now = backup.now
	}
	name := fmt.Sprintf("%s-%s-%s.dump", backup.Database.Name, now().UTC().Format("20060102T150405Z"), hex.EncodeToString(suffix))
	filename := filepath.Join(backup.Directory, name)
	if err := backup.Dump(ctx, filename); err != nil {
		return "", err
	}
	list := exec.CommandContext(ctx, backup.tool("pg_restore"), "--list", filename)
	list.Env = backup.environment()
	list.Stdout = io.Discard
	var listErrors bytes.Buffer
	list.Stderr = &listErrors
	if err := list.Run(); err != nil {
		_ = os.Remove(filename)
		return "", fmt.Errorf("el respaldo no se pudo leer y se descartó: %s", lastLines(listErrors.String()))
	}
	return name, nil
}

// lastLines recorta la salida de error de un programa para la bitácora.
func lastLines(output string) string {
	text := strings.TrimSpace(output)
	if len(text) > 1000 {
		text = text[len(text)-1000:]
	}
	return text
}

// DumpBackup sirve para los respaldos programados (comprobado al compilar).
var _ ScheduledBackuper = (*DumpBackup)(nil)
