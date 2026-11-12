//go:build windows

package main

// Parada ordenada en Windows (etapa 9E). Windows no envía SIGTERM y un proceso sin consola no recibe
// Ctrl+C, así que cada servidor crea un evento con nombre ligado a su PID; el comando «stop --pid N»
// (scripts de desarrollo y pruebas) lo activa y el servidor se apaga igual que con SIGTERM en macOS:
// cierre de HTTP, de los trabajadores y del pool. Antes de activar el evento, «stop» comprueba que el
// proceso indicado sea este mismo programa: nunca se detiene otro proceso.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// stopWaitTimeout es lo que «stop» espera a que el servidor termine (el cierre HTTP dura hasta 5 s).
const stopWaitTimeout = 20 * time.Second

func stopEventName(pid int) string {
	return fmt.Sprintf(`Local\ad-el-pos-server-stop-%d`, pid)
}

// withPlatformStop devuelve un contexto que se cancela cuando otro proceso activa el evento de parada.
func withPlatformStop(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	name, err := windows.UTF16PtrFromString(stopEventName(os.Getpid()))
	if err != nil {
		return ctx, cancel
	}
	// Evento manual, sin señalar, con la seguridad predeterminada (solo la sesión del usuario).
	event, err := windows.CreateEvent(nil, 1, 0, name)
	if err != nil {
		return ctx, cancel
	}
	go func() {
		// La espera termina cuando «stop» activa el evento; al salir el proceso, Windows cierra el handle.
		if status, err := windows.WaitForSingleObject(event, windows.INFINITE); err == nil && status == windows.WAIT_OBJECT_0 {
			cancel()
		}
	}()
	return ctx, func() {
		cancel()
	}
}

// stopProcess pide a un servidor de este mismo programa que se detenga y espera a que termine.
func stopProcess(pid int) error {
	if pid <= 0 {
		return errors.New("indica el PID del servidor con --pid")
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return errors.New("no existe un proceso activo con ese PID")
	}
	defer windows.CloseHandle(process)
	if !sameExecutable(process) {
		return errors.New("el PID pertenece a otro programa; no se envió ninguna orden")
	}
	name, err := windows.UTF16PtrFromString(stopEventName(pid))
	if err != nil {
		return err
	}
	event, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return errors.New("el servidor no ofrece parada ordenada; no se forzó su terminación")
	}
	defer windows.CloseHandle(event)
	if err := windows.SetEvent(event); err != nil {
		return errors.New("no se pudo enviar la orden de parada")
	}
	status, err := windows.WaitForSingleObject(process, uint32(stopWaitTimeout/time.Millisecond))
	if err != nil || status != windows.WAIT_OBJECT_0 {
		return errors.New("no se confirmó el cierre; no se forzará la terminación, revisa el proceso")
	}
	return nil
}

// sameExecutable compara la ruta del ejecutable del proceso con la de este programa.
func sameExecutable(process windows.Handle) bool {
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return false
	}
	other := windows.UTF16ToString(buffer[:size])
	own, err := os.Executable()
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(own); err == nil {
		own = resolved
	}
	if resolved, err := filepath.EvalSymlinks(other); err == nil {
		other = resolved
	}
	return strings.EqualFold(filepath.Clean(own), filepath.Clean(other))
}
