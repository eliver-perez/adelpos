//go:build windows

package kitchen

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Llamadas reales al administrador de impresión de Windows (winspool.drv) para la cola de impresión
// del equipo (etapa 9E). Solo este archivo toca impresoras reales; la lógica de qué impresora se
// puede usar y cómo se interpreta cada resultado está en windows_queue.go y se prueba con un
// administrador simulado. Los nombres llegan validados (validateWindowsPrinterName) y se entregan
// como texto UTF-16, nunca a un shell.

var (
	winspool             = windows.NewLazySystemDLL("winspool.drv")
	procOpenPrinterW     = winspool.NewProc("OpenPrinterW")
	procClosePrinter     = winspool.NewProc("ClosePrinter")
	procGetPrinterW      = winspool.NewProc("GetPrinterW")
	procEnumPrintersW    = winspool.NewProc("EnumPrintersW")
	procEnumPortsW       = winspool.NewProc("EnumPortsW")
	procStartDocPrinterW = winspool.NewProc("StartDocPrinterW")
	procStartPagePrinter = winspool.NewProc("StartPagePrinter")
	procWritePrinter     = winspool.NewProc("WritePrinter")
	procEndPagePrinter   = winspool.NewProc("EndPagePrinter")
	procEndDocPrinter    = winspool.NewProc("EndDocPrinter")
	procAbortPrinter     = winspool.NewProc("AbortPrinter")
)

// Banderas de EnumPrintersW: impresoras instaladas en el equipo y conexiones a impresoras de otro.
const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004
)

// printerInfo2 es PRINTER_INFO_2W con la misma disposición en memoria que en C.
type printerInfo2 struct {
	ServerName         *uint16
	PrinterName        *uint16
	ShareName          *uint16
	PortName           *uint16
	DriverName         *uint16
	Comment            *uint16
	Location           *uint16
	DevMode            uintptr
	SepFile            *uint16
	PrintProcessor     *uint16
	Datatype           *uint16
	Parameters         *uint16
	SecurityDescriptor uintptr
	Attributes         uint32
	Priority           uint32
	DefaultPriority    uint32
	StartTime          uint32
	UntilTime          uint32
	Status             uint32
	Jobs               uint32
	AveragePPM         uint32
}

// portInfo2 es PORT_INFO_2W.
type portInfo2 struct {
	PortName    *uint16
	MonitorName *uint16
	Description *uint16
	PortType    uint32
	Reserved    uint32
}

// docInfo1 es DOC_INFO_1W: título del documento, sin archivo de salida y con el tipo de datos RAW
// (el administrador de impresión no convierte los bytes).
type docInfo1 struct {
	DocName    *uint16
	OutputFile *uint16
	Datatype   *uint16
}

// hostWinspool es el administrador de impresión del equipo donde corre Go.
type hostWinspool struct{}

// callError convierte el error de una llamada fallida en uno legible aunque Windows no informe su
// código.
func callError(err error) error {
	var code windows.Errno
	if errors.As(err, &code) && code == 0 {
		return errors.New("el administrador de impresión de Windows no informó el motivo")
	}
	return err
}

// wideText lee una cadena UTF-16 terminada en cero que devolvió Windows.
func wideText(pointer *uint16) string {
	if pointer == nil {
		return ""
	}
	return windows.UTF16PtrToString(pointer)
}

// enumerationBuffer prepara un búfer alineado para las estructuras que escribe Windows.
func enumerationBuffer(size uint32) []uint64 {
	return make([]uint64, (uintptr(size)+7)/8+1)
}

// enumerate llama dos veces a una función EnumXxxW: la primera para conocer el tamaño necesario y
// la segunda con el búfer. Si la lista creció entre ambas, se repite unas pocas veces.
func enumerate(call func(buffer unsafe.Pointer, size uint32, needed, returned *uint32) error) ([]uint64, uint32, error) {
	var needed, returned uint32
	err := call(nil, 0, &needed, &returned)
	if err == nil {
		return nil, 0, nil
	}
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		return nil, 0, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		buffer := enumerationBuffer(needed)
		size := uint32(len(buffer) * 8)
		err = call(unsafe.Pointer(&buffer[0]), size, &needed, &returned)
		if err == nil {
			return buffer, returned, nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			return nil, 0, err
		}
	}
	return nil, 0, err
}

// convertPrinter copia los datos de PRINTER_INFO_2W a la estructura de Go.
func convertPrinter(info *printerInfo2) spoolerPrinter {
	return spoolerPrinter{
		Server:     wideText(info.ServerName),
		Name:       wideText(info.PrinterName),
		Port:       wideText(info.PortName),
		Driver:     wideText(info.DriverName),
		Attributes: info.Attributes,
		Status:     info.Status,
		Jobs:       info.Jobs,
	}
}

