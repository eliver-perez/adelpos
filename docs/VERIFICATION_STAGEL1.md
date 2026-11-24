# Verificación · Etapa L1 (integración con el servidor de licencias)

Verificación por fases del **23 y el 24 de noviembre de 2026**, en la Mac (Apple Silicon, macOS 26.5.2). Cada fase se
hizo en una rama local que el coordinador fusionó en `etapa-l1`. Esa rama se creó desde `main` (`1749b0b`: 0.14.0,
esquema 27, Android 0.3.0, con la A3 fusionada). La especificación está en `e2687fa`. Fusiones y avances de cada fase:

- **Fase 0**: fusión en `e081a7b` (la copia sin cambios del contrato de licencias V1.0, su guía y `contracts/v1/`, de la
  etiqueta `v1.0.0`); avance en `1ceee93`.
- **Fase A** (núcleo: llave, verificador y estado): fusión en `27cb24c`; avance en `406e730`.
- **Fase D** (cupos): fusión en `f4b42a3`; avance en `dcb6cd3`.
- **Fase B con las correcciones de la revisión R1** (cliente en línea y sin internet): fusión en `5cd23dd`; avance en
  `a8dd193`.
- **Fase C** (la puerta, la hora y el desbloqueo de soporte): fusión en `91f17ba`; avance en `f32d740`.
- **Fase G** (pruebas contra `licensing.sicrat.com`, sin commits de código): avance en `c0217c0`.
- **Fase F** (Android 0.4.0): fusión en `774c6f0`; avance en `e9773df`.
- **Correcciones de la revisión R3 y del hallazgo R2-2**: fusión en `df63422`; avance en `176ebb6`.
- **Fase E** (escritorio): fusión en `20d2725`; `a5ef6e6` (Windows) y avance en `c105bed`.
- **Correcciones de la revisión R2**: fusión en `f7fcbff`; avance en `cdc2ce2`.
- **Fase G2** (el plan nuevo de la Perpetua, sin commits de código): avance en `49e84aa`.
- **Fase H**, parte del servidor, el escritorio y los documentos (rama `etapa-l1-h`): `9384e63` (L1-D56), `623d089`
  (L1-D58), los documentos (`129ab49`, `0ffa6b8`, `8493414`, `16090b8` y este documento) y el comentario de la `0028`
  (`842e088`); fusión en `02f4642`, el `.pkg` rehecho en `ff71144` y avance en `23538ba`.
- **Fase H**, parte de Android (rama `etapa-l1-ha`): `5968922` (la prueba `LicenseOnGoTest` en el teléfono); fusión en
  `06f2b19`.

**Servidor 0.15.0.** API v1, migración `0028_licencias.sql` (esquema **28**). Es una sola para toda la etapa, con tres
secciones:

- **«Fase A (revisión R1)»**: `app.license_revisions` y `app.license_trusted_time`, segundas copias de la revisión más
  alta aceptada de cada licencia y de la última hora confiable (vale el máximo de cada copia y la de `identity/`).
- **«Fase B»**: el permiso `license.manage` («Administrar la licencia»), solo para el rol `administrator`.
- **«Fase D»**: `app.license_seats`, una fila por terminal y tipo de cupo, y el cupo de caja de la terminal que abrió
  cada turno ya abierto.

La licencia, la llave Ed25519 de licencias, la huella del equipo y la última hora confiable viven en `identity/`, que no
viaja en ningún respaldo (L1-D9). `min_client_versions` no cambia (L1-D22); la capacidad nueva es `license-v1`. **Es una
versión de desarrollo**: confía solo en las llaves del entorno `test` y la compilación de producción no compila sin las
llaves de producción ([licencias/PASO_A_PRODUCCION.md](licencias/PASO_A_PRODUCCION.md)).

**Aplicación Android 0.4.0** (`com.adelpos.meseros`, `versionCode` 4, `minSdk` 26): los mensajes de la licencia, del
cupo y de la hora, el aviso fijo de la hora, el aviso de tolerancia y la cola sin conexión con «Requiere tu revisión».
No habla con el servidor de licencias.

**Referencias.**

- Especificación: [etapas/ETAPA_L1_LICENCIAS_v2.md](etapas/ETAPA_L1_LICENCIAS_v2.md). **Manda el contrato**:
  [licencias/CONTRATO_LICENCIAS_ADEL_POS.md](licencias/CONTRATO_LICENCIAS_ADEL_POS.md).
- Avance, con el detalle de cada fase y las decisiones L1-D1 a L1-D59: [ETAPA_L1_AVANCE.md](ETAPA_L1_AVANCE.md).
- Cómo está hecha la integración: [licencias/INTEGRACION.md](licencias/INTEGRACION.md).
- Guía del propietario para Android: [ANDROID_INSTALAR.md](ANDROID_INSTALAR.md).
- Evidencia anterior: [etapa A3](VERIFICATION_STAGEA3.md).

Este documento registra **solo lo que se ejecutó**.

**Alcance de esta evidencia.** Las cifras de las fases son las que cada agente registró sobre su propia copia, según el
avance. Lo que valida el código completo de la etapa es la «Regresión final», sobre una copia exacta (`git archive`) de
la cabeza de la parte del servidor y el escritorio (`623d089`, con todas las fases unidas y los dos cambios de la Fase
H). Desde ahí cambiaron Markdown y un comentario de la `0028` (`842e088`); con ese cambio se repitieron `go-unit` y
la integración de `database` y de `cmd/ad-el-pos-server`, que aprobaron.

**Las pruebas no tocaron la instalación real**: ni el servicio `local.adel.pos.server` (0.14.0), ni `/Library/Application
Support/AD-EL POS`, ni `/Applications/AD-EL POS.app`, ni `.local` del repositorio, ni el Keychain, ni PostgreSQL en
55432, ni Go en 8181 o en la escucha de red 8443. Ninguna prueba usó `sudo`, `installer` ni `launchctl` (salvo el
`launchctl` simulado de las pruebas de los scripts del instalador), ni las órdenes de `dev.sh` de la instalación. **La
hora de la Mac nunca se cambió**: el reloj atrasado y los saltos se probaron con el reloj simulado de Go
(`ADEL_TEST_CLOCK_FILE`).

**Ninguna prueba automática llamó a `licensing.sicrat.com`.** Solo la Fase G y la G2 lo usaron, a mano, con instancias
de desarrollo separadas y `ADEL_LICENSE_ALLOW_REMOTE=1`, como autorizó el propietario (§0 de la especificación).

**Los códigos de las licencias de prueba** nunca se escribieron en el repositorio, en commits, en el avance, en los
registros ni en las capturas. Aquí se nombran «la Perpetua», «la Mensual» y «la Prueba».

**Datos.** Todas las pruebas usan **datos sintéticos**, creados por cada prueba en un Go temporal con base desechable.
La etapa no usó el respaldo del sistema anterior ni datos del restaurante.

## Entorno aislado

El de la A3 ([VERIFICATION_STAGEA3.md](VERIFICATION_STAGEA3.md), «Entorno aislado»), con lo que agregó la etapa:

- Un árbol de trabajo por fase en el directorio temporal de la sesión, con `.tools`, `node_modules` y las cachés
  enlazadas. Ese directorio es el `$SP` de este documento. No se conserva: los registros que se citan vivieron ahí.
