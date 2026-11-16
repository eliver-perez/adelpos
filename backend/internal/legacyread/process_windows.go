//go:build windows

package legacyread

import (
	"errors"

	"golang.org/x/sys/windows"
)

// stillActive es el código de salida de un proceso que no ha terminado (STILL_ACTIVE).
const stillActive = 259

// processAlive dice si existe un proceso con ese PID en este equipo y no ha terminado.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Sin permiso para abrirlo: existe (de otra cuenta).
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return true
	}
	return code == stillActive
}
