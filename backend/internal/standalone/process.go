//go:build windows

package standalone

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// command prepara un programa de PostgreSQL con argumentos separados (nunca un shell con datos) y sin
// ventana de consola: el servidor corre oculto y sus procesos hijos también (§3.1 de la 9E).
func command(ctx context.Context, program string, arguments []string, environment []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, program, arguments...)
	cmd.Env = append(os.Environ(), environment...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
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
	cmd := command(ctx, program, arguments, nil)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

// processImage es la ruta del ejecutable de un proceso activo; vacío si no existe o no se puede consultar.
func processImage(pid int) string {
	if pid <= 0 {
		return ""
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(process)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(process, &exitCode); err != nil || exitCode != 259 {
		// 259 = STILL_ACTIVE: un proceso terminado cuyo handle sigue abierto no cuenta como activo.
		return ""
	}
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buffer[:size])
}

// samePath compara rutas de Windows sin distinguir mayúsculas y con su forma larga: la misma carpeta puede
// llegar como «C:\Users\EXPEDI~1\…» (por ejemplo desde %TEMP%) o como «C:\Users\Expediente\…».
func samePath(first, second string) bool {
	return strings.EqualFold(canonicalPath(first), canonicalPath(second))
}

// canonicalPath es la ruta absoluta en forma larga. Si la ruta todavía no existe, se resuelve la parte que
// sí existe y se le agrega el resto.
func canonicalPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if long, ok := longPathName(absolute); ok {
		return long
	}
	parent := filepath.Dir(absolute)
	if parent == absolute {
		return absolute
	}
	return filepath.Join(canonicalPath(parent), filepath.Base(absolute))
}

// isASCII indica si una ruta solo tiene caracteres ASCII.
func isASCII(path string) bool {
	for index := 0; index < len(path); index++ {
		if path[index] >= 0x80 {
			return false
		}
	}
	return true
}

// asciiPath devuelve la ruta corta de Windows (8.3) de una carpeta existente con caracteres fuera de
// ASCII; si la ruta ya es ASCII, o el volumen no tiene nombres cortos, la devuelve sin cambio.
func asciiPath(path string) string {
	if isASCII(path) {
		return path
	}
	source, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size, err := windows.GetShortPathName(source, &buffer[0], uint32(len(buffer)))
	if err != nil || size == 0 || int(size) > len(buffer) {
		return path
	}
	short := windows.UTF16ToString(buffer[:size])
	if !isASCII(short) {
		return path
	}
	return short
}

func longPathName(path string) (string, bool) {
	source, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", false
	}
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size, err := windows.GetLongPathName(source, &buffer[0], uint32(len(buffer)))
	if err != nil || size == 0 || int(size) > len(buffer) {
		return "", false
	}
	return filepath.Clean(windows.UTF16ToString(buffer[:size])), true
}

// HostLock garantiza un solo servidor por carpeta de datos: un bloqueo exclusivo sobre host.lock que
// Windows libera solo si el proceso termina, aunque termine mal.
type HostLock struct {
	file *os.File
}

// ErrLocked indica que otro proceso (el servidor de la aplicación abierta) ya tiene la carpeta de datos.
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
	overlapped := new(windows.Overlapped)
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, overlapped); err != nil {
		file.Close()
		return nil, ErrLocked
	}
	return &HostLock{file: file}, nil
}

func (lock *HostLock) Release() {
	if lock == nil || lock.file == nil {
		return
	}
	overlapped := new(windows.Overlapped)
	_ = windows.UnlockFileEx(windows.Handle(lock.file.Fd()), 0, 1, 0, overlapped)
	lock.file.Close()
	lock.file = nil
}
