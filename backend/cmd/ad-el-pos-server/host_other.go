//go:build !windows

package main

import (
	"context"
	"errors"
	"log/slog"
)

// En macOS no hay comandos de instalación: PostgreSQL y Go se administran con scripts/dev.sh.
var hostCommands = map[string]bool{}

func runHostCommand(context.Context, *slog.Logger, string, hostOptions, bool) error {
	return errors.New("los comandos de la instalación solo existen en Windows")
}
