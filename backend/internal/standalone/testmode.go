package standalone

import (
	"errors"
)

// RefuseRealDataDir impide que el modo de pruebas use la carpeta de datos real: allí están el clúster, la
// configuración y los respaldos del restaurante (etapa 9F, D1; en macOS desde la etapa 9G). La carpeta
// real sale del sistema (RealDataDir), nunca de una variable que la prueba pueda cambiar, y la comparación
// no distingue la forma de escribir la ruta (mayúsculas, enlaces o forma corta de Windows).
func RefuseRealDataDir(layout Layout, productName string) error {
	real, err := RealDataDir(productName)
	if err != nil {
		return err
	}
	if samePath(layout.DataDir, real) {
		return errors.New("el modo de pruebas (ADEL_TEST_MODE=1) no puede usar la carpeta de datos real de AD-EL POS; " +
			testModeHint)
	}
	return nil
}
