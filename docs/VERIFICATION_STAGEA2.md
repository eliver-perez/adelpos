# Verificación · Etapa A2 (Android)

Verificación por fases del **18 de noviembre de 2026**, en la Mac (Apple Silicon, macOS 26.5.2), en ramas locales por
fase que el coordinador fusionó en `etapa-a2-android`, creada desde `origin/main` (0.12.0, esquema 25, con la A1
fusionada; historia del remoto reescrita el 17/11/2026 sin las líneas `Co-Authored-By`); especificación en `1198148`.
Fase 0 sobre `1198148` (solo verificación, fuera del repositorio); Fase A `4091429`, `825d1f1` y `387441c` y parte B
del servidor `cb9b77f` y `5f52e73` (fusión en `f90fd83`); parte C del servidor `cc9bf57`, `8d2ebfd` y `6c2fcdd`; Fase B
`0534dfc` a `8264d46` (fusión en `bfa7d22`); parte D del servidor `b17f56f`, `c988d16` y `a2c2812` (corrección de su
prueba de Playwright en `084f46a`); Fase C `01c53aa` a `60d6228` (fusión en `c2fe821`, avance `394cab6`); Fase D
`ceee5b9` a `80e8ea9` con el protocolo ajustado en `c4de974` y la fusión con la Fase C en `0357705` (fusión en
`7ec8e16`, avance `afd754e`); revisión independiente del aviso, parte 1 (servidor, reproducciones en `82bf1a7`) y
parte 2 (dispositivo, `882a3fa`); correcciones del servidor `70d3165` y `e3a3a88` (fusión en `2bf8183`, avance
`843b786`) y A2-D12 `b91ae66` (fusión en `03f0f8e`; avance `7b92ea2` y `61b8ba1`); correcciones del dispositivo
`0c0bbf4`, `b3f6377`, `6059890` y `27e430c` (fusión en `6898926`, avance `03a90e6`); y la prueba de Playwright de
Terminales al día en `a4bd335` y **`5f0bf3e`** (solo `desktop/e2e/`).

Servidor **0.13.0** (`min_client_versions` sin cambio: `desktop` 0.10.0 y `android` 0.1.0; `min_client_version` y
`min_server_version` siguen en 0.10.0), API v1, migración `0026_android_a2.sql` (esquema **26**, una sola para la etapa,
con la sección **C** —`devices.precheck_printer_id`, solo para `android`— y la sección **D** —`app.ready_notices` con
`alert_key`, `devices.ready_alerts_enabled`, `_sound` y `_vibration` e índices parciales de las sesiones con PIN—; la
parte B no necesitó migración). Aplicación Android **0.2.0** (`com.adelpos.meseros`, `versionCode` 2, `minSdk` 26,
`targetSdk` 36, `compileSdk` 37), base local `AppDatabase` versión 6 (`captures` con la modalidad y el cliente,
`takeaway_customer_edits`, `ready_alerts_state` y `ready_alerts_seen`) y `catalog.db` versión 1, sin dependencias
nuevas. Especificación: [etapas/ETAPA_A2_ANDROID.md](etapas/ETAPA_A2_ANDROID.md). Avance con el detalle de cada fase y
las decisiones A2-D1 a A2-D12: [ETAPA_A2_AVANCE.md](ETAPA_A2_AVANCE.md). Guía del propietario:
[ANDROID_INSTALAR.md](ANDROID_INSTALAR.md); reglas y pruebas de la aplicación: [android/CLAUDE.md](../android/CLAUDE.md).
Evidencia anterior: [etapa A1](VERIFICATION_STAGEA1.md) y las anteriores desde [VERIFICATION.md](VERIFICATION.md). Este
documento registra **solo lo que se ejecutó**.

**Alcance de esta evidencia.** Las cifras de las fases son las que cada agente registró sobre su propia copia, según el
avance. Lo que valida el código completo de la etapa es la «Regresión final», que se hizo en dos partes sobre commits
distintos porque corrieron a la vez: **servidor y escritorio** sobre copias exactas de `61b8ba1` y de `a4bd335`, y
**Android** sobre `03a90e6` (Fase E). `git diff --stat 03a90e6 5f0bf3e` solo toca `desktop/e2e/pairing.ts` y
`desktop/e2e/workflow-stagea2-terminals.spec.ts`; `git diff --stat 61b8ba1 03a90e6` no toca `backend/`, `desktop/`,
`config/` ni los scripts del servidor y del instalador. **PENDIENTE-COORDINADOR · commit final y Playwright**: la
corrida completa de Playwright en los dos motores se repite al terminar la Fase E, con la Mac sin emuladores, sobre la
cabeza de la etapa.

**Las pruebas no tocaron la instalación real**: ni el servicio `local.adel.pos.server` (0.12.0), ni `/Library/Application
Support/AD-EL POS`, ni `/Applications/AD-EL POS.app`, ni `.local` del repositorio, ni el Keychain, ni PostgreSQL en
55432, ni Go en 8181 o en la escucha de red 8443. Ninguna prueba usó `sudo`, `installer` ni `launchctl` (salvo el
`launchctl` simulado de las pruebas de los scripts del instalador), ni las órdenes `dev.sh` de la instalación. Se aplicó
la protección que dejó el incidente de la Fase F de la A1: las pruebas instrumentadas usan `AdelTestRunner`, que busca
el tipo de pruebas `_adelpostest._tcp`; cada vez que se abrió la aplicación normal (reinicio pegajoso, batería,
`MY_PACKAGE_REPLACED`) la wifi del emulador estaba apagada y el dispositivo ya estaba emparejado por la dirección del Go
temporal; **ninguna lista mostró un servidor real**.

**Datos.** Todas las pruebas usan **datos sintéticos** (meseros, catálogos, mesas, pedidos para llevar, impresoras y
terminales creados por cada prueba en un Go temporal con base desechable). La etapa no usó el respaldo del sistema
anterior ni datos del restaurante.

## Entorno aislado

El de la A1 ([VERIFICATION_STAGEA1.md](VERIFICATION_STAGEA1.md), «Entorno aislado»): un árbol de trabajo por fase en el
directorio temporal de la sesión con `.tools`, `node_modules` y las cachés enlazadas; el clúster PostgreSQL 18.6
temporal en 55499 (solo loopback) con una base `adel_pos_test_*` desechable por paquete, por motor de Playwright y por
Go temporal; para Android, un **Go temporal en modo de pruebas** por corrida (escucha local en `127.0.0.1` y escucha de
red también en loopback, que el emulador alcanza como `10.0.2.2`), construido desde el mismo árbol que se probaba; y los
cuatro emuladores, **sin ventana y de solo lectura**, como mucho dos a la vez y siempre con `ANDROID_SERIAL` y con tope
de tiempo en cada `adb`:

