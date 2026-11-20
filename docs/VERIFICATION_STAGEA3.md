# Verificación · Etapa A3 (ajustes de Android, permisos agrupados y roles de fábrica)

Verificación por fases del **19 de noviembre de 2026**, en la Mac (Apple Silicon, macOS 26.5.2). Cada fase se hizo en
una rama local que el coordinador fusionó en `etapa-a3`. Esa rama se creó desde `origin/main` (`5096f80`: 0.13.0,
esquema 26, Android 0.2.0, con la A2 fusionada). La especificación está en `05af2b3`. Commits de cada fase:

- **Fase 0**: `4c7231b`. Solo verificación, con experimentos fuera del repositorio.
- **Fase A del escritorio y Go**: `2b5c6fb`, `bad4985` y `a7208f1`. Fusión en `ac50dc9`; avance en `d2991cf`.
- **Fase D**: `838562d` a `7e9a957`. Fusión en `699cb3b`; avance en `3f4e6ea`.
- **Fase A de Android**: fusión en `8da5ad3`; avance en `15b8f08`.
- **Fase B con las correcciones de su revisión independiente** (R1–R5): fusión en `215fd81`; avance en `da899ee`.
- **Fase C**: fusión en `beeb183`; avance en `3403598`. Los textos de Android en `CLAUDE.md` están en `2ef6185`.
- **Correcciones del escritorio que encontró la regresión**: `86c9b5f`, `014fe3f`, `403c354` y `d473fed`. Fusión en
  `85be71e`; avance en `e258fb4`.
- **Correcciones de Android que encontró la regresión**: `34266e4` a `c4754f3`. Fusión en `9038b79`; avance en
  `ca120ee`.
- **Correcciones B1 y B2 de la segunda corrida de Android**: `63d8e3b`, `e60ee75` y `dcec52e`. Fusión en `be46a07`;
  avance en **`49961de`**.

**Servidor 0.14.0.** API v1, migración `0027_android_a3_adjustments.sql` (esquema **27**). Es una sola para toda la
etapa, con dos secciones:

- **«Fase A»**: la nota «Formato predeterminado, igual al anterior a la etapa 9B» de las plantillas pasa a «Formato
  predeterminado», solo donde el texto es exactamente ese. El disparador de inmutabilidad se desactiva y se vuelve a
  activar en la misma transacción.
- **«Fase D»**: en los `android`, una pantalla inicial `kitchen` o `cash` pasa a `tables`, con el `CHECK`
  `devices_android_home_check`.

La Fase B no guarda nada nuevo: el catálogo de permisos y los conjuntos de fábrica viven en Go. `min_client_versions`
no cambia (escritorio 0.10.0 y Android 0.1.0, A3-D9); `min_client_version` y `min_server_version` siguen en 0.10.0.
La capacidad nueva es `permission-groups-v1`.

**Aplicación Android 0.3.0** (`com.adelpos.meseros`, `versionCode` 3, `minSdk` 26). La base local `AppDatabase` sigue
en la versión 6 y `catalog.db` en la 1, sin dependencias nuevas. Sigue exigiendo el servidor 0.13.0
(`MIN_SERVER_VERSION`).

**Referencias.**

- Especificación: [etapas/ETAPA_A3_AJUSTES.md](etapas/ETAPA_A3_AJUSTES.md).
- Avance, con el detalle de cada fase y las decisiones A3-D1 a A3-D30: [ETAPA_A3_AVANCE.md](ETAPA_A3_AVANCE.md).
- Guía del propietario: [ANDROID_INSTALAR.md](ANDROID_INSTALAR.md).
- Reglas y pruebas de la aplicación: [android/CLAUDE.md](../android/CLAUDE.md).
- Evidencia anterior: [etapa A2](VERIFICATION_STAGEA2.md).

Este documento registra **solo lo que se ejecutó**.

**Alcance de esta evidencia.** Las cifras de las fases son las que cada agente registró sobre su propia copia, según el
avance. Lo que valida el código completo de la etapa es la «Regresión final», que se hizo en partes sobre copias
exactas (`git archive`):

- **Servidor** (Go, integración de los 14 paquetes, `cargo` y los scripts del instalador): sobre `da899ee`. Desde ahí no
  cambiaron `backend/`, `config/`, `desktop/src-tauri/` ni los scripts del servidor y del instalador.
- **Escritorio** (Vitest, `build` y Playwright completo en los dos motores): sobre `e258fb4`. Desde ahí, fuera de
  `android/` y la documentación, solo cambió `scripts/android-e2e.sh`.
- **Android**: sobre `49961de`, la cabeza (con las corridas anteriores sobre `3403598` y `ca120ee` como antecedente).

Las repeticiones, con su causa, están en cada sección.

**Las pruebas no tocaron la instalación real**: ni el servicio `local.adel.pos.server` (0.13.0), ni `/Library/Application
Support/AD-EL POS`, ni `/Applications/AD-EL POS.app`, ni `.local` del repositorio, ni el Keychain, ni PostgreSQL en
55432, ni Go en 8181 o en la escucha de red 8443. Ninguna prueba usó `sudo`, `installer` ni `launchctl` (salvo el
`launchctl` simulado de las pruebas de los scripts del instalador), ni las órdenes de `dev.sh` de la instalación. Las
pruebas instrumentadas y de punta a punta usan `AdelTestRunner`, que busca el tipo de anuncio de pruebas
`_adelpostest._tcp`, y emparejan por la dirección escrita del Go temporal. La aplicación normal nunca se abrió con la
búsqueda automática en la red real, y **ninguna lista mostró un servidor real**. La Fase 0 midió la conexión contra un Go
temporal, no contra el servicio. La consulta de solo lectura a la base real que pidió la Fase 0 no se hizo: el
coordinador decidió A3-D12 y A3-D13 sin esos conteos.

**Datos.** Todas las pruebas usan **datos sintéticos**: meseros, roles, catálogos, mesas, pedidos y terminales creados
por cada prueba en un Go temporal con base desechable. La etapa no usó el respaldo del sistema anterior ni datos del
restaurante.

## Entorno aislado

El de la A2 ([VERIFICATION_STAGEA2.md](VERIFICATION_STAGEA2.md), «Entorno aislado»):

- Un árbol de trabajo por fase en el directorio temporal de la sesión, con `.tools`, `node_modules` y las cachés
  enlazadas. Ese directorio es el `$SP` de este documento. No se conserva: las capturas y los registros que se citan
  vivieron ahí durante la etapa.
- El clúster PostgreSQL 18.6 temporal en 55499, solo en loopback, con una base `adel_pos_test_*` desechable por paquete,
  por motor de Playwright y por Go temporal.
- Para Android, un **Go temporal en modo de pruebas** por corrida, construido desde el mismo árbol que se probaba. Tiene
  la escucha local en `127.0.0.1` y la escucha de red también en loopback, que el emulador alcanza como `10.0.2.2`.
- Los emuladores, **sin ventana y de solo lectura**, como mucho dos a la vez, siempre con `ANDROID_SERIAL` y con tope de
  tiempo en cada `adb`. Durante las fases se usaron `Pixel_Tablet` (Android 17) y `ADEL_A1_Phone_API33` (Android 13).
  En la regresión final, además, `Pixel_8` (Android 17) y `ADEL_A1_Tablet_API26` (Android 8.0).

Ninguna prueba corrió en un teléfono o una tableta reales, ni en la wifi de un restaurante. Vitest y Playwright
sustituyen el puente nativo del escritorio por la simulación de `e2e/native-driver.ts`, como en las etapas anteriores.

## Fase 0 · verificación

Un solo agente, sin cambios en el repositorio; el detalle está en el avance, §0.1–§0.7. Encontró la causa de los «Sin
conexión» falsos:

- Go cierra una conexión keep-alive libre a los **60 s** exactos.
- OkHttp 5.5.1 guarda las conexiones libres hasta 5 minutos y, para un `GET`, no hace la revisión extensa antes de
  reutilizar una.
- La aplicación tenía `retryOnConnectionFailure(false)`.
- La revisión periódica de cada 60 s caía siempre sobre la conexión ya cerrada. Una sola falla mostraba el aviso,
  mientras la barra solo miraba la red de Android.

Lo reprodujo con `IdleProbeOnGoTest`, fuera del repositorio, contra un Go temporal:

- **Sin arreglo**: 12 y 13 fallas en 13 minutos (todas `GET version` sobre una conexión libre de 60 s o más) y el aviso
  falso en 4 de 5 bloqueos.
- **Con el pool de 30 s**: 0 fallas y 0 avisos en 14 minutos.

La Fase 0 también propuso los grupos (cada permiso en un solo grupo de 10), las 138 dependencias, las 11 capacidades y
los conjuntos de fábrica cerrados bajo esas dependencias. El coordinador tomó las decisiones A3-D1 a A3-D11; ninguna fue
una parada.

