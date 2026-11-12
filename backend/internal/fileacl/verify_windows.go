//go:build windows

package fileacl

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RequirePrivate comprueba que la ACL de un archivo o una carpeta solo conceda acceso a la cuenta actual,
// a SYSTEM y a Administradores. Lo usan las pruebas en lugar de los bits 0600 de macOS.
func RequirePrivate(path string) error {
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return errors.New("no se pudo leer la lista de control de acceso")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return errors.New("el archivo no tiene lista de control de acceso propia")
	}
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	allowed := map[string]bool{user: true, "S-1-5-18": true, "S-1-5-32-544": true}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return errors.New("no se pudo leer una entrada de la lista de control de acceso")
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !allowed[sid.String()] {
			return errors.New("la lista de control de acceso concede acceso a " + sid.String())
		}
	}
	return nil
}