| Emulador               | Android      | Para qué                                             | Corte de red                               |
| ---------------------- | ------------ | ---------------------------------------------------- | ------------------------------------------ |
| `ADEL_A1_Tablet_API26` | 8.0 (API 26) | Tableta económica: el mínimo, sin tipos de servicio  | Desde la Mac: `adb root` + `svc wifi/data` |
| `ADEL_A1_Phone_API33`  | 13 (API 33)  | Teléfono de hace 2–3 años, permiso de notificaciones | `cmd connectivity airplane-mode`           |
| `Pixel_8`              | 17 (API 37)  | Teléfono actual                                      | `cmd connectivity airplane-mode`           |
| `Pixel_Tablet`         | 17 (API 37)  | Tableta actual, dos paneles; batería de referencia   | `cmd connectivity airplane-mode`           |

**Pruebas automatizadas en emuladores, no físicas.** Ninguna prueba corrió en un teléfono o una tableta reales, en la
wifi de un restaurante ni contra el servicio real; la pantalla apagada y Doze se forzaron en el emulador
(`input keyevent KEYCODE_SLEEP`, `dumpsys battery unplug`, `dumpsys deviceidle force-idle`), que no suspende la CPU
como un equipo real. Ninguna impresora física: el agente de impresión es el simulado de las pruebas (`print-agent/*`) y
la impresora de red «que no responde» es un puerto de la Mac donde nada escucha. Vitest y Playwright sustituyen el
puente nativo del escritorio por la simulación de `e2e/native-driver.ts`, como en las etapas anteriores.

## Fase 0 · verificación

Un solo agente, sin cambios en el repositorio. Encontró que todo cabía en cambios compatibles del contrato v1, una
migración `0026` (secciones C y D) y permisos nuevos de mesero, y siete decisiones para el coordinador (A2-D1 a A2-D9 en
el avance; ninguna parada). Una prueba de humo fuera del repositorio (servicio en primer plano con un canal SSE) en
`Pixel_8` y `ADEL_A1_Tablet_API26`: con `specialUse` el canal siguió recibiendo con la pantalla apagada y Doze forzado,
tras quitar la tarea de recientes, tras `kill -9` (reinicio pegajoso) y tras reiniciar el emulador, sin pérdidas ni
repeticiones al reconectar; `dataSync` venció a los 60 s con el límite acortado y se rechazó tras reiniciar (de ahí
A2-D8: `specialUse`).

## Resultados ejecutados por fase

«JVM» = `:core:domain:test testDebugUnitTest` (JVM y Robolectric SDK 36, contadas en los XML), con `ktfmtCheck` y
`lintDebug` aprobados; «instrumentadas» = `connectedDebugAndroidTest` con las aplicaciones (nuevas y de la 0.1.0) y sus
`.test` desinstaladas antes y los XML contados por módulo (las de punta a punta salen omitidas sin los argumentos del
guion); «de punta a punta» = `scripts/android-e2e.sh` contra un Go temporal nuevo. En el servidor, integración con
`-tags=integration` y `-race`. En cada fase se corrieron además `fmt-check` y `sync-check` (y `ktfmtCheck` en las de
Android), y `docs/etapas` quedó intacto por md5.

| Fase o parte (commit)                                | Servidor y escritorio                                                                                                                                                                                                                                                                                                                                                                                                                                               | Android                                                                                                                                                                                                                                                                                       |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A, identificador (`387441c`)                         | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                         | JVM 256 (las de la A1); instrumentadas 94, 0 fallas, en `Pixel_8` y `ADEL_A1_Tablet_API26`; `access` 1–5 5/5 (`Pixel_8`) y `waiter` 19/19 (Android 8, segunda corrida: en la primera `w2` se colgó sin causa confirmada), las dos con la 0.1.0 instalada antes, que el guion desinstaló       |
| B, servidor (`5f52e73`)                              | `TestStageA2Takeaway` 5/5; `TestContractV1` con 34 operaciones; `go-int` de `administration` (215 s) y `operations` (381 s); `go-unit` (35 paquetes)                                                                                                                                                                                                                                                                                                                | —                                                                                                                                                                                                                                                                                             |
| C, servidor y escritorio (`6c2fcdd`)                 | `TestStageA2PrecheckPrinter` 5/5 (con los bytes ESC/POS); `TestStageA2Upgrade` (25 → 26); `TestContractV1` con 37; `go-int` de `database`, `administration`, `operations` y `apicontract`; Vitest 97 archivos, 921 pruebas; `cargo test` 107 y 3 ignoradas; Playwright `workflow-stagea2-terminals` 1/1 (Chromium) y con sus vecinas 6/6 (Chromium) y 4/4 (WebKit)                                                                                                  | —                                                                                                                                                                                                                                                                                             |
| B, dispositivo (`8264d46`)                           | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                         | JVM 280; instrumentadas 108, 0 fallas, en `Pixel_Tablet` y `ADEL_A1_Phone_API33`; `takeaway` 8/8 en los dos (en el teléfono, en la segunda corrida: la prueba no cerraba el teclado); `waiter` 19/19 en `ADEL_A1_Phone_API33`                                                                 |
| D, servidor y escritorio (`a2c2812`, `084f46a`)      | `TestStageA2ReadyAlerts` 8/8; `TestContractV1` con 39; `go-int` de `administration`, `operations`, `apicontract` y `database` (la segunda corrida, tras excluir la tabla nueva del conteo de `TestStage9HUpgrade`); Vitest 97 archivos, 925; Playwright 5/5 (Chromium) y 2/2 (WebKit). La verificación del coordinador vio fallar `workflow-stagea2-terminals` sola (leía Go antes de que terminara el guardado); corregida en `084f46a`: Chromium 3/3 y WebKit 1/1 | —                                                                                                                                                                                                                                                                                             |
| C, dispositivo (`60d6228`)                           | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                         | JVM 301; instrumentadas 118, 0 fallas, en `ADEL_A1_Tablet_API26` (con la migración 5 → 6 en Android 8) y `Pixel_8`; `precheck` 7/7 en los dos; `takeaway` 8/8 en los dos con la corrección `0cd678a` (la lista de Para llevar cerraba la aplicación en Android 8 por una clave repetida)      |
| D, dispositivo (`80e8ea9`, `c4de974`)                | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                         | JVM 316; instrumentadas 123, 0 fallas, en `Pixel_Tablet` y `ADEL_A1_Phone_API33`; `alerts` 10/10 (`Pixel_Tablet`) y 9/10 + `r1 r8` 3/3 (teléfono: `r8b` necesitó root para matar el proceso, `ce0f36b`); `MY_PACKAGE_REPLACED` arranca el servicio sin abrir la aplicación; batería 5 minutos |
| Fusión de C y D (`0357705`)                          | —                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | JVM 337, 0 fallas                                                                                                                                                                                                                                                                             |
| Correcciones de la revisión, servidor (`e3a3a88`)    | `TestStageA2ReadyReview` 4/4 (las reproducciones del revisor, ahora al revés); `TestStageA2ReadyAlerts` 9/9; `TestContractV1`; `TestStageA2Upgrade` y `TestStage9HUpgrade`; `go-int` de `database`, `administration`, `operations`, `kitchen` y `apicontract`; medición de R5                                                                                                                                                                                       | —                                                                                                                                                                                                                                                                                             |
| A2-D12, servidor (`b91ae66`)                         | `TestStageA2ReadyReview` 4/4 y `TestStageA2ReadyAlerts` 9/9; `go-unit`; `go-int` de `administration`, `apicontract` y `kitchen`; `TestStageA2Upgrade`                                                                                                                                                                                                                                                                                                               | —                                                                                                                                                                                                                                                                                             |
| Correcciones de la revisión, dispositivo (`fff2322`) | Sin cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                         | JVM 344, 0 fallas (con `ReadyAlertsReviewTest` del revisor); `:app` instrumentadas 100 (`Pixel_Tablet`) y 101 (`ADEL_A1_Phone_API33`), 0 fallas; `alerts` 11/11 en los dos (con `r10`); `precheck` 7/7 (`Pixel_Tablet`); canal callado 10 minutos                                             |
| Punta de la etapa                                    | «Regresión final» (abajo)                                                                                                                                                                                                                                                                                                                                                                                                                                           | «Regresión final» (Fase E)                                                                                                                                                                                                                                                                    |