## Resultados ejecutados por fase

Qué significa cada término en la tabla:

- **JVM**: `:core:domain:test testDebugUnitTest` (JVM y Robolectric SDK 36, contadas en los XML), con `ktfmtCheck` y
  `lintDebug` aprobados.
- **Instrumentadas**: `connectedDebugAndroidTest` con las aplicaciones y sus `.test` desinstaladas antes, y los XML
  contados por módulo. Las de punta a punta salen omitidas sin los argumentos del guion.
- **De punta a punta**: `scripts/android-e2e.sh` contra un Go temporal nuevo.

En cada fase se corrieron además `format`, `fmt-check` y `sync-check`, y `docs/etapas` quedó intacto por md5
(`3555c65b1300ce044695adcc1ec19f0d`, el md5 de `md5 -q docs/etapas/*.md`, antes y después).

| Fase o parte (fusión)                                   | Servidor y escritorio                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | Android                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| A, escritorio y Go (`ac50dc9`)                          | `TestPlainLanguageMessages` falló con las 14 cadenas antes de reescribirlas (comprueba que detecta); después, `go-unit` (35 paquetes); `go-int ./internal/operations` (`TestRestaurantAcceptance`); Vitest 97 archivos, 926 pruebas; la comprobación del escritorio falló con frases plantadas en TypeScript y Rust; `cargo test` 107 y 3 ignoradas; Playwright 3 de 3 en cada motor (las que buscan textos cambiados)                                                                                  | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| D (`699cb3b`)                                           | `go-int` de `database` (`TestStageA3Upgrade`, de 26 a 27), `apicontract` (`TestContractV1` con `discardAccount`) y `administration` (`TestStageA1WaiterPIN` falló por tiempo con la carga en 17–55 y pasó sola); `go-unit`; Vitest 98 archivos, 932 pruebas (`stageA3Terminals.test.tsx`); `cargo test` 107; Playwright `workflow-stagea2-terminals` ampliada 1 de 1 y las de Terminales 5 de 5, en cada motor                                                                                          | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| A, Android (`8da5ad3`)                                  | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | JVM **369** (`ServerLinkTrackerTest` 14 nuevas, `PinnedHttpClientTest` 6 nuevas, `PlainLanguageTest`); instrumentadas `app` 106 (37 corridas, 0 fallas) y `core:data` 20 en `Pixel_Tablet` y `ADEL_A1_Phone_API33`; **`--suite connection` de 10 minutos en `Pixel_Tablet`: 0 de 557 s con «Sin conexión», 0 sin respuesta, 0 repeticiones**                                                                                                                 |
| B (`215fd81`), con la revisión y sus correcciones       | Pruebas unitarias del catálogo (cada permiso en un solo grupo, sin ciclos, conjuntos cerrados, Mesero dentro de `WaiterPermissions`); `TestStageA3FactoryRoles` (9 casos) y `TestStageA3FactoryRolesReview` (5, del revisor); `go-int` de `administration` (255 s), `apicontract` y `operations` (`TestRestaurantAcceptance`); Vitest 99 archivos, 943 pruebas (más las del revisor); `cargo test` 108 y 3 ignoradas; Playwright `workflow-stagea1-pin` y `workflow-stagea3-roles` 2 de 2 en cada motor | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| C (`beeb183`)                                           | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | JVM **381**; instrumentadas `app` 119 (44 corridas, 0 fallas; `StageA3ScreensTest` 7/7, `DeviceLayoutTest` 2/2), `core:ui` 14/14 y `core:data` 18 + 2 omitidas, en los dos emuladores; `--suite adjustments` (`a1`–`a6`) 6/6 en cada uno; 44 capturas                                                                                                                                                                                                        |
| Correcciones del escritorio de la regresión (`85be71e`) | Vitest 103 archivos, 963 pruebas (`stageA3Sessions`, `auditActions`, `unknownValues`); `build`; Playwright 2 de 2 (`workflow-stage9d-cancel-account` seguida de `workflow-stagea3-roles`) y 6 de 6 (las pantallas con nombres de operaciones y estados) en cada motor                                                                                                                                                                                                                                   | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Correcciones de Android de la regresión (`9038b79`)     | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | JVM **386** (`CallAwaitTest`, `PinPadSizingTest`); instrumentadas `app` 121 (45 corridas, 0 fallas; `PinOrientationTest` 1/1), `core:ui` 14/14 y `core:data` 18 + 2, en los dos emuladores; `precheck` 7/7, `access` (1–5 y 7) 6/6 y `adjustments` 7/7 en los dos; `alerts` 11/11 en los dos (en el teléfono, después de una corrida sin red del emulador, abajo); `waiter` 19/19 en el teléfono                                                             |
| Correcciones B1 y B2 (`be46a07`)                        | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | JVM **393** (`ReadyShownNoticesTest` 5, `AccessRepositoryTest` con dos casos nuevos, el primero falla sin la corrección); instrumentadas `app` 122 (46 corridas, 0 fallas; `ReadyAlertsOnDeviceTest` con una lista de Android que tarda, falla con la decisión anterior) en los dos emuladores; `alerts` 11/11 tres veces en el teléfono y una en la tableta; `access` (1–5 y 6–7 con la segunda identidad) 7/7 dos veces en la tableta y una en el teléfono |
| Punta de la etapa                                       | «Regresión final» (abajo)                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | «Regresión final» (abajo)                                                                                                                                                                                                                                                                                                                                                                                                                                    |

**Notas de las fases.**

- `TestStageA1WaiterPIN` falló por tiempo en la Fase D, con la Mac muy cargada por los emuladores del otro agente; pasó
  sola y aprobó a la primera en la regresión.
- En la Fase C, una vuelta de las instrumentadas en la tableta falló porque la aplicación que dejó la prueba de punta a
  punta no se desinstaló antes. Desinstalada, pasaron.
- En las correcciones de Android, `alerts` falló completa en el teléfono (0/11) porque el emulador se quedó sin red unos
  6 minutos: los DNS no respondían y ninguna petición del dispositivo llegó a Go. Repetida con un Go nuevo: 11/11.

## Revisión independiente (§0: roles de fábrica y «Restablecer permisos predeterminados»)

Un agente aparte revisó `git diff 7e9a957..2915e2b` (la Fase B) sin corregir el código. Escribió pruebas que demuestran
cada hallazgo (`c17cb74` y `7c40f41`, tomadas por el autor). Hallazgos y correcciones:

| Hallazgo                                                                                                                   | Gravedad | Corrección (decisión)                                                                                    |
| -------------------------------------------------------------------------------------------------------------------------- | -------- | -------------------------------------------------------------------------------------------------------- |
| R1 · Un Supervisor de fábrica podía cerrar la sesión del Administrador (`sessions/{id}/revoke` sin la regla de delegación) | Alta     | La misma regla que la edición de usuarios: solo la sesión de quien puede administrar (A3-D16, `a0ef54e`) |
| R2 · «Restablecer permisos predeterminados» aparecía donde Go lo rechaza (rol propio, permisos que el actor no tiene)      | Media    | El botón se esconde y la pantalla dice por qué (A3-D17)                                                  |
| R3 · Con una búsqueda, «Marcar todo el grupo» actuaba también sobre los permisos ocultos                                   | Media    | Con búsqueda, «Marcar el que se ve» y «Quitar el que se ve» actúan solo sobre lo visible (A3-D18)        |
| R4 · El Supervisor de fábrica traía los horarios del catálogo (`catalog.availability.manage`)                              | Baja     | Se quita: el Supervisor queda con 88 (A3-D19)                                                            |
| R5 · Un `ROLE_CHANGED` al confirmar no volvía a leer el rol                                                                | Baja     | Lo dice y carga lo de ahora (A3-D20)                                                                     |

El revisor comprobó además lo siguiente:

- El restablecimiento no permite conceder nada que no se pueda con `PATCH roles/{id}`: rol propio, rol reservado, el
  Administrador, roles sin `code` y el último administrador.
- El orden de los bloqueos y de las comprobaciones es correcto: nada se escribe antes de validar.
- Las dos confirmaciones a la vez y la confirmación contra lo mostrado (`ROLE_CHANGED`) funcionan.
- La auditoría `role.defaults_restored`, con antes y después, va en la misma transacción.
- **Ninguna instalación existente cambia sola**: ni por migración, ni al arrancar, ni por `ensureFoundations`.
- Las operaciones nuevas de `api.rs` son una lista cerrada (`stage_a3_permission_routes_are_closed`).
- Go valida cada permiso en cada ruta, aunque la interfaz lo muestre.

## Cobertura de §6

