//go:build !windows

package kitchen

import "context"

// En macOS (y en cualquier sistema que no sea Windows) la cola de impresión del equipo es CUPS, sin
// ningún cambio respecto de las etapas anteriores.

// hostSpooler no existe fuera de Windows: el transporte «cups» usa CUPS.
func hostSpooler() (spoolerAPI, bool) {
	return nil, false
}

// DiscoverPrinters lista las colas CUPS del equipo servidor (lpstat y lpoptions, solo lecturas).
func DiscoverPrinters(ctx context.Context) ([]DiscoveredPrinter, error) {
	return discoverCUPSPrinters(ctx)
}

// validateSystemQueue valida el destino de una impresora «cola de impresión del equipo»: el nombre
// de una cola CUPS y su opción de corte.
func validateSystemQueue(printer *Printer) error {
	return validateCUPSQueue(printer)
}
