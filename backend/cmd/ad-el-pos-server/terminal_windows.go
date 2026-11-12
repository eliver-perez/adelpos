//go:build windows

package main

import "os"

// openInteractiveTerminal abre la consola de Windows (etapa 9E): CONIN$ para leer, también la contraseña
// sin eco, y CONOUT$ para escribir. Sin una consola interactiva falla, igual que sin /dev/tty en macOS.
func openInteractiveTerminal() (*os.File, *os.File, error) {
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	output, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		input.Close()
		return nil, nil, err
	}
	return input, output, nil
}

func closeInteractiveTerminal(input, output *os.File) {
	input.Close()
	output.Close()
}