| §6                                                                                                                                                      | Dónde se prueba                                                                                                                                                                                                                                                                                                                                                                                                                                |
| ------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Regla de textos: comprobación automática                                                                                                                | Escritorio y Rust: `desktop/src/pages/plainLanguage.test.tsx`. Go: `apicontract/plain_language_test.go` (`TestPlainLanguageMessages`). Android: `PlainLanguageTest` (`core:ui`). Cada una se vio fallar con frases plantadas. Ningún código de Go a la vista: `auditActions.test.ts` (los 253 nombres del Registro, leyendo las fuentes de Go), `unknownValues.test.ts` y `stageA3Sessions.test.tsx` (A3-D22)                                  |
| Regla de textos: revisión con capturas                                                                                                                  | Escritorio: Roles, Restablecer, Terminales y Sesiones en tema claro y oscuro, en los dos motores (Fase E, sobre `da899ee` y otra vez sobre `e258fb4`). Android: las 44 de la Fase C, las de las correcciones y las de la regresión final (sección «Android»)                                                                                                                                                                                   |
| Conexión: un fallo aislado no avisa; varios sí; el primer éxito recupera; indicador y avisos coherentes; revisión espaciada con el dispositivo inactivo | `ServerLinkTrackerTest` (JVM, 19 casos: las reglas de 3 fallas y 20 s, sin red de Android al instante, el primer éxito, «dudando» al arrancar, la revisión cada 5 s mientras duda y cada 60 s sin uso, ninguna con el canal abierto); `PinnedHttpClientTest` (el pool de 30 s y la repetición de una lectura); `ServerLinkScreensTest` (la barra y los avisos dicen lo mismo en «Selecciona tu usuario», Mesas y Para llevar); `access` paso 1 |
| Conexión: una hora sin uso sin avisos falsos, con la Mac sin carga                                                                                      | `--suite connection` de 60 minutos (`ConnectionOnGoTest`) en `Pixel_Tablet`, al final de la regresión y sola (sección «Android»); antes, la de 10 minutos de la Fase A                                                                                                                                                                                                                                                                         |
| Roles de fábrica: un mesero nuevo atiende una mesa sin tocar permisos                                                                                   | Go: `A3_mesero_nuevo_con_pin_atiende_una_mesa_sin_tocar_permisos` (por HTTP, con las cabeceras de la aplicación). Aplicación: la prueba desechable `FactoryWaiterOnGoTest` en `Pixel_Tablet` (sección «Android»). Escritorio: `workflow-stagea1-pin` con el rol Mesero de fábrica                                                                                                                                                              |
| Restablecer con vista previa; dependencias automáticas; el servidor rechaza lo que no se tiene; capacidades parciales                                   | `TestStageA3FactoryRoles` y `TestStageA3FactoryRolesReview` (vista previa y auditoría, `ROLE_CHANGED`, sin escalada ni rol propio, solo roles de fábrica, nunca sin administrador, límite de descuento intacto, la ruta valida aunque el rol no cumpla las dependencias); pruebas unitarias del catálogo; `stageA3Roles.test.tsx` y `stageA3RolesReview.test.tsx` (Vitest)                                                                     |
| Pantalla de permisos (Playwright): grupos, contadores, marcar y quitar el grupo, buscador, dependencias, vista sencilla                                 | `workflow-stagea3-roles.spec.ts`, en los dos motores, sola y en la suite completa                                                                                                                                                                                                                                                                                                                                                              |
| Android: cada punto de §4 en teléfono y tableta, vertical y horizontal; las tarjetas en todos los anchos; la pantalla inicial Para llevar               | `StageA3ScreensTest` (las tarjetas en dos renglones en todos los anchos, Descartar, la flecha y el logotipo, el buscador de una línea, la tarjeta sin imagen, «Selecciona tu usuario» e «Ingresar», Mesas compacta con el mapa); `DeviceLayoutTest`; `PinOrientationTest`; `StageA3PanelRulesTest` y `TablePanelAccountsTest` (JVM); `adjustments` `a1`–`a7` contra Go en las dos orientaciones, con `a6` para la pantalla inicial Para llevar |
| Terminales: opciones según el tipo, columna Caja e identificador                                                                                        | `stageA3Terminals.test.tsx` (Vitest); `workflow-stagea2-terminals` ampliada; `PATCH devices/{id}` que rechaza en un `android` otra pantalla inicial; `TestStageA3Upgrade` (la `0027` de 26 a 27 sin cambiar otras filas); capturas de Terminales                                                                                                                                                                                               |
| Regresión completa: servidor, escritorio y Android en los cuatro emuladores                                                                             | «Regresión final» (abajo)                                                                                                                                                                                                                                                                                                                                                                                                                      |

## No verificado

- **`takeaway t6` en Android 8** (C1, A3-D32): se queda como pendiente conocido por decisión del propietario. El nombre
  del cliente editado en línea llega a Go, pero la lista de la tableta no lo mostró en 40 s. Falló dos veces; aprueba en
  Android 13 y 17. La causa no está determinada.
- **La hora sin uso de §6** (A3-D31): no se corrió, por decisión del propietario. La referencia es la de 10 minutos de
  la Fase A, sobre el código de esa fase.
- **`alerts r3` en Android 13**: la causa de sus fallas es probable, no demostrada (A3-D26). Sobre `ca120ee` aprobó
  gracias a la repetición de la ayuda de la prueba (A3-D30); sobre `49961de`, sin ella.
- **`--suite all` de un tirón en Android 8**: se completó por partes, porque la instalación por Gradle se colgaba
  (A3-D33).
- **Lo que no se ejecutó en ningún entorno real**:
  - **Dispositivo físico, wifi real y papel**: ninguna prueba en un teléfono o una tableta reales, en la wifi de un
    restaurante ni en una impresora física.
  - **La aplicación real de Tauri**: Playwright y las capturas usan la vista previa de Vite con el puente simulado.
  - **Windows**: solo `GOOS=windows` para Go, también con `integration` y `legacydocker`.
  - **El lector con Docker real**: Docker Desktop estaba apagado.
- **La instalación real**: la `0027` y la 0.14.0 llegan cuando el propietario instala el `.pkg`.

## Regresión final

### Servidor y escritorio

> **Resumen.** Se corrió en dos partes. **Sobre `da899ee`** aprobó todo el servidor (Go, integración de los 14 paquetes,
> `cargo` y los scripts del instalador), Vitest y `build`. Playwright completo no aprobó por dos hallazgos: la prueba de
> Roles en la suite completa (hallazgo 1) y un código interno a la vista en Personal › Sesiones (hallazgo 2, §1). Los
> dos se corrigieron en `86c9b5f`, `014fe3f`, `403c354` y `d473fed` (fusión `85be71e`). **Sobre `e258fb4`** (la cabeza
> con las correcciones) se repitió lo del escritorio: **45 de 45 en Chromium y 45 de 45 en WebKit, a la primera**,
> Vitest 103 archivos y 963 pruebas, y las capturas con Sesiones ya en palabras del restaurante. Desde `da899ee` no
> cambiaron `backend/`, `config/`, `desktop/src-tauri/` ni los scripts del servidor y del instalador, así que lo de Go,
> integración, `cargo` y el instalador de la primera parte vale para la cabeza.

Una sola vez (§0, «Eficiencia»), sobre una **copia exacta de `da899ee`** (`git archive`, cabeza de `etapa-a3` con las
fases 0, A, B y D fusionadas; `git rev-parse` = `da899ee0041a98f511a7351cdd519bb6d8ba950b`; 1 783 archivos), sin `.git`,
con `.tools`, `.cache/go-mod`, `.cache/playwright` y `desktop/node_modules` enlazados, y con el arnés aislado
(`ISO_TAG=E`): clúster temporal 55499 con una base desechable por paquete, por corrida de Playwright y por prueba con
`-v`; Playwright con su Go temporal en 8183 y la vista previa en 1421, y un segundo Go limpio (`ISO_TAG=EC`, 8187, con
su escucha de red en loopback 8457; uno nuevo por corrida completa) para las cuatro pruebas de «instalación limpia» de
la 9H. Go 1.27.3, Playwright 1.65.0, PostgreSQL 18.6, macOS 26.5.2. 19/11/2026, de 14:42 a 15:48 (hora de Monterrey).
Hasta las 15:12 la Mac estuvo **cargada** (los dos emuladores de la Fase C, `emulator-5580` y `5582`, y Gradle; carga
media de 8 a 38); después, sin emuladores (carga de 2 a 6). **No se cambió código**: la copia quedó idéntica por md5
(los 1 783 archivos) y `md5 -q docs/etapas/*.md | md5` = `3555c65b1300ce044695adcc1ec19f0d` antes y después.

**`./cmd/ad-el-pos-server` se corrió al final de la integración** (15:12): hasta entonces el Go temporal del agente de
Android (`run-AN`) escuchaba en `127.0.0.1:8193`, que es `legacyListen` de `TestServiceHostOnMacOS`
(`host_unix_integration_test.go:46`; su `net.Listen` fallaría y `adopt-legacy` creería que el servidor anterior «sigue
en marcha»). Se corrió en cuanto el puerto quedó libre, con los emuladores apagados.