**Notas de las fases.** La Fase D corrió `alerts` contra el servidor de la parte D **anterior** a las correcciones de la
revisión (el cliente con el protocolo ajustado, `c4de974`, era compatible con el viejo); por eso `alerts` y `precheck`
se repitieron contra el servidor corregido (`fff2322`). El cuelgue de `w2` en la Fase A (instrumentación arrancada sin
abrir ninguna actividad en 10 minutos, sin pila de Java) no se repitió en las demás corridas de las fases.

## Revisión independiente (§4: servicio en primer plano, reconexión y entrega sin duplicados ni pérdidas)

De solo lectura, en dos partes, con pruebas propias que reproducen cada hallazgo (`TestStageA2ReadyReview` en Go y
`ReadyAlertsReviewTest` en la JVM); las correcciones las hicieron los agentes de cada parte y las reproducciones
quedaron en el repositorio **al revés** (exigiendo lo correcto). Detalle en el avance, «Revisión independiente».

**Parte 1 · servidor** (sobre `394cab6`). El orden del cursor, los locks y la ausencia de importes estaban bien; tres
caminos perdían avisos en silencio.

| Hallazgo                                                                                                                                                                      | Corrección                                                                                                                                                                                                                                                                      |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| R1 · una unidad quedaba «Listo» al cancelarse la parte pendiente, sin aviso                                                                                                   | `recordReadyNotices` registra también la unidad que queda lista por una cancelación en la misma transacción                                                                                                                                                                     |
| R2 · un mesero con una sesión vigente de antes del inicio del día no recibía el aviso fijado a él                                                                             | «Entró en el dispositivo» es la misma regla que `active`; después, **A2-D12** (abajo)                                                                                                                                                                                           |
| R3 · tras restaurar un respaldo, los avisos nuevos repetían números ya vistos y se descartaban                                                                                | `ready_notices.alert_key` y `key` en cada aviso: el dispositivo descarta por `key`; `id` es solo el cursor                                                                                                                                                                      |
| R4 · el canal mandaba un lote cada 750 ms sin novedades (≈ 750 KB por hora y dispositivo)                                                                                     | Solo escribe con novedades, `: ping` cada 15 s, cierre a los 5 minutos; `last_seen_at` del dispositivo como mucho una vez por minuto                                                                                                                                            |
| R5 · las lecturas de sesiones crecían sin límite                                                                                                                              | Índices parciales en la 0026 (0,01–0,07 ms con 580 000 sesiones sintéticas, antes 4,6–4,9 ms)                                                                                                                                                                                   |
| R6 · `interrupted` con `id: 0` y también por errores pasajeros                                                                                                                | Sin `id`, con `{"code"}`; solo `DEVICE_REVOKED` es retiro                                                                                                                                                                                                                       |
| R7–R12 · dispositivo fijado que no podía recibir, interbloqueo posible, día comercial, cuentas combinadas o canceladas, primera lectura sin cursor, 429 tras un cambio de red | Dispositivo elegido entre los que pueden recibir; lock compartido antes de la fila del dispositivo; R9 queda como regla del día comercial (en la guía); cuenta vigente y solo unidades activas; solo lo posterior a la entrada del mesero; un canal nuevo reemplaza al anterior |

**Parte 2 · dispositivo** (sobre la Fase D fusionada). No encontró pérdidas ni repeticiones dentro del día; lo
importante era **cuándo se detiene** el servicio.

| Hallazgo                                                                                                                                                                                               | Corrección                                                                                                                                                                                     |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| RD1 · sin la computadora principal, pasado el fin del día el servicio reintentaba para siempre                                                                                                         | A2-D12: pasados 10 minutos de `day_ends_at` sin alcanzar a Go y sin nadie usando el dispositivo, se detiene sin mover el cursor                                                                |
| RD2 · con sesiones de 12 h, `active` seguía en `true` toda la noche                                                                                                                                    | **A2-D12** en el servidor: una sesión de antes del inicio del día solo cuenta si se usó en los últimos 30 minutos; `r10` comprueba que «¿Quién eres?» no usa ninguna sesión                    |
| RD3–RD7 · arranque tras reiniciar que preguntaba a Go, excepciones que tumbaban el proceso, texto de la notificación, permiso de notificaciones que no se volvía a pedir, lote ilegible que se saltaba | Decide con lo guardado; reintento guardado; «Recibiendo avisos» solo con el canal abierto; se vuelve a ofrecer en cada entrada con conexión y Mesas lo avisa; reconecta desde el último cursor |
| RD8 · temporizadores sin _wake lock_                                                                                                                                                                   | Diseño aceptado; **no verificado** sin dispositivo físico                                                                                                                                      |
| RD9 · un aviso ya descartado vuelve una vez si el proceso muere entre mostrar y guardar                                                                                                                | Diseño aceptado (mostrar y después guardar: nunca perder)                                                                                                                                      |

