// Package brand embebe la marca de AD-EL POS para los documentos que genera Go (etapa 9B).
//
// adel-pos-tema-claro.png es una copia byte a byte de assets/brand/adel-pos-tema-claro.png (el
// original no se modifica; go:embed no puede leer fuera del directorio del paquete). La prueba del
// paquete compara su SHA-256 con el del original del repositorio: si alguien cambia la marca, la
// copia debe volver a copiarse tal cual. Los reportes PDF y Excel la usan como respaldo cuando el
// restaurante no ha cargado su propio logotipo; nunca sustituye al logotipo del restaurante ni se
// imprime en los tickets térmicos.
package brand

import (
	_ "embed"
)

// LightLogoFile es el nombre del original en assets/brand (versión para fondos claros).
const LightLogoFile = "adel-pos-tema-claro.png"

// LightLogoSHA256 es la huella del original, la misma que comprueba la prueba del paquete.
const LightLogoSHA256 = "fece00410137a618703c5921d2ce2aac5ef06227836b75505201e58125e05434"

//go:embed adel-pos-tema-claro.png
var lightLogo []byte

// LightLogoPNG devuelve una copia de los bytes del logotipo de AD-EL POS para fondos claros
// (PNG RGBA de 2048 × 682 píxeles con fondo transparente). Es una copia para que nadie pueda alterar
// la versión embebida.
func LightLogoPNG() []byte {
	copied := make([]byte, len(lightLogo))
	copy(copied, lightLogo)
	return copied
}