| Suite                                                                                  | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Tiempo           |
| -------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------- |
| `fmt-check` (comprobación de datos reales, Prettier, `gofmt`, `cargo fmt`)             | Aprobado. La comprobación de datos reales dice «sin .git … no hay nada que comprobar»; corrida además sobre un índice de git propio de `da899ee` (1 783 archivos): «el índice de git no tiene respaldos ni paquetes de migración.»                                                                                                                                                                                                                                                                                                                                                                                     | 22 s             |
| `sync-check`                                                                           | «Metadatos coherentes.» (0.14.0, esquema 27)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | < 1 s            |
| `go-unit`                                                                              | Aprobado: `go vet` (normal e `integration`), `go test -race -count=1 ./...` (**35 paquetes `ok`**, 5 sin pruebas de unidad: `events`, `idempotency`, `product`, `reports/operational` y `migrations`), `GOOS=windows` `go build` y `go vet` (normal e `integration`)                                                                                                                                                                                                                                                                                                                                                   | 76 s             |
| `go vet -tags 'integration legacydocker'`, en macOS y con `GOOS=windows`               | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | 6 s              |
| Integración PostgreSQL, los **14 paquetes** de `scripts/database.py`                   | **14 de 14**: `database` 19 s, `administration` 282 s, `catalog` 26 s, `operations` 486 s, `payments` 3 s, `reports` 157 s, `reports/operational` 51 s, `designer` 25 s, `identity` 24 s, `apicontract` 45 s, `backupschedule` 9 s, `cmd/ad-el-pos-server` 64 s, `discovery` 5 s y `legacyimport` 15 s. Con `TestStageA3FactoryRoles`, `TestStageA3FactoryRolesReview` y `TestStageA1WaiterPIN` (en `administration`, que empezó con la carga más alta del día, 38, y dos emuladores encendidos: **aprobó a la primera**), `TestStageA3Upgrade` y `TestStageA2Upgrade` (`database`) y `TestContractV1` (`apicontract`) | 1 211 s en total |
| `TestOperationalReports` y `TestServiceHostOnMacOS` con `-v`                           | Aprobadas, **ninguna omitida**: `reports/operational` a las 15:06 (fuera de la franja de 00:00 a 05:00), 16 subpruebas; `TestServiceHostOnMacOS` a las 15:14, 58 s                                                                                                                                                                                                                                                                                                                                                                                                                                                     | 47 s y 63 s      |
| Docker real del lector                                                                 | **No se corrió**: Docker Desktop instalado pero apagado (sin el socket `~/.docker/run/docker.sock`); no se encendió                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | —                |
| `vitest` (`tsc --noEmit` + Vitest)                                                     | **100 archivos, 946 pruebas**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | 27 s             |
| `build` (`npm run build`)                                                              | Aprobado (el aviso de tamaño de fragmento de siempre)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | 11 s             |
| `cargo test --locked --offline`                                                        | **108 aprobadas, 3 ignoradas**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | 38 s             |
| `scripts/macos-pkg/test-scripts.sh` (sin root) y `bash -n`                             | «Scripts del instalador: todo correcto» (31 comprobaciones); `bash -n` de `common.sh`, `test-scripts.sh`, `app/preinstall`, `server/preinstall` y `postinstall`, `service/postinstall`, `build-macos-pkg.sh`, `check-legacy-data.sh` y `android-e2e.sh`                                                                                                                                                                                                                                                                                                                                                                | 4 s              |
| **Playwright Chromium** completo (45 pruebas, 35 especificaciones, con el Go limpio)   | **1.ª corrida, 24 de 45**: falla `workflow-stage9f-kitchen-stations` (intermitente anterior a la etapa, abajo) y 20 en cadena. **2.ª corrida, 43 de 45**: falla `workflow-stagea3-roles` (**hallazgo 1**) y `workflow-zz-test-data` en cadena; las 4 de «instalación limpia» con el Go limpio aprobaron, ninguna omitida, y `workflow-stagea2-terminals` esperó el límite de emparejamientos (9,0 minutos) y aprobó entera                                                                                                                                                                                             | 8,6 y 19,7 min   |
| `workflow-stage9f-kitchen-stations` **sola** (base nueva)                              | 1 de 1                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | 32 s             |
| `workflow-stage9d-cancel-account` seguida de `workflow-stagea3-roles` (base nueva)     | **1 de 2**: la de la A3 falla igual que en la suite completa, con la Mac sin carga (2,1): reproducción del hallazgo 1                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | 51 s             |
| `workflow-stagea3-roles` **sola** (base nueva), Chromium y WebKit                      | **1 de 1 y 1 de 1**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | 16 s y 16 s      |
| `workflow-stagea2-terminals` **sola** (base nueva), Chromium y WebKit                  | **1 de 1 y 1 de 1**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | 15 s y 15 s      |
| **Playwright WebKit** completo                                                         | No se corrió sobre `da899ee`: detenida la regresión por el hallazgo 1. Se corrió sobre `e258fb4` con las correcciones (abajo: 45 de 45)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | —                |
| Capturas del escritorio (especificación desechable fuera del repositorio), dos motores | 1 de 1 en Chromium y 1 de 1 en WebKit: 16 vistas en tema claro y oscuro (64 imágenes y 32 archivos con el texto visible), revisadas abajo                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | 33 s y 34 s      |

Pantalla de la Mac **desbloqueada** (`ioreg … CGSSessionScreenIsLocked` sin la clave) en cada comprobación: antes y
después de las dos corridas completas y antes de las capturas y de las especificaciones solas. Los emuladores estaban
apagados a las 15:12 y a las 15:49, y durante Playwright (de 15:15 a 15:48) la carga se mantuvo entre 1 y 4.

**Hallazgo 1 de la regresión: `workflow-stagea3-roles` no aprueba en la suite completa.** Es de la prueba, no del
producto. `workflow-stagea3-roles.spec.ts:76` busca la fila del Mesero con `page.locator('tbody tr').filter({ hasText:
/^Mesero/ })`, pero en la suite completa la base es compartida y antes corre `workflow-stage9d-cancel-account`, que crea
el rol **«Mesero sin cancelar 9D»** (`workflow-stage9d-cancel-account.spec.ts:266`). Las dos filas empiezan con «Mesero» y
el clic de la línea 77 falla en el modo estricto de Playwright: `strict mode violation: … resolved to 2 elements`
(filas 12 y 13 de la tabla de roles). En la Fase B la prueba solo se corrió sola y junto a `workflow-stagea1-pin`, donde
no aparece. Reproducido con solo esas dos especificaciones y la Mac sin carga, así que **no es del entorno**. Como la
prueba falla, Playwright reinicia el proceso y `workflow-zz-test-data` falla en cadena («La aceptación debe comenzar con
una base sin configurar»). **No se corrigió** (el encargo no permite cambiar código). Corrección posible, a decidir por
el coordinador: buscar la fila por la celda exacta del nombre (por ejemplo `filter({ has: page.getByRole('cell', { name:
'Mesero', exact: true }) })`). Después habría que repetir **Playwright completo en los dos motores**. Mis capturas no se
afectan, porque usan una base nueva sin roles propios.

**Intermitente anterior a la etapa: `workflow-stage9f-kitchen-stations`** (1.ª corrida, carga de 2,6 al empezar y de 3,7
al terminar): en su
recorrido de temas, el fondo de la tarjeta «normal» se leyó con el color oscuro (`rgb(25, 35, 46)`) cuando ya se
esperaba el claro (`--panel` = `rgb(255, 255, 255)`), en `workflow-stage9f-kitchen-stations.spec.ts:380`. La prueba
cambia el tema a mano (`dataset.theme`) y la aplicación vuelve a aplicar el tema del usuario cuando se actualiza la
sesión (`App.tsx:313`), y eso puede pasar entre las dos lecturas. Es la misma clase de intermitencia que la de
`workflow-stage9d-colors` que registró la A2 (allí se resolvió con reintentos; esta prueba no los tiene). La etapa no
tocó ni esa especificación ni cómo se aplica el tema (en `App.tsx` solo cambió lo que recibe `RolesPage`). Aprobó sola y
en la 2.ª corrida completa. Sus 20 fallas siguientes son en cadena: las especificaciones posteriores encuentran la base
ya configurada después del reinicio del proceso.

