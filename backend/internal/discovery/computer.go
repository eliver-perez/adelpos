package discovery

import (
	"context"
	"os"
	"strings"
)

// ComputerName es el nombre de la computadora que se muestra en el anuncio («Mac de caja»). En macOS es el
// nombre que el propietario ve en Ajustes › General › Compartir (systemComputerName, computer_darwin.go); en los
// demás sistemas, el nombre de red del equipo. Sin nombre devuelve "" (InstanceName usa el del establecimiento).
func ComputerName(ctx context.Context) string {
	if name := strings.TrimSpace(systemComputerName(ctx)); name != "" {
		return name
	}
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	host = strings.TrimSuffix(strings.TrimSuffix(host, "."), ".local")
	return strings.ReplaceAll(host, "-", " ")
}