- El clúster PostgreSQL 18.6 temporal en 55499, solo en loopback, con una base `adel_pos_test_*` desechable por paquete,
  por motor de Playwright y por Go temporal.
- **Varios agentes a la vez** (el propietario quitó el límite en esta etapa), cada uno con su `ISO_TAG` y sus puertos.
  `go-int ./cmd/ad-el-pos-server` usa puertos fijos (8192–8196): se corrió con un candado, uno a la vez.
- **Las ayudas de la licencia**, solo con `test_mode` y en una compilación de desarrollo: `ADEL_TEST_LICENSE_TRUST`
  (las llaves de `contracts/licencias/v1/fixtures/vectors.json`), `ADEL_TEST_CLOCK_FILE` (el reloj simulado) y
  `ADEL_TEST_LICENSE_CA` (la autoridad del servidor de licencias simulado de `license-test server`).
- **Playwright** con `ISO_LICENSE_SIM=1`: el arnés arranca el servidor de licencias simulado en loopback y un Go sin
  activar; `native-driver.ts` activa cada base nueva con una licencia de prueba (L1-D51).
- Los emuladores, **sin ventana y de solo lectura**, siempre con `ANDROID_SERIAL` y con tope de tiempo en cada `adb`.
  Durante las fases, `Pixel_Tablet` (Android 17) y `ADEL_A1_Tablet_API26` (Android 8.0); en la regresión final,
  también el teléfono `Pixel_8` (Android 17). `ADEL_A1_Phone_API33`, del propietario, no se tocó.

Ninguna prueba corrió en un teléfono o una tableta reales, ni en la wifi de un restaurante. Vitest y Playwright
sustituyen el puente nativo del escritorio por la simulación de `e2e/native-driver.ts`, como en las etapas anteriores.

## Fase 0 · verificación

Un solo agente. Detalle en el avance, Fase 0.

- El clon del servidor de licencias en `v1.0.0` (commit `309dcff2…`): el SHA-256 del contrato **coincide**
  (`e3435f16…314058`).
- Las 35 copias de `contracts/v1/`, el contrato y la guía, idénticas al clon y a los blobs de la etiqueta. Se excluyen
  del formato (`.prettierignore`), y `ORIGEN.md` lleva el repositorio, la etiqueta, el commit y los SHA-256.
- El verificador de referencia: **67 casos aprobados** (20 + 17 + 10 + 13 + 4 + 3), en el clon y desde la copia.
- Dieciocho dudas del contrato y un plan por fase. El coordinador tomó las decisiones L1-D1 a L1-D24; ninguna fue una
  parada.

## Resultados ejecutados por fase

| Fase o parte (fusión)                               | Lo que se ejecutó y su resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| --------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A (`27cb24c`)                                       | `TestContractVectors`: los **67 casos** de `vectors.json`; el almacén (corte a la mitad, permisos, revisión), lo alterado o de otra instalación, la identidad y la huella (lectura real de `ioreg`, sin mostrar el valor); `TestProductionBuildFailsWithoutProductionKeys`; `go-unit`; `go-int` de `licensing` y de `cmd/ad-el-pos-server` (dos corridas, con el candado)                                                                                                                                                                                  |
| D (`f4b42a3`)                                       | `licenseseats` (unitarias e integración: la concurrencia de 12 vueltas × 3 cupos × 8 transacciones por el último cupo, una sola lo obtiene; **sin el lock la prueba falla**); `operations` completo (543 s, `TestStageL1LicenseSeats` por la API real); `administration` (una prueba ajustada a `limited: true`, L1-D22); `database` (`TestStageL1Upgrade`), `apicontract`, `catalog`, `identity`, `designer`, `reports`, `reports/operational` y `cmd/ad-el-pos-server`; `go-unit`; prueba de humo con el binario real                                    |
| R1 (revisión, sobre `27cb24c`)                      | Siete pruebas nuevas del revisor; tres «…Defect…» fallaron como se esperaba (R1-1 a R1-3) hasta su corrección en la Fase B                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| B con las correcciones de R1 (`5cd23dd`)            | El cliente contra el servidor simulado (cada código de error, el reintento con el mismo cuerpo, `Retry-After`, la transferencia), los `.licreq` idénticos byte por byte a `vectors.json`, `.lic`, el código comercial, el simulador, `license-test` y `TestTestToolsNeverShip`, `TestNoAutomatedTestCallsTheRealLicenseServer` (que falló a propósito con una especificación temporal que nombraba el servidor real); `go-int` de `administration` (274 s, `TestLicenseRoutes`), `cmd`, `database`, `apicontract` y `licensing`; `go-unit`                 |
| C (`91f17ba`)                                       | `TestOperationTableOfTheContract` (11 filas × 6 estados), la clasificación de 75 operaciones, la hora (el límite exacto de 10 minutos), el código de soporte (744 variantes con un carácter cambiado: se aceptan como mucho 2) y el desbloqueo (válido, repetido, de otra instalación, vencido, alterado); `TestEveryOperationHasItsLicenseClass` (**405 operaciones**: `api.rs`, `openapi.json` y las locales); `TestStageL1LicenseGate`, `TestStageL1LicenseGateWithoutSession` y `TestContractV1License`; `go-int` de cinco paquetes, `cmd` y `go-unit` |
| G (sin commits de código)                           | Contra `licensing.sicrat.com`: la §7 de la guía (abajo)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| F (`774c6f0`)                                       | JVM **413** (62 XML, 0 fallas) con `ktfmtCheck` y Lint; instrumentadas 168 en cada tableta (0 fallas); de punta a punta `license` 8/8, `waiter` 19/19 y `access` 6/6 en las dos, `takeaway` 9/9 en Android 17 y 8/9 en Android 8 (`t6`, el pendiente conocido A3-D32, sin ningún rechazo de licencia, cupo u hora, L1-D48); 24 capturas en tema claro                                                                                                                                                                                                      |
| R3 (revisión) y sus correcciones (`df63422`)        | Cuatro hallazgos (R3-1 alto: el comandero por actividad, L1-D36). Las pruebas del revisor fallaron hasta la corrección; la concurrencia por el último cupo pasa y **falla sin el lock** (cocina). La corrección de R2-2 (L1-D43 y L1-D49) con tres mutaciones que hacen fallar sus pruebas. `go-int` de `licenseseats`, `operations`, `administration`, `database`, `apicontract` y `cmd`                                                                                                                                                                  |
| E (`20d2725`)                                       | Vitest 105 archivos, **999 pruebas**; `cargo test` 115 y 3 ignoradas; `workflow-stagel1-*` 5/5 en cada motor; la suite completa de Chromium 46 + 4 omitidas (las de instalación limpia) y las relacionadas en WebKit 15 + 4; una copia aislada de `browser-tests.py` 12/12 en cada motor con las 4 de instalación limpia; 21 capturas por motor. El árbol con D, E y F unidas: 12/12 en cada motor                                                                                                                                                         |
| R2 (revisión) y sus correcciones (`f7fcbff`)        | Seis hallazgos (R2-1 medio: la baja de la hora sin su auditoría). Las pruebas del revisor fallaron hasta la corrección; después, `go-int` de `licensing`, `database`, `administration` (280 s), `operations` (una prueba ajustada; `TestStageL1LicenseSeats` dio una falla de tiempo con la Mac cargada y pasó sola), `cmd` con el candado y `go-unit`                                                                                                                                                                                                     |
| G2 (sin commits de código)                          | La Perpetua con el plan elemental, contra `licensing.sicrat.com` (abajo)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| H, los dos cambios pequeños (`9384e63` y `623d089`) | **L1-D56**: `TestLicenseRoutes` (`administration`) exige 409 `LICENSE_IDENTITY_LOCAL_ONLY` desde la red, con el texto, y que no aumente `access.denied`; aprobada, y **falla con el 403 anterior** («esperado 409, recibido 403»). **L1-D58**: Vitest 105 archivos, **1 002 pruebas** (las nuevas: Vencida con la aclaración, Revocada sin ella, Tolerancia con «Tu licencia está bien…» y la aclaración, Vigente sin ella) y `workflow-stagel1-clock` ajustada al texto nuevo (corrida en la regresión)                                                   |
| Punta de la etapa                                   | «Regresión final» (abajo)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |

