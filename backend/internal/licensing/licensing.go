// Package licensing es el núcleo de las licencias del servidor Go principal (etapa L1, contrato de licencias
// V1.0 en docs/licencias/):
//
//   - verify.go y protocol.go: la verificación de la §10 y los mensajes firmados de la §8.2 y la §9.1,
//     adaptados del verificador de referencia contracts/licencias/v1/verify;
//   - state.go y status.go: el estado temporal de la §5.6 y los seis estados de la §6;
//   - trust*.go: las llaves de confianza del entorno fijado al compilar (§1.2 de la especificación);
//   - store.go: identity/licencia (estado.json y vigente.lic), con escritura atómica;
//   - machine*.go: la huella del equipo (§1.1 de la especificación);
//   - datapresence.go: qué cuenta como operación en la base («Sin activar con datos»);
//   - manager.go: el gestor que usan el arranque, las rutas de la licencia, la puerta y los cupos.
//
// Solo el servidor Go principal habla con el servidor de licencias; las terminales solo muestran el estado.
package licensing

import (
	"errors"
	"fmt"
)

// CheckStartup comprueba, antes de abrir la base, que la configuración corresponde al entorno compilado
// (L1-D18): una compilación de desarrollo solo arranca con environment y license_mode «development», y una
// de producción solo con «production» y «commercial». También comprueba las llaves embebidas.
func CheckStartup(environment, mode string) error {
	if _, err := CompiledTrust(); err != nil {
		return fmt.Errorf("las llaves de confianza de las licencias de esta versión no son válidas: %w", err)
	}
	switch BuildEnvironment {
	case "development":
		if environment == "development" && mode == "development" {
			return nil
		}
		return errors.New("esta versión de AD-EL POS es de desarrollo: solo arranca con environment y license_mode development")
	case "production":
		if environment == "production" && mode == "commercial" {
			return nil
		}
		return errors.New("esta versión de AD-EL POS es de producción: solo arranca con environment production y license_mode commercial")
	default:
		return errors.New("entorno de compilación desconocido")
	}
}
