//go:build windows

package legacyread

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// fileLock es un bloqueo exclusivo del sistema operativo sobre un archivo (LockFileEx): Windows lo suelta al cerrar el
// archivo o al terminar el proceso, también si se termina a la fuerza.
type fileLock struct {
	file *os.File
}

func lockFile(path string) (*fileLock, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	overlapped := new(windows.Overlapped)
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, overlapped); err != nil {
		_ = file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return nil, errAlreadyRunning
		}
		return nil, err
	}
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