En cada fase se corrieron además `format`, `fmt-check` y `sync-check`, y los MD5 de `docs/etapas/`, del contrato, de la
guía y de `contracts/` quedaron iguales antes y después.

## Revisiones independientes

Las tres partes de seguridad que pide la §0 de la especificación, cada una con un agente que no escribió el código.
Ninguna cambió código de producción: solo agregaron pruebas que fallaban hasta corregir su hallazgo.

| Revisión                                   | Hallazgos                                                                                                                                                                                                                                        | Corrección y decisiones                                                                                                                                                                                                         |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **R1**: la verificación (§10) y su almacén | R1-2 (alta): una carpeta copiada con `estado.json` borrado no se detectaba. R1-1 y R1-3 (medias): un estado ilegible se sobrescribía; borrar `estado.json` devolvía una revisión superada. R1-4 a R1-6 (bajas)                                   | En la Fase B: la huella firmada en `license-machine.json` (L1-D29), `ErrStateNotSaved` (L1-D30), la segunda copia de las revisiones en la base (L1-D31), `sync-check` y `.gitignore` (L1-D32); R1-5 no se cambia (L1-D33, V1.1) |
| **R2**: la protección de la hora           | R2-1 (media): la §6.3 y la §6.4 bajaban la hora antes de su auditoría. R2-2 a R2-6 (bajas): la actividad con fecha futura, `estado.json` editado, la semilla sin reintento, el desbloqueo en una carpeta copiada y `LICENSE_LOCAL_ONLY` como 403 | L1-D42 (auditoría primero), L1-D43 y L1-D49 (la fecha futura se baja), L1-D44 (segunda copia de la hora en la base), L1-D45, L1-D46 y L1-D54 (409); la Fase H dio el mismo trato a `LICENSE_IDENTITY_LOCAL_ONLY` (L1-D56)       |
| **R3**: la asignación atómica de cupos     | R3-1 (alta): un pedido sin conexión sin sesión ocupaba un comandero que no contaba. R3-2 a R3-4 (bajas): el reloj de los cupos y el de las sesiones, el orden de los locks y la renovación de las previas                                        | L1-D36 (el comandero por actividad), L1-D37 (`SessionNow`), L1-D38 (`CLAUDE.md` §3: 7820148306 antes de 7820148311) y L1-D39 (las previas sin lock)                                                                             |

## Contra `licensing.sicrat.com` (Fases G y G2)

Con instancias de desarrollo separadas del servicio instalado, cada una con su carpeta, su base temporal y su propia
identidad, y los códigos leídos dentro de un script sin mostrarlos. Detalle en el avance, «Fase G» y «Fase G2».

**Plan y cupos de cada licencia.**

| Licencia    | Tipo                      | Plan                                                                       | Cajas | Comanderos | Pantallas de cocina |
| ----------- | ------------------------- | -------------------------------------------------------------------------- | ----- | ---------- | ------------------- |
| La Perpetua | `perpetual`               | **Elemental** desde el 24/11/2026 (Fase G2, L1-D59); antes, básico (1/0/0) | 3     | 3          | 3                   |
| La Mensual  | `subscription`, `monthly` | Premium                                                                    | 5     | 10         | 5                   |
| La Prueba   | `trial`                   | Elemental                                                                  | 3     | 3          | 3                   |

**Resultados de la §7 de la guía.**

| Prueba                                                                                              | Resultado                                                                                                                                           |
| --------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1 a 6: la cadena de la Prueba (activar, repetir, otra instalación, transferir dos veces, consultar) | **Aprobadas**                                                                                                                                       |
| 7: renovar o cambiar de plan en el panel y «Buscar actualización»                                   | **La hace el propietario** (lista abajo). La G2 recibió el plan nuevo con una activación nueva, no con una revisión mayor de una activación vigente |
| 8: revocar en el panel y consultar                                                                  | **La hace el propietario**                                                                                                                          |
| 9: liberar y activar en otra computadora                                                            | **Aprobada**                                                                                                                                        |
| 10: aprobar en el panel los `.licreq` guardados e importar cada `.lic`                              | **La hace el propietario**                                                                                                                          |
| 11: un código inexistente o mal escrito                                                             | **Aprobada**                                                                                                                                        |
| 12: reintento y `REQUEST_CONFLICT`                                                                  | **Aprobada**, con el defecto G-1 del servidor de licencias (L1-D40)                                                                                 |
| 13: un `.lic` alterado, de otra instalación o con una revisión menor                                | **Aprobada**                                                                                                                                        |
| 14: atrasar el reloj (con el reloj simulado)                                                        | **Aprobada**                                                                                                                                        |

- **Los cupos reales de cada plan**, por la API v1: hasta el límite se ocupan y el siguiente recibe 409
  `LICENSE_SEAT_UNAVAILABLE` con sus cifras, en los tres cupos de los tres planes. Con la Perpetua en el plan elemental
  (G2), 3 terminales capturan sin turno, 3 entran a Preparación y 3 abren turno; la cuarta de cada una, 409.
- **Las tres licencias quedaron liberadas**, sin transferencias pendientes. La Perpetua está libre para activar la Mac
  real. Los `.licreq` de la prueba 10 están en `~/.adel-pos-dev/licreq/` (carpeta 0700, archivos 0600).
- **Defecto G-1** (L1-D40): un `request_id` reutilizado con otro desafío responde `INVALID_PROOF` en vez de
  `REQUEST_CONFLICT`. No afecta a AD-EL POS, que reintenta siempre con el mismo cuerpo. Lo decide el propietario.
- **Búsquedas de códigos** en los archivos de las instancias, en sus registros y en su auditoría: ninguno.

## No verificado

- **La lista del propietario** (en el panel del servidor de licencias): la prueba 7 de la guía (cambio de plan con una
  activación vigente y «Buscar actualización»), la 8 (revocar) y la 10 (aprobar los `.licreq` guardados e importar cada
  `.lic`), y lo que solo se ve en el panel («Computadoras y transferencias», la alerta de `TRANSFER_LIMIT`). Los pasos
  están en «Instalación real».
- **Producción**: no existen el servidor de licencias de producción, sus llaves ni la llave de soporte de producción.
  Solo se comprobó que la compilación de producción **no compila** sin ellas
  ([licencias/PASO_A_PRODUCCION.md](licencias/PASO_A_PRODUCCION.md)).
