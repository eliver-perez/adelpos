//go:build windows

package kitchen

import "context"

// En Windows la cola de impresión del equipo es la de la impresora instalada en Windows, con su
// administrador de impresión en modo RAW (etapa 9E, windows_queue.go). El código de CUPS sigue
// compilado y probado con sus programas simulados, pero no se usa.

// hostSpooler es el administrador de impresión de Windows del equipo donde corre Go.
func hostSpooler() (spoolerAPI, bool) {
	return hostWinspool{}, true
}

// DiscoverPrinters lista las impresoras instaladas en Windows en el equipo servidor (solo
// lecturas).
func DiscoverPrinters(ctx context.Context) ([]DiscoveredPrinter, error) {
	return discoverWindowsPrinters(ctx, hostWinspool{})
}

// validateSystemQueue valida el destino de una impresora «cola de impresión del equipo»: el nombre
// de una impresora instalada en Windows.
func validateSystemQueue(printer *Printer) error {
	return validateWindowsQueue(printer)
}
