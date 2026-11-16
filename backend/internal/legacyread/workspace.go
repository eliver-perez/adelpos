package legacyread

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/fileacl"
	"local/ad-el-pos/server/internal/legacypkg"
)

// Carpeta de trabajo de legacy-prepare: <carpeta temporal del usuario>/adel-legacy-prepare (0700; en Windows con la
// ACL privada). Tiene:
//
//   - lock: el bloqueo del sistema operativo (flock o LockFileEx) que impide dos preparaciones a la vez; el sistema lo
//     suelta solo si el proceso termina, también a la fuerza.
//   - run-<aleatorio>: una carpeta por preparación con las tablas en preparación (cifradas), la copia del respaldo si
//     Docker no pudo montar su carpeta (en claro: es el propio respaldo) y el archivo «partial» con la ruta del paquete a
//     medias.
//
// Si una preparación termina a la fuerza (SIGKILL, cierre forzado en Windows) no puede limpiar: la siguiente, o
// legacy-prepare --cleanup, borra con el bloqueo tomado todo lo que quedó (carpetas run-*, el paquete a medias que
// registraron y los contenedores y volúmenes cuyo proceso ya no existe).
const (
	workspaceName = "adel-legacy-prepare"
	lockName      = "lock"
	runPrefix     = "run-"
	partialRecord = "partial"
	// staleAge es la antigüedad a partir de la cual las carpetas temporales sueltas del formato anterior (adel-migracion-*
	// y adel-respaldo-* directamente en la carpeta temporal) se consideran restos: ninguna preparación dura tanto.
	staleAge = 6 * time.Hour
)

// errAlreadyRunning es el bloqueo ocupado por otra preparación.
var errAlreadyRunning = errors.New("el bloqueo de legacy-prepare está tomado")

type workspace struct {
	tempDir string
	root    string
	lock    *fileLock
	run     string
}

// CleanupReport es lo que borró la limpieza de restos.
type CleanupReport struct {
	Folders    int `json:"folders"`
	Partials   int `json:"partials"`
	Containers int `json:"containers"`
	Volumes    int `json:"volumes"`
}

// openWorkspace crea la carpeta de trabajo y toma el bloqueo (ALREADY_RUNNING si otra preparación lo tiene).
func openWorkspace(tempDir string) (*workspace, error) {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	root := filepath.Join(tempDir, workspaceName)
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fail(CodeWriteFailed, "no se pudo crear la carpeta de trabajo", err)
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return nil, fail(CodeWriteFailed, "la carpeta de trabajo no es una carpeta", err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		return nil, fail(CodeWriteFailed, "no se pudo proteger la carpeta de trabajo", err)
	}
	if err := fileacl.Protect(root); err != nil {
		return nil, fail(CodeWriteFailed, "no se pudo proteger la carpeta de trabajo", err)
	}
	lock, err := lockFile(filepath.Join(root, lockName))
	if errors.Is(err, errAlreadyRunning) {
		return nil, fail(CodeAlreadyRunning, "", err)
	}
	if err != nil {
		return nil, fail(CodeWriteFailed, "no se pudo tomar el bloqueo de la carpeta de trabajo", err)
	}
	return &workspace{tempDir: tempDir, root: root, lock: lock}, nil
}

// removeStale borra lo que dejaron preparaciones anteriores. Solo se llama con el bloqueo tomado: ninguna otra
// preparación puede estar usando esas carpetas.
func (space *workspace) removeStale(report *CleanupReport) error {
	entries, err := os.ReadDir(space.root)
	if err != nil {
		return fail(CodeWriteFailed, "no se pudo leer la carpeta de trabajo", err)
	}
	var failures []error
	for _, entry := range entries {
		if entry.Name() == lockName || entry.Name() == filepath.Base(space.run) {
			continue
		}
		path := filepath.Join(space.root, entry.Name())
		if entry.IsDir() && strings.HasPrefix(entry.Name(), runPrefix) {
			removed, err := removeRecordedPartial(path)
			if err != nil {
				failures = append(failures, err)
			}
			if removed {
				report.Partials++
			}
		}
		if err := os.RemoveAll(path); err != nil {
			failures = append(failures, err)
			continue
		}
		report.Folders++
	}
	// Restos del formato anterior directamente en la carpeta temporal (las tablas cifradas y la copia del respaldo),
	// solo si son viejos: otro programa podría estar usando una carpeta adel-migracion-* reciente.
	loose, _ := os.ReadDir(space.tempDir)
	for _, entry := range loose {
		if !entry.IsDir() || !(strings.HasPrefix(entry.Name(), "adel-migracion-") || strings.HasPrefix(entry.Name(), "adel-respaldo-")) {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < staleAge {
			continue
		}
		if err := os.RemoveAll(filepath.Join(space.tempDir, entry.Name())); err != nil {
			failures = append(failures, err)
			continue
		}
		report.Folders++
	}
	if len(failures) > 0 {
		return fail(CodeWriteFailed, "no se pudieron borrar algunos restos de una preparación anterior", errors.Join(failures...))
	}
	return nil
}

// removeRecordedPartial borra el paquete a medias que registró una preparación (solo si tiene la forma exacta de un
// temporal de legacypkg.CreateOutput).
func removeRecordedPartial(run string) (bool, error) {
	content, err := os.ReadFile(filepath.Join(run, partialRecord))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return legacypkg.RemoveStalePartial(strings.TrimSpace(string(content)))
}

// newRun crea la carpeta de esta preparación.
func (space *workspace) newRun() (string, error) {
	run, err := os.MkdirTemp(space.root, runPrefix)
	if err != nil {
		return "", fail(CodeWriteFailed, "no se pudo crear la carpeta de la preparación", err)
	}
	space.run = run
	return run, nil
}

// recordPartial anota la ruta del paquete a medias, para borrarlo si el proceso termina a la fuerza.
func (space *workspace) recordPartial(path string) error {
	file, err := fileacl.CreatePrivate(filepath.Join(space.run, partialRecord))
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(path + "\n")
	return errors.Join(writeErr, file.Close())
}

// close borra la carpeta de esta preparación y suelta el bloqueo.
func (space *workspace) close() error {
	var removeErr error
	if space.run != "" {
		removeErr = os.RemoveAll(space.run)
		space.run = ""
	}
	return errors.Join(removeErr, space.lock.release())
}