**Revisión de las capturas** (§6: «una revisión de las pantallas principales con capturas»). La hizo una especificación
desechable **fuera del repositorio** (`$SP/e/shots-spec/`, con su propia configuración de Playwright; importa
`native-driver.ts`, `navigation.ts`, `notices.ts` y `pairing.ts` de la copia sin cambiarlos), contra el Go temporal de
la copia con una base nueva y **datos sintéticos**: la mesera «Andrea Capturas» con el rol Mesero de fábrica y su PIN, y
el «Teléfono de Andrea» (`android`, emparejado por la API como lo haría la aplicación) donde ella entró con PIN. El tema
oscuro se guardó en las preferencias del administrador y se aplicó en la página. La ventana es de 1440 px de ancho y
tan alta como la pantalla (con `fullPage`, la barra fija de «Guardar rol» quedaba a media imagen). Las capturas están
en `$SP/shots-E/escritorio/` (`<motor>-<n.º>-<vista>-claro|oscuro.png`, y `<motor>-<n.º>-<vista>.txt` con el texto
visible). Revisé todas las de WebKit (el motor del escritorio en la Mac) y comparé el texto de las de Chromium (es el
mismo, salvo espacios). Busqué en el texto de las 32 vistas «todavía no», «etapa», «próximamente», «provisional»,
identificadores (UUID), claves de permisos, códigos de error y palabras en inglés:

- **Roles** (01–09): la lista con los seis roles de fábrica y el Administrador (156, Cajero 36, Cocina 8, Finanzas 28,
  Inventarios 20, Mesero 14, Supervisor 88; «Origen: Inicial»). La **vista sencilla** del Mesero abre primero: «Atender
  mesas» y «Para llevar» completas y las parciales con «Parcial: 1 de 9» y su casilla indeterminada. La del Supervisor
  tiene cinco completas y seis parciales. La **vista detallada**: los diez grupos con «x de y»; con «Mesas y cuentas»
  abierto, «Marcar todo el grupo», «Quitar todo el grupo» y la descripción corta de cada permiso. Con la **búsqueda**
  «precuenta» queda solo «Caja y cobros» con «Imprimir tickets y precuentas», y los botones dicen «Marcar el que se ve»
  y «Quitar el que se ve» (A3-D18). Al marcar «Cancelar partidas o cantidades confirmadas» aparece «También se marcó lo
  que necesita: «Modificar partidas confirmadas sin aprobación adicional».», y al quitar todo Para llevar, «0 de 4».
  **«Restablecer permisos predeterminados»**: el diálogo «Restablecer permisos de Mesero» lista «Se agregarán (3)» y «Se
  quitarán (2)» con sus nombres, aclara que no cambian el límite de descuento ni los permisos propios de cada usuario y
  que el cambio queda en el Registro de movimientos, y ofrece «Volver» y «Restablecer permisos». Después avisa «Permisos
  predeterminados de Mesero restablecidos.». Sin textos provisionales ni claves de permisos a la vista, en los dos temas.
- **Terminales** (10–14): la columna «Caja» del teléfono dice «—» y ninguna fila muestra «Código» ni el identificador.
  El panel **«Sesiones activas por tipo de terminal»** muestra Escritorio 1, Teléfono o tableta 1 y Cocina 0, sin la
  frase «Solo se cuentan; todavía no se limita ninguna». El **Editar** del teléfono trae nombre, bloqueo por inactividad,
  impresora de precuentas, avisos de platillos listos y «Pantalla inicial» solo con **Mesas | Para llevar**, sin caja
  física ni teclado. El de la terminal de escritorio conserva caja física, teclado y las cuatro pantallas. **«Ver
  configuración»** del teléfono: tipo, sistema, emparejada, pantalla inicial, bloqueo, impresora de precuentas, avisos y
  al final **«Identificador»** con el `id` (el único UUID a la vista, donde lo pide §5.3 y lo decidió la Fase D).
  «Terminales nuevas» dice «Esta computadora no recibe terminales de la red.», que es cierto para el Go temporal (sin
  escucha de red).
- **Personal › Sesiones** (15): **hallazgo 2** (abajo).
- Lo demás a la vista es de las pruebas, no del producto: el nombre de la terminal del driver («WebView temporal»), el
  sistema simulado del teléfono («Android 15 · versión 0.1.0», del emparejamiento de `pairing.ts`), el aviso «Recomendamos
  cambiar tu contraseña inicial» (el administrador de pruebas usa `admin`/`admin`) y los avisos emergentes («Sesión
  iniciada.», «Rol guardado.»).

**Hallazgo 2 (§1, identificador interno a la vista): el «Estado» de Personal › Sesiones.** Una sesión cerrada muestra el
código de Go tal cual: en la captura, **`logout`** (la del administrador que cerró el driver al preparar la base).
`desktop/src/pages/SessionsPage.tsx:57` pinta `{active ? 'Activa' : (session.revoke_reason ?? 'Expirada')}`, y Go guarda
en `app.sessions.revoke_reason` códigos como `logout` (`administration/routes.go:40`), `admin_revoked`
(`sessions_audit.go:48`), `device_revoked` (`devices.go:131`), `expired` (`service.go:469`), `user_disabled`
(`users.go:154`), `support_reset` (`setup.go:177`) y los de cambio de contraseña o de PIN (`auth.go:118`, `pin.go:159`).
Es anterior a la etapa y la comprobación automática de la Fase A no lo ve, porque el texto llega de Go y no está escrito
en el código del escritorio. **No se corrigió** (el encargo no permite cambiar código). Lo decide el coordinador: §1 pide
quitar o reescribir los identificadores internos en ambas aplicaciones; la corrección natural es traducir cada motivo a
una frase («Cerró sesión», «Cerrada por un administrador», «Terminal revocada», «Expirada»…, con una frase general para
uno desconocido) y probarla con Vitest.

**Corrida sobre `e258fb4`** (copia exacta, `git rev-parse` = `e258fb4784a9946a47844a55b5f8a3c90a63849c`, 1 797 archivos,
en `$SP/e/tree3`; mismo arnés, `ISO_TAG=E`, Go en 8183, vista previa en 1421 y Go limpio `ISO_TAG=EC` en 8187; de 17:02
a 17:44, con el agente de Android usando emuladores, carga de 2 a 13). Respecto a `da899ee`, `git diff --name-only da899ee
e258fb4` fuera de `android/` y `docs/` da `CLAUDE.md`, `scripts/android-e2e.sh`, `desktop/e2e/workflow-stagea3-roles.spec.ts`
y 19 archivos de `desktop/src/`. Ninguno de `backend/`, `config/` ni `desktop/src-tauri/`. Las correcciones de los dos
hallazgos:

- **Hallazgo 1**, en `86c9b5f`: la prueba busca la fila del rol por la celda con el nombre exacto.
- **Hallazgo 2**, decisión A3-D22:
  - `014fe3f`: Sesiones dice por qué se cerró cada sesión en palabras del restaurante, con una frase general para un
    motivo desconocido.
  - `403c354` y `d473fed`: lo de la misma clase en el Registro de movimientos, Cuenta y Caja.

Pantalla desbloqueada antes y después de cada corrida de Playwright. La copia quedó idéntica por md5 al terminar.

| Suite                                                                                | Resultado                                                                                                                                                                                                                                                                                                                  | Tiempo   |
| ------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| `fmt-check` (comprobación de datos reales, Prettier, `gofmt`, `cargo fmt`)           | Aprobado. Además, sobre un índice de git propio de `e258fb4` (1 797 archivos): «el índice de git no tiene respaldos ni paquetes de migración.»                                                                                                                                                                             | 16 s     |
| `sync-check`                                                                         | «Metadatos coherentes.»                                                                                                                                                                                                                                                                                                    | < 1 s    |
| `vitest` (`tsc --noEmit` + Vitest)                                                   | **103 archivos, 963 pruebas** (con `stageA3Sessions.test.tsx`, `auditActions.test.ts` y `unknownValues.test.ts`, nuevas)                                                                                                                                                                                                   | 26 s     |
| `build`                                                                              | Aprobado (el aviso de tamaño de fragmento de siempre)                                                                                                                                                                                                                                                                      | 9 s      |
| **Playwright Chromium** completo (45 pruebas, 35 especificaciones, con el Go limpio) | **45 de 45 a la primera**. Las 4 de «instalación limpia» con el Go limpio, ninguna omitida. `workflow-stagea3-roles` y `workflow-zz-test-data` aprobaron dentro de la corrida completa, y `workflow-stagea2-terminals` esperó el límite de emparejamientos (9,0 minutos) y aprobó                                          | 19,7 min |
| **Playwright WebKit** completo, igual                                                | **45 de 45 a la primera** (la de la A2 esperó 8,7 minutos)                                                                                                                                                                                                                                                                 | 19,6 min |
| Capturas (la misma especificación desechable, WebKit)                                | 1 de 1: 16 vistas en claro y oscuro en `$SP/shots-E/escritorio-e258fb4/`. En Personal › Sesiones la sesión cerrada dice **«Cerró su sesión»** (antes `logout`). Sin frases provisionales, códigos ni palabras en inglés en el texto de las 16 vistas; el único UUID sigue siendo el «Identificador» de «Ver configuración» | 34 s     |

