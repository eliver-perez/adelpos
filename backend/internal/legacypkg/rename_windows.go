//go:build windows

package legacypkg

import (
	"errors"

	"golang.org/x/sys/windows"
)

// renameNoReplace da el nombre definitivo sin reemplazar nada: MoveFileEx sin MOVEFILE_REPLACE_EXISTING falla si el
// destino existe (os.Rename sí lo reemplazaría en Windows).
func renameNoReplace(temporary, final string) error {
	from, err := windows.UTF16PtrFromString(temporary)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(final)
	if err != nil {
		return err
	}
	err = windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
		return ErrOutputExists
	}
	return err
}
