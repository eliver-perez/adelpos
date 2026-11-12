//go:build !windows

package main

import (
	"context"
	"errors"
)

// En macOS el servidor se detiene con SIGTERM (scripts/server-process.py); el evento de parada y el
// comando «stop» solo existen en Windows (stop_windows.go).
func withPlatformStop(parent context.Context) (context.Context, func()) {
	return parent, func() {}
}

func stopProcess(int) error {
	return errors.New("el comando stop solo existe en Windows; en macOS usa scripts/dev.sh server-stop")
}
