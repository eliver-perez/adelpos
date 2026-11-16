//go:build !windows

package legacyread

import (
	"errors"
	"os"
	"strconv"
	"syscall"
)

// fileLock es un bloqueo exclusivo del sistema operativo sobre un archivo (flock): se suelta al cerrar el archivo o
// al terminar el proceso, también con SIGKILL, así que nunca queda un bloqueo «huérfano».
type fileLock struct {
	file *os.File
}

func lockFile(path string) (*fileLock, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, errAlreadyRunning
		}
		return nil, err
	}
	// El PID de quien lo tiene, solo informativo (el bloqueo es el del sistema).
	_ = file.Truncate(0)
	_, _ = file.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return &fileLock{file: file}, nil
}

func (lock *fileLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	err := lock.file.Close()
	lock.file = nil
	return err
}
