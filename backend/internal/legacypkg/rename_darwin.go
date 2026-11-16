//go:build darwin

package legacypkg

import (
	"errors"

	"golang.org/x/sys/unix"
)

// renameNoReplace da el nombre definitivo sin reemplazar nada (renamex_np con RENAME_EXCL, atómico). En un sistema de
// archivos que no lo admite (algunas USB), usa el enlace duro o la comprobación.
func renameNoReplace(temporary, final string) error {
	err := unix.RenamexNp(temporary, final, unix.RENAME_EXCL)
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