`workflow-stage9f-kitchen-stations` aprobó en las dos corridas completas: no hubo que repetir nada.

**No verificado en esta parte:** el lector con Docker real (Docker Desktop apagado);
Windows (solo `GOOS=windows` para Go, también con `integration` y `legacydocker`); la aplicación real de Tauri (las
capturas y Playwright usan la vista previa de Vite con el puente simulado de `e2e/native-driver.ts`); las pantallas fuera
de las de la etapa (las capturas cubren Roles, Terminales y Sesiones; el resto solo por las comprobaciones de Vitest y
por Playwright).

### Android

> **Estado: completa, con dos pendientes por decisión del propietario** (20/11/2026): `takeaway t6` en Android 8
> (hallazgo C1, A3-D32) y la hora sin uso (A3-D31).
>
> - **Aprobado sobre `49961de`**:
>   - JVM, Lint y formato.
>   - Instrumentadas en los cuatro emuladores (156 pruebas cada uno, 0 fallas).
>   - `access` (1–5 y 6–7) y `alerts` en `Pixel_Tablet` y `ADEL_A1_Phone_API33`.
>   - `--suite all` completa en `Pixel_8` (76/76).
>   - En `ADEL_A1_Tablet_API26`, todas las partes de `all` menos un paso.
>   - El mesero de fábrica, las capturas y el APK 0.3.0.
> - **Falla que se repitió**: `takeaway t6` en Android 8 (el nombre del cliente editado en línea llega a Go pero no se ve
>   en la lista en 40 s; hallazgo C1).
> - **Problema del entorno**: en Android 8, la instalación de la aplicación por Gradle se colgó en 4 de 9 partes (sección
>   «Android 8: la instalación por Gradle»).
> - **No corrida**: la hora sin uso, por decisión del propietario (20/11/2026).
> - **Ninguna línea «se repite»** en ninguna corrida sobre `49961de`.

**Antecedentes** (las dos corridas anteriores, en `Pixel_Tablet` y `ADEL_A1_Phone_API33`):

- **Sobre `3403598`** (19/11, 15:53–16:58): JVM 381/0; instrumentadas 153 por dispositivo, 0 fallas; `--suite all`
  68/75 y 65/75.
  - **A1**: `precheck p1` esperaba que sin permiso no hubiera botón; desde la Fase C se muestra deshabilitado con el
    motivo (§4.9). `p2`–`p7` fallaban en cadena.
  - **A2**: `alerts r3` en Android 13, `Connection reset` en el `auth/login` de la ayuda de la prueba.
  - Corregido en `9038b79`: `p1`; `showOrder` (`w6`); la repetición de `GoTestServer` (A3-D26); el teclado del PIN en
    horizontal (A3-D24); `Call.awaitResponse` (A3-D25).
- **Sobre `ca120ee`** (19/11, 19:47–20:42): JVM 386/0; instrumentadas 155 por dispositivo, 0 fallas; `--suite all`
  74/76 en la tableta y 75/76 en el teléfono.
  - **B1**: `access` paso 6 en la tableta, dos veces y de dos formas; el 7 en cadena.
  - **B2**: `alerts r5` en Android 13: no se publicaba el resumen del grupo.
  - `alerts r3` aprobó en Android 13 **dos veces gracias a la repetición** de `GoTestServer`: «se repite (1)», un
    `auth/login` sin respuesta, con el patrón de A3-D26. Se aceptó así: A3-D30.
  - Corregido en `be46a07`:
    - B2: `ReadyShownNotices` decide el resumen con lo recién publicado; con una sola cuenta el resumen ya no se quita
      (A3-D28).
    - B1: la sonda de «Sin conexión» informa `ServerIdentityChanged` y el dispositivo pide emparejar de nuevo; el paso 6
      espera el diálogo (A3-D29).

**Corrida sobre `49961de`** (copia exacta, `git rev-parse` = `49961de8849112ce16349805d2fcf5b4a3148533`, 1 805
archivos, en `$SP/e/tree5`; una segunda copia idéntica, `$SP/e/tree5b`, para el segundo emulador; las dos idénticas por
md5 al terminar).

- **Qué cambió**: `git diff --name-only ca120ee 49961de` fuera de `android/` da solo el avance. En `android/`, el código
  sin interfaz de `app` (`ReadyAlertNotifications`), `core:domain` (`ReadyShownNotices`) y `core:data`
  (`AccessRepository`), y sus pruebas.
- **Go temporales**: los del arnés, construidos desde la copia (0.14.0, esquema 27, `test_mode: true`), uno nuevo por
  parte: `ISO_TAG=E` (8195/8455) y `EB` (8199/8459). El registro de Go de cada parte se guardó (`$SP/e/go-logs/`), y el
  registro completo (`logcat -b all`) del teléfono y de la tableta durante `access` y `alerts`.
- **Emuladores**: sin ventana y de solo lectura, dos a la vez como mucho, con `ANDROID_SERIAL` y tope de tiempo en
  `adb`. Ninguna lista mostró un servidor real.
- **Cuándo y con qué carga**: 19/11/2026, de 21:57, al 20/11, a las 00:06, siendo el único agente; carga de 2 a 19 en
  las compilaciones y de 2 a 12 en las corridas.
- **Los procesos de Java 9029 y 9123** (el daemon de Gradle de `~/.gradle` y el de Kotlin, ociosos desde hacía unas 11
  horas):
  - **Ya no están, y no los detuve a mano.** Mis compilaciones usan el mismo JBR y `~/.gradle`, así que Gradle
    **reutilizó** ese daemon ocioso.
  - Cuando dos compilaciones mías coincidieron, Gradle arrancó otro daemon compatible. A las 22:52:15 su propia política
    de expiración detuvo el 9029: «Daemon will be stopped at the end of the build other compatible daemons were started»,
    en `~/.gradle/daemon/9.9.1/daemon-9029.out.log`. El de Kotlin se fue con él.
  - El servicio real (PID 99054) siguió intacto todo el tiempo.

| Qué (en la Mac, una vez)                                                                               | Resultado                                                                                                                                                                                                                                                           | Tiempo |
| ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| `:core:domain:test testDebugUnitTest lintDebug ktfmtCheck` (`--no-build-cache`, copia recién extraída) | **393 pruebas, 0 fallas** (57 XML): `core:domain` 177 (con `ReadyShownNoticesTest`), `core:data` 183, `core:ui` 15, `feature:access` 7, `feature:takeaway` 5, `feature:waiter` 6. Lint en los 7 módulos: 0 errores, 0 avisos (3 sugerencias). `ktfmtCheck` aprobado | 51 s   |
| `assembleDebug assembleRelease` (firmado) y las tres `assembleDebugAndroidTest`                        | `app-debug.apk` 15 701 950 bytes; `app-release.apk` 2 672 443 (firmado, en «Entregables»); `app-debug-androidTest.apk` 1 389 948; `data-debug-androidTest.apk` 5 700 590; `ui-debug-androidTest.apk` 13 178 754                                                     | 36 s   |

Por emulador, con la aplicación, sus pruebas y las de la 0.1.0 desinstaladas antes y después (`pm list packages`: 0
antes de cada corrida): instrumentadas, contando los XML por módulo, y las suites, con un Go nuevo para cada parte. En
`Pixel_Tablet` y `ADEL_A1_Phone_API33`, como pidió el coordinador, solo lo que tocan las correcciones: `access` 1–5 y
6–7 con otra identidad, como en `all`, y `alerts`. El resto de `all` vale de `ca120ee`: tableta y teléfono con 19/19 en
`waiter`, 13/13 en `field`, 9/9 en `takeaway`, 7/7 en `precheck` y 7/7 en `adjustments`, y 3/3 en `catalog`.

| Prueba                                                | `Pixel_Tablet` (Android 17)                                        | `ADEL_A1_Phone_API33` (Android 13)                                 | `Pixel_8` (Android 17)                                             | `ADEL_A1_Tablet_API26` (Android 8.0)                                                                                         |
| ----------------------------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------ | ------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------- |
| Instrumentadas (XML): `app` / `core:data` / `core:ui` | **156, 0 fallas**: 122 / 20 / 14; 78 corridas, 78 omitidas (332 s) | **156, 0 fallas**: 122 / 20 / 14; 78 corridas, 78 omitidas (260 s) | **156, 0 fallas**: 122 / 20 / 14; 78 corridas, 78 omitidas (271 s) | **156, 0 fallas**: 122 / 20 / 14; 76 corridas, 80 omitidas (las dos de más necesitan Android 11+ para cortar la red) (188 s) |
| access (1–5 y 6–7) / catalog / waiter / field         | **5/5 + 2/2**, —, —, —                                             | **5/5 + 2/2**, —, —, —                                             | 7/7, 3/3, 19/19, 13/13                                             | 7/7, 3/3, 19/19, 13/13                                                                                                       |
| takeaway / precheck                                   | —                                                                  | —                                                                  | 9/9, 7/7                                                           | **8/9** (`t6`; repetida: 8/9, `t6` otra vez: hallazgo C1), 7/7                                                               |
| alerts / adjustments                                  | **11/11**, —                                                       | **11/11** (`r5` aprobado), —                                       | 11/11, 7/7 (con `a7`)                                              | 11/11, 7/7 (con `a7`)                                                                                                        |
| Total                                                 | 18/18 (686 s, `access` y `alerts`)                                 | 18/18 (666 s)                                                      | **`--suite all` 76/76** (1 578 s)                                  | **75/76**, en partes (abajo)                                                                                                 |

