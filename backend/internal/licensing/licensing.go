// Package licensing es el único punto de integración del proveedor comercial futuro.
package licensing

import "errors"

func CheckStartup(environment, mode string) error {
	if environment == "development" && mode == "development" {
		return nil
	}
	return errors.New("la distribución comercial requiere integrar el proveedor de licencias; no está implementado en etapa 1")
}