func (hostWinspool) listPrinters() ([]spoolerPrinter, error) {
	buffer, count, err := enumerate(func(buffer unsafe.Pointer, size uint32, needed, returned *uint32) error {
		ok, _, callErr := procEnumPrintersW.Call(
			printerEnumLocal|printerEnumConnections,
			0,
			2,
			uintptr(buffer),
			uintptr(size),
			uintptr(unsafe.Pointer(needed)),
			uintptr(unsafe.Pointer(returned)),
		)
		if ok == 0 {
			return callError(callErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	printers := make([]spoolerPrinter, 0, count)
	if count > 0 {
		entries := unsafe.Slice((*printerInfo2)(unsafe.Pointer(&buffer[0])), count)
		for index := range entries {
			printers = append(printers, convertPrinter(&entries[index]))
		}
	}
	runtime.KeepAlive(buffer)
	return printers, nil
}

func (hostWinspool) printer(name string) (spoolerPrinter, error) {
	handle, err := openPrinter(name)
	if errors.Is(err, windows.ERROR_INVALID_PRINTER_NAME) {
		// Revisión 9F (B4): Windows no tiene una impresora con ese nombre (no instalada o renombrada).
		return spoolerPrinter{}, errPrinterNotInstalled
	}
	if err != nil {
		return spoolerPrinter{}, err
	}
	defer closePrinter(handle)
	// La primera llamada, sin búfer, solo informa el tamaño necesario.
	var needed uint32
	ok, _, callErr := procGetPrinterW.Call(uintptr(handle), 2, 0, 0, uintptr(unsafe.Pointer(&needed)))
	if ok != 0 {
		return spoolerPrinter{}, errors.New("Windows no informó los datos de la impresora")
	}
	if err := callError(callErr); !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		return spoolerPrinter{}, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		buffer := enumerationBuffer(needed)
		size := uint32(len(buffer) * 8)
		ok, _, callErr = procGetPrinterW.Call(
			uintptr(handle),
			2,
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(size),
			uintptr(unsafe.Pointer(&needed)),
		)
		if ok != 0 {
			info := convertPrinter((*printerInfo2)(unsafe.Pointer(&buffer[0])))
			runtime.KeepAlive(buffer)
			return info, nil
		}
		if !errors.Is(callError(callErr), windows.ERROR_INSUFFICIENT_BUFFER) {
			return spoolerPrinter{}, callError(callErr)
		}
	}
	return spoolerPrinter{}, errors.New("Windows no informó los datos de la impresora")
}

func (hostWinspool) ports() ([]spoolerPort, error) {
	buffer, count, err := enumerate(func(buffer unsafe.Pointer, size uint32, needed, returned *uint32) error {
		ok, _, callErr := procEnumPortsW.Call(
			0,
			2,
			uintptr(buffer),
			uintptr(size),
			uintptr(unsafe.Pointer(needed)),
			uintptr(unsafe.Pointer(returned)),
		)
		if ok == 0 {
			return callError(callErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	ports := make([]spoolerPort, 0, count)
	if count > 0 {
		entries := unsafe.Slice((*portInfo2)(unsafe.Pointer(&buffer[0])), count)
		for index := range entries {
			ports = append(ports, spoolerPort{
				Name:    wideText(entries[index].PortName),
				Monitor: wideText(entries[index].MonitorName),
			})
		}
	}
	runtime.KeepAlive(buffer)
	return ports, nil
}

func (hostWinspool) open(name string) (spoolerHandle, error) {
	handle, err := openPrinter(name)
	if err != nil {
		return nil, err
	}
	return &winspoolDocument{handle: handle}, nil
}

// openPrinter abre una impresora con el acceso predeterminado (usar la impresora). Un nombre vacío
// abriría el servidor de impresión: los nombres siempre llegan validados.
func openPrinter(name string) (windows.Handle, error) {
	if name == "" {
		return 0, errors.New("falta el nombre de la impresora")
	}
	pointer, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	var handle windows.Handle
	ok, _, callErr := procOpenPrinterW.Call(uintptr(unsafe.Pointer(pointer)), uintptr(unsafe.Pointer(&handle)), 0)
	if ok == 0 {
		return 0, callError(callErr)
	}
	return handle, nil
}

func closePrinter(handle windows.Handle) error {
	ok, _, callErr := procClosePrinter.Call(uintptr(handle))
	if ok == 0 {
		return callError(callErr)
	}
	return nil
}

// winspoolDocument es una impresora abierta para enviarle un documento RAW.
type winspoolDocument struct {
	handle windows.Handle
	closed bool
}

func (document *winspoolDocument) startDocument(title string) (uint32, error) {
	name, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return 0, err
	}
	datatype, err := windows.UTF16PtrFromString("RAW")
	if err != nil {
		return 0, err
	}
	info := docInfo1{DocName: name, Datatype: datatype}
	jobID, _, callErr := procStartDocPrinterW.Call(uintptr(document.handle), 1, uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(name)
	runtime.KeepAlive(datatype)
	if jobID == 0 {
		return 0, callError(callErr)
	}
	return uint32(jobID), nil
}

func (document *winspoolDocument) startPage() error {
	return document.simple(procStartPagePrinter)
}

func (document *winspoolDocument) write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	var written uint32
	ok, _, callErr := procWritePrinter.Call(
		uintptr(document.handle),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(uint32(len(data))),
		uintptr(unsafe.Pointer(&written)),
	)
	runtime.KeepAlive(data)
	if ok == 0 {
		return int(written), callError(callErr)
	}
	return int(written), nil
}

func (document *winspoolDocument) endPage() error {
	return document.simple(procEndPagePrinter)
}

func (document *winspoolDocument) endDocument() error {
	return document.simple(procEndDocPrinter)
}

func (document *winspoolDocument) abort() error {
	return document.simple(procAbortPrinter)
}

func (document *winspoolDocument) close() error {
	if document.closed {
		return nil
	}
	document.closed = true
	return closePrinter(document.handle)
}

// simple llama a una función de winspool.drv que solo recibe la impresora abierta.
func (document *winspoolDocument) simple(procedure *windows.LazyProc) error {
	ok, _, callErr := procedure.Call(uintptr(document.handle))
	if ok == 0 {
		return callError(callErr)
	}
	return nil
}
