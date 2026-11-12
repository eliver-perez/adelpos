//go:build !windows

package fileacl

// Protect no hace nada fuera de Windows: los permisos 0600/0700 con que se crean los archivos y las
// carpetas privados ya los protegen.
func Protect(string) error {
	return nil
}
