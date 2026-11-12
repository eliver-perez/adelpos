//go:build !windows

package main

import "os"

// openInteractiveTerminal abre la terminal de control (/dev/tty) para leer y escribir, como hasta la 9D.
func openInteractiveTerminal() (*os.File, *os.File, error) {
	terminal, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	return terminal, terminal, nil
}

func closeInteractiveTerminal(input, _ *os.File) {
	input.Close()
}