**Las líneas «se repite» (A3-D26).** Las busqué en la salida y en el registro de cada paso de todas las corridas sobre
`49961de` (`access`, `alerts`, `all`, las capturas y el mesero de fábrica, en los cuatro emuladores): **ninguna**. `r3`
aprobó en Android 13 sin usar la repetición. Lo de `ca120ee` queda como A3-D30.

**B1 y B2 sobre `49961de`.**

- **`access` paso 6** aprobó en los dos. En la tableta volvió a pasar por «Sin conexión» al arrancar y «En línea»
  0,37 s después: el camino que el avance describe y que `AccessRepositoryTest` comprueba de forma determinista.
- **`alerts r5`** aprobó en Android 13 (y en los otros tres).
- La instrumentada nueva **`elResumenNoEsperaAQueAndroidListeLoRecienPublicado`** aprobó en los cuatro
  (`ReadyAlertsOnDeviceTest` 7/7).

**Android 8: cómo se completó `all` (y la instalación por Gradle).** El guion reinstala la aplicación con Gradle
(`:app:installDebug :app:installDebugAndroidTest`) al empezar cada parte. En `ADEL_A1_Tablet_API26` esa instalación se
colgó en 4 de 9 partes. Cada vez fue `ShellCommandUnresponsiveException` de `ddmlib`, «Unable to install» del APK, a
los 4,5 minutos de cada uno. El guion se detiene ahí, sin correr pasos.

| Intento                                                                            | Partes                                                                                      |
| ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `--suite all` (868 s)                                                              | access 7/7, catalog 3/3, waiter 19/19; se colgó la instalación de `field`                   |
| `--suite all` repetida (1 139 s)                                                   | access 7/7, catalog 3/3, waiter 19/19, field 13/13; se colgó la instalación de `takeaway`   |
| Reiniciado el emulador y el servidor de `adb` (remedio de `android/CLAUDE.md`)     | `precheck` 7/7 (88 s) y `adjustments` 7/7 (135 s); se colgaron las de `takeaway` y `alerts` |
| Instalación con `adb install`, igual que `install_app` del guion, y `--no-install` | `alerts` **11/11** (583 s); `takeaway` **8/9** (713 s) y repetida **8/9** (144 s): `t6`     |

- **Instalada a mano**, la aplicación tarda **2 s** (`adb install -r -t`; 0 s el APK de pruebas). La aplicación y
  Android 8 la instalan sin problema: lo que se cuelga es la instalación por Gradle (`ddmlib`).
- En `Pixel_8` y en las corridas de la tableta y el teléfono no pasó.
- Es del entorno, no de la aplicación. El paso de `adb install` hace exactamente lo de `install_app` (desinstala la
  aplicación, sus pruebas y las de la 0.1.0, e instala la aplicación y sus pruebas). **No se cambió el guion**: es una
  orden aparte, `$SP/e/bin/manual-install-suite.sh`.
- Así, en Android 8 cada paso de `all` corrió al menos una vez con su Go nuevo: 75 de 76 aprobados.

**Hallazgo C1 · `takeaway t6` en Android 8 (falló dos veces; en los otros tres emuladores aprobó).**
`t6_nombreYTelefonoDespuesEnLinea`:

- **Lo que hace el paso**: edita en línea el cliente de un pedido para llevar a «Luisa Martínez» y sin teléfono.
- **Lo que pasa**: espera a que Go lo tenga (eso pasó, las dos veces) y a que la lista lo muestre
  (`waitForText("Luisa Martínez")`, `TakeawayOnGoTest.kt:306`). **No se vio en 40 s.** El registro del paso no trae el
  árbol de la pantalla (esa espera no lo imprime) ni ninguna advertencia de la aplicación.
- **Lo que se sabe**: en Android 8 el resto de `takeaway` aprobó, también `t4`–`t5`, la edición sin conexión que sale
  al volver la red. En la A2, `takeaway` aprobó en este emulador; en la A3 es la primera vez que se corre en Android 8.
- **Causa no determinada**: la lista no se puso al día con el nombre que ya estaba en Go, o el texto no se encontró en
  esa pantalla. El propietario decidió cerrar la etapa sin investigarlo (A3-D32).

**Rendimiento** (`field f2`, registro `AdelOnGo`):

| Medida                                                 | `ADEL_A1_Tablet_API26` | `ADEL_A1_Phone_API33` (`ca120ee`) | `Pixel_Tablet` (`ca120ee`) | `Pixel_8` |
| ------------------------------------------------------ | ---------------------- | --------------------------------- | -------------------------- | --------- |
| Arranque en frío (proceso → lista de meseros)          | 1 410 ms               | 1 957 ms                          | 2 476 ms                   | 3 024 ms  |
| PIN (con conexión) → Mesas con la primera mesa         | 797 ms                 | 1 291 ms                          | 985 ms                     | 973 ms    |
| PIN → la mesa 40 a la vista                            | 1 312 ms               | 1 943 ms                          | 1 949 ms                   | 1 623 ms  |
| «Nueva cuenta» → catálogo de ~300 productos a la vista | 266 ms                 | 350 ms                            | 480 ms                     | 338 ms    |
| «Enviar a cocina» → «Recibido por el restaurante»      | 324 ms                 | 323 ms                            | 325 ms                     | 323 ms    |
| Memoria (total de `dumpsys meminfo`)                   | ≈ 91 MiB               | ≈ 147 MiB                         | ≈ 144 MiB                  | ≈ 147 MiB |

**Mesero de fábrica (§6, «un mesero nuevo puede atender una mesa sin tocar permisos»): aprobado en `Pixel_Tablet`
(33 s).** Es una prueba desechable fuera del repositorio: `$SP/e/factory-waiter/`, `git archive 49961de` más
`FactoryWaiterOnGoTest.kt` y una suite `factory` solo en la copia del guion; `tree5` no se tocó. El paso
`fw1_meseroDeFabricaAtiendeUnaMesaSinTocarPermisos`, contra un Go temporal 0.14.0 recién instalado (asistente del
guion):

1. **Tal como quedó tras el asistente**: el rol Mesero de fábrica (`code` = `waiter`), con 14 permisos (`accounts.open`,
   `accounts.read`, `catalog.prices.read`, `catalog.read`, `events.read`, `orders.capture`, `orders.confirm`,
   `payments.tickets.print`, `restaurant.areas.read`, `restaurant.tables.read`, `takeaway.deliver`, `takeaway.manage`,
   `takeaway.read` y `waiter_devices.use`). Su vista previa de «Restablecer» no agrega ni quita nada.
2. **Lo que hace el administrador**: una mesera nueva («Rosa Fábrica») con ese rol y su PIN, una mesa («Mesa 4») y un
   producto. Ningún `PATCH` al rol.
3. **En la aplicación**: se empareja por la dirección, Rosa entra con su PIN, abre la mesa, captura y envía a cocina.
4. **En Go**: el pedido recibido, una partida en la cuenta y nada en borrador.
5. **Al final**: el rol tiene los mismos 14 permisos y la vista previa sigue sin cambios.

Capturas en `$SP/shots-E/android/Pixel_Tablet/mesero-de-fabrica-49961de/`.

**Capturas (§6).** Están en `$SP/shots-E/android/<dispositivo>/<tema>-<commit>/`, de las suites `adjustments` (con
`a7`), `takeaway` (`t9`) y `field` (`f10`, en tema oscuro). El tema oscuro se hizo con `cmd uimode night yes` en el
emulador de solo lectura: `adjustments` y `takeaway` otra vez, cada una con un Go nuevo (7/7 y 9/9 en los dos), y
después «Night mode: no» comprobado. La interfaz no cambió desde `ca120ee`, así que las de tema claro de la tableta son
de esa corrida.

| Dispositivo    | Claro                                                   | Oscuro           | Extra                               |
| -------------- | ------------------------------------------------------- | ---------------- | ----------------------------------- |
| `Pixel_Tablet` | 39, de `ca120ee` (las 11 de `field` son en tema oscuro) | 28, de `49961de` | 4 del mesero de fábrica (`49961de`) |
| `Pixel_8`      | 39, de `49961de` (las 11 de `field` son en tema oscuro) | 28, de `49961de` | —                                   |

