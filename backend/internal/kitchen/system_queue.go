package kitchen

// Cola de impresión del equipo (transporte guardado como «cups»). En macOS y Linux es CUPS (lpstat,
// lpoptions y lp con argumentos separados); en Windows, desde la etapa 9E, la cola de la impresora
// instalada en Windows con su administrador de impresión, en modo RAW (windows_queue.go). El valor
// guardado no cambia (sin migración): solo cambia qué hace el equipo servidor con él. Cada sistema
// declara su cola en system_queue_unix.go o system_queue_windows.go: hostSpooler, DiscoverPrinters
// y validateSystemQueue.

// Colas de impresión del equipo que conoce AD-EL POS.
const (
	SystemQueueCUPS    = "cups"
	SystemQueueWindows = "windows"
)

// windowsSpooler decide con qué cola entrega el transporte el tipo «cups»: con la de Windows (y su
// administrador de impresión) o con CUPS. Las pruebas que sustituyen los programas de CUPS (commands
// o textCUPS) ejercitan siempre CUPS, en cualquier sistema; las que sustituyen el administrador de
// Windows (spooler), siempre Windows. Sin sustitutos, como en producción, decide el sistema donde
// corre Go.
func (transport HostTransport) windowsSpooler() (spoolerAPI, bool) {
	if transport.spooler != nil {
		return transport.spooler, true
	}
	if transport.commands != nil || transport.textCUPS != nil {
		return nil, false
	}
	return hostSpooler()
}

// windowsSpooler aplica al inspector la misma regla que al transporte.
func (inspector HostQueueInspector) windowsSpooler() (spoolerAPI, bool) {
	if inspector.spooler != nil {
		return inspector.spooler, true
	}
	if inspector.commands != nil {
		return nil, false
	}
	return hostSpooler()
}

// SystemQueue indica qué cola del equipo consulta el inspector: SystemQueueCUPS o
// SystemQueueWindows.
func (inspector HostQueueInspector) SystemQueue() string {
	if _, windows := inspector.windowsSpooler(); windows {
		return SystemQueueWindows
	}
	return SystemQueueCUPS
}

// SystemQueueOf indica qué cola del equipo consulta un inspector. Los inspectores que no lo
// declaran (los simulados de las pruebas de otros paquetes) se tratan como CUPS.
func SystemQueueOf(inspector QueueInspector) string {
	if described, ok := inspector.(interface{ SystemQueue() string }); ok {
		return described.SystemQueue()
	}
	return SystemQueueCUPS
}