- **La activación de la Mac real con la Perpetua**: la hace el propietario después de instalar el `.pkg` 0.15.0.
- **Windows**: solo `GOOS=windows` para Go, también con `integration` y `legacydocker`. La lectura real de `MachineGuid`
  y la ACL de los archivos de la licencia, solo compiladas. `scripts/windows/dev.mjs` pasa la confianza de prueba a sus Go
  temporales (L1-D52), pero no arranca el simulador ni el Go sin activar, así que ahí se omitirían la activación y la
  hora; no se probó en Windows.
- **`scripts/browser-tests.py` tal cual** (`dev.sh test-browser`): usa `.local/server.json` y el clúster 55432, que la
  etapa no toca. La Fase E probó una copia aislada con las especificaciones de la etapa y de la 9H; la regresión usó el
  arnés aislado, que hace lo mismo.
- **El desbloqueo de soporte aceptado en la interfaz**: necesita la llave privada de soporte, que nunca está en las
  pruebas; lo aceptan las pruebas de Go con una llave de prueba (L1-D51).
- **Un respaldo restaurado con `pg_restore` en otra computadora**: «Sin activar con datos» se probó con la misma base y
  una identidad sin licencia, que es lo que ve una computadora nueva.
- **Reiniciar la escucha de red después de la identidad nueva**: solo se responde `restart_required`.
- **La aplicación real de Tauri, dispositivos físicos y wifi real**: Playwright usa la vista previa con el puente
  simulado; Android, solo emuladores.
- **El lector con Docker real**: Docker Desktop estaba apagado.
- **La carga con varias terminales reales**: el lock 7820148311 se suma a cada captura; no se midió fuera de las pruebas
  de concurrencia.
- **Limitaciones aceptadas**: un escritorio 0.14.0 conectado a un servidor 0.15.0 entra a Preparación sin ocupar cupo de
  cocina (L1-D28); las sesiones emitidas durante un salto del reloj conservan su vencimiento largo (L1-D50).
- **Android**: `takeaway t6` en Android 8, el pendiente conocido A3-D32 (L1-D48), que volvió a fallar en la regresión
  final sin ningún rechazo de licencia, cupo u hora; `--suite all` de un tirón en Android 8 (la instalación por Gradle
  se colgó, A3-D33, y se completó por partes); `ADEL_A1_Phone_API33`, del propietario; la suite `connection`; los
  avisos de la licencia en tema oscuro; y la actualización de la 0.3.0 a la 0.4.0 en Android 17 (se hizo en Android
  8). Detalle en «Regresión final › Android».

## Regresión final

### Servidor y escritorio

> **Resumen.** Una sola vez (§0, «Eficiencia»), sobre una **copia exacta de `623d089`**, la cabeza de `etapa-l1-h` con
> todas las fases unidas y los dos cambios de la Fase H. **Aprobó todo a la primera**: el servidor (Go, integración de
> los **16 paquetes**, `cargo` y los scripts del instalador), Vitest y `build`, y **Playwright 50 de 50 en Chromium y
> 50 de 50 en WebKit**, con las cuatro de «instalación limpia» de la 9H. No hubo que corregir código ni
> pruebas. Desde `623d089` cambiaron Markdown y un comentario de la `0028` (`842e088`, abajo).

La copia salió de `git archive` (`git rev-parse` = `623d089ba2d340eb50b3fd5f599da7e429533557`; 1 976 archivos), sin
`.git`, con `.tools`, `.cache/go-mod`, `.cache/playwright` y `desktop/node_modules` enlazados, y con el arnés aislado
(`ISO_TAG=HS`): el clúster temporal 55499 con una base desechable por paquete y por corrida de Playwright; Playwright
con su Go temporal en 8217, la vista previa en 1441, el servidor de licencias simulado en 8491 y el Go sin activar en
8219 (`ISO_LICENSE_SIM=1`), y un Go limpio nuevo por corrida (`ISO_TAG=HSC`, 8229, con su escucha de red en loopback 8501) para las cuatro de «instalación limpia». Go 1.27.3, Playwright 1.65.0, PostgreSQL 18.6, macOS 26.5.2.
24/11/2026, de 09:50 a 10:48 (hora de Monterrey). A la vez, otro agente corría la regresión de Android con sus
emuladores (consolas 5580, 5582 y 5586) y su Go temporal en otros puertos, y hasta las 09:55 el coordinador corría `go-unit` y la integración sobre `cdc2ce2`:
la carga media (de un minuto) fue de 26 a 49 hasta las 09:55 y de 4 a 12 desde las 10:00. **No se cambió código en la copia**: quedó idéntica por md5 (los 1 976 archivos) y
`md5 -q docs/etapas/*.md | md5` = `f0a12ff716ded1e1c7a80e72cc52e761` antes y después.

El servidor y Playwright corrieron **en paralelo**, cada uno con su base y sus puertos. `./cmd/ad-el-pos-server` se
corrió al final de la integración, con el candado de los agentes y los puertos 8192–8196 y 55511–55513 libres.

| Suite                                                                          | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `fmt-check` (comprobación de datos reales, Prettier, `gofmt`, `cargo fmt`)     | Aprobado. En la copia, la comprobación de datos reales dice «sin .git … no hay nada que comprobar»; en el árbol de trabajo, con su índice de git en `623d089`: «el índice de git no tiene respaldos ni paquetes de migración.»                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `sync-check`                                                                   | «Metadatos coherentes.» (0.15.0, esquema 28)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `go-unit`                                                                      | Aprobado: `go vet` (normal e `integration`), `go test -race -count=1 ./...` (**39 paquetes `ok`**, 7 sin pruebas de unidad: `events`, `idempotency`, `product`, `reports/operational`, `migrations`, `licensing/licensingtest` y `licenseseats/seatstest`), `GOOS=windows` `go build` y `go vet` (normal e `integration`). Incluye `TestNoAutomatedTestCallsTheRealLicenseServer`, `TestTestToolsNeverShip` y `TestProductionBuildFailsWithoutProductionKeys`                                                                                                                                                                                                                                                                                                    |
| `go vet -tags 'integration legacydocker'`, en macOS y con `GOOS=windows`       | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Integración PostgreSQL, los **16 paquetes** de `scripts/database.py`, con `-v` | **16 de 16**: `database` 19 s, `administration` 313 s, `catalog` 26 s, `operations` 813 s, `payments` 3 s, `reports` 163 s, `reports/operational` 49 s, `designer` 28 s, `identity` 23 s, `apicontract` 53 s, `backupschedule` 10 s, `discovery` 5 s, `legacyimport` 15 s, `licensing` 13 s, `licenseseats` 5 s y `cmd/ad-el-pos-server` 72 s (con `TestServiceHostOnMacOS`, 64 s). **Ninguna falla ni ningún 503**. Las únicas omitidas son las opcionales de siempre: las mediciones (`TestLegacyImportVolume`, `TestStage9HLegacyReportsVolume`, `TestReportsVolumeMeasurement`, `TestOperationalVolume` e `inventory_b_measure_release`), la impresora física (`TestControlledCUPSPrinter`) y la búsqueda mDNS en vivo (`TestHoldAnnouncementForLiveSearch`) |
| `TestOperationalReports`                                                       | Aprobada, **no omitida**: corrió a las 10:16, fuera de la franja de 00:00 a 05:00                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Docker real del lector                                                         | **No se corrió**: Docker Desktop apagado (sin `~/.docker/run/docker.sock`); no se encendió                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `vitest` (`tsc --noEmit` + Vitest)                                             | **105 archivos, 1 002 pruebas**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `build` (`npm run build`)                                                      | Aprobado (el aviso de tamaño de fragmento de siempre)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `cargo test --locked --offline`                                                | **115 aprobadas, 3 ignoradas**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `scripts/macos-pkg/test-scripts.sh` (sin root) y `bash -n`                     | «Scripts del instalador: todo correcto» (31 comprobaciones); `bash -n` de `common.sh`, `test-scripts.sh`, `app/preinstall`, `server/preinstall` y `postinstall`, `service/postinstall`, `build-macos-pkg.sh`, `check-legacy-data.sh` y `android-e2e.sh`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| **Playwright Chromium** completo (50 pruebas, 39 especificaciones)             | **50 de 50, a la primera** (20,6 min), con las 4 de «instalación limpia» de la 9H (ninguna omitida) y las 5 de la etapa: `workflow-stagel1-activation` (2), `-clock` (con el texto de L1-D58), `-notices` y `-seats`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| **Playwright WebKit** completo                                                 | **50 de 50, a la primera** (20,3 min), con las mismas 4 de «instalación limpia» y las 5 de la etapa                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |

