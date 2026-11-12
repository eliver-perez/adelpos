package main

import (
	"errors"
	"os"
	"path/filepath"
)

// maxLogBytes es el tamaño a partir del cual la bitácora se rota al iniciar: el archivo anterior se
// conserva como «.1» (reemplaza al previo) y se empieza uno nuevo. Evita que un equipo que nunca se
// revisa acumule bitácoras sin límite.
const maxLogBytes = 20 * 1024 * 1024

// openLogFile abre la bitácora en modo de anexar y crea su carpeta si falta (etapa 9E).
func openLogFile(filename string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return nil, errors.New("no se pudo crear la carpeta de la bitácora")
	}
	if info, err := os.Stat(filename); err == nil && info.Size() > maxLogBytes {
		_ = os.Remove(filename + ".1")
		_ = os.Rename(filename, filename+".1")
	}
	output, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, errors.New("no se pudo abrir la bitácora")
	}
	return output, nil
}