## Cobertura de la etapa A2

Correspondencia con las pruebas de §6 de la especificación. Rutas de Android desde `android/` (JVM en `src/test`,
instrumentadas en `src/androidTest`); del servidor, desde `backend/`. Los pasos `t`, `p` y `r` son de
`scripts/android-e2e.sh --suite takeaway`, `precheck` y `alerts` (`TakeawayOnGoTest`, `PrecheckOnGoTest` y
`AlertsOnGoTest`), donde la prueba hace de computadora principal, de cocina (KDS por la API), de caja y del agente de
impresión de una terminal «Caja 2» por la escucha local del Go temporal.

| Prueba exigida (§6)                                                                          | Dónde y qué                                                                                                                                                                                                                                                                                                                       |
| -------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Unitarias: destinatario y agrupación de avisos                                               | `ReadyAlertsRulesTest` (una notificación por cuenta con el acumulado, texto A2-D10 con singular y plural, canales A2-D7); en Go, `TestStageA2ReadyAlerts` (destinatario, varias sesiones, transferir)                                                                                                                             |
| Unitarias: avisos sin duplicados y reanudación tras un corte                                 | `ReadyAlertsRulesTest` (repetidos, proceso reiniciado, proceso muerto entre mostrar y guardar, respaldo restaurado), `ReadyAlertsChannelTest` (`Last-Event-ID`/`after`, `: ping`, cierre normal, `interrupted`), `RoomReadyAlertsLedgerTest`, `ReadyAlertsReviewTest`; en Go, `TestStageA2ReadyAlerts` y `TestStageA2ReadyReview` |
| Unitarias: estados de Para llevar                                                            | `TakeawayRulesTest` (preparación, pago, entrega, permisos, límites, la cola con el destino para llevar), `TakeawayQueueTest`, `TakeawayViewModelTest`                                                                                                                                                                             |
| Unitarias: mensajes de impresión                                                             | `PrecheckRulesTest` (cada estado y problema del trabajo, cada rechazo, «Reintentar»), `PrecheckRepositoryTest`, `PrecheckScreensTest`                                                                                                                                                                                             |
| Para llevar: nuevo pedido sin productos que no deja nada                                     | `t2`; `TakeawayQueueTest`                                                                                                                                                                                                                                                                                                         |
| Para llevar: captura y envío en línea y sin conexión                                         | `t3` y `t4`–`t5` (red cortada por el guion: nace al enviarse con `destination.takeaway`)                                                                                                                                                                                                                                          |
| Para llevar: nombre y teléfono después                                                       | `t4` (sin conexión: se envía al volver), `t6` (en línea); en Go, `TestStageA2Takeaway`                                                                                                                                                                                                                                            |
| Para llevar: lista con preparación y pago                                                    | `t7` (cocina y caja desde la principal, al momento)                                                                                                                                                                                                                                                                               |
| Para llevar: entrega bloqueada sin pago                                                      | `t8` («Cobrar en caja antes de entregar» y «Entregado» ya pagado); en Go, `TestStageA2Takeaway` (`BALANCE_PENDING`, `finance` nulo sin `payments.read`)                                                                                                                                                                           |
| Precuenta: asignar impresora en Terminales y nombre visible                                  | `p1`, `p4` (PRECUENTA, TICKET DE VENTA y CUENTA en el botón y en el impreso), `p5` (pedido para llevar); Playwright `workflow-stagea2-terminals`; Vitest `stageA2Terminals.test.tsx`                                                                                                                                              |
| Precuenta: sin impresora asignada                                                            | `p2`; en Go, `TestStageA2PrecheckPrinter` (`PRINTER_NOT_CONFIGURED` con `scope: "device"`, nunca la general de Cobros)                                                                                                                                                                                                            |
| Precuenta: sin conexión                                                                      | `p6` (red cortada por el guion; nada queda en la cola)                                                                                                                                                                                                                                                                            |
| Precuenta: impresora de una terminal desconectada                                            | `p3` («La Caja 2 está desconectada» y sale al volver); `p7` (no responde y «Reintentar» sin motivo)                                                                                                                                                                                                                               |
| Precuenta: contenido verificado en los bytes ESC/POS                                         | `p3`–`p5` (el agente simulado reclama el trabajo: título según el nombre visible, «Atendió: <mesero>», folio, mesa o pedido para llevar y partidas, CP850); en Go, `TestStageA2PrecheckPrinter`                                                                                                                                   |
| Aviso: pantalla apagada y aplicación cerrada                                                 | `r2` (tarea quitada de recientes, pantalla apagada, sin cargador y Doze `IDLE` forzado); `r8a`/`r8b` (`kill -9` y reinicio pegajoso)                                                                                                                                                                                              |
| Aviso: no llega a otro mesero ni a otro dispositivo; transferir                              | `r3` (segundo dispositivo con otro mesero; cuenta transferida antes del «Listo»)                                                                                                                                                                                                                                                  |
| Aviso: corte de red durante el «Listo», una sola vez                                         | `r4` (el guion corta la red; una notificación con un solo aviso)                                                                                                                                                                                                                                                                  |
| Aviso: agrupado por mesa                                                                     | `r5` (acumulado de la misma cuenta y resumen con varias)                                                                                                                                                                                                                                                                          |
| Aviso: al tocarlo abre la cuenta y pide el PIN                                               | `r6` (en «¿Quién eres?»; con la cuenta a la vista, aviso dentro de la aplicación)                                                                                                                                                                                                                                                 |
| Aviso: sonido y vibración apagados desde Terminales                                          | `r7` (un solo canal «Platillos listos» = `listos_silencio`; «Avisar platillos listos» apagado detiene el servicio)                                                                                                                                                                                                                |
| Aviso: el servicio se detiene al retirar el dispositivo                                      | `r9`                                                                                                                                                                                                                                                                                                                              |
| Aviso: «¿Quién eres?» no mantiene viva la sesión (A2-D12)                                    | `r10` (`last_seen_at` sin cambio en 450 s con la pantalla encendida y apagada)                                                                                                                                                                                                                                                    |
| Batería: una hora en el emulador (referencia)                                                | Fase D: 5 y 10 minutos (abajo); la hora completa, **pendiente de la Fase E**                                                                                                                                                                                                                                                      |
| Identificador: la aplicación nueva instala, empareja y funciona; paso de la 0.1.0 a la 0.2.0 | Fase A (`access` y `waiter` con la 0.1.0 instalada antes, que el guion desinstala); Fase E: con la 0.1.0 emparejada, siguiendo la guía paso a paso (`ADEL_A1_Phone_API33`, «Regresión final»)                                                                                                                                     |
| Regresión completa (servidor, escritorio y Android)                                          | «Regresión final»                                                                                                                                                                                                                                                                                                                 |

