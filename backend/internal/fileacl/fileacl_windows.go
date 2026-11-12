//go:build windows

package fileacl

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// Protect fija la ACL privada en un archivo o una carpeta. En una carpeta las entradas se heredan a todo
// lo que se cree dentro (OI: archivos, CI: subcarpetas).
func Protect(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return errors.New("no existe el archivo o la carpeta que se debe proteger")
	}
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	inheritance := ""
	if info.IsDir() {
		inheritance = "OICI"
	}
	// D:P = DACL protegida (sin herencia); FA = control total; SY = SYSTEM; BA = Administradores.
	sddl := "D:P(A;" + inheritance + ";FA;;;" + user + ")(A;" + inheritance + ";FA;;;SY)(A;" + inheritance + ";FA;;;BA)"
	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return errors.New("no se pudo preparar la lista de control de acceso")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return errors.New("no se pudo preparar la lista de control de acceso")
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	if err != nil {
		return errors.New("no se pudo proteger el archivo privado")
	}
	return nil
}

// currentUserSID es el identificador de seguridad de la cuenta que ejecuta el proceso.
func currentUserSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", errors.New("no se pudo identificar la cuenta actual")
	}
	return user.User.Sid.String(), nil
}