Revisé las 138 en hojas de contacto: «Selecciona tu usuario», la entrada con el PIN (también en horizontal: con el
teclado completo en dos columnas, A3-D24), Mesas en lista y en mapa, el panel de la mesa con sus cuentas, la captura,
«Ver la cuenta · 1 por enviar», Pendientes, Para llevar, «Vuelve a escribir tu PIN», «Descartar cuenta vacía» y
«Imprimir precuenta» desde el panel, en vertical y horizontal y en los dos temas. **Ningún texto provisional,
identificador interno ni texto encimado.**

Observaciones ya aceptadas (A3-D27 y las de la Fase C), que siguen igual:

- La pestaña de área siguiente, cortada en la tableta vertical («Terraz…»).
- El contraste de las teclas en «Vuelve a escribir tu PIN» (en tema claro).
- El aviso de 4 s «Lo capturado…» encima de la ayuda del panel.
- En el teléfono, «Mesa A3» del mapa se alcanza desplazando (lado mínimo de 64 dp). Lo mismo en el mapa de 40 mesas de
  `f10`, cortado al borde.

Observaciones nuevas, aceptadas como observaciones (A3-D33):

- **Tableta vertical**: en el panel de la mesa, la etiqueta de una cuenta enviada se corta con puntos suspensivos
  («Enviado · sin confirmar en coci…»). En horizontal y en el teléfono se ve completa.
- **`Pixel_8` vertical**: en la captura de un pedido para llevar nuevo (`t9`), el título de la barra se corta
  («Nuevo pe…» por «Nuevo pedido para llevar») junto al mesero y «En línea». En horizontal se ve completo.
- **Mesero de fábrica, justo después de «Enviar a cocina»**: el panel todavía mostraba la cuenta en «$0.00 · Borrador» con
  «Tienes una captura de esta cuenta en este dispositivo…», aunque Go ya tenía la partida. No medí cuánto tarda en
  ponerse al día.
- **Para llevar como pantalla de inicio** (`a6`): solo en la orientación natural de cada dispositivo; la captura de un
  pedido para llevar sí está en las dos (`t9`).

**La hora sin uso (§6): no corrida por decisión del propietario (20/11/2026).** No se presenta como aprobada.

- **Referencia**: la corrida de **10 minutos** de la Fase A, la misma suite (`ConnectionOnGoTest`,
  `ADELPOS_E2E_CONNECTION_MINUTES=10`). Fue en `Pixel_Tablet` sola, con la Mac sin otra carga y los avisos de platillos
  listos encendidos, y dio «0 de 557 s con «Sin conexión» a la vista; sin respuesta 0, lecturas repetidas 0» (avance,
  «Fase A · Android»).
- **No es la de 60 minutos de §6**, y fue sobre el código de la Fase A. Después cambiaron:
  - el estado al arrancar (Fase C);
  - `Call.awaitResponse` (A3-D25);
  - la sonda ante otro certificado (A3-D29).

**No verificado en esta parte:**

- `takeaway t6` en Android 8 (C1).
- La hora sin uso (no corrida por decisión del propietario).
- `--suite all` de un tirón en Android 8: se completó por partes por la instalación de Gradle.
- En `Pixel_Tablet` y Android 13, las partes de `all` que las correcciones no tocan, que valen de `ca120ee`.
- Un dispositivo físico.

## Entregables

| Archivo                                                                  | Bytes      | SHA-256                                                            | Qué es                                                                                                                                                                                                                                                                                                                                                            |
| ------------------------------------------------------------------------ | ---------- | ------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `android/app/build/outputs/apk/release/AD-EL-POS-Android-0.3.0.apk`      | 2 672 443  | `87477497eab3afa02ee53bea21ec8d81dd224f4c948aa4d681b44f52c9e9ac4d` | `com.adelpos.meseros` 0.3.0 (`versionCode` 3), `minSdk` 26, `targetSdk` 36, `compileSdk` 37, compilado sobre una copia exacta de `49961de` y firmado con la llave de desarrollo: `apksigner verify` (esquema v2), un firmante, «CN=AD-EL POS desarrollo - APK de prueba, O=AD-EL POS», SHA-256 `fb6a4bac93052ca9051342ea122ed90469340a3685de14243bdab288d71c7c86` |
| `android/app/build/outputs/apk/release/AD-EL-POS-Android-0.2.0.apk`      | 2 589 899  | `38d687ce61e8053b82680a4399ba7b2bb0a8da201372f2559ab636b505e6b824` | El de la A2, **sin tocar** (comprobado antes de copiar y al terminar)                                                                                                                                                                                                                                                                                             |
| `desktop/src-tauri/target/release/bundle/macos-pkg/AD-EL POS_0.14.0.pkg` | 23 141 036 | `d9d02a6eda49d65c1ce161790b66ce06c21f87608880a554a5bb2f115831a205` | Aplicación y servidor 0.14.0 (esquema 27), compilado sobre una copia exacta de `e258fb4`; firma ad hoc de la aplicación verificada                                                                                                                                                                                                                                |

**El APK 0.3.0** reemplazó al anterior del mismo nombre (el de `ca120ee`, SHA-256 `46b3137e…ea0d`, del mismo tamaño):
`app/src/main` y `core:data` cambiaron en `be46a07`. También está en `$SP/AD-EL-POS-Android-0.3.0.apk`.

**El `.pkg` 0.14.0 no se volvió a hacer.** Se construyó sobre `e258fb4`, y de `e258fb4` a `49961de` solo cambiaron
`android/`, `scripts/android-e2e.sh` (que no entra en el instalador) y el avance (`git diff --name-only e258fb4
49961de`). Nada de `backend/`, `desktop/`, `config/`, `ops/` ni `scripts/macos-pkg/`, así que el paquete es el mismo
que saldría de la cabeza. Se construyó con `./scripts/dev.sh build-macos-pkg` en la copia, con
`CARGO_NET_OFFLINE=true`, en 119 s, sin instalar nada (ni `sudo`, ni `installer`, ni `launchctl`). Comprobaciones:

- **`pkgutil --expand-full`**:
  - Los tres paquetes son 0.14.0: `local.adel.pos.app`, `.server` y `.service`.
  - La distribución ofrece «Servidor del restaurante» marcada por omisión y el servicio oculto.
  - La aplicación de dentro es 0.14.0, arm64.
  - El servidor de dentro dice `0.14.0` con `version`, y lleva `"schema_version": 27` y la migración
    `0027_android_a3_adjustments.sql`.
  - Los scripts son idénticos a los de la copia.
- **`codesign --verify --deep --strict --verbose=2`** de la aplicación de dentro: «valid on disk» y «satisfies its
  Designated Requirement», firma `adhoc`. El `.pkg` mismo no tiene firma de desarrollador, como dice el script.
- **Copias**: junto a los de 0.10.0 a 0.13.0, que siguen ahí (el de la 0.13.0 con su SHA-256 `8e86573a…0b0f`), y en
  `$SP/AD-EL POS_0.14.0.pkg`.

Instalarlo es la parada del propietario.

## Instalación real

La instalación real de la Mac sigue en **0.13.0, esquema 26**: el servicio `local.adel.pos.server`, con PostgreSQL en
55432 y Go en 8181 y 8443. Ninguna prueba de la etapa la tocó.

Se actualiza con el `.pkg` 0.14.0, que instala el **propietario** con «Servidor del restaurante» marcada. Es una parada,
porque pide la contraseña de administrador. Al arrancar, el `host` respalda la base (`…-antes-de-migrar.dump`) y aplica
la `0027`. Esa migración:

- cambia la nota de la plantilla predeterminada solo donde dice exactamente «Formato predeterminado, igual al anterior a
  la etapa 9B»;
- pasa a Mesas la pantalla inicial `kitchen` o `cash` de los `android` y agrega su `CHECK`.

Para la sección «Fase A», el rol que migra debe ser dueño de las tablas. En la instalación real lo es (A3-D12): la base
del servicio se restauró con `--no-owner --role=<usuario de la aplicación>`, y la `0026` ya hizo un `ALTER TABLE` con él.

**Los roles de la instalación real no cambian.** Los conjuntos de fábrica solo se aplican en el asistente de una
instalación nueva y con el botón «Restablecer permisos predeterminados». Para que el Mesero tome los de fábrica, el
propietario lo restablece desde Administración › Roles: antes de confirmar, la vista previa muestra lo que se agrega y
lo que se quita.

**Los teléfonos y tabletas** con la 0.2.0 se actualizan instalando encima el APK 0.3.0 (misma llave y mismo
identificador). No hace falta desemparejarlos ni borrar sus datos.

Conectar un dispositivo físico, si el propietario tiene uno, es la otra verificación que queda fuera de esta etapa.
