//go:build linux

package legacypkg

import (
	"errors"

	"golang.org/x/sys/unix"
)

// renameNoReplace da el nombre definitivo sin reemplazar nada (renameat2 con RENAME_NOREPLACE, atómico). En un
// sistema de archivos que no lo admite, usa el enlace duro o la comprobación.
func renameNoReplace(temporary, final string) error {
	err := unix.Renameat2(unix.AT_FDCWD, temporary, unix.AT_FDCWD, final, unix.RENAME_NOREPLACE)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EEXIST) {
		return ErrOutputExists
	}
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
		return renameLinkOrCheck(temporary, final)
	}
	return err
}