## Batería (referencia, no es una medición real)

`dumpsys batterystats com.adelpos.meseros` en `Pixel_Tablet` con el servicio en primer plano (`specialUse`), la pantalla
apagada, sin cargador, Doze forzado y la wifi apagada (red móvil simulada). **El perfil de energía del emulador es
ficticio** y el emulador no suspende la CPU como un equipo real: las cifras solo sirven para comparar.

| Corrida                                                     | Red (recibido / enviado)                | CPU (usuario / sistema) | Estimado               | Notas                                                                                                                                                 |
| ----------------------------------------------------------- | --------------------------------------- | ----------------------- | ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| Fase D, protocolo anterior (un lote cada 750 ms), 5 minutos | 103,6 KB / 25,4 KB (459 / 461 paquetes) | 817 / 125 ms            | 0,00125 mAh            | ≈ 1,55 MB por hora                                                                                                                                    |
| Fase D, protocolo corregido, canal callado 10 minutos       | 19,56 KB / 11,33 KB (93 / 97 paquetes)  | 400 / 50 ms             | 0,00123 mAh            | ≈ 185 KB por hora (`: ping` cada 15 s y la reapertura TLS cada 5 minutos); ninguna alarma ni _wake lock_ de la aplicación; 29 despertares de la radio |
| Fase E, una hora con un mesero y un «Listo» cada 10 minutos | pendiente de la Fase E                  | pendiente de la Fase E  | pendiente de la Fase E | pendiente de la Fase E                                                                                                                                |

## No verificado

No se afirma ninguna de estas:

- **Dispositivo físico** (no se conectó ninguno; parada de la especificación): **Doze real y la suspensión de la CPU**
  con la pantalla apagada (los temporizadores del canal no toman _wake lock_, RD8: en un equipo real pueden alargarse
  hasta la siguiente ventana de mantenimiento o el siguiente paquete), los **gestores de batería de cada marca**, la
  **wifi del restaurante**, el sonido real del canal, el diálogo real de la exclusión de la optimización de batería y el
  flujo completo en un teléfono y una tableta reales. Tampoco la búsqueda NSD del servidor real de punta a punta (como
  en la A1).
- **Papel real**: ninguna precuenta salió en una impresora física; los bytes ESC/POS se revisaron en el agente
  simulado.
- **«Forzar detención» o «Detener» del Administrador de tareas** (`REASON_USER_REQUESTED`): solo se probó en la JVM la
  decisión de la ayuda; la guía dice que paran los avisos hasta abrir la aplicación.
- **Lo que la guía dice como limitación**: la entrada **sin conexión** no le dice a Go que el mesero entró (no recibe
  avisos en ese dispositivo hasta su primera entrada con conexión); el día comercial no se «cierra» con ninguna acción
  (termina a la «Hora de inicio del día»); lo que cocina terminó antes de esa hora y no llegó (un corte que la cruza) ya
  no se avisa (R9); un nombre o teléfono cambiado sin conexión sale cuando ese mesero vuelve a tener conexión con su
  sesión.
- **«Cerrar sesión» explícito (A2-D4)**: la aplicación no lo tiene («Cambiar de mesero» no cierra sesiones, regla 15 de
  la A1) y no se agregó; el destinatario lo resuelve Go con la sesión de uso más reciente y A2-D12.
- **El canal de avisos con muchos dispositivos a la vez** bajo carga real (cada canal revisa cada 750 ms con 7820148305
  compartido); R5 solo midió las consultas de sesiones con datos sintéticos.
- **Una restauración real** de un respaldo con avisos (R3 se reprodujo simulándola en `ready_notices`).
- **El aviso dentro de la aplicación con dos `MainActivity`** (multiventana): la última que cambia de pantalla manda.
- **Lo que la Fase E dejó sin verificar**: el reinicio real en `Pixel_Tablet` y `ADEL_A1_Phone_API33` (se hizo en
  `Pixel_8` y en Android 8), la negación del diálogo de batería («Deny»; solo «Allow»), el diálogo de notificaciones de
  Android 17 (en `Pixel_8` solo «Ahora no» en la explicación), la pantalla de Ajustes de batería, la barra inferior en
  horizontal en `ADEL_A1_Phone_API33` (se vio en `Pixel_8`), si la pantalla del PIN se desplaza en el teléfono
  horizontal (solo se ven las teclas 1–6; es de la A1) y el APK firmado en el paso de la 0.1.0 a la 0.2.0 (se usó el
  `app-debug.apk`). `Pixel_Tablet` y la batería de una hora: pendiente de la Fase E.
- **Instalar la 0.2.0 junto a una 0.1.0 emparejada con datos reales**: no hay dispositivos reales con la 0.1.0.
- **El lector con Docker real** (`TestLegacyPrepareWithDocker`): Docker Desktop apagado; el código del lector no cambió
  en la etapa.
- **Windows**: nada de la etapa se ejecutó en Windows (solo `GOOS=windows` para Go, también con `integration` y
  `legacydocker`).
- **El escritorio real**: no se abrió la aplicación de Tauri; Terminales › Editar se probó con Vitest y Playwright.
- Pendientes de etapas anteriores sin cambio: los de la A1 (dispositivo físico, servicio real) y los de la 9G y la 9H.

## Regresión final

### Servidor y escritorio

Una sola vez (§0, «Eficiencia»), sobre una **copia exacta de `61b8ba1`** (`git archive`, punta de `etapa-a2-android`;
`git rev-parse` = `61b8ba18645736562eacc387e0dbfbbee90a3cdf`; 1 759 archivos), sin `.git`, con el arnés aislado
(`ISO_TAG=F`): clúster temporal 55499 con una base desechable por paquete y por corrida de Playwright; Playwright con su
Go temporal en 8183 y la vista previa en 1421, y un segundo Go limpio (`ISO_TAG=FC`, 8187, uno nuevo por corrida
completa) para las cuatro pruebas de «instalación limpia» de la 9H. Go 1.27.3, Playwright 1.65.0. 18/11/2026, de 14:27
a 15:40 (hora de Monterrey), con la Mac **cargada** (emuladores y Gradle de la Fase D a la vez). **No se cambió
código**: la copia quedó idéntica por md5.

