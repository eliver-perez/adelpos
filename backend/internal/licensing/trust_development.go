//go:build !adelpos_production

package licensing

import _ "embed"

// Compilación de desarrollo (sin la etiqueta adelpos_production): confía solo en las llaves del entorno
// «test» del servidor de licencias, exportadas por el propietario (§1.2 de la especificación). Es la que
// generan hoy el .pkg de macOS y el instalador de Windows.

// BuildEnvironment es el `environment` de server.json con que arranca esta compilación (CheckStartup).
const BuildEnvironment = "development"

// compiledEnvironment es el entorno de firma de las licencias en que confía esta compilación.
const compiledEnvironment = "test"

// DefaultServerURL es la dirección base del servidor de licencias del entorno «test». La configuración
// puede cambiarla (Fase B).
const DefaultServerURL = "https://licensing.sicrat.com"

// testHooksAllowed permite, solo con test_mode, las ayudas de las pruebas: confiar además en las llaves de
// vectors.json (ADEL_TEST_LICENSE_TRUST) y el reloj con desfase (ADEL_TEST_CLOCK_FILE). Nunca en producción.
const testHooksAllowed = true

// compiledKeys es la copia sincronizada de config/licencias/llaves-test.json (scripts/sync-metadata.mjs).
//
//go:embed llaves-test.json
var compiledKeys []byte
