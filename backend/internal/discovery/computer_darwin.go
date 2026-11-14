//go:build darwin

package discovery

import (
	"context"
	"os/exec"
	"time"
)

// systemComputerName lee el nombre de la computadora de la configuración del sistema (con acentos y espacios,
// a diferencia del nombre de red «Mac-de-caja.local»).
func systemComputerName(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--get", "ComputerName").Output()
	if err != nil {
		return ""
	}
	return string(output)
}
