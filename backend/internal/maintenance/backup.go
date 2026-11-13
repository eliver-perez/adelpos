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

// DumpBackup respalda con pg_dump en formato custom, con el mismo nombre y la misma comprobación que
// scripts/database.py backup (macOS y desarrollo). La instalación de Windows usa en su lugar el respaldo
// de internal/standalone (host_windows.go).
type DumpBackup struct {
	Database config.Database
	// PGBin es la carpeta de pg_dump y pg_restore.
	PGBin string
	// Directory es la carpeta de los respaldos.
	Directory string
	// now fija la hora del nombre en las pruebas.
	now func() time.Time
}

// NewDumpBackup arma el respaldo con la conexión de la configuración: binarios de postgres_bin, de
// ADEL_PG_BIN o de Homebrew (como scripts/database.py), y carpeta de backup_directory o «backups» junto a
// la carpeta de la configuración (.local/server.json → backups/).
func NewDumpBackup(cfg config.Config, configFile string) *DumpBackup {
	return &DumpBackup{
		Database:  cfg.Database,
		PGBin:     postgresBin(cfg.PostgresBin),
		Directory: backupDirectory(cfg.BackupDirectory, configFile),
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
	output, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", errors.New("no se pudo crear el archivo de respaldo")
	}
	environment := backup.environment()
	dump := exec.CommandContext(ctx, backup.tool("pg_dump"), "--format=custom", "--no-owner", "--no-acl")
	dump.Env = environment
	dump.Stdout = output
	var dumpErrors bytes.Buffer
	dump.Stderr = &dumpErrors
	runError := dump.Run()
	closeError := output.Close()
	if runError != nil || closeError != nil {
		_ = os.Remove(filename)
		return "", fmt.Errorf("pg_dump no pudo crear el respaldo: %s", lastLines(dumpErrors.String()))
	}
	info, err := os.Stat(filename)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(filename)
		return "", errors.New("el respaldo quedó vacío; se descartó")
	}
	list := exec.CommandContext(ctx, backup.tool("pg_restore"), "--list", filename)
	list.Env = environment
	list.Stdout = io.Discard
	var listErrors bytes.Buffer
	list.Stderr = &listErrors
	if err := list.Run(); err != nil {
		_ = os.Remove(filename)
		return "", fmt.Errorf("el respaldo no se pudo leer y se descartó: %s", lastLines(listErrors.String()))
	}
	// En Windows los bits 0600 no aplican: la ACL privada deja el archivo solo para el usuario.
	if err := fileacl.Protect(filename); err != nil {
		_ = os.Remove(filename)
		return "", err
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