Pantalla de la Mac **desbloqueada** (`ioreg … CGSSessionScreenIsLocked` sin la clave) antes y después de cada corrida
completa. Chromium corrió de 10:06 a 10:27 y WebKit de 10:27 a 10:47, con la carga entre 5 y 12.

**Captura de la pantalla cambiada** (§0: solo las pantallas nuevas o cambiadas, en tema claro). Después de la suite,
`workflow-stagel1-clock` sola en WebKit (1 de 1, 37 s) guardó sus capturas en `$SP/l1/hs/capturas/`. La de la licencia
vencida con la hora bloqueada (`webkit-hora-y-licencia.png`) muestra «Además, la licencia: Vencida», su explicación y la
frase nueva de L1-D58, sin «Tu licencia está bien…» ni textos provisionales. La variante de Tolerancia solo la cubre
Vitest.

**Ninguna prueba llamó a `licensing.sicrat.com`**, comprobado en las bitácoras de los Go temporales de Playwright:

- el Go temporal compartido y el Go sin activar tenían `license_server_url` = `https://127.0.0.1:8491` (el simulador). En
  la bitácora del compartido, la única consulta al servidor de licencias fue al simulador: la consulta en segundo plano
  de una licencia firmada con `license-test issue`, que el simulador no conoce (`INVALID_PROOF`, lo esperado, L1-D51);
- el Go limpio no tenía `license_server_url`: su cliente quedó deshabilitado al arrancar (`license_server_disabled`,
  `remote_not_allowed`), y `native-driver.ts` lo activó importando una licencia de prueba (`license_test_trust_enabled`
  y `license_imported`);
- ninguna de esas bitácoras ni la del simulador contiene el nombre del servidor real.

`TestNoAutomatedTestCallsTheRealLicenseServer` (en `go-unit`) comprueba además que ningún archivo de pruebas del
repositorio lo nombra.

### Android

> **Estado: completa, con el pendiente conocido `takeaway t6` en Android 8 (A3-D32, L1-D48)** (24/11/2026).
>
> - **JVM, Robolectric, Lint y formato**: 62 XML, 413 pruebas, 0 fallas; Lint sin errores ni avisos.
> - **Instrumentadas en los tres emuladores**: 168 pruebas cada uno, 0 fallas.
> - **`--suite all`**: 84/84 en `Pixel_Tablet` y en `Pixel_8`; 83/84 en `ADEL_A1_Tablet_API26`, por partes.
> - **Hallazgo H1, en la prueba**: `license l1`, `l2` y `l5` fallaban en el teléfono. Se corrigió en `59689224` y la
>   suite `license` dio 8/8 en `Pixel_8`.
> - **Falla que se repitió**: `takeaway t6` en Android 8, igual que en la A3 y la Fase F, sin ningún rechazo de
>   licencia ni de hora.
> - **Problemas del entorno**:
>   - el primer intento de las instrumentadas en `Pixel_Tablet` chocó con «System UI isn't responding»;
>   - en Android 8, la instalación por Gradle se colgó al empezar `waiter` (A3-D33).
> - **El APK 0.4.0** está firmado con la llave de desarrollo y se instala encima de la 0.3.0 sin desemparejar.
> - **Ninguna prueba llamó a un servidor de licencias**: los 34 Go temporales arrancaron con
>   `license_server_disabled` (`remote_not_allowed`).

**Qué se probó.**

- **La cabeza `cdc2ce2`** (`etapa-l1` con todas las fases unidas), en el árbol `$SP/wt/l1-ha`, rama `etapa-l1-ha`:
  JVM, instrumentadas de `Pixel_Tablet` y `Pixel_8` y `--suite all` en los dos.
- **`59689224`**, la cabeza final: el resto, que es la suite `license` repetida en `Pixel_8`, todo lo de
  `ADEL_A1_Tablet_API26` y el APK.
  - Agrega a `cdc2ce2` solo la corrección de `LicenseOnGoTest.kt` (hallazgo H1). La aplicación es la misma.
- **Qué cambió desde la Fase F**: `git diff --name-only 9c1070f 59689224` no toca nada de `android/` fuera de esa
  corrección, ni `scripts/android-e2e.sh`. Sí cambió el servidor (la revisión R2 de la hora, la Fase G y sus
  correcciones), así que los Go temporales son los de la cabeza (0.15.0, `test_mode: true`).

**Entorno.**

- **Go temporales**: `ISO_TAG=HA` (8221/8495), uno **nuevo para cada parte** (`ADELPOS_E2E_NEW_SERVER`, que detiene el
  anterior y levanta otro en las mismas direcciones).
  - Cada uno confía en las llaves de `vectors.json` y lee su reloj de un archivo propio.
  - `ensure_test_license` les importa la licencia de prueba sin internet.
  - Se guardaron los 34 registros de Go, y el registro completo (`logcat -b all`) de cada corrida, en `$SP/lh/e2e/`.
- **Emuladores**: sin ventana y de solo lectura, con `ANDROID_SERIAL` y tope de tiempo en `adb`.
  - Uno a la vez durante las pruebas; nunca más de dos encendidos.
  - **`ADEL_A1_Phone_API33` (5554, del propietario) no se tocó.**
  - Ninguna lista mostró un servidor real.
- **Cuándo**: 24/11/2026, de 09:41 a 12:06. Otro agente corría a la vez la regresión del servidor y del escritorio, y el
  coordinador, las pruebas de Go: al principio la carga de la Mac llegó a 120 y después bajó a entre 3 y 20.

| Qué (en la Mac, una vez)                                                                                     | Resultado                                                                                                                                                                                                                                                            | Tiempo |
| ------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| `ktfmtCheck :core:domain:test --rerun testDebugUnitTest --rerun lintDebug` (`--no-build-cache`, árbol nuevo) | **413 pruebas, 0 fallas, 0 omitidas (62 XML)**: `core:domain` 189 (26 XML), `core:data` 191 (28), `core:ui` 15 (5), `feature:access` 7, `feature:takeaway` 5 y `feature:waiter` 6. Lint en los 7 módulos: 0 errores, 0 avisos (3 sugerencias). `ktfmtCheck` aprobado | 101 s  |
| Después de la corrección `59689224`: `ktfmtCheck`, `format` y `fmt-check`                                    | Aprobados. Los 56 archivos protegidos (`docs/etapas`, `docs/licencias`, `contracts`) son idénticos, uno por uno, a los de la cabeza                                                                                                                                  | —      |

