//go:build !windows

package fileacl

import (
	"os"
	"syscall"
)

// Protect no hace nada fuera de Windows: los permisos 0600/0700 con que se crean los archivos y las
// carpetas privados ya los protegen.
func Protect(string) error {
	return nil
}

// CreatePrivate crea un archivo nuevo solo para la cuenta que ejecuta el servidor (etapa 9G, respaldos): 0600 desde el
// primer byte (la máscara del proceso solo puede quitar permisos), sin seguir enlaces y sin reemplazar nada (falla si
// el nombre ya existe, también si es un enlace simbólico: O_EXCL y O_NOFOLLOW).
func CreatePrivate(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
}