| Suite                                                                                | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | Tiempo           |
| ------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------- |
| `fmt-check` (comprobación de datos reales, Prettier, `gofmt`, `cargo fmt`)           | Aprobado; la comprobación de datos reales, también sobre un índice de git propio de `61b8ba1` (1 759 archivos)                                                                                                                                                                                                                                                                                                                                                                           | 18 s             |
| `sync-check`                                                                         | «Metadatos coherentes.»                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | < 1 s            |
| `go-unit`                                                                            | Aprobado: `go vet` (normal e `integration`), `go test -race -count=1 ./...` (**35 paquetes**, todos aprobados), `GOOS=windows` `go build` y `go vet`                                                                                                                                                                                                                                                                                                                                     | 69 s             |
| `go vet -tags 'integration legacydocker'`, en macOS y con `GOOS=windows`             | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | 5 s              |
| Integración PostgreSQL, los **14 paquetes** de `scripts/database.py`                 | **14 de 14**: `database` 12,8 s, `administration` 220,2 s, `catalog` 19,6 s, `operations` 454,2 s, `payments` 2,3 s, `reports` 141,6 s, `reports/operational` 42,5 s, `designer` 22,1 s, `identity` 19,6 s, `apicontract` 37,0 s, `backupschedule` 7,0 s, `cmd/ad-el-pos-server` 63,1 s, `discovery` 5,2 s y `legacyimport` 11,7 s; con `TestStageA2Upgrade`, `TestStageA2Takeaway`, `TestStageA2PrecheckPrinter`, `TestStageA2ReadyAlerts`, `TestStageA2ReadyReview` y `TestContractV1` | 1 093 s de reloj |
| `TestOperationalReports` y `TestServiceHostOnMacOS` con `-v`                         | Aprobadas, **ninguna omitida** (`reports/operational` a las 14:49, fuera de la franja de 00:00 a 05:00; 16 subpruebas)                                                                                                                                                                                                                                                                                                                                                                   | 42 s y 58 s      |
| Docker real del lector                                                               | **No se corrió**: Docker Desktop apagado                                                                                                                                                                                                                                                                                                                                                                                                                                                 | —                |
| `vitest` (`tsc --noEmit` + Vitest)                                                   | **97 archivos, 925 pruebas**                                                                                                                                                                                                                                                                                                                                                                                                                                                             | 26 s             |
| `build` (`npm run build`)                                                            | Aprobado (el aviso de tamaño de fragmento de siempre)                                                                                                                                                                                                                                                                                                                                                                                                                                    | 9 s              |
| `cargo test --locked --offline`                                                      | **107 aprobadas, 3 ignoradas**                                                                                                                                                                                                                                                                                                                                                                                                                                                           | 33 s             |
| `scripts/macos-pkg/test-scripts.sh` y `bash -n`                                      | «Scripts del instalador: todo correcto» (31 comprobaciones); `bash -n` de los scripts del instalador, `build-macos-pkg.sh`, `check-legacy-data.sh` y `android-e2e.sh`                                                                                                                                                                                                                                                                                                                    | 3 s              |
| **Playwright Chromium** completo (44 pruebas, 34 especificaciones, con el Go limpio) | **42 de 44** en dos corridas iguales: falla `workflow-stagea2-terminals` (hallazgo, abajo) y `workflow-zz-test-data` en cadena                                                                                                                                                                                                                                                                                                                                                           | 631 y 633 s      |
| **Playwright WebKit** completo, igual                                                | 1.ª corrida: 34 de 44 (`workflow-stage9g-two-terminals`, la intermitencia de la capa «Sin conexión» que registró la A1, y 9 en cadena); **repetida: 42 de 44**, las mismas dos que en Chromium                                                                                                                                                                                                                                                                                           | 611 y 630 s      |
| `workflow-stagea2-terminals` sola (base nueva) y con `workflow-zz-test-data`         | 1 de 1 y 2 de 2 en cada motor: las 44 aprobaron en cada motor, **pero no en una misma corrida completa**                                                                                                                                                                                                                                                                                                                                                                                 | 15–19 s          |

**Hallazgo de la regresión: el límite de emparejamiento en la suite completa.** Go acepta **5 solicitudes de
emparejamiento por grupo de origen cada 10 minutos** (`pairingPerSource` y `pairingWindow`,
`backend/internal/administration/pairing.go:70-71`, etapa 9G). En la suite completa todas salen de `127.0.0.1` contra el
mismo Go temporal, y la de `workflow-stagea2-terminals` («Tableta de precuentas A2») era la **sexta en menos de dos
minutos** (después de las tres de `workflow-stage9g-terminals`, la de `workflow-stage9g-two-terminals` y la de
`workflow-stagea1-pin`): `429 PAIRING_RATE_LIMITED`. Es de la prueba, no del producto: Go aplica el límite como está
especificado. En las partes C y D nunca se vio porque Playwright corrió filtrado (la de la A2 era la quinta). La prueba
se corrigió en **`a4bd335`** (con `PAIRING_RATE_LIMITED` reintenta cada 20 s hasta 11 minutos, con un plazo de 15
minutos) y en **`5f0bf3e`** (entra primero con `installDriver`, que configura la base, y después pide el código: con la
corrección anterior, sola y con una base nueva, Go respondía `409 SETUP_REQUIRED`). Por eso la suite completa tarda unos
9 minutos más por motor.

**Corrida sobre `a4bd335`** (copia exacta, 1 760 archivos; solo cambian `desktop/e2e/pairing.ts`,
`workflow-stagea2-terminals.spec.ts` y, de la Fase D, Android y `scripts/android-e2e.sh`; de 15:45 a 16:36, con la Fase
E en marcha):

| Suite                               | Resultado                                                                                                                                                                                                                                                                                                                                                           | Tiempo          |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------- |
| `fmt-check`, `sync-check`, `vitest` | Aprobados; 97 archivos, 925 pruebas                                                                                                                                                                                                                                                                                                                                 | 17 s, 27 s      |
| **Playwright Chromium** completo    | 1.ª corrida: 34 de 44 (`workflow-stage9g-two-terminals`, «Reintentar», y 9 en cadena). **Repetida: 44 de 44**: la de la A2 esperó 9,0 minutos (28 rechazos `429`, uno cada 20 s) y aprobó entera, y `zz-test-data` también                                                                                                                                          | 10,2 y 19,5 min |
| **Playwright WebKit** completo      | Falla dos veces por pruebas intermitentes **anteriores a la etapa**: `workflow-stage9g-two-terminals` (paso 4: tras «Conservar mis productos nuevos» la terminal A volvió a mostrar el aviso de conflicto) y `workflow-stage9d-colors` (el color de «Caja en arqueo» leído justo después de cambiar el tema), cada una con su cadena; la de la A2 no llegó a correr | 10,2 y 7,6 min  |
| `workflow-stagea2-terminals` sola   | Falla en los dos motores: `409 SETUP_REQUIRED` (pedía el código antes de configurar la base); corregida en `5f0bf3e`                                                                                                                                                                                                                                                | 15 y 14 s       |

