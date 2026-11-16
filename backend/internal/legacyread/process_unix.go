//go:build !windows

package legacyread

import (
	"errors"
	"syscall"
)

// processAlive dice si existe un proceso con ese PID en este equipo (kill con la señal 0 no envía nada).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