Por emulador, siempre con un Go nuevo para cada parte:

- antes de las instrumentadas se desinstalaron la aplicación, sus pruebas y las de la 0.1.0 (`pm list packages`: 0);
- en las instrumentadas se contaron los XML por módulo;
- en `--suite all`, el guion desinstala y reinstala en cada parte.

| Prueba                                                | `Pixel_Tablet` (Android 17, 5580)                                                              | `Pixel_8` (Android 17, 5582)                                       | `ADEL_A1_Tablet_API26` (Android 8.0, 5586)                                                                                                   |
| ----------------------------------------------------- | ---------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Instrumentadas (XML): `app` / `core:data` / `core:ui` | **168, 0 fallas**: 134 / 20 / 14; 82 corridas, 86 omitidas (358 s, segundo intento: ver abajo) | **168, 0 fallas**: 134 / 20 / 14; 82 corridas, 86 omitidas (263 s) | **168, 0 fallas**: 134 / 20 / 14; 80 corridas, 88 omitidas (191 s; Gradle instaló sin colgarse)                                              |
| access (1–5 y 6–7) / catalog                          | 7/7, 3/3                                                                                       | 7/7, 3/3                                                           | 7/7, 3/3 (dentro de `--suite all`)                                                                                                           |
| waiter / field                                        | 19/19, 13/13                                                                                   | 19/19, 13/13                                                       | 19/19, 13/13 (por partes)                                                                                                                    |
| takeaway / precheck                                   | 9/9, 7/7                                                                                       | 9/9, 7/7                                                           | **8/9** (`t6`; repetida: 8/9, `t6` otra vez), 7/7                                                                                            |
| alerts / adjustments                                  | 11/11, 7/7                                                                                     | 11/11, 7/7                                                         | 11/11, 7/7                                                                                                                                   |
| license (`l1`–`l8`)                                   | 8/8                                                                                            | **5/8** (`l1`, `l2` y `l5`: hallazgo H1); con `59689224`, **8/8**  | 8/8 (con `59689224`)                                                                                                                         |
| Total                                                 | **`--suite all` 84/84** (1 839 s)                                                              | **84/84**: `--suite all` 81/84 (1 851 s) y `license` repetida 8/8  | **83/84**: `--suite all` hasta que la instalación de `waiter` se colgó (683 s); el resto por partes con `adb install -r -t` y `--no-install` |

**Las líneas «se repite» (A3-D26)**: ninguna, en la salida y en el registro de cada paso de todas las corridas.

**Ningún servidor de licencias.** Los 34 Go temporales (los 33 de las suites y el de la prueba de actualización)
dicen, al arrancar:

- `license_server_disabled` con `remote_not_allowed`;
- `license_test_trust_enabled` y `license_test_clock_enabled`.

Sus registros no nombran `sicrat` ni `licensing.` ni ningún pedido a un servidor remoto. Las 50 licencias se
importaron sin internet (`license_offline_request_created` → `license_imported`): una para cada Go y las cuatro más
de cada suite `license` (0 comanderos, con cupos, en tolerancia y vencida).

**Hallazgo H1 · `LicenseOnGoTest` en el teléfono (de la prueba; corregido en `59689224`).**

- **Lo que pasaba**: en `Pixel_8`, `license l1`, `l2` y `l5` fallaban en `waitForTables()` justo después de «Enviar a
  cocina» (`LicenseOnGoTest.kt:164`, `:187` y `:272`).
- **Por qué**: en el teléfono, la aplicación vuelve al panel de la mesa (un solo panel, `account-pane`), y ahí el
  pedido ya decía «Requiere tu revisión» y «1 pedido por enviar». La prueba solo aceptaba la lista de Mesas. La
  Fase F la había corrido solo en tabletas, donde la lista siempre está a la vista. `WaiterModeOnGoTest` ya aceptaba
  las dos cosas.
- **La corrección**: `waitForTables()` acepta la lista o el panel de la mesa, como `WaiterModeOnGoTest`, e imprime el
  árbol de la pantalla si no llega. **No cambió la aplicación.**
- **Comprobado**: `license` 8/8 en `Pixel_8` (140 s, Go nuevo) y en `ADEL_A1_Tablet_API26` (121 s).
- `l3`, `l4`, `l6`, `l7` y `l8` ya habían aprobado en el teléfono en la primera corrida.

**Instrumentadas en `Pixel_Tablet`: el primer intento (problema del entorno).**

- **Lo que pasó**: se encendieron `Pixel_Tablet` y `Pixel_8` juntos, con la carga de la Mac entre 45 y 120. Fallaron 2
  de 168 pruebas, las dos con `RootViewWithoutFocusException` (`has-window-focus=false`):
  - `AccessScreensTest.buscarElServidorYElCodigoEnGrande`, en `closeSoftKeyboard`;
  - `AppNavigationTest.mesasYParaLlevarEnLaNavegacionPrincipal`, en `pressBack`.