**PENDIENTE-COORDINADOR · Playwright completo**: repetir Chromium y WebKit completos (y `workflow-stagea2-terminals`
sola) sobre la cabeza de la etapa con `5f0bf3e`, al terminar la Fase E y con la Mac sin emuladores; anotar aquí el
resultado. `workflow-stage9g-two-terminals` (tres fallas en ocho corridas completas del día, en los dos motores y en dos
pasos distintos) y `workflow-stage9d-colors` (una) son intermitentes de etapas anteriores.

### Android

Fase E, sobre **`03a90e6`** (la cabeza de `etapa-a2-android` con las fases A–D y las correcciones de la revisión; el
código de Android no cambió después: `git diff --stat 03a90e6 5f0bf3e` solo toca `desktop/e2e/`), con Go temporales del
arnés construidos desde el mismo árbol (servidor 0.13.0, esquema 26, `test_mode: true`): `ISO_TAG=E` (8261/8521) y, para
dos emuladores a la vez, `ISO_TAG=EB` (8262/8522). Detalle en el avance, «Fase E».

| Qué (en la Mac, una vez)                                                    | Resultado                                                                                                                                                                                                                                                                                         |
| --------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ktfmtCheck` (`--no-build-cache --rerun-tasks`)                             | Aprobado (26 tareas)                                                                                                                                                                                                                                                                              |
| `:core:domain:test` y `testDebugUnitTest` (`--no-build-cache`)              | **344 pruebas, 0 fallas** (50 XML): `core:domain` 148, `core:data` 169, `core:ui` 9, `feature:access` 7, `feature:takeaway` 5, `feature:waiter` 6 (`app` y `core:capture` sin pruebas de la JVM)                                                                                                  |
| `lintDebug`                                                                 | 0 errores, 0 avisos (3 sugerencias de antes)                                                                                                                                                                                                                                                      |
| `assembleDebug`, `assembleRelease` (sin firmar), `assembleDebugAndroidTest` | `app-debug.apk` 15 580 729 bytes; `app-release-unsigned.apk` 2 561 215 bytes; `app-debug-androidTest.apk` 1 329 848 bytes; `com.adelpos.meseros` 0.2.0 (`versionCode` 2), `minSdk` 26, `targetSdk` 36, `compileSdk` 37; permisos del servicio en primer plano, notificaciones, arranque y batería |

Por emulador, con las aplicaciones nuevas y las de la 0.1.0 (y sus `.test`) desinstaladas antes y después:
`connectedDebugAndroidTest` con `ANDROID_SERIAL` y los XML contados por módulo (las omitidas son las pruebas contra el
Go temporal, que sin argumentos se omiten a propósito) y `scripts/android-e2e.sh --suite all` (un Go temporal nuevo
para cada parte: access 1–5, access 6–7 con otra identidad sin reinstalar, catalog, waiter, field, takeaway, precheck
y alerts; `catalog c` y `field f7` con la base temporal). Pasos aprobados / pasos, del resumen del guion.

| Prueba                                                | `ADEL_A1_Tablet_API26` (Android 8)                                                          | `ADEL_A1_Phone_API33` (Android 13)                    | `Pixel_Tablet` (Android 17) | `Pixel_8` (Android 17)                                |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------- | ----------------------------------------------------- | --------------------------- | ----------------------------------------------------- |
| Instrumentadas (XML): `app` / `core:data` / `core:ui` | **134, 0 fallas**: 101 / 20 / 13; 71 omitidas (736 s)                                       | **134, 0 fallas**: 101 / 20 / 13; 69 omitidas (239 s) | pendiente de la Fase E      | **134, 0 fallas**: 101 / 20 / 13; 69 omitidas (258 s) |
| access (1–5 y 6–7) / catalog / waiter / field         | 7/7, 3/3, 19/19, 13/13 (los cortes con `adb root` + `svc`)                                  | 7/7, 3/3, 19/19, 13/13                                | pendiente de la Fase E      | 7/7, 3/3, 19/19, 13/13                                |
| takeaway / precheck                                   | 8/8, 7/7                                                                                    | 8/8, 7/7                                              | pendiente de la Fase E      | 8/8, 7/7                                              |
| alerts (r1–r8b, r10, r9)                              | 10/11: `r4` falló (abajo); repetido, aprobado                                               | 11/11 (`r8b` con el respaldo de root)                 | pendiente de la Fase E      | 11/11                                                 |
| `--suite all` (total)                                 | 1.º intento cortado por el guion tras `w2` (abajo); 2.º **67/68**, y `r4` repetido aprobado | **68/68** (1 275 s)                                   | pendiente de la Fase E      | **68/68** (1 360 s)                                   |

**Las dos repeticiones, en Android 8 y de las pruebas o del entorno, no del producto.** (1) El primer `--suite all`
terminó sin mensaje justo después de `waiter w2` (12 de 12 aprobados hasta ahí): el corte de red por root se cayó en
`adb root` o en el primer `svc` tras reiniciar `adbd` con la Mac cargada, y el guion (`set -euo pipefail`, con la
salida de `adb root` a `/dev/null`) salió sin decir nada; tras `adb unroot` y cuatro ciclos a mano sin error, la suite
completa pasó todos los cortes por root. (2) `alerts r4` («No llegó: el canal nota el corte», 60 s): en Android 8,
`svc wifi/data disable` no siempre aborta la conexión TCP abierta, así que el canal solo la da por muerta a los 45 s
sin un byte (el diseño aprobado) y el corte del guion dura unos 12 s: la red vuelve antes, el aviso llega por la misma
conexión y el canal nunca pasa por «Waiting», que es lo que espera la prueba. Repetido con un Go nuevo y el guion tal
cual: aprobado (el aviso, una sola vez); con un corte de 64 s: el canal vio el corte a los ≈ 47 s y el aviso llegó una
sola vez ≈ 1 s después de volver la red. **El producto cumple «llega una sola vez» en Android 8.** Ninguna
instrumentación se colgó y ningún paso falló por tiempo.

**Rendimiento** (`field f2`, registro `AdelOnGo`, con la Mac cargada):

| Medida                                                 | `ADEL_A1_Tablet_API26` | `ADEL_A1_Phone_API33` | `Pixel_Tablet`         | `Pixel_8` |
| ------------------------------------------------------ | ---------------------- | --------------------- | ---------------------- | --------- |
| Arranque en frío (proceso → lista de meseros)          | 1 348 ms               | 1 823 ms              | pendiente de la Fase E | 2 413 ms  |
| PIN (con conexión) → Mesas con la primera mesa         | 771 ms                 | 840 ms                | pendiente de la Fase E | 1 006 ms  |
| PIN → la mesa 40 a la vista                            | 1 297 ms               | 1 484 ms              | pendiente de la Fase E | 1 791 ms  |
| «Nueva cuenta» → catálogo de ~300 productos a la vista | 249 ms                 | 283 ms                | pendiente de la Fase E | 314 ms    |
| «Enviar a cocina» → «Recibido por el restaurante»      | 324 ms                 | 325 ms                | pendiente de la Fase E | 325 ms    |
| Memoria (total de `dumpsys meminfo`)                   | ≈ 83 MiB               | ≈ 151 MiB             | pendiente de la Fase E | ≈ 145 MiB |

**Lo de §6 que las suites no cubren** (Fase E, con la aplicación normal fuera de la instrumentación, la wifi del
emulador apagada y el dispositivo emparejado por la dirección del Go temporal; ninguna lista mostró un servidor real):

- **Paso de la 0.1.0 a la 0.2.0 siguiendo la guía** (`ADEL_A1_Phone_API33`): la 0.1.0 (el APK de la A1, SHA-256
  `3071539e…`) emparejada con el Go temporal 0.13.0 **sigue funcionando** («Enviar a cocina» → «Recibido por el
  restaurante»); con la 0.2.0 instalada sin quitarla, Ajustes › Apps muestra **dos «AD-EL POS»** con su versión; tras
  desinstalar la 0.1.0 y revocarla (la misma ruta que «Confirmar revocación»), la 0.2.0 abre sin nada de la 0.1.0 y se
  empareja de nuevo; los meseros entran con su PIN de siempre, se ven las cuentas que abrió la 0.1.0 y llegan los
  avisos (también el de la cuenta que abrió la 0.1.0). **La guía dice lo que pasa de verdad**; la única precisión: la
  explicación del permiso de notificaciones vuelve en cada entrada con conexión mientras falte (ya en la sección 8;
  desde esta fase también en el paso 5). La 0.2.0 de esta prueba es el `app-debug.apk` (sin firmar con la llave de
  desarrollo: para Android son paquetes distintos, así que la firma no cambia lo anterior).
- **`BOOT_COMPLETED` con un reinicio real** (`adb reboot`, `boot_id` distinto, sin volver a abrir la aplicación), en
  `Pixel_8` y `ADEL_A1_Tablet_API26`: `ReadyAlertsBootReceiver` arrancó el proceso y el servicio en primer plano
  (`Background started FGS: Allowed … BOOT_COMPLETED` en Android 17, `specialUse`), «AD-EL POS conectado · Recibiendo
  avisos de platillos listos», y el «Listo» de después llegó en ≈ 2 s. En Android 8.0, Android agrega su propia
  notificación «AD-EL POS is running in the background» (es del sistema).
- **El diálogo real del permiso de notificaciones** (Android 13): «Don’t allow» → `POST_NOTIFICATIONS` negado, Mesas
  muestra «Las notificaciones de AD-EL POS están desactivadas…» con «Cómo activarlas» y el servicio corre sin
  notificación visible; en la siguiente entrada con conexión la explicación y el diálogo otra vez → «Allow» → el aviso
  de Mesas desaparece y aparece «AD-EL POS conectado». El de batería, «Allow» (`deviceidle whitelist` con la
  aplicación). En `Pixel_8`, «Ahora no» en las dos explicaciones: la de notificaciones vuelve en cada entrada (RD6).
- **El servicio y el aviso en Android 8** (`ADEL_A1_Tablet_API26`, por primera vez en la etapa): la suite `alerts`
  completa (arriba), con la pantalla apagada y Doze forzado, la aplicación cerrada, otro dispositivo, transferencia,
  agrupado, PIN en «¿Quién eres?», sonido y vibración, reinicio pegajoso y retiro.
- **Hallazgo · la barra inferior en el teléfono horizontal** (`Pixel_8`, 2400 × 1080): con `takeaway.read`, la barra
  «Mesas | Para llevar» ocupa unos 104 dp y **en la captura (de una mesa o de Para llevar) no se ve ningún producto**
  (solo la búsqueda, las categorías y «Ver la cuenta»); en vertical sí; sin la barra (mismo rol sin `takeaway.*`) se
  ven las tarjetas, cortadas. La lista de Para llevar en horizontal también pierde espacio. Las tabletas no se ven
  afectadas. No se cambió nada: **PENDIENTE-COORDINADOR · decisión** (ocultar la barra en la captura con altura
  compacta, una barra lateral en horizontal, o dejarlo y decir en la guía que en el teléfono se captura en vertical).
- **Batería, una hora** (`Pixel_Tablet`, con un mesero y un «Listo» cada 10 minutos): pendiente de la Fase E.

## Entregables

**PENDIENTE-COORDINADOR · entregables**: el APK de prueba `AD-EL-POS-Android-0.2.0.apk` (`assembleRelease` firmado
con la llave de desarrollo de `~/.android/adel-pos-dev/`, la misma de la 0.1.0: certificado SHA-256
`fb6a4bac93052ca9051342ea122ed90469340a3685de14243bdab288d71c7c86`; tamaño y SHA-256 del archivo) y el
`AD-EL POS_0.13.0.pkg` (aplicación y servidor 0.13.0; tamaño y SHA-256).

## Instalación real

Pendiente del propietario (parada de §0 de la especificación). El servicio de la Mac tiene la 0.12.0 (esquema 25). Se
actualiza con el `.pkg` 0.13.0, que instala el propietario con «Servidor del restaurante» marcada (contraseña de
administrador); al arrancar, el `host` respalda la base y aplica la 0026. Después, en cada dispositivo con la 0.1.0:
enviar lo pendiente, desinstalarla, revocarla en Terminales e instalar y emparejar la 0.2.0
([ANDROID_INSTALAR.md](ANDROID_INSTALAR.md), «Pasar de la 0.1.0 a la 0.2.0»); con la impresora de precuentas y los
avisos configurados en Terminales › Editar. Sigue pendiente de la A1 revisar en Terminales que no quedó ninguna
solicitud de emparejamiento del incidente de su Fase F.
