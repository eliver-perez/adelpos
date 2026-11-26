# Paso a producción de las licencias

Qué falta para que AD-EL POS salga en su versión del restaurante, con licencias reales. La etapa L1 solo trabajó con el
entorno `test` del servidor de licencias: **la versión 0.15.0 es de desarrollo**, y también la 0.16.0 de la etapa K1 y la 0.17.0 de la K1.1 (§1.2 de la
[especificación](../etapas/ETAPA_L1_LICENCIAS_v2.md) y L1-D18 del [avance](../ETAPA_L1_AVANCE.md)). Cómo está hecha la
integración: [INTEGRACION.md](INTEGRACION.md).

## Dónde estamos

| Pieza                                    | Desarrollo (hoy)                                                                                  | Producción                                                                                   |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Compilación del servidor Go              | Sin etiqueta: `trust_development.go` y `support_development.go`                                   | `-tags adelpos_production`: `trust_production.go` y `support_production.go`                  |
| Llaves de firma de licencias             | `config/licencias/llaves-test.json` (entorno `test`, `adelpos-test-202611-1`)                     | `config/licencias/llaves-production.json`: **no existe**                                     |
| Servidor de licencias                    | `https://licensing.sicrat.com` (`DefaultServerURL` de desarrollo)                                 | **No existe.** `DefaultServerURL` de producción está vacía y el cliente no se conecta a nada |
| Llave de soporte (desbloqueo de la hora) | Pública en `config/licencias/soporte-development.pub.json`; privada en `~/.adel-pos-dev/soporte/` | `config/licencias/soporte-production.pub.json`: **no existe**                                |
| `server.json`                            | `environment` y `license_mode` `development`                                                      | `environment` `production` y `license_mode` `commercial`                                     |
| Ayudas de prueba (`ADEL_TEST_*`)         | Solo con `test_mode`                                                                              | Nunca: `testHooksAllowed = false`, aunque se configure `test_mode`                           |

**Sin las dos llaves de producción, la compilación de producción no compila** («pattern llaves-production.json: no
matching files found»). Así no puede salir un instalador de producción por accidente. Lo comprueba
`TestProductionBuildFailsWithoutProductionKeys` (`backend/internal/licensing/production_build_test.go`), con las dos.

**Una versión de producción nunca** confía en las llaves `test` ni en las de los archivos de prueba, ni apunta al
servidor de licencias de pruebas (§0 «Secretos» de la especificación).

## Lo que falta, en orden

### 1. El servidor de licencias de producción (lo hace el propietario)

- Instalar el servidor de licencias en su entorno `production`, aparte del de pruebas, con su propia dirección https.
- Crear ahí sus planes, sus clientes y sus licencias. Las de `test` (la Perpetua, la Mensual y la Prueba) **no sirven**
  en producción: están firmadas con la llave `test`.
- Generar la llave de firma de producción **sin activarla todavía** (§3 de la [guía](INTEGRACION_ADEL_POS.md)).

### 2. Sus llaves públicas en AD-EL POS

Con el procedimiento de [ROTAR_LLAVES.md](ROTAR_LLAVES.md), en una rama nueva:

1. Exportar las públicas del entorno `production` (`bin/signing-key.php --action=export` en el servidor de licencias).
2. Guardar la exportación completa como `config/licencias/llaves-production.json`. Todas las llaves deben ser del entorno
   `production`: el servidor rechaza al arrancar una llave de otro entorno, un `kid` repetido o una llave inválida.
3. `./scripts/dev.sh sync`: copia el archivo a `backend/internal/licensing/` y `sync-check` lo vigila.
4. Agregar una prueba que fije la lista exacta de producción, como `TestCompiledTrustIsTheOwnersTestKeyList` hace con
   la de `test` (`backend/internal/licensing/trust_test.go`), con la etiqueta `adelpos_production`.
5. Fijar `DefaultServerURL` de `trust_production.go` con la dirección del servidor de producción. Nunca la de pruebas.

### 3. La llave de soporte de producción (lo hace el propietario, en su computadora)

Ver [DESBLOQUEO_SOPORTE.md](DESBLOQUEO_SOPORTE.md), «Llaves de soporte»:

1. `tools/soporte-desbloqueo/soporte-desbloqueo generar-llave --entorno production --publica
config/licencias/soporte-production.pub.json`. La privada queda en `~/.adel-pos-dev/soporte/` (0600, carpeta 0700).
2. `./scripts/dev.sh sync`.
3. Un respaldo de la llave privada **fuera de línea**. Nunca en el repositorio, en un respaldo del restaurante, en el
   instalador ni en la salida de una orden. Quien la tenga puede desbloquear la hora de cualquier instalación de
   producción.

La propuesta para la V1.1 es pasar este desbloqueo al panel del servidor de licencias, con la llave guardada allá.

### 4. La compilación de producción

Hoy ningún guion compila con la etiqueta. Falta:

- **Los instaladores**: `scripts/build-macos-pkg.sh` y `scripts/build-windows.ps1` compilan el servidor con `go build
-trimpath ./cmd/ad-el-pos-server`. Necesitan una opción para producción que agregue `-tags adelpos_production` y que
  falle si las llaves no están. `TestTestToolsNeverShip` (`backend/cmd/license-test/guard_test.go`) debe seguir
  comprobando que ninguno compila `license-test`.
- **La configuración de la instalación**: `backend/internal/standalone/bootstrap.go` (`serverConfig`) escribe
  `environment` y `license_mode` `development`. En producción debe escribir `production` y `commercial`, porque
  `CheckStartup` no deja arrancar una compilación de producción con otra configuración (y su comentario, «hasta la etapa
  10 no hay licencias», ya no es cierto desde la L1). También falta decidir cómo se actualiza el `server.json` de una
  instalación de desarrollo que pase a producción.
- **La plantilla** `config/server.production.example.json` ya dice `production` y `commercial`; `license_server_url` es
  opcional (por omisión, la compilada).
- **Comprobar**: `go build -tags adelpos_production ./...` y `go vet` con la etiqueta, `go test -tags adelpos_production
./internal/licensing/...`, y que el servidor compilado responde `environment: production` en `license/status`.
- **Lo demás de la distribución** no es de las licencias, pero va con la versión del restaurante (`CLAUDE.md` §17): el
  identificador provisional `local.adel.pos.dev`, la firma de desarrollador y la notarización.

### 5. Activar la llave en el servidor de licencias

Solo cuando las computadoras de producción tengan instalada la versión que confía en ella (orden de
[ROTAR_LLAVES.md](ROTAR_LLAVES.md)).

## Qué pasa con una instalación de desarrollo

Una computadora con una versión de desarrollo, la 0.15.0, la 0.16.0 o la 0.17.0 (como la Mac de pruebas del propietario), que reciba una versión de
producción:

- **La licencia `test` deja de valer**: su `kid` no es de confianza en producción, así que se ignora y la instalación
  queda «Sin activar con datos». Se activa con una licencia de producción. Nada de los datos cambia.
- La llave de licencias de la instalación (`identity/license-key.pem`), la huella y la última hora confiable siguen
  igual.
- Las respuestas de soporte firmadas con la llave de desarrollo ya no se aceptan.

## Lo que no cambia

- El contrato, `contracts/licencias/v1/` y los archivos de prueba: las pruebas automáticas siguen usando solo las llaves
  de `vectors.json` y una compilación de desarrollo.
- Android: nunca habla con el servidor de licencias; muestra lo que dice Go.
- Los cupos, la puerta, la hora y los códigos de error: son los mismos en los dos entornos.
