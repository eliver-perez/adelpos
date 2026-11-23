//go:build adelpos_production

package licensing

import _ "embed"

// Compilación de producción (con la etiqueta adelpos_production): confía solo en las llaves del entorno
// «production». Ese servidor de licencias y sus llaves todavía no existen, así que llaves-production.json
// no existe y **esta compilación falla a propósito** («pattern llaves-production.json: no matching files
// found»): no puede salir un instalador de producción por accidente (§1.2 de la especificación). Una prueba
// lo comprueba (production_build_test.go). Lo que falta está en docs/licencias/PASO_A_PRODUCCION.md.

// BuildEnvironment es el `environment` de server.json con que arranca esta compilación (CheckStartup).
const BuildEnvironment = "production"

// compiledEnvironment es el entorno de firma de las licencias en que confía esta compilación.
const compiledEnvironment = "production"

// DefaultServerURL es la dirección del servidor de licencias de producción. Todavía no existe: queda vacía
// y el cliente no se conecta a ninguna parte hasta que el paso a producción la fije. Nunca es la del entorno
// de pruebas.
const DefaultServerURL = ""

// testHooksAllowed: una versión de producción nunca confía en las llaves de los archivos de prueba ni usa el
// reloj con desfase de las pruebas, aunque se configure test_mode.
const testHooksAllowed = false

// compiledKeys es la copia sincronizada de config/licencias/llaves-production.json.
//
//go:embed llaves-production.json
var compiledKeys []byte
