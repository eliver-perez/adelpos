//go:build !darwin

package discovery

import "context"

// systemComputerName fuera de macOS: se usa el nombre de red del equipo (ComputerName).
func systemComputerName(context.Context) string {
	return ""
}
