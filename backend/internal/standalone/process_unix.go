//go:build !windows

package standalone

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// command prepara un programa de PostgreSQL con argumentos separados (nunca un shell con datos). Las
// variables PG* heredadas se descartan para que no cambien el destino (PGHOST, PGOPTIONS, PGSERVICE…):
// cada llamada indica las suyas, como el respaldo de internal/maintenance.
func command(ctx context.Context, program string, arguments []string, environment []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, program, arguments...)
	inherited := []string{}
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(variable), "PG") {
			inherited = append(inherited, variable)
		}
	}
	cmd.Env = append(inherited, environment...)
	return cmd
}

// runCaptured ejecuta un programa que termina por sí solo (initdb, pg_dump, pg_restore) y devuelve su
// salida para explicar un error. No se usa con pg_ctl start: PostgreSQL heredaría la tubería y la espera
// no terminaría hasta que PostgreSQL se detenga.
func runCaptured(ctx context.Context, program string, arguments []string, environment []string) ([]byte, error) {
	cmd := command(ctx, program, arguments, environment)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.Bytes(), err
}

// runLogged ejecuta un programa con la salida en un archivo de bitácora (sin tuberías).
func runLogged(ctx context.Context, logFile string, program string, arguments []string) error {
	output, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return errors.New("no se pudo abrir la bitácora de PostgreSQL")
	}
	defer output.Close()
	cmd := command(ctx, program, arguments, []string{"LC_MESSAGES=C"})
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

// postmasterAlive indica si el PID de postmaster.pid es un proceso vivo de PostgreSQL. pg_ctl separa a
// PostgreSQL en su propia sesión, así que puede seguir vivo si el servidor terminó mal (por ejemplo, con
// SIGKILL): entonces se adopta. Un postmaster.pid solo se borra si su proceso ya no existe o es claramente
// otro programa; si hubiera un PostgreSQL vivo con ese PID nunca se borra (otro postmaster sobre la misma
// carpeta podría dañar los datos), aunque fuera el de otra versión de Homebrew.
func postmasterAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	name := processName(pid)
	// Sin nombre (no se pudo consultar) se considera vivo: es la opción que no borra nada.
	return name == "" || name == "postgres" || name == "postmaster"
}

// processName es el nombre corto del proceso (sin ruta); vacío si no se pudo consultar.
func processName(pid int) string {
	if runtime.GOOS == "darwin" {
		info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
		if err != nil {
			return ""
		}
		return unix.ByteSliceToString(info.Proc.P_comm[:])
	}
	text, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(text))
}

// samePath compara dos rutas por su forma canónica. En macOS el disco distingue mayúsculas solo si se
// formateó así (lo habitual es que no): para no aceptar como distinta la misma carpeta escrita con otras
// mayúsculas, la comparación no las distingue.
func samePath(first, second string) bool {
	left, right := canonicalPath(first), canonicalPath(second)
	if runtime.GOOS == "darwin" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

// canonicalPath es la ruta absoluta con los enlaces simbólicos resueltos (/tmp → /private/tmp en macOS).
// Si la ruta todavía no existe, se resuelve la parte que sí existe y se le agrega el resto.
func canonicalPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved
	}
	parent := filepath.Dir(absolute)
	if parent == absolute {
		return absolute
	}
	return filepath.Join(canonicalPath(parent), filepath.Base(absolute))
}

// protectDataDir deja la carpeta de datos solo para la cuenta del servicio (0700): MkdirAll no cambia los
// permisos de una carpeta que ya existía (por ejemplo, la que crea el instalador).
func protectDataDir(directory string) error {
	if err := os.Chmod(directory, 0700); err != nil {
		return errors.New("no se pudieron proteger los permisos de la carpeta de datos")
	}
	return nil
}

// privateFile deja un archivo solo para la cuenta del servidor (0600), aunque lo haya creado otro
// programa con la máscara de la consola (pg_dump).
func privateFile(filename string) error {
	if err := os.Chmod(filename, 0600); err != nil {
		return errors.New("no se pudieron proteger los permisos de un archivo privado")
	}
	return nil
}

// checkPostgresTools explica la falta de PostgreSQL en macOS: no viaja en el paquete (decisión D3).
func checkPostgresTools(layout Layout) error {
	if _, err := os.Stat(layout.tool("initdb")); err != nil {
		return errors.New("no se encontró PostgreSQL 18 en este equipo; instálalo (Homebrew postgresql@18) " +
			"o indica su carpeta con postgres_bin o ADEL_PG_BIN")
	}
	return nil
}

// HostLock garantiza un solo servidor por carpeta de datos: un bloqueo exclusivo (flock) sobre host.lock
// que el sistema libera solo si el proceso termina, aunque termine mal.
type HostLock struct {
	file *os.File
}

// ErrLocked indica que otro proceso (el servicio u otro comando) ya tiene la carpeta de datos.
var ErrLocked = errors.New("la carpeta de datos ya está en uso por otro servidor de AD-EL POS")

// AcquireLock toma el bloqueo exclusivo de la carpeta de datos (la crea si falta).
func AcquireLock(layout Layout) (*HostLock, error) {
	if err := os.MkdirAll(layout.DataDir, 0700); err != nil {
		return nil, errors.New("no se pudo crear la carpeta de datos de AD-EL POS")
	}
	return acquireLock(layout.lockFile())
}

func acquireLock(filename string) (*HostLock, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("no se pudo abrir el bloqueo de la carpeta de datos")
	}
	// flock pertenece al archivo abierto: otro open del mismo proceso tampoco lo obtiene.
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, errors.New("no se pudo tomar el bloqueo de la carpeta de datos")
	}
	return &HostLock{file: file}, nil
}

func (lock *HostLock) Release() {
	if lock == nil || lock.file == nil {
		return
	}
	_ = unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
	lock.file.Close()
	lock.file = nil
}

// isPostmaster indica si el PID de postmaster.pid es un PostgreSQL vivo (postmasterAlive).
func (cluster *Cluster) isPostmaster(pid int) bool {
	return postmasterAlive(pid)
}