- **La causa**: en la pantalla estaba el diálogo «System UI isn't responding» (`mCurrentFocus=Application Not
Responding: com.android.systemui`, con captura). Es el caso de «Ojo (emulador cargado)» de `android/CLAUDE.md`.
- **Lo que se hizo**: apagar los dos y repetir con `Pixel_Tablet` recién encendido y solo: 168, 0 fallas.
- Los resultados del primer intento se guardaron aparte.

**Android 8: cómo se completó `all` (A3-D33).**

- **`--suite all`** con la instalación por Gradle: aprobaron `access` (1–5 y 6–7) y `catalog`. Al empezar `waiter`,
  `:app:installDebug` se colgó: `ShellCommandUnresponsiveException` de `ddmlib`, «Unable to install», 9 min 32 s, y el
  guion se detuvo sin correr pasos.
- **El resto, por partes**: cada parte con su Go nuevo. Antes de cada una, `adb install -r -t` de la aplicación y de
  sus pruebas (2 s, «Success»), lo mismo que hace `install_app` del guion: desinstala la aplicación, sus pruebas y las
  de la 0.1.0. La suite corrió con `--no-install`. Los tiempos fueron:
  - `waiter` 167 s, `field` 266 s, `takeaway` 139 s, `precheck` 80 s;
  - `alerts` 612 s, `adjustments` 127 s y `license` 121 s.
- **Las instrumentadas**, en cambio, sí se instalaron por Gradle en este emulador.
- **No se cambió el guion**: es la orden aparte `$SP/lh/parts.sh`.

**`takeaway t6` en Android 8: el pendiente conocido A3-D32 (L1-D48).** `t6_nombreYTelefonoDespuesEnLinea` falló dos
veces (`5586-takeaway-1` y `-2`), en el mismo lugar que en la A3 y en la Fase F:

- **Dónde**: `TakeawayOnGoTest.kt:306`, `waitForText("Luisa Martínez")`, a los 40 s.
- **Lo que sí se cumple**: la condición anterior. El nombre llegó a Go, sin teléfono, y la cola de ediciones quedó
  vacía.
- **Sin rechazos de licencia, cupo u hora**: 0 líneas `LICENSE_`/`SYSTEM_CLOCK` en el logcat del paso y en el registro
  de su Go (solo el arranque y la activación de prueba). No lo causa la L1.
- **En los otros dos emuladores** aprobó.

**Rendimiento** (`field f2`, registro `AdelOnGo`):

| Medida                                                 | `ADEL_A1_Tablet_API26` | `Pixel_Tablet` | `Pixel_8` |
| ------------------------------------------------------ | ---------------------- | -------------- | --------- |
| Arranque en frío (proceso → lista de meseros)          | 1 494 ms               | 2 696 ms       | 2 969 ms  |
| PIN (con conexión) → Mesas con la primera mesa         | 741 ms                 | 1 023 ms       | 938 ms    |
| PIN → la mesa 40 a la vista                            | 1 272 ms               | 1 795 ms       | 1 555 ms  |
| «Nueva cuenta» → catálogo de ~300 productos a la vista | 266 ms                 | 408 ms         | 383 ms    |
| «Enviar a cocina» → «Recibido por el restaurante»      | 323 ms                 | 330 ms         | 312 ms    |
| Memoria (total de `dumpsys meminfo`)                   | ≈ 93 MiB               | ≈ 147 MiB      | ≈ 141 MiB |

Son parecidos a los de la A3 (1 410/797/1 312/266/324 ms y ≈ 91 MiB en Android 8).

**Capturas del teléfono (nuevas; la Fase F solo corrió en tabletas).**

- **Dónde están**: las 12 de la suite `license` en `Pixel_8` (`5582-license-2`) se copiaron a
  `$SP/shots-l1/android/pixel-8/`.
- **Lo que se revisó**:
  - **El aviso de tolerancia en una pantalla angosta**: en la barra solo se ve el ícono, porque el teléfono tiene menos
    de 600 dp (L1-D47). Al tocarlo se abre «La licencia venció · …Quedan 3 días para renovarla…» (`l7-*`). Lo que la
    Fase F dejó sin ver queda visto.
  - **El aviso fijo de la hora** en Mesas y en la captura, con las dos fechas de Go (`l5-*`).
  - **La captura sin cupo**: «La licencia no incluye comanderos (0 de 0)…» (`l1-captura-*`).
  - **La revisión con «Enviar de nuevo»** (`l1-revision-*`) y Pendientes con dos pedidos en revisión (`l3-*`).
- **Resultado**: ningún texto provisional, código ni `message` del servidor a la vista.
- **Observaciones, sin cambio**:
  - En Pendientes del teléfono, la insignia de la barra se corta («2 pedidos por en…»). Con un pedido («1 pedido por
    enviar») se ve completa. La insignia cuenta también los pedidos en revisión, como ya se aceptó en la Fase F.
  - En la captura de `l7` del teléfono, Mesas muestra «Leyendo las mesas…» encima de la lista ya dibujada: la
    lectura en curso al entrar.

**La actualización de la 0.3.0 a la 0.4.0 (APK de entrega).** Se hizo en `ADEL_A1_Tablet_API26`, con la aplicación
normal fuera de la instrumentación.

- **Sin la red real**: la Wi-Fi del emulador estaba apagada (`adb root`, `svc wifi disable`) y el emulador solo tenía
  la red móvil, que llega a 10.0.2.2. La búsqueda dijo «No se encontró ninguna computadora principal en la red».
- **Con un Go temporal nuevo** (HA), configurado por la API de la principal, con la licencia de prueba y una mesera
  («Rosa H», PIN, en un rol con `waiter_devices.use`).

1. **La 0.3.0**: se instaló `AD-EL-POS-Android-0.3.0.apk`, del repositorio (SHA-256 `87477497…ac4d`, misma
   firma `fb6a4bac…7c86`). Se emparejó escribiendo la dirección `10.0.2.2:8495`. La tableta mostró el código de 6
   dígitos de «Restaurante de prueba A1», y la principal lo aprobó por su API. Después, «Selecciona tu usuario» con
   Rosa y, con su PIN, Mesas. En Go, el dispositivo es `4dc6e004…`, «Tableta 1», con `app_version` 0.3.0.
2. **La 0.4.0, encima**: `adb install -r` del APK nuevo («Success»). `dumpsys package`: `versionName` 0.4.0,
   `versionCode` 4, la misma `firstInstallTime` y una `lastUpdateTime` nueva.
3. **Al abrirla**: «Selecciona tu usuario» con Rosa y «En línea». **No pidió emparejar.** Rosa entró con su PIN a
   Mesas. En Go es el mismo dispositivo (`4dc6e004…`), sin retirar y con `last_seen_at` nuevo.
4. **Al terminar**: se volvió a encender la Wi-Fi, se desinstaló la aplicación y se apagaron el Go y el emulador.

- **Observación**: Go sigue mostrando `app_version` «0.3.0» para ese dispositivo.
  - Por qué: Go la guarda solo al emparejar (`backend/internal/administration/pairing.go:547`).
  - Es así desde antes de la L1. Terminales enseñará la versión con que se emparejó, no la instalada.
  - El único uso en el código (`kitchen/preparation.go:186`, ≥ 0.2.0) no cambia.
- Las capturas están en `$SP/lh/upgrade/`.

**No verificado en esta parte:**

- `takeaway t6` en Android 8 (A3-D32, L1-D48).
- `--suite all` de un tirón en Android 8: la instalación por Gradle se colgó en `waiter` y se completó por partes.
- La corrección H1 no se repitió en `Pixel_Tablet`: allí `license` ya aprobaba con la prueba anterior, y la nueva
  aprobó en la otra tableta (Android 8) y en el teléfono.
- `ADEL_A1_Phone_API33` (Android 13): es el emulador del propietario y el encargo lo excluye.
- La suite `connection` (la hora sin uso, fuera de `all`).
- Los avisos de la licencia en tema oscuro: `field f10` hace las capturas oscuras de las pantallas principales, no las
  de `license`.
- Un dispositivo físico.
- La actualización de la 0.3.0 a la 0.4.0 en Android 17: se hizo solo en Android 8.

## Entregables

| Archivo                                                                  | Bytes      | SHA-256                                                            | Qué es                                                                                                                                                                                                       |
| ------------------------------------------------------------------------ | ---------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `desktop/src-tauri/target/release/bundle/macos-pkg/AD-EL POS_0.15.0.pkg` | 23 850 119 | `2ef89832bb703f5bc69ccf01b621c5b28eed4f6b1537b171e2fd01ea3f6b669b` | Aplicación y servidor 0.15.0 (esquema 28), **versión de desarrollo** (confía en el entorno `test`), compilado sobre una copia exacta de `842e088`; firma ad hoc verificada                                   |
| `android/app/build/outputs/apk/release/AD-EL-POS-Android-0.4.0.apk`      | 2 674 175  | `9ed2733f61f55faf88621a08d6c0b942c1a63c3c5eaba85c0c1f74cda0f29146` | `com.adelpos.meseros` 0.4.0 (`versionCode` 4, `minSdk` 26), firmado con la llave de desarrollo (esquema v2, `fb6a4bac…7c86`, la misma de la 0.2.0 y la 0.3.0), compilado sobre una copia exacta de `5968922` |

**El `.pkg` 0.15.0** se construyó con `./scripts/dev.sh build-macos-pkg` en una copia exacta de `842e088` (`git archive`,
1 979 archivos, con `CARGO_NET_OFFLINE=true`, `GOFLAGS=-mod=readonly` y `GOPROXY=off`), en 122 s, sin instalar nada (ni
`sudo`, ni `installer`, ni `launchctl`). Desde `623d089`, la copia de la regresión, cambiaron Markdown y **un comentario
de la `0028`** (`842e088`): decía que el comandero cuenta con una sesión vigente, y desde L1-D36 cuenta por actividad. Se
corrigió antes de instalar, porque las migraciones llevan su SHA-256 en `schema_migrations` y después ya no se podría.
Con ese cambio se repitieron `go-unit` y la integración de `database` y de `cmd/ad-el-pos-server`: aprobaron. El
servidor de dentro lleva la `0028` con el comentario nuevo. Un primer paquete sobre `16090b8` (SHA-256 `fe60d292…9443`)
se reemplazó por este. Comprobaciones:

- **`pkgutil --expand-full`**:
  - Los tres paquetes son 0.15.0: `local.adel.pos.app`, `.server` y `.service`.
  - La distribución ofrece «Servidor del restaurante» marcada por omisión (`start_selected="true"`), el servicio oculto
    y la arquitectura `arm64`.
  - La aplicación de dentro es 0.15.0 (`local.adel.pos.dev`), arm64.
  - El servidor de dentro dice `0.15.0` con `version`, lleva `"schema_version": 28` y la migración
    `0028_licencias.sql` embebida.
  - Los scripts son idénticos a los de la copia.
- **`codesign --verify --deep --strict --verbose=2`** de la aplicación de dentro: «valid on disk» y «satisfies its
  Designated Requirement», firma `adhoc`. El `.pkg` no tiene firma de desarrollador, como dice el script.
- **Confía en el entorno `test`** (compilación de desarrollo, §1.2 de la especificación): `go version -m` del servidor de
  dentro no muestra `-tags` (no es la de `adelpos_production`), y el binario lleva el `kid` `adelpos-test-202611-1` con
  la llave pública de la §1.2, el `kid` de soporte de desarrollo (`adelpos-soporte-development-20261123`) y la dirección
  del servidor de licencias de pruebas.
- **No lleva lo que no debe**: ningún `kid` ni semilla de `vectors.json`, ningún archivo `vectors.json`, `.lic`,
  `.licreq`, `.pem` ni `.key`, ninguna llave privada (`PRIVATE KEY`), ni `license-test` ni `soporte-desbloqueo`. En
  `Contents/MacOS` solo están `ad-el-pos-desktop` y `ad-el-pos-server`.
- **Copias**: junto a los de 0.10.0 a 0.14.0, que siguen ahí con el mismo SHA-256 (el de la 0.14.0, `d9d02a6e…a205`), y
  en `$SP/AD-EL POS_0.15.0.pkg`.

Instalarlo es la parada del propietario.

**El APK 0.4.0** se compiló con `:app:assembleRelease` (`--no-build-cache`, 72 s) sobre una copia exacta de `5968922`
(`git archive`, 1 976 archivos, los mismos que `git ls-files`), nunca con `gradle clean` en el repositorio. De `cdc2ce2`
a `5968922` solo cambió una prueba instrumentada, que no entra en el APK. Comprobaciones:

- **`apksigner verify`**: esquema v2, un firmante, «CN=AD-EL POS desarrollo - APK de prueba, O=AD-EL POS», SHA-256
  `fb6a4bac93052ca9051342ea122ed90469340a3685de14243bdab288d71c7c86`.
- **La versión**: `com.adelpos.meseros` 0.4.0, `versionCode` 4, `minSdk` 26, `targetSdk` 36 y `compileSdk` 37.
- **La actualización**: instalado encima de la 0.3.0 en Android 8, sigue emparejado (sección «Android»).
- **Los anteriores no cambiaron** (SHA-256 antes y después): la 0.3.0 (2 672 443 bytes, `87477497…ac4d`), la 0.2.0
  (2 589 899 bytes, `38d687ce…b824`) y el `app-release.apk` del 18/11.
- **Copia**: también en `$SP/AD-EL-POS-Android-0.4.0.apk`.

## Instalación real

La instalación real de la Mac sigue en **0.14.0, esquema 27**: el servicio `local.adel.pos.server`, con PostgreSQL en
55432 y Go en 8181 y 8443. Ninguna prueba de la etapa la tocó.

**Se actualiza con el `.pkg` 0.15.0**, que instala el **propietario** con «Servidor del restaurante» marcada. Es una
parada, porque pide la contraseña de administrador. Al arrancar:

1. El `host` respalda la base (`…-antes-de-migrar.dump`) y aplica la `0028`: crea `app.license_revisions`,
   `app.license_trusted_time` y `app.license_seats`, agrega el permiso `license.manage` al rol Administrador y da su
   cupo de caja a la terminal que abrió cada turno que siga abierto. No cambia ninguna venta, pago, corte, cuenta,
   pedido, sesión, usuario ni terminal.
2. El servidor crea una sola vez la llave de licencias (`identity/license-key.pem`) y el registro firmado de la huella
   de esta Mac, junto a la identidad que ya tiene. La identidad de la red local no cambia: las terminales emparejadas
   siguen emparejadas.
3. **La Mac queda «Sin activar con datos»**, porque la base tiene operaciones y todavía no hay licencia. **Hasta
   activarla no se captura, no se cobra, no se abre ni se cierra turno y no se administra** (tabla de la §6 del
   contrato). Siguen los reportes, el historial, los respaldos y la pantalla de la licencia. Los teléfonos y tabletas
   tampoco capturan ni se emparejan hasta entonces.

**Los pasos para el propietario** (la prueba final de la §10 de la especificación; **nadie más la activa**):

1. Instalar el `.pkg` 0.15.0 con «Servidor del restaurante» marcada.
2. Abrir AD-EL POS en la Mac y entrar con el usuario Administrador. Si no aparece sola la pantalla de activación, ir a
   **Configuración › Licencia** (el botón «Licencia» de Configuración general).
3. Escribir el código de **la Perpetua** y activar en línea. Se espera «Vigente», «Perpetua», el **plan elemental** y,
   en Configuración › Licencia, el entorno «Servidor de licencias de pruebas».
4. Con la Perpetua activada hay **hasta 3 cajas, 3 comanderos y 3 pantallas de cocina** (L1-D59): la Mac funciona como
   caja, y los teléfonos y tabletas capturan y Preparación se abre dentro de esos cupos.
5. Comprobar que se cobra y se captura en la Mac. En Sistema › Terminales se ven los cupos en uso.

Si la activación en línea no funciona (sin internet), la misma pantalla ofrece «Activar sin internet»: guarda la
solicitud `.licreq`, el propietario la aprueba en el panel del servidor de licencias y se importa el `.lic`.

**Los teléfonos y tabletas** con la 0.3.0 se actualizan instalando encima el APK 0.4.0 (misma llave y mismo
identificador). No hace falta desemparejarlos ni borrar sus datos.

**La lista para el propietario** de la Fase G sigue en el avance («Lista para el propietario»), con las instancias
conservadas en `~/.adel-pos-dev/instancias-l1/` y sin tocar la instalación real:

- **Prueba 7** (cambio de plan con una activación vigente y «Buscar actualización»), con la instancia `en-linea` y la
  Prueba. Los pasos actualizados están en el avance, «Fase G2», «Lo no verificado».
- **Prueba 8** (revocar en el panel y consultar), con la Prueba, al final.
- **Prueba 10** (aprobar en el panel los `.licreq` de `~/.adel-pos-dev/licreq/` e importar cada `.lic`).
